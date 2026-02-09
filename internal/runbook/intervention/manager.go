package intervention

import (
	"context"
	"fmt"
	"regexp"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Storage defines the interface for intervention request persistence.
type Storage interface {
	// SaveRequest saves or updates an intervention request.
	SaveRequest(ctx context.Context, req *Request) error

	// GetRequest retrieves an intervention request by ID.
	GetRequest(ctx context.Context, id string) (*Request, error)

	// GetRequestByExecution retrieves an intervention request by execution ID and step name.
	GetRequestByExecution(ctx context.Context, executionID, stepName string) (*Request, error)

	// ListRequests lists intervention requests with optional filtering.
	ListRequests(ctx context.Context, opts ListOptions) ([]*Request, error)

	// DeleteRequest deletes an intervention request.
	DeleteRequest(ctx context.Context, id string) error
}

// ListOptions specifies filtering options for listing requests.
type ListOptions struct {
	// ExecutionID filters by execution ID.
	ExecutionID string

	// State filters by request state.
	State State

	// Type filters by intervention type.
	Type Type

	// Since filters to requests created after this time.
	Since *time.Time

	// Until filters to requests created before this time.
	Until *time.Time

	// Limit is the maximum number of results.
	Limit int

	// Offset is the number of results to skip.
	Offset int
}

// Notifier defines the interface for sending intervention notifications.
type Notifier interface {
	// NotifyInterventionRequest sends a notification about a new intervention request.
	NotifyInterventionRequest(ctx context.Context, req *Request, channels []string) error

	// NotifyInterventionResponse sends a notification about an intervention response.
	NotifyInterventionResponse(ctx context.Context, req *Request, channels []string) error
}

// Manager handles intervention workflow logic.
type Manager struct {
	storage  Storage
	notifier Notifier

	mu       sync.RWMutex
	waiters  map[string][]chan *Request
	stopCh   chan struct{}
	stopOnce sync.Once
}

// NewManager creates a new intervention manager.
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

// CreateRequest creates a new intervention request.
func (m *Manager) CreateRequest(ctx context.Context, config *Config, executionID, stepName string, metadata map[string]interface{}) (*Request, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}

	if !config.Type.IsValid() {
		return nil, fmt.Errorf("invalid intervention type: %s", config.Type)
	}

	// Validate prompts for prompt-type interventions
	if config.Type == TypePrompt {
		if len(config.Prompts) == 0 {
			return nil, fmt.Errorf("at least one prompt field is required for prompt interventions")
		}
		for i, p := range config.Prompts {
			if p.Name == "" {
				return nil, fmt.Errorf("prompt[%d].name is required", i)
			}
			if !p.Type.IsValid() {
				return nil, fmt.Errorf("prompt[%d].type is invalid: %s", i, p.Type)
			}
		}
	}

	now := time.Now()
	req := &Request{
		ID:          uuid.New().String(),
		ExecutionID: executionID,
		StepName:    stepName,
		Type:        config.Type,
		State:       StatePending,
		Title:       config.Title,
		Description: config.Description,
		Prompts:     config.Prompts,
		Timeout:     config.GetTimeout(),
		Metadata:    metadata,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	// Set expiration time if timeout is configured
	if req.Timeout > 0 {
		expiresAt := now.Add(req.Timeout)
		req.ExpiresAt = &expiresAt
	}

	// Store notification channels in metadata
	if len(config.NotifyChannels) > 0 {
		if req.Metadata == nil {
			req.Metadata = make(map[string]interface{})
		}
		req.Metadata["notify_channels"] = config.NotifyChannels
	}

	// Save to storage
	if err := m.storage.SaveRequest(ctx, req); err != nil {
		return nil, fmt.Errorf("save request: %w", err)
	}

	// Send notifications
	if m.notifier != nil && len(config.NotifyChannels) > 0 {
		if err := m.notifier.NotifyInterventionRequest(ctx, req, config.NotifyChannels); err != nil {
			_ = err // Log but don't fail
		}
	}

	return req, nil
}

// Respond records an operator's response to an intervention request.
func (m *Manager) Respond(ctx context.Context, requestID, operator string, values map[string]interface{}, confirmed bool, comment string) (*Request, error) {
	req, err := m.storage.GetRequest(ctx, requestID)
	if err != nil {
		return nil, fmt.Errorf("get request: %w", err)
	}
	if req == nil {
		return nil, fmt.Errorf("request not found: %s", requestID)
	}

	// Check if request is still pending
	if req.State != StatePending {
		return nil, fmt.Errorf("request is not pending (state: %s)", req.State)
	}

	// Check if request has expired
	if req.IsExpired() {
		req.State = StateExpired
		now := time.Now()
		req.CompletedAt = &now
		req.UpdatedAt = now
		if err := m.storage.SaveRequest(ctx, req); err != nil {
			return nil, fmt.Errorf("save expired request: %w", err)
		}
		return nil, fmt.Errorf("request has expired")
	}

	// Validate response based on intervention type
	if err := m.validateResponse(req, values, confirmed); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Record the response
	now := time.Now()
	req.Response = &Response{
		Operator:    operator,
		Values:      values,
		Confirmed:   confirmed,
		Comment:     comment,
		RespondedAt: now,
	}
	req.State = StateCompleted
	req.UpdatedAt = now
	req.CompletedAt = &now

	// Save updated request
	if err := m.storage.SaveRequest(ctx, req); err != nil {
		return nil, fmt.Errorf("save request: %w", err)
	}

	// Notify waiters
	m.notifyWaiters(req)

	return req, nil
}

// validateResponse validates the response values against the request configuration.
func (m *Manager) validateResponse(req *Request, values map[string]interface{}, confirmed bool) error {
	switch req.Type {
	case TypePrompt:
		return m.validatePromptResponse(req.Prompts, values)
	case TypeConfirm:
		// Confirm type doesn't need validation - confirmed is a boolean
		return nil
	case TypeWaitManual:
		// Wait manual just needs acknowledgment
		return nil
	}
	return nil
}

// validatePromptResponse validates prompt field values.
func (m *Manager) validatePromptResponse(prompts []PromptField, values map[string]interface{}) error {
	for _, prompt := range prompts {
		value, hasValue := values[prompt.Name]

		// Check required fields
		if prompt.Required && !hasValue {
			return fmt.Errorf("field '%s' is required", prompt.Name)
		}

		// Skip further validation if no value
		if !hasValue || value == nil {
			continue
		}

		// Type-specific validation
		switch prompt.Type {
		case FieldTypeNumber:
			num, ok := toFloat64(value)
			if !ok {
				return fmt.Errorf("field '%s' must be a number", prompt.Name)
			}
			if prompt.Validation != nil {
				if prompt.Validation.Min != nil && num < *prompt.Validation.Min {
					return fmt.Errorf("field '%s' must be >= %v", prompt.Name, *prompt.Validation.Min)
				}
				if prompt.Validation.Max != nil && num > *prompt.Validation.Max {
					return fmt.Errorf("field '%s' must be <= %v", prompt.Name, *prompt.Validation.Max)
				}
			}

		case FieldTypeText, FieldTypeTextarea, FieldTypePassword:
			str, ok := value.(string)
			if !ok {
				return fmt.Errorf("field '%s' must be a string", prompt.Name)
			}
			if prompt.Validation != nil {
				if prompt.Validation.Min != nil && float64(len(str)) < *prompt.Validation.Min {
					return fmt.Errorf("field '%s' must be at least %v characters", prompt.Name, int(*prompt.Validation.Min))
				}
				if prompt.Validation.Max != nil && float64(len(str)) > *prompt.Validation.Max {
					return fmt.Errorf("field '%s' must be at most %v characters", prompt.Name, int(*prompt.Validation.Max))
				}
				if prompt.Validation.Pattern != "" {
					matched, err := regexp.MatchString(prompt.Validation.Pattern, str)
					if err != nil {
						return fmt.Errorf("invalid validation pattern for '%s': %w", prompt.Name, err)
					}
					if !matched {
						return fmt.Errorf("field '%s' does not match required pattern", prompt.Name)
					}
				}
			}

		case FieldTypeBoolean:
			if _, ok := value.(bool); !ok {
				return fmt.Errorf("field '%s' must be a boolean", prompt.Name)
			}

		case FieldTypeSelect:
			if len(prompt.Options) > 0 {
				if !m.isValidOption(value, prompt.Options) {
					return fmt.Errorf("field '%s' has invalid value", prompt.Name)
				}
			}

		case FieldTypeMultiSelect:
			values, ok := toSlice(value)
			if !ok {
				return fmt.Errorf("field '%s' must be a list", prompt.Name)
			}
			if len(prompt.Options) > 0 {
				for _, v := range values {
					if !m.isValidOption(v, prompt.Options) {
						return fmt.Errorf("field '%s' has invalid value: %v", prompt.Name, v)
					}
				}
			}
		}
	}

	return nil
}

// isValidOption checks if a value is in the list of options.
func (m *Manager) isValidOption(value interface{}, options []Option) bool {
	for _, opt := range options {
		if fmt.Sprintf("%v", opt.Value) == fmt.Sprintf("%v", value) {
			return true
		}
	}
	return false
}

// Cancel cancels a pending intervention request.
func (m *Manager) Cancel(ctx context.Context, requestID, reason string) (*Request, error) {
	req, err := m.storage.GetRequest(ctx, requestID)
	if err != nil {
		return nil, fmt.Errorf("get request: %w", err)
	}
	if req == nil {
		return nil, fmt.Errorf("request not found: %s", requestID)
	}

	if req.State != StatePending {
		return nil, fmt.Errorf("request is not pending (state: %s)", req.State)
	}

	now := time.Now()
	req.State = StateCancelled
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

// GetRequest retrieves an intervention request by ID.
func (m *Manager) GetRequest(ctx context.Context, requestID string) (*Request, error) {
	return m.storage.GetRequest(ctx, requestID)
}

// GetRequestByExecution retrieves an intervention request by execution ID and step name.
func (m *Manager) GetRequestByExecution(ctx context.Context, executionID, stepName string) (*Request, error) {
	return m.storage.GetRequestByExecution(ctx, executionID, stepName)
}

// ListRequests lists intervention requests with optional filtering.
func (m *Manager) ListRequests(ctx context.Context, opts ListOptions) ([]*Request, error) {
	return m.storage.ListRequests(ctx, opts)
}

// WaitForResponse waits for an intervention request to be completed.
func (m *Manager) WaitForResponse(ctx context.Context, requestID string) (*Request, error) {
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
	requests, err := m.storage.ListRequests(ctx, ListOptions{
		State: StatePending,
	})
	if err != nil {
		return 0, fmt.Errorf("list pending requests: %w", err)
	}

	expired := 0
	for _, req := range requests {
		if !req.IsExpired() {
			continue
		}
		now := time.Now()
		req.State = StateExpired
		req.UpdatedAt = now
		req.CompletedAt = &now

		if err := m.storage.SaveRequest(ctx, req); err != nil {
			continue
		}

		m.notifyWaiters(req)
		expired++
	}

	return expired, nil
}

// Stop stops the manager and releases resources.
func (m *Manager) Stop() {
	m.stopOnce.Do(func() {
		close(m.stopCh)
	})
}

// Helper functions

func toFloat64(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case int32:
		return float64(n), true
	}
	return 0, false
}

func toSlice(v interface{}) ([]interface{}, bool) {
	switch s := v.(type) {
	case []interface{}:
		return s, true
	case []string:
		result := make([]interface{}, len(s))
		for i, item := range s {
			result[i] = item
		}
		return result, true
	}
	return nil, false
}
