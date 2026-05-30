// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

func listCmd(g *globals) *cobra.Command {
	var (
		statusFlag string
		labelFlag  string
		limit      int
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List registered agents",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := validateOutput(g.Output); err != nil {
				return err
			}
			req := &v1.ListAgentsRequest{PageSize: int32(limit)}
			if statusFlag != "" {
				s, err := parseStatusFilter(statusFlag)
				if err != nil {
					return err
				}
				req.Status = s
			}
			if labelFlag != "" {
				k, v, err := parseLabelFilter(labelFlag)
				if err != nil {
					return err
				}
				req.LabelKey = k
				req.LabelValue = v
			}

			ctx := authContext(cmd.Context(), g.APIKey)
			client, closer, err := g.Deps.Dial(ctx, g.Server, g.APIKey)
			if err != nil {
				return err
			}
			defer func() { _ = closer.Close() }()

			resp, err := client.ListAgents(ctx, req)
			if err != nil {
				return fmt.Errorf("agent list: %w", err)
			}
			return renderAgentList(cmd.OutOrStdout(), resp, g.Output)
		},
	}
	cmd.Flags().StringVar(&statusFlag, "status", "",
		"filter: pending | connected | stale | disabled")
	cmd.Flags().StringVar(&labelFlag, "label", "",
		"filter by single label, format key=value")
	cmd.Flags().IntVar(&limit, "limit", 50, "max rows to return (PageSize)")
	return cmd
}

// parseStatusFilter maps a CLI status string to the AgentStatus enum.
func parseStatusFilter(s string) (v1.AgentStatus, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "pending":
		return v1.AgentStatus_AGENT_STATUS_PENDING, nil
	case "connected":
		return v1.AgentStatus_AGENT_STATUS_CONNECTED, nil
	case "stale":
		return v1.AgentStatus_AGENT_STATUS_STALE, nil
	case "disabled":
		return v1.AgentStatus_AGENT_STATUS_DISABLED, nil
	}
	return v1.AgentStatus_AGENT_STATUS_UNSPECIFIED,
		fmt.Errorf("agent list: unknown --status %q", s)
}

// parseLabelFilter splits a "key=value" pair into its two halves.
// Empty key or value is rejected so the server isn't sent an
// ambiguous half-filter.
func parseLabelFilter(s string) (string, string, error) {
	k, v, ok := strings.Cut(s, "=")
	if !ok || k == "" || v == "" {
		return "", "", fmt.Errorf("agent list: --label must be key=value, got %q", s)
	}
	return k, v, nil
}
