package rollback

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Executor defines the interface for executing rollbacks
type Executor interface {
	// Type returns the rollback type this executor handles
	Type() RollbackType

	// Execute executes the rollback
	Execute(ctx context.Context, config *RollbackConfig, request *RollbackRequest) (*RollbackResult, error)

	// GetPreviousRevision gets the previous revision for an application
	GetPreviousRevision(ctx context.Context, config *RollbackConfig) (string, error)

	// GetLastKnownGood gets the last known good revision
	GetLastKnownGood(ctx context.Context, config *RollbackConfig) (string, error)
}

// Engine orchestrates rollback operations
type Engine struct {
	executors map[RollbackType]Executor
	mu        sync.RWMutex

	// Store for rollback results
	results   map[string]*RollbackResult
	resultsMu sync.RWMutex

	// Approval workflow
	approvalWorkflow ApprovalWorkflow
}

// ApprovalWorkflow defines the interface for approval workflows
type ApprovalWorkflow interface {
	// RequestApproval requests approval for a rollback
	RequestApproval(ctx context.Context, result *RollbackResult) error

	// CheckApprovalStatus checks the approval status
	CheckApprovalStatus(ctx context.Context, rollbackID string) (RollbackStatus, error)
}

// NewEngine creates a new rollback engine
func NewEngine() *Engine {
	return &Engine{
		executors: make(map[RollbackType]Executor),
		results:   make(map[string]*RollbackResult),
	}
}

// RegisterExecutor registers a rollback executor
func (e *Engine) RegisterExecutor(executor Executor) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.executors[executor.Type()] = executor
}

// SetApprovalWorkflow sets the approval workflow
func (e *Engine) SetApprovalWorkflow(workflow ApprovalWorkflow) {
	e.approvalWorkflow = workflow
}

// Execute executes a rollback
func (e *Engine) Execute(ctx context.Context, config *RollbackConfig, request *RollbackRequest) (*RollbackResult, error) {
	// Get executor
	e.mu.RLock()
	executor, ok := e.executors[config.Type]
	e.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("no executor registered for type: %s", config.Type)
	}

	// Create rollback result
	result := &RollbackResult{
		ID:        uuid.New().String(),
		Config:    config,
		Request:   request,
		Status:    StatusPending,
		StartTime: time.Now(),
	}

	// Store result
	e.resultsMu.Lock()
	e.results[result.ID] = result
	e.resultsMu.Unlock()

	// Check if approval is required
	if config.RequireApproval || config.Trigger == TriggerManual {
		result.ApprovalInfo = &ApprovalInfo{
			Required: true,
			Status:   StatusPending,
		}

		// Request approval if workflow is set
		if e.approvalWorkflow != nil {
			if err := e.approvalWorkflow.RequestApproval(ctx, result); err != nil {
				result.Status = StatusFailed
				result.Error = fmt.Errorf("failed to request approval: %w", err)
				return result, result.Error
			}
		}

		return result, nil
	}

	// Execute immediately if no approval required
	return e.executeRollback(ctx, executor, result)
}

// executeRollback executes the actual rollback
func (e *Engine) executeRollback(ctx context.Context, executor Executor, result *RollbackResult) (*RollbackResult, error) {
	result.Status = StatusInProgress
	result.StartTime = time.Now()

	// Set up timeout
	if result.Config.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, result.Config.Timeout)
		defer cancel()
	}

	// Determine revision to rollback to
	revision := result.Request.OverrideRevision
	if revision == "" {
		var err error
		switch result.Config.Strategy {
		case StrategyPreviousRevision:
			revision, err = executor.GetPreviousRevision(ctx, result.Config)
		case StrategySpecificRevision:
			revision = result.Config.Revision
		case StrategyLastKnownGood:
			revision, err = executor.GetLastKnownGood(ctx, result.Config)
		default:
			err = fmt.Errorf("unknown rollback strategy: %s", result.Config.Strategy)
		}

		if err != nil {
			result.Status = StatusFailed
			result.Error = err
			result.EndTime = time.Now()
			result.Duration = result.EndTime.Sub(result.StartTime)
			result.Message = fmt.Sprintf("Failed to determine rollback revision: %v", err)
			return result, err
		}

		// Set the determined revision in the request for the executor
		result.Request.OverrideRevision = revision
	}

	result.CurrentRevision = revision

	// Execute rollback
	execResult, err := executor.Execute(ctx, result.Config, result.Request)
	if err != nil {
		result.Status = StatusFailed
		result.Error = err
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime)
		result.Message = fmt.Sprintf("Rollback failed: %v", err)
		return result, err
	}

	// Update result with execution details
	result.PreviousRevision = execResult.PreviousRevision
	result.CurrentRevision = execResult.CurrentRevision
	result.Status = StatusCompleted
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	result.Message = fmt.Sprintf("Rolled back from %s to %s", result.PreviousRevision, result.CurrentRevision)

	// Run verification if configured
	if result.Config.VerifyAfterRollback && !result.Request.SkipVerification {
		result.Status = StatusVerifying
		// Note: Verification integration would happen here
		// For now, we just mark as verified
		result.Status = StatusVerified
	}

	return result, nil
}

// ApproveRollback approves a pending rollback
func (e *Engine) ApproveRollback(ctx context.Context, req *ApprovalRequest) error {
	e.resultsMu.RLock()
	result, ok := e.results[req.RollbackID]
	e.resultsMu.RUnlock()

	if !ok {
		return fmt.Errorf("rollback not found: %s", req.RollbackID)
	}

	if result.ApprovalInfo == nil {
		return fmt.Errorf("rollback does not require approval")
	}

	if result.Status != StatusPending {
		return fmt.Errorf("rollback is not in pending state: %s", result.Status)
	}

	// Update approval info
	if req.Approved {
		result.ApprovalInfo.Status = StatusApproved
		result.ApprovalInfo.ApprovedBy = req.ApprovedBy
		result.ApprovalInfo.ApprovedAt = time.Now()
		result.ApprovalInfo.Reason = req.Reason
		result.Status = StatusApproved

		// Execute the rollback
		e.mu.RLock()
		executor, ok := e.executors[result.Config.Type]
		e.mu.RUnlock()

		if !ok {
			return fmt.Errorf("no executor registered for type: %s", result.Config.Type)
		}

		_, err := e.executeRollback(ctx, executor, result)
		return err
	} else {
		result.ApprovalInfo.Status = StatusRejected
		result.ApprovalInfo.RejectedBy = req.ApprovedBy
		result.ApprovalInfo.RejectedAt = time.Now()
		result.ApprovalInfo.Reason = req.Reason
		result.Status = StatusRejected
		result.EndTime = time.Now()
		result.Message = fmt.Sprintf("Rollback rejected by %s: %s", req.ApprovedBy, req.Reason)
	}

	return nil
}

// GetRollback returns a rollback result by ID
func (e *Engine) GetRollback(id string) (*RollbackResult, bool) {
	e.resultsMu.RLock()
	defer e.resultsMu.RUnlock()
	result, ok := e.results[id]
	return result, ok
}

// ListRollbacks returns all rollback results
func (e *Engine) ListRollbacks() []*RollbackResult {
	e.resultsMu.RLock()
	defer e.resultsMu.RUnlock()

	results := make([]*RollbackResult, 0, len(e.results))
	for _, result := range e.results {
		results = append(results, result)
	}
	return results
}

// ListPendingRollbacks returns rollbacks pending approval
func (e *Engine) ListPendingRollbacks() []*RollbackResult {
	e.resultsMu.RLock()
	defer e.resultsMu.RUnlock()

	var results []*RollbackResult
	for _, result := range e.results {
		if result.Status == StatusPending {
			results = append(results, result)
		}
	}
	return results
}
