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

// New builds the top-level http.Handler. rp is the reverse proxy handler for
// JSON-RPC calls; checker serves the health endpoints.
func New(cfg *config.Config, rp http.Handler, checker *health.Checker, logger *slog.Logger) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", checker.Live)
	mux.HandleFunc("GET /readyz", checker.Ready)
	// Anchor the RPC route to exactly "/"; a non-POST to "/" yields 405 from the
	// mux, other paths yield 404.
	mux.Handle("POST /{$}", rpcGuard(cfg, rp))

	return middleware.Chain(mux,
		middleware.Recover(logger),
		middleware.RequestID,
		middleware.Logging(logger),
	)
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
