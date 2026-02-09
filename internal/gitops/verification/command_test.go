package verification

import (
	"testing"
)

func TestCommandVerifierType(t *testing.T) {
	verifier := NewCommandVerifier()
	if verifier.Type() != TypeCommand {
		t.Errorf("Type() = %v, want %v", verifier.Type(), TypeCommand)
	}
}

func TestCommandVerifierSuccess(t *testing.T) {
	verifier := NewCommandVerifier()
	step := &Step{
		Name: "echo-test",
		Type: TypeCommand,
		Config: map[string]interface{}{
			"command":         "echo hello",
			"expected_output": "hello",
		},
	}

	result, err := verifier.Verify(step)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}

	if !result.Success {
		t.Errorf("Command should succeed: %s", result.Message)
	}

	if result.Data["exit_code"] != 0 {
		t.Errorf("Exit code = %v, want 0", result.Data["exit_code"])
	}
}

func TestCommandVerifierFailure(t *testing.T) {
	verifier := NewCommandVerifier()
	step := &Step{
		Name: "failing-command",
		Type: TypeCommand,
		Config: map[string]interface{}{
			"command": "exit 1",
		},
	}

	result, err := verifier.Verify(step)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}

	if result.Success {
		t.Error("Command should fail")
	}

	if result.Data["exit_code"] != 1 {
		t.Errorf("Exit code = %v, want 1", result.Data["exit_code"])
	}
}

func TestCommandVerifierExpectedExitCode(t *testing.T) {
	verifier := NewCommandVerifier()
	step := &Step{
		Name: "expected-failure",
		Type: TypeCommand,
		Config: map[string]interface{}{
			"command":            "exit 42",
			"expected_exit_code": 42,
		},
	}

	result, err := verifier.Verify(step)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}

	if !result.Success {
		t.Errorf("Command should succeed with expected exit code: %s", result.Message)
	}
}

func TestCommandVerifierMissingCommand(t *testing.T) {
	verifier := NewCommandVerifier()
	step := &Step{
		Name:   "no-command",
		Type:   TypeCommand,
		Config: map[string]interface{}{},
	}

	result, err := verifier.Verify(step)
	if err == nil {
		t.Error("Expected error for missing command")
	}

	if result.Success {
		t.Error("Verification should fail for missing command")
	}
}

func TestScriptVerifierType(t *testing.T) {
	verifier := NewScriptVerifier()
	if verifier.Type() != TypeScript {
		t.Errorf("Type() = %v, want %v", verifier.Type(), TypeScript)
	}
}

func TestScriptVerifierSuccess(t *testing.T) {
	verifier := NewScriptVerifier()
	step := &Step{
		Name: "script-test",
		Type: TypeScript,
		Config: map[string]interface{}{
			"script": "echo",
			"args":   []interface{}{"test"},
		},
	}

	result, err := verifier.Verify(step)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}

	if !result.Success {
		t.Errorf("Script should succeed: %s", result.Message)
	}
}

func TestScriptVerifierMissingScript(t *testing.T) {
	verifier := NewScriptVerifier()
	step := &Step{
		Name:   "no-script",
		Type:   TypeScript,
		Config: map[string]interface{}{},
	}

	result, err := verifier.Verify(step)
	if err == nil {
		t.Error("Expected error for missing script")
	}

	if result.Success {
		t.Error("Verification should fail for missing script")
	}
}
