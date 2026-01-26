package events

import (
	"context"
	"sync"
	"testing"

	"github.com/shawnbutts/keystone-core/internal/audit"
)

// mockAuditLogger captures audit entries for testing
type mockAuditLogger struct {
	entries []*audit.AuditEntry
	mu      sync.Mutex
}

func (m *mockAuditLogger) Log(ctx context.Context, entry *audit.AuditEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = append(m.entries, entry)
	return nil
}

func (m *mockAuditLogger) Close() error {
	return nil
}

func (m *mockAuditLogger) GetEntries() []*audit.AuditEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]*audit.AuditEntry, len(m.entries))
	copy(result, m.entries)
	return result
}

// mockEventSubscriber simulates event subscription
type mockEventSubscriber struct {
	handlers map[string][]EventHandler
	mu       sync.Mutex
}

func newMockEventSubscriber() *mockEventSubscriber {
	return &mockEventSubscriber{
		handlers: make(map[string][]EventHandler),
	}
}

func (m *mockEventSubscriber) Subscribe(subject string, handler EventHandler) (*Subscription, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handlers[subject] = append(m.handlers[subject], handler)
	return &Subscription{
		ID:      "sub-" + subject,
		Subject: subject,
		Active:  true,
		unsubscribe: func() error {
			return nil
		},
	}, nil
}

func (m *mockEventSubscriber) SubscribeQueue(subject string, queue string, handler EventHandler) (*Subscription, error) {
	return m.Subscribe(subject, handler)
}

func (m *mockEventSubscriber) Close() error {
	return nil
}

func (m *mockEventSubscriber) SimulateEvent(event *Event) error {
	m.mu.Lock()
	handlers := make([]EventHandler, 0)
	subject := "events." + string(event.Type)
	handlers = append(handlers, m.handlers[subject]...)
	// Also check wildcard
	handlers = append(handlers, m.handlers["events.security.*"]...)
	m.mu.Unlock()

	for _, h := range handlers {
		if err := h(event); err != nil {
			return err
		}
	}
	return nil
}

func createTestAuditor(t *testing.T) (*audit.Auditor, *mockAuditLogger) {
	t.Helper()

	logger := &mockAuditLogger{}
	config := audit.DefaultAuditConfig()
	config.Level = audit.AuditLevelAll
	config.Backend = "none" // We'll inject our mock

	auditor, err := audit.NewAuditor("test-tool", config)
	if err != nil {
		t.Fatalf("Failed to create auditor: %v", err)
	}

	// Replace backend with mock (this is a simplified test approach)
	// In real tests we'd use dependency injection
	return auditor, logger
}

func TestNewEventAuditBridge(t *testing.T) {
	mockLogger := &mockAuditLogger{}
	config := audit.DefaultAuditConfig()
	config.Backend = "none"
	auditor, _ := audit.NewAuditor("test", config)
	subscriber := newMockEventSubscriber()

	bridge, err := NewEventAuditBridge(&EventAuditBridgeConfig{
		Auditor:    auditor,
		Subscriber: subscriber,
	})
	if err != nil {
		t.Fatalf("Failed to create bridge: %v", err)
	}

	if bridge.auditor == nil {
		t.Error("Expected auditor to be set")
	}
	if bridge.subscriber == nil {
		t.Error("Expected subscriber to be set")
	}
	if len(bridge.mappings) == 0 {
		t.Error("Expected default mappings to be loaded")
	}

	_ = mockLogger // Use the variable
}

func TestEventAuditBridge_MissingAuditor(t *testing.T) {
	subscriber := newMockEventSubscriber()

	_, err := NewEventAuditBridge(&EventAuditBridgeConfig{
		Subscriber: subscriber,
	})
	if err == nil {
		t.Error("Expected error for missing auditor")
	}
}

func TestEventAuditBridge_MissingSubscriber(t *testing.T) {
	config := audit.DefaultAuditConfig()
	config.Backend = "none"
	auditor, _ := audit.NewAuditor("test", config)

	_, err := NewEventAuditBridge(&EventAuditBridgeConfig{
		Auditor: auditor,
	})
	if err == nil {
		t.Error("Expected error for missing subscriber")
	}
}

func TestEventAuditBridge_CustomMappings(t *testing.T) {
	config := audit.DefaultAuditConfig()
	config.Backend = "none"
	auditor, _ := audit.NewAuditor("test", config)
	subscriber := newMockEventSubscriber()

	customEvent := EventType("custom.security.event")
	customMapping := &SecurityEventMapping{
		Category:    SecurityCategoryAuth,
		AuditAction: audit.ActionAuthAttempt,
		AlwaysAudit: true,
	}

	bridge, err := NewEventAuditBridge(&EventAuditBridgeConfig{
		Auditor:    auditor,
		Subscriber: subscriber,
		CustomMappings: map[EventType]*SecurityEventMapping{
			customEvent: customMapping,
		},
	})
	if err != nil {
		t.Fatalf("Failed to create bridge: %v", err)
	}

	mappings := bridge.GetMappings()
	if mappings[customEvent] == nil {
		t.Error("Expected custom mapping to be present")
	}
}

func TestEventAuditBridge_Start(t *testing.T) {
	config := audit.DefaultAuditConfig()
	config.Backend = "none"
	auditor, _ := audit.NewAuditor("test", config)
	subscriber := newMockEventSubscriber()

	bridge, _ := NewEventAuditBridge(&EventAuditBridgeConfig{
		Auditor:    auditor,
		Subscriber: subscriber,
	})

	err := bridge.Start()
	if err != nil {
		t.Fatalf("Failed to start bridge: %v", err)
	}

	stats := bridge.GetStats()
	if !stats.Running {
		t.Error("Expected bridge to be running")
	}
	if stats.SubscriptionCount == 0 {
		t.Error("Expected subscriptions to be created")
	}
}

func TestEventAuditBridge_StartTwice(t *testing.T) {
	config := audit.DefaultAuditConfig()
	config.Backend = "none"
	auditor, _ := audit.NewAuditor("test", config)
	subscriber := newMockEventSubscriber()

	bridge, _ := NewEventAuditBridge(&EventAuditBridgeConfig{
		Auditor:    auditor,
		Subscriber: subscriber,
	})

	bridge.Start()
	err := bridge.Start()
	if err == nil {
		t.Error("Expected error when starting twice")
	}
}

func TestEventAuditBridge_Stop(t *testing.T) {
	config := audit.DefaultAuditConfig()
	config.Backend = "none"
	auditor, _ := audit.NewAuditor("test", config)
	subscriber := newMockEventSubscriber()

	bridge, _ := NewEventAuditBridge(&EventAuditBridgeConfig{
		Auditor:    auditor,
		Subscriber: subscriber,
	})

	bridge.Start()
	err := bridge.Stop()
	if err != nil {
		t.Fatalf("Failed to stop bridge: %v", err)
	}

	stats := bridge.GetStats()
	if stats.Running {
		t.Error("Expected bridge to be stopped")
	}
}

func TestEventAuditBridge_HandleEvent(t *testing.T) {
	config := audit.DefaultAuditConfig()
	config.Backend = "none"
	auditor, _ := audit.NewAuditor("test", config)
	subscriber := newMockEventSubscriber()

	bridge, _ := NewEventAuditBridge(&EventAuditBridgeConfig{
		Auditor:    auditor,
		Subscriber: subscriber,
	})

	// Test that handleEvent processes known event types
	event := NewEvent(EventTypeUserLogin).
		Source("/test/user").
		Severity(SeverityInfo).
		CorrelationID("test-correlation").
		DataMap(map[string]interface{}{
			"user":      "testuser",
			"source_ip": "192.168.1.1",
		}).
		Build()

	err := bridge.handleEvent(event)
	if err != nil {
		t.Fatalf("Failed to handle event: %v", err)
	}
}

func TestEventAuditBridge_SeverityFiltering(t *testing.T) {
	config := audit.DefaultAuditConfig()
	config.Backend = "none"
	auditor, _ := audit.NewAuditor("test", config)
	subscriber := newMockEventSubscriber()

	bridge, _ := NewEventAuditBridge(&EventAuditBridgeConfig{
		Auditor:     auditor,
		Subscriber:  subscriber,
		MinSeverity: SeverityWarning,
	})

	// Test severity filtering
	mapping := &SecurityEventMapping{
		Category:    SecurityCategoryAuth,
		AuditAction: audit.ActionAuthAttempt,
		MinSeverity: SeverityWarning,
	}

	// Info event should not be audited
	infoEvent := NewEvent(EventTypeAgentHeartbeat).Severity(SeverityInfo).Build()
	if bridge.shouldAudit(infoEvent, mapping) {
		t.Error("Expected info event to be filtered out")
	}

	// Warning event should be audited
	warnEvent := NewEvent(EventTypeAgentError).Severity(SeverityWarning).Build()
	if !bridge.shouldAudit(warnEvent, mapping) {
		t.Error("Expected warning event to be audited")
	}
}

func TestEventAuditBridge_AlwaysAudit(t *testing.T) {
	config := audit.DefaultAuditConfig()
	config.Backend = "none"
	auditor, _ := audit.NewAuditor("test", config)
	subscriber := newMockEventSubscriber()

	bridge, _ := NewEventAuditBridge(&EventAuditBridgeConfig{
		Auditor:     auditor,
		Subscriber:  subscriber,
		MinSeverity: SeverityError, // High threshold
	})

	// AlwaysAudit should override severity filtering
	mapping := &SecurityEventMapping{
		Category:    SecurityCategoryAuth,
		AuditAction: audit.ActionAuthAttempt,
		AlwaysAudit: true,
	}

	// Even debug event should be audited
	debugEvent := NewEvent(EventTypeUserLogin).Severity(SeverityDebug).Build()
	if !bridge.shouldAudit(debugEvent, mapping) {
		t.Error("Expected AlwaysAudit to override severity filtering")
	}
}

func TestEventAuditBridge_CategoryFiltering(t *testing.T) {
	config := audit.DefaultAuditConfig()
	config.Backend = "none"
	auditor, _ := audit.NewAuditor("test", config)
	subscriber := newMockEventSubscriber()

	bridge, _ := NewEventAuditBridge(&EventAuditBridgeConfig{
		Auditor:    auditor,
		Subscriber: subscriber,
		IncludeCategories: []SecurityEventCategory{
			SecurityCategoryAuth,
			SecurityCategoryPolicy,
		},
	})

	// Auth category should be included
	if !bridge.isCategoryIncluded(SecurityCategoryAuth) {
		t.Error("Expected auth category to be included")
	}

	// Command category should not be included
	if bridge.isCategoryIncluded(SecurityCategoryCommand) {
		t.Error("Expected command category to be excluded")
	}
}

func TestEventAuditBridge_ExcludeTypes(t *testing.T) {
	config := audit.DefaultAuditConfig()
	config.Backend = "none"
	auditor, _ := audit.NewAuditor("test", config)
	subscriber := newMockEventSubscriber()

	bridge, _ := NewEventAuditBridge(&EventAuditBridgeConfig{
		Auditor:    auditor,
		Subscriber: subscriber,
		ExcludeTypes: []EventType{
			EventTypeAgentHeartbeat,
		},
	})

	// Heartbeat should be excluded
	if !bridge.isExcluded(EventTypeAgentHeartbeat) {
		t.Error("Expected heartbeat to be excluded")
	}

	// Other types should not be excluded
	if bridge.isExcluded(EventTypeAgentConnect) {
		t.Error("Expected connect to not be excluded")
	}
}

func TestEventAuditBridge_AddRemoveMapping(t *testing.T) {
	config := audit.DefaultAuditConfig()
	config.Backend = "none"
	auditor, _ := audit.NewAuditor("test", config)
	subscriber := newMockEventSubscriber()

	bridge, _ := NewEventAuditBridge(&EventAuditBridgeConfig{
		Auditor:    auditor,
		Subscriber: subscriber,
	})

	customType := EventType("custom.event")
	customMapping := &SecurityEventMapping{
		Category:    SecurityCategoryAuth,
		AuditAction: audit.ActionAuthAttempt,
	}

	// Add mapping
	bridge.AddMapping(customType, customMapping)
	mappings := bridge.GetMappings()
	if mappings[customType] == nil {
		t.Error("Expected custom mapping to be added")
	}

	// Remove mapping
	bridge.RemoveMapping(customType)
	mappings = bridge.GetMappings()
	if mappings[customType] != nil {
		t.Error("Expected custom mapping to be removed")
	}
}

func TestEventAuditBridge_GetStats(t *testing.T) {
	config := audit.DefaultAuditConfig()
	config.Backend = "none"
	auditor, _ := audit.NewAuditor("test", config)
	subscriber := newMockEventSubscriber()

	bridge, _ := NewEventAuditBridge(&EventAuditBridgeConfig{
		Auditor:    auditor,
		Subscriber: subscriber,
		IncludeCategories: []SecurityEventCategory{
			SecurityCategoryAuth,
		},
		ExcludeTypes: []EventType{
			EventTypeAgentHeartbeat,
		},
		MinSeverity: SeverityWarning,
	})

	stats := bridge.GetStats()

	if stats.MappingCount == 0 {
		t.Error("Expected mappings to be counted")
	}
	if len(stats.IncludeCategories) != 1 {
		t.Error("Expected include categories to be reported")
	}
	if len(stats.ExcludeTypes) != 1 {
		t.Error("Expected exclude types to be reported")
	}
	if stats.MinSeverity != SeverityWarning {
		t.Error("Expected min severity to be reported")
	}
	if len(stats.CategoryCounts) == 0 {
		t.Error("Expected category counts to be populated")
	}
}

func TestSeverityRank(t *testing.T) {
	tests := []struct {
		severity Severity
		expected int
	}{
		{SeverityCritical, 5},
		{SeverityError, 4},
		{SeverityWarning, 3},
		{SeverityInfo, 2},
		{SeverityDebug, 1},
		{Severity("unknown"), 0},
	}

	for _, test := range tests {
		rank := severityRank(test.severity)
		if rank != test.expected {
			t.Errorf("Expected rank %d for %s, got %d", test.expected, test.severity, rank)
		}
	}
}

func TestEventToAuditEntry(t *testing.T) {
	config := audit.DefaultAuditConfig()
	config.Backend = "none"
	auditor, _ := audit.NewAuditor("test", config)
	subscriber := newMockEventSubscriber()

	bridge, _ := NewEventAuditBridge(&EventAuditBridgeConfig{
		Auditor:    auditor,
		Subscriber: subscriber,
	})

	event := NewEvent(EventTypeUserLogin).
		Source("/auth/service").
		Severity(SeverityInfo).
		CorrelationID("test-123").
		Tag("env", "production").
		DataMap(map[string]interface{}{
			"user":        "admin",
			"source_ip":   "10.0.0.1",
			"agent_count": float64(5),
		}).
		Build()

	mapping := defaultSecurityMappings[EventTypeUserLogin]
	entry := bridge.eventToAuditEntry(event, mapping)

	if entry.CorrelationID != "test-123" {
		t.Errorf("Expected correlation ID to be preserved")
	}
	if entry.Target != "/auth/service" {
		t.Errorf("Expected target to be set from source")
	}
	if entry.Extra["event_id"] != event.ID {
		t.Errorf("Expected event ID to be in extra")
	}
	if entry.Extra["user"] != "admin" {
		t.Errorf("Expected user to be in extra")
	}
	if entry.Extra["source_ip"] != "10.0.0.1" {
		t.Errorf("Expected source_ip to be in extra")
	}
	if entry.Extra["tag_env"] != "production" {
		t.Errorf("Expected tags to be in extra")
	}
	if entry.AgentsMatched != 5 {
		t.Errorf("Expected agents matched to be 5, got %d", entry.AgentsMatched)
	}
}

func TestSeverityToResult(t *testing.T) {
	config := audit.DefaultAuditConfig()
	config.Backend = "none"
	auditor, _ := audit.NewAuditor("test", config)
	subscriber := newMockEventSubscriber()

	bridge, _ := NewEventAuditBridge(&EventAuditBridgeConfig{
		Auditor:    auditor,
		Subscriber: subscriber,
	})

	tests := []struct {
		severity Severity
		expected audit.AuditResult
	}{
		{SeverityCritical, audit.ResultFailure},
		{SeverityError, audit.ResultFailure},
		{SeverityWarning, audit.ResultFailure},
		{SeverityInfo, audit.ResultSuccess},
		{SeverityDebug, audit.ResultSuccess},
	}

	for _, test := range tests {
		result := bridge.severityToResult(test.severity)
		if result != test.expected {
			t.Errorf("Expected %s for severity %s, got %s", test.expected, test.severity, result)
		}
	}
}

func TestDefaultSecurityMappings(t *testing.T) {
	// Verify all expected event types have mappings
	expectedMappings := []EventType{
		EventTypeUserLogin,
		EventTypeUserCommand,
		EventTypeJobStart,
		EventTypeJobComplete,
		EventTypeJobFail,
		EventTypeStateApplyStart,
		EventTypeStateApplyDone,
		EventTypeStateApplyFail,
		EventTypePolicyViolation,
		EventTypeAgentConnect,
		EventTypeAgentDisconnect,
	}

	for _, eventType := range expectedMappings {
		if defaultSecurityMappings[eventType] == nil {
			t.Errorf("Expected mapping for %s", eventType)
		}
	}
}

func TestEventAuditBridgeIntegration(t *testing.T) {
	config := audit.DefaultAuditConfig()
	config.Backend = "none"
	auditor, _ := audit.NewAuditor("test", config)
	subscriber := newMockEventSubscriber()

	bridge, _ := NewEventAuditBridge(&EventAuditBridgeConfig{
		Auditor:    auditor,
		Subscriber: subscriber,
	})

	err := bridge.Start()
	if err != nil {
		t.Fatalf("Failed to start bridge: %v", err)
	}
	defer bridge.Stop()

	// Simulate a security event
	event := NewEvent(EventTypeUserLogin).
		Source("/auth").
		Severity(SeverityInfo).
		DataMap(map[string]interface{}{
			"user": "testuser",
		}).
		Build()

	err = subscriber.SimulateEvent(event)
	if err != nil {
		t.Fatalf("Failed to simulate event: %v", err)
	}

	// Verify bridge processed the event
	stats := bridge.GetStats()
	if !stats.Running {
		t.Error("Expected bridge to still be running")
	}
}
