// Package saga provides a generic saga coordinator for multi-step workflows
// with compensating transactions and persistent execution logging.
//
// A saga breaks a complex operation into discrete steps, each with an action
// and an optional compensating action (rollback). If any step fails, previously
// completed steps are compensated in reverse order.
//
// # Usage
//
//	coord := saga.New("upgrade",
//	    saga.WithLog(saga.NewMemoryLog()),
//	)
//	coord.AddStep(saga.Step{
//	    Name:   "backup",
//	    Action: func(ctx context.Context, data map[string]any) error { ... },
//	    Compensate: func(ctx context.Context, data map[string]any) error { ... },
//	})
//	exec, err := coord.Execute(ctx, "run-1", nil)
package saga

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Status represents the overall status of a saga execution.
type Status string

// Saga execution statuses.
const (
	StatusPending      Status = "pending"
	StatusRunning      Status = "running"
	StatusCompleted    Status = "completed"
	StatusCompensating Status = "compensating"
	StatusCompensated  Status = "compensated"
	StatusFailed       Status = "failed"
)

// StepStatus represents the status of an individual step.
type StepStatus string

// Step execution statuses.
const (
	StepPending     StepStatus = "pending"
	StepRunning     StepStatus = "running"
	StepCompleted   StepStatus = "completed"
	StepFailed      StepStatus = "failed"
	StepCompensated StepStatus = "compensated"
	StepSkipped     StepStatus = "skipped"
)

// Step defines a single saga step with an action and optional compensation.
type Step struct {
	Name       string
	Action     func(ctx context.Context, data map[string]any) error
	Compensate func(ctx context.Context, data map[string]any) error
}

// StepRecord tracks the execution status of a step.
type StepRecord struct {
	Name        string     `json:"name"`
	Status      StepStatus `json:"status"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	Error       string     `json:"error,omitempty"`
}

// Execution represents a saga execution instance.
type Execution struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Status    Status         `json:"status"`
	Data      map[string]any `json:"data,omitempty"`
	Steps     []StepRecord   `json:"steps"`
	StartedAt time.Time      `json:"started_at"`
	EndedAt   *time.Time     `json:"ended_at,omitempty"`
	Error     string         `json:"error,omitempty"`
}

// Log persists saga executions for crash recovery and auditing.
type Log interface {
	Save(ctx context.Context, exec *Execution) error
	Get(ctx context.Context, id string) (*Execution, error)
	List(ctx context.Context, name string, limit int) ([]*Execution, error)
}

// Option configures a Coordinator.
type Option func(*Coordinator)

// WithLog sets the persistence log for the coordinator.
func WithLog(l Log) Option {
	return func(c *Coordinator) {
		c.log = l
	}
}

// Coordinator orchestrates saga execution.
type Coordinator struct {
	name  string
	steps []Step
	log   Log
}

// New creates a new saga coordinator with the given name.
func New(name string, opts ...Option) *Coordinator {
	c := &Coordinator{name: name}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// AddStep appends a step to the saga.
func (c *Coordinator) AddStep(s Step) *Coordinator {
	c.steps = append(c.steps, s)
	return c
}

// Execute runs the saga forward. On step failure, completed steps are
// compensated in reverse order. The execution is persisted after each
// step transition when a Log is configured.
func (c *Coordinator) Execute(ctx context.Context, id string, data map[string]any) (*Execution, error) {
	if data == nil {
		data = make(map[string]any)
	}

	exec := &Execution{
		ID:        id,
		Name:      c.name,
		Status:    StatusRunning,
		Data:      data,
		Steps:     make([]StepRecord, len(c.steps)),
		StartedAt: time.Now(),
	}
	for i, s := range c.steps {
		exec.Steps[i] = StepRecord{Name: s.Name, Status: StepPending}
	}

	if err := c.save(ctx, exec); err != nil {
		return exec, fmt.Errorf("saga: save initial: %w", err)
	}

	return c.run(ctx, exec, 0)
}

// Resume continues a previously started execution from the last pending step.
func (c *Coordinator) Resume(ctx context.Context, id string) (*Execution, error) {
	if c.log == nil {
		return nil, errors.New("saga: resume requires a Log")
	}

	exec, err := c.log.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("saga: load execution: %w", err)
	}
	if exec == nil {
		return nil, fmt.Errorf("saga: execution %q not found", id)
	}

	if exec.Status == StatusCompleted || exec.Status == StatusCompensated || exec.Status == StatusFailed {
		return exec, nil
	}

	// Find the first non-completed step.
	startIdx := 0
	for i, sr := range exec.Steps {
		if sr.Status == StepCompleted {
			startIdx = i + 1
			continue
		}
		break
	}

	exec.Status = StatusRunning
	return c.run(ctx, exec, startIdx)
}

func (c *Coordinator) run(ctx context.Context, exec *Execution, startIdx int) (*Execution, error) {
	for i := startIdx; i < len(c.steps); i++ {
		if err := ctx.Err(); err != nil {
			exec.Status = StatusFailed
			exec.Error = err.Error()
			now := time.Now()
			exec.EndedAt = &now
			_ = c.save(ctx, exec)
			return exec, err
		}

		step := c.steps[i]
		now := time.Now()
		exec.Steps[i].Status = StepRunning
		exec.Steps[i].StartedAt = &now
		_ = c.save(ctx, exec)

		if err := step.Action(ctx, exec.Data); err != nil {
			exec.Steps[i].Status = StepFailed
			exec.Steps[i].Error = err.Error()
			completedAt := time.Now()
			exec.Steps[i].CompletedAt = &completedAt

			return c.compensate(ctx, exec, i-1, err)
		}

		completedAt := time.Now()
		exec.Steps[i].Status = StepCompleted
		exec.Steps[i].CompletedAt = &completedAt
		_ = c.save(ctx, exec)
	}

	exec.Status = StatusCompleted
	now := time.Now()
	exec.EndedAt = &now
	_ = c.save(ctx, exec)

	return exec, nil
}

func (c *Coordinator) compensate(ctx context.Context, exec *Execution, lastCompleted int, originalErr error) (*Execution, error) {
	exec.Status = StatusCompensating
	exec.Error = originalErr.Error()
	_ = c.save(ctx, exec)

	// Mark remaining steps as skipped.
	for j := lastCompleted + 2; j < len(exec.Steps); j++ {
		exec.Steps[j].Status = StepSkipped
	}

	var compensateErr error
	for i := lastCompleted; i >= 0; i-- {
		step := c.steps[i]
		if step.Compensate == nil {
			continue
		}

		if err := step.Compensate(ctx, exec.Data); err != nil {
			compensateErr = errors.Join(compensateErr, fmt.Errorf("compensate %q: %w", step.Name, err))
			continue
		}
		exec.Steps[i].Status = StepCompensated
		_ = c.save(ctx, exec)
	}

	now := time.Now()
	exec.EndedAt = &now
	if compensateErr != nil {
		exec.Status = StatusFailed
		exec.Error = errors.Join(originalErr, compensateErr).Error()
		_ = c.save(ctx, exec)
		return exec, errors.Join(originalErr, compensateErr)
	}

	exec.Status = StatusCompensated
	_ = c.save(ctx, exec)
	return exec, originalErr
}

func (c *Coordinator) save(ctx context.Context, exec *Execution) error {
	if c.log == nil {
		return nil
	}
	return c.log.Save(ctx, exec)
}
