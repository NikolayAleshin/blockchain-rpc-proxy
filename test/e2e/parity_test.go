//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// Differential parity: for a representative method from every RPC category, the
// proxy's response must be equivalent to calling the upstream directly. Because
// the proxy is transparent (it forwards raw bytes and never inspects `method`),
// this proves no method is dropped or altered — coverage equals the upstream's.
//
// For methods with a deterministic result (chainId, keccak, ...) we assert exact
// equality. For volatile methods (blockNumber, gasPrice, ...) we assert the proxy
// returns a non-error result of the same JSON shape as the upstream.

const upstreamURL = "https://polygon.drpc.org"

type rpcResp struct {
	Result json.RawMessage `json:"result"`
	Error  json.RawMessage `json:"error"`
}

var parityMethods = []struct {
	name     string
	category string
	req      string
	stable   bool // deterministic result -> exact match
}{
	{"eth_chainId", "chain", `{"jsonrpc":"2.0","id":1,"method":"eth_chainId","params":[]}`, true},
	{"net_version", "net", `{"jsonrpc":"2.0","id":1,"method":"net_version","params":[]}`, true},
	{"net_listening", "net", `{"jsonrpc":"2.0","id":1,"method":"net_listening","params":[]}`, true},
	{"web3_sha3", "web3", `{"jsonrpc":"2.0","id":1,"method":"web3_sha3","params":["0x68656c6c6f"]}`, true},
	{"web3_clientVersion", "web3", `{"jsonrpc":"2.0","id":1,"method":"web3_clientVersion","params":[]}`, false},
	{"eth_blockNumber", "blocks", `{"jsonrpc":"2.0","id":1,"method":"eth_blockNumber","params":[]}`, false},
	{"eth_getBlockByNumber", "blocks", `{"jsonrpc":"2.0","id":1,"method":"eth_getBlockByNumber","params":["latest",false]}`, false},
	{"eth_gasPrice", "gas", `{"jsonrpc":"2.0","id":1,"method":"eth_gasPrice","params":[]}`, false},
	{"eth_maxPriorityFeePerGas", "gas", `{"jsonrpc":"2.0","id":1,"method":"eth_maxPriorityFeePerGas","params":[]}`, false},
	{"eth_getBalance", "account", `{"jsonrpc":"2.0","id":1,"method":"eth_getBalance","params":["0x0000000000000000000000000000000000000000","latest"]}`, false},
	{"eth_syncing", "sync", `{"jsonrpc":"2.0","id":1,"method":"eth_syncing","params":[]}`, false},
}

func TestParity_NoMethodDropped(t *testing.T) {
	srv := newServer(t)
	defer srv.Close()

	for _, m := range parityMethods {
		t.Run(m.category+"/"+m.name, func(t *testing.T) {
			pStatus, viaProxy := call(t, srv.URL, m.req)
			uStatus, viaUpstream := call(t, upstreamURL, m.req)

			// The proxy forwards the upstream's HTTP status verbatim.
			if pStatus != uStatus {
				t.Errorf("status mismatch: proxy=%d upstream=%d", pStatus, uStatus)
			}

			// If the upstream rejects the method (e.g. -32601 for one it doesn't
			// support), the proxy must forward the SAME error — parity even here.
			if len(viaUpstream.Error) > 0 {
				if string(viaProxy.Error) != string(viaUpstream.Error) {
					t.Errorf("error passthrough mismatch:\n proxy    = %s\n upstream = %s",
						viaProxy.Error, viaUpstream.Error)
				}
				return
			}

			// Upstream succeeded: the proxy must return a result, not drop or error it.
			if len(viaProxy.Error) > 0 {
				t.Fatalf("proxy errored but upstream succeeded: %s", viaProxy.Error)
			}
			if len(viaProxy.Result) == 0 {
				t.Fatalf("proxy returned no result — method dropped?")
			}

			if m.stable {
				if string(viaProxy.Result) != string(viaUpstream.Result) {
					t.Errorf("deterministic result mismatch:\n proxy    = %s\n upstream = %s",
						viaProxy.Result, viaUpstream.Result)
				}
				return
			}
			// Volatile: shapes must match (both a hex string, both an object, ...).
			if pk, uk := jsonKind(viaProxy.Result), jsonKind(viaUpstream.Result); pk != uk {
				t.Errorf("result shape mismatch: proxy=%s upstream=%s", pk, uk)
			}
		})
	}
}

func call(t *testing.T, url, body string) (int, rpcResp) {
	t.Helper()
	resp, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	var r rpcResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		t.Fatalf("decode %s: %v", url, err)
	}
	return resp.StatusCode, r
}

// jsonKind reports the top-level JSON type of a raw message.
func jsonKind(raw json.RawMessage) string {
	s := bytes.TrimSpace(raw)
	if len(s) == 0 {
		return "empty"
	}
	switch s[0] {
	case '{':
		return "object"
	case '[':
		return "array"
	case '"':
		return "string"
	case 't', 'f':
		return "bool"
	case 'n':
		return "null"
	default:
		return "number"
	}
}
