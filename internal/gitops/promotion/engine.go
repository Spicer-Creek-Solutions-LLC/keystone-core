package promotion

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/shawnbutts/keystone-core/pkg/wait"
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

// Remediator defines the interface for executing remediation actions
type Remediator interface {
	// Rollback rolls back to a previous revision
	Rollback(ctx context.Context, env *Environment, toRevision string) error

	// ScaleDown scales down a deployment
	ScaleDown(ctx context.Context, env *Environment, replicas int) error

	// ShiftTraffic shifts traffic away from the failed deployment
	ShiftTraffic(ctx context.Context, env *Environment, weight int) error

	// ExecuteWorkflow runs a custom remediation workflow
	ExecuteWorkflow(ctx context.Context, env *Environment, workflow string, params map[string]string) error

	// CollectDiagnostics gathers diagnostic information
	CollectDiagnostics(ctx context.Context, env *Environment) (*DiagnosticInfo, error)
}

// Notifier defines the interface for sending notifications
type Notifier interface {
	// Notify sends a notification
	Notify(ctx context.Context, channel string, message string, details map[string]interface{}) error
}

// Engine orchestrates promotion pipelines
type Engine struct {
	deployer           Deployer
	remediator         Remediator
	notifier           Notifier
	pipelines          map[string]*Pipeline
	results            map[string]*Result
	thresholdRegistry  *ThresholdRegistry
	thresholdEvaluator *ThresholdEvaluator
	mu                 sync.RWMutex
	resultsMu          sync.RWMutex
}

// EngineOption configures the promotion engine
type EngineOption func(*Engine)

// WithMetricsProvider sets a metrics provider for threshold evaluation
func WithMetricsProvider(provider MetricsProvider) EngineOption {
	return func(e *Engine) {
		e.thresholdEvaluator = NewThresholdEvaluator(provider, e.thresholdRegistry)
	}
}

// WithThresholdRegistry sets a custom threshold registry
func WithThresholdRegistry(registry *ThresholdRegistry) EngineOption {
	return func(e *Engine) {
		e.thresholdRegistry = registry
	}
}

// WithRemediator sets a remediator for automatic remediation
func WithRemediator(remediator Remediator) EngineOption {
	return func(e *Engine) {
		e.remediator = remediator
	}
}

// WithNotifier sets a notifier for remediation notifications
func WithNotifier(notifier Notifier) EngineOption {
	return func(e *Engine) {
		e.notifier = notifier
	}
}

// NewEngine creates a new promotion engine
func NewEngine(deployer Deployer, opts ...EngineOption) *Engine {
	e := &Engine{
		deployer:          deployer,
		pipelines:         make(map[string]*Pipeline),
		results:           make(map[string]*Result),
		thresholdRegistry: NewThresholdRegistry(),
	}

	for _, opt := range opts {
		opt(e)
	}

	return e
}

// GetThresholdRegistry returns the threshold registry for configuration
func (e *Engine) GetThresholdRegistry() *ThresholdRegistry {
	return e.thresholdRegistry
}

// getEffectiveThresholds returns the effective threshold config for a pipeline/environment
func (e *Engine) getEffectiveThresholds(pipeline *Pipeline, env *Environment) *ThresholdConfig {
	// Priority: Environment > Pipeline > Registry defaults

	// Check environment-level preset
	if env != nil && env.ThresholdPreset != "" {
		if preset, ok := GetPreset(env.ThresholdPreset); ok {
			if env.InheritThresholds && pipeline.Thresholds != nil {
				return mergeThresholds(pipeline.Thresholds, preset)
			}
			return preset
		}
	}

	// Check environment-level custom config
	if env != nil && env.Thresholds != nil {
		if env.InheritThresholds && pipeline.Thresholds != nil {
			return mergeThresholds(pipeline.Thresholds, env.Thresholds)
		}
		return env.Thresholds
	}

	// Check pipeline-level preset
	if pipeline.ThresholdPreset != "" {
		if preset, ok := GetPreset(pipeline.ThresholdPreset); ok {
			return preset
		}
	}

	// Check pipeline-level custom config
	if pipeline.Thresholds != nil {
		return pipeline.Thresholds
	}

	// Fall back to registry defaults
	return e.thresholdRegistry.GetThresholds(env.Name, pipeline.Strategy)
}

// evaluateThresholds evaluates thresholds for a deployment stage
func (e *Engine) evaluateThresholds(ctx context.Context, pipeline *Pipeline, env *Environment, thresholdOverride *ThresholdConfig) (*EvaluationResult, error) {
	// No evaluator configured, skip evaluation
	if e.thresholdEvaluator == nil {
		return &EvaluationResult{
			Passed:      true,
			EvaluatedAt: time.Now(),
			Message:     "Threshold evaluation skipped (no metrics provider configured)",
		}, nil
	}

	// Use override if provided (e.g., canary step thresholds)
	config := thresholdOverride
	if config == nil {
		config = e.getEffectiveThresholds(pipeline, env)
	}

	if config == nil {
		return &EvaluationResult{
			Passed:      true,
			EvaluatedAt: time.Now(),
			Message:     "No thresholds configured",
		}, nil
	}

	// Build labels for metric queries
	labels := map[string]string{
		"pipeline":    pipeline.Name,
		"application": pipeline.Application,
		"environment": env.Name,
	}
	if env.Namespace != "" {
		labels["namespace"] = env.Namespace
	}
	if env.Cluster != "" {
		labels["cluster"] = env.Cluster
	}

	return e.thresholdEvaluator.Evaluate(ctx, env.Name, pipeline.Strategy, labels)
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
func (e *Engine) Promote(ctx context.Context, req *Request) (*Result, error) {
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
	result := &Result{
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
func (e *Engine) executePromotion(ctx context.Context, result *Result, pipeline *Pipeline, targetIdx int) (*Result, error) {
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
		result.Error = err
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime)
		result.Message = fmt.Sprintf("Promotion failed: %v", err)

		// Check if remediation was attempted and its status
		if stageResult.RemediationResult != nil {
			switch stageResult.RemediationResult.Status {
			case RemediationStatusSucceeded:
				result.Status = StatusRolledBack
				result.Message = fmt.Sprintf("Promotion failed but automatic remediation succeeded: %s", stageResult.RemediationResult.Message)
			case RemediationStatusPending:
				result.Status = StatusRollingBack
				result.Message = fmt.Sprintf("Promotion failed, awaiting manual remediation: %s", stageResult.RemediationResult.Message)
			case RemediationStatusSkipped:
				result.Status = StatusFailed
			default:
				result.Status = StatusFailed
			}
		} else {
			result.Status = StatusFailed
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
func (e *Engine) promoteToEnvironment(ctx context.Context, result *Result, pipeline *Pipeline, env *Environment) (*StageResult, error) {
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
		err = e.deployCanary(ctx, env, result.Request.Revision, pipeline.CanarySteps, stage, pipeline)
	case StrategyBlueGreen:
		err = e.deployBlueGreen(ctx, env, result.Request.Revision, pipeline, stage)
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
func (e *Engine) deployCanary(ctx context.Context, env *Environment, revision string, steps []CanaryStep, stage *StageResult, pipeline *Pipeline) error {
	stage.Status = StatusRollingOut
	stage.CanaryProgress = make([]CanaryProgress, 0, len(steps))

	// Get previous revision for potential rollback
	previousRevision, _ := e.deployer.GetCurrentRevision(ctx, env)

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
			Metrics:   make(map[string]float64),
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
		if err := wait.ForContext(ctx, step.Duration); err != nil {
			progress.Status = "cancelled"
			progress.EndTime = time.Now()
			stage.CanaryProgress = append(stage.CanaryProgress, progress)
			return err
		}

		// Evaluate thresholds if not skipped
		if !step.SkipVerification {
			// Use step-specific thresholds if provided, otherwise use pipeline/env thresholds
			evalResult, evalErr := e.evaluateThresholds(ctx, pipeline, env, step.Thresholds)
			if evalErr != nil {
				progress.Status = "verification_error"
				progress.EndTime = time.Now()
				stage.CanaryProgress = append(stage.CanaryProgress, progress)
				return fmt.Errorf("failed to evaluate thresholds at step %d: %w", i+1, evalErr)
			}

			// Store metrics from evaluation
			for _, tr := range evalResult.ThresholdResults {
				progress.Metrics[string(tr.Threshold.Metric)] = tr.ActualValue
			}

			if !evalResult.Passed {
				progress.Status = "threshold_failed"
				progress.EndTime = time.Now()
				stage.CanaryProgress = append(stage.CanaryProgress, progress)

				// Handle verification failure with automatic remediation
				return e.handleVerificationFailure(ctx, pipeline, env, evalResult, i+1, step.Weight, previousRevision, stage)
			}
		}

		progress.Status = "completed"
		progress.EndTime = time.Now()
		stage.CanaryProgress = append(stage.CanaryProgress, progress)
	}

	// Final rollout to 100%
	return e.deployer.SetTrafficWeight(ctx, env, 100)
}

// deployBlueGreen performs blue/green deployment
func (e *Engine) deployBlueGreen(ctx context.Context, env *Environment, revision string, pipeline *Pipeline, stage *StageResult) error {
	// Get previous revision for potential rollback
	previousRevision, _ := e.deployer.GetCurrentRevision(ctx, env)

	// Deploy to green environment
	err := e.deployer.Deploy(ctx, env, revision)
	if err != nil {
		return fmt.Errorf("failed to deploy green: %w", err)
	}

	// Get threshold config to determine warmup period
	thresholdConfig := e.getEffectiveThresholds(pipeline, env)
	if thresholdConfig != nil && thresholdConfig.WarmupPeriod > 0 {
		// Wait for warmup period before verification
		if err := wait.ForContext(ctx, thresholdConfig.WarmupPeriod); err != nil {
			return err
		}

		// Verify thresholds before switching traffic
		evalResult, evalErr := e.evaluateThresholds(ctx, pipeline, env, nil)
		if evalErr != nil {
			return fmt.Errorf("failed to evaluate thresholds: %w", evalErr)
		}

		if !evalResult.Passed {
			// Handle verification failure with automatic remediation
			return e.handleVerificationFailure(ctx, pipeline, env, evalResult, 0, 0, previousRevision, stage)
		}
	}

	// Switch traffic to green (100%)
	err = e.deployer.SetTrafficWeight(ctx, env, 100)
	if err != nil {
		return fmt.Errorf("failed to switch traffic: %w", err)
	}

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
	}

	result.ApprovalInfo.Status = StatusRejected
	result.ApprovalInfo.RejectedBy = req.ApprovedBy
	result.ApprovalInfo.RejectedAt = time.Now()
	result.ApprovalInfo.Reason = req.Reason
	result.Status = StatusRejected
	result.EndTime = time.Now()
	result.Message = fmt.Sprintf("Promotion rejected by %s: %s", req.ApprovedBy, req.Reason)

	return nil
}

// GetPromotion returns a promotion by ID
func (e *Engine) GetPromotion(id string) (*Result, bool) {
	e.resultsMu.RLock()
	defer e.resultsMu.RUnlock()
	result, ok := e.results[id]
	return result, ok
}

// ListPromotions returns all promotions
func (e *Engine) ListPromotions() []*Result {
	e.resultsMu.RLock()
	defer e.resultsMu.RUnlock()

	results := make([]*Result, 0, len(e.results))
	for _, result := range e.results {
		results = append(results, result)
	}
	return results
}

// ListPendingPromotions returns promotions pending approval
func (e *Engine) ListPendingPromotions() []*Result {
	e.resultsMu.RLock()
	defer e.resultsMu.RUnlock()

	var results []*Result
	for _, result := range e.results {
		if result.Status == StatusPending {
			results = append(results, result)
		}
	}
	return results
}

// getEffectiveRemediation returns the effective remediation config for a pipeline/environment
func (e *Engine) getEffectiveRemediation(pipeline *Pipeline, env *Environment) *RemediationConfig {
	// Priority: Environment > Pipeline > Default (if RollbackOnFailure is true)

	// Check environment-level config
	if env != nil && env.Remediation != nil {
		return env.Remediation
	}

	// Check pipeline-level config
	if pipeline.Remediation != nil {
		return pipeline.Remediation
	}

	// Fall back to default if RollbackOnFailure is set (backwards compatibility)
	if pipeline.RollbackOnFailure {
		return DefaultRemediationConfig()
	}

	return nil
}

// executeRemediation performs automatic remediation based on configuration
func (e *Engine) executeRemediation(ctx context.Context, pipeline *Pipeline, env *Environment, trigger RemediationTrigger, previousRevision string, stage *StageResult) *RemediationResult {
	config := e.getEffectiveRemediation(pipeline, env)
	if config == nil || !config.Enabled {
		return &RemediationResult{
			ID:        uuid.New().String(),
			Trigger:   trigger,
			Status:    RemediationStatusSkipped,
			StartTime: time.Now(),
			EndTime:   time.Now(),
			Message:   "Automatic remediation is disabled",
		}
	}

	result := &RemediationResult{
		ID:               uuid.New().String(),
		Trigger:          trigger,
		Strategy:         config.Strategy,
		Status:           RemediationStatusInProgress,
		PreviousRevision: stage.Environment,
		TargetRevision:   previousRevision,
		StartTime:        time.Now(),
		AttemptDetails:   make([]RemediationAttempt, 0),
	}

	// Collect diagnostics if enabled
	if config.CollectDiagnostics && e.remediator != nil {
		diag, err := e.remediator.CollectDiagnostics(ctx, env)
		if err == nil {
			result.Diagnostics = diag
		}
	}

	// Send notification if enabled
	if config.NotifyOnRemediation && e.notifier != nil {
		for _, channel := range config.NotificationChannels {
			_ = e.notifier.Notify(ctx, channel, fmt.Sprintf("Automatic remediation triggered for %s/%s", pipeline.Name, env.Name), map[string]interface{}{
				"trigger":     trigger.Type,
				"reason":      trigger.Reason,
				"strategy":    config.Strategy,
				"pipeline":    pipeline.Name,
				"environment": env.Name,
			})
		}
	}

	maxAttempts := config.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 1
	}

	// Execute remediation with retries
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		attemptResult := RemediationAttempt{
			Attempt:   attempt,
			StartTime: time.Now(),
			Status:    RemediationStatusInProgress,
			Actions:   make([]string, 0),
		}

		// Create timeout context for this attempt
		attemptCtx := ctx
		var cancel context.CancelFunc
		if config.TimeoutPerAttempt > 0 {
			attemptCtx, cancel = context.WithTimeout(ctx, config.TimeoutPerAttempt)
		}

		// Execute based on strategy
		err := e.executeRemediationStrategy(attemptCtx, config.Strategy, pipeline, env, previousRevision, &attemptResult)
		if cancel != nil {
			cancel()
		}

		attemptResult.EndTime = time.Now()

		if err != nil {
			attemptResult.Status = RemediationStatusFailed
			attemptResult.Error = err.Error()
			lastErr = err
		} else {
			attemptResult.Status = RemediationStatusSucceeded
			result.Attempts = attempt
			result.AttemptDetails = append(result.AttemptDetails, attemptResult)
			result.Status = RemediationStatusSucceeded
			result.EndTime = time.Now()
			result.Duration = result.EndTime.Sub(result.StartTime)
			result.Message = fmt.Sprintf("Remediation succeeded on attempt %d using %s strategy", attempt, config.Strategy)
			return result
		}

		result.AttemptDetails = append(result.AttemptDetails, attemptResult)

		// Wait before retry (unless last attempt)
		if attempt < maxAttempts && config.RetryDelay > 0 {
			if err := wait.ForContext(ctx, config.RetryDelay); err != nil {
				result.Status = RemediationStatusFailed
				result.EndTime = time.Now()
				result.Duration = result.EndTime.Sub(result.StartTime)
				result.Error = err
				result.Message = "Remediation cancelled"
				return result
			}
		}
	}

	// All attempts failed
	result.Attempts = maxAttempts
	result.Status = RemediationStatusFailed
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	result.Error = lastErr
	result.Message = fmt.Sprintf("Remediation failed after %d attempts: %v", maxAttempts, lastErr)

	return result
}

// executeRemediationStrategy executes a specific remediation strategy
func (e *Engine) executeRemediationStrategy(ctx context.Context, strategy RemediationStrategy, pipeline *Pipeline, env *Environment, targetRevision string, attempt *RemediationAttempt) error {
	if e.remediator == nil {
		return fmt.Errorf("no remediator configured")
	}

	switch strategy {
	case RemediationRollback:
		attempt.Actions = append(attempt.Actions, fmt.Sprintf("Rolling back to revision %s", targetRevision))
		return e.remediator.Rollback(ctx, env, targetRevision)

	case RemediationScaleDown:
		attempt.Actions = append(attempt.Actions, "Scaling down deployment to 0 replicas")
		return e.remediator.ScaleDown(ctx, env, 0)

	case RemediationTrafficShift:
		attempt.Actions = append(attempt.Actions, "Shifting all traffic away from failed deployment")
		return e.remediator.ShiftTraffic(ctx, env, 0)

	case RemediationCustom:
		config := e.getEffectiveRemediation(pipeline, env)
		if config == nil || config.CustomWorkflow == "" {
			return fmt.Errorf("custom remediation strategy requires a workflow name")
		}
		attempt.Actions = append(attempt.Actions, fmt.Sprintf("Executing custom workflow: %s", config.CustomWorkflow))
		return e.remediator.ExecuteWorkflow(ctx, env, config.CustomWorkflow, map[string]string{
			"pipeline":    pipeline.Name,
			"environment": env.Name,
			"application": pipeline.Application,
		})

	default:
		return fmt.Errorf("unknown remediation strategy: %s", strategy)
	}
}

// handleVerificationFailure handles a verification failure and triggers remediation if configured
func (e *Engine) handleVerificationFailure(ctx context.Context, pipeline *Pipeline, env *Environment, evalResult *EvaluationResult, failedStep, failedWeight int, previousRevision string, stage *StageResult) error {
	// Determine action based on failure policy
	thresholdConfig := e.getEffectiveThresholds(pipeline, env)
	failurePolicy := FailurePolicyAbort
	if thresholdConfig != nil {
		failurePolicy = thresholdConfig.FailurePolicy
	}

	trigger := RemediationTrigger{
		Type:             "threshold_failure",
		EvaluationResult: evalResult,
		FailedStep:       failedStep,
		FailedWeight:     failedWeight,
		Reason:           evalResult.Message,
	}

	switch failurePolicy {
	case FailurePolicyRollback:
		// Execute automatic rollback remediation
		remResult := e.executeRemediation(ctx, pipeline, env, trigger, previousRevision, stage)
		stage.RemediationResult = remResult

		if remResult.Status == RemediationStatusSucceeded {
			return fmt.Errorf("threshold verification failed, automatic rollback completed: %s", evalResult.Message)
		}
		return fmt.Errorf("threshold verification failed and remediation failed: %s (remediation error: %w)", evalResult.Message, remResult.Error)

	case FailurePolicyPause:
		// Pause and wait for manual intervention
		stage.RemediationResult = &RemediationResult{
			ID:        uuid.New().String(),
			Trigger:   trigger,
			Status:    RemediationStatusPending,
			StartTime: time.Now(),
			Message:   "Deployment paused due to threshold failure, awaiting manual intervention",
		}
		return fmt.Errorf("threshold verification failed, deployment paused: %s", evalResult.Message)

	case FailurePolicyIgnore:
		// Log warning but continue
		stage.RemediationResult = &RemediationResult{
			ID:        uuid.New().String(),
			Trigger:   trigger,
			Status:    RemediationStatusSkipped,
			StartTime: time.Now(),
			EndTime:   time.Now(),
			Message:   "Threshold failure ignored per policy",
		}
		return nil // No error, continue deployment

	default: // includes FailurePolicyAbort
		// Abort without remediation
		return fmt.Errorf("threshold verification failed: %s", evalResult.Message)
	}
}
