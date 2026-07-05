# Local observability stack (Grafana + Tempo + Prometheus + OpenTelemetry)

A one-command demo of the proxy's **opt-in** observability: OTLP traces →
OpenTelemetry Collector → Tempo, Prometheus scraping `/metrics`, and Grafana with
**provisioned** datasources + a RED dashboard (zero clicks).

## Run

```bash
docker compose -f deploy/observability/docker-compose.yaml up --build
```

Generate some traffic:

```bash
for i in $(seq 1 20); do
  curl -s -XPOST localhost:8080 -H 'content-type: application/json' \
    -d '{"jsonrpc":"2.0","id":1,"method":"eth_blockNumber"}' >/dev/null
done
```

Then open:

| URL | What |
|-----|------|
| http://localhost:3000 | **Grafana** — dashboard *“RPC Proxy — RED overview”* (anonymous admin) |
| http://localhost:3000/explore | Explore **Tempo** traces (inbound + upstream spans) |
| http://localhost:9090 | Prometheus (targets, raw metrics) |
| http://localhost:8080/metrics | Raw RED metrics from the proxy |

Tear down:

```bash
docker compose -f deploy/observability/docker-compose.yaml down -v
```

## How it fits together

```text
proxy ──OTLP:4317──▶ otel-collector ──OTLP──▶ Tempo ─┐
  │                                                   ├──▶ Grafana (Tempo + Prometheus datasources)
  └──/metrics◀── Prometheus (scrape :8080, :8888) ────┘
```

- The proxy is started with `OTEL_EXPORTER_OTLP_ENDPOINT=otel-collector:4317` and
  `METRICS_ENABLED=true`. Remove those and it runs with **no telemetry** — the whole
  stack is opt-in.
- Logs are structured JSON on stdout with `trace_id`/`span_id`; ship them to Loki
  (via a log agent) to complete log↔trace correlation — left as a documented add-on.
