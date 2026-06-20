#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0

#
# scripts/repo/build.sh — build signed APT + DNF package repositories.
#
# Takes the .deb / .rpm artifacts produced by `make release-snapshot`
# (or `make release`) and lays out a host-agnostic static tree that can
# be served from any web server — the production target is
# https://repos.keystone-core.io, self-hosted and published via scp
# (see deploy/repos/README.md).
#
# Output layout (under --out):
#   apt/
#     dists/<channel>/{Release,Release.gpg,InRelease,main/binary-*/Packages[.gz]}
#     pool/main/<all .deb>
#   rpm/<channel>/{x86_64,aarch64}/{<.rpm>,repodata/{repomd.xml,repomd.xml.asc,...}}
#   keystone-core-archive-keyring.asc   (the signing public key)
#
# Usage:
#   scripts/repo/build.sh --packages dist/ --out dist/repos/ [--channel stable] --sign <mode>
#
# Signing modes (--sign):
#   key:<keyid>   sign with an existing gpg secret key (the real flow)
#   test          generate a throwaway ephemeral key (local validation
#                 only — writes a TESTKEY-DO-NOT-PUBLISH marker)
#   skip          do not sign (dev only; apt/dnf clients will reject it
#                 unless they disable signature checks)
#
# Host requirements:
#   gpg (signing runs on the host so the key never enters a container).
#   The Debian/rpm index tools (apt-ftparchive, dpkg-scanpackages,
#   createrepo_c) run natively when present, else in debian:12-slim /
#   rockylinux:9 via docker or podman — so a macOS release host only
#   needs gpg + a container engine. Honors CONTAINER_ENGINE; set
#   KSCORE_REPO_CONTAINER=1 to force the container path on Linux too.
#
# Exit codes:
#   0  repository built
#   1  bad invocation / missing host tool
#   2  build step failed
#

set -euo pipefail

here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)

packages=dist/
out=dist/repos/
channel=stable
sign=skip

while [ $# -gt 0 ]; do
  case "$1" in
    --packages) packages=$2; shift 2 ;;
    --out)      out=$2; shift 2 ;;
    --channel)  channel=$2; shift 2 ;;
    --sign)     sign=$2; shift 2 ;;
    -h|--help)  sed -n '2,50p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "repo-build: unknown argument: $1" >&2; exit 1 ;;
  esac
done

die() { echo "repo-build: $*" >&2; exit "${2:-2}"; }
need() { command -v "$1" >/dev/null 2>&1 || die "missing required tool: $1 ($2)" 1; }

# gpg signs on the host so the key never enters a container. The Debian/
# rpm index tools are resolved per build-apt.sh / build-dnf.sh (native or
# containerised), so they are NOT required on the host — that is what
# lets a macOS release host get by with gpg + a container engine.
need gpg "GnuPG"
if ! command -v apt-ftparchive >/dev/null 2>&1 || ! command -v createrepo_c >/dev/null 2>&1 \
   || [ "${KSCORE_REPO_CONTAINER:-0}" = "1" ]; then
  command -v docker >/dev/null 2>&1 || command -v podman >/dev/null 2>&1 \
    || command -v "${CONTAINER_ENGINE:-}" >/dev/null 2>&1 \
    || die "host lacks the native index tools; docker or podman is required (or set CONTAINER_ENGINE)" 1
fi

[ -d "$packages" ] || die "packages dir does not exist: $packages" 1
packages=$(cd "$packages" && pwd)
shopt -s nullglob
debs=("$packages"/*.deb)
rpms=("$packages"/*.rpm)
shopt -u nullglob
[ ${#debs[@]} -gt 0 ] || die "no .deb files in $packages (run 'make release-snapshot' first)" 1
[ ${#rpms[@]} -gt 0 ] || die "no .rpm files in $packages (run 'make release-snapshot' first)" 1

mkdir -p "$out"
out=$(cd "$out" && pwd)

# ---- Resolve the signing key ---------------------------------------------
# REPO_GPG_KEY empty => sub-scripts skip signing. GNUPGHOME is exported so
# the ephemeral test key is isolated and the real flow honours the
# operator's keyring.
export REPO_GPG_KEY=""
testkey_home=""
cleanup() { [ -n "$testkey_home" ] && rm -rf "$testkey_home"; }
trap cleanup EXIT

case "$sign" in
  key:*)
    REPO_GPG_KEY=${sign#key:}
    [ -n "$REPO_GPG_KEY" ] || die "--sign key: requires a key id" 1
    gpg --list-secret-keys "$REPO_GPG_KEY" >/dev/null 2>&1 \
      || die "gpg secret key not found: $REPO_GPG_KEY" 1
    echo "repo-build: signing with gpg key $REPO_GPG_KEY"
    ;;
  test)
    testkey_home=$(mktemp -d)
    chmod 700 "$testkey_home"
    export GNUPGHOME="$testkey_home"
    cat > "$testkey_home/keyspec" <<'SPEC'
%no-protection
Key-Type: RSA
Key-Length: 3072
Key-Usage: sign
Name-Real: Keystone Core Repo TEST Key
Name-Email: repo-test@keystone-core.io
Expire-Date: 0
%commit
SPEC
    gpg --batch --quiet --gen-key "$testkey_home/keyspec" 2>/dev/null \
      || die "failed to generate ephemeral test key" 2
    REPO_GPG_KEY=$(gpg --list-secret-keys --with-colons | awk -F: '/^sec/{print $5; exit}')
    echo "TESTKEY-DO-NOT-PUBLISH (built $(git -C "$here" rev-parse --short HEAD 2>/dev/null || echo unknown)): \
this tree was signed with a throwaway local key for smoke-testing only." > "$out/TESTKEY-DO-NOT-PUBLISH"
    echo "repo-build: signing with EPHEMERAL test key $REPO_GPG_KEY (DO NOT PUBLISH)"
    ;;
  skip)
    echo "repo-build: WARNING — building UNSIGNED (--sign skip); clients will reject this repo" >&2
    ;;
  *) die "unknown --sign mode: $sign (want key:<id> | test | skip)" 1 ;;
esac

# ---- Build both repositories ---------------------------------------------
echo "repo-build: building APT repository (channel: $channel)"
"$here/build-apt.sh" "$packages" "$out" "$channel"

echo "repo-build: building DNF repository (channel: $channel)"
"$here/build-dnf.sh" "$packages" "$out" "$channel"

# ---- Export the signing public key into the tree -------------------------
if [ -n "$REPO_GPG_KEY" ]; then
  gpg --armor --export "$REPO_GPG_KEY" > "$out/keystone-core-archive-keyring.asc"
  echo "repo-build: exported signing pubkey -> keystone-core-archive-keyring.asc"
fi

echo ""
echo "repo-build: ok — repository tree at $out"
echo "  apt: $out/apt/dists/$channel"
echo "  rpm: $out/rpm/$channel"
[ "$sign" = test ] && echo "  NOTE: test-key signed — see TESTKEY-DO-NOT-PUBLISH; do not scp this tree."
exit 0
