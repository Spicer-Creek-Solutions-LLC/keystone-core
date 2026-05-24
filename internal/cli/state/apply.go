// SPDX-License-Identifier: Apache-2.0

package state

import (
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

func applyCmd(g *globals) *cobra.Command {
	flags := &inputFlags{}
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "apply <file>",
		Short: "Apply a YAML state file",
		Long: "Compiles the YAML server-side, runs Check → Apply → Test per " +
			"declaration in topo order, and streams per-decl outcomes back. " +
			"--dry-run routes to the server's Check path (no Apply).",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runApply(cmd, args, g, flags, dryRun)
		},
	}
	registerInputFlags(cmd, flags)
	cmd.Flags().BoolVar(&dryRun, "dry-run", false,
		"route to server-side Check (no Apply); equivalent to `kscorectl state check`")
	return cmd
}

func runApply(cmd *cobra.Command, args []string, g *globals, flags *inputFlags, dryRun bool) error {
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
	req := &v1.ApplyStateRequest{
		YamlContent:       yaml,
		DryRun:            dryRun,
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

	stream, err := client.ApplyState(authContext(ctx, g.APIKey), req)
	if err != nil {
		return fmt.Errorf("apply: %w", err)
	}
	return drainApplyStream(stream, cmd.OutOrStdout(), g.Output)
}

// drainApplyStream is the receiver loop shared by `apply` and the
// drift `--fix` path. It pulls events off the stream, dispatches
// them through the format helpers, and propagates terminal failure
// status as an exit-code-1 error.
func drainApplyStream(stream v1.StateService_ApplyStateClient, out io.Writer, format string) error {
	var terminal *v1.StateRunTerminal
	for {
		msg, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("apply: stream recv: %w", err)
		}
		switch e := msg.Event.(type) {
		case *v1.ApplyStateResponse_RunId:
			if err := printApplyRunID(out, format, e.RunId); err != nil {
				return err
			}
		case *v1.ApplyStateResponse_DeclResult:
			if err := printApplyDecl(out, format, e.DeclResult); err != nil {
				return err
			}
		case *v1.ApplyStateResponse_Terminal:
			terminal = e.Terminal
			if err := printApplyTerminal(out, format, e.Terminal); err != nil {
				return err
			}
		}
	}
	if terminal != nil && terminal.Status == v1.StateRunStatus_STATE_RUN_STATUS_FAILED {
		return fmt.Errorf("apply: run %s failed", terminal.RunId)
	}
	return nil
}
