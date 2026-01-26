package deprecation

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func TestDeprecateCommand(t *testing.T) {
	registry := &Registry{warnings: make(map[string]bool)}

	cmd := &cobra.Command{
		Use:   "oldcmd",
		Short: "An old command",
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}

	info := &Info{
		DeprecatedIn:   "0.30.0",
		RemoveIn:       "1.0.0",
		Replacement:    "newcmd",
		MigrationGuide: "https://docs.example.com/migrate",
	}

	DeprecateCommandWithRegistry(cmd, info, registry)

	// Check annotations were set
	if cmd.Annotations["deprecated"] != "true" {
		t.Error("expected deprecated annotation to be 'true'")
	}
	if cmd.Annotations["deprecated_in"] != "0.30.0" {
		t.Errorf("expected deprecated_in to be '0.30.0', got '%s'", cmd.Annotations["deprecated_in"])
	}
	if cmd.Annotations["remove_in"] != "1.0.0" {
		t.Errorf("expected remove_in to be '1.0.0', got '%s'", cmd.Annotations["remove_in"])
	}
	if cmd.Annotations["replacement"] != "newcmd" {
		t.Errorf("expected replacement to be 'newcmd', got '%s'", cmd.Annotations["replacement"])
	}

	// Check Cobra's deprecated field was set
	if cmd.Deprecated == "" {
		t.Error("expected Deprecated field to be set")
	}
}

func TestFormatWarning(t *testing.T) {
	info := &Info{
		DeprecatedIn:   "0.30.0",
		RemoveIn:       "1.0.0",
		Replacement:    "kscore-blueprint-publish publish",
		MigrationGuide: "https://docs.example.com/migrate",
		Message:        "This command has been moved to a new location.",
	}

	warning := FormatWarning("kscore-blueprint publish", info)

	// Check warning contains expected content
	expectations := []string{
		"DEPRECATION WARNING",
		"kscore-blueprint publish",
		"0.30.0",
		"1.0.0",
		"kscore-blueprint-publish publish",
		"https://docs.example.com/migrate",
		"This command has been moved",
	}

	for _, expected := range expectations {
		if !strings.Contains(warning, expected) {
			t.Errorf("expected warning to contain '%s', got:\n%s", expected, warning)
		}
	}
}

func TestRegistryWarningTracking(t *testing.T) {
	registry := &Registry{warnings: make(map[string]bool)}

	cmdPath := "test-command"

	if registry.HasWarned(cmdPath) {
		t.Error("expected HasWarned to return false for new command")
	}

	registry.MarkWarned(cmdPath)

	if !registry.HasWarned(cmdPath) {
		t.Error("expected HasWarned to return true after marking")
	}

	registry.Reset()

	if registry.HasWarned(cmdPath) {
		t.Error("expected HasWarned to return false after reset")
	}
}

func TestRegistrySilent(t *testing.T) {
	registry := &Registry{warnings: make(map[string]bool)}

	if registry.IsSilent() {
		t.Error("expected IsSilent to return false by default")
	}

	registry.SetSilent(true)

	if !registry.IsSilent() {
		t.Error("expected IsSilent to return true after setting")
	}
}

func TestIsDeprecated(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		expected    bool
	}{
		{
			name:        "no annotations",
			annotations: nil,
			expected:    false,
		},
		{
			name:        "empty annotations",
			annotations: map[string]string{},
			expected:    false,
		},
		{
			name:        "deprecated true",
			annotations: map[string]string{"deprecated": "true"},
			expected:    true,
		},
		{
			name:        "deprecated false",
			annotations: map[string]string{"deprecated": "false"},
			expected:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "test"}
			cmd.Annotations = tt.annotations

			if got := IsDeprecated(cmd); got != tt.expected {
				t.Errorf("IsDeprecated() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestGetDeprecationInfo(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Annotations = map[string]string{
		"deprecated":    "true",
		"deprecated_in": "0.30.0",
		"remove_in":     "1.0.0",
		"replacement":   "new-test",
	}

	info := GetDeprecationInfo(cmd)
	if info == nil {
		t.Fatal("expected non-nil info")
	}

	if info.DeprecatedIn != "0.30.0" {
		t.Errorf("expected DeprecatedIn '0.30.0', got '%s'", info.DeprecatedIn)
	}
	if info.RemoveIn != "1.0.0" {
		t.Errorf("expected RemoveIn '1.0.0', got '%s'", info.RemoveIn)
	}
	if info.Replacement != "new-test" {
		t.Errorf("expected Replacement 'new-test', got '%s'", info.Replacement)
	}
}

func TestCheckRemovalDate(t *testing.T) {
	tests := []struct {
		name            string
		removalDate     time.Time
		expectWarning   bool
		expectDaysRange [2]int // min, max expected days
	}{
		{
			name:          "zero date",
			removalDate:   time.Time{},
			expectWarning: false,
		},
		{
			name:            "far future",
			removalDate:     time.Now().AddDate(0, 6, 0), // 6 months
			expectWarning:   false,
			expectDaysRange: [2]int{150, 200},
		},
		{
			name:            "approaching (15 days)",
			removalDate:     time.Now().AddDate(0, 0, 15),
			expectWarning:   true,
			expectDaysRange: [2]int{14, 16},
		},
		{
			name:            "past",
			removalDate:     time.Now().AddDate(0, 0, -5),
			expectWarning:   true,
			expectDaysRange: [2]int{-6, -4},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			approaching, days := CheckRemovalDate(tt.removalDate)

			if approaching != tt.expectWarning {
				t.Errorf("CheckRemovalDate() approaching = %v, want %v", approaching, tt.expectWarning)
			}

			if !tt.removalDate.IsZero() {
				if days < tt.expectDaysRange[0] || days > tt.expectDaysRange[1] {
					t.Errorf("CheckRemovalDate() days = %d, want between %d and %d",
						days, tt.expectDaysRange[0], tt.expectDaysRange[1])
				}
			}
		})
	}
}

func TestMigrations(t *testing.T) {
	m := &Migrations{mappings: make(map[string]string)}

	// Register some migrations
	m.Register("kscore-blueprint publish", "kscore-blueprint-publish publish")
	m.Register("kscore-identity federation list", "kscore-federation list")

	// Test GetReplacement
	replacement, ok := m.GetReplacement("kscore-blueprint publish")
	if !ok {
		t.Error("expected to find replacement")
	}
	if replacement != "kscore-blueprint-publish publish" {
		t.Errorf("expected 'kscore-blueprint-publish publish', got '%s'", replacement)
	}

	// Test non-existent
	_, ok = m.GetReplacement("nonexistent")
	if ok {
		t.Error("expected not to find replacement for nonexistent command")
	}

	// Test All
	all := m.All()
	if len(all) != 2 {
		t.Errorf("expected 2 migrations, got %d", len(all))
	}
}

func TestAddAlias(t *testing.T) {
	parent := &cobra.Command{Use: "parent"}

	info := &Info{
		DeprecatedIn: "0.30.0",
		RemoveIn:     "1.0.0",
		Replacement:  "newcmd subcommand",
	}

	alias := AddAlias(parent, "oldsubcmd", "newcmd subcommand", info)

	// Check alias was added to parent
	found := false
	for _, cmd := range parent.Commands() {
		if cmd.Name() == "oldsubcmd" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected alias to be added to parent")
	}

	// Check alias has correct properties
	if alias.Short == "" {
		t.Error("expected alias to have Short description")
	}
	if !strings.Contains(alias.Short, "Deprecated") {
		t.Error("expected alias Short to mention 'Deprecated'")
	}
}

func TestDeprecateCommandShowsWarning(t *testing.T) {
	// Capture stderr
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	registry := &Registry{warnings: make(map[string]bool)}

	cmd := &cobra.Command{
		Use: "testcmd",
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}

	info := &Info{
		DeprecatedIn: "0.30.0",
		RemoveIn:     "1.0.0",
		Replacement:  "newcmd",
	}

	DeprecateCommandWithRegistry(cmd, info, registry)

	// Execute the command to trigger PreRunE
	cmd.SetArgs([]string{})
	_ = cmd.Execute()

	// Close write end and read output
	w.Close()
	var buf bytes.Buffer
	buf.ReadFrom(r)
	os.Stderr = oldStderr

	output := buf.String()
	if !strings.Contains(output, "DEPRECATION WARNING") {
		t.Errorf("expected deprecation warning in output, got:\n%s", output)
	}
}

func TestDeprecateCommandSilentMode(t *testing.T) {
	// Capture stderr
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	registry := &Registry{warnings: make(map[string]bool)}
	registry.SetSilent(true)

	cmd := &cobra.Command{
		Use: "testcmd",
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}

	info := &Info{
		DeprecatedIn: "0.30.0",
		RemoveIn:     "1.0.0",
		Replacement:  "newcmd",
	}

	DeprecateCommandWithRegistry(cmd, info, registry)

	// Execute the command
	cmd.SetArgs([]string{})
	_ = cmd.Execute()

	// Close write end and read output
	w.Close()
	var buf bytes.Buffer
	buf.ReadFrom(r)
	os.Stderr = oldStderr

	output := buf.String()
	if strings.Contains(output, "DEPRECATION WARNING") {
		t.Error("expected no deprecation warning in silent mode")
	}
}

func TestFormatShortDeprecation(t *testing.T) {
	tests := []struct {
		name     string
		info     *Info
		expected string
	}{
		{
			name:     "with replacement",
			info:     &Info{Replacement: "newcmd"},
			expected: "use 'newcmd' instead",
		},
		{
			name:     "with removal version",
			info:     &Info{RemoveIn: "1.0.0"},
			expected: "will be removed in v1.0.0",
		},
		{
			name:     "with v prefix in removal version",
			info:     &Info{RemoveIn: "v1.0.0"},
			expected: "will be removed in v1.0.0",
		},
		{
			name:     "empty info",
			info:     &Info{},
			expected: "this command is deprecated",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatShortDeprecation(tt.info)
			if got != tt.expected {
				t.Errorf("formatShortDeprecation() = %q, want %q", got, tt.expected)
			}
		})
	}
}
