# Epic 12: Audit Log + Policy Engine (Audit-Mode-Only)

**Phase**: G • **Estimate**: 2 weeks • **Depends on**: 02, 03, 11 • **Blocks**: 14 (capability policy hooks)

## Goal

Two related concerns shipped together: full audit log of all sensitive ops (v1.0) AND policy engine in **audit-mode-only** (v1.0; full enforcement v1.8). v1.0 ships the full engine + audit + reporting; the `Enforcer` is wired but always returns Allowed=true regardless of evaluation. v1.8 simply flips `policy.enforcement_enabled=true`.

## Scope (in)

### Audit log (full v1.0)

- `internal/audit/` — emitter; structured events for auth decisions, secret access, command exec, state apply, policy evaluations.
- `internal/policy/audit.go` — `Auditor` (in-memory circular buffer; configurable size).
- `AuditStore` interface + `SQLitePolicyAuditStore` (WAL; indexed on `policy_id`, `actor`, `resource_type`, `timestamp`, `severity`, `allowed`). Extends `internal/state.Store`.
- `AuditEntry{ID, Timestamp, PolicyID, PolicyName, PolicyType, ResourceType, Allowed, Duration, Violations[], EnforcementMode, User, Action, Metadata}`.
- `AuditFilter{PolicyID, Allowed, Severity, ResourceType, User, Action, StartTime, EndTime, Limit}` — pagination.
- `AuditSummary{TotalEvaluations, AllowedCount, DeniedCount, ViolationsByPolicy, ViolationsBySeverity, TimeRange}`.
- `AuditRetentionPolicy{MaxAge default 90d, MaxCount default 100k, MinSeverity, RetentionInterval default 1h}`.
- `AuditRedactionConfig{RedactMetadataKeys, RedactPatterns regex[], RedactUser bool}` — applied on export.
- Audit export: JSON, JSONL, CSV via `kscore-audit` CLI.

### Policy engine (full v1.0; enforcement gated)

- `internal/policy/` — `Engine{registry, opaEvaluator, celEvaluator, builtinEvaluator}`.
- `Policy{ID, Name, Type (OPA|CEL|Builtin), Category (Security|Compliance|Operational|Cost|Custom), Severity (Low|Medium|High|Critical), EnforcementMode (Audit|Warn|Enforce), Code, Enabled, Tags, Metadata, CreatedAt, UpdatedAt}`.
- `PolicySet`, `Bindings` (resource-type → policy/set with optional action/selector).
- **Evaluators**:
  - `OPAEvaluator` via `open-policy-agent/opa/rego`; query `data.<package>.allow`; extract violations from `violations` binding (best-effort).
  - `CELEvaluator` via `google/cel-go`; vars `input`, `resource`, `action`, `user`, `context`.
  - `BuiltinEvaluator` — hardcoded rules: `require-labels`, `require-owner`, `allowed-environments`, `allowed-actions`, `deny-privileged`, `allowed-users`, `time-window`, `no-root-execution`, `require-approval`, `max-concurrent`, `resource-quota`, `pattern-allow`, `pattern-deny`. Config via JSON in policy `Code`.
- `EvaluationInput{Resource, Action, User, Context, Timestamp}`.
- `EvaluationResult{PolicyID, PolicyName, Allowed, Violations[], Warnings[], Message, Duration, EvaluatedAt}`.
- `Violation{Rule, Message, Severity, Path, Expected, Actual, Remediation}`.
- `Engine.Evaluate(ctx, policyID, input)` / `EvaluatePolicySet(ctx, setID, input)` / `EvaluateForResource(ctx, resourceType, input)`.
- **v1.0 enforcement gate**: `Enforcer` exists but **always returns `Allowed=true` regardless of evaluation result**. Config `policy.enforcement_enabled=false` (effectively hardcoded false).
- `ComplianceReport{Period, ComplianceRate, TotalEvaluations, CompliantEvaluations, NonCompliantEvaluations, PolicyStats[], TopViolations[], ViolationsBySeverity, Trend[]}`.
- `ComplianceControl{ID, Framework (CIS|SOC2|NIST-800-53|HIPAA|PCI-DSS|GDPR|ISO-27001|Custom), Title, Severity, PolicyIDs[]}`.
- `ControlMapping` — 2-way framework↔policies lookup.

### APIs

- gRPC `PolicyService` (Epic 03 protos): `EvaluatePolicy`, `EvaluatePolicySet`, `ListPolicies`, `GetPolicy`, `CreatePolicy` *(Unimplemented in v1.0; v1.8)*, `UpdatePolicy` *(v1.8)*, `DeletePolicy` *(v1.8)*, `ListViolations`, `GetComplianceReport`, `GetAuditLog`, `ListPolicySets`, `GetPolicySet`.
- REST: `/api/v1/policies` (list, evaluate, violations, compliance, audit-log).
- v1.0 CLI subset:
  - `kscore-policy list|validate|check|show|eval|test|compliance|violations`. (`create|update|delete|activate|deactivate|remediate|monitor` are v1.8.)
  - `kscore-audit log|report|export|stats|search|analyze|timeline|watch`.

## Scope (out / non-goals)

- **Enforcement modes Enforce + Warn (active blocking)** — v1.8.
- **Enforcement actions** (Block, Warn, Audit, Remediate) — v1.8.
- **Pre/post-execution hooks** — v1.8.
- **Approval workflows for policy violations** — v1.8.
- **Full policy CRUD via API** — v1.8.
- **Policy persistence** (etcd or Postgres dynamic reload) — v1.8.
- **Continuous compliance scan scheduler** — v1.5.
- **CEL custom function library** — v1.5.
- **Anomaly detection** — v1.4.

## Design summary

See `PROJECT-DETAILS.md §4.12`.

## Tasks

1. **`Auditor` (in-memory circular buffer)** + tests.
   _(landed: new `internal/audit/` package opens Epic 12 with the foundational value types + ring-buffer auditor that later tasks layer atop. **Value types** in `entry.go`: `AuditEntry` per §4.12 + **`Severity` field added** to support task 2's SQL index (spec listed Severity in AuditFilter + the index but omitted it from AuditEntry — editorial oversight); `Violation` (per-rule denial detail with own Severity); `Severity` ordered enum `Low < Medium < High < Critical` with `String` / `IsValid` / `AtLeast` / `Parse` / `MarshalText` / `UnmarshalText` / `AllSeverities()`; `EnforcementMode` enum `Audit / Warn / Enforce` with the same enum surface (v1.0 records the value on every entry but the task-10 Enforcer stub ignores it; v1.8 honors it); `PolicyType` enum `OPA / CEL / Builtin` (empty for non-policy entries — task 4's auth/secrets/exec/state-apply hooks). `NewAuditEntry(in AuditEntryInput) (AuditEntry, error)` stamps UUIDv7 `ID` (k-sortable, matches Epic 11 `Event.ID` precedent for task 2's SQL index locality) + UTC Timestamp + defaults Severity to Low + defaults EnforcementMode to Audit; rejects empty Action + unknown PolicyType. `MustNewAuditEntry` test sibling; `IsZero` + `Validate`. **`Auditor` interface** in `auditor.go` (`Emit(ctx, AuditEntry)` — **no error return** per Epic 10/11 precedent "fire and forget; never error back to caller"; strict-fail variant remains on v1.x ROADMAP entry landed during Epic 11 task 10). `NoopAuditor` default. Sentinel `ErrInvalidAuditEntry` (validation family) + `ErrAuditBufferUnusable` (config family). **`BufferedAuditor`** in `buffered.go` — FIFO ring buffer mirroring Epic 10's `secrets.BufferedAuditor` shape: capacity > 0 required (rejected with `ErrAuditBufferUnusable`); `Emit` appends + evicts oldest on overflow; `Snapshot()` returns defensive oldest-first copy (correctly handles ring-wrap via `start = next` when `count == capacity`); mutex-guarded for concurrent Emit + Snapshot; `Len()` + `Capacity()` accessors. **`MultiAuditor`** in `multi.go` fan-out across N inner auditors in registration order; nil entries dropped at construction (dense inner slice); empty `MultiAuditor()` doesn't panic; mirrors `secrets.MultiAuditor` exactly. **Tests** (98.4% coverage): `entry_test.go` covers Severity / EnforcementMode / PolicyType enum surfaces (String / IsValid / Parse / Marshal/Unmarshal round-trip / aliases), `NewAuditEntry` defaults + required-field validation + preserves provided fields, IsZero + Validate table (7 mutation cases), JSON round-trip pinning canonical field names, **UUIDv7 k-sortability** locked by 30-entry sort test mirroring Epic 11 task 1. `buffered_test.go` covers capacity rejection (0 + negative), empty Snapshot returns nil, emit-below-capacity preserves order, **FIFO eviction at capacity** with cap+2 emit verifying oldest 2 dropped, exactly-at-capacity no-eviction, defensive-snapshot (mutating result doesn't affect future calls), **N=50 concurrent emitters + 10 concurrent snapshotters under -race**, **ring-wrap regression** (after 5 emits into a cap-3 ring, Snapshot must walk from `next` not from index 0). `multi_test.go` covers fan-out to N inner + nil-skipped + empty no-panic + all-nil collapses + registration-order preservation via `funcAuditor` shim. Project `make lint` + `make test` + `make docs-lint` + `-race` clean.)_
2. **`AuditStore` interface + `SQLitePolicyAuditStore`** + retention + redaction. Extend `internal/state.Store`.
   _(landed: dual-layer SQL audit store mirroring Epic 11's events split. **Low-level** `internal/state.AuditStore` sub-interface (`CreateAuditEntry` / `CreateAuditEntriesBatch` / `GetAuditEntry` / `ListAuditEntries` / `CountAuditEntries` / `DeleteAuditEntry` / `ApplyAuditRetention`) added to `state.Store` composite; `AuditEntryStoreRecord` is the DB-shape value (Violations as JSON bytes, severity/enforcement_mode/policy_type as canonical lowercase strings, Metadata as `map[string]string`); `AuditEntryFilter` carries PolicyID/User/ResourceType/Action/Severities-IN/Allowed-pointer/Since/Until/Cursor/Limit/Descending; `AuditRetentionPolicy` carries MaxAge/MaxCount/MinSeverity. SQLite + Postgres backends both ship — Postgres uses TIMESTAMPTZ + JSONB; SQLite uses ISO-8601 text timestamps via `tsArgRequired` (matches commands/events precedent); schema indexes on policy_id, "user" (SQL reserved → quoted), resource_type, timestamp DESC, severity, allowed; PRIMARY KEY collision surfaces as `state.ErrDuplicate`; batch is all-or-nothing (SQLite tx; Postgres single multi-row VALUES); cursor pagination keyed on UUIDv7 ID; **MinSeverity exemption**: entries at-or-above threshold are NEVER deleted by retention (compliance-driven). **Consumer-facing** `internal/audit.AuditStore` typed wrapper in `sql_store.go` translates `AuditEntry` ↔ `AuditEntryStoreRecord` (JSON marshal of Violations + canonical enum stringification); `AuditQuery` (typed `MinSeverity Severity`) → `AuditEntryFilter` via `severitiesAtLeast`; `Validate()` rejects Limit < 0 and Since > Until; `AuditPage.NextCursor` set only when page is full; `Close()` is a no-op (SQL pool owned by composite); defaults — `DefaultQueryLimit=100`, `DefaultRetentionMaxAge=90d`, `DefaultRetentionMaxCount=100_000`, `DefaultRetentionInterval=1h`, `DefaultRetentionJitter=0.1`. **`RetentionConfig` + `RetentionEnforcer` scheduler** in `retention.go` mirrors `events.RetentionEnforcer` (Start/Stop/RunOnce + atomic LastRunAt/LastRunDeleted/TotalDeleted/RunsFailed metrics + first-tick-after-interval to avoid boot storm + `LeaderCheck` seam for Epic 13 raft + gosec-G118-clean publisher-owned cancelable worker context); single policy (vs events' list), so `WithRetentionPolicy(RetentionPolicy)` is singular. **Redaction** in `redaction.go`: `RedactionConfigInput` (with `[]string` patterns) → `NewRedactionConfig` compiles regex at construction and rejects malformed; `Apply(AuditEntry) AuditEntry` deep-copies and drops metadata keys, regex-replaces metadata values + violation messages, optionally blanks `User`; default replacement `"***"`; `IsNoop()` short-circuits with nil-receiver safety. **Tests** (audit 95.7% / state 44.1% — state ratio is whole-package, audit-only ratio is well above 70%): `state/sqlite_audit_test.go` CRUD round-trip + duplicate + 7-row validation table + batch all-or-nothing + pre-tx validation guard + filter table (PolicyID/User/ResourceType/Action/Severities IN/Allowed pointer/time range) + cursor forward + descending pagination + Count ignores cursor/limit + Delete + retention MaxAge + MaxCount + **MinSeverity-exempt-from-deletion** (critical entries survive 25h-old + MaxAge=24h when MinSeverity=high); `state/postgres_audit_integration_test.go` (`//go:build integration`) mirrors the set; `audit/sql_store_test.go` end-to-end round-trip + validation + batch all-or-nothing + MinSeverity filter + default-limit + 3-page cursor pagination + RejectsInvalid + Count + Delete + ApplyRetention + Close idempotent; `audit/redaction_test.go` invalid-regex rejected + empty/nil receiver no-op + default replacement + RedactUser blanks + metadata-key drop + pattern replace (with input untouched) + custom replacement + all-redactors-compose + deep-copies-violations + timestamp preserved; `audit/retention_test.go` requires-store + defaults applied + invalid-values fallback + RunOnce happy + store-error recorded + leader-check-false skips + policy passed through + first-tick-after-interval (no boot storm) + ticks-repeatedly + errors-don't-stop-scheduler + Stop-is-idempotent + double-Start-rejected + concurrent RunOnce atomic + nextWait jitter ±20% bound + zero-jitter exact + String-format. `make lint` + `make test` (race) + `make docs-lint` clean.)_
3. **`AuditFilter` query builder** + pagination tests.
4. **Audit emission hooks** wired into: auth (Epic 03), secrets (Epic 10), state apply (Epic 08), command exec (Epic 07), policy evaluation (this epic). **Hard rule**: every sensitive op MUST emit an audit entry. CI test that exercises each path and verifies entry present.
5. **`Engine` + `Registry`** in-memory; `RegisterPolicy`, `RegisterPolicySet`, `RegisterBinding`.
6. **`OPAEvaluator`** wrapping `opa/rego`.
7. **`CELEvaluator`** wrapping `cel-go` with v1.0 var schema.
8. **`BuiltinEvaluator`** — implement the 13 built-in rules.
9. **`Engine.Evaluate` / `EvaluatePolicySet` / `EvaluateForResource`** + tests covering all evaluator types.
10. **`Enforcer` stub** — receives `EvaluationResult`; v1.0 always returns `Allowed=true`; v1.8 will honor `EnforcementMode`.
11. **`ComplianceReport` generator** + framework mappings.
12. **gRPC `PolicyService` server** with Unimplemented for v1.8-gated CRUD.
13. **REST handlers** for v1.0 endpoints.
14. **`kscore-policy` v1.0 CLI** + **`kscore-audit` CLI**.
15. **Audit export formatters** (JSON, JSONL, CSV).
16. **Documentation** explicitly calls out audit-mode-only and the v1.0→v1.8 migration semantics.

## Acceptance criteria

- [ ] Every sensitive operation (auth, secrets, exec, state apply, policy eval) emits an `AuditEntry`. Verified by integration test.
- [ ] `kscore-audit log --since 1h --user alice` returns paginated entries.
- [ ] `kscore-audit export --format jsonl > out.jsonl` writes valid JSONL.
- [ ] Redaction strips configured patterns (e.g., `password=*`) from exported metadata.
- [ ] `kscore-policy eval my-policy.rego --input input.json` evaluates OPA policy and shows result + violations.
- [ ] `kscore-policy eval inline.cel --input input.json` evaluates CEL.
- [ ] `kscore-policy compliance --framework SOC2 --since 30d` returns ComplianceReport with rate, trends, top violations.
- [ ] `kscore-policy violations --severity critical --since 7d` lists violations.
- [ ] **v1.0 critical**: even when policies return `Allowed=false`, operations still proceed (audit-mode-only). Document in release notes loudly.
- [ ] CRUD RPCs return `Unimplemented` cleanly (clients can probe).
- [ ] Coverage >80% on `internal/policy`, `internal/audit`.

## Risks

- **Audit table growth** — retention policy MUST be set in v0.1 defaults; without it, disk fills. Monitor `kscore_audit_table_size_bytes`.
- **Redaction regex complexity** — overly broad patterns produce false positives. Document; recommend explicit pattern review before prod.
- **OPA default query assumption** — `data.<package>.allow` requires policies declare a package and `allow` rule. Provide templates.
- **CEL type errors at eval time** (dynamic typing) — `ValidatePolicy()` does syntax check but not full type validation. Test with realistic inputs.
- **Policy-set all-or-nothing semantics** (AND) — document; OR/threshold semantics for compliance-style sets in v1.5.
- **v1.0→v1.8 enforcement flip** is a behavior-changing release — release notes must call it out loudly.

## References

- PROJECT-DETAILS §4.12.
