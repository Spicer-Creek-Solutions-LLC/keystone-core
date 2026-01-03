package statemgmt

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// ============================================================================
// TimezoneModule Tests
// ============================================================================

func TestNewTimezoneModule(t *testing.T) {
	m := NewTimezoneModule()

	if m.Name() != "timezone" {
		t.Errorf("expected name 'timezone', got '%s'", m.Name())
	}

	states := m.ValidStates()
	expected := []string{"present"}
	if len(states) != len(expected) {
		t.Errorf("expected %d states, got %d", len(expected), len(states))
	}
	for i, s := range expected {
		if states[i] != s {
			t.Errorf("expected state[%d] = '%s', got '%s'", i, s, states[i])
		}
	}
}

func TestTimezoneModule_Check_MissingName(t *testing.T) {
	m := NewTimezoneModule()
	decl := &StateDeclaration{
		ID:         "test",
		Module:     "timezone",
		State:      "present",
		Parameters: map[string]interface{}{},
	}

	_, err := m.Check(nil, decl)
	if err == nil || !strings.Contains(err.Error(), "name parameter is required") {
		t.Errorf("expected name required error, got: %v", err)
	}
}

func TestTimezoneModule_GetCurrentTimezone(t *testing.T) {
	m := NewTimezoneModule()

	tz, err := m.getCurrentTimezone()
	if err != nil {
		// Not an error if timezone tools aren't available
		t.Skipf("could not get current timezone: %v", err)
	}

	if tz == "" {
		t.Error("expected non-empty timezone")
	}

	// Timezone should look like Region/City or a Windows timezone name
	if runtime.GOOS != "windows" && !strings.Contains(tz, "/") && tz != "UTC" && tz != "Etc/UTC" {
		t.Logf("warning: unusual timezone format: %s", tz)
	}
}

// ============================================================================
// LocaleModule Tests
// ============================================================================

func TestNewLocaleModule(t *testing.T) {
	m := NewLocaleModule()

	if m.Name() != "locale" {
		t.Errorf("expected name 'locale', got '%s'", m.Name())
	}

	states := m.ValidStates()
	expected := []string{"present"}
	if len(states) != len(expected) {
		t.Errorf("expected %d states, got %d", len(expected), len(states))
	}
}

func TestLocaleModule_Check_MissingName(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("locale module not supported on Windows")
	}

	m := NewLocaleModule()
	decl := &StateDeclaration{
		ID:         "test",
		Module:     "locale",
		State:      "present",
		Parameters: map[string]interface{}{},
	}

	_, err := m.Check(nil, decl)
	if err == nil || !strings.Contains(err.Error(), "name parameter is required") {
		t.Errorf("expected name required error, got: %v", err)
	}
}

func TestLocaleModule_Check_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("test for Windows platform")
	}

	m := NewLocaleModule()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "locale",
		State:  "present",
		Parameters: map[string]interface{}{
			"name": "en_US.UTF-8",
		},
	}

	_, err := m.Check(nil, decl)
	if err == nil || !strings.Contains(err.Error(), "not supported on Windows") {
		t.Errorf("expected Windows not supported error, got: %v", err)
	}
}

// ============================================================================
// HostnameModule Tests
// ============================================================================

func TestNewHostnameModule(t *testing.T) {
	m := NewHostnameModule()

	if m.Name() != "hostname" {
		t.Errorf("expected name 'hostname', got '%s'", m.Name())
	}

	states := m.ValidStates()
	expected := []string{"present"}
	if len(states) != len(expected) {
		t.Errorf("expected %d states, got %d", len(expected), len(states))
	}
}

func TestHostnameModule_Check_MissingName(t *testing.T) {
	m := NewHostnameModule()
	decl := &StateDeclaration{
		ID:         "test",
		Module:     "hostname",
		State:      "present",
		Parameters: map[string]interface{}{},
	}

	_, err := m.Check(nil, decl)
	if err == nil || !strings.Contains(err.Error(), "name parameter is required") {
		t.Errorf("expected name required error, got: %v", err)
	}
}

func TestHostnameModule_Check_CurrentHostname(t *testing.T) {
	m := NewHostnameModule()

	currentHostname, err := os.Hostname()
	if err != nil {
		t.Fatalf("failed to get current hostname: %v", err)
	}

	decl := &StateDeclaration{
		ID:     "test",
		Module: "hostname",
		State:  "present",
		Parameters: map[string]interface{}{
			"name": currentHostname,
		},
	}

	result, err := m.Check(nil, decl)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	if !result.Matches {
		t.Error("expected current hostname to match")
	}
	if result.CurrentState != currentHostname {
		t.Errorf("expected CurrentState = '%s', got '%s'", currentHostname, result.CurrentState)
	}
}

// ============================================================================
// HostsModule Tests
// ============================================================================

func TestNewHostsModule(t *testing.T) {
	m := NewHostsModule()

	if m.Name() != "hosts" {
		t.Errorf("expected name 'hosts', got '%s'", m.Name())
	}

	states := m.ValidStates()
	expected := []string{"present", "absent"}
	if len(states) != len(expected) {
		t.Errorf("expected %d states, got %d", len(expected), len(states))
	}
}

func TestHostsModule_Check_MissingIP(t *testing.T) {
	m := NewHostsModule()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "hosts",
		State:  "present",
		Parameters: map[string]interface{}{
			"name": "example.com",
		},
	}

	_, err := m.Check(nil, decl)
	if err == nil || !strings.Contains(err.Error(), "ip parameter is required") {
		t.Errorf("expected ip required error, got: %v", err)
	}
}

func TestHostsModule_Check_MissingName(t *testing.T) {
	m := NewHostsModule()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "hosts",
		State:  "present",
		Parameters: map[string]interface{}{
			"ip": "192.168.1.1",
		},
	}

	_, err := m.Check(nil, decl)
	if err == nil || !strings.Contains(err.Error(), "names or name parameter is required") {
		t.Errorf("expected names required error, got: %v", err)
	}
}

func TestHostsModule_EntryParsing(t *testing.T) {
	// Create a temp hosts file
	tmpDir := t.TempDir()
	hostsPath := filepath.Join(tmpDir, "hosts")
	content := `# Comment line
127.0.0.1	localhost
192.168.1.1	server1 server1.local
10.0.0.1	gateway router
`
	if err := os.WriteFile(hostsPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	m := NewHostsModule()

	// Test localhost exists
	exists, names, err := m.entryExists(hostsPath, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Error("expected 127.0.0.1 to exist")
	}
	if len(names) != 1 || names[0] != "localhost" {
		t.Errorf("expected names = [localhost], got %v", names)
	}

	// Test multiple names
	exists, names, err = m.entryExists(hostsPath, "192.168.1.1")
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Error("expected 192.168.1.1 to exist")
	}
	if len(names) != 2 {
		t.Errorf("expected 2 names, got %d: %v", len(names), names)
	}

	// Test non-existent
	exists, _, err = m.entryExists(hostsPath, "1.2.3.4")
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Error("expected 1.2.3.4 to not exist")
	}
}

func TestHostsModule_AddRemoveEntry(t *testing.T) {
	// Create a temp hosts file
	tmpDir := t.TempDir()
	hostsPath := filepath.Join(tmpDir, "hosts")
	content := `127.0.0.1	localhost
`
	if err := os.WriteFile(hostsPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	m := NewHostsModule()

	// Add entry
	if err := m.addEntry(hostsPath, "10.0.0.1", []string{"myserver", "myserver.local"}); err != nil {
		t.Fatalf("failed to add entry: %v", err)
	}

	// Verify entry exists
	exists, names, err := m.entryExists(hostsPath, "10.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Error("expected 10.0.0.1 to exist after add")
	}
	if len(names) != 2 {
		t.Errorf("expected 2 names, got %d", len(names))
	}

	// Remove entry
	if err := m.removeEntry(hostsPath, "10.0.0.1"); err != nil {
		t.Fatalf("failed to remove entry: %v", err)
	}

	// Verify entry is gone
	exists, _, err = m.entryExists(hostsPath, "10.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Error("expected 10.0.0.1 to not exist after remove")
	}
}

// ============================================================================
// SysctlModule Tests
// ============================================================================

func TestNewSysctlModule(t *testing.T) {
	m := NewSysctlModule()

	if m.Name() != "sysctl" {
		t.Errorf("expected name 'sysctl', got '%s'", m.Name())
	}

	states := m.ValidStates()
	expected := []string{"present", "absent"}
	if len(states) != len(expected) {
		t.Errorf("expected %d states, got %d", len(expected), len(states))
	}
}

func TestSysctlModule_Check_NonLinux(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("test for non-Linux platforms")
	}

	m := NewSysctlModule()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "sysctl",
		State:  "present",
		Parameters: map[string]interface{}{
			"name":  "net.ipv4.ip_forward",
			"value": "1",
		},
	}

	_, err := m.Check(nil, decl)
	if err == nil || !strings.Contains(err.Error(), "only available on Linux") {
		t.Errorf("expected Linux-only error, got: %v", err)
	}
}

func TestSysctlModule_Check_MissingName(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("sysctl only available on Linux")
	}

	m := NewSysctlModule()
	decl := &StateDeclaration{
		ID:         "test",
		Module:     "sysctl",
		State:      "present",
		Parameters: map[string]interface{}{},
	}

	_, err := m.Check(nil, decl)
	if err == nil || !strings.Contains(err.Error(), "name parameter is required") {
		t.Errorf("expected name required error, got: %v", err)
	}
}

func TestSysctlModule_GetValue(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("sysctl only available on Linux")
	}

	m := NewSysctlModule()

	// Try a common sysctl that should exist
	value, exists, err := m.getValue("kernel.hostname")
	if err != nil {
		t.Fatalf("getValue failed: %v", err)
	}
	if !exists {
		t.Error("expected kernel.hostname to exist")
	}
	if value == "" {
		t.Error("expected non-empty value for kernel.hostname")
	}
}

// ============================================================================
// KernelModuleModule Tests
// ============================================================================

func TestNewKernelModuleModule(t *testing.T) {
	m := NewKernelModuleModule()

	if m.Name() != "kernel_module" {
		t.Errorf("expected name 'kernel_module', got '%s'", m.Name())
	}

	states := m.ValidStates()
	expected := []string{"loaded", "unloaded", "blacklisted"}
	if len(states) != len(expected) {
		t.Errorf("expected %d states, got %d", len(expected), len(states))
	}
	for i, s := range expected {
		if states[i] != s {
			t.Errorf("expected state[%d] = '%s', got '%s'", i, s, states[i])
		}
	}
}

func TestKernelModuleModule_Check_NonLinux(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("test for non-Linux platforms")
	}

	m := NewKernelModuleModule()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "kernel_module",
		State:  "loaded",
		Parameters: map[string]interface{}{
			"name": "loop",
		},
	}

	_, err := m.Check(nil, decl)
	if err == nil || !strings.Contains(err.Error(), "only available on Linux") {
		t.Errorf("expected Linux-only error, got: %v", err)
	}
}

func TestKernelModuleModule_Check_MissingName(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("kernel_module only available on Linux")
	}

	m := NewKernelModuleModule()
	decl := &StateDeclaration{
		ID:         "test",
		Module:     "kernel_module",
		State:      "loaded",
		Parameters: map[string]interface{}{},
	}

	_, err := m.Check(nil, decl)
	if err == nil || !strings.Contains(err.Error(), "name parameter is required") {
		t.Errorf("expected name required error, got: %v", err)
	}
}

func TestKernelModuleModule_IsLoaded(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("kernel_module only available on Linux")
	}

	m := NewKernelModuleModule()

	// Check if we can read /proc/modules
	_, err := os.ReadFile("/proc/modules")
	if err != nil {
		t.Skipf("cannot read /proc/modules: %v", err)
	}

	// Can't guarantee any specific module is loaded, but function should work
	loaded, err := m.isLoaded("nonexistent_module_xyz")
	if err != nil {
		t.Fatalf("isLoaded failed: %v", err)
	}
	if loaded {
		t.Error("expected nonexistent module to not be loaded")
	}
}

// ============================================================================
// Integration Test: Names Match
// ============================================================================

func TestHostsModule_NamesMatch(t *testing.T) {
	m := NewHostsModule()

	tests := []struct {
		expected []string
		current  []string
		match    bool
	}{
		{[]string{"a"}, []string{"a"}, true},
		{[]string{"a", "b"}, []string{"a", "b"}, true},
		{[]string{"a", "b"}, []string{"b", "a"}, true},
		{[]string{"a"}, []string{"b"}, false},
		{[]string{"a", "b"}, []string{"a"}, false},
		{[]string{"a"}, []string{"a", "b"}, false},
	}

	for _, tt := range tests {
		result := m.namesMatch(tt.expected, tt.current)
		if result != tt.match {
			t.Errorf("namesMatch(%v, %v) = %v, expected %v", tt.expected, tt.current, result, tt.match)
		}
	}
}
