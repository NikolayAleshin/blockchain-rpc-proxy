package transport

import (
	"bytes"
	"io"
	"math/rand/v2"
	"net/http"
	"time"
)

const maxRetryBackoff = 2 * time.Second

// retryRT retries idempotent-looking failures (transport errors, 502/503/504)
// with exponential backoff + full jitter, bounded by maxRetries and — crucially —
// by the request's own context deadline.
type retryRT struct {
	next       http.RoundTripper
	maxRetries int
	base       time.Duration
}

// NewRetry wraps next with up to maxRetries additional attempts.
func NewRetry(next http.RoundTripper, maxRetries int, base time.Duration) http.RoundTripper {
	if base <= 0 {
		base = 100 * time.Millisecond
	}
	return &retryRT{next: next, maxRetries: maxRetries, base: base}
}

func (rt *retryRT) RoundTrip(req *http.Request) (*http.Response, error) {
	// Buffer the (bounded) body once so each attempt can resend it.
	body, err := drainBody(req)
	if err != nil {
		return nil, err
	}

	var resp *http.Response
	for attempt := 0; ; attempt++ {
		resp, err = rt.next.RoundTrip(cloneWithBody(req, body))
		if attempt >= rt.maxRetries || !isUpstreamFailure(resp, err) {
			return resp, err
		}
		drainResponse(resp) // free the connection before retrying
		select {
		case <-time.After(rt.backoff(attempt)):
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
	}
}

// backoff returns a full-jitter delay in (0, min(base*2^attempt, cap)].
func (rt *retryRT) backoff(attempt int) time.Duration {
	window := rt.base << attempt
	if window <= 0 || window > maxRetryBackoff {
		window = maxRetryBackoff
	}
	return time.Duration(rand.Int64N(int64(window)) + 1)
}

func drainBody(req *http.Request) ([]byte, error) {
	if req.Body == nil || req.Body == http.NoBody {
		return nil, nil
	}
	b, err := io.ReadAll(req.Body)
	_ = req.Body.Close()
	return b, err
}

func cloneWithBody(req *http.Request, body []byte) *http.Request {
	r := req.Clone(req.Context())
	if body != nil {
		r.Body = io.NopCloser(bytes.NewReader(body))
		r.ContentLength = int64(len(body))
	}
	return r
}

func drainResponse(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
}
