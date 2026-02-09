package rotation

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/shawnbutts/keystone-core/internal/credentials"
)

type mockSSHCommander struct {
	mu           sync.Mutex
	runErr       error
	testErr      error
	commands     []string
	testCalls    int
}

func (m *mockSSHCommander) RunCommand(_ context.Context, _ DeviceInfo, _ credentials.Credential, command string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.commands = append(m.commands, command)
	return "", m.runErr
}

func (m *mockSSHCommander) TestConnection(_ context.Context, _ DeviceInfo, _ credentials.Credential) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.testCalls++
	return m.testErr
}

var _ SSHCommander = (*mockSSHCommander)(nil)

func TestSSHRotationProvider_SupportsType(t *testing.T) {
	p := NewSSHRotationProvider(nil)

	tests := []struct {
		credType credentials.CredentialType
		want     bool
	}{
		{credentials.CredentialTypeSSHPassword, true},
		{credentials.CredentialTypeSSHKey, true},
		{credentials.CredentialTypeSNMPv2c, false},
		{credentials.CredentialTypeRESTBearer, false},
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

func TestSSHRotationProvider_ValidateOld(t *testing.T) {
	cmd := &mockSSHCommander{}
	p := NewSSHRotationProvider(cmd)

	cred := &credentials.SSHPasswordCredential{
		BaseCredential: credentials.BaseCredential{CredentialID: "c1", CredentialType: credentials.CredentialTypeSSHPassword},
		Username:       "admin",
		Password:       "pass",
	}

	err := p.ValidateOld(context.Background(), DeviceInfo{}, cred)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd.testCalls != 1 {
		t.Errorf("expected 1 test call, got %d", cmd.testCalls)
	}
}

func TestSSHRotationProvider_ValidateOldNoCommander(t *testing.T) {
	p := NewSSHRotationProvider(nil)
	err := p.ValidateOld(context.Background(), DeviceInfo{}, nil)
	if err == nil {
		t.Fatal("expected error without commander")
	}
}

func TestSSHRotationProvider_GeneratePassword(t *testing.T) {
	p := NewSSHRotationProvider(nil)
	p.PasswordLength = 16

	old := &credentials.SSHPasswordCredential{
		BaseCredential: credentials.BaseCredential{
			CredentialID:   "cred-1",
			CredentialType: credentials.CredentialTypeSSHPassword,
			Description:    "test cred",
		},
		Username: "admin",
		Password: "old-pass",
	}

	newCred, err := p.Generate(context.Background(), DeviceInfo{}, old)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	newSSH, ok := newCred.(*credentials.SSHPasswordCredential)
	if !ok {
		t.Fatalf("expected SSHPasswordCredential, got %T", newCred)
	}
	if newSSH.Username != "admin" {
		t.Errorf("expected username admin, got %s", newSSH.Username)
	}
	if newSSH.Password == "old-pass" {
		t.Error("password should be different from old")
	}
	if len(newSSH.Password) != 16 {
		t.Errorf("expected password length 16, got %d", len(newSSH.Password))
	}
	if newSSH.CredentialID != "cred-1" {
		t.Errorf("expected credential ID cred-1, got %s", newSSH.CredentialID)
	}
}

func TestSSHRotationProvider_GenerateKey(t *testing.T) {
	p := NewSSHRotationProvider(nil)

	old := &credentials.SSHKeyCredential{
		BaseCredential: credentials.BaseCredential{
			CredentialID:   "key-1",
			CredentialType: credentials.CredentialTypeSSHKey,
		},
		Username:   "admin",
		PrivateKey: []byte("old-key"),
	}

	newCred, err := p.Generate(context.Background(), DeviceInfo{}, old)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	newKey, ok := newCred.(*credentials.SSHKeyCredential)
	if !ok {
		t.Fatalf("expected SSHKeyCredential, got %T", newCred)
	}
	if newKey.Username != "admin" {
		t.Errorf("expected username admin, got %s", newKey.Username)
	}
	if len(newKey.PrivateKey) == 0 {
		t.Error("private key should not be empty")
	}
	if len(newKey.PublicKey) == 0 {
		t.Error("public key should not be empty")
	}
}

func TestSSHRotationProvider_GenerateUnsupported(t *testing.T) {
	p := NewSSHRotationProvider(nil)
	_, err := p.Generate(context.Background(), DeviceInfo{}, &credentials.SNMPv2cCredential{})
	if err == nil {
		t.Fatal("expected error for unsupported type")
	}
}

func TestSSHRotationProvider_ApplyPassword(t *testing.T) {
	cmd := &mockSSHCommander{}
	p := NewSSHRotationProvider(cmd)

	oldCred := &credentials.SSHPasswordCredential{
		BaseCredential: credentials.BaseCredential{CredentialID: "c1", CredentialType: credentials.CredentialTypeSSHPassword},
		Username:       "admin",
		Password:       "old-pass",
	}
	newCred := &credentials.SSHPasswordCredential{
		BaseCredential: credentials.BaseCredential{CredentialID: "c1", CredentialType: credentials.CredentialTypeSSHPassword},
		Username:       "admin",
		Password:       "new-pass",
	}

	err := p.Apply(context.Background(), DeviceInfo{}, oldCred, newCred)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cmd.commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmd.commands))
	}
}

func TestSSHRotationProvider_ApplyNoCommander(t *testing.T) {
	p := NewSSHRotationProvider(nil)
	err := p.Apply(context.Background(), DeviceInfo{}, nil, &credentials.SSHPasswordCredential{})
	if err == nil {
		t.Fatal("expected error without commander")
	}
}

func TestSSHRotationProvider_Verify(t *testing.T) {
	cmd := &mockSSHCommander{}
	p := NewSSHRotationProvider(cmd)

	err := p.Verify(context.Background(), DeviceInfo{}, &credentials.SSHPasswordCredential{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd.testCalls != 1 {
		t.Errorf("expected 1 test call, got %d", cmd.testCalls)
	}
}

func TestSSHRotationProvider_VerifyFails(t *testing.T) {
	cmd := &mockSSHCommander{testErr: errors.New("connection refused")}
	p := NewSSHRotationProvider(cmd)

	err := p.Verify(context.Background(), DeviceInfo{}, &credentials.SSHPasswordCredential{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSSHRotationProvider_RollbackPassword(t *testing.T) {
	cmd := &mockSSHCommander{}
	p := NewSSHRotationProvider(cmd)

	oldCred := &credentials.SSHPasswordCredential{
		BaseCredential: credentials.BaseCredential{CredentialID: "c1", CredentialType: credentials.CredentialTypeSSHPassword},
		Username:       "admin",
		Password:       "old-pass",
	}
	newCred := &credentials.SSHPasswordCredential{
		BaseCredential: credentials.BaseCredential{CredentialID: "c1", CredentialType: credentials.CredentialTypeSSHPassword},
		Username:       "admin",
		Password:       "new-pass",
	}

	err := p.Rollback(context.Background(), DeviceInfo{}, oldCred, newCred)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cmd.commands) != 1 {
		t.Errorf("expected 1 command, got %d", len(cmd.commands))
	}
}

func TestSSHRotationProvider_Cleanup(t *testing.T) {
	p := NewSSHRotationProvider(nil)
	err := p.Cleanup(context.Background(), DeviceInfo{}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSSHRotationProvider_InterfaceCompliance(t *testing.T) {
	var _ RotationProvider = (*SSHRotationProvider)(nil)
}

func TestGenerateRandomPassword(t *testing.T) {
	p1, err := generateRandomPassword(32)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(p1) != 32 {
		t.Errorf("expected length 32, got %d", len(p1))
	}

	p2, err := generateRandomPassword(32)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Two random passwords should be different (extremely unlikely to be the same)
	if p1 == p2 {
		t.Error("two random passwords should differ")
	}
}
