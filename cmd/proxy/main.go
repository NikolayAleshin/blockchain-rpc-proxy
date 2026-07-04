// Command proxy is the entrypoint for the Polygon JSON-RPC proxy.
//
// Composition root: config -> upstream transport -> httputil.ReverseProxy ->
// router (health + middleware) -> http.Server, with explicit timeouts and
// graceful shutdown on SIGINT/SIGTERM. Resilience and observability layers are
// wired in later phases without changing this wiring. See docs/ARCHITECTURE.md.
package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"blockchain-rpc-proxy/internal/config"
	"blockchain-rpc-proxy/internal/health"
	"blockchain-rpc-proxy/internal/httpserver"
	"blockchain-rpc-proxy/internal/proxy"
	"blockchain-rpc-proxy/internal/transport"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	logger := newLogger(cfg.LogLevel)
	slog.SetDefault(logger)

	// Resilience chain: breaker? -> retry? -> per-attempt timeout -> base.
	roundTripper := transport.New(cfg, http.DefaultTransport)

	rp := proxy.New(cfg, roundTripper, logger)
	checker := health.NewChecker(upstreamPinger(cfg), cfg.UpstreamTimeout, 2*time.Second)
	handler := httpserver.New(cfg, rp, checker, logger)

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           handler,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		logger.Info("proxy listening",
			slog.String("addr", cfg.ListenAddr),
			slog.String("upstream", cfg.UpstreamURL.String()),
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
		stop() // restore default signal handling for a second Ctrl-C
		logger.Info("shutdown signal received, draining in-flight requests")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}

// newLogger builds a JSON slog logger at the configured level.
func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl}))
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
