// SPDX-License-Identifier: Apache-2.0

package events

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/structpb"

	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

type emitOpts struct {
	eventType     string
	source        string
	severity      string
	correlationID string
	tags          []string
	data          string
}

func emitCmd(g *globals) *cobra.Command {
	opts := &emitOpts{}
	cmd := &cobra.Command{
		Use:   "emit",
		Short: "Publish an event (sync; waits for NATS ack)",
		Long: "Emit one event through the EventService's EmitEvent RPC. The " +
			"server stamps id + time if omitted. --data accepts inline JSON " +
			"(e.g. --data '{\"k\":\"v\"}') or a @file.json path.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runEmit(cmd.Context(), cmd.OutOrStdout(), g, opts)
		},
	}
	cmd.Flags().StringVar(&opts.eventType, "type", "", "event type (required, e.g. agent.connect)")
	cmd.Flags().StringVar(&opts.source, "source", "", "event source (required)")
	cmd.Flags().StringVar(&opts.severity, "severity", "info", "severity (debug | info | warn | error | critical)")
	cmd.Flags().StringVar(&opts.correlationID, "correlation-id", "", "correlation id for trace propagation")
	cmd.Flags().StringSliceVar(&opts.tags, "tag", nil, "tag key=value (repeatable)")
	cmd.Flags().StringVar(&opts.data, "data", "", "structured payload as inline JSON or @file.json")
	_ = cmd.MarkFlagRequired("type")
	_ = cmd.MarkFlagRequired("source")
	return cmd
}

func runEmit(ctx context.Context, out io.Writer, g *globals, opts *emitOpts) error {
	if err := validateOutput(g.Output, false); err != nil {
		return err
	}
	sev, err := severityEnumFromName(opts.severity)
	if err != nil {
		return err
	}
	tagMap, err := parseTagFlags(opts.tags)
	if err != nil {
		return err
	}
	dataMap, err := parseDataFlag(opts.data, os.ReadFile)
	if err != nil {
		return err
	}
	ev := &v1.Event{
		Type:          opts.eventType,
		Source:        opts.source,
		Severity:      sev,
		CorrelationId: opts.correlationID,
		Tags:          tagMap,
	}
	if dataMap != nil {
		st, err := structpb.NewStruct(dataMap)
		if err != nil {
			return fmt.Errorf("convert --data to proto Struct: %w", err)
		}
		ev.Data = st
	}

	client, closer, err := g.Deps.Dial(ctx, g.Server, g.APIKey)
	if err != nil {
		return err
	}
	defer func() { _ = closer.Close() }()

	resp, err := client.EmitEvent(authContext(ctx, g.APIKey), &v1.EmitEventRequest{Event: ev})
	if err != nil {
		return fmt.Errorf("EmitEvent: %w", err)
	}

	switch g.Output {
	case FormatJSON:
		return writeJSON(out, resp)
	default:
		_, err := fmt.Fprintf(out, "emitted %s\n", resp.GetEventId())
		return err
	}
}
