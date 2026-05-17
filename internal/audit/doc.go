// Package audit is the v0.x reconstruction of Keystone Core's
// audit log per PROJECT-DETAILS §4.12. The epic-12 design ships
// two related concerns in v1.0:
//
//  1. Full audit log of every sensitive op — auth decisions, secret
//     access, command exec, state-apply outcomes, policy evaluations
//     — persisted in SQL, queryable via gRPC / REST / CLI.
//
//  2. Policy engine in audit-mode-only — full evaluator (OPA + CEL +
//     13 builtins) lands in v1.0; the [internal/policy.Enforcer]
//     stub always returns `Allowed=true` regardless of evaluation
//     result. post-v1.0 flips one config flag.
//
// Why types-first: audit records cross every domain boundary that
// matters for compliance, and the shape is consumed by the policy
// engine + the SQL store + the gRPC + the CLI. The value types
// ([AuditEntry], [Violation], [Severity], [EnforcementMode],
// [PolicyType]) and the seam they ride on ([Auditor]) need to be
// stable before any persistence or hook implementation lands.
// Task 1 ships those shapes plus the in-memory [BufferedAuditor]
// + [MultiAuditor] fan-out.
//
// Task 1 lands the foundational value types and helpers:
//
//   - [AuditEntry] — the §4.12 record exactly; constructed via
//     [NewAuditEntry] which stamps a UUIDv7 [AuditEntry.ID] and a
//     UTC [AuditEntry.Timestamp].
//   - [Violation] — per-rule denial detail used by the engine and
//     surfaced in the audit record.
//   - [Severity] — ordered enum (Low < Medium < High < Critical)
//     used by both [AuditEntry] (max of violations / policy fallback)
//     and [Violation].
//   - [EnforcementMode] — Audit / Warn / Enforce. v1.0 ignores the
//     value at the Enforcer; the audit record preserves intent.
//   - [PolicyType] — OPA / CEL / Builtin; empty for non-policy
//     audit entries (auth, secrets, exec, state-apply hooks).
//   - [Auditor] — the emit interface every producer writes to.
//     No-error Emit per Epic 10/11 precedent ("fire and forget;
//     never error back to caller"); strict-fail variant tracked on
//     v1.x ROADMAP.
//   - [NoopAuditor] / [BufferedAuditor] / [MultiAuditor] —
//     in-memory implementations. The SQL-backed store + filter +
//     retention land in task 2.
//
// Roadmap of the rest of the epic, anchored on the types declared
// here:
//
//   - Task 2 — `AuditStore` interface + `SQLitePolicyAuditStore`
//     extending [internal/state.Store]; retention + redaction.
//   - Task 3 — `AuditFilter` query builder + cursor pagination.
//   - Task 4 — emission hooks into auth / secrets / state-apply /
//     command-exec / policy. CI test pins "every sensitive op
//     emits an entry."
//   - Task 5-9 — policy engine + OPA / CEL / Builtin evaluators.
//   - Task 10 — Enforcer stub.
//   - Task 11 — ComplianceReport + framework mappings.
//   - Task 12 — gRPC `PolicyService` server.
//   - Task 13 — REST handlers.
//   - Task 14 — `kscore-policy` + `kscore-audit` CLIs.
//   - Task 15 — JSON / JSONL / CSV export formatters.
//   - Task 16 — Documentation + integration test +
//     audit-mode→enforcement flip release notes.
package audit
