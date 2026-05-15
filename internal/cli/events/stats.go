package events

import (
	"context"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

type statsOpts struct {
	since string
	until string
}

func statsCmd(g *globals) *cobra.Command {
	opts := &statsOpts{}
	cmd := &cobra.Command{
		Use:   "stats",
		Short: "Aggregate counts by event type + severity",
		Long: "Print total event count + per-type + per-severity breakdowns " +
			"for the given window. Both window bounds accept RFC3339 " +
			"timestamps or relative durations (1h, 30m).",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runStats(cmd.Context(), cmd.OutOrStdout(), g, opts)
		},
	}
	cmd.Flags().StringVar(&opts.since, "since", "", "lower bound (RFC3339 or duration)")
	cmd.Flags().StringVar(&opts.until, "until", "", "upper bound (RFC3339 or duration)")
	return cmd
}

func runStats(ctx context.Context, out io.Writer, g *globals, opts *statsOpts) error {
	if err := validateOutput(g.Output, false); err != nil {
		return err
	}
	now := time.Now()
	sinceT, err := parseRelativeDuration(opts.since, now)
	if err != nil {
		return fmt.Errorf("--since: %w", err)
	}
	untilT, err := parseRelativeDuration(opts.until, now)
	if err != nil {
		return fmt.Errorf("--until: %w", err)
	}
	req := &v1.GetEventStatsRequest{}
	if !sinceT.IsZero() {
		req.Since = timestamppb.New(sinceT)
	}
	if !untilT.IsZero() {
		req.Until = timestamppb.New(untilT)
	}

	client, closer, err := g.Deps.Dial(ctx, g.Server, g.APIKey)
	if err != nil {
		return err
	}
	defer func() { _ = closer.Close() }()

	resp, err := client.GetEventStats(authContext(ctx, g.APIKey), req)
	if err != nil {
		return fmt.Errorf("GetEventStats: %w", err)
	}

	switch g.Output {
	case FormatJSON:
		return writeJSON(out, resp)
	default:
		return printStatsTable(out, resp)
	}
}

func printStatsTable(out io.Writer, resp *v1.GetEventStatsResponse) error {
	_, _ = fmt.Fprintf(out, "total: %d\n\n", resp.GetTotal())
	if len(resp.GetByType()) > 0 {
		_, _ = fmt.Fprintln(out, "BY TYPE")
		printSortedCounts(out, resp.GetByType())
		_, _ = fmt.Fprintln(out)
	}
	if len(resp.GetBySeverity()) > 0 {
		_, _ = fmt.Fprintln(out, "BY SEVERITY")
		printSortedCounts(out, resp.GetBySeverity())
	}
	return nil
}

func printSortedCounts(out io.Writer, m map[string]int64) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	t := newTable(out)
	for _, k := range keys {
		t.row(k, fmt.Sprintf("%d", m[k]))
	}
	_ = t.flush()
}
