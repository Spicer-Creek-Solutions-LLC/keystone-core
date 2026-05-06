# Keystone Core — project Makefile.
#
# Targets are grouped by purpose. Run `make` or `make help` to list them.
# Additional targets (proto*, dev*, release*, e2e*) are added by their
# owning tasks/epics — see the comment block at the bottom of this file.

SHELL := /bin/bash
.DEFAULT_GOAL := help

# ---- Module + binaries ----------------------------------------------------

MODULE   := go.keystone-core.io/keystone-core

# Binaries are auto-detected from cmd/. Drop a new dir under cmd/ and it
# joins the build set automatically — no Makefile edit required.
BINARIES := $(notdir $(wildcard cmd/*))

# ---- Build metadata (consumed by pkg/version via -ldflags -X) -------------

VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS    := -X $(MODULE)/pkg/version.Version=$(VERSION) \
              -X $(MODULE)/pkg/version.GitCommit=$(GIT_COMMIT) \
              -X $(MODULE)/pkg/version.BuildDate=$(BUILD_DATE)

# ---- Cross-compile matrix --------------------------------------------------

PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64

# ---- Project-wide flags ----------------------------------------------------

# Production builds are pure-Go (PROJECT-DETAILS §3.1). Tests override this
# to CGO_ENABLED=1 so the race detector works.
export CGO_ENABLED := 0

# ---- Phony declarations ---------------------------------------------------

.PHONY: help \
        build build-all-platforms clean deps install-tools \
        test test-verbose test-coverage test-integration check \
        fmt lint lint-fix smoke \
        proto proto-lint proto-breaking \
        dev dev-server dev-agent \
        release-snapshot release-dry-run \
        security-secrets security-vulns security-sast

# ---- Help (default) -------------------------------------------------------

help: ## Show this help
	@awk 'BEGIN {FS = ":.*##"; printf "Usage: make <target>\n\nTargets:\n"} /^[a-zA-Z_-]+:.*?##/ {printf "  %-22s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# ---- Build ----------------------------------------------------------------

build: ## Build all binaries for the host platform
	@if [ -z "$(BINARIES)" ]; then \
		echo "no binaries in cmd/ yet (added by task 13)"; \
		exit 0; \
	fi
	@for bin in $(BINARIES); do \
		os=$$(go env GOOS); arch=$$(go env GOARCH); \
		echo ">>> build $$os/$$arch $$bin"; \
		go build -ldflags="$(LDFLAGS)" \
			-o build/bin/$$os/$$arch/$$bin ./cmd/$$bin || exit $$?; \
	done

build-all-platforms: ## Cross-compile all binaries for the v1.0 platform matrix
	@if [ -z "$(BINARIES)" ]; then \
		echo "no binaries in cmd/ yet (added by task 13)"; \
		exit 0; \
	fi
	@for plat in $(PLATFORMS); do \
		os=$${plat%/*}; arch=$${plat#*/}; \
		ext=""; if [ "$$os" = "windows" ]; then ext=".exe"; fi; \
		for bin in $(BINARIES); do \
			echo ">>> build $$os/$$arch $$bin$$ext"; \
			GOOS=$$os GOARCH=$$arch go build -ldflags="$(LDFLAGS)" \
				-o build/bin/$$os/$$arch/$$bin$$ext ./cmd/$$bin || exit $$?; \
		done; \
	done

clean: ## Remove build artifacts
	rm -rf build/ dist/
	rm -f coverage.out

deps: ## Download and verify Go module dependencies
	go mod download
	go mod verify

install-tools: ## Install Go-installable dev tools (golangci-lint, gosec, govulncheck, buf, protoc-gen-go*, goreleaser)
	@command -v golangci-lint >/dev/null || go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
	@command -v gosec >/dev/null || go install github.com/securego/gosec/v2/cmd/gosec@latest
	@command -v govulncheck >/dev/null || go install golang.org/x/vuln/cmd/govulncheck@latest
	@command -v buf >/dev/null || go install github.com/bufbuild/buf/cmd/buf@latest
	@command -v protoc-gen-go >/dev/null || go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	@command -v protoc-gen-go-grpc >/dev/null || go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	@command -v goreleaser >/dev/null || go install github.com/goreleaser/goreleaser/v2@latest
	@command -v gitleaks >/dev/null || { \
		echo "WARN: gitleaks not installed (not go-installable)"; \
		echo "  macOS:   brew install gitleaks"; \
		echo "  Linux:   github.com/zricethezav/gitleaks/releases"; \
	}

# ---- Test -----------------------------------------------------------------

test: ## Run unit tests with -race
	CGO_ENABLED=1 go test -race ./...

test-verbose: ## Run unit tests with -race -v
	CGO_ENABLED=1 go test -race -v ./...

test-coverage: ## Run tests with coverage profile and per-function output
	CGO_ENABLED=1 go test -race -coverprofile=coverage.out ./...
	@go tool cover -func=coverage.out

test-integration: ## Run integration tests (-tags=integration)
	CGO_ENABLED=1 go test -race -tags=integration ./...

check: lint test ## Run lint + tests

smoke: ## Run quick smoke checks (compile + SQLite pragmas)
	scripts/smoke-test.sh quick

# ---- Lint / Format --------------------------------------------------------

fmt: ## Format Go code and tidy go.mod
	@dirs="$(wildcard pkg internal cmd api)"; \
	if [ -n "$$dirs" ]; then \
		find $$dirs -name '*.go' -print0 | xargs -0 -r gofmt -w; \
	fi
	go mod tidy

lint: ## Run golangci-lint
	golangci-lint run ./...

lint-fix: ## Auto-fix lint issues where possible
	golangci-lint run --fix ./...

# ---- Proto ----------------------------------------------------------------

proto: ## Generate Go + gRPC stubs from proto files
	buf generate

proto-lint: ## Lint proto files (buf STANDARD)
	buf lint

proto-breaking: ## Check protos for breaking changes vs. main
	buf breaking --against '.git#branch=main'

# ---- Dev run --------------------------------------------------------------

# `make dev` runs the binary named by DEV_BIN (default: kscore-server) against
# testdata/dev.yaml. Override with: make dev DEV_BIN=kscorectl
DEV_BIN ?= kscore-server

dev: ## Run a binary in dev mode (DEV_BIN=kscore-server by default)
	go run ./cmd/$(DEV_BIN) --config testdata/dev.yaml

dev-server: ## Run kscore-server against testdata/dev.yaml
	go run ./cmd/kscore-server --config testdata/dev.yaml

dev-agent: ## Run kscore-agent against testdata/dev.yaml
	go run ./cmd/kscore-agent --config testdata/dev.yaml

# ---- Release --------------------------------------------------------------

release-snapshot: ## Build multi-arch snapshot tarballs to dist/
	goreleaser release --snapshot --clean

release-dry-run: ## Validate the goreleaser config without building
	goreleaser check

# ---- Security -------------------------------------------------------------

security-secrets: ## Scan for committed secrets (gitleaks)
	gitleaks detect --source . -v

security-vulns: ## Scan deps for known CVEs (govulncheck)
	govulncheck ./...

security-sast: ## Static analysis (gosec)
	gosec -exclude-dir=.cache ./...

# ---------------------------------------------------------------------------
# Targets added by later tasks/epics — intentionally NOT stubbed here so
# `make help` reflects only what currently works.
#
#   e2e-build, e2e-test, e2e-up, e2e-down,
#     e2e-logs                                 -> Epic 19 (release / E2E hardening)
# ---------------------------------------------------------------------------
