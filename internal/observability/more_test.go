package observability

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNormalizeEndpoint(t *testing.T) {
	cases := map[string]string{
		"host:4317":          "http://host:4317",
		"http://host:4317":   "http://host:4317",
		"https://host:4317":  "https://host:4317",
		"collector.svc:4317": "http://collector.svc:4317",
	}
	for in, want := range cases {
		if got := normalizeEndpoint(in); got != want {
			t.Errorf("normalizeEndpoint(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLogger_WithAttrsAndGroup(t *testing.T) {
	var buf bytes.Buffer
	l := NewLogger(&buf, "info").With("service", "proxy").WithGroup("http")
	l.Info("msg", "status", 200)
	if buf.Len() == 0 {
		t.Fatal("expected log output")
	}
	if !bytes.Contains(buf.Bytes(), []byte(`"service":"proxy"`)) {
		t.Errorf("WithAttrs lost: %s", buf.String())
	}
}

func TestRecorder_WriteImpliesOK(t *testing.T) {
	m := NewMetrics()
	h := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("hello")) // no explicit WriteHeader
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (implicit)", rec.Code)
	}
	if rec.Body.String() != "hello" {
		t.Errorf("body = %q, want hello", rec.Body.String())
	}
}

func TestSetupTracing_EnabledLazy(t *testing.T) {
	cfg := mustConfig(t, map[string]string{"OTEL_EXPORTER_OTLP_ENDPOINT": "localhost:4317"})
	tel, err := SetupTracing(context.Background(), cfg, NewLogger(&bytes.Buffer{}, "error"))
	if err != nil {
		t.Fatalf("SetupTracing: %v", err)
	}
	if !tel.TracingEnabled {
		t.Error("tracing should be enabled when an OTLP endpoint is set")
	}
	// Shutdown flushes (no spans queued); bound it so a missing collector can't hang.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = tel.Shutdown(ctx)
}
