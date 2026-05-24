// SPDX-License-Identifier: Apache-2.0

package state

import (
	"fmt"

	"github.com/spf13/cobra"

	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

func checkCmd(g *globals) *cobra.Command {
	flags := &inputFlags{}
	cmd := &cobra.Command{
		Use:   "check <file>",
		Short: "Compile + run Check phase only (no Apply)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCheck(cmd, args, g, flags)
		},
	}
	registerInputFlags(cmd, flags)
	return cmd
}

func runCheck(cmd *cobra.Command, args []string, g *globals, flags *inputFlags) error {
	ctx := cmd.Context()
	yaml, defaultSource, err := readInputYAML(args)
	if err != nil {
		return err
	}
	vars, err := parseKeyValues(flags.Variables)
	if err != nil {
		return fmt.Errorf("--variable: %w", err)
	}
	facts, err := parseKeyValues(flags.Facts)
	if err != nil {
		return fmt.Errorf("--fact: %w", err)
	}
	req := &v1.CheckStateRequest{
		YamlContent:       yaml,
		Facts:             facts,
		VariableOverrides: vars,
		Source:            resolveSource(flags.Source, defaultSource),
		ClusterId:         flags.Cluster,
		AgentId:           flags.Agent,
	}

	client, closer, err := g.Deps.Dial(ctx, g.Server, g.APIKey)
	if err != nil {
		return err
	}
	defer func() { _ = closer.Close() }()

	resp, err := client.CheckState(authContext(ctx, g.APIKey), req)
	if err != nil {
		return fmt.Errorf("check: %w", err)
	}
	if err := printCheck(cmd.OutOrStdout(), g.Output, resp); err != nil {
		return err
	}
	if resp.Status == v1.StateRunStatus_STATE_RUN_STATUS_FAILED {
		return fmt.Errorf("check: run %s failed", resp.RunId)
	}
	return nil
}
