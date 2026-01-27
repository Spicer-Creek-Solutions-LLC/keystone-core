// Package runbook provides types and functions for defining and executing
// operational runbooks in Keystone Core.
package runbook

import (
	"time"
)

// APIVersion is the current runbook API version.
const APIVersion = "runbook.keystone.io/v1"

// Kind is the resource kind for runbooks.
const Kind = "Runbook"

// Runbook represents a complete runbook definition.
type Runbook struct {
	APIVersion string      `yaml:"apiVersion" json:"apiVersion"`
	Kind       string      `yaml:"kind" json:"kind"`
	Metadata   Metadata    `yaml:"metadata" json:"metadata"`
	Spec       RunbookSpec `yaml:"spec" json:"spec"`
}

// Metadata contains runbook identification and annotation information.
type Metadata struct {
	Name        string            `yaml:"name" json:"name"`
	Namespace   string            `yaml:"namespace,omitempty" json:"namespace,omitempty"`
	Version     string            `yaml:"version,omitempty" json:"version,omitempty"`
	Labels      map[string]string `yaml:"labels,omitempty" json:"labels,omitempty"`
	Annotations map[string]string `yaml:"annotations,omitempty" json:"annotations,omitempty"`
	Description string            `yaml:"description,omitempty" json:"description,omitempty"`
}

// RunbookSpec defines the runbook's behavior and structure.
type RunbookSpec struct {
	// Description provides a human-readable summary of the runbook's purpose.
	Description string `yaml:"description,omitempty" json:"description,omitempty"`

	// Inputs defines the input parameters for the runbook.
	Inputs []InputDef `yaml:"inputs,omitempty" json:"inputs,omitempty"`

	// Steps defines the ordered list of steps to execute.
	Steps []Step `yaml:"steps" json:"steps"`

	// OnSuccess defines steps to execute when the runbook completes successfully.
	OnSuccess []Step `yaml:"onSuccess,omitempty" json:"onSuccess,omitempty"`

	// OnFailure defines steps to execute when the runbook fails.
	OnFailure []Step `yaml:"onFailure,omitempty" json:"onFailure,omitempty"`

	// Timeout specifies the maximum duration for runbook execution.
	// Format: Go duration string (e.g., "30m", "1h").
	Timeout string `yaml:"timeout,omitempty" json:"timeout,omitempty"`

	// MaxRetries specifies the maximum number of retries for the entire runbook.
	MaxRetries int `yaml:"maxRetries,omitempty" json:"maxRetries,omitempty"`
}

// InputDef defines a runbook input parameter.
type InputDef struct {
	// Name is the unique identifier for this input.
	Name string `yaml:"name" json:"name"`

	// Type specifies the expected data type.
	// Valid values: string, int, bool, float, list, map.
	Type InputType `yaml:"type" json:"type"`

	// Required indicates whether this input must be provided.
	Required bool `yaml:"required,omitempty" json:"required,omitempty"`

	// Default provides a default value if the input is not supplied.
	Default interface{} `yaml:"default,omitempty" json:"default,omitempty"`

	// Description provides a human-readable explanation of the input.
	Description string `yaml:"description,omitempty" json:"description,omitempty"`

	// Validation specifies optional validation rules.
	Validation *InputValidation `yaml:"validation,omitempty" json:"validation,omitempty"`
}

// InputType represents the data type of an input parameter.
type InputType string

// Input type constants.
const (
	InputTypeString InputType = "string"
	InputTypeInt    InputType = "int"
	InputTypeBool   InputType = "bool"
	InputTypeFloat  InputType = "float"
	InputTypeList   InputType = "list"
	InputTypeMap    InputType = "map"
)

// InputValidation specifies validation rules for input values.
type InputValidation struct {
	// Pattern is a regex pattern for string validation.
	Pattern string `yaml:"pattern,omitempty" json:"pattern,omitempty"`

	// Min is the minimum value for numeric types or minimum length for strings/lists.
	Min *float64 `yaml:"min,omitempty" json:"min,omitempty"`

	// Max is the maximum value for numeric types or maximum length for strings/lists.
	Max *float64 `yaml:"max,omitempty" json:"max,omitempty"`

	// Enum restricts the value to one of the specified options.
	Enum []interface{} `yaml:"enum,omitempty" json:"enum,omitempty"`
}

// ExecutionState represents the current state of a runbook execution.
type ExecutionState string

// Execution state constants.
const (
	ExecutionStatePending   ExecutionState = "pending"
	ExecutionStateRunning   ExecutionState = "running"
	ExecutionStateCompleted ExecutionState = "completed"
	ExecutionStateFailed    ExecutionState = "failed"
	ExecutionStateCancelled ExecutionState = "cancelled"
)

// Execution represents a single execution instance of a runbook.
type Execution struct {
	// ID is the unique identifier for this execution.
	ID string `json:"id"`

	// RunbookName is the name of the runbook being executed.
	RunbookName string `json:"runbookName"`

	// RunbookVersion is the version of the runbook being executed.
	RunbookVersion string `json:"runbookVersion,omitempty"`

	// State is the current execution state.
	State ExecutionState `json:"state"`

	// Inputs contains the input values provided for this execution.
	Inputs map[string]interface{} `json:"inputs,omitempty"`

	// Outputs contains the aggregated outputs from all steps.
	Outputs map[string]interface{} `json:"outputs,omitempty"`

	// StartedAt is when execution began.
	StartedAt *time.Time `json:"startedAt,omitempty"`

	// CompletedAt is when execution finished.
	CompletedAt *time.Time `json:"completedAt,omitempty"`

	// Error contains error details if execution failed.
	Error string `json:"error,omitempty"`

	// Steps contains the execution state of each step.
	Steps map[string]*StepExecution `json:"steps,omitempty"`

	// CreatedAt is when this execution record was created.
	CreatedAt time.Time `json:"createdAt"`
}

// StepExecution represents the execution state of a single step.
type StepExecution struct {
	// Name is the step name.
	Name string `json:"name"`

	// Type is the step type.
	Type StepType `json:"type"`

	// State is the current step execution state.
	State StepState `json:"state"`

	// Inputs contains the resolved input values for this step.
	Inputs map[string]interface{} `json:"inputs,omitempty"`

	// Outputs contains the outputs produced by this step.
	Outputs map[string]interface{} `json:"outputs,omitempty"`

	// StartedAt is when step execution began.
	StartedAt *time.Time `json:"startedAt,omitempty"`

	// CompletedAt is when step execution finished.
	CompletedAt *time.Time `json:"completedAt,omitempty"`

	// Error contains error details if step failed.
	Error string `json:"error,omitempty"`

	// RetryCount is the number of retries attempted.
	RetryCount int `json:"retryCount"`

	// Duration is how long the step took to execute.
	Duration time.Duration `json:"duration,omitempty"`
}

// IsTerminal returns true if the execution state is terminal (completed, failed, or cancelled).
func (s ExecutionState) IsTerminal() bool {
	return s == ExecutionStateCompleted || s == ExecutionStateFailed || s == ExecutionStateCancelled
}

// String returns the string representation of the execution state.
func (s ExecutionState) String() string {
	return string(s)
}

// IsValid returns true if the input type is a recognized type.
func (t InputType) IsValid() bool {
	switch t {
	case InputTypeString, InputTypeInt, InputTypeBool, InputTypeFloat, InputTypeList, InputTypeMap:
		return true
	}
	return false
}

// String returns the string representation of the input type.
func (t InputType) String() string {
	return string(t)
}
