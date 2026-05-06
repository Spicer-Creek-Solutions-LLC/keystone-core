package state

import (
	"context"
	"time"
)

// SQLiteStore is the modernc.org/sqlite-backed Store implementation.
//
// Stub for epic 02 task 1: every method returns ErrNotImplemented.
// Real implementation lands in epic 02 task 3 (auto-DDL, CRUD, JSON
// column handling) per epics/02-storage-layer.md.
type SQLiteStore struct {
	cfg *Config
}

// newSQLiteStore constructs a SQLiteStore from a validated, defaulted
// Config. Called by NewStore.
func newSQLiteStore(cfg *Config) (*SQLiteStore, error) {
	return &SQLiteStore{cfg: cfg}, nil
}

func (s *SQLiteStore) Close() error { return nil }

func (s *SQLiteStore) Ping(_ context.Context) error { return ErrNotImplemented }

// AgentStore

func (s *SQLiteStore) CreateAgent(_ context.Context, _ *AgentRecord) error {
	return ErrNotImplemented
}
func (s *SQLiteStore) GetAgent(_ context.Context, _ string) (*AgentRecord, error) {
	return nil, ErrNotImplemented
}
func (s *SQLiteStore) ListAgents(_ context.Context, _ AgentFilter) ([]*AgentRecord, error) {
	return nil, ErrNotImplemented
}
func (s *SQLiteStore) UpdateAgent(_ context.Context, _ *AgentRecord) error {
	return ErrNotImplemented
}
func (s *SQLiteStore) UpdateAgentHeartbeat(_ context.Context, _ string, _ time.Time) error {
	return ErrNotImplemented
}
func (s *SQLiteStore) UpdateAgentStatus(_ context.Context, _ string, _ AgentStatus) error {
	return ErrNotImplemented
}
func (s *SQLiteStore) DeleteAgent(_ context.Context, _ string) error {
	return ErrNotImplemented
}

// CommandStore

func (s *SQLiteStore) CreateCommand(_ context.Context, _ *CommandRecord) error {
	return ErrNotImplemented
}
func (s *SQLiteStore) GetCommand(_ context.Context, _ string) (*CommandRecord, error) {
	return nil, ErrNotImplemented
}
func (s *SQLiteStore) ListCommands(_ context.Context, _ CommandFilter) ([]*CommandRecord, error) {
	return nil, ErrNotImplemented
}
func (s *SQLiteStore) UpdateCommandResult(_ context.Context, _ string, _ CommandResult) error {
	return ErrNotImplemented
}

// BatchJobStore

func (s *SQLiteStore) CreateBatchJob(_ context.Context, _ *BatchJobRecord) error {
	return ErrNotImplemented
}
func (s *SQLiteStore) GetBatchJob(_ context.Context, _ string) (*BatchJobRecord, error) {
	return nil, ErrNotImplemented
}
func (s *SQLiteStore) ListBatchJobs(_ context.Context, _ BatchJobFilter) ([]*BatchJobRecord, error) {
	return nil, ErrNotImplemented
}
func (s *SQLiteStore) UpdateBatchJobCounts(_ context.Context, _ string, _, _, _ int) error {
	return ErrNotImplemented
}
func (s *SQLiteStore) CreateBatchAgentResult(_ context.Context, _ *BatchAgentResultRecord) error {
	return ErrNotImplemented
}
func (s *SQLiteStore) ListBatchAgentResults(_ context.Context, _ string) ([]*BatchAgentResultRecord, error) {
	return nil, ErrNotImplemented
}
