package promotion

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Deployer defines the interface for deploying to environments
type Deployer interface {
	// Deploy deploys a revision to an environment
	Deploy(ctx context.Context, env *Environment, revision string) error

	// GetCurrentRevision returns the current deployed revision
	GetCurrentRevision(ctx context.Context, env *Environment) (string, error)

	// SetTrafficWeight sets traffic weight for canary deployments
	SetTrafficWeight(ctx context.Context, env *Environment, weight int) error
}

// Engine orchestrates promotion pipelines
type Engine struct {
	deployer  Deployer
	pipelines map[string]*Pipeline
	results   map[string]*PromotionResult
	mu        sync.RWMutex
	resultsMu sync.RWMutex
}

// NewEngine creates a new promotion engine
func NewEngine(deployer Deployer) *Engine {
	return &Engine{
		deployer:  deployer,
		pipelines: make(map[string]*Pipeline),
		results:   make(map[string]*PromotionResult),
	}
}

// RegisterPipeline registers a promotion pipeline
func (e *Engine) RegisterPipeline(pipeline *Pipeline) error {
	if pipeline.Name == "" {
		return fmt.Errorf("pipeline name is required")
	}
	if len(pipeline.Environments) == 0 {
		return fmt.Errorf("pipeline must have at least one environment")
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	e.pipelines[pipeline.Name] = pipeline
	return nil
}

// GetPipeline returns a pipeline by name
func (e *Engine) GetPipeline(name string) (*Pipeline, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	pipeline, ok := e.pipelines[name]
	return pipeline, ok
}

// Promote executes a promotion
func (e *Engine) Promote(ctx context.Context, req *PromotionRequest) (*PromotionResult, error) {
	// Get pipeline
	pipeline, ok := e.GetPipeline(req.Pipeline)
	if !ok {
		return nil, fmt.Errorf("pipeline not found: %s", req.Pipeline)
	}

	// Find target environment
	var targetEnv *Environment
	var targetIdx int
	for i, env := range pipeline.Environments {
		if env.Name == req.ToEnvironment {
			targetEnv = env
			targetIdx = i
			break
		}
	}
	if targetEnv == nil {
		return nil, fmt.Errorf("environment not found: %s", req.ToEnvironment)
	}

	// Create result
	result := &PromotionResult{
		ID:       uuid.New().String(),
		Pipeline: pipeline,
		Request:  req,
		Status:   StatusPending,
		Stages:   make([]*StageResult, 0),
	}

	// Store result
	e.resultsMu.Lock()
	e.results[result.ID] = result
	e.resultsMu.Unlock()

	// Check if approval is required
	if targetEnv.RequireApproval && !req.Force {
		result.ApprovalInfo = &ApprovalInfo{
			Required: true,
			Status:   StatusPending,
		}
		return result, nil
	}

	// Execute promotion
	return e.executePromotion(ctx, result, pipeline, targetIdx)
}

// executePromotion executes the actual promotion
func (e *Engine) executePromotion(ctx context.Context, result *PromotionResult, pipeline *Pipeline, targetIdx int) (*PromotionResult, error) {
	result.Status = StatusInProgress
	result.StartTime = time.Now()

	// Set timeout
	if pipeline.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, pipeline.Timeout)
		defer cancel()
	}

	// Promote to target environment
	targetEnv := pipeline.Environments[targetIdx]
	stageResult, err := e.promoteToEnvironment(ctx, result, pipeline, targetEnv)
	result.Stages = append(result.Stages, stageResult)

	if err != nil {
		result.Status = StatusFailed
		result.Error = err
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime)
		result.Message = fmt.Sprintf("Promotion failed: %v", err)

		// Rollback if configured
		if pipeline.RollbackOnFailure {
			result.Status = StatusRollingBack
			// Rollback logic would go here
			result.Status = StatusRolledBack
		}

		return result, err
	}

	result.Status = StatusCompleted
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	result.Message = fmt.Sprintf("Successfully promoted to %s", targetEnv.Name)

	return result, nil
}

// promoteToEnvironment promotes to a single environment
func (e *Engine) promoteToEnvironment(ctx context.Context, result *PromotionResult, pipeline *Pipeline, env *Environment) (*StageResult, error) {
	stage := &StageResult{
		Environment: env.Name,
		Status:      StatusInProgress,
		StartTime:   time.Now(),
	}

	// Run verification if configured
	if env.VerificationWorkflow != "" && !result.Request.SkipVerification {
		stage.Status = StatusVerifying
		// Verification would happen here
		// For now, we skip actual verification
	}

	// Deploy based on strategy
	var err error
	switch pipeline.Strategy {
	case StrategyImmediate:
		err = e.deployImmediate(ctx, env, result.Request.Revision)
	case StrategyCanary:
		err = e.deployCanary(ctx, env, result.Request.Revision, pipeline.CanarySteps, stage)
	case StrategyBlueGreen:
		err = e.deployBlueGreen(ctx, env, result.Request.Revision)
	case StrategyRolling:
		err = e.deployRolling(ctx, env, result.Request.Revision)
	default:
		err = fmt.Errorf("unknown strategy: %s", pipeline.Strategy)
	}

	if err != nil {
		stage.Status = StatusFailed
		stage.Error = err
		stage.EndTime = time.Now()
		stage.Duration = stage.EndTime.Sub(stage.StartTime)
		stage.Message = fmt.Sprintf("Deployment failed: %v", err)
		return stage, err
	}

	stage.Status = StatusCompleted
	stage.EndTime = time.Now()
	stage.Duration = stage.EndTime.Sub(stage.StartTime)
	stage.Message = fmt.Sprintf("Deployed to %s", env.Name)

	return stage, nil
}

// deployImmediate performs immediate deployment
func (e *Engine) deployImmediate(ctx context.Context, env *Environment, revision string) error {
	return e.deployer.Deploy(ctx, env, revision)
}

// deployCanary performs canary deployment with gradual rollout
func (e *Engine) deployCanary(ctx context.Context, env *Environment, revision string, steps []CanaryStep, stage *StageResult) error {
	stage.Status = StatusRollingOut
	stage.CanaryProgress = make([]CanaryProgress, 0, len(steps))

	// Deploy canary version
	err := e.deployer.Deploy(ctx, env, revision)
	if err != nil {
		return fmt.Errorf("failed to deploy canary: %w", err)
	}

	// Gradually increase traffic
	for i, step := range steps {
		progress := CanaryProgress{
			Step:      i + 1,
			Weight:    step.Weight,
			StartTime: time.Now(),
			Status:    "in_progress",
		}

		// Set traffic weight
		err := e.deployer.SetTrafficWeight(ctx, env, step.Weight)
		if err != nil {
			progress.Status = "failed"
			progress.EndTime = time.Now()
			stage.CanaryProgress = append(stage.CanaryProgress, progress)
			return fmt.Errorf("failed to set traffic weight: %w", err)
		}

		// Wait for duration
		select {
		case <-time.After(step.Duration):
		case <-ctx.Done():
			progress.Status = "cancelled"
			progress.EndTime = time.Now()
			stage.CanaryProgress = append(stage.CanaryProgress, progress)
			return ctx.Err()
		}

		// Verification would happen here
		// For now, assume success

		progress.Status = "completed"
		progress.EndTime = time.Now()
		stage.CanaryProgress = append(stage.CanaryProgress, progress)
	}

	// Final rollout to 100%
	return e.deployer.SetTrafficWeight(ctx, env, 100)
}

// deployBlueGreen performs blue/green deployment
func (e *Engine) deployBlueGreen(ctx context.Context, env *Environment, revision string) error {
	stage := StatusRollingOut

	// Deploy to green environment
	err := e.deployer.Deploy(ctx, env, revision)
	if err != nil {
		return fmt.Errorf("failed to deploy green: %w", err)
	}

	// Switch traffic to green (100%)
	err = e.deployer.SetTrafficWeight(ctx, env, 100)
	if err != nil {
		return fmt.Errorf("failed to switch traffic: %w", err)
	}

	_ = stage // Mark as used
	return nil
}

// deployRolling performs rolling update
func (e *Engine) deployRolling(ctx context.Context, env *Environment, revision string) error {
	// Rolling deployment is typically handled by the underlying platform (K8s)
	// We just trigger the deployment
	return e.deployer.Deploy(ctx, env, revision)
}

// ApprovePromotion approves a pending promotion
func (e *Engine) ApprovePromotion(ctx context.Context, req *ApprovalRequest) error {
	e.resultsMu.RLock()
	result, ok := e.results[req.PromotionID]
	e.resultsMu.RUnlock()

	if !ok {
		return fmt.Errorf("promotion not found: %s", req.PromotionID)
	}

	if result.ApprovalInfo == nil {
		return fmt.Errorf("promotion does not require approval")
	}

	if result.Status != StatusPending {
		return fmt.Errorf("promotion is not in pending state: %s", result.Status)
	}

	// Update approval info
	if req.Approved {
		result.ApprovalInfo.Status = StatusApproved
		result.ApprovalInfo.ApprovedBy = req.ApprovedBy
		result.ApprovalInfo.ApprovedAt = time.Now()
		result.ApprovalInfo.Reason = req.Reason
		result.Status = StatusApproved

		// Find target environment
		var targetIdx int
		for i, env := range result.Pipeline.Environments {
			if env.Name == result.Request.ToEnvironment {
				targetIdx = i
				break
			}
		}

		// Execute the promotion
		_, err := e.executePromotion(ctx, result, result.Pipeline, targetIdx)
		return err
	} else {
		result.ApprovalInfo.Status = StatusRejected
		result.ApprovalInfo.RejectedBy = req.ApprovedBy
		result.ApprovalInfo.RejectedAt = time.Now()
		result.ApprovalInfo.Reason = req.Reason
		result.Status = StatusRejected
		result.EndTime = time.Now()
		result.Message = fmt.Sprintf("Promotion rejected by %s: %s", req.ApprovedBy, req.Reason)
	}

	return nil
}

// GetPromotion returns a promotion by ID
func (e *Engine) GetPromotion(id string) (*PromotionResult, bool) {
	e.resultsMu.RLock()
	defer e.resultsMu.RUnlock()
	result, ok := e.results[id]
	return result, ok
}

// ListPromotions returns all promotions
func (e *Engine) ListPromotions() []*PromotionResult {
	e.resultsMu.RLock()
	defer e.resultsMu.RUnlock()

	results := make([]*PromotionResult, 0, len(e.results))
	for _, result := range e.results {
		results = append(results, result)
	}
	return results
}

// ListPendingPromotions returns promotions pending approval
func (e *Engine) ListPendingPromotions() []*PromotionResult {
	e.resultsMu.RLock()
	defer e.resultsMu.RUnlock()

	var results []*PromotionResult
	for _, result := range e.results {
		if result.Status == StatusPending {
			results = append(results, result)
		}
	}
	return results
}
