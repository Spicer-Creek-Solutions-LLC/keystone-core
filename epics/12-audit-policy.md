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
2. **`AuditStore` interface + `SQLitePolicyAuditStore`** + retention + redaction. Extend `internal/state.Store`.
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
