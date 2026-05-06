//go:build integration

package state

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestPg_AgentCRUD(t *testing.T) {
	s := newPgStoreForTest(t)
	ctx := t.Context()

	a := sampleAgent("a-1")
	if err := s.CreateAgent(ctx, a); err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	got, err := s.GetAgent(ctx, "a-1")
	if err != nil {
		t.Fatalf("GetAgent: %v", err)
	}
	if got.ID != a.ID || got.Hostname != a.Hostname || got.OS != a.OS {
		t.Errorf("scalar fields lost: %+v", got)
	}
	if len(got.IPAddresses) != 2 || got.IPAddresses[0] != "10.0.0.1" {
		t.Errorf("IPAddresses round-trip: %v", got.IPAddresses)
	}
	if got.Labels["role"] != "web" {
		t.Errorf("Labels round-trip: %v", got.Labels)
	}
	if got.Status != AgentStatusConnected {
		t.Errorf("Status = %q", got.Status)
	}
	if !got.RegisteredAt.Equal(a.RegisteredAt) {
		t.Errorf("RegisteredAt: got %v, want %v", got.RegisteredAt, a.RegisteredAt)
	}
	if !got.LastHeartbeatAt.Equal(a.LastHeartbeatAt) {
		t.Errorf("LastHeartbeatAt: got %v, want %v", got.LastHeartbeatAt, a.LastHeartbeatAt)
	}
	if got.Metrics["load"] != 0.42 {
		t.Errorf("Metrics round-trip: %v", got.Metrics)
	}

	a.Hostname = "renamed"
	a.Status = AgentStatusStale
	if err := s.UpdateAgent(ctx, a); err != nil {
		t.Fatalf("UpdateAgent: %v", err)
	}
	got, err = s.GetAgent(ctx, "a-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Hostname != "renamed" || got.Status != AgentStatusStale {
		t.Errorf("UpdateAgent not applied: %+v", got)
	}

	hb := time.Date(2026, 5, 6, 13, 0, 0, 0, time.UTC)
	if err := s.UpdateAgentHeartbeat(ctx, "a-1", hb); err != nil {
		t.Fatal(err)
	}
	got, err = s.GetAgent(ctx, "a-1")
	if err != nil {
		t.Fatal(err)
	}
	if !got.LastHeartbeatAt.Equal(hb) {
		t.Errorf("Heartbeat: got %v", got.LastHeartbeatAt)
	}

	if err := s.UpdateAgentStatus(ctx, "a-1", AgentStatusDisabled); err != nil {
		t.Fatal(err)
	}

	if err := s.DeleteAgent(ctx, "a-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetAgent(ctx, "a-1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("post-delete: got %v, want ErrNotFound", err)
	}
}

func TestPg_Agent_NotFound(t *testing.T) {
	s := newPgStoreForTest(t)
	ctx := t.Context()

	if _, err := s.GetAgent(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetAgent: %v", err)
	}
	if err := s.UpdateAgent(ctx, sampleAgent("missing")); !errors.Is(err, ErrNotFound) {
		t.Errorf("UpdateAgent: %v", err)
	}
	if err := s.UpdateAgentHeartbeat(ctx, "missing", time.Now()); !errors.Is(err, ErrNotFound) {
		t.Errorf("UpdateAgentHeartbeat: %v", err)
	}
	if err := s.UpdateAgentStatus(ctx, "missing", AgentStatusConnected); !errors.Is(err, ErrNotFound) {
		t.Errorf("UpdateAgentStatus: %v", err)
	}
	if err := s.DeleteAgent(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("DeleteAgent: %v", err)
	}
}

func TestPg_Agent_NilRecord(t *testing.T) {
	s := newPgStoreForTest(t)
	if err := s.CreateAgent(t.Context(), nil); err == nil {
		t.Error("CreateAgent(nil): expected error")
	}
	if err := s.UpdateAgent(t.Context(), nil); err == nil {
		t.Error("UpdateAgent(nil): expected error")
	}
}

func TestPg_ListAgents_FilterAndPagination(t *testing.T) {
	s := newPgStoreForTest(t)
	ctx := t.Context()

	a1 := sampleAgent("a-1")
	a1.Labels = map[string]string{"role": "web"}
	a2 := sampleAgent("a-2")
	a2.Labels = map[string]string{"role": "db"}
	a2.Status = AgentStatusStale
	a3 := sampleAgent("a-3")
	a3.Labels = map[string]string{"role": "web"}

	for _, a := range []*AgentRecord{a1, a2, a3} {
		if err := s.CreateAgent(ctx, a); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("by status", func(t *testing.T) {
		got, err := s.ListAgents(ctx, AgentFilter{Status: AgentStatusStale})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].ID != "a-2" {
			t.Errorf("got %d rows", len(got))
		}
	})

	t.Run("by label key+value (JSONB containment)", func(t *testing.T) {
		got, err := s.ListAgents(ctx, AgentFilter{LabelKey: "role", LabelValue: "web"})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 {
			t.Errorf("got %d rows, want 2", len(got))
		}
	})

	t.Run("by label key only (JSONB ?-operator)", func(t *testing.T) {
		got, err := s.ListAgents(ctx, AgentFilter{LabelKey: "role"})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 3 {
			t.Errorf("got %d rows, want 3", len(got))
		}
	})

	t.Run("limit + offset", func(t *testing.T) {
		got, err := s.ListAgents(ctx, AgentFilter{Limit: 2, Offset: 2})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 {
			t.Errorf("got %d rows", len(got))
		}
	})

	t.Run("invalid sort rejected", func(t *testing.T) {
		_, err := s.ListAgents(ctx, AgentFilter{SortColumn: "evil; DROP TABLE agents--"})
		if err == nil {
			t.Error("expected error")
		}
	})
}

func TestPg_Agent_NullableFields(t *testing.T) {
	s := newPgStoreForTest(t)
	ctx := t.Context()

	a := sampleAgent("a-null")
	a.LastHeartbeatAt = time.Time{}
	a.PlatformVersion = ""
	a.AgentVersion = ""
	a.Metrics = nil

	if err := s.CreateAgent(ctx, a); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetAgent(ctx, "a-null")
	if err != nil {
		t.Fatal(err)
	}
	if !got.LastHeartbeatAt.IsZero() {
		t.Errorf("LastHeartbeatAt should be zero; got %v", got.LastHeartbeatAt)
	}
	if got.PlatformVersion != "" {
		t.Errorf("PlatformVersion: %q", got.PlatformVersion)
	}
	if got.Metrics != nil {
		t.Errorf("Metrics should be nil; got %v", got.Metrics)
	}
}

// PROJECT-DETAILS §4.3: malformed JSON on read must surface, not silently
// produce an empty value.
func TestPg_Agent_MalformedJSONSurfaces(t *testing.T) {
	s := newPgStoreForTest(t)
	ctx := t.Context()

	if err := s.CreateAgent(ctx, sampleAgent("bad")); err != nil {
		t.Fatal(err)
	}
	// Bypass JSONB validation by writing as text and casting; Postgres
	// accepts e.g. '{"valid":true,"trailing":` without closing brace
	// only if we trick it. Easier: drop a known-good row, then update
	// directly with a sub-object that's well-formed JSON but the wrong
	// shape for Labels.
	if _, err := s.db.ExecContext(ctx,
		`UPDATE agents SET labels = $1::jsonb WHERE id = $2`,
		`["this","is","not","an","object"]`, "bad",
	); err != nil {
		t.Fatal(err)
	}

	_, err := s.GetAgent(ctx, "bad")
	if err == nil {
		t.Fatal("expected unmarshal error from wrong-shape JSON")
	}
	if !strings.Contains(err.Error(), "unmarshal") &&
		!strings.Contains(err.Error(), "json") {
		t.Errorf("unexpected error: %v", err)
	}
}
