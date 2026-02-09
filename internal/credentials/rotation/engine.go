package rotation

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/shawnbutts/keystone-core/internal/credentials"
	"github.com/shawnbutts/keystone-core/pkg/statemachine"
)

// RotationEngine orchestrates credential rotation using a state machine workflow.
type RotationEngine struct {
	mu        sync.RWMutex
	providers []RotationProvider
	store     credentials.CredentialStore
	audit     credentials.AuditLogger
	jobs      map[string]*managedJob
}

type managedJob struct {
	job     *RotationJob
	machine *statemachine.Machine[CredRotationState, CredRotationEvent]
}

// NewRotationEngine creates a new rotation engine.
func NewRotationEngine(store credentials.CredentialStore, audit credentials.AuditLogger) *RotationEngine {
	return &RotationEngine{
		providers: make([]RotationProvider, 0),
		store:     store,
		audit:     audit,
		jobs:      make(map[string]*managedJob),
	}
}

// RegisterProvider registers a rotation provider.
func (e *RotationEngine) RegisterProvider(provider RotationProvider) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.providers = append(e.providers, provider)
}

// findProvider returns the first provider that supports the credential type.
func (e *RotationEngine) findProvider(credType credentials.CredentialType) RotationProvider {
	for _, p := range e.providers {
		if p.SupportsType(credType) {
			return p
		}
	}
	return nil
}

// Rotate executes a credential rotation job synchronously.
func (e *RotationEngine) Rotate(ctx context.Context, job *RotationJob) (*RotationResult, error) {
	e.mu.RLock()
	provider := e.findProvider(job.CredentialType)
	e.mu.RUnlock()

	if provider == nil {
		return nil, fmt.Errorf("no provider supports credential type %s", job.CredentialType)
	}

	machine := buildRotationMachine()
	mj := &managedJob{job: job, machine: machine}

	e.mu.Lock()
	if _, exists := e.jobs[job.ID]; exists {
		e.mu.Unlock()
		return nil, fmt.Errorf("rotation job %s already exists", job.ID)
	}
	e.jobs[job.ID] = mj
	e.mu.Unlock()

	defer func() {
		e.mu.Lock()
		delete(e.jobs, job.ID)
		e.mu.Unlock()
	}()

	job.State = CredRotationPending
	job.StartedAt = time.Now()

	result := e.executeRotation(ctx, mj, provider)
	return result, nil
}

// GetJob returns a rotation job by ID.
func (e *RotationEngine) GetJob(id string) (*RotationJob, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	mj, ok := e.jobs[id]
	if !ok {
		return nil, false
	}
	return mj.job, true
}

// ListJobs returns all active rotation jobs.
func (e *RotationEngine) ListJobs() []*RotationJob {
	e.mu.RLock()
	defer e.mu.RUnlock()
	jobs := make([]*RotationJob, 0, len(e.jobs))
	for _, mj := range e.jobs {
		jobs = append(jobs, mj.job)
	}
	return jobs
}

func buildRotationMachine() *statemachine.Machine[CredRotationState, CredRotationEvent] {
	builder := statemachine.New[CredRotationState, CredRotationEvent](CredRotationPending).
		WithHistory(50)

	// Happy path
	builder.AddTransition(CredRotationPending, CredRotationEventStart, CredRotationValidating)
	builder.AddTransition(CredRotationValidating, CredRotationEventValidated, CredRotationGenerating)
	builder.AddTransition(CredRotationGenerating, CredRotationEventGenerated, CredRotationApplying)
	builder.AddTransition(CredRotationApplying, CredRotationEventApplied, CredRotationVerifying)
	builder.AddTransition(CredRotationVerifying, CredRotationEventVerified, CredRotationStoring)
	builder.AddTransition(CredRotationStoring, CredRotationEventStored, CredRotationCleanup)
	builder.AddTransition(CredRotationCleanup, CredRotationEventCleanedUp, CredRotationCompleted)

	// Failure transitions from each active state
	builder.AddTransition(CredRotationValidating, CredRotationEventFailed, CredRotationFailed)
	builder.AddTransition(CredRotationGenerating, CredRotationEventFailed, CredRotationFailed)
	builder.AddTransition(CredRotationApplying, CredRotationEventFailed, CredRotationFailed)
	builder.AddTransition(CredRotationVerifying, CredRotationEventFailed, CredRotationFailed)
	builder.AddTransition(CredRotationStoring, CredRotationEventFailed, CredRotationFailed)
	builder.AddTransition(CredRotationCleanup, CredRotationEventFailed, CredRotationFailed)

	// Rollback path
	builder.AddTransition(CredRotationFailed, CredRotationEventRollback, CredRotationRollingBack)
	builder.AddTransition(CredRotationRollingBack, CredRotationEventRolledBack, CredRotationRolledBack)
	builder.AddTransition(CredRotationRollingBack, CredRotationEventFailed, CredRotationFailed)

	return builder.MustBuild()
}

func (e *RotationEngine) executeRotation(ctx context.Context, mj *managedJob, provider RotationProvider) *RotationResult {
	job := mj.job
	machine := mj.machine

	result := &RotationResult{
		JobID:          job.ID,
		CredentialID:   job.CredentialID,
		CredentialType: job.CredentialType,
		DeviceID:       job.DeviceID,
		StartedAt:      job.StartedAt,
	}

	// Start: Pending → Validating
	if err := machine.FireCtx(ctx, CredRotationEventStart); err != nil {
		return e.failResult(result, job, "failed to start rotation: "+err.Error())
	}
	job.State = CredRotationValidating
	e.logAudit(ctx, job, "rotation_start", true, "")

	// Validate old credential
	if err := provider.ValidateOld(ctx, job.Device, job.OldCredential); err != nil {
		return e.handleFailure(ctx, machine, result, job, provider, "validate_old", err)
	}
	if err := machine.FireCtx(ctx, CredRotationEventValidated); err != nil {
		return e.failResult(result, job, "state transition failed: "+err.Error())
	}
	job.State = CredRotationGenerating
	e.logAudit(ctx, job, "rotation_validated", true, "")

	// Generate new credential
	newCred, err := provider.Generate(ctx, job.Device, job.OldCredential)
	if err != nil {
		return e.handleFailure(ctx, machine, result, job, provider, "generate", err)
	}
	job.NewCredential = newCred
	if err := machine.FireCtx(ctx, CredRotationEventGenerated); err != nil {
		return e.failResult(result, job, "state transition failed: "+err.Error())
	}
	job.State = CredRotationApplying
	e.logAudit(ctx, job, "rotation_generated", true, "")

	// Apply new credential to device
	if err := provider.Apply(ctx, job.Device, job.OldCredential, newCred); err != nil {
		return e.handleFailure(ctx, machine, result, job, provider, "apply", err)
	}
	if err := machine.FireCtx(ctx, CredRotationEventApplied); err != nil {
		return e.failResult(result, job, "state transition failed: "+err.Error())
	}
	job.State = CredRotationVerifying
	e.logAudit(ctx, job, "rotation_applied", true, "")

	// Verify new credential works
	if err := provider.Verify(ctx, job.Device, newCred); err != nil {
		return e.handleFailure(ctx, machine, result, job, provider, "verify", err)
	}
	if err := machine.FireCtx(ctx, CredRotationEventVerified); err != nil {
		return e.failResult(result, job, "state transition failed: "+err.Error())
	}
	job.State = CredRotationStoring
	e.logAudit(ctx, job, "rotation_verified", true, "")

	// Store new credential
	if err := e.store.Store(ctx, newCred); err != nil {
		return e.handleFailure(ctx, machine, result, job, provider, "store", err)
	}
	if err := machine.FireCtx(ctx, CredRotationEventStored); err != nil {
		return e.failResult(result, job, "state transition failed: "+err.Error())
	}
	job.State = CredRotationCleanup
	e.logAudit(ctx, job, "rotation_stored", true, "")

	// Cleanup old credential artifacts
	if err := provider.Cleanup(ctx, job.Device, job.OldCredential); err != nil {
		return e.handleFailure(ctx, machine, result, job, provider, "cleanup", err)
	}
	if err := machine.FireCtx(ctx, CredRotationEventCleanedUp); err != nil {
		return e.failResult(result, job, "state transition failed: "+err.Error())
	}
	job.State = CredRotationCompleted
	job.CompletedAt = time.Now()
	e.logAudit(ctx, job, "rotation_completed", true, "")

	result.Success = true
	result.CompletedAt = job.CompletedAt
	result.Duration = job.CompletedAt.Sub(job.StartedAt)
	return result
}

func (e *RotationEngine) handleFailure(
	ctx context.Context,
	machine *statemachine.Machine[CredRotationState, CredRotationEvent],
	result *RotationResult,
	job *RotationJob,
	provider RotationProvider,
	stage string,
	err error,
) *RotationResult {
	errMsg := fmt.Sprintf("%s failed: %v", stage, err)
	e.logAudit(ctx, job, "rotation_"+stage+"_failed", false, errMsg)

	_ = machine.FireCtx(ctx, CredRotationEventFailed)
	job.State = CredRotationFailed
	job.Error = errMsg

	// Attempt rollback if configured and we have both credentials
	if job.RollbackOnFail && job.OldCredential != nil && job.NewCredential != nil {
		e.logAudit(ctx, job, "rotation_rollback_start", true, "")
		if rbErr := machine.FireCtx(ctx, CredRotationEventRollback); rbErr == nil {
			job.State = CredRotationRollingBack
			if rbErr := provider.Rollback(ctx, job.Device, job.OldCredential, job.NewCredential); rbErr == nil {
				_ = machine.FireCtx(ctx, CredRotationEventRolledBack)
				job.State = CredRotationRolledBack
				result.RolledBack = true
				e.logAudit(ctx, job, "rotation_rollback_success", true, "")
			} else {
				_ = machine.FireCtx(ctx, CredRotationEventFailed)
				job.State = CredRotationFailed
				errMsg = fmt.Sprintf("%s; rollback also failed: %v", errMsg, rbErr)
				e.logAudit(ctx, job, "rotation_rollback_failed", false, rbErr.Error())
			}
		}
	}

	return e.failResult(result, job, errMsg)
}

func (e *RotationEngine) failResult(result *RotationResult, job *RotationJob, errMsg string) *RotationResult {
	job.CompletedAt = time.Now()
	job.Error = errMsg
	result.Success = false
	result.Error = errMsg
	result.CompletedAt = job.CompletedAt
	result.Duration = job.CompletedAt.Sub(job.StartedAt)
	return result
}

func (e *RotationEngine) logAudit(ctx context.Context, job *RotationJob, action string, success bool, errMsg string) {
	if e.audit == nil {
		return
	}
	event := &credentials.CredentialAccessEvent{
		CredentialRef:  job.CredentialID,
		CredentialID:   job.CredentialID,
		CredentialType: job.CredentialType,
		ProxyAgentID:   job.ProxyAgentID,
		DeviceID:       job.DeviceID,
		RequestID:      job.ID,
		Action:         action,
		Timestamp:      time.Now(),
		Success:        success,
		Error:          errMsg,
	}
	_ = e.audit.LogCredentialAccess(ctx, event)
}
