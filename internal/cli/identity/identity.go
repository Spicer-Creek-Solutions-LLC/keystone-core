// SPDX-License-Identifier: Apache-2.0

// Package identity implements the `kscore-identity` subcommand
// tree (Epic 09 task 12). v0.1 ships:
//
//	token create / list / revoke / cleanup
//	ca info / rotate-signing / export
//	status
//
// All subcommands talk to the IdentityService gRPC hosted by
// kscore-server. There is no local-only mode — operations are
// always on the live provider so the CLI doesn't race the running
// server's in-memory state.
//
// Federation (`add-domain / list / fetch-bundle`) is post-v1.0
// per §4.10 — ROADMAP entry "kscore-identity federation
// subcommands" tracks it.
package identity

import (
	"context"
	"io"

	"github.com/spf13/cobra"

	"go.keystone-core.io/keystone-core/internal/cli"
	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

// OutputFormat enumerates the supported renderers.
const (
	FormatTable = "table"
	FormatJSON  = "json"
)

// Deps wires the gRPC client factory. Production uses [dialGRPC];
// tests inject a stub that returns an in-process bufconn client.
type Deps struct {
	Dial func(ctx context.Context, target, apiKey string) (v1.IdentityServiceClient, io.Closer, error)
}

// globals are shared by every subcommand under `kscore-identity`;
// populated from the parent command's persistent flags.
type globals struct {
	Server string
	APIKey string
	Output string
	Deps   Deps
}

// NewCommand returns the root cobra.Command for the
// kscore-identity binary. Pass a zero Deps to use the production
// gRPC dialer.
func NewCommand(deps Deps) *cobra.Command {
	if deps.Dial == nil {
		deps.Dial = dialGRPC
	}
	g := &globals{Deps: deps}

	cmd := &cobra.Command{
		Use:   "kscore-identity",
		Short: "Keystone Core identity operator CLI",
		Long: "Operator surface for the Keystone Core identity provider — " +
			"mint join tokens, inspect + rotate the CA, query provider health.\n\n" +
			"All commands talk to the IdentityService gRPC on the running " +
			"kscore-server (default localhost:5397).",
	}
	cmd.PersistentFlags().StringVar(&g.Server, "server", "localhost:5397",
		"identity-service gRPC address (host:port)")
	cmd.PersistentFlags().StringVar(&g.APIKey, "api-key", "",
		"API key for authentication (or set KSCORE_API_KEY)")
	cmd.PersistentFlags().StringVarP(&g.Output, "output", "o", FormatTable,
		"output format: table | json")

	cmd.AddCommand(tokenCmd(g))
	cmd.AddCommand(caCmd(g))
	cmd.AddCommand(statusCmd(g))
	cli.AddVersion(cmd)

	return cmd
}
