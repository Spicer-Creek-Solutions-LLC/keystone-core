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

# Content drift: someone bumped worker_processes by hand.
sed -i 's/^worker_processes .*/worker_processes 1;/' "${WORKDIR}/nginx.conf"

# Mode drift: and loosened permissions on the env file.
chmod 0666 "${WORKDIR}/app.env"

echo "drift induced: nginx.conf content + app.env mode"
