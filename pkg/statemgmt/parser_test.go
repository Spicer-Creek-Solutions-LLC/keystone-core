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
		if f.Path == mainFile {
			mainState = f
		} else if f.Path == includedFile {
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
		input        string
		wantType     SourceType
		wantPath     string
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
