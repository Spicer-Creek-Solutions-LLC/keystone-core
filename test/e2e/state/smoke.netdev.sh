#!/usr/bin/env sh
# SPDX-License-Identifier: Apache-2.0

#
# test/e2e/state/smoke.netdev.sh — the virtual-interface phase of the
# cross-distro matrix (bond + bridge + vlan), run inside each container
# by smoke.sh after the lvm phase.
#
# These three modules create a Linux virtual interface at runtime via
# `ip link add … type bond|bridge|vlan` and, with `persist:`, render the
# boot-survive config (the shared netpersist helper). The phase builds
# the members/parent from `dummy` interfaces, applies a fixture twice,
# and asserts the second pass is a zero-change no-op — exercising each
# module's create + Check path AND the networkd persist renderer live.
#
# Why this is safe in the matrix (unlike a real NIC): the containers run
# in their OWN network namespace (NOT --network=host), so every interface
# created here is private to the container and vanishes with it. Nothing
# on the host — or in a sibling distro's container — is touched. (The
# kernel bonding/bridge/8021q/dummy modules are global and may get
# auto-loaded, which is harmless.)
#
# Self-gating: skips cleanly (exit 0) when a JSON-capable iproute2 can't
# be installed (BusyBox `ip` lacks `-j`), when the `dummy` module isn't
# available (no members/parent possible), and — per interface type — when
# that type's kernel module can't be loaded (the decl is simply omitted,
# like the disk phase's per-fstype gating).
#
# POSIX sh (dash / busybox ash / bash). No bashisms, no `local`.

set -eu

DISTRO="${KSCORE_DISTRO:-unknown}"
PKG_MGR="${KSCORE_PKG_MGR:?KSCORE_PKG_MGR required}"
HARNESS="${KSCORE_HARNESS:-/usr/local/bin/kscore-state-harness}"
ROOT=/var/tmp/kscore-smoke/netdev
mkdir -p "$ROOT"

log() { echo "==> [${DISTRO}] netdev: $*"; }
have() { command -v "$1" >/dev/null 2>&1; }

# Interfaces and rendered files created here, for teardown. All best
# effort — the container's netns and filesystem are torn down anyway, so
# this is hygiene, not load-bearing (no host-global state, unlike lvm).
IFACES=""
cleanup() {
  for i in $IFACES; do ip link del "$i" >/dev/null 2>&1 || true; done
  rm -f /etc/systemd/network/10-kscore-* 2>/dev/null || true
  rm -rf /etc/systemd/network/ksb*.network.d /etc/systemd/network/ksv*.network.d 2>/dev/null || true
  rm -rf "$ROOT" 2>/dev/null || true
}
trap cleanup EXIT

# --- install a JSON-capable iproute2 (best effort) --------------------
case "$PKG_MGR" in
  apt)    DEBIAN_FRONTEND=noninteractive apt-get install -y -qq iproute2 >/dev/null 2>&1 || true ;;
  dnf)    dnf install -y -q iproute >/dev/null 2>&1 || true ;;
  apk)    apk add --no-cache iproute2 >/dev/null 2>&1 || true ;;
  zypper) zypper --non-interactive install iproute2 >/dev/null 2>&1 || true ;;
  pacman) pacman -S --noconfirm --needed iproute2 >/dev/null 2>&1 || true ;;
  *) log "unknown package manager ${PKG_MGR}" ;;
esac

# --- gate: `ip` present and JSON-capable ------------------------------
# Alpine's BusyBox `ip` exists but does not support `-j`; the providers
# parse `ip -d -j link show`, so a JSON-incapable ip is unusable.
if ! have ip || ! ip -j link show lo >/dev/null 2>&1; then
  log "no JSON-capable iproute2 on ${DISTRO} — skipping netdev phase"
  exit 0
fi

# --- prerequisite gate: the `dummy` module (members/parent) -----------
if ip link add ksprobe0 type dummy >/dev/null 2>&1; then
  ip link del ksprobe0 >/dev/null 2>&1 || true
else
  log "dummy interfaces unavailable (no dummy kernel module?) — skipping netdev phase"
  exit 0
fi

# mk_dummy <name> — create a dummy scaffold interface, record for teardown.
mk_dummy() {
  ip link add "$1" type dummy >/dev/null 2>&1 || return 1
  IFACES="$IFACES $1"
}

# type_ok <ip-link-add-args…> — can this interface type be created here?
# Creates then deletes a throwaway interface named ksprobe1.
type_ok() {
  if ip link add ksprobe1 "$@" >/dev/null 2>&1; then
    ip link del ksprobe1 >/dev/null 2>&1 || true
    return 0
  fi
  return 1
}

# Build the fixture from the interface types whose kernel module loads.
FIX="$ROOT/smoke.netdev.yaml"
{
  echo "metadata:"
  echo "  name: kscore-cross-distro-smoke-netdev"
  echo "  version: \"0.1\""
} >"$FIX"
ANY=0

# --- bond -------------------------------------------------------------
if type_ok type bond; then
  if mk_dummy ksbm0; then
    IFACES="$IFACES ksbond0"
    {
      echo "bond:"
      echo "  ksbond0:"
      echo "    state: present"
      echo "    name: ksbond0"
      echo "    mode: active-backup"
      echo "    members:"
      echo "      - ksbm0"
      echo "    miimon: 100"
      echo "    persist: networkd"
    } >>"$FIX"
    ANY=1
    log "bond: enabled (member ksbm0, persist networkd)"
  fi
else
  log "bond: bonding module unavailable — skipping"
fi

# --- bridge -----------------------------------------------------------
if type_ok type bridge; then
  if mk_dummy ksbp0; then
    IFACES="$IFACES ksbr0"
    {
      echo "bridge:"
      echo "  ksbr0:"
      echo "    state: present"
      echo "    name: ksbr0"
      echo "    members:"
      echo "      - ksbp0"
      echo "    stp: true"
      echo "    persist: networkd"
    } >>"$FIX"
    ANY=1
    log "bridge: enabled (port ksbp0, stp, persist networkd)"
  fi
else
  log "bridge: bridge module unavailable — skipping"
fi

# --- vlan -------------------------------------------------------------
# vlan needs a parent to ride on; build it first, then probe on it.
if mk_dummy ksvp0 && type_ok link ksvp0 type vlan id 11; then
  IFACES="$IFACES ksvl0"
  {
    echo "vlan:"
    echo "  ksvl0:"
    echo "    state: present"
    echo "    name: ksvl0"
    echo "    parent: ksvp0"
    echo "    id: 10"
    echo "    persist: networkd"
  } >>"$FIX"
  ANY=1
  log "vlan: enabled (parent ksvp0, id 10, persist networkd)"
else
  log "vlan: 8021q module unavailable — skipping"
fi

if [ "$ANY" -eq 0 ]; then
  log "no virtual-interface types available — nothing to test"
  exit 0
fi

log "applying netdev fixture (apply + idempotency re-apply)"
"$HARNESS" "$FIX"
log "OK"
