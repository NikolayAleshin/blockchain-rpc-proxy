package transport

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// fakeRT is a scriptable upstream transport (no sockets).
type fakeRT struct {
	calls int
	fn    func(attempt int, r *http.Request) (*http.Response, error)
}

func (f *fakeRT) RoundTrip(r *http.Request) (*http.Response, error) {
	n := f.calls
	f.calls++
	return f.fn(n, r)
}

func resp(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

func newReq(ctx context.Context, body string) *http.Request {
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://upstream.test/", strings.NewReader(body))
	if err != nil {
		panic(err)
	}
	return r
}

func TestRetry_On503ThenSuccess(t *testing.T) {
	f := &fakeRT{fn: func(attempt int, _ *http.Request) (*http.Response, error) {
		if attempt == 0 {
			return resp(http.StatusServiceUnavailable, ""), nil
		}
		return resp(http.StatusOK, "ok"), nil
	}}
	rt := NewRetry(f, 2, time.Millisecond)

	r, err := rt.RoundTrip(newReq(context.Background(), `{"id":1}`))
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if r.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", r.StatusCode)
	}
	if f.calls != 2 {
		t.Errorf("calls = %d, want 2 (one retry)", f.calls)
	}
}

func TestRetry_NoRetryOn200WithRPCError(t *testing.T) {
	f := &fakeRT{fn: func(int, *http.Request) (*http.Response, error) {
		return resp(http.StatusOK, `{"jsonrpc":"2.0","id":1,"error":{"code":-32601}}`), nil
	}}
	rt := NewRetry(f, 3, time.Millisecond)

	r, _ := rt.RoundTrip(newReq(context.Background(), `{"id":1}`))
	if r.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", r.StatusCode)
	}
	if f.calls != 1 {
		t.Errorf("calls = %d, want 1 (a 200 body is never retried)", f.calls)
	}
}

func TestRetry_ExhaustsAndReturnsLast(t *testing.T) {
	f := &fakeRT{fn: func(int, *http.Request) (*http.Response, error) {
		return resp(http.StatusServiceUnavailable, ""), nil
	}}
	rt := NewRetry(f, 2, time.Millisecond)

	r, _ := rt.RoundTrip(newReq(context.Background(), `{"id":1}`))
	if r.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", r.StatusCode)
	}
	if f.calls != 3 {
		t.Errorf("calls = %d, want 3 (1 + 2 retries)", f.calls)
	}
}

func TestRetry_TransportErrorRetried(t *testing.T) {
	f := &fakeRT{fn: func(int, *http.Request) (*http.Response, error) {
		return nil, errors.New("connection refused")
	}}
	rt := NewRetry(f, 2, time.Millisecond)

	if _, err := rt.RoundTrip(newReq(context.Background(), `{"id":1}`)); err == nil {
		t.Fatal("expected error")
	}
	if f.calls != 3 {
		t.Errorf("calls = %d, want 3", f.calls)
	}
}

func TestRetry_StopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	f := &fakeRT{fn: func(int, *http.Request) (*http.Response, error) {
		cancel() // cancel after the first attempt; backoff must abort
		return resp(http.StatusServiceUnavailable, ""), nil
	}}
	rt := NewRetry(f, 5, 50*time.Millisecond)

	_, err := rt.RoundTrip(newReq(ctx, `{"id":1}`))
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if f.calls != 1 {
		t.Errorf("calls = %d, want 1 (no further attempts after cancel)", f.calls)
	}
}

func TestTimeout_SetsPerAttemptDeadline(t *testing.T) {
	var had bool
	f := &fakeRT{fn: func(_ int, r *http.Request) (*http.Response, error) {
		_, had = r.Context().Deadline()
		return resp(http.StatusOK, "ok"), nil
	}}
	rt := NewTimeout(f, 50*time.Millisecond)

	r, err := rt.RoundTrip(newReq(context.Background(), ""))
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !had {
		t.Error("expected a per-attempt context deadline")
	}
	_ = r.Body.Close() // triggers cancel; must not panic
}

func TestTimeout_ZeroDisables(t *testing.T) {
	f := &fakeRT{fn: func(_ int, r *http.Request) (*http.Response, error) {
		if _, ok := r.Context().Deadline(); ok {
			t.Error("did not expect a deadline when timeout=0")
		}
		return resp(http.StatusOK, "ok"), nil
	}}
	rt := NewTimeout(f, 0)
	if _, err := rt.RoundTrip(newReq(context.Background(), "")); err != nil {
		t.Fatalf("err = %v", err)
	}
}

func TestBreaker_OpensThenRecovers(t *testing.T) {
	fail := true
	f := &fakeRT{fn: func(int, *http.Request) (*http.Response, error) {
		if fail {
			return resp(http.StatusServiceUnavailable, ""), nil
		}
		return resp(http.StatusOK, "ok"), nil
	}}
	b := NewBreaker(f, 2, time.Minute).(*breakerRT)
	now := time.Unix(1000, 0)
	b.now = func() time.Time { return now }

	// Two consecutive failures open the breaker.
	_, _ = b.RoundTrip(newReq(context.Background(), ""))
	_, _ = b.RoundTrip(newReq(context.Background(), ""))
	if f.calls != 2 {
		t.Fatalf("calls = %d, want 2", f.calls)
	}

	// While open, calls fail fast without touching the upstream.
	if _, err := b.RoundTrip(newReq(context.Background(), "")); !errors.Is(err, ErrOpen) {
		t.Fatalf("err = %v, want ErrOpen", err)
	}
	if f.calls != 2 {
		t.Errorf("calls = %d, want 2 (open breaker must not call upstream)", f.calls)
	}

	// After cooldown, a half-open probe is allowed; success closes the breaker.
	now = now.Add(2 * time.Minute)
	fail = false
	if _, err := b.RoundTrip(newReq(context.Background(), "")); err != nil {
		t.Fatalf("half-open probe err = %v", err)
	}
	if f.calls != 3 {
		t.Errorf("calls = %d, want 3 (probe)", f.calls)
	}
	// Closed again: subsequent calls pass through.
	_, _ = b.RoundTrip(newReq(context.Background(), ""))
	if f.calls != 4 {
		t.Errorf("calls = %d, want 4", f.calls)
	}
}

func TestBreaker_HalfOpenFailureReopens(t *testing.T) {
	f := &fakeRT{fn: func(int, *http.Request) (*http.Response, error) {
		return resp(http.StatusBadGateway, ""), nil
	}}
	b := NewBreaker(f, 1, time.Minute).(*breakerRT)
	now := time.Unix(0, 0)
	b.now = func() time.Time { return now }

	_, _ = b.RoundTrip(newReq(context.Background(), "")) // 1 failure, threshold 1 -> open
	now = now.Add(2 * time.Minute)
	_, _ = b.RoundTrip(newReq(context.Background(), "")) // half-open probe fails -> reopen
	if _, err := b.RoundTrip(newReq(context.Background(), "")); !errors.Is(err, ErrOpen) {
		t.Errorf("err = %v, want ErrOpen after half-open failure", err)
	}
}
