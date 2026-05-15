package events

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

func typesCmd(g *globals) *cobra.Command {
	return &cobra.Command{
		Use:   "types",
		Short: "List the 22 canonical v1.0 event types",
		Long: "Print every canonical event type the server advertises via " +
			"GetEventTypes. Operator-defined subtypes within a known " +
			"category are accepted by the server but NOT included here.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runTypes(cmd.Context(), cmd.OutOrStdout(), g)
		},
	}
}

func runTypes(ctx context.Context, out io.Writer, g *globals) error {
	if err := validateOutput(g.Output, false); err != nil {
		return err
	}
	client, closer, err := g.Deps.Dial(ctx, g.Server, g.APIKey)
	if err != nil {
		return err
	}
	defer func() { _ = closer.Close() }()

	resp, err := client.GetEventTypes(authContext(ctx, g.APIKey), &v1.GetEventTypesRequest{})
	if err != nil {
		return fmt.Errorf("GetEventTypes: %w", err)
	}

	switch g.Output {
	case FormatJSON:
		return writeJSON(out, resp)
	default:
		t := newTable(out)
		t.header("TYPE")
		for _, typ := range resp.GetTypes() {
			t.row(typ)
		}
		return t.flush()
	}
}
