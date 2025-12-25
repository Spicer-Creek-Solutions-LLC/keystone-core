.PHONY: help proto build test clean deps build-all-platforms

# Version information
VERSION ?= dev
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -X github.com/titananvil/titan-anvil/pkg/version.Version=$(VERSION) \
           -X github.com/titananvil/titan-anvil/pkg/version.GitCommit=$(GIT_COMMIT) \
           -X github.com/titananvil/titan-anvil/pkg/version.BuildDate=$(BUILD_DATE)

help:
	@echo "TitanAnvil Build System"
	@echo ""
	@echo "Available targets:"
	@echo "  proto              - Generate protobuf code from .proto files"
	@echo "  build              - Build all binaries for current platform"
	@echo "  build-all-platforms - Build all binaries for all platforms"
	@echo "  server             - Build titananvil-server binary"
	@echo "  agent              - Build titananvil-agent binary"
	@echo "  cli                - Build titanctl binary"
	@echo "  test               - Run tests"
	@echo "  clean              - Remove build artifacts"
	@echo "  deps               - Install/update dependencies"
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

build: server agent cli

server:
	@echo "Building titananvil-server..."
	go build -ldflags "$(LDFLAGS)" -o bin/titananvil-server ./cmd/titananvil-server
	@echo "Built: bin/titananvil-server"

agent:
	@echo "Building titananvil-agent..."
	go build -ldflags "$(LDFLAGS)" -o bin/titananvil-agent ./cmd/titananvil-agent
	@echo "Built: bin/titananvil-agent"

cli:
	@echo "Building titanctl..."
	go build -ldflags "$(LDFLAGS)" -o bin/titanctl ./cmd/titanctl
	@echo "Built: bin/titanctl"

test:
	go test -v -race -coverprofile=coverage.out ./...

clean:
	rm -rf bin/
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
	@mkdir -p bin/linux/amd64 bin/linux/arm64
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o bin/linux/amd64/titananvil-server ./cmd/titananvil-server
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o bin/linux/amd64/titananvil-agent ./cmd/titananvil-agent
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o bin/linux/amd64/titanctl ./cmd/titanctl
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o bin/linux/arm64/titananvil-server ./cmd/titananvil-server
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o bin/linux/arm64/titananvil-agent ./cmd/titananvil-agent
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o bin/linux/arm64/titanctl ./cmd/titanctl
	@echo "Linux builds complete"

build-darwin:
	@echo "Building for macOS..."
	@mkdir -p bin/darwin/amd64 bin/darwin/arm64
	GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o bin/darwin/amd64/titananvil-server ./cmd/titananvil-server
	GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o bin/darwin/amd64/titananvil-agent ./cmd/titananvil-agent
	GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o bin/darwin/amd64/titanctl ./cmd/titanctl
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o bin/darwin/arm64/titananvil-server ./cmd/titananvil-server
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o bin/darwin/arm64/titananvil-agent ./cmd/titananvil-agent
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o bin/darwin/arm64/titanctl ./cmd/titanctl
	@echo "macOS builds complete"

build-windows:
	@echo "Building for Windows..."
	@mkdir -p bin/windows/amd64
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o bin/windows/amd64/titananvil-server.exe ./cmd/titananvil-server
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o bin/windows/amd64/titananvil-agent.exe ./cmd/titananvil-agent
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o bin/windows/amd64/titanctl.exe ./cmd/titanctl
	@echo "Windows builds complete"
