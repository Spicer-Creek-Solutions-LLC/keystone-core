// SPDX-License-Identifier: Apache-2.0

//go:build linux

package timer

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// unitDir is where generated .timer files live; variable for tests.
var unitDir = "/etc/systemd/system"

func defaultProvider() Provider {
	bin, err := exec.LookPath("systemctl")
	if err != nil {
		return &noSystemdProvider{}
	}
	return &systemdProvider{systemctl: bin, run: execRun}
}

type systemdProvider struct {
	systemctl string
	run       commandRunner
}

func (p *systemdProvider) ReadUnit(name string) (string, bool, error) {
	data, err := os.ReadFile(filepath.Join(unitDir, name)) //nolint:gosec // unitDir + a regex-validated unit name
	if errors.Is(err, fs.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("read %s: %w", name, err)
	}
	return string(data), true, nil
}

func (p *systemdProvider) WriteUnit(name, content string) error {
	if err := mkdirAll(unitDir); err != nil {
		return fmt.Errorf("mkdir %s: %w", unitDir, err)
	}
	return writeFileAtomic(filepath.Join(unitDir, name), []byte(content))
}

func (p *systemdProvider) RemoveUnit(name string) error {
	err := os.Remove(filepath.Join(unitDir, name))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("remove %s: %w", name, err)
	}
	return nil
}

func (p *systemdProvider) DaemonReload(ctx context.Context) error {
	_, err := p.run(ctx, p.systemctl, []string{"daemon-reload"})
	return err
}

func (p *systemdProvider) Status(ctx context.Context, name string) (*TimerStatus, error) {
	out, err := p.run(ctx, p.systemctl, []string{
		"show", name, "-p", "LoadState", "-p", "ActiveState", "-p", "UnitFileState",
	})
	if err != nil {
		// `systemctl show` exits 0 even for a nonexistent unit
		// (LoadState=not-found), so a real error here means
		// something structural (no PID 1 systemd, etc).
		return nil, fmt.Errorf("systemctl show %s: %w", name, err)
	}
	return parseShow(name, out)
}

func (p *systemdProvider) EnableNow(ctx context.Context, name string) error {
	_, err := p.run(ctx, p.systemctl, []string{"enable", "--now", name})
	return err
}

func (p *systemdProvider) DisableStop(ctx context.Context, name string) error {
	_, err := p.run(ctx, p.systemctl, []string{"disable", "--now", name})
	return err
}

// parseShow reads the KEY=VALUE output of
//
//	systemctl show <name> -p LoadState -p ActiveState -p UnitFileState
//
// LoadState=loaded → unit exists; not-found/bad-setting/error →
// Exists=false; masked → exists but never enabled/active. ActiveState
// active → Active=true. UnitFileState enabled/enabled-runtime/static/
// indirect/alias → Enabled=true; disabled/masked/linked/generated/
// transient/"" → Enabled=false. Unknown LoadState/UnitFileState
// values are an error so a format change is loud (mirrors the service
// module's parser).
func parseShow(name, out string) (*TimerStatus, error) {
	fields := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("systemctl show: unexpected line %q", line)
		}
		fields[k] = v
	}
	loadState, ok := fields["LoadState"]
	if !ok {
		return nil, fmt.Errorf("systemctl show: missing LoadState for %s", name)
	}
	st := &TimerStatus{}
	switch loadState {
	case "loaded":
		st.Exists = true
	case "not-found", "bad-setting", "error":
		return st, nil // Exists=false; Active/Enabled meaningless
	case "masked":
		st.Exists = true
		return st, nil // masked: exists but can't run; treat as not-enabled/not-active
	default:
		return nil, fmt.Errorf("systemctl show: unknown LoadState %q for %s", loadState, name)
	}
	st.Active = fields["ActiveState"] == "active"
	switch ufs := fields["UnitFileState"]; ufs {
	case "enabled", "enabled-runtime", "static", "indirect", "alias":
		st.Enabled = true
	case "disabled", "masked", "masked-runtime", "linked", "linked-runtime", "generated", "transient", "":
		st.Enabled = false
	default:
		return nil, fmt.Errorf("systemctl show: unknown UnitFileState %q for %s", ufs, name)
	}
	return st, nil
}

// mkdirAll creates dir with the conventional 0755 perms used by
// /etc/systemd/system. gosec flags 0755 on MkdirAll; here it's a
// no-op when the dir already exists (the common case) and 0755 is
// the documented norm for that directory.
func mkdirAll(dir string) error {
	return os.MkdirAll(dir, 0o755) //nolint:gosec // see comment above
}

// writeFileAtomic writes data to path via write-temp-then-rename.
// Unit files are world-readable (0644) by convention.
func writeFileAtomic(path string, data []byte) error {
	tmp := path + ".keystone.tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil { //nolint:gosec // systemd unit files are world-readable
		return fmt.Errorf("write tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename %s: %w", path, err)
	}
	return nil
}

// execRun is the production commandRunner. Captures combined output
// so systemctl's complaint reaches the operator.
func execRun(ctx context.Context, bin string, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, bin, args...) //nolint:gosec // bin resolved via exec.LookPath; args are fixed verbs + a regex-validated unit name
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

// noSystemdProvider stands in when `systemctl` is absent. Unit-file
// reads report "not present" (so `absent` declarations match and
// `present` ones drift); every mutating operation fails with
// ErrNoBackend — without systemd a timer can't be armed anyway.
type noSystemdProvider struct{}

func (*noSystemdProvider) ReadUnit(string) (string, bool, error) { return "", false, nil }
func (*noSystemdProvider) WriteUnit(string, string) error        { return ErrNoBackend }
func (*noSystemdProvider) RemoveUnit(string) error               { return ErrNoBackend }
func (*noSystemdProvider) DaemonReload(context.Context) error    { return ErrNoBackend }
func (*noSystemdProvider) Status(context.Context, string) (*TimerStatus, error) {
	return nil, ErrNoBackend
}
func (*noSystemdProvider) EnableNow(context.Context, string) error   { return ErrNoBackend }
func (*noSystemdProvider) DisableStop(context.Context, string) error { return ErrNoBackend }
