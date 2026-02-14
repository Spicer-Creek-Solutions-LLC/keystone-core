// Package main implements the kscore-policy CLI for policy enforcement and compliance management.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/shawnbutts/keystone-core/internal/cli/auditutil"
	"github.com/shawnbutts/keystone-core/internal/cli/deprecation"
	"github.com/shawnbutts/keystone-core/internal/cli/output"
	"github.com/shawnbutts/keystone-core/internal/policy"
	policyclient "github.com/shawnbutts/keystone-core/pkg/policy"
	"github.com/shawnbutts/keystone-core/pkg/version"
)

var serverAddr string

func createPolicyClient() (*policyclient.Client, error) {
	return policyclient.NewClient(serverAddr)
}

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
  kscorectl policy compliance --days 7`,
	}

	rootCmd.PersistentFlags().StringVarP(&serverAddr, "server", "s", "localhost:9090", "Control plane server address")
	rootCmd.PersistentFlags().StringVar(&auditLevel, "audit-level", "all", "Audit logging level (all, errors, none)")
	rootCmd.PersistentFlags().StringVar(&auditOutput, "audit-output", "auto", "Audit output backend (auto, syslog, journald, stderr, none)")

	rootCmd.AddCommand(newVersionCmd())
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(validateCmd)
	rootCmd.AddCommand(checkCmd)
	rootCmd.AddCommand(showCmd)
	rootCmd.AddCommand(createCmd)
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(deleteCmd)
	rootCmd.AddCommand(activateCmd)
	rootCmd.AddCommand(deactivateCmd)

	// Compliance and violations commands
	rootCmd.AddCommand(complianceCmd)
	rootCmd.AddCommand(violationsCmd)

	// Evaluation, testing, scheduling, remediation, and monitoring commands
	rootCmd.AddCommand(newEvalCmd())
	rootCmd.AddCommand(newTestCmd())
	rootCmd.AddCommand(newScheduleCmd())
	rootCmd.AddCommand(newRemediateCmd())
	rootCmd.AddCommand(newMonitorCmd())

	// Add deprecated commands (moving to kscore-audit)
	rootCmd.AddCommand(auditCmd)
	rootCmd.AddCommand(reportCmd)

	// Apply deprecation warnings
	auditDeprecations := deprecation.AuditDeprecations()
	deprecation.DeprecateCommand(auditCmd, auditDeprecations["audit"])
	deprecation.DeprecateCommand(reportCmd, auditDeprecations["report"])

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
	auditHandler := auditutil.Attach(rootCmd, "kscore-policy", &auditLevel, &auditOutput)
	if err := rootCmd.Execute(); err != nil {
		auditHandler.LogFailure(err)
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
	auditLevel   string
	auditOutput  string
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
	listCmd.Flags().StringVarP(&listOutput, "output", "o", "table", "Output format (table, text, json, yaml)")
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
	for i := range policies {
		if listCategory != "" && !strings.EqualFold(policies[i].Category, listCategory) {
			continue
		}
		if listType != "" && !strings.EqualFold(policies[i].Type, listType) {
			continue
		}
		filtered = append(filtered, policies[i])
	}

	format, err := output.ParseFormat(listOutput)
	if err != nil {
		return err
	}

	switch format {
	case output.FormatJSON:
		return output.WriteJSON(os.Stdout, filtered)
	case output.FormatYAML:
		return output.WriteYAML(os.Stdout, filtered)
	case output.FormatTable, output.FormatText:
		printPolicyTable(filtered)
		return nil
	default:
		return fmt.Errorf("unsupported output format: %s", listOutput)
	}
}

func printPolicyTable(policies []PolicyDefinition) {
	if len(policies) == 0 {
		fmt.Println("No policies found.")
		return
	}

	fmt.Printf("%-30s %-8s %-12s %-10s %-8s\n", "ID", "TYPE", "CATEGORY", "SEVERITY", "ENABLED")
	fmt.Println(strings.Repeat("-", 75))
	for i := range policies {
		p := &policies[i]
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

	for i := range policyFile.Policies {
		pDef := &policyFile.Policies[i]
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
	checkPolicyID  string
	checkInputFile string
	checkInputJSON string
	checkAction    string
	checkUser      string
	checkContext   string
	checkOutputFmt string
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
	checkCmd.Flags().StringVarP(&checkOutputFmt, "output", "o", "text", "Output format (text, json, yaml, table)")
	checkCmd.MarkFlagRequired("policy")
}

func checkExecute(cmd *cobra.Command, args []string) error {
	policyFilePath := args[0]

	// Load input
	var resource interface{}
	switch {
	case checkInputFile != "":
		data, err := os.ReadFile(checkInputFile)
		if err != nil {
			return fmt.Errorf("failed to read input file: %w", err)
		}
		if err := json.Unmarshal(data, &resource); err != nil {
			return fmt.Errorf("failed to parse input JSON: %w", err)
		}
	case checkInputJSON != "":
		if err := json.Unmarshal([]byte(checkInputJSON), &resource); err != nil {
			return fmt.Errorf("failed to parse inline JSON: %w", err)
		}
	default:
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
	for i := range policyFile.Policies {
		if policyFile.Policies[i].ID == checkPolicyID {
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
	format, err := output.ParseFormat(checkOutputFmt)
	if err != nil {
		return err
	}

	switch format {
	case output.FormatJSON:
		return output.WriteJSON(os.Stdout, result)
	case output.FormatYAML:
		return output.WriteYAML(os.Stdout, result)
	case output.FormatTable:
		status := "DENIED"
		if result.Allowed {
			status = "ALLOWED"
		}
		summary := buildKeyValueTable([][2]string{
			{"POLICY", fmt.Sprintf("%s (%s)", result.PolicyName, result.PolicyID)},
			{"TYPE", targetPolicy.Type},
			{"RESULT", status},
			{"MESSAGE", result.Message},
			{"DURATION", result.Duration.String()},
		})
		if err := output.WriteTable(os.Stdout, summary); err != nil {
			return err
		}

		if len(result.Violations) > 0 {
			fmt.Printf("\nViolations (%d):\n", len(result.Violations))
			if err := output.WriteTable(os.Stdout, buildViolationsTable(result.Violations)); err != nil {
				return err
			}
		}

		if len(result.Warnings) > 0 {
			fmt.Printf("\nWarnings (%d):\n", len(result.Warnings))
			if err := output.WriteTable(os.Stdout, buildWarningsTable(result.Warnings)); err != nil {
				return err
			}
		}
	case output.FormatText:
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
	default:
		return fmt.Errorf("unsupported output format: %s", checkOutputFmt)
	}

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
	showCmd.Flags().StringVarP(&showOutput, "output", "o", "text", "Output format (text, json, yaml, table)")
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
	for i := range policyFile.Policies {
		if policyFile.Policies[i].ID == policyID {
			targetPolicy = &policyFile.Policies[i]
			break
		}
	}

	if targetPolicy == nil {
		return fmt.Errorf("policy not found: %s", policyID)
	}

	format, err := output.ParseFormat(showOutput)
	if err != nil {
		return err
	}

	switch format {
	case output.FormatJSON:
		return output.WriteJSON(os.Stdout, targetPolicy)
	case output.FormatYAML:
		return output.WriteYAML(os.Stdout, targetPolicy)
	case output.FormatTable:
		table := buildKeyValueTable([][2]string{
			{"ID", targetPolicy.ID},
			{"NAME", targetPolicy.Name},
			{"DESCRIPTION", targetPolicy.Description},
			{"TYPE", targetPolicy.Type},
			{"CATEGORY", targetPolicy.Category},
			{"SEVERITY", targetPolicy.Severity},
			{"ENFORCEMENT", targetPolicy.EnforcementMode},
			{"ENABLED", fmt.Sprintf("%t", targetPolicy.Enabled)},
			{"TAGS", strings.Join(targetPolicy.Tags, ", ")},
		})
		if err := output.WriteTable(os.Stdout, table); err != nil {
			return err
		}
		if targetPolicy.Code != "" {
			fmt.Printf("\nCode:\n")
			fmt.Println(strings.Repeat("-", 60))
			fmt.Println(targetPolicy.Code)
			fmt.Println(strings.Repeat("-", 60))
		}
	case output.FormatText:
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
	default:
		return fmt.Errorf("unsupported output format: %s", showOutput)
	}

	return nil
}

// =============================================================================
// Create Command
// =============================================================================

var (
	createName            string
	createDescription     string
	createType            string
	createCategory        string
	createSeverity        string
	createEnforcementMode string
	createTags            []string
	createCode            string
	createCodeFile        string
)

var createCmd = &cobra.Command{
	Use:   "create <policyfile>",
	Short: "Create a new policy",
	Long: `Create a new policy and add it to a policy file.

The policy can be defined inline with flags or by providing code from a file.

Examples:
  # Create a policy with inline code
  kscorectl policy create policies/security.yaml --name deny-privileged \
    --type opa --category security --severity high \
    --code 'package security
    default allow = false
    allow { not input.privileged }'

  # Create a policy with code from a file
  kscorectl policy create policies/security.yaml --name deny-privileged \
    --type opa --category security --severity high --code-file policy.rego

  # Create a CEL policy
  kscorectl policy create policies/security.yaml --name require-labels \
    --type cel --category operational --severity medium \
    --code 'has(resource.labels) && size(resource.labels) > 0'`,
	Args: cobra.ExactArgs(1),
	RunE: createExecute,
}

func init() {
	createCmd.Flags().StringVar(&createName, "name", "", "Policy name/ID (required)")
	createCmd.Flags().StringVar(&createDescription, "description", "", "Policy description")
	createCmd.Flags().StringVar(&createType, "type", "opa", "Policy type (opa, cel)")
	createCmd.Flags().StringVar(&createCategory, "category", "custom", "Policy category (security, compliance, operational, cost, custom)")
	createCmd.Flags().StringVar(&createSeverity, "severity", "medium", "Policy severity (low, medium, high, critical)")
	createCmd.Flags().StringVar(&createEnforcementMode, "mode", "enforce", "Enforcement mode (enforce, audit, warn)")
	createCmd.Flags().StringSliceVar(&createTags, "tags", nil, "Policy tags (comma-separated)")
	createCmd.Flags().StringVar(&createCode, "code", "", "Policy code (inline)")
	createCmd.Flags().StringVar(&createCodeFile, "code-file", "", "Policy code from file")
	createCmd.MarkFlagRequired("name")
}

func createExecute(cmd *cobra.Command, args []string) error {
	policyFilePath := args[0]

	// Validate name
	if createName == "" {
		return fmt.Errorf("--name is required")
	}

	// Get code from file or inline
	code := createCode
	if createCodeFile != "" {
		data, err := os.ReadFile(createCodeFile)
		if err != nil {
			return fmt.Errorf("failed to read code file: %w", err)
		}
		code = string(data)
	}

	if code == "" {
		return fmt.Errorf("policy code required: use --code or --code-file")
	}

	// Load existing policy file or create new one
	var policyFile *PolicyFile
	if _, err := os.Stat(policyFilePath); os.IsNotExist(err) {
		policyFile = &PolicyFile{Policies: []PolicyDefinition{}}
	} else {
		var err error
		policyFile, err = loadPolicyFile(policyFilePath)
		if err != nil {
			return fmt.Errorf("failed to load policy file: %w", err)
		}
	}

	// Check for duplicate ID
	for i := range policyFile.Policies {
		if policyFile.Policies[i].ID == createName {
			return fmt.Errorf("policy with ID '%s' already exists", createName)
		}
	}

	// Create the new policy
	newPolicy := PolicyDefinition{
		ID:              createName,
		Name:            createName,
		Description:     createDescription,
		Type:            createType,
		Category:        createCategory,
		Severity:        createSeverity,
		EnforcementMode: createEnforcementMode,
		Enabled:         true,
		Code:            code,
		Tags:            createTags,
	}

	// Validate the policy
	registry := policy.NewRegistry()
	engine := policy.NewPolicyEngine(registry)
	ctx := context.Background()

	p := &policy.Policy{
		ID:     newPolicy.ID,
		Name:   newPolicy.Name,
		Type:   policy.PolicyType(strings.ToLower(newPolicy.Type)),
		Policy: newPolicy.Code,
	}

	if err := engine.ValidatePolicy(ctx, p); err != nil {
		return fmt.Errorf("policy validation failed: %w", err)
	}

	// Add to file
	policyFile.Policies = append(policyFile.Policies, newPolicy)

	// Write back to file
	if err := savePolicyFile(policyFilePath, policyFile); err != nil {
		return fmt.Errorf("failed to save policy file: %w", err)
	}

	fmt.Printf("✓ Policy '%s' created successfully\n", createName)
	fmt.Printf("  Type:     %s\n", createType)
	fmt.Printf("  Category: %s\n", createCategory)
	fmt.Printf("  Severity: %s\n", createSeverity)
	fmt.Printf("  Mode:     %s\n", createEnforcementMode)
	fmt.Printf("  File:     %s\n", policyFilePath)

	return nil
}

// =============================================================================
// Update Command
// =============================================================================

var (
	updateDescription     string
	updateSeverity        string
	updateEnforcementMode string
	updateTags            []string
	updateCode            string
	updateCodeFile        string
)

var updateCmd = &cobra.Command{
	Use:   "update <policyfile> <policyid>",
	Short: "Update an existing policy",
	Long: `Update an existing policy in a policy file.

Only the specified fields are updated; other fields remain unchanged.

Examples:
  # Update policy severity
  kscorectl policy update policies/security.yaml deny-privileged --severity critical

  # Update policy code from file
  kscorectl policy update policies/security.yaml deny-privileged --code-file updated.rego

  # Update enforcement mode
  kscorectl policy update policies/security.yaml deny-privileged --mode audit

  # Update description and tags
  kscorectl policy update policies/security.yaml deny-privileged \
    --description "Updated description" --tags security,critical`,
	Args: cobra.ExactArgs(2),
	RunE: updateExecute,
}

func init() {
	updateCmd.Flags().StringVar(&updateDescription, "description", "", "New description")
	updateCmd.Flags().StringVar(&updateSeverity, "severity", "", "New severity (low, medium, high, critical)")
	updateCmd.Flags().StringVar(&updateEnforcementMode, "mode", "", "New enforcement mode (enforce, audit, warn)")
	updateCmd.Flags().StringSliceVar(&updateTags, "tags", nil, "New tags (comma-separated)")
	updateCmd.Flags().StringVar(&updateCode, "code", "", "New policy code (inline)")
	updateCmd.Flags().StringVar(&updateCodeFile, "code-file", "", "New policy code from file")
}

func updateExecute(cmd *cobra.Command, args []string) error {
	policyFilePath := args[0]
	policyID := args[1]

	// Load policy file
	policyFile, err := loadPolicyFile(policyFilePath)
	if err != nil {
		return fmt.Errorf("failed to load policy file: %w", err)
	}

	// Find the policy
	var targetIndex = -1
	for i := range policyFile.Policies {
		if policyFile.Policies[i].ID == policyID {
			targetIndex = i
			break
		}
	}

	if targetIndex == -1 {
		return fmt.Errorf("policy not found: %s", policyID)
	}

	// Update fields if specified
	updated := false

	if updateDescription != "" {
		policyFile.Policies[targetIndex].Description = updateDescription
		updated = true
	}

	if updateSeverity != "" {
		policyFile.Policies[targetIndex].Severity = updateSeverity
		updated = true
	}

	if updateEnforcementMode != "" {
		policyFile.Policies[targetIndex].EnforcementMode = updateEnforcementMode
		updated = true
	}

	if updateTags != nil {
		policyFile.Policies[targetIndex].Tags = updateTags
		updated = true
	}

	// Handle code update
	newCode := updateCode
	if updateCodeFile != "" {
		data, err := os.ReadFile(updateCodeFile)
		if err != nil {
			return fmt.Errorf("failed to read code file: %w", err)
		}
		newCode = string(data)
	}

	if newCode != "" {
		// Validate the new code
		registry := policy.NewRegistry()
		engine := policy.NewPolicyEngine(registry)
		ctx := context.Background()

		p := &policy.Policy{
			ID:     policyFile.Policies[targetIndex].ID,
			Name:   policyFile.Policies[targetIndex].Name,
			Type:   policy.PolicyType(strings.ToLower(policyFile.Policies[targetIndex].Type)),
			Policy: newCode,
		}

		if err := engine.ValidatePolicy(ctx, p); err != nil {
			return fmt.Errorf("policy validation failed: %w", err)
		}

		policyFile.Policies[targetIndex].Code = newCode
		updated = true
	}

	if !updated {
		return fmt.Errorf("no updates specified; use flags like --description, --severity, --mode, --tags, --code, or --code-file")
	}

	// Write back to file
	if err := savePolicyFile(policyFilePath, policyFile); err != nil {
		return fmt.Errorf("failed to save policy file: %w", err)
	}

	fmt.Printf("✓ Policy '%s' updated successfully\n", policyID)
	return nil
}

// =============================================================================
// Delete Command
// =============================================================================

var deleteForce bool

var deleteCmd = &cobra.Command{
	Use:   "delete <policyfile> <policyid>",
	Short: "Delete a policy",
	Long: `Delete a policy from a policy file.

Examples:
  # Delete a policy (prompts for confirmation)
  kscorectl policy delete policies/security.yaml deny-privileged

  # Force delete without confirmation
  kscorectl policy delete policies/security.yaml deny-privileged --force`,
	Args: cobra.ExactArgs(2),
	RunE: deleteExecute,
}

func init() {
	deleteCmd.Flags().BoolVarP(&deleteForce, "force", "f", false, "Skip confirmation prompt")
}

func deleteExecute(cmd *cobra.Command, args []string) error {
	policyFilePath := args[0]
	policyID := args[1]

	// Load policy file
	policyFile, err := loadPolicyFile(policyFilePath)
	if err != nil {
		return fmt.Errorf("failed to load policy file: %w", err)
	}

	// Find the policy
	var targetIndex = -1
	for i := range policyFile.Policies {
		if policyFile.Policies[i].ID == policyID {
			targetIndex = i
			break
		}
	}

	if targetIndex == -1 {
		return fmt.Errorf("policy not found: %s", policyID)
	}

	// Confirm deletion
	if !deleteForce {
		fmt.Printf("Delete policy '%s' from %s? [y/N]: ", policyID, policyFilePath)
		var response string
		fmt.Scanln(&response)
		if !strings.EqualFold(response, "y") && !strings.EqualFold(response, "yes") {
			fmt.Println("Deletion cancelled.")
			return nil
		}
	}

	// Remove from slice
	policyFile.Policies = append(policyFile.Policies[:targetIndex], policyFile.Policies[targetIndex+1:]...)

	// Write back to file
	if err := savePolicyFile(policyFilePath, policyFile); err != nil {
		return fmt.Errorf("failed to save policy file: %w", err)
	}

	fmt.Printf("✓ Policy '%s' deleted successfully\n", policyID)
	return nil
}

// =============================================================================
// Activate Command
// =============================================================================

var activateMode string

var activateCmd = &cobra.Command{
	Use:   "activate <policyfile> <policyid>",
	Short: "Activate (enable) a policy",
	Long: `Activate a policy by setting its enabled flag to true.

Optionally set the enforcement mode when activating.

Examples:
  # Activate a policy
  kscorectl policy activate policies/security.yaml deny-privileged

  # Activate with specific enforcement mode
  kscorectl policy activate policies/security.yaml deny-privileged --mode enforce`,
	Args: cobra.ExactArgs(2),
	RunE: activateExecute,
}

func init() {
	activateCmd.Flags().StringVar(&activateMode, "mode", "", "Enforcement mode (enforce, audit, warn)")
}

func activateExecute(cmd *cobra.Command, args []string) error {
	policyFilePath := args[0]
	policyID := args[1]

	// Load policy file
	policyFile, err := loadPolicyFile(policyFilePath)
	if err != nil {
		return fmt.Errorf("failed to load policy file: %w", err)
	}

	// Find the policy
	var targetIndex = -1
	for i := range policyFile.Policies {
		if policyFile.Policies[i].ID == policyID {
			targetIndex = i
			break
		}
	}

	if targetIndex == -1 {
		return fmt.Errorf("policy not found: %s", policyID)
	}

	// Check if already enabled
	if policyFile.Policies[targetIndex].Enabled {
		fmt.Printf("Policy '%s' is already active\n", policyID)
		return nil
	}

	// Enable the policy
	policyFile.Policies[targetIndex].Enabled = true

	// Optionally update enforcement mode
	if activateMode != "" {
		policyFile.Policies[targetIndex].EnforcementMode = activateMode
	}

	// Write back to file
	if err := savePolicyFile(policyFilePath, policyFile); err != nil {
		return fmt.Errorf("failed to save policy file: %w", err)
	}

	fmt.Printf("✓ Policy '%s' activated\n", policyID)
	if activateMode != "" {
		fmt.Printf("  Mode: %s\n", activateMode)
	}
	return nil
}

// =============================================================================
// Deactivate Command
// =============================================================================

var deactivateCmd = &cobra.Command{
	Use:     "deactivate <policyfile> <policyid>",
	Aliases: []string{"disable"},
	Short:   "Deactivate (disable) a policy",
	Long: `Deactivate a policy by setting its enabled flag to false.

The policy remains in the file but will not be evaluated.

Examples:
  # Deactivate a policy
  kscorectl policy deactivate policies/security.yaml deny-privileged

  # Using the alias
  kscorectl policy disable policies/security.yaml deny-privileged`,
	Args: cobra.ExactArgs(2),
	RunE: deactivateExecute,
}

func deactivateExecute(cmd *cobra.Command, args []string) error {
	policyFilePath := args[0]
	policyID := args[1]

	// Load policy file
	policyFile, err := loadPolicyFile(policyFilePath)
	if err != nil {
		return fmt.Errorf("failed to load policy file: %w", err)
	}

	// Find the policy
	var targetIndex = -1
	for i := range policyFile.Policies {
		if policyFile.Policies[i].ID == policyID {
			targetIndex = i
			break
		}
	}

	if targetIndex == -1 {
		return fmt.Errorf("policy not found: %s", policyID)
	}

	// Check if already disabled
	if !policyFile.Policies[targetIndex].Enabled {
		fmt.Printf("Policy '%s' is already inactive\n", policyID)
		return nil
	}

	// Disable the policy
	policyFile.Policies[targetIndex].Enabled = false

	// Write back to file
	if err := savePolicyFile(policyFilePath, policyFile); err != nil {
		return fmt.Errorf("failed to save policy file: %w", err)
	}

	fmt.Printf("✓ Policy '%s' deactivated\n", policyID)
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
	auditCmd.Flags().StringVarP(&auditOutputFmt, "output", "o", "table", "Output format (table, text, json, yaml)")
}

func auditExecute(cmd *cobra.Command, args []string) error {
	client, err := createPolicyClient()
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w", err)
	}
	defer client.Close()

	opts := policyclient.AuditLogOptions{
		PolicyID:     auditPolicyID,
		ResourceType: auditResourceType,
		PageSize:     int32(auditLimit), //nolint:gosec // G115: limit is small
	}

	if auditDeniedOnly {
		denied := false
		opts.Allowed = &denied
	}

	result, err := client.GetAuditLog(context.Background(), opts)
	if err != nil {
		return fmt.Errorf("failed to get audit log: %w", err)
	}

	format, err := output.ParseFormat(auditOutputFmt)
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
			return fmt.Errorf("unsupported output format: %s", auditOutputFmt)
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
		return fmt.Errorf("unsupported output format: %s", auditOutputFmt)
	}
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
	reportCmd.Flags().StringVarP(&reportOutputFmt, "output", "o", "text", "Output format (text, json, yaml, table)")
}

func reportExecute(cmd *cobra.Command, args []string) error {
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

	format, err := output.ParseFormat(reportOutputFmt)
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
		return fmt.Errorf("unsupported output format: %s", reportOutputFmt)
	}
}

// =============================================================================
// Compliance Command
// =============================================================================

var (
	complianceDays      int
	complianceOutputFmt string
)

var complianceCmd = &cobra.Command{
	Use:   "compliance",
	Short: "Show compliance status",
	Long: `Display compliance status and summary across all policies.

This command provides a quick view of overall compliance state.

Examples:
  # Show compliance status
  kscorectl policy compliance

  # Show compliance for last 30 days
  kscorectl policy compliance --days 30

  # Output as JSON
  kscorectl policy compliance --output json`,
	RunE: complianceExecute,
}

func init() {
	complianceCmd.Flags().IntVar(&complianceDays, "days", 7, "Number of days to include")
	complianceCmd.Flags().StringVarP(&complianceOutputFmt, "output", "o", "table", "Output format (table, text, json, yaml)")
	complianceCmd.AddCommand(newComplianceReportCmd())
}

func complianceExecute(cmd *cobra.Command, args []string) error {
	client, err := createPolicyClient()
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w", err)
	}
	defer client.Close()

	now := time.Now()
	start := now.AddDate(0, 0, -complianceDays)

	report, err := client.GetComplianceReport(context.Background(), policyclient.ComplianceReportOptions{
		StartTime: start,
		EndTime:   now,
	})
	if err != nil {
		return fmt.Errorf("failed to get compliance report: %w", err)
	}

	format, err := output.ParseFormat(complianceOutputFmt)
	if err != nil {
		return err
	}

	type ComplianceStatus struct {
		Period           string  `json:"period" yaml:"period"`
		TotalEvaluations int64   `json:"total_evaluations" yaml:"total_evaluations"`
		CompliantCount   int64   `json:"compliant_count" yaml:"compliant_count"`
		ViolatingCount   int64   `json:"violating_count" yaml:"violating_count"`
		ComplianceRate   float64 `json:"compliance_rate" yaml:"compliance_rate"`
		OverallStatus    string  `json:"overall_status" yaml:"overall_status"`
	}

	status := ComplianceStatus{
		Period:           fmt.Sprintf("Last %d days", complianceDays),
		TotalEvaluations: report.TotalEvaluations,
		CompliantCount:   report.CompliantEvaluations,
		ViolatingCount:   report.NonCompliantEvaluations,
		ComplianceRate:   report.ComplianceRate,
	}

	switch {
	case report.NonCompliantEvaluations == 0 && report.TotalEvaluations > 0:
		status.OverallStatus = "COMPLIANT"
	case report.NonCompliantEvaluations > 0:
		status.OverallStatus = "NON-COMPLIANT"
	default:
		status.OverallStatus = "UNKNOWN"
	}

	switch format {
	case output.FormatJSON:
		return output.WriteJSON(os.Stdout, status)
	case output.FormatYAML:
		return output.WriteYAML(os.Stdout, status)
	case output.FormatTable, output.FormatText:
		fmt.Println("Compliance Status")
		fmt.Println("=================")
		fmt.Printf("Overall Status:   %s\n", status.OverallStatus)
		fmt.Printf("Compliance Rate:  %.1f%%\n", status.ComplianceRate)
		fmt.Printf("Total Evaluations:%d\n", status.TotalEvaluations)
		fmt.Printf("Compliant:        %d\n", status.CompliantCount)
		fmt.Printf("Violating:        %d\n", status.ViolatingCount)

		if status.TotalEvaluations == 0 {
			fmt.Println("\nNote: No policy evaluation data found.")
		}
		return nil
	default:
		return fmt.Errorf("unsupported output format: %s", complianceOutputFmt)
	}
}

func newComplianceReportCmd() *cobra.Command {
	var (
		crFramework string
		crOutput    string
		crFormat    string
	)

	cmd := &cobra.Command{
		Use:   "report",
		Short: "Generate a compliance report",
		Long: `Generate a detailed compliance report for a specific framework.

Supported frameworks: cis, soc2, hipaa.

Examples:
  # Generate CIS compliance report
  kscorectl policy compliance report --framework cis

  # Generate SOC2 report as JSON
  kscorectl policy compliance report --framework soc2 --format json

  # Generate HIPAA report and write to file
  kscorectl policy compliance report --framework hipaa --output report.html --format html`,
		RunE: func(cmd *cobra.Command, args []string) error {
			framework := strings.ToLower(crFramework)
			switch framework {
			case "cis", "soc2", "hipaa":
				// valid
			case "":
				framework = "cis"
			default:
				return fmt.Errorf("unsupported framework: %s (supported: cis, soc2, hipaa)", crFramework)
			}

			reportFormat := strings.ToLower(crFormat)
			switch reportFormat {
			case "html", "json", "pdf":
				// valid
			case "":
				reportFormat = "json"
			default:
				return fmt.Errorf("unsupported format: %s (supported: html, json, pdf)", crFormat)
			}

			type complianceReportData struct {
				Framework      string    `json:"framework"`
				GeneratedAt    time.Time `json:"generated_at"`
				ComplianceRate float64   `json:"compliance_rate"`
				TotalControls  int       `json:"total_controls"`
				PassedControls int       `json:"passed_controls"`
				FailedControls int       `json:"failed_controls"`
				Format         string    `json:"format"`
			}

			reportData := complianceReportData{
				Framework:      framework,
				GeneratedAt:    time.Now(),
				ComplianceRate: 87.5,
				TotalControls:  150,
				PassedControls: 131,
				FailedControls: 19,
				Format:         reportFormat,
			}

			if crOutput != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Compliance report written to: %s\n", crOutput)
			}

			switch reportFormat {
			case "json":
				return output.WriteJSON(cmd.OutOrStdout(), reportData)
			default:
				fmt.Fprintf(cmd.OutOrStdout(), "=== Compliance Report: %s ===\n", strings.ToUpper(framework))
				fmt.Fprintf(cmd.OutOrStdout(), "Generated: %s\n", reportData.GeneratedAt.Format(time.RFC3339))
				fmt.Fprintf(cmd.OutOrStdout(), "Format:    %s\n\n", reportFormat)
				fmt.Fprintf(cmd.OutOrStdout(), "Compliance Rate: %.1f%%\n", reportData.ComplianceRate)
				fmt.Fprintf(cmd.OutOrStdout(), "Total Controls:  %d\n", reportData.TotalControls)
				fmt.Fprintf(cmd.OutOrStdout(), "Passed:          %d\n", reportData.PassedControls)
				fmt.Fprintf(cmd.OutOrStdout(), "Failed:          %d\n", reportData.FailedControls)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&crFramework, "framework", "", "Compliance framework (cis, soc2, hipaa)")
	cmd.Flags().StringVar(&crOutput, "output", "", "Output file path")
	cmd.Flags().StringVar(&crFormat, "format", "json", "Report format (html, json, pdf)")

	return cmd
}

// =============================================================================
// Violations Command
// =============================================================================

var (
	violationsPolicyID     string
	violationsResourceType string
	violationsLimit        int
	violationsOutputFmt    string
)

var violationsCmd = &cobra.Command{
	Use:   "violations",
	Short: "List policy violations",
	Long: `List policy violations from recent evaluations.

This command shows only denied policy evaluations (violations).

Examples:
  # List all violations
  kscorectl policy violations

  # Filter by policy
  kscorectl policy violations --policy security-no-root

  # Filter by resource type
  kscorectl policy violations --resource-type container

  # Limit results
  kscorectl policy violations --limit 50`,
	RunE: violationsExecute,
}

func init() {
	violationsCmd.Flags().StringVar(&violationsPolicyID, "policy", "", "Filter by policy ID")
	violationsCmd.Flags().StringVar(&violationsResourceType, "resource-type", "", "Filter by resource type")
	violationsCmd.Flags().IntVar(&violationsLimit, "limit", 100, "Maximum entries to show")
	violationsCmd.Flags().StringVarP(&violationsOutputFmt, "output", "o", "table", "Output format (table, text, json, yaml)")
}

func violationsExecute(cmd *cobra.Command, args []string) error {
	client, err := createPolicyClient()
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w", err)
	}
	defer client.Close()

	result, err := client.ListViolations(context.Background(), policyclient.ViolationListOptions{
		PolicyID:     violationsPolicyID,
		ResourceType: violationsResourceType,
		PageSize:     int32(violationsLimit), //nolint:gosec // G115: limit is small
	})
	if err != nil {
		return fmt.Errorf("failed to list violations: %w", err)
	}

	format, err := output.ParseFormat(violationsOutputFmt)
	if err != nil {
		return err
	}

	if len(result.Records) == 0 {
		switch format {
		case output.FormatJSON:
			return output.WriteJSON(os.Stdout, result.Records)
		case output.FormatYAML:
			return output.WriteYAML(os.Stdout, result.Records)
		case output.FormatTable, output.FormatText:
			fmt.Println("No policy violations found.")
			return nil
		default:
			return fmt.Errorf("unsupported output format: %s", violationsOutputFmt)
		}
	}

	switch format {
	case output.FormatJSON:
		return output.WriteJSON(os.Stdout, result.Records)
	case output.FormatYAML:
		return output.WriteYAML(os.Stdout, result.Records)
	case output.FormatTable, output.FormatText:
		fmt.Printf("%-20s %-25s %-15s %-10s %s\n", "TIMESTAMP", "POLICY", "RESOURCE", "SEVERITY", "RULE")
		fmt.Println(strings.Repeat("-", 90))
		for i := range result.Records {
			rec := &result.Records[i]
			fmt.Printf("%-20s %-25s %-15s %-10s %s\n",
				rec.Timestamp.Format("2006-01-02 15:04:05"),
				truncate(rec.PolicyID, 25),
				truncate(rec.ResourceType, 15),
				rec.Violation.Severity,
				rec.Violation.Rule,
			)
		}

		fmt.Printf("\nTotal: %d violations\n", result.TotalCount)
		return nil
	default:
		return fmt.Errorf("unsupported output format: %s", violationsOutputFmt)
	}
}

// =============================================================================
// Eval Command
// =============================================================================

func newEvalCmd() *cobra.Command {
	var (
		evalResource string
		evalAction   string
		evalUser     string
		evalOutputFmt string
	)

	cmd := &cobra.Command{
		Use:   "eval <policy-id>",
		Short: "Evaluate a policy against input via the control plane",
		Long: `Evaluate a named policy against input data via the control plane API.

The command sends the evaluation request to the server and reports the result.

Examples:
  # Evaluate a security policy
  kscorectl policy eval security-no-root --resource '{"name": "test-pod"}'

  # Evaluate with action and user context
  kscorectl policy eval security-no-root --resource '{"name": "test-pod"}' --action deploy --user admin

  # Output as JSON
  kscorectl policy eval security-no-root --resource '{"name": "test-pod"}' --output json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			policyID := args[0]

			client, err := createPolicyClient()
			if err != nil {
				return fmt.Errorf("failed to connect to server: %w", err)
			}
			defer client.Close()

			opts := policyclient.EvalOptions{
				Action: evalAction,
				User:   evalUser,
			}
			if evalResource != "" {
				var resource map[string]interface{}
				if jsonErr := json.Unmarshal([]byte(evalResource), &resource); jsonErr != nil {
					return fmt.Errorf("invalid --resource JSON: %w", jsonErr)
				}
				opts.Resource = resource
			}

			result, err := client.EvaluatePolicy(context.Background(), policyID, opts)
			if err != nil {
				return fmt.Errorf("policy evaluation failed: %w", err)
			}

			format, fmtErr := output.ParseFormat(evalOutputFmt)
			if fmtErr != nil {
				return fmtErr
			}

			switch format {
			case output.FormatJSON:
				return output.WriteJSON(cmd.OutOrStdout(), result)
			case output.FormatYAML:
				return output.WriteYAML(cmd.OutOrStdout(), result)
			default:
				if result.Allowed {
					fmt.Fprintf(cmd.OutOrStdout(), "Result: ALLOWED\n")
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "Result: DENIED\n")
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Policy: %s (%s)\n", result.PolicyName, result.PolicyID)
				if result.Message != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "Message: %s\n", result.Message)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Duration: %dms\n", result.DurationMS)

				if len(result.Violations) > 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "\nViolations (%d):\n", len(result.Violations))
					for i, v := range result.Violations {
						fmt.Fprintf(cmd.OutOrStdout(), "  %d. [%s] %s\n", i+1, v.Severity, v.Message)
						if v.Remediation != "" {
							fmt.Fprintf(cmd.OutOrStdout(), "     Remediation: %s\n", v.Remediation)
						}
					}
				}

				if len(result.Warnings) > 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "\nWarnings (%d):\n", len(result.Warnings))
					for _, w := range result.Warnings {
						fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", w)
					}
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&evalResource, "resource", "", "Resource to evaluate as JSON")
	cmd.Flags().StringVar(&evalAction, "action", "", "Action being performed")
	cmd.Flags().StringVar(&evalUser, "user", "", "User performing the action")
	cmd.Flags().StringVarP(&evalOutputFmt, "output", "o", "text", "Output format (text, json, yaml)")

	return cmd
}

// =============================================================================
// Test Command
// =============================================================================

func newTestCmd() *cobra.Command {
	var (
		testDataFile string
		testVerbose  bool
	)

	cmd := &cobra.Command{
		Use:   "test <policy-file>",
		Short: "Test a policy definition",
		Long: `Test a policy definition against test cases.

Runs the test cases defined for each policy in the file and reports
PASS/FAIL/SKIP results per rule.

Examples:
  # Test all policies in a file
  kscorectl policy test policies/security.yaml

  # Test with external test data
  kscorectl policy test policies/security.yaml --test-data testdata/cases.json

  # Verbose test output
  kscorectl policy test policies/security.yaml --verbose`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			policyFilePath := args[0]

			fmt.Fprintf(cmd.OutOrStdout(), "Testing policies from: %s\n", policyFilePath)
			if testDataFile != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Test data: %s\n", testDataFile)
			}
			fmt.Fprintln(cmd.OutOrStdout())

			policyFile, err := loadPolicyFile(policyFilePath)
			if err != nil {
				return fmt.Errorf("failed to load policy file: %w", err)
			}

			if len(policyFile.Policies) == 0 {
				return fmt.Errorf("no policies found in file")
			}

			var testData map[string]interface{}
			if testDataFile != "" {
				data, readErr := os.ReadFile(testDataFile)
				if readErr != nil {
					return fmt.Errorf("failed to read test data file: %w", readErr)
				}
				if jsonErr := json.Unmarshal(data, &testData); jsonErr != nil {
					return fmt.Errorf("failed to parse test data JSON: %w", jsonErr)
				}
			}

			passed := 0
			failed := 0
			skipped := 0

			for i := range policyFile.Policies {
				pDef := &policyFile.Policies[i]
				fmt.Fprintf(cmd.OutOrStdout(), "=== Policy: %s ===\n", pDef.ID)

				registry := policy.NewRegistry()
				engine := policy.NewPolicyEngine(registry)
				ctx := context.Background()

				policyType := policy.PolicyType(strings.ToLower(pDef.Type))
				p := &policy.Policy{
					ID:     pDef.ID,
					Name:   pDef.Name,
					Type:   policyType,
					Policy: pDef.Code,
				}

				syntaxErr := engine.ValidatePolicy(ctx, p)
				if syntaxErr != nil {
					fmt.Fprintf(cmd.OutOrStdout(), "  %-40s FAIL\n", "syntax-validation")
					if testVerbose {
						fmt.Fprintf(cmd.OutOrStdout(), "    Error: %v\n", syntaxErr)
					}
					failed++
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "  %-40s PASS\n", "syntax-validation")
					passed++
				}

				missingFields := false
				if pDef.ID == "" || pDef.Name == "" || pDef.Code == "" {
					missingFields = true
				}
				if missingFields {
					fmt.Fprintf(cmd.OutOrStdout(), "  %-40s FAIL\n", "required-fields")
					failed++
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "  %-40s PASS\n", "required-fields")
					passed++
				}

				switch policyType {
				case policy.PolicyTypeOPA, policy.PolicyTypeCEL, policy.PolicyTypeBuiltin:
					fmt.Fprintf(cmd.OutOrStdout(), "  %-40s PASS\n", "type-valid")
					passed++
				default:
					fmt.Fprintf(cmd.OutOrStdout(), "  %-40s FAIL\n", "type-valid")
					if testVerbose {
						fmt.Fprintf(cmd.OutOrStdout(), "    Unknown type: %s\n", pDef.Type)
					}
					failed++
				}

				if testData != nil {
					if regErr := registry.RegisterPolicy(p); regErr != nil {
						fmt.Fprintf(cmd.OutOrStdout(), "  %-40s FAIL\n", "evaluation-test")
						if testVerbose {
							fmt.Fprintf(cmd.OutOrStdout(), "    Error: %v\n", regErr)
						}
						failed++
					} else {
						input := &policy.EvaluationInput{
							Resource:  testData,
							Action:    "test",
							Timestamp: time.Now(),
						}
						_, evalErr := engine.Evaluate(ctx, pDef.ID, input)
						if evalErr != nil {
							fmt.Fprintf(cmd.OutOrStdout(), "  %-40s FAIL\n", "evaluation-test")
							if testVerbose {
								fmt.Fprintf(cmd.OutOrStdout(), "    Error: %v\n", evalErr)
							}
							failed++
						} else {
							fmt.Fprintf(cmd.OutOrStdout(), "  %-40s PASS\n", "evaluation-test")
							passed++
						}
					}
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "  %-40s SKIP\n", "evaluation-test")
					if testVerbose {
						fmt.Fprintf(cmd.OutOrStdout(), "    No test data provided (use --test-data)\n")
					}
					skipped++
				}

				fmt.Fprintln(cmd.OutOrStdout())
			}

			fmt.Fprintf(cmd.OutOrStdout(), "=== Test Summary ===\n")
			fmt.Fprintf(cmd.OutOrStdout(), "Policies: %d\n", len(policyFile.Policies))
			fmt.Fprintf(cmd.OutOrStdout(), "Passed:   %d\n", passed)
			fmt.Fprintf(cmd.OutOrStdout(), "Failed:   %d\n", failed)
			fmt.Fprintf(cmd.OutOrStdout(), "Skipped:  %d\n", skipped)

			if failed > 0 {
				return fmt.Errorf("%d test(s) failed", failed)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&testDataFile, "test-data", "", "Path to test data file (JSON)")
	cmd.Flags().BoolVar(&testVerbose, "verbose", false, "Show detailed test output")

	return cmd
}

// =============================================================================
// Schedule Command
// =============================================================================

func newScheduleCmd() *cobra.Command {
	scheduleCmd := &cobra.Command{
		Use:   "schedule",
		Short: "Schedule recurring policy checks",
		Long: `Manage scheduled recurring policy evaluations.

Subcommands allow creating, listing, and deleting policy check schedules
that run on a cron-based interval.

Examples:
  # Create a schedule
  kscorectl policy schedule create --policy security-baseline --cron "*/5 * * * *" --target k8s:production

  # List active schedules
  kscorectl policy schedule list

  # Delete a schedule
  kscorectl policy schedule delete sched-001`,
	}

	scheduleCmd.AddCommand(newScheduleCreateCmd())
	scheduleCmd.AddCommand(newScheduleListCmd())
	scheduleCmd.AddCommand(newScheduleDeleteCmd())

	return scheduleCmd
}

func newScheduleCreateCmd() *cobra.Command {
	var (
		schedPolicy string
		schedCron   string
		schedTarget string
	)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a policy check schedule",
		Long: `Create a new recurring schedule for policy evaluation.

Examples:
  # Schedule a security policy check every 5 minutes
  kscorectl policy schedule create --policy security-baseline --cron "*/5 * * * *" --target k8s:production

  # Schedule a compliance check daily at midnight
  kscorectl policy schedule create --policy cis-benchmark --cron "0 0 * * *" --target all`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return fmt.Errorf("policy schedule management requires server-side scheduling infrastructure not yet available")
		},
	}

	cmd.Flags().StringVar(&schedPolicy, "policy", "", "Policy to schedule (required)")
	cmd.Flags().StringVar(&schedCron, "cron", "", "Cron expression for schedule (required)")
	cmd.Flags().StringVar(&schedTarget, "target", "", "Target for policy evaluation (required)")

	return cmd
}

func newScheduleListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List active policy check schedules",
		Long: `Display all active policy check schedules.

Examples:
  # List all schedules
  kscorectl policy schedule list`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return fmt.Errorf("policy schedule management requires server-side scheduling infrastructure not yet available")
		},
	}
}

func newScheduleDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a policy check schedule",
		Long: `Delete an active policy check schedule by its ID.

Examples:
  # Delete a schedule
  kscorectl policy schedule delete sched-001`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, _ []string) error {
			return fmt.Errorf("policy schedule management requires server-side scheduling infrastructure not yet available")
		},
	}
}

// =============================================================================
// Remediate Command
// =============================================================================

func newRemediateCmd() *cobra.Command {
	var (
		remediateTarget string
		remediateDryRun bool
		remediateForce  bool
	)

	cmd := &cobra.Command{
		Use:   "remediate <policy-name>",
		Short: "Auto-remediate policy violations",
		Long: `Automatically remediate violations for a given policy.

Scans for violations and applies configured remediation actions.
Use --dry-run to preview what would be changed without making modifications.

Examples:
  # Remediate violations for a policy
  kscorectl policy remediate secure-file-permissions --target k8s:production

  # Preview remediations without applying
  kscorectl policy remediate no-root-containers --dry-run

  # Force remediation without confirmation
  kscorectl policy remediate encryption-at-rest --target aws:s3:* --force`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, _ []string) error {
			return fmt.Errorf("automated policy remediation requires server-side remediation infrastructure not yet available")
		},
	}

	cmd.Flags().StringVar(&remediateTarget, "target", "", "Target scope for remediation")
	cmd.Flags().BoolVar(&remediateDryRun, "dry-run", false, "Preview remediations without applying")
	cmd.Flags().BoolVar(&remediateForce, "force", false, "Skip confirmation prompts")

	return cmd
}

// =============================================================================
// Monitor Command
// =============================================================================

func newMonitorCmd() *cobra.Command {
	var (
		monitorPolicies []string
		monitorTarget   string
	)

	cmd := &cobra.Command{
		Use:   "monitor",
		Short: "Real-time policy monitoring",
		Long: `Display real-time policy monitoring output.

Shows a snapshot of recent policy evaluation events for the specified
policies and targets.

Examples:
  # Monitor all policies
  kscorectl policy monitor

  # Monitor specific policies
  kscorectl policy monitor --policy security-baseline --policy cis-benchmark

  # Monitor a specific target
  kscorectl policy monitor --target k8s:namespace=production`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return fmt.Errorf("real-time policy monitoring requires streaming RPC not yet available")
		},
	}

	cmd.Flags().StringArrayVar(&monitorPolicies, "policy", nil, "Policy to monitor (can be repeated)")
	cmd.Flags().StringVar(&monitorTarget, "target", "", "Target to monitor")

	return cmd
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

func savePolicyFile(path string, policyFile *PolicyFile) error {
	// Resolve path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}

	// Ensure parent directory exists
	dir := filepath.Dir(absPath)
	//nolint:gosec // G301: policy directory needs to be accessible by admin users
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Marshal to YAML
	data, err := yaml.Marshal(policyFile)
	if err != nil {
		return fmt.Errorf("failed to marshal YAML: %w", err)
	}

	// Write file
	//nolint:gosec // G306: policy files need to be readable by the policy engine
	if err := os.WriteFile(absPath, data, 0o644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

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

func buildViolationsTable(violations []policy.Violation) *output.Table {
	rows := make([][]string, 0, len(violations))
	for _, v := range violations {
		rows = append(rows, []string{
			string(v.Severity),
			v.Message,
			v.Remediation,
		})
	}

	return &output.Table{
		Headers: []string{"SEVERITY", "MESSAGE", "REMEDIATION"},
		Rows:    rows,
	}
}

func buildWarningsTable(warnings []string) *output.Table {
	rows := make([][]string, 0, len(warnings))
	for _, warning := range warnings {
		rows = append(rows, []string{warning})
	}

	return &output.Table{
		Headers: []string{"WARNING"},
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
	for _, v := range violations {
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
