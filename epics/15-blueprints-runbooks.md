# Epic 15: Blueprints + Runbooks + Saga + StateMachine Library

**Phase**: J • **Estimate**: 2 weeks • **Depends on**: 02, 03, 08, 11, 14 • **Blocks**: 16 (rollback uses StateMachine)

## Goal

Two related composability layers shipped together: pre-packaged state collections (blueprints, Salt-formula-shaped) and trigger-based workflow automation (runbooks). Plus the cross-cutting Saga coordinator and StateMachine library used by rollback, promotion, schedule, and runbook engines.

## Scope (in)

### Blueprints (`internal/blueprint/`)

- `Manifest{Metadata, Compatibility, Dependencies (requires, requires_before), Features, Entrypoints (default, rollback, named), Parameters (JSON-Schema), Outputs, Hooks (pre_apply, post_apply, pre_rollback, post_rollback), SourcePath}`.
- Loader → parses `blueprint.yaml`; validates manifest.
- Parameter validation against JSON-Schema; coercion (string→int/bool); `sensitive: true, source: secret` triggers credential lookup via SecretBroker (Epic 10).
- Dependency resolver builds DAG; cycle detection; soft (`requires_before`) vs hard (`requires`) edges.
- Feature flag evaluation (`features:` block; conditional state inclusion).
- Multi-instance: `as: <namespace>` deploys same blueprint twice with namespaced state names.
- Template rendering with parameter context.
- Apply: invokes State Runner (Epic 08) with the rendered state collection.
- Hooks run as runbooks (pre_apply / post_apply / pre_rollback / post_rollback).

### v1.0 standard catalog (6 blueprints) in `modules/examples/blueprints/`

- `demo` — single-node demo deployment.
- `production-cluster` — 3-node HA control plane with embedded etcd, Postgres, NATS cluster.
- `monitoring-stack` — Prometheus + Grafana + Loki.
- `security-baseline` — CIS-aligned host hardening.
- `postgres-ha` — Postgres + WAL replication + monitoring.
- `nats-cluster` — NATS cluster with JetStream.

### Runbooks (`internal/runbook/`)

- `Runbook{Metadata{Name, Namespace, Version, Labels, Annotations}, Spec{Inputs, Steps, OnSuccess, OnFailure, Timeout, MaxRetries}}`.
- `Step{Type, Name, Description, DependsOn, Condition, Timeout, Retries, Config}`.
- **v1.0 step types** (~9): `command`, `api`, `state`, `notification`, `wait`, `noop`, `fail`, `script`, `query`.
- Variable templating: `{{ steps.<name>.outputs.<field> }}`.
- Step dependency DAG; cycle detection; conditional pre-execution.
- Retries with exp backoff per step.
- Audit trail per execution.

### Saga coordinator (`pkg/saga/`)

- `Step{Name, Action func(ctx, data) → (data, error), Compensate func(ctx, data) → error}`.
- `Execution{ID, Name, Status, Data, Steps[], StartedAt, EndedAt, Error}`.
- v1.0: forward-execute steps; on first error walk completed steps in reverse invoking `Compensate`.
- In-memory or SQLite log (`log_memory.go`, `log_sqlite.go`).

### StateMachine library (`pkg/statemachine/`)

- `Machine[S, E]{Builder, States, Transitions, Guards, Callbacks (OnEnter, OnExit, OnTransition), History, Metrics, Checkpointer (optional)}`.
- Used internally by: runbook executor, Epic 16 rollback engine, Epic 13 cluster lifecycle, future Epic 16 promotion engine.

### APIs

- `kscore-blueprint` CLI: `init`, `validate`, `lint`, `info`, `install`, `apply`, `update`, `remove`, `applied`, `rollback`, `bundle`.
- `kscore-runbook` CLI: `list`, `execute`, `status`, `list-executions`, `audit`, `test`.
- REST: `GET/POST /api/v1/runbooks`, `GET /api/v1/executions/{id}`, `GET /api/v1/blueprints`, `POST /api/v1/blueprints/{name}/apply`.
- gRPC: minimal Blueprint/Runbook service (definition in Epic 03 protos; v1.0 ships query + execute).

## Scope (out / non-goals)

- Schedule + maintenance windows — v1.1.
- Runbook conditional steps (`if`, `switch`, `loop`, `parallel`, `sub-runbook`) — v1.2.
- Per-step approvals + delegations + interventions (`prompt`, `wait-manual`, `confirm`, `rollback` step types) — v1.3.
- Runbook dry-run mode — v1.2.
- Saga checkpoint resume — v1.4.
- Standard catalog expansion to 14 blueprints — v1.4.
- Blueprint signing + signed bundles — v1.5.
- Blueprint mirror for air-gap — v1.5.

## Design summary

See `PROJECT-DETAILS.md §4.17`.

## Tasks

> **Execution order**: 1, 2, 3, 4, 7, 8, 9, 5, 6, 10, 11, 12, 13. The blueprint executor (5) runs blueprint hooks *as runbooks*, so the runbook engine (7–9) is built first and the executor wires a real runbook-backed hook runner instead of a stub. The numbered list below is the dependency-grouped plan, not the execution sequence.

1. **`pkg/statemachine`** — generic FSM implementation with all listed features + tests.
2. **`pkg/saga`** — Step + Execution + log interface + in-memory + SQLite impls + tests.
3. **`internal/blueprint/`** — Manifest types, loader, validator, parameter validation w/ JSON-Schema, dependency resolver w/ cycle detection.
4. **Feature flag eval + template render**.
5. **Blueprint executor** — coordinates with State Runner + hooks (which are runbooks).
6. **6-blueprint catalog** in `modules/examples/blueprints/` with full content.
7. **`internal/runbook/`** — Runbook + Step types; runner with dependency DAG; variable templating; retry logic.
8. **9 v1.0 step types** with each having tests for normal + error paths.
9. **Audit + event integration** — runbook lifecycle + step transitions emit events.
10. **`cmd/kscore-blueprint`** + **`cmd/kscore-runbook`** CLIs.
11. **REST handlers** in `pkg/api/runbook/` + new `pkg/api/blueprint/`.
12. **gRPC services** for blueprint + runbook (minimal — query, execute, status).
13. **Integration test**: apply `production-cluster` blueprint end-to-end on docker-compose; verify all states applied + hooks ran.

## Acceptance criteria

- [ ] `kscorectl blueprint apply demo --target id:agent-1` deploys demo blueprint successfully.
- [ ] `kscorectl blueprint apply production-cluster --param postgres_password=secret://kv/db --param cluster_name=test` substitutes secret + applies.
- [ ] Blueprint dependency cycle detected with cycle path in error.
- [ ] `kscorectl blueprint rollback <run-id>` reverts to prior state.
- [ ] `kscorectl runbook execute db-restart.yaml --input agent_id=x` executes 5-step runbook with audit trail.
- [ ] Runbook step failure triggers `onFailure` chain.
- [ ] Variable templating: step 2 references step 1 output successfully.
- [ ] Saga compensation runs on multi-step failure (steps 1-3 succeed, step 4 fails → 3,2,1 compensate in reverse).
- [ ] Coverage >80% on `internal/blueprint`, `internal/runbook`, `pkg/saga`, `pkg/statemachine`.

## Risks

- **Blueprint dependency cycles** — detect at resolve time; fail loud.
- **Param coercion edge cases** — invalid input must surface clearly, not silently coerce to zero value.
- **Sensitive params** — never log; mask in audit.
- **Runbook variable scope** — explicit reference (`{{ steps.X.outputs.Y }}`); silent variables don't cross steps.
- **Multi-instance namespacing** — collision detection between namespaced and unnamespaced state names.
- **Saga compensation ordering** — reverse of completion order; failure of compensation = aggregate-and-continue (not abort).
- **Catalog content quality** — 6 blueprints set the bar for the entire catalog. Treat as documentation; review carefully.

## References

- PROJECT-DETAILS §4.17.
