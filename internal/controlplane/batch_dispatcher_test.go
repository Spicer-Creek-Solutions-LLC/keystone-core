// SPDX-License-Identifier: Apache-2.0

package controlplane_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/controlplane"
	"go.keystone-core.io/keystone-core/internal/state"
)

// batchFixture wires a BatchDispatcher against the per-test SQLite store.
// Agents and a parent batch are seeded as needed by individual tests via
// helpers below.
type batchFixture struct {
	store state.Store
	disp  *controlplane.BatchDispatcher
	clk   *fakeClock
}

func newBatchFixture(t *testing.T, opts ...func(*controlplane.BatchDispatcherConfig)) *batchFixture {
	t.Helper()
	store := newTestStore(t)
	clk := newFakeClock(time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC))

	cfg := controlplane.BatchDispatcherConfig{
		Store: store,
		Clock: clk.Now,
		NewID: seqBatchIDGenerator(),
	}
	for _, o := range opts {
		o(&cfg)
	}
	disp, err := controlplane.NewBatchDispatcher(cfg)
	if err != nil {
		t.Fatalf("NewBatchDispatcher: %v", err)
	}
	return &batchFixture{store: store, disp: disp, clk: clk}
}

func seqBatchIDGenerator() func() string {
	var n atomic.Int64
	return func() string { return fmt.Sprintf("batch-%d", n.Add(1)) }
}

// seedAgents creates the parent agent rows so batch_agent_results FKs
// satisfy when RecordAgentResult fires.
func (f *batchFixture) seedAgents(t *testing.T, ids ...string) {
	t.Helper()
	for _, id := range ids {
		if err := f.store.CreateAgent(context.Background(), &state.AgentRecord{
			ID:              id,
			Hostname:        id + ".example.com",
			OS:              "linux",
			Architecture:    "amd64",
			Status:          state.AgentStatusConnected,
			RegisteredAt:    f.clk.Now(),
			LastHeartbeatAt: f.clk.Now(),
		}); err != nil {
			t.Fatalf("seed agent %s: %v", id, err)
		}
	}
}

func TestNewBatchDispatcher_Validation(t *testing.T) {
	store := newTestStore(t)
	if _, err := controlplane.NewBatchDispatcher(controlplane.BatchDispatcherConfig{}); err == nil {
		t.Error("nil store should error")
	}
	if _, err := controlplane.NewBatchDispatcher(controlplane.BatchDispatcherConfig{
		Store: store, DefaultConcurrency: -1,
	}); err == nil {
		t.Error("negative concurrency should error")
	}
	if _, err := controlplane.NewBatchDispatcher(controlplane.BatchDispatcherConfig{Store: store}); err != nil {
		t.Errorf("default cfg: %v", err)
	}
}

func TestCreateBatch_HappyPath(t *testing.T) {
	ctx := context.Background()
	f := newBatchFixture(t)

	id, err := f.disp.CreateBatch(ctx, controlplane.BatchRequest{
		Target:      map[string]any{"role": "web"},
		Command:     "uptime",
		Args:        []string{"-p"},
		TotalAgents: 5,
	})
	if err != nil {
		t.Fatalf("CreateBatch: %v", err)
	}
	if id != "batch-1" {
		t.Errorf("ID = %q, want batch-1", id)
	}

	rec, err := f.store.GetBatchJob(ctx, id)
	if err != nil {
		t.Fatalf("GetBatchJob: %v", err)
	}
	if rec.Status != state.BatchJobStatusPending {
		t.Errorf("Status = %q, want pending", rec.Status)
	}
	if rec.Concurrency != 10 {
		t.Errorf("Concurrency = %d, want default 10", rec.Concurrency)
	}
	if rec.TotalAgents != 5 {
		t.Errorf("TotalAgents = %d, want 5", rec.TotalAgents)
	}
	if rec.Target["role"] != "web" {
		t.Errorf("Target = %v", rec.Target)
	}
}

func TestCreateBatch_RespectsSuppliedID(t *testing.T) {
	ctx := context.Background()
	f := newBatchFixture(t)

	id, err := f.disp.CreateBatch(ctx, controlplane.BatchRequest{
		ID: "custom-id", Command: "uptime", TotalAgents: 1,
	})
	if err != nil {
		t.Fatalf("CreateBatch: %v", err)
	}
	if id != "custom-id" {
		t.Errorf("ID = %q, want custom-id", id)
	}
}

func TestCreateBatch_RejectsInvalidRequests(t *testing.T) {
	ctx := context.Background()
	f := newBatchFixture(t)

	if _, err := f.disp.CreateBatch(ctx, controlplane.BatchRequest{TotalAgents: 1}); !errors.Is(err, controlplane.ErrInvalidBatchRequest) {
		t.Errorf("missing Command: %v", err)
	}
	if _, err := f.disp.CreateBatch(ctx, controlplane.BatchRequest{Command: "uptime"}); !errors.Is(err, controlplane.ErrInvalidBatchRequest) {
		t.Errorf("missing TotalAgents: %v", err)
	}
	if _, err := f.disp.CreateBatch(ctx, controlplane.BatchRequest{Command: "uptime", TotalAgents: -1}); !errors.Is(err, controlplane.ErrInvalidBatchRequest) {
		t.Errorf("negative TotalAgents: %v", err)
	}
}

func TestMarkRunning_TransitionsAndStampsStartedAt(t *testing.T) {
	ctx := context.Background()
	f := newBatchFixture(t)

	id, err := f.disp.CreateBatch(ctx, controlplane.BatchRequest{
		Command: "uptime", TotalAgents: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	f.clk.Advance(time.Second)

	if err := f.disp.MarkRunning(ctx, id); err != nil {
		t.Fatalf("MarkRunning: %v", err)
	}
	rec, err := f.store.GetBatchJob(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Status != state.BatchJobStatusRunning {
		t.Errorf("Status = %q, want running", rec.Status)
	}
	if !rec.StartedAt.Equal(f.clk.Now()) {
		t.Errorf("StartedAt = %v, want %v", rec.StartedAt, f.clk.Now())
	}
}

func TestMarkRunning_RejectsNonPending(t *testing.T) {
	ctx := context.Background()
	f := newBatchFixture(t)

	id, err := f.disp.CreateBatch(ctx, controlplane.BatchRequest{
		Command: "uptime", TotalAgents: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.disp.MarkRunning(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := f.disp.MarkRunning(ctx, id); !errors.Is(err, controlplane.ErrBatchInvalidState) {
		t.Errorf("re-MarkRunning: %v, want ErrBatchInvalidState", err)
	}
	if err := f.disp.MarkRunning(ctx, "ghost"); !errors.Is(err, controlplane.ErrBatchNotFound) {
		t.Errorf("ghost ID: %v, want ErrBatchNotFound", err)
	}
}

func TestRecordAgentResult_UpdatesCounts(t *testing.T) {
	ctx := context.Background()
	f := newBatchFixture(t)
	f.seedAgents(t, "agent-1", "agent-2", "agent-3")

	id, err := f.disp.CreateBatch(ctx, controlplane.BatchRequest{
		Command: "uptime", TotalAgents: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.disp.MarkRunning(ctx, id); err != nil {
		t.Fatal(err)
	}

	results := []state.BatchAgentResultRecord{
		{AgentID: "agent-1", Success: true, ExitCode: 0},
		{AgentID: "agent-2", Success: false, ExitCode: 1, Error: "boom"},
		{AgentID: "agent-3", Success: true, ExitCode: 0},
	}
	for _, r := range results {
		if err := f.disp.RecordAgentResult(ctx, id, r); err != nil {
			t.Fatalf("RecordAgentResult %s: %v", r.AgentID, err)
		}
	}

	rec, err := f.store.GetBatchJob(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if rec.CompletedAgents != 3 || rec.SuccessfulAgents != 2 || rec.FailedAgents != 1 {
		t.Errorf("counts = (c=%d s=%d f=%d), want (3,2,1)",
			rec.CompletedAgents, rec.SuccessfulAgents, rec.FailedAgents)
	}

	// Per-agent rows persisted
	rows, err := f.store.ListBatchAgentResults(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Errorf("agent result rows = %d, want 3", len(rows))
	}
}

func TestRecordAgentResult_RejectsNonRunning(t *testing.T) {
	ctx := context.Background()
	f := newBatchFixture(t)
	f.seedAgents(t, "agent-1")

	id, err := f.disp.CreateBatch(ctx, controlplane.BatchRequest{
		Command: "uptime", TotalAgents: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Pending — should reject.
	if err := f.disp.RecordAgentResult(ctx, id, state.BatchAgentResultRecord{
		AgentID: "agent-1", Success: true,
	}); !errors.Is(err, controlplane.ErrBatchInvalidState) {
		t.Errorf("pending RecordAgentResult: %v, want ErrBatchInvalidState", err)
	}
}

func TestRecordAgentResult_OverReportRejected(t *testing.T) {
	ctx := context.Background()
	f := newBatchFixture(t)
	f.seedAgents(t, "agent-1", "agent-2")

	id, err := f.disp.CreateBatch(ctx, controlplane.BatchRequest{
		Command: "uptime", TotalAgents: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.disp.MarkRunning(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := f.disp.RecordAgentResult(ctx, id, state.BatchAgentResultRecord{
		AgentID: "agent-1", Success: true,
	}); err != nil {
		t.Fatal(err)
	}
	err = f.disp.RecordAgentResult(ctx, id, state.BatchAgentResultRecord{
		AgentID: "agent-2", Success: true,
	})
	if !errors.Is(err, controlplane.ErrBatchInvalidState) {
		t.Errorf("over-report: %v, want ErrBatchInvalidState", err)
	}
}

func TestRecordAgentResult_RequiresIDs(t *testing.T) {
	ctx := context.Background()
	f := newBatchFixture(t)
	if err := f.disp.RecordAgentResult(ctx, "", state.BatchAgentResultRecord{AgentID: "x"}); err == nil {
		t.Error("empty batch ID should error")
	}
	if err := f.disp.RecordAgentResult(ctx, "x", state.BatchAgentResultRecord{}); err == nil {
		t.Error("empty agent ID should error")
	}
}

func TestFinalize_StatusMatrix(t *testing.T) {
	ctx := context.Background()

	cases := []struct {
		name      string
		successes int
		failures  int
		want      state.BatchJobStatus
	}{
		{"all success", 3, 0, state.BatchJobStatusCompleted},
		{"all failure", 0, 3, state.BatchJobStatusFailed},
		{"mixed", 2, 1, state.BatchJobStatusPartial},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newBatchFixture(t)
			ids := []string{"a1", "a2", "a3"}
			f.seedAgents(t, ids...)

			id, err := f.disp.CreateBatch(ctx, controlplane.BatchRequest{
				Command: "uptime", TotalAgents: 3,
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := f.disp.MarkRunning(ctx, id); err != nil {
				t.Fatal(err)
			}
			i := 0
			for k := 0; k < tc.successes; k++ {
				if err := f.disp.RecordAgentResult(ctx, id, state.BatchAgentResultRecord{
					AgentID: ids[i], Success: true,
				}); err != nil {
					t.Fatal(err)
				}
				i++
			}
			for k := 0; k < tc.failures; k++ {
				if err := f.disp.RecordAgentResult(ctx, id, state.BatchAgentResultRecord{
					AgentID: ids[i], Success: false, ExitCode: 1,
				}); err != nil {
					t.Fatal(err)
				}
				i++
			}

			f.clk.Advance(time.Second)
			got, err := f.disp.Finalize(ctx, id)
			if err != nil {
				t.Fatalf("Finalize: %v", err)
			}
			if got != tc.want {
				t.Errorf("status = %q, want %q", got, tc.want)
			}
			rec, err := f.store.GetBatchJob(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			if rec.Status != tc.want {
				t.Errorf("persisted Status = %q", rec.Status)
			}
			if !rec.CompletedAt.Equal(f.clk.Now()) {
				t.Errorf("CompletedAt = %v, want %v", rec.CompletedAt, f.clk.Now())
			}
		})
	}
}

func TestFinalize_RejectsIncomplete(t *testing.T) {
	ctx := context.Background()
	f := newBatchFixture(t)
	f.seedAgents(t, "agent-1")

	id, err := f.disp.CreateBatch(ctx, controlplane.BatchRequest{
		Command: "uptime", TotalAgents: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.disp.MarkRunning(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := f.disp.RecordAgentResult(ctx, id, state.BatchAgentResultRecord{
		AgentID: "agent-1", Success: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.disp.Finalize(ctx, id); !errors.Is(err, controlplane.ErrBatchInvalidState) {
		t.Errorf("Finalize incomplete: %v, want ErrBatchInvalidState", err)
	}
}

func TestFinalize_RejectsNonRunning(t *testing.T) {
	ctx := context.Background()
	f := newBatchFixture(t)
	id, err := f.disp.CreateBatch(ctx, controlplane.BatchRequest{
		Command: "uptime", TotalAgents: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.disp.Finalize(ctx, id); !errors.Is(err, controlplane.ErrBatchInvalidState) {
		t.Errorf("Finalize pending: %v, want ErrBatchInvalidState", err)
	}
}

func TestCancel_FromPendingAndRunning(t *testing.T) {
	ctx := context.Background()

	t.Run("from pending", func(t *testing.T) {
		f := newBatchFixture(t)
		id, err := f.disp.CreateBatch(ctx, controlplane.BatchRequest{
			Command: "uptime", TotalAgents: 1,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := f.disp.Cancel(ctx, id); err != nil {
			t.Fatalf("Cancel: %v", err)
		}
		rec, _ := f.store.GetBatchJob(ctx, id)
		if rec.Status != state.BatchJobStatusCancelled {
			t.Errorf("Status = %q", rec.Status)
		}
	})

	t.Run("from running", func(t *testing.T) {
		f := newBatchFixture(t)
		id, err := f.disp.CreateBatch(ctx, controlplane.BatchRequest{
			Command: "uptime", TotalAgents: 1,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := f.disp.MarkRunning(ctx, id); err != nil {
			t.Fatal(err)
		}
		if err := f.disp.Cancel(ctx, id); err != nil {
			t.Fatalf("Cancel: %v", err)
		}
	})
}

func TestCancel_TerminalRejected(t *testing.T) {
	ctx := context.Background()
	f := newBatchFixture(t)
	f.seedAgents(t, "agent-1")

	id, err := f.disp.CreateBatch(ctx, controlplane.BatchRequest{
		Command: "uptime", TotalAgents: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.disp.MarkRunning(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := f.disp.RecordAgentResult(ctx, id, state.BatchAgentResultRecord{
		AgentID: "agent-1", Success: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.disp.Finalize(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := f.disp.Cancel(ctx, id); !errors.Is(err, controlplane.ErrBatchFinalized) {
		t.Errorf("Cancel finalized: %v, want ErrBatchFinalized", err)
	}
}

func TestReadPassthroughs(t *testing.T) {
	ctx := context.Background()
	f := newBatchFixture(t)
	f.seedAgents(t, "agent-1")

	id, err := f.disp.CreateBatch(ctx, controlplane.BatchRequest{
		Command: "uptime", TotalAgents: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	got, err := f.disp.GetBatch(ctx, id)
	if err != nil || got.ID != id {
		t.Errorf("GetBatch: %v / %+v", err, got)
	}
	if _, err := f.disp.GetBatch(ctx, "ghost"); !errors.Is(err, controlplane.ErrBatchNotFound) {
		t.Errorf("GetBatch ghost: %v, want ErrBatchNotFound", err)
	}

	list, err := f.disp.ListBatches(ctx, state.BatchJobFilter{})
	if err != nil || len(list) != 1 {
		t.Errorf("ListBatches: %v / %d rows", err, len(list))
	}

	if err := f.disp.MarkRunning(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := f.disp.RecordAgentResult(ctx, id, state.BatchAgentResultRecord{
		AgentID: "agent-1", Success: true,
	}); err != nil {
		t.Fatal(err)
	}
	rows, err := f.disp.ListAgentResults(ctx, id)
	if err != nil || len(rows) != 1 {
		t.Errorf("ListAgentResults: %v / %d rows", err, len(rows))
	}
}

func TestConcurrentRecordAgentResult_SingleBatchSerialized(t *testing.T) {
	ctx := context.Background()
	f := newBatchFixture(t)

	const n = 16
	ids := make([]string, n)
	for i := 0; i < n; i++ {
		ids[i] = fmt.Sprintf("agent-%02d", i)
	}
	f.seedAgents(t, ids...)

	id, err := f.disp.CreateBatch(ctx, controlplane.BatchRequest{
		Command: "uptime", TotalAgents: n,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.disp.MarkRunning(ctx, id); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := f.disp.RecordAgentResult(ctx, id, state.BatchAgentResultRecord{
				AgentID: ids[i], Success: i%2 == 0,
			}); err != nil {
				t.Errorf("RecordAgentResult %d: %v", i, err)
			}
		}()
	}
	wg.Wait()

	rec, err := f.store.GetBatchJob(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if rec.CompletedAgents != n {
		t.Errorf("CompletedAgents = %d, want %d", rec.CompletedAgents, n)
	}
	if rec.SuccessfulAgents+rec.FailedAgents != n {
		t.Errorf("counts don't sum: %+v", rec)
	}
}

func TestConcurrentRecordAgentResult_CrossBatchParallel(t *testing.T) {
	// Two independent batches make progress concurrently — exercises
	// the cross-batch concurrency path (per-batch mutex isolation).
	ctx := context.Background()
	f := newBatchFixture(t)

	type batch struct {
		id     string
		agents []string
	}
	mk := func(prefix string) batch {
		ids := []string{prefix + "-a", prefix + "-b"}
		f.seedAgents(t, ids...)
		id, err := f.disp.CreateBatch(ctx, controlplane.BatchRequest{
			Command: "uptime", TotalAgents: len(ids),
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := f.disp.MarkRunning(ctx, id); err != nil {
			t.Fatal(err)
		}
		return batch{id: id, agents: ids}
	}
	b1, b2 := mk("b1"), mk("b2")

	var wg sync.WaitGroup
	for _, b := range []batch{b1, b2} {
		b := b
		for _, a := range b.agents {
			a := a
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := f.disp.RecordAgentResult(ctx, b.id, state.BatchAgentResultRecord{
					AgentID: a, Success: true,
				}); err != nil {
					t.Errorf("RecordAgentResult %s/%s: %v", b.id, a, err)
				}
			}()
		}
	}
	wg.Wait()

	for _, b := range []batch{b1, b2} {
		rec, err := f.store.GetBatchJob(ctx, b.id)
		if err != nil {
			t.Fatal(err)
		}
		if rec.CompletedAgents != len(b.agents) {
			t.Errorf("batch %s completed = %d, want %d",
				b.id, rec.CompletedAgents, len(b.agents))
		}
	}
}
