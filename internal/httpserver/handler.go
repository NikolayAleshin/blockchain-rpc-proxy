// Package httpserver assembles the HTTP routing and the global middleware chain:
// it maps /healthz, /readyz and the JSON-RPC proxy route, and wraps everything in
// recover -> request-id -> logging.
package httpserver

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"blockchain-rpc-proxy/internal/config"
	"blockchain-rpc-proxy/internal/health"
	"blockchain-rpc-proxy/internal/middleware"
	"blockchain-rpc-proxy/internal/proxy"
)

// Option configures optional wiring (observability) without bloating New's
// signature or coupling httpserver to otel/prometheus/sentry.
type Option func(*options)

type options struct {
	globals        []func(http.Handler) http.Handler
	metricsHandler http.Handler
}

// WithGlobalMiddleware adds middlewares applied around the whole router, just
// inside Recover and outside request-id/logging (e.g. tracing, metrics, Sentry).
// They run in the order given (first = outermost).
func WithGlobalMiddleware(mws ...func(http.Handler) http.Handler) Option {
	return func(o *options) { o.globals = append(o.globals, mws...) }
}

// WithMetricsEndpoint registers h at GET /metrics (nil = not exposed).
func WithMetricsEndpoint(h http.Handler) Option {
	return func(o *options) { o.metricsHandler = h }
}

// New builds the top-level http.Handler. rp is the reverse proxy handler for
// JSON-RPC calls; checker serves the health endpoints.
func New(cfg *config.Config, rp http.Handler, checker *health.Checker, logger *slog.Logger, opts ...Option) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	var o options
	for _, opt := range opts {
		opt(&o)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", checker.Live)
	mux.HandleFunc("GET /readyz", checker.Ready)
	if o.metricsHandler != nil {
		mux.Handle("GET /metrics", o.metricsHandler)
	}
	// Anchor the RPC route to exactly "/"; a non-POST to "/" yields 405 from the
	// mux, other paths yield 404. Rate-limiting wraps only the RPC route (not
	// health), sheds load before the guard/proxy, and is off by default.
	rpcRoute := middleware.Chain(rpcGuard(cfg, rp), middleware.RateLimit(cfg.RateLimitRPS))
	mux.Handle("POST /{$}", rpcRoute)

	// Chain (outer -> inner): Recover -> [globals] -> request-id -> logging -> mux.
	chain := []func(http.Handler) http.Handler{middleware.Recover(logger)}
	chain = append(chain, o.globals...)
	chain = append(chain, middleware.RequestID, middleware.Logging(logger))
	return middleware.Chain(mux, chain...)
}

// rpcGuard bounds the request body and validates JSON-RPC framing before
// forwarding. Malformed JSON is answered locally with -32700 (SRS FR-2.2) instead
// of spending an upstream round-trip; the (small, bounded) body is then handed to
// the proxy unchanged, preserving byte-parity.
func rpcGuard(cfg *config.Config, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, cfg.MaxBodyBytes)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				proxy.WriteError(w, http.StatusRequestEntityTooLarge, nil, proxy.CodeInvalidRequest, "request body too large")
				return
			}
			proxy.WriteError(w, http.StatusBadRequest, nil, proxy.CodeParseError, "could not read request body")
			return
		}

		switch proxy.ClassifyBody(body) {
		case proxy.KindInvalid, proxy.KindEmpty:
			// JSON-RPC is transport-agnostic: return HTTP 200 with a -32700 body.
			proxy.WriteError(w, http.StatusOK, nil, proxy.CodeParseError, "Parse error")
			return
		}

		// Restore the consumed body for the proxy.
		r.Body = io.NopCloser(bytes.NewReader(body))
		r.ContentLength = int64(len(body))
		next.ServeHTTP(w, r)
	})
}
