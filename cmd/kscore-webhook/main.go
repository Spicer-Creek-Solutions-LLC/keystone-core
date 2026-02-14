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

func runList(_ *cobra.Command, _ []string) error {
	return fmt.Errorf("inbound webhook listing API not yet available — server-side webhook management endpoints required")
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

func runShow(_ *cobra.Command, _ []string) error {
	return fmt.Errorf("inbound webhook details API not yet available — server-side webhook management endpoints required")
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

	switch webhookType {
	case "argocd", "flux", "github", "gitlab":
	default:
		return clierrors.New(clierrors.KindInvalidArgument,
			fmt.Sprintf("invalid webhook type: %s (use: argocd, flux, github, gitlab)", webhookType))
	}

	payload := map[string]interface{}{
		"type":      webhookType,
		"test":      true,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"source":    "kscore-webhook test",
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal test payload: %w", err)
	}

	url := fmt.Sprintf("http://%s/webhooks/%s", serverAddr, webhookType)
	fmt.Fprintf(cmd.OutOrStdout(), "Sending test webhook to %s\n", url)

	httpReq, err := http.NewRequestWithContext(cmd.Context(), http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to send test webhook to %s: %w", url, err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	fmt.Fprintf(cmd.OutOrStdout(), "Response: %d %s\n", resp.StatusCode, http.StatusText(resp.StatusCode))
	if len(respBody) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "Body: %s\n", string(respBody))
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("test webhook returned error status %d", resp.StatusCode)
	}

	fmt.Fprintln(cmd.OutOrStdout(), "Test webhook delivered successfully.")
	return nil
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

func runHistory(_ *cobra.Command, _ []string) error {
	return fmt.Errorf("inbound webhook delivery history API not yet available — server-side webhook tracking endpoints required")
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
		RunE: func(_ *cobra.Command, _ []string) error {
			return fmt.Errorf("webhook secrets listing API not yet available — server-side webhook secret management endpoints required")
		},
	}
}

func newSecretsRotateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rotate <name>",
		Short: "Rotate a webhook secret",
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, _ []string) error {
			return fmt.Errorf("webhook secret rotation API not yet available — server-side webhook secret management endpoints required")
		},
	}
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
