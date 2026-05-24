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
# Fork the repository on Codeberg, then clone your fork
git clone https://codeberg.org/YOUR_USERNAME/keystone-core.git
cd keystone-core

# Add upstream remote
git remote add upstream https://codeberg.org/Spicer-Creek-Solutions-LLC/keystone-core.git

# Install dependencies
go mod download

# Build (writes to build/bin/$GOOS/$GOARCH/)
make build

# Run tests
make test
```

## Build Artifact Discipline

All binaries belong under `build/bin/$GOOS/$GOARCH/`. The Makefile
enforces this:

- `make build` writes host-platform binaries to
  `build/bin/$(go env GOOS)/$(go env GOARCH)/`.
- `make build-all-platforms` cross-compiles to the same layout for
  every entry in the v1.0 platform matrix.
- `make release` / `make release-snapshot` write goreleaser artifacts
  to `dist/`.

**Do not** run `go build ./cmd/foo` or `go build ./...` from the repo
root — both write binaries (named after the package or the cwd) into
the working directory. Those stray binaries are gitignored but pollute
`ls`, confuse new contributors, and slow down `make clean`.

If you genuinely need to invoke `go build` directly (e.g., debugging a
specific build flag), pass `-o`:

```bash
go build -o build/bin/$(go env GOOS)/$(go env GOARCH)/kscore-server ./cmd/kscore-server
```

Two clean targets cover removal:

- `make clean` — removes every gitignored build/runtime artifact
  category: `build/`, `dist/`, root strays (`kscore-*`, `trackerctl`,
  `*.test`), per-tool binaries, integration-test runtime state
  (`data/`, `*.db`), dev-mode configs, Hugo output.
- `make clean-all` — `clean` plus `.cache/` (grype/trivy scan DBs).
  Forces a slow re-download on the next security scan; use only when
  you genuinely want a fresh cache.

A CI lint (`make clean-check`, run in the `lint` job) fails any PR
that committed a stray binary at repo root. Run it locally before
opening a PR if you suspect you bypassed the Makefile:

```bash
make clean-check
```

## Local Dev Topology

The fastest way to iterate on server / agent code locally is the
docker-compose harness (`make e2e-up`), which brings up a full
multi-component topology — server + 2 agents + Postgres + NATS — from
source. This is the development equivalent of the operator-install
walkthrough in [`GETTING-STARTED.md`](GETTING-STARTED.md); use the
operator path for real-host validation, this harness for ad-hoc dev
iteration.

### Start

```bash
make e2e-up
```

Builds the `kscore-server` and `kscore-agent` images, starts a
5-container compose stack (1× server, 2× agents, Postgres 16,
NATS 2.10), and waits for every healthcheck. On a cold cache the
first run can take a few minutes — most of that is image pull plus
the Go build inside the container.

Ports exposed on `127.0.0.1`:

| Port | Service | Use |
|------|---------|-----|
| `8080` | server HTTP | `/health/*`, `/metrics`, REST API |
| `5397` | server gRPC | operator-facing gRPC |
| `8081` | server GitOps webhook | inbound webhook receiver |
| `5432` | postgres | DB access for queries |
| `8222` | nats monitoring | broker introspection |

Quick check:

```bash
curl -fsS http://127.0.0.1:8080/health/ready | jq
```

### Talk to the dev server

The server logs a dev admin API key cleartext once at boot (see
[`pkg/api/apikeys/dev.go`](../../pkg/api/apikeys/dev.go)). Extract it
into a shell variable for use in subsequent calls:

```bash
DEV_KEY=$(docker logs kscore-e2e-server 2>&1 \
  | grep "DEV API KEY GENERATED" \
  | python3 -c 'import sys, json; print(json.loads(sys.stdin.read().strip().split("\n")[-1])["key"])')
```

(In a real deployment, use `kscorectl apikey create` instead.)

The agents in the harness are distroless (no shell), so commands sent
to them must reference binaries that already exist inside the image —
`/usr/local/bin/kscore --version` is a clean exit-0 sanity check:

```bash
grpcurl -plaintext -H "authorization: Bearer $DEV_KEY" \
    -d '{
          "agent_id":"agent-1",
          "command":"/usr/local/bin/kscore",
          "args":["--version"],
          "timeout_seconds":10
        }' \
    127.0.0.1:5397 keystone.core.v1.ControlPlaneService/ExecuteCommand
```

You get a stream of events: a `command_id` first, then a `completion`.
Note that proto3 omits zero-valued fields from JSON output, so an
`exit_code: 0` may appear as an absent `exit_code` field rather than
the literal `0` — that's a success, not a missing value.

### Tear down

```bash
make e2e-down
```

Removes containers, volumes, and the docker network.

### Dev-topology troubleshooting

**`make e2e-up` hangs at "Container kscore-e2e-server Healthy".** The
first run downloads Go modules + builds the binary inside the
container. On a cold cache this can take several minutes. Run
`docker logs -f kscore-e2e-server` in another shell to watch progress.

**"Authorization: Bearer" returns 401.** The dev API key is
cleartext-once per server-process. If you restarted the server
(`make e2e-down && make e2e-up`), re-export `DEV_KEY` from the new
process's logs.

**`grpcurl` not installed.**

```bash
go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest
```

The gRPC server doesn't have reflection enabled in dev mode; pass
`-import-path api/proto -proto api/proto/keystone/core/v1/controlplane.proto`
(or pre-compiled descriptors) for grpcurl to discover services.

**State apply fails inside the distroless agent.** `/tmp` is writable
in the distroless image, but parent directories under `/etc`, `/var`,
etc. are read-only. Stick to `/tmp/...` for quick experiments; real
deployments use a full Linux host.

**Tests want Postgres.** `make test-integration` is gated on
`KSCORE_TEST_POSTGRES_DSN`. If unset, Postgres-dependent tests skip.
Set it to a reachable Postgres (the harness exposes one at
`127.0.0.1:5432`):

```bash
export KSCORE_TEST_POSTGRES_DSN="postgres://kscore:kscore@127.0.0.1:5432/kscore?sslmode=disable"
make test-integration
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

Then create a pull request on Codeberg.

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
- **Use `golangci-lint`**: Run `make lint` before committing. `make lint-fix` auto-applies the safe fixes; `make docs-lint-fix` does the same for markdown.
- **Follow Effective Go**: See [Effective Go](https://golang.org/doc/effective_go.html)

**Code organization:**

```
pkg/           # Public, importable surface (Go API + protobuf)
├── api/       #   gRPC + REST handlers + auth + apikeys
├── module/    #   module loader / verify / registry / manifest / runtime
├── saga/      #   saga primitives
├── semver/    #   version parsing
└── version/   #   build-time version metadata

internal/     # Implementation, not importable outside the module
├── agent/    #   agent runtime + bootstrap + systemd installer
├── audit/    #   audit emitter + buffered / store / multi auditors
├── blueprint/#   blueprint engine
├── cli/      #   shared cobra root + per-binary CLI packages
├── cluster/  #   etcd-backed clustering (membership, leader, fencing)
├── config/   #   config loader + validation
├── controlplane/  # gRPC server wiring + command + state + event services
├── events/   #   event bus + JetStream publisher + SQL store
├── gitops/   #   verification + rollback + webhook receivers
├── identity/ #   CA + SVID + join tokens
├── nats/     #   NATS connection mgmt + JetStream setup
├── policy/   #   OPA + CEL evaluators + audit-mode runtime
├── runbook/  #   runbook engine + execution store
├── secrets/  #   broker + leases + transit + backends
├── statemgmt/#   state runner + saga + stdlib modules (35)
└── ...       #   plus health, logging, metrics, profiling, ratelimit, ...

cmd/          # Binary entry points (kscore-* + kscorectl)
api/proto/    # Protobuf sources (auto-generated Go in pkg/api/v1)
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

Use structured logging with `internal/logging`. Note that this is an internal package:

```go
import "go.keystone-core.io/keystone-core/internal/logging"

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
go test ./internal/state/...

# With coverage
go test -cover ./...

# With race detection
go test -race ./...

# E2E tests (requires Docker)
KSCORE_E2E_TESTS=1 make e2e-test

# Security checks (runs tools in Docker/Podman containers)
make security
```

For HA and IPv6 E2E runs, both server and agent configs use `nats.jetstream.storedir: /var/lib/keystone-core/jetstream`,
require the container filesystem to allow writes under `/var/lib/keystone-core`, and the servers set
`server.allowinsecurenonloopback: true` because TLS is disabled in test environments.

### VM E2E Testing

For VM-based E2E testing (all-in-one, HA, IPv6, performance), see
`docs/project/E2E-VM-TESTING.md`.

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

When adding features, update documentation. Until the Hugo site
lands (planned for v0.5), the canonical doc surfaces are:

- **Reference docs**: CLI / config / API changes go in
  `docs/project/{CLI,CONFIGURATION,API}-REFERENCE.md` — all three are
  auto-generated by `make docs-sync`; edit the source (cmd flag
  defs, config struct tags, `.proto` files) and regenerate.
- **Operator guides**: `docs/project/GETTING-STARTED.md` for the
  install-to-first-state walkthrough; `docs/runbooks/` for scenario-
  specific operational procedures.
- **Project / design**: `docs/project/DESIGN.md`,
  `docs/project/SECURITY-*.md`, plus the per-domain files under
  `docs/project/`.
- **Examples**: Add to the `examples/` directory.
- **Diagrams**: Use Mermaid syntax for all diagrams.

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
