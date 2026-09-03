#!/usr/bin/env bash
# Render the 30-second promo video from assets/promo/.
#
# Stages:
#   deps      assert vhs / ttyd / ffmpeg / docker are present
#   up        bring the promo topology up (single-topology E2E + the
#             /srv/promo bind-mount overlay) and export KSCORE_API_KEY
#   render    run each tape through vhs -> build/promo/clips/<id>.mp4
#   assemble  trim each clip to its manifest duration, burn in the
#             lower-third captions, concatenate, write dist/promo/
#   down      tear the topology back down
#
# The shot list is never parsed here: `promogen plan` is the single
# manifest parser and this script consumes its TSV.
#
# Usage: assets/promo/pipeline/build.sh [--keep-up] [--skip-render]
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
cd "${REPO_ROOT}"

PROMO_DIR="assets/promo"
CLIP_DIR="build/promo/clips"
WORK_DIR="build/promo/workdir"
STAGE_DIR="build/promo/stage"
OUT_DIR="dist/promo"

COMPOSE=(docker compose
  -f test/e2e/single/docker-compose.yml
  -f assets/promo/scenario/docker-compose.promo.yml)

KEEP_UP=0
SKIP_RENDER=0
for arg in "$@"; do
  case "$arg" in
    --keep-up)     KEEP_UP=1 ;;
    --skip-render) SKIP_RENDER=1 ;;
    *) echo "unknown flag: $arg" >&2; exit 2 ;;
  esac
done

log() { printf '>>> %s\n' "$*"; }

# ---- deps -----------------------------------------------------------
# vhs is go-installable; ttyd and ffmpeg are system packages and cannot
# be, so this reports rather than silently installing.
check_deps() {
  local missing=()
  for tool in vhs ttyd ffmpeg docker; do
    command -v "$tool" >/dev/null || missing+=("$tool")
  done
  if ((${#missing[@]})); then
    cat >&2 <<EOF
ERROR: missing promo render dependencies: ${missing[*]}

  vhs     go install github.com/charmbracelet/vhs@latest   (or: make install-promo-tools)
  ttyd    system package (apt install ttyd / brew install ttyd) — vhs drives it
  ffmpeg  system package (apt install ffmpeg / brew install ffmpeg)
  docker  required for the promo topology

The generated half of the pipeline (make update-promo, make promo-check)
needs none of these and runs anywhere.
EOF
    exit 1
  fi
}

# ---- topology -------------------------------------------------------
up() {
  log "bringing the promo topology up"
  # The kscore images are distroless and run as UID 65532; the bind
  # mount has to be writable by that UID, and this host directory is
  # throwaway scratch under the gitignored build/ tree.
  rm -rf "${WORK_DIR}"
  mkdir -p "${WORK_DIR}"
  chmod 0777 "${WORK_DIR}"

  "${COMPOSE[@]}" up -d --wait

  log "extracting the dev API key from the server log"
  local key=""
  for _ in $(seq 1 30); do
    key="$(docker logs kscore-e2e-server 2>&1 \
      | grep 'DEV API KEY GENERATED' \
      | tail -1 \
      | python3 -c 'import json,sys; line=sys.stdin.read().strip(); print(json.loads(line)["key"] if line else "")' \
      2>/dev/null || true)"
    [[ -n "$key" ]] && break
    sleep 1
  done
  if [[ -z "$key" ]]; then
    echo "ERROR: could not read the dev API key from kscore-e2e-server logs" >&2
    exit 1
  fi
  export KSCORE_API_KEY="$key"

  # kscorectl must be on PATH for the tapes' `Require kscorectl`.
  local goos goarch
  goos="$(go env GOOS)"; goarch="$(go env GOARCH)"
  export PATH="${REPO_ROOT}/build/bin/${goos}/${goarch}:${PATH}"
  command -v kscorectl >/dev/null || {
    echo "ERROR: kscorectl not on PATH — run 'make build' first" >&2
    exit 1
  }
}

down() {
  if ((KEEP_UP)); then
    log "leaving the promo topology up (--keep-up)"
    return
  fi
  log "tearing the promo topology down"
  "${COMPOSE[@]}" down -v >/dev/null 2>&1 || true
}

# ---- render ---------------------------------------------------------

# vhs drives headless Chrome (via go-rod) to screenshot the ttyd
# terminal. Ubuntu 23.10+ ships AppArmor's
# kernel.apparmor_restrict_unprivileged_userns=1, which blocks Chrome's
# namespace sandbox: it aborts in ZygoteHostImpl::Init with SIGABRT and
# vhs reports only "recording failed".
#
# vhs exposes VHS_NO_SANDBOX for exactly this. Set it only when the
# restriction is actually in force, so machines without it keep the
# sandbox on. Chrome here renders our own local terminal, not untrusted
# web content. The alternative fixes both need root (flipping the
# sysctl, or installing an AppArmor profile), and a build tool should
# not require that.
configure_sandbox() {
  local restricted
  restricted="$(sysctl -n kernel.apparmor_restrict_unprivileged_userns 2>/dev/null || echo 0)"
  if [[ "$restricted" == "1" ]]; then
    log "unprivileged userns restricted by AppArmor; setting VHS_NO_SANDBOX"
    export VHS_NO_SANDBOX=1
  fi
}

render() {
  configure_sandbox
  log "rendering tapes"
  rm -rf "${CLIP_DIR}"
  mkdir -p "${CLIP_DIR}"

  while IFS=$'\t' read -r id kind duration tape caption; do
    : "${caption}"  # unused during render; burned in at assemble time
    log "  ${id} (${kind}, ${duration}s)"
    vhs "${PROMO_DIR}/${tape}" </dev/null

    local clip="${CLIP_DIR}/$(basename "${tape}" .tape).mp4"
    [[ -f "$clip" ]] || { echo "ERROR: ${id}: vhs produced no ${clip}" >&2; exit 1; }

    # A stale tape still renders happily with a traceback in frame.
    # Assert the shot did not capture an obvious failure.
    assert_clean "$id" "$clip"
  done < <(go run ./tools/promogen plan)
}

# assert_clean is a cheap tripwire for the failure mode that matters:
# a stale tape whose commands now error still renders happily, just
# with a traceback in frame. A blank or near-empty clip is the one
# symptom detectable without watching it. The real gates are
# `make promo-check` and a human viewing the 30 seconds before it is
# published anywhere.
assert_clean() {
  local id="$1" clip="$2"
  local bytes
  bytes="$(stat -c %s "$clip" 2>/dev/null || stat -f %z "$clip")"
  if (( bytes < 10000 )); then
    echo "ERROR: ${id}: clip is only ${bytes} bytes — the shot probably rendered blank" >&2
    exit 1
  fi
}

# ---- assemble -------------------------------------------------------
assemble() {
  log "assembling"
  rm -rf "${STAGE_DIR}"
  mkdir -p "${STAGE_DIR}" "${OUT_DIR}"

  local concat="${STAGE_DIR}/concat.txt"
  : > "$concat"

  # Every ffmpeg below runs with -nostdin. Without it ffmpeg reads the
  # same stdin this `while read` loop is consuming from `promogen plan`
  # and eats the remaining shot rows -- the symptom is ffmpeg parsing a
  # half-swallowed TSV line as an interactive command and then trying to
  # open a clip named after a caption.

  while IFS=$'\t' read -r id kind duration tape caption; do
    : "${kind}"  # unused during assemble; drives nothing but the log line
    local src staged
    src="${CLIP_DIR}/$(basename "${tape}" .tape).mp4"
    staged="${STAGE_DIR}/${id}.mp4"

    # Trim to the manifest duration; pad with the final frame when the
    # clip came up short, so the runtime budget is exact either way.
    local filters="tpad=stop_mode=clone:stop_duration=5"

    if [[ -n "$caption" ]]; then
      # Lower third. Escapes: ffmpeg's drawtext treats : and ' specially.
      local escaped="${caption//\\/\\\\}"
      escaped="${escaped//:/\\:}"
      escaped="${escaped//\'/\\\'}"
      filters+=",drawbox=y=ih-150:w=iw:h=150:color=black@0.62:t=fill"
      filters+=",drawtext=font='DejaVu Sans':text='${escaped}'"
      filters+=":fontcolor=white:fontsize=52:x=90:y=h-108"
    fi

    if [[ "$id" == "endcard" ]]; then
      # Logo sits ABOVE the tagline and is left-aligned with it. Centring
      # it (x=(W-w)/2) put it straight through the middle of the
      # left-aligned text, covering the last word of the positioning
      # line. x=215 matches the 8-space indent the card prints at
      # FontSize 30.
      ffmpeg -nostdin -y -loglevel error -i "$src" -i assets/logo.png \
        -filter_complex "[0:v]${filters}[base];[1:v]scale=150:-1[logo];[base][logo]overlay=x=215:y=150" \
        -t "$duration" -an -c:v libx264 -pix_fmt yuv420p -r 30 "$staged"
    else
      ffmpeg -nostdin -y -loglevel error -i "$src" -vf "$filters" \
        -t "$duration" -an -c:v libx264 -pix_fmt yuv420p -r 30 "$staged"
    fi

    printf "file '%s'\n" "${id}.mp4" >> "$concat"
  done < <(go run ./tools/promogen plan)

  local total
  total="$(go run ./tools/promogen plan | awk -F'\t' '{s+=$3} END {printf "%.3f", s}')"

  # Hard cuts between shots, with a fade in at the head and out at the
  # tail. Per-shot crossfades are a follow-up: a 6-stage xfade chain is
  # markedly harder to keep correct than it is worth at this length.
  log "  landscape 1920x1080"
  ffmpeg -nostdin -y -loglevel error -f concat -safe 0 -i "$concat" \
    -vf "fade=t=in:st=0:d=0.4,fade=t=out:st=$(awk -v t="$total" 'BEGIN{printf "%.3f", t-0.5}'):d=0.5" \
    -an -c:v libx264 -pix_fmt yuv420p -r 30 -movflags +faststart \
    "${OUT_DIR}/keystone-30s.mp4"

  log "  square 1080x1080"
  ffmpeg -nostdin -y -loglevel error -i "${OUT_DIR}/keystone-30s.mp4" \
    -vf "scale=1080:-2,pad=1080:1080:0:(oh-ih)/2:color=#11111b" \
    -an -c:v libx264 -pix_fmt yuv420p -r 30 -movflags +faststart \
    "${OUT_DIR}/keystone-30s-square.mp4"

  log "wrote ${OUT_DIR}/keystone-30s.mp4 (${total}s) + keystone-30s-square.mp4"
}

# ---- main -----------------------------------------------------------
check_deps
go run ./tools/promogen validate
# Cheapest possible failure. A tape whose command has been renamed still
# records happily with the error in frame, and the only other guard here
# (assert_clean) catches a blank clip, not a wrong one. Two seconds of
# probing beats a three-minute render of a broken shot.
log "verifying tape commands resolve"
go run ./tools/promogen tapes

if ((SKIP_RENDER)); then
  assemble
  exit 0
fi

trap down EXIT
up
render
assemble
