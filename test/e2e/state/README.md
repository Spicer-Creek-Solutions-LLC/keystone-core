# State stdlib cross-distro harness

Cross-distro end-to-end smoke for the Epic 08 stdlib modules that
touch live system state — `package`, `service`, `user`, `group`,
`hostname`, `timezone`, `sysctl`, `mount`, `swap`, `lvm`, `disk`,
`firewall`, `iptables`, `nftables`, `firewalld`, `ssh`, `security`,
`pki`. Modules that can be exercised against a `t.TempDir()` (the
hermetic set: `file`, `link`, `cmd`, `config`) are already covered
by `cmd/kscore-server/state_integration_test.go` and don't need a
container.

## Status — v0.5-gated scaffold

This directory is a **scaffold**. Only one distro (`debian:12`) is
wired up today, and only the smoke probe runs — not the full module
set. Filling out the matrix is tracked under
`docs/project/ROADMAP.md` →
`Cross-distro state stdlib docker matrix harness` (priority
`gate-v0.5`).

The harness is invoked through `make test-cross-distro`, which
delegates to `run.sh` in this directory. `run.sh` exits cleanly
(non-fatal) when Docker is unavailable so the target can sit in
`make check` chains without breaking developer machines.

## Distro matrix (v0.5 acceptance)

| Distro        | Init system | Pkg mgr | v0.5 status |
|---------------|-------------|---------|-------------|
| Debian 12     | systemd     | apt     | scaffold ✓  |
| Ubuntu 22.04  | systemd     | apt     | TODO        |
| Ubuntu 24.04  | systemd     | apt     | TODO        |
| Rocky 9       | systemd     | dnf     | TODO        |
| Alpine 3.19   | OpenRC      | apk     | TODO        |

Rationale for the choice — see Epic 08 acceptance criteria and
PROJECT-DETAILS §4.19 (multi-env / platform detection).

## Layout

| File              | Purpose |
|-------------------|---------|
| `run.sh`          | Entry point — invoked by `make test-cross-distro`. Skips with a clear message when Docker isn't installed. |
| `docker-compose.yml` | One service per distro. Mounts the repo read-only and runs `smoke.sh`. |
| `smoke.sh`        | Runs inside the container: builds `kscore-server` + `kscorectl`, applies `smoke.yaml`, asserts on-disk state. |
| `smoke.yaml`      | The state file the smoke probe applies. Keep it light — heavy probes go through Epic 19's perf harness, not this one. |

## Local invocation

```sh
make test-cross-distro
```

Requires Docker (or a compatible runtime — Podman with the
docker-compose shim works). Without one, the target prints what
would have run and exits 0.

## Adding a distro

1. Add a service to `docker-compose.yml` named for the distro
   (`alpine-3-19`, `rocky-9`, …).
2. The service runs `smoke.sh` with `KSCORE_DISTRO` set to the
   service name so the script can branch where init / pkg-manager
   semantics differ (systemd vs OpenRC; apt vs dnf vs apk).
3. Tick the row in the table above.
4. Once every row is ✓, drop the gate-v0.5 ROADMAP entry.
