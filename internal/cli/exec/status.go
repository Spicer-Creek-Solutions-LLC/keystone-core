// SPDX-License-Identifier: Apache-2.0

package exec

import (
	"fmt"

	"github.com/spf13/cobra"

	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

func statusCmd(g *globals) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status <batch-id>",
		Short: "Show the current state of a batch job",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := authContext(cmd.Context(), g.APIKey)
			client, closer, err := g.Deps.Dial(ctx, g.Server, g.APIKey)
			if err != nil {
				return err
			}
			defer func() { _ = closer.Close() }()

			resp, err := client.GetBatchJob(ctx, &v1.GetBatchJobRequest{BatchJobId: args[0]})
			if err != nil {
				return fmt.Errorf("exec status: %w", err)
			}
			return RenderBatchJob(cmd.OutOrStdout(), resp.GetBatch(), g.Output)
		},
	}
	return cmd
}
