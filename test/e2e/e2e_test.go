//go:build e2e

// Package e2e contains opt-in smoke tests that hit the real upstream
// (https://polygon.drpc.org). Run with: go test -tags=e2e ./test/e2e/...
// These are excluded from normal CI to avoid flakiness and public rate limits.
package e2e

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"blockchain-rpc-proxy/internal/config"
	"blockchain-rpc-proxy/internal/health"
	"blockchain-rpc-proxy/internal/httpserver"
	"blockchain-rpc-proxy/internal/proxy"
	"blockchain-rpc-proxy/internal/transport"
)

func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	cfg, err := config.Load(os.Getenv) // defaults => https://polygon.drpc.org
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	rp := proxy.New(cfg, transport.New(cfg, http.DefaultTransport), logger)
	h := httpserver.New(cfg, rp, health.NewChecker(nil, 0, 0), logger)
	return httptest.NewServer(h)
}

func TestE2E_EthBlockNumber(t *testing.T) {
	srv := newServer(t)
	defer srv.Close()

	resp, err := http.Post(srv.URL, "application/json",
		strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"eth_blockNumber"}`))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var out struct {
		Result string `json:"result"`
		Error  any    `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Error != nil {
		t.Fatalf("rpc error: %v", out.Error)
	}
	if !strings.HasPrefix(out.Result, "0x") {
		t.Errorf("result = %q, want a 0x-prefixed block number", out.Result)
	}
	t.Logf("live eth_blockNumber => %s", out.Result)
}
