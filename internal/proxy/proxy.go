// Package proxy builds the transparent JSON-RPC reverse proxy to the upstream.
//
// Forwarding is done by net/http/httputil.ReverseProxy using the modern Rewrite
// hook (Director is deprecated). The request/response body stays opaque on the
// hot path, guaranteeing byte-parity with the upstream and automatic support for
// every RPC method and for batch requests. See docs/ADR/0002-transparent-passthrough.md.
package proxy

import (
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"

	"blockchain-rpc-proxy/internal/config"
)

// New returns a ReverseProxy targeting cfg.UpstreamURL.
//
// transport is used as ReverseProxy.Transport; pass nil to use
// http.DefaultTransport. In tests, inject a fake http.RoundTripper. logger may be
// nil (slog.Default is used). Later phases wrap transport with the resilience
// RoundTripper chain (timeout/retry/breaker) — the proxy itself does not change.
func New(cfg *config.Config, transport http.RoundTripper, logger *slog.Logger) *httputil.ReverseProxy {
	if logger == nil {
		logger = slog.Default()
	}
	target := cfg.UpstreamURL

	return &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			// SetURL routes to target's scheme/host/base-path and rewrites the
			// outbound Host header to the target host. The client's body, method
			// and content headers are forwarded unchanged.
			r.SetURL(target)
			// Drop any inbound forwarding headers we don't want to trust/echo.
			r.Out.Header.Del("X-Forwarded-Host")
		},
		Transport:    transport,
		ErrorHandler: upstreamErrorHandler(logger, target),
	}
}

// upstreamErrorHandler converts a transport-level failure (upstream unreachable,
// timeout, connection reset) into a spec-faithful JSON-RPC error rather than the
// default bare "502 Bad Gateway" text — so clients always receive JSON-RPC.
func upstreamErrorHandler(logger *slog.Logger, target *url.URL) func(http.ResponseWriter, *http.Request, error) {
	return func(w http.ResponseWriter, r *http.Request, err error) {
		logger.WarnContext(r.Context(), "upstream request failed",
			slog.String("upstream", target.Host),
			slog.String("error", err.Error()),
		)
		// The request id is not reliably recoverable here (body already streamed),
		// so we use a null id, which the JSON-RPC spec permits in this case.
		WriteError(w, http.StatusBadGateway, nil, CodeInternalError, "upstream request failed")
	}
}
