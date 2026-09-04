// SPDX-License-Identifier: Apache-2.0

package controlplane

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"go.keystone-core.io/keystone-core/internal/state"
)

// DefaultBatchConcurrency aligns with PROJECT-DETAILS §4.7 — the v1.0
// fleet-side concurrency cap when a caller doesn't override it.
const DefaultBatchConcurrency = 10

// BatchExecutor is the per-agent execution surface that Epic 07's
// runner implements. The dispatcher in Epic 04 task 3 only orchestrates
// state — it never calls Execute itself; the type lives here so Epic 07
// can wire its runner against a stable interface.
//
// Implementations must enforce concurrency, timeouts, and result
// propagation; the dispatcher only accepts results via RecordAgentResult.
type BatchExecutor interface {
	Execute(ctx context.Context, batchID, agentID, command string, args []string) (state.BatchAgentResultRecord, error)
}

// BatchRequest is the input shape for CreateBatch.
type BatchRequest struct {
	ID          string         // optional; UUID generated if empty
	Target      map[string]any // opaque selector (Epic 07 compiles to TargetExpression)
	Command     string
	Args        []string
	Concurrency int // 0 → BatchDispatcherConfig.DefaultConcurrency
	TotalAgents int // size of pre-resolved agent set; must be > 0
}

// BatchDispatcherConfig configures a BatchDispatcher. Store is
// required; everything else has a default.
type BatchDispatcherConfig struct {
	Store              state.BatchJobStore
	Logger             *slog.Logger
	DefaultConcurrency int
	Clock              func() time.Time
	NewID              func() string
}

// BatchDispatcher is the v1.0 batch state machine + persistence
// orchestrator. Epic 04 task 3 shipped the bookkeeping primitives
// (CreateBatch / MarkRunning / RecordAgentResult / Finalize / Cancel);
// Epic 07 task 8 added ExecuteBatch on top to drive the per-agent
// dispatch loop with semaphore concurrency, a 500ms progress ticker,
// and result aggregation per §4.7.
type BatchDispatcher struct {
	store              state.BatchJobStore
	logger             *slog.Logger
	defaultConcurrency int
	now                func() time.Time
	newID              func() string

	mu       sync.Mutex
	counters map[string]*batchCounter
	cancels  map[string]context.CancelFunc // per-running-batch orchestrator cancels
	wg       sync.WaitGroup                // tracks orchestration goroutines for shutdown
}

// batchCounter holds in-memory live counts for a single batch, guarded
// by its own mutex so cross-batch traffic stays concurrent.
//
// On crash mid-batch the counters are lost and the batch row stays
// RUNNING — orphan recovery is deferred to Epic 07 / 13. Documented
// gap, not a bug.
type batchCounter struct {
	mu         sync.Mutex
	total      int
	completed  int
	successful int
	failed     int
}

// NewBatchDispatcher validates cfg, fills defaults, and returns a
// dispatcher. There is no Start/Stop — the dispatcher is "lazy" per
// §4.4 step 8.
func NewBatchDispatcher(cfg BatchDispatcherConfig) (*BatchDispatcher, error) {
	if cfg.Store == nil {
		return nil, errors.New("controlplane: batch dispatcher Store is required")
	}
	if cfg.DefaultConcurrency < 0 {
		return nil, fmt.Errorf("controlplane: DefaultConcurrency must be >= 0, got %d", cfg.DefaultConcurrency)
	}
	if cfg.DefaultConcurrency == 0 {
		cfg.DefaultConcurrency = DefaultBatchConcurrency
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	if cfg.NewID == nil {
		cfg.NewID = uuid.NewString
	}
	return &BatchDispatcher{
		store:              cfg.Store,
		logger:             cfg.Logger,
		defaultConcurrency: cfg.DefaultConcurrency,
		now:                cfg.Clock,
		newID:              cfg.NewID,
		counters:           make(map[string]*batchCounter),
		cancels:            make(map[string]context.CancelFunc),
	}, nil
}

// CreateBatch persists a pending batch and returns its ID. Does NOT
// start execution — Epic 07's runner picks up via MarkRunning.
func (d *BatchDispatcher) CreateBatch(ctx context.Context, req BatchRequest) (string, error) {
	if req.Command == "" {
		return "", fmt.Errorf("%w: Command is required", ErrInvalidBatchRequest)
	}
	if req.TotalAgents <= 0 {
		return "", fmt.Errorf("%w: TotalAgents must be > 0", ErrInvalidBatchRequest)
	}

	id := req.ID
	if id == "" {
		id = d.newID()
	}
	concurrency := req.Concurrency
	if concurrency <= 0 {
		concurrency = d.defaultConcurrency
	}

	rec := &state.BatchJobRecord{
		ID:          id,
		Target:      req.Target,
		Command:     req.Command,
		Args:        req.Args,
		Status:      state.BatchJobStatusPending,
		Concurrency: concurrency,
		TotalAgents: req.TotalAgents,
		CreatedAt:   d.now(),
	}
	if err := d.store.CreateBatchJob(ctx, rec); err != nil {
		return "", fmt.Errorf("controlplane: persist batch: %w", err)
	}

	d.mu.Lock()
	d.counters[id] = &batchCounter{total: req.TotalAgents}
	d.mu.Unlock()

	d.logger.Debug("controlplane: batch created",
		"batch_id", id, "total_agents", req.TotalAgents, "concurrency", concurrency)
	return id, nil
}

// MarkRunning transitions pending → running and stamps StartedAt.
// Returns ErrBatchInvalidState if the current status is anything else.
func (d *BatchDispatcher) MarkRunning(ctx context.Context, id string) error {
	rec, err := d.getBatch(ctx, id)
	if err != nil {
		return err
	}
	if rec.Status != state.BatchJobStatusPending {
		return fmt.Errorf("%w: status=%q", ErrBatchInvalidState, rec.Status)
	}
	if err := d.store.MarkBatchJobRunning(ctx, id, d.now()); err != nil {
		return fmt.Errorf("controlplane: mark batch running %q: %w", id, err)
	}

	// Make sure the in-memory counter exists; CreateBatch usually
	// allocates it, but MarkRunning may also be called against a
	// recovered/external batch row.
	d.mu.Lock()
	if _, ok := d.counters[id]; !ok {
		d.counters[id] = &batchCounter{total: rec.TotalAgents}
	}
	d.mu.Unlock()

	return nil
}

// RecordAgentResult appends a per-agent result and increments live
// count fields atomically per-batch. Returns ErrBatchInvalidState if
// the batch is not running, ErrBatchInvalidState if the result would
// over-report (count > total).
func (d *BatchDispatcher) RecordAgentResult(ctx context.Context, batchID string, result state.BatchAgentResultRecord) error {
	if batchID == "" {
		return errors.New("controlplane: RecordAgentResult requires a batch ID")
	}
	if result.AgentID == "" {
		return errors.New("controlplane: RecordAgentResult requires an agent ID")
	}

	rec, err := d.getBatch(ctx, batchID)
	if err != nil {
		return err
	}
	if rec.Status != state.BatchJobStatusRunning {
		return fmt.Errorf("%w: status=%q", ErrBatchInvalidState, rec.Status)
	}

	counter := d.getCounter(batchID, rec.TotalAgents)
	counter.mu.Lock()
	defer counter.mu.Unlock()

	if counter.completed >= counter.total {
		return fmt.Errorf("%w: completed=%d already equals total=%d",
			ErrBatchInvalidState, counter.completed, counter.total)
	}

	result.BatchJobID = batchID
	if err := d.store.CreateBatchAgentResult(ctx, &result); err != nil {
		return fmt.Errorf("controlplane: persist agent result: %w", err)
	}

	counter.completed++
	if result.Success {
		counter.successful++
	} else {
		counter.failed++
	}

	if err := d.store.UpdateBatchJobCounts(ctx, batchID,
		counter.completed, counter.successful, counter.failed); err != nil {
		// In-memory counter already advanced; reverting to keep DB
		// and memory in lockstep would require a compensating
		// transaction we don't have. Log loudly so an operator
		// notices the divergence.
		d.logger.Warn("controlplane: update batch counts failed; in-memory counter ahead of DB",
			"batch_id", batchID, "err", err)
		return fmt.Errorf("controlplane: update batch counts: %w", err)
	}
	return nil
}

// Finalize transitions a running batch to the appropriate terminal
// status based on the live counters:
//
//	successful == total → completed
//	failed == total     → failed
//	otherwise           → partial
//
// Returns ErrBatchInvalidState if not all agents have reported.
func (d *BatchDispatcher) Finalize(ctx context.Context, id string) (state.BatchJobStatus, error) {
	rec, err := d.getBatch(ctx, id)
	if err != nil {
		return "", err
	}
	if rec.Status != state.BatchJobStatusRunning {
		return "", fmt.Errorf("%w: status=%q", ErrBatchInvalidState, rec.Status)
	}

	counter := d.getCounter(id, rec.TotalAgents)
	counter.mu.Lock()
	defer counter.mu.Unlock()

	if counter.completed < counter.total {
		return "", fmt.Errorf("%w: %d of %d agents reported",
			ErrBatchInvalidState, counter.completed, counter.total)
	}

	var status state.BatchJobStatus
	switch {
	case counter.successful == counter.total:
		status = state.BatchJobStatusCompleted
	case counter.failed == counter.total:
		status = state.BatchJobStatusFailed
	default:
		status = state.BatchJobStatusPartial
	}

	if err := d.store.FinalizeBatchJob(ctx, id, status, d.now()); err != nil {
		return "", fmt.Errorf("controlplane: finalize batch %q: %w", id, err)
	}

	d.mu.Lock()
	delete(d.counters, id)
	d.mu.Unlock()

	d.logger.Info("controlplane: batch finalized",
		"batch_id", id, "status", status,
		"successful", counter.successful, "failed", counter.failed)
	return status, nil
}

// Cancel transitions pending or running → cancelled. Best-effort
// signal-the-agents wiring lands with Epic 07's runner.
func (d *BatchDispatcher) Cancel(ctx context.Context, id string) error {
	rec, err := d.getBatch(ctx, id)
	if err != nil {
		return err
	}
	switch rec.Status {
	case state.BatchJobStatusPending, state.BatchJobStatusRunning:
		// allowed
	case state.BatchJobStatusCompleted, state.BatchJobStatusFailed,
		state.BatchJobStatusPartial, state.BatchJobStatusCancelled:
		return ErrBatchFinalized
	default:
		return fmt.Errorf("%w: status=%q", ErrBatchInvalidState, rec.Status)
	}

	if err := d.store.FinalizeBatchJob(ctx, id, state.BatchJobStatusCancelled, d.now()); err != nil {
		return fmt.Errorf("controlplane: cancel batch %q: %w", id, err)
	}

	d.mu.Lock()
	delete(d.counters, id)
	cancel := d.cancels[id]
	delete(d.cancels, id)
	d.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
}

// GetBatch is a read passthrough.
func (d *BatchDispatcher) GetBatch(ctx context.Context, id string) (*state.BatchJobRecord, error) {
	return d.getBatch(ctx, id)
}

// ListBatches is a read passthrough.
func (d *BatchDispatcher) ListBatches(ctx context.Context, filter state.BatchJobFilter) ([]*state.BatchJobRecord, error) {
	return d.store.ListBatchJobs(ctx, filter)
}

// ListAgentResults is a read passthrough.
func (d *BatchDispatcher) ListAgentResults(ctx context.Context, batchID string) ([]*state.BatchAgentResultRecord, error) {
	return d.store.ListBatchAgentResults(ctx, batchID)
}

// GetAgentResult is a read passthrough that maps state.ErrNotFound to
// the dispatcher's ErrBatchNotFound sentinel so callers can use one
// errors.Is check across the surface.
func (d *BatchDispatcher) GetAgentResult(ctx context.Context, batchID, agentID string) (*state.BatchAgentResultRecord, error) {
	r, err := d.store.GetBatchAgentResult(ctx, batchID, agentID)
	if err != nil {
		if errors.Is(err, state.ErrNotFound) {
			return nil, ErrBatchNotFound
		}
		return nil, fmt.Errorf("controlplane: get agent result: %w", err)
	}
	return r, nil
}

// ---- helpers --------------------------------------------------------------

func (d *BatchDispatcher) getBatch(ctx context.Context, id string) (*state.BatchJobRecord, error) {
	if id == "" {
		return nil, errors.New("controlplane: batch ID is required")
	}
	rec, err := d.store.GetBatchJob(ctx, id)
	if err != nil {
		if errors.Is(err, state.ErrNotFound) {
			return nil, ErrBatchNotFound
		}
		return nil, fmt.Errorf("controlplane: get batch %q: %w", id, err)
	}
	return rec, nil
}

func (d *BatchDispatcher) getCounter(id string, total int) *batchCounter {
	d.mu.Lock()
	defer d.mu.Unlock()
	c, ok := d.counters[id]
	if !ok {
		c = &batchCounter{total: total}
		d.counters[id] = c
	}
	return c
}
