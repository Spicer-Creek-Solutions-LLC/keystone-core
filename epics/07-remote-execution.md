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

1. ~~**`TargetExpression` + parser** using `expr-lang/expr`. Inject `match()` custom function for glob-pattern eval.~~ ✅ (`internal/targeting/{expression,parse,match}.go`; shorthand → expr-source translator; `gobwas/glob`-backed cached `match()`; non-builtin fields desugar to `labels.<name>`.)
2. ~~**Agent metadata flattener** (id, hostname, os, arch, labels.*, ip, status) for matcher input.~~ ✅ (`internal/targeting/flatten.go`; `state.AgentRecord` → env map; `connected` normalized to `online`; `match()` extended for `[]string` slices and CIDR patterns so `ip:10.0.0.0/8` resolves against `IPAddresses`.)
3. ~~**`Matcher.Match(agent)`** + extensive table tests (positive + negative cases, compound expressions).~~ ✅ (`internal/targeting/matcher.go`; thin wrapper around `expr.Run` against `Flatten`'d agent records; extensive table covers all builtins, status normalization, IPv4/IPv6 CIDR, label sugar, AND/OR/NOT compounds, parens, nil-safety, run-error and non-bool result paths.)
4. ~~**`Executor.Execute`** + state-machine wrapper + callbacks. Tests for retry, timeout, cancel.~~ ✅ (`internal/execution/{doc,executor,managed}.go`; `internal/agent.Executor` already wraps `os/exec` per §4.6 — Epic 07's `internal/execution` adds an Executor *interface* plus `ManagedExecution` lifecycle wrapper with `Callbacks{OnStarted,OnCompleted,OnFailed,OnTimeout,OnCancelled,OnRetrying,OnRetry}` and a `RetryPolicy{MaxAttempts,InitialBackoff,BackoffMultiplier,MaxBackoff}`. State machine PENDING → RUNNING → COMPLETED/FAILED/TIMEOUT/CANCELLED/RETRYING → RUNNING; ctx-aware sleep between retries.)
5. ~~**`Pipeline` sequential executor** + tests (rare external use; underlies blueprint apply).~~ ✅ (`internal/execution/pipeline.go`; `Stage{Request, Transform}`; stdout → next-stage stdin; per-stage Transform optional; `FailFast` returns `ErrPipelineFailed` wrapped with stage index; ctx cancel honored between stages.)
6. ~~**Shell abstraction** + per-platform tests (Linux runs bash/sh; Windows skipped in v1.0 — covered by v1.1).~~ ✅ (`internal/execution/shell.go`; `Shell` enum (Default/Bash/Sh/Powershell/Cmd) + `ParseShell`, `GetDefaultShell` (GOOS-aware; bash with sh fallback on linux/darwin, powershell on windows), `IsAvailable` via `exec.LookPath`, `CommandLine(cmd) (bin, argv)` returning `bash -c` / `sh -c` / `pwsh -NoProfile -Command` / `cmd /c`. `pwsh` preferred over `powershell` when both present. `shell_linux_test.go` build-tagged to linux for the platform-required assertions.)
7. ~~**`CommandPolicy.Validate` / `ValidateForShell`** + tests (positive + injection attempts).~~ ✅ (`internal/execution/policy.go`; `CommandPolicySpec` (raw) → `NewCommandPolicy` → compiled `CommandPolicy`. Modes Normal/Strict/Permissive; allow/block lists for exact `Command` match plus regex patterns against the full command line; deny-wins-over-allow; `MaxCommandLength` default 64 KiB. **Renamed `ValidateForShell` → `ValidateNoShell`** for clarity — it's the strict variant for direct-exec (no shell), blocks `;` `&` `|` backtick. PROJECT-DETAILS §4.7 amended with the rename note.)
8. ~~**`BatchDispatcher.ExecuteBatch`** with semaphore concurrency + progress ticker + result aggregation.~~ ✅ (`internal/controlplane/execute_batch.go`; sync part: validate, `CreateBatch`, register per-batch cancel; async goroutine: `MarkRunning` → semaphore-bounded fan-out (`req.Concurrency` or default 10) → `RecordAgentResult` per agent → `Finalize`; `Cancel(id)` triggers per-batch ctx → fan-out drains. 500ms progress ticker emits `BatchProgressEvent{Phase: Start|Progress|Complete}` non-blocking; final event sourced from persisted record (counter is wiped by Finalize). `Summary(batchID)` aggregates `BatchSummary{Total, Successful, Failed, SuccessRate}`. Orchestrator detaches from request ctx via `context.WithoutCancel` so gRPC handlers can return immediately.)
9. **gRPC streaming server** for `BatchExecuteCommand` and single `ExecuteCommand` (calls into BatchDispatcher with batch-of-one). — 9a-9c ✅ (`internal/controlplane/grpc_server.go` + `agent_resolver.go`; `streamingDecorator` wraps the injected `BatchExecutor` to emit AGENT_START / AGENT_COMPLETE / AGENT_FAILED; `BatchTerminal` sourced from orchestrator's Complete event; single-agent `ExecuteCommand` is a batch-of-one with `command_id` + `CommandCompletion`. Zero-match Targets emit a synthetic Completed terminal. Wired into `cmd/kscore-server` between `server.New` and `srv.Start` with `BatchExecutor=nil` — streaming RPCs respond `Unavailable` until task 9d/12 lands the NATS-backed runner. `BatchAgentOutput` chunks are V1X — agent returns buffered output in `CommandResponse`, no live streaming.) — 9d deferred to task 12.
10. ~~**`state.BatchJobStore` + `batch_agent_results`** (extends Store interface from Epic 02).~~ ✅ (Epic 04 shipped the base surface; this task extends it for the wire end-to-end. `BatchAgentResultRecord` gains `Stdout []byte`, `Stderr []byte`, `StdoutTruncated bool`, `StderrTruncated bool`; SQLite + Postgres DDL gains matching columns (BLOB / BYTEA + INTEGER / BOOLEAN truncation flags); `BatchJobStore.GetBatchAgentResult(batchID, agentID)` added so `kscorectl exec output --agent X` (task 11) can fetch a single row without listing the whole batch. Migrator's INSERT mirrors the new columns. Retention loop deferred to V1X.)
11. ~~**`cmd/kscore-exec`** Cobra commands; output formatters (table, json, yaml).~~ ✅ Subcommands live under `kscorectl exec` (operator's split — no separate binary). **11a**: `run`/`async`/`script` (`internal/cli/exec/`; `BatchStreamRenderer` → table/json/yaml; `ParseTarget` translates AND-of-labels-plus-hostname-glob client-side, full shorthand on V1X). **11b**: `status`/`list`/`cancel`/`output` + `--dry-run`; backed by five new gRPC RPCs (`GetBatchJob`, `ListBatchJobs`, `CancelBatchJob`, `ListBatchAgentResults`, `GetBatchAgentResult`) and a `dry_run` field + `BatchPreview` oneof variant on `BatchExecuteCommand{Request,Response}`. `output` defaults to human-readable framed sections; `--raw` switches to bytes-only mode (single-agent required). `--all` renders both stdout + stderr. `list --status` / `--since` / `--limit` / `--offset`.
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
