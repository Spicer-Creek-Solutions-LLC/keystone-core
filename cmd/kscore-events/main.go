// Package main implements the kscore-events CLI for event management and querying.
package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gopkg.in/yaml.v3"

	"github.com/shawnbutts/keystone-core/internal/cli/output"
	"github.com/shawnbutts/keystone-core/internal/events"
	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
	"github.com/shawnbutts/keystone-core/pkg/version"
)

func newRootCmd() *cobra.Command {
	cfg := &Config{}

	rootCmd := &cobra.Command{
		Use:   "kscore-events",
		Short: "Keystone Core event management plugin",
		Long: `kscore-events is a CLI plugin for managing Keystone Core events.

This plugin provides commands for:
  - Listing and querying events
  - Emitting custom events
  - Watching events in real-time
  - Subscribing to event patterns
  - Exporting events to files
  - Analyzing event patterns
  - Managing retention policies
  - Managing the dead letter queue

Usage via kscorectl:
  kscorectl events list
  kscorectl events emit --type custom.event --data '{"key":"value"}'
  kscorectl events watch
  kscorectl events subscribe "agent.*"
  kscorectl events export --output events.json`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	rootCmd.PersistentFlags().StringVarP(&cfg.ServerAddr, "server", "s", "localhost:50051", "Control plane server address")
	rootCmd.PersistentFlags().StringVarP(&cfg.OutputFormat, "output", "o", "table", "Output format (table, text, json, yaml)")
	rootCmd.PersistentFlags().BoolVarP(&cfg.Verbose, "verbose", "v", false, "Enable verbose output")
	rootCmd.PersistentFlags().DurationVar(&cfg.Timeout, "timeout", 30*time.Second, "Request timeout")

	// TLS flags
	rootCmd.PersistentFlags().BoolVar(&cfg.TLS, "tls", false, "Enable TLS for server connection")
	rootCmd.PersistentFlags().StringVar(&cfg.TLSCACert, "tls-ca-cert", "", "Path to CA certificate for verifying the server")
	rootCmd.PersistentFlags().StringVar(&cfg.TLSCert, "tls-cert", "", "Path to client certificate for mTLS authentication")
	rootCmd.PersistentFlags().StringVar(&cfg.TLSKey, "tls-key", "", "Path to client private key for mTLS authentication")
	rootCmd.PersistentFlags().BoolVar(&cfg.TLSSkipVerify, "tls-skip-verify", false, "Skip TLS certificate verification (INSECURE - for development only)")
	rootCmd.PersistentFlags().StringVar(&cfg.TLSServerName, "tls-server-name", "", "Server name for TLS verification (defaults to server host)")
	rootCmd.PersistentFlags().StringVar(&cfg.TLSMinVersion, "tls-min-version", "1.3", "Minimum TLS version (1.2 or 1.3)")

	rootCmd.AddCommand(
		newVersionCmd(),
		newListCmd(cfg),
		newQueryCmd(cfg),
		newEmitCmd(cfg),
		newReplayCmd(),
		newWatchCmd(cfg),
		newSubscribeCmd(cfg),
		newExportCmd(cfg),
		newRetentionCmd(),
		newDLQCmd(),
		newAnalyzeCmd(cfg),
		newSubscribersCmd(),
		newStorageStatsCmd(cfg),
		newPruneCmd(),
		newArchiveCmd(),
	)

	return rootCmd
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			info := version.Get()
			fmt.Fprintln(cmd.OutOrStdout(), info.String())
		},
	}
}

// ListOptions holds list command options.
type ListOptions struct {
	Type          string
	Source        string
	Severity      string
	Since         string
	Before        string
	CorrelationID string
	Limit         int
	Cursor        string
	Tags          []string
}

func newListCmd(cfg *Config) *cobra.Command {
	opts := &ListOptions{}

	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List events",
		Long: `List events with optional filtering.

Examples:
  # List recent events
  kscorectl events list

  # List events by type
  kscorectl events list --type agent.connect

  # List events from a specific source
  kscorectl events list --source /agents/web-01

  # List events with severity warning or higher
  kscorectl events list --severity warning

  # List events in a time range
  kscorectl events list --since 1h --before 30m

  # List events with specific tags
  kscorectl events list --tag env:prod --tag role:web`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd, cfg, opts)
		},
	}

	cmd.Flags().StringVar(&opts.Type, "type", "", "Filter by event type (e.g., agent.connect, state.change)")
	cmd.Flags().StringVar(&opts.Source, "source", "", "Filter by event source")
	cmd.Flags().StringVar(&opts.Severity, "severity", "", "Filter by minimum severity (debug, info, warning, error, critical)")
	cmd.Flags().StringVar(&opts.Since, "since", "", "Show events since (e.g., 1h, 24h, 7d)")
	cmd.Flags().StringVar(&opts.Before, "before", "", "Show events before (e.g., 1h, 24h, 7d)")
	cmd.Flags().StringVar(&opts.Before, "until", "", "Show events until (alias for --before)")
	cmd.MarkFlagsMutuallyExclusive("before", "until")
	cmd.Flags().StringVar(&opts.CorrelationID, "correlation-id", "", "Filter by correlation ID")
	cmd.Flags().IntVar(&opts.Limit, "limit", 50, "Maximum number of events to show")
	cmd.Flags().StringVar(&opts.Cursor, "cursor", "", "Pagination cursor from previous response")
	cmd.Flags().StringArrayVar(&opts.Tags, "tag", nil, "Filter by tag (key:value format, can be specified multiple times)")

	return cmd
}

func runList(cmd *cobra.Command, cfg *Config, opts *ListOptions) error {
	client, conn, err := createEventClient(cfg)
	if err != nil {
		return err
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	req := &pb.ListEventsRequest{
		Type:          opts.Type,
		Source:        opts.Source,
		CorrelationId: opts.CorrelationID,
		PageSize:      int32(opts.Limit), //nolint:gosec // Limit is bounded by CLI flag default (100) and server caps
		PageToken:     opts.Cursor,
	}

	if opts.Severity != "" {
		req.MinSeverity = severityToProto(opts.Severity)
	}

	if opts.Since != "" {
		t, err := parseTimeOrDuration(opts.Since)
		if err != nil {
			return fmt.Errorf("invalid --since value: %w", err)
		}
		req.StartTime = timestamppb.New(t)
	}

	if opts.Before != "" {
		t, err := parseTimeOrDuration(opts.Before)
		if err != nil {
			return fmt.Errorf("invalid --before value: %w", err)
		}
		req.EndTime = timestamppb.New(t)
	}

	if len(opts.Tags) > 0 {
		req.Tags = parseTags(opts.Tags)
	}

	resp, err := client.ListEvents(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to list events: %w", err)
	}

	if len(resp.Events) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No events found matching criteria")
		return nil
	}

	return outputEvents(cmd.OutOrStdout(), cfg.OutputFormat, resp.Events, resp.NextPageToken)
}

func newQueryCmd(cfg *Config) *cobra.Command {
	var filter string
	var limit int
	var since, until string

	cmd := &cobra.Command{
		Use:   "query <filter-expression>",
		Short: "Query events using filter expressions",
		Long: `Query events using powerful filter expressions.

Filter expression syntax:
  - Comparisons: type == "agent.connect", severity >= "warning"
  - Logical operators: and, or, not
  - Grouping: (type == "state.change" or type == "state.drift")
  - Field access: data.agent_id == "web-01"
  - Tag matching: tags.env == "prod"
  - Time functions: timestamp() > now() - duration("1h")

Examples:
  # Query agent events with errors
  kscorectl events query 'type =~ "agent.*" and severity == "error"'

  # Query state changes in production
  kscorectl events query 'type == "state.change" and tags.env == "prod"'

  # Query events from the last hour
  kscorectl events query 'severity >= "warning"' --since 1h`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filter = args[0]
			return runQuery(cmd, cfg, filter, limit, since, until)
		},
	}

	cmd.Flags().IntVarP(&limit, "limit", "n", 100, "Maximum number of events to return")
	cmd.Flags().StringVar(&since, "since", "", "Query events since time (duration like 1h or RFC3339 timestamp)")
	cmd.Flags().StringVar(&until, "until", "", "Query events until time (duration like 1h or RFC3339 timestamp)")

	return cmd
}

func runQuery(cmd *cobra.Command, cfg *Config, filter string, limit int, since, until string) error {
	client, conn, err := createEventClient(cfg)
	if err != nil {
		return err
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	req := &pb.ListEventsRequest{
		Filter:   filter,
		PageSize: int32(limit), //nolint:gosec // limit bounded by CLI flag default (50)
	}

	if since != "" {
		t, err := parseTimeOrDuration(since)
		if err != nil {
			return fmt.Errorf("invalid --since value: %w", err)
		}
		req.StartTime = timestamppb.New(t)
	}

	if until != "" {
		t, err := parseTimeOrDuration(until)
		if err != nil {
			return fmt.Errorf("invalid --until value: %w", err)
		}
		req.EndTime = timestamppb.New(t)
	}

	resp, err := client.ListEvents(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to query events: %w", err)
	}

	if len(resp.Events) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No events match the query")
		return nil
	}

	return outputEvents(cmd.OutOrStdout(), cfg.OutputFormat, resp.Events, resp.NextPageToken)
}

// EmitOptions holds emit command options.
type EmitOptions struct {
	Type          string
	Source        string
	Severity      string
	CorrelationID string
	Tags          []string
	Data          string
	DataFile      string
}

func newEmitCmd(cfg *Config) *cobra.Command {
	opts := &EmitOptions{}

	cmd := &cobra.Command{
		Use:   "emit",
		Short: "Emit a custom event",
		Long: `Emit a custom event to the event bus.

This is useful for:
  - Integration with external systems
  - Manual event injection for testing
  - Custom automation triggers

Examples:
  # Emit a simple event
  kscorectl events emit --type custom.deploy --source /ci/jenkins

  # Emit with data
  kscorectl events emit --type custom.deploy --data '{"version":"1.2.3","env":"prod"}'

  # Emit with tags
  kscorectl events emit --type custom.alert --severity warning --tag env:prod --tag team:platform

  # Emit with correlation ID
  kscorectl events emit --type custom.step --correlation-id job-12345`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEmit(cmd, cfg, opts)
		},
	}

	cmd.Flags().StringVar(&opts.Type, "type", "", "Event type (required)")
	cmd.Flags().StringVar(&opts.Source, "source", "", "Event source (defaults to /cli/kscore-events)")
	cmd.Flags().StringVar(&opts.Severity, "severity", "info", "Event severity (debug, info, warning, error, critical)")
	cmd.Flags().StringVar(&opts.CorrelationID, "correlation-id", "", "Correlation ID for linking related events")
	cmd.Flags().StringArrayVar(&opts.Tags, "tag", nil, "Event tags (key:value format)")
	cmd.Flags().StringVarP(&opts.Data, "data", "d", "", "Event data as JSON string")
	cmd.Flags().StringVarP(&opts.DataFile, "data-file", "f", "", "Event data from JSON file")

	cmd.MarkFlagRequired("type") //nolint:errcheck // cobra flag always exists

	return cmd
}

func runEmit(cmd *cobra.Command, cfg *Config, opts *EmitOptions) error {
	source := opts.Source
	if source == "" {
		source = "/cli/kscore-events"
	}

	// Validate severity
	severity := events.Severity(opts.Severity)
	switch severity {
	case events.SeverityDebug, events.SeverityInfo, events.SeverityWarning, events.SeverityError, events.SeverityCritical:
	default:
		return fmt.Errorf("invalid severity: %s (expected debug, info, warning, error, or critical)", opts.Severity)
	}

	// Parse data
	var dataStruct *structpb.Struct
	if opts.DataFile != "" {
		content, err := os.ReadFile(opts.DataFile)
		if err != nil {
			return fmt.Errorf("failed to read data file: %w", err)
		}
		var m map[string]interface{}
		if err := json.Unmarshal(content, &m); err != nil {
			return fmt.Errorf("failed to parse data file as JSON: %w", err)
		}
		dataStruct, err = structpb.NewStruct(m)
		if err != nil {
			return fmt.Errorf("failed to convert data to protobuf Struct: %w", err)
		}
	} else if opts.Data != "" {
		var m map[string]interface{}
		if err := json.Unmarshal([]byte(opts.Data), &m); err != nil {
			return fmt.Errorf("failed to parse data as JSON: %w", err)
		}
		var err error
		dataStruct, err = structpb.NewStruct(m)
		if err != nil {
			return fmt.Errorf("failed to convert data to protobuf Struct: %w", err)
		}
	}

	// Parse tags
	tags := parseTags(opts.Tags)

	client, conn, err := createEventClient(cfg)
	if err != nil {
		return err
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	req := &pb.EmitEventRequest{
		Type:          opts.Type,
		Source:        source,
		Severity:      severityToProto(opts.Severity),
		CorrelationId: opts.CorrelationID,
		Tags:          tags,
		Data:          dataStruct,
	}

	resp, err := client.EmitEvent(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to emit event: %w", err)
	}

	w := cmd.OutOrStdout()
	fmt.Fprintln(w, "Event emitted successfully:")
	fmt.Fprintf(w, "  ID:        %s\n", resp.EventId)
	fmt.Fprintf(w, "  Type:      %s\n", opts.Type)
	fmt.Fprintf(w, "  Source:    %s\n", source)
	fmt.Fprintf(w, "  Severity:  %s\n", opts.Severity)
	if resp.Timestamp != nil {
		fmt.Fprintf(w, "  Timestamp: %s\n", resp.Timestamp.AsTime().Format(time.RFC3339))
	}
	if opts.CorrelationID != "" {
		fmt.Fprintf(w, "  Correlation ID: %s\n", opts.CorrelationID)
	}

	return nil
}

// WatchOptions holds watch command options.
type WatchOptions struct {
	Type     string
	Source   string
	Severity string
	Filter   string
	Tags     []string
	Format   string
}

func newWatchCmd(cfg *Config) *cobra.Command {
	opts := &WatchOptions{}

	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Watch events in real-time",
		Long: `Watch events in real-time as they occur.

Press Ctrl+C to stop watching.

Examples:
  # Watch all events
  kscorectl events watch

  # Watch specific event types
  kscorectl events watch --type "agent.*"

  # Watch events from specific source
  kscorectl events watch --source "/agents/web-01"

  # Watch only warnings and above
  kscorectl events watch --severity warning

  # Watch with filter expression
  kscorectl events watch --filter 'tags.env == "prod"'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWatch(cmd, cfg, opts)
		},
	}

	cmd.Flags().StringVar(&opts.Type, "type", "", "Filter by event type pattern (supports wildcards)")
	cmd.Flags().StringVar(&opts.Source, "source", "", "Filter by event source")
	cmd.Flags().StringVar(&opts.Severity, "severity", "", "Filter by minimum severity")
	cmd.Flags().StringVar(&opts.Filter, "filter", "", "Filter expression")
	cmd.Flags().StringArrayVar(&opts.Tags, "tag", nil, "Filter by tag (key:value format)")
	cmd.Flags().StringVar(&opts.Format, "format", "text", "Output format: text, json, jsonl")

	return cmd
}

func runWatch(cmd *cobra.Command, cfg *Config, opts *WatchOptions) error {
	switch opts.Format {
	case "text", "json", "jsonl":
	default:
		return fmt.Errorf("unsupported format %q: use text, json, or jsonl", opts.Format)
	}

	client, conn, err := createEventClient(cfg)
	if err != nil {
		return err
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle Ctrl+C
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	go func() {
		<-sigCh
		cancel()
	}()

	req := &pb.SubscribeEventsRequest{
		Filter: opts.Filter,
	}
	if opts.Type != "" {
		req.Types = []string{opts.Type}
	}
	if opts.Source != "" {
		req.Sources = []string{opts.Source}
	}
	if opts.Severity != "" {
		req.MinSeverity = severityToProto(opts.Severity)
	}
	if len(opts.Tags) > 0 {
		req.Tags = parseTags(opts.Tags)
	}

	stream, err := client.SubscribeEvents(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to subscribe to events: %w", err)
	}

	w := cmd.OutOrStdout()

	if opts.Format == "text" {
		fmt.Fprintln(w, "Watching events... (press Ctrl+C to stop)")
		fmt.Fprintln(w)
		if opts.Type != "" {
			fmt.Fprintf(w, "  Type filter:     %s\n", opts.Type)
		}
		if opts.Source != "" {
			fmt.Fprintf(w, "  Source filter:   %s\n", opts.Source)
		}
		if opts.Severity != "" {
			fmt.Fprintf(w, "  Severity filter: %s+\n", opts.Severity)
		}
		if opts.Filter != "" {
			fmt.Fprintf(w, "  Filter:          %s\n", opts.Filter)
		}
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Timestamp           Type                    Source               Severity")
		fmt.Fprintln(w, strings.Repeat("-", 80))
	}

	type watchEvent struct {
		Timestamp     string            `json:"timestamp"`
		Type          string            `json:"type"`
		Source        string            `json:"source"`
		Severity      string            `json:"severity"`
		ID            string            `json:"id"`
		CorrelationID string            `json:"correlation_id,omitempty"`
		Tags          map[string]string `json:"tags,omitempty"`
	}

	var jsonEvents []watchEvent

	for {
		event, err := stream.Recv()
		if err != nil {
			if ctx.Err() != nil {
				break // graceful shutdown via Ctrl+C
			}
			if errors.Is(err, io.EOF) {
				break
			}
			return fmt.Errorf("stream error: %w", err)
		}

		ts := event.Time.AsTime()
		sev := protoToSeverity(event.Severity)

		switch opts.Format {
		case "text":
			fmt.Fprintf(w, "%s  %-22s  %-18s  %s\n",
				ts.Format("15:04:05.000"),
				event.Type,
				truncate(event.Source, 18),
				sev,
			)
		case "jsonl":
			evt := watchEvent{
				Timestamp:     ts.Format(time.RFC3339Nano),
				Type:          event.Type,
				Source:        event.Source,
				Severity:      sev,
				ID:            event.Id,
				CorrelationID: event.CorrelationId,
				Tags:          event.Tags,
			}
			data, _ := json.Marshal(evt)
			fmt.Fprintln(w, string(data))
		case "json":
			jsonEvents = append(jsonEvents, watchEvent{
				Timestamp:     ts.Format(time.RFC3339Nano),
				Type:          event.Type,
				Source:        event.Source,
				Severity:      sev,
				ID:            event.Id,
				CorrelationID: event.CorrelationId,
				Tags:          event.Tags,
			})
		}
	}

	if opts.Format == "json" && len(jsonEvents) > 0 {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		if err := enc.Encode(jsonEvents); err != nil {
			return fmt.Errorf("failed to encode events: %w", err)
		}
	}

	if opts.Format == "text" {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Watch stopped")
	}
	return nil
}

// SubscribeOptions holds subscribe command options.
type SubscribeOptions struct {
	FilterType     string
	FilterSeverity string
	Output         string
}

func newSubscribeCmd(cfg *Config) *cobra.Command {
	opts := &SubscribeOptions{}

	cmd := &cobra.Command{
		Use:   "subscribe <pattern>",
		Short: "Subscribe to events matching a pattern",
		Long: `Subscribe to events matching a pattern and display them as they arrive.

The pattern supports wildcards to match event types. Press Ctrl+C to stop.

Examples:
  # Subscribe to all agent events
  kscorectl events subscribe "agent.*"

  # Subscribe to state drift events
  kscorectl events subscribe "state.drift.*"

  # Subscribe with severity filter
  kscorectl events subscribe "agent.*" --filter-severity warning

  # Subscribe with JSON output
  kscorectl events subscribe "state.*" --output json`,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSubscribe(cmd, cfg, args[0], opts)
		},
	}

	cmd.Flags().StringVar(&opts.FilterType, "filter-type", "", "Additional filter by event type")
	cmd.Flags().StringVar(&opts.FilterSeverity, "filter-severity", "", "Filter by minimum severity (debug, info, warning, error, critical)")
	cmd.Flags().StringVar(&opts.Output, "output", "table", "Output format (json, table)")

	return cmd
}

func runSubscribe(cmd *cobra.Command, cfg *Config, pattern string, opts *SubscribeOptions) error {
	if opts.FilterSeverity != "" {
		if _, ok := severityLevels[opts.FilterSeverity]; !ok {
			return fmt.Errorf("invalid severity: %s (expected debug, info, warning, error, or critical)", opts.FilterSeverity)
		}
	}

	client, conn, err := createEventClient(cfg)
	if err != nil {
		return err
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	go func() {
		<-sigCh
		cancel()
	}()

	req := &pb.SubscribeEventsRequest{
		Types: []string{pattern},
	}
	if opts.FilterSeverity != "" {
		req.MinSeverity = severityToProto(opts.FilterSeverity)
	}

	stream, err := client.SubscribeEvents(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to subscribe to events: %w", err)
	}

	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "Subscribed to %s...\n", pattern)

	for {
		event, err := stream.Recv()
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			if errors.Is(err, io.EOF) {
				break
			}
			return fmt.Errorf("stream error: %w", err)
		}

		// Apply additional type filter client-side
		if opts.FilterType != "" && event.Type != opts.FilterType {
			continue
		}

		ts := event.Time.AsTime()
		sev := protoToSeverity(event.Severity)

		switch opts.Output {
		case "json":
			type eventJSON struct {
				ID            string            `json:"id"`
				Type          string            `json:"type"`
				Source        string            `json:"source"`
				Severity      string            `json:"severity"`
				Timestamp     string            `json:"timestamp"`
				CorrelationID string            `json:"correlation_id,omitempty"`
				Tags          map[string]string `json:"tags,omitempty"`
			}
			data, err := json.Marshal(eventJSON{
				ID:            event.Id,
				Type:          event.Type,
				Source:        event.Source,
				Severity:      sev,
				Timestamp:     ts.Format(time.RFC3339Nano),
				CorrelationID: event.CorrelationId,
				Tags:          event.Tags,
			})
			if err != nil {
				return fmt.Errorf("failed to marshal event: %w", err)
			}
			fmt.Fprintln(w, string(data))
		default:
			fmt.Fprintf(w, "%s  %-22s  %-18s  %s\n",
				ts.Format("15:04:05.000"),
				event.Type,
				truncate(event.Source, 18),
				sev,
			)
		}
	}

	return nil
}

// ExportOptions holds export command options.
type ExportOptions struct {
	Output string
	Format string
	Start  string
	End    string
	Type   string
	Limit  int
}

func newExportCmd(cfg *Config) *cobra.Command {
	opts := &ExportOptions{}

	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export events to a file",
		Long: `Export events to a file in JSON or CSV format.

Events are exported from the event store to a local file for
offline analysis, archival, or integration with external systems.

Examples:
  # Export events to JSON
  kscorectl events export --output events.json

  # Export events to CSV
  kscorectl events export --output events.csv --format csv

  # Export with time range
  kscorectl events export --output events.json --start 2024-01-01T00:00:00Z --end 2024-01-02T00:00:00Z

  # Export specific event type with limit
  kscorectl events export --output agent-events.json --type agent.connect --limit 500`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExport(cmd, cfg, opts)
		},
	}

	cmd.Flags().StringVar(&opts.Output, "output", "", "Output file path (required)")
	cmd.Flags().StringVar(&opts.Format, "format", "json", "Export format (json, csv)")
	cmd.Flags().StringVar(&opts.Start, "start", "", "Start time (RFC3339 format)")
	cmd.Flags().StringVar(&opts.End, "end", "", "End time (RFC3339 format)")
	cmd.Flags().StringVar(&opts.Type, "type", "", "Filter by event type")
	cmd.Flags().IntVar(&opts.Limit, "limit", 1000, "Maximum number of events to export")

	_ = cmd.MarkFlagRequired("output")

	return cmd
}

func runExport(cmd *cobra.Command, cfg *Config, opts *ExportOptions) error {
	switch opts.Format {
	case "json", "csv":
	default:
		return fmt.Errorf("unsupported format: %s (expected json or csv)", opts.Format)
	}

	if opts.Start != "" {
		if _, err := time.Parse(time.RFC3339, opts.Start); err != nil {
			return fmt.Errorf("invalid start time: %w", err)
		}
	}
	if opts.End != "" {
		if _, err := time.Parse(time.RFC3339, opts.End); err != nil {
			return fmt.Errorf("invalid end time: %w", err)
		}
	}

	client, conn, err := createEventClient(cfg)
	if err != nil {
		return err
	}
	defer conn.Close()

	// Collect events via pagination
	var allEvents []*pb.Event
	pageToken := ""
	for {
		ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
		req := &pb.ListEventsRequest{
			Type:      opts.Type,
			PageSize:  int32(opts.Limit - len(allEvents)), //nolint:gosec // Limit bounded by CLI flag default (1000)
			PageToken: pageToken,
		}
		if opts.Start != "" {
			t, _ := time.Parse(time.RFC3339, opts.Start)
			req.StartTime = timestamppb.New(t)
		}
		if opts.End != "" {
			t, _ := time.Parse(time.RFC3339, opts.End)
			req.EndTime = timestamppb.New(t)
		}

		resp, err := client.ListEvents(ctx, req)
		cancel()
		if err != nil {
			return fmt.Errorf("failed to list events: %w", err)
		}

		allEvents = append(allEvents, resp.Events...)
		if resp.NextPageToken == "" || len(allEvents) >= opts.Limit {
			break
		}
		pageToken = resp.NextPageToken
	}

	if len(allEvents) > opts.Limit {
		allEvents = allEvents[:opts.Limit]
	}

	file, err := os.Create(opts.Output)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer file.Close()

	switch opts.Format {
	case "csv":
		csvWriter := csv.NewWriter(file)
		defer csvWriter.Flush()

		if err := csvWriter.Write([]string{"id", "type", "source", "severity", "timestamp", "correlation_id"}); err != nil {
			return fmt.Errorf("failed to write CSV header: %w", err)
		}

		for _, event := range allEvents {
			record := []string{
				event.Id,
				event.Type,
				event.Source,
				protoToSeverity(event.Severity),
				event.Time.AsTime().Format(time.RFC3339),
				event.CorrelationId,
			}
			if err := csvWriter.Write(record); err != nil {
				return fmt.Errorf("failed to write CSV record: %w", err)
			}
		}
	default:
		type exportEvent struct {
			ID            string            `json:"id"`
			Type          string            `json:"type"`
			Source        string            `json:"source"`
			Severity      string            `json:"severity"`
			Timestamp     string            `json:"timestamp"`
			CorrelationID string            `json:"correlation_id,omitempty"`
			Tags          map[string]string `json:"tags,omitempty"`
		}
		var jsonEvents []exportEvent
		for _, e := range allEvents {
			jsonEvents = append(jsonEvents, exportEvent{
				ID:            e.Id,
				Type:          e.Type,
				Source:        e.Source,
				Severity:      protoToSeverity(e.Severity),
				Timestamp:     e.Time.AsTime().Format(time.RFC3339),
				CorrelationID: e.CorrelationId,
				Tags:          e.Tags,
			})
		}
		enc := json.NewEncoder(file)
		enc.SetIndent("", "  ")
		if err := enc.Encode(jsonEvents); err != nil {
			return fmt.Errorf("failed to write JSON: %w", err)
		}
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Exported %d event(s) to %s (format: %s)\n", len(allEvents), opts.Output, opts.Format)
	return nil
}

// AnalyzeOptions holds analyze command options.
type AnalyzeOptions struct {
	EventType string
	Since     string
	Limit     int
}

func newAnalyzeCmd(cfg *Config) *cobra.Command {
	opts := &AnalyzeOptions{}

	cmd := &cobra.Command{
		Use:   "analyze",
		Short: "Analyze event patterns and trends",
		Long: `Analyze event patterns, frequency, and trends over time.

Provides summary statistics about event types, sources, severities,
and identifies potential anomalies.

Examples:
  kscorectl events analyze
  kscorectl events analyze --type agent.heartbeat --since 24h
  kscorectl events analyze --limit 1000`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return analyzeExecute(cmd, cfg, opts)
		},
	}

	cmd.Flags().StringVar(&opts.EventType, "type", "", "Filter analysis to specific event type")
	cmd.Flags().StringVar(&opts.Since, "since", "1h", "Analyze events since this duration ago")
	cmd.Flags().IntVar(&opts.Limit, "limit", 10000, "Maximum events to analyze")

	return cmd
}

func analyzeExecute(cmd *cobra.Command, cfg *Config, opts *AnalyzeOptions) error {
	client, conn, err := createEventClient(cfg)
	if err != nil {
		return err
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	req := &pb.GetEventStatsRequest{}

	if opts.Since != "" {
		t, err := parseTimeOrDuration(opts.Since)
		if err != nil {
			return fmt.Errorf("invalid --since value: %w", err)
		}
		req.StartTime = timestamppb.New(t)
	}

	resp, err := client.GetEventStats(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to get event stats: %w", err)
	}

	w := cmd.OutOrStdout()

	fmt.Fprintf(w, "=== Event Analysis ===\n")
	fmt.Fprintf(w, "Time range: %s\n", opts.Since)
	fmt.Fprintf(w, "Total events: %d\n", resp.TotalEvents)
	fmt.Fprintf(w, "Event rate: %.2f events/sec\n\n", resp.EventRate)

	if len(resp.ByType) > 0 {
		fmt.Fprintf(w, "By Type:\n")
		for t, c := range resp.ByType {
			if opts.EventType != "" && !strings.Contains(t, opts.EventType) {
				continue
			}
			pct := float64(0)
			if resp.TotalEvents > 0 {
				pct = float64(c) / float64(resp.TotalEvents) * 100
			}
			fmt.Fprintf(w, "  %-40s %6d (%5.1f%%)\n", t, c, pct)
		}
	}

	if len(resp.BySource) > 0 {
		fmt.Fprintf(w, "\nBy Source:\n")
		for s, c := range resp.BySource {
			pct := float64(0)
			if resp.TotalEvents > 0 {
				pct = float64(c) / float64(resp.TotalEvents) * 100
			}
			fmt.Fprintf(w, "  %-40s %6d (%5.1f%%)\n", s, c, pct)
		}
	}

	if len(resp.BySeverity) > 0 {
		fmt.Fprintf(w, "\nBy Severity:\n")
		for s, c := range resp.BySeverity {
			pct := float64(0)
			if resp.TotalEvents > 0 {
				pct = float64(c) / float64(resp.TotalEvents) * 100
			}
			fmt.Fprintf(w, "  %-20s %6d (%5.1f%%)\n", s, c, pct)
		}
	}

	return nil
}

func newStorageStatsCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "storage-stats",
		Short: "Show event storage statistics",
		Long: `Display storage statistics for the event system.

Shows event counts, storage size, retention settings, and
JetStream stream information.

Examples:
  kscorectl events storage-stats
  kscorectl events storage-stats --output json`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return storageStatsExecute(cmd, cfg)
		},
	}

	return cmd
}

func storageStatsExecute(cmd *cobra.Command, cfg *Config) error {
	client, conn, err := createEventClient(cfg)
	if err != nil {
		return err
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	resp, err := client.GetEventStats(ctx, &pb.GetEventStatsRequest{})
	if err != nil {
		return fmt.Errorf("failed to get storage stats: %w", err)
	}

	w := cmd.OutOrStdout()

	type storageStats struct {
		TotalEvents int64              `json:"total_events" yaml:"total_events"`
		EventRate   float32            `json:"event_rate" yaml:"event_rate"`
		ByType      map[string]int64   `json:"by_type,omitempty" yaml:"by_type,omitempty"`
		BySeverity  map[string]int64   `json:"by_severity,omitempty" yaml:"by_severity,omitempty"`
	}

	stats := storageStats{
		TotalEvents: resp.TotalEvents,
		EventRate:   resp.EventRate,
		ByType:      resp.ByType,
		BySeverity:  resp.BySeverity,
	}

	switch cfg.OutputFormat {
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(stats)
	case "yaml":
		return yaml.NewEncoder(w).Encode(stats)
	default:
		fmt.Fprintf(w, "=== Event Storage Statistics ===\n\n")
		fmt.Fprintf(w, "Total Events: %d\n", stats.TotalEvents)
		fmt.Fprintf(w, "Event Rate:   %.2f events/sec\n", stats.EventRate)
		if len(stats.ByType) > 0 {
			fmt.Fprintf(w, "\nBy Type:\n")
			for t, c := range stats.ByType {
				fmt.Fprintf(w, "  %-30s %d\n", t, c)
			}
		}
	}

	return nil
}

// --- Commands that require server-side support not yet available ---

func notAvailableError(feature string) error {
	return fmt.Errorf("%s requires server-side support that is not yet available", feature)
}

func newReplayCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "replay",
		Short: "Replay historical events",
		Long: `Replay historical events through the event bus.

This is useful for:
  - Testing reactors with historical data
  - Recovering from missed events
  - Debugging event-driven workflows

Examples:
  kscorectl events replay --since 1h
  kscorectl events replay --type state.change --since 24h
  kscorectl events replay --since 1h --dry-run`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return notAvailableError("event replay")
		},
	}

	cmd.Flags().String("type", "", "Filter by event type")
	cmd.Flags().String("source", "", "Filter by event source")
	cmd.Flags().String("since", "1h", "Replay events since (e.g., 1h, 24h, 7d)")
	cmd.Flags().String("before", "", "Replay events before (e.g., 1h, 24h, 7d)")
	cmd.Flags().String("correlation-id", "", "Filter by correlation ID")
	cmd.Flags().String("filter", "", "Filter expression")
	cmd.Flags().Bool("dry-run", false, "Show what would be replayed without actually replaying")
	cmd.Flags().Float64("speed", 1.0, "Replay speed multiplier")

	return cmd
}

func newRetentionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "retention",
		Short: "Manage event retention policies",
		Long: `Manage event retention policies.

Retention policies control how long events are stored and when they are cleaned up.

Examples:
  kscorectl events retention list
  kscorectl events retention set --max-age 30d
  kscorectl events retention apply`,
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List retention policies",
		RunE: func(cmd *cobra.Command, args []string) error {
			return notAvailableError("retention policy listing")
		},
	}

	setCmd := &cobra.Command{
		Use:   "set",
		Short: "Set a retention policy",
		RunE: func(cmd *cobra.Command, args []string) error {
			return notAvailableError("retention policy management")
		},
	}
	setCmd.Flags().String("type", "", "Event type pattern")
	setCmd.Flags().String("max-age", "", "Maximum age (e.g., 30d, 90d)")
	setCmd.Flags().Int("max-count", 0, "Maximum number of events to keep")
	setCmd.Flags().String("min-severity", "", "Minimum severity to keep")

	applyCmd := &cobra.Command{
		Use:   "apply",
		Short: "Apply retention policies now",
		RunE: func(cmd *cobra.Command, args []string) error {
			return notAvailableError("retention policy application")
		},
	}
	applyCmd.Flags().Bool("dry-run", false, "Show what would be deleted")

	cmd.AddCommand(listCmd, setCmd, applyCmd)
	return cmd
}

func newDLQCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dlq",
		Short: "Manage the dead letter queue",
		Long: `Manage the dead letter queue for failed events.

Events that fail to process (e.g., due to reactor errors) are moved to the DLQ.
You can inspect, retry, or purge these events.

Examples:
  kscorectl events dlq list
  kscorectl events dlq retry <event-id>
  kscorectl events dlq purge`,
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List events in the dead letter queue",
		RunE: func(cmd *cobra.Command, args []string) error {
			return notAvailableError("dead letter queue listing")
		},
	}
	listCmd.Flags().IntP("limit", "n", 50, "Maximum number of entries to show")
	listCmd.Flags().String("reason", "", "Filter by failure reason")

	showCmd := &cobra.Command{
		Use:   "show <event-id>",
		Short: "Show details of a DLQ event",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return notAvailableError("dead letter queue event details")
		},
	}

	retryCmd := &cobra.Command{
		Use:   "retry [event-id]",
		Short: "Retry processing a DLQ event",
		RunE: func(cmd *cobra.Command, args []string) error {
			return notAvailableError("dead letter queue retry")
		},
	}
	retryCmd.Flags().Bool("all", false, "Retry all events in the DLQ")
	retryCmd.Flags().String("type", "", "Retry events of specific type")

	purgeCmd := &cobra.Command{
		Use:   "purge",
		Short: "Purge events from the DLQ",
		RunE: func(cmd *cobra.Command, args []string) error {
			return notAvailableError("dead letter queue purge")
		},
	}
	purgeCmd.Flags().BoolP("force", "f", false, "Skip confirmation prompt")
	purgeCmd.Flags().String("older-than", "", "Purge events older than duration")
	purgeCmd.Flags().String("reason", "", "Purge events with specific failure reason")

	cmd.AddCommand(listCmd, showCmd, retryCmd, purgeCmd)
	return cmd
}

func newSubscribersCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "subscribers",
		Short: "List active event subscribers",
		Long: `List all active event subscribers and their configurations.

Shows which components are subscribed to event streams, their
filter patterns, and processing status.

Examples:
  kscorectl events subscribers
  kscorectl events subscribers --output json`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return notAvailableError("subscriber listing")
		},
	}
}

func newPruneCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Remove old events based on criteria",
		Long: `Prune old events from storage based on age or type.

Events matching the criteria will be permanently deleted.
Use --dry-run to preview what would be pruned.

Examples:
  kscorectl events prune --older-than 30d
  kscorectl events prune --older-than 7d --type agent.heartbeat
  kscorectl events prune --older-than 7d --dry-run`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return notAvailableError("event pruning")
		},
	}

	cmd.Flags().String("older-than", "", "Prune events older than this duration (e.g., 7d, 30d)")
	cmd.Flags().String("type", "", "Only prune events of this type")
	cmd.Flags().Bool("dry-run", false, "Preview what would be pruned")
	cmd.Flags().BoolP("force", "f", false, "Skip confirmation prompt")

	_ = cmd.MarkFlagRequired("older-than")

	return cmd
}

func newArchiveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "archive",
		Short: "Archive events to a file before pruning",
		Long: `Archive events to a file for long-term storage.

Events are exported to the specified file and can optionally be
pruned from the active store afterward.

Examples:
  kscorectl events archive --older-than 30d --output events-archive.json
  kscorectl events archive --type audit.* --output audit-archive.json
  kscorectl events archive --older-than 7d --dry-run`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return notAvailableError("event archiving")
		},
	}

	cmd.Flags().String("older-than", "", "Archive events older than this duration")
	cmd.Flags().String("type", "", "Only archive events of this type")
	cmd.Flags().StringP("output", "o", "", "Output file for archived events")
	cmd.Flags().Bool("dry-run", false, "Preview what would be archived")

	_ = cmd.MarkFlagRequired("older-than")
	_ = cmd.MarkFlagRequired("output")

	return cmd
}

// --- Helper functions ---

var severityLevels = map[string]int{
	"debug":    0,
	"info":     1,
	"warning":  2,
	"error":    3,
	"critical": 4,
}

func parseTimeOrDuration(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}

	if len(s) < 2 {
		return time.Time{}, fmt.Errorf("invalid time or duration: %q", s)
	}

	unit := s[len(s)-1]
	numStr := s[:len(s)-1]
	n, err := strconv.Atoi(numStr)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid time or duration: %q", s)
	}

	var d time.Duration
	switch unit {
	case 's':
		d = time.Duration(n) * time.Second
	case 'm':
		d = time.Duration(n) * time.Minute
	case 'h':
		d = time.Duration(n) * time.Hour
	case 'd':
		d = time.Duration(n) * 24 * time.Hour
	default:
		return time.Time{}, fmt.Errorf("invalid duration unit %q in %q (use s, m, h, or d)", string(unit), s)
	}

	return time.Now().Add(-d), nil
}

func parseDurationWithDays(s string) (time.Duration, error) {
	if strings.HasSuffix(s, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil {
			return 0, fmt.Errorf("invalid day value: %s", s)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func parseTags(tags []string) map[string]string {
	result := make(map[string]string, len(tags))
	for _, tag := range tags {
		parts := strings.SplitN(tag, ":", 2)
		if len(parts) == 2 {
			result[parts[0]] = parts[1]
		}
	}
	return result
}

func outputEvents(w io.Writer, format string, evts []*pb.Event, nextPageToken string) error {
	type eventDisplay struct {
		ID            string            `json:"id" yaml:"id"`
		Type          string            `json:"type" yaml:"type"`
		Source        string            `json:"source" yaml:"source"`
		Severity      string            `json:"severity" yaml:"severity"`
		Timestamp     string            `json:"timestamp" yaml:"timestamp"`
		CorrelationID string            `json:"correlation_id,omitempty" yaml:"correlation_id,omitempty"`
		Tags          map[string]string `json:"tags,omitempty" yaml:"tags,omitempty"`
	}

	displayEvents := make([]eventDisplay, 0, len(evts))
	for _, e := range evts {
		displayEvents = append(displayEvents, eventDisplay{
			ID:            e.Id,
			Type:          e.Type,
			Source:        e.Source,
			Severity:      protoToSeverity(e.Severity),
			Timestamp:     e.Time.AsTime().Format(time.RFC3339),
			CorrelationID: e.CorrelationId,
			Tags:          e.Tags,
		})
	}

	switch format {
	case "json":
		data, err := json.MarshalIndent(displayEvents, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal JSON: %w", err)
		}
		fmt.Fprintln(w, string(data))
	case "yaml":
		data, err := yaml.Marshal(displayEvents)
		if err != nil {
			return fmt.Errorf("failed to marshal YAML: %w", err)
		}
		fmt.Fprint(w, string(data))
	default:
		table := &output.Table{
			Headers: []string{"ID", "TYPE", "SOURCE", "SEVERITY", "TIMESTAMP", "CORRELATION"},
		}
		for _, event := range displayEvents {
			corr := event.CorrelationID
			if len(corr) > 12 {
				corr = corr[:12] + "..."
			}
			id := event.ID
			if len(id) > 8 {
				id = id[:8] + "..."
			}
			table.Rows = append(table.Rows, []string{
				id,
				event.Type,
				truncate(event.Source, 20),
				event.Severity,
				event.Timestamp,
				corr,
			})
		}
		output.WriteTable(w, table)
		fmt.Fprintf(w, "\nTotal: %d event(s)\n", len(displayEvents))
		if nextPageToken != "" {
			fmt.Fprintf(w, "Next page: --cursor %s\n", nextPageToken)
		}
	}

	return nil
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
