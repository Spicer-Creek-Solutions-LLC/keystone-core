#!/usr/bin/env sh
# SPDX-License-Identifier: Apache-2.0

#
# test/e2e/state/smoke.firewall.sh — the `firewall`-module phase of the
# cross-distro matrix, run inside each container by smoke.sh after the
# disk phase.
#
# The `firewall` module is the cross-backend abstraction (one
# declaration → "allow this service / port inbound"); it auto-detects a
# backend (firewalld if its daemon is running, else iptables, else nft)
# and drives the concrete backend module against the live system. This
# phase installs `iptables` and applies a small fixture twice, asserting
# the second pass is a zero-change no-op — so the abstraction's
# detection + the iptables backend + its dual-stack (IPv4 + IPv6) and
# named-service catalog are exercised live, not mocked.
#
# Why iptables specifically: it is net-namespaced, so the rules apply
# inside the container's own network namespace (the matrix containers do
# NOT use --network=host) and vanish with the container — no host state
# is touched, and no teardown is load-bearing. firewalld is only picked
# by the detector when its daemon is *running* (`firewall-cmd --state`
# exits 0); this phase never starts it, so installing iptables makes the
# backend deterministically iptables on every distro. nftables-pinned
# and firewalld-daemon coverage are follow-ups (see README.md).
#
# Self-gating: if no backend tool can be installed (so `iptables` is not
# on PATH), it skips cleanly (exit 0) rather than failing the matrix —
# the same discipline as the disk phase's loop-device gate.
#
# POSIX sh (dash / busybox ash / bash). No bashisms, no `local`.

set -eu

DISTRO="${KSCORE_DISTRO:-unknown}"
PKG_MGR="${KSCORE_PKG_MGR:?KSCORE_PKG_MGR required}"
HARNESS="${KSCORE_HARNESS:-/usr/local/bin/kscore-state-harness}"
ROOT=/var/tmp/kscore-smoke/firewall
mkdir -p "$ROOT"

log() { echo "==> [${DISTRO}] firewall: $*"; }
have() { command -v "$1" >/dev/null 2>&1; }

# --- ensure a backend tool (best effort; the gate below handles gaps) -
# Debian/RHEL/SUSE ship ip6tables alongside the iptables package; Alpine
# splits it out; Arch's iptables-nft provides the `iptables` command.
case "$PKG_MGR" in
  apt)    DEBIAN_FRONTEND=noninteractive apt-get install -y -qq iptables >/dev/null 2>&1 || true ;;
  dnf)    dnf install -y -q iptables >/dev/null 2>&1 || true ;;
  apk)    apk add --no-cache iptables ip6tables >/dev/null 2>&1 || true ;;
  zypper) zypper --non-interactive install iptables >/dev/null 2>&1 || true ;;
  pacman) pacman -S --noconfirm --needed iptables-nft >/dev/null 2>&1 || true ;;
  *) log "unknown package manager ${PKG_MGR}" ;;
esac

# --- gate: a firewall backend must be on PATH --------------------------
if ! have iptables && ! have nft && ! have firewall-cmd; then
  log "no firewall backend installable on ${DISTRO} — skipping firewall phase"
  exit 0
fi

# --- IPv6 capability probe (container environment) ---------------------
# The iptables backend is dual-stack: when ip6tables is present it applies
# the IPv6 half too. A *legacy* ip6tables (vs iptables-nft) needs the
# kernel's ip6table_filter module loaded; in a container that module often
# can't be loaded (the host kernel's .ko files aren't in the container's
# /lib/modules), so the IPv6 filter table fails to initialize ("Table does
# not exist"). That makes the container an effectively IPv4-only-iptables
# host. When the probe shows the IPv6 filter table is unusable, neutralize
# ip6tables so the backend takes its documented IPv4-only graceful skip
# (the same path a real IPv4-only host takes) rather than hard-failing the
# dual-stack apply. iptables-nft auto-initializes the table, so this
# leaves the dual-stack distros untouched.
if have ip6tables && ! ip6tables -t filter -L INPUT >/dev/null 2>&1; then
  ip6bin=$(command -v ip6tables)
  log "IPv6 filter table unusable here (no loadable ip6 netfilter module) — disabling ip6tables; firewall phase runs IPv4-only via the backend's graceful skip"
  mv "$ip6bin" "${ip6bin}.smoke-disabled" 2>/dev/null || true
fi

# --- fixture -----------------------------------------------------------
# `service: ssh` resolves through the named-service catalog (→ 22/tcp);
# `port:` exercises the explicit-port path. On the iptables backend each
# becomes one rule per family (IPv4 + IPv6 when ip6tables is present).
FIX="$ROOT/smoke.firewall.yaml"
{
  echo "metadata:"
  echo "  name: kscore-cross-distro-smoke-firewall"
  echo "  version: \"0.1\""
  echo "firewall:"
  echo "  allow-ssh:"
  echo "    state: present"
  echo "    service: ssh"
  echo "  allow-https:"
  echo "    state: present"
  echo "    port: \"443/tcp\""
} >"$FIX"

log "applying firewall fixture (apply + idempotency re-apply)"
"$HARNESS" "$FIX"
log "OK"
