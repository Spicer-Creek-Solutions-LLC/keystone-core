package remediation

import (
	"context"
	"sync"
	"testing"
	"time"
)

// mockHandler for testing
type mockHandler struct {
	handlerType   Type
	executeFunc   func(ctx context.Context, action *Action, event *Event) (*ActionResult, error)
	validateFunc  func(action *Action) error
	executeCalls  int
	validateCalls int
	mu            sync.Mutex
}

func (h *mockHandler) Type() Type {
	return h.handlerType
}

func (h *mockHandler) Execute(ctx context.Context, action *Action, event *Event) (*ActionResult, error) {
	h.mu.Lock()
	h.executeCalls++
	h.mu.Unlock()
	if h.executeFunc != nil {
		return h.executeFunc(ctx, action, event)
	}
	return &ActionResult{
		Name:   action.Name,
		Type:   action.Type,
		Status: StatusSucceeded,
		Output: "Mock action executed",
	}, nil
}

func (h *mockHandler) Validate(action *Action) error {
	h.mu.Lock()
	h.validateCalls++
	h.mu.Unlock()
	if h.validateFunc != nil {
		return h.validateFunc(action)
	}
	return nil
}

func (h *mockHandler) getExecuteCalls() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.executeCalls
}

// mockNotifier for testing
type mockNotifier struct {
	notifications []struct {
		channels []string
		message  string
	}
	mu sync.Mutex
}

func (n *mockNotifier) Notify(ctx context.Context, channels []string, message string, event *Event) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.notifications = append(n.notifications, struct {
		channels []string
		message  string
	}{channels, message})
	return nil
}

func (n *mockNotifier) getNotificationCount() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return len(n.notifications)
}

func expireCooldown(engine *Engine, key string, cooldown time.Duration) {
	engine.cooldownMu.Lock()
	engine.cooldowns[key] = time.Now().Add(-cooldown)
	engine.cooldownMu.Unlock()
}

// mockApprovalStore for testing
type mockApprovalStore struct {
	approved bool
	approver string
}

func (s *mockApprovalStore) RequestApproval(ctx context.Context, event *Event, approvers []string) (string, error) {
	return "approval-123", nil
}

func (s *mockApprovalStore) GetApprovalStatus(ctx context.Context, requestID string) (bool, string, error) {
	return s.approved, s.approver, nil
}

func (s *mockApprovalStore) WaitForApproval(ctx context.Context, requestID string, timeout time.Duration) (bool, string, error) {
	return s.approved, s.approver, nil
}

// mockEventListener for testing
type mockEventListener struct {
	started   []*Event
	completed []*Event
	actions   []struct {
		event  *Event
		result *ActionResult
	}
	mu sync.Mutex
}

func (l *mockEventListener) OnRemediationStarted(event *Event) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.started = append(l.started, event)
}

func (l *mockEventListener) OnRemediationCompleted(event *Event) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.completed = append(l.completed, event)
}

func (l *mockEventListener) OnActionCompleted(event *Event, result *ActionResult) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.actions = append(l.actions, struct {
		event  *Event
		result *ActionResult
	}{event, result})
}

func TestNewEngine(t *testing.T) {
	engine := NewEngine()
	if engine == nil {
		t.Fatal("Expected engine to be non-nil")
	}
	if engine.handlers == nil {
		t.Error("Expected handlers map to be initialized")
	}
	if engine.policies == nil {
		t.Error("Expected policies map to be initialized")
	}
}

func TestEngine_RegisterHandler(t *testing.T) {
	engine := NewEngine()
	handler := &mockHandler{handlerType: TypeRollback}

	engine.RegisterHandler(handler)

	// Verify handler is registered
	if len(engine.handlers) != 1 {
		t.Errorf("Expected 1 handler, got %d", len(engine.handlers))
	}
}

func TestEngine_RegisterPolicy(t *testing.T) {
	engine := NewEngine()
	handler := &mockHandler{handlerType: TypeRollback}
	engine.RegisterHandler(handler)

	policy := &Policy{
		Name:    "test-policy",
		Enabled: true,
		Triggers: []Trigger{
			{Type: TriggerVerificationFailure, Condition: ConditionAny},
		},
		Actions: []Action{
			{Name: "rollback", Type: TypeRollback},
		},
		MaxAttempts:    3,
		CooldownPeriod: 5 * time.Minute,
	}

	err := engine.RegisterPolicy(policy)
	if err != nil {
		t.Fatalf("RegisterPolicy() error = %v", err)
	}

	// Verify policy is registered
	got, ok := engine.GetPolicy("test-policy")
	if !ok {
		t.Fatal("Expected to find policy")
	}
	if got.Name != policy.Name {
		t.Errorf("Policy name = %q, want %q", got.Name, policy.Name)
	}
}

func TestEngine_RegisterPolicy_Validation(t *testing.T) {
	engine := NewEngine()

	tests := []struct {
		name    string
		policy  *Policy
		wantErr bool
	}{
		{
			name:    "empty name",
			policy:  &Policy{},
			wantErr: true,
		},
		{
			name:    "no actions",
			policy:  &Policy{Name: "test"},
			wantErr: true,
		},
		{
			name: "no handler for action type",
			policy: &Policy{
				Name: "test",
				Actions: []Action{
					{Name: "rollback", Type: TypeRollback},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := engine.RegisterPolicy(tt.policy)
			if (err != nil) != tt.wantErr {
				t.Errorf("RegisterPolicy() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestEngine_Trigger(t *testing.T) {
	engine := NewEngine()
	handler := &mockHandler{handlerType: TypeRollback}
	engine.RegisterHandler(handler)

	policy := &Policy{
		Name:        "test-policy",
		Enabled:     true,
		Environment: "production",
		Triggers: []Trigger{
			{Type: TriggerVerificationFailure, Condition: ConditionAny},
		},
		Actions: []Action{
			{Name: "rollback", Type: TypeRollback},
		},
		MaxAttempts:    3,
		CooldownPeriod: 1 * time.Second,
	}
	_ = engine.RegisterPolicy(policy)

	req := &TriggerRequest{
		TriggerType: TriggerVerificationFailure,
		Environment: "production",
		Application: "myapp",
		TriggeredBy: "system",
	}

	events, err := engine.Trigger(context.Background(), req)
	if err != nil {
		t.Fatalf("Trigger() error = %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}

	event := events[0]
	if event.Status != StatusSucceeded {
		t.Errorf("Status = %q, want %q", event.Status, StatusSucceeded)
	}
	if event.PolicyName != "test-policy" {
		t.Errorf("PolicyName = %q, want %q", event.PolicyName, "test-policy")
	}
	if len(event.Actions) != 1 {
		t.Errorf("Actions count = %d, want 1", len(event.Actions))
	}
	if handler.getExecuteCalls() != 1 {
		t.Errorf("Handler execute calls = %d, want 1", handler.getExecuteCalls())
	}
}

func TestEngine_Trigger_NoMatchingPolicy(t *testing.T) {
	engine := NewEngine()
	handler := &mockHandler{handlerType: TypeRollback}
	engine.RegisterHandler(handler)

	policy := &Policy{
		Name:        "test-policy",
		Enabled:     true,
		Environment: "production",
		Triggers: []Trigger{
			{Type: TriggerVerificationFailure, Condition: ConditionAny},
		},
		Actions: []Action{
			{Name: "rollback", Type: TypeRollback},
		},
	}
	_ = engine.RegisterPolicy(policy)

	// Trigger for different environment
	req := &TriggerRequest{
		TriggerType: TriggerVerificationFailure,
		Environment: "staging",
		Application: "myapp",
		TriggeredBy: "system",
	}

	events, err := engine.Trigger(context.Background(), req)
	if err != nil {
		t.Fatalf("Trigger() error = %v", err)
	}

	if len(events) != 0 {
		t.Errorf("Expected 0 events, got %d", len(events))
	}
}

func TestEngine_Cooldown(t *testing.T) {
	engine := NewEngine()
	handler := &mockHandler{handlerType: TypeRollback}
	engine.RegisterHandler(handler)

	policy := &Policy{
		Name:    "test-policy",
		Enabled: true,
		Triggers: []Trigger{
			{Type: TriggerVerificationFailure, Condition: ConditionAny},
		},
		Actions: []Action{
			{Name: "rollback", Type: TypeRollback},
		},
		CooldownPeriod: 1 * time.Hour, // Long cooldown
	}
	_ = engine.RegisterPolicy(policy)

	req := &TriggerRequest{
		TriggerType: TriggerVerificationFailure,
		Environment: "production",
		Application: "myapp",
		TriggeredBy: "system",
	}

	// First trigger should succeed
	events1, _ := engine.Trigger(context.Background(), req)
	if events1[0].Status != StatusSucceeded {
		t.Errorf("First trigger status = %q, want %q", events1[0].Status, StatusSucceeded)
	}

	// Second trigger should be in cooldown
	events2, _ := engine.Trigger(context.Background(), req)
	if events2[0].Status != StatusCooldown {
		t.Errorf("Second trigger status = %q, want %q", events2[0].Status, StatusCooldown)
	}

	// Force should bypass cooldown
	req.Force = true
	events3, _ := engine.Trigger(context.Background(), req)
	if events3[0].Status != StatusSucceeded {
		t.Errorf("Forced trigger status = %q, want %q", events3[0].Status, StatusSucceeded)
	}
}

func TestEngine_MaxAttempts(t *testing.T) {
	engine := NewEngine()
	handler := &mockHandler{
		handlerType: TypeRollback,
		executeFunc: func(ctx context.Context, action *Action, event *Event) (*ActionResult, error) {
			return &ActionResult{
				Name:   action.Name,
				Type:   action.Type,
				Status: StatusFailed,
				Error:  "Simulated failure",
			}, nil
		},
	}
	engine.RegisterHandler(handler)

	notifier := &mockNotifier{}
	engine.SetNotifier(notifier)

	policy := &Policy{
		Name:    "test-policy",
		Enabled: true,
		Triggers: []Trigger{
			{Type: TriggerVerificationFailure, Condition: ConditionAny},
		},
		Actions: []Action{
			{Name: "rollback", Type: TypeRollback},
		},
		MaxAttempts:    2,
		CooldownPeriod: 1 * time.Millisecond, // Short cooldown for test
		EscalationPolicy: &EscalationPolicy{
			Type:     EscalationPage,
			Channels: []string{"oncall"},
		},
	}
	_ = engine.RegisterPolicy(policy)

	req := &TriggerRequest{
		TriggerType: TriggerVerificationFailure,
		Environment: "production",
		Application: "myapp",
		TriggeredBy: "system",
	}
	key := "production:myapp"

	// First two attempts fail
	engine.Trigger(context.Background(), req)
	expireCooldown(engine, key, policy.CooldownPeriod)
	engine.Trigger(context.Background(), req)
	expireCooldown(engine, key, policy.CooldownPeriod)

	// Third attempt should be max attempts reached
	events, _ := engine.Trigger(context.Background(), req)
	if events[0].Status != StatusMaxAttemptsReached {
		t.Errorf("Status = %q, want %q", events[0].Status, StatusMaxAttemptsReached)
	}

	// Should have notified
	if notifier.getNotificationCount() != 1 {
		t.Errorf("Notification count = %d, want 1", notifier.getNotificationCount())
	}
}

func TestEngine_DryRun(t *testing.T) {
	engine := NewEngine()
	handler := &mockHandler{handlerType: TypeRollback}
	engine.RegisterHandler(handler)

	policy := &Policy{
		Name:    "test-policy",
		Enabled: true,
		Triggers: []Trigger{
			{Type: TriggerVerificationFailure, Condition: ConditionAny},
		},
		Actions: []Action{
			{Name: "rollback", Type: TypeRollback},
		},
		DryRun: true,
	}
	_ = engine.RegisterPolicy(policy)

	req := &TriggerRequest{
		TriggerType: TriggerVerificationFailure,
		Environment: "production",
		Application: "myapp",
		TriggeredBy: "system",
	}

	events, _ := engine.Trigger(context.Background(), req)

	if events[0].Status != StatusSucceeded {
		t.Errorf("Status = %q, want %q", events[0].Status, StatusSucceeded)
	}

	// Handler should not have been called
	if handler.getExecuteCalls() != 0 {
		t.Errorf("Handler execute calls = %d, want 0 (dry run)", handler.getExecuteCalls())
	}

	// Action output should indicate dry run
	if len(events[0].Actions) == 0 || events[0].Actions[0].Output == "" {
		t.Error("Expected dry run output in action")
	}
}

func TestEngine_EventListener(t *testing.T) {
	engine := NewEngine()
	handler := &mockHandler{handlerType: TypeRollback}
	engine.RegisterHandler(handler)

	listener := &mockEventListener{}
	engine.AddEventListener(listener)

	policy := &Policy{
		Name:    "test-policy",
		Enabled: true,
		Triggers: []Trigger{
			{Type: TriggerVerificationFailure, Condition: ConditionAny},
		},
		Actions: []Action{
			{Name: "rollback", Type: TypeRollback},
		},
	}
	_ = engine.RegisterPolicy(policy)

	req := &TriggerRequest{
		TriggerType: TriggerVerificationFailure,
		Environment: "production",
		Application: "myapp",
		TriggeredBy: "system",
	}

	engine.Trigger(context.Background(), req)

	listener.mu.Lock()
	defer listener.mu.Unlock()

	if len(listener.started) != 1 {
		t.Errorf("Started events = %d, want 1", len(listener.started))
	}
	if len(listener.completed) != 1 {
		t.Errorf("Completed events = %d, want 1", len(listener.completed))
	}
	if len(listener.actions) != 1 {
		t.Errorf("Action completions = %d, want 1", len(listener.actions))
	}
}

func TestEngine_ApprovalRequired(t *testing.T) {
	engine := NewEngine()
	handler := &mockHandler{handlerType: TypeRollback}
	engine.RegisterHandler(handler)

	// Approval denied
	approvalStore := &mockApprovalStore{approved: false}
	engine.SetApprovalStore(approvalStore)

	policy := &Policy{
		Name:    "test-policy",
		Enabled: true,
		Triggers: []Trigger{
			{Type: TriggerVerificationFailure, Condition: ConditionAny},
		},
		Actions: []Action{
			{Name: "rollback", Type: TypeRollback},
		},
		RequireApproval: true,
		Approvers:       []string{"admin"},
	}
	_ = engine.RegisterPolicy(policy)

	req := &TriggerRequest{
		TriggerType: TriggerVerificationFailure,
		Environment: "production",
		Application: "myapp",
		TriggeredBy: "system",
	}

	events, _ := engine.Trigger(context.Background(), req)

	if events[0].Status != StatusSkipped {
		t.Errorf("Status = %q, want %q (approval denied)", events[0].Status, StatusSkipped)
	}
}

func TestEngine_ApprovalGranted(t *testing.T) {
	engine := NewEngine()
	handler := &mockHandler{handlerType: TypeRollback}
	engine.RegisterHandler(handler)

	// Approval granted
	approvalStore := &mockApprovalStore{approved: true, approver: "admin"}
	engine.SetApprovalStore(approvalStore)

	policy := &Policy{
		Name:    "test-policy",
		Enabled: true,
		Triggers: []Trigger{
			{Type: TriggerVerificationFailure, Condition: ConditionAny},
		},
		Actions: []Action{
			{Name: "rollback", Type: TypeRollback},
		},
		RequireApproval: true,
		Approvers:       []string{"admin"},
	}
	_ = engine.RegisterPolicy(policy)

	req := &TriggerRequest{
		TriggerType: TriggerVerificationFailure,
		Environment: "production",
		Application: "myapp",
		TriggeredBy: "system",
	}

	events, _ := engine.Trigger(context.Background(), req)

	if events[0].Status != StatusSucceeded {
		t.Errorf("Status = %q, want %q", events[0].Status, StatusSucceeded)
	}
	if events[0].ApprovedBy != "admin" {
		t.Errorf("ApprovedBy = %q, want %q", events[0].ApprovedBy, "admin")
	}
}

func TestEngine_MultipleActions(t *testing.T) {
	engine := NewEngine()

	rollbackHandler := &mockHandler{handlerType: TypeRollback}
	alertHandler := &mockHandler{handlerType: TypeAlert}
	engine.RegisterHandler(rollbackHandler)
	engine.RegisterHandler(alertHandler)

	policy := &Policy{
		Name:    "test-policy",
		Enabled: true,
		Triggers: []Trigger{
			{Type: TriggerVerificationFailure, Condition: ConditionAny},
		},
		Actions: []Action{
			{Name: "alert", Type: TypeAlert, Priority: 100},
			{Name: "rollback", Type: TypeRollback, Priority: 50},
		},
	}
	_ = engine.RegisterPolicy(policy)

	req := &TriggerRequest{
		TriggerType: TriggerVerificationFailure,
		Environment: "production",
		Application: "myapp",
		TriggeredBy: "system",
	}

	events, _ := engine.Trigger(context.Background(), req)

	if len(events[0].Actions) != 2 {
		t.Errorf("Actions count = %d, want 2", len(events[0].Actions))
	}
	if rollbackHandler.getExecuteCalls() != 1 {
		t.Errorf("Rollback handler calls = %d, want 1", rollbackHandler.getExecuteCalls())
	}
	if alertHandler.getExecuteCalls() != 1 {
		t.Errorf("Alert handler calls = %d, want 1", alertHandler.getExecuteCalls())
	}
}

func TestEngine_ContinueOnFailure(t *testing.T) {
	engine := NewEngine()

	failingHandler := &mockHandler{
		handlerType: TypeAlert,
		executeFunc: func(ctx context.Context, action *Action, event *Event) (*ActionResult, error) {
			return &ActionResult{
				Name:   action.Name,
				Type:   action.Type,
				Status: StatusFailed,
				Error:  "Alert failed",
			}, nil
		},
	}
	rollbackHandler := &mockHandler{handlerType: TypeRollback}
	engine.RegisterHandler(failingHandler)
	engine.RegisterHandler(rollbackHandler)

	policy := &Policy{
		Name:    "test-policy",
		Enabled: true,
		Triggers: []Trigger{
			{Type: TriggerVerificationFailure, Condition: ConditionAny},
		},
		Actions: []Action{
			{Name: "alert", Type: TypeAlert, ContinueOnFailure: true},
			{Name: "rollback", Type: TypeRollback},
		},
	}
	_ = engine.RegisterPolicy(policy)

	req := &TriggerRequest{
		TriggerType: TriggerVerificationFailure,
		Environment: "production",
		Application: "myapp",
		TriggeredBy: "system",
	}

	events, _ := engine.Trigger(context.Background(), req)

	// Should have executed both actions
	if len(events[0].Actions) != 2 {
		t.Errorf("Actions count = %d, want 2", len(events[0].Actions))
	}

	// Overall status should be failed (one action failed)
	if events[0].Status != StatusFailed {
		t.Errorf("Status = %q, want %q", events[0].Status, StatusFailed)
	}

	// But rollback should have been executed
	if rollbackHandler.getExecuteCalls() != 1 {
		t.Errorf("Rollback handler calls = %d, want 1", rollbackHandler.getExecuteCalls())
	}
}

func TestEngine_StopOnFailure(t *testing.T) {
	engine := NewEngine()

	failingHandler := &mockHandler{
		handlerType: TypeAlert,
		executeFunc: func(ctx context.Context, action *Action, event *Event) (*ActionResult, error) {
			return &ActionResult{
				Name:   action.Name,
				Type:   action.Type,
				Status: StatusFailed,
				Error:  "Alert failed",
			}, nil
		},
	}
	rollbackHandler := &mockHandler{handlerType: TypeRollback}
	engine.RegisterHandler(failingHandler)
	engine.RegisterHandler(rollbackHandler)

	policy := &Policy{
		Name:    "test-policy",
		Enabled: true,
		Triggers: []Trigger{
			{Type: TriggerVerificationFailure, Condition: ConditionAny},
		},
		Actions: []Action{
			{Name: "alert", Type: TypeAlert, ContinueOnFailure: false},
			{Name: "rollback", Type: TypeRollback},
		},
	}
	_ = engine.RegisterPolicy(policy)

	req := &TriggerRequest{
		TriggerType: TriggerVerificationFailure,
		Environment: "production",
		Application: "myapp",
		TriggeredBy: "system",
	}

	events, _ := engine.Trigger(context.Background(), req)

	// Should have only executed first action
	if len(events[0].Actions) != 1 {
		t.Errorf("Actions count = %d, want 1 (should stop on failure)", len(events[0].Actions))
	}

	// Rollback should not have been executed
	if rollbackHandler.getExecuteCalls() != 0 {
		t.Errorf("Rollback handler calls = %d, want 0", rollbackHandler.getExecuteCalls())
	}
}

func TestEngine_GetEvents(t *testing.T) {
	engine := NewEngine()
	handler := &mockHandler{handlerType: TypeRollback}
	engine.RegisterHandler(handler)

	policy := &Policy{
		Name:    "test-policy",
		Enabled: true,
		Triggers: []Trigger{
			{Type: TriggerVerificationFailure, Condition: ConditionAny},
		},
		Actions: []Action{
			{Name: "rollback", Type: TypeRollback},
		},
		CooldownPeriod: 1 * time.Millisecond,
	}
	_ = engine.RegisterPolicy(policy)

	// Trigger a few events
	for i := 0; i < 3; i++ {
		req := &TriggerRequest{
			TriggerType: TriggerVerificationFailure,
			Environment: "production",
			Application: "myapp",
			TriggeredBy: "system",
		}
		engine.Trigger(context.Background(), req)
		expireCooldown(engine, "production:myapp", policy.CooldownPeriod)
	}

	// Get all events
	events := engine.GetEvents("", "", 0)
	if len(events) != 3 {
		t.Errorf("Events count = %d, want 3", len(events))
	}

	// Get with limit
	events = engine.GetEvents("", "", 2)
	if len(events) != 2 {
		t.Errorf("Events with limit = %d, want 2", len(events))
	}

	// Get by environment
	events = engine.GetEvents("production", "", 0)
	if len(events) != 3 {
		t.Errorf("Events for production = %d, want 3", len(events))
	}

	// Get by different environment
	events = engine.GetEvents("staging", "", 0)
	if len(events) != 0 {
		t.Errorf("Events for staging = %d, want 0", len(events))
	}
}

func TestEngine_ResetAttempts(t *testing.T) {
	engine := NewEngine()
	handler := &mockHandler{
		handlerType: TypeRollback,
		executeFunc: func(ctx context.Context, action *Action, event *Event) (*ActionResult, error) {
			return &ActionResult{
				Name:   action.Name,
				Type:   action.Type,
				Status: StatusFailed,
				Error:  "Simulated failure",
			}, nil
		},
	}
	engine.RegisterHandler(handler)

	policy := &Policy{
		Name:    "test-policy",
		Enabled: true,
		Triggers: []Trigger{
			{Type: TriggerVerificationFailure, Condition: ConditionAny},
		},
		Actions: []Action{
			{Name: "rollback", Type: TypeRollback},
		},
		MaxAttempts:    2,
		CooldownPeriod: 1 * time.Millisecond,
	}
	_ = engine.RegisterPolicy(policy)

	req := &TriggerRequest{
		TriggerType: TriggerVerificationFailure,
		Environment: "production",
		Application: "myapp",
		TriggeredBy: "system",
	}
	key := "production:myapp"

	// Exhaust attempts
	engine.Trigger(context.Background(), req)
	expireCooldown(engine, key, policy.CooldownPeriod)
	engine.Trigger(context.Background(), req)

	count := engine.GetAttemptCount("production", "myapp")
	if count != 2 {
		t.Errorf("Attempt count = %d, want 2", count)
	}

	// Reset attempts
	engine.ResetAttempts("production", "myapp")

	count = engine.GetAttemptCount("production", "myapp")
	if count != 0 {
		t.Errorf("Attempt count after reset = %d, want 0", count)
	}
}

func TestGetDefaultPolicy(t *testing.T) {
	tests := []struct {
		name       string
		policyName string
		wantOK     bool
	}{
		{"verification failure rollback", "verification-failure-rollback", true},
		{"health check restart", "health-check-restart", true},
		{"deployment failure alert", "deployment-failure-alert", true},
		{"unknown policy", "unknown", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy, ok := GetDefaultPolicy(tt.policyName)
			if ok != tt.wantOK {
				t.Errorf("GetDefaultPolicy() ok = %v, want %v", ok, tt.wantOK)
			}
			if tt.wantOK && policy == nil {
				t.Error("Expected policy to be non-nil")
			}
		})
	}
}

func TestTypes(t *testing.T) {
	types := []Type{
		TypeRollback,
		TypeRestart,
		TypeScale,
		TypeRunbook,
		TypeScript,
		TypeWebhook,
		TypeAlert,
		TypeCustom,
	}

	for _, rt := range types {
		if rt == "" {
			t.Error("Remediation type should not be empty")
		}
	}
}

func TestTriggerTypes(t *testing.T) {
	types := []TriggerType{
		TriggerVerificationFailure,
		TriggerMetricThreshold,
		TriggerHealthCheckFailure,
		TriggerDeploymentFailure,
		TriggerAlertFiring,
		TriggerManual,
	}

	for _, tt := range types {
		if tt == "" {
			t.Error("Trigger type should not be empty")
		}
	}
}
