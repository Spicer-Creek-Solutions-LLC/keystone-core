---
title: "Contributing Guide"
weight: 1
description: >
  Learn how to contribute code, documentation, and bug reports to Keystone Core
---

Thank you for your interest in contributing to Keystone Core! This guide will help you get started with contributing code, documentation, bug reports, and feature requests.

## Ways to Contribute

### Report Bugs

Found a bug? Please [open an issue](https://github.com/shawnbutts/keystone-core/issues/new?template=bug_report.md) with:

- **Clear title**: Descriptive summary of the issue
- **Steps to reproduce**: Detailed steps to trigger the bug
- **Expected behavior**: What should happen
- **Actual behavior**: What actually happens
- **Environment**: OS, Keystone Core version, Go version
- **Logs**: Relevant log output (use `--log-level=debug`)

**Example bug report:**
```
Title: Agent disconnects after 5 minutes with embedded NATS

Environment:
- OS: Ubuntu 22.04
- Keystone Core: v0.5.0
- Go: 1.21.5

Steps to reproduce:
1. Start kscore-server with embedded NATS
2. Start kscore-agent connecting to server
3. Wait 5 minutes

Expected: Agent stays connected
Actual: Agent disconnects with "i/o timeout" error

Logs:
2024-12-27 10:15:23 ERROR agent disconnected error="i/o timeout"
```

### Suggest Features

Have an idea? [Open a feature request](https://github.com/shawnbutts/keystone-core/issues/new?template=feature_request.md) with:

- **Use case**: What problem does this solve?
- **Proposed solution**: How should it work?
- **Alternatives**: Other approaches considered
- **Additional context**: Examples, mockups, related issues

### Improve Documentation

Documentation improvements are always welcome:

- Fix typos or clarify confusing sections
- Add examples and tutorials
- Improve API/CLI reference
- Translate documentation

See [Development Guide](development/) for how to build and test docs locally.

### Contribute Code

We welcome code contributions! See the sections below for the contribution workflow.

## Contribution Workflow

### 1. Fork and Clone

```bash
# Fork the repository on GitHub, then clone your fork
git clone https://github.com/YOUR_USERNAME/keystone-core.git
cd keystone-core

# Add upstream remote
git remote add upstream https://github.com/shawnbutts/keystone-core.git
```

### 2. Create a Branch

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

### 3. Make Changes

Follow our [coding standards](#coding-standards) and ensure:

- **Code compiles**: `make build` succeeds
- **Tests pass**: `make test` succeeds
- **Coverage maintained**: Aim for >80% coverage on new code
- **Documentation updated**: Add/update docs for new features

### 4. Commit Changes

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

### 5. Push and Create PR

```bash
# Push to your fork
git push origin feature/your-feature-name

# Create a pull request on GitHub
```

**Pull request checklist:**
- [ ] Tests added/updated and passing
- [ ] Documentation updated
- [ ] Code follows style guidelines
- [ ] Commit messages are clear
- [ ] Branch is up to date with main
- [ ] PR description explains changes and motivation

### 6. Code Review

Maintainers will review your PR and may request changes:

- **Address feedback promptly**: Make requested changes
- **Keep discussion focused**: Stay on topic
- **Be patient**: Reviews may take a few days
- **Update your branch**: Sync with main if needed

```bash
# Sync with upstream main
git fetch upstream
git rebase upstream/main

# Push updates (may need --force-with-lease)
git push origin feature/your-feature-name --force-with-lease
```

### 7. Merge

Once approved, maintainers will merge your PR. Congratulations, you're a contributor! 🎉

## Coding Standards

### Go Style Guide

We follow standard Go conventions:

- **Use `gofmt`**: All code must be formatted with `gofmt`
- **Run `go vet`**: No vet warnings allowed
- **Use `golangci-lint`**: Run `make lint` before committing
- **Follow effective Go**: See [Effective Go](https://golang.org/doc/effective_go.html)

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

// Create logger
log := logging.NewLogger(logging.Config{
    Level: logging.Info,
    Name:  "mycomponent",
})

// Log with fields
log.Info("Agent connected",
    logging.String("agent_id", agentID),
    logging.String("ip", ipAddress),
)
```

### Testing

#### Unit Tests

- **Table-driven tests**: Use for multiple test cases
- **Descriptive names**: TestFunctionName_Scenario_ExpectedBehavior
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
        // More test cases...
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

#### Integration Tests

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

### Documentation

#### Code Comments

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

#### User-Facing Documentation

When adding features, update documentation:

- **Reference docs**: API/CLI changes go in `docs/content/en/docs/reference/`
- **Guides**: Tutorials and how-tos go in `docs/content/en/docs/guides/`
- **Examples**: Add to `examples/` directory

## Pull Request Guidelines

### PR Title Format

```
<type>(<scope>): <subject>

Examples:
feat(policy): add CEL expression support
fix(agent): resolve heartbeat timeout issue
docs(api): update authentication examples
```

### PR Description Template

```markdown
## Description
Brief summary of changes.

## Motivation
Why is this change needed? What problem does it solve?

## Changes
- Bullet list of changes
- Include both code and docs

## Testing
How was this tested?
- Unit tests: description
- Integration tests: description
- Manual testing: steps performed

## Screenshots (if applicable)
Add screenshots for UI changes.

## Checklist
- [ ] Tests added/updated
- [ ] Documentation updated
- [ ] CHANGELOG.md updated (for user-facing changes)
- [ ] Breaking changes noted in commit message
```

### Review Process

PRs require:
- **Approval from at least one maintainer**
- **All CI checks passing** (tests, linting, builds)
- **No unresolved conversations**
- **Up-to-date with main branch**

## First-Time Contributors

New to open source? We've got you covered:

### Good First Issues

Look for issues labeled `good first issue`:
- These are beginner-friendly tasks
- Clear scope and requirements
- Maintainers available to help

### Getting Help

- **GitHub Discussions**: Ask questions about contributing
- **Discord**: Real-time chat with maintainers and community
- **Issue comments**: Ask for clarification on specific issues

### Pairing Sessions

Maintainers offer pairing sessions for first-time contributors:
- Schedule via Discord
- Screen-sharing walkthrough
- Learn the codebase and workflow

## License

By contributing to Keystone Core, you agree that your contributions will be licensed under the Apache License 2.0.

All contributions must include a sign-off certifying that you have the right to submit the code under the project's license:

```bash
git commit --signoff -m "Your commit message"
```

This adds:
```
Signed-off-by: Your Name <your.email@example.com>
```

## Recognition

Contributors are recognized in:
- **CONTRIBUTORS.md**: All contributors listed
- **Release notes**: Notable contributions highlighted
- **Annual report**: Top contributors featured

## Next Steps

- **Set up your development environment**: See [Development Guide](development/)
- **Find an issue to work on**: Browse [good first issues](https://github.com/shawnbutts/keystone-core/labels/good%20first%20issue)
- **Join the community**: Say hello on [Discord](https://discord.gg/kscore)

Thank you for contributing to Keystone Core! 🚀
