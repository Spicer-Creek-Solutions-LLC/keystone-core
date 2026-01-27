package approval

import (
	"context"
	"sync"
	"testing"
	"time"
)

// mockNotifier implements Notifier for testing.
type mockNotifier struct {
	mu               sync.Mutex
	requestCalls     []*Request
	decisionCalls    []decisionCall
	reminderCalls    []*Request
	expiredCalls     []*Request
}

type decisionCall struct {
	request  *Request
	response Response
}

func newMockNotifier() *mockNotifier {
	return &mockNotifier{}
}

func (n *mockNotifier) NotifyApprovalRequest(ctx context.Context, req *Request, channels []string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.requestCalls = append(n.requestCalls, req)
	return nil
}

func (n *mockNotifier) NotifyApprovalDecision(ctx context.Context, req *Request, resp Response, channels []string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.decisionCalls = append(n.decisionCalls, decisionCall{req, resp})
	return nil
}

func (n *mockNotifier) NotifyApprovalReminder(ctx context.Context, req *Request, channels []string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.reminderCalls = append(n.reminderCalls, req)
	return nil
}

func (n *mockNotifier) NotifyApprovalExpired(ctx context.Context, req *Request, channels []string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.expiredCalls = append(n.expiredCalls, req)
	return nil
}

func (n *mockNotifier) ReminderCount() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return len(n.reminderCalls)
}

func (n *mockNotifier) RequestCount() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return len(n.requestCalls)
}

func TestReminderScheduler_ProcessReminders(t *testing.T) {
	storage := newMockStorage()
	notifier := newMockNotifier()
	manager := NewManager(storage)

	scheduler := NewReminderScheduler(manager, notifier, storage, ReminderSchedulerConfig{
		DefaultInterval: 10 * time.Millisecond,
		CheckInterval:   5 * time.Millisecond,
	})

	// Create a pending request with notify channels
	now := time.Now()
	req := &Request{
		ID:          "req-1",
		ExecutionID: "exec-1",
		StepName:    "step1",
		State:       RequestStatePending,
		Title:       "Test",
		Approvers:   []string{"user1"},
		Mode:        ApprovalModeAny,
		CreatedAt:   now.Add(-20 * time.Millisecond), // Created 20ms ago
		UpdatedAt:   now,
		Metadata: map[string]interface{}{
			"notify_channels": []interface{}{"slack"},
		},
	}
	storage.SaveRequest(context.Background(), req)

	// Process reminders
	scheduler.processReminders(context.Background())

	// Should have sent a reminder since creation was > defaultInterval ago
	if notifier.ReminderCount() != 1 {
		t.Errorf("ReminderCount = %d, want 1", notifier.ReminderCount())
	}

	// Process again immediately - should not send another reminder
	scheduler.processReminders(context.Background())
	if notifier.ReminderCount() != 1 {
		t.Errorf("ReminderCount after second call = %d, want 1", notifier.ReminderCount())
	}
}

func TestReminderScheduler_ProcessEscalations(t *testing.T) {
	storage := newMockStorage()
	notifier := newMockNotifier()
	manager := NewManager(storage)

	scheduler := NewReminderScheduler(manager, notifier, storage, ReminderSchedulerConfig{
		DefaultInterval: time.Hour,
		CheckInterval:   5 * time.Millisecond,
	})

	// Create a pending request that should be escalated
	now := time.Now()
	req := &Request{
		ID:          "req-1",
		ExecutionID: "exec-1",
		StepName:    "step1",
		State:       RequestStatePending,
		Title:       "Test",
		Approvers:   []string{"user1"},
		Mode:        ApprovalModeAny,
		CreatedAt:   now.Add(-time.Hour), // Created 1 hour ago
		UpdatedAt:   now,
		Metadata: map[string]interface{}{
			"escalate_after": "30m",
			"escalate_to":    []interface{}{"manager@example.com"},
		},
	}
	storage.SaveRequest(context.Background(), req)

	// Process escalations
	scheduler.processEscalations(context.Background())

	// Should have sent escalation notification
	if notifier.RequestCount() != 1 {
		t.Errorf("RequestCount = %d, want 1", notifier.RequestCount())
	}

	// Check that the request was updated with escalation marker
	updated, _ := storage.GetRequest(context.Background(), "req-1")
	if updated.Metadata["escalated"] != true {
		t.Error("expected escalated=true in metadata")
	}

	// Check that manager was added to approvers
	hasManager := false
	for _, a := range updated.Approvers {
		if a == "manager@example.com" {
			hasManager = true
			break
		}
	}
	if !hasManager {
		t.Error("expected manager@example.com in approvers")
	}

	// Process again - should not escalate again
	scheduler.processEscalations(context.Background())
	if notifier.RequestCount() != 1 {
		t.Errorf("RequestCount after second call = %d, want 1", notifier.RequestCount())
	}
}

func TestReminderScheduler_NoEscalationIfNotConfigured(t *testing.T) {
	storage := newMockStorage()
	notifier := newMockNotifier()
	manager := NewManager(storage)

	scheduler := NewReminderScheduler(manager, notifier, storage, ReminderSchedulerConfig{})

	// Create a pending request without escalation config
	now := time.Now()
	req := &Request{
		ID:          "req-1",
		ExecutionID: "exec-1",
		StepName:    "step1",
		State:       RequestStatePending,
		Title:       "Test",
		Approvers:   []string{"user1"},
		Mode:        ApprovalModeAny,
		CreatedAt:   now.Add(-time.Hour),
		UpdatedAt:   now,
	}
	storage.SaveRequest(context.Background(), req)

	// Process escalations
	scheduler.processEscalations(context.Background())

	// Should not have sent any notifications
	if notifier.RequestCount() != 0 {
		t.Errorf("RequestCount = %d, want 0", notifier.RequestCount())
	}
}

func TestReminderScheduler_StartStop(t *testing.T) {
	storage := newMockStorage()
	notifier := newMockNotifier()
	manager := NewManager(storage)

	scheduler := NewReminderScheduler(manager, notifier, storage, ReminderSchedulerConfig{
		CheckInterval: 10 * time.Millisecond,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	scheduler.Start(ctx)

	// Let it run a bit
	time.Sleep(30 * time.Millisecond)

	// Stop should not hang
	done := make(chan struct{})
	go func() {
		scheduler.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Good
	case <-time.After(time.Second):
		t.Fatal("Stop() timed out")
	}
}

func TestReminderScheduler_GetNotifyChannels(t *testing.T) {
	scheduler := &ReminderScheduler{}

	tests := []struct {
		name     string
		metadata map[string]interface{}
		expected []string
	}{
		{
			name:     "nil metadata",
			metadata: nil,
			expected: nil,
		},
		{
			name: "string slice",
			metadata: map[string]interface{}{
				"notify_channels": []string{"slack", "email:test@example.com"},
			},
			expected: []string{"slack", "email:test@example.com"},
		},
		{
			name: "interface slice",
			metadata: map[string]interface{}{
				"notify_channels": []interface{}{"slack", "email:test@example.com"},
			},
			expected: []string{"slack", "email:test@example.com"},
		},
		{
			name: "no channels key",
			metadata: map[string]interface{}{
				"other": "value",
			},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &Request{Metadata: tt.metadata}
			got := scheduler.getNotifyChannels(req)

			if len(got) != len(tt.expected) {
				t.Errorf("len = %d, want %d", len(got), len(tt.expected))
				return
			}

			for i, v := range got {
				if v != tt.expected[i] {
					t.Errorf("channels[%d] = %q, want %q", i, v, tt.expected[i])
				}
			}
		})
	}
}

func TestReminderScheduler_GetReminderInterval(t *testing.T) {
	scheduler := &ReminderScheduler{
		defaultInterval: 30 * time.Minute,
	}

	tests := []struct {
		name     string
		metadata map[string]interface{}
		expected time.Duration
	}{
		{
			name:     "nil metadata",
			metadata: nil,
			expected: 30 * time.Minute,
		},
		{
			name: "string duration",
			metadata: map[string]interface{}{
				"reminder_interval": "15m",
			},
			expected: 15 * time.Minute,
		},
		{
			name: "invalid string",
			metadata: map[string]interface{}{
				"reminder_interval": "invalid",
			},
			expected: 30 * time.Minute,
		},
		{
			name: "float64 nanoseconds",
			metadata: map[string]interface{}{
				"reminder_interval": float64(time.Hour),
			},
			expected: time.Hour,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &Request{Metadata: tt.metadata}
			got := scheduler.getReminderInterval(req)

			if got != tt.expected {
				t.Errorf("getReminderInterval = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestReminderScheduler_GetEscalateAfter(t *testing.T) {
	scheduler := &ReminderScheduler{}

	tests := []struct {
		name     string
		metadata map[string]interface{}
		expected time.Duration
	}{
		{
			name:     "nil metadata",
			metadata: nil,
			expected: 0,
		},
		{
			name: "string duration",
			metadata: map[string]interface{}{
				"escalate_after": "2h",
			},
			expected: 2 * time.Hour,
		},
		{
			name: "no key",
			metadata: map[string]interface{}{
				"other": "value",
			},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &Request{Metadata: tt.metadata}
			got := scheduler.getEscalateAfter(req)

			if got != tt.expected {
				t.Errorf("getEscalateAfter = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestReminderScheduler_GetEscalateTo(t *testing.T) {
	scheduler := &ReminderScheduler{}

	tests := []struct {
		name     string
		metadata map[string]interface{}
		expected []string
	}{
		{
			name:     "nil metadata",
			metadata: nil,
			expected: nil,
		},
		{
			name: "string slice",
			metadata: map[string]interface{}{
				"escalate_to": []string{"manager1", "manager2"},
			},
			expected: []string{"manager1", "manager2"},
		},
		{
			name: "interface slice",
			metadata: map[string]interface{}{
				"escalate_to": []interface{}{"manager1", "manager2"},
			},
			expected: []string{"manager1", "manager2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &Request{Metadata: tt.metadata}
			got := scheduler.getEscalateTo(req)

			if len(got) != len(tt.expected) {
				t.Errorf("len = %d, want %d", len(got), len(tt.expected))
				return
			}

			for i, v := range got {
				if v != tt.expected[i] {
					t.Errorf("escalate_to[%d] = %q, want %q", i, v, tt.expected[i])
				}
			}
		})
	}
}
