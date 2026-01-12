# Development Guide

This document contains the detailed workflow, standards, and expectations for developing on
Keystone Core.

## Prerequisites

- Go 1.25 or later
- Git
- Make (optional, for convenience targets)
- Docker/Podman (for E2E tests)

## Setting Up Your Environment

```bash
# Fork the repository on GitHub, then clone your fork
git clone https://github.com/YOUR_USERNAME/keystone-core.git
cd keystone-core

# Add upstream remote
git remote add upstream https://github.com/shawnbutts/keystone-core.git

# Install dependencies
go mod download

# Build
make build
# or: go build ./...

# Run tests
make test
# or: go test ./...
```

## Contribution Workflow

### 1. Create a Branch

```bash
# Sync with upstream
git fetch upstream
git checkout main
git merge upstream/main

# Create a feature branch
git checkout -b feature/your-feature-name
```

**Branch naming conventions:**
- `feature/` - New features
- `fix/` - Bug fixes
- `docs/` - Documentation changes
- `refactor/` - Code refactoring
- `test/` - Test additions/improvements

### 2. Make Changes

Ensure:

- **Code compiles**: `make build` succeeds
- **Tests pass**: `make test` succeeds
- **Coverage maintained**: Aim for >80% coverage on new code
- **Documentation updated**: Add/update docs for new features

### 3. Commit Changes

Write clear, descriptive commit messages:

```bash
git add .
git commit -m "Add deployment verification for ArgoCD

- Implement ArgoCD client for deployment status checks
- Add verification workflow engine
- Include retry logic and timeout handling
- Add comprehensive unit tests

Closes #123"
```

**Commit message format:**
```
<type>: <summary line (50 chars max)>

<body: detailed description of changes (72 chars per line)>

<footer: references to issues/PRs>
```

**Types:** `feat`, `fix`, `docs`, `test`, `refactor`, `perf`, `chore`

### 4. Push and Create PR

```bash
git push origin feature/your-feature-name
```

Then create a pull request on GitHub.

### 5. Code Review

Maintainers will review your PR and may request changes:

- **Address feedback promptly**
- **Keep discussion focused**
- **Update your branch if needed**

```bash
# Sync with upstream main
git fetch upstream
git rebase upstream/main
git push origin feature/your-feature-name --force-with-lease
```

## Coding Standards

### Go Style Guide

We follow standard Go conventions:

- **Use `gofmt`**: All code must be formatted with `gofmt`
- **Run `go vet`**: No vet warnings allowed
- **Use `golangci-lint`**: Run `make lint` before committing
- **Follow Effective Go**: See [Effective Go](https://golang.org/doc/effective_go.html)

**Code organization:**
```
pkg/
├── api/           # Public APIs (gRPC/REST)
├── agent/         # Agent implementation
├── controlplane/  # Control plane services
├── state/         # State management
├── events/        # Event system
├── policy/        # Policy engine
└── ...
```

### Error Handling

```go
// Good: Wrap errors with context
if err := doSomething(); err != nil {
    return fmt.Errorf("failed to do something: %w", err)
}

// Bad: Swallow errors or lose context
doSomething()
err = doSomething()
return err
```

### Logging

Use structured logging with `pkg/logging`:

```go
import "github.com/shawnbutts/keystone-core/pkg/logging"

log := logging.NewLogger(logging.Config{
    Level: logging.Info,
    Name:  "mycomponent",
})

log.Info("Agent connected",
    logging.String("agent_id", agentID),
    logging.String("ip", ipAddress),
)
```

## Testing

### Running Tests

```bash
# All tests
make test

# Specific package
go test ./pkg/state/...

# With coverage
go test -cover ./...

# With race detection
go test -race ./...

# E2E tests (requires Docker)
KSCORE_E2E_TESTS=1 make e2e-test
```

### Writing Tests

- **Table-driven tests**: Use for multiple test cases
- **Descriptive names**: `TestFunctionName_Scenario_ExpectedBehavior`
- **Mock external dependencies**: Use interfaces and mocks
- **Test edge cases**: Not just happy paths

```go
func TestStateExecutor_Apply_Success(t *testing.T) {
    tests := []struct {
        name     string
        state    *StateDeclaration
        want     StateResult
        wantErr  bool
    }{
        {
            name: "file present creates file",
            state: &StateDeclaration{
                Module: "file",
                Params: map[string]interface{}{
                    "path":  "/tmp/test.txt",
                    "state": "present",
                },
            },
            want:    StateResult{Changed: true, Success: true},
            wantErr: false,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            executor := NewStateExecutor()
            got, err := executor.Apply(tt.state)

            if (err != nil) != tt.wantErr {
                t.Errorf("Apply() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if !reflect.DeepEqual(got, tt.want) {
                t.Errorf("Apply() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

### Integration Tests

Place integration tests in `*_test.go` files with build tags:

```go
//go:build integration
// +build integration

package integration

func TestFullWorkflow(t *testing.T) {
    // Integration test code
}
```

Run with: `go test -tags=integration ./...`

## Documentation

### Code Comments

```go
// AgentManager manages the lifecycle of connected agents.
// It handles registration, heartbeat tracking, and graceful shutdown.
type AgentManager struct {
    agents map[string]*Agent
    mu     sync.RWMutex
}

// RegisterAgent registers a new agent with the control plane.
// It returns an error if the agent is already registered or if
// the registration fails validation.
func (m *AgentManager) RegisterAgent(agent *Agent) error {
    // Implementation
}
```

### User-Facing Documentation

When adding features, update documentation:

- **Reference docs**: API/CLI changes go in `docs/content/en/docs/reference/`
- **Guides**: Tutorials and how-tos go in `docs/content/en/docs/guides/`
- **Examples**: Add to `examples/` directory

## AI-Assisted Contributions

AI-assisted contributions are welcome. Expectations:

- Verify generated code compiles and passes tests
- Rewrite unclear text for clarity
- Do not blindly paste large diffs
- Attribute AI usage in PR description if substantial

## Developer Certificate of Origin (DCO)

By contributing to Keystone Core, you agree that your contributions will be licensed under the
Apache License 2.0.

All contributions must include a sign-off:

```bash
git commit --signoff -m "Your commit message"
```

This adds:
```
Signed-off-by: Your Name <your.email@example.com>
```

## Getting Help

- **GitHub Discussions**: Ask questions about contributing
- **Discord**: Real-time chat with maintainers
- **Issue comments**: Ask for clarification on specific issues
