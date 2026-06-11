#!/usr/bin/env sh
# SPDX-License-Identifier: Apache-2.0

#
# test/e2e/state/smoke.lvm.sh — the `lvm`-module phase of the
# cross-distro matrix, run inside each container by smoke.sh after the
# firewall phase.
#
# lvm operates on real block devices and the device-mapper, which a
# container lacks, so each Physical Volume is backed by a loopback device
# (losetup over a sparse image). The phase drives the module through its
# create lifecycle and the gate-v0.5 reconcile/resize additions, each as
# a separate harness invocation so the grow/extend paths are exercised
# idempotently without a raw pre-seed:
#
#   1. create  — pv → vg → lv (size); applied twice (create + no-op).
#   2. LV grow — re-declare the LV larger; the module `lvextend`s it on
#                pass 1 and no-ops on pass 2 (`lvextend`, #170).
#   3. VG reconcile (needs a 2nd loop) — declare a second PV in the VG;
#                the module `pvcreate`s it and `vgextend`s the VG on pass
#                1 and no-ops on pass 2 (VG PV-set reconcile, #178).
#
# It is self-gating: it skips cleanly (exit 0) when lvm2 can't be
# installed, when loop devices aren't available (host kernel), or — for
# the reconcile scenario — when a second free loop can't be allocated
# (hosts can be short on loops, e.g. snapd holds several). Teardown is
# guaranteed by the trap (raw lvm + dmsetup + losetup), independent of
# any module apply, so nothing leaks.
#
# POSIX sh (dash / busybox ash / bash). No bashisms, no `local`. Helper
# functions return values via a global (KSCORE_RESULT) rather than
# command substitution, so their LOOPS append survives (a $(...)
# subshell would discard it) — same shape as smoke.disk.sh.

set -eu

DISTRO="${KSCORE_DISTRO:-unknown}"
PKG_MGR="${KSCORE_PKG_MGR:?KSCORE_PKG_MGR required}"
HARNESS="${KSCORE_HARNESS:-/usr/local/bin/kscore-state-harness}"
ROOT=/var/tmp/kscore-smoke/lvm
mkdir -p "$ROOT"

# Device-mapper is a host-global namespace shared by every privileged
# container, so the VG name must be unique per distro — otherwise the
# `<vg>-<lv>` dm node (e.g. ksvg-kslv) collides across the matrix's
# sequentially-run containers when a sibling's teardown lags, and
# `lvcreate` fails "device-mapper: create ioctl … Device or resource
# busy". Derive it from the distro label (sanitised to a valid LVM name).
VGNAME="ksvg_$(printf '%s' "$DISTRO" | tr -c 'A-Za-z0-9' '_')"
LVNAME=kslv

# Suppress the leaked-fd warnings the tools print under `docker exec`.
export LVM_SUPPRESS_FD_WARNINGS=1

# LVM-in-a-container needs a tailored config, applied via LVM_SYSTEM_DIR
# so every lvm invocation inherits it (the module's tools and this
# script's teardown alike). Three things matter:
#
#   * Node management without functional udev. The systemd distros boot
#     systemd-udevd, but it does not reliably create the device-mapper
#     nodes a container needs, so `lvcreate`'s zeroing step fails to open
#     the new LV ("device not cleared"). Setting `udev_sync = 0` /
#     `udev_rules = 0` makes LVM create and manage the nodes itself.
#     NOTE: we do this via the config, NOT the `DM_DISABLE_UDEV`
#     environment variable — that variable, when udev *is* running,
#     makes libdevmapper print a "Udev is running and DM_DISABLE_UDEV …
#     Bypassing udev" warning to stderr, and the module reads its tools'
#     combined stdout+stderr, so the warning corrupts the pvs/vgs/lvs
#     output it parses ("post-apply Test returned false" / "unexpected
#     lvs size output …"). The config-only bypass avoids the warning.
#   * Device listing without udev: `obtain_device_list_from_udev = 0`
#     scans /dev directly (a no-udev distro's listing returns nothing
#     otherwise), and `hints = "none"` / `use_lvmetad = 0` drop the
#     caches that go stale without udev/lvmetad.
#   * A loop-only `global_filter` so LVM never scans (let alone touches)
#     the host's disks, which are visible inside the privileged
#     container — both a safety boundary and a determinism/speed win.
#     `use_devicesfile = 0` is required for the filter to apply at all:
#     newer LVM (RHEL 9 / Rocky) defaults to a "devices file", which
#     *ignores* global_filter and prints "remove the lvm.conf
#     global_filter, it is ignored with the devices file" to stderr —
#     which would again corrupt the module's combined-output parse.
#     Disabling it restores uniform filter-based scanning everywhere.
#     But the setting itself only exists in LVM >= 2.03.12, and an older
#     LVM (e.g. Ubuntu 22.04's 2.03.11) warns "unknown" about it on every
#     command — the same parse-corrupting stderr noise — so it is emitted
#     only when a capability probe confirms the running LVM recognises it.
#
# All are safe in a throwaway container and on every distro in the
# matrix. `backup`/`archive` are off to avoid metadata-backup writes.
#
# The `global_filter` accepts ONLY the loop devices this phase allocates
# (rewritten by write_lvm_conf as each is created), never a bare
# /dev/loop* glob: a privileged container shares the host's loop devices
# and device-mapper, so any *other* loop carrying an LVM VG — a leftover
# from a crashed prior run, or a sibling distro in this same matrix whose
# teardown lagged — would otherwise be visible and drag the VG PV-set
# reconcile into trying to `vgreduce` a foreign PV. Scoping the filter to
# our own loops isolates the phase completely.
export LVM_SYSTEM_DIR="$ROOT/lvmconf"
mkdir -p "$LVM_SYSTEM_DIR"

# Set to the use_devicesfile line once a capability probe (below, after
# the lvm tools are installed) confirms the running LVM knows the
# setting; empty on older LVM that would warn "unknown" about it.
DEVICESFILE_LINE=""

# write_lvm_conf <space-separated device list> — (re)write the container
# lvm.conf with a global_filter accepting exactly those devices.
write_lvm_conf() {
  _accept=""
  for _d in $1; do _accept="${_accept} \"a|^${_d}\$|\","; done
  _accept="${_accept} \"r|.*|\""
  cat >"$LVM_SYSTEM_DIR/lvm.conf" <<LVMCONF
devices {
${DEVICESFILE_LINE}
    obtain_device_list_from_udev = 0
    hints = "none"
    global_filter = [ ${_accept} ]
}
global {
    use_lvmetad = 0
}
activation {
    udev_sync = 0
    udev_rules = 0
    verify_udev_operations = 0
}
backup {
    backup = 0
    archive = 0
}
LVMCONF
}

# Until a loop is allocated, reject every device (so any stray lvm
# invocation — including teardown — touches nothing on the host).
write_lvm_conf ""

log() { echo "==> [${DISTRO}] lvm: $*"; }
have() { command -v "$1" >/dev/null 2>&1; }

# make_sparse <path> <sizeMB>
make_sparse() {
  rm -f "$1"
  truncate -s "${2}M" "$1" 2>/dev/null || dd if=/dev/zero of="$1" bs=1M count="$2" >/dev/null 2>&1
}

LOOPS=""
# lvm_teardown — remove OUR VG and dm node only; never `dmsetup
# remove_all`, which is host-global and would nuke unrelated (even the
# host's own) inactive device-mapper devices.
lvm_teardown() {
  vgchange -an "$VGNAME" >/dev/null 2>&1 || true
  vgremove -f "$VGNAME" >/dev/null 2>&1 || true
  dmsetup remove "${VGNAME}-${LVNAME}" >/dev/null 2>&1 || true
}
cleanup() {
  # Guaranteed teardown, independent of the module: deactivate + remove
  # our VG (takes its LV + dm node with it), wipe the PV labels, then
  # detach the loops. All best-effort, all scoped to our own objects.
  lvm_teardown
  for l in $LOOPS; do pvremove -ff -y "$l" >/dev/null 2>&1 || true; done
  for l in $LOOPS; do losetup -d "$l" 2>/dev/null || true; done
  rm -rf "$ROOT" 2>/dev/null || true
}
trap cleanup EXIT

# --- install lvm2 (best effort; the gate below handles gaps) ----------
case "$PKG_MGR" in
  apt)    DEBIAN_FRONTEND=noninteractive apt-get install -y -qq lvm2 >/dev/null 2>&1 || true ;;
  dnf)    dnf install -y -q lvm2 >/dev/null 2>&1 || true ;;
  apk)    apk add --no-cache lvm2 >/dev/null 2>&1 || true ;;
  zypper) zypper --non-interactive install lvm2 >/dev/null 2>&1 || true ;;
  pacman) pacman -S --noconfirm --needed lvm2 >/dev/null 2>&1 || true ;;
  *) log "unknown package manager ${PKG_MGR}" ;;
esac

# --- gate: the lvm tools must be on PATH ------------------------------
if ! have pvcreate || ! have vgcreate || ! have lvcreate; then
  log "lvm2 not installable on ${DISTRO} — skipping lvm phase"
  exit 0
fi

# --- probe: does this LVM know `use_devicesfile`? ---------------------
# Added in LVM 2.03.12. Probe with `--type full`, which resolves the
# setting against the built-in *defaults*: it exits 0 (printing the
# default value) when the build recognises the setting and non-zero
# ("node … not found") when it doesn't. A bare `lvm config <path>` is
# NOT a capability test — it reports only *explicitly set* values, so a
# recognised-but-unset setting (the usual case, e.g. on Rocky) reads
# "not found" and would be wrongly treated as unsupported. Only emit the
# line when recognised: older LVM (Ubuntu 22.04's 2.03.11) warns about an
# unknown setting on every command, corrupting the module's
# combined-output parse — the same class of bug the config steers around.
if have lvm && lvm config --type full devices/use_devicesfile >/dev/null 2>&1; then
  DEVICESFILE_LINE="    use_devicesfile = 0"
fi

# --- gate: loop-device support (host kernel) --------------------------
if ! have losetup; then
  log "losetup absent — skipping lvm phase"
  exit 0
fi
probe="$ROOT/.probe.img"
make_sparse "$probe" 1
if pl=$(losetup -f --show "$probe" 2>/dev/null); then
  losetup -d "$pl" 2>/dev/null || true
else
  log "loop devices unavailable (no loop kernel module?) — skipping lvm phase"
  rm -f "$probe"
  exit 0
fi
rm -f "$probe"

# mk_loop <name> <sizeMB> → sets KSCORE_RESULT to the loop device and
# records it for teardown; returns 1 (KSCORE_RESULT="") when no free loop
# device can be allocated, so the caller can skip that scenario rather
# than fail the phase. Not run in a subshell (so LOOPS survives).
mk_loop() {
  _img="$ROOT/$1.img"
  make_sparse "$_img" "$2"
  if ! KSCORE_RESULT=$(losetup -f --show "$_img" 2>/dev/null); then
    rm -f "$_img"
    KSCORE_RESULT=""
    return 1
  fi
  LOOPS="$LOOPS $KSCORE_RESULT"
}

# A PV needs a few MB of metadata headroom; 64MB images give the small
# LVs (16/32MB) room without straining the host loop budget.
PV0=""
if ! mk_loop pv0 64; then
  log "no free loop device — skipping lvm phase (host loops exhausted)"
  exit 0
fi
PV0="$KSCORE_RESULT"
write_lvm_conf "$PV0"

# Pre-clean any stale VG / dm node from a crashed prior run of THIS
# distro (the name is distro-unique, so this never touches a sibling's
# or the host's objects) before we create — otherwise a lingering
# <vg>-<lv> dm node would collide on lvcreate.
lvm_teardown

# emit_fixture <file> writes a metadata header; the caller appends ops.
fixture_header() {
  {
    echo "metadata:"
    echo "  name: kscore-cross-distro-smoke-lvm"
    echo "  version: \"0.1\""
  } >"$1"
}

# --- scenario 1: create lifecycle (1 loop) ---------------------------
F1="$ROOT/lvm-create.yaml"
fixture_header "$F1"
{
  echo "lvm:"
  echo "  pv0:"
  echo "    state: present"
  echo "    pv: $PV0"
  echo "  vg:"
  echo "    state: present"
  echo "    vg: $VGNAME"
  echo "    pvs:"
  echo "      - $PV0"
  echo "    require:"
  echo "      - lvm: pv0"
  echo "  lv:"
  echo "    state: present"
  echo "    lv: $LVNAME"
  echo "    vg: $VGNAME"
  echo "    size: 16M"
  echo "    require:"
  echo "      - lvm: vg"
} >>"$F1"
log "create: pv -> vg -> lv (apply + idempotency re-apply)"
"$HARNESS" "$F1"

# --- scenario 2: LV grow on the same VG (lvextend, #170) -------------
F2="$ROOT/lvm-grow.yaml"
fixture_header "$F2"
{
  echo "lvm:"
  echo "  lv:"
  echo "    state: present"
  echo "    lv: $LVNAME"
  echo "    vg: $VGNAME"
  echo "    size: 32M"
} >>"$F2"
log "grow: lvextend ${LVNAME} 16M -> 32M (grow on pass 1, no-op on pass 2)"
"$HARNESS" "$F2"

# --- scenario 3: VG PV-set reconcile (vgextend, #178) — 2nd loop -----
if mk_loop pv1 64; then
  PV1="$KSCORE_RESULT"
  write_lvm_conf "$PV0 $PV1"
  F3="$ROOT/lvm-reconcile.yaml"
  fixture_header "$F3"
  {
    echo "lvm:"
    echo "  pv0:"
    echo "    state: present"
    echo "    pv: $PV0"
    echo "  pv1:"
    echo "    state: present"
    echo "    pv: $PV1"
    echo "  vg:"
    echo "    state: present"
    echo "    vg: $VGNAME"
    echo "    pvs:"
    echo "      - $PV0"
    echo "      - $PV1"
    echo "    require:"
    echo "      - lvm: pv0"
    echo "      - lvm: pv1"
  } >>"$F3"
  log "reconcile: pvcreate ${PV1} + vgextend ${VGNAME} (extend on pass 1, no-op on pass 2)"
  "$HARNESS" "$F3"
else
  log "reconcile: no second free loop device — skipping VG PV-set reconcile scenario"
fi

log "OK"
