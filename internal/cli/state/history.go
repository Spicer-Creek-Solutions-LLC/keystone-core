package state

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

type historyFlags struct {
	Agent  string
	Mode   string
	Status string
	Since  string
	Until  string
	Limit  int32
	Offset int32
}

func historyCmd(g *globals) *cobra.Command {
	flags := &historyFlags{Limit: 50}
	cmd := &cobra.Command{
		Use:   "history",
		Short: "List past state runs",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runHistory(cmd, g, flags)
		},
	}
	cmd.Flags().StringVar(&flags.Agent, "agent", "", "filter by agent ID")
	cmd.Flags().StringVar(&flags.Mode, "mode", "", "filter by mode (apply | check | drift)")
	cmd.Flags().StringVar(&flags.Status, "status", "", "filter by status (running | completed | failed | cancelled)")
	cmd.Flags().StringVar(&flags.Since, "since", "", "lower bound: duration back from now (2h, 30m) or RFC3339 timestamp")
	cmd.Flags().StringVar(&flags.Until, "until", "", "upper bound: duration back from now (2h) or RFC3339 timestamp")
	cmd.Flags().Int32Var(&flags.Limit, "limit", 50, "maximum rows")
	cmd.Flags().Int32Var(&flags.Offset, "offset", 0, "pagination offset")
	return cmd
}

func runHistory(cmd *cobra.Command, g *globals, flags *historyFlags) error {
	ctx := cmd.Context()
	mode, err := parseModeFlag(flags.Mode)
	if err != nil {
		return err
	}
	status, err := parseStatusFlag(flags.Status)
	if err != nil {
		return err
	}
	since, err := parseTimeBound(flags.Since)
	if err != nil {
		return fmt.Errorf("--since: %w", err)
	}
	until, err := parseTimeBound(flags.Until)
	if err != nil {
		return fmt.Errorf("--until: %w", err)
	}
	req := &v1.GetStateHistoryRequest{
		AgentId:    flags.Agent,
		Mode:       mode,
		Status:     status,
		PageSize:   flags.Limit,
		PageOffset: flags.Offset,
	}
	if !since.IsZero() {
		req.Since = timestamppb.New(since)
	}
	if !until.IsZero() {
		req.Until = timestamppb.New(until)
	}

	client, closer, err := g.Deps.Dial(ctx, g.Server, g.APIKey)
	if err != nil {
		return err
	}
	defer func() { _ = closer.Close() }()

	resp, err := client.GetStateHistory(authContext(ctx, g.APIKey), req)
	if err != nil {
		return fmt.Errorf("history: %w", err)
	}
	return printHistory(cmd.OutOrStdout(), g.Output, resp.GetRuns())
}

func parseModeFlag(s string) (v1.StateRunMode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "":
		return v1.StateRunMode_STATE_RUN_MODE_UNSPECIFIED, nil
	case "apply":
		return v1.StateRunMode_STATE_RUN_MODE_APPLY, nil
	case "check":
		return v1.StateRunMode_STATE_RUN_MODE_CHECK, nil
	case "drift":
		return v1.StateRunMode_STATE_RUN_MODE_DRIFT, nil
	default:
		return 0, fmt.Errorf("--mode: expected apply | check | drift; got %q", s)
	}
}

func parseStatusFlag(s string) (v1.StateRunStatus, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "":
		return v1.StateRunStatus_STATE_RUN_STATUS_UNSPECIFIED, nil
	case "running":
		return v1.StateRunStatus_STATE_RUN_STATUS_RUNNING, nil
	case "completed":
		return v1.StateRunStatus_STATE_RUN_STATUS_COMPLETED, nil
	case "failed":
		return v1.StateRunStatus_STATE_RUN_STATUS_FAILED, nil
	case "cancelled":
		return v1.StateRunStatus_STATE_RUN_STATUS_CANCELLED, nil
	default:
		return 0, fmt.Errorf("--status: expected running | completed | failed | cancelled; got %q", s)
	}
}
