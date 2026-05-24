// SPDX-License-Identifier: Apache-2.0

// Package exec implements the `kscorectl exec` subcommand tree
// (Epic 07 task 11). v1.0 ships three streaming subcommands —
// `run`, `async`, `script` — all backed by the gRPC
// ControlPlaneService.BatchExecuteCommand RPC. The five inspection
// subcommands (status / list / cancel / output) plus `--dry-run` land
// in 11b alongside their proto RPC additions.
package exec

import (
	"context"
	"io"

	"github.com/spf13/cobra"

	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

// OutputFormat enumerates the supported renderers.
const (
	FormatTable = "table"
	FormatJSON  = "json"
	FormatYAML  = "yaml"
)

// Deps wires the gRPC client factory. Production code uses dialGRPC;
// tests inject a stub that returns a fake client.
type Deps struct {
	Dial func(ctx context.Context, target, apiKey string) (v1.ControlPlaneServiceClient, io.Closer, error)
}

// globals are shared by every subcommand under exec; populated from
// the parent command's persistent flags.
type globals struct {
	Server string
	APIKey string
	Output string
	Deps   Deps
}

// NewCommand returns the `exec` parent command with its subcommands
// attached. Pass an empty Deps to use the production gRPC dialer.
func NewCommand(deps Deps) *cobra.Command {
	if deps.Dial == nil {
		deps.Dial = dialGRPC
	}
	g := &globals{Deps: deps}

	cmd := &cobra.Command{
		Use:   "exec",
		Short: "Execute commands across agents",
		Long: "Streaming command dispatch backed by the control plane's " +
			"BatchExecuteCommand RPC. Targets are AND'd label / hostname-glob " +
			"selectors today; full expression-string targeting (os: / arch: / " +
			"OR / NOT / glob-on-id) lands in v1.x with a proto extension.",
	}
	cmd.PersistentFlags().StringVar(&g.Server, "server", "localhost:5397",
		"control-plane gRPC address (host:port)")
	cmd.PersistentFlags().StringVar(&g.APIKey, "api-key", "",
		"API key for authentication (or set KSCORE_API_KEY)")
	cmd.PersistentFlags().StringVarP(&g.Output, "output", "o", FormatTable,
		"output format: table | json | yaml")

	cmd.AddCommand(runCmd(g))
	cmd.AddCommand(asyncCmd(g))
	cmd.AddCommand(scriptCmd(g))
	cmd.AddCommand(statusCmd(g))
	cmd.AddCommand(listCmd(g))
	cmd.AddCommand(cancelCmd(g))
	cmd.AddCommand(outputCmd(g))

	return cmd
}
