package schedule

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestNewExecutor(t *testing.T) {
	store := NewMockStore()
	managerConfig := &ManagerConfig{MemberID: "member-1"}
	manager, _ := NewManager(managerConfig, store)

	tests := []struct {
		name    string
		config  *ExecutorConfig
		store   Store
		manager *Manager
		wantErr bool
	}{
		{
			name:    "valid with config",
			config:  &ExecutorConfig{MemberID: "member-1"},
			store:   store,
			manager: manager,
			wantErr: false,
		},
		{
			name:    "nil store",
			config:  &ExecutorConfig{MemberID: "member-1"},
			store:   nil,
			manager: manager,
			wantErr: true,
		},
		{
			name:    "nil manager",
			config:  &ExecutorConfig{MemberID: "member-1"},
			store:   store,
			manager: nil,
			wantErr: true,
		},
		{
			name:    "missing member ID",
			config:  &ExecutorConfig{},
			store:   store,
			manager: manager,
			wantErr: true,
		},
		{
			name:    "default config with member ID",
			config:  nil,
			store:   store,
			manager: manager,
			wantErr: true, // member ID required even with default config
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewExecutor(tt.config, tt.store, tt.manager, nil)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewExecutor() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestExecutor_RegisterHandler(t *testing.T) {
	store := NewMockStore()
	managerConfig := &ManagerConfig{MemberID: "member-1"}
	manager, _ := NewManager(managerConfig, store)
	config := &ExecutorConfig{MemberID: "member-1"}

	executor, err := NewExecutor(config, store, manager, nil)
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	// Register handler
	handler := &mockHandler{scheduleType: TypeCommand}
	if err := executor.RegisterHandler(handler); err != nil {
		t.Errorf("RegisterHandler() error = %v", err)
	}

	// Register duplicate
	if err := executor.RegisterHandler(handler); err == nil {
		t.Error("RegisterHandler() should fail for duplicate")
	}

	// Register nil
	if err := executor.RegisterHandler(nil); err == nil {
		t.Error("RegisterHandler() should fail for nil")
	}

	// Unregister
	executor.UnregisterHandler(TypeCommand)

	// Register again should work
	if err := executor.RegisterHandler(handler); err != nil {
		t.Errorf("RegisterHandler() after unregister error = %v", err)
	}
}

func TestExecutor_StartStop(t *testing.T) {
	store := NewMockStore()
	managerConfig := &ManagerConfig{MemberID: "member-1"}
	manager, _ := NewManager(managerConfig, store)
	config := &ExecutorConfig{
		MemberID:                 "member-1",
		CheckInterval:            100 * time.Millisecond,
		CleanupInterval:          100 * time.Millisecond,
		MaintenanceCheckInterval: 100 * time.Millisecond,
	}

	executor, err := NewExecutor(config, store, manager, nil)
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	ctx := context.Background()

	// Start
	if err := executor.Start(ctx); err != nil {
		t.Errorf("Start() error = %v", err)
	}

	// Start again should fail
	if err := executor.Start(ctx); err == nil {
		t.Error("Start() twice should fail")
	}

	// Stop
	stopCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if err := executor.Stop(stopCtx); err != nil {
		t.Errorf("Stop() error = %v", err)
	}
}

func TestExecutor_ExecuteNow(t *testing.T) {
	store := NewMockStore()
	managerConfig := &ManagerConfig{MemberID: "member-1"}
	manager, _ := NewManager(managerConfig, store)
	config := &ExecutorConfig{MemberID: "member-1"}

	executor, err := NewExecutor(config, store, manager, nil)
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	ctx := context.Background()

	// Create schedule
	schedule := &Schedule{
		Name: "test-schedule",
		Type: TypeCommand,
		Cron: "0 2 * * *",
		Target: &Target{
			All: true,
		},
	}
	if err := manager.Create(ctx, schedule); err != nil {
		t.Fatalf("Failed to create schedule: %v", err)
	}

	// Execute now
	exec, err := executor.ExecuteNow(ctx, schedule.ID, "admin")
	if err != nil {
		t.Fatalf("ExecuteNow() error = %v", err)
	}

	if exec.TriggerType != TriggerTypeManual {
		t.Errorf("TriggerType = %v, want manual", exec.TriggerType)
	}
}

func TestExecutor_GetActiveExecutions(t *testing.T) {
	store := NewMockStore()
	managerConfig := &ManagerConfig{MemberID: "member-1"}
	manager, _ := NewManager(managerConfig, store)
	config := &ExecutorConfig{MemberID: "member-1"}

	executor, err := NewExecutor(config, store, manager, nil)
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	// Initially empty
	active := executor.GetActiveExecutions()
	if len(active) != 0 {
		t.Errorf("GetActiveExecutions() returned %d, want 0", len(active))
	}
}

func TestExecutor_AddListener(t *testing.T) {
	store := NewMockStore()
	managerConfig := &ManagerConfig{MemberID: "member-1"}
	manager, _ := NewManager(managerConfig, store)
	config := &ExecutorConfig{MemberID: "member-1"}

	executor, err := NewExecutor(config, store, manager, nil)
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	var events []*ExecutorEvent
	executor.AddListener(func(event *ExecutorEvent) {
		events = append(events, event)
	})

	// Emit event directly for testing
	executor.emitEvent(&ExecutorEvent{
		Type:        "test.event",
		ScheduleID:  "schedule-1",
		ExecutionID: "exec-1",
		Timestamp:   time.Now().UTC(),
		Message:     "test message",
	})

	if len(events) != 1 {
		t.Errorf("Expected 1 event, got %d", len(events))
	}
	if events[0].Type != "test.event" {
		t.Errorf("Event type = %v, want test.event", events[0].Type)
	}
}

func TestExecutor_CalculateRetryDelay(t *testing.T) {
	store := NewMockStore()
	managerConfig := &ManagerConfig{MemberID: "member-1"}
	manager, _ := NewManager(managerConfig, store)
	config := &ExecutorConfig{MemberID: "member-1"}

	executor, err := NewExecutor(config, store, manager, nil)
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	tests := []struct {
		name    string
		policy  *RetryPolicy
		attempt int
		wantMin time.Duration
		wantMax time.Duration
	}{
		{
			name:    "nil policy",
			policy:  nil,
			attempt: 1,
			wantMin: time.Minute,
			wantMax: time.Minute,
		},
		{
			name: "first retry",
			policy: &RetryPolicy{
				RetryDelay:        time.Minute,
				BackoffMultiplier: 2.0,
				MaxDelay:          time.Hour,
			},
			attempt: 0,
			wantMin: time.Minute,
			wantMax: time.Minute,
		},
		{
			name: "second retry with backoff",
			policy: &RetryPolicy{
				RetryDelay:        time.Minute,
				BackoffMultiplier: 2.0,
				MaxDelay:          time.Hour,
			},
			attempt: 1,
			wantMin: 2 * time.Minute,
			wantMax: 2 * time.Minute,
		},
		{
			name: "third retry with backoff",
			policy: &RetryPolicy{
				RetryDelay:        time.Minute,
				BackoffMultiplier: 2.0,
				MaxDelay:          time.Hour,
			},
			attempt: 2,
			wantMin: 4 * time.Minute,
			wantMax: 4 * time.Minute,
		},
		{
			name: "max delay cap",
			policy: &RetryPolicy{
				RetryDelay:        time.Minute,
				BackoffMultiplier: 10.0,
				MaxDelay:          5 * time.Minute,
			},
			attempt: 3,
			wantMin: 5 * time.Minute,
			wantMax: 5 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			delay := executor.calculateRetryDelay(tt.policy, tt.attempt)
			if delay < tt.wantMin || delay > tt.wantMax {
				t.Errorf("calculateRetryDelay() = %v, want between %v and %v", delay, tt.wantMin, tt.wantMax)
			}
		})
	}
}

// mockHandler implements Handler for testing
type mockHandler struct {
	scheduleType Type
	executeErr   error
	validateErr  error
	executeCalls int
}

func (h *mockHandler) Type() Type {
	return h.scheduleType
}

func (h *mockHandler) Execute(ctx context.Context, schedule *Schedule, execution *Execution) error {
	h.executeCalls++
	if h.executeErr != nil {
		return h.executeErr
	}
	execution.SuccessCount = 1
	execution.TargetCount = 1
	return nil
}

func (h *mockHandler) Validate(schedule *Schedule) error {
	return h.validateErr
}

func TestCommandHandler_Validate(t *testing.T) {
	handler := &CommandHandler{}

	tests := []struct {
		name    string
		payload json.RawMessage
		wantErr bool
	}{
		{
			name:    "valid payload",
			payload: json.RawMessage(`{"command": "echo hello"}`),
			wantErr: false,
		},
		{
			name:    "nil payload",
			payload: nil,
			wantErr: true,
		},
		{
			name:    "invalid JSON",
			payload: json.RawMessage(`invalid`),
			wantErr: true,
		},
		{
			name:    "missing command",
			payload: json.RawMessage(`{}`),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schedule := &Schedule{Payload: tt.payload}
			err := handler.Validate(schedule)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCommandHandler_Execute(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		executeFunc func(ctx context.Context, target *Target, payload *CommandPayload) (map[string]*AgentExecutionResult, error)
		payload     json.RawMessage
		wantErr     bool
	}{
		{
			name: "successful execution",
			executeFunc: func(ctx context.Context, target *Target, payload *CommandPayload) (map[string]*AgentExecutionResult, error) {
				return map[string]*AgentExecutionResult{
					"agent-1": {Status: ExecutionStatusCompleted},
					"agent-2": {Status: ExecutionStatusCompleted},
				}, nil
			},
			payload: json.RawMessage(`{"command": "echo hello"}`),
			wantErr: false,
		},
		{
			name: "partial failure",
			executeFunc: func(ctx context.Context, target *Target, payload *CommandPayload) (map[string]*AgentExecutionResult, error) {
				return map[string]*AgentExecutionResult{
					"agent-1": {Status: ExecutionStatusCompleted},
					"agent-2": {Status: ExecutionStatusFailed},
				}, nil
			},
			payload: json.RawMessage(`{"command": "echo hello"}`),
			wantErr: true, // Returns error when failures exist
		},
		{
			name:        "nil execute func",
			executeFunc: nil,
			payload:     json.RawMessage(`{"command": "echo hello"}`),
			wantErr:     true,
		},
		{
			name: "execute error",
			executeFunc: func(ctx context.Context, target *Target, payload *CommandPayload) (map[string]*AgentExecutionResult, error) {
				return nil, errors.New("execute failed")
			},
			payload: json.RawMessage(`{"command": "echo hello"}`),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &CommandHandler{ExecuteFunc: tt.executeFunc}
			schedule := &Schedule{
				Payload: tt.payload,
				Target:  &Target{All: true},
			}
			execution := &Execution{}
			err := handler.Execute(ctx, schedule, execution)
			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestStateHandler_Validate(t *testing.T) {
	handler := &StateHandler{}

	tests := []struct {
		name    string
		payload json.RawMessage
		wantErr bool
	}{
		{
			name:    "valid with path",
			payload: json.RawMessage(`{"state_path": "/path/to/state"}`),
			wantErr: false,
		},
		{
			name:    "valid with content",
			payload: json.RawMessage(`{"state_content": "pkg.installed: nginx"}`),
			wantErr: false,
		},
		{
			name:    "nil payload",
			payload: nil,
			wantErr: true,
		},
		{
			name:    "invalid JSON",
			payload: json.RawMessage(`invalid`),
			wantErr: true,
		},
		{
			name:    "missing path and content",
			payload: json.RawMessage(`{}`),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schedule := &Schedule{Payload: tt.payload}
			err := handler.Validate(schedule)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestStateHandler_Execute(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name      string
		applyFunc func(ctx context.Context, target *Target, payload *StatePayload) (map[string]*AgentExecutionResult, error)
		payload   json.RawMessage
		wantErr   bool
	}{
		{
			name: "successful execution",
			applyFunc: func(ctx context.Context, target *Target, payload *StatePayload) (map[string]*AgentExecutionResult, error) {
				return map[string]*AgentExecutionResult{
					"agent-1": {Status: ExecutionStatusCompleted},
				}, nil
			},
			payload: json.RawMessage(`{"state_path": "/path/to/state"}`),
			wantErr: false,
		},
		{
			name:      "nil apply func",
			applyFunc: nil,
			payload:   json.RawMessage(`{"state_path": "/path/to/state"}`),
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &StateHandler{ApplyFunc: tt.applyFunc}
			schedule := &Schedule{
				Payload: tt.payload,
				Target:  &Target{All: true},
			}
			execution := &Execution{}
			err := handler.Execute(ctx, schedule, execution)
			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestBlueprintHandler_Validate(t *testing.T) {
	handler := &BlueprintHandler{}

	tests := []struct {
		name    string
		payload json.RawMessage
		wantErr bool
	}{
		{
			name:    "valid payload",
			payload: json.RawMessage(`{"blueprint_name": "webserver"}`),
			wantErr: false,
		},
		{
			name:    "nil payload",
			payload: nil,
			wantErr: true,
		},
		{
			name:    "invalid JSON",
			payload: json.RawMessage(`invalid`),
			wantErr: true,
		},
		{
			name:    "missing blueprint name",
			payload: json.RawMessage(`{}`),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schedule := &Schedule{Payload: tt.payload}
			err := handler.Validate(schedule)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestBlueprintHandler_Execute(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name      string
		applyFunc func(ctx context.Context, target *Target, payload *BlueprintPayload) (map[string]*AgentExecutionResult, error)
		payload   json.RawMessage
		wantErr   bool
	}{
		{
			name: "successful execution",
			applyFunc: func(ctx context.Context, target *Target, payload *BlueprintPayload) (map[string]*AgentExecutionResult, error) {
				return map[string]*AgentExecutionResult{
					"agent-1": {Status: ExecutionStatusCompleted},
				}, nil
			},
			payload: json.RawMessage(`{"blueprint_name": "webserver"}`),
			wantErr: false,
		},
		{
			name:      "nil apply func",
			applyFunc: nil,
			payload:   json.RawMessage(`{"blueprint_name": "webserver"}`),
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &BlueprintHandler{ApplyFunc: tt.applyFunc}
			schedule := &Schedule{
				Payload: tt.payload,
				Target:  &Target{All: true},
			}
			execution := &Execution{}
			err := handler.Execute(ctx, schedule, execution)
			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDefaultExecutorConfig(t *testing.T) {
	config := DefaultExecutorConfig()

	if config.CheckInterval != time.Minute {
		t.Errorf("CheckInterval = %v, want 1m", config.CheckInterval)
	}
	if config.LockTimeout != 10*time.Second {
		t.Errorf("LockTimeout = %v, want 10s", config.LockTimeout)
	}
	if config.MaxConcurrentExecutions != 10 {
		t.Errorf("MaxConcurrentExecutions = %v, want 10", config.MaxConcurrentExecutions)
	}
	if config.ExecutionTimeout != time.Hour {
		t.Errorf("ExecutionTimeout = %v, want 1h", config.ExecutionTimeout)
	}
	if config.CleanupInterval != time.Hour {
		t.Errorf("CleanupInterval = %v, want 1h", config.CleanupInterval)
	}
	if config.MaintenanceCheckInterval != time.Minute {
		t.Errorf("MaintenanceCheckInterval = %v, want 1m", config.MaintenanceCheckInterval)
	}
}
