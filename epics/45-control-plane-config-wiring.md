# Epic 45: Control Plane Configuration Wiring

## Overview

Wire documented control plane configuration sections into `internal/config.Config` so that `kscore-server` reads and applies these settings at startup. Currently, the following configuration blocks are documented but not parsed or used by the control plane:

- **Agent Management** (`agents:`): heartbeat intervals, timeouts, metadata refresh, concurrent command limits
- **Command Execution** (`execution:`): default/max timeouts, batch size/delay, streaming buffer, result retention
- **State Management** (`state:`): apply timeout, concurrency, drift check interval, result retention
- **Event System** (`events:`): storage retention, publisher/subscriber buffer sizes, ack timeouts
- **GitOps Integration** (`gitops:`): webhook settings, git-sync repositories and schedules
- **Security** (`security:`): authentication type, API keys, mTLS, authorization/RBAC

## Goal

Make the control plane configurable via these settings so operators can tune behavior without code changes.

## Success Criteria

- [ ] All 6 config sections parsed into `internal/config.Config` struct fields
- [ ] `kscore-server` reads and applies each setting
- [ ] Viper bindings and environment variable overrides work for all new fields
- [ ] Defaults are set for all fields (zero-config still works)
- [ ] `kscorectl config validate` validates the new sections
- [ ] Configuration reference documentation matches the struct fields exactly
- [ ] Unit tests for config parsing, defaults, and validation

## Technical Tasks

### Phase 1: Config Struct Definitions (Week 1)

**T1.1: Agent Management Config**
- Add `AgentManagement` field to `Config` struct
- Fields: `HeartbeatInterval`, `HeartbeatTimeout`, `MetadataRefresh`, `MaxConcurrentCommands`
- Wire into agent registration and heartbeat monitoring in `kscore-server`

**T1.2: Execution Config**
- Add `Execution` field to `Config` struct
- Fields: `DefaultTimeout`, `MaxTimeout`, `BatchSize`, `BatchDelay`, `StreamingBuffer`, `ResultRetention`
- Wire into command dispatch in `kscore-server`

**T1.3: State Management Config**
- Add `StateManagement` field to `Config` struct
- Fields: `DefaultTimeout`, `MaxConcurrent`, `DriftCheckInterval`, `ResultRetention`
- Wire into state apply and drift check scheduling

### Phase 2: Event and GitOps Config (Week 2)

**T2.1: Event System Config**
- Add `Events` field to `Config` struct
- Fields: `Storage` (enabled, retention), `Publisher` (buffer/batch), `Subscriber` (buffer, ack_wait)
- Wire into event system initialization in `kscore-server`

**T2.2: GitOps Config**
- Add `GitOps` field to `Config` struct
- Fields: `Webhooks` (enabled, listen, path, auth), `GitSync` (enabled, repositories)
- Wire into gitops webhook receiver and git-sync scheduler
- Note: `WebhookConfig` already exists for the inbound receiver; this adds outbound/git-sync config

**T2.3: Security Config**
- Add `Security` field to `Config` struct
- Fields: `Authentication` (type, api_keys, mtls), `Authorization` (enabled, default_deny, rbac)
- Wire into auth middleware initialization
- Note: `AuthConfig` already exists for basic auth; this extends with RBAC and mTLS settings

### Phase 3: Wiring and Validation (Week 3)

**T3.1: Viper Bindings**
- Register environment variable bindings for all new fields
- Add config aliases for common patterns
- Set sensible defaults matching current behavior

**T3.2: Config Validation**
- Add validation for new fields in `kscorectl config validate`
- Validate duration formats, positive integers, enum values
- Warn on conflicting settings

**T3.3: Documentation Update**
- Ensure configuration.md matches struct fields exactly
- Add inline comments with defaults
- Update environment variable reference

## Dependencies

- **Epic 1**: Core Infrastructure (NATS, agents, control plane)
- **Internal packages**: `internal/config`, `cmd/kscore-server`

## Testing Strategy

- Unit tests for config parsing with all new sections
- Unit tests for default values
- Unit tests for validation (invalid durations, out-of-range values)
- Integration test: `kscore-server` starts with full config file

## Definition of Done

- [ ] All 6 config sections wired into Config struct
- [ ] kscore-server reads and applies all settings
- [ ] Environment variable overrides work
- [ ] Config validation covers new fields
- [ ] Documentation matches implementation
- [ ] Tests passing with race detector
