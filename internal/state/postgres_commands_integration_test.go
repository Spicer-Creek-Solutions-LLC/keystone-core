//go:build integration

package state

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func pgPreCommandAgent(t *testing.T, s *PostgreSQLStore, id string) {
	t.Helper()
	if err := s.CreateAgent(t.Context(), sampleAgent(id)); err != nil {
		t.Fatalf("seed agent: %v", err)
	}
}

func TestPg_CommandCRUD(t *testing.T) {
	s := newPgStoreForTest(t)
	ctx := t.Context()
	pgPreCommandAgent(t, s, "agent-cmd")

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
	if !got.StartedAt.Equal(c.StartedAt) {
		t.Errorf("StartedAt: got %v, want %v", got.StartedAt, c.StartedAt)
	}
	if got.ExitCode != 0 {
		t.Errorf("ExitCode for pending: %d", got.ExitCode)
	}

	result := CommandResult{
		Status:      CommandStatusCompleted,
		ExitCode:    0,
		Stdout:      "up 5 days",
		CompletedAt: time.Date(2026, 5, 6, 14, 0, 5, 0, time.UTC),
	}
	if err := s.UpdateCommandResult(ctx, "cmd-1", result); err != nil {
		t.Fatal(err)
	}
	got, err = s.GetCommand(ctx, "cmd-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != CommandStatusCompleted || got.Stdout != "up 5 days" {
		t.Errorf("after result update: %+v", got)
	}
}

func TestPg_Command_NotFound(t *testing.T) {
	s := newPgStoreForTest(t)
	if _, err := s.GetCommand(t.Context(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetCommand: %v", err)
	}
	if err := s.UpdateCommandResult(t.Context(), "missing", CommandResult{}); !errors.Is(err, ErrNotFound) {
		t.Errorf("UpdateCommandResult: %v", err)
	}
}

func TestPg_Command_NilRecord(t *testing.T) {
	s := newPgStoreForTest(t)
	if err := s.CreateCommand(t.Context(), nil); err == nil {
		t.Error("CreateCommand(nil): expected error")
	}
}

func TestPg_Command_ForeignKeyEnforced(t *testing.T) {
	s := newPgStoreForTest(t)
	c := sampleCommand("cmd-orphan", "no-such-agent")
	err := s.CreateCommand(t.Context(), c)
	if err == nil {
		t.Fatal("expected FK violation")
	}
	if !strings.Contains(err.Error(), "foreign key") &&
		!strings.Contains(err.Error(), "violates") {
		t.Errorf("expected FK error; got: %v", err)
	}
}

func TestPg_ListCommands_Filters(t *testing.T) {
	s := newPgStoreForTest(t)
	ctx := t.Context()
	pgPreCommandAgent(t, s, "a")
	pgPreCommandAgent(t, s, "b")

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
			t.Errorf("AgentID=a: %d", len(got))
		}
	})
	t.Run("by status", func(t *testing.T) {
		got, err := s.ListCommands(ctx, CommandFilter{Status: CommandStatusRunning})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].ID != "c-2" {
			t.Errorf("got %d rows", len(got))
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
			t.Errorf("got %d rows", len(got))
		}
	})
	t.Run("invalid sort rejected", func(t *testing.T) {
		_, err := s.ListCommands(ctx, CommandFilter{SortColumn: "evil"})
		if err == nil {
			t.Error("expected error")
		}
	})
}
