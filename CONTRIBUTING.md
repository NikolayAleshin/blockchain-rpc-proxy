# Contributing

Thanks for taking a look! This document covers the local workflow and the bar that
CI enforces.

## Prerequisites

- **Go 1.26+**
- **Docker** (for the container build / local observability stack)
- **OpenTofu** or **Terraform** (for the IaC under `deploy/terraform/`)

## Local workflow

```bash
make run            # run the proxy locally
make test-race      # go test -race ./...
make cover          # tests + coverage summary
make fmt            # gofmt -w -s .
make vet            # go vet ./...
make lint           # golangci-lint (if installed)
make ci             # fast local gate: fmt + vet + race tests
```

Run everything the pipeline runs before pushing: `make ci`, plus `golangci-lint run`
and `govulncheck ./...` if installed.

## Conventions

- **stdlib-first.** Don't add a dependency if the standard library does it in ~30
  lines. No web frameworks, no config engines (see the design notes in the README).
- **Proxying** is `httputil.ReverseProxy` with `Rewrite`; resilience/observability
  live in `http.RoundTripper`s and middleware, not in the handler.
- **Transparency:** never parse-and-re-serialize the JSON-RPC body on the hot path.
- **Observability export is opt-in** and must never block or fail a request.
- **Tests** are table-driven and use `httptest` / fake `http.RoundTripper` — no real
  network in unit/integration tests. Keep core coverage ≥ 80%.
- **Config** is env-only (12-factor). Update `.env.example` and the README config
  table together.
- **Commits:** clear, descriptive subjects (imperative mood). No secrets in code,
  logs, or commit messages.

## CI gates (must pass)

- `gofmt` + `golangci-lint`
- `go vet`, `go build`, `go test -race -coverprofile`
- `govulncheck` (Go deps)
- Docker build + container smoke test + `trivy` image scan (HIGH/CRITICAL) + SBOM
- `tofu fmt -check`, `tofu validate`, `tflint`, `trivy config` (IaC)

## Terraform changes

Validate without cloud access:

```bash
cd deploy/terraform/envs/dev
tofu init -backend=false && tofu validate
tofu fmt -check -recursive
```

See [`deploy/terraform/README.md`](deploy/terraform/README.md) for the apply/destroy
flow and cost notes.
