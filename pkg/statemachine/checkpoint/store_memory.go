package checkpoint

import (
	"context"
	"sync"
)

// MemoryStore is an in-memory Store implementation for testing.
type MemoryStore struct {
	mu      sync.Mutex
	records map[string]*Record
}

// NewMemoryStore creates a new in-memory checkpoint store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{records: make(map[string]*Record)}
}

// Save persists a checkpoint record (deep copy to avoid external mutation).
func (m *MemoryStore) Save(_ context.Context, r *Record) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cp := *r
	if r.Metadata != nil {
		cp.Metadata = make(map[string]any, len(r.Metadata))
		for k, v := range r.Metadata {
			cp.Metadata[k] = v
		}
	}
	m.records[r.MachineID] = &cp
	return nil
}

// Load retrieves a checkpoint by machine ID. Returns nil if not found.
func (m *MemoryStore) Load(_ context.Context, machineID string) (*Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	r, ok := m.records[machineID]
	if !ok {
		return nil, nil
	}
	return r, nil
}

// Delete removes a checkpoint by machine ID.
func (m *MemoryStore) Delete(_ context.Context, machineID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.records, machineID)
	return nil
}

// List returns all checkpoints filtered by machine name.
func (m *MemoryStore) List(_ context.Context, machineName string) ([]*Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var result []*Record
	for _, r := range m.records {
		if machineName != "" && r.MachineName != machineName {
			continue
		}
		result = append(result, r)
	}
	return result, nil
}
