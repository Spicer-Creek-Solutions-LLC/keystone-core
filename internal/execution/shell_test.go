package execution

import (
	"runtime"
	"strings"
	"testing"
)

func TestShell_StringAndParse(t *testing.T) {
	t.Parallel()

	cases := []struct {
		s    Shell
		text string
	}{
		{ShellDefault, "default"},
		{ShellBash, "bash"},
		{ShellSh, "sh"},
		{ShellPowershell, "powershell"},
		{ShellCmd, "cmd"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.text, func(t *testing.T) {
			t.Parallel()
			if got := tc.s.String(); got != tc.text {
				t.Errorf("%v.String() = %q, want %q", tc.s, got, tc.text)
			}
			parsed, err := ParseShell(tc.text)
			if err != nil {
				t.Fatalf("ParseShell(%q): %v", tc.text, err)
			}
			if parsed != tc.s {
				t.Errorf("ParseShell(%q) = %v, want %v", tc.text, parsed, tc.s)
			}
		})
	}
}

func TestShell_StringUnknown(t *testing.T) {
	t.Parallel()
	if got := Shell(99).String(); got != "Shell(99)" {
		t.Errorf("Shell(99).String() = %q", got)
	}
}

func TestParseShell_AlternateForms(t *testing.T) {
	t.Parallel()

	cases := map[string]Shell{
		"":           ShellDefault,
		"  ":         ShellDefault,
		"BASH":       ShellBash,
		" Sh ":       ShellSh,
		"pwsh":       ShellPowershell, // alias
		"PowerShell": ShellPowershell,
	}
	for in, want := range cases {
		got, err := ParseShell(in)
		if err != nil {
			t.Errorf("ParseShell(%q): %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("ParseShell(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestParseShell_Unknown(t *testing.T) {
	t.Parallel()
	_, err := ParseShell("zsh-nope")
	if err == nil {
		t.Fatal("expected error for unknown shell")
	}
	if !strings.Contains(err.Error(), "unknown shell") {
		t.Errorf("error = %q, want substring 'unknown shell'", err)
	}
}

func TestShell_CommandLine(t *testing.T) {
	t.Parallel()

	cases := []struct {
		shell    Shell
		wantBin  string
		wantArgv []string
	}{
		{ShellBash, "bash", []string{"-c", "echo hi"}},
		{ShellSh, "sh", []string{"-c", "echo hi"}},
		{ShellCmd, "cmd", []string{"/c", "echo hi"}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.shell.String(), func(t *testing.T) {
			t.Parallel()
			bin, argv := tc.shell.CommandLine("echo hi")
			if bin != tc.wantBin {
				t.Errorf("bin = %q, want %q", bin, tc.wantBin)
			}
			if !sliceEq(argv, tc.wantArgv) {
				t.Errorf("argv = %v, want %v", argv, tc.wantArgv)
			}
		})
	}
}

func TestShell_PowershellArgv(t *testing.T) {
	t.Parallel()

	bin, argv := ShellPowershell.CommandLine("Get-Process")
	if bin != "pwsh" && bin != "powershell" {
		t.Errorf("bin = %q, want pwsh or powershell", bin)
	}
	want := []string{"-NoProfile", "-Command", "Get-Process"}
	if !sliceEq(argv, want) {
		t.Errorf("argv = %v, want %v", argv, want)
	}
}

func TestShell_DefaultResolvesToPlatformShell(t *testing.T) {
	t.Parallel()

	def := GetDefaultShell()
	switch runtime.GOOS {
	case "windows":
		if def != ShellPowershell {
			t.Errorf("windows default = %v, want ShellPowershell", def)
		}
	default:
		if def != ShellBash && def != ShellSh {
			t.Errorf("non-windows default = %v, want ShellBash or ShellSh", def)
		}
	}

	// ShellDefault.CommandLine should match the resolved shell's
	// CommandLine for the same input.
	wantBin, wantArgv := def.CommandLine("echo hi")
	gotBin, gotArgv := ShellDefault.CommandLine("echo hi")
	if gotBin != wantBin || !sliceEq(gotArgv, wantArgv) {
		t.Errorf("ShellDefault.CommandLine = %s %v; want %s %v", gotBin, gotArgv, wantBin, wantArgv)
	}
}

func TestShell_IsAvailable_NonExistent(t *testing.T) {
	t.Parallel()

	// ShellCmd is Windows-only; on Linux runners it should not be
	// available. The reverse is true on Windows.
	if runtime.GOOS == "windows" {
		t.Skip("cmd ships with windows; this assertion runs on non-windows hosts")
	}
	if ShellCmd.IsAvailable() {
		t.Error("ShellCmd should not be available on non-windows host")
	}
}

func TestShell_BinaryUnknown(t *testing.T) {
	t.Parallel()
	if got := Shell(99).binary(); got != "" {
		t.Errorf("Shell(99).binary() = %q, want empty", got)
	}
	if Shell(99).IsAvailable() {
		t.Error("Shell(99).IsAvailable() = true, want false")
	}
}

func sliceEq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
