package transport

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"blockchain-rpc-proxy/internal/config"
)

func loadCfg(t *testing.T, env map[string]string) *config.Config {
	t.Helper()
	cfg, err := config.Load(func(k string) string { return env[k] })
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	return cfg
}

// TestNew_ComposesFullChain exercises New building breaker -> retry -> timeout ->
// base and verifies a 503 is retried to success through the whole chain.
func TestNew_ComposesFullChain(t *testing.T) {
	cfg := loadCfg(t, map[string]string{
		"MAX_RETRIES":          "2",
		"CB_ENABLED":           "true",
		"CB_FAILURE_THRESHOLD": "5",
	})
	base := &fakeRT{fn: func(attempt int, _ *http.Request) (*http.Response, error) {
		if attempt == 0 {
			return resp(http.StatusServiceUnavailable, ""), nil
		}
		return resp(http.StatusOK, "ok"), nil
	}}

	rt := New(cfg, base)
	r, err := rt.RoundTrip(newReq(context.Background(), `{"id":1}`))
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if r.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", r.StatusCode)
	}
	if base.calls != 2 {
		t.Errorf("base calls = %d, want 2 (retry through the chain)", base.calls)
	}
}

func TestNew_NilBaseDefaults(t *testing.T) {
	cfg := loadCfg(t, nil)
	if New(cfg, nil) == nil {
		t.Fatal("New returned nil")
	}
}

func TestTimeout_PropagatesTransportError(t *testing.T) {
	f := &fakeRT{fn: func(int, *http.Request) (*http.Response, error) {
		return nil, errors.New("boom")
	}}
	rt := NewTimeout(f, 0) // zero disables wrapping but error still propagates
	if _, err := rt.RoundTrip(newReq(context.Background(), "")); err == nil {
		t.Fatal("expected error")
	}

	rt2 := NewTimeout(f, 100) // with a deadline, error path must cancel + return err
	if _, err := rt2.RoundTrip(newReq(context.Background(), "")); err == nil {
		t.Fatal("expected error with deadline set")
	}
}
