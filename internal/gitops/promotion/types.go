// Package promotion provides deployment promotion pipelines for managing
// staged rollouts across environments with configurable strategies.
package promotion

import "time"

// Strategy defines the deployment strategy
type Strategy string

const (
	// StrategyBlueGreen blue/green deployment
	StrategyBlueGreen Strategy = "blue_green"
	// StrategyCanary canary deployment with gradual rollout
	StrategyCanary Strategy = "canary"
	// StrategyRolling rolling update
	StrategyRolling Strategy = "rolling"
	// StrategyImmediate immediate full deployment
	StrategyImmediate Strategy = "immediate"
)

// Environment represents a deployment environment
type Environment struct {
	// Name of the environment
	Name string `json:"name"`

	// Namespace for the environment
	Namespace string `json:"namespace,omitempty"`

	// Cluster for the environment
	Cluster string `json:"cluster,omitempty"`

	// AutoPromote automatically promotes to this environment
	AutoPromote bool `json:"auto_promote"`

	// RequireApproval requires manual approval
	RequireApproval bool `json:"require_approval"`

	// Approvers who can approve promotions
	Approvers []string `json:"approvers,omitempty"`

	// VerificationWorkflow to run before promotion
	VerificationWorkflow string `json:"verification_workflow,omitempty"`

	// Thresholds defines custom threshold configuration for this environment
	// Overrides pipeline-level thresholds when specified
	Thresholds *ThresholdConfig `json:"thresholds,omitempty"`

	// ThresholdPreset uses a named preset for this environment
	// One of: "strict", "relaxed", "latency-sensitive", "throughput-sensitive"
	ThresholdPreset string `json:"threshold_preset,omitempty"`

	// InheritThresholds merges environment thresholds with pipeline thresholds
	// If false, environment thresholds completely replace pipeline thresholds
	InheritThresholds bool `json:"inherit_thresholds,omitempty"`

	// Remediation configures automatic remediation for this environment
	// Overrides pipeline-level remediation when specified
	Remediation *RemediationConfig `json:"remediation,omitempty"`
}

// Pipeline defines a promotion pipeline across environments
type Pipeline struct {
	// Name of the pipeline
	Name string `json:"name"`

	// Application being promoted
	Application string `json:"application"`

	// Environments in order of promotion
	Environments []*Environment `json:"environments"`

	// Strategy for promotions
	Strategy Strategy `json:"strategy"`

	// CanarySteps for canary deployments
	CanarySteps []CanaryStep `json:"canary_steps,omitempty"`

	// RollbackOnFailure automatically rollback on failure.
	//
	// Deprecated: Use Remediation.Enabled with Strategy=RemediationRollback instead.
	RollbackOnFailure bool `json:"rollback_on_failure"`

	// Timeout for each stage
	Timeout time.Duration `json:"timeout"`

	// Thresholds defines custom threshold configuration for this pipeline
	// If nil, uses registry defaults for the strategy
	Thresholds *ThresholdConfig `json:"thresholds,omitempty"`

	// ThresholdPreset uses a named preset configuration
	// One of: "strict", "relaxed", "latency-sensitive", "throughput-sensitive"
	ThresholdPreset string `json:"threshold_preset,omitempty"`

	// Remediation configures automatic remediation on verification failure
	// If nil, uses default remediation config when RollbackOnFailure is true
	Remediation *RemediationConfig `json:"remediation,omitempty"`
}

// CanaryStep defines a step in canary deployment
type CanaryStep struct {
	// Weight is the percentage of traffic (0-100)
	Weight int `json:"weight"`

	// Duration to keep this weight before next step
	Duration time.Duration `json:"duration"`

	// VerificationWorkflow to run at this step
	VerificationWorkflow string `json:"verification_workflow,omitempty"`

	// Thresholds for this specific canary step
	// If nil, uses environment or pipeline thresholds
	Thresholds *ThresholdConfig `json:"thresholds,omitempty"`

	// SkipVerification skips threshold verification for this step
	SkipVerification bool `json:"skip_verification,omitempty"`
}

// Request represents a request to promote
type Request struct {
	// Pipeline name
	Pipeline string

	// FromEnvironment to promote from
	FromEnvironment string

	// ToEnvironment to promote to
	ToEnvironment string

	// Revision to promote
	Revision string

	// RequestedBy user requesting promotion
	RequestedBy string

	// Reason for promotion
	Reason string

	// SkipVerification skips verification step
	SkipVerification bool

	// Force promotion even if checks fail
	Force bool
}

// Status represents the status of a promotion
type Status string

const (
	// StatusPending promotion pending approval
	StatusPending Status = "pending"
	// StatusApproved promotion approved
	StatusApproved Status = "approved"
	// StatusRejected promotion rejected
	StatusRejected Status = "rejected"
	// StatusInProgress promotion in progress
	StatusInProgress Status = "in_progress"
	// StatusVerifying running verification
	StatusVerifying Status = "verifying"
	// StatusRollingOut rolling out changes
	StatusRollingOut Status = "rolling_out"
	// StatusCompleted promotion completed
	StatusCompleted Status = "completed"
	// StatusFailed promotion failed
	StatusFailed Status = "failed"
	// StatusRollingBack rolling back
	StatusRollingBack Status = "rolling_back"
	// StatusRolledBack rolled back
	StatusRolledBack Status = "rolled_back"
)

// Result represents the result of a promotion
type Result struct {
	// ID of the promotion
	ID string `json:"id"`

	// Pipeline configuration
	Pipeline *Pipeline `json:"pipeline"`

	// Request details
	Request *Request `json:"request"`

	// Status of the promotion
	Status Status `json:"status"`

	// CurrentStage in the promotion
	CurrentStage int `json:"current_stage"`

	// Stages completed
	Stages []*StageResult `json:"stages"`

	// StartTime of promotion
	StartTime time.Time `json:"start_time"`

	// EndTime of promotion
	EndTime time.Time `json:"end_time"`

	// Duration of promotion
	Duration time.Duration `json:"duration"`

	// Message contains details
	Message string `json:"message"`

	// Error if promotion failed
	Error error `json:"error,omitempty"`

	// ApprovalInfo for manual approvals
	ApprovalInfo *ApprovalInfo `json:"approval_info,omitempty"`
}

// StageResult represents the result of a promotion stage
type StageResult struct {
	// Environment name
	Environment string `json:"environment"`

	// Status of this stage
	Status Status `json:"status"`

	// StartTime of stage
	StartTime time.Time `json:"start_time"`

	// EndTime of stage
	EndTime time.Time `json:"end_time"`

	// Duration of stage
	Duration time.Duration `json:"duration"`

	// VerificationResult if verification was run
	VerificationResult interface{} `json:"verification_result,omitempty"`

	// CanaryProgress for canary deployments
	CanaryProgress []CanaryProgress `json:"canary_progress,omitempty"`

	// RemediationResult if remediation was attempted
	RemediationResult *RemediationResult `json:"remediation_result,omitempty"`

	// Message contains stage details
	Message string `json:"message"`

	// Error if stage failed
	Error error `json:"error,omitempty"`
}

// CanaryProgress tracks canary deployment progress
type CanaryProgress struct {
	// Step number
	Step int `json:"step"`

	// Weight percentage
	Weight int `json:"weight"`

	// StartTime of this step
	StartTime time.Time `json:"start_time"`

	// EndTime of this step
	EndTime time.Time `json:"end_time"`

	// Status of this step
	Status string `json:"status"`

	// Metrics collected during this step
	Metrics map[string]float64 `json:"metrics,omitempty"`
}

// ApprovalInfo contains promotion approval information
type ApprovalInfo struct {
	// Required indicates if approval is required
	Required bool `json:"required"`

	// Status of approval
	Status Status `json:"status"`

	// ApprovedBy user who approved
	ApprovedBy string `json:"approved_by,omitempty"`

	// ApprovedAt timestamp
	ApprovedAt time.Time `json:"approved_at,omitempty"`

	// RejectedBy user who rejected
	RejectedBy string `json:"rejected_by,omitempty"`

	// RejectedAt timestamp
	RejectedAt time.Time `json:"rejected_at,omitempty"`

	// Reason for approval/rejection
	Reason string `json:"reason,omitempty"`
}

// ApprovalRequest represents an approval or rejection
type ApprovalRequest struct {
	// PromotionID to approve/reject
	PromotionID string

	// Approved indicates approval
	Approved bool

	// ApprovedBy user approving/rejecting
	ApprovedBy string

	// Reason for decision
	Reason string
}

// RemediationStrategy defines how remediation is performed
type RemediationStrategy string

const (
	// RemediationRollback rollback to previous revision
	RemediationRollback RemediationStrategy = "rollback"
	// RemediationScaleDown scale down the failed deployment
	RemediationScaleDown RemediationStrategy = "scale_down"
	// RemediationTrafficShift shift traffic away from failed deployment
	RemediationTrafficShift RemediationStrategy = "traffic_shift"
	// RemediationCustom execute a custom remediation workflow
	RemediationCustom RemediationStrategy = "custom"
)

// RemediationConfig configures automatic remediation behavior
type RemediationConfig struct {
	// Enabled enables automatic remediation
	Enabled bool `json:"enabled"`

	// Strategy defines how remediation is performed
	Strategy RemediationStrategy `json:"strategy"`

	// MaxAttempts maximum remediation attempts before giving up
	MaxAttempts int `json:"max_attempts,omitempty"`

	// RetryDelay delay between remediation attempts
	RetryDelay time.Duration `json:"retry_delay,omitempty"`

	// TimeoutPerAttempt timeout for each remediation attempt
	TimeoutPerAttempt time.Duration `json:"timeout_per_attempt,omitempty"`

	// CustomWorkflow name of workflow to run for custom remediation
	CustomWorkflow string `json:"custom_workflow,omitempty"`

	// NotifyOnRemediation send notifications when remediation occurs
	NotifyOnRemediation bool `json:"notify_on_remediation,omitempty"`

	// NotificationChannels channels to notify (slack, pagerduty, etc.)
	NotificationChannels []string `json:"notification_channels,omitempty"`

	// PreserveFailedPods keep failed pods for debugging
	PreserveFailedPods bool `json:"preserve_failed_pods,omitempty"`

	// CollectDiagnostics gather diagnostic info before remediation
	CollectDiagnostics bool `json:"collect_diagnostics,omitempty"`
}

// DefaultRemediationConfig returns default remediation configuration
func DefaultRemediationConfig() *RemediationConfig {
	return &RemediationConfig{
		Enabled:             true,
		Strategy:            RemediationRollback,
		MaxAttempts:         3,
		RetryDelay:          10 * time.Second,
		TimeoutPerAttempt:   2 * time.Minute,
		NotifyOnRemediation: true,
		CollectDiagnostics:  true,
	}
}

// RemediationStatus represents the status of a remediation action
type RemediationStatus string

const (
	// RemediationStatusPending remediation not yet started
	RemediationStatusPending RemediationStatus = "pending"
	// RemediationStatusInProgress remediation in progress
	RemediationStatusInProgress RemediationStatus = "in_progress"
	// RemediationStatusSucceeded remediation succeeded
	RemediationStatusSucceeded RemediationStatus = "succeeded"
	// RemediationStatusFailed remediation failed
	RemediationStatusFailed RemediationStatus = "failed"
	// RemediationStatusSkipped remediation was skipped
	RemediationStatusSkipped RemediationStatus = "skipped"
)

// RemediationResult tracks the result of a remediation action
type RemediationResult struct {
	// ID unique identifier for this remediation
	ID string `json:"id"`

	// PromotionID the promotion this remediation relates to
	PromotionID string `json:"promotion_id"`

	// Trigger what triggered the remediation
	Trigger RemediationTrigger `json:"trigger"`

	// Strategy used for remediation
	Strategy RemediationStrategy `json:"strategy"`

	// Status of the remediation
	Status RemediationStatus `json:"status"`

	// Attempts made
	Attempts int `json:"attempts"`

	// AttemptDetails details for each attempt
	AttemptDetails []RemediationAttempt `json:"attempt_details,omitempty"`

	// PreviousRevision the revision being rolled back from
	PreviousRevision string `json:"previous_revision,omitempty"`

	// TargetRevision the revision being rolled back to
	TargetRevision string `json:"target_revision,omitempty"`

	// StartTime when remediation started
	StartTime time.Time `json:"start_time"`

	// EndTime when remediation completed
	EndTime time.Time `json:"end_time,omitempty"`

	// Duration of remediation
	Duration time.Duration `json:"duration,omitempty"`

	// Diagnostics collected diagnostic information
	Diagnostics *DiagnosticInfo `json:"diagnostics,omitempty"`

	// Message summary message
	Message string `json:"message"`

	// Error if remediation failed
	Error error `json:"error,omitempty"`
}

// RemediationTrigger defines what triggered the remediation
type RemediationTrigger struct {
	// Type of trigger
	Type string `json:"type"`

	// EvaluationResult if triggered by threshold failure
	EvaluationResult *EvaluationResult `json:"evaluation_result,omitempty"`

	// FailedStep the canary step that failed
	FailedStep int `json:"failed_step,omitempty"`

	// FailedWeight the traffic weight at failure
	FailedWeight int `json:"failed_weight,omitempty"`

	// Reason human-readable description
	Reason string `json:"reason"`
}

// RemediationAttempt tracks a single remediation attempt
type RemediationAttempt struct {
	// Attempt number (1-based)
	Attempt int `json:"attempt"`

	// StartTime when this attempt started
	StartTime time.Time `json:"start_time"`

	// EndTime when this attempt ended
	EndTime time.Time `json:"end_time,omitempty"`

	// Status of this attempt
	Status RemediationStatus `json:"status"`

	// Actions taken during this attempt
	Actions []string `json:"actions,omitempty"`

	// Error if attempt failed
	Error string `json:"error,omitempty"`
}

// DiagnosticInfo contains diagnostic information collected before/during remediation
type DiagnosticInfo struct {
	// CollectedAt when diagnostics were collected
	CollectedAt time.Time `json:"collected_at"`

	// PodLogs relevant pod logs
	PodLogs map[string]string `json:"pod_logs,omitempty"`

	// Events Kubernetes events
	Events []string `json:"events,omitempty"`

	// Metrics relevant metrics at time of failure
	Metrics map[string]float64 `json:"metrics,omitempty"`

	// ResourceStatus status of relevant resources
	ResourceStatus map[string]string `json:"resource_status,omitempty"`
}
