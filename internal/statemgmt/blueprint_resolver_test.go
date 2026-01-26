package statemgmt

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/shawnbutts/keystone-core/internal/blueprint"
)

func TestNewBlueprintResolver(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := blueprint.NewLocalStorage(tmpDir, false)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	resolver := NewBlueprintResolver(storage, tmpDir)

	if resolver == nil {
		t.Fatal("NewBlueprintResolver returned nil")
	}

	if resolver.loader == nil {
		t.Error("resolver.loader is nil")
	}

	if resolver.parser == nil {
		t.Error("resolver.parser is nil")
	}
}

func TestBlueprintResolver_Resolve_NoBlueprints(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := blueprint.NewLocalStorage(tmpDir, false)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	resolver := NewBlueprintResolver(storage, tmpDir)

	stateFile := &StateFile{
		Path: "/test/state.yaml",
		States: map[string][]StateDeclaration{
			"file": {
				{ID: "/etc/test.conf", Module: "file", State: "present"},
			},
		},
		Variables: map[string]interface{}{
			"key": "value",
		},
	}

	ctx := context.Background()
	resolved, err := resolver.Resolve(ctx, stateFile)

	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	if resolved.Original != stateFile {
		t.Error("resolved.Original should be the original state file")
	}

	// Variables should be copied
	if resolved.MergedVariables["key"] != "value" {
		t.Errorf("MergedVariables[key] = %v, want 'value'", resolved.MergedVariables["key"])
	}

	// States should be copied
	if len(resolved.MergedStates["file"]) != 1 {
		t.Errorf("MergedStates[file] length = %d, want 1", len(resolved.MergedStates["file"]))
	}

	// No blueprint states
	if len(resolved.BlueprintStates) != 0 {
		t.Errorf("BlueprintStates length = %d, want 0", len(resolved.BlueprintStates))
	}
}

func TestBlueprintResolver_Resolve_WithBlueprint(t *testing.T) {
	tmpDir := t.TempDir()
	blueprintsDir := filepath.Join(tmpDir, "blueprints")

	// Create a blueprint directory (storage expects {basePath}/{vendor}/{name})
	bpDir := filepath.Join(blueprintsDir, "test", "web-stack")
	if err := os.MkdirAll(bpDir, 0755); err != nil {
		t.Fatalf("Failed to create blueprint dir: %v", err)
	}

	// Create blueprint.yaml
	bpManifest := `
apiVersion: blueprints.kscore.io/v1
kind: Blueprint
metadata:
  name: web-stack
  version: 1.0.0
parameters:
  port:
    type: integer
    default: 8080
entrypoints:
  default: states/init.yaml
`
	if err := os.WriteFile(filepath.Join(bpDir, "blueprint.yaml"), []byte(bpManifest), 0644); err != nil {
		t.Fatalf("Failed to write blueprint.yaml: %v", err)
	}

	// Create states directory
	statesDir := filepath.Join(bpDir, "states")
	if err := os.MkdirAll(statesDir, 0755); err != nil {
		t.Fatalf("Failed to create states dir: %v", err)
	}

	// Create init.yaml with template
	initState := `
service:
  web-app:
    state: running
    port: {{ .port }}
`
	if err := os.WriteFile(filepath.Join(statesDir, "init.yaml"), []byte(initState), 0644); err != nil {
		t.Fatalf("Failed to write init.yaml: %v", err)
	}

	// Create storage and resolver
	storage, err := blueprint.NewLocalStorage(blueprintsDir, false)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	resolver := NewBlueprintResolver(storage, blueprintsDir)

	// Create state file with blueprint include
	stateFile := &StateFile{
		Path: filepath.Join(tmpDir, "main.yaml"),
		BlueprintIncludes: []BlueprintInclude{
			{
				Blueprint: "test/web-stack",
				Version:   "1.0.0",
				Parameters: map[string]interface{}{
					"port": 3000,
				},
			},
		},
		States: map[string][]StateDeclaration{
			"package": {
				{ID: "nginx", Module: "package", State: "installed"},
			},
		},
	}

	ctx := context.Background()
	resolved, err := resolver.Resolve(ctx, stateFile)

	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	// Should have one blueprint state
	if len(resolved.BlueprintStates) != 1 {
		t.Fatalf("BlueprintStates length = %d, want 1", len(resolved.BlueprintStates))
	}

	// Check the blueprint was resolved
	bpState := resolved.BlueprintStates["web-stack"]
	if bpState == nil {
		t.Fatal("BlueprintStates[web-stack] is nil")
	}

	if bpState.Namespace != "web-stack" {
		t.Errorf("Namespace = %s, want 'web-stack'", bpState.Namespace)
	}

	// Original states should still be present
	if len(resolved.MergedStates["package"]) != 1 {
		t.Errorf("MergedStates[package] length = %d, want 1", len(resolved.MergedStates["package"]))
	}
}

func TestBlueprintResolver_Resolve_WithNamespace(t *testing.T) {
	tmpDir := t.TempDir()
	blueprintsDir := filepath.Join(tmpDir, "blueprints")

	// Create a blueprint directory (storage expects {basePath}/{vendor}/{name})
	bpDir := filepath.Join(blueprintsDir, "test", "simple")
	if err := os.MkdirAll(bpDir, 0755); err != nil {
		t.Fatalf("Failed to create blueprint dir: %v", err)
	}

	// Create blueprint.yaml
	bpManifest := `
apiVersion: blueprints.kscore.io/v1
kind: Blueprint
metadata:
  name: simple
  version: 1.0.0
entrypoints:
  default: states/main.yaml
`
	if err := os.WriteFile(filepath.Join(bpDir, "blueprint.yaml"), []byte(bpManifest), 0644); err != nil {
		t.Fatalf("Failed to write blueprint.yaml: %v", err)
	}

	// Create states directory
	statesDir := filepath.Join(bpDir, "states")
	if err := os.MkdirAll(statesDir, 0755); err != nil {
		t.Fatalf("Failed to create states dir: %v", err)
	}

	// Create main.yaml
	mainState := `
file:
  /tmp/test.txt:
    state: present
    content: "test"
`
	if err := os.WriteFile(filepath.Join(statesDir, "main.yaml"), []byte(mainState), 0644); err != nil {
		t.Fatalf("Failed to write main.yaml: %v", err)
	}

	// Create storage and resolver
	storage, err := blueprint.NewLocalStorage(blueprintsDir, false)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	resolver := NewBlueprintResolver(storage, blueprintsDir)

	// Create state file with blueprint include using "as" namespace
	stateFile := &StateFile{
		Path: filepath.Join(tmpDir, "main.yaml"),
		BlueprintIncludes: []BlueprintInclude{
			{
				Blueprint: "test/simple",
				Version:   "1.0.0",
				As:        "myns",
			},
		},
	}

	ctx := context.Background()
	resolved, err := resolver.Resolve(ctx, stateFile)

	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	// Check the blueprint was resolved with the namespace
	bpState := resolved.BlueprintStates["myns"]
	if bpState == nil {
		t.Fatal("BlueprintStates[myns] is nil")
	}

	if bpState.Namespace != "myns" {
		t.Errorf("Namespace = %s, want 'myns'", bpState.Namespace)
	}

	// State IDs should be namespaced
	if len(resolved.MergedStates["file"]) > 0 {
		stateID := resolved.MergedStates["file"][0].ID
		if stateID != "myns:/tmp/test.txt" {
			t.Errorf("State ID = %s, want 'myns:/tmp/test.txt'", stateID)
		}
	}
}

func TestNamespaceStateDeclarations(t *testing.T) {
	resolver := &BlueprintResolver{}

	decls := []StateDeclaration{
		{
			ID:     "/etc/test.conf",
			Module: "file",
			State:  "present",
			Requisites: Requisites{
				Require: []StateReference{
					{Module: "package", ID: "nginx"},
				},
			},
		},
	}

	// Without namespace
	result := resolver.namespaceStateDeclarations(decls, "ns", false)
	if result[0].ID != "/etc/test.conf" {
		t.Errorf("Without namespace: ID = %s, want '/etc/test.conf'", result[0].ID)
	}

	// With namespace
	result = resolver.namespaceStateDeclarations(decls, "ns", true)
	if result[0].ID != "ns:/etc/test.conf" {
		t.Errorf("With namespace: ID = %s, want 'ns:/etc/test.conf'", result[0].ID)
	}

	if result[0].Requisites.Require[0].ID != "ns:nginx" {
		t.Errorf("Requisite ID = %s, want 'ns:nginx'", result[0].Requisites.Require[0].ID)
	}
}

func TestNamespaceReferences(t *testing.T) {
	resolver := &BlueprintResolver{}

	refs := []StateReference{
		{Module: "file", ID: "/etc/test.conf"},
		{Module: "service", ID: "nginx"},
	}

	result := resolver.namespaceReferences(refs, "myns")

	if len(result) != 2 {
		t.Fatalf("result length = %d, want 2", len(result))
	}

	if result[0].ID != "myns:/etc/test.conf" {
		t.Errorf("result[0].ID = %s, want 'myns:/etc/test.conf'", result[0].ID)
	}

	if result[1].ID != "myns:nginx" {
		t.Errorf("result[1].ID = %s, want 'myns:nginx'", result[1].ID)
	}

	// Module should be unchanged
	if result[0].Module != "file" {
		t.Errorf("result[0].Module = %s, want 'file'", result[0].Module)
	}
}

func TestNamespaceReferences_Empty(t *testing.T) {
	resolver := &BlueprintResolver{}

	var refs []StateReference
	result := resolver.namespaceReferences(refs, "ns")

	if len(result) != 0 {
		t.Errorf("result length = %d, want 0", len(result))
	}
}

func TestParser_ParseBytes(t *testing.T) {
	parser := NewParser("")

	data := []byte(`
metadata:
  name: test
file:
  /tmp/test.txt:
    state: present
variables:
  key: value
`)

	stateFile, err := parser.parseBytes(data, "/test/state.yaml")
	if err != nil {
		t.Fatalf("parseBytes() error = %v", err)
	}

	if stateFile.Path != "/test/state.yaml" {
		t.Errorf("Path = %s, want '/test/state.yaml'", stateFile.Path)
	}

	if stateFile.Metadata.Name != "test" {
		t.Errorf("Metadata.Name = %s, want 'test'", stateFile.Metadata.Name)
	}

	if len(stateFile.States["file"]) != 1 {
		t.Errorf("States[file] length = %d, want 1", len(stateFile.States["file"]))
	}

	if stateFile.Variables["key"] != "value" {
		t.Errorf("Variables[key] = %v, want 'value'", stateFile.Variables["key"])
	}
}

func TestParser_ParseBytes_InvalidYAML(t *testing.T) {
	parser := NewParser("")

	data := []byte(`invalid: yaml: [`)

	_, err := parser.parseBytes(data, "/test/state.yaml")
	if err == nil {
		t.Error("parseBytes() should return error for invalid YAML")
	}
}
