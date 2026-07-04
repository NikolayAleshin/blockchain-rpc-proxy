# syntax=docker/dockerfile:1

# --- build stage ---
FROM golang:1.26-bookworm AS build
WORKDIR /src

# Cache dependencies first.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG VERSION=dev
# Static, stripped, reproducible binary (no CGO) so it runs on scratch/distroless.
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/proxy ./cmd/proxy

# --- runtime stage ---
# distroless static: no shell, no package manager; :nonroot runs as uid 65532.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/proxy /proxy

EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/proxy"]

# Shell-free healthcheck via the binary's own subcommand.
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD ["/proxy", "healthcheck"]
