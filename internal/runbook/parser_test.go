package runbook

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParser_ParseBytes_ValidRunbook(t *testing.T) {
	yaml := `
apiVersion: runbook.keystone.io/v1
kind: Runbook
metadata:
  name: test-runbook
  version: "1.0.0"
spec:
  description: A test runbook
  inputs:
    - name: target
      type: string
      required: true
    - name: retries
      type: int
      default: 3
  steps:
    - name: check-health
      type: command
      config:
        command: "curl -s http://localhost/health"
      outputs:
        - name: status
          source: stdout
          parser: json
          path: status
    - name: notify
      type: notification
      dependsOn: [check-health]
      config:
        channel: slack
        message: "Health check completed"
`

	p := NewParser()
	rb, err := p.ParseBytes([]byte(yaml))
	if err != nil {
		t.Fatalf("ParseBytes() error = %v", err)
	}

	if rb.Metadata.Name != "test-runbook" {
		t.Errorf("Metadata.Name = %q, want %q", rb.Metadata.Name, "test-runbook")
	}
	if rb.Metadata.Version != "1.0.0" {
		t.Errorf("Metadata.Version = %q, want %q", rb.Metadata.Version, "1.0.0")
	}
	if rb.Spec.Description != "A test runbook" {
		t.Errorf("Spec.Description = %q, want %q", rb.Spec.Description, "A test runbook")
	}
	if len(rb.Spec.Inputs) != 2 {
		t.Errorf("len(Spec.Inputs) = %d, want 2", len(rb.Spec.Inputs))
	}
	if len(rb.Spec.Steps) != 2 {
		t.Errorf("len(Spec.Steps) = %d, want 2", len(rb.Spec.Steps))
	}

	// Check first input
	if rb.Spec.Inputs[0].Name != "target" {
		t.Errorf("Inputs[0].Name = %q, want %q", rb.Spec.Inputs[0].Name, "target")
	}
	if rb.Spec.Inputs[0].Type != InputTypeString {
		t.Errorf("Inputs[0].Type = %q, want %q", rb.Spec.Inputs[0].Type, InputTypeString)
	}
	if !rb.Spec.Inputs[0].Required {
		t.Error("Inputs[0].Required = false, want true")
	}

	// Check second input default value
	if rb.Spec.Inputs[1].Default != 3 {
		t.Errorf("Inputs[1].Default = %v, want 3", rb.Spec.Inputs[1].Default)
	}

	// Check first step
	if rb.Spec.Steps[0].Name != "check-health" {
		t.Errorf("Steps[0].Name = %q, want %q", rb.Spec.Steps[0].Name, "check-health")
	}
	if rb.Spec.Steps[0].Type != StepTypeCommand {
		t.Errorf("Steps[0].Type = %q, want %q", rb.Spec.Steps[0].Type, StepTypeCommand)
	}

	// Check second step dependencies
	if len(rb.Spec.Steps[1].DependsOn) != 1 || rb.Spec.Steps[1].DependsOn[0] != "check-health" {
		t.Errorf("Steps[1].DependsOn = %v, want [check-health]", rb.Spec.Steps[1].DependsOn)
	}
}

func TestParser_ParseBytes_MinimalRunbook(t *testing.T) {
	yaml := `
kind: Runbook
metadata:
  name: minimal
spec:
  steps:
    - name: step1
      type: noop
      config: {}
`

	p := NewParser()
	rb, err := p.ParseBytes([]byte(yaml))
	if err != nil {
		t.Fatalf("ParseBytes() error = %v", err)
	}

	// Check defaults are applied
	if rb.APIVersion != APIVersion {
		t.Errorf("APIVersion = %q, want %q", rb.APIVersion, APIVersion)
	}
	if rb.Kind != Kind {
		t.Errorf("Kind = %q, want %q", rb.Kind, Kind)
	}
}

func TestParser_ParseBytes_InvalidKind(t *testing.T) {
	yaml := `
apiVersion: runbook.keystone.io/v1
kind: InvalidKind
metadata:
  name: test
spec:
  steps: []
`

	p := NewParser()
	_, err := p.ParseBytes([]byte(yaml))
	if err == nil {
		t.Error("ParseBytes() expected error for invalid kind, got nil")
	}
	if !strings.Contains(err.Error(), "invalid kind") {
		t.Errorf("error = %q, want to contain 'invalid kind'", err.Error())
	}
}

func TestParser_ParseBytes_InvalidAPIVersion(t *testing.T) {
	yaml := `
apiVersion: runbook.keystone.io/v99
kind: Runbook
metadata:
  name: test
spec:
  steps: []
`

	p := NewParser()
	_, err := p.ParseBytes([]byte(yaml))
	if err == nil {
		t.Error("ParseBytes() expected error for invalid API version, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported API version") {
		t.Errorf("error = %q, want to contain 'unsupported API version'", err.Error())
	}
}

func TestParser_ParseBytes_InvalidYAML(t *testing.T) {
	yaml := `
this is not valid yaml: [
`

	p := NewParser()
	_, err := p.ParseBytes([]byte(yaml))
	if err == nil {
		t.Error("ParseBytes() expected error for invalid YAML, got nil")
	}
}

func TestParser_ParseBytes_AppliesDefaults(t *testing.T) {
	yaml := `
kind: Runbook
metadata:
  name: defaults-test
spec:
  inputs:
    - name: input1
  steps:
    - name: step1
      type: command
      config:
        command: "echo hello"
      retries:
        maxAttempts: 0
      outputs:
        - name: out1
          source: stdout
`

	p := NewParser()
	rb, err := p.ParseBytes([]byte(yaml))
	if err != nil {
		t.Fatalf("ParseBytes() error = %v", err)
	}

	// Check input type default
	if rb.Spec.Inputs[0].Type != InputTypeString {
		t.Errorf("Input type default = %q, want %q", rb.Spec.Inputs[0].Type, InputTypeString)
	}

	// Check retry defaults
	if rb.Spec.Steps[0].Retries.Backoff != BackoffConstant {
		t.Errorf("Retry backoff default = %q, want %q", rb.Spec.Steps[0].Retries.Backoff, BackoffConstant)
	}
	if rb.Spec.Steps[0].Retries.MaxAttempts != 1 {
		t.Errorf("Retry maxAttempts default = %d, want 1", rb.Spec.Steps[0].Retries.MaxAttempts)
	}

	// Check output parser default
	if rb.Spec.Steps[0].Outputs[0].Parser != OutputParserRaw {
		t.Errorf("Output parser default = %q, want %q", rb.Spec.Steps[0].Outputs[0].Parser, OutputParserRaw)
	}
}

func TestParser_ParseFile(t *testing.T) {
	yaml := `
apiVersion: runbook.keystone.io/v1
kind: Runbook
metadata:
  name: file-test
spec:
  steps:
    - name: step1
      type: noop
      config: {}
`

	// Create temp file
	dir := t.TempDir()
	path := filepath.Join(dir, "test.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	p := NewParser()
	rb, err := p.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	if rb.Metadata.Name != "file-test" {
		t.Errorf("Metadata.Name = %q, want %q", rb.Metadata.Name, "file-test")
	}
}

func TestParser_ParseFile_NotFound(t *testing.T) {
	p := NewParser()
	_, err := p.ParseFile("/nonexistent/path/runbook.yaml")
	if err == nil {
		t.Error("ParseFile() expected error for nonexistent file, got nil")
	}
}

func TestParser_Parse_Reader(t *testing.T) {
	yaml := `
apiVersion: runbook.keystone.io/v1
kind: Runbook
metadata:
  name: reader-test
spec:
  steps:
    - name: step1
      type: noop
      config: {}
`

	p := NewParser()
	rb, err := p.Parse(strings.NewReader(yaml))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if rb.Metadata.Name != "reader-test" {
		t.Errorf("Metadata.Name = %q, want %q", rb.Metadata.Name, "reader-test")
	}
}

func TestParseString(t *testing.T) {
	yaml := `
kind: Runbook
metadata:
  name: string-test
spec:
  steps:
    - name: step1
      type: noop
      config: {}
`

	rb, err := ParseString(yaml)
	if err != nil {
		t.Fatalf("ParseString() error = %v", err)
	}

	if rb.Metadata.Name != "string-test" {
		t.Errorf("Metadata.Name = %q, want %q", rb.Metadata.Name, "string-test")
	}
}

func TestToYAML(t *testing.T) {
	rb := &Runbook{
		APIVersion: APIVersion,
		Kind:       Kind,
		Metadata: Metadata{
			Name:    "yaml-test",
			Version: "1.0.0",
		},
		Spec: RunbookSpec{
			Description: "Test runbook",
			Steps: []Step{
				{
					Name: "step1",
					Type: StepTypeNoop,
					Config: map[string]interface{}{
						"key": "value",
					},
				},
			},
		},
	}

	data, err := ToYAML(rb)
	if err != nil {
		t.Fatalf("ToYAML() error = %v", err)
	}

	// Parse it back
	rb2, err := ParseBytes(data)
	if err != nil {
		t.Fatalf("ParseBytes() error = %v", err)
	}

	if rb2.Metadata.Name != rb.Metadata.Name {
		t.Errorf("Round-trip Name = %q, want %q", rb2.Metadata.Name, rb.Metadata.Name)
	}
	if rb2.Spec.Description != rb.Spec.Description {
		t.Errorf("Round-trip Description = %q, want %q", rb2.Spec.Description, rb.Spec.Description)
	}
}

func TestParser_ParseBytes_WithConditions(t *testing.T) {
	yaml := `
kind: Runbook
metadata:
  name: condition-test
spec:
  steps:
    - name: step1
      type: command
      config:
        command: "echo hello"
    - name: step2
      type: command
      dependsOn: [step1]
      condition: '{{ eq .steps.step1.outputs.status "success" }}'
      config:
        command: "echo success"
`

	rb, err := ParseString(yaml)
	if err != nil {
		t.Fatalf("ParseString() error = %v", err)
	}

	if rb.Spec.Steps[1].Condition == "" {
		t.Error("Step condition should not be empty")
	}
}

func TestParser_ParseBytes_WithRetries(t *testing.T) {
	yaml := `
kind: Runbook
metadata:
  name: retry-test
spec:
  steps:
    - name: step1
      type: api
      config:
        url: "http://example.com"
      retries:
        maxAttempts: 3
        delay: "5s"
        maxDelay: "30s"
        backoff: exponential
        retryOn:
          - "503"
          - "timeout"
`

	rb, err := ParseString(yaml)
	if err != nil {
		t.Fatalf("ParseString() error = %v", err)
	}

	retries := rb.Spec.Steps[0].Retries
	if retries == nil {
		t.Fatal("Retries should not be nil")
	}
	if retries.MaxAttempts != 3 {
		t.Errorf("MaxAttempts = %d, want 3", retries.MaxAttempts)
	}
	if retries.Delay != "5s" {
		t.Errorf("Delay = %q, want %q", retries.Delay, "5s")
	}
	if retries.MaxDelay != "30s" {
		t.Errorf("MaxDelay = %q, want %q", retries.MaxDelay, "30s")
	}
	if retries.Backoff != BackoffExponential {
		t.Errorf("Backoff = %q, want %q", retries.Backoff, BackoffExponential)
	}
	if len(retries.RetryOn) != 2 {
		t.Errorf("len(RetryOn) = %d, want 2", len(retries.RetryOn))
	}
}

func TestParser_ParseBytes_WithHandlers(t *testing.T) {
	yaml := `
kind: Runbook
metadata:
  name: handlers-test
spec:
  steps:
    - name: main
      type: command
      config:
        command: "echo main"
  onSuccess:
    - name: success-notify
      type: notification
      config:
        channel: slack
        message: "Success!"
  onFailure:
    - name: failure-notify
      type: notification
      config:
        channel: pagerduty
        message: "Failed!"
`

	rb, err := ParseString(yaml)
	if err != nil {
		t.Fatalf("ParseString() error = %v", err)
	}

	if len(rb.Spec.OnSuccess) != 1 {
		t.Errorf("len(OnSuccess) = %d, want 1", len(rb.Spec.OnSuccess))
	}
	if len(rb.Spec.OnFailure) != 1 {
		t.Errorf("len(OnFailure) = %d, want 1", len(rb.Spec.OnFailure))
	}
}

func TestParser_ParseBytes_WithTimeout(t *testing.T) {
	yaml := `
kind: Runbook
metadata:
  name: timeout-test
spec:
  timeout: "1h"
  maxRetries: 2
  steps:
    - name: step1
      type: command
      timeout: "5m"
      config:
        command: "long-running-command"
`

	rb, err := ParseString(yaml)
	if err != nil {
		t.Fatalf("ParseString() error = %v", err)
	}

	if rb.Spec.Timeout != "1h" {
		t.Errorf("Spec.Timeout = %q, want %q", rb.Spec.Timeout, "1h")
	}
	if rb.Spec.MaxRetries != 2 {
		t.Errorf("Spec.MaxRetries = %d, want 2", rb.Spec.MaxRetries)
	}
	if rb.Spec.Steps[0].Timeout != "5m" {
		t.Errorf("Steps[0].Timeout = %q, want %q", rb.Spec.Steps[0].Timeout, "5m")
	}
}
