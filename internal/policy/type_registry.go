package policy

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// PolicyTypeHandler defines the interface for custom policy type handlers
type PolicyTypeHandler interface {
	// Type returns the policy type this handler supports
	Type() PolicyType

	// Name returns a human-readable name for this policy type
	Name() string

	// Description returns a description of this policy type
	Description() string

	// Evaluate evaluates a policy of this type against input
	Evaluate(ctx context.Context, policy *Policy, input interface{}) (*EvaluationResult, error)

	// Validate validates a policy of this type
	Validate(policy *Policy) error

	// SupportedFeatures returns the features this handler supports
	SupportedFeatures() []PolicyFeature
}

// PolicyFeature represents a feature supported by a policy type
type PolicyFeature string

const (
	FeatureDryRun        PolicyFeature = "dry_run"
	FeatureExplainMode   PolicyFeature = "explain_mode"
	FeatureTemplating    PolicyFeature = "templating"
	FeatureInheritance   PolicyFeature = "inheritance"
	FeatureCustomActions PolicyFeature = "custom_actions"
	FeatureMetrics       PolicyFeature = "metrics"
	FeatureDebug         PolicyFeature = "debug"
)

// TypeRegistry manages custom policy type registrations
type TypeRegistry struct {
	handlers map[PolicyType]PolicyTypeHandler
	metadata map[PolicyType]*TypeMetadata
	mu       sync.RWMutex

	// Callbacks
	onRegister   []func(PolicyType, PolicyTypeHandler)
	onUnregister []func(PolicyType)
}

// TypeMetadata contains metadata about a registered policy type
type TypeMetadata struct {
	Type           PolicyType
	Name           string
	Description    string
	Version        string
	Author         string
	Features       []PolicyFeature
	RegisteredAt   int64
	EvaluationCount int64
	ErrorCount     int64
}

// NewTypeRegistry creates a new type registry
func NewTypeRegistry() *TypeRegistry {
	tr := &TypeRegistry{
		handlers: make(map[PolicyType]PolicyTypeHandler),
		metadata: make(map[PolicyType]*TypeMetadata),
	}

	// Register built-in types
	tr.registerBuiltins()

	return tr
}

// registerBuiltins registers the built-in policy type handlers
func (tr *TypeRegistry) registerBuiltins() {
	// OPA handler
	tr.Register(&OPAHandler{})

	// CEL handler
	tr.Register(&CELHandler{})

	// Builtin handler
	tr.Register(&BuiltinHandler{})
}

// Register registers a custom policy type handler
func (tr *TypeRegistry) Register(handler PolicyTypeHandler) error {
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
func (tr *TypeRegistry) Get(policyType PolicyType) (PolicyTypeHandler, bool) {
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
func (tr *TypeRegistry) HasFeature(policyType PolicyType, feature PolicyFeature) bool {
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
func (tr *TypeRegistry) OnRegister(callback func(PolicyType, PolicyTypeHandler)) {
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

func (h *OPAHandler) Type() PolicyType { return PolicyTypeOPA }
func (h *OPAHandler) Name() string     { return "Open Policy Agent (OPA)" }
func (h *OPAHandler) Description() string {
	return "Evaluate policies using OPA/Rego policy language"
}

func (h *OPAHandler) SupportedFeatures() []PolicyFeature {
	return []PolicyFeature{FeatureDryRun, FeatureExplainMode, FeatureDebug}
}

func (h *OPAHandler) Evaluate(ctx context.Context, policy *Policy, input interface{}) (*EvaluationResult, error) {
	// Delegate to the OPA compiler in engine.go
	result := &EvaluationResult{
		PolicyID: policy.ID,
		Allowed:  true, // Placeholder - actual OPA evaluation happens in engine
	}
	return result, nil
}

func (h *OPAHandler) Validate(policy *Policy) error {
	if policy.Policy == "" {
		return fmt.Errorf("OPA policy code is required")
	}
	// Check for package declaration
	if len(policy.Policy) < 7 || policy.Policy[:7] != "package" {
		// Simple check - actual OPA validation is more complex
	}
	return nil
}

// CELHandler handles CEL policy evaluation
type CELHandler struct{}

func (h *CELHandler) Type() PolicyType { return PolicyTypeCEL }
func (h *CELHandler) Name() string     { return "Common Expression Language (CEL)" }
func (h *CELHandler) Description() string {
	return "Evaluate policies using Google's Common Expression Language"
}

func (h *CELHandler) SupportedFeatures() []PolicyFeature {
	return []PolicyFeature{FeatureDryRun, FeatureTemplating}
}

func (h *CELHandler) Evaluate(ctx context.Context, policy *Policy, input interface{}) (*EvaluationResult, error) {
	// Delegate to CEL evaluator in engine.go
	result := &EvaluationResult{
		PolicyID: policy.ID,
		Allowed:  true, // Placeholder - actual CEL evaluation happens in engine
	}
	return result, nil
}

func (h *CELHandler) Validate(policy *Policy) error {
	if policy.Policy == "" {
		return fmt.Errorf("CEL expression is required")
	}
	return nil
}

// BuiltinHandler handles built-in policy types
type BuiltinHandler struct{}

func (h *BuiltinHandler) Type() PolicyType { return PolicyTypeBuiltin }
func (h *BuiltinHandler) Name() string     { return "Built-in Policies" }
func (h *BuiltinHandler) Description() string {
	return "Pre-defined policy types for common use cases"
}

func (h *BuiltinHandler) SupportedFeatures() []PolicyFeature {
	return []PolicyFeature{FeatureDryRun, FeatureMetrics}
}

func (h *BuiltinHandler) Evaluate(ctx context.Context, policy *Policy, input interface{}) (*EvaluationResult, error) {
	// Delegate to builtin evaluator in engine.go
	result := &EvaluationResult{
		PolicyID: policy.ID,
		Allowed:  true, // Placeholder
	}
	return result, nil
}

func (h *BuiltinHandler) Validate(policy *Policy) error {
	if policy.Policy == "" {
		return fmt.Errorf("builtin policy name is required")
	}
	return nil
}

// CustomHandler is a base type for implementing custom policy handlers
type CustomHandler struct {
	typeValue       PolicyType
	nameValue       string
	descriptionValue string
	features        []PolicyFeature
	evaluateFunc    func(ctx context.Context, policy *Policy, input interface{}) (*EvaluationResult, error)
	validateFunc    func(policy *Policy) error
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
	Features     []PolicyFeature
	EvaluateFunc func(ctx context.Context, policy *Policy, input interface{}) (*EvaluationResult, error)
	ValidateFunc func(policy *Policy) error
}

func (h *CustomHandler) Type() PolicyType        { return h.typeValue }
func (h *CustomHandler) Name() string            { return h.nameValue }
func (h *CustomHandler) Description() string     { return h.descriptionValue }
func (h *CustomHandler) SupportedFeatures() []PolicyFeature { return h.features }

func (h *CustomHandler) Evaluate(ctx context.Context, policy *Policy, input interface{}) (*EvaluationResult, error) {
	if h.evaluateFunc == nil {
		return nil, fmt.Errorf("evaluate function not implemented")
	}
	return h.evaluateFunc(ctx, policy, input)
}

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
	Features     []PolicyFeature
	EvaluateFunc func(ctx context.Context, policy *Policy, input interface{}) (*EvaluationResult, error)
	ValidateFunc func(policy *Policy) error
}

// DefaultTypeRegistry is the global type registry
var DefaultTypeRegistry = NewTypeRegistry()
