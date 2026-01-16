package bootstrap

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/shawnbutts/keystone-core/pkg/blueprint"
	"github.com/shawnbutts/keystone-core/pkg/statemgmt"
)

const defaultBlueprintsDir = "/etc/kscore/blueprints"

func blueprintPhase(ctx context.Context, state *State) error {
	if state.BootstrapConfig == nil {
		return nil
	}

	cfg := state.BootstrapConfig
	if cfg.BlueprintsDir == "" && len(cfg.ApplyBlueprints) == 0 {
		return nil
	}

	blueprintDir := cfg.BlueprintsDir
	if blueprintDir == "" {
		blueprintDir = defaultBlueprintsDir
	}
	exportDir := strings.TrimSpace(cfg.ExportStatesDir)
	if exportDir != "" {
		if err := os.MkdirAll(exportDir, 0o755); err != nil {
			return fmt.Errorf("create export dir: %w", err)
		}
	}

	storage, err := blueprint.NewLocalStorage(blueprintDir, true)
	if err != nil {
		return err
	}
	defer storage.Close()

	if len(cfg.ApplyBlueprints) == 0 {
		if state.Verbose || state.DryRun {
			return listBlueprints(ctx, storage, state.Output)
		}
		return nil
	}

	loader := blueprint.NewLoader(storage)
	executor := statemgmt.NewExecutor()
	executor.DryRun = state.DryRun

	tmpDir, err := os.MkdirTemp("", "kscore-blueprints-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	for _, ref := range cfg.ApplyBlueprints {
		name, version := blueprint.ParseBlueprintReference(ref)
		normalized := normalizeBlueprintKey(ref)
		entrypoint := cfg.BlueprintEntrypoints[normalized]
		params := cfg.BlueprintParams[normalized]
		features := cfg.BlueprintFeatures[normalized]
		loadConfig := &blueprint.LoadConfig{
			Name:       name,
			Version:    version,
			Entrypoint: entrypoint,
			Parameters: params,
			Features:   features,
			Validate:   true,
		}

		result, err := loader.Load(ctx, loadConfig)
		if err != nil {
			return fmt.Errorf("load blueprint %s: %w", ref, err)
		}

		if state.Verbose || state.DryRun {
			fmt.Fprintf(state.Output, "blueprint %s loaded (%d state files)\n", result.Blueprint.FullName(), len(result.StateFiles))
		}

		if exportDir != "" {
			if err := exportBlueprintStates(ctx, state, loader, result, exportDir); err != nil {
				return err
			}
			continue
		}
		if err := applyBlueprintStates(ctx, state, loader, executor, result, tmpDir); err != nil {
			return err
		}
	}

	return nil
}

func applyBlueprintStates(ctx context.Context, state *State, loader *blueprint.Loader, executor *statemgmt.Executor, result *blueprint.LoadResult, tmpDir string) error {
	blueprintName := result.Blueprint.FullName()
	hooks := result.Blueprint.Hooks
	applyErr := runHookStates(ctx, state, loader, executor, result, tmpDir, hooks, "pre_apply")
	if applyErr == nil {
		applyErr = applyStateFiles(ctx, state, loader, executor, result, tmpDir, result.StateFiles, "apply")
	}
	if applyErr == nil {
		applyErr = runHookStates(ctx, state, loader, executor, result, tmpDir, hooks, "post_apply")
	}
	if applyErr == nil {
		if verifyPath := result.Blueprint.Entrypoints["verify"]; verifyPath != "" {
			applyErr = applyStateFiles(ctx, state, loader, executor, result, tmpDir, []string{verifyPath}, "verify")
		}
	}
	if applyErr == nil {
		return nil
	}

	if hooks != nil {
		if rollbackErr := runHookStates(ctx, state, loader, executor, result, tmpDir, hooks, "pre_rollback"); rollbackErr != nil {
			return fmt.Errorf("blueprint %s failed: %v (pre-rollback error: %v)", blueprintName, applyErr, rollbackErr)
		}
		if rollbackErr := runHookStates(ctx, state, loader, executor, result, tmpDir, hooks, "post_rollback"); rollbackErr != nil {
			return fmt.Errorf("blueprint %s failed: %v (post-rollback error: %v)", blueprintName, applyErr, rollbackErr)
		}
	}

	return fmt.Errorf("blueprint %s failed: %w", blueprintName, applyErr)
}

func runHookStates(ctx context.Context, state *State, loader *blueprint.Loader, executor *statemgmt.Executor, result *blueprint.LoadResult, tmpDir string, hooks *blueprint.Hooks, hookName string) error {
	if hooks == nil {
		return nil
	}
	var paths []string
	switch hookName {
	case "pre_apply":
		paths = hooks.PreApply
	case "post_apply":
		paths = hooks.PostApply
	case "pre_rollback":
		paths = hooks.PreRollback
	case "post_rollback":
		paths = hooks.PostRollback
	default:
		return nil
	}
	if len(paths) == 0 {
		return nil
	}
	return applyStateFiles(ctx, state, loader, executor, result, tmpDir, paths, hookName)
}

func applyStateFiles(ctx context.Context, state *State, loader *blueprint.Loader, executor *statemgmt.Executor, result *blueprint.LoadResult, tmpDir string, stateFiles []string, label string) error {
	parser := statemgmt.NewParser(tmpDir)
	for idx, stateFile := range stateFiles {
		rendered, err := loader.RenderState(ctx, result.Blueprint, stateFile, result.ResolvedParameters)
		if err != nil {
			return fmt.Errorf("render state %s: %w", stateFile, err)
		}

		filename := fmt.Sprintf("%s-%d-%s", result.Blueprint.Metadata.Name, idx, sanitizeStateFilename(stateFile))
		statePath := filepath.Join(tmpDir, filename)
		if err := os.WriteFile(statePath, rendered, 0o644); err != nil {
			return fmt.Errorf("write rendered state: %w", err)
		}

		parsedState, err := parser.ParseFile(statePath)
		if err != nil {
			return fmt.Errorf("parse rendered state: %w", err)
		}

		run, err := executor.ExecuteState(ctx, parsedState)
		if err != nil {
			return fmt.Errorf("execute state %s: %w", stateFile, err)
		}

		if state.Verbose || state.DryRun {
			fmt.Fprintf(state.Output, "%s state file %s (%d states, success=%t)\n", label, stateFile, len(run.Results), run.Summary.Success)
		}
	}

	return nil
}

func sanitizeStateFilename(path string) string {
	replacer := strings.NewReplacer("/", "_", "\\", "_")
	return replacer.Replace(strings.TrimPrefix(path, "./"))
}

func exportBlueprintStates(ctx context.Context, state *State, loader *blueprint.Loader, result *blueprint.LoadResult, exportDir string) error {
	paths := make([]string, 0, len(result.StateFiles)+4)
	if hooks := result.Blueprint.Hooks; hooks != nil {
		paths = append(paths, hooks.PreApply...)
		paths = append(paths, hooks.PostApply...)
		paths = append(paths, hooks.PreRollback...)
		paths = append(paths, hooks.PostRollback...)
	}
	paths = append(paths, result.StateFiles...)
	if verifyPath := result.Blueprint.Entrypoints["verify"]; verifyPath != "" {
		paths = append(paths, verifyPath)
	}
	paths = uniqueStatePaths(paths)
	for idx, stateFile := range paths {
		rendered, err := loader.RenderState(ctx, result.Blueprint, stateFile, result.ResolvedParameters)
		if err != nil {
			return fmt.Errorf("render state %s: %w", stateFile, err)
		}
		filename := fmt.Sprintf("%s-%d-%s", result.Blueprint.Metadata.Name, idx, sanitizeStateFilename(stateFile))
		statePath := filepath.Join(exportDir, filename)
		if err := os.WriteFile(statePath, rendered, 0o644); err != nil {
			return fmt.Errorf("write rendered state: %w", err)
		}
		if state.Verbose || state.DryRun {
			fmt.Fprintf(state.Output, "exported state file %s\n", statePath)
		}
	}
	return nil
}

func uniqueStatePaths(paths []string) []string {
	seen := make(map[string]bool, len(paths))
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		trimmed := strings.TrimSpace(path)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		result = append(result, trimmed)
	}
	return result
}

func listBlueprints(ctx context.Context, storage blueprint.Storage, output io.Writer) error {
	blueprints, err := storage.List(ctx, nil)
	if err != nil {
		return err
	}
	if len(blueprints) == 0 {
		fmt.Fprintln(output, "no blueprints found")
		return nil
	}
	fmt.Fprintln(output, "available blueprints:")
	for _, bp := range blueprints {
		version := bp.Version
		if version == "" && len(bp.AvailableVersions) > 0 {
			version = bp.AvailableVersions[0]
		}
		if version != "" {
			fmt.Fprintf(output, "- %s@%s\n", bp.Name, version)
		} else {
			fmt.Fprintf(output, "- %s\n", bp.Name)
		}
	}
	return nil
}
