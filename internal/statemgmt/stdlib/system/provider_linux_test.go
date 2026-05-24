// SPDX-License-Identifier: Apache-2.0

//go:build linux

package system

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
		shutdownBin:  "shutdown",
		localectlBin: "localectl",
		run:          run,
	}, &calls
}

// --- banner ------------------------------------------------------------

func bannerProvider(t *testing.T) (*linuxProvider, string, string, string) {
	t.Helper()
	dir := t.TempDir()
	motd := filepath.Join(dir, "motd")
	issue := filepath.Join(dir, "issue")
	issueNet := filepath.Join(dir, "issue.net")
	return &linuxProvider{
		bannerPaths: map[string]string{
			BannerMOTD:     motd,
			BannerIssue:    issue,
			BannerIssueNet: issueNet,
		},
	}, motd, issue, issueNet
}

func TestLinuxProvider_ReadBanner(t *testing.T) {
	t.Parallel()
	p, motd, _, _ := bannerProvider(t)
	// missing file → ("", nil)
	got, err := p.ReadBanner(context.Background(), BannerMOTD)
	if err != nil || got != "" {
		t.Errorf("missing: %q,%v", got, err)
	}
	// existing file
	if err := os.WriteFile(motd, []byte("Welcome\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err = p.ReadBanner(context.Background(), BannerMOTD)
	if err != nil || got != "Welcome\n" {
		t.Errorf("read: %q,%v", got, err)
	}
	// unknown banner name
	if _, err := p.ReadBanner(context.Background(), "frob"); err == nil {
		t.Error("unknown banner should error")
	}
}

func TestLinuxProvider_WriteBanner(t *testing.T) {
	t.Parallel()
	p, motd, issue, _ := bannerProvider(t)
	// create new file → 0644
	if err := p.WriteBanner(context.Background(), BannerMOTD, "Hello"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(motd)
	if string(data) != "Hello" {
		t.Errorf("contents: %q", data)
	}
	if fi, _ := os.Stat(motd); fi.Mode().Perm() != 0o644 {
		t.Errorf("mode = %o", fi.Mode().Perm())
	}
	// preserve existing mode
	if err := os.WriteFile(issue, []byte("old"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := p.WriteBanner(context.Background(), BannerIssue, "new"); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(issue); fi.Mode().Perm() != 0o640 {
		t.Errorf("mode preserved: %o", fi.Mode().Perm())
	}
	// unknown banner
	if err := p.WriteBanner(context.Background(), "frob", "x"); err == nil {
		t.Error("unknown banner should error")
	}
}

// --- reboot ------------------------------------------------------------

func TestLinuxProvider_IsRebootNeeded(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	marker := filepath.Join(dir, "reboot-required")
	p := &linuxProvider{}
	// absent
	need, err := p.IsRebootNeeded(context.Background(), marker)
	if err != nil || need {
		t.Errorf("absent: %v,%v", need, err)
	}
	// present
	if err := os.WriteFile(marker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	need, err = p.IsRebootNeeded(context.Background(), marker)
	if err != nil || !need {
		t.Errorf("present: %v,%v", need, err)
	}
}

func TestLinuxProvider_ScheduleReboot(t *testing.T) {
	t.Parallel()
	// delay > 0 → -r +N
	p, calls := newRecordingProvider("", nil)
	if err := p.ScheduleReboot(context.Background(), 5); err != nil {
		t.Fatal(err)
	}
	if (*calls)[0].bin != "shutdown" || strings.Join((*calls)[0].args, " ") != "-r +5" {
		t.Errorf("delay=5 args: %+v", (*calls)[0])
	}
	// delay == 0 → -r now
	p, calls = newRecordingProvider("", nil)
	if err := p.ScheduleReboot(context.Background(), 0); err != nil {
		t.Fatal(err)
	}
	if strings.Join((*calls)[0].args, " ") != "-r now" {
		t.Errorf("delay=0 args: %+v", (*calls)[0])
	}
	// runner error propagates
	p, _ = newRecordingProvider("", errors.New("denied"))
	if err := p.ScheduleReboot(context.Background(), 1); err == nil {
		t.Error("runner error should propagate")
	}
	// missing binary
	if err := (&linuxProvider{}).ScheduleReboot(context.Background(), 1); !errors.Is(err, ErrNoShutdown) {
		t.Errorf("missing shutdown → %v", err)
	}
}

// --- locale ------------------------------------------------------------

func TestParseLangValue(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"LANG=en_US.UTF-8\n":          "en_US.UTF-8",
		"# header\nLANG=C\n":          "C",
		`LANG="en_US.UTF-8"` + "\n":   "en_US.UTF-8",
		"LANG=C # comment\n":          "C",
		"  LANG=de_DE.UTF-8\n":        "de_DE.UTF-8",
		"# LANG=enforced\nLANG=POSIX": "POSIX",
		"":                            "",
		"# no LANG line\n":            "",
		"LC_ALL=en_US.UTF-8\n":        "",
	} {
		if got := parseLangValue([]byte(in)); got != want {
			t.Errorf("parseLangValue(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRewriteLangValue(t *testing.T) {
	t.Parallel()
	cases := []struct{ in, want, lang string }{
		{"# header\nLANG=C\nLC_ALL=C\n", "# header\nLANG=en_US.UTF-8\nLC_ALL=C\n", "en_US.UTF-8"},
		{"LANG=C", "LANG=en_US.UTF-8", "en_US.UTF-8"},
		{"# nothing yet\n", "# nothing yet\nLANG=C\n", "C"},
		{"", "LANG=C\n", "C"},
		{"LANG=C\nLANG=POSIX\n", "LANG=en_US.UTF-8\nLANG=POSIX\n", "en_US.UTF-8"},
		{"# LANG=enforced\nLANG=C\n", "# LANG=enforced\nLANG=POSIX\n", "POSIX"},
	}
	for _, c := range cases {
		if got := string(rewriteLangValue([]byte(c.in), c.lang)); got != c.want {
			t.Errorf("rewrite(%q, %q) = %q, want %q", c.in, c.lang, got, c.want)
		}
	}
}

func TestLinuxProvider_ReadLocale(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "locale.conf")
	// missing file
	p := &linuxProvider{localeConfPath: path}
	got, err := p.ReadLocale(context.Background())
	if err != nil || got != "" {
		t.Errorf("missing: %q,%v", got, err)
	}
	// present
	if err := os.WriteFile(path, []byte("LANG=en_US.UTF-8\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err = p.ReadLocale(context.Background())
	if err != nil || got != "en_US.UTF-8" {
		t.Errorf("read: %q,%v", got, err)
	}
}

func TestLinuxProvider_WriteLocale_WithLocalectl(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "locale.conf")
	if err := os.WriteFile(path, []byte("LANG=C\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var calls []capture
	run := func(_ context.Context, bin string, args []string) (string, error) {
		calls = append(calls, capture{bin: bin, args: args})
		return "", nil
	}
	p := &linuxProvider{localeConfPath: path, localectlBin: "localectl", run: run}
	if err := p.WriteLocale(context.Background(), "en_US.UTF-8"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "LANG=en_US.UTF-8\n" {
		t.Errorf("contents: %q", data)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode preserved: %o", fi.Mode().Perm())
	}
	if len(calls) != 1 || calls[0].bin != "localectl" || strings.Join(calls[0].args, " ") != "set-locale LANG=en_US.UTF-8" {
		t.Errorf("localectl call: %+v", calls)
	}
}

func TestLinuxProvider_WriteLocale_WithoutLocalectl(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "locale.conf")
	p := &linuxProvider{localeConfPath: path}
	if err := p.WriteLocale(context.Background(), "C"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "LANG=C\n" {
		t.Errorf("contents: %q", data)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o644 {
		t.Errorf("new-file mode: %o", fi.Mode().Perm())
	}
}

func TestLinuxProvider_WriteLocale_LocalectlError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "locale.conf")
	run := func(context.Context, string, []string) (string, error) {
		return "", errors.New("dbus not available")
	}
	p := &linuxProvider{localeConfPath: path, localectlBin: "localectl", run: run}
	if err := p.WriteLocale(context.Background(), "C"); err == nil {
		t.Error("localectl error should propagate")
	}
}

func TestLinuxProvider_WriteLocale_UnwritableParent(t *testing.T) {
	t.Parallel()
	p := &linuxProvider{localeConfPath: "/no-such-dir/locale.conf"}
	if err := p.WriteLocale(context.Background(), "C"); err == nil {
		t.Error("unwritable parent should error")
	}
}

// --- exec + defaultProvider ------------------------------------------

func TestExecRun(t *testing.T) {
	t.Parallel()
	if _, err := execRun(context.Background(), "false", nil); err == nil {
		t.Error("expected an error from `false`")
	}
	if _, err := execRun(context.Background(), "/nonexistent/shutdown", nil); err == nil {
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
