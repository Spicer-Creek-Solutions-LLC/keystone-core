package policy

import (
	"context"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/events"
)

// mockEventPublisher implements a simple mock publisher for testing
type mockEventPublisher struct {
	published []*events.Event
}

func (m *mockEventPublisher) Publish(event *events.Event) error {
	m.published = append(m.published, event)
	return nil
}

func (m *mockEventPublisher) PublishAsync(event *events.Event) error {
	m.published = append(m.published, event)
	return nil
}

func (m *mockEventPublisher) Close() error {
	return nil
}

func TestPolicyEnforcerEnforceForResource(t *testing.T) {
	registry := NewRegistry()
	engine := NewPolicyEngine(registry)

	// Register policy
	policy := &Policy{
		ID:              "test-policy",
		Name:            "Test Policy",
		Type:            PolicyTypeCEL,
		Enabled:         true,
		Policy:          `action == "allowed"`,
		EnforcementMode: ModeEnforce,
	}
	registry.RegisterPolicy(policy)

	// Register binding
	binding := &PolicyBinding{
		ID:           "binding1",
		PolicyID:     "test-policy",
		ResourceType: "pod",
		Enabled:      true,
	}
	registry.RegisterBinding(binding)

	config := &EnforcementConfig{
		Point:          EnforcementPointPreExecution,
		Action:         EnforcementActionBlock,
		ResourceTypes:  []string{"pod"},
		EnableEventEmission: false,
	}

	enforcer := NewPolicyEnforcer(engine, config)

	tests := []struct {
		name         string
		resourceType string
		input        *EvaluationInput
		allowed      bool
	}{
		{
			name:         "allowed action",
			resourceType: "pod",
			input: &EvaluationInput{
				Action: "allowed",
			},
			allowed: true,
		},
		{
			name:         "denied action",
			resourceType: "pod",
			input: &EvaluationInput{
				Action: "denied",
			},
			allowed: false,
		},
		{
			name:         "out of scope resource",
			resourceType: "service",
			input: &EvaluationInput{
				Action: "denied",
			},
			allowed: true, // Not in enforcement scope
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			result, err := enforcer.EnforceForResource(ctx, tt.resourceType, tt.input)
			if err != nil {
				t.Fatalf("EnforceForResource failed: %v", err)
			}

			if result.Allowed != tt.allowed {
				t.Errorf("Allowed = %v, want %v", result.Allowed, tt.allowed)
			}
		})
	}
}

func TestPolicyEnforcerEnforcementActions(t *testing.T) {
	registry := NewRegistry()
	engine := NewPolicyEngine(registry)

	// Register policy that always denies
	policy := &Policy{
		ID:              "deny-all",
		Name:            "Deny All",
		Type:            PolicyTypeCEL,
		Enabled:         true,
		Policy:          `false`, // Always deny
		EnforcementMode: ModeEnforce,
	}
	registry.RegisterPolicy(policy)

	binding := &PolicyBinding{
		ID:           "binding1",
		PolicyID:     "deny-all",
		ResourceType: "pod",
		Enabled:      true,
	}
	registry.RegisterBinding(binding)

	tests := []struct {
		name            string
		action          EnforcementAction
		expectedAllowed bool
	}{
		{
			name:            "block action",
			action:          EnforcementActionBlock,
			expectedAllowed: false,
		},
		{
			name:            "warn action",
			action:          EnforcementActionWarn,
			expectedAllowed: true,
		},
		{
			name:            "audit action",
			action:          EnforcementActionAudit,
			expectedAllowed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &EnforcementConfig{
				Point:               EnforcementPointPreExecution,
				Action:              tt.action,
				ResourceTypes:       []string{"pod"},
				EnableEventEmission: false,
			}

			enforcer := NewPolicyEnforcer(engine, config)

			ctx := context.Background()
			input := &EvaluationInput{
				Action: "test",
			}

			result, err := enforcer.EnforceForResource(ctx, "pod", input)
			if err != nil {
				t.Fatalf("EnforceForResource failed: %v", err)
			}

			if result.Allowed != tt.expectedAllowed {
				t.Errorf("Allowed = %v, want %v for action %s", result.Allowed, tt.expectedAllowed, tt.action)
			}
		})
	}
}

func TestPolicyEnforcerEventEmission(t *testing.T) {
	registry := NewRegistry()
	engine := NewPolicyEngine(registry)

	// Register policy
	policy := &Policy{
		ID:              "event-test",
		Name:            "Event Test Policy",
		Type:            PolicyTypeCEL,
		Enabled:         true,
		Policy:          `action == "allowed"`,
		Severity:        SeverityHigh,
		EnforcementMode: ModeEnforce,
	}
	registry.RegisterPolicy(policy)

	binding := &PolicyBinding{
		ID:           "binding1",
		PolicyID:     "event-test",
		ResourceType: "pod",
		Enabled:      true,
	}
	registry.RegisterBinding(binding)

	mockPublisher := &mockEventPublisher{
		published: make([]*events.Event, 0),
	}

	config := &EnforcementConfig{
		Point:               EnforcementPointPreExecution,
		Action:              EnforcementActionBlock,
		ResourceTypes:       []string{"pod"},
		EnableEventEmission: true,
		EventPublisher:      mockPublisher,
	}

	enforcer := NewPolicyEnforcer(engine, config)

	tests := []struct {
		name          string
		input         *EvaluationInput
		expectedEvent events.EventType
	}{
		{
			name: "violation event",
			input: &EvaluationInput{
				Action: "denied",
			},
			expectedEvent: events.EventTypePolicyViolation,
		},
		{
			name: "pass event",
			input: &EvaluationInput{
				Action: "allowed",
			},
			expectedEvent: events.EventTypePolicyPass,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockPublisher.published = make([]*events.Event, 0)

			ctx := context.Background()
			_, err := enforcer.EnforceForResource(ctx, "pod", tt.input)
			if err != nil {
				t.Fatalf("EnforceForResource failed: %v", err)
			}

			if len(mockPublisher.published) != 1 {
				t.Fatalf("Expected 1 event, got %d", len(mockPublisher.published))
			}

			event := mockPublisher.published[0]
			if event.Type != tt.expectedEvent {
				t.Errorf("Event type = %s, want %s", event.Type, tt.expectedEvent)
			}

			if event.Source != "policy-enforcer" {
				t.Errorf("Event source = %s, want policy-enforcer", event.Source)
			}
		})
	}
}

func TestPolicyEnforcerViolationHandler(t *testing.T) {
	registry := NewRegistry()
	engine := NewPolicyEngine(registry)

	// Register policy that denies
	policy := &Policy{
		ID:              "violation-test",
		Name:            "Violation Test",
		Type:            PolicyTypeCEL,
		Enabled:         true,
		Policy:          `false`, // Always deny
		EnforcementMode: ModeEnforce,
	}
	registry.RegisterPolicy(policy)

	binding := &PolicyBinding{
		ID:           "binding1",
		PolicyID:     "violation-test",
		ResourceType: "pod",
		Enabled:      true,
	}
	registry.RegisterBinding(binding)

	config := &EnforcementConfig{
		Point:               EnforcementPointPreExecution,
		Action:              EnforcementActionBlock,
		ResourceTypes:       []string{"pod"},
		EnableEventEmission: false,
	}

	enforcer := NewPolicyEnforcer(engine, config)

	// Add custom violation handler
	handlerCalled := false
	enforcer.AddViolationHandler(func(ctx context.Context, result *PolicyResult) error {
		handlerCalled = true
		if result.Summary.TotalViolations == 0 {
			t.Error("Expected violations in handler")
		}
		return nil
	})

	ctx := context.Background()
	input := &EvaluationInput{
		Action: "test",
	}

	_, err := enforcer.EnforceForResource(ctx, "pod", input)
	if err != nil {
		t.Fatalf("EnforceForResource failed: %v", err)
	}

	if !handlerCalled {
		t.Error("Violation handler was not called")
	}
}

func TestPolicyEnforcerEnforcePolicy(t *testing.T) {
	registry := NewRegistry()
	engine := NewPolicyEngine(registry)

	// Register policy
	policy := &Policy{
		ID:              "single-policy",
		Name:            "Single Policy",
		Type:            PolicyTypeCEL,
		Enabled:         true,
		Policy:          `user == "admin"`,
		EnforcementMode: ModeEnforce,
	}
	registry.RegisterPolicy(policy)

	config := &EnforcementConfig{
		Point:               EnforcementPointPreExecution,
		Action:              EnforcementActionBlock,
		EnableEventEmission: false,
	}

	enforcer := NewPolicyEnforcer(engine, config)

	tests := []struct {
		name    string
		input   *EvaluationInput
		allowed bool
	}{
		{
			name: "allowed user",
			input: &EvaluationInput{
				User: "admin",
			},
			allowed: true,
		},
		{
			name: "denied user",
			input: &EvaluationInput{
				User: "guest",
			},
			allowed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			result, err := enforcer.EnforcePolicy(ctx, "single-policy", tt.input)
			if err != nil {
				t.Fatalf("EnforcePolicy failed: %v", err)
			}

			if result.Allowed != tt.allowed {
				t.Errorf("Allowed = %v, want %v", result.Allowed, tt.allowed)
			}
		})
	}
}

func TestPolicyEnforcerStateHook(t *testing.T) {
	registry := NewRegistry()
	engine := NewPolicyEngine(registry)

	// Register policy
	policy := &Policy{
		ID:              "state-hook-policy",
		Name:            "State Hook Policy",
		Type:            PolicyTypeCEL,
		Enabled:         true,
		Policy:          `true`, // Always allow
		EnforcementMode: ModeEnforce,
	}
	registry.RegisterPolicy(policy)

	binding := &PolicyBinding{
		ID:           "binding1",
		PolicyID:     "state-hook-policy",
		ResourceType: "deployment",
		Enabled:      true,
	}
	registry.RegisterBinding(binding)

	config := &EnforcementConfig{
		Point:               EnforcementPointPreExecution,
		Action:              EnforcementActionBlock,
		ResourceTypes:       []string{"deployment"},
		EnableEventEmission: false,
	}

	enforcer := NewPolicyEnforcer(engine, config)

	// Create state hook
	hook := enforcer.StateEnforcementHook("deployment")

	ctx := context.Background()
	resource := map[string]interface{}{
		"name":     "test-deployment",
		"replicas": 3,
	}

	err := hook(ctx, resource)
	if err != nil {
		t.Errorf("State hook failed: %v", err)
	}
}

func TestPolicyEnforcerStateHookDenial(t *testing.T) {
	registry := NewRegistry()
	engine := NewPolicyEngine(registry)

	// Register policy that denies
	policy := &Policy{
		ID:              "deny-hook-policy",
		Name:            "Deny Hook Policy",
		Type:            PolicyTypeCEL,
		Enabled:         true,
		Policy:          `false`, // Always deny
		EnforcementMode: ModeEnforce,
	}
	registry.RegisterPolicy(policy)

	binding := &PolicyBinding{
		ID:           "binding1",
		PolicyID:     "deny-hook-policy",
		ResourceType: "deployment",
		Enabled:      true,
	}
	registry.RegisterBinding(binding)

	config := &EnforcementConfig{
		Point:               EnforcementPointPreExecution,
		Action:              EnforcementActionBlock,
		ResourceTypes:       []string{"deployment"},
		EnableEventEmission: false,
	}

	enforcer := NewPolicyEnforcer(engine, config)

	// Create state hook
	hook := enforcer.StateEnforcementHook("deployment")

	ctx := context.Background()
	resource := map[string]interface{}{
		"name": "test-deployment",
	}

	err := hook(ctx, resource)
	if err == nil {
		t.Error("Expected state hook to fail on policy violation")
	}
}

func TestPolicyEnforcerEventReactor(t *testing.T) {
	registry := NewRegistry()
	engine := NewPolicyEngine(registry)

	// Register policy
	policy := &Policy{
		ID:              "reactor-policy",
		Name:            "Reactor Policy",
		Type:            PolicyTypeCEL,
		Enabled:         true,
		Policy:          `true`, // Always allow
		EnforcementMode: ModeEnforce,
	}
	registry.RegisterPolicy(policy)

	binding := &PolicyBinding{
		ID:           "binding1",
		PolicyID:     "reactor-policy",
		ResourceType: "pod",
		Enabled:      true,
	}
	registry.RegisterBinding(binding)

	config := &EnforcementConfig{
		Point:               EnforcementPointOnEvent,
		Action:              EnforcementActionBlock,
		ResourceTypes:       []string{"pod"},
		EnableEventEmission: false,
	}

	enforcer := NewPolicyEnforcer(engine, config)

	// Create event reactor
	reactor := enforcer.EventEnforcementReactor("pod")

	if reactor.ID != "policy-enforcer-pod" {
		t.Errorf("Reactor ID = %s, want policy-enforcer-pod", reactor.ID)
	}

	if !reactor.Enabled {
		t.Error("Reactor should be enabled")
	}

	if reactor.Timeout != 30*time.Second {
		t.Errorf("Reactor timeout = %v, want 30s", reactor.Timeout)
	}

	// Test reactor action
	ctx := context.Background()
	event := events.NewEvent(events.EventTypeStateChange).
		Source("test").
		DataMap(map[string]interface{}{
			"resource": "pod",
		}).
		Build()

	if len(reactor.Actions) == 0 {
		t.Fatal("Reactor has no actions")
	}

	err := reactor.Actions[0].Execute(ctx, event)
	if err != nil {
		t.Errorf("Reactor action failed: %v", err)
	}
}

func TestPolicyEnforcerSeverityConversion(t *testing.T) {
	config := &EnforcementConfig{
		Point:               EnforcementPointPreExecution,
		Action:              EnforcementActionBlock,
		EnableEventEmission: false,
	}

	enforcer := NewPolicyEnforcer(nil, config)

	tests := []struct {
		name             string
		severityMap      map[Severity]int
		expectedSeverity events.Severity
	}{
		{
			name: "critical violations",
			severityMap: map[Severity]int{
				SeverityCritical: 1,
				SeverityHigh:     2,
			},
			expectedSeverity: events.SeverityCritical,
		},
		{
			name: "high violations",
			severityMap: map[Severity]int{
				SeverityHigh:   1,
				SeverityMedium: 2,
			},
			expectedSeverity: events.SeverityError,
		},
		{
			name: "medium violations",
			severityMap: map[Severity]int{
				SeverityMedium: 1,
				SeverityLow:    2,
			},
			expectedSeverity: events.SeverityWarning,
		},
		{
			name: "low violations",
			severityMap: map[Severity]int{
				SeverityLow: 1,
			},
			expectedSeverity: events.SeverityInfo,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			severity := enforcer.severityToEventSeverity(tt.severityMap)
			if severity != tt.expectedSeverity {
				t.Errorf("Severity = %s, want %s", severity, tt.expectedSeverity)
			}
		})
	}
}
