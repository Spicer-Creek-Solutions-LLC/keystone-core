// SPDX-License-Identifier: Apache-2.0

package runbook

import "time"

// Runbook is the parsed shape of a runbook.yaml.
type Runbook struct {
	Metadata Metadata `yaml:"metadata"`
	Spec     Spec     `yaml:"spec"`
}

// Metadata identifies a runbook.
type Metadata struct {
	Name        string            `yaml:"name"`
	Namespace   string            `yaml:"namespace"`
	Version     string            `yaml:"version"`
	Labels      map[string]string `yaml:"labels"`
	Annotations map[string]string `yaml:"annotations"`
}

// Spec is the runbook body.
//
// OnSuccess / OnFailure name steps run after the main DAG terminates
// (respectively when every step succeeded or when a step failed).
// Timeout / MaxRetries are strings ("30s") / ints applied as the
// per-step default when a Step does not set its own.
type Spec struct {
	Inputs     []InputSpec `yaml:"inputs"`
	Steps      []Step      `yaml:"steps"`
	OnSuccess  []string    `yaml:"on_success"`
	OnFailure  []string    `yaml:"on_failure"`
	Timeout    string      `yaml:"timeout"`
	MaxRetries int         `yaml:"max_retries"`
}

// InputSpec declares one runbook input.
type InputSpec struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Required    bool   `yaml:"required"`
	Default     any    `yaml:"default"`
	Sensitive   bool   `yaml:"sensitive"`
}

// Step is one node of the runbook DAG.
//
// Type selects the registered [StepExecutor]. DependsOn lists step
// names that must complete first. Condition, when non-empty, is
// rendered and must evaluate truthy for the step to run (otherwise
// it is skipped). Timeout ("30s") and Retries override the Spec
// defaults. Config is the executor-specific payload (templated
// before dispatch).
type Step struct {
	Type        string         `yaml:"type"`
	Name        string         `yaml:"name"`
	Description string         `yaml:"description"`
	DependsOn   []string       `yaml:"depends_on"`
	Condition   string         `yaml:"condition"`
	Timeout     string         `yaml:"timeout"`
	Retries     int            `yaml:"retries"`
	Config      map[string]any `yaml:"config"`
}

// Status is the terminal (or in-flight) classification of an
// Execution or a StepResult.
type Status string

const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
	StatusSkipped   Status = "skipped" // Condition was falsey
)

// StepResult records what happened to one step during an Execution.
// Attempts counts every dispatch (1 + retries). Output is the
// executor's returned outputs (templating source for later steps).
type StepResult struct {
	Name      string
	Type      string
	Status    Status
	Attempts  int
	StartedAt time.Time
	Duration  time.Duration
	Output    map[string]any
	Error     error
}

// TrailEntry is one audit-trail row: a status transition for the
// execution as a whole or for a named step.
type TrailEntry struct {
	At    time.Time
	Step  string // "" for execution-level transitions
	From  Status
	To    Status
	Note  string
}

// Execution is the run record for one runbook invocation. Steps is
// in declaration order; Trail is the chronological audit trail. Error
// is the first failing step's error (nil on success).
type Execution struct {
	ID        string
	Runbook   string
	Status    Status
	Inputs    map[string]any
	Steps     []StepResult
	Trail     []TrailEntry
	StartedAt time.Time
	EndedAt   time.Time
	Error     error
}

// StepView is the read-only projection of a completed step exposed to
// templates as `.steps.<name>`.
type StepView struct {
	Outputs map[string]any `json:"outputs"`
	Status  string         `json:"status"`
}
