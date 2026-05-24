// SPDX-License-Identifier: Apache-2.0

// Package secrets implements the `kscore-secrets` subcommand tree
// (Epic 10 task 10). v1.0 ships:
//
//	get / put / delete / list
//	leases  {list, get, renew, revoke}
//	transit {encrypt, decrypt, sign, verify}
//
// All subcommands talk to the SecretsService gRPC hosted by
// kscore-server. There is no local-only mode — operations are
// always on the live broker so the CLI doesn't race the running
// server's in-memory state.
//
// Subcommand groups deferred to v1.x ROADMAP (no v1.0 gRPC RPC
// backing OR substantial complexity):
//
//	backends  — list configured backends + capabilities
//	audit     — query the audit log (Epic 12)
//	dynamic   — issue a dynamic secret
//	cache     — stats + operator-driven clear
//	template  — render a config template with secret placeholders
package secrets

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
	Dial func(ctx context.Context, target, apiKey string) (v1.SecretsServiceClient, io.Closer, error)
}

// globals are shared by every subcommand under `kscore-secrets`;
// populated from the parent command's persistent flags.
type globals struct {
	Server string
	APIKey string
	Output string
	Deps   Deps
}

// NewCommand returns the root cobra.Command for the
// kscore-secrets binary. Pass a zero Deps to use the production
// gRPC dialer.
func NewCommand(deps Deps) *cobra.Command {
	if deps.Dial == nil {
		deps.Dial = dialGRPC
	}
	g := &globals{Deps: deps}

	cmd := &cobra.Command{
		Use:   "kscore-secrets",
		Short: "Keystone Core secrets operator CLI",
		Long: "Operator surface for the Keystone Core secrets domain — " +
			"read / write KV secrets, manage leases for dynamic credentials, " +
			"and run transit encrypt/decrypt/sign/verify operations.\n\n" +
			"All commands talk to the SecretsService gRPC on the running " +
			"kscore-server (default localhost:5397).",
	}
	cmd.PersistentFlags().StringVar(&g.Server, "server", "localhost:5397",
		"secrets-service gRPC address (host:port)")
	cmd.PersistentFlags().StringVar(&g.APIKey, "api-key", "",
		"API key for authentication (or set KSCORE_API_KEY)")
	cmd.PersistentFlags().StringVarP(&g.Output, "output", "o", FormatTable,
		"output format: table | json")

	cmd.AddCommand(getCmd(g))
	cmd.AddCommand(putCmd(g))
	cmd.AddCommand(deleteCmd(g))
	cmd.AddCommand(listCmd(g))
	cmd.AddCommand(leasesCmd(g))
	cmd.AddCommand(transitCmd(g))
	cli.AddVersion(cmd)

	return cmd
}
