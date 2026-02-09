package bootstrap

import (
	"errors"
	"testing"
)

func TestClassifyError_Permission(t *testing.T) {
	tests := []struct {
		name    string
		errMsg  string
		wantCat ErrorCategory
	}{
		{"permission denied", "open /etc/keystone-core: permission denied", ErrorCategoryPermission},
		{"access denied", "access denied to /var/lib/keystone-core", ErrorCategoryPermission},
		{"operation not permitted", "operation not permitted: bind to port 443", ErrorCategoryPermission},
		{"eacces", "write file: eacces", ErrorCategoryPermission},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := errors.New(tt.errMsg)
			bErr := ClassifyError(err, PhaseInstall)

			if bErr.Category != tt.wantCat {
				t.Errorf("Category = %v, want %v", bErr.Category, tt.wantCat)
			}

			if bErr.Severity != SeverityCritical {
				t.Errorf("Severity = %v, want %v", bErr.Severity, SeverityCritical)
			}

			if len(bErr.RecoveryActions) == 0 {
				t.Error("Expected recovery actions for permission error")
			}
		})
	}
}

func TestClassifyError_Network(t *testing.T) {
	tests := []struct {
		name    string
		errMsg  string
		wantCat ErrorCategory
	}{
		{"connection refused", "dial tcp 127.0.0.1:5432: connection refused", ErrorCategoryNetwork},
		{"no route to host", "dial tcp: no route to host", ErrorCategoryNetwork},
		{"network unreachable", "network unreachable", ErrorCategoryNetwork},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := errors.New(tt.errMsg)
			bErr := ClassifyError(err, PhaseInstall)

			if bErr.Category != tt.wantCat {
				t.Errorf("Category = %v, want %v", bErr.Category, tt.wantCat)
			}

			if !bErr.IsRetryable() {
				t.Error("Network errors should be retryable")
			}
		})
	}
}

func TestClassifyError_Database(t *testing.T) {
	tests := []struct {
		name    string
		errMsg  string
		wantCat ErrorCategory
	}{
		{"postgres connection", "postgres: connection refused", ErrorCategoryDatabase},
		{"database auth", "pq: password authentication failed", ErrorCategoryDatabase},
		{"database not exist", "database 'kscore' does not exist", ErrorCategoryDatabase},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := errors.New(tt.errMsg)
			bErr := ClassifyError(err, PhaseInstall)

			if bErr.Category != tt.wantCat {
				t.Errorf("Category = %v, want %v", bErr.Category, tt.wantCat)
			}
		})
	}
}

func TestClassifyError_TLS(t *testing.T) {
	tests := []struct {
		name    string
		errMsg  string
		wantCat ErrorCategory
	}{
		{"certificate expired", "x509: certificate has expired", ErrorCategoryTLS},
		{"self signed", "x509: certificate signed by unknown authority (self signed)", ErrorCategoryTLS},
		{"tls handshake", "tls: handshake failure", ErrorCategoryTLS},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := errors.New(tt.errMsg)
			bErr := ClassifyError(err, PhaseInstall)

			if bErr.Category != tt.wantCat {
				t.Errorf("Category = %v, want %v", bErr.Category, tt.wantCat)
			}
		})
	}
}

func TestClassifyError_Resource(t *testing.T) {
	tests := []struct {
		name    string
		errMsg  string
		wantCat ErrorCategory
		wantSev ErrorSeverity
	}{
		{"no space left", "write /var/lib/keystone-core/data: no space left on device", ErrorCategoryResource, SeverityCritical},
		{"disk full", "disk full: cannot write", ErrorCategoryResource, SeverityCritical},
		{"out of memory", "out of memory", ErrorCategoryResource, SeverityCritical},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := errors.New(tt.errMsg)
			bErr := ClassifyError(err, PhaseInstall)

			if bErr.Category != tt.wantCat {
				t.Errorf("Category = %v, want %v", bErr.Category, tt.wantCat)
			}

			if bErr.Severity != tt.wantSev {
				t.Errorf("Severity = %v, want %v", bErr.Severity, tt.wantSev)
			}
		})
	}
}

func TestClassifyError_Timeout(t *testing.T) {
	err := errors.New("context deadline exceeded")
	bErr := ClassifyError(err, PhaseInstall)

	if bErr.Category != ErrorCategoryTimeout {
		t.Errorf("Category = %v, want %v", bErr.Category, ErrorCategoryTimeout)
	}

	if !bErr.IsRetryable() {
		t.Error("Timeout errors should be retryable")
	}

	// Should have retry action
	hasRetryAction := false
	for _, action := range bErr.RecoveryActions {
		if action.ID == "retry-operation" {
			hasRetryAction = true
			break
		}
	}
	if !hasRetryAction {
		t.Error("Expected retry-operation action for timeout error")
	}
}

func TestClassifyError_Package(t *testing.T) {
	tests := []struct {
		name    string
		errMsg  string
		wantCat ErrorCategory
	}{
		{"apt not found", "apt: unable to locate package kscore-server", ErrorCategoryPackage},
		{"dpkg lock", "dpkg: error: dpkg status database is locked", ErrorCategoryPackage},
		{"dnf error", "dnf: error downloading packages", ErrorCategoryPackage},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := errors.New(tt.errMsg)
			bErr := ClassifyError(err, PhaseInstall)

			if bErr.Category != tt.wantCat {
				t.Errorf("Category = %v, want %v", bErr.Category, tt.wantCat)
			}
		})
	}
}

func TestClassifyError_Service(t *testing.T) {
	err := errors.New("systemctl: failed to start kscore-server.service")
	bErr := ClassifyError(err, PhaseInstall)

	if bErr.Category != ErrorCategoryService {
		t.Errorf("Category = %v, want %v", bErr.Category, ErrorCategoryService)
	}

	if !bErr.IsRetryable() {
		t.Error("Service errors should be retryable")
	}
}

func TestClassifyError_Config(t *testing.T) {
	tests := []struct {
		name    string
		errMsg  string
		wantCat ErrorCategory
	}{
		{"required field", "postgres-host is required for postgres storage", ErrorCategoryConfig},
		{"missing value", "missing nats urls", ErrorCategoryConfig},
		{"invalid config", "invalid storage type: must be sqlite or postgres", ErrorCategoryConfig},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := errors.New(tt.errMsg)
			bErr := ClassifyError(err, PhaseValidate)

			if bErr.Category != tt.wantCat {
				t.Errorf("Category = %v, want %v", bErr.Category, tt.wantCat)
			}
		})
	}
}

func TestClassifyError_Unknown(t *testing.T) {
	err := errors.New("some unexpected error occurred")
	bErr := ClassifyError(err, PhaseInstall)

	if bErr.Category != ErrorCategoryUnknown {
		t.Errorf("Category = %v, want %v", bErr.Category, ErrorCategoryUnknown)
	}
}

func TestClassifyError_PreservesPhase(t *testing.T) {
	err := errors.New("test error")

	phases := []PhaseName{PhaseDetect, PhaseConfigure, PhaseValidate, PhaseInstall, PhaseVerify}
	for _, phase := range phases {
		bErr := ClassifyError(err, phase)
		if bErr.Phase != phase {
			t.Errorf("Phase = %v, want %v", bErr.Phase, phase)
		}
	}
}

func TestClassifyError_NilError(t *testing.T) {
	bErr := ClassifyError(nil, PhaseInstall)
	if bErr != nil {
		t.Error("ClassifyError(nil) should return nil")
	}
}

func TestClassifyError_AlreadyBootstrapError(t *testing.T) {
	original := &Error{
		Category: ErrorCategoryDatabase,
		Severity: SeverityCritical,
		Message:  "original error",
	}

	bErr := ClassifyError(original, PhaseInstall)

	if bErr.Category != ErrorCategoryDatabase {
		t.Errorf("Should preserve original category: got %v, want %v", bErr.Category, ErrorCategoryDatabase)
	}

	if bErr.Phase != PhaseInstall {
		t.Errorf("Should update phase: got %v, want %v", bErr.Phase, PhaseInstall)
	}
}

func TestBootstrapError_Error(t *testing.T) {
	bErr := &Error{
		Message: "test error message",
	}

	if bErr.Error() != "test error message" {
		t.Errorf("Error() = %v, want 'test error message'", bErr.Error())
	}

	// Test with no message but original error
	bErr2 := &Error{
		Original: errors.New("original error"),
	}

	if bErr2.Error() != "original error" {
		t.Errorf("Error() = %v, want 'original error'", bErr2.Error())
	}

	// Test with neither
	bErr3 := &Error{}
	if bErr3.Error() != "bootstrap error" {
		t.Errorf("Error() = %v, want 'bootstrap error'", bErr3.Error())
	}
}

func TestBootstrapError_Unwrap(t *testing.T) {
	original := errors.New("original error")
	bErr := &Error{
		Original: original,
	}

	if !errors.Is(bErr.Unwrap(), original) {
		t.Error("Unwrap() should return original error")
	}
}

func TestBootstrapError_IsRetryable(t *testing.T) {
	retryableCategories := []ErrorCategory{
		ErrorCategoryNetwork,
		ErrorCategoryTimeout,
		ErrorCategoryService,
	}

	nonRetryableCategories := []ErrorCategory{
		ErrorCategoryPermission,
		ErrorCategoryConfig,
		ErrorCategoryFileSystem,
		ErrorCategoryResource,
		ErrorCategoryUnknown,
	}

	for _, cat := range retryableCategories {
		bErr := &Error{Category: cat}
		if !bErr.IsRetryable() {
			t.Errorf("Category %v should be retryable", cat)
		}
	}

	for _, cat := range nonRetryableCategories {
		bErr := &Error{Category: cat}
		if bErr.IsRetryable() {
			t.Errorf("Category %v should not be retryable", cat)
		}
	}
}

func TestBootstrapError_HasAutomaticRecovery(t *testing.T) {
	bErr := &Error{
		RecoveryActions: []RecoveryAction{
			{Type: RecoveryTypeManual},
			{Type: RecoveryTypeInteractive},
		},
	}

	if bErr.HasAutomaticRecovery() {
		t.Error("Should return false when no automatic actions")
	}

	bErr.RecoveryActions = append(bErr.RecoveryActions, RecoveryAction{Type: RecoveryTypeAutomatic})
	if !bErr.HasAutomaticRecovery() {
		t.Error("Should return true when automatic action exists")
	}
}

func TestBootstrapError_GetAutomaticRecoveryActions(t *testing.T) {
	bErr := &Error{
		RecoveryActions: []RecoveryAction{
			{ID: "manual-1", Type: RecoveryTypeManual},
			{ID: "auto-1", Type: RecoveryTypeAutomatic},
			{ID: "interactive-1", Type: RecoveryTypeInteractive},
			{ID: "auto-2", Type: RecoveryTypeAutomatic},
		},
	}

	autoActions := bErr.GetAutomaticRecoveryActions()
	if len(autoActions) != 2 {
		t.Errorf("Expected 2 automatic actions, got %d", len(autoActions))
	}

	for _, action := range autoActions {
		if action.Type != RecoveryTypeAutomatic {
			t.Errorf("Action %s should be automatic", action.ID)
		}
	}
}

func TestFormatRecoveryActions(t *testing.T) {
	actions := []RecoveryAction{
		{
			ID:              "test-action",
			Description:     "Test recovery action",
			Type:            RecoveryTypeManual,
			Risk:            RiskLow,
			Command:         "echo 'test'",
			ExpectedOutcome: "Test succeeds",
		},
	}

	// Non-verbose
	output := FormatRecoveryActions(actions, false)
	if output == "" {
		t.Error("Expected non-empty output")
	}

	if !contains(output, "Test recovery action") {
		t.Error("Output should contain action description")
	}

	if !contains(output, "manual") {
		t.Error("Output should contain action type")
	}

	// Verbose
	verboseOutput := FormatRecoveryActions(actions, true)
	if !contains(verboseOutput, "Test succeeds") {
		t.Error("Verbose output should contain expected outcome")
	}

	// Empty actions
	emptyOutput := FormatRecoveryActions(nil, false)
	if emptyOutput != "" {
		t.Error("Empty actions should produce empty output")
	}
}

func TestFormatRecoveryActions_MultipleCommands(t *testing.T) {
	actions := []RecoveryAction{
		{
			ID:          "multi-cmd",
			Description: "Multiple commands",
			Type:        RecoveryTypeManual,
			Risk:        RiskMedium,
			Commands: []string{
				"command1",
				"command2",
				"command3",
			},
		},
	}

	output := FormatRecoveryActions(actions, false)

	if !contains(output, "command1") {
		t.Error("Output should contain all commands")
	}
	if !contains(output, "command2") {
		t.Error("Output should contain all commands")
	}
}

func TestContainsAny(t *testing.T) {
	tests := []struct {
		s      string
		subs   []string
		expect bool
	}{
		{"hello world", []string{"hello"}, true},
		{"hello world", []string{"foo", "world"}, true},
		{"hello world", []string{"foo", "bar"}, false},
		{"", []string{"foo"}, false},
		{"hello", []string{}, false},
	}

	for _, tt := range tests {
		result := containsAny(tt.s, tt.subs...)
		if result != tt.expect {
			t.Errorf("containsAny(%q, %v) = %v, want %v", tt.s, tt.subs, result, tt.expect)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
