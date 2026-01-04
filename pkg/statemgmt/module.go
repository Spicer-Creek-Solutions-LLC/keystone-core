package statemgmt

import (
	"context"
	"fmt"
)

// Module is the interface that all state modules must implement
type Module interface {
	// Name returns the module name (e.g., "file", "package", "service")
	Name() string

	// ValidStates returns the list of valid state values for this module
	ValidStates() []string

	// Check checks the current state without making changes
	// Returns current state info and whether it matches desired state
	Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error)

	// Apply applies the state declaration, making changes if needed
	// Should be idempotent - safe to run multiple times
	Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error)

	// Test tests if the state was applied successfully
	// Returns true if the state is in the desired configuration
	Test(ctx context.Context, decl *StateDeclaration) (bool, error)
}

// ModuleCheckResult contains the result of checking current state
type ModuleCheckResult struct {
	// Present indicates if the resource exists
	Present bool

	// CurrentState is the current state (e.g., "running", "stopped")
	CurrentState string

	// Matches indicates if current state matches desired state
	Matches bool

	// Diff contains details of what would change
	Diff map[string]interface{}

	// Metadata contains additional information about the resource
	Metadata map[string]interface{}
}

// ModuleRegistry manages available state modules
type ModuleRegistry struct {
	modules map[string]Module
}

// NewModuleRegistry creates a new module registry
func NewModuleRegistry() *ModuleRegistry {
	return &ModuleRegistry{
		modules: make(map[string]Module),
	}
}

// Register registers a new module
func (r *ModuleRegistry) Register(module Module) error {
	name := module.Name()
	if name == "" {
		return fmt.Errorf("module name cannot be empty")
	}

	if _, exists := r.modules[name]; exists {
		return fmt.Errorf("module %s is already registered", name)
	}

	r.modules[name] = module
	return nil
}

// Get retrieves a module by name
func (r *ModuleRegistry) Get(name string) (Module, error) {
	module, exists := r.modules[name]
	if !exists {
		return nil, fmt.Errorf("module %s not found", name)
	}
	return module, nil
}

// List returns all registered module names
func (r *ModuleRegistry) List() []string {
	names := make([]string, 0, len(r.modules))
	for name := range r.modules {
		names = append(names, name)
	}
	return names
}

// DefaultRegistry is the default global module registry
var DefaultRegistry = NewModuleRegistry()

// RegisterModule registers a module in the default registry
func RegisterModule(module Module) error {
	return DefaultRegistry.Register(module)
}

// GetModule retrieves a module from the default registry
func GetModule(name string) (Module, error) {
	return DefaultRegistry.Get(name)
}

// BaseModule provides common functionality for all modules
type BaseModule struct {
	name        string
	validStates []string
}

// Name returns the module name
func (m *BaseModule) Name() string {
	return m.name
}

// ValidStates returns valid state values
func (m *BaseModule) ValidStates() []string {
	return m.validStates
}

// NewBaseModule creates a new base module
func NewBaseModule(name string, validStates []string) *BaseModule {
	return &BaseModule{
		name:        name,
		validStates: validStates,
	}
}

// hasParameter checks if a parameter exists in the declaration
func hasParameter(decl *StateDeclaration, key string) bool {
	if decl.Parameters == nil {
		return false
	}
	_, ok := decl.Parameters[key]
	return ok
}

// getParameter gets a parameter from the declaration with type assertion
func getParameter(decl *StateDeclaration, key string, defaultValue interface{}) interface{} {
	if val, ok := decl.Parameters[key]; ok {
		return val
	}
	return defaultValue
}

// getStringParameter gets a string parameter
func getStringParameter(decl *StateDeclaration, key string, defaultValue string) string {
	if val, ok := decl.Parameters[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return defaultValue
}

// getBoolParameter gets a boolean parameter
func getBoolParameter(decl *StateDeclaration, key string, defaultValue bool) bool {
	if val, ok := decl.Parameters[key]; ok {
		if b, ok := val.(bool); ok {
			return b
		}
	}
	return defaultValue
}

// getIntParameter gets an integer parameter
func getIntParameter(decl *StateDeclaration, key string, defaultValue int) int {
	if val, ok := decl.Parameters[key]; ok {
		// Handle both int and float64 (from JSON/YAML parsing)
		switch v := val.(type) {
		case int:
			return v
		case int64:
			return int(v)
		case float64:
			return int(v)
		}
	}
	return defaultValue
}

// getSliceParameter gets a slice parameter
func getSliceParameter(decl *StateDeclaration, key string) []interface{} {
	if val, ok := decl.Parameters[key]; ok {
		if slice, ok := val.([]interface{}); ok {
			return slice
		}
	}
	return nil
}

// getStringSliceParameter gets a string slice parameter
func getStringSliceParameter(decl *StateDeclaration, key string) []string {
	if decl.Parameters == nil {
		return nil
	}
	val, ok := decl.Parameters[key]
	if !ok {
		return nil
	}

	// Handle direct []string type
	if strSlice, ok := val.([]string); ok {
		return strSlice
	}

	// Handle []interface{} type (from JSON/YAML parsing)
	if slice, ok := val.([]interface{}); ok {
		result := make([]string, 0, len(slice))
		for _, item := range slice {
			if str, ok := item.(string); ok {
				result = append(result, str)
			}
		}
		return result
	}

	return nil
}
