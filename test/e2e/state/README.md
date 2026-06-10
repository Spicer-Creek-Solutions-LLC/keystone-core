# State stdlib cross-distro matrix

End-to-end smoke for the Epic 08 stdlib modules that touch live system
state, run across the supported Linux distro matrix. Each distro boots
in a **privileged container with its real init system** (systemd or
OpenRC); the harness applies a state fixture **twice** and asserts the
second pass is a zero-change no-op (idempotency) — so the cross-distro
`package` (apt/dnf/apk) and `service` (systemd/OpenRC) backends are
exercised against the live system, not mocked.

Hermetic modules (`file`, `link`, `cmd`, `config`) are already covered
by `cmd/kscore-server/state_integration_test.go` and don't need a
container.

## Running

```sh
make test-cross-distro                 # all distros
bash test/e2e/state/run.sh alpine-3-19 # narrow to a subset
```

Requires Docker; the target **skips cleanly** (exit 0) when Docker is
absent, so it's safe in `make check` chains. It needs **privileged**
containers (systemd-as-PID-1 needs cgroup write access), so it is a
manual / Docker-host gate and is **not wired into CI** (the shared
runner pool is unprivileged).

## Distro matrix

| Distro        | Init system | Pkg mgr | Status |
|---------------|-------------|---------|--------|
| Debian 12     | systemd     | apt     | ✓      |
| Ubuntu 22.04  | systemd     | apt     | ✓      |
| Ubuntu 24.04  | systemd     | apt     | ✓      |
| Rocky 9       | systemd     | dnf     | ✓      |
| Alpine 3.19   | OpenRC      | apk     | ✓      |
| openSUSE Leap | systemd     | zypper  | ✓      |
| Arch          | systemd     | pacman  | ✓      |
| Devuan        | sysvinit    | apt     | ✓      |

The bottom three were added to exercise the backends that landed after
the original five — **zypper** (openSUSE) and **pacman** (Arch) for the
`package` module, and **sysvinit** (Devuan) for the `service` module —
and all three run green via `make test-cross-distro`. Notes: the
`package` index refreshes via `zypper --non-interactive refresh` /
`pacman -Sy`; Devuan's sysvinit ops are script-based, so it boots a
keep-alive container (no PID-1 init) and `wait_ready` waits for the boot
to finish (PID 1 == `sleep`) before the smoke runs, so the smoke's apt
doesn't race the boot's. The `disk` phase degrades gracefully when the
host is short on loop devices (e.g. snapd holds several): a scenario that
can't allocate a loop is skipped, not failed.

## Layout

| File                 | Purpose |
|----------------------|---------|
| `run.sh`             | Orchestrator (invoked by `make test-cross-distro`). Builds the static harness, then per distro boots a privileged init container, runs `smoke.sh` via `docker exec`, and aggregates pass/fail. Skips without Docker. |
| `smoke.sh`           | Runs inside each container: refreshes the package index, renders the init-appropriate fixture with this distro's package/service names, runs the harness. |
| `harness/`           | A small static (CGO-free, runs on glibc + musl) Go program that compiles a state file and drives `Runner.Run` twice against the local registry — no kscore-server / NATS / Postgres needed. Has its own hermetic unit test. |
| `smoke.systemd.yaml` | Fixture for the systemd distros — `service` declared `running` + `enable`. |
| `smoke.openrc.yaml`  | Fixture for Alpine — `service` declared `stopped` + `enable` (OpenRC service supervision doesn't daemonise reliably in a container; the runlevel/enable path and status/exists queries do, so the smoke asserts those). |
| `smoke.sysvinit.yaml`| Fixture for Devuan — same shape as the OpenRC one (`service` `stopped` + `enable`). sysvinit's ops are script-based (`service` / `update-rc.d`), so a keep-alive container with no booted init is enough to exercise the enable + status/exists paths. |
| `smoke.disk.sh`      | The `disk`-module phase, run by `smoke.sh` after the main fixture. `disk` needs real block devices, so it backs each scenario with a loopback device (`losetup` over a sparse image), generates a disk fixture, and runs the harness on it. Self-gates on loop-device + per-fstype tool availability; tears loops/mounts down on exit. |

## Coverage scope

The fixtures currently exercise `file` + `package` + `service` +
`user`/`group` + `hostname` + `disk` — the modules whose backends vary
across distros and now have cross-distro implementations. `user`/`group`
route to shadow-utils on the systemd distros and to BusyBox
`adduser`/`addgroup` on Alpine (where shadow-utils is absent).
`hostname` uses `hostnamectl` on the systemd distros and the
`/etc/hostname` + `hostname(1)` fallback on Alpine.

`disk` exercises `mkfs` and `resize_fs` per fstype against loop-backed
devices (`smoke.disk.sh`): a *blank* loop tests the format path, and a
loop pre-seeded with a small fs on a *grown* device tests the resize
path (which converges on pass 1 and no-ops on pass 2). It covers the
four resizable fstypes — `ext4` (`resize2fs`), `xfs` (`xfs_growfs`,
mounted), `btrfs` (`btrfs filesystem resize`, mounted), and `f2fs`
(`resize.f2fs`, offline; the fill-check reads the on-disk superblock) —
each gated on the presence of its tools.

Caveat — `disk` loop devices: the phase needs the host kernel's `loop`
module (and, for xfs/btrfs, their filesystem modules) — like the
privileged/cgroup requirement, this is a host dependency. When loop
devices can't be created the disk phase **skips cleanly**.

Caveat — `disk` per-distro fstype availability: `btrfs-progs` and
`f2fs-tools` are not in the Rocky 9 base repos (RHEL dropped btrfs;
f2fs-tools is EPEL-only), so on Rocky the disk phase covers `ext4` +
`xfs` only and skips `btrfs`/`f2fs` with a log line. The other distros
cover all four. A weak `losetup` (e.g. BusyBox's, lacking `-c`) makes
the resize scenario a no-op rather than a grow — still idempotent, just
not exercising the grow on that host.

Caveat — `user`/`group`: BusyBox ships no `usermod`/`groupmod`, so on
Alpine those backends cannot modify an existing account's scalar fields
(or a group's GID) — those paths return `ErrModUnsupported`. The create
/ delete / lookup + supplementary-group paths the harness exercises are
fully supported, and the smoke fixtures only create (then re-apply for
idempotency), so this caveat doesn't affect the matrix.

Caveat — `hostname`: Docker bind-mounts `/etc/hostname` into the
container (it manages the container hostname), so the inode can't be
replaced — `hostnamectl` and the rename-based fallback both fail with
`EBUSY`. `smoke.sh` detaches the bind-mount (`umount /etc/hostname`)
before applying so `/etc/hostname` is an ordinary, settable file; the
running UTS-namespace hostname is unaffected. On a real host (no
bind-mount) this is a no-op.

## Adding a distro

Append a `name|image|init|pkg_mgr|pkg|svc|boot` row to the `DISTROS`
array in `run.sh` (the `boot` field is the container's PID-1 command —
install + exec the init system), and tick the table above.
