//go:build linux

package kmod

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// procModules is the loaded-modules pseudo-file; variable for tests.
var procModules = "/proc/modules"

// modulesLoadDir is the boot-time auto-load drop-in directory;
// variable for tests.
var modulesLoadDir = "/etc/modules-load.d"

type linuxProvider struct {
	modprobeBin string
	runner      commandRunner
}

// commandRunner is the injection point for modprobe. Production
// wires execRun; tests inject a capturing shim.
type commandRunner func(ctx context.Context, bin string, args []string) error

func defaultProvider() Provider {
	bin, err := exec.LookPath("modprobe")
	if err != nil {
		bin = "" // Loaded + persist ops still work; Load/Unload error clearly.
	}
	return &linuxProvider{modprobeBin: bin, runner: execRun}
}

func (p *linuxProvider) Loaded(name string) (bool, error) {
	name = normalizeName(name)
	f, err := os.Open(procModules) //nolint:gosec // fixed pseudo-file path
	if err != nil {
		return false, fmt.Errorf("open %s: %w", procModules, err)
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		// /proc/modules lines: "<name> <size> <refcount> <deps> <state> <addr>"
		fields := strings.Fields(sc.Text())
		if len(fields) == 0 {
			continue
		}
		if normalizeName(fields[0]) == name {
			return true, nil
		}
	}
	if err := sc.Err(); err != nil {
		return false, fmt.Errorf("scan %s: %w", procModules, err)
	}
	return false, nil
}

func (p *linuxProvider) Load(ctx context.Context, name string) error {
	if p.modprobeBin == "" {
		return fmt.Errorf("kernel_module: `modprobe` binary not found on PATH")
	}
	return p.runner(ctx, p.modprobeBin, []string{normalizeName(name)})
}

func (p *linuxProvider) Unload(ctx context.Context, name string) error {
	if p.modprobeBin == "" {
		return fmt.Errorf("kernel_module: `modprobe` binary not found on PATH")
	}
	return p.runner(ctx, p.modprobeBin, []string{"-r", normalizeName(name)})
}

func (p *linuxProvider) PersistExists(name string) (bool, error) {
	_, err := os.Stat(persistFilePath(name))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (p *linuxProvider) AddPersist(name string) error {
	name = normalizeName(name)
	path := persistFilePath(name)
	dir := filepath.Dir(path)
	if err := mkdirAll(dir); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	content := fmt.Sprintf("# managed by keystone-core\n%s\n", name)
	return writeFileAtomic(path, []byte(content))
}

func (p *linuxProvider) RemovePersist(name string) error {
	err := os.Remove(persistFilePath(name))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

func persistFilePath(name string) string {
	return filepath.Join(modulesLoadDir, "keystone-"+normalizeName(name)+".conf")
}

// mkdirAll creates dir with the conventional 0755 perms used by
// /etc/sysctl.d and /etc/modules-load.d. gosec flags 0755 on
// MkdirAll; here it's a no-op when the dir already exists (the
// common case) and 0755 is the documented norm for these
// drop-in directories.
func mkdirAll(dir string) error {
	return os.MkdirAll(dir, 0o755) //nolint:gosec // see comment above
}

func writeFileAtomic(path string, data []byte) error {
	tmp := path + ".keystone.tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil { //nolint:gosec // modules-load.d configs are world-readable
		return fmt.Errorf("write tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename %s: %w", path, err)
	}
	return nil
}

func execRun(ctx context.Context, bin string, args []string) error {
	cmd := exec.CommandContext(ctx, bin, args...) //nolint:gosec // bin resolved via exec.LookPath; args are -r? + a validated module name
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return fmt.Errorf("%s %s: exit %d: %s", bin, strings.Join(args, " "), exitErr.ExitCode(), strings.TrimSpace(string(out)))
	}
	return fmt.Errorf("%s %s: %w", bin, strings.Join(args, " "), err)
}
