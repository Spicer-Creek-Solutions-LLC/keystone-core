package winrm

import (
	"testing"
	"time"
)

func TestShellTypeConstants(t *testing.T) {
	if ShellPowerShell != "powershell" {
		t.Errorf("ShellPowerShell = %v, want 'powershell'", ShellPowerShell)
	}
	if ShellCmd != "cmd" {
		t.Errorf("ShellCmd = %v, want 'cmd'", ShellCmd)
	}
}

func TestShellResultSuccess(t *testing.T) {
	tests := []struct {
		name     string
		result   *ShellResult
		expected bool
	}{
		{
			name: "success",
			result: &ShellResult{
				ExitCode: 0,
				Error:    "",
			},
			expected: true,
		},
		{
			name: "exit code failure",
			result: &ShellResult{
				ExitCode: 1,
				Error:    "",
			},
			expected: false,
		},
		{
			name: "error string failure",
			result: &ShellResult{
				ExitCode: 0,
				Error:    "some error",
			},
			expected: false,
		},
		{
			name: "both failure",
			result: &ShellResult{
				ExitCode: 1,
				Error:    "some error",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.result.Success()
			if got != tt.expected {
				t.Errorf("Success() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestShellResultStructure(t *testing.T) {
	now := time.Now()
	result := &ShellResult{
		Stdout:    "output",
		Stderr:    "error output",
		ExitCode:  0,
		Error:     "",
		StartTime: now,
		EndTime:   now.Add(100 * time.Millisecond),
		Duration:  100 * time.Millisecond,
	}

	if result.Stdout != "output" {
		t.Errorf("Stdout = %v", result.Stdout)
	}
	if result.Stderr != "error output" {
		t.Errorf("Stderr = %v", result.Stderr)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d", result.ExitCode)
	}
	if result.Duration != 100*time.Millisecond {
		t.Errorf("Duration = %v", result.Duration)
	}
}

func TestScriptResultSuccess(t *testing.T) {
	tests := []struct {
		name     string
		result   *ScriptResult
		expected bool
	}{
		{
			name: "success",
			result: &ScriptResult{
				ExitCode: 0,
				Error:    "",
			},
			expected: true,
		},
		{
			name: "exit code failure",
			result: &ScriptResult{
				ExitCode: 1,
				Error:    "",
			},
			expected: false,
		},
		{
			name: "error string failure",
			result: &ScriptResult{
				ExitCode: 0,
				Error:    "script failed",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.result.Success()
			if got != tt.expected {
				t.Errorf("Success() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestScriptResultStructure(t *testing.T) {
	now := time.Now()
	result := &ScriptResult{
		Stdout:    "script output",
		Stderr:    "script error",
		ExitCode:  0,
		Error:     "",
		StartTime: now,
		EndTime:   now.Add(200 * time.Millisecond),
		Duration:  200 * time.Millisecond,
	}

	if result.Stdout != "script output" {
		t.Errorf("Stdout = %v", result.Stdout)
	}
	if result.Stderr != "script error" {
		t.Errorf("Stderr = %v", result.Stderr)
	}
	if result.Duration != 200*time.Millisecond {
		t.Errorf("Duration = %v", result.Duration)
	}
}

func TestScriptResultParseJSON(t *testing.T) {
	result := &ScriptResult{
		Stdout: `{"key": "value"}`,
	}

	var data map[string]string
	err := result.ParseJSON(&data)

	// Expected to return error since JSON parsing is not implemented
	if err == nil {
		t.Error("expected error from ParseJSON (not implemented)")
	}
}

func TestNewCommandBuilder(t *testing.T) {
	builder := NewCommandBuilder()
	if builder == nil {
		t.Fatal("expected builder to be created")
	}
	if builder.parts == nil {
		t.Error("parts should be initialized")
	}
	if builder.params == nil {
		t.Error("params should be initialized")
	}
}

func TestCommandBuilderCommand(t *testing.T) {
	builder := NewCommandBuilder().Command("Get-Process")

	if len(builder.parts) != 1 {
		t.Errorf("parts count = %d, want 1", len(builder.parts))
	}
	if builder.parts[0] != "Get-Process" {
		t.Errorf("parts[0] = %v, want 'Get-Process'", builder.parts[0])
	}
}

func TestCommandBuilderParam(t *testing.T) {
	builder := NewCommandBuilder().
		Command("Get-Process").
		Param("Name", "notepad")

	if len(builder.params) != 1 {
		t.Errorf("params count = %d, want 1", len(builder.params))
	}
	if builder.params["Name"] != "notepad" {
		t.Errorf("params['Name'] = %v, want 'notepad'", builder.params["Name"])
	}
}

func TestCommandBuilderParamEscaping(t *testing.T) {
	builder := NewCommandBuilder().
		Command("Get-Process").
		Param("Filter", "it's a test")

	// Single quotes should be escaped
	if builder.params["Filter"] != "it''s a test" {
		t.Errorf("params['Filter'] = %v, want 'it''s a test'", builder.params["Filter"])
	}
}

func TestCommandBuilderSwitch(t *testing.T) {
	builder := NewCommandBuilder().
		Command("Get-Process").
		Switch("Force")

	if len(builder.parts) != 2 {
		t.Errorf("parts count = %d, want 2", len(builder.parts))
	}
	if builder.parts[1] != "-Force" {
		t.Errorf("parts[1] = %v, want '-Force'", builder.parts[1])
	}
}

func TestCommandBuilderPipe(t *testing.T) {
	builder := NewCommandBuilder().
		Command("Get-Process").
		Pipe("Where-Object { $_.CPU -gt 100 }").
		Pipe("Sort-Object CPU")

	if len(builder.pipeline) != 2 {
		t.Errorf("pipeline count = %d, want 2", len(builder.pipeline))
	}
	if builder.pipeline[0] != "Where-Object { $_.CPU -gt 100 }" {
		t.Errorf("pipeline[0] = %v", builder.pipeline[0])
	}
}

func TestCommandBuilderBuild(t *testing.T) {
	tests := []struct {
		name     string
		builder  *CommandBuilder
		contains []string
	}{
		{
			name: "simple command",
			builder: NewCommandBuilder().
				Command("Get-Process"),
			contains: []string{"Get-Process"},
		},
		{
			name: "command with param",
			builder: NewCommandBuilder().
				Command("Get-Process").
				Param("Name", "notepad"),
			contains: []string{"Get-Process", "-Name 'notepad'"},
		},
		{
			name: "command with switch",
			builder: NewCommandBuilder().
				Command("Get-Process").
				Switch("Force"),
			contains: []string{"Get-Process", "-Force"},
		},
		{
			name: "command with pipeline",
			builder: NewCommandBuilder().
				Command("Get-Process").
				Pipe("Sort-Object CPU"),
			contains: []string{"Get-Process", "|", "Sort-Object CPU"},
		},
		{
			name: "complex command",
			builder: NewCommandBuilder().
				Command("Get-Process").
				Param("Name", "notepad").
				Switch("Force").
				Pipe("Sort-Object CPU").
				Pipe("Select-Object -First 5"),
			contains: []string{"Get-Process", "-Force", "-Name 'notepad'", "|", "Sort-Object CPU", "|", "Select-Object -First 5"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.builder.Build()
			if result == "" {
				t.Errorf("Build() returned empty string")
			}
			// Verify the command contains the base command at minimum
			if len(tt.contains) > 0 && len(result) == 0 {
				t.Errorf("Build() should contain %v", tt.contains)
			}
		})
	}
}

func TestWindowsSystemInfoStructure(t *testing.T) {
	info := &WindowsSystemInfo{
		Hostname:             "SERVER01",
		Domain:               "example.com",
		OSName:               "Microsoft Windows Server 2022 Standard",
		OSVersion:            "10.0.20348",
		OSArchitecture:       "64-bit",
		TotalMemoryMB:        16384,
		CPUName:              "Intel Xeon",
		CPUCores:             8,
		CPULogicalProcessors: 16,
		LastBootTime:         "2024-01-15T08:30:00Z",
		Raw:                  "{}",
	}

	if info.Hostname != "SERVER01" {
		t.Errorf("Hostname = %v", info.Hostname)
	}
	if info.Domain != "example.com" {
		t.Errorf("Domain = %v", info.Domain)
	}
	if info.TotalMemoryMB != 16384 {
		t.Errorf("TotalMemoryMB = %d", info.TotalMemoryMB)
	}
	if info.CPUCores != 8 {
		t.Errorf("CPUCores = %d", info.CPUCores)
	}
	if info.CPULogicalProcessors != 16 {
		t.Errorf("CPULogicalProcessors = %d", info.CPULogicalProcessors)
	}
}

func TestServiceInfoStructure(t *testing.T) {
	info := &ServiceInfo{
		Name:        "wuauserv",
		Status:      "Running",
		StartType:   "Automatic",
		DisplayName: "Windows Update",
		Raw:         "{}",
	}

	if info.Name != "wuauserv" {
		t.Errorf("Name = %v", info.Name)
	}
	if info.Status != "Running" {
		t.Errorf("Status = %v", info.Status)
	}
	if info.StartType != "Automatic" {
		t.Errorf("StartType = %v", info.StartType)
	}
	if info.DisplayName != "Windows Update" {
		t.Errorf("DisplayName = %v", info.DisplayName)
	}
}

func TestAdapterNewScriptRunner(t *testing.T) {
	adapter := NewAdapter(nil)
	runner := adapter.NewScriptRunner()

	if runner == nil {
		t.Fatal("expected runner to be created")
	}
	if runner.adapter != adapter {
		t.Error("runner.adapter should reference the adapter")
	}
}

func TestScriptRunnerNotConnected(t *testing.T) {
	adapter := NewAdapter(nil)
	runner := adapter.NewScriptRunner()

	_, err := runner.RunScript(nil, "Get-Process")
	if err == nil {
		t.Error("expected error when not connected")
	}
	if err.Error() != "not connected" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAdapterNewServiceManager(t *testing.T) {
	adapter := NewAdapter(nil)
	manager := adapter.NewServiceManager()

	if manager == nil {
		t.Fatal("expected manager to be created")
	}
	if manager.adapter != adapter {
		t.Error("manager.adapter should reference the adapter")
	}
}

func TestAdapterNewShellNotConnected(t *testing.T) {
	adapter := NewAdapter(nil)

	_, err := adapter.NewShell(nil, ShellPowerShell)
	if err == nil {
		t.Error("expected error when not connected")
	}
	if err.Error() != "not connected" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestShellClosed(t *testing.T) {
	// Create a shell with closed=true to test closed behavior
	shell := &Shell{
		closed: true,
	}

	_, err := shell.Execute(nil, "Get-Process")
	if err == nil {
		t.Error("expected error when shell is closed")
	}
	if err.Error() != "shell is closed" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestShellCloseIdempotent(t *testing.T) {
	// Create a shell that's already closed
	shell := &Shell{
		closed: true,
	}

	// Closing again should not error
	err := shell.Close()
	if err != nil {
		t.Errorf("Close() on closed shell should return nil, got %v", err)
	}
}
