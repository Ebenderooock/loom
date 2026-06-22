SHELL := /bin/bash
GO    ?= go
PKG   := github.com/ebenderooock/loom
BIN   := loom
DIST  := dist

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -s -w \
	-X $(PKG)/internal/buildinfo.Version=$(VERSION) \
	-X $(PKG)/internal/buildinfo.Commit=$(COMMIT) \
	-X $(PKG)/internal/buildinfo.Date=$(DATE)

# TAGS keeps nosqlite for compatibility with sqlite feature-gating in
# mixed dependency graphs. It is safe to leave enabled by default.
TAGS ?= nosqlite

.PHONY: all build test lint fmt vet tidy run dev clean docker sync-definitions help

all: lint test build ## Lint, test, and build

build: ## Build the loom binary (API only, no embedded UI)
	@mkdir -p $(DIST)
	$(GO) build -trimpath -tags '$(TAGS)' -ldflags '$(LDFLAGS)' -o $(DIST)/$(BIN) ./cmd/loom

build-all: web-build ## Build the loom binary with embedded React UI
	@mkdir -p $(DIST)
	$(GO) build -trimpath -tags 'embed $(TAGS)' -ldflags '$(LDFLAGS)' -o $(DIST)/$(BIN) ./cmd/loom

web-build: ## Build the React frontend into web/dist
	cd web && npm ci --no-audit --no-fund && npm run build

test: ## Run unit tests with race detector and coverage
	$(GO) test -race -count=1 -tags '$(TAGS)' -coverprofile=coverage.out ./...

test-short: ## Run only short tests
	$(GO) test -short -count=1 -tags '$(TAGS)' ./...

lint: ## Run golangci-lint
	@command -v golangci-lint >/dev/null 2>&1 || { \
		echo 'installing golangci-lint…'; \
		$(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@latest; \
	}
	golangci-lint run ./...

fmt: ## Format Go code
	$(GO) fmt ./...

vet: ## Run go vet
	$(GO) vet -tags '$(TAGS)' ./...

tidy: ## go mod tidy
	$(GO) mod tidy

run: build ## Build and run loom
	$(DIST)/$(BIN) serve

dev: ## Run with auto-reload (requires `air`)
	@command -v air >/dev/null 2>&1 || $(GO) install github.com/air-verse/air@latest
	air

clean: ## Remove build artifacts
	rm -rf $(DIST) coverage.out

cross: web-build ## Cross-compile for common platforms (Linux amd64, ARM64, Windows, macOS)
	@mkdir -p $(DIST)
	GOOS=linux   GOARCH=amd64 $(GO) build -trimpath -tags 'embed $(TAGS)' -ldflags '$(LDFLAGS)' -o $(DIST)/$(BIN)-linux-amd64   ./cmd/loom
	GOOS=linux   GOARCH=arm64 $(GO) build -trimpath -tags 'embed $(TAGS)' -ldflags '$(LDFLAGS)' -o $(DIST)/$(BIN)-linux-arm64   ./cmd/loom
	GOOS=linux   GOARCH=arm   GOARM=7 $(GO) build -trimpath -tags 'embed $(TAGS)' -ldflags '$(LDFLAGS)' -o $(DIST)/$(BIN)-linux-armv7 ./cmd/loom
	GOOS=darwin  GOARCH=arm64 $(GO) build -trimpath -tags 'embed $(TAGS)' -ldflags '$(LDFLAGS)' -o $(DIST)/$(BIN)-darwin-arm64  ./cmd/loom
	GOOS=darwin  GOARCH=amd64 $(GO) build -trimpath -tags 'embed $(TAGS)' -ldflags '$(LDFLAGS)' -o $(DIST)/$(BIN)-darwin-amd64  ./cmd/loom
	GOOS=windows GOARCH=amd64 $(GO) build -trimpath -tags 'embed $(TAGS)' -ldflags '$(LDFLAGS)' -o $(DIST)/$(BIN)-windows-amd64.exe ./cmd/loom
	@echo "Built binaries:" && ls -lh $(DIST)/$(BIN)-*

docker: ## Build the local Docker image
	docker build -t loom:dev -f deploy/docker/Dockerfile .

sync-definitions: ## Download Prowlarr v11 Cardigann definitions
	@bash scripts/sync-prowlarr-defs.sh

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'
