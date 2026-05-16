// kscore-audit is the Keystone Core operator CLI for the audit log
// (Epic 12 task 14).
//
// v1.0 subcommand surface:
//
//	log    — paginated audit-log query
//	report — compliance report (rate, top violations, trend)
//	stats  — headline counts over a window
//
// All commands talk to the PolicyService gRPC on the running
// kscore-server (default `localhost:9090`; override via
// `--server host:port`). API-key auth via `--api-key` or
// `KSCORE_API_KEY`.
//
// Deferred: `export` is owned by Epic 12 task 15 (JSON/JSONL/CSV
// formatters + redaction). search / analyze / timeline / watch are
// v1.x ROADMAP (no RPC / fuzzy spec).
package main

import (
	"os"

	"go.keystone-core.io/keystone-core/internal/cli/audit"
)

func main() {
	if err := audit.NewCommand(audit.Deps{}).Execute(); err != nil {
		os.Exit(1)
	}
}
