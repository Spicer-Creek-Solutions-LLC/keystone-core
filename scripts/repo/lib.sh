#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
#
# scripts/repo/lib.sh — shared helpers for the repo build/smoke scripts.
# Sourced, not executed.

# container_engine: print the container runtime to use and return 0, or
# return 1 if none is available. Honors an explicit CONTAINER_ENGINE
# override, else prefers docker, else podman — mirroring the Makefile's
# docs-lint-container `cre` idiom so the whole project resolves the
# runtime the same way.
container_engine() {
  if [ -n "${CONTAINER_ENGINE:-}" ]; then
    if command -v "$CONTAINER_ENGINE" >/dev/null 2>&1; then
      echo "$CONTAINER_ENGINE"; return 0
    fi
    echo "repo: CONTAINER_ENGINE=$CONTAINER_ENGINE not found on PATH" >&2
    return 1
  fi
  command -v docker >/dev/null 2>&1 && { echo docker; return 0; }
  command -v podman >/dev/null 2>&1 && { echo podman; return 0; }
  return 1
}

# want_container: true when the caller should use a container instead of
# host tools — either because KSCORE_REPO_CONTAINER=1 forces it (used to
# exercise the macOS code path on a Linux host) or because the named host
# tool is absent (the macOS reality, where the Debian/rpm tools don't
# exist natively).
#   want_container <host-tool-name>
want_container() {
  [ "${KSCORE_REPO_CONTAINER:-0}" = "1" ] && return 0
  command -v "$1" >/dev/null 2>&1 && return 1
  return 0
}
