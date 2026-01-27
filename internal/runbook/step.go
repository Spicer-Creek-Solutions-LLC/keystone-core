package runbook

import (
	"time"
)

// StepType represents the type of step to execute.
type StepType string

// Step type constants.
const (
	// StepTypeCommand executes a shell command or script.
	StepTypeCommand StepType = "command"

	// StepTypeAPI makes an HTTP API call.
	StepTypeAPI StepType = "api"

	// StepTypeNotification sends a notification.
	StepTypeNotification StepType = "notification"

	// StepTypeWait pauses execution for a duration or until a condition is met.
	StepTypeWait StepType = "wait"

	// StepTypeNoop performs no operation (useful for testing/placeholders).
	StepTypeNoop StepType = "noop"

	// StepTypeFail intentionally fails (useful for testing error handling).
	StepTypeFail StepType = "fail"

	// StepTypeState applies a Keystone state.
	StepTypeState StepType = "state"

	// StepTypeApproval waits for human approval (Phase 2).
	StepTypeApproval StepType = "approval"

	// StepTypeIf provides conditional branching (Phase 2).
	StepTypeIf StepType = "if"

	// StepTypeSwitch provides multi-way conditional branching (Phase 2).
	StepTypeSwitch StepType = "switch"

	// StepTypeLoop iterates over a collection (Phase 2).
	StepTypeLoop StepType = "loop"

	// StepTypeParallel executes steps in parallel (Phase 2).
	StepTypeParallel StepType = "parallel"

	// StepTypeSubRunbook invokes another runbook (Phase 3).
	StepTypeSubRunbook StepType = "runbook"

	// StepTypeScript executes embedded scripts (Phase 3).
	StepTypeScript StepType = "script"

	// StepTypeQuery queries data sources (Phase 4).
	StepTypeQuery StepType = "query"

	// StepTypeDeploy performs a GitOps deployment.
	StepTypeDeploy StepType = "deploy"

	// StepTypeRollback performs a deployment rollback.
	StepTypeRollback StepType = "rollback"

	// StepTypePrompt requests operator input (Phase 3).
	StepTypePrompt StepType = "prompt"

	// StepTypeWaitManual waits for manual confirmation (Phase 3).
	StepTypeWaitManual StepType = "wait_manual"

	// StepTypeConfirm requests yes/no confirmation (Phase 3).
	StepTypeConfirm StepType = "confirm"
)

// Step represents a single step in a runbook.
type Step struct {
	// Name is the unique identifier for this step within the runbook.
	Name string `yaml:"name" json:"name"`

	// Type specifies which step handler to use.
	Type StepType `yaml:"type" json:"type"`

	// Description provides a human-readable explanation of the step.
	Description string `yaml:"description,omitempty" json:"description,omitempty"`

	// DependsOn lists step names that must complete before this step runs.
	DependsOn []string `yaml:"dependsOn,omitempty" json:"dependsOn,omitempty"`

	// Condition is a Go template expression that determines if the step should run.
	// If the expression evaluates to false, the step is skipped.
	Condition string `yaml:"condition,omitempty" json:"condition,omitempty"`

	// Timeout specifies the maximum duration for this step.
	// Format: Go duration string (e.g., "30s", "5m").
	Timeout string `yaml:"timeout,omitempty" json:"timeout,omitempty"`

	// Retries configures retry behavior for this step.
	Retries *RetryConfig `yaml:"retries,omitempty" json:"retries,omitempty"`

	// Config contains step-type-specific configuration.
	Config map[string]interface{} `yaml:"config" json:"config"`

	// Outputs defines how to extract outputs from step execution.
	Outputs []OutputDef `yaml:"outputs,omitempty" json:"outputs,omitempty"`

	// ContinueOnError allows the runbook to continue even if this step fails.
	ContinueOnError bool `yaml:"continueOnError,omitempty" json:"continueOnError,omitempty"`
}

// RetryConfig specifies retry behavior for step execution.
type RetryConfig struct {
	// MaxAttempts is the maximum number of execution attempts (including the first).
	MaxAttempts int `yaml:"maxAttempts" json:"maxAttempts"`

	// Delay is the initial delay between retry attempts.
	// Format: Go duration string (e.g., "5s", "1m").
	Delay string `yaml:"delay,omitempty" json:"delay,omitempty"`

	// MaxDelay is the maximum delay between retry attempts when using backoff.
	// Format: Go duration string.
	MaxDelay string `yaml:"maxDelay,omitempty" json:"maxDelay,omitempty"`

	// Backoff specifies the backoff strategy.
	// Valid values: constant, linear, exponential.
	Backoff BackoffType `yaml:"backoff,omitempty" json:"backoff,omitempty"`

	// RetryOn specifies conditions that trigger a retry.
	// If empty, any failure triggers a retry.
	RetryOn []string `yaml:"retryOn,omitempty" json:"retryOn,omitempty"`
}

// BackoffType represents the type of backoff strategy for retries.
type BackoffType string

// Backoff type constants.
const (
	BackoffConstant    BackoffType = "constant"
	BackoffLinear      BackoffType = "linear"
	BackoffExponential BackoffType = "exponential"
)

// OutputDef defines how to extract outputs from step execution.
type OutputDef struct {
	// Name is the identifier for this output.
	Name string `yaml:"name" json:"name"`

	// Source specifies where to extract the value from.
	// Valid values: stdout, stderr, exitCode, json, header, body.
	Source OutputSource `yaml:"source" json:"source"`

	// Parser specifies how to parse the source value.
	// Valid values: raw, json, regex, line, jsonpath.
	Parser OutputParser `yaml:"parser,omitempty" json:"parser,omitempty"`

	// Path is the extraction path (JSONPath, regex pattern, or line number).
	Path string `yaml:"path,omitempty" json:"path,omitempty"`

	// Default is the value to use if extraction fails.
	Default interface{} `yaml:"default,omitempty" json:"default,omitempty"`
}

// OutputSource specifies where to extract output values from.
type OutputSource string

// Output source constants.
const (
	OutputSourceStdout   OutputSource = "stdout"
	OutputSourceStderr   OutputSource = "stderr"
	OutputSourceExitCode OutputSource = "exitCode"
	OutputSourceJSON     OutputSource = "json"
	OutputSourceHeader   OutputSource = "header"
	OutputSourceBody     OutputSource = "body"
)

// OutputParser specifies how to parse output values.
type OutputParser string

// Output parser constants.
const (
	OutputParserRaw      OutputParser = "raw"
	OutputParserJSON     OutputParser = "json"
	OutputParserRegex    OutputParser = "regex"
	OutputParserLine     OutputParser = "line"
	OutputParserJSONPath OutputParser = "jsonpath"
)

// StepState represents the current state of a step execution.
type StepState string

// Step state constants.
const (
	StepStatePending   StepState = "pending"
	StepStateRunning   StepState = "running"
	StepStateCompleted StepState = "completed"
	StepStateFailed    StepState = "failed"
	StepStateSkipped   StepState = "skipped"
)

// StepResult represents the result of executing a step.
type StepResult struct {
	// Success indicates whether the step completed successfully.
	Success bool `json:"success"`

	// Outputs contains the extracted output values.
	Outputs map[string]interface{} `json:"outputs,omitempty"`

	// Message provides additional context about the result.
	Message string `json:"message,omitempty"`

	// Duration is how long the step took to execute.
	Duration time.Duration `json:"duration"`

	// ExitCode is the exit code for command steps.
	ExitCode int `json:"exitCode,omitempty"`

	// Stdout is the standard output for command steps.
	Stdout string `json:"stdout,omitempty"`

	// Stderr is the standard error for command steps.
	Stderr string `json:"stderr,omitempty"`

	// Response contains HTTP response data for API steps.
	Response *APIResponse `json:"response,omitempty"`
}

// APIResponse contains HTTP response data from API steps.
type APIResponse struct {
	StatusCode int               `json:"statusCode"`
	Headers    map[string]string `json:"headers,omitempty"`
	Body       string            `json:"body,omitempty"`
}

// IsValid returns true if the step type is a recognized Phase 1, 2, 3, or 4 type.
func (t StepType) IsValid() bool {
	switch t {
	case StepTypeCommand, StepTypeAPI, StepTypeNotification,
		StepTypeWait, StepTypeNoop, StepTypeFail,
		StepTypeIf, StepTypeSwitch, StepTypeLoop, StepTypeParallel,
		StepTypeSubRunbook, StepTypeApproval,
		StepTypePrompt, StepTypeWaitManual, StepTypeConfirm,
		StepTypeState, StepTypeDeploy, StepTypeRollback,
		StepTypeScript:
		return true
	}
	return false
}

// IsValidExtended returns true if the step type is any recognized type.
func (t StepType) IsValidExtended() bool {
	switch t {
	case StepTypeCommand, StepTypeAPI, StepTypeNotification,
		StepTypeWait, StepTypeNoop, StepTypeFail, StepTypeState,
		StepTypeApproval, StepTypeIf, StepTypeSwitch, StepTypeLoop,
		StepTypeParallel, StepTypeSubRunbook, StepTypeScript, StepTypeQuery,
		StepTypePrompt, StepTypeWaitManual, StepTypeConfirm,
		StepTypeDeploy, StepTypeRollback:
		return true
	}
	return false
}

// String returns the string representation of the step type.
func (t StepType) String() string {
	return string(t)
}

// IsTerminal returns true if the step state is terminal (completed, failed, or skipped).
func (s StepState) IsTerminal() bool {
	return s == StepStateCompleted || s == StepStateFailed || s == StepStateSkipped
}

// String returns the string representation of the step state.
func (s StepState) String() string {
	return string(s)
}

// IsValid returns true if the backoff type is a recognized type.
func (t BackoffType) IsValid() bool {
	switch t {
	case BackoffConstant, BackoffLinear, BackoffExponential, "":
		return true
	}
	return false
}

// String returns the string representation of the backoff type.
func (t BackoffType) String() string {
	if t == "" {
		return string(BackoffConstant)
	}
	return string(t)
}

// IsValid returns true if the output source is a recognized source.
func (s OutputSource) IsValid() bool {
	switch s {
	case OutputSourceStdout, OutputSourceStderr, OutputSourceExitCode,
		OutputSourceJSON, OutputSourceHeader, OutputSourceBody:
		return true
	}
	return false
}

// String returns the string representation of the output source.
func (s OutputSource) String() string {
	return string(s)
}

// IsValid returns true if the output parser is a recognized parser.
func (p OutputParser) IsValid() bool {
	switch p {
	case OutputParserRaw, OutputParserJSON, OutputParserRegex,
		OutputParserLine, OutputParserJSONPath, "":
		return true
	}
	return false
}

// String returns the string representation of the output parser.
func (p OutputParser) String() string {
	if p == "" {
		return string(OutputParserRaw)
	}
	return string(p)
}

// GetDelay returns the retry delay as a time.Duration.
// Returns 0 if the delay is not specified or invalid.
func (r *RetryConfig) GetDelay() time.Duration {
	if r == nil || r.Delay == "" {
		return 0
	}
	d, err := time.ParseDuration(r.Delay)
	if err != nil {
		return 0
	}
	return d
}

// GetMaxDelay returns the maximum retry delay as a time.Duration.
// Returns 0 if the max delay is not specified or invalid.
func (r *RetryConfig) GetMaxDelay() time.Duration {
	if r == nil || r.MaxDelay == "" {
		return 0
	}
	d, err := time.ParseDuration(r.MaxDelay)
	if err != nil {
		return 0
	}
	return d
}

// GetTimeout returns the step timeout as a time.Duration.
// Returns 0 if the timeout is not specified or invalid.
func (s *Step) GetTimeout() time.Duration {
	if s.Timeout == "" {
		return 0
	}
	d, err := time.ParseDuration(s.Timeout)
	if err != nil {
		return 0
	}
	return d
}
