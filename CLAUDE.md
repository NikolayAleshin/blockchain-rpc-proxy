# CLAUDE.md — repo conventions & AI guardrails

Guidance for any AI assistant (and humans) working in this repo.

## What this is
A transparent **JSON-RPC 2.0 proxy** in Go for `https://polygon.drpc.org`, deployed
to AWS ECS Fargate via Terraform. See the top-level `README.md` for the design
overview, quickstart, and trade-offs.

## Golden-path commands
- `make run` — run locally
- `make test-race` / `make cover` — tests
- `make fmt vet lint` — format & static checks
- `make ci` — fast local gate before pushing

## Conventions
- **stdlib-first.** Don't add a dependency if the standard library does it in ~30
  LOC. No web frameworks / config engines (no Gin/Viper/Cobra).
- **Proxying** is `httputil.ReverseProxy` with `Rewrite` (not the deprecated `Director`).
- **Resilience & observability** live in `http.RoundTripper`s on
  `ReverseProxy.Transport` and in middleware, not in the handler.
- **Transparency:** never parse-and-re-serialize the JSON-RPC body on the hot path.
- **Observability export is opt-in:** no `OTEL_EXPORTER_OTLP_ENDPOINT` => no-op.
  Telemetry must never block or fail a request.
- **Tests:** table-driven; fake `http.RoundTripper` / `httptest.Server`, no real
  network in unit/integration tests. Keep core coverage ≥ 80%.
- **Config:** env vars only (12-factor). Keep `.env.example` and the README config
  table in sync.

## Non-negotiable guardrails
- CI is the gate: `gofmt`, `go vet`, `golangci-lint`, `-race` tests and `govulncheck`
  must pass. No bypassing.
- **No secrets in code, logs, prompts, or commits.** Real secrets go to AWS SSM.
- Decisions about what *not* to build are as important as what to build — keep the
  scope tight and prefer the standard library.
