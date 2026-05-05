SHELL := /bin/bash
GO    ?= go
PKG   := github.com/loomctl/loom
BIN   := loom
DIST  := dist

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -s -w \
	-X $(PKG)/internal/buildinfo.Version=$(VERSION) \
	-X $(PKG)/internal/buildinfo.Commit=$(COMMIT) \
	-X $(PKG)/internal/buildinfo.Date=$(DATE)

.PHONY: all build test lint fmt vet tidy run dev clean docker sync-definitions help

all: lint test build ## Lint, test, and build

build: ## Build the loom binary
	@mkdir -p $(DIST)
	$(GO) build -trimpath -ldflags '$(LDFLAGS)' -o $(DIST)/$(BIN) ./cmd/loom

test: ## Run unit tests with race detector and coverage
	$(GO) test -race -count=1 -coverprofile=coverage.out ./...

test-short: ## Run only short tests
	$(GO) test -short -count=1 ./...

lint: ## Run golangci-lint
	@command -v golangci-lint >/dev/null 2>&1 || { \
		echo 'installing golangci-lint…'; \
		$(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@latest; \
	}
	golangci-lint run ./...

fmt: ## Format Go code
	$(GO) fmt ./...

vet: ## Run go vet
	$(GO) vet ./...

tidy: ## go mod tidy
	$(GO) mod tidy

run: build ## Build and run loom
	$(DIST)/$(BIN) serve

dev: ## Run with auto-reload (requires `air`)
	@command -v air >/dev/null 2>&1 || $(GO) install github.com/air-verse/air@latest
	air

clean: ## Remove build artifacts
	rm -rf $(DIST) coverage.out

docker: ## Build the local Docker image
	docker build -t loom:dev -f deploy/docker/Dockerfile .

sync-definitions: ## Download Prowlarr v11 Cardigann definitions
	@bash scripts/sync-prowlarr-defs.sh

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'
