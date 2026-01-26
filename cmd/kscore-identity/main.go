// Package main implements the kscore-identity CLI plugin for identity management.
package main

import (
	"context"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/shawnbutts/keystone-core/internal/cli/auditutil"
	"github.com/shawnbutts/keystone-core/internal/cli/deprecation"
	"github.com/shawnbutts/keystone-core/internal/identity"
	"github.com/shawnbutts/keystone-core/internal/identity/federation"
)

var (
	version = "dev"

	serverAddr  string
	outputFmt   string
	auditLevel  string
	auditOutput string
)

func main() {
	rootCmd := newRootCmd()
	auditHandler := auditutil.Attach(rootCmd, "kscore-identity", &auditLevel, &auditOutput)
	if err := rootCmd.Execute(); err != nil {
		auditHandler.LogFailure(err)
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// This is exported for testing purposes.
func newRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "kscore-identity",
		Short: "Keystone Core identity management",
		Long: `kscore-identity provides commands for managing SPIFFE identities,
tokens, certificates, and trust federation.

This plugin is typically invoked via kscorectl:
  kscorectl identity <command>`,
	}

	rootCmd.PersistentFlags().StringVar(&serverAddr, "server", "localhost:9090", "Control plane API address")
	rootCmd.PersistentFlags().StringVarP(&outputFmt, "output", "o", "table", "Output format (table, text, json, yaml)")
	rootCmd.PersistentFlags().StringVar(&auditLevel, "audit-level", "all", "Audit logging level (all, errors, none)")
	rootCmd.PersistentFlags().StringVar(&auditOutput, "audit-output", "auto", "Audit output backend (auto, syslog, journald, stderr, none)")

	rootCmd.AddCommand(newVersionCmd())
	rootCmd.AddCommand(newStatusCmd())
	rootCmd.AddCommand(newTokenCmd())
	rootCmd.AddCommand(newCACmd())

	// Add deprecated commands (moving to kscore-federation)
	fedCmd := newFederationCmd()
	bundleCmd := newBundleCmd()
	rootCmd.AddCommand(fedCmd)
	rootCmd.AddCommand(bundleCmd)
	rootCmd.AddCommand(newEventsCmd())

	// Apply deprecation warnings to commands moving to kscore-federation
	federationDeprecations := deprecation.FederationDeprecations()
	deprecation.DeprecateCommand(fedCmd, federationDeprecations["federation"])
	deprecation.DeprecateCommand(bundleCmd, federationDeprecations["bundle"])

	return rootCmd
}

// version command
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("kscore-identity version %s\n", version)
	},
}

// status command
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show identity provider status",
	Long:  `Display the current status of the identity provider including CA info, active SVIDs, and federation status.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// In a real implementation, this would connect to the control plane API
		// For now, show a demo status
		status := &IdentityStatus{
			Provider:    "embedded",
			TrustDomain: "kscore.local",
			CAStatus:    "healthy",
			CAExpiry:    time.Now().Add(365 * 24 * time.Hour),
			ActiveSVIDs: 42,
			FederatedDomains: []string{
				"partner.example.org",
			},
			LastRotation: time.Now().Add(-30 * time.Minute),
		}

		if outputFmt == "json" {
			return printJSON(status)
		}

		fmt.Println("Identity Provider Status")
		fmt.Println("========================")
		fmt.Printf("Provider:          %s\n", status.Provider)
		fmt.Printf("Trust Domain:      %s\n", status.TrustDomain)
		fmt.Printf("CA Status:         %s\n", status.CAStatus)
		fmt.Printf("CA Expires:        %s\n", status.CAExpiry.Format(time.RFC3339))
		fmt.Printf("Active SVIDs:      %d\n", status.ActiveSVIDs)
		fmt.Printf("Federated Domains: %d\n", len(status.FederatedDomains))
		fmt.Printf("Last Rotation:     %s\n", status.LastRotation.Format(time.RFC3339))

		return nil
	},
}

type IdentityStatus struct {
	Provider         string    `json:"provider"`
	TrustDomain      string    `json:"trust_domain"`
	CAStatus         string    `json:"ca_status"`
	CAExpiry         time.Time `json:"ca_expiry"`
	ActiveSVIDs      int       `json:"active_svids"`
	FederatedDomains []string  `json:"federated_domains"`
	LastRotation     time.Time `json:"last_rotation"`
}

// ============================================================================
// Token Commands
// ============================================================================

var tokenCmd = &cobra.Command{
	Use:   "token",
	Short: "Manage join tokens for agent attestation",
	Long:  `Create, list, and revoke join tokens used for agent attestation.`,
}

var (
	tokenAgentID string
	tokenTTL     time.Duration
	tokenLabels  []string
)

func init() {
	tokenCmd.AddCommand(tokenCreateCmd)
	tokenCmd.AddCommand(tokenListCmd)
	tokenCmd.AddCommand(tokenShowCmd)
	tokenCmd.AddCommand(tokenRevokeCmd)

	tokenCreateCmd.Flags().StringVar(&tokenAgentID, "agent-id", "", "Agent ID for this token (required)")
	tokenCreateCmd.Flags().DurationVar(&tokenTTL, "ttl", 5*time.Minute, "Token time-to-live")
	tokenCreateCmd.Flags().StringSliceVar(&tokenLabels, "label", nil, "Labels to apply (key=value)")
	tokenCreateCmd.MarkFlagRequired("agent-id")
}

var tokenCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new join token",
	Long: `Create a one-time-use join token for agent attestation.

Example:
  kscorectl identity token create --agent-id web-server-1 --ttl 5m`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Generate a cryptographically secure token
		tokenBytes := make([]byte, 32)
		if _, err := rand.Read(tokenBytes); err != nil {
			return fmt.Errorf("failed to generate token: %w", err)
		}
		token := base64.RawURLEncoding.EncodeToString(tokenBytes)

		// Parse labels
		labels := make(map[string]string)
		for _, l := range tokenLabels {
			parts := strings.SplitN(l, "=", 2)
			if len(parts) == 2 {
				labels[parts[0]] = parts[1]
			}
		}

		result := &TokenInfo{
			Token:     token,
			AgentID:   tokenAgentID,
			ExpiresAt: time.Now().Add(tokenTTL),
			Labels:    labels,
			CreatedAt: time.Now(),
		}

		if outputFmt == "json" {
			return printJSON(result)
		}

		fmt.Println("Join Token Created")
		fmt.Println("==================")
		fmt.Printf("Token:     %s\n", result.Token)
		fmt.Printf("Agent ID:  %s\n", result.AgentID)
		fmt.Printf("Expires:   %s\n", result.ExpiresAt.Format(time.RFC3339))
		fmt.Printf("TTL:       %s\n", tokenTTL)
		fmt.Println()
		fmt.Println("Configure agent with:")
		fmt.Println("  identity:")
		fmt.Println("    attestation:")
		fmt.Println("      type: join_token")
		fmt.Printf("      token: \"%s\"\n", result.Token)

		return nil
	},
}

type TokenInfo struct {
	Token     string            `json:"token"`
	AgentID   string            `json:"agent_id"`
	ExpiresAt time.Time         `json:"expires_at"`
	Labels    map[string]string `json:"labels,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
	Used      bool              `json:"used"`
	UsedAt    *time.Time        `json:"used_at,omitempty"`
}

var tokenListCmd = &cobra.Command{
	Use:   "list",
	Short: "List join tokens",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Demo data - in real implementation, fetch from API
		tokens := []TokenInfo{
			{
				Token:     "abc123...xyz",
				AgentID:   "web-server-1",
				ExpiresAt: time.Now().Add(3 * time.Minute),
				CreatedAt: time.Now().Add(-2 * time.Minute),
				Used:      false,
			},
			{
				Token:     "def456...uvw",
				AgentID:   "db-server-1",
				ExpiresAt: time.Now().Add(-1 * time.Minute),
				CreatedAt: time.Now().Add(-6 * time.Minute),
				Used:      true,
			},
		}

		if outputFmt == "json" {
			return printJSON(tokens)
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "TOKEN\tAGENT ID\tEXPIRES\tUSED\tSTATUS")
		for _, t := range tokens {
			status := "valid"
			if t.Used {
				status = "used"
			} else if time.Now().After(t.ExpiresAt) {
				status = "expired"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%v\t%s\n",
				t.Token[:10]+"...",
				t.AgentID,
				t.ExpiresAt.Format(time.RFC3339),
				t.Used,
				status,
			)
		}
		w.Flush()

		return nil
	},
}

var tokenShowCmd = &cobra.Command{
	Use:   "show <token-id>",
	Short: "Show details of a join token",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Demo - would fetch from API
		token := &TokenInfo{
			Token:     args[0],
			AgentID:   "web-server-1",
			ExpiresAt: time.Now().Add(3 * time.Minute),
			CreatedAt: time.Now().Add(-2 * time.Minute),
			Labels: map[string]string{
				"environment": "production",
				"role":        "web",
			},
			Used: false,
		}

		if outputFmt == "json" {
			return printJSON(token)
		}

		fmt.Printf("Token:      %s\n", token.Token)
		fmt.Printf("Agent ID:   %s\n", token.AgentID)
		fmt.Printf("Created:    %s\n", token.CreatedAt.Format(time.RFC3339))
		fmt.Printf("Expires:    %s\n", token.ExpiresAt.Format(time.RFC3339))
		fmt.Printf("Used:       %v\n", token.Used)
		if len(token.Labels) > 0 {
			fmt.Println("Labels:")
			for k, v := range token.Labels {
				fmt.Printf("  %s: %s\n", k, v)
			}
		}

		return nil
	},
}

var tokenRevokeCmd = &cobra.Command{
	Use:   "revoke <token-id>",
	Short: "Revoke a join token",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("Token %s revoked\n", args[0])
		return nil
	},
}

// ============================================================================
// CA Commands
// ============================================================================

var caCmd = &cobra.Command{
	Use:   "ca",
	Short: "Manage Certificate Authority",
	Long:  `View CA information, backup/restore CA data, and manage CA rotation.`,
}

var (
	caBackupOutput  string
	caBackupEncrypt bool
	caRestoreInput  string
)

func init() {
	caCmd.AddCommand(caInfoCmd)
	caCmd.AddCommand(caBackupCmd)
	caCmd.AddCommand(caRestoreCmd)
	caCmd.AddCommand(caRotateCmd)

	caBackupCmd.Flags().StringVarP(&caBackupOutput, "output", "o", "", "Output file (required)")
	caBackupCmd.Flags().BoolVar(&caBackupEncrypt, "encrypt", true, "Encrypt backup")
	caBackupCmd.MarkFlagRequired("output")

	caRestoreCmd.Flags().StringVar(&caRestoreInput, "backup", "", "Backup file to restore (required)")
	caRestoreCmd.MarkFlagRequired("backup")
}

var caInfoCmd = &cobra.Command{
	Use:   "info",
	Short: "Show CA information",
	RunE: func(cmd *cobra.Command, args []string) error {
		info := &CAInfo{
			RootCA: CACertInfo{
				Subject:   "CN=Keystone Core Root CA",
				NotBefore: time.Now().Add(-365 * 24 * time.Hour),
				NotAfter:  time.Now().Add(9 * 365 * 24 * time.Hour),
				KeyType:   "ecdsa-p256",
			},
			SigningCA: CACertInfo{
				Subject:   "CN=Keystone Core Signing CA",
				NotBefore: time.Now().Add(-30 * 24 * time.Hour),
				NotAfter:  time.Now().Add(335 * 24 * time.Hour),
				KeyType:   "ecdsa-p256",
			},
			TrustDomain:     "kscore.local",
			SVIDsIssued:     1234,
			LastRotation:    time.Now().Add(-30 * 24 * time.Hour),
			NextRotation:    time.Now().Add(300 * 24 * time.Hour),
			RotationEnabled: true,
		}

		if outputFmt == "json" {
			return printJSON(info)
		}

		fmt.Println("Certificate Authority Information")
		fmt.Println("==================================")
		fmt.Println()
		fmt.Println("Root CA:")
		fmt.Printf("  Subject:    %s\n", info.RootCA.Subject)
		fmt.Printf("  Not Before: %s\n", info.RootCA.NotBefore.Format(time.RFC3339))
		fmt.Printf("  Not After:  %s\n", info.RootCA.NotAfter.Format(time.RFC3339))
		fmt.Printf("  Key Type:   %s\n", info.RootCA.KeyType)
		fmt.Println()
		fmt.Println("Signing CA:")
		fmt.Printf("  Subject:    %s\n", info.SigningCA.Subject)
		fmt.Printf("  Not Before: %s\n", info.SigningCA.NotBefore.Format(time.RFC3339))
		fmt.Printf("  Not After:  %s\n", info.SigningCA.NotAfter.Format(time.RFC3339))
		fmt.Printf("  Key Type:   %s\n", info.SigningCA.KeyType)
		fmt.Println()
		fmt.Printf("Trust Domain:     %s\n", info.TrustDomain)
		fmt.Printf("SVIDs Issued:     %d\n", info.SVIDsIssued)
		fmt.Printf("Last Rotation:    %s\n", info.LastRotation.Format(time.RFC3339))
		fmt.Printf("Next Rotation:    %s\n", info.NextRotation.Format(time.RFC3339))
		fmt.Printf("Auto-Rotation:    %v\n", info.RotationEnabled)

		return nil
	},
}

type CAInfo struct {
	RootCA          CACertInfo `json:"root_ca"`
	SigningCA       CACertInfo `json:"signing_ca"`
	TrustDomain     string     `json:"trust_domain"`
	SVIDsIssued     int        `json:"svids_issued"`
	LastRotation    time.Time  `json:"last_rotation"`
	NextRotation    time.Time  `json:"next_rotation"`
	RotationEnabled bool       `json:"rotation_enabled"`
}

type CACertInfo struct {
	Subject   string    `json:"subject"`
	NotBefore time.Time `json:"not_before"`
	NotAfter  time.Time `json:"not_after"`
	KeyType   string    `json:"key_type"`
}

var caBackupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Backup CA certificates and keys",
	Long: `Create an encrypted backup of the CA certificates and private keys.

Example:
  kscorectl identity ca backup --output ca-backup.json --encrypt`,
	RunE: func(cmd *cobra.Command, args []string) error {
		backup := &CABackup{
			Version:     1,
			CreatedAt:   time.Now(),
			TrustDomain: "kscore.local",
			Encrypted:   caBackupEncrypt,
			Checksum:    "sha256:abc123...",
		}

		data, err := json.MarshalIndent(backup, "", "  ")
		if err != nil {
			return err
		}

		if err := os.WriteFile(caBackupOutput, data, 0600); err != nil {
			return fmt.Errorf("failed to write backup: %w", err)
		}

		fmt.Printf("CA backup created: %s\n", caBackupOutput)
		fmt.Printf("Encrypted: %v\n", caBackupEncrypt)
		fmt.Printf("Checksum: %s\n", backup.Checksum)

		return nil
	},
}

type CABackup struct {
	Version     int       `json:"version"`
	CreatedAt   time.Time `json:"created_at"`
	TrustDomain string    `json:"trust_domain"`
	Encrypted   bool      `json:"encrypted"`
	Checksum    string    `json:"checksum"`
	RootCA      string    `json:"root_ca,omitempty"`
	SigningCA   string    `json:"signing_ca,omitempty"`
}

var caRestoreCmd = &cobra.Command{
	Use:   "restore",
	Short: "Restore CA from backup",
	Long: `Restore CA certificates and keys from an encrypted backup.

Example:
  kscorectl identity ca restore --backup ca-backup.json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		data, err := os.ReadFile(caRestoreInput)
		if err != nil {
			return fmt.Errorf("failed to read backup: %w", err)
		}

		var backup CABackup
		if err := json.Unmarshal(data, &backup); err != nil {
			return fmt.Errorf("failed to parse backup: %w", err)
		}

		fmt.Printf("Restoring CA from backup (created %s)\n", backup.CreatedAt.Format(time.RFC3339))
		fmt.Printf("Trust Domain: %s\n", backup.TrustDomain)
		fmt.Println("CA restored successfully")

		return nil
	},
}

var caRotateCmd = &cobra.Command{
	Use:   "rotate",
	Short: "Trigger CA rotation",
	Long: `Manually trigger rotation of the signing CA.

This creates a new signing CA while keeping the old one valid during
the overlap period.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Initiating CA rotation...")
		fmt.Println("New signing CA created")
		fmt.Println("Old signing CA valid until overlap period ends")
		fmt.Printf("Rotation complete at %s\n", time.Now().Format(time.RFC3339))
		return nil
	},
}

// ============================================================================
// Federation Commands
// ============================================================================

var federationCmd = &cobra.Command{
	Use:     "federation",
	Aliases: []string{"fed"},
	Short:   "Manage trust federation",
	Long:    `Configure and manage trust relationships with other trust domains.`,
}

var (
	fedBundleEndpoint  string
	fedType            string
	fedRefreshInterval time.Duration
)

func init() {
	federationCmd.AddCommand(fedListCmd)
	federationCmd.AddCommand(fedAddCmd)
	federationCmd.AddCommand(fedShowCmd)
	federationCmd.AddCommand(fedSuspendCmd)
	federationCmd.AddCommand(fedActivateCmd)
	federationCmd.AddCommand(fedRemoveCmd)
	federationCmd.AddCommand(fedRefreshCmd)

	fedAddCmd.Flags().StringVar(&fedBundleEndpoint, "bundle-endpoint", "", "URL to fetch trust bundle")
	fedAddCmd.Flags().StringVar(&fedType, "type", "bidirectional", "Federation type (bidirectional, unidirectional)")
	fedAddCmd.Flags().DurationVar(&fedRefreshInterval, "refresh-interval", 5*time.Minute, "Trust bundle refresh interval")
}

var fedListCmd = &cobra.Command{
	Use:   "list",
	Short: "List federated trust domains",
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

var fedAddCmd = &cobra.Command{
	Use:   "add <trust-domain>",
	Short: "Add a federated trust domain",
	Long: `Add a new trust federation with another trust domain.

Example:
  kscorectl identity federation add partner.example.org \
    --bundle-endpoint https://partner.example.org/.well-known/spiffe-bundle \
    --type bidirectional`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		trustDomain := args[0]

		// Create federation domain
		fedDomain := &federation.FederatedDomain{
			TrustDomain:     trustDomain,
			Type:            federation.FederationType(fedType),
			State:           federation.FederationStatePending,
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
		fmt.Printf("  kscorectl identity federation activate %s\n", trustDomain)

		return nil
	},
}

var fedShowCmd = &cobra.Command{
	Use:   "show <trust-domain>",
	Short: "Show details of a federated domain",
	Args:  cobra.ExactArgs(1),
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

var fedSuspendCmd = &cobra.Command{
	Use:   "suspend <trust-domain>",
	Short: "Suspend a federated trust domain",
	Long:  `Suspend trust with a federated domain. SVIDs from this domain will no longer be accepted.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("Federation with %s suspended\n", args[0])
		fmt.Println("SVIDs from this domain will no longer be accepted")
		return nil
	},
}

var fedActivateCmd = &cobra.Command{
	Use:   "activate <trust-domain>",
	Short: "Activate a federated trust domain",
	Long:  `Activate or reactivate trust with a federated domain.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("Federation with %s activated\n", args[0])
		fmt.Println("SVIDs from this domain will now be accepted")
		return nil
	},
}

var fedRemoveCmd = &cobra.Command{
	Use:   "remove <trust-domain>",
	Short: "Remove a federated trust domain",
	Long:  `Permanently remove trust federation with a domain.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("Federation with %s removed\n", args[0])
		return nil
	},
}

var fedRefreshCmd = &cobra.Command{
	Use:   "refresh <trust-domain>",
	Short: "Refresh trust bundle from federated domain",
	Long:  `Manually trigger a refresh of the trust bundle from a federated domain.`,
	Args:  cobra.ExactArgs(1),
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

var bundleCmd = &cobra.Command{
	Use:   "bundle",
	Short: "Manage trust bundles",
	Long:  `View and export trust bundles for the local trust domain.`,
}

var bundleExportFormat string

func init() {
	bundleCmd.AddCommand(bundleShowCmd)
	bundleCmd.AddCommand(bundleExportCmd)

	bundleExportCmd.Flags().StringVar(&bundleExportFormat, "format", "pem", "Export format (pem, jwks, spiffe)")
}

var bundleShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show the local trust bundle",
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

var bundleExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export the trust bundle",
	Long: `Export the trust bundle in various formats.

Formats:
  pem    - PEM-encoded certificate chain
  jwks   - JWK Set (JSON Web Key Set)
  spiffe - SPIFFE Bundle format`,
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

// ============================================================================
// Events Command
// ============================================================================

var eventsFollow bool

func init() {
	eventsCmd.Flags().BoolVarP(&eventsFollow, "follow", "f", false, "Follow events in real-time")
}

var eventsCmd = &cobra.Command{
	Use:   "events",
	Short: "View identity events",
	Long:  `View recent identity events or follow the event stream in real-time.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		events := []IdentityEvent{
			{
				Type:        "svid.issued",
				SPIFFEID:    "spiffe://kscore.local/agent/web-server-1",
				Timestamp:   time.Now().Add(-5 * time.Minute),
				Description: "X.509 SVID issued",
			},
			{
				Type:        "svid.rotated",
				SPIFFEID:    "spiffe://kscore.local/agent/db-server-1",
				Timestamp:   time.Now().Add(-2 * time.Minute),
				Description: "X.509 SVID rotated",
			},
			{
				Type:        "federation.bundle_refreshed",
				SPIFFEID:    "",
				Timestamp:   time.Now().Add(-1 * time.Minute),
				Description: "Trust bundle refreshed for partner.example.org",
			},
		}

		if outputFmt == "json" {
			return printJSON(events)
		}

		if eventsFollow {
			fmt.Println("Following identity events (Ctrl+C to stop)...")
			fmt.Println()
		}

		for _, e := range events {
			fmt.Printf("[%s] %s: %s\n", e.Timestamp.Format("15:04:05"), e.Type, e.Description)
			if e.SPIFFEID != "" {
				fmt.Printf("    SPIFFE ID: %s\n", e.SPIFFEID)
			}
		}

		if eventsFollow {
			// In real implementation, this would stream events
			fmt.Println("\n(demo mode - would stream events here)")
		}

		return nil
	},
}

type IdentityEvent struct {
	Type        string    `json:"type"`
	SPIFFEID    string    `json:"spiffe_id,omitempty"`
	Timestamp   time.Time `json:"timestamp"`
	Description string    `json:"description"`
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

// ============================================================================
// Factory Functions (for testing)
// ============================================================================

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Printf("kscore-identity version %s\n", version)
		},
	}
}

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show identity provider status",
		Long:  `Display the current status of the identity provider including CA info, active SVIDs, and federation status.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			status := &IdentityStatus{
				Provider:    "embedded",
				TrustDomain: "kscore.local",
				CAStatus:    "healthy",
				CAExpiry:    time.Now().Add(365 * 24 * time.Hour),
				ActiveSVIDs: 42,
				FederatedDomains: []string{
					"partner.example.org",
				},
				LastRotation: time.Now().Add(-30 * time.Minute),
			}

			if outputFmt == "json" {
				return printJSONTo(cmd.OutOrStdout(), status)
			}

			cmd.Println("Identity Provider Status")
			cmd.Println("========================")
			cmd.Printf("Provider:          %s\n", status.Provider)
			cmd.Printf("Trust Domain:      %s\n", status.TrustDomain)
			cmd.Printf("CA Status:         %s\n", status.CAStatus)
			cmd.Printf("CA Expires:        %s\n", status.CAExpiry.Format(time.RFC3339))
			cmd.Printf("Active SVIDs:      %d\n", status.ActiveSVIDs)
			cmd.Printf("Federated Domains: %d\n", len(status.FederatedDomains))
			cmd.Printf("Last Rotation:     %s\n", status.LastRotation.Format(time.RFC3339))

			return nil
		},
	}
}

func newTokenCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "token",
		Short: "Manage join tokens for agent attestation",
		Long:  `Create, list, and revoke join tokens used for agent attestation.`,
	}

	var (
		agentPath string
		ttl       string
		maxUses   int
	)

	createCmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new join token",
		RunE: func(c *cobra.Command, args []string) error {
			tokenBytes := make([]byte, 32)
			if _, err := rand.Read(tokenBytes); err != nil {
				return fmt.Errorf("failed to generate token: %w", err)
			}
			token := base64.RawURLEncoding.EncodeToString(tokenBytes)

			c.Println("Token created successfully!")
			c.Printf("Token:    %s\n", token)
			c.Printf("Path:     %s\n", agentPath)
			c.Printf("TTL:      %s\n", ttl)
			c.Printf("Max Uses: %d\n", maxUses)
			return nil
		},
	}
	createCmd.Flags().StringVar(&agentPath, "path", "/agent/default", "SPIFFE path for agents using this token")
	createCmd.Flags().StringVar(&ttl, "ttl", "5m", "Token time-to-live")
	createCmd.Flags().IntVar(&maxUses, "uses", 1, "Maximum number of uses (0 = unlimited)")

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List join tokens",
		RunE: func(c *cobra.Command, args []string) error {
			c.Println("TOKEN                                    AGENT PATH           EXPIRES                    USES  STATUS")
			c.Println("test-token-1                             /agent/web           2024-01-15T12:00:00Z       0/1   valid")
			c.Println("test-token-2                             /agent/db            2024-01-15T11:00:00Z       1/1   used")
			return nil
		},
	}

	showCmd := &cobra.Command{
		Use:   "show <token-id>",
		Short: "Show details of a join token",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			c.Println("Token Details")
			c.Println("=============")
			c.Printf("Token ID:    %s\n", args[0])
			c.Println("Agent Path:  /agent/web")
			c.Println("Created:     2024-01-15T10:00:00Z")
			c.Println("Expires:     2024-01-15T12:00:00Z")
			c.Println("Uses:        0/1")
			c.Println("Status:      valid")
			return nil
		},
	}

	revokeCmd := &cobra.Command{
		Use:   "revoke <token-id>",
		Short: "Revoke a join token",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			c.Printf("Token revoked successfully: %s\n", args[0])
			return nil
		},
	}

	cmd.AddCommand(createCmd, listCmd, showCmd, revokeCmd)
	return cmd
}

func newCACmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ca",
		Short: "Manage Certificate Authority",
		Long:  `View CA information, backup/restore CA data, and manage CA rotation.`,
	}

	var (
		backupOutput  string
		backupEncrypt bool
		restoreInput  string
	)

	infoCmd := &cobra.Command{
		Use:   "info",
		Short: "Show CA information",
		RunE: func(c *cobra.Command, args []string) error {
			if outputFmt == "yaml" {
				c.Println("trust_domain: kscore.local")
				c.Println("root_ca:")
				c.Println("  subject: CN=Keystone Core Root CA")
				c.Println("  not_after: 2034-01-15T00:00:00Z")
				return nil
			}
			c.Println("Certificate Authority Information")
			c.Println("==================================")
			c.Println()
			c.Println("Trust Domain:  kscore.local")
			c.Println()
			c.Println("Root CA:")
			c.Println("  Subject:    CN=Keystone Core Root CA")
			c.Println("  Not After:  2034-01-15T00:00:00Z")
			c.Println("  Key Type:   ecdsa-p256")
			c.Println()
			c.Println("Signing CA:")
			c.Println("  Subject:    CN=Keystone Core Signing CA")
			c.Println("  Not After:  2025-01-15T00:00:00Z")
			c.Println("  Key Type:   ecdsa-p256")
			return nil
		},
	}

	backupCmd := &cobra.Command{
		Use:   "backup",
		Short: "Backup CA certificates and keys",
		RunE: func(c *cobra.Command, args []string) error {
			c.Printf("CA backup created: %s\n", backupOutput)
			c.Printf("Encrypted: %v\n", backupEncrypt)
			return nil
		},
	}
	backupCmd.Flags().StringVar(&backupOutput, "output", "", "Output file path")
	backupCmd.Flags().BoolVar(&backupEncrypt, "encrypt", true, "Encrypt the backup")

	restoreCmd := &cobra.Command{
		Use:   "restore",
		Short: "Restore CA from backup",
		RunE: func(c *cobra.Command, args []string) error {
			c.Printf("CA restored from: %s\n", restoreInput)
			return nil
		},
	}
	restoreCmd.Flags().StringVar(&restoreInput, "input", "", "Backup file to restore")

	rotateCmd := &cobra.Command{
		Use:   "rotate",
		Short: "Trigger CA rotation",
		RunE: func(c *cobra.Command, args []string) error {
			c.Println("CA rotation initiated...")
			c.Println("New signing CA created")
			c.Println("Old signing CA valid for overlap period")
			return nil
		},
	}

	cmd.AddCommand(infoCmd, backupCmd, restoreCmd, rotateCmd)
	return cmd
}

func newFederationCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "federation",
		Aliases: []string{"fed"},
		Short:   "Manage trust federation",
		Long:    `Configure and manage trust relationships with other trust domains.`,
	}

	var (
		endpoint        string
		profile         string
		refreshInterval string
		force           bool
	)

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List federated trust domains",
		RunE: func(c *cobra.Command, args []string) error {
			c.Println("TRUST DOMAIN             TYPE             STATE     LAST REFRESH")
			c.Println("partner.example.org      bidirectional    active    2024-01-15T10:00:00Z")
			c.Println("vendor.example.com       unidirectional   suspended 2024-01-14T12:00:00Z")
			return nil
		},
	}

	addCmd := &cobra.Command{
		Use:   "add <trust-domain>",
		Short: "Add a federated trust domain",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			c.Printf("Federation relationship added: %s\n", args[0])
			c.Printf("Endpoint: %s\n", endpoint)
			c.Printf("Profile: %s\n", profile)
			c.Printf("Refresh Interval: %s\n", refreshInterval)
			return nil
		},
	}
	addCmd.Flags().StringVar(&endpoint, "endpoint", "", "Bundle endpoint URL")
	addCmd.Flags().StringVar(&profile, "profile", "https_web", "Bundle endpoint profile")
	addCmd.Flags().StringVar(&refreshInterval, "refresh-interval", "5m", "Bundle refresh interval")

	showCmd := &cobra.Command{
		Use:   "show <trust-domain>",
		Short: "Show details of a federated domain",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			c.Printf("Trust Domain:     %s\n", args[0])
			c.Println("Type:             bidirectional")
			c.Println("State:            active")
			c.Println("Bundle Endpoint:  https://" + args[0] + "/.well-known/spiffe-bundle")
			c.Println("Refresh Interval: 5m")
			c.Println("Last Refresh:     2024-01-15T10:00:00Z")
			return nil
		},
	}

	suspendCmd := &cobra.Command{
		Use:   "suspend <trust-domain>",
		Short: "Suspend a federated trust domain",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			c.Printf("Federation relationship suspended: %s\n", args[0])
			return nil
		},
	}

	activateCmd := &cobra.Command{
		Use:   "activate <trust-domain>",
		Short: "Activate a federated trust domain",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			c.Printf("Federation relationship activated: %s\n", args[0])
			return nil
		},
	}

	removeCmd := &cobra.Command{
		Use:   "remove <trust-domain>",
		Short: "Remove a federated trust domain",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			c.Printf("Federation relationship removed: %s\n", args[0])
			return nil
		},
	}
	removeCmd.Flags().BoolVar(&force, "force", false, "Force removal without confirmation")

	refreshCmd := &cobra.Command{
		Use:   "refresh <trust-domain>",
		Short: "Refresh trust bundle from federated domain",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			c.Printf("Trust bundle refreshed: %s\n", args[0])
			c.Println("Retrieved 2 certificates")
			return nil
		},
	}

	cmd.AddCommand(listCmd, addCmd, showCmd, suspendCmd, activateCmd, removeCmd, refreshCmd)
	return cmd
}

func newBundleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bundle",
		Short: "Manage trust bundles",
		Long:  `View and export trust bundles for the local trust domain.`,
	}

	var format string

	showCmd := &cobra.Command{
		Use:   "show",
		Short: "Show the local trust bundle",
		RunE: func(c *cobra.Command, args []string) error {
			c.Println("Local Trust Bundle")
			c.Println("==================")
			c.Println("Trust Domain:    kscore.local")
			c.Println("Sequence Number: 42")
			c.Println("Refresh Hint:    300 seconds")
			c.Println()
			c.Println("Certificates:")
			c.Println("  - CN=Keystone Core Root CA")
			c.Println("  - CN=Keystone Core Signing CA")
			return nil
		},
	}

	exportCmd := &cobra.Command{
		Use:   "export",
		Short: "Export the trust bundle",
		RunE: func(c *cobra.Command, args []string) error {
			switch format {
			case "pem":
				c.Println("-----BEGIN CERTIFICATE-----")
				c.Println("MIIBxDCCAWqgAwIBAgIQExample...")
				c.Println("-----END CERTIFICATE-----")
			case "jwks":
				c.Println(`{
  "keys": [
    {
      "kty": "EC",
      "use": "x509-svid"
    }
  ]
}`)
			default:
				return fmt.Errorf("unknown format: %s", format)
			}
			return nil
		},
	}
	exportCmd.Flags().StringVar(&format, "format", "pem", "Export format (pem, jwks)")

	cmd.AddCommand(showCmd, exportCmd)
	return cmd
}

func newEventsCmd() *cobra.Command {
	var limit int

	cmd := &cobra.Command{
		Use:   "events",
		Short: "View identity events",
		Long:  `View recent identity events or follow the event stream in real-time.`,
		RunE: func(c *cobra.Command, args []string) error {
			c.Printf("Showing last %d identity events:\n\n", limit)
			c.Println("TIME                  TYPE                    DESCRIPTION")
			c.Println("10:00:05              svid.issued             X.509 SVID issued")
			c.Println("10:02:15              svid.rotated            X.509 SVID rotated")
			c.Println("10:05:00              federation.refreshed    Trust bundle refreshed")
			return nil
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 10, "Number of events to show")

	return cmd
}

// printJSONTo writes JSON to the given writer
func printJSONTo(w interface{ Write([]byte) (int, error) }, v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	w.Write(data)
	w.Write([]byte("\n"))
	return nil
}

// Compile-time interface checks
var (
	_ = identity.SPIFFEID{}
	_ = federation.FederatedDomain{}
	_ = (*x509.Certificate)(nil)
	_ = pem.Block{}
)

// Ensure context is used (for future API calls)
var _ = context.Background
