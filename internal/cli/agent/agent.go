// SPDX-License-Identifier: Apache-2.0

// Package agent implements the `kscorectl agent` subcommand tree:
// `list`, plus the incident-response pair `quarantine` /
// `unquarantine`. Other per-agent operations (get, …) are future
// tickets.
package agent

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
)

// Deps wires the gRPC client factory. Production code uses dialGRPC;
// tests inject a stub that returns a fake client.
type Deps struct {
	Dial func(ctx context.Context, target, apiKey string) (v1.ControlPlaneServiceClient, io.Closer, error)
}

// globals are shared by every subcommand under agent; populated from
// the parent command's persistent flags.
type globals struct {
	Server string
	APIKey string
	Output string
	Deps   Deps
}

// NewCommand returns the `agent` parent command with its subcommands
// attached. Pass an empty Deps to use the production gRPC dialer.
func NewCommand(deps Deps) *cobra.Command {
	if deps.Dial == nil {
		deps.Dial = dialGRPC
	}
	g := &globals{Deps: deps}

	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Inspect registered agents",
		Long: "Operator queries against the control plane's ControlPlaneService. " +
			"v0.x ships `list`; per-agent operations land as separate tickets.",
	}
	cmd.PersistentFlags().StringVar(&g.Server, "server", "localhost:5397",
		"control-plane gRPC address (host:port)")
	cmd.PersistentFlags().StringVar(&g.APIKey, "api-key", "",
		"API key for authentication (or set KSCORE_API_KEY)")
	cmd.PersistentFlags().StringVarP(&g.Output, "output", "o", FormatTable,
		"output format: table | json")

	cmd.AddCommand(listCmd(g))
	cmd.AddCommand(quarantineCmd(g))
	cmd.AddCommand(unquarantineCmd(g))
	cmd.AddCommand(verifyCmd(g))

	return cmd
}
