package audit

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewAPISnapshot(t *testing.T) {
	s := NewAPISnapshot("test-module", "1.0.0")

	if s.Module != "test-module" {
		t.Errorf("Module = %s, want test-module", s.Module)
	}
	if s.Version != "1.0.0" {
		t.Errorf("Version = %s, want 1.0.0", s.Version)
	}
	if len(s.Elements) != 0 {
		t.Errorf("Elements should be empty")
	}
}

func TestAPISnapshot_Add(t *testing.T) {
	s := NewAPISnapshot("test", "1.0")

	elem := &APIElement{
		Name:     "TestFunc",
		Kind:     "function",
		Exported: true,
	}

	s.Add(elem)

	if len(s.Elements) != 1 {
		t.Errorf("Elements count = %d, want 1", len(s.Elements))
	}

	key := "function.TestFunc"
	if _, exists := s.Elements[key]; !exists {
		t.Errorf("Element not found with key %s", key)
	}
}

func TestNewAuditor(t *testing.T) {
	a := NewAuditor()

	if !a.IgnorePrivate {
		t.Error("IgnorePrivate should be true by default")
	}
	if !a.IgnoreInternal {
		t.Error("IgnoreInternal should be true by default")
	}
	if a.StrictMode {
		t.Error("StrictMode should be false by default")
	}
}

func TestAuditor_CompareSnapshots_NoChanges(t *testing.T) {
	a := NewAuditor()

	old := NewAPISnapshot("test", "1.0.0")
	old.Add(&APIElement{Name: "Func1", Kind: "function", Exported: true})

	updated := NewAPISnapshot("test", "1.0.1")
	updated.Add(&APIElement{Name: "Func1", Kind: "function", Exported: true})

	result := a.CompareSnapshots(old, updated)

	if len(result.Changes) != 0 {
		t.Errorf("Expected no changes, got %d", len(result.Changes))
	}
	if !result.Compatible {
		t.Error("Should be compatible")
	}
}

func TestAuditor_CompareSnapshots_Added(t *testing.T) {
	a := NewAuditor()

	old := NewAPISnapshot("test", "1.0.0")
	old.Add(&APIElement{Name: "Func1", Kind: "function", Exported: true})

	updated := NewAPISnapshot("test", "1.1.0")
	updated.Add(&APIElement{Name: "Func1", Kind: "function", Exported: true})
	updated.Add(&APIElement{Name: "Func2", Kind: "function", Exported: true})

	result := a.CompareSnapshots(old, updated)

	if len(result.Changes) != 1 {
		t.Errorf("Expected 1 change, got %d", len(result.Changes))
	}
	if result.Changes[0].Type != ChangeAdded {
		t.Errorf("Change type = %s, want added", result.Changes[0].Type)
	}
	if result.Changes[0].Severity != SeverityMinor {
		t.Errorf("Severity = %s, want minor", result.Changes[0].Severity)
	}
	if result.MinorCount != 1 {
		t.Errorf("MinorCount = %d, want 1", result.MinorCount)
	}
	if !result.Compatible {
		t.Error("Adding should be compatible")
	}
}

func TestAuditor_CompareSnapshots_Removed(t *testing.T) {
	a := NewAuditor()

	old := NewAPISnapshot("test", "1.0.0")
	old.Add(&APIElement{Name: "Func1", Kind: "function", Exported: true})
	old.Add(&APIElement{Name: "Func2", Kind: "function", Exported: true})

	updated := NewAPISnapshot("test", "2.0.0")
	updated.Add(&APIElement{Name: "Func1", Kind: "function", Exported: true})

	result := a.CompareSnapshots(old, updated)

	if len(result.Changes) != 1 {
		t.Errorf("Expected 1 change, got %d", len(result.Changes))
	}
	if result.Changes[0].Type != ChangeRemoved {
		t.Errorf("Change type = %s, want removed", result.Changes[0].Type)
	}
	if result.Changes[0].Severity != SeverityMajor {
		t.Errorf("Severity = %s, want major", result.Changes[0].Severity)
	}
	if result.BreakingCount != 1 {
		t.Errorf("BreakingCount = %d, want 1", result.BreakingCount)
	}
	if result.Compatible {
		t.Error("Removing should not be compatible")
	}
}

func TestAuditor_CompareSnapshots_SignatureChanged(t *testing.T) {
	a := NewAuditor()

	old := NewAPISnapshot("test", "1.0.0")
	old.Add(&APIElement{
		Name:      "Func1",
		Kind:      "function",
		Signature: "(a int) error",
		Exported:  true,
	})

	updated := NewAPISnapshot("test", "2.0.0")
	updated.Add(&APIElement{
		Name:      "Func1",
		Kind:      "function",
		Signature: "(a int, b string) error",
		Exported:  true,
	})

	result := a.CompareSnapshots(old, updated)

	if len(result.Changes) != 1 {
		t.Errorf("Expected 1 change, got %d", len(result.Changes))
	}
	if result.Changes[0].Type != ChangeModified {
		t.Errorf("Change type = %s, want modified", result.Changes[0].Type)
	}
	if result.Changes[0].Severity != SeverityMajor {
		t.Errorf("Severity = %s, want major", result.Changes[0].Severity)
	}
	if result.Compatible {
		t.Error("Signature change should not be compatible")
	}
}

func TestAuditor_CompareSnapshots_Deprecated(t *testing.T) {
	a := NewAuditor()

	old := NewAPISnapshot("test", "1.0.0")
	old.Add(&APIElement{
		Name:       "Func1",
		Kind:       "function",
		Exported:   true,
		Deprecated: false,
	})

	updated := NewAPISnapshot("test", "1.0.1")
	updated.Add(&APIElement{
		Name:       "Func1",
		Kind:       "function",
		Exported:   true,
		Deprecated: true,
	})

	result := a.CompareSnapshots(old, updated)

	if len(result.Changes) != 1 {
		t.Errorf("Expected 1 change, got %d", len(result.Changes))
	}
	if result.Changes[0].Type != ChangeDeprecated {
		t.Errorf("Change type = %s, want deprecated", result.Changes[0].Type)
	}
	if result.Changes[0].Severity != SeverityPatch {
		t.Errorf("Severity = %s, want patch", result.Changes[0].Severity)
	}
	if !result.Compatible {
		t.Error("Deprecation should be compatible")
	}
}

func TestAuditor_CompareSnapshots_IgnorePrivate(t *testing.T) {
	a := NewAuditor()
	a.IgnorePrivate = true

	old := NewAPISnapshot("test", "1.0.0")
	old.Add(&APIElement{Name: "privateFunc", Kind: "function", Exported: false})

	updated := NewAPISnapshot("test", "1.0.1")
	// Private function removed

	result := a.CompareSnapshots(old, updated)

	if len(result.Changes) != 0 {
		t.Errorf("Expected no changes when ignoring private, got %d", len(result.Changes))
	}
}

func TestAuditor_CompareSnapshots_StrictMode(t *testing.T) {
	a := NewAuditor()
	a.StrictMode = true

	old := NewAPISnapshot("test", "1.0.0")
	old.Add(&APIElement{
		Name:     "Func1",
		Kind:     "function",
		Exported: true,
		Doc:      "Old documentation",
	})

	updated := NewAPISnapshot("test", "1.0.1")
	updated.Add(&APIElement{
		Name:     "Func1",
		Kind:     "function",
		Exported: true,
		Doc:      "Updated documentation",
	})

	result := a.CompareSnapshots(old, updated)

	if len(result.Changes) != 1 {
		t.Errorf("Expected 1 change in strict mode, got %d", len(result.Changes))
	}
	if result.Changes[0].Severity != SeverityPatch {
		t.Errorf("Doc change should be patch severity")
	}
}

func TestAuditor_ExtractSnapshot(t *testing.T) {
	tmpDir := t.TempDir()

	testCode := `// Package example provides example functionality.
package example

// ErrExample is an example error.
var ErrExample = errors.New("example error")

// MaxSize is the maximum size constant.
const MaxSize = 1024

// Config represents configuration.
type Config struct {
	// Name is the config name.
	Name string
	// Value is the config value.
	Value int
}

// NewConfig creates a new Config.
func NewConfig(name string, value int) *Config {
	return &Config{Name: name, Value: value}
}

// Validate validates the config.
func (c *Config) Validate() error {
	return nil
}

// Handler defines a handler interface.
type Handler interface {
	Handle(data []byte) error
}

// privateFunc is not exported.
func privateFunc() {}
`

	err := os.WriteFile(filepath.Join(tmpDir, "example.go"), []byte(testCode), 0644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	a := NewAuditor()
	snapshot, err := a.ExtractSnapshot("example-module", "1.0.0", tmpDir)
	if err != nil {
		t.Fatalf("ExtractSnapshot failed: %v", err)
	}

	if snapshot.Module != "example-module" {
		t.Errorf("Module = %s, want example-module", snapshot.Module)
	}

	// Check for exported elements
	expectedElements := []string{
		"var.ErrExample",
		"const.MaxSize",
		"struct.Config",
		"field.Config.Name",
		"field.Config.Value",
		"function.NewConfig",
		"method.Config.Validate",
		"interface.Handler",
		"interface_method.Handler.Handle",
	}

	for _, key := range expectedElements {
		if _, exists := snapshot.Elements[key]; !exists {
			t.Errorf("Missing expected element: %s", key)
		}
	}

	// privateFunc should not be present
	if _, exists := snapshot.Elements["function.privateFunc"]; exists {
		t.Error("Private function should not be included")
	}
}

func TestResult_Format(t *testing.T) {
	result := &Result{
		Module:     "test-module",
		OldVersion: "1.0.0",
		NewVersion: "2.0.0",
		Changes: []*Change{
			{
				Type:        ChangeRemoved,
				Severity:    SeverityMajor,
				Element:     "OldFunc",
				Kind:        "function",
				Description: "Removed function OldFunc",
				Suggestion:  "Use NewFunc instead",
			},
			{
				Type:        ChangeAdded,
				Severity:    SeverityMinor,
				Element:     "NewFunc",
				Kind:        "function",
				Description: "Added function NewFunc",
			},
			{
				Type:        ChangeDeprecated,
				Severity:    SeverityPatch,
				Element:     "LegacyFunc",
				Kind:        "function",
				Description: "Deprecated function LegacyFunc",
			},
		},
		BreakingCount: 1,
		MinorCount:    1,
		PatchCount:    1,
		Compatible:    false,
	}

	output := result.Format()

	if output == "" {
		t.Error("Format should return non-empty string")
	}

	// Check for key sections
	expectedContents := []string{
		"test-module",
		"1.0.0 -> 2.0.0",
		"BREAKING CHANGES",
		"Removed function OldFunc",
		"Use NewFunc instead",
		"Minor Changes",
		"Added function NewFunc",
		"Patch Changes",
		"Deprecated function LegacyFunc",
		"Breaking: 1",
		"breaking changes",
	}

	for _, expected := range expectedContents {
		if !containsString(output, expected) {
			t.Errorf("Output missing expected content: %s", expected)
		}
	}
}

func TestResult_Format_NoChanges(t *testing.T) {
	result := &Result{
		Module:     "test-module",
		OldVersion: "1.0.0",
		NewVersion: "1.0.1",
		Changes:    []*Change{},
		Compatible: true,
	}

	output := result.Format()

	if !containsString(output, "No API changes detected") {
		t.Error("Should indicate no changes")
	}
}

func TestResult_SuggestedVersion(t *testing.T) {
	tests := []struct {
		breaking int
		minor    int
		patch    int
		expected string
	}{
		{1, 0, 0, "major"},
		{0, 1, 0, "minor"},
		{0, 0, 1, "patch"},
		{0, 0, 0, "patch"},
		{1, 1, 1, "major"},
		{0, 1, 1, "minor"},
	}

	for _, tt := range tests {
		result := &Result{
			BreakingCount: tt.breaking,
			MinorCount:    tt.minor,
			PatchCount:    tt.patch,
		}

		suggested := result.SuggestedVersion()
		if suggested != tt.expected {
			t.Errorf("SuggestedVersion with breaking=%d, minor=%d, patch=%d = %s, want %s",
				tt.breaking, tt.minor, tt.patch, suggested, tt.expected)
		}
	}
}

func TestFormatFuncSignature(t *testing.T) {
	// This is tested indirectly through ExtractSnapshot
	// but we can verify the output format
	a := NewAuditor()
	tmpDir := t.TempDir()

	testCode := `package test

func SimpleFunc() {}
func WithParams(a int, b string) {}
func WithReturn() error { return nil }
func Complex(a, b int, c string) (int, error) { return 0, nil }
func Variadic(args ...string) {}
`

	err := os.WriteFile(filepath.Join(tmpDir, "test.go"), []byte(testCode), 0644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	snapshot, err := a.ExtractSnapshot("test", "1.0", tmpDir)
	if err != nil {
		t.Fatalf("ExtractSnapshot failed: %v", err)
	}

	// Check signatures are captured
	if elem, ok := snapshot.Elements["function.SimpleFunc"]; ok {
		if elem.Signature != "()" {
			t.Errorf("SimpleFunc signature = %s, want ()", elem.Signature)
		}
	}

	if elem, ok := snapshot.Elements["function.WithReturn"]; ok {
		if elem.Signature != "() error" {
			t.Errorf("WithReturn signature = %s, want () error", elem.Signature)
		}
	}
}

func TestCompareSnapshots_MultipleChanges(t *testing.T) {
	a := NewAuditor()

	old := NewAPISnapshot("test", "1.0.0")
	old.Add(&APIElement{Name: "Func1", Kind: "function", Exported: true})
	old.Add(&APIElement{Name: "Func2", Kind: "function", Exported: true})
	old.Add(&APIElement{Name: "Func3", Kind: "function", Exported: true, Signature: "(a int)"})

	updated := NewAPISnapshot("test", "2.0.0")
	// Func1 removed
	updated.Add(&APIElement{Name: "Func2", Kind: "function", Exported: true})
	updated.Add(&APIElement{Name: "Func3", Kind: "function", Exported: true, Signature: "(a int, b string)"})
	updated.Add(&APIElement{Name: "Func4", Kind: "function", Exported: true})

	result := a.CompareSnapshots(old, updated)

	if result.BreakingCount != 2 {
		t.Errorf("BreakingCount = %d, want 2 (1 removed + 1 signature change)", result.BreakingCount)
	}
	if result.MinorCount != 1 {
		t.Errorf("MinorCount = %d, want 1 (1 added)", result.MinorCount)
	}
	if result.Compatible {
		t.Error("Should not be compatible with breaking changes")
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
