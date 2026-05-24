// SPDX-License-Identifier: Apache-2.0

package exec

import (
	"bytes"
	"strings"
	"testing"
)

func TestNewCommand_Wiring(t *testing.T) {
	t.Parallel()
	cmd := NewCommand(Deps{})
	if cmd.Use != "exec" {
		t.Errorf("Use = %q, want exec", cmd.Use)
	}

	want := map[string]bool{"run": true, "async": true, "script": true}
	for _, c := range cmd.Commands() {
		delete(want, c.Name())
	}
	if len(want) > 0 {
		t.Errorf("missing subcommands: %v", want)
	}

	// Persistent flags should resolve on subcommands.
	for _, name := range []string{"server", "api-key", "output"} {
		if cmd.PersistentFlags().Lookup(name) == nil {
			t.Errorf("missing persistent flag %q", name)
		}
	}
}

func TestNewCommand_HelpDoesNotDial(t *testing.T) {
	t.Parallel()
	cmd := NewCommand(Deps{})
	cmd.SetArgs([]string{"run", "--help"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"--target", "--concurrency", "--shell", "--env"} {
		if !strings.Contains(out, want) {
			t.Errorf("help missing %q", want)
		}
	}
}

func TestRun_RequiresTarget(t *testing.T) {
	t.Parallel()
	cmd := NewCommand(Deps{})
	cmd.SetArgs([]string{"run", "uptime"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error: --target required")
	}
	if !strings.Contains(err.Error(), "--target") {
		t.Errorf("err = %v, want --target message", err)
	}
}

func TestWrapWithShell(t *testing.T) {
	t.Parallel()
	cases := []struct {
		shell    string
		cmd      string
		args     []string
		wantBin  string
		wantArgv []string
	}{
		{"bash", "uptime", nil, "bash", []string{"-c", "uptime"}},
		{"sh", "ls", []string{"/tmp"}, "sh", []string{"-c", "ls /tmp"}},
		{"powershell", "Get-Process", nil, "powershell", []string{"-NoProfile", "-Command", "Get-Process"}},
		{"cmd", "dir", nil, "cmd", []string{"/c", "dir"}},
		{"unknown", "raw", []string{"a", "b"}, "raw", []string{"a", "b"}},
	}
	for _, tc := range cases {
		bin, argv := wrapWithShell(tc.shell, tc.cmd, tc.args)
		if bin != tc.wantBin {
			t.Errorf("[%s] bin = %q, want %q", tc.shell, bin, tc.wantBin)
		}
		if len(argv) != len(tc.wantArgv) {
			t.Errorf("[%s] argv = %v, want %v", tc.shell, argv, tc.wantArgv)
			continue
		}
		for i := range argv {
			if argv[i] != tc.wantArgv[i] {
				t.Errorf("[%s] argv[%d] = %q, want %q", tc.shell, i, argv[i], tc.wantArgv[i])
			}
		}
	}
}
