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

// scriptedBatchExecutor is the test stand-in for the eventual NATS-
// backed runner. It returns a deterministic outcome per agent and
// optionally blocks long enough that the orchestrator's progress
// ticker can fire and concurrency can be observed.
type scriptedBatchExecutor struct {
	mu           sync.Mutex
	calls        int
	inFlight     atomic.Int32
	peakInFlight atomic.Int32

	// outcomes keyed by agentID; falls back to defaultSuccess.
	outcomes       map[string]agentOutcome
	defaultSuccess bool
	delay          time.Duration

	// onExecute, if non-nil, runs at the start of every Execute call.
	onExecute func(agentID string)
}

type agentOutcome struct {
	success  bool
	exit     int
	errorStr string
	delay    time.Duration
	err      error // returned as Go error from Execute
}

func (e *scriptedBatchExecutor) Execute(ctx context.Context, batchID, agentID, command string, args []string) (state.BatchAgentResultRecord, error) {
	cur := e.inFlight.Add(1)
	defer e.inFlight.Add(-1)
	for {
		peak := e.peakInFlight.Load()
		if cur <= peak || e.peakInFlight.CompareAndSwap(peak, cur) {
			break
		}
	}

	e.mu.Lock()
	e.calls++
	out, ok := e.outcomes[agentID]
	delay := e.delay
	if out.delay > 0 {
		delay = out.delay
	}
	if e.onExecute != nil {
		e.onExecute(agentID)
	}
	e.mu.Unlock()

	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
		}
	}

	if !ok {
		out = agentOutcome{success: e.defaultSuccess, exit: 0}
		if !e.defaultSuccess {
			out.errorStr = "default-fail"
		}
	}
	if out.err != nil {
		return state.BatchAgentResultRecord{}, out.err
	}
	return state.BatchAgentResultRecord{
		AgentID:     agentID,
		Success:     out.success,
		ExitCode:    out.exit,
		Error:       out.errorStr,
		StartedAt:   time.Now(),
		CompletedAt: time.Now(),
	}, nil
}

func TestExecuteBatch_HappyPath_FiveAgentsAllSucceed(t *testing.T) {
	t.Parallel()

	fix := newBatchFixture(t)
	agentIDs := []string{"a-1", "a-2", "a-3", "a-4", "a-5"}
	fix.seedAgents(t, agentIDs...)

	exec := &scriptedBatchExecutor{defaultSuccess: true}

	id, err := fix.disp.ExecuteBatch(context.Background(),
		controlplane.BatchRequest{Command: "uptime"},
		agentIDs, exec, nil)
	if err != nil {
		t.Fatalf("ExecuteBatch: %v", err)
	}

	waitForBatchStatus(t, fix, id, state.BatchJobStatusCompleted, 2*time.Second)

	rec, _ := fix.disp.GetBatch(context.Background(), id)
	if rec.SuccessfulAgents != 5 || rec.FailedAgents != 0 || rec.CompletedAgents != 5 {
		t.Errorf("counts: succ=%d fail=%d comp=%d, want 5/0/5",
			rec.SuccessfulAgents, rec.FailedAgents, rec.CompletedAgents)
	}
	if exec.calls != 5 {
		t.Errorf("executor calls = %d, want 5", exec.calls)
	}
	results, _ := fix.disp.ListAgentResults(context.Background(), id)
	if len(results) != 5 {
		t.Errorf("agent results = %d, want 5", len(results))
	}

	sum, err := fix.disp.Summary(context.Background(), id)
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if sum.Total != 5 || sum.Successful != 5 || sum.Failed != 0 || sum.SuccessRate != 1.0 {
		t.Errorf("summary = %+v, want all-success", sum)
	}
}

func TestExecuteBatch_MixedResults_StatusPartial(t *testing.T) {
	t.Parallel()

	fix := newBatchFixture(t)
	agentIDs := []string{"m-1", "m-2", "m-3", "m-4", "m-5"}
	fix.seedAgents(t, agentIDs...)

	exec := &scriptedBatchExecutor{
		outcomes: map[string]agentOutcome{
			"m-1": {success: true},
			"m-2": {success: true},
			"m-3": {success: false, exit: 1, errorStr: "boom"},
			"m-4": {success: false, exit: 1, errorStr: "boom"},
			"m-5": {success: true},
		},
	}

	id, err := fix.disp.ExecuteBatch(context.Background(),
		controlplane.BatchRequest{Command: "flaky"},
		agentIDs, exec, nil)
	if err != nil {
		t.Fatal(err)
	}
	waitForBatchStatus(t, fix, id, state.BatchJobStatusPartial, 2*time.Second)

	sum, _ := fix.disp.Summary(context.Background(), id)
	if sum.Successful != 3 || sum.Failed != 2 {
		t.Errorf("summary = %+v, want 3/2", sum)
	}
	if sum.SuccessRate != 0.6 {
		t.Errorf("SuccessRate = %v, want 0.6", sum.SuccessRate)
	}
}

func TestExecuteBatch_AllFail_StatusFailed(t *testing.T) {
	t.Parallel()

	fix := newBatchFixture(t)
	agentIDs := []string{"f-1", "f-2", "f-3"}
	fix.seedAgents(t, agentIDs...)

	exec := &scriptedBatchExecutor{defaultSuccess: false}

	id, _ := fix.disp.ExecuteBatch(context.Background(),
		controlplane.BatchRequest{Command: "always-fail"},
		agentIDs, exec, nil)
	waitForBatchStatus(t, fix, id, state.BatchJobStatusFailed, 2*time.Second)

	sum, _ := fix.disp.Summary(context.Background(), id)
	if sum.Failed != 3 {
		t.Errorf("Failed = %d, want 3", sum.Failed)
	}
}

func TestExecuteBatch_ConcurrencyCap(t *testing.T) {
	t.Parallel()

	fix := newBatchFixture(t)
	agentIDs := []string{"c-1", "c-2", "c-3", "c-4", "c-5", "c-6"}
	fix.seedAgents(t, agentIDs...)

	exec := &scriptedBatchExecutor{
		defaultSuccess: true,
		delay:          50 * time.Millisecond,
	}

	id, _ := fix.disp.ExecuteBatch(context.Background(),
		controlplane.BatchRequest{Command: "wait", Concurrency: 2},
		agentIDs, exec, nil)
	waitForBatchStatus(t, fix, id, state.BatchJobStatusCompleted, 5*time.Second)

	if peak := exec.peakInFlight.Load(); peak > 2 {
		t.Errorf("peak in-flight = %d, want <= 2 (concurrency cap)", peak)
	}
}

func TestExecuteBatch_ProgressEvents(t *testing.T) {
	t.Parallel()

	fix := newBatchFixture(t)
	agentIDs := []string{"p-1", "p-2", "p-3"}
	fix.seedAgents(t, agentIDs...)

	exec := &scriptedBatchExecutor{
		defaultSuccess: true,
		delay:          200 * time.Millisecond, // ensure ticker fires
	}

	progress := make(chan controlplane.BatchProgressEvent, 64)
	if _, err := fix.disp.ExecuteBatch(context.Background(),
		controlplane.BatchRequest{Command: "slow"},
		agentIDs, exec, progress); err != nil {
		t.Fatal(err)
	}

	// Drain the channel until we see a Complete event, then stop. The
	// orchestrator owns the channel lifetime; closing from the test
	// would race with its final send.
	var phases []controlplane.BatchProgressPhase
	var lastComplete controlplane.BatchProgressEvent
	deadline := time.After(5 * time.Second)
drain:
	for {
		select {
		case ev := <-progress:
			phases = append(phases, ev.Phase)
			if ev.Phase == controlplane.BatchProgressPhaseComplete {
				lastComplete = ev
				break drain
			}
		case <-deadline:
			t.Fatalf("never received Complete event; phases so far = %v", phases)
		}
	}
	if len(phases) < 2 {
		t.Fatalf("phases = %v, want at least Start + Complete", phases)
	}
	if phases[0] != controlplane.BatchProgressPhaseStart {
		t.Errorf("first phase = %v, want Start", phases[0])
	}
	if last := phases[len(phases)-1]; last != controlplane.BatchProgressPhaseComplete {
		t.Errorf("last phase = %v, want Complete", last)
	}
	if lastComplete.Status != state.BatchJobStatusCompleted {
		t.Errorf("Complete event Status = %q, want completed", lastComplete.Status)
	}
	if lastComplete.Successful != 3 {
		t.Errorf("Complete event Successful = %d, want 3", lastComplete.Successful)
	}
}

func TestExecuteBatch_NilProgressChannelSafe(t *testing.T) {
	t.Parallel()

	fix := newBatchFixture(t)
	fix.seedAgents(t, "n-1")
	exec := &scriptedBatchExecutor{defaultSuccess: true}
	id, err := fix.disp.ExecuteBatch(context.Background(),
		controlplane.BatchRequest{Command: "x"},
		[]string{"n-1"}, exec, nil)
	if err != nil {
		t.Fatal(err)
	}
	waitForBatchStatus(t, fix, id, state.BatchJobStatusCompleted, 2*time.Second)
}

func TestExecuteBatch_CancelMidFlight(t *testing.T) {
	t.Parallel()

	fix := newBatchFixture(t)
	agentIDs := []string{"k-1", "k-2", "k-3", "k-4", "k-5"}
	fix.seedAgents(t, agentIDs...)

	started := make(chan struct{}, 1)
	exec := &scriptedBatchExecutor{
		defaultSuccess: true,
		delay:          500 * time.Millisecond,
		onExecute: func(_ string) {
			select {
			case started <- struct{}{}:
			default:
			}
		},
	}

	id, err := fix.disp.ExecuteBatch(context.Background(),
		controlplane.BatchRequest{Command: "long", Concurrency: 1},
		agentIDs, exec, nil)
	if err != nil {
		t.Fatal(err)
	}

	<-started
	if err := fix.disp.Cancel(context.Background(), id); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	// Wait for the orchestrator to drain.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		rec, _ := fix.disp.GetBatch(context.Background(), id)
		if rec.Status == state.BatchJobStatusCancelled {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	rec, _ := fix.disp.GetBatch(context.Background(), id)
	if rec.Status != state.BatchJobStatusCancelled {
		t.Errorf("status = %q, want cancelled", rec.Status)
	}
	if exec.calls >= len(agentIDs) {
		t.Errorf("executor calls = %d, want < %d (cancel should have stopped fan-out)",
			exec.calls, len(agentIDs))
	}
}

func TestExecuteBatch_EmptyAgentIDs(t *testing.T) {
	t.Parallel()

	fix := newBatchFixture(t)
	exec := &scriptedBatchExecutor{}
	_, err := fix.disp.ExecuteBatch(context.Background(),
		controlplane.BatchRequest{Command: "x"},
		nil, exec, nil)
	if !errors.Is(err, controlplane.ErrInvalidBatchRequest) {
		t.Errorf("err = %v, want ErrInvalidBatchRequest", err)
	}
}

func TestExecuteBatch_NilExecutor(t *testing.T) {
	t.Parallel()

	fix := newBatchFixture(t)
	fix.seedAgents(t, "x-1")
	_, err := fix.disp.ExecuteBatch(context.Background(),
		controlplane.BatchRequest{Command: "x"},
		[]string{"x-1"}, nil, nil)
	if !errors.Is(err, controlplane.ErrInvalidBatchRequest) {
		t.Errorf("err = %v, want ErrInvalidBatchRequest", err)
	}
}

func TestExecuteBatch_GoErrorFromExecutorCountsAsFailure(t *testing.T) {
	t.Parallel()

	fix := newBatchFixture(t)
	fix.seedAgents(t, "g-1", "g-2")

	exec := &scriptedBatchExecutor{
		outcomes: map[string]agentOutcome{
			"g-1": {success: true},
			"g-2": {err: fmt.Errorf("nats publish blew up")},
		},
	}
	id, _ := fix.disp.ExecuteBatch(context.Background(),
		controlplane.BatchRequest{Command: "x"},
		[]string{"g-1", "g-2"}, exec, nil)
	waitForBatchStatus(t, fix, id, state.BatchJobStatusPartial, 2*time.Second)

	results, _ := fix.disp.ListAgentResults(context.Background(), id)
	var failed *state.BatchAgentResultRecord
	for _, r := range results {
		if r.AgentID == "g-2" {
			r := r
			failed = r
		}
	}
	if failed == nil || failed.Success {
		t.Fatalf("expected g-2 result to be a recorded failure; got %+v", failed)
	}
	if failed.Error == "" {
		t.Error("failed result must carry the executor error string")
	}
}

func TestExecuteBatch_SlowProgressConsumerDoesNotStall(t *testing.T) {
	t.Parallel()

	fix := newBatchFixture(t)
	fix.seedAgents(t, "s-1", "s-2", "s-3")

	exec := &scriptedBatchExecutor{defaultSuccess: true, delay: 200 * time.Millisecond}

	// Capacity-1 channel never read — sends after the first must
	// drop, but the orchestrator must still complete.
	progress := make(chan controlplane.BatchProgressEvent, 1)
	id, _ := fix.disp.ExecuteBatch(context.Background(),
		controlplane.BatchRequest{Command: "slow"},
		[]string{"s-1", "s-2", "s-3"}, exec, progress)
	waitForBatchStatus(t, fix, id, state.BatchJobStatusCompleted, 5*time.Second)
}

func TestExecuteBatch_CallerSuppliedID(t *testing.T) {
	t.Parallel()

	fix := newBatchFixture(t)
	fix.seedAgents(t, "x-1")
	exec := &scriptedBatchExecutor{defaultSuccess: true}
	got, err := fix.disp.ExecuteBatch(context.Background(),
		controlplane.BatchRequest{ID: "explicit-id", Command: "x"},
		[]string{"x-1"}, exec, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != "explicit-id" {
		t.Errorf("ID = %q, want explicit-id", got)
	}
}

// waitForBatchStatus polls the batch row until it reaches want or the
// deadline elapses. Required because ExecuteBatch is async.
func waitForBatchStatus(t *testing.T, fix *batchFixture, id string, want state.BatchJobStatus, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		rec, err := fix.disp.GetBatch(context.Background(), id)
		if err == nil && rec.Status == want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	rec, _ := fix.disp.GetBatch(context.Background(), id)
	t.Fatalf("waiting for status %q timed out; current = %q", want, rec.Status)
}
