package mocks

import (
	"context"
	"sync"

	"github.com/shawnbutts/keystone-core/internal/files/access"
)

// PolicyEvaluator is a mock for access.PolicyEvaluator.
type PolicyEvaluator struct {
	Result   *access.Result
	Err      error
	Requests []*access.Request
	mu       sync.Mutex
}

// Evaluate evaluates the policy against the input.
func (m *PolicyEvaluator) Evaluate(ctx context.Context, req *access.Request) (*access.Result, error) {
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
	return &access.Result{Allowed: true}, nil
}
