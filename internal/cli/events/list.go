// SPDX-License-Identifier: Apache-2.0

package events

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

type listOpts struct {
	eventType     string
	category      string
	source        string
	minSeverity   string
	since         string
	until         string
	correlationID string
	tags          []string
	cursor        string
	limit         int
	desc          bool
}

func listCmd(g *globals) *cobra.Command {
	opts := &listOpts{}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Paginated event list (structural filters)",
		Long: "List events matching the given structural filters. Cursor-based " +
			"pagination via --cursor / --limit. For CEL filter expressions and " +
			"long-running streams use `subscribe`; for ad-hoc historical " +
			"playback use `replay`.\n\nType and category are mutually exclusive.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runList(cmd.Context(), cmd.OutOrStdout(), g, opts)
		},
	}
	cmd.Flags().StringVar(&opts.eventType, "type", "", "exact event type (e.g. agent.connect); mutually exclusive with --category")
	cmd.Flags().StringVar(&opts.category, "category", "", "category fan-in (agent | job | state | system | user | policy)")
	cmd.Flags().StringVar(&opts.source, "source", "", "exact event source")
	cmd.Flags().StringVar(&opts.minSeverity, "min-severity", "", "minimum severity (debug | info | warn | error | critical)")
	cmd.Flags().StringVar(&opts.since, "since", "", "lower-bound timestamp (RFC3339 or duration like 1h / 5m)")
	cmd.Flags().StringVar(&opts.until, "until", "", "upper-bound timestamp (RFC3339 or duration like 30m)")
	cmd.Flags().StringVar(&opts.correlationID, "correlation-id", "", "exact correlation id")
	cmd.Flags().StringSliceVar(&opts.tags, "tag", nil, "tag predicate key=value (repeatable, ANDed)")
	cmd.Flags().StringVar(&opts.cursor, "cursor", "", "pagination cursor from a prior page's next_page_token")
	cmd.Flags().IntVar(&opts.limit, "limit", 50, "page size (max events returned)")
	cmd.Flags().BoolVar(&opts.desc, "desc", false, "return events newest-first (default ascending)")
	return cmd
}

func runList(ctx context.Context, out io.Writer, g *globals, opts *listOpts) error {
	if err := validateOutput(g.Output, false); err != nil {
		return err
	}
	filter, err := buildEventFilter(opts.eventType, opts.category, opts.source, opts.minSeverity, opts.since, opts.until, opts.correlationID, opts.tags, time.Now)
	if err != nil {
		return err
	}
	req := &v1.ListEventsRequest{
		Filter:     filter,
		PageToken:  opts.cursor,
		PageSize:   int32(opts.limit), //nolint:gosec // operator-supplied; large values are caller's choice
		Descending: opts.desc,
	}

	client, closer, err := g.Deps.Dial(ctx, g.Server, g.APIKey)
	if err != nil {
		return err
	}
	defer func() { _ = closer.Close() }()

	resp, err := client.ListEvents(authContext(ctx, g.APIKey), req)
	if err != nil {
		return fmt.Errorf("ListEvents: %w", err)
	}

	switch g.Output {
	case FormatJSON:
		return writeJSON(out, resp)
	default:
		return printListTable(out, resp)
	}
}

// buildEventFilter materialises an [v1.EventFilter] from the
// CLI flag set. Reused by list / replay / stats. Returns nil for
// the empty case so the server sees "no filter" rather than a
// stub object.
func buildEventFilter(eventType, category, source, minSeverity, since, until, correlationID string, tags []string, now func() time.Time) (*v1.EventFilter, error) {
	if eventType != "" && category != "" {
		return nil, fmt.Errorf("--type and --category are mutually exclusive")
	}
	tagMap, err := parseTagFlags(tags)
	if err != nil {
		return nil, err
	}
	sev, err := severityEnumFromName(minSeverity)
	if err != nil {
		return nil, err
	}
	sinceT, err := parseRelativeDuration(since, now())
	if err != nil {
		return nil, fmt.Errorf("--since: %w", err)
	}
	untilT, err := parseRelativeDuration(until, now())
	if err != nil {
		return nil, fmt.Errorf("--until: %w", err)
	}

	if eventType == "" && category == "" && source == "" && minSeverity == "" &&
		sinceT.IsZero() && untilT.IsZero() && correlationID == "" && len(tagMap) == 0 {
		return nil, nil
	}
	f := &v1.EventFilter{
		Type:          eventType,
		Source:        source,
		MinSeverity:   sev,
		CorrelationId: correlationID,
		Tags:          tagMap,
	}
	if category != "" {
		f.Categories = []string{category}
	}
	if !sinceT.IsZero() {
		f.Since = timestamppb.New(sinceT)
	}
	if !untilT.IsZero() {
		f.Until = timestamppb.New(untilT)
	}
	return f, nil
}

func printListTable(out io.Writer, resp *v1.ListEventsResponse) error {
	if len(resp.GetEvents()) == 0 {
		_, _ = fmt.Fprintln(out, "no events")
		return nil
	}
	t := newTable(out)
	t.header("TIME", "SEVERITY", "TYPE", "SOURCE", "ID")
	for _, e := range resp.GetEvents() {
		t.row(
			formatProtoTimestamp(e.GetTime()),
			severityNameFromEnum(e.GetSeverity()),
			e.GetType(),
			e.GetSource(),
			e.GetId(),
		)
	}
	if err := t.flush(); err != nil {
		return err
	}
	if cursor := resp.GetNextPageToken(); cursor != "" {
		_, _ = fmt.Fprintln(out)
		_, _ = fmt.Fprintf(out, "next page: --cursor %s\n", cursor)
	}
	return nil
}
