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

#### Module system boot wiring (loader PolicyChecker/Hosts/trust-policy + runtime registration)

- **Priority**: gate-v1.0
- **What**: Epic 14 builds the plugin/module system as runtime-agnostic, all-seams (the recurring dep-light pattern): task 3's capability backends take injected `capability.Hosts` (real `internal/secrets`/`os-exec`/`net-http`); task 4's verifier takes an injected `*verify.TrustPolicy`; task 10's `loader.ModuleLoader` takes an injected `PolicyChecker` (the production impl = an adapter over the Epic-12 `internal/policy.Engine`), a `*verify.Verifier`, the `Hosts`, and a `RuntimeRegistry`. None of these seams is wired at `cmd/kscore-server` / `cmd/kscore-module` boot yet — until then the loader can parse/verify/policy-check/runtime-init only with explicitly supplied seams (unit/fake-tested), and a server cannot load+execute a published module end-to-end.
- **Why deferred**: same root as the other boot-wiring items — the seams are intentionally injected so `pkg/module/*` stays dependency-light and unit-testable; production construction (trust policy from config, Hosts from the live secrets/exec/http stacks, the `internal/policy`→`PolicyChecker` adapter, the Starlark runtime registered for `manifest.TypeStarlark`) lands with the kscore-module CLI (task 14, which wires its own seams for the author/install flow) and the server-lifecycle boot integration, so all module wiring happens in one coherent place.
- **Acceptance for unblock**: `kscore-module` (task 14) constructs a `ModuleLoader` with the real verifier+trust-policy, the `internal/policy` `PolicyChecker` adapter, real `capability.Hosts`, and the Starlark runtime (task 11) registered — and `kscore-module install`→`run` loads, verifies, policy-checks, and executes a signed published module end-to-end; the same wiring is available to `kscore-server` for server-side module execution.
- **References**: `pkg/module/loader` (task 10, landed — seams); `pkg/module/capability` `Hosts` (task 3); `pkg/module/verify` `TrustPolicy` (task 4); `internal/policy.Engine` (Epic 12, the `PolicyChecker` production impl); Epic 14 tasks 3/4/10, forward 11/12/14; companion of "Cluster gRPC services boot registration …".

#### Cluster gRPC services boot registration (ClusterService/CoordinationService + mTLS listener)

- **Priority**: gate-v1.0
- **What**: Epic 13 task 12 ships `CoordinationGRPCServer` (the server↔server NATS-down recovery channel; mTLS-only — rejects callers without a verified client cert) and task 13 ships the matching `CoordinationClient` (per-peer pooled client + heartbeat liveness + retry/backoff). Neither is wired at `cmd/kscore-server` boot, and task 15 now ships `ClusterGRPCServer` (`internal/controlplane/grpc_cluster_server.go`) + the cluster REST handler (`pkg/api/cluster/handler.go`) + `internal/cluster/backup.go`, none of which is wired either. CoordinationService specifically needs a **dedicated mTLS server↔server listener** (separate from the agent/operator surface), and the client needs its pool driven from `MembershipManager` + real mTLS creds + the "NATS down ⇒ use this channel" switchover. `ClusterGRPCServer` needs registering on the operator/API gRPC surface and the REST handler needs its real `ClusterProviders` (Status/Leader/Members/Rebalance/Backup) + the `Evictor` hook (a `MembershipManager`-backed RemoveMember) constructed at boot — until then the cluster REST routes return 503 and the gRPC server is unit/bufconn-tested only. Task 14's `GracefulShutdown` orchestrator also needs hooking into the kscore-server SIGTERM/Stop path with a real `StopAccepting` (connection-manager/listener drain). Until wired, the implementations exist + are unit/bufconn-tested but peers cannot actually reach each other over this channel (NATS-down recovery falls back to slower etcd-only paths), the cluster API surface is dark, and shutdown is not driven on SIGTERM.
- **Why deferred**: same root as the other cluster boot-wiring items — there is no cluster→kscore-server boot wiring yet (deferred since Epic 13 task 1). Server registration + the mTLS listener land with the server-lifecycle/boot integration (Epic 13 task 14 or a dedicated boot task) so all clustering wiring happens in one coherent place.
- **Acceptance for unblock**: kscore-server boot registers `CoordinationGRPCServer` on a dedicated mTLS-only listener and `ClusterGRPCServer` (task 15) on the operator/API surface; a 3-node cluster exchanges health/leader/recovery over CoordinationService with NATS down (verified by the HA NATS-failure E2E, task 17). The task-17 HA suite (`test/e2e/ha/`) already exercises this mechanism in-process — the real `CoordinationGRPCServer` over a real mTLS listener with NATS toggled down + non-mTLS rejection — so the remaining unblock is the *boot registration* itself plus the multi-process form (see "HA E2E multi-process / iptables-partition form").
- **References**: `internal/controlplane/grpc_coordination_server.go`; `internal/controlplane/coordination_client.go`; `internal/controlplane/grpc_cluster_server.go`; `pkg/api/cluster/handler.go` (+ `pkg/api/server/handlers.go` call site); `internal/cluster/backup.go`; `internal/cluster/shutdown.go` (`GracefulShutdown`); `api/proto/keystone/core/v1/{coordination,cluster}.proto`; Epic 13 tasks 12/13/14/15; companions: "Cluster leader-check boot wiring (kscore-server)", "Fencing guard wired around server write paths".

#### Fencing guard wired around server write paths

- **Priority**: gate-v1.0
- **What**: Epic 13 task 11 ships `FencingManager` with `Guard(OpType) (release, error)` + `ValidEpoch(e)` — the split-brain enforcement (minority/stale-epoch nodes block writes). The mechanism is built and reactive (HealthMonitor quorum signal + etcd epoch watch), but it is **not yet invoked around the actual server write paths** (gRPC state/command writes in `cmd/kscore-server` / `pkg/api`). Until wired, a minority/stale node detects + self-fences but still *accepts* write RPCs — correctness for shared state is still protected by etcd quorum at the storage layer, but the fast (<1s) application-level rejection the acceptance criterion wants is not in force.
- **Why deferred**: same root as the leader-check boot wiring — there is no cluster→kscore-server boot wiring yet (deferred since Epic 13 task 1); the `Guard` call sites are server-request middleware, landing with the server-lifecycle/boot integration (Epic 13 task 14 or a dedicated boot task) so all clustering wiring happens in one coherent place.
- **Acceptance for unblock**: write-bearing gRPC/REST handlers acquire `FencingManager.Guard(OpWrite)` (release on completion) and epoch-fenced operations check `ValidEpoch` before commit; the HA E2E suite (task 17) verifies a minority partition rejects writes within 1s while the majority serves. Task 17's `TestHA_MinorityPartitionBlocksWritesWithin1s`/`SplitBrainNoDoubleLeader` already verify the real `FencingManager.Guard` mechanism (writes fenced <1s, reads continue, auto-heal, no double-leader) against a controllable quorum seam; the remaining unblock is wiring `Guard` into the real server write paths so a *partitioned running server* rejects write RPCs.
- **References**: `internal/cluster/fencing.go` (`Guard`/`ValidEpoch`/`Fenced`); Epic 13 task 11; companion of "Cluster leader-check boot wiring (kscore-server)".

#### Cluster leader-check boot wiring (kscore-server)

- **Priority**: gate-v1.0
- **What**: Epic 13 tasks 6/8/9 expose leader-gate seams — `ShardManager.LeaderCheck`, `FailoverManager.LeaderCheck`, and the canonical `SingletonTaskManager.LeaderCheck()` — plus the long-standing `WithRetentionLeaderCheck` seam on the `internal/audit` + `internal/events` `RetentionEnforcer`s (default `AlwaysLeader`). The actual wiring (`cfg.LeaderCheck = le.IsLeader` / registering the RetentionEnforcers as `StartStopTask`s on the `SingletonTaskManager`) is a `cmd/kscore-server` boot concern and depends on clustering being constructed at server boot, which is itself deferred (Epic 13 task 1 left boot wiring out). Until then a clustered deployment runs those leader-only side-effects on every node (audit/events retention deletes, shard rebalance writes, failover orchestration) — correctness-safe (idempotent / CAS-guarded) but duplicated work.
- **Why deferred**: there is no cluster→kscore-server boot wiring yet; this lands with the server-lifecycle/boot integration (Epic 13 task 14 graceful-shutdown wires into the Epic 04 server lifecycle, or a dedicated boot task) so all clustering components are constructed and wired in one coherent place.
- **Acceptance for unblock**: kscore-server boot constructs the `LeaderElector` + `SingletonTaskManager`; `ShardManager`/`FailoverManager` configs receive `stm.LeaderCheck()`; the audit + events `RetentionEnforcer`s are constructed `WithRetentionLeaderCheck(stm.LeaderCheck())` (or registered as `StartStopTask`s); a 3-node cluster runs each leader-only task on exactly one node (verified by the HA E2E suite, task 17). Task 17 exercises real election + leader-gated `ShardManager`/`FailoverManager` in-process (single-leader invariant, leader-kill failover); the remaining unblock is the boot construction/wiring so the leader-only side-effects are deduplicated in a *running* multi-node deployment.
- **References**: `internal/cluster/singleton.go` (`SingletonTaskManager.LeaderCheck`); `internal/cluster/{shardmanager,failover}.go` (`LeaderCheck` seam); `internal/audit/retention.go` + `internal/events/retention.go` (`WithRetentionLeaderCheck`/`AlwaysLeader`); Epic 13 tasks 6/8/9.

#### HA E2E multi-process / iptables-partition form

- **Priority**: gate-v1.0
- **What**: Epic 13 task 17 ships the HA E2E suite (`test/e2e/ha/`) as **in-process, component-level** tests against the real `internal/cluster` stack + real `CoordinationGRPCServer` over a real mTLS listener + real embedded etcd + real Postgres. The literal forms in the epic text — a multi-process `3×kscore-server` deployment, real `iptables`/network-partition injection, killing real `etcd`/`NATS`/`Postgres` *processes* and asserting via a *running server's* RPC surface — are deferred. They require the cluster→`kscore-server` boot wiring (the three companion `gate-v1.0` entries) to exist first, plus a process/container orchestration harness (the `test/e2e/state` docker-compose precedent) and root/network-namespace control for iptables.
- **Why deferred**: there is no cluster→kscore-server boot wiring yet (deferred since Epic 13 task 1), so there is no running clustered server to multi-process-test or partition; the in-process suite already proves every HA *mechanism* (election/failover/fencing/recovery/coordination/backup) against the real implementation. The multi-process form is integration packaging on top of boot wiring, not new clustering logic, and is gated behind the same boot-wiring milestone.
- **Acceptance for unblock**: with the boot-wiring entries landed, a containerised 3×`kscore-server` cluster forms; an `iptables`/netem partition makes the minority reject writes within 1s while the majority serves; killing the leader process elects a new leader and completes failover; stopping the etcd/NATS/Postgres containers and restarting them recovers with zero data loss — all asserted through the running servers' gRPC/REST surfaces and run in CI. **The server-integrated SLO numbers also ride here**: Epic 13 task 18 ships the `slo` gate (`make slo`, `test/e2e/ha/slo_test.go`) verifying the §4.15 SLOs against the in-process mechanisms; the *server-integrated* SLO measurements (agent-reconnect latency through a running multi-process kscore-server, graceful-shutdown zero-disconnect timing) land with this multi-process form.
- **References**: `test/e2e/ha/` (task 17, landed — in-process form); `test/e2e/ha/slo_test.go` + `make slo` (task 18, landed — in-process SLO gate); `test/e2e/state/` (docker-compose harness precedent); companions: "Cluster gRPC services boot registration …", "Fencing guard wired around server write paths", "Cluster leader-check boot wiring (kscore-server)"; Epic 13 tasks 17/18.

#### Blueprint applied-runs store (durable)

- **Priority**: gate-v1.0
- **What**: Epic 15 task 5 ships the blueprint `Executor` with an `AppliedStore` interface + in-memory implementation only. `kscorectl blueprint rollback <run-id>` resolves the recorded apply (blueprint, version, namespace, resolved entrypoint, redacted params) from this store; with the in-memory impl a server restart loses every applied-run record, so rollback only works within the lifetime of the process that ran the apply.
- **Why deferred**: matches the `pkg/saga` precedent (ships in-memory, durable backend later). A durable applied-runs store is a real persistence surface (schema, migration, masking of sensitive params at rest, integration with the existing SQLite/state store) that is orthogonal to the executor logic itself; the v1.0 minimum is enough for the trial flow where apply + rollback happen in one server lifetime.
- **Acceptance for unblock**: a blueprint applied on one server process is rollback-able from a *restarted* process via `kscorectl blueprint rollback <run-id>`; sensitive (`source: secret` / `sensitive: true`) params are masked in the persisted record.
- **References**: `internal/blueprint/applied_store.go` (`AppliedStore` + `NewMemoryAppliedStore`); `internal/blueprint/executor.go`; `pkg/saga/log_sqlite.go` (durable-store precedent); Epic 15 task 5.

#### Remote / distributed blueprint apply wiring

- **Priority**: gate-v1.0
- **What**: `kscore-blueprint apply` / `rollback` (Epic 15 task 10) drive `internal/blueprint.Executor` against an injected `StateRunner`. `cmd/kscore-blueprint` wires a **local-host** StateRunner (statemgmt + stdlib) so single-node apply works; a `--target` selecting remote agents has no wired distributed StateRunner and returns a clear "remote apply not configured" error. Until wired, `kscorectl blueprint apply demo --target id:agent-1` works only for the local host, not a remote agent fleet.
- **Why deferred**: same root as the cluster/module boot-wiring items — distributed apply needs the blueprint executor hooked to the remote-execution path (Epic 07) at `kscore-server` boot, which is the deferred server-lifecycle integration. The local path is enough for the v0.5 single-node trial.
- **Acceptance for unblock**: `kscorectl blueprint apply <bp> --target id:<agent>` applies the rendered state collection on a remote agent and records the AppliedRun; rollback targets the same fleet.
- **References**: `internal/cli/blueprint/apply.go`; `cmd/kscore-blueprint/main.go`; `internal/blueprint/executor.go`; `pkg/api/blueprint/handler.go` + `internal/controlplane/grpc_blueprint_server.go` (tasks 11/12 — REST routes + gRPC `BlueprintService` ship with empty providers, return 503 / `codes.Unavailable` until this boot wiring supplies `BlueprintCatalog`/`BlueprintApplier`); Epic 15 tasks 10/11/12; companion of the cluster boot-wiring entries.

#### Durable runbook execution store

- **Priority**: gate-v1.0
- **What**: Epic 15 task 10 ships `kscore-runbook status` / `list-executions` / `audit` against an `ExecutionStore` interface + in-memory implementation only. The runbook engine (task 7) keeps an in-memory `Execution`; task 9 added audit/event emission but not a queryable run store. With the in-memory store, `status`/`list-executions` only see runs from the current process — a CLI invocation after the run cannot query it.
- **Why deferred**: mirrors the `pkg/saga` / blueprint applied-store precedent (ships in-memory, durable backend later). A durable runbook execution store is a real persistence surface (schema, sensitive-input masking at rest, integration with the SQLite/state store) orthogonal to the engine; the v1.0 minimum is enough for the trial flow where execute + inspect happen in one process.
- **Acceptance for unblock**: a runbook executed by one process is queryable via `kscore-runbook status <id>` / `list-executions` from a later process; sensitive inputs are masked in the persisted record.
- **References**: `internal/cli/runbook/query.go`; `internal/runbook/engine.go` (`Execution`); `pkg/api/runbook/handler.go` + `internal/controlplane/grpc_runbook_server.go` (tasks 11/12 — REST + gRPC `RunbookService` ship with empty providers, return 503 / `codes.Unavailable` until this boot wiring supplies `RunbookCatalog`/`RunbookRunner`/`ExecutionStore`); `pkg/saga/log_sqlite.go` (durable-store precedent); Epic 15 tasks 10/11/12.

#### Blueprint e2e real docker-compose convergence form

- **Priority**: gate-v1.0
- **What**: Epic 15 task 13 ships `test/e2e/blueprint/` as a build-tagged in-process suite that drives the real Epic-15 engines against the real catalog with a recording State Runner (it asserts every resolved declaration is dispatched + hooks run + rollback reverts). It does not converge `production-cluster`'s etcd/postgres/nats packages+services on real hosts/containers — that is not CI-safe with the v1.0 stdlib modules and is orthogonal to proving the integration wiring.
- **Why deferred**: same shape as the Epic 13 "HA E2E multi-process / iptables-partition form" deferral — the in-process suite proves the mechanism; the multi-container form needs a docker-compose harness (cf. the shell-driven, v0.5-gated `test/e2e/state/`) plus provider modules able to converge real services, which is a separate hardening surface (Epic 19).
- **Acceptance for unblock**: `production-cluster` applied via the blueprint executor against a docker-compose topology brings up etcd + Postgres + a NATS cluster and the suite asserts the services are actually reachable/healthy, not just that declarations were dispatched.
- **References**: `test/e2e/blueprint/e2e_test.go` (in-process form, landed); `test/e2e/state/docker-compose.yml` + `run.sh` (compose-harness precedent); Epic 13 "HA E2E multi-process / iptables-partition form" (companion deferral); Epic 15 task 13; Epic 19 (E2E hardening).

#### GitOps webhook receiver boot registration (kscore-server)

- **Priority**: gate-v1.0
- **What**: Epic 16 tasks 1-4 ship `internal/gitops/webhook` — the inbound webhook HTTP `Receiver` (its own `http.Server`, default `:8081/webhooks`, body-capped), the `Handler` interface + `Registry`, the four v1.0 provider handlers (ArgoCD/Flux/GitHub/GitLab) with a `NewDefaultRegistry`, header-based source auto-detection (`Registry.Detect`), per-source authentication (`Authenticator`: HMAC-SHA256 / Bearer / None via `BuildAuthenticators`), and normalization + best-effort re-emission on the Keystone event bus (`ToKscoreEvent` → `gitops.<provider>.<subtype>`; `EventEmitter` seam, `events.CategoryGitops` added to the closed taxonomy). The full receiver→auth→parse→emit pipeline is complete and httptest-verified with a fake emitter (acceptance 102/103 ticked). `config.GitOpsConfig` gates it (`gitops.webhook.enabled` default off; `gitops.webhook.sources.<provider>` per-source auth), but the `Receiver` is **not** constructed in the `cmd/kscore-server` boot sequence: the `config.GitOpsSourceAuthConfig`→`webhook.AuthSpec` translation and the live `events.EventPublisher` wiring are not built at boot. Until then the receiver runs only when started directly; a running kscore-server does not expose `:8081/webhooks`.
- **Why deferred**: same root as the Epic 13/15 dark-until-boot deferrals — boot registration (constructing the `Receiver` from config: registry + auth specs from `gitops.webhook.sources` + the live event publisher, started/stopped on the server lifecycle) belongs with the server-lifecycle integration so all GitOps webhook wiring happens in one coherent place. The mechanism itself is fully built and tested; only the boot construction remains.
- **Acceptance for unblock**: kscore-server boot constructs the `Receiver` from `config.GitOpsConfig` with the default registry, the per-source authenticators built from `gitops.webhook.sources`, and the live event publisher; a signed ArgoCD/GitHub webhook to a running server's `:8081/webhooks` is authenticated, parsed, and re-emitted on the bus as `gitops.<provider>.*`.
- **References**: `internal/gitops/webhook/{receiver,handler,detect,auth,auth_build,emit,register,event}.go`; `internal/config/gitops.go` (`GitOpsConfig` + `GitOpsSourceAuthConfig`); `internal/events/category.go` (`CategoryGitops`); Epic 16 tasks 1-4 (landed), forward 10; companions: the cluster/blueprint/runbook boot-wiring entries.

#### K8s rollout-undo client-go adapter + GitOps rollback boot wiring

- **Priority**: gate-v1.0
- **What**: Epic 16 task 7 ships `internal/gitops/rollback` — the `Executor` interface + `Registry` + three executors (Git-revert, ArgoCD sync-to-revision, K8s rollout-undo) behind narrow client seams (`GitClient`/`ArgoClient`/`K8sRolloutClient`). The Git seam is implemented by `rollback/gitexec` (go-git v5) and the ArgoCD seam by `rollback/argoexec` (stdlib REST); both are real and tested. The **`K8sRolloutClient` concrete adapter is deliberately not written** — it would pull `k8s.io/client-go` + `k8s.io/api` + `k8s.io/apimachinery` (3-module version-lockstep) and no v1.0 acceptance line needs a live cluster at task 7. The K8s executor is real + fake-tested but `RegisterAll` is commonly called with a nil `K8s` seam, so `kscore-gitops rollback … (k8s)` returns `ErrNotConfigured` until the adapter exists. Task 8 landed the rollback `Engine` (`engine.go`/`state.go`: `pkg/statemachine`-driven Pending→Approved/Rejected→InProgress→Completed/Failed→Verifying→Verified/VerificationFailed, optional approval gate, `PostVerifier` seam). Task 9 landed the **durable stores**: `rollback.SQLiteStore` and `verification.{Memory,SQLite}ResultStore`, both via `pkg/dbutil` mirroring the `pkg/saga` SQLite contract. Task 10 landed the **user-facing surfaces**: the §4.13 REST routes on `pkg/api/gitops` wired to a `Providers` (Rollback / Verifications) blueprint-style struct, plus the `cmd/kscore-gitops` CLI (reachable as `kscorectl gitops …` via the Epic-14 plugin dispatch) with `verify <file>` and `rollback`/`rollback approve|reject|get|list`. Task 10 also fixed a T8/T9 gap surfaced in scope (`Rollback.Config` now persisted via an additive SQLite column with a `PRAGMA table_info` ALTER guard, so approval-gated rollbacks resume with the originally-supplied executor configuration — acceptance 106 unblocked). The mechanism is complete + unit-tested, but it is **not yet constructed at `cmd/kscore-server` boot**: the REST handler ships with an empty `Providers{}` (routes return 503) until boot constructs the engine + the durable `SQLiteStore` + the live `verification.ResultStore`, the K8s `client-go` adapter still needs to be written so `--executor k8s` works against a real cluster, and `GetLastKnownGood` is still provider-history best-effort (verification-confirmed signal needs the wiring at boot too).
- **Why deferred**: same dependency-light / dark-until-boot root as the other Epic 16 entries — heavy clients stay behind seams so `internal/gitops/rollback` is unit-testable without a cluster; the concrete client-go adapter + the Engine/CLI boot construction land together at the GitOps boot-wiring step so all of it is wired in one coherent place, and go.mod stays free of the k8s.io trio until it is actually exercised.
- **Acceptance for unblock**: a `K8sRolloutClient` `client-go` adapter (Deployment get + ReplicaSet revision history + rollout-undo) is written so `kscore-gitops rollback --app web --strategy previous --executor k8s` against a real cluster rolls a Deployment back; `kscore-server` boot constructs the rollback Engine over a `rollback.SQLiteStore` + the `verification.SQLiteResultStore` and passes them as the `pkg/api/gitops.Providers` so the REST routes light up (today: 503); `GetLastKnownGood` consults the task-9 `verification.ResultStore` rather than raw provider history.
- **References**: `internal/gitops/rollback/{executor,git,argocd,k8s,register,engine,state,store_sqlite}.go`; `internal/gitops/verification/{result_store,result_store_sqlite}.go`; `internal/gitops/rollback/gitexec` (go-git, landed); `internal/gitops/rollback/argoexec` (REST, landed); `pkg/api/gitops/handler.go` (REST §4.13, landed); `cmd/kscore-gitops` + `internal/cli/gitops` (CLI, landed); `pkg/statemachine` (engine FSM); `pkg/dbutil` (SQLite stores); PROJECT-DETAILS §3.2 (K8s row — `client-go` deferred), §4.13 external-clients; Epic 16 tasks 7-10 (landed); companions: the GitOps webhook receiver + cluster/blueprint/runbook boot-wiring entries.

#### Outbound webhook Manager boot wiring (kscore-server)

- **Priority**: gate-v1.0
- **What**: Epic 16 tasks 11-18 ship `internal/webhook/outbound` — push-driven `Manager.Handle(ctx, events.Event)` with glob filter + bounded fan-out via `filepath.Match` + `sync.WaitGroup`; durable `SubscriptionStore` (memory + SQLite); `HTTPDispatcher` (HMAC-SHA256 signing + per-sub timeout); in-Manager retry loop with exp backoff + jitter; per-endpoint `CircuitBreaker` (5/30s/2); §4.14 REST routes on `pkg/api/webhooks` (subscription CRUD + `POST {id}/test` + `GET {id}/deliveries`, secret masked on every non-create response per §4.14); `cmd/kscore-webhook outbound` CLI (list/create/show/delete/history/test); public `Sign` + `Verify` helpers; and a build-tagged `test/e2e/webhook/` end-to-end suite (happy path / retry exhaustion / circuit-breaker) that drives the whole pipeline against a real httptest.Server receiver. `config.WebhookOutboundConfig` carries the §4.14 keys plus the task-12 tunables (`max_concurrent_deliveries`, `refresh_interval`); `webhook.outbound.enabled` defaults off (opt-in, the `gitops.webhook` precedent). It is **not yet constructed at `cmd/kscore-server` boot**: nothing subscribes `events.EventSubscriber` to route `kscore.<cluster>.events.>` deliveries into `Manager.Handle`, and the REST handler ships with empty `Providers{}` (routes return 503) until boot wires the live `Store` + `Manager`. Until boot lands, the package surface is unit + integration-tested only; a running kscore-server emits no outbound webhooks, and the `/api/v1/webhooks/subscriptions` routes are dark.
- **Why deferred**: same dependency-light / dark-until-boot root as the other Epic 16 boot entries — the Manager is push-driven precisely so the package stays free of the NATS / events subscriber API; the boot wiring (subscribe + route + select Memory vs SQLite store + construct Dispatcher + RetryPolicy + CircuitBreaker + pass to REST `Providers`) lands in one coherent place.
- **Acceptance for unblock**: kscore-server boot constructs the Manager over the SQLite `SubscriptionStore` with the task-14 `RetryPolicy` populated from `webhook.outbound` config and the Dispatcher wrapped by the task-15 circuit breaker, subscribes `events.EventSubscriber` to `kscore.<cluster>.events.>` and routes each delivery to `Manager.Handle`; the same Manager + Store are passed as `webhooks.Providers` so the §4.14 REST routes (today: 503) light up, and post-CRUD `Manager.Refresh` picks up new subscriptions immediately.
- **References**: `internal/webhook/outbound/{types,store,store_sqlite,manager,dispatcher,retry,circuit_breaker,sign}.go`; `internal/config/webhook.go` (`WebhookOutboundConfig`); `pkg/api/webhooks/handler.go` (REST §4.14, landed); `cmd/kscore-webhook` + `internal/cli/webhook` (CLI, landed); `test/e2e/webhook/` (build-tagged integration suite, landed); `internal/events/subscriber.go` (the subscriber API the boot path will consume); Epic 16 tasks 11-18 (landed, **Epic 16 CLOSED 18/18 / 13/13 acceptance**); companions: the GitOps webhook receiver + GitOps rollback boot-wiring entries.

#### Pre-rotation signing-CA retention in the bootstrap trust bundle

- **Priority**: gate-v1.0
- **What**: Epic 09 task 14's `SVIDBootstrapIssuer` populates `AgentCredentials.TrustBundlePEM` from `provider.GetTrustBundle().X509Authorities()` — which in v0.1 is just the root cert. `CAManager.RotateSigningCA` doesn't retain the previous signing CA cert, so an agent that bootstrapped before a rotation but disconnects through it needs to refresh its bundle from the server before its cached SVID chain verifies (the leaf still chains to a now-discarded signing CA whose public material the bundle no longer advertises). Acceptable for v0.5 single-CP (rotation rare, agents reconnect fast); multi-CP HA needs a grace window of retained signing CAs in the bundle.
- **Why deferred**: v0.5 single-CP single-server doesn't experience the disconnect-across-rotation scenario at scale (rotation is hourly-poll + 50%-of-lifetime). Multi-CP HA (Epic 13) introduces wider clock skew + more aggressive rotation; that's the natural place to land the retention window.
- **Acceptance**: `CAManager.RotateSigningCA` records prior signing CAs into a retention list; `BuildTrustBundle` includes the active root + retained pre-rotation signing CAs; retention window is config-driven (default 24h, matching MaxSVIDTTL); `SVIDBootstrapIssuer` and `BuildServerTLSConfig` both benefit transparently.
- **References**: `internal/identity/ca.go` CAManager; `internal/identity/embedded_provider.go` buildBundle; `internal/controlplane/bootstrap_jointoken.go` SVIDBootstrapIssuer; Epic 09 task 14 (landed); Epic 13 (clustering).

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
- **Why deferred**: gRPC + REST land together post-v1.0 per design.
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
- **Why deferred**: Gates on the SPIRE provider (post-v1.0). v1.0 rotation = restart.
- **Acceptance**: Agent re-issues NATS creds on cert rotation event without dropping the connection.
- **References**: Epic 06 scope-out (line 33); PROJECT-DETAILS §4.6 (line 482).

#### Rotation orchestrator

- **Priority**: gate-v1.0
- **What**: Strategies (blue-green, rolling, canary, immediate) with health checks + auto-rollback.
- **Why deferred**: v1.0 secrets ship with manual rotation; orchestration is its own domain.
- **Acceptance**: `kscore-secrets rotate` invokes a strategy, runs health checks, rolls back on failure.
- **References**: Epic 00 deferred list (line 77); PROJECT-DETAILS §4.11 (line 765).

#### Vault AWS IAM auth method

- **Priority**: gate-v1.0
- **What**: Epic 10 task 5 ships the Vault backend with Token / AppRole / Kubernetes / LDAP auth methods. AWS IAM auth (`vault/api/auth/aws`) is detected via the config-validation rejection path but not wired, because that sub-package pulls in `github.com/aws/aws-sdk-go-v2/credentials/stscreds` for the STS request-signing. AWS-EC2-hosted Vault deployments that prefer instance-profile auth are blocked on this.
- **Why deferred**: aws-sdk-go-v2 weight is real — adding it to v1.0 ships a heavier binary for every operator regardless of whether they use AWS. Trial-scope deployments (the v0.5 target) typically use Token or AppRole; production AWS deployments are the natural v1.0 audience for IAM.
- **Acceptance**: `Auth.Method=aws` with an `AWSAuthConfig{Role, Region, IAMServerIDHeader, ...}` round-trips against a `vault dev` server with the AWS auth method enabled; the AWS SDK dep is brought in only when AWS auth is configured (build tag or explicit constructor).
- **References**: `internal/secrets/vault/config.go` AuthConfig (current four-method enum); `github.com/hashicorp/vault/api/auth/aws` (the SDK helper); Epic 10 task 5 (landed).

#### Encrypted-file backend master-key rotation tooling

- **Priority**: gate-v1.0
- **What**: Epic 10 task 4 ships the encrypted-file backend with `ResolveMasterKey` (env / file / inline schemes). The on-disk envelope carries a key fingerprint so a wrong-key boot fails with a recognisable mismatch error, but there is no first-class way to rotate the master key without operator surgery: v0.1 procedure is stop server → decrypt with the old key via a one-off Go program → re-encrypt with the new key → start with new key. PROJECT-DETAILS §4.11 explicitly lists "Master key rotation breaks cache; mitigate via dual-key window" as a gotcha. A `kscore-secrets rotate-master-key --from <old-source> --to <new-source>` subcommand makes this a first-class operation.
- **Why deferred**: v0.5 single-CP single-server deployments tolerate the manual procedure (rotation is uncommon at trial scale); the dual-key window the gotcha calls for needs the broker-side cache (task 8) to honour two fingerprints simultaneously during a rotation window, which is a wider design than task 4 covers. v1.0 SemVer stability should not lock in a master-key story that has no rotation path.
- **Acceptance**: `kscore-secrets rotate-master-key` decrypts the state file under the old key, re-encrypts under the new key, persists atomically via the existing `writeAtomic`; the broker's running backends pick up the new key without process restart; the cache holds entries under both fingerprints during a configurable grace window then evicts the old.
- **References**: `internal/secrets/file/master_key.go` `ResolveMasterKey`; `internal/secrets/file/storage.go` envelope (`FingerprintLen` keyID); PROJECT-DETAILS §4.11 ("Master key rotation breaks cache; mitigate via dual-key window"); Epic 10 task 4 (landed); future Epic 10 task 8 cache.

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

#### Bootstrap phase handlers + durable checkpointer

- **Priority**: gate-v1.0
- **What**: Real `PhaseHandler` implementations for `kscore-bootstrap` (host detect via `/etc/os-release`, config-file writing, binary install + systemd unit, blueprint engine invocation, /health verify) plus a SQLite-backed `statemachine.Checkpointer` that survives process restarts.
- **Why deferred**: Epic 18 task 2 ships the FSM seam + `internal/selfmgmt.BootstrapManager` + an in-memory checkpointer + a recording test double. Real handlers depend on install paths, systemd helpers, and the blueprint runtime; they compose at the CLI layer (Epic 18 task 7).
- **Acceptance**: `kscore-bootstrap --seed dev-seed.yaml` runs end-to-end on a fresh Linux host and lands at `StateVerified`; SIGKILL mid-Configuring then restart resumes at `StateConfiguring`.
- **References**: Epic 18 task 2 (this commit); Epic 18 task 7; `internal/selfmgmt/bootstrap.go`.

#### Backup + restore component adapters (storage/JetStream/etcd/config/secrets/cluster)

- **Priority**: gate-v1.0
- **What**: Real adapter implementations behind the Epic-18 task-3 backup seams AND the task-6 restore seams: SQLite online-backup + Postgres `pg_dump` for write, file-replace + `pg_restore` for read; per-stream `nats.go/jsm` snapshots both ways; `clientv3.Snapshot()` + etcd snapshot restore; filesystem `ConfigCollector` + `ConfigRestore`; encrypted-file backend `Copy` + overwrite; MembershipManager+ShardStore-driven `ClusterMetadata` + `ClusterRestore`. Plus a real `ClusterDetector` wired from MembershipManager + the state store so the populated-cluster safety guard fires in production.
- **Why deferred**: Each adapter is a non-trivial dep dance (`pg_dump` / `pg_restore` shell-out vs Go-native, NATS Manager threading, etcd client coupling, secrets backend internals). Epic 18 tasks 3 + 6 ship the orchestrator + 12 narrow seams + manifest + tar artifact + integrity verification + populated-cluster guard so adapters land independently and get wired by the `kscore-backup` CLI (Epic 18 task 7).
- **Acceptance**: `kscore-backup create` then `kscore-backup restore` round-trips populated state onto a fresh server; restore over a populated server fails without `--force`; partial restore (`--config-only`) writes only the config files.
- **References**: Epic 18 tasks 3, 6, 7; `internal/backup/manager.go`; `internal/backup/restore.go`.

#### Policy CRUD via gRPC (`CreatePolicy`/`UpdatePolicy`/`DeletePolicy`/`activate`/`deactivate`/`remediate`/`monitor`)

- **Priority**: gate-v1.0
- **What**: Server-side mutating endpoints for policies (today returns Unimplemented).
- **Why deferred**: Mutations gate on enforcement going GA so operators can author policies that actually run.
- **Acceptance**: gRPC clients author policies via the API; CLI subcommands `create|update|delete|activate|deactivate|remediate|monitor` wire to the new endpoints.
- **References**: Epic 03 task spec (line 16); `pkg/api/policy/handler.go:5`; `pkg/api/v1/policy.pb.go:3`.

#### `kscore-agent service start|stop` subcommands

- **Priority**: gate-v1.0
- **What**: PROJECT-DETAILS §4.6 lists `service install|uninstall|start|stop|status`. v1.0 ships `install|uninstall|status` only.
- **Why now**: `systemctl start kscore-agent` / `systemctl stop kscore-agent` are universally known by Linux operators; wrapping them adds maintenance for zero ergonomic value. Picked up post-v1.0 because the Windows agent's SCM integration needs the same command shape and makes the wrapper worthwhile.
- **Acceptance for unblock**: `kscore-agent service start|stop` exist and proxy to the platform service manager; documented as the cross-platform form. Additive — no change to existing subcommands.
- **References**: Epic 06 task 9 `_(landed)_`; PROJECT-DETAILS §4.6 (line 473).

#### Batch dispatcher: no orphan-job recovery

- **Priority**: gate-v1.0
- **What**: Jobs in RUNNING state at process start are not auto-recovered; operators must inspect + retry.
- **Why now**: Orphan recovery needs ownership semantics (which CP node owns the job?) which settle once clustering has shipped — so the first post-v1.0 cleanup release.
- **Acceptance for unblock**: On startup the dispatcher reclaims/retries jobs it owns that were RUNNING; additive, no API change.
- **References**: `internal/controlplane/batch_dispatcher.go:72`.

#### API key issuance: non-transactional

- **Priority**: gate-v1.0
- **What**: `internal/controlplane.BootstrapHandler` issues credentials via separate write paths (not a single transaction).
- **Why now**: v1.0 doesn't have a multi-table tx wrapper (the multi-table-tx backlog entry above) — this is a pure consumer of that, so it tracks that work. Internal refactor, no external surface change.
- **References**: `internal/controlplane/bootstrap.go:270`.

#### Bootstrap: demo mode only (TUI + non-interactive)

- **Priority**: gate-v1.0
- **What**: Both `kscore-agent bootstrap` paths (TUI wizard from task 7 and `--non-interactive` flags from task 8) accept all three modes structurally but `bootstrap.ValidateForV10` rejects production / enterprise with a v1.x deferral message before the Engine reaches Validate.
- **Why now**: Production mode needs TLS cert collection (gates on Identity/cert tooling); enterprise mode needs blueprint selection. Production-mode unblock lands with the SPIRE/cert-rotation cycle; enterprise/blueprint pieces (the wizard screens) trail into the blueprint-runtime cycle — see that entry. Lifting the gate is purely additive for existing demo-mode users.
- **Acceptance for unblock**: TUI screens for cert paths + cert-generation toggle (production); equivalent `--generate-certs` CLI flag wired to the non-interactive path. Drop or no-op `bootstrap.ValidateForV10` for production.
- **References**: Epic 06 tasks 7 + 8 `_(landed)_`; `internal/agent/bootstrap/configure.go` (search `ValidateForV10`); `cmd/kscore-agent/main.go` (search `buildConfigurer`).

#### Bootstrap CLI flags dropped from v1.0 surface

- **Priority**: gate-v1.0
- **What**: The original Epic 06 task 8 spec listed `--postgres-*`, `--nats-*` beyond `--join`/`--join-token`, `--generate-certs`, and `--apply-blueprint`. v1.0 ships without them.
- **Why now**: `--postgres-*` is server-only and belongs in the future unified `kscore-bootstrap` binary (the agent doesn't run a database). Extra `--nats-*` flags are unnecessary because v1.0 agents are external-mode only — embedded NATS is v2.x+. `--generate-certs` re-appears with the cert tooling; `--apply-blueprint` trails to the blueprint runtime. All additive flag additions.
- **Acceptance for unblock**: Per-flag — when its blocking work lands, add the flag with appropriate plumbing. The `--state-path` flag added in task 8 stays.
- **References**: Epic 06 task 8 `_(landed)_`; `cmd/kscore-agent/main.go` (search `registerBootstrapFlags`).

#### Bootstrap auto-installs systemd unit (production mode)

- **Priority**: gate-v1.0
- **What**: When demo-only mode-gate lifts, the bootstrap Engine's Install phase should call `systemd.Install` automatically — operator runs `kscore-agent bootstrap` once, gets both config and unit installed/enabled.
- **Why now**: Downstream of production mode (above); rides the same SPIRE/cert-rotation cycle. Convenience chaining, additive — the explicit two-step flow still works.
- **Acceptance for unblock**: When `bootstrap.ValidateForV10` lifts for production, `bootstrap.NewDefaultInstaller` (or a production-mode wrapper) chains `systemd.Install` after the YAML render.
- **References**: Epic 06 task 9 `_(landed)_`; `internal/agent/bootstrap/install.go`; `internal/agent/systemd/install.go`.

#### Boostrap PSK consumption: in-memory tracking only

- **Priority**: gate-v1.0
- **What**: Used PSKs are tracked in-memory. Server restart re-permits a previously-used PSK.
- **Why now**: Persistence path needs the bootstrap-state-store work, which rides the bootstrap-hardening cycle. Bug-fix-shaped (makes restart behaviour correct), not a compatibility break.
- **References**: `internal/config/nats.go:53`; Epic 05 task 9 `_(landed)_`.

#### Type=notify systemd integration (sd_notify)

- **Priority**: gate-v1.0
- **What**: v1.0 unit uses `Type=exec`. `Type=notify` would let systemd track agent readiness via `sd_notify("READY=1")` calls — useful for `Wants=kscore-agent.service` ordering and reliable health checks.
- **Why now**: Requires the daemon to call into `coreos/go-systemd/v22/daemon`'s `SdNotify`, the dep explicitly skipped for v1.0. The telemetry-gateway work is the first real consumer of agent readiness signalling. New unit + daemon ship together, so no break for existing installs.
- **Acceptance for unblock**: Daemon calls `SdNotify("READY=1")` after NATS connect + initial heartbeat publishes; unit flips to `Type=notify`; `systemctl is-active` reports `activating` until ready.
- **References**: Epic 06 task 9 `_(landed)_`; `internal/agent/systemd/unit.go` (search `Type=exec`).

#### Bootstrap wizard: storage backend + blueprint selection screens

- **Priority**: gate-v1.0
- **What**: PROJECT-DETAILS §4.6 + Epic 06 task 7 originally envisioned the agent wizard collecting storage backend and applying blueprints. Both were dropped from the v1.0 agent surface — storage is server-only (the future unified `kscore-bootstrap` binary's concern); blueprint apply gates on the blueprint runtime.
- **Why now**: Blueprint apply needs the plugin/module system + the full blueprint catalogue, which is the blueprint-runtime cycle's headline. Storage screens still wait on the unified binary (no committed release) and may slip further. Additive wizard screens — existing wizard flows unchanged.
- **Acceptance for unblock**: For blueprints — the blueprint runtime lands, then a "select blueprints to apply" screen feeds an installer-side blueprint apply step. For storage — `cmd/kscore-bootstrap` (unified server+agent binary) gains the storage screens.
- **References**: Epic 06 task 7 `_(landed)_`; PROJECT-DETAILS §4.6 (line 451).

#### Bootstrap: no rollback / transactional revert

- **Priority**: gate-v1.0
- **What**: Bootstrap engine resumes from checkpoint but doesn't revert side effects (config files, systemd units) on failure.
- **Why now**: Rollback semantics need install-step inversion + snapshot tracking. Slotted with the compliance/scan-scheduler cycle's general hardening; new `--rollback` flag is additive.
- **Acceptance**: Failed bootstrap re-runs cleanly; for true rollback, operator runs `kscore-agent bootstrap --rollback`.
- **References**: Epic 06 task 6; `internal/agent/bootstrap/doc.go:19`.

#### Dedicated `keystone-core` system user auto-creation

- **Priority**: gate-v1.0
- **What**: v1.0 systemd unit defaults to root; `--user/--group` flags let operators run as a dedicated user, but the user must already exist (no auto-create).
- **Why now**: User creation belongs in package-mgmt territory (rpm/deb post-install via `useradd --system`), and packaging is part of the air-gap / supply-chain work. The default-user flip is handled inside the package post-install (not as a surprise to in-place tarball upgrades), so it stays non-breaking.
- **Acceptance for unblock**: Packaging creates the `keystone-core` system user as part of `apt install` / `dnf install`. After that, `service install` can default `--user keystone-core --group keystone-core` and update the rendered ReadWritePaths to match.
- **References**: Epic 06 task 9 `_(landed)_`; the air-gap / supply-chain packaging work.

#### Config: no per-endpoint TLS overrides

- **Priority**: gate-v1.0
- **What**: `cfg.NATS.Endpoints[]` use the cluster-wide TLS config; per-endpoint overrides are reserved schema fields.
- **Why now**: v1.0 has one TLS strategy per cluster; mixed-TLS topologies become relevant alongside the v2.x+ multi-region work, but the override fields already exist and wiring them is additive (existing configs keep working), so it slots into later platform-polish rather than waiting for that v2.x+ work.
- **References**: `internal/config/nats.go:126`.

#### Cluster-wide HMAC secret (vs per-agent)

- **Priority**: gate-v1.0
- **What**: All agents share one HMAC secret in v1.0. Per-agent keys derived from the bootstrap exchange replace it.
- **Why now**: Breaking change — it changes the agent↔server authentication model and needs a key-distribution mechanism still being designed; lands with the v2.x+ auth/security infra changes (cloud KMS, federation).
- **Acceptance for unblock**: Bootstrap exchange establishes a per-agent key; server authenticates inbound by agent identity; cluster-wide secret removed (or relegated to a legacy compatibility window decided at design time).
- **References**: Epic 06 task 6 `_(landed)_`; `internal/agent/security.go:71`; `internal/config/security.go:15`.

## v0.x — desirable pre-v1.0 (no specific gate)

#### Rewrite GETTING-STARTED.md around the operator (package-install) path

- **Priority**: v0.x
- **What**: Current `docs/project/GETTING-STARTED.md` walks a stranger through the docker-compose dev harness (clone source, `make e2e-up`, grpcurl). That's the contributor/developer path, not the operator path. Rewrite around `apt install kscore-server kscore-cli` → `systemctl start` → `kscorectl exec/state apply/audit log` so a v0.1.x trial install fits the "GitOps deploys it, we keep it running" positioning. Keep the docker-compose walkthrough as a secondary "for contributors" section.
- **Why deferred**: surfaced during Phase D1 of the public-launch checklist; rewriting the doc is bigger than the Phase D fix-cycle. Package-install path is now functional (commit `4be8d19d` shipped postinst + default config); the docs just don't show it as the primary path yet.
- **Acceptance**: `docs/project/GETTING-STARTED.md` leads with `apt install ./kscore-server*.deb ./kscore-cli*.deb && systemctl start kscore-server && curl /health/ready`; §§4-7 use `kscorectl audit log` / `kscorectl exec run` / `kscorectl state apply` instead of grpcurl; docker-compose path moved to a `For contributors` section; the §troubleshooting "REST alternative at :8080" claim removed (or made true — see entry below); F1 (`jq` not in prereq line), F2 (`golang-1.26` apt package non-existent on jammy + tarball fallback uncommented), F4 (`/health/ready` grace-period response shape), F6 (`exit_code:0` proto3 default), F7 (first-run `make e2e-up` 30-90s upper bound widened) all addressed at the same time.
- **References**: `docs/project/GETTING-STARTED.md` current state; `docs/project/PUBLIC-LAUNCH-CHECKLIST.md` D1 evidence note (Pass A + Pass B); deploy/packaging/* (postinst rendering the default config); deploy/config/server.yaml.template (the default config the new docs would reference).

#### `kscorectl agent list` subcommand

- **Priority**: v0.x
- **What**: `kscorectl` ships with built-in `exec` and `state` subcommands plus the dispatcher pattern that routes to `kscore-<plugin>` binaries on PATH (audit, secrets, files, …). No `kscorectl agent list` exists; the only path to "show me the registered agents" today is grpcurl against `ControlPlaneService/ListAgents`. Add either a top-level `agent` subcommand in `internal/cli/` or a separate `kscore-agent-cli` binary that dispatches to it (distinct from the `kscore-agent` runtime binary).
- **Why deferred**: surfaced as a gap during Phase D1 operator-path walkthrough; not blocking the .deb/.rpm install smoke, but operator can't see fleet state without grpcurl. The control-plane RPC is there + tested; just no CLI on top.
- **Acceptance**: `kscorectl agent list` returns a table of registered agents with id / status / labels / last-heartbeat; `--output json` works; integrates with the standard `--server` / `--api-key` flag pattern from `internal/cli/exec/` + `state/`; smoke test under `internal/cli/agent/` matches the audit/state coverage bar.
- **References**: `api/proto/keystone/core/v1/controlplane.proto` `ControlPlaneService.ListAgents` RPC; `internal/cli/cli.go` RootCommand pattern; `cmd/kscorectl/main.go` newCommand `AddCommand(exec.NewCommand(...))` + `AddCommand(state.NewCommand(...))`; `docs/project/PUBLIC-LAUNCH-CHECKLIST.md` D1 evidence (operator-path Pass B).

#### REST stubs for control-plane RPCs — decide: remove docs claims or implement

- **Priority**: v0.x
- **What**: `pkg/api/agents/handler.go`, `pkg/api/state/handler.go`, `pkg/api/schedule/handler.go` register routes (`GET /api/v1/agents`, `POST /api/v1/state/apply`, etc.) that all return HTTP 501 via `notImplemented`. The GETTING-STARTED.md §4 footnote ("If you don't have grpcurl handy, the REST equivalent: …") and the §troubleshooting "Alternative: drive everything via REST at :8080" both promise this surface works — it does not. Either (a) implement the REST→gRPC passthrough handlers so the docs' claim becomes true, or (b) remove the false claims as part of the GETTING-STARTED.md rewrite.
- **Why deferred**: surfaced as F5 during Phase D1 (docker-compose walkthrough). Implementing the passthrough is the bigger-but-more-useful option; removing the claims is the cheap-but-narrowing option. Either way the docs need to stop lying.
- **Acceptance**: pick a direction. If (a): `GET /api/v1/agents` returns the same JSON shape as `ControlPlaneService.ListAgents`; `POST /api/v1/state/apply` accepts base64'd YAML + agent id and streams back; existing 501-stub tests in `pkg/api/agents/handler_test.go` + `pkg/api/state/handler_test.go` flip to 200 + content assertions. If (b): the 501 handlers stay; GETTING-STARTED.md §4 + §troubleshooting drop the REST-alternative claims; the route handlers move from "stub" to "intentionally not part of v0.1 — gRPC only" with a doc comment.
- **References**: `pkg/api/agents/handler.go` + `state/handler.go` + `schedule/handler.go` (stub registrations); `docs/project/GETTING-STARTED.md` §4 footnote + §troubleshooting "Alternative: drive everything via REST"; `docs/project/PUBLIC-LAUNCH-CHECKLIST.md` D1 evidence F5.

#### kscore-server default gRPC port collides with Cockpit on Rocky 10

- **Priority**: v0.x
- **What**: `kscore-server`'s default gRPC port is `127.0.0.1:9090`. Cockpit (the RHEL-family web admin UI) listens on `:9090` by default — and Rocky 10 ships with `cockpit.socket` enabled out-of-box. Result: `apt install kscore-server && systemctl start kscore-server` fails on Rocky 10 with an undisclosed bind error (the cli.go RootCommand swallows it via `SilenceErrors: true`, so the systemd journal only shows "stopped" / status=1/FAILURE). Rocky 8/9 don't enable Cockpit by default so the conflict isn't observed there. Decide: (a) change `kscore-server`'s default gRPC port to something less popular (50051 is the gRPC convention), with the cost of touching every CLI subcommand's `--server` default; (b) detect the conflict in postinst + write a non-conflicting port into `/etc/kscore/server.yaml` (a `ss -tlnp` check at install time); (c) document the conflict in GETTING-STARTED.md and tell Rocky 10 operators to `systemctl stop cockpit.socket` or pick a different port.
- **Why deferred**: surfaced as F13 during Phase D2 on rocky10; not blocking the broader Phase D pass since 5/6 distros work and rocky10 install completes cleanly after the conflict is resolved manually. The right fix is a real engineering decision, not a hot-patch.
- **Acceptance**: pick a direction. If (a): default port changes in `deploy/config/server.yaml.template` + `internal/config/config.go` defaultConfig + every `internal/cli/*/` subcommand's `--server` default + docs/runbooks. If (b): postinst probes `ss -tln` for 9090 + falls back to 9091 (or similar) + logs a one-line WARNING that the operator should be aware of. If (c): GETTING-STARTED.md and the v0.x install runbook gain a "Rocky 10 / RHEL 10 note" callout box + the Phase D evidence stays as documented record.
- **References**: `deploy/config/server.yaml.template` `server.grpcport`; `internal/config/config.go` defaultConfig `GRPCPort: 9090`; `internal/cli/*/`(many subcommands) `--server` default `localhost:9090`; Cockpit upstream docs (default 9090); `docs/project/PUBLIC-LAUNCH-CHECKLIST.md` D2 evidence F13; Phase D rocky10 journalctl excerpt (silent exit-1 right after "controlplane: command dispatcher started").

#### kscore-server: surface fatal-init errors to stderr instead of silently exiting

- **Priority**: v0.x
- **What**: `internal/cli/cli.go` builds the cobra root command with `SilenceErrors: true`. When `kscore-server` fails during `run()` — e.g. gRPC bind conflict (the Rocky 10 cockpit case above), invalid config, metrics-registry collision — cobra swallows the wrapped error and `main()` does a bare `os.Exit(1)` with no message. The systemd journal shows only the routine `"stopped"` info line + `status=1/FAILURE` from systemd. Operators have to attach a debugger or read source to diagnose.
- **Why deferred**: surfaced during Phase D rocky10 diagnosis. Easy fix (print the runErr to stderr before exiting in main()), but spans every cobra-rooted binary (kscore-server, kscore-agent, kscorectl, kscore-bootstrap, kscore-files, kscore-backup…) so wants a coordinated touch.
- **Acceptance**: every `cmd/kscore-*/main.go` `main()` that calls `newCommand().Execute()` checks the returned error and writes it to stderr (`fmt.Fprintln(os.Stderr, "error:", err)`) before `os.Exit(1)`. The systemd journal shows `kscore-server: error: gRPC: listen tcp 127.0.0.1:9090: bind: address already in use` instead of just `stopped` then `status=1/FAILURE`.
- **References**: `internal/cli/cli.go` `RootCommand` (SilenceErrors=true is appropriate — the wrapper is fine, main() just needs to print before exit); `cmd/kscore-server/main.go` `main()` `if err := newCommand().Execute(); err != nil { os.Exit(1) }`; `docs/project/PUBLIC-LAUNCH-CHECKLIST.md` D2 evidence (the half-day diagnosis to find Cockpit was the cost of the missing stderr message).

#### Phase D3: macOS dev-build sanity test

- **Priority**: v0.x
- **What**: `docs/project/PUBLIC-LAUNCH-CHECKLIST.md` Phase D3 calls for `make build` + `make test` on real macOS hardware to validate the `internal/statemgmt/stdlib/fs_unix.go` / `fs_windows.go` split + the cross-platform code paths. The Phase D rerun for v0.1.x deferred D3 because no maintainer/contributor currently has Mac access. Cross-compile via goreleaser already produces darwin amd64 + arm64 archives; what's missing is running the tests on a real Mac kernel.
- **Why deferred**: hardware availability. The build target compiles cleanly via cross-compile on every release; the test gap is specifically about runtime behavior on Darwin.
- **Acceptance**: a maintainer or named near-term contributor runs `make build && make test` (or `make check`) on macOS 14+ (Apple Silicon and/or Intel), reports the results, and either ticks D3 in PUBLIC-LAUNCH-CHECKLIST.md or files specific Darwin-runtime bugs as separate roadmap entries.
- **References**: `docs/project/PUBLIC-LAUNCH-CHECKLIST.md` D3; `.goreleaser.yaml` darwin builds; `internal/statemgmt/stdlib/fs_unix.go` + `fs_windows.go`.

#### Persist dispatch principal on CommandRecord for audit-log User field

- **Priority**: v0.x
- **What**: Epic 12 task 4 wired `controlplane.TerminalCommandFunc` audit emission for command exec. The agent-reported terminal path (`CommandDispatcher.RecordResult`) emits with empty `User` because `state.CommandRecord` has no `Principal` column — the dispatch-time principal in `DispatchRequest.Principal` is stamped into the NATS `CommandMessage` but isn't persisted. The dispatch-time error paths (marshal-fail, publish-fail) and the sweepTimeouts path that fetches the record still emit with the correct principal because those callers have the request in scope; only the agent-reported success path lands a blank actor.
- **Why deferred**: schema change (add `principal TEXT` column to `commands` in both SQLite + Postgres schemas, plus types + sqlite_commands.go + postgres_commands.go) is outside Epic 12 task 4's touched scope. The audit row is still useful without it — `agent_id` + `command` + `status` identify the op; the missing field is the human/SPIFFE actor for compliance review.
- **Acceptance**: `state.CommandRecord.Principal` field landed; `commands.principal` column in both schemas; `Dispatch` writes the principal at `CreateCommand` time; the audit row for a `RecordResult`-driven terminal carries the dispatch principal in `User`.
- **References**: `internal/state/types.go` CommandRecord; `internal/controlplane/command_dispatcher.go` Dispatch + RecordResult + sweepTimeouts; `cmd/kscore-server/command_emitter.go` user fallback to `record.User`; Epic 12 task 4 (landed).

<!-- "`service` stdlib module — make systemdRunDir test-mutable
     without a package-level global" landed during Phase C1
     gauntlet (public-launch checklist): defaultProvider now takes
     systemdRunDir as a parameter; package-level `var` is gone.
     `go test -race ./internal/statemgmt/stdlib/service/...` clean.
     Entry retired from this list; the test parameterization is
     visible in the code. -->

#### Vault auto re-authentication on token expiry

- **Priority**: v0.x
- **What**: Epic 10 task 5's `tokenRenewer` keeps the Vault auth token alive by calling `auth/token/renew-self` shortly before expiry. If renewal fails (network glitch, Vault restart, policy change), the renewer logs WARN and the backend's `Health` reports unhealthy — operators must restart the server to recover, since the renewer doesn't try to re-login from the stored credentials. For unattended trial deployments this means a transient Vault outage can require manual intervention.
- **Why deferred**: re-login from stored credentials needs careful thought about credential lifecycle. AppRole SecretIDs can be single-use, in which case re-login fails the same way; Token auth credentials don't change. The right behavior is method-aware and adds enough complexity to deserve its own task.
- **Acceptance**: On `auth/token/renew-self` failure, the renewer re-invokes the configured auth method to obtain a fresh token and resumes the renewal loop on success; method-specific lockout heuristics (skip re-login if the previous N attempts failed) prevent runaway login storms.
- **References**: `internal/secrets/vault/token_renewer.go`; `internal/secrets/vault/client.go` authenticate; Epic 10 task 5 (landed).

#### Bootstrap protocol versioning

- **Priority**: v0.x
- **What**: Epic 09 task 14 extended `controlplane.AgentCredentials` from three JSON fields to six (added `cert_chain_pem`, `private_key_pem`, `trust_bundle_pem`). All new fields use `omitempty` so older agents reading the JSON ignore them, but a formal `protocol_version` field would make compatibility explicit + give operators a clean knob for opt-in protocol upgrades down the line.
- **Why deferred**: cosmetic. Both wire shapes (Epic 05 task 9 three-field form + Epic 09 task 14 six-field form) round-trip cleanly through `encoding/json` and the agent runtime treats new fields as additive. Adding explicit versioning is forward-thinking but not v0.5-blocking.
- **Acceptance**: `AgentCredentials.ProtocolVersion uint32` field; bootstrap response carries the current version; agents check + log on mismatch.
- **References**: `internal/controlplane/bootstrap.go` AgentCredentials; Epic 09 task 14 (landed).

#### Direct signing-cert accessor on CAManager

- **Priority**: v0.x
- **What**: Epic 09 task 12's `IdentityGRPCServer.ExportCA WHAT_SIGNING` and `GetStatus SigningExpiresAt` paths currently issue a throwaway 1-minute probe cert to reach the signing CA's cert (`issued.Chain[1]`). Works, but it's awkward — every probe consumes a serial number + briefly bloats the on-disk metadata. Add `CAManager.SigningCACert() *x509.Certificate` (and `SigningCAExpiresAt() time.Time` for the status path) so the server reads the in-memory state directly.
- **Why deferred**: cosmetic. The probe-issue path is correct + tested; the cleanup is an internal refactor with no external behavior change.
- **Acceptance**: `IdentityGRPCServer.ExportCA WHAT_SIGNING` returns the same PEM without invoking `IssueCertificate`; `GetStatus.SigningExpiresAt` reads from `SigningCAExpiresAt()`; the throwaway-probe path is removed.
- **References**: `internal/controlplane/grpc_identity_server.go` ExportCA + GetStatus; `internal/identity/ca.go` CAManager.

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
- **Why now**: stdlib `path.Match` covers the v1.0 command-allowlist use cases. Small, self-contained; folded into the linting/capabilities work. Note: broadening allowlist matching is a behaviour change to watch in review, but not a compatibility break.
- **References**: `internal/agent/security.go:59`.

#### Migration journal: no per-table checkpoint resume

- **Priority**: v0.x
- **What**: `kscore-migrate` records per-table checkpoints in the txlog but recovery from a partial migration restarts from the last full-table boundary, not the row-level checkpoint.
- **Why now**: Row-level resume needs a transactional checkpoint protocol that v1.0's `state.Tx` (deferred post-v1.0) would unlock. Improvement to recovery only; no compatibility break.
- **References**: `internal/state/migrate_txlog.go:25`.

## v1.x — post-v1.0 feature additions

#### Hugo docs site

- **Priority**: v1.x
- **What**: The v1.0 reference docs live as Markdown under `docs/project/` (per AGENTS.md §5: "Until the Hugo site lands (post-v1.0), the canonical doc surfaces are README.md, epics/NN-*.md, docs/project/*.md"). v1.x migrates them into a Hugo + Docsy site under `docs/content/en/docs/...` with the layout the epic-19 §Documentation block originally called for (reference/, getting-started/, operations/).
- **Why deferred**: Hugo setup + Docsy theming + per-page front-matter + CI build pipeline is its own scope. The v1.0 Markdown docs are complete and discoverable — graduating them to a rendered site is a polish task, not a release blocker.
- **Acceptance**: `docs/` builds a Hugo site under `docs/public/`; auto-generated CLI / config / API references regenerate via `make docs-sync` into the Hugo content tree; the published site mirrors the structure of `docs/project/` with per-page navigation.
- **References**: epic 19 task 9 `_(landed)_`; `docs/project/{CLI,CONFIGURATION,API}-REFERENCE.md` + `GETTING-STARTED.md`.

#### Expanded getting-started guides (per-domain tutorials)

- **Priority**: v1.x
- **What**: Epic 19 §Documentation listed five getting-started guides (install, first cluster, first command, first state apply, first blueprint, first module). v1.0 ships one consolidated 30-minute walkthrough at `docs/project/GETTING-STARTED.md` that covers install + topology + command + state. v1.x expands to per-domain tutorials covering blueprint authoring, module authoring + publishing, secrets, GitOps integration, audit/policy queries, HA cluster topology.
- **Why deferred**: One thorough walkthrough satisfies the epic-19 acceptance line ("Quick-start guide can be followed end-to-end on a fresh Ubuntu VM in <30 minutes"). The longer-form per-domain content is Hugo-era editorial work.
- **Acceptance**: Six guides under `docs/content/en/docs/getting-started/` (or equivalent post-Hugo); each is runnable end-to-end on a fresh Ubuntu VM; each is link-checked.
- **References**: epic 19 task 9 `_(landed)_`; `docs/project/GETTING-STARTED.md`.

#### Operations runbooks v1.x sweep

- **Priority**: v1.x
- **What**: `docs/runbooks/` carries pre-v1.0 operational content (`bootstrap-new-cluster.md`, `backup-restore.md`, `capacity-scaling.md`, `security-incident.md`). v1.x sweeps them for accuracy against the v1.0 release — current binary names, current CLI flags, current config keys, current REST/gRPC endpoints — and graduates them into the Hugo `docs/content/en/docs/operations/` tree.
- **Why deferred**: The runbooks are correct in shape (procedures, decision trees, recovery steps) but contain references to interfaces that have since shifted during reconstruction. A sweep against the now-frozen v1.0 contracts is post-release work.
- **Acceptance**: Each runbook step is verified against a fresh `make e2e-up` topology; commands run as documented; outputs match.
- **References**: epic 19 task 9 `_(landed)_`; `docs/runbooks/*`.

#### Soak-test infrastructure for fd / connection / goroutine leaks

- **Priority**: v1.x
- **What**: Epic 19 §Acceptance line 109 asks for "no goroutine / connection / fd leaks in 1-hour soak test." Task 6 (goleak in integration tests) and task 8 (hardening pass — per-component lifecycle audit in `docs/project/HARDENING-BASELINE.md`) shipped the v1.0 mechanisms; the 1-hour soak harness itself ships post-v1.0. v1.x adds: a soak target (`make soak` or similar) that runs the topology under representative load for N hours, captures `lsof` baselines + diffs, snapshots goroutine count via `runtime.NumGoroutine`, and asserts each is bounded.
- **Why deferred**: The mechanism is in place (goleak under integration, lifecycle table for connections). Building the soak orchestration — load generator, time-boxed runner, lsof differ, goroutine snapshotter, CI integration with a long-running job — is dedicated scope.
- **Acceptance**: `make soak` runs the docker-compose topology under sustained load for ≥1 hour, asserts: (a) `runtime.NumGoroutine` flat ±5%; (b) per-process `lsof` count flat ±5%; (c) per-process RSS flat ±10%. CI runs it nightly.
- **References**: epic 19 task 6 `_(landed)_`; epic 19 task 8 `_(landed)_`; `docs/project/HARDENING-BASELINE.md`.

#### Sustained-load profiling baseline

- **Priority**: v1.x
- **What**: Epic 19 task 8 captured a CPU + allocation baseline against the perf SLO workload (`make profile`; `docs/project/PROFILING-BASELINE.md`). That workload is ~250 ms total — useful for catching obvious hot spots, insufficient for production-shape profiling. v1.x adds a sustained-load harness (10-minute-ish runs against a multi-agent topology with steady command + event + state-apply load) and a per-domain profile breakdown (state engine, blueprint, secrets, audit).
- **Why deferred**: The v1.0 baseline catches the bar the epic asked for (>5% CPU / >50 MB alloc). Production-shape profiling is operations-tuning work, not a v1.0 release blocker.
- **Acceptance**: `make profile-sustained` (or equivalent) runs the harness; per-domain profile files land next to `docs/project/PROFILING-BASELINE.md`'s baseline numbers; the doc gains a per-domain section.
- **References**: epic 19 task 8 `_(landed)_`; `docs/project/PROFILING-BASELINE.md`.

#### Error-message docs URLs (post Hugo site)

- **Priority**: v1.x
- **What**: Many error messages would benefit from a docs URL (`See https://keystone-core.io/docs/errors/<slug>` style). Epic 19 §Hardening calls this out; epic 19 task 8 deferred it because the Hugo docs site is post-v1.0. v1.x adds the URLs once the docs site is live + has the per-error slug pages.
- **Why deferred**: URLs to a non-existent docs site would rot.
- **Acceptance**: A `pkg/api/apierror` (or similar) helper produces "<message>. See <docs-url>" strings; the docs site has the matching slug pages; key user-facing errors (config validation, secrets read, command exec failures) carry the URLs.
- **References**: epic 19 task 8 `_(landed)_`; the Hugo docs ROADMAP entry (post-v1.0).

#### Logging: context-aware threading of deep helpers

- **Priority**: v1.x
- **What**: 122 `slog.Info/Warn/Error/Debug` (non-context) calls live in deep helpers (collectors, init paths, shutdown helpers) that don't have `ctx` in scope. They drop the correlation ID that the request-scoped path threads via `logging.WithCorrelationID`. v1.x graduates each site to `slog.*Context` by threading `ctx` through the relevant call chains (or accepting a logger that carries the correlation ID).
- **Why deferred**: Each site is small but the threading is cross-cutting; bundling it into task 8's hardening pass would balloon scope. Request-scoped logging already carries the correlation ID — the gap is only in deep helpers.
- **Acceptance**: `tools/logaudit` (or `grep` audit) reports zero non-context `slog.*` calls outside an explicit allowList; allowList entries each name why the site can't take a ctx (e.g., process-wide init).
- **References**: epic 19 task 8 `_(landed)_`; `docs/project/HARDENING-BASELINE.md` "Logging audit" section.

#### Security baseline expansion

- **Priority**: v1.x
- **What**: Epic 19 task 7 shipped the v1.0 baseline pipeline: `make security-{secrets,vulns,sast,licenses}` running gitleaks, govulncheck, gosec (HIGH-only, G115 excluded), go-licenses (strict). v1.x expands the scan set + tightens the configuration:
  - **Re-enable G115 + per-site audit**. Currently ~84 G115 (integer-overflow conversion) findings sit at proto<->Go field-width boundaries, parser-checked archive headers, and range-validated config. Re-enabling the rule requires a per-site audit + `//#nosec G115` annotations (or, where the conversion is genuinely unbounded, a `range check + return error` fix).
  - **semgrep** — cross-language SAST; catches patterns gosec doesn't (SSRF, template injection, regexp DoS).
  - **trivy** / **grype** — container-image + lockfile scanning; lands with the v1.0 release-image build.
  - **syft** — SBOM generation; lands with epic 19's release-artifact work.
  - **hadolint** — Dockerfile linting (`test/e2e/single/Dockerfile.kscore` + the prod release Dockerfile when it lands).
  - **gosec MEDIUM gate** — current baseline gates HIGH only per epic 19 acceptance. v1.x graduates MEDIUM to a required floor after a one-time triage pass.
- **Why deferred**: The v1.0 baseline ships the four scans the epic explicitly names. Expanding to the broader suite is post-v1.0 release-prep, not a v1.0 blocker. The G115 re-enablement specifically would require a large triage PR with little signal-to-noise improvement for the v1.0 codebase.
- **Acceptance**: Each sub-bullet adds its own Make target + CI step + brief docs entry under `docs/project/SECURITY-GOVERNANCE.md` "Security Baseline Pipeline." `make security` (or equivalent umbrella) runs all.
- **References**: epic 19 task 7 `_(landed)_`; `Makefile` `security-*` targets; `docs/project/SECURITY-GOVERNANCE.md` "Security Baseline Pipeline" section.

#### Release dry-run expansion

- **Priority**: v1.x
- **What**: Epic 19 task 13 shipped `make release-dry-run`: goreleaser snapshot + `scripts/release-smoke.sh` asserting checksum, archive content, linux-binary `--version`, deb/rpm content (via `debian:12-slim`), and opt-in deb/rpm install smoke in fresh `debian:12-slim` + `rockylinux:9` containers. v1.x rounds out the smoke surface:
  - **SBOM generation + verification**. `.goreleaser.yaml` notes SBOM (CycloneDX + SPDX) is out-of-scope for v1.0; v1.x adds it via a separate `make security-sbom` path + a smoke assertion that an SBOM file is present and parseable.
  - **Cross-arch install smoke**. v1.0 ships arm64 packages but the install smoke only exercises the host-arch package. v1.x adds `qemu-user-static` / `binfmt-misc` registration to run arm64 install smoke on amd64 hosts.
  - **`systemd-analyze verify`** on the installed unit files. Requires `systemd` in the smoke container, inflating pull size; deferred until SBOM lands so the trade-off is made once.
- **Why deferred**: The v1.0 smoke covers every artifact-shape failure mode that has shown up in practice (the task 13 implementation surfaced 15 binaries missing `--version` wiring + 6 regex anchor bugs in the smoke script itself; both fixed inline). The remaining gaps are belt-and-suspenders, not v1.0 blockers.
- **Acceptance**: Each sub-bullet adds its own check function in `scripts/release-smoke.sh` (or the container companion) and a row in `docs/project/SECURITY-GOVERNANCE.md` "Release Dry-Run Smoke."
- **References**: epic 19 task 13 `_(landed)_`; `scripts/release-smoke.sh`; `docs/project/SECURITY-GOVERNANCE.md` "Release Dry-Run Smoke" section.

#### Dependency posture re-audit

- **Priority**: v1.x
- **What**: A periodic deep-audit of the direct + indirect Go dependency tree. The v0.x baseline audit (2026-05-23, Phase B3 of [`PUBLIC-LAUNCH-CHECKLIST.md`](PUBLIC-LAUNCH-CHECKLIST.md)) flagged a small set of single-maintainer / lightly-maintained deps that are acceptable for v0.x but warrant explicit re-evaluation before v1.0 / v2.0. Specifically: (a) **`lib/pq` → `jackc/pgx` graduation** — lib/pq is in maintenance-only mode; pgx is the community-active Postgres driver; (b) **`gobwas/glob` re-audit** — minimal-scope glob matcher, feature-complete per maintainer, no known CVEs but worth re-validating; (c) **single-maintainer-but-active deps re-check** — `modernc.org/sqlite`, `santhosh-tekuri/jsonschema/v6`, `shirou/gopsutil/v4`.
- **Why deferred**: No security signal warrants action at v0.x. The audit's purpose is to keep the dependency profile honest before SemVer stability kicks in at v1.0.
- **Acceptance**: A timestamped re-audit lands in [`docs/project/SECURITY-GOVERNANCE.md`](SECURITY-GOVERNANCE.md) "Dependency posture" section with the new module count + license distribution + maintainer-health categorization. Any newly-introduced single-maintainer dep gets an explicit accept/replace decision.
- **References**: `docs/project/SECURITY-GOVERNANCE.md` "Dependency posture" section (2026-05-23 baseline); `go.mod` direct + indirect blocks.

#### Rate-limit: `Retry-After` HTTP-date format alternative

- **Priority**: v1.x
- **What**: RFC 7231 allows `Retry-After` to carry either a delta-seconds integer or an HTTP-date. v1.0 ships delta-seconds only (`internal/ratelimit/middleware/http.go::writeRejected429`). v1.x adds an opt-in HTTP-date variant for proxies / caches that prefer the absolute form.
- **Why deferred**: Delta-seconds is the more widely-supported shape and satisfies the v1.0 acceptance line. HTTP-date is operator preference; no concrete demand yet.
- **Acceptance**: An operator flag selects between formats; tests cover both round-trips through a representative client.
- **References**: `internal/ratelimit/middleware/http.go`; RFC 7231 §7.1.3.

#### Rate-limit: configurable gRPC `retry-after-ms` trailer key

- **Priority**: v1.x
- **What**: The gRPC interceptor emits a `retry-after-ms` trailer with the suggested retry delay (`internal/ratelimit/middleware/grpc.go::setRetryAfterTrailer`). The key name is hard-coded; some operator environments use different conventions (e.g. `grpc-retry-pushback-ms`). v1.x exposes the trailer key as a constructor option.
- **Why deferred**: There is no formal gRPC standard for retry-after; the v1.0 key is the convention this codebase ships. Renaming is mechanical when an operator needs it.
- **Acceptance**: Interceptor constructor accepts a trailer-key option; default is unchanged; tests cover override.
- **References**: `internal/ratelimit/middleware/grpc.go`.

#### Rate-limit: Auditor hook for rejections

- **Priority**: v1.x
- **What**: The file-distribution transport.Service has an optional `Auditor func(principal, op, path, reason error)` callback that fires on ACL denials (`internal/files/transport/service.go`). The rate-limit middleware has no analogous hook today; rejections are countable via metrics but not auditable. v1.x adds a symmetric `Auditor` seam so Epic 12's audit store can record rate-limit denials with the inbound key + the configured limit.
- **Why deferred**: The metric (`kscore_ratelimit_rejected_total`) already gives operators rejection visibility for v1.0 dashboards. Auditing rejections is a compliance-shop ask; bundle it with the post-v1.0 rate-limit expansion (per-namespace quotas, per-route rules).
- **Acceptance**: New `WithAuditor(fn)` option on the HTTP middleware + gRPC interceptor; auditor fires synchronously on each deny carrying key + reason + observed RPS.
- **References**: `internal/ratelimit/middleware/http.go`; `internal/files/transport/service.go::Auditor` (the symmetry target).

#### File distribution: PUT-side resume

- **Priority**: v1.x
- **What**: Resume a chunked PUT after a network interrupt — client retries with the same transfer-id, server picks up at the last received chunk. v1.0 ships GET-side resume only (`internal/files/transport.GetOptions.FromChunk`); a partial PUT today must restart from chunk 0.
- **Why deferred**: PUT resume needs durable server-side scratch state — the service must remember which chunks of which in-flight transfer it has received across crashes. v1.0's `internal/files/transport.Service` keeps in-flight state in memory only; the durable layer (JetStream-backed chunk inbox, or a SQLite scratch table) is meaningful design surface. GET resume is mechanically simple (re-read from the backend at the requested chunk offset) and satisfies Epic 18's "Resume after network interrupt works" acceptance line for the common direction.
- **Acceptance**: A PUT interrupted at chunk K resumed by the client with `FromChunk=K` completes without re-uploading chunks 0..K-1; server-side scratch state survives restart; per-transfer scratch is reaped on a configurable TTL.
- **References**: `internal/files/transport/service.go::handlePut`; `internal/files/transport/doc.go` (Resume §); Epic 18 task 11.

#### Backup destinations: Backblaze B2 smoke test + SFTP + GCS + Azure Blob + advanced S3 auth

- **Priority**: v1.x
- **What**: (a) Document Backblaze B2 as a first-class S3-compatible destination — works today via `s3://` + `Endpoint: s3.<region>.backblazeb2.com` (no code change needed); add an integration smoke test once an account is provisioned. (b) Three NEW destination backends behind the Epic-18 task 5 `Destination` seam: SFTP (golang.org/x/crypto/ssh + go-sftp), Google Cloud Storage, Azure Blob. (c) Advanced S3 auth flavors not wired in v1.0 (IRSA, instance profiles, assumed roles, web identity) — reachable through minio-go's IAM credentials provider but not exposed via `Config` until needed.
- **Why deferred**: v1.0 ships local filesystem + S3-compatible (AWS / MinIO / B2 / Wasabi / R2 / DigitalOcean Spaces via the same `s3://` scheme + endpoint). Backblaze B2 is the next-priority compatible service per operator direction. SFTP / GCS / Azure each need a new SDK + auth surface; bundling them now would balloon v1.0's dep graph. Epic 18 Scope § lists "SFTP, GCS, Azure for v1.5".
- **Acceptance**: B2 documented in `internal/backup/dest/dest.go` package comment + README; `kscore-backup create --dest sftp://host/path/foo.tar` succeeds; same for `gs://` and `azblob://`; `AWS_WEB_IDENTITY_TOKEN_FILE` produces a valid S3 client on EKS.
- **References**: Epic 18 Scope § (lines 26-28); `internal/backup/dest/`.

#### Backup encryption: AWS KMS + Vault key providers

- **Priority**: v1.x
- **What**: KeyProvider adapters that wrap AWS KMS Encrypt/Decrypt and Vault Transit operations so `kscore-backup` can encrypt with cloud-managed keys instead of operator-managed age private keys. The age envelope stays as the on-disk format; KMS/Vault wrap the age recipient material (envelope-of-envelope).
- **Why deferred**: v1.0 ships file-backed age keys only (`internal/backup/age.LoadIdentityFile` / `LoadRecipientsFile`). The KMS path needs the cloud-credential surface that arrives with the v1.x cloud-identity work; mixing the two now would force partial cloud-credential code into v1.0. Epic 18 Risks § names "v1.5 integrates KMS".
- **Acceptance**: `kscore-backup create --key-provider=aws-kms:arn:...` writes an artifact whose age recipients are wrapped under the KMS key; restore unwraps via the same KMS. Same for `vault:transit:keystone-backup`.
- **References**: Epic 18 Risks §; PROJECT-DETAILS §4.20 ("master key from env or KMS"); `internal/backup/age/`.

#### Policy enforcement side-effects (Warn events + Enforce violation handlers)

- **Priority**: v1.x
- **What**: Epic 12 task 10's `policy.Enforcer` ships the v1.0 audit-mode gate plus the post-v1.0 allow/deny *gate switch* behind `WithEnforcementEnabled` (Audit/Warn → allow, Enforce → block on a denying verdict). PROJECT-DETAILS §4.12's enforcement spec also requires the **side-effects**: `Warn` mode must emit a warn-level policy event; `Enforce` mode must invoke registered violation handlers before denying. v1.0 implements only the gate decision — the side-effects are deferred because they need infrastructure that does not exist yet (a policy-violation event type on the Epic 11 bus + a violation-handler registration/dispatch mechanism on the Enforcer).
- **Why deferred**: v1.0 ships the policy engine in audit-mode-only; enforcement is hardcoded off (`policy.enforcement_enabled=false`). The gate logic is testable today and proves the post-v1.0 shape; the side-effects have no v1.0 consumer and would be untested speculative infra. The enforcement flip is a separately-tracked behavior-changing release.
- **Acceptance**: `policy.enforcement_enabled=true` is operator-settable (config plumbing); `Warn` mode emits a `policy.warn` (or §4.9-canonical) event through the events bus on a denying verdict and still allows; `Enforce` mode invokes each registered violation handler (new `Enforcer.RegisterViolationHandler` seam) before returning `Allowed=false`; release notes call out the enforcement behavior change loudly.
- **References**: `internal/policy/enforcer.go` (`Enforce` gate switch — the `// Side-effects ... deferred` comment); PROJECT-DETAILS §4.12 "enforcement changes"; Epic 12 task 10 (landed).

#### kscore-secrets backends subcommand

- **Priority**: v1.x
- **What**: Epic 10 task 10 ships the `kscore-secrets` CLI with seven subcommand groups (get / put / delete / list / leases / transit / leases-leaf-cmds). PROJECT-DETAILS §4.11 also lists `backends` — list configured backends + capabilities — which v1.0 defers because the gRPC service has no `ListBackends` RPC yet. Operators today can read this from the server's YAML config or the `/api/status` payload, but a CLI surface would be friendlier.
- **Why deferred**: needs either a new gRPC method (`SecretsService.ListBackends() → BackendInfo[]`) or an extension of `/api/status`'s payload. v1.0 trial workflows can read the YAML directly.
- **Acceptance**: `kscore-secrets backends` prints one row per configured backend with name + type + capabilities; gRPC method (or REST endpoint) populates the data.
- **References**: PROJECT-DETAILS §4.11 CLI list; `api/proto/keystone/core/v1/secrets.proto` (no ListBackends); Epic 10 task 10 (landed).

#### kscore-secrets audit subcommand

- **Priority**: v1.x
- **What**: `kscore-secrets audit` queries the secrets-domain audit log — every `secret.access` event the broker fires on every op. v1.0 emits events via the slog-backed `LogAuditor` (Epic 10 task 3); Epic 12 ships the SQLite `AuditStore` + `AuditService` gRPC that this subcommand queries.
- **Why deferred**: depends on Epic 12's audit-query API which is a separate epic.
- **Acceptance**: `kscore-secrets audit --action=get_secret --since=1h` returns matching audit rows; flags include `--principal`, `--path`, `--allowed`, `--limit`, `--page-token`.
- **References**: PROJECT-DETAILS §4.11 + §4.12; Epic 12 (Audit & Policy); Epic 10 task 10 (landed).

#### kscore-secrets dynamic subcommand

- **Priority**: v1.x
- **What**: `kscore-secrets dynamic <path>` issues a dynamic credential through the broker. v1.0 lacks an `IssueDynamicSecret` RPC — operators can drive this through the in-process API but not over gRPC.
- **Why deferred**: requires adding a new proto RPC `IssueDynamicSecret(path, role, ttl, params) → Secret`. Additive but uses proto-breaking-pipeline time + needs careful thought on the `params` shape (map<string, string> vs google.protobuf.Struct for arbitrary types like PKI `alt_names`).
- **Acceptance**: proto adds `IssueDynamicSecret` RPC; CLI subcommand exists; ENG round-trips an actual Vault DB credential round-trip in the integration test.
- **References**: `api/proto/keystone/core/v1/secrets.proto`; `internal/secrets/backend.go` `IssueDynamicSecret`; Epic 10 task 10 (landed).

#### kscore-secrets cache subcommand

- **Priority**: v1.x
- **What**: `kscore-secrets cache stats` reports hit-rate / entry count / memory bytes; `kscore-secrets cache clear` drops every entry (operator-driven post-rotation). Epic 10 task 8's `SecretCache` exposes both internally; v1.0 doesn't expose them over the wire.
- **Why deferred**: needs new proto RPCs `GetCacheStats` + `ClearCache` (or a REST `/api/v1/secrets/cache` endpoint). Operator demand at trial scale is low; `kscore-server` log lines emit cache stats on a regular cadence.
- **Acceptance**: `kscore-secrets cache stats` prints hit/miss/eviction counters; `kscore-secrets cache clear` empties the cache + the next operator request observes a cache miss.
- **References**: `internal/secrets/secret_cache.go` `SecretCache.{Stats,Clear}`; Epic 10 task 8 (landed); Epic 10 task 10 (landed).

#### kscore-secrets template subcommand

- **Priority**: v1.x
- **What**: Consul-template-style config rendering. Operators write a config template with `{{ secret "kv/app/db" "password" }}` placeholders; the subcommand fetches every referenced secret and renders the output. Common pattern for app-config integration where the app doesn't natively talk to the secrets API.
- **Why deferred**: substantial feature — needs a template language pick (Go text/template + custom funcmap is the obvious starting point), file output / atomic-rewrite semantics, optional watch mode that re-renders on secret changes (depends on Epic 11 events), and security review of what placeholder syntax is permitted.
- **Acceptance**: `kscore-secrets template -i /etc/app/config.tmpl -o /etc/app/config` renders the template, atomically replaces the output, exits 0; missing-secret references fail-fast with a clear error; `--watch` re-renders on a SIGUSR1 (or Epic 11 event when ready).
- **References**: PROJECT-DETAILS §4.11 CLI list; analogous to `consul-template` / `vault agent`; Epic 10 task 10 (landed); Epic 11 (Events) for the watch path.

#### kscore-events retention subcommand

- **Priority**: v1.x
- **What**: PROJECT-DETAILS §4.9 lists `retention` in the kscore-events CLI surface — operator-driven manual application of retention policies + inspection of recent retention runs. The v1.0 retention enforcer (Epic 11 task 8) runs hourly on the cluster leader; operators have no CLI surface to trigger an ad-hoc run, inspect the current policy table, or audit recent retention deletions.
- **Why deferred**: needs new gRPC RPCs `ApplyRetention(policies) → (deleted_count)` + `GetRetentionPolicy() → []RetentionPolicy` + `GetRetentionHistory(limit) → []RetentionRun`. The retention enforcer itself (Epic 11 task 8) ships the scheduling; the CLI is the operator-facing manual hook on top.
- **Acceptance**: `kscore-events retention show` prints the active policy table; `kscore-events retention apply --type agent.heartbeat --max-age 24h --max-count 10000` invokes a one-shot retention pass against that policy; `kscore-events retention history --limit 10` shows recent runs with timestamp + rows-deleted.
- **References**: PROJECT-DETAILS §4.9 CLI list; `api/proto/keystone/core/v1/event.proto`; Epic 11 task 7 (kscore-events CLI; landed); Epic 11 task 8 (retention enforcer; pending).

#### kscore-events analyze subcommand

- **Priority**: v1.x
- **What**: PROJECT-DETAILS §4.9 lists `analyze` as an operator analysis tool over the event stream. The spec is fuzzy — likely something like top-N event types by frequency, error-rate over a window, source-distribution histograms, or anomaly detection. Defer until we know the concrete operator pain points it should solve.
- **Why deferred**: spec is intentionally fuzzy in §4.9 ("analyze" without acceptance criteria). v1.0's `stats` subcommand covers the simple total + per-type + per-severity case; everything else needs operator demand data to scope. `query` (deferred separately) covers ad-hoc filtered exploration.
- **Acceptance**: TBD — gather operator pain points from v0.x trial deployments first; likely lands as one or more sub-subcommands under `kscore-events analyze`.
- **References**: PROJECT-DETAILS §4.9 CLI list; Epic 11 task 7 (kscore-events CLI; landed).

#### kscore-events query subcommand (CEL post-filter)

- **Priority**: v1.x
- **What**: PROJECT-DETAILS §4.9 lists `query` alongside `list`. The intended distinction was a CEL-filtered post-fetch query — `kscore-events query --filter "tags.role == 'web' && severity.at_least('warn')"` runs ListEvents with the indexed filters, then applies the CEL filter client-side, then emits matching events. v1.0 task 7 ships `list` (structural filters) + `subscribe --replay <window>` (CEL on the streaming path); the standalone `query` subcommand is subsumed by `subscribe --replay <large-window>` for ad-hoc work.
- **Why deferred**: 80%+ of `query`'s use cases are covered by `subscribe --replay`. A dedicated subcommand only adds value when (1) operators want a non-streaming, bounded-output exit-when-done flow against CEL, or (2) CEL push-down to SQL is implemented so the filter narrows the query rather than running post-fetch. Until either lands, the subcommand is sugar.
- **Acceptance**: `kscore-events query --filter "..."` iterates ListEvents pages, applies CEL filter client-side, emits matching events as JSON lines, exits when the bound is exhausted. CEL syntax: `severity.at_least('warn')` form per Epic 11 task 5.
- **References**: PROJECT-DETAILS §4.9 CLI list; `internal/events/filter.go` (CompileFilter, landed); Epic 11 task 7 (kscore-events CLI; landed).

#### kscore-policy check / test subcommands

- **Priority**: v1.x
- **What**: PROJECT-DETAILS §4.12's CLI v1.0 list includes `check` and `test` alongside `list|validate|show|eval|compliance|violations`. Epic 12 task 14 shipped the latter six; `check` and `test` were deferred. `check` is plausibly a fold of `validate` + a dry-run against a sample input (overlaps `eval`); `test` is a policy unit-test harness (a testdata directory of input→expected-verdict cases, à la `opa test`). Both are policy-authoring conveniences with no acceptance criteria in the epic.
- **Why deferred**: §4.12 gives neither an acceptance line nor a concrete shape (mirrors the kscore-events `analyze` deferral). v1.0's `validate` (compile-check) + `eval` (run against `--input`) already cover the authoring loop; `check`/`test` only earn their keep once operators define the testdata/assertion format they want (likely `opa test`-compatible for the OPA case). Scoping needs trial-deployment demand.
- **Acceptance**: TBD — gather authoring-workflow pain from v0.x trials; `test` likely lands as `kscore-policy test <dir>` running a fixtures directory of `{input, expect_allowed, expect_violations}` cases with a non-zero exit on mismatch; `check` likely folds into `validate --input`.
- **References**: PROJECT-DETAILS §4.12 CLI v1.0 list; `internal/cli/policy` (validate/eval landed); Epic 12 task 14 (landed).

#### kscore-audit search / analyze / timeline / watch subcommands

- **Priority**: v1.x
- **What**: PROJECT-DETAILS §4.12's `kscore-audit` v1.0 list is `log|report|export|stats|search|analyze|timeline|watch`. Epic 12 task 14 shipped `log|report|stats`; `export` is owned by task 15 (formatters + redaction). The remaining four are deferred: **search** (richer ad-hoc filtered query — subsumed by `log`'s filter flags for v1.0); **analyze** (fuzzy operator-analysis tool, no acceptance criteria — same shape as the deferred kscore-events `analyze`); **timeline** (all evaluations for a single resource over time — maps to `policy.ReportGenerator.ResourceAuditTrail`, which has **no gRPC RPC** on PolicyService); **watch** (live audit tail — needs an audit-tail **streaming RPC** + the `audit.BufferedAuditor` exposed over the wire; neither exists in v1.0).
- **Why deferred**: `search` is sugar over `log` until a distinct query model is demanded; `analyze` is fuzzy-spec (gather demand first); `timeline` and `watch` require new server surface (a `ResourceAuditTrail` unary RPC and an audit-tail server-stream respectively) that is out of task 14's scope and not on the v1.0 critical path.
- **Acceptance**: `timeline` — add `PolicyService.GetResourceAuditTrail` (or an audit-service RPC) wrapping `ReportGenerator.ResourceAuditTrail`, then `kscore-audit timeline <resource-type> --since …` renders it oldest-first. `watch` — add an audit-tail server-streaming RPC fed by `audit.BufferedAuditor`, then `kscore-audit watch` tails it like `kscore-events watch`. `search`/`analyze` — TBD from trial demand.
- **References**: PROJECT-DETAILS §4.12 `kscore-audit` CLI list; `internal/policy/compliance.go` `ResourceAuditTrail` (landed, no RPC); `internal/audit` `BufferedAuditor` (landed, not wire-exposed); Epic 12 task 14 (landed).

#### kscore-audit export `--redaction-config` file

- **Priority**: v1.x
- **What**: Epic 12 task 15 ships `kscore-audit export` with redaction driven by repeatable flags (`--redact-key`, `--redact-pattern`, `--redact-user`, `--redact-replacement`). Add a `--redaction-config <file>` that unmarshals a YAML/JSON `audit.RedactionConfigInput` so a vetted redaction policy is reusable + reviewable in source control rather than retyped as flags per invocation.
- **Why deferred**: the flag form satisfies the §4.12 acceptance bar (`--redact-pattern 'password=\S+'`) and the export path already takes a `*RedactionConfig` (the file would just be a second constructor of the same struct). §4.12's risk note ("redaction regex must be reviewed before prod") makes a checked-in config the prod-grade ergonomic, but it's additive over a complete v1.0 redaction path — no behavior gap, pure convenience.
- **Acceptance**: `kscore-audit export --redaction-config redaction.yaml` loads `{redact_metadata_keys, redact_patterns, redact_user, replacement}` into `audit.NewRedactionConfig`; flags, when also given, layer over / override the file; a malformed file or bad regex fails loudly before the first entry is written.
- **References**: `internal/cli/audit/export.go` (flag-driven `RedactionConfigInput`); `internal/audit/redaction.go` `NewRedactionConfig` (landed); Epic 12 task 15 (landed).

#### kscore-cluster-backup schedule + kscore-cluster watch subcommands

- **Priority**: v1.x
- **What**: Epic 13 task 16 shipped `kscore-cluster` (`status|members|leader|add|remove|transfer-leader|rebalance|backup|restore`) and `kscore-cluster-backup` (`backup|restore|list|verify`). Two subcommands from the §4.15 / epic surface are deferred: **`kscore-cluster-backup schedule`** (automated periodic snapshots) — the epic explicitly tags backup scheduling/automation as a future release (epic lines 47/60); and a **`kscore-cluster watch`** (live membership/leadership tail over the existing `WatchMembership`/`WatchLeadership` server-streams) — not in the FEATURES/acceptance CLI list, the same stream-CLI gap deferred for kscore-events/kscore-audit.
- **Why deferred**: `schedule` is out of v1.0 epic scope by the epic's own wording (no acceptance line; needs a scheduler/retention design — overlaps the general backup-automation deferral); `watch` is pure CLI sugar over RPCs that already exist server-side (the stream handlers landed in task 15) — it earns its keep once operators ask for a live cluster tail, mirroring the kscore-audit `watch` rationale. The acceptance-critical surface (`status`, `backup --output`, `restore --input --force`) all shipped.
- **Acceptance**: `watch` — `kscore-cluster watch [--leadership]` consumes `ClusterServiceClient.WatchMembership`/`WatchLeadership` and renders events until interrupted, like `kscore-events watch`. `schedule` — TBD from trial demand; likely `kscore-cluster-backup schedule add --cron … --output-dir …` registering a periodic snapshot job with retention.
- **References**: `internal/cli/cluster` (landed: the 13 shipped subcommands); `pkg/api/v1` `ClusterServiceClient.WatchMembership`/`WatchLeadership` (landed in Epic 13 task 15, not yet CLI-exposed); Epic 13 task 16 (landed); companion: the general backup automation/scheduling deferral.

#### Strict audit-on-access via Auditor.Emit error return

- **Priority**: v1.x
- **What**: §4.11's "failure to log = bug" invariant currently can't fail the in-flight secrets op when the events-bridge audit publish fails — `secrets.Auditor.Emit` has no error return (deliberately, per the v1.0 contract "fire and forget; never error back to the caller"). The Epic 11 task 10 bridge (`cmd/kscore-server/audit_bridge.go`) logs publish failures at WARN and bumps `events.AuditEmitter.FailedPublishes`, but the broker continues regardless. Stronger consistency requires the Auditor interface to surface errors so the broker can fail the op (or fail-open with a configurable policy).
- **Why deferred**: changing `secrets.Auditor.Emit` to return an error is a broad refactor — every existing implementation (LogAuditor, BufferedAuditor, SamplingAuditor, MultiAuditor, NoopAuditor, every test fake) updates; the broker grows policy code for "what to do when emit fails" (fail-open vs fail-closed, configurable). Operator demand at v0.x trial scale is "log + alert when it fails," which the current bridge already provides via the FailedPublishes counter + ERROR slog line.
- **Acceptance**: `secrets.Auditor.Emit` returns `error`; the broker has an `AuditFailurePolicy` config (`fail_open` / `fail_closed`); the bridge propagates publish errors; integration test confirms a denied publish under `fail_closed` causes the secrets op to error.
- **References**: PROJECT-DETAILS §4.11 "failure to log = bug"; `internal/secrets/audit.go` Auditor interface; `cmd/kscore-server/audit_bridge.go`; Epic 11 task 10 (landed).

#### Secrets transit batch API on the wire

- **Priority**: v1.x
- **What**: Epic 10 task 7's `TransitBackend` interface supports batch ops (each method takes `Items []…Input`), but the v1.0 `api/proto/keystone/core/v1/secrets.proto` `EncryptRequest` / `DecryptRequest` / `SignRequest` / `VerifyRequest` are singleton-only (one plaintext per call). Operators with bulk-transit workloads can drive the in-process `TransitBackend` directly but can't reach batch over gRPC/REST.
- **Why deferred**: a proto change is the right tool — adding a `BatchEncrypt` RPC with a `repeated EncryptItem` shape is additive but needs the buf-breaking pipeline run and the gRPC service stubs regenerated. Operator demand for batch over the wire isn't pressing at trial scale; the in-process API serves the only known caller.
- **Acceptance**: `api/proto/keystone/core/v1/secrets.proto` adds `BatchEncrypt` / `BatchDecrypt` / `BatchSign` / `BatchVerify` RPCs with `repeated …Item` requests and per-item-error responses; the gRPC service implementation routes them through `secrets.TransitBackend.{Encrypt,Decrypt,Sign,Verify}` directly (no item-by-item loop on the server side); REST handlers gain `POST /api/v1/transit/{op}/batch` endpoints.
- **References**: `internal/secrets/transit.go` `TransitBackend`; `api/proto/keystone/core/v1/secrets.proto`; Epic 10 task 9 (landed).

#### Secrets transit GenerateDataKey on the wire

- **Priority**: v1.x
- **What**: Epic 10 task 7 added `TransitBackend.GenerateDataKey` (envelope encryption — returns a plaintext key + the Vault-wrapped form). v1.0's proto + REST surface doesn't expose it; operators can only reach it via the in-process API.
- **Why deferred**: envelope encryption is a specialist pattern; the v1.0 audience (`Encrypt` / `Decrypt` / `Sign` / `Verify` for app-layer integration) doesn't depend on it. Adding a proto RPC + REST endpoint is straightforward but uses proto-breaking pipeline time.
- **Acceptance**: `api/proto/keystone/core/v1/secrets.proto` adds `GenerateDataKey` RPC with a `Mode` enum field (`PLAINTEXT` / `WRAPPED`) + `bits` + `context`; the gRPC service forwards to `TransitBackend.GenerateDataKey`; `POST /api/v1/transit/datakey/{plaintext|wrapped}/{key}` REST endpoint mirrors it.
- **References**: `internal/secrets/transit.go` `TransitBackend.GenerateDataKey`; `api/proto/keystone/core/v1/secrets.proto`; Epic 10 task 9 (landed).

#### Encrypted-file per-secret TTL expiry

- **Priority**: v1.x
- **What**: Epic 10 task 9's gRPC + REST `WriteSecret` accepts a `ttl_seconds` field (per the proto declared in Epic 03). The current behavior stores the value as `Metadata["ttl_seconds"]` so backends that honor it natively (Vault KV v2 via lease TTLs on dynamic mounts) can read it, but the encrypted-file backend (task 4) doesn't auto-expire entries past their TTL — the metadata is observational only.
- **Why deferred**: per-secret expiry on the file backend needs an additional disk sweep alongside the existing CAS / atomic-rewrite logic; not a v0.5 / v1.0 gating need. Operators that want per-secret TTL today use the cache TTL (Epic 10 task 8) or the Vault backend.
- **Acceptance**: encrypted-file backend reaps entries whose `Metadata["ttl_seconds"]` has elapsed since `UpdatedAt`; `GetSecret` on an expired path returns `ErrSecretNotFound` even without an explicit `DeleteSecret`; a configurable background sweep runs at a default 1m cadence; rewrite-on-expiry is atomic.
- **References**: `internal/secrets/file/backend.go`; `api/proto/keystone/core/v1/secrets.proto` `WriteSecretRequest.ttl_seconds`; Epic 10 task 9 (landed).

#### kscore-identity federation subcommands

- **Priority**: v1.x
- **What**: Epic 09 task 12's `kscore-identity` CLI ships seven subcommands (`token {create,list,revoke,cleanup}` + `ca {info,rotate-signing,export}` + `status`). PROJECT-DETAILS §4.10 names a fourth top-level group — `federation {add-domain, list, fetch-bundle}` — for cross-trust-domain operation. v0.1 explicitly defers it (the spec marks it post-v1.0). The CLI + gRPC service + EmbeddedProvider would gain `FederatedTrustDomain` records, a bundle-endpoint exposure, and per-domain refresh hints; lands alongside the Provider trust-federation work itself.
- **Why deferred**: federation needs a wire protocol for bundle distribution + cross-domain trust policy + a federated `Attest` path — its own design pass. v0.1 ships single-domain only.
- **Acceptance**: `kscore-identity federation add-domain spiffe://peer.example.org/` registers + fetches the peer bundle; `kscore-identity federation list` shows registered domains + last-refresh timestamps; `kscore-identity federation fetch-bundle <domain>` retrieves it ad-hoc; the existing trust-federation v1.x ROADMAP entry covers the underlying provider work.
- **References**: Epic 09 scope-out (line 23); `internal/cli/identity` (current CLI surface); the existing "Trust federation (cross-domain bundle endpoint)" entry below.

#### Unbiased base62 for join-token generation

- **Priority**: v1.x
- **What**: Epic 09 task 10's `randomBase62` (in `internal/identity/join_token_generate.go`) uses `byte % 62` for the cleartext-token body. `256 mod 62 = 8`, so values 0-7 are slightly more likely than the rest (≈4.3% vs ≈3.5%). At `joinTokenBodyLen = 40` chars the residual entropy is ≈ 235 bits — well above v0.1's threat model for one-time tokens — but a strict security audit would prefer rejection sampling for deterministic uniformity. Drop-in replacement; no wire-format change. Phase B5 (2026-05-23) audit additionally noted that the bias affects the **prefix** distribution (`token[:JoinTokenPrefixLen]` used as the store lookup key), which concentrates lookup keys toward characters early in the base62 alphabet — increases distinguishability for someone enumerating prefix space, though still below the operational risk threshold at v0.1.
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

#### Module signing: cosign keyless / Rekor transparency / encrypted cosign keyfile interop

- **Priority**: v1.x
- **What**: Epic 14 task 4 ships `pkg/module/verify` as a pure-stdlib, cosign-*compatible* **keyed** detached-blob verifier (RSA/ECDSA/Ed25519, KeyID-indexed trust policy) — explicitly Option C, no `sigstore/cosign` dependency. Deferred: (a) cosign **keyless** signing/verification (Fulcio-issued short-lived certs + OIDC identity); (b) **Rekor** transparency-log inclusion proofs; (c) reading the **encrypted cosign keyfile** format that `cosign generate-key-pair` emits (scrypt + nacl/secretbox), so `kscore-module sign` could consume an existing `cosign.key`. v1.0 `kscore-module sign` uses a plain PKCS8/SEC1 PEM ("local.key").
- **Why deferred**: the epic's v0.1 decision is "Cosign-only verification, no SumDB transparency log" and OCI is v1.1; keyless/Rekor pull the full `sigstore/cosign` + Fulcio/Rekor/TUF dependency tree (Kubernetes-scale) for capability that is entirely post-v1.0 scope. Keyed verification + a TLS-trusted registry is the v1.0 supply-chain baseline; the stdlib verifier is forward-compatible (the trust policy / `Signature` types extend to a keyless issuer without an API break).
- **Acceptance**: a module signed by the real `cosign` CLI in keyless mode verifies through an extended trust policy with a Rekor inclusion check; `kscore-module sign` can load a cosign-encrypted `cosign.key`.
- **References**: `pkg/module/verify` (task 4, landed — keyed/stdlib); epic 14 "Scope (out)" (SumDB v1.2); companion of "WASM module runtime".

#### Module registry publish authentication

- **Priority**: v1.x
- **What**: Epic 14 task 9 ships `cmd/kscore-registry` with an unauthenticated `POST /publish` (multipart manifest + module ZIP). Deferred: authenticating publishers (API key / mTLS client cert / signed-publish token) and per-namespace publish authorization so only an owner can publish under `vendor/*`.
- **Why deferred**: §4.18's v1.0 trust model is the **TLS-trusted registry transport + Cosign verification at load time** (the loader/task-4's job), and the v1.0 filesystem registry runs on a trusted boundary (local / trusted network); the epic gives no publish-auth acceptance criterion. Read integrity is already covered (content-addressed hashes + signature verification at install). Publisher identity is an additive concern that does not change the artifact format.
- **Acceptance**: `POST /publish` requires a valid credential (API key or mTLS); an unauthenticated publish is rejected 401/403; a publish under a namespace the credential does not own is rejected; existing read endpoints are unaffected.
- **References**: `pkg/module/registry/handler.go` (`publish`), `cmd/kscore-registry` (task 9, landed — unauthenticated); companion of "Module signing: cosign keyless / Rekor transparency / encrypted cosign keyfile interop".

#### Starlark hard heap-bytes cap

- **Priority**: v1.x
- **What**: Epic 14 task 11's Starlark runtime bounds a module by an execution-step cap (`thread.SetMaxExecutionSteps`), a wall-clock timeout (`thread.Cancel`), and Starlark's intrinsic recursion / no-`while` strictness. §4.18 also lists "memory heap limits"; go.starlark.net has **no public allocation/heap-bytes ceiling API**, so v1.0 approximates it via the step+time bounds. Add a true per-execution heap-bytes cap once upstream exposes one (the long-proposed `thread.SetMaxAllocs`/allocation accounting) or via an out-of-process / cgroup memory bound for module execution.
- **Why deferred**: the step + wall-clock bounds already prevent unbounded CPU and runaway loops (the practical DoS vectors); a precise heap ceiling needs runtime support that go.starlark.net does not currently provide, and re-implementing allocation accounting is out of v1.0 scope. The manifest already carries `limits.memory` (parsed, task 1) so the contract is forward-compatible — only the enforcement is deferred.
- **Acceptance**: a module exceeding `limits.memory` is terminated with a typed error before exhausting host memory; verified by a high-allocation test module.
- **References**: `pkg/module/runtime/starlark` (task 11, landed — step+time bounds); `pkg/module/manifest` `Limits.Memory` (task 1, parsed); go.starlark.net `Thread`; companion of "WASM module runtime".

#### Per-capability-call context propagation in the Starlark SDK

- **Priority**: v1.x
- **What**: Epic 14 task 12's `modules/sdk/starlark` capability builtins call their backends with `context.Background()` — Starlark builtins receive a `*starlark.Thread`, not a `context.Context`, so the per-call ctx (deadline/cancellation) is not threaded into individual capability invocations (e.g. an `http.get` does not inherit a caller deadline beyond the module-wide bound). Add propagation of the module-execution context (and any per-call deadline) into each capability call.
- **Why deferred**: the task-11 thread watchdog (`thread.Cancel` on timeout/ctx-cancel) already aborts the *entire* Starlark execution mid-call, so a runaway or hung capability call is still bounded at the module level for v1.0; fine-grained per-call ctx only changes *which* call observes the cancellation first, not whether execution is bounded. Threading ctx requires either a thread-local convention (`thread.SetLocal`) set by the runtime before `Call` or a signature change, and is a clean follow-up rather than a v1.0 correctness gap.
- **Acceptance**: a capability call (e.g. `http.get`) observes the module execution context's deadline/cancellation directly (verified by cancelling mid-call and asserting the backend received a cancelled ctx), with the module-level watchdog unchanged.
- **References**: `modules/sdk/starlark/sdk.go` (`guard` uses `context.Background()`); `pkg/module/runtime/starlark` (task-11 thread watchdog); Epic 14 task 12; companion of "Starlark hard heap-bytes cap".

#### Module test framework: injectable record/replay http/exec/secrets test hosts

- **Priority**: v1.x
- **What**: Epic 14 task-15 `pkg/module/testing` wires os-backed `fs` + a discard `Logger` for test runs but leaves `http`/`exec`/`secrets` hosts nil (fail-closed + audited if a module both requested and uses them). Add injectable, deterministic record/replay hosts so a module's unit tests can exercise its `http`/`exec`/`secrets` capability paths without real network/process/secret-store side effects.
- **Why deferred**: network/process/secret side effects in unit tests are an anti-pattern; for v1.0 a module's pure logic + `kv`/`log`/`fs`-scope behaviour is unit-testable, and the nil-host fail-closed path is itself testable and demonstrates the security contract. Record/replay fixtures are an ergonomics follow-up, not a v1.0 correctness gap (`Options.Hosts` already lets a caller inject fakes today — this is about a built-in fixture format).
- **Acceptance**: `kscore-module test` can run a module whose tests call `http.get`/`exec`/`secrets.read` against a declared fixture (recorded responses), with the capability scoping + audit trail unchanged.
- **References**: `pkg/module/testing/hosts.go` (`defaultHosts`), `pkg/module/testing/runner.go` (`Options.Hosts`); Epic 14 task 15.

#### Module test framework: multi-file module tests via Starlark `load()`

- **Priority**: v1.x
- **What**: the task-15 runner uses the single-entrypoint module model — a `*_test.star` reaches the module's functions via a merged predeclared environment, with no Starlark `load()` support. Add a module-aware `load()` so a multi-file module (helpers split across `.star` files) can be tested with the same import graph the runtime would use.
- **Why deferred**: v1.0 modules are single-entrypoint (the manifest declares one `entrypoint`); the merged-predeclared approach gives tests full access to module-level definitions for that model. `load()` requires a thread `Load` resolver + cycle handling that only matters once multi-file modules are supported (itself post-v1.0).
- **Acceptance**: a module split across multiple `.star` files, loaded via `load()`, is testable end-to-end through `kscore-module test` with deterministic, cycle-safe resolution.
- **References**: `pkg/module/testing/runner.go` (`mergeDicts`, entrypoint exec); Epic 14 task 15; companion of "Module test framework: injectable record/replay http/exec/secrets test hosts".

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
- **What**: Checkpoint-resume (re-load a crashed saga and continue from the last completed step with the pre-step Data restored, persisting per-step transition rows); cross-state compensation graphs (compensation by dependency graph instead of strict reverse-linear); richer multi-failure aggregation reporting; gRPC `StateService.RollbackStateSaga` (saga-driven rollback, distinct from today's whole-run rollback CLI). The durable `pkg/saga/log_sqlite` `Log` itself is NO LONGER deferred — it landed in Epic 15 task 2 (v1.0 scope per PROJECT-DETAILS §4.17 "in-memory or SQLite log" / FEATURES "no checkpoint resume"); only the per-step checkpoint-resume protocol on top of it remains v1.x.
- **Why deferred**: v0.1 ships the minimal scaffold per Epic 08 task 12 — forward-execute steps, on first error walk completed steps in reverse with aggregate-and-continue compensation semantics, in-memory and (Epic 15 task 2) durable SQLite log, `Runner.RunSaga(History)` integration that re-applies the most recent prior `StateRunRecord` per decl. Resume-from-checkpoint is a distinct durability surface beyond a persistent log (atomic per-step transitions, recovery-on-startup hook, integration with `kscore-server` lifecycle); cross-state compensation graphs reshape the algorithm; the v0.1 minimum is enough for the trial use case where a saga either finishes or is restartable from the top.
- **Acceptance**: A 10-step saga survives a crash mid-step 5, resumes from checkpoint, completes the remainder; a cross-state compensation graph compensates step 3 before step 2 when the graph dictates; `kscorectl state rollback-saga <run-id>` reconstructs the saga from history and walks the compensation graph.
- **References**: PROJECT-DETAILS §4.17 (saga coordinator block, v1.x line 1197); Epic 08 task 12 *(landed)*; Epic 15 task 2 *(durable SQLite log landed)*; `pkg/saga/doc.go` package overview.

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

#### Adaptive sampling tied to error metrics

- **Priority**: v2.x+
- **What**: OTel `Sampler` implementation that rebalances sample rate on the observed error rate from `kscore_*_failed_total` metrics. PROJECT-DETAILS §4.16 line 1126 names `adaptive (rebalances on observed error rate)` as part of the v1.0 sampler enum, but Epic 17 Scope-out (line 82) defers it to v2.0 with the rationale "refinement". Epic 17 task 4 ships the other four samplers (`always_on/off`, `probabilistic`, `parent_based`, `rate_limiting`); `internal/config/TracingConfig.Validate` rejects `sampler=adaptive` with a pointer to this entry.
- **Why deferred**: needs a stable kscore metric set + an error-rate-feedback path that doesn't exist in v1.0. The other four samplers cover every v1.0 use case (probabilistic at 0.1 is the documented default).
- **Acceptance**: `tracing.sampler: adaptive` validates and constructs a sampler that increases sample rate on error spikes (>1% per-route 5xx) and decays back to baseline; integration test with a stub metric source.
- **References**: Epic 17 Scope-out (line 82); PROJECT-DETAILS §4.16 line 1126; `internal/config/tracing.go` `Validate` adaptive branch.

#### Zipkin exporter replacement — Zipkin module upstream-deprecated

- **Priority**: v2.x+ (action well before early 2027)
- **What**: `go.opentelemetry.io/otel/exporters/zipkin` is upstream-deprecated with planned removal in early 2027. Epic 17 task 4 ships Zipkin support because the epic explicitly lists it. Before the upstream removal lands, migrate operators currently using Zipkin to OTLP (HTTP or gRPC) — OTLP is the OTel-blessed wire format and most observability backends now accept it natively.
- **Why deferred**: v1.0 still names Zipkin; switching defaults during a stability commitment is disruptive. The upstream deprecation gives a 2+ year runway.
- **Acceptance**: Zipkin exporter removed from `internal/tracing/exporters.go`; migration note in release notes; `tracing.exporter=zipkin` validates with a deprecation warning pointing at OTLP.
- **References**: `internal/tracing/exporters.go`; `go get` output noting `module go.opentelemetry.io/otel/exporters/zipkin is deprecated: The zipkin exporter is deprecated and will be removed in early 2027`.

#### Embedded NATS / hybrid mode / leaf node / endpoint advertiser / supercluster / WebSocket

- **Priority**: v2.x+
- **What**: Agents run embedded nats-servers; cluster forms via leaf-node mesh; reverse-leaf NAT traversal for agents behind firewalls; WebSocket transport for browser-side ops.
- **Why deferred**: v1.0 ships agent-as-NATS-client only; hybrid topology + WebSocket multiply test surface.
- **Acceptance**: Agent runs embedded NATS; reverse-leaf publishes its endpoint; supercluster gateway federates two clusters.
- **References**: Epic 05 scope-out (lines 38-42); Epic 06 scope-out (lines 27-28).
