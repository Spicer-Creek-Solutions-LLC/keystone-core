#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0

#
# scripts/repo/publish.sh — publish the apt/dnf repositories to the
# self-hosted server (server-canonical, with an incremental local cache).
#
# Flow:
#   1. pull the server's current repo into the local cache (rsync down)
#   2. merge this release's packages and regenerate + sign the metadata
#      over the FULL set (so every published version stays installable —
#      users can pin/downgrade)
#   3. verify the signatures locally
#   4. rsync up to the server (--delay-updates → near-atomic switchover)
#   5. verify the live URL (optional)
#
# The GPG signing key is the same one as the release ceremony and signs
# on the host; nothing here uploads key material.
#
# Required env:
#   REPO_PUBLISH_DEST   rsync destination of the web root, taken verbatim
#                       — e.g. deploy@repos.example.io:/srv/www/repos
#                       (a local path works too, for testing). Host, user,
#                       and path are entirely yours to set.
#   REPO_SIGN           key:<gpg-id> to publish a signed repo, or
#                       `unsigned` to publish without a key (the v0.1–v0.7
#                       posture — clients use [trusted=yes] /
#                       repo_gpgcheck=0). test/skip are refused.
#
# Optional env:
#   REPO_DIR            local cache dir (default: dist/repos; it is a
#                       cache — the server is canonical, so losing it just
#                       triggers a re-pull)
#   REPO_PACKAGES       new packages to merge (default: dist/)
#   REPO_CHANNEL        default: stable
#   REPO_PUBLIC_URL     https base for the post-publish live check, e.g.
#                       https://repos.keystone-core.io (skipped if unset)
#   RSYNC_SSH           remote shell for rsync (default: ssh)
#
# Flags:
#   --first-publish   the server is empty: skip the pull and the prune
#                     (additive upload, no --delete)
#   --dry-run         do everything except the real upload (rsync --dry-run)
#
# Exit codes: 0 ok; 1 bad invocation; 2 build/verify refused; 3 rsync failed
#

set -euo pipefail

here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)

first_publish=0
dry_run=0
while [ $# -gt 0 ]; do
  case "$1" in
    --first-publish) first_publish=1; shift ;;
    --dry-run)       dry_run=1; shift ;;
    -h|--help)       sed -n '2,55p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "repo-publish: unknown argument: $1" >&2; exit 1 ;;
  esac
done

die() { echo "repo-publish: $*" >&2; exit "${2:-2}"; }

dest=${REPO_PUBLISH_DEST:-}
[ -n "$dest" ] || die "set REPO_PUBLISH_DEST to the rsync web-root destination" 1
sign=${REPO_SIGN:-}
case "$sign" in
  key:*)    signed=1 ;;
  unsigned) signed=0
            echo "repo-publish: WARNING — publishing UNSIGNED (REPO_SIGN=unsigned)." >&2
            echo "  Clients must trust the repo without a signature ([trusted=yes] /" >&2
            echo "  repo_gpgcheck=0). This matches the v0.1–v0.7 unsigned posture;" >&2
            echo "  signatures land at v0.8 (RELEASE-PLAYBOOK §6)." >&2 ;;
  test|skip) die "refusing to publish with REPO_SIGN=$sign — use key:<id> or unsigned" 1 ;;
  "")  die "set REPO_SIGN=key:<gpg-id> (signed) or REPO_SIGN=unsigned" 1 ;;
  *)   die "unknown REPO_SIGN=$sign — use key:<id> or unsigned" 1 ;;
esac
cache=${REPO_DIR:-dist/repos}
packages=${REPO_PACKAGES:-dist/}
channel=${REPO_CHANNEL:-stable}
rsh=${RSYNC_SSH:-ssh}

command -v rsync >/dev/null 2>&1 || die "rsync required" 1
mkdir -p "$cache"

# ---- 1. Pull (server is canonical) ---------------------------------------
if [ "$first_publish" = 1 ]; then
  echo "repo-publish: --first-publish — skipping pull (additive upload)"
else
  echo "repo-publish: pulling current repo from $dest"
  rsync -a --delete -e "$rsh" "$dest/" "$cache/" \
    || die "pull failed — for an empty server use --first-publish" 3
fi

# ---- 2. Merge this release + regenerate metadata -------------------------
# Signed mode passes the key through to build.sh; unsigned mode builds with
# --sign skip (no key required), producing an unsigned Release / repomd.
echo "repo-publish: merging $packages and rebuilding metadata"
build_sign=$sign
[ "$signed" = 0 ] && build_sign=skip
"$here/build.sh" --packages "$packages" --out "$cache" --channel "$channel" --sign "$build_sign" \
  || die "repo-build failed" 2

# ---- 3. Local safety + signature verification ----------------------------
# A test-key tree must never reach the server, regardless of mode.
[ -f "$cache/TESTKEY-DO-NOT-PUBLISH" ] && die "refusing to publish a test-key tree (REPO_SIGN=test)" 2

if [ "$signed" = 1 ]; then
  [ -f "$cache/keystone-core-archive-keyring.asc" ] || die "no signing pubkey in tree — refusing to publish unsigned (use REPO_SIGN=unsigned to publish without a key)" 2
  inrelease="$cache/apt/dists/$channel/InRelease"
  [ -f "$inrelease" ] || die "missing $inrelease — refusing to publish" 2
  # The signing key (REPO_SIGN=key:<id>) is in the operator's keyring, so a
  # plain --verify confirms the metadata is intact and signed by it.
  gpg --verify "$inrelease" >/dev/null 2>&1 || die "InRelease failed local signature verify" 2
  for asc in "$cache"/rpm/"$channel"/*/repodata/repomd.xml.asc; do
    [ -f "$asc" ] || continue
    gpg --verify "$asc" "${asc%.asc}" >/dev/null 2>&1 \
      || die "repomd.xml failed local signature verify ($asc)" 2
  done
  echo "repo-publish: local signature verification ok"
else
  echo "repo-publish: unsigned mode — skipping signature verification"
fi

# ---- 4. Upload (near-atomic) ---------------------------------------------
# --delay-updates stages every transferred file and renames them into
# place at the very end, so a client's `apt-get update` never sees
# metadata that points at a not-yet-uploaded package.
up=(-a --delay-updates --human-readable -e "$rsh")
[ "$first_publish" = 1 ] || up+=(--delete-after)   # prune stale metadata; packages are all in the cache
if [ "$dry_run" = 1 ]; then up+=(--dry-run); echo "repo-publish: uploading to $dest (dry-run)"; else echo "repo-publish: uploading to $dest"; fi
rsync "${up[@]}" "$cache/" "$dest/" || die "rsync upload failed" 3

if [ "$dry_run" = 1 ]; then
  echo "repo-publish: dry-run complete (no changes pushed)"
  exit 0
fi

# ---- 5. Live verification (optional) -------------------------------------
if [ -n "${REPO_PUBLIC_URL:-}" ]; then
  if command -v curl >/dev/null 2>&1; then
    echo "repo-publish: verifying live $REPO_PUBLIC_URL"
    live_rel="$cache/.live-Release"
    name=Release; [ "$signed" = 1 ] && name=InRelease
    curl -fsSL "$REPO_PUBLIC_URL/apt/dists/$channel/$name" -o "$live_rel" \
      || die "could not fetch live $name from $REPO_PUBLIC_URL" 2
    if [ "$signed" = 1 ]; then
      gpg --verify "$live_rel" >/dev/null 2>&1 || die "LIVE InRelease failed signature verify" 2
      echo "repo-publish: live signature verification ok"
    else
      echo "repo-publish: live reachability ok (unsigned — no signature to verify)"
    fi
    rm -f "$live_rel"
  else
    echo "repo-publish: curl not found — skipping live verification" >&2
  fi
fi

echo ""
echo "repo-publish: ok — published to $dest"
[ -n "${REPO_PUBLIC_URL:-}" ] && echo "  live at $REPO_PUBLIC_URL"
exit 0
