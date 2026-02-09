package rotation

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/shawnbutts/keystone-core/internal/credentials"
)

type mockSNMPCommander struct {
	mu             sync.Mutex
	setCommunityErr    error
	setV3Err           error
	testErr            error
	communitySets      []string
	v3Usernames        []string
	testCalls          int
}

func (m *mockSNMPCommander) SetCommunity(_ context.Context, _ DeviceInfo, _ credentials.Credential, newCommunity string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.communitySets = append(m.communitySets, newCommunity)
	return m.setCommunityErr
}

func (m *mockSNMPCommander) SetSNMPv3Credentials(_ context.Context, _ DeviceInfo, _ credentials.Credential, username, _, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.v3Usernames = append(m.v3Usernames, username)
	return m.setV3Err
}

func (m *mockSNMPCommander) TestSNMPAccess(_ context.Context, _ DeviceInfo, _ credentials.Credential) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.testCalls++
	return m.testErr
}

var _ SNMPCommander = (*mockSNMPCommander)(nil)

func TestSNMPProvider_SupportsType(t *testing.T) {
	p := NewSNMPProvider(nil)

	tests := []struct {
		credType credentials.CredentialType
		want     bool
	}{
		{credentials.CredentialTypeSNMPv2c, true},
		{credentials.CredentialTypeSNMPv3, true},
		{credentials.CredentialTypeSSHPassword, false},
		{credentials.CredentialTypeRESTBearer, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.credType), func(t *testing.T) {
			if got := p.SupportsType(tt.credType); got != tt.want {
				t.Errorf("SupportsType(%s) = %v, want %v", tt.credType, got, tt.want)
			}
		})
	}
}

func TestSNMPProvider_ValidateOld(t *testing.T) {
	cmd := &mockSNMPCommander{}
	p := NewSNMPProvider(cmd)

	err := p.ValidateOld(context.Background(), DeviceInfo{}, &credentials.SNMPv2cCredential{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd.testCalls != 1 {
		t.Errorf("expected 1 test call, got %d", cmd.testCalls)
	}
}

func TestSNMPProvider_ValidateOldNoCommander(t *testing.T) {
	p := NewSNMPProvider(nil)
	err := p.ValidateOld(context.Background(), DeviceInfo{}, nil)
	if err == nil {
		t.Fatal("expected error without commander")
	}
}

func TestSNMPProvider_GenerateV2c(t *testing.T) {
	p := NewSNMPProvider(nil)
	p.CommunityLength = 16

	old := &credentials.SNMPv2cCredential{
		BaseCredential: credentials.BaseCredential{
			CredentialID:   "snmp-1",
			CredentialType: credentials.CredentialTypeSNMPv2c,
			Description:    "test v2c",
		},
		Community: "old-community",
	}

	newCred, err := p.Generate(context.Background(), DeviceInfo{}, old)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	v2c, ok := newCred.(*credentials.SNMPv2cCredential)
	if !ok {
		t.Fatalf("expected SNMPv2cCredential, got %T", newCred)
	}
	if v2c.Community == "old-community" {
		t.Error("community should be different")
	}
	if len(v2c.Community) != 16 {
		t.Errorf("expected community length 16, got %d", len(v2c.Community))
	}
	if v2c.CredentialID != "snmp-1" {
		t.Errorf("expected ID snmp-1, got %s", v2c.CredentialID)
	}
}

func TestSNMPProvider_GenerateV3AuthPriv(t *testing.T) {
	p := NewSNMPProvider(nil)
	p.PasswordLength = 20

	old := &credentials.SNMPv3Credential{
		BaseCredential: credentials.BaseCredential{
			CredentialID:   "snmpv3-1",
			CredentialType: credentials.CredentialTypeSNMPv3,
		},
		Username:        "snmpuser",
		SecurityLevel:   credentials.SNMPv3SecurityAuthPriv,
		AuthProtocol:    credentials.SNMPv3AuthSHA256,
		AuthPassword:    "old-auth-pass",
		PrivacyProtocol: credentials.SNMPv3PrivAES256,
		PrivacyPassword: "old-priv-pass",
		ContextName:     "ctx1",
	}

	newCred, err := p.Generate(context.Background(), DeviceInfo{}, old)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	v3, ok := newCred.(*credentials.SNMPv3Credential)
	if !ok {
		t.Fatalf("expected SNMPv3Credential, got %T", newCred)
	}
	if v3.Username != "snmpuser" {
		t.Errorf("expected username snmpuser, got %s", v3.Username)
	}
	if v3.AuthPassword == "old-auth-pass" {
		t.Error("auth password should be rotated")
	}
	if v3.PrivacyPassword == "old-priv-pass" {
		t.Error("privacy password should be rotated")
	}
	if len(v3.AuthPassword) != 20 {
		t.Errorf("expected auth password length 20, got %d", len(v3.AuthPassword))
	}
	if v3.SecurityLevel != credentials.SNMPv3SecurityAuthPriv {
		t.Error("security level should be preserved")
	}
	if v3.AuthProtocol != credentials.SNMPv3AuthSHA256 {
		t.Error("auth protocol should be preserved")
	}
	if v3.ContextName != "ctx1" {
		t.Error("context name should be preserved")
	}
}

func TestSNMPProvider_GenerateV3AuthNoPriv(t *testing.T) {
	p := NewSNMPProvider(nil)

	old := &credentials.SNMPv3Credential{
		BaseCredential: credentials.BaseCredential{
			CredentialID:   "snmpv3-2",
			CredentialType: credentials.CredentialTypeSNMPv3,
		},
		Username:      "user2",
		SecurityLevel: credentials.SNMPv3SecurityAuthNoPriv,
		AuthProtocol:  credentials.SNMPv3AuthSHA,
		AuthPassword:  "old-auth",
	}

	newCred, err := p.Generate(context.Background(), DeviceInfo{}, old)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	v3 := newCred.(*credentials.SNMPv3Credential)
	if v3.AuthPassword == "old-auth" {
		t.Error("auth password should be rotated")
	}
	if v3.PrivacyPassword != "" {
		t.Error("privacy password should be empty for authNoPriv")
	}
}

func TestSNMPProvider_GenerateUnsupported(t *testing.T) {
	p := NewSNMPProvider(nil)
	_, err := p.Generate(context.Background(), DeviceInfo{}, &credentials.SSHPasswordCredential{})
	if err == nil {
		t.Fatal("expected error for unsupported type")
	}
}

func TestSNMPProvider_ApplyV2c(t *testing.T) {
	cmd := &mockSNMPCommander{}
	p := NewSNMPProvider(cmd)

	newCred := &credentials.SNMPv2cCredential{
		BaseCredential: credentials.BaseCredential{CredentialID: "s1"},
		Community:      "new-community",
	}

	err := p.Apply(context.Background(), DeviceInfo{}, &credentials.SNMPv2cCredential{}, newCred)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cmd.communitySets) != 1 || cmd.communitySets[0] != "new-community" {
		t.Error("expected SetCommunity with new-community")
	}
}

func TestSNMPProvider_ApplyV3(t *testing.T) {
	cmd := &mockSNMPCommander{}
	p := NewSNMPProvider(cmd)

	newCred := &credentials.SNMPv3Credential{
		BaseCredential: credentials.BaseCredential{CredentialID: "s1"},
		Username:       "user1",
		AuthPassword:   "new-auth",
		PrivacyPassword: "new-priv",
	}

	err := p.Apply(context.Background(), DeviceInfo{}, &credentials.SNMPv3Credential{}, newCred)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cmd.v3Usernames) != 1 || cmd.v3Usernames[0] != "user1" {
		t.Error("expected SetSNMPv3Credentials with user1")
	}
}

func TestSNMPProvider_ApplyFails(t *testing.T) {
	cmd := &mockSNMPCommander{setCommunityErr: errors.New("device error")}
	p := NewSNMPProvider(cmd)

	err := p.Apply(context.Background(), DeviceInfo{}, &credentials.SNMPv2cCredential{}, &credentials.SNMPv2cCredential{Community: "x"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSNMPProvider_Verify(t *testing.T) {
	cmd := &mockSNMPCommander{}
	p := NewSNMPProvider(cmd)

	err := p.Verify(context.Background(), DeviceInfo{}, &credentials.SNMPv2cCredential{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSNMPProvider_RollbackV2c(t *testing.T) {
	cmd := &mockSNMPCommander{}
	p := NewSNMPProvider(cmd)

	oldCred := &credentials.SNMPv2cCredential{
		BaseCredential: credentials.BaseCredential{CredentialID: "s1"},
		Community:      "old-community",
	}

	err := p.Rollback(context.Background(), DeviceInfo{}, oldCred, &credentials.SNMPv2cCredential{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cmd.communitySets) != 1 || cmd.communitySets[0] != "old-community" {
		t.Error("expected rollback to old-community")
	}
}

func TestSNMPProvider_Cleanup(t *testing.T) {
	p := NewSNMPProvider(nil)
	err := p.Cleanup(context.Background(), DeviceInfo{}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSNMPProvider_InterfaceCompliance(t *testing.T) {
	var _ Provider = (*SNMPProvider)(nil)
}
