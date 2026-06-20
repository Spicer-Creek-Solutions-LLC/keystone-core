#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0

#
# scripts/repo/build-apt.sh — generate a signed APT repository.
#
# Invoked by build.sh (which resolves the signing key); also runnable
# standalone. Produces a flat-pool dists/<channel> APT repo from the
# .deb files in <packages-dir>.
#
# Usage:
#   scripts/repo/build-apt.sh <packages-dir> <out-dir> <channel>
#
# The index/Release generation needs Debian tools (apt-ftparchive,
# dpkg-scanpackages). When they are absent on the host — e.g. macOS, the
# release-from-a-Mac case — it runs in a debian:12-slim container via
# docker/podman (set KSCORE_REPO_CONTAINER=1 to force the container path
# even on Linux). Signing always runs on the host so the GPG key never
# enters a container.
#
# Signing: if REPO_GPG_KEY is set in the environment, the Release file is
# signed (InRelease + Release.gpg); otherwise it is left unsigned.
#
# The repo's trust chain is: signed InRelease -> per-arch Packages
# checksums -> per-.deb checksums. APT does not use per-package GPG
# signatures; the Release signature secures the whole repository.
#

set -euo pipefail

here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=scripts/repo/lib.sh
. "$here/lib.sh"

packages=${1:?usage: build-apt.sh <packages-dir> <out-dir> <channel>}
out=${2:?missing out-dir}
channel=${3:?missing channel}

ARCHES="amd64 arm64"
COMPONENT=main
ORIGIN="Keystone Core"

aptroot="$out/apt"
pool="$aptroot/pool/$COMPONENT"
distdir="$aptroot/dists/$channel"

rm -rf "$aptroot"
mkdir -p "$pool"
for arch in $ARCHES; do
  mkdir -p "$distdir/$COMPONENT/binary-$arch"
done

cp "$packages"/*.deb "$pool/"

# The metadata-generation steps, parameterised entirely by environment so
# the identical script runs natively or inside the container. Paths are
# relative to APTROOT so a client's base URL + Filename resolves the .deb.
read -r -d '' apt_meta <<'SCRIPT' || true
set -euo pipefail
cd "$APTROOT"
for arch in $ARCHES; do
  idx="dists/$CHANNEL/$COMPONENT/binary-$arch/Packages"
  dpkg-scanpackages --arch "$arch" "pool/$COMPONENT" > "$idx" 2>/dev/null
  gzip -9c "$idx" > "$idx.gz"
done
apt-ftparchive \
  -o "APT::FTPArchive::Release::Origin=$ORIGIN" \
  -o "APT::FTPArchive::Release::Label=$ORIGIN" \
  -o "APT::FTPArchive::Release::Suite=$CHANNEL" \
  -o "APT::FTPArchive::Release::Codename=$CHANNEL" \
  -o "APT::FTPArchive::Release::Components=$COMPONENT" \
  -o "APT::FTPArchive::Release::Architectures=$ARCHES" \
  -o "APT::FTPArchive::Release::Description=Keystone Core APT repository" \
  release "dists/$CHANNEL" > "dists/$CHANNEL/Release"
SCRIPT

if want_container apt-ftparchive; then
  engine=$(container_engine) || { echo "build-apt: need apt-ftparchive on host, or docker/podman" >&2; exit 1; }
  echo "build-apt: generating metadata in debian:12-slim via $engine"
  "$engine" run --rm \
    -e APTROOT=/apt -e ARCHES="$ARCHES" -e CHANNEL="$channel" \
    -e COMPONENT="$COMPONENT" -e ORIGIN="$ORIGIN" \
    -v "$aptroot":/apt debian:12-slim bash -c \
    "export DEBIAN_FRONTEND=noninteractive; apt-get update -qq && apt-get install -y -qq apt-utils dpkg-dev >/dev/null && $apt_meta" \
    || { echo "build-apt: container metadata generation failed" >&2; exit 2; }
  # Re-own container-written files so host signing / clean / scp work.
  "$engine" run --rm -v "$aptroot":/apt debian:12-slim chown -R "$(id -u):$(id -g)" /apt
else
  APTROOT="$aptroot" ARCHES="$ARCHES" CHANNEL="$channel" \
    COMPONENT="$COMPONENT" ORIGIN="$ORIGIN" bash -c "$apt_meta"
fi

# Signing always runs on the host (gpg is available on Linux + macOS).
if [ -n "${REPO_GPG_KEY:-}" ]; then
  rm -f "$distdir/InRelease" "$distdir/Release.gpg"
  gpg --batch --yes --default-key "$REPO_GPG_KEY" \
    --clearsign --output "$distdir/InRelease" "$distdir/Release"
  gpg --batch --yes --default-key "$REPO_GPG_KEY" \
    --detach-sign --armor --output "$distdir/Release.gpg" "$distdir/Release"
else
  echo "build-apt: WARNING — Release left unsigned (REPO_GPG_KEY unset)" >&2
fi

echo "build-apt: ok ($(ls "$pool"/*.deb | wc -l | tr -d ' ') packages, arches: $ARCHES)"
