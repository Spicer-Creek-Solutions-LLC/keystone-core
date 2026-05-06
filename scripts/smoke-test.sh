#!/usr/bin/env bash
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
    go test -count=1 -timeout 10s \
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
