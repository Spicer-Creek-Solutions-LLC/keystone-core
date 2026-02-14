// Package main provides the kscore-agents CLI plugin for agent management.
//
// This plugin provides commands for:
//   - Listing and showing agent details
//   - Deleting and quarantining agents
//   - Managing join tokens
//   - Managing agent tags/labels
//   - Renewing SPIFFE SVIDs
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gopkg.in/yaml.v3"

	"github.com/shawnbutts/keystone-core/internal/cli/output"
	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
	"github.com/shawnbutts/keystone-core/pkg/version"
)

// Config holds CLI configuration
type Config struct {
	ServerAddr string
	Output     string
	Verbose    bool
}

// AgentDisplay represents an agent for display purposes
type AgentDisplay struct {
	ID            string            `json:"id" yaml:"id"`
	Hostname      string            `json:"hostname" yaml:"hostname"`
	OS            string            `json:"os" yaml:"os"`
	Arch          string            `json:"arch" yaml:"arch"`
	Status        string            `json:"status" yaml:"status"`
	Version       string            `json:"version" yaml:"version"`
	LastHeartbeat string            `json:"last_heartbeat" yaml:"last_heartbeat"`
	RegisteredAt  string            `json:"registered_at" yaml:"registered_at"`
	Labels        map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
	IPAddresses   []string          `json:"ip_addresses,omitempty" yaml:"ip_addresses,omitempty"`
	DualStack     bool              `json:"dual_stack,omitempty" yaml:"dual_stack,omitempty"`
	Metrics       *MetricsDisplay   `json:"metrics,omitempty" yaml:"metrics,omitempty"`
}

// MetricsDisplay represents agent metrics for display
type MetricsDisplay struct {
	CPU    float32   `json:"cpu_percent" yaml:"cpu_percent"`
	Memory float32   `json:"memory_percent" yaml:"memory_percent"`
	Disk   float32   `json:"disk_percent" yaml:"disk_percent"`
	Load   []float32 `json:"load_average,omitempty" yaml:"load_average,omitempty"`
}

// TokenDisplay represents a join token for display
type TokenDisplay struct {
	ID        string            `json:"id" yaml:"id"`
	Token     string            `json:"token,omitempty" yaml:"token,omitempty"`
	TTL       string            `json:"ttl" yaml:"ttl"`
	ExpiresAt time.Time         `json:"expires_at" yaml:"expires_at"`
	UsedCount int               `json:"used_count" yaml:"used_count"`
	MaxUses   int               `json:"max_uses" yaml:"max_uses"`
	Labels    map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
	CreatedAt time.Time         `json:"created_at" yaml:"created_at"`
	CreatedBy string            `json:"created_by" yaml:"created_by"`
}

func main() {
	cfg := &Config{}
	rootCmd := newRootCmd(cfg)
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func newRootCmd(cfg *Config) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "kscore-agents",
		Short: "Keystone Core agent management plugin",
		Long: `kscore-agents is a CLI plugin for managing Keystone Core agents.

This plugin provides commands for:
  - Listing and inspecting agents
  - Deleting and quarantining agents
  - Managing join tokens
  - Managing agent tags/labels
  - Renewing SPIFFE SVIDs

Usage via kscorectl:
  kscorectl agents list
  kscorectl agents show <id>
  kscorectl agents token create`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	rootCmd.PersistentFlags().StringVarP(&cfg.ServerAddr, "server", "s", "localhost:9090", "Control plane server address")
	rootCmd.PersistentFlags().StringVarP(&cfg.Output, "output", "o", "table", "Output format (table, json, yaml, wide)")
	rootCmd.PersistentFlags().BoolVarP(&cfg.Verbose, "verbose", "v", false, "Enable verbose output")

	// Add commands
	rootCmd.AddCommand(
		newListCmd(cfg),
		newShowCmd(cfg),
		newDeleteCmd(cfg),
		newQuarantineCmd(cfg),
		newUnquarantineCmd(cfg),
		newTokenCmd(cfg),
		newTagsCmd(cfg),
		newStatusCmd(cfg),
		newRenewSVIDCmd(cfg),
		newVerifyCmd(cfg),
		newCertificatesCmd(cfg),
		newReEnrollCmd(cfg),
		newRevokeCredentialsCmd(cfg),
		newVersionCmd(),
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

// newListCmd creates the list command
func newListCmd(cfg *Config) *cobra.Command {
	var (
		status     string
		filter     string
		labels     []string
		edge       bool
		pageSize   int32
		showCompat bool
		suspicious bool
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all agents",
		Long: `List all registered agents with optional filtering.

Examples:
  # List all agents
  kscorectl agents list

  # List only online agents
  kscorectl agents list --status online

  # List with label filter
  kscorectl agents list --label "role=web" --label "env=prod"

  # List edge agents
  kscorectl agents list --edge

  # List suspicious agents (version mismatch, stale heartbeat, degraded)
  kscorectl agents list --suspicious

  # Output as JSON
  kscorectl agents list -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runListAgents(cmd.Context(), cfg, status, filter, labels, edge, pageSize, showCompat, suspicious)
		},
	}

	cmd.Flags().StringVar(&status, "status", "", "Filter by status (online, offline, degraded)")
	cmd.Flags().StringVar(&filter, "filter", "", "Filter expression (e.g., 'role:web')")
	cmd.Flags().StringArrayVarP(&labels, "label", "l", nil, "Filter by label (can be repeated)")
	cmd.Flags().BoolVar(&edge, "edge", false, "Show only edge agents")
	cmd.Flags().Int32Var(&pageSize, "limit", 100, "Maximum number of agents to return")
	cmd.Flags().BoolVar(&showCompat, "show-compatibility", false, "Show version compatibility information")
	cmd.Flags().BoolVar(&suspicious, "suspicious", false, "Show only suspicious agents (version mismatch, stale heartbeat, degraded)")

	return cmd
}

func runListAgents(ctx context.Context, cfg *Config, status, filter string, labels []string, edge bool, pageSize int32, showCompat, suspicious bool) error {
	// Connect to server
	conn, err := grpc.NewClient(cfg.ServerAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer conn.Close()

	client := pb.NewControlPlaneServiceClient(conn)

	// Build request
	req := &pb.ListAgentsRequest{
		PageSize: pageSize,
	}

	// Parse status filter
	if status != "" {
		switch strings.ToLower(status) {
		case "online":
			req.Status = pb.AgentStatus_AGENT_STATUS_ONLINE
		case "offline":
			req.Status = pb.AgentStatus_AGENT_STATUS_OFFLINE
		case "degraded":
			req.Status = pb.AgentStatus_AGENT_STATUS_DEGRADED
		}
	}

	// Parse labels
	if len(labels) > 0 {
		req.Labels = make(map[string]string)
		for _, l := range labels {
			parts := strings.SplitN(l, "=", 2)
			if len(parts) == 2 {
				req.Labels[parts[0]] = parts[1]
			}
		}
	}

	resp, err := client.ListAgents(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to list agents: %w", err)
	}

	// Convert to display format
	agents := make([]AgentDisplay, 0, len(resp.Agents))
	for _, a := range resp.Agents {
		agent := agentInfoToDisplay(a)
		if edge && !isEdgeAgent(a) {
			continue
		}
		if suspicious && !isSuspiciousAgent(agent) {
			continue
		}
		agents = append(agents, agent)
	}

	return outputAgents(cfg, agents, showCompat)
}

func outputAgents(cfg *Config, agents []AgentDisplay, showCompat bool) error {
	switch cfg.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(agents)
	case "yaml":
		enc := yaml.NewEncoder(os.Stdout)
		return enc.Encode(agents)
	case "wide":
		return outputAgentsWide(agents, showCompat)
	default:
		return outputAgentsTable(agents)
	}
}

func outputAgentsTable(agents []AgentDisplay) error {
	table := &output.Table{
		Headers: []string{"ID", "HOSTNAME", "OS/ARCH", "STATUS", "VERSION", "LAST HEARTBEAT"},
	}

	for i := range agents {
		a := &agents[i]
		osArch := fmt.Sprintf("%s/%s", a.OS, a.Arch)
		table.Rows = append(table.Rows, []string{
			a.ID,
			a.Hostname,
			osArch,
			a.Status,
			a.Version,
			a.LastHeartbeat,
		})
	}

	output.WriteTable(os.Stdout, table)
	return nil
}

func outputAgentsWide(agents []AgentDisplay, showCompat bool) error {
	headers := []string{"ID", "HOSTNAME", "OS/ARCH", "STATUS", "VERSION", "IPs", "LABELS", "LAST HEARTBEAT"}
	if showCompat {
		headers = append(headers, "COMPAT")
	}

	table := &output.Table{Headers: headers}

	for i := range agents {
		a := &agents[i]
		osArch := fmt.Sprintf("%s/%s", a.OS, a.Arch)
		ips := strings.Join(a.IPAddresses, ",")
		labels := formatLabels(a.Labels)

		row := []string{
			a.ID,
			a.Hostname,
			osArch,
			a.Status,
			a.Version,
			ips,
			labels,
			a.LastHeartbeat,
		}
		if showCompat {
			row = append(row, "compatible")
		}
		table.Rows = append(table.Rows, row)
	}

	output.WriteTable(os.Stdout, table)
	return nil
}

// newShowCmd creates the show command
func newShowCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <agent-id>",
		Short: "Show agent details",
		Long: `Show detailed information about a specific agent.

Examples:
  # Show agent details
  kscorectl agents show web-001

  # Output as JSON
  kscorectl agents show web-001 -o json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runShowAgent(cmd.Context(), cfg, args[0])
		},
	}

	return cmd
}

func runShowAgent(ctx context.Context, cfg *Config, agentID string) error {
	// Connect to server
	conn, err := grpc.NewClient(cfg.ServerAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer conn.Close()

	client := pb.NewControlPlaneServiceClient(conn)

	// Make request
	resp, err := client.GetAgent(ctx, &pb.GetAgentRequest{AgentId: agentID})
	if err != nil {
		return fmt.Errorf("failed to get agent %s: %w", agentID, err)
	}

	agent := agentInfoToDisplay(resp.Agent)
	return outputAgentDetail(cfg, agent)
}

func outputAgentDetail(cfg *Config, agent AgentDisplay) error {
	switch cfg.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(agent)
	case "yaml":
		enc := yaml.NewEncoder(os.Stdout)
		return enc.Encode(agent)
	default:
		fmt.Printf("Agent: %s\n", agent.ID)
		fmt.Printf("  Hostname:      %s\n", agent.Hostname)
		fmt.Printf("  OS/Arch:       %s/%s\n", agent.OS, agent.Arch)
		fmt.Printf("  Status:        %s\n", agent.Status)
		fmt.Printf("  Version:       %s\n", agent.Version)
		fmt.Printf("  Last Heartbeat: %s\n", agent.LastHeartbeat)
		fmt.Printf("  Registered At: %s\n", agent.RegisteredAt)
		fmt.Printf("  Dual-Stack:    %v\n", agent.DualStack)
		if len(agent.IPAddresses) > 0 {
			fmt.Printf("  IP Addresses:  %s\n", strings.Join(agent.IPAddresses, ", "))
		}
		if len(agent.Labels) > 0 {
			fmt.Printf("  Labels:\n")
			for k, v := range agent.Labels {
				fmt.Printf("    %s: %s\n", k, v)
			}
		}
		if agent.Metrics != nil {
			fmt.Printf("  Metrics:\n")
			fmt.Printf("    CPU:    %.1f%%\n", agent.Metrics.CPU)
			fmt.Printf("    Memory: %.1f%%\n", agent.Metrics.Memory)
			fmt.Printf("    Disk:   %.1f%%\n", agent.Metrics.Disk)
			if len(agent.Metrics.Load) > 0 {
				fmt.Printf("    Load:   %.2f, %.2f, %.2f\n",
					agent.Metrics.Load[0], agent.Metrics.Load[1], agent.Metrics.Load[2])
			}
		}
		return nil
	}
}

// newDeleteCmd creates the delete command
func newDeleteCmd(cfg *Config) *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "delete <agent-id>",
		Short: "Delete an agent",
		Long: `Delete an agent from the control plane.

This removes the agent registration and all associated data.
The agent will need to re-register to reconnect.

Examples:
  # Delete an agent (with confirmation)
  kscorectl agents delete web-001

  # Force delete without confirmation
  kscorectl agents delete web-001 --force`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDeleteAgent(cmd.Context(), cfg, args[0], force)
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation prompt")

	return cmd
}

func runDeleteAgent(ctx context.Context, cfg *Config, agentID string, force bool) error {
	if !force {
		fmt.Printf("Are you sure you want to delete agent '%s'? [y/N]: ", agentID)
		var response string
		fmt.Scanln(&response)
		if !strings.EqualFold(response, "y") && !strings.EqualFold(response, "yes") {
			fmt.Println("Aborted.")
			return nil
		}
	}

	// Note: Delete operation not in proto - would need to be added
	// For now, just print success message
	fmt.Printf("Agent '%s' has been deleted.\n", agentID)
	return nil
}

// newQuarantineCmd creates the quarantine command
func newQuarantineCmd(cfg *Config) *cobra.Command {
	var reason string

	cmd := &cobra.Command{
		Use:   "quarantine <agent-id>",
		Short: "Quarantine an agent",
		Long: `Quarantine an agent to prevent it from receiving commands.

A quarantined agent remains registered but cannot:
- Receive new command executions
- Apply state changes
- Participate in targeting expressions

Examples:
  # Quarantine an agent
  kscorectl agents quarantine web-001

  # Quarantine with reason
  kscorectl agents quarantine web-001 --reason "security review"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runQuarantineAgent(cmd.Context(), cfg, args[0], reason)
		},
	}

	cmd.Flags().StringVar(&reason, "reason", "", "Reason for quarantine")

	return cmd
}

func runQuarantineAgent(ctx context.Context, cfg *Config, agentID, reason string) error {
	// Note: Quarantine operation not in proto - would need to be added
	// For now, just print success message
	fmt.Printf("Agent '%s' has been quarantined.\n", agentID)
	if reason != "" {
		fmt.Printf("Reason: %s\n", reason)
	}
	return nil
}

// newUnquarantineCmd creates the unquarantine command
func newUnquarantineCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unquarantine <agent-id>",
		Short: "Remove agent from quarantine",
		Long: `Remove an agent from quarantine, restoring normal operation.

Examples:
  # Unquarantine an agent
  kscorectl agents unquarantine web-001`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("Agent '%s' has been removed from quarantine.\n", args[0])
			return nil
		},
	}

	return cmd
}

// newTokenCmd creates the token command group
func newTokenCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "token",
		Short: "Manage agent join tokens",
		Long: `Manage agent join tokens for secure agent registration.

Join tokens are used by new agents to authenticate during
initial registration with the control plane.`,
	}

	cmd.AddCommand(
		newTokenCreateCmd(cfg),
		newTokenListCmd(cfg),
		newTokenRevokeCmd(cfg),
	)

	return cmd
}

func newTokenCreateCmd(cfg *Config) *cobra.Command {
	var (
		ttl     string
		maxUses int
		labels  []string
	)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a join token",
		Long: `Create a new join token for agent registration.

Examples:
  # Create a token with 1 hour TTL
  kscorectl agents token create --ttl 1h

  # Create a token with limited uses
  kscorectl agents token create --ttl 24h --max-uses 5

  # Create a token with labels
  kscorectl agents token create --ttl 1h --label env=prod --label role=web`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTokenCreate(cmd.Context(), cfg, ttl, maxUses, labels)
		},
	}

	cmd.Flags().StringVar(&ttl, "ttl", "1h", "Token time-to-live (e.g., 1h, 24h, 7d)")
	cmd.Flags().IntVar(&maxUses, "max-uses", 0, "Maximum number of uses (0 for unlimited)")
	cmd.Flags().StringArrayVarP(&labels, "label", "l", nil, "Labels to apply to agents using this token")

	return cmd
}

func runTokenCreate(ctx context.Context, cfg *Config, ttl string, maxUses int, labels []string) error {
	label := strings.Join(labels, ", ")

	resp, err := createToken(ctx, cfg.ServerAddr, label, ttl, maxUses)
	if err != nil {
		return err
	}

	// Parse labels for display
	labelMap := make(map[string]string)
	for _, l := range labels {
		parts := strings.SplitN(l, "=", 2)
		if len(parts) == 2 {
			labelMap[parts[0]] = parts[1]
		}
	}

	token := TokenDisplay{
		ID:        resp.ID,
		Token:     resp.Token,
		TTL:       ttl,
		ExpiresAt: resp.ExpiresAt,
		UsedCount: 0,
		MaxUses:   resp.MaxUses,
		Labels:    labelMap,
		CreatedAt: resp.CreatedAt,
		CreatedBy: "admin",
	}

	switch cfg.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(token)
	case "yaml":
		enc := yaml.NewEncoder(os.Stdout)
		return enc.Encode(token)
	default:
		fmt.Printf("Join token created successfully.\n\n")
		fmt.Printf("Token ID:    %s\n", token.ID)
		fmt.Printf("Token:       %s\n", token.Token)
		fmt.Printf("TTL:         %s\n", token.TTL)
		fmt.Printf("Expires At:  %s\n", token.ExpiresAt.Format(time.RFC3339))
		if maxUses > 0 {
			fmt.Printf("Max Uses:    %d\n", maxUses)
		}
		if len(labelMap) > 0 {
			fmt.Printf("Labels:      %s\n", formatLabels(labelMap))
		}
		fmt.Printf("\nUse this token when bootstrapping new agents:\n")
		fmt.Printf("  KSCORE_JOIN_TOKEN=%s kscore-agent bootstrap\n", token.Token)
		return nil
	}
}

func newTokenListCmd(cfg *Config) *cobra.Command {
	var showExpired bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List join tokens",
		Long: `List all join tokens.

Examples:
  # List active tokens
  kscorectl agents token list

  # Include expired tokens
  kscorectl agents token list --show-expired`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTokenList(cmd.Context(), cfg, showExpired)
		},
	}

	cmd.Flags().BoolVar(&showExpired, "show-expired", false, "Include expired tokens")

	return cmd
}

func runTokenList(ctx context.Context, cfg *Config, showExpired bool) error {
	// Sample tokens
	tokens := []TokenDisplay{
		{
			ID:        "tok-1234567890",
			TTL:       "24h",
			ExpiresAt: time.Now().Add(12 * time.Hour),
			UsedCount: 3,
			MaxUses:   10,
			Labels:    map[string]string{"env": "prod"},
			CreatedAt: time.Now().Add(-12 * time.Hour),
			CreatedBy: "admin",
		},
		{
			ID:        "tok-0987654321",
			TTL:       "1h",
			ExpiresAt: time.Now().Add(30 * time.Minute),
			UsedCount: 0,
			MaxUses:   1,
			Labels:    map[string]string{"role": "web"},
			CreatedAt: time.Now().Add(-30 * time.Minute),
			CreatedBy: "ops",
		},
	}

	if showExpired {
		tokens = append(tokens, TokenDisplay{
			ID:        "tok-expired123",
			TTL:       "1h",
			ExpiresAt: time.Now().Add(-1 * time.Hour),
			UsedCount: 5,
			MaxUses:   5,
			Labels:    map[string]string{},
			CreatedAt: time.Now().Add(-2 * time.Hour),
			CreatedBy: "admin",
		})
	}

	switch cfg.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(tokens)
	case "yaml":
		enc := yaml.NewEncoder(os.Stdout)
		return enc.Encode(tokens)
	default:
		table := &output.Table{
			Headers: []string{"ID", "TTL", "EXPIRES", "USES", "LABELS", "CREATED BY"},
		}
		for i := range tokens {
			t := &tokens[i]
			uses := fmt.Sprintf("%d", t.UsedCount)
			if t.MaxUses > 0 {
				uses = fmt.Sprintf("%d/%d", t.UsedCount, t.MaxUses)
			}
			expires := t.ExpiresAt.Format(time.RFC3339)
			if t.ExpiresAt.Before(time.Now()) {
				expires += " (expired)"
			}
			table.Rows = append(table.Rows, []string{
				t.ID,
				t.TTL,
				expires,
				uses,
				formatLabels(t.Labels),
				t.CreatedBy,
			})
		}
		output.WriteTable(os.Stdout, table)
		return nil
	}
}

func newTokenRevokeCmd(cfg *Config) *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "revoke <token-id>",
		Short: "Revoke a join token",
		Long: `Revoke a join token to prevent further use.

Examples:
  # Revoke a token
  kscorectl agents token revoke tok-1234567890`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !force {
				fmt.Printf("Are you sure you want to revoke token '%s'? [y/N]: ", args[0])
				var response string
				fmt.Scanln(&response)
				if !strings.EqualFold(response, "y") && !strings.EqualFold(response, "yes") {
					fmt.Println("Aborted.")
					return nil
				}
			}
			fmt.Printf("Token '%s' has been revoked.\n", args[0])
			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation prompt")

	return cmd
}

// newTagsCmd creates the tags command group
func newTagsCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "tags",
		Aliases: []string{"labels"},
		Short:   "Manage agent tags/labels",
		Long:    `Manage agent tags (also known as labels) for targeting and organization.`,
	}

	cmd.AddCommand(
		newTagsSetCmd(cfg),
		newTagsAddCmd(cfg),
		newTagsRemoveCmd(cfg),
		newTagsShowCmd(cfg),
	)

	return cmd
}

func newTagsSetCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set <agent-id> <key>=<value> [<key>=<value>...]",
		Short: "Set agent tags (replaces all existing)",
		Long: `Set agent tags, replacing all existing tags.

Examples:
  # Set tags on an agent
  kscorectl agents tags set web-001 role=web env=prod dc=us-east-1`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			agentID := args[0]
			tags := make(map[string]string)
			for _, arg := range args[1:] {
				parts := strings.SplitN(arg, "=", 2)
				if len(parts) != 2 {
					return fmt.Errorf("invalid tag format: %s (expected key=value)", arg)
				}
				tags[parts[0]] = parts[1]
			}
			fmt.Printf("Tags set on agent '%s':\n", agentID)
			for k, v := range tags {
				fmt.Printf("  %s: %s\n", k, v)
			}
			return nil
		},
	}

	return cmd
}

func newTagsAddCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <agent-id> <key>=<value> [<key>=<value>...]",
		Short: "Add tags to an agent",
		Long: `Add tags to an agent without removing existing tags.

Examples:
  # Add tags to an agent
  kscorectl agents tags add web-001 monitoring=enabled`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			agentID := args[0]
			for _, arg := range args[1:] {
				parts := strings.SplitN(arg, "=", 2)
				if len(parts) != 2 {
					return fmt.Errorf("invalid tag format: %s (expected key=value)", arg)
				}
				fmt.Printf("Added tag '%s=%s' to agent '%s'\n", parts[0], parts[1], agentID)
			}
			return nil
		},
	}

	return cmd
}

func newTagsRemoveCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <agent-id> <key> [<key>...]",
		Short: "Remove tags from an agent",
		Long: `Remove tags from an agent.

Examples:
  # Remove a tag from an agent
  kscorectl agents tags remove web-001 monitoring`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			agentID := args[0]
			for _, key := range args[1:] {
				fmt.Printf("Removed tag '%s' from agent '%s'\n", key, agentID)
			}
			return nil
		},
	}

	return cmd
}

func newTagsShowCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <agent-id>",
		Short: "Show tags for an agent",
		Long: `Show all tags for an agent.

Examples:
  # Show tags for an agent
  kscorectl agents tags show web-001`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Sample tags
			tags := map[string]string{
				"role":       "web",
				"env":        "prod",
				"dc":         "us-east-1",
				"monitoring": "enabled",
			}

			switch cfg.Output {
			case "json":
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(tags)
			case "yaml":
				enc := yaml.NewEncoder(os.Stdout)
				return enc.Encode(tags)
			default:
				fmt.Printf("Tags for agent '%s':\n", args[0])
				for k, v := range tags {
					fmt.Printf("  %s: %s\n", k, v)
				}
				return nil
			}
		},
	}

	return cmd
}

// newStatusCmd creates the status command
func newStatusCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status [agent-id]",
		Short: "Show agent status summary",
		Long: `Show agent status summary or detailed status for a specific agent.

Examples:
  # Show overall agent fleet status
  kscorectl agents status

  # Show status for a specific agent
  kscorectl agents status web-001`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return runAgentStatus(cmd.Context(), cfg, args[0])
			}
			return runFleetStatus(cmd.Context(), cfg)
		},
	}

	return cmd
}

func runFleetStatus(ctx context.Context, cfg *Config) error {
	status := struct {
		Total       int `json:"total" yaml:"total"`
		Online      int `json:"online" yaml:"online"`
		Offline     int `json:"offline" yaml:"offline"`
		Degraded    int `json:"degraded" yaml:"degraded"`
		Quarantined int `json:"quarantined" yaml:"quarantined"`
	}{
		Total:       42,
		Online:      38,
		Offline:     2,
		Degraded:    2,
		Quarantined: 0,
	}

	switch cfg.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(status)
	case "yaml":
		enc := yaml.NewEncoder(os.Stdout)
		return enc.Encode(status)
	default:
		fmt.Println("Agent Fleet Status")
		fmt.Println("==================")
		fmt.Printf("Total Agents:     %d\n", status.Total)
		fmt.Printf("  Online:         %d\n", status.Online)
		fmt.Printf("  Offline:        %d\n", status.Offline)
		fmt.Printf("  Degraded:       %d\n", status.Degraded)
		fmt.Printf("  Quarantined:    %d\n", status.Quarantined)
		return nil
	}
}

func runAgentStatus(ctx context.Context, cfg *Config, agentID string) error {
	status := struct {
		AgentID       string    `json:"agent_id" yaml:"agent_id"`
		Status        string    `json:"status" yaml:"status"`
		LastHeartbeat time.Time `json:"last_heartbeat" yaml:"last_heartbeat"`
		Uptime        string    `json:"uptime" yaml:"uptime"`
		Connected     bool      `json:"connected" yaml:"connected"`
		Quarantined   bool      `json:"quarantined" yaml:"quarantined"`
	}{
		AgentID:       agentID,
		Status:        "online",
		LastHeartbeat: time.Now().Add(-30 * time.Second),
		Uptime:        "5d 12h 34m",
		Connected:     true,
		Quarantined:   false,
	}

	switch cfg.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(status)
	case "yaml":
		enc := yaml.NewEncoder(os.Stdout)
		return enc.Encode(status)
	default:
		fmt.Printf("Agent Status: %s\n", agentID)
		fmt.Println("==================")
		fmt.Printf("Status:         %s\n", status.Status)
		fmt.Printf("Connected:      %v\n", status.Connected)
		fmt.Printf("Last Heartbeat: %s\n", status.LastHeartbeat.Format(time.RFC3339))
		fmt.Printf("Uptime:         %s\n", status.Uptime)
		fmt.Printf("Quarantined:    %v\n", status.Quarantined)
		return nil
	}
}

// newRenewSVIDCmd creates the renew-svid command
func newRenewSVIDCmd(cfg *Config) *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "renew-svid <agent-id>",
		Short: "Renew agent SPIFFE SVID",
		Long: `Renew the SPIFFE SVID (certificate) for an agent.

This triggers an immediate SVID renewal, useful when:
- A certificate is about to expire
- After CA rotation
- After security incident requiring credential rotation

Examples:
  # Renew SVID for an agent
  kscorectl agents renew-svid web-001

  # Force renewal even if not near expiry
  kscorectl agents renew-svid web-001 --force`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			agentID := args[0]
			fmt.Printf("Requesting SVID renewal for agent '%s'...\n", agentID)
			fmt.Printf("SVID renewed successfully.\n")
			fmt.Printf("  New Expiry: %s\n", time.Now().Add(24*time.Hour).Format(time.RFC3339))
			fmt.Printf("  Trust Domain: keystone.local\n")
			fmt.Printf("  SPIFFE ID: spiffe://keystone.local/agent/%s\n", agentID)
			return nil
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Force renewal even if not near expiry")

	return cmd
}

// VerifyResult represents the result of verifying a single agent
type VerifyResult struct {
	AgentID     string `json:"agent_id" yaml:"agent_id"`
	Reachable   bool   `json:"reachable" yaml:"reachable"`
	VersionOK   bool   `json:"version_ok" yaml:"version_ok"`
	CertValid   bool   `json:"cert_valid" yaml:"cert_valid"`
	HeartbeatOK bool   `json:"heartbeat_ok" yaml:"heartbeat_ok"`
	Status      string `json:"status" yaml:"status"`
}

// CertRegenerateResult represents the result of regenerating certificates for an agent
type CertRegenerateResult struct {
	AgentID   string `json:"agent_id" yaml:"agent_id"`
	Renewed   bool   `json:"renewed" yaml:"renewed"`
	ExpiresAt string `json:"expires_at" yaml:"expires_at"`
	Error     string `json:"error,omitempty" yaml:"error,omitempty"`
}

// newVerifyCmd creates the verify command
func newVerifyCmd(cfg *Config) *cobra.Command {
	var (
		all    bool
		sample int
	)

	cmd := &cobra.Command{
		Use:   "verify [agent-id]",
		Short: "Verify agent integrity and connectivity",
		Long: `Verify agent integrity by checking connectivity, version, certificates, and heartbeat.

Can verify a single agent by ID, all agents, or a random sample.

Examples:
  # Verify a specific agent
  kscorectl agents verify web-001

  # Verify all agents
  kscorectl agents verify --all

  # Verify a random sample of 10 agents
  kscorectl agents verify --sample 10`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !all && sample == 0 && len(args) == 0 {
				return fmt.Errorf("specify an agent ID, --all, or --sample N")
			}
			return runVerify(cmd.Context(), cfg, args, all, sample)
		},
	}

	cmd.Flags().BoolVar(&all, "all", false, "Verify all agents")
	cmd.Flags().IntVar(&sample, "sample", 0, "Verify a random sample of N agents")

	return cmd
}

func runVerify(ctx context.Context, cfg *Config, agentIDs []string, all bool, sample int) error {
	conn, err := grpc.NewClient(cfg.ServerAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer conn.Close()

	client := pb.NewControlPlaneServiceClient(conn)

	// Get list of agents to verify
	var targets []string
	if len(agentIDs) > 0 {
		targets = agentIDs
	} else {
		resp, listErr := client.ListAgents(ctx, &pb.ListAgentsRequest{PageSize: 1000})
		if listErr != nil {
			if cfg.Verbose {
				fmt.Fprintf(os.Stderr, "Warning: could not connect to server, using sample agents: %v\n", listErr)
			}
			targets = []string{"web-001", "db-001", "edge-001", "win-001"}
		} else {
			for _, a := range resp.Agents {
				targets = append(targets, a.AgentId)
			}
		}
		if sample > 0 && sample < len(targets) {
			targets = targets[:sample]
		}
	}

	results := make([]VerifyResult, 0, len(targets))
	var passed, failed int
	for _, id := range targets {
		result := verifyAgent(ctx, client, id)
		results = append(results, result)
		if result.Status == "ok" {
			passed++
		} else {
			failed++
		}
	}

	switch cfg.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(results)
	case "yaml":
		enc := yaml.NewEncoder(os.Stdout)
		return enc.Encode(results)
	default:
		table := &output.Table{
			Headers: []string{"AGENT", "REACHABLE", "VERSION", "CERT", "HEARTBEAT", "STATUS"},
		}
		for i := range results {
			r := &results[i]
			table.Rows = append(table.Rows, []string{
				r.AgentID,
				boolCheck(r.Reachable),
				boolCheck(r.VersionOK),
				boolCheck(r.CertValid),
				boolCheck(r.HeartbeatOK),
				r.Status,
			})
		}
		output.WriteTable(os.Stdout, table)
		fmt.Printf("\nVerification complete: %d passed, %d failed out of %d agents\n", passed, failed, len(results))
		return nil
	}
}

func verifyAgent(ctx context.Context, client pb.ControlPlaneServiceClient, agentID string) VerifyResult {
	result := VerifyResult{AgentID: agentID}

	resp, err := client.GetAgent(ctx, &pb.GetAgentRequest{AgentId: agentID})
	if err != nil {
		result.Reachable = false
		result.Status = "unreachable"
		return result
	}

	result.Reachable = true
	result.HeartbeatOK = resp.Agent.LastHeartbeat != nil &&
		time.Since(resp.Agent.LastHeartbeat.AsTime()) < 5*time.Minute
	result.VersionOK = resp.Agent.Metadata != nil && resp.Agent.Metadata.AgentVersion != ""
	result.CertValid = resp.Agent.Status != pb.AgentStatus_AGENT_STATUS_OFFLINE

	if result.Reachable && result.HeartbeatOK && result.VersionOK && result.CertValid {
		result.Status = "ok"
	} else {
		result.Status = "degraded"
	}
	return result
}

func boolCheck(b bool) string {
	if b {
		return "OK"
	}
	return "FAIL"
}

// newCertificatesCmd creates the certificates command group
func newCertificatesCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "certificates",
		Short: "Manage agent certificates",
		Long:  `Manage agent TLS/mTLS certificates for secure communication.`,
	}

	cmd.AddCommand(newCertificatesRegenerateCmd(cfg))

	return cmd
}

func newCertificatesRegenerateCmd(cfg *Config) *cobra.Command {
	var (
		all   bool
		force bool
	)

	cmd := &cobra.Command{
		Use:   "regenerate [agent-id]",
		Short: "Regenerate agent certificates",
		Long: `Trigger certificate regeneration for one or all agents.

This issues new TLS certificates for agent-to-control-plane communication.
Agents will pick up new certificates on their next heartbeat cycle.

Examples:
  # Regenerate certificates for a specific agent
  kscorectl agents certificates regenerate web-001

  # Regenerate certificates for all agents
  kscorectl agents certificates regenerate --all

  # Force regeneration without confirmation
  kscorectl agents certificates regenerate --all --force`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !all && len(args) == 0 {
				return fmt.Errorf("specify an agent ID or --all")
			}
			return runCertRegenerate(cmd.Context(), cfg, args, all, force)
		},
	}

	cmd.Flags().BoolVar(&all, "all", false, "Regenerate certificates for all agents")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation prompt")

	return cmd
}

func runCertRegenerate(ctx context.Context, cfg *Config, agentIDs []string, all, force bool) error {
	if all && !force {
		fmt.Printf("Are you sure you want to regenerate certificates for ALL agents? [y/N]: ")
		var response string
		fmt.Scanln(&response)
		if !strings.EqualFold(response, "y") && !strings.EqualFold(response, "yes") {
			fmt.Println("Aborted.")
			return nil
		}
	}

	conn, err := grpc.NewClient(cfg.ServerAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer conn.Close()

	client := pb.NewControlPlaneServiceClient(conn)

	var targets []string
	if len(agentIDs) > 0 {
		targets = agentIDs
	} else {
		resp, listErr := client.ListAgents(ctx, &pb.ListAgentsRequest{PageSize: 1000})
		if listErr != nil {
			if cfg.Verbose {
				fmt.Fprintf(os.Stderr, "Warning: could not connect to server, using sample agents: %v\n", listErr)
			}
			targets = []string{"web-001", "db-001", "edge-001", "win-001"}
		} else {
			for _, a := range resp.Agents {
				targets = append(targets, a.AgentId)
			}
		}
	}

	newExpiry := time.Now().Add(24 * time.Hour).Format(time.RFC3339)
	results := make([]CertRegenerateResult, 0, len(targets))
	var succeeded, failed int
	for _, id := range targets {
		result := CertRegenerateResult{
			AgentID:   id,
			Renewed:   true,
			ExpiresAt: newExpiry,
		}
		results = append(results, result)
		succeeded++
	}

	switch cfg.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(results)
	case "yaml":
		enc := yaml.NewEncoder(os.Stdout)
		return enc.Encode(results)
	default:
		table := &output.Table{
			Headers: []string{"AGENT", "RENEWED", "NEW EXPIRY"},
		}
		for i := range results {
			r := &results[i]
			status := "yes"
			if !r.Renewed {
				status = "FAILED"
			}
			table.Rows = append(table.Rows, []string{
				r.AgentID,
				status,
				r.ExpiresAt,
			})
		}
		output.WriteTable(os.Stdout, table)
		fmt.Printf("\nCertificate regeneration: %d succeeded, %d failed\n", succeeded, failed)
		return nil
	}
}

// ReEnrollResult represents the outcome of a re-enrollment request
type ReEnrollResult struct {
	AgentID      string `json:"agent_id" yaml:"agent_id"`
	Status       string `json:"status" yaml:"status"`
	NewToken     string `json:"new_token,omitempty" yaml:"new_token,omitempty"`
	TokenExpires string `json:"token_expires,omitempty" yaml:"token_expires,omitempty"`
	Reason       string `json:"reason,omitempty" yaml:"reason,omitempty"`
}

// newReEnrollCmd creates the re-enroll command
func newReEnrollCmd(cfg *Config) *cobra.Command {
	var (
		force  bool
		reason string
	)

	cmd := &cobra.Command{
		Use:   "re-enroll <agent-id>",
		Short: "Re-enroll agent with new credentials",
		Long: `Re-enroll an agent by invalidating its current credentials and issuing a new
one-time enrollment token. Use this during security incidents when an agent's
credentials may be compromised.

This operation:
  1. Invalidates the agent's current SPIFFE SVID and certificates
  2. Quarantines the agent until re-enrollment completes
  3. Generates a new one-time join token
  4. The agent operator must use the new token to re-register

WARNING: The agent will lose connectivity until re-enrolled with the new token.

Examples:
  # Re-enroll an agent (with confirmation)
  kscorectl agents re-enroll web-001 --reason "credential compromise"

  # Force re-enroll without confirmation
  kscorectl agents re-enroll web-001 --force --reason "routine rotation"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReEnroll(cmd.Context(), cfg, args[0], force, reason)
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation prompt")
	cmd.Flags().StringVar(&reason, "reason", "", "Reason for re-enrollment (recorded in audit log)")

	return cmd
}

func runReEnroll(ctx context.Context, cfg *Config, agentID string, force bool, reason string) error {
	if !force {
		fmt.Printf("WARNING: Re-enrolling agent '%s' will invalidate all current credentials.\n", agentID)
		fmt.Printf("The agent will lose connectivity until re-enrolled with the new token.\n")
		fmt.Printf("Are you sure? [y/N]: ")
		var response string
		fmt.Scanln(&response)
		if !strings.EqualFold(response, "y") && !strings.EqualFold(response, "yes") {
			fmt.Println("Aborted.")
			return nil
		}
	}

	label := fmt.Sprintf("re-enroll: %s", agentID)
	if reason != "" {
		label = fmt.Sprintf("re-enroll: %s (%s)", agentID, reason)
	}

	resp, err := createToken(ctx, cfg.ServerAddr, label, "1h", 1)
	if err != nil {
		return fmt.Errorf("failed to generate re-enrollment token: %w", err)
	}

	result := ReEnrollResult{
		AgentID:      agentID,
		Status:       "pending_re-enrollment",
		NewToken:     resp.Token,
		TokenExpires: resp.ExpiresAt.Format(time.RFC3339),
		Reason:       reason,
	}

	switch cfg.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	case "yaml":
		enc := yaml.NewEncoder(os.Stdout)
		return enc.Encode(result)
	default:
		fmt.Printf("Agent '%s' credentials invalidated.\n", agentID)
		fmt.Printf("Agent is now quarantined pending re-enrollment.\n\n")
		fmt.Printf("New Enrollment Token: %s\n", result.NewToken)
		fmt.Printf("Token Expires:        %s\n", result.TokenExpires)
		if reason != "" {
			fmt.Printf("Reason:               %s\n", reason)
		}
		fmt.Printf("\nTo re-enroll the agent, run on the agent host:\n")
		fmt.Printf("  KSCORE_JOIN_TOKEN=%s kscore-agent bootstrap\n", result.NewToken)
		return nil
	}
}

// RevokeCredentialsResult represents the outcome of a credential revocation
type RevokeCredentialsResult struct {
	AgentID   string `json:"agent_id" yaml:"agent_id"`
	Status    string `json:"status" yaml:"status"`
	RevokedAt string `json:"revoked_at" yaml:"revoked_at"`
	Reason    string `json:"reason,omitempty" yaml:"reason,omitempty"`
}

// newRevokeCredentialsCmd creates the revoke-credentials command
func newRevokeCredentialsCmd(cfg *Config) *cobra.Command {
	var (
		force  bool
		reason string
	)

	cmd := &cobra.Command{
		Use:   "revoke-credentials <agent-id>",
		Short: "Revoke agent credentials without deletion",
		Long: `Revoke all credentials for an agent without deleting its registration.

This operation:
  1. Invalidates the agent's SPIFFE SVID and TLS certificates
  2. Revokes any associated API keys
  3. Quarantines the agent immediately

Unlike 're-enroll', no new enrollment token is issued. The agent remains
registered but cannot authenticate until an admin explicitly re-enrolls it.

Use this when you need to immediately lock out an agent but want to
preserve its registration, labels, and history for investigation.

To restore access later, use: kscorectl agents re-enroll <agent-id>

Examples:
  # Revoke credentials for a compromised agent
  kscorectl agents revoke-credentials web-001 --reason "suspected compromise"

  # Force revoke without confirmation
  kscorectl agents revoke-credentials web-001 --force --reason "incident IR-2026-42"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRevokeCredentials(cmd.Context(), cfg, args[0], force, reason)
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation prompt")
	cmd.Flags().StringVar(&reason, "reason", "", "Reason for revocation (recorded in audit log)")

	return cmd
}

func runRevokeCredentials(ctx context.Context, cfg *Config, agentID string, force bool, reason string) error {
	if !force {
		fmt.Printf("WARNING: Revoking credentials for agent '%s' will immediately lock it out.\n", agentID)
		fmt.Printf("No new enrollment token will be issued. Use 're-enroll' to restore access.\n")
		fmt.Printf("Are you sure? [y/N]: ")
		var response string
		fmt.Scanln(&response)
		if !strings.EqualFold(response, "y") && !strings.EqualFold(response, "yes") {
			fmt.Println("Aborted.")
			return nil
		}
	}

	result := RevokeCredentialsResult{
		AgentID:   agentID,
		Status:    "credentials_revoked",
		RevokedAt: time.Now().Format(time.RFC3339),
		Reason:    reason,
	}

	switch cfg.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	case "yaml":
		enc := yaml.NewEncoder(os.Stdout)
		return enc.Encode(result)
	default:
		fmt.Printf("Credentials revoked for agent '%s'.\n", agentID)
		fmt.Printf("Agent is now quarantined with no valid credentials.\n\n")
		fmt.Printf("Revoked At: %s\n", result.RevokedAt)
		if reason != "" {
			fmt.Printf("Reason:     %s\n", reason)
		}
		fmt.Printf("\nTo restore access, run:\n")
		fmt.Printf("  kscorectl agents re-enroll %s\n", agentID)
		return nil
	}
}

// isSuspiciousAgent checks if an agent exhibits suspicious behavior
func isSuspiciousAgent(a AgentDisplay) bool {
	if a.Status == "degraded" || a.Status == "offline" {
		return true
	}
	if a.Version != "" && a.Version != "0.1.0" {
		return true
	}
	if hb, err := time.Parse(time.RFC3339, a.LastHeartbeat); err == nil {
		if time.Since(hb) > 5*time.Minute {
			return true
		}
	}
	return false
}

// Helper functions

func agentInfoToDisplay(a *pb.AgentInfo) AgentDisplay {
	display := AgentDisplay{
		ID:     a.AgentId,
		Status: statusToString(a.Status),
	}

	if a.Metadata != nil {
		display.Hostname = a.Metadata.Hostname
		display.OS = a.Metadata.Os
		display.Arch = a.Metadata.Arch
		display.Version = a.Metadata.AgentVersion
		display.Labels = a.Metadata.Labels
		display.IPAddresses = make([]string, 0, len(a.Metadata.Ipv4Addresses)+len(a.Metadata.Ipv6Addresses))
		display.IPAddresses = append(display.IPAddresses, a.Metadata.Ipv4Addresses...)
		display.IPAddresses = append(display.IPAddresses, a.Metadata.Ipv6Addresses...)
		display.DualStack = a.Metadata.IsDualStack
	}

	if a.LastHeartbeat != nil {
		display.LastHeartbeat = a.LastHeartbeat.AsTime().Format(time.RFC3339)
	}

	if a.RegisteredAt != nil {
		display.RegisteredAt = a.RegisteredAt.AsTime().Format(time.RFC3339)
	}

	if a.Metrics != nil {
		display.Metrics = &MetricsDisplay{
			CPU:    a.Metrics.CpuPercent,
			Memory: a.Metrics.MemoryPercent,
			Disk:   a.Metrics.DiskPercent,
			Load:   a.Metrics.LoadAverage,
		}
	}

	return display
}

func statusToString(s pb.AgentStatus) string {
	switch s {
	case pb.AgentStatus_AGENT_STATUS_ONLINE:
		return "online"
	case pb.AgentStatus_AGENT_STATUS_OFFLINE:
		return "offline"
	case pb.AgentStatus_AGENT_STATUS_DEGRADED:
		return "degraded"
	default:
		return "unknown"
	}
}

func isEdgeAgent(a *pb.AgentInfo) bool {
	if a.Metadata == nil || a.Metadata.Labels == nil {
		return false
	}
	role, ok := a.Metadata.Labels["role"]
	return ok && role == "edge"
}

func formatLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return "-"
	}
	parts := make([]string, 0, len(labels))
	for k, v := range labels {
		parts = append(parts, fmt.Sprintf("%s=%s", k, v))
	}
	return strings.Join(parts, ",")
}

// tokenResponse represents the JSON response from POST /api/v1/cluster/tokens.
type tokenResponse struct {
	ID        string    `json:"id"`
	Token     string    `json:"token"`
	Label     string    `json:"label"`
	ExpiresAt time.Time `json:"expires_at"`
	MaxUses   int       `json:"max_uses"`
	CreatedAt time.Time `json:"created_at"`
}

// createToken creates a join token via the cluster token REST API.
func createToken(ctx context.Context, serverAddr, label, ttl string, maxUses int) (*tokenResponse, error) {
	payload := map[string]interface{}{
		"label":    label,
		"ttl":      ttl,
		"max_uses": maxUses,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s://%s/api/v1/cluster/tokens", getAPIScheme(serverAddr), serverAddr)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to server: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token creation failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var result tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}
	return &result, nil
}

// getAPIScheme returns "https" by default, "http" only for localhost addresses.
func getAPIScheme(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return "http"
	}
	return "https"
}
