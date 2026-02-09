package approval

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDefaultNotifier_NotifyApprovalRequest_Slack(t *testing.T) {
	// Create a mock Slack server
	var receivedPayload map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if err := json.NewDecoder(r.Body).Decode(&receivedPayload); err != nil {
			t.Errorf("decode payload: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := NotifierConfig{
		SlackWebhookURL: server.URL,
		SlackChannel:    "#approvals",
		ApprovalURLBase: "https://keystone.example.com/approvals",
	}

	notifier, err := NewDefaultNotifier(config)
	if err != nil {
		t.Fatalf("NewDefaultNotifier: %v", err)
	}

	expiresAt := time.Now().Add(time.Hour)
	req := &Request{
		ID:            "req-123",
		ExecutionID:   "exec-456",
		StepName:      "deploy-approval",
		State:         RequestStatePending,
		Title:         "Deploy to Production",
		Description:   "Please review the deployment",
		Approvers:     []string{"admin@example.com", "ops-team"},
		Mode:          ModeAny,
		RequiredCount: 1,
		ExpiresAt:     &expiresAt,
		Metadata: map[string]interface{}{
			"runbook_name": "deploy-runbook",
		},
		CreatedAt: time.Now(),
	}

	err = notifier.NotifyApprovalRequest(context.Background(), req, []string{"slack"})
	if err != nil {
		t.Fatalf("NotifyApprovalRequest: %v", err)
	}

	// Verify the payload was sent
	if receivedPayload == nil {
		t.Fatal("expected payload to be sent")
	}

	// Check that text field exists
	if _, ok := receivedPayload["text"]; !ok {
		t.Error("expected 'text' field in payload")
	}
}

func TestDefaultNotifier_NotifyApprovalDecision_Slack(t *testing.T) {
	var receivedPayload map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedPayload)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := NotifierConfig{
		SlackWebhookURL: server.URL,
	}

	notifier, err := NewDefaultNotifier(config)
	if err != nil {
		t.Fatalf("NewDefaultNotifier: %v", err)
	}

	req := &Request{
		ID:    "req-123",
		Title: "Deploy to Production",
		State: RequestStateApproved,
	}

	resp := Response{
		Approver:    "admin@example.com",
		Decision:    DecisionApproved,
		Comment:     "LGTM",
		RespondedAt: time.Now(),
	}

	err = notifier.NotifyApprovalDecision(context.Background(), req, resp, []string{"slack"})
	if err != nil {
		t.Fatalf("NotifyApprovalDecision: %v", err)
	}

	if receivedPayload == nil {
		t.Fatal("expected payload to be sent")
	}
}

func TestDefaultNotifier_NotifyApprovalReminder_Slack(t *testing.T) {
	var receivedPayload map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedPayload)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := NotifierConfig{
		SlackWebhookURL: server.URL,
	}

	notifier, err := NewDefaultNotifier(config)
	if err != nil {
		t.Fatalf("NewDefaultNotifier: %v", err)
	}

	expiresAt := time.Now().Add(30 * time.Minute)
	req := &Request{
		ID:        "req-123",
		Title:     "Deploy to Production",
		State:     RequestStatePending,
		ExpiresAt: &expiresAt,
	}

	err = notifier.NotifyApprovalReminder(context.Background(), req, []string{"slack"})
	if err != nil {
		t.Fatalf("NotifyApprovalReminder: %v", err)
	}

	if receivedPayload == nil {
		t.Fatal("expected payload to be sent")
	}
}

func TestDefaultNotifier_NotifyApprovalExpired_Slack(t *testing.T) {
	var receivedPayload map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedPayload)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := NotifierConfig{
		SlackWebhookURL: server.URL,
	}

	notifier, err := NewDefaultNotifier(config)
	if err != nil {
		t.Fatalf("NewDefaultNotifier: %v", err)
	}

	req := &Request{
		ID:            "req-123",
		StepName:      "deploy-approval",
		Title:         "Deploy to Production",
		State:         RequestStateExpired,
		RequiredCount: 2,
		Responses: []Response{
			{Approver: "user1", Decision: DecisionApproved},
		},
	}

	err = notifier.NotifyApprovalExpired(context.Background(), req, []string{"slack"})
	if err != nil {
		t.Fatalf("NotifyApprovalExpired: %v", err)
	}

	if receivedPayload == nil {
		t.Fatal("expected payload to be sent")
	}
}

func TestDefaultNotifier_MultipleChannels(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := NotifierConfig{
		SlackWebhookURL: server.URL,
	}

	notifier, err := NewDefaultNotifier(config)
	if err != nil {
		t.Fatalf("NewDefaultNotifier: %v", err)
	}

	req := &Request{
		ID:    "req-123",
		Title: "Test",
		State: RequestStatePending,
	}

	// Send to multiple slack channels
	err = notifier.NotifyApprovalRequest(context.Background(), req, []string{"slack:#channel1", "slack:#channel2"})
	if err != nil {
		t.Fatalf("NotifyApprovalRequest: %v", err)
	}

	if callCount != 2 {
		t.Errorf("expected 2 calls, got %d", callCount)
	}
}

func TestDefaultNotifier_SlackError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	config := NotifierConfig{
		SlackWebhookURL: server.URL,
	}

	notifier, err := NewDefaultNotifier(config)
	if err != nil {
		t.Fatalf("NewDefaultNotifier: %v", err)
	}

	req := &Request{
		ID:    "req-123",
		Title: "Test",
	}

	err = notifier.NotifyApprovalRequest(context.Background(), req, []string{"slack"})
	if err == nil {
		t.Error("expected error for Slack 500 response")
	}
}

func TestDefaultNotifier_NoSlackWebhook(t *testing.T) {
	config := NotifierConfig{}

	notifier, err := NewDefaultNotifier(config)
	if err != nil {
		t.Fatalf("NewDefaultNotifier: %v", err)
	}

	req := &Request{
		ID:    "req-123",
		Title: "Test",
	}

	err = notifier.NotifyApprovalRequest(context.Background(), req, []string{"slack"})
	if err == nil {
		t.Error("expected error when Slack webhook not configured")
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		duration time.Duration
		expected string
	}{
		{30 * time.Second, "30 seconds"},
		{time.Minute, "1 minutes"},
		{45 * time.Minute, "45 minutes"},
		{time.Hour, "1 hour"},
		{2 * time.Hour, "2 hours"},
		{24 * time.Hour, "1 day"},
		{48 * time.Hour, "2 days"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			got := formatDuration(tt.duration)
			if got != tt.expected {
				t.Errorf("formatDuration(%v) = %q, want %q", tt.duration, got, tt.expected)
			}
		})
	}
}

func TestBuildTemplateData(t *testing.T) {
	config := NotifierConfig{
		ApprovalURLBase: "https://example.com/approvals",
	}

	notifier, _ := NewDefaultNotifier(config)

	expiresAt := time.Now().Add(3 * time.Hour)
	req := &Request{
		ID:        "req-123",
		Title:     "Test Request",
		State:     RequestStatePending,
		ExpiresAt: &expiresAt,
	}

	data := notifier.buildTemplateData(req, nil)

	if data.ApproveURL != "https://example.com/approvals/approve/req-123" {
		t.Errorf("ApproveURL = %q, want %q", data.ApproveURL, "https://example.com/approvals/approve/req-123")
	}

	if data.RejectURL != "https://example.com/approvals/reject/req-123" {
		t.Errorf("RejectURL = %q, want %q", data.RejectURL, "https://example.com/approvals/reject/req-123")
	}

	if data.ViewURL != "https://example.com/approvals/view/req-123" {
		t.Errorf("ViewURL = %q, want %q", data.ViewURL, "https://example.com/approvals/view/req-123")
	}

	// TimeRemaining should be approximately 3 hours (may round to 2 hours due to test timing)
	if data.TimeRemaining != "3 hours" && data.TimeRemaining != "2 hours" {
		t.Errorf("TimeRemaining = %q, want approximately '3 hours' or '2 hours'", data.TimeRemaining)
	}
}
