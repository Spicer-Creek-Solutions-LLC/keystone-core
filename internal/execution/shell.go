// SPDX-License-Identifier: Apache-2.0

package execution

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// Shell is the cross-platform shell selector used by the --shell flag
// and by callers that want to run a command string through a shell
// rather than as a raw exec'd binary.
type Shell int

const (
	// ShellDefault resolves to the host's natural shell at use-time
	// (see GetDefaultShell). Use this when the caller doesn't care.
	ShellDefault Shell = iota
	ShellBash
	ShellSh
	ShellPowershell
	ShellCmd
)

// String returns the canonical lowercase name; ParseShell is the
// inverse. Unknown values render as Shell(N) for diagnostics.
func (s Shell) String() string {
	switch s {
	case ShellDefault:
		return "default"
	case ShellBash:
		return "bash"
	case ShellSh:
		return "sh"
	case ShellPowershell:
		return "powershell"
	case ShellCmd:
		return "cmd"
	default:
		return fmt.Sprintf("Shell(%d)", int(s))
	}
}

// ParseShell maps a flag string ("bash", "sh", "powershell", "cmd",
// "default", "") to a Shell. Empty input returns ShellDefault so
// `--shell ""` is the same as omitting the flag.
func ParseShell(s string) (Shell, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "default":
		return ShellDefault, nil
	case "bash":
		return ShellBash, nil
	case "sh":
		return ShellSh, nil
	case "powershell", "pwsh":
		return ShellPowershell, nil
	case "cmd":
		return ShellCmd, nil
	default:
		return ShellDefault, fmt.Errorf("execution: unknown shell %q", s)
	}
}

// GetDefaultShell picks the host's natural shell from runtime.GOOS:
// bash on Linux/Darwin (falling back to sh if bash is missing),
// powershell on Windows.
func GetDefaultShell() Shell {
	switch runtime.GOOS {
	case "windows":
		return ShellPowershell
	default:
		if ShellBash.IsAvailable() {
			return ShellBash
		}
		return ShellSh
	}
}

// resolved returns s with ShellDefault collapsed to the platform's
// natural shell. Other values pass through.
func (s Shell) resolved() Shell {
	if s == ShellDefault {
		return GetDefaultShell()
	}
	return s
}

// IsAvailable reports whether the shell's binary is on PATH. The
// resolved shell is checked, so ShellDefault.IsAvailable() reflects
// the platform default.
func (s Shell) IsAvailable() bool {
	bin := s.resolved().binary()
	if bin == "" {
		return false
	}
	_, err := exec.LookPath(bin)
	return err == nil
}

// binary returns the executable name to look up via exec.LookPath.
// Returns "" for ShellDefault (callers must resolve first).
func (s Shell) binary() string {
	switch s {
	case ShellBash:
		return "bash"
	case ShellSh:
		return "sh"
	case ShellPowershell:
		// Prefer pwsh (PowerShell 7+, cross-platform) when available;
		// fall back to powershell.exe for Windows hosts. LookPath
		// happens at IsAvailable / CommandLine time.
		if _, err := exec.LookPath("pwsh"); err == nil {
			return "pwsh"
		}
		return "powershell"
	case ShellCmd:
		return "cmd"
	default:
		return ""
	}
}

// CommandLine returns the (executable, args) pair that runs cmd via
// the resolved shell. The returned executable is the binary name;
// callers using ExecuteRequest set req.Command and req.Args from this
// result. ShellDefault is resolved through GetDefaultShell first.
func (s Shell) CommandLine(cmd string) (string, []string) {
	switch s.resolved() {
	case ShellBash:
		return "bash", []string{"-c", cmd}
	case ShellSh:
		return "sh", []string{"-c", cmd}
	case ShellPowershell:
		return ShellPowershell.binary(), []string{"-NoProfile", "-Command", cmd}
	case ShellCmd:
		return "cmd", []string{"/c", cmd}
	default:
		// Unreachable in practice — resolved() never returns
		// ShellDefault — but keep a sane fallback for forward-compat.
		return "sh", []string{"-c", cmd}
	}
}
