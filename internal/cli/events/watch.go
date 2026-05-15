package events

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

type watchOpts struct {
	eventType   string
	category    string
	source      string
	minSeverity string
	tags        []string
}

func watchCmd(g *globals) *cobra.Command {
	opts := &watchOpts{}
	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Tail the event stream live (compact one-line-per-event)",
		Long: "Live tail of the event stream. Like `subscribe` but always " +
			"renders compact one-line-per-event output (time | severity | " +
			"type | source | tags) for human eyeballs. No replay; no CEL " +
			"filter (use `subscribe` for those).",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runWatch(cmd.Context(), cmd.OutOrStdout(), g, opts)
		},
	}
	cmd.Flags().StringVar(&opts.eventType, "type", "", "narrow: exact event type")
	cmd.Flags().StringVar(&opts.category, "category", "", "narrow: category fan-in")
	cmd.Flags().StringVar(&opts.source, "source", "", "narrow: exact source")
	cmd.Flags().StringVar(&opts.minSeverity, "min-severity", "", "narrow: minimum severity")
	cmd.Flags().StringSliceVar(&opts.tags, "tag", nil, "narrow: tag key=value (repeatable, ANDed)")
	return cmd
}

func runWatch(ctx context.Context, out io.Writer, g *globals, opts *watchOpts) error {
	// watch ignores --output (always compact); we validate that the
	// caller didn't pass an obviously broken value though.
	if err := validateOutput(g.Output, true); err != nil {
		return err
	}
	filter, err := buildEventFilter(opts.eventType, opts.category, opts.source, opts.minSeverity, "", "", "", opts.tags, time.Now)
	if err != nil {
		return err
	}

	client, closer, err := g.Deps.Dial(ctx, g.Server, g.APIKey)
	if err != nil {
		return err
	}
	defer func() { _ = closer.Close() }()

	stream, err := client.SubscribeEvents(authContext(ctx, g.APIKey), &v1.SubscribeEventsRequest{
		Filter: filter,
		// Live only — no replay, no CEL.
	})
	if err != nil {
		return fmt.Errorf("SubscribeEvents: %w", err)
	}

	for {
		resp, err := stream.Recv()
		if err == nil {
			if werr := writeEventCompactLine(out, resp.GetEvent()); werr != nil {
				return werr
			}
			continue
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if status.Code(err) == codes.Canceled {
			return nil
		}
		return fmt.Errorf("stream recv: %w", err)
	}
}
