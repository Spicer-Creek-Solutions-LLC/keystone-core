package dns

import (
	"context"
	"fmt"
	"sync"
)

// Provider defines the interface for DNS providers.
// This interface is compatible with libdns and allows implementing
// custom providers or wrapping libdns implementations.
type Provider interface {
	// GetRecords retrieves all DNS records for a zone.
	GetRecords(ctx context.Context, zone string) ([]Record, error)

	// CreateRecord creates a new DNS record.
	CreateRecord(ctx context.Context, zone string, record Record) (*Record, error)

	// UpdateRecord updates an existing DNS record.
	UpdateRecord(ctx context.Context, zone string, record Record) (*Record, error)

	// DeleteRecord deletes a DNS record.
	DeleteRecord(ctx context.Context, zone string, record Record) error

	// Capabilities returns the provider's capabilities.
	Capabilities() ProviderCapabilities
}

// ProviderFactory creates a Provider instance from credentials.
type ProviderFactory func(creds ResolvedCredentials) (Provider, error)

// ResolvedCredentials contains resolved provider credentials.
type ResolvedCredentials struct {
	// APIKey is the API key (if applicable)
	APIKey string

	// APIToken is the API token (if applicable)
	APIToken string

	// AccountID is the account/project ID (if applicable)
	AccountID string

	// Extra contains provider-specific credentials
	Extra map[string]string
}

// Registry manages DNS provider factories.
type Registry struct {
	mu        sync.RWMutex
	providers map[string]ProviderFactory
	caps      map[string]ProviderCapabilities
}

// NewRegistry creates a new provider registry.
func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]ProviderFactory),
		caps:      make(map[string]ProviderCapabilities),
	}
}

// Register registers a provider factory.
func (r *Registry) Register(name string, factory ProviderFactory, caps ProviderCapabilities) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.providers[name]; exists {
		return fmt.Errorf("provider already registered: %s", name)
	}

	r.providers[name] = factory
	r.caps[name] = caps
	return nil
}

// Get retrieves a provider factory by name.
func (r *Registry) Get(name string) (ProviderFactory, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	factory, exists := r.providers[name]
	return factory, exists
}

// GetCapabilities retrieves provider capabilities by name.
func (r *Registry) GetCapabilities(name string) (ProviderCapabilities, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	caps, exists := r.caps[name]
	return caps, exists
}

// List returns all registered provider names.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	return names
}

// CreateProvider creates a provider instance.
func (r *Registry) CreateProvider(name string, creds ResolvedCredentials) (Provider, error) {
	factory, exists := r.Get(name)
	if !exists {
		return nil, fmt.Errorf("unknown provider: %s", name)
	}
	return factory(creds)
}

// DefaultRegistry is the global provider registry.
var DefaultRegistry = NewRegistry()

// RegisterProvider registers a provider with the default registry.
func RegisterProvider(name string, factory ProviderFactory, caps ProviderCapabilities) error {
	return DefaultRegistry.Register(name, factory, caps)
}

// GetProvider retrieves a provider factory from the default registry.
func GetProvider(name string) (ProviderFactory, bool) {
	return DefaultRegistry.Get(name)
}

// ListProviders returns all registered provider names from the default registry.
func ListProviders() []string {
	return DefaultRegistry.List()
}
