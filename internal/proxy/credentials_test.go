package proxy

import (
	"context"
	"testing"
)

// Verify interface compliance.
var _ CredentialProvider = (*EnvCredentialProvider)(nil)

func TestEnvCredentialProvider_GetCredential(t *testing.T) {
	t.Setenv("KSCORE_CRED_ROUTER_CREDS_USERNAME", "admin")
	t.Setenv("KSCORE_CRED_ROUTER_CREDS_PASSWORD", "secret123")

	p := NewEnvCredentialProvider("")
	cred, err := p.GetCredential(context.Background(), "router-creds")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cred.Username != "admin" {
		t.Errorf("Username = %q, want admin", cred.Username)
	}
	if cred.Password != "secret123" {
		t.Errorf("Password = %q, want secret123", cred.Password)
	}
	if cred.Type != "password" {
		t.Errorf("Type = %q, want password", cred.Type)
	}
}

func TestEnvCredentialProvider_Token(t *testing.T) {
	t.Setenv("KSCORE_CRED_API_KEY_TOKEN", "bearer-abc123")

	p := NewEnvCredentialProvider("")
	cred, err := p.GetCredential(context.Background(), "api-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cred.Token != "bearer-abc123" {
		t.Errorf("Token = %q, want bearer-abc123", cred.Token)
	}
	if cred.Type != "token" {
		t.Errorf("Type = %q, want token", cred.Type)
	}
}

func TestEnvCredentialProvider_SNMPCommunity(t *testing.T) {
	t.Setenv("KSCORE_CRED_SNMP_RO_COMMUNITY", "public")

	p := NewEnvCredentialProvider("")
	cred, err := p.GetCredential(context.Background(), "snmp-ro")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cred.Community != "public" {
		t.Errorf("Community = %q, want public", cred.Community)
	}
	if cred.Type != "snmp_community" {
		t.Errorf("Type = %q, want snmp_community", cred.Type)
	}
}

func TestEnvCredentialProvider_ExplicitType(t *testing.T) {
	t.Setenv("KSCORE_CRED_SSH_KEY_USERNAME", "deploy")
	t.Setenv("KSCORE_CRED_SSH_KEY_TYPE", "key")

	p := NewEnvCredentialProvider("")
	cred, err := p.GetCredential(context.Background(), "ssh-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cred.Type != "key" {
		t.Errorf("Type = %q, want key", cred.Type)
	}
}

func TestEnvCredentialProvider_CustomPrefix(t *testing.T) {
	t.Setenv("MYAPP_DEV_CREDS_USERNAME", "testuser")
	t.Setenv("MYAPP_DEV_CREDS_PASSWORD", "testpass")

	p := NewEnvCredentialProvider("MYAPP")
	cred, err := p.GetCredential(context.Background(), "dev-creds")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cred.Username != "testuser" {
		t.Errorf("Username = %q, want testuser", cred.Username)
	}
}

func TestEnvCredentialProvider_NotFound(t *testing.T) {
	p := NewEnvCredentialProvider("")
	_, err := p.GetCredential(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing credentials")
	}
}

func TestEnvCredentialProvider_DotRef(t *testing.T) {
	t.Setenv("KSCORE_CRED_NETWORK_SWITCHES_USERNAME", "admin")
	t.Setenv("KSCORE_CRED_NETWORK_SWITCHES_PASSWORD", "pass")

	p := NewEnvCredentialProvider("")
	cred, err := p.GetCredential(context.Background(), "network.switches")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cred.Username != "admin" {
		t.Errorf("Username = %q, want admin", cred.Username)
	}
}

func TestEnvCredentialProvider_DefaultPrefix(t *testing.T) {
	p := NewEnvCredentialProvider("")
	if p.prefix != "KSCORE_CRED" {
		t.Errorf("prefix = %q, want KSCORE_CRED", p.prefix)
	}
}

func TestEnvKey(t *testing.T) {
	p := NewEnvCredentialProvider("KSCORE_CRED")

	tests := []struct {
		ref  string
		want string
	}{
		{"router-creds", "KSCORE_CRED_ROUTER_CREDS"},
		{"api-key", "KSCORE_CRED_API_KEY"},
		{"network.switches", "KSCORE_CRED_NETWORK_SWITCHES"},
		{"ALREADY_UPPER", "KSCORE_CRED_ALREADY_UPPER"},
	}

	for _, tt := range tests {
		got := p.envKey(tt.ref)
		if got != tt.want {
			t.Errorf("envKey(%q) = %q, want %q", tt.ref, got, tt.want)
		}
	}
}
