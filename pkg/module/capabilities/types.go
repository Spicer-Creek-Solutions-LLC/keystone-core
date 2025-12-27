package capabilities

import "context"

// CapabilityContext provides context for capability execution
type CapabilityContext struct {
	ModuleName    string
	ModuleVersion string
	CorrelationID string
	Context       context.Context
}

// Capability is the interface that all capabilities must implement
type Capability interface {
	Name() string
}

// CapabilityRegistry manages registered capabilities
type CapabilityRegistry struct {
	// Implementation omitted for now
}

// NewCapabilityRegistry creates a new capability registry
func NewCapabilityRegistry() *CapabilityRegistry {
	return &CapabilityRegistry{}
}

// Register registers a capability
func (r *CapabilityRegistry) Register(cap Capability) error {
	// Stub implementation
	return nil
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
