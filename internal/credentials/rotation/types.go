package rotation

import (
	"time"

	"github.com/shawnbutts/keystone-core/internal/credentials"
	"github.com/shawnbutts/keystone-core/internal/vendors"
)

// CredRotationState represents the state of a credential rotation job.
type CredRotationState string

// Credential rotation states.
const (
	CredRotationPending     CredRotationState = "pending"
	CredRotationValidating  CredRotationState = "validating_old"
	CredRotationGenerating  CredRotationState = "generating"
	CredRotationApplying    CredRotationState = "applying"
	CredRotationVerifying   CredRotationState = "verifying_new"
	CredRotationStoring     CredRotationState = "storing"
	CredRotationCleanup     CredRotationState = "cleanup"
	CredRotationCompleted   CredRotationState = "completed"
	CredRotationFailed      CredRotationState = "failed"
	CredRotationRollingBack CredRotationState = "rolling_back"
	CredRotationRolledBack  CredRotationState = "rolled_back"
)

// IsTerminal returns true if the state is a terminal state.
func (s CredRotationState) IsTerminal() bool {
	switch s {
	case CredRotationCompleted, CredRotationFailed, CredRotationRolledBack:
		return true
	default:
		return false
	}
}

// CredRotationEvent represents events that trigger state transitions.
type CredRotationEvent string

// Credential rotation event types.
const (
	CredRotationEventStart      CredRotationEvent = "start"
	CredRotationEventValidated  CredRotationEvent = "validated"
	CredRotationEventGenerated  CredRotationEvent = "generated"
	CredRotationEventApplied    CredRotationEvent = "applied"
	CredRotationEventVerified   CredRotationEvent = "verified"
	CredRotationEventStored     CredRotationEvent = "stored"
	CredRotationEventCleanedUp  CredRotationEvent = "cleaned_up"
	CredRotationEventFailed     CredRotationEvent = "failed"
	CredRotationEventRollback   CredRotationEvent = "rollback"
	CredRotationEventRolledBack CredRotationEvent = "rolled_back"
)

// DeviceInfo identifies a network device for credential rotation.
type DeviceInfo struct {
	ID           string            `json:"id"`
	Host         string            `json:"host"`
	Port         int               `json:"port"`
	VendorType   vendors.VendorType `json:"vendor_type"`
	ProxyAgentID string            `json:"proxy_agent_id"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// Job represents a single credential rotation operation.
type Job struct {
	ID             string                   `json:"id"`
	DeviceID       string                   `json:"device_id"`
	CredentialID   string                   `json:"credential_id"`
	CredentialType credentials.CredentialType `json:"credential_type"`
	ProxyAgentID   string                   `json:"proxy_agent_id"`
	Device         DeviceInfo               `json:"device"`
	State          CredRotationState        `json:"state"`
	OldCredential  credentials.Credential   `json:"-"`
	NewCredential  credentials.Credential   `json:"-"`
	RollbackOnFail bool                     `json:"rollback_on_fail"`
	StartedAt      time.Time                `json:"started_at,omitempty"`
	CompletedAt    time.Time                `json:"completed_at,omitempty"`
	Error          string                   `json:"error,omitempty"`
}

// Result contains the outcome of a credential rotation.
type Result struct {
	JobID          string                   `json:"job_id"`
	CredentialID   string                   `json:"credential_id"`
	CredentialType credentials.CredentialType `json:"credential_type"`
	DeviceID       string                   `json:"device_id"`
	Success        bool                     `json:"success"`
	RolledBack     bool                     `json:"rolled_back"`
	Error          string                   `json:"error,omitempty"`
	Duration       time.Duration            `json:"duration"`
	StartedAt      time.Time                `json:"started_at"`
	CompletedAt    time.Time                `json:"completed_at"`
}

// PolicyAction indicates what action a policy evaluation recommends.
type PolicyAction string

// Policy evaluation action results.
const (
	PolicyActionOK      PolicyAction = "ok"
	PolicyActionWarning PolicyAction = "warning"
	PolicyActionRotate  PolicyAction = "rotate"
	PolicyActionExpired PolicyAction = "expired"
)

// Policy defines when and how credentials should be rotated.
type Policy struct {
	ID              string                      `json:"id"`
	Name            string                      `json:"name"`
	CredentialTypes []credentials.CredentialType `json:"credential_types"`
	MaxAge          time.Duration               `json:"max_age"`
	WarningAge      time.Duration               `json:"warning_age"`
	Schedule        string                      `json:"schedule"`
	AutoRotate      bool                        `json:"auto_rotate"`
	RollbackOnFail  bool                        `json:"rollback_on_fail"`
	Enabled         bool                        `json:"enabled"`
}

// PolicyResult is the outcome of evaluating a credential against a policy.
type PolicyResult struct {
	PolicyID     string       `json:"policy_id"`
	CredentialID string       `json:"credential_id"`
	Action       PolicyAction `json:"action"`
	Age          time.Duration `json:"age"`
	MaxAge       time.Duration `json:"max_age"`
	Message      string       `json:"message,omitempty"`
}

// SchedulerStatus reports the current state of the rotation scheduler.
type SchedulerStatus struct {
	Running        bool      `json:"running"`
	PolicyCount    int       `json:"policy_count"`
	ActiveJobs     int       `json:"active_jobs"`
	CompletedJobs  int       `json:"completed_jobs"`
	FailedJobs     int       `json:"failed_jobs"`
	LastCheckTime  time.Time `json:"last_check_time,omitempty"`
	NextCheckTime  time.Time `json:"next_check_time,omitempty"`
}
