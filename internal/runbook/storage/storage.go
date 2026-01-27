// Package storage provides persistence for runbook executions.
package storage

import (
	"context"
	"time"

	"github.com/shawnbutts/keystone-core/internal/runbook"
)

// Storage defines the interface for runbook execution persistence.
type Storage interface {
	// SaveExecution saves or updates an execution record.
	SaveExecution(ctx context.Context, exec *runbook.Execution) error

	// GetExecution retrieves an execution by ID.
	GetExecution(ctx context.Context, id string) (*runbook.Execution, error)

	// ListExecutions lists executions with optional filtering.
	ListExecutions(ctx context.Context, opts ListOptions) ([]*runbook.Execution, error)

	// DeleteExecution deletes an execution and its step records.
	DeleteExecution(ctx context.Context, id string) error

	// SaveStepExecution saves or updates a step execution record.
	SaveStepExecution(ctx context.Context, executionID string, step *runbook.StepExecution) error

	// GetStepExecution retrieves a step execution by execution ID and step name.
	GetStepExecution(ctx context.Context, executionID, stepName string) (*runbook.StepExecution, error)

	// ListStepExecutions lists all step executions for an execution.
	ListStepExecutions(ctx context.Context, executionID string) ([]*runbook.StepExecution, error)

	// Close closes the storage connection.
	Close() error
}

// ListOptions provides filtering options for listing executions.
type ListOptions struct {
	// RunbookName filters by runbook name.
	RunbookName string

	// State filters by execution state.
	State runbook.ExecutionState

	// Since filters to executions created after this time.
	Since *time.Time

	// Until filters to executions created before this time.
	Until *time.Time

	// Limit limits the number of results.
	Limit int

	// Offset skips this many results (for pagination).
	Offset int
}
