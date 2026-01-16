.PHONY: help proto build test clean deps build-all-platforms docs docs-serve docs-pdf docs-all \
       docs-container-build docs-pdf-container docs-pdf-book-container docs-all-container docs-all-container-fast \
       docs-validate docs-validate-build docs-validate-links docs-validate-examples docs-validate-godoc \
       docs-validate-drift docs-validate-sync docs-validate-all \
       release release-snapshot release-dry-run lint sdk-verify \
       e2e-build e2e-test e2e-up e2e-down e2e-logs e2e-clean e2e-full e2e-perf e2e-scenarios \
       e2e-ha e2e-ha-up e2e-ha-down e2e-ha-logs \
       e2e-ipv6 e2e-ipv6-up e2e-ipv6-down e2e-ipv6-logs \
       e2e-ha-ipv6 e2e-ha-ipv6-up e2e-ha-ipv6-down e2e-ha-ipv6-logs \
       e2e-allinone e2e-all-topologies \
       server agent cli exec state monitor policy gitops cluster migrate module registry identity gateway

# Version information
VERSION ?= dev
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -X github.com/shawnbutts/keystone-core/pkg/version.Version=$(VERSION) \
           -X github.com/shawnbutts/keystone-core/pkg/version.GitCommit=$(GIT_COMMIT) \
           -X github.com/shawnbutts/keystone-core/pkg/version.BuildDate=$(BUILD_DATE)

# Platform detection for native builds
NATIVE_OS := $(shell go env GOOS)
NATIVE_ARCH := $(shell go env GOARCH)
NATIVE_BIN_DIR := build/bin/$(NATIVE_OS)/$(NATIVE_ARCH)

# All binaries to build - add new binaries here and they'll automatically be included
# Format: binary-name:cmd-directory
BINARIES := \
	kscore-server:kscore-server \
	kscore-agent:kscore-agent \
	kscore-registry:kscore-registry \
	kscorectl:kscorectl \
	kscore-exec:kscore-exec \
	kscore-state:kscore-state \
	kscore-monitor:kscore-monitor \
	kscore-policy:kscore-policy \
	kscore-gitops:kscore-gitops \
	kscore-cluster:kscore-cluster \
	kscore-migrate:kscore-migrate \
	kscore-module:kscore-module \
	kscore-identity:kscore-identity \
	kscore-telemetry-gateway:kscore-telemetry-gateway

# Extract just the binary names for .PHONY
BINARY_NAMES := $(foreach b,$(BINARIES),$(firstword $(subst :, ,$(b))))

# Cross-platform build configurations
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64

help:
	@echo "Keystone Core Build System"
	@echo ""
	@echo "Build targets (output: build/bin/$(NATIVE_OS)/$(NATIVE_ARCH)/):"
	@echo "  proto              - Generate protobuf code from .proto files"
	@echo "  build              - Build all binaries for current platform ($(NATIVE_OS)/$(NATIVE_ARCH))"
	@echo "  build-all-platforms - Build all binaries for all platforms"
	@echo ""
	@echo "  Server binaries:"
	@echo "    server           - Build kscore-server (control plane)"
	@echo "    agent            - Build kscore-agent (managed node agent)"
	@echo "    registry         - Build kscore-registry (module registry server)"
	@echo "    gateway          - Build kscore-telemetry-gateway (telemetry aggregation)"
	@echo ""
	@echo "  CLI and plugins:"
	@echo "    cli              - Build kscorectl (main CLI dispatcher)"
	@echo "    exec             - Build kscore-exec plugin"
	@echo "    state            - Build kscore-state plugin"
	@echo "    monitor          - Build kscore-monitor TUI"
	@echo "    policy           - Build kscore-policy plugin"
	@echo "    gitops           - Build kscore-gitops plugin"
	@echo "    cluster          - Build kscore-cluster plugin"
	@echo "    migrate          - Build kscore-migrate plugin"
	@echo "    module           - Build kscore-module plugin"
	@echo "    identity         - Build kscore-identity plugin"
	@echo ""
	@echo "  test               - Run tests"
	@echo "  lint               - Run linters"
	@echo "  sdk-verify         - Build SDK examples (Go/Rust/C++)"
	@echo "  clean              - Remove all build artifacts (build/)"
	@echo "  deps               - Install/update dependencies"
	@echo ""
	@echo "Release targets (requires goreleaser):"
	@echo "  release            - Create a release (requires GITHUB_TOKEN)"
	@echo "  release-snapshot   - Create a snapshot release (no publish)"
	@echo "  release-dry-run    - Dry run release (validate config)"
	@echo ""
	@echo "Documentation targets (output: build/docs/ and build/pdfs/):"
	@echo "  docs               - Build Hugo documentation site → build/docs/"
	@echo "  docs-serve         - Build and serve docs locally with live reload"
	@echo "  docs-pdf           - Generate PDF documentation (Playwright + print CSS)"
	@echo "  docs-pdf-simple    - Generate PDFs (fast, minimal formatting)"
	@echo "  docs-pdf-book      - Generate book-quality PDFs (requires Pandoc+LaTeX)"
	@echo "  docs-all           - Build site and generate PDFs"
	@echo "  docs-all-book      - Build site and generate book-quality PDFs"
	@echo ""
	@echo "Containerized documentation (no local deps required):"
	@echo "  docs-container-build     - Build docs container image"
	@echo "  docs-pdf-container       - Generate PDFs using container (Docker/Podman)"
	@echo "  docs-pdf-book-container  - Generate book-quality PDFs using container"
	@echo "  docs-all-container       - Build site and generate all PDFs in container"
	@echo "  docs-all-container-fast  - Same as above but skip Mermaid diagram rendering"
	@echo ""
	@echo "Documentation validation targets:"
	@echo "  docs-validate            - Run all documentation validation checks"
	@echo "  docs-validate-build      - Build the docvalidation tool"
	@echo "  docs-validate-links      - Check internal documentation links"
	@echo "  docs-validate-examples   - Validate code examples in documentation"
	@echo "  docs-validate-godoc      - Check godoc coverage for packages"
	@echo "  docs-validate-drift      - Detect documentation drift from implementation"
	@echo "  docs-validate-sync       - Check documentation sync across files"
	@echo "  docs-validate-all        - Run all validation checks (verbose)"
	@echo ""
	@echo "E2E testing targets (requires Docker/Podman):"
	@echo "  e2e-build          - Build container images for E2E testing"
	@echo "  e2e-test           - Run E2E tests (quick topology tests)"
	@echo "  e2e-full           - Run full E2E test suite (all tests)"
	@echo "  e2e-scenarios      - Run E2E scenario tests"
	@echo "  e2e-perf           - Run E2E performance tests"
	@echo "  e2e-up             - Start E2E test environment"
	@echo "  e2e-down           - Stop E2E test environment"
	@echo "  e2e-logs           - Show logs from E2E containers"
	@echo "  e2e-clean          - Remove E2E containers and images"
	@echo ""
	@echo "HA Cluster E2E testing targets:"
	@echo "  e2e-ha             - Run HA cluster E2E tests (3 servers + 5 agents)"
	@echo "  e2e-ha-up          - Start HA cluster test environment"
	@echo "  e2e-ha-down        - Stop HA cluster test environment"
	@echo "  e2e-ha-logs        - Show logs from HA cluster containers"
	@echo ""
	@echo "IPv6 E2E testing targets:"
	@echo "  e2e-ipv6           - Run IPv6 E2E tests (IPv6-only network)"
	@echo "  e2e-ipv6-up        - Start IPv6 test environment"
	@echo "  e2e-ipv6-down      - Stop IPv6 test environment"
	@echo "  e2e-ipv6-logs      - Show logs from IPv6 containers"
	@echo ""
	@echo "HA Cluster IPv6 E2E testing targets:"
	@echo "  e2e-ha-ipv6        - Run HA cluster tests over IPv6-only network"
	@echo "  e2e-ha-ipv6-up     - Start HA IPv6 test environment"
	@echo "  e2e-ha-ipv6-down   - Stop HA IPv6 test environment"
	@echo "  e2e-ha-ipv6-logs   - Show logs from HA IPv6 containers"
	@echo ""

deps:
	go mod download
	go mod tidy

proto:
	@echo "Generating protobuf code..."
	@mkdir -p pkg/api/v1
	protoc --go_out=pkg/api/v1 --go_opt=paths=source_relative \
	       --go-grpc_out=pkg/api/v1 --go-grpc_opt=paths=source_relative \
	       -I api/proto \
	       api/proto/*.proto
	@echo "Protobuf code generated successfully"

# =============================================================================
# Native Platform Build (current OS/ARCH)
# =============================================================================

# Helper function to build a single binary
# Usage: $(call build-binary,binary-name,cmd-dir)
define build-binary
	@echo "Building $(1)..."
	@mkdir -p $(NATIVE_BIN_DIR)
	go build -ldflags "$(LDFLAGS)" -o $(NATIVE_BIN_DIR)/$(1) ./cmd/$(2)
	@echo "Built: $(NATIVE_BIN_DIR)/$(1)"
endef

# Build all binaries for current platform
build:
	@echo "Building all binaries for $(NATIVE_OS)/$(NATIVE_ARCH)..."
	@mkdir -p $(NATIVE_BIN_DIR)
	@$(foreach b,$(BINARIES), \
		$(eval name := $(word 1,$(subst :, ,$(b)))) \
		$(eval cmd := $(word 2,$(subst :, ,$(b)))) \
		echo "Building $(name)..." && \
		go build -ldflags "$(LDFLAGS)" -o $(NATIVE_BIN_DIR)/$(name) ./cmd/$(cmd) && \
	) true
	@echo "All binaries built: $(NATIVE_BIN_DIR)/"

# Individual binary targets for convenience
server:
	$(call build-binary,kscore-server,kscore-server)

agent:
	$(call build-binary,kscore-agent,kscore-agent)

cli:
	$(call build-binary,kscorectl,kscorectl)

exec:
	$(call build-binary,kscore-exec,kscore-exec)

monitor:
	$(call build-binary,kscore-monitor,kscore-monitor)

state:
	$(call build-binary,kscore-state,kscore-state)

policy:
	$(call build-binary,kscore-policy,kscore-policy)

gitops:
	$(call build-binary,kscore-gitops,kscore-gitops)

cluster:
	$(call build-binary,kscore-cluster,kscore-cluster)

migrate:
	$(call build-binary,kscore-migrate,kscore-migrate)

module:
	$(call build-binary,kscore-module,kscore-module)

registry:
	$(call build-binary,kscore-registry,kscore-registry)

identity:
	$(call build-binary,kscore-identity,kscore-identity)

gateway:
	$(call build-binary,kscore-telemetry-gateway,kscore-telemetry-gateway)

# =============================================================================
# Cross-Platform Builds
# =============================================================================

# Helper function to build all binaries for a specific platform
# Usage: $(call build-platform,os,arch,ext)
define build-platform
	@echo "Building for $(1)/$(2)..."
	@mkdir -p build/bin/$(1)/$(2)
	@$(foreach b,$(BINARIES), \
		$(eval name := $(word 1,$(subst :, ,$(b)))) \
		$(eval cmd := $(word 2,$(subst :, ,$(b)))) \
		GOOS=$(1) GOARCH=$(2) go build -ldflags "$(LDFLAGS)" -o build/bin/$(1)/$(2)/$(name)$(3) ./cmd/$(cmd) && \
	) true
	@echo "$(1)/$(2) builds complete"
endef

build-all-platforms: build-linux build-darwin build-windows
	@echo "All platform builds complete"

build-linux:
	$(call build-platform,linux,amd64,)
	$(call build-platform,linux,arm64,)

build-darwin:
	$(call build-platform,darwin,amd64,)
	$(call build-platform,darwin,arm64,)

build-windows:
	$(call build-platform,windows,amd64,.exe)

# =============================================================================
# Testing
# =============================================================================

test:
	go test -v -race -coverprofile=coverage.out ./...

# =============================================================================
# Cleanup
# =============================================================================

clean:
	rm -rf build/
	rm -rf dist/
	rm -rf data/
	rm -f coverage.out
	# Rust build artifacts
	rm -rf modules/sdk/rust/target/
	rm -rf modules/sdk/rust/examples/*/target/
	# Go SDK test cache (if any)
	rm -rf modules/sdk/go/examples/*/tmp/
	# C++ build artifacts
	rm -rf modules/sdk/cpp/build/
	rm -rf modules/sdk/cpp/examples/*/build/
	# E2E test reports
	rm -rf test/e2e/performance/reports/

install-tools:
	@echo "Installing protoc plugins..."
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	@echo "Tools installed successfully"

# =============================================================================
# Documentation targets
# =============================================================================

docs:
	@echo "Building Hugo documentation site..."
	@cd docs && hugo --quiet
	@echo "Documentation built: build/docs/"

docs-serve:
	@echo "Starting Hugo development server..."
	@echo "Open http://localhost:1313 in your browser"
	@echo "Press Ctrl+C to stop"
	@cd docs && hugo server

docs-pdf:
	@echo "Generating PDF documentation (Paged.js + Playwright)..."
	@if [ ! -d "docs/node_modules" ]; then \
		echo "Installing npm dependencies..."; \
		cd docs && npm install; \
	fi
	@if [ ! -d "$$HOME/.cache/ms-playwright/chromium-"* ] 2>/dev/null; then \
		echo "Installing Playwright browsers..."; \
		cd docs && npm run install-browsers; \
	fi
	@cd docs && npm run generate-pdfs

docs-pdf-simple:
	@echo "Generating PDF documentation (simple mode, no Paged.js)..."
	@if [ ! -d "docs/node_modules" ]; then \
		echo "Installing npm dependencies..."; \
		cd docs && npm install; \
	fi
	@if [ ! -d "$$HOME/.cache/ms-playwright/chromium-"* ] 2>/dev/null; then \
		echo "Installing Playwright browsers..."; \
		cd docs && npm run install-browsers; \
	fi
	@cd docs && node generate-pdfs.js --simple

docs-pdf-book:
	@echo "Generating book-quality PDFs (Pandoc + LaTeX)..."
	@echo "Note: Requires pandoc and LaTeX. See docs/generate-pdfs-book.sh for setup."
	@cd docs && ./generate-pdfs-book.sh

docs-all: docs docs-pdf
	@echo "Documentation site and PDFs complete"

docs-all-book: docs docs-pdf-book
	@echo "Documentation site and book-quality PDFs complete"

# Container-based documentation targets (Docker/Podman)
# Automatically detects docker or podman
CONTAINER_ENGINE := $(shell command -v podman 2>/dev/null || command -v docker 2>/dev/null)
DOCS_IMAGE := kscore-docs

docs-container-build:
	@echo "Building documentation container image..."
	@if [ -z "$(CONTAINER_ENGINE)" ]; then \
		echo "Error: Neither docker nor podman found in PATH"; \
		exit 1; \
	fi
	$(CONTAINER_ENGINE) build -t $(DOCS_IMAGE) docs/
	@echo "Documentation container image built: $(DOCS_IMAGE)"

docs-pdf-container: docs-container-build
	@echo "Generating PDF documentation using container..."
	@mkdir -p build/pdfs
	$(CONTAINER_ENGINE) run --rm \
		-v "$(shell pwd)":/workspace \
		-w /workspace/docs \
		$(DOCS_IMAGE) \
		bash -c "hugo --quiet && npm run generate-pdfs"
	@echo "PDFs generated: build/pdfs/"

docs-pdf-book-container: docs-container-build
	@echo "Generating book-quality PDFs using container..."
	@mkdir -p build/pdfs
	$(CONTAINER_ENGINE) run --rm \
		-v "$(shell pwd)":/workspace \
		-w /workspace/docs \
		$(DOCS_IMAGE) \
		bash -c "hugo --quiet && ./generate-pdfs-book.sh"
	@echo "Book-quality PDFs generated: build/pdfs/"

docs-all-container: docs-container-build
	@echo "Building documentation site and all PDFs using container..."
	@mkdir -p build/pdfs
	$(CONTAINER_ENGINE) run --rm \
		-v "$(shell pwd)":/workspace \
		-w /workspace/docs \
		$(DOCS_IMAGE) \
		bash -c "hugo --quiet && npm run generate-pdfs && ./generate-pdfs-book.sh"
	@echo "Documentation site and PDFs complete"

docs-all-container-fast: docs-container-build
	@echo "Building documentation site and PDFs (skipping Mermaid diagrams)..."
	@mkdir -p build/pdfs
	$(CONTAINER_ENGINE) run --rm \
		-v "$(shell pwd)":/workspace \
		-w /workspace/docs \
		$(DOCS_IMAGE) \
		bash -c "hugo --quiet && npm run generate-pdfs && ./generate-pdfs-book.sh --skip-mermaid"
	@echo "Documentation site and PDFs complete (Mermaid diagrams as code blocks)"

# =============================================================================
# Documentation Validation targets
# =============================================================================

DOCVALIDATION_BIN := scripts/docvalidation/docvalidation

docs-validate-build:
	@echo "Building docvalidation tool..."
	@cd scripts/docvalidation && go build -o docvalidation .
	@echo "docvalidation tool built: $(DOCVALIDATION_BIN)"

docs-validate-links: docs-validate-build
	@echo "Checking internal documentation links..."
	@$(DOCVALIDATION_BIN) links
	@if [ -f scripts/docvalidation/link-check-report.md ]; then \
		echo "Report: scripts/docvalidation/link-check-report.md"; \
	fi

docs-validate-examples: docs-validate-build
	@echo "Validating code examples in documentation..."
	@$(DOCVALIDATION_BIN) examples
	@if [ -f scripts/docvalidation/example-validation-report.md ]; then \
		echo "Report: scripts/docvalidation/example-validation-report.md"; \
	fi

docs-validate-godoc: docs-validate-build
	@echo "Checking godoc coverage for packages..."
	@$(DOCVALIDATION_BIN) godoc
	@if [ -f scripts/docvalidation/godoc-coverage-report.md ]; then \
		echo "Report: scripts/docvalidation/godoc-coverage-report.md"; \
	fi

docs-validate-drift: docs-validate-build
	@echo "Detecting documentation drift from implementation..."
	@$(DOCVALIDATION_BIN) drift
	@if [ -f scripts/docvalidation/drift-report.md ]; then \
		echo "Report: scripts/docvalidation/drift-report.md"; \
	fi

docs-validate-sync: docs-validate-build
	@echo "Checking documentation sync across files..."
	@$(DOCVALIDATION_BIN) sync
	@if [ -f scripts/docvalidation/sync-report.md ]; then \
		echo "Report: scripts/docvalidation/sync-report.md"; \
	fi

docs-validate-all: docs-validate-build
	@echo "Running all documentation validation checks (verbose)..."
	@$(DOCVALIDATION_BIN) all -verbose
	@echo ""
	@echo "Reports generated in scripts/docvalidation/"
	@ls -la scripts/docvalidation/*-report.md 2>/dev/null || echo "No reports generated"

docs-validate: docs-validate-build
	@echo "Running documentation validation..."
	@$(DOCVALIDATION_BIN) -format markdown -output docs-inventory.md
	@$(DOCVALIDATION_BIN) links
	@$(DOCVALIDATION_BIN) examples
	@$(DOCVALIDATION_BIN) godoc
	@$(DOCVALIDATION_BIN) drift
	@$(DOCVALIDATION_BIN) sync
	@echo ""
	@echo "Documentation validation complete."
	@echo "Reports generated:"
	@ls -la scripts/docvalidation/*-report.md 2>/dev/null || echo "  (no reports)"
	@ls -la docs-inventory.md 2>/dev/null || echo "  (no inventory)"

# =============================================================================
# Linting
# =============================================================================

lint:
	@echo "Running linters..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
		exit 1; \
	fi

# =============================================================================
# SDK verification
# =============================================================================

sdk-verify:
	@echo "Verifying SDK examples..."
	@if command -v cargo >/dev/null 2>&1; then \
		if command -v rustup >/dev/null 2>&1 && rustup target list --installed | grep -q wasm32-wasi; then \
			echo "Building Rust SDK example..."; \
			(cd modules/sdk/rust/examples/hello-world && cargo build --target wasm32-wasi --release); \
		else \
			echo "Skipping Rust SDK: install wasm32-wasi target (rustup target add wasm32-wasi)"; \
		fi; \
	else \
		echo "Skipping Rust SDK: cargo not installed"; \
	fi
	@if command -v tinygo >/dev/null 2>&1; then \
		echo "Building Go SDK example (TinyGo)..."; \
		mkdir -p modules/sdk/go/examples/hello-world/build; \
		(cd modules/sdk/go/examples/hello-world && tinygo build -o build/module.wasm -target wasm32-wasi .); \
	else \
		echo "Skipping Go SDK: tinygo not installed"; \
	fi
	@if [ -n "$$WASI_SDK_PATH" ] && [ -d "$$WASI_SDK_PATH" ] && command -v cmake >/dev/null 2>&1; then \
		echo "Building C++ SDK example (WASI SDK)..."; \
		mkdir -p modules/sdk/cpp/examples/hello-world/build; \
		cmake -S modules/sdk/cpp/examples/hello-world -B modules/sdk/cpp/examples/hello-world/build \
			-DCMAKE_TOOLCHAIN_FILE=$$WASI_SDK_PATH/share/cmake/wasi-sdk.cmake; \
		cmake --build modules/sdk/cpp/examples/hello-world/build; \
	else \
		echo "Skipping C++ SDK: set WASI_SDK_PATH and install cmake"; \
	fi

# =============================================================================
# Release targets (requires goreleaser)
# =============================================================================

release:
	@echo "Creating release..."
	@if [ -z "$$GITHUB_TOKEN" ]; then \
		echo "Error: GITHUB_TOKEN environment variable is not set"; \
		exit 1; \
	fi
	goreleaser release --clean

release-snapshot:
	@echo "Creating snapshot release..."
	goreleaser release --snapshot --clean

release-dry-run:
	@echo "Dry run release (validating config)..."
	goreleaser check
	goreleaser release --snapshot --clean --skip=publish

# Install goreleaser
install-goreleaser:
	@echo "Installing goreleaser..."
	go install github.com/goreleaser/goreleaser/v2@latest
	@echo "goreleaser installed successfully"

# =============================================================================
# E2E Testing targets
# =============================================================================

E2E_COMPOSE := docker compose -f test/e2e/containers/docker-compose.yml -p kscore-e2e

e2e-build:
	@echo "Building E2E test container images..."
	$(E2E_COMPOSE) build
	@echo "E2E images built successfully"

e2e-up: e2e-build
	@echo "Starting E2E test environment..."
	$(E2E_COMPOSE) up -d --wait
	@echo "E2E environment is running"
	@echo "Server gRPC: localhost:8080"
	@echo "Server HTTP: localhost:8081"
	@echo "Run 'make e2e-logs' to see container logs"

e2e-down:
	@echo "Stopping E2E test environment..."
	$(E2E_COMPOSE) down -v --remove-orphans
	@echo "E2E environment stopped"

e2e-logs:
	$(E2E_COMPOSE) logs -f

e2e-test: e2e-build
	@echo "Running E2E tests..."
	KSCORE_E2E_TESTS=1 KSCORE_ROOT=$(shell pwd) go test -v -timeout 10m ./test/e2e/topology/...
	@echo "E2E tests complete"
	@echo "Cleaning up..."
	$(E2E_COMPOSE) down -v --remove-orphans

e2e-clean:
	@echo "Cleaning up E2E environment..."
	$(E2E_COMPOSE) down -v --remove-orphans --rmi local
	@echo "E2E cleanup complete"

e2e-allinone: e2e-build
	@echo "Running all-in-one topology E2E tests..."
	KSCORE_E2E_TESTS=1 KSCORE_ROOT=$(shell pwd) go test -v -timeout 20m ./test/e2e/topology/... -run "TestAllInOne"
	KSCORE_E2E_TESTS=1 KSCORE_SKIP_BUILD=1 KSCORE_ROOT=$(shell pwd) go test -v -timeout 20m ./test/e2e/scenarios/...
	@echo "All-in-one E2E tests complete"
	$(E2E_COMPOSE) down -v --remove-orphans

e2e-full: e2e-build
	@echo "=============================================="
	@echo "Running FULL E2E test suite (all topologies)"
	@echo "=============================================="
	@echo ""
	@echo "=== Phase 1: All-in-one topology tests ==="
	KSCORE_E2E_TESTS=1 KSCORE_ROOT=$(shell pwd) go test -v -timeout 20m ./test/e2e/topology/... -run "TestAllInOne"
	KSCORE_E2E_TESTS=1 KSCORE_SKIP_BUILD=1 KSCORE_ROOT=$(shell pwd) go test -v -timeout 20m ./test/e2e/scenarios/...
	$(E2E_COMPOSE) down -v --remove-orphans
	@echo ""
	@echo "=== Phase 2: HA cluster topology tests ==="
	$(HA_COMPOSE) build
	KSCORE_E2E_TESTS=1 KSCORE_TOPOLOGY=ha-cluster KSCORE_SKIP_BUILD=1 KSCORE_ROOT=$(shell pwd) go test -v -timeout 30m ./test/e2e/topology/... -run "HACluster"
	$(HA_COMPOSE) down -v --remove-orphans
	@echo ""
	@echo "=== Phase 3: IPv6 topology tests ==="
	$(IPV6_COMPOSE) build
	KSCORE_E2E_TESTS=1 KSCORE_TOPOLOGY=ipv6 KSCORE_SKIP_BUILD=1 KSCORE_ROOT=$(shell pwd) go test -v -timeout 15m ./test/e2e/topology/... -run "IPv6"
	$(IPV6_COMPOSE) down -v --remove-orphans
	@echo ""
	@echo "=== Phase 4: HA cluster IPv6 topology tests ==="
	$(HA_IPV6_COMPOSE) build
	KSCORE_E2E_TESTS=1 KSCORE_TOPOLOGY=ha-cluster-ipv6 KSCORE_SKIP_BUILD=1 KSCORE_ROOT=$(shell pwd) go test -v -timeout 30m ./test/e2e/topology/... -run "HAClusterIPv6"
	$(HA_IPV6_COMPOSE) down -v --remove-orphans
	@echo ""
	@echo "=== Phase 5: Performance tests ==="
	$(E2E_COMPOSE) up -d --wait
	KSCORE_E2E_TESTS=1 KSCORE_PERF_TESTS=1 KSCORE_SKIP_BUILD=1 KSCORE_ROOT=$(shell pwd) go test -v -timeout 30m ./test/e2e/performance/...
	$(E2E_COMPOSE) down -v --remove-orphans
	@echo ""
	@echo "=============================================="
	@echo "Full E2E test suite COMPLETE"
	@echo "=============================================="

e2e-perf: e2e-build
	@echo "Running E2E performance tests..."
	KSCORE_E2E_TESTS=1 KSCORE_PERF_TESTS=1 KSCORE_ROOT=$(shell pwd) go test -v -timeout 30m ./test/e2e/performance/...
	@echo "Performance tests complete"
	$(E2E_COMPOSE) down -v --remove-orphans

e2e-scenarios: e2e-build
	@echo "Running E2E scenario tests..."
	KSCORE_E2E_TESTS=1 KSCORE_SKIP_BUILD=1 KSCORE_ROOT=$(shell pwd) go test -v -timeout 20m ./test/e2e/scenarios/...
	@echo "Scenario tests complete"
	$(E2E_COMPOSE) down -v --remove-orphans

# =============================================================================
# HA Cluster E2E Testing targets
# =============================================================================

HA_COMPOSE := docker compose -f test/e2e/topologies/ha-cluster/docker-compose.yml -p kscore-e2e-ha

e2e-ha-up:
	@echo "Starting HA cluster E2E test environment..."
	@echo "Building container images..."
	$(HA_COMPOSE) build
	@echo "Starting HA cluster (3 servers + NATS cluster + etcd + PostgreSQL + 5 agents)..."
	$(HA_COMPOSE) up -d --wait
	@echo ""
	@echo "HA Cluster environment is running:"
	@echo "  Server 1 gRPC: localhost:8080   HTTP: localhost:8081"
	@echo "  Server 2 gRPC: localhost:8082   HTTP: localhost:8083"
	@echo "  Server 3 gRPC: localhost:8084   HTTP: localhost:8085"
	@echo ""
	@echo "Run 'make e2e-ha-logs' to see container logs"

e2e-ha-down:
	@echo "Stopping HA cluster E2E test environment..."
	$(HA_COMPOSE) down -v --remove-orphans
	@echo "HA cluster environment stopped"

e2e-ha-logs:
	$(HA_COMPOSE) logs -f

e2e-ha: e2e-build
	@echo "Running HA cluster E2E tests..."
	@echo "Building HA cluster images..."
	$(HA_COMPOSE) build
	KSCORE_E2E_TESTS=1 KSCORE_TOPOLOGY=ha-cluster KSCORE_SKIP_BUILD=1 KSCORE_ROOT=$(shell pwd) go test -v -timeout 30m ./test/e2e/topology/... -run "HACluster"
	@echo "HA cluster E2E tests complete"
	$(HA_COMPOSE) down -v --remove-orphans

# =============================================================================
# IPv6 E2E Testing targets
# =============================================================================

IPV6_COMPOSE := docker compose -f test/e2e/topologies/ipv6/docker-compose.yml -p kscore-e2e-ipv6

e2e-ipv6-up: e2e-build
	@echo "Starting IPv6 E2E test environment..."
	@echo "Building IPv6 container images..."
	$(IPV6_COMPOSE) build
	@echo "Starting IPv6 network environment..."
	$(IPV6_COMPOSE) up -d --wait
	@echo ""
	@echo "IPv6 environment is running:"
	@echo "  Server gRPC: [::1]:8080   HTTP: [::1]:8081"
	@echo "  Network: fd00:1::/64 (IPv6 only)"
	@echo ""
	@echo "Run 'make e2e-ipv6-logs' to see container logs"

e2e-ipv6-down:
	@echo "Stopping IPv6 E2E test environment..."
	$(IPV6_COMPOSE) down -v --remove-orphans
	@echo "IPv6 environment stopped"

e2e-ipv6-logs:
	$(IPV6_COMPOSE) logs -f

e2e-ipv6: e2e-build
	@echo "Running IPv6 E2E tests..."
	@echo "Building IPv6 images..."
	$(IPV6_COMPOSE) build
	KSCORE_E2E_TESTS=1 KSCORE_TOPOLOGY=ipv6 KSCORE_SKIP_BUILD=1 KSCORE_ROOT=$(shell pwd) go test -v -timeout 15m ./test/e2e/topology/... -run "IPv6"
	@echo "IPv6 E2E tests complete"
	$(IPV6_COMPOSE) down -v --remove-orphans

# =============================================================================
# HA Cluster IPv6 E2E Testing targets
# =============================================================================

HA_IPV6_COMPOSE := docker compose -f test/e2e/topologies/ha-cluster-ipv6/docker-compose.yml -p kscore-e2e-ha-ipv6

e2e-ha-ipv6-up:
	@echo "Starting HA cluster IPv6 E2E test environment..."
	@echo "Building container images..."
	$(HA_IPV6_COMPOSE) build
	@echo "Starting HA cluster over IPv6 (3 servers + NATS cluster + etcd + PostgreSQL + 5 agents)..."
	$(HA_IPV6_COMPOSE) up -d --wait
	@echo ""
	@echo "HA Cluster IPv6 environment is running:"
	@echo "  Server 1 gRPC: localhost:8080   HTTP: localhost:8081"
	@echo "  Server 2 gRPC: localhost:8082   HTTP: localhost:8083"
	@echo "  Server 3 gRPC: localhost:8084   HTTP: localhost:8085"
	@echo "  Network: fd00:2::/64 (IPv6 only)"
	@echo ""
	@echo "Run 'make e2e-ha-ipv6-logs' to see container logs"

e2e-ha-ipv6-down:
	@echo "Stopping HA cluster IPv6 E2E test environment..."
	$(HA_IPV6_COMPOSE) down -v --remove-orphans
	@echo "HA cluster IPv6 environment stopped"

e2e-ha-ipv6-logs:
	$(HA_IPV6_COMPOSE) logs -f

e2e-ha-ipv6: e2e-build
	@echo "Running HA cluster IPv6 E2E tests..."
	@echo "Building HA cluster IPv6 images..."
	$(HA_IPV6_COMPOSE) build
	KSCORE_E2E_TESTS=1 KSCORE_TOPOLOGY=ha-cluster-ipv6 KSCORE_SKIP_BUILD=1 KSCORE_ROOT=$(shell pwd) go test -v -timeout 30m ./test/e2e/topology/... -run "HAClusterIPv6"
	@echo "HA cluster IPv6 E2E tests complete"
	$(HA_IPV6_COMPOSE) down -v --remove-orphans
