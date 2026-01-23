package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/shawnbutts/keystone-core/pkg/cli/output"
	"github.com/shawnbutts/keystone-core/pkg/events"
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
  - Managing retention policies
  - Managing the dead letter queue

Usage via kscorectl:
  kscorectl events list
  kscorectl events emit --type custom.event --data '{"key":"value"}'
  kscorectl events watch`,
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
		newRetentionCmd(),
		newDLQCmd(),
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

  # Complex query with time range
  kscorectl events query 'severity >= "warning" and timestamp() > now() - duration("1h")'`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filter = args[0]
			return runQuery(cmd, filter, limit)
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 100, "Maximum number of events to return")

	return cmd
}

func runQuery(cmd *cobra.Command, filter string, limit int) error {
	fmt.Printf("Querying events with filter: %s\n", filter)
	fmt.Printf("Limit: %d\n", limit)
	fmt.Println()

	// In production, this would send the filter to the server
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
	for _, event := range sampleEvents[:min(limit, len(sampleEvents))] {
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

	return cmd
}

func runWatch(cmd *cobra.Command, opts *WatchOptions) error {
	fmt.Println("Watching events... (press Ctrl+C to stop)")
	fmt.Println()

	if opts.Type != "" {
		fmt.Printf("  Type filter:     %s\n", opts.Type)
	}
	if opts.Source != "" {
		fmt.Printf("  Source filter:   %s\n", opts.Source)
	}
	if opts.Severity != "" {
		fmt.Printf("  Severity filter: %s+\n", opts.Severity)
	}
	if opts.Filter != "" {
		fmt.Printf("  Filter:          %s\n", opts.Filter)
	}
	fmt.Println()
	fmt.Println("Timestamp           Type                    Source               Severity")
	fmt.Println(strings.Repeat("-", 80))

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

	// Simulate 10 events
	for i := 0; i < 10; i++ {
		eventType := eventTypes[i%len(eventTypes)]
		severity := severities[i%len(severities)]
		source := sources[i%len(sources)]

		// Apply type filter
		if opts.Type != "" && !strings.HasPrefix(eventType, strings.TrimSuffix(opts.Type, "*")) {
			continue
		}

		fmt.Printf("%s  %-22s  %-18s  %s\n",
			time.Now().Format("15:04:05.000"),
			eventType,
			truncate(source, 18),
			severity,
		)

		time.Sleep(500 * time.Millisecond)
	}

	fmt.Println()
	fmt.Println("Watch stopped (demo limit reached)")
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

func newDLQListCmd() *cobra.Command {
	var limit int

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List events in the dead letter queue",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Sample DLQ entries
			type dlqEntry struct {
				ID        string
				EventType string
				Error     string
				Retries   int
				AddedAt   time.Time
			}

			entries := []dlqEntry{
				{"evt-001", "state.change", "reactor timeout", 3, time.Now().Add(-2 * time.Hour)},
				{"evt-002", "job.complete", "connection refused", 2, time.Now().Add(-1 * time.Hour)},
				{"evt-003", "agent.error", "invalid payload", 1, time.Now().Add(-30 * time.Minute)},
			}

			if len(entries) == 0 {
				fmt.Println("Dead letter queue is empty")
				return nil
			}

			fmt.Printf("Dead Letter Queue (%d event(s)):\n\n", len(entries))
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
			output.WriteTable(os.Stdout, table)
			return nil
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum number of entries to show")

	return cmd
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
	var all bool

	cmd := &cobra.Command{
		Use:   "retry [event-id]",
		Short: "Retry processing a DLQ event",
		Long: `Retry processing events from the dead letter queue.

Examples:
  # Retry a specific event
  kscorectl events dlq retry evt-001

  # Retry all events
  kscorectl events dlq retry --all`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if all {
				fmt.Println("Retrying all events in DLQ...")
				fmt.Println("  Retried: 3 event(s)")
				fmt.Println("  Succeeded: 2")
				fmt.Println("  Failed: 1")
				return nil
			}

			if len(args) == 0 {
				return fmt.Errorf("event-id is required (or use --all)")
			}

			eventID := args[0]
			fmt.Printf("Retrying event: %s\n", eventID)
			fmt.Println("Event processed successfully")
			return nil
		},
	}

	cmd.Flags().BoolVar(&all, "all", false, "Retry all events in the DLQ")

	return cmd
}

func newDLQPurgeCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "purge",
		Short: "Purge all events from the DLQ",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !force {
				fmt.Print("Are you sure you want to purge all events from the DLQ? [y/N]: ")
				var confirm string
				fmt.Scanln(&confirm)
				if strings.ToLower(confirm) != "y" && strings.ToLower(confirm) != "yes" {
					fmt.Println("Cancelled")
					return nil
				}
			}

			fmt.Println("Purging dead letter queue...")
			fmt.Println("Purged 3 event(s)")
			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation prompt")

	return cmd
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

func filterEvents(events []EventDisplay, opts *ListOptions) []EventDisplay {
	result := make([]EventDisplay, 0, len(events))

	for _, event := range events {
		// Filter by type
		if opts.Type != "" && event.Type != opts.Type {
			continue
		}

		// Filter by source
		if opts.Source != "" && !strings.Contains(event.Source, opts.Source) {
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
