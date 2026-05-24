// SPDX-License-Identifier: Apache-2.0

package exec

import (
	"fmt"

	"github.com/spf13/cobra"

	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

func outputCmd(g *globals) *cobra.Command {
	var (
		agentFlag  string
		stderrFlag bool
		bothFlag   bool
		rawFlag    bool
	)
	cmd := &cobra.Command{
		Use:   "output <batch-id>",
		Short: "Show captured stdout/stderr for a batch job",
		Long: "Renders per-agent output. Default: human-readable framed " +
			"sections (\"=== agent <id> stdout ===\"). With --raw and a " +
			"single agent, writes raw bytes to stdout (pipe-friendly).",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			batchID := args[0]
			ctx := authContext(cmd.Context(), g.APIKey)
			client, closer, err := g.Deps.Dial(ctx, g.Server, g.APIKey)
			if err != nil {
				return err
			}
			defer func() { _ = closer.Close() }()

			opts := OutputRenderOpts{
				IncludeStdout: !stderrFlag || bothFlag,
				IncludeStderr: stderrFlag || bothFlag,
				Raw:           rawFlag,
				Format:        g.Output,
			}
			if rawFlag && opts.IncludeStdout && opts.IncludeStderr {
				return fmt.Errorf("exec output: --raw is incompatible with both streams (use --stderr or omit --all)")
			}

			if agentFlag != "" {
				resp, err := client.GetBatchAgentResult(ctx, &v1.GetBatchAgentResultRequest{
					BatchJobId: batchID,
					AgentId:    agentFlag,
				})
				if err != nil {
					return fmt.Errorf("exec output: %w", err)
				}
				return RenderAgentResults(cmd.OutOrStdout(),
					[]*v1.BatchAgentResult{resp.GetResult()}, opts)
			}

			resp, err := client.ListBatchAgentResults(ctx, &v1.ListBatchAgentResultsRequest{BatchJobId: batchID})
			if err != nil {
				return fmt.Errorf("exec output: %w", err)
			}
			results := resp.GetResults()
			if rawFlag && len(results) > 1 {
				return fmt.Errorf("exec output: --raw requires a single agent (got %d); pass --agent <id>", len(results))
			}
			return RenderAgentResults(cmd.OutOrStdout(), results, opts)
		},
	}
	cmd.Flags().StringVar(&agentFlag, "agent", "", "only show output for this agent ID")
	cmd.Flags().BoolVar(&stderrFlag, "stderr", false, "show stderr instead of stdout")
	cmd.Flags().BoolVar(&bothFlag, "all", false, "show both stdout and stderr (framed only)")
	cmd.Flags().BoolVar(&rawFlag, "raw", false,
		"write raw bytes to stdout (single-agent only; pipe-friendly)")
	return cmd
}
