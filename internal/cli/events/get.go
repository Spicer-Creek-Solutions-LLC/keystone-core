// SPDX-License-Identifier: Apache-2.0

package events

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

func getCmd(g *globals) *cobra.Command {
	return &cobra.Command{
		Use:   "get <event-id>",
		Short: "Fetch one event by ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGet(cmd.Context(), cmd.OutOrStdout(), g, args[0])
		},
	}
}

func runGet(ctx context.Context, out io.Writer, g *globals, id string) error {
	if err := validateOutput(g.Output, false); err != nil {
		return err
	}
	client, closer, err := g.Deps.Dial(ctx, g.Server, g.APIKey)
	if err != nil {
		return err
	}
	defer func() { _ = closer.Close() }()

	resp, err := client.GetEvent(authContext(ctx, g.APIKey), &v1.GetEventRequest{EventId: id})
	if err != nil {
		return fmt.Errorf("GetEvent: %w", err)
	}

	switch g.Output {
	case FormatJSON:
		return writeJSON(out, resp)
	default:
		return printGetTable(out, resp.GetEvent())
	}
}

func printGetTable(out io.Writer, e *v1.Event) error {
	if e == nil {
		_, _ = fmt.Fprintln(out, "no event returned")
		return nil
	}
	t := newTable(out)
	t.header("FIELD", "VALUE")
	t.row("id", e.GetId())
	t.row("type", e.GetType())
	t.row("source", e.GetSource())
	t.row("severity", severityNameFromEnum(e.GetSeverity()))
	t.row("time", formatProtoTimestamp(e.GetTime()))
	if v := e.GetCorrelationId(); v != "" {
		t.row("correlation_id", v)
	}
	if v := e.GetSubject(); v != "" {
		t.row("subject", v)
	}
	if tags := formatTagsCompact(e.GetTags()); tags != "—" {
		t.row("tags", tags)
	}
	if data := e.GetData(); data != nil && len(data.GetFields()) > 0 {
		t.row("data", "(use --output json to inspect)")
	}
	return t.flush()
}
