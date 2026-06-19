# Versioning

How Keystone Core numbers releases, what each milestone signals, and what's frozen vs. fair-game at each stage.

This document is canonical. When other docs (README, PROJECT-DETAILS, epics, V1X / ROADMAP entries, module doc-comments) talk about versioning, they defer here.

## Scheme

Keystone Core follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html) with three milestone tiers before v1.0:

| Version line | What it means |
|---|---|
| **`v0.1.x`** | First public release. **Linux-only**. Epics 01–08 complete. **Genuinely try-able**: curious operators and early adopters are explicitly invited to install it, run it, and give feedback — expect rough edges and **breaking changes between patch releases** (minimised, always with a migration note). It is *not* the formal external-tester hand-off — that bar is the v0.5 checklist below — but it should be useful and interesting enough to attract people to the project. |
| **`v0.2.x` – `v0.4.x`** | Incremental releases as features land. Each release closes one or more epics in some form. Cadence is **release-as-ready** — no calendar-pre-allocated scope. Breaking changes between minor versions are expected. The v0.x line stays attractive and try-able throughout. |
| **`v0.5.x`** | **Formal external-tester milestone** (a checklist, not a calendar point — see the v0.5 gate below). Cross-Linux-distribution support solid; persistence renderers for the things early adopters actually need (networking, firewall backends); enough of Epics 09–13 to be usable as a multi-user/multi-host control plane. The first release we *formally* hand to operators outside the dev loop with a "what works / what doesn't" guarantee. |
| **`v0.6.x` – `v0.9.x`** | Polish + remaining epic work toward v1.0. Same release-as-ready cadence. Breaking changes possible but rare; each is documented with a migration note. |
| **`v1.0.0`** | **All 19 epics complete and acceptance-tested. API + CLI + state-file YAML + gRPC frozen.** SemVer stability commitment begins here: post-v1.0 minor releases add features, never break existing ones. |
| **`v1.x`** | Feature additions on the stable v1.0 line. |
| **`v2.0` / `v2.x`** | Federation, multi-region, supercluster, cloud KMS. Identity unchanged from the original roadmap — distant. |
| **`v3.0+`** | Marketplace, web UI, multi-cloud. |

## v0.5 gate — "external-tester ready, all Linux"

The v0.5 milestone is not a calendar point; it's a checklist. v0.5.0 ships when every item below is verifiable on the cross-distro CI matrix.

### Platform support

1. **All major Linux distribution families pass the cross-distro CI matrix** for applicable stdlib modules:
   - Debian 12, Ubuntu 22.04 + 24.04
   - RHEL 9, Rocky 9
   - Alpine 3.19
   - openSUSE (Leap or Tumbleweed) — *stretch target; may slip to v0.6*
2. **Idempotency hammered**: same state file applies twice produces zero changes on every supported distro for every applicable module.

### Network base (the biggest current gap)

1. **At least two persistence renderers** for `network` / `route` / `bond` / `bridge` / `vlan`:
   - **netplan** (Ubuntu's default)
   - **systemd-networkd** (cross-distro)
   - NetworkManager / RHEL `ifcfg-*` may follow in v0.6+.
2. Persistence is opt-in via `persist: <renderer>|auto` on each network-base decl.

### Firewall base

1. **All four firewall modules verified on every distro** where each is native:
   - `iptables` — every distro.
   - `nftables` — every distro (modern kernels).
   - `firewalld` — RHEL/Rocky/Fedora native; runs on Debian/Ubuntu when installed.
   - `firewall` (abstraction) — auto-detects and dispatches correctly; explicit `backend:` override works.
2. **firewalld rich-rule canonicalisation** in the firewalld module (today documented as a v0.x backlog item — required for v0.5 because every RHEL host depends on it).

### Package management

1. **`package` module supports apt + dnf + apk** minimum. zypper optional for v0.5 (block on v1.0). v0.1's apt-only single-backend is alpha-only.

### Security

1. **AppArmor backend** in the `security` module — Ubuntu/Debian operators expect it.
2. Both backends behind the same `security` module surface (auto-detect + explicit `backend:` override).

### Reboot + system

1. **Cross-distro reboot detection** in the `system` module:
   - Debian: `/var/run/reboot-required` marker (v0.1).
   - RHEL: `dnf needs-restarting -r` exit-code parsing.
2. **Reboot result delivery across the disconnect** — the kscore-agent coordinates with its reconnect logic to deliver a `system.rebooted` follow-up event after the host comes back.

### Storage

1. **`disk` filesystem-resize** — `resize2fs` + `xfs_growfs` at minimum.
2. **`lvm` VG PV-set + LV resize** — `vgextend` / `vgreduce` for live VGs, `lvextend --resizefs` for filesystems-on-LV.

### Engine + acceptance

1. **All Epic 08 acceptance criteria checked off** (the ones the epic listed but task 13 was scheduled to verify).
2. **Identity + Auth (Epic 09) complete** — testers running multi-user/multi-host need real auth.
3. **At least Events (Epic 11) wired through** — operators need to see what's happening.
4. **Engine coverage >85%**, module coverage >80%.

### Documentation

1. **A "v0.5 — what works, what doesn't" doc** lists every state-file param and each one's status (`stable` / `experimental` / `deprecated`). Operators evaluating v0.5 can know in advance what's safe to depend on. Delivered as [`STATE-SUPPORT-MATRIX.md`](STATE-SUPPORT-MATRIX.md).
2. **Hugo docs site live** under [`docs/`](.). The v0.1.x Markdown surface (`README.md`, `docs/project/*.md`, `docs/runbooks/*.md`) graduates into a Hugo + Docsy site with per-page navigation and full-text search. The auto-generated CLI / config / API references regenerate via `make docs-sync` into the Hugo content tree. *Pulled forward from a former v1.x position because v0.5's "external-tester ready" audience benefits from a polished doc experience; see [`ROADMAP.md`](ROADMAP.md) § Hugo docs site.*

## v1.0 gate — "stable, production-ready"

The v1.0 milestone is the SemVer stability commitment. Once cut, breaking changes require a v2.0.

1. **All 19 epics complete** (`epics/01-*.md` through `epics/19-*.md`) and their acceptance criteria checked off.
2. **API + CLI + state-file YAML + gRPC contracts frozen** for at least one full v0.x cycle before v1.0 cut (i.e. v0.9.x must match v1.0.0 on contracts — no breaking changes in the v0.9→v1.0 jump).
3. **Cross-distro CI matrix green** for the full applicable-module set on all supported distros — same matrix as v0.5, expanded to include zypper/SUSE and any post-v0.5 distros.
4. **Engine coverage >85%, module coverage >80%** across the board (same standard as v0.5; verified at v1.0).
5. **Performance + load test baseline** passing (per Epic 19's harden-and-release acceptance).
6. **Security review + audit complete** (per Epics 12 + 19).
7. **Migration tooling** for any state-file or schema changes between v0.x and v1.0 — operators can move data forward without a manual rewrite.
8. **Outstanding "must-haves" from external testers addressed** — the v0.5+ tester feedback loop closes before v1.0 cut.
9. **CHANGELOG + RELEASE-PLAYBOOK ready** for the long-term v1.x cadence.

> The former gate-7 "Documentation site live" moved to the v0.5 gate (above). All other v1.0 gates renumbered down by one.

## v0.x roadmap

There is **no pre-allocated table of v0.2 contents, v0.3 contents, etc.** Pre-allocating that table is what got us into the original "v1.0 contains everything" problem.

Instead:

- One ranked roadmap in [`docs/project/ROADMAP.md`](ROADMAP.md) (replaces `ROADMAP.md`).
- Each entry carries a **priority**: `gate-v0.5` | `gate-v1.0` | `v0.x` | `v1.x` | `v2.x+` (the taxonomy in [`docs/project/ROADMAP.md`](ROADMAP.md) and `tools/trackerctl`).
- v0.x minor releases are cut when natural feature sets land. A `v0.3.0` might close one epic; a `v0.4.0` might close two and ship a renderer; the boundaries fall where they fall.
- The v0.5 gate is the only intermediate stop where the version specifically *waits* for a checklist.

## Breaking changes pre-v1.0

This is the whole point of staying on v0.x for now:

- **Patch releases (`v0.N.X` → `v0.N.X+1`)** prefer compatibility but may break.
- **Minor releases (`v0.N` → `v0.N+1`)** routinely break.
- Each breaking change ships with:
  - A `CHANGELOG.md` entry under `## Breaking`
  - A migration note in the release announcement
  - When possible, a deprecation period: the prior version warns + continues to work for one minor cycle before removal.
- **Once v1.0 ships, no breaking changes without a v2.0.**

## Implications for downstream consumers

- **Do not pin `keystone-core@latest` in production** before v1.0.
- Pin to exact patch versions (e.g. `keystone-core@v0.3.2`) and read the CHANGELOG before bumping.
- State files written for v0.N should be tested against v0.N+1 before bulk upgrade.
- v0.x → v0.5 is an upgrade path explicitly engineered for; v0.5 → v1.0 is too.

## Cross-references

- Release process: [`RELEASE-PLAYBOOK.md`](../../RELEASE-PLAYBOOK.md)
- Roadmap entries: [`ROADMAP.md`](ROADMAP.md)
- Epic plans: [`epics/00-meta-reconstruction-plan.md`](../../epics/00-meta-reconstruction-plan.md)
- API deprecation tracking: `pkg/api/versioning` package
- The original "single v1.0 with everything in it" framing this scheme replaces: see git history of `PROJECT-DETAILS.md` §6.2 prior to this commit.
