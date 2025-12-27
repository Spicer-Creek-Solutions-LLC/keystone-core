---
title: "Development Guide"
weight: 2
description: >
  Set up your development environment and learn about Keystone Core development workflows
---

This guide covers setting up a development environment for Keystone Core, building from source, running tests, and understanding the project structure.

## Prerequisites

### Required Software

| Tool | Version | Purpose |
|------|---------|---------|
| Go | 1.21+ | Primary development language |
| Git | 2.30+ | Version control |
| Make | 3.81+ | Build automation |
| Docker | 20.10+ | Container builds (optional) |

### Optional Software

| Tool | Purpose |
|------|---------|
| golangci-lint | Code linting |
| Hugo Extended | Documentation development |
| Delve | Go debugger |
| goreleaser | Release builds |

### Install Go

**Linux/macOS:**
```bash
# Download and install Go 1.21+
wget https://go.dev/dl/go1.21.5.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.21.5.linux-amd64.tar.gz

# Add to PATH in ~/.bashrc or ~/.zshrc
export PATH=$PATH:/usr/local/go/bin
export PATH=$PATH:$(go env GOPATH)/bin
```

**macOS (Homebrew):**
```bash
brew install go
```

**Windows:**
Download installer from https://go.dev/dl/ and run.

**Verify installation:**
```bash
go version
# Output: go version go1.21.5 linux/amd64
```

### Install Development Tools

```bash
# Install golangci-lint for linting
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Install Delve for debugging
go install github.com/go-delve/delve/cmd/dlv@latest

# Install Hugo for documentation (optional)
# macOS
brew install hugo

# Linux
wget https://github.com/gohugoio/hugo/releases/download/v0.121.1/hugo_extended_0.121.1_linux-amd64.tar.gz
tar -xzf hugo_extended_0.121.1_linux-amd64.tar.gz
sudo mv hugo /usr/local/bin/
```

## Getting the Source

### Clone the Repository

```bash
# Clone the main repository
git clone https://github.com/shawnbutts/keystone-core.git
cd keystone-core

# Or clone your fork
git clone https://github.com/YOUR_USERNAME/keystone-core.git
cd keystone-core
git remote add upstream https://github.com/shawnbutts/keystone-core.git
```

### Repository Structure

```
keystone-core/
├── cmd/                    # Binary entry points
│   ├── kscorectl/          # Main CLI dispatcher
│   ├── kscore-server/      # Control plane server
│   ├── kscore-agent/       # Agent daemon
│   ├── kscore-state/       # State management plugin
│   ├── kscore-exec/        # Remote execution plugin
│   ├── kscore-monitor/     # TUI monitoring tool
│   └── kscore-registry/    # Module registry server
├── pkg/                    # Public packages
│   ├── agent/              # Agent implementation
│   ├── controlplane/       # Control plane services
│   ├── state/              # State persistence
│   ├── statemgmt/          # Declarative state management
│   ├── events/             # Event system
│   ├── policy/             # Policy engine (OPA/CEL)
│   ├── gitops/             # GitOps integration
│   ├── metrics/            # Prometheus metrics
│   ├── logging/            # Structured logging
│   ├── tracing/            # OpenTelemetry tracing
│   ├── health/             # Health checks
│   ├── module/             # Plugin module system
│   ├── k8s/                # Kubernetes integration
│   ├── platform/           # Platform detection
│   ├── cloud/              # Cloud provider integration
│   ├── edge/               # Edge deployment support
│   └── ...
├── modules/                # Module system
│   ├── sdk/                # Module SDKs (Rust, Go, C++)
│   ├── stdlib/             # Standard library modules
│   └── examples/           # Example modules
├── deploy/                 # Deployment configurations
│   ├── kubernetes/         # K8s manifests
│   ├── docker/             # Dockerfiles
│   └── grafana/            # Grafana dashboards
├── docs/                   # Documentation (Hugo)
├── epics/                  # Design documents
├── Makefile                # Build automation
├── go.mod                  # Go module definition
└── go.sum                  # Dependency checksums
```

## Building from Source

### Quick Build

```bash
# Build all binaries
make build

# Binaries are placed in build/bin/
ls build/bin/
# kscore-server  kscore-agent  kscore-state  kscore-exec  kscore-monitor
```

### Build Individual Components

```bash
# Build specific binaries
make build-server      # Control plane server
make build-agent       # Agent daemon
make build-cli         # All CLI tools

# Or build manually
go build -o build/bin/kscore-server ./cmd/kscore-server
go build -o build/bin/kscore-agent ./cmd/kscore-agent
```

### Build with Version Information

```bash
# Version is injected at build time
make build VERSION=v0.6.0 COMMIT=$(git rev-parse HEAD)

# Check version
./build/bin/kscore-server version
# kscore-server v0.6.0 (commit: abc123def)
```

### Cross-Compilation

```bash
# Build for Linux
GOOS=linux GOARCH=amd64 make build

# Build for Windows
GOOS=windows GOARCH=amd64 make build

# Build for macOS ARM64
GOOS=darwin GOARCH=arm64 make build

# Build all platforms
make build-all
```

### Docker Build

```bash
# Build Docker images
make docker-build

# Build specific image
docker build -t kscore-server:dev -f deploy/docker/Dockerfile.server .
docker build -t kscore-agent:dev -f deploy/docker/Dockerfile.agent .

# Run container
docker run -p 8080:8080 -p 4222:4222 kscore-server:dev
```

## Running Tests

### Unit Tests

```bash
# Run all unit tests
make test

# Run with verbose output
make test-verbose

# Run with coverage
make test-coverage

# Run specific package tests
go test ./pkg/state/...
go test ./pkg/events/...

# Run specific test
go test -run TestAgentRegistration ./pkg/agent/
```

### Integration Tests

Integration tests require external dependencies (NATS, database):

```bash
# Run integration tests (requires Docker)
make test-integration

# Run manually with build tag
go test -tags=integration ./...

# Run specific integration test
go test -tags=integration -run TestFullWorkflow ./test/integration/
```

### Test Coverage

```bash
# Generate coverage report
make test-coverage

# View HTML coverage report
go tool cover -html=coverage.out -o coverage.html
open coverage.html

# Coverage by package
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out | grep total
```

**Coverage targets:**
- Unit tests: >80% overall
- Critical packages: >90% (state, agent, policy)
- New code: 100% coverage expected

### Benchmarks

```bash
# Run benchmarks
make benchmark

# Run specific benchmark
go test -bench=BenchmarkStateApply ./pkg/statemgmt/

# Run with memory profiling
go test -bench=. -benchmem ./pkg/events/
```

## Linting and Formatting

### Code Formatting

```bash
# Format all code
make fmt

# Or manually
gofmt -w .
go fmt ./...
```

### Linting

```bash
# Run all linters
make lint

# Fix auto-fixable issues
make lint-fix

# Run specific linter
golangci-lint run --enable=goimports ./...
```

### Pre-commit Checks

```bash
# Run all pre-commit checks
make check

# This runs:
# - go fmt
# - go vet
# - golangci-lint
# - go mod tidy
# - make test
```

## Local Development

### Running Locally

**Start the Control Plane:**
```bash
# Run with embedded NATS and SQLite (development mode)
./build/bin/kscore-server --config configs/server-dev.yaml

# Or run from source
go run ./cmd/kscore-server --log-level=debug
```

**Start an Agent:**
```bash
# Connect to local control plane
./build/bin/kscore-agent \
  --server-url=nats://localhost:4222 \
  --agent-id=dev-agent-1 \
  --log-level=debug

# Or run from source
go run ./cmd/kscore-agent --server-url=nats://localhost:4222
```

**Use CLI Tools:**
```bash
# Execute a remote command
./build/bin/kscore-exec run --target="*" -- hostname

# Apply a state file
./build/bin/kscore-state apply states/webserver.yaml

# Start TUI monitor
./build/bin/kscore-monitor
```

### Development Configuration

Create a development config file:

```yaml
# configs/server-dev.yaml
server:
  listen_address: "0.0.0.0:8080"
  grpc_address: "0.0.0.0:50051"

nats:
  mode: embedded
  embedded:
    port: 4222
    jetstream: true

database:
  driver: sqlite
  sqlite:
    path: "./data/dev.db"

logging:
  level: debug
  format: text

telemetry:
  enabled: true
  prometheus:
    enabled: true
    address: ":9090"
```

### Hot Reload

For faster development iteration:

```bash
# Install air for hot reload
go install github.com/cosmtrek/air@latest

# Run with hot reload
air

# Or use make target
make dev
```

### Debugging

**Using Delve:**
```bash
# Debug server
dlv debug ./cmd/kscore-server -- --config configs/server-dev.yaml

# Debug with breakpoint
(dlv) break pkg/agent/manager.go:42
(dlv) continue

# Debug tests
dlv test ./pkg/state/... -- -test.run TestStateApply
```

**VS Code Configuration:**
```json
// .vscode/launch.json
{
  "version": "0.2.0",
  "configurations": [
    {
      "name": "Debug Server",
      "type": "go",
      "request": "launch",
      "mode": "debug",
      "program": "${workspaceFolder}/cmd/kscore-server",
      "args": ["--config", "configs/server-dev.yaml"],
      "env": {
        "LOG_LEVEL": "debug"
      }
    },
    {
      "name": "Debug Tests",
      "type": "go",
      "request": "launch",
      "mode": "test",
      "program": "${workspaceFolder}/pkg/state",
      "args": ["-test.run", "TestStateApply"]
    }
  ]
}
```

## Working with Modules

### Starlark Module Development

```bash
# Create a new Starlark module
mkdir -p modules/mymodule/states

# Write module manifest
cat > modules/mymodule/module.yaml << 'EOF'
name: myorg/mymodule
version: 0.1.0
description: My custom module
capabilities:
  - fs.read
  - fs.write
entrypoint: states/main.star
EOF

# Write Starlark code
cat > modules/mymodule/states/main.star << 'EOF'
def apply(ctx):
    # Module implementation
    return {"status": "success"}
EOF

# Test module
go run ./cmd/kscore-module test modules/mymodule
```

### WASM Module Development

**Rust:**
```bash
cd modules/sdk/rust/examples/hello-world

# Build WASM module
cargo build --target wasm32-wasi --release

# Test module
go run ./cmd/kscore-module test target/wasm32-wasi/release/hello_world.wasm
```

**Go (TinyGo):**
```bash
cd modules/sdk/go/examples/hello-world

# Build WASM module
tinygo build -o hello.wasm -target wasi main.go

# Test module
go run ./cmd/kscore-module test hello.wasm
```

## Documentation Development

### Build Documentation

```bash
# Install dependencies
cd docs
npm install

# Start development server
hugo server -D

# Build static site
hugo

# Output is in build/docs/
```

### Documentation Structure

```
docs/
├── content/en/docs/
│   ├── getting-started/    # Installation, quick start
│   ├── concepts/           # Core concepts explained
│   ├── reference/          # API, CLI, config reference
│   ├── operations/         # Deployment, maintenance
│   └── community/          # Contributing, roadmap
├── static/images/          # Screenshots, diagrams
└── hugo.toml              # Hugo configuration
```

### Writing Documentation

**Front matter:**
```yaml
---
title: "Page Title"
weight: 10  # Controls ordering in sidebar
description: >
  Brief description for search and SEO
---
```

**Code blocks:**
```markdown
```bash
# Shell commands
kscorectl exec run --target="*" -- hostname
```

```yaml
# YAML configuration
server:
  listen: ":8080"
```

```go
// Go code examples
func main() {
    // code
}
```
```

**Admonitions:**
```markdown
{{% alert title="Warning" color="warning" %}}
This is a warning message.
{{% /alert %}}

{{% alert title="Note" %}}
This is an informational note.
{{% /alert %}}
```

## Release Process

Keystone Core uses [GoReleaser](https://goreleaser.com/) for building release artifacts, including binaries, archives, and Linux packages (DEB/RPM).

### Install GoReleaser

```bash
# Install goreleaser
make install-goreleaser

# Or manually
go install github.com/goreleaser/goreleaser/v2@latest
```

### Version Tagging

```bash
# Create a release tag
git tag -a v0.6.0 -m "Release v0.6.0"
git push origin v0.6.0

# Release builds are automated via GitHub Actions
```

### Local Release Build

```bash
# Validate goreleaser configuration
make release-dry-run

# Build snapshot release (all artifacts, no publish)
make release-snapshot

# Build full release (requires GITHUB_TOKEN)
export GITHUB_TOKEN=your_token
make release
```

### Release Artifacts

GoReleaser produces the following artifacts:

**Binaries:**
- `kscore-server` - Control plane server
- `kscore-agent` - Agent daemon
- `kscorectl` - Main CLI
- `kscore-exec` - Remote execution plugin
- `kscore-state` - State management plugin
- `kscore-monitor` - TUI monitoring tool

**Archives:**
- `keystone-core_VERSION_linux_amd64.tar.gz`
- `keystone-core_VERSION_linux_arm64.tar.gz`
- `keystone-core_VERSION_darwin_amd64.tar.gz`
- `keystone-core_VERSION_darwin_arm64.tar.gz`
- `keystone-core_VERSION_windows_amd64.zip`

**Linux Packages:**
- `kscore-server_VERSION_linux_amd64.deb` / `.rpm`
- `kscore-agent_VERSION_linux_amd64.deb` / `.rpm`
- `kscore-cli_VERSION_linux_amd64.deb` / `.rpm`

**Checksums:**
- `checksums.txt` - SHA256 checksums for all artifacts

### Installing Linux Packages

```bash
# Debian/Ubuntu
sudo dpkg -i kscore-server_*.deb
sudo dpkg -i kscore-agent_*.deb
sudo dpkg -i kscore-cli_*.deb

# RHEL/CentOS/Fedora
sudo rpm -i kscore-server-*.rpm
sudo rpm -i kscore-agent-*.rpm
sudo rpm -i kscore-cli-*.rpm
```

### Changelog

Update `CHANGELOG.md` following Keep a Changelog format:

```markdown
## [0.6.0] - 2024-12-27

### Added
- Feature X for improved Y

### Changed
- Updated Z for better performance

### Fixed
- Bug in W that caused Q

### Deprecated
- Old API endpoint (use new one instead)
```

## Troubleshooting

### Build Issues

**Module download errors:**
```bash
# Clear module cache
go clean -modcache

# Re-download dependencies
go mod download
```

**CGO issues:**
```bash
# Disable CGO if having issues
CGO_ENABLED=0 make build
```

**Version mismatch:**
```bash
# Ensure Go version matches
go version  # Should be 1.21+

# Update dependencies
go mod tidy
```

### Test Issues

**Flaky tests:**
```bash
# Run with race detector
go test -race ./...

# Run test multiple times
go test -count=10 ./pkg/agent/...
```

**Integration test failures:**
```bash
# Check Docker is running
docker ps

# Check ports are available
lsof -i :4222  # NATS port
lsof -i :8080  # Server port
```

### Debug Logging

```bash
# Enable debug logging
export LOG_LEVEL=debug

# Enable trace logging for specific packages
export LOG_PKG_AGENT=trace
export LOG_PKG_NATS=trace
```

## IDE Setup

### VS Code

**Recommended extensions:**
- Go (official)
- Go Test Explorer
- GitLens
- YAML
- Markdown All in One

**Settings (`.vscode/settings.json`):**
```json
{
  "go.useLanguageServer": true,
  "go.lintTool": "golangci-lint",
  "go.lintFlags": ["--fast"],
  "go.testFlags": ["-v"],
  "editor.formatOnSave": true,
  "[go]": {
    "editor.defaultFormatter": "golang.go"
  }
}
```

### GoLand

**Recommended setup:**
1. Enable Go modules integration
2. Configure golangci-lint as external tool
3. Set up file watchers for `gofmt`
4. Configure test runner with `-v` flag

### Vim/Neovim

**Recommended plugins:**
- vim-go (Vim) or nvim-lspconfig with gopls (Neovim)
- ale or nvim-lint for linting
- vim-test for running tests

## Next Steps

- **Start Contributing**: See [Contributing Guide](../contributing/)
- **Explore the Codebase**: Read through `pkg/` directories
- **Pick an Issue**: Browse [good first issues](https://github.com/shawnbutts/keystone-core/labels/good%20first%20issue)
- **Join the Community**: Connect on [Discord](https://discord.gg/kscore)
