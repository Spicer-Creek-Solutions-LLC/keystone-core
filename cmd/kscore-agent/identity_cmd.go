package main

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/shawnbutts/keystone-core/internal/identity"
)

const (
	defaultIdentityDir = "/var/lib/keystone-core/identity"
	svidCertFile       = "svid.pem"
	trustBundleFile    = "trust-bundle.pem"
)

func newIdentityCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "identity",
		Short: "Manage the agent's SPIFFE identity",
		Long:  `Commands for viewing and managing the agent's SPIFFE identity, including SVID certificates and trust bundles.`,
	}

	cmd.AddCommand(newIdentityShowCmd())
	cmd.AddCommand(newIdentityRenewCmd())
	cmd.AddCommand(newIdentityStatusCmd())

	return cmd
}

func newIdentityShowCmd() *cobra.Command {
	var dataDir string

	cmd := &cobra.Command{
		Use:   "show",
		Short: "Display the agent's current SPIFFE identity",
		Long: `Display the agent's current SPIFFE identity including the SPIFFE ID,
trust domain, certificate expiry, and issuer information.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runIdentityShow(cmd, dataDir)
		},
		SilenceUsage: true,
	}

	cmd.Flags().StringVar(&dataDir, "data-dir", defaultIdentityDir, "Path to the identity data directory")

	return cmd
}

func newIdentityRenewCmd() *cobra.Command {
	var (
		dataDir string
		timeout time.Duration
	)

	cmd := &cobra.Command{
		Use:   "renew",
		Short: "Request identity renewal from the SPIRE server",
		Long: `Request an immediate renewal of the agent's SPIFFE SVID certificate.
This contacts the identity provider to obtain a fresh certificate
before the current one expires.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runIdentityRenew(cmd, dataDir, timeout)
		},
		SilenceUsage: true,
	}

	cmd.Flags().StringVar(&dataDir, "data-dir", defaultIdentityDir, "Path to the identity data directory")
	cmd.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "Timeout for the renewal request")

	return cmd
}

func newIdentityStatusCmd() *cobra.Command {
	var dataDir string

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show identity health status",
		Long: `Show the health status of the agent's SPIFFE identity.
Reports whether the identity is valid, expiring soon, or expired.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runIdentityStatus(cmd, dataDir)
		},
		SilenceUsage: true,
	}

	cmd.Flags().StringVar(&dataDir, "data-dir", defaultIdentityDir, "Path to the identity data directory")

	return cmd
}

func runIdentityShow(cmd *cobra.Command, dataDir string) error {
	svid, err := loadSVIDFromDisk(dataDir)
	if err != nil {
		return fmt.Errorf("failed to load SVID: %w", err)
	}

	w := cmd.OutOrStdout()

	fmt.Fprintf(w, "SPIFFE Identity:\n")
	fmt.Fprintf(w, "  SPIFFE ID:    %s\n", svid.SPIFFEID.String())
	fmt.Fprintf(w, "  Trust Domain: %s\n", svid.SPIFFEID.TrustDomain)
	fmt.Fprintf(w, "  Path:         %s\n", svid.SPIFFEID.Path)
	fmt.Fprintf(w, "  Issued At:    %s\n", svid.IssuedAt.Format(time.RFC3339))
	fmt.Fprintf(w, "  Expires At:   %s\n", svid.ExpiresAt.Format(time.RFC3339))

	remaining := time.Until(svid.ExpiresAt)
	if remaining > 0 {
		fmt.Fprintf(w, "  TTL:          %s\n", remaining.Truncate(time.Second))
	} else {
		fmt.Fprintf(w, "  TTL:          expired %s ago\n", (-remaining).Truncate(time.Second))
	}

	if len(svid.Certificates) > 0 {
		leaf := svid.Certificates[0]
		fmt.Fprintf(w, "\nCertificate Details:\n")
		fmt.Fprintf(w, "  Subject:      %s\n", leaf.Subject.CommonName)
		fmt.Fprintf(w, "  Issuer:       %s\n", leaf.Issuer.CommonName)
		fmt.Fprintf(w, "  Serial:       %s\n", leaf.SerialNumber.String())
		fmt.Fprintf(w, "  Key Algorithm: %s\n", leaf.PublicKeyAlgorithm.String())
	}

	return nil
}

func runIdentityRenew(cmd *cobra.Command, dataDir string, timeout time.Duration) error {
	svid, err := loadSVIDFromDisk(dataDir)
	if err != nil {
		return fmt.Errorf("failed to load current SVID: %w", err)
	}

	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "Current SPIFFE ID: %s\n", svid.SPIFFEID.String())
	fmt.Fprintf(w, "Current expiry:    %s\n", svid.ExpiresAt.Format(time.RFC3339))
	fmt.Fprintf(w, "Requesting renewal...\n")

	ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
	defer cancel()

	newSVID, err := requestRenewal(ctx, svid, dataDir)
	if err != nil {
		return fmt.Errorf("renewal failed: %w", err)
	}

	fmt.Fprintf(w, "Renewal successful.\n")
	fmt.Fprintf(w, "New expiry:        %s\n", newSVID.ExpiresAt.Format(time.RFC3339))
	fmt.Fprintf(w, "New TTL:           %s\n", time.Until(newSVID.ExpiresAt).Truncate(time.Second))

	return nil
}

func runIdentityStatus(cmd *cobra.Command, dataDir string) error {
	w := cmd.OutOrStdout()

	certPath := filepath.Join(dataDir, svidCertFile)
	if _, err := os.Stat(certPath); os.IsNotExist(err) {
		fmt.Fprintf(w, "Status: NO IDENTITY\n")
		fmt.Fprintf(w, "The agent has not yet obtained a SPIFFE identity.\n")
		fmt.Fprintf(w, "Data directory: %s\n", dataDir)
		return nil
	}

	svid, err := loadSVIDFromDisk(dataDir)
	if err != nil {
		fmt.Fprintf(w, "Status: ERROR\n")
		fmt.Fprintf(w, "Failed to load identity: %v\n", err)
		return nil
	}

	remaining := time.Until(svid.ExpiresAt)
	lifetime := svid.ExpiresAt.Sub(svid.IssuedAt)
	fractionRemaining := float64(remaining) / float64(lifetime)

	fmt.Fprintf(w, "SPIFFE ID:    %s\n", svid.SPIFFEID.String())
	fmt.Fprintf(w, "Trust Domain: %s\n", svid.SPIFFEID.TrustDomain)

	switch {
	case remaining <= 0:
		fmt.Fprintf(w, "Status:       EXPIRED\n")
		fmt.Fprintf(w, "Expired:      %s ago\n", (-remaining).Truncate(time.Second))
	case fractionRemaining < 0.1:
		fmt.Fprintf(w, "Status:       CRITICAL\n")
		fmt.Fprintf(w, "Expires in:   %s\n", remaining.Truncate(time.Second))
		fmt.Fprintf(w, "Remaining:    %.1f%% of lifetime\n", fractionRemaining*100)
	case svid.ShouldRotate():
		fmt.Fprintf(w, "Status:       EXPIRING SOON\n")
		fmt.Fprintf(w, "Expires in:   %s\n", remaining.Truncate(time.Second))
		fmt.Fprintf(w, "Remaining:    %.1f%% of lifetime\n", fractionRemaining*100)
	default:
		fmt.Fprintf(w, "Status:       VALID\n")
		fmt.Fprintf(w, "Expires in:   %s\n", remaining.Truncate(time.Second))
		fmt.Fprintf(w, "Remaining:    %.1f%% of lifetime\n", fractionRemaining*100)
	}

	fmt.Fprintf(w, "Issued At:    %s\n", svid.IssuedAt.Format(time.RFC3339))
	fmt.Fprintf(w, "Expires At:   %s\n", svid.ExpiresAt.Format(time.RFC3339))

	bundlePath := filepath.Join(dataDir, trustBundleFile)
	if _, err := os.Stat(bundlePath); err == nil {
		fmt.Fprintf(w, "Trust Bundle: present\n")
	} else {
		fmt.Fprintf(w, "Trust Bundle: missing\n")
	}

	return nil
}

func loadSVIDFromDisk(dataDir string) (*identity.X509SVID, error) {
	certPath := filepath.Join(dataDir, svidCertFile)
	certData, err := os.ReadFile(certPath)
	if err != nil {
		return nil, fmt.Errorf("cannot read SVID certificate at %s: %w", certPath, err)
	}

	var certs []*x509.Certificate
	rest := certData
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("failed to parse certificate: %w", err)
		}
		certs = append(certs, cert)
	}

	if len(certs) == 0 {
		return nil, fmt.Errorf("no certificates found in %s", certPath)
	}

	leaf := certs[0]

	var spiffeID identity.SPIFFEID
	for _, uri := range leaf.URIs {
		if uri.Scheme == "spiffe" {
			parsed, err := identity.ParseSPIFFEID(uri.String())
			if err != nil {
				return nil, fmt.Errorf("failed to parse SPIFFE ID from certificate: %w", err)
			}
			spiffeID = parsed
			break
		}
	}

	if spiffeID.TrustDomain == "" {
		return nil, fmt.Errorf("no SPIFFE ID found in certificate SANs")
	}

	return &identity.X509SVID{
		SPIFFEID:     spiffeID,
		Certificates: certs,
		IssuedAt:     leaf.NotBefore,
		ExpiresAt:    leaf.NotAfter,
	}, nil
}

func requestRenewal(ctx context.Context, currentSVID *identity.X509SVID, dataDir string) (*identity.X509SVID, error) {
	cfg, err := loadAgentConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load agent config: %w", err)
	}

	clientConfig := &identity.AgentIdentityClientConfig{
		ProviderAddress:  cfg.providerAddress,
		TrustDomain:      currentSVID.SPIFFEID.TrustDomain,
		AgentID:          extractAgentID(currentSVID.SPIFFEID),
		AttestationType:  cfg.attestationType,
		RotationInterval: 30 * time.Second,
	}

	client, err := identity.NewAgentIdentityClient(clientConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create identity client: %w", err)
	}

	if err := client.Connect(ctx); err != nil {
		return nil, fmt.Errorf("failed to connect to identity provider: %w", err)
	}
	defer client.Close()

	newSVID, err := client.FetchX509SVID(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch renewed SVID: %w", err)
	}

	return newSVID, nil
}

type agentIdentityConfig struct {
	providerAddress string
	attestationType string
}

func loadAgentConfig() (*agentIdentityConfig, error) {
	return &agentIdentityConfig{
		providerAddress: os.Getenv("KSCORE_IDENTITY_PROVIDER"),
		attestationType: getAttestationType(),
	}, nil
}

func getAttestationType() string {
	if t := os.Getenv("KSCORE_ATTESTATION_TYPE"); t != "" {
		return t
	}
	return identity.AttestationTypeJoinToken
}

func extractAgentID(id identity.SPIFFEID) string {
	const prefix = "/agent/"
	if strings.HasPrefix(id.Path, prefix) {
		return id.Path[len(prefix):]
	}
	return id.Path
}
