package proxy

import (
	"encoding/json"
	"net/http"
)

// JSON-RPC 2.0 error codes (https://www.jsonrpc.org/specification#error_object).
const (
	CodeParseError     = -32700 // invalid JSON was received
	CodeInvalidRequest = -32600 // the JSON is not a valid Request object
	CodeMethodNotFound = -32601 // the method does not exist
	CodeInvalidParams  = -32602 // invalid method parameters
	CodeInternalError  = -32603 // internal JSON-RPC error (e.g. upstream failure)
)

// errorObject is the JSON-RPC 2.0 "error" member.
type errorObject struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// errorEnvelope is a full JSON-RPC 2.0 error response.
type errorEnvelope struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Error   errorObject     `json:"error"`
}

// nullID is the JSON-RPC id used when the request id cannot be determined
// (per spec, id must be null in that case).
var nullID = json.RawMessage(`null`)

// WriteError writes a JSON-RPC 2.0 error response with the given HTTP status.
// id may be nil, in which case a null id is used.
func WriteError(w http.ResponseWriter, httpStatus int, id json.RawMessage, code int, msg string) {
	if id == nil {
		id = nullID
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)
	// Encoding a small fixed struct cannot fail; ignore the (nil) error.
	_ = json.NewEncoder(w).Encode(errorEnvelope{
		JSONRPC: "2.0",
		ID:      id,
		Error:   errorObject{Code: code, Message: msg},
	})
}

// BodyKind classifies a request body without fully deserializing it.
type BodyKind int

const (
	KindInvalid BodyKind = iota // not valid JSON
	KindEmpty                   // whitespace/empty
	KindSingle                  // a single JSON-RPC object
	KindBatch                   // a JSON-RPC batch (array)
)

// ClassifyBody reports whether body is a single request, a batch, empty, or
// invalid JSON. It is transparent-proxy-safe: it never mutates the body and is
// used only for error handling (parse errors) and observability labelling.
//
// It is intentionally allocation-free and panic-free for any input, so it is a
// good fuzz target (FuzzClassifyBody).
func ClassifyBody(body []byte) BodyKind {
	first, ok := firstNonSpace(body)
	if !ok {
		return KindEmpty
	}
	if !json.Valid(body) {
		return KindInvalid
	}
	switch first {
	case '[':
		return KindBatch
	default:
		return KindSingle
	}
}

// firstNonSpace returns the first non-whitespace byte and whether one exists.
// Whitespace follows the JSON grammar: space, tab, newline, carriage return.
func firstNonSpace(b []byte) (byte, bool) {
	for _, c := range b {
		switch c {
		case ' ', '\t', '\n', '\r':
			continue
		default:
			return c, true
		}
	}
	return 0, false
}
