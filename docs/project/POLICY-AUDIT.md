# Policy & Audit (v1.0)

Keystone Core's audit log and policy engine ship together (Epic 12,
[`PROJECT-DETAILS.md §4.12`](../../PROJECT-DETAILS.md)). This document is the
operator-facing guide to **how policy behaves in v1.0**, **what changes at
v1.8**, and the **gotchas** you must know before relying on either in
production.

## TL;DR

- The audit log is **fully live in v1.0**: every sensitive operation (auth
  decision, secret access, command exec, state apply, policy evaluation) writes
  an `AuditEntry`.
- The policy engine is **audit-mode-only in v1.0**: policies evaluate, audit,
  and report — **a denying policy never blocks the operation**.
- v1.8 flips enforcement on. That is a **behavior-changing release** — see
  [Migration: v1.0 → v1.8](#migration-v10--v18).

## Audit-mode-only — the v1.0 contract

> **v1.0 critical:** even when a policy returns `Allowed=false`, the operation
> still proceeds. Policy is *observability*, not *control*, until v1.8.

In v1.0 the `Enforcer` (`internal/policy/enforcer.go`) **always returns
`Allowed=true`**, regardless of the evaluation verdict or the policy's declared
`EnforcementMode`. The config knob `policy.enforcement_enabled` is **hardcoded
`false`** and is not operator-settable in v1.0.

What you still get:

- Every evaluation is recorded in the audit log (`action = "policy.evaluate"`).
- The `EnforcementDecision` carries `WouldDeny` — the *real* verdict
  (`!AllowedAll(results)`). Compliance reporting and the audit trail surface
  `WouldDeny` so you can **see exactly what would have been blocked** if
  enforcement were on. Run real policies against real workloads and build
  confidence before the v1.8 flip.

Why this design: full enforcement carries breaking-change risk — a
misconfigured policy could block the fleet. v1.0 lets you observe first.

## Migration: v1.0 → v1.8

v1.8 makes `policy.enforcement_enabled=true` available and the `Enforcer`
honors each policy's `EnforcementMode`:

| Mode      | v1.0 behavior        | v1.8 behavior (enforcement enabled)               |
|-----------|----------------------|---------------------------------------------------|
| `audit`   | log only             | log only (unchanged)                              |
| `warn`    | log only             | log + emit a warn event; **operation allowed**    |
| `enforce` | log only             | log + invoke violation handlers; **operation denied** |

This is a **behavior-changing release**. Before upgrading to v1.8:

1. Review every registered policy's `EnforcementMode`. Anything left at
   `enforce` will start **blocking** once `enforcement_enabled=true`.
2. Use the v1.0 audit trail / `WouldDeny` data to find policies that would have
   denied real traffic. Fix or downgrade them to `warn`/`audit` first.
3. The v1.8 release notes will call this out loudly; treat the flip as a
   staged rollout, not a silent minor bump.

The v1.8 *gate logic* is already implemented and tested behind the
`WithEnforcementEnabled` seam; the v1.8 **side-effects** (warn-event emission,
violation-handler dispatch) are tracked on the v1.x ROADMAP
([`docs/project/ROADMAP.md`](ROADMAP.md): "Policy enforcement side-effects").

## Gotchas

### OPA (Rego) policies

- A policy module **must** declare `package keystone.policy` and an `allow`
  rule. The evaluator queries `data.keystone.policy.{allow,violations,warnings}`.
- An **undefined or non-boolean `allow`** is fail-closed: the result is
  `Allowed=false` plus a synthetic violation (a misconfiguration is surfaced
  in the audit trail, not silently denied — and not an engine error).
- **Restricted builtins:** `http.send`, `net.*`, and `opa.runtime` are removed
  from the evaluator's capability set. A policy referencing them **fails to
  compile**. Policies must be pure decision logic — no network calls during
  evaluation (SSRF / exfil guard for operator-supplied policies).

### CEL policies

- A CEL policy is a single **boolean expression**. A non-bool expression is a
  compile error. `true` = allow; `false` = deny + a synthetic violation.
- Vars: `input` (the composite doc) plus convenience `resource` / `action` /
  `user` / `context`. `input.timestamp` is an RFC3339 string (parity with the
  OPA input doc) — wrap time-window checks with `timestamp(input.timestamp)`.
- A CEL **runtime** error (e.g. `no_such_key`) is a policy-authoring bug and is
  returned as an evaluator error — it is *not* coerced to a deny.

### Redaction on export

- `kscore-audit export` applies redaction at the export boundary. Configure it
  with `--redact-key` (drop a metadata key), `--redact-pattern` (regex-replace
  in metadata values + violation messages), `--redact-user` (blank the user
  field).
- **Review redaction regexes before production.** An overly broad pattern
  produces false positives and silently mangles otherwise-useful audit data.

### Audit table growth

- The audit log grows with every sensitive op. Retention defaults (90d /
  100k / hourly enforcement) are set in v0.1 defaults — **do not disable
  retention without a plan**, or the audit table will fill the disk. The
  `MinSeverity` exemption keeps high/critical entries beyond the age/count
  caps for compliance.

## CLI quickstart

`kscore-policy` — authoring + read surface (audit-mode in v1.0):

```text
# Local (no server) — author + CI-gate a policy file:
kscore-policy validate my-policy.rego
kscore-policy eval my-policy.rego --input input.json

# Remote (PolicyService gRPC on a running kscore-server):
kscore-policy list
kscore-policy show require-labels
kscore-policy compliance --framework soc2 --since 30d
kscore-policy violations --severity critical --since 7d
```

`kscore-audit` — audit-log + compliance roll-ups:

```text
kscore-audit log --since 1h --user alice
kscore-audit report --framework soc2 --since 30d
kscore-audit stats --since 7d
kscore-audit export --format jsonl --redact-pattern 'password=\S+' > out.jsonl
```

Both CLIs default to `--server localhost:9090`; auth via `--api-key` or
`KSCORE_API_KEY`. `kscore-policy eval`/`validate` are local and need no server.
Deferred subcommands (`kscore-policy check|test`,
`kscore-audit search|analyze|timeline|watch`, the export
`--redaction-config` file) are tracked on the v1.x ROADMAP.

## See also

- [`PROJECT-DETAILS.md §4.12`](../../PROJECT-DETAILS.md) — full design + the
  evaluator / Enforcer / ComplianceReport specifications.
- [`epics/12-audit-policy.md`](../../epics/12-audit-policy.md) — per-task
  implementation notes.
- [`docs/project/ROADMAP.md`](ROADMAP.md) — deferred policy/audit work
  (v1.8 enforcement side-effects, deferred CLI subcommands).
