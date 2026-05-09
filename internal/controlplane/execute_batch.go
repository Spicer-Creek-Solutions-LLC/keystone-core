package controlplane

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.keystone-core.io/keystone-core/internal/state"
)

// progressTickInterval is the §4.7 v1.0 cadence. Slow consumers drop
// progress events but never block the orchestrator (back-pressure
// boundary documented in PROJECT-DETAILS §4.7).
const progressTickInterval = 500 * time.Millisecond

// BatchProgressPhase tags a BatchProgressEvent so consumers can
// distinguish the initial fan-out announcement, the periodic count
// updates, and the terminal event.
type BatchProgressPhase string

const (
	BatchProgressPhaseStart    BatchProgressPhase = "start"
	BatchProgressPhaseProgress BatchProgressPhase = "progress"
	BatchProgressPhaseComplete BatchProgressPhase = "complete"
)

// BatchProgressEvent is one tick of the in-flight progress stream.
// Status is set on Complete events; intermediate events leave it as
// the running status (which the dispatcher does not surface here —
// callers infer "still running" from a Progress phase).
type BatchProgressEvent struct {
	BatchID    string
	Phase      BatchProgressPhase
	Total      int
	Completed  int
	Successful int
	Failed     int
	Status     state.BatchJobStatus // set on Complete; empty otherwise
}

// BatchSummary aggregates a finalized batch's counts. SuccessRate is
// successful / total in [0, 1]; 0 for batches with zero agents to
// avoid divide-by-zero.
type BatchSummary struct {
	Total       int
	Successful  int
	Failed      int
	SuccessRate float64
}

// ExecuteBatch persists a new batch and orchestrates per-agent dispatch
// asynchronously. The synchronous portion validates inputs, calls
// CreateBatch, and registers a per-batch cancel function so Cancel(id)
// can stop the orchestration mid-flight. The async portion is a
// goroutine that:
//
//  1. MarkRunning.
//  2. Spawns a 500ms-ticker progress goroutine (skipped when progress
//     is nil).
//  3. Fans out per-agent BatchExecutor.Execute calls under a semaphore
//     sized to req.Concurrency (or the dispatcher default).
//  4. Records each result via RecordAgentResult.
//  5. On all-agents-reported, Finalize. On Cancel mid-flight, drains
//     in-flight goroutines and skips Finalize (Cancel already wrote
//     the terminal status).
//
// The orchestrator runs under a context decoupled from ctx (via
// context.WithoutCancel) so request-scoped callers (gRPC handlers)
// can return immediately without aborting the batch.
func (d *BatchDispatcher) ExecuteBatch(
	ctx context.Context,
	req BatchRequest,
	agentIDs []string,
	exec BatchExecutor,
	progress chan<- BatchProgressEvent,
) (string, error) {
	if exec == nil {
		return "", fmt.Errorf("%w: BatchExecutor is required", ErrInvalidBatchRequest)
	}
	if len(agentIDs) == 0 {
		return "", fmt.Errorf("%w: agentIDs must be non-empty", ErrInvalidBatchRequest)
	}

	req.TotalAgents = len(agentIDs)
	id, err := d.CreateBatch(ctx, req)
	if err != nil {
		return "", err
	}

	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	d.mu.Lock()
	d.cancels[id] = cancel
	d.mu.Unlock()

	d.wg.Add(1)
	go d.runBatch(runCtx, cancel, id, req, agentIDs, exec, progress) //nolint:gosec // G118: detaching from request ctx is the intent — orchestration outlives the gRPC handler that returned the batch ID
	return id, nil
}

// Summary reads the finalized batch row and computes BatchSummary.
// Safe to call before Finalize — counts reflect whatever has been
// persisted at the moment of the call.
func (d *BatchDispatcher) Summary(ctx context.Context, batchID string) (BatchSummary, error) {
	rec, err := d.getBatch(ctx, batchID)
	if err != nil {
		return BatchSummary{}, err
	}
	s := BatchSummary{
		Total:      rec.TotalAgents,
		Successful: rec.SuccessfulAgents,
		Failed:     rec.FailedAgents,
	}
	if s.Total > 0 {
		s.SuccessRate = float64(s.Successful) / float64(s.Total)
	}
	return s, nil
}

// runBatch is the orchestration goroutine spawned by ExecuteBatch. It
// owns its own ctx and cleans up the per-batch cancel registration on
// exit.
func (d *BatchDispatcher) runBatch(
	ctx context.Context,
	cancel context.CancelFunc,
	batchID string,
	req BatchRequest,
	agentIDs []string,
	exec BatchExecutor,
	progress chan<- BatchProgressEvent,
) {
	defer d.wg.Done()
	defer d.unregisterCancel(batchID, cancel)

	if err := d.MarkRunning(ctx, batchID); err != nil {
		// Cancelled before start, or already running — either way the
		// orchestration aborts. Cancel already persisted the terminal
		// status if applicable.
		d.logger.Debug("controlplane: batch did not start",
			"batch_id", batchID, "err", err)
		d.sendProgress(progress, d.snapshotEvent(batchID, BatchProgressPhaseComplete))
		return
	}

	concurrency := req.Concurrency
	if concurrency <= 0 {
		concurrency = d.defaultConcurrency
	}

	tickerCtx, stopTicker := context.WithCancel(ctx)
	defer stopTicker()
	if progress != nil {
		d.sendProgress(progress, d.snapshotEvent(batchID, BatchProgressPhaseStart))
		go d.progressLoop(tickerCtx, batchID, progress)
	}

	sem := make(chan struct{}, concurrency)
	var fanoutWG sync.WaitGroup

agentLoop:
	for _, agentID := range agentIDs {
		select {
		case <-ctx.Done():
			break agentLoop
		case sem <- struct{}{}:
		}

		agentID := agentID
		fanoutWG.Add(1)
		go func() {
			defer fanoutWG.Done()
			defer func() { <-sem }()
			d.runOneAgent(ctx, batchID, agentID, req.Command, req.Args, exec)
		}()
	}

	fanoutWG.Wait()
	stopTicker()

	cancelled := ctx.Err() != nil
	if !cancelled {
		if _, err := d.Finalize(ctx, batchID); err != nil {
			d.logger.Warn("controlplane: finalize batch failed",
				"batch_id", batchID, "err", err)
		}
	}

	// Build the terminal event from the persisted record. The
	// in-memory counter was wiped by Finalize / Cancel, so reading
	// the store is the only reliable source of truth here.
	final := BatchProgressEvent{BatchID: batchID, Phase: BatchProgressPhaseComplete}
	if rec, err := d.store.GetBatchJob(context.Background(), batchID); err == nil {
		final.Total = rec.TotalAgents
		final.Completed = rec.CompletedAgents
		final.Successful = rec.SuccessfulAgents
		final.Failed = rec.FailedAgents
		final.Status = rec.Status
	}
	d.sendProgress(progress, final)
}

// runOneAgent calls exec.Execute and records the result. Errors from
// exec are converted into a failed BatchAgentResultRecord so the
// dispatcher's state machine sees one result per agent regardless of
// whether the executor succeeded or returned a Go error.
func (d *BatchDispatcher) runOneAgent(
	ctx context.Context,
	batchID, agentID, command string,
	args []string,
	exec BatchExecutor,
) {
	result, err := exec.Execute(ctx, batchID, agentID, command, args)
	if err != nil {
		result = state.BatchAgentResultRecord{
			AgentID:     agentID,
			Success:     false,
			Error:       err.Error(),
			StartedAt:   d.now(),
			CompletedAt: d.now(),
		}
	}
	result.AgentID = agentID
	if recErr := d.RecordAgentResult(ctx, batchID, result); recErr != nil {
		// After Cancel the batch is no longer Running and
		// RecordAgentResult will refuse with ErrBatchInvalidState —
		// expected, log at debug.
		if errors.Is(recErr, ErrBatchInvalidState) {
			d.logger.Debug("controlplane: result dropped after cancel",
				"batch_id", batchID, "agent_id", agentID)
			return
		}
		d.logger.Warn("controlplane: record agent result failed",
			"batch_id", batchID, "agent_id", agentID, "err", recErr)
	}
}

// progressLoop emits a Progress event every progressTickInterval until
// ctx fires. Sends are non-blocking — slow consumers drop progress
// events per the §4.7 back-pressure boundary.
func (d *BatchDispatcher) progressLoop(ctx context.Context, batchID string, progress chan<- BatchProgressEvent) {
	t := time.NewTicker(progressTickInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			d.sendProgress(progress, d.snapshotEvent(batchID, BatchProgressPhaseProgress))
		}
	}
}

// snapshotEvent reads the current counter without blocking other
// recorders — uses Lock briefly to copy the four fields.
func (d *BatchDispatcher) snapshotEvent(batchID string, phase BatchProgressPhase) BatchProgressEvent {
	d.mu.Lock()
	c := d.counters[batchID]
	d.mu.Unlock()
	if c == nil {
		return BatchProgressEvent{BatchID: batchID, Phase: phase}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return BatchProgressEvent{
		BatchID:    batchID,
		Phase:      phase,
		Total:      c.total,
		Completed:  c.completed,
		Successful: c.successful,
		Failed:     c.failed,
	}
}

// sendProgress is a non-blocking send. If progress is nil or the
// receiver is slow, the event is dropped silently.
func (d *BatchDispatcher) sendProgress(progress chan<- BatchProgressEvent, ev BatchProgressEvent) {
	if progress == nil {
		return
	}
	select {
	case progress <- ev:
	default:
	}
}

// unregisterCancel removes the per-batch cancel from the registry on
// orchestrator exit. Cancel() may have already deleted the entry; that
// double-delete is a no-op. CreateBatch rejects duplicate IDs, so a
// fresh ExecuteBatch can't have racing-registered a different cancel
// under the same batchID.
func (d *BatchDispatcher) unregisterCancel(batchID string, _ context.CancelFunc) {
	d.mu.Lock()
	delete(d.cancels, batchID)
	d.mu.Unlock()
}
