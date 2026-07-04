# blockchain-rpc-proxy

A small, production-shaped **JSON-RPC 2.0 proxy** in Go that sits in front of the
public Polygon RPC endpoint [`https://polygon.drpc.org`](https://polygon.drpc.org)
and exposes **the same RPC interface** as its upstream. Point any Ethereum/Polygon
client at the proxy URL instead of the upstream — no code changes.

Built as a take-home: the goal is clean architecture, real tests, resilience,
observability and Terraform IaC to AWS ECS Fargate — **without over-engineering a
stateless proxy**.

## Status

| Area | State |
|------|-------|
| Core proxy (`ReverseProxy`) + config + JSON-RPC helpers | ✅ implemented & tested |
| HTTP server, health, middleware, graceful shutdown | 🚧 next |
| Resilience (timeout/retry/breaker/rate-limit) | 🚧 |
| Observability (OpenTelemetry, opt-in) | 🚧 |
| Docker, Terraform (ECS Fargate + ALB), CI/CD | 🚧 |

## Design at a glance

- **stdlib-first:** `net/http` + `net/http/httputil.ReverseProxy` (`Rewrite`, not the
  deprecated `Director`) do the proxying. No web framework.
- **Transparent passthrough:** the request/response body stays opaque on the hot path,
  so forwarding is byte-identical to the upstream and every method — single **and**
  batch — works automatically. (Confirmed against the dRPC contract: `POST /`,
  `content-type: application/json`, JSON-RPC 2.0, `params` optional.)
- **Resilience & observability as `http.RoundTripper`s** on `ReverseProxy.Transport`:
  per-attempt timeout is core; retry / circuit-breaker / rate-limit are opt-in and
  off-or-light by default to preserve drop-in behavior. This `Transport` is also the
  clean, mockable seam used in tests (a fake `RoundTripper` — no sockets).
- **12-factor config** via environment variables (see below).

## Quickstart

```bash
# tests
make test-race        # or: go test -race ./...
make cover            # coverage on the core

# run (once the server entrypoint lands in the next phase)
make run              # or: go run ./cmd/proxy

# proxy a real call (drop-in replacement for the upstream)
curl -s -X POST http://localhost:8080 \
  -H 'content-type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"eth_blockNumber","params":[]}'
# => {"jsonrpc":"2.0","id":1,"result":"0x..."}
```

## Configuration (environment variables)

Copy `.env.example` to `.env` for local runs. Real secrets belong in AWS SSM, never
in the repo.

| Env var | Default | Meaning |
|---------|---------|---------|
| `LISTEN_ADDR` | `:8080` | Bind address |
| `UPSTREAM_URL` | `https://polygon.drpc.org` | Upstream RPC endpoint |
| `UPSTREAM_TIMEOUT` | `30s` | Per-attempt upstream timeout |
| `READ_HEADER_TIMEOUT` | `5s` | Server slowloris guard (must be > 0) |
| `READ_TIMEOUT` / `WRITE_TIMEOUT` / `IDLE_TIMEOUT` | `15s` / `30s` / `60s` | `http.Server` timeouts |
| `SHUTDOWN_TIMEOUT` | `15s` | Graceful-shutdown drain deadline |
| `MAX_BODY_BYTES` | `1048576` | Max request body (bounded read) |
| `MAX_RETRIES` | `2` | Retry attempts on transport errors / 502/503/504 |
| `RATE_LIMIT_RPS` | `0` | Global RPS limit (0 = disabled) |
| `CB_ENABLED` | `false` | Circuit breaker (off by default) |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | *(empty)* | Empty → telemetry export disabled (no-op) |
| `METRICS_ENABLED` | `false` | Expose Prometheus `/metrics` |
| `LOG_LEVEL` | `info` | `debug`/`info`/`warn`/`error` |

## Project layout

```text
cmd/proxy/            entrypoint (composition root)
internal/config/      env-based configuration + validation
internal/proxy/       ReverseProxy + JSON-RPC helpers (error codes, body classify)
deploy/               Dockerfile, Terraform (ECS Fargate), local observability stack
.github/workflows/    CI (lint, vet, test -race, govulncheck, build)
```

## Testing

Standard-library `testing` + `net/http/httptest`, table-driven, plus a fuzz target
for input classification. Unit and integration tests use an in-process fake upstream
or a fake `http.RoundTripper` — **no real network**, deterministic and fast.

```bash
go test -race -cover ./...
```

## Deployment (planned)

Terraform provisions AWS **ECS Fargate** behind an **Application Load Balancer**
(ALB health check → `/healthz`), with ECR, CloudWatch Logs, remote state (S3 +
DynamoDB lock), and cost/security toggles (`enable_nat`). A `terraform destroy` path
is documented (the NAT gateway has an hourly cost).

## Scope & trade-offs

- **In:** JSON-RPC single + batch passthrough, resilience, observability, tests,
  Docker, Terraform (ECS Fargate), CI.
- **Out (deliberately, for a "simple" proxy):** response caching (would diverge from
  upstream), auth/API keys, WebSocket `eth_subscribe`, multi-upstream failover.
- **Assumptions:** the upstream is a standard Ethereum JSON-RPC 2.0 endpoint and
  requires no API key for basic access; health checks are served locally and never
  forwarded upstream; non-transparent resilience ships off by default so the proxy
  stays a true drop-in replacement.

## License

[MIT](./LICENSE)
