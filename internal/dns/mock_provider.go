package dns

import (
	"context"
	"fmt"
	"sync"
)

// MockProvider is a thread-safe mock DNS provider for testing.
type MockProvider struct {
	mu      sync.RWMutex
	records map[string][]Record // records by zone
	caps    ProviderCapabilities

	// Error injection for testing
	GetErr    error
	CreateErr error
	UpdateErr error
	DeleteErr error

	// Call counters for verification
	GetCalled    int
	CreateCalled int
	UpdateCalled int
	DeleteCalled int
}

// NewMockProvider creates a new mock provider.
func NewMockProvider() *MockProvider {
	return &MockProvider{
		records: make(map[string][]Record),
		caps: ProviderCapabilities{
			SupportedRecordTypes: AllRecordTypes(),
			SupportsProxied:      true,
			MinTTL:               60,
			MaxTTL:               86400,
			SupportsRootRecords:  true,
			SupportsALIAS:        true,
		},
	}
}

// SetRecords sets the initial records for a zone (for test setup).
func (m *MockProvider) SetRecords(zone string, records []Record) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.records[zone] = make([]Record, len(records))
	copy(m.records[zone], records)
}

// GetRecords retrieves all DNS records for a zone.
func (m *MockProvider) GetRecords(ctx context.Context, zone string) ([]Record, error) {
	m.mu.Lock()
	m.GetCalled++
	m.mu.Unlock()

	if m.GetErr != nil {
		return nil, m.GetErr
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	records := m.records[zone]
	result := make([]Record, len(records))
	copy(result, records)
	return result, nil
}

// CreateRecord creates a new DNS record.
func (m *MockProvider) CreateRecord(ctx context.Context, zone string, record Record) (*Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.CreateCalled++

	if m.CreateErr != nil {
		return nil, m.CreateErr
	}

	// Generate ID
	record.ID = fmt.Sprintf("mock-%s-%s-%d", record.Type, record.Name, m.CreateCalled)

	m.records[zone] = append(m.records[zone], record)
	return &record, nil
}

// UpdateRecord updates an existing DNS record.
func (m *MockProvider) UpdateRecord(ctx context.Context, zone string, record Record) (*Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.UpdateCalled++

	if m.UpdateErr != nil {
		return nil, m.UpdateErr
	}

	records := m.records[zone]
	for i, r := range records {
		if r.ID == record.ID {
			m.records[zone][i] = record
			return &record, nil
		}
	}

	return nil, fmt.Errorf("record not found: %s", record.ID)
}

// DeleteRecord deletes a DNS record.
func (m *MockProvider) DeleteRecord(ctx context.Context, zone string, record Record) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.DeleteCalled++

	if m.DeleteErr != nil {
		return m.DeleteErr
	}

	records := m.records[zone]
	for i, r := range records {
		if r.ID == record.ID {
			m.records[zone] = append(records[:i], records[i+1:]...)
			return nil
		}
	}

	return fmt.Errorf("record not found: %s", record.ID)
}

// Capabilities returns the provider's capabilities.
func (m *MockProvider) Capabilities() ProviderCapabilities {
	return m.caps
}

// SetCapabilities sets the provider capabilities (for testing).
func (m *MockProvider) SetCapabilities(caps ProviderCapabilities) {
	m.caps = caps
}

// Reset resets all call counters and error injections.
func (m *MockProvider) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.GetCalled = 0
	m.CreateCalled = 0
	m.UpdateCalled = 0
	m.DeleteCalled = 0
	m.GetErr = nil
	m.CreateErr = nil
	m.UpdateErr = nil
	m.DeleteErr = nil
}

// Ensure MockProvider implements Provider
var _ Provider = (*MockProvider)(nil)

// RegisterMockProvider registers a mock provider factory with the given registry.
func RegisterMockProvider(registry *Registry, name string) *MockProvider {
	mock := NewMockProvider()
	_ = registry.Register(name, func(creds ResolvedCredentials) (Provider, error) { //nolint:errcheck // registration in init
		return mock, nil
	}, mock.Capabilities())
	return mock
}

func init() {
	// Register mock provider in default registry for testing
	RegisterMockProvider(DefaultRegistry, "mock")
}
