package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"go.keystone-core.io/keystone-core/internal/cli/identity"
)

// TestRootHelp confirms the kscore-identity binary's command tree
// is wired correctly: `--help` lists the three top-level
// subcommands (token / ca / status) without contacting any
// server. Smoke test only; full subcommand behavior is exercised
// in internal/cli/identity/identity_test.go.
func TestRootHelp(t *testing.T) {
	t.Parallel()
	cmd := identity.NewCommand(identity.Deps{})
	cmd.SetArgs([]string{"--help"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetContext(context.Background())
	if err := cmd.Execute(); err != nil {
		t.Fatalf("--help returned err: %v", err)
	}
	got := buf.String()
	for _, want := range []string{"token", "ca", "status", "--server", "--api-key"} {
		if !strings.Contains(got, want) {
			t.Errorf("--help output missing %q\n%s", want, got)
		}
	}
}
