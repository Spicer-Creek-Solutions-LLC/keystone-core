.PHONY: help proto build test clean deps build-all-platforms docs docs-serve docs-pdf docs-all

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
	@echo "  monitor            - Build kscore-monitor TUI"
	@echo "  test               - Run tests"
	@echo "  clean              - Remove all build artifacts (build/)"
	@echo "  deps               - Install/update dependencies"
	@echo ""
	@echo "Documentation targets (output: build/docs/ and build/pdfs/):"
	@echo "  docs               - Build Hugo documentation site → build/docs/"
	@echo "  docs-serve         - Build and serve docs locally with live reload"
	@echo "  docs-pdf           - Generate PDF documentation → build/pdfs/"
	@echo "  docs-all           - Build site and generate PDFs"
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

build: server agent cli exec monitor

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

test:
	go test -v -race -coverprofile=coverage.out ./...

clean:
	rm -rf build/
	rm -rf data/
	rm -f coverage.out

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
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o build/bin/linux/amd64/kscore-monitor ./cmd/kscore-monitor
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o build/bin/linux/amd64/kscorectl ./cmd/kscorectl
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o build/bin/linux/arm64/kscore-server ./cmd/kscore-server
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o build/bin/linux/arm64/kscore-agent ./cmd/kscore-agent
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o build/bin/linux/arm64/kscore-exec ./cmd/kscore-exec
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o build/bin/linux/arm64/kscore-monitor ./cmd/kscore-monitor
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o build/bin/linux/arm64/kscorectl ./cmd/kscorectl
	@echo "Linux builds complete"

build-darwin:
	@echo "Building for macOS..."
	@mkdir -p build/bin/darwin/amd64 build/bin/darwin/arm64
	GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o build/bin/darwin/amd64/kscore-server ./cmd/kscore-server
	GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o build/bin/darwin/amd64/kscore-agent ./cmd/kscore-agent
	GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o build/bin/darwin/amd64/kscore-exec ./cmd/kscore-exec
	GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o build/bin/darwin/amd64/kscore-monitor ./cmd/kscore-monitor
	GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o build/bin/darwin/amd64/kscorectl ./cmd/kscorectl
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o build/bin/darwin/arm64/kscore-server ./cmd/kscore-server
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o build/bin/darwin/arm64/kscore-agent ./cmd/kscore-agent
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o build/bin/darwin/arm64/kscore-exec ./cmd/kscore-exec
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o build/bin/darwin/arm64/kscore-monitor ./cmd/kscore-monitor
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o build/bin/darwin/arm64/kscorectl ./cmd/kscorectl
	@echo "macOS builds complete"

build-windows:
	@echo "Building for Windows..."
	@mkdir -p build/bin/windows/amd64
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o build/bin/windows/amd64/kscore-server.exe ./cmd/kscore-server
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o build/bin/windows/amd64/kscore-agent.exe ./cmd/kscore-agent
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o build/bin/windows/amd64/kscore-exec.exe ./cmd/kscore-exec
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
	@echo "Generating PDF documentation..."
	@if [ ! -d "docs/node_modules" ]; then \
		echo "Installing npm dependencies..."; \
		cd docs && npm install; \
	fi
	@if [ ! -d "$$HOME/.cache/ms-playwright/chromium-"* ] 2>/dev/null; then \
		echo "Installing Playwright browsers..."; \
		cd docs && npm run install-browsers; \
	fi
	@cd docs && npm run generate-pdfs

docs-all: docs docs-pdf
	@echo "Documentation site and PDFs complete"
