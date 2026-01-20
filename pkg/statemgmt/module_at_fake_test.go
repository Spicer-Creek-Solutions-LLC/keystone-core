// Package statemgmt provides state management modules.
package statemgmt

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAtModule_CheckAndApply_WithFakeCommands(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("at module not supported on Windows")
	}

	tmpDir := t.TempDir()
	writeFakeCommand(t, tmpDir, "atq", `#!/bin/sh
echo "$ATQ_OUTPUT"
`)
	writeFakeCommand(t, tmpDir, "at", `#!/bin/sh
if [ "$1" = "-c" ]; then
  echo "# Keystone Core: ${AT_JOB_NAME}"
  exit 0
fi
echo "job 123 at Wed Jan 1 00:00:00 2025"
`)
	writeFakeCommand(t, tmpDir, "atrm", "#!/bin/sh\nexit 0\n")

	origPath := os.Getenv("PATH")
	t.Setenv("PATH", tmpDir+string(os.PathListSeparator)+origPath)

	module := NewAtModule()
	decl := &StateDeclaration{
		ID:     "job-1",
		Module: "at",
		State:  "present",
		Parameters: map[string]interface{}{
			"command": "echo hello",
			"time":    "now + 1 hour",
		},
	}

	t.Setenv("AT_JOB_NAME", decl.ID)
	t.Setenv("ATQ_OUTPUT", "123\tWed Jan 1 00:00:00 2025")

	check, err := module.Check(context.Background(), decl)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if !check.Matches {
		t.Fatal("Expected Check to match when job exists")
	}

	t.Setenv("ATQ_OUTPUT", "")
	check, err = module.Check(context.Background(), decl)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if check.Matches {
		t.Fatal("Expected Check to not match when job is absent")
	}

	apply, err := module.Apply(context.Background(), decl)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if !apply.Success || !apply.Changed {
		t.Fatalf("Expected apply to succeed and change, got success=%v changed=%v", apply.Success, apply.Changed)
	}
	if !strings.Contains(apply.Comment, "created") {
		t.Fatalf("Expected create comment, got %q", apply.Comment)
	}

	decl.State = "absent"
	t.Setenv("ATQ_OUTPUT", "123\tWed Jan 1 00:00:00 2025")
	apply, err = module.Apply(context.Background(), decl)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if !apply.Success || !apply.Changed {
		t.Fatalf("Expected apply to remove job, got success=%v changed=%v", apply.Success, apply.Changed)
	}
}

func writeFakeCommand(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0755); err != nil {
		t.Fatalf("Failed to write fake command %s: %v", name, err)
	}
}
