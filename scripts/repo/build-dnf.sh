#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0

#
# scripts/repo/build-dnf.sh — generate a signed DNF/YUM repository.
#
# Invoked by build.sh (which resolves the signing key); also runnable
# standalone. Produces per-arch repodata from the .rpm files in
# <packages-dir>.
#
# Usage:
#   scripts/repo/build-dnf.sh <packages-dir> <out-dir> <channel>
#
# createrepo_c runs natively if present, otherwise inside a rockylinux:9
# container (the same host-tool fallback release-smoke.sh uses for rpm).
#
# Signing: if REPO_GPG_KEY is set, repodata/repomd.xml is detached-signed
# to repomd.xml.asc (this is what `repo_gpgcheck=1` verifies). The
# trust chain is: signed repomd.xml -> primary.xml.gz checksums ->
# per-.rpm checksums. Per-package RPM header signing (`gpgcheck=1`) is a
# separate hardening step that lands with the release-signing ceremony.
#

set -euo pipefail

here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=scripts/repo/lib.sh
. "$here/lib.sh"

packages=${1:?usage: build-dnf.sh <packages-dir> <out-dir> <channel>}
out=${2:?missing out-dir}
channel=${3:?missing channel}

rpmroot="$out/rpm/$channel"
rm -rf "$rpmroot"
mkdir -p "$rpmroot/x86_64" "$rpmroot/aarch64"

# Route each .rpm to its arch dir. goreleaser names nfpm rpms
# <pkg>_<ver>_linux_<goarch>.rpm; also accept the canonical rpm arch
# tokens in case the naming template changes.
for f in "$packages"/*.rpm; do
  base=$(basename "$f")
  case "$base" in
    *_linux_amd64.rpm|*x86_64*) cp "$f" "$rpmroot/x86_64/" ;;
    *_linux_arm64.rpm|*aarch64*) cp "$f" "$rpmroot/aarch64/" ;;
    *) echo "build-dnf: WARNING — cannot determine arch for $base, skipping" >&2 ;;
  esac
done

run_createrepo() {
  local dir=$1
  if ! want_container createrepo_c; then
    createrepo_c --quiet "$dir"
    return 0
  fi
  local engine
  engine=$(container_engine) || {
    echo "build-dnf: need createrepo_c on host, or docker/podman" >&2; return 1; }
  # Build metadata in a container (the macOS reality — createrepo_c is
  # Linux-only), then chown the output back to the invoking user so
  # `make clean` / scp don't trip over root-owned files.
  echo "build-dnf: generating $(basename "$dir") metadata in rockylinux:9 via $engine"
  "$engine" run --rm -v "$dir":/data rockylinux:9 sh -c \
    'dnf -q -y install createrepo_c >/dev/null 2>&1 && createrepo_c --quiet /data' \
    || { echo "build-dnf: createrepo_c container run failed" >&2; return 2; }
  "$engine" run --rm -v "$dir":/data rockylinux:9 \
    chown -R "$(id -u):$(id -g)" /data
}

for arch in x86_64 aarch64; do
  dir="$rpmroot/$arch"
  # Skip an arch with no packages (don't emit empty repodata).
  if ! ls "$dir"/*.rpm >/dev/null 2>&1; then
    rmdir "$dir" 2>/dev/null || true
    continue
  fi
  run_createrepo "$dir"
  if [ -n "${REPO_GPG_KEY:-}" ]; then
    rm -f "$dir/repodata/repomd.xml.asc"
    gpg --batch --yes --default-key "$REPO_GPG_KEY" \
      --detach-sign --armor --output "$dir/repodata/repomd.xml.asc" \
      "$dir/repodata/repomd.xml"
  else
    echo "build-dnf: WARNING — repomd.xml left unsigned (REPO_GPG_KEY unset)" >&2
  fi
  echo "build-dnf: ok ($arch: $(ls "$dir"/*.rpm | wc -l) packages)"
done
