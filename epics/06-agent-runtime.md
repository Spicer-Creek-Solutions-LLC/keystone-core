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
4. **`SecurityEnforcer`** with all v1.0 protections. _(landed: `internal/agent.SecurityEnforcer` is a pure policy gate — HMAC-SHA-256 over canonical-encoded `CommandRequest` (length-prefixed Args, sorted Env keys; same defensive pattern as Task 6's dedup hash); principal allowlist; command rules with deny-wins-over-allow ordering (stdlib `path.Match` globs against `req.Command`; stdlib regex against full command line); env-var allowlist; max-args byte cap (default 64 KiB per §4.7); operator-configurable default policy (recommended deny). Audit logs every decision — WARN on reject, INFO on accept. Sentinel errors (`ErrHMACInvalid`, `ErrPrincipalDenied`, `ErrCommandDenied`, `ErrArgsTooLong`) for typed-rejection paths. Replay protection (timestamp window + nonce dedup) is a v1.1 addition per §4.10. Task 5 wires this between Agent.handleCommand and Executor.)_
5. **NATS subscription handler** for `kscore.{cluster}.agent.{id}.command`; validates HMAC; routes to executor; publishes response on `kscore.{cluster}.agent.{id}.response`. _(landed: full command flow end-to-end. `Agent.handleCommand` parses inbound `CommandRequest` → `SecurityEnforcer.Validate` → `Executor.Execute` → publishes `CommandResponse` on response subject (CorrelationID = inbound MessageID). Rejection paths publish `Rejected: true` with reason so the dispatcher sees a clean signal vs timeout. CP-side: `CommandDispatcher` gains a `Signer` interface; `DispatchRequest.Principal` plumbed; signs `CommandMessage` before publish. `cfg.Security` (HMACSecret hex, allowlists, command rules, env allowlist, MaxArgsBytes default 64 KiB, DefaultPolicy default deny) flows through both binaries — `securityPolicyFromConfig` translator + `commandSignerAdapter` (server-side) bridge config to the agent-package types. `CommandRequest` extended with `WorkingDir` + `TimeoutSeconds` and canonical() updated so signatures cover them. Wire format: `internal/agent/wire.go` defines `CommandResponse`; `internal/controlplane.CommandMessage` is the CP-side mirror with the same field set.)_
6. **Bootstrap engine** — phases as state machine (`pkg/statemachine` from Epic 15 — placeholder simple FSM for v1.0):
   - Detect (OS, distro, init system, package manager, existing install).
   - Configure (TUI or non-interactive).
   - Validate (DNS, network, storage backend reachable, certs ready).
   - Install (package repo setup, binary + systemd install, certs, schema migration, service enable).
   - Verify (services up, NATS reachable, DB reachable, cluster joined).
   _(landed: `internal/agent/bootstrap` package with the full Detect → Configure → Validate → Install → Verify FSM. State persisted to `/var/lib/keystone-core/bootstrap.json` (atomic temp+rename, 0o600) with explicit schema version + `LastError` so re-runs resume from the last successful checkpoint and operators can diagnose failures from the state file alone. Phase impls are interfaces (`Detector`, `Configurer`, `Validator`, `Installer`, `Verifier`) so Tasks 7 (TUI) and 8 (non-interactive flags) can swap impls without touching the engine. Default impls: gopsutil-backed host detect + `/etc/os-release` parse + init-system / package-manager probe; `StaticConfigurer` (non-interactive — Task 8 will wrap CLI flags); validator runs `Configuration.Validate` + TCP-dial of `JoinURL` + parent-dir creatable check; installer renders `internal/config.AgentConfig` to YAML, atomic-writes 0o640, byte-compares for idempotency (Created/Updated flags), `DryRun` skips write; verifier round-trips `internal/config.Load` against the rendered file + dials `JoinURL`. Cluster-wide HMAC secret in v1.0 — bootstrap-derived per-agent keys deferred to v1.x. CLI/TUI wiring is Tasks 7 + 8.)_
7. **TUI bootstrap** with Bubble Tea — multi-screen wizard, mode selection, cluster details, node role, storage backend, NATS config, TLS strategy, blueprint selection.
   _(landed: `internal/agent/bootstrap/tui` package implementing `bootstrap.Configurer` via `charmbracelet/huh` (built on bubbletea + lipgloss). Six-group form: (1) detection banner + Mode select, (2) ClusterName + AgentID, (3) NodeRole, (4) JoinURL + JoinToken (password echo), (5) ConfigPath + DryRun, (6) Confirm. Validators (`validate.go`) bind to each Input — clusterName / agentID / joinURL / configPath — so typos surface inline; the IPv6-bracket typo (`nats://::1:4222`) catches early with a friendlier message than url.Parse's "invalid port". Defaults seed from `cfg.Agent` + `cfg.NATS` + hostname fallback. Engine sees the TUI only as a Configurer — no engine/installer changes needed. cmd/kscore-agent gains a `bootstrap` subcommand; daemon mode unchanged. v1.0 supports demo mode end-to-end; production / enterprise modes show as visibly-deferred options and abort with a v1.x deferral message — TLS cert collection gates on Epic 11, blueprint selection gates on Epics 14/17. Tracked in `docs/project/V1X-BACKLOG.md`. Storage backend dropped from agent surface — server-only, lives in the future unified `kscore-bootstrap` binary. Manual <5-minute walkthrough is the acceptance test for the "TUI walks demo in <5 min" bullet; deferred until manual run.)_
8. **Non-interactive bootstrap** with full flag/env coverage; errors out on ambiguous config.
   _(landed: `kscore-agent bootstrap --non-interactive` flag set with `KSCORE_BOOTSTRAP_*` env-var fallback (flag wins on conflict). v1.0 demo-only flag set: `--mode --cluster-name --agent-id --node-role --join --join-token --config-path --state-path --dry-run` plus the `--non-interactive` switch itself. Branch in `runBootstrap`: non-interactive builds a `bootstrap.StaticConfigurer` from flags + ValidateForV10 gate; interactive seeds the TUI Defaults from the same flag values then runs Task 7's wizard. Required-flag enforcement: `--mode` and `--join` are hard errors when missing. The v1.0 mode gate (`bootstrap.ValidateForV10`) was lifted out of `tui.configurationFromValues` so both paths share one rejection — production / enterprise still error with the v1.x deferral message. `--state-path` is a small scope addition — needed so CI can target a writeable tmpdir instead of the engine default `/var/lib/keystone-core/bootstrap.json`. Dropped from the v1.0 surface vs. the original epic spec: `--postgres-*` (server-only), `--nats-*` beyond `--join`/`--join-token` (agents are external-mode only; embedded NATS is v2.0), `--generate-certs` (Epic 11), `--apply-blueprint` (Epic 14 + 17). `cmd/kscore-agent/bootstrap_test.go` covers happy path (engine end-to-end through Detect→Verify, config file written + parses, state file persisted), missing-mode rejection, missing-join rejection, production-mode v1.x rejection, env-var fallback (`KSCORE_BOOTSTRAP_MODE=production` reads through and is rejected), and flag-beats-env precedence (env says production but `--mode demo` wins).)_
9. **Systemd unit template** + install/uninstall logic.
   _(landed: `internal/agent/systemd` package — `Render(Params)` builds the canonical `keystone-core-agent.service` from operator-tunable fields (BinaryPath, ConfigPath, User/Group, EnvironmentFile, ExtraEnv, ReadWritePaths). Hardening directives baked-in: `NoNewPrivileges`, `ProtectSystem=strict`, `ProtectHome`, `PrivateTmp`, `ProtectKernelTunables/Modules/ControlGroups`, with `ReadWritePaths=/var/lib/keystone-core /var/log/keystone-core` as the default exemption. `Install` atomic-writes the unit, runs `systemctl daemon-reload`, optionally `enable --now`. Idempotent — same content on a re-run fires zero systemctl commands. `Uninstall` is the inverse (stop + disable + remove + daemon-reload), tolerant of stop/disable errors so already-inactive units still get cleaned up; no-op when nothing's installed. `Status` wraps `is-active` + `is-enabled` for scriptable output. systemctl invocations go through a `Runner` interface (production: `defaultRunner` shells out via os/exec; tests: `FakeRunner` that records call sequences and serves canned responses) — keeps coreos/go-systemd's transitive 5MB dep tree out for the 6 calls v1.0 actually needs. `kscore-agent service install|uninstall|status` Cobra subcommand exposes the package; flags + `KSCORE_SERVICE_*` env-var fallbacks (flag wins) match the bootstrap subcommand pattern. Linux-only — non-Linux invocations short-circuit with "Linux-only in v1.0" pointing at v1.1 (Windows) / v1.2 (macOS). Two-step operator flow for v1.0 (`bootstrap` writes config, `service install` writes unit) — production-mode bootstrap auto-installation gates on Epic 11 + 14 + 17 lifting the demo-only mode gate. Dropped vs PROJECT-DETAILS §4.6 spec: `service start|stop` (use native `systemctl` — no value over native invocation) and dedicated `keystone-core` user auto-creation (gates on package-mgmt; operator creates manually + passes `--user/--group`). All deferrals tracked in `docs/project/V1X-BACKLOG.md`.)_
10. **Reconnect logic** with exp backoff; logs every reconnect attempt.
11. **Graceful shutdown** — SIGTERM → unsubscribe → cancel contexts → wait for in-flight (max 30s) → exit.
12. **Integration test**: spawn embedded server + agent in same process; verify registration, heartbeat, command exec, graceful shutdown.

## Acceptance criteria

- [ ] `kscore-agent` Linux amd64 + arm64 binaries build with no CGO.
- [ ] Agent registers with control plane on startup (visible in `kscorectl agents list`).
- [ ] Heartbeat every 30s; control plane marks stale after 3 missed.
- [ ] Metadata published on startup + every 60s; visible via `kscorectl agents show <id>`.
- [ ] `kscorectl exec run "uptime" --target id:<agent-id>` returns within 1s.
- [x] HMAC-invalid command rejected with audit log entry. _(task 5 — `TestAgent_CommandFlow_HMACInvalidPublishesRejection` covers the full flow: bad signature → SecurityEnforcer.Validate returns ErrHMACInvalid → audit WARN line → CommandResponse{Rejected: true} published.)_
- [x] Allowlist-blocked command rejected; audit logged. _(task 5 — `TestAgent_CommandFlow_AllowlistBlocksPublishesRejection` covers default-deny enforcement → audit WARN → CommandResponse{Rejected: true} published.)_
- [ ] Bootstrap TUI walks through demo mode in <5 minutes (manual test).
- [x] Non-interactive `kscore-agent bootstrap --non-interactive --mode demo --cluster-name x --join nats://server:4222` succeeds end-to-end in CI. _(task 8 — `TestBootstrap_NonInteractiveHappyPath` in `cmd/kscore-agent/bootstrap_test.go` runs the in-process bootstrap subcommand with this exact flag shape against an ephemeral TCP listener and asserts state persisted + config rendered + parses via `internal/config.Load`.)_
- [x] Re-running bootstrap is idempotent (no duplicate systemd units, no broken state). _(task 6 — `TestEngine_ReRunAfterDoneIsNoOp` covers PhaseDone short-circuit; `TestDefaultInstaller_Idempotent` + `TestDefaultInstaller_DetectsChange` cover byte-compare config rewrite; `TestEngine_ResumeSkipsCompletedPhases` + `TestEngine_ResumeAfterFailureContinuesFromCheckpoint` cover mid-flow checkpoint resume. Systemd unit installation is Task 9.)_
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
