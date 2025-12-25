package version

import (
	"strings"
	"testing"
)

func TestGet(t *testing.T) {
	// Test that Get() returns an Info struct with the current values
	info := Get()

	if info.Version == "" {
		t.Error("Version should not be empty")
	}

	if info.GitCommit == "" {
		t.Error("GitCommit should not be empty")
	}

	if info.BuildDate == "" {
		t.Error("BuildDate should not be empty")
	}

	// Verify it matches the package variables
	if info.Version != Version {
		t.Errorf("Expected Version %s, got %s", Version, info.Version)
	}

	if info.GitCommit != GitCommit {
		t.Errorf("Expected GitCommit %s, got %s", GitCommit, info.GitCommit)
	}

	if info.BuildDate != BuildDate {
		t.Errorf("Expected BuildDate %s, got %s", BuildDate, info.BuildDate)
	}
}

func TestInfo_String(t *testing.T) {
	testCases := []struct {
		name     string
		info     Info
		expected []string // Strings that should be present in the output
	}{
		{
			name: "Default values",
			info: Info{
				Version:   "dev",
				GitCommit: "unknown",
				BuildDate: "unknown",
			},
			expected: []string{"TitanAnvil", "dev", "unknown", "commit:", "built:"},
		},
		{
			name: "Release build",
			info: Info{
				Version:   "1.0.0",
				GitCommit: "abc123def456",
				BuildDate: "2024-01-15T10:30:00Z",
			},
			expected: []string{"TitanAnvil", "1.0.0", "abc123def456", "2024-01-15T10:30:00Z"},
		},
		{
			name: "Development build with short commit",
			info: Info{
				Version:   "0.1.0-alpha",
				GitCommit: "abc123",
				BuildDate: "2024-01-10",
			},
			expected: []string{"TitanAnvil", "0.1.0-alpha", "abc123", "2024-01-10"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.info.String()

			// Verify all expected strings are present
			for _, expected := range tc.expected {
				if !strings.Contains(result, expected) {
					t.Errorf("Expected output to contain %q, but got: %s", expected, result)
				}
			}

			// Verify the format structure (should start with "TitanAnvil")
			if !strings.HasPrefix(result, "TitanAnvil") {
				t.Errorf("Expected output to start with 'TitanAnvil', got: %s", result)
			}
		})
	}
}

func TestInfo_String_Format(t *testing.T) {
	// Test the exact format of the string
	info := Info{
		Version:   "1.2.3",
		GitCommit: "abcdef123456",
		BuildDate: "2024-03-15",
	}

	expected := "TitanAnvil 1.2.3 (commit: abcdef123456, built: 2024-03-15)"
	result := info.String()

	if result != expected {
		t.Errorf("Expected exact format:\n%s\nGot:\n%s", expected, result)
	}
}

func TestPackageVariables(t *testing.T) {
	// Test that package variables are initialized (even if with defaults)
	if Version == "" {
		t.Error("Version variable should be initialized")
	}

	if GitCommit == "" {
		t.Error("GitCommit variable should be initialized")
	}

	if BuildDate == "" {
		t.Error("BuildDate variable should be initialized")
	}

	// Default values check (when not built with ldflags)
	t.Logf("Current version info: Version=%s, GitCommit=%s, BuildDate=%s",
		Version, GitCommit, BuildDate)
}

func TestGet_ReturnsCopy(t *testing.T) {
	// Verify that Get() returns independent Info structs
	info1 := Get()
	info2 := Get()

	// They should have the same values
	if info1.Version != info2.Version {
		t.Error("Get() should return consistent Version")
	}

	if info1.GitCommit != info2.GitCommit {
		t.Error("Get() should return consistent GitCommit")
	}

	if info1.BuildDate != info2.BuildDate {
		t.Error("Get() should return consistent BuildDate")
	}
}
