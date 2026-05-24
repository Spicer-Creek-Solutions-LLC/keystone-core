// SPDX-License-Identifier: Apache-2.0

//go:build linux

package security

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// defaultConfigPath is the canonical SELinux config file. Overridden
// in tests via NewLinuxProviderForTests-equivalent helpers (the
// configPath field is exported through the seam below).
const defaultConfigPath = "/etc/selinux/config"

func defaultProvider() Provider {
	p := &linuxProvider{configPath: defaultConfigPath, run: execRun}
	p.getenforceBin, _ = exec.LookPath("getenforce")
	p.setenforceBin, _ = exec.LookPath("setenforce")
	p.getseboolBin, _ = exec.LookPath("getsebool")
	p.setseboolBin, _ = exec.LookPath("setsebool")
	return p
}

type linuxProvider struct {
	getenforceBin string
	setenforceBin string
	getseboolBin  string
	setseboolBin  string
	configPath    string
	run           commandRunner
}

// --- mode ---------------------------------------------------------------

func (p *linuxProvider) GetPersistentMode(_ context.Context) (string, error) {
	data, err := os.ReadFile(p.configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("%w (%s not found)", ErrSELinuxUnavailable, p.configPath)
		}
		return "", fmt.Errorf("read %s: %w", p.configPath, err)
	}
	return parseConfigMode(data)
}

// selinuxLineRE matches the first uncommented `SELINUX=...` line. We
// use `(?m)^[[:space:]]*SELINUX=` so a leading tab or spaces is
// tolerated; a `#` prefix is not matched (a comment).
var selinuxLineRE = regexp.MustCompile(`(?m)^[[:space:]]*SELINUX=[^\n]*`)

func parseConfigMode(data []byte) (string, error) {
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if !strings.HasPrefix(trimmed, "SELINUX=") {
			continue
		}
		v := strings.TrimPrefix(trimmed, "SELINUX=")
		if i := strings.IndexByte(v, '#'); i >= 0 {
			v = v[:i]
		}
		v = strings.TrimSpace(v)
		v = strings.Trim(v, `"`)
		v = strings.ToLower(v)
		if _, ok := validModes[v]; !ok {
			return "", fmt.Errorf("%s: unrecognised SELINUX=%q", "/etc/selinux/config", v)
		}
		return v, nil
	}
	return "", fmt.Errorf("SELINUX= line not found in config")
}

func (p *linuxProvider) SetPersistentMode(_ context.Context, mode string) error {
	data, err := os.ReadFile(p.configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w (%s not found)", ErrSELinuxUnavailable, p.configPath)
		}
		return fmt.Errorf("read %s: %w", p.configPath, err)
	}
	fi, statErr := os.Stat(p.configPath)
	perm := os.FileMode(0o644)
	if statErr == nil {
		perm = fi.Mode().Perm()
	}
	newData := rewriteConfigMode(data, mode)
	return writeFileAtomic(p.configPath, newData, perm)
}

// rewriteConfigMode replaces the first uncommented `SELINUX=` line
// with `SELINUX=<mode>`. If no such line exists, the line is
// appended. Trailing-newline style is preserved.
func rewriteConfigMode(data []byte, mode string) []byte {
	repl := []byte("SELINUX=" + mode)
	if loc := selinuxLineRE.FindIndex(data); loc != nil {
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

// --- runtime ------------------------------------------------------------

func (p *linuxProvider) GetRuntimeMode(ctx context.Context) (string, error) {
	if p.getenforceBin == "" {
		return "", fmt.Errorf("%w (getenforce missing)", ErrSELinuxUnavailable)
	}
	out, err := p.run(ctx, p.getenforceBin, nil)
	if err != nil {
		return "", err
	}
	v := strings.TrimSpace(out)
	switch v {
	case "Enforcing":
		return ModeEnforcing, nil
	case "Permissive":
		return ModePermissive, nil
	case "Disabled":
		return ModeDisabled, nil
	}
	return "", fmt.Errorf("unexpected getenforce output: %q", v)
}

func (p *linuxProvider) SetRuntimeMode(ctx context.Context, mode string) error {
	if p.setenforceBin == "" {
		return fmt.Errorf("%w (setenforce missing)", ErrSELinuxUnavailable)
	}
	var arg string
	switch mode {
	case ModeEnforcing:
		arg = "1"
	case ModePermissive:
		arg = "0"
	case ModeDisabled:
		return fmt.Errorf("SELinux cannot be disabled at runtime — reboot required")
	default:
		return fmt.Errorf("unknown mode %q", mode)
	}
	_, err := p.run(ctx, p.setenforceBin, []string{arg})
	return err
}

// --- booleans -----------------------------------------------------------

// boolOutRE matches `getsebool` output: "NAME --> on" / "NAME --> off"
// (with arbitrary whitespace around `-->`).
var boolOutRE = regexp.MustCompile(`^\S+\s*-->\s*(on|off)\s*$`)

func (p *linuxProvider) GetBoolean(ctx context.Context, name string) (bool, error) {
	if p.getseboolBin == "" {
		return false, fmt.Errorf("%w (getsebool missing)", ErrSELinuxUnavailable)
	}
	out, err := p.run(ctx, p.getseboolBin, []string{name})
	if err != nil {
		return false, err
	}
	line := strings.TrimSpace(out)
	m := boolOutRE.FindStringSubmatch(line)
	if m == nil {
		return false, fmt.Errorf("unexpected getsebool output: %q", line)
	}
	return m[1] == "on", nil
}

func (p *linuxProvider) SetBoolean(ctx context.Context, name string, value bool) error {
	if p.setseboolBin == "" {
		return fmt.Errorf("%w (setsebool missing)", ErrSELinuxUnavailable)
	}
	val := "off"
	if value {
		val = "on"
	}
	_, err := p.run(ctx, p.setseboolBin, []string{"-P", name + "=" + val})
	return err
}

// --- exec ---------------------------------------------------------------

// execRun is the production commandRunner. Captures combined output
// so the SELinux tool's complaint reaches the operator.
func execRun(ctx context.Context, bin string, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, bin, args...) //nolint:gosec // bin resolved via exec.LookPath; args are fixed SELinux-tool flags + a validated boolean name from a validated declaration
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
