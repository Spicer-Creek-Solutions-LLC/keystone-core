# Bootstrap Test Infrastructure

This directory scaffolds the Epic 29 bootstrap test infrastructure. It will host Docker-based
CI scenarios and optional VM-based validation runs.

## Layout

- `framework/` shared test harness utilities
- `containers/` Dockerfiles and docker-compose definitions
- `scenarios/` scenario-driven tests (demo, production, cluster, join, blueprints)
- `platforms/` platform-specific validations
- `vm/` VM-based runners and provider adapters

## Config

Use `test/bootstrap/config.yaml` to define platforms and scenarios. The initial config mirrors
Epic 29 expectations and can be customized per environment.

## Running Docker-Based Tests

Bootstrap scenarios and platform checks are gated behind `KSCORE_BOOTSTRAP_TESTS=1`. These tests
run in dry-run mode and expect a locally built `kscore-agent` binary.

```
make agent
cd test/bootstrap
make test-docker
```

Optional environment variables:

- `KSCORE_TEST_PLATFORM` to filter to a single platform (e.g., `ubuntu-22.04`)
- `KSCORE_SKIP_BUILD=1` to skip container image rebuilds
- `KSCORE_BOOTSTRAP_AGENT_BIN` to point at a custom agent binary path

## Running VM-Based Tests

VM tests require `KSCORE_VM_TESTS=1` and a VM config file. The default sample config lives in
`test/bootstrap/vm/config.yaml` and should be replaced with real VM endpoints.

```
KSCORE_VM_TESTS=1 KSCORE_VM_CONFIG=test/bootstrap/vm/config.yaml \
  go test ./test/bootstrap/vm/... -timeout 2h
```

## Status

Phase 4+ scaffolding implemented. Docker-based dry-run scenarios and platform checks are ready,
VM scaffolding is in place, and CI workflow is configured.
