package approval

import (
	"testing"
	"time"
)

func TestRequestState_IsTerminal(t *testing.T) {
	tests := []struct {
		state    RequestState
		terminal bool
	}{
		{RequestStatePending, false},
		{RequestStateApproved, true},
		{RequestStateRejected, true},
		{RequestStateExpired, true},
		{RequestStateCancelled, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			if got := tt.state.IsTerminal(); got != tt.terminal {
				t.Errorf("RequestState(%q).IsTerminal() = %v, want %v", tt.state, got, tt.terminal)
			}
		})
	}
}

func TestApprovalMode_IsValid(t *testing.T) {
	tests := []struct {
		mode  ApprovalMode
		valid bool
	}{
		{ApprovalModeAny, true},
		{ApprovalModeAll, true},
		{ApprovalModeCount, true},
		{ApprovalMode("invalid"), false},
		{ApprovalMode(""), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.mode), func(t *testing.T) {
			if got := tt.mode.IsValid(); got != tt.valid {
				t.Errorf("ApprovalMode(%q).IsValid() = %v, want %v", tt.mode, got, tt.valid)
			}
		})
	}
}

func TestRequest_ApprovalCount(t *testing.T) {
	req := &Request{
		Responses: []Response{
			{Approver: "user1", Decision: DecisionApproved},
			{Approver: "user2", Decision: DecisionRejected},
			{Approver: "user3", Decision: DecisionApproved},
		},
	}

	if got := req.ApprovalCount(); got != 2 {
		t.Errorf("ApprovalCount() = %d, want 2", got)
	}
}

func TestRequest_RejectionCount(t *testing.T) {
	req := &Request{
		Responses: []Response{
			{Approver: "user1", Decision: DecisionApproved},
			{Approver: "user2", Decision: DecisionRejected},
			{Approver: "user3", Decision: DecisionRejected},
		},
	}

	if got := req.RejectionCount(); got != 2 {
		t.Errorf("RejectionCount() = %d, want 2", got)
	}
}

func TestRequest_HasResponded(t *testing.T) {
	req := &Request{
		Responses: []Response{
			{Approver: "user1", Decision: DecisionApproved},
		},
	}

	if !req.HasResponded("user1") {
		t.Error("HasResponded(user1) should be true")
	}

	if req.HasResponded("user2") {
		t.Error("HasResponded(user2) should be false")
	}
}

func TestRequest_IsApprover(t *testing.T) {
	req := &Request{
		Approvers: []string{"user1", "user2", "admin-group"},
	}

	tests := []struct {
		identity string
		expected bool
	}{
		{"user1", true},
		{"user2", true},
		{"admin-group", true},
		{"user3", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.identity, func(t *testing.T) {
			if got := req.IsApprover(tt.identity); got != tt.expected {
				t.Errorf("IsApprover(%q) = %v, want %v", tt.identity, got, tt.expected)
			}
		})
	}
}

func TestRequest_IsExpired(t *testing.T) {
	t.Run("no_expiration", func(t *testing.T) {
		req := &Request{}
		if req.IsExpired() {
			t.Error("IsExpired() should be false when ExpiresAt is nil")
		}
	})

	t.Run("not_expired", func(t *testing.T) {
		future := time.Now().Add(time.Hour)
		req := &Request{ExpiresAt: &future}
		if req.IsExpired() {
			t.Error("IsExpired() should be false when ExpiresAt is in the future")
		}
	})

	t.Run("expired", func(t *testing.T) {
		past := time.Now().Add(-time.Hour)
		req := &Request{ExpiresAt: &past}
		if !req.IsExpired() {
			t.Error("IsExpired() should be true when ExpiresAt is in the past")
		}
	})
}

func TestConfig_GetMode(t *testing.T) {
	tests := []struct {
		name     string
		config   Config
		expected ApprovalMode
	}{
		{"default", Config{}, ApprovalModeAny},
		{"any", Config{Mode: ApprovalModeAny}, ApprovalModeAny},
		{"all", Config{Mode: ApprovalModeAll}, ApprovalModeAll},
		{"count", Config{Mode: ApprovalModeCount}, ApprovalModeCount},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.config.GetMode(); got != tt.expected {
				t.Errorf("GetMode() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestConfig_GetRequiredCount(t *testing.T) {
	tests := []struct {
		name     string
		config   Config
		expected int
	}{
		{"any_mode", Config{Mode: ApprovalModeAny, Approvers: []string{"a", "b", "c"}}, 1},
		{"all_mode", Config{Mode: ApprovalModeAll, Approvers: []string{"a", "b", "c"}}, 3},
		{"count_mode", Config{Mode: ApprovalModeCount, RequiredCount: 2}, 2},
		{"count_mode_default", Config{Mode: ApprovalModeCount}, 1},
		{"default_mode", Config{Approvers: []string{"a", "b"}}, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.config.GetRequiredCount(); got != tt.expected {
				t.Errorf("GetRequiredCount() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestConfig_GetTimeout(t *testing.T) {
	tests := []struct {
		name     string
		timeout  string
		expected time.Duration
	}{
		{"empty", "", 0},
		{"1_hour", "1h", time.Hour},
		{"30_minutes", "30m", 30 * time.Minute},
		{"invalid", "invalid", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := Config{Timeout: tt.timeout}
			if got := config.GetTimeout(); got != tt.expected {
				t.Errorf("GetTimeout() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestConfig_GetReminderInterval(t *testing.T) {
	tests := []struct {
		name     string
		interval string
		expected time.Duration
	}{
		{"empty", "", 0},
		{"15_minutes", "15m", 15 * time.Minute},
		{"invalid", "not-a-duration", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := Config{ReminderInterval: tt.interval}
			if got := config.GetReminderInterval(); got != tt.expected {
				t.Errorf("GetReminderInterval() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestConfig_GetEscalateAfter(t *testing.T) {
	tests := []struct {
		name     string
		after    string
		expected time.Duration
	}{
		{"empty", "", 0},
		{"2_hours", "2h", 2 * time.Hour},
		{"invalid", "bad", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := Config{EscalateAfter: tt.after}
			if got := config.GetEscalateAfter(); got != tt.expected {
				t.Errorf("GetEscalateAfter() = %v, want %v", got, tt.expected)
			}
		})
	}
}
