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
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show cluster status and health",
		Long: `Display the current status of the Keystone Core cluster.

Shows:
  - Overall cluster health
  - Member count and quorum status
  - Current leader
  - Individual member status`,
		RunE: runStatus,
	}

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

	if outputFormat == "json" || outputFormat == "yaml" {
		return outputResult(members, outputFormat)
	}

	// Table output
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	if showDetails {
		fmt.Fprintln(w, "ID\tADDRESS\tSTATUS\tLEADER\tVERSION\tAGENTS\tJOBS\tLAST SEEN")
		for _, m := range members {
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
		for _, m := range members {
			leader := ""
			if m.IsLeader {
				leader = "*"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", m.ID, m.Address, m.Status, leader)
		}
	}

	return w.Flush()
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
	cmd := &cobra.Command{
		Use:   "add <address>",
		Short: "Add a new member to the cluster",
		Long: `Add a new Keystone Core server to the cluster.

The new member must be running and accessible at the specified address.
The cluster will automatically rebalance agents after the new member joins.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAdd(args[0])
		},
	}

	return cmd
}

func runAdd(address string) error {
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

	cmd := &cobra.Command{
		Use:   "remove <member-id>",
		Short: "Remove a member from the cluster",
		Long: `Remove a Keystone Core server from the cluster.

The member's agents will be redistributed to remaining members.
Use --force to remove an unresponsive member.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRemove(args[0], force)
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Force remove even if member is unresponsive")

	return cmd
}

func runRemove(memberID string, force bool) error {
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
	cmd := &cobra.Command{
		Use:   "transfer-leader <member-id>",
		Short: "Transfer leadership to another member",
		Long: `Transfer cluster leadership to a specific member.

This is useful before performing maintenance on the current leader.
The specified member must be healthy and capable of becoming leader.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTransferLeader(args[0])
		},
	}

	return cmd
}

func runTransferLeader(targetID string) error {
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

	cmd := &cobra.Command{
		Use:   "rebalance",
		Short: "Rebalance agents across cluster members",
		Long: `Trigger a manual rebalancing of agents across cluster members.

This redistributes agent connections to achieve a more even load distribution.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRebalance(reason)
		},
	}

	cmd.Flags().StringVar(&reason, "reason", "CLI request", "Reason for rebalancing")

	return cmd
}

func runRebalance(reason string) error {
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
	var outputPath string

	cmd := &cobra.Command{
		Use:   "backup",
		Short: "Backup cluster state",
		Long: `Create a backup of the cluster state.

This creates a snapshot of:
  - Cluster configuration
  - Member information
  - Shard assignments
  - etcd data`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBackup(outputPath)
		},
	}

	cmd.Flags().StringVarP(&outputPath, "output", "f", "", "Output file path (default: stdout)")

	return cmd
}

func runBackup(outputPath string) error {
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

	if err := os.WriteFile(outputPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write backup file: %w", err)
	}

	fmt.Printf("Backup saved to: %s\n", outputPath)
	return nil
}

// newRestoreCommand creates the 'restore' command
func newRestoreCommand() *cobra.Command {
	var inputPath string
	var force bool

	cmd := &cobra.Command{
		Use:   "restore",
		Short: "Restore cluster state from backup",
		Long: `Restore cluster state from a backup file.

WARNING: This will overwrite the current cluster state.
Use --force to skip confirmation.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRestore(inputPath, force)
		},
	}

	cmd.Flags().StringVarP(&inputPath, "input", "f", "", "Input backup file path (required)")
	cmd.Flags().BoolVar(&force, "force", false, "Skip confirmation prompt")
	cmd.MarkFlagRequired("input")

	return cmd
}

func runRestore(inputPath string, force bool) error {
	if !force {
		fmt.Print("WARNING: This will overwrite the current cluster state. Continue? [y/N]: ")
		var response string
		fmt.Scanln(&response)
		if strings.ToLower(response) != "y" {
			fmt.Println("Restore cancelled")
			return nil
		}
	}

	data, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("failed to read backup file: %w", err)
	}

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
	default:
		// Table format - handled by each command
		return outputTable(v)
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
		for _, m := range val.Members {
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
	for _, m := range members {
		if m.Status == "healthy" {
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

// Sort members by ID for consistent output
func sortMembers(members []MemberStatus) {
	sort.Slice(members, func(i, j int) bool {
		return members[i].ID < members[j].ID
	})
}
