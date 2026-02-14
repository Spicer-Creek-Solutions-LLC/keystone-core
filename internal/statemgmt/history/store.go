// Package history provides persistent storage for state application history and status.
package history

import (
	"context"
	"time"
)

// StateRunRecord represents a historical state application run.
type StateRunRecord struct {
	RunID         string
	AgentID       string
	StateFiles    []string
	Target        string
	DryRun        bool
	Success       bool
	Total         int
	Succeeded     int
	Failed        int
	Changed       int
	Unchanged     int
	StartTime     time.Time
	EndTime       time.Time
	DurationMs    int64
	CorrelationID string
	User          string
}

// StateStatusRecord represents the current status of a state on an agent.
type StateStatusRecord struct {
	AgentID      string
	StateID      string
	Module       string
	CurrentState string
	DesiredState string
	Compliant    bool
	LastApplied  time.Time
	LastChecked  time.Time
}

// ListFilter specifies filters for listing state runs.
type ListFilter struct {
	AgentID   string
	StatePath string
	Success   *bool
	StartTime *time.Time
	EndTime   *time.Time
	PageSize  int
	PageToken string
}

// Store provides persistent storage for state history and status.
type Store interface {
	SaveRun(ctx context.Context, run *StateRunRecord) error
	ListRuns(ctx context.Context, filter *ListFilter) ([]*StateRunRecord, string, error)
	GetStatus(ctx context.Context, agentID, statePath string) ([]*StateStatusRecord, error)
	SaveStatus(ctx context.Context, status *StateStatusRecord) error
	Close() error
}
