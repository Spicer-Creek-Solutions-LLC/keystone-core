package rotation

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/shawnbutts/keystone-core/internal/credentials"
)

type mockRESTCommander struct {
	mu              sync.Mutex
	rotatePassErr   error
	rotateTokenErr  error
	rotateKeyErr    error
	rotateOAuthErr  error
	testErr         error
	revokeErr       error
	newPassword     string
	newToken        string
	newAPIKey       string
	newOAuthSecret  string
	testCalls       int
	revokeCalls     int
}

func (m *mockRESTCommander) RotatePassword(_ context.Context, _ DeviceInfo, _ *credentials.RESTBasicCredential) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.newPassword == "" {
		m.newPassword = "rotated-pass-123"
	}
	return m.newPassword, m.rotatePassErr
}

func (m *mockRESTCommander) RotateToken(_ context.Context, _ DeviceInfo, _ *credentials.RESTBearerCredential) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.newToken == "" {
		m.newToken = "rotated-token-456"
	}
	return m.newToken, m.rotateTokenErr
}

func (m *mockRESTCommander) RotateAPIKey(_ context.Context, _ DeviceInfo, _ *credentials.RESTAPIKeyCredential) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.newAPIKey == "" {
		m.newAPIKey = "rotated-apikey-789"
	}
	return m.newAPIKey, m.rotateKeyErr
}

func (m *mockRESTCommander) RotateOAuth2Secret(_ context.Context, _ DeviceInfo, _ *credentials.RESTOAuth2Credential) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.newOAuthSecret == "" {
		m.newOAuthSecret = "rotated-secret-abc"
	}
	return m.newOAuthSecret, m.rotateOAuthErr
}

func (m *mockRESTCommander) TestRESTAccess(_ context.Context, _ DeviceInfo, _ credentials.Credential) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.testCalls++
	return m.testErr
}

func (m *mockRESTCommander) RevokeCredential(_ context.Context, _ DeviceInfo, _ credentials.Credential) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.revokeCalls++
	return m.revokeErr
}

var _ RESTCommander = (*mockRESTCommander)(nil)

func TestRESTProvider_SupportsType(t *testing.T) {
	p := NewRESTProvider(nil)

	tests := []struct {
		credType credentials.CredentialType
		want     bool
	}{
		{credentials.CredentialTypeRESTBasic, true},
		{credentials.CredentialTypeRESTBearer, true},
		{credentials.CredentialTypeRESTAPIKey, true},
		{credentials.CredentialTypeRESTOAuth2, true},
		{credentials.CredentialTypeSSHPassword, false},
		{credentials.CredentialTypeSNMPv2c, false},
		{credentials.CredentialTypeGNMI, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.credType), func(t *testing.T) {
			if got := p.SupportsType(tt.credType); got != tt.want {
				t.Errorf("SupportsType(%s) = %v, want %v", tt.credType, got, tt.want)
			}
		})
	}
}

func TestRESTProvider_GenerateBasicAuth(t *testing.T) {
	cmd := &mockRESTCommander{}
	p := NewRESTProvider(cmd)

	old := &credentials.RESTBasicCredential{
		BaseCredential: credentials.BaseCredential{
			CredentialID:   "rest-1",
			CredentialType: credentials.CredentialTypeRESTBasic,
		},
		Username: "admin",
		Password: "old-pass",
	}

	newCred, err := p.Generate(context.Background(), DeviceInfo{}, old)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	basic, ok := newCred.(*credentials.RESTBasicCredential)
	if !ok {
		t.Fatalf("expected RESTBasicCredential, got %T", newCred)
	}
	if basic.Username != "admin" {
		t.Error("username should be preserved")
	}
	if basic.Password != "rotated-pass-123" {
		t.Errorf("expected rotated password, got %s", basic.Password)
	}
}

func TestRESTProvider_GenerateBearer(t *testing.T) {
	cmd := &mockRESTCommander{}
	p := NewRESTProvider(cmd)

	old := &credentials.RESTBearerCredential{
		BaseCredential: credentials.BaseCredential{
			CredentialID:   "rest-2",
			CredentialType: credentials.CredentialTypeRESTBearer,
		},
		Token: "old-token",
	}

	newCred, err := p.Generate(context.Background(), DeviceInfo{}, old)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	bearer := newCred.(*credentials.RESTBearerCredential)
	if bearer.Token != "rotated-token-456" {
		t.Errorf("expected rotated token, got %s", bearer.Token)
	}
}

func TestRESTProvider_GenerateAPIKey(t *testing.T) {
	cmd := &mockRESTCommander{}
	p := NewRESTProvider(cmd)

	old := &credentials.RESTAPIKeyCredential{
		BaseCredential: credentials.BaseCredential{
			CredentialID:   "rest-3",
			CredentialType: credentials.CredentialTypeRESTAPIKey,
		},
		APIKey:     "old-key",
		HeaderName: "X-Custom-Key",
	}

	newCred, err := p.Generate(context.Background(), DeviceInfo{}, old)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	apiKey := newCred.(*credentials.RESTAPIKeyCredential)
	if apiKey.APIKey != "rotated-apikey-789" {
		t.Errorf("expected rotated API key, got %s", apiKey.APIKey)
	}
	if apiKey.HeaderName != "X-Custom-Key" {
		t.Error("header name should be preserved")
	}
}

func TestRESTProvider_GenerateOAuth2(t *testing.T) {
	cmd := &mockRESTCommander{}
	p := NewRESTProvider(cmd)

	old := &credentials.RESTOAuth2Credential{
		BaseCredential: credentials.BaseCredential{
			CredentialID:   "rest-4",
			CredentialType: credentials.CredentialTypeRESTOAuth2,
		},
		ClientID:     "client-1",
		ClientSecret: "old-secret",
		TokenURL:     "https://auth.example.com/token",
		Scopes:       []string{"read", "write"},
	}

	newCred, err := p.Generate(context.Background(), DeviceInfo{}, old)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	oauth2 := newCred.(*credentials.RESTOAuth2Credential)
	if oauth2.ClientSecret != "rotated-secret-abc" {
		t.Errorf("expected rotated secret, got %s", oauth2.ClientSecret)
	}
	if oauth2.ClientID != "client-1" {
		t.Error("client ID should be preserved")
	}
	if oauth2.TokenURL != "https://auth.example.com/token" {
		t.Error("token URL should be preserved")
	}
}

func TestRESTProvider_GenerateNoCommander(t *testing.T) {
	p := NewRESTProvider(nil)
	_, err := p.Generate(context.Background(), DeviceInfo{}, &credentials.RESTBasicCredential{})
	if err == nil {
		t.Fatal("expected error without commander")
	}
}

func TestRESTProvider_GenerateUnsupported(t *testing.T) {
	cmd := &mockRESTCommander{}
	p := NewRESTProvider(cmd)
	_, err := p.Generate(context.Background(), DeviceInfo{}, &credentials.SSHPasswordCredential{})
	if err == nil {
		t.Fatal("expected error for unsupported type")
	}
}

func TestRESTProvider_GenerateFailure(t *testing.T) {
	cmd := &mockRESTCommander{rotatePassErr: errors.New("API error")}
	p := NewRESTProvider(cmd)

	old := &credentials.RESTBasicCredential{
		BaseCredential: credentials.BaseCredential{CredentialID: "r1", CredentialType: credentials.CredentialTypeRESTBasic},
		Username:       "admin",
		Password:       "pass",
	}

	_, err := p.Generate(context.Background(), DeviceInfo{}, old)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRESTProvider_Apply(t *testing.T) {
	p := NewRESTProvider(nil)
	// Apply is a no-op for REST (generation already applied)
	err := p.Apply(context.Background(), DeviceInfo{}, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRESTProvider_Verify(t *testing.T) {
	cmd := &mockRESTCommander{}
	p := NewRESTProvider(cmd)

	err := p.Verify(context.Background(), DeviceInfo{}, &credentials.RESTBasicCredential{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd.testCalls != 1 {
		t.Errorf("expected 1 test call, got %d", cmd.testCalls)
	}
}

func TestRESTProvider_Rollback(t *testing.T) {
	cmd := &mockRESTCommander{}
	p := NewRESTProvider(cmd)

	err := p.Rollback(context.Background(), DeviceInfo{}, &credentials.RESTBasicCredential{}, &credentials.RESTBasicCredential{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd.revokeCalls != 1 {
		t.Errorf("expected 1 revoke call, got %d", cmd.revokeCalls)
	}
}

func TestRESTProvider_Cleanup(t *testing.T) {
	cmd := &mockRESTCommander{}
	p := NewRESTProvider(cmd)

	err := p.Cleanup(context.Background(), DeviceInfo{}, &credentials.RESTBasicCredential{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd.revokeCalls != 1 {
		t.Errorf("expected 1 revoke call for cleanup, got %d", cmd.revokeCalls)
	}
}

func TestRESTProvider_CleanupNilCommander(t *testing.T) {
	p := NewRESTProvider(nil)
	err := p.Cleanup(context.Background(), DeviceInfo{}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRESTProvider_InterfaceCompliance(t *testing.T) {
	var _ Provider = (*RESTProvider)(nil)
}
