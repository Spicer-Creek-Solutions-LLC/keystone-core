package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"go.keystone-core.io/keystone-core/internal/cli/policy"
)

func TestRootHelp(t *testing.T) {
	t.Parallel()
	cmd := policy.NewCommand(policy.Deps{})
	cmd.SetArgs([]string{"--help"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetContext(context.Background())
	if err := cmd.Execute(); err != nil {
		t.Fatalf("--help err: %v", err)
	}
	got := buf.String()
	for _, want := range []string{"list", "show", "compliance", "violations", "eval", "validate", "--server", "--api-key", "--output"} {
		if !strings.Contains(got, want) {
			t.Errorf("--help missing %q\n%s", want, got)
		}
	}
}
