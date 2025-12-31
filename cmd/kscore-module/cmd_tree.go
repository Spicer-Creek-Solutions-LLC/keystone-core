package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/spf13/cobra"

	"github.com/shawnbutts/keystone-core/pkg/module/manifest"
)

var (
	treeDepth int
	treeFlat  bool
)

var treeCmd = &cobra.Command{
	Use:   "tree [path]",
	Short: "Display dependency tree",
	Long: `Display the dependency tree for a module.

Reads module.yaml and module.lock to show:
  - Direct dependencies
  - Transitive dependencies
  - Version constraints vs resolved versions

Examples:
  # Show dependency tree
  kscorectl module tree

  # Limit depth
  kscorectl module tree --depth 2

  # Flat list (no tree structure)
  kscorectl module tree --flat`,
	Args: cobra.MaximumNArgs(1),
	RunE: treeExecute,
}

func init() {
	treeCmd.Flags().IntVar(&treeDepth, "depth", 0, "Maximum depth to display (0 = unlimited)")
	treeCmd.Flags().BoolVar(&treeFlat, "flat", false, "Show as flat list instead of tree")
}

func treeExecute(cmd *cobra.Command, args []string) error {
	// Determine path
	modulePath := "."
	if len(args) > 0 {
		modulePath = args[0]
	}

	absPath, err := filepath.Abs(modulePath)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	// Parse module.yaml
	manifestPath := filepath.Join(absPath, "module.yaml")
	m, err := manifest.ParseFile(manifestPath)
	if err != nil {
		return fmt.Errorf("failed to parse module.yaml: %w", err)
	}

	// Try to parse lock file
	lockFilePath := filepath.Join(absPath, "module.lock")
	var lockFile *manifest.LockFile
	if _, err := os.Stat(lockFilePath); err == nil {
		lockFile, err = manifest.ParseLockFileFromFile(lockFilePath)
		if err != nil {
			fmt.Printf("Warning: failed to parse lock file: %v\n", err)
		}
	}

	// Print root
	fmt.Printf("%s@%s\n", m.Name, m.Version)

	if len(m.Dependencies) == 0 {
		fmt.Println("└── (no dependencies)")
		return nil
	}

	if treeFlat {
		return printFlatDeps(m, lockFile)
	}

	return printTreeDeps(m, lockFile, 0)
}

func printFlatDeps(m *manifest.Manifest, lockFile *manifest.LockFile) error {
	// Collect all dependencies
	type depInfo struct {
		name       string
		constraint string
		resolved   string
		hash       string
	}

	var deps []depInfo

	// Direct dependencies
	for name, constraint := range m.Dependencies {
		info := depInfo{
			name:       name,
			constraint: constraint,
		}
		if lockFile != nil {
			if locked, ok := lockFile.Modules[name]; ok {
				info.resolved = locked.Version
				info.hash = locked.Hash
			}
		}
		deps = append(deps, info)
	}

	// Transitive dependencies from lock file
	if lockFile != nil {
		for name, locked := range lockFile.Modules {
			// Skip if already in direct deps
			if _, ok := m.Dependencies[name]; ok {
				continue
			}
			deps = append(deps, depInfo{
				name:       name,
				constraint: "(transitive)",
				resolved:   locked.Version,
				hash:       locked.Hash,
			})
		}
	}

	// Sort by name
	sort.Slice(deps, func(i, j int) bool {
		return deps[i].name < deps[j].name
	})

	// Print
	fmt.Println()
	fmt.Printf("%-40s %-15s %-15s %s\n", "NAME", "CONSTRAINT", "RESOLVED", "HASH")
	fmt.Printf("%-40s %-15s %-15s %s\n", "----", "----------", "--------", "----")

	for _, dep := range deps {
		resolved := dep.resolved
		if resolved == "" {
			resolved = "(unresolved)"
		}
		hash := dep.hash
		if len(hash) > 12 {
			hash = hash[:12] + "..."
		}
		fmt.Printf("%-40s %-15s %-15s %s\n", dep.name, dep.constraint, resolved, hash)
	}

	fmt.Printf("\nTotal: %d dependencies\n", len(deps))
	return nil
}

func printTreeDeps(m *manifest.Manifest, lockFile *manifest.LockFile, depth int) error {
	// Get sorted dependency names
	var names []string
	for name := range m.Dependencies {
		names = append(names, name)
	}
	sort.Strings(names)

	// Print each dependency
	for i, name := range names {
		isLast := i == len(names)-1
		constraint := m.Dependencies[name]

		// Get resolved version from lock file
		var resolved string
		if lockFile != nil {
			if locked, ok := lockFile.Modules[name]; ok {
				resolved = locked.Version
			}
		}

		// Print the dependency
		prefix := "├── "
		if isLast {
			prefix = "└── "
		}

		if resolved != "" && resolved != constraint {
			fmt.Printf("%s%s@%s (constraint: %s)\n", prefix, name, resolved, constraint)
		} else if resolved != "" {
			fmt.Printf("%s%s@%s\n", prefix, name, resolved)
		} else {
			fmt.Printf("%s%s@%s (unresolved)\n", prefix, name, constraint)
		}

		// In a full implementation, we would recursively print transitive deps
		// This requires the dependency graph from the resolver
		// For now, we just show direct dependencies
	}

	// Show transitive deps summary if we have a lock file
	if lockFile != nil {
		transitive := 0
		for name := range lockFile.Modules {
			if _, ok := m.Dependencies[name]; !ok {
				transitive++
			}
		}
		if transitive > 0 {
			fmt.Printf("\n(+ %d transitive dependencies in lock file)\n", transitive)
		}
	}

	return nil
}
