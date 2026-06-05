#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0

#
# test/e2e/state/run.sh — Epic 08 cross-distro state stdlib matrix.
#
# Invoked by `make test-cross-distro`. Boots each distro in a
# privileged container running its real init system (systemd / OpenRC),
# then runs smoke.sh inside it (via `docker exec`) to apply the state
# fixture twice and assert idempotency — exercising the package
# (apt/dnf/apk) and service (systemd/OpenRC) backends against the live
# system.
#
# Why `docker run -d` + `docker exec` rather than docker-compose: the
# service module needs a *booted* init as PID 1 (systemctl/rc-service
# talk to it), so each container's PID 1 is systemd (or, for Alpine,
# a keep-alive after the OpenRC runtime is primed) and the smoke runs
# as an exec into the live system. compose's run-to-exit model doesn't
# fit a PID-1-is-init container.
#
# Privileged: systemd-as-PID-1 needs cgroup write access; this target
# is a manual / Docker-host gate (it is NOT wired into CI, which runs
# on a shared unprivileged runner pool), so privileged is acceptable
# here. Without Docker the target skips cleanly so it stays usable in
# `make check` chains on developer machines.

set -euo pipefail

HERE="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd -- "$HERE/../../.." && pwd)"

if ! command -v docker >/dev/null 2>&1; then
  cat <<'MSG'
test-cross-distro: docker not installed — skipping.

  This target boots each distro (Debian / Ubuntu / Rocky / Alpine) in a
  privileged container with its real init system and applies the state
  smoke fixtures, exercising the package + service backends live.
  Install Docker to enable it. See test/e2e/state/README.md.
MSG
  exit 0
fi
if ! docker info >/dev/null 2>&1; then
  echo "test-cross-distro: docker daemon not reachable — skipping." >&2
  exit 0
fi

# --- the matrix -----------------------------------------------------
#
# Fields (|-separated): name image init pkg_mgr pkg svc boot
#   boot — the container's PID-1 command. For systemd distros it
#   installs systemd (the base images ship without it) then execs it;
#   for Alpine it primes the OpenRC runtime then keep-alives so the
#   smoke can exec rc-service/rc-update against it.
DISTROS=(
  "debian-12|debian:12|systemd|apt|cron|cron|apt-get update -qq >/dev/null 2>&1 && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq systemd systemd-sysv >/dev/null 2>&1; exec /lib/systemd/systemd"
  "ubuntu-22-04|ubuntu:22.04|systemd|apt|cron|cron|apt-get update -qq >/dev/null 2>&1 && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq systemd systemd-sysv >/dev/null 2>&1; exec /lib/systemd/systemd"
  "ubuntu-24-04|ubuntu:24.04|systemd|apt|cron|cron|apt-get update -qq >/dev/null 2>&1 && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq systemd systemd-sysv >/dev/null 2>&1; exec /lib/systemd/systemd"
  "rocky-9|rockylinux:9|systemd|dnf|cronie|crond|dnf install -y -q systemd >/dev/null 2>&1; exec /usr/lib/systemd/systemd"
  "alpine-3-19|alpine:3.19|openrc|apk|dcron|dcron|apk add --no-cache openrc >/dev/null 2>&1; mkdir -p /run/openrc; touch /run/openrc/softlevel; exec sleep infinity"
)

# --- build the static apply-harness once, on the host --------------
#
# CGO-free so the one binary runs on glibc (Debian/Ubuntu/Rocky) and
# musl (Alpine) alike; mounted read-only into every container.
BINDIR="$(mktemp -d)"
HARNESS_BIN="$BINDIR/kscore-state-harness"
cleanup() { rm -rf "$BINDIR" 2>/dev/null || true; }
trap cleanup EXIT

echo "==> Building static apply-harness…"
( cd "$ROOT" && CGO_ENABLED=0 go build -trimpath -o "$HARNESS_BIN" ./test/e2e/state/harness )

# wait_ready polls until the container's init is up (or times out).
wait_ready() {
  local cname="$1" init="$2" i st
  for i in $(seq 1 40); do
    case "$init" in
      systemd)
        st="$(docker exec "$cname" systemctl is-system-running 2>/dev/null || true)"
        case "$st" in running|degraded) return 0 ;; esac
        ;;
      openrc)
        if docker exec "$cname" test -f /run/openrc/softlevel 2>/dev/null; then return 0; fi
        ;;
    esac
    sleep 3
  done
  return 1
}

PASS=(); FAIL=()

run_distro() {
  local name="$1" image="$2" init="$3" pkg_mgr="$4" pkg="$5" svc="$6" boot="$7"
  local cname="kscore-e2e-$name"
  echo
  echo "==> [$name] booting $image ($init)…"
  docker rm -f "$cname" >/dev/null 2>&1 || true

  if ! docker run -d --name "$cname" --privileged --cgroupns=host \
      -v /sys/fs/cgroup:/sys/fs/cgroup:rw \
      -v "$ROOT":/src:ro \
      -v "$HARNESS_BIN":/usr/local/bin/kscore-state-harness:ro \
      "$image" bash -c "$boot" >/dev/null 2>&1; then
    # Alpine's base has no bash; fall back to sh for the boot command.
    docker rm -f "$cname" >/dev/null 2>&1 || true
    docker run -d --name "$cname" --privileged --cgroupns=host \
      -v /sys/fs/cgroup:/sys/fs/cgroup:rw \
      -v "$ROOT":/src:ro \
      -v "$HARNESS_BIN":/usr/local/bin/kscore-state-harness:ro \
      "$image" sh -c "$boot" >/dev/null 2>&1 || true
  fi

  if ! wait_ready "$cname" "$init"; then
    echo "==> [$name] init did not come up in time" >&2
    docker logs "$cname" 2>&1 | tail -5 >&2 || true
    docker rm -f "$cname" >/dev/null 2>&1 || true
    FAIL+=("$name"); return
  fi

  if docker exec \
      -e KSCORE_DISTRO="$name" -e KSCORE_PKG_MGR="$pkg_mgr" -e KSCORE_INIT="$init" \
      -e KSCORE_PKG="$pkg" -e KSCORE_SVC="$svc" \
      "$cname" sh /src/test/e2e/state/smoke.sh; then
    PASS+=("$name")
  else
    echo "==> [$name] smoke FAILED" >&2
    FAIL+=("$name")
  fi
  docker rm -f "$cname" >/dev/null 2>&1 || true
}

# Allow narrowing to a subset: `run.sh debian-12 alpine-3-19`.
SELECT=("$@")
selected() {
  [[ ${#SELECT[@]} -eq 0 ]] && return 0
  local want="$1" s
  for s in "${SELECT[@]}"; do [[ "$s" == "$want" ]] && return 0; done
  return 1
}

for entry in "${DISTROS[@]}"; do
  IFS='|' read -r name image init pkg_mgr pkg svc boot <<<"$entry"
  selected "$name" || continue
  run_distro "$name" "$image" "$init" "$pkg_mgr" "$pkg" "$svc" "$boot"
done

echo
echo "==> Matrix summary: ${#PASS[@]} passed, ${#FAIL[@]} failed"
[[ ${#PASS[@]} -gt 0 ]] && echo "    pass: ${PASS[*]}"
if [[ ${#FAIL[@]} -gt 0 ]]; then
  echo "    fail: ${FAIL[*]}" >&2
  exit 1
fi
echo "==> All selected distros green."
