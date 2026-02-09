---
title: "Compatibility & Support Policy"
weight: 6
description: >
  Release compatibility, support windows, upgrade paths, and technical guidelines for operators and developers
---

# Keystone Core — Compatibility & Upgrade Technical Guidelines

This document describes the technical rules, invariants, and implementation mechanics behind the
Keystone Core compatibility and upgrade model. The focus is on maintainability, correctness, and
operational safety.

## Core Design Principles

1. Upgrade Safety Comes First
2. Breaking Changes Are Allowed, Not Free
3. Compatibility Is Time-Bounded (Not Forever)
4. Configuration Should Be Forward-Compatible
5. Schema Changes Use Expand → Migrate → Contract
6. Controller May Lead Agents During Rolling Upgrades
7. Surface Area Freezes Over Time (Maturity Path)

## Versions & Release Lines

Each release line is tracked via SemVer:

MAJOR.MINOR.PATCH

With a 6-month release cadence, the support window spans 4 active release lines.

Developers must consider compatibility impacts across those lines.

## Compatibility Boundaries (Invariants)

Keystone enforces compatibility across three main surfaces:

1. RPC / Protocol Compatibility
2. Schema / Storage Compatibility
3. Configuration Compatibility

Breaking any of these surfaces requires a MAJOR release.

## Controller ↔ Agent Negotiation

On connect, the agent provides:

- agent_version (SemVer)
- supported_protocols (vector)
- schema_version
- capabilities

Controller chooses:

- highest compatible protocol
- compatibility mode
- or rejects with a clear diagnostic

Rules:

- Controller may be newer than agent
- Agent should not be significantly newer than controller
- Compatibility window = current release + previous 2 releases (adjustable)

Agents older than support window may be:

- rejected
- put in legacy/read-only mode
- or scheduled for upgrade

## Schema Migration Strategy

Schema upgrades use a three-phase model:

1. Expand
2. Migrate
3. Contract

Metadata stored in schema_meta with version and mode (legacy|mixed|new).

## State Migration Responsibility

Migration executes during software upgrade, not runtime.

Goals: deterministic, restart-safe, observable, operator-controllable.

Failures must be reversible and resumable.

## Configuration Compatibility

Configs must be forward-compatible.

Deprecated fields must warn for ≥2 releases.

Removal requires a MAJOR or end-of-support window.

## Breaking Changes (MAJOR)

Breaking changes include:

- protocol removal
- RPC removal
- schema removal
- config removal
- operator-impacting behavior shifts
- CLI/UX surface changes

A MAJOR bump requires migration notes and tooling.

## Release-Time Responsibilities

For each release, devs must provide:

- release notes
- migration notes
- schema diffs
- config deprecation diffs
- agent compatibility statement
- controller compatibility statement

## CI/Testing Considerations

CI must verify:

- rolling upgrade
- agent compatibility
- schema correctness (expand/migrate/contract)
- downgrade safety (where appropriate)
- config deprecation warnings

Test matrix:

current
current-1
current-2
current-3

## Maturity & Future Compatibility Promise

As the platform stabilizes, a Go-like compatibility promise may be adopted once surfaces mature.

## Version Compatibility Matrix

This matrix shows compatibility between components across supported versions.

### Control Plane ↔ Agent Compatibility

| Control Plane | Agent 0.1.x | Agent 0.2.x | Agent 0.3.x | Agent 0.4.x |
|---------------|-------------|-------------|-------------|-------------|
| 0.1.x | ✅ Full | ⚠️ Limited | ❌ Unsupported | ❌ Unsupported |
| 0.2.x | ✅ Full | ✅ Full | ⚠️ Limited | ❌ Unsupported |
| 0.3.x | ⚠️ Limited | ✅ Full | ✅ Full | ⚠️ Limited |
| 0.4.x | ❌ Unsupported | ⚠️ Limited | ✅ Full | ✅ Full |

**Legend**:
- ✅ **Full**: All features work, fully tested combination
- ⚠️ **Limited**: Core features work, some new features may be unavailable
- ❌ **Unsupported**: Not tested, may have protocol incompatibilities

**Recommendation**: Control plane should be same version or newer than agents.

### Database Backend Compatibility

| Version | SQLite | PostgreSQL 13 | PostgreSQL 14 | PostgreSQL 15 | PostgreSQL 16 |
|---------|--------|---------------|---------------|---------------|---------------|
| 0.1.x | ✅ | ✅ | ✅ | ✅ | ⚠️ |
| 0.2.x | ✅ | ✅ | ✅ | ✅ | ✅ |
| 0.3.x | ✅ | ✅ | ✅ | ✅ | ✅ |
| 0.4.x | ✅ | ⚠️ | ✅ | ✅ | ✅ |

**Notes**:
- SQLite recommended for deployments < 500 agents
- PostgreSQL 13 reaches EOL November 2025
- PostgreSQL 16 requires 0.2.x+ for full feature support

### NATS Server Compatibility

| Version | NATS 2.9.x | NATS 2.10.x | NATS 2.11.x |
|---------|------------|-------------|-------------|
| 0.1.x | ✅ | ✅ | ⚠️ |
| 0.2.x | ✅ | ✅ | ✅ |
| 0.3.x | ⚠️ | ✅ | ✅ |
| 0.4.x | ❌ | ✅ | ✅ |

**Notes**:
- JetStream required for all versions
- NATS 2.9.x deprecated in 0.4.x
- NATS 2.11.x recommended for new deployments

### etcd Compatibility (HA Clustering)

| Version | etcd 3.4.x | etcd 3.5.x |
|---------|------------|------------|
| 0.1.x | ✅ | ✅ |
| 0.2.x | ✅ | ✅ |
| 0.3.x | ⚠️ | ✅ |
| 0.4.x | ❌ | ✅ |

**Notes**:
- etcd 3.5.x recommended for all deployments
- etcd 3.4.x support removed in 0.4.x

### Operating System Compatibility

#### Control Plane

| Version | Ubuntu 22.04 | Ubuntu 24.04 | RHEL 8 | RHEL 9 | Debian 11 | Debian 12 |
|---------|--------------|--------------|--------|--------|-----------|-----------|
| 0.1.x | ✅ | ⚠️ | ✅ | ✅ | ✅ | ⚠️ |
| 0.2.x | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| 0.3.x | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| 0.4.x | ✅ | ✅ | ⚠️ | ✅ | ⚠️ | ✅ |

#### Agent

| Version | Ubuntu 20.04+ | RHEL 7+ | Debian 10+ | Windows Server 2019+ | macOS 12+ |
|---------|---------------|---------|------------|----------------------|-----------|
| 0.1.x | ✅ | ✅ | ✅ | ⚠️ | ⚠️ |
| 0.2.x | ✅ | ✅ | ✅ | ✅ | ⚠️ |
| 0.3.x | ✅ | ✅ | ✅ | ✅ | ✅ |
| 0.4.x | ✅ | ✅ | ✅ | ✅ | ✅ |

### Kubernetes Compatibility

| Version | K8s 1.26 | K8s 1.27 | K8s 1.28 | K8s 1.29 | K8s 1.30 |
|---------|----------|----------|----------|----------|----------|
| 0.1.x | ✅ | ✅ | ✅ | ⚠️ | ❌ |
| 0.2.x | ✅ | ✅ | ✅ | ✅ | ⚠️ |
| 0.3.x | ⚠️ | ✅ | ✅ | ✅ | ✅ |
| 0.4.x | ❌ | ⚠️ | ✅ | ✅ | ✅ |

**Notes**:
- Helm chart compatibility follows same matrix
- Operator supports N-3 Kubernetes versions
- CRD versions may differ; see Helm chart docs

### API Version Compatibility

| Version | REST API v1 | REST API v2 | gRPC v1 | gRPC v2 |
|---------|-------------|-------------|---------|---------|
| 0.1.x | ✅ | ❌ | ✅ | ❌ |
| 0.2.x | ✅ | ⚠️ beta | ✅ | ❌ |
| 0.3.x | ✅ | ✅ | ✅ | ⚠️ beta |
| 0.4.x | ⚠️ deprecated | ✅ | ⚠️ deprecated | ✅ |

**Migration Notes**:
- REST API v1 deprecated in 0.4.x, removed in 0.6.x
- gRPC v1 deprecated in 0.4.x, removed in 0.6.x
- Use API version header to specify desired version

### CLI Compatibility

| CLI Version | Control Plane 0.1.x | Control Plane 0.2.x | Control Plane 0.3.x | Control Plane 0.4.x |
|-------------|---------------------|---------------------|---------------------|---------------------|
| 0.1.x | ✅ | ⚠️ | ❌ | ❌ |
| 0.2.x | ✅ | ✅ | ⚠️ | ❌ |
| 0.3.x | ⚠️ | ✅ | ✅ | ⚠️ |
| 0.4.x | ❌ | ⚠️ | ✅ | ✅ |

**Recommendation**: Keep CLI version aligned with control plane version.

### SDK Compatibility

| SDK | Language | Supported Versions | Notes |
|-----|----------|-------------------|-------|
| `kscore-go` | Go | 0.2.x+ | Primary SDK |
| `kscore-python` | Python 3.9+ | 0.2.x+ | Community maintained |
| `kscore-typescript` | TypeScript/Node | 0.3.x+ | Beta |
| `kscore-rust` | Rust | 0.4.x+ | Alpha |

### Module Compatibility

| Module Version | Runtime 0.1.x | Runtime 0.2.x | Runtime 0.3.x | Runtime 0.4.x |
|----------------|---------------|---------------|---------------|---------------|
| Starlark v1 | ✅ | ✅ | ✅ | ✅ |
| Starlark v2 | ❌ | ⚠️ | ✅ | ✅ |
| WASM v1 | ❌ | ❌ | ⚠️ | ✅ |

### Terraform Provider Compatibility

| Provider Version | Keystone 0.2.x | Keystone 0.3.x | Keystone 0.4.x |
|------------------|----------------|----------------|----------------|
| 0.1.x | ✅ | ⚠️ | ❌ |
| 0.2.x | ✅ | ✅ | ⚠️ |
| 0.3.x | ⚠️ | ✅ | ✅ |

## Upgrade Path Matrix

### Direct Upgrade Support

| From → To | 0.1.x | 0.2.x | 0.3.x | 0.4.x |
|-----------|-------|-------|-------|-------|
| 0.1.x | — | ✅ | ⚠️ | ❌ |
| 0.2.x | ❌ | — | ✅ | ⚠️ |
| 0.3.x | ❌ | ❌ | — | ✅ |
| 0.4.x | ❌ | ❌ | ❌ | — |

**Legend**:
- ✅ Direct upgrade supported
- ⚠️ Requires intermediate version (step upgrade)
- ❌ Downgrade not supported

### Recommended Upgrade Paths

```
0.1.x → 0.2.x → 0.3.x → 0.4.x (sequential)
0.1.x → 0.2.x → 0.4.x (skip 0.3.x if needed)
0.2.x → 0.4.x (direct, requires migration script)
```

### Rolling Upgrade Support

| Upgrade | Rolling | Zero-Downtime | Notes |
|---------|---------|---------------|-------|
| 0.1.x → 0.2.x | ✅ | ⚠️ | Brief API interruption during schema migration |
| 0.2.x → 0.3.x | ✅ | ✅ | Full rolling upgrade support |
| 0.3.x → 0.4.x | ✅ | ✅ | Full rolling upgrade support |

## Deprecation Timeline

### Scheduled Deprecations

| Component | Deprecated In | Removed In | Replacement |
|-----------|---------------|------------|-------------|
| REST API v1 | 0.4.x | 0.6.x | REST API v2 |
| gRPC v1 | 0.4.x | 0.6.x | gRPC v2 |
| SQLite CGO | 0.3.x | 0.5.x | Pure Go SQLite |
| NATS 2.9.x | 0.3.x | 0.4.x | NATS 2.10.x+ |
| etcd 3.4.x | 0.3.x | 0.4.x | etcd 3.5.x |
| Python SDK 0.1.x | 0.4.x | 0.6.x | Python SDK 0.2.x |

### Configuration Deprecations

| Config Key | Deprecated In | Removed In | Replacement |
|------------|---------------|------------|-------------|
| `server.legacy_auth` | 0.2.x | 0.4.x | `server.auth.method` |
| `agent.nats_url` | 0.3.x | 0.5.x | `nats.url` |
| `database.sqlite_path` | 0.3.x | 0.5.x | `database.sqlite.path` |

## Support Windows

### Version Support Policy

| Version | Release Date | Active Support | Security Support | End of Life |
|---------|--------------|----------------|------------------|-------------|
| 0.1.x | 2026-01-28 | Active | 2027-01-28 | 2027-07-28 |
| 0.2.x | 2026-07 (planned) | — | — | — |
| 0.3.x | 2027-01 (planned) | — | — | — |
| 0.4.x | 2027-07 (planned) | — | — | — |

### Support Levels

- **Active Support**: Bug fixes, security patches, minor features
- **Security Support**: Critical security patches only
- **End of Life**: No further updates

## Checking Compatibility

### CLI Commands

```bash
# Check component versions (detailed output)
kscorectl version --verbose

# Output (JSON):
# {
#   "version": "0.1.0",
#   "commit": "abc123",
#   "buildDate": "2026-01-28",
#   "goVersion": "go1.25"
# }

# Check agent compatibility
kscorectl agents list --show-compatibility

# Output:
# AGENT          VERSION   COMPATIBLE   FEATURES
# agent-001      0.1.0     ✅ Full       All
# agent-002      0.1.0     ✅ Full       All
# agent-003      0.1.0     ✅ Full       All
```

### Programmatic Check

```go
import "github.com/shawnbutts/keystone-core/compatibility"

// Check version compatibility
result := compatibility.Check(
    compatibility.ControlPlane("0.4.1"),
    compatibility.Agent("0.3.5"),
)

if result.Status == compatibility.Limited {
    log.Warn("Limited compatibility", "missing", result.MissingFeatures)
}
```

## Design Goals

- avoid stagnation
- avoid legacy burden
- support ecosystems
- provide operational safety
- maintain internal design flexibility
- keep codebase refactorable
