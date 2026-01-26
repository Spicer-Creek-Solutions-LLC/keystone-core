package policy

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// TestCase represents a single policy test case
type TestCase struct {
	// Name is the name of the test case
	Name string `json:"name"`

	// Description describes what the test verifies
	Description string `json:"description,omitempty"`

	// Input is the evaluation input for this test
	Input *EvaluationInput `json:"input"`

	// Expected is the expected outcome
	Expected *ExpectedOutcome `json:"expected"`

	// Tags for categorizing test cases
	Tags []string `json:"tags,omitempty"`

	// Skip if true, this test is skipped
	Skip bool `json:"skip,omitempty"`

	// SkipReason explains why the test is skipped
	SkipReason string `json:"skip_reason,omitempty"`
}

// ExpectedOutcome defines the expected result of a test case
type ExpectedOutcome struct {
	// Allowed is the expected allowed value
	Allowed bool `json:"allowed"`

	// ViolationCount is the expected number of violations (optional)
	ViolationCount *int `json:"violation_count,omitempty"`

	// ViolationRules are expected rule names in violations
	ViolationRules []string `json:"violation_rules,omitempty"`

	// ViolationSeverities are expected severities
	ViolationSeverities []Severity `json:"violation_severities,omitempty"`

	// WarningCount is the expected number of warnings (optional)
	WarningCount *int `json:"warning_count,omitempty"`

	// ContainsMessage checks if result message contains this string
	ContainsMessage string `json:"contains_message,omitempty"`

	// Metadata checks for specific metadata values
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// TestSuite represents a collection of test cases for a policy
type TestSuite struct {
	// Name is the name of the test suite
	Name string `json:"name"`

	// Description describes the test suite
	Description string `json:"description,omitempty"`

	// PolicyID is the ID of the policy being tested
	PolicyID string `json:"policy_id"`

	// Policy is the policy definition (if not using an existing policy)
	Policy *Policy `json:"policy,omitempty"`

	// TestCases in this suite
	TestCases []*TestCase `json:"test_cases"`

	// Setup runs before each test
	Setup *TestSetup `json:"setup,omitempty"`

	// Teardown runs after each test
	Teardown *TestTeardown `json:"teardown,omitempty"`

	// Tags for categorizing test suites
	Tags []string `json:"tags,omitempty"`
}

// TestSetup defines setup operations before tests
type TestSetup struct {
	// Context values to set
	Context map[string]interface{} `json:"context,omitempty"`

	// Variables to define
	Variables map[string]interface{} `json:"variables,omitempty"`
}

// TestTeardown defines teardown operations after tests
type TestTeardown struct {
	// Actions to perform
	Actions []string `json:"actions,omitempty"`
}

// TestResult represents the result of running a single test case
type TestResult struct {
	// TestCase that was run
	TestCase *TestCase `json:"test_case"`

	// Passed indicates if the test passed
	Passed bool `json:"passed"`

	// Skipped indicates if the test was skipped
	Skipped bool `json:"skipped"`

	// Actual is the actual evaluation result
	Actual *EvaluationResult `json:"actual,omitempty"`

	// Failures describes what failed
	Failures []string `json:"failures,omitempty"`

	// Duration of the test
	Duration time.Duration `json:"duration"`

	// Error if the test errored (not failed)
	Error string `json:"error,omitempty"`
}

// SuiteResult represents the result of running a test suite
type SuiteResult struct {
	// Suite that was run
	Suite *TestSuite `json:"suite"`

	// Results for each test case
	Results []*TestResult `json:"results"`

	// Passed count
	Passed int `json:"passed"`

	// Failed count
	Failed int `json:"failed"`

	// Skipped count
	Skipped int `json:"skipped"`

	// Errored count
	Errored int `json:"errored"`

	// TotalDuration for the suite
	TotalDuration time.Duration `json:"total_duration"`

	// StartedAt timestamp
	StartedAt time.Time `json:"started_at"`

	// FinishedAt timestamp
	FinishedAt time.Time `json:"finished_at"`
}

// Success returns true if all tests passed
func (r *SuiteResult) Success() bool {
	return r.Failed == 0 && r.Errored == 0
}

// Summary returns a human-readable summary
func (r *SuiteResult) Summary() string {
	total := r.Passed + r.Failed + r.Skipped + r.Errored
	return fmt.Sprintf("%d/%d passed, %d failed, %d skipped, %d errored (%.2fs)",
		r.Passed, total, r.Failed, r.Skipped, r.Errored,
		r.TotalDuration.Seconds())
}

// PolicyTester provides policy testing capabilities
type PolicyTester struct {
	engine   *PolicyEngine
	registry *Registry
	verbose  bool
}

// NewPolicyTester creates a new policy tester
func NewPolicyTester(engine *PolicyEngine, registry *Registry) *PolicyTester {
	return &PolicyTester{
		engine:   engine,
		registry: registry,
	}
}

// SetVerbose enables verbose output
func (t *PolicyTester) SetVerbose(verbose bool) {
	t.verbose = verbose
}

// RunTestCase runs a single test case against a policy
func (t *PolicyTester) RunTestCase(ctx context.Context, policy *Policy, tc *TestCase) *TestResult {
	result := &TestResult{
		TestCase: tc,
		Failures: make([]string, 0),
	}

	// Check for skip
	if tc.Skip {
		result.Skipped = true
		return result
	}

	startTime := time.Now()

	// Validate test case
	if tc.Input == nil {
		result.Error = "test case input is nil"
		return result
	}
	if tc.Expected == nil {
		result.Error = "test case expected outcome is nil"
		return result
	}

	// Ensure policy has an ID for registration
	if policy.ID == "" {
		policy.ID = fmt.Sprintf("test-policy-%d", time.Now().UnixNano())
	}

	// Register policy temporarily in the engine's registry
	if t.registry != nil {
		// Use a temporary registry to avoid polluting the main one
		_ = t.registry.RegisterPolicy(policy)
		defer func() {
			_ = t.registry.DeletePolicy(policy.ID)
		}()
	}

	// Evaluate policy
	evalResult, err := t.engine.Evaluate(ctx, policy.ID, tc.Input)
	if err != nil {
		result.Error = fmt.Sprintf("evaluation error: %v", err)
		result.Duration = time.Since(startTime)
		return result
	}

	result.Actual = evalResult
	result.Duration = time.Since(startTime)

	// Compare results
	t.compareResults(tc.Expected, evalResult, result)

	result.Passed = len(result.Failures) == 0 && result.Error == ""
	return result
}

// compareResults compares expected and actual results
func (t *PolicyTester) compareResults(expected *ExpectedOutcome, actual *EvaluationResult, result *TestResult) {
	// Check allowed
	if actual.Allowed != expected.Allowed {
		result.Failures = append(result.Failures,
			fmt.Sprintf("expected allowed=%v, got allowed=%v", expected.Allowed, actual.Allowed))
	}

	// Check violation count
	if expected.ViolationCount != nil {
		actualCount := len(actual.Violations)
		if actualCount != *expected.ViolationCount {
			result.Failures = append(result.Failures,
				fmt.Sprintf("expected %d violations, got %d", *expected.ViolationCount, actualCount))
		}
	}

	// Check violation rules
	if len(expected.ViolationRules) > 0 {
		actualRules := make(map[string]bool)
		for _, v := range actual.Violations {
			actualRules[v.Rule] = true
		}
		for _, expectedRule := range expected.ViolationRules {
			if !actualRules[expectedRule] {
				result.Failures = append(result.Failures,
					fmt.Sprintf("expected violation rule '%s' not found", expectedRule))
			}
		}
	}

	// Check violation severities
	if len(expected.ViolationSeverities) > 0 {
		actualSeverities := make(map[Severity]bool)
		for _, v := range actual.Violations {
			actualSeverities[v.Severity] = true
		}
		for _, expectedSev := range expected.ViolationSeverities {
			if !actualSeverities[expectedSev] {
				result.Failures = append(result.Failures,
					fmt.Sprintf("expected severity '%s' not found in violations", expectedSev))
			}
		}
	}

	// Check warning count
	if expected.WarningCount != nil {
		actualCount := len(actual.Warnings)
		if actualCount != *expected.WarningCount {
			result.Failures = append(result.Failures,
				fmt.Sprintf("expected %d warnings, got %d", *expected.WarningCount, actualCount))
		}
	}

	// Check message contains
	if expected.ContainsMessage != "" {
		if !strings.Contains(actual.Message, expected.ContainsMessage) {
			result.Failures = append(result.Failures,
				fmt.Sprintf("expected message to contain '%s', got '%s'",
					expected.ContainsMessage, actual.Message))
		}
	}

	// Check metadata
	if expected.Metadata != nil {
		for key, expectedVal := range expected.Metadata {
			actualVal, ok := actual.Metadata[key]
			if !ok {
				result.Failures = append(result.Failures,
					fmt.Sprintf("expected metadata key '%s' not found", key))
			} else if !compareValues(expectedVal, actualVal) {
				result.Failures = append(result.Failures,
					fmt.Sprintf("metadata '%s': expected %v, got %v", key, expectedVal, actualVal))
			}
		}
	}
}

// compareValues compares two interface values
func compareValues(expected, actual interface{}) bool {
	// Convert to JSON and compare
	expectedJSON, _ := json.Marshal(expected)
	actualJSON, _ := json.Marshal(actual)
	return string(expectedJSON) == string(actualJSON)
}

// RunTestSuite runs a complete test suite
func (t *PolicyTester) RunTestSuite(ctx context.Context, suite *TestSuite) *SuiteResult {
	result := &SuiteResult{
		Suite:     suite,
		Results:   make([]*TestResult, 0),
		StartedAt: time.Now(),
	}

	// Get or create policy
	var policy *Policy
	var ok bool

	if suite.Policy != nil {
		policy = suite.Policy
	} else if suite.PolicyID != "" && t.registry != nil {
		policy, ok = t.registry.GetPolicy(suite.PolicyID)
		if !ok {
			// Create a single failed result for the suite
			result.Results = append(result.Results, &TestResult{
				TestCase: &TestCase{Name: "suite_setup"},
				Error:    fmt.Sprintf("policy not found: %s", suite.PolicyID),
			})
			result.Errored = 1
			result.FinishedAt = time.Now()
			result.TotalDuration = result.FinishedAt.Sub(result.StartedAt)
			return result
		}
	} else {
		result.Results = append(result.Results, &TestResult{
			TestCase: &TestCase{Name: "suite_setup"},
			Error:    "no policy specified for test suite",
		})
		result.Errored = 1
		result.FinishedAt = time.Now()
		result.TotalDuration = result.FinishedAt.Sub(result.StartedAt)
		return result
	}

	// Apply setup
	if suite.Setup != nil {
		for _, tc := range suite.TestCases {
			if tc.Input != nil && tc.Input.Context == nil {
				tc.Input.Context = make(map[string]interface{})
			}
			for k, v := range suite.Setup.Context {
				if tc.Input != nil && tc.Input.Context != nil {
					tc.Input.Context[k] = v
				}
			}
		}
	}

	// Run each test case
	for _, tc := range suite.TestCases {
		testResult := t.RunTestCase(ctx, policy, tc)
		result.Results = append(result.Results, testResult)

		if testResult.Skipped {
			result.Skipped++
		} else if testResult.Error != "" {
			result.Errored++
		} else if testResult.Passed {
			result.Passed++
		} else {
			result.Failed++
		}
	}

	result.FinishedAt = time.Now()
	result.TotalDuration = result.FinishedAt.Sub(result.StartedAt)

	return result
}

// ValidatePolicy validates a policy without executing it
func (t *PolicyTester) ValidatePolicy(ctx context.Context, policy *Policy) *PolicyValidationResult {
	result := &PolicyValidationResult{
		PolicyID: policy.ID,
		Valid:    true,
		Errors:   make([]string, 0),
		Warnings: make([]string, 0),
	}

	// Basic validation
	if policy.ID == "" {
		result.Errors = append(result.Errors, "policy ID is required")
	}
	if policy.Name == "" {
		result.Errors = append(result.Errors, "policy name is required")
	}
	if policy.Type == "" {
		result.Errors = append(result.Errors, "policy type is required")
	}
	if policy.Policy == "" {
		result.Errors = append(result.Errors, "policy code is required")
	}

	// Type-specific validation
	switch policy.Type {
	case PolicyTypeOPA:
		t.validateOPAPolicy(policy, result)
	case PolicyTypeCEL:
		t.validateCELPolicy(policy, result)
	case PolicyTypeBuiltin:
		t.validateBuiltinPolicy(policy, result)
	default:
		result.Errors = append(result.Errors, fmt.Sprintf("unknown policy type: %s", policy.Type))
	}

	// Check severity
	if policy.Severity != "" {
		validSeverities := map[Severity]bool{
			SeverityLow:      true,
			SeverityMedium:   true,
			SeverityHigh:     true,
			SeverityCritical: true,
		}
		if !validSeverities[policy.Severity] {
			result.Errors = append(result.Errors, fmt.Sprintf("invalid severity: %s", policy.Severity))
		}
	}

	// Check enforcement mode
	if policy.EnforcementMode != "" {
		validModes := map[EnforcementMode]bool{
			ModeAudit:   true,
			ModeEnforce: true,
			ModeWarn:    true,
		}
		if !validModes[policy.EnforcementMode] {
			result.Errors = append(result.Errors, fmt.Sprintf("invalid enforcement mode: %s", policy.EnforcementMode))
		}
	}

	result.Valid = len(result.Errors) == 0
	return result
}

// validateOPAPolicy validates OPA/Rego policy syntax
func (t *PolicyTester) validateOPAPolicy(policy *Policy, result *PolicyValidationResult) {
	// Basic Rego syntax checks
	if !strings.Contains(policy.Policy, "package") {
		result.Errors = append(result.Errors, "OPA policy must contain a package declaration")
	}

	// Check for common issues
	if strings.Contains(policy.Policy, "allow") || strings.Contains(policy.Policy, "deny") {
		// Good - has standard allow/deny rules
	} else if strings.Contains(policy.Policy, "violation") {
		// Good - has violation rule
	} else {
		result.Warnings = append(result.Warnings, "OPA policy should define 'allow', 'deny', or 'violation' rules")
	}

	// Check for unclosed braces
	openBraces := strings.Count(policy.Policy, "{")
	closeBraces := strings.Count(policy.Policy, "}")
	if openBraces != closeBraces {
		result.Errors = append(result.Errors, fmt.Sprintf("mismatched braces: %d open, %d close", openBraces, closeBraces))
	}
}

// validateCELPolicy validates CEL policy syntax
func (t *PolicyTester) validateCELPolicy(policy *Policy, result *PolicyValidationResult) {
	// Basic CEL syntax checks
	if len(strings.TrimSpace(policy.Policy)) == 0 {
		result.Errors = append(result.Errors, "CEL policy expression is empty")
		return
	}

	// Check for balanced parentheses
	openParens := strings.Count(policy.Policy, "(")
	closeParens := strings.Count(policy.Policy, ")")
	if openParens != closeParens {
		result.Errors = append(result.Errors, fmt.Sprintf("mismatched parentheses: %d open, %d close", openParens, closeParens))
	}

	// Check for common CEL patterns
	if !strings.Contains(policy.Policy, "input") && !strings.Contains(policy.Policy, "resource") {
		result.Warnings = append(result.Warnings, "CEL policy should reference 'input' or 'resource'")
	}
}

// validateBuiltinPolicy validates builtin policy configuration
func (t *PolicyTester) validateBuiltinPolicy(policy *Policy, result *PolicyValidationResult) {
	// Parse as JSON to validate structure
	var config map[string]interface{}
	if err := json.Unmarshal([]byte(policy.Policy), &config); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("builtin policy must be valid JSON: %v", err))
		return
	}

	// Check for required fields based on builtin type
	if ruleType, ok := config["rule"].(string); ok {
		switch ruleType {
		case "required_tags", "required_labels":
			if _, ok := config["tags"]; !ok {
				result.Errors = append(result.Errors, "required_tags rule must specify 'tags' field")
			}
		case "naming_convention":
			if _, ok := config["pattern"]; !ok {
				result.Errors = append(result.Errors, "naming_convention rule must specify 'pattern' field")
			}
		case "resource_limits":
			// Valid without additional fields
		default:
			result.Warnings = append(result.Warnings, fmt.Sprintf("unknown builtin rule type: %s", ruleType))
		}
	} else {
		result.Errors = append(result.Errors, "builtin policy must specify 'rule' field")
	}
}

// PolicyValidationResult contains the result of policy validation
type PolicyValidationResult struct {
	// PolicyID that was validated
	PolicyID string `json:"policy_id"`

	// Valid indicates if the policy is valid
	Valid bool `json:"valid"`

	// Errors found during validation
	Errors []string `json:"errors,omitempty"`

	// Warnings found during validation
	Warnings []string `json:"warnings,omitempty"`
}

// Summary returns a human-readable summary
func (r *PolicyValidationResult) Summary() string {
	if r.Valid {
		if len(r.Warnings) > 0 {
			return fmt.Sprintf("valid with %d warning(s)", len(r.Warnings))
		}
		return "valid"
	}
	return fmt.Sprintf("invalid: %d error(s)", len(r.Errors))
}

// TestReporter formats test results for output
type TestReporter struct {
	verbose bool
}

// NewTestReporter creates a new test reporter
func NewTestReporter(verbose bool) *TestReporter {
	return &TestReporter{verbose: verbose}
}

// FormatSuiteResult formats a suite result for output
func (r *TestReporter) FormatSuiteResult(result *SuiteResult) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("\n=== Test Suite: %s ===\n", result.Suite.Name))
	if result.Suite.Description != "" {
		sb.WriteString(fmt.Sprintf("Description: %s\n", result.Suite.Description))
	}
	sb.WriteString(fmt.Sprintf("Policy: %s\n\n", result.Suite.PolicyID))

	for _, tr := range result.Results {
		r.formatTestResult(&sb, tr)
	}

	sb.WriteString("\n--- Summary ---\n")
	sb.WriteString(result.Summary())
	sb.WriteString("\n")

	if !result.Success() {
		sb.WriteString("\nFailed tests:\n")
		for _, tr := range result.Results {
			if !tr.Passed && !tr.Skipped {
				sb.WriteString(fmt.Sprintf("  - %s\n", tr.TestCase.Name))
			}
		}
	}

	return sb.String()
}

// formatTestResult formats a single test result
func (r *TestReporter) formatTestResult(sb *strings.Builder, result *TestResult) {
	var status string
	switch {
	case result.Skipped:
		status = "SKIP"
	case result.Error != "":
		status = "ERROR"
	case result.Passed:
		status = "PASS"
	default:
		status = "FAIL"
	}

	sb.WriteString(fmt.Sprintf("[%s] %s (%.3fs)\n", status, result.TestCase.Name, result.Duration.Seconds()))

	if r.verbose || !result.Passed {
		if result.TestCase.Description != "" {
			sb.WriteString(fmt.Sprintf("  Description: %s\n", result.TestCase.Description))
		}

		if result.Skipped && result.TestCase.SkipReason != "" {
			sb.WriteString(fmt.Sprintf("  Skip reason: %s\n", result.TestCase.SkipReason))
		}

		if result.Error != "" {
			sb.WriteString(fmt.Sprintf("  Error: %s\n", result.Error))
		}

		for _, failure := range result.Failures {
			sb.WriteString(fmt.Sprintf("  Failure: %s\n", failure))
		}
	}
}

// FormatValidationResult formats a validation result for output
func (r *TestReporter) FormatValidationResult(result *PolicyValidationResult) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("\n=== Policy Validation: %s ===\n", result.PolicyID))
	sb.WriteString(fmt.Sprintf("Status: %s\n", result.Summary()))

	if len(result.Errors) > 0 {
		sb.WriteString("\nErrors:\n")
		for _, err := range result.Errors {
			sb.WriteString(fmt.Sprintf("  - %s\n", err))
		}
	}

	if len(result.Warnings) > 0 {
		sb.WriteString("\nWarnings:\n")
		for _, warn := range result.Warnings {
			sb.WriteString(fmt.Sprintf("  - %s\n", warn))
		}
	}

	return sb.String()
}

// LoadTestSuiteFromJSON loads a test suite from JSON
func LoadTestSuiteFromJSON(data []byte) (*TestSuite, error) {
	var suite TestSuite
	if err := json.Unmarshal(data, &suite); err != nil {
		return nil, fmt.Errorf("failed to parse test suite: %w", err)
	}
	return &suite, nil
}

// LoadTestCaseFromJSON loads a test case from JSON
func LoadTestCaseFromJSON(data []byte) (*TestCase, error) {
	var tc TestCase
	if err := json.Unmarshal(data, &tc); err != nil {
		return nil, fmt.Errorf("failed to parse test case: %w", err)
	}
	return &tc, nil
}
