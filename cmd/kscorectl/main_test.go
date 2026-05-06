package main

import (
	"context"
	"testing"
)

func TestNewCommand(t *testing.T) {
	cmd := newCommand()
	if cmd.Use != "kscorectl" {
		t.Errorf("Use = %q, want %q", cmd.Use, "kscorectl")
	}
	if cmd.Short == "" {
		t.Error("Short is empty")
	}
}

// Run is a no-op for the v1.0 hello-world; verify it returns nil immediately
// without blocking on ctx.
func TestRun_ReturnsImmediately(t *testing.T) {
	if err := run(context.Background(), nil, nil); err != nil {
		t.Fatalf("run returned error: %v", err)
	}
}
