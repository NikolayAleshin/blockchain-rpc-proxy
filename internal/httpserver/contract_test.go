package httpserver_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"blockchain-rpc-proxy/internal/config"
	"blockchain-rpc-proxy/internal/health"
	"blockchain-rpc-proxy/internal/httpserver"
	"blockchain-rpc-proxy/internal/proxy"
)

// Contract tests assert the JSON-RPC 2.0 wire contract against golden fixtures:
// successful single/batch responses are byte-parity-forwarded from the upstream,
// and the proxy's own error envelopes (-32700/-32603/-32600) are stable.

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func handler(t *testing.T, rp http.Handler, env map[string]string) http.Handler {
	t.Helper()
	cfg, err := config.Load(func(k string) string { return env[k] })
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	return httpserver.New(cfg, rp, health.NewChecker(nil, 0, 0), discardLogger())
}

func proxyTo(t *testing.T, upstreamURL string, env map[string]string) http.Handler {
	t.Helper()
	if env == nil {
		env = map[string]string{}
	}
	env["UPSTREAM_URL"] = upstreamURL
	cfg, err := config.Load(func(k string) string { return env[k] })
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	return handler(t, proxy.New(cfg, nil, discardLogger()), env)
}

func golden(t *testing.T, name string) any {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	return decode(t, b)
}

func decode(t *testing.T, b []byte) any {
	t.Helper()
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("decode json %q: %v", b, err)
	}
	return v
}

func TestContract(t *testing.T) {
	tests := []struct {
		name       string
		reqBody    string
		env        map[string]string
		wantStatus int
		wantGolden string
		// upstreamBody, if set, is served by a fake upstream (success cases).
		upstreamBody string
		// failTransport injects a transport error (internal-error case).
		failTransport bool
	}{
		{
			name:         "single success (byte-parity)",
			reqBody:      `{"jsonrpc":"2.0","id":1,"method":"eth_blockNumber"}`,
			wantStatus:   http.StatusOK,
			wantGolden:   "single_response.json",
			upstreamBody: `{"jsonrpc":"2.0","id":1,"result":"0x557fae6"}`,
		},
		{
			name:         "batch success (array in/out)",
			reqBody:      `[{"jsonrpc":"2.0","id":1,"method":"eth_chainId"},{"jsonrpc":"2.0","id":2,"method":"eth_blockNumber"}]`,
			wantStatus:   http.StatusOK,
			wantGolden:   "batch_response.json",
			upstreamBody: `[{"jsonrpc":"2.0","id":1,"result":"0x89"},{"jsonrpc":"2.0","id":2,"result":"0x557fae6"}]`,
		},
		{
			name:       "parse error -32700",
			reqBody:    `{ not json `,
			wantStatus: http.StatusOK,
			wantGolden: "parse_error.json",
		},
		{
			name:          "internal error -32603 on upstream failure",
			reqBody:       `{"jsonrpc":"2.0","id":1,"method":"eth_blockNumber"}`,
			wantStatus:    http.StatusBadGateway,
			wantGolden:    "internal_error.json",
			failTransport: true,
		},
		{
			name:       "too large -32600",
			reqBody:    `{"jsonrpc":"2.0","id":1,"method":"eth_blockNumber","params":[]}`,
			env:        map[string]string{"MAX_BODY_BYTES": "8"},
			wantStatus: http.StatusRequestEntityTooLarge,
			wantGolden: "too_large.json",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var h http.Handler
			switch {
			case tc.failTransport:
				rt := roundTripFunc(func(*http.Request) (*http.Response, error) {
					return nil, errors.New("connection refused")
				})
				cfg, _ := config.Load(func(k string) string { return map[string]string{"UPSTREAM_URL": "https://polygon.drpc.org"}[k] })
				h = handler(t, proxy.New(cfg, rt, discardLogger()), tc.env)
			case tc.upstreamBody != "":
				up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(tc.upstreamBody))
				}))
				defer up.Close()
				h = proxyTo(t, up.URL, tc.env)
			default:
				h = proxyTo(t, "https://polygon.drpc.org", tc.env)
			}

			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tc.reqBody))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body=%s)", rec.Code, tc.wantStatus, rec.Body.String())
			}
			got := decode(t, rec.Body.Bytes())
			want := golden(t, tc.wantGolden)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("contract mismatch:\n got = %v\nwant = %v", got, want)
			}
		})
	}
}
