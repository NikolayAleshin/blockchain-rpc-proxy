package transport

import (
	"context"
	"io"
	"net/http"
	"time"
)

// timeoutRT bounds each attempt with a context deadline. The deadline must
// outlive reading of the response body, so cancellation is deferred to Body.Close
// via cancelReadCloser instead of firing right after RoundTrip returns.
type timeoutRT struct {
	next http.RoundTripper
	d    time.Duration
}

// NewTimeout wraps next so every attempt is bounded by d (0 disables).
func NewTimeout(next http.RoundTripper, d time.Duration) http.RoundTripper {
	return &timeoutRT{next: next, d: d}
}

func (t *timeoutRT) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.d <= 0 {
		return t.next.RoundTrip(req)
	}
	ctx, cancel := context.WithTimeout(req.Context(), t.d)
	resp, err := t.next.RoundTrip(req.WithContext(ctx))
	if err != nil {
		cancel()
		return nil, err
	}
	resp.Body = &cancelReadCloser{ReadCloser: resp.Body, cancel: cancel}
	return resp, nil
}

// cancelReadCloser cancels the per-attempt context when the response body is
// closed, so streaming reads are not aborted prematurely.
type cancelReadCloser struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (c *cancelReadCloser) Close() error {
	c.cancel()
	return c.ReadCloser.Close()
}
