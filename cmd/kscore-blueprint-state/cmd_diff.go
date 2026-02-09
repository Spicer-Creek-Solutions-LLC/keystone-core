// Package main implements the kscore-blueprint-state CLI for managing blueprint state snapshots.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/spf13/cobra"

	"github.com/shawnbutts/keystone-core/internal/blueprint"
)

var diffCmd = &cobra.Command{
	Use:   "diff <snapshot1> <snapshot2>",
	Short: "Compare two snapshots",
	Long: `Compare two snapshots to see what changed between them.

Shows differences in files, packages, services, and other captured state
between two snapshots.

Examples:
  # Compare two snapshots
  kscore-blueprint-state diff abc123 def456

  # Output as JSON
  kscore-blueprint-state diff abc123 def456 --json

  # Show only changed files
  kscore-blueprint-state diff abc123 def456 --files-only`,
	Args: cobra.ExactArgs(2),
	RunE: diffExecute,
}

var (
	diffDir       string
	diffJSON      bool
	diffFilesOnly bool
)

func init() {
	diffCmd.Flags().StringVar(&diffDir, "dir", "", "Snapshot directory")
	diffCmd.Flags().BoolVar(&diffJSON, "json", false, "Output in JSON format")
	diffCmd.Flags().BoolVar(&diffFilesOnly, "files-only", false, "Show only file differences")
}

// DiffResult represents the differences between two snapshots
type DiffResult struct {
	Snapshot1 string      `json:"snapshot1"`
	Snapshot2 string      `json:"snapshot2"`
	Files     FileDiff    `json:"files,omitempty"`
	Packages  PackageDiff `json:"packages,omitempty"`
	Services  ServiceDiff `json:"services,omitempty"`
	Summary   DiffSummary `json:"summary"`
}

type FileDiff struct {
	Added    []string           `json:"added,omitempty"`
	Removed  []string           `json:"removed,omitempty"`
	Modified []FileModification `json:"modified,omitempty"`
}

type FileModification struct {
	Path    string `json:"path"`
	OldHash string `json:"old_hash,omitempty"`
	NewHash string `json:"new_hash,omitempty"`
	OldMode string `json:"old_mode,omitempty"`
	NewMode string `json:"new_mode,omitempty"`
}

type PackageDiff struct {
	Added   []PackageInfo   `json:"added,omitempty"`
	Removed []PackageInfo   `json:"removed,omitempty"`
	Changed []PackageChange `json:"changed,omitempty"`
}

type PackageInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type PackageChange struct {
	Name       string `json:"name"`
	OldVersion string `json:"old_version"`
	NewVersion string `json:"new_version"`
}

type ServiceDiff struct {
	Added   []ServiceInfo   `json:"added,omitempty"`
	Removed []ServiceInfo   `json:"removed,omitempty"`
	Changed []ServiceChange `json:"changed,omitempty"`
}

type ServiceInfo struct {
	Name  string `json:"name"`
	State string `json:"state"`
}

type ServiceChange struct {
	Name     string `json:"name"`
	OldState string `json:"old_state"`
	NewState string `json:"new_state"`
}

type DiffSummary struct {
	FilesAdded      int `json:"files_added"`
	FilesRemoved    int `json:"files_removed"`
	FilesModified   int `json:"files_modified"`
	PackagesAdded   int `json:"packages_added"`
	PackagesRemoved int `json:"packages_removed"`
	PackagesChanged int `json:"packages_changed"`
	ServicesAdded   int `json:"services_added"`
	ServicesRemoved int `json:"services_removed"`
	ServicesChanged int `json:"services_changed"`
}

func diffExecute(cmd *cobra.Command, args []string) error {
	snapshot1ID := args[0]
	snapshot2ID := args[1]

	snapshotPath := diffDir
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

	snapshot1, err := snapshotManager.GetSnapshot(snapshot1ID)
	if err != nil {
		return fmt.Errorf("failed to get snapshot %s: %w", snapshot1ID, err)
	}

	snapshot2, err := snapshotManager.GetSnapshot(snapshot2ID)
	if err != nil {
		return fmt.Errorf("failed to get snapshot %s: %w", snapshot2ID, err)
	}

	result := compareSnapshots(snapshot1, snapshot2)

	if diffJSON {
		data, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	printDiff(result)
	return nil
}

func compareSnapshots(s1, s2 *blueprint.Snapshot) *DiffResult {
	result := &DiffResult{
		Snapshot1: s1.ID,
		Snapshot2: s2.ID,
	}

	if s1.StateCapture != nil && s2.StateCapture != nil {
		result.Files = compareFiles(s1.StateCapture.Files, s2.StateCapture.Files)
		result.Packages = comparePackages(s1.StateCapture.Packages, s2.StateCapture.Packages)
		result.Services = compareServices(s1.StateCapture.Services, s2.StateCapture.Services)
	}

	result.Summary = DiffSummary{
		FilesAdded:      len(result.Files.Added),
		FilesRemoved:    len(result.Files.Removed),
		FilesModified:   len(result.Files.Modified),
		PackagesAdded:   len(result.Packages.Added),
		PackagesRemoved: len(result.Packages.Removed),
		PackagesChanged: len(result.Packages.Changed),
		ServicesAdded:   len(result.Services.Added),
		ServicesRemoved: len(result.Services.Removed),
		ServicesChanged: len(result.Services.Changed),
	}

	return result
}

func compareFiles(files1, files2 []blueprint.FileCaptureEntry) FileDiff {
	diff := FileDiff{}

	map1 := make(map[string]blueprint.FileCaptureEntry)
	map2 := make(map[string]blueprint.FileCaptureEntry)

	for i := range files1 {
		map1[files1[i].Path] = files1[i]
	}
	for i := range files2 {
		map2[files2[i].Path] = files2[i]
	}

	for path := range map2 {
		if _, exists := map1[path]; !exists {
			diff.Added = append(diff.Added, path)
		}
	}

	for path := range map1 {
		if _, exists := map2[path]; !exists {
			diff.Removed = append(diff.Removed, path)
		}
	}

	for path := range map1 {
		f1 := map1[path]
		if f2, exists := map2[path]; exists {
			if f1.Checksum != f2.Checksum || f1.Mode != f2.Mode {
				diff.Modified = append(diff.Modified, FileModification{
					Path:    path,
					OldHash: f1.Checksum,
					NewHash: f2.Checksum,
					OldMode: fmt.Sprintf("%o", f1.Mode),
					NewMode: fmt.Sprintf("%o", f2.Mode),
				})
			}
		}
	}

	sort.Strings(diff.Added)
	sort.Strings(diff.Removed)
	sort.Slice(diff.Modified, func(i, j int) bool {
		return diff.Modified[i].Path < diff.Modified[j].Path
	})

	return diff
}

func comparePackages(pkgs1, pkgs2 []blueprint.PackageCaptureEntry) PackageDiff {
	diff := PackageDiff{}

	map1 := make(map[string]blueprint.PackageCaptureEntry)
	map2 := make(map[string]blueprint.PackageCaptureEntry)

	for _, p := range pkgs1 {
		map1[p.Name] = p
	}
	for _, p := range pkgs2 {
		map2[p.Name] = p
	}

	for name, p := range map2 {
		if _, exists := map1[name]; !exists {
			diff.Added = append(diff.Added, PackageInfo{
				Name:    name,
				Version: p.Version,
			})
		}
	}

	for name, p := range map1 {
		if _, exists := map2[name]; !exists {
			diff.Removed = append(diff.Removed, PackageInfo{
				Name:    name,
				Version: p.Version,
			})
		}
	}

	for name, p1 := range map1 {
		if p2, exists := map2[name]; exists {
			if p1.Version != p2.Version {
				diff.Changed = append(diff.Changed, PackageChange{
					Name:       name,
					OldVersion: p1.Version,
					NewVersion: p2.Version,
				})
			}
		}
	}

	sort.Slice(diff.Added, func(i, j int) bool {
		return diff.Added[i].Name < diff.Added[j].Name
	})
	sort.Slice(diff.Removed, func(i, j int) bool {
		return diff.Removed[i].Name < diff.Removed[j].Name
	})
	sort.Slice(diff.Changed, func(i, j int) bool {
		return diff.Changed[i].Name < diff.Changed[j].Name
	})

	return diff
}

func compareServices(svcs1, svcs2 []blueprint.ServiceCaptureEntry) ServiceDiff {
	diff := ServiceDiff{}

	map1 := make(map[string]blueprint.ServiceCaptureEntry)
	map2 := make(map[string]blueprint.ServiceCaptureEntry)

	for _, s := range svcs1 {
		map1[s.Name] = s
	}
	for _, s := range svcs2 {
		map2[s.Name] = s
	}

	for name, s := range map2 {
		if _, exists := map1[name]; !exists {
			diff.Added = append(diff.Added, ServiceInfo{
				Name:  name,
				State: serviceState(s),
			})
		}
	}

	for name, s := range map1 {
		if _, exists := map2[name]; !exists {
			diff.Removed = append(diff.Removed, ServiceInfo{
				Name:  name,
				State: serviceState(s),
			})
		}
	}

	for name, s1 := range map1 {
		if s2, exists := map2[name]; exists {
			state1 := serviceState(s1)
			state2 := serviceState(s2)
			if state1 != state2 {
				diff.Changed = append(diff.Changed, ServiceChange{
					Name:     name,
					OldState: state1,
					NewState: state2,
				})
			}
		}
	}

	sort.Slice(diff.Added, func(i, j int) bool {
		return diff.Added[i].Name < diff.Added[j].Name
	})
	sort.Slice(diff.Removed, func(i, j int) bool {
		return diff.Removed[i].Name < diff.Removed[j].Name
	})
	sort.Slice(diff.Changed, func(i, j int) bool {
		return diff.Changed[i].Name < diff.Changed[j].Name
	})

	return diff
}

func printDiff(result *DiffResult) {
	fmt.Printf("Diff between %s and %s:\n\n", result.Snapshot1, result.Snapshot2)

	hasChanges := false

	if !diffFilesOnly || len(result.Files.Added) > 0 || len(result.Files.Removed) > 0 || len(result.Files.Modified) > 0 {
		if len(result.Files.Added) > 0 {
			hasChanges = true
			fmt.Printf("Files Added (%d):\n", len(result.Files.Added))
			for _, f := range result.Files.Added {
				fmt.Printf("  + %s\n", f)
			}
			fmt.Println()
		}

		if len(result.Files.Removed) > 0 {
			hasChanges = true
			fmt.Printf("Files Removed (%d):\n", len(result.Files.Removed))
			for _, f := range result.Files.Removed {
				fmt.Printf("  - %s\n", f)
			}
			fmt.Println()
		}

		if len(result.Files.Modified) > 0 {
			hasChanges = true
			fmt.Printf("Files Modified (%d):\n", len(result.Files.Modified))
			for _, f := range result.Files.Modified {
				fmt.Printf("  ~ %s\n", f.Path)
				if f.OldHash != f.NewHash {
					fmt.Printf("      hash: %s -> %s\n", truncateHash(f.OldHash), truncateHash(f.NewHash))
				}
				if f.OldMode != f.NewMode {
					fmt.Printf("      mode: %s -> %s\n", f.OldMode, f.NewMode)
				}
			}
			fmt.Println()
		}
	}

	if !diffFilesOnly {
		if len(result.Packages.Added) > 0 {
			hasChanges = true
			fmt.Printf("Packages Added (%d):\n", len(result.Packages.Added))
			for _, p := range result.Packages.Added {
				fmt.Printf("  + %s (%s)\n", p.Name, p.Version)
			}
			fmt.Println()
		}

		if len(result.Packages.Removed) > 0 {
			hasChanges = true
			fmt.Printf("Packages Removed (%d):\n", len(result.Packages.Removed))
			for _, p := range result.Packages.Removed {
				fmt.Printf("  - %s (%s)\n", p.Name, p.Version)
			}
			fmt.Println()
		}

		if len(result.Packages.Changed) > 0 {
			hasChanges = true
			fmt.Printf("Packages Changed (%d):\n", len(result.Packages.Changed))
			for _, p := range result.Packages.Changed {
				fmt.Printf("  ~ %s: %s -> %s\n", p.Name, p.OldVersion, p.NewVersion)
			}
			fmt.Println()
		}

		if len(result.Services.Added) > 0 {
			hasChanges = true
			fmt.Printf("Services Added (%d):\n", len(result.Services.Added))
			for _, s := range result.Services.Added {
				fmt.Printf("  + %s (%s)\n", s.Name, s.State)
			}
			fmt.Println()
		}

		if len(result.Services.Removed) > 0 {
			hasChanges = true
			fmt.Printf("Services Removed (%d):\n", len(result.Services.Removed))
			for _, s := range result.Services.Removed {
				fmt.Printf("  - %s (%s)\n", s.Name, s.State)
			}
			fmt.Println()
		}

		if len(result.Services.Changed) > 0 {
			hasChanges = true
			fmt.Printf("Services Changed (%d):\n", len(result.Services.Changed))
			for _, s := range result.Services.Changed {
				fmt.Printf("  ~ %s: %s -> %s\n", s.Name, s.OldState, s.NewState)
			}
			fmt.Println()
		}
	}

	if !hasChanges {
		fmt.Println("No differences found.")
		return
	}

	fmt.Printf("Summary:\n")
	fmt.Printf("  Files:    +%d -%d ~%d\n",
		result.Summary.FilesAdded,
		result.Summary.FilesRemoved,
		result.Summary.FilesModified)
	if !diffFilesOnly {
		fmt.Printf("  Packages: +%d -%d ~%d\n",
			result.Summary.PackagesAdded,
			result.Summary.PackagesRemoved,
			result.Summary.PackagesChanged)
		fmt.Printf("  Services: +%d -%d ~%d\n",
			result.Summary.ServicesAdded,
			result.Summary.ServicesRemoved,
			result.Summary.ServicesChanged)
	}
}

func truncateHash(hash string) string {
	if len(hash) > 12 {
		return hash[:12]
	}
	return hash
}

func serviceState(s blueprint.ServiceCaptureEntry) string {
	switch {
	case s.Running && s.Enabled:
		return "running, enabled"
	case s.Running:
		return "running, disabled"
	case s.Enabled:
		return "stopped, enabled"
	default:
		return "stopped, disabled"
	}
}
