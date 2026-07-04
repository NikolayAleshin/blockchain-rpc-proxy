// Package health serves liveness and readiness endpoints. Liveness reports that
// the process is up; readiness reports whether the upstream RPC is reachable.
// Health checks are always answered locally and never forwarded upstream.
package health

import (
	"context"
	"io"
	"net/http"
	"sync"
	"time"
)

// Pinger performs a cheap upstream-reachability check. It should return nil when
// the upstream is reachable (any HTTP response counts) and an error otherwise.
type Pinger func(context.Context) error

// Checker answers /healthz and /readyz. Readiness results are cached for ttl to
// avoid hammering the upstream on frequent probes.
type Checker struct {
	ping    Pinger
	timeout time.Duration
	ttl     time.Duration
	now     func() time.Time

	mu        sync.Mutex
	checkedAt time.Time
	lastErr   error
	has       bool
}

// NewChecker builds a Checker. ping may be nil (readiness then always succeeds).
// timeout bounds each ping; ttl caches the last result (0 = no caching).
func NewChecker(ping Pinger, timeout, ttl time.Duration) *Checker {
	return &Checker{ping: ping, timeout: timeout, ttl: ttl, now: time.Now}
}

// Live reports process liveness (always 200 while serving).
func (c *Checker) Live(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, `{"status":"ok"}`)
}

// Ready reports 200 when the upstream is reachable, 503 otherwise.
func (c *Checker) Ready(w http.ResponseWriter, r *http.Request) {
	if err := c.check(r.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, `{"status":"unavailable"}`)
		return
	}
	writeJSON(w, http.StatusOK, `{"status":"ready"}`)
}

func (c *Checker) check(ctx context.Context) error {
	if c.ping == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.has && c.ttl > 0 && c.now().Sub(c.checkedAt) < c.ttl {
		return c.lastErr
	}
	if c.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}
	err := c.ping(ctx)
	c.lastErr, c.checkedAt, c.has = err, c.now(), true
	return err
}

func writeJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}
