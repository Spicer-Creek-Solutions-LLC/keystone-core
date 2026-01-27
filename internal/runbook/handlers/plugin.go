package handlers

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/shawnbutts/keystone-core/internal/runbook"
)

// PluginHandler represents an external step handler plugin.
// Plugins are executable files that implement the runbook step handler protocol.
type PluginHandler struct {
	name       string
	path       string
	stepType   runbook.StepType
	configSpec map[string]PluginConfigSpec
}

// PluginConfigSpec describes a configuration field for a plugin.
type PluginConfigSpec struct {
	Name        string `json:"name"`
	Type        string `json:"type"` // string, int, bool, list, map
	Required    bool   `json:"required"`
	Description string `json:"description,omitempty"`
	Default     any    `json:"default,omitempty"`
}

// NewPluginHandler creates a plugin handler from a plugin path.
func NewPluginHandler(name string, path string, stepType runbook.StepType) *PluginHandler {
	return &PluginHandler{
		name:     name,
		path:     path,
		stepType: stepType,
	}
}

// Type returns the step type this plugin handles.
func (h *PluginHandler) Type() runbook.StepType {
	return h.stepType
}

// Validate validates the step configuration.
func (h *PluginHandler) Validate(step *runbook.Step) error {
	// Check required fields from config spec
	for _, spec := range h.configSpec {
		if spec.Required {
			if _, ok := step.Config[spec.Name]; !ok {
				return fmt.Errorf("plugin %s requires '%s' configuration", h.name, spec.Name)
			}
		}
	}
	return nil
}

// Execute executes the plugin with the step configuration.
func (h *PluginHandler) Execute(ctx context.Context, step *runbook.Step, varCtx VariableContext) (*runbook.StepResult, error) {
	// Build command with step config as JSON environment variable
	cmd := exec.CommandContext(ctx, h.path, "execute")

	// Pass step configuration through environment
	cmd.Env = os.Environ()
	for k, v := range step.Config {
		// Resolve template variables
		if vStr, ok := v.(string); ok {
			resolved, err := varCtx.Resolve(vStr)
			if err == nil {
				v = resolved
			}
		}
		cmd.Env = append(cmd.Env, fmt.Sprintf("RUNBOOK_CONFIG_%s=%v", strings.ToUpper(k), v))
	}

	// Add execution context
	cmd.Env = append(cmd.Env, fmt.Sprintf("RUNBOOK_EXECUTION_ID=%s", varCtx.ExecutionID()))
	cmd.Env = append(cmd.Env, fmt.Sprintf("RUNBOOK_NAME=%s", varCtx.RunbookName()))
	cmd.Env = append(cmd.Env, fmt.Sprintf("RUNBOOK_STEP_NAME=%s", step.Name))
	cmd.Env = append(cmd.Env, fmt.Sprintf("RUNBOOK_STEP_TYPE=%s", step.Type))

	// Capture output
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Execute
	err := cmd.Run()

	// Parse output (expects JSON format)
	// Format: {"success": bool, "message": string, "outputs": {...}}
	outputs := map[string]interface{}{
		"stdout": stdout.String(),
		"stderr": stderr.String(),
	}

	if err != nil {
		return &runbook.StepResult{
			Success: false,
			Message: fmt.Sprintf("plugin execution failed: %v", err),
			Outputs: outputs,
		}, err
	}

	return &runbook.StepResult{
		Success: true,
		Message: fmt.Sprintf("plugin %s executed successfully", h.name),
		Outputs: outputs,
	}, nil
}

// SetConfigSpec sets the configuration specification for the plugin.
func (h *PluginHandler) SetConfigSpec(spec map[string]PluginConfigSpec) {
	h.configSpec = spec
}

// PluginRegistry manages discovery and registration of plugin handlers.
type PluginRegistry struct {
	mu       sync.RWMutex
	plugins  map[runbook.StepType]*PluginHandler
	paths    []string
	registry *Registry
}

// NewPluginRegistry creates a new plugin registry.
func NewPluginRegistry(registry *Registry) *PluginRegistry {
	return &PluginRegistry{
		plugins:  make(map[runbook.StepType]*PluginHandler),
		paths:    getDefaultPluginPaths(),
		registry: registry,
	}
}

// getDefaultPluginPaths returns the default plugin search paths.
func getDefaultPluginPaths() []string {
	paths := []string{
		"/usr/local/lib/keystone/runbook-plugins",
		"/usr/lib/keystone/runbook-plugins",
	}

	// Add user plugin directory
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".keystone", "runbook-plugins"))
	}

	// Add from environment
	if envPath := os.Getenv("KEYSTONE_RUNBOOK_PLUGIN_PATH"); envPath != "" {
		paths = append(paths, strings.Split(envPath, string(os.PathListSeparator))...)
	}

	return paths
}

// AddPluginPath adds a path to the plugin search paths.
func (r *PluginRegistry) AddPluginPath(path string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.paths = append(r.paths, path)
}

// DiscoverPlugins discovers plugins in the configured paths.
func (r *PluginRegistry) DiscoverPlugins() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, basePath := range r.paths {
		if _, err := os.Stat(basePath); os.IsNotExist(err) {
			continue
		}

		entries, err := os.ReadDir(basePath)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}

			// Check if it's executable
			path := filepath.Join(basePath, entry.Name())
			info, err := entry.Info()
			if err != nil {
				continue
			}

			// Check executable permission on Unix
			if info.Mode()&0111 == 0 {
				continue
			}

			// Extract step type from filename (e.g., kscore-runbook-custom -> custom)
			name := entry.Name()
			if strings.HasPrefix(name, "kscore-runbook-") {
				stepType := strings.TrimPrefix(name, "kscore-runbook-")
				// Remove extension if present
				stepType = strings.TrimSuffix(stepType, filepath.Ext(stepType))

				plugin := NewPluginHandler(name, path, runbook.StepType(stepType))
				r.plugins[plugin.Type()] = plugin

				// Register with main registry
				if r.registry != nil {
					_ = r.registry.Register(plugin)
				}
			}
		}
	}

	return nil
}

// GetPlugin returns a plugin by step type.
func (r *PluginRegistry) GetPlugin(stepType runbook.StepType) (*PluginHandler, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	plugin, ok := r.plugins[stepType]
	return plugin, ok
}

// RegisterPlugin manually registers a plugin.
func (r *PluginRegistry) RegisterPlugin(plugin *PluginHandler) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.plugins[plugin.Type()]; exists {
		return fmt.Errorf("plugin for step type %q already registered", plugin.Type())
	}

	r.plugins[plugin.Type()] = plugin

	// Register with main registry
	if r.registry != nil {
		return r.registry.Register(plugin)
	}

	return nil
}

// ListPlugins returns all registered plugins.
func (r *PluginRegistry) ListPlugins() []*PluginHandler {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*PluginHandler, 0, len(r.plugins))
	for _, plugin := range r.plugins {
		result = append(result, plugin)
	}
	return result
}
