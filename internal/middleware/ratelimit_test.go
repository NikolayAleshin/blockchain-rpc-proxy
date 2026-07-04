package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"blockchain-rpc-proxy/internal/middleware"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
}

func TestRateLimit_Blocks(t *testing.T) {
	h := middleware.RateLimit(1)(okHandler()) // 1 rps, burst 1

	first := httptest.NewRecorder()
	h.ServeHTTP(first, httptest.NewRequest(http.MethodPost, "/", nil))
	if first.Code != http.StatusOK {
		t.Fatalf("first request = %d, want 200", first.Code)
	}

	second := httptest.NewRecorder()
	h.ServeHTTP(second, httptest.NewRequest(http.MethodPost, "/", nil))
	if second.Code != http.StatusTooManyRequests {
		t.Errorf("second request = %d, want 429", second.Code)
	}
	if second.Header().Get("Retry-After") == "" {
		t.Error("missing Retry-After header on 429")
	}
}

func TestRateLimit_DisabledIsPassthrough(t *testing.T) {
	h := middleware.RateLimit(0)(okHandler())
	for i := 0; i < 100; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d = %d, want 200 (limiter disabled)", i, rec.Code)
		}
	}
}
