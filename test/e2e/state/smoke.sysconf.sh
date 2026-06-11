#!/usr/bin/env sh
# SPDX-License-Identifier: Apache-2.0

#
# test/e2e/state/smoke.sysconf.sh — the system-config phase of the
# cross-distro matrix (timezone + sysctl), run inside each container by
# smoke.sh after the sched phase.
#
#   * timezone — the system timezone. `timedatectl` is used when present
#                (the systemd distros), otherwise the /etc/localtime
#                symlink + /etc/timezone fallback (Alpine / Devuan) — so
#                this exercises both code paths across the matrix.
#   * sysctl   — a kernel parameter + its /etc/sysctl.d persist drop-in.
#                A `net.*` key is used deliberately: net sysctls are
#                per-network-namespace, so setting it touches only the
#                container's netns, never the host.
#
# The phase installs `tzdata` (for the zoneinfo database), applies a
# fixture twice, and asserts the second pass is a zero-change no-op.
#
# Self-gating: skips the timezone declaration when the zoneinfo database
# can't be installed; skips the whole phase only if neither piece can run.
#
# POSIX sh (dash / busybox ash / bash). No bashisms, no `local`.

set -eu

DISTRO="${KSCORE_DISTRO:-unknown}"
PKG_MGR="${KSCORE_PKG_MGR:?KSCORE_PKG_MGR required}"
HARNESS="${KSCORE_HARNESS:-/usr/local/bin/kscore-state-harness}"
ROOT=/var/tmp/kscore-smoke/sysconf
mkdir -p "$ROOT"

ZONE="America/New_York"
SYSCTL_KEY="net.ipv4.ip_forward"

log() { echo "==> [${DISTRO}] sysconf: $*"; }
have() { command -v "$1" >/dev/null 2>&1; }

cleanup() {
  rm -f /etc/sysctl.d/*kscore* 2>/dev/null || true
  rm -rf "$ROOT" 2>/dev/null || true
}
trap cleanup EXIT

# --- install tzdata (zoneinfo) + procps (the sysctl binary) -----------
case "$PKG_MGR" in
  apt)    DEBIAN_FRONTEND=noninteractive apt-get install -y -qq tzdata procps >/dev/null 2>&1 || true ;;
  dnf)    dnf install -y -q tzdata procps-ng >/dev/null 2>&1 || true ;;
  apk)    apk add --no-cache tzdata procps >/dev/null 2>&1 || true ;;
  zypper) zypper --non-interactive install timezone procps >/dev/null 2>&1 || true ;;
  pacman) pacman -S --noconfirm --needed tzdata procps-ng >/dev/null 2>&1 || true ;;
  *) log "unknown package manager ${PKG_MGR}" ;;
esac

FIX="$ROOT/smoke.sysconf.yaml"
{
  echo "metadata:"
  echo "  name: kscore-cross-distro-smoke-sysconf"
  echo "  version: \"0.1\""
} >"$FIX"
ANY=0

# --- timezone ---------------------------------------------------------
# Declaration.Name IS the desired zone; needs the zoneinfo file present.
if [ -f "/usr/share/zoneinfo/${ZONE}" ]; then
  {
    echo "timezone:"
    echo "  ${ZONE}: {}"
  } >>"$FIX"
  ANY=1
  if have timedatectl; then
    log "timezone: enabled (${ZONE}, timedatectl path)"
  else
    log "timezone: enabled (${ZONE}, /etc/localtime symlink path)"
  fi
else
  log "timezone: zoneinfo for ${ZONE} unavailable — skipping"
fi

# --- sysctl -----------------------------------------------------------
# net.* keys are per-netns, so this is contained to the container.
if have sysctl && [ -e "/proc/sys/net/ipv4/ip_forward" ]; then
  {
    echo "sysctl:"
    echo "  ${SYSCTL_KEY}:"
    echo "    value: \"1\""
    echo "    persist: true"
  } >>"$FIX"
  ANY=1
  log "sysctl: enabled (${SYSCTL_KEY}=1, persist drop-in)"
else
  log "sysctl: unavailable — skipping"
fi

if [ "$ANY" -eq 0 ]; then
  log "no system-config tooling available — nothing to test"
  exit 0
fi

log "applying sysconf fixture (apply + idempotency re-apply)"
"$HARNESS" "$FIX"
log "OK"
