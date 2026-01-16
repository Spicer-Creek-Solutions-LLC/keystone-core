package bootstrap

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBlueprintPhaseLoadsBlueprints(t *testing.T) {
	tmpDir := t.TempDir()
	bpDir := filepath.Join(tmpDir, "community", "demo")
	if err := os.MkdirAll(bpDir, 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	manifest := `apiVersion: blueprints.kscore.io/v1
kind: Blueprint
metadata:
  name: demo
  version: 1.0.0
entrypoints:
  default: states/default.yaml
`
	if err := os.WriteFile(filepath.Join(bpDir, "blueprint.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest failed: %v", err)
	}
	stateContent := `cmd:
  "echo hello":
    state: run
`
	statePath := filepath.Join(bpDir, "states")
	if err := os.MkdirAll(statePath, 0o755); err != nil {
		t.Fatalf("mkdir states failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(statePath, "default.yaml"), []byte(stateContent), 0o644); err != nil {
		t.Fatalf("write state failed: %v", err)
	}

	buf := new(bytes.Buffer)
	state := &State{
		Output:          buf,
		Verbose:         true,
		DryRun:          true,
		BootstrapConfig: &BootstrapConfig{BlueprintsDir: tmpDir, ApplyBlueprints: []string{"blueprints/community/demo"}},
	}

	if err := blueprintPhase(context.Background(), state); err != nil {
		t.Fatalf("blueprintPhase returned error: %v", err)
	}
}

func TestBlueprintPhaseRunsHooksAndVerify(t *testing.T) {
	tmpDir := t.TempDir()
	bpDir := filepath.Join(tmpDir, "community", "hooks")
	if err := os.MkdirAll(filepath.Join(bpDir, "states", "hooks"), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	manifest := `apiVersion: blueprints.kscore.io/v1
kind: Blueprint
metadata:
  name: hooks
  version: 1.0.0
entrypoints:
  default: states/default.yaml
  verify: states/hooks/verify.yaml
hooks:
  pre_apply:
    - states/hooks/pre.yaml
  post_apply:
    - states/hooks/post.yaml
`
	if err := os.WriteFile(filepath.Join(bpDir, "blueprint.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest failed: %v", err)
	}

	stateContent := `cmd:
  "echo hello":
    state: run
`
	for _, file := range []string{"default.yaml", "hooks/pre.yaml", "hooks/post.yaml", "hooks/verify.yaml"} {
		path := filepath.Join(bpDir, "states", file)
		if err := os.WriteFile(path, []byte(stateContent), 0o644); err != nil {
			t.Fatalf("write state failed: %v", err)
		}
	}

	buf := new(bytes.Buffer)
	state := &State{
		Output:          buf,
		Verbose:         true,
		DryRun:          true,
		BootstrapConfig: &BootstrapConfig{BlueprintsDir: tmpDir, ApplyBlueprints: []string{"blueprints/community/hooks"}},
	}

	if err := blueprintPhase(context.Background(), state); err != nil {
		t.Fatalf("blueprintPhase returned error: %v", err)
	}

	output := buf.String()
	for _, expected := range []string{"pre_apply state file", "apply state file", "post_apply state file", "verify state file"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("expected output to include %q, got %s", expected, output)
		}
	}
}

func TestBlueprintPhaseExportsStates(t *testing.T) {
	tmpDir := t.TempDir()
	bpDir := filepath.Join(tmpDir, "community", "export")
	if err := os.MkdirAll(filepath.Join(bpDir, "states"), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	manifest := `apiVersion: blueprints.kscore.io/v1
kind: Blueprint
metadata:
  name: export
  version: 1.0.0
entrypoints:
  default: states/default.yaml
`
	if err := os.WriteFile(filepath.Join(bpDir, "blueprint.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest failed: %v", err)
	}
	stateContent := `cmd:
  "echo hello":
    state: run
`
	if err := os.WriteFile(filepath.Join(bpDir, "states", "default.yaml"), []byte(stateContent), 0o644); err != nil {
		t.Fatalf("write state failed: %v", err)
	}
	exportDir := filepath.Join(tmpDir, "exported")

	buf := new(bytes.Buffer)
	state := &State{
		Output:          buf,
		Verbose:         true,
		DryRun:          true,
		BootstrapConfig: &BootstrapConfig{BlueprintsDir: tmpDir, ApplyBlueprints: []string{"blueprints/community/export"}, ExportStatesDir: exportDir},
	}

	if err := blueprintPhase(context.Background(), state); err != nil {
		t.Fatalf("blueprintPhase returned error: %v", err)
	}
	files, err := os.ReadDir(exportDir)
	if err != nil {
		t.Fatalf("read export dir failed: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("expected exported states")
	}
}
