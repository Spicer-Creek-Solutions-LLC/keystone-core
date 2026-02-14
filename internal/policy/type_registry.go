package policy

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// TypeHandler defines the interface for custom policy type handlers
type TypeHandler interface {
	// Type returns the policy type this handler supports
	Type() Type

	// Name returns a human-readable name for this policy type
	Name() string

	// Description returns a description of this policy type
	Description() string

	// Evaluate evaluates a policy of this type against input
	Evaluate(ctx context.Context, policy *Policy, input interface{}) (*EvaluationResult, error)

	// Validate validates a policy of this type
	Validate(policy *Policy) error

	// SupportedFeatures returns the features this handler supports
	SupportedFeatures() []Feature
}

// Feature represents a feature supported by a policy type
type Feature string

// FeatureDryRun and related constants.
const (
	FeatureDryRun        Feature = "dry_run"
	FeatureExplainMode   Feature = "explain_mode"
	FeatureTemplating    Feature = "templating"
	FeatureInheritance   Feature = "inheritance"
	FeatureCustomActions Feature = "custom_actions"
	FeatureMetrics       Feature = "metrics"
	FeatureDebug         Feature = "debug"
)

// TypeRegistry manages custom policy type registrations
type TypeRegistry struct {
	handlers map[PolicyType]TypeHandler
	metadata map[PolicyType]*TypeMetadata
	mu       sync.RWMutex

	// Callbacks
	onRegister   []func(PolicyType, TypeHandler)
	onUnregister []func(PolicyType)
}

// TypeMetadata contains metadata about a registered policy type
type TypeMetadata struct {
	Type            PolicyType
	Name            string
	Description     string
	Version         string
	Author          string
	Features        []Feature
	RegisteredAt    int64
	EvaluationCount int64
	ErrorCount      int64
}

// NewTypeRegistry creates a new type registry
func NewTypeRegistry() *TypeRegistry {
	tr := &TypeRegistry{
		handlers: make(map[PolicyType]TypeHandler),
		metadata: make(map[PolicyType]*TypeMetadata),
	}

	// Register built-in types
	tr.registerBuiltins()

	return tr
}

// registerBuiltins registers the built-in policy type handlers
func (tr *TypeRegistry) registerBuiltins() {
	// OPA handler
	_ = tr.Register(&OPAHandler{}) //nolint:errcheck // builtin handlers are valid

	// CEL handler
	_ = tr.Register(&CELHandler{}) //nolint:errcheck // builtin handlers are valid

	// Builtin handler
	_ = tr.Register(&BuiltinHandler{}) //nolint:errcheck // builtin handlers are valid
}

// Register registers a custom policy type handler
func (tr *TypeRegistry) Register(handler TypeHandler) error {
	if handler == nil {
		return fmt.Errorf("handler cannot be nil")
	}

	policyType := handler.Type()
	if policyType == "" {
		return fmt.Errorf("policy type cannot be empty")
	}

	tr.mu.Lock()
	defer tr.mu.Unlock()

	// Check if already registered
	if existing, ok := tr.handlers[policyType]; ok {
		return fmt.Errorf("policy type %s already registered by %s", policyType, existing.Name())
	}

	tr.handlers[policyType] = handler
	tr.metadata[policyType] = &TypeMetadata{
		Type:         policyType,
		Name:         handler.Name(),
		Description:  handler.Description(),
		Features:     handler.SupportedFeatures(),
		RegisteredAt: timeNowFunc(),
	}

	// Notify callbacks
	for _, cb := range tr.onRegister {
		cb(policyType, handler)
	}

	return nil
}

// timeNowFunc is a variable for testing - returns UnixNano timestamp
var timeNowFunc = func() int64 {
	return time.Now().UnixNano()
}

// Unregister removes a custom policy type handler
func (tr *TypeRegistry) Unregister(policyType PolicyType) error {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	if _, ok := tr.handlers[policyType]; !ok {
		return fmt.Errorf("policy type %s not registered", policyType)
	}

	delete(tr.handlers, policyType)
	delete(tr.metadata, policyType)

	// Notify callbacks
	for _, cb := range tr.onUnregister {
		cb(policyType)
	}

	return nil
}

// Get returns the handler for a policy type
func (tr *TypeRegistry) Get(policyType PolicyType) (TypeHandler, bool) {
	tr.mu.RLock()
	defer tr.mu.RUnlock()
	handler, ok := tr.handlers[policyType]
	return handler, ok
}

// List returns all registered policy types
func (tr *TypeRegistry) List() []PolicyType {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	types := make([]PolicyType, 0, len(tr.handlers))
	for t := range tr.handlers {
		types = append(types, t)
	}
	return types
}

// GetMetadata returns metadata for a policy type
func (tr *TypeRegistry) GetMetadata(policyType PolicyType) (*TypeMetadata, bool) {
	tr.mu.RLock()
	defer tr.mu.RUnlock()
	metadata, ok := tr.metadata[policyType]
	return metadata, ok
}

// ListMetadata returns metadata for all registered types
func (tr *TypeRegistry) ListMetadata() []*TypeMetadata {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	result := make([]*TypeMetadata, 0, len(tr.metadata))
	for _, m := range tr.metadata {
		result = append(result, m)
	}
	return result
}

// HasFeature checks if a policy type supports a feature
func (tr *TypeRegistry) HasFeature(policyType Type, feature Feature) bool {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	metadata, ok := tr.metadata[policyType]
	if !ok {
		return false
	}

	for _, f := range metadata.Features {
		if f == feature {
			return true
		}
	}
	return false
}

// Evaluate evaluates a policy using the appropriate handler
func (tr *TypeRegistry) Evaluate(ctx context.Context, policy *Policy, input interface{}) (*EvaluationResult, error) {
	handler, ok := tr.Get(policy.Type)
	if !ok {
		return nil, fmt.Errorf("no handler for policy type: %s", policy.Type)
	}

	result, err := handler.Evaluate(ctx, policy, input)

	// Update metrics
	tr.mu.Lock()
	if metadata, ok := tr.metadata[policy.Type]; ok {
		metadata.EvaluationCount++
		if err != nil {
			metadata.ErrorCount++
		}
	}
	tr.mu.Unlock()

	return result, err
}

// Validate validates a policy using the appropriate handler
func (tr *TypeRegistry) Validate(policy *Policy) error {
	handler, ok := tr.Get(policy.Type)
	if !ok {
		return fmt.Errorf("no handler for policy type: %s", policy.Type)
	}
	return handler.Validate(policy)
}

// OnRegister adds a callback for when a type is registered
func (tr *TypeRegistry) OnRegister(callback func(PolicyType, TypeHandler)) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	tr.onRegister = append(tr.onRegister, callback)
}

// OnUnregister adds a callback for when a type is unregistered
func (tr *TypeRegistry) OnUnregister(callback func(PolicyType)) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	tr.onUnregister = append(tr.onUnregister, callback)
}

// OPAHandler handles OPA/Rego policy evaluation
type OPAHandler struct{}

// Type returns the type.
func (h *OPAHandler) Type() Type { return TypeOPA }
// Name returns the name.
func (h *OPAHandler) Name() string     { return "Open Policy Agent (OPA)" }
// Description returns a description of the handler.
func (h *OPAHandler) Description() string {
	return "Evaluate policies using OPA/Rego policy language"
}

// SupportedFeatures returns the features supported by the handler.
func (h *OPAHandler) SupportedFeatures() []Feature {
	return []Feature{FeatureDryRun, FeatureExplainMode, FeatureDebug}
}

// Evaluate is not supported on TypeHandler directly — use Engine.Evaluate() instead.
// The OPA evaluation logic lives in the Engine's OPAEvaluator.
func (h *OPAHandler) Evaluate(_ context.Context, _ *Policy, _ interface{}) (*EvaluationResult, error) {
	return nil, fmt.Errorf("OPA evaluation must go through Engine.Evaluate(), not TypeHandler directly")
}

// Validate validates the policy definition.
func (h *OPAHandler) Validate(policy *Policy) error {
	if policy.Policy == "" {
		return fmt.Errorf("OPA policy code is required")
	}
	// Note: Simple package declaration check - actual OPA validation is more complex
	// and happens during evaluation. We don't fail here for flexibility.
	return nil
}

// CELHandler handles CEL policy evaluation
type CELHandler struct{}

// Type returns the type.
func (h *CELHandler) Type() Type { return TypeCEL }
// Name returns the name.
func (h *CELHandler) Name() string     { return "Common Expression Language (CEL)" }
// Description returns a description of the handler.
func (h *CELHandler) Description() string {
	return "Evaluate policies using Google's Common Expression Language"
}

// SupportedFeatures returns the features supported by the handler.
func (h *CELHandler) SupportedFeatures() []Feature {
	return []Feature{FeatureDryRun, FeatureTemplating}
}

// Evaluate is not supported on TypeHandler directly — use Engine.Evaluate() instead.
// The CEL evaluation logic lives in the Engine's CELEvaluator.
func (h *CELHandler) Evaluate(_ context.Context, _ *Policy, _ interface{}) (*EvaluationResult, error) {
	return nil, fmt.Errorf("CEL evaluation must go through Engine.Evaluate(), not TypeHandler directly")
}

// Validate validates the policy definition.
func (h *CELHandler) Validate(policy *Policy) error {
	if policy.Policy == "" {
		return fmt.Errorf("CEL expression is required")
	}
	return nil
}

// BuiltinHandler handles built-in policy types
type BuiltinHandler struct{}

// Type returns the type.
func (h *BuiltinHandler) Type() Type { return TypeBuiltin }
// Name returns the name.
func (h *BuiltinHandler) Name() string     { return "Built-in Policies" }
// Description returns a description of the handler.
func (h *BuiltinHandler) Description() string {
	return "Pre-defined policy types for common use cases"
}

// SupportedFeatures returns the features supported by the handler.
func (h *BuiltinHandler) SupportedFeatures() []Feature {
	return []Feature{FeatureDryRun, FeatureMetrics}
}

// Evaluate is not supported on TypeHandler directly — use Engine.Evaluate() instead.
// The builtin evaluation logic lives in the Engine's BuiltinEvaluator.
func (h *BuiltinHandler) Evaluate(_ context.Context, _ *Policy, _ interface{}) (*EvaluationResult, error) {
	return nil, fmt.Errorf("builtin evaluation must go through Engine.Evaluate(), not TypeHandler directly")
}

// Validate validates the policy definition.
func (h *BuiltinHandler) Validate(policy *Policy) error {
	if policy.Policy == "" {
		return fmt.Errorf("builtin policy name is required")
	}
	return nil
}

// CustomHandler is a base type for implementing custom policy handlers
type CustomHandler struct {
	typeValue        PolicyType
	nameValue        string
	descriptionValue string
	features         []Feature
	evaluateFunc     func(ctx context.Context, policy *Policy, input interface{}) (*EvaluationResult, error)
	validateFunc     func(policy *Policy) error
}

// NewCustomHandler creates a new custom handler
func NewCustomHandler(opts CustomHandlerOptions) *CustomHandler {
	return &CustomHandler{
		typeValue:        opts.Type,
		nameValue:        opts.Name,
		descriptionValue: opts.Description,
		features:         opts.Features,
		evaluateFunc:     opts.EvaluateFunc,
		validateFunc:     opts.ValidateFunc,
	}
}

// CustomHandlerOptions configures a custom handler
type CustomHandlerOptions struct {
	Type         PolicyType
	Name         string
	Description  string
	Features     []Feature
	EvaluateFunc func(ctx context.Context, policy *Policy, input interface{}) (*EvaluationResult, error)
	ValidateFunc func(policy *Policy) error
}

// Type returns the type.
func (h *CustomHandler) Type() Type                         { return h.typeValue }
// Name returns the name.
func (h *CustomHandler) Name() string                       { return h.nameValue }
// Description returns a description of the handler.
func (h *CustomHandler) Description() string                { return h.descriptionValue }
// SupportedFeatures returns the features supported by the handler.
func (h *CustomHandler) SupportedFeatures() []Feature { return h.features }

// Evaluate evaluates the policy against the input.
func (h *CustomHandler) Evaluate(ctx context.Context, policy *Policy, input interface{}) (*EvaluationResult, error) {
	if h.evaluateFunc == nil {
		return nil, fmt.Errorf("evaluate function not implemented")
	}
	return h.evaluateFunc(ctx, policy, input)
}

// Validate validates the policy definition.
func (h *CustomHandler) Validate(policy *Policy) error {
	if h.validateFunc == nil {
		return nil // No validation by default
	}
	return h.validateFunc(policy)
}

// PluginLoader loads policy type plugins
type PluginLoader struct {
	registry *TypeRegistry
	mu       sync.Mutex
	loaded   map[string]bool
}

// NewPluginLoader creates a new plugin loader
func NewPluginLoader(registry *TypeRegistry) *PluginLoader {
	return &PluginLoader{
		registry: registry,
		loaded:   make(map[string]bool),
	}
}

// LoadPlugin loads a plugin handler from a configuration
func (pl *PluginLoader) LoadPlugin(config *PluginConfig) error {
	pl.mu.Lock()
	defer pl.mu.Unlock()

	if pl.loaded[config.Name] {
		return fmt.Errorf("plugin %s already loaded", config.Name)
	}

	handler := NewCustomHandler(CustomHandlerOptions{
		Type:         PolicyType(config.Type),
		Name:         config.Name,
		Description:  config.Description,
		Features:     config.Features,
		EvaluateFunc: config.EvaluateFunc,
		ValidateFunc: config.ValidateFunc,
	})

	if err := pl.registry.Register(handler); err != nil {
		return fmt.Errorf("failed to register plugin %s: %w", config.Name, err)
	}

	pl.loaded[config.Name] = true
	return nil
}

// UnloadPlugin unloads a plugin
func (pl *PluginLoader) UnloadPlugin(name string) error {
	pl.mu.Lock()
	defer pl.mu.Unlock()

	if !pl.loaded[name] {
		return fmt.Errorf("plugin %s not loaded", name)
	}

	// Find the type for this plugin
	for t, m := range pl.registry.metadata {
		if m.Name == name {
			if err := pl.registry.Unregister(t); err != nil {
				return err
			}
			break
		}
	}

	delete(pl.loaded, name)
	return nil
}

// IsLoaded checks if a plugin is loaded
func (pl *PluginLoader) IsLoaded(name string) bool {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	return pl.loaded[name]
}

// ListLoaded returns all loaded plugins
func (pl *PluginLoader) ListLoaded() []string {
	pl.mu.Lock()
	defer pl.mu.Unlock()

	names := make([]string, 0, len(pl.loaded))
	for name := range pl.loaded {
		names = append(names, name)
	}
	return names
}

// PluginConfig configures a policy type plugin
type PluginConfig struct {
	Name         string
	Type         string
	Description  string
	Features     []Feature
	EvaluateFunc func(ctx context.Context, policy *Policy, input interface{}) (*EvaluationResult, error)
	ValidateFunc func(policy *Policy) error
}

// DefaultTypeRegistry is the global type registry
var DefaultTypeRegistry = NewTypeRegistry()

// Deprecated aliases for backward compatibility
//
//nolint:revive,staticcheck // stuttering names kept for backward compatibility
type (
	// PolicyTypeHandler is deprecated: Use TypeHandler instead.
	PolicyTypeHandler = TypeHandler
	// PolicyFeature is deprecated: Use Feature instead.
	PolicyFeature = Feature
)
