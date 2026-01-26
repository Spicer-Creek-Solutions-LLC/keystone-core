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

// ============================================================================
// AuthorizedKeysModule addKey/removeKey Tests
// ============================================================================

func TestAuthorizedKeysModule_AddKey(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("authorized_keys not supported on Windows")
	}

	m := NewAuthorizedKeysModule()

	tests := []struct {
		name     string
		user     string
		keyType  string
		key      string
		comment  string
		options  string
		expected string
	}{
		{
			name:     "simple key without options",
			user:     "testuser",
			keyType:  "ssh-rsa",
			key:      "AAAAB3NzaC1yc2EAAAADAQABAAABAQ1234",
			comment:  "user@host",
			options:  "",
			expected: "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQ1234 user@host\n",
		},
		{
			name:     "key with options",
			user:     "testuser",
			keyType:  "ssh-ed25519",
			key:      "AAAAC3NzaC1lZDI1NTE5AAAAIG5678",
			comment:  "restricted@host",
			options:  "no-port-forwarding,no-agent-forwarding",
			expected: "no-port-forwarding,no-agent-forwarding ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIG5678 restricted@host\n",
		},
		{
			name:     "key without comment",
			user:     "testuser",
			keyType:  "ssh-rsa",
			key:      "AAAAB3NzaC1yc2EAAAADAQABAAABAQ9999",
			comment:  "",
			options:  "",
			expected: "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQ9999\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			sshDir := filepath.Join(tmpDir, ".ssh")
			authKeysPath := filepath.Join(sshDir, "authorized_keys")

			err := m.addKey(authKeysPath, tt.user, tt.keyType, tt.key, tt.comment, tt.options)
			if err != nil {
				t.Fatalf("addKey failed: %v", err)
			}

			// Verify file was created and contains expected content
			content, err := os.ReadFile(authKeysPath)
			if err != nil {
				t.Fatalf("failed to read authorized_keys: %v", err)
			}

			if string(content) != tt.expected {
				t.Errorf("expected:\n%s\ngot:\n%s", tt.expected, string(content))
			}

			// Verify directory permissions
			info, err := os.Stat(sshDir)
			if err != nil {
				t.Fatalf("failed to stat .ssh directory: %v", err)
			}
			if info.Mode().Perm() != 0700 {
				t.Errorf("expected .ssh directory permissions 0700, got %o", info.Mode().Perm())
			}

			// Verify file permissions
			fileInfo, err := os.Stat(authKeysPath)
			if err != nil {
				t.Fatalf("failed to stat authorized_keys: %v", err)
			}
			if fileInfo.Mode().Perm() != 0600 {
				t.Errorf("expected authorized_keys permissions 0600, got %o", fileInfo.Mode().Perm())
			}
		})
	}
}

func TestAuthorizedKeysModule_AddKey_AppendToExisting(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("authorized_keys not supported on Windows")
	}

	m := NewAuthorizedKeysModule()
	tmpDir := t.TempDir()
	sshDir := filepath.Join(tmpDir, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		t.Fatal(err)
	}
	authKeysPath := filepath.Join(sshDir, "authorized_keys")

	// Create initial file
	initial := "ssh-rsa EXISTINGKEY user@existing\n"
	if err := os.WriteFile(authKeysPath, []byte(initial), 0600); err != nil {
		t.Fatal(err)
	}

	// Add another key
	err := m.addKey(authKeysPath, "testuser", "ssh-ed25519", "NEWKEY", "user@new", "")
	if err != nil {
		t.Fatalf("addKey failed: %v", err)
	}

	content, err := os.ReadFile(authKeysPath)
	if err != nil {
		t.Fatal(err)
	}

	expected := "ssh-rsa EXISTINGKEY user@existing\nssh-ed25519 NEWKEY user@new\n"
	if string(content) != expected {
		t.Errorf("expected:\n%s\ngot:\n%s", expected, string(content))
	}
}

func TestAuthorizedKeysModule_RemoveKey(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("authorized_keys not supported on Windows")
	}

	m := NewAuthorizedKeysModule()

	tests := []struct {
		name     string
		initial  string
		keyType  string
		key      string
		expected string
	}{
		{
			name: "remove simple key",
			initial: `ssh-rsa AAAAB3NzaC1yc2EKEY1 user1@host
ssh-ed25519 AAAAC3NzaC1lZDI1KEY2 user2@host
ssh-rsa AAAAB3NzaC1yc2EKEY3 user3@host
`,
			keyType:  "ssh-ed25519",
			key:      "AAAAC3NzaC1lZDI1KEY2",
			expected: "ssh-rsa AAAAB3NzaC1yc2EKEY1 user1@host\nssh-rsa AAAAB3NzaC1yc2EKEY3 user3@host\n",
		},
		{
			name: "remove key with options",
			initial: `# Comment
no-port-forwarding ssh-rsa AAAAB3KEY1 user1@host
ssh-rsa AAAAB3KEY2 user2@host
`,
			keyType:  "ssh-rsa",
			key:      "AAAAB3KEY1",
			expected: "# Comment\nssh-rsa AAAAB3KEY2 user2@host\n",
		},
		{
			name: "remove key preserves comments and empty lines",
			initial: `# This is a comment

ssh-rsa AAAAB3KEY1 user@host
# Another comment
`,
			keyType:  "ssh-rsa",
			key:      "AAAAB3KEY1",
			expected: "# This is a comment\n\n# Another comment\n",
		},
		{
			name: "remove nonexistent key",
			initial: `ssh-rsa AAAAB3KEY1 user@host
`,
			keyType:  "ssh-rsa",
			key:      "NONEXISTENT",
			expected: "ssh-rsa AAAAB3KEY1 user@host\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			authKeysPath := filepath.Join(tmpDir, "authorized_keys")

			if err := os.WriteFile(authKeysPath, []byte(tt.initial), 0600); err != nil {
				t.Fatal(err)
			}

			err := m.removeKey(authKeysPath, tt.keyType, tt.key)
			if err != nil {
				t.Fatalf("removeKey failed: %v", err)
			}

			content, err := os.ReadFile(authKeysPath)
			if err != nil {
				t.Fatal(err)
			}

			if string(content) != tt.expected {
				t.Errorf("expected:\n%q\ngot:\n%q", tt.expected, string(content))
			}
		})
	}
}

func TestAuthorizedKeysModule_RemoveKey_NonExistentFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("authorized_keys not supported on Windows")
	}

	m := NewAuthorizedKeysModule()
	tmpDir := t.TempDir()
	nonExistent := filepath.Join(tmpDir, "nonexistent")

	// Should not return error for non-existent file
	err := m.removeKey(nonExistent, "ssh-rsa", "AAAAB3KEY")
	if err != nil {
		t.Errorf("expected no error for non-existent file, got: %v", err)
	}
}

// ============================================================================
// KnownHostsModule addHostKey/removeHostKey Tests
// ============================================================================

func TestKnownHostsModule_AddHostKey(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("known_hosts not supported on Windows")
	}

	m := NewKnownHostsModule()

	tests := []struct {
		name     string
		host     string
		keyType  string
		key      string
		hashHost bool
		expected string
	}{
		{
			name:     "simple host key",
			host:     "github.com",
			keyType:  "ssh-rsa",
			key:      "AAAAB3NzaC1yc2EAAAABIwAAAQEAq2A7",
			hashHost: false,
			expected: "github.com ssh-rsa AAAAB3NzaC1yc2EAAAABIwAAAQEAq2A7\n",
		},
		{
			name:     "host with port",
			host:     "[gitlab.com]:2222",
			keyType:  "ecdsa-sha2-nistp256",
			key:      "AAAAE2VjZHNhLXNoYTItbmlzdHAyNTY",
			hashHost: false,
			expected: "[gitlab.com]:2222 ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTY\n",
		},
		{
			name:     "ed25519 key",
			host:     "192.168.1.100",
			keyType:  "ssh-ed25519",
			key:      "AAAAC3NzaC1lZDI1NTE5AAAAIG5678",
			hashHost: false,
			expected: "192.168.1.100 ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIG5678\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			knownHostsPath := filepath.Join(tmpDir, "known_hosts")

			err := m.addHostKey(knownHostsPath, tt.host, tt.keyType, tt.key, tt.hashHost)
			if err != nil {
				t.Fatalf("addHostKey failed: %v", err)
			}

			content, err := os.ReadFile(knownHostsPath)
			if err != nil {
				t.Fatalf("failed to read known_hosts: %v", err)
			}

			if string(content) != tt.expected {
				t.Errorf("expected:\n%s\ngot:\n%s", tt.expected, string(content))
			}
		})
	}
}

func TestKnownHostsModule_AddHostKey_ReplacesExisting(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("known_hosts not supported on Windows")
	}

	m := NewKnownHostsModule()
	tmpDir := t.TempDir()
	knownHostsPath := filepath.Join(tmpDir, "known_hosts")

	// Create initial file with existing entry
	initial := "github.com ssh-rsa OLDKEY\nexample.com ssh-rsa OTHERKEY\n"
	if err := os.WriteFile(knownHostsPath, []byte(initial), 0644); err != nil {
		t.Fatal(err)
	}

	// Add key for same host (should replace)
	err := m.addHostKey(knownHostsPath, "github.com", "ssh-ed25519", "NEWKEY", false)
	if err != nil {
		t.Fatalf("addHostKey failed: %v", err)
	}

	content, err := os.ReadFile(knownHostsPath)
	if err != nil {
		t.Fatal(err)
	}

	// Should have removed old github.com entry and added new one
	if strings.Contains(string(content), "OLDKEY") {
		t.Error("expected old key to be removed")
	}
	if !strings.Contains(string(content), "NEWKEY") {
		t.Error("expected new key to be present")
	}
	if !strings.Contains(string(content), "OTHERKEY") {
		t.Error("expected unrelated key to remain")
	}
}

func TestKnownHostsModule_RemoveHostKey(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("known_hosts not supported on Windows")
	}

	m := NewKnownHostsModule()

	tests := []struct {
		name     string
		initial  string
		host     string
		expected string
	}{
		{
			name: "remove simple host",
			initial: `github.com ssh-rsa AAAAB3KEY1
gitlab.com ssh-rsa AAAAB3KEY2
bitbucket.org ssh-rsa AAAAB3KEY3
`,
			host:     "gitlab.com",
			expected: "github.com ssh-rsa AAAAB3KEY1\nbitbucket.org ssh-rsa AAAAB3KEY3\n",
		},
		{
			name: "remove host with port bracket notation",
			initial: `github.com ssh-rsa AAAAB3KEY1
[gitlab.com]:2222 ssh-rsa AAAAB3KEY2
`,
			host:     "[gitlab.com]:2222",
			expected: "github.com ssh-rsa AAAAB3KEY1\n",
		},
		{
			name: "remove host with multiple aliases",
			initial: `github.com,192.168.1.1 ssh-rsa AAAAB3KEY1
gitlab.com ssh-rsa AAAAB3KEY2
`,
			host:     "192.168.1.1",
			expected: "gitlab.com ssh-rsa AAAAB3KEY2\n",
		},
		{
			name: "preserve comments",
			initial: `# GitHub host key
github.com ssh-rsa AAAAB3KEY1
# GitLab host key
gitlab.com ssh-rsa AAAAB3KEY2
`,
			host:     "github.com",
			expected: "# GitHub host key\n# GitLab host key\ngitlab.com ssh-rsa AAAAB3KEY2\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			knownHostsPath := filepath.Join(tmpDir, "known_hosts")

			if err := os.WriteFile(knownHostsPath, []byte(tt.initial), 0644); err != nil {
				t.Fatal(err)
			}

			err := m.removeHostKey(knownHostsPath, tt.host)
			if err != nil {
				t.Fatalf("removeHostKey failed: %v", err)
			}

			content, err := os.ReadFile(knownHostsPath)
			if err != nil {
				t.Fatal(err)
			}

			if string(content) != tt.expected {
				t.Errorf("expected:\n%q\ngot:\n%q", tt.expected, string(content))
			}
		})
	}
}

func TestKnownHostsModule_RemoveHostKey_NonExistentFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("known_hosts not supported on Windows")
	}

	m := NewKnownHostsModule()
	tmpDir := t.TempDir()
	nonExistent := filepath.Join(tmpDir, "nonexistent")

	// Should not return error for non-existent file
	err := m.removeHostKey(nonExistent, "github.com")
	if err != nil {
		t.Errorf("expected no error for non-existent file, got: %v", err)
	}
}

// ============================================================================
// SSHDConfigModule removeConfigValue Tests
// ============================================================================

func TestSSHDConfigModule_RemoveConfigValue(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sshd_config not supported on Windows")
	}

	m := NewSSHDConfigModule()

	tests := []struct {
		name     string
		initial  string
		setting  string
		backup   bool
		expected string
	}{
		{
			name: "remove simple setting",
			initial: `Port 22
PermitRootLogin no
PasswordAuthentication yes
`,
			setting:  "PermitRootLogin",
			backup:   false,
			expected: "Port 22\n#PermitRootLogin no\nPasswordAuthentication yes\n",
		},
		{
			name: "case insensitive removal",
			initial: `Port 22
permitrootlogin no
`,
			setting:  "PermitRootLogin",
			backup:   false,
			expected: "Port 22\n#permitrootlogin no\n",
		},
		{
			name: "preserve comments",
			initial: `# Comment
Port 22
# Another comment
PasswordAuthentication yes
`,
			setting:  "Port",
			backup:   false,
			expected: "# Comment\n#Port 22\n# Another comment\nPasswordAuthentication yes\n",
		},
		{
			name: "remove nonexistent setting",
			initial: `Port 22
PasswordAuthentication yes
`,
			setting:  "NonExistent",
			backup:   false,
			expected: "Port 22\nPasswordAuthentication yes\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "sshd_config")

			if err := os.WriteFile(configPath, []byte(tt.initial), 0600); err != nil {
				t.Fatal(err)
			}

			err := m.removeConfigValue(configPath, tt.setting, tt.backup)
			if err != nil {
				t.Fatalf("removeConfigValue failed: %v", err)
			}

			content, err := os.ReadFile(configPath)
			if err != nil {
				t.Fatal(err)
			}

			if string(content) != tt.expected {
				t.Errorf("expected:\n%q\ngot:\n%q", tt.expected, string(content))
			}
		})
	}
}

func TestSSHDConfigModule_RemoveConfigValue_WithBackup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sshd_config not supported on Windows")
	}

	m := NewSSHDConfigModule()
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "sshd_config")

	initial := "Port 22\nPermitRootLogin no\n"
	if err := os.WriteFile(configPath, []byte(initial), 0600); err != nil {
		t.Fatal(err)
	}

	// Remove with backup
	err := m.removeConfigValue(configPath, "Port", true)
	if err != nil {
		t.Fatalf("removeConfigValue failed: %v", err)
	}

	// Verify backup file exists
	backupPath := configPath + ".bak"
	backupContent, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("failed to read backup file: %v", err)
	}

	if string(backupContent) != initial {
		t.Errorf("backup content mismatch, expected:\n%s\ngot:\n%s", initial, string(backupContent))
	}
}

func TestSSHDConfigModule_RemoveConfigValue_NonExistentFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sshd_config not supported on Windows")
	}

	m := NewSSHDConfigModule()
	tmpDir := t.TempDir()
	nonExistent := filepath.Join(tmpDir, "nonexistent")

	// Should not return error for non-existent file
	err := m.removeConfigValue(nonExistent, "Port", false)
	if err != nil {
		t.Errorf("expected no error for non-existent file, got: %v", err)
	}
}
