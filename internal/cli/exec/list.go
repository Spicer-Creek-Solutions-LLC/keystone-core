// SPDX-License-Identifier: Apache-2.0

package exec

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

func listCmd(g *globals) *cobra.Command {
	var (
		statusFlag string
		sinceFlag  time.Duration
		limit      int
		offset     int
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List batch jobs",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := authContext(cmd.Context(), g.APIKey)
			client, closer, err := g.Deps.Dial(ctx, g.Server, g.APIKey)
			if err != nil {
				return err
			}
			defer func() { _ = closer.Close() }()

			req := &v1.ListBatchJobsRequest{
				Limit:  int32(limit),
				Offset: int32(offset),
			}
			if statusFlag != "" {
				s, err := parseStatusFilter(statusFlag)
				if err != nil {
					return err
				}
				req.Status = s
			}
			if sinceFlag > 0 {
				req.Since = timestamppb.New(time.Now().Add(-sinceFlag))
			}

			resp, err := client.ListBatchJobs(ctx, req)
			if err != nil {
				return fmt.Errorf("exec list: %w", err)
			}
			return RenderBatchList(cmd.OutOrStdout(), resp.GetBatches(), g.Output)
		},
	}
	cmd.Flags().StringVar(&statusFlag, "status", "",
		"filter: pending | running | completed | failed | partial | cancelled")
	cmd.Flags().DurationVar(&sinceFlag, "since", 0,
		"only batches created within this duration (e.g. 24h)")
	cmd.Flags().IntVar(&limit, "limit", 50, "max rows to return")
	cmd.Flags().IntVar(&offset, "offset", 0, "skip the first N rows")
	return cmd
}

// parseStatusFilter maps a CLI status string to the proto enum.
func parseStatusFilter(s string) (v1.BatchJobStatus, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "pending":
		return v1.BatchJobStatus_BATCH_JOB_STATUS_PENDING, nil
	case "running":
		return v1.BatchJobStatus_BATCH_JOB_STATUS_RUNNING, nil
	case "completed":
		return v1.BatchJobStatus_BATCH_JOB_STATUS_COMPLETED, nil
	case "failed":
		return v1.BatchJobStatus_BATCH_JOB_STATUS_FAILED, nil
	case "partial":
		return v1.BatchJobStatus_BATCH_JOB_STATUS_PARTIAL, nil
	case "cancelled":
		return v1.BatchJobStatus_BATCH_JOB_STATUS_CANCELLED, nil
	}
	return v1.BatchJobStatus_BATCH_JOB_STATUS_UNSPECIFIED,
		fmt.Errorf("exec list: unknown --status %q", s)
}
