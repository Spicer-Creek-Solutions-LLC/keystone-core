// Package federation provides trust federation between identity providers.
package federation

import (
	"context"
	"fmt"
	"sync"
)

// InMemoryStore is an in-memory implementation of Store.
type InMemoryStore struct {
	mu      sync.RWMutex
	domains map[string]*FederatedDomain
}

// NewInMemoryStore creates a new in-memory federation store.
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		domains: make(map[string]*FederatedDomain),
	}
}

// Save saves a federated domain.
func (s *InMemoryStore) Save(ctx context.Context, domain *FederatedDomain) error {
	if domain == nil {
		return fmt.Errorf("domain is required")
	}
	if domain.TrustDomain == "" {
		return fmt.Errorf("trust domain is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Deep copy to avoid external modifications
	copied := *domain
	s.domains[domain.TrustDomain] = &copied

	return nil
}

// Load loads a federated domain.
func (s *InMemoryStore) Load(ctx context.Context, trustDomain string) (*FederatedDomain, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	domain, exists := s.domains[trustDomain]
	if !exists {
		return nil, fmt.Errorf("trust domain %s not found", trustDomain)
	}

	// Return a copy
	copied := *domain
	return &copied, nil
}

// Delete deletes a federated domain.
func (s *InMemoryStore) Delete(ctx context.Context, trustDomain string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.domains[trustDomain]; !exists {
		return fmt.Errorf("trust domain %s not found", trustDomain)
	}

	delete(s.domains, trustDomain)
	return nil
}

// List lists all federated domains.
func (s *InMemoryStore) List(ctx context.Context) ([]*FederatedDomain, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	domains := make([]*FederatedDomain, 0, len(s.domains))
	for _, domain := range s.domains {
		copied := *domain
		domains = append(domains, &copied)
	}

	return domains, nil
}

// Clear clears all federated domains.
func (s *InMemoryStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.domains = make(map[string]*FederatedDomain)
}

// Count returns the number of federated domains.
func (s *InMemoryStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.domains)
}

// Verify InMemoryStore implements Store
var _ Store = (*InMemoryStore)(nil)
