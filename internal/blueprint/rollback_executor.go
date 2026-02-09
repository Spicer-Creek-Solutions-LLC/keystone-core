// Package blueprint provides rollback entrypoint execution.
package blueprint

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// RollbackExecutorConfig configures the rollback executor.
type RollbackExecutorConfig struct {
	// BlueprintPath is the base path for blueprints.
	BlueprintPath string

	// DryRun if true, only reports what would happen.
	DryRun bool

	// Timeout for rollback execution.
	Timeout time.Duration

	// Parameters for the rollback entrypoint.
	Parameters map[string]interface{}
}

// DefaultRollbackExecutorConfig returns default configuration.
func DefaultRollbackExecutorConfig() *RollbackExecutorConfig {
	return &RollbackExecutorConfig{
		DryRun:     false,
		Timeout:    5 * time.Minute,
		Parameters: make(map[string]interface{}),
	}
}

// RollbackExecutor handles execution of rollback entrypoints.
type RollbackExecutor struct {
	config *RollbackExecutorConfig
	loader *Loader
}

// RollbackResult contains the result of a rollback execution.
type RollbackResult struct {
	// Success indicates if the rollback succeeded.
	Success bool `json:"success"`

	// Blueprint is the blueprint that was rolled back.
	Blueprint string `json:"blueprint"`

	// FromVersion is the version we rolled back from.
	FromVersion string `json:"from_version"`

	// ToVersion is the version we rolled back to.
	ToVersion string `json:"to_version"`

	// EntrypointExecuted is the rollback entrypoint that was executed.
	EntrypointExecuted string `json:"entrypoint_executed,omitempty"`

	// StatesApplied is the number of states applied during rollback.
	StatesApplied int `json:"states_applied"`

	// StatesChanged is the number of states that made changes.
	StatesChanged int `json:"states_changed"`

	// StatesFailed is the number of states that failed.
	StatesFailed int `json:"states_failed"`

	// Duration is how long the rollback took.
	Duration time.Duration `json:"duration"`

	// Errors are any errors that occurred.
	Errors []string `json:"errors,omitempty"`

	// ExpandedStates contains the parsed state declarations.
	ExpandedStates map[string][]StateDeclaration `json:"expanded_states,omitempty"`

	// RenderedContent is the raw rendered state file content (for dry-run).
	RenderedContent []byte `json:"-"`
}

// NewRollbackExecutor creates a new rollback executor.
func NewRollbackExecutor(config *RollbackExecutorConfig) (*RollbackExecutor, error) {
	if config == nil {
		config = DefaultRollbackExecutorConfig()
	}

	// Create local storage for blueprints
	var storage Storage
	if config.BlueprintPath != "" {
		var err error
		storage, err = NewLocalStorage(config.BlueprintPath, false)
		if err != nil {
			return nil, fmt.Errorf("failed to create storage: %w", err)
		}
	}

	loader := NewLoader(storage)

	return &RollbackExecutor{
		config: config,
		loader: loader,
	}, nil
}

// ExecuteRollbackEntrypoint executes the rollback entrypoint for a blueprint.
func (r *RollbackExecutor) ExecuteRollbackEntrypoint(ctx context.Context, blueprintName, fromVersion, toVersion string, snapshot *Snapshot) (*RollbackResult, error) {
	startTime := time.Now()

	result := &RollbackResult{
		Blueprint:   blueprintName,
		FromVersion: fromVersion,
		ToVersion:   toVersion,
	}

	// Apply timeout if configured
	if r.config.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.config.Timeout)
		defer cancel()
	}

	// Load the blueprint
	blueprintPath := filepath.Join(r.config.BlueprintPath, blueprintName)
	manifestPath := filepath.Join(blueprintPath, "blueprint.yaml")

	bp, err := ParseManifestFile(manifestPath)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("failed to load blueprint: %v", err))
		result.Duration = time.Since(startTime)
		return result, err
	}

	// Get the rollback entrypoint
	rollbackEntrypoint, ok := bp.Entrypoints["rollback"]
	if !ok {
		// No rollback entrypoint defined, not an error
		result.Duration = time.Since(startTime)
		result.Success = true
		return result, nil
	}

	result.EntrypointExecuted = rollbackEntrypoint

	// Build parameters for the rollback
	params := make(map[string]interface{})
	for k, v := range r.config.Parameters {
		params[k] = v
	}
	// Add rollback-specific context
	params["rollback"] = map[string]interface{}{
		"from_version": fromVersion,
		"to_version":   toVersion,
		"dry_run":      r.config.DryRun,
	}
	if snapshot != nil {
		params["rollback"].(map[string]interface{})["snapshot_id"] = snapshot.ID
		params["rollback"].(map[string]interface{})["snapshot_time"] = snapshot.CreatedAt
	}

	// Render the rollback state file
	rendered, err := r.renderRollbackState(ctx, bp, rollbackEntrypoint, params)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("failed to render rollback state: %v", err))
		result.Duration = time.Since(startTime)
		return result, err
	}

	result.RenderedContent = rendered

	// Parse the rendered state into declarations
	executor, err := NewExecutor(&ExecutorConfig{
		Loader: r.loader,
		DryRun: r.config.DryRun,
	})
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("failed to create executor: %v", err))
		result.Duration = time.Since(startTime)
		return result, err
	}

	states, err := executor.parseRenderedState(rendered)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("failed to parse rendered states: %v", err))
		result.Duration = time.Since(startTime)
		return result, err
	}

	result.ExpandedStates = states

	// Count total states
	for _, declarations := range states {
		result.StatesApplied += len(declarations)
	}

	// If dry-run, we're done
	if r.config.DryRun {
		result.Success = true
		result.Duration = time.Since(startTime)
		return result, nil
	}

	// Execute the states
	// Note: Full state execution would integrate with pkg/statemgmt
	// For now, we return the parsed states for external execution
	result.Success = true
	result.Duration = time.Since(startTime)

	return result, nil
}

// renderRollbackState renders a rollback state file with parameters.
func (r *RollbackExecutor) renderRollbackState(ctx context.Context, bp *Blueprint, statePath string, params map[string]interface{}) ([]byte, error) {
	// Build the full path to the state file to verify it exists
	fullPath := filepath.Join(bp.SourcePath, statePath)

	// Verify the state file exists
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("state file not found: %s", statePath)
	}

	// Use the loader to render the template
	return r.loader.RenderState(ctx, bp, statePath, params)
}

// GetRollbackEntrypoint returns the rollback entrypoint path for a blueprint.
func (r *RollbackExecutor) GetRollbackEntrypoint(blueprintName string) (string, error) {
	blueprintPath := filepath.Join(r.config.BlueprintPath, blueprintName)
	manifestPath := filepath.Join(blueprintPath, "blueprint.yaml")

	bp, err := ParseManifestFile(manifestPath)
	if err != nil {
		return "", fmt.Errorf("failed to load blueprint: %w", err)
	}

	entrypoint, ok := bp.Entrypoints["rollback"]
	if !ok {
		return "", nil
	}

	return entrypoint, nil
}

// HasRollbackEntrypoint checks if a blueprint has a rollback entrypoint.
func (r *RollbackExecutor) HasRollbackEntrypoint(blueprintName string) bool {
	entrypoint, err := r.GetRollbackEntrypoint(blueprintName)
	return err == nil && entrypoint != ""
}

// ValidateRollbackEntrypoint validates that a rollback entrypoint exists and is valid.
func (r *RollbackExecutor) ValidateRollbackEntrypoint(blueprintName string) error {
	blueprintPath := filepath.Join(r.config.BlueprintPath, blueprintName)
	manifestPath := filepath.Join(blueprintPath, "blueprint.yaml")

	bp, err := ParseManifestFile(manifestPath)
	if err != nil {
		return fmt.Errorf("failed to load blueprint: %w", err)
	}

	entrypoint, ok := bp.Entrypoints["rollback"]
	if !ok {
		return fmt.Errorf("blueprint does not define a rollback entrypoint")
	}

	// Check if the state file exists
	statePath := filepath.Join(bp.SourcePath, entrypoint)
	if _, err := os.Stat(statePath); os.IsNotExist(err) {
		return fmt.Errorf("rollback entrypoint file does not exist: %s", entrypoint)
	}

	return nil
}

// ExecuteStateRestore executes state restoration from a snapshot.
// This provides more fine-grained control than full rollback.
func (r *RollbackExecutor) ExecuteStateRestore(ctx context.Context, snapshot *Snapshot) (*RollbackResult, error) {
	startTime := time.Now()

	result := &RollbackResult{
		Blueprint:   snapshot.BlueprintName,
		ToVersion:   snapshot.BlueprintVersion,
		FromVersion: snapshot.PreviousVersion,
	}

	if snapshot.StateCapture == nil {
		result.Success = true
		result.Duration = time.Since(startTime)
		return result, nil
	}

	capture := snapshot.StateCapture

	// Generate restore states from the snapshot
	states := make(map[string][]StateDeclaration)

	// Generate file states
	for i := range capture.Files {
		file := &capture.Files[i]
		decl := StateDeclaration{
			ID:     fmt.Sprintf("restore_%s", sanitizeID(file.Path)),
			Module: "file",
			Parameters: map[string]interface{}{
				"path": file.Path,
			},
		}

		if file.Exists {
			decl.State = "present"
			if file.IsDir {
				decl.State = "directory"
			} else if file.IsSymlink {
				decl.State = "symlink"
				decl.Parameters["target"] = file.LinkTarget
			}
			if file.Mode != 0 {
				decl.Parameters["mode"] = fmt.Sprintf("%04o", file.Mode)
			}
			if file.Owner != "" {
				decl.Parameters["owner"] = file.Owner
			}
			if file.Group != "" {
				decl.Parameters["group"] = file.Group
			}
		} else {
			decl.State = "absent"
		}

		states["file"] = append(states["file"], decl)
		result.StatesApplied++
	}

	// Generate package states
	for _, pkg := range capture.Packages {
		decl := StateDeclaration{
			ID:     fmt.Sprintf("restore_pkg_%s", sanitizeID(pkg.Name)),
			Module: "package",
			Parameters: map[string]interface{}{
				"name": pkg.Name,
			},
		}

		if pkg.Installed {
			decl.State = "installed"
			if pkg.Version != "" {
				decl.Parameters["version"] = pkg.Version
			}
		} else {
			decl.State = "removed"
		}

		states["package"] = append(states["package"], decl)
		result.StatesApplied++
	}

	// Generate service states
	for _, svc := range capture.Services {
		decl := StateDeclaration{
			ID:     fmt.Sprintf("restore_svc_%s", sanitizeID(svc.Name)),
			Module: "service",
			Parameters: map[string]interface{}{
				"name":    svc.Name,
				"enabled": svc.Enabled,
			},
		}

		if svc.Running {
			decl.State = "running"
		} else {
			decl.State = "stopped"
		}

		states["service"] = append(states["service"], decl)
		result.StatesApplied++
	}

	// Generate user states
	for _, user := range capture.Users {
		decl := StateDeclaration{
			ID:     fmt.Sprintf("restore_user_%s", sanitizeID(user.Name)),
			Module: "user",
			Parameters: map[string]interface{}{
				"name": user.Name,
			},
		}

		if user.Exists {
			decl.State = "present"
			if user.UID != 0 {
				decl.Parameters["uid"] = user.UID
			}
			if user.GID != 0 {
				decl.Parameters["gid"] = user.GID
			}
			if user.Home != "" {
				decl.Parameters["home"] = user.Home
			}
			if user.Shell != "" {
				decl.Parameters["shell"] = user.Shell
			}
			if len(user.Groups) > 0 {
				decl.Parameters["groups"] = user.Groups
			}
		} else {
			decl.State = "absent"
		}

		states["user"] = append(states["user"], decl)
		result.StatesApplied++
	}

	// Generate group states
	for _, group := range capture.Groups {
		decl := StateDeclaration{
			ID:     fmt.Sprintf("restore_group_%s", sanitizeID(group.Name)),
			Module: "group",
			Parameters: map[string]interface{}{
				"name": group.Name,
			},
		}

		if group.Exists {
			decl.State = "present"
			if group.GID != 0 {
				decl.Parameters["gid"] = group.GID
			}
		} else {
			decl.State = "absent"
		}

		states["group"] = append(states["group"], decl)
		result.StatesApplied++
	}

	result.ExpandedStates = states
	result.Success = true
	result.Duration = time.Since(startTime)

	return result, nil
}

// sanitizeID converts a path or name to a valid state ID.
func sanitizeID(s string) string {
	// Replace path separators and special characters with underscores
	result := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' {
			result = append(result, c)
		} else {
			result = append(result, '_')
		}
	}
	return string(result)
}
