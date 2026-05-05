# Epic 08: State Management Engine + 40-Module Base Stdlib

**Phase**: E • **Estimate**: 4 weeks (engine 1.5w + modules 2.5w, parallel) • **Depends on**: 02, 03, 04, 06 • **Blocks**: 14, 15

## Goal

Declarative state management — apply YAML state files to agents, detect drift, remediate. Salt-Project-shaped UX. Ship the engine plus ~40 universal Linux sysadmin modules ("base stdlib") covering ~90% of daily ops.

## Scope (in)

### Engine

- `internal/statemgmt/` runner pipeline: parse → template render → validate → resolve dependencies (DAG, cycle detection) → topo-sort → check phase → apply phase → test phase → report.
- Module interface (minimal): `Name() string`, `ValidStates() []string`, `Check(ctx, decl) (*ModuleCheckResult, error)`, `Apply(ctx, decl) (*StateResult, error)`, `Test(ctx, decl) (bool, error)`.
- Requisite system: `require`, `require_in`, `watch`, `watch_in`, `prereq`, `prereq_in`, `onchanges`, `onchanges_in`.
- Template rendering via `text/template` with custom filters (`upper`, `lower`, `title`, `trim`, `join`, `split`, `default`).
- Drift detection with severity (none / low / medium / high / critical) and `DriftReport`.
- Cross-platform dispatch via build tags + runtime.GOOS switch in modules.
- History store (extends `internal/state.Store` with `StateHistoryStore` sub-interface).
- Event emission per state apply (`state.apply.start`, `state.change`, `state.apply.done|fail`, `state.drift`).
- Audit emission per state apply.
- Dry-run / check mode.
- gRPC `StateService` impl: ApplyState (stream), CheckState, DetectDrift, GetStateHistory, GetStateStatus.
- `cmd/kscore-state` and `kscorectl state` CLI: `apply`, `check`, `drift [--fix]`, `compile`, `show`, `test`, `history`, `rollback`, `export`, `restore`. `kscorectl vars get`.

### Base stdlib (~40 modules)

Per `PROJECT-DETAILS.md §4.8` — categorized:

| Category | Modules |
|---|---|
| System & core | `file`, `package`, `service`, `user`, `group`, `cmd`, `system` |
| Scheduled tasks | `cron`, `systemd_timer`, `at` |
| Storage | `mount`, `swap`, `lvm`, `disk`, `link` |
| Network (base) | `network`, `route`, `bond`, `bridge`, `vlan` |
| Firewall (base) | `firewall` (abstraction), `iptables`, `nftables`, `firewalld` |
| SSH & security | `ssh`, `security` (SELinux/AppArmor) |
| System config | `hostname`, `timezone`, `sysctl`, `kernel_module` |
| Files & VCS | `git`, `config`, `archive`, `langpkg` |
| Certificates | `x509` |

## Scope (out / non-goals)

- Windows-native modules (`win_*`) — v1.1.
- Container modules (`docker_*`) — v1.1.
- Web server modules (`web`) — v1.1.
- Database admin modules (`postgres_database`, `mysql_database`, `redis`) — v1.1.
- Kubernetes modules (`k8s_*`) — v2.0.
- DNS provider modules — v2.0.
- Niche networking (`promisc`, `wifi`, `dot1x`) — v2.x.
- Vendor-specific modules — v2.x.
- Parallel execution of independent states — v1.1 (sequential default in v1.0 for stability).
- Resource locking across state files — v1.1.
- Saga checkpoint resume — v1.4.

## Design summary

See `PROJECT-DETAILS.md §4.8`.

## Tasks

1. **Module interface + DefaultRegistry** with `RegisterModule(name, factory)`.
2. **State file YAML loader** (`StateFile{metadata, includes, variables, declarations}`).
3. **Template renderer** with custom filters.
4. **Validator** — module exists, params valid, requisite refs valid.
5. **Dependency resolver + cycle detection** + topological sort.
6. **State runner** — full pipeline; emit events at each transition.
7. **Drift detector** + `DriftReport` aggregation + severity calculation.
8. **History store** in DB.
9. **gRPC StateService** + REST handlers.
10. **`kscore-state` CLI**.
11. **40-module base stdlib** — group by category; one PR per category for review:
    - Each module: Check, Apply, Test functions; OS-dispatch where needed; idempotent; tests for normal path, edge cases, error paths.
    - Cross-platform tests: Linux distros (Debian/Ubuntu, RHEL/Fedora, Alpine) for system/file/package/service/user/group; macOS skipped in v1.0.
12. **Saga coordinator integration (minimal)** — state run optionally wraps in saga; compensate by re-applying prior state from history.
13. **Integration test**: apply 10-state file with mixed module types end-to-end on docker-compose.

## Acceptance criteria

- [ ] `kscorectl state apply tests/webserver.yaml --target role:web` applies on matching agents.
- [ ] `kscorectl state check tests/webserver.yaml` is dry-run; reports planned changes without applying.
- [ ] `kscorectl state drift tests/webserver.yaml` returns `DriftReport`; `--fix` re-applies.
- [ ] `kscorectl state history` lists past runs; `kscorectl state rollback <run-id>` reverts.
- [ ] All ~40 modules pass cross-distro Docker matrix (Debian 12, Ubuntu 22.04/24.04, RHEL 9, Rocky 9, Alpine 3.19) for applicable modules.
- [ ] Idempotency verified: same state apply twice produces zero changes on second run for every module.
- [ ] Coverage >80% per stdlib module; >85% on engine.
- [ ] Requisite cycles detected with full cycle path in error message.

## Risks

- **Module count is large** (~40) — sequence categories; ship MVP set first (file, package, service, user, group, cmd) then iterate.
- **Idempotency bugs** — Check-Apply-Test pattern is non-negotiable; CI must hammer.
- **Cross-distro fragility** — abstract package manager / init system in `internal/platform`; modules dispatch.
- **Template injection** — vars from agents may contain template syntax; document as untrusted; consider sandboxing template eval.
- **Drift false positives** — exclude transient attributes (mtime, SELinux contexts where not relevant); use content hash for files.

## References

- PROJECT-DETAILS §4.8, §4.19 (multi-env / platform detection).
