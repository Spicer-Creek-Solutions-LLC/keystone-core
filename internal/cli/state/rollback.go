// SPDX-License-Identifier: Apache-2.0

package state

import (
	"fmt"

	"github.com/spf13/cobra"
	"google.golang.org/grpc"

	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

// rollbackClientAdapter presents a RollbackStateResponse client
// stream as an ApplyStateResponse one (the two messages are
// shape-identical; RollbackStateResponse is distinct only to satisfy
// buf RPC_REQUEST_RESPONSE_UNIQUE) so the shared drainApplyStream
// loop is reused unchanged.
type rollbackClientAdapter struct {
	grpc.ServerStreamingClient[v1.RollbackStateResponse]
}

func (a rollbackClientAdapter) Recv() (*v1.ApplyStateResponse, error) {
	m, err := a.ServerStreamingClient.Recv()
	if err != nil {
		return nil, err
	}
	out := &v1.ApplyStateResponse{}
	switch e := m.GetEvent().(type) {
	case *v1.RollbackStateResponse_RunId:
		out.Event = &v1.ApplyStateResponse_RunId{RunId: e.RunId}
	case *v1.RollbackStateResponse_DeclResult:
		out.Event = &v1.ApplyStateResponse_DeclResult{DeclResult: e.DeclResult}
	case *v1.RollbackStateResponse_Terminal:
		out.Event = &v1.ApplyStateResponse_Terminal{Terminal: e.Terminal}
	}
	return out, nil
}

type rollbackFlags struct {
	DryRun  bool
	Source  string
	Agent   string
	Cluster string
}

func rollbackCmd(g *globals) *cobra.Command {
	flags := &rollbackFlags{}
	cmd := &cobra.Command{
		Use:   "rollback <run-id>",
		Short: "Re-apply a stored run's declarations",
		Long: "Reads the original run's persisted declarations server-side, " +
			"re-validates against the current Registry, and runs them through " +
			"the runner. New run_id; new history row. The runner is " +
			"idempotent so in-sync decls cost nothing to re-check.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRollback(cmd, args[0], g, flags)
		},
	}
	cmd.Flags().BoolVar(&flags.DryRun, "dry-run", false,
		"route to server-side Check (no Apply)")
	cmd.Flags().StringVar(&flags.Source, "source", "",
		"override the rollback's source label (default: rollback-of-<run-id>)")
	cmd.Flags().StringVar(&flags.Agent, "agent", "",
		"override target agent (default: original run's agent)")
	cmd.Flags().StringVar(&flags.Cluster, "cluster", "",
		"override cluster ID (default: original run's cluster)")
	return cmd
}

func runRollback(cmd *cobra.Command, runID string, g *globals, flags *rollbackFlags) error {
	ctx := cmd.Context()
	client, closer, err := g.Deps.Dial(ctx, g.Server, g.APIKey)
	if err != nil {
		return err
	}
	defer func() { _ = closer.Close() }()

	out := cmd.OutOrStdout()
	if g.Output != FormatJSON {
		fmt.Fprintf(out, "rollback of: %s\n", runID)
	}
	stream, err := client.RollbackState(authContext(ctx, g.APIKey), &v1.RollbackStateRequest{
		RunId:     runID,
		DryRun:    flags.DryRun,
		Source:    flags.Source,
		AgentId:   flags.Agent,
		ClusterId: flags.Cluster,
	})
	if err != nil {
		return fmt.Errorf("rollback: %w", err)
	}
	return drainApplyStream(rollbackClientAdapter{stream}, out, g.Output)
}
