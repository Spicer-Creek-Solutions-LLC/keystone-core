package promotion

import "time"

// PromotionStrategy defines the deployment strategy
type PromotionStrategy string

const (
	// StrategyBlueGreen blue/green deployment
	StrategyBlueGreen PromotionStrategy = "blue_green"
	// StrategyCanary canary deployment with gradual rollout
	StrategyCanary PromotionStrategy = "canary"
	// StrategyRolling rolling update
	StrategyRolling PromotionStrategy = "rolling"
	// StrategyImmediate immediate full deployment
	StrategyImmediate PromotionStrategy = "immediate"
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
	Strategy PromotionStrategy `json:"strategy"`

	// CanarySteps for canary deployments
	CanarySteps []CanaryStep `json:"canary_steps,omitempty"`

	// RollbackOnFailure automatically rollback on failure
	RollbackOnFailure bool `json:"rollback_on_failure"`

	// Timeout for each stage
	Timeout time.Duration `json:"timeout"`
}

// CanaryStep defines a step in canary deployment
type CanaryStep struct {
	// Weight is the percentage of traffic (0-100)
	Weight int `json:"weight"`

	// Duration to keep this weight before next step
	Duration time.Duration `json:"duration"`

	// VerificationWorkflow to run at this step
	VerificationWorkflow string `json:"verification_workflow,omitempty"`
}

// PromotionRequest represents a request to promote
type PromotionRequest struct {
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

// PromotionStatus represents the status of a promotion
type PromotionStatus string

const (
	// StatusPending promotion pending approval
	StatusPending PromotionStatus = "pending"
	// StatusApproved promotion approved
	StatusApproved PromotionStatus = "approved"
	// StatusRejected promotion rejected
	StatusRejected PromotionStatus = "rejected"
	// StatusInProgress promotion in progress
	StatusInProgress PromotionStatus = "in_progress"
	// StatusVerifying running verification
	StatusVerifying PromotionStatus = "verifying"
	// StatusRollingOut rolling out changes
	StatusRollingOut PromotionStatus = "rolling_out"
	// StatusCompleted promotion completed
	StatusCompleted PromotionStatus = "completed"
	// StatusFailed promotion failed
	StatusFailed PromotionStatus = "failed"
	// StatusRollingBack rolling back
	StatusRollingBack PromotionStatus = "rolling_back"
	// StatusRolledBack rolled back
	StatusRolledBack PromotionStatus = "rolled_back"
)

// PromotionResult represents the result of a promotion
type PromotionResult struct {
	// ID of the promotion
	ID string `json:"id"`

	// Pipeline configuration
	Pipeline *Pipeline `json:"pipeline"`

	// Request details
	Request *PromotionRequest `json:"request"`

	// Status of the promotion
	Status PromotionStatus `json:"status"`

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
	Status PromotionStatus `json:"status"`

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
	Status PromotionStatus `json:"status"`

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
