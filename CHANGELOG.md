# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

Pending entries for the next release live as YAML fragments under
[`.changes/unreleased/`](.changes/unreleased/). They roll up into a
versioned section here via `make changelog-batch VERSION=v0.x.y` at
release time. Preview the accumulated set without writing the file
via `make changelog-preview`.

Per-PR workflow: instead of editing this section directly, run `make
changelog-new` (or `changie new`) to create a fragment file. See
[CONTRIBUTING.md § Changelog entries](CONTRIBUTING.md#changelog-entries).

## [v1.0.0] — Planned

Pending all 19 epics complete + the v1.0 gate checklist in
[`docs/project/VERSIONING.md`](docs/project/VERSIONING.md). The full v1.0.0
entry will land with the v1.0 cut; the in-progress feature inventory tracks
under [`FEATURES.md`](FEATURES.md). Until then, v0.x is the active release
line per the v0.1 → v0.5 → v1.0 ladder.

## [v0.5.0] — 2026-06-27

### Added

- **`service` module gains OpenRC and sysvinit backends.** v0.1 shipped
  systemd-only and returned `ErrNoBackend` from mutating ops on
  non-systemd hosts. The module now auto-detects the host init system
  and dispatches accordingly: systemd (Debian/Ubuntu/RHEL/Rocky), OpenRC
  (Alpine/Gentoo, via `rc-service` + `rc-update` managing the default
  runlevel), and sysvinit (older RHEL/CentOS/Devuan, via `chkconfig` /
  `update-rc.d`). OpenRC is the gate-v0.5 backend; sysvinit was pulled
  forward from post-v1.0. launchd (macOS) remains post-v1.0. systemd
  stays preferred when more than one init system is present.
- **`disk` filesystem resize and `lvm` LV/VG reconciliation.** Both
  modules were create-only — an LV below its declared size, a VG whose
  PV set had drifted, or a filesystem not filling a grown device was
  left unreconciled. Now `disk` with `resize_fs: true` grows ext2/3/4
  (`resize2fs`), xfs (`xfs_growfs`), btrfs (`btrfs filesystem resize`),
  and f2fs (`resize.f2fs`) to fill the block device, each only when it
  does not already fill it (idempotent, never shrinks). `lvm` grows a
  size-based LV (`lvextend`, optionally `--resizefs`) using "at least"
  semantics that never shrink, and reconciles an existing VG's PV set
  (`vgextend`/`vgreduce`, matched against LVM's canonical `pv_name`).
  `vgreduce` runs without `-f`, so LVM refuses to drop a PV still
  holding extents. `extents`-based LVs stay create-only.
- **`kscorectl agent verify` re-checks agent certificates against the
  trust bundle.** `verify <agent-id>` (or `verify --all` for the fleet)
  validates each agent's stored certificate against the control plane's
  current trust bundle: it chains to a trusted authority, is within its
  validity window, and carries a matching `agent/*` SPIFFE identity.
  Useful after a CA rotation to confirm agents are still trusted. Exits
  non-zero when any agent with a stored cert fails, so it scripts cleanly;
  agents with no stored cert report `has_cert=false` and are skipped, not
  failed. Backed by a new `VerifyAgent` RPC and a reusable
  `identity.VerifyAgentCert` helper (chain + expiry + SPIFFE checks
  reported separately, so an expired-but-trusted cert is distinguishable
  from an untrusted one).
- **CA private keys can be encrypted at rest (`EncryptedFileCAStorage`).**
  Setting `identity.encryption_key` (an `env:`/`file:`/`inline:`
  master-key source) seals the persisted CA private-key files
  (`root-key.pem`, `signing-key.pem`) as AES-256-GCM envelopes while
  leaving the certificate files as ordinary public PEM; leaving it empty
  keeps the backward-compatible plaintext `FileCAStorage`. The envelope
  carries a master-key fingerprint and per-write nonce, so a wrong key
  fails fast on the fingerprint guard, a tampered file fails GCM
  authentication, and a still-plaintext key is reported with a migration
  hint rather than failing opaquely. The key bytes never log — only the
  fingerprint. `kscore-identity ca encrypt` migrates an existing
  plaintext CA directory in place.
- **Clustering boots in `kscore-server` (Epic 13, PR-A).** Setting
  `cluster.enabled: true` now constructs the etcd-backed clustering
  stack at boot — membership, leader election, the canonical
  `SingletonTaskManager` leader gate, and the shard store + manager —
  and registers the `ClusterService` gRPC + `/api/v1/cluster/*` REST
  surface (status excepted) against the live managers, replacing the
  503/Unimplemented stubs. The events retention enforcer and shard
  rebalance are now leader-gated, so a cluster runs those side-effects
  on exactly one node instead of every node. A new `cluster.node`
  config block (`name`, `advertise_addr`) identifies this member.
  
  The single-node default path is unchanged: `cluster.enabled` stays
  `false`, no etcd is started, and the cluster surfaces remain dark.
  
  Deferred to follow-up PRs: the dedicated mTLS server↔server
  CoordinationService listener + CoordinationClient switchover and the
  `GracefulShutdown` SIGTERM hookup (PR-B); `FailoverManager` wiring
  (waits on a production agent/job reassigner); and HealthMonitor-backed
  quorum status.
- **Server↔server coordination channel + graceful shutdown (Epic 13,
  PR-B).** Setting `cluster.coordination.listen_addr` now starts a
  dedicated mTLS-only `CoordinationService` gRPC listener (separate
  from the agent/operator surface) and the peer-dialing
  `CoordinationClient`, whose dial pool is reconciled from cluster
  membership. Peers exchange health/leader/recovery over this channel
  when NATS is unavailable. The channel is mTLS-only by contract, so it
  requires `identity.enabled: true` — a listener configured without
  identity fails the boot loudly.
  
  `kscore-server` now also drives the §4.15 graceful-shutdown sequence
  on SIGTERM when clustering is enabled: the node is marked `LEAVING`
  (peers rebalance its shards off before it exits), leadership is
  transferred if held, then the member deregisters — before the API
  server stops.
  
  A new client-side mTLS builder (`identity.BuildClientTLSConfig`)
  issues this node's control-plane SVID for dialing and verifies peers
  against the trust bundle by SPIFFE ID (not DNS), with the same
  signing-CA rotation watcher as the server path.
  
  Deferred to follow-ups: the HealthMonitor-backed quorum status
  (`/cluster/status` stays 503; the coordination `ClusterHealth`/NATS
  status report best-effort until then), the `FencingManager` in-flight
  drain, and `FailoverManager` wiring.
- **Split-brain fencing enforced at the server write paths (Epic 13,
  PR-D).** When clustering is enabled, `kscore-server` now constructs
  the `FencingManager` at boot and guards both request surfaces through
  it: a gRPC interceptor rejects write RPCs with `Unavailable`, and an
  HTTP middleware rejects mutating REST requests
  (`POST`/`PUT`/`PATCH`/`DELETE`) with `503`, whenever this node is
  fenced — i.e. it has lost etcd quorum (a minority partition) or been
  superseded as leader (a stale epoch).
  
  The fence mode follows `cluster.fencing.mode` (default `read_only`:
  reads continue, writes are blocked; `strict` blocks reads too). Reads
  and unlisted methods keep flowing on a minority node, with
  storage-layer etcd quorum backstopping correctness. The
  `CoordinationService` is never fenced — it is the server↔server
  recovery channel that must work during a partition.
  
  Graceful shutdown now also drains in-flight guarded operations (via
  `FencingManager.Drain`) before the member deregisters.
  
  No new configuration — `cluster.fencing.mode` already existed.
  
  Deferred: the `ValidEpoch`-at-commit refinement for the narrow window
  where an operation begins before a fence and commits after.
- **Cluster health monitoring + `/cluster/status` (Epic 13, PR-C).**
  When clustering is enabled, `kscore-server` now runs the cluster
  `HealthMonitor` at boot: built-in etcd + heartbeat checkers (the
  canonical quorum signal) plus non-critical `storage` and `nats` ping
  checkers, so a database or NATS outage surfaces this node as
  `degraded` while only loss of etcd quorum drives it `unhealthy`.
  
  The monitor's status + quorum verdict now back three previously-dark
  surfaces: `GET /api/v1/cluster/status` returns the cluster identity,
  leader, member/healthy counts and quorum (was 503); the
  `ClusterService.GetClusterStatus` gRPC reports quorum; and the
  server↔server `CoordinationService.ClusterHealth` RPC answers instead
  of `Unavailable`. The monitor also drives each member's lifecycle
  status (`healthy`/`degraded`/`unhealthy`) in the membership record.
  
  No new configuration — the monitor reads the existing
  `cluster.health` block (`check_interval`, `failure_threshold`,
  `latency_window`).
  
  Deferred to follow-ups: the coordination `NATSStatus` field (needs a
  connectivity seam on the NATS manager), the `FencingManager`
  write-path guard, and `FailoverManager` wiring.
- **Coordination channel reports real NATS reachability (Epic 13
  follow-up).** `nats.Manager` gains `Connected()` and `Detail()`
  methods (thin, non-blocking wrappers over its existing health check),
  and `kscore-server` wires them into the server↔server
  `CoordinationService`. `CoordinationService.NATSStatus` now answers
  with this node's real NATS connection state (connected + a
  mode/URL detail) instead of `"unknown"`, and
  `ClusterHealth.nats_healthy` reflects it. No configuration change.
- **`firewall` abstraction now opens IPv6 and supports multi-port
  services.** A `firewall: {service: ssh}` declaration on an iptables
  host previously opened IPv4 only (the nftables and firewalld backends
  already covered both families); it now emits both an IPv4 and an IPv6
  sub-rule by default, matching the other backends. On an IPv4-only host
  the IPv6 sub is skipped gracefully with a loud `IPv6 NOT APPLIED`
  notice rather than leaving perpetual drift. The service catalog is now
  multi-port — `samba` opens its four ports, firewalld-native names
  (`dhcpv6-client`, `cockpit`, `nfs`, …) are recognised, and an opt-in
  `strict_catalog: false` falls back to the host's `/etc/services` for
  names not in the curated catalog.
- **`kscore-module run` executes modules end-to-end (Epic 14).** The
  module CLI gains a `run` subcommand that loads a module (a directory
  is packaged on the fly, or pass a built `.zip`), verifies its
  signature against trusted keys (`--key` + `--sig`, or
  `--skip-verification` for local development), and executes
  `main(input)` — printing the returned object as JSON.
  
  Module capability calls are now live: the Epic 14 task-12
  capability→Starlark builtin shims landed, so a module can call
  `fs_read`, `fs_write`, `http_get`, `http_post`, `exec_run`,
  `secret_read`, `secret_write`, `kv_get`/`kv_set`/`kv_delete`, and
  `log` — each routed through the manifest-declared, scope-enforced
  capability backend (path globs, domain allowlists, command
  allowlists, size/rate/timeout limits). A capability a module did not
  declare is simply absent from its namespace.
  
  For the standalone CLI the security boundary is signature
  verification plus the capability layer; server-side module execution
  (and the policy-engine / secrets-broker host adapters) is a separate
  follow-up.
- **`kscorectl agent list` subcommand.** Lists registered agents over
  the gRPC `ControlPlaneService`, rendered as a table
  (id/status/hostname/os/last-heartbeat/labels) or JSON. Supports
  `--status` (pending/connected/stale/disabled), `--label key=value`,
  and `--limit` filters, and inherits the standard
  `--server`/`--api-key`/`--output` flags (with the `KSCORE_API_KEY`
  fallback) from the parent `agent` command.
- **Boot-survive persistence for the network-base modules (`network`,
  `route`, `bond`, `bridge`, `vlan`).** These modules previously
  reconciled only the live runtime config (`ip` ops), which does not
  survive a reboot. A new opt-in `persist: networkd|netplan|auto`
  declaration additionally renders a boot-survive file — a
  systemd-networkd unit/drop-in under `/etc/systemd/network/` or a
  netplan YAML document under `/etc/netplan/` — mirroring the declared
  addresses, routes, MTU, and device attributes. It is additive to the
  runtime reconcile (the file is for the next boot; the runtime is
  already live), and `auto` picks netplan when `/etc/netplan` exists
  else networkd. networkd drop-ins are the multi-route / multi-member
  backend (netplan replaces lists across files). NetworkManager, RHEL
  `ifcfg-*`, and `ifupdown` renderers remain v0.x.
- **Tooling to build signed APT and DNF/YUM package repositories.** New
  `make repo-build` / `repo-smoke` / `repo-clean` targets (backed by
  `scripts/repo/`) turn the `.deb` / `.rpm` artifacts from
  `make release-snapshot` into a host-agnostic static repository tree —
  an `apt-ftparchive` `dists/stable` pool with a signed `InRelease`, and
  per-arch `createrepo_c` metadata with a signed `repomd.xml.asc` — ready
  to serve from `repos.keystone-core.io`. Signing supports a real GPG key
  (`REPO_SIGN=key:<id>`, the same key as the release ceremony), an
  ephemeral throwaway key for local validation (`REPO_SIGN=test`), or
  unsigned dev builds (`REPO_SIGN=skip`). `make repo-smoke` install-tests
  the tree in fresh `debian:12-slim` (apt) and `rockylinux:9` (dnf)
  containers (over `file://`), exercising signature verification
  end-to-end. The Linux-only index tools (`apt-ftparchive`,
  `dpkg-scanpackages`, `createrepo_c`) run in those containers via
  **docker or podman** when absent on the host, so the whole flow runs
  from macOS with only `gpg` + a container engine — signing always runs
  on the host, keeping the key out of any container. `release-smoke` is
  likewise docker/podman-agnostic and macOS-safe. Operator install
  templates land under `deploy/repos/`. `make repo-publish` ships the tree
  to the self-hosted server over rsync-over-ssh: it is **server-canonical
  with an incremental local cache** and **multi-version**, so it pulls the
  current repo, merges the new release, regenerates the metadata over
  every published version (users can pin/downgrade —
  `apt-get install kscore-cli=<ver>`), verifies signatures when signing,
  and uploads with `--delay-updates` for a near-atomic switchover.
  `REPO_SIGN=unsigned` publishes an unsigned repo (the v0.1–v0.7 posture:
  `apt [trusted=yes]` / `dnf repo_gpgcheck=0`, same trust level as the
  direct downloads); a test-key tree is always refused. Leading the
  getting-started guide with the `apt install` / `dnf install` path, and
  GPG-signing the repos, are the v0.8 follow-ups.
- **`package` module gains dnf, apk, zypper, and pacman backends.** v0.1
  shipped apt-only (Debian/Ubuntu) and returned `ErrNoBackend` on every
  other host. The module now auto-detects and dispatches to the native
  package manager on each supported Linux family — apt (Debian/Ubuntu),
  dnf+rpm (RHEL/Rocky/Fedora), apk (Alpine), zypper (openSUSE/SLES), and
  pacman (Arch). dnf and apk are the gate-v0.5 set; zypper and pacman
  were pulled forward from post-v1.0. Each backend handles its own
  version-pin syntax (`name=version` for apt/apk/zypper, dnf's
  `name-version`); pacman, a rolling release with no version-pin install
  spec, returns `ErrVersionPinUnsupported` rather than silently
  installing latest. Detection order is apt → dnf → apk → zypper →
  pacman, so a mixed EPEL-on-RHEL host still resolves to its primary
  manager.
- **`security` module gains AppArmor per-profile mode management.** The
  module shipped SELinux-only (mode + booleans); it now also manages
  AppArmor profiles on Debian/Ubuntu via `apparmor.profile: <name>` +
  `apparmor.profile_mode: enforce|complain|disable`. The op is selected
  by which params are set — the same dispatch the module already uses
  for SELinux — so existing SELinux declarations are unchanged.
  Idempotency reads `aa-status --json`; mode changes shell out to
  `aa-enforce`/`aa-complain`/`aa-disable`. Returns
  `ErrAppArmorUnavailable` when the tooling is absent or AppArmor is not
  the active LSM. Framework on/off and `apparmor_parser` load/reload
  stay v0.x.
- **`kscorectl agent quarantine` / `unquarantine` — isolate a compromised
  agent.** `quarantine <agent-id>` transitions an agent to DISABLED so the
  control plane rejects its heartbeats and dispatches no commands to it —
  the incident-response isolation step (an optional `--reason` is recorded
  in the server log). `unquarantine <agent-id>` reverses it, restoring the
  agent to CONNECTED (the connection monitor re-stales it if it is
  actually gone). Backed by new `QuarantineAgent`/`UnquarantineAgent`
  RPCs on `ControlPlaneService`, wired to the existing
  `ConnectionManager` disable/enable path (whose isolation semantics —
  heartbeat rejection + command-dispatch refusal — were already in place).
- **Agents now record their issued certificate's metadata.** When the
  control plane mints an agent's X509 SVID during bootstrap, it captures
  the issued chain plus the leaf's SHA-256 fingerprint, expiry, and
  SPIFFE ID onto the agent record. `kscorectl agent list -o json` surfaces
  `cert_fingerprint`, `cert_not_after`, and `spiffe_id` (empty for agents
  registered before this landed). The stored chain is server-internal,
  laying the groundwork for an `agent verify` command that re-checks each
  agent's cert against the current trust bundle. The four nullable columns
  are added to the agents table automatically at store-open on both SQLite
  and Postgres (no operator migration step).
- **Cross-distro reboot detection and a `system.rebooted` event across
  the reboot disconnect.** The `system` module's reboot-needed check
  previously gated solely on the Debian/Ubuntu
  `/var/run/reboot-required` marker, so RHEL/Rocky/Fedora always
  reported no reboot needed; when the marker is absent it now also
  consults `needs-restarting -r` (dnf-utils), by binary detection.
  Separately, the agent now stamps its Linux boot-ID on every heartbeat,
  and when a managed host reboots — heartbeat stops, server marks it
  stale, a new boot-ID proves the gap was a planned reboot — the control
  plane emits a new canonical `system.rebooted` event (carrying the
  old/new boot-ID and whether the reboot coincided with a real
  disconnect) instead of leaving an unexplained gap. Correlating the
  event with the originating reboot command, and persisting boot-IDs
  across server restarts, stay v0.x.
- **`user` and `group` modules gain a BusyBox backend (Alpine).** Both
  modules previously shelled out unconditionally to shadow-utils
  (`useradd`/`groupadd`/`usermod`/…), which Alpine does not ship unless
  the `shadow` package is installed. They now auto-detect: shadow-utils
  where present (Debian/Ubuntu/Rocky keep the full-featured backend),
  else BusyBox (`adduser`/`deluser`/`addgroup`/`delgroup`) on Alpine,
  else an undetected provider whose Lookup still works via NSS. BusyBox
  ships no `usermod`/`groupmod`, so in-place attribute modification
  returns `ErrModUnsupported`; create/delete/lookup and
  supplementary-group reconciliation — everything an idempotent apply
  exercises — are fully supported.
- **Auto-generated State Modules reference on the docs site.** Every one
  of the 35 built-in state stdlib modules (`file`, `package`, `service`,
  `cron`, `mount`, `lvm`, `firewall`, …) now has a reference page —
  purpose, valid states, parameters, runnable Salt-style examples, and
  distro/support notes — grouped by category under **Docs → State
  Modules**. The pages are generated from each module's structured `Doc`
  method (`internal/statemgmt/stdlib/*/doc.go`) by a new
  `tools/gendocs/modules` generator wired into `make docs-sync`, so they
  stay in sync with the code: the generator requires every registered
  module to be documented and cross-checks each page's states against the
  module's live `ValidStates()`, and `make docs-sync-check` (a CI gate)
  fails on any drift.
- **"Using State Management" guide + an Examples section on the docs
  site.** A new getting-started guide (now the first one) teaches the
  state-file language, requisites (`require` / `watch` / `onchanges` —
  ordering and reactivity), a complete worked example (user + package +
  file + service), and the `apply` / `check` / `drift --fix` / `history` /
  `rollback` workflow. A new **Examples** section maps the in-repo
  blueprints (postgres-ha, monitoring-stack, security-baseline, …) and
  the Starlark module examples (hello → cmdrun → opsbundle) with one-line
  descriptions and links. Together with the auto-generated
  [State Modules](../../modules/) reference this closes the "how do I
  actually use it / what modules are there / where are the examples" gap.

### Changed

- **Release signing deferred to v0.8; v0.1–v0.7 ship unsigned.** The
  `RELEASE-PLAYBOOK.md` §6 carve-out — originally a one-time v0.1.0
  exception — now covers the whole `v0.1`–`v0.7.x` line: every release
  through v0.7 ships unsigned (no signed tags / checksums / SBOMs),
  verified via `sha256sum -c` over TLS-to-forge. Release signing (signed
  artifacts, signed-commit enforcement, and **GPG signatures on the
  apt/dnf repos**) becomes the **v0.8 supply-chain milestone**, landing as
  one key-onboarding batch rather than gating each earlier v0.x cut —
  notably the v0.5 external-tester milestone, which therefore has no
  remaining release blockers. The hosted repos themselves ship at v0.5
  **unsigned** (`apt [trusted=yes]` / `dnf repo_gpgcheck=0`), at the same
  trust level as the unsigned direct downloads. Documented across
  `RELEASE-PLAYBOOK.md` §6 + the release-lines table, `SECURITY.md`,
  `VERSIONING.md`, and the three `ROADMAP.md` signing entries.
- **Standardized the `kscore` vs `keystone-core` naming split and
  documented it.** Runtime / on-disk identity is `kscore` (binaries,
  `/etc/kscore`, `/var/lib/kscore`(`-agent`), systemd units, the
  dedicated system user); project / brand identity is `keystone-core`
  (repo, Go vanity module, release-artifact names, domain, maintainer,
  the `# Managed by keystone-core` markers, the gitops-rollback author).
  The rubric is now codified in `docs/project/GLOSSARY.md`. The agent
  systemd path/unit fix already moved the on-disk paths to `kscore`; this
  cleans up the last stragglers that contradicted the deployed
  convention: the unit generator's stale "recommended user is
  keystone-core" comment (the server package actually creates the
  `kscore` user) and its `keystone-core` test fixtures, the
  `Dedicated keystone-core system user` ROADMAP entry, and a vestigial
  `keystone-core.yaml` dev-config entry in `.gitignore` / `make clean`
  that nothing generates. No behavior change — the shipped packages and
  units already used `kscore`.
- **Five unimplemented REST stub endpoints now return `410 Gone`
  instead of `501 Not Implemented`.** The `agents`, `state`,
  `execution`, `schedule`, and `maintenance` REST handler packages were
  never implemented behind their stubs; they now return `410 Gone` with
  a body pointing callers at the gRPC equivalent (`ControlPlaneService`
  / `StateService`) where one exists, or a post-v1.0 marker where the
  domain itself ships later. The operator path for agents is served by
  `kscorectl agent list`. If a concrete REST consumer surfaces, the
  decision is to implement passthrough rather than silently un-410-ing
  the routes.

### Fixed

- **Agent bootstrap and systemd unit now agree on the config path.** The
  bootstrap installer rendered the agent config to
  `/etc/keystone-core/keystone-core-agent.yaml` while the generated
  systemd unit (and the shipped package unit) started the agent with
  `--config /etc/kscore/agent.yaml` — so a default `kscore-agent
  bootstrap` followed by `kscore-agent service install` produced a unit
  pointing at a config file that bootstrap never wrote, and the agent
  failed to find its config. The bootstrap subsystem now uses the same
  `/etc/kscore/agent.yaml` everything else does, with the FSM state file
  at `/var/lib/kscore-agent/bootstrap.json` (the agent's own state dir,
  writable under the hardened unit's `ReadWritePaths`). The config-path
  default is collapsed to a single source of truth
  (`bootstrap.DefaultAgentConfigPath`), and a regression test enforces
  `bootstrap.DefaultAgentConfigPath == systemd.DefaultConfigPath` so the
  two decoupled packages cannot drift apart again. The generated unit
  name is also aligned to `kscore-agent.service` to match the
  package-shipped/enabled unit.
- **Outbound webhooks now circuit-break failing endpoints (Epic 16
  §4.14).** `kscore-server` boot constructed the outbound webhook
  delivery `Manager` with a bare HTTP dispatcher, so the per-endpoint
  circuit breaker (5 failures / 30s open / 2 half-open successes) was
  never active — a persistently-failing receiver was retried on every
  delivery instead of being short-circuited. Boot now wraps the
  dispatcher in the breaker as the §4.14 spec requires. No
  configuration change.
- **nftables backend is now idempotent on nft versions that emit chain
  handles.** The backend lists a chain with `nft --handle list chain`,
  which annotates the chain-opening line with a `# handle N` comment.
  The parser only entered a chain when the line ended in an open brace,
  so the annotated `chain input { # handle 1` form was never entered,
  zero rules were parsed, and the module re-added a rule it had just
  added on every run (e.g. nft 1.0.6 on Debian 12). The parser now
  strips a trailing handle comment before the brace check, so both the
  plain and `--handle` forms parse.
- **firewalld rich rules are now idempotent.** The `firewalld` module
  checked rich-rule presence with `--query-rich-rule` against the
  operator's verbatim string, but firewalld stores rich rules in a
  normalised form — so a rule written with different whitespace,
  attribute quoting, or attribute order read as absent and was re-added
  on every apply. Check now lists the zone's stored rich rules and
  compares both the declared rule and each stored rule by canonical form
  (a quote-aware syntactic normaliser), so a re-formatted rule
  converges. Values firewalld itself rewrites (e.g. MAC case, CIDR form)
  must still be written as firewalld stores them. This resolves the
  gate-v0.5 firewalld caveat.
- **Command-completion audit records now carry the requesting
  principal.** When an agent reported a command result, the
  control-plane's terminal audit callback fired with an empty principal
  because the dispatch-time identity was stamped into the NATS message
  but never persisted on the command record — so the audit log lost the
  actor that requested the dispatch. The `commands` table gains a
  `principal` column (added via an idempotent inline migration on both
  Postgres and SQLite), populated on dispatch and surfaced on both the
  result and timeout-sweep terminal paths. `Principal` (the operator /
  SPIFFE identity that requested the dispatch) is documented as distinct
  from `User` (the OS user the command runs as on the host).
- **Docs site no longer renders a doubled page title.** Every mounted
  reference / operations / ADR page showed its title twice (e.g.
  `/docs/reference/versioning/` rendered "Versioning" then "Versioning"
  again) — the Hextra theme emits the cascade-curated page title as an
  `<h1>`, and the mounted canonical Markdown then re-rendered its own
  `# Title` H1. A new `render-heading.html` hook suppresses content-level
  H1s, so the authoritative cascade title (which also drives the sidebar
  nav and fixes source casing — e.g. `# API reference` → "API Reference")
  is the only one shown. H2–H6 render unchanged with their anchors. The
  canonical files keep their `# Title` for GitHub viewing (mount-not
  -migrate).
- **Quickstart `state apply` command corrected.** `GETTING-STARTED.md`
  showed `kscorectl state apply --agent <id> --file <file>`, but the state
  file is a positional argument and there is no `--file` flag — so the
  command as written failed. Corrected to
  `kscorectl state apply <file> --agent <id>`.

### Docs

- **v0.1.0 release prep landed.** Aggregated 42 changie fragments into
  `CHANGELOG.md`'s `[v0.1.0]` section via `make changelog-batch
  VERSION=v0.1.0` and hand-merged the by-epic Highlights narrative +
  audit-mode callout + Known limitations + Migration + Verification +
  Acknowledgments sections (the changie `merge` step doesn't preserve
  these; a follow-up `headerPath` ROADMAP entry tracks the fix).
  Epic 19 acceptance criteria walked and reconciled against the v0.1.0
  release line — most lines ticked, soak-test + cluster-round-trip
  annotated as v1.0-scope, rc cycle (tasks 14/15/16) retargeted to
  v1.0.0. Three new v0.x ROADMAP entries added with release-order
  mirrors: "Release signing ceremony" (target v0.2.0; v0.1.0 ships
  unsigned), "Native package repositories — APT, DNF/YUM", and
  "Changie configuration: add headerPath" (so future `changie merge`
  doesn't wipe the persistent top-of-file content). `RELEASE-PLAYBOOK.md`
  §6 carries a v0.1.0-only carve-out callout for the unsigned posture
  and §3 carries a pre-filled v0.1.0 release record template. `SECURITY.md`
  "Supply Chain Security & Release Verification" updated for the
  unsigned-v0.1.0 / signed-v0.2.0+ split.
- **Marked the v0.5 external-tester gate as met across the docs.** The
  README project-status now states the v0.5 gate checklist is complete
  (with the v0.5.0 release itself pending the signing ceremony); Epic 08's
  last open acceptance box — the cross-distro Docker matrix — is checked
  now that the live 8-distro matrix runs every applicable module; and
  `FEATURES.md` corrects the stale `gate-v0.5` tag on hosted package-repo
  generation to `v0.x` (it and the signing batch were re-bucketed off the
  v0.5 gate on 2026-06-19). Epics 09 and 11 were already fully checked.
- **State module support matrix added — the v0.5 "what works / what
  doesn't" doc.** `docs/project/STATE-SUPPORT-MATRIX.md` lists every
  state-file parameter across all 36 stdlib modules with a per-parameter
  maturity status (`stable` / `experimental`), a module-maturity-at-a-glance
  table, and per-module "not yet supported (planned, #NN)" pointers into the
  open `gate-v0.5` / `v0.x` backlog. Status reflects code reality and live
  cross-distro-matrix coverage: `package` apt/dnf/apk and `service`
  systemd/openrc are stable, while zypper/pacman and sysvinit backends,
  `security` (SELinux/AppArmor) and `langpkg` are experimental. Satisfies the
  v0.5 gate's Documentation checklist item in `VERSIONING.md`, which now links
  the doc; also indexed in `AGENTS.md` §7.
- **v0.1.0 release follow-ups landed.** Three items bundled in one PR
  per the deferred-from-release-cut list:
  
  1. **RELEASE-PLAYBOOK.md §3 v0.1.0 release record filled in** with
     the actual timestamps and verification evidence from the v0.1.0
     ceremony (build commit 8a48da100; tag pushed 00:00 UTC 2026-05-28;
     release id 9667643; published 00:03:08 UTC; third-machine
     verification with sha256sum -c + container install smoke in
     debian:12-slim + rockylinux:9 — all PASS).
  2. **`syft` v1.44.0 wired into `make install-tools`** alongside the
     other supply-chain tools. Pinned to v1.44.0 (the version used for
     the v0.1.0 SBOMs); future releases get syft via the standard
     install-tools path instead of ad-hoc `go install`. Pin rationale
     documented inline in the Makefile.
  3. **Changie `headerPath` fix.** `.changie.yaml` now references
     `.changes/header.tpl.md` containing the persistent top-of-file
     content (Changelog header + Keep-a-Changelog preamble +
     `[Unreleased]` + `[v1.0.0] — Planned`). Without this, every
     `make changelog-batch VERSION=v0.x.y` rewrote CHANGELOG.md with
     only the per-version files, losing the header. `.changes/v0.1.0.md`
     now contains the complete v0.1.0 section including the
     Markdown reference links at the bottom; the merge round-trips
     to a byte-identical CHANGELOG.md (verified by diff vs pre-merge
     backup). Reference-link automation via `replacements:` is the
     remaining ROADMAP entry under "Changie configuration:
     `replacements` for reference-link maintenance".
- **Operations runbooks: corrected CLI interface drift (accuracy sweep,
  phase 1).** The `docs/runbooks/` procedures referenced binaries,
  subcommands, and flags that shifted during reconstruction; the
  mechanical renames are now fixed so the documented commands actually
  run. Audited every command against the real `--help` output:
  `kscore-cli` -> `kscorectl` (package/binary name); `kscorectl audit|events|identity`
  -> the standalone `kscore-audit|kscore-events|kscore-identity` binaries
  (kscorectl only carries agent/exec/state); `kscorectl cluster health`
  -> `kscore-cluster status` (plus members/rebalance/remove via the
  `kscore-cluster` binary); `kscorectl agents` -> `kscorectl agent`;
  `events stream` -> `events subscribe`; `identity ca rotate --force` ->
  `ca rotate-signing`; `kscore-cluster-backup verify <path>` ->
  `verify --input <path>`; `--status online` -> `--status connected`;
  `kscorectl version` -> `kscorectl --version`. A follow-up phase covers
  the runbook procedures that depend on capabilities not present in v0.1
  (cluster drain/undrain, maintenance mode, federation, encrypted/
  incremental backups, config-key namespaces).
- **Operations runbooks accuracy sweep, phase 2.** The runbooks now
  document only what v0.1 actually ships, and reference the real commands:
  the new `kscorectl agent quarantine`/`unquarantine` (incident isolation)
  and `agent verify --all` (post-CA-rotation cert check) replace the
  fictional `agents quarantine`/`agents verify`; encrypted/portable backup
  and restore are re-pointed to the real `kscore-backup` binary (the runbook
  had conflated it with `kscore-cluster-backup`, which only does
  unencrypted cluster snapshots) and `kscore-bootstrap restore/import` (which
  never existed) is corrected to the real restore commands; the `database:`
  config blocks become the real `storage: {driver, dsn}` schema. Capabilities
  that genuinely don't exist in v0.1 — maintenance mode, scheduled jobs,
  multi-cluster federation, cluster drain/undrain, per-agent cert
  regeneration, and incremental backups — are now honest scope-note callouts
  pointing at the roadmap (with the real v0.1 workaround where one exists,
  e.g. `kscore-cluster remove` for node eviction). All remaining on-disk
  paths are normalized to the ratified `kscore` convention across the
  runbooks plus DESIGN / E2E-VM-TESTING / DEVELOPMENT and the epic-06 notes.
- **Keystone logo added to the README and docs site.** The canonical
  brand logo (`assets/logo.svg` + `assets/logo.png`) now appears
  centered atop the README and as the Hextra navbar logo sitewide,
  replacing the theme's stock placeholder. The logo is mounted, not
  copied: `docs/hugo.toml` mounts repo-root `assets/` into the Hugo
  static tree (`static/keystone`) so it is served verbatim from the
  single source of truth — `SITE.md` documents the mount.

## [v0.1.0] — 2026-05-27

First public release of the post-reset codebase. **Linux-only,
`v0.x`-quality.** The reconstruction baseline established on 2026-05-05
landed Epics 01–18 in full and Epic 19's release-hardening tasks 1–13;
the v1.0 rc cycle (Epic 19 tasks 14–16) is retargeted to v1.0.0.
v0.1.0 is the first release on the v0.x line — the "genuinely try-able"
cut shipped to curious operators and early adopters per
[`docs/project/VERSIONING.md`](docs/project/VERSIONING.md). **Expect
breaking changes between minor versions** (minimised, always with a
migration note). The formal external-tester milestone is the v0.5
checklist; the SemVer stability commitment begins at v1.0.

Implementation tracked in [`epics/`](epics/); ranked backlog in
[`docs/project/ROADMAP.md`](docs/project/ROADMAP.md). The prior
implementation is not preserved in this repository — the reconstruction
reset on 2026-05-05 was a clean break.

### Notable behavior — the policy engine is AUDIT-MODE-ONLY

> **Read this before relying on policy.** The policy engine evaluates,
> audits, and reports — but **it does not block anything**. Even when a
> policy returns `Allowed=false`, the operation **still proceeds**.
> `policy.enforcement_enabled` is hardcoded `false` and is not
> operator-settable on the `v0.x → v1.0` line.

This is intentional: full enforcement carries breaking-change risk (a
misconfigured policy could block the fleet), so v1.0 ships policy as
*observability* — run real policies against real workloads, inspect what
*would* have been blocked via the audit trail / `WouldDeny`, and build
confidence first. **A future post-v1.0 release flips enforcement on
and is a behavior-changing release** — policies left at
`EnforcementMode=enforce` will start blocking at that point. See
[`docs/project/POLICY-AUDIT.md`](docs/project/POLICY-AUDIT.md)
for the full audit-mode contract and the v1.0 → enabling-enforcement
migration steps.

The audit log itself is fully live: every sensitive op (auth, secret
access, command exec, state apply, policy eval) writes an `AuditEntry`.

### Highlights — what shipped in v0.1.0

Grouped by epic. Each entry names the deliverable; the linked epic file
carries the per-task acceptance details. Some entries include a v0.1.0
caveat (this release line allows breaking changes); the v0.5 + v1.0 gates
in [`docs/project/VERSIONING.md`](docs/project/VERSIONING.md) are the
graduation targets.

- **Foundations** ([epic 01](epics/01-foundations.md)) — `pkg/{version,semver,wait,dbutil}`, `internal/{config,logging,cli}`, `pkg/api/{apierror,v1}` (proto codegen), `Makefile`-driven workflow (`make build`, `test`, `lint`, `proto`, `release-snapshot`), cross-compile matrix (linux/{amd64,arm64} + darwin/{amd64,arm64} + windows/{amd64,arm64}; pure Go, no CGO), CI on Forgejo Actions + Codeberg Woodpecker.

- **Storage layer** ([epic 02](epics/02-storage-layer.md)) — `internal/state` with SQLite + PostgreSQL backends. `Store` interface composes per-domain sub-interfaces (Agent, Command, BatchJob, APIKey, Health, State, Saga, Secrets, Events, Audit, Cluster, etc.). Migrator runs on `Open` and supports forward-only migrations + rollback transactions. SQLite store ships with sane PRAGMAs (`journal_mode=WAL`, `foreign_keys=ON`).

- **API surface** ([epic 03](epics/03-api-surface.md)) — gRPC + REST proto schemas covering 13 services. Auth chain (`pkg/api/auth`: APIKey / JWT / mTLS authenticators, RBAC authorizer, sliding-window rate limiter). Per-domain REST handlers, gateway via grpc-gateway. OpenAPI 3.0 spec lints in CI.

- **Control plane core** ([epic 04](epics/04-control-plane-core.md)) — `kscore-server` is a real daemon. `internal/controlplane`: `ConnectionManager`, `CommandDispatcher`, `BatchDispatcher`. `pkg/api/server` runs a 21-step deterministic init, dual-stack listeners, auth middleware chain, `/health/{live,ready,status}` + `/api/status` endpoints. Dev mode auto-generates an admin API key once at boot (printed once; not recoverable).

- **NATS messaging** ([epic 05](epics/05-nats-messaging.md)) — `internal/nats.Manager` (external client + embedded server modes), `SubjectBuilder` with `kscore.{cluster}.…` prefix enforced on both sides, `Envelope` wire format with length-prefixed dedup, per-endpoint circuit breakers, JetStream stream provisioning, server-side bootstrap registration handler with PSK validator + API-key issuer.

- **Agent runtime + bootstrap UX** ([epic 06](epics/06-agent-runtime.md)) — `kscore-agent` is real. Subscribes to its command subject; runs `Executor` (os/exec wrap with SIGTERM-grace-then-SIGKILL, hard-cap output truncation, optional uid switch), `MetadataCollector` (gopsutil-backed; distro / kernel / NIC / virt / CPU / memory / disk), `SecurityEnforcer` (HMAC-SHA-256 + principal/command allowlists + env filter). Drains in-flight commands on SIGTERM. systemd unit + non-interactive bootstrap flags.

- **Remote execution & targeting** ([epic 07](epics/07-remote-execution.md)) — operator-facing dispatch end-to-end. `internal/targeting`: shorthand expression compiler (`expr-lang/expr` + `gobwas/glob`) → `Matcher.Match(AgentRecord)` against flattened metadata; AND-of-labels-plus-hostname-glob today. `internal/execution`: `Executor` interface + `ManagedExecution` (PENDING / RUNNING / COMPLETED / FAILED / TIMEOUT / CANCELLED / RETRYING with `Callbacks` + `RetryPolicy`), `Pipeline` (sequential stages with stdout-piping), `Shell` selectors (bash / sh / powershell / cmd), `CommandPolicy` (`Validate` / `ValidateNoShell` modes). `internal/controlplane.BatchDispatcher.ExecuteBatch` (semaphore concurrency, 500 ms progress ticker, async orchestration detached from request ctx). `kscorectl exec` with `run` / `async` / `script` / `status` / `list` / `cancel` / `output` subcommands + `--dry-run`; table / json / yaml formatters; `--raw` pipe-friendly single-agent output.

- **State management engine + 35 stdlib modules** ([epic 08](epics/08-state-management.md)) — `internal/statemgmt` ships the engine (parse → render → validate → resolve → check / apply / test) plus **35 stdlib modules** across nine categories: system & core / scheduled tasks / storage / network base / firewall / SSH & security / system config / files & VCS / certificates. `pkg/saga` provides aggregate-and-continue compensation that `Runner.RunSaga` wires into the state runner against the `StateHistoryStore`. `StateGRPCServer` implements `ApplyState` (streaming), `CheckState`, `DetectDrift`, `GetStateHistory`, `RollbackState`, `GetStateStatus`. Integration test covers five paths (apply / idempotency / drift / rollback / saga compensation / requisite-cycle error).

- **Identity & auth** ([epic 09](epics/09-identity-auth.md)) — embedded SPIFFE-shaped CA, mTLS-ready join tokens, `kscore-identity` operator CLI, mTLS-on-gRPC default-on. `internal/identity` ships the full surface — `SPIFFEID` / `X509SVID` / `JWTSVID` / `TrustBundle`, two-tier root + signing CA with auto-rotation, `JoinTokenStore` (in-memory + state-backed) with constant-time hash + atomic MaxUses, `JoinTokenAttestor`, `EmbeddedProvider` composing everything behind the `Provider` interface. `IdentityService` gRPC exposes `token {create,list,revoke,cleanup}` + `ca {info,rotate-signing,export}` + `status`. Server-side mTLS derives from the running provider with auto-refresh on signing-CA rotation; `defaultConfig().Server.TLS.Enabled = true`. NATS bootstrap upgrades the epic-05 PSK path to a SPIFFE-aware `JoinTokenBootstrapValidator` + `SVIDBootstrapIssuer`.

- **Secrets management** ([epic 10](epics/10-secrets.md)) — `internal/secrets`: encrypted-file backend (XChaCha20-Poly1305 envelope + Argon2id KDF) and HashiCorp Vault backend (AppRole / Kubernetes / LDAP auth modes). Per-secret KV2 versioning; capability-based access via `pkg/api/secrets` REST + gRPC. `kscore-secrets` CLI: `get` / `put` / `list` / `delete` / `versions` / `rotate`. `SecretsBackend` interface lets new backends slot in without server changes. Audit-mode-only policy can evaluate "what would be denied" on every read.

- **Event system** ([epic 11](epics/11-events.md)) — `internal/events.JetStreamPublisher` (in-process embedded + external JetStream backed) with envelope + length-prefixed dedup + at-least-once delivery. Per-event-type retention policies. `internal/events.Subscriber` ships the pull-consumer side with at-most-once ACK + dead-letter on persistent failure. `EventService` REST + gRPC with `Publish` / `Subscribe` (streaming) / `Replay` / `Tail`.

- **Audit log + policy engine** ([epic 12](epics/12-audit-policy.md)) — `internal/audit.Store` (Postgres + SQLite) records every sensitive op as an immutable `AuditEntry` chained to the prior via SHA-256. `PolicyService` evaluates OPA-style Rego or CEL policies in **audit-mode only** for v1.0; `WouldDeny` enriches the audit trail without blocking. `kscore-audit` + `kscore-policy` CLIs cover query + bundle / load / test workflows. See [`docs/project/POLICY-AUDIT.md`](docs/project/POLICY-AUDIT.md) for the v1.0 → enabling-enforcement migration path.

- **Clustering & HA** ([epic 13](epics/13-clustering-ha.md)) — the v1.0 differentiator. `internal/cluster` ships embedded etcd v3 (`Manager.Mode = embedded`) + external etcd modes. `Membership`, `Leader`, `Shard`, `Routing`, `Health`, `Recovery` subsystems. Server-side `ClusterService` gRPC: `GetClusterStatus`, `ListMembers`, `AddMember`, `RemoveMember`, `GetLeader`, `TransferLeader`, `Rebalance`, `CreateBackup`, `RestoreBackup`, `WatchMembership` + `WatchLeadership` (streaming). `kscore-cluster` + `kscore-cluster-backup` CLIs. Wall-clock SLOs gate the build: first leader <3 s, failover <5 s/10 s, minority-block <1 s, recovery <15 s.

- **Plugin / module system** ([epic 14](epics/14-plugin-module-system.md)) — Starlark module runtime with capability-based sandboxing. Cosign-signed `.kscore-module` bundles; filesystem registry (`KSCORE_MODULE_PATH`). `kscore-module` CLI: `scaffold` / `pack` / `sign` / `verify` / `publish` (filesystem) / `inspect` / `lint` / `test`. `pkg/module/testing` runner harness for offline module tests. `internal/statemgmt/stdlib` modules graduate from in-tree to module-loadable in v1.x.

- **Blueprints + runbooks + saga + state-machine library** ([epic 15](epics/15-blueprints-runbooks.md)) — `internal/blueprint` composes states into reusable bundles (`Blueprint`, `BlueprintLibrary`, `BlueprintBundle` for sign / publish / import). `internal/runbook` runs imperative procedures with checkpoint / resume / rollback via `pkg/saga`. `internal/statemachine` shared FSM library used by GitOps rollback (epic 16) + self-management (epic 18). `kscore-blueprint` + `kscore-runbook` CLIs.

- **GitOps integration + outbound webhooks** ([epic 16](epics/16-gitops-webhooks.md)) — inbound webhook receiver (`webhook` HTTP server on `:8081`) with per-source HMAC / Bearer / `none` auth methods for Argo CD / Flux / GitHub / GitLab. `internal/gitops.Verifier` runs state + drift checks against the post-deploy fleet. `internal/gitops.Rollback` is an FSM-driven recovery loop wired to epic 15's state-machine library. Outbound webhook fan-out (`internal/webhook`) for downstream notifications. `kscore-gitops` + `kscore-webhook` CLIs.

- **Observability** ([epic 17](epics/17-observability.md)) — slog handler with masking layer (regex-based for tokens / API keys / passwords). Prometheus metrics for every domain (`kscore_*_total` / `_seconds` / `_bytes` counters + histograms). OpenTelemetry tracing wired through gRPC + REST handlers; OTLP exporter. Grafana dashboard JSON ships under `deploy/grafana/`. `/health/{live,ready,status}` for orchestrator probes.

- **Self-management + file distribution + rate limiting** ([epic 18](epics/18-self-mgmt-files-ratelimit.md)) — `internal/selfmgmt` covers backup (`kscore-backup` CLI; SQLite + Postgres + JetStream snapshots), restore, upgrade-staging, and seed-bootstrap. `internal/files` is the chunked file-distribution transport with resumable GETs, ACL-gated PUTs, content-addressed cache, `kscore-files` CLI. Token-bucket rate-limit middleware on both HTTP and gRPC with `Retry-After` (delta-seconds) responses; per-key extractors (`api_key`, `principal`, `client_ip`).

- **Test, harden, & release infrastructure** ([epic 19](epics/19-test-harden-release.md)) — release-quality gates for v0.1.0: docker-compose E2E (`make e2e-test`; 11 scenarios), in-process module + secrets + blueprint + self-mgmt + webhook integration suites, in-process HA SLOs (cluster forms <10 s, leader <3 s, failover <10 s), perf SLOs (command latency <100 ms, event throughput >10k/s, batch-10 fan-out <2 s), per-package coverage gates (`make coverage-gate` — critical ≥70%, CLI ≥40%), race detector on every `go test` (`make race-policy`), `goleak` in every integration package (`make goleak-policy`), four-scan security baseline (`make security-{secrets,vulns,sast,licenses}` — gitleaks / govulncheck / gosec / go-licenses; CI-gated), hardening pass (audit tables in [`docs/project/HARDENING-BASELINE.md`](docs/project/HARDENING-BASELINE.md), pprof baseline in [`docs/project/PROFILING-BASELINE.md`](docs/project/PROFILING-BASELINE.md)), auto-generated reference docs (`make docs-sync` + `docs/project/{CLI,CONFIGURATION,API}-REFERENCE.md`), single-signer release ceremony documented in `RELEASE-PLAYBOOK.md` (v0.1.0 ships unsigned as a one-time carve-out; signed releases begin v0.2.0 — see ROADMAP).

### Security

- Upgraded `golang.org/x/net` to v0.55.0 to close GO-2026-5026 (Phase B1,
  commit `43c5590a`).
- Empty `security.hmacsecret` now emits a loud production-mode warning
  (Phase B5 C1, commit `03d511e0`) — operators must set this explicitly
  in production; dev-mode bootstrap UX remains unchanged.
- Join-token base62-prefix bias documented + exec capability allowlist
  semantics tightened (Phase B5 M1+M2, commit `1f0164e0`).
- Webhook handler error leakage, gRPC bypass on misrouted methods, and
  HMAC hex-decode timing — three high-severity audit findings closed
  (Phase B5 H1–H3, commit `7b46b28e`).

### Added

- **SPDX license headers on every hand-written source file**
  (`// SPDX-License-Identifier: Apache-2.0`): 1,332 `.go` files + 6
  `.sh` files. Generated `.pb.go` excluded (already `linters: all` in
  `.golangci.yml`). Enforced going forward by enabling the `goheader`
  linter — new files without the header fail lint. Ecosystem-standard
  posture (Kubernetes / etcd / NATS / CoreDNS / Prometheus all do this).
- **Project logo** (commit `6cd4359d1`). Visible on the Codeberg repo header + ready
  for the documentation site when the Hugo build lands at gate-v0.5.

### Changed

- Debian/RPM packaging: postinst hooks create the `kscore` system user,
  `/etc/kscore`, `/var/lib/kscore`, `/var/log/kscore`, `/run/kscore`;
  auto-generate the HMAC secret; ship a default config so
  `systemctl start kscore-server` works out-of-box (commit `4be8d19d`).
- Binary install path moved from `/usr/local/bin/` to FHS-canonical
  `/usr/bin/` for distro packages (commit `4be8d19d`).
- **Forgejo Actions CI workflow moved from `.github/workflows/` to
  `.forgejo/workflows/`** with `runs-on:` switched from `ubuntu-latest`
  to `docker` across all 12 jobs. Triggered by the Codeberg-hosted
  Actions runner coming online — Codeberg's pool advertises the
  `docker` label, and `.forgejo/workflows/` is Forgejo's
  first-preference workflow lookup path. Same workflow now drives both
  the local Forgejo runner at `192.168.10.21` and Codeberg's hosted
  runner. The GitHub mirror is code-only — no CI runs there. The
  legacy `.woodpecker/` pipeline was retired in a follow-up (see the
  `.woodpecker/` directory removed entry).
- **CI Go cache restructured to warmer + readers pattern** in
  `.forgejo/workflows/{ci,ci-full}.yml`. `actions/setup-go@v5`'s
  built-in `cache: true` was uploading the Go module + build cache
  at the end of every job — a 4-6 minute single-threaded
  `tar -cf cache.tgz -z` step per job × 11 jobs = ~50 minutes of
  cache-tar overhead per PR run on Codeberg's shared runner.
  Restructured: `lint` (the warmer) writes the cache via
  `actions/cache@v4`; every other Go job uses
  `actions/cache/restore@v4` (read-only) and pays no save cost.
  `docs` + `openapi` skip the cache entirely (neither runs Go code).
  Additionally, installs `zstd` in every cache-using job so
  `actions/cache@v4` auto-detects it and uses
  `tar --use-compress-program zstdmt` (parallel zstd, ~500 MB/s)
  instead of single-threaded `tar -z` gzip (~30-50 MB/s) — a
  ~20-40× speedup for the warmer's save step on a 2 GB cache
  (seconds, not minutes). Total expected savings: ~45-55 min per
  per-PR run; ~60-70 min per push-to-main run.
- **Per-command response timeouts in `test/e2e/perf/slo_test.go` bumped
  `2s → 10s`** (3 sites: single-agent `CommandTimeout`, single-agent
  test-side `ctx` deadline, 10-agent batch `CommandTimeout`). The SLO
  assertion thresholds (`sloCommandLatency = 100ms`, `sloBatchExec =
  2s`) are unchanged — the bump only widens the per-command response
  ceiling so flaky shared-CI hardware doesn't trip the test before the
  assertion stage can run. First seen on the initial Codeberg Actions
  run (run #5, `slo` job) where one agent out of ten hit a 2s response
  timeout while the batch wall-clock measured 18ms (well under the 2s
  SLO).
- **`slo` test flake follow-up: server-side response timeouts bumped
  `5s → 30s` + 10-agent warmup barrier added** in
  `test/e2e/perf/slo_test.go`. The earlier `2s → 10s` per-command
  bump addressed the agent-side timeout but missed
  `DispatcherConfig.DefaultTimeoutSeconds` and
  `NATSBatchExecutorConfig.DefaultTimeout` — both at 5s on the
  server side — which kept tripping on Codeberg's shared runner
  (slo `Failing after 1m49s`, test wall-clock 5.03s matching the
  5s ceiling exactly). All four server-side sites now 30s (two
  per test). New warmup loop in `TestSLO_BatchExec_10Agents`
  serially probes each of the 10 agents with `/bin/true` before
  the parallel measurement, catching subscription-not-yet-routable
  flakes as clean fatal errors instead of measurement noise. SLO
  assertion thresholds remain untouched (median <100ms, batch <2s).
- **`trackerctl --repo` default flipped to Codeberg canonical**
  (`Spicer-Creek-Solutions-LLC/keystone-core`, commit `22ad0fada`). The
  CLI is now Codeberg-ready out of the box; the self-hosted Forgejo
  test path is `--repo sbutts/keystone-core`. Same commit fixed a
  drift in `tools/trackerctl/config/release-order.yaml` where the
  Hugo / error-URL ROADMAP entries had not been mirrored from the
  `e495fb5a` Hugo-pull-forward commit (caught by the trackerctl
  test gate at `tools/trackerctl/tracker_test.go:128` which asserts
  ROADMAP ↔ release-order parity).
- **trackerctl umbrella label renamed `v1x-backlog` → `roadmap-backlog`**
  (commit `7e42bbae3`); the legacy `v1.0-narrowing` marker label is
  retired. Both names predated the v0.x rename (the document was
  renamed `V1X-BACKLOG.md` → `docs/project/ROADMAP.md` earlier); the
  umbrella now reflects the document name with no version pin.
  Applied to the live Codeberg label set during the pre-public-launch
  trackerctl provisioning sweep. The paired source label
  `source/v1x-backlog` still carries the legacy name; renaming it
  is tracked as a v0.x ROADMAP entry to avoid forcing a Forgejo-side
  relabel migration once issues exist.
- **`.woodpecker/` directory removed.** With the Codeberg Actions
  surface green on `main` (the `slo` flake fixed in this same
  release), the legacy Woodpecker pipeline is no longer needed.
  Removes `.woodpecker/build.yml` + `.woodpecker/ci.yml` and
  updates the now-stale `.woodpecker/ci.yml` references in
  `test/e2e/ha/README.md` and `PROJECT-DETAILS.md` § 4.15 to
  point at the canonical `.forgejo/workflows/ci-fast.yml`.
- **Default gRPC server port moved from `9090` → `5397`** to avoid the
  Cockpit collision on Rocky 10 / RHEL 10 (commit `3d482fa1`). Cockpit's
  default `9090` made `apt install kscore-server` fail to start out-of-box
  on Rocky 10; 5397 has no known popular collision. Operators with old
  `kscorectl` defaults pointing at `9090` get `connection refused` and
  pass `--server localhost:5397` to recover.
- **CI workflow split into per-PR fast loop + push-to-main full pipeline.**
  `release-dry-run` (~33min) and 5 of 6 cross-build matrix variants
  (`linux/arm64`, `darwin/amd64`, `darwin/arm64`, `windows/amd64`,
  `windows/arm64`) moved from `ci-fast.yml` into a new
  `.forgejo/workflows/ci-full.yml` triggered by push-to-main +
  nightly cron (05:00 UTC) + `workflow_dispatch`. `linux/amd64`
  build stays in `ci-fast.yml` as the per-PR smoke; the other five still
  gate every main push. Triggered by single-runner serialization on
  the Codeberg-hosted Actions pool — a per-PR run with 17 jobs took
  ~2 hours wall-clock; the split cuts per-PR work to 11 jobs.
- **`.forgejo/workflows/ci.yml` renamed to `ci-fast.yml`** so it sorts
  lexically before `ci-full.yml`. Forgejo Actions enumerates workflow
  files in directory order; with the previous naming, `ci-full.yml`
  (the heavy push-to-main pipeline) was picked up first because `-`
  (0x2D) sorts before `.` (0x2E). That meant `ci-full.yml`'s
  release-dry-run + 5 cross-build variants ran BEFORE `ci-fast.yml`'s
  `lint` cache warmer had a chance to save the Go module + build
  cache, defeating the warmer-readers pattern across workflows. The
  rename makes `ci-fast.yml` (lint + the 10 per-PR readers) sort
  first; `lint` warms the cache; `ci-full.yml`'s readers then hit
  the warm cache on the same push-to-main run. The workflow's
  internal `name:` field stays `ci`, so the PR "Checks" UI is
  unchanged. Cross-references in `ci-full.yml`,
  `docs/project/PUBLIC-LAUNCH-CHECKLIST.md`, and
  `PROJECT-DETAILS.md` § 4.15 updated to the new path.
- **CHANGELOG workflow switched to per-PR YAML fragments** (powered by
  [changie](https://changie.dev)). Per-PR entries now live as separate
  files under `.changes/unreleased/` instead of editing
  `CHANGELOG.md`'s `[Unreleased]` section directly. Eliminates the
  mechanical merge conflicts that hit every concurrent PR touching the
  same section anchor (observed multiple times during the post-Codeberg-
  Actions push: each new PR required a rebase + manual CHANGELOG conflict
  resolution). New Make targets: `make changelog-new` (interactive
  fragment draft), `make changelog-preview` (dry-run accumulated
  section), `make changelog-batch VERSION=v0.x.y` (release-time
  aggregation). All existing `[Unreleased]` entries migrated to
  fragments in this same PR; see
  [CONTRIBUTING.md § Changelog entries](CONTRIBUTING.md#changelog-entries)
  for the per-PR workflow.

### Fixed

- CI: musl `lychee` variant for the Forgejo runner's older glibc
  (commit `af46bb52`); `install-tools` now pulls a pinned lychee binary
  (commit `af4cb9e7`).
- Three timing-sensitive test flakes exposed by the Forgejo runner —
  queue-group `≥1` assumption, observer-vs-state race, NATS
  subscription-flush race (commit `928874b5`).
- State integration tests: TRUNCATE + boundary + JSONB regressions
  uncovered during clean-tree Phase C run (commit `dd7f03b4`).
- CI release-smoke: native-execution fallback when Docker is absent on
  the Forgejo runner image (commit `032219cc`).
- CI: pinned `protoc-gen-go@v1.36.11` + `protoc-gen-go-grpc@v1.6.1` so
  generated stubs don't drift from `@latest` (commit `cbc78351`).

### Docs

- **E5 (repo-root inventory) ticked in the public-launch checklist.**
  Inventory pass found every committed top-level file in scope —
  required docs, Go project files, protobuf tooling, standard
  dotfile configs, and `.safety-net.json` (project-wide AI-agent
  guardrails, consistent with the AI-contributions posture). Local
  dev artifacts (`.claude/`, `.idea/`, `.python-version`,
  `build/`, `dist/`, `kscore-server` binary) verified properly
  gitignored — none appear in clones. No cleanup needed.
- **`docs/project/GETTING-STARTED.md` rewritten** as a guided
  ~30-minute fresh-VM operator tutorial: package install, smoke
  checks, agent online, run a command via `kscorectl exec`, apply
  state via `kscorectl state apply`, browse audit via `kscorectl
  audit log`. Closes the matching v0.x ROADMAP entry. Part of the
  v0.1.x first-impression doc pass.
- **Go vanity-import static-site source** added under
  [`deploy/vanity/`](deploy/vanity/) using
  [vangen](https://github.com/leighmcculloch/vangen) (canonical
  module `4d63.com/vangen`): `vangen.json` config + generated
  `site/keystone-core/index.html` carrying the `go-import` +
  `go-source` meta tags pointing at the Codeberg primary. Two new
  `make` targets (`vanity-regen` + `vanity-regen-check`); vangen
  added to `make install-tools`. Closes the last remaining piece
  of the `keystone-core.io` domain-provisioning story — DNS,
  mailboxes, web hosting, TLS, and key-material hosting are
  operator-side; this file is the code-side preparation. Once
  deployed at `go.keystone-core.io`, the existing
  `go.keystone-core.io/keystone-core` Go module path resolves
  end-to-end for external `go get` users.
- **Hugo docs site pulled forward from v1.x to gate-v0.5**: updates
  AGENTS.md §5, FEATURES.md §1, VERSIONING.md (resolves the prior
  v1.0-gate-7-vs-v1.x-FEATURES contradiction — Hugo is now a v0.5
  gate; v1.0 gates renumbered 8/9/10 → 7/8/9), ROADMAP.md (Hugo
  entry moved from v1.x to gate-v0.5; dependent v1.x entries
  "Expanded getting-started guides" + "Error-message docs URLs"
  re-framed against Hugo's new position). Rationale: a polished,
  searchable doc experience benefits the v0.5 external-tester
  audience; pre-v0.5 Markdown + subtree READMEs remain sufficient
  for the v0.1.x invited-installer audience. PDF export stays
  v1.x (FEATURES.md §1).
- **Markdown lint coverage expanded to repo-root files** (commits
  `c043ef41b`, `f50f6b6a8`). The `.markdownlint-cli2.yaml` glob was
  `docs/**/*.md` only; root files accumulated ~1100 silent errors
  over time. FEATURES.md (840 errors — mostly the
  `_Reasoning: ..._` to `*Reasoning: ...*` MD049 emphasis-style
  sweep, plus 2 broken Domain Index anchor fragments and
  blockquote/list spacing), RELEASE-PLAYBOOK.md (31 errors, mostly
  MD031 blanks-around-fences inside numbered-list steps), AGENTS.md,
  CODE_OF_CONDUCT.md, and CONTRIBUTING.md (7 errors total) all
  cleaned. The root `*.md` pattern added to the glob so future
  drift is caught at PR time. Two carve-outs documented in config:
  `CLAUDE.md` (incompatible with MD041 by design — the file's
  entire content is `@AGENTS.md`, a Claude Code import directive)
  and `PROJECT-DETAILS.md` (~210 deferred-cleanup errors tracked
  as a v0.x ROADMAP entry, mainly MD032 list-spacing and MD029
  ordered-list-prefix style call).
- NOTICE accuracy audit: dropped `wazero`, added 8 notable deps
  (HashiCorp Vault, gRPC, OpenTelemetry, Prometheus client,
  SPIFFE go-spiffe, minio-go, modernc.org/sqlite, go-git), reorganized
  by domain, documented the `modernc.org/mathutil` "Unknown" license
  exception inline (commit `33f19178`).
- **Pre-public-launch hygiene** (commit `3328945bc`): all
  `archive/v0` branch and `archive/v0-final` tag references dropped
  from committed content. The reset-baseline acknowledgment in
  AGENTS.md / README / CHANGELOG / PUBLIC-LAUNCH-CHECKLIST.md
  remains, but no committed text points at deleted refs. Test
  fixture IP `192.168.10.4` (in `internal/targeting/matcher_test.go`)
  swapped for `192.0.2.4` (RFC5737 documentation range) — caught
  during the gitleaks + internal-IP grep audit pass.
- Public-launch checklist Phases A–D ticked across 4 commits
  (`524757a0` → `9622ed36`): code-vs-docs sync, link health, epic
  acceptance audit, security baseline + dummy-report-flow doc, threat-
  model refresh, clean-tree CI gates green, six-VM cross-distro
  environment validation (debian12 / ubuntu22 / ubuntu24 / rocky8 /
  rocky9 / rocky10).
- **`README.md` Quickstart rewritten** around the `apt install` /
  `systemctl` / `kscorectl` operator path (was `git clone`,
  `make e2e-up`, and grpcurl). Mirrors
  [`docs/runbooks/bootstrap-new-cluster.md`](docs/runbooks/bootstrap-new-cluster.md).
  Part of the v0.1.x first-impression doc pass.
- **FEATURES.md Domain Index status markers refreshed** (commit
  `203cafa0b`). 15 of 20 domains were misleadingly marked `*(pending)*`
  in the index — that marker was a pre-reset planning convention
  meaning "rebuild has not started yet" relative to the reconstruction
  baseline. Epics 01-19 closed and rebuilt 13 of those 15 domains;
  the index now uses `*(landed)*` and `*(landed, with gaps)*` to
  honestly reflect epic-close status, with a one-line preamble
  pointing readers at `docs/project/ROADMAP.md` for the
  authoritative gate-v1.0 deferral list. The §6 Agent Runtime body
  has always described a working `kscore-agent` daemon; the index
  now matches.
- **F4 release-incident response plan** lands at new
  [`docs/project/RELEASE-INCIDENT.md`](docs/project/RELEASE-INCIDENT.md).
  Covers the post-publication decision tree (yank vs patch vs
  communicate-only), yank procedure, fast follow-up release
  numbering + communication, and post-incident process (CHANGELOG,
  post-mortem, process change). Kept distinct from
  INCIDENT-RESPONSE.md (production security incidents) and
  RELEASE-PLAYBOOK § 14 (expedited release ceremony) which it
  cross-references. v0.1.x-specific: operator-distributed-package
  reality shapes the "yank" mechanics (no public APT/DNF repo to
  withdraw from yet).
- **F3 triage SLO commitment** documented in
  [`docs/project/MAINTAINERS.md`](docs/project/MAINTAINERS.md) §
  Triage and Response. v0.1.x posture: best-effort, no formal SLO;
  rough cadences stated for issues / PRs / security reports;
  cadences reassessed at v0.5 (formal SLO possible from v0.5+).
- **F1 soft-launch decision** for v0.1.x recorded durably in
  [`docs/project/GOVERNANCE.md`](docs/project/GOVERNANCE.md) § Launch
  Posture; F1 ticked in the public-launch checklist; F2
  (announcement draft) marked not-applicable for v0.1.x.
- **NOTICE accuracy audit (E4 of public-launch checklist): MPL-2.0
  attribution generalized to cover all 14 MPL-2.0 deps shipped with
  the binaries.** Previous NOTICE called out only `hashicorp/vault/api`
  by name; `go-licenses report ./cmd/...` confirmed 13 additional
  MPL-2.0 deps require the same library-client attribution: Vault
  auth submodules (approle / kubernetes / ldap), Vault transitive
  HashiCorp utilities (errwrap, go-cleanhttp, go-multierror,
  go-retryablehttp, go-rootcerts, go-secure-stdlib/{parseutil,strutil},
  go-sockaddr, hcl), and the unrelated `cyphar/filepath-securejoin`.
  Same library-consumer logic — referencing each upstream LICENSE
  satisfies the MPL-2.0 attribution requirement. All 13 entries in
  the "Notable dependencies" list verified present in the report
  (no stale entries to remove). `modernc.org/mathutil` "Unknown"
  exception unchanged (already documented).
- **E2 Codeberg / Forgejo templates** land under
  [`.forgejo/`](.forgejo/) (Forgejo's first-preference lookup path;
  Codeberg picks them up automatically). Four issue templates
  (bug, feature, documentation, security-redirect [new]) +
  `config.yml` disabling blank issues + a PR template audited
  against AGENTS.md § 5 (tests-required, docs-updated,
  SPDX-header reminder, AI disclosure, DCO sign-off). The
  parallel `.github/` template duplicates are deleted —
  only a tightened `.github/ISSUE_TEMPLATE/config.yml` redirect
  (now with `blank_issues_enabled: false`) is kept. Closes E2 in
  the public-launch checklist.
- **E1 (Codeberg repo settings) ticked — Phase E complete.** Audit
  doc landed at `docs/project/CODEBERG-SETTINGS-AUDIT.md` as the
  in-repo source of truth for Codeberg-side configuration (metadata,
  feature toggles, merge methods, branch protection). Branch
  protection on `main` enforces 11 required status-check contexts
  (every `ci-fast.yml` job), blocks force-push + deletion, requires
  up-to-date branches. DCO sign-off now enforced by a new
  `dco-check` job in `.forgejo/workflows/ci-fast.yml` that walks
  every non-merge PR commit and fails on a missing
  `Signed-off-by:` trailer (replaces honor-system enforcement via
  the PR template). Required-signed-commits enforcement deferred to
  v0.x — tracked in `ROADMAP.md` "Phase E1: required signed commits".
  Packages + Projects features disabled; merge methods tightened so
  every PR landing on `main` produces a merge commit (squash and
  plain rebase both off). Phase E (E1–E5) entirely closed; only
  Phase G (documentation site) remains.
- **`docs.keystone-core.io` placeholder site source** added under
  [`deploy/docs/`](deploy/docs/) — small branded HTML page that
  links visitors at the canonical Markdown docs in the source
  repository, with a "polished site coming with v0.5" note. Hosted
  same shape as the Go vanity-import site (catch-all rewrite to
  `index.html` for any path under `/`). At gate-v0.5, this
  placeholder is replaced by the full Hugo-generated output per
  the matching ROADMAP entry.
- **Docker-compose dev topology + grpcurl walkthrough relocated**
  from `README.md` to `docs/project/DEVELOPMENT.md` § Local Dev
  Topology, where it belongs as contributor onboarding. Part of the
  v0.1.x first-impression doc pass.

### Known limitations for v0.1.0

The v0.5 + v1.0 gates in [`docs/project/VERSIONING.md`](docs/project/VERSIONING.md)
are the graduation targets. Items shipping in v0.1.0 with reduced
coverage or behind a v0.x flag:

- **Release artifacts ship UNSIGNED.** v0.1.0 is a one-time carve-out:
  no signed tag, no signed `checksums.txt`, no signed SBOM. Trust
  model collapses to TLS-to-codeberg.org + Codeberg's auth + manual
  `sha256sum -c` verification. **Signed releases land in v0.2.0**;
  see the `Release signing ceremony` entry in
  [`docs/project/ROADMAP.md`](docs/project/ROADMAP.md) for the
  graduation plan + the RELEASE-PLAYBOOK §6 v0.1.0-only callout.
- **Native package repositories (APT / DNF / YUM) not hosted.** `.deb`
  and `.rpm` packages are produced by the release ceremony and
  attached to the Codeberg release page as direct downloads;
  operators `dpkg -i` / `rpm -i` the file directly. A hosted apt /
  dnf / zypper repo with signed indices lands before v1.0 — tracked
  in [`docs/project/ROADMAP.md`](docs/project/ROADMAP.md) as
  `Native package repositories — APT, DNF/YUM`.
- **No container images.** No `dockers:` section in
  `.goreleaser.yaml`, no `Dockerfile` for the binaries. Operators
  who want containers build their own from the `.deb` / `.rpm` or
  the binary archives. Container shipping is post-v1.0.
- **Cross-distro CI matrix is in-progress.** State stdlib runs against Debian 12 / Ubuntu 22.04+24.04 / Rocky 9 / Alpine 3.19 locally; the v0.5 checklist expands the matrix.
- **`package` module ships apt + dnf** as the v0.1 backends; **apk + zypper** graduate at v0.5.
- **Network base: firewalld rich-rule canonicalisation deferred** to v0.5 per [`docs/project/ROADMAP.md`](docs/project/ROADMAP.md).
- **Persistence renderers** for networking (NetworkManager, netplan, systemd-networkd) — limited coverage in v0.1; full set graduates at v0.5.
- **Policy enforcement** is locked at audit-mode for the entire v0.x → v1.0 line per [`docs/project/POLICY-AUDIT.md`](docs/project/POLICY-AUDIT.md); enforcement flips post-v1.0.
- **Hugo docs site** — planned for v0.5 (pulled forward from a former v1.x position; see [`docs/project/VERSIONING.md`](docs/project/VERSIONING.md) § v0.5 gate § Documentation). v0.1.x ships Markdown references under `docs/project/` with curated subtree-index `README.md` files for navigation.
- **Multi-party release signing** — v1.2 graduation per [`RELEASE-PLAYBOOK.md`](RELEASE-PLAYBOOK.md). v0.x / v1.0 / v1.1 ship under the single-signer ceremony (v0.1.0 ships unsigned per above; v0.2.0+ ships single-signer-signed).
- **Windows + macOS agents, WASM modules, full SPIRE, Kubernetes operator, federation, web UI, blueprint marketplace** — explicitly post-v1.0 (epic 19 §Scope out).
- **gRPC server reflection disabled in dev** — the workaround (pass `-import-path api/proto -proto api/proto/keystone/core/v1/controlplane.proto` to grpcurl, or use the REST surface) is documented in [`docs/project/DEVELOPMENT.md`](docs/project/DEVELOPMENT.md) § Local Dev Topology.
- **gosec G115 (integer overflow conversion) excluded project-wide** per [`docs/project/SECURITY-GOVERNANCE.md`](docs/project/SECURITY-GOVERNANCE.md) "Security Baseline Pipeline." v1.x ROADMAP entry "Security baseline expansion" tracks the per-site re-audit.
- **Per-domain sustained-load profiling, 1-hour fd-leak soak, docs-URL injection in errors, context-aware threading of 122 deep-helper log sites** — tracked as v1.x ROADMAP entries; the v1.0 baseline is in [`docs/project/PROFILING-BASELINE.md`](docs/project/PROFILING-BASELINE.md) + [`docs/project/HARDENING-BASELINE.md`](docs/project/HARDENING-BASELINE.md).

### Breaking changes

This is the v0.x line — breaking changes between minor versions are
expected. Each future release will list its breaking changes under a
`### Breaking changes` section here, with a migration note in the
release announcement and (when possible) a one-cycle deprecation
period. **For v0.1.0 specifically: no breaking changes — this is the
first release.**

### Migration

**First release.** No upgrade path applies. Operators evaluating
v0.1.0 should follow [`docs/project/GETTING-STARTED.md`](docs/project/GETTING-STARTED.md)
end-to-end on a fresh VM. The reconstruction reset on 2026-05-05 was
a clean break from the prior implementation; the pre-reset code is
not preserved in this repository and is not a migration source.

### Verification

**v0.1.0 ships unsigned** as an explicit one-time carve-out. Signed
releases begin v0.2.0 — see the `Release signing ceremony` entry in
[`docs/project/ROADMAP.md`](docs/project/ROADMAP.md) for the
graduation plan. RELEASE-PLAYBOOK §6 (Signing) carries a matching
v0.1.0-only callout near the top.

End-user verification for v0.1.0:

1. Download the archive (or `.deb` / `.rpm`) for your platform from
   the [v0.1.0 release page](https://codeberg.org/Spicer-Creek-Solutions-LLC/keystone-core/releases/tag/v0.1.0).
2. Download the matching `checksums.txt` from the same release page.
3. Verify artifact integrity against checksums:

   ```bash
   sha256sum -c checksums.txt
   ```

**Trust model for v0.1.0:** TLS connection to codeberg.org + Codeberg's
own infrastructure authentication. The `checksums.txt` catches transport
corruption and single-artifact-swap attacks, but without a signature it
cannot authenticate against a forge-side compromise. v0.2.0 closes that
gap with signed `checksums.txt` + signed tag + signed SBOM.

Audience for v0.1.0 is curious operators and early adopters explicitly
invited to install per the soft-launch posture in
[`docs/project/GOVERNANCE.md`](docs/project/GOVERNANCE.md) § Launch
Posture; the security tradeoff is documented and intentional.

Multi-party verification (`.sig.A` / `.sig.B` / `.sig.C` suffixes) is
deferred to v1.2 onward — see the playbook's v1.2 graduation checklist
for the signer-onboarding path.

### Acknowledgments

Project sponsor: **Spicer Creek Solutions LLC** ([`OWNERSHIP.md`](OWNERSHIP.md)).

Contribution policy: [`CONTRIBUTING.md`](CONTRIBUTING.md) (DCO
sign-off required on every commit). AI-assisted contributions are
disclosed per [`docs/project/AI-CONTRIBUTIONS.md`](docs/project/AI-CONTRIBUTIONS.md);
the reconstruction was done substantially with AI assistance and the
practice is documented openly.

Public hosting (primary): [`codeberg.org/Spicer-Creek-Solutions-LLC/keystone-core`](https://codeberg.org/Spicer-Creek-Solutions-LLC/keystone-core).
GitHub mirror for discoverability: [`github.com/Spicer-Creek-Solutions-LLC/keystone-core`](https://github.com/Spicer-Creek-Solutions-LLC/keystone-core).

Issues, RFCs, and discussion live on Codeberg.

[Unreleased]: https://codeberg.org/Spicer-Creek-Solutions-LLC/keystone-core/compare/v0.5.0...HEAD
[v0.5.0]: https://codeberg.org/Spicer-Creek-Solutions-LLC/keystone-core/compare/v0.1.0...v0.5.0
[v0.1.0]: https://codeberg.org/Spicer-Creek-Solutions-LLC/keystone-core/releases/tag/v0.1.0
[v1.0.0]: https://codeberg.org/Spicer-Creek-Solutions-LLC/keystone-core/releases/tag/v1.0.0
