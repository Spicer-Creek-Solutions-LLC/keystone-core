#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0

#
# scripts/repo/smoke.sh — install-test a built package-repository tree.
#
# Serves the repo tree over a throwaway local HTTP server and installs
# kscore-cli from it in fresh debian:12-slim (apt) and rockylinux:9 (dnf)
# containers — the repo-side analogue of release-smoke.sh's package
# install checks. When the tree carries a signing pubkey
# (keystone-core-archive-keyring.asc), signature verification is
# exercised end-to-end (apt signed-by + dnf repo_gpgcheck=1).
#
# Usage:
#   scripts/repo/smoke.sh <repo-tree-dir> [channel]
#
# Host requirements: docker, python3.
#
# Exit codes:
#   0  apt + dnf install smoke passed
#   1  bad invocation / missing tool
#   3  an install smoke assertion failed
#

set -euo pipefail

tree=${1:?usage: smoke.sh <repo-tree-dir> [channel]}
channel=${2:-stable}
port=${REPO_SMOKE_PORT:-8973}

[ -d "$tree" ] || { echo "repo-smoke: $tree does not exist" >&2; exit 1; }
tree=$(cd "$tree" && pwd)
command -v docker  >/dev/null 2>&1 || { echo "repo-smoke: docker required" >&2; exit 1; }
command -v python3 >/dev/null 2>&1 || { echo "repo-smoke: python3 required" >&2; exit 1; }

keyring=""
[ -f "$tree/keystone-core-archive-keyring.asc" ] && keyring=/repo/keystone-core-archive-keyring.asc
[ -n "$keyring" ] || echo "repo-smoke: WARNING — no signing pubkey in tree; testing with signature checks DISABLED" >&2

# ---- Serve the tree on localhost -----------------------------------------
python3 -m http.server "$port" --bind 127.0.0.1 --directory "$tree" >/dev/null 2>&1 &
http_pid=$!
cleanup() { kill "$http_pid" 2>/dev/null || true; }
trap cleanup EXIT
# Wait for the server to accept connections.
for _ in $(seq 1 50); do
  (exec 3<>/dev/tcp/127.0.0.1/"$port") 2>/dev/null && { exec 3>&- 3<&-; break; }
  sleep 0.2
done

base="http://127.0.0.1:$port"
fail=0

# ---- APT (debian:12-slim) ------------------------------------------------
echo "== repo-smoke: apt install in debian:12-slim =="
if [ -n "$keyring" ]; then
  apt_src="deb [signed-by=/etc/apt/keyrings/keystone-core.gpg] $base/apt $channel main"
  apt_key_setup="install -d /etc/apt/keyrings && gpg --dearmor < $keyring > /etc/apt/keyrings/keystone-core.gpg"
else
  apt_src="deb [trusted=yes] $base/apt $channel main"
  apt_key_setup="true"
fi
if docker run --rm --network host -v "$tree":/repo:ro debian:12-slim bash -c "
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
echo "== repo-smoke: dnf install in rockylinux:9 =="
if [ -n "$keyring" ]; then
  dnf_gpg="repo_gpgcheck=1
gpgkey=file://$keyring"
else
  dnf_gpg="repo_gpgcheck=0"
fi
if docker run --rm --network host -v "$tree":/repo:ro rockylinux:9 bash -c "
  set -e
  cat > /etc/yum.repos.d/keystone-core.repo <<EOF
[keystone-core]
name=Keystone Core
baseurl=$base/rpm/$channel/\\\$basearch
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
