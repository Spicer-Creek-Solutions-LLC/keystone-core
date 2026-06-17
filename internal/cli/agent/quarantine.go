// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

func quarantineCmd(g *globals) *cobra.Command {
	var reason string
	cmd := &cobra.Command{
		Use:   "quarantine <agent-id>",
		Short: "Isolate an agent (reject its heartbeats, dispatch no commands)",
		Long: "Transitions the agent to DISABLED so the control plane rejects " +
			"its heartbeats and dispatches no commands to it — the incident-" +
			"response isolation step. Reverse with `unquarantine`.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOutput(g.Output); err != nil {
				return err
			}
			ctx := authContext(cmd.Context(), g.APIKey)
			client, closer, err := g.Deps.Dial(ctx, g.Server, g.APIKey)
			if err != nil {
				return err
			}
			defer func() { _ = closer.Close() }()

			resp, err := client.QuarantineAgent(ctx, &v1.QuarantineAgentRequest{
				AgentId: args[0],
				Reason:  reason,
			})
			if err != nil {
				return fmt.Errorf("agent quarantine: %w", err)
			}
			return renderQuarantine(cmd.OutOrStdout(), "quarantined", resp.GetAgentId(), resp.GetStatus(), g.Output)
		},
	}
	cmd.Flags().StringVar(&reason, "reason", "", "optional note recorded with the action")
	return cmd
}

func unquarantineCmd(g *globals) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unquarantine <agent-id>",
		Short: "Re-admit a quarantined agent",
		Long: "Reverses `quarantine`: returns the agent to PENDING so its next " +
			"heartbeat promotes it back to CONNECTED.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOutput(g.Output); err != nil {
				return err
			}
			ctx := authContext(cmd.Context(), g.APIKey)
			client, closer, err := g.Deps.Dial(ctx, g.Server, g.APIKey)
			if err != nil {
				return err
			}
			defer func() { _ = closer.Close() }()

			resp, err := client.UnquarantineAgent(ctx, &v1.UnquarantineAgentRequest{
				AgentId: args[0],
			})
			if err != nil {
				return fmt.Errorf("agent unquarantine: %w", err)
			}
			return renderQuarantine(cmd.OutOrStdout(), "unquarantined", resp.GetAgentId(), resp.GetStatus(), g.Output)
		},
	}
	return cmd
}

// renderQuarantine prints the action result as a human line or a JSON
// object, matching the agent command's --output flag.
func renderQuarantine(w io.Writer, action, id string, st v1.AgentStatus, format string) error {
	if format == FormatJSON {
		return json.NewEncoder(w).Encode(map[string]string{
			"agent_id": id,
			"action":   action,
			"status":   statusName(st),
		})
	}
	_, err := fmt.Fprintf(w, "agent %q %s (status: %s)\n", id, action, statusName(st))
	return err
}
