#!/usr/bin/env sh
# SPDX-License-Identifier: Apache-2.0

#
# test/e2e/state/smoke.mount.sh — the `mount`-module phase of the
# cross-distro matrix, run inside each container by smoke.sh after the
# netdev phase.
#
# `mount` manages a filesystem's /etc/fstab entry and its live mount
# state (mount(8)/umount(8), /proc/mounts). It needs a real block device
# and is safe in the matrix because a privileged container has its OWN
# mount namespace — the mount is private to the container and is torn
# down with it. The phase backs the mount with a loopback device
# (losetup over a sparse image), formats it ext4, applies a `mounted`
# declaration twice, and asserts the second pass is a zero-change no-op
# (the fstab entry is upserted + the device mounted on pass 1).
#
# Self-gating: skips cleanly (exit 0) when loop devices aren't available
# (host kernel) or the mkfs/mount tools are missing. The loop, mount and
# image are torn down on exit.
#
# POSIX sh (dash / busybox ash / bash). No bashisms, no `local`.

set -eu

DISTRO="${KSCORE_DISTRO:-unknown}"
PKG_MGR="${KSCORE_PKG_MGR:?KSCORE_PKG_MGR required}"
HARNESS="${KSCORE_HARNESS:-/usr/local/bin/kscore-state-harness}"
ROOT=/var/tmp/kscore-smoke/mount
MNT="$ROOT/m0"
mkdir -p "$ROOT"

log() { echo "==> [${DISTRO}] mount: $*"; }
have() { command -v "$1" >/dev/null 2>&1; }

make_sparse() {
  rm -f "$1"
  truncate -s "${2}M" "$1" 2>/dev/null || dd if=/dev/zero of="$1" bs=1M count="$2" >/dev/null 2>&1
}

LOOP=""
cleanup() {
  umount "$MNT" >/dev/null 2>&1 || true
  [ -n "$LOOP" ] && losetup -d "$LOOP" >/dev/null 2>&1 || true
  rm -rf "$ROOT" 2>/dev/null || true
}
trap cleanup EXIT

# --- install mkfs.ext4 + mount tooling (best effort) ------------------
case "$PKG_MGR" in
  apt)    DEBIAN_FRONTEND=noninteractive apt-get install -y -qq e2fsprogs mount util-linux >/dev/null 2>&1 || true ;;
  dnf)    dnf install -y -q e2fsprogs util-linux >/dev/null 2>&1 || true ;;
  apk)    apk add --no-cache e2fsprogs util-linux >/dev/null 2>&1 || true ;;
  zypper) zypper --non-interactive install e2fsprogs util-linux >/dev/null 2>&1 || true ;;
  pacman) pacman -S --noconfirm --needed e2fsprogs util-linux >/dev/null 2>&1 || true ;;
  *) log "unknown package manager ${PKG_MGR}" ;;
esac

# --- gate: mkfs + mount tools -----------------------------------------
if ! have mkfs.ext4 || ! have mount || ! have umount || ! have losetup; then
  log "mkfs.ext4 / mount tools unavailable on ${DISTRO} — skipping mount phase"
  exit 0
fi

# --- gate: loop-device support (host kernel) --------------------------
probe="$ROOT/.probe.img"
make_sparse "$probe" 1
if pl=$(losetup -f --show "$probe" 2>/dev/null); then
  losetup -d "$pl" 2>/dev/null || true
else
  log "loop devices unavailable (no loop kernel module?) — skipping mount phase"
  rm -f "$probe"
  exit 0
fi
rm -f "$probe"

# --- set up a loop-backed ext4 device ---------------------------------
img="$ROOT/fs.img"
make_sparse "$img" 32
if ! LOOP=$(losetup -f --show "$img" 2>/dev/null); then
  log "no free loop device — skipping mount phase (host loops exhausted)"
  exit 0
fi
if ! mkfs.ext4 -q -F "$LOOP" >/dev/null 2>&1; then
  log "mkfs.ext4 failed — skipping mount phase"
  exit 0
fi

# --- fixture: a `mounted` declaration at a scratch mount point --------
# Declaration.Name is the mount point; the module upserts the fstab
# entry, creates the dir (mkmnt), and mounts the device.
FIX="$ROOT/smoke.mount.yaml"
{
  echo "metadata:"
  echo "  name: kscore-cross-distro-smoke-mount"
  echo "  version: \"0.1\""
  echo "mount:"
  echo "  ${MNT}:"
  echo "    state: mounted"
  echo "    device: ${LOOP}"
  echo "    fstype: ext4"
  echo "    mkmnt: true"
} >"$FIX"

log "applying mount fixture (mount + fstab, then idempotency re-apply)"
"$HARNESS" "$FIX"
log "OK"
