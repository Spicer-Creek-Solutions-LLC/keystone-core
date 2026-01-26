package statemgmt

import (
	"context"
	"runtime"
	"testing"
)

func TestUserModule_NewUserModule(t *testing.T) {
	module := NewUserModule()
	if module == nil {
		t.Fatal("NewUserModule returned nil")
	}
	if module.Name() != "user" {
		t.Errorf("Expected name 'user', got '%s'", module.Name())
	}
	validStates := module.ValidStates()
	if len(validStates) != 2 {
		t.Errorf("Expected 2 valid states, got %d", len(validStates))
	}
}

func TestUserModule_CheckUserExists(t *testing.T) {
	module := NewUserModule()
	ctx := context.Background()

	// Check for current user (should exist on all systems)
	decl := &StateDeclaration{
		ID:         "root", // root exists on all Unix-like systems
		Module:     "user",
		State:      "present",
		Parameters: map[string]interface{}{},
	}

	result, err := module.Check(ctx, decl)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	if !result.Present {
		t.Error("Expected root user to be present")
	}
	if result.CurrentState != "present" {
		t.Errorf("Expected current state 'present', got '%s'", result.CurrentState)
	}
}

func TestUserModule_CheckUserNotExists(t *testing.T) {
	module := NewUserModule()
	ctx := context.Background()

	// Check for non-existent user
	decl := &StateDeclaration{
		ID:         "thisuserdefinitelydoesnotexist12345",
		Module:     "user",
		State:      "absent",
		Parameters: map[string]interface{}{},
	}

	result, err := module.Check(ctx, decl)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	if result.Present {
		t.Error("Expected non-existent user to not be present")
	}
	if result.CurrentState != "absent" {
		t.Errorf("Expected current state 'absent', got '%s'", result.CurrentState)
	}
	if !result.Matches {
		t.Error("Expected state to match (absent user should match absent state)")
	}
}

func TestUserModule_GetUserShell(t *testing.T) {
	module := NewUserModule()

	// Get shell for root user (exists on all Unix-like systems)
	shell, err := module.getUserShell("root")
	if err != nil {
		t.Fatalf("getUserShell failed: %v", err)
	}

	if shell == "" {
		t.Error("Expected non-empty shell")
	}

	// Shell should be a valid path
	if shell[0] != '/' {
		t.Errorf("Expected shell to be absolute path, got '%s'", shell)
	}
}

func TestUserModule_GetUserGroups(t *testing.T) {
	module := NewUserModule()

	// Get groups for root user
	groups, err := module.getUserGroups("root")
	if err != nil {
		t.Fatalf("getUserGroups failed: %v", err)
	}

	if len(groups) == 0 {
		t.Error("Expected at least one group")
	}
}

func TestUserModule_StringSlicesEqual(t *testing.T) {
	tests := []struct {
		name     string
		a        []string
		b        []string
		expected bool
	}{
		{
			name:     "empty slices",
			a:        []string{},
			b:        []string{},
			expected: true,
		},
		{
			name:     "same elements same order",
			a:        []string{"a", "b", "c"},
			b:        []string{"a", "b", "c"},
			expected: true,
		},
		{
			name:     "same elements different order",
			a:        []string{"a", "b", "c"},
			b:        []string{"c", "a", "b"},
			expected: true,
		},
		{
			name:     "different lengths",
			a:        []string{"a", "b"},
			b:        []string{"a", "b", "c"},
			expected: false,
		},
		{
			name:     "different elements",
			a:        []string{"a", "b", "c"},
			b:        []string{"a", "b", "d"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stringSlicesEqual(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("stringSlicesEqual(%v, %v) = %v, want %v", tt.a, tt.b, result, tt.expected)
			}
		})
	}
}

// Darwin-specific tests

func TestUserModule_FindNextAvailableUID_Darwin(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Skipping macOS-specific test")
	}

	module := NewUserModule()
	ctx := context.Background()

	uid, err := module.findNextAvailableUID(ctx)
	if err != nil {
		t.Fatalf("findNextAvailableUID failed: %v", err)
	}

	// UID should be >= 501 (regular users on macOS start at 501)
	if uid < 501 {
		t.Errorf("Expected UID >= 501, got %d", uid)
	}
}

func TestUserModule_DsclRead_Darwin(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Skipping macOS-specific test")
	}

	module := NewUserModule()
	ctx := context.Background()

	// Read shell for root user
	shell, err := module.dsclRead(ctx, "/Users/root", "UserShell")
	if err != nil {
		t.Fatalf("dsclRead failed: %v", err)
	}

	if shell == "" {
		t.Error("Expected non-empty shell")
	}
}

func TestUserModule_GetUserShell_Darwin(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Skipping macOS-specific test")
	}

	module := NewUserModule()

	// Test with current user
	shell, err := module.getUserShell("root")
	if err != nil {
		t.Fatalf("getUserShell failed: %v", err)
	}

	// Root on macOS typically has /bin/sh
	if shell == "" {
		t.Error("Expected non-empty shell for root")
	}
}

func TestUserModule_Check_Darwin_UserAttributes(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Skipping macOS-specific test")
	}

	module := NewUserModule()
	ctx := context.Background()

	// Check root user with specific attributes
	decl := &StateDeclaration{
		ID:     "root",
		Module: "user",
		State:  "present",
		Parameters: map[string]interface{}{
			"uid": 0, // root is always UID 0
		},
	}

	result, err := module.Check(ctx, decl)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	if !result.Present {
		t.Error("Expected root user to be present")
	}

	// UID 0 should match
	if !result.Matches {
		t.Errorf("Expected state to match (root should have UID 0), diff: %v", result.Diff)
	}
}
