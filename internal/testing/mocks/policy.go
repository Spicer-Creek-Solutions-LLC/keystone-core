package mocks

import (
	"context"
	"sync"

	"github.com/shawnbutts/keystone-core/internal/files/access"
)

// PolicyEvaluator is a mock for access.PolicyEvaluator.
type PolicyEvaluator struct {
	Result   *access.AccessResult
	Err      error
	Requests []*access.AccessRequest
	mu       sync.Mutex
}

func (m *PolicyEvaluator) Evaluate(ctx context.Context, req *access.AccessRequest) (*access.AccessResult, error) {
	_ = ctx
	m.mu.Lock()
	m.Requests = append(m.Requests, req)
	m.mu.Unlock()

	if m.Err != nil {
		return nil, m.Err
	}
	if m.Result != nil {
		return m.Result, nil
	}
	return &access.AccessResult{Allowed: true}, nil
}
