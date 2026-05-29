// SPDX-License-Identifier: Apache-2.0

// kscore-audit is the Keystone Core operator CLI for the audit log
// (Epic 12 task 14).
//
// v1.0 subcommand surface:
//
//	log    — paginated audit-log query
//	report — compliance report (rate, top violations, trend)
//	stats  — headline counts over a window
//	export — stream JSON / JSONL / CSV with redaction on export
//
// All commands talk to the PolicyService gRPC on the running
// kscore-server (default `localhost:5397`; override via
// `--server host:port`). API-key auth via `--api-key` or
// `KSCORE_API_KEY`.
//
// Deferred to v1.x ROADMAP (no RPC / fuzzy spec): search / analyze /
// timeline / watch.
package main

import (
	"fmt"
	"os"

	"go.keystone-core.io/keystone-core/internal/cli/audit"
)

func main() {
	if err := audit.NewCommand(audit.Deps{}).Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
