.PHONY: help proto build test test-verbose test-coverage test-integration benchmark \
       fmt lint-fix check \
       clean deps build-all-platforms docs docs-serve docs-pdf docs-all \
       docs-container-build docs-pdf-container docs-pdf-book-container docs-all-container docs-all-container-fast \
       docs-validate docs-validate-build docs-validate-links docs-validate-examples docs-validate-godoc \
       docs-validate-drift docs-validate-sync docs-validate-all \
       docs-lint-container docs-links-container docs-check-container \
       release release-snapshot release-dry-run lint sdk-verify \
       security security-secrets security-vulns security-sast security-licenses security-sbom security-fuzz \
       security-report security-install-tools \
       e2e-build e2e-test e2e-up e2e-down e2e-logs e2e-clean e2e-full e2e-perf e2e-scenarios \
       e2e-ha e2e-ha-up e2e-ha-down e2e-ha-logs \
       e2e-ipv6 e2e-ipv6-up e2e-ipv6-down e2e-ipv6-logs \
       e2e-ha-ipv6 e2e-ha-ipv6-up e2e-ha-ipv6-down e2e-ha-ipv6-logs \
       e2e-allinone e2e-all-topologies \
       test-vm test-vm-demo test-vm-smoke repo-server \
       repos repo-gen repos-dnf repos-apt repos-windows repos-blueprints repos-modules \
       server agent cli exec state monitor policy gitops cluster migrate module registry identity gateway schedule loadtest

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
	kscore-telemetry-gateway:kscore-telemetry-gateway \
	kscore-blueprint-publish:kscore-blueprint-publish \
	kscore-blueprint-state:kscore-blueprint-state \
	kscore-federation:kscore-federation \
	kscore-cluster-backup:kscore-cluster-backup \
	kscore-files-storage:kscore-files-storage \
	kscore-audit:kscore-audit \
	kscore-webhook:kscore-webhook \
	kscore-schedule:kscore-schedule \
	kscore-loadtest:kscore-loadtest \
	kscore-repo-gen:kscore-repo-gen

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
	@echo "    schedule         - Build kscore-schedule plugin"
	@echo "    loadtest         - Build kscore-loadtest utility"
	@echo ""
	@echo "  test               - Run tests"
	@echo "  lint               - Run linters"
	@echo "  sdk-verify         - Build SDK examples (Go/Rust/C++)"
	@echo "  clean              - Remove all build artifacts (build/)"
	@echo "  deps               - Install/update dependencies"
	@echo ""
	@echo "Security scanning targets:"
	@echo "  security           - Run all security checks"
	@echo "  security-secrets   - Detect secrets with gitleaks"
	@echo "  security-vulns     - Check vulnerabilities (govulncheck + nancy)"
	@echo "  security-sast      - Static analysis (gosec + semgrep)"
	@echo "  security-licenses  - Check dependency licenses"
	@echo "  security-sbom      - Generate SBOM (CycloneDX + SPDX)"
	@echo "  security-fuzz      - Run fuzz tests"
	@echo "  security-report    - Run all scans and generate markdown report"
	@echo "  security-install-tools - Install security scanning tools"
	@echo ""
	@echo "Release targets (requires goreleaser):"
	@echo "  release            - Create a release (requires GITHUB_TOKEN)"
	@echo "  release-snapshot   - Create a snapshot release (no publish)"
	@echo "  release-dry-run    - Dry run release (validate config)"
	@echo ""
	@echo "Repository generation targets (output: build/repos/):"
	@echo "  repos              - Generate all distribution repositories"
	@echo "  repos-dnf          - Generate DNF/YUM repository (RPM packages)"
	@echo "  repos-apt          - Generate APT repository (DEB packages)"
	@echo "  repos-windows      - Generate Windows repository (MSI/ZIP)"
	@echo "  repos-blueprints   - Generate blueprint registry"
	@echo "  repos-modules      - Generate module registry"
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
	@echo "  docs-lint-container      - Run markdown linting in a container"
	@echo "  docs-links-container     - Run link checking in a container"
	@echo "  docs-check-container     - Run lint + link checks in containers"
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
	@echo "VM Bootstrap testing targets (requires SSH-accessible VM):"
	@echo "  test-vm            - Run all VM bootstrap tests"
	@echo "  test-vm-demo       - Run single-node demo test (edit single-node-demo.yaml first)"
	@echo "  test-vm-smoke      - Verify VM config without running tests"
	@echo "  repo-server        - Start local HTTP server for package repos"
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

schedule:
	$(call build-binary,kscore-schedule,kscore-schedule)

loadtest:
	$(call build-binary,kscore-loadtest,kscore-loadtest)

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

test-verbose: test

test-coverage: test
	@go tool cover -func=coverage.out
	@echo ""
	@echo "HTML coverage report: go tool cover -html=coverage.out"

test-integration:
	go test -v -race -tags integration ./...

benchmark:
	go test -bench=. -benchmem -run=^$$ ./...

fmt:
	gofmt -w -s cmd/ internal/ pkg/ test/

lint-fix:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run --fix ./...; \
	else \
		echo "golangci-lint not installed. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
		exit 1; \
	fi

check: fmt lint test
	@echo "All checks passed."

# =============================================================================
# Cleanup
# =============================================================================

clean:
	rm -rf build/
	rm -rf dist/
	rm -rf data/
	rm -rf reports/
	rm -rf internal/loadtest/reports/
	rm -f coverage.out
	rm -f docvalidation scripts/docvalidation/docvalidation
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

# =============================================================================
# Repository Generation
# =============================================================================

# DIST_DIR is the goreleaser output directory containing packages
DIST_DIR ?= dist

repos: repo-gen
	@echo "Generating distribution repositories from $(DIST_DIR)..."
	@if [ ! -d "$(DIST_DIR)" ]; then \
		echo "Error: dist directory not found. Run 'make release-snapshot' first."; \
		exit 1; \
	fi
	$(NATIVE_BIN_DIR)/kscore-repo-gen all --version $(VERSION) --dist $(DIST_DIR) --output build/repos

repo-gen:
	@echo "Building kscore-repo-gen..."
	@mkdir -p $(NATIVE_BIN_DIR)
	go build -ldflags "$(LDFLAGS)" -o $(NATIVE_BIN_DIR)/kscore-repo-gen ./cmd/kscore-repo-gen

repos-dnf: repo-gen
	$(NATIVE_BIN_DIR)/kscore-repo-gen dnf --version $(VERSION) --dist $(DIST_DIR) --output build/repos/dnf

repos-apt: repo-gen
	$(NATIVE_BIN_DIR)/kscore-repo-gen apt --version $(VERSION) --dist $(DIST_DIR) --output build/repos/apt

repos-windows: repo-gen
	$(NATIVE_BIN_DIR)/kscore-repo-gen windows --version $(VERSION) --dist $(DIST_DIR) --output build/repos/windows

repos-blueprints: repo-gen
	$(NATIVE_BIN_DIR)/kscore-repo-gen blueprints --output build/repos/blueprints

repos-modules: repo-gen
	$(NATIVE_BIN_DIR)/kscore-repo-gen modules --output build/repos/modules

# Goreleaser binary (can be overridden: make GORELEASER=/path/to/goreleaser release-packages)
GORELEASER ?= $(shell which goreleaser 2>/dev/null || echo "$(HOME)/go/bin/goreleaser")

# Build packages with goreleaser (snapshot for testing, no publish)
release-packages:
	@echo "Building packages with goreleaser..."
	$(GORELEASER) release --snapshot --clean

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
DOCS_NODE_IMAGE := docker.io/node:20-bookworm
DOCS_LYCHEE_IMAGE := lycheeverse/lychee:latest

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

docs-lint-container:
	@echo "Running markdown lint (containerized)..."
	@if [ -z "$(CONTAINER_ENGINE)" ]; then \
		echo "Error: docker or podman is required for docs-lint-container"; \
		exit 1; \
	fi
	@$(CONTAINER_ENGINE) run --rm \
		-v "$(PWD)":/workspace \
		-w /workspace \
		$(DOCS_NODE_IMAGE) \
		bash -c "npx -y markdownlint-cli2 \"docs/**/*.md\""

docs-links-container: docs-container-build
	@echo "Running link check (containerized)..."
	@if [ -z "$(CONTAINER_ENGINE)" ]; then \
		echo "Error: docker or podman is required for docs-links-container"; \
		exit 1; \
	fi
	@echo "Building Hugo site for link checking..."
	@mkdir -p build/docs
	@$(CONTAINER_ENGINE) run --rm \
		-v "$(PWD)":/workspace \
		-w /workspace/docs \
		$(DOCS_IMAGE) \
		bash -c "hugo --quiet"
	@echo "Checking links in rendered site..."
	@$(CONTAINER_ENGINE) run --rm \
		-v "$(PWD)":/workspace \
		-w /workspace \
		$(DOCS_LYCHEE_IMAGE) \
		--config .lychee.toml \
		--no-progress \
		--root-dir /workspace/build/docs \
		--scheme https \
		--scheme http \
		--scheme mailto \
		--scheme file \
		build/docs

docs-check-container: docs-lint-container docs-links-container
	@echo "Containerized doc checks complete."

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
# Security Scanning
# =============================================================================

SECURITY_CONTAINER_ENGINE := $(CONTAINER_ENGINE)
SECURITY_WORKDIR := /workspace
SECURITY_CACHE_DIR ?= $(PWD)/.cache/security
SECURITY_TIMEOUT ?= 10m
SECURITY_GO_IMAGE := golang:1.25
SECURITY_GITLEAKS_IMAGE := zricethezav/gitleaks:latest
SECURITY_TRIVY_IMAGE := aquasec/trivy:latest
SECURITY_GOSEC_IMAGE := securego/gosec:latest
SECURITY_SEMGREP_IMAGE := semgrep/semgrep:latest
SECURITY_SYFT_IMAGE := anchore/syft:latest
SECURITY_GRYPE_IMAGE := anchore/grype:latest
SECURITY_KICS_IMAGE := checkmarx/kics:latest
SECURITY_HADOLINT_IMAGE := hadolint/hadolint:latest
SECURITY_SCORECARD_IMAGE := gcr.io/openssf/scorecard:stable

SECURITY_CONTAINER_RUN := $(SECURITY_CONTAINER_ENGINE) run --rm \
	-v $(PWD):$(SECURITY_WORKDIR) \
	-v $(SECURITY_CACHE_DIR)/go:/tmp/go \
	-v $(SECURITY_CACHE_DIR)/gomod:/tmp/gomod \
	-v $(SECURITY_CACHE_DIR)/gocache:/tmp/gocache \
	-v $(SECURITY_CACHE_DIR)/trivy:/tmp/trivy \
	-v $(SECURITY_CACHE_DIR)/semgrep:/tmp/semgrep \
	-w $(SECURITY_WORKDIR)
SECURITY_GO_ENV := -e GOPATH=/tmp/go -e GOMODCACHE=/tmp/gomod -e GOCACHE=/tmp/gocache \
	-e PATH=/usr/local/go/bin:/tmp/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
SECURITY_GO_RUN := $(SECURITY_CONTAINER_RUN) $(SECURITY_GO_ENV) $(SECURITY_GO_IMAGE) sh -c

security-container-check:
	@if [ -z "$(SECURITY_CONTAINER_ENGINE)" ]; then \
		echo "Error: Neither docker nor podman found in PATH"; \
		exit 1; \
	fi

security-install-tools:
	@echo "Installing security scanning tools..."
	go install golang.org/x/vuln/cmd/govulncheck@latest
	go install github.com/securego/gosec/v2/cmd/gosec@latest
	go install github.com/sonatype-nexus-community/nancy@latest
	go install github.com/google/go-licenses@latest
	@echo ""
	@echo "Tools that require manual installation:"
	@echo "  gitleaks: https://github.com/gitleaks/gitleaks#installing"
	@echo "  trivy: https://aquasecurity.github.io/trivy/latest/getting-started/installation/"
	@echo "  syft: https://github.com/anchore/syft#installation"
	@echo "  semgrep: pip install semgrep (or see https://semgrep.dev/docs/getting-started/)"
	@echo ""
	@echo "Go-based tools installed successfully"

security: security-container-check
	@echo "=== Running security checks (timeout $(SECURITY_TIMEOUT) per target, if timeout is available) ==="
	@$(call RUN_WITH_TIMEOUT,security-secrets)
	@$(call RUN_WITH_TIMEOUT,security-vulns)
	@$(call RUN_WITH_TIMEOUT,security-sast)
	@$(call RUN_WITH_TIMEOUT,security-licenses)
	@echo ""
	@echo "=== All security checks complete ==="

define RUN_WITH_TIMEOUT
	@if command -v timeout >/dev/null; then \
		timeout $(SECURITY_TIMEOUT) $(MAKE) --no-print-directory $(1); \
	else \
		$(MAKE) --no-print-directory $(1); \
	fi
endef

security-secrets: security-container-check
	@echo "=== Running secret detection (gitleaks) ==="
	@$(SECURITY_CONTAINER_RUN) $(SECURITY_GITLEAKS_IMAGE) \
		detect --source $(SECURITY_WORKDIR) --config $(SECURITY_WORKDIR)/.gitleaks.toml --verbose

security-vulns: security-container-check
	@echo "=== Running vulnerability checks ==="
	@echo ""
	@echo "--- govulncheck ---"
	@# Known unfixable k8s.io/kubernetes vulnerabilities (documented in SECURITY.md):
	@# - GO-2025-3547: Race condition in kube-apiserver
	@# - GO-2025-3521: GitRepo Volume local repository access
	@$(SECURITY_GO_RUN) 'set -e; \
		go install golang.org/x/vuln/cmd/govulncheck@latest; \
		govulncheck ./... 2>&1 | tee /tmp/govulncheck.out; \
		if grep -q "No vulnerabilities found" /tmp/govulncheck.out; then \
			echo "No vulnerabilities found"; \
		else \
			NEW_VULNS=$$(grep -oE "GO-[0-9]{4}-[0-9]+" /tmp/govulncheck.out | sort -u | grep -v -E "(GO-2025-3547|GO-2025-3521)" | wc -l); \
			if [ "$$NEW_VULNS" -eq 0 ]; then \
				echo ""; \
				echo "WARNING: Only known unfixable k8s.io/kubernetes vulnerabilities found - documented in SECURITY.md"; \
			else \
				echo ""; \
				echo "ERROR: New vulnerabilities found that need attention"; \
				exit 1; \
			fi; \
		fi'
	@echo ""
	@echo "--- nancy (Sonatype OSS Index) ---"
	@$(SECURITY_GO_RUN) 'set -e; \
		go install github.com/sonatype-nexus-community/nancy@latest; \
		go list -json -deps ./... | nancy sleuth 2>&1 || \
		echo "nancy check skipped (requires OSS Index API token for full results)"'
	@echo ""
	@echo "--- trivy filesystem scan ---"
	@$(SECURITY_CONTAINER_RUN) -e TRIVY_CACHE_DIR=/tmp/trivy $(SECURITY_TRIVY_IMAGE) \
		fs --severity HIGH,CRITICAL $(SECURITY_WORKDIR)

security-sast: security-container-check
	@echo "=== Running static analysis ==="
	@echo ""
	@echo "--- gosec ---"
	@# Only fail on HIGH severity issues; MEDIUM/LOW reported but non-blocking
	@# Exclusions:
	@#   G115: Integer overflow false positives (bounded gRPC int32 conversions)
	@#   G404: Weak random is acceptable for non-crypto uses (jitter)
	@#   G101: False positives on variable names containing "key", "token", etc.
	@$(SECURITY_CONTAINER_RUN) --entrypoint /bin/gosec $(SECURITY_GOSEC_IMAGE) \
		-severity high -exclude=G115,G404,G101 -exclude-dir=test -exclude-dir=modules -exclude-dir=.cache ./...
	@echo ""
	@echo "--- semgrep ---"
	@$(SECURITY_CONTAINER_ENGINE) run --rm -v $(PWD):/src -w /src -e SEMGREP_CACHE_DIR=/tmp/semgrep --entrypoint semgrep $(SECURITY_SEMGREP_IMAGE) \
		scan --config auto --config p/golang --exclude .cache --error

security-licenses: security-container-check
	@echo "=== Checking dependency licenses ==="
	@# NOTE: modernc.org/mathutil is BSD-3-Clause but the module lacks a detectable LICENSE file; ignore to avoid false "unknown license".
	@$(SECURITY_GO_RUN) 'set -e; \
		go install github.com/google/go-licenses@latest; \
		go-licenses check --ignore modernc.org/mathutil ./... 2>&1 || true; \
		echo ""; \
		echo "License report:"; \
		go-licenses report --ignore modernc.org/mathutil ./... --template="{{range .}}{{.Name}}: {{.LicenseName}}{{\"\n\"}}{{end}}" 2>/dev/null | head -50'

security-sbom: security-container-check
	@echo "=== Generating SBOM ==="
	@mkdir -p build
	@echo "Generating CycloneDX SBOM..."
	@$(SECURITY_CONTAINER_RUN) $(SECURITY_SYFT_IMAGE) \
		. -o cyclonedx-json=build/sbom-cyclonedx.json
	@echo "Generating SPDX SBOM..."
	@$(SECURITY_CONTAINER_RUN) $(SECURITY_SYFT_IMAGE) \
		. -o spdx-json=build/sbom-spdx.json
	@echo ""
	@echo "SBOMs generated:"
	@ls -la build/sbom-*.json

security-fuzz: security-container-check
	@echo "=== Running fuzz tests ==="
	@echo "Note: Running for 30 seconds per test. For thorough testing, run longer locally."
	@$(SECURITY_GO_RUN) 'set -e; \
		for fuzztest in $$(go test -list "Fuzz.*" ./pkg/security/... 2>/dev/null | grep "^Fuzz"); do \
			echo "Running $$fuzztest for 30 seconds..."; \
			go test -fuzz="^$${fuzztest}$$" -fuzztime=30s ./pkg/security/... || true; \
		done'
	@echo "Fuzz testing complete"

security-report: security-container-check
	@echo "=== Generating security report ==="
	@CONTAINER_ENGINE="$(SECURITY_CONTAINER_ENGINE)" \
		SECURITY_CACHE_DIR="$(SECURITY_CACHE_DIR)" \
		SECURITY_GO_IMAGE="$(SECURITY_GO_IMAGE)" \
		SECURITY_GITLEAKS_IMAGE="$(SECURITY_GITLEAKS_IMAGE)" \
		SECURITY_TRIVY_IMAGE="$(SECURITY_TRIVY_IMAGE)" \
		SECURITY_GOSEC_IMAGE="$(SECURITY_GOSEC_IMAGE)" \
		SECURITY_SEMGREP_IMAGE="$(SECURITY_SEMGREP_IMAGE)" \
		SECURITY_GRYPE_IMAGE="$(SECURITY_GRYPE_IMAGE)" \
		SECURITY_KICS_IMAGE="$(SECURITY_KICS_IMAGE)" \
		SECURITY_HADOLINT_IMAGE="$(SECURITY_HADOLINT_IMAGE)" \
		SECURITY_SCORECARD_IMAGE="$(SECURITY_SCORECARD_IMAGE)" \
		./scripts/security-report.sh
	@echo ""
	@echo "Report generated: build/security/security-report.md"
	@echo "Raw scan outputs: build/security/"

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

# =============================================================================
# VM Bootstrap Testing targets
# =============================================================================

# VM testing requires:
#   1. A VM accessible via SSH
#   2. KSCORE_VM_TESTS=1 environment variable
#   3. KSCORE_VM_CONFIG pointing to a config file (optional, defaults to test/bootstrap/vm/config.yaml)
#
# For single-node demo testing:
#   1. Export KSCORE_VM_DEMO_HOST and edit test/bootstrap/vm/single-node-demo.yaml credentials
#   2. Run: make test-vm-demo

.PHONY: test-vm test-vm-demo test-vm-smoke repo-server

test-vm:
	@echo "Running VM bootstrap tests..."
	@echo "Note: Requires KSCORE_VM_TESTS=1 and configured VM(s)"
	KSCORE_VM_TESTS=1 go test -v -timeout 2h ./test/bootstrap/vm/scenarios/...

test-vm-demo:
	@echo "Running single-node demo VM bootstrap test..."
	@echo ""
	@echo "Prerequisites:"
	@echo "  1. Edit test/bootstrap/vm/single-node-demo.yaml with your VM credentials"
	@echo "  1a. Export KSCORE_VM_DEMO_HOST to your VM hostname/IP"
	@echo "  2. Ensure your VM is accessible via SSH"
	@echo "  3. Start a local repo server: make repo-server (in another terminal)"
	@echo "  4. Set KSCORE_REPO_URL to your repo host (e.g., http://repo-host.example.internal:8080/repos)"
	@echo ""
	@if [ -z "$$KSCORE_REPO_URL" ]; then \
		echo "WARNING: KSCORE_REPO_URL not set. The VM must be able to reach your repo server."; \
		echo "Example: KSCORE_REPO_URL=http://repo-host.example.internal:8080/repos make test-vm-demo"; \
		echo ""; \
	fi
	KSCORE_VM_TESTS=1 KSCORE_VM_CONFIG=test/bootstrap/vm/single-node-demo.yaml \
		go test -v -timeout 30m ./test/bootstrap/vm/scenarios/... -run TestDemoSingleNodeBootstrap

test-vm-smoke:
	@echo "Running VM smoke test (verifies config only)..."
	KSCORE_VM_TESTS=1 go test -v -timeout 5m ./test/bootstrap/vm/scenarios/... -run TestVMSmokeConfig

# Serve the generated repositories locally for VM testing
repo-server:
	@echo "Starting local repository server..."
	@echo "Serving build/repos/ at http://localhost:8080/repos/"
	@echo "Press Ctrl+C to stop"
	@cd build && python3 -m http.server 8080
