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
# phase exercises ALL THREE backends, each as its own harness invocation
# asserting apply + a zero-change re-apply:
#
#   1. iptables  — auto-detected (firewalld not yet running); covers the
#                  detection path, the iptables backend, its dual-stack
#                  (IPv4 + IPv6), and the named-service catalog.
#   2. nftables  — pinned via `backend: nftables`. The abstraction targets
#                  `inet filter input`, which v1.0 does NOT create, so the
#                  phase pre-creates that base chain first.
#   3. firewalld — auto-detected once its daemon is running (so this also
#                  covers the detector's firewalld branch). systemd distros
#                  only — it needs the daemon + dbus.
#
# Ordering matters: firewalld is last because starting it reconfigures
# netfilter; each earlier scenario has already asserted its idempotency by
# then. Everything is net-namespaced (the matrix containers do NOT use
# --network=host), so the rules apply inside the container's own network
# namespace and vanish with it — no host state is touched, no teardown is
# load-bearing.
#
# Self-gating: each scenario skips cleanly when its tooling is missing
# (no iptables / no nft / not a booted-systemd host or firewalld won't
# come up), rather than failing the matrix.
#
# POSIX sh (dash / busybox ash / bash). No bashisms, no `local`.

set -eu

DISTRO="${KSCORE_DISTRO:-unknown}"
PKG_MGR="${KSCORE_PKG_MGR:?KSCORE_PKG_MGR required}"
INIT="${KSCORE_INIT:-unknown}"
HARNESS="${KSCORE_HARNESS:-/usr/local/bin/kscore-state-harness}"
ROOT=/var/tmp/kscore-smoke/firewall
mkdir -p "$ROOT"

log() { echo "==> [${DISTRO}] firewall: $*"; }
have() { command -v "$1" >/dev/null 2>&1; }

# apply_scenario <label> <fixture-file> — run the harness on one fixture.
apply_scenario() {
  log "$1: applying (apply + idempotency re-apply)"
  "$HARNESS" "$2"
}

# =====================================================================
# Scenario 1 — iptables (auto-detected)
# =====================================================================
case "$PKG_MGR" in
  apt)    DEBIAN_FRONTEND=noninteractive apt-get install -y -qq iptables >/dev/null 2>&1 || true ;;
  dnf)    dnf install -y -q iptables >/dev/null 2>&1 || true ;;
  apk)    apk add --no-cache iptables ip6tables >/dev/null 2>&1 || true ;;
  zypper) zypper --non-interactive install iptables >/dev/null 2>&1 || true ;;
  pacman) pacman -S --noconfirm --needed iptables-nft >/dev/null 2>&1 || true ;;
  *) log "unknown package manager ${PKG_MGR}" ;;
esac

if have iptables; then
  # IPv6 capability probe: the iptables backend is dual-stack when
  # ip6tables is present. A *legacy* ip6tables (vs iptables-nft) needs the
  # kernel's ip6table_filter module loaded; a container often can't load
  # it (the host kernel's .ko files aren't in the container's
  # /lib/modules), so the IPv6 filter table fails to initialize. When the
  # probe shows it's unusable, neutralize ip6tables so the backend takes
  # its documented IPv4-only graceful skip rather than hard-failing.
  if have ip6tables && ! ip6tables -t filter -L INPUT >/dev/null 2>&1; then
    ip6bin=$(command -v ip6tables)
    log "iptables: IPv6 filter table unusable here — disabling ip6tables; running IPv4-only via the backend's graceful skip"
    mv "$ip6bin" "${ip6bin}.smoke-disabled" 2>/dev/null || true
  fi
  FIX_IPT="$ROOT/ipt.yaml"
  {
    echo "metadata:"
    echo "  name: kscore-cross-distro-smoke-firewall-iptables"
    echo "  version: \"0.1\""
    echo "firewall:"
    echo "  allow-ssh:"
    echo "    state: present"
    echo "    service: ssh"
    echo "  allow-https:"
    echo "    state: present"
    echo "    port: \"443/tcp\""
  } >"$FIX_IPT"
  log "iptables: backend auto-detected (service ssh + port 443/tcp)"
  apply_scenario iptables "$FIX_IPT"
else
  log "iptables: unavailable — skipping iptables scenario"
fi

# =====================================================================
# Scenario 2 — nftables (pinned via backend: nftables)
# =====================================================================
case "$PKG_MGR" in
  apt)    DEBIAN_FRONTEND=noninteractive apt-get install -y -qq nftables >/dev/null 2>&1 || true ;;
  dnf)    dnf install -y -q nftables >/dev/null 2>&1 || true ;;
  apk)    apk add --no-cache nftables >/dev/null 2>&1 || true ;;
  zypper) zypper --non-interactive install nftables >/dev/null 2>&1 || true ;;
  pacman) pacman -S --noconfirm --needed nftables >/dev/null 2>&1 || true ;;
esac

if have nft; then
  # The abstraction's nftables sub-decl targets `inet filter input`; v1.0
  # requires it to already exist (chain creation is a V1X item), so
  # pre-create the base chain — the realistic "firewall framework already
  # set up its standard inbound chain" precondition.
  nft add table inet filter 2>/dev/null || true
  nft add chain inet filter input "{ type filter hook input priority 0 ; policy accept ; }" 2>/dev/null || true
  if nft list chain inet filter input >/dev/null 2>&1; then
    FIX_NFT="$ROOT/nft.yaml"
    {
      echo "metadata:"
      echo "  name: kscore-cross-distro-smoke-firewall-nftables"
      echo "  version: \"0.1\""
      echo "firewall:"
      echo "  allow-ssh-nft:"
      echo "    state: present"
      echo "    backend: nftables"
      echo "    service: ssh"
    } >"$FIX_NFT"
    log "nftables: backend pinned (inet filter input pre-created, service ssh)"
    apply_scenario nftables "$FIX_NFT"
  else
    log "nftables: could not create inet filter input chain — skipping nftables scenario"
  fi
else
  log "nftables: nft unavailable — skipping nftables scenario"
fi

# =====================================================================
# Scenario 3 — firewalld (auto-detected; systemd distros only)
# =====================================================================
if [ "$INIT" = systemd ] && have systemctl; then
  case "$PKG_MGR" in
    apt)    DEBIAN_FRONTEND=noninteractive apt-get install -y -qq firewalld >/dev/null 2>&1 || true ;;
    dnf)    dnf install -y -q firewalld >/dev/null 2>&1 || true ;;
    zypper) zypper --non-interactive install firewalld >/dev/null 2>&1 || true ;;
    pacman) pacman -S --noconfirm --needed firewalld >/dev/null 2>&1 || true ;;
  esac
  if have firewall-cmd; then
    systemctl unmask firewalld >/dev/null 2>&1 || true
    systemctl start firewalld >/dev/null 2>&1 || true
    # Poll until the daemon reports running (it needs dbus + a netfilter
    # backend; container startup can lag or fail).
    _i=0
    while [ "$_i" -lt 20 ]; do
      if firewall-cmd --state 2>/dev/null | grep -q running; then break; fi
      sleep 1
      _i=$((_i + 1))
    done
    if firewall-cmd --state 2>/dev/null | grep -q running; then
      FIX_FWD="$ROOT/fwd.yaml"
      {
        echo "metadata:"
        echo "  name: kscore-cross-distro-smoke-firewall-firewalld"
        echo "  version: \"0.1\""
        echo "firewall:"
        echo "  allow-ssh-fwd:"
        echo "    state: present"
        echo "    service: ssh"
      } >"$FIX_FWD"
      log "firewalld: daemon running, backend auto-detected (zone public, service ssh)"
      apply_scenario firewalld "$FIX_FWD"
    else
      log "firewalld: daemon did not come up — skipping firewalld scenario"
    fi
  else
    log "firewalld: firewall-cmd unavailable — skipping firewalld scenario"
  fi
else
  log "firewalld: not a booted-systemd host — skipping firewalld scenario"
fi

log "OK"
