package nats

import (
	"context"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/testing/helpers"
)

func TestBootstrapClaims_IsExpired(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt time.Time
		want      bool
	}{
		{
			name:      "not expired",
			expiresAt: time.Now().Add(time.Hour),
			want:      false,
		},
		{
			name:      "expired",
			expiresAt: time.Now().Add(-time.Hour),
			want:      true,
		},
		{
			name:      "just expired",
			expiresAt: time.Now().Add(-time.Millisecond),
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &BootstrapClaims{
				ExpiresAt: tt.expiresAt,
			}
			if got := c.IsExpired(); got != tt.want {
				t.Errorf("IsExpired() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBootstrapClaims_TTL(t *testing.T) {
	expiresAt := time.Now().Add(5 * time.Minute)
	c := &BootstrapClaims{
		ExpiresAt: expiresAt,
	}

	ttl := c.TTL()
	// TTL should be roughly 5 minutes (allow for test execution time)
	if ttl < 4*time.Minute || ttl > 5*time.Minute+time.Second {
		t.Errorf("TTL() = %v, want approximately 5 minutes", ttl)
	}
}

func TestNewInMemoryBootstrapProvider(t *testing.T) {
	provider := NewInMemoryBootstrapProvider("test-issuer")
	if provider == nil {
		t.Fatal("NewInMemoryBootstrapProvider returned nil")
	}
	if provider.issuer != "test-issuer" {
		t.Errorf("issuer = %s, want test-issuer", provider.issuer)
	}
}

func TestInMemoryBootstrapProvider_Generate(t *testing.T) {
	ctx := context.Background()
	provider := NewInMemoryBootstrapProvider("test-issuer")

	tests := []struct {
		name    string
		req     BootstrapCredentialRequest
		wantErr bool
	}{
		{
			name: "default NKey credential",
			req: BootstrapCredentialRequest{
				Cluster: "test-cluster",
			},
			wantErr: false,
		},
		{
			name: "token credential",
			req: BootstrapCredentialRequest{
				Cluster: "test-cluster",
				Type:    BootstrapCredentialTypeToken,
			},
			wantErr: false,
		},
		{
			name: "JWT credential",
			req: BootstrapCredentialRequest{
				Cluster: "test-cluster",
				Type:    BootstrapCredentialTypeJWT,
			},
			wantErr: false,
		},
		{
			name: "with custom TTL",
			req: BootstrapCredentialRequest{
				Cluster: "test-cluster",
				TTL:     10 * time.Minute,
			},
			wantErr: false,
		},
		{
			name: "with max TTL exceeded",
			req: BootstrapCredentialRequest{
				Cluster: "test-cluster",
				TTL:     48 * time.Hour, // Exceeds MaxBootstrapTTL
			},
			wantErr: false, // Should clamp to MaxBootstrapTTL
		},
		{
			name: "with allowed agent ID",
			req: BootstrapCredentialRequest{
				Cluster:        "test-cluster",
				AllowedAgentID: "agent-123",
			},
			wantErr: false,
		},
		{
			name: "with allowed labels",
			req: BootstrapCredentialRequest{
				Cluster: "test-cluster",
				AllowedLabels: map[string]string{
					"env": "production",
				},
			},
			wantErr: false,
		},
		{
			name: "with max uses",
			req: BootstrapCredentialRequest{
				Cluster: "test-cluster",
				MaxUses: 1,
			},
			wantErr: false,
		},
		{
			name: "with empty cluster (uses default)",
			req: BootstrapCredentialRequest{
				Cluster: "",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cred, err := provider.Generate(ctx, tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("Generate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			// Verify credential
			if cred == nil {
				t.Fatal("Generate() returned nil credential")
			}
			if cred.Claims.ID == "" {
				t.Error("credential ID is empty")
			}
			if cred.Claims.IssuedAt.IsZero() {
				t.Error("IssuedAt is zero")
			}
			if cred.Claims.ExpiresAt.IsZero() {
				t.Error("ExpiresAt is zero")
			}
			if cred.Claims.ExpiresAt.Before(cred.Claims.IssuedAt) {
				t.Error("ExpiresAt is before IssuedAt")
			}
			if cred.Claims.Issuer != "test-issuer" {
				t.Errorf("Issuer = %s, want test-issuer", cred.Claims.Issuer)
			}

			// Check type-specific fields
			switch cred.Type {
			case BootstrapCredentialTypeNKey:
				if cred.PublicKey == "" {
					t.Error("NKey credential missing public key")
				}
				if cred.PrivateKey == "" {
					t.Error("NKey credential missing private key")
				}
			case BootstrapCredentialTypeToken:
				if cred.Token == "" {
					t.Error("Token credential missing token")
				}
			case BootstrapCredentialTypeJWT:
				if cred.JWT == "" {
					t.Error("JWT credential missing JWT")
				}
			}

			// Check subject permissions
			if len(cred.NATSSubjects.Publish) == 0 {
				t.Error("credential has no publish subjects")
			}
			if len(cred.NATSSubjects.Subscribe) == 0 {
				t.Error("credential has no subscribe subjects")
			}
		})
	}
}

func TestInMemoryBootstrapProvider_Validate(t *testing.T) {
	ctx := context.Background()
	provider := NewInMemoryBootstrapProvider("test-issuer")

	// Generate a valid credential
	cred, err := provider.Generate(ctx, BootstrapCredentialRequest{
		Cluster: "test-cluster",
	})
	if err != nil {
		t.Fatalf("failed to generate credential: %v", err)
	}

	tests := []struct {
		name       string
		credential *BootstrapCredential
		wantValid  bool
		wantErr    error
	}{
		{
			name:       "valid credential",
			credential: cred,
			wantValid:  true,
			wantErr:    nil,
		},
		{
			name:       "nil credential",
			credential: nil,
			wantValid:  false,
			wantErr:    ErrBootstrapInvalid,
		},
		{
			name: "unknown credential",
			credential: &BootstrapCredential{
				Type: BootstrapCredentialTypeNKey,
				Claims: BootstrapClaims{
					ID: "unknown-id",
				},
			},
			wantValid: false,
			wantErr:   ErrBootstrapNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := provider.Validate(ctx, tt.credential)
			if err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
			if result.Valid != tt.wantValid {
				t.Errorf("Validate() valid = %v, want %v", result.Valid, tt.wantValid)
			}
			if tt.wantErr != nil && result.Error != tt.wantErr {
				t.Errorf("Validate() error = %v, want %v", result.Error, tt.wantErr)
			}
		})
	}
}

func TestInMemoryBootstrapProvider_Validate_Revoked(t *testing.T) {
	ctx := context.Background()
	provider := NewInMemoryBootstrapProvider("test-issuer")

	// Generate a credential
	cred, err := provider.Generate(ctx, BootstrapCredentialRequest{
		Cluster: "test-cluster",
	})
	if err != nil {
		t.Fatalf("failed to generate credential: %v", err)
	}

	// Revoke it
	err = provider.Revoke(ctx, cred.Claims.ID, "test revocation")
	if err != nil {
		t.Fatalf("failed to revoke credential: %v", err)
	}

	// Validate should fail
	result, err := provider.Validate(ctx, cred)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if result.Valid {
		t.Error("Validate() should return invalid for revoked credential")
	}
	if result.Error != ErrBootstrapRevoked {
		t.Errorf("Validate() error = %v, want ErrBootstrapRevoked", result.Error)
	}
}

func TestInMemoryBootstrapProvider_Validate_MaxUses(t *testing.T) {
	ctx := context.Background()
	provider := NewInMemoryBootstrapProvider("test-issuer")

	// Generate a credential with max uses = 1
	cred, err := provider.Generate(ctx, BootstrapCredentialRequest{
		Cluster: "test-cluster",
		MaxUses: 1,
	})
	if err != nil {
		t.Fatalf("failed to generate credential: %v", err)
	}

	// First validation should pass
	result, err := provider.Validate(ctx, cred)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if !result.Valid {
		t.Error("first Validate() should pass")
	}

	// Record use
	err = provider.RecordUse(ctx, cred.Claims.ID, "agent-1")
	if err != nil {
		t.Fatalf("RecordUse() error = %v", err)
	}

	// Second validation should fail
	result, err = provider.Validate(ctx, cred)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if result.Valid {
		t.Error("Validate() should fail after max uses exceeded")
	}
	if result.Error != ErrBootstrapAlreadyUsed {
		t.Errorf("Validate() error = %v, want ErrBootstrapAlreadyUsed", result.Error)
	}
}

func TestInMemoryBootstrapProvider_Revoke(t *testing.T) {
	ctx := context.Background()
	provider := NewInMemoryBootstrapProvider("test-issuer")

	// Generate a credential
	cred, err := provider.Generate(ctx, BootstrapCredentialRequest{
		Cluster: "test-cluster",
	})
	if err != nil {
		t.Fatalf("failed to generate credential: %v", err)
	}

	// Revoke it
	err = provider.Revoke(ctx, cred.Claims.ID, "test revocation")
	if err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}

	// Get status
	status, err := provider.GetStatus(ctx, cred.Claims.ID)
	if err != nil {
		t.Fatalf("GetStatus() error = %v", err)
	}
	if !status.Revoked {
		t.Error("credential should be revoked")
	}
	if status.RevokedReason != "test revocation" {
		t.Errorf("RevokedReason = %s, want 'test revocation'", status.RevokedReason)
	}
	if status.RevokedAt == nil {
		t.Error("RevokedAt should be set")
	}
}

func TestInMemoryBootstrapProvider_Revoke_NotFound(t *testing.T) {
	ctx := context.Background()
	provider := NewInMemoryBootstrapProvider("test-issuer")

	err := provider.Revoke(ctx, "non-existent-id", "reason")
	if err != ErrBootstrapNotFound {
		t.Errorf("Revoke() error = %v, want ErrBootstrapNotFound", err)
	}
}

func TestInMemoryBootstrapProvider_RecordUse(t *testing.T) {
	ctx := context.Background()
	provider := NewInMemoryBootstrapProvider("test-issuer")

	// Generate a credential
	cred, err := provider.Generate(ctx, BootstrapCredentialRequest{
		Cluster: "test-cluster",
	})
	if err != nil {
		t.Fatalf("failed to generate credential: %v", err)
	}

	// Record use
	err = provider.RecordUse(ctx, cred.Claims.ID, "agent-1")
	if err != nil {
		t.Fatalf("RecordUse() error = %v", err)
	}

	// Get status
	status, err := provider.GetStatus(ctx, cred.Claims.ID)
	if err != nil {
		t.Fatalf("GetStatus() error = %v", err)
	}
	if status.UseCount != 1 {
		t.Errorf("UseCount = %d, want 1", status.UseCount)
	}
	if len(status.UsedByAgents) != 1 || status.UsedByAgents[0] != "agent-1" {
		t.Errorf("UsedByAgents = %v, want [agent-1]", status.UsedByAgents)
	}
	if status.LastUsedAt == nil {
		t.Error("LastUsedAt should be set")
	}
}

func TestInMemoryBootstrapProvider_RecordUse_NotFound(t *testing.T) {
	ctx := context.Background()
	provider := NewInMemoryBootstrapProvider("test-issuer")

	err := provider.RecordUse(ctx, "non-existent-id", "agent-1")
	if err != ErrBootstrapNotFound {
		t.Errorf("RecordUse() error = %v, want ErrBootstrapNotFound", err)
	}
}

func TestInMemoryBootstrapProvider_GetStatus(t *testing.T) {
	ctx := context.Background()
	provider := NewInMemoryBootstrapProvider("test-issuer")

	// Generate a credential
	cred, err := provider.Generate(ctx, BootstrapCredentialRequest{
		Cluster:        "test-cluster",
		AllowedAgentID: "expected-agent",
		MaxUses:        3,
	})
	if err != nil {
		t.Fatalf("failed to generate credential: %v", err)
	}

	// Get status
	status, err := provider.GetStatus(ctx, cred.Claims.ID)
	if err != nil {
		t.Fatalf("GetStatus() error = %v", err)
	}

	if status.ID != cred.Claims.ID {
		t.Errorf("ID = %s, want %s", status.ID, cred.Claims.ID)
	}
	if status.Claims.AllowedAgentID != "expected-agent" {
		t.Errorf("AllowedAgentID = %s, want expected-agent", status.Claims.AllowedAgentID)
	}
	if status.Claims.MaxUses != 3 {
		t.Errorf("MaxUses = %d, want 3", status.Claims.MaxUses)
	}
	if status.UseCount != 0 {
		t.Errorf("UseCount = %d, want 0", status.UseCount)
	}
	if status.Revoked {
		t.Error("credential should not be revoked")
	}
}

func TestInMemoryBootstrapProvider_GetStatus_NotFound(t *testing.T) {
	ctx := context.Background()
	provider := NewInMemoryBootstrapProvider("test-issuer")

	_, err := provider.GetStatus(ctx, "non-existent-id")
	if err != ErrBootstrapNotFound {
		t.Errorf("GetStatus() error = %v, want ErrBootstrapNotFound", err)
	}
}

func TestInMemoryBootstrapProvider_ListActive(t *testing.T) {
	ctx := context.Background()
	provider := NewInMemoryBootstrapProvider("test-issuer")

	// Generate some credentials
	cred1, _ := provider.Generate(ctx, BootstrapCredentialRequest{Cluster: "test-cluster"})
	cred2, _ := provider.Generate(ctx, BootstrapCredentialRequest{Cluster: "test-cluster"})
	cred3, _ := provider.Generate(ctx, BootstrapCredentialRequest{Cluster: "test-cluster"})

	// Revoke one
	_ = provider.Revoke(ctx, cred2.Claims.ID, "test")

	// List active
	active, err := provider.ListActive(ctx)
	if err != nil {
		t.Fatalf("ListActive() error = %v", err)
	}

	// Should have 2 active credentials
	if len(active) != 2 {
		t.Errorf("ListActive() returned %d credentials, want 2", len(active))
	}

	// Check that the revoked one is not in the list
	for _, status := range active {
		if status.ID == cred2.Claims.ID {
			t.Error("revoked credential should not be in active list")
		}
	}

	// Verify the active ones are cred1 and cred3
	hasActive := make(map[string]bool)
	for _, status := range active {
		hasActive[status.ID] = true
	}
	if !hasActive[cred1.Claims.ID] {
		t.Error("cred1 should be in active list")
	}
	if !hasActive[cred3.Claims.ID] {
		t.Error("cred3 should be in active list")
	}
}

func TestInMemoryBootstrapProvider_Cleanup(t *testing.T) {
	ctx := context.Background()
	provider := NewInMemoryBootstrapProvider("test-issuer")

	// Generate a credential with very short TTL
	_, err := provider.Generate(ctx, BootstrapCredentialRequest{
		Cluster: "test-cluster",
		TTL:     1 * time.Millisecond, // Very short TTL
	})
	if err != nil {
		t.Fatalf("failed to generate credential: %v", err)
	}

	// Generate another with normal TTL
	_, err = provider.Generate(ctx, BootstrapCredentialRequest{
		Cluster: "test-cluster",
		TTL:     5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("failed to generate credential: %v", err)
	}

	// Wait for short TTL to expire
	start := time.Now()
	if err := helpers.WaitForTimeout(2*time.Second, 1*time.Millisecond, func() (bool, error) {
		return time.Since(start) >= 5*time.Millisecond, nil
	}); err != nil {
		t.Fatalf("expiry wait did not elapse: %v", err)
	}

	// Cleanup
	removed, err := provider.Cleanup(ctx)
	if err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}

	if removed != 1 {
		t.Errorf("Cleanup() removed %d, want 1", removed)
	}

	// List active should now have only 1
	active, _ := provider.ListActive(ctx)
	if len(active) != 1 {
		t.Errorf("ListActive() returned %d credentials after cleanup, want 1", len(active))
	}
}

func TestInMemoryBootstrapProvider_Validate_ByPublicKey(t *testing.T) {
	ctx := context.Background()
	provider := NewInMemoryBootstrapProvider("test-issuer")

	// Generate an NKey credential
	cred, err := provider.Generate(ctx, BootstrapCredentialRequest{
		Cluster: "test-cluster",
		Type:    BootstrapCredentialTypeNKey,
	})
	if err != nil {
		t.Fatalf("failed to generate credential: %v", err)
	}

	// Validate using only the public key (without claims ID)
	credWithoutID := &BootstrapCredential{
		Type:      BootstrapCredentialTypeNKey,
		PublicKey: cred.PublicKey,
	}

	result, err := provider.Validate(ctx, credWithoutID)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if !result.Valid {
		t.Error("Validate() should pass when validating by public key")
	}
}

func TestInMemoryBootstrapProvider_Validate_ByToken(t *testing.T) {
	ctx := context.Background()
	provider := NewInMemoryBootstrapProvider("test-issuer")

	// Generate a token credential
	cred, err := provider.Generate(ctx, BootstrapCredentialRequest{
		Cluster: "test-cluster",
		Type:    BootstrapCredentialTypeToken,
	})
	if err != nil {
		t.Fatalf("failed to generate credential: %v", err)
	}

	// Validate using only the token (without claims ID)
	credWithoutID := &BootstrapCredential{
		Type:  BootstrapCredentialTypeToken,
		Token: cred.Token,
	}

	result, err := provider.Validate(ctx, credWithoutID)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if !result.Valid {
		t.Error("Validate() should pass when validating by token")
	}
}

func TestNoOpBootstrapAuditLogger(t *testing.T) {
	ctx := context.Background()
	logger := &NoOpBootstrapAuditLogger{}

	event := BootstrapAuditEvent{
		Timestamp:    time.Now(),
		EventType:    BootstrapAuditEventGenerate,
		CredentialID: "test-id",
		Cluster:      "test-cluster",
		Success:      true,
	}

	err := logger.Log(ctx, event)
	if err != nil {
		t.Errorf("NoOpBootstrapAuditLogger.Log() error = %v, want nil", err)
	}
}

func TestBootstrapRegistrationRequest(t *testing.T) {
	req := BootstrapRegistrationRequest{
		BootstrapCredential: &BootstrapCredential{
			Type: BootstrapCredentialTypeNKey,
			Claims: BootstrapClaims{
				ID:      "test-id",
				Cluster: "test-cluster",
			},
		},
		AgentID: "agent-123",
		Labels: map[string]string{
			"env": "production",
		},
		Metadata: map[string]string{
			"version": "1.0.0",
		},
	}

	if req.BootstrapCredential == nil {
		t.Error("BootstrapCredential should not be nil")
	}
	if req.AgentID != "agent-123" {
		t.Errorf("AgentID = %s, want agent-123", req.AgentID)
	}
}

func TestBootstrapRegistrationResponse(t *testing.T) {
	resp := BootstrapRegistrationResponse{
		Success: true,
		AgentID: "agent-123",
		Credentials: &IssuedCredentials{
			AgentID:       "agent-123",
			NKeyPublicKey: "UABC123",
		},
	}

	if !resp.Success {
		t.Error("Success should be true")
	}
	if resp.AgentID != "agent-123" {
		t.Errorf("AgentID = %s, want agent-123", resp.AgentID)
	}
	if resp.Credentials == nil {
		t.Error("Credentials should not be nil")
	}
}

func TestIdentityVerificationResult(t *testing.T) {
	result := IdentityVerificationResult{
		Verified:   true,
		Identity:   "spiffe://cluster.local/agent/agent-123",
		TrustLevel: "high",
		Attributes: map[string]string{
			"cluster": "production",
		},
	}

	if !result.Verified {
		t.Error("Verified should be true")
	}
	if result.Identity != "spiffe://cluster.local/agent/agent-123" {
		t.Errorf("Identity = %s, want spiffe://...", result.Identity)
	}
}

func TestCredentialIssueRequest(t *testing.T) {
	req := CredentialIssueRequest{
		AgentID: "agent-123",
		Cluster: "test-cluster",
		Labels: map[string]string{
			"env": "production",
		},
		BootstrapID: "bootstrap-abc",
	}

	if req.AgentID != "agent-123" {
		t.Errorf("AgentID = %s, want agent-123", req.AgentID)
	}
	if req.BootstrapID != "bootstrap-abc" {
		t.Errorf("BootstrapID = %s, want bootstrap-abc", req.BootstrapID)
	}
}

func TestIssuedCredentials(t *testing.T) {
	issued := IssuedCredentials{
		AgentID:       "agent-123",
		NKeyPublicKey: "UABC123",
		Subjects: SubjectPermissions{
			Publish:   []string{"kscore.default.agent.heartbeat"},
			Subscribe: []string{"kscore.default.agent.agent-123.command"},
		},
	}

	if issued.AgentID != "agent-123" {
		t.Errorf("AgentID = %s, want agent-123", issued.AgentID)
	}
	if len(issued.Subjects.Publish) == 0 {
		t.Error("Publish subjects should not be empty")
	}
}
