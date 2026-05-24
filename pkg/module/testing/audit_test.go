// SPDX-License-Identifier: Apache-2.0

package moduletest

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/pkg/module/audit"
)

func TestNewAuditor_InvalidLevel(t *testing.T) {
	if _, _, err := newAuditor(AuditOptions{Level: "verbose"}); !errors.Is(err, ErrAuditOption) {
		t.Fatalf("err = %v, want ErrAuditOption", err)
	}
}

func TestNewAuditor_NoOutputIsNoop(t *testing.T) {
	a, closeFn, err := newAuditor(AuditOptions{Level: "all"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := a.(audit.NoopAuditor); !ok {
		t.Fatalf("want NoopAuditor, got %T", a)
	}
	if err := closeFn(); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func TestNewAuditor_StdoutStderr(t *testing.T) {
	for _, out := range []string{"stdout", "stderr"} {
		a, closeFn, err := newAuditor(AuditOptions{Output: out})
		if err != nil {
			t.Fatalf("%s: %v", out, err)
		}
		if _, ok := a.(*jsonAuditor); !ok {
			t.Fatalf("%s: want *jsonAuditor, got %T", out, a)
		}
		_ = closeFn()
	}
}

func TestNewAuditor_FileWritesAndFilters(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	a, closeFn, err := newAuditor(AuditOptions{Level: "failure", Output: path})
	if err != nil {
		t.Fatalf("newAuditor: %v", err)
	}
	ctx := context.Background()
	a.Emit(ctx, audit.Entry{Capability: "kv", Operation: "set", Success: true,
		Timestamp: time.Now(), Duration: 2 * time.Millisecond})
	a.Emit(ctx, audit.Entry{Capability: "fs.write", Operation: "write", Success: false,
		Timestamp: time.Now(), Details: map[string]string{"path": "/etc"}})
	if err := closeFn(); err != nil {
		t.Fatalf("close: %v", err)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 1 {
		t.Fatalf("failOnly should drop the success entry, got %d lines: %q", len(lines), b)
	}
	if !strings.Contains(lines[0], `"capability":"fs.write"`) ||
		!strings.Contains(lines[0], `"success":false`) {
		t.Fatalf("unexpected entry: %s", lines[0])
	}
}

func TestNewAuditor_FileOpenError(t *testing.T) {
	// A path whose parent is not a directory cannot be opened.
	bad := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(bad, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := newAuditor(AuditOptions{Output: filepath.Join(bad, "nested")}); !errors.Is(err, ErrAuditOption) {
		t.Fatalf("err = %v, want ErrAuditOption", err)
	}
}

func TestJSONAuditor_AllLevelKeepsSuccess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.jsonl")
	a, closeFn, err := newAuditor(AuditOptions{Level: "all", Output: path})
	if err != nil {
		t.Fatal(err)
	}
	a.Emit(context.Background(), audit.Entry{Capability: "log", Success: true})
	_ = closeFn()
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), `"capability":"log"`) {
		t.Fatalf("all level must keep success entries: %q", b)
	}
}
