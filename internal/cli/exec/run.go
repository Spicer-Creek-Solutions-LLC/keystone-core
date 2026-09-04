// SPDX-License-Identifier: Apache-2.0

package exec

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

func runCmd(g *globals) *cobra.Command {
	var flags dispatchFlags
	cmd := &cobra.Command{
		Use:   "run <command> [args...]",
		Short: "Dispatch a command to agents and stream results",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDispatch(cmd, g, &flags, args[0], args[1:], false)
		},
	}
	bindDispatchFlags(cmd.Flags(), &flags)
	return cmd
}

func asyncCmd(g *globals) *cobra.Command {
	var flags dispatchFlags
	cmd := &cobra.Command{
		Use:   "async <command> [args...]",
		Short: "Dispatch a command and return the batch job ID without streaming",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDispatch(cmd, g, &flags, args[0], args[1:], true)
		},
	}
	bindDispatchFlags(cmd.Flags(), &flags)
	return cmd
}

// runDispatch wires both `run` and `async`: builds the
// BatchExecuteCommandRequest, opens the stream, and either streams to
// terminal (run) or disconnects after the batch_job_id event (async).
func runDispatch(cmd *cobra.Command, g *globals, flags *dispatchFlags, command string, args []string, asyncMode bool) error {
	target, err := ParseTarget(flags.Target)
	if err != nil {
		return err
	}
	if target == nil {
		return fmt.Errorf("exec: --target is required")
	}
	env, err := envMap(flags.Env)
	if err != nil {
		return err
	}

	ctx := authContext(cmd.Context(), g.APIKey)
	client, closer, err := g.Deps.Dial(ctx, g.Server, g.APIKey)
	if err != nil {
		return err
	}
	defer func() { _ = closer.Close() }()

	req := &v1.BatchExecuteCommandRequest{
		Target:            target,
		Command:           command,
		Args:              args,
		Env:               env,
		WorkingDir:        flags.WorkingDir,
		User:              flags.User,
		TimeoutSeconds:    int32(flags.CommandTimeout.Seconds()),
		Concurrency:       int32(flags.Concurrency),
		ContinueOnFailure: flags.ContinueOnFailure,
		DryRun:            flags.DryRun,
	}
	if flags.Shell != "" {
		req.Command, req.Args = wrapWithShell(flags.Shell, command, args)
	}

	stream, err := client.BatchExecuteCommand(ctx, req)
	if err != nil {
		return fmt.Errorf("exec: BatchExecuteCommand: %w", err)
	}

	if flags.DryRun {
		return RenderPreviewStream(cmd.OutOrStdout(), stream.Recv, g.Output)
	}

	if asyncMode {
		// Consume only until we see the batch_job_id event, print it,
		// and bail. The server keeps running the batch.
		for {
			ev, err := stream.Recv()
			if err != nil {
				return fmt.Errorf("exec: async recv: %w", err)
			}
			if id := ev.GetBatchJobId(); id != "" {
				fmt.Fprintln(cmd.OutOrStdout(), id)
				return nil
			}
		}
	}

	r := NewBatchStreamRenderer(cmd.OutOrStdout(), g.Output).WithOutput(flags.ShowOutput)
	if err := r.Render(stream.Recv); err != nil {
		return err
	}
	if !flags.ShowOutput {
		return nil
	}
	// The batch stream does not carry per-agent output — the server
	// stores it and serves it from ListBatchAgentResults, which is what
	// `exec output <batch-id>` calls. Without this an operator has to
	// scrape the batch id out of the run and make a second call just to
	// see what the command printed, which is the common case.
	return renderBatchOutput(ctx, client, cmd.OutOrStdout(), r.BatchID(), g.Output)
}

// wrapWithShell returns (shellBinary, [args...]) for the given shell
// flag (bash | sh | powershell | cmd). The command + original args
// are space-joined into a single string passed via `-c` / `/c` /
// `-Command`.
func wrapWithShell(shell, command string, originalArgs []string) (string, []string) {
	full := command
	for _, a := range originalArgs {
		full += " " + a
	}
	switch shell {
	case "bash":
		return "bash", []string{"-c", full}
	case "sh":
		return "sh", []string{"-c", full}
	case "powershell", "pwsh":
		return "powershell", []string{"-NoProfile", "-Command", full}
	case "cmd":
		return "cmd", []string{"/c", full}
	default:
		// Unknown shell: pass through unchanged. The server's
		// CommandPolicy validation will reject if needed.
		return command, originalArgs
	}
}

// renderBatchOutput fetches and prints every agent's captured streams
// for a finished batch. Shares RenderAgentResults with `exec output`
// so the two render identically.
func renderBatchOutput(ctx context.Context, client v1.ControlPlaneServiceClient,
	out io.Writer, batchID, format string) error {
	if batchID == "" {
		return nil
	}
	resp, err := client.ListBatchAgentResults(ctx, &v1.ListBatchAgentResultsRequest{BatchJobId: batchID})
	if err != nil {
		return fmt.Errorf("exec run: fetch output: %w", err)
	}
	return RenderAgentResults(out, resp.GetResults(), OutputRenderOpts{
		IncludeStdout: true,
		Format:        format,
	})
}
