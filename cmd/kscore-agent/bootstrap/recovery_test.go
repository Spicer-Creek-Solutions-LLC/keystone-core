package bootstrap

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultRecoveryConfig(t *testing.T) {
	cfg := DefaultRecoveryConfig()

	if !cfg.EnableAutoRecovery {
		t.Error("EnableAutoRecovery should be true by default")
	}

	if cfg.MaxAutoRecoveryAttempts != 2 {
		t.Errorf("MaxAutoRecoveryAttempts = %d, want 2", cfg.MaxAutoRecoveryAttempts)
	}

	if !cfg.EnablePreflightChecks {
		t.Error("EnablePreflightChecks should be true by default")
	}

	if !cfg.GenerateRecoveryScript {
		t.Error("GenerateRecoveryScript should be true by default")
	}
}

func TestNewRecoveryManager(t *testing.T) {
	cfg := DefaultRecoveryConfig()
	output := &bytes.Buffer{}
	mgr := NewRecoveryManager(cfg, output, true)

	if mgr == nil {
		t.Fatal("NewRecoveryManager returned nil")
	}

	if mgr.output != output {
		t.Error("output writer not set correctly")
	}

	if !mgr.verbose {
		t.Error("verbose flag not set correctly")
	}
}

func TestRecoveryManager_AttemptAutomaticRecovery_Disabled(t *testing.T) {
	cfg := RecoveryConfig{
		EnableAutoRecovery: false,
	}
	output := &bytes.Buffer{}
	mgr := NewRecoveryManager(cfg, output, false)

	bErr := &Error{
		Category: ErrorCategoryNetwork,
		RecoveryActions: []RecoveryAction{
			{ID: "test", Type: RecoveryTypeAutomatic},
		},
	}

	results := mgr.AttemptAutomaticRecovery(context.Background(), bErr)
	if results != nil {
		t.Error("Expected nil results when auto recovery disabled")
	}
}

func TestRecoveryManager_AttemptAutomaticRecovery_NoActions(t *testing.T) {
	cfg := DefaultRecoveryConfig()
	output := &bytes.Buffer{}
	mgr := NewRecoveryManager(cfg, output, false)

	bErr := &Error{
		Category: ErrorCategoryNetwork,
		RecoveryActions: []RecoveryAction{
			{ID: "manual", Type: RecoveryTypeManual},
		},
	}

	results := mgr.AttemptAutomaticRecovery(context.Background(), bErr)
	if results != nil {
		t.Error("Expected nil results when no automatic actions")
	}
}

func TestRecoveryManager_AttemptAutomaticRecovery_WithAction(t *testing.T) {
	cfg := DefaultRecoveryConfig()
	output := &bytes.Buffer{}
	mgr := NewRecoveryManager(cfg, output, true)

	bErr := &Error{
		Category: ErrorCategoryNetwork,
		RecoveryActions: []RecoveryAction{
			{
				ID:          "test-action",
				Description: "Test recovery",
				Type:        RecoveryTypeAutomatic,
				Command:     "echo 'test'",
			},
		},
	}

	results := mgr.AttemptAutomaticRecovery(context.Background(), bErr)
	if len(results) == 0 {
		t.Fatal("Expected at least one result")
	}

	if !results[0].Success {
		t.Errorf("Expected success, got: %s", results[0].Message)
	}

	if results[0].ActionID != "test-action" {
		t.Errorf("ActionID = %s, want test-action", results[0].ActionID)
	}
}

func TestRecoveryManager_AttemptAutomaticRecovery_NoCommands(t *testing.T) {
	cfg := DefaultRecoveryConfig()
	output := &bytes.Buffer{}
	mgr := NewRecoveryManager(cfg, output, false)

	bErr := &Error{
		Category: ErrorCategoryTimeout,
		RecoveryActions: []RecoveryAction{
			{
				ID:          "no-cmd",
				Type:        RecoveryTypeAutomatic,
				Description: "Action with no commands",
			},
		},
	}

	results := mgr.AttemptAutomaticRecovery(context.Background(), bErr)
	if len(results) == 0 {
		t.Fatal("Expected result")
	}

	if !results[0].Success {
		t.Error("Expected success for action with no commands")
	}

	if results[0].Message != "no commands to execute" {
		t.Errorf("Message = %s, want 'no commands to execute'", results[0].Message)
	}
}

func TestRecoveryManager_AttemptAutomaticRecovery_SkipsComments(t *testing.T) {
	cfg := DefaultRecoveryConfig()
	output := &bytes.Buffer{}
	mgr := NewRecoveryManager(cfg, output, false)

	bErr := &Error{
		Category: ErrorCategoryNetwork,
		RecoveryActions: []RecoveryAction{
			{
				ID:   "with-comments",
				Type: RecoveryTypeAutomatic,
				Commands: []string{
					"# This is a comment",
					"echo 'actual command'",
				},
			},
		},
	}

	results := mgr.AttemptAutomaticRecovery(context.Background(), bErr)
	if len(results) == 0 {
		t.Fatal("Expected result")
	}

	if !results[0].Success {
		t.Error("Expected success")
	}
}

func TestRecoveryManager_AttemptAutomaticRecovery_SkipsPlaceholders(t *testing.T) {
	cfg := DefaultRecoveryConfig()
	output := &bytes.Buffer{}
	mgr := NewRecoveryManager(cfg, output, false)

	bErr := &Error{
		Category: ErrorCategoryDatabase,
		RecoveryActions: []RecoveryAction{
			{
				ID:   "with-placeholders",
				Type: RecoveryTypeAutomatic,
				Commands: []string{
					"psql -h <host> -U <user>",
					"echo 'done'",
				},
			},
		},
	}

	results := mgr.AttemptAutomaticRecovery(context.Background(), bErr)
	if len(results) == 0 {
		t.Fatal("Expected result")
	}

	if !results[0].Success {
		t.Error("Expected success (placeholder commands should be skipped)")
	}
}

func TestRecoveryManager_GenerateRecoveryScript_Disabled(t *testing.T) {
	cfg := RecoveryConfig{
		GenerateRecoveryScript: false,
	}
	mgr := NewRecoveryManager(cfg, nil, false)

	bErr := &Error{
		Category: ErrorCategoryNetwork,
	}

	path, err := mgr.GenerateRecoveryScript(bErr)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if path != "" {
		t.Error("Expected empty path when script generation disabled")
	}
}

func TestRecoveryManager_GenerateRecoveryScript(t *testing.T) {
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "recovery", "recovery.sh")

	cfg := RecoveryConfig{
		GenerateRecoveryScript: true,
		RecoveryScriptPath:     scriptPath,
	}
	mgr := NewRecoveryManager(cfg, nil, false)

	bErr := &Error{
		Message:  "test error",
		Category: ErrorCategoryNetwork,
		RecoveryActions: []RecoveryAction{
			{
				ID:              "check-network",
				Description:     "Check network connectivity",
				Type:            RecoveryTypeManual,
				Risk:            RiskLow,
				Command:         "ping -c 1 8.8.8.8",
				Precondition:    "Network interface is up",
				ExpectedOutcome: "Ping succeeds",
			},
			{
				ID:          "auto-retry",
				Description: "Automatic retry",
				Type:        RecoveryTypeAutomatic,
				Commands: []string{
					"echo 'retrying'",
					"# comment line",
				},
			},
		},
	}

	path, err := mgr.GenerateRecoveryScript(bErr)
	if err != nil {
		t.Fatalf("GenerateRecoveryScript failed: %v", err)
	}

	if path != scriptPath {
		t.Errorf("path = %s, want %s", path, scriptPath)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read script: %v", err)
	}

	script := string(content)

	// Check script header
	if !strings.HasPrefix(script, "#!/bin/bash") {
		t.Error("Script should start with shebang")
	}

	// Check error info in comments
	if !strings.Contains(script, "Error: test error") {
		t.Error("Script should contain error message")
	}

	if !strings.Contains(script, "Category: network") {
		t.Error("Script should contain error category")
	}

	// Check action descriptions
	if !strings.Contains(script, "Check network connectivity") {
		t.Error("Script should contain action descriptions")
	}

	// Check manual action has confirmation
	if !strings.Contains(script, "read -p") {
		t.Error("Manual actions should have confirmation prompt")
	}

	// Check auto action runs directly
	if !strings.Contains(script, "echo 'retrying'") {
		t.Error("Script should contain auto action commands")
	}
}

func TestNewPreflightChecker(t *testing.T) {
	output := &bytes.Buffer{}
	checker := NewPreflightChecker(output, true)

	if checker == nil {
		t.Fatal("NewPreflightChecker returned nil")
	}

	if len(checker.checks) == 0 {
		t.Error("Expected default checks to be registered")
	}

	// Verify some default checks exist
	checkNames := make(map[string]bool)
	for _, check := range checker.checks {
		checkNames[check.Name] = true
	}

	expectedChecks := []string{"root-privileges", "disk-space", "memory", "package-manager"}
	for _, name := range expectedChecks {
		if !checkNames[name] {
			t.Errorf("Expected default check '%s' not found", name)
		}
	}
}

func TestPreflightChecker_AddCheck(t *testing.T) {
	output := &bytes.Buffer{}
	checker := NewPreflightChecker(output, false)

	initialCount := len(checker.checks)

	checker.AddCheck(PreflightCheck{
		Name:        "custom-check",
		Description: "A custom check",
		Required:    true,
		Check: func(ctx context.Context, state *State) error {
			return nil
		},
	})

	if len(checker.checks) != initialCount+1 {
		t.Errorf("Check count = %d, want %d", len(checker.checks), initialCount+1)
	}
}

func TestPreflightChecker_RunChecks_AllPass(t *testing.T) {
	output := &bytes.Buffer{}
	checker := &PreflightChecker{
		output:  output,
		verbose: true,
	}

	checker.checks = []PreflightCheck{
		{
			Name:        "always-pass",
			Description: "This always passes",
			Required:    true,
			Check: func(ctx context.Context, state *State) error {
				return nil
			},
		},
	}

	results, err := checker.RunChecks(context.Background(), nil, PhaseDetect)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results))
	}

	if !results[0].Passed {
		t.Error("Expected check to pass")
	}
}

func TestPreflightChecker_RunChecks_RequiredFails(t *testing.T) {
	output := &bytes.Buffer{}
	checker := &PreflightChecker{
		output:  output,
		verbose: true,
	}

	checker.checks = []PreflightCheck{
		{
			Name:        "always-fail",
			Description: "This always fails",
			Required:    true,
			Check: func(ctx context.Context, state *State) error {
				return errors.New("check failed")
			},
		},
	}

	results, err := checker.RunChecks(context.Background(), nil, PhaseDetect)
	if err == nil {
		t.Error("Expected error for required check failure")
	}

	if !strings.Contains(err.Error(), "always-fail") {
		t.Errorf("Error should contain check name: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results))
	}

	if results[0].Passed {
		t.Error("Expected check to fail")
	}

	if results[0].Error != "check failed" {
		t.Errorf("Error = %s, want 'check failed'", results[0].Error)
	}
}

func TestPreflightChecker_RunChecks_OptionalFails(t *testing.T) {
	output := &bytes.Buffer{}
	checker := &PreflightChecker{
		output:  output,
		verbose: true,
	}

	checker.checks = []PreflightCheck{
		{
			Name:        "optional-fail",
			Description: "This optionally fails",
			Required:    false,
			Check: func(ctx context.Context, state *State) error {
				return errors.New("warning")
			},
		},
	}

	results, err := checker.RunChecks(context.Background(), nil, PhaseDetect)
	if err != nil {
		t.Errorf("Optional failure should not return error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results))
	}

	if results[0].Passed {
		t.Error("Expected check to fail")
	}
}

func TestPreflightChecker_RunChecks_AutoFix(t *testing.T) {
	output := &bytes.Buffer{}
	checker := &PreflightChecker{
		output:  output,
		verbose: true,
	}

	fixCalled := false
	checker.checks = []PreflightCheck{
		{
			Name:        "auto-fixable",
			Description: "Can be auto-fixed",
			Required:    true,
			Check: func(ctx context.Context, state *State) error {
				return errors.New("needs fixing")
			},
			AutoFix: func(ctx context.Context, state *State) error {
				fixCalled = true
				return nil
			},
		},
	}

	results, err := checker.RunChecks(context.Background(), nil, PhaseDetect)
	if err != nil {
		t.Errorf("Auto-fix should prevent error: %v", err)
	}

	if !fixCalled {
		t.Error("AutoFix should have been called")
	}

	if len(results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results))
	}

	if !results[0].Fixed {
		t.Error("Expected Fixed to be true")
	}

	if !results[0].Passed {
		t.Error("Expected Passed to be true after auto-fix")
	}
}

func TestPreflightChecker_RunChecks_PhaseFiltering(t *testing.T) {
	output := &bytes.Buffer{}
	checker := &PreflightChecker{
		output:  output,
		verbose: false,
	}

	checker.checks = []PreflightCheck{
		{
			Name:     "install-only",
			Phase:    PhaseInstall,
			Required: true,
			Check: func(ctx context.Context, state *State) error {
				return errors.New("should not run")
			},
		},
		{
			Name:     "all-phases",
			Phase:    "", // Empty means all phases
			Required: true,
			Check: func(ctx context.Context, state *State) error {
				return nil
			},
		},
	}

	// Run for Detect phase - install-only check should be skipped
	results, err := checker.RunChecks(context.Background(), nil, PhaseDetect)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("Expected 1 result (phase-specific check should be skipped), got %d", len(results))
	}
}

func TestFormatPreflightResults_Text(t *testing.T) {
	results := []PreflightResult{
		{
			Name:        "passed-check",
			Description: "Check that passed",
			Passed:      true,
			Required:    true,
		},
		{
			Name:        "failed-required",
			Description: "Required check that failed",
			Passed:      false,
			Required:    true,
			Error:       "something wrong",
		},
		{
			Name:        "failed-optional",
			Description: "Optional check that failed",
			Passed:      false,
			Required:    false,
			Error:       "just a warning",
		},
		{
			Name:        "fixed-check",
			Description: "Check that was fixed",
			Passed:      true,
			Required:    true,
			Fixed:       true,
		},
	}

	output := FormatPreflightResults(results, false)

	// Check passed indicator
	if !strings.Contains(output, "✓") {
		t.Error("Output should contain pass indicator")
	}

	// Check failed required indicator
	if !strings.Contains(output, "✗") {
		t.Error("Output should contain fail indicator")
	}

	// Check warning indicator
	if !strings.Contains(output, "!") {
		t.Error("Output should contain warning indicator")
	}

	// Check fixed indicator
	if !strings.Contains(output, "⚡") {
		t.Error("Output should contain fixed indicator")
	}

	// Check descriptions
	if !strings.Contains(output, "Check that passed") {
		t.Error("Output should contain check descriptions")
	}

	// Check error messages
	if !strings.Contains(output, "something wrong") {
		t.Error("Output should contain error messages")
	}

	// Check auto-fixed label
	if !strings.Contains(output, "(auto-fixed)") {
		t.Error("Output should indicate auto-fixed checks")
	}
}

func TestFormatPreflightResults_JSON(t *testing.T) {
	results := []PreflightResult{
		{
			Name:        "test-check",
			Description: "A test check",
			Passed:      true,
			Required:    true,
		},
	}

	output := FormatPreflightResults(results, true)

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("Output should be valid JSON: %v", err)
	}

	if parsed["event"] != "preflight" {
		t.Errorf("event = %v, want 'preflight'", parsed["event"])
	}

	if parsed["results"] == nil {
		t.Error("results should be present in JSON output")
	}
}

func TestRecoveryResult_Fields(t *testing.T) {
	result := RecoveryResult{
		Success:  true,
		ActionID: "test-action",
		Message:  "completed",
		Output:   "command output",
		Duration: 100,
	}

	// Verify JSON marshaling
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var parsed RecoveryResult
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if parsed.Success != result.Success {
		t.Error("Success field mismatch")
	}

	if parsed.ActionID != result.ActionID {
		t.Error("ActionID field mismatch")
	}

	if parsed.Message != result.Message {
		t.Error("Message field mismatch")
	}
}
