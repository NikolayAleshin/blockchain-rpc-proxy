package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"blockchain-rpc-proxy/internal/config"
)

func cfgWithUpstream(t *testing.T, url string) *config.Config {
	t.Helper()
	cfg, err := config.Load(func(k string) string {
		return map[string]string{"UPSTREAM_URL": url}[k]
	})
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	return cfg
}

func TestUpstreamPinger_Reachable(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer up.Close()

	if err := upstreamPinger(cfgWithUpstream(t, up.URL))(context.Background()); err != nil {
		t.Errorf("ping reachable upstream: %v", err)
	}
}

func TestUpstreamPinger_Unreachable(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := up.URL
	up.Close() // now nothing is listening -> connection refused

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := upstreamPinger(cfgWithUpstream(t, url))(ctx); err == nil {
		t.Error("expected error pinging a closed upstream")
	}
}
