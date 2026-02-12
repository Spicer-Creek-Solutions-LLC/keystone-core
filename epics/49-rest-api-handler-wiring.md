# Epic 49: REST API Handler Wiring

## Overview

Wire the 8 remaining REST API handler packages into `kscore-server` so that their HTTP routes are live and functional. Each handler already has a `RegisterRoutes()` method and tests, but they are not called in `cmd/kscore-server/main.go` because their dependency objects are not yet constructed in the server startup path.

**Goal**: All REST API handler packages registered in the server with real, functional dependencies — not nil stubs.

## Problem Statement

**Current State:**
- 7 handler packages are wired: agents, execution, state, maintenance, apikeys, config, rbac
- 8 handler packages have `RegisterRoutes()` but are NOT called:
  - `pkg/api/cluster` — needs `internal/cluster` objects (MembershipManager, LeaderElector, ShardManager, HealthMonitor, Config)
  - `pkg/api/events` — needs `events.EventStore` and `events.EventPublisher` interfaces
  - `pkg/api/webhooks` — needs `*webhook.Receiver` and `*webhook.HandlerRegistry`
  - `pkg/api/policy` — needs `*policy.PolicyEngine`, `*policy.Auditor`, `*policy.ComplianceReporter`
  - `pkg/api/gitops` — needs `*verification.Engine` and `*rollback.Engine`
  - `pkg/api/runbook` — needs `approval.Storage`, `intervention.Storage`, `*approval.Manager`, `*intervention.Manager`
  - `internal/files/mirror` — needs `*mirror.Registry` and `*mirror.SyncEngine`
  - `internal/proxy/discovery` — needs `*discovery.Discovery`
- Some dependency packages are already imported in kscore-server (events, webhook, policy) but their objects aren't constructed
- Other dependency packages (cluster, gitops/verification, gitops/rollback, files/mirror, proxy/discovery, runbook) need new initialization code

**Target State:**
- All 15 handler packages registered and functional in kscore-server
- Dependencies constructed from config or with sensible defaults
- Handlers that require optional infrastructure (etcd for cluster, NATS streams for events) gracefully degrade when infrastructure is unavailable
- Server startup logs which handlers are active vs degraded

## Success Criteria

- [ ] All 8 handler packages call `RegisterRoutes(httpMux)` in kscore-server
- [ ] Each handler has real dependencies, not nil stubs
- [ ] Handlers degrade gracefully if optional infrastructure (etcd, NATS JetStream) is unavailable
- [ ] Server startup logs handler registration status
- [ ] Integration tests verify routes are registered and return valid responses
- [ ] Tests with >70% coverage for new wiring code
- [ ] Documentation updated (configuration reference, API reference)

## Dependencies

- **Epic 1** (Core Infrastructure) — NATS, agents, control plane
- **Epic 4** (Event System) — event store and publisher
- **Epic 5** (GitOps Integration) — verification and rollback engines
- **Epic 6** (Policy Enforcement) — policy engine, auditor, reporter
- **Epic 11** (Clustering) — cluster membership, leader election, sharding
- **Epic 21** (Proxy Agents) — discovery engine
- **Epic 22** (File Distribution) — mirror registry and sync
- **Epic 37** (Enhanced Runbooks) — approval and intervention storage/managers

## Technical Design

### Phase 1: Event and Policy Handlers (Week 1-2)

These handlers have the simplest dependency chains — `events` and `policy` packages are already imported in kscore-server.

**Task 1.1: Wire events handler**
- Construct `events.EventStore` — use NATS JetStream-backed store if JetStream is available, otherwise use in-memory store
- Construct `events.EventPublisher` — use NATS publisher if connected, otherwise use no-op publisher
- Register `apievents.NewHandler(store, publisher).RegisterRoutes(httpMux)`
- Files: `cmd/kscore-server/main.go`

**Task 1.2: Wire policy handler**
- Construct `policy.PolicyEngine` with default config
- Construct `policy.Auditor` (may use event publisher from T1.1)
- Construct `policy.ComplianceReporter`
- Register `apipolicy.NewHandler(engine, auditor, reporter).RegisterRoutes(httpMux)`
- Files: `cmd/kscore-server/main.go`

**Task 1.3: Wire webhooks handler**
- Construct `webhook.HandlerRegistry` (already imported)
- Construct `webhook.Receiver` using the registry
- Register `apiwebhooks.NewHandler(receiver, registry).RegisterRoutes(httpMux)`
- Files: `cmd/kscore-server/main.go`

### Phase 2: Runbook and GitOps Handlers (Week 3-4)

**Task 2.1: Wire runbook handler**
- Construct `approval.Storage` — use SQLite-backed storage (reuse server's DB path from config)
- Construct `intervention.Storage` — same
- Construct `approval.Manager` and `intervention.Manager` from their storages
- Register `apirunbook.NewHandler(approvalStore, interventionStore, approvalMgr, interventionMgr).RegisterRoutes(httpMux)`
- Files: `cmd/kscore-server/main.go`

**Task 2.2: Wire gitops handler**
- Construct `verification.Engine` with default config
- Construct `rollback.Engine` with default config
- Register `apigitops.NewHandler(verificationEngine, rollbackEngine).RegisterRoutes(httpMux)`
- Files: `cmd/kscore-server/main.go`

### Phase 3: Infrastructure-Dependent Handlers (Week 5-6)

These handlers need infrastructure that may not be available in all deployments.

**Task 3.1: Wire cluster handler**
- Only activate when clustering is enabled in config (`cfg.Cluster.Enabled`)
- Construct cluster objects from etcd coordinator (already started in kscore-server when clustering is enabled)
- If clustering is disabled, log a message and skip registration
- Register conditionally: `apicluster.NewHandler(membership, leader, sharding, health, clusterCfg).RegisterRoutes(httpMux)`
- Files: `cmd/kscore-server/main.go`

**Task 3.2: Wire mirror handler**
- Construct `mirror.Registry` and `mirror.SyncEngine` from file distribution config
- If file distribution is not configured, skip registration
- Register: `mirror.NewAPIHandler(registry, syncEngine).RegisterRoutes(httpMux)`
- Files: `cmd/kscore-server/main.go`

**Task 3.3: Wire discovery handler**
- Construct `discovery.Discovery` from proxy config
- If proxy/discovery is not configured, skip registration
- Register: `discovery.NewAPI(discoveryEngine).RegisterRoutes(httpMux)`
- Files: `cmd/kscore-server/main.go`

### Phase 4: Testing and Documentation (Week 7-8)

**Task 4.1: Integration tests**
- Test that all handlers are registered and respond to requests
- Test conditional registration (cluster disabled, file distribution disabled)
- Test graceful degradation when infrastructure is unavailable

**Task 4.2: Documentation**
- Update API reference with all available endpoints
- Update configuration reference for handler-related config sections
- Document which handlers require which infrastructure

## Handler Dependency Summary

| Handler | Package | Constructor Params | Dep Source |
|---------|---------|-------------------|------------|
| Events | `pkg/api/events` | `EventStore`, `EventPublisher` (interfaces) | `internal/events` |
| Policy | `pkg/api/policy` | `*PolicyEngine`, `*Auditor`, `*ComplianceReporter` | `internal/policy` |
| Webhooks | `pkg/api/webhooks` | `*Receiver`, `*HandlerRegistry` | `internal/gitops/webhook` |
| Runbook | `pkg/api/runbook` | `Storage` x2, `*Manager` x2 | `internal/runbook/{approval,intervention}` |
| GitOps | `pkg/api/gitops` | `*verification.Engine`, `*rollback.Engine` | `internal/gitops/{verification,rollback}` |
| Cluster | `pkg/api/cluster` | 5 cluster objects | `internal/cluster` |
| Mirror | `internal/files/mirror` | `*Registry`, `*SyncEngine` | `internal/files/mirror` |
| Discovery | `internal/proxy/discovery` | `*Discovery` | `internal/proxy/discovery` |

## Risks and Mitigations

| Risk | Mitigation |
|------|-----------|
| Constructing real dependencies may panic if config is missing | Use sensible defaults; only construct when relevant config section is present |
| Some dependencies need NATS/etcd which may not be running | Conditional registration with clear logging |
| Handler initialization may slow down server startup | Initialize in parallel where possible; keep init lightweight |
| Circular dependencies between server components | Keep handler construction after all infrastructure is initialized |

## Testing Strategy

- **Unit tests**: Each handler's existing test suite already covers route registration
- **Integration tests**: New tests in `cmd/kscore-server/` verifying all routes respond
- **Conditional tests**: Verify handlers skip registration gracefully when config is absent
- **Startup tests**: Verify server starts successfully with all handlers registered

## Definition of Done

- [ ] All 8 handler packages registered in kscore-server
- [ ] Real dependencies constructed (not nil stubs)
- [ ] Conditional registration for infrastructure-dependent handlers
- [ ] Server logs handler registration status
- [ ] Integration tests passing
- [ ] Documentation updated
- [ ] All existing tests still passing
