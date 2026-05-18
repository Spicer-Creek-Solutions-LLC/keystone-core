package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.keystone-core.io/keystone-core/internal/cli/module"
	"go.keystone-core.io/keystone-core/pkg/module/manifest"
)

func writeModule(t *testing.T, testSrc string) string {
	t.Helper()
	dir := t.TempDir()
	m := &manifest.Manifest{
		Name: "acme/mod", Version: "1.0.0",
		Type: manifest.TypeStarlark, Entrypoint: "main.star",
	}
	my, err := manifest.MarshalManifest(m)
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, "manifest.yaml"), my)
	mustWrite(t, filepath.Join(dir, "main.star"), []byte("def main(input):\n    return {}\n"))
	mustWrite(t, filepath.Join(dir, "m_test.star"), []byte(testSrc))
	return dir
}

func mustWrite(t *testing.T, p string, b []byte) {
	t.Helper()
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestRun_TestCommand(t *testing.T) {
	t.Run("all pass -> exit 0", func(t *testing.T) {
		dir := writeModule(t, "def test_ok():\n    assert.eq(2, 1 + 1)\n")
		var out, errb bytes.Buffer
		if code := run([]string{"test", dir}, &out, &errb); code != 0 {
			t.Fatalf("exit %d, stderr=%s", code, errb.String())
		}
		if !strings.Contains(out.String(), "tests: 1 passed, 0 failed") {
			t.Fatalf("stdout = %q", out.String())
		}
	})

	t.Run("a failing test -> exit 1", func(t *testing.T) {
		dir := writeModule(t, "def test_ok():\n    assert.true(True)\n\ndef test_bad():\n    assert.fail('x')\n")
		var out, errb bytes.Buffer
		if code := run([]string{"test", dir}, &out, &errb); code != 1 {
			t.Fatalf("exit %d, want 1", code)
		}
		if !strings.Contains(out.String(), "1 passed, 1 failed") {
			t.Fatalf("stdout = %q", out.String())
		}
	})

	t.Run("infra error (missing dir) -> exit 1", func(t *testing.T) {
		var out, errb bytes.Buffer
		if code := run([]string{"test", filepath.Join(t.TempDir(), "nope")}, &out, &errb); code != 1 {
			t.Fatalf("exit %d, want 1", code)
		}
	})

	t.Run("--help -> exit 0", func(t *testing.T) {
		var out, errb bytes.Buffer
		if code := run([]string{"--help"}, &out, &errb); code != 0 {
			t.Fatalf("exit %d", code)
		}
	})
}

func TestTestRunnerAdapter(t *testing.T) {
	dir := writeModule(t, "def test_ok():\n    assert.true(True)\n")
	passed, failed, err := (testRunner{}).RunTests(context.Background(), dir, module.AuditOptions{})
	if err != nil || passed != 1 || failed != 0 {
		t.Fatalf("RunTests = %d,%d,%v", passed, failed, err)
	}
	if _, _, err := (testRunner{}).RunTests(context.Background(), filepath.Join(t.TempDir(), "x"), module.AuditOptions{}); err == nil {
		t.Fatal("missing module dir should error")
	}
}
