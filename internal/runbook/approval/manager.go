package approval

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Storage defines the interface for approval request persistence.
type Storage interface {
	// SaveRequest saves or updates an approval request.
	SaveRequest(ctx context.Context, req *Request) error

	// GetRequest retrieves an approval request by ID.
	GetRequest(ctx context.Context, id string) (*Request, error)

	// GetRequestByExecution retrieves an approval request by execution ID and step name.
	GetRequestByExecution(ctx context.Context, executionID, stepName string) (*Request, error)

	// ListRequests lists approval requests with optional filtering.
	ListRequests(ctx context.Context, opts ListOptions) ([]*Request, error)

	// DeleteRequest deletes an approval request.
	DeleteRequest(ctx context.Context, id string) error
}

// ListOptions specifies filtering options for listing requests.
type ListOptions struct {
	// ExecutionID filters by execution ID.
	ExecutionID string

	// State filters by request state.
	State RequestState

	// Approver filters to requests where this identity is an approver.
	Approver string

	// IncludePending includes pending requests.
	IncludePending bool

	// IncludeExpired includes expired requests.
	IncludeExpired bool

	// Since filters to requests created after this time.
	Since *time.Time

	// Until filters to requests created before this time.
	Until *time.Time

	// Limit is the maximum number of results.
	Limit int

	// Offset is the number of results to skip.
	Offset int
}

// Notifier defines the interface for sending approval notifications.
type Notifier interface {
	// NotifyApprovalRequest sends a notification about a new approval request.
	NotifyApprovalRequest(ctx context.Context, req *Request, channels []string) error

	// NotifyApprovalDecision sends a notification about an approval decision.
	NotifyApprovalDecision(ctx context.Context, req *Request, resp Response, channels []string) error

	// NotifyApprovalReminder sends a reminder for a pending approval.
	NotifyApprovalReminder(ctx context.Context, req *Request, channels []string) error

	// NotifyApprovalExpired sends a notification that an approval has expired.
	NotifyApprovalExpired(ctx context.Context, req *Request, channels []string) error
}

// EventPublisher defines the interface for publishing approval events.
type EventPublisher interface {
	// PublishApprovalRequested publishes an event when approval is requested.
	PublishApprovalRequested(ctx context.Context, req *Request) error

	// PublishApprovalDecision publishes an event when a decision is made.
	PublishApprovalDecision(ctx context.Context, req *Request, resp Response) error

	// PublishApprovalCompleted publishes an event when approval is complete.
	PublishApprovalCompleted(ctx context.Context, req *Request) error
}

// Manager handles approval workflow logic.
type Manager struct {
	storage   Storage
	notifier  Notifier
	publisher EventPublisher

	mu       sync.RWMutex
	waiters  map[string][]chan *Request // Channels waiting for approval completion
	stopCh   chan struct{}
	stopOnce sync.Once
}

// NewManager creates a new approval manager.
func NewManager(storage Storage, opts ...ManagerOption) *Manager {
	m := &Manager{
		storage: storage,
		waiters: make(map[string][]chan *Request),
		stopCh:  make(chan struct{}),
	}

	for _, opt := range opts {
		opt(m)
	}

	return m
}

// ManagerOption configures the Manager.
type ManagerOption func(*Manager)

// WithNotifier sets the notifier for the manager.
func WithNotifier(n Notifier) ManagerOption {
	return func(m *Manager) {
		m.notifier = n
	}
}

// WithEventPublisher sets the event publisher for the manager.
func WithEventPublisher(p EventPublisher) ManagerOption {
	return func(m *Manager) {
		m.publisher = p
	}
}

// CreateRequest creates a new approval request.
func (m *Manager) CreateRequest(ctx context.Context, config *Config, executionID, stepName string, metadata map[string]interface{}) (*Request, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}

	if len(config.Approvers) == 0 {
		return nil, fmt.Errorf("at least one approver is required")
	}

	if !config.GetMode().IsValid() {
		return nil, fmt.Errorf("invalid approval mode: %s", config.Mode)
	}

	now := time.Now()
	req := &Request{
		ID:            uuid.New().String(),
		ExecutionID:   executionID,
		StepName:      stepName,
		State:         RequestStatePending,
		Title:         config.Title,
		Description:   config.Description,
		Approvers:     config.Approvers,
		Mode:          config.GetMode(),
		RequiredCount: config.GetRequiredCount(),
		Timeout:       config.GetTimeout(),
		Metadata:      metadata,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	// Set expiration time if timeout is configured
	if req.Timeout > 0 {
		expiresAt := now.Add(req.Timeout)
		req.ExpiresAt = &expiresAt
	}

	// Save to storage
	if err := m.storage.SaveRequest(ctx, req); err != nil {
		return nil, fmt.Errorf("save request: %w", err)
	}

	// Send notifications
	if m.notifier != nil && len(config.NotifyChannels) > 0 {
		if err := m.notifier.NotifyApprovalRequest(ctx, req, config.NotifyChannels); err != nil {
			// Log but don't fail - notification is best effort
			_ = err
		}
	}

	// Publish event
	if m.publisher != nil {
		if err := m.publisher.PublishApprovalRequested(ctx, req); err != nil {
			_ = err
		}
	}

	return req, nil
}

// Respond records an approval decision.
func (m *Manager) Respond(ctx context.Context, requestID string, approver string, decision Decision, comment string) (*Request, error) {
	req, err := m.storage.GetRequest(ctx, requestID)
	if err != nil {
		return nil, fmt.Errorf("get request: %w", err)
	}
	if req == nil {
		return nil, fmt.Errorf("request not found: %s", requestID)
	}

	// Check if request is still pending
	if req.State != RequestStatePending {
		return nil, fmt.Errorf("request is not pending (state: %s)", req.State)
	}

	// Check if request has expired
	if req.IsExpired() {
		req.State = RequestStateExpired
		now := time.Now()
		req.CompletedAt = &now
		req.UpdatedAt = now
		if err := m.storage.SaveRequest(ctx, req); err != nil {
			return nil, fmt.Errorf("save expired request: %w", err)
		}
		return nil, fmt.Errorf("request has expired")
	}

	// Check if approver is authorized
	if !req.IsApprover(approver) {
		return nil, fmt.Errorf("'%s' is not authorized to approve this request", approver)
	}

	// Check if approver has already responded
	if req.HasResponded(approver) {
		return nil, fmt.Errorf("'%s' has already responded to this request", approver)
	}

	// Record the response
	resp := Response{
		Approver:    approver,
		Decision:    decision,
		Comment:     comment,
		RespondedAt: time.Now(),
	}
	req.Responses = append(req.Responses, resp)
	req.UpdatedAt = time.Now()

	// Evaluate the request state
	newState := m.evaluateState(req)
	if newState != req.State {
		req.State = newState
		if newState.IsTerminal() {
			now := time.Now()
			req.CompletedAt = &now
		}
	}

	// Save updated request
	if err := m.storage.SaveRequest(ctx, req); err != nil {
		return nil, fmt.Errorf("save request: %w", err)
	}

	// Publish decision event
	if m.publisher != nil {
		if err := m.publisher.PublishApprovalDecision(ctx, req, resp); err != nil {
			_ = err
		}
	}

	// If completed, notify waiters and publish completion event
	if req.State.IsTerminal() {
		m.notifyWaiters(req)
		if m.publisher != nil {
			if err := m.publisher.PublishApprovalCompleted(ctx, req); err != nil {
				_ = err
			}
		}
	}

	return req, nil
}

// evaluateState determines the request state based on responses.
func (m *Manager) evaluateState(req *Request) RequestState {
	approvals := req.ApprovalCount()
	rejections := req.RejectionCount()

	switch req.Mode {
	case ApprovalModeAny:
		// Any single approval is enough
		if approvals >= 1 {
			return RequestStateApproved
		}
		// If all approvers have rejected, the request is rejected
		if rejections == len(req.Approvers) {
			return RequestStateRejected
		}

	case ApprovalModeAll:
		// Any rejection rejects the whole request
		if rejections > 0 {
			return RequestStateRejected
		}
		// All approvers must approve
		if approvals == len(req.Approvers) {
			return RequestStateApproved
		}

	case ApprovalModeCount:
		// Check if we have enough approvals
		if approvals >= req.RequiredCount {
			return RequestStateApproved
		}
		// Check if it's impossible to reach required count
		remaining := len(req.Approvers) - len(req.Responses)
		if approvals+remaining < req.RequiredCount {
			return RequestStateRejected
		}
	}

	return RequestStatePending
}

// Cancel cancels a pending approval request.
func (m *Manager) Cancel(ctx context.Context, requestID string, reason string) (*Request, error) {
	req, err := m.storage.GetRequest(ctx, requestID)
	if err != nil {
		return nil, fmt.Errorf("get request: %w", err)
	}
	if req == nil {
		return nil, fmt.Errorf("request not found: %s", requestID)
	}

	if req.State != RequestStatePending {
		return nil, fmt.Errorf("request is not pending (state: %s)", req.State)
	}

	now := time.Now()
	req.State = RequestStateCancelled
	req.UpdatedAt = now
	req.CompletedAt = &now
	if reason != "" {
		if req.Metadata == nil {
			req.Metadata = make(map[string]interface{})
		}
		req.Metadata["cancel_reason"] = reason
	}

	if err := m.storage.SaveRequest(ctx, req); err != nil {
		return nil, fmt.Errorf("save request: %w", err)
	}

	m.notifyWaiters(req)

	return req, nil
}

// GetRequest retrieves an approval request by ID.
func (m *Manager) GetRequest(ctx context.Context, requestID string) (*Request, error) {
	return m.storage.GetRequest(ctx, requestID)
}

// GetRequestByExecution retrieves an approval request by execution ID and step name.
func (m *Manager) GetRequestByExecution(ctx context.Context, executionID, stepName string) (*Request, error) {
	return m.storage.GetRequestByExecution(ctx, executionID, stepName)
}

// ListRequests lists approval requests with optional filtering.
func (m *Manager) ListRequests(ctx context.Context, opts ListOptions) ([]*Request, error) {
	return m.storage.ListRequests(ctx, opts)
}

// ListPendingRequests lists all pending requests for an approver.
func (m *Manager) ListPendingRequests(ctx context.Context, approver string) ([]*Request, error) {
	return m.storage.ListRequests(ctx, ListOptions{
		Approver:       approver,
		State:          RequestStatePending,
		IncludePending: true,
	})
}

// WaitForApproval waits for an approval request to complete.
func (m *Manager) WaitForApproval(ctx context.Context, requestID string) (*Request, error) {
	// Check if already complete
	req, err := m.storage.GetRequest(ctx, requestID)
	if err != nil {
		return nil, fmt.Errorf("get request: %w", err)
	}
	if req == nil {
		return nil, fmt.Errorf("request not found: %s", requestID)
	}

	if req.State.IsTerminal() {
		return req, nil
	}

	// Register waiter
	ch := make(chan *Request, 1)
	m.mu.Lock()
	m.waiters[requestID] = append(m.waiters[requestID], ch)
	m.mu.Unlock()

	// Clean up on exit
	defer func() {
		m.mu.Lock()
		waiters := m.waiters[requestID]
		for i, w := range waiters {
			if w == ch {
				m.waiters[requestID] = append(waiters[:i], waiters[i+1:]...)
				break
			}
		}
		if len(m.waiters[requestID]) == 0 {
			delete(m.waiters, requestID)
		}
		m.mu.Unlock()
		close(ch)
	}()

	// Wait for completion or context cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case req := <-ch:
		return req, nil
	case <-m.stopCh:
		return nil, fmt.Errorf("manager stopped")
	}
}

// notifyWaiters notifies all waiters that a request has completed.
func (m *Manager) notifyWaiters(req *Request) {
	m.mu.RLock()
	waiters := m.waiters[req.ID]
	m.mu.RUnlock()

	for _, ch := range waiters {
		select {
		case ch <- req:
		default:
		}
	}
}

// CheckExpired checks for and marks expired requests.
func (m *Manager) CheckExpired(ctx context.Context) (int, error) {
	// List pending requests
	requests, err := m.storage.ListRequests(ctx, ListOptions{
		State:          RequestStatePending,
		IncludePending: true,
	})
	if err != nil {
		return 0, fmt.Errorf("list pending requests: %w", err)
	}

	expired := 0
	for _, req := range requests {
		if req.IsExpired() {
			now := time.Now()
			req.State = RequestStateExpired
			req.UpdatedAt = now
			req.CompletedAt = &now

			if err := m.storage.SaveRequest(ctx, req); err != nil {
				continue
			}

			m.notifyWaiters(req)

			if m.notifier != nil {
				_ = m.notifier.NotifyApprovalExpired(ctx, req, nil)
			}

			if m.publisher != nil {
				_ = m.publisher.PublishApprovalCompleted(ctx, req)
			}

			expired++
		}
	}

	return expired, nil
}

// Stop stops the manager and releases resources.
func (m *Manager) Stop() {
	m.stopOnce.Do(func() {
		close(m.stopCh)
	})
}
