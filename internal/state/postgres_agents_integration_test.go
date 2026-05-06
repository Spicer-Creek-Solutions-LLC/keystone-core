//go:build integration

package state

import (
	"errors"
	"fmt"
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

// PROJECT-DETAILS §4.3: JSON unmarshal errors must surface from every
// JSON column on every backend. Postgres-side regression mirror of
// TestSQLiteStore_MalformedJSONSurfacesAllColumns.
//
// Note: Postgres rejects malformed JSONB at INSERT/UPDATE time, so we
// can't write `not-valid-json` and re-read. Instead we write
// well-formed JSON of the WRONG SHAPE — e.g., an array where Go
// expects a map — which the Go side rejects on Unmarshal.
func TestPg_MalformedJSONSurfacesAllColumns(t *testing.T) {
	s := newPgStoreForTest(t)
	ctx := t.Context()

	const wrongShape = `["wrong","shape","for","scan"]`

	cases := []struct {
		name   string
		seed   func(id string)
		table  string
		column string
		get    func(id string) error
	}{
		{
			name:   "agents.ip_addresses",
			seed:   func(id string) { mustCreatePgAgent(t, s, id) },
			table:  "agents",
			column: "ip_addresses",
			get:    func(id string) error { _, err := s.GetAgent(ctx, id); return err },
		},
		{
			name: "agents.labels",
			seed: func(id string) { mustCreatePgAgent(t, s, id) },
			// Wrong shape: Labels is map[string]string; an array won't
			// unmarshal into it.
			table:  "agents",
			column: "labels",
			get:    func(id string) error { _, err := s.GetAgent(ctx, id); return err },
		},
		{
			name:   "agents.metrics",
			seed:   func(id string) { mustCreatePgAgent(t, s, id) },
			table:  "agents",
			column: "metrics",
			get:    func(id string) error { _, err := s.GetAgent(ctx, id); return err },
		},
		{
			name: "commands.args",
			// commands.args is []string; an object would mismatch. The
			// `wrongShape` array IS valid for []string{} (just strings),
			// so flip to an object for this case.
			seed: func(id string) {
				mustCreatePgAgent(t, s, "agent-"+id)
				if err := s.CreateCommand(ctx, sampleCommand(id, "agent-"+id)); err != nil {
					t.Fatal(err)
				}
			},
			table:  "commands",
			column: "args",
			get:    func(id string) error { _, err := s.GetCommand(ctx, id); return err },
		},
		{
			name: "commands.env",
			seed: func(id string) {
				mustCreatePgAgent(t, s, "agent-"+id)
				if err := s.CreateCommand(ctx, sampleCommand(id, "agent-"+id)); err != nil {
					t.Fatal(err)
				}
			},
			table:  "commands",
			column: "env",
			get:    func(id string) error { _, err := s.GetCommand(ctx, id); return err },
		},
		{
			name: "batch_jobs.target",
			seed: func(id string) {
				if err := s.CreateBatchJob(ctx, &BatchJobRecord{
					ID: id, Target: map[string]any{"r": "w"}, Command: "u",
					Args: []string{}, Status: BatchJobStatusPending, CreatedAt: time.Now().UTC(),
				}); err != nil {
					t.Fatal(err)
				}
			},
			table:  "batch_jobs",
			column: "target",
			get:    func(id string) error { _, err := s.GetBatchJob(ctx, id); return err },
		},
		{
			name: "batch_jobs.args",
			seed: func(id string) {
				if err := s.CreateBatchJob(ctx, &BatchJobRecord{
					ID: id, Target: map[string]any{"r": "w"}, Command: "u",
					Args: []string{}, Status: BatchJobStatusPending, CreatedAt: time.Now().UTC(),
				}); err != nil {
					t.Fatal(err)
				}
			},
			table:  "batch_jobs",
			column: "args",
			get:    func(id string) error { _, err := s.GetBatchJob(ctx, id); return err },
		},
	}

	// agents.ip_addresses, commands.args, batch_jobs.args expect array
	// types — `wrongShape` is itself a string array, which would
	// successfully unmarshal. Override these cases to write an object
	// (wrong shape for an array).
	objectShape := `{"a":"b"}`
	objectCases := map[string]bool{
		"agents.ip_addresses": true,
		"commands.args":       true,
		"batch_jobs.args":     true,
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id := fmt.Sprintf("bad-%d", i)
			tc.seed(id)
			shape := wrongShape
			if objectCases[tc.name] {
				shape = objectShape
			}
			q := fmt.Sprintf(`UPDATE %s SET %s = $1::jsonb WHERE id = $2`, tc.table, tc.column)
			if _, err := s.db.ExecContext(ctx, q, shape, id); err != nil {
				t.Fatalf("corrupt: %v", err)
			}
			assertJSONUnmarshalError(t, tc.get(id), tc.name)
		})
	}
}

func mustCreatePgAgent(t *testing.T, s *PostgreSQLStore, id string) {
	t.Helper()
	if err := s.CreateAgent(t.Context(), sampleAgent(id)); err != nil {
		t.Fatalf("seed agent %q: %v", id, err)
	}
}

// json.Marshal failures (e.g., chan/func values) must propagate from
// Create/Update.
func TestPg_Agent_MarshalErrorSurfaces(t *testing.T) {
	s := newPgStoreForTest(t)
	a := sampleAgent("a-marshal")
	a.Metrics = map[string]any{"unmarshalable": make(chan int)}

	err := s.CreateAgent(t.Context(), a)
	if err == nil {
		t.Fatal("expected marshal error; CreateAgent succeeded")
	}
	if !strings.Contains(err.Error(), "marshal") &&
		!strings.Contains(err.Error(), "json") {
		t.Errorf("expected marshal-related error; got: %v", err)
	}
}
