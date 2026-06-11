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
| `smoke.firewall.sh`  | The `firewall`-abstraction phase, run by `smoke.sh` after the disk phase. Installs `iptables`, then applies a `firewall` fixture (`service:` + `port:`) and re-applies for idempotency. The rules apply inside the container's own network namespace, so nothing on the host is touched and no teardown is needed. Self-gates if no backend tool can be installed. |
| `smoke.lvm.sh`       | The `lvm`-module phase, run by `smoke.sh` after the firewall phase. Installs `lvm2`, then drives the module through its create lifecycle (`pv` → `vg` → `lv`), an LV grow (`lvextend`), and — when a second loop is free — a VG PV-set reconcile (`vgextend`), each as its own harness invocation so the grow/extend paths re-apply idempotently. Each Physical Volume is a loopback device. Self-gates on `lvm2` + loop-device availability; teardown (raw `lvm` + `dmsetup` + `losetup`) is guaranteed by the trap. |
| `smoke.netdev.sh`    | The virtual-interface phase (`bond` + `bridge` + `vlan`), run by `smoke.sh` after the lvm phase. Installs a JSON-capable `iproute2`, builds `dummy` scaffold interfaces for the members/parent, then applies a fixture that creates one bond, one bridge, and one VLAN at runtime — each with `persist: networkd` so the boot-config renderer is exercised too — and re-applies for idempotency. The interfaces live in the container's own network namespace, so nothing on the host is touched. Self-gates on JSON `iproute2`, the `dummy` module, and per-interface-type kernel-module availability. |
| `smoke.mount.sh`     | The `mount`-module phase, run by `smoke.sh` after the netdev phase. Backs a `mounted` declaration with a loop-backed ext4 device and asserts the fstab entry + live mount converge then no-op. The mount is private to the container's own mount namespace. Self-gates on loop-device + mkfs/mount tool availability; unmounts + detaches on exit. |
| `smoke.sched.sh`     | The scheduled-task phase (`cron` + `at` + `systemd_timer`), run by `smoke.sh` after the mount phase. Installs `cron` + `at` (and starts `atd`), then applies a crontab entry, an `at` queue entry, and — on systemd distros — a `.timer` unit. Each module is self-gated on its tooling (`crontab` / a startable `atd` / a booted systemd). |
| `smoke.sysconf.sh`   | The system-config phase (`timezone` + `sysctl`), run by `smoke.sh` after the sched phase. Installs `tzdata` + `procps`, then sets the timezone (`timedatectl` on systemd, `/etc/localtime` symlink elsewhere) and a `net.*` sysctl with its persist drop-in. The sysctl is per-netns, so it stays inside the container. |

## Coverage scope

The fixtures currently exercise `file` + `package` + `service` +
`user`/`group` + `hostname` + `disk` + `firewall` + `lvm` +
`bond`/`bridge`/`vlan` + `mount` + `cron`/`at`/`systemd_timer` +
`timezone`/`sysctl` — the modules whose backends vary across distros
and now have cross-distro implementations. `user`/`group` route to
shadow-utils on the systemd distros and to BusyBox `adduser`/`addgroup`
on Alpine (where shadow-utils is absent). `hostname` uses `hostnamectl`
on the systemd distros and the `/etc/hostname` + `hostname(1)` fallback
on Alpine.

`bond`/`bridge`/`vlan` create a Linux virtual interface at runtime via
`ip link add … type …` (`smoke.netdev.sh`): the phase builds `dummy`
scaffold interfaces, then applies a fixture that creates one bond (over
a dummy member), one bridge (over a dummy port), and one VLAN (on a
dummy parent), each carrying `persist: networkd` so the shared
`netpersist` renderer (a `.netdev` plus member-side enslave drop-ins)
is exercised alongside the runtime `ip link` path. The interfaces are
created in the container's own network namespace, so they are private
to the container and never touch the host or a sibling distro's
container — the kernel `dummy`/`bonding`/`bridge`/`8021q` modules are
global and may be auto-loaded, which is harmless.

`mount` (`smoke.mount.sh`) mounts a loop-backed ext4 device at a scratch
point — the mount namespace is per-container, so it stays private to the
container. `cron`/`at`/`systemd_timer` (`smoke.sched.sh`) manage a
crontab entry, an `at` queue entry, and a systemd `.timer` unit (the
last on systemd distros only). `timezone`/`sysctl` (`smoke.sysconf.sh`)
set the system timezone — `timedatectl` on the systemd distros, the
`/etc/localtime` symlink + `/etc/timezone` fallback on Alpine/Devuan, so
both code paths are covered — and a `net.*` kernel parameter (per-netns,
so contained to the container) with its `/etc/sysctl.d` persist drop-in.

`firewall` is the cross-backend abstraction (one declaration → "allow
this service / port inbound"); `smoke.firewall.sh` installs `iptables`
and applies a `service: ssh` (resolved through the named-service
catalog → 22/tcp) plus an explicit `port: 443/tcp`, each becoming one
rule per family — so the abstraction's backend detection, the iptables
backend, its dual-stack (IPv4 + IPv6 when `ip6tables` is present), and
the service catalog are all exercised live. The detector only selects
firewalld when its daemon is running (`firewall-cmd --state` exits 0),
which the phase never starts, so with `iptables` installed the backend
is deterministically iptables on every distro.

`disk` exercises `mkfs` and `resize_fs` per fstype against loop-backed
devices (`smoke.disk.sh`): a *blank* loop tests the format path, and a
loop pre-seeded with a small fs on a *grown* device tests the resize
path (which converges on pass 1 and no-ops on pass 2). It covers the
four resizable fstypes — `ext4` (`resize2fs`), `xfs` (`xfs_growfs`,
mounted), `btrfs` (`btrfs filesystem resize`, mounted), and `f2fs`
(`resize.f2fs`, offline; the fill-check reads the on-disk superblock) —
each gated on the presence of its tools.

`lvm` drives the module against loop-backed Physical Volumes
(`smoke.lvm.sh`), one harness invocation per behaviour so the grow /
extend paths re-apply idempotently without a raw pre-seed: a **create**
fixture (`pv` → `vg` → `lv` with a size), then an **LV grow** fixture
that re-declares the LV larger (`lvextend` on pass 1, no-op on pass 2),
then — when a second loop is free — a **VG reconcile** fixture that
declares a second PV in the VG (`pvcreate` + `vgextend` on pass 1, no-op
on pass 2). This exercises the `pvcreate`/`vgcreate`/`lvcreate` create
lifecycle plus the gate-v0.5 LV-resize and VG-PV-set-reconcile
additions. Teardown (force-`vgremove`/`pvremove` + `dmsetup` +
`losetup -d`) is guaranteed by the trap, independent of any module
apply.

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

Caveat — `lvm` loops + device-mapper: the phase needs `lvm2`, the host
kernel's `loop` module, and a working device-mapper (the
privileged/cgroup container provides the last). It is loop-frugal — one
loop covers the create + LV-grow scenarios, and the VG-reconcile
scenario needs a second free loop; when one can't be allocated (a host
short on loops, e.g. snapd holds several) that scenario **skips
cleanly** while create + grow still run. `extents:`-based LVs,
`vgreduce`, and the `absent` removal path are not yet exercised here
(follow-ups).

Caveat — `lvm` udev + a private LVM config: LVM in a container is
finicky, so `smoke.lvm.sh` points the tools at a tailored
`lvm.conf` (via `LVM_SYSTEM_DIR`). The systemd distros boot
`systemd-udevd`, but it does not reliably create the device-mapper
nodes a container needs, so `lvcreate`'s zeroing step fails ("device
not cleared"); `activation { udev_sync = 0  udev_rules = 0 }` makes LVM
manage the nodes itself. This is deliberately done in the **config**,
not via the `DM_DISABLE_UDEV` environment variable — when udev is
running that variable makes libdevmapper print a "Bypassing udev"
warning to **stderr**, and the `lvm` module reads its tools' *combined*
stdout+stderr, so the warning corrupts the `pvs`/`vgs`/`lvs` output it
parses. (The module's combined-output parsing is a latent fragility —
any LVM stderr warning would trip it — flagged as a follow-up; the
phase simply avoids emitting one.) The config also sets
`obtain_device_list_from_udev = 0` (scan `/dev` directly, since a
no-udev distro's udev-sourced list is empty) and a `global_filter` that
accepts **only the loop devices this phase allocates**: a privileged
container shares the host's loop devices and device-mapper, so a bare
`/dev/loop*` glob would expose foreign VGs (a leftover from a crashed
run, or a sibling distro mid-teardown in the same matrix) and drag the
VG PV-set reconcile into a spurious `vgreduce`. Scoping the filter to
the phase's own loops isolates it — and keeps LVM from ever scanning
the host's real disks. `use_devicesfile = 0` is set — when the running
LVM is new enough to know the setting — so the filter actually applies:
newer LVM (RHEL 9 / Rocky) defaults to a "devices file" that ignores
`global_filter` (and warns about it on stderr, re-tripping the
combined-output parse); disabling it restores uniform filter-based
scanning. The setting only exists in LVM ≥ 2.03.12, and an older LVM
(Ubuntu 22.04's 2.03.11) warns "unknown" about it — the same
parse-corrupting noise — so the phase emits the line only after an `lvm
config devices/use_devicesfile` capability probe confirms it is
recognised. Across the matrix this exercises four LVM versions
(2.03.11 / .22 / .33 / .41), each with its own container quirks.

Caveat — `lvm` device-mapper namespace: device-mapper is a host-global
namespace shared by every privileged container, so the VG name is made
**unique per distro** (`ksvg_<distro>`). With a shared `ksvg`/`kslv`,
the `ksvg-kslv` dm node collides across the matrix's
sequentially-run containers when a sibling's teardown lags
(`lvcreate` fails "device-mapper: create ioctl … Device or resource
busy"). The phase also pre-cleans its own (distro-unique) VG before
creating, and its teardown removes only its own VG + dm node — never
`dmsetup remove_all`, which is host-global and would take unrelated
(even the host's own) inactive device-mapper devices with it.

Caveat — `bond`/`bridge`/`vlan` iproute2 + kernel modules: the phase
needs a JSON-capable `iproute2` (the providers parse `ip -d -j link
show`; Alpine's BusyBox `ip` lacks `-j`, so the full `iproute2` package
is installed) and the host kernel's `dummy` module (for the scaffold
members/parent). Each interface type is gated on its own kernel module
(`bonding` / `bridge` / `8021q`): a type whose module can't be loaded is
dropped from the fixture with a log line, the same way the disk phase
skips an unavailable fstype. Unlike the `network` and `route` modules —
which reconcile a *real* interface's addresses/routes and so would
disturb the container's own networking — these three only ever create
*new* virtual interfaces over dummies, which is why they can join the
matrix at all. Because the interfaces live in the container's network
namespace (and the rendered `persist` files in its filesystem), teardown
is hygiene only; nothing leaks to the host.

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

Caveat — `firewall` backend scope: the phase pins the **iptables**
backend (installed and deterministically detected, as described above),
so the abstraction's detection + iptables backend + dual-stack +
catalog get live cross-distro coverage, but the **nftables** and
**firewalld** backends are not yet exercised live — pinning `backend:
nftables` and standing up a running firewalld daemon are follow-ups. As
with the disk phase, when no backend tool can be installed the firewall
phase **skips cleanly**.

Caveat — `firewall` IPv6 in a container: the iptables backend is
dual-stack (it applies the IPv6 half when `ip6tables` is present). The
iptables-nft variant (Debian/Ubuntu/Rocky/Alpine/Arch/Devuan)
auto-initializes the IPv6 filter table via `nf_tables`, so the dual-stack
path runs live there. A **legacy** `ip6tables` (openSUSE) instead needs
the kernel's `ip6table_filter` module loaded, which a container generally
can't load (the host kernel's modules aren't in the container's
`/lib/modules`). When `smoke.firewall.sh` probes the IPv6 filter table
and finds it unusable, it disables `ip6tables` so the backend takes its
documented **IPv4-only graceful skip** — the same path a real
IPv4-only-iptables host takes (so that skip path is itself exercised
live). The IPv6 dual-stack apply is still covered by the six iptables-nft
distros.

## Modules deliberately not in the matrix

Every stdlib module with distro-varying, container-safe live behaviour
is now exercised above. The rest are excluded for a concrete reason, not
an oversight:

- **Hermetic — covered by `cmd/kscore-server/state_integration_test.go`,
  not a container**: `file`, `link`, `cmd`, `config`, and also `ssh`
  (an `authorized_keys` line) and `archive` (tar/zip extraction) — these
  are pure file operations with no per-distro variance.
- **Matrix-hostile — would disturb the host or can't run isolated**:
  `network` and `route` reconcile a *real* interface's
  addresses/routes, which would break the container's own networking;
  `swap` activation (`swapon`) fails on an overlayfs swapfile and is
  host-global (not namespaced) regardless; `kernel_module` loads modules
  into the *host* kernel; `security` (SELinux/AppArmor) reads/writes
  host-global LSM state; `system`'s `reboot` op would kill the container.
- **Network-dependent**: `git` (clones a URL) and `langpkg`
  (pip/npm/gem registries) need outbound network and external services,
  which the hermetic, offline matrix deliberately avoids.

## Adding a distro

Append a `name|image|init|pkg_mgr|pkg|svc|boot` row to the `DISTROS`
array in `run.sh` (the `boot` field is the container's PID-1 command —
install + exec the init system), and tick the table above.
