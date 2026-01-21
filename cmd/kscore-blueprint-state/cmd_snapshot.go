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

var snapshotCmd = &cobra.Command{
	Use:   "snapshot",
	Short: "Manage blueprint snapshots",
	Long: `Manage snapshots of blueprint state for rollback support.

Snapshots capture the state of a system before blueprint changes are applied,
allowing you to rollback to a known good state if needed.

Commands:
  list    - List available snapshots
  delete  - Delete a snapshot
  info    - Show detailed snapshot information`,
}

func init() {
	snapshotCmd.AddCommand(snapshotListCmd)
	snapshotCmd.AddCommand(snapshotDeleteCmd)
	snapshotCmd.AddCommand(snapshotInfoCmd)
}

var snapshotListCmd = &cobra.Command{
	Use:   "list [blueprint]",
	Short: "List available snapshots",
	Long: `List available snapshots for rollback.

Examples:
  # List all snapshots
  kscore-blueprint-state snapshot list

  # List snapshots for a specific blueprint
  kscore-blueprint-state snapshot list myorg/web-stack

  # Output as JSON
  kscore-blueprint-state snapshot list --json`,
	RunE: snapshotListExecute,
}

var (
	snapshotListDir        string
	snapshotListLimit      int
	snapshotListOutputJSON bool
)

func init() {
	snapshotListCmd.Flags().StringVar(&snapshotListDir, "dir", "", "Snapshot directory")
	snapshotListCmd.Flags().IntVar(&snapshotListLimit, "limit", 20, "Maximum snapshots to show")
	snapshotListCmd.Flags().BoolVar(&snapshotListOutputJSON, "json", false, "Output in JSON format")
}

func snapshotListExecute(cmd *cobra.Command, args []string) error {
	blueprintName := ""
	if len(args) > 0 {
		blueprintName = parseReference(args[0])
	}

	snapshotPath := snapshotListDir
	if snapshotPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		snapshotPath = filepath.Join(home, ".kscore", "snapshots")
	}

	snapshotManager, err := blueprint.NewSnapshotManager(&blueprint.SnapshotConfig{
		StorePath:                snapshotPath,
		MaxSnapshotsPerBlueprint: 100,
		MaxTotalSnapshots:        1000,
	})
	if err != nil {
		return fmt.Errorf("failed to create snapshot manager: %w", err)
	}

	agentID, err := os.Hostname()
	if err != nil {
		agentID = ""
	}

	snapshots, err := snapshotManager.ListSnapshots(agentID, blueprintName, "")
	if err != nil {
		return fmt.Errorf("failed to list snapshots: %w", err)
	}

	if len(snapshots) == 0 {
		fmt.Println("No snapshots found.")
		return nil
	}

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
  kscore-blueprint-state snapshot delete abc123def456`,
	Args: cobra.ExactArgs(1),
	RunE: snapshotDeleteExecute,
}

var snapshotDeleteDir string

func init() {
	snapshotDeleteCmd.Flags().StringVar(&snapshotDeleteDir, "dir", "", "Snapshot directory")
}

func snapshotDeleteExecute(cmd *cobra.Command, args []string) error {
	snapshotID := args[0]

	snapshotPath := snapshotDeleteDir
	if snapshotPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		snapshotPath = filepath.Join(home, ".kscore", "snapshots")
	}

	snapshotManager, err := blueprint.NewSnapshotManager(&blueprint.SnapshotConfig{
		StorePath: snapshotPath,
	})
	if err != nil {
		return fmt.Errorf("failed to create snapshot manager: %w", err)
	}

	if err := snapshotManager.DeleteSnapshot(snapshotID); err != nil {
		return fmt.Errorf("failed to delete snapshot: %w", err)
	}

	fmt.Printf("✓ Deleted snapshot %s\n", snapshotID)
	return nil
}

var snapshotInfoCmd = &cobra.Command{
	Use:   "info <snapshot-id>",
	Short: "Show detailed snapshot information",
	Long: `Show detailed information about a specific snapshot.

Examples:
  # Show snapshot details
  kscore-blueprint-state snapshot info abc123def456

  # Output as JSON
  kscore-blueprint-state snapshot info abc123def456 --json`,
	Args: cobra.ExactArgs(1),
	RunE: snapshotInfoExecute,
}

var (
	snapshotInfoDir  string
	snapshotInfoJSON bool
)

func init() {
	snapshotInfoCmd.Flags().StringVar(&snapshotInfoDir, "dir", "", "Snapshot directory")
	snapshotInfoCmd.Flags().BoolVar(&snapshotInfoJSON, "json", false, "Output in JSON format")
}

func snapshotInfoExecute(cmd *cobra.Command, args []string) error {
	snapshotID := args[0]

	// Get snapshot directory
	snapshotPath := snapshotInfoDir
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

	// Get snapshot
	snapshot, err := snapshotManager.GetSnapshot(snapshotID)
	if err != nil {
		return fmt.Errorf("failed to get snapshot: %w", err)
	}

	if snapshotInfoJSON {
		data, _ := json.MarshalIndent(snapshot, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	fmt.Printf("Snapshot Details:\n\n")
	fmt.Printf("  ID:              %s\n", snapshot.ID)
	fmt.Printf("  Blueprint:       %s\n", snapshot.BlueprintName)
	fmt.Printf("  Version:         %s\n", snapshot.BlueprintVersion)
	fmt.Printf("  Agent:           %s\n", snapshot.AgentID)
	fmt.Printf("  Created:         %s\n", snapshot.CreatedAt.Format(time.RFC3339))
	fmt.Printf("  Size:            %d bytes\n", snapshot.Size)
	if snapshot.Checksum != "" {
		fmt.Printf("  Checksum:        %s\n", snapshot.Checksum)
	}
	fmt.Println()

	if snapshot.StateCapture != nil {
		fmt.Printf("State Capture:\n")
		if len(snapshot.StateCapture.Files) > 0 {
			fmt.Printf("  Files: %d\n", len(snapshot.StateCapture.Files))
			for i, f := range snapshot.StateCapture.Files {
				if i >= 5 {
					fmt.Printf("    ... and %d more\n", len(snapshot.StateCapture.Files)-5)
					break
				}
				fmt.Printf("    - %s\n", f.Path)
			}
		}
		if len(snapshot.StateCapture.Packages) > 0 {
			fmt.Printf("  Packages: %d\n", len(snapshot.StateCapture.Packages))
			for i, p := range snapshot.StateCapture.Packages {
				if i >= 5 {
					fmt.Printf("    ... and %d more\n", len(snapshot.StateCapture.Packages)-5)
					break
				}
				fmt.Printf("    - %s (%s)\n", p.Name, p.Version)
			}
		}
		if len(snapshot.StateCapture.Services) > 0 {
			fmt.Printf("  Services: %d\n", len(snapshot.StateCapture.Services))
			for i, s := range snapshot.StateCapture.Services {
				if i >= 5 {
					fmt.Printf("    ... and %d more\n", len(snapshot.StateCapture.Services)-5)
					break
				}
				state := "stopped"
				if s.Running {
					state = "running"
				}
				fmt.Printf("    - %s (%s)\n", s.Name, state)
			}
		}
	}

	if len(snapshot.Metadata) > 0 {
		fmt.Printf("\nMetadata:\n")
		for k, v := range snapshot.Metadata {
			fmt.Printf("  %s: %v\n", k, v)
		}
	}

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
