package statemgmt

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// ============================================================================
// AuthorizedKeysModule Tests
// ============================================================================

func TestNewAuthorizedKeysModule(t *testing.T) {
	m := NewAuthorizedKeysModule()

	if m.Name() != "authorized_keys" {
		t.Errorf("expected name 'authorized_keys', got '%s'", m.Name())
	}

	states := m.ValidStates()
	expected := []string{"present", "absent"}
	if len(states) != len(expected) {
		t.Errorf("expected %d states, got %d", len(expected), len(states))
	}
	for i, s := range expected {
		if states[i] != s {
			t.Errorf("expected state[%d] = '%s', got '%s'", i, s, states[i])
		}
	}
}

func TestAuthorizedKeysModule_Check_MissingUser(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("authorized_keys not supported on Windows")
	}

	m := NewAuthorizedKeysModule()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "authorized_keys",
		State:  "present",
		Parameters: map[string]interface{}{
			"key": "AAAAB3NzaC1yc2EAAAADAQABAAABAQ...",
		},
	}

	_, err := m.Check(nil, decl)
	if err == nil || !strings.Contains(err.Error(), "user parameter is required") {
		t.Errorf("expected user required error, got: %v", err)
	}
}

func TestAuthorizedKeysModule_Check_MissingKey(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("authorized_keys not supported on Windows")
	}

	m := NewAuthorizedKeysModule()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "authorized_keys",
		State:  "present",
		Parameters: map[string]interface{}{
			"user": "testuser",
		},
	}

	_, err := m.Check(nil, decl)
	if err == nil || !strings.Contains(err.Error(), "key parameter is required") {
		t.Errorf("expected key required error, got: %v", err)
	}
}

func TestAuthorizedKeysModule_KeyParsing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("authorized_keys not supported on Windows")
	}

	// Create a temp authorized_keys file
	tmpDir := t.TempDir()
	sshDir := filepath.Join(tmpDir, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		t.Fatal(err)
	}

	authKeysPath := filepath.Join(sshDir, "authorized_keys")
	content := `# Comment line
ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQC1234 user@host
ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIG5678 another@host
no-port-forwarding ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQC9012 restricted@host
`
	if err := os.WriteFile(authKeysPath, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	m := NewAuthorizedKeysModule()

	// Test key exists
	present, comment, err := m.keyExists(authKeysPath, "ssh-rsa", "AAAAB3NzaC1yc2EAAAADAQABAAABgQC1234")
	if err != nil {
		t.Fatal(err)
	}
	if !present {
		t.Error("expected key to be present")
	}
	if comment != "user@host" {
		t.Errorf("expected comment 'user@host', got '%s'", comment)
	}

	// Test key with options
	present, _, err = m.keyExists(authKeysPath, "ssh-rsa", "AAAAB3NzaC1yc2EAAAADAQABAAABgQC9012")
	if err != nil {
		t.Fatal(err)
	}
	if !present {
		t.Error("expected key with options to be present")
	}

	// Test key not present
	present, _, err = m.keyExists(authKeysPath, "ssh-rsa", "NONEXISTENT")
	if err != nil {
		t.Fatal(err)
	}
	if present {
		t.Error("expected key to not be present")
	}
}

// ============================================================================
// KnownHostsModule Tests
// ============================================================================

func TestNewKnownHostsModule(t *testing.T) {
	m := NewKnownHostsModule()

	if m.Name() != "known_hosts" {
		t.Errorf("expected name 'known_hosts', got '%s'", m.Name())
	}

	states := m.ValidStates()
	expected := []string{"present", "absent"}
	if len(states) != len(expected) {
		t.Errorf("expected %d states, got %d", len(expected), len(states))
	}
}

func TestKnownHostsModule_Check_MissingHost(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("known_hosts not supported on Windows")
	}

	m := NewKnownHostsModule()
	decl := &StateDeclaration{
		ID:         "test",
		Module:     "known_hosts",
		State:      "present",
		Parameters: map[string]interface{}{},
	}

	_, err := m.Check(nil, decl)
	if err == nil || !strings.Contains(err.Error(), "host parameter is required") {
		t.Errorf("expected host required error, got: %v", err)
	}
}

func TestKnownHostsModule_HostParsing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("known_hosts not supported on Windows")
	}

	// Create a temp known_hosts file
	tmpDir := t.TempDir()
	knownHostsPath := filepath.Join(tmpDir, "known_hosts")
	content := `# Comment line
github.com ssh-rsa AAAAB3NzaC1yc2EAAAABIwAAAQEAq2A7
example.com,192.168.1.1 ssh-ed25519 AAAAC3NzaC1lZDI1NTE5
[gitlab.com]:2222 ecdsa-sha2-nistp256 AAAAE2VjZHN...
`
	if err := os.WriteFile(knownHostsPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	m := NewKnownHostsModule()

	// Test host exists
	present, key, keyType, err := m.hostExists(knownHostsPath, "github.com")
	if err != nil {
		t.Fatal(err)
	}
	if !present {
		t.Error("expected github.com to be present")
	}
	if keyType != "ssh-rsa" {
		t.Errorf("expected key type 'ssh-rsa', got '%s'", keyType)
	}
	if key != "AAAAB3NzaC1yc2EAAAABIwAAAQEAq2A7" {
		t.Errorf("unexpected key: %s", key)
	}

	// Test host with alias
	present, _, _, err = m.hostExists(knownHostsPath, "192.168.1.1")
	if err != nil {
		t.Fatal(err)
	}
	if !present {
		t.Error("expected 192.168.1.1 to be present (alias)")
	}

	// Test host not present
	present, _, _, err = m.hostExists(knownHostsPath, "unknown.com")
	if err != nil {
		t.Fatal(err)
	}
	if present {
		t.Error("expected unknown.com to not be present")
	}
}

// ============================================================================
// SSHDConfigModule Tests
// ============================================================================

func TestNewSSHDConfigModule(t *testing.T) {
	m := NewSSHDConfigModule()

	if m.Name() != "sshd_config" {
		t.Errorf("expected name 'sshd_config', got '%s'", m.Name())
	}

	states := m.ValidStates()
	expected := []string{"present", "absent"}
	if len(states) != len(expected) {
		t.Errorf("expected %d states, got %d", len(expected), len(states))
	}
}

func TestSSHDConfigModule_Check_MissingName(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sshd_config not supported on Windows")
	}

	m := NewSSHDConfigModule()
	decl := &StateDeclaration{
		ID:         "test",
		Module:     "sshd_config",
		State:      "present",
		Parameters: map[string]interface{}{},
	}

	_, err := m.Check(nil, decl)
	if err == nil || !strings.Contains(err.Error(), "name parameter is required") {
		t.Errorf("expected name required error, got: %v", err)
	}
}

func TestSSHDConfigModule_ConfigParsing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sshd_config not supported on Windows")
	}

	// Create a temp sshd_config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "sshd_config")
	content := `# SSH daemon configuration
Port 22
PermitRootLogin no
PasswordAuthentication yes
PubkeyAuthentication yes
# Commented setting
#UseDNS yes
`
	if err := os.WriteFile(configPath, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	m := NewSSHDConfigModule()

	// Test getting existing value
	value, exists, err := m.getConfigValue(configPath, "Port")
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Error("expected Port to exist")
	}
	if value != "22" {
		t.Errorf("expected Port = 22, got %s", value)
	}

	// Test case-insensitive matching
	value, exists, err = m.getConfigValue(configPath, "permitrootlogin")
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Error("expected PermitRootLogin to exist (case-insensitive)")
	}
	if value != "no" {
		t.Errorf("expected PermitRootLogin = no, got %s", value)
	}

	// Test commented value is not found
	_, exists, err = m.getConfigValue(configPath, "UseDNS")
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Error("expected commented UseDNS to not exist")
	}
}

func TestSSHDConfigModule_SetValue(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sshd_config not supported on Windows")
	}

	// Create a temp sshd_config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "sshd_config")
	content := `Port 22
PermitRootLogin no
`
	if err := os.WriteFile(configPath, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	m := NewSSHDConfigModule()

	// Test updating existing value
	if err := m.setConfigValue(configPath, "Port", "2222", false); err != nil {
		t.Fatal(err)
	}

	value, _, err := m.getConfigValue(configPath, "Port")
	if err != nil {
		t.Fatal(err)
	}
	if value != "2222" {
		t.Errorf("expected Port = 2222, got %s", value)
	}

	// Test adding new value
	if err := m.setConfigValue(configPath, "MaxAuthTries", "3", false); err != nil {
		t.Fatal(err)
	}

	value, exists, err := m.getConfigValue(configPath, "MaxAuthTries")
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Error("expected MaxAuthTries to exist")
	}
	if value != "3" {
		t.Errorf("expected MaxAuthTries = 3, got %s", value)
	}
}

// ============================================================================
// SELinuxModule Tests
// ============================================================================

func TestNewSELinuxModule(t *testing.T) {
	m := NewSELinuxModule()

	if m.Name() != "selinux" {
		t.Errorf("expected name 'selinux', got '%s'", m.Name())
	}

	states := m.ValidStates()
	expected := []string{"enforcing", "permissive", "disabled"}
	if len(states) != len(expected) {
		t.Errorf("expected %d states, got %d", len(expected), len(states))
	}
	for i, s := range expected {
		if states[i] != s {
			t.Errorf("expected state[%d] = '%s', got '%s'", i, s, states[i])
		}
	}
}

func TestSELinuxModule_Check_NonLinux(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("test for non-Linux platforms")
	}

	m := NewSELinuxModule()
	decl := &StateDeclaration{
		ID:         "test",
		Module:     "selinux",
		State:      "enforcing",
		Parameters: map[string]interface{}{},
	}

	_, err := m.Check(nil, decl)
	if err == nil || !strings.Contains(err.Error(), "only available on Linux") {
		t.Errorf("expected Linux-only error, got: %v", err)
	}
}

// ============================================================================
// SELinuxBooleanModule Tests
// ============================================================================

func TestNewSELinuxBooleanModule(t *testing.T) {
	m := NewSELinuxBooleanModule()

	if m.Name() != "selinux_boolean" {
		t.Errorf("expected name 'selinux_boolean', got '%s'", m.Name())
	}

	states := m.ValidStates()
	expected := []string{"on", "off"}
	if len(states) != len(expected) {
		t.Errorf("expected %d states, got %d", len(expected), len(states))
	}
}

func TestSELinuxBooleanModule_Check_MissingName(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("SELinux only available on Linux")
	}

	m := NewSELinuxBooleanModule()
	decl := &StateDeclaration{
		ID:         "test",
		Module:     "selinux_boolean",
		State:      "on",
		Parameters: map[string]interface{}{},
	}

	_, err := m.Check(nil, decl)
	if err == nil || !strings.Contains(err.Error(), "name parameter is required") {
		t.Errorf("expected name required error, got: %v", err)
	}
}

// ============================================================================
// AppArmorModule Tests
// ============================================================================

func TestNewAppArmorModule(t *testing.T) {
	m := NewAppArmorModule()

	if m.Name() != "apparmor" {
		t.Errorf("expected name 'apparmor', got '%s'", m.Name())
	}

	states := m.ValidStates()
	expected := []string{"enforce", "complain", "disabled"}
	if len(states) != len(expected) {
		t.Errorf("expected %d states, got %d", len(expected), len(states))
	}
	for i, s := range expected {
		if states[i] != s {
			t.Errorf("expected state[%d] = '%s', got '%s'", i, s, states[i])
		}
	}
}

func TestAppArmorModule_Check_NonLinux(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("test for non-Linux platforms")
	}

	m := NewAppArmorModule()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "apparmor",
		State:  "enforce",
		Parameters: map[string]interface{}{
			"profile": "test-profile",
		},
	}

	_, err := m.Check(nil, decl)
	if err == nil || !strings.Contains(err.Error(), "only available on Linux") {
		t.Errorf("expected Linux-only error, got: %v", err)
	}
}

func TestAppArmorModule_Check_MissingProfile(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("AppArmor only available on Linux")
	}

	// Check if AppArmor is enabled
	if _, err := os.Stat("/sys/kernel/security/apparmor"); os.IsNotExist(err) {
		t.Skip("AppArmor not enabled on this system")
	}

	m := NewAppArmorModule()
	decl := &StateDeclaration{
		ID:         "test",
		Module:     "apparmor",
		State:      "enforce",
		Parameters: map[string]interface{}{},
	}

	_, err := m.Check(nil, decl)
	if err == nil || !strings.Contains(err.Error(), "profile parameter is required") {
		t.Errorf("expected profile required error, got: %v", err)
	}
}

// ============================================================================
// AppArmorProfileModule Tests
// ============================================================================

func TestNewAppArmorProfileModule(t *testing.T) {
	m := NewAppArmorProfileModule()

	if m.Name() != "apparmor_profile" {
		t.Errorf("expected name 'apparmor_profile', got '%s'", m.Name())
	}

	states := m.ValidStates()
	expected := []string{"present", "absent"}
	if len(states) != len(expected) {
		t.Errorf("expected %d states, got %d", len(expected), len(states))
	}
}

func TestAppArmorProfileModule_Check_NonLinux(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("test for non-Linux platforms")
	}

	m := NewAppArmorProfileModule()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "apparmor_profile",
		State:  "present",
		Parameters: map[string]interface{}{
			"name": "test-profile",
		},
	}

	_, err := m.Check(nil, decl)
	if err == nil || !strings.Contains(err.Error(), "only available on Linux") {
		t.Errorf("expected Linux-only error, got: %v", err)
	}
}

func TestAppArmorProfileModule_Check_MissingName(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("AppArmor only available on Linux")
	}

	m := NewAppArmorProfileModule()
	decl := &StateDeclaration{
		ID:         "test",
		Module:     "apparmor_profile",
		State:      "present",
		Parameters: map[string]interface{}{},
	}

	_, err := m.Check(nil, decl)
	if err == nil || !strings.Contains(err.Error(), "name parameter is required") {
		t.Errorf("expected name required error, got: %v", err)
	}
}

// ============================================================================
// Helper Function Tests
// ============================================================================

func TestMinFunction(t *testing.T) {
	tests := []struct {
		a, b     int
		expected int
	}{
		{1, 2, 1},
		{5, 3, 3},
		{10, 10, 10},
		{0, 5, 0},
		{-1, 1, -1},
	}

	for _, tt := range tests {
		result := min(tt.a, tt.b)
		if result != tt.expected {
			t.Errorf("min(%d, %d) = %d, expected %d", tt.a, tt.b, result, tt.expected)
		}
	}
}
