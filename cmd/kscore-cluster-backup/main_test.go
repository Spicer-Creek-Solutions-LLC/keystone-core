package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"go.keystone-core.io/keystone-core/internal/cli/cluster"
)

func TestRootHelp(t *testing.T) {
	t.Parallel()
	cmd := cluster.NewBackupCommand(cluster.Deps{})
	cmd.SetArgs([]string{"--help"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetContext(context.Background())
	if err := cmd.Execute(); err != nil {
		t.Fatalf("--help err: %v", err)
	}
	got := buf.String()
	for _, want := range []string{"backup", "restore", "list", "verify", "--server", "--output"} {
		if !strings.Contains(got, want) {
			t.Errorf("--help missing %q\n%s", want, got)
		}
	}
}
