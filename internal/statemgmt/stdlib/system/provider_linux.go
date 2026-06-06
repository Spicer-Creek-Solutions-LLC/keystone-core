// SPDX-License-Identifier: Apache-2.0

//go:build linux

package system

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Canonical file paths. Tests override these via the linuxProvider
// fields directly (the package-internal constructor lets a test
// linuxProvider point at a tempdir).
const (
	defaultLocaleConfPath = "/etc/locale.conf"
)

// bannerPath maps a banner name to its on-disk path.
var bannerPath = map[string]string{
	BannerMOTD:     "/etc/motd",
	BannerIssue:    "/etc/issue",
	BannerIssueNet: "/etc/issue.net",
}

func defaultProvider() Provider {
	p := &linuxProvider{
		bannerPaths:    bannerPath,
		localeConfPath: defaultLocaleConfPath,
		run:            execRun,
	}
	p.shutdownBin, _ = exec.LookPath("shutdown")
	p.localectlBin, _ = exec.LookPath("localectl")
	p.rebootDetect = detectRebootProbe(exec.LookPath)
	return p
}

type linuxProvider struct {
	bannerPaths    map[string]string
	localeConfPath string
	shutdownBin    string
	localectlBin   string
	run            commandRunner

	// rebootDetect is the platform reboot-needed probe consulted when
	// the marker file is absent (the RHEL/Fedora path: `needs-restarting
	// -r`). It is nil on hosts where no such tool is present (e.g.
	// Debian, which relies on the marker file, or Alpine, which has no
	// reboot-required convention). A test injects a fake.
	rebootDetect rebootProbe
}

// rebootProbe reports whether the host needs a reboot. It returns an
// error only for a genuine probe failure (the detector binary vanished,
// an unexpected exit code) — a clean "reboot not needed" is (false, nil).
type rebootProbe func(ctx context.Context) (bool, error)

// detectRebootProbe wires the RHEL-family reboot detector by binary
// presence (lookPath is exec.LookPath in production, a fake in tests).
// Returns nil when no detector binary is present — the host then relies
// on the marker file alone.
func detectRebootProbe(lookPath func(string) (string, error)) rebootProbe {
	bin, args, ok := resolveRebootDetector(lookPath)
	if !ok {
		return nil
	}
	return realRebootProbe(bin, args)
}

// resolveRebootDetector picks the reboot-needed command: the standalone
// `needs-restarting` (from dnf-utils/yum-utils) is preferred, falling
// back to `dnf needs-restarting`. Both support the `-r` reboot-hint mode
// whose exit code is the signal (1 = reboot needed, 0 = not). ok is false
// when neither binary is present.
func resolveRebootDetector(lookPath func(string) (string, error)) (bin string, args []string, ok bool) {
	if b, err := lookPath("needs-restarting"); err == nil {
		return b, []string{"-r"}, true
	}
	if d, err := lookPath("dnf"); err == nil {
		return d, []string{"needs-restarting", "-r"}, true
	}
	return "", nil, false
}

// realRebootProbe runs `<bin> <args…>` and maps its exit code:
// 0 → reboot not needed, 1 → reboot needed (the `needs-restarting -r`
// contract). Any other exit code, or a failure to launch the binary, is
// a genuine error so a broken probe never silently reads as "no reboot
// needed".
func realRebootProbe(bin string, args []string) rebootProbe {
	return func(ctx context.Context) (bool, error) {
		cmd := exec.CommandContext(ctx, bin, args...) //nolint:gosec // bin resolved via exec.LookPath at detect time; args are the fixed needs-restarting reboot-hint verbs
		err := cmd.Run()
		if err == nil {
			return false, nil // exit 0 — no reboot needed
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			if exitErr.ExitCode() == 1 {
				return true, nil // exit 1 — reboot needed
			}
			return false, fmt.Errorf("%s %s: unexpected exit %d", bin, strings.Join(args, " "), exitErr.ExitCode())
		}
		return false, fmt.Errorf("%s %s: %w", bin, strings.Join(args, " "), err)
	}
}

// --- banner -------------------------------------------------------------

func (p *linuxProvider) ReadBanner(_ context.Context, name string) (string, error) {
	path, ok := p.bannerPaths[name]
	if !ok {
		return "", fmt.Errorf("unknown banner %q", name)
	}
	data, err := os.ReadFile(path) //nolint:gosec // path resolved from the validated banner name via p.bannerPaths (a fixed map): /etc/motd / /etc/issue / /etc/issue.net
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return string(data), nil
}

func (p *linuxProvider) WriteBanner(_ context.Context, name, content string) error {
	path, ok := p.bannerPaths[name]
	if !ok {
		return fmt.Errorf("unknown banner %q", name)
	}
	perm := os.FileMode(0o644)
	if fi, err := os.Stat(path); err == nil {
		perm = fi.Mode().Perm()
	}
	return writeFileAtomic(path, []byte(content), perm)
}

// --- reboot ------------------------------------------------------------

func (p *linuxProvider) IsRebootNeeded(ctx context.Context, markerFile string) (bool, error) {
	// 1. Marker file. The Debian/Ubuntu convention
	// (/var/run/reboot-required) and any operator-supplied when_file
	// override; present → reboot needed, on every distro.
	_, err := os.Stat(markerFile)
	if err == nil {
		return true, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("stat %s: %w", markerFile, err)
	}
	// 2. Marker absent → consult the platform reboot-needed probe
	// (RHEL/Fedora: `needs-restarting -r`). nil on hosts without one
	// (Debian relies on the marker; Alpine has no convention) → the
	// honest answer there is "not needed".
	if p.rebootDetect != nil {
		return p.rebootDetect(ctx)
	}
	return false, nil
}

func (p *linuxProvider) ScheduleReboot(ctx context.Context, delayMinutes int) error {
	if p.shutdownBin == "" {
		return ErrNoShutdown
	}
	arg := "now"
	if delayMinutes > 0 {
		arg = "+" + strconv.Itoa(delayMinutes)
	}
	_, err := p.run(ctx, p.shutdownBin, []string{"-r", arg})
	return err
}

// --- locale -----------------------------------------------------------

// langLineRE matches the first uncommented `LANG=...` line. Leading
// whitespace is tolerated; `#` prefixes are not (they're comments).
var langLineRE = regexp.MustCompile(`(?m)^[[:space:]]*LANG=[^\n]*`)

func (p *linuxProvider) ReadLocale(_ context.Context) (string, error) {
	data, err := os.ReadFile(p.localeConfPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("read %s: %w", p.localeConfPath, err)
	}
	return parseLangValue(data), nil
}

// parseLangValue returns the first uncommented LANG= value (with
// surrounding quotes / inline-comment / whitespace stripped), or ""
// when no LANG= line exists.
func parseLangValue(data []byte) string {
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if !strings.HasPrefix(trimmed, "LANG=") {
			continue
		}
		v := strings.TrimPrefix(trimmed, "LANG=")
		if i := strings.IndexByte(v, '#'); i >= 0 {
			v = v[:i]
		}
		v = strings.TrimSpace(v)
		v = strings.Trim(v, `"`)
		return v
	}
	return ""
}

func (p *linuxProvider) WriteLocale(ctx context.Context, lang string) error {
	data, err := os.ReadFile(p.localeConfPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read %s: %w", p.localeConfPath, err)
	}
	perm := os.FileMode(0o644)
	if fi, statErr := os.Stat(p.localeConfPath); statErr == nil {
		perm = fi.Mode().Perm()
	}
	newData := rewriteLangValue(data, lang)
	if err := writeFileAtomic(p.localeConfPath, newData, perm); err != nil {
		return err
	}
	if p.localectlBin != "" {
		if _, err := p.run(ctx, p.localectlBin, []string{"set-locale", "LANG=" + lang}); err != nil {
			return fmt.Errorf("localectl: %w", err)
		}
	}
	return nil
}

// rewriteLangValue replaces the first uncommented LANG= line with
// `LANG=<value>`. If no such line exists, the line is appended.
// Trailing-newline style is preserved.
func rewriteLangValue(data []byte, lang string) []byte {
	repl := []byte("LANG=" + lang)
	if loc := langLineRE.FindIndex(data); loc != nil {
		out := make([]byte, 0, len(data)-(loc[1]-loc[0])+len(repl))
		out = append(out, data[:loc[0]]...)
		out = append(out, repl...)
		out = append(out, data[loc[1]:]...)
		return out
	}
	if len(data) == 0 {
		return append(repl, '\n')
	}
	if data[len(data)-1] != '\n' {
		data = append(data, '\n')
	}
	return append(data, append(repl, '\n')...)
}

// --- helpers -----------------------------------------------------------

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = tmp.Close()
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("write %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close %s: %w", tmpName, err)
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		return fmt.Errorf("chmod %s: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename %s → %s: %w", tmpName, path, err)
	}
	cleanup = false
	return nil
}

// execRun is the production commandRunner. Captures combined output
// so the tool's complaint reaches the operator.
func execRun(ctx context.Context, bin string, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, bin, args...) //nolint:gosec // bin resolved via exec.LookPath; args are fixed shutdown/localectl flags + validated values from a validated declaration
	out, err := cmd.CombinedOutput()
	if err == nil {
		return string(out), nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return "", fmt.Errorf("%s %s: exit %d: %s", bin, strings.Join(args, " "), exitErr.ExitCode(), strings.TrimSpace(string(out)))
	}
	return "", fmt.Errorf("%s %s: %w", bin, strings.Join(args, " "), err)
}
