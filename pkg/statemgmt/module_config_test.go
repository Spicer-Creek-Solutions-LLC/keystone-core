// Package statemgmt provides state management modules.
package statemgmt

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// =============================================================================
// Logrotate Module Tests
// =============================================================================

func TestNewLogrotateModule(t *testing.T) {
	m := NewLogrotateModule()
	if m == nil {
		t.Fatal("NewLogrotateModule returned nil")
	}
	if m.Name() != "logrotate" {
		t.Errorf("expected name 'logrotate', got '%s'", m.Name())
	}
	states := m.ValidStates()
	if len(states) != 2 {
		t.Errorf("expected 2 states, got %d", len(states))
	}
}

func TestLogrotateModule_Check_MissingName(t *testing.T) {
	m := NewLogrotateModule()
	decl := &StateDeclaration{
		ID:         "test",
		Module:     "logrotate",
		State:      "present",
		Parameters: map[string]interface{}{},
	}

	_, err := m.Check(context.Background(), decl)
	if err == nil {
		t.Error("expected error for missing name parameter")
	}
}

func TestLogrotateModule_Test(t *testing.T) {
	m := NewLogrotateModule()

	// Missing name
	decl := &StateDeclaration{
		ID:         "test",
		Module:     "logrotate",
		State:      "present",
		Parameters: map[string]interface{}{},
	}
	ok, err := m.Test(context.Background(), decl)
	if err == nil || ok {
		t.Error("expected error for missing name")
	}

	// Missing path for present state
	decl.Parameters["name"] = "myapp"
	ok, err = m.Test(context.Background(), decl)
	if err == nil || ok {
		t.Error("expected error for missing path on present state")
	}
}

func TestLogrotateModule_BuildConfig(t *testing.T) {
	m := NewLogrotateModule()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "logrotate",
		State:  "present",
		Parameters: map[string]interface{}{
			"name":      "myapp",
			"path":      "/var/log/myapp/*.log",
			"frequency": "daily",
			"rotate":    7,
			"compress":  true,
		},
	}

	config := m.buildLogrotateConfig(decl)
	if config == "" {
		t.Error("expected non-empty config")
	}
	if !strings.Contains(config, "daily") {
		t.Error("expected config to contain 'daily'")
	}
	if !strings.Contains(config, "rotate 7") {
		t.Error("expected config to contain 'rotate 7'")
	}
}

// =============================================================================
// Sudoers Module Tests
// =============================================================================

func TestNewSudoersModule(t *testing.T) {
	m := NewSudoersModule()
	if m == nil {
		t.Fatal("NewSudoersModule returned nil")
	}
	if m.Name() != "sudoers" {
		t.Errorf("expected name 'sudoers', got '%s'", m.Name())
	}
}

func TestSudoersModule_Check_MissingName(t *testing.T) {
	m := NewSudoersModule()
	decl := &StateDeclaration{
		ID:         "test",
		Module:     "sudoers",
		State:      "present",
		Parameters: map[string]interface{}{},
	}

	_, err := m.Check(context.Background(), decl)
	if err == nil {
		t.Error("expected error for missing name parameter")
	}
}

func TestSudoersModule_Test(t *testing.T) {
	m := NewSudoersModule()

	// Missing name
	decl := &StateDeclaration{
		ID:         "test",
		Module:     "sudoers",
		State:      "present",
		Parameters: map[string]interface{}{},
	}
	ok, err := m.Test(context.Background(), decl)
	if err == nil || ok {
		t.Error("expected error for missing name")
	}

	// Missing user/group/content
	decl.Parameters["name"] = "myuser"
	ok, err = m.Test(context.Background(), decl)
	if err == nil || ok {
		t.Error("expected error for missing user/group/content")
	}

	// Valid with user
	decl.Parameters["user"] = "admin"
	ok, err = m.Test(context.Background(), decl)
	if err != nil || !ok {
		t.Errorf("expected success, got error: %v", err)
	}
}

func TestSudoersModule_BuildContent(t *testing.T) {
	m := NewSudoersModule()

	// Test with user
	decl := &StateDeclaration{
		ID:     "test",
		Module: "sudoers",
		State:  "present",
		Parameters: map[string]interface{}{
			"name":     "admin",
			"user":     "admin",
			"nopasswd": true,
		},
	}

	content := m.buildSudoersContent(decl)
	if !strings.Contains(content, "admin") {
		t.Error("expected content to contain 'admin'")
	}
	if !strings.Contains(content, "NOPASSWD") {
		t.Error("expected content to contain 'NOPASSWD'")
	}

	// Test with group
	decl.Parameters["user"] = ""
	decl.Parameters["group"] = "wheel"
	content = m.buildSudoersContent(decl)
	if !strings.Contains(content, "%wheel") {
		t.Error("expected content to contain '%wheel'")
	}

	// Test with raw content
	decl.Parameters["content"] = "custom sudoers line"
	content = m.buildSudoersContent(decl)
	if content != "custom sudoers line\n" {
		t.Errorf("expected raw content, got '%s'", content)
	}
}

func TestSudoersModule_Apply_PathTraversal(t *testing.T) {
	m := NewSudoersModule()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "sudoers",
		State:  "present",
		Parameters: map[string]interface{}{
			"name": "../etc/passwd",
			"user": "admin",
		},
	}

	_, err := m.Apply(context.Background(), decl)
	if err == nil {
		t.Error("expected error for path traversal attempt")
	}
}

// =============================================================================
// Limits Module Tests
// =============================================================================

func TestNewLimitsModule(t *testing.T) {
	m := NewLimitsModule()
	if m == nil {
		t.Fatal("NewLimitsModule returned nil")
	}
	if m.Name() != "limits" {
		t.Errorf("expected name 'limits', got '%s'", m.Name())
	}
}

func TestLimitsModule_Check_MissingName(t *testing.T) {
	m := NewLimitsModule()
	decl := &StateDeclaration{
		ID:         "test",
		Module:     "limits",
		State:      "present",
		Parameters: map[string]interface{}{},
	}

	_, err := m.Check(context.Background(), decl)
	if err == nil {
		t.Error("expected error for missing name parameter")
	}
}

func TestLimitsModule_BuildContent(t *testing.T) {
	m := NewLimitsModule()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "limits",
		State:  "present",
		Parameters: map[string]interface{}{
			"name":       "nofile",
			"domain":     "*",
			"limit_type": "soft",
			"item":       "nofile",
			"value":      "65535",
		},
	}

	content := m.buildLimitsContent(decl)
	if !strings.Contains(content, "nofile") {
		t.Error("expected content to contain 'nofile'")
	}
	if !strings.Contains(content, "65535") {
		t.Error("expected content to contain '65535'")
	}
}

func TestLimitsModule_Test(t *testing.T) {
	m := NewLimitsModule()
	decl := &StateDeclaration{
		ID:         "limits",
		Module:     "limits",
		State:      "present",
		Parameters: map[string]interface{}{},
	}

	ok, err := m.Test(context.Background(), decl)
	if err == nil || ok {
		t.Fatal("expected error for missing name")
	}

	decl.Parameters["name"] = "test"
	ok, err = m.Test(context.Background(), decl)
	if err != nil || !ok {
		t.Fatalf("expected ok after setting name, err=%v", err)
	}
}

func TestLimitsModule_BuildContent_MultipleLimits(t *testing.T) {
	m := NewLimitsModule()
	decl := &StateDeclaration{
		ID:     "limits",
		Module: "limits",
		State:  "present",
		Parameters: map[string]interface{}{
			"name": "multi",
			"limits": []interface{}{
				map[string]interface{}{
					"domain":     "*",
					"limit_type": "soft",
					"item":       "nofile",
					"value":      "4096",
				},
				map[string]interface{}{
					"domain":     "root",
					"limit_type": "hard",
					"item":       "nproc",
					"value":      "8192",
				},
			},
		},
	}

	content := m.buildLimitsContent(decl)
	if !strings.Contains(content, "soft nofile 4096") || !strings.Contains(content, "root soft nproc 8192") {
		t.Fatalf("expected limits to be rendered, got:\n%s", content)
	}
}

// =============================================================================
// Modprobe Module Tests
// =============================================================================

func TestNewModprobeModule(t *testing.T) {
	m := NewModprobeModule()
	if m == nil {
		t.Fatal("NewModprobeModule returned nil")
	}
	if m.Name() != "modprobe" {
		t.Errorf("expected name 'modprobe', got '%s'", m.Name())
	}
	states := m.ValidStates()
	if len(states) != 3 {
		t.Errorf("expected 3 states, got %d", len(states))
	}
}

func TestModprobeModule_Check_MissingName(t *testing.T) {
	m := NewModprobeModule()
	decl := &StateDeclaration{
		ID:         "test",
		Module:     "modprobe",
		State:      "present",
		Parameters: map[string]interface{}{},
	}

	_, err := m.Check(context.Background(), decl)
	if err == nil {
		t.Error("expected error for missing name parameter")
	}
}

// =============================================================================
// Syslog Module Tests
// =============================================================================

func TestNewSyslogModule(t *testing.T) {
	m := NewSyslogModule()
	if m == nil {
		t.Fatal("NewSyslogModule returned nil")
	}
	if m.Name() != "syslog" {
		t.Errorf("expected name 'syslog', got '%s'", m.Name())
	}
}

func TestSyslogModule_Check_MissingName(t *testing.T) {
	m := NewSyslogModule()
	decl := &StateDeclaration{
		ID:         "test",
		Module:     "syslog",
		State:      "present",
		Parameters: map[string]interface{}{},
	}

	_, err := m.Check(context.Background(), decl)
	if err == nil {
		t.Error("expected error for missing name parameter")
	}
}

func TestSyslogModule_BuildContent(t *testing.T) {
	m := NewSyslogModule()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "syslog",
		State:  "present",
		Parameters: map[string]interface{}{
			"name":        "myapp",
			"facility":    "local0",
			"priority":    "info",
			"destination": "/var/log/myapp.log",
		},
	}

	content := m.buildSyslogContent(decl)
	if !strings.Contains(content, "local0.info") {
		t.Error("expected content to contain 'local0.info'")
	}
	if !strings.Contains(content, "/var/log/myapp.log") {
		t.Error("expected content to contain destination")
	}
}

func TestSyslogModule_Test(t *testing.T) {
	m := NewSyslogModule()
	decl := &StateDeclaration{
		ID:         "syslog",
		Module:     "syslog",
		State:      "present",
		Parameters: map[string]interface{}{},
	}

	ok, err := m.Test(context.Background(), decl)
	if err == nil || ok {
		t.Fatal("expected error for missing name")
	}

	decl.Parameters["name"] = "test"
	ok, err = m.Test(context.Background(), decl)
	if err != nil || !ok {
		t.Fatalf("expected ok after setting name, err=%v", err)
	}
}

func TestModprobeModule_IsModuleLoadedFalse(t *testing.T) {
	m := NewModprobeModule()
	loaded, err := m.isModuleLoaded("kscore-not-a-real-module")
	if err != nil {
		t.Fatalf("isModuleLoaded returned error: %v", err)
	}
	if loaded {
		t.Fatal("expected module to not be loaded")
	}
}

func TestModprobeModule_IsModuleBlacklistedFalse(t *testing.T) {
	m := NewModprobeModule()
	if m.isModuleBlacklisted("kscore-not-a-real-module") {
		t.Fatal("expected module to not be blacklisted")
	}
}

// =============================================================================
// Lineinfile Module Tests
// =============================================================================

func TestNewLineinfileModule(t *testing.T) {
	m := NewLineinfileModule()
	if m == nil {
		t.Fatal("NewLineinfileModule returned nil")
	}
	if m.Name() != "lineinfile" {
		t.Errorf("expected name 'lineinfile', got '%s'", m.Name())
	}
}

func TestLineinfileModule_Check_MissingPath(t *testing.T) {
	m := NewLineinfileModule()
	decl := &StateDeclaration{
		ID:         "test",
		Module:     "lineinfile",
		State:      "present",
		Parameters: map[string]interface{}{},
	}

	_, err := m.Check(context.Background(), decl)
	if err == nil {
		t.Error("expected error for missing path parameter")
	}
}

func TestLineinfileModule_Check_FileNotFound(t *testing.T) {
	m := NewLineinfileModule()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "lineinfile",
		State:  "present",
		Parameters: map[string]interface{}{
			"path": "/nonexistent/file.txt",
			"line": "test line",
		},
	}

	result, err := m.Check(context.Background(), decl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Present {
		t.Error("expected file to not be present")
	}
}

func TestLineinfileModule_Apply(t *testing.T) {
	m := NewLineinfileModule()

	// Create temp file
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(tmpFile, []byte("line1\nline2\nline3\n"), 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	// Add a line
	decl := &StateDeclaration{
		ID:     "test",
		Module: "lineinfile",
		State:  "present",
		Parameters: map[string]interface{}{
			"path": tmpFile,
			"line": "new line",
		},
	}

	result, err := m.Apply(context.Background(), decl)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if !result.Success {
		t.Error("expected success")
	}
	if !result.Changed {
		t.Error("expected change")
	}

	// Verify line was added
	content, _ := os.ReadFile(tmpFile)
	if !strings.Contains(string(content), "new line") {
		t.Error("expected file to contain 'new line'")
	}

	// Apply again - should be idempotent
	result, err = m.Apply(context.Background(), decl)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if !result.Success {
		t.Error("expected success")
	}
	if result.Changed {
		t.Error("expected no change on second apply")
	}
}

func TestLineinfileModule_Apply_Absent(t *testing.T) {
	m := NewLineinfileModule()

	// Create temp file
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(tmpFile, []byte("line1\nremove me\nline3\n"), 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	// Remove a line
	decl := &StateDeclaration{
		ID:     "test",
		Module: "lineinfile",
		State:  "absent",
		Parameters: map[string]interface{}{
			"path": tmpFile,
			"line": "remove me",
		},
	}

	result, err := m.Apply(context.Background(), decl)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if !result.Success {
		t.Error("expected success")
	}
	if !result.Changed {
		t.Error("expected change")
	}

	// Verify line was removed
	content, _ := os.ReadFile(tmpFile)
	if strings.Contains(string(content), "remove me") {
		t.Error("expected file to not contain 'remove me'")
	}
}

func TestLineinfileModule_Apply_Regexp(t *testing.T) {
	m := NewLineinfileModule()

	// Create temp file
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(tmpFile, []byte("setting=old_value\nother=stuff\n"), 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	// Replace using regexp
	decl := &StateDeclaration{
		ID:     "test",
		Module: "lineinfile",
		State:  "present",
		Parameters: map[string]interface{}{
			"path":   tmpFile,
			"regexp": "^setting=",
			"line":   "setting=new_value",
		},
	}

	result, err := m.Apply(context.Background(), decl)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if !result.Changed {
		t.Error("expected change")
	}

	// Verify line was replaced
	content, _ := os.ReadFile(tmpFile)
	if !strings.Contains(string(content), "setting=new_value") {
		t.Error("expected file to contain 'setting=new_value'")
	}
	if strings.Contains(string(content), "old_value") {
		t.Error("expected file to not contain 'old_value'")
	}
}

// =============================================================================
// INI File Module Tests
// =============================================================================

func TestNewIniFileModule(t *testing.T) {
	m := NewIniFileModule()
	if m == nil {
		t.Fatal("NewIniFileModule returned nil")
	}
	if m.Name() != "ini_file" {
		t.Errorf("expected name 'ini_file', got '%s'", m.Name())
	}
}

func TestIniFileModule_Check_MissingPath(t *testing.T) {
	m := NewIniFileModule()
	decl := &StateDeclaration{
		ID:         "test",
		Module:     "ini_file",
		State:      "present",
		Parameters: map[string]interface{}{},
	}

	_, err := m.Check(context.Background(), decl)
	if err == nil {
		t.Error("expected error for missing path parameter")
	}
}

func TestIniFileModule_ParseINI(t *testing.T) {
	m := NewIniFileModule()

	// Create temp INI file
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.ini")
	content := `[section1]
key1 = value1
key2 = value2

[section2]
foo = bar
`
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	ini, err := m.parseINI(tmpFile)
	if err != nil {
		t.Fatalf("parseINI failed: %v", err)
	}

	if len(ini) != 2 {
		t.Errorf("expected 2 sections, got %d", len(ini))
	}
	if ini["section1"]["key1"] != "value1" {
		t.Errorf("expected 'value1', got '%s'", ini["section1"]["key1"])
	}
	if ini["section2"]["foo"] != "bar" {
		t.Errorf("expected 'bar', got '%s'", ini["section2"]["foo"])
	}
}

func TestIniFileModule_Apply(t *testing.T) {
	m := NewIniFileModule()

	// Create temp INI file
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.ini")
	content := `[database]
host = localhost
port = 5432
`
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	// Modify existing option
	decl := &StateDeclaration{
		ID:     "test",
		Module: "ini_file",
		State:  "present",
		Parameters: map[string]interface{}{
			"path":    tmpFile,
			"section": "database",
			"option":  "port",
			"value":   "3306",
		},
	}

	result, err := m.Apply(context.Background(), decl)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if !result.Changed {
		t.Error("expected change")
	}

	// Verify change
	newContent, _ := os.ReadFile(tmpFile)
	if !strings.Contains(string(newContent), "port = 3306") {
		t.Error("expected file to contain 'port = 3306'")
	}

	// Add new option
	decl.Parameters["option"] = "user"
	decl.Parameters["value"] = "admin"

	result, err = m.Apply(context.Background(), decl)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if !result.Changed {
		t.Error("expected change")
	}

	// Verify new option added
	newContent, _ = os.ReadFile(tmpFile)
	if !strings.Contains(string(newContent), "user = admin") {
		t.Error("expected file to contain 'user = admin'")
	}
}

func TestIniFileModule_Apply_NewSection(t *testing.T) {
	m := NewIniFileModule()

	// Create temp INI file
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.ini")
	content := `[existing]
key = value
`
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	// Add option to new section
	decl := &StateDeclaration{
		ID:     "test",
		Module: "ini_file",
		State:  "present",
		Parameters: map[string]interface{}{
			"path":    tmpFile,
			"section": "newsection",
			"option":  "newkey",
			"value":   "newvalue",
		},
	}

	result, err := m.Apply(context.Background(), decl)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if !result.Changed {
		t.Error("expected change")
	}

	// Verify new section and option
	newContent, _ := os.ReadFile(tmpFile)
	if !strings.Contains(string(newContent), "[newsection]") {
		t.Error("expected file to contain '[newsection]'")
	}
	if !strings.Contains(string(newContent), "newkey = newvalue") {
		t.Error("expected file to contain 'newkey = newvalue'")
	}
}

// =============================================================================
// Archive Module Tests
// =============================================================================

func TestNewArchiveModule(t *testing.T) {
	m := NewArchiveModule()
	if m == nil {
		t.Fatal("NewArchiveModule returned nil")
	}
	if m.Name() != "archive" {
		t.Errorf("expected name 'archive', got '%s'", m.Name())
	}
	states := m.ValidStates()
	if len(states) != 3 {
		t.Errorf("expected 3 states, got %d", len(states))
	}
}

func TestArchiveModule_DetectFormat(t *testing.T) {
	m := NewArchiveModule()

	tests := []struct {
		filename string
		expected string
	}{
		{"file.tar.gz", "tar.gz"},
		{"file.tgz", "tar.gz"},
		{"file.tar.bz2", "tar.bz2"},
		{"file.tbz2", "tar.bz2"},
		{"file.tar.xz", "tar.xz"},
		{"file.txz", "tar.xz"},
		{"file.tar", "tar"},
		{"file.zip", "zip"},
		{"file.unknown", "tar.gz"}, // default
	}

	for _, tc := range tests {
		format := m.detectFormat(tc.filename)
		if format != tc.expected {
			t.Errorf("detectFormat(%s): expected '%s', got '%s'", tc.filename, tc.expected, format)
		}
	}
}

func TestArchiveModule_Check_MissingDest(t *testing.T) {
	m := NewArchiveModule()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "archive",
		State:  "extracted",
		Parameters: map[string]interface{}{
			"src": "/path/to/archive.tar.gz",
		},
	}

	_, err := m.Check(context.Background(), decl)
	if err == nil {
		t.Error("expected error for missing dest parameter")
	}
}

func TestArchiveModule_Test(t *testing.T) {
	m := NewArchiveModule()

	// extracted state - missing src and dest
	decl := &StateDeclaration{
		ID:         "test",
		Module:     "archive",
		State:      "extracted",
		Parameters: map[string]interface{}{},
	}
	ok, err := m.Test(context.Background(), decl)
	if err == nil || ok {
		t.Error("expected error for missing src/dest")
	}

	// present state - missing src and dest
	decl.State = "present"
	ok, err = m.Test(context.Background(), decl)
	if err == nil || ok {
		t.Error("expected error for missing src/dest")
	}

	// absent state - missing dest
	decl.State = "absent"
	ok, err = m.Test(context.Background(), decl)
	if err == nil || ok {
		t.Error("expected error for missing dest")
	}

	// Valid absent
	decl.Parameters["dest"] = "/path/to/dest"
	ok, err = m.Test(context.Background(), decl)
	if err != nil || !ok {
		t.Errorf("expected success, got error: %v", err)
	}
}
