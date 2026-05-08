# Epic 06: Agent Runtime + Bootstrap UX

**Phase**: D • **Estimate**: 2 weeks • **Depends on**: 01, 02, 03, 05 • **Blocks**: 07, 08, 14

## Goal

`kscore-agent` Linux daemon. Registers with control plane via NATS, heartbeats with metrics + metadata, executes commands, hosts plugins, applies state. Single-binary TUI-guided bootstrap (Epic 27 in old repo) with non-interactive flag/env support.

## Scope (in)

- `cmd/kscore-agent/main.go` — Cobra root + subcommands: daemon (default), `bootstrap`, `config {enable-embedded-nats, disable-embedded-nats}`, `identity` (stub), `nats` (diagnostics).
- `internal/agent/agent.go` — `Agent` struct; lifecycle (Start, Shutdown); spawns heartbeat + metadata loops via WaitGroup; subscribes to `kscore.{cluster}.agent.{id}.command`; security enforcement before exec.
- `internal/agent/executor.go` — `Executor` wraps `os/exec`; timeout via context; env injection; working dir; user switching; SIGTERM 5s grace then SIGKILL.
- `internal/agent/metadata.go` — `gopsutil`-backed metadata collector. Collects on startup + periodic refresh (default 60s). Heartbeats carry only lightweight metrics (CPU, memory, disk %).
- `internal/agent/security.go` — `SecurityEnforcer` with HMAC validation, principal allowlist, command allowlist/blocklist (glob + regex), env-var filter, max arg length.
- `internal/agent/bootstrap/` — bootstrap engine with phases (detect, configure, validate, install, verify) + Bubble Tea TUI.
- Bootstrap CLI flags: `--mode demo|production|enterprise`, `--non-interactive`, `--cluster-name`, `--node-role`, `--join`, `--join-token`, `--postgres-*`, `--nats-*`, `--generate-certs`, `--apply-blueprint`, `--dry-run`.
- On-disk layout per PROJECT-DETAILS §4.6.
- Systemd unit installation (template `keystone-core-agent.service`).
- Reconnect with exp backoff.
- Graceful shutdown (SIGTERM, drain in-flight commands, exit).
- Plugin host integration with `pkg/plugin` discovery (Epic 14).
- State runner integration (Epic 08) via local module dispatch.

## Scope (out / non-goals)

- Embedded NATS / hybrid mode / leaf node — v2.0.
- Endpoint advertiser / reverse-leaf NAT traversal — v2.0.
- Windows agent (native service) — v1.1.
- macOS agent — v1.2.
- Interactive shell sessions — v1.x.
- VM-based bootstrap test harness — v1.2 (Docker CI suffices for v1.0).
- Auto-rotation of in-memory NATS creds — v1.3 (gates on SPIRE).

## Design summary

See `PROJECT-DETAILS.md §4.6`.

## Tasks

1. **`Agent` struct + lifecycle** in `internal/agent/agent.go`. Start spawns heartbeat (default 30s) + metadata (default 60s) loops; subscribes to command topic; runs until SIGTERM. _(landed: `internal/agent.Agent` with narrow `NATSClient`/`Subjects`/`MessageHandler`/`Subscription` interfaces (mirrors the `internal/controlplane` pattern — internal/agent stays free of internal/nats imports). Heartbeat + metadata payloads are minimal stubs (`{agent_id, ts}` / `{agent_id, labels}`); Task 3 swaps them for gopsutil-backed metrics. Command-subscription handler logs receipt; Tasks 4 + 5 wire SecurityEnforcer + Executor + response publication. cmd/kscore-agent now constructs an external-mode Manager + Agent + adapter (mirrors cmd/kscore-server's pattern); rejects embedded-mode and empty AgentID. `internal/config.AgentConfig` added with §4.6 defaults (heartbeat 30s, metadata 60s, command timeout 5m).)_
2. **`Executor`** with `os/exec` wrapping; signal grace; output capture; user switching (Linux uid/gid). _(landed: `internal/agent.Executor` with structured `ExecuteRequest`/`ExecuteResult`, configurable kill grace (5s default), hard-cap output truncation (1 MiB stdout / 256 KiB stderr defaults per §4.7), env injection + allowlist filter, working-dir override, ctx-derived timeout. SIGTERM-grace-then-SIGKILL kill protocol verified against a SIGTERM-trapping shell loop. User switching split via build tags: `exec_user_unix.go` uses `syscall.Credential`; `exec_user_windows.go` returns a v1.0-not-supported error. System-level errors surface in `Result.Error` rather than as a Go error so callers serialize the outcome cleanly. Task 5 wires this into the Agent's command-handler pipeline.)_
3. **`MetadataCollector`** via `gopsutil` (CPU, memory, disk %, virt detection, NICs incl IPv4 + IPv6 separated, dual-stack flagged). _(landed: `internal/agent/metadata.go` defines `MetricsCollector` interface + `gopsutilCollector` impl + the wire-format types `HeartbeatMetrics` (CPU%, mem%, top-5 disks by total size) and `AgentMetadata` (full per-host: distro, kernel, NICs with IPv4/IPv6 separated and `DualStack` flag, virt detection via `host.Info`, CPU count, mem total, all physical-fs mounts). Stub payloads from Task 1 deleted; Agent.New now requires a MetricsCollector. `cmd/kscore-agent` wires `NewGopsutilCollector(log)`. cpu.Percent first-call zero documented inline. Pseudo-filesystems blocklist (tmpfs, proc, sysfs, cgroup, etc.) keeps disk inventory clean.)_
4. **`SecurityEnforcer`** with all v1.0 protections.
5. **NATS subscription handler** for `kscore.{cluster}.agent.{id}.command`; validates HMAC; routes to executor; publishes response on `kscore.{cluster}.agent.{id}.response`.
6. **Bootstrap engine** — phases as state machine (`pkg/statemachine` from Epic 15 — placeholder simple FSM for v1.0):
   - Detect (OS, distro, init system, package manager, existing install).
   - Configure (TUI or non-interactive).
   - Validate (DNS, network, storage backend reachable, certs ready).
   - Install (package repo setup, binary + systemd install, certs, schema migration, service enable).
   - Verify (services up, NATS reachable, DB reachable, cluster joined).
7. **TUI bootstrap** with Bubble Tea — multi-screen wizard, mode selection, cluster details, node role, storage backend, NATS config, TLS strategy, blueprint selection.
8. **Non-interactive bootstrap** with full flag/env coverage; errors out on ambiguous config.
9. **Systemd unit template** + install/uninstall logic.
10. **Reconnect logic** with exp backoff; logs every reconnect attempt.
11. **Graceful shutdown** — SIGTERM → unsubscribe → cancel contexts → wait for in-flight (max 30s) → exit.
12. **Integration test**: spawn embedded server + agent in same process; verify registration, heartbeat, command exec, graceful shutdown.

## Acceptance criteria

- [ ] `kscore-agent` Linux amd64 + arm64 binaries build with no CGO.
- [ ] Agent registers with control plane on startup (visible in `kscorectl agents list`).
- [ ] Heartbeat every 30s; control plane marks stale after 3 missed.
- [ ] Metadata published on startup + every 60s; visible via `kscorectl agents show <id>`.
- [ ] `kscorectl exec run "uptime" --target id:<agent-id>` returns within 1s.
- [ ] HMAC-invalid command rejected with audit log entry.
- [ ] Allowlist-blocked command rejected; audit logged.
- [ ] Bootstrap TUI walks through demo mode in <5 minutes (manual test).
- [ ] Non-interactive `kscore-agent bootstrap --non-interactive --mode demo --cluster-name x --join nats://server:4222` succeeds end-to-end in CI.
- [ ] Re-running bootstrap is idempotent (no duplicate systemd units, no broken state).
- [ ] SIGTERM produces clean unsubscribe + exit.
- [ ] Coverage >75% on `internal/agent`.

## Risks

- **Reconnect storms** if many agents hit a flapping CP — exp backoff with jitter is mandatory; document recommended NTP sync.
- **Clock skew** — heartbeats are timestamped; CP trust window must exceed expected skew. Document NTP.
- **Bootstrap idempotency** — every phase has a checkpoint. Re-running must not break existing state. Hammer with tests.
- **gopsutil cross-platform quirks** — Linux only in v1.0; flag clearly.
- **File-descriptor leaks** — every command goroutine must defer cleanup; verified by `goleak` in integration test.

## References

- PROJECT-DETAILS §4.6, §4.7 (exec).
