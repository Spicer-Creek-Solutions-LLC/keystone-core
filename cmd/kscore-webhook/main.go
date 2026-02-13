// Package main implements the kscore-webhook CLI for webhook handler management.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/shawnbutts/keystone-core/internal/cli/auditutil"
	clierrors "github.com/shawnbutts/keystone-core/internal/cli/errors"
	"github.com/shawnbutts/keystone-core/internal/cli/output"
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
		Use:   "kscore-webhook",
		Short: "Webhook management for Keystone Core",
		Long: `kscore-webhook manages webhook handlers for GitOps and external integrations.

This command provides comprehensive webhook management:
  - List and manage registered webhook handlers
  - Test webhooks with sample payloads
  - View webhook delivery history
  - Configure webhook secrets and authentication

Supported webhook sources:
  - ArgoCD (sync, health, deployment events)
  - Flux (reconciliation events)
  - GitHub (deployment, workflow, push events)
  - GitLab (deployment, pipeline, push events)

Commands:
  list       - List registered webhook handlers
  show       - Show details of a webhook handler
  test       - Send a test webhook
  history    - View webhook delivery history
  secrets    - Manage webhook secrets

Examples:
  # List all webhook handlers
  kscore-webhook list

  # Show details of a specific handler
  kscore-webhook show argocd

  # Test a webhook with sample payload
  kscore-webhook test argocd

  # View webhook delivery history
  kscore-webhook history --limit 50

  # Manage webhook secrets
  kscore-webhook secrets list`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	rootCmd.PersistentFlags().StringVarP(&serverAddr, "server", "s", "localhost:9090", "Control plane server address")
	rootCmd.PersistentFlags().StringVarP(&outputFormat, "format", "o", "table", "Output format (table, text, json, yaml)")
	rootCmd.PersistentFlags().StringVar(&auditLevel, "audit-level", "all", "Audit logging level (all, errors, none)")
	rootCmd.PersistentFlags().StringVar(&auditOutput, "audit-output", "auto", "Audit output backend (auto, syslog, journald, stderr, none)")

	rootCmd.AddCommand(
		newVersionCmd(),
		newListCommand(),
		newShowCommand(),
		newTestCommand(),
		newHistoryCommand(),
		newSecretsCommand(),
		newOutboundCommand(),
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
	auditHandler := auditutil.Attach(rootCmd, "kscore-webhook", &auditLevel, &auditOutput)
	if err := rootCmd.Execute(); err != nil {
		auditHandler.LogFailure(err)
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// ============================================================================
// List Command
// ============================================================================

func newListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List registered webhook handlers",
		Long: `List all registered webhook handlers.

Keystone Core supports webhooks from:
  - ArgoCD (sync, health, deployment events)
  - Flux (reconciliation events)
  - GitHub (deployment, workflow, push events)
  - GitLab (deployment, pipeline, push events)

Examples:
  # List all webhook handlers
  kscore-webhook list

  # List as JSON
  kscore-webhook list --format json`,
		RunE: runList,
	}

	return cmd
}

// WebhookHandler represents a registered webhook handler
type WebhookHandler struct {
	Type        string   `json:"type" yaml:"type"`
	Description string   `json:"description" yaml:"description"`
	Events      []string `json:"events" yaml:"events"`
	Endpoint    string   `json:"endpoint" yaml:"endpoint"`
	Enabled     bool     `json:"enabled" yaml:"enabled"`
	SecretRef   string   `json:"secret_ref,omitempty" yaml:"secret_ref,omitempty"`
}

func runList(cmd *cobra.Command, args []string) error {
	handlers := []WebhookHandler{
		{
			Type:        "argocd",
			Description: "ArgoCD application events",
			Events:      []string{"sync", "health", "deployment"},
			Endpoint:    "/webhooks/argocd",
			Enabled:     true,
			SecretRef:   "argocd-webhook-secret",
		},
		{
			Type:        "flux",
			Description: "Flux reconciliation events",
			Events:      []string{"kustomization", "helmrelease", "gitrepository"},
			Endpoint:    "/webhooks/flux",
			Enabled:     true,
			SecretRef:   "flux-webhook-secret",
		},
		{
			Type:        "github",
			Description: "GitHub repository events",
			Events:      []string{"deployment", "workflow_run", "push"},
			Endpoint:    "/webhooks/github",
			Enabled:     true,
			SecretRef:   "github-webhook-secret",
		},
		{
			Type:        "gitlab",
			Description: "GitLab project events",
			Events:      []string{"deployment", "pipeline", "push"},
			Endpoint:    "/webhooks/gitlab",
			Enabled:     true,
			SecretRef:   "gitlab-webhook-secret",
		},
	}

	format, err := output.ParseFormat(outputFormat)
	if err != nil {
		return err
	}

	switch format {
	case output.FormatJSON:
		return output.WriteJSON(cmd.OutOrStdout(), handlers)
	case output.FormatYAML:
		return output.WriteYAML(cmd.OutOrStdout(), handlers)
	case output.FormatTable:
		table := buildHandlersTable(handlers)
		if err := output.WriteTable(cmd.OutOrStdout(), table); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout())
		fmt.Fprintln(cmd.OutOrStdout(), "Webhook Base URL: http://<server>:<port>")
		fmt.Fprintln(cmd.OutOrStdout(), "\nNote: Configure webhooks on the control plane server.")
		fmt.Fprintln(cmd.OutOrStdout(), "Use the server's webhook.port and webhook.path settings.")
	case output.FormatText:
		fmt.Fprintln(cmd.OutOrStdout(), "Registered Webhook Handlers")
		fmt.Fprintln(cmd.OutOrStdout(), "===========================")
		fmt.Fprintln(cmd.OutOrStdout())

		for _, h := range handlers {
			status := "enabled"
			if !h.Enabled {
				status = "disabled"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s (%s)\n", strings.ToUpper(h.Type), status)
			fmt.Fprintf(cmd.OutOrStdout(), "  Description: %s\n", h.Description)
			fmt.Fprintf(cmd.OutOrStdout(), "  Events:      %s\n", strings.Join(h.Events, ", "))
			fmt.Fprintf(cmd.OutOrStdout(), "  Endpoint:    %s\n", h.Endpoint)
			if h.SecretRef != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "  Secret:      %s\n", h.SecretRef)
			}
			fmt.Fprintln(cmd.OutOrStdout())
		}

		fmt.Fprintln(cmd.OutOrStdout(), "Webhook Base URL: http://<server>:<port>")
		fmt.Fprintln(cmd.OutOrStdout(), "\nNote: Configure webhooks on the control plane server.")
		fmt.Fprintln(cmd.OutOrStdout(), "Use the server's webhook.port and webhook.path settings.")
	default:
		return clierrors.New(clierrors.KindInvalidArgument, fmt.Sprintf("unsupported output format: %s", outputFormat))
	}

	return nil
}

func buildHandlersTable(handlers []WebhookHandler) *output.Table {
	rows := make([][]string, 0, len(handlers))
	for _, h := range handlers {
		status := "enabled"
		if !h.Enabled {
			status = "disabled"
		}
		rows = append(rows, []string{
			h.Type,
			h.Description,
			strings.Join(h.Events, ", "),
			h.Endpoint,
			status,
		})
	}

	return &output.Table{
		Headers: []string{"TYPE", "DESCRIPTION", "EVENTS", "ENDPOINT", "STATUS"},
		Rows:    rows,
	}
}

// ============================================================================
// Show Command
// ============================================================================

func newShowCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <type>",
		Short: "Show details of a webhook handler",
		Long: `Display detailed information about a specific webhook handler.

Webhook types: argocd, flux, github, gitlab

Examples:
  # Show ArgoCD webhook details
  kscore-webhook show argocd

  # Show GitHub webhook details as JSON
  kscore-webhook show github --format json`,
		Args: cobra.ExactArgs(1),
		RunE: runShow,
	}

	return cmd
}

func runShow(cmd *cobra.Command, args []string) error {
	webhookType := strings.ToLower(args[0])

	// Get handler details
	handler, err := getHandlerDetails(webhookType)
	if err != nil {
		return err
	}

	format, err := output.ParseFormat(outputFormat)
	if err != nil {
		return err
	}

	switch format {
	case output.FormatJSON:
		return output.WriteJSON(cmd.OutOrStdout(), handler)
	case output.FormatYAML:
		return output.WriteYAML(cmd.OutOrStdout(), handler)
	case output.FormatTable, output.FormatText:
		fmt.Fprintf(cmd.OutOrStdout(), "Webhook Handler: %s\n", strings.ToUpper(handler.Type))
		fmt.Fprintln(cmd.OutOrStdout(), strings.Repeat("=", 40))
		fmt.Fprintln(cmd.OutOrStdout())
		fmt.Fprintf(cmd.OutOrStdout(), "Description:  %s\n", handler.Description)
		fmt.Fprintf(cmd.OutOrStdout(), "Endpoint:     %s\n", handler.Endpoint)
		fmt.Fprintf(cmd.OutOrStdout(), "Status:       %s\n", func() string {
			if handler.Enabled {
				return "enabled"
			}
			return "disabled"
		}())
		fmt.Fprintf(cmd.OutOrStdout(), "Secret:       %s\n", handler.SecretRef)
		fmt.Fprintln(cmd.OutOrStdout())
		fmt.Fprintln(cmd.OutOrStdout(), "Supported Events:")
		for _, event := range handler.Events {
			fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", event)
		}
		fmt.Fprintln(cmd.OutOrStdout())
		fmt.Fprintln(cmd.OutOrStdout(), "Configuration:")
		fmt.Fprintln(cmd.OutOrStdout(), "  To receive webhooks from "+handler.Type+", configure your")
		fmt.Fprintln(cmd.OutOrStdout(), "  integration to POST to:")
		fmt.Fprintf(cmd.OutOrStdout(), "    http://<server>:<port>%s\n", handler.Endpoint)
	default:
		return clierrors.New(clierrors.KindInvalidArgument, fmt.Sprintf("unsupported output format: %s", outputFormat))
	}

	return nil
}

func getHandlerDetails(webhookType string) (*WebhookHandler, error) {
	handlers := map[string]*WebhookHandler{
		"argocd": {
			Type:        "argocd",
			Description: "ArgoCD application events",
			Events:      []string{"sync", "health", "deployment", "resourceUpdated", "resourceCreated", "resourceDeleted"},
			Endpoint:    "/webhooks/argocd",
			Enabled:     true,
			SecretRef:   "argocd-webhook-secret",
		},
		"flux": {
			Type:        "flux",
			Description: "Flux reconciliation events",
			Events:      []string{"kustomization", "helmrelease", "gitrepository", "ocirepository", "helmrepository"},
			Endpoint:    "/webhooks/flux",
			Enabled:     true,
			SecretRef:   "flux-webhook-secret",
		},
		"github": {
			Type:        "github",
			Description: "GitHub repository events",
			Events:      []string{"deployment", "deployment_status", "workflow_run", "workflow_job", "push", "pull_request"},
			Endpoint:    "/webhooks/github",
			Enabled:     true,
			SecretRef:   "github-webhook-secret",
		},
		"gitlab": {
			Type:        "gitlab",
			Description: "GitLab project events",
			Events:      []string{"deployment", "pipeline", "push", "merge_request", "tag_push"},
			Endpoint:    "/webhooks/gitlab",
			Enabled:     true,
			SecretRef:   "gitlab-webhook-secret",
		},
	}

	handler, ok := handlers[webhookType]
	if !ok {
		return nil, clierrors.New(clierrors.KindInvalidArgument,
			fmt.Sprintf("unknown webhook type: %s (use: argocd, flux, github, gitlab)", webhookType))
	}

	return handler, nil
}

// ============================================================================
// Test Command
// ============================================================================

func newTestCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "test <type>",
		Short: "Send a test webhook",
		Long: `Send a test webhook to verify handler configuration.

This command generates a sample payload for the specified webhook type
and shows how to test the webhook endpoint.

Webhook types: argocd, flux, github, gitlab

Examples:
  # Test ArgoCD webhook
  kscore-webhook test argocd

  # Test GitHub webhook
  kscore-webhook test github`,
		Args: cobra.ExactArgs(1),
		RunE: runTest,
	}

	return cmd
}

func runTest(cmd *cobra.Command, args []string) error {
	webhookType := strings.ToLower(args[0])

	// Validate type
	switch webhookType {
	case "argocd", "flux", "github", "gitlab":
		// Valid
	default:
		return clierrors.New(clierrors.KindInvalidArgument,
			fmt.Sprintf("invalid webhook type: %s (use: argocd, flux, github, gitlab)", webhookType))
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Test Webhook: %s\n", strings.ToUpper(webhookType))
	fmt.Fprintln(cmd.OutOrStdout())

	// Generate sample payload
	payload := generateSamplePayload(webhookType)

	fmt.Fprintln(cmd.OutOrStdout(), "Sample Payload:")
	fmt.Fprintln(cmd.OutOrStdout(), strings.Repeat("-", 40))
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	enc.Encode(payload)
	fmt.Fprintln(cmd.OutOrStdout())

	fmt.Fprintln(cmd.OutOrStdout(), "To test this webhook, run:")
	fmt.Fprintln(cmd.OutOrStdout(), strings.Repeat("-", 40))
	fmt.Fprintf(cmd.OutOrStdout(), "curl -X POST http://<server>:<port>/webhooks/%s \\\n", webhookType)
	fmt.Fprintln(cmd.OutOrStdout(), "  -H 'Content-Type: application/json' \\")
	fmt.Fprintln(cmd.OutOrStdout(), "  -d '<payload>'")
	fmt.Fprintln(cmd.OutOrStdout())
	fmt.Fprintln(cmd.OutOrStdout(), "Note: Replace <server>:<port> with your control plane address.")
	fmt.Fprintln(cmd.OutOrStdout(), "Add appropriate authentication headers based on your webhook secret.")

	return nil
}

func generateSamplePayload(webhookType string) map[string]interface{} {
	switch webhookType {
	case "argocd":
		return map[string]interface{}{
			"type": "sync",
			"application": map[string]interface{}{
				"name":      "test-app",
				"namespace": "argocd",
				"project":   "default",
			},
			"status": map[string]interface{}{
				"sync": map[string]interface{}{
					"status":   "Synced",
					"revision": "abc123",
				},
				"health": map[string]interface{}{
					"status": "Healthy",
				},
			},
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		}
	case "flux":
		return map[string]interface{}{
			"involvedObject": map[string]interface{}{
				"kind":      "Kustomization",
				"name":      "test-kustomization",
				"namespace": "flux-system",
			},
			"severity":            "info",
			"timestamp":           time.Now().UTC().Format(time.RFC3339),
			"message":             "Reconciliation succeeded",
			"reason":              "ReconciliationSucceeded",
			"reportingController": "kustomize-controller",
		}
	case "github":
		return map[string]interface{}{
			"action": "completed",
			"deployment": map[string]interface{}{
				"id":          12345,
				"environment": "production",
				"sha":         "abc123def456",
				"ref":         "main",
			},
			"repository": map[string]interface{}{
				"full_name": "org/repo",
				"html_url":  "https://github.com/org/repo",
			},
			"sender": map[string]interface{}{
				"login": "deploy-bot",
			},
		}
	case "gitlab":
		return map[string]interface{}{
			"object_kind": "deployment",
			"status":      "success",
			"project": map[string]interface{}{
				"id":                  12345,
				"path_with_namespace": "org/repo",
				"web_url":             "https://gitlab.com/org/repo",
			},
			"environment": "production",
			"deployable": map[string]interface{}{
				"id":     67890,
				"status": "success",
			},
			"commit": map[string]interface{}{
				"sha": "abc123def456",
			},
		}
	default:
		return map[string]interface{}{}
	}
}

// ============================================================================
// History Command
// ============================================================================

var historyLimit int

func newHistoryCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "history",
		Short: "View webhook delivery history",
		Long: `Display the history of webhook deliveries.

Shows recent webhook events including:
  - Delivery timestamp
  - Webhook type and event
  - Delivery status
  - Response time

Examples:
  # View recent webhook deliveries
  kscore-webhook history

  # Limit results
  kscore-webhook history --limit 50

  # Output as JSON
  kscore-webhook history --format json`,
		RunE: runHistory,
	}

	cmd.Flags().IntVar(&historyLimit, "limit", 20, "Maximum entries to show")

	return cmd
}

// WebhookDelivery represents a webhook delivery record
type WebhookDelivery struct {
	ID           string    `json:"id" yaml:"id"`
	Type         string    `json:"type" yaml:"type"`
	Event        string    `json:"event" yaml:"event"`
	Status       string    `json:"status" yaml:"status"`
	StatusCode   int       `json:"status_code" yaml:"status_code"`
	ResponseTime string    `json:"response_time" yaml:"response_time"`
	Timestamp    time.Time `json:"timestamp" yaml:"timestamp"`
	Source       string    `json:"source" yaml:"source"`
}

func runHistory(cmd *cobra.Command, args []string) error {
	// Sample delivery history (in production, would query control plane)
	deliveries := []WebhookDelivery{
		{
			ID:           "wh-001",
			Type:         "argocd",
			Event:        "sync",
			Status:       "success",
			StatusCode:   200,
			ResponseTime: "45ms",
			Timestamp:    time.Now().Add(-10 * time.Minute),
			Source:       "argocd-server",
		},
		{
			ID:           "wh-002",
			Type:         "github",
			Event:        "deployment",
			Status:       "success",
			StatusCode:   200,
			ResponseTime: "120ms",
			Timestamp:    time.Now().Add(-30 * time.Minute),
			Source:       "github.com/org/repo",
		},
		{
			ID:           "wh-003",
			Type:         "flux",
			Event:        "kustomization",
			Status:       "success",
			StatusCode:   200,
			ResponseTime: "35ms",
			Timestamp:    time.Now().Add(-1 * time.Hour),
			Source:       "flux-system/app-kustomization",
		},
		{
			ID:           "wh-004",
			Type:         "gitlab",
			Event:        "pipeline",
			Status:       "failed",
			StatusCode:   500,
			ResponseTime: "250ms",
			Timestamp:    time.Now().Add(-2 * time.Hour),
			Source:       "gitlab.com/org/repo",
		},
	}

	// Apply limit
	if len(deliveries) > historyLimit {
		deliveries = deliveries[:historyLimit]
	}

	format, err := output.ParseFormat(outputFormat)
	if err != nil {
		return err
	}

	switch format {
	case output.FormatJSON:
		return output.WriteJSON(cmd.OutOrStdout(), deliveries)
	case output.FormatYAML:
		return output.WriteYAML(cmd.OutOrStdout(), deliveries)
	case output.FormatTable:
		if len(deliveries) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "No webhook deliveries found.")
			return nil
		}
		table := buildDeliveryTable(deliveries)
		if err := output.WriteTable(cmd.OutOrStdout(), table); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "\nTotal: %d deliveries\n", len(deliveries))
		fmt.Fprintln(cmd.OutOrStdout(), "\nNote: This CLI shows sample data.")
		fmt.Fprintln(cmd.OutOrStdout(), "For real delivery history, connect to the control plane API.")
	case output.FormatText:
		if len(deliveries) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "No webhook deliveries found.")
			return nil
		}

		fmt.Fprintln(cmd.OutOrStdout(), "Webhook Delivery History")
		fmt.Fprintln(cmd.OutOrStdout(), strings.Repeat("=", 100))
		fmt.Fprintln(cmd.OutOrStdout())

		for i := range deliveries {
			d := &deliveries[i]
			statusIcon := "✓"
			if d.Status != "success" {
				statusIcon = "✗"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s [%s] %s/%s\n", statusIcon, d.Timestamp.Format("2006-01-02 15:04:05"), d.Type, d.Event)
			fmt.Fprintf(cmd.OutOrStdout(), "   ID: %s  Status: %d  Time: %s  Source: %s\n", d.ID, d.StatusCode, d.ResponseTime, d.Source)
			fmt.Fprintln(cmd.OutOrStdout())
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Total: %d deliveries\n", len(deliveries))
		fmt.Fprintln(cmd.OutOrStdout(), "\nNote: This CLI shows sample data.")
		fmt.Fprintln(cmd.OutOrStdout(), "For real delivery history, connect to the control plane API.")
	default:
		return clierrors.New(clierrors.KindInvalidArgument, fmt.Sprintf("unsupported output format: %s", outputFormat))
	}

	return nil
}

func buildDeliveryTable(deliveries []WebhookDelivery) *output.Table {
	rows := make([][]string, 0, len(deliveries))
	for i := range deliveries {
		d := &deliveries[i]
		rows = append(rows, []string{
			d.Timestamp.Format("2006-01-02 15:04:05"),
			d.Type,
			d.Event,
			d.Status,
			fmt.Sprintf("%d", d.StatusCode),
			d.ResponseTime,
		})
	}

	return &output.Table{
		Headers: []string{"TIMESTAMP", "TYPE", "EVENT", "STATUS", "CODE", "TIME"},
		Rows:    rows,
	}
}

// ============================================================================
// Secrets Command
// ============================================================================

func newSecretsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "secrets",
		Short: "Manage webhook secrets",
		Long:  `Manage secrets used for webhook authentication and validation.`,
	}

	cmd.AddCommand(newSecretsListCmd())
	cmd.AddCommand(newSecretsRotateCmd())

	return cmd
}

func newSecretsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List webhook secrets",
		Long: `List all webhook secrets.

Examples:
  # List webhook secrets
  kscore-webhook secrets list`,
		RunE: runSecretsList,
	}
}

// WebhookSecret represents a webhook secret
type WebhookSecret struct {
	Name        string    `json:"name" yaml:"name"`
	Type        string    `json:"type" yaml:"type"`
	CreatedAt   time.Time `json:"created_at" yaml:"created_at"`
	LastRotated time.Time `json:"last_rotated" yaml:"last_rotated"`
	ExpiresAt   time.Time `json:"expires_at,omitempty" yaml:"expires_at,omitempty"`
}

func runSecretsList(cmd *cobra.Command, args []string) error {
	secrets := []WebhookSecret{
		{
			Name:        "argocd-webhook-secret",
			Type:        "argocd",
			CreatedAt:   time.Now().AddDate(0, -3, 0),
			LastRotated: time.Now().AddDate(0, -1, 0),
		},
		{
			Name:        "flux-webhook-secret",
			Type:        "flux",
			CreatedAt:   time.Now().AddDate(0, -3, 0),
			LastRotated: time.Now().AddDate(0, -1, 0),
		},
		{
			Name:        "github-webhook-secret",
			Type:        "github",
			CreatedAt:   time.Now().AddDate(0, -3, 0),
			LastRotated: time.Now().AddDate(0, 0, -15),
		},
		{
			Name:        "gitlab-webhook-secret",
			Type:        "gitlab",
			CreatedAt:   time.Now().AddDate(0, -3, 0),
			LastRotated: time.Now().AddDate(0, 0, -15),
		},
	}

	format, err := output.ParseFormat(outputFormat)
	if err != nil {
		return err
	}

	switch format {
	case output.FormatJSON:
		return output.WriteJSON(cmd.OutOrStdout(), secrets)
	case output.FormatYAML:
		return output.WriteYAML(cmd.OutOrStdout(), secrets)
	case output.FormatTable:
		table := buildSecretsTable(secrets)
		if err := output.WriteTable(cmd.OutOrStdout(), table); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout())
		fmt.Fprintln(cmd.OutOrStdout(), "Note: Secret values are not displayed for security.")
		fmt.Fprintln(cmd.OutOrStdout(), "Use 'kscore-webhook secrets rotate <name>' to rotate a secret.")
	case output.FormatText:
		fmt.Fprintln(cmd.OutOrStdout(), "Webhook Secrets")
		fmt.Fprintln(cmd.OutOrStdout(), strings.Repeat("=", 60))
		fmt.Fprintln(cmd.OutOrStdout())

		for _, s := range secrets {
			fmt.Fprintf(cmd.OutOrStdout(), "Name:         %s\n", s.Name)
			fmt.Fprintf(cmd.OutOrStdout(), "Type:         %s\n", s.Type)
			fmt.Fprintf(cmd.OutOrStdout(), "Created:      %s\n", s.CreatedAt.Format("2006-01-02"))
			fmt.Fprintf(cmd.OutOrStdout(), "Last Rotated: %s\n", s.LastRotated.Format("2006-01-02"))
			fmt.Fprintln(cmd.OutOrStdout())
		}

		fmt.Fprintln(cmd.OutOrStdout(), "Note: Secret values are not displayed for security.")
		fmt.Fprintln(cmd.OutOrStdout(), "Use 'kscore-webhook secrets rotate <name>' to rotate a secret.")
	default:
		return clierrors.New(clierrors.KindInvalidArgument, fmt.Sprintf("unsupported output format: %s", outputFormat))
	}

	return nil
}

func buildSecretsTable(secrets []WebhookSecret) *output.Table {
	rows := make([][]string, 0, len(secrets))
	for _, s := range secrets {
		rows = append(rows, []string{
			s.Name,
			s.Type,
			s.CreatedAt.Format("2006-01-02"),
			s.LastRotated.Format("2006-01-02"),
		})
	}

	return &output.Table{
		Headers: []string{"NAME", "TYPE", "CREATED", "LAST ROTATED"},
		Rows:    rows,
	}
}

func newSecretsRotateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rotate <name>",
		Short: "Rotate a webhook secret",
		Long: `Rotate a webhook secret to generate a new value.

Examples:
  # Rotate the GitHub webhook secret
  kscore-webhook secrets rotate github-webhook-secret`,
		Args: cobra.ExactArgs(1),
		RunE: runSecretsRotate,
	}
}

func runSecretsRotate(cmd *cobra.Command, args []string) error {
	secretName := args[0]

	fmt.Fprintf(cmd.OutOrStdout(), "Rotating secret: %s\n", secretName)
	fmt.Fprintln(cmd.OutOrStdout())

	// In production, this would connect to the control plane
	fmt.Fprintln(cmd.OutOrStdout(), "Secret rotation would be performed here.")
	fmt.Fprintln(cmd.OutOrStdout())
	fmt.Fprintln(cmd.OutOrStdout(), "Note: This CLI shows sample output.")
	fmt.Fprintln(cmd.OutOrStdout(), "For actual secret rotation, connect to the control plane API.")
	fmt.Fprintln(cmd.OutOrStdout())
	fmt.Fprintln(cmd.OutOrStdout(), "After rotating, update your webhook configuration with the new secret.")

	return nil
}

// ============================================================================
// Outbound Command Group
// ============================================================================

func newOutboundCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "outbound",
		Short: "Manage outbound webhook subscriptions",
		Long: `Manage outbound webhook subscriptions that deliver Keystone Core events
to external HTTP endpoints.

Outbound webhooks allow external systems to receive notifications when events
occur in Keystone Core. Each subscription filters events by type pattern and
delivers matching events with HMAC-SHA256 signing.

Commands:
  list     - List outbound webhook subscriptions
  create   - Create a new subscription
  show     - Show subscription details
  delete   - Delete a subscription
  history  - View delivery history for a subscription
  test     - Send a test event to a subscription`,
	}

	cmd.AddCommand(
		newOutboundListCmd(),
		newOutboundCreateCmd(),
		newOutboundShowCmd(),
		newOutboundDeleteCmd(),
		newOutboundHistoryCmd(),
		newOutboundTestCmd(),
	)
	return cmd
}

func outboundURL(pathSuffix string) string {
	return fmt.Sprintf("http://%s/api/v1/webhooks/subscriptions%s", serverAddr, pathSuffix)
}

func newOutboundListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List outbound webhook subscriptions",
		RunE: func(cmd *cobra.Command, args []string) error {
			req, err := http.NewRequestWithContext(cmd.Context(), http.MethodGet, outboundURL(""), http.NoBody)
			if err != nil {
				return err
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return fmt.Errorf("request failed: %w", err)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)

			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
			}

			format, fmtErr := output.ParseFormat(outputFormat)
			if fmtErr != nil {
				return fmtErr
			}

			switch format {
			case output.FormatJSON:
				cmd.OutOrStdout().Write(body)
				fmt.Fprintln(cmd.OutOrStdout())
			case output.FormatYAML:
				var data interface{}
				json.Unmarshal(body, &data)
				return output.WriteYAML(cmd.OutOrStdout(), data)
			default:
				var subs []map[string]interface{}
				if err := json.Unmarshal(body, &subs); err != nil {
					return err
				}
				if len(subs) == 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "No outbound webhook subscriptions found.")
					return nil
				}
				rows := make([][]string, 0, len(subs))
				for _, s := range subs {
					enabled := "yes"
					if e, ok := s["enabled"].(bool); ok && !e {
						enabled = "no"
					}
					events := ""
					if evts, ok := s["events"].([]interface{}); ok {
						parts := make([]string, len(evts))
						for i, e := range evts {
							parts[i] = fmt.Sprint(e)
						}
						events = strings.Join(parts, ", ")
					}
					id, _ := s["id"].(string)
					name, _ := s["name"].(string)
					url, _ := s["url"].(string)
					rows = append(rows, []string{id, name, url, events, enabled})
				}
				table := &output.Table{
					Headers: []string{"ID", "NAME", "URL", "EVENTS", "ENABLED"},
					Rows:    rows,
				}
				return output.WriteTable(cmd.OutOrStdout(), table)
			}
			return nil
		},
	}
}

var (
	outboundCreateName       string
	outboundCreateURL        string
	outboundCreateEvents     []string
	outboundCreateSecret     string
	outboundCreateMaxRetries int
	outboundCreateTimeout    int
)

func newOutboundCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an outbound webhook subscription",
		Long: `Create a new outbound webhook subscription.

Examples:
  kscore-webhook outbound create --name slack-alerts --url https://hooks.slack.com/xxx --events "agent.*"
  kscore-webhook outbound create --name pagerduty --url https://events.pagerduty.com/xxx --events "state.drift" --secret mysecret`,
		RunE: func(cmd *cobra.Command, args []string) error {
			payload := map[string]interface{}{
				"name":   outboundCreateName,
				"url":    outboundCreateURL,
				"events": outboundCreateEvents,
			}
			if outboundCreateSecret != "" {
				payload["secret"] = outboundCreateSecret
			}
			if outboundCreateMaxRetries >= 0 {
				payload["max_retries"] = outboundCreateMaxRetries
			}
			if outboundCreateTimeout > 0 {
				payload["timeout_secs"] = outboundCreateTimeout
			}

			data, _ := json.Marshal(payload)
			req, err := http.NewRequestWithContext(cmd.Context(), http.MethodPost, outboundURL(""), bytes.NewReader(data))
			if err != nil {
				return err
			}
			req.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return fmt.Errorf("request failed: %w", err)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)

			if resp.StatusCode != http.StatusCreated {
				return fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
			}

			var result map[string]interface{}
			json.Unmarshal(body, &result)

			format, fmtErr := output.ParseFormat(outputFormat)
			if fmtErr != nil {
				return fmtErr
			}
			switch format {
			case output.FormatJSON:
				cmd.OutOrStdout().Write(body)
				fmt.Fprintln(cmd.OutOrStdout())
			case output.FormatYAML:
				return output.WriteYAML(cmd.OutOrStdout(), result)
			default:
				fmt.Fprintf(cmd.OutOrStdout(), "Created subscription: %s\n", result["id"])
				fmt.Fprintf(cmd.OutOrStdout(), "  Name:   %s\n", result["name"])
				fmt.Fprintf(cmd.OutOrStdout(), "  URL:    %s\n", result["url"])
				fmt.Fprintf(cmd.OutOrStdout(), "  Events: %v\n", result["events"])
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&outboundCreateName, "name", "", "Subscription name (required)")
	cmd.Flags().StringVar(&outboundCreateURL, "url", "", "Webhook endpoint URL (required)")
	cmd.Flags().StringSliceVar(&outboundCreateEvents, "events", nil, "Event type patterns (required, e.g. agent.*)")
	cmd.Flags().StringVar(&outboundCreateSecret, "secret", "", "HMAC signing secret")
	cmd.Flags().IntVar(&outboundCreateMaxRetries, "max-retries", 3, "Maximum delivery retries")
	cmd.Flags().IntVar(&outboundCreateTimeout, "timeout", 10, "HTTP timeout in seconds")
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("url")
	_ = cmd.MarkFlagRequired("events")

	return cmd
}

func newOutboundShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Short: "Show outbound webhook subscription details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			req, err := http.NewRequestWithContext(cmd.Context(), http.MethodGet, outboundURL("/"+args[0]), http.NoBody)
			if err != nil {
				return err
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return fmt.Errorf("request failed: %w", err)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)

			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
			}

			format, fmtErr := output.ParseFormat(outputFormat)
			if fmtErr != nil {
				return fmtErr
			}
			switch format {
			case output.FormatJSON:
				cmd.OutOrStdout().Write(body)
				fmt.Fprintln(cmd.OutOrStdout())
			case output.FormatYAML:
				var data interface{}
				json.Unmarshal(body, &data)
				return output.WriteYAML(cmd.OutOrStdout(), data)
			default:
				var sub map[string]interface{}
				json.Unmarshal(body, &sub)
				fmt.Fprintf(cmd.OutOrStdout(), "ID:      %s\n", sub["id"])
				fmt.Fprintf(cmd.OutOrStdout(), "Name:    %s\n", sub["name"])
				fmt.Fprintf(cmd.OutOrStdout(), "URL:     %s\n", sub["url"])
				fmt.Fprintf(cmd.OutOrStdout(), "Events:  %v\n", sub["events"])
				fmt.Fprintf(cmd.OutOrStdout(), "Enabled: %v\n", sub["enabled"])
				fmt.Fprintf(cmd.OutOrStdout(), "Secret:  %s\n", sub["secret"])
				fmt.Fprintf(cmd.OutOrStdout(), "Retries: %v\n", sub["max_retries"])
				fmt.Fprintf(cmd.OutOrStdout(), "Timeout: %vs\n", sub["timeout_secs"])
			}
			return nil
		},
	}
}

func newOutboundDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete an outbound webhook subscription",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			req, err := http.NewRequestWithContext(cmd.Context(), http.MethodDelete, outboundURL("/"+args[0]), http.NoBody)
			if err != nil {
				return err
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return fmt.Errorf("request failed: %w", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusNoContent {
				fmt.Fprintf(cmd.OutOrStdout(), "Deleted subscription: %s\n", args[0])
				return nil
			}
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
		},
	}
}

var outboundHistoryLimit int

func newOutboundHistoryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "history <id>",
		Short: "View delivery history for an outbound webhook subscription",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			historyURL := fmt.Sprintf("%s/%s/deliveries?limit=%d", outboundURL(""), args[0], outboundHistoryLimit)
			req, err := http.NewRequestWithContext(cmd.Context(), http.MethodGet, historyURL, http.NoBody)
			if err != nil {
				return err
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return fmt.Errorf("request failed: %w", err)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)

			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
			}

			format, fmtErr := output.ParseFormat(outputFormat)
			if fmtErr != nil {
				return fmtErr
			}
			switch format {
			case output.FormatJSON:
				cmd.OutOrStdout().Write(body)
				fmt.Fprintln(cmd.OutOrStdout())
			case output.FormatYAML:
				var data interface{}
				json.Unmarshal(body, &data)
				return output.WriteYAML(cmd.OutOrStdout(), data)
			default:
				var records []map[string]interface{}
				json.Unmarshal(body, &records)
				if len(records) == 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "No delivery history found.")
					return nil
				}
				rows := make([][]string, 0, len(records))
				for _, r := range records {
					rows = append(rows, []string{
						fmt.Sprint(r["id"]),
						fmt.Sprint(r["event_type"]),
						fmt.Sprint(r["status"]),
						fmt.Sprint(r["status_code"]),
						fmt.Sprint(r["attempt"]),
						fmt.Sprint(r["delivered_at"]),
					})
				}
				table := &output.Table{
					Headers: []string{"ID", "EVENT TYPE", "STATUS", "CODE", "ATTEMPT", "DELIVERED AT"},
					Rows:    rows,
				}
				return output.WriteTable(cmd.OutOrStdout(), table)
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&outboundHistoryLimit, "limit", 50, "Maximum delivery records to show")
	return cmd
}

func newOutboundTestCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "test <id>",
		Short: "Send a test event to an outbound webhook subscription",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			req, err := http.NewRequestWithContext(cmd.Context(), http.MethodPost, outboundURL("/"+args[0]+"/test"), http.NoBody)
			if err != nil {
				return err
			}
			req.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return fmt.Errorf("request failed: %w", err)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)

			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
			}

			var result map[string]interface{}
			json.Unmarshal(body, &result)

			if result["success"] == true {
				fmt.Fprintf(cmd.OutOrStdout(), "Test delivery successful (HTTP %v)\n", result["status_code"])
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Test delivery failed: %s\n", result["error"])
			}
			return nil
		},
	}
}
