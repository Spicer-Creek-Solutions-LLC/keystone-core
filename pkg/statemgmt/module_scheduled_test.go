// Package statemgmt provides state management module tests.
package statemgmt

import (
	"strings"
	"testing"
)

// Helper to create a StateDeclaration from params
func makeDecl(id, state string, params map[string]interface{}) *StateDeclaration {
	return &StateDeclaration{
		ID:         id,
		State:      state,
		Parameters: params,
	}
}

// ==================== Cron Module Tests ====================

func TestNewCronModule(t *testing.T) {
	m := NewCronModule()
	if m == nil {
		t.Fatal("expected non-nil module")
	}
	if m.Name() != "cron" {
		t.Errorf("expected name 'cron', got %q", m.Name())
	}
}

func TestCronModule_ParseConfig(t *testing.T) {
	m := NewCronModule()

	tests := []struct {
		name    string
		id      string
		params  map[string]interface{}
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid basic config",
			id:   "backup",
			params: map[string]interface{}{
				"command": "/usr/bin/backup.sh",
				"minute":  "0",
				"hour":    "2",
			},
			wantErr: false,
		},
		{
			name: "valid with special schedule",
			id:   "startup_task",
			params: map[string]interface{}{
				"command": "/usr/bin/startup.sh",
				"special": "@reboot",
			},
			wantErr: false,
		},
		{
			name: "valid with user",
			id:   "user_job",
			params: map[string]interface{}{
				"command": "/home/user/script.sh",
				"user":    "deploy",
				"minute":  "*/5",
			},
			wantErr: false,
		},
		{
			name: "missing command",
			id:   "test",
			params: map[string]interface{}{
				"minute": "0",
			},
			wantErr: true,
			errMsg:  "command is required",
		},
		{
			name: "invalid special schedule",
			id:   "bad_special",
			params: map[string]interface{}{
				"command": "/usr/bin/test.sh",
				"special": "@invalid",
			},
			wantErr: true,
			errMsg:  "invalid special schedule",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decl := makeDecl(tt.id, "present", tt.params)
			config, err := m.parseCronConfig(decl)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errMsg)
				} else if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errMsg, err.Error())
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if config == nil {
				t.Error("expected non-nil config")
			}
		})
	}
}

func TestCronModule_BuildEntry(t *testing.T) {
	m := NewCronModule()

	tests := []struct {
		name     string
		config   *CronConfig
		contains []string
	}{
		{
			name: "basic entry",
			config: &CronConfig{
				Name:    "backup",
				Minute:  "0",
				Hour:    "2",
				Day:     "*",
				Month:   "*",
				Weekday: "*",
				Command: "/usr/bin/backup.sh",
			},
			contains: []string{"0 2 * * *", "/usr/bin/backup.sh", "# Keystone Core: backup"},
		},
		{
			name: "special entry",
			config: &CronConfig{
				Name:    "startup",
				Special: "@reboot",
				Command: "/usr/bin/startup.sh",
			},
			contains: []string{"@reboot", "/usr/bin/startup.sh", "# Keystone Core: startup"},
		},
		{
			name: "disabled entry",
			config: &CronConfig{
				Name:     "disabled_job",
				Minute:   "0",
				Hour:     "0",
				Day:      "*",
				Month:    "*",
				Weekday:  "*",
				Command:  "/usr/bin/test.sh",
				Disabled: true,
			},
			contains: []string{"#0 0 * * *"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := m.buildEntry(tt.config)
			for _, c := range tt.contains {
				if !strings.Contains(entry, c) {
					t.Errorf("expected entry to contain %q, got %q", c, entry)
				}
			}
		})
	}
}

func TestCronModule_RemoveEntry(t *testing.T) {
	m := NewCronModule()

	entries := []string{
		"# Some other entry",
		"0 * * * * /usr/bin/other.sh # Keystone Core: other",
		"0 2 * * * /usr/bin/backup.sh # Keystone Core: backup",
		"# Another entry",
	}

	result := m.removeEntry(entries, "backup")

	if len(result) != 3 {
		t.Errorf("expected 3 entries after removal, got %d", len(result))
	}

	for _, entry := range result {
		if strings.Contains(entry, "Keystone Core: backup") {
			t.Error("expected backup entry to be removed")
		}
	}
}

func TestIsValidSpecial(t *testing.T) {
	valid := []string{"@reboot", "@hourly", "@daily", "@midnight", "@weekly", "@monthly", "@yearly", "@annually"}
	for _, s := range valid {
		if !isValidSpecial(s) {
			t.Errorf("expected %q to be valid", s)
		}
	}

	invalid := []string{"@invalid", "reboot", "@hourly2", ""}
	for _, s := range invalid {
		if isValidSpecial(s) {
			t.Errorf("expected %q to be invalid", s)
		}
	}
}

// ==================== Systemd Timer Module Tests ====================

func TestNewSystemdTimerModule(t *testing.T) {
	m := NewSystemdTimerModule()
	if m == nil {
		t.Fatal("expected non-nil module")
	}
	if m.Name() != "systemd_timer" {
		t.Errorf("expected name 'systemd_timer', got %q", m.Name())
	}
}

func TestSystemdTimerModule_ParseConfig(t *testing.T) {
	m := NewSystemdTimerModule()

	tests := []struct {
		name    string
		id      string
		params  map[string]interface{}
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid with on_calendar",
			id:   "backup",
			params: map[string]interface{}{
				"on_calendar": "daily",
				"exec_start":  "/usr/bin/backup.sh",
			},
			wantErr: false,
		},
		{
			name: "valid with on_boot_sec",
			id:   "startup",
			params: map[string]interface{}{
				"on_boot_sec": "5min",
				"command":     "/usr/bin/startup.sh",
			},
			wantErr: false,
		},
		{
			name: "valid with multiple triggers",
			id:   "complex",
			params: map[string]interface{}{
				"on_calendar":        "*-*-* 00:00:00",
				"on_unit_active_sec": "1h",
				"exec_start":         "/usr/bin/test.sh",
			},
			wantErr: false,
		},
		{
			name: "missing command",
			id:   "test",
			params: map[string]interface{}{
				"on_calendar": "daily",
			},
			wantErr: true,
			errMsg:  "exec_start or command is required",
		},
		{
			name: "missing trigger",
			id:   "no_trigger",
			params: map[string]interface{}{
				"exec_start": "/usr/bin/test.sh",
			},
			wantErr: true,
			errMsg:  "at least one trigger is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decl := makeDecl(tt.id, "present", tt.params)
			config, err := m.parseConfig(decl)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errMsg)
				} else if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errMsg, err.Error())
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if config == nil {
				t.Error("expected non-nil config")
			}
		})
	}
}

func TestSystemdTimerModule_GenerateTimerUnit(t *testing.T) {
	m := NewSystemdTimerModule()

	config := &SystemdTimerConfig{
		Name:              "backup",
		Description:       "Daily backup timer",
		OnCalendar:        "daily",
		AccuracySec:       "1min",
		Persistent:        true,
		RemainAfterElapse: true,
	}

	unit := m.generateTimerUnit(config)

	expected := []string{
		"[Unit]",
		"Description=Daily backup timer",
		"[Timer]",
		"OnCalendar=daily",
		"AccuracySec=1min",
		"Persistent=true",
		"[Install]",
		"WantedBy=timers.target",
	}

	for _, exp := range expected {
		if !strings.Contains(unit, exp) {
			t.Errorf("expected unit to contain %q", exp)
		}
	}
}

func TestSystemdTimerModule_GenerateServiceUnit(t *testing.T) {
	m := NewSystemdTimerModule()

	config := &SystemdTimerConfig{
		Name:             "backup",
		Description:      "Backup service",
		ExecStart:        "/usr/bin/backup.sh",
		Type:             "oneshot",
		User:             "backup",
		WorkingDirectory: "/var/backup",
	}

	unit := m.generateServiceUnit(config)

	expected := []string{
		"[Unit]",
		"Description=Backup service (service)",
		"[Service]",
		"Type=oneshot",
		"ExecStart=/usr/bin/backup.sh",
		"User=backup",
		"WorkingDirectory=/var/backup",
	}

	for _, exp := range expected {
		if !strings.Contains(unit, exp) {
			t.Errorf("expected unit to contain %q", exp)
		}
	}
}

// ==================== Launchd Module Tests ====================

func TestNewLaunchdModule(t *testing.T) {
	m := NewLaunchdModule()
	if m == nil {
		t.Fatal("expected non-nil module")
	}
	if m.Name() != "launchd" {
		t.Errorf("expected name 'launchd', got %q", m.Name())
	}
}

func TestLaunchdModule_ParseConfig(t *testing.T) {
	m := NewLaunchdModule()

	tests := []struct {
		name    string
		id      string
		params  map[string]interface{}
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid with program",
			id:   "com.example.test",
			params: map[string]interface{}{
				"program":     "/usr/bin/test",
				"run_at_load": true,
			},
			wantErr: false,
		},
		{
			name: "valid with program_arguments",
			id:   "com.example.test",
			params: map[string]interface{}{
				"program_arguments": []interface{}{"/usr/bin/test", "--arg"},
				"start_interval":    300,
			},
			wantErr: false,
		},
		{
			name: "valid with calendar interval",
			id:   "com.example.daily",
			params: map[string]interface{}{
				"program": "/usr/bin/daily.sh",
				"start_calendar_interval": map[string]interface{}{
					"hour":   2,
					"minute": 0,
				},
			},
			wantErr: false,
		},
		{
			name:   "missing program (uses ID as label)",
			id:     "com.example.test",
			params: map[string]interface{}{},
			wantErr: true,
			errMsg:  "either program or program_arguments is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decl := makeDecl(tt.id, "present", tt.params)
			config, err := m.parseConfig(decl)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errMsg)
				} else if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errMsg, err.Error())
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if config == nil {
				t.Error("expected non-nil config")
			}
		})
	}
}

func TestLaunchdModule_GeneratePlist(t *testing.T) {
	m := NewLaunchdModule()

	config := &LaunchdConfig{
		Label:         "com.example.test",
		Program:       "/usr/bin/test",
		RunAtLoad:     true,
		KeepAlive:     true,
		StartInterval: 300,
	}

	plist := m.generatePlist(config)

	expected := []string{
		`<?xml version="1.0" encoding="UTF-8"?>`,
		"<plist version=\"1.0\">",
		"<key>Label</key>",
		"<string>com.example.test</string>",
		"<key>Program</key>",
		"<string>/usr/bin/test</string>",
		"<key>RunAtLoad</key>",
		"<true/>",
		"<key>KeepAlive</key>",
		"<true/>",
		"<key>StartInterval</key>",
		"<integer>300</integer>",
		"</plist>",
	}

	for _, exp := range expected {
		if !strings.Contains(plist, exp) {
			t.Errorf("expected plist to contain %q", exp)
		}
	}
}

func TestXmlEscape(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello", "hello"},
		{"<script>", "&lt;script&gt;"},
		{"a & b", "a &amp; b"},
		{`"quoted"`, "&#34;quoted&#34;"},
		{"normal text", "normal text"},
	}

	for _, tt := range tests {
		result := xmlEscape(tt.input)
		if result != tt.expected {
			t.Errorf("xmlEscape(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

// ==================== Scheduled Task Module Tests ====================

func TestNewScheduledTaskModule(t *testing.T) {
	m := NewScheduledTaskModule()
	if m == nil {
		t.Fatal("expected non-nil module")
	}
	if m.Name() != "scheduled_task" {
		t.Errorf("expected name 'scheduled_task', got %q", m.Name())
	}
}

func TestScheduledTaskModule_ParseConfig(t *testing.T) {
	m := NewScheduledTaskModule()

	tests := []struct {
		name    string
		id      string
		params  map[string]interface{}
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid daily task",
			id:   "DailyBackup",
			params: map[string]interface{}{
				"execute":      "C:\\backup.bat",
				"trigger_type": "daily",
				"start_time":   "02:00",
			},
			wantErr: false,
		},
		{
			name: "valid weekly task",
			id:   "WeeklyReport",
			params: map[string]interface{}{
				"execute":      "C:\\report.exe",
				"trigger_type": "weekly",
				"days_of_week": []interface{}{"MON", "FRI"},
			},
			wantErr: false,
		},
		{
			name: "valid at_startup task",
			id:   "StartupTask",
			params: map[string]interface{}{
				"execute":      "C:\\startup.bat",
				"trigger_type": "at_startup",
			},
			wantErr: false,
		},
		{
			name: "missing execute",
			id:   "test",
			params: map[string]interface{}{
				"trigger_type": "daily",
			},
			wantErr: true,
			errMsg:  "execute is required",
		},
		{
			name: "missing trigger_type",
			id:   "test",
			params: map[string]interface{}{
				"execute": "C:\\test.bat",
			},
			wantErr: true,
			errMsg:  "trigger_type is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decl := makeDecl(tt.id, "present", tt.params)
			config, err := m.parseConfig(decl)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errMsg)
				} else if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errMsg, err.Error())
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if config == nil {
				t.Error("expected non-nil config")
			}
		})
	}
}

func TestScheduledTaskModule_BuildTriggerArgs(t *testing.T) {
	m := NewScheduledTaskModule()

	tests := []struct {
		name     string
		config   *ScheduledTaskConfig
		expected []string
	}{
		{
			name: "daily trigger",
			config: &ScheduledTaskConfig{
				TriggerType:  "daily",
				DaysInterval: 1,
				StartTime:    "02:00",
			},
			expected: []string{"/SC", "DAILY", "/ST", "02:00"},
		},
		{
			name: "weekly trigger with days",
			config: &ScheduledTaskConfig{
				TriggerType:   "weekly",
				WeeksInterval: 2,
				DaysOfWeek:    []string{"MON", "FRI"},
				StartTime:     "10:00",
			},
			expected: []string{"/SC", "WEEKLY", "/MO", "2", "/D", "MON,FRI", "/ST", "10:00"},
		},
		{
			name: "at_startup trigger",
			config: &ScheduledTaskConfig{
				TriggerType: "at_startup",
			},
			expected: []string{"/SC", "ONSTART"},
		},
		{
			name: "at_logon trigger with delay",
			config: &ScheduledTaskConfig{
				TriggerType: "at_logon",
				Delay:       "0000:30",
			},
			expected: []string{"/SC", "ONLOGON", "/DELAY", "0000:30"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := m.buildTriggerArgs(tt.config)
			for _, exp := range tt.expected {
				found := false
				for _, arg := range args {
					if arg == exp {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected args to contain %q, got %v", exp, args)
				}
			}
		})
	}
}

func TestScheduledTaskModule_GetTaskName(t *testing.T) {
	m := NewScheduledTaskModule()

	tests := []struct {
		config   *ScheduledTaskConfig
		expected string
	}{
		{
			config:   &ScheduledTaskConfig{Name: "Test", TaskPath: "\\"},
			expected: "\\Test",
		},
		{
			config:   &ScheduledTaskConfig{Name: "Test", TaskPath: "\\MyTasks"},
			expected: "\\MyTasks\\Test",
		},
		{
			config:   &ScheduledTaskConfig{Name: "Test", TaskPath: "\\MyTasks\\"},
			expected: "\\MyTasks\\Test",
		},
	}

	for _, tt := range tests {
		result := m.getTaskName(tt.config)
		if result != tt.expected {
			t.Errorf("getTaskName() = %q, want %q", result, tt.expected)
		}
	}
}

// ==================== At Module Tests ====================

func TestNewAtModule(t *testing.T) {
	m := NewAtModule()
	if m == nil {
		t.Fatal("expected non-nil module")
	}
	if m.Name() != "at" {
		t.Errorf("expected name 'at', got %q", m.Name())
	}
}

func TestAtModule_ParseConfig(t *testing.T) {
	m := NewAtModule()

	tests := []struct {
		name    string
		id      string
		params  map[string]interface{}
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid basic config",
			id:   "one_time_job",
			params: map[string]interface{}{
				"command": "/usr/bin/task.sh",
				"time":    "10:00",
			},
			wantErr: false,
		},
		{
			name: "valid with relative time",
			id:   "delayed_job",
			params: map[string]interface{}{
				"command": "/usr/bin/task.sh",
				"time":    "now + 1 hour",
			},
			wantErr: false,
		},
		{
			name: "valid with date",
			id:   "scheduled_job",
			params: map[string]interface{}{
				"command": "/usr/bin/task.sh",
				"time":    "10:00",
				"date":    "tomorrow",
			},
			wantErr: false,
		},
		{
			name: "valid with queue",
			id:   "queued_job",
			params: map[string]interface{}{
				"command": "/usr/bin/task.sh",
				"time":    "noon",
				"queue":   "b",
			},
			wantErr: false,
		},
		{
			name: "missing command",
			id:   "test",
			params: map[string]interface{}{
				"time": "10:00",
			},
			wantErr: true,
			errMsg:  "command is required",
		},
		{
			name: "missing time",
			id:   "test",
			params: map[string]interface{}{
				"command": "/usr/bin/task.sh",
			},
			wantErr: true,
			errMsg:  "time is required",
		},
		{
			name: "invalid queue",
			id:   "test",
			params: map[string]interface{}{
				"command": "/usr/bin/task.sh",
				"time":    "10:00",
				"queue":   "abc",
			},
			wantErr: true,
			errMsg:  "queue must be a single letter",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decl := makeDecl(tt.id, "present", tt.params)
			config, err := m.parseConfig(decl)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errMsg)
				} else if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errMsg, err.Error())
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if config == nil {
				t.Error("expected non-nil config")
			}
		})
	}
}

func TestNormalizeWhitespace(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello  world", "hello world"},
		{"  spaces  ", "spaces"},
		{"tab\there", "tab here"},
		{"multiple   spaces   here", "multiple spaces here"},
		{"", ""},
	}

	for _, tt := range tests {
		result := normalizeWhitespace(tt.input)
		if result != tt.expected {
			t.Errorf("normalizeWhitespace(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}
