// Package approval provides types and functions for runbook approval workflows.
package approval

import (
	"time"
)

// RequestState represents the current state of an approval request.
type RequestState string

// Request state constants.
const (
	RequestStatePending   RequestState = "pending"
	RequestStateApproved  RequestState = "approved"
	RequestStateRejected  RequestState = "rejected"
	RequestStateExpired   RequestState = "expired"
	RequestStateCancelled RequestState = "cancelled"
)

// Mode defines how multiple approvers are handled.
type Mode string

// Approval mode constants.
const (
	// ModeAny requires any one approver to approve.
	ModeAny Mode = "any"
	// ModeAll requires all approvers to approve.
	ModeAll Mode = "all"
	// ModeCount requires a specific number of approvals.
	ModeCount Mode = "count"
)

// Request represents an approval request for a runbook step.
type Request struct {
	// ID is the unique identifier for this request.
	ID string `json:"id"`

	// ExecutionID is the runbook execution this request belongs to.
	ExecutionID string `json:"executionId"`

	// StepName is the name of the step requiring approval.
	StepName string `json:"stepName"`

	// State is the current state of the request.
	State RequestState `json:"state"`

	// Title is a brief description of what's being approved.
	Title string `json:"title"`

	// Description provides detailed context for the approval decision.
	Description string `json:"description,omitempty"`

	// Approvers lists the users or groups who can approve this request.
	Approvers []string `json:"approvers"`

	// Mode specifies how multiple approvers are handled.
	Mode Mode `json:"mode"`

	// RequiredCount is the number of approvals needed (for ModeCount).
	RequiredCount int `json:"requiredCount,omitempty"`

	// Responses contains the approval decisions made so far.
	Responses []Response `json:"responses,omitempty"`

	// Timeout is the duration after which the request expires.
	Timeout time.Duration `json:"timeout,omitempty"`

	// ExpiresAt is when the request will expire.
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`

	// Metadata contains additional context for the approval.
	Metadata map[string]interface{} `json:"metadata,omitempty"`

	// CreatedAt is when the request was created.
	CreatedAt time.Time `json:"createdAt"`

	// UpdatedAt is when the request was last updated.
	UpdatedAt time.Time `json:"updatedAt"`

	// CompletedAt is when the request reached a terminal state.
	CompletedAt *time.Time `json:"completedAt,omitempty"`
}

// Response represents a single approval or rejection decision.
type Response struct {
	// Approver is the user or identity who responded.
	Approver string `json:"approver"`

	// Decision is whether the request was approved or rejected.
	Decision Decision `json:"decision"`

	// Comment provides optional context for the decision.
	Comment string `json:"comment,omitempty"`

	// RespondedAt is when the response was recorded.
	RespondedAt time.Time `json:"respondedAt"`
}

// Decision represents an approval decision.
type Decision string

// Decision constants.
const (
	DecisionApproved Decision = "approved"
	DecisionRejected Decision = "rejected"
)

// IsTerminal returns true if the request state is terminal.
func (s RequestState) IsTerminal() bool {
	return s == RequestStateApproved || s == RequestStateRejected ||
		s == RequestStateExpired || s == RequestStateCancelled
}

// String returns the string representation of the request state.
func (s RequestState) String() string {
	return string(s)
}

// IsValid returns true if the approval mode is valid.
func (m Mode) IsValid() bool {
	switch m {
	case ModeAny, ModeAll, ModeCount:
		return true
	}
	return false
}

// String returns the string representation of the approval mode.
func (m Mode) String() string {
	return string(m)
}

// ApprovalCount returns the number of approvals received.
func (r *Request) ApprovalCount() int {
	count := 0
	for _, resp := range r.Responses {
		if resp.Decision == DecisionApproved {
			count++
		}
	}
	return count
}

// RejectionCount returns the number of rejections received.
func (r *Request) RejectionCount() int {
	count := 0
	for _, resp := range r.Responses {
		if resp.Decision == DecisionRejected {
			count++
		}
	}
	return count
}

// HasResponded returns true if the given approver has already responded.
func (r *Request) HasResponded(approver string) bool {
	for _, resp := range r.Responses {
		if resp.Approver == approver {
			return true
		}
	}
	return false
}

// IsApprover returns true if the given identity is in the approvers list.
func (r *Request) IsApprover(identity string) bool {
	for _, approver := range r.Approvers {
		if approver == identity {
			return true
		}
	}
	return false
}

// IsExpired returns true if the request has expired.
func (r *Request) IsExpired() bool {
	if r.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*r.ExpiresAt)
}

// Config defines the configuration for an approval step.
type Config struct {
	// Title is the approval request title.
	Title string `yaml:"title" json:"title"`

	// Description provides context for the approval.
	Description string `yaml:"description,omitempty" json:"description,omitempty"`

	// Approvers lists users or groups who can approve.
	Approvers []string `yaml:"approvers" json:"approvers"`

	// Mode specifies how multiple approvers are handled.
	// Valid values: any, all, count. Default: any.
	Mode Mode `yaml:"mode,omitempty" json:"mode,omitempty"`

	// RequiredCount is the number of approvals needed (for mode=count).
	RequiredCount int `yaml:"requiredCount,omitempty" json:"requiredCount,omitempty"`

	// Timeout is how long to wait for approval before expiring.
	// Format: Go duration string (e.g., "1h", "24h").
	Timeout string `yaml:"timeout,omitempty" json:"timeout,omitempty"`

	// NotifyChannels lists notification channels for approval requests.
	NotifyChannels []string `yaml:"notifyChannels,omitempty" json:"notifyChannels,omitempty"`

	// ReminderInterval specifies how often to send reminders.
	// Format: Go duration string.
	ReminderInterval string `yaml:"reminderInterval,omitempty" json:"reminderInterval,omitempty"`

	// EscalateAfter specifies when to escalate if no response.
	// Format: Go duration string.
	EscalateAfter string `yaml:"escalateAfter,omitempty" json:"escalateAfter,omitempty"`

	// EscalateTo lists users or groups to escalate to.
	EscalateTo []string `yaml:"escalateTo,omitempty" json:"escalateTo,omitempty"`
}

// GetMode returns the approval mode, defaulting to "any".
func (c *Config) GetMode() Mode {
	if c.Mode == "" {
		return ModeAny
	}
	return c.Mode
}

// GetRequiredCount returns the required approval count based on mode.
func (c *Config) GetRequiredCount() int {
	switch c.GetMode() {
	case ModeAny:
		return 1
	case ModeAll:
		return len(c.Approvers)
	case ModeCount:
		if c.RequiredCount > 0 {
			return c.RequiredCount
		}
		return 1
	}
	return 1
}

// GetTimeout returns the timeout duration, or zero if not set.
func (c *Config) GetTimeout() time.Duration {
	if c.Timeout == "" {
		return 0
	}
	d, err := time.ParseDuration(c.Timeout)
	if err != nil {
		return 0
	}
	return d
}

// GetReminderInterval returns the reminder interval, or zero if not set.
func (c *Config) GetReminderInterval() time.Duration {
	if c.ReminderInterval == "" {
		return 0
	}
	d, err := time.ParseDuration(c.ReminderInterval)
	if err != nil {
		return 0
	}
	return d
}

// GetEscalateAfter returns the escalation duration, or zero if not set.
func (c *Config) GetEscalateAfter() time.Duration {
	if c.EscalateAfter == "" {
		return 0
	}
	d, err := time.ParseDuration(c.EscalateAfter)
	if err != nil {
		return 0
	}
	return d
}
