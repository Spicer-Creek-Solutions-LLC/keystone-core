package nats

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/testing/helpers"
)

func TestDefaultBootstrapHandlerConfig(t *testing.T) {
	tests := []struct {
		name     string
		cluster  string
		serverID string
	}{
		{
			name:     "with cluster and server",
			cluster:  "test-cluster",
			serverID: "server-1",
		},
		{
			name:     "empty cluster uses default",
			cluster:  "",
			serverID: "server-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultBootstrapHandlerConfig(tt.cluster, tt.serverID)
			if config == nil {
				t.Fatal("DefaultBootstrapHandlerConfig returned nil")
			}
			if tt.cluster == "" && config.Cluster != DefaultCluster {
				t.Errorf("Cluster = %s, want %s", config.Cluster, DefaultCluster)
			}
			if tt.cluster != "" && config.Cluster != tt.cluster {
				t.Errorf("Cluster = %s, want %s", config.Cluster, tt.cluster)
			}
			if config.ServerID != tt.serverID {
				t.Errorf("ServerID = %s, want %s", config.ServerID, tt.serverID)
			}
		})
	}
}

func TestNewBootstrapRegistrationHandler(t *testing.T) {
	provider := NewInMemoryBootstrapProvider("test-issuer")

	tests := []struct {
		name     string
		config   *BootstrapHandlerConfig
		provider BootstrapCredentialProvider
		wantErr  bool
	}{
		{
			name:     "valid config and provider",
			config:   DefaultBootstrapHandlerConfig("test-cluster", "server-1"),
			provider: provider,
			wantErr:  false,
		},
		{
			name:     "nil config",
			config:   nil,
			provider: provider,
			wantErr:  true,
		},
		{
			name:     "nil provider",
			config:   DefaultBootstrapHandlerConfig("test-cluster", "server-1"),
			provider: nil,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, err := NewBootstrapRegistrationHandler(tt.config, tt.provider)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewBootstrapRegistrationHandler() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && handler == nil {
				t.Error("NewBootstrapRegistrationHandler() returned nil handler")
			}
		})
	}
}

func TestBootstrapRegistrationHandler_SetCredentialIssuer(t *testing.T) {
	provider := NewInMemoryBootstrapProvider("test-issuer")
	config := DefaultBootstrapHandlerConfig("test-cluster", "server-1")

	handler, err := NewBootstrapRegistrationHandler(config, provider)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	// Set issuer
	issuer := &mockCredentialIssuer{}
	handler.SetCredentialIssuer(issuer)

	// Verify it was set (indirectly by checking it doesn't panic)
	handler.mu.RLock()
	if handler.credentialIssuer != issuer {
		t.Error("credential issuer was not set")
	}
	handler.mu.RUnlock()
}

func TestBootstrapRegistrationHandler_AddIdentityVerifier(t *testing.T) {
	provider := NewInMemoryBootstrapProvider("test-issuer")
	config := DefaultBootstrapHandlerConfig("test-cluster", "server-1")

	handler, err := NewBootstrapRegistrationHandler(config, provider)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	// Add verifiers
	verifier1 := &mockIdentityVerifier{verifierType: "type1"}
	verifier2 := &mockIdentityVerifier{verifierType: "type2"}

	handler.AddIdentityVerifier(verifier1)
	handler.AddIdentityVerifier(verifier2)

	handler.mu.RLock()
	if len(handler.identityVerifiers) != 2 {
		t.Errorf("expected 2 verifiers, got %d", len(handler.identityVerifiers))
	}
	handler.mu.RUnlock()
}

func TestBootstrapRegistrationHandler_SetAuditLogger(t *testing.T) {
	provider := NewInMemoryBootstrapProvider("test-issuer")
	config := DefaultBootstrapHandlerConfig("test-cluster", "server-1")

	handler, err := NewBootstrapRegistrationHandler(config, provider)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	logger := &mockBootstrapAuditLogger{}
	handler.SetAuditLogger(logger)

	handler.mu.RLock()
	if handler.auditLogger != logger {
		t.Error("audit logger was not set")
	}
	handler.mu.RUnlock()
}

func TestBootstrapRegistrationHandler_SetRegistrationCallback(t *testing.T) {
	provider := NewInMemoryBootstrapProvider("test-issuer")
	config := DefaultBootstrapHandlerConfig("test-cluster", "server-1")

	handler, err := NewBootstrapRegistrationHandler(config, provider)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	callbackCalled := false
	callback := func(ctx context.Context, agentID string, labels map[string]string, credentials *IssuedCredentials) error {
		callbackCalled = true
		return nil
	}

	handler.SetRegistrationCallback(callback)

	handler.mu.RLock()
	if handler.registrationCallback == nil {
		t.Error("registration callback was not set")
	}
	handler.mu.RUnlock()

	// Verify callback is set by checking it doesn't panic on access
	_ = callbackCalled
}

func TestBootstrapRegistrationHandler_GenerateBootstrapCredential(t *testing.T) {
	ctx := context.Background()
	provider := NewInMemoryBootstrapProvider("test-issuer")
	config := DefaultBootstrapHandlerConfig("test-cluster", "server-1")

	handler, err := NewBootstrapRegistrationHandler(config, provider)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	cred, err := handler.GenerateBootstrapCredential(ctx, BootstrapCredentialRequest{
		AllowedAgentID: "agent-123",
		MaxUses:        1,
	})
	if err != nil {
		t.Fatalf("GenerateBootstrapCredential() error = %v", err)
	}

	if cred == nil {
		t.Fatal("GenerateBootstrapCredential() returned nil credential")
	}
	if cred.Claims.ID == "" {
		t.Error("credential ID is empty")
	}
	if cred.Claims.Cluster != "test-cluster" {
		t.Errorf("Cluster = %s, want test-cluster", cred.Claims.Cluster)
	}
	if cred.Claims.AllowedAgentID != "agent-123" {
		t.Errorf("AllowedAgentID = %s, want agent-123", cred.Claims.AllowedAgentID)
	}
}

func TestBootstrapRegistrationHandler_RevokeBootstrapCredential(t *testing.T) {
	ctx := context.Background()
	provider := NewInMemoryBootstrapProvider("test-issuer")
	config := DefaultBootstrapHandlerConfig("test-cluster", "server-1")

	handler, err := NewBootstrapRegistrationHandler(config, provider)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	// Generate a credential
	cred, err := handler.GenerateBootstrapCredential(ctx, BootstrapCredentialRequest{})
	if err != nil {
		t.Fatalf("failed to generate credential: %v", err)
	}

	// Revoke it
	err = handler.RevokeBootstrapCredential(ctx, cred.Claims.ID, "test revocation")
	if err != nil {
		t.Fatalf("RevokeBootstrapCredential() error = %v", err)
	}

	// Verify it's revoked
	status, err := handler.GetCredentialStatus(ctx, cred.Claims.ID)
	if err != nil {
		t.Fatalf("GetCredentialStatus() error = %v", err)
	}
	if !status.Revoked {
		t.Error("credential should be revoked")
	}
}

func TestBootstrapRegistrationHandler_ListActiveCredentials(t *testing.T) {
	ctx := context.Background()
	provider := NewInMemoryBootstrapProvider("test-issuer")
	config := DefaultBootstrapHandlerConfig("test-cluster", "server-1")

	handler, err := NewBootstrapRegistrationHandler(config, provider)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	// Generate some credentials
	_, _ = handler.GenerateBootstrapCredential(ctx, BootstrapCredentialRequest{})
	_, _ = handler.GenerateBootstrapCredential(ctx, BootstrapCredentialRequest{})

	// List active
	active, err := handler.ListActiveCredentials(ctx)
	if err != nil {
		t.Fatalf("ListActiveCredentials() error = %v", err)
	}

	if len(active) != 2 {
		t.Errorf("expected 2 active credentials, got %d", len(active))
	}
}

func TestBootstrapRegistrationHandler_CleanupExpiredCredentials(t *testing.T) {
	ctx := context.Background()
	provider := NewInMemoryBootstrapProvider("test-issuer")
	config := DefaultBootstrapHandlerConfig("test-cluster", "server-1")

	handler, err := NewBootstrapRegistrationHandler(config, provider)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	// Generate a short-lived credential
	_, _ = handler.GenerateBootstrapCredential(ctx, BootstrapCredentialRequest{
		TTL: 1 * time.Millisecond,
	})

	// Generate a normal credential
	_, _ = handler.GenerateBootstrapCredential(ctx, BootstrapCredentialRequest{
		TTL: 5 * time.Minute,
	})

	// Wait for short-lived to expire
	start := time.Now()
	if err := helpers.WaitForTimeout(2*time.Second, 1*time.Millisecond, func() (bool, error) {
		return time.Since(start) >= 5*time.Millisecond, nil
	}); err != nil {
		t.Fatalf("expiry wait did not elapse: %v", err)
	}

	// Cleanup
	removed, err := handler.CleanupExpiredCredentials(ctx)
	if err != nil {
		t.Fatalf("CleanupExpiredCredentials() error = %v", err)
	}

	if removed != 1 {
		t.Errorf("expected 1 removed, got %d", removed)
	}
}

func TestBootstrapRegistrationHandler_Stop(t *testing.T) {
	provider := NewInMemoryBootstrapProvider("test-issuer")
	config := DefaultBootstrapHandlerConfig("test-cluster", "server-1")

	handler, err := NewBootstrapRegistrationHandler(config, provider)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	// Stop without starting should not panic
	err = handler.Stop()
	if err != nil {
		t.Errorf("Stop() error = %v", err)
	}
}

func TestBootstrapRegistrationHandler_generateDefaultCredentials(t *testing.T) {
	provider := NewInMemoryBootstrapProvider("test-issuer")
	config := DefaultBootstrapHandlerConfig("test-cluster", "server-1")
	config.CredentialTTL = 1 * time.Hour

	handler, err := NewBootstrapRegistrationHandler(config, provider)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	creds, err := handler.generateDefaultCredentials("agent-123", "test-cluster")
	if err != nil {
		t.Fatalf("generateDefaultCredentials() error = %v", err)
	}

	if creds.AgentID != "agent-123" {
		t.Errorf("AgentID = %s, want agent-123", creds.AgentID)
	}
	if creds.NKeyPublicKey == "" {
		t.Error("NKeyPublicKey is empty")
	}
	if creds.NKeyPrivateKey == "" {
		t.Error("NKeyPrivateKey is empty")
	}
	if len(creds.Subjects.Publish) == 0 {
		t.Error("Publish subjects is empty")
	}
	if len(creds.Subjects.Subscribe) == 0 {
		t.Error("Subscribe subjects is empty")
	}
	if creds.ExpiresAt == nil {
		t.Error("ExpiresAt should be set when CredentialTTL is configured")
	}
}

func TestBootstrapRegistrationRequest_JSON(t *testing.T) {
	req := BootstrapRegistrationRequest{
		BootstrapCredential: &BootstrapCredential{
			Type: BootstrapCredentialTypeNKey,
			Claims: BootstrapClaims{
				ID:      "bootstrap-123",
				Cluster: "test-cluster",
			},
			PublicKey: "UABC123",
		},
		AgentID: "agent-123",
		Labels: map[string]string{
			"env": "production",
		},
		Metadata: map[string]string{
			"version": "1.0.0",
		},
	}

	// Marshal
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	// Unmarshal
	var decoded BootstrapRegistrationRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.AgentID != req.AgentID {
		t.Errorf("AgentID = %s, want %s", decoded.AgentID, req.AgentID)
	}
	if decoded.BootstrapCredential.PublicKey != req.BootstrapCredential.PublicKey {
		t.Errorf("PublicKey = %s, want %s", decoded.BootstrapCredential.PublicKey, req.BootstrapCredential.PublicKey)
	}
}

func TestBootstrapRegistrationResponse_JSON(t *testing.T) {
	resp := BootstrapRegistrationResponse{
		Success: true,
		AgentID: "agent-123",
		Credentials: &IssuedCredentials{
			AgentID:       "agent-123",
			NKeyPublicKey: "UABC123",
		},
	}

	// Marshal
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	// Unmarshal
	var decoded BootstrapRegistrationResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if !decoded.Success {
		t.Error("Success should be true")
	}
	if decoded.AgentID != resp.AgentID {
		t.Errorf("AgentID = %s, want %s", decoded.AgentID, resp.AgentID)
	}
}

// Mock implementations for testing

type mockCredentialIssuer struct {
	issueResult *IssuedCredentials
	issueErr    error
}

func (m *mockCredentialIssuer) Issue(ctx context.Context, req CredentialIssueRequest) (*IssuedCredentials, error) {
	if m.issueErr != nil {
		return nil, m.issueErr
	}
	if m.issueResult != nil {
		return m.issueResult, nil
	}
	return &IssuedCredentials{
		AgentID:       req.AgentID,
		NKeyPublicKey: "mock-public-key",
	}, nil
}

func (m *mockCredentialIssuer) Revoke(ctx context.Context, agentID string, reason string) error {
	return nil
}

func (m *mockCredentialIssuer) Rotate(ctx context.Context, agentID string) (*IssuedCredentials, error) {
	return nil, nil
}

type mockIdentityVerifier struct {
	verifierType string
	verifyResult *IdentityVerificationResult
	verifyErr    error
}

func (m *mockIdentityVerifier) Verify(ctx context.Context, claims map[string]interface{}) (*IdentityVerificationResult, error) {
	if m.verifyErr != nil {
		return nil, m.verifyErr
	}
	if m.verifyResult != nil {
		return m.verifyResult, nil
	}
	return &IdentityVerificationResult{
		Verified:   true,
		Identity:   "verified-identity",
		TrustLevel: "high",
	}, nil
}

func (m *mockIdentityVerifier) Type() string {
	return m.verifierType
}

type mockBootstrapAuditLogger struct {
	events []BootstrapAuditEvent
}

func (m *mockBootstrapAuditLogger) Log(ctx context.Context, event BootstrapAuditEvent) error {
	m.events = append(m.events, event)
	return nil
}

func TestBootstrapRegistrationHandler_verifyIdentity(t *testing.T) {
	ctx := context.Background()
	provider := NewInMemoryBootstrapProvider("test-issuer")
	config := DefaultBootstrapHandlerConfig("test-cluster", "server-1")

	handler, err := NewBootstrapRegistrationHandler(config, provider)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	t.Run("no verifiers", func(t *testing.T) {
		_, err := handler.verifyIdentity(ctx, nil)
		if err == nil {
			t.Error("expected error when no verifiers configured")
		}
	})

	t.Run("successful verification", func(t *testing.T) {
		handler.AddIdentityVerifier(&mockIdentityVerifier{
			verifierType: "test",
			verifyResult: &IdentityVerificationResult{
				Verified:   true,
				Identity:   "test-identity",
				TrustLevel: "high",
			},
		})

		result, err := handler.verifyIdentity(ctx, map[string]interface{}{"test": "claim"})
		if err != nil {
			t.Fatalf("verifyIdentity() error = %v", err)
		}
		if !result.Verified {
			t.Error("expected verified=true")
		}
	})
}
