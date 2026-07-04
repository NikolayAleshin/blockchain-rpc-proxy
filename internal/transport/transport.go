// Package transport composes the upstream resilience chain as http.RoundTrippers
// plugged into httputil.ReverseProxy.Transport. The proxy itself is unaware of
// resilience; swapping/adding layers here never touches proxy or handler code.
//
// Composition (outer -> inner):
//
//	breaker? -> retry? -> per-attempt timeout -> base
//
// so each retry attempt is independently timed and the breaker wraps the whole
// retry loop (we never retry into a known-down upstream). See docs/ADR/0003.
package transport

import (
	"net/http"
	"time"

	"blockchain-rpc-proxy/internal/config"
)

const defaultBreakerCooldown = 5 * time.Second

// New builds the resilience chain around base (http.DefaultTransport if nil).
// Timeout is always applied (core); retry and breaker are config-gated.
func New(cfg *config.Config, base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	rt := NewTimeout(base, cfg.UpstreamTimeout)
	if cfg.MaxRetries > 0 {
		rt = NewRetry(rt, cfg.MaxRetries, cfg.RetryBackoff)
	}
	if cfg.CBEnabled {
		rt = NewBreaker(rt, cfg.CBFailureThreshold, defaultBreakerCooldown)
	}
	return rt
}

// isUpstreamFailure reports whether a RoundTrip outcome counts as an upstream
// failure for retry/breaker purposes: a transport error, or a 502/503/504. A 200
// carrying a JSON-RPC error body is NOT a failure — we never inspect the body.
func isUpstreamFailure(resp *http.Response, err error) bool {
	if err != nil {
		return true
	}
	switch resp.StatusCode {
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	}
	return false
}
