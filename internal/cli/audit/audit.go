// Package audit implements the `kscore-audit` operator CLI per
// Epic 12 task 14. v1.0 ships:
//
//	log    — paginated audit-log query (GetAuditLog)
//	report — compliance report (GetComplianceReport)
//	stats  — headline counts over a window (GetComplianceReport)
//	export — stream the audit log as JSON / JSONL / CSV with
//	         redaction applied on export (Epic 12 task 15)
//
// All subcommands talk to the PolicyService gRPC on a running
// kscore-server (default `localhost:5397`; override via
// `--server host:port`). API-key auth via `--api-key` or
// `KSCORE_API_KEY`.
//
// Deferred to v1.x ROADMAP:
//
//	search   — subsumed by `log` filters.
//	analyze  — fuzzy §4.12 spec, no acceptance criteria (v1.x ROADMAP).
//	timeline — needs a ResourceAuditTrail RPC not on PolicyService (v1.x ROADMAP).
//	watch    — needs an audit-tail streaming RPC + BufferedAuditor
//	           exposure; neither exists in v1.0 (v1.x ROADMAP).
package audit

import (
	"context"
	"io"

	"github.com/spf13/cobra"

	"go.keystone-core.io/keystone-core/internal/cli"
	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

const (
	FormatTable = "table"
	FormatJSON  = "json"
)

// Deps wires the gRPC client factory. Production uses [dialGRPC];
// tests inject a bufconn stub.
type Deps struct {
	Dial func(ctx context.Context, target, apiKey string) (v1.PolicyServiceClient, io.Closer, error)
}

type globals struct {
	Server string
	APIKey string
	Output string
	Deps   Deps
}

// NewCommand returns the root cobra.Command for kscore-audit. Pass
// a zero Deps for the production dialer.
func NewCommand(deps Deps) *cobra.Command {
	if deps.Dial == nil {
		deps.Dial = dialGRPC
	}
	g := &globals{Deps: deps}

	cmd := &cobra.Command{
		Use:   "kscore-audit",
		Short: "Keystone Core audit-log operator CLI",
		Long: "Query the Keystone Core audit log + compliance roll-ups.\n\n" +
			"log / report / stats talk to the PolicyService gRPC on a " +
			"running kscore-server (default localhost:5397). Audit-mode " +
			"v1.0: policy evaluations are recorded but never block.",
	}
	cmd.PersistentFlags().StringVar(&g.Server, "server", "localhost:5397",
		"policy-service gRPC address (host:port)")
	cmd.PersistentFlags().StringVar(&g.APIKey, "api-key", "",
		"API key for authentication (or set KSCORE_API_KEY)")
	cmd.PersistentFlags().StringVarP(&g.Output, "output", "o", FormatTable,
		"output format: table | json")

	cmd.AddCommand(logCmd(g))
	cmd.AddCommand(reportCmd(g))
	cmd.AddCommand(statsCmd(g))
	cmd.AddCommand(exportCmd(g))
	cli.AddVersion(cmd)

	return cmd
}
