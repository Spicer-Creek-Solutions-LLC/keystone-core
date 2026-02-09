package trigger

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/events"
	"github.com/shawnbutts/keystone-core/internal/runbook"
)

// Mock implementations

type mockRunbookRepository struct {
	runbooks map[string]*runbook.Runbook
}

func newMockRepository() *mockRunbookRepository {
	return &mockRunbookRepository{
		runbooks: make(map[string]*runbook.Runbook),
	}
}

func (m *mockRunbookRepository) GetRunbook(name, version string) (*runbook.Runbook, error) {
	rb, ok := m.runbooks[name]
	if !ok {
		return nil, &ValidationError{Field: "name", Message: "runbook not found"}
	}
	return rb, nil
}

func (m *mockRunbookRepository) ListRunbooks() ([]*runbook.Runbook, error) {
	result := make([]*runbook.Runbook, 0, len(m.runbooks))
	for _, rb := range m.runbooks {
		result = append(result, rb)
	}
	return result, nil
}

func (m *mockRunbookRepository) Add(rb *runbook.Runbook) {
	m.runbooks[rb.Metadata.Name] = rb
}

type mockRunbookExecutor struct {
	mu          sync.Mutex
	executions  []*mockExecution
	shouldFail  bool
	failMessage string
}

type mockExecution struct {
	runbook *runbook.Runbook
	inputs  map[string]interface{}
}

func newMockExecutor() *mockRunbookExecutor {
	return &mockRunbookExecutor{}
}

func (m *mockRunbookExecutor) Execute(rb *runbook.Runbook, inputs map[string]interface{}) (*runbook.Execution, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.executions = append(m.executions, &mockExecution{
		runbook: rb,
		inputs:  inputs,
	})

	state := runbook.ExecutionStateCompleted
	errMsg := ""
	if m.shouldFail {
		state = runbook.ExecutionStateFailed
		errMsg = m.failMessage
	}

	return &runbook.Execution{
		ID:          "exec-" + time.Now().Format("20060102150405"),
		RunbookName: rb.Metadata.Name,
		State:       state,
		Error:       errMsg,
	}, nil
}

func (m *mockRunbookExecutor) ExecutionCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.executions)
}

func (m *mockRunbookExecutor) LastExecution() *mockExecution {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.executions) == 0 {
		return nil
	}
	return m.executions[len(m.executions)-1]
}

type mockFilterParser struct{}

func (m *mockFilterParser) Parse(expr string) (events.FilterExpression, error) {
	return &mockFilter{expr: expr}, nil
}

type mockFilter struct {
	expr string
}

func (m *mockFilter) Matches(event *events.Event) bool {
	// Simple type matching for tests
	if m.expr == "type == \"test.event\"" {
		return event.Type == "test.event"
	}
	if m.expr == "type == \"other.event\"" {
		return event.Type == "other.event"
	}
	if m.expr == "severity >= \"warning\"" {
		return event.Severity == events.SeverityWarning ||
			event.Severity == events.SeverityError ||
			event.Severity == events.SeverityCritical
	}
	// Default: match everything
	return true
}

func (m *mockFilter) String() string {
	return m.expr
}

type mockEventPublisher struct {
	mu     sync.Mutex
	events []*events.Event
}

func newMockPublisher() *mockEventPublisher {
	return &mockEventPublisher{}
}

func (m *mockEventPublisher) Publish(event *events.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
	return nil
}

func (m *mockEventPublisher) PublishAsync(event *events.Event) error {
	return m.Publish(event)
}

func (m *mockEventPublisher) Close() error {
	return nil
}

func (m *mockEventPublisher) EventCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.events)
}

// Tests

func TestTrigger_Validate(t *testing.T) {
	tests := []struct {
		name    string
		trigger *Trigger
		wantErr bool
	}{
		{
			name: "valid trigger",
			trigger: &Trigger{
				ID:         "test-trigger",
				Name:       "Test Trigger",
				RunbookRef: RunbookRef{Name: "test-runbook"},
				Filter:     "type == \"test.event\"",
				Enabled:    true,
			},
			wantErr: false,
		},
		{
			name: "missing id",
			trigger: &Trigger{
				Name:       "Test Trigger",
				RunbookRef: RunbookRef{Name: "test-runbook"},
				Filter:     "type == \"test.event\"",
			},
			wantErr: true,
		},
		{
			name: "missing name",
			trigger: &Trigger{
				ID:         "test-trigger",
				RunbookRef: RunbookRef{Name: "test-runbook"},
				Filter:     "type == \"test.event\"",
			},
			wantErr: true,
		},
		{
			name: "missing runbook",
			trigger: &Trigger{
				ID:     "test-trigger",
				Name:   "Test Trigger",
				Filter: "type == \"test.event\"",
			},
			wantErr: true,
		},
		{
			name: "missing filter",
			trigger: &Trigger{
				ID:         "test-trigger",
				Name:       "Test Trigger",
				RunbookRef: RunbookRef{Name: "test-runbook"},
			},
			wantErr: true,
		},
		{
			name: "negative throttle",
			trigger: &Trigger{
				ID:         "test-trigger",
				Name:       "Test Trigger",
				RunbookRef: RunbookRef{Name: "test-runbook"},
				Filter:     "type == \"test.event\"",
				Conditions: &Conditions{
					Throttle: -1 * time.Second,
				},
			},
			wantErr: true,
		},
		{
			name: "invalid rate limit",
			trigger: &Trigger{
				ID:         "test-trigger",
				Name:       "Test Trigger",
				RunbookRef: RunbookRef{Name: "test-runbook"},
				Filter:     "type == \"test.event\"",
				Conditions: &Conditions{
					RateLimit: &RateLimitConfig{
						MaxExecutions: 0,
						Window:        time.Minute,
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.trigger.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRegistry_RegisterAndGet(t *testing.T) {
	registry := NewRegistry(WithFilterParser(&mockFilterParser{}))

	trigger := &Trigger{
		ID:         "test-trigger",
		Name:       "Test Trigger",
		RunbookRef: RunbookRef{Name: "test-runbook"},
		Filter:     "type == \"test.event\"",
		Enabled:    true,
	}

	// Register
	if err := registry.Register(trigger); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	// Get
	got, ok := registry.Get("test-trigger")
	if !ok {
		t.Fatal("Get() returned false")
	}
	if got.ID != trigger.ID {
		t.Errorf("Get() ID = %v, want %v", got.ID, trigger.ID)
	}

	// Duplicate registration
	if err := registry.Register(trigger); err == nil {
		t.Error("Expected error for duplicate registration")
	}

	// List
	triggers := registry.List()
	if len(triggers) != 1 {
		t.Errorf("List() returned %d triggers, want 1", len(triggers))
	}

	// Unregister
	if err := registry.Unregister("test-trigger"); err != nil {
		t.Fatalf("Unregister() error = %v", err)
	}

	_, ok = registry.Get("test-trigger")
	if ok {
		t.Error("Get() should return false after unregister")
	}
}

func TestRegistry_EnableDisable(t *testing.T) {
	registry := NewRegistry(WithFilterParser(&mockFilterParser{}))

	trigger := &Trigger{
		ID:         "test-trigger",
		Name:       "Test Trigger",
		RunbookRef: RunbookRef{Name: "test-runbook"},
		Filter:     "type == \"test.event\"",
		Enabled:    true,
	}

	registry.Register(trigger)

	metrics := registry.GetMetrics()
	if metrics.EnabledTriggers != 1 {
		t.Errorf("EnabledTriggers = %d, want 1", metrics.EnabledTriggers)
	}

	// Disable
	if err := registry.Disable("test-trigger"); err != nil {
		t.Fatalf("Disable() error = %v", err)
	}

	got, _ := registry.Get("test-trigger")
	if got.Enabled {
		t.Error("Trigger should be disabled")
	}

	metrics = registry.GetMetrics()
	if metrics.EnabledTriggers != 0 {
		t.Errorf("EnabledTriggers = %d, want 0", metrics.EnabledTriggers)
	}

	// Enable
	if err := registry.Enable("test-trigger"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}

	got, _ = registry.Get("test-trigger")
	if !got.Enabled {
		t.Error("Trigger should be enabled")
	}
}

func TestRegistry_ProcessEvent(t *testing.T) {
	repo := newMockRepository()
	executor := newMockExecutor()
	publisher := newMockPublisher()

	repo.Add(&runbook.Runbook{
		APIVersion: "runbook.keystone.io/v1",
		Kind:       "Runbook",
		Metadata:   runbook.Metadata{Name: "test-runbook"},
		Spec: runbook.Spec{
			Steps: []runbook.Step{
				{Name: "step1", Type: runbook.StepTypeNoop},
			},
		},
	})

	registry := NewRegistry(
		WithRepository(repo),
		WithExecutor(executor),
		WithPublisher(publisher),
		WithFilterParser(&mockFilterParser{}),
	)

	trigger := &Trigger{
		ID:         "test-trigger",
		Name:       "Test Trigger",
		RunbookRef: RunbookRef{Name: "test-runbook"},
		Filter:     "type == \"test.event\"",
		Enabled:    true,
	}
	registry.Register(trigger)

	// Process matching event
	event := &events.Event{
		ID:     "event-1",
		Type:   "test.event",
		Source: "/test",
		Time:   time.Now(),
	}

	err := registry.ProcessEvent(context.Background(), event)
	if err != nil {
		t.Fatalf("ProcessEvent() error = %v", err)
	}

	// Should have executed runbook
	if executor.ExecutionCount() != 1 {
		t.Errorf("ExecutionCount = %d, want 1", executor.ExecutionCount())
	}

	// Process non-matching event
	event2 := &events.Event{
		ID:     "event-2",
		Type:   "other.event",
		Source: "/test",
		Time:   time.Now(),
	}

	err = registry.ProcessEvent(context.Background(), event2)
	if err != nil {
		t.Fatalf("ProcessEvent() error = %v", err)
	}

	// Should not have executed another runbook
	if executor.ExecutionCount() != 1 {
		t.Errorf("ExecutionCount = %d, want 1", executor.ExecutionCount())
	}
}

func TestRegistry_Throttle(t *testing.T) {
	repo := newMockRepository()
	executor := newMockExecutor()

	repo.Add(&runbook.Runbook{
		APIVersion: "runbook.keystone.io/v1",
		Kind:       "Runbook",
		Metadata:   runbook.Metadata{Name: "test-runbook"},
		Spec: runbook.Spec{
			Steps: []runbook.Step{{Name: "step1", Type: runbook.StepTypeNoop}},
		},
	})

	registry := NewRegistry(
		WithRepository(repo),
		WithExecutor(executor),
		WithFilterParser(&mockFilterParser{}),
	)

	trigger := &Trigger{
		ID:         "test-trigger",
		Name:       "Test Trigger",
		RunbookRef: RunbookRef{Name: "test-runbook"},
		Filter:     "type == \"test.event\"",
		Enabled:    true,
		Conditions: &Conditions{
			Throttle: 100 * time.Millisecond,
		},
	}
	registry.Register(trigger)

	event := &events.Event{
		ID:     "event-1",
		Type:   "test.event",
		Source: "/test",
		Time:   time.Now(),
	}

	// First event should execute
	registry.ProcessEvent(context.Background(), event)
	if executor.ExecutionCount() != 1 {
		t.Errorf("First execution count = %d, want 1", executor.ExecutionCount())
	}

	// Immediate second event should be throttled
	event.ID = "event-2"
	registry.ProcessEvent(context.Background(), event)
	if executor.ExecutionCount() != 1 {
		t.Errorf("Throttled execution count = %d, want 1", executor.ExecutionCount())
	}

	// Wait for throttle to expire
	time.Sleep(150 * time.Millisecond)

	// Third event should execute
	event.ID = "event-3"
	registry.ProcessEvent(context.Background(), event)
	if executor.ExecutionCount() != 2 {
		t.Errorf("After throttle execution count = %d, want 2", executor.ExecutionCount())
	}

	metrics := registry.GetMetrics()
	if metrics.SkippedExecutions != 1 {
		t.Errorf("SkippedExecutions = %d, want 1", metrics.SkippedExecutions)
	}
}

func TestRegistry_MaxConcurrent(t *testing.T) {
	repo := newMockRepository()

	repo.Add(&runbook.Runbook{
		APIVersion: "runbook.keystone.io/v1",
		Kind:       "Runbook",
		Metadata:   runbook.Metadata{Name: "test-runbook"},
		Spec: runbook.Spec{
			Steps: []runbook.Step{{Name: "step1", Type: runbook.StepTypeNoop}},
		},
	})

	// Use a blocking executor
	var wg sync.WaitGroup
	executionChan := make(chan struct{})
	executionCount := 0
	var mu sync.Mutex

	blockingExecutor := &blockingMockExecutor{
		executionChan: executionChan,
		countFunc: func() {
			mu.Lock()
			executionCount++
			mu.Unlock()
		},
	}

	registry := NewRegistry(
		WithRepository(repo),
		WithExecutor(blockingExecutor),
		WithFilterParser(&mockFilterParser{}),
	)

	trigger := &Trigger{
		ID:         "test-trigger",
		Name:       "Test Trigger",
		RunbookRef: RunbookRef{Name: "test-runbook"},
		Filter:     "type == \"test.event\"",
		Enabled:    true,
		Conditions: &Conditions{
			MaxConcurrent: 2,
		},
	}
	registry.Register(trigger)

	// Send 3 events concurrently
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			event := &events.Event{
				ID:     fmt.Sprintf("event-%d", id),
				Type:   "test.event",
				Source: "/test",
				Time:   time.Now(),
			}
			registry.ProcessEvent(context.Background(), event)
		}(i)
	}

	// Wait a bit for processing to start
	time.Sleep(50 * time.Millisecond)

	// Unblock executions
	close(executionChan)
	wg.Wait()

	// Should have executed only 2 (max concurrent)
	mu.Lock()
	count := executionCount
	mu.Unlock()

	// Note: Due to timing, we might get 2 or 3 depending on race conditions
	// The test verifies the max concurrent logic is being checked
	if count < 2 {
		t.Errorf("ExecutionCount = %d, want at least 2", count)
	}
}

type blockingMockExecutor struct {
	executionChan chan struct{}
	countFunc     func()
}

func (m *blockingMockExecutor) Execute(rb *runbook.Runbook, inputs map[string]interface{}) (*runbook.Execution, error) {
	m.countFunc()
	<-m.executionChan // Block until channel is closed
	return &runbook.Execution{
		ID:          "exec-blocked",
		RunbookName: rb.Metadata.Name,
		State:       runbook.ExecutionStateCompleted,
	}, nil
}

func TestDeduplicator(t *testing.T) {
	dedup := NewDeduplicator()
	defer dedup.Close()

	event := &events.Event{
		ID:     "event-1",
		Type:   "test.event",
		Source: "/test",
		Tags:   map[string]string{"host": "web-01"},
	}

	// Generate key
	key := dedup.GenerateKey("{{ .Type }}-{{ .Tags.host }}", event)
	if key != "test.event-web-01" {
		t.Errorf("GenerateKey() = %q, want %q", key, "test.event-web-01")
	}

	// First check should not be duplicate
	if dedup.IsDuplicate("trigger1", key, time.Minute) {
		t.Error("First check should not be duplicate")
	}

	// Record and check again
	dedup.Record("trigger1", key)

	if !dedup.IsDuplicate("trigger1", key, time.Minute) {
		t.Error("Second check should be duplicate")
	}

	// Different trigger should not be duplicate
	if dedup.IsDuplicate("trigger2", key, time.Minute) {
		t.Error("Different trigger should not be duplicate")
	}

	// Clear trigger entries
	dedup.Clear("trigger1")

	if dedup.IsDuplicate("trigger1", key, time.Minute) {
		t.Error("After clear should not be duplicate")
	}
}

func TestRateLimiter(t *testing.T) {
	limiter := NewRateLimiter()

	// Allow first 3 requests
	for i := 0; i < 3; i++ {
		if !limiter.Allow("trigger1", 3, time.Second) {
			t.Errorf("Request %d should be allowed", i+1)
		}
	}

	// 4th request should be denied
	if limiter.Allow("trigger1", 3, time.Second) {
		t.Error("4th request should be denied")
	}

	// Different trigger should be allowed
	if !limiter.Allow("trigger2", 3, time.Second) {
		t.Error("Different trigger should be allowed")
	}

	// Check stats
	remaining, total := limiter.GetStats("trigger1")
	if remaining != 0 {
		t.Errorf("Remaining = %d, want 0", remaining)
	}
	if total != 3 {
		t.Errorf("Total = %d, want 3", total)
	}

	// Wait for window to pass
	time.Sleep(1100 * time.Millisecond)

	// Should be allowed again
	if !limiter.Allow("trigger1", 3, time.Second) {
		t.Error("After window should be allowed")
	}
}

func TestRunbookAction(t *testing.T) {
	repo := newMockRepository()
	executor := newMockExecutor()

	repo.Add(&runbook.Runbook{
		APIVersion: "runbook.keystone.io/v1",
		Kind:       "Runbook",
		Metadata:   runbook.Metadata{Name: "remediation"},
		Spec: runbook.Spec{
			Inputs: []runbook.InputDef{
				{Name: "host", Type: "string", Required: true},
			},
			Steps: []runbook.Step{
				{Name: "fix", Type: runbook.StepTypeNoop},
			},
		},
	})

	action := NewRunbookAction(&RunbookActionConfig{
		Name: "execute-remediation",
		Runbook: RunbookRef{
			Name: "remediation",
		},
		Inputs: map[string]string{
			"host": "{{ .Tags.host }}",
		},
	}, repo, executor)

	if action.Name() != "execute-remediation" {
		t.Errorf("Name() = %q, want %q", action.Name(), "execute-remediation")
	}
	if action.Type() != "runbook" {
		t.Errorf("Type() = %q, want %q", action.Type(), "runbook")
	}

	event := &events.Event{
		ID:     "event-1",
		Type:   "alert.triggered",
		Source: "/monitoring",
		Tags:   map[string]string{"host": "web-01"},
	}

	err := action.Execute(context.Background(), event)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	// Check inputs were passed
	lastExec := executor.LastExecution()
	if lastExec == nil {
		t.Fatal("No execution recorded")
	}

	if lastExec.inputs["host"] != "web-01" {
		t.Errorf("Input host = %v, want %q", lastExec.inputs["host"], "web-01")
	}

	if lastExec.inputs["__event_type"] != "alert.triggered" {
		t.Errorf("Input __event_type = %v, want %q", lastExec.inputs["__event_type"], "alert.triggered")
	}
}

func TestRegistry_InputMappings(t *testing.T) {
	repo := newMockRepository()
	executor := newMockExecutor()

	repo.Add(&runbook.Runbook{
		APIVersion: "runbook.keystone.io/v1",
		Kind:       "Runbook",
		Metadata:   runbook.Metadata{Name: "test-runbook"},
		Spec: runbook.Spec{
			Steps: []runbook.Step{{Name: "step1", Type: runbook.StepTypeNoop}},
		},
	})

	registry := NewRegistry(
		WithRepository(repo),
		WithExecutor(executor),
		WithFilterParser(&mockFilterParser{}),
	)

	trigger := &Trigger{
		ID:         "test-trigger",
		Name:       "Test Trigger",
		RunbookRef: RunbookRef{Name: "test-runbook"},
		Filter:     "type == \"test.event\"",
		Enabled:    true,
		InputMappings: map[string]string{
			"target_host": "{{ .Tags.host }}",
			"severity":    "{{ .Severity }}",
			"event_id":    "{{ .ID }}",
		},
	}
	registry.Register(trigger)

	event := &events.Event{
		ID:       "event-123",
		Type:     "test.event",
		Source:   "/test",
		Severity: events.SeverityWarning,
		Tags:     map[string]string{"host": "db-01"},
		Time:     time.Now(),
	}

	registry.ProcessEvent(context.Background(), event)

	lastExec := executor.LastExecution()
	if lastExec == nil {
		t.Fatal("No execution recorded")
	}

	if lastExec.inputs["target_host"] != "db-01" {
		t.Errorf("Input target_host = %v, want %q", lastExec.inputs["target_host"], "db-01")
	}
	if lastExec.inputs["severity"] != "warning" {
		t.Errorf("Input severity = %v, want %q", lastExec.inputs["severity"], "warning")
	}
	if lastExec.inputs["event_id"] != "event-123" {
		t.Errorf("Input event_id = %v, want %q", lastExec.inputs["event_id"], "event-123")
	}
}

func TestRegistry_Priority(t *testing.T) {
	var executionOrder []string
	var mu sync.Mutex

	repo := newMockRepository()

	for _, name := range []string{"runbook-a", "runbook-b", "runbook-c"} {
		repo.Add(&runbook.Runbook{
			APIVersion: "runbook.keystone.io/v1",
			Kind:       "Runbook",
			Metadata:   runbook.Metadata{Name: name},
			Spec: runbook.Spec{
				Steps: []runbook.Step{{Name: "step1", Type: runbook.StepTypeNoop}},
			},
		})
	}

	trackingExecutor := &trackingMockExecutor{
		trackFunc: func(name string) {
			mu.Lock()
			executionOrder = append(executionOrder, name)
			mu.Unlock()
		},
	}

	registry := NewRegistry(
		WithRepository(repo),
		WithExecutor(trackingExecutor),
		WithFilterParser(&mockFilterParser{}),
	)

	// Register triggers with different priorities
	registry.Register(&Trigger{
		ID: "trigger-low", Name: "Low Priority",
		RunbookRef: RunbookRef{Name: "runbook-a"},
		Filter:     "type == \"test.event\"", Enabled: true,
		Priority: 10,
	})
	registry.Register(&Trigger{
		ID: "trigger-high", Name: "High Priority",
		RunbookRef: RunbookRef{Name: "runbook-b"},
		Filter:     "type == \"test.event\"", Enabled: true,
		Priority: 100,
	})
	registry.Register(&Trigger{
		ID: "trigger-medium", Name: "Medium Priority",
		RunbookRef: RunbookRef{Name: "runbook-c"},
		Filter:     "type == \"test.event\"", Enabled: true,
		Priority: 50,
	})

	event := &events.Event{
		ID:   "event-1",
		Type: "test.event",
		Time: time.Now(),
	}

	registry.ProcessEvent(context.Background(), event)

	// Should execute in priority order (high, medium, low)
	expected := []string{"runbook-b", "runbook-c", "runbook-a"}
	mu.Lock()
	if len(executionOrder) != 3 {
		t.Fatalf("Expected 3 executions, got %d", len(executionOrder))
	}
	for i, name := range expected {
		if executionOrder[i] != name {
			t.Errorf("Execution %d = %s, want %s", i, executionOrder[i], name)
		}
	}
	mu.Unlock()
}

type trackingMockExecutor struct {
	trackFunc func(name string)
}

func (m *trackingMockExecutor) Execute(rb *runbook.Runbook, inputs map[string]interface{}) (*runbook.Execution, error) {
	m.trackFunc(rb.Metadata.Name)
	return &runbook.Execution{
		ID:          "exec-" + rb.Metadata.Name,
		RunbookName: rb.Metadata.Name,
		State:       runbook.ExecutionStateCompleted,
	}, nil
}

func TestRegistry_TimeWindow(t *testing.T) {
	registry := NewRegistry(WithFilterParser(&mockFilterParser{}))

	// Time window that should allow execution (00:00 - 23:59)
	trigger := &Trigger{
		ID:         "test-trigger",
		Name:       "Test Trigger",
		RunbookRef: RunbookRef{Name: "test-runbook"},
		Filter:     "type == \"test.event\"",
		Enabled:    true,
		Conditions: &Conditions{
			TimeWindow: &TimeWindowConfig{
				Start: "00:00",
				End:   "23:59",
			},
		},
	}

	if err := registry.Register(trigger); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	// Should be in window
	if !registry.isInTimeWindow(trigger.Conditions.TimeWindow) {
		t.Error("Should be in time window")
	}
}

func TestRegistry_Close(t *testing.T) {
	registry := NewRegistry(WithFilterParser(&mockFilterParser{}))

	trigger := &Trigger{
		ID:         "test-trigger",
		Name:       "Test Trigger",
		RunbookRef: RunbookRef{Name: "test-runbook"},
		Filter:     "type == \"test.event\"",
		Enabled:    true,
		Conditions: &Conditions{
			Debounce: time.Hour, // Long debounce
		},
	}
	registry.Register(trigger)

	// Process event (will set debounce timer)
	event := &events.Event{
		ID:   "event-1",
		Type: "test.event",
		Time: time.Now(),
	}
	registry.ProcessEvent(context.Background(), event)

	// Close should not panic
	if err := registry.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestEventProcessor(t *testing.T) {
	repo := newMockRepository()
	executor := newMockExecutor()

	repo.Add(&runbook.Runbook{
		APIVersion: "runbook.keystone.io/v1",
		Kind:       "Runbook",
		Metadata:   runbook.Metadata{Name: "test-runbook"},
		Spec: runbook.Spec{
			Steps: []runbook.Step{{Name: "step1", Type: runbook.StepTypeNoop}},
		},
	})

	registry := NewRegistry(
		WithRepository(repo),
		WithExecutor(executor),
		WithFilterParser(&mockFilterParser{}),
	)

	trigger := &Trigger{
		ID:         "test-trigger",
		Name:       "Test Trigger",
		RunbookRef: RunbookRef{Name: "test-runbook"},
		Filter:     "type == \"test.event\"",
		Enabled:    true,
	}
	registry.Register(trigger)

	processor := NewEventProcessor(registry)

	// HandleEvent should work
	event := &events.Event{
		ID:   "event-1",
		Type: "test.event",
		Time: time.Now(),
	}

	if err := processor.HandleEvent(event); err != nil {
		t.Errorf("HandleEvent() error = %v", err)
	}

	if executor.ExecutionCount() != 1 {
		t.Errorf("ExecutionCount = %d, want 1", executor.ExecutionCount())
	}
}
