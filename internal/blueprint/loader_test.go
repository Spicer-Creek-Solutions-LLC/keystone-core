package blueprint

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

const validManifestYAML = `
metadata:
  name: demo
  version: 1.0.0
  description: demo blueprint
entrypoints:
  default: demo.apply
parameters:
  replicas:
    type: integer
    default: 1
`

func writeManifest(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ManifestFilename), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestLoad(t *testing.T) {
	t.Run("from directory", func(t *testing.T) {
		dir := writeManifest(t, validManifestYAML)
		m, err := Load(dir)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if m.Metadata.Name != "demo" || m.SourcePath != dir {
			t.Errorf("got name=%q sourcePath=%q", m.Metadata.Name, m.SourcePath)
		}
	})

	t.Run("from file path", func(t *testing.T) {
		dir := writeManifest(t, validManifestYAML)
		m, err := Load(filepath.Join(dir, ManifestFilename))
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if m.SourcePath != dir {
			t.Errorf("SourcePath=%q want %q", m.SourcePath, dir)
		}
	})

	t.Run("missing path", func(t *testing.T) {
		_, err := Load(filepath.Join(t.TempDir(), "nope"))
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("err=%v want ErrNotFound", err)
		}
	})

	t.Run("directory without manifest", func(t *testing.T) {
		_, err := Load(t.TempDir())
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("err=%v want ErrNotFound", err)
		}
	})

	t.Run("unknown field is rejected", func(t *testing.T) {
		dir := writeManifest(t, validManifestYAML+"\nbogus_top_level: true\n")
		_, err := Load(dir)
		if err == nil || errors.Is(err, ErrNotFound) {
			t.Fatalf("expected strict decode error, got %v", err)
		}
	})

	t.Run("malformed yaml", func(t *testing.T) {
		dir := writeManifest(t, "metadata: [unterminated")
		if _, err := Load(dir); err == nil {
			t.Fatal("expected decode error")
		}
	})

	t.Run("valid yaml invalid manifest", func(t *testing.T) {
		dir := writeManifest(t, "metadata:\n  version: 1.0.0\nentrypoints:\n  default: x\n")
		_, err := Load(dir)
		if !errors.Is(err, ErrInvalidManifest) {
			t.Fatalf("err=%v want ErrInvalidManifest", err)
		}
	})
}
