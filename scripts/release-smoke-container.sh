#!/usr/bin/env bash
#
# scripts/release-smoke-container.sh — Epic 19 task 13.
#
# Linux-side artifact checks. Runs inside a debian:12-slim container
# (because debian has dpkg-deb natively + rpm via apt; the host only
# needs Docker, sha256sum, tar, unzip). Driven by
# scripts/release-smoke.sh — do not invoke directly from the host.
#
# Usage (inside container):
#   release-smoke-container.sh <dist-mount>
#
# Receives a read-only mount of dist/ at the supplied path. Exits 0
# when every check passes, non-zero on the first failure.
#
# Checks (in order):
#   1. linux/amd64 binary --version: extract the tarball, run each
#      kscore-* + kscorectl, assert semver-shaped output.
#   2. .deb content: every nfpm-produced .deb (6 files) contains the
#      expected binary + systemd unit / doc paths.
#   3. .rpm content: same six checks against the rpm payload.

set -euo pipefail

dist=${1:?"usage: $0 <dist-mount>"}

# Install rpm if missing (debian has dpkg-deb natively; rpm needs an apt
# install in container mode). When invoked natively on a host that already
# has both tools, this no-ops. apt-get is required only when rpm is absent.
if ! command -v rpm >/dev/null 2>&1; then
  if ! command -v apt-get >/dev/null 2>&1; then
    echo "FAIL: rpm missing and apt-get not available to install it" >&2
    exit 1
  fi
  apt-get update -qq >/dev/null
  apt-get install -y --no-install-recommends rpm >/dev/null 2>&1
fi
if ! command -v dpkg-deb >/dev/null 2>&1; then
  echo "FAIL: dpkg-deb missing on host" >&2
  exit 1
fi

BINARIES=(
  kscore-server kscore-agent kscorectl
  kscore-audit kscore-backup kscore-blueprint kscore-bootstrap
  kscore-cluster kscore-cluster-backup kscore-events kscore-files
  kscore-gitops kscore-identity kscore-migrate kscore-module
  kscore-policy kscore-registry kscore-runbook kscore-secrets
  kscore-webhook
)

# kscore-cli bundle subbinaries — every operator subcommand binary
# except kscorectl itself.
CLI_SUBS=(
  kscore-audit kscore-backup kscore-blueprint kscore-bootstrap
  kscore-cluster kscore-cluster-backup kscore-events kscore-files
  kscore-gitops kscore-identity kscore-migrate kscore-module
  kscore-policy kscore-registry kscore-runbook kscore-secrets
  kscore-webhook
)

NFPM_FAMILIES=(
  "kscore-server /lib/systemd/system/kscore-server.service"
  "kscore-agent  /lib/systemd/system/kscore-agent.service"
  "kscore-cli    /usr/share/doc/kscore-cli/LICENSE"
)
NFPM_ARCHES=(amd64 arm64)

pass() { printf 'PASS: %s\n' "$*"; }
fail() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }

# -- 1. binary --version on linux/amd64 -----------------------------------

check_binary_versions() {
  printf '\n-- binary --version (linux/amd64) --\n'
  local archive tmp
  archive=$(ls "$dist"/keystone-core_*_linux_amd64.tar.gz | head -n1)
  [ -n "$archive" ] || fail "linux/amd64 archive not found"

  tmp=$(mktemp -d)
  tar -xzf "$archive" -C "$tmp"

  for bin in "${BINARIES[@]}"; do
    local p="$tmp/$bin"
    [ -x "$p" ] || fail "$bin extracted but not executable"
    local out
    out=$("$p" --version 2>&1) || fail "$bin --version exited non-zero (output: $out)"
    [ -n "$out" ] || fail "$bin --version produced empty output"
    echo "$out" | grep -qE '[0-9]+\.[0-9]+\.[0-9]+' \
      || fail "$bin --version output lacks semver: $out"
  done
  pass "all 20 linux/amd64 binaries report a semver --version"
  rm -rf "$tmp"
}

# -- 2. .deb content ------------------------------------------------------

check_deb_contents() {
  printf '\n-- .deb content (6 expected) --\n'
  local family expect arch deb listing sub
  for fam_spec in "${NFPM_FAMILIES[@]}"; do
    # shellcheck disable=SC2086
    set -- $fam_spec
    family=$1 expect=$2
    for arch in "${NFPM_ARCHES[@]}"; do
      deb=$(ls "$dist"/"${family}"_*_linux_"${arch}".deb 2>/dev/null | head -n1)
      [ -n "$deb" ] || fail ".deb missing for $family/$arch"
      listing=$(dpkg-deb --contents "$deb" | awk '{print $NF}')

      case "$family" in
        kscore-server)
          echo "$listing" | grep -qE '^\./usr/local/bin/kscore-server$' \
            || fail "$deb missing /usr/local/bin/kscore-server"
          ;;
        kscore-agent)
          echo "$listing" | grep -qE '^\./usr/local/bin/kscore-agent$' \
            || fail "$deb missing /usr/local/bin/kscore-agent"
          ;;
        kscore-cli)
          echo "$listing" | grep -qE '^\./usr/local/bin/kscorectl$' \
            || fail "$deb missing /usr/local/bin/kscorectl"
          for sub in "${CLI_SUBS[@]}"; do
            echo "$listing" | grep -qE "^\./usr/local/bin/${sub}$" \
              || fail "$deb missing /usr/local/bin/${sub} (kscore-cli bundle)"
          done
          ;;
      esac

      echo "$listing" | grep -qE "^\.${expect}$" \
        || fail "$deb missing $expect"
      pass "$(basename "$deb")"
    done
  done
}

# -- 3. .rpm content ------------------------------------------------------

check_rpm_contents() {
  printf '\n-- .rpm content (6 expected) --\n'
  local family expect arch rpm listing sub
  for fam_spec in "${NFPM_FAMILIES[@]}"; do
    # shellcheck disable=SC2086
    set -- $fam_spec
    family=$1 expect=$2
    for arch in "${NFPM_ARCHES[@]}"; do
      rpm=$(ls "$dist"/"${family}"_*_linux_"${arch}".rpm 2>/dev/null | head -n1)
      [ -n "$rpm" ] || fail ".rpm missing for $family/$arch"
      listing=$(rpm -qlp "$rpm" 2>/dev/null)

      case "$family" in
        kscore-server)
          echo "$listing" | grep -qE '^/usr/local/bin/kscore-server$' \
            || fail "$rpm missing /usr/local/bin/kscore-server"
          ;;
        kscore-agent)
          echo "$listing" | grep -qE '^/usr/local/bin/kscore-agent$' \
            || fail "$rpm missing /usr/local/bin/kscore-agent"
          ;;
        kscore-cli)
          echo "$listing" | grep -qE '^/usr/local/bin/kscorectl$' \
            || fail "$rpm missing /usr/local/bin/kscorectl"
          for sub in "${CLI_SUBS[@]}"; do
            echo "$listing" | grep -qE "^/usr/local/bin/${sub}$" \
              || fail "$rpm missing /usr/local/bin/${sub} (kscore-cli bundle)"
          done
          ;;
      esac

      echo "$listing" | grep -qE "^${expect}$" \
        || fail "$rpm missing $expect"
      pass "$(basename "$rpm")"
    done
  done
}

check_binary_versions
check_deb_contents
check_rpm_contents
