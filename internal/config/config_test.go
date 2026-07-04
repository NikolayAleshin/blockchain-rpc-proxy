package config_test

import (
	"testing"
	"time"

	"blockchain-rpc-proxy/internal/config"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoad_Defaults(t *testing.T) {
	cfg, err := config.Load(env(nil))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ListenAddr != ":8080" {
		t.Errorf("ListenAddr = %q, want :8080", cfg.ListenAddr)
	}
	if cfg.UpstreamURL.String() != "https://polygon.drpc.org" {
		t.Errorf("UpstreamURL = %q, want https://polygon.drpc.org", cfg.UpstreamURL)
	}
	if cfg.UpstreamTimeout != 30*time.Second {
		t.Errorf("UpstreamTimeout = %s, want 30s", cfg.UpstreamTimeout)
	}
	if cfg.MaxRetries != 2 {
		t.Errorf("MaxRetries = %d, want 2", cfg.MaxRetries)
	}
	// Transparency-preserving defaults: these are off.
	if cfg.RateLimitRPS != 0 || cfg.CBEnabled || cfg.MetricsEnabled || cfg.OTLPEndpoint != "" {
		t.Errorf("expected non-transparent features off by default, got %+v", cfg)
	}
	if cfg.MaxBodyBytes != 1<<20 {
		t.Errorf("MaxBodyBytes = %d, want 1048576", cfg.MaxBodyBytes)
	}
}

func TestLoad_Overrides(t *testing.T) {
	cfg, err := config.Load(env(map[string]string{
		"LISTEN_ADDR":      ":9000",
		"UPSTREAM_URL":     "http://localhost:1234/rpc",
		"UPSTREAM_TIMEOUT": "5s",
		"MAX_RETRIES":      "0",
		"RATE_LIMIT_RPS":   "50",
		"CB_ENABLED":       "true",
		"METRICS_ENABLED":  "true",
		"WRITE_TIMEOUT":    "10s",
	}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ListenAddr != ":9000" {
		t.Errorf("ListenAddr = %q", cfg.ListenAddr)
	}
	if cfg.UpstreamURL.String() != "http://localhost:1234/rpc" {
		t.Errorf("UpstreamURL = %q", cfg.UpstreamURL)
	}
	if cfg.UpstreamTimeout != 5*time.Second || cfg.MaxRetries != 0 {
		t.Errorf("timeouts/retries not applied: %+v", cfg)
	}
	if cfg.RateLimitRPS != 50 || !cfg.CBEnabled || !cfg.MetricsEnabled {
		t.Errorf("feature toggles not applied: %+v", cfg)
	}
}

func TestLoad_Invalid(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
	}{
		{"bad duration", map[string]string{"UPSTREAM_TIMEOUT": "not-a-duration"}},
		{"bad int", map[string]string{"MAX_RETRIES": "abc"}},
		{"bad bool", map[string]string{"CB_ENABLED": "maybe"}},
		{"non-absolute url", map[string]string{"UPSTREAM_URL": "polygon.drpc.org"}},
		{"bad url scheme", map[string]string{"UPSTREAM_URL": "ftp://example.com"}},
		{"write < upstream timeout", map[string]string{"UPSTREAM_TIMEOUT": "60s", "WRITE_TIMEOUT": "30s"}},
		{"zero read header timeout", map[string]string{"READ_HEADER_TIMEOUT": "0s"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := config.Load(env(c.env)); err == nil {
				t.Errorf("expected error for %s, got nil", c.name)
			}
		})
	}
}
