#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0

# Keystone Core smoke test — fast pre-commit gate.
#
# Modes:
#   quick   compile gate + SQLite WAL/FK/busy-timeout pragmas (default)
#
# Budget: must finish in well under 10s on a warm cache. See epic 01 risks.

set -euo pipefail

mode="${1:-quick}"

log() { printf 'smoke[%s]: %s\n' "$mode" "$*"; }

run_quick() {
    log "compile (go build ./...)"
    go build ./...

    log "sqlite pragmas (pkg/dbutil)"
    # Epic 19 task 5 — -race matches the project default (every go
    # test invocation race-instrumented unless explicitly opted out
    # for wall-clock SLOs).
    CGO_ENABLED=1 go test -race -count=1 -timeout 30s \
        -run 'TestOpenSQLite_(OpensSuccessfully|PragmaJournalMode|PragmaForeignKeys|PragmaBusyTimeout)' \
        ./pkg/dbutil/...

    # TODO(epic-05): embedded NATS smoke — server start + publish/subscribe
    # round-trip when internal/nats lands.

    log "PASS"
}

case "$mode" in
    quick) run_quick ;;
    *)
        printf 'usage: %s {quick}\n' "$0" >&2
        exit 2
        ;;
esac
