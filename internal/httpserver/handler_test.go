package httpserver_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"blockchain-rpc-proxy/internal/config"
	"blockchain-rpc-proxy/internal/health"
	"blockchain-rpc-proxy/internal/httpserver"
	"blockchain-rpc-proxy/internal/proxy"
)

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// buildHandler wires a full router against a fake upstream. extra env overrides
// let individual tests tweak config (e.g. MAX_BODY_BYTES).
func buildHandler(t *testing.T, upstreamURL string, extra map[string]string) http.Handler {
	t.Helper()
	env := map[string]string{"UPSTREAM_URL": upstreamURL}
	for k, v := range extra {
		env[k] = v
	}
	cfg, err := config.Load(func(k string) string { return env[k] })
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	rp := proxy.New(cfg, nil, discardLogger())
	checker := health.NewChecker(nil, 0, 0) // nil ping => always ready
	return httpserver.New(cfg, rp, checker, discardLogger())
}

func do(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestHealthz(t *testing.T) {
	h := buildHandler(t, "https://polygon.drpc.org", nil)
	rec := do(h, http.MethodGet, "/healthz", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("/healthz status = %d, want 200", rec.Code)
	}
}

func TestReadyz(t *testing.T) {
	h := buildHandler(t, "https://polygon.drpc.org", nil)
	rec := do(h, http.MethodGet, "/readyz", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("/readyz status = %d, want 200 (nil ping => ready)", rec.Code)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	h := buildHandler(t, "https://polygon.drpc.org", nil)
	rec := do(h, http.MethodGet, "/", "")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET / status = %d, want 405", rec.Code)
	}
}

func TestParseError(t *testing.T) {
	h := buildHandler(t, "https://polygon.drpc.org", nil)
	rec := do(h, http.MethodPost, "/", `{ this is not json `)

	var env struct {
		Error struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	if env.Error.Code != proxy.CodeParseError {
		t.Errorf("error.code = %d, want %d (-32700)", env.Error.Code, proxy.CodeParseError)
	}
}

func TestEmptyBodyIsParseError(t *testing.T) {
	h := buildHandler(t, "https://polygon.drpc.org", nil)
	rec := do(h, http.MethodPost, "/", "")

	var env struct {
		Error struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	if env.Error.Code != proxy.CodeParseError {
		t.Errorf("empty body: error.code = %d, want -32700", env.Error.Code)
	}
}

func TestTooLargeBody(t *testing.T) {
	h := buildHandler(t, "https://polygon.drpc.org", map[string]string{"MAX_BODY_BYTES": "16"})
	rec := do(h, http.MethodPost, "/", `{"jsonrpc":"2.0","id":1,"method":"eth_blockNumber","params":[]}`)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", rec.Code)
	}
}

func TestProxyHappyPath(t *testing.T) {
	const want = `{"jsonrpc":"2.0","id":1,"result":"0xfd2fdb"}`
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, want)
	}))
	defer upstream.Close()

	h := buildHandler(t, upstream.URL, nil)
	rec := do(h, http.MethodPost, "/", `{"jsonrpc":"2.0","id":1,"method":"eth_blockNumber"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Body.String(); got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
	if rec.Header().Get("X-Request-ID") == "" {
		t.Error("missing X-Request-ID on proxied response")
	}
}
