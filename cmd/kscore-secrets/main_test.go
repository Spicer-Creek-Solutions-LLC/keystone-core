package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"go.keystone-core.io/keystone-core/internal/cli/secrets"
)

// TestRootHelp confirms the kscore-secrets binary's command tree
// is wired correctly: `--help` lists the top-level subcommands
// (get / put / delete / list / leases / transit) without
// contacting any server. Smoke test only; full subcommand behavior
// is exercised in internal/cli/secrets/secrets_test.go.
func TestRootHelp(t *testing.T) {
	t.Parallel()
	cmd := secrets.NewCommand(secrets.Deps{})
	cmd.SetArgs([]string{"--help"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetContext(context.Background())
	if err := cmd.Execute(); err != nil {
		t.Fatalf("--help returned err: %v", err)
	}
	got := buf.String()
	for _, want := range []string{"get", "put", "delete", "list", "leases", "transit", "--server", "--api-key"} {
		if !strings.Contains(got, want) {
			t.Errorf("--help output missing %q\n%s", want, got)
		}
	}
}
