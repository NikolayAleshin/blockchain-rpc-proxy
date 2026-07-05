# Security Policy

## Reporting a vulnerability

Please report suspected vulnerabilities privately via GitHub Security Advisories
(**Security → Report a vulnerability**) rather than opening a public issue. Include
a description, reproduction steps, and impact. You can expect an initial response
within a few business days.

## Supported versions

This is a take-home project; the `main` branch is the only supported version.

## Security posture

The proxy is designed to be safe by default:

- **No SSRF surface** — requests are forwarded only to the fixed, configured
  `UPSTREAM_URL`; clients control the JSON-RPC body, never the target.
- **Bounded request bodies** — `MAX_BODY_BYTES` caps reads to prevent memory abuse.
- **Slowloris hardening** — explicit `http.Server` `ReadHeaderTimeout` and related
  timeouts.
- **No secrets in the image or repo** — configuration is via environment variables;
  sensitive values belong in AWS SSM. `.env` is git-ignored (`.env.example` only).
- **Minimal runtime** — distroless, non-root, static binary; no shell or package
  manager in the image.
- **Least-privilege infra** — ALB → task on port 8080 only; tasks in private
  subnets (with NAT) or public-IP-only otherwise; scoped IAM roles.

## Supply chain

- `govulncheck` (Go modules) and `trivy` (container image) run in CI and fail the
  build on HIGH/CRITICAL findings.
- An SBOM (`syft`, SPDX) is produced for every image build.
- IaC is scanned with `trivy config` (HIGH/CRITICAL gate); intentional findings are
  documented inline.
- CI authenticates to AWS via **GitHub OIDC** — no long-lived cloud credentials.

## Handling of RPC data

The proxy is transparent and stateless: request/response bodies are forwarded
without persistence or caching. Logs contain request metadata (id, method, status,
latency), not payloads, by default.
