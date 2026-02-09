// Package apikeys provides HTTP handlers for API key management endpoints.
package apikeys

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// APIKey represents a stored API key.
type APIKey struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	KeyHash   string    `json:"-"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
	LastUsed  time.Time `json:"last_used,omitempty"`
	Revoked   bool      `json:"revoked"`
}

// Store defines the interface for API key persistence.
type Store interface {
	Create(key *APIKey) error
	List() ([]*APIKey, error)
	Get(id string) (*APIKey, error)
	Delete(id string) error
}

// memoryStore stores API keys in memory.
type memoryStore struct {
	mu   sync.RWMutex
	keys map[string]*APIKey
}

// NewMemoryStore creates a Store backed by local memory.
func NewMemoryStore() Store {
	return &memoryStore{
		keys: make(map[string]*APIKey),
	}
}

func (s *memoryStore) Create(key *APIKey) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *key
	s.keys[key.ID] = &cp
	return nil
}

func (s *memoryStore) List() ([]*APIKey, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*APIKey, 0, len(s.keys))
	for _, k := range s.keys {
		if !k.Revoked {
			cp := *k
			result = append(result, &cp)
		}
	}
	return result, nil
}

func (s *memoryStore) Get(id string) (*APIKey, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	k, ok := s.keys[id]
	if !ok {
		return nil, fmt.Errorf("api key not found: %s", id)
	}
	cp := *k
	return &cp, nil
}

func (s *memoryStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k, ok := s.keys[id]
	if !ok {
		return fmt.Errorf("api key not found: %s", id)
	}
	k.Revoked = true
	return nil
}

// GenerateID creates a short random hex ID for API keys.
func GenerateID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("crypto/rand failed: %v", err))
	}
	return hex.EncodeToString(b)
}

// GenerateKey creates a random API key string.
func GenerateKey() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("crypto/rand failed: %v", err))
	}
	return "ks_" + hex.EncodeToString(b)
}
