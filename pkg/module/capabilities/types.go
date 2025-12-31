package capabilities

import (
	"context"
	"fmt"
	"time"
)

// CapabilityContext provides context for capability execution
type CapabilityContext struct {
	ModuleName    string
	ModuleVersion string
	CorrelationID string
	Context       context.Context
	StartTime     time.Time
	Metadata      map[string]interface{}
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
	if c.Metadata == nil {
		c.Metadata = make(map[string]interface{})
	}
	c.Metadata[key] = value
	return c
}

// Duration returns the elapsed time since context creation
func (c *CapabilityContext) Duration() time.Duration {
	return time.Since(c.StartTime)
}

// generateCorrelationID generates a unique correlation ID
func generateCorrelationID() string {
	return fmt.Sprintf("cap-%d", time.Now().UnixNano())
}

// AuditEntry represents an audit log entry for capability invocations
type AuditEntry struct {
	Timestamp      time.Time
	ModuleName     string
	ModuleVersion  string
	CapabilityName string
	Capability     string // Same as CapabilityName for compatibility
	Operation      string
	Success        bool
	Error          string
	Details        map[string]interface{}
}

// Capability is the interface that all capabilities must implement
type Capability interface {
	Name() string
}

// CapabilityRegistry manages registered capabilities
type CapabilityRegistry struct {
	capabilities map[string]Capability
}

// NewCapabilityRegistry creates a new capability registry
func NewCapabilityRegistry() *CapabilityRegistry {
	return &CapabilityRegistry{
		capabilities: make(map[string]Capability),
	}
}

// ValidatableCapability is a capability that can be validated
type ValidatableCapability interface {
	Capability
	Validate() error
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
	if _, exists := r.capabilities[name]; exists {
		return fmt.Errorf("%w: %s", ErrCapabilityAlreadyRegistered, name)
	}
	// Validate capability if it implements ValidatableCapability
	if validatable, ok := cap.(ValidatableCapability); ok {
		if err := validatable.Validate(); err != nil {
			return fmt.Errorf("capability validation failed: %w", err)
		}
	}
	r.capabilities[name] = cap
	return nil
}

// Has checks if a capability is registered
func (r *CapabilityRegistry) Has(name string) bool {
	_, exists := r.capabilities[name]
	return exists
}

// Get retrieves a registered capability
func (r *CapabilityRegistry) Get(name string) (Capability, error) {
	cap, exists := r.capabilities[name]
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrCapabilityNotFound, name)
	}
	return cap, nil
}

// List returns all registered capability names
func (r *CapabilityRegistry) List() []string {
	names := make([]string, 0, len(r.capabilities))
	for name := range r.capabilities {
		names = append(names, name)
	}
	return names
}

// Unregister removes a capability from the registry
func (r *CapabilityRegistry) Unregister(name string) error {
	if _, exists := r.capabilities[name]; !exists {
		return fmt.Errorf("%w: %s", ErrCapabilityNotFound, name)
	}
	delete(r.capabilities, name)
	return nil
}

// Clear removes all registered capabilities
func (r *CapabilityRegistry) Clear() {
	r.capabilities = make(map[string]Capability)
}

// CapabilityInvoker wraps capability execution with auditing
type CapabilityInvoker struct {
	registry *CapabilityRegistry
	auditor  AuditLogger
}

// AuditLogger is an interface for logging capability invocations
type AuditLogger interface {
	Log(entry AuditEntry)
}

// NewCapabilityInvoker creates a new capability invoker
func NewCapabilityInvoker(registry *CapabilityRegistry, auditor AuditLogger) *CapabilityInvoker {
	return &CapabilityInvoker{
		registry: registry,
		auditor:  auditor,
	}
}

// InvokableCapability is a capability that can be invoked
type InvokableCapability interface {
	Capability
	Invoke(ctx *CapabilityContext, args ...interface{}) (interface{}, error)
}

// Invoke invokes a capability by name with auditing using a callback function
func (i *CapabilityInvoker) Invoke(ctx *CapabilityContext, name string, fn func(Capability) (interface{}, error)) (interface{}, error) {
	startTime := time.Now()

	cap, err := i.registry.Get(name)
	if err != nil {
		// Log failed lookup attempt
		if i.auditor != nil {
			entry := AuditEntry{
				Timestamp:      startTime,
				ModuleName:     ctx.ModuleName,
				ModuleVersion:  ctx.ModuleVersion,
				CapabilityName: name,
				Capability:     name,
				Operation:      "invoke",
				Success:        false,
				Error:          err.Error(),
			}
			i.auditor.Log(entry)
		}
		return nil, err
	}

	result, invokeErr := fn(cap)
	duration := time.Since(startTime)

	// Log audit entry
	entry := AuditEntry{
		Timestamp:      startTime,
		ModuleName:     ctx.ModuleName,
		ModuleVersion:  ctx.ModuleVersion,
		CapabilityName: name,
		Capability:     name,
		Operation:      "invoke",
		Success:        invokeErr == nil,
		Details: map[string]interface{}{
			"duration": duration.String(),
		},
	}
	if invokeErr != nil {
		entry.Error = invokeErr.Error()
	}

	if i.auditor != nil {
		i.auditor.Log(entry)
	}

	return result, invokeErr
}

// InMemorySecretsStore is a simple in-memory secrets store
type InMemorySecretsStore struct {
	Secrets map[string]string
}

// Get retrieves a secret
func (s *InMemorySecretsStore) Get(path string) (string, error) {
	if val, ok := s.Secrets[path]; ok {
		return val, nil
	}
	return "", nil
}

// Set stores a secret
func (s *InMemorySecretsStore) Set(path, value string) error {
	s.Secrets[path] = value
	return nil
}

// Delete removes a secret
func (s *InMemorySecretsStore) Delete(path string) error {
	delete(s.Secrets, path)
	return nil
}

// DefaultLogger is a simple logger
type DefaultLogger struct{}

// Log writes a log message
func (l *DefaultLogger) Log(level, message string, fields map[string]interface{}) {
	// Stub implementation
}

// InMemoryKVStore is a simple in-memory KV store
type InMemoryKVStore struct {
	Data map[string]string
}

// Get retrieves a value
func (k *InMemoryKVStore) Get(key string) (string, error) {
	if val, ok := k.Data[key]; ok {
		return val, nil
	}
	return "", nil
}

// Set stores a value
func (k *InMemoryKVStore) Set(key, value string) error {
	k.Data[key] = value
	return nil
}

// Delete removes a value
func (k *InMemoryKVStore) Delete(key string) error {
	delete(k.Data, key)
	return nil
}

// List returns keys with prefix
func (k *InMemoryKVStore) List(prefix string) ([]string, error) {
	keys := make([]string, 0)
	for key := range k.Data {
		if len(prefix) == 0 || len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			keys = append(keys, key)
		}
	}
	return keys, nil
}
