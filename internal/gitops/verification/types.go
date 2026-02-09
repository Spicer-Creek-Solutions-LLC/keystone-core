// Package verification provides deployment verification through HTTP, gRPC,
// Kubernetes, Prometheus, and custom health checks.
package verification

import "time"

// Type represents the type of verification to perform
type Type string

const (
	// TypeHTTP performs HTTP health checks
	TypeHTTP Type = "http_check"

	// TypeGRPC performs gRPC health checks
	TypeGRPC Type = "grpc_check"

	// TypeK8s checks Kubernetes resources
	TypeK8s Type = "k8s_check"

	// TypePrometheus queries Prometheus metrics
	TypePrometheus Type = "prometheus_query"

	// TypeLogs analyzes logs
	TypeLogs Type = "logs_check"

	// TypeCommand executes a command
	TypeCommand Type = "command"

	// TypeScript executes a custom script
	TypeScript Type = "script"
)

// Step represents a single verification step
type Step struct {
	// Name of the step
	Name string `json:"name"`

	// Type of verification
	Type Type `json:"type"`

	// Timeout for this step
	Timeout time.Duration `json:"timeout,omitempty"`

	// Retries allowed
	Retries int `json:"retries,omitempty"`

	// RetryDelay between retries
	RetryDelay time.Duration `json:"retry_delay,omitempty"`

	// Config contains step-specific configuration
	Config map[string]interface{} `json:"config"`

	// ContinueOnFailure continues workflow even if this step fails
	ContinueOnFailure bool `json:"continue_on_failure,omitempty"`
}

// Workflow represents a complete verification workflow
type Workflow struct {
	// Name of the workflow
	Name string `json:"name"`

	// Description
	Description string `json:"description,omitempty"`

	// Steps to execute
	Steps []*Step `json:"steps"`

	// Parallel indicates if steps should run in parallel
	Parallel bool `json:"parallel,omitempty"`

	// Timeout for entire workflow
	Timeout time.Duration `json:"timeout,omitempty"`

	// OnFailure actions
	OnFailure []string `json:"on_failure,omitempty"`
}

// Result represents the result of a verification step
type Result struct {
	// StepName is the name of the step
	StepName string `json:"step_name"`

	// Success indicates if the step passed
	Success bool `json:"success"`

	// Message contains result details
	Message string `json:"message,omitempty"`

	// Data contains step-specific result data
	Data map[string]interface{} `json:"data,omitempty"`

	// Duration of the step
	Duration time.Duration `json:"duration"`

	// Timestamp when the step was executed
	Timestamp time.Time `json:"timestamp"`

	// Error if the step failed
	Error error `json:"error,omitempty"`

	// Retries attempted
	Retries int `json:"retries,omitempty"`
}

// WorkflowResult represents the overall workflow result
type WorkflowResult struct {
	// WorkflowName is the name of the workflow
	WorkflowName string `json:"workflow_name"`

	// Success indicates if all required steps passed
	Success bool `json:"success"`

	// Steps contains individual step results
	Steps []*Result `json:"steps"`

	// Duration of entire workflow
	Duration time.Duration `json:"duration"`

	// StartTime when workflow started
	StartTime time.Time `json:"start_time"`

	// EndTime when workflow completed
	EndTime time.Time `json:"end_time"`

	// Summary statistics
	TotalSteps   int `json:"total_steps"`
	PassedSteps  int `json:"passed_steps"`
	FailedSteps  int `json:"failed_steps"`
	SkippedSteps int `json:"skipped_steps"`
}

// Verifier is the interface for verification modules
type Verifier interface {
	// Type returns the verification type this verifier handles
	Type() Type

	// Verify executes the verification
	Verify(step *Step) (*Result, error)
}
