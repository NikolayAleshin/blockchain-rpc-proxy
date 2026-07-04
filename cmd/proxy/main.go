// Command proxy is the entrypoint for the Polygon JSON-RPC proxy.
//
// Phase 0 provides a buildable skeleton. Later phases wire the composition root:
// config -> upstream transport (resilience) -> httputil.ReverseProxy -> HTTP server,
// plus health endpoints, middleware, graceful shutdown and (opt-in) observability.
// See docs/ARCHITECTURE.md and docs/CHECKLIST.md.
package main

import (
	"fmt"
	"os"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

// run holds real startup logic so main stays a thin error-to-exit-code shim.
func run() error {
	// Phase 1+ wires config, proxy, transport and server here.
	return nil
}
