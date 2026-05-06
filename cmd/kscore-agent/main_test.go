package main

import (
	"context"
	"testing"
)

func TestNewCommand(t *testing.T) {
	cmd := newCommand()
	if cmd.Use != "kscore-agent" {
		t.Errorf("Use = %q, want %q", cmd.Use, "kscore-agent")
	}
	if cmd.Short == "" {
		t.Error("Short is empty")
	}
}

// Run blocks until ctx cancels; verify that contract holds.
func TestRun_ReturnsOnCtxCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := run(ctx, nil, nil); err != nil {
		t.Fatalf("run returned error: %v", err)
	}
}
