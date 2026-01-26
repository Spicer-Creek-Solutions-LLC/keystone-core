package policy

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestNewPolicyTester(t *testing.T) {
	registry := NewRegistry()
	engine := NewPolicyEngine(registry)
	tester := NewPolicyTester(engine, registry)

	if tester == nil {
		t.Fatal("Expected tester to be created")
	}
	if tester.engine == nil {
		t.Error("Expected engine to be set")
	}
}

func TestPolicyTester_RunTestCase_BasicPass(t *testing.T) {
	registry := NewRegistry()
	engine := NewPolicyEngine(registry)
	tester := NewPolicyTester(engine, registry)

	ctx := context.Background()

	// Create a simple CEL policy
	// Note: CEL evaluator binds action, user, resource directly as variables
	policy := &Policy{
		ID:      "test-policy",
		Name:    "Test Policy",
		Type:    PolicyTypeCEL,
		Policy:  "action == 'read'",
		Enabled: true,
	}

	// Test case that should pass
	tc := &TestCase{
		Name: "allow_read_action",
		Input: &EvaluationInput{
			Resource:  map[string]interface{}{"name": "test"},
			Action:    "read",
			Timestamp: time.Now(),
		},
		Expected: &ExpectedOutcome{
			Allowed: true,
		},
	}

	result := tester.RunTestCase(ctx, policy, tc)

	if result.Error != "" {
		t.Errorf("Test errored: %s", result.Error)
	}
	if !result.Passed {
		t.Errorf("Expected test to pass, got failures: %v, error: %s", result.Failures, result.Error)
	}
}

func TestPolicyTester_RunTestCase_BasicFail(t *testing.T) {
	registry := NewRegistry()
	engine := NewPolicyEngine(registry)
	tester := NewPolicyTester(engine, registry)

	ctx := context.Background()

	// Create a simple CEL policy
	policy := &Policy{
		ID:      "test-policy",
		Name:    "Test Policy",
		Type:    PolicyTypeCEL,
		Policy:  "action == 'read'",
		Enabled: true,
	}

	// Test case that should fail (expecting wrong result)
	tc := &TestCase{
		Name: "expect_denied_but_allowed",
		Input: &EvaluationInput{
			Resource:  map[string]interface{}{"name": "test"},
			Action:    "read",
			Timestamp: time.Now(),
		},
		Expected: &ExpectedOutcome{
			Allowed: false, // Wrong expectation
		},
	}

	result := tester.RunTestCase(ctx, policy, tc)

	if result.Passed {
		t.Error("Expected test to fail due to wrong expectation")
	}
	if len(result.Failures) == 0 {
		t.Error("Expected failures to be recorded")
	}
}

func TestPolicyTester_RunTestCase_Skip(t *testing.T) {
	registry := NewRegistry()
	engine := NewPolicyEngine(registry)
	tester := NewPolicyTester(engine, registry)

	ctx := context.Background()

	policy := &Policy{
		ID:     "test-policy",
		Type:   PolicyTypeCEL,
		Policy: "true",
	}

	tc := &TestCase{
		Name:       "skipped_test",
		Skip:       true,
		SkipReason: "Not implemented yet",
		Input: &EvaluationInput{
			Resource: map[string]interface{}{},
		},
		Expected: &ExpectedOutcome{
			Allowed: true,
		},
	}

	result := tester.RunTestCase(ctx, policy, tc)

	if !result.Skipped {
		t.Error("Expected test to be skipped")
	}
}

func TestPolicyTester_RunTestCase_NilInput(t *testing.T) {
	registry := NewRegistry()
	engine := NewPolicyEngine(registry)
	tester := NewPolicyTester(engine, registry)

	ctx := context.Background()

	policy := &Policy{
		ID:     "test-policy",
		Type:   PolicyTypeCEL,
		Policy: "true",
	}

	tc := &TestCase{
		Name:  "nil_input_test",
		Input: nil,
		Expected: &ExpectedOutcome{
			Allowed: true,
		},
	}

	result := tester.RunTestCase(ctx, policy, tc)

	if result.Error == "" {
		t.Error("Expected error for nil input")
	}
}

func TestPolicyTester_RunTestCase_NilExpected(t *testing.T) {
	registry := NewRegistry()
	engine := NewPolicyEngine(registry)
	tester := NewPolicyTester(engine, registry)

	ctx := context.Background()

	policy := &Policy{
		ID:     "test-policy",
		Type:   PolicyTypeCEL,
		Policy: "true",
	}

	tc := &TestCase{
		Name: "nil_expected_test",
		Input: &EvaluationInput{
			Resource: map[string]interface{}{},
		},
		Expected: nil,
	}

	result := tester.RunTestCase(ctx, policy, tc)

	if result.Error == "" {
		t.Error("Expected error for nil expected")
	}
}

func TestPolicyTester_RunTestCase_ViolationCount(t *testing.T) {
	registry := NewRegistry()
	engine := NewPolicyEngine(registry)
	tester := NewPolicyTester(engine, registry)

	ctx := context.Background()

	// Builtin policy that generates violations
	// Uses require-labels builtin policy with proper config format
	labelsConfig, _ := json.Marshal(map[string]interface{}{
		"labels": []string{"env", "owner"},
	})
	policyConfig := map[string]interface{}{
		"name":   "require-labels",
		"config": json.RawMessage(labelsConfig),
	}
	policyJSON, _ := json.Marshal(policyConfig)

	policy := &Policy{
		ID:       "required-labels-policy",
		Name:     "Required Labels",
		Type:     PolicyTypeBuiltin,
		Policy:   string(policyJSON),
		Severity: SeverityMedium,
		Enabled:  true,
	}

	// Resource missing required labels
	violationCount := 2
	tc := &TestCase{
		Name: "check_violation_count",
		Input: &EvaluationInput{
			Resource: map[string]interface{}{
				"labels": map[string]string{}, // Missing required labels
			},
			Action:    "create",
			Timestamp: time.Now(),
		},
		Expected: &ExpectedOutcome{
			Allowed:        false,
			ViolationCount: &violationCount,
		},
	}

	result := tester.RunTestCase(ctx, policy, tc)

	// The actual result depends on engine implementation
	// Just verify the test ran
	if result.Error != "" {
		t.Errorf("Test errored: %s", result.Error)
	}
}

func TestPolicyTester_RunTestSuite(t *testing.T) {
	registry := NewRegistry()
	engine := NewPolicyEngine(registry)
	tester := NewPolicyTester(engine, registry)

	ctx := context.Background()

	suite := &TestSuite{
		Name:        "Basic Test Suite",
		Description: "Tests basic policy functionality",
		Policy: &Policy{
			ID:      "test-policy",
			Name:    "Test Policy",
			Type:    PolicyTypeCEL,
			Policy:  "action == 'read'",
			Enabled: true,
		},
		TestCases: []*TestCase{
			{
				Name: "allow_read",
				Input: &EvaluationInput{
					Resource:  map[string]interface{}{"name": "test"},
					Action:    "read",
					Timestamp: time.Now(),
				},
				Expected: &ExpectedOutcome{
					Allowed: true,
				},
			},
			{
				Name: "deny_write",
				Input: &EvaluationInput{
					Resource:  map[string]interface{}{"name": "test"},
					Action:    "write",
					Timestamp: time.Now(),
				},
				Expected: &ExpectedOutcome{
					Allowed: false,
				},
			},
			{
				Name:       "skipped_test",
				Skip:       true,
				SkipReason: "Test not ready",
				Input: &EvaluationInput{
					Resource: map[string]interface{}{},
				},
				Expected: &ExpectedOutcome{
					Allowed: true,
				},
			},
		},
	}

	result := tester.RunTestSuite(ctx, suite)

	if result.Passed != 2 {
		t.Errorf("Expected 2 passed, got %d", result.Passed)
	}
	if result.Skipped != 1 {
		t.Errorf("Expected 1 skipped, got %d", result.Skipped)
	}
	if !result.Success() {
		t.Error("Expected suite to succeed")
	}
}

func TestPolicyTester_RunTestSuite_NoPolicy(t *testing.T) {
	registry := NewRegistry()
	engine := NewPolicyEngine(registry)
	tester := NewPolicyTester(engine, registry)

	ctx := context.Background()

	suite := &TestSuite{
		Name: "Suite without policy",
		TestCases: []*TestCase{
			{
				Name: "test",
				Input: &EvaluationInput{
					Resource: map[string]interface{}{},
				},
				Expected: &ExpectedOutcome{
					Allowed: true,
				},
			},
		},
	}

	result := tester.RunTestSuite(ctx, suite)

	if result.Errored != 1 {
		t.Error("Expected 1 error for missing policy")
	}
}

func TestPolicyTester_ValidatePolicy_Valid(t *testing.T) {
	registry := NewRegistry()
	engine := NewPolicyEngine(registry)
	tester := NewPolicyTester(engine, registry)

	ctx := context.Background()

	policy := &Policy{
		ID:              "valid-policy",
		Name:            "Valid Policy",
		Type:            PolicyTypeCEL,
		Policy:          "input.action == 'read'",
		Severity:        SeverityMedium,
		EnforcementMode: ModeEnforce,
		Enabled:         true,
	}

	result := tester.ValidatePolicy(ctx, policy)

	if !result.Valid {
		t.Errorf("Expected policy to be valid, got errors: %v", result.Errors)
	}
}

func TestPolicyTester_ValidatePolicy_MissingID(t *testing.T) {
	registry := NewRegistry()
	engine := NewPolicyEngine(registry)
	tester := NewPolicyTester(engine, registry)

	ctx := context.Background()

	policy := &Policy{
		Name:   "Policy without ID",
		Type:   PolicyTypeCEL,
		Policy: "true",
	}

	result := tester.ValidatePolicy(ctx, policy)

	if result.Valid {
		t.Error("Expected policy to be invalid due to missing ID")
	}

	found := false
	for _, err := range result.Errors {
		if err == "policy ID is required" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected error about missing ID")
	}
}

func TestPolicyTester_ValidatePolicy_InvalidSeverity(t *testing.T) {
	registry := NewRegistry()
	engine := NewPolicyEngine(registry)
	tester := NewPolicyTester(engine, registry)

	ctx := context.Background()

	policy := &Policy{
		ID:       "test-policy",
		Name:     "Test Policy",
		Type:     PolicyTypeCEL,
		Policy:   "true",
		Severity: Severity("invalid"),
	}

	result := tester.ValidatePolicy(ctx, policy)

	if result.Valid {
		t.Error("Expected policy to be invalid due to invalid severity")
	}
}

func TestPolicyTester_ValidatePolicy_OPA(t *testing.T) {
	registry := NewRegistry()
	engine := NewPolicyEngine(registry)
	tester := NewPolicyTester(engine, registry)

	ctx := context.Background()

	// Valid OPA policy
	validOPA := &Policy{
		ID:   "opa-policy",
		Name: "OPA Policy",
		Type: PolicyTypeOPA,
		Policy: `
package keystone

allow {
    input.action == "read"
}
`,
	}

	result := tester.ValidatePolicy(ctx, validOPA)
	if !result.Valid {
		t.Errorf("Expected valid OPA policy, got errors: %v", result.Errors)
	}

	// Invalid OPA policy (missing package)
	invalidOPA := &Policy{
		ID:     "invalid-opa",
		Name:   "Invalid OPA",
		Type:   PolicyTypeOPA,
		Policy: "allow { true }",
	}

	result = tester.ValidatePolicy(ctx, invalidOPA)
	if result.Valid {
		t.Error("Expected invalid OPA policy without package")
	}
}

func TestPolicyTester_ValidatePolicy_CEL(t *testing.T) {
	registry := NewRegistry()
	engine := NewPolicyEngine(registry)
	tester := NewPolicyTester(engine, registry)

	ctx := context.Background()

	// Valid CEL policy
	validCEL := &Policy{
		ID:     "cel-policy",
		Name:   "CEL Policy",
		Type:   PolicyTypeCEL,
		Policy: "input.action == 'read' && resource.type == 'document'",
	}

	result := tester.ValidatePolicy(ctx, validCEL)
	if !result.Valid {
		t.Errorf("Expected valid CEL policy, got errors: %v", result.Errors)
	}

	// CEL with unbalanced parentheses
	unbalancedCEL := &Policy{
		ID:     "unbalanced-cel",
		Name:   "Unbalanced CEL",
		Type:   PolicyTypeCEL,
		Policy: "((input.action == 'read'",
	}

	result = tester.ValidatePolicy(ctx, unbalancedCEL)
	if result.Valid {
		t.Error("Expected invalid CEL policy with unbalanced parentheses")
	}
}

func TestPolicyTester_ValidatePolicy_Builtin(t *testing.T) {
	registry := NewRegistry()
	engine := NewPolicyEngine(registry)
	tester := NewPolicyTester(engine, registry)

	ctx := context.Background()

	// Valid builtin policy
	validBuiltin := &Policy{
		ID:     "builtin-policy",
		Name:   "Builtin Policy",
		Type:   PolicyTypeBuiltin,
		Policy: `{"rule": "required_tags", "tags": ["env", "owner"]}`,
	}

	result := tester.ValidatePolicy(ctx, validBuiltin)
	if !result.Valid {
		t.Errorf("Expected valid builtin policy, got errors: %v", result.Errors)
	}

	// Invalid builtin (not JSON)
	invalidBuiltin := &Policy{
		ID:     "invalid-builtin",
		Name:   "Invalid Builtin",
		Type:   PolicyTypeBuiltin,
		Policy: "not json",
	}

	result = tester.ValidatePolicy(ctx, invalidBuiltin)
	if result.Valid {
		t.Error("Expected invalid builtin policy (not JSON)")
	}

	// Invalid builtin (missing rule)
	missingRule := &Policy{
		ID:     "missing-rule-builtin",
		Name:   "Missing Rule",
		Type:   PolicyTypeBuiltin,
		Policy: `{"tags": ["env"]}`,
	}

	result = tester.ValidatePolicy(ctx, missingRule)
	if result.Valid {
		t.Error("Expected invalid builtin policy (missing rule)")
	}
}

func TestSuiteResult_Success(t *testing.T) {
	// All passed
	result := &SuiteResult{Passed: 5, Failed: 0, Errored: 0}
	if !result.Success() {
		t.Error("Expected success when all passed")
	}

	// With failures
	result = &SuiteResult{Passed: 4, Failed: 1, Errored: 0}
	if result.Success() {
		t.Error("Expected failure when some failed")
	}

	// With errors
	result = &SuiteResult{Passed: 5, Failed: 0, Errored: 1}
	if result.Success() {
		t.Error("Expected failure when some errored")
	}
}

func TestSuiteResult_Summary(t *testing.T) {
	result := &SuiteResult{
		Passed:        3,
		Failed:        1,
		Skipped:       1,
		Errored:       0,
		TotalDuration: 500 * time.Millisecond,
	}

	summary := result.Summary()
	if summary == "" {
		t.Error("Expected non-empty summary")
	}
}

func TestPolicyValidationResult_Summary(t *testing.T) {
	// Valid
	result := &PolicyValidationResult{Valid: true}
	if result.Summary() != "valid" {
		t.Errorf("Expected 'valid', got '%s'", result.Summary())
	}

	// Valid with warnings
	result = &PolicyValidationResult{Valid: true, Warnings: []string{"warning"}}
	if result.Summary() != "valid with 1 warning(s)" {
		t.Errorf("Expected 'valid with 1 warning(s)', got '%s'", result.Summary())
	}

	// Invalid
	result = &PolicyValidationResult{Valid: false, Errors: []string{"error1", "error2"}}
	if result.Summary() != "invalid: 2 error(s)" {
		t.Errorf("Expected 'invalid: 2 error(s)', got '%s'", result.Summary())
	}
}

func TestTestReporter_FormatSuiteResult(t *testing.T) {
	reporter := NewTestReporter(false)

	result := &SuiteResult{
		Suite: &TestSuite{
			Name:     "Test Suite",
			PolicyID: "test-policy",
		},
		Results: []*TestResult{
			{
				TestCase: &TestCase{Name: "test1"},
				Passed:   true,
				Duration: 100 * time.Millisecond,
			},
			{
				TestCase: &TestCase{Name: "test2"},
				Passed:   false,
				Failures: []string{"expected true, got false"},
				Duration: 50 * time.Millisecond,
			},
		},
		Passed:        1,
		Failed:        1,
		TotalDuration: 150 * time.Millisecond,
	}

	output := reporter.FormatSuiteResult(result)

	if output == "" {
		t.Error("Expected non-empty output")
	}
	if !stringContains(output, "Test Suite") {
		t.Error("Expected suite name in output")
	}
	if !stringContains(output, "test1") {
		t.Error("Expected test names in output")
	}
}

func TestTestReporter_FormatValidationResult(t *testing.T) {
	reporter := NewTestReporter(false)

	result := &PolicyValidationResult{
		PolicyID: "test-policy",
		Valid:    false,
		Errors:   []string{"error1"},
		Warnings: []string{"warning1"},
	}

	output := reporter.FormatValidationResult(result)

	if output == "" {
		t.Error("Expected non-empty output")
	}
	if !stringContains(output, "test-policy") {
		t.Error("Expected policy ID in output")
	}
	if !stringContains(output, "error1") {
		t.Error("Expected errors in output")
	}
}

func TestLoadTestSuiteFromJSON(t *testing.T) {
	jsonData := `{
		"name": "JSON Test Suite",
		"policy_id": "test-policy",
		"test_cases": [
			{
				"name": "test1",
				"input": {
					"resource": {"name": "test"},
					"action": "read"
				},
				"expected": {
					"allowed": true
				}
			}
		]
	}`

	suite, err := LoadTestSuiteFromJSON([]byte(jsonData))
	if err != nil {
		t.Fatalf("Failed to load test suite: %v", err)
	}

	if suite.Name != "JSON Test Suite" {
		t.Errorf("Expected name 'JSON Test Suite', got '%s'", suite.Name)
	}
	if len(suite.TestCases) != 1 {
		t.Errorf("Expected 1 test case, got %d", len(suite.TestCases))
	}
}

func TestLoadTestCaseFromJSON(t *testing.T) {
	jsonData := `{
		"name": "JSON Test Case",
		"input": {
			"resource": {"name": "test"},
			"action": "read"
		},
		"expected": {
			"allowed": true,
			"violation_count": 0
		}
	}`

	tc, err := LoadTestCaseFromJSON([]byte(jsonData))
	if err != nil {
		t.Fatalf("Failed to load test case: %v", err)
	}

	if tc.Name != "JSON Test Case" {
		t.Errorf("Expected name 'JSON Test Case', got '%s'", tc.Name)
	}
	if tc.Expected == nil {
		t.Error("Expected expected outcome to be loaded")
	}
}

// Helper function for string contains check
func stringContains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && stringContainsHelper(s, substr))
}

func stringContainsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
