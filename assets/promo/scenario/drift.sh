#!/usr/bin/env bash
# Induce the drift that shot 4 detects.
#
# Writes to the host side of the /srv/promo bind mount, out of band and
# behind Keystone's back — exactly the "someone SSH'd in and edited it"
# case the product exists to catch. No shell inside the container
# required (the kscore images are distroless).
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
WORKDIR="${REPO_ROOT}/build/promo/workdir"

if [[ ! -f "${WORKDIR}/nginx.conf" ]]; then
  echo "ERROR: ${WORKDIR}/nginx.conf missing — run the state-apply shot first" >&2
  exit 1
fi

# Only nginx.conf is driven, and only by content. Both managed files are
# owned by the container's nonroot UID (65532), so from the host side of
# the bind mount:
#   - chmod fails      -- the host user does not own the file
#   - app.env is 0640  -- the host user cannot even read it to edit it
# nginx.conf is 0644, so sed -i can read it and rename a new file into
# the world-writable directory over it.
#
# Leaving app.env in sync is the better shot anyway: the drift report
# lands one `drifted` row next to one `in_sync` row, which shows the
# check discriminating rather than flagging everything it looks at.

# Someone halved worker_processes on the box.
sed -i 's/^worker_processes .*/worker_processes 1;/' "${WORKDIR}/nginx.conf"

echo "drift induced: nginx.conf content"
