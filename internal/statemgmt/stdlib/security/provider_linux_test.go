// SPDX-License-Identifier: Apache-2.0

//go:build linux

package security

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type capture struct {
	bin  string
	args []string
}

func newRecordingProvider(out string, runErr error) (*linuxProvider, *[]capture) {
	var calls []capture
	run := func(_ context.Context, bin string, args []string) (string, error) {
		calls = append(calls, capture{bin: bin, args: args})
		return out, runErr
	}
	return &linuxProvider{
		getenforceBin: "getenforce",
		setenforceBin: "setenforce",
		getseboolBin:  "getsebool",
		setseboolBin:  "setsebool",
		run:           run,
	}, &calls
}

// --- parseConfigMode -------------------------------------------------

func TestParseConfigMode(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		in    string
		want  string
		isErr bool
	}{
		{"enforcing", "# header\nSELINUX=enforcing\nSELINUXTYPE=targeted\n", "enforcing", false},
		{"permissive", "SELINUX=permissive\n", "permissive", false},
		{"disabled", "SELINUX=disabled\n", "disabled", false},
		{"quoted", `SELINUX="enforcing"` + "\n", "enforcing", false},
		{"upper", "SELINUX=ENFORCING\n", "enforcing", false},
		{"trailing comment", "SELINUX=permissive # see config(5)\n", "permissive", false},
		{"leading whitespace", "  SELINUX=enforcing\n", "enforcing", false},
		{"comment ignored", "# SELINUX=enforcing\nSELINUX=permissive\n", "permissive", false},
		{"unknown value", "SELINUX=strict\n", "", true},
		{"missing line", "# nothing here\n", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseConfigMode([]byte(c.in))
			if (err != nil) != c.isErr {
				t.Fatalf("err=%v wantErr=%v", err, c.isErr)
			}
			if !c.isErr && got != c.want {
				t.Errorf("got %q want %q", got, c.want)
			}
		})
	}
}

// --- rewriteConfigMode -----------------------------------------------

func TestRewriteConfigMode(t *testing.T) {
	t.Parallel()
	// replace an existing line
	in := []byte("# header\nSELINUX=permissive\nSELINUXTYPE=targeted\n")
	got := string(rewriteConfigMode(in, "enforcing"))
	want := "# header\nSELINUX=enforcing\nSELINUXTYPE=targeted\n"
	if got != want {
		t.Errorf("replace: got %q want %q", got, want)
	}
	// preserve absence of trailing newline
	in = []byte("SELINUX=permissive")
	got = string(rewriteConfigMode(in, "disabled"))
	if got != "SELINUX=disabled" {
		t.Errorf("no-trailing-newline: got %q", got)
	}
	// append when no SELINUX= line exists
	in = []byte("# header\nSELINUXTYPE=targeted\n")
	got = string(rewriteConfigMode(in, "enforcing"))
	want = "# header\nSELINUXTYPE=targeted\nSELINUX=enforcing\n"
	if got != want {
		t.Errorf("append: got %q want %q", got, want)
	}
	// empty input
	got = string(rewriteConfigMode(nil, "permissive"))
	if got != "SELINUX=permissive\n" {
		t.Errorf("empty: got %q", got)
	}
	// only the first SELINUX= line is replaced (defensive)
	in = []byte("SELINUX=permissive\nSELINUX=disabled\n")
	got = string(rewriteConfigMode(in, "enforcing"))
	if got != "SELINUX=enforcing\nSELINUX=disabled\n" {
		t.Errorf("multi: got %q", got)
	}
	// commented-out SELINUX= is preserved
	in = []byte("# SELINUX=enforcing\nSELINUX=permissive\n")
	got = string(rewriteConfigMode(in, "disabled"))
	if got != "# SELINUX=enforcing\nSELINUX=disabled\n" {
		t.Errorf("comment-preserved: got %q", got)
	}
}

// --- GetPersistentMode / SetPersistentMode (filesystem) --------------

func writeConfig(t *testing.T, dir, contents string, perm os.FileMode) string {
	t.Helper()
	path := filepath.Join(dir, "config")
	if err := os.WriteFile(path, []byte(contents), perm); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLinuxProvider_GetPersistentMode(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := writeConfig(t, dir, "SELINUX=enforcing\n", 0o644)
	p := &linuxProvider{configPath: path}
	got, err := p.GetPersistentMode(context.Background())
	if err != nil || got != "enforcing" {
		t.Fatalf("got %q err %v", got, err)
	}
	// missing file → ErrSELinuxUnavailable
	p = &linuxProvider{configPath: filepath.Join(dir, "no-such-file")}
	if _, err := p.GetPersistentMode(context.Background()); !errors.Is(err, ErrSELinuxUnavailable) {
		t.Errorf("missing → %v", err)
	}
}

func TestLinuxProvider_SetPersistentMode(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// pre-existing file with custom mode
	path := writeConfig(t, dir, "# header\nSELINUX=permissive\nSELINUXTYPE=targeted\n", 0o640)
	p := &linuxProvider{configPath: path}
	if err := p.SetPersistentMode(context.Background(), "enforcing"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "SELINUX=enforcing") {
		t.Errorf("file contents: %q", data)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o640 {
		t.Errorf("mode = %o, want 0640", fi.Mode().Perm())
	}
	// missing file
	p = &linuxProvider{configPath: filepath.Join(dir, "no-such-file")}
	if err := p.SetPersistentMode(context.Background(), "enforcing"); !errors.Is(err, ErrSELinuxUnavailable) {
		t.Errorf("missing → %v", err)
	}
	// unwritable directory (temp create fails)
	bad := filepath.Join(dir, "no-such-dir", "config")
	// pre-create the original file in a real path, then point configPath at one whose parent doesn't exist
	if err := os.WriteFile(filepath.Join(dir, "elsewhere"), []byte("SELINUX=enforcing\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	p = &linuxProvider{configPath: bad}
	if err := p.SetPersistentMode(context.Background(), "enforcing"); err == nil {
		t.Error("unwritable parent should error (the file is missing, so we expect ErrSELinuxUnavailable wrap)")
	}
}

// --- GetRuntimeMode / SetRuntimeMode ----------------------------------

func TestLinuxProvider_GetRuntimeMode(t *testing.T) {
	t.Parallel()
	for raw, want := range map[string]string{
		"Enforcing\n":  ModeEnforcing,
		"Permissive\n": ModePermissive,
		"Disabled\n":   ModeDisabled,
		"Permissive":   ModePermissive,
	} {
		p, calls := newRecordingProvider(raw, nil)
		got, err := p.GetRuntimeMode(context.Background())
		if err != nil || got != want {
			t.Errorf("getenforce=%q: got %q,%v", raw, got, err)
		}
		if (*calls)[0].bin != "getenforce" {
			t.Errorf("bin %q", (*calls)[0].bin)
		}
	}
	// unexpected output
	p, _ := newRecordingProvider("Whatever\n", nil)
	if _, err := p.GetRuntimeMode(context.Background()); err == nil {
		t.Error("unexpected output should error")
	}
	// runner error propagates
	p, _ = newRecordingProvider("", errors.New("exit 1"))
	if _, err := p.GetRuntimeMode(context.Background()); err == nil {
		t.Error("runner error should propagate")
	}
	// missing binary
	if _, err := (&linuxProvider{run: nil}).GetRuntimeMode(context.Background()); !errors.Is(err, ErrSELinuxUnavailable) {
		t.Errorf("missing getenforce → %v", err)
	}
}

func TestLinuxProvider_SetRuntimeMode(t *testing.T) {
	t.Parallel()
	// enforcing
	p, calls := newRecordingProvider("", nil)
	if err := p.SetRuntimeMode(context.Background(), ModeEnforcing); err != nil {
		t.Fatal(err)
	}
	if (*calls)[0].bin != "setenforce" || strings.Join((*calls)[0].args, " ") != "1" {
		t.Errorf("enforcing args: %+v", (*calls)[0])
	}
	// permissive
	p, calls = newRecordingProvider("", nil)
	if err := p.SetRuntimeMode(context.Background(), ModePermissive); err != nil {
		t.Fatal(err)
	}
	if strings.Join((*calls)[0].args, " ") != "0" {
		t.Errorf("permissive args: %+v", (*calls)[0])
	}
	// disabled → not allowed
	p, _ = newRecordingProvider("", nil)
	if err := p.SetRuntimeMode(context.Background(), ModeDisabled); err == nil {
		t.Error("disabled should error (kernel only)")
	}
	// unknown mode
	if err := p.SetRuntimeMode(context.Background(), "strict"); err == nil {
		t.Error("unknown mode should error")
	}
	// runner error propagates
	p, _ = newRecordingProvider("", errors.New("perm denied"))
	if err := p.SetRuntimeMode(context.Background(), ModeEnforcing); err == nil {
		t.Error("runner error should propagate")
	}
	// missing binary
	if err := (&linuxProvider{run: nil}).SetRuntimeMode(context.Background(), ModeEnforcing); !errors.Is(err, ErrSELinuxUnavailable) {
		t.Errorf("missing setenforce → %v", err)
	}
}

// --- GetBoolean / SetBoolean -----------------------------------------

func TestLinuxProvider_GetBoolean(t *testing.T) {
	t.Parallel()
	p, calls := newRecordingProvider("httpd_can_network_connect --> on\n", nil)
	v, err := p.GetBoolean(context.Background(), "httpd_can_network_connect")
	if err != nil || !v {
		t.Fatalf("on: %v %v", v, err)
	}
	if (*calls)[0].bin != "getsebool" || strings.Join((*calls)[0].args, " ") != "httpd_can_network_connect" {
		t.Errorf("args: %+v", (*calls)[0])
	}
	p, _ = newRecordingProvider("frob --> off\n", nil)
	v, err = p.GetBoolean(context.Background(), "frob")
	if err != nil || v {
		t.Errorf("off: %v %v", v, err)
	}
	// unexpected output
	p, _ = newRecordingProvider("Unknown boolean", nil)
	if _, err := p.GetBoolean(context.Background(), "x"); err == nil {
		t.Error("unexpected output should error")
	}
	// runner error
	p, _ = newRecordingProvider("", errors.New("invalid"))
	if _, err := p.GetBoolean(context.Background(), "x"); err == nil {
		t.Error("runner error should propagate")
	}
	// missing binary
	if _, err := (&linuxProvider{run: nil}).GetBoolean(context.Background(), "x"); !errors.Is(err, ErrSELinuxUnavailable) {
		t.Errorf("missing getsebool → %v", err)
	}
}

func TestLinuxProvider_SetBoolean(t *testing.T) {
	t.Parallel()
	p, calls := newRecordingProvider("", nil)
	if err := p.SetBoolean(context.Background(), "httpd_can_network_connect", true); err != nil {
		t.Fatal(err)
	}
	if strings.Join((*calls)[0].args, " ") != "-P httpd_can_network_connect=on" {
		t.Errorf("on args: %+v", (*calls)[0])
	}
	p, calls = newRecordingProvider("", nil)
	if err := p.SetBoolean(context.Background(), "frob", false); err != nil {
		t.Fatal(err)
	}
	if strings.Join((*calls)[0].args, " ") != "-P frob=off" {
		t.Errorf("off args: %+v", (*calls)[0])
	}
	// runner error
	p, _ = newRecordingProvider("", errors.New("denied"))
	if err := p.SetBoolean(context.Background(), "x", true); err == nil {
		t.Error("runner error should propagate")
	}
	// missing binary
	if err := (&linuxProvider{run: nil}).SetBoolean(context.Background(), "x", true); !errors.Is(err, ErrSELinuxUnavailable) {
		t.Errorf("missing setsebool → %v", err)
	}
}

// --- execRun + defaultProvider ----------------------------------------

func TestExecRun(t *testing.T) {
	t.Parallel()
	if _, err := execRun(context.Background(), "false", nil); err == nil {
		t.Error("expected an error from `false`")
	}
	if _, err := execRun(context.Background(), "/nonexistent/getenforce", nil); err == nil {
		t.Error("expected an error from a missing binary")
	}
	out, err := execRun(context.Background(), "echo", []string{"-n", "ok"})
	if err != nil || out != "ok" {
		t.Errorf("echo: %q %v", out, err)
	}
}

func TestDefaultProvider_NonNil(t *testing.T) {
	t.Parallel()
	if defaultProvider() == nil {
		t.Fatal("defaultProvider returned nil")
	}
}
