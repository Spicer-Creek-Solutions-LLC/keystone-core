package backend

import (
	"testing"
)

func TestBackendTypes(t *testing.T) {
	types := []BackendType{
		BackendTypeFilesystem,
		BackendTypeS3,
		BackendTypeGCS,
		BackendTypeAzure,
		BackendTypeNATSObject,
		BackendTypeGit,
		BackendTypeHTTP,
	}

	seen := make(map[BackendType]bool)
	for _, bt := range types {
		if seen[bt] {
			t.Errorf("Duplicate backend type: %s", bt)
		}
		seen[bt] = true

		if bt == "" {
			t.Error("Empty backend type")
		}
	}
}

func TestMatchGlob(t *testing.T) {
	tests := []struct {
		pattern string
		path    string
		match   bool
	}{
		// Exact match
		{"foo", "foo", true},
		{"foo", "bar", false},

		// Single wildcard
		{"*.txt", "file.txt", true},
		{"*.txt", "file.doc", false},
		{"foo/*", "foo/bar", true},
		{"foo/*", "foo/bar/baz", false},
		{"*/bar", "foo/bar", true},

		// Double wildcard
		{"**", "anything", true},
		{"**", "foo/bar/baz", true},
		{"foo/**", "foo/bar", true},
		{"foo/**", "foo/bar/baz", true},
		{"**/baz", "foo/bar/baz", true},
		{"**/baz", "baz", true},

		// Combined patterns
		{"foo/*/bar", "foo/x/bar", true},
		{"foo/*/bar", "foo/x/y/bar", false},
		{"foo/**/bar", "foo/x/y/bar", true},
		{"*.txt", "foo/bar.txt", false},
		{"**/*.txt", "foo/bar.txt", true},
		{"**/*.txt", "foo/bar/baz.txt", true},

		// Edge cases
		{"", "", true},
		{"", "anything", false},
		{"*", "", true},
		{"**", "", true},
		{"/packages/**", "/packages/nginx/1.24.deb", true},
		{"/configs/**", "/configs/app/production.yaml", true},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.path, func(t *testing.T) {
			result := matchGlob(tt.pattern, tt.path)
			if result != tt.match {
				t.Errorf("matchGlob(%q, %q) = %v, want %v", tt.pattern, tt.path, result, tt.match)
			}
		})
	}
}

func TestConfigMatchesPath(t *testing.T) {
	tests := []struct {
		name    string
		paths   []string
		path    string
		matches bool
	}{
		{
			name:    "single pattern match",
			paths:   []string{"/packages/**"},
			path:    "/packages/nginx/1.24.deb",
			matches: true,
		},
		{
			name:    "single pattern no match",
			paths:   []string{"/packages/**"},
			path:    "/configs/app.yaml",
			matches: false,
		},
		{
			name:    "multiple patterns first match",
			paths:   []string{"/packages/**", "/configs/**"},
			path:    "/packages/nginx/1.24.deb",
			matches: true,
		},
		{
			name:    "multiple patterns second match",
			paths:   []string{"/packages/**", "/configs/**"},
			path:    "/configs/app.yaml",
			matches: true,
		},
		{
			name:    "multiple patterns no match",
			paths:   []string{"/packages/**", "/configs/**"},
			path:    "/other/file.txt",
			matches: false,
		},
		{
			name:    "empty paths",
			paths:   []string{},
			path:    "/packages/nginx/1.24.deb",
			matches: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &Config{Paths: tt.paths}
			result := config.MatchesPath(tt.path)
			if result != tt.matches {
				t.Errorf("MatchesPath(%q) = %v, want %v", tt.path, result, tt.matches)
			}
		})
	}
}

func TestIsNotFound(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "ErrNotFound",
			err:      ErrNotFound,
			expected: true,
		},
		{
			name:     "wrapped not found",
			err:      &BackendError{Op: "get", Err: errNotFound{}},
			expected: true,
		},
		{
			name:     "access denied",
			err:      ErrAccessDenied,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsNotFound(tt.err)
			if result != tt.expected {
				t.Errorf("IsNotFound(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

func TestBackendErrorError(t *testing.T) {
	tests := []struct {
		name     string
		err      *BackendError
		expected string
	}{
		{
			name:     "with path",
			err:      &BackendError{Backend: "filesystem", Op: "get", Path: "/test/file.txt", Err: errNotFound{}},
			expected: "filesystem: get /test/file.txt: file not found",
		},
		{
			name:     "without path",
			err:      &BackendError{Backend: "s3", Op: "list", Err: errAccessDenied{}},
			expected: "s3: list: access denied",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.err.Error()
			if result != tt.expected {
				t.Errorf("BackendError.Error() = %q, want %q", result, tt.expected)
			}
		})
	}
}
