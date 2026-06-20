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
# Signing: if REPO_GPG_KEY is set in the environment, the Release file is
# signed (InRelease + Release.gpg); otherwise it is left unsigned.
#
# The repo's trust chain is: signed InRelease -> per-arch Packages
# checksums -> per-.deb checksums. APT does not use per-package GPG
# signatures; the Release signature secures the whole repository.
#

set -euo pipefail

packages=${1:?usage: build-apt.sh <packages-dir> <out-dir> <channel>}
out=${2:?missing out-dir}
channel=${3:?missing channel}

ARCHES=(amd64 arm64)
COMPONENT=main
ORIGIN="Keystone Core"

aptroot="$out/apt"
pool="$aptroot/pool/$COMPONENT"
distdir="$aptroot/dists/$channel"

rm -rf "$aptroot"
mkdir -p "$pool"
for arch in "${ARCHES[@]}"; do
  mkdir -p "$distdir/$COMPONENT/binary-$arch"
done

cp "$packages"/*.deb "$pool/"

# Per-arch Packages indices. dpkg-scanpackages -a <arch> filters by the
# .deb's own Architecture field (arch + "all"), so the index is correct
# regardless of how the files are named. Paths are written relative to
# the apt root so a client's base URL + Filename resolves the .deb.
(
  cd "$aptroot"
  for arch in "${ARCHES[@]}"; do
    idx="dists/$channel/$COMPONENT/binary-$arch/Packages"
    dpkg-scanpackages --arch "$arch" "pool/$COMPONENT" > "$idx" 2>/dev/null
    gzip -9c "$idx" > "$idx.gz"
  done
)

# Release file: apt-ftparchive walks the dists/<channel> tree and records
# the size + MD5/SHA1/SHA256 of every Packages[.gz] it finds.
apt-ftparchive \
  -o "APT::FTPArchive::Release::Origin=$ORIGIN" \
  -o "APT::FTPArchive::Release::Label=$ORIGIN" \
  -o "APT::FTPArchive::Release::Suite=$channel" \
  -o "APT::FTPArchive::Release::Codename=$channel" \
  -o "APT::FTPArchive::Release::Components=$COMPONENT" \
  -o "APT::FTPArchive::Release::Architectures=${ARCHES[*]}" \
  -o "APT::FTPArchive::Release::Description=Keystone Core APT repository" \
  release "$distdir" > "$distdir/Release"

if [ -n "${REPO_GPG_KEY:-}" ]; then
  rm -f "$distdir/InRelease" "$distdir/Release.gpg"
  gpg --batch --yes --default-key "$REPO_GPG_KEY" \
    --clearsign --output "$distdir/InRelease" "$distdir/Release"
  gpg --batch --yes --default-key "$REPO_GPG_KEY" \
    --detach-sign --armor --output "$distdir/Release.gpg" "$distdir/Release"
else
  echo "build-apt: WARNING — Release left unsigned (REPO_GPG_KEY unset)" >&2
fi

echo "build-apt: ok ($(ls "$pool"/*.deb | wc -l) packages, arches: ${ARCHES[*]})"
