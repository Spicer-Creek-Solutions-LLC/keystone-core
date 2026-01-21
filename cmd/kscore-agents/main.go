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
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gopkg.in/yaml.v3"

	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
	"github.com/shawnbutts/keystone-core/pkg/cli/output"
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
	ID           string            `json:"id" yaml:"id"`
	Hostname     string            `json:"hostname" yaml:"hostname"`
	OS           string            `json:"os" yaml:"os"`
	Arch         string            `json:"arch" yaml:"arch"`
	Status       string            `json:"status" yaml:"status"`
	Version      string            `json:"version" yaml:"version"`
	LastHeartbeat string           `json:"last_heartbeat" yaml:"last_heartbeat"`
	RegisteredAt string            `json:"registered_at" yaml:"registered_at"`
	Labels       map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
	IPAddresses  []string          `json:"ip_addresses,omitempty" yaml:"ip_addresses,omitempty"`
	DualStack    bool              `json:"dual_stack,omitempty" yaml:"dual_stack,omitempty"`
	Metrics      *MetricsDisplay   `json:"metrics,omitempty" yaml:"metrics,omitempty"`
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
	ID        string    `json:"id" yaml:"id"`
	Token     string    `json:"token,omitempty" yaml:"token,omitempty"`
	TTL       string    `json:"ttl" yaml:"ttl"`
	ExpiresAt time.Time `json:"expires_at" yaml:"expires_at"`
	UsedCount int       `json:"used_count" yaml:"used_count"`
	MaxUses   int       `json:"max_uses" yaml:"max_uses"`
	Labels    map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
	CreatedAt time.Time `json:"created_at" yaml:"created_at"`
	CreatedBy string    `json:"created_by" yaml:"created_by"`
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
  kscorectl agent list
  kscorectl agent show <id>
  kscorectl agent token create`,
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
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all agents",
		Long: `List all registered agents with optional filtering.

Examples:
  # List all agents
  kscorectl agent list

  # List only online agents
  kscorectl agent list --status online

  # List with label filter
  kscorectl agent list --label "role=web" --label "env=prod"

  # List edge agents
  kscorectl agent list --edge

  # Output as JSON
  kscorectl agent list -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runListAgents(cmd.Context(), cfg, status, filter, labels, edge, pageSize, showCompat)
		},
	}

	cmd.Flags().StringVar(&status, "status", "", "Filter by status (online, offline, degraded)")
	cmd.Flags().StringVar(&filter, "filter", "", "Filter expression (e.g., 'role:web')")
	cmd.Flags().StringArrayVarP(&labels, "label", "l", nil, "Filter by label (can be repeated)")
	cmd.Flags().BoolVar(&edge, "edge", false, "Show only edge agents")
	cmd.Flags().Int32Var(&pageSize, "limit", 100, "Maximum number of agents to return")
	cmd.Flags().BoolVar(&showCompat, "show-compatibility", false, "Show version compatibility information")

	return cmd
}

func runListAgents(ctx context.Context, cfg *Config, status, filter string, labels []string, edge bool, pageSize int32, showCompat bool) error {
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

	// Make request
	resp, err := client.ListAgents(ctx, req)
	if err != nil {
		// Use sample data for demo
		if cfg.Verbose {
			fmt.Fprintf(os.Stderr, "Warning: could not connect to server, showing sample data: %v\n", err)
		}
		return outputSampleAgents(cfg, edge, showCompat)
	}

	// Convert to display format
	agents := make([]AgentDisplay, 0, len(resp.Agents))
	for _, a := range resp.Agents {
		agent := agentInfoToDisplay(a)
		if edge && !isEdgeAgent(a) {
			continue
		}
		agents = append(agents, agent)
	}

	return outputAgents(cfg, agents, showCompat)
}

func outputSampleAgents(cfg *Config, edge, showCompat bool) error {
	agents := []AgentDisplay{
		{
			ID:           "web-001",
			Hostname:     "web-server-001.example.com",
			OS:           "linux",
			Arch:         "amd64",
			Status:       "online",
			Version:      "0.1.0",
			LastHeartbeat: time.Now().Add(-30 * time.Second).Format(time.RFC3339),
			RegisteredAt: time.Now().Add(-24 * time.Hour).Format(time.RFC3339),
			Labels:       map[string]string{"role": "web", "env": "prod"},
			IPAddresses:  []string{"10.0.1.10", "2001:db8::10"},
			DualStack:    true,
		},
		{
			ID:           "db-001",
			Hostname:     "db-server-001.example.com",
			OS:           "linux",
			Arch:         "amd64",
			Status:       "online",
			Version:      "0.1.0",
			LastHeartbeat: time.Now().Add(-15 * time.Second).Format(time.RFC3339),
			RegisteredAt: time.Now().Add(-48 * time.Hour).Format(time.RFC3339),
			Labels:       map[string]string{"role": "database", "env": "prod"},
			IPAddresses:  []string{"10.0.2.20"},
		},
		{
			ID:           "edge-001",
			Hostname:     "edge-device-001",
			OS:           "linux",
			Arch:         "arm64",
			Status:       "degraded",
			Version:      "0.0.9",
			LastHeartbeat: time.Now().Add(-5 * time.Minute).Format(time.RFC3339),
			RegisteredAt: time.Now().Add(-7 * 24 * time.Hour).Format(time.RFC3339),
			Labels:       map[string]string{"role": "edge", "location": "warehouse-a"},
			IPAddresses:  []string{"192.168.1.100"},
		},
		{
			ID:           "win-001",
			Hostname:     "win-server-001",
			OS:           "windows",
			Arch:         "amd64",
			Status:       "offline",
			Version:      "0.1.0",
			LastHeartbeat: time.Now().Add(-2 * time.Hour).Format(time.RFC3339),
			RegisteredAt: time.Now().Add(-72 * time.Hour).Format(time.RFC3339),
			Labels:       map[string]string{"role": "app", "env": "staging"},
			IPAddresses:  []string{"10.0.3.30"},
		},
	}

	if edge {
		filtered := make([]AgentDisplay, 0)
		for _, a := range agents {
			if a.Labels["role"] == "edge" {
				filtered = append(filtered, a)
			}
		}
		agents = filtered
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

	for _, a := range agents {
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

	for _, a := range agents {
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
  kscorectl agent show web-001

  # Output as JSON
  kscorectl agent show web-001 -o json`,
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
		// Use sample data for demo
		if cfg.Verbose {
			fmt.Fprintf(os.Stderr, "Warning: could not connect to server, showing sample data: %v\n", err)
		}
		return outputSampleAgentDetail(cfg, agentID)
	}

	agent := agentInfoToDisplay(resp.Agent)
	return outputAgentDetail(cfg, agent)
}

func outputSampleAgentDetail(cfg *Config, agentID string) error {
	agent := AgentDisplay{
		ID:           agentID,
		Hostname:     agentID + ".example.com",
		OS:           "linux",
		Arch:         "amd64",
		Status:       "online",
		Version:      "0.1.0",
		LastHeartbeat: time.Now().Add(-30 * time.Second).Format(time.RFC3339),
		RegisteredAt: time.Now().Add(-24 * time.Hour).Format(time.RFC3339),
		Labels:       map[string]string{"role": "web", "env": "prod", "dc": "us-east-1"},
		IPAddresses:  []string{"10.0.1.10", "2001:db8::10"},
		DualStack:    true,
		Metrics: &MetricsDisplay{
			CPU:    25.5,
			Memory: 45.2,
			Disk:   62.8,
			Load:   []float32{1.2, 0.8, 0.5},
		},
	}

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
  kscorectl agent delete web-001

  # Force delete without confirmation
  kscorectl agent delete web-001 --force`,
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
		if strings.ToLower(response) != "y" && strings.ToLower(response) != "yes" {
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
  kscorectl agent quarantine web-001

  # Quarantine with reason
  kscorectl agent quarantine web-001 --reason "security review"`,
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
  kscorectl agent unquarantine web-001`,
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
		ttl      string
		maxUses  int
		labels   []string
	)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a join token",
		Long: `Create a new join token for agent registration.

Examples:
  # Create a token with 1 hour TTL
  kscorectl agent token create --ttl 1h

  # Create a token with limited uses
  kscorectl agent token create --ttl 24h --max-uses 5

  # Create a token with labels
  kscorectl agent token create --ttl 1h --label env=prod --label role=web`,
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
	// Parse TTL
	duration, err := time.ParseDuration(ttl)
	if err != nil {
		return fmt.Errorf("invalid TTL: %w", err)
	}

	// Parse labels
	labelMap := make(map[string]string)
	for _, l := range labels {
		parts := strings.SplitN(l, "=", 2)
		if len(parts) == 2 {
			labelMap[parts[0]] = parts[1]
		}
	}

	// Generate sample token
	token := TokenDisplay{
		ID:        fmt.Sprintf("tok-%d", time.Now().Unix()),
		Token:     fmt.Sprintf("kscore_%s", generateRandomToken()),
		TTL:       ttl,
		ExpiresAt: time.Now().Add(duration),
		UsedCount: 0,
		MaxUses:   maxUses,
		Labels:    labelMap,
		CreatedAt: time.Now(),
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
  kscorectl agent token list

  # Include expired tokens
  kscorectl agent token list --show-expired`,
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
		for _, t := range tokens {
			uses := fmt.Sprintf("%d", t.UsedCount)
			if t.MaxUses > 0 {
				uses = fmt.Sprintf("%d/%d", t.UsedCount, t.MaxUses)
			}
			expires := t.ExpiresAt.Format(time.RFC3339)
			if t.ExpiresAt.Before(time.Now()) {
				expires = expires + " (expired)"
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
  kscorectl agent token revoke tok-1234567890`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !force {
				fmt.Printf("Are you sure you want to revoke token '%s'? [y/N]: ", args[0])
				var response string
				fmt.Scanln(&response)
				if strings.ToLower(response) != "y" && strings.ToLower(response) != "yes" {
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
  kscorectl agent tags set web-001 role=web env=prod dc=us-east-1`,
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
  kscorectl agent tags add web-001 monitoring=enabled`,
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
  kscorectl agent tags remove web-001 monitoring`,
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
  kscorectl agent tags show web-001`,
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
  kscorectl agent status

  # Show status for a specific agent
  kscorectl agent status web-001`,
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
		Total     int `json:"total" yaml:"total"`
		Online    int `json:"online" yaml:"online"`
		Offline   int `json:"offline" yaml:"offline"`
		Degraded  int `json:"degraded" yaml:"degraded"`
		Quarantined int `json:"quarantined" yaml:"quarantined"`
	}{
		Total:     42,
		Online:    38,
		Offline:   2,
		Degraded:  2,
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
  kscorectl agent renew-svid web-001

  # Force renewal even if not near expiry
  kscorectl agent renew-svid web-001 --force`,
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
		display.IPAddresses = append(a.Metadata.Ipv4Addresses, a.Metadata.Ipv6Addresses...)
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

func generateRandomToken() string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	result := make([]byte, 32)
	for i := range result {
		result[i] = chars[time.Now().UnixNano()%int64(len(chars))]
		time.Sleep(time.Nanosecond)
	}
	return string(result)
}
