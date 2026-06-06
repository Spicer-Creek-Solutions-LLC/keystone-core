#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0

#
# test/e2e/state/smoke.sh — runs inside each cross-distro container
# (invoked via `docker exec` by run.sh, after the container's init
# system is up).
#
# It refreshes the package index, renders the init-appropriate smoke
# fixture with this distro's package/service names substituted, and
# runs the apply-harness — which applies the state twice and asserts
# the second pass is a zero-change no-op (idempotency).
#
# The harness drives the real stdlib modules against the live system
# (package manager + init), so this exercises the cross-distro
# package (apt/dnf/apk) and service (systemd/OpenRC) backends for
# real — not a mock.
#
# Required env (set by run.sh on the `docker exec`):
#   KSCORE_DISTRO   informational label (debian-12, rocky-9, …)
#   KSCORE_PKG_MGR  apt | dnf | apk
#   KSCORE_INIT     systemd | openrc
#   KSCORE_PKG      package to install (cron / cronie / dcron)
#   KSCORE_SVC      its service unit (cron / crond / dcron)
#   KSCORE_HARNESS  path to the mounted static harness binary

# Portable across the matrix's shells: dash (Debian/Ubuntu /bin/sh),
# busybox ash (Alpine), and bash. No pipelines here, so -o pipefail
# (which dash lacks) is unneeded.
set -eu

DISTRO="${KSCORE_DISTRO:-unknown}"
PKG_MGR="${KSCORE_PKG_MGR:?KSCORE_PKG_MGR required}"
INIT="${KSCORE_INIT:?KSCORE_INIT required}"
PKG="${KSCORE_PKG:?KSCORE_PKG required}"
SVC="${KSCORE_SVC:?KSCORE_SVC required}"
HARNESS="${KSCORE_HARNESS:-/usr/local/bin/kscore-state-harness}"
ROOT=/var/tmp/kscore-smoke

echo "==> [${DISTRO}] init=${INIT} pkg=${PKG_MGR} (${PKG} / ${SVC})"

# Refresh the package index so the package module's install can
# resolve a candidate. (The module installs but never updates the
# index — that is deliberately a host/setup concern.)
case "$PKG_MGR" in
  apt) apt-get update -qq >/dev/null 2>&1 ;;
  apk) apk update -q >/dev/null 2>&1 ;;
  dnf) : ;; # dnf fetches repo metadata on demand
  *) echo "==> [${DISTRO}] unknown package manager ${PKG_MGR}" >&2; exit 2 ;;
esac

case "$INIT" in
  systemd) FIXTURE=/src/test/e2e/state/smoke.systemd.yaml ;;
  openrc)  FIXTURE=/src/test/e2e/state/smoke.openrc.yaml ;;
  *) echo "==> [${DISTRO}] unknown init ${INIT}" >&2; exit 2 ;;
esac

mkdir -p "$ROOT"
sed -e "s|\${ROOT}|${ROOT}|g" -e "s|\${PKG}|${PKG}|g" -e "s|\${SVC}|${SVC}|g" \
  "$FIXTURE" > "$ROOT/smoke.yaml"

echo "==> [${DISTRO}] applying $(basename "$FIXTURE") (apply + idempotency re-apply)"
"$HARNESS" "$ROOT/smoke.yaml"

echo "==> [${DISTRO}] OK"
