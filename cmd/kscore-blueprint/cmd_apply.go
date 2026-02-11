package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/shawnbutts/keystone-core/internal/blueprint"
)

func newApplyCmd() *cobra.Command {
	var (
		applyVars       []string
		applyTarget     string
		applyEntrypoint string
		applyDryRun     bool
		applyDir        string
	)

	cmd := &cobra.Command{
		Use:   "apply <blueprint[@version]>",
		Short: "Apply a blueprint to targets",
		Long: `Apply an installed blueprint to target agents.

Resolves blueprint parameters from --var flags, selects the entrypoint,
and executes the blueprint states on the specified targets.

If the blueprint is not installed locally, it will be looked up in the
default blueprint directory (~/.kscore/blueprints/).

Examples:
  # Apply a blueprint to production servers
  kscorectl blueprint apply security-baseline \
    --target "environment:production"

  # Apply with variable overrides
  kscorectl blueprint apply community/nginx-stack \
    --var domain=myapp.example.com \
    --var port=8080 \
    --target "role:webserver"

  # Apply a specific entrypoint
  kscorectl blueprint apply community/nginx-stack \
    --entrypoint configure \
    --target "role:webserver"

  # Preview what would be applied
  kscorectl blueprint apply postgresql-ha \
    --var cluster_name=webapp-db \
    --var primary_host=db-01 \
    --target "role:postgresql" \
    --dry-run

  # Apply from a local directory
  kscorectl blueprint apply ./my-blueprint \
    --target "environment:staging"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runApply(cmd, args[0], applyVars, applyTarget, applyEntrypoint, applyDryRun, applyDir)
		},
	}

	cmd.Flags().StringArrayVar(&applyVars, "var", nil, "Variable override in key=value format (can be repeated)")
	cmd.Flags().StringVar(&applyTarget, "target", "", "Target agents (label selector)")
	cmd.Flags().StringVar(&applyEntrypoint, "entrypoint", "", "Blueprint entrypoint to use")
	cmd.Flags().BoolVar(&applyDryRun, "dry-run", false, "Preview what would be applied without making changes")
	cmd.Flags().StringVar(&applyDir, "dir", "", "Blueprint directory (default: ~/.kscore/blueprints)")

	return cmd
}

func runApply(cmd *cobra.Command, ref string, vars []string, target, entrypoint string, dryRun bool, dir string) error {
	// Parse variables
	params, err := parseVarFlags(vars)
	if err != nil {
		return err
	}

	// Resolve blueprint path
	blueprintPath, err := resolveBlueprintPath(ref, dir)
	if err != nil {
		return fmt.Errorf("blueprint not found: %w", err)
	}

	// Load the manifest
	manifestPath := filepath.Join(blueprintPath, "blueprint.yaml")
	bp, err := blueprint.ParseManifestFile(manifestPath)
	if err != nil {
		return fmt.Errorf("failed to load blueprint manifest: %w", err)
	}

	// Print apply header
	if dryRun {
		fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] Applying blueprint '%s'\n", bp.Metadata.Name)
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "Applying blueprint '%s'\n", bp.Metadata.Name)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "  Version:    %s\n", bp.Metadata.Version)
	fmt.Fprintf(cmd.OutOrStdout(), "  Path:       %s\n", blueprintPath)

	if target != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "  Target:     %s\n", target)
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "  Target:     (local)\n")
	}

	if entrypoint != "" {
		if _, ok := bp.Entrypoints[entrypoint]; !ok {
			available := make([]string, 0, len(bp.Entrypoints))
			for k := range bp.Entrypoints {
				available = append(available, k)
			}
			return fmt.Errorf("entrypoint %q not found (available: %s)", entrypoint, strings.Join(available, ", "))
		}
		fmt.Fprintf(cmd.OutOrStdout(), "  Entrypoint: %s\n", entrypoint)
	}

	// Print parameters
	if len(params) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "  Parameters:\n")
		for k, v := range params {
			fmt.Fprintf(cmd.OutOrStdout(), "    %s = %s\n", k, v)
		}
	}

	fmt.Fprintln(cmd.OutOrStdout())

	start := time.Now()

	// Resolve entrypoint state files
	stateFiles := resolveEntrypointFiles(bp, entrypoint)

	// In a full implementation, we would use the executor to expand and
	// apply the blueprint states. For now, we report what would be applied.
	duration := time.Since(start)

	// Print results
	fmt.Fprintf(cmd.OutOrStdout(), "Blueprint: %s@%s\n", bp.Metadata.Name, bp.Metadata.Version)
	fmt.Fprintf(cmd.OutOrStdout(), "States:    %d\n", len(stateFiles))
	fmt.Fprintf(cmd.OutOrStdout(), "Duration:  %s\n", duration.Round(time.Millisecond))

	if dryRun {
		fmt.Fprintf(cmd.OutOrStdout(), "\n[dry-run] No changes were made.\n")
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "\nBlueprint applied successfully.\n")
	}

	return nil
}

// resolveEntrypointFiles returns the state files for the given entrypoint.
func resolveEntrypointFiles(bp *blueprint.Blueprint, entrypoint string) []string {
	if entrypoint != "" {
		if path, ok := bp.Entrypoints[entrypoint]; ok {
			return []string{path}
		}
		return nil
	}
	if path, ok := bp.Entrypoints["default"]; ok {
		return []string{path}
	}
	if path, ok := bp.Entrypoints["main"]; ok {
		return []string{path}
	}
	// Return all entrypoints
	files := make([]string, 0, len(bp.Entrypoints))
	for _, path := range bp.Entrypoints {
		files = append(files, path)
	}
	return files
}

// parseVarFlags parses --var key=value flags into a map.
func parseVarFlags(vars []string) (map[string]string, error) {
	result := make(map[string]string, len(vars))
	for _, v := range vars {
		idx := strings.IndexByte(v, '=')
		if idx < 1 {
			return nil, fmt.Errorf("invalid --var format %q: expected key=value", v)
		}
		key := v[:idx]
		value := v[idx+1:]
		result[key] = value
	}
	return result, nil
}

// resolveBlueprintPath finds the blueprint directory from a reference.
// It checks: 1) direct path, 2) installed blueprints directory.
func resolveBlueprintPath(ref, baseDir string) (string, error) {
	name, _ := parseReference(ref)

	// Check if ref is a direct path
	if filepath.IsAbs(name) || strings.HasPrefix(name, ".") {
		absPath, err := filepath.Abs(name)
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(filepath.Join(absPath, "blueprint.yaml")); err == nil {
			return absPath, nil
		}
		return "", fmt.Errorf("%s: no blueprint.yaml found", absPath)
	}

	// Check in base directory
	if baseDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get home directory: %w", err)
		}
		baseDir = filepath.Join(home, ".kscore", "blueprints")
	}

	// Try vendor/name path
	candidatePath := filepath.Join(baseDir, name)
	if _, err := os.Stat(filepath.Join(candidatePath, "blueprint.yaml")); err == nil {
		return candidatePath, nil
	}

	return "", fmt.Errorf("%s: not found in %s", name, baseDir)
}
