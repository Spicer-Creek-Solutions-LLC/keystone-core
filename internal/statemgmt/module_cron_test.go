// Package statemgmt provides state management modules.
// Additional cron module tests (non-duplicate tests from module_scheduled_test.go)
package statemgmt

import (
	"testing"
)

// =============================================================================
// Additional CronModule Tests (not covered in module_scheduled_test.go)
// =============================================================================

func TestCronModule_ParseCronConfig_WithEnvironment(t *testing.T) {
	m := NewCronModule()
	decl := &StateDeclaration{
		ID:     "env-job",
		Module: "cron",
		State:  "present",
		Parameters: map[string]interface{}{
			"command": "/usr/local/bin/test.sh",
			"environment": map[string]interface{}{
				"PATH":  "/usr/local/bin:/usr/bin:/bin",
				"SHELL": "/bin/bash",
			},
		},
	}

	config, err := m.parseCronConfig(decl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(config.Environment) != 2 {
		t.Errorf("expected 2 environment variables, got %d", len(config.Environment))
	}
	if config.Environment["PATH"] != "/usr/local/bin:/usr/bin:/bin" {
		t.Errorf("expected PATH='/usr/local/bin:/usr/bin:/bin', got '%s'", config.Environment["PATH"])
	}
	if config.Environment["SHELL"] != "/bin/bash" {
		t.Errorf("expected SHELL='/bin/bash', got '%s'", config.Environment["SHELL"])
	}
}

func TestCronModule_ParseCronConfig_Disabled(t *testing.T) {
	m := NewCronModule()
	decl := &StateDeclaration{
		ID:     "disabled-job",
		Module: "cron",
		State:  "present",
		Parameters: map[string]interface{}{
			"command":  "/usr/local/bin/test.sh",
			"disabled": true,
		},
	}

	config, err := m.parseCronConfig(decl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !config.Disabled {
		t.Error("expected Disabled=true")
	}
}

func TestCronModule_ParseCronConfig_AllTimeFields(t *testing.T) {
	m := NewCronModule()
	decl := &StateDeclaration{
		ID:     "full-config",
		Module: "cron",
		State:  "present",
		Parameters: map[string]interface{}{
			"command": "/usr/local/bin/test.sh",
			"minute":  "30",
			"hour":    "5",
			"day":     "15",
			"month":   "6",
			"weekday": "1",
		},
	}

	config, err := m.parseCronConfig(decl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if config.Minute != "30" {
		t.Errorf("expected Minute='30', got '%s'", config.Minute)
	}
	if config.Hour != "5" {
		t.Errorf("expected Hour='5', got '%s'", config.Hour)
	}
	if config.Day != "15" {
		t.Errorf("expected Day='15', got '%s'", config.Day)
	}
	if config.Month != "6" {
		t.Errorf("expected Month='6', got '%s'", config.Month)
	}
	if config.Weekday != "1" {
		t.Errorf("expected Weekday='1', got '%s'", config.Weekday)
	}
}

func TestCronModule_FindEntry(t *testing.T) {
	m := NewCronModule()
	entries := []string{
		"0 3 * * * /usr/local/bin/backup.sh # Keystone Core: backup-job",
		"0 * * * * /usr/local/bin/hourly.sh # Keystone Core: hourly-job",
		"# This is a comment",
		"0 5 * * * /some/other/script.sh",
	}

	tests := []struct {
		name         string
		configName   string
		expectExists bool
	}{
		{"find existing backup", "backup-job", true},
		{"find existing hourly", "hourly-job", true},
		{"not find missing", "missing-job", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &CronConfig{
				Name:    tt.configName,
				Minute:  "0",
				Hour:    "3",
				Day:     "*",
				Month:   "*",
				Weekday: "*",
				Command: "/usr/local/bin/backup.sh",
			}
			exists, _ := m.findEntry(entries, config)
			if exists != tt.expectExists {
				t.Errorf("findEntry() exists = %v, want %v", exists, tt.expectExists)
			}
		})
	}
}

func TestCronModule_FindEntry_Matches(t *testing.T) {
	m := NewCronModule()
	entries := []string{
		"0 3 * * * /usr/local/bin/backup.sh # Keystone Core: backup-job",
	}

	config := &CronConfig{
		Name:    "backup-job",
		Minute:  "0",
		Hour:    "3",
		Day:     "*",
		Month:   "*",
		Weekday: "*",
		Command: "/usr/local/bin/backup.sh",
	}

	exists, matches := m.findEntry(entries, config)
	if !exists {
		t.Error("expected exists=true")
	}
	if !matches {
		t.Error("expected matches=true for identical entry")
	}
}

func TestCronModule_FindEntry_ExistsButDifferent(t *testing.T) {
	m := NewCronModule()
	entries := []string{
		"0 3 * * * /usr/local/bin/backup.sh # Keystone Core: backup-job",
	}

	config := &CronConfig{
		Name:    "backup-job",
		Minute:  "30", // Different minute
		Hour:    "3",
		Day:     "*",
		Month:   "*",
		Weekday: "*",
		Command: "/usr/local/bin/backup.sh",
	}

	exists, matches := m.findEntry(entries, config)
	if !exists {
		t.Error("expected exists=true (found by marker)")
	}
	if matches {
		t.Error("expected matches=false for different entry")
	}
}

func TestCronModule_RemoveEntry_NotFound(t *testing.T) {
	m := NewCronModule()
	entries := []string{
		"0 3 * * * /usr/local/bin/backup.sh # Keystone Core: backup-job",
	}

	result := m.removeEntry(entries, "nonexistent-job")
	if len(result) != len(entries) {
		t.Errorf("expected %d entries (unchanged), got %d", len(entries), len(result))
	}
}

func TestCronConfig_Defaults(t *testing.T) {
	m := NewCronModule()
	decl := &StateDeclaration{
		ID:     "minimal-job",
		Module: "cron",
		State:  "present",
		Parameters: map[string]interface{}{
			"command": "/usr/local/bin/test.sh",
		},
	}

	config, err := m.parseCronConfig(decl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check defaults are applied
	if config.Minute != "*" {
		t.Errorf("expected default Minute='*', got '%s'", config.Minute)
	}
	if config.Hour != "*" {
		t.Errorf("expected default Hour='*', got '%s'", config.Hour)
	}
	if config.Day != "*" {
		t.Errorf("expected default Day='*', got '%s'", config.Day)
	}
	if config.Month != "*" {
		t.Errorf("expected default Month='*', got '%s'", config.Month)
	}
	if config.Weekday != "*" {
		t.Errorf("expected default Weekday='*', got '%s'", config.Weekday)
	}
	if config.User != "" {
		t.Errorf("expected default User='', got '%s'", config.User)
	}
	if config.Disabled {
		t.Error("expected default Disabled=false")
	}
}
