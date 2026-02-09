package identity

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseSPIFFEID(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantDomain string
		wantPath   string
		wantErr    bool
	}{
		{
			name:       "valid agent ID",
			input:      "spiffe://example.com/agent/agent-1",
			wantDomain: "example.com",
			wantPath:   "/agent/agent-1",
			wantErr:    false,
		},
		{
			name:       "valid server ID",
			input:      "spiffe://kscore.local/server/server-1",
			wantDomain: "kscore.local",
			wantPath:   "/server/server-1",
			wantErr:    false,
		},
		{
			name:       "valid service ID",
			input:      "spiffe://example.com/service/nats",
			wantDomain: "example.com",
			wantPath:   "/service/nats",
			wantErr:    false,
		},
		{
			name:    "invalid scheme",
			input:   "https://example.com/agent/1",
			wantErr: true,
		},
		{
			name:    "missing trust domain",
			input:   "spiffe:///agent/1",
			wantErr: true,
		},
		{
			name:    "invalid URL",
			input:   "not a valid url",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := ParseSPIFFEID(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseSPIFFEID() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if id.TrustDomain != tt.wantDomain {
					t.Errorf("TrustDomain = %v, want %v", id.TrustDomain, tt.wantDomain)
				}
				if id.Path != tt.wantPath {
					t.Errorf("Path = %v, want %v", id.Path, tt.wantPath)
				}
			}
		})
	}
}

func TestSPIFFEIDHelpers(t *testing.T) {
	t.Run("NewAgentSPIFFEID", func(t *testing.T) {
		id := NewAgentSPIFFEID("example.com", "agent-1")
		if id.String() != "spiffe://example.com/agent/agent-1" {
			t.Errorf("got %s, want spiffe://example.com/agent/agent-1", id.String())
		}
	})

	t.Run("NewServerSPIFFEID", func(t *testing.T) {
		id := NewServerSPIFFEID("example.com", "server-1")
		if id.String() != "spiffe://example.com/server/server-1" {
			t.Errorf("got %s, want spiffe://example.com/server/server-1", id.String())
		}
	})

	t.Run("NewServiceSPIFFEID", func(t *testing.T) {
		id := NewServiceSPIFFEID("example.com", "nats")
		if id.String() != "spiffe://example.com/service/nats" {
			t.Errorf("got %s, want spiffe://example.com/service/nats", id.String())
		}
	})
}

func TestX509SVIDMethods(t *testing.T) {
	now := time.Now()

	t.Run("Expired", func(t *testing.T) {
		svid := &X509SVID{
			ExpiresAt: now.Add(-1 * time.Hour),
		}
		if !svid.Expired() {
			t.Error("expected SVID to be expired")
		}

		svid.ExpiresAt = now.Add(1 * time.Hour)
		if svid.Expired() {
			t.Error("expected SVID to not be expired")
		}
	})

	t.Run("ShouldRotate", func(t *testing.T) {
		// SVID with 1 hour lifetime, issued now
		svid := &X509SVID{
			IssuedAt:  now,
			ExpiresAt: now.Add(1 * time.Hour),
		}
		if svid.ShouldRotate() {
			t.Error("expected SVID to not need rotation yet")
		}

		// SVID with 1 hour lifetime, issued 40 minutes ago
		svid.IssuedAt = now.Add(-40 * time.Minute)
		svid.ExpiresAt = now.Add(20 * time.Minute)
		if !svid.ShouldRotate() {
			t.Error("expected SVID to need rotation")
		}
	})

	t.Run("TimeUntilExpiry", func(t *testing.T) {
		svid := &X509SVID{
			ExpiresAt: now.Add(30 * time.Minute),
		}
		ttl := svid.TimeUntilExpiry()
		if ttl < 29*time.Minute || ttl > 31*time.Minute {
			t.Errorf("expected TTL around 30 minutes, got %v", ttl)
		}
	})
}

func TestValidateTrustDomain(t *testing.T) {
	tests := []struct {
		domain  string
		wantErr bool
	}{
		{"example.com", false},
		{"kscore.local", false},
		{"my-domain.example.com", false},
		{"", true},
		{" has spaces ", true},
		{".starts.with.dot", true},
		{"ends.with.dot.", true},
		{"has..double.dot", true},
	}

	for _, tt := range tests {
		t.Run(tt.domain, func(t *testing.T) {
			err := ValidateTrustDomain(tt.domain)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateTrustDomain(%q) error = %v, wantErr %v", tt.domain, err, tt.wantErr)
			}
		})
	}
}

func TestValidateSPIFFEID(t *testing.T) {
	tests := []struct {
		name    string
		id      SPIFFEID
		wantErr bool
	}{
		{
			name:    "valid agent ID",
			id:      SPIFFEID{TrustDomain: "example.com", Path: "/agent/1"},
			wantErr: false,
		},
		{
			name:    "empty trust domain",
			id:      SPIFFEID{TrustDomain: "", Path: "/agent/1"},
			wantErr: true,
		},
		{
			name:    "empty path",
			id:      SPIFFEID{TrustDomain: "example.com", Path: ""},
			wantErr: true,
		},
		{
			name:    "path without leading slash",
			id:      SPIFFEID{TrustDomain: "example.com", Path: "agent/1"},
			wantErr: true,
		},
		{
			name:    "path with double slashes",
			id:      SPIFFEID{TrustDomain: "example.com", Path: "/agent//1"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSPIFFEID(tt.id)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateSPIFFEID() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if !config.Enabled {
		t.Error("expected Enabled to be true")
	}
	if config.TrustDomain != "kscore.local" {
		t.Errorf("expected TrustDomain kscore.local, got %s", config.TrustDomain)
	}
	if config.Provider.Type != ProviderTypeEmbedded {
		t.Errorf("expected Provider.Type embedded, got %s", config.Provider.Type)
	}
	if config.SVID.DefaultTTL != 1*time.Hour {
		t.Errorf("expected SVID.DefaultTTL 1h, got %v", config.SVID.DefaultTTL)
	}
	if config.CA.KeyType != "ecdsa-p256" {
		t.Errorf("expected CA.KeyType ecdsa-p256, got %s", config.CA.KeyType)
	}
	if len(config.Attestation.AllowedAttestors) != 1 {
		t.Error("expected 1 allowed attestor")
	}

	// Validate the default config
	if err := config.Validate(); err != nil {
		t.Errorf("default config validation failed: %v", err)
	}
}

func TestConfigValidation(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		config := DefaultConfig()
		if err := config.Validate(); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("disabled config skips validation", func(t *testing.T) {
		config := &Config{
			Enabled:     false,
			TrustDomain: "", // Invalid but should be skipped
		}
		if err := config.Validate(); err != nil {
			t.Errorf("unexpected error for disabled config: %v", err)
		}
	})

	t.Run("invalid trust domain", func(t *testing.T) {
		config := DefaultConfig()
		config.TrustDomain = ""
		if err := config.Validate(); err == nil {
			t.Error("expected error for empty trust domain")
		}
	})

	t.Run("invalid SVID TTL", func(t *testing.T) {
		config := DefaultConfig()
		config.SVID.DefaultTTL = 0
		if err := config.Validate(); err == nil {
			t.Error("expected error for zero TTL")
		}
	})

	t.Run("default TTL exceeds max TTL", func(t *testing.T) {
		config := DefaultConfig()
		config.SVID.DefaultTTL = 48 * time.Hour
		config.SVID.MaxTTL = 24 * time.Hour
		if err := config.Validate(); err == nil {
			t.Error("expected error when default TTL exceeds max TTL")
		}
	})
}

func TestCAManager(t *testing.T) {
	// Create temp directory for CA storage
	tmpDir, err := os.MkdirTemp("", "ca-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	config := &CAManagerConfig{
		KeyType:               "ecdsa-p256",
		RootCATTL:             10 * 365 * 24 * time.Hour,
		SigningCATTL:          365 * 24 * time.Hour,
		RotateSigningCABefore: 30 * 24 * time.Hour,
		StoragePath:           tmpDir,
		TrustDomain:           "test.local",
		Subject: CASubjectConfig{
			Organization:       "Test Org",
			OrganizationalUnit: "Test Unit",
		},
	}

	t.Run("create CA manager", func(t *testing.T) {
		ca, err := NewCAManager(config)
		if err != nil {
			t.Fatalf("failed to create CA manager: %v", err)
		}
		if ca == nil {
			t.Fatal("CA manager is nil")
		}
	})

	t.Run("initialize CA", func(t *testing.T) {
		ca, _ := NewCAManager(config)
		ctx := context.Background()

		if err := ca.Initialize(ctx); err != nil {
			t.Fatalf("failed to initialize CA: %v", err)
		}

		// Check that certificates were generated
		info := ca.Info()
		if info.KeyType != "ecdsa-p256" {
			t.Errorf("expected KeyType ecdsa-p256, got %s", info.KeyType)
		}
		if info.TrustDomain != "test.local" {
			t.Errorf("expected TrustDomain test.local, got %s", info.TrustDomain)
		}
		if info.RootCAExpires.Before(time.Now().Add(9 * 365 * 24 * time.Hour)) {
			t.Error("root CA expires too soon")
		}
		if info.SigningCAExpires.Before(time.Now().Add(364 * 24 * time.Hour)) {
			t.Error("signing CA expires too soon")
		}

		// Check files were created
		files := []string{"root-ca.crt", "root-ca.key", "signing-ca.crt", "signing-ca.key"}
		for _, f := range files {
			path := filepath.Join(tmpDir, f)
			if _, err := os.Stat(path); os.IsNotExist(err) {
				t.Errorf("expected file %s to exist", f)
			}
		}
	})

	t.Run("reload CA from disk", func(t *testing.T) {
		ca, _ := NewCAManager(config)
		ctx := context.Background()

		if err := ca.Initialize(ctx); err != nil {
			t.Fatalf("failed to reload CA: %v", err)
		}

		// Should load existing CA, not generate new
		chain := ca.GetTrustChain()
		if len(chain) != 2 {
			t.Errorf("expected 2 certificates in trust chain, got %d", len(chain))
		}
	})

	t.Run("sign SVID", func(t *testing.T) {
		ca, _ := NewCAManager(config)
		ctx := context.Background()
		_ = ca.Initialize(ctx)

		spiffeID := NewAgentSPIFFEID("test.local", "agent-1")
		key, _ := ca.generateKey()
		pubKey := publicKey(key)

		cert, err := ca.SignX509SVID(spiffeID, pubKey, 1*time.Hour, nil, nil)
		if err != nil {
			t.Fatalf("failed to sign SVID: %v", err)
		}

		if cert == nil {
			t.Fatal("certificate is nil")
		}

		// Verify SPIFFE ID is in URI SAN
		found := false
		for _, uri := range cert.URIs {
			if uri.String() == spiffeID.String() {
				found = true
				break
			}
		}
		if !found {
			t.Error("SPIFFE ID not found in certificate URIs")
		}
	})
}

func TestJoinTokenStore(t *testing.T) {
	store := NewInMemoryTokenStore()
	ctx := context.Background()

	t.Run("create token", func(t *testing.T) {
		token := &JoinToken{
			Token:     "test-token-1",
			ExpiresAt: time.Now().Add(5 * time.Minute),
			CreatedAt: time.Now(),
		}
		if err := store.Create(ctx, token); err != nil {
			t.Fatalf("failed to create token: %v", err)
		}
	})

	t.Run("get token", func(t *testing.T) {
		token, err := store.Get(ctx, "test-token-1")
		if err != nil {
			t.Fatalf("failed to get token: %v", err)
		}
		// Token field should be empty after retrieval (security: tokens are stored as hashes)
		if token.Token != "" {
			t.Errorf("expected token field to be empty (redacted), got %s", token.Token)
		}
		// TokenHash and Salt should be populated
		if token.TokenHash == "" {
			t.Error("expected TokenHash to be populated")
		}
		if token.Salt == "" {
			t.Error("expected Salt to be populated")
		}
		// TokenPrefix should show first 8 chars
		if token.TokenPrefix == "" {
			t.Error("expected TokenPrefix to be populated")
		}
		if !token.IsValid() {
			t.Error("expected token to be valid")
		}
	})

	t.Run("mark used", func(t *testing.T) {
		if err := store.MarkUsed(ctx, "test-token-1", "agent-1"); err != nil {
			t.Fatalf("failed to mark token as used: %v", err)
		}

		token, _ := store.Get(ctx, "test-token-1")
		if !token.Used {
			t.Error("expected token to be marked as used")
		}
		if token.UsedBy != "agent-1" {
			t.Errorf("expected UsedBy agent-1, got %s", token.UsedBy)
		}
		if token.IsValid() {
			t.Error("expected used token to be invalid")
		}
	})

	t.Run("list tokens", func(t *testing.T) {
		// Create another token
		token2 := &JoinToken{
			Token:     "test-token-2",
			ExpiresAt: time.Now().Add(5 * time.Minute),
			CreatedAt: time.Now(),
		}
		_ = store.Create(ctx, token2)

		tokens, err := store.List(ctx)
		if err != nil {
			t.Fatalf("failed to list tokens: %v", err)
		}
		if len(tokens) != 2 {
			t.Errorf("expected 2 tokens, got %d", len(tokens))
		}
		// Verify tokens are redacted in listings (security)
		for _, tok := range tokens {
			if tok.Token != "" {
				t.Errorf("expected Token field to be empty in listing, got %s", tok.Token)
			}
			if tok.TokenPrefix == "" {
				t.Error("expected TokenPrefix to be populated for identification")
			}
		}
	})

	t.Run("cleanup", func(t *testing.T) {
		count, err := store.Cleanup(ctx)
		if err != nil {
			t.Fatalf("failed to cleanup: %v", err)
		}
		// Should have cleaned up the used token
		if count != 1 {
			t.Errorf("expected to cleanup 1 token, got %d", count)
		}

		tokens, _ := store.List(ctx)
		if len(tokens) != 1 {
			t.Errorf("expected 1 token after cleanup, got %d", len(tokens))
		}
	})

	t.Run("delete token", func(t *testing.T) {
		if err := store.Delete(ctx, "test-token-2"); err != nil {
			t.Fatalf("failed to delete token: %v", err)
		}

		tokens, _ := store.List(ctx)
		if len(tokens) != 0 {
			t.Errorf("expected 0 tokens after delete, got %d", len(tokens))
		}
	})
}

func TestAttestationEngine(t *testing.T) {
	store := NewInMemoryTokenStore()

	config := &AttestationEngineConfig{
		TrustDomain:      "test.local",
		AllowedAttestors: []string{AttestationTypeJoinToken, AttestationTypeNone},
		AllowNone:        true,
		JoinTokenStore:   store,
	}

	engine, err := NewAttestationEngine(config)
	if err != nil {
		t.Fatalf("failed to create attestation engine: %v", err)
	}

	ctx := context.Background()

	t.Run("join token attestation", func(t *testing.T) {
		// Create a token
		token := &JoinToken{
			Token:     "attest-token-1",
			ExpiresAt: time.Now().Add(5 * time.Minute),
			CreatedAt: time.Now(),
		}
		_ = store.Create(ctx, token)

		evidence := &AttestationEvidence{
			Type:     AttestationTypeJoinToken,
			Data:     []byte("attest-token-1"),
			Metadata: map[string]string{"agent_id": "agent-1"},
		}

		result, err := engine.Attest(ctx, evidence)
		if err != nil {
			t.Fatalf("attestation failed: %v", err)
		}

		if !result.Success {
			t.Errorf("expected attestation to succeed, got error: %s", result.Error)
		}

		if result.SPIFFEID.String() != "spiffe://test.local/agent/agent-1" {
			t.Errorf("unexpected SPIFFE ID: %s", result.SPIFFEID.String())
		}
	})

	t.Run("invalid token", func(t *testing.T) {
		evidence := &AttestationEvidence{
			Type: AttestationTypeJoinToken,
			Data: []byte("invalid-token"),
		}

		result, err := engine.Attest(ctx, evidence)
		if err != nil {
			t.Fatalf("attestation call failed: %v", err)
		}

		if result.Success {
			t.Error("expected attestation to fail for invalid token")
		}
	})

	t.Run("none attestor", func(t *testing.T) {
		evidence := &AttestationEvidence{
			Type:     AttestationTypeNone,
			Metadata: map[string]string{"agent_id": "dev-agent"},
		}

		result, err := engine.Attest(ctx, evidence)
		if err != nil {
			t.Fatalf("attestation failed: %v", err)
		}

		if !result.Success {
			t.Errorf("expected none attestation to succeed: %s", result.Error)
		}
	})

	t.Run("unknown attestor", func(t *testing.T) {
		evidence := &AttestationEvidence{
			Type: "unknown",
			Data: []byte("data"),
		}

		result, err := engine.Attest(ctx, evidence)
		if err != nil {
			t.Fatalf("attestation call failed: %v", err)
		}

		if result.Success {
			t.Error("expected attestation to fail for unknown attestor")
		}
	})

	t.Run("list attestors", func(t *testing.T) {
		attestors := engine.ListAttestors()
		if len(attestors) < 2 {
			t.Errorf("expected at least 2 attestors, got %d", len(attestors))
		}
	})
}

func TestAttestationEngineFallback(t *testing.T) {
	store := NewInMemoryTokenStore()
	ctx := context.Background()

	t.Run("auto detection mode", func(t *testing.T) {
		config := &AttestationEngineConfig{
			TrustDomain:      "test.local",
			AllowedAttestors: []string{AttestationTypeJoinToken, AttestationTypeNone},
			AllowNone:        true,
			EnableFallback:   true,
			FallbackOrder:    []string{AttestationTypeJoinToken, AttestationTypeNone},
			JoinTokenStore:   store,
		}

		engine, err := NewAttestationEngine(config)
		if err != nil {
			t.Fatalf("failed to create attestation engine: %v", err)
		}

		// Use auto-detection with none attestor data (should fall back to none)
		evidence := &AttestationEvidence{
			Type:     AttestationTypeAuto,
			Metadata: map[string]string{"agent_id": "auto-agent"},
		}

		result, err := engine.Attest(ctx, evidence)
		if err != nil {
			t.Fatalf("attestation failed: %v", err)
		}

		if !result.Success {
			t.Errorf("expected auto attestation to succeed, got error: %s", result.Error)
		}

		// Should have tried attestors and recorded them
		if len(result.AttemptedAttestors) == 0 {
			t.Error("expected AttemptedAttestors to be populated")
		}

		// Should have used the none attestor since join_token would fail without valid token
		if result.Attestor != AttestationTypeNone {
			t.Errorf("expected attestor to be 'none', got %s", result.Attestor)
		}
	})

	t.Run("auto detection with valid token", func(t *testing.T) {
		config := &AttestationEngineConfig{
			TrustDomain:      "test.local",
			AllowedAttestors: []string{AttestationTypeJoinToken, AttestationTypeNone},
			AllowNone:        true,
			EnableFallback:   true,
			FallbackOrder:    []string{AttestationTypeJoinToken, AttestationTypeNone},
			JoinTokenStore:   store,
		}

		engine, err := NewAttestationEngine(config)
		if err != nil {
			t.Fatalf("failed to create attestation engine: %v", err)
		}

		// Create a valid token
		token := &JoinToken{
			Token:     "auto-fallback-token",
			ExpiresAt: time.Now().Add(5 * time.Minute),
			CreatedAt: time.Now(),
		}
		_ = store.Create(ctx, token)

		// Use auto-detection with valid token data
		evidence := &AttestationEvidence{
			Type:     AttestationTypeAuto,
			Data:     []byte("auto-fallback-token"),
			Metadata: map[string]string{"agent_id": "token-agent"},
		}

		result, err := engine.Attest(ctx, evidence)
		if err != nil {
			t.Fatalf("attestation failed: %v", err)
		}

		if !result.Success {
			t.Errorf("expected attestation to succeed, got error: %s", result.Error)
		}

		// Should have used join_token attestor first
		if result.Attestor != AttestationTypeJoinToken {
			t.Errorf("expected attestor to be 'join_token', got %s", result.Attestor)
		}

		if len(result.AttemptedAttestors) != 1 {
			t.Errorf("expected 1 attempted attestor, got %d", len(result.AttemptedAttestors))
		}
	})

	t.Run("fallback on primary failure", func(t *testing.T) {
		config := &AttestationEngineConfig{
			TrustDomain:      "test.local",
			AllowedAttestors: []string{AttestationTypeJoinToken, AttestationTypeNone},
			AllowNone:        true,
			EnableFallback:   true,
			FallbackOrder:    []string{AttestationTypeJoinToken, AttestationTypeNone},
			JoinTokenStore:   store,
		}

		engine, err := NewAttestationEngine(config)
		if err != nil {
			t.Fatalf("failed to create attestation engine: %v", err)
		}

		// Try join_token with invalid token - should fallback to none
		evidence := &AttestationEvidence{
			Type:     AttestationTypeJoinToken,
			Data:     []byte("invalid-token-for-fallback"),
			Metadata: map[string]string{"agent_id": "fallback-agent"},
		}

		result, err := engine.Attest(ctx, evidence)
		if err != nil {
			t.Fatalf("attestation failed: %v", err)
		}

		if !result.Success {
			t.Errorf("expected fallback attestation to succeed, got error: %s", result.Error)
		}

		// Should have used the none attestor after join_token failed
		if result.Attestor != AttestationTypeNone {
			t.Errorf("expected attestor to be 'none', got %s", result.Attestor)
		}

		// AttemptedAttestors should include both
		if len(result.AttemptedAttestors) < 2 {
			t.Errorf("expected at least 2 attempted attestors, got %d: %v", len(result.AttemptedAttestors), result.AttemptedAttestors)
		}
	})

	t.Run("no fallback when disabled", func(t *testing.T) {
		config := &AttestationEngineConfig{
			TrustDomain:      "test.local",
			AllowedAttestors: []string{AttestationTypeJoinToken, AttestationTypeNone},
			AllowNone:        true,
			EnableFallback:   false, // Fallback disabled
			JoinTokenStore:   store,
		}

		engine, err := NewAttestationEngine(config)
		if err != nil {
			t.Fatalf("failed to create attestation engine: %v", err)
		}

		// Try join_token with invalid token - should NOT fallback
		evidence := &AttestationEvidence{
			Type:     AttestationTypeJoinToken,
			Data:     []byte("another-invalid-token"),
			Metadata: map[string]string{"agent_id": "no-fallback-agent"},
		}

		result, err := engine.Attest(ctx, evidence)
		if err != nil {
			t.Fatalf("attestation call failed: %v", err)
		}

		// Should fail without fallback
		if result.Success {
			t.Error("expected attestation to fail without fallback")
		}

		// AttemptedAttestors should be empty (not using fallback)
		if len(result.AttemptedAttestors) != 0 {
			t.Errorf("expected 0 attempted attestors when fallback disabled, got %d", len(result.AttemptedAttestors))
		}
	})

	t.Run("custom fallback order", func(t *testing.T) {
		config := &AttestationEngineConfig{
			TrustDomain:      "test.local",
			AllowedAttestors: []string{AttestationTypeJoinToken, AttestationTypeNone},
			AllowNone:        true,
			EnableFallback:   true,
			FallbackOrder:    []string{AttestationTypeNone, AttestationTypeJoinToken}, // None first
			JoinTokenStore:   store,
		}

		engine, err := NewAttestationEngine(config)
		if err != nil {
			t.Fatalf("failed to create attestation engine: %v", err)
		}

		// Use auto-detection - should try none first
		evidence := &AttestationEvidence{
			Type:     AttestationTypeAuto,
			Metadata: map[string]string{"agent_id": "custom-order-agent"},
		}

		result, err := engine.Attest(ctx, evidence)
		if err != nil {
			t.Fatalf("attestation failed: %v", err)
		}

		if !result.Success {
			t.Errorf("expected attestation to succeed, got error: %s", result.Error)
		}

		// Should have used none attestor first (per custom order)
		if result.Attestor != AttestationTypeNone {
			t.Errorf("expected attestor to be 'none' (first in custom order), got %s", result.Attestor)
		}

		// Should have only tried the first one since it succeeded
		if len(result.AttemptedAttestors) != 1 {
			t.Errorf("expected 1 attempted attestor, got %d", len(result.AttemptedAttestors))
		}
	})

	t.Run("all attestors fail", func(t *testing.T) {
		// Create engine with only join_token (no none attestor)
		config := &AttestationEngineConfig{
			TrustDomain:      "test.local",
			AllowedAttestors: []string{AttestationTypeJoinToken},
			AllowNone:        false,
			EnableFallback:   true,
			JoinTokenStore:   store,
		}

		engine, err := NewAttestationEngine(config)
		if err != nil {
			t.Fatalf("failed to create attestation engine: %v", err)
		}

		// Use auto-detection with no valid token
		evidence := &AttestationEvidence{
			Type:     AttestationTypeAuto,
			Data:     []byte("definitely-not-valid"),
			Metadata: map[string]string{"agent_id": "failing-agent"},
		}

		result, err := engine.Attest(ctx, evidence)
		if err != nil {
			t.Fatalf("attestation call failed: %v", err)
		}

		if result.Success {
			t.Error("expected all attestors to fail")
		}

		if result.Error == "" {
			t.Error("expected error message when all attestors fail")
		}

		// Should have recorded the attempted attestor
		if len(result.AttemptedAttestors) != 1 {
			t.Errorf("expected 1 attempted attestor, got %d", len(result.AttemptedAttestors))
		}
	})
}

func TestMatchSPIFFEPattern(t *testing.T) {
	tests := []struct {
		pattern  string
		spiffeID string
		want     bool
	}{
		{"spiffe://example.com/agent/1", "spiffe://example.com/agent/1", true},
		{"spiffe://example.com/agent/*", "spiffe://example.com/agent/1", true},
		{"spiffe://example.com/agent/*", "spiffe://example.com/agent/any-id", true},
		{"spiffe://example.com/agent/*", "spiffe://example.com/server/1", false},
		{"spiffe://example.com/*", "spiffe://example.com/agent", true},
		{"spiffe://other.com/agent/*", "spiffe://example.com/agent/1", false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.spiffeID, func(t *testing.T) {
			if got := matchSPIFFEPattern(tt.pattern, tt.spiffeID); got != tt.want {
				t.Errorf("matchSPIFFEPattern(%q, %q) = %v, want %v", tt.pattern, tt.spiffeID, got, tt.want)
			}
		})
	}
}

func TestMatchSubjectPattern(t *testing.T) {
	tests := []struct {
		pattern string
		subject string
		want    bool
	}{
		{">", "any.subject.here", true},
		{"kscore.agent.*", "kscore.agent.heartbeat", true},
		{"kscore.agent.*", "kscore.agent.command", true},
		{"kscore.agent.*", "kscore.server.status", false},
		{"kscore.*.command", "kscore.agent.command", true},
		{"kscore.agent.>", "kscore.agent.foo.bar.baz", true},
		{"exact.match", "exact.match", true},
		{"exact.match", "not.exact.match", false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.subject, func(t *testing.T) {
			if got := matchSubjectPattern(tt.pattern, tt.subject); got != tt.want {
				t.Errorf("matchSubjectPattern(%q, %q) = %v, want %v", tt.pattern, tt.subject, got, tt.want)
			}
		})
	}
}

func TestSVIDCache(t *testing.T) {
	cache := NewSVIDCache()

	now := time.Now()
	svid := &X509SVID{
		SPIFFEID:  NewAgentSPIFFEID("test.local", "agent-1"),
		IssuedAt:  now,
		ExpiresAt: now.Add(1 * time.Hour),
	}

	t.Run("put and get", func(t *testing.T) {
		cache.Put(svid)

		got, ok := cache.Get(svid.SPIFFEID.String())
		if !ok {
			t.Error("expected to find SVID in cache")
		}
		if got.SPIFFEID.String() != svid.SPIFFEID.String() {
			t.Errorf("got wrong SVID: %s", got.SPIFFEID.String())
		}
	})

	t.Run("size", func(t *testing.T) {
		if cache.Size() != 1 {
			t.Errorf("expected size 1, got %d", cache.Size())
		}
	})

	t.Run("delete", func(t *testing.T) {
		cache.Delete(svid.SPIFFEID.String())
		if cache.Size() != 0 {
			t.Errorf("expected size 0 after delete, got %d", cache.Size())
		}
	})

	t.Run("cleanup expired", func(t *testing.T) {
		// Add expired SVID
		expired := &X509SVID{
			SPIFFEID:  NewAgentSPIFFEID("test.local", "expired"),
			IssuedAt:  now.Add(-2 * time.Hour),
			ExpiresAt: now.Add(-1 * time.Hour),
		}
		cache.Put(expired)
		cache.Put(svid) // Add valid one too

		count := cache.Cleanup()
		if count != 1 {
			t.Errorf("expected to cleanup 1, got %d", count)
		}
		if cache.Size() != 1 {
			t.Errorf("expected size 1 after cleanup, got %d", cache.Size())
		}
	})
}
