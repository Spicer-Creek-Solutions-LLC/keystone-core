// SPDX-License-Identifier: Apache-2.0

package state

import (
	"fmt"

	"github.com/spf13/cobra"

	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

func showCmd(g *globals) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <run-id>",
		Short: "Show details of a stored state run",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runShow(cmd, args[0], g)
		},
	}
	return cmd
}

func runShow(cmd *cobra.Command, runID string, g *globals) error {
	ctx := cmd.Context()
	client, closer, err := g.Deps.Dial(ctx, g.Server, g.APIKey)
	if err != nil {
		return err
	}
	defer func() { _ = closer.Close() }()

	resp, err := client.GetStateStatus(authContext(ctx, g.APIKey), &v1.GetStateStatusRequest{RunId: runID})
	if err != nil {
		return fmt.Errorf("show: %w", err)
	}
	return printShow(cmd.OutOrStdout(), g.Output, resp)
}
