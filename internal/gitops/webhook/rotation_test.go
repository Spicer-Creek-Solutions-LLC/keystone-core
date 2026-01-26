package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDefaultRotationConfig(t *testing.T) {
	config := DefaultRotationConfig()

	if config == nil {
		t.Fatal("expected non-nil config")
	}
	if !config.Enabled {
		t.Error("expected enabled by default")
	}
	if config.RotationInterval != 30*24*time.Hour {
		t.Errorf("expected 30 day rotation interval, got %v", config.RotationInterval)
	}
	if config.GracePeriod != 24*time.Hour {
		t.Errorf("expected 24 hour grace period, got %v", config.GracePeriod)
	}
	if config.SecretLength != 32 {
		t.Errorf("expected secret length 32, got %d", config.SecretLength)
	}
	if config.MaxVersions != 5 {
		t.Errorf("expected max versions 5, got %d", config.MaxVersions)
	}
}

func TestNewSecretRotator(t *testing.T) {
	store := NewInMemorySecretStore()
	config := DefaultRotationConfig()

	rotator := NewSecretRotator(config, store)

	if rotator == nil {
		t.Fatal("expected non-nil rotator")
	}
	if rotator.config != config {
		t.Error("expected config to be set")
	}
	if rotator.store != store {
		t.Error("expected store to be set")
	}
}

func TestNewSecretRotator_NilConfig(t *testing.T) {
	store := NewInMemorySecretStore()
	rotator := NewSecretRotator(nil, store)

	if rotator == nil {
		t.Fatal("expected non-nil rotator")
	}
	if rotator.config == nil {
		t.Error("expected default config to be created")
	}
}

func TestSecretRotator_CreateInitialSecret(t *testing.T) {
	store := NewInMemorySecretStore()
	rotator := NewSecretRotator(DefaultRotationConfig(), store)

	ctx := context.Background()
	version, err := rotator.CreateInitialSecret(ctx, "webhook-1")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if version == nil {
		t.Fatal("expected non-nil version")
	}
	if version.Version != 1 {
		t.Errorf("expected version 1, got %d", version.Version)
	}
	if version.Status != SecretStatusActive {
		t.Errorf("expected active status, got %s", version.Status)
	}
	if version.Secret == "" {
		t.Error("expected non-empty secret")
	}
	if version.ID == "" {
		t.Error("expected non-empty ID")
	}

	// Verify stored
	active, _ := store.GetActiveSecret(ctx, "webhook-1")
	if active == nil {
		t.Error("expected active secret in store")
	}
}

func TestSecretRotator_RotateSecret(t *testing.T) {
	store := NewInMemorySecretStore()
	rotator := NewSecretRotator(DefaultRotationConfig(), store)

	ctx := context.Background()

	// Create initial secret
	initial, _ := rotator.CreateInitialSecret(ctx, "webhook-1")

	// Rotate
	newVersion, err := rotator.RotateSecret(ctx, "webhook-1")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if newVersion == nil {
		t.Fatal("expected non-nil version")
	}
	if newVersion.Version != 2 {
		t.Errorf("expected version 2, got %d", newVersion.Version)
	}
	if newVersion.Secret == initial.Secret {
		t.Error("expected different secret after rotation")
	}

	// Verify old secret is in grace period
	versions, _ := store.GetVersions(ctx, "webhook-1")
	foundGracePeriod := false
	for _, v := range versions {
		if v.ID == initial.ID && v.Status == SecretStatusGracePeriod {
			foundGracePeriod = true
			break
		}
	}
	if !foundGracePeriod {
		t.Error("expected old secret to be in grace period")
	}

	// Verify valid secrets includes both
	valid, _ := store.GetValidSecrets(ctx, "webhook-1")
	if len(valid) != 2 {
		t.Errorf("expected 2 valid secrets during grace period, got %d", len(valid))
	}
}

func TestSecretRotator_RevokeSecret(t *testing.T) {
	store := NewInMemorySecretStore()
	rotator := NewSecretRotator(DefaultRotationConfig(), store)

	ctx := context.Background()
	version, _ := rotator.CreateInitialSecret(ctx, "webhook-1")

	err := rotator.RevokeSecret(ctx, "webhook-1", version.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify revoked
	versions, _ := store.GetVersions(ctx, "webhook-1")
	if len(versions) > 0 && versions[0].Status != SecretStatusRevoked {
		t.Error("expected secret to be revoked")
	}
}

func TestSecretRotator_OnRotation(t *testing.T) {
	store := NewInMemorySecretStore()
	rotator := NewSecretRotator(DefaultRotationConfig(), store)

	var receivedEvent *RotationEvent
	rotator.OnRotation(func(event *RotationEvent) {
		receivedEvent = event
	})

	ctx := context.Background()
	rotator.CreateInitialSecret(ctx, "webhook-1")

	if receivedEvent == nil {
		t.Error("expected rotation event callback to be called")
	}
	if receivedEvent.Type != RotationEventCreated {
		t.Errorf("expected event type 'created', got '%s'", receivedEvent.Type)
	}
	if receivedEvent.WebhookID != "webhook-1" {
		t.Errorf("expected webhook ID 'webhook-1', got '%s'", receivedEvent.WebhookID)
	}
}

func TestSecretRotator_GetRotationStatus(t *testing.T) {
	store := NewInMemorySecretStore()
	rotator := NewSecretRotator(DefaultRotationConfig(), store)

	ctx := context.Background()
	rotator.CreateInitialSecret(ctx, "webhook-1")

	status, err := rotator.GetRotationStatus(ctx, "webhook-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if status == nil {
		t.Fatal("expected non-nil status")
	}
	if status.WebhookID != "webhook-1" {
		t.Errorf("expected webhook ID 'webhook-1', got '%s'", status.WebhookID)
	}
	if status.ActiveVersion == nil {
		t.Error("expected active version")
	}
	if status.ValidVersions != 1 {
		t.Errorf("expected 1 valid version, got %d", status.ValidVersions)
	}
	if status.NextRotation.IsZero() {
		t.Error("expected next rotation time to be set")
	}
}

func TestSecretRotator_ForceRotation(t *testing.T) {
	store := NewInMemorySecretStore()
	rotator := NewSecretRotator(DefaultRotationConfig(), store)

	ctx := context.Background()
	rotator.CreateInitialSecret(ctx, "webhook-1")

	newVersion, err := rotator.ForceRotation(ctx, "webhook-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if newVersion.Version != 2 {
		t.Errorf("expected version 2 after force rotation, got %d", newVersion.Version)
	}
}

func TestSecretRotator_UpdateConfig(t *testing.T) {
	store := NewInMemorySecretStore()
	rotator := NewSecretRotator(DefaultRotationConfig(), store)

	newConfig := &RotationConfig{
		RotationInterval: 7 * 24 * time.Hour,
		GracePeriod:      12 * time.Hour,
	}

	rotator.UpdateConfig(newConfig)

	if rotator.GetConfig().RotationInterval != 7*24*time.Hour {
		t.Error("expected config to be updated")
	}
}

func TestInMemorySecretStore_Basic(t *testing.T) {
	store := NewInMemorySecretStore()
	ctx := context.Background()

	// Create version
	version := &SecretVersion{
		ID:        "v-1",
		Version:   1,
		Secret:    "secret-value",
		CreatedAt: time.Now(),
		Status:    SecretStatusActive,
	}

	err := store.SaveVersion(ctx, "webhook-1", version)
	if err != nil {
		t.Fatalf("unexpected error saving: %v", err)
	}

	// Get versions
	versions, err := store.GetVersions(ctx, "webhook-1")
	if err != nil {
		t.Fatalf("unexpected error getting versions: %v", err)
	}
	if len(versions) != 1 {
		t.Errorf("expected 1 version, got %d", len(versions))
	}

	// Get active
	active, err := store.GetActiveSecret(ctx, "webhook-1")
	if err != nil {
		t.Fatalf("unexpected error getting active: %v", err)
	}
	if active == nil {
		t.Error("expected active secret")
	}

	// Update status
	err = store.UpdateVersionStatus(ctx, "webhook-1", "v-1", SecretStatusGracePeriod)
	if err != nil {
		t.Fatalf("unexpected error updating status: %v", err)
	}

	// Now no active secret
	active, _ = store.GetActiveSecret(ctx, "webhook-1")
	if active != nil {
		t.Error("expected no active secret after status update")
	}

	// Delete
	err = store.DeleteVersion(ctx, "webhook-1", "v-1")
	if err != nil {
		t.Fatalf("unexpected error deleting: %v", err)
	}

	versions, _ = store.GetVersions(ctx, "webhook-1")
	if len(versions) != 0 {
		t.Errorf("expected 0 versions after delete, got %d", len(versions))
	}
}

func TestInMemorySecretStore_ListWebhooks(t *testing.T) {
	store := NewInMemorySecretStore()
	ctx := context.Background()

	store.SaveVersion(ctx, "webhook-1", &SecretVersion{ID: "v-1", Status: SecretStatusActive})
	store.SaveVersion(ctx, "webhook-2", &SecretVersion{ID: "v-2", Status: SecretStatusActive})

	webhooks, err := store.ListWebhooks(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(webhooks) != 2 {
		t.Errorf("expected 2 webhooks, got %d", len(webhooks))
	}
}

func TestRotatingAuthenticator(t *testing.T) {
	store := NewInMemorySecretStore()
	rotator := NewSecretRotator(DefaultRotationConfig(), store)
	ctx := context.Background()

	// Create initial secret
	version, _ := rotator.CreateInitialSecret(ctx, "webhook-1")

	auth := NewRotatingAuthenticator(rotator, "webhook-1")

	// Create valid request
	body := []byte(`{"event": "test"}`)
	mac := hmac.New(sha256.New, []byte(version.Secret))
	mac.Write(body)
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest("POST", "/webhook", strings.NewReader(string(body)))
	req.Header.Set("X-Hub-Signature-256", signature)

	err := auth.Authenticate(req, body)
	if err != nil {
		t.Errorf("unexpected authentication error: %v", err)
	}
}

func TestRotatingAuthenticator_InvalidSignature(t *testing.T) {
	store := NewInMemorySecretStore()
	rotator := NewSecretRotator(DefaultRotationConfig(), store)
	ctx := context.Background()

	rotator.CreateInitialSecret(ctx, "webhook-1")

	auth := NewRotatingAuthenticator(rotator, "webhook-1")

	body := []byte(`{"event": "test"}`)
	req := httptest.NewRequest("POST", "/webhook", strings.NewReader(string(body)))
	req.Header.Set("X-Hub-Signature-256", "sha256=invalid")

	err := auth.Authenticate(req, body)
	if err == nil {
		t.Error("expected authentication error for invalid signature")
	}
}

func TestRotatingAuthenticator_GracePeriod(t *testing.T) {
	store := NewInMemorySecretStore()
	rotator := NewSecretRotator(DefaultRotationConfig(), store)
	ctx := context.Background()

	// Create and rotate
	oldVersion, _ := rotator.CreateInitialSecret(ctx, "webhook-1")
	rotator.RotateSecret(ctx, "webhook-1")

	// Old secret should still work during grace period
	auth := NewRotatingAuthenticator(rotator, "webhook-1")

	body := []byte(`{"event": "test"}`)
	mac := hmac.New(sha256.New, []byte(oldVersion.Secret))
	mac.Write(body)
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest("POST", "/webhook", strings.NewReader(string(body)))
	req.Header.Set("X-Hub-Signature-256", signature)

	err := auth.Authenticate(req, body)
	if err != nil {
		t.Errorf("expected old secret to work during grace period: %v", err)
	}
}

func TestRotatingAuthenticator_NoSecrets(t *testing.T) {
	store := NewInMemorySecretStore()
	rotator := NewSecretRotator(DefaultRotationConfig(), store)

	auth := NewRotatingAuthenticator(rotator, "webhook-1")

	body := []byte(`{"event": "test"}`)
	req := httptest.NewRequest("POST", "/webhook", strings.NewReader(string(body)))
	req.Header.Set("X-Hub-Signature-256", "sha256=something")

	err := auth.Authenticate(req, body)
	if err == nil {
		t.Error("expected error when no secrets configured")
	}
}

func TestRotatingAuthenticator_WithHeader(t *testing.T) {
	store := NewInMemorySecretStore()
	rotator := NewSecretRotator(DefaultRotationConfig(), store)
	ctx := context.Background()

	version, _ := rotator.CreateInitialSecret(ctx, "webhook-1")

	auth := NewRotatingAuthenticator(rotator, "webhook-1").WithHeader("X-Custom-Signature")

	body := []byte(`{"event": "test"}`)
	mac := hmac.New(sha256.New, []byte(version.Secret))
	mac.Write(body)
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest("POST", "/webhook", strings.NewReader(string(body)))
	req.Header.Set("X-Custom-Signature", signature)

	err := auth.Authenticate(req, body)
	if err != nil {
		t.Errorf("unexpected error with custom header: %v", err)
	}
}

func TestSecretRotator_StartStop(t *testing.T) {
	store := NewInMemorySecretStore()
	config := DefaultRotationConfig()
	config.Enabled = true

	rotator := NewSecretRotator(config, store)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := rotator.Start(ctx)
	if err != nil {
		t.Fatalf("unexpected error starting: %v", err)
	}

	// Stop should not hang
	done := make(chan struct{})
	go func() {
		rotator.Stop()
		close(done)
	}()

	select {
	case <-done:
		// OK
	case <-time.After(1 * time.Second):
		t.Error("Stop() timed out")
	}
}

func TestSecretRotator_Start_Disabled(t *testing.T) {
	store := NewInMemorySecretStore()
	config := DefaultRotationConfig()
	config.Enabled = false

	rotator := NewSecretRotator(config, store)

	ctx := context.Background()
	err := rotator.Start(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Stop should work even if not started
	rotator.Stop()
}

func TestSecretVersion_Fields(t *testing.T) {
	now := time.Now()
	version := &SecretVersion{
		ID:        "v-1",
		Version:   1,
		Secret:    "test-secret",
		CreatedAt: now,
		ExpiresAt: now.Add(24 * time.Hour),
		Status:    SecretStatusActive,
		Metadata:  map[string]string{"key": "value"},
	}

	if version.ID != "v-1" {
		t.Errorf("expected ID 'v-1', got '%s'", version.ID)
	}
	if version.Version != 1 {
		t.Errorf("expected version 1, got %d", version.Version)
	}
	if version.Metadata["key"] != "value" {
		t.Error("expected metadata to be preserved")
	}
}

func TestSecretStatus_Values(t *testing.T) {
	statuses := []SecretStatus{
		SecretStatusActive,
		SecretStatusPending,
		SecretStatusGracePeriod,
		SecretStatusExpired,
		SecretStatusRevoked,
	}

	for _, s := range statuses {
		if s == "" {
			t.Error("expected non-empty status value")
		}
	}
}

func TestRotationEventType_Values(t *testing.T) {
	types := []RotationEventType{
		RotationEventCreated,
		RotationEventRotated,
		RotationEventExpired,
		RotationEventRevoked,
		RotationEventGracePeriodEnd,
		RotationEventExpiryWarning,
	}

	for _, typ := range types {
		if typ == "" {
			t.Error("expected non-empty event type value")
		}
	}
}

func TestRotationEvent_Fields(t *testing.T) {
	event := &RotationEvent{
		Type:            RotationEventRotated,
		WebhookID:       "webhook-1",
		SecretVersionID: "v-1",
		Version:         2,
		Timestamp:       time.Now(),
		Details: map[string]interface{}{
			"previous_version": 1,
		},
	}

	if event.Type != RotationEventRotated {
		t.Error("expected rotated type")
	}
	if event.Details["previous_version"] != 1 {
		t.Error("expected details to be preserved")
	}
}

func BenchmarkSecretGeneration(b *testing.B) {
	store := NewInMemorySecretStore()
	rotator := NewSecretRotator(DefaultRotationConfig(), store)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rotator.generateSecret()
	}
}

func BenchmarkAuthentication(b *testing.B) {
	store := NewInMemorySecretStore()
	rotator := NewSecretRotator(DefaultRotationConfig(), store)
	ctx := context.Background()

	version, _ := rotator.CreateInitialSecret(ctx, "webhook-1")
	auth := NewRotatingAuthenticator(rotator, "webhook-1")

	body := []byte(`{"event": "test", "data": "some payload"}`)
	mac := hmac.New(sha256.New, []byte(version.Secret))
	mac.Write(body)
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("POST", "/webhook", nil)
		req.Header.Set("X-Hub-Signature-256", signature)
		auth.Authenticate(req, body)
	}
}
