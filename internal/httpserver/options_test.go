package httpserver_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"blockchain-rpc-proxy/internal/config"
	"blockchain-rpc-proxy/internal/health"
	"blockchain-rpc-proxy/internal/httpserver"
	"blockchain-rpc-proxy/internal/proxy"
)

func TestOptions_MetricsEndpointAndGlobalMiddleware(t *testing.T) {
	cfg, err := config.Load(func(k string) string {
		return map[string]string{"UPSTREAM_URL": "https://polygon.drpc.org"}[k]
	})
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}

	var wrapped bool
	global := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			wrapped = true
			next.ServeHTTP(w, r)
		})
	}
	metrics := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("# metrics"))
	})

	h := httpserver.New(cfg, proxy.New(cfg, nil, discardLogger()),
		health.NewChecker(nil, 0, 0), discardLogger(),
		httpserver.WithGlobalMiddleware(global),
		httpserver.WithMetricsEndpoint(metrics),
	)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if rec.Code != http.StatusOK || rec.Body.String() != "# metrics" {
		t.Errorf("/metrics = %d %q, want 200 '# metrics'", rec.Code, rec.Body.String())
	}
	if !wrapped {
		t.Error("global middleware was not applied")
	}
}
