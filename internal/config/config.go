// Package config loads all runtime configuration from environment variables
// (12-factor). See docs/SRS.md §6 for the authoritative list of keys/defaults.
package config

import (
	"fmt"
	"net/url"
	"strconv"
	"time"
)

// Config is the fully-parsed, validated runtime configuration.
type Config struct {
	// Server
	ListenAddr        string
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
	MaxBodyBytes      int64

	// Upstream
	UpstreamURL     *url.URL
	UpstreamTimeout time.Duration

	// Resilience
	MaxRetries         int
	RetryBackoff       time.Duration
	RateLimitRPS       float64
	CBEnabled          bool
	CBFailureThreshold int

	// Observability
	LogLevel        string
	MetricsEnabled  bool
	OTLPEndpoint    string
	OTelServiceName string
	OTelSamplerArg  float64
	OTelSDKDisabled bool
	SentryDSN       string
}

// Load reads configuration using the provided getenv function. Passing getenv
// (rather than calling os.Getenv directly) keeps Load fully unit-testable.
func Load(getenv func(string) string) (*Config, error) {
	p := parser{getenv: getenv}

	cfg := &Config{
		ListenAddr:        p.str("LISTEN_ADDR", ":8080"),
		ReadHeaderTimeout: p.dur("READ_HEADER_TIMEOUT", 5*time.Second),
		ReadTimeout:       p.dur("READ_TIMEOUT", 15*time.Second),
		WriteTimeout:      p.dur("WRITE_TIMEOUT", 30*time.Second),
		IdleTimeout:       p.dur("IDLE_TIMEOUT", 60*time.Second),
		ShutdownTimeout:   p.dur("SHUTDOWN_TIMEOUT", 15*time.Second),
		MaxBodyBytes:      p.i64("MAX_BODY_BYTES", 1<<20),

		UpstreamTimeout: p.dur("UPSTREAM_TIMEOUT", 30*time.Second),

		MaxRetries:         p.int("MAX_RETRIES", 2),
		RetryBackoff:       p.dur("RETRY_BACKOFF", 100*time.Millisecond),
		RateLimitRPS:       p.float("RATE_LIMIT_RPS", 0),
		CBEnabled:          p.boolean("CB_ENABLED", false),
		CBFailureThreshold: p.int("CB_FAILURE_THRESHOLD", 5),

		LogLevel:        p.str("LOG_LEVEL", "info"),
		MetricsEnabled:  p.boolean("METRICS_ENABLED", false),
		OTLPEndpoint:    p.str("OTEL_EXPORTER_OTLP_ENDPOINT", ""),
		OTelServiceName: p.str("OTEL_SERVICE_NAME", "polygon-rpc-proxy"),
		OTelSamplerArg:  p.float("OTEL_TRACES_SAMPLER_ARG", 1.0),
		OTelSDKDisabled: p.boolean("OTEL_SDK_DISABLED", false),
		SentryDSN:       p.str("SENTRY_DSN", ""),
	}
	cfg.UpstreamURL = p.rawURL("UPSTREAM_URL", "https://polygon.drpc.org")

	if p.err != nil {
		return nil, p.err
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) validate() error {
	if c.UpstreamURL == nil || !c.UpstreamURL.IsAbs() || c.UpstreamURL.Host == "" {
		return fmt.Errorf("UPSTREAM_URL must be an absolute http(s) URL, got %q", c.UpstreamURL)
	}
	if s := c.UpstreamURL.Scheme; s != "http" && s != "https" {
		return fmt.Errorf("UPSTREAM_URL scheme must be http or https, got %q", s)
	}
	if c.ReadHeaderTimeout <= 0 {
		return fmt.Errorf("READ_HEADER_TIMEOUT must be > 0 (slowloris guard)")
	}
	if c.UpstreamTimeout <= 0 {
		return fmt.Errorf("UPSTREAM_TIMEOUT must be > 0")
	}
	// SRS NFR-1.6: the server must not cut off legitimate slow upstream responses.
	if c.WriteTimeout > 0 && c.WriteTimeout < c.UpstreamTimeout {
		return fmt.Errorf("WRITE_TIMEOUT (%s) must be >= UPSTREAM_TIMEOUT (%s)", c.WriteTimeout, c.UpstreamTimeout)
	}
	if c.MaxBodyBytes <= 0 {
		return fmt.Errorf("MAX_BODY_BYTES must be > 0")
	}
	return nil
}

// parser reads typed values from getenv, recording the first error encountered
// so callers can surface all-or-nothing config failures cleanly.
type parser struct {
	getenv func(string) string
	err    error
}

func (p *parser) fail(key, raw string, err error) {
	if p.err == nil {
		p.err = fmt.Errorf("invalid %s=%q: %w", key, raw, err)
	}
}

func (p *parser) str(key, def string) string {
	if v := p.getenv(key); v != "" {
		return v
	}
	return def
}

func (p *parser) dur(key string, def time.Duration) time.Duration {
	v := p.getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		p.fail(key, v, err)
		return def
	}
	return d
}

func (p *parser) int(key string, def int) int {
	v := p.getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		p.fail(key, v, err)
		return def
	}
	return n
}

func (p *parser) i64(key string, def int64) int64 {
	v := p.getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		p.fail(key, v, err)
		return def
	}
	return n
}

func (p *parser) float(key string, def float64) float64 {
	v := p.getenv(key)
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		p.fail(key, v, err)
		return def
	}
	return f
}

func (p *parser) boolean(key string, def bool) bool {
	v := p.getenv(key)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		p.fail(key, v, err)
		return def
	}
	return b
}

func (p *parser) rawURL(key, def string) *url.URL {
	raw := p.str(key, def)
	u, err := url.Parse(raw)
	if err != nil {
		p.fail(key, raw, err)
		return nil
	}
	return u
}
