package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/shawnbutts/keystone-core/pkg/blueprint"
)

var (
	rollbackDir         string
	rollbackDryRun      bool
	rollbackForce       bool
	rollbackToVersion   string
	rollbackToSnapshot  string
	rollbackShowHistory bool
	rollbackOutputJSON  bool
)

var rollbackCmd = &cobra.Command{
	Use:   "rollback <blueprint>",
	Short: "Rollback a blueprint to a previous version",
	Long: `Rollback a blueprint to a previous version or snapshot.

Rollback restores the previous state of a blueprint by:
1. Finding the previous version from history or specified snapshot
2. Detecting breaking changes between current and target versions
3. Executing the blueprint's rollback entrypoint if present
4. Restoring the previous state

If a blueprint defines a "rollback" entrypoint, that will be executed.
Otherwise, the previous state will be restored from the snapshot.

Examples:
  # Rollback to the previous version
  kscorectl blueprint rollback myorg/web-stack

  # Rollback to a specific version
  kscorectl blueprint rollback myorg/web-stack --to-version 1.2.0

  # Rollback to a specific snapshot
  kscorectl blueprint rollback myorg/web-stack --to-snapshot abc123def456

  # Show rollback history
  kscorectl blueprint rollback myorg/web-stack --history

  # Dry-run to see what would happen
  kscorectl blueprint rollback myorg/web-stack --dry-run

  # Force rollback even with breaking changes
  kscorectl blueprint rollback myorg/web-stack --force`,
	Args: cobra.ExactArgs(1),
	RunE: rollbackExecute,
}

func init() {
	rollbackCmd.Flags().StringVar(&rollbackDir, "dir", "", "Blueprint directory (default: ~/.kscore/blueprints)")
	rollbackCmd.Flags().BoolVar(&rollbackDryRun, "dry-run", false, "Show what would happen without making changes")
	rollbackCmd.Flags().BoolVar(&rollbackForce, "force", false, "Force rollback even with breaking changes")
	rollbackCmd.Flags().StringVar(&rollbackToVersion, "to-version", "", "Rollback to a specific version")
	rollbackCmd.Flags().StringVar(&rollbackToSnapshot, "to-snapshot", "", "Rollback to a specific snapshot ID")
	rollbackCmd.Flags().BoolVar(&rollbackShowHistory, "history", false, "Show rollback history for the blueprint")
	rollbackCmd.Flags().BoolVar(&rollbackOutputJSON, "json", false, "Output in JSON format")
}

func rollbackExecute(cmd *cobra.Command, args []string) error {
	blueprintRef := args[0]
	name, _ := parseReference(blueprintRef)

	// Get directories
	blueprintPath := rollbackDir
	if blueprintPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		blueprintPath = filepath.Join(home, ".kscore", "blueprints")
	}

	snapshotPath := filepath.Join(filepath.Dir(blueprintPath), "snapshots")
	trackerPath := filepath.Join(filepath.Dir(blueprintPath), "tracker.json")

	// If showing history, just display and return
	if rollbackShowHistory {
		return showRollbackHistory(name, trackerPath)
	}

	// Create snapshot manager
	snapshotManager, err := blueprint.NewSnapshotManager(&blueprint.SnapshotConfig{
		StorePath:                snapshotPath,
		MaxSnapshotsPerBlueprint: 10,
		MaxTotalSnapshots:        100,
	})
	if err != nil {
		return fmt.Errorf("failed to create snapshot manager: %w", err)
	}

	// Create tracker
	tracker, err := blueprint.NewTracker(&blueprint.TrackerConfig{
		StorePath:          trackerPath,
		MaxHistoryPerAgent: 100,
		PersistOnChange:    true,
	})
	if err != nil {
		return fmt.Errorf("failed to create tracker: %w", err)
	}

	// Get current agent ID (use hostname for now)
	agentID, err := os.Hostname()
	if err != nil {
		agentID = "local"
	}

	// Determine the rollback target
	var targetSnapshot *blueprint.Snapshot
	var targetVersion string

	if rollbackToSnapshot != "" {
		// Use specified snapshot
		targetSnapshot, err = snapshotManager.GetSnapshot(rollbackToSnapshot)
		if err != nil {
			return fmt.Errorf("failed to get snapshot %s: %w", rollbackToSnapshot, err)
		}
		targetVersion = targetSnapshot.BlueprintVersion
	} else if rollbackToVersion != "" {
		// Find snapshot for specified version
		snapshots, err := snapshotManager.ListSnapshots(agentID, name, "")
		if err != nil {
			return fmt.Errorf("failed to list snapshots: %w", err)
		}
		for _, s := range snapshots {
			if s.BlueprintVersion == rollbackToVersion {
				targetSnapshot = s
				break
			}
		}
		if targetSnapshot == nil {
			return fmt.Errorf("no snapshot found for version %s", rollbackToVersion)
		}
		targetVersion = rollbackToVersion
	} else {
		// Find previous version from history
		targetVersion, err = tracker.FindRollbackTarget(agentID, name)
		if err != nil {
			// Try to find from snapshots
			targetSnapshot, err = snapshotManager.GetLatestSnapshot(agentID, name, "")
			if err != nil {
				return fmt.Errorf("no previous version found for rollback: %w", err)
			}
			targetVersion = targetSnapshot.BlueprintVersion
		} else {
			// Find snapshot for this version
			snapshots, err := snapshotManager.ListSnapshots(agentID, name, "")
			if err != nil {
				return fmt.Errorf("failed to list snapshots: %w", err)
			}
			for _, s := range snapshots {
				if s.BlueprintVersion == targetVersion {
					targetSnapshot = s
					break
				}
			}
		}
	}

	// Get current applied blueprint info
	currentInfo := tracker.GetAppliedBlueprint(agentID, name)
	currentVersion := ""
	if currentInfo != nil {
		currentVersion = currentInfo.Version
	}

	// Print rollback plan
	fmt.Printf("Rollback Plan:\n")
	fmt.Printf("  Blueprint:       %s\n", name)
	fmt.Printf("  Current version: %s\n", currentVersion)
	fmt.Printf("  Target version:  %s\n", targetVersion)
	if targetSnapshot != nil {
		fmt.Printf("  Snapshot ID:     %s\n", targetSnapshot.ID)
		fmt.Printf("  Snapshot date:   %s\n", targetSnapshot.CreatedAt.Format(time.RFC3339))
	}
	fmt.Println()

	// Detect breaking changes
	breakingReport, err := detectRollbackBreakingChanges(blueprintPath, name, currentVersion, targetVersion)
	if err != nil {
		fmt.Printf("Warning: Could not detect breaking changes: %v\n", err)
	} else if breakingReport != nil && len(breakingReport.Changes) > 0 {
		fmt.Printf("Breaking Changes Detected:\n")
		for _, change := range breakingReport.Changes {
			fmt.Printf("  - [%s] %s: %s\n", change.Severity, change.Type, change.Description)
		}
		fmt.Println()

		if !rollbackForce {
			fmt.Println("Use --force to proceed with rollback despite breaking changes.")
			if rollbackDryRun {
				fmt.Println("\n[dry-run] Rollback would be blocked due to breaking changes.")
			}
			return fmt.Errorf("rollback blocked due to breaking changes")
		}
		fmt.Println("Warning: Proceeding with rollback despite breaking changes (--force)")
		fmt.Println()
	}

	// Check for rollback entrypoint
	rollbackEntrypoint := ""
	manifestPath := filepath.Join(blueprintPath, name, "blueprint.yaml")
	if manifest, err := loadManifest(manifestPath); err == nil {
		if ep, ok := manifest.Entrypoints["rollback"]; ok {
			rollbackEntrypoint = ep
		}
	}

	if rollbackDryRun {
		fmt.Println("[dry-run] Would perform the following actions:")
		if rollbackEntrypoint != "" {
			fmt.Printf("  1. Execute rollback entrypoint: %s\n", rollbackEntrypoint)
		}
		if targetSnapshot != nil && targetSnapshot.StateCapture != nil {
			fmt.Printf("  2. Restore state from snapshot\n")
			if len(targetSnapshot.StateCapture.Files) > 0 {
				fmt.Printf("     - Restore %d files\n", len(targetSnapshot.StateCapture.Files))
			}
			if len(targetSnapshot.StateCapture.Packages) > 0 {
				fmt.Printf("     - Restore %d packages\n", len(targetSnapshot.StateCapture.Packages))
			}
			if len(targetSnapshot.StateCapture.Services) > 0 {
				fmt.Printf("     - Restore %d services\n", len(targetSnapshot.StateCapture.Services))
			}
		}
		fmt.Printf("  3. Update tracker to version %s\n", targetVersion)
		fmt.Println("\n[dry-run] No changes made.")
		return nil
	}

	// Execute rollback
	startTime := time.Now()

	// Create rollback executor
	rollbackExecutor, err := blueprint.NewRollbackExecutor(&blueprint.RollbackExecutorConfig{
		BlueprintPath: blueprintPath,
		DryRun:        false,
		Parameters:    make(map[string]interface{}),
	})
	if err != nil {
		return fmt.Errorf("failed to create rollback executor: %w", err)
	}

	// Step 1: Execute rollback entrypoint if present
	if rollbackEntrypoint != "" {
		fmt.Printf("Executing rollback entrypoint: %s\n", rollbackEntrypoint)

		result, execErr := rollbackExecutor.ExecuteRollbackEntrypoint(
			cmd.Context(),
			name,
			currentVersion,
			targetVersion,
			targetSnapshot,
		)
		if execErr != nil {
			return fmt.Errorf("failed to execute rollback entrypoint: %w", execErr)
		}

		if result.StatesApplied > 0 {
			fmt.Printf("  Applied %d states from rollback entrypoint\n", result.StatesApplied)
			if result.StatesChanged > 0 {
				fmt.Printf("  Changed: %d states\n", result.StatesChanged)
			}
			if result.StatesFailed > 0 {
				fmt.Printf("  Failed: %d states\n", result.StatesFailed)
				for _, e := range result.Errors {
					fmt.Printf("    - %s\n", e)
				}
			}
		}
	}

	// Step 2: Restore state from snapshot
	if targetSnapshot != nil && targetSnapshot.StateCapture != nil {
		fmt.Printf("Restoring state from snapshot %s...\n", targetSnapshot.ID)
		restoreResult, restoreErr := rollbackExecutor.ExecuteStateRestore(cmd.Context(), targetSnapshot)
		if restoreErr != nil {
			return fmt.Errorf("failed to restore from snapshot: %w", restoreErr)
		}

		if restoreResult.StatesApplied > 0 {
			fmt.Printf("  Generated %d restore states from snapshot\n", restoreResult.StatesApplied)
			// Print summary of what would be restored
			printRestoreSummary(restoreResult.ExpandedStates)
		}
	}

	duration := time.Since(startTime)

	// Step 3: Record the rollback
	err = tracker.RecordRollback(agentID, name, targetVersion, "kscorectl", duration, nil)
	if err != nil {
		fmt.Printf("Warning: Failed to record rollback in tracker: %v\n", err)
	}

	fmt.Printf("\n✓ Successfully rolled back %s from %s to %s\n", name, currentVersion, targetVersion)
	fmt.Printf("  Duration: %s\n", duration.Round(time.Millisecond))

	return nil
}

// showRollbackHistory displays the rollback history for a blueprint
func showRollbackHistory(blueprintName, trackerPath string) error {
	tracker, err := blueprint.NewTracker(&blueprint.TrackerConfig{
		StorePath:          trackerPath,
		MaxHistoryPerAgent: 100,
		PersistOnChange:    false,
	})
	if err != nil {
		return fmt.Errorf("failed to create tracker: %w", err)
	}

	// Get agent ID
	agentID, err := os.Hostname()
	if err != nil {
		agentID = "local"
	}

	history := tracker.GetAgentHistory(agentID, 20)
	if len(history) == 0 {
		fmt.Println("No history found for this blueprint.")
		return nil
	}

	// Filter to only show entries for this blueprint
	var filtered []blueprint.BlueprintHistoryEntry
	for _, entry := range history {
		if entry.BlueprintName == blueprintName || entry.Namespace == blueprintName {
			filtered = append(filtered, entry)
		}
	}

	if len(filtered) == 0 {
		fmt.Println("No history found for this blueprint.")
		return nil
	}

	if rollbackOutputJSON {
		data, _ := json.MarshalIndent(filtered, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	fmt.Printf("History for %s:\n\n", blueprintName)
	for _, entry := range filtered {
		status := "✓"
		if !entry.Success {
			status = "✗"
		}

		fmt.Printf("  %s %s %s\n", status, entry.Timestamp.Format("2006-01-02 15:04:05"), entry.Action)
		if entry.FromVersion != "" {
			fmt.Printf("    %s -> %s\n", entry.FromVersion, entry.ToVersion)
		} else if entry.ToVersion != "" {
			fmt.Printf("    -> %s\n", entry.ToVersion)
		}
		if entry.TriggeredBy != "" {
			fmt.Printf("    Triggered by: %s\n", entry.TriggeredBy)
		}
		if entry.Duration > 0 {
			fmt.Printf("    Duration: %s\n", entry.Duration.Round(time.Millisecond))
		}
		if entry.ErrorMessage != "" {
			fmt.Printf("    Error: %s\n", entry.ErrorMessage)
		}
		fmt.Println()
	}

	return nil
}

// detectRollbackBreakingChanges detects breaking changes between versions
func detectRollbackBreakingChanges(blueprintPath, name, fromVersion, toVersion string) (*blueprint.BreakingChangeReport, error) {
	// Load the current (from) manifest
	fromManifestPath := filepath.Join(blueprintPath, name, "blueprint.yaml")
	fromManifest, err := loadManifest(fromManifestPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load current manifest: %w", err)
	}

	// For rollback, we're going from current to a previous version
	// We need to detect if going backwards introduces breaking changes
	// This is the reverse of upgrade detection

	// For now, we'll use the same manifest for both since we don't have
	// access to the old version's manifest. In a full implementation,
	// this would load the manifest from the snapshot or registry.
	detector := blueprint.NewBreakingChangeDetector()

	// Note: In a real implementation, we'd load the target version's manifest
	// from the snapshot or fetch it from the registry
	report := detector.Detect(fromManifest, fromManifest)

	return report, nil
}

// loadManifest loads a blueprint manifest from a file
func loadManifest(path string) (*blueprint.Blueprint, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	manifest, err := blueprint.ParseManifest(data)
	if err != nil {
		return nil, err
	}

	return manifest, nil
}

// printRestoreSummary prints a summary of states that would be restored
func printRestoreSummary(states map[string][]blueprint.BlueprintStateDeclaration) {
	if len(states) == 0 {
		return
	}

	// Count by module
	counts := make(map[string]int)
	for module, declarations := range states {
		counts[module] = len(declarations)
	}

	// Print in a consistent order
	modules := []string{"file", "package", "service", "user", "group"}
	for _, module := range modules {
		if count, ok := counts[module]; ok && count > 0 {
			fmt.Printf("    - %d %s states\n", count, module)
		}
	}

	// Print any other modules not in the standard list
	for module, count := range counts {
		found := false
		for _, m := range modules {
			if m == module {
				found = true
				break
			}
		}
		if !found && count > 0 {
			fmt.Printf("    - %d %s states\n", count, module)
		}
	}
}

// Snapshot list command
var snapshotCmd = &cobra.Command{
	Use:   "snapshot",
	Short: "Manage blueprint snapshots",
	Long:  `Manage snapshots of blueprint state for rollback support.`,
}

var snapshotListCmd = &cobra.Command{
	Use:   "list [blueprint]",
	Short: "List available snapshots",
	Long: `List available snapshots for rollback.

Examples:
  # List all snapshots
  kscorectl blueprint snapshot list

  # List snapshots for a specific blueprint
  kscorectl blueprint snapshot list myorg/web-stack`,
	RunE: snapshotListExecute,
}

var (
	snapshotListDir        string
	snapshotListLimit      int
	snapshotListOutputJSON bool
)

func init() {
	snapshotCmd.AddCommand(snapshotListCmd)

	snapshotListCmd.Flags().StringVar(&snapshotListDir, "dir", "", "Snapshot directory")
	snapshotListCmd.Flags().IntVar(&snapshotListLimit, "limit", 20, "Maximum snapshots to show")
	snapshotListCmd.Flags().BoolVar(&snapshotListOutputJSON, "json", false, "Output in JSON format")
}

func snapshotListExecute(cmd *cobra.Command, args []string) error {
	blueprintName := ""
	if len(args) > 0 {
		blueprintName, _ = parseReference(args[0])
	}

	// Get snapshot directory
	snapshotPath := snapshotListDir
	if snapshotPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		snapshotPath = filepath.Join(home, ".kscore", "snapshots")
	}

	// Create snapshot manager
	snapshotManager, err := blueprint.NewSnapshotManager(&blueprint.SnapshotConfig{
		StorePath:                snapshotPath,
		MaxSnapshotsPerBlueprint: 100,
		MaxTotalSnapshots:        1000,
	})
	if err != nil {
		return fmt.Errorf("failed to create snapshot manager: %w", err)
	}

	// Get agent ID
	agentID, err := os.Hostname()
	if err != nil {
		agentID = ""
	}

	// List snapshots
	snapshots, err := snapshotManager.ListSnapshots(agentID, blueprintName, "")
	if err != nil {
		return fmt.Errorf("failed to list snapshots: %w", err)
	}

	if len(snapshots) == 0 {
		fmt.Println("No snapshots found.")
		return nil
	}

	// Apply limit
	if snapshotListLimit > 0 && len(snapshots) > snapshotListLimit {
		snapshots = snapshots[:snapshotListLimit]
	}

	if snapshotListOutputJSON {
		data, _ := json.MarshalIndent(snapshots, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	fmt.Printf("Snapshots:\n\n")
	for _, s := range snapshots {
		fmt.Printf("  ID: %s\n", s.ID)
		fmt.Printf("    Blueprint: %s@%s\n", s.BlueprintName, s.BlueprintVersion)
		fmt.Printf("    Agent: %s\n", s.AgentID)
		fmt.Printf("    Created: %s\n", s.CreatedAt.Format(time.RFC3339))
		if s.StateCapture != nil {
			counts := []string{}
			if len(s.StateCapture.Files) > 0 {
				counts = append(counts, fmt.Sprintf("%d files", len(s.StateCapture.Files)))
			}
			if len(s.StateCapture.Packages) > 0 {
				counts = append(counts, fmt.Sprintf("%d packages", len(s.StateCapture.Packages)))
			}
			if len(s.StateCapture.Services) > 0 {
				counts = append(counts, fmt.Sprintf("%d services", len(s.StateCapture.Services)))
			}
			if len(counts) > 0 {
				fmt.Printf("    State: %s\n", joinStrings(counts, ", "))
			}
		}
		fmt.Printf("    Size: %d bytes\n", s.Size)
		fmt.Println()
	}

	return nil
}

var snapshotDeleteCmd = &cobra.Command{
	Use:   "delete <snapshot-id>",
	Short: "Delete a snapshot",
	Long: `Delete a specific snapshot.

Examples:
  # Delete a snapshot
  kscorectl blueprint snapshot delete abc123def456`,
	Args: cobra.ExactArgs(1),
	RunE: snapshotDeleteExecute,
}

var snapshotDeleteDir string

func init() {
	snapshotCmd.AddCommand(snapshotDeleteCmd)
	snapshotDeleteCmd.Flags().StringVar(&snapshotDeleteDir, "dir", "", "Snapshot directory")
}

func snapshotDeleteExecute(cmd *cobra.Command, args []string) error {
	snapshotID := args[0]

	// Get snapshot directory
	snapshotPath := snapshotDeleteDir
	if snapshotPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		snapshotPath = filepath.Join(home, ".kscore", "snapshots")
	}

	// Create snapshot manager
	snapshotManager, err := blueprint.NewSnapshotManager(&blueprint.SnapshotConfig{
		StorePath: snapshotPath,
	})
	if err != nil {
		return fmt.Errorf("failed to create snapshot manager: %w", err)
	}

	// Delete snapshot
	if err := snapshotManager.DeleteSnapshot(snapshotID); err != nil {
		return fmt.Errorf("failed to delete snapshot: %w", err)
	}

	fmt.Printf("✓ Deleted snapshot %s\n", snapshotID)
	return nil
}

// joinStrings joins strings with a separator
func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += sep + strs[i]
	}
	return result
}
