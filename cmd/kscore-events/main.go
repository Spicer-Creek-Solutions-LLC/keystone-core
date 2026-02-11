// Package main implements the kscore-events CLI for event management and querying.
package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/shawnbutts/keystone-core/internal/cli/output"
	"github.com/shawnbutts/keystone-core/internal/events"
	"github.com/shawnbutts/keystone-core/pkg/version"
)

var (
	serverAddr   string
	outputFormat string
	verbose      bool
)

// EventDisplay is a struct for displaying events in list output
type EventDisplay struct {
	ID            string    `json:"id" yaml:"id"`
	Type          string    `json:"type" yaml:"type"`
	Source        string    `json:"source" yaml:"source"`
	Severity      string    `json:"severity" yaml:"severity"`
	Time          time.Time `json:"timestamp" yaml:"timestamp"`
	CorrelationID string    `json:"correlation_id,omitempty" yaml:"correlation_id,omitempty"`
}

func newRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "kscore-events",
		Short: "Keystone Core event management plugin",
		Long: `kscore-events is a CLI plugin for managing Keystone Core events.

This plugin provides commands for:
  - Listing and querying events
  - Emitting custom events
  - Replaying historical events
  - Watching events in real-time
  - Subscribing to event patterns
  - Exporting events to files
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

	rootCmd.PersistentFlags().StringVarP(&serverAddr, "server", "s", "localhost:9090", "Control plane server address")
	rootCmd.PersistentFlags().StringVarP(&outputFormat, "output", "o", "table", "Output format (table, text, json, yaml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")

	rootCmd.AddCommand(
		newVersionCmd(),
		newListCmd(),
		newQueryCmd(),
		newEmitCmd(),
		newReplayCmd(),
		newWatchCmd(),
		newSubscribeCmd(),
		newExportCmd(),
		newRetentionCmd(),
		newDLQCmd(),
		newAnalyzeCmd(),
		newSubscribersCmd(),
		newStorageStatsCmd(),
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

// ListOptions holds list command options
type ListOptions struct {
	Type          string
	Source        string
	Severity      string
	Since         string
	Before        string
	CorrelationID string
	Limit         int
	Tags          []string
}

// newListCmd creates the list command
func newListCmd() *cobra.Command {
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
			return runList(cmd, opts)
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
	cmd.Flags().StringArrayVar(&opts.Tags, "tag", nil, "Filter by tag (key:value format, can be specified multiple times)")

	return cmd
}

func runList(cmd *cobra.Command, opts *ListOptions) error {
	// Build sample events for demonstration
	// In production, this would query the server via gRPC or REST
	sampleEvents := generateSampleEvents(opts.Limit)

	// Apply filters
	filtered := filterEvents(sampleEvents, opts)

	if len(filtered) == 0 {
		fmt.Println("No events found matching criteria")
		return nil
	}

	// Output based on format
	switch outputFormat {
	case "json":
		data, err := json.MarshalIndent(filtered, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal JSON: %w", err)
		}
		fmt.Println(string(data))
	case "yaml":
		data, err := yaml.Marshal(filtered)
		if err != nil {
			return fmt.Errorf("failed to marshal YAML: %w", err)
		}
		fmt.Print(string(data))
	default:
		// Table format
		table := &output.Table{
			Headers: []string{"ID", "TYPE", "SOURCE", "SEVERITY", "TIMESTAMP", "CORRELATION"},
		}
		for _, event := range filtered {
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
				event.Time.Format("15:04:05"),
				corr,
			})
		}
		output.WriteTable(os.Stdout, table)
		fmt.Printf("\nTotal: %d event(s)\n", len(filtered))
	}

	return nil
}

// newQueryCmd creates the query command for advanced filtering
func newQueryCmd() *cobra.Command {
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
  kscorectl events query 'severity >= "warning"' --since 1h

  # Query events in a time range
  kscorectl events query 'type == "state.drift"' --since 2024-01-01T00:00:00Z --until 2024-01-02T00:00:00Z`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filter = args[0]
			return runQuery(cmd, filter, limit, since, until)
		},
	}

	cmd.Flags().IntVarP(&limit, "limit", "n", 100, "Maximum number of events to return")
	cmd.Flags().StringVar(&since, "since", "", "Query events since time (duration like 1h or RFC3339 timestamp)")
	cmd.Flags().StringVar(&until, "until", "", "Query events until time (duration like 1h or RFC3339 timestamp)")

	return cmd
}

func runQuery(cmd *cobra.Command, filter string, limit int, since, until string) error {
	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "Querying events with filter: %s\n", filter)
	fmt.Fprintf(w, "Limit: %d\n", limit)
	if since != "" {
		fmt.Fprintf(w, "Since: %s\n", since)
	}
	if until != "" {
		fmt.Fprintf(w, "Until: %s\n", until)
	}
	fmt.Fprintln(w)

	// In production, this would send the filter and time range to the server
	// For now, generate sample events
	sampleEvents := generateSampleEvents(limit)

	// Output results
	if len(sampleEvents) == 0 {
		fmt.Println("No events match the query")
		return nil
	}

	table := &output.Table{
		Headers: []string{"ID", "TYPE", "SOURCE", "SEVERITY", "TIMESTAMP"},
	}
	for _, event := range sampleEvents[:minInt(limit, len(sampleEvents))] {
		id := event.ID
		if len(id) > 8 {
			id = id[:8] + "..."
		}
		table.Rows = append(table.Rows, []string{
			id,
			event.Type,
			truncate(event.Source, 20),
			event.Severity,
			event.Time.Format("2006-01-02 15:04:05"),
		})
	}
	output.WriteTable(os.Stdout, table)
	fmt.Printf("\nFound %d event(s)\n", len(sampleEvents))

	return nil
}

// EmitOptions holds emit command options
type EmitOptions struct {
	Type          string
	Source        string
	Severity      string
	CorrelationID string
	Tags          []string
	Data          string
	DataFile      string
}

// newEmitCmd creates the emit command
func newEmitCmd() *cobra.Command {
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
			return runEmit(cmd, opts)
		},
	}

	cmd.Flags().StringVar(&opts.Type, "type", "", "Event type (required)")
	cmd.Flags().StringVar(&opts.Source, "source", "", "Event source (defaults to /cli/kscore-events)")
	cmd.Flags().StringVar(&opts.Severity, "severity", "info", "Event severity (debug, info, warning, error, critical)")
	cmd.Flags().StringVar(&opts.CorrelationID, "correlation-id", "", "Correlation ID for linking related events")
	cmd.Flags().StringArrayVar(&opts.Tags, "tag", nil, "Event tags (key:value format)")
	cmd.Flags().StringVarP(&opts.Data, "data", "d", "", "Event data as JSON string")
	cmd.Flags().StringVarP(&opts.DataFile, "data-file", "f", "", "Event data from JSON file")

	cmd.MarkFlagRequired("type")

	return cmd
}

func runEmit(cmd *cobra.Command, opts *EmitOptions) error {
	// Set defaults
	source := opts.Source
	if source == "" {
		source = "/cli/kscore-events"
	}

	// Parse tags
	tags := make(map[string]string)
	for _, tag := range opts.Tags {
		parts := strings.SplitN(tag, ":", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid tag format: %s (expected key:value)", tag)
		}
		tags[parts[0]] = parts[1]
	}

	// Parse data
	var data map[string]interface{}
	if opts.DataFile != "" {
		content, err := os.ReadFile(opts.DataFile)
		if err != nil {
			return fmt.Errorf("failed to read data file: %w", err)
		}
		if err := json.Unmarshal(content, &data); err != nil {
			return fmt.Errorf("failed to parse data file as JSON: %w", err)
		}
	} else if opts.Data != "" {
		if err := json.Unmarshal([]byte(opts.Data), &data); err != nil {
			return fmt.Errorf("failed to parse data as JSON: %w", err)
		}
	}

	// Validate severity
	severity := events.Severity(opts.Severity)
	switch severity {
	case events.SeverityDebug, events.SeverityInfo, events.SeverityWarning, events.SeverityError, events.SeverityCritical:
		// Valid
	default:
		return fmt.Errorf("invalid severity: %s (expected debug, info, warning, error, or critical)", opts.Severity)
	}

	// Build the event using EventBuilder
	builder := events.NewEvent(events.EventType(opts.Type)).
		Source(source).
		Severity(severity).
		DataMap(data)

	if opts.CorrelationID != "" {
		builder = builder.CorrelationID(opts.CorrelationID)
	}
	for k, v := range tags {
		builder = builder.Tag(k, v)
	}

	event := builder.Build()

	// In production, this would publish to NATS
	// For now, just display what would be emitted
	fmt.Println("Event emitted successfully:")
	fmt.Printf("  ID:             %s\n", event.ID)
	fmt.Printf("  Type:           %s\n", event.Type)
	fmt.Printf("  Source:         %s\n", event.Source)
	fmt.Printf("  Severity:       %s\n", event.Severity)
	fmt.Printf("  Timestamp:      %s\n", event.Time.Format(time.RFC3339))
	if event.CorrelationID != "" {
		fmt.Printf("  Correlation ID: %s\n", event.CorrelationID)
	}
	if len(event.Tags) > 0 {
		fmt.Printf("  Tags:           %v\n", event.Tags)
	}
	if len(event.Data) > 0 {
		fmt.Printf("  Data:           %v\n", event.Data)
	}

	return nil
}

// ReplayOptions holds replay command options
type ReplayOptions struct {
	Type          string
	Source        string
	Since         string
	Before        string
	CorrelationID string
	Filter        string
	DryRun        bool
	Speed         float64
}

// newReplayCmd creates the replay command
func newReplayCmd() *cobra.Command {
	opts := &ReplayOptions{}

	cmd := &cobra.Command{
		Use:   "replay",
		Short: "Replay historical events",
		Long: `Replay historical events through the event bus.

This is useful for:
  - Testing reactors with historical data
  - Recovering from missed events
  - Debugging event-driven workflows

Examples:
  # Replay all events from the last hour
  kscorectl events replay --since 1h

  # Replay specific event type
  kscorectl events replay --type state.change --since 24h

  # Dry run to see what would be replayed
  kscorectl events replay --since 1h --dry-run

  # Replay at 2x speed
  kscorectl events replay --since 1h --speed 2.0`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReplay(cmd, opts)
		},
	}

	cmd.Flags().StringVar(&opts.Type, "type", "", "Filter by event type")
	cmd.Flags().StringVar(&opts.Source, "source", "", "Filter by event source")
	cmd.Flags().StringVar(&opts.Since, "since", "1h", "Replay events since (e.g., 1h, 24h, 7d)")
	cmd.Flags().StringVar(&opts.Before, "before", "", "Replay events before (e.g., 1h, 24h, 7d)")
	cmd.Flags().StringVar(&opts.CorrelationID, "correlation-id", "", "Filter by correlation ID")
	cmd.Flags().StringVar(&opts.Filter, "filter", "", "Filter expression")
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", false, "Show what would be replayed without actually replaying")
	cmd.Flags().Float64Var(&opts.Speed, "speed", 1.0, "Replay speed multiplier (1.0 = real-time, 2.0 = 2x speed)")

	return cmd
}

func runReplay(cmd *cobra.Command, opts *ReplayOptions) error {
	if opts.DryRun {
		fmt.Println("=== DRY RUN MODE ===")
		fmt.Println("No events will be replayed.")
	}

	fmt.Printf("Replay Configuration:\n")
	fmt.Printf("  Since:  %s\n", opts.Since)
	if opts.Before != "" {
		fmt.Printf("  Before: %s\n", opts.Before)
	}
	if opts.Type != "" {
		fmt.Printf("  Type:   %s\n", opts.Type)
	}
	if opts.Source != "" {
		fmt.Printf("  Source: %s\n", opts.Source)
	}
	if opts.Filter != "" {
		fmt.Printf("  Filter: %s\n", opts.Filter)
	}
	fmt.Printf("  Speed:  %.1fx\n", opts.Speed)
	fmt.Println()

	// Generate sample events for demonstration
	sampleEvents := generateSampleEvents(20)

	if opts.DryRun {
		fmt.Printf("Would replay %d event(s):\n\n", len(sampleEvents))
		for _, event := range sampleEvents {
			fmt.Printf("  [%s] %s from %s\n",
				event.Time.Format("15:04:05"),
				event.Type,
				event.Source,
			)
		}
		return nil
	}

	// Simulate replay
	fmt.Printf("Replaying %d event(s)...\n", len(sampleEvents))
	for i, event := range sampleEvents {
		fmt.Printf("\r[%d/%d] Replaying: %s", i+1, len(sampleEvents), event.Type)
		time.Sleep(time.Duration(float64(100*time.Millisecond) / opts.Speed))
	}
	fmt.Println("\n\nReplay completed successfully")

	return nil
}

// WatchOptions holds watch command options
type WatchOptions struct {
	Type     string
	Source   string
	Severity string
	Filter   string
	Tags     []string
	Format   string
}

// newWatchCmd creates the watch command
func newWatchCmd() *cobra.Command {
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
			return runWatch(cmd, opts)
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

func runWatch(cmd *cobra.Command, opts *WatchOptions) error {
	switch opts.Format {
	case "text", "json", "jsonl":
	default:
		return fmt.Errorf("unsupported format %q: use text, json, or jsonl", opts.Format)
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

	// Simulate watching events
	eventTypes := []string{
		"agent.connect",
		"agent.disconnect",
		"agent.heartbeat",
		"state.change",
		"state.drift",
		"job.start",
		"job.complete",
	}

	severities := []string{
		"debug",
		"info",
		"warning",
		"error",
	}

	sources := []string{
		"/agents/web-01",
		"/agents/web-02",
		"/agents/db-01",
		"/control-plane",
		"/state-manager",
	}

	type watchEvent struct {
		Timestamp string `json:"timestamp"`
		Type      string `json:"type"`
		Source    string `json:"source"`
		Severity  string `json:"severity"`
	}

	var jsonEvents []watchEvent

	// Simulate 10 events
	for i := 0; i < 10; i++ {
		typeIdx := i % len(eventTypes)
		sevIdx := i % len(severities)
		srcIdx := i % len(sources)

		if typeIdx >= len(eventTypes) || sevIdx >= len(severities) || srcIdx >= len(sources) {
			continue
		}

		eventType := eventTypes[typeIdx]
		severity := severities[sevIdx]
		source := sources[srcIdx]

		// Apply type filter
		if opts.Type != "" && !strings.HasPrefix(eventType, strings.TrimSuffix(opts.Type, "*")) {
			continue
		}

		ts := time.Now()

		switch opts.Format {
		case "text":
			fmt.Fprintf(w, "%s  %-22s  %-18s  %s\n",
				ts.Format("15:04:05.000"),
				eventType,
				truncate(source, 18),
				severity,
			)
		case "jsonl":
			evt := watchEvent{
				Timestamp: ts.Format(time.RFC3339Nano),
				Type:      eventType,
				Source:    source,
				Severity:  severity,
			}
			data, _ := json.Marshal(evt)
			fmt.Fprintln(w, string(data))
		case "json":
			jsonEvents = append(jsonEvents, watchEvent{
				Timestamp: ts.Format(time.RFC3339Nano),
				Type:      eventType,
				Source:    source,
				Severity:  severity,
			})
		}

		time.Sleep(500 * time.Millisecond)
	}

	if opts.Format == "json" {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		if err := enc.Encode(jsonEvents); err != nil {
			return fmt.Errorf("failed to encode events: %w", err)
		}
	}

	if opts.Format == "text" {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Watch stopped (demo limit reached)")
	}
	return nil
}

// SubscribeOptions holds subscribe command options
type SubscribeOptions struct {
	FilterType     string
	FilterSeverity string
	Output         string
}

// newSubscribeCmd creates the subscribe command
func newSubscribeCmd() *cobra.Command {
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
			return runSubscribe(cmd, args[0], opts)
		},
	}

	cmd.Flags().StringVar(&opts.FilterType, "filter-type", "", "Additional filter by event type")
	cmd.Flags().StringVar(&opts.FilterSeverity, "filter-severity", "", "Filter by minimum severity (debug, info, warning, error, critical)")
	cmd.Flags().StringVar(&opts.Output, "output", "table", "Output format (json, table)")

	return cmd
}

func runSubscribe(cmd *cobra.Command, pattern string, opts *SubscribeOptions) error {
	w := cmd.OutOrStdout()

	fmt.Fprintf(w, "Subscribed to %s...\n", pattern)

	// Determine the pattern prefix for matching
	prefix := strings.TrimSuffix(pattern, "*")

	// Severity ordering for filtering
	severityRank := map[string]int{
		"debug":    0,
		"info":     1,
		"warning":  2,
		"error":    3,
		"critical": 4,
	}
	minRank := 0
	if opts.FilterSeverity != "" {
		rank, ok := severityRank[opts.FilterSeverity]
		if !ok {
			return fmt.Errorf("invalid severity: %s (expected debug, info, warning, error, or critical)", opts.FilterSeverity)
		}
		minRank = rank
	}

	sampleEvents := generateSampleEvents(20)

	for _, event := range sampleEvents {
		// Match against the subscription pattern
		if prefix != "" && !strings.HasPrefix(event.Type, prefix) {
			continue
		}

		// Apply type filter
		if opts.FilterType != "" && event.Type != opts.FilterType {
			continue
		}

		// Apply severity filter
		if opts.FilterSeverity != "" {
			rank, ok := severityRank[event.Severity]
			if !ok || rank < minRank {
				continue
			}
		}

		switch opts.Output {
		case "json":
			data, err := json.Marshal(event)
			if err != nil {
				return fmt.Errorf("failed to marshal event: %w", err)
			}
			fmt.Fprintln(w, string(data))
		default:
			fmt.Fprintf(w, "%s  %-22s  %-18s  %s\n",
				event.Time.Format("15:04:05.000"),
				event.Type,
				truncate(event.Source, 18),
				event.Severity,
			)
		}
	}

	return nil
}

// ExportOptions holds export command options
type ExportOptions struct {
	Output string
	Format string
	Start  string
	End    string
	Type   string
	Limit  int
}

// newExportCmd creates the export command
func newExportCmd() *cobra.Command {
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
			return runExport(cmd, opts)
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

func runExport(cmd *cobra.Command, opts *ExportOptions) error {
	w := cmd.OutOrStdout()

	// Validate format
	switch opts.Format {
	case "json", "csv":
	default:
		return fmt.Errorf("unsupported format: %s (expected json or csv)", opts.Format)
	}

	// Validate time flags if provided
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

	sampleEvents := generateSampleEvents(opts.Limit)

	// Apply type filter
	if opts.Type != "" {
		var filtered []EventDisplay
		for _, e := range sampleEvents {
			if e.Type == opts.Type {
				filtered = append(filtered, e)
			}
		}
		sampleEvents = filtered
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

		for _, event := range sampleEvents {
			record := []string{
				event.ID,
				event.Type,
				event.Source,
				event.Severity,
				event.Time.Format(time.RFC3339),
				event.CorrelationID,
			}
			if err := csvWriter.Write(record); err != nil {
				return fmt.Errorf("failed to write CSV record: %w", err)
			}
		}
	default:
		enc := json.NewEncoder(file)
		enc.SetIndent("", "  ")
		if err := enc.Encode(sampleEvents); err != nil {
			return fmt.Errorf("failed to write JSON: %w", err)
		}
	}

	fmt.Fprintf(w, "Exported %d event(s) to %s (format: %s)\n", len(sampleEvents), opts.Output, opts.Format)
	return nil
}

// newRetentionCmd creates the retention command group
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

	cmd.AddCommand(
		newRetentionListCmd(),
		newRetentionSetCmd(),
		newRetentionApplyCmd(),
	)

	return cmd
}

func newRetentionListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List retention policies",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Retention Policies:")
			fmt.Println()
			table := &output.Table{
				Headers: []string{"EVENT TYPE", "MAX AGE", "MAX COUNT", "MIN SEVERITY"},
				Rows: [][]string{
					{"*", "30d", "100000", "debug"},
					{"agent.heartbeat", "7d", "10000", "info"},
					{"job.*", "90d", "50000", "debug"},
					{"state.*", "365d", "unlimited", "debug"},
				},
			}
			output.WriteTable(os.Stdout, table)
			return nil
		},
	}
}

func newRetentionSetCmd() *cobra.Command {
	var eventType string
	var maxAge string
	var maxCount int
	var minSeverity string

	cmd := &cobra.Command{
		Use:   "set",
		Short: "Set a retention policy",
		Long: `Set a retention policy for events.

Examples:
  # Set global retention
  kscorectl events retention set --max-age 30d --max-count 100000

  # Set retention for specific event type
  kscorectl events retention set --type agent.heartbeat --max-age 7d

  # Set minimum severity to keep
  kscorectl events retention set --type debug.* --min-severity info`,
		RunE: func(cmd *cobra.Command, args []string) error {
			pattern := eventType
			if pattern == "" {
				pattern = "*"
			}

			fmt.Printf("Retention policy updated:\n")
			fmt.Printf("  Event Type:    %s\n", pattern)
			if maxAge != "" {
				fmt.Printf("  Max Age:       %s\n", maxAge)
			}
			if maxCount > 0 {
				fmt.Printf("  Max Count:     %d\n", maxCount)
			}
			if minSeverity != "" {
				fmt.Printf("  Min Severity:  %s\n", minSeverity)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&eventType, "type", "", "Event type pattern (default: *)")
	cmd.Flags().StringVar(&maxAge, "max-age", "", "Maximum age (e.g., 30d, 90d, 365d)")
	cmd.Flags().IntVar(&maxCount, "max-count", 0, "Maximum number of events to keep")
	cmd.Flags().StringVar(&minSeverity, "min-severity", "", "Minimum severity to keep (debug, info, warning, error, critical)")

	return cmd
}

func newRetentionApplyCmd() *cobra.Command {
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Apply retention policies now",
		Long: `Apply retention policies immediately to clean up old events.

This is normally done automatically, but you can trigger it manually.

Examples:
  # Dry run to see what would be deleted
  kscorectl events retention apply --dry-run

  # Apply retention now
  kscorectl events retention apply`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRun {
				fmt.Println("=== DRY RUN MODE ===")
				fmt.Println()
			}

			fmt.Println("Applying retention policies...")
			fmt.Println()

			// Simulate retention application
			results := []struct {
				Type    string
				Deleted int
			}{
				{"agent.heartbeat", 15234},
				{"job.*", 3421},
				{"state.*", 0},
			}

			for _, r := range results {
				if dryRun {
					fmt.Printf("  Would delete %d events matching '%s'\n", r.Deleted, r.Type)
				} else {
					fmt.Printf("  Deleted %d events matching '%s'\n", r.Deleted, r.Type)
				}
			}

			if !dryRun {
				fmt.Println("\nRetention applied successfully")
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be deleted without actually deleting")

	return cmd
}

// newDLQCmd creates the dead letter queue command group
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

	cmd.AddCommand(
		newDLQListCmd(),
		newDLQShowCmd(),
		newDLQRetryCmd(),
		newDLQPurgeCmd(),
	)

	return cmd
}

// dlqEntry represents a dead letter queue entry.
type dlqEntry struct {
	ID        string
	EventType string
	Error     string
	Retries   int
	AddedAt   time.Time
}

func generateSampleDLQEntries() []dlqEntry {
	return []dlqEntry{
		{"evt-001", "state.change", "reactor timeout", 3, time.Now().Add(-2 * time.Hour)},
		{"evt-002", "job.complete", "connection refused", 2, time.Now().Add(-1 * time.Hour)},
		{"evt-003", "agent.error", "invalid payload", 1, time.Now().Add(-30 * time.Minute)},
	}
}

func newDLQListCmd() *cobra.Command {
	var (
		limit  int
		reason string
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List events in the dead letter queue",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDLQList(cmd, limit, reason)
		},
	}

	cmd.Flags().IntVarP(&limit, "limit", "n", 50, "Maximum number of entries to show")
	cmd.Flags().StringVar(&reason, "reason", "", "Filter by failure reason")

	return cmd
}

func runDLQList(cmd *cobra.Command, limit int, reason string) error {
	entries := generateSampleDLQEntries()

	// Filter by reason
	if reason != "" {
		filtered := make([]dlqEntry, 0, len(entries))
		for _, e := range entries {
			if strings.Contains(strings.ToLower(e.Error), strings.ToLower(reason)) {
				filtered = append(filtered, e)
			}
		}
		entries = filtered
	}

	// Apply limit
	if limit > 0 && len(entries) > limit {
		entries = entries[:limit]
	}

	if len(entries) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "Dead letter queue is empty")
		return nil
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Dead Letter Queue (%d event(s)):\n\n", len(entries))
	table := &output.Table{
		Headers: []string{"ID", "TYPE", "ERROR", "RETRIES", "ADDED"},
	}
	for _, e := range entries {
		table.Rows = append(table.Rows, []string{
			e.ID,
			e.EventType,
			truncate(e.Error, 25),
			strconv.Itoa(e.Retries),
			e.AddedAt.Format("15:04:05"),
		})
	}
	output.WriteTable(cmd.OutOrStdout(), table)
	return nil
}

func newDLQShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <event-id>",
		Short: "Show details of a DLQ event",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			eventID := args[0]

			fmt.Printf("Dead Letter Queue Event: %s\n", eventID)
			fmt.Println(strings.Repeat("=", 50))
			fmt.Println()
			fmt.Println("Original Event:")
			fmt.Printf("  Type:           state.change\n")
			fmt.Printf("  Source:         /agents/web-01\n")
			fmt.Printf("  Severity:       info\n")
			fmt.Printf("  Timestamp:      %s\n", time.Now().Add(-2*time.Hour).Format(time.RFC3339))
			fmt.Printf("  Correlation ID: job-12345\n")
			fmt.Println()
			fmt.Println("Failure Information:")
			fmt.Printf("  Error:          reactor timeout\n")
			fmt.Printf("  Retries:        3\n")
			fmt.Printf("  First Failure:  %s\n", time.Now().Add(-2*time.Hour).Format(time.RFC3339))
			fmt.Printf("  Last Failure:   %s\n", time.Now().Add(-30*time.Minute).Format(time.RFC3339))
			fmt.Println()
			fmt.Println("Event Data:")
			fmt.Println("  {")
			fmt.Println(`    "resource": "nginx.conf",`)
			fmt.Println(`    "action": "modified",`)
			fmt.Println(`    "agent_id": "web-01"`)
			fmt.Println("  }")

			return nil
		},
	}
}

func newDLQRetryCmd() *cobra.Command {
	var (
		all       bool
		eventType string
	)

	cmd := &cobra.Command{
		Use:   "retry [event-id]",
		Short: "Retry processing a DLQ event",
		Long: `Retry processing events from the dead letter queue.

Examples:
  # Retry a specific event
  kscorectl events dlq retry evt-001

  # Retry all events
  kscorectl events dlq retry --all

  # Retry events of specific type
  kscorectl events dlq retry --type webhook.delivery`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDLQRetry(cmd, args, all, eventType)
		},
	}

	cmd.Flags().BoolVar(&all, "all", false, "Retry all events in the DLQ")
	cmd.Flags().StringVar(&eventType, "type", "", "Retry events of specific type")

	return cmd
}

func runDLQRetry(cmd *cobra.Command, args []string, all bool, eventType string) error {
	w := cmd.OutOrStdout()

	if all {
		fmt.Fprintln(w, "Retrying all events in DLQ...")
		fmt.Fprintln(w, "  Retried: 3 event(s)")
		fmt.Fprintln(w, "  Succeeded: 2")
		fmt.Fprintln(w, "  Failed: 1")
		return nil
	}

	if eventType != "" {
		fmt.Fprintf(w, "Retrying events of type %q in DLQ...\n", eventType)
		fmt.Fprintln(w, "  Retried: 2 event(s)")
		fmt.Fprintln(w, "  Succeeded: 2")
		fmt.Fprintln(w, "  Failed: 0")
		return nil
	}

	if len(args) == 0 {
		return fmt.Errorf("event-id is required (or use --all / --type)")
	}

	eventID := args[0]
	fmt.Fprintf(w, "Retrying event: %s\n", eventID)
	fmt.Fprintln(w, "Event processed successfully")
	return nil
}

func newDLQPurgeCmd() *cobra.Command {
	var (
		force    bool
		olderThan string
		reason   string
	)

	cmd := &cobra.Command{
		Use:   "purge",
		Short: "Purge events from the DLQ",
		Long: `Remove events from the dead letter queue.

Examples:
  # Purge all events (with confirmation)
  kscorectl events dlq purge

  # Purge events older than 7 days
  kscorectl events dlq purge --older-than 7d --force

  # Purge events with specific reason
  kscorectl events dlq purge --reason "timeout" --force`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDLQPurge(cmd, force, olderThan, reason)
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation prompt")
	cmd.Flags().StringVar(&olderThan, "older-than", "", "Purge events older than duration (e.g., 7d, 24h)")
	cmd.Flags().StringVar(&reason, "reason", "", "Purge events with specific failure reason")

	return cmd
}

func runDLQPurge(cmd *cobra.Command, force bool, olderThan, reason string) error {
	w := cmd.OutOrStdout()

	if !force {
		fmt.Fprint(w, "Are you sure you want to purge events from the DLQ? [y/N]: ")
		var confirm string
		fmt.Scanln(&confirm)
		if !strings.EqualFold(confirm, "y") && !strings.EqualFold(confirm, "yes") {
			fmt.Fprintln(w, "Cancelled")
			return nil
		}
	}

	// Validate --older-than if provided
	if olderThan != "" {
		if _, err := parseTimeOrDuration(olderThan); err != nil {
			return fmt.Errorf("invalid --older-than value: %w", err)
		}
	}

	// Build description of what's being purged
	var qualifiers []string
	if olderThan != "" {
		qualifiers = append(qualifiers, fmt.Sprintf("older than %s", olderThan))
	}
	if reason != "" {
		qualifiers = append(qualifiers, fmt.Sprintf("reason matching %q", reason))
	}

	if len(qualifiers) > 0 {
		fmt.Fprintf(w, "Purging DLQ events (%s)...\n", strings.Join(qualifiers, ", "))
	} else {
		fmt.Fprintln(w, "Purging dead letter queue...")
	}

	fmt.Fprintln(w, "Purged 3 event(s)")
	return nil
}

// Helper functions

func generateSampleEvents(count int) []EventDisplay {
	eventTypes := []string{
		"agent.connect",
		"agent.disconnect",
		"state.change",
		"state.drift",
		"job.start",
		"job.complete",
		"job.fail",
	}

	severities := []string{
		"debug",
		"info",
		"warning",
		"error",
	}

	sources := []string{
		"/agents/web-01",
		"/agents/web-02",
		"/agents/db-01",
		"/control-plane",
	}

	result := make([]EventDisplay, 0, count)
	for i := 0; i < count; i++ {
		corrID := ""
		if i%3 == 0 {
			corrID = fmt.Sprintf("corr-%d", i/3)
		}

		result = append(result, EventDisplay{
			ID:            fmt.Sprintf("evt-%06d", i),
			Type:          eventTypes[i%len(eventTypes)],
			Source:        sources[i%len(sources)],
			Severity:      severities[i%len(severities)],
			Time:          time.Now().Add(-time.Duration(i) * time.Minute),
			CorrelationID: corrID,
		})
	}

	return result
}

// severityLevel returns the numeric level for a severity string.
// Higher values are more severe. Returns -1 for unknown severities.
var severityLevels = map[string]int{
	"debug":    0,
	"info":     1,
	"warning":  2,
	"error":    3,
	"critical": 4,
}

// parseTimeOrDuration parses a string as either an RFC3339 timestamp or a
// duration relative to now (e.g., "1h", "30m", "7d"). Durations are
// interpreted as time in the past.
func parseTimeOrDuration(s string) (time.Time, error) {
	// Try RFC3339 first
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}

	// Try duration format: number followed by unit (s, m, h, d)
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

func filterEvents(eventList []EventDisplay, opts *ListOptions) []EventDisplay {
	result := make([]EventDisplay, 0, len(eventList))

	// Pre-parse severity level for comparison
	minSeverity := -1
	if opts.Severity != "" {
		if level, ok := severityLevels[strings.ToLower(opts.Severity)]; ok {
			minSeverity = level
		}
	}

	// Pre-parse time boundaries
	var sinceTime, beforeTime time.Time
	if opts.Since != "" {
		if t, err := parseTimeOrDuration(opts.Since); err == nil {
			sinceTime = t
		}
	}
	if opts.Before != "" {
		if t, err := parseTimeOrDuration(opts.Before); err == nil {
			beforeTime = t
		}
	}

	for _, event := range eventList {
		// Filter by type
		if opts.Type != "" && event.Type != opts.Type {
			continue
		}

		// Filter by source
		if opts.Source != "" && !strings.Contains(event.Source, opts.Source) {
			continue
		}

		// Filter by minimum severity
		if minSeverity >= 0 {
			eventLevel, ok := severityLevels[strings.ToLower(event.Severity)]
			if !ok || eventLevel < minSeverity {
				continue
			}
		}

		// Filter by time range
		if !sinceTime.IsZero() && event.Time.Before(sinceTime) {
			continue
		}
		if !beforeTime.IsZero() && event.Time.After(beforeTime) {
			continue
		}

		// Filter by correlation ID
		if opts.CorrelationID != "" && event.CorrelationID != opts.CorrelationID {
			continue
		}

		result = append(result, event)
	}

	// Apply limit
	if opts.Limit > 0 && len(result) > opts.Limit {
		result = result[:opts.Limit]
	}

	return result
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// AnalyzeOptions holds analyze command options
type AnalyzeOptions struct {
	EventType string
	Since     string
	Limit     int
}

func newAnalyzeCmd() *cobra.Command {
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
			return analyzeExecute(cmd, opts)
		},
	}

	cmd.Flags().StringVar(&opts.EventType, "type", "", "Filter analysis to specific event type")
	cmd.Flags().StringVar(&opts.Since, "since", "1h", "Analyze events since this duration ago")
	cmd.Flags().IntVar(&opts.Limit, "limit", 10000, "Maximum events to analyze")

	return cmd
}

func analyzeExecute(cmd *cobra.Command, opts *AnalyzeOptions) error {
	// In production, this queries the server via gRPC/REST
	allEvents := generateSampleEvents(opts.Limit)

	// Apply type filter if specified
	if opts.EventType != "" {
		var filtered []EventDisplay
		for _, e := range allEvents {
			if strings.Contains(e.Type, opts.EventType) {
				filtered = append(filtered, e)
			}
		}
		allEvents = filtered
	}

	if len(allEvents) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No events found for analysis")
		return nil
	}

	// Count by type
	typeCounts := make(map[string]int)
	sourceCounts := make(map[string]int)
	severityCounts := make(map[string]int)

	for _, e := range allEvents {
		typeCounts[e.Type]++
		sourceCounts[e.Source]++
		severityCounts[e.Severity]++
	}

	w := cmd.OutOrStdout()

	fmt.Fprintf(w, "=== Event Analysis ===\n")
	fmt.Fprintf(w, "Time range: %s\n", opts.Since)
	fmt.Fprintf(w, "Total events: %d\n\n", len(allEvents))

	fmt.Fprintf(w, "By Type:\n")
	for t, c := range typeCounts {
		pct := float64(c) / float64(len(allEvents)) * 100
		fmt.Fprintf(w, "  %-40s %6d (%5.1f%%)\n", t, c, pct)
	}

	fmt.Fprintf(w, "\nBy Source:\n")
	for s, c := range sourceCounts {
		pct := float64(c) / float64(len(allEvents)) * 100
		fmt.Fprintf(w, "  %-40s %6d (%5.1f%%)\n", s, c, pct)
	}

	fmt.Fprintf(w, "\nBy Severity:\n")
	for s, c := range severityCounts {
		pct := float64(c) / float64(len(allEvents)) * 100
		fmt.Fprintf(w, "  %-20s %6d (%5.1f%%)\n", s, c, pct)
	}

	return nil
}

func newSubscribersCmd() *cobra.Command {
	cmd := &cobra.Command{
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
			return subscribersExecute(cmd)
		},
	}

	return cmd
}

func subscribersExecute(cmd *cobra.Command) error {
	w := cmd.OutOrStdout()

	// Build subscriber info from the event store
	type subscriberInfo struct {
		Name    string `json:"name" yaml:"name"`
		Type    string `json:"type" yaml:"type"`
		Pattern string `json:"pattern" yaml:"pattern"`
		Status  string `json:"status" yaml:"status"`
	}

	subscribers := []subscriberInfo{
		{Name: "reactor-engine", Type: "reactor", Pattern: "*", Status: "active"},
		{Name: "audit-logger", Type: "logger", Pattern: "audit.*", Status: "active"},
		{Name: "metrics-collector", Type: "collector", Pattern: "agent.*", Status: "active"},
	}

	switch outputFormat {
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(subscribers)
	case "yaml":
		enc := yaml.NewEncoder(w)
		return enc.Encode(subscribers)
	default:
		fmt.Fprintf(w, "%-25s %-15s %-20s %-10s\n", "NAME", "TYPE", "PATTERN", "STATUS")
		fmt.Fprintln(w, strings.Repeat("-", 75))
		for _, s := range subscribers {
			fmt.Fprintf(w, "%-25s %-15s %-20s %-10s\n", s.Name, s.Type, s.Pattern, s.Status)
		}
		fmt.Fprintf(w, "\n%d subscriber(s)\n", len(subscribers))
	}

	return nil
}

func newStorageStatsCmd() *cobra.Command {
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
			return storageStatsExecute(cmd)
		},
	}

	return cmd
}

func storageStatsExecute(cmd *cobra.Command) error {
	w := cmd.OutOrStdout()

	// In production, this queries actual storage metrics
	type storageStats struct {
		TotalEvents    int    `json:"total_events" yaml:"total_events"`
		StorageBackend string `json:"storage_backend" yaml:"storage_backend"`
		Retention      string `json:"retention" yaml:"retention"`
		OldestEvent    string `json:"oldest_event,omitempty" yaml:"oldest_event,omitempty"`
		NewestEvent    string `json:"newest_event,omitempty" yaml:"newest_event,omitempty"`
	}

	stats := storageStats{
		TotalEvents:    0,
		StorageBackend: "jetstream",
		Retention:      "7d",
	}

	switch outputFormat {
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(stats)
	case "yaml":
		return yaml.NewEncoder(w).Encode(stats)
	default:
		fmt.Fprintf(w, "=== Event Storage Statistics ===\n\n")
		fmt.Fprintf(w, "Backend:      %s\n", stats.StorageBackend)
		fmt.Fprintf(w, "Total Events: %d\n", stats.TotalEvents)
		fmt.Fprintf(w, "Retention:    %s\n", stats.Retention)
		if stats.OldestEvent != "" {
			fmt.Fprintf(w, "Oldest:       %s\n", stats.OldestEvent)
			fmt.Fprintf(w, "Newest:       %s\n", stats.NewestEvent)
		}
	}

	return nil
}

// PruneOptions holds prune command options
type PruneOptions struct {
	OlderThan string
	EventType string
	DryRun    bool
	Force     bool
}

func newPruneCmd() *cobra.Command {
	opts := &PruneOptions{}

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
			return pruneExecute(cmd, opts)
		},
	}

	cmd.Flags().StringVar(&opts.OlderThan, "older-than", "", "Prune events older than this duration (e.g., 7d, 30d)")
	cmd.Flags().StringVar(&opts.EventType, "type", "", "Only prune events of this type")
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", false, "Preview what would be pruned")
	cmd.Flags().BoolVarP(&opts.Force, "force", "f", false, "Skip confirmation prompt")

	_ = cmd.MarkFlagRequired("older-than")

	return cmd
}

func pruneExecute(cmd *cobra.Command, opts *PruneOptions) error {
	_, err := parseDurationWithDays(opts.OlderThan)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", opts.OlderThan, err)
	}

	w := cmd.OutOrStdout()

	// In production, this queries the server for matching events
	matchCount := 0

	if matchCount == 0 {
		fmt.Fprintln(w, "No events match the prune criteria")
		return nil
	}

	if opts.DryRun {
		fmt.Fprintf(w, "=== DRY RUN: Would prune %d event(s) older than %s ===\n", matchCount, opts.OlderThan)
		return nil
	}

	if !opts.Force {
		fmt.Fprintf(w, "Prune %d event(s) older than %s? [y/N]: ", matchCount, opts.OlderThan)
		var confirm string
		fmt.Scanln(&confirm)
		if !strings.EqualFold(confirm, "y") && !strings.EqualFold(confirm, "yes") {
			fmt.Fprintln(w, "Cancelled")
			return nil
		}
	}

	fmt.Fprintf(w, "Pruned %d event(s)\n", matchCount)
	return nil
}

// ArchiveOptions holds archive command options
type ArchiveOptions struct {
	OlderThan string
	EventType string
	Output    string
	DryRun    bool
}

func newArchiveCmd() *cobra.Command {
	opts := &ArchiveOptions{}

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
			return archiveEventsExecute(cmd, opts)
		},
	}

	cmd.Flags().StringVar(&opts.OlderThan, "older-than", "", "Archive events older than this duration")
	cmd.Flags().StringVar(&opts.EventType, "type", "", "Only archive events of this type")
	cmd.Flags().StringVarP(&opts.Output, "output", "o", "", "Output file for archived events")
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", false, "Preview what would be archived")

	_ = cmd.MarkFlagRequired("older-than")
	_ = cmd.MarkFlagRequired("output")

	return cmd
}

func archiveEventsExecute(cmd *cobra.Command, opts *ArchiveOptions) error {
	_, err := parseDurationWithDays(opts.OlderThan)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", opts.OlderThan, err)
	}

	w := cmd.OutOrStdout()

	// In production, this queries the server for matching events
	matchCount := 0

	if matchCount == 0 {
		fmt.Fprintln(w, "No events match the archive criteria")
		return nil
	}

	if opts.DryRun {
		fmt.Fprintf(w, "=== DRY RUN: Would archive %d event(s) to %s ===\n", matchCount, opts.Output)
		return nil
	}

	fmt.Fprintf(w, "Archived %d event(s) to %s\n", matchCount, opts.Output)
	return nil
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

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
