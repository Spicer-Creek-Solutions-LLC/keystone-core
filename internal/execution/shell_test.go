package execution

import (
	"os/exec"
	"runtime"
	"strings"
	"testing"
)

func TestGetShell(t *testing.T) {
	tests := []struct {
		name      string
		shellType ShellType
		wantType  ShellType
		wantErr   bool
	}{
		{
			name:      "bash",
			shellType: ShellTypeBash,
			wantType:  ShellTypeBash,
			wantErr:   false,
		},
		{
			name:      "sh",
			shellType: ShellTypeSh,
			wantType:  ShellTypeSh,
			wantErr:   false,
		},
		{
			name:      "powershell",
			shellType: ShellTypePowershell,
			wantType:  ShellTypePowershell,
			wantErr:   false,
		},
		{
			name:      "cmd",
			shellType: ShellTypeCmd,
			wantType:  ShellTypeCmd,
			wantErr:   false,
		},
		{
			name:      "default",
			shellType: ShellTypeDefault,
			wantErr:   false, // Type will vary by OS
		},
		{
			name:      "invalid",
			shellType: "invalid",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shell, err := GetShell(tt.shellType)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetShell() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil && tt.wantType != "" && shell.Type() != tt.wantType {
				t.Errorf("GetShell() type = %v, want %v", shell.Type(), tt.wantType)
			}
		})
	}
}

func TestGetDefaultShell(t *testing.T) {
	shell, err := GetDefaultShell()
	if err != nil {
		t.Fatalf("GetDefaultShell() error = %v", err)
	}

	if shell == nil {
		t.Fatal("GetDefaultShell() returned nil shell")
	}

	// Verify we got a valid shell for this OS
	switch runtime.GOOS {
	case "windows":
		shellType := shell.Type()
		if shellType != ShellTypePowershell && shellType != ShellTypeCmd {
			t.Errorf("Expected PowerShell or Cmd on Windows, got %v", shellType)
		}
	case "darwin", "linux", "freebsd", "openbsd", "netbsd":
		shellType := shell.Type()
		if shellType != ShellTypeBash && shellType != ShellTypeSh {
			t.Errorf("Expected Bash or Sh on Unix, got %v", shellType)
		}
	}

	// Verify the shell is available
	if !shell.IsAvailable() {
		t.Errorf("Default shell %s is not available", shell.Name())
	}
}

func TestBashShell(t *testing.T) {
	shell := &BashShell{}

	if shell.Name() != "bash" {
		t.Errorf("Name() = %v, want bash", shell.Name())
	}

	if shell.Type() != ShellTypeBash {
		t.Errorf("Type() = %v, want %v", shell.Type(), ShellTypeBash)
	}

	cmd, args := shell.Command("echo hello")
	if cmd != "bash" {
		t.Errorf("Command() cmd = %v, want bash", cmd)
	}
	if len(args) != 2 || args[0] != "-c" || args[1] != "echo hello" {
		t.Errorf("Command() args = %v, want [-c, echo hello]", args)
	}

	if shell.EnvVarSeparator() != ":" {
		t.Errorf("EnvVarSeparator() = %v, want :", shell.EnvVarSeparator())
	}

	// IsAvailable depends on the system
	available := shell.IsAvailable()
	_, err := exec.LookPath("bash")
	expectedAvailable := (err == nil)
	if available != expectedAvailable {
		t.Errorf("IsAvailable() = %v, want %v", available, expectedAvailable)
	}
}

func TestShShell(t *testing.T) {
	shell := &ShShell{}

	if shell.Name() != "sh" {
		t.Errorf("Name() = %v, want sh", shell.Name())
	}

	if shell.Type() != ShellTypeSh {
		t.Errorf("Type() = %v, want %v", shell.Type(), ShellTypeSh)
	}

	cmd, args := shell.Command("ls -la")
	if cmd != "sh" {
		t.Errorf("Command() cmd = %v, want sh", cmd)
	}
	if len(args) != 2 || args[0] != "-c" || args[1] != "ls -la" {
		t.Errorf("Command() args = %v, want [-c, ls -la]", args)
	}

	if shell.EnvVarSeparator() != ":" {
		t.Errorf("EnvVarSeparator() = %v, want :", shell.EnvVarSeparator())
	}
}

func TestPowershellShell(t *testing.T) {
	shell := &PowershellShell{}

	if shell.Name() != "powershell" {
		t.Errorf("Name() = %v, want powershell", shell.Name())
	}

	if shell.Type() != ShellTypePowershell {
		t.Errorf("Type() = %v, want %v", shell.Type(), ShellTypePowershell)
	}

	cmd, args := shell.Command("Get-Process")
	if cmd != "powershell" {
		t.Errorf("Command() cmd = %v, want powershell", cmd)
	}
	if len(args) != 3 || args[0] != "-NoProfile" || args[1] != "-Command" || args[2] != "Get-Process" {
		t.Errorf("Command() args = %v, want [-NoProfile, -Command, Get-Process]", args)
	}

	if shell.EnvVarSeparator() != ";" {
		t.Errorf("EnvVarSeparator() = %v, want ;", shell.EnvVarSeparator())
	}
}

func TestCmdShell(t *testing.T) {
	shell := &CmdShell{}

	if shell.Name() != "cmd" {
		t.Errorf("Name() = %v, want cmd", shell.Name())
	}

	if shell.Type() != ShellTypeCmd {
		t.Errorf("Type() = %v, want %v", shell.Type(), ShellTypeCmd)
	}

	cmd, args := shell.Command("dir")
	if cmd != "cmd" {
		t.Errorf("Command() cmd = %v, want cmd", cmd)
	}
	if len(args) != 2 || args[0] != "/C" || args[1] != "dir" {
		t.Errorf("Command() args = %v, want [/C, dir]", args)
	}

	if shell.EnvVarSeparator() != ";" {
		t.Errorf("EnvVarSeparator() = %v, want ;", shell.EnvVarSeparator())
	}

	// Cmd is available on Windows
	available := shell.IsAvailable()
	expectedAvailable := (runtime.GOOS == "windows")
	if available != expectedAvailable {
		t.Errorf("IsAvailable() = %v, want %v", available, expectedAvailable)
	}
}

// TestShellExecution tests actual shell execution (only on Unix-like systems)
func TestShellExecution(t *testing.T) {
	// Skip on Windows for now (would need different test commands)
	if runtime.GOOS == "windows" {
		t.Skip("Skipping shell execution test on Windows")
	}

	tests := []struct {
		name      string
		shellType ShellType
		script    string
		wantOut   string
	}{
		{
			name:      "bash echo",
			shellType: ShellTypeBash,
			script:    "echo 'hello from bash'",
			wantOut:   "hello from bash",
		},
		{
			name:      "sh echo",
			shellType: ShellTypeSh,
			script:    "echo 'hello from sh'",
			wantOut:   "hello from sh",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shell, err := GetShell(tt.shellType)
			if err != nil {
				t.Fatalf("GetShell() error = %v", err)
			}

			if !shell.IsAvailable() {
				t.Skipf("Shell %s not available on this system", shell.Name())
			}

			cmd, args := shell.Command(tt.script)
			execCmd := exec.Command(cmd, args...)
			output, err := execCmd.CombinedOutput()
			if err != nil {
				t.Fatalf("Command execution failed: %v, output: %s", err, output)
			}

			outputStr := strings.TrimSpace(string(output))
			if outputStr != tt.wantOut {
				t.Errorf("Command output = %q, want %q", outputStr, tt.wantOut)
			}
		})
	}
}

// TestShellVariableExpansion tests that shells properly expand variables
func TestShellVariableExpansion(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping variable expansion test on Windows")
	}

	tests := []struct {
		name      string
		shellType ShellType
		script    string
		wantOut   string
	}{
		{
			name:      "bash variable",
			shellType: ShellTypeBash,
			script:    "VAR=test; echo $VAR",
			wantOut:   "test",
		},
		{
			name:      "sh variable",
			shellType: ShellTypeSh,
			script:    "VAR=test; echo $VAR",
			wantOut:   "test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shell, err := GetShell(tt.shellType)
			if err != nil {
				t.Fatalf("GetShell() error = %v", err)
			}

			if !shell.IsAvailable() {
				t.Skipf("Shell %s not available", shell.Name())
			}

			cmd, args := shell.Command(tt.script)
			execCmd := exec.Command(cmd, args...)
			output, err := execCmd.CombinedOutput()
			if err != nil {
				t.Fatalf("Command execution failed: %v", err)
			}

			outputStr := strings.TrimSpace(string(output))
			if outputStr != tt.wantOut {
				t.Errorf("Output = %q, want %q", outputStr, tt.wantOut)
			}
		})
	}
}

// TestShellErrorHandling tests that shells properly handle errors
func TestShellErrorHandling(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping error handling test on Windows")
	}

	shell, err := GetShell(ShellTypeBash)
	if err != nil {
		t.Fatalf("GetShell() error = %v", err)
	}

	if !shell.IsAvailable() {
		t.Skip("Bash not available")
	}

	// Run a command that should fail
	cmd, args := shell.Command("exit 1")
	execCmd := exec.Command(cmd, args...)
	err = execCmd.Run()

	if err == nil {
		t.Error("Expected error from 'exit 1', got nil")
	}
}

// TestAllShellTypes tests that we can create all shell types
func TestAllShellTypes(t *testing.T) {
	shellTypes := []ShellType{
		ShellTypeBash,
		ShellTypeSh,
		ShellTypePowershell,
		ShellTypeCmd,
		ShellTypeDefault,
	}

	for _, st := range shellTypes {
		t.Run(string(st), func(t *testing.T) {
			shell, err := GetShell(st)
			if err != nil {
				t.Fatalf("GetShell(%s) error = %v", st, err)
			}
			if shell == nil {
				t.Fatalf("GetShell(%s) returned nil shell", st)
			}

			// Verify all interface methods work
			_ = shell.Name()
			_ = shell.Type()
			_ = shell.IsAvailable()
			_ = shell.EnvVarSeparator()
			_, _ = shell.Command("test")
		})
	}
}

// TestShellCommandFormat tests command formatting for all shells
func TestShellCommandFormat(t *testing.T) {
	tests := []struct {
		name       string
		shell      Shell
		script     string
		wantCmd    string
		wantArgs   []string
	}{
		{
			name:       "bash",
			shell:      &BashShell{},
			script:     "ls -la",
			wantCmd:    "bash",
			wantArgs:   []string{"-c", "ls -la"},
		},
		{
			name:       "sh",
			shell:      &ShShell{},
			script:     "pwd",
			wantCmd:    "sh",
			wantArgs:   []string{"-c", "pwd"},
		},
		{
			name:       "powershell",
			shell:      &PowershellShell{},
			script:     "Get-ChildItem",
			wantCmd:    "powershell",
			wantArgs:   []string{"-NoProfile", "-Command", "Get-ChildItem"},
		},
		{
			name:       "cmd",
			shell:      &CmdShell{},
			script:     "dir",
			wantCmd:    "cmd",
			wantArgs:   []string{"/C", "dir"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, args := tt.shell.Command(tt.script)
			if cmd != tt.wantCmd {
				t.Errorf("Command() cmd = %v, want %v", cmd, tt.wantCmd)
			}
			if len(args) != len(tt.wantArgs) {
				t.Errorf("Command() args length = %v, want %v", len(args), len(tt.wantArgs))
				return
			}
			for i, arg := range args {
				if arg != tt.wantArgs[i] {
					t.Errorf("Command() args[%d] = %v, want %v", i, arg, tt.wantArgs[i])
				}
			}
		})
	}
}

// TestEnvVarSeparators tests environment variable separators
func TestEnvVarSeparators(t *testing.T) {
	tests := []struct {
		name      string
		shell     Shell
		wantSep   string
	}{
		{"bash", &BashShell{}, ":"},
		{"sh", &ShShell{}, ":"},
		{"powershell", &PowershellShell{}, ";"},
		{"cmd", &CmdShell{}, ";"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sep := tt.shell.EnvVarSeparator()
			if sep != tt.wantSep {
				t.Errorf("EnvVarSeparator() = %v, want %v", sep, tt.wantSep)
			}
		})
	}
}
