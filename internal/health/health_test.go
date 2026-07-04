package health

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLive_AlwaysOK(t *testing.T) {
	c := NewChecker(nil, 0, 0)
	rec := httptest.NewRecorder()
	c.Live(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

func TestReady_ReachableAndNot(t *testing.T) {
	cases := []struct {
		name string
		ping Pinger
		want int
	}{
		{"nil ping => ready", nil, http.StatusOK},
		{"reachable", func(context.Context) error { return nil }, http.StatusOK},
		{"unreachable", func(context.Context) error { return errors.New("dial fail") }, http.StatusServiceUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := NewChecker(tc.ping, time.Second, 0)
			rec := httptest.NewRecorder()
			c.Ready(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
			if rec.Code != tc.want {
				t.Errorf("status = %d, want %d", rec.Code, tc.want)
			}
		})
	}
}

func TestReady_CachesResult(t *testing.T) {
	var calls int
	c := NewChecker(func(context.Context) error { calls++; return nil }, time.Second, time.Minute)
	c.now = func() time.Time { return time.Unix(1000, 0) } // frozen clock within ttl

	for i := 0; i < 3; i++ {
		c.Ready(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/readyz", nil))
	}
	if calls != 1 {
		t.Errorf("ping called %d times, want 1 (result should be cached within ttl)", calls)
	}
}

func TestReady_NoCacheWhenTTLZero(t *testing.T) {
	var calls int
	c := NewChecker(func(context.Context) error { calls++; return nil }, time.Second, 0)
	for i := 0; i < 3; i++ {
		c.Ready(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/readyz", nil))
	}
	if calls != 3 {
		t.Errorf("ping called %d times, want 3 (no caching)", calls)
	}
}
