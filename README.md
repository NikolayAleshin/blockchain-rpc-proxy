# blockchain-rpc-proxy

[![CI](https://github.com/NikolayAleshin/blockchain-rpc-proxy/actions/workflows/ci.yml/badge.svg)](https://github.com/NikolayAleshin/blockchain-rpc-proxy/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/NikolayAleshin/blockchain-rpc-proxy/graph/badge.svg)](https://codecov.io/gh/NikolayAleshin/blockchain-rpc-proxy)
[![Go](https://img.shields.io/badge/go-1.26-00ADD8?logo=go)](go.mod)
[![License: MIT](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)

A small, production-shaped **JSON-RPC 2.0 proxy** in Go that sits in front of the
public Polygon RPC endpoint [`https://polygon.drpc.org`](https://polygon.drpc.org)
and exposes **the same RPC interface** as its upstream. Point any Ethereum/Polygon
client at the proxy URL instead of the upstream — no code changes.

Built as a take-home: clean architecture, real tests, resilience, observability and
Terraform IaC to AWS ECS Fargate — **without over-engineering a stateless proxy**.

## Features

- **Drop-in JSON-RPC 2.0 proxy** — transparent single + batch passthrough; every
  upstream method works by construction (no method whitelist).
- **Resilience** — per-attempt timeout (core) + opt-in retry with backoff, circuit
  breaker, and rate limiting, as a composable `http.RoundTripper` chain.
- **Observability (opt-in)** — OpenTelemetry traces, Prometheus metrics, Sentry
  errors, and `trace_id`-correlated structured logs.
- **Health & lifecycle** — `/healthz`, `/readyz`, hardened `http.Server` timeouts,
  graceful shutdown.
- **Container** — multi-stage build to a distroless, non-root image (~17.7 MB).
- **Infrastructure as Code** — Terraform/OpenTofu for AWS ECS Fargate + ALB, with
  autoscaling, zero-downtime rollout, and remote state.
- **CI/CD** — lint, race tests + coverage, `govulncheck`, image scan + SBOM, IaC
  scan, and an OIDC-based release pipeline.
- **Tested** — unit / integration / contract / fuzz, plus a live differential parity
  test proving the proxy matches the upstream; core coverage ~90%+.

## Design at a glance

- **stdlib-first:** `net/http` + `net/http/httputil.ReverseProxy` (`Rewrite`, not the
  deprecated `Director`). No web framework.
- **Transparent passthrough:** the body stays opaque on the hot path, so forwarding is
  byte-identical to the upstream and every method — single **and** batch — works
  automatically. Malformed JSON is answered locally with `-32700`.
- **Resilience & observability as `http.RoundTripper`s** on `ReverseProxy.Transport`:
  per-attempt timeout is core; retry / breaker / rate-limit are opt-in and off-or-light
  by default to preserve drop-in behavior. That `Transport` is also the clean, mockable
  seam used in tests (a fake `RoundTripper` — no sockets).
- **12-factor config** via environment variables.

```text
client ─POST /─▶  recover → request-id → logging → [otel span] → [metrics] → [sentry]
                     ├─ GET /healthz /readyz /metrics        (served locally)
                     └─ POST /  → rpcGuard (bounded body, -32700 on bad JSON)
                                    └─ httputil.ReverseProxy (Rewrite → upstream)
                                         └─ Transport: timeout → breaker? → retry?
                                              → otelhttp → net/http ─▶ polygon.drpc.org
```

## Quickstart

```bash
# run locally
make run                               # or: go run ./cmd/proxy

# proxy a real call (drop-in replacement for the upstream)
curl -s -X POST http://localhost:8080 \
  -H 'content-type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"eth_blockNumber"}'
# => {"jsonrpc":"2.0","id":1,"result":"0x..."}

# batch works too (array in / array out)
curl -s -X POST http://localhost:8080 -H 'content-type: application/json' \
  -d '[{"jsonrpc":"2.0","id":1,"method":"eth_chainId"},{"jsonrpc":"2.0","id":2,"method":"eth_blockNumber"}]'

# tests
make test-race                         # go test -race ./...
make cover                             # coverage
go test -tags=e2e ./test/e2e/...       # opt-in live smoke test vs real upstream
```

## Docker

```bash
make docker-build                      # multi-stage -> distroless:nonroot (~17.7 MB)
docker run --rm -p 8080:8080 polygon-rpc-proxy
# or the full local run with metrics:
docker compose up
```

The image is a static, non-root binary on `distroless/static`; the container
`HEALTHCHECK` is shell-free (`/proxy healthcheck`).

## Configuration (environment variables)

Copy `.env.example` to `.env` for local runs. Real secrets belong in AWS SSM.

| Env var | Default | Meaning |
|---------|---------|---------|
| `LISTEN_ADDR` | `:8080` | Bind address |
| `UPSTREAM_URL` | `https://polygon.drpc.org` | Upstream RPC endpoint |
| `UPSTREAM_TIMEOUT` | `30s` | Per-attempt upstream timeout |
| `READ_HEADER_TIMEOUT` / `READ_TIMEOUT` / `WRITE_TIMEOUT` / `IDLE_TIMEOUT` | `5s`/`15s`/`30s`/`60s` | `http.Server` timeouts (slowloris guard) |
| `SHUTDOWN_TIMEOUT` | `15s` | Graceful-shutdown drain deadline |
| `MAX_BODY_BYTES` | `1048576` | Max request body (bounded read) |
| `MAX_RETRIES` / `RETRY_BACKOFF` | `2` / `100ms` | Retry on transport errors / 502/503/504 |
| `RATE_LIMIT_RPS` | `0` | Global RPS limit (0 = disabled) |
| `CB_ENABLED` | `false` | Circuit breaker |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | *(empty)* | Empty → tracing export disabled (no-op) |
| `METRICS_ENABLED` | `false` | Expose Prometheus `/metrics` |
| `SENTRY_DSN` | *(empty)* | Enable Sentry error tracking |
| `LOG_LEVEL` | `info` | `debug`/`info`/`warn`/`error` |

## Observability (opt-in)

One OpenTelemetry-instrumented service; **export is opt-in**, so the proxy runs
identically with zero telemetry infra:

- **Traces** — set `OTEL_EXPORTER_OTLP_ENDPOINT` to export OTLP spans (inbound +
  upstream, with W3C propagation). Empty → no-op.
- **Metrics** — `METRICS_ENABLED=true` exposes Prometheus RED metrics at `/metrics`.
- **Logs** — structured JSON (`slog`) with a per-request `X-Request-ID`, and
  `trace_id`/`span_id` when tracing is active.
- **Errors** — `SENTRY_DSN` enables Sentry (panics + 5xx). Off by default.

## Deployment (AWS ECS Fargate, Terraform)

Terraform/OpenTofu provisions **ECS Fargate** behind an **ALB** (health check →
`/healthz`), with ECR, CloudWatch Logs, IAM, autoscaling (CPU + request-count),
zero-downtime rollout (deployment circuit breaker + auto-rollback) and remote state
(S3 + DynamoDB lock). Full guide and cost/teardown notes:
**[`deploy/terraform/README.md`](deploy/terraform/README.md)**.

```bash
cd deploy/terraform/envs/dev
tofu init -backend=false && tofu validate   # validate without cloud
# ... configure backend + creds, then: tofu apply   (tofu destroy to tear down)
```

- `enable_nat=false` (default) runs tasks in public subnets — **no NAT gateway**
  (~$32/mo saved) for a cheap, short-lived review. `enable_nat=true` = private posture.

## Project layout

```text
cmd/proxy/                entrypoint, healthcheck subcommand, graceful shutdown
internal/config/          env config + validation
internal/proxy/           ReverseProxy (Rewrite) + JSON-RPC helpers
internal/transport/       resilience RoundTripper chain (timeout/retry/breaker)
internal/middleware/      recover, request-id, logging, rate limit
internal/health/          liveness + readiness (upstream ping, TTL cache)
internal/httpserver/      routing + middleware composition (+ contract golden tests)
internal/observability/   OpenTelemetry, Prometheus, Sentry (opt-in)
test/e2e/                 opt-in live smoke test (-tags=e2e)
deploy/terraform/         ECS Fargate IaC (modules: network, ecs-service; envs/dev)
Dockerfile, docker-compose.yaml
.github/workflows/        ci.yml (lint/test/vuln/docker+scan/terraform), release.yml (OIDC)
```

## Testing

Standard-library `testing` + `net/http/httptest`, table-driven, contract tests against
golden fixtures, and a fuzz target for input classification. Unit/integration tests use
an in-process fake upstream or a fake `http.RoundTripper` — **no real network**,
`-race`-clean. Tagged live tests hit the real upstream on demand:

```bash
go test -tags=e2e ./test/e2e/...   # smoke + differential parity
```

The **differential parity** test proves no method is dropped or altered: for a method
from every RPC category (chain, net, web3, blocks, gas, account, sync) it sends the same
request to the proxy and to the upstream and asserts they match — status, results, and
even the upstream's own errors (e.g. `-32601` for a method the node doesn't support).
Because forwarding is transparent, method coverage equals the upstream's by construction.

## Scope & trade-offs

- **In:** JSON-RPC single + batch passthrough, resilience, observability, tests,
  Docker, Terraform (ECS Fargate), CI/CD + supply-chain scanning.
- **Out (deliberately, for a "simple" proxy):** response caching (would diverge from
  upstream), auth/API keys, WebSocket `eth_subscribe`, multi-upstream failover.
- **Assumptions:** the upstream is a standard Ethereum JSON-RPC 2.0 endpoint needing no
  API key; health checks are local and never forwarded; non-transparent resilience and
  all telemetry export ship off by default, so the proxy stays a true drop-in.

## Future work

Response caching for immutable methods (e.g. `eth_chainId`), API keys / multi-tenancy,
WebSocket `eth_subscribe`, multi-upstream failover, and the observability edition's
target-state (local Grafana/Tempo/Loki stack, ADOT sidecar on ECS, image signing).

## License

[MIT](LICENSE)
