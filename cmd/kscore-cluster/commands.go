package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// ClusterStatus represents the overall cluster status
type ClusterStatus struct {
	Healthy     bool           `json:"healthy" yaml:"healthy"`
	MemberCount int            `json:"member_count" yaml:"member_count"`
	QuorumSize  int            `json:"quorum_size" yaml:"quorum_size"`
	HasQuorum   bool           `json:"has_quorum" yaml:"has_quorum"`
	LeaderID    string         `json:"leader_id" yaml:"leader_id"`
	Members     []MemberStatus `json:"members" yaml:"members"`
	UpdatedAt   time.Time      `json:"updated_at" yaml:"updated_at"`
}

// MemberStatus represents a single cluster member's status
type MemberStatus struct {
	ID         string    `json:"id" yaml:"id"`
	Address    string    `json:"address" yaml:"address"`
	Status     string    `json:"status" yaml:"status"`
	IsLeader   bool      `json:"is_leader" yaml:"is_leader"`
	Version    string    `json:"version" yaml:"version"`
	StartedAt  time.Time `json:"started_at" yaml:"started_at"`
	LastSeen   time.Time `json:"last_seen" yaml:"last_seen"`
	AgentCount int       `json:"agent_count" yaml:"agent_count"`
	JobCount   int       `json:"job_count" yaml:"job_count"`
}

// newStatusCommand creates the 'status' command
func newStatusCommand() *cobra.Command {
	var (
		watch  bool
		filter string
		format string
	)

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show cluster status and health",
		Long: `Display the current status of the Keystone Core cluster.

Shows:
  - Overall cluster health
  - Member count and quorum status
  - Current leader
  - Individual member status

Examples:
  kscorectl cluster status
  kscorectl cluster status --watch
  kscorectl cluster status --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if format != "" {
				outputFormat = format
			}
			return runStatus(cmd, args)
		},
	}

	cmd.Flags().BoolVarP(&watch, "watch", "w", false, "Watch for status changes")
	cmd.Flags().StringVar(&filter, "filter", "", "Filter expression for members")
	cmd.Flags().StringVar(&format, "format", "", "Output format (overrides global --output)")

	return cmd
}

func runStatus(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := newClusterClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w", err)
	}
	defer client.Close()

	status, err := client.GetStatus(ctx)
	if err != nil {
		return fmt.Errorf("failed to get cluster status: %w", err)
	}

	return outputResult(status, outputFormat)
}

// newMembersCommand creates the 'members' command
func newMembersCommand() *cobra.Command {
	var showDetails bool

	cmd := &cobra.Command{
		Use:   "members",
		Short: "List cluster members",
		Long: `List all members of the Keystone Core cluster.

Shows each member's:
  - ID and address
  - Status (healthy, unhealthy, unknown)
  - Whether it's the current leader
  - Agent and job counts`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMembers(showDetails)
		},
	}

	cmd.Flags().BoolVar(&showDetails, "details", false, "Show detailed member information")

	return cmd
}

func runMembers(showDetails bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := newClusterClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w", err)
	}
	defer client.Close()

	members, err := client.GetMembers(ctx)
	if err != nil {
		return fmt.Errorf("failed to get cluster members: %w", err)
	}

	switch outputFormat {
	case "json", "yaml":
		return outputResult(members, outputFormat)
	case "table", "text":
		// Table output
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

		if showDetails {
			fmt.Fprintln(w, "ID\tADDRESS\tSTATUS\tLEADER\tVERSION\tAGENTS\tJOBS\tLAST SEEN")
			for i := range members {
				m := &members[i]
				leader := ""
				if m.IsLeader {
					leader = "*"
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%d\t%d\t%s\n",
					m.ID, m.Address, m.Status, leader, m.Version,
					m.AgentCount, m.JobCount, formatDuration(time.Since(m.LastSeen)))
			}
		} else {
			fmt.Fprintln(w, "ID\tADDRESS\tSTATUS\tLEADER")
			for i := range members {
				m := &members[i]
				leader := ""
				if m.IsLeader {
					leader = "*"
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", m.ID, m.Address, m.Status, leader)
			}
		}

		return w.Flush()
	default:
		return fmt.Errorf("unsupported output format: %s", outputFormat)
	}
}

// newLeaderCommand creates the 'leader' command
func newLeaderCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "leader",
		Short: "Show current cluster leader",
		Long:  `Display information about the current cluster leader.`,
		RunE:  runLeader,
	}

	return cmd
}

func runLeader(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := newClusterClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w", err)
	}
	defer client.Close()

	leader, err := client.GetLeader(ctx)
	if err != nil {
		return fmt.Errorf("failed to get cluster leader: %w", err)
	}

	if leader == nil {
		fmt.Println("No leader elected")
		return nil
	}

	return outputResult(leader, outputFormat)
}

// newAddCommand creates the 'add' command
func newAddCommand() *cobra.Command {
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "add <address>",
		Short: "Add a new member to the cluster",
		Long: `Add a new Keystone Core server to the cluster.

The new member must be running and accessible at the specified address.
The cluster will automatically rebalance agents after the new member joins.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAdd(args[0], dryRun)
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be done without adding a member")

	return cmd
}

func runAdd(address string, dryRun bool) error {
	if dryRun {
		fmt.Printf("Dry run: would add cluster member at %s\n", address)
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := newClusterClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w", err)
	}
	defer client.Close()

	memberID, err := client.AddMember(ctx, address)
	if err != nil {
		return fmt.Errorf("failed to add member: %w", err)
	}

	fmt.Printf("Member added successfully: %s\n", memberID)
	return nil
}

// newRemoveCommand creates the 'remove' command
func newRemoveCommand() *cobra.Command {
	var force bool
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "remove <member-id>",
		Short: "Remove a member from the cluster",
		Long: `Remove a Keystone Core server from the cluster.

The member's agents will be redistributed to remaining members.
Use --force to remove an unresponsive member.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRemove(args[0], force, dryRun)
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Force remove even if member is unresponsive")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be done without removing the member")

	return cmd
}

func runRemove(memberID string, force, dryRun bool) error {
	if dryRun {
		fmt.Printf("Dry run: would remove member %s (force=%t)\n", memberID, force)
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := newClusterClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w", err)
	}
	defer client.Close()

	if err := client.RemoveMember(ctx, memberID, force); err != nil {
		return fmt.Errorf("failed to remove member: %w", err)
	}

	fmt.Printf("Member removed successfully: %s\n", memberID)
	return nil
}

// newTransferLeaderCommand creates the 'transfer-leader' command
func newTransferLeaderCommand() *cobra.Command {
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "transfer-leader <member-id>",
		Short: "Transfer leadership to another member",
		Long: `Transfer cluster leadership to a specific member.

This is useful before performing maintenance on the current leader.
The specified member must be healthy and capable of becoming leader.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTransferLeader(args[0], dryRun)
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be done without transferring leadership")

	return cmd
}

func runTransferLeader(targetID string, dryRun bool) error {
	if dryRun {
		fmt.Printf("Dry run: would transfer leadership to %s\n", targetID)
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := newClusterClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w", err)
	}
	defer client.Close()

	if err := client.TransferLeader(ctx, targetID); err != nil {
		return fmt.Errorf("failed to transfer leadership: %w", err)
	}

	fmt.Printf("Leadership transferred to: %s\n", targetID)
	return nil
}

// newRebalanceCommand creates the 'rebalance' command
func newRebalanceCommand() *cobra.Command {
	var reason string
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "rebalance",
		Short: "Rebalance agents across cluster members",
		Long: `Trigger a manual rebalancing of agents across cluster members.

This redistributes agent connections to achieve a more even load distribution.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRebalance(reason, dryRun)
		},
	}

	cmd.Flags().StringVar(&reason, "reason", "CLI request", "Reason for rebalancing")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be done without rebalancing")

	return cmd
}

func runRebalance(reason string, dryRun bool) error {
	if dryRun {
		fmt.Printf("Dry run: would rebalance agents (reason: %s)\n", reason)
		return nil
	}

	fmt.Println("Rebalancing agents...")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client, err := newClusterClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w", err)
	}
	defer client.Close()

	result, err := client.Rebalance(ctx, reason)
	if err != nil {
		return fmt.Errorf("failed to rebalance: %w", err)
	}

	if outputFormat == "table" {
		fmt.Printf("Rebalance completed:\n")
		fmt.Printf("  Reason:       %s\n", result.Reason)
		fmt.Printf("  Moved Agents: %d\n", result.MovedAgents)
		fmt.Printf("  Duration:     %s\n", result.Duration)
		return nil
	}

	return outputResult(result, outputFormat)
}

// newBackupCommand creates the 'backup' command
func newBackupCommand() *cobra.Command {
	var (
		outputPath string
		shardsOnly bool
		configOnly bool
	)

	cmd := &cobra.Command{
		Use:   "backup",
		Short: "Backup cluster state",
		Long: `Create a backup of the cluster state.

This creates a snapshot of:
  - Cluster configuration
  - Member information
  - Shard assignments
  - etcd data

Examples:
  kscorectl cluster backup --output backup.tar.gz
  kscorectl cluster backup --shards-only --output shards.json
  kscorectl cluster backup --config-only --output config.yaml`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBackup(outputPath, shardsOnly, configOnly)
		},
	}

	cmd.Flags().StringVarP(&outputPath, "output", "f", "", "Output file path (default: stdout)")
	cmd.Flags().BoolVar(&shardsOnly, "shards-only", false, "Backup only shard assignments")
	cmd.Flags().BoolVar(&configOnly, "config-only", false, "Backup only cluster configuration")
	cmd.MarkFlagsMutuallyExclusive("shards-only", "config-only")

	return cmd
}

func runBackup(outputPath string, shardsOnly, configOnly bool) error {
	backupType := "full"
	if shardsOnly {
		backupType = "shards"
	} else if configOnly {
		backupType = "config"
	}
	fmt.Printf("Creating cluster backup (%s)...\n", backupType)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	client, err := newClusterClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w", err)
	}
	defer client.Close()

	data, err := client.Backup(ctx)
	if err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}

	if outputPath == "" {
		os.Stdout.Write(data)
		return nil
	}

	if err := os.WriteFile(outputPath, data, 0o600); err != nil {
		return fmt.Errorf("failed to write backup file: %w", err)
	}

	fmt.Printf("Backup saved to: %s\n", outputPath)
	return nil
}

// newRestoreCommand creates the 'restore' command
func newRestoreCommand() *cobra.Command {
	var inputPath string
	var force bool
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "restore",
		Short: "Restore cluster state from backup",
		Long: `Restore cluster state from a backup file.

	WARNING: This will overwrite the current cluster state.
	Use --force to skip confirmation.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRestore(inputPath, force, dryRun)
		},
	}

	cmd.Flags().StringVarP(&inputPath, "input", "f", "", "Input backup file path (required)")
	cmd.Flags().BoolVar(&force, "force", false, "Skip confirmation prompt")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be done without restoring")
	cmd.MarkFlagRequired("input")

	return cmd
}

func runRestore(inputPath string, force, dryRun bool) error {
	if dryRun {
		fmt.Printf("Dry run: would restore cluster state from %s\n", inputPath)
		return nil
	}

	if !force {
		fmt.Print("WARNING: This will overwrite the current cluster state. Continue? [y/N]: ")
		var response string
		fmt.Scanln(&response)
		if !strings.EqualFold(response, "y") {
			fmt.Println("Restore cancelled")
			return nil
		}
	}

	data, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("failed to read backup file: %w", err)
	}

	fmt.Println("Restoring cluster state...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	client, err := newClusterClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w", err)
	}
	defer client.Close()

	if err := client.Restore(ctx, data); err != nil {
		return fmt.Errorf("failed to restore backup: %w", err)
	}

	fmt.Println("Cluster state restored successfully")
	return nil
}

// Helper functions

func outputResult(v interface{}, format string) error {
	switch format {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(v)
	case "yaml":
		enc := yaml.NewEncoder(os.Stdout)
		return enc.Encode(v)
	case "table", "text":
		return outputTable(v)
	default:
		return fmt.Errorf("unsupported output format: %s", format)
	}
}

func outputTable(v interface{}) error {
	switch val := v.(type) {
	case *ClusterStatus:
		fmt.Printf("Cluster Health: %s\n", formatBool(val.Healthy, "Healthy", "Unhealthy"))
		fmt.Printf("Members:        %d/%d (quorum: %d)\n", countHealthy(val.Members), val.MemberCount, val.QuorumSize)
		fmt.Printf("Has Quorum:     %s\n", formatBool(val.HasQuorum, "Yes", "No"))
		fmt.Printf("Leader:         %s\n", val.LeaderID)
		fmt.Printf("Updated:        %s\n", val.UpdatedAt.Format(time.RFC3339))
		fmt.Println()

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "MEMBER\tSTATUS\tLEADER\tAGENTS\tJOBS")
		for i := range val.Members {
			m := &val.Members[i]
			leader := ""
			if m.IsLeader {
				leader = "*"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%d\n", m.ID, m.Status, leader, m.AgentCount, m.JobCount)
		}
		return w.Flush()
	case *MemberStatus:
		fmt.Printf("Member ID:   %s\n", val.ID)
		fmt.Printf("Address:     %s\n", val.Address)
		fmt.Printf("Status:      %s\n", val.Status)
		fmt.Printf("Is Leader:   %v\n", val.IsLeader)
		fmt.Printf("Version:     %s\n", val.Version)
		fmt.Printf("Started:     %s\n", val.StartedAt.Format(time.RFC3339))
		fmt.Printf("Last Seen:   %s ago\n", formatDuration(time.Since(val.LastSeen)))
		fmt.Printf("Agents:      %d\n", val.AgentCount)
		fmt.Printf("Jobs:        %d\n", val.JobCount)
		return nil
	default:
		return json.NewEncoder(os.Stdout).Encode(v)
	}
}

func formatBool(b bool, trueVal, falseVal string) string {
	if b {
		return trueVal
	}
	return falseVal
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return "just now"
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

func countHealthy(members []MemberStatus) int {
	count := 0
	for i := range members {
		if members[i].Status == "healthy" {
			count++
		}
	}
	return count
}

// RebalanceResult represents the result of a rebalance operation
type RebalanceResult struct {
	Success     bool      `json:"success" yaml:"success"`
	Reason      string    `json:"reason" yaml:"reason"`
	MovedAgents int       `json:"moved_agents" yaml:"moved_agents"`
	TriggerID   string    `json:"trigger_member_id" yaml:"trigger_member_id"`
	StartTime   time.Time `json:"start_time" yaml:"start_time"`
	EndTime     time.Time `json:"end_time" yaml:"end_time"`
	Duration    string    `json:"duration" yaml:"duration"`
}

// HealthReport represents detailed cluster health information
type HealthReport struct {
	Healthy           bool           `json:"healthy" yaml:"healthy"`
	Status            string         `json:"status" yaml:"status"`
	QuorumEstablished bool           `json:"quorum_established" yaml:"quorum_established"`
	LeaderElected     bool           `json:"leader_elected" yaml:"leader_elected"`
	LeaderID          string         `json:"leader_id" yaml:"leader_id"`
	EtcdConnected     bool           `json:"etcd_connected" yaml:"etcd_connected"`
	NATSConnected     bool           `json:"nats_connected" yaml:"nats_connected"`
	AllAgentsAssigned bool           `json:"all_agents_assigned" yaml:"all_agents_assigned"`
	MembersTotal      int            `json:"members_total" yaml:"members_total"`
	MembersHealthy    int            `json:"members_healthy" yaml:"members_healthy"`
	AgentsTotal       int            `json:"agents_total" yaml:"agents_total"`
	AgentDistribution map[string]int `json:"agent_distribution" yaml:"agent_distribution"`
	Checks            []HealthCheck  `json:"checks" yaml:"checks"`
	UpdatedAt         time.Time      `json:"updated_at" yaml:"updated_at"`
}

// HealthCheck represents a single health check result
type HealthCheck struct {
	Name    string `json:"name" yaml:"name"`
	Passed  bool   `json:"passed" yaml:"passed"`
	Message string `json:"message" yaml:"message"`
}

// ShardReport represents shard assignment information
type ShardReport struct {
	TotalShards int               `json:"total_shards" yaml:"total_shards"`
	TotalAgents int               `json:"total_agents" yaml:"total_agents"`
	Assignments []ShardAssignment `json:"assignments" yaml:"assignments"`
	UpdatedAt   time.Time         `json:"updated_at" yaml:"updated_at"`
}

// ShardAssignment represents the assignment of agents to a member
type ShardAssignment struct {
	MemberID   string   `json:"member_id" yaml:"member_id"`
	ShardRange string   `json:"shard_range" yaml:"shard_range"`
	AgentCount int      `json:"agent_count" yaml:"agent_count"`
	AgentIDs   []string `json:"agent_ids,omitempty" yaml:"agent_ids,omitempty"`
}

// Sort members by ID for consistent output
func sortMembers(members []MemberStatus) {
	sort.Slice(members, func(i, j int) bool {
		return members[i].ID < members[j].ID
	})
}

// newHealthCommand creates the 'health' command
func newHealthCommand() *cobra.Command {
	var verbose bool

	cmd := &cobra.Command{
		Use:   "health",
		Short: "Perform cluster health check",
		Long: `Perform a comprehensive health check of the cluster.

Checks:
  - Quorum established
  - Leader elected
  - etcd connectivity
  - NATS cluster connection
  - Agent assignments`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runHealth(verbose)
		},
	}

	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Show detailed health information")

	return cmd
}

func runHealth(verbose bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := newClusterClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w", err)
	}
	defer client.Close()

	health, err := client.GetHealth(ctx)
	if err != nil {
		return fmt.Errorf("failed to get cluster health: %w", err)
	}

	switch outputFormat {
	case "json", "yaml":
		return outputResult(health, outputFormat)
	case "table", "text":
		return outputHealthTable(health, verbose)
	default:
		return fmt.Errorf("unsupported output format: %s", outputFormat)
	}
}

func outputHealthTable(health *HealthReport, verbose bool) error {
	statusStr := "HEALTHY"
	if !health.Healthy {
		statusStr = "UNHEALTHY"
	}
	fmt.Printf("Cluster Health: %s\n\n", statusStr)

	// Output health checks
	for _, check := range health.Checks {
		checkMark := "✓"
		if !check.Passed {
			checkMark = "✗"
		}
		fmt.Printf("%s %s\n", checkMark, check.Message)
	}

	fmt.Println()
	fmt.Printf("Total agents: %d\n", health.AgentsTotal)

	if len(health.AgentDistribution) > 0 {
		parts := make([]string, 0, len(health.AgentDistribution))
		for member, count := range health.AgentDistribution {
			parts = append(parts, fmt.Sprintf("%s=%d", member, count))
		}
		sort.Strings(parts)
		fmt.Printf("Agent distribution: %s\n", strings.Join(parts, ", "))
	}

	if verbose {
		fmt.Printf("\nMembers: %d/%d healthy\n", health.MembersHealthy, health.MembersTotal)
		fmt.Printf("Leader: %s\n", health.LeaderID)
		fmt.Printf("Updated: %s\n", health.UpdatedAt.Format(time.RFC3339))
	}

	return nil
}

// newShardsCommand creates the 'shards' command
func newShardsCommand() *cobra.Command {
	var showAgents bool

	cmd := &cobra.Command{
		Use:   "shards",
		Short: "Show shard assignments",
		Long: `Display shard assignment information for the cluster.

Shows how agents are distributed across cluster members using consistent hashing.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runShards(showAgents)
		},
	}

	cmd.Flags().BoolVar(&showAgents, "show-agents", false, "Show individual agent IDs per shard")

	return cmd
}

func runShards(showAgents bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := newClusterClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w", err)
	}
	defer client.Close()

	shards, err := client.GetShards(ctx)
	if err != nil {
		return fmt.Errorf("failed to get shard assignments: %w", err)
	}

	switch outputFormat {
	case "json", "yaml":
		return outputResult(shards, outputFormat)
	case "table", "text":
		fmt.Printf("Total Shards: %d\n", shards.TotalShards)
		fmt.Printf("Total Agents: %d\n\n", shards.TotalAgents)

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "MEMBER\tSHARD RANGE\tAGENTS")
		for _, a := range shards.Assignments {
			fmt.Fprintf(w, "%s\t%s\t%d\n", a.MemberID, a.ShardRange, a.AgentCount)
		}
		w.Flush()

		if showAgents {
			fmt.Println()
			for _, a := range shards.Assignments {
				if len(a.AgentIDs) > 0 {
					fmt.Printf("%s:\n", a.MemberID)
					for _, id := range a.AgentIDs {
						fmt.Printf("  - %s\n", id)
					}
				}
			}
		}

		return nil
	default:
		return fmt.Errorf("unsupported output format: %s", outputFormat)
	}
}

// newJoinCommand creates the 'join' command
func newJoinCommand() *cobra.Command {
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "join <cluster-address>",
		Short: "Join an existing cluster",
		Long: `Join this node to an existing Keystone Core cluster.

The cluster address should be the address of any existing cluster member.
This node will be configured to join the cluster and start participating
in leader election and agent distribution.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runJoin(args[0], dryRun)
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be done without joining")

	return cmd
}

func runJoin(clusterAddr string, dryRun bool) error {
	if dryRun {
		fmt.Printf("Dry run: would join cluster at %s\n", clusterAddr)
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := newClusterClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w", err)
	}
	defer client.Close()

	if err := client.JoinCluster(ctx, clusterAddr); err != nil {
		return fmt.Errorf("failed to join cluster: %w", err)
	}

	fmt.Printf("Successfully joined cluster at %s\n", clusterAddr)
	return nil
}

// newLeaveCommand creates the 'leave' command
func newLeaveCommand() *cobra.Command {
	var force bool
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "leave",
		Short: "Leave the cluster",
		Long: `Remove this node from the cluster.

The node's agents will be redistributed to remaining members before leaving.
Use --force to leave immediately without graceful agent migration.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLeave(force, dryRun)
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Force leave without graceful migration")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be done without leaving")

	return cmd
}

func runLeave(force, dryRun bool) error {
	if dryRun {
		fmt.Printf("Dry run: would leave cluster (force=%t)\n", force)
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client, err := newClusterClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w", err)
	}
	defer client.Close()

	if err := client.LeaveCluster(ctx, force); err != nil {
		return fmt.Errorf("failed to leave cluster: %w", err)
	}

	fmt.Println("Successfully left the cluster")
	return nil
}

// newDrainCommand creates the 'drain' command
func newDrainCommand() *cobra.Command {
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "drain <member-id>",
		Short: "Drain agents from a cluster member",
		Long: `Drain all agents from a cluster member.

This moves all agents from the specified member to other healthy members.
Useful before performing maintenance on a server.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDrain(args[0], dryRun)
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be done without draining")

	return cmd
}

func runDrain(memberID string, dryRun bool) error {
	if dryRun {
		fmt.Printf("Dry run: would drain agents from %s\n", memberID)
		return nil
	}

	fmt.Printf("Draining agents from %s...\n", memberID)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	client, err := newClusterClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w", err)
	}
	defer client.Close()

	if err := client.DrainMember(ctx, memberID); err != nil {
		return fmt.Errorf("failed to drain member: %w", err)
	}

	fmt.Printf("Successfully drained agents from %s\n", memberID)
	return nil
}

// newUndrainCommand creates the 'undrain' command
func newUndrainCommand() *cobra.Command {
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "undrain <member-id>",
		Short: "Allow agents on a drained cluster member",
		Long: `Mark a cluster member as available for agent assignment again.

This reverses the effect of the drain command, allowing new agents to be
assigned to the member and existing agents to migrate back.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUndrain(args[0], dryRun)
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be done without undraining")

	return cmd
}

func runUndrain(memberID string, dryRun bool) error {
	if dryRun {
		fmt.Printf("Dry run: would undrain %s\n", memberID)
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := newClusterClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w", err)
	}
	defer client.Close()

	if err := client.UndrainMember(ctx, memberID); err != nil {
		return fmt.Errorf("failed to undrain member: %w", err)
	}

	fmt.Printf("Successfully undrained %s\n", memberID)
	return nil
}
