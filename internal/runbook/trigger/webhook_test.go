package trigger

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shawnbutts/keystone-core/internal/runbook"
)

func TestWebhookTrigger_Validate(t *testing.T) {
	tests := []struct {
		name    string
		trigger *WebhookTrigger
		wantErr bool
	}{
		{
			name: "valid with no auth",
			trigger: &WebhookTrigger{
				ID:         "test-webhook",
				Name:       "Test Webhook",
				RunbookRef: RunbookRef{Name: "test-runbook"},
				Path:       "/webhooks/test",
				Enabled:    true,
			},
			wantErr: false,
		},
		{
			name: "valid with token auth",
			trigger: &WebhookTrigger{
				ID:         "test-webhook",
				Name:       "Test Webhook",
				RunbookRef: RunbookRef{Name: "test-runbook"},
				Path:       "/webhooks/test",
				Authentication: &WebhookAuth{
					Type:   WebhookAuthToken,
					Header: "X-API-Key",
					Token:  "secret-token",
				},
				Enabled: true,
			},
			wantErr: false,
		},
		{
			name: "valid with HMAC auth",
			trigger: &WebhookTrigger{
				ID:         "test-webhook",
				Name:       "Test Webhook",
				RunbookRef: RunbookRef{Name: "test-runbook"},
				Path:       "/webhooks/test",
				Authentication: &WebhookAuth{
					Type:            WebhookAuthHMAC,
					Secret:          "webhook-secret",
					SignatureHeader: "X-Hub-Signature-256",
					SignaturePrefix: "sha256=",
				},
				Enabled: true,
			},
			wantErr: false,
		},
		{
			name: "missing id",
			trigger: &WebhookTrigger{
				Name:       "Test Webhook",
				RunbookRef: RunbookRef{Name: "test-runbook"},
				Path:       "/webhooks/test",
			},
			wantErr: true,
		},
		{
			name: "missing path",
			trigger: &WebhookTrigger{
				ID:         "test-webhook",
				Name:       "Test Webhook",
				RunbookRef: RunbookRef{Name: "test-runbook"},
			},
			wantErr: true,
		},
		{
			name: "path without leading slash",
			trigger: &WebhookTrigger{
				ID:         "test-webhook",
				Name:       "Test Webhook",
				RunbookRef: RunbookRef{Name: "test-runbook"},
				Path:       "webhooks/test",
			},
			wantErr: true,
		},
		{
			name: "token auth missing header",
			trigger: &WebhookTrigger{
				ID:         "test-webhook",
				Name:       "Test Webhook",
				RunbookRef: RunbookRef{Name: "test-runbook"},
				Path:       "/webhooks/test",
				Authentication: &WebhookAuth{
					Type:  WebhookAuthToken,
					Token: "secret",
				},
			},
			wantErr: true,
		},
		{
			name: "HMAC auth missing secret",
			trigger: &WebhookTrigger{
				ID:         "test-webhook",
				Name:       "Test Webhook",
				RunbookRef: RunbookRef{Name: "test-runbook"},
				Path:       "/webhooks/test",
				Authentication: &WebhookAuth{
					Type:            WebhookAuthHMAC,
					SignatureHeader: "X-Signature",
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

func TestWebhookTriggerManager_RegisterAndGet(t *testing.T) {
	manager := NewWebhookTriggerManager()

	trigger := &WebhookTrigger{
		ID:         "test-webhook",
		Name:       "Test Webhook",
		RunbookRef: RunbookRef{Name: "test-runbook"},
		Path:       "/webhooks/test",
		Enabled:    true,
	}

	// Register
	if err := manager.Register(trigger); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	// Get by ID
	got, ok := manager.Get("test-webhook")
	if !ok {
		t.Fatal("Get() returned false")
	}
	if got.ID != trigger.ID {
		t.Errorf("Get() ID = %v, want %v", got.ID, trigger.ID)
	}

	// Get by path
	got, ok = manager.GetByPath("/webhooks/test")
	if !ok {
		t.Fatal("GetByPath() returned false")
	}
	if got.ID != trigger.ID {
		t.Errorf("GetByPath() ID = %v, want %v", got.ID, trigger.ID)
	}

	// List
	triggers := manager.List()
	if len(triggers) != 1 {
		t.Errorf("List() returned %d triggers, want 1", len(triggers))
	}

	// Duplicate registration
	if err := manager.Register(trigger); err == nil {
		t.Error("Expected error for duplicate registration")
	}

	// Duplicate path
	trigger2 := &WebhookTrigger{
		ID:         "test-webhook-2",
		Name:       "Test Webhook 2",
		RunbookRef: RunbookRef{Name: "test-runbook"},
		Path:       "/webhooks/test", // Same path
		Enabled:    true,
	}
	if err := manager.Register(trigger2); err == nil {
		t.Error("Expected error for duplicate path")
	}

	// Unregister
	if err := manager.Unregister("test-webhook"); err != nil {
		t.Fatalf("Unregister() error = %v", err)
	}

	_, ok = manager.Get("test-webhook")
	if ok {
		t.Error("Get() should return false after unregister")
	}

	_, ok = manager.GetByPath("/webhooks/test")
	if ok {
		t.Error("GetByPath() should return false after unregister")
	}
}

func TestWebhookTriggerManager_HandleRequest_NotFound(t *testing.T) {
	manager := NewWebhookTriggerManager()

	req := &WebhookRequest{
		Method: "POST",
		Path:   "/webhooks/nonexistent",
	}

	resp := manager.HandleRequest(context.Background(), req)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("StatusCode = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func TestWebhookTriggerManager_HandleRequest_Disabled(t *testing.T) {
	manager := NewWebhookTriggerManager()

	trigger := &WebhookTrigger{
		ID:         "test-webhook",
		Name:       "Test Webhook",
		RunbookRef: RunbookRef{Name: "test-runbook"},
		Path:       "/webhooks/test",
		Enabled:    false, // Disabled
	}
	manager.Register(trigger)

	req := &WebhookRequest{
		Method: "POST",
		Path:   "/webhooks/test",
	}

	resp := manager.HandleRequest(context.Background(), req)
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("StatusCode = %d, want %d", resp.StatusCode, http.StatusServiceUnavailable)
	}
}

func TestWebhookTriggerManager_HandleRequest_MethodNotAllowed(t *testing.T) {
	manager := NewWebhookTriggerManager()

	trigger := &WebhookTrigger{
		ID:         "test-webhook",
		Name:       "Test Webhook",
		RunbookRef: RunbookRef{Name: "test-runbook"},
		Path:       "/webhooks/test",
		Methods:    []string{"POST"},
		Enabled:    true,
	}
	manager.Register(trigger)

	req := &WebhookRequest{
		Method: "GET", // Not allowed
		Path:   "/webhooks/test",
	}

	resp := manager.HandleRequest(context.Background(), req)
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("StatusCode = %d, want %d", resp.StatusCode, http.StatusMethodNotAllowed)
	}
}

func TestWebhookTriggerManager_TokenAuth(t *testing.T) {
	manager := NewWebhookTriggerManager()

	trigger := &WebhookTrigger{
		ID:         "test-webhook",
		Name:       "Test Webhook",
		RunbookRef: RunbookRef{Name: "test-runbook"},
		Path:       "/webhooks/test",
		Authentication: &WebhookAuth{
			Type:   WebhookAuthToken,
			Header: "X-API-Key",
			Token:  "secret-token",
		},
		Enabled: true,
	}
	manager.Register(trigger)

	// Missing token
	req := &WebhookRequest{
		Method:  "POST",
		Path:    "/webhooks/test",
		Headers: http.Header{},
	}
	resp := manager.HandleRequest(context.Background(), req)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Missing token: StatusCode = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}

	// Wrong token
	req.Headers = http.Header{"X-Api-Key": []string{"wrong-token"}}
	resp = manager.HandleRequest(context.Background(), req)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Wrong token: StatusCode = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}

	// Correct token (note: header lookup is case-insensitive)
	req.Headers = http.Header{"X-Api-Key": []string{"secret-token"}}
	resp = manager.HandleRequest(context.Background(), req)
	// Will fail with internal error due to no executor, but auth passed
	if resp.StatusCode == http.StatusUnauthorized {
		t.Error("Correct token should pass authentication")
	}
}

func TestWebhookTriggerManager_HMACAuth(t *testing.T) {
	manager := NewWebhookTriggerManager()

	secret := "webhook-secret"
	trigger := &WebhookTrigger{
		ID:         "test-webhook",
		Name:       "Test Webhook",
		RunbookRef: RunbookRef{Name: "test-runbook"},
		Path:       "/webhooks/test",
		Authentication: &WebhookAuth{
			Type:            WebhookAuthHMAC,
			Secret:          secret,
			SignatureHeader: "X-Hub-Signature-256",
			SignaturePrefix: "sha256=",
		},
		Enabled: true,
	}
	manager.Register(trigger)

	body := []byte(`{"action":"deploy"}`)

	// Calculate correct signature
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	// Missing signature
	req := &WebhookRequest{
		Method:  "POST",
		Path:    "/webhooks/test",
		Headers: http.Header{},
		Body:    body,
	}
	resp := manager.HandleRequest(context.Background(), req)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Missing signature: StatusCode = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}

	// Wrong signature
	req.Headers = http.Header{"X-Hub-Signature-256": []string{"sha256=wrongsignature"}}
	resp = manager.HandleRequest(context.Background(), req)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Wrong signature: StatusCode = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}

	// Correct signature
	req.Headers = http.Header{"X-Hub-Signature-256": []string{signature}}
	resp = manager.HandleRequest(context.Background(), req)
	// Will fail with internal error due to no executor, but auth passed
	if resp.StatusCode == http.StatusUnauthorized {
		t.Error("Correct signature should pass authentication")
	}
}

func TestWebhookTriggerManager_BasicAuth(t *testing.T) {
	manager := NewWebhookTriggerManager()

	// Base64 encode "user:password"
	credentials := base64.StdEncoding.EncodeToString([]byte("user:password"))

	trigger := &WebhookTrigger{
		ID:         "test-webhook",
		Name:       "Test Webhook",
		RunbookRef: RunbookRef{Name: "test-runbook"},
		Path:       "/webhooks/test",
		Authentication: &WebhookAuth{
			Type:  WebhookAuthBasic,
			Token: credentials,
		},
		Enabled: true,
	}
	manager.Register(trigger)

	// Missing auth
	req := &WebhookRequest{
		Method:  "POST",
		Path:    "/webhooks/test",
		Headers: http.Header{},
	}
	resp := manager.HandleRequest(context.Background(), req)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Missing auth: StatusCode = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}

	// Wrong credentials
	wrongCreds := base64.StdEncoding.EncodeToString([]byte("wrong:creds"))
	req.Headers = http.Header{"Authorization": []string{"Basic " + wrongCreds}}
	resp = manager.HandleRequest(context.Background(), req)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Wrong credentials: StatusCode = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}

	// Correct credentials
	req.Headers = http.Header{"Authorization": []string{"Basic " + credentials}}
	resp = manager.HandleRequest(context.Background(), req)
	if resp.StatusCode == http.StatusUnauthorized {
		t.Error("Correct credentials should pass authentication")
	}
}

func TestWebhookTriggerManager_IPAllowlist(t *testing.T) {
	manager := NewWebhookTriggerManager()

	trigger := &WebhookTrigger{
		ID:         "test-webhook",
		Name:       "Test Webhook",
		RunbookRef: RunbookRef{Name: "test-runbook"},
		Path:       "/webhooks/test",
		AllowedIPs: []string{"192.168.1.1", "10.0.0.1"},
		Enabled:    true,
	}
	manager.Register(trigger)

	// Allowed IP
	req := &WebhookRequest{
		Method:     "POST",
		Path:       "/webhooks/test",
		Headers:    http.Header{},
		RemoteAddr: "192.168.1.1:12345",
	}
	resp := manager.HandleRequest(context.Background(), req)
	// Will fail with internal error due to no executor, but IP check passed
	if resp.StatusCode == http.StatusForbidden {
		t.Error("Allowed IP should pass")
	}

	// Blocked IP
	req.RemoteAddr = "192.168.1.2:12345"
	resp = manager.HandleRequest(context.Background(), req)
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("Blocked IP: StatusCode = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}
}

func TestWebhookTriggerManager_RateLimit(t *testing.T) {
	manager := NewWebhookTriggerManager()

	trigger := &WebhookTrigger{
		ID:         "test-webhook",
		Name:       "Test Webhook",
		RunbookRef: RunbookRef{Name: "test-runbook"},
		Path:       "/webhooks/test",
		RateLimit: &RateLimitConfig{
			MaxExecutions: 2,
			Window:        60 * 1000 * 1000 * 1000, // 1 minute in nanoseconds
		},
		Enabled: true,
	}
	manager.Register(trigger)

	req := &WebhookRequest{
		Method:  "POST",
		Path:    "/webhooks/test",
		Headers: http.Header{},
	}

	// First two requests should pass (rate limit check)
	for i := 0; i < 2; i++ {
		resp := manager.HandleRequest(context.Background(), req)
		if resp.StatusCode == http.StatusTooManyRequests {
			t.Errorf("Request %d should not be rate limited", i+1)
		}
	}

	// Third request should be rate limited
	resp := manager.HandleRequest(context.Background(), req)
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Errorf("Third request: StatusCode = %d, want %d", resp.StatusCode, http.StatusTooManyRequests)
	}
}

func TestWebhookTriggerManager_Execute(t *testing.T) {
	repo := newMockRepository()
	executor := newMockExecutor()
	publisher := newMockPublisher()

	repo.Add(&runbook.Runbook{
		APIVersion: "runbook.keystone.io/v1",
		Kind:       "Runbook",
		Metadata:   runbook.Metadata{Name: "deploy"},
		Spec: runbook.RunbookSpec{
			Steps: []runbook.Step{{Name: "step1", Type: runbook.StepTypeNoop}},
		},
	})

	manager := NewWebhookTriggerManager(
		WithWebhookRepository(repo),
		WithWebhookExecutor(executor),
		WithWebhookPublisher(publisher),
	)

	trigger := &WebhookTrigger{
		ID:         "deploy-webhook",
		Name:       "Deploy Webhook",
		RunbookRef: RunbookRef{Name: "deploy"},
		Path:       "/webhooks/deploy",
		StaticInputs: map[string]interface{}{
			"environment": "production",
		},
		InputMappings: map[string]string{
			"version": "{{ .body.version }}",
		},
		Enabled: true,
	}
	manager.Register(trigger)

	req := &WebhookRequest{
		Method:  "POST",
		Path:    "/webhooks/deploy",
		Headers: http.Header{"Content-Type": []string{"application/json"}},
		Body:    []byte(`{"version":"1.2.3","commit":"abc123"}`),
	}

	resp := manager.HandleRequest(context.Background(), req)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want %d, error: %s", resp.StatusCode, http.StatusOK, resp.Error)
	}

	if resp.ExecutionID == "" {
		t.Error("ExecutionID should not be empty")
	}

	// Verify execution
	if executor.ExecutionCount() != 1 {
		t.Errorf("ExecutionCount = %d, want 1", executor.ExecutionCount())
	}

	lastExec := executor.LastExecution()
	if lastExec.inputs["environment"] != "production" {
		t.Errorf("Input environment = %v, want %q", lastExec.inputs["environment"], "production")
	}
	if lastExec.inputs["version"] != "1.2.3" {
		t.Errorf("Input version = %v, want %q", lastExec.inputs["version"], "1.2.3")
	}
	if lastExec.inputs["body_commit"] != "abc123" {
		t.Errorf("Input body_commit = %v, want %q", lastExec.inputs["body_commit"], "abc123")
	}
}

func TestWebhookTriggerManager_HTTPHandler(t *testing.T) {
	repo := newMockRepository()
	executor := newMockExecutor()

	repo.Add(&runbook.Runbook{
		APIVersion: "runbook.keystone.io/v1",
		Kind:       "Runbook",
		Metadata:   runbook.Metadata{Name: "test-runbook"},
		Spec: runbook.RunbookSpec{
			Steps: []runbook.Step{{Name: "step1", Type: runbook.StepTypeNoop}},
		},
	})

	manager := NewWebhookTriggerManager(
		WithWebhookRepository(repo),
		WithWebhookExecutor(executor),
	)

	trigger := &WebhookTrigger{
		ID:         "test-webhook",
		Name:       "Test Webhook",
		RunbookRef: RunbookRef{Name: "test-runbook"},
		Path:       "/webhooks/test",
		Enabled:    true,
	}
	manager.Register(trigger)

	handler := manager.HTTPHandler()

	// Test with httptest
	req := httptest.NewRequest("POST", "/webhooks/test", strings.NewReader(`{"foo":"bar"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("HTTP StatusCode = %d, want %d", w.Code, http.StatusOK)
	}

	if executor.ExecutionCount() != 1 {
		t.Errorf("ExecutionCount = %d, want 1", executor.ExecutionCount())
	}
}

func TestWebhookTriggerManager_EnableDisable(t *testing.T) {
	manager := NewWebhookTriggerManager()

	trigger := &WebhookTrigger{
		ID:         "test-webhook",
		Name:       "Test Webhook",
		RunbookRef: RunbookRef{Name: "test-runbook"},
		Path:       "/webhooks/test",
		Enabled:    true,
	}
	manager.Register(trigger)

	// Disable
	if err := manager.Disable("test-webhook"); err != nil {
		t.Fatalf("Disable() error = %v", err)
	}

	got, _ := manager.Get("test-webhook")
	if got.Enabled {
		t.Error("Trigger should be disabled")
	}

	// Enable
	if err := manager.Enable("test-webhook"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}

	got, _ = manager.Get("test-webhook")
	if !got.Enabled {
		t.Error("Trigger should be enabled")
	}
}
