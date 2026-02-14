// Package main implements the kscore-audit CLI for policy evaluation auditing and compliance reporting.
package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/shawnbutts/keystone-core/internal/cli/auditutil"
	"github.com/shawnbutts/keystone-core/internal/cli/output"
	policyclient "github.com/shawnbutts/keystone-core/pkg/policy"
	"github.com/shawnbutts/keystone-core/pkg/version"
)

var (
	serverAddr   string
	outputFormat string
	auditLevel   string
	auditOutput  string
)

func newRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "kscore-audit",
		Short: "Policy audit and compliance reporting for Keystone Core",
		Long: `kscore-audit provides policy evaluation auditing and compliance reporting.

This command provides:
  - Policy evaluation audit logs
  - Compliance reporting and metrics
  - Trend analysis for policy evaluations
  - Export capabilities for audit data

Commands:
  log       - View policy evaluation audit logs
  report    - Generate compliance reports
  export    - Export audit data
  stats     - View audit statistics and trends

Examples:
  # View recent audit entries
  kscore-audit log

  # Filter by policy
  kscore-audit log --policy security-no-root

  # Show only denied evaluations
  kscore-audit log --denied

  # Generate compliance report for last 7 days
  kscore-audit report --days 7

  # Export audit data as JSON
  kscore-audit export --days 30 --output audit-data.json`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	rootCmd.PersistentFlags().StringVarP(&serverAddr, "server", "s", "localhost:9090", "Control plane server address")
	rootCmd.PersistentFlags().StringVarP(&outputFormat, "format", "o", "table", "Output format (table, text, json, yaml)")
	rootCmd.PersistentFlags().StringVar(&auditLevel, "audit-level", "all", "Audit logging level (all, errors, none)")
	rootCmd.PersistentFlags().StringVar(&auditOutput, "audit-output", "auto", "Audit output backend (auto, syslog, journald, stderr, none)")

	rootCmd.AddCommand(
		newVersionCmd(),
		newLogCommand(),
		newReportCommand(),
		newExportCommand(),
		newStatsCommand(),
		newSearchCommand(),
		newAnalyzeCommand(),
		newTimelineCommand(),
		newWatchCommand(),
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

func main() {
	rootCmd := newRootCmd()
	auditHandler := auditutil.Attach(rootCmd, "kscore-audit", &auditLevel, &auditOutput)
	if err := rootCmd.Execute(); err != nil {
		auditHandler.LogFailure(err)
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func createPolicyClient() (*policyclient.Client, error) {
	return policyclient.NewClient(serverAddr)
}

// ============================================================================
// Log Command
// ============================================================================

var (
	logPolicyID     string
	logResourceType string
	logDeniedOnly   bool
	logLimit        int
	logSince        string
	logUntil        string
)

func newLogCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "log",
		Short: "View policy evaluation audit log",
		Long: `Display the policy evaluation audit log.

Shows a history of policy evaluations including:
  - Timestamps and policy IDs
  - Evaluation results (allowed/denied)
  - Violation counts and details
  - Resource information

Examples:
  # Show recent audit entries
  kscore-audit log

  # Filter by policy
  kscore-audit log --policy security-no-root

  # Show only denied evaluations
  kscore-audit log --denied

  # Limit results
  kscore-audit log --limit 50

  # Filter by time range
  kscore-audit log --since 2026-01-01 --until 2026-01-17`,
		RunE: runLog,
	}

	cmd.Flags().StringVar(&logPolicyID, "policy", "", "Filter by policy ID")
	cmd.Flags().StringVar(&logResourceType, "resource-type", "", "Filter by resource type")
	cmd.Flags().BoolVar(&logDeniedOnly, "denied", false, "Show only denied evaluations")
	cmd.Flags().IntVar(&logLimit, "limit", 100, "Maximum entries to show")
	cmd.Flags().StringVar(&logSince, "since", "", "Show entries since this date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&logUntil, "until", "", "Show entries until this date (YYYY-MM-DD)")

	return cmd
}

func runLog(cmd *cobra.Command, args []string) error {
	client, err := createPolicyClient()
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w", err)
	}
	defer client.Close()

	opts := policyclient.AuditLogOptions{
		PolicyID:     logPolicyID,
		ResourceType: logResourceType,
		PageSize:     int32(logLimit), //nolint:gosec // G115: limit is small
	}

	if logDeniedOnly {
		denied := false
		opts.Allowed = &denied
	}

	if logSince != "" {
		t, parseErr := time.Parse("2006-01-02", logSince)
		if parseErr != nil {
			return fmt.Errorf("invalid --since date format (use YYYY-MM-DD): %w", parseErr)
		}
		opts.StartTime = t
	}
	if logUntil != "" {
		t, parseErr := time.Parse("2006-01-02", logUntil)
		if parseErr != nil {
			return fmt.Errorf("invalid --until date format (use YYYY-MM-DD): %w", parseErr)
		}
		t = t.Add(23*time.Hour + 59*time.Minute + 59*time.Second)
		opts.EndTime = t
	}

	result, err := client.GetAuditLog(context.Background(), opts)
	if err != nil {
		return fmt.Errorf("failed to get audit log: %w", err)
	}

	format, err := output.ParseFormat(outputFormat)
	if err != nil {
		return err
	}

	if len(result.Entries) == 0 {
		switch format {
		case output.FormatJSON:
			return output.WriteJSON(os.Stdout, result.Entries)
		case output.FormatYAML:
			return output.WriteYAML(os.Stdout, result.Entries)
		case output.FormatTable, output.FormatText:
			fmt.Println("No audit entries found.")
			return nil
		default:
			return fmt.Errorf("unsupported output format: %s", outputFormat)
		}
	}

	switch format {
	case output.FormatJSON:
		return output.WriteJSON(os.Stdout, result.Entries)
	case output.FormatYAML:
		return output.WriteYAML(os.Stdout, result.Entries)
	case output.FormatTable, output.FormatText:
		fmt.Printf("%-20s %-25s %-15s %-10s %-10s\n", "TIMESTAMP", "POLICY", "RESOURCE", "RESULT", "VIOLATIONS")
		fmt.Println(strings.Repeat("-", 85))
		for i := range result.Entries {
			entry := &result.Entries[i]
			entryResult := "ALLOWED"
			if !entry.Allowed {
				entryResult = "DENIED"
			}
			fmt.Printf("%-20s %-25s %-15s %-10s %-10d\n",
				entry.Timestamp.Format("2006-01-02 15:04:05"),
				truncate(entry.PolicyID, 25),
				truncate(entry.ResourceType, 15),
				entryResult,
				len(entry.Violations),
			)
		}

		fmt.Printf("\nTotal: %d entries\n", len(result.Entries))
		return nil
	default:
		return fmt.Errorf("unsupported output format: %s", outputFormat)
	}
}

// ============================================================================
// Report Command
// ============================================================================

var (
	reportDays     int
	reportCategory string
	reportSeverity string
)

func newReportCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Generate compliance report",
		Long: `Generate a compliance report based on policy evaluations.

Reports include:
  - Overall compliance rate
  - Violations by severity
  - Top violating policies
  - Trend analysis

Examples:
  # Generate report for last 7 days
  kscore-audit report --days 7

  # Generate report for last 30 days as JSON
  kscore-audit report --days 30 --format json

  # Filter by category
  kscore-audit report --days 7 --category security

  # Filter by severity
  kscore-audit report --days 7 --severity critical`,
		RunE: runReport,
	}

	cmd.Flags().IntVar(&reportDays, "days", 7, "Number of days to include in report")
	cmd.Flags().StringVar(&reportCategory, "category", "", "Filter by policy category")
	cmd.Flags().StringVar(&reportSeverity, "severity", "", "Filter by severity (low, medium, high, critical)")

	return cmd
}

func runReport(cmd *cobra.Command, args []string) error {
	client, err := createPolicyClient()
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w", err)
	}
	defer client.Close()

	now := time.Now()
	start := now.AddDate(0, 0, -reportDays)

	report, err := client.GetComplianceReport(context.Background(), policyclient.ComplianceReportOptions{
		StartTime:         start,
		EndTime:           now,
		IncludeViolations: true,
	})
	if err != nil {
		return fmt.Errorf("failed to get compliance report: %w", err)
	}

	format, err := output.ParseFormat(outputFormat)
	if err != nil {
		return err
	}

	switch format {
	case output.FormatJSON:
		return output.WriteJSON(os.Stdout, report)
	case output.FormatYAML:
		return output.WriteYAML(os.Stdout, report)
	case output.FormatTable:
		summary := buildKeyValueTable([][2]string{
			{"PERIOD", fmt.Sprintf("%s to %s", start.Format("2006-01-02"), now.Format("2006-01-02"))},
			{"TOTAL EVALUATIONS", fmt.Sprintf("%d", report.TotalEvaluations)},
			{"COMPLIANT", fmt.Sprintf("%d", report.CompliantEvaluations)},
			{"NON-COMPLIANT", fmt.Sprintf("%d", report.NonCompliantEvaluations)},
			{"COMPLIANCE RATE", fmt.Sprintf("%.1f%%", report.ComplianceRate)},
		})
		if err := output.WriteTable(os.Stdout, summary); err != nil {
			return err
		}

		if len(report.ViolationsBySeverity) > 0 {
			fmt.Println("\nViolations by Severity:")
			if err := output.WriteTable(os.Stdout, buildSeverityTable(report.ViolationsBySeverity)); err != nil {
				return err
			}
		}

		if len(report.TopViolations) > 0 {
			fmt.Println("\nTop Violations:")
			if err := output.WriteTable(os.Stdout, buildTopViolationsTable(report.TopViolations)); err != nil {
				return err
			}
		}

		if report.TotalEvaluations == 0 {
			fmt.Println("\nNote: No policy evaluation data found.")
		}
		return nil
	case output.FormatText:
		fmt.Println("=== Compliance Report ===")
		fmt.Printf("Period:    %s to %s\n", start.Format("2006-01-02"), now.Format("2006-01-02"))
		fmt.Println()

		fmt.Println("Summary:")
		fmt.Printf("  Total Evaluations:  %d\n", report.TotalEvaluations)
		fmt.Printf("  Compliant:          %d\n", report.CompliantEvaluations)
		fmt.Printf("  Non-Compliant:      %d\n", report.NonCompliantEvaluations)
		fmt.Printf("  Compliance Rate:    %.1f%%\n", report.ComplianceRate)
		fmt.Println()

		if len(report.ViolationsBySeverity) > 0 {
			fmt.Println("Violations by Severity:")
			for severity, count := range report.ViolationsBySeverity {
				fmt.Printf("  %-10s: %d\n", severity, count)
			}
			fmt.Println()
		}

		if len(report.TopViolations) > 0 {
			fmt.Println("Top Violations:")
			for i := range report.TopViolations {
				v := &report.TopViolations[i]
				fmt.Printf("  %d. %s (%s) - %d violations\n", i+1, v.PolicyName, v.Severity, v.Count)
			}
		}

		if report.TotalEvaluations == 0 {
			fmt.Println("\nNote: No policy evaluation data found.")
		}

		return nil
	default:
		return fmt.Errorf("unsupported output format: %s", outputFormat)
	}
}

// ============================================================================
// Export Command
// ============================================================================

var (
	exportDays       int
	exportOutputFile string
	exportFormat     string
)

func newExportCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export audit data",
		Long: `Export audit data for external analysis or archiving.

Supports multiple export formats:
  - JSON for programmatic access
  - CSV for spreadsheet analysis
  - YAML for configuration management

Examples:
  # Export last 30 days to JSON
  kscore-audit export --days 30 --output audit-data.json

  # Export to CSV
  kscore-audit export --days 7 --output audit-data.csv --export-format csv

  # Export to stdout as YAML
  kscore-audit export --days 7 --export-format yaml`,
		RunE: runExport,
	}

	cmd.Flags().IntVar(&exportDays, "days", 30, "Number of days to export")
	cmd.Flags().StringVar(&exportOutputFile, "output", "", "Output file (default: stdout)")
	cmd.Flags().StringVar(&exportFormat, "export-format", "json", "Export format (json, csv, yaml)")

	return cmd
}

func runExport(cmd *cobra.Command, args []string) error {
	client, err := createPolicyClient()
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w", err)
	}
	defer client.Close()

	since := time.Now().AddDate(0, 0, -exportDays)
	result, err := client.GetAuditLog(context.Background(), policyclient.AuditLogOptions{
		StartTime: since,
	})
	if err != nil {
		return fmt.Errorf("failed to get audit log: %w", err)
	}

	var out *os.File
	if exportOutputFile != "" {
		f, createErr := os.Create(exportOutputFile)
		if createErr != nil {
			return fmt.Errorf("failed to create output file: %w", createErr)
		}
		defer f.Close()
		out = f
	} else {
		out = os.Stdout
	}

	switch strings.ToLower(exportFormat) {
	case "json":
		if err := output.WriteJSON(out, result.Entries); err != nil {
			return err
		}
	case "yaml":
		if err := output.WriteYAML(out, result.Entries); err != nil {
			return err
		}
	case "csv":
		fmt.Fprintln(out, "timestamp,policy_id,resource_type,allowed,violations_count,duration_ms")
		for i := range result.Entries {
			entry := &result.Entries[i]
			fmt.Fprintf(out, "%s,%s,%s,%t,%d,%d\n",
				entry.Timestamp.Format(time.RFC3339),
				entry.PolicyID,
				entry.ResourceType,
				entry.Allowed,
				len(entry.Violations),
				entry.DurationMS,
			)
		}
	default:
		return fmt.Errorf("unsupported export format: %s", exportFormat)
	}

	if exportOutputFile != "" {
		fmt.Fprintf(os.Stderr, "Exported %d audit entries to %s\n", len(result.Entries), exportOutputFile)
	}

	return nil
}

// ============================================================================
// Stats Command
// ============================================================================

var statsDays int

func newStatsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stats",
		Short: "View audit statistics and trends",
		Long: `Display statistics and trends from the audit log.

Shows:
  - Total evaluations
  - Allow/deny ratios
  - Most evaluated policies
  - Peak evaluation times
  - Trend analysis

Examples:
  # Show stats for last 7 days
  kscore-audit stats --days 7

  # Show stats for last 30 days
  kscore-audit stats --days 30`,
		RunE: runStats,
	}

	cmd.Flags().IntVar(&statsDays, "days", 7, "Number of days to analyze")

	return cmd
}

func runStats(cmd *cobra.Command, args []string) error {
	client, err := createPolicyClient()
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w", err)
	}
	defer client.Close()

	now := time.Now()
	since := now.AddDate(0, 0, -statsDays)

	report, err := client.GetComplianceReport(context.Background(), policyclient.ComplianceReportOptions{
		StartTime: since,
		EndTime:   now,
	})
	if err != nil {
		return fmt.Errorf("failed to get compliance stats: %w", err)
	}

	format, err := output.ParseFormat(outputFormat)
	if err != nil {
		return err
	}

	allowRate := float64(0)
	denyRate := float64(0)
	if report.TotalEvaluations > 0 {
		allowRate = float64(report.CompliantEvaluations) / float64(report.TotalEvaluations) * 100
		denyRate = float64(report.NonCompliantEvaluations) / float64(report.TotalEvaluations) * 100
	}

	stats := struct {
		Period            string                       `json:"period" yaml:"period"`
		TotalEvaluations  int64                        `json:"total_evaluations" yaml:"total_evaluations"`
		AllowedCount      int64                        `json:"allowed_count" yaml:"allowed_count"`
		DeniedCount       int64                        `json:"denied_count" yaml:"denied_count"`
		AllowRate         float64                      `json:"allow_rate" yaml:"allow_rate"`
		DenyRate          float64                      `json:"deny_rate" yaml:"deny_rate"`
		ComplianceRate    float64                      `json:"compliance_rate" yaml:"compliance_rate"`
		PolicyStats       []policyclient.ComplianceStats `json:"policy_stats,omitempty" yaml:"policy_stats,omitempty"`
	}{
		Period:           fmt.Sprintf("Last %d days", statsDays),
		TotalEvaluations: report.TotalEvaluations,
		AllowedCount:     report.CompliantEvaluations,
		DeniedCount:      report.NonCompliantEvaluations,
		AllowRate:        allowRate,
		DenyRate:         denyRate,
		ComplianceRate:   report.ComplianceRate,
		PolicyStats:      report.PolicyStats,
	}

	switch format {
	case output.FormatJSON:
		return output.WriteJSON(os.Stdout, stats)
	case output.FormatYAML:
		return output.WriteYAML(os.Stdout, stats)
	case output.FormatTable, output.FormatText:
		fmt.Printf("=== Audit Statistics (Last %d days) ===\n\n", statsDays)

		fmt.Println("Evaluation Summary:")
		fmt.Printf("  Total Evaluations: %d\n", stats.TotalEvaluations)
		fmt.Printf("  Compliant:         %d (%.1f%%)\n", stats.AllowedCount, stats.AllowRate)
		fmt.Printf("  Non-Compliant:     %d (%.1f%%)\n", stats.DeniedCount, stats.DenyRate)
		fmt.Printf("  Compliance Rate:   %.1f%%\n", stats.ComplianceRate)
		fmt.Println()

		if len(report.PolicyStats) > 0 {
			fmt.Println("Evaluations by Policy:")

			type policyCount struct {
				id    string
				count int64
			}
			sorted := make([]policyCount, 0, len(report.PolicyStats))
			for i := range report.PolicyStats {
				s := &report.PolicyStats[i]
				sorted = append(sorted, policyCount{s.PolicyName, s.TotalEvaluations})
			}
			sort.Slice(sorted, func(i, j int) bool {
				return sorted[i].count > sorted[j].count
			})

			limit := 10
			if len(sorted) < limit {
				limit = len(sorted)
			}
			for i := 0; i < limit; i++ {
				fmt.Printf("  %-40s %d\n", sorted[i].id, sorted[i].count)
			}
		}

		if stats.TotalEvaluations == 0 {
			fmt.Println("\nNote: No audit entries found in the specified period.")
		}

		return nil
	default:
		return fmt.Errorf("unsupported output format: %s", outputFormat)
	}
}

// ============================================================================
// Search Command
// ============================================================================

func newSearchCommand() *cobra.Command {
	var (
		searchPolicy string
		searchUser   string
		searchAction string
		searchSince  string
		searchOutput string
		searchDenied bool
		limit        int
	)

	cmd := &cobra.Command{
		Use:     "search",
		Aliases: []string{"query"},
		Short:   "Search audit log entries",
		Long: `Search audit log entries with flexible filters.

Supports filtering by policy, user, action, and time range.
Results can be output as JSON for piping to other tools.

The "query" alias is also available: kscore-audit query ...

Examples:
  # Search for events related to a policy
  kscore-audit search --policy "security-no-root" --since "7d"

  # Search for denied evaluations
  kscore-audit search --denied --since "24h"

  # Search by user
  kscore-audit search --user "admin"

  # Output to file
  kscore-audit search --policy "sec-*" --output /tmp/results.json

  # Limit results
  kscore-audit search --limit 10`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSearch(cmd, searchPolicy, searchUser, searchAction, searchSince, searchOutput, searchDenied, limit)
		},
	}

	cmd.Flags().StringVar(&searchPolicy, "policy", "", "Filter by policy ID")
	cmd.Flags().StringVar(&searchUser, "user", "", "Filter by username")
	cmd.Flags().StringVar(&searchAction, "action", "", "Filter by action")
	cmd.Flags().StringVar(&searchSince, "since", "", "Show entries since duration (e.g., '7d', '24h')")
	cmd.Flags().StringVar(&searchOutput, "output", "", "Output file path (default: stdout)")
	cmd.Flags().BoolVar(&searchDenied, "denied", false, "Show only denied evaluations")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum entries to return (0 for unlimited)")

	return cmd
}

func runSearch(cmd *cobra.Command, policyFilter, user, action, since, outputFile string, denied bool, limit int) error {
	client, err := createPolicyClient()
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w", err)
	}
	defer client.Close()

	opts := policyclient.AuditLogOptions{
		PolicyID: policyFilter,
		User:     user,
		Action:   action,
	}

	if denied {
		d := false
		opts.Allowed = &d
	}

	if limit > 0 {
		opts.PageSize = int32(limit) //nolint:gosec // G115: limit is small
	}

	if since != "" {
		d, parseErr := parseDuration(since)
		if parseErr != nil {
			return fmt.Errorf("invalid --since value: %w", parseErr)
		}
		opts.StartTime = time.Now().Add(-d)
	}

	result, err := client.GetAuditLog(context.Background(), opts)
	if err != nil {
		return fmt.Errorf("failed to search audit log: %w", err)
	}

	var out = os.Stdout
	if outputFile != "" {
		f, createErr := os.Create(outputFile)
		if createErr != nil {
			return fmt.Errorf("failed to create output file: %w", createErr)
		}
		defer f.Close()
		out = f
	}

	format, err := output.ParseFormat(outputFormat)
	if err != nil {
		return err
	}

	switch format {
	case output.FormatJSON:
		if err := output.WriteJSON(out, result.Entries); err != nil {
			return err
		}
	case output.FormatYAML:
		if err := output.WriteYAML(out, result.Entries); err != nil {
			return err
		}
	default:
		tbl := &output.Table{
			Headers: []string{"TIMESTAMP", "POLICY", "RESOURCE", "RESULT", "USER", "VIOLATIONS"},
		}
		for i := range result.Entries {
			e := &result.Entries[i]
			entryResult := "ALLOWED"
			if !e.Allowed {
				entryResult = "DENIED"
			}
			tbl.Rows = append(tbl.Rows, []string{
				e.Timestamp.Format("2006-01-02 15:04:05"),
				truncate(e.PolicyID, 25),
				truncate(e.ResourceType, 15),
				entryResult,
				e.User,
				fmt.Sprintf("%d", len(e.Violations)),
			})
		}
		if err := output.WriteTable(out, tbl); err != nil {
			return err
		}
	}

	if outputFile != "" {
		fmt.Fprintf(os.Stderr, "Exported %d entries to %s\n", len(result.Entries), outputFile)
	}
	return nil
}

// ============================================================================
// Analyze Command
// ============================================================================

func newAnalyzeCommand() *cobra.Command {
	var (
		inputGlob  string
		baseline   string
		analyzeOut string
	)

	cmd := &cobra.Command{
		Use:   "analyze",
		Short: "Analyze audit data for anomalies",
		Long: `Analyze audit data to identify anomalous patterns.

Compares recent activity against a historical baseline to detect:
  - Unusual login patterns
  - Privilege escalation attempts
  - Abnormal command execution rates
  - Geographic anomalies

Examples:
  # Analyze audit data against 30-day baseline
  kscore-audit analyze --input "/tmp/*.json" --baseline "30d"

  # Output anomalies to file
  kscore-audit analyze --input "/tmp/*.json" --baseline "30d" --output anomalies.json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("audit analysis requires server-side analytics not yet available")
		},
	}

	cmd.Flags().StringVar(&inputGlob, "input", "", "Input file glob pattern (e.g., '/tmp/*.json')")
	cmd.Flags().StringVar(&baseline, "baseline", "30d", "Baseline period for comparison (e.g., '7d', '30d')")
	cmd.Flags().StringVar(&analyzeOut, "output", "", "Output file for analysis results")

	return cmd
}

// ============================================================================
// Timeline Command
// ============================================================================

func newTimelineCommand() *cobra.Command {
	var (
		from        string
		to          string
		timelineOut string
	)

	cmd := &cobra.Command{
		Use:   "timeline",
		Short: "Generate incident timeline",
		Long: `Generate a chronological timeline of audit events for incident documentation.

Produces a timeline in HTML or text format showing:
  - All audit events in chronological order
  - Event categorization and severity
  - Key milestones and markers

Examples:
  # Generate timeline for a time range
  kscore-audit timeline --from "2026-01-01T00:00:00Z" --to "2026-01-02T00:00:00Z"

  # Output as HTML
  kscore-audit timeline --from "2026-01-01T00:00:00Z" --to "2026-01-02T00:00:00Z" --output incident-timeline.html`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTimeline(cmd, from, to, timelineOut)
		},
	}

	cmd.Flags().StringVar(&from, "from", "", "Start time (RFC3339 or human-readable)")
	cmd.Flags().StringVar(&to, "to", "", "End time (RFC3339 or human-readable)")
	cmd.Flags().StringVar(&timelineOut, "output", "", "Output file path (default: stdout)")

	return cmd
}

func runTimeline(cmd *cobra.Command, from, to, timelineOut string) error {
	client, err := createPolicyClient()
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w", err)
	}
	defer client.Close()

	opts := policyclient.AuditLogOptions{}
	if from != "" {
		t, parseErr := time.Parse(time.RFC3339, from)
		if parseErr != nil {
			return fmt.Errorf("invalid --from time (use RFC3339): %w", parseErr)
		}
		opts.StartTime = t
	}
	if to != "" {
		t, parseErr := time.Parse(time.RFC3339, to)
		if parseErr != nil {
			return fmt.Errorf("invalid --to time (use RFC3339): %w", parseErr)
		}
		opts.EndTime = t
	}

	result, err := client.GetAuditLog(context.Background(), opts)
	if err != nil {
		return fmt.Errorf("failed to get audit log: %w", err)
	}

	var out = os.Stdout
	if timelineOut != "" {
		f, createErr := os.Create(timelineOut)
		if createErr != nil {
			return fmt.Errorf("failed to create output file: %w", createErr)
		}
		defer f.Close()
		out = f
	}

	isHTML := strings.HasSuffix(timelineOut, ".html")

	format, err := output.ParseFormat(outputFormat)
	if err != nil {
		return err
	}

	if isHTML {
		fmt.Fprintln(out, "<html><head><title>Incident Timeline</title></head><body>")
		fmt.Fprintln(out, "<h1>Incident Timeline</h1>")
		if from != "" {
			fmt.Fprintf(out, "<p>From: %s To: %s</p>\n", from, to)
		}
		fmt.Fprintln(out, "<table border='1' cellpadding='4'>")
		fmt.Fprintln(out, "<tr><th>Time</th><th>Policy</th><th>Resource</th><th>Result</th><th>User</th></tr>")
		for i := range result.Entries {
			e := &result.Entries[i]
			entryResult := "ALLOWED"
			if !e.Allowed {
				entryResult = "DENIED"
			}
			fmt.Fprintf(out, "<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>\n",
				e.Timestamp.Format(time.RFC3339), e.PolicyID, e.ResourceType, entryResult, e.User)
		}
		fmt.Fprintln(out, "</table></body></html>")
		fmt.Fprintf(os.Stderr, "Timeline written to %s\n", timelineOut)
		return nil
	}

	switch format {
	case output.FormatJSON:
		return output.WriteJSON(out, result.Entries)
	case output.FormatYAML:
		return output.WriteYAML(out, result.Entries)
	default:
		fmt.Fprintln(out, "Incident Timeline")
		fmt.Fprintln(out, "=================")
		if from != "" {
			fmt.Fprintf(out, "From: %s  To: %s\n", from, to)
		}
		fmt.Fprintln(out)

		for i := range result.Entries {
			e := &result.Entries[i]
			entryResult := "ALLOWED"
			if !e.Allowed {
				entryResult = "DENIED"
			}
			fmt.Fprintf(out, "  %s  [%s] %s\n", e.Timestamp.Format(time.RFC3339), entryResult, e.PolicyID)
			fmt.Fprintf(out, "    User: %s\n", e.User)
			fmt.Fprintf(out, "    Resource: %s\n\n", e.ResourceType)
		}
	}

	if timelineOut != "" && !isHTML {
		fmt.Fprintf(os.Stderr, "Timeline written to %s\n", timelineOut)
	}
	return nil
}

// ============================================================================
// Watch Command
// ============================================================================

func newWatchCommand() *cobra.Command {
	var (
		watchType     string
		watchStatus   string
		watchAgent    string
		watchUser     string
		watchAPIKey   string
		watchInterval time.Duration
	)

	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Real-time audit log monitoring",
		Long: `Monitor audit log events in real-time with optional filters.

Streams audit events as they occur, similar to 'tail -f' for audit logs.
Press Ctrl+C to stop.

Supports filtering by event type, status, agent, user, and API key.
In JSON format, outputs one JSON object per line (NDJSON).

Examples:
  # Watch all events
  kscore-audit watch

  # Watch only auth events
  kscore-audit watch --type "auth.*"

  # Watch failed events only
  kscore-audit watch --type "auth.*" --status "failed"

  # Watch events for a specific agent
  kscore-audit watch --agent "web-001"

  # Watch with faster polling
  kscore-audit watch --interval 500ms

  # Output as NDJSON
  kscore-audit watch --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("real-time audit log streaming requires a streaming RPC not yet available")
		},
	}

	cmd.Flags().StringVar(&watchType, "type", "", "Event type pattern (e.g., 'auth.*', 'exec.*')")
	cmd.Flags().StringVar(&watchStatus, "status", "", "Filter by status (e.g., 'failed', 'success')")
	cmd.Flags().StringVar(&watchAgent, "agent", "", "Filter by agent ID")
	cmd.Flags().StringVar(&watchUser, "user", "", "Filter by username")
	cmd.Flags().StringVar(&watchAPIKey, "api-key", "", "Filter by API key name")
	cmd.Flags().DurationVar(&watchInterval, "interval", 2*time.Second, "Polling interval (e.g., '1s', '500ms')")

	return cmd
}

// ============================================================================
// Helper Functions
// ============================================================================

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func parseDuration(s string) (time.Duration, error) {
	if strings.HasSuffix(s, "d") {
		s = strings.TrimSuffix(s, "d")
		var days int
		if _, err := fmt.Sscanf(s, "%d", &days); err != nil {
			return 0, fmt.Errorf("invalid duration: %s", s)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}

func buildKeyValueTable(pairs [][2]string) *output.Table {
	rows := make([][]string, 0, len(pairs))
	for _, pair := range pairs {
		if pair[1] == "" {
			continue
		}
		rows = append(rows, []string{pair[0], pair[1]})
	}

	return &output.Table{
		Headers: []string{"KEY", "VALUE"},
		Rows:    rows,
	}
}

func buildSeverityTable(severityCounts map[string]int64) *output.Table {
	keys := make([]string, 0, len(severityCounts))
	for severity := range severityCounts {
		keys = append(keys, severity)
	}
	sort.Strings(keys)

	rows := make([][]string, 0, len(keys))
	for _, key := range keys {
		rows = append(rows, []string{key, fmt.Sprintf("%d", severityCounts[key])})
	}

	return &output.Table{
		Headers: []string{"SEVERITY", "COUNT"},
		Rows:    rows,
	}
}

func buildTopViolationsTable(violations []policyclient.ViolationSummary) *output.Table {
	rows := make([][]string, 0, len(violations))
	for i := range violations {
		v := &violations[i]
		rows = append(rows, []string{
			v.PolicyName,
			v.Severity,
			fmt.Sprintf("%d", v.Count),
		})
	}

	return &output.Table{
		Headers: []string{"POLICY", "SEVERITY", "COUNT"},
		Rows:    rows,
	}
}
