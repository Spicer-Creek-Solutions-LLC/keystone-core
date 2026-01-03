.PHONY: help proto build test clean deps build-all-platforms docs docs-serve docs-pdf docs-all \
       docs-container-build docs-pdf-container docs-pdf-book-container docs-all-container \
       release release-snapshot release-dry-run lint \
       e2e-build e2e-test e2e-up e2e-down e2e-logs e2e-clean e2e-full e2e-perf e2e-scenarios \
       e2e-ha e2e-ha-up e2e-ha-down e2e-ha-logs

# Version information
VERSION ?= dev
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -X github.com/shawnbutts/keystone-core/pkg/version.Version=$(VERSION) \
           -X github.com/shawnbutts/keystone-core/pkg/version.GitCommit=$(GIT_COMMIT) \
           -X github.com/shawnbutts/keystone-core/pkg/version.BuildDate=$(BUILD_DATE)

help:
	@echo "Keystone Core Build System"
	@echo ""
	@echo "Build targets (output: build/bin/):"
	@echo "  proto              - Generate protobuf code from .proto files"
	@echo "  build              - Build all binaries for current platform"
	@echo "  build-all-platforms - Build all binaries for all platforms"
	@echo "  server             - Build kscore-server binary"
	@echo "  agent              - Build kscore-agent binary"
	@echo "  cli                - Build kscorectl binary"
	@echo "  exec               - Build kscore-exec plugin"
	@echo "  state              - Build kscore-state plugin"
	@echo "  monitor            - Build kscore-monitor TUI"
	@echo "  test               - Run tests"
	@echo "  lint               - Run linters"
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

build: server agent cli exec state monitor

server:
	@echo "Building kscore-server..."
	@mkdir -p build/bin
	go build -ldflags "$(LDFLAGS)" -o build/bin/kscore-server ./cmd/kscore-server
	@echo "Built: build/bin/kscore-server"

agent:
	@echo "Building kscore-agent..."
	@mkdir -p build/bin
	go build -ldflags "$(LDFLAGS)" -o build/bin/kscore-agent ./cmd/kscore-agent
	@echo "Built: build/bin/kscore-agent"

cli:
	@echo "Building kscorectl..."
	@mkdir -p build/bin
	go build -ldflags "$(LDFLAGS)" -o build/bin/kscorectl ./cmd/kscorectl
	@echo "Built: build/bin/kscorectl"

exec:
	@echo "Building kscore-exec..."
	@mkdir -p build/bin
	go build -ldflags "$(LDFLAGS)" -o build/bin/kscore-exec ./cmd/kscore-exec
	@echo "Built: build/bin/kscore-exec"

monitor:
	@echo "Building kscore-monitor..."
	@mkdir -p build/bin
	go build -ldflags "$(LDFLAGS)" -o build/bin/kscore-monitor ./cmd/kscore-monitor
	@echo "Built: build/bin/kscore-monitor"

state:
	@echo "Building kscore-state..."
	@mkdir -p build/bin
	go build -ldflags "$(LDFLAGS)" -o build/bin/kscore-state ./cmd/kscore-state
	@echo "Built: build/bin/kscore-state"

test:
	go test -v -race -coverprofile=coverage.out ./...

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

# Cross-platform builds
build-all-platforms: build-linux build-darwin build-windows
	@echo "All platform builds complete"

build-linux:
	@echo "Building for Linux..."
	@mkdir -p build/bin/linux/amd64 build/bin/linux/arm64
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o build/bin/linux/amd64/kscore-server ./cmd/kscore-server
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o build/bin/linux/amd64/kscore-agent ./cmd/kscore-agent
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o build/bin/linux/amd64/kscore-exec ./cmd/kscore-exec
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o build/bin/linux/amd64/kscore-state ./cmd/kscore-state
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o build/bin/linux/amd64/kscore-monitor ./cmd/kscore-monitor
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o build/bin/linux/amd64/kscorectl ./cmd/kscorectl
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o build/bin/linux/arm64/kscore-server ./cmd/kscore-server
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o build/bin/linux/arm64/kscore-agent ./cmd/kscore-agent
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o build/bin/linux/arm64/kscore-exec ./cmd/kscore-exec
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o build/bin/linux/arm64/kscore-state ./cmd/kscore-state
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o build/bin/linux/arm64/kscore-monitor ./cmd/kscore-monitor
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o build/bin/linux/arm64/kscorectl ./cmd/kscorectl
	@echo "Linux builds complete"

build-darwin:
	@echo "Building for macOS..."
	@mkdir -p build/bin/darwin/amd64 build/bin/darwin/arm64
	GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o build/bin/darwin/amd64/kscore-server ./cmd/kscore-server
	GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o build/bin/darwin/amd64/kscore-agent ./cmd/kscore-agent
	GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o build/bin/darwin/amd64/kscore-exec ./cmd/kscore-exec
	GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o build/bin/darwin/amd64/kscore-state ./cmd/kscore-state
	GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o build/bin/darwin/amd64/kscore-monitor ./cmd/kscore-monitor
	GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o build/bin/darwin/amd64/kscorectl ./cmd/kscorectl
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o build/bin/darwin/arm64/kscore-server ./cmd/kscore-server
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o build/bin/darwin/arm64/kscore-agent ./cmd/kscore-agent
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o build/bin/darwin/arm64/kscore-exec ./cmd/kscore-exec
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o build/bin/darwin/arm64/kscore-state ./cmd/kscore-state
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o build/bin/darwin/arm64/kscore-monitor ./cmd/kscore-monitor
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o build/bin/darwin/arm64/kscorectl ./cmd/kscorectl
	@echo "macOS builds complete"

build-windows:
	@echo "Building for Windows..."
	@mkdir -p build/bin/windows/amd64
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o build/bin/windows/amd64/kscore-server.exe ./cmd/kscore-server
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o build/bin/windows/amd64/kscore-agent.exe ./cmd/kscore-agent
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o build/bin/windows/amd64/kscore-exec.exe ./cmd/kscore-exec
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o build/bin/windows/amd64/kscore-state.exe ./cmd/kscore-state
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o build/bin/windows/amd64/kscore-monitor.exe ./cmd/kscore-monitor
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o build/bin/windows/amd64/kscorectl.exe ./cmd/kscorectl
	@echo "Windows builds complete"

# Documentation targets
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

# Linting
lint:
	@echo "Running linters..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
		exit 1; \
	fi

# Release targets (requires goreleaser)
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

# E2E Testing targets
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

e2e-full: e2e-build
	@echo "Running full E2E test suite..."
	KSCORE_E2E_TESTS=1 KSCORE_ROOT=$(shell pwd) go test -v -timeout 30m ./test/e2e/...
	@echo "Full E2E tests complete"
	$(E2E_COMPOSE) down -v --remove-orphans

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

# HA Cluster E2E Testing targets
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
	KSCORE_E2E_TESTS=1 KSCORE_TOPOLOGY=ha-cluster KSCORE_SKIP_BUILD=1 KSCORE_ROOT=$(shell pwd) go test -v -timeout 30m ./test/e2e/topology/...
	@echo "HA cluster E2E tests complete"
	$(HA_COMPOSE) down -v --remove-orphans
