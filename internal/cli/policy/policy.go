// Package policy implements the `kscore-policy` operator CLI per
// Epic 12 task 14. v1.0 ships:
//
//	list / show / compliance / violations   (remote — PolicyService gRPC)
//	eval / validate                          (local — in-process evaluators)
//
// kscore-policy is a HYBRID CLI. list/show/compliance/violations talk
// to the PolicyService gRPC on a running kscore-server (default
// `localhost:5397`; override via `--server host:port`; API-key auth
// via `--api-key` or `KSCORE_API_KEY`). eval/validate are policy-
// authoring tools that read a source file and run the OPA / CEL /
// Builtin evaluator in-process — no server, works in CI (matches
// PROJECT-DETAILS §4.12's validate/eval being authoring tools).
//
// Subcommands deferred to v1.x ROADMAP (fuzzy §4.12 spec, no
// acceptance criteria / no RPC):
//
//	check — fold of validate + dry-run; scope undefined for v1.0.
//	test  — policy unit-test harness; spec fuzzy, no acceptance line.
package policy

import (
	"context"
	"io"

	"github.com/spf13/cobra"

	"go.keystone-core.io/keystone-core/internal/cli"
	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

// Output format names. Non-streaming commands default to table;
// json is the scripting-friendly alternative.
const (
	FormatTable = "table"
	FormatJSON  = "json"
)

// Deps wires the gRPC client factory. Production uses [dialGRPC];
// tests inject a stub returning an in-process bufconn client.
type Deps struct {
	Dial func(ctx context.Context, target, apiKey string) (v1.PolicyServiceClient, io.Closer, error)
}

// globals are shared by every subcommand; populated from the root
// command's persistent flags.
type globals struct {
	Server string
	APIKey string
	Output string
	Deps   Deps
}

// NewCommand returns the root cobra.Command for the kscore-policy
// binary. Pass a zero Deps to use the production gRPC dialer.
func NewCommand(deps Deps) *cobra.Command {
	if deps.Dial == nil {
		deps.Dial = dialGRPC
	}
	g := &globals{Deps: deps}

	cmd := &cobra.Command{
		Use:   "kscore-policy",
		Short: "Keystone Core policy operator CLI",
		Long: "Operator + authoring surface for the Keystone Core policy " +
			"engine.\n\nlist / show / compliance / violations talk to the " +
			"PolicyService gRPC on a running kscore-server (default " +
			"localhost:5397). eval / validate run the evaluator in-process " +
			"against a policy source file — no server required.\n\n" +
			"v1.0 is audit-mode: policies evaluate + record but never block.",
	}
	cmd.PersistentFlags().StringVar(&g.Server, "server", "localhost:5397",
		"policy-service gRPC address (host:port) — used by remote subcommands")
	cmd.PersistentFlags().StringVar(&g.APIKey, "api-key", "",
		"API key for authentication (or set KSCORE_API_KEY)")
	cmd.PersistentFlags().StringVarP(&g.Output, "output", "o", FormatTable,
		"output format: table | json")

	cmd.AddCommand(listCmd(g))
	cmd.AddCommand(showCmd(g))
	cmd.AddCommand(complianceCmd(g))
	cmd.AddCommand(violationsCmd(g))
	cmd.AddCommand(evalCmd(g))
	cmd.AddCommand(validateCmd(g))
	cli.AddVersion(cmd)

	return cmd
}
