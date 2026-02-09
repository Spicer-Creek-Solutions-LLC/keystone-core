package telnet

import (
	"bytes"
	"fmt"
	"log/slog"
	"testing"
	"time"
)

func TestNewSecurityEnforcer_NilConfig(t *testing.T) {
	enforcer, err := NewSecurityEnforcer(nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if enforcer == nil {
		t.Fatal("expected non-nil enforcer")
	}
	// Default should have deprecation warning enabled
	if !enforcer.config.DeprecationWarning {
		t.Error("expected DeprecationWarning=true for nil config")
	}
}

func TestNewSecurityEnforcer_InvalidCIDR(t *testing.T) {
	config := &SecurityConfig{
		AllowedNetworks: []string{"not-a-cidr"},
	}
	_, err := NewSecurityEnforcer(config, nil)
	if err == nil {
		t.Error("expected error for invalid CIDR")
	}
}

func TestNewSecurityEnforcer_ValidCIDR(t *testing.T) {
	config := &SecurityConfig{
		AllowedNetworks: []string{"10.0.0.0/8", "192.168.1.0/24"},
	}
	enforcer, err := NewSecurityEnforcer(config, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(enforcer.parsedNetworks) != 2 {
		t.Errorf("expected 2 parsed networks, got %d", len(enforcer.parsedNetworks))
	}
}

func TestCheckConnection_NoAllowlist(t *testing.T) {
	enforcer, _ := NewSecurityEnforcer(&SecurityConfig{}, nil)
	// No allowlist means all addresses allowed
	if err := enforcer.CheckConnection("1.2.3.4:23"); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestCheckConnection_AllowedIP(t *testing.T) {
	config := &SecurityConfig{
		AllowedNetworks: []string{"10.0.0.0/8"},
	}
	enforcer, _ := NewSecurityEnforcer(config, nil)
	if err := enforcer.CheckConnection("10.1.2.3:23"); err != nil {
		t.Errorf("expected allowed, got %v", err)
	}
}

func TestCheckConnection_DeniedIP(t *testing.T) {
	config := &SecurityConfig{
		AllowedNetworks: []string{"10.0.0.0/8"},
	}
	enforcer, _ := NewSecurityEnforcer(config, nil)
	err := enforcer.CheckConnection("192.168.1.1:23")
	if err == nil {
		t.Error("expected error for denied IP")
	}
}

func TestCheckConnection_IPWithoutPort(t *testing.T) {
	config := &SecurityConfig{
		AllowedNetworks: []string{"10.0.0.0/8"},
	}
	enforcer, _ := NewSecurityEnforcer(config, nil)
	// No port in address
	if err := enforcer.CheckConnection("10.1.2.3"); err != nil {
		t.Errorf("expected allowed, got %v", err)
	}
}

func TestCheckConnection_MultipleNetworks(t *testing.T) {
	config := &SecurityConfig{
		AllowedNetworks: []string{"10.0.0.0/8", "172.16.0.0/12"},
	}
	enforcer, _ := NewSecurityEnforcer(config, nil)
	tests := []struct {
		addr    string
		allowed bool
	}{
		{"10.1.2.3:23", true},
		{"172.16.5.10:23", true},
		{"192.168.1.1:23", false},
	}
	for _, tt := range tests {
		err := enforcer.CheckConnection(tt.addr)
		if tt.allowed && err != nil {
			t.Errorf("expected %s to be allowed, got %v", tt.addr, err)
		}
		if !tt.allowed && err == nil {
			t.Errorf("expected %s to be denied", tt.addr)
		}
	}
}

func TestIsSessionExpired_NoLimit(t *testing.T) {
	enforcer, _ := NewSecurityEnforcer(&SecurityConfig{}, nil)
	if enforcer.IsSessionExpired(time.Now().Add(-24 * time.Hour)) {
		t.Error("expected no expiry with zero duration")
	}
}

func TestIsSessionExpired_NotExpired(t *testing.T) {
	config := &SecurityConfig{
		MaxSessionDuration: 1 * time.Hour,
	}
	enforcer, _ := NewSecurityEnforcer(config, nil)
	if enforcer.IsSessionExpired(time.Now()) {
		t.Error("expected session not expired")
	}
}

func TestIsSessionExpired_Expired(t *testing.T) {
	config := &SecurityConfig{
		MaxSessionDuration: 1 * time.Second,
	}
	enforcer, _ := NewSecurityEnforcer(config, nil)
	// Session started 2 seconds ago
	if !enforcer.IsSessionExpired(time.Now().Add(-2 * time.Second)) {
		t.Error("expected session expired")
	}
}

func TestEmitDeprecationWarning_Enabled(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	config := &SecurityConfig{DeprecationWarning: true}
	enforcer, _ := NewSecurityEnforcer(config, logger)

	enforcer.EmitDeprecationWarning("device-1", "10.0.0.1:23")

	if buf.Len() == 0 {
		t.Error("expected deprecation warning to be logged")
	}
}

func TestEmitDeprecationWarning_Disabled(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	config := &SecurityConfig{DeprecationWarning: false}
	enforcer, _ := NewSecurityEnforcer(config, logger)

	enforcer.EmitDeprecationWarning("device-1", "10.0.0.1:23")

	if buf.Len() != 0 {
		t.Error("expected no log output when disabled")
	}
}

func TestAuditCommand_Enabled(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	config := &SecurityConfig{RequireAudit: true}
	enforcer, _ := NewSecurityEnforcer(config, logger)

	enforcer.AuditCommand("device-1", "show version", 100*time.Millisecond, nil)

	if buf.Len() == 0 {
		t.Error("expected audit log output")
	}
}

func TestAuditCommand_Disabled(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	config := &SecurityConfig{RequireAudit: false}
	enforcer, _ := NewSecurityEnforcer(config, logger)

	enforcer.AuditCommand("device-1", "show version", 100*time.Millisecond, nil)

	if buf.Len() != 0 {
		t.Error("expected no audit log when disabled")
	}
}

func TestAuditCommand_WithError(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	config := &SecurityConfig{RequireAudit: true}
	enforcer, _ := NewSecurityEnforcer(config, logger)

	enforcer.AuditCommand("device-1", "bad-command", 50*time.Millisecond, fmt.Errorf("command not found"))

	output := buf.String()
	if !bytes.Contains([]byte(output), []byte("command failed")) {
		t.Error("expected error audit log")
	}
}
