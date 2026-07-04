// Command proxy is the entrypoint for the Polygon JSON-RPC proxy.
//
// Composition root: config -> telemetry -> upstream transport -> ReverseProxy ->
// router (health + middleware + observability) -> http.Server, with explicit
// timeouts and graceful shutdown on SIGINT/SIGTERM. All telemetry is opt-in; the
// proxy runs identically with it off. See docs/ARCHITECTURE.md.
package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"blockchain-rpc-proxy/internal/config"
	"blockchain-rpc-proxy/internal/health"
	"blockchain-rpc-proxy/internal/httpserver"
	"blockchain-rpc-proxy/internal/observability"
	"blockchain-rpc-proxy/internal/proxy"
	"blockchain-rpc-proxy/internal/transport"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	// `proxy healthcheck` is a shell-free container HEALTHCHECK: it GETs /healthz
	// on the local listener and exits 0/1. Used by the distroless image.
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(healthcheck())
	}
	if err := run(); err != nil {
		slog.Error("fatal", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

// healthcheck returns 0 if the local /healthz endpoint responds 200.
func healthcheck() int {
	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	_, port, err := net.SplitHostPort(addr)
	if err != nil || port == "" {
		port = "8080"
	}
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://127.0.0.1:" + port + "/healthz")
	if err != nil {
		return 1
	}
	_ = resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return 0
	}
	return 1
}

func run() error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	logger := observability.NewLogger(os.Stdout, cfg.LogLevel)
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Telemetry (opt-in). Failures degrade to no-op rather than crashing.
	tel, err := observability.SetupTracing(ctx, cfg, logger)
	if err != nil {
		logger.Warn("tracing setup failed; continuing without export", slog.String("error", err.Error()))
		tel = &observability.Telemetry{}
	}
	sentryClient, err := observability.InitSentry(cfg)
	if err != nil {
		logger.Warn("sentry init failed; continuing without it", slog.String("error", err.Error()))
	}

	// Upstream transport: otelhttp (client spans + propagation, no-op when tracing
	// is off) wrapped by the resilience chain.
	base := otelhttp.NewTransport(http.DefaultTransport)
	rp := proxy.New(cfg, transport.New(cfg, base), logger)
	checker := health.NewChecker(upstreamPinger(cfg), cfg.UpstreamTimeout, 2*time.Second)

	// Global middlewares (outer -> inner): server span, metrics?, Sentry.
	globals := []func(http.Handler) http.Handler{
		func(next http.Handler) http.Handler { return otelhttp.NewHandler(next, "rpc") },
	}
	serverOpts := []httpserver.Option{}
	if cfg.MetricsEnabled {
		metrics := observability.NewMetrics()
		globals = append(globals, metrics.Middleware)
		serverOpts = append(serverOpts, httpserver.WithMetricsEndpoint(metrics.Handler()))
	}
	globals = append(globals, sentryClient.Middleware)
	serverOpts = append(serverOpts, httpserver.WithGlobalMiddleware(globals...))

	handler := httpserver.New(cfg, rp, checker, logger, serverOpts...)

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           handler,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("proxy listening",
			slog.String("version", version),
			slog.String("addr", cfg.ListenAddr),
			slog.String("upstream", cfg.UpstreamURL.String()),
			slog.Bool("tracing", tel.TracingEnabled),
			slog.Bool("metrics", cfg.MetricsEnabled),
			slog.Bool("sentry", sentryClient.Enabled()),
		)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-ctx.Done():
		stop()
		logger.Info("shutdown signal received, draining in-flight requests")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		err := srv.Shutdown(shutdownCtx)
		_ = tel.Shutdown(shutdownCtx)
		sentryClient.Flush(2 * time.Second)
		return err
	}
}

// upstreamPinger returns a readiness probe that POSTs a cheap eth_chainId call to
// the upstream. Any HTTP response (even an error status) means "reachable"; only a
// transport error means "not ready".
func upstreamPinger(cfg *config.Config) health.Pinger {
	client := &http.Client{}
	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"eth_chainId"}`)
	target := cfg.UpstreamURL.String()
	return func(ctx context.Context) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		_ = resp.Body.Close()
		return nil
	}
}
