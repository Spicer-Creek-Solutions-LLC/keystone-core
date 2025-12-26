package rollback

import "time"

// RollbackType represents the type of rollback operation
type RollbackType string

const (
	// RollbackTypeArgoCD rollback via ArgoCD
	RollbackTypeArgoCD RollbackType = "argocd"
	// RollbackTypeFlux rollback via Flux
	RollbackTypeFlux RollbackType = "flux"
	// RollbackTypeGit rollback via Git revert/reset
	RollbackTypeGit RollbackType = "git"
	// RollbackTypeManual manual rollback requiring approval
	RollbackTypeManual RollbackType = "manual"
)

// RollbackStrategy defines how the rollback should be performed
type RollbackStrategy string

const (
	// StrategyPreviousRevision rollback to the immediately previous revision
	StrategyPreviousRevision RollbackStrategy = "previous"
	// StrategySpecificRevision rollback to a specific revision
	StrategySpecificRevision RollbackStrategy = "specific"
	// StrategyLastKnownGood rollback to the last known good revision
	StrategyLastKnownGood RollbackStrategy = "last_known_good"
)

// RollbackTrigger defines when a rollback should be triggered
type RollbackTrigger string

const (
	// TriggerManual manual trigger (requires approval)
	TriggerManual RollbackTrigger = "manual"
	// TriggerAutomatic automatic trigger on failure
	TriggerAutomatic RollbackTrigger = "automatic"
	// TriggerScheduled scheduled trigger
	TriggerScheduled RollbackTrigger = "scheduled"
)

// RollbackConfig defines the configuration for a rollback operation
type RollbackConfig struct {
	// Name of the rollback configuration
	Name string `json:"name"`

	// Type of rollback
	Type RollbackType `json:"type"`

	// Strategy for rollback
	Strategy RollbackStrategy `json:"strategy"`

	// Trigger for rollback
	Trigger RollbackTrigger `json:"trigger"`

	// Application or resource name
	Application string `json:"application"`

	// Namespace for the application
	Namespace string `json:"namespace,omitempty"`

	// Revision to rollback to (for StrategySpecificRevision)
	Revision string `json:"revision,omitempty"`

	// RequireApproval indicates if manual approval is needed
	RequireApproval bool `json:"require_approval"`

	// Approvers is a list of users who can approve
	Approvers []string `json:"approvers,omitempty"`

	// Timeout for rollback operation
	Timeout time.Duration `json:"timeout"`

	// VerifyAfterRollback indicates if verification should run after rollback
	VerifyAfterRollback bool `json:"verify_after_rollback"`

	// VerificationWorkflow name to run after rollback
	VerificationWorkflow string `json:"verification_workflow,omitempty"`

	// Metadata for additional configuration
	Metadata map[string]string `json:"metadata,omitempty"`
}

// RollbackRequest represents a request to perform a rollback
type RollbackRequest struct {
	// ConfigName is the name of the rollback configuration to use
	ConfigName string

	// Reason for the rollback
	Reason string

	// RequestedBy is the user requesting the rollback
	RequestedBy string

	// OverrideRevision allows overriding the revision from config
	OverrideRevision string

	// SkipVerification skips post-rollback verification
	SkipVerification bool
}

// RollbackStatus represents the status of a rollback operation
type RollbackStatus string

const (
	// StatusPending rollback is pending approval
	StatusPending RollbackStatus = "pending"
	// StatusApproved rollback has been approved
	StatusApproved RollbackStatus = "approved"
	// StatusRejected rollback has been rejected
	StatusRejected RollbackStatus = "rejected"
	// StatusInProgress rollback is in progress
	StatusInProgress RollbackStatus = "in_progress"
	// StatusCompleted rollback completed successfully
	StatusCompleted RollbackStatus = "completed"
	// StatusFailed rollback failed
	StatusFailed RollbackStatus = "failed"
	// StatusVerifying verification in progress
	StatusVerifying RollbackStatus = "verifying"
	// StatusVerified verification passed
	StatusVerified RollbackStatus = "verified"
	// StatusVerificationFailed verification failed
	StatusVerificationFailed RollbackStatus = "verification_failed"
)

// RollbackResult represents the result of a rollback operation
type RollbackResult struct {
	// ID is the unique identifier for this rollback
	ID string `json:"id"`

	// Config is the rollback configuration used
	Config *RollbackConfig `json:"config"`

	// Request is the original request
	Request *RollbackRequest `json:"request"`

	// Status of the rollback
	Status RollbackStatus `json:"status"`

	// PreviousRevision before rollback
	PreviousRevision string `json:"previous_revision"`

	// CurrentRevision after rollback
	CurrentRevision string `json:"current_revision"`

	// StartTime of the rollback
	StartTime time.Time `json:"start_time"`

	// EndTime of the rollback
	EndTime time.Time `json:"end_time"`

	// Duration of the rollback
	Duration time.Duration `json:"duration"`

	// Message contains details about the rollback
	Message string `json:"message"`

	// Error if rollback failed
	Error error `json:"error,omitempty"`

	// VerificationResult if verification was run
	VerificationResult interface{} `json:"verification_result,omitempty"`

	// ApprovalInfo contains approval details
	ApprovalInfo *ApprovalInfo `json:"approval_info,omitempty"`
}

// ApprovalInfo contains information about rollback approval
type ApprovalInfo struct {
	// Required indicates if approval is required
	Required bool `json:"required"`

	// Status of the approval
	Status RollbackStatus `json:"status"`

	// ApprovedBy is the user who approved
	ApprovedBy string `json:"approved_by,omitempty"`

	// ApprovedAt is when the approval was granted
	ApprovedAt time.Time `json:"approved_at,omitempty"`

	// RejectedBy is the user who rejected
	RejectedBy string `json:"rejected_by,omitempty"`

	// RejectedAt is when the rejection occurred
	RejectedAt time.Time `json:"rejected_at,omitempty"`

	// Reason for approval/rejection
	Reason string `json:"reason,omitempty"`
}

// ApprovalRequest represents a request to approve or reject a rollback
type ApprovalRequest struct {
	// RollbackID is the ID of the rollback to approve/reject
	RollbackID string

	// Approved indicates if the rollback is approved
	Approved bool

	// ApprovedBy is the user approving/rejecting
	ApprovedBy string

	// Reason for the decision
	Reason string
}
