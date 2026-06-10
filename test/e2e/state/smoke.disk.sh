#!/usr/bin/env sh
# SPDX-License-Identifier: Apache-2.0

#
# test/e2e/state/smoke.disk.sh — the `disk`-module phase of the
# cross-distro matrix, run inside each container by smoke.sh after the
# main fixture.
#
# disk operates on block devices, which a container lacks, so each
# scenario is backed by a loopback device (losetup over a sparse image):
#
#   * mkfs   — a blank loop; the module formats it (pass 1) and reports
#              a no-op (pass 2).
#   * resize — a loop pre-seeded with a small fs on a *grown* device
#              (mkfs small, then grow the backing image + `losetup -c`);
#              the module grows the fs to fill it (pass 1) and no-ops
#              (pass 2). ext/f2fs resize the unmounted device directly;
#              xfs/btrfs resize by mountpoint, so those are mounted first.
#
# It is self-gating: it skips cleanly (exit 0) when loop devices aren't
# available (host kernel) and, per fstype, when that fstype's tools
# aren't present (e.g. btrfs-progs / f2fs-tools are absent on Rocky 9).
# Loops, mounts and images are torn down on exit.
#
# POSIX sh (dash / busybox ash / bash). No bashisms, no `local`. Helper
# functions return values via a global (KSCORE_RESULT) rather than
# command substitution, so their LOOPS/MOUNTS appends survive (a $(...)
# subshell would discard them).

set -eu

DISTRO="${KSCORE_DISTRO:-unknown}"
PKG_MGR="${KSCORE_PKG_MGR:?KSCORE_PKG_MGR required}"
HARNESS="${KSCORE_HARNESS:-/usr/local/bin/kscore-state-harness}"
ROOT=/var/tmp/kscore-smoke/disk
mkdir -p "$ROOT"

LOOPS=""
MOUNTS=""
cleanup() {
  for m in $MOUNTS; do umount "$m" 2>/dev/null || true; done
  for l in $LOOPS; do losetup -d "$l" 2>/dev/null || true; done
  rm -rf "$ROOT" 2>/dev/null || true
}
trap cleanup EXIT

log() { echo "==> [${DISTRO}] disk: $*"; }
have() { command -v "$1" >/dev/null 2>&1; }

# make_sparse <path> <sizeMB>
make_sparse() {
  rm -f "$1"
  truncate -s "${2}M" "$1" 2>/dev/null || dd if=/dev/zero of="$1" bs=1M count="$2" >/dev/null 2>&1
}

# --- gate: loop-device support (host kernel) ------------------------
if ! have losetup; then
  log "losetup absent — skipping disk phase"
  exit 0
fi
probe="$ROOT/.probe.img"
make_sparse "$probe" 1
if pl=$(losetup -f --show "$probe" 2>/dev/null); then
  losetup -d "$pl" 2>/dev/null || true
else
  log "loop devices unavailable (no loop kernel module?) — skipping disk phase"
  rm -f "$probe"
  exit 0
fi
rm -f "$probe"

# --- install fs tools (best effort; per-fstype gating handles gaps) -
case "$PKG_MGR" in
  apt) DEBIAN_FRONTEND=noninteractive apt-get install -y -qq e2fsprogs xfsprogs btrfs-progs f2fs-tools util-linux >/dev/null 2>&1 || true ;;
  dnf) dnf install -y -q e2fsprogs xfsprogs btrfs-progs f2fs-tools util-linux >/dev/null 2>&1 || true ;;
  apk) apk add --no-cache e2fsprogs xfsprogs btrfs-progs f2fs-tools util-linux >/dev/null 2>&1 || true ;;
  *) log "unknown package manager ${PKG_MGR}" ;;
esac

# --- helpers --------------------------------------------------------
img_for() { echo "$ROOT/$1.img"; }

# mk_loop <name> <sizeMB> → sets KSCORE_RESULT to the loop device and
# records it for teardown; returns 1 (KSCORE_RESULT="") when no free loop
# device can be allocated, so the caller can skip that scenario rather
# than fail the phase. Hosts can be short on loops (e.g. snapd holds
# several), and `losetup -f` then fails with "No such file or directory".
# Not run in a subshell (so LOOPS survives).
mk_loop() {
  _img=$(img_for "$1")
  make_sparse "$_img" "$2"
  if ! KSCORE_RESULT=$(losetup -f --show "$_img" 2>/dev/null); then
    rm -f "$_img"
    KSCORE_RESULT=""
    return 1
  fi
  LOOPS="$LOOPS $KSCORE_RESULT"
}

# mkfs_one <fstype> <device> — pre-seed the resize scenario's small fs.
mkfs_one() {
  case "$1" in
    ext4)  mkfs.ext4  -q -F "$2" >/dev/null 2>&1 ;;
    xfs)   mkfs.xfs   -f "$2"    >/dev/null 2>&1 ;;
    btrfs) mkfs.btrfs -f "$2"    >/dev/null 2>&1 ;;
    f2fs)  mkfs.f2fs  -f "$2"    >/dev/null 2>&1 ;;
    *) return 1 ;;
  esac
}

# tools_present <fstype> — the module's tools for this fstype are all on
# PATH (mkfs + fill-check + resize, plus mount tooling for xfs/btrfs).
tools_present() {
  have blockdev || return 1
  case "$1" in
    ext4)  have mkfs.ext4  && have resize2fs  && have dumpe2fs ;;
    xfs)   have mkfs.xfs   && have xfs_growfs && have xfs_info && have findmnt && have mount ;;
    btrfs) have mkfs.btrfs && have btrfs      && have findmnt  && have mount ;;
    f2fs)  have mkfs.f2fs  && have resize.f2fs && have findmnt ;;
    *) return 1 ;;
  esac
}

DECLS=""
# emit <label> <device> <fstype> <resize:yes|no> [mkfs_force_flag]
#
# A loop device is a *whole* device, so mkfs.ext4 would print
# "is entire device... Proceed anyway?" and hang without input; the
# force flag (-F for ext, -f for xfs/btrfs/f2fs) makes the module's mkfs
# non-interactive — the realistic way to mkfs a whole device.
emit() {
  DECLS="${DECLS}  $1:
    state: present
    device: $2
    fstype: $3
"
  if [ "$4" = yes ]; then
    DECLS="${DECLS}    resize_fs: true
"
  fi
  if [ -n "${5:-}" ]; then
    DECLS="${DECLS}    mkfs_options:
      - \"$5\"
"
  fi
}

# setup_fstype <fstype> — build the mkfs + resize scenarios.
setup_fstype() {
  _fs="$1"
  case "$_fs" in
    ext4)  _base=16;  _grown=32;  _mnt=no;  _force=-F ;;
    xfs)   _base=320; _grown=400; _mnt=yes; _force=-f ;;
    btrfs) _base=256; _grown=320; _mnt=yes; _force=-f ;;
    f2fs)  _base=64;  _grown=128; _mnt=no;  _force=-f ;;
    *) return 0 ;;
  esac

  # mkfs scenario: a blank loop the module formats (force flag → the
  # whole-device mkfs is non-interactive).
  if ! mk_loop "${_fs}-mkfs" "$_base"; then
    log "${_fs}: no free loop device — skipping (host loops exhausted)"
    return 0
  fi
  emit "${_fs}-mkfs" "$KSCORE_RESULT" "$_fs" no "$_force"

  # resize scenario: a small fs on a grown device.
  if ! mk_loop "${_fs}-resize" "$_base"; then
    log "${_fs}: no free loop device — skipping resize scenario"
    return 0
  fi
  _dev="$KSCORE_RESULT"
  if ! mkfs_one "$_fs" "$_dev"; then
    log "${_fs}: pre-seed mkfs failed — skipping resize scenario"
    return 0
  fi
  make_sparse_grow "$(img_for "${_fs}-resize")" "$_grown"
  losetup -c "$_dev" 2>/dev/null || true # capacity refresh; a weak losetup is a harmless no-op
  if [ "$_mnt" = yes ]; then
    _mp="$ROOT/mnt-${_fs}"
    mkdir -p "$_mp"
    if mount "$_dev" "$_mp" 2>/dev/null; then
      MOUNTS="$MOUNTS $_mp"
    else
      log "${_fs}: mount failed (no kernel support?) — skipping resize scenario"
      return 0
    fi
  fi
  emit "${_fs}-resize" "$_dev" "$_fs" yes
}

# make_sparse_grow <path> <sizeMB> — extend an existing sparse image.
make_sparse_grow() {
  truncate -s "${2}M" "$1" 2>/dev/null || dd if=/dev/zero of="$1" bs=1M count=1 seek="$(( $2 - 1 ))" >/dev/null 2>&1
}

# --- build the scenarios for every available fstype -----------------
for fs in ext4 xfs btrfs f2fs; do
  if tools_present "$fs"; then
    log "${fs}: setting up loop-backed scenarios"
    setup_fstype "$fs"
  else
    log "${fs}: tools unavailable on ${DISTRO} — skipping"
  fi
done

if [ -z "$DECLS" ]; then
  log "no fstypes available — nothing to test"
  exit 0
fi

FIX="$ROOT/smoke.disk.yaml"
{
  echo "metadata:"
  echo "  name: kscore-cross-distro-smoke-disk"
  echo "  version: \"0.1\""
  echo "disk:"
  printf '%s' "$DECLS"
} >"$FIX"

log "applying disk fixture (apply + idempotency re-apply)"
"$HARNESS" "$FIX"
log "OK"
