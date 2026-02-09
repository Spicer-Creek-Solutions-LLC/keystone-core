// Package main implements the kscore-audit CLI for policy evaluation auditing and compliance reporting.
package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/shawnbutts/keystone-core/internal/cli/auditutil"
	"github.com/shawnbutts/keystone-core/internal/cli/output"
	"github.com/shawnbutts/keystone-core/internal/policy"
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

// ============================================================================
// Log Command (formerly "audit" in kscore-policy)
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
	// Create auditor (in production, would connect to control plane)
	auditor := policy.NewAuditor(1000)

	// Build filter
	filter := &policy.AuditFilter{
		PolicyID:     logPolicyID,
		ResourceType: logResourceType,
		Limit:        logLimit,
	}

	if logDeniedOnly {
		denied := false
		filter.Allowed = &denied
	}

	// Parse time filters if provided
	if logSince != "" {
		t, err := time.Parse("2006-01-02", logSince)
		if err != nil {
			return fmt.Errorf("invalid --since date format (use YYYY-MM-DD): %w", err)
		}
		filter.StartTime = t
	}
	if logUntil != "" {
		t, err := time.Parse("2006-01-02", logUntil)
		if err != nil {
			return fmt.Errorf("invalid --until date format (use YYYY-MM-DD): %w", err)
		}
		// Set to end of day
		t = t.Add(23*time.Hour + 59*time.Minute + 59*time.Second)
		filter.EndTime = t
	}

	// Get entries
	entries := auditor.GetEntries(filter)

	format, err := output.ParseFormat(outputFormat)
	if err != nil {
		return err
	}

	if len(entries) == 0 {
		switch format {
		case output.FormatJSON:
			return output.WriteJSON(os.Stdout, entries)
		case output.FormatYAML:
			return output.WriteYAML(os.Stdout, entries)
		case output.FormatTable, output.FormatText:
			fmt.Println("No audit entries found.")
			fmt.Println("\nNote: This CLI reads from an in-memory store.")
			fmt.Println("For production audit logs, use the control plane API.")
			return nil
		default:
			return fmt.Errorf("unsupported output format: %s", outputFormat)
		}
	}

	switch format {
	case output.FormatJSON:
		return output.WriteJSON(os.Stdout, entries)
	case output.FormatYAML:
		return output.WriteYAML(os.Stdout, entries)
	case output.FormatTable, output.FormatText:
		fmt.Printf("%-20s %-25s %-15s %-10s %-10s\n", "TIMESTAMP", "POLICY", "RESOURCE", "RESULT", "VIOLATIONS")
		fmt.Println(strings.Repeat("-", 85))
		for i := range entries {
			entry := &entries[i]
			result := "ALLOWED"
			if !entry.Allowed {
				result = "DENIED"
			}
			fmt.Printf("%-20s %-25s %-15s %-10s %-10d\n",
				entry.Timestamp.Format("2006-01-02 15:04:05"),
				truncate(entry.PolicyID, 25),
				truncate(entry.ResourceType, 15),
				result,
				len(entry.Violations),
			)
		}

		fmt.Printf("\nTotal: %d entries\n", len(entries))
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
	// Create components (in production, connect to control plane)
	registry := policy.NewRegistry()
	auditor := policy.NewAuditor(10000)
	reporter := policy.NewComplianceReporter(auditor, registry)

	// Generate report
	period := policy.ReportPeriod{
		Start: time.Now().AddDate(0, 0, -reportDays),
		End:   time.Now(),
	}

	report := reporter.GenerateReport(period)

	// Output
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
			{"GENERATED", report.GeneratedAt.Format(time.RFC3339)},
			{"PERIOD", fmt.Sprintf("%s to %s",
				report.Period.Start.Format("2006-01-02"),
				report.Period.End.Format("2006-01-02"))},
			{"TOTAL POLICIES", fmt.Sprintf("%d", report.TotalPolicies)},
			{"COMPLIANT", fmt.Sprintf("%d", report.CompliantPolicies)},
			{"VIOLATING", fmt.Sprintf("%d", report.ViolatingPolicies)},
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

		if report.TotalPolicies == 0 {
			fmt.Println("\nNote: No policy evaluation data found in the audit store.")
			fmt.Println("For production compliance reports, use the control plane API.")
		}
		return nil
	case output.FormatText:
		fmt.Println("=== Compliance Report ===")
		fmt.Printf("Generated: %s\n", report.GeneratedAt.Format(time.RFC3339))
		fmt.Printf("Period:    %s to %s\n",
			report.Period.Start.Format("2006-01-02"),
			report.Period.End.Format("2006-01-02"))
		fmt.Println()

		fmt.Println("Summary:")
		fmt.Printf("  Total Policies:     %d\n", report.TotalPolicies)
		fmt.Printf("  Compliant:          %d\n", report.CompliantPolicies)
		fmt.Printf("  Violating:          %d\n", report.ViolatingPolicies)
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
			for i, v := range report.TopViolations {
				fmt.Printf("  %d. %s (%s) - %d violations\n", i+1, v.PolicyName, v.Severity, v.Count)
			}
		}

		if report.TotalPolicies == 0 {
			fmt.Println("\nNote: No policy evaluation data found in the audit store.")
			fmt.Println("For production compliance reports, use the control plane API.")
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
	// Create auditor (in production, connect to control plane)
	auditor := policy.NewAuditor(10000)

	// Build filter with time range
	since := time.Now().AddDate(0, 0, -exportDays)
	filter := &policy.AuditFilter{
		StartTime: since,
		Limit:     0, // No limit for export
	}

	// Get entries
	entries := auditor.GetEntries(filter)

	// Determine output destination
	var out *os.File
	if exportOutputFile != "" {
		f, err := os.Create(exportOutputFile)
		if err != nil {
			return fmt.Errorf("failed to create output file: %w", err)
		}
		defer f.Close()
		out = f
	} else {
		out = os.Stdout
	}

	// Export based on format
	switch strings.ToLower(exportFormat) {
	case "json":
		if err := output.WriteJSON(out, entries); err != nil {
			return err
		}
	case "yaml":
		if err := output.WriteYAML(out, entries); err != nil {
			return err
		}
	case "csv":
		// Write CSV header
		fmt.Fprintln(out, "timestamp,policy_id,resource_type,allowed,violations_count,duration_ms")
		for i := range entries {
			entry := &entries[i]
			fmt.Fprintf(out, "%s,%s,%s,%t,%d,%d\n",
				entry.Timestamp.Format(time.RFC3339),
				entry.PolicyID,
				entry.ResourceType,
				entry.Allowed,
				len(entry.Violations),
				entry.Duration.Milliseconds(),
			)
		}
	default:
		return fmt.Errorf("unsupported export format: %s", exportFormat)
	}

	if exportOutputFile != "" {
		fmt.Fprintf(os.Stderr, "Exported %d audit entries to %s\n", len(entries), exportOutputFile)
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
	// Create auditor (in production, connect to control plane)
	auditor := policy.NewAuditor(10000)

	// Build filter
	since := time.Now().AddDate(0, 0, -statsDays)
	filter := &policy.AuditFilter{
		StartTime: since,
	}

	// Get entries
	entries := auditor.GetEntries(filter)

	// Calculate statistics
	totalEvaluations := len(entries)
	allowedCount := 0
	deniedCount := 0
	policyEvaluations := make(map[string]int)
	totalViolations := 0

	for i := range entries {
		entry := &entries[i]
		if entry.Allowed {
			allowedCount++
		} else {
			deniedCount++
		}
		policyEvaluations[entry.PolicyID]++
		totalViolations += len(entry.Violations)
	}

	// Output statistics
	format, err := output.ParseFormat(outputFormat)
	if err != nil {
		return err
	}

	stats := struct {
		Period            string         `json:"period" yaml:"period"`
		TotalEvaluations  int            `json:"total_evaluations" yaml:"total_evaluations"`
		AllowedCount      int            `json:"allowed_count" yaml:"allowed_count"`
		DeniedCount       int            `json:"denied_count" yaml:"denied_count"`
		AllowRate         float64        `json:"allow_rate" yaml:"allow_rate"`
		DenyRate          float64        `json:"deny_rate" yaml:"deny_rate"`
		TotalViolations   int            `json:"total_violations" yaml:"total_violations"`
		PolicyEvaluations map[string]int `json:"policy_evaluations" yaml:"policy_evaluations"`
	}{
		Period:            fmt.Sprintf("Last %d days", statsDays),
		TotalEvaluations:  totalEvaluations,
		AllowedCount:      allowedCount,
		DeniedCount:       deniedCount,
		TotalViolations:   totalViolations,
		PolicyEvaluations: policyEvaluations,
	}

	if totalEvaluations > 0 {
		stats.AllowRate = float64(allowedCount) / float64(totalEvaluations) * 100
		stats.DenyRate = float64(deniedCount) / float64(totalEvaluations) * 100
	}

	switch format {
	case output.FormatJSON:
		return output.WriteJSON(os.Stdout, stats)
	case output.FormatYAML:
		return output.WriteYAML(os.Stdout, stats)
	case output.FormatTable, output.FormatText:
		fmt.Printf("=== Audit Statistics (Last %d days) ===\n\n", statsDays)

		fmt.Println("Evaluation Summary:")
		fmt.Printf("  Total Evaluations: %d\n", totalEvaluations)
		fmt.Printf("  Allowed:           %d (%.1f%%)\n", allowedCount, stats.AllowRate)
		fmt.Printf("  Denied:            %d (%.1f%%)\n", deniedCount, stats.DenyRate)
		fmt.Printf("  Total Violations:  %d\n", totalViolations)
		fmt.Println()

		if len(policyEvaluations) > 0 {
			fmt.Println("Evaluations by Policy:")

			// Sort policies by count
			type policyCount struct {
				id    string
				count int
			}
			sorted := make([]policyCount, 0, len(policyEvaluations))
			for id, count := range policyEvaluations {
				sorted = append(sorted, policyCount{id, count})
			}
			sort.Slice(sorted, func(i, j int) bool {
				return sorted[i].count > sorted[j].count
			})

			// Show top 10
			limit := 10
			if len(sorted) < limit {
				limit = len(sorted)
			}
			for i := 0; i < limit; i++ {
				fmt.Printf("  %-40s %d\n", sorted[i].id, sorted[i].count)
			}
		}

		if totalEvaluations == 0 {
			fmt.Println("\nNote: No audit entries found in the specified period.")
			fmt.Println("For production statistics, use the control plane API.")
		}

		return nil
	default:
		return fmt.Errorf("unsupported output format: %s", outputFormat)
	}
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

func buildSeverityTable(severityCounts map[policy.Severity]int) *output.Table {
	keys := make([]string, 0, len(severityCounts))
	for severity := range severityCounts {
		keys = append(keys, string(severity))
	}
	sort.Strings(keys)

	rows := make([][]string, 0, len(keys))
	for _, key := range keys {
		rows = append(rows, []string{key, fmt.Sprintf("%d", severityCounts[policy.Severity(key)])})
	}

	return &output.Table{
		Headers: []string{"SEVERITY", "COUNT"},
		Rows:    rows,
	}
}

func buildTopViolationsTable(violations []policy.ViolationSummary) *output.Table {
	rows := make([][]string, 0, len(violations))
	for _, v := range violations {
		rows = append(rows, []string{
			v.PolicyName,
			string(v.Severity),
			fmt.Sprintf("%d", v.Count),
		})
	}

	return &output.Table{
		Headers: []string{"POLICY", "SEVERITY", "COUNT"},
		Rows:    rows,
	}
}
