package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"go.keystone-core.io/keystone-core/internal/cli/events"
)

// TestRootHelp confirms the kscore-events binary's command tree
// is wired correctly: `--help` lists every top-level subcommand
// without contacting any server. Smoke test only; full subcommand
// behavior is exercised in internal/cli/events/events_test.go.
func TestRootHelp(t *testing.T) {
	t.Parallel()
	cmd := events.NewCommand(events.Deps{})
	cmd.SetArgs([]string{"--help"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetContext(context.Background())
	if err := cmd.Execute(); err != nil {
		t.Fatalf("--help returned err: %v", err)
	}
	got := buf.String()
	for _, want := range []string{
		"list", "get", "emit",
		"subscribe", "watch", "replay",
		"types", "stats",
		"--server", "--api-key", "--output",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("--help output missing %q\n%s", want, got)
		}
	}
}
