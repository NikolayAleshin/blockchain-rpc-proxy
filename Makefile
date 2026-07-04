# Polygon JSON-RPC Proxy — golden-path commands (see docs/AI_DEVX.md)
BINARY      := proxy
PKG         := ./...
CMD         := ./cmd/proxy
IMAGE       := polygon-rpc-proxy
TF_DIR      := deploy/terraform/envs/dev

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

## --- Go ---
.PHONY: run
run: ## Run the proxy locally
	go run $(CMD)

.PHONY: build
build: ## Build the binary into ./bin
	go build -o bin/$(BINARY) $(CMD)

.PHONY: test
test: ## Run tests
	go test $(PKG)

.PHONY: test-race
test-race: ## Run tests with the race detector
	go test -race $(PKG)

.PHONY: cover
cover: ## Run tests with coverage report
	go test -race -coverprofile=coverage.out $(PKG)
	go tool cover -func=coverage.out | tail -1

.PHONY: fmt
fmt: ## Format code
	gofmt -w -s .

.PHONY: vet
vet: ## Run go vet
	go vet $(PKG)

.PHONY: lint
lint: ## Run golangci-lint (must be installed)
	golangci-lint run

.PHONY: tidy
tidy: ## Tidy modules
	go mod tidy

.PHONY: vuln
vuln: ## Run govulncheck (must be installed)
	govulncheck $(PKG)

## --- Docker ---
.PHONY: docker-build
docker-build: ## Build the container image
	docker build -t $(IMAGE) .

.PHONY: docker-run
docker-run: ## Run the container image
	docker run --rm -p 8080:8080 --env-file .env $(IMAGE)

## --- Terraform ---
.PHONY: tf-fmt
tf-fmt: ## terraform fmt (recursive)
	terraform -chdir=$(TF_DIR) fmt -recursive

.PHONY: tf-validate
tf-validate: ## terraform validate
	terraform -chdir=$(TF_DIR) init -backend=false && terraform -chdir=$(TF_DIR) validate

.PHONY: tf-plan
tf-plan: ## terraform plan
	terraform -chdir=$(TF_DIR) plan

## --- Observability ---
.PHONY: obs-up
obs-up: ## Start local Grafana/OTel stack
	docker compose -f deploy/observability/docker-compose.yaml up -d

.PHONY: obs-down
obs-down: ## Stop local Grafana/OTel stack
	docker compose -f deploy/observability/docker-compose.yaml down

.PHONY: ci
ci: fmt vet test-race ## Fast local CI (fmt, vet, race tests)
