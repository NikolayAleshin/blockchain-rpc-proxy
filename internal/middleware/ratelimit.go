package middleware

import (
	"net/http"

	"golang.org/x/time/rate"

	"blockchain-rpc-proxy/internal/proxy"
)

// RateLimit returns a handler-level global token-bucket limiter that sheds load
// before any upstream work. rps <= 0 disables it (pass-through), which is the
// default, keeping the proxy a transparent drop-in. Over the limit → 429.
func RateLimit(rps float64) func(http.Handler) http.Handler {
	if rps <= 0 {
		return func(next http.Handler) http.Handler { return next }
	}
	burst := int(rps)
	if burst < 1 {
		burst = 1
	}
	limiter := rate.NewLimiter(rate.Limit(rps), burst)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !limiter.Allow() {
				w.Header().Set("Retry-After", "1")
				proxy.WriteError(w, http.StatusTooManyRequests, nil, proxy.CodeInternalError, "rate limit exceeded")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
