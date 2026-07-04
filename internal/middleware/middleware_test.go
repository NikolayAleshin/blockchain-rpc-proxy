package middleware_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"blockchain-rpc-proxy/internal/middleware"
)

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestRequestID_GeneratesAndEchoes(t *testing.T) {
	var seen string
	h := middleware.RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = middleware.RequestIDFrom(r.Context())
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if seen == "" {
		t.Fatal("request id not present in context")
	}
	if got := rec.Header().Get("X-Request-ID"); got != seen {
		t.Errorf("response header id = %q, context id = %q; want equal", got, seen)
	}
}

func TestRequestID_ReusesInbound(t *testing.T) {
	var seen string
	h := middleware.RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = middleware.RequestIDFrom(r.Context())
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", "abc-123")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if seen != "abc-123" || rec.Header().Get("X-Request-ID") != "abc-123" {
		t.Errorf("inbound id not reused: ctx=%q header=%q", seen, rec.Header().Get("X-Request-ID"))
	}
}

func TestRecover_ReturnsJSONRPC500(t *testing.T) {
	h := middleware.Recover(discardLogger())(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	var env struct {
		Error struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	if env.Error.Code != -32603 {
		t.Errorf("error.code = %d, want -32603", env.Error.Code)
	}
}

func TestLogging_RecordsStatusAndID(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	h := middleware.Chain(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTeapot)
			_, _ = io.WriteString(w, "x")
		}),
		middleware.RequestID,
		middleware.Logging(logger),
	)
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/rpc", nil))

	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("log line not JSON: %v (%s)", err, buf.String())
	}
	if m["status"].(float64) != http.StatusTeapot {
		t.Errorf("logged status = %v, want 418", m["status"])
	}
	if m["path"] != "/rpc" {
		t.Errorf("logged path = %v, want /rpc", m["path"])
	}
	if id, _ := m["request_id"].(string); id == "" {
		t.Error("logged request_id is empty")
	}
}
