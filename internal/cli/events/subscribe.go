// SPDX-License-Identifier: Apache-2.0

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

type subscribeOpts struct {
	filter         string
	replay         time.Duration
	queueGroup     string
	eventType      string
	category       string
	source         string
	minSeverity    string
	tags           []string
}

func subscribeCmd(g *globals) *cobra.Command {
	opts := &subscribeOpts{}
	cmd := &cobra.Command{
		Use:   "subscribe",
		Short: "Stream events from the bus (live; optional historical replay)",
		Long: "Open a server-streaming subscription. Output is JSON-lines by " +
			"default (one event per line, jq-friendly); pass --output table " +
			"for the compact one-line-per-event format used by `watch`, or " +
			"--output json for a buffered single-document form (events " +
			"buffered until Ctrl-C).\n\n" +
			"CEL filter syntax: severity threshold uses the method form " +
			"`severity.at_least('warn')` — plain `>=` lex-compares the " +
			"string and gives the wrong ordering.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			explicit := cmd.Flags().Changed("output")
			return runSubscribe(cmd.Context(), cmd.OutOrStdout(), g, opts, explicit)
		},
	}
	cmd.Flags().StringVar(&opts.filter, "filter", "", "CEL filter expression (e.g. severity.at_least('warn') && tags.role == 'web')")
	cmd.Flags().DurationVar(&opts.replay, "replay", 0, "stream events from the last N (e.g. 60s) before going live")
	cmd.Flags().StringVar(&opts.queueGroup, "queue-group", "", "NATS queue group for load-balanced consumers")
	cmd.Flags().StringVar(&opts.eventType, "type", "", "structural narrow: exact event type")
	cmd.Flags().StringVar(&opts.category, "category", "", "structural narrow: category fan-in")
	cmd.Flags().StringVar(&opts.source, "source", "", "structural narrow: exact source")
	cmd.Flags().StringVar(&opts.minSeverity, "min-severity", "", "structural narrow: minimum severity")
	cmd.Flags().StringSliceVar(&opts.tags, "tag", nil, "structural narrow: tag key=value (repeatable, ANDed)")
	return cmd
}

func runSubscribe(ctx context.Context, out io.Writer, g *globals, opts *subscribeOpts, outputExplicit bool) error {
	if err := validateOutput(g.Output, true); err != nil {
		return err
	}
	// Streaming default is JSON lines (jq-friendly). If the user
	// didn't pass --output explicitly, override the persistent
	// default (table) with jsonlines. Explicit --output table keeps
	// the compact format; --output json buffers a single document.
	format := g.Output
	if !outputExplicit {
		format = FormatJSONLines
	}
	filter, err := buildEventFilter(opts.eventType, opts.category, opts.source, opts.minSeverity, "", "", "", opts.tags, time.Now)
	if err != nil {
		return err
	}
	req := &v1.SubscribeEventsRequest{
		Filter:           filter,
		FilterExpression: opts.filter,
		ReplaySeconds:    int32(opts.replay / time.Second), //nolint:gosec // operator-supplied
		QueueGroup:       opts.queueGroup,
	}

	client, closer, err := g.Deps.Dial(ctx, g.Server, g.APIKey)
	if err != nil {
		return err
	}
	defer func() { _ = closer.Close() }()

	stream, err := client.SubscribeEvents(authContext(ctx, g.APIKey), req)
	if err != nil {
		return fmt.Errorf("SubscribeEvents: %w", err)
	}

	useTable := format == FormatTable

	for {
		resp, err := stream.Recv()
		if err == nil {
			if useTable {
				if werr := writeEventCompactLine(out, resp.GetEvent()); werr != nil {
					return werr
				}
			} else {
				if werr := writeEventJSONLine(out, resp.GetEvent()); werr != nil {
					return werr
				}
			}
			continue
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		// gRPC translates ctx-cancel into Canceled — that's the
		// normal exit when the operator hits Ctrl-C.
		if status.Code(err) == codes.Canceled {
			return nil
		}
		return fmt.Errorf("stream recv: %w", err)
	}
}
