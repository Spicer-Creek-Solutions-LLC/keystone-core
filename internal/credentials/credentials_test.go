// Package credentials provides secure credential management for proxy agents.
package credentials

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/testing/helpers"
)

// =============================================================================
// Credential Types Tests
// =============================================================================

func TestSSHPasswordCredential(t *testing.T) {
	cred := &SSHPasswordCredential{
		BaseCredential: BaseCredential{
			CredentialID:   "ssh-pass-1",
			CredentialType: CredentialTypeSSHPassword,
			Description:    "Test SSH Password",
			Expires:        time.Now().Add(time.Hour),
		},
		Username: "admin",
		Password: "secret123",
	}

	if cred.Type() != CredentialTypeSSHPassword {
		t.Errorf("expected type %s, got %s", CredentialTypeSSHPassword, cred.Type())
	}

	if cred.ID() != "ssh-pass-1" {
		t.Errorf("expected ID ssh-pass-1, got %s", cred.ID())
	}

	if cred.IsExpired() {
		t.Error("credential should not be expired")
	}

	if err := cred.Validate(); err != nil {
		t.Errorf("validation should pass: %v", err)
	}

	// Test redaction
	redacted := cred.Redact().(*SSHPasswordCredential)
	if redacted.Password != "[REDACTED]" {
		t.Errorf("password should be redacted, got %s", redacted.Password)
	}
}

func TestSSHKeyCredential(t *testing.T) {
	cred := &SSHKeyCredential{
		BaseCredential: BaseCredential{
			CredentialID:   "ssh-key-1",
			CredentialType: CredentialTypeSSHKey,
			Description:    "Test SSH Key",
			Expires:        time.Now().Add(time.Hour),
		},
		Username:   "admin",
		PrivateKey: []byte("-----BEGIN RSA PRIVATE KEY-----\ntest\n-----END RSA PRIVATE KEY-----"),
	}

	if cred.Type() != CredentialTypeSSHKey {
		t.Errorf("expected type %s, got %s", CredentialTypeSSHKey, cred.Type())
	}

	if err := cred.Validate(); err != nil {
		t.Errorf("validation should pass: %v", err)
	}

	// Test redaction
	redacted := cred.Redact().(*SSHKeyCredential)
	if string(redacted.PrivateKey) != "[REDACTED]" {
		t.Errorf("private key should be redacted")
	}
}

func TestSNMPv2cCredential(t *testing.T) {
	cred := &SNMPv2cCredential{
		BaseCredential: BaseCredential{
			CredentialID:   "snmp-v2-1",
			CredentialType: CredentialTypeSNMPv2c,
			Description:    "Test SNMP v2c",
			Expires:        time.Now().Add(time.Hour),
		},
		Community: "public",
	}

	if cred.Type() != CredentialTypeSNMPv2c {
		t.Errorf("expected type %s, got %s", CredentialTypeSNMPv2c, cred.Type())
	}

	if err := cred.Validate(); err != nil {
		t.Errorf("validation should pass: %v", err)
	}

	// Test redaction
	redacted := cred.Redact().(*SNMPv2cCredential)
	if redacted.Community != "[REDACTED]" {
		t.Errorf("community should be redacted")
	}
}

func TestSNMPv3Credential(t *testing.T) {
	cred := &SNMPv3Credential{
		BaseCredential: BaseCredential{
			CredentialID:   "snmp-v3-1",
			CredentialType: CredentialTypeSNMPv3,
			Description:    "Test SNMP v3",
			Expires:        time.Now().Add(time.Hour),
		},
		Username:        "snmpuser",
		SecurityLevel:   SNMPv3SecurityAuthPriv,
		AuthProtocol:    SNMPv3AuthSHA,
		AuthPassword:    "authpass",
		PrivacyProtocol: SNMPv3PrivAES,
		PrivacyPassword: "privpass",
	}

	if cred.Type() != CredentialTypeSNMPv3 {
		t.Errorf("expected type %s, got %s", CredentialTypeSNMPv3, cred.Type())
	}

	if err := cred.Validate(); err != nil {
		t.Errorf("validation should pass: %v", err)
	}

	// Test redaction
	redacted := cred.Redact().(*SNMPv3Credential)
	if redacted.AuthPassword != "[REDACTED]" {
		t.Errorf("auth password should be redacted")
	}
	if redacted.PrivacyPassword != "[REDACTED]" {
		t.Errorf("priv password should be redacted")
	}
}

func TestWinRMCredential(t *testing.T) {
	cred := &WinRMCredential{
		BaseCredential: BaseCredential{
			CredentialID:   "winrm-1",
			CredentialType: CredentialTypeWinRM,
			Description:    "Test WinRM",
			Expires:        time.Now().Add(time.Hour),
		},
		Username: "Administrator",
		Password: "secret",
		UseHTTPS: false,
	}

	if cred.Type() != CredentialTypeWinRM {
		t.Errorf("expected type %s, got %s", CredentialTypeWinRM, cred.Type())
	}

	if err := cred.Validate(); err != nil {
		t.Errorf("validation should pass: %v", err)
	}

	// Test redaction
	redacted := cred.Redact().(*WinRMCredential)
	if redacted.Password != "[REDACTED]" {
		t.Errorf("password should be redacted")
	}
}

func TestRESTBasicCredential(t *testing.T) {
	cred := &RESTBasicCredential{
		BaseCredential: BaseCredential{
			CredentialID:   "rest-basic-1",
			CredentialType: CredentialTypeRESTBasic,
			Description:    "Test REST Basic",
			Expires:        time.Now().Add(time.Hour),
		},
		Username: "apiuser",
		Password: "apipass",
	}

	if cred.Type() != CredentialTypeRESTBasic {
		t.Errorf("expected type %s, got %s", CredentialTypeRESTBasic, cred.Type())
	}

	if err := cred.Validate(); err != nil {
		t.Errorf("validation should pass: %v", err)
	}
}

func TestRESTBearerCredential(t *testing.T) {
	cred := &RESTBearerCredential{
		BaseCredential: BaseCredential{
			CredentialID:   "rest-bearer-1",
			CredentialType: CredentialTypeRESTBearer,
			Description:    "Test REST Bearer",
			Expires:        time.Now().Add(time.Hour),
		},
		Token: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
	}

	if cred.Type() != CredentialTypeRESTBearer {
		t.Errorf("expected type %s, got %s", CredentialTypeRESTBearer, cred.Type())
	}

	if err := cred.Validate(); err != nil {
		t.Errorf("validation should pass: %v", err)
	}

	// Test redaction
	redacted := cred.Redact().(*RESTBearerCredential)
	if redacted.Token != "[REDACTED]" {
		t.Errorf("token should be redacted")
	}
}

func TestRESTAPIKeyCredential(t *testing.T) {
	cred := &RESTAPIKeyCredential{
		BaseCredential: BaseCredential{
			CredentialID:   "rest-apikey-1",
			CredentialType: CredentialTypeRESTAPIKey,
			Description:    "Test REST API Key",
			Expires:        time.Now().Add(time.Hour),
		},
		APIKey:     "sk-1234567890",
		HeaderName: "X-API-Key",
	}

	if cred.Type() != CredentialTypeRESTAPIKey {
		t.Errorf("expected type %s, got %s", CredentialTypeRESTAPIKey, cred.Type())
	}

	if err := cred.Validate(); err != nil {
		t.Errorf("validation should pass: %v", err)
	}

	// Test redaction
	redacted := cred.Redact().(*RESTAPIKeyCredential)
	if redacted.APIKey != "[REDACTED]" {
		t.Errorf("API key should be redacted")
	}
}

func TestRESTOAuth2Credential(t *testing.T) {
	cred := &RESTOAuth2Credential{
		BaseCredential: BaseCredential{
			CredentialID:   "rest-oauth2-1",
			CredentialType: CredentialTypeRESTOAuth2,
			Description:    "Test REST OAuth2",
			Expires:        time.Now().Add(time.Hour),
		},
		ClientID:     "client-123",
		ClientSecret: "secret-456",
		TokenURL:     "https://auth.example.com/token",
		Scopes:       []string{"read", "write"},
	}

	if cred.Type() != CredentialTypeRESTOAuth2 {
		t.Errorf("expected type %s, got %s", CredentialTypeRESTOAuth2, cred.Type())
	}

	if err := cred.Validate(); err != nil {
		t.Errorf("validation should pass: %v", err)
	}

	// Test redaction
	redacted := cred.Redact().(*RESTOAuth2Credential)
	if redacted.ClientSecret != "[REDACTED]" {
		t.Errorf("client secret should be redacted")
	}
	if redacted.AccessToken != "[REDACTED]" {
		t.Errorf("access token should be redacted")
	}
	if redacted.RefreshToken != "[REDACTED]" {
		t.Errorf("refresh token should be redacted")
	}
}

func TestCredentialValidation(t *testing.T) {
	tests := []struct {
		name    string
		cred    Credential
		wantErr bool
	}{
		{
			name: "SSH password missing username",
			cred: &SSHPasswordCredential{
				BaseCredential: BaseCredential{CredentialID: "test"},
				Password:       "pass",
			},
			wantErr: true,
		},
		{
			name: "SSH password missing password",
			cred: &SSHPasswordCredential{
				BaseCredential: BaseCredential{CredentialID: "test"},
				Username:       "user",
			},
			wantErr: true,
		},
		{
			name: "SSH key missing username",
			cred: &SSHKeyCredential{
				BaseCredential: BaseCredential{CredentialID: "test"},
				PrivateKey:     []byte("key"),
			},
			wantErr: true,
		},
		{
			name: "SNMPv2c missing community",
			cred: &SNMPv2cCredential{
				BaseCredential: BaseCredential{CredentialID: "test"},
			},
			wantErr: true,
		},
		{
			name: "SNMPv3 missing username",
			cred: &SNMPv3Credential{
				BaseCredential: BaseCredential{CredentialID: "test"},
				SecurityLevel:  SNMPv3SecurityNoAuthNoPriv,
			},
			wantErr: true,
		},
		{
			name: "WinRM missing username",
			cred: &WinRMCredential{
				BaseCredential: BaseCredential{CredentialID: "test"},
				Password:       "pass",
			},
			wantErr: true,
		},
		{
			name: "REST Basic missing username",
			cred: &RESTBasicCredential{
				BaseCredential: BaseCredential{CredentialID: "test"},
				Password:       "pass",
			},
			wantErr: true,
		},
		{
			name: "REST Bearer missing token",
			cred: &RESTBearerCredential{
				BaseCredential: BaseCredential{CredentialID: "test"},
			},
			wantErr: true,
		},
		{
			name: "REST API Key missing key",
			cred: &RESTAPIKeyCredential{
				BaseCredential: BaseCredential{CredentialID: "test"},
				HeaderName:     "X-API-Key",
			},
			wantErr: true,
		},
		{
			name: "REST OAuth2 missing client ID",
			cred: &RESTOAuth2Credential{
				BaseCredential: BaseCredential{CredentialID: "test"},
				ClientSecret:   "secret",
				TokenURL:       "https://example.com/token",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cred.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCredentialExpiration(t *testing.T) {
	// Expired credential
	expired := &SSHPasswordCredential{
		BaseCredential: BaseCredential{
			CredentialID: "expired",
			Expires:      time.Now().Add(-time.Hour),
		},
		Username: "user",
		Password: "pass",
	}
	if !expired.IsExpired() {
		t.Error("credential should be expired")
	}

	// Non-expired credential
	valid := &SSHPasswordCredential{
		BaseCredential: BaseCredential{
			CredentialID: "valid",
			Expires:      time.Now().Add(time.Hour),
		},
		Username: "user",
		Password: "pass",
	}
	if valid.IsExpired() {
		t.Error("credential should not be expired")
	}

	// Zero expiry (never expires)
	neverExpires := &SSHPasswordCredential{
		BaseCredential: BaseCredential{
			CredentialID: "never",
		},
		Username: "user",
		Password: "pass",
	}
	if neverExpires.IsExpired() {
		t.Error("credential with zero expiry should not be expired")
	}
}

func TestParseCredential(t *testing.T) {
	tests := []struct {
		name     string
		credType CredentialType
		data     []byte
		wantErr  bool
	}{
		{
			name:     "SSH password",
			credType: CredentialTypeSSHPassword,
			data:     []byte(`{"id":"test","username":"user","password":"pass"}`),
			wantErr:  false,
		},
		{
			name:     "SSH key",
			credType: CredentialTypeSSHKey,
			data:     []byte(`{"id":"test","username":"user","private_key":"a2V5"}`),
			wantErr:  false,
		},
		{
			name:     "Unknown type",
			credType: "unknown",
			data:     []byte(`{}`),
			wantErr:  true,
		},
		{
			name:     "Invalid JSON",
			credType: CredentialTypeSSHPassword,
			data:     []byte(`{invalid}`),
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseCredential(tt.credType, tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseCredential() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestOAuth2NeedsRefresh(t *testing.T) {
	// No access token
	cred1 := &RESTOAuth2Credential{
		BaseCredential: BaseCredential{CredentialID: "test"},
		ClientID:       "client",
		ClientSecret:   "secret",
		TokenURL:       "https://example.com/token",
	}
	if !cred1.NeedsRefresh() {
		t.Error("credential without access token should need refresh")
	}

	// Token expiring soon
	cred2 := &RESTOAuth2Credential{
		BaseCredential: BaseCredential{CredentialID: "test"},
		ClientID:       "client",
		ClientSecret:   "secret",
		TokenURL:       "https://example.com/token",
		AccessToken:    "token",
		TokenExpiry:    time.Now().Add(2 * time.Minute), // Expires in 2 min, threshold is 5 min
	}
	if !cred2.NeedsRefresh() {
		t.Error("credential expiring within 5 minutes should need refresh")
	}

	// Token still valid
	cred3 := &RESTOAuth2Credential{
		BaseCredential: BaseCredential{CredentialID: "test"},
		ClientID:       "client",
		ClientSecret:   "secret",
		TokenURL:       "https://example.com/token",
		AccessToken:    "token",
		TokenExpiry:    time.Now().Add(time.Hour),
	}
	if cred3.NeedsRefresh() {
		t.Error("credential with valid token should not need refresh")
	}

	// Token with zero expiry (never expires)
	cred4 := &RESTOAuth2Credential{
		BaseCredential: BaseCredential{CredentialID: "test"},
		ClientID:       "client",
		ClientSecret:   "secret",
		TokenURL:       "https://example.com/token",
		AccessToken:    "token",
	}
	if cred4.NeedsRefresh() {
		t.Error("credential with zero expiry should not need refresh")
	}
}

// =============================================================================
// Credential Store Tests
// =============================================================================

func TestInMemoryCredentialStore(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryCredentialStore()

	cred := &SSHPasswordCredential{
		BaseCredential: BaseCredential{
			CredentialID:   "test-cred",
			CredentialType: CredentialTypeSSHPassword,
			Description:    "Test Credential",
			Expires:        time.Now().Add(time.Hour),
		},
		Username: "admin",
		Password: "secret",
	}

	// Store credential
	if err := store.Store(ctx, cred); err != nil {
		t.Fatalf("Store() error = %v", err)
	}

	// Get credential
	got, err := store.Get(ctx, "test-cred")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	sshCred, ok := got.(*SSHPasswordCredential)
	if !ok {
		t.Fatalf("expected *SSHPasswordCredential, got %T", got)
	}

	if sshCred.Username != "admin" {
		t.Errorf("expected username admin, got %s", sshCred.Username)
	}

	// Check exists
	exists, err := store.Exists(ctx, "test-cred")
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}
	if !exists {
		t.Error("credential should exist")
	}

	// List credentials
	ids, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(ids) != 1 || ids[0] != "test-cred" {
		t.Errorf("expected [test-cred], got %v", ids)
	}

	// Delete credential
	if err := store.Delete(ctx, "test-cred"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// Verify deleted
	_, err = store.Get(ctx, "test-cred")
	if !errors.Is(err, ErrCredentialNotFound) {
		t.Errorf("expected ErrCredentialNotFound, got %v", err)
	}
}

func TestInMemoryCredentialStore_ExpiredCredential(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryCredentialStore()

	expired := &SSHPasswordCredential{
		BaseCredential: BaseCredential{
			CredentialID: "expired",
			Expires:      time.Now().Add(-time.Hour),
		},
		Username: "user",
		Password: "pass",
	}

	// Store expired credential
	if err := store.Store(ctx, expired); err != nil {
		t.Fatalf("Store() error = %v", err)
	}

	// Get should return expired error
	_, err := store.Get(ctx, "expired")
	if !errors.Is(err, ErrCredentialExpired) {
		t.Errorf("expected ErrCredentialExpired, got %v", err)
	}
}

func TestFileCredentialStore(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	// Create store with encryption
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}

	store, err := NewFileCredentialStore(&FileStoreConfig{
		BasePath:      tempDir,
		EncryptionKey: key,
	})
	if err != nil {
		t.Fatalf("NewFileCredentialStore() error = %v", err)
	}

	cred := &SSHPasswordCredential{
		BaseCredential: BaseCredential{
			CredentialID:   "file-test",
			CredentialType: CredentialTypeSSHPassword,
			Description:    "File Test",
			Expires:        time.Now().Add(time.Hour),
		},
		Username: "admin",
		Password: "secret",
	}

	// Store credential
	if err := store.Store(ctx, cred); err != nil {
		t.Fatalf("Store() error = %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(filepath.Join(tempDir, "file-test.cred")); os.IsNotExist(err) {
		t.Error("credential file should exist")
	}

	// Get credential
	got, err := store.Get(ctx, "file-test")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	sshCred, ok := got.(*SSHPasswordCredential)
	if !ok {
		t.Fatalf("expected *SSHPasswordCredential, got %T", got)
	}

	if sshCred.Username != "admin" {
		t.Errorf("expected username admin, got %s", sshCred.Username)
	}

	// Delete credential
	if err := store.Delete(ctx, "file-test"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// Verify file deleted
	if _, err := os.Stat(filepath.Join(tempDir, "file-test.cred")); !os.IsNotExist(err) {
		t.Error("credential file should be deleted")
	}
}

func TestCompositeCredentialStore(t *testing.T) {
	ctx := context.Background()

	store1 := NewInMemoryCredentialStore()
	store2 := NewInMemoryCredentialStore()
	composite := NewCompositeCredentialStore(store1, store2)

	cred := &SSHPasswordCredential{
		BaseCredential: BaseCredential{
			CredentialID: "composite-test",
			Expires:      time.Now().Add(time.Hour),
		},
		Username: "user",
		Password: "pass",
	}

	// Store in composite (goes to first store)
	if err := composite.Store(ctx, cred); err != nil {
		t.Fatalf("Store() error = %v", err)
	}

	// Should be retrievable
	_, err := composite.Get(ctx, "composite-test")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	// Store another credential directly in store2
	cred2 := &SSHPasswordCredential{
		BaseCredential: BaseCredential{
			CredentialID: "in-store2",
			Expires:      time.Now().Add(time.Hour),
		},
		Username: "user2",
		Password: "pass2",
	}
	if err := store2.Store(ctx, cred2); err != nil {
		t.Fatalf("store2.Store() error = %v", err)
	}

	// Should find it in composite
	got, err := composite.Get(ctx, "in-store2")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if got.ID() != "in-store2" {
		t.Errorf("expected ID in-store2, got %s", got.ID())
	}

	// List should include both
	ids, err := composite.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(ids) != 2 {
		t.Errorf("expected 2 credentials, got %d", len(ids))
	}
}

// =============================================================================
// Encryption Tests
// =============================================================================

func TestAESEncryptor(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}

	enc, err := NewAESEncryptor(key)
	if err != nil {
		t.Fatalf("NewAESEncryptor() error = %v", err)
	}

	plaintext := []byte("secret credential data")

	// Encrypt
	ciphertext, err := enc.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}

	// Ciphertext should be different from plaintext
	if string(ciphertext) == string(plaintext) {
		t.Error("ciphertext should differ from plaintext")
	}

	// Decrypt
	decrypted, err := enc.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}

	if string(decrypted) != string(plaintext) {
		t.Errorf("decrypted = %s, want %s", decrypted, plaintext)
	}
}

func TestAESEncryptor_InvalidKeySize(t *testing.T) {
	_, err := NewAESEncryptor([]byte("short"))
	if err == nil {
		t.Error("expected error for invalid key size")
	}
}

func TestX25519KeyPair(t *testing.T) {
	kp1, err := GenerateX25519KeyPair()
	if err != nil {
		t.Fatalf("GenerateX25519KeyPair() error = %v", err)
	}

	kp2, err := GenerateX25519KeyPair()
	if err != nil {
		t.Fatalf("GenerateX25519KeyPair() error = %v", err)
	}

	// Compute shared secrets (should be identical)
	secret1, err := kp1.ComputeSharedSecret(kp2.PublicKey)
	if err != nil {
		t.Fatalf("ComputeSharedSecret() error = %v", err)
	}

	secret2, err := kp2.ComputeSharedSecret(kp1.PublicKey)
	if err != nil {
		t.Fatalf("ComputeSharedSecret() error = %v", err)
	}

	if secret1 != secret2 {
		t.Error("shared secrets should be identical")
	}
}

func TestX25519Encryptor(t *testing.T) {
	kp1, _ := GenerateX25519KeyPair()
	kp2, _ := GenerateX25519KeyPair()

	enc1, err := NewX25519Encryptor(kp1, kp2.PublicKey)
	if err != nil {
		t.Fatalf("NewX25519Encryptor() error = %v", err)
	}

	enc2, err := NewX25519Encryptor(kp2, kp1.PublicKey)
	if err != nil {
		t.Fatalf("NewX25519Encryptor() error = %v", err)
	}

	plaintext := []byte("secret message for X25519")

	// Encrypt with enc1
	ciphertext, err := enc1.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}

	// Decrypt with enc2
	decrypted, err := enc2.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}

	if string(decrypted) != string(plaintext) {
		t.Errorf("decrypted = %s, want %s", decrypted, plaintext)
	}
}

func TestCredentialEncryptor(t *testing.T) {
	enc1, err := NewCredentialEncryptor()
	if err != nil {
		t.Fatalf("NewCredentialEncryptor() error = %v", err)
	}

	enc2, err := NewCredentialEncryptor()
	if err != nil {
		t.Fatalf("NewCredentialEncryptor() error = %v", err)
	}

	credData := []byte(`{"username":"admin","password":"secret"}`)

	// Encrypt for peer
	envelope, err := enc1.EncryptForPeer(CredentialTypeSSHPassword, credData, enc2.GetPublicKey())
	if err != nil {
		t.Fatalf("EncryptForPeer() error = %v", err)
	}

	if envelope.CredentialType != CredentialTypeSSHPassword {
		t.Errorf("expected credential type %s, got %s", CredentialTypeSSHPassword, envelope.CredentialType)
	}

	// Decrypt from peer
	decrypted, err := enc2.DecryptFromPeer(envelope)
	if err != nil {
		t.Fatalf("DecryptFromPeer() error = %v", err)
	}

	if string(decrypted) != string(credData) {
		t.Errorf("decrypted = %s, want %s", decrypted, credData)
	}
}

func TestDeriveKeyFromPassword(t *testing.T) {
	password := "my-secret-password"
	salt := []byte("random-salt-value-32-bytes-long!")

	key1, err := DeriveKeyFromPassword(password, salt)
	if err != nil {
		t.Fatalf("DeriveKeyFromPassword() error = %v", err)
	}

	// Same password and salt should produce same key
	key2, err := DeriveKeyFromPassword(password, salt)
	if err != nil {
		t.Fatalf("DeriveKeyFromPassword() error = %v", err)
	}

	if string(key1) != string(key2) {
		t.Error("same password and salt should produce same key")
	}

	// Different password should produce different key
	key3, err := DeriveKeyFromPassword("different-password", salt)
	if err != nil {
		t.Fatalf("DeriveKeyFromPassword() error = %v", err)
	}

	if string(key1) == string(key3) {
		t.Error("different password should produce different key")
	}
}

func TestGenerateRandomKey(t *testing.T) {
	key1, err := GenerateRandomKey(32)
	if err != nil {
		t.Fatalf("GenerateRandomKey() error = %v", err)
	}

	if len(key1) != 32 {
		t.Errorf("expected key length 32, got %d", len(key1))
	}

	key2, err := GenerateRandomKey(32)
	if err != nil {
		t.Fatalf("GenerateRandomKey() error = %v", err)
	}

	// Two random keys should be different
	if string(key1) == string(key2) {
		t.Error("two random keys should be different")
	}
}

// =============================================================================
// Cache Tests
// =============================================================================

func TestCredentialCache(t *testing.T) {
	ctx := context.Background()
	backend := NewInMemoryCredentialStore()
	cache := NewCredentialCache(DefaultCacheConfig(), backend)
	defer cache.Close()

	cred := &SSHPasswordCredential{
		BaseCredential: BaseCredential{
			CredentialID: "cache-test",
			Expires:      time.Now().Add(time.Hour),
		},
		Username: "admin",
		Password: "secret",
	}

	// Store in backend
	if err := backend.Store(ctx, cred); err != nil {
		t.Fatalf("backend.Store() error = %v", err)
	}

	// First get - cache miss, fetches from backend
	got, err := cache.Get(ctx, "cache-test")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if got.ID() != "cache-test" {
		t.Errorf("expected ID cache-test, got %s", got.ID())
	}

	// Second get - should be cached
	got2, err := cache.Get(ctx, "cache-test")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if got2.ID() != "cache-test" {
		t.Errorf("expected ID cache-test, got %s", got2.ID())
	}

	// Check stats
	stats := cache.Stats()
	if stats.Entries != 1 {
		t.Errorf("expected 1 cache entry, got %d", stats.Entries)
	}
}

func TestCredentialCache_TTL(t *testing.T) {
	ctx := context.Background()
	config := &CacheConfig{
		DefaultTTL:      50 * time.Millisecond,
		MaxEntries:      100,
		CleanupInterval: 10 * time.Millisecond,
	}
	backend := NewInMemoryCredentialStore()
	cache := NewCredentialCache(config, backend)
	defer cache.Close()

	cred := &SSHPasswordCredential{
		BaseCredential: BaseCredential{
			CredentialID: "ttl-test",
			Expires:      time.Now().Add(time.Hour),
		},
		Username: "admin",
		Password: "secret",
	}

	if err := backend.Store(ctx, cred); err != nil {
		t.Fatalf("backend.Store() error = %v", err)
	}

	// Get to cache it
	_, err := cache.Get(ctx, "ttl-test")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	start := time.Now()
	if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		return time.Since(start) >= 100*time.Millisecond, nil
	}); err != nil {
		t.Fatalf("TTL wait did not elapse: %v", err)
	}

	// Entry should be expired, will be fetched from backend again
	_, err = cache.Get(ctx, "ttl-test")
	if err != nil {
		t.Fatalf("Get() after expiry error = %v", err)
	}
}

func TestCredentialCache_Invalidate(t *testing.T) {
	ctx := context.Background()
	backend := NewInMemoryCredentialStore()
	cache := NewCredentialCache(DefaultCacheConfig(), backend)
	defer cache.Close()

	cred := &SSHPasswordCredential{
		BaseCredential: BaseCredential{
			CredentialID: "invalidate-test",
			Expires:      time.Now().Add(time.Hour),
		},
		Username: "admin",
		Password: "secret",
	}

	if err := backend.Store(ctx, cred); err != nil {
		t.Fatalf("backend.Store() error = %v", err)
	}

	// Get to cache it
	_, _ = cache.Get(ctx, "invalidate-test")

	stats := cache.Stats()
	if stats.Entries != 1 {
		t.Errorf("expected 1 entry, got %d", stats.Entries)
	}

	// Invalidate
	cache.Invalidate("invalidate-test")

	stats = cache.Stats()
	if stats.Entries != 0 {
		t.Errorf("expected 0 entries after invalidate, got %d", stats.Entries)
	}
}

func TestCredentialCache_MaxEntries(t *testing.T) {
	ctx := context.Background()
	config := &CacheConfig{
		DefaultTTL:      time.Minute,
		MaxEntries:      3,
		CleanupInterval: time.Hour,
	}
	backend := NewInMemoryCredentialStore()
	cache := NewCredentialCache(config, backend)
	defer cache.Close()

	// Store 5 credentials
	for i := 0; i < 5; i++ {
		cred := &SSHPasswordCredential{
			BaseCredential: BaseCredential{
				CredentialID: string(rune('a' + i)),
				Expires:      time.Now().Add(time.Hour),
			},
			Username: "user",
			Password: "pass",
		}
		if err := backend.Store(ctx, cred); err != nil {
			t.Fatalf("backend.Store() error = %v", err)
		}
		// Get to trigger caching
		_, _ = cache.Get(ctx, string(rune('a'+i)))
	}

	// Should only have MaxEntries cached
	stats := cache.Stats()
	if stats.Entries > config.MaxEntries {
		t.Errorf("expected at most %d entries, got %d", config.MaxEntries, stats.Entries)
	}
}

func TestCachedCredentialStore(t *testing.T) {
	ctx := context.Background()
	backend := NewInMemoryCredentialStore()
	cached := NewCachedCredentialStore(DefaultCacheConfig(), backend)
	defer cached.Close()

	cred := &SSHPasswordCredential{
		BaseCredential: BaseCredential{
			CredentialID: "cached-store-test",
			Expires:      time.Now().Add(time.Hour),
		},
		Username: "admin",
		Password: "secret",
	}

	// Store through cached store
	if err := cached.Store(ctx, cred); err != nil {
		t.Fatalf("Store() error = %v", err)
	}

	// Should be in cache
	stats := cached.Cache().Stats()
	if stats.Entries != 1 {
		t.Errorf("expected 1 cached entry, got %d", stats.Entries)
	}

	// Get should work
	got, err := cached.Get(ctx, "cached-store-test")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if got.ID() != "cached-store-test" {
		t.Errorf("expected ID cached-store-test, got %s", got.ID())
	}

	// Delete should invalidate cache
	if err := cached.Delete(ctx, "cached-store-test"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	stats = cached.Cache().Stats()
	if stats.Entries != 0 {
		t.Errorf("expected 0 entries after delete, got %d", stats.Entries)
	}
}

// =============================================================================
// Audit Logger Tests
// =============================================================================

func TestInMemoryAuditLogger(t *testing.T) {
	ctx := context.Background()
	logger := NewInMemoryAuditLogger(nil)

	event := &CredentialAccessEvent{
		CredentialRef: "vault://secret/path",
		ProxyAgentID:  "proxy-1",
		RequestID:     "req-123",
		Action:        AuditActionFetch,
		Timestamp:     time.Now(),
		Success:       true,
	}

	if err := logger.LogCredentialAccess(ctx, event); err != nil {
		t.Fatalf("LogCredentialAccess() error = %v", err)
	}

	events := logger.GetEvents()
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}

	if events[0].RequestID != "req-123" {
		t.Errorf("expected request ID req-123, got %s", events[0].RequestID)
	}
}

func TestInMemoryAuditLogger_MaxSize(t *testing.T) {
	ctx := context.Background()
	logger := NewInMemoryAuditLogger(&InMemoryAuditLoggerConfig{
		MaxSize: 3,
	})

	// Log 5 events
	for i := 0; i < 5; i++ {
		event := &CredentialAccessEvent{
			RequestID: string(rune('a' + i)),
			Timestamp: time.Now(),
		}
		_ = logger.LogCredentialAccess(ctx, event)
	}

	// Should only have last 3
	events := logger.GetEvents()
	if len(events) != 3 {
		t.Errorf("expected 3 events, got %d", len(events))
	}

	// First event should be 'c' (third one logged)
	if events[0].RequestID != "c" {
		t.Errorf("expected first event to be 'c', got %s", events[0].RequestID)
	}
}

func TestInMemoryAuditLogger_Filters(t *testing.T) {
	ctx := context.Background()
	logger := NewInMemoryAuditLogger(nil)

	now := time.Now()

	events := []*CredentialAccessEvent{
		{CredentialRef: "cred-1", ProxyAgentID: "proxy-1", Action: AuditActionFetch, Timestamp: now, Success: true},
		{CredentialRef: "cred-2", ProxyAgentID: "proxy-1", Action: AuditActionFetchFailed, Timestamp: now.Add(time.Minute), Success: false},
		{CredentialRef: "cred-1", ProxyAgentID: "proxy-2", Action: AuditActionFetch, Timestamp: now.Add(2 * time.Minute), Success: true},
	}

	for _, e := range events {
		_ = logger.LogCredentialAccess(ctx, e)
	}

	// Filter by credential
	byCredential := logger.GetEventsByCredential("cred-1")
	if len(byCredential) != 2 {
		t.Errorf("expected 2 events for cred-1, got %d", len(byCredential))
	}

	// Filter by proxy agent
	byAgent := logger.GetEventsByProxyAgent("proxy-1")
	if len(byAgent) != 2 {
		t.Errorf("expected 2 events for proxy-1, got %d", len(byAgent))
	}

	// Filter by time range
	byTime := logger.GetEventsByTimeRange(now.Add(30*time.Second), now.Add(3*time.Minute))
	if len(byTime) != 2 {
		t.Errorf("expected 2 events in time range, got %d", len(byTime))
	}
}

func TestInMemoryAuditLogger_FilteredQuery(t *testing.T) {
	ctx := context.Background()
	logger := NewInMemoryAuditLogger(nil)

	now := time.Now()

	for i := 0; i < 10; i++ {
		event := &CredentialAccessEvent{
			CredentialRef: "cred-1",
			ProxyAgentID:  "proxy-1",
			Action:        AuditActionFetch,
			Timestamp:     now.Add(time.Duration(i) * time.Minute),
			Success:       i%2 == 0,
		}
		_ = logger.LogCredentialAccess(ctx, event)
	}

	// Filter with limit
	filtered := logger.GetEventsFiltered(&AuditFilter{
		CredentialRef: "cred-1",
		Limit:         5,
	})
	if len(filtered) != 5 {
		t.Errorf("expected 5 events with limit, got %d", len(filtered))
	}

	// Filter success only
	successOnly := logger.GetEventsFiltered(&AuditFilter{
		SuccessOnly: true,
	})
	if len(successOnly) != 5 {
		t.Errorf("expected 5 successful events, got %d", len(successOnly))
	}

	// Filter failure only
	failureOnly := logger.GetEventsFiltered(&AuditFilter{
		FailureOnly: true,
	})
	if len(failureOnly) != 5 {
		t.Errorf("expected 5 failed events, got %d", len(failureOnly))
	}
}

func TestInMemoryAuditLogger_Summary(t *testing.T) {
	ctx := context.Background()
	logger := NewInMemoryAuditLogger(nil)

	now := time.Now()

	events := []*CredentialAccessEvent{
		{CredentialType: CredentialTypeSSHPassword, ProxyAgentID: "proxy-1", Action: AuditActionFetch, Timestamp: now, Success: true, Duration: time.Second},
		{CredentialType: CredentialTypeSSHPassword, ProxyAgentID: "proxy-1", Action: AuditActionFetchFailed, Timestamp: now.Add(time.Minute), Success: false, Duration: 2 * time.Second},
		{CredentialType: CredentialTypeSNMPv3, ProxyAgentID: "proxy-2", Action: AuditActionFetch, Timestamp: now.Add(2 * time.Minute), Success: true, Duration: 500 * time.Millisecond},
	}

	for _, e := range events {
		_ = logger.LogCredentialAccess(ctx, e)
	}

	summary := logger.GetSummary()

	if summary.TotalEvents != 3 {
		t.Errorf("expected 3 total events, got %d", summary.TotalEvents)
	}

	if summary.SuccessCount != 2 {
		t.Errorf("expected 2 successes, got %d", summary.SuccessCount)
	}

	if summary.FailureCount != 1 {
		t.Errorf("expected 1 failure, got %d", summary.FailureCount)
	}

	if len(summary.EventsByAction) != 2 {
		t.Errorf("expected 2 action types, got %d", len(summary.EventsByAction))
	}

	if len(summary.EventsByCredentialType) != 2 {
		t.Errorf("expected 2 credential types, got %d", len(summary.EventsByCredentialType))
	}

	if len(summary.EventsByProxyAgent) != 2 {
		t.Errorf("expected 2 proxy agents, got %d", len(summary.EventsByProxyAgent))
	}
}

func TestNoopAuditLogger(t *testing.T) {
	ctx := context.Background()
	logger := &NoopAuditLogger{}

	err := logger.LogCredentialAccess(ctx, &CredentialAccessEvent{})
	if err != nil {
		t.Errorf("NoopAuditLogger should not return error: %v", err)
	}
}

func TestMultiAuditLogger(t *testing.T) {
	ctx := context.Background()
	logger1 := NewInMemoryAuditLogger(nil)
	logger2 := NewInMemoryAuditLogger(nil)
	multi := NewMultiAuditLogger(logger1, logger2)

	event := &CredentialAccessEvent{
		RequestID: "multi-test",
		Timestamp: time.Now(),
	}

	if err := multi.LogCredentialAccess(ctx, event); err != nil {
		t.Fatalf("LogCredentialAccess() error = %v", err)
	}

	// Both loggers should have the event
	if logger1.GetEventCount() != 1 {
		t.Errorf("logger1 should have 1 event")
	}
	if logger2.GetEventCount() != 1 {
		t.Errorf("logger2 should have 1 event")
	}
}

// =============================================================================
// Proxy Credential Provider Tests
// =============================================================================

func TestParseCredentialRef(t *testing.T) {
	tests := []struct {
		name    string
		ref     string
		want    string
		wantErr bool
	}{
		{
			name:    "Direct ID",
			ref:     "id:my-credential-id",
			want:    "my-credential-id",
			wantErr: false,
		},
		{
			name:    "Vault reference",
			ref:     "vault://secret/data/myapp",
			want:    "vault://secret/data/myapp",
			wantErr: false,
		},
		{
			name:    "Kubernetes reference",
			ref:     "k8s://default/my-secret/password",
			want:    "k8s://default/my-secret/password",
			wantErr: false,
		},
		{
			name:    "File reference",
			ref:     "file:///etc/secrets/cred.json",
			want:    "file:///etc/secrets/cred.json",
			wantErr: false,
		},
		{
			name:    "Plain ID (no prefix)",
			ref:     "plain-credential-id",
			want:    "plain-credential-id",
			wantErr: false,
		},
		{
			name:    "Empty reference",
			ref:     "",
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseCredentialRef(tt.ref)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseCredentialRef() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ParseCredentialRef() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCredentialRefBuilder(t *testing.T) {
	builder := NewCredentialRefBuilder()

	tests := []struct {
		name     string
		method   func() string
		expected string
	}{
		{
			name:     "Vault reference",
			method:   func() string { return builder.Vault("secret/data/myapp") },
			expected: "vault://secret/data/myapp",
		},
		{
			name:     "K8s reference",
			method:   func() string { return builder.K8s("default", "my-secret", "password") },
			expected: "k8s://default/my-secret/password",
		},
		{
			name:     "File reference",
			method:   func() string { return builder.File("/etc/secrets/cred.json") },
			expected: "file:///etc/secrets/cred.json",
		},
		{
			name:     "ID reference",
			method:   func() string { return builder.ID("my-credential-id") },
			expected: "id:my-credential-id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.method()
			if got != tt.expected {
				t.Errorf("got %s, want %s", got, tt.expected)
			}
		})
	}
}

// =============================================================================
// Error Tests
// =============================================================================

func TestCredentialErrors(t *testing.T) {
	// Verify error messages are meaningful
	if ErrCredentialNotFound.Error() == "" {
		t.Error("ErrCredentialNotFound should have a message")
	}
	if ErrCredentialExpired.Error() == "" {
		t.Error("ErrCredentialExpired should have a message")
	}
	if ErrInvalidCredential.Error() == "" {
		t.Error("ErrInvalidCredential should have a message")
	}
	if ErrEncryptionFailed.Error() == "" {
		t.Error("ErrEncryptionFailed should have a message")
	}
	if ErrDecryptionFailed.Error() == "" {
		t.Error("ErrDecryptionFailed should have a message")
	}
}
