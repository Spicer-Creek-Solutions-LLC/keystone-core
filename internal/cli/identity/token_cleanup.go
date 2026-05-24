// SPDX-License-Identifier: Apache-2.0

package identity

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

func tokenCleanupCmd(g *globals) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cleanup",
		Short: "Delete every expired join token (ad-hoc; the hourly loop does this automatically)",
		Long: "Runs CleanupJoinTokens(now) once on the server. The background " +
			"cleanup loop (Epic 09 task 11) handles this hourly; this command " +
			"exists for operator-driven sweeps after a long pause or for " +
			"emergency garbage collection.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runTokenCleanup(cmd.Context(), cmd.OutOrStdout(), g)
		},
	}
	return cmd
}

func runTokenCleanup(ctx context.Context, out io.Writer, g *globals) error {
	if err := validateOutput(g.Output); err != nil {
		return err
	}
	client, closer, err := g.Deps.Dial(ctx, g.Server, g.APIKey)
	if err != nil {
		return err
	}
	defer func() { _ = closer.Close() }()

	resp, err := client.CleanupJoinTokens(authContext(ctx, g.APIKey), &v1.CleanupJoinTokensRequest{})
	if err != nil {
		return fmt.Errorf("CleanupJoinTokens: %w", err)
	}

	switch g.Output {
	case FormatJSON:
		return writeJSON(out, resp)
	default:
		_, _ = fmt.Fprintf(out, "removed %d expired join token(s)\n", resp.GetRemoved())
		return nil
	}
}
