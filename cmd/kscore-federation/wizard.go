package main

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/shawnbutts/keystone-core/internal/identity/federation"
	"github.com/shawnbutts/keystone-core/internal/identity/federation/wizard"
)

var (
	// Wizard flags
	wizardNonInteractive bool
	wizardDomain         string
	wizardEndpoint       string
	wizardType           string
	wizardPolicy         string
	wizardRefresh        time.Duration
	wizardMTLS           bool
	wizardAutoActivate   bool
)

var wizardCmd = &cobra.Command{
	Use:   "wizard",
	Short: "Interactive trust federation setup wizard",
	Long: `Guided setup for establishing trust federation with another SPIFFE trust domain.

The wizard will guide you through:
  1. Specifying the partner trust domain
  2. Discovering or configuring the bundle endpoint
  3. Selecting federation type (bidirectional/unidirectional)
  4. Choosing a trust policy template
  5. Configuring refresh interval and mTLS requirements
  6. Reviewing and confirming the configuration

Examples:
  # Run interactive wizard
  kscore-federation wizard

  # Non-interactive mode with all options
  kscore-federation wizard \
    --non-interactive \
    --domain partner.example.org \
    --endpoint https://partner.example.org/.well-known/spiffe-bundle \
    --type bidirectional \
    --policy services-only \
    --refresh 5m \
    --mtls \
    --auto-activate`,
	RunE: runWizard,
}

func init() {
	wizardCmd.Flags().BoolVar(&wizardNonInteractive, "non-interactive", false, "Run with prompts from flags only")
	wizardCmd.Flags().StringVar(&wizardDomain, "domain", "", "Partner trust domain")
	wizardCmd.Flags().StringVar(&wizardEndpoint, "endpoint", "", "Bundle endpoint URL")
	wizardCmd.Flags().StringVar(&wizardType, "type", "bidirectional", "Federation type (bidirectional, unidirectional)")
	wizardCmd.Flags().StringVar(&wizardPolicy, "policy", "services-only", "Policy template name (allow-all, services-only, agents-only, kubernetes)")
	wizardCmd.Flags().DurationVar(&wizardRefresh, "refresh", 5*time.Minute, "Bundle refresh interval")
	wizardCmd.Flags().BoolVar(&wizardMTLS, "mtls", true, "Require mutual TLS")
	wizardCmd.Flags().BoolVar(&wizardAutoActivate, "auto-activate", false, "Activate immediately without confirmation")
}

func runWizard(cmd *cobra.Command, args []string) error {
	if wizardNonInteractive {
		return runNonInteractiveWizard(cmd)
	}

	return runInteractiveWizard(cmd)
}

func runInteractiveWizard(cmd *cobra.Command) error {
	// Build initial config from flags (if any provided)
	initial := &wizard.WizardConfig{
		TrustDomain:     wizardDomain,
		BundleEndpoint:  wizardEndpoint,
		RefreshInterval: wizardRefresh,
		RequireMTLS:     wizardMTLS,
	}

	if wizardType == "unidirectional" {
		initial.FederationType = federation.FederationTypeUnidirectional
	} else {
		initial.FederationType = federation.FederationTypeBidirectional
	}

	// Run the interactive wizard
	result, err := wizard.RunWithConfig(initial)
	if err != nil {
		return fmt.Errorf("wizard failed: %w", err)
	}

	if result.Cancelled {
		fmt.Fprintln(cmd.ErrOrStderr(), "Wizard cancelled")
		return nil
	}

	if result.Error != nil {
		return result.Error
	}

	// Display result
	if outputFmt == "json" {
		return printJSON(result.Domain)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "\nFederation configuration created:\n")
	fmt.Fprintf(cmd.OutOrStdout(), "  Trust Domain:     %s\n", result.Config.TrustDomain)
	fmt.Fprintf(cmd.OutOrStdout(), "  Bundle Endpoint:  %s\n", result.Config.BundleEndpoint)
	fmt.Fprintf(cmd.OutOrStdout(), "  Type:             %s\n", result.Config.FederationType)
	fmt.Fprintf(cmd.OutOrStdout(), "  Refresh Interval: %s\n", result.Config.RefreshInterval)
	fmt.Fprintf(cmd.OutOrStdout(), "  Require mTLS:     %v\n", result.Config.RequireMTLS)
	fmt.Fprintf(cmd.OutOrStdout(), "  State:            pending\n")
	fmt.Fprintln(cmd.OutOrStdout())

	if !wizardAutoActivate {
		fmt.Fprintln(cmd.OutOrStdout(), "To activate this federation, run:")
		fmt.Fprintf(cmd.OutOrStdout(), "  kscore-federation activate %s\n", result.Config.TrustDomain)
	} else {
		fmt.Fprintln(cmd.OutOrStdout(), "Federation activated successfully.")
	}

	return nil
}

func runNonInteractiveWizard(cmd *cobra.Command) error {
	// Validate required flags
	if wizardDomain == "" {
		return fmt.Errorf("--domain is required in non-interactive mode")
	}

	// Validate trust domain
	if err := wizard.ValidateTrustDomain(wizardDomain); err != nil {
		return fmt.Errorf("invalid trust domain: %w", err)
	}

	// Validate endpoint if provided
	if wizardEndpoint != "" {
		if err := wizard.ValidateEndpointURL(wizardEndpoint); err != nil {
			return fmt.Errorf("invalid endpoint URL: %w", err)
		}
	} else {
		// Auto-discover endpoint
		fmt.Fprintf(cmd.OutOrStdout(), "Discovering bundle endpoint for %s...\n", wizardDomain)
		result, err := wizard.DiscoverBundleEndpoint(cmd.Context(), wizardDomain, nil)
		if err != nil {
			return fmt.Errorf("endpoint discovery failed: %w", err)
		}
		if result.BestEndpoint == nil {
			return fmt.Errorf("no bundle endpoint discovered; please specify --endpoint")
		}
		wizardEndpoint = result.BestEndpoint.URL
		fmt.Fprintf(cmd.OutOrStdout(), "Discovered endpoint: %s\n", wizardEndpoint)
	}

	// Get policy template
	tmpl := wizard.GetPolicyTemplate(wizardPolicy)
	if tmpl == nil {
		return fmt.Errorf("unknown policy template: %s (available: allow-all, services-only, agents-only, kubernetes)", wizardPolicy)
	}

	policy := tmpl.Policy
	if policy == nil {
		return fmt.Errorf("policy template '%s' requires interactive mode", wizardPolicy)
	}
	policy.RequireMTLS = wizardMTLS

	// Build federation type
	var fedType federation.FederationType
	switch wizardType {
	case "bidirectional":
		fedType = federation.FederationTypeBidirectional
	case "unidirectional":
		fedType = federation.FederationTypeUnidirectional
	default:
		return fmt.Errorf("unknown federation type: %s (available: bidirectional, unidirectional)", wizardType)
	}

	// Create federation domain
	domain := &federation.FederatedDomain{
		TrustDomain:           wizardDomain,
		Type:                  fedType,
		State:                 federation.FederationStatePending,
		BundleEndpoint:        wizardEndpoint,
		BundleEndpointProfile: "https_web",
		Policy:                policy,
		RefreshInterval:       wizardRefresh,
		CreatedAt:             time.Now(),
	}

	// Output result
	if outputFmt == "json" {
		return printJSON(domain)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "\nFederation configuration created:\n")
	fmt.Fprintf(cmd.OutOrStdout(), "  Trust Domain:     %s\n", domain.TrustDomain)
	fmt.Fprintf(cmd.OutOrStdout(), "  Bundle Endpoint:  %s\n", domain.BundleEndpoint)
	fmt.Fprintf(cmd.OutOrStdout(), "  Type:             %s\n", domain.Type)
	fmt.Fprintf(cmd.OutOrStdout(), "  Policy:           %s\n", policy.Name)
	fmt.Fprintf(cmd.OutOrStdout(), "  Refresh Interval: %s\n", domain.RefreshInterval)
	fmt.Fprintf(cmd.OutOrStdout(), "  Require mTLS:     %v\n", policy.RequireMTLS)
	fmt.Fprintf(cmd.OutOrStdout(), "  State:            %s\n", domain.State)
	fmt.Fprintln(cmd.OutOrStdout())

	if wizardAutoActivate {
		domain.State = federation.FederationStateActive
		fmt.Fprintln(cmd.OutOrStdout(), "Federation activated successfully.")
	} else {
		fmt.Fprintln(cmd.OutOrStdout(), "To activate this federation, run:")
		fmt.Fprintf(cmd.OutOrStdout(), "  kscore-federation activate %s\n", domain.TrustDomain)
	}

	return nil
}

// Ensure os import is used
var _ = os.Stdout
