package nats

import (
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/shawnbutts/keystone-core/pkg/testing/helpers"
)

func TestMessagePriorityString(t *testing.T) {
	tests := []struct {
		priority MessagePriority
		expected string
	}{
		{PriorityLow, "low"},
		{PriorityNormal, "normal"},
		{PriorityHigh, "high"},
		{PriorityCritical, "critical"},
		{MessagePriority(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.priority.String(); got != tt.expected {
				t.Errorf("String() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestEnvelopeBuilder(t *testing.T) {
	env := NewEnvelope(MessageTypeAgentCommand, "server-1", "prod").
		CorrelationID("corr-123").
		Destination("agent-456").
		Priority(PriorityHigh).
		TTL(30*time.Second).
		Payload([]byte("test payload")).
		Header("custom", "value").
		Trace("trace-abc", "span-def").
		Build()

	if env.MessageID == "" {
		t.Error("MessageID should not be empty")
	}
	if env.Type != MessageTypeAgentCommand {
		t.Errorf("Type = %q, want %q", env.Type, MessageTypeAgentCommand)
	}
	if env.Source != "server-1" {
		t.Errorf("Source = %q, want %q", env.Source, "server-1")
	}
	if env.Cluster != "prod" {
		t.Errorf("Cluster = %q, want %q", env.Cluster, "prod")
	}
	if env.CorrelationID != "corr-123" {
		t.Errorf("CorrelationID = %q, want %q", env.CorrelationID, "corr-123")
	}
	if env.Destination != "agent-456" {
		t.Errorf("Destination = %q, want %q", env.Destination, "agent-456")
	}
	if env.Priority != PriorityHigh {
		t.Errorf("Priority = %d, want %d", env.Priority, PriorityHigh)
	}
	if env.TTL != 30*time.Second {
		t.Errorf("TTL = %v, want %v", env.TTL, 30*time.Second)
	}
	if string(env.Payload) != "test payload" {
		t.Errorf("Payload = %q, want %q", string(env.Payload), "test payload")
	}
	if env.Headers["custom"] != "value" {
		t.Errorf("Headers[custom] = %q, want %q", env.Headers["custom"], "value")
	}
	if env.TraceID != "trace-abc" {
		t.Errorf("TraceID = %q, want %q", env.TraceID, "trace-abc")
	}
	if env.SpanID != "span-def" {
		t.Errorf("SpanID = %q, want %q", env.SpanID, "span-def")
	}
	if env.Timestamp.IsZero() {
		t.Error("Timestamp should not be zero")
	}
}

func TestEnvelopeBuilderPayloadJSON(t *testing.T) {
	type testPayload struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	env := NewEnvelope(MessageTypeAgentEvent, "agent-1", "default").
		PayloadJSON(testPayload{Name: "test", Value: 42}).
		Build()

	expected := `{"name":"test","value":42}`
	if string(env.Payload) != expected {
		t.Errorf("Payload = %q, want %q", string(env.Payload), expected)
	}
}

func TestEnvelopeBuilderHeaders(t *testing.T) {
	env := NewEnvelope(MessageTypeAgentHeartbeat, "agent-1", "default").
		Headers(map[string]string{
			"key1": "value1",
			"key2": "value2",
		}).
		Header("key3", "value3").
		Build()

	if env.Headers["key1"] != "value1" {
		t.Errorf("Headers[key1] = %q, want %q", env.Headers["key1"], "value1")
	}
	if env.Headers["key2"] != "value2" {
		t.Errorf("Headers[key2] = %q, want %q", env.Headers["key2"], "value2")
	}
	if env.Headers["key3"] != "value3" {
		t.Errorf("Headers[key3] = %q, want %q", env.Headers["key3"], "value3")
	}
}

func TestEnvelopeIsExpired(t *testing.T) {
	// Not expired - no TTL
	env1 := NewEnvelope(MessageTypeAgentHeartbeat, "agent-1", "default").Build()
	if env1.IsExpired() {
		t.Error("Message with no TTL should not be expired")
	}

	// Not expired - TTL not reached
	env2 := NewEnvelope(MessageTypeAgentHeartbeat, "agent-1", "default").
		TTL(1 * time.Hour).
		Build()
	if env2.IsExpired() {
		t.Error("Message with future TTL should not be expired")
	}

	// Expired - TTL in the past
	env3 := &Envelope{
		MessageID: "test",
		Timestamp: time.Now().Add(-2 * time.Second),
		TTL:       1 * time.Second,
	}
	if !env3.IsExpired() {
		t.Error("Message with past TTL should be expired")
	}
}

func TestEnvelopeAge(t *testing.T) {
	env := NewEnvelope(MessageTypeAgentHeartbeat, "agent-1", "default").Build()

	// Should be very recent
	if env.Age() > 1*time.Second {
		t.Errorf("Age() = %v, should be less than 1 second", env.Age())
	}

	// Simulate older message
	env.Timestamp = time.Now().Add(-5 * time.Second)
	if env.Age() < 4*time.Second || env.Age() > 6*time.Second {
		t.Errorf("Age() = %v, should be around 5 seconds", env.Age())
	}
}

func TestEnvelopeRemainingTTL(t *testing.T) {
	// No TTL
	env1 := NewEnvelope(MessageTypeAgentHeartbeat, "agent-1", "default").Build()
	if env1.RemainingTTL() != 0 {
		t.Errorf("RemainingTTL() = %v, want 0 for no TTL", env1.RemainingTTL())
	}

	// TTL with remaining time
	env2 := NewEnvelope(MessageTypeAgentHeartbeat, "agent-1", "default").
		TTL(1 * time.Hour).
		Build()
	remaining := env2.RemainingTTL()
	if remaining < 59*time.Minute || remaining > 1*time.Hour {
		t.Errorf("RemainingTTL() = %v, should be around 1 hour", remaining)
	}

	// Expired TTL
	env3 := &Envelope{
		MessageID: "test",
		Timestamp: time.Now().Add(-2 * time.Second),
		TTL:       1 * time.Second,
	}
	if env3.RemainingTTL() != 0 {
		t.Errorf("RemainingTTL() = %v, want 0 for expired", env3.RemainingTTL())
	}
}

func TestEnvelopeSerializeDeserialize(t *testing.T) {
	original := NewEnvelope(MessageTypeAgentCommand, "server-1", "prod").
		CorrelationID("corr-123").
		Destination("agent-456").
		Priority(PriorityHigh).
		Payload([]byte("test payload")).
		Header("custom", "value").
		Build()

	// Serialize
	data, err := original.Serialize()
	if err != nil {
		t.Fatalf("Serialize() error: %v", err)
	}

	// Deserialize
	restored, err := Deserialize(data)
	if err != nil {
		t.Fatalf("Deserialize() error: %v", err)
	}

	// Compare
	if restored.MessageID != original.MessageID {
		t.Errorf("MessageID = %q, want %q", restored.MessageID, original.MessageID)
	}
	if restored.Type != original.Type {
		t.Errorf("Type = %q, want %q", restored.Type, original.Type)
	}
	if restored.Source != original.Source {
		t.Errorf("Source = %q, want %q", restored.Source, original.Source)
	}
	if restored.CorrelationID != original.CorrelationID {
		t.Errorf("CorrelationID = %q, want %q", restored.CorrelationID, original.CorrelationID)
	}
	if restored.Destination != original.Destination {
		t.Errorf("Destination = %q, want %q", restored.Destination, original.Destination)
	}
	if restored.Priority != original.Priority {
		t.Errorf("Priority = %d, want %d", restored.Priority, original.Priority)
	}
	if string(restored.Payload) != string(original.Payload) {
		t.Errorf("Payload = %q, want %q", string(restored.Payload), string(original.Payload))
	}
	if restored.Headers["custom"] != original.Headers["custom"] {
		t.Errorf("Headers[custom] = %q, want %q", restored.Headers["custom"], original.Headers["custom"])
	}
}

func TestEnvelopeToNATSMsg(t *testing.T) {
	env := NewEnvelope(MessageTypeAgentCommand, "server-1", "prod").
		CorrelationID("corr-123").
		Destination("agent-456").
		Priority(PriorityHigh).
		Trace("trace-id", "span-id").
		Build()

	msg, err := env.ToNATSMsg("kscore.prod.agent.agent-456.command")
	if err != nil {
		t.Fatalf("ToNATSMsg() error: %v", err)
	}

	// Check subject
	if msg.Subject != "kscore.prod.agent.agent-456.command" {
		t.Errorf("Subject = %q, want %q", msg.Subject, "kscore.prod.agent.agent-456.command")
	}

	// Check headers
	if msg.Header.Get("Kscore-Message-ID") != env.MessageID {
		t.Errorf("Header Kscore-Message-ID = %q, want %q", msg.Header.Get("Kscore-Message-ID"), env.MessageID)
	}
	if msg.Header.Get("Kscore-Message-Type") != string(MessageTypeAgentCommand) {
		t.Errorf("Header Kscore-Message-Type = %q, want %q", msg.Header.Get("Kscore-Message-Type"), MessageTypeAgentCommand)
	}
	if msg.Header.Get("Kscore-Priority") != "high" {
		t.Errorf("Header Kscore-Priority = %q, want %q", msg.Header.Get("Kscore-Priority"), "high")
	}
	if msg.Header.Get("Kscore-Cluster") != "prod" {
		t.Errorf("Header Kscore-Cluster = %q, want %q", msg.Header.Get("Kscore-Cluster"), "prod")
	}
	if msg.Header.Get("Kscore-Correlation-ID") != "corr-123" {
		t.Errorf("Header Kscore-Correlation-ID = %q, want %q", msg.Header.Get("Kscore-Correlation-ID"), "corr-123")
	}
	if msg.Header.Get("Kscore-Destination") != "agent-456" {
		t.Errorf("Header Kscore-Destination = %q, want %q", msg.Header.Get("Kscore-Destination"), "agent-456")
	}
	if msg.Header.Get("Kscore-Trace-ID") != "trace-id" {
		t.Errorf("Header Kscore-Trace-ID = %q, want %q", msg.Header.Get("Kscore-Trace-ID"), "trace-id")
	}
}

func TestFromNATSMsg(t *testing.T) {
	// Create an envelope and convert to NATS message
	original := NewEnvelope(MessageTypeAgentHeartbeat, "agent-1", "default").
		Payload([]byte("heartbeat data")).
		Build()

	data, err := original.Serialize()
	if err != nil {
		t.Fatalf("Serialize() error: %v", err)
	}

	msg := &nats.Msg{
		Subject: "kscore.default.agent.heartbeat",
		Data:    data,
	}

	// Parse it back
	restored, err := FromNATSMsg(msg)
	if err != nil {
		t.Fatalf("FromNATSMsg() error: %v", err)
	}

	if restored.MessageID != original.MessageID {
		t.Errorf("MessageID = %q, want %q", restored.MessageID, original.MessageID)
	}
	if restored.Type != original.Type {
		t.Errorf("Type = %q, want %q", restored.Type, original.Type)
	}
}

func TestDeduplicationTracker(t *testing.T) {
	tracker := NewDeduplicationTracker(1*time.Second, 100)

	// First time should not be duplicate
	if tracker.IsDuplicate("msg-1") {
		t.Error("First occurrence should not be duplicate")
	}

	// Second time should be duplicate
	if !tracker.IsDuplicate("msg-1") {
		t.Error("Second occurrence should be duplicate")
	}

	// Different message should not be duplicate
	if tracker.IsDuplicate("msg-2") {
		t.Error("Different message should not be duplicate")
	}

	// Size should be 2
	if tracker.Size() != 2 {
		t.Errorf("Size() = %d, want 2", tracker.Size())
	}

	// Clear and check
	tracker.Clear()
	if tracker.Size() != 0 {
		t.Errorf("Size() after Clear() = %d, want 0", tracker.Size())
	}

	// After clear, same message should not be duplicate
	if tracker.IsDuplicate("msg-1") {
		t.Error("After Clear(), message should not be duplicate")
	}
}

func TestDeduplicationTrackerExpiry(t *testing.T) {
	// Very short window for testing
	tracker := NewDeduplicationTracker(50*time.Millisecond, 100)

	// First time should not be duplicate
	if tracker.IsDuplicate("msg-1") {
		t.Error("First occurrence should not be duplicate")
	}

	// Immediately should be duplicate
	if !tracker.IsDuplicate("msg-1") {
		t.Error("Immediate second occurrence should be duplicate")
	}

	// Wait for expiry
	start := time.Now()
	if err := helpers.WaitForTimeout(2*time.Second, 5*time.Millisecond, func() (bool, error) {
		return time.Since(start) >= 100*time.Millisecond, nil
	}); err != nil {
		t.Fatalf("expiry wait did not elapse: %v", err)
	}

	// After expiry, should not be duplicate anymore
	if tracker.IsDuplicate("msg-1") {
		t.Error("After expiry, message should not be duplicate")
	}
}

func TestDeduplicationTrackerMaxSize(t *testing.T) {
	tracker := NewDeduplicationTracker(1*time.Hour, 5)

	// Add 10 messages
	for i := 0; i < 10; i++ {
		tracker.IsDuplicate("msg-" + string(rune('0'+i)))
	}

	// Size should be at most 5
	if tracker.Size() > 5 {
		t.Errorf("Size() = %d, should be <= 5", tracker.Size())
	}
}

func TestEnvelopeHandler(t *testing.T) {
	var receivedEnv *Envelope
	handler := NewEnvelopeHandler(func(env *Envelope, msg *nats.Msg) error {
		receivedEnv = env
		return nil
	}, 1*time.Minute)

	// Create and send a message
	env := NewEnvelope(MessageTypeAgentHeartbeat, "agent-1", "default").
		Payload([]byte("test")).
		Build()

	data, _ := env.Serialize()
	msg := &nats.Msg{
		Subject: "test",
		Data:    data,
	}

	handler.Handle(msg)

	if receivedEnv == nil {
		t.Fatal("Handler was not called")
	}
	if receivedEnv.MessageID != env.MessageID {
		t.Errorf("MessageID = %q, want %q", receivedEnv.MessageID, env.MessageID)
	}
}

func TestEnvelopeHandlerDeduplication(t *testing.T) {
	callCount := 0
	handler := NewEnvelopeHandler(func(env *Envelope, msg *nats.Msg) error {
		callCount++
		return nil
	}, 1*time.Minute)

	// Create a message
	env := NewEnvelope(MessageTypeAgentHeartbeat, "agent-1", "default").Build()
	data, _ := env.Serialize()
	msg := &nats.Msg{
		Subject: "test",
		Data:    data,
	}

	// Handle same message twice
	handler.Handle(msg)
	handler.Handle(msg)

	// Handler should only be called once (dedup)
	if callCount != 1 {
		t.Errorf("Handler called %d times, expected 1 (dedup should prevent second call)", callCount)
	}
}

func TestEnvelopeHandlerExpiredMessage(t *testing.T) {
	callCount := 0
	handler := NewEnvelopeHandler(func(env *Envelope, msg *nats.Msg) error {
		callCount++
		return nil
	}, 0) // No dedup

	// Create an expired message
	env := &Envelope{
		MessageID: "expired-msg",
		Type:      MessageTypeAgentHeartbeat,
		Source:    "agent-1",
		Cluster:   "default",
		Timestamp: time.Now().Add(-2 * time.Second),
		TTL:       1 * time.Second, // Already expired
	}
	data, _ := env.Serialize()
	msg := &nats.Msg{
		Subject: "test",
		Data:    data,
	}

	handler.Handle(msg)

	// Handler should not be called (message expired)
	if callCount != 0 {
		t.Errorf("Handler called %d times, expected 0 (expired message should be ignored)", callCount)
	}
}

func TestEnvelopeHandlerNATSHandler(t *testing.T) {
	var called bool
	handler := NewEnvelopeHandler(func(env *Envelope, msg *nats.Msg) error {
		called = true
		return nil
	}, 0)

	natsHandler := handler.NATSHandler()
	if natsHandler == nil {
		t.Fatal("NATSHandler() returned nil")
	}

	// Create a valid message
	env := NewEnvelope(MessageTypeAgentHeartbeat, "agent-1", "default").Build()
	data, _ := env.Serialize()
	msg := &nats.Msg{
		Subject: "test",
		Data:    data,
	}

	natsHandler(msg)

	if !called {
		t.Error("Handler was not called through NATSHandler")
	}
}

func TestGenerateMessageIDUniqueness(t *testing.T) {
	ids := make(map[string]bool)

	// Generate 1000 IDs and check for duplicates
	for i := 0; i < 1000; i++ {
		id := generateMessageID()
		if ids[id] {
			t.Errorf("Duplicate message ID generated: %s", id)
		}
		ids[id] = true
	}
}
