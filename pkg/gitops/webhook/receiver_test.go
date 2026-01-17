package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/testing/helpers"
)

// MockEventProcessor for testing
type MockEventProcessor struct {
	mu           sync.Mutex
	events       []*WebhookEvent
	processError error
}

func (m *MockEventProcessor) ProcessEvent(ctx context.Context, event *WebhookEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
	return m.processError
}

func (m *MockEventProcessor) GetEvents() []*WebhookEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Return a copy of the slice
	events := make([]*WebhookEvent, len(m.events))
	copy(events, m.events)
	return events
}

func (m *MockEventProcessor) EventCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.events)
}

func TestReceiverArgoCD(t *testing.T) {
	processor := &MockEventProcessor{}
	config := &WebhookConfig{
		Enabled:  true,
		Addr:     ":8080",
		Path:     "/webhooks",
		Auth:     AuthConfig{Type: AuthTypeNone},
		Handlers: []string{"argocd", "flux", "github", "gitlab"},
	}

	receiver := NewReceiver(config, processor)

	payload := `{
		"application": {
			"metadata": {
				"name": "test-app",
				"namespace": "argocd"
			},
			"spec": {
				"source": {
					"repoURL": "https://github.com/example/repo",
					"targetRevision": "main"
				}
			},
			"status": {
				"sync": {
					"status": "Synced",
					"revision": "abc123"
				},
				"health": {
					"status": "Healthy"
				}
			}
		},
		"type": "sync"
	}`

	req := httptest.NewRequest(http.MethodPost, "/webhooks", bytes.NewBufferString(payload))
	req.Header.Set("X-Argo-CD-Webhook", "true")
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	receiver.handleWebhook(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("handleWebhook() status = %d, want %d", w.Code, http.StatusOK)
	}

	if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		return processor.EventCount() == 1, nil
	}); err != nil {
		t.Fatalf("processor received %d events, want 1: %v", processor.EventCount(), err)
	}

	events := processor.GetEvents()
	if len(events) != 1 {
		t.Fatalf("processor received %d events, want 1", len(events))
	}

	event := events[0]
	if event.Type != WebhookTypeArgoCD {
		t.Errorf("event.Type = %v, want %v", event.Type, WebhookTypeArgoCD)
	}

	// Check stats
	stats := receiver.GetStats()
	if stats.TotalReceived != 1 {
		t.Errorf("stats.TotalReceived = %d, want 1", stats.TotalReceived)
	}
}

func TestReceiverFlux(t *testing.T) {
	processor := &MockEventProcessor{}
	config := DefaultWebhookConfig()

	receiver := NewReceiver(config, processor)

	payload := `{
		"involvedObject": {
			"kind": "HelmRelease",
			"name": "test-app",
			"namespace": "production"
		},
		"severity": "info",
		"timestamp": "2025-01-01T00:00:00Z",
		"message": "Reconciled",
		"reason": "ReconciliationSucceeded"
	}`

	req := httptest.NewRequest(http.MethodPost, "/webhooks", bytes.NewBufferString(payload))
	req.Header.Set("X-Flux-Event", "reconciliation")
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	receiver.handleWebhook(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("handleWebhook() status = %d, want %d", w.Code, http.StatusOK)
	}

	if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		return processor.EventCount() == 1, nil
	}); err != nil {
		t.Fatalf("processor received %d events, want 1: %v", processor.EventCount(), err)
	}

	events := processor.GetEvents()
	if len(events) != 1 {
		t.Fatalf("processor received %d events, want 1", len(events))
	}

	event := events[0]
	if event.Type != WebhookTypeFlux {
		t.Errorf("event.Type = %v, want %v", event.Type, WebhookTypeFlux)
	}
}

func TestReceiverMethodNotAllowed(t *testing.T) {
	processor := &MockEventProcessor{}
	receiver := NewReceiver(DefaultWebhookConfig(), processor)

	req := httptest.NewRequest(http.MethodGet, "/webhooks", nil)
	w := httptest.NewRecorder()

	receiver.handleWebhook(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("handleWebhook() status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestReceiverUnknownWebhookType(t *testing.T) {
	processor := &MockEventProcessor{}
	receiver := NewReceiver(DefaultWebhookConfig(), processor)

	req := httptest.NewRequest(http.MethodPost, "/webhooks", bytes.NewBufferString("{}"))
	req.Header.Set("Content-Type", "application/json")
	// No identifying headers

	w := httptest.NewRecorder()
	receiver.handleWebhook(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("handleWebhook() status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestReceiverHealth(t *testing.T) {
	processor := &MockEventProcessor{}
	receiver := NewReceiver(DefaultWebhookConfig(), processor)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	receiver.handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("handleHealth() status = %d, want %d", w.Code, http.StatusOK)
	}

	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response["status"] != "healthy" {
		t.Errorf("status = %v, want healthy", response["status"])
	}
}

func TestReceiverStats(t *testing.T) {
	processor := &MockEventProcessor{}
	receiver := NewReceiver(DefaultWebhookConfig(), processor)

	// Send a webhook
	payload := `{
		"application": {
			"metadata": {"name": "test", "namespace": "default"},
			"status": {"sync": {"status": "Synced"}}
		}
	}`

	req := httptest.NewRequest(http.MethodPost, "/webhooks", bytes.NewBufferString(payload))
	req.Header.Set("X-Argo-CD-Webhook", "true")

	w := httptest.NewRecorder()
	receiver.handleWebhook(w, req)

	if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		stats := receiver.GetStats()
		return stats.TotalReceived == 1 && stats.TotalProcessed == 1, nil
	}); err != nil {
		t.Fatalf("expected stats to record webhook: %v", err)
	}

	// Get stats
	req = httptest.NewRequest(http.MethodGet, "/stats", nil)
	w = httptest.NewRecorder()

	receiver.handleStats(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("handleStats() status = %d, want %d", w.Code, http.StatusOK)
	}

	var stats ReceiverStats
	if err := json.NewDecoder(w.Body).Decode(&stats); err != nil {
		t.Fatalf("failed to decode stats: %v", err)
	}

	if stats.TotalReceived != 1 {
		t.Errorf("stats.TotalReceived = %d, want 1", stats.TotalReceived)
	}

	if stats.TotalProcessed != 1 {
		t.Errorf("stats.TotalProcessed = %d, want 1", stats.TotalProcessed)
	}
}

func TestReceiverAuthentication(t *testing.T) {
	processor := &MockEventProcessor{}
	config := &WebhookConfig{
		Enabled: true,
		Addr:    ":8080",
		Path:    "/webhooks",
		Auth: AuthConfig{
			Type:  AuthTypeBearer,
			Token: "secret-token",
		},
		Handlers: []string{"argocd"},
	}

	receiver := NewReceiver(config, processor)

	payload := `{
		"application": {
			"metadata": {"name": "test", "namespace": "default"},
			"status": {"sync": {"status": "Synced"}}
		}
	}`

	tests := []struct {
		name       string
		authHeader string
		wantStatus int
	}{
		{
			name:       "valid token",
			authHeader: "Bearer secret-token",
			wantStatus: http.StatusOK,
		},
		{
			name:       "invalid token",
			authHeader: "Bearer wrong-token",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "missing token",
			authHeader: "",
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/webhooks", bytes.NewBufferString(payload))
			req.Header.Set("X-Argo-CD-Webhook", "true")
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			w := httptest.NewRecorder()
			receiver.handleWebhook(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("handleWebhook() status = %d, want %d", w.Code, tt.wantStatus)
			}
		})
	}
}
