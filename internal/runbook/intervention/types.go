// Package intervention provides types and functions for manual intervention
// in runbook workflows.
package intervention

import (
	"time"
)

// State represents the current state of an intervention request.
type State string

// Intervention state constants.
const (
	StatePending   State = "pending"
	StateCompleted State = "completed"
	StateExpired   State = "expired"
	StateCancelled State = "cancelled"
)

// Type defines the type of intervention requested.
type Type string

// Intervention type constants.
const (
	// TypePrompt requests operator input.
	TypePrompt Type = "prompt"

	// TypeWaitManual waits for manual confirmation to proceed.
	TypeWaitManual Type = "wait_manual"

	// TypeConfirm requests confirmation (yes/no) before proceeding.
	TypeConfirm Type = "confirm"
)

// Request represents a manual intervention request.
type Request struct {
	// ID is the unique identifier for this request.
	ID string `json:"id"`

	// ExecutionID is the runbook execution this request belongs to.
	ExecutionID string `json:"executionId"`

	// StepName is the name of the step requiring intervention.
	StepName string `json:"stepName"`

	// Type is the type of intervention requested.
	Type Type `json:"type"`

	// State is the current state of the request.
	State State `json:"state"`

	// Title is a brief description of what's being requested.
	Title string `json:"title"`

	// Description provides detailed context for the intervention.
	Description string `json:"description,omitempty"`

	// Prompts defines the input fields for prompt-type interventions.
	Prompts []PromptField `json:"prompts,omitempty"`

	// Response contains the operator's response.
	Response *Response `json:"response,omitempty"`

	// Timeout is the duration after which the request expires.
	Timeout time.Duration `json:"timeout,omitempty"`

	// ExpiresAt is when the request will expire.
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`

	// Metadata contains additional context.
	Metadata map[string]interface{} `json:"metadata,omitempty"`

	// CreatedAt is when the request was created.
	CreatedAt time.Time `json:"createdAt"`

	// UpdatedAt is when the request was last updated.
	UpdatedAt time.Time `json:"updatedAt"`

	// CompletedAt is when the request reached a terminal state.
	CompletedAt *time.Time `json:"completedAt,omitempty"`
}

// PromptField defines a single input field for prompt interventions.
type PromptField struct {
	// Name is the identifier for this field.
	Name string `json:"name" yaml:"name"`

	// Label is the display label for the field.
	Label string `json:"label" yaml:"label"`

	// Type is the field type (text, number, boolean, select, multiselect, textarea).
	Type FieldType `json:"type" yaml:"type"`

	// Required indicates whether this field must be provided.
	Required bool `json:"required,omitempty" yaml:"required,omitempty"`

	// Default is the default value for the field.
	Default interface{} `json:"default,omitempty" yaml:"default,omitempty"`

	// Description provides help text for the field.
	Description string `json:"description,omitempty" yaml:"description,omitempty"`

	// Options is the list of options for select/multiselect fields.
	Options []Option `json:"options,omitempty" yaml:"options,omitempty"`

	// Validation specifies validation rules for the field.
	Validation *FieldValidation `json:"validation,omitempty" yaml:"validation,omitempty"`
}

// FieldType represents the type of a prompt field.
type FieldType string

// Field type constants.
const (
	FieldTypeText        FieldType = "text"
	FieldTypeNumber      FieldType = "number"
	FieldTypeBoolean     FieldType = "boolean"
	FieldTypeSelect      FieldType = "select"
	FieldTypeMultiSelect FieldType = "multiselect"
	FieldTypeTextarea    FieldType = "textarea"
	FieldTypePassword    FieldType = "password"
)

// Option represents a choice for select/multiselect fields.
type Option struct {
	// Value is the option value.
	Value interface{} `json:"value" yaml:"value"`

	// Label is the display label for the option.
	Label string `json:"label" yaml:"label"`
}

// FieldValidation specifies validation rules for a prompt field.
type FieldValidation struct {
	// Pattern is a regex pattern for text validation.
	Pattern string `json:"pattern,omitempty" yaml:"pattern,omitempty"`

	// Min is the minimum value for numbers or minimum length for strings.
	Min *float64 `json:"min,omitempty" yaml:"min,omitempty"`

	// Max is the maximum value for numbers or maximum length for strings.
	Max *float64 `json:"max,omitempty" yaml:"max,omitempty"`
}

// Response contains the operator's response to an intervention request.
type Response struct {
	// Operator is the identity of the responding operator.
	Operator string `json:"operator"`

	// Values contains the input values for prompt-type interventions.
	Values map[string]interface{} `json:"values,omitempty"`

	// Confirmed indicates whether the operator confirmed (for confirm type).
	Confirmed bool `json:"confirmed,omitempty"`

	// Comment provides optional context for the response.
	Comment string `json:"comment,omitempty"`

	// RespondedAt is when the response was recorded.
	RespondedAt time.Time `json:"respondedAt"`
}

// IsTerminal returns true if the intervention state is terminal.
func (s State) IsTerminal() bool {
	return s == StateCompleted || s == StateExpired || s == StateCancelled
}

// String returns the string representation of the intervention state.
func (s State) String() string {
	return string(s)
}

// IsValid returns true if the intervention type is valid.
func (t Type) IsValid() bool {
	switch t {
	case TypePrompt, TypeWaitManual, TypeConfirm:
		return true
	}
	return false
}

// String returns the string representation of the intervention type.
func (t Type) String() string {
	return string(t)
}

// IsValid returns true if the field type is valid.
func (t FieldType) IsValid() bool {
	switch t {
	case FieldTypeText, FieldTypeNumber, FieldTypeBoolean,
		FieldTypeSelect, FieldTypeMultiSelect, FieldTypeTextarea, FieldTypePassword:
		return true
	}
	return false
}

// String returns the string representation of the field type.
func (t FieldType) String() string {
	return string(t)
}

// IsExpired returns true if the request has expired.
func (r *Request) IsExpired() bool {
	if r.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*r.ExpiresAt)
}

// Config defines the configuration for an intervention step.
type Config struct {
	// Type is the type of intervention (prompt, wait_manual, confirm).
	Type Type `yaml:"type" json:"type"`

	// Title is the intervention request title.
	Title string `yaml:"title" json:"title"`

	// Description provides context for the intervention.
	Description string `yaml:"description,omitempty" json:"description,omitempty"`

	// Prompts defines the input fields for prompt-type interventions.
	Prompts []PromptField `yaml:"prompts,omitempty" json:"prompts,omitempty"`

	// ConfirmMessage is the message to display for confirm-type interventions.
	ConfirmMessage string `yaml:"confirmMessage,omitempty" json:"confirmMessage,omitempty"`

	// Timeout is how long to wait for the intervention.
	// Format: Go duration string (e.g., "1h", "24h").
	Timeout string `yaml:"timeout,omitempty" json:"timeout,omitempty"`

	// NotifyChannels lists notification channels for intervention requests.
	NotifyChannels []string `yaml:"notifyChannels,omitempty" json:"notifyChannels,omitempty"`
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
