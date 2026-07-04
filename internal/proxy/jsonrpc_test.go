package proxy_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"blockchain-rpc-proxy/internal/proxy"
)

// mapEnv turns a map into a getenv function for config.Load in tests.
func mapEnv(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestClassifyBody(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want proxy.BodyKind
	}{
		{"single", `{"jsonrpc":"2.0","id":1,"method":"eth_blockNumber"}`, proxy.KindSingle},
		{"single leading ws", "  \n\t{\"id\":1}", proxy.KindSingle},
		{"batch", `[{"id":1},{"id":2}]`, proxy.KindBatch},
		{"batch leading ws", "\n[{}]", proxy.KindBatch},
		{"empty", "", proxy.KindEmpty},
		{"whitespace only", "   \n\t ", proxy.KindEmpty},
		{"invalid trailing", `{"id":1}garbage`, proxy.KindInvalid},
		{"invalid unclosed", `{"id":1`, proxy.KindInvalid},
		{"invalid bare word", `notjson`, proxy.KindInvalid},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := proxy.ClassifyBody([]byte(c.in)); got != c.want {
				t.Errorf("ClassifyBody(%q) = %d, want %d", c.in, got, c.want)
			}
		})
	}
}

func TestWriteError(t *testing.T) {
	rec := httptest.NewRecorder()
	proxy.WriteError(rec, http.StatusBadRequest, nil, proxy.CodeParseError, "Parse error")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
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
		t.Fatalf("not valid JSON: %v", err)
	}
	if env.JSONRPC != "2.0" || env.Error.Code != proxy.CodeParseError || string(env.ID) != "null" {
		t.Errorf("unexpected envelope: %s", rec.Body.String())
	}
}

func TestWriteError_EchoesID(t *testing.T) {
	rec := httptest.NewRecorder()
	proxy.WriteError(rec, http.StatusOK, json.RawMessage(`42`), proxy.CodeInvalidRequest, "Invalid Request")

	var env struct {
		ID json.RawMessage `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
	if string(env.ID) != "42" {
		t.Errorf("id = %s, want 42", env.ID)
	}
}

// FuzzClassifyBody asserts ClassifyBody never panics and always returns a valid
// kind for arbitrary input (TEST_PLAN robustness target).
func FuzzClassifyBody(f *testing.F) {
	for _, seed := range []string{
		"", " ", "{}", "[]", `{"id":1}`, `[{"id":1}]`, "notjson", `{"a":`, "\x00\x01",
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, body []byte) {
		got := proxy.ClassifyBody(body)
		switch got {
		case proxy.KindInvalid, proxy.KindEmpty, proxy.KindSingle, proxy.KindBatch:
			// valid
		default:
			t.Fatalf("ClassifyBody returned unknown kind %d for %q", got, body)
		}
	})
}
