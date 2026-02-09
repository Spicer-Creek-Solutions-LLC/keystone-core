package rotation

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/credentials"
)

type mockCertCommander struct {
	mu            sync.Mutex
	installErr    error
	verifyErr     error
	removeErr     error
	installCalls  int
	verifyCalls   int
	removeCalls   int
}

func (m *mockCertCommander) InstallCertificate(_ context.Context, _ DeviceInfo, _, _ []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.installCalls++
	return m.installErr
}

func (m *mockCertCommander) VerifyCertificate(_ context.Context, _ DeviceInfo, _ []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.verifyCalls++
	return m.verifyErr
}

func (m *mockCertCommander) RemoveCertificate(_ context.Context, _ DeviceInfo, _ []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removeCalls++
	return m.removeErr
}

var _ CertificateCommander = (*mockCertCommander)(nil)

func TestCertificateRotationProvider_SupportsType(t *testing.T) {
	p := NewCertificateRotationProvider(nil)

	tests := []struct {
		credType credentials.CredentialType
		want     bool
	}{
		{credentials.CredentialTypeGNMI, true},
		{credentials.CredentialTypeSSHPassword, false},
		{credentials.CredentialTypeSNMPv2c, false},
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

func TestCertificateRotationProvider_ValidateOld(t *testing.T) {
	cmd := &mockCertCommander{}
	p := NewCertificateRotationProvider(cmd)

	cred := &credentials.GNMICredential{
		BaseCredential: credentials.BaseCredential{
			CredentialID:   "gnmi-1",
			CredentialType: credentials.CredentialTypeGNMI,
		},
		ClientCert: []byte("old-cert"),
	}

	err := p.ValidateOld(context.Background(), DeviceInfo{}, cred)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd.verifyCalls != 1 {
		t.Errorf("expected 1 verify call, got %d", cmd.verifyCalls)
	}
}

func TestCertificateRotationProvider_ValidateOldWrongType(t *testing.T) {
	cmd := &mockCertCommander{}
	p := NewCertificateRotationProvider(cmd)

	err := p.ValidateOld(context.Background(), DeviceInfo{}, &credentials.SSHPasswordCredential{})
	if err == nil {
		t.Fatal("expected error for wrong credential type")
	}
}

func TestCertificateRotationProvider_ValidateOldNoCommander(t *testing.T) {
	p := NewCertificateRotationProvider(nil)

	cred := &credentials.GNMICredential{
		BaseCredential: credentials.BaseCredential{CredentialID: "g1"},
	}
	err := p.ValidateOld(context.Background(), DeviceInfo{}, cred)
	if err == nil {
		t.Fatal("expected error without commander")
	}
}

func TestCertificateRotationProvider_Generate(t *testing.T) {
	p := NewCertificateRotationProvider(nil)
	p.CertValidity = 30 * 24 * time.Hour

	old := &credentials.GNMICredential{
		BaseCredential: credentials.BaseCredential{
			CredentialID:   "gnmi-1",
			CredentialType: credentials.CredentialTypeGNMI,
			Description:    "test cert",
		},
		Username:   "admin",
		Password:   "pass",
		CACert:     []byte("ca-cert"),
		ClientCert: []byte("old-cert"),
		ClientKey:  []byte("old-key"),
		SkipVerify: true,
	}

	device := DeviceInfo{Host: "switch1.example.com"}
	newCred, err := p.Generate(context.Background(), device, old)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	gnmi, ok := newCred.(*credentials.GNMICredential)
	if !ok {
		t.Fatalf("expected GNMICredential, got %T", newCred)
	}

	if gnmi.CredentialID != "gnmi-1" {
		t.Errorf("expected ID gnmi-1, got %s", gnmi.CredentialID)
	}
	if gnmi.Username != "admin" {
		t.Error("username should be preserved")
	}
	if gnmi.Password != "pass" {
		t.Error("password should be preserved")
	}
	if string(gnmi.CACert) != "ca-cert" {
		t.Error("CA cert should be preserved")
	}
	if !gnmi.SkipVerify {
		t.Error("skip verify should be preserved")
	}
	if len(gnmi.ClientCert) == 0 {
		t.Error("client cert should be generated")
	}
	if len(gnmi.ClientKey) == 0 {
		t.Error("client key should be generated")
	}
	if string(gnmi.ClientCert) == "old-cert" {
		t.Error("client cert should be different from old")
	}
	if gnmi.Expires.IsZero() {
		t.Error("expiry should be set")
	}
}

func TestCertificateRotationProvider_GenerateWrongType(t *testing.T) {
	p := NewCertificateRotationProvider(nil)
	_, err := p.Generate(context.Background(), DeviceInfo{}, &credentials.SSHPasswordCredential{})
	if err == nil {
		t.Fatal("expected error for wrong type")
	}
}

func TestCertificateRotationProvider_Apply(t *testing.T) {
	cmd := &mockCertCommander{}
	p := NewCertificateRotationProvider(cmd)

	newCred := &credentials.GNMICredential{
		BaseCredential: credentials.BaseCredential{CredentialID: "g1"},
		ClientCert:     []byte("new-cert"),
		ClientKey:      []byte("new-key"),
	}

	err := p.Apply(context.Background(), DeviceInfo{}, nil, newCred)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd.installCalls != 1 {
		t.Errorf("expected 1 install call, got %d", cmd.installCalls)
	}
}

func TestCertificateRotationProvider_ApplyNoCommander(t *testing.T) {
	p := NewCertificateRotationProvider(nil)
	err := p.Apply(context.Background(), DeviceInfo{}, nil, &credentials.GNMICredential{})
	if err == nil {
		t.Fatal("expected error without commander")
	}
}

func TestCertificateRotationProvider_ApplyWrongType(t *testing.T) {
	cmd := &mockCertCommander{}
	p := NewCertificateRotationProvider(cmd)
	err := p.Apply(context.Background(), DeviceInfo{}, nil, &credentials.SSHPasswordCredential{})
	if err == nil {
		t.Fatal("expected error for wrong type")
	}
}

func TestCertificateRotationProvider_Verify(t *testing.T) {
	cmd := &mockCertCommander{}
	p := NewCertificateRotationProvider(cmd)

	newCred := &credentials.GNMICredential{
		BaseCredential: credentials.BaseCredential{CredentialID: "g1"},
		ClientCert:     []byte("new-cert"),
	}

	err := p.Verify(context.Background(), DeviceInfo{}, newCred)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd.verifyCalls != 1 {
		t.Errorf("expected 1 verify call, got %d", cmd.verifyCalls)
	}
}

func TestCertificateRotationProvider_VerifyFails(t *testing.T) {
	cmd := &mockCertCommander{verifyErr: errors.New("cert invalid")}
	p := NewCertificateRotationProvider(cmd)

	err := p.Verify(context.Background(), DeviceInfo{}, &credentials.GNMICredential{ClientCert: []byte("bad")})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCertificateRotationProvider_Rollback(t *testing.T) {
	cmd := &mockCertCommander{}
	p := NewCertificateRotationProvider(cmd)

	oldCred := &credentials.GNMICredential{
		BaseCredential: credentials.BaseCredential{CredentialID: "g1"},
		ClientCert:     []byte("old-cert"),
		ClientKey:      []byte("old-key"),
	}
	newCred := &credentials.GNMICredential{
		BaseCredential: credentials.BaseCredential{CredentialID: "g1"},
		ClientCert:     []byte("new-cert"),
	}

	err := p.Rollback(context.Background(), DeviceInfo{}, oldCred, newCred)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd.removeCalls != 1 {
		t.Errorf("expected 1 remove call, got %d", cmd.removeCalls)
	}
	if cmd.installCalls != 1 {
		t.Errorf("expected 1 install call (reinstall old), got %d", cmd.installCalls)
	}
}

func TestCertificateRotationProvider_RollbackWrongType(t *testing.T) {
	cmd := &mockCertCommander{}
	p := NewCertificateRotationProvider(cmd)

	err := p.Rollback(context.Background(), DeviceInfo{}, &credentials.SSHPasswordCredential{}, &credentials.GNMICredential{})
	if err == nil {
		t.Fatal("expected error for wrong old cred type")
	}
}

func TestCertificateRotationProvider_Cleanup(t *testing.T) {
	cmd := &mockCertCommander{}
	p := NewCertificateRotationProvider(cmd)

	oldCred := &credentials.GNMICredential{
		BaseCredential: credentials.BaseCredential{CredentialID: "g1"},
		ClientCert:     []byte("old-cert"),
	}

	err := p.Cleanup(context.Background(), DeviceInfo{}, oldCred)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd.removeCalls != 1 {
		t.Errorf("expected 1 remove call, got %d", cmd.removeCalls)
	}
}

func TestCertificateRotationProvider_CleanupNilCommander(t *testing.T) {
	p := NewCertificateRotationProvider(nil)
	err := p.Cleanup(context.Background(), DeviceInfo{}, &credentials.GNMICredential{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCertificateRotationProvider_InterfaceCompliance(t *testing.T) {
	var _ RotationProvider = (*CertificateRotationProvider)(nil)
}
