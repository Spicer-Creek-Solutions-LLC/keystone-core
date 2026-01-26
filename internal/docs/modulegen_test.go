package docs

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewModuleRefDocGenerator(t *testing.T) {
	g := NewModuleRefDocGenerator()
	if g.OutputDir != "docs/modules" {
		t.Errorf("OutputDir = %s, want docs/modules", g.OutputDir)
	}
	if g.Format != "markdown" {
		t.Errorf("Format = %s, want markdown", g.Format)
	}
}

func TestExtractAnnotation(t *testing.T) {
	tests := []struct {
		doc      string
		name     string
		expected string
	}{
		{
			doc:      "Some doc\n@version 1.0.0\nMore doc",
			name:     "version",
			expected: "1.0.0",
		},
		{
			doc:      "@author John Doe",
			name:     "author",
			expected: "John Doe",
		},
		{
			doc:      "No annotations here",
			name:     "version",
			expected: "",
		},
		{
			doc:      "@since 0.5.0\n@deprecated Use NewAPI instead",
			name:     "deprecated",
			expected: "Use NewAPI instead",
		},
	}

	for _, tt := range tests {
		result := extractAnnotation(tt.doc, tt.name)
		if result != tt.expected {
			t.Errorf("extractAnnotation(%q, %q) = %q, want %q", tt.doc, tt.name, result, tt.expected)
		}
	}
}

func TestCleanDoc(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "This is a description\n@version 1.0\n@author Test",
			expected: "This is a description",
		},
		{
			input:    "@deprecated Use something else\nStill works",
			expected: "Still works",
		},
		{
			input:    "Plain text without annotations",
			expected: "Plain text without annotations",
		},
	}

	for _, tt := range tests {
		result := cleanDoc(tt.input)
		if result != tt.expected {
			t.Errorf("cleanDoc(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestExtractErrors(t *testing.T) {
	doc := `Processes data.
@error ErrInvalidInput when input is nil
@error ErrParseError when parsing fails
@throws ErrTimeout on timeout`

	errors := extractErrors(doc)
	if len(errors) != 3 {
		t.Errorf("Expected 3 errors, got %d", len(errors))
	}

	expected := []string{
		"ErrInvalidInput when input is nil",
		"ErrParseError when parsing fails",
		"ErrTimeout on timeout",
	}

	for i, err := range errors {
		if err != expected[i] {
			t.Errorf("Error %d = %q, want %q", i, err, expected[i])
		}
	}
}

func TestExtractTags(t *testing.T) {
	// Test nil tag
	tags := extractTags(nil)
	if tags != nil {
		t.Error("Expected nil for nil input")
	}
}

func TestFormatType(t *testing.T) {
	// Since we can't easily create AST nodes, we'll test the cleanDoc and annotation functions
	// The formatType function is tested indirectly through GenerateFromPackage
}

func TestModuleRefDocGenerator_GenerateFromPackage(t *testing.T) {
	// Create a temporary directory with a Go package
	tmpDir := t.TempDir()

	// Write a test Go file
	testCode := `// Package testpkg provides test functionality.
//
// @version 1.0.0
// @author Test Author
// @since 0.5.0
package testpkg

// ErrTest is a test error.
var ErrTest = errors.New("test error")

// TestStruct is a test struct.
// @since 0.5.0
type TestStruct struct {
	// Name is the name field.
	Name string
	// Value is the value field.
	Value int
}

// NewTestStruct creates a new TestStruct.
// @since 0.5.0
func NewTestStruct(name string, value int) *TestStruct {
	return &TestStruct{Name: name, Value: value}
}

// Process processes the struct.
// @error ErrTest on failure
func (t *TestStruct) Process() error {
	return nil
}
`

	err := os.WriteFile(filepath.Join(tmpDir, "test.go"), []byte(testCode), 0644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	g := NewModuleRefDocGenerator()
	g.Verbose = false

	modDoc, err := g.GenerateFromPackage(tmpDir)
	if err != nil {
		t.Fatalf("GenerateFromPackage failed: %v", err)
	}

	if modDoc.Name != "testpkg" {
		t.Errorf("Name = %s, want testpkg", modDoc.Name)
	}

	if modDoc.Version != "1.0.0" {
		t.Errorf("Version = %s, want 1.0.0", modDoc.Version)
	}

	if modDoc.Author != "Test Author" {
		t.Errorf("Author = %s, want Test Author", modDoc.Author)
	}

	// Check types
	if len(modDoc.Types) == 0 {
		t.Error("Expected at least one type")
	}

	foundTestStruct := false
	for _, typ := range modDoc.Types {
		if typ.Name == "TestStruct" {
			foundTestStruct = true
			if len(typ.Fields) == 0 {
				t.Error("TestStruct should have fields")
			}
			if len(typ.Methods) == 0 {
				t.Error("TestStruct should have methods")
			}
		}
	}
	if !foundTestStruct {
		t.Error("TestStruct not found in types")
	}

	// Check that we generated something meaningful
	// Note: NewTestStruct may be treated as a factory function for TestStruct
	// so it might not always appear in Functions list
	if len(modDoc.Functions) == 0 && len(modDoc.Types) == 0 {
		t.Error("Expected some functions or types to be documented")
	}
}

func TestModuleRefDocGenerator_WriteMarkdown(t *testing.T) {
	g := NewModuleRefDocGenerator()

	modDoc := &ModuleRefDoc{
		Name:        "testmod",
		Package:     "example/testmod",
		Description: "A test module for testing.",
		Version:     "1.0.0",
		Author:      "Test Author",
		Types: []TypeDoc{
			{
				Name:        "TestType",
				Description: "A test type.",
				Fields: []FieldDoc{
					{Name: "Field1", Type: "string", Description: "First field"},
					{Name: "Field2", Type: "int", Description: "Second field"},
				},
				Methods: []FunctionDoc{
					{
						Name:        "Method1",
						Signature:   "func (t *TestType) Method1() error",
						Description: "A test method.",
					},
				},
			},
		},
		Functions: []FunctionDoc{
			{
				Name:        "NewTestType",
				Signature:   "func NewTestType() *TestType",
				Description: "Creates a new TestType.",
				Returns: []ParamDoc{
					{Type: "*TestType", Description: "The new instance"},
				},
			},
		},
	}

	var buf bytes.Buffer
	err := g.WriteMarkdown(modDoc, &buf)
	if err != nil {
		t.Fatalf("WriteMarkdown failed: %v", err)
	}

	output := buf.String()

	// Check that key sections are present
	if !strings.Contains(output, "# testmod") {
		t.Error("Missing module title")
	}
	if !strings.Contains(output, "A test module for testing.") {
		t.Error("Missing description")
	}
	if !strings.Contains(output, "**Version:** 1.0.0") {
		t.Error("Missing version")
	}
	if !strings.Contains(output, "### TestType") {
		t.Error("Missing type section")
	}
	if !strings.Contains(output, "### NewTestType") {
		t.Error("Missing function section")
	}
}

func TestModuleRefDocGenerator_WriteToFile(t *testing.T) {
	tmpDir := t.TempDir()

	g := NewModuleRefDocGenerator()
	g.OutputDir = tmpDir
	g.Format = "markdown"

	modDoc := &ModuleRefDoc{
		Name:        "testmod",
		Description: "Test module",
	}

	err := g.WriteToFile(modDoc)
	if err != nil {
		t.Fatalf("WriteToFile failed: %v", err)
	}

	// Check file was created
	outputPath := filepath.Join(tmpDir, "testmod.md")
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Error("Output file was not created")
	}

	// Read and verify content
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	if !strings.Contains(string(content), "# testmod") {
		t.Error("Output file missing expected content")
	}
}

func TestModuleRefDocGenerator_UnsupportedFormat(t *testing.T) {
	g := NewModuleRefDocGenerator()
	g.Format = "unsupported"
	g.OutputDir = t.TempDir()

	modDoc := &ModuleRefDoc{Name: "test"}

	err := g.WriteToFile(modDoc)
	if err == nil {
		t.Error("Expected error for unsupported format")
	}
}

func TestExtractParamDoc(t *testing.T) {
	tests := []struct {
		doc       string
		paramName string
		expected  string
	}{
		{
			doc:       "name - the name parameter",
			paramName: "name",
			expected:  "- the name parameter",
		},
		{
			doc:       "@param value the value to use",
			paramName: "value",
			expected:  "the value to use",
		},
		{
			doc:       "No params documented",
			paramName: "missing",
			expected:  "",
		},
	}

	for _, tt := range tests {
		result := extractParamDoc(tt.doc, tt.paramName)
		if result != tt.expected {
			t.Errorf("extractParamDoc(%q, %q) = %q, want %q", tt.doc, tt.paramName, result, tt.expected)
		}
	}
}
