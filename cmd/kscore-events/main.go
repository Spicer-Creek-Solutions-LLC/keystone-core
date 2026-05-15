// kscore-events is the Keystone Core operator CLI for the events
// domain (Epic 11 task 7).
//
// v1.0 subcommand surface:
//
//	list / get / emit
//	subscribe / watch / replay
//	types / stats
//
// All commands talk to the EventService gRPC hosted by
// kscore-server (default `localhost:9090`; override via
// `--server host:port`). API-key auth flows through the
// `authorization: Bearer …` header, sourced from `--api-key`
// or the `KSCORE_API_KEY` environment variable.
//
// Subcommands deferred to v0.x ROADMAP:
//
//	retention — needs the retention RPC that lands with task 8.
//	analyze   — operator analysis tool; fuzzy spec for v1.0.
//	query     — CEL-filtered query; subsumed by
//	            `subscribe --replay <window>` for v1.0 use cases.
package main

import (
	"os"

	"go.keystone-core.io/keystone-core/internal/cli/events"
)

func main() {
	if err := events.NewCommand(events.Deps{}).Execute(); err != nil {
		os.Exit(1)
	}
}
