// Package state implements the `kscorectl state` subcommand tree
// (Epic 08 task 10a). v1.0 ships five subcommands:
//
//	apply    — stream ApplyState; per-decl outcomes + terminal summary
//	check    — unary CheckState; dry-run with table of declarations
//	drift    — DetectDrift; with --fix, re-applies the same YAML
//	compile  — local Parse → Render → Validate → Resolve; no server call
//	vars get — local compile; print merged Variables map
//
// History / show / rollback land in 10b. The CLI talks to the gRPC
// StateService directly; the local subcommands (compile, vars) share
// the engine code from internal/statemgmt so behaviour is identical
// between server-side compilation and the client-side dry-run.
package state

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
	Dial func(ctx context.Context, target, apiKey string) (v1.StateServiceClient, io.Closer, error)
}

// globals are shared by every subcommand under state; populated from
// the parent command's persistent flags.
type globals struct {
	Server string
	APIKey string
	Output string
	Deps   Deps
}

// NewCommand returns the `state` parent command with subcommands
// attached. Pass an empty Deps to use the production gRPC dialer.
func NewCommand(deps Deps) *cobra.Command {
	if deps.Dial == nil {
		deps.Dial = dialGRPC
	}
	g := &globals{Deps: deps}

	cmd := &cobra.Command{
		Use:   "state",
		Short: "Declarative state management (apply / check / drift / compile)",
		Long: "Compile and apply YAML state files via the control plane's StateService. " +
			"Local subcommands (compile, vars) run the engine client-side without " +
			"contacting the server.",
	}
	cmd.PersistentFlags().StringVar(&g.Server, "server", "localhost:9090",
		"control-plane gRPC address (host:port)")
	cmd.PersistentFlags().StringVar(&g.APIKey, "api-key", "",
		"API key for authentication (or set KSCORE_API_KEY)")
	cmd.PersistentFlags().StringVarP(&g.Output, "output", "o", FormatTable,
		"output format: table | json")

	cmd.AddCommand(applyCmd(g))
	cmd.AddCommand(checkCmd(g))
	cmd.AddCommand(driftCmd(g))
	cmd.AddCommand(compileCmd(g))
	cmd.AddCommand(varsCmd(g))

	return cmd
}
