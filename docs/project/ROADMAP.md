# Roadmap — Ranked v0.x Backlog

Single source of truth for scope **narrowed during implementation** of the v0.1 line plus the larger pre-/post-v1.0 work pulled forward from `FEATURES.md` and the old `PROJECT-DETAILS.md §6.2` table. Distinct from `FEATURES.md` (up-front product scope), `PROJECT-DETAILS.md §6.2` (high-level release-line summary), and `docs/project/VERSIONING.md` (canonical milestone gates) — those capture what was *planned* or *gated*. This file captures what is *queued*, with a per-entry priority.

## Priority taxonomy

Every entry below carries one of five priorities. See [`VERSIONING.md`](VERSIONING.md) for the milestone scheme this hangs off.

- **`gate-v0.5`** — Blocks the v0.5 external-tester milestone. Must land before v0.5 ships. ~13 entries today.
- **`gate-v1.0`** — Blocks the v1.0 SemVer-stability commitment. Must land in the v0.x line between v0.5 and v1.0. Production polish + epic completion. ~35 entries today.
- **`v0.x`** — Desirable pre-v1.0; lands opportunistically in v0.x releases as effort permits. No specific gate. ~20 entries today.
- **`v1.x`** — Post-v1.0 feature additions on the stable v1.0 line. Windows agent, K8s operator, TUI, saga checkpoint-resume, telemetry gateway, etc.
- **`v2.x+`** — Architectural post-v1.0: federation, supercluster, cloud KMS, marketplace, Web UI.

Entries may carry a **partial** priority — e.g. `gate-v0.5 (rich-rule canonicalisation); v0.x (the rest)` — when only some sub-items block a gate.

## How to use this file

- **When deferring scope mid-task**: add an entry with a `Priority:` field. Cite the epic + task that produced the deferral and the source-of-truth file/line.
- **When planning the next release**: filter on `Priority: gate-v0.5` (or `gate-v1.0`), prioritise within each gate.
- **When the deferred work lands**: move to a `### Done` block with a one-line "landed in commit/PR" note, or delete if cleanup is obvious.

Format: each entry is a `####` heading; body opens with `**Priority**:` then **What / Why deferred / Acceptance / References** bullets.

## Tracker integration

`tools/trackerctl/config/release-order.yaml` groups entries into tracker buckets that match the priority taxonomy (`gate-v0.5`, `gate-v1.0`, `v0.x`, `v1.x`, `v2.x+`). Reorder within a bucket there to change tracker-issue ordering; re-tag a priority here to move an entry between buckets.

---

## gate-v0.5 — blocks v0.5 external-tester milestone

#### `service` stdlib module — OpenRC / sysvinit / launchd backends

- **Priority**: gate-v0.5
- **What**: Epic 08 task 11f ships the `service` module with systemd-only. Hosts with a different init system (Alpine's default OpenRC, Gentoo OpenRC/sysvinit, older RHEL/CentOS sysvinit, macOS launchd) get `service.ErrNoBackend` from mutating ops. Add provider implementations:
  - `openrc` (Alpine / Gentoo) — task 11f2
  - `sysvinit` — post-v1.0
  - `launchd` (macOS) — post-v1.0
- **Why deferred**: systemd covers the whole Epic 08 cross-distro Docker matrix (Debian 12, Ubuntu 22.04/24.04, RHEL 9, Rocky 9 — Alpine 3.19 defaults to OpenRC but a systemd variant exists). One backend at a time keeps PRs reviewable; the Provider interface + detection skeleton is in place.
- **Acceptance**: openrc backend added (rc-service / rc-update wrappers); auto-detect picks the right init system per host; the `service` module's idempotency tests pass on Alpine 3.19 (OpenRC) in addition to the systemd distros.
- **References**: Epic 08 task 11f; `internal/statemgmt/stdlib/service/detect_linux.go` `defaultProvider`; `internal/statemgmt/stdlib/service/systemd.go` as the template.

#### `package` stdlib module — dnf, apk, zypper, pacman backends

- **Priority**: gate-v0.5
- **What**: Epic 08 task 11e ships the `package` module with apt-only (Debian / Ubuntu). On hosts where no supported package manager is detected the module returns `pkg.ErrNoBackend` ("no supported package manager detected on this host") rather than silently doing nothing. Add provider implementations for the other Linux package managers:
  - `dnf` (RHEL 8+ / Rocky / Fedora) — task 11e2
  - `apk` (Alpine) — task 11e3
  - `zypper` (openSUSE / SLES) — post-v1.0
  - `pacman` (Arch) — post-v1.0
- **Why deferred**: One backend at a time keeps PRs reviewable; the Provider interface + detection skeleton is in place, so adding a backend is a new file + a branch in `detect_linux.go` + per-backend tests.
- **Acceptance**: dnf + apk backends added (with appropriate command-line parsers); auto-detect picks the right backend per host; the Epic 08 cross-distro Docker matrix (Debian 12, Ubuntu 22.04/24.04, RHEL 9, Rocky 9, Alpine 3.19) passes the `package` module's idempotency tests.
- **References**: Epic 08 task 11e; `internal/statemgmt/stdlib/pkg/detect_linux.go` `defaultProvider`; `internal/statemgmt/stdlib/pkg/apt.go` as the template.

#### `firewalld` stdlib module — whole-zone management, masquerade/forward-port, direct rules

- **Priority**: gate-v0.5 (rich-rule canonicalisation); v0.x (the rest)
- **What**: Epic 08 task 11 ships the `firewalld` module (manage one item — a `service`, a `port`, or a `rich_rule` — in an existing firewalld zone via `firewall-cmd --permanent --zone=Z --query-/add-/remove-…`; states `present` / `absent`; runs `--reload` after a change unless `reload: false`). Reserved for v0.x:
  - Whole-zone management (declare the complete set of services / ports / rich rules / sources / interfaces on a zone and prune the rest) and zone creation; binding interfaces or source addresses to a zone; default-zone management; per-zone target (`ACCEPT`/`REJECT`/`DROP`).
  - Toggles for masquerade, ICMP-block (and ICMP-block-inversion), forward ports, ICMP types, and protocol items; `--direct` rules; ipset management; lockdown / panic mode.
  - Runtime-only (non-permanent) changes (v1.0 always operates on `--permanent` so changes survive a reboot).
  - Canonical-form rich-rule comparison (`--query-rich-rule` matches against firewalld's stored, normalised form — whitespace and attribute order may not survive verbatim; v1.0 documents this; canonicalise-before-compare is the follow-up).
  - `firewall` is its own (planned) abstraction module that dispatches across iptables / nftables / firewalld; firewalld is its own backend here.
- **Why deferred**: "enable / disable this one service / port / rich rule on this zone" is the v0.1 scope; whole-zone management needs a diffing pass over multiple `--list-…` outputs, masquerade / forward-ports / direct rules each have their own flag families and corner cases, and the rich-rule canonicaliser is a real grammar parse. The `Provider` (`Has` / `Add` / `Remove` / `Reload`) and the `Item` (`Kind` + `Value`) types extend cleanly.
- **Acceptance**: a `manage_zone: true` declaration replaces the zone's full service/port/rich-rule/source/interface set; `masquerade: true` toggles masquerade; `forward_port: { port: 80, proto: tcp, to_port: 8080 }` round-trips; a `direct_rule:` declaration round-trips; a re-formatted rich rule (different whitespace or attribute order) still matches the stored one; a stopped firewalld → permanent change persists, reload reports clearly.
- **References**: Epic 08 task 11 `_(landed)_`; `internal/statemgmt/stdlib/firewalld/firewalld.go` package comment; `internal/statemgmt/stdlib/firewalld/params.go`.

#### `firewall` abstraction — deny action, IPv6 on iptables, chain/table overrides, service catalog expansion

- **Priority**: gate-v0.5 (IPv6 on iptables + service-catalog expansion); v0.x (the rest)
- **What**: Epic 08 task 11 ships the `firewall` cross-backend abstraction (one declaration → "allow this service / port inbound", auto-detected backend, hard-coded standard inbound chain / `public` zone / IPv4 — see `internal/statemgmt/stdlib/firewall/firewall.go` for the full translation table). Reserved for v0.x:
  - `action: deny` (v1.0 is allow-only — translate to `-j DROP` / `drop` / a firewalld deny rich-rule).
  - `family: both` (or implicit dual-stack) on the iptables backend — v1.0 manages IPv4 only; for IPv6 coverage operators currently pin `backend: nftables` (which uses `inet`, both families) or `backend: firewalld` (also both). The abstraction should run TWO iptables sub-applies (v4 + v6) and aggregate the results when dual-stack is asked for.
  - `chain` / `table` / `family` overrides on the iptables / nftables backends (v1.0 hard-codes filter+INPUT+ipv4 / inet+filter+input). Operators who need a different chain currently bypass the abstraction and use the backend module directly.
  - Named-service catalog expansion: firewalld-native names (`dhcpv6-client`, `cockpit`, `mountd`, …) — the catalog currently rejects them with a clear error; multi-port services (`samba` opens 137/udp + 138/udp + 139/tcp + 445/tcp atomically — needs N decls or a multi-port expander); an `/etc/services` lookup with a `strict_catalog: false` opt-in for operators who accept the loose semantics.
  - Per-source filtering (`source: 10.0.0.0/8`) and richer rich-rule-style matches — v1.0 abstraction is allow-port-from-anywhere only.
  - nftables backend chain creation — v1.0 requires `inet filter input` to already exist (see the nftables module's V1X entry).
  - Backend attribution in `StateResult.Comment` (e.g. "applied via firewalld") — v1.0 passes the backend's Comment through verbatim.
- **Why deferred**: "allow one service / port on the standard inbound" is the v0.1 scope; the rest each open a meaningful design surface (`action: deny` cascades into iptables vs firewalld rich-rule asymmetry; dual-stack iptables needs two backend invocations and result-merging; catalog expansion needs a firewalld-services parser or a hand-curated multi-port table). The `Module` + `BackendDetector` + `buildSubDecl` shape extends cleanly.
- **Acceptance**: `action: deny` round-trips on every backend; `firewall.service: ssh` opens both v4 and v6 on every backend by default; `chain: custom` / `family: ipv6` on the iptables backend round-trips; the catalog accepts `dhcpv6-client` (firewalld backend) and `samba` (every backend, expanding to 4 sub-rules); `source: 10.0.0.0/8` filters the allow on every backend; the StateResult Comment carries `via <backend>`.
- **References**: Epic 08 task 11 `_(landed)_`; `internal/statemgmt/stdlib/firewall/firewall.go` package comment; `internal/statemgmt/stdlib/firewall/services.go`.

#### `security` stdlib module — AppArmor, SELinux file contexts / ports / modules / logins, absent semantics

- **Priority**: gate-v0.5 (AppArmor); v0.x (the rest)
- **What**: Epic 08 task 11 ships the `security` module (SELinux-only in v1.0: `mode: enforcing|permissive|disabled` for the global SELinux mode — persistent via `/etc/selinux/config` + runtime via `setenforce`; `boolean: NAME` + `value: on|off` for SELinux booleans via `setsebool -P`). Reserved for v0.x:
  - **AppArmor** in any form — per-profile `enforce|complain|disable` modes (`aa-enforce` / `aa-complain` / `aa-disable`), framework on/off, profile load/reload (`apparmor_parser`). The module is structured with a Provider interface so a second `AppArmorProvider` + a backend selector slot in cleanly.
  - SELinux file contexts (`semanage fcontext -a -t TYPE PATTERN` + `restorecon -R PATH`) — by far the deepest SELinux daily-ops surface beyond booleans.
  - SELinux port labels (`semanage port -a -t TYPE -p PROTO PORT`); SELinux policy module install (`semodule -i / -r`); SELinux login / user mappings (`semanage login`).
  - `state: absent` for booleans (v1.0 toggle-by-value — `value: off` — only) and for mode (semantics are ambiguous: "ensure NOT enforcing"?). A `value:`-less boolean op that just queries / reports could pair naturally with absent.
  - A `persist: false` opt-out that only flips the runtime via `setenforce` without touching `/etc/selinux/config` (uncommon but a real use case for short-lived debugging).
  - Managing SELinux on hosts where `getenforce` is missing but `/etc/selinux/config` is present (split the Provider so persistent-only ops don't require the user-space tools).
- **Why deferred**: "ensure SELinux is in this mode" + "ensure this boolean is in this state" covers the day-2 SELinux operations operators most commonly automate; AppArmor's per-profile mode flips are a meaningfully different shape (parsing `aa-status --json` etc.), and `fcontext`/`semanage port`/etc. each carry their own grammars and patterns. The `Provider` interface + the op-dispatch in `Module.Check`/`Apply` extend cleanly.
- **Acceptance**: `apparmor.profile: /etc/apparmor.d/usr.bin.foo` + `apparmor.profile_mode: enforce|complain|disable` round-trips and is idempotent against `aa-status --json`; `selinux.fcontext: "/srv/www(/.*)?"` + `selinux.context: httpd_sys_content_t` round-trips, with `restorecon` invoked when the rule changes; `selinux.port: 8443/tcp` + `selinux.context: http_port_t` round-trips; a `selinux.module:` declaration installs / removes a policy module via `semodule`; `state: absent` for a boolean removes the persistent override; a `persist: false` runtime flip leaves `/etc/selinux/config` untouched.
- **References**: Epic 08 task 11 `_(landed)_`; `internal/statemgmt/stdlib/security/security.go` package comment; `internal/statemgmt/stdlib/security/params.go`.

#### `system` stdlib module — reboot disconnect-tolerance, cross-distro reboot detection, locale dual-file, absent semantics for reboot/locale

- **Priority**: gate-v0.5 (cross-distro reboot detection + reboot disconnect-tolerance); v0.x (the rest)
- **What**: Epic 08 task 11 ships the `system` module with three operations (exactly one per declaration): `banner: motd|issue|issue_net` + `content`; `reboot: true` + `when_file` (default `/var/run/reboot-required`) + `delay` (0–60 min, default 1); `locale: <LANG>`. Reserved for v0.x:
  - **Reboot result delivery across the disconnect** — v1.0 launches `shutdown -r +<delay>` asynchronously and returns; for `delay: 0` the kscore-agent's RPC reply may not reach the controller before kernel kill. The Comment documents this; a robust solution coordinates with the agent's reconnect to deliver a "rebooted, marker cleared" follow-up event.
  - **Cross-distro reboot-needed detection** — v1.0 is Debian-flavoured (marker file gate). RHEL needs `dnf needs-restarting -r` (exit code 1 = reboot needed); Arch has `checkservices` / `needrestart`; Alpine has its own pattern. A platform-detected detector would centralise this.
  - **Unconditional reboot** (`force: true`) — v1.0 requires a marker so every Apply is gated. A forced-reboot escape hatch is sometimes needed (e.g. after a manual config change with no marker to set).
  - **Debian's `/etc/default/locale` dual-file** — v1.0 writes `/etc/locale.conf` only (the systemd canonical path). Debian (and downstream Ubuntu) historically uses `/etc/default/locale`; pre-systemd hosts and some images still read that. A dual-write or platform-detected target path is V1X.
  - **Per-`LC_*` overrides** (`LC_ALL`, `LC_MESSAGES`, etc.) and console keymap / X11 keyboard layout management (`vconsole.conf`, `00-keyboard.conf`).
  - **`absent` semantics for `reboot`** (cancel a scheduled reboot via `shutdown -c`) and for `locale` (revert to the compile-time default — ambiguous, may simply mean removing the LANG= line).
- **Why deferred**: "manage these three settings idempotently" is the v0.1 scope; reboot-across-the-disconnect needs a follow-up-event mechanism the engine doesn't yet have, RHEL-flavoured reboot detection involves a different tool and exit-code dance, and the locale dual-file is a small but real distro divergence. The op-dispatch in `Module.Check`/`Apply` and the `Provider` interface extend cleanly — each V1X item is roughly one more method or one more branch.
- **Acceptance**: a `reboot: true` decl on RHEL 9 detects need via `dnf needs-restarting -r` exit code without an `/var/run/reboot-required` marker; a `reboot: true` with `force: true` reboots unconditionally; a `locale: en_US.UTF-8` decl on Debian writes both `/etc/locale.conf` and `/etc/default/locale`; an `lc_all:` param round-trips into `LC_ALL=` in the locale conf; the agent delivers a `system.rebooted` event after a reboot Apply, with the boot-ID change as proof; `state: absent` for `reboot` cancels a pending `shutdown -r`.
- **References**: Epic 08 task 11 `_(landed)_`; `internal/statemgmt/stdlib/system/system.go` package comment; `internal/statemgmt/stdlib/system/params.go`.

#### `lvm` stdlib module — existing-VG PV-set mgmt, LV resize, metadata, thin/cache/snapshot

- **Priority**: gate-v0.5 (LV resize + VG PV-set mgmt); v0.x (the rest)
- **What**: Epic 08 task 11 ships the `lvm` module with three ops (exactly one per decl): `pv: <device>` (`pvcreate` / `pvremove`); `vg: <name>` + `pvs: [<device>, …]` (`vgcreate` / `vgremove -y`); `lv: <name>` + `vg: <vgname>` + `size: <human>` xor `extents: <N>%{FREE|VG|PVS|ORIGIN}` (`lvcreate -y -n <lv> {-L size|-l extents} <vg>` / `lvremove -y <vg>/<lv>`). Reserved for v0.x:
  - **Existing-VG PV-set management** — `vgextend` (add a PV to an existing VG) and `vgreduce` (remove a PV); `vgchange` allocation policy; missing-PV cleanup. v1.0 only verifies the VG exists by name; the declared `pvs:` list is used solely at create time.
  - **Existing-LV resize** — `lvextend` and `lvresize --resizefs`; `lvreduce` (fundamentally dangerous — filesystem must support shrink). v1.0 errors out on a mismatched-size existing LV by way of doing nothing about it (we report exists=true and don't reconcile size).
  - **LV metadata**: tags, allocation policy, stripes / mirror / RAID levels, thin pools + thin volumes, cache origin / pool, snapshots (origin + snapshot LV pair).
  - **PV metadata**: `--metadatasize`, allocation tags, restore from a backup.
  - **Filesystem creation on an LV** — v1.0 punts to the `disk` module on the resulting device, or `cmd` with `mkfs.X`; an `lvm` `mkfs:` shortcut would land cleanly.
- **Why deferred**: "create/remove an LVM object" is the v0.1 scope; PV-set mgmt on existing VGs needs a diffing pass over `vgs -o pv_name`, LV resize needs filesystem coordination, and thin / cache / RAID each open their own option family. The Provider (`HasPV`/`CreatePV`/…/`RemoveLV`) and the Op-dispatch extend cleanly — each V1X item is roughly one more Provider method or one more branch in `applyVG`/`applyLV`.
- **Acceptance**: a `vg:` decl with a different `pvs:` set than the live VG reconciles (extends or reduces) idempotently; an `lv:` decl with a new size larger than the live LV runs `lvextend` (with `--resizefs` when the FS supports it); `tags: [app=web]` on an LV round-trips; a thin-pool `lv:` decl creates the pool + the thin volume; a snapshot `lv:` declares an origin and creates the COW LV.
- **References**: Epic 08 task 11 `_(landed)_`; `internal/statemgmt/stdlib/lvm/lvm.go` package comment; `internal/statemgmt/stdlib/lvm/params.go`.

#### `disk` stdlib module — partition mgmt, resize, label/UUID, encryption, fstype catalog expansion

- **Priority**: gate-v0.5 (filesystem resize); v0.x (the rest)
- **What**: Epic 08 task 11 ships the `disk` module (single op: ensure `device:` has filesystem `fstype:` — `present` or `absent` — gated by an explicit `force: true` for any apply that would destroy existing data; mkfs binary resolved per fstype from a curated 9-entry catalog: ext2/3/4, xfs, btrfs, f2fs, vfat, exfat, swap). Reserved for v0.x:
  - **Partition management** via parted / sgdisk: create / remove / resize partitions, partition flags (boot, lvm, raid, esp), label types (GPT vs MBR), partition labels. Partitioning is destructive enough to deserve its own module (`partition`?) — co-existing with `disk` (filesystem layer) once both ship.
  - **Filesystem resize** without destroying data: `resize2fs` (ext2/3/4), `xfs_growfs` (xfs), `btrfs filesystem resize`, `f2fs_resize`.
  - **Filesystem label and UUID** management (no re-format): `tune2fs -L <label> -U <uuid>` (ext), `xfs_admin -L -U`, `btrfs filesystem label`, `swaplabel -L -U` (swap), `dosfslabel` (vfat).
  - **Encryption** (LUKS via cryptsetup) — `luksFormat`, `luksOpen`/`Close`, key slot management, passphrase rotation; integration with `mount` for opening at boot.
  - **fstype catalog expansion**: ntfs (`mkfs.ntfs` via ntfs-3g), zfs (zpool create), bcachefs, reiserfs (legacy), jfs (legacy), tmpfs (it's a mount-time fs, but a `disk` shortcut could be useful).
  - **Re-format on mismatch without explicit `force: true`** — a per-decl policy like `on_mismatch: reformat|error|warn` would let operators opt into more permissive defaults; v1.0 always errors on mismatch unless `force: true`.
- **Why deferred**: "ensure this device has this filesystem" is the v0.1 scope; partitioning is a different (more dangerous) operation surface, fs-resize is per-fstype and per-fs-state, encryption introduces a key-management dimension, and the catalog expansion is mostly more `mkfs.<fstype>` mappings + signature handling. The Provider (`GetFilesystem`/`MakeFilesystem`/`WipeFilesystem`) extends cleanly with `ResizeFilesystem`, `RelabelFilesystem`, etc.
- **Acceptance**: a `partition:` decl (in a new `partition` module, sibling to `disk`) creates `/dev/sdb1` at the declared start/size/type, idempotent against `parted -m print`; a `disk` decl with `resize_fs: true` extends the fs to fill the partition; a `label: mylabel` decl on an existing ext4 device updates the label without re-format; a `luks: { passphrase: …, key_slot: 0 }` decl initialises LUKS on the device; `fstype: ntfs` round-trips via ntfs-3g; `on_mismatch: warn` reports the drift in the StateResult Comment but doesn't error.
- **References**: Epic 08 task 11 `_(landed)_`; `internal/statemgmt/stdlib/disk/disk.go` package comment; `internal/statemgmt/stdlib/disk/params.go`.

#### `network` stdlib module — boot-survive configuration, per-family addresses, DNS / NTP / domain mgmt

- **Priority**: gate-v0.5 (boot-survive via netplan + networkd); v0.x (the rest)
- **What**: Epic 08 task 11 ships the `network` module (single op: reconcile one interface's *runtime* state via iproute2 — `addresses: [...]`, `mtu:`, `up: <bool>` — via `ip -j addr show` for Check and `ip addr add/del` / `ip link set mtu/up/down` for Apply). Reserved for v0.x:
  - **Boot-survive / persistent configuration** rendered to the host's actual network manager: systemd-networkd `*.network` units, NetworkManager `system-connections/`, netplan YAML, Debian `/etc/network/interfaces`, RHEL `ifcfg-*` scripts. Each is its own distro-aware renderer; a `persist: networkd|nm|netplan|ifupdown|sysconfig|auto` knob with auto-detect would be the natural surface.
  - **Per-family address management** — declare IPv4 and IPv6 sets independently (`ipv4_addresses: [...]` + `ipv6_addresses: [...]`); v1.0 merges them into one `addresses:` list.
  - **Address scope, valid_lft, preferred_lft, broadcast, peer** per `ip addr add` options.
  - **DNS resolvers, search domains, NTP servers** — these live in /etc/resolv.conf, systemd-resolved, NetworkManager, /etc/systemd/timesyncd.conf, etc. Distinct V1X module(s) (`resolver` / `ntp`).
  - **Wireless** (`wpa_supplicant`, NM Wi-Fi), 802.1X, **WireGuard / OpenVPN / IPsec** — vendor / protocol modules.
  - **Interface creation / removal** for physical NICs (impossible) and SR-IOV VFs (possible); v1.0 errors out with `ErrInterfaceNotFound` if the interface doesn't exist. Virtual-interface creation is the `bond` / `bridge` / `vlan` modules' job.
  - **Address-removal safety** — v1.0 will happily strip an in-use IP that the operator forgot to declare; a `dry_run_safety: true` mode would surface the in-use-IP risk before applying.
- **Why deferred**: "ensure this interface has these addresses, this MTU, this admin state right now" is the v0.1 scope — the runtime layer is what the operator most often needs in day-2 ops (container hosts, transient overlays, troubleshooting). Persistent config is a distro-render problem that adds ~5× the surface; per-family + scope + lft attributes are V1X polish on top. The Provider (`GetInterface` / `AddAddress` / `DelAddress` / `SetMTU` / `SetLinkUp`) extends cleanly along all of those axes.
- **Acceptance**: a `persist: networkd` decl writes a syntactically-valid `*.network` file matching the runtime config and survives a reboot; `ipv4_addresses:` + `ipv6_addresses:` round-trip independently; `valid_lft: 3600` on an address is preserved by Check; `dns_resolvers: [1.1.1.1, …]` writes the host's resolver config (systemd-resolved or /etc/resolv.conf); a `dry_run_safety: true` mode logs the planned removals without applying them.
- **References**: Epic 08 task 11 `_(landed)_`; `internal/statemgmt/stdlib/network/network.go` package comment; `internal/statemgmt/stdlib/network/params.go`.

#### `route` stdlib module — persistent configuration, route attributes, source-routing rules, multipath

- **Priority**: gate-v0.5 (persistent configuration); v0.x (the rest)
- **What**: Epic 08 task 11 ships the `route` module (one routing-table entry per declaration via `ip route replace` / `ip route del`; identity keyed on `(destination, metric, table)`; `gateway:` + `interface:` are reconciled). Reserved for v0.x:
  - **Boot-survive / persistent configuration** rendered to the host's network manager: networkd `[Route]`, NetworkManager static-routes, /etc/sysconfig/network-scripts/route-*, netplan `routes:`, /etc/network/interfaces `post-up ip route add …`.
  - **Route attributes**: `proto` (boot / dhcp / static / kernel), `scope` (link / host / global), `src` (source IP for the route), `mtu`, `advmss`, `pref` (high / medium / low), `onlink`, `realms`, `congctl`.
  - **Multipath nexthops** — `nexthop via X weight 1 nexthop via Y weight 2` style ECMP routes.
  - **Source-routing policy rules** via `ip rule add` — a separate `rule` module that pairs with `route` and the `table:` knob this module already exposes.
  - **VRF awareness** beyond the `table:` knob (the `vrf` interface type + L3 master).
  - **IPv6 specific**: `expires` (lifetime), `pref` med/high/low (RA preference).
  - **Default-table inference from metric** (some operator workflows imply a table from a metric range — v1.0 keeps them orthogonal).
- **Why deferred**: "ensure this one route exists / doesn't" via the modern `ip route replace` idempotency is the v0.1 scope; persistence is the same distro-renderer problem the `network` module faces; attributes are an option-by-option extension; multipath nexthops + policy rules are meaningful new surfaces. The Provider (`GetRoute` / `ReplaceRoute` / `DelRoute`) extends cleanly with extra `RouteSpec` / `RouteEntry` fields.
- **Acceptance**: a `persist: networkd|nm|netplan|…` decl renders a syntactically-valid route entry in the host's config; `proto: static` + `scope: link` + `src: 10.0.0.5` round-trip; a `nexthop:` list creates a multipath route; a separate `rule` module declares a `from 10.0.0.0/24 lookup vpn` policy rule and the `route` module's `table: vpn` declarations populate that table.
- **References**: Epic 08 task 11 `_(landed)_`; `internal/statemgmt/stdlib/route/route.go` package comment; `internal/statemgmt/stdlib/route/params.go`.

#### `bond` stdlib module — in-place attribute / member reconciliation, persistent configuration, slave attributes

- **Priority**: gate-v0.5 (persistent configuration); v0.x (the rest)
- **What**: Epic 08 task 11 ships the `bond` module (create / delete a Linux bonding interface at runtime via `ip link add … type bond mode … [miimon N]` + `ip link set <member> master <bond>`). Reserved for v0.x:
  - **In-place attribute reconciliation** on an existing bond: `mode`, `miimon`, `xmit_hash_policy`, `lacp_rate`, `ad_select`, `primary`, `primary_reselect`, `fail_over_mac`, `num_grat_arp`, `all_slaves_active`, etc. v1.0 considers an existing bond converged regardless of attrs (operators delete + recreate to change).
  - **Member-set reconciliation** on an existing bond — adding / removing slaves without destroying the bond. v1.0 enslaves declared members only at create time.
  - **Persistent / boot-survive configuration** rendered to the host's network manager (see the `network` module's V1X entry).
  - **Slave-level attributes**: per-slave queue id, priority.
- **Why deferred**: "create or delete this bond" is the v0.1 scope; changing bond mode on a live aggregation has subtle implications (LACP renegotiation, traffic interruption, slave release/reattach) that operators typically want behind an explicit step. The Provider (`GetLink` / `CreateBond` / `DeleteLink` / `SetMaster`) extends cleanly with `SetBondAttr` / `ClearMaster` methods for the V1X path.
- **Acceptance**: a `mode:` change on a live bond reconciles via `echo <mode> > /sys/class/net/<bond>/bonding/mode` (or down-up-cycle if required) and reports the before→after in the Diff; `members: [eth0, eth1, eth2]` against a live bond with `eth0, eth1` adds eth2; `members: [eth0]` against `eth0, eth1` removes eth1 (`ip link set eth1 nomaster`); a `persist: networkd` decl renders a `*.netdev` + `*.network` pair.
- **References**: Epic 08 task 11 `_(landed)_`; `internal/statemgmt/stdlib/bond/bond.go` package comment; `internal/statemgmt/stdlib/bond/params.go`.

#### `bridge` stdlib module — in-place attribute / port reconciliation, per-port attributes, persistent configuration

- **Priority**: gate-v0.5 (persistent configuration); v0.x (the rest)
- **What**: Epic 08 task 11 ships the `bridge` module (create / delete a Linux bridge interface at runtime via `ip link add … type bridge [stp_state 1]` + `ip link set <member> master <bridge>`). Reserved for v0.x:
  - **In-place attribute reconciliation** on an existing bridge: stp_state, forward_delay, hello_time, max_age, ageing_time, vlan_filtering, vlan_default_pvid, mcast_snooping, mcast_querier, group_fwd_mask.
  - **Port-set reconciliation** on an existing bridge — adding / removing ports without destroying it. v1.0 attaches declared ports only at create time.
  - **Per-port bridge attributes**: state (disabled / listening / learning / forwarding), priority, path-cost, pvid, learning, unicast_flood, mcast_flood, mcast_router, neigh_suppress.
  - **VLAN-aware bridge** filtering (`bridge vlan add vid 10 dev port`) — a separate `bridge_vlan` op or sub-module would land cleanly.
  - **Persistent / boot-survive configuration** rendered to the host's network manager.
- **Why deferred**: "create or delete this bridge" is the v0.1 scope; live-bridge attribute changes (especially stp_state) interrupt connected traffic and operators typically want behind an explicit step. The Provider (`GetLink` / `CreateBridge` / `DeleteLink` / `SetMaster`) extends cleanly along these axes.
- **Acceptance**: a `members:` change on a live bridge attaches/detaches ports without re-creating; `stp: true` on a live STP-disabled bridge enables STP and reports the change; `port_pvid: { eth0: 10, eth1: 20 }` round-trips; a `vlan_filtering: true` bridge takes a list of `bridge_vlan:` declarations that populate the VLAN table.
- **References**: Epic 08 task 11 `_(landed)_`; `internal/statemgmt/stdlib/bridge/bridge.go` package comment; `internal/statemgmt/stdlib/bridge/params.go`.

#### `vlan` stdlib module — in-place attribute reconciliation, QinQ, VLAN ranges, persistent configuration

- **Priority**: gate-v0.5 (persistent configuration); v0.x (the rest)
- **What**: Epic 08 task 11 ships the `vlan` module (create / delete an 802.1Q VLAN interface at runtime via `ip link add link <parent> name <name> type vlan id <id>`). Reserved for v0.x:
  - **In-place attribute reconciliation** on an existing VLAN: `id`, `parent`, ingress-qos-map, egress-qos-map, reorder_hdr, gvrp, mvrp, loose_binding.
  - **QinQ / 802.1ad** stacked-VLAN tagging (`proto 802.1ad`).
  - **VLAN ranges** — declare 100-200 in one decl that creates that many subinterfaces.
  - **Bridge VLAN filtering** (`bridge vlan add vid 10 dev port`) — see the `bridge` module's V1X scope.
  - **Persistent / boot-survive configuration** rendered to the host's network manager.
- **Why deferred**: "create or delete this VLAN" is the v0.1 scope; the in-place attribute change has subtle implications (VLAN id change effectively reroutes the L2 segment), and QinQ + VLAN ranges open new param-shape questions (a range op would have a different `name:` semantic). The Provider (`GetLink` / `CreateVLAN` / `DeleteLink`) extends cleanly.
- **Acceptance**: an `id:` change on a live VLAN reports drift and reconciles via delete-and-recreate (or `ip link set <vlan> type vlan id <new>` if the kernel supports it); `proto: 802.1ad` round-trips; `id_range: 100-200` creates 101 VLAN interfaces in one Apply; a `persist: networkd` decl renders a `*.network` for the VLAN.
- **References**: Epic 08 task 11 `_(landed)_`; `internal/statemgmt/stdlib/vlan/vlan.go` package comment; `internal/statemgmt/stdlib/vlan/params.go`.

#### Cross-distro state stdlib docker matrix harness

- **Priority**: gate-v0.5
- **What**: Epic 08 task 13 Layer C ships the test harness under `test/e2e/state/` (`run.sh` + `docker-compose.yml` + `smoke.sh` + `smoke.yaml`) with one distro (`debian-12`) wired up and Docker auto-skip on hosts that don't have it. The remaining v0.5 matrix entries (Ubuntu 22.04, Ubuntu 24.04, Rocky 9, Alpine 3.19) are scaffolded but commented out, and the smoke probe today only exercises the hermetic stdlib subset (file / link / cmd / config) — not the modules that touch live system state.
- **Why deferred**: Adding each distro is a real per-image dance (Rocky needs a `Dockerfile.rocky-9` that installs Go on top of `rockylinux:9`; Alpine needs `golang:1.23-alpine3.19` plus the OpenRC `service` backend the `service` stdlib ROADMAP entry covers separately). Wiring the modules that actually need root-owned system state (`package`, `service`, `user`, `hostname`, `mount`, `sysctl`, …) needs a per-module idempotency check per distro × per backend. v0.5 closes this end-to-end; v0.1 ships the scaffolding so the entry sites are reviewable.
- **Acceptance**: every row in `test/e2e/state/README.md`'s distro matrix is ticked ✓; `make test-cross-distro` runs all five services green on a Docker-equipped host; smoke.sh runs the full module set applicable to each distro (the `package` module against apt / dnf / apk, the `service` module against systemd + OpenRC, etc.) — gated by the per-module gate-v0.5 entries above so the matrix expansion lands alongside the backend implementations.
- **References**: `test/e2e/state/README.md`; `test/e2e/state/run.sh`; the per-module gate-v0.5 entries above for `package`, `service`, `firewall`, `firewalld`, `security`, `system`, `lvm`, `disk`, `network`, `route`, `bond`, `bridge`, `vlan` — those entries name the per-distro acceptance their module must clear inside this harness.

#### Encrypt CA material at rest

- **Priority**: gate-v0.5
- **What**: Epic 09 task 5 ships `internal/identity.FileCAStorage` as plaintext PEM (cert `0644`, key `0600`) under a `0700` directory — the v0.1 surface for the embedded provider's two-tier root + signing CA. PROJECT-DETAILS §4.10 calls for "optional encryption key" on persisted CA material; v0.1 defers that to a separate task because it pulls in key-management questions of its own (where does the encryption key live — config file, env var, KDF from a passphrase, OS keyring?). The `CAStorage` interface is stable; encryption is a wrapping layer.
- **Why deferred**: encryption-at-rest needs a real key-management story that's its own scope (which is why §4.10 phrases it as "optional"). v0.1 ships the filesystem-permission baseline + the interface seam; an `EncryptedFileCAStorage` lands ahead of v0.5 alongside Epic 10 (Secrets) which will share the master-key resolver.
- **Acceptance**: an `EncryptedFileCAStorage` (AES-256-GCM or NaCl secretbox; key sourced via a documented resolver shared with Epic 10) round-trips with `FileCAStorage`; existing plaintext deployments migrate via an explicit `kscore-identity ca encrypt` command; the v0.5 gate test boots a clean embedded provider with encryption enabled end-to-end.
- **References**: `internal/identity/ca_storage.go` `FileCAStorage`; PROJECT-DETAILS §4.10 ("with optional encryption key"); Epic 10 (Secrets) for the master-key resolver this shares.

## gate-v1.0 — blocks v1.0 SemVer-stability commitment

#### Agent-side cancel propagation (SIGTERM to in-flight commands)

- **Priority**: gate-v1.0
- **What**: When `CancelBatchJob` fires server-side, also signal the affected agents over NATS so any in-flight `os/exec` process is SIGTERM'd. v1.0 cancel persists CANCELLED status server-side; agent-side in-flight processes keep running until they exit naturally.
- **Why deferred**: Needs a new NATS cancel-command message type (subject scheme, envelope shape, signing) and an agent-side handler that maps inbound cancels to per-command context.CancelFuncs in the executor. Each piece is small but together they're a meaningful surface. v0.x trial scope tolerates the gap — long-running commands time out via agent.ExecutorConfig.DefaultTimeout.
- **Acceptance**: `kscorectl exec cancel <id>` mid-batch results in agent-side processes receiving SIGTERM (then SIGKILL after KillGrace); per-agent batch_agent_result rows record the cancelled state.
- **References**: Epic 07 task 12 acceptance bullet ("agent receives SIGTERM"); `internal/agent/executor.go` for the SIGTERM-then-SIGKILL kill protocol that already exists.

#### Unified single + batch dispatch persistence

- **Priority**: gate-v1.0
- **What**: Today every batch agent dispatch creates both a `commands` row (via `CommandDispatcher.Dispatch`) and a `batch_agent_results` row. At fleet scale that's a 2× write cost per command.
- **Why deferred**: Reusing `CommandDispatcher` gives us free retention + consistent signing for v1.0, but past a certain batch volume the duplication will show up in disk pressure / query cost.
- **Acceptance**: Batch dispatches write to a single row family (TBD: extend batch_agent_results, OR keep commands and drop batch_agent_results, OR linked via a `commands.batch_id` FK). Pick depends on observed query shape.
- **References**: Epic 07 task 12 layering note; `internal/controlplane/nats_batch_executor.go`.

#### Server-side target expression compile (proto extension)

- **Priority**: gate-v1.0
- **What**: Add `string target_expression` to `v1.BatchExecuteCommandRequest` (or a new `Target.expression` field) plus a server-side resolver that runs the full `internal/targeting` shorthand — `os:linux`, `arch:amd64`, `status:online`, `ip:10.0.0.0/8`, `id:web-*` (glob), `OR`, `NOT`, parens — and returns the matching agent set. kscorectl `exec` subcommands switch from client-side `ParseTarget` (limited to AND-of-labels-plus-hostname-glob) to sending the raw expression string.
- **Why deferred**: v1.0 proto `Target` only carries `agent_ids`, `labels`, `hostname_pattern` — three AND'd dimensions. kscorectl `exec` 11a parses the shorthand client-side and rejects (`ErrTargetUnsupported`) anything that doesn't fit. Adding a string field is a proto change + regenerated stubs + GRPCServer wiring; we held it out of 11a to keep the CLI commit narrow.
- **Acceptance**: `kscorectl exec run "uptime" --target "os:linux AND (role:web OR role:cache)"` resolves correctly against a stretched fleet; the AND-of-labels-plus-hostname-glob path remains a valid (degenerate) subset.
- **References**: Epic 07 task 11a; `internal/cli/exec/target.go` `ParseTarget` + `ErrTargetUnsupported`; `internal/controlplane/agent_resolver.go` (the existing server-side filter is the foundation).

#### Batch job retention (DeleteBatchJobsBefore + cascading FK)

- **Priority**: gate-v1.0
- **What**: `BatchJobStore.DeleteBatchJobsBefore(t)` analogous to `CommandStore.DeleteCommandsBefore`, plus `ON DELETE CASCADE` on the `batch_agent_results.batch_job_id` FK so batch cleanup wipes the per-agent rows in lockstep. A retention loop on `BatchDispatcher` (`runRetention`-shaped, mirroring CommandDispatcher) drops batches older than the configured TTL.
- **Why deferred**: Pre-1.0 deployments don't accumulate enough batches to need retention before trial-readiness. The store surface and the retention loop are independent additions; either can land first.
- **Acceptance**: `BatchJobStore.DeleteBatchJobsBefore(t)` deletes both `batch_jobs` rows and dependent `batch_agent_results` rows in a single tx (or cascade); `BatchDispatcher` has a configurable retention TTL and a periodic sweeper; CLI shows expected behavior after one TTL elapses.
- **References**: Epic 07 task 10; `internal/state/store.go` `BatchJobStore`; `internal/controlplane/command_dispatcher.go` `runRetention` as the pattern.

#### Replay protection on agent commands

- **Priority**: gate-v1.0
- **What**: Timestamp window + nonce dedup in `internal/agent.SecurityEnforcer.Validate`. Today HMAC alone gates command execution.
- **Why deferred**: HMAC covers v0.x trial scope. Nonce store needs a persistence layer + TTL eviction; not worth the complexity for the v1.0 ship date.
- **Acceptance**: Replayed `CommandRequest` (same nonce within window) is rejected with a typed error and audit log; legitimate commands inside the window pass.
- **References**: Epic 06 task 4 `_(landed)_` annotation; PROJECT-DETAILS §4.10; `internal/agent/security.go`.

#### Schema versioning via `golang-migrate`

- **Priority**: gate-v1.0
- **What**: Replace auto-DDL on startup with versioned migrations.
- **Why deferred**: v1.0 schema is new and stable; auto-DDL is fine until the first breaking change.
- **Acceptance**: `kscore-migrate up/down` cycles cleanly across at least one breaking schema change; CI runs migrations against PostgreSQL + SQLite.
- **References**: PROJECT-DETAILS §4.3 (line 301); Epic 02 scope-out.

#### Reactor engine + event lifecycle tracking

- **Priority**: gate-v1.0
- **What**: Filter→action chains with throttle/debounce/DLQ + lifecycle states (created/published/routed/processing/processed/failed/expired).
- **Why deferred**: v1.0 ships passive event system (emit/subscribe/query). Reactors need runbook + policy boundaries to be settled.
- **Acceptance**: `LogAction` / `EventAction` / `WebhookAction` reactors fire on event match with throttle + DLQ semantics.
- **References**: PROJECT-DETAILS §4.9 (lines 679-681).

#### Maintenance + Schedule gRPC

- **Priority**: gate-v1.0
- **What**: gRPC handlers for `MaintenanceService` and `ScheduleService` (REST is in v1.0).
- **Why deferred**: gRPC + REST land together at v1.1 per design.
- **Acceptance**: gRPC clients call `MaintenanceService.{Plan,Apply,Status}` and `ScheduleService.{Create,List,Run}`.
- **References**: Epic 03 scope-out (line 33); `pkg/api/maintenance/handler.go:5`.

#### Server-side heartbeat / metadata NATS subscriber → agent registry

- **Priority**: gate-v1.0
- **What**: kscore-server has `internal/controlplane.ConnectionManager.Heartbeat(ctx, id)` and `UpdateAgent` plumbing, but nothing in v0.1 subscribes to `kscore.{cluster}.agent.heartbeat` / `kscore.{cluster}.agent.{id}.state` and feeds those payloads into the registry. Agents publish heartbeats and metadata into the void; the server's agent registry stays empty.
- **Why deferred**: Epic 06 owns the agent side (publishing) — it shipped. The consumer side is a server-runtime concern that fits naturally with Epic 07 (remote execution targeting reads from the registry) or Epic 13 (clustering / agent registry HA). Either of those epics will land the bridge.
- **Acceptance**: Three Epic 06 acceptance bullets that gate on this consumer flip to ✓ once the bridge lands —
    1. "Agent registers with control plane on startup (visible in `kscorectl agents list`)"
    2. "Heartbeat every 30s; control plane marks stale after 3 missed"
    3. "Metadata published on startup + every 60s; visible via `kscorectl agents show <id>`"
  Plus: a server-side integration test driving an agent → server registry visible round-trip.
- **References**: Epic 06 task 12 `_(landed)_` surfaced the gap; `internal/controlplane/connection_manager.go:218` (Heartbeat method waiting for a caller); agent publishes at `internal/agent/agent.go` (`runHeartbeatLoop`, `runMetadataLoop`).

#### `state.apply.skip` event taxonomy + wiring

- **Priority**: gate-v1.0
- **What**: Epic 08 task 6 ships a `RunObserver.Skip` callback for cascade-skipped declarations (an earlier failure aborted the run; subsequent decls don't execute but are surfaced via the observer so external subscribers — alerting, audit, dashboards — see them). The statemgmt runner does not own event-subject naming; that's Epic 11. The corresponding event type `state.apply.skip` is therefore NOT yet listed in PROJECT-DETAILS §4.9's event taxonomy ("agent 5 / job 4 / state 5 / system 3 / user 3 / policy 2 = 22 types"). It needs to be added when Epic 11 wires the runner observer to the NATS event bus.
- **Why deferred**: The statemgmt task adds the observer hook; the event system itself lives in Epic 11. Updating §4.9's taxonomy now would predict an event Epic 11 hasn't shipped.
- **Acceptance**: PROJECT-DETAILS §4.9 lists `state.apply.skip` alongside the existing 5 `state.*` types (`state.*` becomes 6 types, total 23); Epic 11's event publisher emits `kscore.{cluster}.events.state.apply.skip` for each cascade-skipped decl; an integration test asserts the subject lands in JetStream.
- **References**: Epic 08 task 6; `internal/statemgmt/runner.go` `RunObserver.Skip`; PROJECT-DETAILS §4.9 event taxonomy; Epic 11 (event system).

#### Salt-faithful `prereq` direction in statemgmt resolver

- **Priority**: gate-v1.0
- **What**: The Epic 08 dependency resolver applies a **uniform direction rule** to all eight requisite keys: `<key>: [B]` on A puts B before A; `<key>_in: [B]` puts A before B. Salt's actual `prereq` semantic is the opposite — Salt reads `prereq: [B]` on A as "A is a prerequisite for B" (A first). Keystone's rule deviates so all eight keys teach the same way.
- **Why deferred**: One rule is much easier to teach + remember than a per-key direction table. v0.x trial scope hasn't surfaced a real workflow that needs Salt-faithful prereq; if it does, we add a per-key direction policy in the resolver and surface it on the DSL.
- **Acceptance**: Resolver applies a per-key direction policy where `prereq` and `prereq_in` use the Salt-faithful convention while keeping `require` / `watch` / `onchanges` (and their `_in` variants) on the existing uniform rule; docs in PROJECT-DETAILS §4.8 reflect the per-key directions explicitly.
- **References**: Epic 08 task 5; `internal/statemgmt/resolve.go` package comment; PROJECT-DETAILS §4.8.

#### Full RBAC role/permission CRUD

- **Priority**: gate-v1.0
- **What**: `Role`/`Permission` CRUD with per-resource permissions + dynamic policy. v1.0 ships fixed admin/operator/readonly.
- **Why deferred**: 3-role model covers trial scope; full CRUD needs UI affordances we don't have yet.
- **Acceptance**: Operators define custom roles via `kscorectl roles create`; bindings to principals enforce on API calls.
- **References**: Epic 09 scope-out (line 22); PROJECT-DETAILS §4.10 (line 737).

#### Multi-table transaction wrapper

- **Priority**: gate-v1.0
- **What**: A `state.Tx` type that batches mutations across tables atomically.
- **Why deferred**: v1.0 stores can serialize through caller-side coordination; full transaction wrapper needs careful error-handling design.
- **Acceptance**: Cross-table writes (agent + apikey + audit) commit/rollback atomically; tests verify rollback on mid-batch failure.
- **References**: Epic 02 scope-out (line 24); `internal/controlplane/bootstrap.go:270` (API key issuance currently non-transactional).

#### Auto-rotation of in-memory NATS creds

- **Priority**: gate-v1.0
- **What**: Agent rotates NATS credentials without restart.
- **Why deferred**: Gates on SPIRE provider (v1.3). v1.0 rotation = restart.
- **Acceptance**: Agent re-issues NATS creds on cert rotation event without dropping the connection.
- **References**: Epic 06 scope-out (line 33); PROJECT-DETAILS §4.6 (line 482).

#### Rotation orchestrator

- **Priority**: gate-v1.0
- **What**: Strategies (blue-green, rolling, canary, immediate) with health checks + auto-rollback.
- **Why deferred**: v1.0 secrets ship with manual rotation; orchestration is its own domain.
- **Acceptance**: `kscore-secrets rotate` invokes a strategy, runs health checks, rolls back on failure.
- **References**: Epic 00 deferred list (line 77); PROJECT-DETAILS §4.11 (line 765).

#### Encryption at rest (`KeyProvider`)

- **Priority**: gate-v1.0
- **What**: Pluggable `KeyProvider` (file/age/Vault/Cloud KMS) encrypts secrets + audit logs at rest.
- **Why deferred**: PostgreSQL TDE + filesystem encryption cover v0.x trial. Full implementation gates on cloud KMS work.
- **Acceptance**: `cfg.storage.encryption.provider = age|vault|aws-kms` round-trips encrypted blobs; key rotation re-wraps.
- **References**: Epic 02 scope-out (line 23); PROJECT-DETAILS §4.3 (line 303); §4.11.

#### File distribution: NATS Object Store + Git backends, mirror groups, conflict resolution

- **Priority**: gate-v1.0
- **What**: Multi-backend file dist with mirror groups, sync engine, conflict resolution strategies.
- **Why deferred**: v1.0 ships local FS + S3.
- **Acceptance**: Mirror group spans 3 backends; conflict resolution (newest-wins/largest-wins/primary-wins/manual) is operator-selectable.
- **References**: Epic 18 scope-out (lines 69-73).

#### Backup orchestration features

- **Priority**: gate-v1.0
- **What**: Automated scheduling, rolling upgrades, drift detection on self-config, self-healing, DR test harness.
- **Why deferred**: v1.0 ships manual `kscore-backup` only.
- **Acceptance**: `kscore-backup schedule add` registers a cron; rolling upgrade verifies health between waves.
- **References**: Epic 18 scope-out (lines 61-65).

#### Policy CRUD via gRPC (`CreatePolicy`/`UpdatePolicy`/`DeletePolicy`/`activate`/`deactivate`/`remediate`/`monitor`)

- **Priority**: gate-v1.0
- **What**: Server-side mutating endpoints for policies (today returns Unimplemented).
- **Why deferred**: Mutations gate on enforcement going GA so operators can author policies that actually run.
- **Acceptance**: gRPC clients author policies via the API; CLI subcommands `create|update|delete|activate|deactivate|remediate|monitor` wire to the new endpoints.
- **References**: Epic 03 task spec (line 16); `pkg/api/policy/handler.go:5`; `pkg/api/v1/policy.pb.go:3`.

#### `kscore-agent service start|stop` subcommands

- **Priority**: gate-v1.0
- **What**: PROJECT-DETAILS §4.6 lists `service install|uninstall|start|stop|status`. v1.0 ships `install|uninstall|status` only.
- **Why now**: `systemctl start kscore-agent` / `systemctl stop kscore-agent` are universally known by Linux operators; wrapping them adds maintenance for zero ergonomic value. Picked up in v1.1 because the Windows agent's SCM integration needs the same command shape and makes the wrapper worthwhile.
- **Acceptance for unblock**: `kscore-agent service start|stop` exist and proxy to the platform service manager; documented as the cross-platform form. Additive — no change to existing subcommands.
- **References**: Epic 06 task 9 `_(landed)_`; PROJECT-DETAILS §4.6 (line 473).

#### Batch dispatcher: no orphan-job recovery

- **Priority**: gate-v1.0
- **What**: Jobs in RUNNING state at process start are not auto-recovered; operators must inspect + retry.
- **Why now**: Orphan recovery needs ownership semantics (which CP node owns the job?) which settle once clustering has shipped — so the first post-v1.0 cleanup release.
- **Acceptance for unblock**: On startup the dispatcher reclaims/retries jobs it owns that were RUNNING; additive, no API change.
- **References**: `internal/controlplane/batch_dispatcher.go:72`.

### Targeted: v1.2

#### API key issuance: non-transactional

- **Priority**: gate-v1.0
- **What**: `internal/controlplane.BootstrapHandler` issues credentials via separate write paths (not a single transaction).
- **Why now**: v1.0 doesn't have a multi-table tx wrapper (the v1.2 backlog entry above) — this is a pure consumer of that, so it tracks v1.2. Internal refactor, no external surface change.
- **References**: `internal/controlplane/bootstrap.go:270`.

#### Bootstrap: demo mode only (TUI + non-interactive)

- **Priority**: gate-v1.0
- **What**: Both `kscore-agent bootstrap` paths (TUI wizard from task 7 and `--non-interactive` flags from task 8) accept all three modes structurally but `bootstrap.ValidateForV10` rejects production / enterprise with a v1.x deferral message before the Engine reaches Validate.
- **Why now**: Production mode needs TLS cert collection (gates on Identity/cert tooling); enterprise mode needs blueprint selection. Production-mode unblock lands in the v1.3 SPIRE/cert-rotation cycle; enterprise/blueprint pieces (the wizard screens) trail into v1.5 — see that entry. Lifting the gate is purely additive for existing demo-mode users.
- **Acceptance for unblock**: TUI screens for cert paths + cert-generation toggle (production); equivalent `--generate-certs` CLI flag wired to the non-interactive path. Drop or no-op `bootstrap.ValidateForV10` for production.
- **References**: Epic 06 tasks 7 + 8 `_(landed)_`; `internal/agent/bootstrap/configure.go` (search `ValidateForV10`); `cmd/kscore-agent/main.go` (search `buildConfigurer`).

#### Bootstrap CLI flags dropped from v1.0 surface

- **Priority**: gate-v1.0
- **What**: The original Epic 06 task 8 spec listed `--postgres-*`, `--nats-*` beyond `--join`/`--join-token`, `--generate-certs`, and `--apply-blueprint`. v1.0 ships without them.
- **Why now**: `--postgres-*` is server-only and belongs in the future unified `kscore-bootstrap` binary (the agent doesn't run a database). Extra `--nats-*` flags are unnecessary because v1.0 agents are external-mode only — embedded NATS is v2.0. `--generate-certs` re-appears with the v1.3 cert tooling; `--apply-blueprint` trails to v1.5 with the blueprint runtime. All additive flag additions.
- **Acceptance for unblock**: Per-flag — when its blocking work lands, add the flag with appropriate plumbing. The `--state-path` flag added in task 8 stays.
- **References**: Epic 06 task 8 `_(landed)_`; `cmd/kscore-agent/main.go` (search `registerBootstrapFlags`).

#### Bootstrap auto-installs systemd unit (production mode)

- **Priority**: gate-v1.0
- **What**: When demo-only mode-gate lifts, the bootstrap Engine's Install phase should call `systemd.Install` automatically — operator runs `kscore-agent bootstrap` once, gets both config and unit installed/enabled.
- **Why now**: Downstream of production mode (above); rides the same v1.3 cycle. Convenience chaining, additive — the explicit two-step flow still works.
- **Acceptance for unblock**: When `bootstrap.ValidateForV10` lifts for production, `bootstrap.NewDefaultInstaller` (or a production-mode wrapper) chains `systemd.Install` after the YAML render.
- **References**: Epic 06 task 9 `_(landed)_`; `internal/agent/bootstrap/install.go`; `internal/agent/systemd/install.go`.

#### Boostrap PSK consumption: in-memory tracking only

- **Priority**: gate-v1.0
- **What**: Used PSKs are tracked in-memory. Server restart re-permits a previously-used PSK.
- **Why now**: Persistence path needs the bootstrap-state-store work, which rides the v1.3 bootstrap-hardening cycle. Bug-fix-shaped (makes restart behaviour correct), not a compatibility break.
- **References**: `internal/config/nats.go:53`; Epic 05 task 9 `_(landed)_`.

### Targeted: v1.4

#### Type=notify systemd integration (sd_notify)

- **Priority**: gate-v1.0
- **What**: v1.0 unit uses `Type=exec`. `Type=notify` would let systemd track agent readiness via `sd_notify("READY=1")` calls — useful for `Wants=kscore-agent.service` ordering and reliable health checks.
- **Why now**: Requires the daemon to call into `coreos/go-systemd/v22/daemon`'s `SdNotify`, the dep explicitly skipped for v1.0. The v1.4 telemetry-gateway work is the first real consumer of agent readiness signalling. New unit + daemon ship together, so no break for existing installs.
- **Acceptance for unblock**: Daemon calls `SdNotify("READY=1")` after NATS connect + initial heartbeat publishes; unit flips to `Type=notify`; `systemctl is-active` reports `activating` until ready.
- **References**: Epic 06 task 9 `_(landed)_`; `internal/agent/systemd/unit.go` (search `Type=exec`).

### Targeted: v1.5

#### Bootstrap wizard: storage backend + blueprint selection screens

- **Priority**: gate-v1.0
- **What**: PROJECT-DETAILS §4.6 + Epic 06 task 7 originally envisioned the agent wizard collecting storage backend and applying blueprints. Both were dropped from the v1.0 agent surface — storage is server-only (the future unified `kscore-bootstrap` binary's concern); blueprint apply gates on the blueprint runtime.
- **Why now**: Blueprint apply needs the plugin/module system + the full blueprint catalogue, which is v1.5's headline. Storage screens still wait on the unified binary (no committed release) and may slip further. Additive wizard screens — existing wizard flows unchanged.
- **Acceptance for unblock**: For blueprints — the blueprint runtime lands, then a "select blueprints to apply" screen feeds an installer-side blueprint apply step. For storage — `cmd/kscore-bootstrap` (unified server+agent binary) gains the storage screens.
- **References**: Epic 06 task 7 `_(landed)_`; PROJECT-DETAILS §4.6 (line 451).

### Targeted: v1.6

#### Bootstrap: no rollback / transactional revert

- **Priority**: gate-v1.0
- **What**: Bootstrap engine resumes from checkpoint but doesn't revert side effects (config files, systemd units) on failure.
- **Why now**: Rollback semantics need install-step inversion + snapshot tracking. Slotted in v1.6 with the compliance/scan-scheduler cycle's general hardening; new `--rollback` flag is additive.
- **Acceptance**: Failed bootstrap re-runs cleanly; for true rollback, operator runs `kscore-agent bootstrap --rollback`.
- **References**: Epic 06 task 6; `internal/agent/bootstrap/doc.go:19`.

### Targeted: v1.7

#### Dedicated `keystone-core` system user auto-creation

- **Priority**: gate-v1.0
- **What**: v1.0 systemd unit defaults to root; `--user/--group` flags let operators run as a dedicated user, but the user must already exist (no auto-create).
- **Why now**: User creation belongs in package-mgmt territory (rpm/deb post-install via `useradd --system`), and packaging is part of the v1.7 air-gap / supply-chain work. The default-user flip is handled inside the package post-install (not as a surprise to in-place tarball upgrades), so it stays non-breaking.
- **Acceptance for unblock**: Packaging creates the `keystone-core` system user as part of `apt install` / `dnf install`. After that, `service install` can default `--user keystone-core --group keystone-core` and update the rendered ReadWritePaths to match.
- **References**: Epic 06 task 9 `_(landed)_`; v1.7 packaging work.

### Targeted: v1.9

#### Config: no per-endpoint TLS overrides

- **Priority**: gate-v1.0
- **What**: `cfg.NATS.Endpoints[]` use the cluster-wide TLS config; per-endpoint overrides are reserved schema fields.
- **Why now**: v1.0 has one TLS strategy per cluster; mixed-TLS topologies become relevant alongside the v2.0 multi-region work, but the override fields already exist and wiring them is additive (existing configs keep working), so it slots into v1.9 platform-polish rather than waiting for v2.0.
- **References**: `internal/config/nats.go:126`.

### Targeted: v2.0

#### Cluster-wide HMAC secret (vs per-agent)

- **Priority**: gate-v1.0
- **What**: All agents share one HMAC secret in v1.0. Per-agent keys derived from the bootstrap exchange replace it.
- **Why now**: Breaking change — it changes the agent↔server authentication model and needs a key-distribution mechanism still being designed; lands in v2.0 with the other auth/security infra changes (cloud KMS, federation).
- **Acceptance for unblock**: Bootstrap exchange establishes a per-agent key; server authenticates inbound by agent identity; cluster-wide secret removed (or relegated to a legacy compatibility window decided at design time).
- **References**: Epic 06 task 6 `_(landed)_`; `internal/agent/security.go:71`; `internal/config/security.go:15`.

## v0.x — desirable pre-v1.0 (no specific gate)

#### Join-token "any agent" mode (AgentID-less binding)

- **Priority**: v0.x
- **What**: Epic 09 task 8's `JoinTokenAttestor` REQUIRES the stored `JoinToken.AgentID` to be non-empty — empty AgentID is rejected at attestation with a `v0.x` mention. The §4.10 spec describes `AgentID` as an optional bind ("Metadata"); v0.1 makes it required so the attestor always returns a deterministic SPIFFE ID. v0.x adds an "any-agent" mode where the new agent supplies its claimed ID in the `AttestRequest` and the attestor binds the claimed AgentID at `MarkUsed` time.
- **Why deferred**: avoids two interaction loops (the agent-claims-then-server-binds dance + what happens if two agents present the same token concurrently with different claims) that aren't on the v0.5 gate critical path. Operators today either issue one token per agent or use the `MaxUses > 1` mode with explicit AgentIDs per cohort.
- **Acceptance**: a `kscore-identity token create --any-agent` creates a token with empty `AgentID`; agents pass `--agent-id agent-1` on attestation; the attestor binds the claimed AgentID atomically at `MarkUsed` time and returns the right SPIFFE ID; concurrent agents-with-different-claims see exactly one success per `MaxUses` slot.
- **References**: `internal/identity/join_token_attestor.go` `JoinTokenAttestor.Attest` (the v0.1 empty-AgentID rejection); Epic 09 task 8 (landed); the §4.10 spec's "optional bind" phrasing.

#### Live mid-execution stdout / stderr streaming

- **Priority**: v0.x
- **What**: Wire `BatchAgentOutput` chunks into the `BatchExecuteCommand` (and `CommandOutputChunk`s into the `ExecuteCommand`) gRPC streams as the agent produces them, instead of one buffered chunk at completion.
- **Why deferred**: v1.0 agents return stdout / stderr buffered inside `CommandResponse` — no live mid-execution publish. The gRPC streaming protocol's shape allows interleaved output chunks (per PROJECT-DETAILS §4.7), so callers won't see kscorectl `--follow`-style live output until the agent grows an incremental publish path and the server-side response correlator forwards chunks before the terminal `CommandResponse`.
- **Acceptance**: A long-running command (e.g., `tail -F`) streams partial stdout to a connected `kscorectl exec` over gRPC; agent disconnect mid-stream surfaces as `AGENT_FAILED` with the partial bytes already flushed.
- **References**: Epic 07 task 9; `internal/controlplane/grpc_server.go`; `internal/agent/agent.go` command handler.

#### Eval-time observability for malformed targeting patterns

- **Priority**: v0.x
- **What**: Slog hooks (or counters) for `match()` calls that swallow a malformed glob or an unparseable IP-vs-CIDR comparison. Today both fall through to `false` silently.
- **Why deferred**: The match function lives in a hot path (one call per agent per term per batch). Per-Matcher logger threading needs either a closure-built program per dispatch or a context plumbed through `expr.Run`; both add overhead the v0.x trial doesn't need. Operators get a clear *parse* error from `Compile`; the gap is only at runtime.
- **Acceptance**: A target expression with a literal-asterisk pattern (`role:*`-with-meta-on-empty-value) or a bad CIDR logs once per dispatch with the offending pattern, evaluation count, and target-expression raw form; no per-agent log spam.
- **References**: Epic 07 task 3; `internal/targeting/match.go` `matchValue`.

#### `git` stdlib module — authentication, submodules, advanced clone

- **Priority**: v0.x
- **What**: Epic 08 task 11 ships the `git` module (states `present` / `latest` / `absent`) relying on whatever the agent's existing git / SSH configuration provides for auth. Reserved for v0.x:
  - Deploy keys / per-repo SSH identity files, credential-helper configuration, token-in-URL rotation, SSH known-hosts management.
  - Submodules (`--recurse-submodules` and submodule sync on `latest`).
  - Sparse checkout, partial clone (`--filter`), bare repos.
  - Branch tracking on `latest` (v1.0 updates whatever ref is currently checked out to the fetched commit; it does not switch branches or maintain a named local tracking branch).
  - Reliable shallow clone + arbitrary-SHA checkout (v1.0 falls back to a full clone then `git checkout <sha>`, which can fail on a shallow clone).
- **Why deferred**: Auth in particular is a security-sensitive surface (key-material handling, host-key TOFU policy) better designed alongside Epic 09/10; the other items are scope-trims to keep the v1.0 module reviewable. The Provider interface (`Inspect` / `RemoteSHA` / `Clone` / `Sync` / `Remove`) is the extension point.
- **Acceptance**: a `credentials:` / `identity_file:` param flows an SSH key or credential helper into the clone/fetch; `submodules: true` recurses; the integration test clones a private repo over SSH with a deploy key.
- **References**: Epic 08 task 11 `_(landed)_`; `internal/statemgmt/stdlib/git/git.go` package comment; `internal/statemgmt/stdlib/git/gitcli.go`.

#### `link` stdlib module — relative-target normalisation

- **Priority**: v0.x
- **What**: Epic 08 task 11 ships the `link` module (symlink + hard link, states `present` / `absent`). Symlink targets are compared and stored verbatim — a relative target is not canonicalised against the link's directory, so `target: ../foo` and `target: /abs/foo` are treated as distinct even when they resolve to the same path. v1.x: an opt-in canonicalising compare (resolve relative targets, optionally chase intermediate symlinks) and Windows link support.
- **Why deferred**: Verbatim compare is unambiguous and matches what the operator wrote; canonicalisation has surprising corner cases (symlink chains, the link's own directory not existing yet) that deserve their own design pass. Low impact — operators who want absolute behaviour write absolute targets.
- **References**: Epic 08 task 11 `_(landed)_`; `internal/statemgmt/stdlib/link/link.go` package comment.

#### `cron` stdlib module — per-field schedule, cron.d, env lines

- **Priority**: v0.x
- **What**: Epic 08 task 11 ships the `cron` module (per-user crontab entries via `crontab(1)`, states `present` / `absent`, identified by a `# keystone-cron: <name>` marker comment). v1.0 takes one `schedule` string (five fields or an `@`-shortcut) and validates only its shape (field count / known shortcut). Reserved for v0.x:
  - Salt-style separate `minute` / `hour` / `day_of_month` / `month` / `day_of_week` params.
  - `/etc/cron.d` drop-in mode (a `cron_d: true` switch, or a separate module) — for now the `file` module manages those.
  - Environment-variable lines (`KEY=value`) in the crontab.
  - Deep cron-field syntax validation (ranges, steps, month/day names).
- **Why deferred**: One `schedule` string covers the common case ergonomically; the rest are scope-trims to keep the v1.0 module reviewable. The marker-comment design and the `Provider` (`Read`/`Write`) seam accommodate all of it.
- **Acceptance**: `minute: "*/5"` etc. compose into a schedule; `cron_d: true` writes `/etc/cron.d/<name>` with a `user` column; an `env:` map emits `KEY=value` lines above the entry; malformed fields are rejected at validate time.
- **References**: Epic 08 task 11 `_(landed)_`; `internal/statemgmt/stdlib/cron/cron.go` package comment; `internal/statemgmt/stdlib/cron/params.go` `validateSchedule`.

#### `systemd_timer` stdlib module — generated service, user timers, more [Timer] knobs

- **Priority**: v0.x
- **What**: Epic 08 task 11 ships the `systemd_timer` module — generates a `.timer` unit from `on_calendar` (+ optional `persistent`) and manages its enabled/active state; the triggered `.service` is the operator's job (compose with `file` + `service`, or point `service:` at an existing unit). Reserved for v0.x:
  - Also generate the paired `.service` unit (an `exec_start:` / `user:` / `working_dir:` param set).
  - `--user` (per-user) timers.
  - `on_boot_sec` / `on_unit_active_sec` / `on_startup_sec` / `randomized_delay_sec` and other `[Timer]` directives (v1.0 takes `OnCalendar` + `Persistent`).
  - Calendar-expression validation (v1.0 lets `systemctl enable` reject malformed expressions at apply time).
- **Why deferred**: Composing with the `file`/`service` modules is the Unix-y v1.0 path and keeps the module small; the `[Timer]` knob set and the generated-service feature each warrant their own design pass. The `Provider` interface (unit-file ops + the systemctl verbs) extends cleanly.
- **Acceptance**: `exec_start:` + `user:` generate a paired `<name>.service`; `on_boot_sec:` emits `OnBootSec=`; a `user: true` timer lands under `~/.config/systemd/user/`; a malformed `on_calendar` is rejected before any file is written.
- **References**: Epic 08 task 11 `_(landed)_`; `internal/statemgmt/stdlib/timer/timer.go` package comment; `internal/statemgmt/stdlib/timer/unit.go` `renderTimerUnit`.

#### `config` stdlib module — more formats, separators, uncomment-aware updates

- **Priority**: v0.x
- **What**: Epic 08 task 11 ships the `config` module (one key/value in a config file, formats `keyvalue` + `ini`, both `=`-delimited, case-sensitive keys, full-line comments only). Reserved for v0.x:
  - Case-insensitive key matching (a `case_insensitive: true` switch) — INI and sshd-style configs often want it.
  - Configurable separator (`separator: " "` for sshd-style `Key Value`, `": "` for YAML-ish, etc.) — v1.0 is `=`-only.
  - Inline / trailing comments preservation (`key=value # note`) — v1.0 treats `# note` as part of the value unless `#`/`;` starts the line.
  - Uncomment-aware updates (`#PermitRootLogin yes` → set the real directive) — v1.0 just appends a new line.
  - Repeated-key directives (multiple `HostKey` lines), multi-line values / continuation lines, indentation-aware insertion under `[section]` headers, duplicate `[section]` headers.
  - TOML / YAML / JSON / XML formats; creating parent directories for a new file.
- **Why deferred**: `keyvalue` + `ini` cover the bulk of day-to-day "set this one directive" needs; the rest each carry parsing/round-trip subtleties (especially inline comments and multi-line values) that warrant their own design pass. The line-oriented `format.go` core + the `format` param extend cleanly.
- **Acceptance**: `case_insensitive: true` matches `Key`/`key`; `separator: " "` round-trips an `sshd_config` directive; an inline `# comment` survives a value change; setting a directive that exists only as `#directive ...` rewrites the comment line in place.
- **References**: Epic 08 task 11 `_(landed)_`; `internal/statemgmt/stdlib/config/config.go` package comment; `internal/statemgmt/stdlib/config/format.go`.

#### `archive` stdlib module — absent state, clean mode, safe symlinks, more formats

- **Priority**: v0.x
- **What**: Epic 08 task 11 ships the `archive` module (extract tar/tar.gz/tar.bz2/zip into `target`, `present` only, idempotent via a `creates` path or a size+mtime sentinel, path-escape defense, symlink/hardlink entries skipped). Reserved for v0.x:
  - `state: absent` — needs an extraction manifest (which files came from the archive) to remove only those; for now use `file: <target>` `state: absent`.
  - `clean: true` — remove `target` before extracting (Salt's `archive.extracted` clean mode).
  - Safe symlink / hardlink extraction (create them only when the link target stays within `target`).
  - `.tar.xz` / `.tar.zst` / `.7z` and other formats (need a third-party decompressor dep).
  - sha-based source identity instead of size+mtime (so a `touch` of the archive doesn't trigger a needless re-extract).
  - `owner` / `group` chown of the extracted tree; mtime preservation of extracted files.
  - Extraction size / entry-count limits (zip-bomb hardening) — `max_extracted_bytes` / `max_entries`.
  - `skip_existing` (don't overwrite files already present in `target`).
- **Why deferred**: "extract a release tarball once" — the v0.1 scope — is the dominant case, and `state: absent` plus the security/format extensions each carry real design weight (manifest tracking, decompressor deps, symlink-resolution policy). The `extract.go` core + the `format` param + the `Provider`-less direct-fs shape extend cleanly.
- **Acceptance**: `state: absent` removes exactly the previously-extracted entries; `clean: true` wipes `target` first; a `.tar.xz` archive extracts; a symlink entry pointing inside `target` is created while one pointing outside is rejected; a zip bomb is refused once it exceeds the configured cap.
- **References**: Epic 08 task 11 `_(landed)_`; `internal/statemgmt/stdlib/archive/archive.go` package comment; `internal/statemgmt/stdlib/archive/extract.go`.

#### `at` stdlib module — replace-on-change, per-user queues, batch

- **Priority**: v0.x
- **What**: Epic 08 task 11 ships the `at` module (one-shot scheduled jobs via the `at` toolchain, tagged with a `# keystone-at: <name>` marker comment, `present` / `absent`, matched by name only). Reserved for v0.x:
  - Replace-on-change — detect a queued job whose command or time differs from the declaration and re-queue it (v1.0 leaves an existing tagged job untouched; you change the declaration name or `atrm` it first).
  - Per-user `at` queues — submit/list/remove as another user (via su); v1.0 manages the agent's own queue.
  - The `batch` low-load variant (the `batch` command, or `at -b`).
  - Queue-letter scoping of the queue scan (`atq -q <letter>`); richer multi-line-script handling and submit-time-environment control.
- **Why deferred**: "queue this command once at a given time" is the v0.1 scope; `at`'s fire-once model makes replace-on-change and recurring semantics genuinely ambiguous (re-resolving a relative time spec like "now + 1 hour" never equals the daemon's frozen timestamp), so they want their own design pass. The `Provider` (`ListJobs` / `JobScript` / `Submit` / `Remove`) and the marker-comment identity extend cleanly.
- **Acceptance**: a `present` declaration whose command changed re-queues the job (old one removed); a `user:` param queues a job for that user; `batch: true` submits via `batch`; the queue scan can be limited to one queue letter.
- **References**: Epic 08 task 11 `_(landed)_`; `internal/statemgmt/stdlib/at/at.go` package comment; `internal/statemgmt/stdlib/at/provider_linux.go`.

#### `x509` stdlib module — combined PEM, encrypted keys, more issuance options

- **Priority**: v0.x
- **What**: Epic 08 task 11 ships the `x509` module (Go package `pki`; manage a TLS cert + private-key pair with crypto/x509, self-signed or CA-signed, RSA/ECDSA/Ed25519, `present` / `absent`). Reserved for v0.x:
  - Combined cert+key PEM in a single file (HAProxy-style) — v1.0 requires `key_path` ≠ the cert path.
  - OpenSSL-style SAN prefixes (`IP:` / `DNS:` / `email:` / `URI:`) — v1.0 auto-detects IP vs DNS and never emits email/URI SANs.
  - More Subject fields (Country, Locality, State, OU, …); encrypted (passphrase-protected) private keys.
  - Explicit key/cert file mode + owner params (v1.0: new key 0600, new cert 0644; rewrites preserve the mode).
  - Key reuse policy on regeneration (v1.0 keeps a still-valid key); CRL / OCSP / AIA / Name-Constraints extensions; `MaxPathLen` for CA certs.
  - PKCS#12 (`.p12` / `.pfx`) bundles; CSR generation (`x509.private_key_managed` + `x509.csr` split à la Salt); ACME / external issuer integration.
- **Why deferred**: "generate a server cert for this host (self- or CA-signed) and renew it before it expires" is the dominant case and the v0.1 scope; the rest each carry their own format/protocol weight (combined PEM round-tripping, encrypted-key passphrase handling, PKCS#12, ACME). The pure-Go `cert.go` core (`generateKey` / `loadPrivateKey` / `loadCertificate` / `checkState` / `buildTemplate` / `signCert`) and the param set extend cleanly.
- **Acceptance**: a combined cert+key file round-trips; a `key_passphrase` decrypts/encrypts the key on disk; `country: US` etc. land in the Subject; a `.p12` bundle is produced; a CSR is emitted for an external CA.
- **References**: Epic 08 task 11 `_(landed)_`; `internal/statemgmt/stdlib/pki/x509.go` package comment; `internal/statemgmt/stdlib/pki/cert.go`.

#### `mount` stdlib module — remount-on-change, escaping, swap, crypttab

- **Priority**: v0.x
- **What**: Epic 08 task 11 ships the `mount` module (manage an /etc/fstab entry + the live mount via /proc/mounts + mount(8)/umount(8); states `mounted` / `present` / `unmounted` / `absent`). Reserved for v0.x:
  - Remount-on-change — `mount -o remount` when the fstab options change for an already-mounted filesystem; reconcile a live device change (v1.0 updates fstab but doesn't touch a stale live mount, and doesn't re-verify the live device against the declaration because the kernel resolves UUID=/LABEL= to a real device).
  - fstab `\040` escaping for whitespace in mount points / devices / options — v1.0 rejects whitespace in those fields.
  - `findmnt`-based inspection (richer than /proc/mounts); `noauto` / `nofail` awareness so a `mounted` declaration on a `noauto` entry isn't reported drifted just because it isn't mounted at the moment.
  - swap-type fstab entries (`swap` is the `swap` module's job); loop-device / encrypted (crypttab) coordination; per-mount fsck/dump heuristics for the default `pass`/`dump`.
  - `unmounted` with `persist: true` (also drop the fstab entry — v1.0: use `absent`); bind/move/rbind helpers beyond putting `bind` in `opts`.
- **Why deferred**: "ensure this device is mounted here with these options, and the fstab agrees" is the v0.1 scope; remount-on-change and the live-device reconciliation need the UUID/LABEL-resolution problem solved (resolve the declared identifier to a kernel device before comparing), and fstab escaping / crypttab / swap each carry their own format weight. The pure `fstab.go` editor and the `Provider` (`Lookup` / `Mount` / `Unmount`) extend cleanly.
- **Acceptance**: changing `opts` on a `mounted` declaration triggers a `mount -o remount`; a mount point with a space round-trips through fstab and /proc/mounts; a `noauto` `mounted` entry isn't flagged drifted while down; an encrypted device coordinates with crypttab.
- **References**: Epic 08 task 11 `_(landed)_`; `internal/statemgmt/stdlib/mount/mount.go` package comment; `internal/statemgmt/stdlib/mount/fstab.go`.

#### `swap` stdlib module — UUID sources, resize, fallocate, custom opts

- **Priority**: v0.x
- **What**: Epic 08 task 11 ships the `swap` module (manage a swapfile/partition + its fstab entry + its live swapon state; states `on` / `present` / `off` / `absent`; a not-yet-existing swapfile is created with `dd`). Reserved for v0.x:
  - `UUID=` / `LABEL=` swap sources — v1.0 requires the source to be an absolute path (swapfile or device).
  - Enforcing/changing the size of an *existing* swapfile (`size:` only governs creation today); `fallocate`-based fast swapfile creation (v1.0 uses `dd`, slow for large files).
  - Custom fstab options for swap (`nofail`, `discard`, …) beyond `defaults` + `pri=N`; `mkswap -L <label>` / `-f`.
  - Not rotating a swap *partition*'s UUID when re-activating (v1.0 runs `mkswap` before `swapon` on a not-active source, which re-initialises a pre-existing swap area); btrfs (NOCOW) swapfiles; zram / dphys-swapfile flavours.
- **Why deferred**: "create a swapfile (or use a partition), enable it, put it in fstab" is the v0.1 scope; `UUID=`/`LABEL=` need the same identifier-resolution work as `mount`, swapfile resize needs a swapoff/recreate/mkswap/swapon cycle, and the `mkswap`-on-re-activate UUID-rotation concern wants an `IsSwapArea` probe (`blkid`/`swaplabel`) to skip `mkswap` when the area is already valid. The pure `fstab.go` editor and the `Provider` (`Lookup` / `MakeSwap` / `SwapOn` / `SwapOff` / `CreateSwapfile`) extend cleanly.
- **Acceptance**: a `UUID=…` swap source resolves and is managed; changing `size:` on an existing swapfile resizes it (swapoff → recreate → mkswap → swapon); `fallocate` creates a large swapfile instantly; `nofail` lands in the fstab opts; re-activating a partition doesn't change its UUID.
- **References**: Epic 08 task 11 `_(landed)_`; `internal/statemgmt/stdlib/swap/swap.go` package comment; `internal/statemgmt/stdlib/swap/fstab.go`.

#### `ssh` stdlib module — key validation, options-set compare, whole-file management

- **Priority**: v0.x
- **What**: Epic 08 task 11 ships the `ssh` module (manage one entry in a user's ~/.ssh/authorized_keys; states `present` / `absent`; matched by key material). Reserved for v0.x:
  - Validating the key blob as a well-formed SSH public key (via `golang.org/x/crypto/ssh.ParseAuthorizedKey`) — v1.0 only charset-checks the `<keytype> <blob>` pair, to avoid promoting `golang.org/x/crypto` to a direct dependency.
  - Quote-aware comma-splitting / set-comparison of the options field (v1.0 compares it verbatim, after collapsing whitespace) — so `no-pty,no-X11-forwarding` and `no-X11-forwarding,no-pty` aren't treated as equal.
  - An `authorized_keys2` / custom-path override; whole-file management (`ssh_auth_file.managed`-style — declare the complete key set, removing unmanaged keys).
  - `AuthorizedKeysCommand` / SSH certificate (`*-cert-v01@openssh.com`) handling; per-line `from=` / `environment=` / `tunnel=` helpers; `ssh_known_hosts` management; sshd_config tweaks (overlaps with the `config` module today).
- **Why deferred**: "ensure this public key is in <user>'s authorized_keys (with these options/comment)" is the v0.1 scope; full key parsing wants the x/crypto dep promoted (a deliberate hold), and options-set comparison needs a quote-aware splitter, and whole-file management is a different (and more dangerous — it removes keys) operation. The pure `authkeys.go` line editor and the `key` / `options` / `comment` params extend cleanly.
- **Acceptance**: a malformed key blob is rejected at validate time; `options: [no-pty, no-X11-forwarding]` matches an existing line with those options in any order; a `manage_file: true` declaration replaces the whole file; a cert-type key (`ssh-ed25519-cert-v01@openssh.com`) is handled.
- **References**: Epic 08 task 11 `_(landed)_`; `internal/statemgmt/stdlib/ssh/ssh.go` package comment; `internal/statemgmt/stdlib/ssh/authkeys.go`.

#### `iptables` stdlib module — structured rules, ordering, both-family, distro persistence

- **Priority**: v0.x
- **What**: Epic 08 task 11 ships the `iptables` module (manage a single iptables/ip6tables rule via `iptables -C`/`-A`/`-I`/`-D`; states `present` / `absent`; optional `save: <path>` for `iptables-save` output). Reserved for v0.x:
  - `family: both` (apply the rule to both iptables and ip6tables, requiring it in both); structured rule params (`proto`/`dport`/`source`/`jump`/… à la Salt) as an alternative to the raw `rule` string; rule *position* / ordering management (re-place a rule that exists but is in the wrong spot — v1.0 never moves an existing rule).
  - Quote-aware rule parsing (`--comment "with spaces"` — v1.0 splits on whitespace, so multi-word comments need the list form or a single word).
  - Distro-aware persistence (Debian `netfilter-persistent` / `/etc/iptables/rules.v4`, RHEL the `iptables` service / `/etc/sysconfig/iptables`) and whole-rules-file management, beyond the plain `save: <path>`; `iptables-nft` vs `iptables-legacy` backend selection; chain creation (`-N`) and chain policy (`-P`).
  - `nftables` is its own separate module.
- **Why deferred**: "ensure this one rule is (not) in this chain" is the v0.1 scope; `family: both` doubles the bookkeeping, structured params explode the surface (iptables has dozens of match options), ordering management needs `iptables -S`/handle bookkeeping, and persistence is genuinely distro-divergent. The `Provider` (`HasRule` / `AddRule` / `DeleteRule` / `Save`, parameterised by family) and the `rule` parsing extend cleanly.
- **Acceptance**: `family: both` keeps the rule in both ruleset families; `proto: tcp, dport: 22, jump: ACCEPT` builds the rule; a rule that exists at the wrong position is moved when a `position:` is declared; `--comment "two words"` round-trips; on Debian the rule survives a reboot via `netfilter-persistent`; an `iptables-nft` host is detected.
- **References**: Epic 08 task 11 `_(landed)_`; `internal/statemgmt/stdlib/iptables/iptables.go` package comment; `internal/statemgmt/stdlib/iptables/params.go`.

#### `nftables` stdlib module — structured rules, ordering, table/chain management, comment matching

- **Priority**: v0.x
- **What**: Epic 08 task 11 ships the `nftables` module (manage a single nft rule in an existing table/chain: list the chain with `nft --handle list chain`, match on the rule's canonical text, `nft add rule` / `nft insert rule … index N` / `nft delete rule … handle N`; states `present` / `absent`; optional `save: <path>` for `nft list ruleset` output). Reserved for v0.x:
  - Structured rule params (`proto`/`dport`/`saddr`/`jump`/… à la Salt) as an alternative to the raw `rule` expression; rule ordering / re-placement (re-place a rule that exists but is in the wrong spot — v1.0 never moves an existing rule); matching a rule by its `comment "…"` or by handle instead of by canonical text (so a rule whose body nft has re-normalised — service names, abbreviations — still matches and isn't re-added).
  - Quote-aware rule parsing (`comment "with spaces"` — v1.0 splits on whitespace, so a multi-word comment needs the list form with the quotes embedded).
  - Managing tables and chains themselves (`nft add table|chain`, base-chain `type … hook … priority …; policy …;`), named sets / maps / flowtables, and atomic whole-ruleset file management / `nftables.service` persistence beyond the plain `save: <path>`.
  - `iptables` is its own separate module.
- **Why deferred**: "ensure this one rule is (not) in this chain" is the v0.1 scope; structured params explode the surface (nft's expression grammar is large), ordering management needs handle/index bookkeeping, and table/chain/set management is a meaningfully different (and more destructive) operation. The `Provider` (`ListRuleHandles` / `AddRule` / `DeleteRule` / `SaveRuleset`) and the `rule` parsing extend cleanly; the canonical-text matcher is the obvious place to add comment/handle matching.
- **Acceptance**: `proto: tcp, dport: 22, jump: accept` builds the rule; a rule that exists at the wrong index is moved when an `index:` is declared; `tcp dport ssh accept` matches the stored `tcp dport 22 accept` (canonicalised before comparison); `comment "two words"` round-trips; a `manage_table: true` / `manage_chain: true` declaration creates the table/chain with the declared hook+policy; a saved ruleset is reloadable via `nft -f`.
- **References**: Epic 08 task 11 `_(landed)_`; `internal/statemgmt/stdlib/nftables/nftables.go` package comment; `internal/statemgmt/stdlib/nftables/params.go`.

#### `langpkg` stdlib module — per-user/per-project installs, lockfiles, semver ranges, more ecosystems

- **Priority**: v0.x
- **What**: Epic 08 task 11 ships the `langpkg` module (one package per declaration via `manager: pip|npm|gem`, with optional strict-equality `version:` pin; global / system-wide installs only — `pip install`, `npm install -g`, `gem install`). Reserved for v0.x:
  - **Per-user / per-project installs**: pip `--user` + venv + pipx; npm non-global / `--prefix`; gem `--user-install` + Bundler. A `working_dir:` param + a `user:` param give operators the per-project / per-user mode they need without leaving the abstraction.
  - **PEP-668 `--break-system-packages`** opt-in for pip on modern Debian / Ubuntu / etc. where system-wide pip is blocked by default. v1.0 lets pip's own error surface to the operator; the eventual flag (`pep668_override: true`?) keeps the override explicit and decl-local.
  - **Lockfile-driven installs**: `npm ci` against package-lock.json; `pip install -r requirements.txt`; `bundle install --deployment` against Gemfile.lock. These would be a separate op ("install from lockfile") in the same module, mutually exclusive with `name:`.
  - **Semver / version-range matching**: `>= 1.2`, `~> 2.5`, `^3.0.0`. v1.0 is strict equality — a manifest like `version: ">=2"` is rejected at validate.
  - **Manager-options pass-through** (`mkfs_options`-style list): arbitrary install flags — `--index-url`, `--registry`, `--no-cache`, `--registry-mirror`, env vars (`NPM_CONFIG_REGISTRY=…`), proxy configuration.
  - **Additional managers**: `cargo` (Rust), `composer` (PHP), `mvn` / `gradle` (Java), `go install` (Go binaries — somewhat self-referential but useful), `cpan` / `cpanm` (Perl).
- **Why deferred**: "install / remove this package globally" is the v0.1 scope; per-project mode opens working-directory + cwd + permission concerns; lockfile installs need a different `name`-free op shape; semver ranges need a per-manager constraint parser (or a vendored library); the catalog expansion is mostly more Provider methods. The Provider (3 methods × 3 managers) extends cleanly along both axes.
- **Acceptance**: a `working_dir: /opt/app` + `user: app` decl installs `gunicorn` into the app user's venv; a `lockfile: package-lock.json` decl runs `npm ci` and is idempotent against the lockfile hash; `version: ">=2,<3"` matches any 2.x version; `manager_options: ["--index-url=https://internal.pypi/"]` round-trips; `manager: cargo` installs a Rust binary via `cargo install`.
- **References**: Epic 08 task 11 `_(landed)_`; `internal/statemgmt/stdlib/langpkg/langpkg.go` package comment; `internal/statemgmt/stdlib/langpkg/params.go`.

#### PROJECT-DETAILS §4.8 DSL example uses plural module keys

- **Priority**: v0.x
- **What**: The state-file DSL example in `PROJECT-DETAILS.md §4.8` uses plural top-level keys (`packages:`, `files:`, `services:`) while its requisite references use singular module names (`[package: nginx]`, `[file: ...]`). Epic 08 task 2 ships the parser with **singular** top-level keys so one rule covers both surfaces. The doc example needs to be updated to match.
- **Why deferred**: Pure doc cleanup; not blocking. Touching `PROJECT-DETAILS.md` mid-implementation muddies the diff for Epic 08 task 2 (the parser PR), which is the natural place to confirm the DSL shape. Easier to land the parser, get DSL examples committed to `internal/statemgmt/testdata/`, then sync the spec doc in a small follow-up.
- **Acceptance**: §4.8's YAML example uses `package:`, `file:`, `service:` (singular) consistent with the parser and the requisite-reference syntax; testdata fixtures are referenced as the canonical examples.
- **References**: Epic 08 task 2; `internal/statemgmt/parse.go`; `internal/statemgmt/testdata/webserver.yaml`; `PROJECT-DETAILS.md` §4.8 lines 580–605.

#### Percentage-based / rolling batch execution

- **Priority**: v0.x
- **What**: `kscorectl exec run --rolling 25%` for staged rollouts.
- **Why deferred**: v1.0 ships full-fanout + concurrency-cap; rolling needs progress-pause/resume semantics.
- **Acceptance**: A 100-target batch with `--rolling 10%` runs in 10 waves; failure-rate threshold halts the rollout.
- **References**: Epic 07 scope-out (line 29).

#### Active dial-time circuit breaker eviction

- **Priority**: v0.x
- **What**: Skip OPEN endpoints when nats.go picks the next reconnect target.
- **Why deferred**: Requires replacing nats.go's native multi-URL failover with a per-endpoint dial loop — substantial refactor for marginal v1.0 benefit.
- **Acceptance**: An endpoint with breaker OPEN is skipped during reconnect attempts until the breaker half-opens.
- **References**: Epic 05 task 7 `_(landed)_`; `internal/nats/breaker.go:20`.

#### Glob matching: no `**` (double-star)

- **Priority**: v0.x
- **What**: `internal/agent.SecurityEnforcer` uses `path.Match` (single-star only). gobwas/glob with double-star semantics is reserved for v1.x.
- **Why now**: stdlib `path.Match` covers the v1.0 command-allowlist use cases. Small, self-contained; folded into v1.2's linting/capabilities work. Note: broadening allowlist matching is a behaviour change to watch in review, but not a compatibility break.
- **References**: `internal/agent/security.go:59`.

#### Migration journal: no per-table checkpoint resume

- **Priority**: v0.x
- **What**: `kscore-migrate` records per-table checkpoints in the txlog but recovery from a partial migration restarts from the last full-table boundary, not the row-level checkpoint.
- **Why now**: Row-level resume needs a transactional checkpoint protocol that v1.0's `state.Tx` (deferred to v1.2) would unlock. Improvement to recovery only; no compatibility break.
- **References**: `internal/state/migrate_txlog.go:25`.

### Targeted: v1.3

## v1.x — post-v1.0 feature additions

#### Unbiased base62 for join-token generation

- **Priority**: v1.x
- **What**: Epic 09 task 10's `randomBase62` (in `internal/identity/join_token_generate.go`) uses `byte % 62` for the cleartext-token body. `256 mod 62 = 8`, so values 0-7 are slightly more likely than the rest (≈4.3% vs ≈3.5%). At `joinTokenBodyLen = 40` chars the residual entropy is ≈ 235 bits — well above v0.1's threat model for one-time tokens — but a strict security audit would prefer rejection sampling for deterministic uniformity. Drop-in replacement; no wire-format change.
- **Why deferred**: real-world attack surface is dominated by the raw entropy of the output, not the alphabet ratio. v0.1 ships the simpler implementation; v1.x replaces it during a routine hardening pass alongside other crypto-shape improvements.
- **Acceptance**: `randomBase62` uses rejection sampling (`crypto/rand.Int(rand.Reader, big.NewInt(62))`) per character; a statistical test over 1M samples shows no per-character bias > 0.1%; the existing alphabet + length tests continue to pass.
- **References**: `internal/identity/join_token_generate.go` `randomBase62`; Epic 09 task 10 (landed).

#### Trust federation (cross-domain bundle endpoint)

- **Priority**: v1.x
- **What**: SPIFFE federation — fetch + verify trust bundles from peer domains.
- **Why deferred**: v1.0 ships single-domain embedded CA; federation needs bundle distribution protocol.
- **Acceptance**: `kscore-identity federation add-domain/list/fetch-bundle` works against a second cluster.
- **References**: Epic 09 scope-out (line 23); PROJECT-DETAILS §4.10.

#### Windows agent (native service)

- **Priority**: v1.x
- **What**: kscore-agent on Windows with `service install/uninstall/start/stop/status` subcommands and SCM integration.
- **Why deferred**: v1.0 platform target = Linux amd64+arm64. Windows needs separate stdlib (`win_feature`, `win_firewall`, etc.) and user-switching — `internal/agent/exec_user_windows.go:15` is a stub returning `not supported in v1.0`.
- **Acceptance**: kscore-agent runs as a Windows service; SIGTERM equivalent triggers clean shutdown; user switching works via `LogonUser` / `CreateProcessAsUser`.
- **References**: PROJECT-DETAILS §4.6 (line 487), §4.8 (line 619); `internal/agent/exec_user_windows.go`.

#### WASM module runtime

- **Priority**: v1.x
- **What**: Wazero-based module execution alongside the v1.0 Starlark runtime.
- **Why deferred**: Starlark covers v1.0 module needs; WASM adds a second runtime to maintain.
- **Acceptance**: `pkg/plugin/runtime/wasm` loads + runs a signed wasm module via the same `Runtime` interface as Starlark.
- **References**: Epic 00 deferred list (line 69).

#### TUI monitor (`kscore-monitor`)

- **Priority**: v1.x
- **What**: Bubble Tea single-pane-of-glass — 8 base views (dashboard/agents/events/state/policy/jobs/logs/metrics), 13 with enhancements (cluster/secrets/leases/schedules/runbooks/webhooks).
- **Why deferred**: v1.0 ships Grafana dashboards + CLI. TUI needs gRPC-multiplex client + NATS subscriber. Not blocking trial.
- **Acceptance**: `kscore-monitor` opens, navigates between views, refreshes live.
- **References**: PROJECT-DETAILS §4.16 (line 1123); Epic 17.

#### macOS agent

- **Priority**: v1.x
- **What**: kscore-agent on macOS with launchd integration.
- **Why deferred**: Linux is v1.0 target; macOS adds `launchd` stdlib + Keychain integration.
- **Acceptance**: kscore-agent runs under launchd; agent identity stored in Keychain.
- **References**: PROJECT-DETAILS §4.6 (line 487), §4.8 (line 619).

#### SPIRE-backed identity provider

- **Priority**: v1.x
- **What**: Swap embedded CA for external SPIRE server + agent socket.
- **Why deferred**: Embedded CA covers v1.0; SPIRE needs operational tooling.
- **Acceptance**: `IdentityProvider` interface backed by SPIRE socket; SVID rotation is automatic.
- **References**: Epic 09 scope-out (line 24); PROJECT-DETAILS §4.10 (line 695).

#### K8s operator

- **Priority**: v1.x
- **What**: CRDs + reconciler for declarative Keystone Core management.
- **Why deferred**: K8s is one deployment target; v1.0 ships standalone-binary baseline.
- **Acceptance**: `Cluster`, `Agent`, `Blueprint` CRDs reconcile against a live cluster.
- **References**: Epic 00 deferred list (line 73).

#### Weighted endpoint load distribution + K8s endpoint discovery

- **Priority**: v1.x
- **What**: `cfg.NATS.Endpoints[].Weight` actually distributes load; K8s service-name endpoint discovery.
- **Why deferred**: v1.0 uses priority-only endpoint selection; weight is reserved in the schema. K8s discovery is part of the operator work.
- **Acceptance**: Load measurably distributes proportionally to weights; `nats.urls = ["k8s://service-name"]` resolves through discovery.
- **References**: `internal/config/nats.go:113`; `internal/nats/subject.go:135`.

#### AWS decorrelated jitter for fleet-scale reconnect storms

- **Priority**: v1.x
- **What**: Replace symmetric exp-jitter (`reconnectDelay` in `internal/nats/backoff.go`) with AWS decorrelated jitter — `delay = min(max, random(base, prev_delay * 3))`. Better herd-avoidance properties at >500-agent scale.
- **Why deferred**: Symmetric jitter is adequate for v0.x trial fleets (≤500 agents per design). Decorrelated needs careful per-call state tracking and is harder to test deterministically.
- **Acceptance**: `reconnectDelay` (or a sibling) implements decorrelated jitter; benchmark/sim shows tighter reconnect-time distribution at 1000+ agent scale; opt-in via a config knob (`reconnectjitterstrategy: symmetric|decorrelated`).
- **References**: Epic 06 task 10 `_(landed)_`; `internal/nats/backoff.go`.

#### Telemetry gateway

- **Priority**: v1.x
- **What**: `kscore-telemetry-gateway` — standalone collector for logs/metrics/traces over NATS subjects.
- **Why deferred**: v1.0 emits OTel/Prom directly to operator-supplied backends.
- **Acceptance**: Gateway aggregates from N agents; forwards to Loki/Prom/Jaeger.
- **References**: PROJECT-DETAILS §4.16 (line 1152); Epic 17.

#### Output archival to object storage cold-tier

- **Priority**: v1.x
- **What**: Long-running batch results overflow to S3/GCS/Azure cold tier.
- **Why deferred**: v1.0 keeps results in PostgreSQL with size cap.
- **Acceptance**: Outputs > N MiB archive to operator-configured bucket; `kscorectl exec show` fetches from cold tier.
- **References**: Epic 07 scope-out (line 30).

#### Saga/checkpoint advanced features

- **Priority**: v1.x
- **What**: SQLite-backed `pkg/saga/log_sqlite` (today's v0.1 ships `NewInMemoryLog` only); checkpoint-resume (re-load a crashed saga and continue from the last completed step with the pre-step Data restored); cross-state compensation graphs (compensation by dependency graph instead of strict reverse-linear); richer multi-failure aggregation reporting; gRPC `StateService.RollbackStateSaga` (saga-driven rollback, distinct from today's whole-run rollback CLI).
- **Why deferred**: v0.1 ships the minimal scaffold per Epic 08 task 12 — forward-execute steps, on first error walk completed steps in reverse with aggregate-and-continue compensation semantics, in-memory log, `Runner.RunSaga(History)` integration that re-applies the most recent prior `StateRunRecord` per decl. Persistent log + resume-from-checkpoint is a real durability surface (atomic transitions, recovery-on-startup hook, integration with `kscore-server` lifecycle); cross-state compensation graphs reshape the algorithm; the v0.1 minimum is enough for the trial use case where a saga either finishes or is restartable from the top.
- **Acceptance**: A 10-step saga survives a crash mid-step 5, resumes from checkpoint, completes the remainder; a cross-state compensation graph compensates step 3 before step 2 when the graph dictates; `kscorectl state rollback-saga <run-id>` reconstructs the saga from history and walks the compensation graph.
- **References**: PROJECT-DETAILS §4.17 (saga coordinator block, v1.x line 1197); Epic 08 task 12 *(landed)*; `pkg/saga/doc.go` package overview.

#### Air-gap baseline

- **Priority**: v1.x
- **What**: Offline registry, bootstrap packages, upgrade archives, full security scanning suite, signed module bundles, signing ceremony.
- **Why deferred**: v1.0 assumes online package repos.
- **Acceptance**: Operator installs Keystone Core in a fully air-gapped network; module updates flow through offline registry.
- **References**: PROJECT-DETAILS §6.2 (line 1496).

#### Policy enforcement (Enforce + Warn modes)

- **Priority**: v1.x
- **What**: Flip `policy.enforcement_enabled=true`. v1.0 ships policy engine in audit-mode-only.
- **Why deferred**: A misconfigured policy blocks the fleet. Audit-mode lets operators build confidence first.
- **Acceptance**: Policy in Enforce mode blocks command exec; Warn mode logs and allows.
- **References**: Epic 03 task 6; Epic 12 (audit/policy); PROJECT-DETAILS §4.12 (line 859).

#### Cloud workload identity (AWS IRSA, GCP WI, Azure MI)

- **Priority**: v1.x
- **What**: Cloud-native identity binding without static credentials.
- **Why deferred**: v1.0 ships embedded CA + JWT/PSK; cloud bindings need per-provider integration.
- **Acceptance**: Agent on EC2 with IRSA receives identity from instance metadata.
- **References**: Epic 09 scope-out (line 25).

#### gRPC-gateway annotation-driven REST + OpenAPI auto-gen

- **Priority**: v1.x
- **What**: REST + OpenAPI generated from proto annotations.
- **Why deferred**: v1.0 hand-codes both for control and simplicity (the v0 reset traced its complexity in part to grpc-gateway tooling friction).
- **Acceptance**: A new gRPC method automatically gets REST + OpenAPI without hand-edits.
- **References**: Epic 03 scope-out (line 30); PROJECT-DETAILS §4.5 (line 426).

## v2.x+ — architectural post-v1.0

#### Embedded NATS / hybrid mode / leaf node / endpoint advertiser / supercluster / WebSocket

- **Priority**: v2.x+
- **What**: Agents run embedded nats-servers; cluster forms via leaf-node mesh; reverse-leaf NAT traversal for agents behind firewalls; WebSocket transport for browser-side ops.
- **Why deferred**: v1.0 ships agent-as-NATS-client only; hybrid topology + WebSocket multiply test surface.
- **Acceptance**: Agent runs embedded NATS; reverse-leaf publishes its endpoint; supercluster gateway federates two clusters.
- **References**: Epic 05 scope-out (lines 38-42); Epic 06 scope-out (lines 27-28).
