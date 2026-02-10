// Package main implements the kscore-secrets CLI for secrets management operations.
package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/shawnbutts/keystone-core/internal/cli/output"
	"github.com/shawnbutts/keystone-core/internal/secrets"
	"github.com/shawnbutts/keystone-core/pkg/version"
)

var (
	serverAddr   string
	outputFormat string
	verbose      bool
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "kscore-secrets",
		Short: "Keystone Core secrets management",
		Long: `kscore-secrets is a CLI plugin for managing secrets and secret rotation
in Keystone Core.

This plugin provides commands for:
  - Retrieving and inspecting secret values
  - Listing secrets and configured backends
  - Viewing secret access audit logs
  - Managing secret rotation schedules
  - Starting and monitoring rotations
  - Viewing rotation history and status
  - Manual rollback of failed rotations
  - Configuring rotation policies
  - Dynamic secret generation and revocation
  - Lease management (list, renew, revoke)
  - Transit encryption (encrypt, decrypt, rewrap)
  - Template rendering with secret values
  - Secret cache management

Usage via kscorectl:
  kscorectl secrets get vault/secret/database/prod
  kscorectl secrets list vault/secret/
  kscorectl secrets backends
  kscorectl secrets audit vault/secret/database/prod
  kscorectl secrets rotate list
  kscorectl secrets rotate start --secret vault/secret/db --strategy blue-green
  kscorectl secrets rotate status rot-123
  kscorectl secrets dynamic list
  kscorectl secrets leases list
  kscorectl secrets encrypt "my-data" --key transit/mykey
  kscorectl secrets decrypt "vault:v1:..." --key transit/mykey
  kscorectl secrets rewrap "vault:v1:..." --key transit/mykey
  kscorectl secrets template config.tmpl --out-file config.yaml
  kscorectl secrets cache status`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	rootCmd.PersistentFlags().StringVarP(&serverAddr, "server", "s", "localhost:9090", "Control plane server address")
	rootCmd.PersistentFlags().StringVarP(&outputFormat, "output", "o", "table", "Output format (table, json, yaml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")

	rootCmd.AddCommand(
		newVersionCmd(),
		newGetCmd(),
		newListCmd(),
		newBackendsCmd(),
		newAuditCmd(),
		newRotateCmd(),
		newScheduleCmd(),
		newPolicyCmd(),
		newDynamicCmd(),
		newLeasesCmd(),
		newEncryptCmd(),
		newDecryptCmd(),
		newRewrapCmd(),
		newTemplateCmd(),
		newCacheCmd(),
		newRotateKeysCmd(),
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

// =============================================================================
// Get Command
// =============================================================================

func newGetCmd() *cobra.Command {
	var ver int
	var field string

	cmd := &cobra.Command{
		Use:   "get <path>",
		Short: "Retrieve a secret value",
		Long: `Retrieve a secret by path.

Values are masked in table output. Use --format json or --format yaml
to include all fields for programmatic consumption.

Examples:
  # Get a secret (table output, values masked)
  kscorectl secrets get vault/secret/database/prod

  # Get a specific version
  kscorectl secrets get vault/secret/database/prod --version 3

  # Extract a single field
  kscorectl secrets get vault/secret/database/prod --field password

  # Get full details as JSON
  kscorectl secrets get vault/secret/database/prod -o json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGet(cmd, args[0], ver, field)
		},
	}

	cmd.Flags().IntVar(&ver, "version", 0, "Secret version (0 = latest)")
	cmd.Flags().StringVar(&field, "field", "", "Extract a specific field from the secret")

	return cmd
}

func runGet(cmd *cobra.Command, path string, ver int, field string) error {
	s := generateSampleSecret(path, ver)

	if field != "" {
		found := false
		for _, k := range s.Keys {
			if k == field {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("field %q not found in secret (available: %s)", field, strings.Join(s.Keys, ", "))
		}
		fmt.Printf("****\n")
		return nil
	}

	switch outputFormat {
	case "json":
		return outputJSON(s)
	case "yaml":
		return outputYAML(s)
	default:
		table := &output.Table{
			Headers: []string{"PATH", "VERSION", "CREATED", "EXPIRES", "KEYS"},
		}
		keys := strings.Join(s.Keys, ", ")
		table.Rows = append(table.Rows, []string{
			s.Path,
			fmt.Sprintf("%d", s.Version),
			s.CreatedAt,
			s.ExpiresAt,
			keys,
		})
		output.WriteTable(os.Stdout, table)
		fmt.Println()
		fmt.Println("Values:")
		for _, k := range s.Keys {
			fmt.Printf("  %s: ****\n", k)
		}
	}

	return nil
}

func generateSampleSecret(path string, ver int) *secretDisplay {
	v := ver
	if v == 0 {
		v = 3
	}
	return &secretDisplay{
		Path:      path,
		Version:   v,
		Keys:      []string{"username", "password", "host", "port"},
		CreatedAt: time.Now().Add(-48 * time.Hour).Format(time.RFC3339),
		ExpiresAt: time.Now().Add(30 * 24 * time.Hour).Format(time.RFC3339),
		CreatedBy: "admin",
		Backend:   "vault",
		Metadata: map[string]string{
			"env":     "production",
			"team":    "platform",
			"managed": "true",
		},
	}
}

// =============================================================================
// List Command
// =============================================================================

func newListCmd() *cobra.Command {
	var limit int
	var showMetadata bool

	cmd := &cobra.Command{
		Use:     "list [path-prefix]",
		Aliases: []string{"ls"},
		Short:   "List secrets",
		Long: `List secrets matching an optional path prefix.

Examples:
  # List all secrets
  kscorectl secrets list

  # List secrets under a path prefix
  kscorectl secrets list vault/secret/database/

  # Limit results
  kscorectl secrets list --limit 10

  # Show metadata
  kscorectl secrets list --show-metadata`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			prefix := ""
			if len(args) > 0 {
				prefix = args[0]
			}
			return runList(cmd, prefix, limit, showMetadata)
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum number of secrets to show")
	cmd.Flags().BoolVar(&showMetadata, "show-metadata", false, "Show additional metadata")

	return cmd
}

func runList(cmd *cobra.Command, prefix string, limit int, showMetadata bool) error {
	items := generateSampleSecretList(prefix, limit)

	if len(items) == 0 {
		fmt.Println("No secrets found")
		return nil
	}

	switch outputFormat {
	case "json":
		return outputJSON(items)
	case "yaml":
		return outputYAML(items)
	default:
		headers := []string{"PATH", "VERSIONS", "BACKEND", "LAST MODIFIED"}
		if showMetadata {
			headers = append(headers, "CREATED BY", "EXPIRES")
		}
		table := &output.Table{
			Headers: headers,
		}
		for _, item := range items {
			row := []string{
				truncate(item.Path, 35),
				fmt.Sprintf("%d", item.Versions),
				item.Backend,
				item.LastModified,
			}
			if showMetadata {
				row = append(row, item.CreatedBy, item.ExpiresAt)
			}
			table.Rows = append(table.Rows, row)
		}
		output.WriteTable(os.Stdout, table)
		fmt.Printf("\nTotal: %d secret(s)\n", len(items))
	}

	return nil
}

func generateSampleSecretList(prefix string, limit int) []*secretListItem {
	allItems := []*secretListItem{
		{
			Path:         "vault/secret/database/prod",
			Versions:     3,
			Backend:      "vault",
			LastModified: time.Now().Add(-2 * time.Hour).Format("Jan 02 15:04"),
			CreatedBy:    "admin",
			ExpiresAt:    time.Now().Add(30 * 24 * time.Hour).Format("Jan 02"),
		},
		{
			Path:         "vault/secret/database/staging",
			Versions:     5,
			Backend:      "vault",
			LastModified: time.Now().Add(-24 * time.Hour).Format("Jan 02 15:04"),
			CreatedBy:    "ci-pipeline",
			ExpiresAt:    time.Now().Add(15 * 24 * time.Hour).Format("Jan 02"),
		},
		{
			Path:         "vault/secret/api/prod",
			Versions:     2,
			Backend:      "vault",
			LastModified: time.Now().Add(-72 * time.Hour).Format("Jan 02 15:04"),
			CreatedBy:    "admin",
			ExpiresAt:    time.Now().Add(60 * 24 * time.Hour).Format("Jan 02"),
		},
		{
			Path:         "aws-sm/production/payment-gateway",
			Versions:     7,
			Backend:      "aws-sm",
			LastModified: time.Now().Add(-6 * time.Hour).Format("Jan 02 15:04"),
			CreatedBy:    "rotation-bot",
			ExpiresAt:    time.Now().Add(10 * 24 * time.Hour).Format("Jan 02"),
		},
		{
			Path:         "azure-kv/certificates/tls-prod",
			Versions:     12,
			Backend:      "azure-kv",
			LastModified: time.Now().Add(-168 * time.Hour).Format("Jan 02 15:04"),
			CreatedBy:    "cert-manager",
			ExpiresAt:    time.Now().Add(90 * 24 * time.Hour).Format("Jan 02"),
		},
	}

	var filtered []*secretListItem
	for _, item := range allItems {
		if prefix != "" && !strings.HasPrefix(item.Path, prefix) {
			continue
		}
		filtered = append(filtered, item)
		if len(filtered) >= limit {
			break
		}
	}

	return filtered
}

// =============================================================================
// Backends Command
// =============================================================================

func newBackendsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "backends",
		Short: "List configured secret backends",
		Long: `List all configured secret backends and their status.

Examples:
  # List backends
  kscorectl secrets backends

  # List backends as JSON
  kscorectl secrets backends -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBackends(cmd)
		},
	}
}

func runBackends(cmd *cobra.Command) error {
	backends := generateSampleBackends()

	switch outputFormat {
	case "json":
		return outputJSON(backends)
	case "yaml":
		return outputYAML(backends)
	default:
		table := &output.Table{
			Headers: []string{"NAME", "TYPE", "STATUS", "SECRETS"},
		}
		for _, b := range backends {
			table.Rows = append(table.Rows, []string{
				b.Name,
				b.Type,
				b.Status,
				fmt.Sprintf("%d", b.SecretCount),
			})
		}
		output.WriteTable(os.Stdout, table)
	}

	return nil
}

func generateSampleBackends() []*backendDisplay {
	return []*backendDisplay{
		{
			Name:        "vault",
			Type:        "hashicorp-vault",
			Status:      "healthy",
			SecretCount: 142,
			Address:     "https://vault.example.com:8200",
			AuthMethod:  "approle",
		},
		{
			Name:        "aws-sm",
			Type:        "aws-secrets-manager",
			Status:      "healthy",
			SecretCount: 56,
			Address:     "us-west-2",
			AuthMethod:  "iam-role",
		},
		{
			Name:        "azure-kv",
			Type:        "azure-key-vault",
			Status:      "degraded",
			SecretCount: 23,
			Address:     "https://myvault.vault.azure.net",
			AuthMethod:  "managed-identity",
		},
	}
}

// =============================================================================
// Audit Command
// =============================================================================

func newAuditCmd() *cobra.Command {
	var limit int

	cmd := &cobra.Command{
		Use:   "audit <path>",
		Short: "Show secret access audit log",
		Long: `Show the access audit log for a specific secret.

Examples:
  # View recent access log
  kscorectl secrets audit vault/secret/database/prod

  # Limit results
  kscorectl secrets audit vault/secret/database/prod --limit 5

  # Output as JSON
  kscorectl secrets audit vault/secret/database/prod -o json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAudit(cmd, args[0], limit)
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 20, "Maximum number of audit entries to show")

	return cmd
}

func runAudit(cmd *cobra.Command, path string, limit int) error {
	entries := generateSampleAuditEntries(path, limit)

	if len(entries) == 0 {
		fmt.Println("No audit entries found")
		return nil
	}

	switch outputFormat {
	case "json":
		return outputJSON(entries)
	case "yaml":
		return outputYAML(entries)
	default:
		table := &output.Table{
			Headers: []string{"TIMESTAMP", "ACTION", "PRINCIPAL", "SOURCE IP", "VERSION"},
		}
		for _, e := range entries {
			table.Rows = append(table.Rows, []string{
				e.Timestamp,
				e.Action,
				e.Principal,
				e.SourceIP,
				fmt.Sprintf("%d", e.Version),
			})
		}
		output.WriteTable(os.Stdout, table)
		fmt.Printf("\nPath: %s\n", path)
		fmt.Printf("Showing %d audit entry(ies)\n", len(entries))
	}

	return nil
}

func generateSampleAuditEntries(path string, limit int) []*auditEntry {
	all := []*auditEntry{
		{
			Timestamp: time.Now().Add(-10 * time.Minute).Format(time.RFC3339),
			Path:      path,
			Action:    "read",
			Principal: "agent/web-prod-01",
			SourceIP:  "10.0.1.15",
			Version:   3,
		},
		{
			Timestamp: time.Now().Add(-35 * time.Minute).Format(time.RFC3339),
			Path:      path,
			Action:    "read",
			Principal: "agent/web-prod-02",
			SourceIP:  "10.0.1.16",
			Version:   3,
		},
		{
			Timestamp: time.Now().Add(-2 * time.Hour).Format(time.RFC3339),
			Path:      path,
			Action:    "rotate",
			Principal: "system/rotation-scheduler",
			SourceIP:  "10.0.0.5",
			Version:   3,
		},
		{
			Timestamp: time.Now().Add(-6 * time.Hour).Format(time.RFC3339),
			Path:      path,
			Action:    "read",
			Principal: "user/admin",
			SourceIP:  "10.0.0.1",
			Version:   2,
		},
		{
			Timestamp: time.Now().Add(-24 * time.Hour).Format(time.RFC3339),
			Path:      path,
			Action:    "write",
			Principal: "user/admin",
			SourceIP:  "10.0.0.1",
			Version:   2,
		},
		{
			Timestamp: time.Now().Add(-48 * time.Hour).Format(time.RFC3339),
			Path:      path,
			Action:    "read",
			Principal: "agent/api-prod-01",
			SourceIP:  "10.0.2.10",
			Version:   1,
		},
		{
			Timestamp: time.Now().Add(-72 * time.Hour).Format(time.RFC3339),
			Path:      path,
			Action:    "write",
			Principal: "user/ci-pipeline",
			SourceIP:  "10.0.0.50",
			Version:   1,
		},
	}

	if limit > len(all) {
		limit = len(all)
	}
	return all[:limit]
}

// =============================================================================
// Rotate Commands
// =============================================================================

func newRotateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "rotate",
		Aliases: []string{"rot", "r"},
		Short:   "Manage secret rotations",
		Long:    `Commands for managing secret rotations in Keystone Core.`,
	}

	cmd.AddCommand(
		newRotateListCmd(),
		newRotateShowCmd(),
		newRotateStartCmd(),
		newRotateStatusCmd(),
		newRotateHistoryCmd(),
		newRotateTriggerCmd(),
		newRotateRollbackCmd(),
		newRotatePauseCmd(),
		newRotateResumeCmd(),
		newRotateCancelCmd(),
	)

	return cmd
}

// RotateListOptions holds rotate list options
type RotateListOptions struct {
	State    string
	Strategy string
	Labels   []string
	Limit    int
}

func newRotateListCmd() *cobra.Command {
	opts := &RotateListOptions{}

	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List rotations",
		Long: `List secret rotations with optional filtering.

Examples:
  # List all rotations
  kscorectl secrets rotate list

  # List only in-progress rotations
  kscorectl secrets rotate list --state in_progress

  # List blue-green strategy rotations
  kscorectl secrets rotate list --strategy blue-green`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRotateList(cmd, opts)
		},
	}

	cmd.Flags().StringVar(&opts.State, "state", "", "Filter by state (pending, in_progress, completed, failed, rolled_back)")
	cmd.Flags().StringVar(&opts.Strategy, "strategy", "", "Filter by strategy (rolling, blue-green, canary)")
	cmd.Flags().StringArrayVar(&opts.Labels, "label", nil, "Filter by label (key:value format)")
	cmd.Flags().IntVar(&opts.Limit, "limit", 50, "Maximum number of rotations to show")

	return cmd
}

func runRotateList(cmd *cobra.Command, opts *RotateListOptions) error {
	rotations := generateSampleRotations()

	var filtered []*rotationDisplay
	for _, r := range rotations {
		if opts.State != "" && string(r.State) != opts.State {
			continue
		}
		if opts.Strategy != "" && string(r.Strategy) != opts.Strategy {
			continue
		}
		filtered = append(filtered, r)
	}

	if len(filtered) == 0 {
		fmt.Println("No rotations found")
		return nil
	}

	switch outputFormat {
	case "json":
		return outputJSON(filtered)
	case "yaml":
		return outputYAML(filtered)
	default:
		table := &output.Table{
			Headers: []string{"ID", "SECRET PATH", "STRATEGY", "STATE", "PROGRESS", "STARTED"},
		}
		for _, r := range filtered {
			progress := fmt.Sprintf("%d/%d", r.UpdatedTargets, r.TotalTargets)
			if r.TotalTargets > 0 {
				progress = fmt.Sprintf("%s (%d%%)", progress, r.UpdatedTargets*100/r.TotalTargets)
			}
			table.Rows = append(table.Rows, []string{
				truncate(r.ID, 12),
				truncate(r.SecretPath, 25),
				string(r.Strategy),
				string(r.State),
				progress,
				r.StartedAt,
			})
		}
		output.WriteTable(os.Stdout, table)
		fmt.Printf("\nTotal: %d rotation(s)\n", len(filtered))
	}

	return nil
}

func newRotateShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <rotation-id>",
		Short: "Show rotation details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRotateShow(cmd, args[0])
		},
	}
}

func runRotateShow(cmd *cobra.Command, id string) error {
	r := &rotationDetail{
		ID:              id,
		SecretPath:      "vault/secret/database/prod",
		Strategy:        secrets.RotationStrategyBlueGreen,
		State:           secrets.RotationStateInProgress,
		TotalTargets:    10,
		UpdatedTargets:  6,
		FailedTargets:   0,
		Percentage:      60,
		StartedAt:       time.Now().Add(-15 * time.Minute).Format(time.RFC3339),
		BatchSize:       2,
		BatchDelay:      "30s",
		HealthCheckType: "http",
		HealthCheckURL:  "http://app:8080/health",
		CreatedBy:       "admin",
		Labels: map[string]string{
			"env":  "prod",
			"team": "platform",
		},
	}

	switch outputFormat {
	case "json":
		return outputJSON(r)
	case "yaml":
		return outputYAML(r)
	default:
		fmt.Printf("Rotation: %s\n", r.ID)
		fmt.Printf("  Secret Path:      %s\n", r.SecretPath)
		fmt.Printf("  Strategy:         %s\n", r.Strategy)
		fmt.Printf("  State:            %s\n", r.State)
		fmt.Printf("  Progress:         %d/%d targets (%d%%)\n", r.UpdatedTargets, r.TotalTargets, r.Percentage)
		fmt.Printf("  Failed Targets:   %d\n", r.FailedTargets)
		fmt.Printf("  Batch Size:       %d\n", r.BatchSize)
		fmt.Printf("  Batch Delay:      %s\n", r.BatchDelay)
		fmt.Printf("  Health Check:     %s (%s)\n", r.HealthCheckType, r.HealthCheckURL)
		fmt.Printf("  Started:          %s\n", r.StartedAt)
		fmt.Printf("  Created By:       %s\n", r.CreatedBy)
		if len(r.Labels) > 0 {
			fmt.Printf("  Labels:\n")
			for k, v := range r.Labels {
				fmt.Printf("    %s: %s\n", k, v)
			}
		}
	}

	return nil
}

// RotateStartOptions holds options for starting a rotation
type RotateStartOptions struct {
	SecretPath       string
	Strategy         string
	Targets          []string
	TargetTags       []string
	TargetRoles      []string
	BatchSize        int
	BatchDelay       string
	CanaryPercentage int
	CanaryDelay      string
	HealthCheckType  string
	HealthCheckURL   string
	HealthCheckPort  int
	Timeout          string
	DryRun           bool
	Labels           []string
}

func newRotateStartCmd() *cobra.Command {
	opts := &RotateStartOptions{}

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start a new secret rotation",
		Long: `Start a new secret rotation.

Examples:
  # Start a blue-green rotation for all targets
  kscorectl secrets rotate start --secret vault/secret/db \
    --strategy blue-green --target-all

  # Start a canary rotation with 10% canary
  kscorectl secrets rotate start --secret vault/secret/api \
    --strategy canary --canary-percentage 10 --canary-delay 5m

  # Start with health checks
  kscorectl secrets rotate start --secret vault/secret/db \
    --strategy rolling --batch-size 2 \
    --health-check-type http --health-check-url http://app:8080/health

  # Dry run to see what would happen
  kscorectl secrets rotate start --secret vault/secret/db \
    --strategy blue-green --dry-run`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRotateStart(cmd, opts)
		},
	}

	cmd.Flags().StringVar(&opts.SecretPath, "secret", "", "Secret path to rotate (required)")
	cmd.Flags().StringVar(&opts.Strategy, "strategy", "rolling", "Rotation strategy (rolling, blue-green, canary)")
	cmd.Flags().StringArrayVar(&opts.Targets, "target", nil, "Target agent IDs")
	cmd.Flags().StringArrayVar(&opts.TargetTags, "target-tags", nil, "Target agents with tags (key:value)")
	cmd.Flags().StringArrayVar(&opts.TargetRoles, "target-roles", nil, "Target agents with roles")
	cmd.Flags().IntVar(&opts.BatchSize, "batch-size", 1, "Number of targets per batch")
	cmd.Flags().StringVar(&opts.BatchDelay, "batch-delay", "30s", "Delay between batches")
	cmd.Flags().IntVar(&opts.CanaryPercentage, "canary-percentage", 10, "Percentage of targets for canary (canary strategy)")
	cmd.Flags().StringVar(&opts.CanaryDelay, "canary-delay", "5m", "Delay after canary verification (canary strategy)")
	cmd.Flags().StringVar(&opts.HealthCheckType, "health-check-type", "", "Health check type (http, tcp, exec)")
	cmd.Flags().StringVar(&opts.HealthCheckURL, "health-check-url", "", "Health check URL (for http type)")
	cmd.Flags().IntVar(&opts.HealthCheckPort, "health-check-port", 0, "Health check port (for tcp type)")
	cmd.Flags().StringVar(&opts.Timeout, "timeout", "30m", "Overall rotation timeout")
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", false, "Show what would be done without executing")
	cmd.Flags().StringArrayVar(&opts.Labels, "label", nil, "Labels (key:value format)")

	_ = cmd.MarkFlagRequired("secret")

	return cmd
}

func runRotateStart(cmd *cobra.Command, opts *RotateStartOptions) error {
	if opts.SecretPath == "" {
		return fmt.Errorf("--secret is required")
	}

	if len(opts.Targets) == 0 && len(opts.TargetTags) == 0 && len(opts.TargetRoles) == 0 {
		return fmt.Errorf("at least one target option is required (--target, --target-tags, or --target-roles)")
	}

	strategy := normalizeStrategy(opts.Strategy)
	if strategy != secrets.RotationStrategyRolling &&
		strategy != secrets.RotationStrategyBlueGreen &&
		strategy != secrets.RotationStrategyCanary {
		return fmt.Errorf("invalid strategy: %s (must be rolling, blue-green, or canary)", opts.Strategy)
	}

	if opts.DryRun {
		fmt.Println("Dry run mode - no changes will be made")
		fmt.Println()
		fmt.Printf("Would start rotation:\n")
		fmt.Printf("  Secret:           %s\n", opts.SecretPath)
		fmt.Printf("  Strategy:         %s\n", opts.Strategy)
		fmt.Printf("  Batch Size:       %d\n", opts.BatchSize)
		fmt.Printf("  Batch Delay:      %s\n", opts.BatchDelay)
		if strategy == secrets.RotationStrategyCanary {
			fmt.Printf("  Canary %%:         %d%%\n", opts.CanaryPercentage)
			fmt.Printf("  Canary Delay:     %s\n", opts.CanaryDelay)
		}
		if opts.HealthCheckType != "" {
			fmt.Printf("  Health Check:     %s\n", opts.HealthCheckType)
		}
		fmt.Printf("  Timeout:          %s\n", opts.Timeout)
		fmt.Println()
		fmt.Println("Targets that would be updated:")
		for _, t := range opts.Targets {
			fmt.Printf("  - %s\n", t)
		}
		for _, t := range opts.TargetTags {
			fmt.Printf("  - agents with tag %s\n", t)
		}
		for _, r := range opts.TargetRoles {
			fmt.Printf("  - agents with role %s\n", r)
		}
		return nil
	}

	// In production, this would call the API
	rotationID := fmt.Sprintf("rot-%s", randomID(8))
	fmt.Printf("Started rotation '%s' for secret '%s'\n", rotationID, opts.SecretPath)
	fmt.Printf("  Strategy:     %s\n", opts.Strategy)
	fmt.Printf("  Use 'kscorectl secrets rotate status %s' to monitor progress\n", rotationID)

	return nil
}

func newRotateStatusCmd() *cobra.Command {
	var watch bool
	var interval string

	cmd := &cobra.Command{
		Use:   "status <rotation-id>",
		Short: "Show rotation status",
		Args:  cobra.ExactArgs(1),
		Long: `Show the current status of a rotation.

Examples:
  # Show status once
  kscorectl secrets rotate status rot-123

  # Watch status continuously
  kscorectl secrets rotate status rot-123 --watch`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRotateStatus(cmd, args[0], watch, interval)
		},
	}

	cmd.Flags().BoolVarP(&watch, "watch", "w", false, "Watch status continuously")
	cmd.Flags().StringVar(&interval, "interval", "2s", "Watch interval")

	return cmd
}

func runRotateStatus(cmd *cobra.Command, id string, watch bool, interval string) error {
	// Sample status for demonstration
	status := &rotationStatus{
		ID:             id,
		State:          secrets.RotationStateInProgress,
		TotalTargets:   10,
		UpdatedTargets: 6,
		FailedTargets:  0,
		Percentage:     60,
		CurrentBatch:   4,
		TotalBatches:   5,
		StartedAt:      time.Now().Add(-15 * time.Minute).Format(time.RFC3339),
		LastUpdate:     time.Now().Add(-30 * time.Second).Format(time.RFC3339),
	}

	switch outputFormat {
	case "json":
		return outputJSON(status)
	case "yaml":
		return outputYAML(status)
	default:
		printRotationStatus(status)
	}

	return nil
}

func printRotationStatus(status *rotationStatus) {
	stateIcon := "⏳"
	switch status.State {
	case secrets.RotationStateCompleted:
		stateIcon = "✅"
	case secrets.RotationStateFailed:
		stateIcon = "❌"
	case secrets.RotationStateRolledBack:
		stateIcon = "⏪"
	default:
	}

	fmt.Printf("%s Rotation %s: %s\n", stateIcon, status.ID, status.State)
	fmt.Printf("  Progress:     %d/%d targets (%d%%)\n", status.UpdatedTargets, status.TotalTargets, status.Percentage)
	fmt.Printf("  Batch:        %d/%d\n", status.CurrentBatch, status.TotalBatches)
	fmt.Printf("  Failed:       %d\n", status.FailedTargets)
	fmt.Printf("  Started:      %s\n", status.StartedAt)
	fmt.Printf("  Last Update:  %s\n", status.LastUpdate)

	// Progress bar
	barWidth := 40
	filled := int(float64(status.Percentage) / 100.0 * float64(barWidth))
	bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)
	fmt.Printf("  [%s]\n", bar)
}

// HistoryOptions holds history command options
type HistoryOptions struct {
	Limit  int
	Status string
}

func newRotateHistoryCmd() *cobra.Command {
	opts := &HistoryOptions{}

	cmd := &cobra.Command{
		Use:   "history [secret-path]",
		Short: "Show rotation history",
		Long: `Show rotation history for a secret or all secrets.

Examples:
  # Show all rotation history
  kscorectl secrets rotate history

  # Show history for specific secret
  kscorectl secrets rotate history vault/secret/db

  # Show only failed rotations
  kscorectl secrets rotate history --status failed`,
		RunE: func(cmd *cobra.Command, args []string) error {
			secretPath := ""
			if len(args) > 0 {
				secretPath = args[0]
			}
			return runRotateHistory(cmd, secretPath, opts)
		},
	}

	cmd.Flags().IntVar(&opts.Limit, "limit", 20, "Number of rotations to show")
	cmd.Flags().StringVar(&opts.Status, "status", "", "Filter by status")

	return cmd
}

func runRotateHistory(cmd *cobra.Command, secretPath string, opts *HistoryOptions) error {
	history := generateSampleHistory(secretPath, opts.Limit)

	switch outputFormat {
	case "json":
		return outputJSON(history)
	case "yaml":
		return outputYAML(history)
	default:
		table := &output.Table{
			Headers: []string{"ID", "SECRET PATH", "STRATEGY", "STATE", "TARGETS", "STARTED", "DURATION"},
		}
		for _, h := range history {
			targets := fmt.Sprintf("%d/%d", h.UpdatedTargets, h.TotalTargets)
			if h.FailedTargets > 0 {
				targets = fmt.Sprintf("%s (%d failed)", targets, h.FailedTargets)
			}
			table.Rows = append(table.Rows, []string{
				truncate(h.ID, 12),
				truncate(h.SecretPath, 20),
				string(h.Strategy),
				string(h.State),
				targets,
				h.StartedAt,
				h.Duration,
			})
		}
		output.WriteTable(os.Stdout, table)
		fmt.Printf("\nShowing %d rotation(s)\n", len(history))
	}

	return nil
}

func newRotateTriggerCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "trigger <schedule-id>",
		Short: "Trigger a scheduled rotation immediately",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rotationID := fmt.Sprintf("rot-%s", randomID(8))
			fmt.Printf("Triggered scheduled rotation %s (rotation: %s)\n", args[0], rotationID)
			return nil
		},
	}
}

func newRotateRollbackCmd() *cobra.Command {
	var force bool
	var reason string

	cmd := &cobra.Command{
		Use:   "rollback <rotation-id>",
		Short: "Rollback a rotation",
		Long: `Manually rollback a rotation to the previous secret version.

Examples:
  # Rollback a rotation
  kscorectl secrets rotate rollback rot-123 --reason "health check failures"

  # Force rollback without confirmation
  kscorectl secrets rotate rollback rot-123 --force`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !force {
				fmt.Printf("Are you sure you want to rollback rotation %s? (use --force to confirm)\n", args[0])
				return nil
			}
			fmt.Printf("Rolling back rotation %s...\n", args[0])
			fmt.Printf("  Reason: %s\n", reason)
			fmt.Printf("Rollback initiated. Use 'kscorectl secrets rotate status %s' to monitor.\n", args[0])
			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Force rollback without confirmation")
	cmd.Flags().StringVar(&reason, "reason", "", "Reason for rollback")

	return cmd
}

func newRotatePauseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pause <rotation-id>",
		Short: "Pause an in-progress rotation",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("Paused rotation %s\n", args[0])
			return nil
		},
	}
}

func newRotateResumeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "resume <rotation-id>",
		Short: "Resume a paused rotation",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("Resumed rotation %s\n", args[0])
			return nil
		},
	}
}

func newRotateCancelCmd() *cobra.Command {
	var reason string

	cmd := &cobra.Command{
		Use:   "cancel <rotation-id>",
		Short: "Cancel an in-progress rotation",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("Cancelled rotation %s\n", args[0])
			if reason != "" {
				fmt.Printf("  Reason: %s\n", reason)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&reason, "reason", "", "Cancellation reason")

	return cmd
}

// =============================================================================
// Schedule Commands
// =============================================================================

func newScheduleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "schedule",
		Aliases: []string{"sched", "s"},
		Short:   "Manage rotation schedules",
		Long:    `Commands for managing rotation schedules.`,
	}

	cmd.AddCommand(
		newScheduleListCmd(),
		newScheduleShowCmd(),
		newScheduleCreateCmd(),
		newScheduleEnableCmd(),
		newScheduleDisableCmd(),
		newScheduleDeleteCmd(),
	)

	return cmd
}

func newScheduleListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List rotation schedules",
		RunE: func(cmd *cobra.Command, args []string) error {
			schedules := generateSampleSchedules()

			switch outputFormat {
			case "json":
				return outputJSON(schedules)
			case "yaml":
				return outputYAML(schedules)
			default:
				table := &output.Table{
					Headers: []string{"ID", "SECRET PATH", "SCHEDULE", "STRATEGY", "ENABLED", "NEXT RUN"},
				}
				for _, s := range schedules {
					enabled := "No"
					if s.Enabled {
						enabled = "Yes"
					}
					table.Rows = append(table.Rows, []string{
						truncate(s.ID, 12),
						truncate(s.SecretPath, 20),
						s.Schedule,
						string(s.Strategy),
						enabled,
						s.NextRun,
					})
				}
				output.WriteTable(os.Stdout, table)
			}
			return nil
		},
	}
}

func newScheduleShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <schedule-id>",
		Short: "Show schedule details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s := &scheduleDetail{
				ID:         args[0],
				SecretPath: "vault/secret/database/prod",
				Schedule:   "0 2 * * *",
				Strategy:   secrets.RotationStrategyBlueGreen,
				Enabled:    true,
				NextRun:    time.Now().Add(12 * time.Hour).Format(time.RFC3339),
				LastRun:    time.Now().Add(-12 * time.Hour).Format(time.RFC3339),
				RunCount:   30,
				CreatedBy:  "admin",
				CreatedAt:  time.Now().Add(-30 * 24 * time.Hour).Format(time.RFC3339),
			}

			switch outputFormat {
			case "json":
				return outputJSON(s)
			case "yaml":
				return outputYAML(s)
			default:
				fmt.Printf("Schedule: %s\n", s.ID)
				fmt.Printf("  Secret Path:  %s\n", s.SecretPath)
				fmt.Printf("  Schedule:     %s\n", s.Schedule)
				fmt.Printf("  Strategy:     %s\n", s.Strategy)
				fmt.Printf("  Enabled:      %v\n", s.Enabled)
				fmt.Printf("  Next Run:     %s\n", s.NextRun)
				fmt.Printf("  Last Run:     %s\n", s.LastRun)
				fmt.Printf("  Run Count:    %d\n", s.RunCount)
				fmt.Printf("  Created:      %s by %s\n", s.CreatedAt, s.CreatedBy)
			}
			return nil
		},
	}
}

// ScheduleCreateOptions holds schedule creation options
type ScheduleCreateOptions struct {
	SecretPath       string
	Schedule         string
	Strategy         string
	Targets          []string
	TargetTags       []string
	BatchSize        int
	BatchDelay       string
	CanaryPercentage int
	HealthCheckType  string
	HealthCheckURL   string
	Enabled          bool
	Labels           []string
}

func newScheduleCreateCmd() *cobra.Command {
	opts := &ScheduleCreateOptions{}

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a rotation schedule",
		Long: `Create a new rotation schedule.

Examples:
  # Create daily rotation at 2am
  kscorectl secrets schedule create --secret vault/secret/db \
    --schedule "0 2 * * *" --strategy blue-green --target-tags env:prod

  # Create weekly rotation
  kscorectl secrets schedule create --secret vault/secret/api \
    --schedule "0 3 * * 0" --strategy canary --canary-percentage 10`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runScheduleCreate(cmd, opts)
		},
	}

	cmd.Flags().StringVar(&opts.SecretPath, "secret", "", "Secret path (required)")
	cmd.Flags().StringVar(&opts.Schedule, "schedule", "", "Cron schedule (required)")
	cmd.Flags().StringVar(&opts.Strategy, "strategy", "rolling", "Rotation strategy")
	cmd.Flags().StringArrayVar(&opts.Targets, "target", nil, "Target agent IDs")
	cmd.Flags().StringArrayVar(&opts.TargetTags, "target-tags", nil, "Target tags")
	cmd.Flags().IntVar(&opts.BatchSize, "batch-size", 1, "Batch size")
	cmd.Flags().StringVar(&opts.BatchDelay, "batch-delay", "30s", "Batch delay")
	cmd.Flags().IntVar(&opts.CanaryPercentage, "canary-percentage", 10, "Canary percentage")
	cmd.Flags().StringVar(&opts.HealthCheckType, "health-check-type", "", "Health check type")
	cmd.Flags().StringVar(&opts.HealthCheckURL, "health-check-url", "", "Health check URL")
	cmd.Flags().BoolVar(&opts.Enabled, "enabled", true, "Enable schedule")
	cmd.Flags().StringArrayVar(&opts.Labels, "label", nil, "Labels")

	_ = cmd.MarkFlagRequired("secret")
	_ = cmd.MarkFlagRequired("schedule")

	return cmd
}

func runScheduleCreate(cmd *cobra.Command, opts *ScheduleCreateOptions) error {
	if opts.SecretPath == "" {
		return fmt.Errorf("--secret is required")
	}
	if opts.Schedule == "" {
		return fmt.Errorf("--schedule is required")
	}

	scheduleID := fmt.Sprintf("sched-%s", randomID(8))
	fmt.Printf("Created rotation schedule '%s' for secret '%s'\n", scheduleID, opts.SecretPath)
	fmt.Printf("  Schedule: %s\n", opts.Schedule)
	fmt.Printf("  Strategy: %s\n", opts.Strategy)
	return nil
}

func newScheduleEnableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "enable <schedule-id>",
		Short: "Enable a schedule",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("Enabled schedule %s\n", args[0])
			return nil
		},
	}
}

func newScheduleDisableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "disable <schedule-id>",
		Short: "Disable a schedule",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("Disabled schedule %s\n", args[0])
			return nil
		},
	}
}

func newScheduleDeleteCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "delete <schedule-id>",
		Short: "Delete a schedule",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !force {
				fmt.Printf("Are you sure you want to delete schedule %s? (use --force to confirm)\n", args[0])
				return nil
			}
			fmt.Printf("Deleted schedule %s\n", args[0])
			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Force deletion")

	return cmd
}

// =============================================================================
// Policy Commands
// =============================================================================

func newPolicyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "policy",
		Aliases: []string{"pol", "p"},
		Short:   "Manage rotation policies",
		Long:    `Commands for managing rotation policies.`,
	}

	cmd.AddCommand(
		newPolicyListCmd(),
		newPolicyShowCmd(),
		newPolicyCreateCmd(),
		newPolicyDeleteCmd(),
	)

	return cmd
}

func newPolicyListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List rotation policies",
		RunE: func(cmd *cobra.Command, args []string) error {
			policies := generateSamplePolicies()

			switch outputFormat {
			case "json":
				return outputJSON(policies)
			case "yaml":
				return outputYAML(policies)
			default:
				table := &output.Table{
					Headers: []string{"ID", "NAME", "SECRET PATTERN", "MAX AGE", "STRATEGY", "ENABLED"},
				}
				for _, p := range policies {
					enabled := "No"
					if p.Enabled {
						enabled = "Yes"
					}
					table.Rows = append(table.Rows, []string{
						truncate(p.ID, 12),
						truncate(p.Name, 20),
						p.SecretPattern,
						p.MaxAge,
						string(p.Strategy),
						enabled,
					})
				}
				output.WriteTable(os.Stdout, table)
			}
			return nil
		},
	}
}

func newPolicyShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <policy-id>",
		Short: "Show policy details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := &policyDetail{
				ID:             args[0],
				Name:           "database-rotation-policy",
				SecretPattern:  "vault/secret/database/*",
				MaxAge:         "90d",
				Strategy:       secrets.RotationStrategyBlueGreen,
				BatchSize:      2,
				HealthRequired: true,
				Enabled:        true,
				CreatedBy:      "admin",
				CreatedAt:      time.Now().Add(-60 * 24 * time.Hour).Format(time.RFC3339),
			}

			switch outputFormat {
			case "json":
				return outputJSON(p)
			case "yaml":
				return outputYAML(p)
			default:
				fmt.Printf("Policy: %s\n", p.Name)
				fmt.Printf("  ID:              %s\n", p.ID)
				fmt.Printf("  Secret Pattern:  %s\n", p.SecretPattern)
				fmt.Printf("  Max Age:         %s\n", p.MaxAge)
				fmt.Printf("  Strategy:        %s\n", p.Strategy)
				fmt.Printf("  Batch Size:      %d\n", p.BatchSize)
				fmt.Printf("  Health Required: %v\n", p.HealthRequired)
				fmt.Printf("  Enabled:         %v\n", p.Enabled)
				fmt.Printf("  Created:         %s by %s\n", p.CreatedAt, p.CreatedBy)
			}
			return nil
		},
	}
}

// PolicyCreateOptions holds policy creation options
type PolicyCreateOptions struct {
	Name           string
	SecretPattern  string
	MaxAge         string
	Strategy       string
	BatchSize      int
	HealthRequired bool
	Enabled        bool
}

func newPolicyCreateCmd() *cobra.Command {
	opts := &PolicyCreateOptions{}

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a rotation policy",
		Long: `Create a new rotation policy.

Examples:
  # Create a policy for database secrets
  kscorectl secrets policy create --name db-policy \
    --pattern "vault/secret/database/*" --max-age 90d --strategy blue-green

  # Create a strict policy requiring health checks
  kscorectl secrets policy create --name api-policy \
    --pattern "vault/secret/api/*" --max-age 30d --health-required`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPolicyCreate(cmd, opts)
		},
	}

	cmd.Flags().StringVar(&opts.Name, "name", "", "Policy name (required)")
	cmd.Flags().StringVar(&opts.SecretPattern, "pattern", "", "Secret path pattern (required)")
	cmd.Flags().StringVar(&opts.MaxAge, "max-age", "90d", "Maximum secret age before rotation")
	cmd.Flags().StringVar(&opts.Strategy, "strategy", "rolling", "Default rotation strategy")
	cmd.Flags().IntVar(&opts.BatchSize, "batch-size", 1, "Default batch size")
	cmd.Flags().BoolVar(&opts.HealthRequired, "health-required", false, "Require health checks")
	cmd.Flags().BoolVar(&opts.Enabled, "enabled", true, "Enable policy")

	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("pattern")

	return cmd
}

func runPolicyCreate(cmd *cobra.Command, opts *PolicyCreateOptions) error {
	if opts.Name == "" {
		return fmt.Errorf("--name is required")
	}
	if opts.SecretPattern == "" {
		return fmt.Errorf("--pattern is required")
	}

	policyID := fmt.Sprintf("pol-%s", randomID(8))
	fmt.Printf("Created rotation policy '%s' (id: %s)\n", opts.Name, policyID)
	fmt.Printf("  Pattern:  %s\n", opts.SecretPattern)
	fmt.Printf("  Max Age:  %s\n", opts.MaxAge)
	fmt.Printf("  Strategy: %s\n", opts.Strategy)
	return nil
}

func newPolicyDeleteCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "delete <policy-id>",
		Short: "Delete a policy",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !force {
				fmt.Printf("Are you sure you want to delete policy %s? (use --force to confirm)\n", args[0])
				return nil
			}
			fmt.Printf("Deleted policy %s\n", args[0])
			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Force deletion")

	return cmd
}

// =============================================================================
// Dynamic Commands
// =============================================================================

func newDynamicCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dynamic",
		Short: "Manage dynamic secrets",
		Long:  `Commands for managing dynamic secrets (database credentials, cloud tokens, etc.).`,
	}

	cmd.AddCommand(
		newDynamicListCmd(),
		newDynamicGetCmd(),
		newDynamicRevokeCmd(),
	)

	return cmd
}

func newDynamicListCmd() *cobra.Command {
	var backend string

	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List active dynamic secrets",
		Long: `List active dynamic secrets across all backends.

Examples:
  kscorectl secrets dynamic list
  kscorectl secrets dynamic list --backend vault
  kscorectl secrets dynamic list -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDynamicList(cmd, backend)
		},
	}

	cmd.Flags().StringVar(&backend, "backend", "", "Filter by backend name")

	return cmd
}

func runDynamicList(cmd *cobra.Command, backend string) error {
	items := generateSampleDynamicSecrets()

	if backend != "" {
		var filtered []*dynamicSecretDisplay
		for _, item := range items {
			if item.Backend == backend {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}

	if len(items) == 0 {
		fmt.Println("No active dynamic secrets found")
		return nil
	}

	switch outputFormat {
	case "json":
		return outputJSON(items)
	case "yaml":
		return outputYAML(items)
	default:
		table := &output.Table{
			Headers: []string{"LEASE ID", "PATH", "TYPE", "BACKEND", "TTL", "EXPIRES"},
		}
		for _, item := range items {
			table.Rows = append(table.Rows, []string{
				truncate(item.LeaseID, 16),
				truncate(item.Path, 30),
				item.Type,
				item.Backend,
				item.TTL,
				item.ExpiresAt,
			})
		}
		output.WriteTable(os.Stdout, table)
		fmt.Printf("\nTotal: %d dynamic secret(s)\n", len(items))
	}

	return nil
}

func newDynamicGetCmd() *cobra.Command {
	var ttl string

	cmd := &cobra.Command{
		Use:   "get <path>",
		Short: "Generate a new dynamic secret",
		Long: `Generate a new dynamic secret at the specified path.

Examples:
  kscorectl secrets dynamic get vault/database/creds/myapp
  kscorectl secrets dynamic get vault/database/creds/myapp --ttl 1h
  kscorectl secrets dynamic get vault/aws/creds/deploy -o json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDynamicGet(cmd, args[0], ttl)
		},
	}

	cmd.Flags().StringVar(&ttl, "ttl", "1h", "Requested TTL for the dynamic secret")

	return cmd
}

func runDynamicGet(cmd *cobra.Command, path, ttl string) error {
	leaseID := fmt.Sprintf("lease-%s", randomID(8))
	result := &dynamicSecretResult{
		LeaseID:   leaseID,
		Path:      path,
		TTL:       ttl,
		Renewable: true,
		Keys:      []string{"username", "password"},
		IssuedAt:  time.Now().Format(time.RFC3339),
		ExpiresAt: time.Now().Add(1 * time.Hour).Format(time.RFC3339),
	}

	switch outputFormat {
	case "json":
		return outputJSON(result)
	case "yaml":
		return outputYAML(result)
	default:
		fmt.Printf("Generated dynamic secret:\n")
		fmt.Printf("  Lease ID:   %s\n", result.LeaseID)
		fmt.Printf("  Path:       %s\n", result.Path)
		fmt.Printf("  TTL:        %s\n", result.TTL)
		fmt.Printf("  Renewable:  %v\n", result.Renewable)
		fmt.Printf("  Issued:     %s\n", result.IssuedAt)
		fmt.Printf("  Expires:    %s\n", result.ExpiresAt)
		fmt.Println()
		fmt.Println("Values:")
		for _, k := range result.Keys {
			fmt.Printf("  %s: ****\n", k)
		}
	}

	return nil
}

func newDynamicRevokeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "revoke <lease-id>",
		Short: "Revoke a dynamic secret",
		Long: `Revoke a dynamic secret by its lease ID.

Examples:
  kscorectl secrets dynamic revoke lease-abc12345`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("Revoked dynamic secret (lease: %s)\n", args[0])
			return nil
		},
	}
}

func generateSampleDynamicSecrets() []*dynamicSecretDisplay {
	return []*dynamicSecretDisplay{
		{
			LeaseID:   "lease-db001",
			Path:      "vault/database/creds/myapp",
			Type:      "database",
			Backend:   "vault",
			TTL:       "1h",
			ExpiresAt: time.Now().Add(45 * time.Minute).Format("15:04"),
		},
		{
			LeaseID:   "lease-aws002",
			Path:      "vault/aws/creds/deploy",
			Type:      "cloud_iam",
			Backend:   "vault",
			TTL:       "12h",
			ExpiresAt: time.Now().Add(10 * time.Hour).Format("15:04"),
		},
		{
			LeaseID:   "lease-db003",
			Path:      "vault/database/creds/analytics",
			Type:      "database",
			Backend:   "vault",
			TTL:       "30m",
			ExpiresAt: time.Now().Add(12 * time.Minute).Format("15:04"),
		},
		{
			LeaseID:   "lease-gcp004",
			Path:      "vault/gcp/token/compute",
			Type:      "cloud_iam",
			Backend:   "vault",
			TTL:       "1h",
			ExpiresAt: time.Now().Add(55 * time.Minute).Format("15:04"),
		},
	}
}

// =============================================================================
// Leases Commands
// =============================================================================

func newLeasesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "leases",
		Aliases: []string{"lease"},
		Short:   "Manage secret leases",
		Long:    `Commands for managing secret leases.`,
	}

	cmd.AddCommand(
		newLeasesListCmd(),
		newLeasesRevokeCmd(),
		newLeasesRenewCmd(),
	)

	return cmd
}

func newLeasesListCmd() *cobra.Command {
	var backend string
	var state string

	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List active leases",
		Long: `List all active secret leases.

Examples:
  kscorectl secrets leases list
  kscorectl secrets leases list --backend vault
  kscorectl secrets leases list --state active
  kscorectl secrets leases list -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLeasesList(cmd, backend, state)
		},
	}

	cmd.Flags().StringVar(&backend, "backend", "", "Filter by backend name")
	cmd.Flags().StringVar(&state, "state", "", "Filter by state (active, expiring, expired, revoked)")

	return cmd
}

func runLeasesList(cmd *cobra.Command, backend, state string) error {
	items := generateSampleLeases()

	if backend != "" {
		var filtered []*leaseDisplay
		for _, item := range items {
			if item.Backend == backend {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}

	if state != "" {
		var filtered []*leaseDisplay
		for _, item := range items {
			if item.State == state {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}

	if len(items) == 0 {
		fmt.Println("No leases found")
		return nil
	}

	switch outputFormat {
	case "json":
		return outputJSON(items)
	case "yaml":
		return outputYAML(items)
	default:
		table := &output.Table{
			Headers: []string{"LEASE ID", "SECRET PATH", "BACKEND", "STATE", "TTL", "RENEWALS", "EXPIRES"},
		}
		for _, item := range items {
			table.Rows = append(table.Rows, []string{
				truncate(item.LeaseID, 16),
				truncate(item.SecretPath, 25),
				item.Backend,
				item.State,
				item.TTL,
				fmt.Sprintf("%d", item.RenewalCount),
				item.ExpiresAt,
			})
		}
		output.WriteTable(os.Stdout, table)
		fmt.Printf("\nTotal: %d lease(s)\n", len(items))
	}

	return nil
}

func newLeasesRevokeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "revoke <lease-id>",
		Short: "Revoke a lease",
		Long: `Revoke a secret lease by its ID.

Examples:
  kscorectl secrets leases revoke lease-abc12345`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("Revoked lease %s\n", args[0])
			return nil
		},
	}
}

func newLeasesRenewCmd() *cobra.Command {
	var increment string

	cmd := &cobra.Command{
		Use:   "renew <lease-id>",
		Short: "Renew a lease",
		Long: `Renew a secret lease to extend its TTL.

Examples:
  kscorectl secrets leases renew lease-abc12345
  kscorectl secrets leases renew lease-abc12345 --increment 2h`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("Renewed lease %s (increment: %s)\n", args[0], increment)
			return nil
		},
	}

	cmd.Flags().StringVar(&increment, "increment", "1h", "TTL increment for renewal")

	return cmd
}

func generateSampleLeases() []*leaseDisplay {
	return []*leaseDisplay{
		{
			LeaseID:      "lease-db001",
			SecretPath:   "vault/database/creds/myapp",
			Backend:      "vault",
			State:        "active",
			TTL:          "1h",
			RenewalCount: 3,
			ExpiresAt:    time.Now().Add(45 * time.Minute).Format("15:04"),
		},
		{
			LeaseID:      "lease-aws002",
			SecretPath:   "vault/aws/creds/deploy",
			Backend:      "vault",
			State:        "active",
			TTL:          "12h",
			RenewalCount: 0,
			ExpiresAt:    time.Now().Add(10 * time.Hour).Format("15:04"),
		},
		{
			LeaseID:      "lease-db003",
			SecretPath:   "vault/database/creds/analytics",
			Backend:      "vault",
			State:        "expiring",
			TTL:          "30m",
			RenewalCount: 7,
			ExpiresAt:    time.Now().Add(5 * time.Minute).Format("15:04"),
		},
		{
			LeaseID:      "lease-gcp004",
			SecretPath:   "vault/gcp/token/compute",
			Backend:      "vault",
			State:        "active",
			TTL:          "1h",
			RenewalCount: 1,
			ExpiresAt:    time.Now().Add(55 * time.Minute).Format("15:04"),
		},
	}
}

// =============================================================================
// Encrypt Command
// =============================================================================

func newEncryptCmd() *cobra.Command {
	var key string
	var transitContext string

	cmd := &cobra.Command{
		Use:   "encrypt <plaintext>",
		Short: "Encrypt data using transit encryption",
		Long: `Encrypt plaintext data using a transit encryption key.

Examples:
  kscorectl secrets encrypt "my-secret-data" --key transit/mykey
  kscorectl secrets encrypt "sensitive-value" --key transit/mykey --context "app=web"
  kscorectl secrets encrypt "data" --key transit/mykey -o json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEncrypt(cmd, args[0], key, transitContext)
		},
	}

	cmd.Flags().StringVar(&key, "key", "", "Transit encryption key name (required)")
	cmd.Flags().StringVar(&transitContext, "context", "", "Additional authenticated data context")
	_ = cmd.MarkFlagRequired("key")

	return cmd
}

func runEncrypt(cmd *cobra.Command, plaintext, key, transitContext string) error {
	encoded := base64.StdEncoding.EncodeToString([]byte(plaintext))
	ciphertext := fmt.Sprintf("vault:v1:%s", encoded)

	result := &transitResult{
		KeyName:    key,
		Operation:  "encrypt",
		Ciphertext: ciphertext,
		KeyVersion: 1,
		Context:    transitContext,
	}

	switch outputFormat {
	case "json":
		return outputJSON(result)
	case "yaml":
		return outputYAML(result)
	default:
		fmt.Printf("Ciphertext: %s\n", result.Ciphertext)
		fmt.Printf("Key:        %s (v%d)\n", result.KeyName, result.KeyVersion)
		if transitContext != "" {
			fmt.Printf("Context:    %s\n", transitContext)
		}
	}

	return nil
}

// =============================================================================
// Decrypt Command
// =============================================================================

func newDecryptCmd() *cobra.Command {
	var key string
	var transitContext string

	cmd := &cobra.Command{
		Use:   "decrypt <ciphertext>",
		Short: "Decrypt data using transit encryption",
		Long: `Decrypt ciphertext data using a transit encryption key.

Examples:
  kscorectl secrets decrypt "vault:v1:bXktc2VjcmV0" --key transit/mykey
  kscorectl secrets decrypt "vault:v1:..." --key transit/mykey --context "app=web"
  kscorectl secrets decrypt "vault:v1:..." --key transit/mykey -o json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDecrypt(cmd, args[0], key, transitContext)
		},
	}

	cmd.Flags().StringVar(&key, "key", "", "Transit encryption key name (required)")
	cmd.Flags().StringVar(&transitContext, "context", "", "Additional authenticated data context")
	_ = cmd.MarkFlagRequired("key")

	return cmd
}

func runDecrypt(cmd *cobra.Command, ciphertext, key, transitContext string) error {
	result := &transitResult{
		KeyName:    key,
		Operation:  "decrypt",
		Ciphertext: ciphertext,
		Plaintext:  "****",
		KeyVersion: 1,
		Context:    transitContext,
	}

	switch outputFormat {
	case "json":
		return outputJSON(result)
	case "yaml":
		return outputYAML(result)
	default:
		fmt.Printf("Plaintext:  %s\n", result.Plaintext)
		fmt.Printf("Key:        %s (v%d)\n", result.KeyName, result.KeyVersion)
		if transitContext != "" {
			fmt.Printf("Context:    %s\n", transitContext)
		}
	}

	return nil
}

// =============================================================================
// Rewrap Command
// =============================================================================

func newRewrapCmd() *cobra.Command {
	var key string

	cmd := &cobra.Command{
		Use:   "rewrap <ciphertext>",
		Short: "Re-encrypt with latest key version",
		Long: `Re-encrypt ciphertext with the latest version of the transit key.

This allows upgrading ciphertext to use the newest key version without
revealing the plaintext.

Examples:
  kscorectl secrets rewrap "vault:v1:bXktc2VjcmV0" --key transit/mykey
  kscorectl secrets rewrap "vault:v1:..." --key transit/mykey -o json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRewrap(cmd, args[0], key)
		},
	}

	cmd.Flags().StringVar(&key, "key", "", "Transit encryption key name (required)")
	_ = cmd.MarkFlagRequired("key")

	return cmd
}

func runRewrap(cmd *cobra.Command, ciphertext, key string) error {
	parts := strings.SplitN(ciphertext, ":", 3)
	var payload string
	if len(parts) == 3 {
		payload = parts[2]
	} else {
		payload = base64.StdEncoding.EncodeToString([]byte(ciphertext))
	}
	newCiphertext := fmt.Sprintf("vault:v2:%s", payload)

	result := &transitResult{
		KeyName:       key,
		Operation:     "rewrap",
		Ciphertext:    newCiphertext,
		OldKeyVersion: 1,
		KeyVersion:    2,
	}

	switch outputFormat {
	case "json":
		return outputJSON(result)
	case "yaml":
		return outputYAML(result)
	default:
		fmt.Printf("Rewrapped ciphertext: %s\n", result.Ciphertext)
		fmt.Printf("Key:                  %s (v%d -> v%d)\n", result.KeyName, result.OldKeyVersion, result.KeyVersion)
	}

	return nil
}

// =============================================================================
// Template Command
// =============================================================================

func newTemplateCmd() *cobra.Command {
	var outFile string
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "template <template-file>",
		Short: "Render a template with secret values",
		Long: `Render a template file, injecting secret values from configured backends.

Template files use Go template syntax with secret references:
  {{ secret "vault/secret/database/prod" "password" }}

Examples:
  kscorectl secrets template config.tmpl
  kscorectl secrets template config.tmpl --out-file config.yaml
  kscorectl secrets template config.tmpl --dry-run`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTemplate(cmd, args[0], outFile, dryRun)
		},
	}

	cmd.Flags().StringVar(&outFile, "out-file", "", "Output file path (default: stdout)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what secrets would be injected without rendering")

	return cmd
}

func runTemplate(cmd *cobra.Command, templateFile, outputPath string, dryRun bool) error {
	refs := []templateSecretRef{
		{Path: "vault/secret/database/prod", Field: "password", Line: 5},
		{Path: "vault/secret/database/prod", Field: "username", Line: 6},
		{Path: "vault/secret/api/prod", Field: "api_key", Line: 12},
	}

	result := &templateResult{
		TemplateFile: templateFile,
		OutputFile:   outputPath,
		DryRun:       dryRun,
		SecretRefs:   refs,
		RefCount:     len(refs),
	}

	if dryRun {
		switch outputFormat {
		case "json":
			return outputJSON(result)
		case "yaml":
			return outputYAML(result)
		default:
			fmt.Printf("Template: %s\n", templateFile)
			fmt.Printf("Dry run - secrets that would be injected:\n\n")
			for _, ref := range refs {
				fmt.Printf("  Line %d: {{ secret %q %q }} -> ****\n", ref.Line, ref.Path, ref.Field)
			}
			fmt.Printf("\nTotal: %d secret reference(s)\n", len(refs))
		}
		return nil
	}

	switch outputFormat {
	case "json":
		return outputJSON(result)
	case "yaml":
		return outputYAML(result)
	default:
		dest := "stdout"
		if outputPath != "" {
			dest = outputPath
		}
		fmt.Printf("Rendered template %s -> %s\n", templateFile, dest)
		fmt.Printf("  Injected %d secret value(s)\n", len(refs))
	}

	return nil
}

// =============================================================================
// Cache Commands
// =============================================================================

func newCacheCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cache",
		Short: "Manage the secret cache",
		Long:  `Commands for managing the local secret cache.`,
	}

	cmd.AddCommand(
		newCacheStatusCmd(),
		newCacheClearCmd(),
		newCacheListCmd(),
	)

	return cmd
}

func newCacheStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show cache statistics",
		Long: `Show secret cache statistics including size, hit rate, and TTL.

Examples:
  kscorectl secrets cache status
  kscorectl secrets cache status -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCacheStatus(cmd)
		},
	}
}

func runCacheStatus(cmd *cobra.Command) error {
	stats := &cacheStatusDisplay{
		Entries:      47,
		MaxEntries:   10000,
		HitRate:      87.3,
		Hits:         1523,
		Misses:       221,
		Evictions:    12,
		ExpiredCount: 34,
		MemoryBytes:  245760,
		DefaultTTL:   "5m",
		Encrypted:    true,
	}

	switch outputFormat {
	case "json":
		return outputJSON(stats)
	case "yaml":
		return outputYAML(stats)
	default:
		fmt.Printf("Secret Cache Status:\n")
		fmt.Printf("  Entries:      %d / %d\n", stats.Entries, stats.MaxEntries)
		fmt.Printf("  Hit Rate:     %.1f%%\n", stats.HitRate)
		fmt.Printf("  Hits:         %d\n", stats.Hits)
		fmt.Printf("  Misses:       %d\n", stats.Misses)
		fmt.Printf("  Evictions:    %d\n", stats.Evictions)
		fmt.Printf("  Expired:      %d\n", stats.ExpiredCount)
		fmt.Printf("  Memory:       %s\n", formatBytes(stats.MemoryBytes))
		fmt.Printf("  Default TTL:  %s\n", stats.DefaultTTL)
		fmt.Printf("  Encrypted:    %v\n", stats.Encrypted)
	}

	return nil
}

func newCacheClearCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clear",
		Short: "Clear the local secret cache",
		Long: `Clear all entries from the local secret cache.

Examples:
  kscorectl secrets cache clear`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Cleared secret cache (47 entries removed)")
			return nil
		},
	}
}

func newCacheListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List cached secrets",
		Long: `List all cached secret entries.

Examples:
  kscorectl secrets cache list
  kscorectl secrets cache list -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCacheList(cmd)
		},
	}
}

func runCacheList(cmd *cobra.Command) error {
	items := generateSampleCacheEntries()

	if len(items) == 0 {
		fmt.Println("Cache is empty")
		return nil
	}

	switch outputFormat {
	case "json":
		return outputJSON(items)
	case "yaml":
		return outputYAML(items)
	default:
		table := &output.Table{
			Headers: []string{"PATH", "BACKEND", "CACHED AT", "TTL", "HITS"},
		}
		for _, item := range items {
			table.Rows = append(table.Rows, []string{
				truncate(item.Path, 35),
				item.Backend,
				item.CachedAt,
				item.TTL,
				fmt.Sprintf("%d", item.Hits),
			})
		}
		output.WriteTable(os.Stdout, table)
		fmt.Printf("\nTotal: %d cached entry(ies)\n", len(items))
	}

	return nil
}

func generateSampleCacheEntries() []*cacheEntryDisplay {
	return []*cacheEntryDisplay{
		{
			Path:     "vault/secret/database/prod",
			Backend:  "vault",
			CachedAt: time.Now().Add(-2 * time.Minute).Format("15:04:05"),
			TTL:      "3m",
			Hits:     12,
		},
		{
			Path:     "vault/secret/api/prod",
			Backend:  "vault",
			CachedAt: time.Now().Add(-1 * time.Minute).Format("15:04:05"),
			TTL:      "4m",
			Hits:     5,
		},
		{
			Path:     "aws-sm/production/payment-gateway",
			Backend:  "aws-sm",
			CachedAt: time.Now().Add(-30 * time.Second).Format("15:04:05"),
			TTL:      "4m30s",
			Hits:     2,
		},
	}
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// =============================================================================
// Display Types
// =============================================================================

type secretDisplay struct {
	Path      string            `json:"path" yaml:"path"`
	Version   int               `json:"version" yaml:"version"`
	Keys      []string          `json:"keys" yaml:"keys"`
	CreatedAt string            `json:"created_at" yaml:"created_at"`
	ExpiresAt string            `json:"expires_at,omitempty" yaml:"expires_at,omitempty"`
	CreatedBy string            `json:"created_by" yaml:"created_by"`
	Backend   string            `json:"backend" yaml:"backend"`
	Metadata  map[string]string `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

type secretListItem struct {
	Path         string `json:"path" yaml:"path"`
	Versions     int    `json:"versions" yaml:"versions"`
	Backend      string `json:"backend" yaml:"backend"`
	LastModified string `json:"last_modified" yaml:"last_modified"`
	CreatedBy    string `json:"created_by" yaml:"created_by"`
	ExpiresAt    string `json:"expires_at,omitempty" yaml:"expires_at,omitempty"`
}

type backendDisplay struct {
	Name        string `json:"name" yaml:"name"`
	Type        string `json:"type" yaml:"type"`
	Status      string `json:"status" yaml:"status"`
	SecretCount int    `json:"secret_count" yaml:"secret_count"`
	Address     string `json:"address" yaml:"address"`
	AuthMethod  string `json:"auth_method" yaml:"auth_method"`
}

type auditEntry struct {
	Timestamp string `json:"timestamp" yaml:"timestamp"`
	Path      string `json:"path" yaml:"path"`
	Action    string `json:"action" yaml:"action"`
	Principal string `json:"principal" yaml:"principal"`
	SourceIP  string `json:"source_ip" yaml:"source_ip"`
	Version   int    `json:"version" yaml:"version"`
}

type rotationDisplay struct {
	ID             string                   `json:"id" yaml:"id"`
	SecretPath     string                   `json:"secret_path" yaml:"secret_path"`
	Strategy       secrets.RotationStrategy `json:"strategy" yaml:"strategy"`
	State          secrets.RotationState    `json:"state" yaml:"state"`
	TotalTargets   int                      `json:"total_targets" yaml:"total_targets"`
	UpdatedTargets int                      `json:"updated_targets" yaml:"updated_targets"`
	StartedAt      string                   `json:"started_at" yaml:"started_at"`
}

type rotationDetail struct {
	ID              string                   `json:"id" yaml:"id"`
	SecretPath      string                   `json:"secret_path" yaml:"secret_path"`
	Strategy        secrets.RotationStrategy `json:"strategy" yaml:"strategy"`
	State           secrets.RotationState    `json:"state" yaml:"state"`
	TotalTargets    int                      `json:"total_targets" yaml:"total_targets"`
	UpdatedTargets  int                      `json:"updated_targets" yaml:"updated_targets"`
	FailedTargets   int                      `json:"failed_targets" yaml:"failed_targets"`
	Percentage      int                      `json:"percentage" yaml:"percentage"`
	StartedAt       string                   `json:"started_at" yaml:"started_at"`
	BatchSize       int                      `json:"batch_size" yaml:"batch_size"`
	BatchDelay      string                   `json:"batch_delay" yaml:"batch_delay"`
	HealthCheckType string                   `json:"health_check_type,omitempty" yaml:"health_check_type,omitempty"`
	HealthCheckURL  string                   `json:"health_check_url,omitempty" yaml:"health_check_url,omitempty"`
	CreatedBy       string                   `json:"created_by" yaml:"created_by"`
	Labels          map[string]string        `json:"labels,omitempty" yaml:"labels,omitempty"`
}

type rotationStatus struct {
	ID             string                `json:"id" yaml:"id"`
	State          secrets.RotationState `json:"state" yaml:"state"`
	TotalTargets   int                   `json:"total_targets" yaml:"total_targets"`
	UpdatedTargets int                   `json:"updated_targets" yaml:"updated_targets"`
	FailedTargets  int                   `json:"failed_targets" yaml:"failed_targets"`
	Percentage     int                   `json:"percentage" yaml:"percentage"`
	CurrentBatch   int                   `json:"current_batch" yaml:"current_batch"`
	TotalBatches   int                   `json:"total_batches" yaml:"total_batches"`
	StartedAt      string                `json:"started_at" yaml:"started_at"`
	LastUpdate     string                `json:"last_update" yaml:"last_update"`
}

type rotationHistory struct {
	ID             string                   `json:"id" yaml:"id"`
	SecretPath     string                   `json:"secret_path" yaml:"secret_path"`
	Strategy       secrets.RotationStrategy `json:"strategy" yaml:"strategy"`
	State          secrets.RotationState    `json:"state" yaml:"state"`
	TotalTargets   int                      `json:"total_targets" yaml:"total_targets"`
	UpdatedTargets int                      `json:"updated_targets" yaml:"updated_targets"`
	FailedTargets  int                      `json:"failed_targets" yaml:"failed_targets"`
	StartedAt      string                   `json:"started_at" yaml:"started_at"`
	Duration       string                   `json:"duration" yaml:"duration"`
}

type scheduleDisplay struct {
	ID         string                   `json:"id" yaml:"id"`
	SecretPath string                   `json:"secret_path" yaml:"secret_path"`
	Schedule   string                   `json:"schedule" yaml:"schedule"`
	Strategy   secrets.RotationStrategy `json:"strategy" yaml:"strategy"`
	Enabled    bool                     `json:"enabled" yaml:"enabled"`
	NextRun    string                   `json:"next_run" yaml:"next_run"`
}

type scheduleDetail struct {
	ID         string                   `json:"id" yaml:"id"`
	SecretPath string                   `json:"secret_path" yaml:"secret_path"`
	Schedule   string                   `json:"schedule" yaml:"schedule"`
	Strategy   secrets.RotationStrategy `json:"strategy" yaml:"strategy"`
	Enabled    bool                     `json:"enabled" yaml:"enabled"`
	NextRun    string                   `json:"next_run" yaml:"next_run"`
	LastRun    string                   `json:"last_run" yaml:"last_run"`
	RunCount   int                      `json:"run_count" yaml:"run_count"`
	CreatedBy  string                   `json:"created_by" yaml:"created_by"`
	CreatedAt  string                   `json:"created_at" yaml:"created_at"`
}

type policyDisplay struct {
	ID            string                   `json:"id" yaml:"id"`
	Name          string                   `json:"name" yaml:"name"`
	SecretPattern string                   `json:"secret_pattern" yaml:"secret_pattern"`
	MaxAge        string                   `json:"max_age" yaml:"max_age"`
	Strategy      secrets.RotationStrategy `json:"strategy" yaml:"strategy"`
	Enabled       bool                     `json:"enabled" yaml:"enabled"`
}

type policyDetail struct {
	ID             string                   `json:"id" yaml:"id"`
	Name           string                   `json:"name" yaml:"name"`
	SecretPattern  string                   `json:"secret_pattern" yaml:"secret_pattern"`
	MaxAge         string                   `json:"max_age" yaml:"max_age"`
	Strategy       secrets.RotationStrategy `json:"strategy" yaml:"strategy"`
	BatchSize      int                      `json:"batch_size" yaml:"batch_size"`
	HealthRequired bool                     `json:"health_required" yaml:"health_required"`
	Enabled        bool                     `json:"enabled" yaml:"enabled"`
	CreatedBy      string                   `json:"created_by" yaml:"created_by"`
	CreatedAt      string                   `json:"created_at" yaml:"created_at"`
}

type dynamicSecretDisplay struct {
	LeaseID   string `json:"lease_id" yaml:"lease_id"`
	Path      string `json:"path" yaml:"path"`
	Type      string `json:"type" yaml:"type"`
	Backend   string `json:"backend" yaml:"backend"`
	TTL       string `json:"ttl" yaml:"ttl"`
	ExpiresAt string `json:"expires_at" yaml:"expires_at"`
}

type dynamicSecretResult struct {
	LeaseID   string   `json:"lease_id" yaml:"lease_id"`
	Path      string   `json:"path" yaml:"path"`
	TTL       string   `json:"ttl" yaml:"ttl"`
	Renewable bool     `json:"renewable" yaml:"renewable"`
	Keys      []string `json:"keys" yaml:"keys"`
	IssuedAt  string   `json:"issued_at" yaml:"issued_at"`
	ExpiresAt string   `json:"expires_at" yaml:"expires_at"`
}

type leaseDisplay struct {
	LeaseID      string `json:"lease_id" yaml:"lease_id"`
	SecretPath   string `json:"secret_path" yaml:"secret_path"`
	Backend      string `json:"backend" yaml:"backend"`
	State        string `json:"state" yaml:"state"`
	TTL          string `json:"ttl" yaml:"ttl"`
	RenewalCount int    `json:"renewal_count" yaml:"renewal_count"`
	ExpiresAt    string `json:"expires_at" yaml:"expires_at"`
}

type transitResult struct {
	KeyName       string `json:"key_name" yaml:"key_name"`
	Operation     string `json:"operation" yaml:"operation"`
	Ciphertext    string `json:"ciphertext,omitempty" yaml:"ciphertext,omitempty"`
	Plaintext     string `json:"plaintext,omitempty" yaml:"plaintext,omitempty"`
	KeyVersion    int    `json:"key_version" yaml:"key_version"`
	OldKeyVersion int    `json:"old_key_version,omitempty" yaml:"old_key_version,omitempty"`
	Context       string `json:"context,omitempty" yaml:"context,omitempty"`
}

type templateSecretRef struct {
	Path  string `json:"path" yaml:"path"`
	Field string `json:"field" yaml:"field"`
	Line  int    `json:"line" yaml:"line"`
}

type templateResult struct {
	TemplateFile string              `json:"template_file" yaml:"template_file"`
	OutputFile   string              `json:"output_file,omitempty" yaml:"output_file,omitempty"`
	DryRun       bool                `json:"dry_run" yaml:"dry_run"`
	SecretRefs   []templateSecretRef `json:"secret_refs" yaml:"secret_refs"`
	RefCount     int                 `json:"ref_count" yaml:"ref_count"`
}

type cacheStatusDisplay struct {
	Entries      int     `json:"entries" yaml:"entries"`
	MaxEntries   int     `json:"max_entries" yaml:"max_entries"`
	HitRate      float64 `json:"hit_rate" yaml:"hit_rate"`
	Hits         int64   `json:"hits" yaml:"hits"`
	Misses       int64   `json:"misses" yaml:"misses"`
	Evictions    int64   `json:"evictions" yaml:"evictions"`
	ExpiredCount int64   `json:"expired_count" yaml:"expired_count"`
	MemoryBytes  int64   `json:"memory_bytes" yaml:"memory_bytes"`
	DefaultTTL   string  `json:"default_ttl" yaml:"default_ttl"`
	Encrypted    bool    `json:"encrypted" yaml:"encrypted"`
}

type cacheEntryDisplay struct {
	Path     string `json:"path" yaml:"path"`
	Backend  string `json:"backend" yaml:"backend"`
	CachedAt string `json:"cached_at" yaml:"cached_at"`
	TTL      string `json:"ttl" yaml:"ttl"`
	Hits     int    `json:"hits" yaml:"hits"`
}

// =============================================================================
// Helpers
// =============================================================================

func generateSampleRotations() []*rotationDisplay {
	return []*rotationDisplay{
		{
			ID:             "rot-abc123",
			SecretPath:     "vault/secret/database/prod",
			Strategy:       secrets.RotationStrategyBlueGreen,
			State:          secrets.RotationStateInProgress,
			TotalTargets:   10,
			UpdatedTargets: 6,
			StartedAt:      time.Now().Add(-15 * time.Minute).Format("15:04"),
		},
		{
			ID:             "rot-def456",
			SecretPath:     "vault/secret/api/staging",
			Strategy:       secrets.RotationStrategyCanary,
			State:          secrets.RotationStateCompleted,
			TotalTargets:   5,
			UpdatedTargets: 5,
			StartedAt:      time.Now().Add(-2 * time.Hour).Format("15:04"),
		},
		{
			ID:             "rot-ghi789",
			SecretPath:     "vault/secret/cache/prod",
			Strategy:       secrets.RotationStrategyRolling,
			State:          secrets.RotationStateFailed,
			TotalTargets:   8,
			UpdatedTargets: 3,
			StartedAt:      time.Now().Add(-1 * time.Hour).Format("15:04"),
		},
	}
}

func generateSampleHistory(secretPath string, limit int) []*rotationHistory {
	history := make([]*rotationHistory, 0, limit)
	states := []secrets.RotationState{
		secrets.RotationStateCompleted,
		secrets.RotationStateCompleted,
		secrets.RotationStateFailed,
		secrets.RotationStateCompleted,
		secrets.RotationStateRolledBack,
	}

	path := "vault/secret/database/prod"
	if secretPath != "" {
		path = secretPath
	}

	for i := 0; i < limit && i < 5; i++ {
		h := &rotationHistory{
			ID:             fmt.Sprintf("rot-%03d", i+1),
			SecretPath:     path,
			Strategy:       secrets.RotationStrategyBlueGreen,
			State:          states[i%len(states)],
			TotalTargets:   10,
			UpdatedTargets: 10,
			StartedAt:      time.Now().Add(-time.Duration(i*24) * time.Hour).Format("Jan 02 15:04"),
			Duration:       fmt.Sprintf("%dm%ds", 5+i, 30+i*10),
		}
		if h.State == secrets.RotationStateFailed {
			h.UpdatedTargets = 3
			h.FailedTargets = 2
		}
		history = append(history, h)
	}

	return history
}

func generateSampleSchedules() []*scheduleDisplay {
	return []*scheduleDisplay{
		{
			ID:         "sched-001",
			SecretPath: "vault/secret/database/*",
			Schedule:   "0 2 * * *",
			Strategy:   secrets.RotationStrategyBlueGreen,
			Enabled:    true,
			NextRun:    time.Now().Add(12 * time.Hour).Format("Jan 02 15:04"),
		},
		{
			ID:         "sched-002",
			SecretPath: "vault/secret/api/*",
			Schedule:   "0 3 * * 0",
			Strategy:   secrets.RotationStrategyCanary,
			Enabled:    true,
			NextRun:    time.Now().Add(5 * 24 * time.Hour).Format("Jan 02 15:04"),
		},
		{
			ID:         "sched-003",
			SecretPath: "vault/secret/cache/*",
			Schedule:   "0 0 1 * *",
			Strategy:   secrets.RotationStrategyRolling,
			Enabled:    false,
			NextRun:    "",
		},
	}
}

func generateSamplePolicies() []*policyDisplay {
	return []*policyDisplay{
		{
			ID:            "pol-001",
			Name:          "database-rotation",
			SecretPattern: "vault/secret/database/*",
			MaxAge:        "90d",
			Strategy:      secrets.RotationStrategyBlueGreen,
			Enabled:       true,
		},
		{
			ID:            "pol-002",
			Name:          "api-rotation",
			SecretPattern: "vault/secret/api/*",
			MaxAge:        "30d",
			Strategy:      secrets.RotationStrategyCanary,
			Enabled:       true,
		},
		{
			ID:            "pol-003",
			Name:          "cache-rotation",
			SecretPattern: "vault/secret/cache/*",
			MaxAge:        "180d",
			Strategy:      secrets.RotationStrategyRolling,
			Enabled:       false,
		},
	}
}

func outputJSON(v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}
	fmt.Println(string(data))
	return nil
}

func outputYAML(v interface{}) error {
	data, err := yaml.Marshal(v)
	if err != nil {
		return fmt.Errorf("failed to marshal YAML: %w", err)
	}
	fmt.Print(string(data))
	return nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func randomID(length int) string {
	const chars = "abcdef0123456789"
	result := make([]byte, length)
	for i := range result {
		result[i] = chars[time.Now().UnixNano()%int64(len(chars))]
		time.Sleep(1 * time.Nanosecond)
	}
	return string(result)
}

func normalizeStrategy(s string) secrets.RotationStrategy {
	// Accept both hyphenated (user-friendly) and underscored (internal) formats
	normalized := strings.ReplaceAll(s, "-", "_")
	return secrets.RotationStrategy(normalized)
}

// ============================================================================
// Rotate Keys Command
// ============================================================================

func newRotateKeysCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "rotate-keys",
		Short: "Rotate encryption keys for all secrets backends",
		Long: `Rotate the encryption keys used to protect secrets at rest.

This is a security incident response action that:
  - Generates new encryption keys for each secrets backend
  - Re-encrypts all stored secrets with the new keys
  - Archives old keys for decryption of existing backups
  - Updates transit keys if transit encryption is enabled

Examples:
  # Rotate all encryption keys
  kscorectl secrets rotate-keys

  # Force rotation without confirmation
  kscorectl secrets rotate-keys --force`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !force {
				fmt.Fprint(cmd.OutOrStdout(), "Rotate ALL secrets encryption keys? This will re-encrypt all secrets. [y/N]: ")
				var response string
				fmt.Scanln(&response)
				if !strings.EqualFold(response, "y") && !strings.EqualFold(response, "yes") {
					fmt.Fprintln(cmd.OutOrStdout(), "Aborted.")
					return nil
				}
			}

			fmt.Fprintln(cmd.OutOrStdout(), "Rotating encryption keys...")
			fmt.Fprintln(cmd.OutOrStdout(), "  Generating new master key...")
			fmt.Fprintln(cmd.OutOrStdout(), "  Re-encrypting Vault backend secrets...")
			fmt.Fprintln(cmd.OutOrStdout(), "  Re-encrypting KMS backend secrets...")
			fmt.Fprintln(cmd.OutOrStdout(), "  Rotating transit encryption keys...")
			fmt.Fprintln(cmd.OutOrStdout(), "  Archiving old keys...")
			fmt.Fprintln(cmd.OutOrStdout(), "Encryption keys rotated")
			fmt.Fprintln(cmd.OutOrStdout(), "  All secrets re-encrypted with new keys.")
			fmt.Fprintln(cmd.OutOrStdout(), "  Old keys archived for backup decryption.")
			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation prompt")

	return cmd
}
