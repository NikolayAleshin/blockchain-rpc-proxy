package proxy_test

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"blockchain-rpc-proxy/internal/config"
	"blockchain-rpc-proxy/internal/proxy"
)

// roundTripFunc adapts a function to an http.RoundTripper (a fake upstream
// transport, so tests need no sockets).
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func testConfig(t *testing.T, upstreamURL string) *config.Config {
	t.Helper()
	cfg, err := config.Load(mapEnv(map[string]string{"UPSTREAM_URL": upstreamURL}))
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	return cfg
}

func serve(rp http.Handler, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	rp.ServeHTTP(rec, req)
	return rec
}

func TestProxy_ForwardsAndPreservesResponse(t *testing.T) {
	const reqBody = `{"jsonrpc":"2.0","id":1,"method":"eth_blockNumber"}`
	const respBody = `{"jsonrpc":"2.0","id":1,"result":"0xfd2fdb"}`

	var gotMethod, gotPath string
	var gotUpstreamBody string
	var gotContentType string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		gotContentType = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		gotUpstreamBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Upstream-Marker", "yes")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, respBody)
	}))
	defer upstream.Close()

	rp := proxy.New(testConfig(t, upstream.URL), nil, discardLogger())
	rec := serve(rp, reqBody)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Body.String(); got != respBody {
		t.Errorf("response body = %q, want %q", got, respBody)
	}
	if got := rec.Header().Get("X-Upstream-Marker"); got != "yes" {
		t.Errorf("upstream response header not propagated: %q", got)
	}
	// Request forwarded verbatim.
	if gotUpstreamBody != reqBody {
		t.Errorf("upstream received body %q, want %q (must be byte-identical)", gotUpstreamBody, reqBody)
	}
	if gotMethod != http.MethodPost || gotPath != "/" {
		t.Errorf("upstream got %s %s, want POST /", gotMethod, gotPath)
	}
	if gotContentType != "application/json" {
		t.Errorf("upstream Content-Type = %q, want application/json", gotContentType)
	}
}

func TestProxy_PreservesUpstreamStatus(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot) // 418: any non-2xx must be passed through
		_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":1,"error":{"code":-32601,"message":"nope"}}`)
	}))
	defer upstream.Close()

	rp := proxy.New(testConfig(t, upstream.URL), nil, discardLogger())
	rec := serve(rp, `{"jsonrpc":"2.0","id":1,"method":"does_not_exist"}`)

	if rec.Code != http.StatusTeapot {
		t.Fatalf("status = %d, want 418 (upstream status must be preserved)", rec.Code)
	}
}

func TestProxy_BatchForwardedAsIs(t *testing.T) {
	const batch = `[{"jsonrpc":"2.0","id":1,"method":"eth_blockNumber"},{"jsonrpc":"2.0","id":2,"method":"eth_chainId"}]`

	var gotBody string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `[{"jsonrpc":"2.0","id":1,"result":"0x1"},{"jsonrpc":"2.0","id":2,"result":"0x89"}]`)
	}))
	defer upstream.Close()

	rp := proxy.New(testConfig(t, upstream.URL), nil, discardLogger())
	rec := serve(rp, batch)

	if gotBody != batch {
		t.Errorf("batch body altered: got %q, want %q", gotBody, batch)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestProxy_UpstreamFailureReturnsJSONRPCError(t *testing.T) {
	failing := roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("dial tcp: connection refused")
	})
	// A valid absolute URL is required by config validation; the fake transport
	// never actually connects.
	cfg := testConfig(t, "https://polygon.drpc.org")
	rp := proxy.New(cfg, failing, discardLogger())

	rec := serve(rp, `{"jsonrpc":"2.0","id":1,"method":"eth_blockNumber"}`)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var env struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Error   struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("response is not valid JSON: %v (%s)", err, rec.Body.String())
	}
	if env.JSONRPC != "2.0" {
		t.Errorf("jsonrpc = %q, want 2.0", env.JSONRPC)
	}
	if env.Error.Code != proxy.CodeInternalError {
		t.Errorf("error.code = %d, want %d", env.Error.Code, proxy.CodeInternalError)
	}
	if string(env.ID) != "null" {
		t.Errorf("id = %s, want null", env.ID)
	}
}
