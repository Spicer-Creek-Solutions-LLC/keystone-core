package mocks

import (
	"context"
	"strings"
	"sync"

	"github.com/shawnbutts/keystone-core/internal/controlplane"
)

// AgentStore is an in-memory mock for controlplane.AgentStore.
type AgentStore struct {
	mu      sync.RWMutex
	agents  map[string]controlplane.StoredAgent
	ListErr error
	GetErr  error
	SaveErr error
}

// NewAgentStore returns a new mock agent store.
func NewAgentStore() *AgentStore {
	return &AgentStore{
		agents: make(map[string]controlplane.StoredAgent),
	}
}

// ListAgents lists all registered agents.
func (s *AgentStore) ListAgents(ctx context.Context, filter *controlplane.AgentFilter) ([]controlplane.StoredAgent, error) {
	_ = ctx
	if s.ListErr != nil {
		return nil, s.ListErr
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []controlplane.StoredAgent
	for i := range s.agents {
		if filter == nil || filter.Status == "" || strings.EqualFold(s.agents[i].Status, filter.Status) {
			result = append(result, s.agents[i])
		}
	}
	return result, nil
}

// GetAgent retrieves an agent by ID.
func (s *AgentStore) GetAgent(ctx context.Context, agentID string) (*controlplane.StoredAgent, error) {
	_ = ctx
	if s.GetErr != nil {
		return nil, s.GetErr
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	agent, ok := s.agents[agentID]
	if !ok {
		return nil, nil
	}
	clone := agent
	return &clone, nil
}

// SaveAgent saves an agent record.
func (s *AgentStore) SaveAgent(ctx context.Context, agent *controlplane.StoredAgent) error {
	_ = ctx
	if s.SaveErr != nil {
		return s.SaveErr
	}
	if agent == nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.agents[agent.ID] = *agent
	return nil
}
