// SPDX-License-Identifier: Apache-2.0

package state

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// preCommandAgent inserts a parent agent so command FK doesn't violate.
func preCommandAgent(t *testing.T, s *SQLiteStore, id string) {
	t.Helper()
	if err := s.CreateAgent(t.Context(), sampleAgent(id)); err != nil {
		t.Fatalf("seed agent: %v", err)
	}
}

func sampleCommand(id, agentID string) *CommandRecord {
	return &CommandRecord{
		ID:             id,
		AgentID:        agentID,
		Command:        "uptime",
		Args:           []string{"-p"},
		Env:            map[string]string{"FOO": "bar"},
		WorkingDir:     "/tmp",
		User:           "root",
		Principal:      "alice@example.com",
		TimeoutSeconds: 30,
		Status:         CommandStatusPending,
		StartedAt:      time.Date(2026, 5, 6, 14, 0, 0, 0, time.UTC),
	}
}

func TestSQLiteStore_CommandCRUD(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := t.Context()
	preCommandAgent(t, s, "agent-cmd")

	c := sampleCommand("cmd-1", "agent-cmd")
	if err := s.CreateCommand(ctx, c); err != nil {
		t.Fatalf("CreateCommand: %v", err)
	}

	got, err := s.GetCommand(ctx, "cmd-1")
	if err != nil {
		t.Fatalf("GetCommand: %v", err)
	}
	if got.ID != c.ID || got.AgentID != c.AgentID || got.Command != c.Command {
		t.Errorf("scalar fields lost: %+v", got)
	}
	if len(got.Args) != 1 || got.Args[0] != "-p" {
		t.Errorf("Args round-trip: %v", got.Args)
	}
	if got.Env["FOO"] != "bar" {
		t.Errorf("Env round-trip: %v", got.Env)
	}
	if got.Status != CommandStatusPending {
		t.Errorf("Status = %q, want %q", got.Status, CommandStatusPending)
	}
	if got.Principal != "alice@example.com" {
		t.Errorf("Principal round-trip: got %q, want %q", got.Principal, "alice@example.com")
	}
	if !got.StartedAt.Equal(c.StartedAt) {
		t.Errorf("StartedAt: got %v, want %v", got.StartedAt, c.StartedAt)
	}
	// pending command: exit_code stays NULL -> Go zero
	if got.ExitCode != 0 {
		t.Errorf("ExitCode for pending: got %d, want 0", got.ExitCode)
	}

	// UpdateCommandResult
	result := CommandResult{
		Status:      CommandStatusCompleted,
		ExitCode:    0,
		Stdout:      "up 5 days",
		Stderr:      "",
		CompletedAt: time.Date(2026, 5, 6, 14, 0, 5, 0, time.UTC),
	}
	if err := s.UpdateCommandResult(ctx, "cmd-1", result); err != nil {
		t.Fatalf("UpdateCommandResult: %v", err)
	}
	got, err = s.GetCommand(ctx, "cmd-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != CommandStatusCompleted {
		t.Errorf("Status after result: %q, want %q", got.Status, CommandStatusCompleted)
	}
	if got.Stdout != "up 5 days" {
		t.Errorf("Stdout: %q", got.Stdout)
	}
	if !got.CompletedAt.Equal(result.CompletedAt) {
		t.Errorf("CompletedAt: got %v, want %v", got.CompletedAt, result.CompletedAt)
	}
}

func TestSQLiteStore_Command_NotFound(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := t.Context()

	if _, err := s.GetCommand(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetCommand missing: %v, want ErrNotFound", err)
	}
	if err := s.UpdateCommandResult(ctx, "missing", CommandResult{}); !errors.Is(err, ErrNotFound) {
		t.Errorf("UpdateCommandResult missing: %v, want ErrNotFound", err)
	}
}

func TestSQLiteStore_Command_NilRecord(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	if err := s.CreateCommand(t.Context(), nil); err == nil {
		t.Error("CreateCommand(nil): expected error")
	}
}

func TestSQLiteStore_Command_ForeignKeyEnforced(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	c := sampleCommand("cmd-orphan", "no-such-agent")
	err := s.CreateCommand(t.Context(), c)
	if err == nil {
		t.Fatal("expected FK violation; insert succeeded")
	}
	if !strings.Contains(err.Error(), "FOREIGN KEY") &&
		!strings.Contains(err.Error(), "constraint") {
		t.Errorf("expected FK error; got: %v", err)
	}
}

func TestSQLiteStore_ListCommands_Filters(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := t.Context()
	preCommandAgent(t, s, "a")
	preCommandAgent(t, s, "b")

	t0 := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	c1 := sampleCommand("c-1", "a")
	c1.StartedAt = t0
	c2 := sampleCommand("c-2", "a")
	c2.Status = CommandStatusRunning
	c2.StartedAt = t0.Add(time.Hour)
	c3 := sampleCommand("c-3", "b")
	c3.StartedAt = t0.Add(2 * time.Hour)

	for _, c := range []*CommandRecord{c1, c2, c3} {
		if err := s.CreateCommand(ctx, c); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("by agent_id", func(t *testing.T) {
		got, err := s.ListCommands(ctx, CommandFilter{AgentID: "a"})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 {
			t.Errorf("AgentID=a: %d rows, want 2", len(got))
		}
	})

	t.Run("by status", func(t *testing.T) {
		got, err := s.ListCommands(ctx, CommandFilter{Status: CommandStatusRunning})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].ID != "c-2" {
			t.Errorf("Status=running: %d rows", len(got))
		}
	})

	t.Run("by since/until", func(t *testing.T) {
		got, err := s.ListCommands(ctx, CommandFilter{
			Since: t0.Add(30 * time.Minute),
			Until: t0.Add(90 * time.Minute),
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].ID != "c-2" {
			t.Errorf("time-range filter: %d rows", len(got))
		}
	})

	t.Run("invalid sort column rejected", func(t *testing.T) {
		_, err := s.ListCommands(ctx, CommandFilter{SortColumn: "evil"})
		if err == nil {
			t.Error("expected error for non-allowlisted sort column")
		}
	})
}

func TestSQLiteStore_DeleteCommandsBefore(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := t.Context()
	preCommandAgent(t, s, "agent-d")

	old := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	recent := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	cutoff := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)

	mkTerminal := func(id string, completedAt time.Time, status CommandStatus) *CommandRecord {
		c := sampleCommand(id, "agent-d")
		c.Status = status
		c.StartedAt = completedAt.Add(-time.Minute)
		c.CompletedAt = completedAt
		c.ExitCode = 0
		return c
	}
	mkPending := func(id string) *CommandRecord {
		c := sampleCommand(id, "agent-d")
		c.Status = CommandStatusPending
		c.StartedAt = old.Add(-time.Hour) // very old, but no CompletedAt
		c.CompletedAt = time.Time{}
		return c
	}

	for _, c := range []*CommandRecord{
		mkTerminal("old-completed", old, CommandStatusCompleted),
		mkTerminal("old-failed", old, CommandStatusFailed),
		mkTerminal("old-cancelled", old, CommandStatusCancelled),
		mkTerminal("recent-completed", recent, CommandStatusCompleted),
		mkPending("pending-very-old"),
	} {
		if err := s.CreateCommand(ctx, c); err != nil {
			t.Fatalf("seed %s: %v", c.ID, err)
		}
	}

	t.Run("rejects empty status list", func(t *testing.T) {
		if _, err := s.DeleteCommandsBefore(ctx, cutoff, nil); err == nil {
			t.Error("nil statuses should error")
		}
	})

	t.Run("rejects zero cutoff", func(t *testing.T) {
		if _, err := s.DeleteCommandsBefore(ctx, time.Time{}, []CommandStatus{CommandStatusCompleted}); err == nil {
			t.Error("zero cutoff should error")
		}
	})

	t.Run("deletes only matching terminal rows", func(t *testing.T) {
		n, err := s.DeleteCommandsBefore(ctx, cutoff, []CommandStatus{
			CommandStatusCompleted, CommandStatusFailed, CommandStatusTimeout, CommandStatusCancelled,
		})
		if err != nil {
			t.Fatalf("DeleteCommandsBefore: %v", err)
		}
		if n != 3 {
			t.Errorf("deleted = %d, want 3", n)
		}

		// Recent and pending must remain.
		for _, id := range []string{"recent-completed", "pending-very-old"} {
			if _, err := s.GetCommand(ctx, id); err != nil {
				t.Errorf("expected %s preserved: %v", id, err)
			}
		}
		// Old terminal rows must be gone.
		for _, id := range []string{"old-completed", "old-failed", "old-cancelled"} {
			if _, err := s.GetCommand(ctx, id); !errors.Is(err, ErrNotFound) {
				t.Errorf("expected %s deleted, got err=%v", id, err)
			}
		}
	})

	t.Run("status allowlist narrows matches", func(t *testing.T) {
		// Re-seed.
		_ = s.CreateCommand(ctx, mkTerminal("only-completed", old, CommandStatusCompleted))
		_ = s.CreateCommand(ctx, mkTerminal("only-failed", old, CommandStatusFailed))

		n, err := s.DeleteCommandsBefore(ctx, cutoff, []CommandStatus{CommandStatusFailed})
		if err != nil {
			t.Fatalf("DeleteCommandsBefore: %v", err)
		}
		if n != 1 {
			t.Errorf("deleted = %d, want 1", n)
		}
		if _, err := s.GetCommand(ctx, "only-completed"); err != nil {
			t.Errorf("status-allowlisted-out row was deleted: %v", err)
		}
	})
}
