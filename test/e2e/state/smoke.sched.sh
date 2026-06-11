#!/usr/bin/env sh
# SPDX-License-Identifier: Apache-2.0

#
# test/e2e/state/smoke.sched.sh — the scheduled-task phase of the
# cross-distro matrix (cron + at + systemd_timer), run inside each
# container by smoke.sh after the mount phase.
#
#   * cron — a per-user crontab entry (crontab(1)), tagged with a
#            keystone marker so the module owns exactly that line.
#   * at   — a one-shot job in the `at` queue (needs atd running).
#   * systemd_timer — a `.timer` unit under /etc/systemd/system
#            (systemd distros only; needs systemctl + a booted systemd).
#
# All three operate on per-container state (crontab files, the at spool,
# unit files), so they are matrix-safe. The phase builds a fixture from
# whichever pieces are available, applies it twice, and asserts the
# second pass is a zero-change no-op.
#
# Self-gating: each module is included only when its tooling is present —
# crontab for cron, a startable atd for at, and systemctl + a booted
# systemd for systemd_timer. If none are available the phase skips.
#
# POSIX sh (dash / busybox ash / bash). No bashisms, no `local`.

set -eu

DISTRO="${KSCORE_DISTRO:-unknown}"
PKG_MGR="${KSCORE_PKG_MGR:?KSCORE_PKG_MGR required}"
INIT="${KSCORE_INIT:-unknown}"
HARNESS="${KSCORE_HARNESS:-/usr/local/bin/kscore-state-harness}"
ROOT=/var/tmp/kscore-smoke/sched
mkdir -p "$ROOT"

log() { echo "==> [${DISTRO}] sched: $*"; }
have() { command -v "$1" >/dev/null 2>&1; }

DID_CRON=0
DID_AT=0
DID_TIMER=0
cleanup() {
  [ "$DID_CRON" -eq 1 ] && crontab -r >/dev/null 2>&1 || true
  if [ "$DID_AT" -eq 1 ] && have atq; then
    atq 2>/dev/null | awk '{print $1}' | while read -r _j; do atrm "$_j" >/dev/null 2>&1 || true; done
  fi
  if [ "$DID_TIMER" -eq 1 ] && have systemctl; then
    systemctl disable --now kscron.timer >/dev/null 2>&1 || true
    rm -f /etc/systemd/system/kscron.timer
    systemctl daemon-reload >/dev/null 2>&1 || true
  fi
  rm -rf "$ROOT" 2>/dev/null || true
}
trap cleanup EXIT

# --- install cron + at (best effort) ----------------------------------
case "$PKG_MGR" in
  apt)    DEBIAN_FRONTEND=noninteractive apt-get install -y -qq cron at >/dev/null 2>&1 || true ;;
  dnf)    dnf install -y -q cronie at >/dev/null 2>&1 || true ;;
  apk)    apk add --no-cache dcron at >/dev/null 2>&1 || true ;;
  zypper) zypper --non-interactive install cron at >/dev/null 2>&1 || true ;;
  pacman) pacman -S --noconfirm --needed cronie at >/dev/null 2>&1 || true ;;
  *) log "unknown package manager ${PKG_MGR}" ;;
esac

FIX="$ROOT/smoke.sched.yaml"
{
  echo "metadata:"
  echo "  name: kscore-cross-distro-smoke-sched"
  echo "  version: \"0.1\""
} >"$FIX"
ANY=0

# --- cron -------------------------------------------------------------
if have crontab; then
  {
    echo "cron:"
    echo "  ksjob:"
    echo "    state: present"
    echo "    schedule: \"*/5 * * * *\""
    echo "    command: /bin/true"
    echo "    user: root"
  } >>"$FIX"
  DID_CRON=1; ANY=1
  log "cron: enabled (root crontab entry)"
else
  log "cron: crontab unavailable — skipping"
fi

# --- at ---------------------------------------------------------------
# at submission needs atd running. Start it best-effort (no init manager
# is assumed — run the daemon directly).
if have at && have atd; then
  atd >/dev/null 2>&1 || true
  sleep 1
  {
    echo "at:"
    echo "  ksat:"
    echo "    state: present"
    echo "    time: \"now + 1 hour\""
    echo "    command: /bin/true"
  } >>"$FIX"
  DID_AT=1; ANY=1
  log "at: enabled (one-shot queue entry, atd started)"
else
  log "at: at/atd unavailable — skipping"
fi

# --- systemd_timer (systemd distros only) -----------------------------
# Needs systemctl and a booted systemd (daemon-reload + unit state).
if [ "$INIT" = systemd ] && have systemctl && systemctl daemon-reload >/dev/null 2>&1; then
  {
    echo "systemd_timer:"
    echo "  kscron:"
    echo "    state: present"
    echo "    on_calendar: \"*-*-* 03:00:00\""
    echo "    enable: false"
  } >>"$FIX"
  DID_TIMER=1; ANY=1
  log "systemd_timer: enabled (kscron.timer, enable=false)"
else
  log "systemd_timer: not a booted-systemd host — skipping"
fi

if [ "$ANY" -eq 0 ]; then
  log "no scheduled-task tooling available — nothing to test"
  exit 0
fi

log "applying sched fixture (apply + idempotency re-apply)"
"$HARNESS" "$FIX"
log "OK"
