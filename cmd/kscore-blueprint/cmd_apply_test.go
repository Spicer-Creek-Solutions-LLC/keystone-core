package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyCommandExists(t *testing.T) {
	cmd := newRootCmd()
	applyCmd := findSubcommand(cmd, "apply")
	if applyCmd == nil {
		t.Fatal("apply subcommand not found")
	}
}

func TestApplyCommandHelp(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"apply", "--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("apply --help failed: %v", err)
	}

	output := buf.String()
	for _, expected := range []string{"--var", "--target", "--entrypoint", "--dry-run"} {
		if !strings.Contains(output, expected) {
			t.Errorf("expected help to mention %s", expected)
		}
	}
}

func TestApplyFlags(t *testing.T) {
	cmd := newRootCmd()
	applyCmd := findSubcommand(cmd, "apply")
	if applyCmd == nil {
		t.Fatal("apply subcommand not found")
	}

	expectedFlags := []string{"var", "target", "entrypoint", "dry-run", "dir"}
	for _, flag := range expectedFlags {
		if applyCmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected --%s flag", flag)
		}
	}
}

func TestApplyBlueprintNotFound(t *testing.T) {
	cmd := newRootCmd()
	cmd.SilenceUsage = true
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"apply", "nonexistent/blueprint", "--dir", t.TempDir()})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for nonexistent blueprint")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestApplyLocalPath(t *testing.T) {
	dir := createTestBlueprint(t, "test/local-bp", "1.0.0")

	cmd := newRootCmd()
	cmd.SilenceUsage = true
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"apply", dir, "--dry-run"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "test/local-bp") {
		t.Errorf("expected output to contain blueprint name, got: %s", output)
	}
	if !strings.Contains(output, "[dry-run]") {
		t.Errorf("expected output to contain [dry-run], got: %s", output)
	}
	if !strings.Contains(output, "1.0.0") {
		t.Errorf("expected output to contain version, got: %s", output)
	}
}

func TestApplyWithVars(t *testing.T) {
	dir := createTestBlueprint(t, "test/vars-bp", "0.1.0")

	cmd := newRootCmd()
	cmd.SilenceUsage = true
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"apply", dir, "--var", "domain=myapp.com", "--var", "port=8080", "--dry-run"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("apply with vars failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "domain = myapp.com") {
		t.Errorf("expected output to show domain var, got: %s", output)
	}
	if !strings.Contains(output, "port = 8080") {
		t.Errorf("expected output to show port var, got: %s", output)
	}
}

func TestApplyWithTarget(t *testing.T) {
	dir := createTestBlueprint(t, "test/target-bp", "0.1.0")

	cmd := newRootCmd()
	cmd.SilenceUsage = true
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"apply", dir, "--target", "environment:production", "--dry-run"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("apply with target failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "environment:production") {
		t.Errorf("expected output to show target, got: %s", output)
	}
}

func TestApplyInvalidEntrypoint(t *testing.T) {
	dir := createTestBlueprint(t, "test/entrypoint-bp", "0.1.0")

	cmd := newRootCmd()
	cmd.SilenceUsage = true
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"apply", dir, "--entrypoint", "nonexistent", "--dry-run"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for invalid entrypoint")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestApplyInstalledBlueprint(t *testing.T) {
	// Simulate installed blueprint at baseDir/myorg/web-stack/
	baseDir := t.TempDir()
	bpDir := filepath.Join(baseDir, "myorg", "web-stack")
	writeTestBlueprintAt(t, bpDir, "myorg/web-stack", "2.0.0")

	cmd := newRootCmd()
	cmd.SilenceUsage = true
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"apply", "myorg/web-stack", "--dir", baseDir, "--dry-run"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("apply installed blueprint failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "myorg/web-stack") {
		t.Errorf("expected output to contain blueprint name, got: %s", output)
	}
	if !strings.Contains(output, "2.0.0") {
		t.Errorf("expected output to contain version, got: %s", output)
	}
}

func TestApplySuccessMessage(t *testing.T) {
	dir := createTestBlueprint(t, "test/success-bp", "1.0.0")

	cmd := newRootCmd()
	cmd.SilenceUsage = true
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"apply", dir})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Blueprint applied successfully") {
		t.Errorf("expected success message, got: %s", output)
	}
	if strings.Contains(output, "[dry-run]") {
		t.Errorf("should not contain dry-run marker without --dry-run flag")
	}
}

func TestParseVarFlags(t *testing.T) {
	tests := []struct {
		name    string
		vars    []string
		want    map[string]string
		wantErr bool
	}{
		{
			name: "single var",
			vars: []string{"key=value"},
			want: map[string]string{"key": "value"},
		},
		{
			name: "multiple vars",
			vars: []string{"a=1", "b=2"},
			want: map[string]string{"a": "1", "b": "2"},
		},
		{
			name: "value with equals",
			vars: []string{"url=http://host:8080/path?a=1"},
			want: map[string]string{"url": "http://host:8080/path?a=1"},
		},
		{
			name: "empty value",
			vars: []string{"key="},
			want: map[string]string{"key": ""},
		},
		{
			name:    "no equals sign",
			vars:    []string{"invalid"},
			wantErr: true,
		},
		{
			name:    "equals at start",
			vars:    []string{"=value"},
			wantErr: true,
		},
		{
			name: "empty input",
			vars: nil,
			want: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseVarFlags(tt.vars)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %d vars, want %d", len(got), len(tt.want))
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("var %s: got %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

func TestResolveBlueprintPath(t *testing.T) {
	t.Run("absolute path", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "blueprint.yaml"), []byte("metadata:\n  name: test\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		got, err := resolveBlueprintPath(dir, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != dir {
			t.Errorf("got %s, want %s", got, dir)
		}
	})

	t.Run("installed blueprint", func(t *testing.T) {
		baseDir := t.TempDir()
		bpDir := filepath.Join(baseDir, "mybp")
		if err := os.MkdirAll(bpDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(bpDir, "blueprint.yaml"), []byte("metadata:\n  name: test\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		got, err := resolveBlueprintPath("mybp", baseDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != bpDir {
			t.Errorf("got %s, want %s", got, bpDir)
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := resolveBlueprintPath("nonexistent", t.TempDir())
		if err == nil {
			t.Fatal("expected error for nonexistent blueprint")
		}
	})

	t.Run("no manifest", func(t *testing.T) {
		baseDir := t.TempDir()
		bpDir := filepath.Join(baseDir, "empty")
		if err := os.MkdirAll(bpDir, 0o755); err != nil {
			t.Fatal(err)
		}

		_, err := resolveBlueprintPath("empty", baseDir)
		if err == nil {
			t.Fatal("expected error when no blueprint.yaml")
		}
	})
}

// createTestBlueprint creates a minimal blueprint directory and returns its path.
func createTestBlueprint(t *testing.T, name, version string) string {
	t.Helper()
	dir := t.TempDir()
	writeTestBlueprintAt(t, dir, name, version)
	return dir
}

// writeTestBlueprintAt writes a minimal blueprint at the given directory.
func writeTestBlueprintAt(t *testing.T, dir, name, version string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	manifest := "metadata:\n  name: " + name + "\n  version: \"" + version + "\"\n  description: Test blueprint\nentrypoints:\n  default: states/main.yaml\n"
	if err := os.WriteFile(filepath.Join(dir, "blueprint.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	statesDir := filepath.Join(dir, "states")
	if err := os.MkdirAll(statesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	stateContent := "file:\n  test_file:\n    state: present\n    path: /tmp/test\n    contents: hello\n"
	if err := os.WriteFile(filepath.Join(statesDir, "main.yaml"), []byte(stateContent), 0o644); err != nil {
		t.Fatal(err)
	}
}
