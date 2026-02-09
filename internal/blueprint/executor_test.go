package blueprint

import (
	"context"
	"testing"
)

func TestNewExecutor(t *testing.T) {
	tests := []struct {
		name    string
		config  *ExecutorConfig
		wantErr bool
	}{
		{
			name:    "nil config uses defaults",
			config:  nil,
			wantErr: false,
		},
		{
			name: "custom config",
			config: &ExecutorConfig{
				ConcurrentBlueprints: 4,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec, err := NewExecutor(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewExecutor() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && exec == nil {
				t.Error("NewExecutor() returned nil executor")
			}
		})
	}
}

func TestDefaultExecutorConfig(t *testing.T) {
	config := DefaultExecutorConfig()

	if config == nil {
		t.Fatal("DefaultExecutorConfig() returned nil")
	}

	if config.ConcurrentBlueprints != 1 {
		t.Errorf("ConcurrentBlueprints = %d, want 1", config.ConcurrentBlueprints)
	}

	if config.Timeout == 0 {
		t.Error("Timeout should not be zero")
	}
}

func TestExpandIncludes_NoBlueprintIncludes(t *testing.T) {
	exec, err := NewExecutor(nil)
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	stateFile := &StateFile{
		Path:              "/test/state.yaml",
		BlueprintIncludes: nil,
		Variables: map[string]interface{}{
			"foo": "bar",
		},
		States: map[string][]StateDeclaration{
			"file": {
				{
					ID:     "test_file",
					Module: "file",
					State:  "present",
				},
			},
		},
	}

	ctx := context.Background()
	expanded, err := exec.ExpandIncludes(ctx, stateFile)
	if err != nil {
		t.Fatalf("ExpandIncludes() error = %v", err)
	}

	// Should return the same state file when no blueprint includes
	if expanded.Path != stateFile.Path {
		t.Errorf("Path = %s, want %s", expanded.Path, stateFile.Path)
	}

	if len(expanded.States["file"]) != 1 {
		t.Errorf("States[file] length = %d, want 1", len(expanded.States["file"]))
	}
}

func TestExecutor_GetAppliedBlueprints(t *testing.T) {
	exec, err := NewExecutor(nil)
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	// Initially empty
	applied := exec.GetAppliedBlueprints()
	if len(applied) != 0 {
		t.Errorf("GetAppliedBlueprints() length = %d, want 0", len(applied))
	}

	// After clearing should still be empty
	exec.ClearAppliedBlueprints()
	applied = exec.GetAppliedBlueprints()
	if len(applied) != 0 {
		t.Errorf("GetAppliedBlueprints() after clear length = %d, want 0", len(applied))
	}
}

func TestExecutor_GetAppliedBlueprint(t *testing.T) {
	exec, err := NewExecutor(nil)
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	// Non-existent namespace
	bp := exec.GetAppliedBlueprint("non-existent")
	if bp != nil {
		t.Error("GetAppliedBlueprint() should return nil for non-existent namespace")
	}
}

func TestParseBlueprintReference(t *testing.T) {
	tests := []struct {
		ref         string
		wantName    string
		wantVersion string
	}{
		{
			ref:         "vendor/name",
			wantName:    "vendor/name",
			wantVersion: "",
		},
		{
			ref:         "vendor/name@1.0.0",
			wantName:    "vendor/name",
			wantVersion: "1.0.0",
		},
		{
			ref:         "vendor/name@>=1.0.0",
			wantName:    "vendor/name",
			wantVersion: ">=1.0.0",
		},
		{
			ref:         "myorg/web-stack@1.2.3",
			wantName:    "myorg/web-stack",
			wantVersion: "1.2.3",
		},
		{
			ref:         "simple",
			wantName:    "simple",
			wantVersion: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.ref, func(t *testing.T) {
			name, version := ParseBlueprintReference(tt.ref)
			if name != tt.wantName {
				t.Errorf("ParseBlueprintReference() name = %s, want %s", name, tt.wantName)
			}
			if version != tt.wantVersion {
				t.Errorf("ParseBlueprintReference() version = %s, want %s", version, tt.wantVersion)
			}
		})
	}
}

func TestGetDefaultState(t *testing.T) {
	tests := []struct {
		module string
		want   string
	}{
		{"file", "present"},
		{"package", "installed"},
		{"service", "running"},
		{"user", "present"},
		{"group", "present"},
		{"unknown", "present"},
	}

	for _, tt := range tests {
		t.Run(tt.module, func(t *testing.T) {
			got := getDefaultState(tt.module)
			if got != tt.want {
				t.Errorf("getDefaultState(%s) = %s, want %s", tt.module, got, tt.want)
			}
		})
	}
}

func TestIsRequisiteField(t *testing.T) {
	requisiteFields := []string{
		"require", "require_in",
		"watch", "watch_in",
		"prereq", "prereq_in",
		"onchanges", "onchanges_in",
	}

	for _, field := range requisiteFields {
		if !isRequisiteField(field) {
			t.Errorf("isRequisiteField(%s) = false, want true", field)
		}
	}

	nonRequisiteFields := []string{
		"name", "state", "mode", "user", "group", "contents",
	}

	for _, field := range nonRequisiteFields {
		if isRequisiteField(field) {
			t.Errorf("isRequisiteField(%s) = true, want false", field)
		}
	}
}

func TestParseRequisites(t *testing.T) {
	params := map[string]interface{}{
		"require": []interface{}{
			map[string]interface{}{"file": "test_file"},
		},
		"watch": []interface{}{
			map[string]interface{}{"service": "nginx"},
		},
		"name": "should_remain",
	}

	reqs := parseRequisites(params)

	if len(reqs.Require) != 1 {
		t.Errorf("Require length = %d, want 1", len(reqs.Require))
	}

	if len(reqs.Watch) != 1 {
		t.Errorf("Watch length = %d, want 1", len(reqs.Watch))
	}

	// Check that non-requisite fields remain
	if _, ok := params["name"]; !ok {
		t.Error("Non-requisite field 'name' should remain in params")
	}

	// Check that requisite fields were deleted
	if _, ok := params["require"]; ok {
		t.Error("Requisite field 'require' should have been deleted")
	}
}

func TestParseStateReferences(t *testing.T) {
	refs := []interface{}{
		map[string]interface{}{"file": "test_file"},
		map[string]interface{}{"service": "nginx"},
		"invalid", // Should be skipped
	}

	result := parseStateReferences(refs)

	if len(result) != 2 {
		t.Errorf("parseStateReferences() length = %d, want 2", len(result))
	}

	// Check first reference
	if result[0].Module != "file" || result[0].ID != "test_file" {
		t.Errorf("First reference = %+v, want {file, test_file}", result[0])
	}

	// Check second reference
	if result[1].Module != "service" || result[1].ID != "nginx" {
		t.Errorf("Second reference = %+v, want {service, nginx}", result[1])
	}
}

func TestParseRenderedState(t *testing.T) {
	exec, err := NewExecutor(nil)
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	rendered := []byte(`
file:
  test_file:
    state: present
    name: /tmp/test
    mode: "0644"
    require:
      - package: nginx

package:
  nginx:
    state: installed

service:
  nginx_service:
    state: running
    watch:
      - file: test_file

metadata:
  name: should_be_skipped

variables:
  should_be_skipped: true
`)

	states, err := exec.parseRenderedState(rendered)
	if err != nil {
		t.Fatalf("parseRenderedState() error = %v", err)
	}

	// Check file states
	if len(states["file"]) != 1 {
		t.Errorf("file states length = %d, want 1", len(states["file"]))
	}
	if states["file"][0].ID != "test_file" {
		t.Errorf("file state ID = %s, want test_file", states["file"][0].ID)
	}
	if states["file"][0].State != "present" {
		t.Errorf("file state State = %s, want present", states["file"][0].State)
	}
	if len(states["file"][0].Requisites.Require) != 1 {
		t.Errorf("file state Require length = %d, want 1", len(states["file"][0].Requisites.Require))
	}

	// Check package states
	if len(states["package"]) != 1 {
		t.Errorf("package states length = %d, want 1", len(states["package"]))
	}

	// Check service states
	if len(states["service"]) != 1 {
		t.Errorf("service states length = %d, want 1", len(states["service"]))
	}
	if len(states["service"][0].Requisites.Watch) != 1 {
		t.Errorf("service state Watch length = %d, want 1", len(states["service"][0].Requisites.Watch))
	}

	// metadata and variables should not appear in states
	if _, ok := states["metadata"]; ok {
		t.Error("metadata should not appear in states")
	}
	if _, ok := states["variables"]; ok {
		t.Error("variables should not appear in states")
	}
}
