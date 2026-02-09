package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/spf13/cobra"

	"github.com/shawnbutts/keystone-core/internal/blueprint"
)

var appliedCmd = &cobra.Command{
	Use:   "applied",
	Short: "Show applied blueprint information",
	Long: `Show information about blueprints applied to agents.

Provides subcommands to list applied blueprints, view history,
and check usage across the fleet.

Subcommands:
  list    - List all applied blueprints on an agent
  show    - Show detailed info for a specific applied blueprint
  history - View blueprint application history
  usage   - Find which agents use a specific blueprint`,
}

// Applied list command

var (
	appliedListAgent      string
	appliedListOutputJSON bool
)

var appliedListCmd = &cobra.Command{
	Use:   "list",
	Short: "List applied blueprints on an agent",
	Long: `List all blueprints currently applied on an agent.

Shows namespace, blueprint name, version, status, and application time
for each applied blueprint.

Examples:
  # List all applied blueprints on an agent
  kscorectl blueprint applied list --agent agent-1

  # List applied blueprints on local host
  kscorectl blueprint applied list

  # Output as JSON
  kscorectl blueprint applied list --agent agent-1 --json`,
	RunE: appliedListExecute,
}

func init() {
	appliedCmd.AddCommand(appliedListCmd)
	appliedCmd.AddCommand(appliedShowCmd)
	appliedCmd.AddCommand(appliedHistoryCmd)
	appliedCmd.AddCommand(appliedUsageCmd)

	appliedListCmd.Flags().StringVar(&appliedListAgent, "agent", "", "Agent ID (default: local hostname)")
	appliedListCmd.Flags().BoolVar(&appliedListOutputJSON, "json", false, "Output in JSON format")
}

func appliedListExecute(cmd *cobra.Command, _ []string) error {
	tracker, agentID, err := loadTrackerAndAgent(appliedListAgent)
	if err != nil {
		return err
	}

	state := tracker.GetAgentState(agentID)
	if state == nil || len(state.AppliedBlueprints) == 0 {
		fmt.Printf("No blueprints applied on %s\n", agentID)
		return nil
	}

	if appliedListOutputJSON {
		data, err := json.MarshalIndent(state.AppliedBlueprints, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal JSON: %w", err)
		}
		fmt.Fprintln(cmd.OutOrStdout(), string(data))
		return nil
	}

	// Sort namespaces for consistent output
	namespaces := make([]string, 0, len(state.AppliedBlueprints))
	for ns := range state.AppliedBlueprints {
		namespaces = append(namespaces, ns)
	}
	sort.Strings(namespaces)

	// Print header
	fmt.Fprintf(cmd.OutOrStdout(), "%-12s %-30s %-10s %-10s %s\n",
		"NAMESPACE", "BLUEPRINT", "VERSION", "STATUS", "APPLIED AT")

	for _, ns := range namespaces {
		info := state.AppliedBlueprints[ns]
		fmt.Fprintf(cmd.OutOrStdout(), "%-12s %-30s %-10s %-10s %s\n",
			ns,
			info.Name,
			info.Version,
			info.Status,
			info.AppliedAt.Format(time.RFC3339),
		)
	}

	return nil
}

// Applied show command

var (
	appliedShowAgent     string
	appliedShowNamespace string
	appliedShowJSON      bool
)

var appliedShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show details for an applied blueprint",
	Long: `Show detailed information about a specific applied blueprint on an agent.

Displays full metadata including parameters, features, state counts,
and error information.

Examples:
  # Show detailed info for a specific applied blueprint
  kscorectl blueprint applied show --agent agent-1 --namespace web

  # Show as JSON
  kscorectl blueprint applied show --agent agent-1 --namespace web --json`,
	RunE: appliedShowExecute,
}

func init() {
	appliedShowCmd.Flags().StringVar(&appliedShowAgent, "agent", "", "Agent ID (default: local hostname)")
	appliedShowCmd.Flags().StringVar(&appliedShowNamespace, "namespace", "", "Blueprint namespace (required)")
	appliedShowCmd.Flags().BoolVar(&appliedShowJSON, "json", false, "Output in JSON format")
}

func appliedShowExecute(cmd *cobra.Command, _ []string) error {
	if appliedShowNamespace == "" {
		return fmt.Errorf("--namespace is required")
	}

	tracker, agentID, err := loadTrackerAndAgent(appliedShowAgent)
	if err != nil {
		return err
	}

	info := tracker.GetAppliedBlueprint(agentID, appliedShowNamespace)
	if info == nil {
		return fmt.Errorf("no blueprint applied in namespace %q on agent %s", appliedShowNamespace, agentID)
	}

	if appliedShowJSON {
		data, err := json.MarshalIndent(info, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal JSON: %w", err)
		}
		fmt.Fprintln(cmd.OutOrStdout(), string(data))
		return nil
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Name:            %s\n", info.Name)
	fmt.Fprintf(out, "Version:         %s\n", info.Version)
	fmt.Fprintf(out, "Namespace:       %s\n", info.Namespace)
	fmt.Fprintf(out, "Status:          %s\n", info.Status)
	fmt.Fprintf(out, "Applied At:      %s\n", info.AppliedAt.Format(time.RFC3339))
	if info.AppliedBy != "" {
		fmt.Fprintf(out, "Applied By:      %s\n", info.AppliedBy)
	}
	fmt.Fprintf(out, "States:          %d total, %d successful, %d failed\n",
		info.StateCount, info.SuccessfulStates, info.FailedStates)
	if info.Checksum != "" {
		fmt.Fprintf(out, "Checksum:        %s\n", info.Checksum)
	}
	if info.LastError != "" {
		fmt.Fprintf(out, "Last Error:      %s\n", info.LastError)
	}
	if len(info.EnabledFeatures) > 0 {
		fmt.Fprintf(out, "Features:        %s\n", joinStrings(info.EnabledFeatures, ", "))
	}
	if len(info.Parameters) > 0 {
		fmt.Fprintf(out, "Parameters:\n")
		paramNames := make([]string, 0, len(info.Parameters))
		for k := range info.Parameters {
			paramNames = append(paramNames, k)
		}
		sort.Strings(paramNames)
		for _, k := range paramNames {
			fmt.Fprintf(out, "  %s: %v\n", k, info.Parameters[k])
		}
	}

	return nil
}

// Applied history command

var (
	appliedHistoryAgent string
	appliedHistoryLimit int
	appliedHistoryJSON  bool
)

var appliedHistoryCmd = &cobra.Command{
	Use:   "history",
	Short: "View blueprint application history",
	Long: `View the history of blueprint applications on an agent.

Shows a chronological log of blueprint applies, updates, removals,
and rollbacks with timestamps, versions, and status.

Examples:
  # View history for an agent
  kscorectl blueprint applied history --agent agent-1 --limit 10

  # Export history for audit
  kscorectl blueprint applied history --agent agent-1 --json`,
	RunE: appliedHistoryExecute,
}

func init() {
	appliedHistoryCmd.Flags().StringVar(&appliedHistoryAgent, "agent", "", "Agent ID (default: local hostname)")
	appliedHistoryCmd.Flags().IntVar(&appliedHistoryLimit, "limit", 20, "Maximum number of history entries")
	appliedHistoryCmd.Flags().BoolVar(&appliedHistoryJSON, "json", false, "Output in JSON format")
	appliedHistoryCmd.Flags().StringP("output", "o", "", "Output format (json for JSON output)")
}

func appliedHistoryExecute(cmd *cobra.Command, _ []string) error {
	tracker, agentID, err := loadTrackerAndAgent(appliedHistoryAgent)
	if err != nil {
		return err
	}

	history := tracker.GetAgentHistory(agentID, appliedHistoryLimit)
	if len(history) == 0 {
		fmt.Printf("No history found for agent %s\n", agentID)
		return nil
	}

	// Check if --output json was passed
	outputFmt, _ := cmd.Flags().GetString("output")
	useJSON := appliedHistoryJSON || outputFmt == "json"

	if useJSON {
		data, err := json.MarshalIndent(history, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal JSON: %w", err)
		}
		fmt.Fprintln(cmd.OutOrStdout(), string(data))
		return nil
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "%-22s %-12s %-25s %-10s %-10s %s\n",
		"TIMESTAMP", "ACTION", "BLUEPRINT", "FROM", "TO", "SUCCESS")

	for i := range history {
		entry := &history[i]
		success := "yes"
		if !entry.Success {
			success = "no"
		}
		fmt.Fprintf(out, "%-22s %-12s %-25s %-10s %-10s %s\n",
			entry.Timestamp.Format("2006-01-02 15:04:05"),
			entry.Action,
			entry.BlueprintName,
			entry.FromVersion,
			entry.ToVersion,
			success,
		)
	}

	return nil
}

// Applied usage command

var (
	appliedUsageJSON bool
)

var appliedUsageCmd = &cobra.Command{
	Use:   "usage <blueprint>",
	Short: "Find which agents use a specific blueprint",
	Long: `Find which agents have a specific blueprint applied.

Shows all agents that have the specified blueprint applied, along with
version, namespace, status, and application time.

Examples:
  # Find which agents use a specific blueprint
  kscorectl blueprint applied usage myorg/web-stack

  # Output as JSON
  kscorectl blueprint applied usage myorg/web-stack --json`,
	Args: cobra.ExactArgs(1),
	RunE: appliedUsageExecute,
}

func init() {
	appliedUsageCmd.Flags().BoolVar(&appliedUsageJSON, "json", false, "Output in JSON format")
}

func appliedUsageExecute(cmd *cobra.Command, args []string) error {
	blueprintName := args[0]

	tracker, err := loadTracker()
	if err != nil {
		return err
	}

	usage := tracker.GetBlueprintUsage(blueprintName)
	if len(usage) == 0 {
		fmt.Printf("No agents found using blueprint %s\n", blueprintName)
		return nil
	}

	if appliedUsageJSON {
		data, err := json.MarshalIndent(usage, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal JSON: %w", err)
		}
		fmt.Fprintln(cmd.OutOrStdout(), string(data))
		return nil
	}

	// Sort agent IDs for consistent output
	agents := make([]string, 0, len(usage))
	for agent := range usage {
		agents = append(agents, agent)
	}
	sort.Strings(agents)

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "%-15s %-12s %-10s %-10s %s\n",
		"AGENT", "NAMESPACE", "VERSION", "STATUS", "APPLIED AT")

	for _, agent := range agents {
		info := usage[agent]
		fmt.Fprintf(out, "%-15s %-12s %-10s %-10s %s\n",
			agent,
			info.Namespace,
			info.Version,
			info.Status,
			info.AppliedAt.Format(time.RFC3339),
		)
	}

	return nil
}

// Helper functions

func getTrackerPath() string {
	if path := os.Getenv("KSCORE_TRACKER_PATH"); path != "" {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "/var/lib/keystone-core/blueprint-tracker.json"
	}
	return filepath.Join(home, ".kscore", "tracker.json")
}

func loadTracker() (*blueprint.Tracker, error) {
	trackerPath := getTrackerPath()
	tracker, err := blueprint.NewTracker(&blueprint.TrackerConfig{
		StorePath:          trackerPath,
		MaxHistoryPerAgent: 100,
		PersistOnChange:    false,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to load tracker data: %w", err)
	}
	return tracker, nil
}

func loadTrackerAndAgent(agentFlag string) (*blueprint.Tracker, string, error) {
	tracker, err := loadTracker()
	if err != nil {
		return nil, "", err
	}

	agentID := agentFlag
	if agentID == "" {
		agentID, err = os.Hostname()
		if err != nil {
			agentID = "local"
		}
	}

	return tracker, agentID, nil
}
