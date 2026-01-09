package files

import (
	"testing"
)

func TestNamespaceFromPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "standard path",
			path:     "/packages/nginx/1.24.deb",
			expected: "packages",
		},
		{
			name:     "configs namespace",
			path:     "/configs/app/production.yaml",
			expected: "configs",
		},
		{
			name:     "empty path",
			path:     "",
			expected: "",
		},
		{
			name:     "path without leading slash",
			path:     "packages/nginx/1.24.deb",
			expected: "packages",
		},
		{
			name:     "single element path",
			path:     "/packages",
			expected: "packages",
		},
		{
			name:     "single element without slash",
			path:     "packages",
			expected: "packages",
		},
		{
			name:     "root only",
			path:     "/",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NamespaceFromPath(tt.path)
			if result != tt.expected {
				t.Errorf("NamespaceFromPath(%q) = %q, want %q", tt.path, result, tt.expected)
			}
		})
	}
}

func TestValidatePath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{
			name:    "valid path",
			path:    "/packages/nginx/1.24.deb",
			wantErr: false,
		},
		{
			name:    "valid nested path",
			path:    "/configs/app/env/production.yaml",
			wantErr: false,
		},
		{
			name:    "empty path",
			path:    "",
			wantErr: true,
		},
		{
			name:    "no leading slash",
			path:    "packages/nginx",
			wantErr: true,
		},
		{
			name:    "path traversal",
			path:    "/packages/../etc/passwd",
			wantErr: true,
		},
		{
			name:    "path traversal at start",
			path:    "/../etc/passwd",
			wantErr: true,
		},
		{
			name:    "path traversal at end",
			path:    "/packages/..",
			wantErr: true,
		},
		{
			name:    "double dots in filename (ok)",
			path:    "/packages/file..ext",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePath(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
			}
		})
	}
}

func TestFileErrorInterface(t *testing.T) {
	err := &FileError{
		RequestID: "test-123",
		Code:      ErrCodeNotFound,
		Message:   "file not found",
	}

	// Test that FileError implements error interface
	var _ error = err

	if err.Error() != "file not found" {
		t.Errorf("FileError.Error() = %q, want %q", err.Error(), "file not found")
	}
}

func TestAckStatus(t *testing.T) {
	tests := []struct {
		status AckStatus
		valid  bool
	}{
		{AckStatusComplete, true},
		{AckStatusPartial, true},
		{AckStatusError, true},
		{AckStatusRetry, true},
		{"invalid", false},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			valid := tt.status == AckStatusComplete ||
				tt.status == AckStatusPartial ||
				tt.status == AckStatusError ||
				tt.status == AckStatusRetry
			if valid != tt.valid {
				t.Errorf("AckStatus %q validity = %v, want %v", tt.status, valid, tt.valid)
			}
		})
	}
}

func TestFileErrorCodes(t *testing.T) {
	codes := []FileErrorCode{
		ErrCodeNotFound,
		ErrCodeAccessDenied,
		ErrCodeInvalidRequest,
		ErrCodeBackendError,
		ErrCodeTimeout,
		ErrCodeChecksumFailed,
		ErrCodeQuotaExceeded,
		ErrCodeFileTooLarge,
		ErrCodeInternal,
	}

	seen := make(map[FileErrorCode]bool)
	for _, code := range codes {
		if seen[code] {
			t.Errorf("Duplicate error code: %s", code)
		}
		seen[code] = true

		if code == "" {
			t.Error("Empty error code")
		}
	}
}

func TestFileAction(t *testing.T) {
	tests := []struct {
		action FileAction
		valid  bool
	}{
		{FileActionCreated, true},
		{FileActionUpdated, true},
		{FileActionDeleted, true},
		{"invalid", false},
	}

	for _, tt := range tests {
		t.Run(string(tt.action), func(t *testing.T) {
			valid := tt.action == FileActionCreated ||
				tt.action == FileActionUpdated ||
				tt.action == FileActionDeleted
			if valid != tt.valid {
				t.Errorf("FileAction %q validity = %v, want %v", tt.action, valid, tt.valid)
			}
		})
	}
}

func TestConstants(t *testing.T) {
	// Verify default values are reasonable
	if DefaultChunkSize <= 0 {
		t.Errorf("DefaultChunkSize should be positive, got %d", DefaultChunkSize)
	}
	if MaxChunkSize < DefaultChunkSize {
		t.Errorf("MaxChunkSize (%d) should be >= DefaultChunkSize (%d)", MaxChunkSize, DefaultChunkSize)
	}
	if DefaultMaxFileSize <= 0 {
		t.Errorf("DefaultMaxFileSize should be positive, got %d", DefaultMaxFileSize)
	}
	if DefaultCacheSize <= 0 {
		t.Errorf("DefaultCacheSize should be positive, got %d", DefaultCacheSize)
	}
	if DefaultCacheTTL <= 0 {
		t.Errorf("DefaultCacheTTL should be positive, got %v", DefaultCacheTTL)
	}
	if DefaultRetryAttempts <= 0 {
		t.Errorf("DefaultRetryAttempts should be positive, got %d", DefaultRetryAttempts)
	}
	if DefaultRetryDelay <= 0 {
		t.Errorf("DefaultRetryDelay should be positive, got %v", DefaultRetryDelay)
	}
}

func TestPriority(t *testing.T) {
	// Verify priority order
	if PriorityNormal >= PriorityHigh {
		t.Error("PriorityNormal should be less than PriorityHigh")
	}
	if PriorityHigh >= PriorityCritical {
		t.Error("PriorityHigh should be less than PriorityCritical")
	}
}
