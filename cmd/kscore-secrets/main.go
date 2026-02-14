// Package main implements the kscore-secrets CLI for secrets management operations.
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"gopkg.in/yaml.v3"

	"github.com/shawnbutts/keystone-core/internal/cli/output"
	"github.com/shawnbutts/keystone-core/internal/secrets"
	apisecrets "github.com/shawnbutts/keystone-core/pkg/api/secrets"
	pkgsecrets "github.com/shawnbutts/keystone-core/pkg/secrets"
	"github.com/shawnbutts/keystone-core/pkg/version"
)

// Config holds CLI configuration.
type Config struct {
	ServerAddr    string
	RESTAddr      string // REST API address override (for testing)
	OutputFormat  string
	Verbose       bool
	TLS           bool
	TLSCACert     string
	TLSCert       string
	TLSKey        string
	TLSSkipVerify bool
	TLSServerName string
	TLSMinVersion string
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	cfg := &Config{}

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

	rootCmd.PersistentFlags().StringVarP(&cfg.ServerAddr, "server", "s", "localhost:9090", "Control plane server address")
	rootCmd.PersistentFlags().StringVarP(&cfg.OutputFormat, "output", "o", "table", "Output format (table, json, yaml)")
	rootCmd.PersistentFlags().BoolVarP(&cfg.Verbose, "verbose", "v", false, "Enable verbose output")

	// TLS flags
	rootCmd.PersistentFlags().BoolVar(&cfg.TLS, "tls", false, "Enable TLS for server connection")
	rootCmd.PersistentFlags().StringVar(&cfg.TLSCACert, "tls-ca-cert", "", "Path to CA certificate for verifying the server")
	rootCmd.PersistentFlags().StringVar(&cfg.TLSCert, "tls-cert", "", "Path to client certificate for mTLS authentication")
	rootCmd.PersistentFlags().StringVar(&cfg.TLSKey, "tls-key", "", "Path to client private key for mTLS authentication")
	rootCmd.PersistentFlags().BoolVar(&cfg.TLSSkipVerify, "tls-skip-verify", false, "Skip TLS certificate verification (INSECURE - for development only)")
	rootCmd.PersistentFlags().StringVar(&cfg.TLSServerName, "tls-server-name", "", "Server name for TLS verification (defaults to server host)")
	rootCmd.PersistentFlags().StringVar(&cfg.TLSMinVersion, "tls-min-version", "1.3", "Minimum TLS version (1.2 or 1.3)")
	rootCmd.PersistentFlags().StringVar(&cfg.RESTAddr, "rest-addr", "", "REST API address override")
	_ = rootCmd.PersistentFlags().MarkHidden("rest-addr")

	rootCmd.AddCommand(
		newVersionCmd(),
		newGetCmd(cfg),
		newListCmd(cfg),
		newBackendsCmd(cfg),
		newAuditCmd(cfg),
		newRotateCmd(cfg),
		newScheduleCmd(cfg),
		newPolicyCmd(cfg),
		newDynamicCmd(cfg),
		newLeasesCmd(cfg),
		newEncryptCmd(cfg),
		newDecryptCmd(cfg),
		newRewrapCmd(cfg),
		newTemplateCmd(cfg),
		newCacheCmd(cfg),
		newRotateKeysCmd(cfg),
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
// gRPC Client Helpers
// =============================================================================

// createSecretsClient creates a secrets gRPC client from the CLI config.
func createSecretsClient(cfg *Config) (*pkgsecrets.Client, error) {
	var opts []grpc.DialOption

	if cfg.TLS || cfg.TLSCACert != "" || cfg.TLSCert != "" {
		tlsConfig, err := buildTLSConfig(cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to configure TLS: %w", err)
		}
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	return pkgsecrets.NewClient(cfg.ServerAddr, opts...)
}

// buildTLSConfig builds a TLS configuration from the CLI flags.
func buildTLSConfig(cfg *Config) (*tls.Config, error) {
	minVersion, err := parseTLSMinVersion(cfg.TLSMinVersion)
	if err != nil {
		return nil, err
	}

	tlsConfig := &tls.Config{ // nosemgrep: problem-based-packs.insecure-transport.go-stdlib.bypass-tls-verification.bypass-tls-verification -- InsecureSkipVerify allowed only when KSCORE_ALLOW_INSECURE_TLS=1 is set for dev/test
		MinVersion: minVersion, // #nosec G402 -- validated to TLS 1.2+ defaults
	}

	if cfg.TLSSkipVerify {
		if os.Getenv("KSCORE_ALLOW_INSECURE_TLS") != "1" {
			return nil, fmt.Errorf("TLS skip verify requires KSCORE_ALLOW_INSECURE_TLS=1 for development/testing only")
		}
		fmt.Fprintln(os.Stderr, "WARNING: TLS certificate verification is disabled. This is insecure and should only be used for development.")
		tlsConfig.InsecureSkipVerify = true // #nosec G402 -- gated by KSCORE_ALLOW_INSECURE_TLS
	}

	if cfg.TLSServerName != "" {
		tlsConfig.ServerName = cfg.TLSServerName
	}

	if cfg.TLSCACert != "" {
		caCert, err := os.ReadFile(cfg.TLSCACert)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA certificate: %w", err)
		}
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}
		tlsConfig.RootCAs = caCertPool
	}

	if cfg.TLSCert != "" || cfg.TLSKey != "" {
		if cfg.TLSCert == "" || cfg.TLSKey == "" {
			return nil, fmt.Errorf("both --tls-cert and --tls-key must be provided for mTLS")
		}
		cert, err := tls.LoadX509KeyPair(cfg.TLSCert, cfg.TLSKey)
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	return tlsConfig, nil
}

func parseTLSMinVersion(value string) (uint16, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "1.3", "tls1.3", "tls13":
		return tls.VersionTLS13, nil
	case "1.2", "tls1.2", "tls12":
		return tls.VersionTLS12, nil
	default:
		return 0, fmt.Errorf("unsupported TLS minimum version: %s", value)
	}
}

// createRESTClient creates a REST client for the secrets REST API endpoints.
// If RESTAddr is set (e.g. for testing), it's used directly. Otherwise the
// address is derived from the gRPC server address with port 8443.
func createRESTClient(cfg *Config) *RESTClient {
	if cfg.RESTAddr != "" {
		return NewRESTClient(cfg.RESTAddr)
	}
	scheme := "http"
	host := cfg.ServerAddr
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		host = host[:idx]
	}
	host += ":8443"
	if cfg.TLS {
		scheme = "https"
	}
	return NewRESTClient(scheme + "://" + host)
}

// =============================================================================
// Get Command (wired to gRPC)
// =============================================================================

func newGetCmd(cfg *Config) *cobra.Command {
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
			client, err := createSecretsClient(cfg)
			if err != nil {
				return fmt.Errorf("failed to connect: %w", err)
			}
			defer client.Close()
			return runGet(cmd, cfg, client, args[0], ver, field)
		},
	}

	cmd.Flags().IntVar(&ver, "version", 0, "Secret version (0 = latest)")
	cmd.Flags().StringVar(&field, "field", "", "Extract a specific field from the secret")

	return cmd
}

func runGet(cmd *cobra.Command, cfg *Config, client *pkgsecrets.Client, path string, ver int, field string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	s, err := client.GetSecret(ctx, path, ver)
	if err != nil {
		return fmt.Errorf("failed to get secret: %w", err)
	}

	if field != "" {
		if _, ok := s.Data[field]; !ok {
			keys := dataKeys(s.Data)
			return fmt.Errorf("field %q not found in secret (available: %s)", field, strings.Join(keys, ", "))
		}
		fmt.Printf("****\n")
		return nil
	}

	switch cfg.OutputFormat {
	case "json":
		return outputJSON(s)
	case "yaml":
		return outputYAML(s)
	default:
		keys := dataKeys(s.Data)
		createdAt := ""
		if !s.CreatedAt.IsZero() {
			createdAt = s.CreatedAt.Format(time.RFC3339)
		}
		expiresAt := ""
		if !s.ExpiresAt.IsZero() {
			expiresAt = s.ExpiresAt.Format(time.RFC3339)
		}
		table := &output.Table{
			Headers: []string{"PATH", "VERSION", "CREATED", "EXPIRES", "KEYS"},
		}
		table.Rows = append(table.Rows, []string{
			s.Path,
			fmt.Sprintf("%d", s.Version),
			createdAt,
			expiresAt,
			strings.Join(keys, ", "),
		})
		output.WriteTable(os.Stdout, table)
		fmt.Println()
		fmt.Println("Values:")
		for _, k := range keys {
			fmt.Printf("  %s: ****\n", k)
		}
	}

	return nil
}

// =============================================================================
// List Command (wired to gRPC)
// =============================================================================

func newListCmd(cfg *Config) *cobra.Command {
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
			client, err := createSecretsClient(cfg)
			if err != nil {
				return fmt.Errorf("failed to connect: %w", err)
			}
			defer client.Close()
			return runList(cmd, cfg, client, prefix, limit, showMetadata)
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum number of secrets to show")
	cmd.Flags().BoolVar(&showMetadata, "show-metadata", false, "Show additional metadata")

	return cmd
}

func runList(cmd *cobra.Command, cfg *Config, client *pkgsecrets.Client, prefix string, limit int, _ bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := client.ListSecrets(ctx, "", prefix, int32(limit), "") //nolint:gosec // G115: limit is small
	if err != nil {
		return fmt.Errorf("failed to list secrets: %w", err)
	}

	if len(result.Keys) == 0 {
		fmt.Println("No secrets found")
		return nil
	}

	switch cfg.OutputFormat {
	case "json":
		return outputJSON(result)
	case "yaml":
		return outputYAML(result)
	default:
		table := &output.Table{
			Headers: []string{"PATH"},
		}
		for _, key := range result.Keys {
			table.Rows = append(table.Rows, []string{key})
		}
		output.WriteTable(os.Stdout, table)
		fmt.Printf("\nTotal: %d secret(s)\n", len(result.Keys))
	}

	return nil
}

// =============================================================================
// Backends Command (stub — no gRPC RPC)
// =============================================================================

func newBackendsCmd(cfg *Config) *cobra.Command {
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
			return runBackends(cmd, cfg)
		},
	}
}

func runBackends(cmd *cobra.Command, cfg *Config) error {
	client := createRESTClient(cfg)
	resp, err := client.ListBackends()
	if err != nil {
		return fmt.Errorf("failed to list backends: %w", err)
	}

	if len(resp.Backends) == 0 {
		fmt.Println("No backends configured")
		return nil
	}

	switch cfg.OutputFormat {
	case "json":
		return outputJSON(resp.Backends)
	case "yaml":
		return outputYAML(resp.Backends)
	default:
		table := &output.Table{
			Headers: []string{"NAME", "TYPE", "HEALTHY"},
		}
		for _, b := range resp.Backends {
			healthy := "yes"
			if !b.Healthy {
				healthy = "no"
			}
			table.Rows = append(table.Rows, []string{
				b.Name,
				b.Type,
				healthy,
			})
		}
		output.WriteTable(os.Stdout, table)
		fmt.Printf("\nTotal: %d backend(s)\n", resp.Total)
	}

	return nil
}

// =============================================================================
// Audit Command (stub — no gRPC RPC)
// =============================================================================

func newAuditCmd(cfg *Config) *cobra.Command {
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
			return runAudit(cmd, cfg, args[0], limit)
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 20, "Maximum number of audit entries to show")

	return cmd
}

func runAudit(cmd *cobra.Command, cfg *Config, path string, limit int) error {
	client := createRESTClient(cfg)
	resp, err := client.ListAuditEntries(AuditListOpts{
		Path:  path,
		Limit: limit,
	})
	if err != nil {
		return fmt.Errorf("failed to query audit log: %w", err)
	}

	if len(resp.Events) == 0 {
		fmt.Println("No audit entries found")
		return nil
	}

	switch cfg.OutputFormat {
	case "json":
		return outputJSON(resp.Events)
	case "yaml":
		return outputYAML(resp.Events)
	default:
		table := &output.Table{
			Headers: []string{"TIMESTAMP", "ACTION", "AGENT", "SOURCE IP", "SUCCESS"},
		}
		for _, e := range resp.Events {
			table.Rows = append(table.Rows, []string{
				e.Timestamp.Format(time.RFC3339),
				e.Action,
				e.AgentID,
				e.SourceIP,
				fmt.Sprintf("%v", e.Success),
			})
		}
		output.WriteTable(os.Stdout, table)
		fmt.Printf("\nPath: %s\n", path)
		fmt.Printf("Showing %d audit entry(ies)\n", len(resp.Events))
	}

	return nil
}

// =============================================================================
// Rotate Commands (stub — no gRPC RPCs)
// =============================================================================

func newRotateCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "rotate",
		Aliases: []string{"rot", "r"},
		Short:   "Manage secret rotations",
		Long:    `Commands for managing secret rotations in Keystone Core.`,
	}

	cmd.AddCommand(
		newRotateListCmd(cfg),
		newRotateShowCmd(cfg),
		newRotateStartCmd(cfg),
		newRotateStatusCmd(cfg),
		newRotateHistoryCmd(cfg),
		newRotateTriggerCmd(cfg),
		newRotateRollbackCmd(cfg),
		newRotatePauseCmd(cfg),
		newRotateResumeCmd(cfg),
		newRotateCancelCmd(cfg),
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

func newRotateListCmd(cfg *Config) *cobra.Command {
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
			return runRotateList(cmd, cfg, opts)
		},
	}

	cmd.Flags().StringVar(&opts.State, "state", "", "Filter by state (pending, in_progress, completed, failed, rolled_back)")
	cmd.Flags().StringVar(&opts.Strategy, "strategy", "", "Filter by strategy (rolling, blue-green, canary)")
	cmd.Flags().StringArrayVar(&opts.Labels, "label", nil, "Filter by label (key:value format)")
	cmd.Flags().IntVar(&opts.Limit, "limit", 50, "Maximum number of rotations to show")

	return cmd
}

func runRotateList(cmd *cobra.Command, cfg *Config, opts *RotateListOptions) error {
	client := createRESTClient(cfg)
	resp, err := client.ListRotations()
	if err != nil {
		return fmt.Errorf("failed to list rotations: %w", err)
	}

	rotations := resp.Rotations
	if opts.State != "" {
		var filtered []apisecrets.RotationResponse
		for i := range rotations {
			if rotations[i].State == opts.State {
				filtered = append(filtered, rotations[i])
			}
		}
		rotations = filtered
	}
	if opts.Strategy != "" {
		var filtered []apisecrets.RotationResponse
		for i := range rotations {
			if rotations[i].Strategy == opts.Strategy {
				filtered = append(filtered, rotations[i])
			}
		}
		rotations = filtered
	}

	if len(rotations) == 0 {
		fmt.Println("No rotations found")
		return nil
	}

	switch cfg.OutputFormat {
	case "json":
		return outputJSON(rotations)
	case "yaml":
		return outputYAML(rotations)
	default:
		table := &output.Table{
			Headers: []string{"ID", "SECRET PATH", "STRATEGY", "STATE", "PROGRESS", "STARTED"},
		}
		for i := range rotations {
			r := &rotations[i]
			progress := fmt.Sprintf("%d/%d", r.UpdatedTargets, r.TotalTargets)
			if r.TotalTargets > 0 {
				progress = fmt.Sprintf("%s (%d%%)", progress, r.UpdatedTargets*100/r.TotalTargets)
			}
			table.Rows = append(table.Rows, []string{
				truncate(r.ID, 12),
				truncate(r.SecretPath, 25),
				r.Strategy,
				r.State,
				progress,
				r.StartedAt.Format(time.RFC3339),
			})
		}
		output.WriteTable(os.Stdout, table)
		fmt.Printf("\nTotal: %d rotation(s)\n", len(rotations))
	}

	return nil
}

func newRotateShowCmd(cfg *Config) *cobra.Command {
	return &cobra.Command{
		Use:   "show <rotation-id>",
		Short: "Show rotation details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRotateShow(cmd, cfg, args[0])
		},
	}
}

func runRotateShow(cmd *cobra.Command, cfg *Config, id string) error {
	client := createRESTClient(cfg)
	r, err := client.GetRotation(id)
	if err != nil {
		return fmt.Errorf("failed to get rotation: %w", err)
	}

	switch cfg.OutputFormat {
	case "json":
		return outputJSON(r)
	case "yaml":
		return outputYAML(r)
	default:
		fmt.Printf("Rotation: %s\n", r.ID)
		fmt.Printf("  Secret Path:      %s\n", r.SecretPath)
		fmt.Printf("  Strategy:         %s\n", r.Strategy)
		fmt.Printf("  State:            %s\n", r.State)
		pct := 0
		if r.TotalTargets > 0 {
			pct = r.UpdatedTargets * 100 / r.TotalTargets
		}
		fmt.Printf("  Progress:         %d/%d targets (%d%%)\n", r.UpdatedTargets, r.TotalTargets, pct)
		fmt.Printf("  Failed Targets:   %d\n", r.FailedTargets)
		fmt.Printf("  Started:          %s\n", r.StartedAt.Format(time.RFC3339))
		if r.Error != "" {
			fmt.Printf("  Error:            %s\n", r.Error)
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

func newRotateStartCmd(cfg *Config) *cobra.Command {
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
			return runRotateStart(cmd, cfg, opts)
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

func runRotateStart(cmd *cobra.Command, cfg *Config, opts *RotateStartOptions) error {
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

	rotationID := fmt.Sprintf("rot-%s", randomID(8))
	batchDelay, _ := time.ParseDuration(opts.BatchDelay)

	targets := make([]apisecrets.RotationTargetRequest, 0, len(opts.Targets))
	for _, t := range opts.Targets {
		targets = append(targets, apisecrets.RotationTargetRequest{
			ID:      t,
			AgentID: t,
		})
	}

	client := createRESTClient(cfg)
	resp, err := client.StartRotation(&apisecrets.StartRotationRequest{
		ID:         rotationID,
		SecretPath: opts.SecretPath,
		Strategy:   string(strategy),
		Targets:    targets,
		Config: apisecrets.RotationConfigRequest{
			Strategy:          string(strategy),
			BatchSize:         opts.BatchSize,
			BatchDelaySeconds: int64(batchDelay.Seconds()),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to start rotation: %w", err)
	}

	fmt.Printf("Started rotation '%s' for secret '%s'\n", resp.ID, resp.SecretPath)
	fmt.Printf("  Strategy:     %s\n", resp.Strategy)
	fmt.Printf("  Use 'kscorectl secrets rotate status %s' to monitor progress\n", resp.ID)

	return nil
}

func newRotateStatusCmd(cfg *Config) *cobra.Command {
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
			return runRotateStatus(cmd, cfg, args[0], watch, interval)
		},
	}

	cmd.Flags().BoolVarP(&watch, "watch", "w", false, "Watch status continuously")
	cmd.Flags().StringVar(&interval, "interval", "2s", "Watch interval")

	return cmd
}

func runRotateStatus(cmd *cobra.Command, cfg *Config, id string, _ bool, _ string) error {
	client := createRESTClient(cfg)
	r, err := client.GetRotation(id)
	if err != nil {
		return fmt.Errorf("failed to get rotation status: %w", err)
	}

	switch cfg.OutputFormat {
	case "json":
		return outputJSON(r)
	case "yaml":
		return outputYAML(r)
	default:
		printRotationStatus(r)
	}

	return nil
}

func printRotationStatus(r *apisecrets.RotationResponse) {
	stateIcon := "..."
	switch r.State {
	case "completed":
		stateIcon = "OK"
	case "failed":
		stateIcon = "FAIL"
	case "rolled_back":
		stateIcon = "ROLLBACK"
	case "in_progress":
		stateIcon = "RUNNING"
	}

	pct := 0
	if r.TotalTargets > 0 {
		pct = r.UpdatedTargets * 100 / r.TotalTargets
	}

	fmt.Printf("[%s] Rotation %s: %s\n", stateIcon, r.ID, r.State)
	fmt.Printf("  Progress:     %d/%d targets (%d%%)\n", r.UpdatedTargets, r.TotalTargets, pct)
	fmt.Printf("  Failed:       %d\n", r.FailedTargets)
	fmt.Printf("  Started:      %s\n", r.StartedAt.Format(time.RFC3339))

	barWidth := 40
	filled := int(float64(pct) / 100.0 * float64(barWidth))
	bar := strings.Repeat("=", filled) + strings.Repeat("-", barWidth-filled)
	fmt.Printf("  [%s]\n", bar)
}

// HistoryOptions holds history command options
type HistoryOptions struct {
	Limit  int
	Status string
}

func newRotateHistoryCmd(cfg *Config) *cobra.Command {
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
			return runRotateHistory(cmd, cfg, secretPath, opts)
		},
	}

	cmd.Flags().IntVar(&opts.Limit, "limit", 20, "Number of rotations to show")
	cmd.Flags().StringVar(&opts.Status, "status", "", "Filter by status")

	return cmd
}

func runRotateHistory(_ *cobra.Command, _ *Config, _ string, _ *HistoryOptions) error {
	return fmt.Errorf("rotation history requires persistent storage not yet available; use 'rotate list' to view active rotations")
}

func newRotateTriggerCmd(cfg *Config) *cobra.Command {
	return &cobra.Command{
		Use:   "trigger <rotation-id>",
		Short: "Trigger a scheduled rotation immediately",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := createRESTClient(cfg)
			resp, err := client.TriggerRotation(args[0])
			if err != nil {
				return fmt.Errorf("failed to trigger rotation: %w", err)
			}
			fmt.Printf("Triggered rotation %s (success: %v)\n", resp.RotationID, resp.Success)
			return nil
		},
	}
}

func newRotateRollbackCmd(cfg *Config) *cobra.Command {
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
			client := createRESTClient(cfg)
			resp, err := client.RollbackRotation(args[0])
			if err != nil {
				return fmt.Errorf("failed to rollback rotation: %w", err)
			}
			fmt.Printf("Rollback initiated for rotation %s (success: %v)\n", resp.RotationID, resp.Success)
			if reason != "" {
				fmt.Printf("  Reason: %s\n", reason)
			}
			fmt.Printf("Use 'kscorectl secrets rotate status %s' to monitor.\n", args[0])
			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Force rollback without confirmation")
	cmd.Flags().StringVar(&reason, "reason", "", "Reason for rollback")

	return cmd
}

func newRotatePauseCmd(cfg *Config) *cobra.Command {
	return &cobra.Command{
		Use:   "pause <rotation-id>",
		Short: "Pause an in-progress rotation",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := createRESTClient(cfg)
			resp, err := client.PauseRotation(args[0])
			if err != nil {
				return fmt.Errorf("failed to pause rotation: %w", err)
			}
			fmt.Printf("Paused rotation %s (success: %v)\n", resp.RotationID, resp.Success)
			return nil
		},
	}
}

func newRotateResumeCmd(cfg *Config) *cobra.Command {
	return &cobra.Command{
		Use:   "resume <rotation-id>",
		Short: "Resume a paused rotation",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := createRESTClient(cfg)
			resp, err := client.ResumeRotation(args[0])
			if err != nil {
				return fmt.Errorf("failed to resume rotation: %w", err)
			}
			fmt.Printf("Resumed rotation %s (success: %v)\n", resp.RotationID, resp.Success)
			return nil
		},
	}
}

func newRotateCancelCmd(cfg *Config) *cobra.Command {
	var reason string

	cmd := &cobra.Command{
		Use:   "cancel <rotation-id>",
		Short: "Cancel an in-progress rotation",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := createRESTClient(cfg)
			resp, err := client.CancelRotation(args[0])
			if err != nil {
				return fmt.Errorf("failed to cancel rotation: %w", err)
			}
			fmt.Printf("Cancelled rotation %s (success: %v)\n", resp.RotationID, resp.Success)
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
// Schedule Commands (stub — no gRPC RPCs)
// =============================================================================

func newScheduleCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "schedule",
		Aliases: []string{"sched", "s"},
		Short:   "Manage rotation schedules",
		Long:    `Commands for managing rotation schedules.`,
	}

	cmd.AddCommand(
		newScheduleListCmd(cfg),
		newScheduleShowCmd(cfg),
		newScheduleCreateCmd(cfg),
		newScheduleEnableCmd(cfg),
		newScheduleDisableCmd(cfg),
		newScheduleDeleteCmd(cfg),
	)

	return cmd
}

func newScheduleListCmd(cfg *Config) *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List rotation schedules",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := createRESTClient(cfg)
			resp, err := client.ListRotationPolicies()
			if err != nil {
				return fmt.Errorf("failed to list schedules: %w", err)
			}

			if len(resp.Policies) == 0 {
				fmt.Println("No schedules found")
				return nil
			}

			switch cfg.OutputFormat {
			case "json":
				return outputJSON(resp.Policies)
			case "yaml":
				return outputYAML(resp.Policies)
			default:
				table := &output.Table{
					Headers: []string{"ID", "NAME", "SCHEDULE", "MAX AGE", "ENABLED"},
				}
				for i := range resp.Policies {
					p := &resp.Policies[i]
					enabled := "No"
					if p.Enabled {
						enabled = "Yes"
					}
					table.Rows = append(table.Rows, []string{
						truncate(p.ID, 12),
						truncate(p.Name, 20),
						p.Schedule,
						p.MaxAge,
						enabled,
					})
				}
				output.WriteTable(os.Stdout, table)
			}
			return nil
		},
	}
}

func newScheduleShowCmd(cfg *Config) *cobra.Command {
	return &cobra.Command{
		Use:   "show <policy-id>",
		Short: "Show schedule details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := createRESTClient(cfg)
			p, err := client.GetRotationPolicy(args[0])
			if err != nil {
				return fmt.Errorf("failed to get schedule: %w", err)
			}

			switch cfg.OutputFormat {
			case "json":
				return outputJSON(p)
			case "yaml":
				return outputYAML(p)
			default:
				fmt.Printf("Schedule: %s\n", p.ID)
				fmt.Printf("  Name:         %s\n", p.Name)
				fmt.Printf("  Schedule:     %s\n", p.Schedule)
				fmt.Printf("  Max Age:      %s\n", p.MaxAge)
				fmt.Printf("  Auto Rotate:  %v\n", p.AutoRotate)
				fmt.Printf("  Enabled:      %v\n", p.Enabled)
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

func newScheduleCreateCmd(cfg *Config) *cobra.Command {
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
			return runScheduleCreate(cmd, cfg, opts)
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

func runScheduleCreate(_ *cobra.Command, cfg *Config, opts *ScheduleCreateOptions) error {
	if opts.SecretPath == "" {
		return fmt.Errorf("--secret is required")
	}
	if opts.Schedule == "" {
		return fmt.Errorf("--schedule is required")
	}

	policyID := fmt.Sprintf("sched-%s", randomID(8))
	client := createRESTClient(cfg)
	resp, err := client.CreateRotationPolicy(&apisecrets.CreateRotationPolicyRequest{
		ID:         policyID,
		Name:       opts.SecretPath,
		MaxAge:     "90d",
		Schedule:   opts.Schedule,
		AutoRotate: true,
		Enabled:    opts.Enabled,
	})
	if err != nil {
		return fmt.Errorf("failed to create schedule: %w", err)
	}

	fmt.Printf("Created rotation schedule '%s' for '%s'\n", resp.ID, resp.Name)
	fmt.Printf("  Schedule: %s\n", resp.Schedule)
	return nil
}

func newScheduleEnableCmd(cfg *Config) *cobra.Command {
	return &cobra.Command{
		Use:   "enable <policy-id>",
		Short: "Enable a schedule",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := createRESTClient(cfg)
			resp, err := client.EnableRotationPolicy(args[0])
			if err != nil {
				return fmt.Errorf("failed to enable schedule: %w", err)
			}
			fmt.Printf("Enabled schedule %s (success: %v)\n", resp.PolicyID, resp.Success)
			return nil
		},
	}
}

func newScheduleDisableCmd(cfg *Config) *cobra.Command {
	return &cobra.Command{
		Use:   "disable <policy-id>",
		Short: "Disable a schedule",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := createRESTClient(cfg)
			resp, err := client.DisableRotationPolicy(args[0])
			if err != nil {
				return fmt.Errorf("failed to disable schedule: %w", err)
			}
			fmt.Printf("Disabled schedule %s (success: %v)\n", resp.PolicyID, resp.Success)
			return nil
		},
	}
}

func newScheduleDeleteCmd(cfg *Config) *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "delete <policy-id>",
		Short: "Delete a schedule",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !force {
				fmt.Printf("Are you sure you want to delete schedule %s? (use --force to confirm)\n", args[0])
				return nil
			}
			client := createRESTClient(cfg)
			resp, err := client.DeleteRotationPolicy(args[0])
			if err != nil {
				return fmt.Errorf("failed to delete schedule: %w", err)
			}
			fmt.Printf("Deleted schedule %s (success: %v)\n", resp.PolicyID, resp.Success)
			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Force deletion")

	return cmd
}

// =============================================================================
// Policy Commands (stub — no gRPC RPCs)
// =============================================================================

func newPolicyCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "policy",
		Aliases: []string{"pol", "p"},
		Short:   "Manage rotation policies",
		Long:    `Commands for managing rotation policies.`,
	}

	cmd.AddCommand(
		newPolicyListCmd(cfg),
		newPolicyShowCmd(cfg),
		newPolicyCreateCmd(cfg),
		newPolicyDeleteCmd(cfg),
	)

	return cmd
}

func newPolicyListCmd(cfg *Config) *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List rotation policies",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := createRESTClient(cfg)
			resp, err := client.ListRotationPolicies()
			if err != nil {
				return fmt.Errorf("failed to list policies: %w", err)
			}

			if len(resp.Policies) == 0 {
				fmt.Println("No policies found")
				return nil
			}

			switch cfg.OutputFormat {
			case "json":
				return outputJSON(resp.Policies)
			case "yaml":
				return outputYAML(resp.Policies)
			default:
				table := &output.Table{
					Headers: []string{"ID", "NAME", "MAX AGE", "SCHEDULE", "AUTO ROTATE", "ENABLED"},
				}
				for i := range resp.Policies {
					p := &resp.Policies[i]
					enabled := "No"
					if p.Enabled {
						enabled = "Yes"
					}
					autoRotate := "No"
					if p.AutoRotate {
						autoRotate = "Yes"
					}
					table.Rows = append(table.Rows, []string{
						truncate(p.ID, 12),
						truncate(p.Name, 20),
						p.MaxAge,
						p.Schedule,
						autoRotate,
						enabled,
					})
				}
				output.WriteTable(os.Stdout, table)
			}
			return nil
		},
	}
}

func newPolicyShowCmd(cfg *Config) *cobra.Command {
	return &cobra.Command{
		Use:   "show <policy-id>",
		Short: "Show policy details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := createRESTClient(cfg)
			p, err := client.GetRotationPolicy(args[0])
			if err != nil {
				return fmt.Errorf("failed to get policy: %w", err)
			}

			switch cfg.OutputFormat {
			case "json":
				return outputJSON(p)
			case "yaml":
				return outputYAML(p)
			default:
				fmt.Printf("Policy: %s\n", p.Name)
				fmt.Printf("  ID:              %s\n", p.ID)
				fmt.Printf("  Max Age:         %s\n", p.MaxAge)
				fmt.Printf("  Warning Age:     %s\n", p.WarningAge)
				fmt.Printf("  Schedule:        %s\n", p.Schedule)
				fmt.Printf("  Auto Rotate:     %v\n", p.AutoRotate)
				fmt.Printf("  Rollback on Fail: %v\n", p.RollbackOnFail)
				fmt.Printf("  Enabled:         %v\n", p.Enabled)
				if len(p.CredentialTypes) > 0 {
					fmt.Printf("  Credential Types: %s\n", strings.Join(p.CredentialTypes, ", "))
				}
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

func newPolicyCreateCmd(cfg *Config) *cobra.Command {
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
			return runPolicyCreate(cmd, cfg, opts)
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

func runPolicyCreate(_ *cobra.Command, cfg *Config, opts *PolicyCreateOptions) error {
	if opts.Name == "" {
		return fmt.Errorf("--name is required")
	}
	if opts.SecretPattern == "" {
		return fmt.Errorf("--pattern is required")
	}

	policyID := fmt.Sprintf("pol-%s", randomID(8))
	client := createRESTClient(cfg)
	resp, err := client.CreateRotationPolicy(&apisecrets.CreateRotationPolicyRequest{
		ID:             policyID,
		Name:           opts.Name,
		MaxAge:         opts.MaxAge,
		RollbackOnFail: opts.HealthRequired,
		Enabled:        opts.Enabled,
	})
	if err != nil {
		return fmt.Errorf("failed to create policy: %w", err)
	}

	fmt.Printf("Created rotation policy '%s' (id: %s)\n", resp.Name, resp.ID)
	fmt.Printf("  Max Age:  %s\n", resp.MaxAge)
	fmt.Printf("  Enabled:  %v\n", resp.Enabled)
	return nil
}

func newPolicyDeleteCmd(cfg *Config) *cobra.Command {
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
			client := createRESTClient(cfg)
			resp, err := client.DeleteRotationPolicy(args[0])
			if err != nil {
				return fmt.Errorf("failed to delete policy: %w", err)
			}
			fmt.Printf("Deleted policy %s (success: %v)\n", resp.PolicyID, resp.Success)
			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Force deletion")

	return cmd
}

// =============================================================================
// Dynamic Commands (wired to gRPC)
// =============================================================================

func newDynamicCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dynamic",
		Short: "Manage dynamic secrets",
		Long:  `Commands for managing dynamic secrets (database credentials, cloud tokens, etc.).`,
	}

	cmd.AddCommand(
		newDynamicListCmd(cfg),
		newDynamicGetCmd(cfg),
		newDynamicRevokeCmd(cfg),
	)

	return cmd
}

func newDynamicListCmd(cfg *Config) *cobra.Command {
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
			client, err := createSecretsClient(cfg)
			if err != nil {
				return fmt.Errorf("failed to connect: %w", err)
			}
			defer client.Close()
			return runDynamicList(cmd, cfg, client, backend)
		},
	}

	cmd.Flags().StringVar(&backend, "backend", "", "Filter by backend name")

	return cmd
}

func runDynamicList(cmd *cobra.Command, cfg *Config, client *pkgsecrets.Client, backend string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := client.ListLeases(ctx, "", backend, 100, "")
	if err != nil {
		return fmt.Errorf("failed to list dynamic secrets: %w", err)
	}

	if len(result.Leases) == 0 {
		fmt.Println("No active dynamic secrets found")
		return nil
	}

	switch cfg.OutputFormat {
	case "json":
		return outputJSON(result.Leases)
	case "yaml":
		return outputYAML(result.Leases)
	default:
		table := &output.Table{
			Headers: []string{"LEASE ID", "SECRET PATH", "BACKEND", "STATE", "TTL", "EXPIRES"},
		}
		for _, l := range result.Leases {
			expiresAt := ""
			if !l.ExpiresAt.IsZero() {
				expiresAt = l.ExpiresAt.Format("15:04")
			}
			table.Rows = append(table.Rows, []string{
				truncate(l.ID, 16),
				truncate(l.SecretPath, 30),
				l.Backend,
				l.State,
				l.TTL.String(),
				expiresAt,
			})
		}
		output.WriteTable(os.Stdout, table)
		fmt.Printf("\nTotal: %d dynamic secret(s)\n", len(result.Leases))
	}

	return nil
}

func newDynamicGetCmd(cfg *Config) *cobra.Command {
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
			client, err := createSecretsClient(cfg)
			if err != nil {
				return fmt.Errorf("failed to connect: %w", err)
			}
			defer client.Close()
			return runDynamicGet(cmd, cfg, client, args[0], ttl)
		},
	}

	cmd.Flags().StringVar(&ttl, "ttl", "1h", "Requested TTL for the dynamic secret")

	return cmd
}

func runDynamicGet(cmd *cobra.Command, cfg *Config, client *pkgsecrets.Client, path, _ string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	s, err := client.GetSecret(ctx, path, 0)
	if err != nil {
		return fmt.Errorf("failed to get dynamic secret: %w", err)
	}

	switch cfg.OutputFormat {
	case "json":
		return outputJSON(s)
	case "yaml":
		return outputYAML(s)
	default:
		fmt.Printf("Generated dynamic secret:\n")
		fmt.Printf("  Lease ID:   %s\n", s.LeaseID)
		fmt.Printf("  Path:       %s\n", s.Path)
		fmt.Printf("  Renewable:  %v\n", s.Renewable)
		if !s.CreatedAt.IsZero() {
			fmt.Printf("  Issued:     %s\n", s.CreatedAt.Format(time.RFC3339))
		}
		if !s.ExpiresAt.IsZero() {
			fmt.Printf("  Expires:    %s\n", s.ExpiresAt.Format(time.RFC3339))
		}
		fmt.Println()
		fmt.Println("Values:")
		for k := range s.Data {
			fmt.Printf("  %s: ****\n", k)
		}
	}

	return nil
}

func newDynamicRevokeCmd(cfg *Config) *cobra.Command {
	return &cobra.Command{
		Use:   "revoke <lease-id>",
		Short: "Revoke a dynamic secret",
		Long: `Revoke a dynamic secret by its lease ID.

Examples:
  kscorectl secrets dynamic revoke lease-abc12345`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := createSecretsClient(cfg)
			if err != nil {
				return fmt.Errorf("failed to connect: %w", err)
			}
			defer client.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			if err := client.RevokeLease(ctx, args[0]); err != nil {
				return fmt.Errorf("failed to revoke dynamic secret: %w", err)
			}
			fmt.Printf("Revoked dynamic secret (lease: %s)\n", args[0])
			return nil
		},
	}
}

// =============================================================================
// Leases Commands (wired to gRPC)
// =============================================================================

func newLeasesCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "leases",
		Aliases: []string{"lease"},
		Short:   "Manage secret leases",
		Long:    `Commands for managing secret leases.`,
	}

	cmd.AddCommand(
		newLeasesListCmd(cfg),
		newLeasesRevokeCmd(cfg),
		newLeasesRenewCmd(cfg),
	)

	return cmd
}

func newLeasesListCmd(cfg *Config) *cobra.Command {
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
			client, err := createSecretsClient(cfg)
			if err != nil {
				return fmt.Errorf("failed to connect: %w", err)
			}
			defer client.Close()
			return runLeasesList(cmd, cfg, client, backend, state)
		},
	}

	cmd.Flags().StringVar(&backend, "backend", "", "Filter by backend name")
	cmd.Flags().StringVar(&state, "state", "", "Filter by state (active, expiring, expired, revoked)")

	return cmd
}

func runLeasesList(cmd *cobra.Command, cfg *Config, client *pkgsecrets.Client, backend, state string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := client.ListLeases(ctx, "", backend, 100, "")
	if err != nil {
		return fmt.Errorf("failed to list leases: %w", err)
	}

	leases := result.Leases
	if state != "" {
		var filtered []*pkgsecrets.Lease
		for _, l := range leases {
			if l.State == state {
				filtered = append(filtered, l)
			}
		}
		leases = filtered
	}

	if len(leases) == 0 {
		fmt.Println("No leases found")
		return nil
	}

	switch cfg.OutputFormat {
	case "json":
		return outputJSON(leases)
	case "yaml":
		return outputYAML(leases)
	default:
		table := &output.Table{
			Headers: []string{"LEASE ID", "SECRET PATH", "BACKEND", "STATE", "TTL", "RENEWALS", "EXPIRES"},
		}
		for _, l := range leases {
			expiresAt := ""
			if !l.ExpiresAt.IsZero() {
				expiresAt = l.ExpiresAt.Format("15:04")
			}
			table.Rows = append(table.Rows, []string{
				truncate(l.ID, 16),
				truncate(l.SecretPath, 25),
				l.Backend,
				l.State,
				l.TTL.String(),
				fmt.Sprintf("%d", l.RenewalCount),
				expiresAt,
			})
		}
		output.WriteTable(os.Stdout, table)
		fmt.Printf("\nTotal: %d lease(s)\n", len(leases))
	}

	return nil
}

func newLeasesRevokeCmd(cfg *Config) *cobra.Command {
	return &cobra.Command{
		Use:   "revoke <lease-id>",
		Short: "Revoke a lease",
		Long: `Revoke a secret lease by its ID.

Examples:
  kscorectl secrets leases revoke lease-abc12345`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := createSecretsClient(cfg)
			if err != nil {
				return fmt.Errorf("failed to connect: %w", err)
			}
			defer client.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			if err := client.RevokeLease(ctx, args[0]); err != nil {
				return fmt.Errorf("failed to revoke lease: %w", err)
			}
			fmt.Printf("Revoked lease %s\n", args[0])
			return nil
		},
	}
}

func newLeasesRenewCmd(cfg *Config) *cobra.Command {
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
			client, err := createSecretsClient(cfg)
			if err != nil {
				return fmt.Errorf("failed to connect: %w", err)
			}
			defer client.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			dur, err := time.ParseDuration(increment)
			if err != nil {
				return fmt.Errorf("invalid increment duration: %w", err)
			}

			lease, err := client.RenewLease(ctx, args[0], dur)
			if err != nil {
				return fmt.Errorf("failed to renew lease: %w", err)
			}
			fmt.Printf("Renewed lease %s (new TTL: %s)\n", args[0], lease.TTL)
			return nil
		},
	}

	cmd.Flags().StringVar(&increment, "increment", "1h", "TTL increment for renewal")

	return cmd
}

// =============================================================================
// Encrypt Command (wired to gRPC)
// =============================================================================

func newEncryptCmd(cfg *Config) *cobra.Command {
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
			client, err := createSecretsClient(cfg)
			if err != nil {
				return fmt.Errorf("failed to connect: %w", err)
			}
			defer client.Close()
			return runEncrypt(cmd, cfg, client, args[0], key, transitContext)
		},
	}

	cmd.Flags().StringVar(&key, "key", "", "Transit encryption key name (required)")
	cmd.Flags().StringVar(&transitContext, "context", "", "Additional authenticated data context")
	_ = cmd.MarkFlagRequired("key")

	return cmd
}

func runEncrypt(cmd *cobra.Command, cfg *Config, client *pkgsecrets.Client, plaintext, key, transitContext string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var aadContext []byte
	if transitContext != "" {
		aadContext = []byte(transitContext)
	}

	result, err := client.Encrypt(ctx, key, []byte(plaintext), aadContext, 0)
	if err != nil {
		return fmt.Errorf("failed to encrypt: %w", err)
	}

	switch cfg.OutputFormat {
	case "json":
		return outputJSON(result)
	case "yaml":
		return outputYAML(result)
	default:
		fmt.Printf("Ciphertext: %s\n", result.Ciphertext)
		fmt.Printf("Key:        %s (v%d)\n", key, result.KeyVersion)
		if transitContext != "" {
			fmt.Printf("Context:    %s\n", transitContext)
		}
	}

	return nil
}

// =============================================================================
// Decrypt Command (wired to gRPC)
// =============================================================================

func newDecryptCmd(cfg *Config) *cobra.Command {
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
			client, err := createSecretsClient(cfg)
			if err != nil {
				return fmt.Errorf("failed to connect: %w", err)
			}
			defer client.Close()
			return runDecrypt(cmd, cfg, client, args[0], key, transitContext)
		},
	}

	cmd.Flags().StringVar(&key, "key", "", "Transit encryption key name (required)")
	cmd.Flags().StringVar(&transitContext, "context", "", "Additional authenticated data context")
	_ = cmd.MarkFlagRequired("key")

	return cmd
}

func runDecrypt(cmd *cobra.Command, cfg *Config, client *pkgsecrets.Client, ciphertext, key, transitContext string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var aadContext []byte
	if transitContext != "" {
		aadContext = []byte(transitContext)
	}

	result, err := client.Decrypt(ctx, key, ciphertext, aadContext)
	if err != nil {
		return fmt.Errorf("failed to decrypt: %w", err)
	}

	switch cfg.OutputFormat {
	case "json":
		return outputJSON(result)
	case "yaml":
		return outputYAML(result)
	default:
		fmt.Printf("Plaintext:  %s\n", string(result.Plaintext))
		fmt.Printf("Key:        %s (v%d)\n", key, result.KeyVersion)
		if transitContext != "" {
			fmt.Printf("Context:    %s\n", transitContext)
		}
	}

	return nil
}

// =============================================================================
// Rewrap Command (stub — no gRPC RPC)
// =============================================================================

func newRewrapCmd(cfg *Config) *cobra.Command {
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
			return runRewrap(cmd, cfg, args[0], key)
		},
	}

	cmd.Flags().StringVar(&key, "key", "", "Transit encryption key name (required)")
	_ = cmd.MarkFlagRequired("key")

	return cmd
}

func runRewrap(cmd *cobra.Command, cfg *Config, ciphertext, key string) error {
	client := createRESTClient(cfg)
	resp, err := client.TransitRewrap(key, &apisecrets.TransitRewrapRequest{
		Ciphertext: ciphertext,
	})
	if err != nil {
		return fmt.Errorf("failed to rewrap: %w", err)
	}

	result := &transitResult{
		KeyName:    key,
		Operation:  "rewrap",
		Ciphertext: resp.Ciphertext,
		KeyVersion: resp.KeyVersion,
	}

	switch cfg.OutputFormat {
	case "json":
		return outputJSON(result)
	case "yaml":
		return outputYAML(result)
	default:
		fmt.Printf("Rewrapped ciphertext: %s\n", result.Ciphertext)
		fmt.Printf("Key:                  %s (v%d)\n", result.KeyName, result.KeyVersion)
	}

	return nil
}

// =============================================================================
// Template Command (stub — no gRPC RPC)
// =============================================================================

func newTemplateCmd(cfg *Config) *cobra.Command {
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
			return runTemplate(cmd, cfg, args[0], outFile, dryRun)
		},
	}

	cmd.Flags().StringVar(&outFile, "out-file", "", "Output file path (default: stdout)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what secrets would be injected without rendering")

	return cmd
}

func runTemplate(_ *cobra.Command, _ *Config, _, _ string, _ bool) error {
	return fmt.Errorf("template rendering requires server-side secret resolution not yet available")
}

// =============================================================================
// Cache Commands (stub — no gRPC RPCs)
// =============================================================================

func newCacheCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cache",
		Short: "Manage the secret cache",
		Long:  `Commands for managing the local secret cache.`,
	}

	cmd.AddCommand(
		newCacheStatusCmd(cfg),
		newCacheClearCmd(cfg),
		newCacheListCmd(cfg),
	)

	return cmd
}

func newCacheStatusCmd(cfg *Config) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show cache statistics",
		Long: `Show secret cache statistics including size, hit rate, and TTL.

Examples:
  kscorectl secrets cache status
  kscorectl secrets cache status -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCacheStatus(cmd, cfg)
		},
	}
}

func runCacheStatus(cmd *cobra.Command, cfg *Config) error {
	client := createRESTClient(cfg)
	stats, err := client.GetCacheStats()
	if err != nil {
		return fmt.Errorf("failed to get cache stats: %w", err)
	}

	switch cfg.OutputFormat {
	case "json":
		return outputJSON(stats)
	case "yaml":
		return outputYAML(stats)
	default:
		total := stats.Hits + stats.Misses
		hitRate := 0.0
		if total > 0 {
			hitRate = float64(stats.Hits) / float64(total) * 100
		}
		fmt.Printf("Secret Cache Status:\n")
		fmt.Printf("  Entries:      %d / %d\n", stats.Entries, stats.MaxEntries)
		fmt.Printf("  Hit Rate:     %.1f%%\n", hitRate)
		fmt.Printf("  Hits:         %d\n", stats.Hits)
		fmt.Printf("  Misses:       %d\n", stats.Misses)
		fmt.Printf("  Evictions:    %d\n", stats.Evictions)
		fmt.Printf("  Expired:      %d\n", stats.ExpiredCount)
		fmt.Printf("  Memory:       %s\n", formatBytes(stats.MemoryBytes))
	}

	return nil
}

func newCacheClearCmd(cfg *Config) *cobra.Command {
	return &cobra.Command{
		Use:   "clear",
		Short: "Clear the secret cache",
		Long: `Clear all entries from the secret cache on the server.

Examples:
  kscorectl secrets cache clear`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client := createRESTClient(cfg)
			resp, err := client.ClearCache()
			if err != nil {
				return fmt.Errorf("failed to clear cache: %w", err)
			}
			fmt.Printf("Cleared secret cache (%d entries removed)\n", resp.Cleared)
			return nil
		},
	}
}

func newCacheListCmd(cfg *Config) *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List cached secrets",
		Long: `List all cached secret entries.

Examples:
  kscorectl secrets cache list
  kscorectl secrets cache list -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCacheList(cmd, cfg)
		},
	}
}

func runCacheList(cmd *cobra.Command, cfg *Config) error {
	client := createRESTClient(cfg)
	stats, err := client.GetCacheStats()
	if err != nil {
		return fmt.Errorf("failed to get cache info: %w", err)
	}

	if stats.Entries == 0 {
		fmt.Println("Cache is empty")
		return nil
	}

	switch cfg.OutputFormat {
	case "json":
		return outputJSON(stats)
	case "yaml":
		return outputYAML(stats)
	default:
		fmt.Printf("Cache contains %d entries (max %d)\n", stats.Entries, stats.MaxEntries)
		fmt.Printf("  Hits:      %d\n", stats.Hits)
		fmt.Printf("  Misses:    %d\n", stats.Misses)
		fmt.Printf("  Evictions: %d\n", stats.Evictions)
		fmt.Printf("  Expired:   %d\n", stats.ExpiredCount)
		if stats.MemoryBytes > 0 {
			fmt.Printf("  Memory:    %s\n", formatBytes(stats.MemoryBytes))
		}
	}

	return nil
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

// ============================================================================
// Rotate Keys Command (stub — no gRPC RPC)
// ============================================================================

func newRotateKeysCmd(_ *Config) *cobra.Command {
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
		RunE: func(_ *cobra.Command, _ []string) error {
			return fmt.Errorf("encryption key rotation requires server-side support not yet available")
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation prompt")

	return cmd
}

// =============================================================================
// Display Types
// =============================================================================

type transitResult struct {
	KeyName       string `json:"key_name" yaml:"key_name"`
	Operation     string `json:"operation" yaml:"operation"`
	Ciphertext    string `json:"ciphertext,omitempty" yaml:"ciphertext,omitempty"`
	Plaintext     string `json:"plaintext,omitempty" yaml:"plaintext,omitempty"`
	KeyVersion    int    `json:"key_version" yaml:"key_version"`
	OldKeyVersion int    `json:"old_key_version,omitempty" yaml:"old_key_version,omitempty"`
	Context       string `json:"context,omitempty" yaml:"context,omitempty"`
}


// =============================================================================
// Helpers
// =============================================================================

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
	normalized := strings.ReplaceAll(s, "-", "_")
	return secrets.RotationStrategy(normalized)
}

// dataKeys returns sorted keys from a map for stable output.
func dataKeys(data map[string]interface{}) []string {
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	return keys
}
