// SPDX-License-Identifier: Apache-2.0

package exec

import (
	"fmt"

	"github.com/spf13/cobra"

	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

func cancelCmd(g *globals) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cancel <batch-id>",
		Short: "Cancel an in-flight batch job",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := authContext(cmd.Context(), g.APIKey)
			client, closer, err := g.Deps.Dial(ctx, g.Server, g.APIKey)
			if err != nil {
				return err
			}
			defer func() { _ = closer.Close() }()

			if _, err := client.CancelBatchJob(ctx, &v1.CancelBatchJobRequest{BatchJobId: args[0]}); err != nil {
				return fmt.Errorf("exec cancel: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "cancelled: %s\n", args[0])
			return nil
		},
	}
	return cmd
}
