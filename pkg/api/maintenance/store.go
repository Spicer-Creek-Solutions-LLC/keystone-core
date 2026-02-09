// Package maintenance provides HTTP handlers for maintenance mode API endpoints.
package maintenance

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/shawnbutts/keystone-core/internal/cluster"
)

// State represents the current maintenance mode state.
type State struct {
	Enabled   bool      `json:"enabled"`
	Reason    string    `json:"reason,omitempty"`
	EnabledAt time.Time `json:"enabled_at,omitempty"`
	Duration  string    `json:"duration,omitempty"`
	EnabledBy string    `json:"enabled_by,omitempty"`
}

// Store defines the interface for maintenance state persistence.
type Store interface {
	GetState(ctx context.Context) (*State, error)
	SetState(ctx context.Context, state *State) error
	ClearState(ctx context.Context) error
}

// etcdStore stores maintenance state in etcd.
type etcdStore struct {
	client *cluster.EtcdClient
	key    string
}

// NewEtcdStore creates a Store backed by etcd.
func NewEtcdStore(client *cluster.EtcdClient) Store {
	return &etcdStore{
		client: client,
		key:    "maintenance/state",
	}
}

func (s *etcdStore) GetState(ctx context.Context) (*State, error) {
	data, err := s.client.Get(ctx, s.key)
	if err != nil {
		return nil, fmt.Errorf("etcd get: %w", err)
	}
	if data == nil {
		return &State{}, nil
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("unmarshal state: %w", err)
	}
	return &state, nil
}

func (s *etcdStore) SetState(ctx context.Context, state *State) error {
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	if err := s.client.Put(ctx, s.key, data, 0); err != nil {
		return fmt.Errorf("etcd put: %w", err)
	}
	return nil
}

func (s *etcdStore) ClearState(ctx context.Context) error {
	if err := s.client.Delete(ctx, s.key); err != nil {
		return fmt.Errorf("etcd delete: %w", err)
	}
	return nil
}

// memoryStore stores maintenance state in memory. Used as the database-backed
// fallback when etcd is not available. In a full implementation this would
// use SQLite/PostgreSQL via the state.Store interface; for now it provides
// the same contract with process-local storage.
type memoryStore struct {
	mu    sync.RWMutex
	state *State
}

// NewMemoryStore creates a Store backed by local memory.
// This is used when etcd is not available (standalone deployments).
func NewMemoryStore() Store {
	return &memoryStore{
		state: &State{},
	}
}

func (s *memoryStore) GetState(_ context.Context) (*State, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cp := *s.state
	return &cp, nil
}

func (s *memoryStore) SetState(_ context.Context, state *State) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *state
	s.state = &cp
	return nil
}

func (s *memoryStore) ClearState(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = &State{}
	return nil
}
