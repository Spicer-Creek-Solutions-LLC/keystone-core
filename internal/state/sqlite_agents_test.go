package state

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func sampleAgent(id string) *AgentRecord {
	return &AgentRecord{
		ID:              id,
		Hostname:        "host-" + id,
		OS:              "linux",
		Architecture:    "amd64",
		IPAddresses:     []string{"10.0.0.1", "fe80::1"},
		PlatformVersion: "Ubuntu 24.04",
		AgentVersion:    "0.0.1",
		Labels:          map[string]string{"role": "web", "env": "prod"},
		Status:          AgentStatusConnected,
		RegisteredAt:    time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC),
		LastHeartbeatAt: time.Date(2026, 5, 6, 12, 5, 0, 0, time.UTC),
		Metrics:         map[string]any{"load": 0.42, "uptime": float64(3600)},
	}
}

func TestSQLiteStore_AgentCRUD(t *testing.T) {
	s := newSQLiteStoreForTest(t)
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
	if got.Labels["role"] != "web" || got.Labels["env"] != "prod" {
		t.Errorf("Labels round-trip: %v", got.Labels)
	}
	if got.Status != AgentStatusConnected {
		t.Errorf("Status = %q, want %q", got.Status, AgentStatusConnected)
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

	// UpdateAgent
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
		t.Errorf("UpdateAgent didn't apply: %+v", got)
	}

	// UpdateAgentHeartbeat
	hb := time.Date(2026, 5, 6, 13, 0, 0, 0, time.UTC)
	if err := s.UpdateAgentHeartbeat(ctx, "a-1", hb); err != nil {
		t.Fatalf("UpdateAgentHeartbeat: %v", err)
	}
	got, err = s.GetAgent(ctx, "a-1")
	if err != nil {
		t.Fatal(err)
	}
	if !got.LastHeartbeatAt.Equal(hb) {
		t.Errorf("UpdateAgentHeartbeat: got %v, want %v", got.LastHeartbeatAt, hb)
	}

	// UpdateAgentStatus
	if err := s.UpdateAgentStatus(ctx, "a-1", AgentStatusDisabled); err != nil {
		t.Fatalf("UpdateAgentStatus: %v", err)
	}
	got, err = s.GetAgent(ctx, "a-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != AgentStatusDisabled {
		t.Errorf("Status after UpdateAgentStatus = %q, want %q", got.Status, AgentStatusDisabled)
	}

	// DeleteAgent
	if err := s.DeleteAgent(ctx, "a-1"); err != nil {
		t.Fatalf("DeleteAgent: %v", err)
	}
	if _, err := s.GetAgent(ctx, "a-1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("post-delete Get err = %v, want ErrNotFound", err)
	}
}

func TestSQLiteStore_Agent_NotFound(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := t.Context()

	if _, err := s.GetAgent(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetAgent missing: %v, want ErrNotFound", err)
	}
	if err := s.UpdateAgent(ctx, sampleAgent("missing")); !errors.Is(err, ErrNotFound) {
		t.Errorf("UpdateAgent missing: %v, want ErrNotFound", err)
	}
	if err := s.UpdateAgentHeartbeat(ctx, "missing", time.Now()); !errors.Is(err, ErrNotFound) {
		t.Errorf("UpdateAgentHeartbeat missing: %v, want ErrNotFound", err)
	}
	if err := s.UpdateAgentStatus(ctx, "missing", AgentStatusConnected); !errors.Is(err, ErrNotFound) {
		t.Errorf("UpdateAgentStatus missing: %v, want ErrNotFound", err)
	}
	if err := s.DeleteAgent(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("DeleteAgent missing: %v, want ErrNotFound", err)
	}
}

func TestSQLiteStore_Agent_NilRecord(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	if err := s.CreateAgent(t.Context(), nil); err == nil {
		t.Error("CreateAgent(nil): expected error")
	}
	if err := s.UpdateAgent(t.Context(), nil); err == nil {
		t.Error("UpdateAgent(nil): expected error")
	}
}

func TestSQLiteStore_ListAgents_EmptyAndUnfiltered(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := t.Context()

	got, err := s.ListAgents(ctx, AgentFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("ListAgents on empty: %d rows, want 0", len(got))
	}

	for _, id := range []string{"a-1", "a-2", "a-3"} {
		if err := s.CreateAgent(ctx, sampleAgent(id)); err != nil {
			t.Fatal(err)
		}
	}

	got, err = s.ListAgents(ctx, AgentFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Errorf("ListAgents: %d rows, want 3", len(got))
	}
}

func TestSQLiteStore_ListAgents_FilterAndPagination(t *testing.T) {
	s := newSQLiteStoreForTest(t)
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
			t.Errorf("filter by Status: got %d rows", len(got))
		}
	})

	t.Run("by label key+value", func(t *testing.T) {
		got, err := s.ListAgents(ctx, AgentFilter{LabelKey: "role", LabelValue: "web"})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 {
			t.Errorf("filter by Label: got %d rows, want 2", len(got))
		}
	})

	t.Run("by label key only", func(t *testing.T) {
		got, err := s.ListAgents(ctx, AgentFilter{LabelKey: "role"})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 3 {
			t.Errorf("filter by LabelKey: got %d rows, want 3", len(got))
		}
	})

	t.Run("limit", func(t *testing.T) {
		got, err := s.ListAgents(ctx, AgentFilter{Limit: 2})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 {
			t.Errorf("Limit=2: got %d rows", len(got))
		}
	})

	t.Run("offset", func(t *testing.T) {
		got, err := s.ListAgents(ctx, AgentFilter{Limit: 2, Offset: 2})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 {
			t.Errorf("Limit=2, Offset=2: got %d rows", len(got))
		}
	})

	t.Run("explicit sort by hostname asc", func(t *testing.T) {
		got, err := s.ListAgents(ctx, AgentFilter{SortColumn: "hostname", SortDesc: false})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 3 {
			t.Fatal("expected 3 rows")
		}
		if got[0].Hostname > got[1].Hostname || got[1].Hostname > got[2].Hostname {
			t.Errorf("ASC sort failed: %s, %s, %s",
				got[0].Hostname, got[1].Hostname, got[2].Hostname)
		}
	})

	t.Run("invalid sort column rejected", func(t *testing.T) {
		_, err := s.ListAgents(ctx, AgentFilter{SortColumn: "password; DROP TABLE agents--"})
		if err == nil {
			t.Error("expected error for non-allowlisted sort column")
		}
		if !strings.Contains(err.Error(), "not allowed") {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestSQLiteStore_Agent_NullableFields(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := t.Context()

	a := sampleAgent("a-null")
	a.LastHeartbeatAt = time.Time{} // zero -> NULL
	a.PlatformVersion = ""           // empty -> NULL
	a.AgentVersion = ""               // empty -> NULL
	a.Metrics = nil                   // nil map -> NULL

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
		t.Errorf("PlatformVersion should be empty; got %q", got.PlatformVersion)
	}
	if got.Metrics != nil {
		t.Errorf("Metrics should be nil; got %v", got.Metrics)
	}
}

// PROJECT-DETAILS §4.3 explicitly requires unmarshal errors to surface;
// inject malformed JSON directly and verify GetAgent returns an error.
func TestSQLiteStore_Agent_MalformedJSONSurfaces(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := t.Context()

	if err := s.CreateAgent(ctx, sampleAgent("bad")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE agents SET labels = ? WHERE id = ?`,
		`not-valid-json`, "bad",
	); err != nil {
		t.Fatal(err)
	}

	_, err := s.GetAgent(ctx, "bad")
	if err == nil {
		t.Fatal("expected unmarshal error")
	}
	if !strings.Contains(err.Error(), "unmarshal") &&
		!strings.Contains(err.Error(), "json") {
		t.Errorf("expected unmarshal-related error; got: %v", err)
	}
}
