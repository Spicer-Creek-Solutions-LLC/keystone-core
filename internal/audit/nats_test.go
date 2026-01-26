package audit

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestDefaultNATSAuditConfig(t *testing.T) {
	config := DefaultNATSAuditConfig()

	if config.URL != "nats://localhost:4222" {
		t.Errorf("expected URL nats://localhost:4222, got %s", config.URL)
	}

	if config.Subject != "kscore.audit" {
		t.Errorf("expected subject kscore.audit, got %s", config.Subject)
	}

	if config.BufferSize != 1000 {
		t.Errorf("expected buffer size 1000, got %d", config.BufferSize)
	}

	if config.FlushInterval != 1*time.Second {
		t.Errorf("expected flush interval 1s, got %v", config.FlushInterval)
	}

	if config.ConnectTimeout != 5*time.Second {
		t.Errorf("expected connect timeout 5s, got %v", config.ConnectTimeout)
	}

	if config.MaxReconnects != -1 {
		t.Errorf("expected max reconnects -1, got %d", config.MaxReconnects)
	}

	if config.SubjectPerAction {
		t.Error("expected SubjectPerAction to be false")
	}

	if config.SubjectPerTool {
		t.Error("expected SubjectPerTool to be false")
	}
}

func TestNATSAuditMessage(t *testing.T) {
	msg := NATSAuditMessage{
		Timestamp:     "2024-01-15T10:30:00.123456789Z",
		AuditType:     "command_executed",
		User:          "testuser",
		UID:           1000,
		TTY:           "/dev/pts/0",
		PID:           12345,
		Tool:          "kscore-exec",
		Command:       "run",
		Args:          []string{"--target", "web-*", "uptime"},
		Target:        "web-*",
		AgentsMatched: 3,
		Result:        "success",
		ExitCode:      0,
		DurationMS:    150,
		CorrelationID: "abc123",
		Service:       "test-service",
		Hostname:      "host1",
	}

	// Test JSON marshaling
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded NATSAuditMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.AuditType != msg.AuditType {
		t.Errorf("expected audit type %s, got %s", msg.AuditType, decoded.AuditType)
	}

	if decoded.User != msg.User {
		t.Errorf("expected user %s, got %s", msg.User, decoded.User)
	}

	if decoded.AgentsMatched != msg.AgentsMatched {
		t.Errorf("expected agents matched %d, got %d", msg.AgentsMatched, decoded.AgentsMatched)
	}

	if len(decoded.Args) != len(msg.Args) {
		t.Errorf("expected %d args, got %d", len(msg.Args), len(decoded.Args))
	}
}

func TestNATSAuditMessageWithError(t *testing.T) {
	msg := NATSAuditMessage{
		Timestamp:     "2024-01-15T10:30:00Z",
		AuditType:     "command_executed",
		User:          "testuser",
		Result:        "failure",
		ExitCode:      1,
		Error:         "connection refused",
		CorrelationID: "xyz789",
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded NATSAuditMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.Error != "connection refused" {
		t.Errorf("expected error 'connection refused', got %s", decoded.Error)
	}

	if decoded.Result != "failure" {
		t.Errorf("expected result failure, got %s", decoded.Result)
	}
}

func TestNATSAuditMessageWithExtra(t *testing.T) {
	msg := NATSAuditMessage{
		Timestamp: "2024-01-15T10:30:00Z",
		AuditType: "state_applied",
		User:      "testuser",
		Result:    "success",
		Extra: map[string]interface{}{
			"states_total":    10,
			"states_changed":  3,
			"states_failed":   0,
			"execution_order": []string{"pkg.install", "file.managed", "service.running"},
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded NATSAuditMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.Extra == nil {
		t.Fatal("expected extra to be non-nil")
	}

	if decoded.Extra["states_total"].(float64) != 10 {
		t.Errorf("expected states_total 10, got %v", decoded.Extra["states_total"])
	}
}

func TestNATSAuditLoggerBuildSubject(t *testing.T) {
	tests := []struct {
		name             string
		config           *NATSAuditConfig
		entry            *AuditEntry
		expectedSubject  string
	}{
		{
			name: "base subject only",
			config: &NATSAuditConfig{
				Subject:          "kscore.audit",
				SubjectPerAction: false,
				SubjectPerTool:   false,
			},
			entry: &AuditEntry{
				Tool:      "kscore-exec",
				AuditType: ActionCommandExecuted,
			},
			expectedSubject: "kscore.audit",
		},
		{
			name: "with tool",
			config: &NATSAuditConfig{
				Subject:          "kscore.audit",
				SubjectPerAction: false,
				SubjectPerTool:   true,
			},
			entry: &AuditEntry{
				Tool:      "kscore-exec",
				AuditType: ActionCommandExecuted,
			},
			expectedSubject: "kscore.audit.kscore-exec",
		},
		{
			name: "with action",
			config: &NATSAuditConfig{
				Subject:          "kscore.audit",
				SubjectPerAction: true,
				SubjectPerTool:   false,
			},
			entry: &AuditEntry{
				Tool:      "kscore-exec",
				AuditType: ActionCommandExecuted,
			},
			expectedSubject: "kscore.audit.command_executed",
		},
		{
			name: "with tool and action",
			config: &NATSAuditConfig{
				Subject:          "kscore.audit",
				SubjectPerAction: true,
				SubjectPerTool:   true,
			},
			entry: &AuditEntry{
				Tool:      "kscore-exec",
				AuditType: ActionStateApplied,
			},
			expectedSubject: "kscore.audit.kscore-exec.state_applied",
		},
		{
			name: "empty tool with SubjectPerTool",
			config: &NATSAuditConfig{
				Subject:          "kscore.audit",
				SubjectPerAction: false,
				SubjectPerTool:   true,
			},
			entry: &AuditEntry{
				Tool:      "",
				AuditType: ActionCommandExecuted,
			},
			expectedSubject: "kscore.audit",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := &NATSAuditLogger{
				config: tt.config,
			}

			subject := logger.buildSubject(tt.entry)
			if subject != tt.expectedSubject {
				t.Errorf("expected subject %s, got %s", tt.expectedSubject, subject)
			}
		})
	}
}

func TestNATSAuditLoggerEntryToMessage(t *testing.T) {
	config := &NATSAuditConfig{
		ServiceName: "test-service",
	}

	logger := &NATSAuditLogger{
		config:   config,
		hostname: "test-host",
	}

	entry := &AuditEntry{
		Timestamp:     time.Date(2024, 1, 15, 10, 30, 0, 123456789, time.UTC),
		AuditType:     ActionCommandExecuted,
		User:          "testuser",
		UID:           1000,
		TTY:           "/dev/pts/0",
		PID:           12345,
		Tool:          "kscore-exec",
		Command:       "run",
		Args:          []string{"--target", "web-*"},
		Target:        "web-*",
		AgentsMatched: 5,
		Result:        ResultSuccess,
		ExitCode:      0,
		DurationMS:    200,
		CorrelationID: "corr123",
		RemoteAddr:    "192.168.1.100",
		Extra: map[string]interface{}{
			"custom": "value",
		},
	}

	msg := logger.entryToMessage(entry)

	if msg.AuditType != "command_executed" {
		t.Errorf("expected audit type command_executed, got %s", msg.AuditType)
	}

	if msg.User != "testuser" {
		t.Errorf("expected user testuser, got %s", msg.User)
	}

	if msg.Service != "test-service" {
		t.Errorf("expected service test-service, got %s", msg.Service)
	}

	if msg.Hostname != "test-host" {
		t.Errorf("expected hostname test-host, got %s", msg.Hostname)
	}

	if msg.AgentsMatched != 5 {
		t.Errorf("expected agents matched 5, got %d", msg.AgentsMatched)
	}

	if msg.RemoteAddr != "192.168.1.100" {
		t.Errorf("expected remote addr 192.168.1.100, got %s", msg.RemoteAddr)
	}

	if msg.Extra["custom"] != "value" {
		t.Errorf("expected extra custom=value, got %v", msg.Extra["custom"])
	}
}

func TestNATSAuditLoggerStats(t *testing.T) {
	logger := &NATSAuditLogger{
		entriesPublished: 100,
		entriesDropped:   5,
		lastError:        nil,
	}

	published, dropped, lastErr, _ := logger.Stats()

	if published != 100 {
		t.Errorf("expected published 100, got %d", published)
	}

	if dropped != 5 {
		t.Errorf("expected dropped 5, got %d", dropped)
	}

	if lastErr != nil {
		t.Errorf("expected no error, got %v", lastErr)
	}
}

func TestNATSAuditLoggerIsConnectedNil(t *testing.T) {
	logger := &NATSAuditLogger{
		conn: nil,
	}

	if logger.IsConnected() {
		t.Error("expected IsConnected to return false for nil connection")
	}
}

func TestNATSAuditLoggerLogClosed(t *testing.T) {
	logger := &NATSAuditLogger{
		closed:  true,
		entries: make(chan *AuditEntry, 10),
	}

	entry := &AuditEntry{
		AuditType: ActionCommandExecuted,
	}

	err := logger.Log(context.Background(), entry)
	if err == nil {
		t.Error("expected error when logging to closed logger")
	}
}

func TestNATSAuditLoggerLogBufferFull(t *testing.T) {
	logger := &NATSAuditLogger{
		entries: make(chan *AuditEntry, 1),
	}

	// Fill the buffer
	logger.entries <- &AuditEntry{}

	// This should fail with buffer full
	entry := &AuditEntry{
		AuditType: ActionCommandExecuted,
	}

	err := logger.Log(context.Background(), entry)
	if err == nil {
		t.Error("expected error when buffer is full")
	}

	if logger.entriesDropped != 1 {
		t.Errorf("expected 1 dropped entry, got %d", logger.entriesDropped)
	}
}

func TestNATSAuditSubscriberUnsubscribeNil(t *testing.T) {
	s := &NATSAuditSubscriber{
		sub: nil,
	}

	err := s.Unsubscribe()
	if err != nil {
		t.Errorf("expected no error for nil subscription, got %v", err)
	}
}

func TestNATSAuditSubscriberClose(t *testing.T) {
	s := &NATSAuditSubscriber{
		sub: nil,
	}

	err := s.Close()
	if err != nil {
		t.Errorf("expected no error for close, got %v", err)
	}
}

func TestNATSAuditBatch(t *testing.T) {
	batch := NATSAuditBatch{
		Messages: []NATSAuditMessage{
			{AuditType: "command_executed", User: "user1"},
			{AuditType: "state_applied", User: "user2"},
		},
		Timestamp: "2024-01-15T10:30:00Z",
		Service:   "test-service",
		Count:     2,
	}

	data, err := json.Marshal(batch)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded NATSAuditBatch
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.Count != 2 {
		t.Errorf("expected count 2, got %d", decoded.Count)
	}

	if len(decoded.Messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(decoded.Messages))
	}
}

func TestNATSAuditAggregator(t *testing.T) {
	agg := NewNATSAuditAggregator(100)

	if agg.Count() != 0 {
		t.Errorf("expected count 0, got %d", agg.Count())
	}

	// Add some messages
	agg.Add(&NATSAuditMessage{
		AuditType: "command_executed",
		User:      "user1",
		Tool:      "kscore-exec",
		Result:    "success",
		Timestamp: "2024-01-15T10:30:00Z",
	})

	agg.Add(&NATSAuditMessage{
		AuditType: "state_applied",
		User:      "user2",
		Tool:      "kscore-state",
		Result:    "failure",
		Timestamp: "2024-01-15T10:31:00Z",
	})

	agg.Add(&NATSAuditMessage{
		AuditType: "command_executed",
		User:      "user1",
		Tool:      "kscore-exec",
		Result:    "success",
		Timestamp: "2024-01-15T10:32:00Z",
	})

	if agg.Count() != 3 {
		t.Errorf("expected count 3, got %d", agg.Count())
	}

	// Test Messages()
	msgs := agg.Messages()
	if len(msgs) != 3 {
		t.Errorf("expected 3 messages, got %d", len(msgs))
	}
}

func TestNATSAuditAggregatorFilterByUser(t *testing.T) {
	agg := NewNATSAuditAggregator(100)

	agg.Add(&NATSAuditMessage{User: "user1", AuditType: "command_executed"})
	agg.Add(&NATSAuditMessage{User: "user2", AuditType: "state_applied"})
	agg.Add(&NATSAuditMessage{User: "user1", AuditType: "command_executed"})

	filtered := agg.FilterByUser("user1")
	if len(filtered) != 2 {
		t.Errorf("expected 2 messages for user1, got %d", len(filtered))
	}

	filtered = agg.FilterByUser("user2")
	if len(filtered) != 1 {
		t.Errorf("expected 1 message for user2, got %d", len(filtered))
	}

	filtered = agg.FilterByUser("nonexistent")
	if len(filtered) != 0 {
		t.Errorf("expected 0 messages for nonexistent user, got %d", len(filtered))
	}
}

func TestNATSAuditAggregatorFilterByAction(t *testing.T) {
	agg := NewNATSAuditAggregator(100)

	agg.Add(&NATSAuditMessage{AuditType: "command_executed"})
	agg.Add(&NATSAuditMessage{AuditType: "state_applied"})
	agg.Add(&NATSAuditMessage{AuditType: "command_executed"})

	filtered := agg.FilterByAction(ActionCommandExecuted)
	if len(filtered) != 2 {
		t.Errorf("expected 2 command_executed, got %d", len(filtered))
	}

	filtered = agg.FilterByAction(ActionStateApplied)
	if len(filtered) != 1 {
		t.Errorf("expected 1 state_applied, got %d", len(filtered))
	}
}

func TestNATSAuditAggregatorFilterByResult(t *testing.T) {
	agg := NewNATSAuditAggregator(100)

	agg.Add(&NATSAuditMessage{Result: "success"})
	agg.Add(&NATSAuditMessage{Result: "failure"})
	agg.Add(&NATSAuditMessage{Result: "success"})
	agg.Add(&NATSAuditMessage{Result: "denied"})

	filtered := agg.FilterByResult(ResultSuccess)
	if len(filtered) != 2 {
		t.Errorf("expected 2 success, got %d", len(filtered))
	}

	filtered = agg.FilterByResult(ResultFailure)
	if len(filtered) != 1 {
		t.Errorf("expected 1 failure, got %d", len(filtered))
	}

	filtered = agg.FilterByResult(ResultDenied)
	if len(filtered) != 1 {
		t.Errorf("expected 1 denied, got %d", len(filtered))
	}
}

func TestNATSAuditAggregatorFilterByTool(t *testing.T) {
	agg := NewNATSAuditAggregator(100)

	agg.Add(&NATSAuditMessage{Tool: "kscore-exec"})
	agg.Add(&NATSAuditMessage{Tool: "kscore-state"})
	agg.Add(&NATSAuditMessage{Tool: "kscore-exec"})

	filtered := agg.FilterByTool("kscore-exec")
	if len(filtered) != 2 {
		t.Errorf("expected 2 kscore-exec, got %d", len(filtered))
	}

	filtered = agg.FilterByTool("kscore-state")
	if len(filtered) != 1 {
		t.Errorf("expected 1 kscore-state, got %d", len(filtered))
	}
}

func TestNATSAuditAggregatorFilterByTimeRange(t *testing.T) {
	agg := NewNATSAuditAggregator(100)

	agg.Add(&NATSAuditMessage{Timestamp: "2024-01-15T10:00:00Z"})
	agg.Add(&NATSAuditMessage{Timestamp: "2024-01-15T11:00:00Z"})
	agg.Add(&NATSAuditMessage{Timestamp: "2024-01-15T12:00:00Z"})

	start := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	end := time.Date(2024, 1, 15, 12, 30, 0, 0, time.UTC)

	filtered := agg.FilterByTimeRange(start, end)
	if len(filtered) != 2 {
		t.Errorf("expected 2 messages in range, got %d", len(filtered))
	}
}

func TestNATSAuditAggregatorClear(t *testing.T) {
	agg := NewNATSAuditAggregator(100)

	agg.Add(&NATSAuditMessage{AuditType: "command_executed"})
	agg.Add(&NATSAuditMessage{AuditType: "state_applied"})

	if agg.Count() != 2 {
		t.Errorf("expected count 2, got %d", agg.Count())
	}

	agg.Clear()

	if agg.Count() != 0 {
		t.Errorf("expected count 0 after clear, got %d", agg.Count())
	}
}

func TestNATSAuditAggregatorMaxSize(t *testing.T) {
	agg := NewNATSAuditAggregator(3)

	agg.Add(&NATSAuditMessage{AuditType: "msg1"})
	agg.Add(&NATSAuditMessage{AuditType: "msg2"})
	agg.Add(&NATSAuditMessage{AuditType: "msg3"})
	agg.Add(&NATSAuditMessage{AuditType: "msg4"})

	if agg.Count() != 3 {
		t.Errorf("expected count 3 (max size), got %d", agg.Count())
	}

	msgs := agg.Messages()
	// Should have msg2, msg3, msg4 (oldest evicted)
	if msgs[0].AuditType != "msg2" {
		t.Errorf("expected first message to be msg2, got %s", msgs[0].AuditType)
	}
}

func TestNATSAuditAggregatorSummary(t *testing.T) {
	agg := NewNATSAuditAggregator(100)

	agg.Add(&NATSAuditMessage{
		AuditType: "command_executed",
		User:      "user1",
		Tool:      "kscore-exec",
		Result:    "success",
		Timestamp: "2024-01-15T10:00:00Z",
	})

	agg.Add(&NATSAuditMessage{
		AuditType: "command_executed",
		User:      "user2",
		Tool:      "kscore-exec",
		Result:    "failure",
		Timestamp: "2024-01-15T10:05:00Z",
	})

	agg.Add(&NATSAuditMessage{
		AuditType: "state_applied",
		User:      "user1",
		Tool:      "kscore-state",
		Result:    "success",
		Timestamp: "2024-01-15T10:10:00Z",
	})

	agg.Add(&NATSAuditMessage{
		AuditType: "command_executed",
		User:      "user3",
		Tool:      "kscore-exec",
		Result:    "denied",
		Timestamp: "2024-01-15T10:15:00Z",
	})

	summary := agg.Summary()

	if summary.TotalCount != 4 {
		t.Errorf("expected total 4, got %d", summary.TotalCount)
	}

	if summary.SuccessCount != 2 {
		t.Errorf("expected 2 success, got %d", summary.SuccessCount)
	}

	if summary.FailureCount != 1 {
		t.Errorf("expected 1 failure, got %d", summary.FailureCount)
	}

	if summary.DeniedCount != 1 {
		t.Errorf("expected 1 denied, got %d", summary.DeniedCount)
	}

	if summary.UniqueUsers != 3 {
		t.Errorf("expected 3 unique users, got %d", summary.UniqueUsers)
	}

	if summary.CountByAction["command_executed"] != 3 {
		t.Errorf("expected 3 command_executed, got %d", summary.CountByAction["command_executed"])
	}

	if summary.CountByAction["state_applied"] != 1 {
		t.Errorf("expected 1 state_applied, got %d", summary.CountByAction["state_applied"])
	}

	if summary.CountByTool["kscore-exec"] != 3 {
		t.Errorf("expected 3 kscore-exec, got %d", summary.CountByTool["kscore-exec"])
	}

	if summary.CountByTool["kscore-state"] != 1 {
		t.Errorf("expected 1 kscore-state, got %d", summary.CountByTool["kscore-state"])
	}

	if summary.OldestEntry != "2024-01-15T10:00:00Z" {
		t.Errorf("expected oldest 2024-01-15T10:00:00Z, got %s", summary.OldestEntry)
	}

	if summary.NewestEntry != "2024-01-15T10:15:00Z" {
		t.Errorf("expected newest 2024-01-15T10:15:00Z, got %s", summary.NewestEntry)
	}
}

func TestNewNATSAuditAggregatorDefaultSize(t *testing.T) {
	agg := NewNATSAuditAggregator(0)

	// Should use default of 10000
	if agg.maxSize != 10000 {
		t.Errorf("expected default max size 10000, got %d", agg.maxSize)
	}
}

func TestNATSAuditSummaryJSON(t *testing.T) {
	summary := &NATSAuditSummary{
		TotalCount:    100,
		SuccessCount:  80,
		FailureCount:  15,
		DeniedCount:   5,
		TimeoutCount:  0,
		CountByAction: map[string]int{"command_executed": 70, "state_applied": 30},
		CountByTool:   map[string]int{"kscore-exec": 60, "kscore-state": 40},
		CountByUser:   map[string]int{"admin": 50, "operator": 50},
		UniqueUsers:   2,
		OldestEntry:   "2024-01-15T00:00:00Z",
		NewestEntry:   "2024-01-15T23:59:59Z",
	}

	data, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("failed to marshal summary: %v", err)
	}

	var decoded NATSAuditSummary
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal summary: %v", err)
	}

	if decoded.TotalCount != 100 {
		t.Errorf("expected total 100, got %d", decoded.TotalCount)
	}

	if decoded.SuccessCount != 80 {
		t.Errorf("expected success 80, got %d", decoded.SuccessCount)
	}
}
