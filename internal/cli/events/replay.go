package events

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

type replayOpts struct {
	since         string
	until         string
	eventType     string
	category      string
	source        string
	minSeverity   string
	correlationID string
	tags          []string
	pageSize      int
}

func replayCmd(g *globals) *cobra.Command {
	opts := &replayOpts{}
	cmd := &cobra.Command{
		Use:   "replay",
		Short: "Historical-only playback (no live tail)",
		Long: "Iterate the EventStore for events in [since, until), emitting " +
			"one JSON line per event. Exits when the window is exhausted. " +
			"Useful for forensic / audit work; for ongoing tailing use " +
			"`subscribe` or `watch`.\n\n--since is required.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			explicit := cmd.Flags().Changed("output")
			return runReplay(cmd.Context(), cmd.OutOrStdout(), g, opts, explicit)
		},
	}
	cmd.Flags().StringVar(&opts.since, "since", "", "lower bound (RFC3339 or duration like 1h); REQUIRED")
	cmd.Flags().StringVar(&opts.until, "until", "", "upper bound (RFC3339 or duration like 30m); defaults to now")
	cmd.Flags().StringVar(&opts.eventType, "type", "", "narrow: exact event type")
	cmd.Flags().StringVar(&opts.category, "category", "", "narrow: category fan-in")
	cmd.Flags().StringVar(&opts.source, "source", "", "narrow: exact source")
	cmd.Flags().StringVar(&opts.minSeverity, "min-severity", "", "narrow: minimum severity")
	cmd.Flags().StringVar(&opts.correlationID, "correlation-id", "", "narrow: exact correlation id")
	cmd.Flags().StringSliceVar(&opts.tags, "tag", nil, "narrow: tag key=value (repeatable, ANDed)")
	cmd.Flags().IntVar(&opts.pageSize, "page-size", 200, "events per ListEvents round-trip")
	_ = cmd.MarkFlagRequired("since")
	return cmd
}

func runReplay(ctx context.Context, out io.Writer, g *globals, opts *replayOpts, outputExplicit bool) error {
	if err := validateOutput(g.Output, true); err != nil {
		return err
	}
	// Streaming default is JSON lines. Explicit --output table /
	// json keeps the user's choice.
	format := g.Output
	if !outputExplicit {
		format = FormatJSONLines
	}
	filter, err := buildEventFilter(opts.eventType, opts.category, opts.source, opts.minSeverity, opts.since, opts.until, opts.correlationID, opts.tags, time.Now)
	if err != nil {
		return err
	}
	if filter == nil || filter.GetSince() == nil {
		return fmt.Errorf("--since is required")
	}

	client, closer, err := g.Deps.Dial(ctx, g.Server, g.APIKey)
	if err != nil {
		return err
	}
	defer func() { _ = closer.Close() }()

	useTable := format == FormatTable
	useJSON := format == FormatJSON
	var allEvents []*v1.Event

	cursor := ""
	for {
		req := &v1.ListEventsRequest{
			Filter:    filter,
			PageToken: cursor,
			PageSize:  int32(opts.pageSize), //nolint:gosec // operator-supplied
		}
		resp, err := client.ListEvents(authContext(ctx, g.APIKey), req)
		if err != nil {
			return fmt.Errorf("ListEvents: %w", err)
		}
		for _, e := range resp.GetEvents() {
			switch {
			case useJSON:
				allEvents = append(allEvents, e)
			case useTable:
				if werr := writeEventCompactLine(out, e); werr != nil {
					return werr
				}
			default: // jsonlines
				if werr := writeEventJSONLine(out, e); werr != nil {
					return werr
				}
			}
		}
		if resp.GetNextPageToken() == "" {
			break
		}
		cursor = resp.GetNextPageToken()
	}

	if useJSON {
		// Accumulate-and-emit for the single-response JSON form so
		// the output is a single document rather than concatenated
		// pages.
		return writeJSON(out, &v1.ListEventsResponse{Events: allEvents})
	}
	return nil
}
