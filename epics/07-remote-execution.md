# Epic 07: Remote Execution & Targeting

**Phase**: E • **Estimate**: 1.5 weeks • **Depends on**: 02, 03, 04, 05, 06 • **Blocks**: nothing critical (downstream features use it)

## Goal

Server-side orchestration of "run this command on these agents." Targeting expressions resolve to agent sets; commands stream output back. Single-agent and batch modes. Cross-platform shell abstraction. Command policy.

## Scope (in)

- `internal/targeting/` — `TargetExpression` (compiled `expr-lang/expr` VM program + raw string), `Matcher.Match(agent)` evaluating against flattened metadata, `BatchExecutor` semaphore-controlled parallel runner.
- Targeting capabilities (v1.0):
  - Glob patterns: `id:web-*`, `hostname:db-prod-*`.
  - Direct field match: `os:linux`, `arch:amd64`, `status:online`, `ip:10.0.0.0/8`.
  - Label match: `role:web`, `labels.env:prod`.
  - Compound: AND/OR/NOT with parens.
- `internal/execution/` — `Executor.Execute(ExecuteRequest)`, `ManagedExecution` with state machine (PENDING → RUNNING → RETRYING/COMPLETED/FAILED/TIMEOUT/CANCELLED), `Callbacks` for observable transitions, `Pipeline` for sequential stages with output piping.
- Shell abstraction: `Shell` interface with `Bash`, `Sh`, `Powershell`, `Cmd`, `Default`. `GetDefaultShell()` from `runtime.GOOS`; `IsAvailable()` via `exec.LookPath`.
- `CommandPolicy` (Strict/Normal/Permissive; AllowedCommands, AllowedPatterns, BlockedCommands, BlockedPatterns, AllowShellExecution, MaxCommandLength default 64KB). `ValidateForShell()` blocks shell metacharacters (`;`, `&`, `|`, backtick).
- `internal/controlplane/batch_dispatcher.go` (extends Epic 04 stub) — `ExecuteBatch(req) → batchID`; spawns async goroutine; progress messages on 500ms ticker.
- Streaming protocol per gRPC: BATCH_START → AGENT_START → AGENT_OUTPUT → AGENT_COMPLETE → BATCH_COMPLETE / BATCH_FAILED with `BatchSummary{total, successful, failed, success_rate}`.
- Persistence: `state.BatchJobStore` (job, target, command, status, counts, timestamps); `batch_agent_results` (per-agent success/exit/error/timing).
- `cmd/kscore-exec/main.go` and `kscorectl exec` dispatching: `run`, `async`, `status`, `list`, `cancel`, `output`, `script`. Flags: `--target`, `--concurrency` (default 10), `--command-timeout` (default 5m), `--continue-on-failure`, `--shell`, `--working-dir`, `--user`, `--env KEY=VAL` (repeatable), `--job-id`, `--dry-run`.
- Output truncation defaults: stdout 1 MB, stderr 256 KB, combined 2 MB; configurable.

## Scope (out / non-goals)

- Fact-based selectors (`facts.memory > 16Gi`) — v1.1 (need agent fact schema first).
- Percentage-based / rolling batches — v1.2.
- Output archival to object storage cold-tier — v1.4.
- Interactive shell over stream — v1.x.

## Design summary

See `PROJECT-DETAILS.md §4.7`.

## Tasks

1. **`TargetExpression` + parser** using `expr-lang/expr`. Inject `match()` custom function for glob-pattern eval.
2. **Agent metadata flattener** (id, hostname, os, arch, labels.*, ip, status) for matcher input.
3. **`Matcher.Match(agent)`** + extensive table tests (positive + negative cases, compound expressions).
4. **`Executor.Execute`** + state-machine wrapper + callbacks. Tests for retry, timeout, cancel.
5. **`Pipeline` sequential executor** + tests (rare external use; underlies blueprint apply).
6. **Shell abstraction** + per-platform tests (Linux runs bash/sh; Windows skipped in v1.0 — covered by v1.1).
7. **`CommandPolicy.Validate` / `ValidateForShell`** + tests (positive + injection attempts).
8. **`BatchDispatcher.ExecuteBatch`** with semaphore concurrency + progress ticker + result aggregation.
9. **gRPC streaming server** for `BatchExecuteCommand` and single `ExecuteCommand` (calls into BatchDispatcher with batch-of-one).
10. **`state.BatchJobStore` + `batch_agent_results`** (extends Store interface from Epic 02).
11. **`cmd/kscore-exec`** Cobra commands; output formatters (table, json, yaml).
12. **Integration test**: 5-agent docker-compose; targeting expression hits 3; batch returns combined summary.

## Acceptance criteria

- [ ] `kscorectl exec run "uptime" --target os:linux` runs against all linux agents and streams output.
- [ ] `--target "role:web AND env:prod"` resolves with AND semantics.
- [ ] `--target "role:db OR role:cache"` resolves with OR.
- [ ] `--target "NOT role:legacy"` excludes correctly.
- [ ] `--dry-run` prints matched agents without dispatching.
- [ ] Async mode returns job ID immediately; `kscorectl exec status <id>` returns RUNNING then COMPLETED.
- [ ] `kscorectl exec cancel <id>` cancels in-flight execution; agent receives SIGTERM.
- [ ] Shell metacharacters blocked in Normal mode; allowed only when `--shell` flag passed.
- [ ] Output truncation works at configured limits.
- [ ] Single-agent latency <1s for trivial command on local NATS; 5-agent batch <2s.
- [ ] Coverage >80% on `internal/targeting`, >75% on `internal/execution`.

## Risks

- **Targeting eval cost** at fleet scale (O(N×E)) — narrow expressions; no agent-set indexing in v1.0.
- **Output buffering** — large outputs go entirely into memory at agent and CP. Truncation at storage layer; document.
- **Stream reconnection** — clients re-call `output <job-id>` to resume; no in-stream resume in v1.0.
- **Race in pipeline output handlers** — sync primitives in user-callback aggregators.

## References

- PROJECT-DETAILS §4.7.
