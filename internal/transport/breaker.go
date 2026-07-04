package transport

import (
	"errors"
	"net/http"
	"sync"
	"time"
)

// ErrOpen is returned when the breaker is open and fails fast without calling the
// upstream. ReverseProxy's ErrorHandler turns it into a JSON-RPC 502.
var ErrOpen = errors.New("upstream circuit breaker open")

type cbState int

const (
	cbClosed cbState = iota
	cbOpen
	cbHalfOpen
)

// breakerRT is a small circuit breaker: it opens after `threshold` consecutive
// upstream failures, fails fast while open, then allows a single probe after
// `cooldown` (half-open) to test recovery.
type breakerRT struct {
	next      http.RoundTripper
	threshold int
	cooldown  time.Duration
	now       func() time.Time

	mu       sync.Mutex
	state    cbState
	failures int
	openedAt time.Time
}

// NewBreaker wraps next with a circuit breaker.
func NewBreaker(next http.RoundTripper, threshold int, cooldown time.Duration) http.RoundTripper {
	if threshold <= 0 {
		threshold = 5
	}
	return &breakerRT{next: next, threshold: threshold, cooldown: cooldown, now: time.Now}
}

func (b *breakerRT) RoundTrip(req *http.Request) (*http.Response, error) {
	if !b.allow() {
		return nil, ErrOpen
	}
	resp, err := b.next.RoundTrip(req)
	b.record(isUpstreamFailure(resp, err))
	return resp, err
}

// allow reports whether a call may proceed, transitioning open -> half-open once
// the cooldown has elapsed.
func (b *breakerRT) allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.state == cbOpen {
		if b.now().Sub(b.openedAt) >= b.cooldown {
			b.state = cbHalfOpen
			return true
		}
		return false
	}
	return true
}

func (b *breakerRT) record(failed bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if failed {
		b.failures++
		if b.state == cbHalfOpen || (b.state == cbClosed && b.failures >= b.threshold) {
			b.state = cbOpen
			b.openedAt = b.now()
		}
		return
	}
	// success closes a half-open breaker and resets the counter
	b.state = cbClosed
	b.failures = 0
}
