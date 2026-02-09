package statemgmt

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseFile_BasicState(t *testing.T) {
	// Create temporary state file
	tmpDir := t.TempDir()
	stateFile := filepath.Join(tmpDir, "test.yaml")

	content := `
metadata:
  name: Test State
  description: A test state file
  version: "1.0"
  tags:
    - test
    - web

file:
  /etc/nginx/nginx.conf:
    state: present
    source: file://nginx/nginx.conf
    mode: "0644"
    user: root
    group: root

package:
  nginx:
    state: installed
    version: ">=1.20"

service:
  nginx:
    state: running
    enable: true
`

	if err := os.WriteFile(stateFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Parse the file
	parser := NewParser(tmpDir)
	state, err := parser.ParseFile(stateFile)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	// Verify metadata
	if state.Metadata.Name != "Test State" {
		t.Errorf("Metadata.Name = %s, want 'Test State'", state.Metadata.Name)
	}

	if state.Metadata.Description != "A test state file" {
		t.Errorf("Metadata.Description = %s, want 'A test state file'", state.Metadata.Description)
	}

	if len(state.Metadata.Tags) != 2 {
		t.Errorf("len(Metadata.Tags) = %d, want 2", len(state.Metadata.Tags))
	}

	// Verify file state
	if len(state.States["file"]) != 1 {
		t.Fatalf("Expected 1 file state, got %d", len(state.States["file"]))
	}

	fileState := state.States["file"][0]
	if fileState.ID != "/etc/nginx/nginx.conf" {
		t.Errorf("File state ID = %s, want '/etc/nginx/nginx.conf'", fileState.ID)
	}

	if fileState.State != "present" {
		t.Errorf("File state = %s, want 'present'", fileState.State)
	}

	if source, ok := fileState.Parameters["source"].(string); !ok || source != "file://nginx/nginx.conf" {
		t.Errorf("File source = %v, want 'file://nginx/nginx.conf'", fileState.Parameters["source"])
	}

	// Verify package state
	if len(state.States["package"]) != 1 {
		t.Fatalf("Expected 1 package state, got %d", len(state.States["package"]))
	}

	pkgState := state.States["package"][0]
	if pkgState.ID != "nginx" {
		t.Errorf("Package state ID = %s, want 'nginx'", pkgState.ID)
	}

	if pkgState.State != "installed" {
		t.Errorf("Package state = %s, want 'installed'", pkgState.State)
	}

	// Verify service state
	if len(state.States["service"]) != 1 {
		t.Fatalf("Expected 1 service state, got %d", len(state.States["service"]))
	}

	svcState := state.States["service"][0]
	if svcState.ID != "nginx" {
		t.Errorf("Service state ID = %s, want 'nginx'", svcState.ID)
	}

	if svcState.State != "running" {
		t.Errorf("Service state = %s, want 'running'", svcState.State)
	}
}

func TestParseFile_Requisites(t *testing.T) {
	tmpDir := t.TempDir()
	stateFile := filepath.Join(tmpDir, "requisites.yaml")

	content := `
file:
  /etc/nginx/nginx.conf:
    state: present
    source: file://nginx.conf
    require:
      - package: nginx

service:
  nginx:
    state: running
    require:
      - file: /etc/nginx/nginx.conf
    watch:
      - file: /etc/nginx/nginx.conf

package:
  nginx:
    state: installed
`

	if err := os.WriteFile(stateFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	parser := NewParser(tmpDir)
	state, err := parser.ParseFile(stateFile)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	// Check file requisites
	fileState := state.States["file"][0]
	if len(fileState.Requisites.Require) != 1 {
		t.Fatalf("Expected 1 require, got %d", len(fileState.Requisites.Require))
	}

	if fileState.Requisites.Require[0].Module != "package" {
		t.Errorf("Require module = %s, want 'package'", fileState.Requisites.Require[0].Module)
	}

	if fileState.Requisites.Require[0].ID != "nginx" {
		t.Errorf("Require ID = %s, want 'nginx'", fileState.Requisites.Require[0].ID)
	}

	// Check service requisites
	svcState := state.States["service"][0]
	if len(svcState.Requisites.Require) != 1 {
		t.Fatalf("Expected 1 require, got %d", len(svcState.Requisites.Require))
	}

	if len(svcState.Requisites.Watch) != 1 {
		t.Fatalf("Expected 1 watch, got %d", len(svcState.Requisites.Watch))
	}
}

func TestParseFile_RetryConfig(t *testing.T) {
	tmpDir := t.TempDir()
	stateFile := filepath.Join(tmpDir, "retry.yaml")

	content := `
service:
  nginx:
    state: running
    retry:
      attempts: 3
      delay: "5s"
      backoff_multiplier: 2.0
      max_delay: "30s"
`

	if err := os.WriteFile(stateFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	parser := NewParser(tmpDir)
	state, err := parser.ParseFile(stateFile)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	svcState := state.States["service"][0]
	if svcState.Retry == nil {
		t.Fatal("Expected retry config, got nil")
	}

	if svcState.Retry.Attempts != 3 {
		t.Errorf("Retry attempts = %d, want 3", svcState.Retry.Attempts)
	}

	if svcState.Retry.Delay != 5*time.Second {
		t.Errorf("Retry delay = %v, want 5s", svcState.Retry.Delay)
	}

	if svcState.Retry.BackoffMultiplier != 2.0 {
		t.Errorf("Retry backoff = %f, want 2.0", svcState.Retry.BackoffMultiplier)
	}

	if svcState.Retry.MaxDelay != 30*time.Second {
		t.Errorf("Retry max delay = %v, want 30s", svcState.Retry.MaxDelay)
	}
}

func TestParseFile_Includes(t *testing.T) {
	tmpDir := t.TempDir()

	// Create included file
	includedFile := filepath.Join(tmpDir, "included.yaml")
	includedContent := `
package:
  nginx:
    state: installed
`

	if err := os.WriteFile(includedFile, []byte(includedContent), 0644); err != nil {
		t.Fatalf("Failed to write included file: %v", err)
	}

	// Create main file that includes the other
	mainFile := filepath.Join(tmpDir, "main.yaml")
	mainContent := `
include:
  - included.yaml

service:
  nginx:
    state: running
`

	if err := os.WriteFile(mainFile, []byte(mainContent), 0644); err != nil {
		t.Fatalf("Failed to write main file: %v", err)
	}

	// Load with includes
	parser := NewParser(tmpDir)
	files, err := parser.LoadStateFiles([]string{mainFile})
	if err != nil {
		t.Fatalf("LoadStateFiles() error = %v", err)
	}

	if len(files) != 2 {
		t.Fatalf("Expected 2 state files (main + included), got %d", len(files))
	}

	// Verify main file
	var mainState, includedState *StateFile
	for _, f := range files {
		switch f.Path {
		case mainFile:
			mainState = f
		case includedFile:
			includedState = f
		}
	}

	if mainState == nil {
		t.Fatal("Main state file not found")
	}

	if includedState == nil {
		t.Fatal("Included state file not found")
	}

	// Verify included file has package state
	if len(includedState.States["package"]) != 1 {
		t.Errorf("Expected 1 package state in included file, got %d", len(includedState.States["package"]))
	}

	// Verify main file has service state
	if len(mainState.States["service"]) != 1 {
		t.Errorf("Expected 1 service state in main file, got %d", len(mainState.States["service"]))
	}
}

func TestParseFile_Variables(t *testing.T) {
	tmpDir := t.TempDir()
	stateFile := filepath.Join(tmpDir, "variables.yaml")

	content := `
variables:
  nginx_version: "1.20"
  web_user: www-data
  web_port: 80

package:
  nginx:
    state: installed
    version: "{{ nginx_version }}"
`

	if err := os.WriteFile(stateFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	parser := NewParser(tmpDir)
	state, err := parser.ParseFile(stateFile)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	// Verify variables are parsed
	if len(state.Variables) != 3 {
		t.Errorf("Expected 3 variables, got %d", len(state.Variables))
	}

	if nginxVer, ok := state.Variables["nginx_version"].(string); !ok || nginxVer != "1.20" {
		t.Errorf("nginx_version = %v, want '1.20'", state.Variables["nginx_version"])
	}
}

func TestParseSource(t *testing.T) {
	tests := []struct {
		input    string
		wantType SourceType
		wantPath string
	}{
		{
			input:    "file:///etc/nginx/nginx.conf",
			wantType: SourceTypeFile,
			wantPath: "/etc/nginx/nginx.conf",
		},
		{
			input:    "http://example.com/nginx.conf",
			wantType: SourceTypeHTTP,
			wantPath: "http://example.com/nginx.conf",
		},
		{
			input:    "https://example.com/nginx.conf",
			wantType: SourceTypeHTTP,
			wantPath: "https://example.com/nginx.conf",
		},
		{
			input:    "template://nginx.conf.j2",
			wantType: SourceTypeTemplate,
			wantPath: "nginx.conf.j2",
		},
		{
			input:    "/etc/nginx/nginx.conf",
			wantType: SourceTypeFile,
			wantPath: "/etc/nginx/nginx.conf",
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			gotType, gotPath := ParseSource(tt.input)
			if gotType != tt.wantType {
				t.Errorf("ParseSource() type = %v, want %v", gotType, tt.wantType)
			}
			if gotPath != tt.wantPath {
				t.Errorf("ParseSource() path = %v, want %v", gotPath, tt.wantPath)
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

func TestParseFile_BlueprintIncludes(t *testing.T) {
	tmpDir := t.TempDir()
	stateFile := filepath.Join(tmpDir, "with_blueprint.yaml")

	content := `
include:
  - blueprint: blueprints/community/web-stack
    version: "^1.0.0"
    as: web
    entrypoint: production
    features:
      ssl: true
      monitoring: false
    params:
      domain: example.com
      port: 8080
      workers: 4

service:
  myapp:
    state: running
`

	if err := os.WriteFile(stateFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	parser := NewParser(tmpDir)
	state, err := parser.ParseFile(stateFile)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	// Verify blueprint include is parsed
	if len(state.BlueprintIncludes) != 1 {
		t.Fatalf("Expected 1 blueprint include, got %d", len(state.BlueprintIncludes))
	}

	bp := state.BlueprintIncludes[0]

	if bp.Blueprint != "blueprints/community/web-stack" {
		t.Errorf("Blueprint = %s, want 'blueprints/community/web-stack'", bp.Blueprint)
	}

	if bp.Version != "^1.0.0" {
		t.Errorf("Version = %s, want '^1.0.0'", bp.Version)
	}

	if bp.As != "web" {
		t.Errorf("As = %s, want 'web'", bp.As)
	}

	if bp.Entrypoint != "production" {
		t.Errorf("Entrypoint = %s, want 'production'", bp.Entrypoint)
	}

	// Check features
	if len(bp.Features) != 2 {
		t.Fatalf("Expected 2 features, got %d", len(bp.Features))
	}

	if ssl, ok := bp.Features["ssl"]; !ok || !ssl {
		t.Errorf("Feature ssl = %v, want true", bp.Features["ssl"])
	}

	if monitoring, ok := bp.Features["monitoring"]; !ok || monitoring {
		t.Errorf("Feature monitoring = %v, want false", bp.Features["monitoring"])
	}

	// Check parameters
	if len(bp.Parameters) != 3 {
		t.Fatalf("Expected 3 parameters, got %d", len(bp.Parameters))
	}

	if domain, ok := bp.Parameters["domain"].(string); !ok || domain != "example.com" {
		t.Errorf("Parameter domain = %v, want 'example.com'", bp.Parameters["domain"])
	}

	if port, ok := bp.Parameters["port"].(int); !ok || port != 8080 {
		t.Errorf("Parameter port = %v, want 8080", bp.Parameters["port"])
	}

	// Verify regular states still work
	if len(state.States["service"]) != 1 {
		t.Errorf("Expected 1 service state, got %d", len(state.States["service"]))
	}
}

func TestParseFile_MixedIncludes(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a simple include file
	includeFile := filepath.Join(tmpDir, "base.yaml")
	includeContent := `
package:
  nginx:
    state: installed
`
	if err := os.WriteFile(includeFile, []byte(includeContent), 0644); err != nil {
		t.Fatalf("Failed to write include file: %v", err)
	}

	// Create main file with both file and blueprint includes
	mainFile := filepath.Join(tmpDir, "main.yaml")
	mainContent := `
include:
  - base.yaml
  - blueprint: blueprints/myorg/database
    params:
      engine: postgres
  - file: another.yaml

service:
  app:
    state: running
`

	if err := os.WriteFile(mainFile, []byte(mainContent), 0644); err != nil {
		t.Fatalf("Failed to write main file: %v", err)
	}

	parser := NewParser(tmpDir)
	state, err := parser.ParseFile(mainFile)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	// Should have 2 file includes (string and file: key)
	if len(state.Includes) != 2 {
		t.Errorf("Expected 2 file includes, got %d", len(state.Includes))
	}

	// Should have 1 blueprint include
	if len(state.BlueprintIncludes) != 1 {
		t.Errorf("Expected 1 blueprint include, got %d", len(state.BlueprintIncludes))
	}

	if state.BlueprintIncludes[0].Blueprint != "blueprints/myorg/database" {
		t.Errorf("Blueprint = %s, want 'blueprints/myorg/database'", state.BlueprintIncludes[0].Blueprint)
	}
}

func TestParseBlueprintInclude_Basic(t *testing.T) {
	data := map[string]interface{}{
		"blueprint": "blueprints/test/example",
	}

	include := parseBlueprintInclude("blueprints/test/example", data)

	if include.Blueprint != "blueprints/test/example" {
		t.Errorf("Blueprint = %s, want 'blueprints/test/example'", include.Blueprint)
	}

	if include.Version != "" {
		t.Errorf("Version = %s, want empty", include.Version)
	}
}

func TestParseBlueprintInclude_AllFields(t *testing.T) {
	data := map[string]interface{}{
		"blueprint":  "blueprints/vendor/stack",
		"version":    ">=2.0.0",
		"as":         "mystack",
		"entrypoint": "custom",
		"features": map[string]interface{}{
			"feature1": true,
			"feature2": false,
		},
		"params": map[string]interface{}{
			"key1": "value1",
			"key2": 123,
		},
	}

	include := parseBlueprintInclude("blueprints/vendor/stack", data)

	if include.Blueprint != "blueprints/vendor/stack" {
		t.Errorf("Blueprint = %s, want 'blueprints/vendor/stack'", include.Blueprint)
	}

	if include.Version != ">=2.0.0" {
		t.Errorf("Version = %s, want '>=2.0.0'", include.Version)
	}

	if include.As != "mystack" {
		t.Errorf("As = %s, want 'mystack'", include.As)
	}

	if include.Entrypoint != "custom" {
		t.Errorf("Entrypoint = %s, want 'custom'", include.Entrypoint)
	}

	if len(include.Features) != 2 {
		t.Fatalf("Expected 2 features, got %d", len(include.Features))
	}

	if !include.Features["feature1"] {
		t.Error("feature1 should be true")
	}

	if include.Features["feature2"] {
		t.Error("feature2 should be false")
	}

	if len(include.Parameters) != 2 {
		t.Fatalf("Expected 2 parameters, got %d", len(include.Parameters))
	}

	if include.Parameters["key1"] != "value1" {
		t.Errorf("key1 = %v, want 'value1'", include.Parameters["key1"])
	}
}

func TestParseBlueprintInclude_ParametersKey(t *testing.T) {
	// Test with "parameters" key instead of "params"
	data := map[string]interface{}{
		"blueprint": "blueprints/test/example",
		"parameters": map[string]interface{}{
			"param1": "value1",
		},
	}

	include := parseBlueprintInclude("blueprints/test/example", data)

	if len(include.Parameters) != 1 {
		t.Fatalf("Expected 1 parameter, got %d", len(include.Parameters))
	}

	if include.Parameters["param1"] != "value1" {
		t.Errorf("param1 = %v, want 'value1'", include.Parameters["param1"])
	}
}

func TestBlueprintInclude_IsBlueprint(t *testing.T) {
	include := BlueprintInclude{
		Blueprint: "blueprints/test/example",
	}

	if !include.IsBlueprint() {
		t.Error("IsBlueprint() should return true")
	}
}

func TestParseFile_AllRequisiteTypes(t *testing.T) {
	tmpDir := t.TempDir()
	stateFile := filepath.Join(tmpDir, "all_requisites.yaml")

	content := `
file:
  /etc/app/config:
    state: present
    require:
      - package: app-pkg
    require_in:
      - service: app
    watch:
      - file: /etc/app/settings
    watch_in:
      - service: app
    prereq:
      - cmd: check-deps
    prereq_in:
      - file: /etc/app/ready
    onchanges:
      - file: /etc/app/source
    onchanges_in:
      - cmd: notify-change

package:
  app-pkg:
    state: installed

service:
  app:
    state: running

cmd:
  check-deps:
    state: run
  notify-change:
    state: run
`

	if err := os.WriteFile(stateFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	parser := NewParser(tmpDir)
	state, err := parser.ParseFile(stateFile)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	fileState := state.States["file"][0]

	// Check all requisite types
	tests := []struct {
		name    string
		reqs    []StateReference
		wantLen int
		wantMod string
		wantID  string
	}{
		{"Require", fileState.Requisites.Require, 1, "package", "app-pkg"},
		{"RequireIn", fileState.Requisites.RequireIn, 1, "service", "app"},
		{"Watch", fileState.Requisites.Watch, 1, "file", "/etc/app/settings"},
		{"WatchIn", fileState.Requisites.WatchIn, 1, "service", "app"},
		{"Prereq", fileState.Requisites.Prereq, 1, "cmd", "check-deps"},
		{"PrereqIn", fileState.Requisites.PrereqIn, 1, "file", "/etc/app/ready"},
		{"Onchanges", fileState.Requisites.Onchanges, 1, "file", "/etc/app/source"},
		{"OnchangesIn", fileState.Requisites.OnchangesIn, 1, "cmd", "notify-change"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if len(tt.reqs) != tt.wantLen {
				t.Errorf("len(%s) = %d, want %d", tt.name, len(tt.reqs), tt.wantLen)
				return
			}
			if tt.reqs[0].Module != tt.wantMod {
				t.Errorf("%s[0].Module = %s, want %s", tt.name, tt.reqs[0].Module, tt.wantMod)
			}
			if tt.reqs[0].ID != tt.wantID {
				t.Errorf("%s[0].ID = %s, want %s", tt.name, tt.reqs[0].ID, tt.wantID)
			}
		})
	}
}

func TestParseFile_StateDeclarationFields(t *testing.T) {
	tmpDir := t.TempDir()
	stateFile := filepath.Join(tmpDir, "decl_fields.yaml")

	content := `
file:
  /etc/app/config:
    state: present
    order: 10
    fail_hard: true
    unless: "test -f /etc/app/skip"
    only_if: "test -d /etc/app"
    mode: "0644"
    user: root
`

	if err := os.WriteFile(stateFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	parser := NewParser(tmpDir)
	state, err := parser.ParseFile(stateFile)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	fileState := state.States["file"][0]

	if fileState.Order != 10 {
		t.Errorf("Order = %d, want 10", fileState.Order)
	}

	if !fileState.FailHard {
		t.Error("FailHard should be true")
	}

	if fileState.Unless != "test -f /etc/app/skip" {
		t.Errorf("Unless = %s, want 'test -f /etc/app/skip'", fileState.Unless)
	}

	if fileState.OnlyIf != "test -d /etc/app" {
		t.Errorf("OnlyIf = %s, want 'test -d /etc/app'", fileState.OnlyIf)
	}

	// Verify parameters are still parsed
	if mode, ok := fileState.Parameters["mode"].(string); !ok || mode != "0644" {
		t.Errorf("Parameter mode = %v, want '0644'", fileState.Parameters["mode"])
	}
}

func TestIsRequisiteField(t *testing.T) {
	tests := []struct {
		field string
		want  bool
	}{
		{"require", true},
		{"require_in", true},
		{"watch", true},
		{"watch_in", true},
		{"prereq", true},
		{"prereq_in", true},
		{"onchanges", true},
		{"onchanges_in", true},
		{"state", false},
		{"mode", false},
		{"user", false},
		{"source", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			got := isRequisiteField(tt.field)
			if got != tt.want {
				t.Errorf("isRequisiteField(%q) = %v, want %v", tt.field, got, tt.want)
			}
		})
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    time.Duration
		wantErr bool
	}{
		{"seconds", "5s", 5 * time.Second, false},
		{"minutes", "10m", 10 * time.Minute, false},
		{"hours", "2h", 2 * time.Hour, false},
		{"milliseconds", "500ms", 500 * time.Millisecond, false},
		{"complex", "1h30m", 90 * time.Minute, false},
		{"with_spaces", "  10s  ", 10 * time.Second, false},
		{"empty", "", 0, true},
		{"spaces_only", "   ", 0, true},
		{"invalid", "abc", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDuration(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseDuration(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("parseDuration(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseStateReferences_InvalidRef(t *testing.T) {
	// Test that non-map references are skipped
	refs := []interface{}{
		map[string]interface{}{"package": "nginx"},
		"invalid-string-ref",
		123,
		nil,
		map[string]interface{}{"service": "app"},
	}

	result := parseStateReferences(refs)

	// Should only have 2 valid references
	if len(result) != 2 {
		t.Errorf("Expected 2 valid references, got %d", len(result))
	}

	if result[0].Module != "package" || result[0].ID != "nginx" {
		t.Errorf("result[0] = %+v, want package:nginx", result[0])
	}

	if result[1].Module != "service" || result[1].ID != "app" {
		t.Errorf("result[1] = %+v, want service:app", result[1])
	}
}

func TestParseStateReferences_NonStringID(t *testing.T) {
	// Test that non-string IDs are skipped
	refs := []interface{}{
		map[string]interface{}{"package": "nginx"},
		map[string]interface{}{"service": 123}, // non-string ID should be skipped
		map[string]interface{}{"file": "/etc/config"},
	}

	result := parseStateReferences(refs)

	// Should only have 2 valid references (service:123 skipped)
	if len(result) != 2 {
		t.Errorf("Expected 2 valid references, got %d", len(result))
	}
}

func TestParseMetadata_AllFields(t *testing.T) {
	raw := map[string]interface{}{
		"name":        "Test State",
		"description": "A test description",
		"version":     "1.2.3",
		"tags":        []interface{}{"tag1", "tag2", 123}, // include non-string
	}

	metadata := parseMetadata(raw)

	if metadata.Name != "Test State" {
		t.Errorf("Name = %s, want 'Test State'", metadata.Name)
	}

	if metadata.Description != "A test description" {
		t.Errorf("Description = %s, want 'A test description'", metadata.Description)
	}

	if metadata.Version != "1.2.3" {
		t.Errorf("Version = %s, want '1.2.3'", metadata.Version)
	}

	// Should only have 2 string tags (123 is int, skipped)
	if len(metadata.Tags) != 2 {
		t.Errorf("len(Tags) = %d, want 2", len(metadata.Tags))
	}
}

func TestParseMetadata_Empty(t *testing.T) {
	raw := map[string]interface{}{}

	metadata := parseMetadata(raw)

	if metadata.Name != "" {
		t.Errorf("Name = %s, want empty", metadata.Name)
	}

	if metadata.Description != "" {
		t.Errorf("Description = %s, want empty", metadata.Description)
	}

	if len(metadata.Tags) != 0 {
		t.Errorf("len(Tags) = %d, want 0", len(metadata.Tags))
	}
}

func TestParseFile_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	stateFile := filepath.Join(tmpDir, "invalid.yaml")

	content := `
file:
  /etc/config:
    state: present
    - invalid yaml structure
`

	if err := os.WriteFile(stateFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	parser := NewParser(tmpDir)
	_, err := parser.ParseFile(stateFile)
	if err == nil {
		t.Fatal("Expected error parsing invalid YAML")
	}
}

func TestParseFile_InvalidStateDeclaration(t *testing.T) {
	tmpDir := t.TempDir()
	stateFile := filepath.Join(tmpDir, "invalid_decl.yaml")

	// Module declarations should be maps, not a list
	content := `
file:
  - /etc/config
`

	if err := os.WriteFile(stateFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	parser := NewParser(tmpDir)
	_, err := parser.ParseFile(stateFile)
	if err == nil {
		t.Fatal("Expected error parsing invalid state declarations")
	}
}

func TestParseFile_InvalidStateParams(t *testing.T) {
	tmpDir := t.TempDir()
	stateFile := filepath.Join(tmpDir, "invalid_params.yaml")

	// State params should be a map, not a string
	content := `
file:
  /etc/config: "not a map"
`

	if err := os.WriteFile(stateFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	parser := NewParser(tmpDir)
	_, err := parser.ParseFile(stateFile)
	if err == nil {
		t.Fatal("Expected error parsing invalid state params")
	}
}

func TestParseFile_FileNotFound(t *testing.T) {
	parser := NewParser("/tmp")
	_, err := parser.ParseFile("/nonexistent/path/file.yaml")
	if err == nil {
		t.Fatal("Expected error for nonexistent file")
	}
}

func TestParseFile_NilModule(t *testing.T) {
	tmpDir := t.TempDir()
	stateFile := filepath.Join(tmpDir, "nil_module.yaml")

	content := `
file:
package:
`

	if err := os.WriteFile(stateFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	parser := NewParser(tmpDir)
	state, err := parser.ParseFile(stateFile)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	// Both modules should be empty (nil declarations are skipped)
	if len(state.States["file"]) != 0 {
		t.Errorf("len(States['file']) = %d, want 0", len(state.States["file"]))
	}
	if len(state.States["package"]) != 0 {
		t.Errorf("len(States['package']) = %d, want 0", len(state.States["package"]))
	}
}

func TestParseRetry_PartialConfig(t *testing.T) {
	// Test retry config with only some fields
	retry := map[string]interface{}{
		"attempts": 5,
	}

	config := parseRetry(retry)

	if config.Attempts != 5 {
		t.Errorf("Attempts = %d, want 5", config.Attempts)
	}

	// Delay and MaxDelay should be zero since not provided
	if config.Delay != 0 {
		t.Errorf("Delay = %v, want 0", config.Delay)
	}

	if config.MaxDelay != 0 {
		t.Errorf("MaxDelay = %v, want 0", config.MaxDelay)
	}

	if config.BackoffMultiplier != 0 {
		t.Errorf("BackoffMultiplier = %f, want 0", config.BackoffMultiplier)
	}
}

func TestParseRetry_InvalidDuration(t *testing.T) {
	// Test retry config with invalid duration string
	retry := map[string]interface{}{
		"attempts":  3,
		"delay":     "invalid",
		"max_delay": "also_invalid",
	}

	config := parseRetry(retry)

	if config.Attempts != 3 {
		t.Errorf("Attempts = %d, want 3", config.Attempts)
	}

	// Invalid durations should result in zero
	if config.Delay != 0 {
		t.Errorf("Delay = %v, want 0 (invalid duration)", config.Delay)
	}

	if config.MaxDelay != 0 {
		t.Errorf("MaxDelay = %v, want 0 (invalid duration)", config.MaxDelay)
	}
}

func TestLoadStateFiles_CircularInclude(t *testing.T) {
	tmpDir := t.TempDir()

	// Create two files that include each other
	file1 := filepath.Join(tmpDir, "file1.yaml")
	file2 := filepath.Join(tmpDir, "file2.yaml")

	content1 := `
include:
  - file2.yaml
package:
  nginx:
    state: installed
`

	content2 := `
include:
  - file1.yaml
service:
  nginx:
    state: running
`

	if err := os.WriteFile(file1, []byte(content1), 0644); err != nil {
		t.Fatalf("Failed to write file1: %v", err)
	}
	if err := os.WriteFile(file2, []byte(content2), 0644); err != nil {
		t.Fatalf("Failed to write file2: %v", err)
	}

	parser := NewParser(tmpDir)
	files, err := parser.LoadStateFiles([]string{file1})
	if err != nil {
		t.Fatalf("LoadStateFiles() error = %v", err)
	}

	// Should have both files loaded (circular detected, not an error)
	if len(files) != 2 {
		t.Errorf("Expected 2 files (circular include handled), got %d", len(files))
	}
}
