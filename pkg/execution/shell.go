package execution

import (
	"fmt"
	"os/exec"
	"runtime"
)

// ShellType represents the type of shell to use for command execution
type ShellType string

const (
	// ShellTypeBash represents the Bash shell
	ShellTypeBash ShellType = "bash"

	// ShellTypeSh represents the POSIX shell (sh)
	ShellTypeSh ShellType = "sh"

	// ShellTypePowershell represents PowerShell (Windows)
	ShellTypePowershell ShellType = "powershell"

	// ShellTypeCmd represents Windows Command Prompt
	ShellTypeCmd ShellType = "cmd"

	// ShellTypeDefault uses the system default shell
	ShellTypeDefault ShellType = "default"
)

// Shell defines the interface for different shell types
type Shell interface {
	// Name returns the shell name
	Name() string

	// Type returns the shell type
	Type() ShellType

	// Command returns the shell command and arguments to execute a script
	Command(script string) (string, []string)

	// IsAvailable checks if the shell is available on the system
	IsAvailable() bool

	// EnvVarSeparator returns the environment variable path separator for this shell
	EnvVarSeparator() string
}

// GetShell returns a Shell implementation for the given type
func GetShell(shellType ShellType) (Shell, error) {
	switch shellType {
	case ShellTypeBash:
		return &BashShell{}, nil
	case ShellTypeSh:
		return &ShShell{}, nil
	case ShellTypePowershell:
		return &PowershellShell{}, nil
	case ShellTypeCmd:
		return &CmdShell{}, nil
	case ShellTypeDefault:
		return GetDefaultShell()
	default:
		return nil, fmt.Errorf("unknown shell type: %s", shellType)
	}
}

// GetDefaultShell returns the default shell for the current OS
func GetDefaultShell() (Shell, error) {
	switch runtime.GOOS {
	case "windows":
		// Prefer PowerShell on Windows if available
		ps := &PowershellShell{}
		if ps.IsAvailable() {
			return ps, nil
		}
		return &CmdShell{}, nil
	case "darwin", "linux", "freebsd", "openbsd", "netbsd":
		// Prefer bash on Unix-like systems if available
		bash := &BashShell{}
		if bash.IsAvailable() {
			return bash, nil
		}
		return &ShShell{}, nil
	default:
		return nil, fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}

// BashShell implements the Bash shell
type BashShell struct{}

func (s *BashShell) Name() string {
	return "bash"
}

func (s *BashShell) Type() ShellType {
	return ShellTypeBash
}

func (s *BashShell) Command(script string) (string, []string) {
	return "bash", []string{"-c", script}
}

func (s *BashShell) IsAvailable() bool {
	_, err := exec.LookPath("bash")
	return err == nil
}

func (s *BashShell) EnvVarSeparator() string {
	return ":"
}

// ShShell implements the POSIX shell
type ShShell struct{}

func (s *ShShell) Name() string {
	return "sh"
}

func (s *ShShell) Type() ShellType {
	return ShellTypeSh
}

func (s *ShShell) Command(script string) (string, []string) {
	return "sh", []string{"-c", script}
}

func (s *ShShell) IsAvailable() bool {
	_, err := exec.LookPath("sh")
	return err == nil
}

func (s *ShShell) EnvVarSeparator() string {
	return ":"
}

// PowershellShell implements PowerShell
type PowershellShell struct{}

func (s *PowershellShell) Name() string {
	return "powershell"
}

func (s *PowershellShell) Type() ShellType {
	return ShellTypePowershell
}

func (s *PowershellShell) Command(script string) (string, []string) {
	// Use -Command for executing scripts
	// -NoProfile speeds up execution by skipping profile loading
	return "powershell", []string{"-NoProfile", "-Command", script}
}

func (s *PowershellShell) IsAvailable() bool {
	// Try both "powershell" and "pwsh" (PowerShell Core)
	_, err := exec.LookPath("powershell")
	if err == nil {
		return true
	}
	_, err = exec.LookPath("pwsh")
	return err == nil
}

func (s *PowershellShell) EnvVarSeparator() string {
	return ";"
}

// CmdShell implements Windows Command Prompt
type CmdShell struct{}

func (s *CmdShell) Name() string {
	return "cmd"
}

func (s *CmdShell) Type() ShellType {
	return ShellTypeCmd
}

func (s *CmdShell) Command(script string) (string, []string) {
	// /C executes the command and then terminates
	return "cmd", []string{"/C", script}
}

func (s *CmdShell) IsAvailable() bool {
	// cmd.exe is always available on Windows
	if runtime.GOOS == "windows" {
		return true
	}
	_, err := exec.LookPath("cmd")
	return err == nil
}

func (s *CmdShell) EnvVarSeparator() string {
	return ";"
}
