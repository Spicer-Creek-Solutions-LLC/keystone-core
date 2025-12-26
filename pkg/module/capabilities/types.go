package capabilities

import (
	"context"
	"fmt"
	"time"
)

// Capability represents a host capability that can be granted to modules
type Capability interface {
	// Name returns the capability name (e.g., "fs.read", "http.get")
	Name() string

	// Validate checks if the capability configuration is valid
	Validate() error
}

// CapabilityContext provides execution context for capability invocations
type CapabilityContext struct {
	// Context for cancellation and deadlines
	Context context.Context

	// ModuleName is the name of the module invoking the capability
	ModuleName string

	// CorrelationID for tracing capability invocations
	CorrelationID string

	// StartTime when the capability invocation started
	StartTime time.Time

	// Metadata for additional context
	Metadata map[string]interface{}
}

// NewCapabilityContext creates a new capability context
func NewCapabilityContext(ctx context.Context, moduleName string) *CapabilityContext {
	return &CapabilityContext{
		Context:       ctx,
		ModuleName:    moduleName,
		CorrelationID: generateCorrelationID(),
		StartTime:     time.Now(),
		Metadata:      make(map[string]interface{}),
	}
}

// WithCorrelationID sets the correlation ID
func (c *CapabilityContext) WithCorrelationID(id string) *CapabilityContext {
	c.CorrelationID = id
	return c
}

// WithMetadata adds metadata to the context
func (c *CapabilityContext) WithMetadata(key string, value interface{}) *CapabilityContext {
	c.Metadata[key] = value
	return c
}

// Duration returns how long the capability has been executing
func (c *CapabilityContext) Duration() time.Duration {
	return time.Since(c.StartTime)
}

// CapabilityRegistry manages available capabilities
type CapabilityRegistry struct {
	capabilities map[string]Capability
}

// NewCapabilityRegistry creates a new capability registry
func NewCapabilityRegistry() *CapabilityRegistry {
	return &CapabilityRegistry{
		capabilities: make(map[string]Capability),
	}
}

// Register registers a capability
func (r *CapabilityRegistry) Register(cap Capability) error {
	if cap == nil {
		return ErrNilCapability
	}

	name := cap.Name()
	if name == "" {
		return ErrEmptyCapabilityName
	}

	if err := cap.Validate(); err != nil {
		return fmt.Errorf("invalid capability %s: %w", name, err)
	}

	if _, exists := r.capabilities[name]; exists {
		return fmt.Errorf("%w: %s", ErrCapabilityAlreadyRegistered, name)
	}

	r.capabilities[name] = cap
	return nil
}

// Get retrieves a capability by name
func (r *CapabilityRegistry) Get(name string) (Capability, error) {
	cap, exists := r.capabilities[name]
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrCapabilityNotFound, name)
	}
	return cap, nil
}

// Has checks if a capability is registered
func (r *CapabilityRegistry) Has(name string) bool {
	_, exists := r.capabilities[name]
	return exists
}

// List returns all registered capability names
func (r *CapabilityRegistry) List() []string {
	names := make([]string, 0, len(r.capabilities))
	for name := range r.capabilities {
		names = append(names, name)
	}
	return names
}

// Unregister removes a capability
func (r *CapabilityRegistry) Unregister(name string) error {
	if !r.Has(name) {
		return fmt.Errorf("%w: %s", ErrCapabilityNotFound, name)
	}
	delete(r.capabilities, name)
	return nil
}

// Clear removes all capabilities
func (r *CapabilityRegistry) Clear() {
	r.capabilities = make(map[string]Capability)
}

// CapabilityInvoker wraps capability invocations with common functionality
type CapabilityInvoker struct {
	registry *CapabilityRegistry
	auditor  AuditLogger
}

// NewCapabilityInvoker creates a new capability invoker
func NewCapabilityInvoker(registry *CapabilityRegistry, auditor AuditLogger) *CapabilityInvoker {
	return &CapabilityInvoker{
		registry: registry,
		auditor:  auditor,
	}
}

// Invoke executes a capability with auditing
func (i *CapabilityInvoker) Invoke(ctx *CapabilityContext, capName string, fn func(Capability) (interface{}, error)) (interface{}, error) {
	// Get the capability
	cap, err := i.registry.Get(capName)
	if err != nil {
		i.audit(ctx, capName, nil, err)
		return nil, err
	}

	// Execute the capability function
	result, err := fn(cap)

	// Audit the invocation
	i.audit(ctx, capName, result, err)

	return result, err
}

// audit logs capability invocations
func (i *CapabilityInvoker) audit(ctx *CapabilityContext, capName string, result interface{}, err error) {
	if i.auditor != nil {
		entry := AuditEntry{
			Timestamp:     time.Now(),
			ModuleName:    ctx.ModuleName,
			Capability:    capName,
			CorrelationID: ctx.CorrelationID,
			Duration:      ctx.Duration(),
			Success:       err == nil,
			Error:         err,
		}
		i.auditor.Log(entry)
	}
}

// AuditLogger logs capability invocations
type AuditLogger interface {
	Log(entry AuditEntry)
}

// AuditEntry represents a capability invocation audit log
type AuditEntry struct {
	Timestamp     time.Time
	ModuleName    string
	Capability    string
	CorrelationID string
	Duration      time.Duration
	Success       bool
	Error         error
	Metadata      map[string]interface{}
}

// Helper function to generate correlation IDs
func generateCorrelationID() string {
	return fmt.Sprintf("cap-%d", time.Now().UnixNano())
}
