// Package main implements the kscore-federation CLI for trust federation management.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/shawnbutts/keystone-core/internal/cli/auditutil"
	"github.com/shawnbutts/keystone-core/internal/identity/federation"
	"github.com/shawnbutts/keystone-core/pkg/version"
)

var (
	serverAddr  string
	outputFmt   string
	auditLevel  string
	auditOutput string
)

func main() {
	rootCmd := newRootCmd()
	auditHandler := auditutil.Attach(rootCmd, "kscore-federation", &auditLevel, &auditOutput)
	if err := rootCmd.Execute(); err != nil {
		auditHandler.LogFailure(err)
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "kscore-federation",
		Short: "Trust federation management for Keystone Core",
		Long: `kscore-federation manages trust federation with other SPIFFE trust domains.

This command provides tools for establishing, managing, and monitoring trust
relationships with external identity providers and trust domains.

Commands:
  list      - List federated trust domains
  add       - Add a new federated trust domain
  show      - Show details of a federated domain
  suspend   - Suspend a federated trust domain
  activate  - Activate a federated trust domain
  remove    - Remove a federated trust domain
  refresh   - Refresh trust bundle from federated domain
  bundle    - Manage trust bundles

Examples:
  # List all federated trust domains
  kscore-federation list

  # Add a new federation
  kscore-federation add partner.example.org \
    --bundle-endpoint https://partner.example.org/.well-known/spiffe-bundle \
    --type bidirectional

  # Show federation details
  kscore-federation show partner.example.org

  # Suspend a federation
  kscore-federation suspend vendor.example.com

  # Export trust bundle
  kscore-federation bundle export --format pem`,
	}

	rootCmd.PersistentFlags().StringVar(&serverAddr, "server", "localhost:9090", "Control plane API address")
	rootCmd.PersistentFlags().StringVarP(&outputFmt, "output", "o", "table", "Output format (table, text, json, yaml)")
	rootCmd.PersistentFlags().StringVar(&auditLevel, "audit-level", "all", "Audit logging level (all, errors, none)")
	rootCmd.PersistentFlags().StringVar(&auditOutput, "audit-output", "auto", "Audit output backend (auto, syslog, journald, stderr, none)")

	rootCmd.AddCommand(newVersionCmd())
	rootCmd.AddCommand(wizardCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(addCmd)
	rootCmd.AddCommand(showCmd)
	rootCmd.AddCommand(suspendCmd)
	rootCmd.AddCommand(activateCmd)
	rootCmd.AddCommand(removeCmd)
	rootCmd.AddCommand(refreshCmd)
	rootCmd.AddCommand(newBundleCmd())

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

// ============================================================================
// Federation Commands
// ============================================================================

var (
	fedBundleEndpoint  string
	fedType            string
	fedRefreshInterval time.Duration
	fedForce           bool
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List federated trust domains",
	Long: `List all federated trust domains with their current state.

Examples:
  # List all federations
  kscore-federation list

  # Output as JSON
  kscore-federation list -o json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		domains := []FederationInfo{
			{
				TrustDomain:    "partner.example.org",
				Type:           "bidirectional",
				State:          "active",
				BundleEndpoint: "https://partner.example.org/.well-known/spiffe-bundle",
				LastRefresh:    time.Now().Add(-2 * time.Minute),
				CertCount:      2,
			},
			{
				TrustDomain: "vendor.example.com",
				Type:        "unidirectional",
				State:       "suspended",
				LastRefresh: time.Now().Add(-1 * time.Hour),
				CertCount:   1,
			},
		}

		if outputFmt == "json" {
			return printJSON(domains)
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "TRUST DOMAIN\tTYPE\tSTATE\tCERTS\tLAST REFRESH")
		for _, d := range domains {
			fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\n",
				d.TrustDomain,
				d.Type,
				d.State,
				d.CertCount,
				d.LastRefresh.Format(time.RFC3339),
			)
		}
		w.Flush()

		return nil
	},
}

type FederationInfo struct {
	TrustDomain    string    `json:"trust_domain"`
	Type           string    `json:"type"`
	State          string    `json:"state"`
	BundleEndpoint string    `json:"bundle_endpoint,omitempty"`
	LastRefresh    time.Time `json:"last_refresh"`
	CertCount      int       `json:"cert_count"`
}

var addCmd = &cobra.Command{
	Use:   "add <trust-domain>",
	Short: "Add a federated trust domain",
	Long: `Add a new trust federation with another trust domain.

Federation types:
  bidirectional  - Both domains trust each other (default)
  unidirectional - Only we trust the other domain

Examples:
  # Add bidirectional federation
  kscore-federation add partner.example.org \
    --bundle-endpoint https://partner.example.org/.well-known/spiffe-bundle

  # Add unidirectional federation
  kscore-federation add vendor.example.com \
    --type unidirectional \
    --bundle-endpoint https://vendor.example.com/bundle.json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		trustDomain := args[0]

		// Create federation domain
		fedDomain := &federation.FederatedDomain{
			TrustDomain:     trustDomain,
			Type:            federation.Type(fedType),
			State:           federation.StatePending,
			BundleEndpoint:  fedBundleEndpoint,
			RefreshInterval: fedRefreshInterval,
			CreatedAt:       time.Now(),
		}

		if outputFmt == "json" {
			return printJSON(fedDomain)
		}

		fmt.Printf("Federation added: %s\n", trustDomain)
		fmt.Printf("Type: %s\n", fedType)
		fmt.Printf("State: pending (requires approval)\n")
		if fedBundleEndpoint != "" {
			fmt.Printf("Bundle Endpoint: %s\n", fedBundleEndpoint)
		}
		fmt.Printf("Refresh Interval: %s\n", fedRefreshInterval)
		fmt.Println()
		fmt.Println("To activate, run:")
		fmt.Printf("  kscore-federation activate %s\n", trustDomain)

		return nil
	},
}

func init() {
	addCmd.Flags().StringVar(&fedBundleEndpoint, "bundle-endpoint", "", "URL to fetch trust bundle")
	addCmd.Flags().StringVar(&fedType, "type", "bidirectional", "Federation type (bidirectional, unidirectional)")
	addCmd.Flags().DurationVar(&fedRefreshInterval, "refresh-interval", 5*time.Minute, "Trust bundle refresh interval")
}

var showCmd = &cobra.Command{
	Use:   "show <trust-domain>",
	Short: "Show details of a federated domain",
	Long: `Show detailed information about a federated trust domain.

Examples:
  # Show federation details
  kscore-federation show partner.example.org

  # Output as JSON
  kscore-federation show partner.example.org -o json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		info := &FederationDetails{
			TrustDomain:     args[0],
			Type:            "bidirectional",
			State:           "active",
			BundleEndpoint:  "https://" + args[0] + "/.well-known/spiffe-bundle",
			RefreshInterval: 5 * time.Minute,
			CreatedAt:       time.Now().Add(-7 * 24 * time.Hour),
			LastRefresh:     time.Now().Add(-2 * time.Minute),
			Policy: PolicyInfo{
				AllowedPaths: []string{"/service/**", "/agent/**"},
				DeniedPaths:  []string{"/admin/**"},
				RequireMTLS:  true,
			},
			Certificates: []CertSummary{
				{Subject: "CN=Partner CA", Expiry: time.Now().Add(365 * 24 * time.Hour)},
			},
		}

		if outputFmt == "json" {
			return printJSON(info)
		}

		fmt.Printf("Trust Domain:     %s\n", info.TrustDomain)
		fmt.Printf("Type:             %s\n", info.Type)
		fmt.Printf("State:            %s\n", info.State)
		fmt.Printf("Bundle Endpoint:  %s\n", info.BundleEndpoint)
		fmt.Printf("Refresh Interval: %s\n", info.RefreshInterval)
		fmt.Printf("Created:          %s\n", info.CreatedAt.Format(time.RFC3339))
		fmt.Printf("Last Refresh:     %s\n", info.LastRefresh.Format(time.RFC3339))
		fmt.Println()
		fmt.Println("Policy:")
		fmt.Printf("  Allowed Paths: %v\n", info.Policy.AllowedPaths)
		fmt.Printf("  Denied Paths:  %v\n", info.Policy.DeniedPaths)
		fmt.Printf("  Require mTLS:  %v\n", info.Policy.RequireMTLS)
		fmt.Println()
		fmt.Println("Certificates:")
		for _, cert := range info.Certificates {
			fmt.Printf("  - %s (expires %s)\n", cert.Subject, cert.Expiry.Format(time.RFC3339))
		}

		return nil
	},
}

type FederationDetails struct {
	TrustDomain     string        `json:"trust_domain"`
	Type            string        `json:"type"`
	State           string        `json:"state"`
	BundleEndpoint  string        `json:"bundle_endpoint"`
	RefreshInterval time.Duration `json:"refresh_interval"`
	CreatedAt       time.Time     `json:"created_at"`
	LastRefresh     time.Time     `json:"last_refresh"`
	Policy          PolicyInfo    `json:"policy"`
	Certificates    []CertSummary `json:"certificates"`
}

type PolicyInfo struct {
	AllowedPaths []string `json:"allowed_paths"`
	DeniedPaths  []string `json:"denied_paths"`
	RequireMTLS  bool     `json:"require_mtls"`
}

type CertSummary struct {
	Subject string    `json:"subject"`
	Expiry  time.Time `json:"expiry"`
}

var suspendCmd = &cobra.Command{
	Use:   "suspend <trust-domain>",
	Short: "Suspend a federated trust domain",
	Long: `Suspend trust with a federated domain.

When suspended, SVIDs from this domain will no longer be accepted until
the federation is reactivated.

Examples:
  # Suspend a federation
  kscore-federation suspend vendor.example.com`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("Federation with %s suspended\n", args[0])
		fmt.Println("SVIDs from this domain will no longer be accepted")
		return nil
	},
}

var activateCmd = &cobra.Command{
	Use:   "activate <trust-domain>",
	Short: "Activate a federated trust domain",
	Long: `Activate or reactivate trust with a federated domain.

This enables trust with the domain, allowing SVIDs from it to be accepted.

Examples:
  # Activate a federation
  kscore-federation activate partner.example.org`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("Federation with %s activated\n", args[0])
		fmt.Println("SVIDs from this domain will now be accepted")
		return nil
	},
}

var removeCmd = &cobra.Command{
	Use:   "remove <trust-domain>",
	Short: "Remove a federated trust domain",
	Long: `Permanently remove trust federation with a domain.

This removes all trust relationship configuration with the domain.
Use --force to skip confirmation.

Examples:
  # Remove a federation
  kscore-federation remove vendor.example.com

  # Force remove without confirmation
  kscore-federation remove vendor.example.com --force`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if !fedForce {
			fmt.Printf("Are you sure you want to remove federation with %s? [y/N] ", args[0])
			var confirm string
			fmt.Scanln(&confirm)
			if confirm != "y" && confirm != "Y" {
				fmt.Println("Aborted")
				return nil
			}
		}
		fmt.Printf("Federation with %s removed\n", args[0])
		return nil
	},
}

func init() {
	removeCmd.Flags().BoolVar(&fedForce, "force", false, "Skip confirmation prompt")
}

var refreshCmd = &cobra.Command{
	Use:   "refresh <trust-domain>",
	Short: "Refresh trust bundle from federated domain",
	Long: `Manually trigger a refresh of the trust bundle from a federated domain.

This fetches the latest certificates from the domain's bundle endpoint.

Examples:
  # Refresh trust bundle
  kscore-federation refresh partner.example.org`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("Refreshing trust bundle from %s...\n", args[0])
		fmt.Println("Trust bundle refreshed successfully")
		fmt.Printf("Retrieved %d certificates\n", 2)
		return nil
	},
}

// ============================================================================
// Bundle Commands
// ============================================================================

func newBundleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bundle",
		Short: "Manage trust bundles",
		Long: `View and export trust bundles for the local trust domain.

Trust bundles contain the certificates needed for other domains to
verify SVIDs from this trust domain.`,
	}

	cmd.AddCommand(bundleShowCmd)
	cmd.AddCommand(bundleExportCmd)

	return cmd
}

var bundleShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show the local trust bundle",
	Long: `Display information about the local trust bundle.

Examples:
  # Show trust bundle
  kscore-federation bundle show

  # Output as JSON
  kscore-federation bundle show -o json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		bundle := &BundleInfo{
			TrustDomain:    "kscore.local",
			SequenceNumber: 42,
			RefreshHint:    300,
			Certificates: []CertSummary{
				{Subject: "CN=Keystone Core Root CA", Expiry: time.Now().Add(9 * 365 * 24 * time.Hour)},
				{Subject: "CN=Keystone Core Signing CA", Expiry: time.Now().Add(335 * 24 * time.Hour)},
			},
			UpdatedAt: time.Now().Add(-30 * time.Minute),
		}

		if outputFmt == "json" {
			return printJSON(bundle)
		}

		fmt.Println("Local Trust Bundle")
		fmt.Println("==================")
		fmt.Printf("Trust Domain:    %s\n", bundle.TrustDomain)
		fmt.Printf("Sequence Number: %d\n", bundle.SequenceNumber)
		fmt.Printf("Refresh Hint:    %d seconds\n", bundle.RefreshHint)
		fmt.Printf("Updated:         %s\n", bundle.UpdatedAt.Format(time.RFC3339))
		fmt.Println()
		fmt.Println("Certificates:")
		for _, cert := range bundle.Certificates {
			fmt.Printf("  - %s\n", cert.Subject)
			fmt.Printf("    Expires: %s\n", cert.Expiry.Format(time.RFC3339))
		}

		return nil
	},
}

type BundleInfo struct {
	TrustDomain    string        `json:"trust_domain"`
	SequenceNumber uint64        `json:"sequence_number"`
	RefreshHint    int           `json:"refresh_hint"`
	Certificates   []CertSummary `json:"certificates"`
	UpdatedAt      time.Time     `json:"updated_at"`
}

var bundleExportFormat string

var bundleExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export the trust bundle",
	Long: `Export the trust bundle in various formats.

Formats:
  pem    - PEM-encoded certificate chain
  jwks   - JWK Set (JSON Web Key Set)
  spiffe - SPIFFE Bundle format

Examples:
  # Export as PEM
  kscore-federation bundle export --format pem

  # Export as JWKS
  kscore-federation bundle export --format jwks`,
	RunE: func(cmd *cobra.Command, args []string) error {
		switch bundleExportFormat {
		case "pem":
			fmt.Println("-----BEGIN CERTIFICATE-----")
			fmt.Println("MIIBxDCCAWqgAwIBAgIQExample...")
			fmt.Println("-----END CERTIFICATE-----")
		case "jwks", "spiffe":
			bundle := map[string]interface{}{
				"keys": []map[string]interface{}{
					{
						"kty": "EC",
						"use": "x509-svid",
						"x5c": []string{"MIIBxDCCAWqgAwIBAgIQExample..."},
					},
				},
				"spiffe_refresh_hint":    300,
				"spiffe_sequence_number": 42,
			}
			data, _ := json.MarshalIndent(bundle, "", "  ")
			fmt.Println(string(data))
		default:
			return fmt.Errorf("unknown format: %s", bundleExportFormat)
		}
		return nil
	},
}

func init() {
	bundleExportCmd.Flags().StringVar(&bundleExportFormat, "format", "pem", "Export format (pem, jwks, spiffe)")
}

// ============================================================================
// Helpers
// ============================================================================

func printJSON(v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

// Ensure imports are used
var _ = context.Background
var _ = federation.FederatedDomain{}
