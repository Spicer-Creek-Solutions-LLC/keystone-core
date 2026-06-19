# Keystone Core — State Module Support Matrix (v0.5)

This is the "what works / what doesn't" guarantee for the v0.5 external-tester
milestone (see [`VERSIONING.md`](VERSIONING.md) § v0.5 gate). It lists every
state-file parameter Keystone Core's stdlib modules accept and the maturity
status of each, so operators evaluating v0.5 know in advance what is safe to
depend on.

Scope: this matrix covers the **state-management stdlib modules** under
`internal/statemgmt/stdlib`. It is generated from the modules' validated
parameter surface, not from marketing copy — if a parameter is listed here, the
engine accepts and acts on it today.

## How to read this

### Status taxonomy

- **stable** — Implemented, validated, and either covered by the live
  cross-distro CI matrix or distro-agnostic and exercised by unit/integration
  tests. Safe to depend on; breaking changes will carry a migration note.
- **experimental** — Implemented and usable, but with narrower coverage: not yet
  exercised on the live distro matrix, recently added, or limited in scope. May
  change between v0.x releases without a long deprecation window.
- **deprecated** — Scheduled for removal; a replacement exists. **No parameters
  are deprecated as of v0.5.**

### Cross-distro matrix

The "matrix" column refers to the live 8-distro privileged-container harness
(`make test-cross-distro` → `test/e2e/state/run.sh`): Debian 12,
Ubuntu 22.04/24.04, Rocky 9, Alpine 3.19, exercising each applicable module
twice for idempotency against the real init system and package manager.

- **yes** — Verified live on the distro matrix.
- **agnostic** — Behaviour does not vary by distribution (filesystem, user,
  exec, crypto primitives); covered by unit/integration tests rather than the
  distro matrix.

### Parameters every module shares

These are handled by the engine and are not repeated in each table below:

- **The declaration name** is the primary identity. For several modules it *is*
  the managed value — e.g. `hostname`, `timezone` (`America/New_York`), `kmod`
  (the module name), `sysctl` (the kernel key), `file`/`link`/`pki` (the path),
  `package`/`user`/`group` (the object name).
- **`state`** — Desired state. The legal values are module-specific and are
  noted per module (e.g. `present`/`absent`, `running`/`stopped`, `on`/`off`,
  `run`). Where a module supports only one state, that is called out.
- **`severity`** — Optional drift-severity override (`low`/`medium`/`high`).
  Reserved engine parameter; **stable** on every module.

---

## Module maturity at a glance

| Module | Category | Maturity | Matrix | Notes |
|--------|----------|----------|--------|-------|
| `file` | Files & content | stable | agnostic | content/source, mode, owner, symlink |
| `link` | Files & content | stable | agnostic | symlink + hardlink |
| `archive` | Files & content | stable | agnostic | extract-only (`present`) |
| `config` | Files & content | stable | agnostic | keyvalue + INI formats |
| `git` | Files & content | stable | agnostic | clone / track rev |
| `package` | Packages | stable | yes | apt/dnf/apk verified; zypper/pacman experimental |
| `langpkg` | Packages | experimental | agnostic | pip/npm/gem, global scope only |
| `service` | Services & scheduling | stable | yes | systemd/openrc verified; sysvinit experimental |
| `cron` | Services & scheduling | stable | yes | per-user crontab |
| `at` | Services & scheduling | stable | yes | one-shot at jobs |
| `systemd_timer` | Services & scheduling | stable | yes | timer units |
| `user` | Users & access | stable | agnostic | account management |
| `group` | Users & access | stable | agnostic | group management |
| `ssh` | Users & access | stable | agnostic | authorized_keys entries |
| `sysctl` | Kernel & system | stable | yes | runtime + `/etc/sysctl.d` persist |
| `kmod` | Kernel & system | stable | agnostic | load + boot persistence |
| `hostname` | Kernel & system | stable | yes | static hostname |
| `timezone` | Kernel & system | stable | yes | system timezone |
| `system` | Kernel & system | stable | agnostic | banner / reboot / locale |
| `swap` | Kernel & system | stable | yes | swapfile / partition + fstab |
| `disk` | Storage | stable | yes | mkfs + resize |
| `mount` | Storage | stable | yes | fstab + live mount |
| `lvm` | Storage | stable | yes | PV / VG / LV |
| `network` | Networking | stable | yes | runtime + persist (networkd/netplan) |
| `route` | Networking | stable | yes | runtime + persist (networkd/netplan) |
| `bond` | Networking | stable | yes | create/remove; persist optional |
| `bridge` | Networking | stable | yes | create/remove; persist optional |
| `vlan` | Networking | stable | yes | 802.1Q create/remove |
| `iptables` | Firewall | stable | yes | single rule per decl |
| `nftables` | Firewall | stable | yes | single rule per decl |
| `firewalld` | Firewall | stable | yes | zone service/port/rich-rule |
| `firewall` | Firewall | stable | yes | backend-dispatching abstraction |
| `security` | Security | experimental | partial | SELinux + AppArmor, `present` only |
| `pki` | PKI | stable | agnostic | x509 cert/key on disk |
| `command` | Execution | stable | agnostic | guarded shell exec |

> `persist` renderers are limited to **systemd-networkd** and **netplan** in
> v0.5. NetworkManager / RHEL `ifcfg-*` are planned for v0.6+ (see the network
> module section).

---

## Files & content

### `file`

Manages files, directories, and symlinks. States: `present`, `absent`,
`directory`, `symlink`.

| Parameter | Type | Required | Status | Notes |
|-----------|------|----------|--------|-------|
| `content` | string | optional | stable | File body; mutually exclusive with `source` |
| `source` | string | optional | stable | Local source path; mutually exclusive with `content` |
| `mode` | string | optional | stable | Octal permissions, e.g. `"0644"` |
| `owner` | string | optional | stable | User name or numeric UID |
| `group` | string | optional | stable | Group name or numeric GID |
| `target` | string | conditional | stable | Symlink target; required for `state: symlink` |

### `link`

Manages symbolic and hard links. States: `present`, `absent`.

| Parameter | Type | Required | Status | Notes |
|-----------|------|----------|--------|-------|
| `target` | string | conditional | stable | Link target; required for `present` |
| `kind` | string | optional | stable | `symlink` (default) or `hard` |
| `force` | bool | optional | stable | Replace an existing non-matching file (never a directory) |

Not yet supported (planned, #104): relative-target normalisation — relative
symlink targets are stored verbatim.

### `archive`

Extracts tar/tar.gz/tar.bz2/zip archives. State: `present` only.

| Parameter | Type | Required | Status | Notes |
|-----------|------|----------|--------|-------|
| `target` | string | required | stable | Destination directory |
| `format` | string | optional | stable | `auto` (default), `tar`, `tar.gz`, `tar.bz2`, `zip` |
| `creates` | string | optional | stable | Idempotency short-circuit path |
| `strip_components` | int | optional | stable | Remove leading path levels |

Not yet supported (planned, #108): `absent`/clean-before-extract mode and safe
symlink/hardlink extraction.

### `config`

Manages individual key/value pairs in `keyvalue` or INI files. States:
`present`, `absent`.

| Parameter | Type | Required | Status | Notes |
|-----------|------|----------|--------|-------|
| `key` | string | required | stable | Config key (no `=`, newlines, or comment/section leaders) |
| `value` | string | conditional | stable | Required for `present` |
| `format` | string | optional | stable | `keyvalue` (default) or `ini` |
| `section` | string | optional | stable | INI section header; INI format only |
| `space_around_separator` | bool | optional | stable | `key = value` vs `key=value` |
| `create` | bool | optional | stable | Create file if missing (default true) |

Not yet supported (planned, #107): TOML/YAML/JSON/XML formats, configurable
separators, case-insensitive keys, uncomment-aware updates.

### `git`

Manages a git working tree. States: `present` (clone if absent), `latest`
(track remote), `absent`.

| Parameter | Type | Required | Status | Notes |
|-----------|------|----------|--------|-------|
| `url` | string | conditional | stable | Required for `present`/`latest` |
| `rev` | string | optional | stable | Branch/tag/SHA (default: remote default branch) |
| `depth` | int | optional | stable | Shallow-clone depth; `0` = full |
| `remote` | string | optional | stable | Remote name (default `origin`) |
| `force` | bool | optional | stable | Allow `latest` to discard local changes |

Not yet supported (planned, #103): authenticated clones, submodules, advanced
clone options.

---

## Packages

### `package`

Linux package management. States: `present`, `absent`. Backend is auto-detected.

| Parameter | Type | Required | Status | Notes |
|-----------|------|----------|--------|-------|
| `version` | string | optional | stable | Version pin; invalid with `absent` |

Backends:

- **apt, dnf, apk** — **stable**; verified on the live distro matrix
  (Debian/Ubuntu, Rocky, Alpine).
- **zypper, pacman** — **experimental**; implemented and unit-tested, but no
  SUSE/Arch distro is in the live matrix yet. (Gate: zypper is optional for
  v0.5, required by v1.0 — see #18.)

### `langpkg`

Language-ecosystem packages via pip/npm/gem. States: `present`, `absent`.
**Module maturity: experimental.**

| Parameter | Type | Required | Status | Notes |
|-----------|------|----------|--------|-------|
| `name` | string | required | experimental | Package name |
| `manager` | string | required | experimental | `pip`, `npm`, or `gem` |
| `version` | string | optional | experimental | Strict-equality pin; invalid with `absent` |

Not yet supported (planned, #116): version ranges, per-user / per-project
installs, lockfiles, PEP-668, and additional ecosystems (cargo, composer, …).

---

## Services & scheduling

### `service`

Manages service running state and boot-enablement. States: `running`,
`stopped`.

| Parameter | Type | Required | Status | Notes |
|-----------|------|----------|--------|-------|
| `enable` | bool | optional | stable | Boot-enablement; unset = leave as-is |

Backends:

- **systemd, openrc** — **stable**; verified on the live distro matrix
  (systemd everywhere, openrc on Alpine).
- **sysvinit** — **experimental**; implemented and fixture-tested, but no
  sysvinit-default distro is in the live matrix.
- **launchd** (macOS) — not supported; out of scope for the Linux-only v0.5
  (#17).

### `cron`

Per-user crontab entries with marker comments. States: `present`, `absent`.

| Parameter | Type | Required | Status | Notes |
|-----------|------|----------|--------|-------|
| `command` | string | conditional | stable | Required for `present` |
| `schedule` | string | conditional | stable | 5-field spec or `@`-shortcut; required for `present` |
| `user` | string | optional | stable | Crontab owner (default `root`) |

Not yet supported (planned, #105): per-field schedule params, `/etc/cron.d`
drop-ins, environment lines, deep field-syntax validation.

### `at`

One-shot jobs in the `at` queue. States: `present`, `absent`.

| Parameter | Type | Required | Status | Notes |
|-----------|------|----------|--------|-------|
| `command` | string | conditional | stable | Required for `present` |
| `time` | string | conditional | stable | `at` time spec; required for `present` |
| `queue` | string | optional | stable | Queue letter `a`–`z` (default `a`) |

Not yet supported (planned, #109): replace-on-change re-queue, per-user queues,
batch mode.

### `systemd_timer`

systemd timer units. States: `present`, `absent`.

| Parameter | Type | Required | Status | Notes |
|-----------|------|----------|--------|-------|
| `on_calendar` | string | conditional | stable | Calendar expression; required for `present` |
| `service` | string | optional | stable | Target unit (default `<name>.service`) |
| `persistent` | bool | optional | stable | systemd `Persistent=` flag |
| `description` | string | optional | stable | Unit description |
| `enable` | bool | optional | stable | Enable + activate at boot (default true) |

Not yet supported (planned, #106): generated companion service, per-user timers,
additional `[Timer]` knobs.

---

## Users & access

### `user`

Linux user accounts. States: `present`, `absent`.

| Parameter | Type | Required | Status | Notes |
|-----------|------|----------|--------|-------|
| `uid` | int | optional | stable | Numeric UID |
| `gid` | int | optional | stable | Numeric primary GID; mutually exclusive with `group` |
| `group` | string | optional | stable | Primary group name; mutually exclusive with `gid` |
| `home` | string | optional | stable | Home directory (absolute path) |
| `shell` | string | optional | stable | Login shell (absolute path) |
| `comment` | string | optional | stable | GECOS field |
| `groups` | []string | optional | stable | Supplementary groups |
| `system` | bool | optional | stable | Create as system user |
| `create_home` | bool | optional | stable | Create home directory |
| `remove_home` | bool | optional | stable | Delete home on `absent` |

Note: on BusyBox systems, scalar-field modification of an existing account is
unsupported (`ErrModUnsupported`); creation/removal work.

### `group`

Unix groups. States: `present`, `absent`.

| Parameter | Type | Required | Status | Notes |
|-----------|------|----------|--------|-------|
| `gid` | int | optional | stable | Numeric GID |
| `system` | bool | optional | stable | Create as system group |

### `ssh`

`authorized_keys` entries. States: `present`, `absent`.

| Parameter | Type | Required | Status | Notes |
|-----------|------|----------|--------|-------|
| `key` | string | required | stable | Public key (`keytype blob [comment]`) |
| `user` | string | required | stable | Target user |
| `options` | string \| []string | optional | stable | `authorized_keys` options prefix |
| `comment` | string | optional | stable | Line comment override |

Not yet supported (planned, #113): key validation, options-set comparison,
whole-file management.

---

## Kernel & system

### `sysctl`

Kernel parameters with optional `/etc/sysctl.d` persistence. State: `present`.

| Parameter | Type | Required | Status | Notes |
|-----------|------|----------|--------|-------|
| `value` | string | required | stable | Parameter value |
| `persist` | bool | optional | stable | Write drop-in (default true) |

Note: one drop-in file per key; a consolidated file is a future enhancement.

### `kmod`

Kernel module load + boot persistence. States: `present`, `absent`.

| Parameter | Type | Required | Status | Notes |
|-----------|------|----------|--------|-------|
| `persist` | bool | optional | stable | Add to boot-load config (default true) |

### `hostname`

Static hostname (the declaration name is the desired hostname). State:
`present`. No parameters beyond `severity`.

### `timezone`

System timezone (the declaration name is the zone, e.g. `UTC`). State:
`present`. No parameters beyond `severity`.

### `system`

Banner / reboot / locale — exactly one operation per declaration. State:
`present`.

| Parameter | Type | Required | Status | Notes |
|-----------|------|----------|--------|-------|
| `banner` | string | optional | stable | `motd`, `issue`, or `issue_net` |
| `content` | string | conditional | stable | Banner content; required with `banner` |
| `reboot` | bool | optional | stable | Trigger reboot (must be `true`) |
| `when_file` | string | optional | stable | Reboot marker (default `/var/run/reboot-required`) |
| `delay` | int | optional | stable | Minutes before reboot (0–60, default 1) |
| `locale` | string | optional | stable | POSIX locale identifier |

Not yet supported (planned, #22): reboot disconnect-tolerance, cross-distro
reboot detection, dual-file locale, `absent` semantics.

### `swap`

Swapfile or swap partition with fstab + swapon state. States: `on`, `present`
(fstab only), `off`, `absent`.

| Parameter | Type | Required | Status | Notes |
|-----------|------|----------|--------|-------|
| `size` | string | conditional | stable | Swapfile size (e.g. `2G`); `on` state only |
| `priority` | int | optional | stable | `-1`–`32767`; invalid with `off`/`absent` |

Not yet supported (planned, #112): `UUID=`/`LABEL=` sources, resize, custom
opts. Sources must be absolute paths in v0.5.

---

## Storage

### `disk`

Filesystem signatures on block devices. States: `present`, `absent`.

| Parameter | Type | Required | Status | Notes |
|-----------|------|----------|--------|-------|
| `device` | string | required | stable | Block device path |
| `fstype` | string | conditional | stable | ext2–4, xfs, btrfs, f2fs, vfat, exfat, swap; required for `present` |
| `mkfs_options` | []string | optional | stable | Flags passed to mkfs |
| `force` | bool | optional | stable | Overwrite an existing filesystem |
| `resize_fs` | bool | optional | stable | Grow fs to device (ext/xfs/btrfs/f2fs) |

Not yet supported (planned, #24): partition management, encryption, label/UUID
management, broader fstype catalog.

### `mount`

`/etc/fstab` entry + live mount. States: `mounted`, `present` (fstab only),
`unmounted`, `absent`.

| Parameter | Type | Required | Status | Notes |
|-----------|------|----------|--------|-------|
| `device` | string | conditional | stable | Required for `mounted`/`present` |
| `fstype` | string | conditional | stable | Required for `mounted`/`present` |
| `opts` | string | optional | stable | Mount options (default `defaults`) |
| `dump` | int | optional | stable | fstab dump field |
| `pass` | int | optional | stable | fstab fsck pass field |
| `mkmnt` | bool | optional | stable | Create mount point (default true) |

Not yet supported (planned, #111): remount-on-change, crypttab integration,
swap mounts.

### `lvm`

LVM PV / VG / LV — exactly one object per declaration. States: `present`,
`absent`.

| Parameter | Type | Required | Status | Notes |
|-----------|------|----------|--------|-------|
| `pv` | string | conditional | stable | PV device; PV operation |
| `vg` | string | conditional | stable | VG name; VG/LV operation |
| `pvs` | []string | conditional | stable | VG member devices; reconciled (add/remove) |
| `lv` | string | conditional | stable | LV name; requires `vg` |
| `size` | string | optional | stable | LV size (e.g. `10G`); mutually exclusive with `extents` |
| `extents` | string | optional | stable | e.g. `100%FREE`; create-only; mutually exclusive with `size` |
| `resize_fs` | bool | optional | stable | `--resizefs` on grow |

Not yet supported (planned, #23): LV shrink, extents-based resize, thin/cache/
snapshot, RAID, PV metadata management, allocation policy.

---

## Networking

`persist` selects the boot-survive renderer: `networkd` (systemd-networkd),
`netplan` (Ubuntu), or `auto`. These two renderers are the v0.5 commitment;
NetworkManager / RHEL `ifcfg-*` are planned for v0.6+ (#25). Omitting `persist`
applies runtime-only configuration that does not survive reboot.

### `network`

One interface's runtime config + optional persistence. States: `present`,
`absent`.

| Parameter | Type | Required | Status | Notes |
|-----------|------|----------|--------|-------|
| `interface` | string | required | stable | Interface name (≤15 chars) |
| `addresses` | []string | optional | stable | CIDR list (full per-interface set) |
| `mtu` | int | optional | stable | Link MTU (68–65535) |
| `up` | bool | optional | stable | Admin up/down |
| `persist` | string | optional | stable | `networkd`/`netplan`/`auto` |

At least one of `addresses`/`mtu`/`up` is required. Not yet supported
(planned, #25): per-family addresses, DNS/NTP/domain management.

### `route`

One routing-table entry. States: `present`, `absent`.

| Parameter | Type | Required | Status | Notes |
|-----------|------|----------|--------|-------|
| `destination` | string | required | stable | CIDR or host IP |
| `gateway` | string | conditional | stable | Next hop; one of gateway/interface required for `present` |
| `interface` | string | conditional | stable | Output interface; required for `persist` |
| `metric` | int | optional | stable | Route metric |
| `table` | string \| int | optional | stable | Routing table |
| `persist` | string | optional | stable | `networkd`/`netplan`/`auto` |

Not yet supported (planned, #26): source-routing rules, multipath, richer route
attributes.

### `bond`

Link-aggregation interface. States: `present`, `absent`.

| Parameter | Type | Required | Status | Notes |
|-----------|------|----------|--------|-------|
| `name` | string | required | stable | Bond interface name (≤15 chars) |
| `mode` | string | optional | stable | `balance-rr` (default) … `802.3ad`, or numeric 0–6 |
| `members` | []string | optional | stable | Interfaces enslaved at creation |
| `miimon` | int | optional | stable | Link-monitor interval (ms) |
| `persist` | string | optional | stable | `networkd`/`netplan`/`auto` |

Not yet supported (planned, #27): in-place attribute/member reconciliation —
to change a live bond, delete then re-declare.

### `bridge`

Bridge interface. States: `present`, `absent`.

| Parameter | Type | Required | Status | Notes |
|-----------|------|----------|--------|-------|
| `name` | string | required | stable | Bridge interface name (≤15 chars) |
| `members` | []string | optional | stable | Ports added at creation |
| `stp` | bool | optional | stable | Spanning Tree Protocol (default false) |
| `persist` | string | optional | stable | `networkd`/`netplan`/`auto` |

Not yet supported (planned, #28): in-place reconciliation of `stp`/`members`.

### `vlan`

802.1Q VLAN interface. States: `present`, `absent`.

| Parameter | Type | Required | Status | Notes |
|-----------|------|----------|--------|-------|
| `name` | string | required | stable | VLAN interface name (≤15 chars) |
| `parent` | string | conditional | stable | Underlying interface; required for `present` |
| `id` | int | conditional | stable | VLAN ID 1–4094; required for `present` |
| `persist` | string | optional | stable | `networkd`/`netplan`/`auto` |

Not yet supported (planned, #29): in-place reconciliation — an existing VLAN of
the same name is treated as converged regardless of `id`/`parent`; QinQ; VLAN
ranges.

---

## Firewall

### `firewall`

Backend-dispatching abstraction over iptables/nftables/firewalld.

| Parameter | Type | Required | Status | Notes |
|-----------|------|----------|--------|-------|
| `service` | string | conditional | stable | One of service/port required |
| `port` | string | conditional | stable | `PORT[-PORT]/{tcp,udp,sctp,dccp}` |
| `backend` | string | optional | stable | Force `iptables`/`nftables`/`firewalld` |
| `zone` | string | optional | stable | firewalld zone (firewalld backend) |
| `strict_catalog` | bool | optional | stable | Require catalog service name (default true) |

Not yet supported (planned, #20): deny action, IPv6 on the iptables backend,
chain/table overrides, service-catalog expansion.

### `firewalld`

firewalld zone items. States: `present`, `absent`.

| Parameter | Type | Required | Status | Notes |
|-----------|------|----------|--------|-------|
| `zone` | string | required | stable | Zone name (default `public`) |
| `service` | string | conditional | stable | One of service/port/rich_rule required |
| `port` | string | conditional | stable | `PORT[-PORT]/{tcp,udp,sctp,dccp}` |
| `rich_rule` | string | conditional | stable | Rich-rule syntax (canonicalised) |
| `reload` | bool | optional | stable | Reload after change (default true) |

Not yet supported (planned, #19): whole-zone management, masquerade/forward-port,
direct rules.

### `iptables`

A single iptables/ip6tables rule. States: `present`, `absent`.

| Parameter | Type | Required | Status | Notes |
|-----------|------|----------|--------|-------|
| `chain` | string | required | stable | Target chain |
| `rule` | string \| []string | required | stable | Match spec + target |
| `table` | string | optional | stable | `filter` (default)/`nat`/`mangle`/`raw`/`security` |
| `family` | string | optional | stable | `ipv4` (default)/`ipv6` |
| `position` | int | optional | stable | Insert position; `present` only |
| `save` | string | optional | stable | Path for `iptables-save` output |

Not yet supported (planned, #114): structured rule model, ordering guarantees,
both-family in one decl, distro persistence integration.

### `nftables`

A single nftables rule. States: `present`, `absent`.

| Parameter | Type | Required | Status | Notes |
|-----------|------|----------|--------|-------|
| `table` | string | required | stable | Existing table |
| `chain` | string | required | stable | Existing chain |
| `rule` | string \| []string | required | stable | Match + statement |
| `family` | string | optional | stable | Default `inet` |
| `index` | int | optional | stable | Insert position; `present` only |
| `save` | string | optional | stable | Path for ruleset dump |

Not yet supported (planned, #115): structured rules, ordering, table/chain
management, comment matching.

---

## Security

### `security`

SELinux modes/booleans and AppArmor profile modes — exactly one operation per
declaration. State: `present` only. **Module maturity: experimental** (highly
distro-specific; only partially exercised on the matrix).

| Parameter | Type | Required | Status | Notes |
|-----------|------|----------|--------|-------|
| `mode` | string | conditional | experimental | SELinux mode `enforcing`/`permissive`/`disabled` |
| `boolean` | string | conditional | experimental | SELinux boolean to toggle |
| `value` | bool | conditional | experimental | Boolean value; requires `boolean` |
| `apparmor.profile` | string | conditional | experimental | AppArmor profile name/path |
| `apparmor.profile_mode` | string | conditional | experimental | `enforce`/`complain`/`disable`; requires profile |

Note: SELinux `mode: disabled` cannot transition at runtime (reboot required).
Not yet supported (planned, #21): SELinux file contexts / ports / modules /
logins, `absent` semantics.

---

## PKI

### `pki`

A TLS certificate + private key on disk (the declaration name is the cert
path). States: `present`, `absent`.

| Parameter | Type | Required | Status | Notes |
|-----------|------|----------|--------|-------|
| `key_path` | string | required | stable | Private-key path (must differ from cert path) |
| `common_name` | string | conditional | stable | CN; one of CN/SAN required for `present` |
| `subject_alt_names` | string \| []string | conditional | stable | SANs |
| `organization` | string | optional | stable | Subject organization |
| `days` | int | optional | stable | Validity (default 365) |
| `renew_days` | int | optional | stable | Renewal threshold (default 30) |
| `key_type` | string | optional | stable | `rsa` (default)/`ecdsa`/`ed25519` |
| `rsa_bits` | int | optional | stable | RSA size (default/min 2048) |
| `ecdsa_curve` | string | optional | stable | `p256` (default)/`p384`/`p521` |
| `is_ca` | bool | optional | stable | Mark certificate as a CA |
| `ca_cert` | string | optional | stable | Signing CA cert; pair with `ca_key` |
| `ca_key` | string | optional | stable | Signing CA key; pair with `ca_cert` |

`ca_cert`/`ca_key` must be set together (omit both for self-signed). Not yet
supported (planned, #110): combined PEM output, encrypted keys, broader issuance
options.

---

## Execution

### `command`

Guarded shell execution. State: `run` only. At least one guard
(`unless`/`onlyif`/`creates`) is required for idempotency.

| Parameter | Type | Required | Status | Notes |
|-----------|------|----------|--------|-------|
| `command` | string | required | stable | Executed via `/bin/sh -c` |
| `cwd` | string | optional | stable | Working directory |
| `env` | map[string]string | optional | stable | Environment variables |
| `timeout_seconds` | int | optional | stable | Default 60, max 3600 |
| `unless` | string | optional | stable | Skip if guard exits zero |
| `onlyif` | string | optional | stable | Skip if guard exits non-zero |
| `creates` | string | optional | stable | Skip if absolute path exists |
| `shell` | string | optional | experimental | Only `/bin/sh` accepted; alternate shells rejected |

Note: `shell` rejects any non-default value today (`"shell not supported … only
/bin/sh"`). To force an always-run command, use `onlyif: /bin/true`.

---

## See also

- [`VERSIONING.md`](VERSIONING.md) — the v0.5 gate this matrix satisfies.
- [`ROADMAP.md`](ROADMAP.md) — ranked backlog; the `#NN` references above are
  Codeberg issues in the `gate-v0.5` / `v0.x` milestones.
- [`COMPATIBILITY.md`](COMPATIBILITY.md) — release cadence, support windows, and
  upgrade/compatibility policy.
