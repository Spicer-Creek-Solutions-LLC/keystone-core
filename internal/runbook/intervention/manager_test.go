package intervention

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// mockStorage implements Storage for testing.
type mockStorage struct {
	mu       sync.Mutex
	requests map[string]*Request
}

func newMockStorage() *mockStorage {
	return &mockStorage{
		requests: make(map[string]*Request),
	}
}

func (s *mockStorage) SaveRequest(ctx context.Context, req *Request) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests[req.ID] = req
	return nil
}

func (s *mockStorage) GetRequest(ctx context.Context, id string) (*Request, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.requests[id], nil
}

func (s *mockStorage) GetRequestByExecution(ctx context.Context, executionID, stepName string) (*Request, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, req := range s.requests {
		if req.ExecutionID == executionID && req.StepName == stepName {
			return req, nil
		}
	}
	return nil, nil
}

func (s *mockStorage) ListRequests(ctx context.Context, opts ListOptions) ([]*Request, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []*Request
	for _, req := range s.requests {
		if opts.ExecutionID != "" && req.ExecutionID != opts.ExecutionID {
			continue
		}
		if opts.State != "" && req.State != opts.State {
			continue
		}
		if opts.Type != "" && req.Type != opts.Type {
			continue
		}
		result = append(result, req)
	}
	return result, nil
}

func (s *mockStorage) DeleteRequest(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.requests, id)
	return nil
}

// mockNotifier implements Notifier for testing.
type mockNotifier struct {
	mu            sync.Mutex
	requestCalls  []*Request
	responseCalls []struct {
		req      *Request
		channels []string
	}
}

func newMockNotifier() *mockNotifier {
	return &mockNotifier{}
}

func (n *mockNotifier) NotifyInterventionRequest(ctx context.Context, req *Request, channels []string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.requestCalls = append(n.requestCalls, req)
	return nil
}

func (n *mockNotifier) NotifyInterventionResponse(ctx context.Context, req *Request, channels []string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.responseCalls = append(n.responseCalls, struct {
		req      *Request
		channels []string
	}{req, channels})
	return nil
}

func (n *mockNotifier) RequestCount() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return len(n.requestCalls)
}

func TestManager_CreateRequest_Confirm(t *testing.T) {
	storage := newMockStorage()
	manager := NewManager(storage)

	config := &Config{
		Type:        TypeConfirm,
		Title:       "Confirm deployment",
		Description: "Please confirm to proceed with deployment",
		Timeout:     "1h",
	}

	req, err := manager.CreateRequest(context.Background(), config, "exec-1", "deploy", nil)
	if err != nil {
		t.Fatalf("CreateRequest: %v", err)
	}

	if req.ID == "" {
		t.Error("expected ID to be set")
	}
	if req.ExecutionID != "exec-1" {
		t.Errorf("ExecutionID = %q, want %q", req.ExecutionID, "exec-1")
	}
	if req.StepName != "deploy" {
		t.Errorf("StepName = %q, want %q", req.StepName, "deploy")
	}
	if req.Type != TypeConfirm {
		t.Errorf("Type = %q, want %q", req.Type, TypeConfirm)
	}
	if req.State != StatePending {
		t.Errorf("State = %q, want %q", req.State, StatePending)
	}
	if req.Timeout != time.Hour {
		t.Errorf("Timeout = %v, want %v", req.Timeout, time.Hour)
	}
	if req.ExpiresAt == nil {
		t.Error("expected ExpiresAt to be set")
	}
}

func TestManager_CreateRequest_Prompt(t *testing.T) {
	storage := newMockStorage()
	manager := NewManager(storage)

	config := &Config{
		Type:  TypePrompt,
		Title: "Enter configuration",
		Prompts: []PromptField{
			{
				Name:     "version",
				Label:    "Version",
				Type:     FieldTypeText,
				Required: true,
			},
			{
				Name:    "replicas",
				Label:   "Replica Count",
				Type:    FieldTypeNumber,
				Default: 3,
			},
		},
	}

	req, err := manager.CreateRequest(context.Background(), config, "exec-1", "config", nil)
	if err != nil {
		t.Fatalf("CreateRequest: %v", err)
	}

	if req.Type != TypePrompt {
		t.Errorf("Type = %q, want %q", req.Type, TypePrompt)
	}
	if len(req.Prompts) != 2 {
		t.Errorf("len(Prompts) = %d, want 2", len(req.Prompts))
	}
}

func TestManager_CreateRequest_PromptValidation(t *testing.T) {
	storage := newMockStorage()
	manager := NewManager(storage)

	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
		},
		{
			name: "invalid type",
			config: &Config{
				Type:  "invalid",
				Title: "Test",
			},
			wantErr: true,
		},
		{
			name: "prompt without fields",
			config: &Config{
				Type:    TypePrompt,
				Title:   "Test",
				Prompts: []PromptField{},
			},
			wantErr: true,
		},
		{
			name: "prompt field without name",
			config: &Config{
				Type:  TypePrompt,
				Title: "Test",
				Prompts: []PromptField{
					{Label: "Test", Type: FieldTypeText},
				},
			},
			wantErr: true,
		},
		{
			name: "prompt field with invalid type",
			config: &Config{
				Type:  TypePrompt,
				Title: "Test",
				Prompts: []PromptField{
					{Name: "test", Type: "invalid"},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := manager.CreateRequest(context.Background(), tt.config, "exec-1", "step1", nil)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateRequest error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestManager_CreateRequest_WaitManual(t *testing.T) {
	storage := newMockStorage()
	manager := NewManager(storage)

	config := &Config{
		Type:        TypeWaitManual,
		Title:       "Manual verification",
		Description: "Please verify the deployment and acknowledge",
	}

	req, err := manager.CreateRequest(context.Background(), config, "exec-1", "verify", nil)
	if err != nil {
		t.Fatalf("CreateRequest: %v", err)
	}

	if req.Type != TypeWaitManual {
		t.Errorf("Type = %q, want %q", req.Type, TypeWaitManual)
	}
}

func TestManager_CreateRequest_WithNotifier(t *testing.T) {
	storage := newMockStorage()
	notifier := newMockNotifier()
	manager := NewManager(storage, WithNotifier(notifier))

	config := &Config{
		Type:           TypeConfirm,
		Title:          "Test",
		NotifyChannels: []string{"slack"},
	}

	_, err := manager.CreateRequest(context.Background(), config, "exec-1", "step1", nil)
	if err != nil {
		t.Fatalf("CreateRequest: %v", err)
	}

	if notifier.RequestCount() != 1 {
		t.Errorf("RequestCount = %d, want 1", notifier.RequestCount())
	}
}

func TestManager_Respond_Confirm(t *testing.T) {
	storage := newMockStorage()
	manager := NewManager(storage)

	config := &Config{
		Type:  TypeConfirm,
		Title: "Confirm action",
	}

	req, _ := manager.CreateRequest(context.Background(), config, "exec-1", "step1", nil)

	// Respond with confirmation
	updated, err := manager.Respond(context.Background(), req.ID, "operator@example.com", nil, true, "Looks good")
	if err != nil {
		t.Fatalf("Respond: %v", err)
	}

	if updated.State != StateCompleted {
		t.Errorf("State = %q, want %q", updated.State, StateCompleted)
	}
	if updated.Response == nil {
		t.Fatal("expected Response to be set")
	}
	if updated.Response.Operator != "operator@example.com" {
		t.Errorf("Operator = %q, want %q", updated.Response.Operator, "operator@example.com")
	}
	if !updated.Response.Confirmed {
		t.Error("expected Confirmed = true")
	}
	if updated.Response.Comment != "Looks good" {
		t.Errorf("Comment = %q, want %q", updated.Response.Comment, "Looks good")
	}
	if updated.CompletedAt == nil {
		t.Error("expected CompletedAt to be set")
	}
}

func TestManager_Respond_Prompt(t *testing.T) {
	storage := newMockStorage()
	manager := NewManager(storage)

	minVal := float64(1)
	maxVal := float64(100)
	config := &Config{
		Type:  TypePrompt,
		Title: "Enter values",
		Prompts: []PromptField{
			{Name: "version", Type: FieldTypeText, Required: true},
			{Name: "replicas", Type: FieldTypeNumber, Validation: &FieldValidation{Min: &minVal, Max: &maxVal}},
		},
	}

	req, _ := manager.CreateRequest(context.Background(), config, "exec-1", "step1", nil)

	values := map[string]interface{}{
		"version":  "1.0.0",
		"replicas": 5,
	}

	updated, err := manager.Respond(context.Background(), req.ID, "operator@example.com", values, false, "")
	if err != nil {
		t.Fatalf("Respond: %v", err)
	}

	if updated.State != StateCompleted {
		t.Errorf("State = %q, want %q", updated.State, StateCompleted)
	}
	if updated.Response.Values["version"] != "1.0.0" {
		t.Errorf("Values[version] = %v, want %q", updated.Response.Values["version"], "1.0.0")
	}
}

func TestManager_Respond_PromptValidation(t *testing.T) {
	storage := newMockStorage()
	manager := NewManager(storage)

	minVal := float64(1)
	maxVal := float64(10)
	minLen := float64(3)
	config := &Config{
		Type:  TypePrompt,
		Title: "Test",
		Prompts: []PromptField{
			{Name: "required_field", Type: FieldTypeText, Required: true},
			{Name: "number_field", Type: FieldTypeNumber, Validation: &FieldValidation{Min: &minVal, Max: &maxVal}},
			{Name: "text_field", Type: FieldTypeText, Validation: &FieldValidation{Min: &minLen, Pattern: "^[a-z]+$"}},
			{Name: "bool_field", Type: FieldTypeBoolean},
			{Name: "select_field", Type: FieldTypeSelect, Options: []Option{{Value: "a"}, {Value: "b"}}},
		},
	}

	req, _ := manager.CreateRequest(context.Background(), config, "exec-1", "step1", nil)

	tests := []struct {
		name    string
		values  map[string]interface{}
		wantErr bool
	}{
		{
			name:    "missing required field",
			values:  map[string]interface{}{},
			wantErr: true,
		},
		{
			name:    "number too low",
			values:  map[string]interface{}{"required_field": "x", "number_field": 0},
			wantErr: true,
		},
		{
			name:    "number too high",
			values:  map[string]interface{}{"required_field": "x", "number_field": 100},
			wantErr: true,
		},
		{
			name:    "text too short",
			values:  map[string]interface{}{"required_field": "x", "text_field": "ab"},
			wantErr: true,
		},
		{
			name:    "text pattern mismatch",
			values:  map[string]interface{}{"required_field": "x", "text_field": "ABC123"},
			wantErr: true,
		},
		{
			name:    "invalid boolean",
			values:  map[string]interface{}{"required_field": "x", "bool_field": "not-a-bool"},
			wantErr: true,
		},
		{
			name:    "invalid select option",
			values:  map[string]interface{}{"required_field": "x", "select_field": "c"},
			wantErr: true,
		},
		{
			name:    "valid values",
			values:  map[string]interface{}{"required_field": "x", "number_field": 5, "text_field": "abc", "bool_field": true, "select_field": "a"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset request state for each test
			req.State = StatePending
			req.Response = nil
			req.CompletedAt = nil
			storage.SaveRequest(context.Background(), req)

			_, err := manager.Respond(context.Background(), req.ID, "op", tt.values, false, "")
			if (err != nil) != tt.wantErr {
				t.Errorf("Respond error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestManager_Respond_MultiSelect(t *testing.T) {
	storage := newMockStorage()
	manager := NewManager(storage)

	config := &Config{
		Type:  TypePrompt,
		Title: "Test",
		Prompts: []PromptField{
			{
				Name: "regions",
				Type: FieldTypeMultiSelect,
				Options: []Option{
					{Value: "us-east-1"},
					{Value: "us-west-2"},
					{Value: "eu-west-1"},
				},
			},
		},
	}

	req, _ := manager.CreateRequest(context.Background(), config, "exec-1", "step1", nil)

	// Valid multi-select
	values := map[string]interface{}{
		"regions": []interface{}{"us-east-1", "eu-west-1"},
	}
	updated, err := manager.Respond(context.Background(), req.ID, "op", values, false, "")
	if err != nil {
		t.Fatalf("Respond: %v", err)
	}
	if updated.State != StateCompleted {
		t.Errorf("State = %q, want %q", updated.State, StateCompleted)
	}

	// Invalid multi-select value
	req.State = StatePending
	req.Response = nil
	storage.SaveRequest(context.Background(), req)

	values = map[string]interface{}{
		"regions": []interface{}{"us-east-1", "invalid-region"},
	}
	_, err = manager.Respond(context.Background(), req.ID, "op", values, false, "")
	if err == nil {
		t.Error("expected error for invalid multi-select value")
	}
}

func TestManager_Respond_NotFound(t *testing.T) {
	storage := newMockStorage()
	manager := NewManager(storage)

	_, err := manager.Respond(context.Background(), "nonexistent", "op", nil, true, "")
	if err == nil {
		t.Error("expected error for nonexistent request")
	}
}

func TestManager_Respond_NotPending(t *testing.T) {
	storage := newMockStorage()
	manager := NewManager(storage)

	config := &Config{
		Type:  TypeConfirm,
		Title: "Test",
	}
	req, _ := manager.CreateRequest(context.Background(), config, "exec-1", "step1", nil)

	// Complete it first
	manager.Respond(context.Background(), req.ID, "op1", nil, true, "")

	// Try to respond again
	_, err := manager.Respond(context.Background(), req.ID, "op2", nil, true, "")
	if err == nil {
		t.Error("expected error for non-pending request")
	}
}

func TestManager_Respond_Expired(t *testing.T) {
	storage := newMockStorage()
	manager := NewManager(storage)

	config := &Config{
		Type:    TypeConfirm,
		Title:   "Test",
		Timeout: "1ns", // Effectively already expired
	}
	req, _ := manager.CreateRequest(context.Background(), config, "exec-1", "step1", nil)

	// Wait a tiny bit to ensure expiration
	time.Sleep(time.Millisecond)

	_, err := manager.Respond(context.Background(), req.ID, "op", nil, true, "")
	if err == nil {
		t.Error("expected error for expired request")
	}

	// Verify request was marked expired
	updated, _ := storage.GetRequest(context.Background(), req.ID)
	if updated.State != StateExpired {
		t.Errorf("State = %q, want %q", updated.State, StateExpired)
	}
}

func TestManager_Cancel(t *testing.T) {
	storage := newMockStorage()
	manager := NewManager(storage)

	config := &Config{
		Type:  TypeConfirm,
		Title: "Test",
	}
	req, _ := manager.CreateRequest(context.Background(), config, "exec-1", "step1", nil)

	cancelled, err := manager.Cancel(context.Background(), req.ID, "No longer needed")
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	if cancelled.State != StateCancelled {
		t.Errorf("State = %q, want %q", cancelled.State, StateCancelled)
	}
	if cancelled.Metadata["cancel_reason"] != "No longer needed" {
		t.Errorf("cancel_reason = %v, want %q", cancelled.Metadata["cancel_reason"], "No longer needed")
	}
	if cancelled.CompletedAt == nil {
		t.Error("expected CompletedAt to be set")
	}
}

func TestManager_Cancel_NotPending(t *testing.T) {
	storage := newMockStorage()
	manager := NewManager(storage)

	config := &Config{
		Type:  TypeConfirm,
		Title: "Test",
	}
	req, _ := manager.CreateRequest(context.Background(), config, "exec-1", "step1", nil)
	manager.Respond(context.Background(), req.ID, "op", nil, true, "")

	_, err := manager.Cancel(context.Background(), req.ID, "reason")
	if err == nil {
		t.Error("expected error for non-pending request")
	}
}

func TestManager_WaitForResponse(t *testing.T) {
	storage := newMockStorage()
	manager := NewManager(storage)

	config := &Config{
		Type:  TypeConfirm,
		Title: "Test",
	}
	req, _ := manager.CreateRequest(context.Background(), config, "exec-1", "step1", nil)

	// Respond in background
	go func() {
		time.Sleep(10 * time.Millisecond)
		manager.Respond(context.Background(), req.ID, "op", nil, true, "")
	}()

	result, err := manager.WaitForResponse(context.Background(), req.ID)
	if err != nil {
		t.Fatalf("WaitForResponse: %v", err)
	}
	if result.State != StateCompleted {
		t.Errorf("State = %q, want %q", result.State, StateCompleted)
	}
}

func TestManager_WaitForResponse_AlreadyComplete(t *testing.T) {
	storage := newMockStorage()
	manager := NewManager(storage)

	config := &Config{
		Type:  TypeConfirm,
		Title: "Test",
	}
	req, _ := manager.CreateRequest(context.Background(), config, "exec-1", "step1", nil)
	manager.Respond(context.Background(), req.ID, "op", nil, true, "")

	result, err := manager.WaitForResponse(context.Background(), req.ID)
	if err != nil {
		t.Fatalf("WaitForResponse: %v", err)
	}
	if result.State != StateCompleted {
		t.Errorf("State = %q, want %q", result.State, StateCompleted)
	}
}

func TestManager_WaitForResponse_ContextCancelled(t *testing.T) {
	storage := newMockStorage()
	manager := NewManager(storage)

	config := &Config{
		Type:  TypeConfirm,
		Title: "Test",
	}
	req, _ := manager.CreateRequest(context.Background(), config, "exec-1", "step1", nil)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	_, err := manager.WaitForResponse(ctx, req.ID)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want %v", err, context.Canceled)
	}
}

func TestManager_WaitForResponse_ManagerStopped(t *testing.T) {
	storage := newMockStorage()
	manager := NewManager(storage)

	config := &Config{
		Type:  TypeConfirm,
		Title: "Test",
	}
	req, _ := manager.CreateRequest(context.Background(), config, "exec-1", "step1", nil)

	go func() {
		time.Sleep(10 * time.Millisecond)
		manager.Stop()
	}()

	_, err := manager.WaitForResponse(context.Background(), req.ID)
	if err == nil {
		t.Error("expected error when manager stopped")
	}
}

func TestManager_CheckExpired(t *testing.T) {
	storage := newMockStorage()
	manager := NewManager(storage)

	// Create an expired request
	config := &Config{
		Type:    TypeConfirm,
		Title:   "Test",
		Timeout: "1ns",
	}
	manager.CreateRequest(context.Background(), config, "exec-1", "step1", nil)

	// Create a non-expired request
	config2 := &Config{
		Type:    TypeConfirm,
		Title:   "Test 2",
		Timeout: "1h",
	}
	manager.CreateRequest(context.Background(), config2, "exec-2", "step1", nil)

	time.Sleep(time.Millisecond) // Ensure first request is expired

	expired, err := manager.CheckExpired(context.Background())
	if err != nil {
		t.Fatalf("CheckExpired: %v", err)
	}
	if expired != 1 {
		t.Errorf("expired = %d, want 1", expired)
	}
}

func TestManager_GetRequest(t *testing.T) {
	storage := newMockStorage()
	manager := NewManager(storage)

	config := &Config{
		Type:  TypeConfirm,
		Title: "Test",
	}
	created, _ := manager.CreateRequest(context.Background(), config, "exec-1", "step1", nil)

	req, err := manager.GetRequest(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetRequest: %v", err)
	}
	if req.ID != created.ID {
		t.Errorf("ID = %q, want %q", req.ID, created.ID)
	}
}

func TestManager_GetRequestByExecution(t *testing.T) {
	storage := newMockStorage()
	manager := NewManager(storage)

	config := &Config{
		Type:  TypeConfirm,
		Title: "Test",
	}
	manager.CreateRequest(context.Background(), config, "exec-1", "step1", nil)

	req, err := manager.GetRequestByExecution(context.Background(), "exec-1", "step1")
	if err != nil {
		t.Fatalf("GetRequestByExecution: %v", err)
	}
	if req == nil {
		t.Fatal("expected request to be found")
	}
	if req.ExecutionID != "exec-1" {
		t.Errorf("ExecutionID = %q, want %q", req.ExecutionID, "exec-1")
	}
}

func TestManager_ListRequests(t *testing.T) {
	storage := newMockStorage()
	manager := NewManager(storage)

	// Create multiple requests
	for i := 0; i < 3; i++ {
		config := &Config{
			Type:  TypeConfirm,
			Title: "Test",
		}
		manager.CreateRequest(context.Background(), config, "exec-1", "step"+string(rune('0'+i)), nil)
	}

	requests, err := manager.ListRequests(context.Background(), ListOptions{ExecutionID: "exec-1"})
	if err != nil {
		t.Fatalf("ListRequests: %v", err)
	}
	if len(requests) != 3 {
		t.Errorf("len(requests) = %d, want 3", len(requests))
	}
}

func TestInterventionState_IsTerminal(t *testing.T) {
	tests := []struct {
		state    State
		terminal bool
	}{
		{StatePending, false},
		{StateCompleted, true},
		{StateExpired, true},
		{StateCancelled, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			if got := tt.state.IsTerminal(); got != tt.terminal {
				t.Errorf("IsTerminal() = %v, want %v", got, tt.terminal)
			}
		})
	}
}

func TestInterventionType_IsValid(t *testing.T) {
	tests := []struct {
		typ   Type
		valid bool
	}{
		{TypePrompt, true},
		{TypeWaitManual, true},
		{TypeConfirm, true},
		{"invalid", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(string(tt.typ), func(t *testing.T) {
			if got := tt.typ.IsValid(); got != tt.valid {
				t.Errorf("IsValid() = %v, want %v", got, tt.valid)
			}
		})
	}
}

func TestFieldType_IsValid(t *testing.T) {
	tests := []struct {
		typ   FieldType
		valid bool
	}{
		{FieldTypeText, true},
		{FieldTypeNumber, true},
		{FieldTypeBoolean, true},
		{FieldTypeSelect, true},
		{FieldTypeMultiSelect, true},
		{FieldTypeTextarea, true},
		{FieldTypePassword, true},
		{"invalid", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(string(tt.typ), func(t *testing.T) {
			if got := tt.typ.IsValid(); got != tt.valid {
				t.Errorf("IsValid() = %v, want %v", got, tt.valid)
			}
		})
	}
}

func TestRequest_IsExpired(t *testing.T) {
	now := time.Now()
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	tests := []struct {
		name      string
		expiresAt *time.Time
		expired   bool
	}{
		{"nil expires", nil, false},
		{"past expires", &past, true},
		{"future expires", &future, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &Request{ExpiresAt: tt.expiresAt}
			if got := req.IsExpired(); got != tt.expired {
				t.Errorf("IsExpired() = %v, want %v", got, tt.expired)
			}
		})
	}
}

func TestConfig_GetTimeout(t *testing.T) {
	tests := []struct {
		timeout  string
		expected time.Duration
	}{
		{"", 0},
		{"1h", time.Hour},
		{"30m", 30 * time.Minute},
		{"invalid", 0},
	}

	for _, tt := range tests {
		t.Run(tt.timeout, func(t *testing.T) {
			c := &Config{Timeout: tt.timeout}
			if got := c.GetTimeout(); got != tt.expected {
				t.Errorf("GetTimeout() = %v, want %v", got, tt.expected)
			}
		})
	}
}
