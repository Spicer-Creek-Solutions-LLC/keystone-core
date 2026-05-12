//go:build linux

package sysctl

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

// procSysRoot is the kernel sysctl filesystem root; variable for
// tests.
var procSysRoot = "/proc/sys"

// sysctlConfDir is the drop-in directory for persisted settings;
// variable for tests.
var sysctlConfDir = "/etc/sysctl.d"

// sysctlBin is resolved at construction time via exec.LookPath.
type linuxProvider struct {
	sysctlBin string
	runner    commandRunner
}

// commandRunner is the injection point for `sysctl -w`. Production
// wires execRun; tests inject a capturing shim.
type commandRunner func(ctx context.Context, bin string, args []string) error

func defaultProvider() Provider {
	bin, err := exec.LookPath("sysctl")
	if err != nil {
		// /proc/sys is usually present even when the `sysctl`
		// binary isn't (busybox lacks it on some minimal images).
		// We still allow Get + persist-file ops; Set will fail
		// with a clear "sysctl binary not found" error.
		bin = ""
	}
	return &linuxProvider{sysctlBin: bin, runner: execRun}
}

func (p *linuxProvider) Get(key string) (string, bool, error) {
	path := filepath.Join(procSysRoot, keyToPath(key))
	data, err := os.ReadFile(path) //nolint:gosec // path is procSysRoot + a validated key
	if errors.Is(err, fs.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("read %s: %w", path, err)
	}
	// /proc/sys values can be multi-line for some keys; trim and
	// collapse internal runs of whitespace to single spaces so the
	// comparison matches `sysctl -n` formatting.
	return normalizeValue(string(data)), true, nil
}

func (p *linuxProvider) Set(ctx context.Context, key, value string) error {
	if p.sysctlBin == "" {
		return fmt.Errorf("sysctl: `sysctl` binary not found on PATH")
	}
	return p.runner(ctx, p.sysctlBin, []string{"-w", normalizeKey(key) + "=" + value})
}

func (p *linuxProvider) ReadPersist(key string) (string, bool, error) {
	key = normalizeKey(key)
	path := persistFilePath(key)
	f, err := os.Open(path) //nolint:gosec // path derived from a validated key under a fixed dir
	if errors.Is(err, fs.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		if normalizeKey(strings.TrimSpace(k)) == key {
			return normalizeValue(v), true, nil
		}
	}
	if err := sc.Err(); err != nil {
		return "", false, fmt.Errorf("scan %s: %w", path, err)
	}
	return "", false, nil
}

func (p *linuxProvider) WritePersist(key, value string) error {
	key = normalizeKey(key)
	path := persistFilePath(key)
	dir := filepath.Dir(path)
	if err := mkdirAll(dir); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	content := fmt.Sprintf("# managed by keystone-core\n%s = %s\n", key, value)
	return writeFileAtomic(path, []byte(content))
}

// keyToPath converts a sysctl key (dotted or slashed) to its
// /proc/sys path. Normalises to the dotted form first so both
// notations resolve to the same path.
func keyToPath(key string) string {
	return strings.ReplaceAll(normalizeKey(key), ".", "/")
}

// persistFilePath returns the keystone-managed drop-in for key.
// Dots are filename-safe; the key is normalised first so the
// dotted and slashed notations resolve to the same file.
func persistFilePath(key string) string {
	return filepath.Join(sysctlConfDir, "99-keystone-"+normalizeKey(key)+".conf")
}

// normalizeValue trims and collapses whitespace so multi-field
// values ("4096   16384   4194304") compare equal regardless of the
// exact spacing the kernel or the operator used.
func normalizeValue(s string) string {
	return strings.Join(strings.Fields(s), " ")
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
	if err := os.WriteFile(tmp, data, 0o644); err != nil { //nolint:gosec // sysctl.d configs are world-readable
		return fmt.Errorf("write tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename %s: %w", path, err)
	}
	return nil
}

func execRun(ctx context.Context, bin string, args []string) error {
	cmd := exec.CommandContext(ctx, bin, args...) //nolint:gosec // bin resolved via exec.LookPath; args are -w + validated key=value
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
