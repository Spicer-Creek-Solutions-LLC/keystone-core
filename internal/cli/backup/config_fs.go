package backup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	bkp "go.keystone-core.io/keystone-core/internal/backup"
)

// FilesystemConfigCollector reads operator-supplied config files
// from local paths and hands them to [bkp.BackupManager] as the
// concrete [bkp.ConfigCollector] implementation. Each file becomes
// one [bkp.ConfigFile] keyed by its basename — the original
// directory path is dropped so restore can place files under an
// operator-chosen output dir without leaking the source layout.
type FilesystemConfigCollector struct {
	Paths []string
}

// Collect reads every Path in order and returns the resulting
// ConfigFile slice. A read error aborts the collection with the
// offending path wrapped in.
func (c *FilesystemConfigCollector) Collect(_ context.Context) ([]bkp.ConfigFile, error) {
	out := make([]bkp.ConfigFile, 0, len(c.Paths))
	for _, p := range c.Paths {
		b, err := os.ReadFile(p) //nolint:gosec // operator-supplied config path
		if err != nil {
			return nil, fmt.Errorf("backup: collect %q: %w", p, err)
		}
		out = append(out, bkp.ConfigFile{Name: filepath.Base(p), Body: b})
	}
	return out, nil
}

// FilesystemConfigRestore writes the restored [bkp.ConfigFile] list
// to Dir, creating the directory if needed. File names are passed
// through [filepath.Base] again on the write side as a defense-in-
// depth guard against a malicious manifest carrying a basename like
// "../../etc/passwd".
type FilesystemConfigRestore struct {
	Dir string
}

// Restore writes every file to Dir/<basename>. Existing files are
// overwritten with mode 0o600; the directory is created with mode
// 0o750 if absent (group-readable for ops shared accounts; world bits
// off because the contents may include kscore-server config that
// names secret paths).
func (r *FilesystemConfigRestore) Restore(_ context.Context, files []bkp.ConfigFile) error {
	if r.Dir == "" {
		return fmt.Errorf("backup: FilesystemConfigRestore.Dir must not be empty")
	}
	if err := os.MkdirAll(r.Dir, 0o750); err != nil {
		return fmt.Errorf("backup: mkdir %q: %w", r.Dir, err)
	}
	for _, f := range files {
		name := filepath.Base(f.Name)
		if name == "." || name == "/" || name == "" {
			return fmt.Errorf("backup: rejected config name %q from manifest", f.Name)
		}
		path := filepath.Join(r.Dir, name)
		if err := os.WriteFile(path, f.Body, 0o600); err != nil {
			return fmt.Errorf("backup: write %q: %w", path, err)
		}
	}
	return nil
}
