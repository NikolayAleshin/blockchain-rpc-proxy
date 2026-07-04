package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
	"go.opentelemetry.io/otel/trace"

	"blockchain-rpc-proxy/internal/config"
)

// --- logger: trace correlation ---

func TestLogger_AddsTraceIDWhenSpanPresent(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, "info")

	tid, _ := trace.TraceIDFromHex("0102030405060708090a0b0c0d0e0f10")
	sid, _ := trace.SpanIDFromHex("0102030405060708")
	sc := trace.NewSpanContext(trace.SpanContextConfig{TraceID: tid, SpanID: sid, TraceFlags: trace.FlagsSampled})
	ctx := trace.ContextWithSpanContext(context.Background(), sc)

	logger.InfoContext(ctx, "hello")

	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("log not JSON: %v", err)
	}
	if m["trace_id"] != tid.String() {
		t.Errorf("trace_id = %v, want %s", m["trace_id"], tid)
	}
	if m["span_id"] != sid.String() {
		t.Errorf("span_id = %v, want %s", m["span_id"], sid)
	}
}

func TestLogger_NoTraceWithoutSpan(t *testing.T) {
	var buf bytes.Buffer
	NewLogger(&buf, "info").Info("plain")

	var m map[string]any
	_ = json.Unmarshal(buf.Bytes(), &m)
	if _, ok := m["trace_id"]; ok {
		t.Error("trace_id should be absent without a span")
	}
}

// --- tracing: opt-in no-op path ---

func TestSetupTracing_DisabledIsNoOp(t *testing.T) {
	cfg := mustConfig(t, map[string]string{}) // no OTLP endpoint
	tel, err := SetupTracing(context.Background(), cfg, NewLogger(&bytes.Buffer{}, "error"))
	if err != nil {
		t.Fatalf("SetupTracing: %v", err)
	}
	if tel.TracingEnabled {
		t.Error("tracing should be disabled without an OTLP endpoint")
	}
	if err := tel.Shutdown(context.Background()); err != nil {
		t.Errorf("Shutdown (no-op) err = %v", err)
	}
}

// --- metrics: RED recording + scrape ---

func TestMetrics_RecordsAndExposes(t *testing.T) {
	m := NewMetrics()
	h := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", nil))

	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := rec.Body.String()

	if !bytes.Contains([]byte(body), []byte(`rpc_requests_total`)) {
		t.Errorf("/metrics missing rpc_requests_total:\n%s", body)
	}
	if !bytes.Contains([]byte(body), []byte(`status="200"`)) {
		t.Errorf("/metrics missing status label:\n%s", body)
	}
}

// --- sentry: 5xx capture via mock transport ---

type mockSentryTransport struct {
	mu     sync.Mutex
	events []*sentry.Event
}

func (t *mockSentryTransport) Configure(sentry.ClientOptions) {}
func (t *mockSentryTransport) SendEvent(e *sentry.Event) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.events = append(t.events, e)
}
func (t *mockSentryTransport) Flush(time.Duration) bool              { return true }
func (t *mockSentryTransport) FlushWithContext(context.Context) bool { return true }
func (t *mockSentryTransport) Close()                                {}
func (t *mockSentryTransport) count() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.events)
}

func TestSentry_CapturesOn5xx(t *testing.T) {
	mt := &mockSentryTransport{}
	cfg := mustConfig(t, map[string]string{"SENTRY_DSN": "http://public@localhost/1"})
	s, err := InitSentry(cfg, func(o *sentry.ClientOptions) { o.Transport = mt })
	if err != nil {
		t.Fatalf("InitSentry: %v", err)
	}
	if !s.Enabled() {
		t.Fatal("sentry should be enabled with a DSN")
	}

	h := s.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway) // 502
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", nil))
	s.Flush(time.Second)

	if mt.count() == 0 {
		t.Error("expected a Sentry event captured for a 5xx response")
	}
}

func TestSentry_DisabledIsPassthrough(t *testing.T) {
	cfg := mustConfig(t, map[string]string{}) // no DSN
	s, err := InitSentry(cfg)
	if err != nil {
		t.Fatalf("InitSentry: %v", err)
	}
	if s.Enabled() {
		t.Error("sentry should be disabled without a DSN")
	}
	called := false
	h := s.Middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", nil))
	if !called {
		t.Error("disabled Sentry middleware must be pass-through")
	}
}

func mustConfig(t *testing.T, env map[string]string) *config.Config {
	t.Helper()
	cfg, err := config.Load(func(k string) string { return env[k] })
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	return cfg
}
