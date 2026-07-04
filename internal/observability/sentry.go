package observability

import (
	"net/http"
	"strconv"
	"time"

	"github.com/getsentry/sentry-go"

	"blockchain-rpc-proxy/internal/config"
)

// Sentry provides opt-in error tracking. It is enabled only when SENTRY_DSN is
// set; otherwise every method is a no-op and Middleware is pass-through.
type Sentry struct {
	enabled bool
}

// InitSentry initializes the global Sentry client when a DSN is configured. The
// optional configure funcs let tests inject a transport.
func InitSentry(cfg *config.Config, configure ...func(*sentry.ClientOptions)) (*Sentry, error) {
	if cfg.SentryDSN == "" {
		return &Sentry{}, nil
	}
	opts := sentry.ClientOptions{
		Dsn:           cfg.SentryDSN,
		ServerName:    cfg.OTelServiceName,
		EnableTracing: false,
	}
	for _, fn := range configure {
		fn(&opts)
	}
	if err := sentry.Init(opts); err != nil {
		return &Sentry{}, err
	}
	return &Sentry{enabled: true}, nil
}

// Enabled reports whether Sentry reporting is active.
func (s *Sentry) Enabled() bool { return s.enabled }

// Flush waits for buffered events to be sent (best effort).
func (s *Sentry) Flush(d time.Duration) {
	if s.enabled {
		sentry.Flush(d)
	}
}

// Middleware captures panics (then re-panics for the Recover middleware to build
// the response) and reports 5xx responses. It is pass-through when disabled.
func (s *Sentry) Middleware(next http.Handler) http.Handler {
	if !s.enabled {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				hub := sentry.CurrentHub().Clone()
				hub.RecoverWithContext(r.Context(), rec)
				hub.Flush(2 * time.Second)
				panic(rec) // let Recover produce the JSON-RPC 500 response
			}
		}()

		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)

		if sw.status >= 500 {
			sentry.WithScope(func(scope *sentry.Scope) {
				scope.SetTag("http.status", strconv.Itoa(sw.status))
				scope.SetTag("http.method", r.Method)
				sentry.CaptureMessage("server error: HTTP " + strconv.Itoa(sw.status))
			})
		}
	})
}
