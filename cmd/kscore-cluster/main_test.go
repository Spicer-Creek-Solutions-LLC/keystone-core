// SPDX-License-Identifier: Apache-2.0

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
	cmd := cluster.NewClusterCommand(cluster.Deps{})
	cmd.SetArgs([]string{"--help"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetContext(context.Background())
	if err := cmd.Execute(); err != nil {
		t.Fatalf("--help err: %v", err)
	}
	got := buf.String()
	for _, want := range []string{"status", "members", "leader", "add", "remove", "transfer-leader", "rebalance", "backup", "restore", "--server", "--api-key", "--output"} {
		if !strings.Contains(got, want) {
			t.Errorf("--help missing %q\n%s", want, got)
		}
	}
}
