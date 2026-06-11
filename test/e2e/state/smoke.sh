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
  zypper) zypper --non-interactive refresh >/dev/null 2>&1 ;;
  pacman) pacman -Sy --noconfirm >/dev/null 2>&1 ;;
  *) echo "==> [${DISTRO}] unknown package manager ${PKG_MGR}" >&2; exit 2 ;;
esac

case "$INIT" in
  systemd)  FIXTURE=/src/test/e2e/state/smoke.systemd.yaml ;;
  openrc)   FIXTURE=/src/test/e2e/state/smoke.openrc.yaml ;;
  sysvinit) FIXTURE=/src/test/e2e/state/smoke.sysvinit.yaml ;;
  *) echo "==> [${DISTRO}] unknown init ${INIT}" >&2; exit 2 ;;
esac

# Docker bind-mounts /etc/hostname into the container (it manages the
# container hostname), so the inode can't be replaced — hostnamectl and
# the module's rename-based fallback both fail with EBUSY. Detach the
# bind-mount so /etc/hostname is an ordinary file the `hostname` module
# can set; the running (UTS-namespace) hostname is unaffected. On a host
# without the bind-mount this is a harmless no-op.
umount /etc/hostname 2>/dev/null || true

mkdir -p "$ROOT"
sed -e "s|\${ROOT}|${ROOT}|g" -e "s|\${PKG}|${PKG}|g" -e "s|\${SVC}|${SVC}|g" \
  "$FIXTURE" > "$ROOT/smoke.yaml"

echo "==> [${DISTRO}] applying $(basename "$FIXTURE") (apply + idempotency re-apply)"
"$HARNESS" "$ROOT/smoke.yaml"

# Disk phase: the `disk` module needs real block devices, so it runs in
# its own loop-device-backed script (which self-gates on loop-device +
# per-fstype tool availability). Its failure fails the smoke.
export KSCORE_HARNESS="$HARNESS"
sh /src/test/e2e/state/smoke.disk.sh

# Firewall phase: the `firewall` abstraction drives a live backend
# (iptables, net-namespaced to this container), so it runs in its own
# script that installs the tool and self-gates if none is available. Its
# failure fails the smoke.
sh /src/test/e2e/state/smoke.firewall.sh

# LVM phase: the `lvm` module needs real block devices + device-mapper,
# so it runs in its own loop-device-backed script (self-gating on lvm2 +
# loop availability). Its failure fails the smoke.
sh /src/test/e2e/state/smoke.lvm.sh

# Netdev phase: the `bond`/`bridge`/`vlan` modules create virtual
# interfaces (over dummy scaffolds) in the container's own network
# namespace, so they run in their own script that installs iproute2 and
# self-gates per interface type. Its failure fails the smoke.
sh /src/test/e2e/state/smoke.netdev.sh

# Mount phase: the `mount` module manages a filesystem's fstab entry +
# live mount state in the container's own mount namespace (loop-backed).
sh /src/test/e2e/state/smoke.mount.sh

# Scheduled-task phase: cron + at + systemd_timer, each self-gating on
# its tooling (crontab / atd / a booted systemd).
sh /src/test/e2e/state/smoke.sched.sh

# System-config phase: timezone (timedatectl / symlink) + sysctl (a
# per-netns net.* key + its persist drop-in).
sh /src/test/e2e/state/smoke.sysconf.sh

echo "==> [${DISTRO}] OK"
