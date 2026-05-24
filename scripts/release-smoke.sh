#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0

#
# scripts/release-smoke.sh — Epic 19 task 13.
#
# Asserts that the artifacts produced by `make release-snapshot` (or
# `make release`) are well-formed. Invoked by `make release-dry-run`
# after the goreleaser snapshot build.
#
# Usage:
#   scripts/release-smoke.sh <dist-dir>
#
# Environment:
#   RELEASE_SMOKE_CONTAINERS=1   Also install one .deb in debian:12-slim
#                                and one .rpm in rockylinux:9 and assert
#                                the installed binary runs. Adds ~2 min
#                                to the smoke run. Default: 0.
#
# Host requirements:
#   sha256sum, tar, unzip, docker.
#   NOT required: rpm, dpkg-deb. The linux-side checks (binary
#   --version, .deb / .rpm content) run inside debian:12-slim via
#   scripts/release-smoke-container.sh — debian has dpkg-deb natively
#   and gets rpm via apt at container start.
#
# Exit codes:
#   0  every assertion passed
#   1  bad invocation (missing dir, missing tool)
#   2  artifact-shape assertion failed
#   3  container-install smoke assertion failed

set -euo pipefail

dist=${1:-}
if [ -z "$dist" ]; then
  echo "usage: $0 <dist-dir>" >&2
  exit 1
fi
if [ ! -d "$dist" ]; then
  echo "release-smoke: $dist does not exist or is not a directory" >&2
  exit 1
fi

# Resolve to an absolute path so docker -v always sees the right dir.
dist=$(cd "$dist" && pwd)

# 20 binaries (every cmd/kscore-* + kscorectl). Source of truth:
# .goreleaser.yaml builds: section. Used here only for the archive
# content check; the in-container script repeats the list for the
# binary --version check.
BINARIES=(
  kscore-server kscore-agent kscorectl
  kscore-audit kscore-backup kscore-blueprint kscore-bootstrap
  kscore-cluster kscore-cluster-backup kscore-events kscore-files
  kscore-gitops kscore-identity kscore-migrate kscore-module
  kscore-policy kscore-registry kscore-runbook kscore-secrets
  kscore-webhook
)

# (goos goarch ext archive-ext) — drives the archive check.
PLATFORMS=(
  "linux  amd64 ''  tar.gz"
  "linux  arm64 ''  tar.gz"
  "darwin amd64 ''  tar.gz"
  "darwin arm64 ''  tar.gz"
  "windows amd64 .exe zip"
)

# Resolved relative to this script so docker -v works regardless of
# the caller's cwd.
script_dir=$(cd "$(dirname "$0")" && pwd)

pass() { printf 'PASS: %s\n' "$*"; }
fail() { printf 'FAIL: %s\n' "$*" >&2; exit 2; }
info() { printf '  %s\n' "$*"; }

# -- 1. checksums.txt -----------------------------------------------------

check_checksums() {
  printf '\n== checksums.txt ==\n'
  local f="$dist/checksums.txt"
  [ -f "$f" ] || fail "checksums.txt missing"
  pass "checksums.txt exists ($(wc -l <"$f") entries)"
  ( cd "$dist" && sha256sum --quiet -c checksums.txt ) \
    || fail "sha256sum -c failed against checksums.txt"
  pass "all listed files verify against their sha256"
}

# -- 2. archive content ---------------------------------------------------

archive_glob() {
  local plat=$1 arch=$2 ext=$3
  ls "$dist"/keystone-core_*_"${plat}_${arch}.${ext}" 2>/dev/null | head -n1
}

check_archives() {
  printf '\n== archives (5 expected) ==\n'
  local plat arch ext archext file
  for spec in "${PLATFORMS[@]}"; do
    # shellcheck disable=SC2086
    set -- $spec
    plat=$1 arch=$2 ext=$3 archext=$4
    [ "$ext" = "''" ] && ext=""
    file=$(archive_glob "$plat" "$arch" "$archext")
    [ -n "$file" ] || fail "archive missing for $plat/$arch ($archext)"
    [ -s "$file" ] || fail "archive $file is empty"

    local listing
    if [ "$archext" = "zip" ]; then
      listing=$(unzip -Z1 "$file")
    else
      listing=$(tar -tzf "$file")
    fi
    local count=0
    for bin in "${BINARIES[@]}"; do
      echo "$listing" | grep -qE "(^|/)${bin}${ext}$" \
        || fail "$file missing binary: ${bin}${ext}"
      count=$((count+1))
    done
    [ "$count" -eq 20 ] || fail "$file: expected 20 binaries, found $count"

    for doc in LICENSE NOTICE README.md CHANGELOG.md; do
      echo "$listing" | grep -qE "(^|/)${doc}$" \
        || fail "$file missing bundled file: $doc"
    done
    pass "$(basename "$file"): 20 binaries + LICENSE/NOTICE/README/CHANGELOG"
  done
}

# -- 3. linux-side checks (binary --version, deb content, rpm content) ----

check_linux_in_container() {
  if command -v docker >/dev/null; then
    printf '\n== linux artifacts (debian:12-slim) ==\n'
    # Mount /dist read-only and /smoke read-only with just the in-container
    # script (one file, not the full scripts/ tree). The container script
    # installs rpm via apt, then runs binary --version + deb/rpm content
    # checks. Output is forwarded; container exit code propagates.
    docker run --rm \
      -v "$dist:/dist:ro" \
      -v "$script_dir/release-smoke-container.sh:/smoke.sh:ro" \
      debian:12-slim \
      bash /smoke.sh /dist \
      || fail "linux-side artifact checks failed (see container output above)"
  else
    printf '\n== linux artifacts (native — no docker available) ==\n'
    # Native fallback for environments without docker (e.g. Forgejo
    # runner image). release-smoke-container.sh self-adapts: dpkg-deb
    # is expected, rpm is installed via apt-get only if missing.
    bash "$script_dir/release-smoke-container.sh" "$dist" \
      || fail "linux-side artifact checks failed (native mode)"
  fi
}

# -- 4. install smoke (opt-in) -------------------------------------------

check_install_smoke() {
  printf '\n== install smoke (host arch) ==\n'
  if [ "${RELEASE_SMOKE_CONTAINERS:-0}" != "1" ]; then
    info "RELEASE_SMOKE_CONTAINERS=0 (skipped — set =1 to enable)"
    return 0
  fi
  if ! command -v docker >/dev/null; then
    info "docker unavailable — skipping install smoke (RELEASE_SMOKE_CONTAINERS=1 ignored)"
    return 0
  fi

  local host_arch
  case "$(uname -m)" in
    x86_64)        host_arch=amd64 ;;
    aarch64|arm64) host_arch=arm64 ;;
    *) info "unsupported host arch $(uname -m) — skipping install smoke"; return 0 ;;
  esac

  local deb rpm
  deb=$(ls "$dist"/kscore-server_*_linux_"${host_arch}".deb | head -n1)
  rpm=$(ls "$dist"/kscore-server_*_linux_"${host_arch}".rpm | head -n1)

  info "debian:12-slim + $(basename "$deb")"
  docker run --rm -v "$dist:/dist:ro" debian:12-slim bash -c '
    set -e
    dpkg -i /dist/'"$(basename "$deb")"' >/dev/null 2>&1
    [ -x /usr/bin/kscore-server ] || { echo "binary not installed at /usr/bin/kscore-server"; exit 1; }
    /usr/bin/kscore-server --version >/dev/null
    [ -f /lib/systemd/system/kscore-server.service ] || { echo "systemd unit not at /lib/systemd/system/kscore-server.service"; exit 1; }
  ' || { echo "FAIL: debian:12-slim deb install" >&2; exit 3; }
  pass "debian:12-slim: dpkg -i + --version + systemd unit present"

  info "rockylinux:9 + $(basename "$rpm")"
  docker run --rm -v "$dist:/dist:ro" rockylinux:9 bash -c '
    set -e
    rpm -i --nodeps /dist/'"$(basename "$rpm")"' >/dev/null 2>&1
    [ -x /usr/bin/kscore-server ] || { echo "binary not installed at /usr/bin/kscore-server"; exit 1; }
    /usr/bin/kscore-server --version >/dev/null
    [ -f /lib/systemd/system/kscore-server.service ] || { echo "systemd unit not at /lib/systemd/system/kscore-server.service"; exit 1; }
  ' || { echo "FAIL: rockylinux:9 rpm install" >&2; exit 3; }
  pass "rockylinux:9: rpm -i + --version + systemd unit present"
}

# -- main -----------------------------------------------------------------

main() {
  printf '== release-smoke: dist=%s ==\n' "$dist"
  check_checksums
  check_archives
  check_linux_in_container
  check_install_smoke
  printf '\nrelease-smoke: ok (every artifact-shape + content check passed)\n'
}

main "$@"
