// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"testing"
)

func TestNewCommand_Metadata(t *testing.T) {
	t.Parallel()
	cmd := NewCommand(Deps{})
	if got, want := cmd.Use, "agent"; got != want {
		t.Errorf("Use = %q, want %q", got, want)
	}
	if cmd.Short == "" {
		t.Error("Short is empty")
	}
	for _, name := range []string{"server", "api-key", "output"} {
		if cmd.PersistentFlags().Lookup(name) == nil {
			t.Errorf("missing persistent flag %q", name)
		}
	}
	if cmd.Commands() == nil || len(cmd.Commands()) == 0 {
		t.Fatal("no subcommands registered")
	}
	found := false
	for _, sub := range cmd.Commands() {
		if sub.Name() == "list" {
			found = true
		}
	}
	if !found {
		t.Error("`list` subcommand not registered")
	}
}

func TestNewCommand_DefaultDialer(t *testing.T) {
	t.Parallel()
	cmd := NewCommand(Deps{}) // empty Deps must auto-wire dialGRPC
	cmd.SetArgs([]string{"list", "--server", ""})
	// Empty --server makes dialGRPC error early before any network IO,
	// which proves the default dialer was wired in.
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error from empty --server, got nil")
	}
}
