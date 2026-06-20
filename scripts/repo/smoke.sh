#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0

#
# scripts/repo/smoke.sh — install-test a built package-repository tree.
#
# Mounts the repo tree into fresh debian:12-slim (apt) and rockylinux:9
# (dnf) containers and installs kscore-cli from it via file:// — the
# repo-side analogue of release-smoke.sh's package install checks. When
# the tree carries a signing pubkey (keystone-core-archive-keyring.asc),
# signature verification is exercised end-to-end (apt signed-by + dnf
# repo_gpgcheck=1).
#
# file:// (not a host HTTP server) is deliberate: it works identically on
# Linux and macOS, where docker/podman run a Linux VM and a host-side
# server is not reachable as localhost from inside a container.
#
# Usage:
#   scripts/repo/smoke.sh <repo-tree-dir> [channel]
#
# Host requirements: docker or podman (honors CONTAINER_ENGINE).
#
# Exit codes:
#   0  apt + dnf install smoke passed
#   1  bad invocation / missing tool
#   3  an install smoke assertion failed
#

set -euo pipefail

here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=scripts/repo/lib.sh
. "$here/lib.sh"

tree=${1:?usage: smoke.sh <repo-tree-dir> [channel]}
channel=${2:-stable}

[ -d "$tree" ] || { echo "repo-smoke: $tree does not exist" >&2; exit 1; }
tree=$(cd "$tree" && pwd)
engine=$(container_engine) || { echo "repo-smoke: docker or podman required" >&2; exit 1; }

keyring=""
[ -f "$tree/keystone-core-archive-keyring.asc" ] && keyring=/repo/keystone-core-archive-keyring.asc
[ -n "$keyring" ] || echo "repo-smoke: WARNING — no signing pubkey in tree; testing with signature checks DISABLED" >&2

fail=0

# ---- APT (debian:12-slim) ------------------------------------------------
echo "== repo-smoke: apt install in debian:12-slim (via $engine) =="
if [ -n "$keyring" ]; then
  apt_src="deb [signed-by=/etc/apt/keyrings/keystone-core.gpg] file:/repo/apt $channel main"
  apt_key_setup="install -d /etc/apt/keyrings && gpg --dearmor < $keyring > /etc/apt/keyrings/keystone-core.gpg"
else
  apt_src="deb [trusted=yes] file:/repo/apt $channel main"
  apt_key_setup="true"
fi
if "$engine" run --rm -v "$tree":/repo:ro debian:12-slim bash -c "
  set -e
  export DEBIAN_FRONTEND=noninteractive
  apt-get update -qq
  apt-get install -y -qq gnupg >/dev/null
  $apt_key_setup
  echo '$apt_src' > /etc/apt/sources.list.d/keystone-core.list
  apt-get update -qq
  apt-get install -y -qq kscore-cli >/dev/null
  dpkg-query -W -f='\${Status}\n' kscore-cli | grep -q 'install ok installed'
  echo 'apt: kscore-cli installed ok'
"; then echo "  apt smoke: PASS"; else echo "  apt smoke: FAIL" >&2; fail=1; fi

# ---- DNF (rockylinux:9) --------------------------------------------------
echo "== repo-smoke: dnf install in rockylinux:9 (via $engine) =="
if [ -n "$keyring" ]; then
  dnf_gpg="repo_gpgcheck=1
gpgkey=file://$keyring"
else
  dnf_gpg="repo_gpgcheck=0"
fi
if "$engine" run --rm -v "$tree":/repo:ro rockylinux:9 bash -c "
  set -e
  cat > /etc/yum.repos.d/keystone-core.repo <<EOF
[keystone-core]
name=Keystone Core
baseurl=file:///repo/rpm/$channel/\\\$basearch
enabled=1
gpgcheck=0
$dnf_gpg
EOF
  dnf -q -y install kscore-cli >/dev/null
  rpm -q kscore-cli
  echo 'dnf: kscore-cli installed ok'
"; then echo "  dnf smoke: PASS"; else echo "  dnf smoke: FAIL" >&2; fail=1; fi

echo ""
if [ "$fail" -eq 0 ]; then echo "repo-smoke: ok"; exit 0; else echo "repo-smoke: FAILED" >&2; exit 3; fi
