// Package statemgmt provides state management modules.
package statemgmt

import (
	"context"
	"runtime"
	"testing"
)

// =============================================================================
// Pip Module Tests
// =============================================================================

func TestNewPipModule(t *testing.T) {
	m := NewPipModule()
	if m == nil {
		t.Fatal("NewPipModule returned nil")
	}
	if m.Name() != "pip" {
		t.Errorf("expected name 'pip', got '%s'", m.Name())
	}
	states := m.ValidStates()
	if len(states) != 3 {
		t.Errorf("expected 3 states (installed, removed, latest), got %d", len(states))
	}
}

func TestPipModule_Check_MissingName(t *testing.T) {
	m := NewPipModule()
	decl := &StateDeclaration{
		ID:     "",
		Module: "pip",
		State:  "installed",
		Parameters: map[string]interface{}{
			"name": "",
		},
	}

	_, err := m.Check(context.Background(), decl)
	if err == nil {
		t.Error("expected error for missing name parameter")
	}
}

func TestPipModule_GetPipCommand(t *testing.T) {
	m := NewPipModule()

	// Default pip command
	decl := &StateDeclaration{
		ID:         "test",
		Module:     "pip",
		State:      "installed",
		Parameters: map[string]interface{}{},
	}

	cmd := m.getPipCommand(decl)
	if cmd != "pip" && cmd != "pip3" {
		t.Errorf("expected 'pip' or 'pip3', got '%s'", cmd)
	}

	// Custom pip3 command
	decl.Parameters["pip3"] = true
	cmd = m.getPipCommand(decl)
	if cmd != "pip3" {
		t.Errorf("expected 'pip3', got '%s'", cmd)
	}
}

func TestPipModule_Test(t *testing.T) {
	m := NewPipModule()

	// Missing name
	decl := &StateDeclaration{
		ID:     "",
		Module: "pip",
		State:  "installed",
		Parameters: map[string]interface{}{
			"name": "",
		},
	}

	ok, err := m.Test(context.Background(), decl)
	if err == nil && ok {
		t.Error("expected error or false for missing name")
	}
}

// =============================================================================
// NPM Module Tests
// =============================================================================

func TestNewNpmModule(t *testing.T) {
	m := NewNpmModule()
	if m == nil {
		t.Fatal("NewNpmModule returned nil")
	}
	if m.Name() != "npm" {
		t.Errorf("expected name 'npm', got '%s'", m.Name())
	}
	states := m.ValidStates()
	if len(states) != 3 {
		t.Errorf("expected 3 states (installed, removed, latest), got %d", len(states))
	}
}

func TestNpmModule_Check_MissingName(t *testing.T) {
	m := NewNpmModule()
	decl := &StateDeclaration{
		ID:     "",
		Module: "npm",
		State:  "installed",
		Parameters: map[string]interface{}{
			"name": "",
		},
	}

	_, err := m.Check(context.Background(), decl)
	if err == nil {
		t.Error("expected error for missing name parameter")
	}
}

func TestNpmModule_Test(t *testing.T) {
	m := NewNpmModule()

	// Missing name
	decl := &StateDeclaration{
		ID:     "",
		Module: "npm",
		State:  "installed",
		Parameters: map[string]interface{}{
			"name": "",
		},
	}

	ok, err := m.Test(context.Background(), decl)
	if err == nil && ok {
		t.Error("expected error or false for missing name")
	}
}

// =============================================================================
// Gem Module Tests
// =============================================================================

func TestNewGemModule(t *testing.T) {
	m := NewGemModule()
	if m == nil {
		t.Fatal("NewGemModule returned nil")
	}
	if m.Name() != "gem" {
		t.Errorf("expected name 'gem', got '%s'", m.Name())
	}
	states := m.ValidStates()
	if len(states) != 3 {
		t.Errorf("expected 3 states (installed, removed, latest), got %d", len(states))
	}
}

func TestGemModule_Check_MissingName(t *testing.T) {
	m := NewGemModule()
	decl := &StateDeclaration{
		ID:     "",
		Module: "gem",
		State:  "installed",
		Parameters: map[string]interface{}{
			"name": "",
		},
	}

	_, err := m.Check(context.Background(), decl)
	if err == nil {
		t.Error("expected error for missing name parameter")
	}
}

func TestGemModule_Test(t *testing.T) {
	m := NewGemModule()

	// Missing name
	decl := &StateDeclaration{
		ID:     "",
		Module: "gem",
		State:  "installed",
		Parameters: map[string]interface{}{
			"name": "",
		},
	}

	ok, err := m.Test(context.Background(), decl)
	if err == nil && ok {
		t.Error("expected error or false for missing name")
	}
}

// =============================================================================
// UFW Module Tests
// =============================================================================

func TestNewUfwModule(t *testing.T) {
	m := NewUfwModule()
	if m == nil {
		t.Fatal("NewUfwModule returned nil")
	}
	if m.Name() != "ufw" {
		t.Errorf("expected name 'ufw', got '%s'", m.Name())
	}
	states := m.ValidStates()
	if len(states) != 6 {
		t.Errorf("expected 6 states (enabled, disabled, allow, deny, reject, absent), got %d", len(states))
	}
}

func TestUfwModule_Check_NotLinux(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("skipping on Linux")
	}

	m := NewUfwModule()
	decl := &StateDeclaration{
		ID:         "test",
		Module:     "ufw",
		State:      "enabled",
		Parameters: map[string]interface{}{},
	}

	_, err := m.Check(context.Background(), decl)
	if err == nil {
		t.Error("expected error on non-Linux platform")
	}
}

func TestUfwModule_BuildRuleSpec(t *testing.T) {
	m := NewUfwModule()

	tests := []struct {
		name     string
		params   map[string]interface{}
		expected string
	}{
		{
			name: "port only",
			params: map[string]interface{}{
				"port": "22",
			},
			expected: "22",
		},
		{
			name: "port with proto",
			params: map[string]interface{}{
				"port":  "80",
				"proto": "tcp",
			},
			expected: "80 /tcp",
		},
		{
			name: "port with from",
			params: map[string]interface{}{
				"port": "443",
				"from": "192.168.1.0/24",
			},
			expected: "443 from 192.168.1.0/24",
		},
		{
			name: "port with to",
			params: map[string]interface{}{
				"port": "3306",
				"to":   "127.0.0.1",
			},
			expected: "3306 to 127.0.0.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decl := &StateDeclaration{
				ID:         "test",
				Module:     "ufw",
				State:      "allow",
				Parameters: tt.params,
			}

			rule := m.buildRuleSpec(decl)
			if rule != tt.expected {
				t.Errorf("expected rule '%s', got '%s'", tt.expected, rule)
			}
		})
	}
}

func TestUfwModule_Apply_NotLinux(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("skipping on Linux")
	}

	m := NewUfwModule()
	decl := &StateDeclaration{
		ID:         "test",
		Module:     "ufw",
		State:      "enabled",
		Parameters: map[string]interface{}{},
	}

	result, err := m.Apply(context.Background(), decl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error == nil {
		t.Error("expected error in result on non-Linux platform")
	}
}

// =============================================================================
// Alternatives Module Tests
// =============================================================================

func TestNewAlternativesModule(t *testing.T) {
	m := NewAlternativesModule()
	if m == nil {
		t.Fatal("NewAlternativesModule returned nil")
	}
	if m.Name() != "alternatives" {
		t.Errorf("expected name 'alternatives', got '%s'", m.Name())
	}
	states := m.ValidStates()
	if len(states) != 2 {
		t.Errorf("expected 2 states (set, auto), got %d", len(states))
	}
}

func TestAlternativesModule_Check_NotLinux(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("skipping on Linux")
	}

	m := NewAlternativesModule()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "alternatives",
		State:  "set",
		Parameters: map[string]interface{}{
			"name": "java",
			"path": "/usr/lib/jvm/java-11/bin/java",
		},
	}

	_, err := m.Check(context.Background(), decl)
	if err == nil {
		t.Error("expected error on non-Linux platform")
	}
}

func TestAlternativesModule_Check_MissingName(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("skipping on non-Linux platform")
	}

	m := NewAlternativesModule()
	decl := &StateDeclaration{
		ID:         "test",
		Module:     "alternatives",
		State:      "set",
		Parameters: map[string]interface{}{},
	}

	_, err := m.Check(context.Background(), decl)
	if err == nil {
		t.Error("expected error for missing name parameter")
	}
}

func TestAlternativesModule_Check_MissingPath(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("skipping on non-Linux platform")
	}

	m := NewAlternativesModule()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "alternatives",
		State:  "set",
		Parameters: map[string]interface{}{
			"name": "java",
		},
	}

	_, err := m.Check(context.Background(), decl)
	if err == nil {
		t.Error("expected error for missing path parameter with set state")
	}
}

func TestAlternativesModule_Apply_NotLinux(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("skipping on Linux")
	}

	m := NewAlternativesModule()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "alternatives",
		State:  "set",
		Parameters: map[string]interface{}{
			"name": "java",
			"path": "/usr/lib/jvm/java-11/bin/java",
		},
	}

	result, err := m.Apply(context.Background(), decl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error == nil {
		t.Error("expected error in result on non-Linux platform")
	}
}

func TestAlternativesModule_Test(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("skipping on non-Linux platform")
	}

	m := NewAlternativesModule()

	// Missing name
	decl := &StateDeclaration{
		ID:         "test",
		Module:     "alternatives",
		State:      "set",
		Parameters: map[string]interface{}{},
	}

	ok, err := m.Test(context.Background(), decl)
	if err == nil && ok {
		t.Error("expected error or false for missing name")
	}
}
