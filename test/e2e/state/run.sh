#!/usr/bin/env bash
#
# test/e2e/state/run.sh — Layer C entry point for Epic 08 task 13.
#
# Invoked by `make test-cross-distro`. Runs the scaffolded smoke
# probe against every distro defined in docker-compose.yml.
#
# - When Docker is not available the script exits 0 with a clear
#   "skipped — install Docker to enable" message. The Makefile
#   target stays usable on developer machines without containers.
# - When Docker IS available, today only one distro (debian-12) is
#   wired up; the rest of the v0.5 matrix is TODO and tracked in
#   docs/project/ROADMAP.md.
#
# Anatomy: each docker-compose service runs smoke.sh, which builds
# the kscore-server + kscorectl binaries inside the container and
# applies smoke.yaml against the local FS. The compose call exits
# non-zero if any service returns non-zero.
#
# Re-runnable: containers + networks are removed on every run; the
# build artifacts live under .e2e-build/ inside the container's
# private layer, never on the host.

set -euo pipefail

HERE="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd -- "$HERE/../../.." && pwd)"

if ! command -v docker >/dev/null 2>&1; then
  cat <<'MSG'
test-cross-distro: docker not installed — skipping.

  This target runs the Epic 08 stdlib cross-distro smoke matrix in
  Linux containers (Debian / Ubuntu / Rocky / Alpine). Install Docker
  (or a docker-compose-compatible runtime like Podman) to enable it.

  See test/e2e/state/README.md for the full matrix and
  docs/project/ROADMAP.md "Cross-distro state stdlib docker matrix
  harness" (priority gate-v0.5) for the v0.5 acceptance criteria.
MSG
  exit 0
fi

# `docker compose` v2 is the modern entry; v1 is `docker-compose`.
# Prefer v2.
if docker compose version >/dev/null 2>&1; then
  DC=(docker compose)
elif command -v docker-compose >/dev/null 2>&1; then
  DC=(docker-compose)
else
  echo "test-cross-distro: 'docker compose' / 'docker-compose' missing — skipping." >&2
  exit 0
fi

cd "$HERE"

echo "==> Building + running cross-distro smoke (v0.5 scaffold)…"
echo "    project root: $ROOT"
echo

# Bring up + run in one shot. --abort-on-container-exit returns the
# exit code of the first service that exits, which is what we want
# for a CI-style pass/fail report.
"${DC[@]}" up --build --abort-on-container-exit --exit-code-from debian-12
status=$?

echo "==> Tearing down…"
"${DC[@]}" down --remove-orphans --volumes >/dev/null 2>&1 || true

if [[ $status -ne 0 ]]; then
  echo "test-cross-distro: at least one distro failed (exit $status)." >&2
  exit "$status"
fi
echo "==> All distros green."
