package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/shawnbutts/keystone-core/pkg/policy"
	"github.com/shawnbutts/keystone-core/pkg/version"
)

// newRootCmd creates the root command for kscore-policy
func newRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "kscore-policy",
		Short: "Policy enforcement and compliance management",
		Long: `Manage and evaluate policies using OPA (Rego) and CEL.

Keystone Core Policy provides:
  - Policy validation (syntax and semantic checks)
  - Policy evaluation against input data
  - Audit logging and compliance reporting
  - Support for OPA (Rego) and CEL policy languages

Examples:
  # List all policies
  kscorectl policy list

  # Validate a policy file
  kscorectl policy validate policies/security.yaml

  # Check a policy against input
  kscorectl policy check --policy security-no-root --input input.json

  # Generate compliance report
  kscorectl policy report --days 7`,
	}

	rootCmd.AddCommand(newVersionCmd())
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(validateCmd)
	rootCmd.AddCommand(checkCmd)
	rootCmd.AddCommand(showCmd)
	rootCmd.AddCommand(auditCmd)
	rootCmd.AddCommand(reportCmd)

	return rootCmd
}

// newVersionCmd creates the version command
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
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// PolicyFile represents a policy definition file
type PolicyFile struct {
	Policies []PolicyDefinition `yaml:"policies" json:"policies"`
}

// PolicyDefinition represents a single policy in a file
type PolicyDefinition struct {
	ID              string            `yaml:"id" json:"id"`
	Name            string            `yaml:"name" json:"name"`
	Description     string            `yaml:"description" json:"description"`
	Type            string            `yaml:"type" json:"type"`
	Category        string            `yaml:"category" json:"category"`
	Severity        string            `yaml:"severity" json:"severity"`
	EnforcementMode string            `yaml:"enforcement_mode" json:"enforcement_mode"`
	Enabled         bool              `yaml:"enabled" json:"enabled"`
	Code            string            `yaml:"code" json:"code"`
	Tags            []string          `yaml:"tags,omitempty" json:"tags,omitempty"`
	Metadata        map[string]string `yaml:"metadata,omitempty" json:"metadata,omitempty"`
}

// =============================================================================
// List Command
// =============================================================================

var (
	listCategory string
	listType     string
	listOutput   string
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List policies",
	Long: `List policies from a policy file or the registry.

Examples:
  # List all policies from a file
  kscorectl policy list policies/all.yaml

  # Filter by category
  kscorectl policy list policies/all.yaml --category security

  # Filter by type
  kscorectl policy list policies/all.yaml --type opa

  # Output as JSON
  kscorectl policy list policies/all.yaml --output json`,
	Args: cobra.MaximumNArgs(1),
	RunE: listExecute,
}

func init() {
	listCmd.Flags().StringVar(&listCategory, "category", "", "Filter by category (security, compliance, operational, cost, custom)")
	listCmd.Flags().StringVar(&listType, "type", "", "Filter by type (opa, cel)")
	listCmd.Flags().StringVarP(&listOutput, "output", "o", "table", "Output format (table, json, yaml)")
}

func listExecute(cmd *cobra.Command, args []string) error {
	var policies []PolicyDefinition

	if len(args) > 0 {
		// Load from file
		policyFile, err := loadPolicyFile(args[0])
		if err != nil {
			return err
		}
		policies = policyFile.Policies
	} else {
		return fmt.Errorf("policy file required")
	}

	// Apply filters
	filtered := make([]PolicyDefinition, 0)
	for _, p := range policies {
		if listCategory != "" && !strings.EqualFold(p.Category, listCategory) {
			continue
		}
		if listType != "" && !strings.EqualFold(p.Type, listType) {
			continue
		}
		filtered = append(filtered, p)
	}

	// Output
	switch listOutput {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(filtered)
	case "yaml":
		enc := yaml.NewEncoder(os.Stdout)
		return enc.Encode(filtered)
	default:
		printPolicyTable(filtered)
	}

	return nil
}

func printPolicyTable(policies []PolicyDefinition) {
	if len(policies) == 0 {
		fmt.Println("No policies found.")
		return
	}

	fmt.Printf("%-30s %-8s %-12s %-10s %-8s\n", "ID", "TYPE", "CATEGORY", "SEVERITY", "ENABLED")
	fmt.Println(strings.Repeat("-", 75))
	for _, p := range policies {
		enabled := "yes"
		if !p.Enabled {
			enabled = "no"
		}
		fmt.Printf("%-30s %-8s %-12s %-10s %-8s\n",
			truncate(p.ID, 30),
			p.Type,
			p.Category,
			p.Severity,
			enabled,
		)
	}
	fmt.Printf("\nTotal: %d policies\n", len(policies))
}

// =============================================================================
// Validate Command
// =============================================================================

var validateCmd = &cobra.Command{
	Use:   "validate <policyfile>",
	Short: "Validate policy syntax",
	Long: `Validate the syntax of policies in a YAML file.

Checks:
  - YAML syntax
  - Required fields (id, name, type, code)
  - Policy code syntax (OPA Rego or CEL)
  - Valid enum values (type, category, severity)

Examples:
  # Validate a policy file
  kscorectl policy validate policies/security.yaml`,
	Args: cobra.ExactArgs(1),
	RunE: validateExecute,
}

func validateExecute(cmd *cobra.Command, args []string) error {
	policyFilePath := args[0]

	fmt.Printf("Validating policy file: %s\n\n", policyFilePath)

	// Load policy file
	policyFile, err := loadPolicyFile(policyFilePath)
	if err != nil {
		return fmt.Errorf("failed to load policy file: %w", err)
	}

	if len(policyFile.Policies) == 0 {
		return fmt.Errorf("no policies found in file")
	}

	// Create policy engine for validation
	registry := policy.NewRegistry()
	engine := policy.NewPolicyEngine(registry)
	ctx := context.Background()

	errors := 0
	warnings := 0

	for _, pDef := range policyFile.Policies {
		fmt.Printf("Validating: %s\n", pDef.ID)

		// Check required fields
		if pDef.ID == "" {
			fmt.Printf("  ✗ Error: id is required\n")
			errors++
			continue
		}
		if pDef.Name == "" {
			fmt.Printf("  ✗ Error: name is required\n")
			errors++
		}
		if pDef.Code == "" {
			fmt.Printf("  ✗ Error: code is required\n")
			errors++
		}

		// Validate type
		policyType := policy.PolicyType(strings.ToLower(pDef.Type))
		if policyType != policy.PolicyTypeOPA && policyType != policy.PolicyTypeCEL && policyType != policy.PolicyTypeBuiltin {
			fmt.Printf("  ✗ Error: invalid type '%s' (must be opa, cel, or builtin)\n", pDef.Type)
			errors++
		}

		// Validate category
		category := policy.PolicyCategory(strings.ToLower(pDef.Category))
		switch category {
		case policy.CategorySecurity, policy.CategoryCompliance, policy.CategoryOperational, policy.CategoryCost, policy.CategoryCustom:
			// Valid
		default:
			fmt.Printf("  ! Warning: unknown category '%s'\n", pDef.Category)
			warnings++
		}

		// Validate severity
		severity := policy.Severity(strings.ToLower(pDef.Severity))
		switch severity {
		case policy.SeverityLow, policy.SeverityMedium, policy.SeverityHigh, policy.SeverityCritical:
			// Valid
		default:
			fmt.Printf("  ! Warning: unknown severity '%s'\n", pDef.Severity)
			warnings++
		}

		// Validate policy code syntax
		if pDef.Code != "" && policyType != "" {
			p := &policy.Policy{
				ID:     pDef.ID,
				Name:   pDef.Name,
				Type:   policyType,
				Policy: pDef.Code,
			}

			if err := engine.ValidatePolicy(ctx, p); err != nil {
				fmt.Printf("  ✗ Error: invalid policy code: %v\n", err)
				errors++
			} else {
				fmt.Printf("  ✓ Policy code is valid\n")
			}
		}
	}

	fmt.Printf("\n=== Validation Summary ===\n")
	fmt.Printf("Policies: %d\n", len(policyFile.Policies))
	fmt.Printf("Errors:   %d\n", errors)
	fmt.Printf("Warnings: %d\n", warnings)

	if errors > 0 {
		fmt.Println("\n✗ Validation failed!")
		os.Exit(1)
	}

	fmt.Println("\n✓ All policies valid!")
	return nil
}

// =============================================================================
// Check Command
// =============================================================================

var (
	checkPolicyID   string
	checkInputFile  string
	checkInputJSON  string
	checkAction     string
	checkUser       string
	checkContext    string
	checkOutputFmt  string
)

var checkCmd = &cobra.Command{
	Use:   "check <policyfile>",
	Short: "Evaluate a policy against input",
	Long: `Evaluate a policy against input data and report the result.

The input can be provided as:
  - JSON file (--input-file)
  - Inline JSON (--input)

Examples:
  # Check a specific policy with input from file
  kscorectl policy check policies/security.yaml --policy security-no-root --input-file input.json

  # Check with inline JSON input
  kscorectl policy check policies/security.yaml --policy security-no-root --input '{"command": "rm", "args": ["-rf", "/"]}'

  # Check with action and user context
  kscorectl policy check policies/security.yaml --policy security-no-root --input-file input.json --action execute --user admin`,
	Args: cobra.ExactArgs(1),
	RunE: checkExecute,
}

func init() {
	checkCmd.Flags().StringVar(&checkPolicyID, "policy", "", "Policy ID to evaluate (required)")
	checkCmd.Flags().StringVar(&checkInputFile, "input-file", "", "Input JSON file")
	checkCmd.Flags().StringVar(&checkInputJSON, "input", "", "Inline input JSON")
	checkCmd.Flags().StringVar(&checkAction, "action", "check", "Action being performed")
	checkCmd.Flags().StringVar(&checkUser, "user", "", "User performing the action")
	checkCmd.Flags().StringVar(&checkContext, "context", "", "Additional context as JSON")
	checkCmd.Flags().StringVarP(&checkOutputFmt, "output", "o", "text", "Output format (text, json)")
	checkCmd.MarkFlagRequired("policy")
}

func checkExecute(cmd *cobra.Command, args []string) error {
	policyFilePath := args[0]

	// Load input
	var resource interface{}
	if checkInputFile != "" {
		data, err := os.ReadFile(checkInputFile)
		if err != nil {
			return fmt.Errorf("failed to read input file: %w", err)
		}
		if err := json.Unmarshal(data, &resource); err != nil {
			return fmt.Errorf("failed to parse input JSON: %w", err)
		}
	} else if checkInputJSON != "" {
		if err := json.Unmarshal([]byte(checkInputJSON), &resource); err != nil {
			return fmt.Errorf("failed to parse inline JSON: %w", err)
		}
	} else {
		return fmt.Errorf("input required: use --input-file or --input")
	}

	// Parse context if provided
	var evalContext map[string]interface{}
	if checkContext != "" {
		if err := json.Unmarshal([]byte(checkContext), &evalContext); err != nil {
			return fmt.Errorf("failed to parse context JSON: %w", err)
		}
	}

	// Load policy file
	policyFile, err := loadPolicyFile(policyFilePath)
	if err != nil {
		return fmt.Errorf("failed to load policy file: %w", err)
	}

	// Find the policy
	var targetPolicy *PolicyDefinition
	for i, p := range policyFile.Policies {
		if p.ID == checkPolicyID {
			targetPolicy = &policyFile.Policies[i]
			break
		}
	}

	if targetPolicy == nil {
		return fmt.Errorf("policy not found: %s", checkPolicyID)
	}

	// Create registry and register the policy
	registry := policy.NewRegistry()
	p := &policy.Policy{
		ID:              targetPolicy.ID,
		Name:            targetPolicy.Name,
		Description:     targetPolicy.Description,
		Type:            policy.PolicyType(strings.ToLower(targetPolicy.Type)),
		Category:        policy.PolicyCategory(strings.ToLower(targetPolicy.Category)),
		Severity:        policy.Severity(strings.ToLower(targetPolicy.Severity)),
		EnforcementMode: policy.EnforcementMode(strings.ToLower(targetPolicy.EnforcementMode)),
		Policy:          targetPolicy.Code,
		Enabled:         targetPolicy.Enabled,
		Tags:            targetPolicy.Tags,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	if err := registry.RegisterPolicy(p); err != nil {
		return fmt.Errorf("failed to register policy: %w", err)
	}

	// Create engine and evaluate
	engine := policy.NewPolicyEngine(registry)
	ctx := context.Background()

	input := &policy.EvaluationInput{
		Resource:  resource,
		Action:    checkAction,
		User:      checkUser,
		Context:   evalContext,
		Timestamp: time.Now(),
	}

	result, err := engine.Evaluate(ctx, targetPolicy.ID, input)
	if err != nil {
		return fmt.Errorf("evaluation failed: %w", err)
	}

	// Output result
	if checkOutputFmt == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	// Text output
	fmt.Printf("Policy: %s (%s)\n", result.PolicyName, result.PolicyID)
	fmt.Printf("Type:   %s\n", targetPolicy.Type)
	fmt.Println()

	if result.Allowed {
		fmt.Println("Result: ✓ ALLOWED")
	} else {
		fmt.Println("Result: ✗ DENIED")
	}

	if result.Message != "" {
		fmt.Printf("\nMessage: %s\n", result.Message)
	}

	if len(result.Violations) > 0 {
		fmt.Printf("\nViolations (%d):\n", len(result.Violations))
		for i, v := range result.Violations {
			fmt.Printf("  %d. [%s] %s\n", i+1, v.Severity, v.Message)
			if v.Remediation != "" {
				fmt.Printf("     Remediation: %s\n", v.Remediation)
			}
		}
	}

	if len(result.Warnings) > 0 {
		fmt.Printf("\nWarnings (%d):\n", len(result.Warnings))
		for _, w := range result.Warnings {
			fmt.Printf("  - %s\n", w)
		}
	}

	fmt.Printf("\nDuration: %s\n", result.Duration)

	if !result.Allowed {
		os.Exit(1)
	}

	return nil
}

// =============================================================================
// Show Command
// =============================================================================

var showOutput string

var showCmd = &cobra.Command{
	Use:   "show <policyfile> <policyid>",
	Short: "Show policy details",
	Long: `Display detailed information about a specific policy.

Examples:
  # Show policy details
  kscorectl policy show policies/security.yaml security-no-root

  # Output as YAML
  kscorectl policy show policies/security.yaml security-no-root --output yaml`,
	Args: cobra.ExactArgs(2),
	RunE: showExecute,
}

func init() {
	showCmd.Flags().StringVarP(&showOutput, "output", "o", "text", "Output format (text, json, yaml)")
}

func showExecute(cmd *cobra.Command, args []string) error {
	policyFilePath := args[0]
	policyID := args[1]

	// Load policy file
	policyFile, err := loadPolicyFile(policyFilePath)
	if err != nil {
		return fmt.Errorf("failed to load policy file: %w", err)
	}

	// Find the policy
	var targetPolicy *PolicyDefinition
	for i, p := range policyFile.Policies {
		if p.ID == policyID {
			targetPolicy = &policyFile.Policies[i]
			break
		}
	}

	if targetPolicy == nil {
		return fmt.Errorf("policy not found: %s", policyID)
	}

	// Output
	switch showOutput {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(targetPolicy)
	case "yaml":
		enc := yaml.NewEncoder(os.Stdout)
		return enc.Encode(targetPolicy)
	default:
		fmt.Printf("ID:              %s\n", targetPolicy.ID)
		fmt.Printf("Name:            %s\n", targetPolicy.Name)
		fmt.Printf("Description:     %s\n", targetPolicy.Description)
		fmt.Printf("Type:            %s\n", targetPolicy.Type)
		fmt.Printf("Category:        %s\n", targetPolicy.Category)
		fmt.Printf("Severity:        %s\n", targetPolicy.Severity)
		fmt.Printf("Enforcement:     %s\n", targetPolicy.EnforcementMode)
		fmt.Printf("Enabled:         %t\n", targetPolicy.Enabled)
		if len(targetPolicy.Tags) > 0 {
			fmt.Printf("Tags:            %s\n", strings.Join(targetPolicy.Tags, ", "))
		}
		fmt.Printf("\nCode:\n")
		fmt.Println(strings.Repeat("-", 60))
		fmt.Println(targetPolicy.Code)
		fmt.Println(strings.Repeat("-", 60))
	}

	return nil
}

// =============================================================================
// Audit Command
// =============================================================================

var (
	auditPolicyID     string
	auditResourceType string
	auditDeniedOnly   bool
	auditLimit        int
	auditOutputFmt    string
)

var auditCmd = &cobra.Command{
	Use:   "audit",
	Short: "Show policy evaluation audit log",
	Long: `Display the policy evaluation audit log.

Note: This command shows audit entries from the in-memory store.
For production use, connect to the control plane API.

Examples:
  # Show recent audit entries
  kscorectl policy audit

  # Filter by policy
  kscorectl policy audit --policy security-no-root

  # Show only denied evaluations
  kscorectl policy audit --denied

  # Limit results
  kscorectl policy audit --limit 50`,
	RunE: auditExecute,
}

func init() {
	auditCmd.Flags().StringVar(&auditPolicyID, "policy", "", "Filter by policy ID")
	auditCmd.Flags().StringVar(&auditResourceType, "resource-type", "", "Filter by resource type")
	auditCmd.Flags().BoolVar(&auditDeniedOnly, "denied", false, "Show only denied evaluations")
	auditCmd.Flags().IntVar(&auditLimit, "limit", 100, "Maximum entries to show")
	auditCmd.Flags().StringVarP(&auditOutputFmt, "output", "o", "table", "Output format (table, json)")
}

func auditExecute(cmd *cobra.Command, args []string) error {
	// Create a sample auditor (in production, this would connect to the control plane)
	auditor := policy.NewPolicyAuditor(1000)

	// Build filter
	filter := &policy.AuditFilter{
		PolicyID:     auditPolicyID,
		ResourceType: auditResourceType,
		Limit:        auditLimit,
	}

	if auditDeniedOnly {
		denied := false
		filter.Allowed = &denied
	}

	// Get entries
	entries := auditor.GetEntries(filter)

	if len(entries) == 0 {
		fmt.Println("No audit entries found.")
		fmt.Println("\nNote: This CLI reads from an in-memory store.")
		fmt.Println("For production audit logs, use the control plane API.")
		return nil
	}

	// Output
	if auditOutputFmt == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(entries)
	}

	// Table output
	fmt.Printf("%-20s %-25s %-15s %-10s %-10s\n", "TIMESTAMP", "POLICY", "RESOURCE", "RESULT", "VIOLATIONS")
	fmt.Println(strings.Repeat("-", 85))
	for _, entry := range entries {
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
}

// =============================================================================
// Report Command
// =============================================================================

var (
	reportDays      int
	reportOutputFmt string
)

var reportCmd = &cobra.Command{
	Use:   "report",
	Short: "Generate compliance report",
	Long: `Generate a compliance report based on policy evaluations.

Note: This command generates a report from the in-memory audit store.
For production use, connect to the control plane API.

Examples:
  # Generate report for last 7 days
  kscorectl policy report --days 7

  # Generate report for last 30 days as JSON
  kscorectl policy report --days 30 --output json`,
	RunE: reportExecute,
}

func init() {
	reportCmd.Flags().IntVar(&reportDays, "days", 7, "Number of days to include in report")
	reportCmd.Flags().StringVarP(&reportOutputFmt, "output", "o", "text", "Output format (text, json)")
}

func reportExecute(cmd *cobra.Command, args []string) error {
	// Create sample components (in production, connect to control plane)
	registry := policy.NewRegistry()
	auditor := policy.NewPolicyAuditor(10000)
	reporter := policy.NewComplianceReporter(auditor, registry)

	// Generate report
	period := policy.ReportPeriod{
		Start: time.Now().AddDate(0, 0, -reportDays),
		End:   time.Now(),
	}

	report := reporter.GenerateReport(period)

	// Output
	if reportOutputFmt == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}

	// Text output
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
}

// =============================================================================
// Helper Functions
// =============================================================================

func loadPolicyFile(path string) (*PolicyFile, error) {
	// Resolve path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve path: %w", err)
	}

	// Read file
	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	// Parse YAML
	var policyFile PolicyFile
	if err := yaml.Unmarshal(data, &policyFile); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	return &policyFile, nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
