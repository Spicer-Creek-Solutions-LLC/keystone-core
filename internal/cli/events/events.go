// SPDX-License-Identifier: Apache-2.0

// Package events implements the `kscore-events` operator CLI per
// Epic 11 task 7. v1.0 ships:
//
//	list / get / emit
//	subscribe / watch / replay
//	types / stats
//
// All subcommands talk to the EventService gRPC hosted by
// kscore-server (default `localhost:5397`; override via
// `--server host:port`). API-key auth flows through the
// `authorization: Bearer <key>` metadata header, sourced from
// `--api-key` or the `KSCORE_API_KEY` environment variable.
//
// Subcommands deferred to v0.x ROADMAP:
//
//	retention — needs the retention RPC that lands with task 8.
//	analyze   — operator analysis tool; spec is fuzzy in v1.0.
//	query     — CEL-filtered query; subsumed by
//	            `subscribe --replay <window>` for the v1.0 use cases.
package events

import (
	"context"
	"io"

	"github.com/spf13/cobra"

	"go.keystone-core.io/keystone-core/internal/cli"
	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

// Output format names. JSONLines is the default for streaming
// subcommands so jq pipelines work out of the box; the single-event
// subcommands default to FormatTable.
const (
	FormatTable     = "table"
	FormatJSON      = "json"
	FormatJSONLines = "jsonlines"
)

// Deps wires the gRPC client factory. Production uses [dialGRPC];
// tests inject a stub that returns an in-process bufconn client.
type Deps struct {
	Dial func(ctx context.Context, target, apiKey string) (v1.EventServiceClient, io.Closer, error)
}

// globals are shared by every subcommand under `kscore-events`;
// populated from the parent command's persistent flags.
type globals struct {
	Server string
	APIKey string
	Output string
	Deps   Deps
}

// NewCommand returns the root cobra.Command for the kscore-events
// binary. Pass a zero Deps to use the production gRPC dialer.
func NewCommand(deps Deps) *cobra.Command {
	if deps.Dial == nil {
		deps.Dial = dialGRPC
	}
	g := &globals{Deps: deps}

	cmd := &cobra.Command{
		Use:   "kscore-events",
		Short: "Keystone Core events operator CLI",
		Long: "Operator surface for the Keystone Core event bus — list / get / " +
			"emit events; subscribe / watch / replay event streams; inspect the " +
			"taxonomy + storage stats.\n\n" +
			"All commands talk to the EventService gRPC on the running " +
			"kscore-server (default localhost:5397).",
	}
	cmd.PersistentFlags().StringVar(&g.Server, "server", "localhost:5397",
		"events-service gRPC address (host:port)")
	cmd.PersistentFlags().StringVar(&g.APIKey, "api-key", "",
		"API key for authentication (or set KSCORE_API_KEY)")
	cmd.PersistentFlags().StringVarP(&g.Output, "output", "o", FormatTable,
		"output format for non-streaming commands: table | json")

	cmd.AddCommand(listCmd(g))
	cmd.AddCommand(getCmd(g))
	cmd.AddCommand(emitCmd(g))
	cmd.AddCommand(subscribeCmd(g))
	cmd.AddCommand(watchCmd(g))
	cmd.AddCommand(replayCmd(g))
	cmd.AddCommand(typesCmd(g))
	cmd.AddCommand(statsCmd(g))
	cli.AddVersion(cmd)

	return cmd
}
