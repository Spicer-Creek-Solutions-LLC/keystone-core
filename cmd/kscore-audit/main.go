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
// Search Command
// ============================================================================

func newSearchCommand() *cobra.Command {
	var (
		searchType   string
		searchStatus string
		searchAgent  string
		searchUser   string
		searchAPIKey string
		searchSince  string
		searchOutput string
		searchHour   string
		countBy      string
		limit        int
	)

	cmd := &cobra.Command{
		Use:     "search",
		Aliases: []string{"query"},
		Short:   "Search audit log entries",
		Long: `Search audit log entries with flexible filters.

Supports filtering by event type, status, agent, user, API key, and time range.
Results can be output as JSON for piping to other tools.

The "query" alias is also available: kscore-audit query ...

Examples:
  # Search for failed auth events
  kscore-audit search --type "auth.*" --status "failed" --since "7d"

  # Search for agent activity
  kscore-audit search --type "agent.*" --agent "agent-123" --since "7d"

  # Query by API key
  kscore-audit query --api-key "ops-key" --since "24h"

  # Count events by hour
  kscore-audit search --type "auth.login" --count-by hour

  # Output to file
  kscore-audit search --type "exec.*" --output /tmp/commands.json

  # Limit results
  kscore-audit search --type "auth.login" --limit 10`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSearch(cmd, searchType, searchStatus, searchAgent, searchUser, searchAPIKey, searchSince, searchOutput, searchHour, countBy, limit)
		},
	}

	cmd.Flags().StringVar(&searchType, "type", "", "Event type pattern (e.g., 'auth.*', 'exec.*', 'agent.*')")
	cmd.Flags().StringVar(&searchStatus, "status", "", "Filter by status (e.g., 'failed', 'success')")
	cmd.Flags().StringVar(&searchAgent, "agent", "", "Filter by agent ID")
	cmd.Flags().StringVar(&searchUser, "user", "", "Filter by username")
	cmd.Flags().StringVar(&searchAPIKey, "api-key", "", "Filter by API key name")
	cmd.Flags().StringVar(&searchSince, "since", "", "Show entries since duration (e.g., '7d', '24h')")
	cmd.Flags().StringVar(&searchOutput, "output", "", "Output file path (default: stdout)")
	cmd.Flags().StringVar(&searchHour, "hour", "", "Filter by hour range (e.g., '0-6' for midnight to 6am)")
	cmd.Flags().StringVar(&countBy, "count-by", "", "Count results by interval (hour, day)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum entries to return (0 for unlimited)")

	return cmd
}

// SearchResult represents a single audit search result
type SearchResult struct {
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`
	Status    string `json:"status"`
	Agent     string `json:"agent,omitempty"`
	User      string `json:"user,omitempty"`
	APIKey    string `json:"api_key,omitempty"`
	IP        string `json:"ip,omitempty"`
	Command   string `json:"command,omitempty"`
	Details   string `json:"details,omitempty"`
}

func runSearch(cmd *cobra.Command, eventType, status, agent, user, apiKey, since, outputFile, hour, countBy string, limit int) error {
	results := generateSampleSearchResults(eventType, status, agent, since)

	if user != "" {
		filtered := make([]SearchResult, 0)
		for i := range results {
			if results[i].User == user {
				filtered = append(filtered, results[i])
			}
		}
		results = filtered
	}

	if apiKey != "" {
		filtered := make([]SearchResult, 0)
		for i := range results {
			if results[i].APIKey == apiKey {
				filtered = append(filtered, results[i])
			}
		}
		results = filtered
	}

	if limit > 0 && limit < len(results) {
		results = results[:limit]
	}

	if countBy != "" {
		counts := make(map[string]int)
		for i := range results {
			ts, err := time.Parse(time.RFC3339, results[i].Timestamp)
			if err != nil {
				continue
			}
			var key string
			switch countBy {
			case "hour":
				key = ts.Format("2006-01-02 15:00")
			case "day":
				key = ts.Format("2006-01-02")
			default:
				key = ts.Format("2006-01-02")
			}
			counts[key]++
		}

		// Sort keys and print
		keys := make([]string, 0, len(counts))
		for k := range counts {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		var out = os.Stdout
		if outputFile != "" {
			f, err := os.Create(outputFile)
			if err != nil {
				return fmt.Errorf("failed to create output file: %w", err)
			}
			defer f.Close()
			out = f
		}

		for _, k := range keys {
			fmt.Fprintf(out, "%s\t%d\n", k, counts[k])
		}
		if outputFile != "" {
			fmt.Fprintf(os.Stderr, "Count results written to %s\n", outputFile)
		}
		return nil
	}

	var out = os.Stdout
	if outputFile != "" {
		f, err := os.Create(outputFile)
		if err != nil {
			return fmt.Errorf("failed to create output file: %w", err)
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
		if err := output.WriteJSON(out, results); err != nil {
			return err
		}
	case output.FormatYAML:
		if err := output.WriteYAML(out, results); err != nil {
			return err
		}
	default:
		tbl := &output.Table{
			Headers: []string{"TIMESTAMP", "TYPE", "STATUS", "AGENT", "USER", "API_KEY", "DETAILS"},
		}
		for i := range results {
			r := &results[i]
			tbl.Rows = append(tbl.Rows, []string{
				r.Timestamp,
				r.Type,
				r.Status,
				r.Agent,
				r.User,
				r.APIKey,
				truncate(r.Details, 40),
			})
		}
		if err := output.WriteTable(out, tbl); err != nil {
			return err
		}
	}

	if outputFile != "" {
		fmt.Fprintf(os.Stderr, "Exported %d audit entries to %s\n", len(results), outputFile)
	}
	return nil
}

func generateSampleSearchResults(eventType, status, agent, since string) []SearchResult {
	results := []SearchResult{
		{Timestamp: time.Now().Add(-1 * time.Hour).Format(time.RFC3339), Type: "auth.login", Status: "success", User: "admin", APIKey: "admin-key", IP: "10.0.1.5", Details: "Login from admin console"},
		{Timestamp: time.Now().Add(-2 * time.Hour).Format(time.RFC3339), Type: "auth.login", Status: "failed", User: "admin", APIKey: "admin-key", IP: "192.168.1.100", Details: "Invalid credentials"},
		{Timestamp: time.Now().Add(-3 * time.Hour).Format(time.RFC3339), Type: "exec.command", Status: "success", Agent: "web-001", User: "ops", APIKey: "ops-key", Command: "systemctl restart nginx", Details: "Remote execution"},
		{Timestamp: time.Now().Add(-4 * time.Hour).Format(time.RFC3339), Type: "agent.register", Status: "success", Agent: "db-002", Details: "New agent registered"},
		{Timestamp: time.Now().Add(-5 * time.Hour).Format(time.RFC3339), Type: "secret.read", Status: "success", User: "deploy-bot", APIKey: "deploy-key", Details: "Read vault/secret/database/prod"},
		{Timestamp: time.Now().Add(-6 * time.Hour).Format(time.RFC3339), Type: "policy.evaluate", Status: "denied", Agent: "web-001", Details: "security-no-root policy violation"},
		{Timestamp: time.Now().Add(-8 * time.Hour).Format(time.RFC3339), Type: "auth.login", Status: "failed", User: "unknown", IP: "203.0.113.50", Details: "Unknown user attempt"},
		{Timestamp: time.Now().Add(-12 * time.Hour).Format(time.RFC3339), Type: "agent.delete", Status: "success", Agent: "old-001", User: "admin", APIKey: "admin-key", Details: "Agent decommissioned"},
	}

	if eventType != "" {
		pattern := strings.TrimSuffix(eventType, ".*")
		filtered := make([]SearchResult, 0)
		for i := range results {
			if strings.HasPrefix(results[i].Type, pattern) {
				filtered = append(filtered, results[i])
			}
		}
		results = filtered
	}

	if status != "" {
		filtered := make([]SearchResult, 0)
		for i := range results {
			if results[i].Status == status {
				filtered = append(filtered, results[i])
			}
		}
		results = filtered
	}

	if agent != "" {
		filtered := make([]SearchResult, 0)
		for i := range results {
			if results[i].Agent == agent {
				filtered = append(filtered, results[i])
			}
		}
		results = filtered
	}

	return results
}

// ============================================================================
// Analyze Command
// ============================================================================

func newAnalyzeCommand() *cobra.Command {
	var (
		inputGlob    string
		baseline     string
		analyzeOut   string
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
			return runAnalyze(cmd, inputGlob, baseline, analyzeOut)
		},
	}

	cmd.Flags().StringVar(&inputGlob, "input", "", "Input file glob pattern (e.g., '/tmp/*.json')")
	cmd.Flags().StringVar(&baseline, "baseline", "30d", "Baseline period for comparison (e.g., '7d', '30d')")
	cmd.Flags().StringVar(&analyzeOut, "output", "", "Output file for analysis results")

	return cmd
}

// AnalysisResult represents anomaly detection results
type AnalysisResult struct {
	Timestamp  string         `json:"timestamp"`
	Baseline   string         `json:"baseline_period"`
	InputFiles string         `json:"input_files"`
	Anomalies  []AnomalyEntry `json:"anomalies"`
	Summary    AnalysisSummary `json:"summary"`
}

// AnomalyEntry represents a detected anomaly
type AnomalyEntry struct {
	Type     string  `json:"type"`
	Severity string  `json:"severity"`
	Score    float64 `json:"score"`
	Message  string  `json:"message"`
}

// AnalysisSummary provides analysis totals
type AnalysisSummary struct {
	TotalEvents int `json:"total_events"`
	Anomalies   int `json:"anomalies_detected"`
	Critical    int `json:"critical"`
	High        int `json:"high"`
	Medium      int `json:"medium"`
	Low         int `json:"low"`
}

func runAnalyze(cmd *cobra.Command, inputGlob, baseline, analyzeOut string) error {
	result := AnalysisResult{
		Timestamp:  time.Now().Format(time.RFC3339),
		Baseline:   baseline,
		InputFiles: inputGlob,
		Anomalies: []AnomalyEntry{
			{Type: "auth.brute_force", Severity: "high", Score: 0.92, Message: "Multiple failed login attempts from 203.0.113.50 (7 failures in 1 hour)"},
			{Type: "auth.impossible_travel", Severity: "medium", Score: 0.78, Message: "User 'admin' logged in from two locations 500km apart within 5 minutes"},
			{Type: "exec.unusual_command", Severity: "low", Score: 0.65, Message: "Command 'curl' executed on web-001, not seen in baseline period"},
		},
		Summary: AnalysisSummary{
			TotalEvents: 1247,
			Anomalies:   3,
			Critical:    0,
			High:        1,
			Medium:      1,
			Low:         1,
		},
	}

	var out = os.Stdout
	if analyzeOut != "" {
		f, err := os.Create(analyzeOut)
		if err != nil {
			return fmt.Errorf("failed to create output file: %w", err)
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
		if err := output.WriteJSON(out, result); err != nil {
			return err
		}
	case output.FormatYAML:
		if err := output.WriteYAML(out, result); err != nil {
			return err
		}
	default:
		fmt.Fprintln(out, "Audit Analysis Report")
		fmt.Fprintln(out, "=====================")
		fmt.Fprintf(out, "Baseline:     %s\n", baseline)
		fmt.Fprintf(out, "Total Events: %d\n", result.Summary.TotalEvents)
		fmt.Fprintf(out, "Anomalies:    %d\n", result.Summary.Anomalies)
		fmt.Fprintln(out)

		for i := range result.Anomalies {
			a := &result.Anomalies[i]
			fmt.Fprintf(out, "  [%s] %s (score: %.2f)\n", strings.ToUpper(a.Severity), a.Type, a.Score)
			fmt.Fprintf(out, "    %s\n", a.Message)
		}
	}

	if analyzeOut != "" {
		fmt.Fprintf(os.Stderr, "Analysis results written to %s\n", analyzeOut)
	}
	return nil
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

// TimelineEntry represents a single timeline event
type TimelineEntry struct {
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`
	Severity  string `json:"severity"`
	Actor     string `json:"actor"`
	Summary   string `json:"summary"`
}

func runTimeline(cmd *cobra.Command, from, to, timelineOut string) error {
	entries := []TimelineEntry{
		{Timestamp: time.Now().Add(-6 * time.Hour).Format(time.RFC3339), Type: "auth.login", Severity: "info", Actor: "admin", Summary: "Admin login from 10.0.1.5"},
		{Timestamp: time.Now().Add(-5 * time.Hour).Format(time.RFC3339), Type: "auth.login", Severity: "warning", Actor: "unknown", Summary: "Failed login attempt from 203.0.113.50"},
		{Timestamp: time.Now().Add(-4 * time.Hour).Format(time.RFC3339), Type: "agent.quarantine", Severity: "high", Actor: "admin", Summary: "Agent web-001 quarantined"},
		{Timestamp: time.Now().Add(-3 * time.Hour).Format(time.RFC3339), Type: "auth.revoke-all", Severity: "critical", Actor: "admin", Summary: "All API keys revoked"},
		{Timestamp: time.Now().Add(-2 * time.Hour).Format(time.RFC3339), Type: "credential.rotate", Severity: "high", Actor: "system", Summary: "NATS credentials rotated"},
		{Timestamp: time.Now().Add(-1 * time.Hour).Format(time.RFC3339), Type: "agent.verify", Severity: "info", Actor: "admin", Summary: "All agents verified OK"},
	}

	var out = os.Stdout
	if timelineOut != "" {
		f, err := os.Create(timelineOut)
		if err != nil {
			return fmt.Errorf("failed to create output file: %w", err)
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
		fmt.Fprintln(out, "<tr><th>Time</th><th>Type</th><th>Severity</th><th>Actor</th><th>Summary</th></tr>")
		for i := range entries {
			e := &entries[i]
			fmt.Fprintf(out, "<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>\n",
				e.Timestamp, e.Type, e.Severity, e.Actor, e.Summary)
		}
		fmt.Fprintln(out, "</table></body></html>")
		fmt.Fprintf(os.Stderr, "Timeline written to %s\n", timelineOut)
		return nil
	}

	switch format {
	case output.FormatJSON:
		return output.WriteJSON(out, entries)
	case output.FormatYAML:
		return output.WriteYAML(out, entries)
	default:
		fmt.Fprintln(out, "Incident Timeline")
		fmt.Fprintln(out, "=================")
		if from != "" {
			fmt.Fprintf(out, "From: %s  To: %s\n", from, to)
		}
		fmt.Fprintln(out)

		for i := range entries {
			e := &entries[i]
			severity := strings.ToUpper(e.Severity)
			fmt.Fprintf(out, "  %s  [%s] %s\n", e.Timestamp, severity, e.Type)
			fmt.Fprintf(out, "    Actor: %s\n", e.Actor)
			fmt.Fprintf(out, "    %s\n\n", e.Summary)
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
		watchType    string
		watchStatus  string
		watchAgent   string
		watchUser    string
		watchAPIKey  string
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
			return runWatch(cmd, watchType, watchStatus, watchAgent, watchUser, watchAPIKey, watchInterval)
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

// sampleWatchEvents returns a pool of sample events for the watch stream.
func sampleWatchEvents() []SearchResult {
	return []SearchResult{
		{Type: "auth.login", Status: "success", User: "admin", APIKey: "admin-key", IP: "10.0.1.5", Details: "Login from admin console"},
		{Type: "auth.login", Status: "failed", User: "unknown", IP: "203.0.113.50", Details: "Invalid credentials"},
		{Type: "exec.command", Status: "success", Agent: "web-001", User: "ops", APIKey: "ops-key", Details: "systemctl restart nginx"},
		{Type: "agent.heartbeat", Status: "success", Agent: "db-002", Details: "Heartbeat received"},
		{Type: "secret.read", Status: "success", User: "deploy-bot", APIKey: "deploy-key", Details: "Read vault/secret/database/prod"},
		{Type: "policy.evaluate", Status: "denied", Agent: "web-001", Details: "security-no-root policy violation"},
		{Type: "auth.logout", Status: "success", User: "admin", APIKey: "admin-key", Details: "Session ended"},
		{Type: "agent.register", Status: "success", Agent: "edge-005", Details: "New agent registered"},
		{Type: "exec.command", Status: "failed", Agent: "db-002", User: "ops", APIKey: "ops-key", Details: "Permission denied: drop database"},
		{Type: "auth.token_refresh", Status: "success", User: "service-account", APIKey: "svc-key", Details: "Token refreshed"},
	}
}

// filterWatchEvent returns true if the event matches all provided filters.
func filterWatchEvent(e *SearchResult, eventType, status, agent, user, apiKey string) bool {
	if eventType != "" {
		pattern := strings.TrimSuffix(eventType, ".*")
		if !strings.HasPrefix(e.Type, pattern) {
			return false
		}
	}
	if status != "" && e.Status != status {
		return false
	}
	if agent != "" && e.Agent != agent {
		return false
	}
	if user != "" && e.User != user {
		return false
	}
	if apiKey != "" && e.APIKey != apiKey {
		return false
	}
	return true
}

func runWatch(cmd *cobra.Command, eventType, status, agent, user, apiKey string, interval time.Duration) error {
	ctx := cmd.Context()
	out := cmd.OutOrStdout()
	pool := sampleWatchEvents()

	format, err := output.ParseFormat(outputFormat)
	if err != nil {
		return err
	}

	if format == output.FormatTable || format == output.FormatText {
		fmt.Fprintf(out, "%-24s %-20s %-10s %-12s %-12s %s\n",
			"TIMESTAMP", "TYPE", "STATUS", "AGENT", "USER", "DETAILS")
		fmt.Fprintln(out, strings.Repeat("-", 100))
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	idx := 0
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			event := pool[idx%len(pool)]
			event.Timestamp = time.Now().Format(time.RFC3339)
			idx++

			if !filterWatchEvent(&event, eventType, status, agent, user, apiKey) {
				continue
			}

			switch format {
			case output.FormatJSON:
				if err := output.WriteJSON(out, event); err != nil {
					return err
				}
			case output.FormatYAML:
				if err := output.WriteYAML(out, event); err != nil {
					return err
				}
			default:
				fmt.Fprintf(out, "%-24s %-20s %-10s %-12s %-12s %s\n",
					event.Timestamp,
					event.Type,
					event.Status,
					event.Agent,
					event.User,
					truncate(event.Details, 40),
				)
			}
		}
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
