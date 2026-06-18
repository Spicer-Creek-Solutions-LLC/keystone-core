// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

func verifyCmd(g *globals) *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "verify [agent-id]",
		Short: "Verify an agent's certificate against the trust bundle",
		Long: "Re-checks the agent's stored certificate against the control " +
			"plane's current trust bundle: chains to a trusted authority, " +
			"within its validity window, and carries a matching agent SPIFFE " +
			"identity. Pass an agent-id, or --all to sweep the fleet. Exits " +
			"non-zero if any agent with a stored cert fails verification.",
		Args:          cobra.MaximumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOutput(g.Output); err != nil {
				return err
			}
			switch {
			case all && len(args) > 0:
				return fmt.Errorf("agent verify: pass an agent-id or --all, not both")
			case !all && len(args) == 0:
				return fmt.Errorf("agent verify: provide an agent-id or --all")
			}

			ctx := authContext(cmd.Context(), g.APIKey)
			client, closer, err := g.Deps.Dial(ctx, g.Server, g.APIKey)
			if err != nil {
				return err
			}
			defer func() { _ = closer.Close() }()

			var ids []string
			if all {
				list, err := client.ListAgents(ctx, &v1.ListAgentsRequest{})
				if err != nil {
					return fmt.Errorf("agent verify: list agents: %w", err)
				}
				for _, a := range list.GetAgents() {
					ids = append(ids, a.GetId())
				}
			} else {
				ids = []string{args[0]}
			}

			results := make([]*v1.VerifyAgentResponse, 0, len(ids))
			for _, id := range ids {
				r, err := client.VerifyAgent(ctx, &v1.VerifyAgentRequest{AgentId: id})
				if err != nil {
					return fmt.Errorf("agent verify %s: %w", id, err)
				}
				results = append(results, r)
			}
			if err := renderVerify(cmd.OutOrStdout(), results, g.Output); err != nil {
				return err
			}
			// Non-zero exit when a verifiable agent failed (cert present
			// but not OK). has_cert=false agents are skipped, not failures.
			failed := 0
			for _, r := range results {
				if r.GetHasCert() && !r.GetOk() {
					failed++
				}
			}
			if failed > 0 {
				return fmt.Errorf("%d agent(s) failed certificate verification", failed)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "verify every registered agent")
	return cmd
}

func renderVerify(w io.Writer, results []*v1.VerifyAgentResponse, format string) error {
	if format == FormatJSON {
		out := make([]map[string]any, 0, len(results))
		for _, r := range results {
			m := map[string]any{
				"agent_id":      r.GetAgentId(),
				"has_cert":      r.GetHasCert(),
				"ok":            r.GetOk(),
				"chain_valid":   r.GetChainValid(),
				"expired":       r.GetExpired(),
				"not_yet_valid": r.GetNotYetValid(),
				"spiffe_match":  r.GetSpiffeMatch(),
				"spiffe_id":     r.GetSpiffeId(),
			}
			if r.GetExpiresAt() != nil {
				m["expires_at"] = r.GetExpiresAt().AsTime().UTC().Format(time.RFC3339)
			}
			out = append(out, m)
		}
		return json.NewEncoder(w).Encode(out)
	}

	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "AGENT-ID\tOK\tHAS-CERT\tCHAIN\tEXPIRED\tSPIFFE-MATCH\tEXPIRES\tSPIFFE-ID")
	for _, r := range results {
		expires := "-"
		if r.GetExpiresAt() != nil {
			expires = r.GetExpiresAt().AsTime().UTC().Format(time.RFC3339)
		}
		fmt.Fprintf(tw, "%s\t%v\t%v\t%v\t%v\t%v\t%s\t%s\n",
			r.GetAgentId(), r.GetOk(), r.GetHasCert(), r.GetChainValid(),
			r.GetExpired(), r.GetSpiffeMatch(), expires, r.GetSpiffeId())
	}
	return tw.Flush()
}
