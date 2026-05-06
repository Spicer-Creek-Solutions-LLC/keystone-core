package state

import (
	"context"
	"time"
)

// PostgreSQLStore is the lib/pq-backed Store implementation.
//
// Stub for epic 02 task 1: every method returns ErrNotImplemented.
// Real implementation lands in epic 02 task 4 (auto-DDL, CRUD, JSON
// column handling, IPv6-safe DSN) per epics/02-storage-layer.md.
type PostgreSQLStore struct {
	cfg *Config
}

// newPostgreSQLStore constructs a PostgreSQLStore from a validated,
// defaulted Config. Called by NewStore.
func newPostgreSQLStore(cfg *Config) (*PostgreSQLStore, error) {
	return &PostgreSQLStore{cfg: cfg}, nil
}

func (s *PostgreSQLStore) Close() error { return nil }

func (s *PostgreSQLStore) Ping(_ context.Context) error { return ErrNotImplemented }

// AgentStore

func (s *PostgreSQLStore) CreateAgent(_ context.Context, _ *AgentRecord) error {
	return ErrNotImplemented
}
func (s *PostgreSQLStore) GetAgent(_ context.Context, _ string) (*AgentRecord, error) {
	return nil, ErrNotImplemented
}
func (s *PostgreSQLStore) ListAgents(_ context.Context, _ AgentFilter) ([]*AgentRecord, error) {
	return nil, ErrNotImplemented
}
func (s *PostgreSQLStore) UpdateAgent(_ context.Context, _ *AgentRecord) error {
	return ErrNotImplemented
}
func (s *PostgreSQLStore) UpdateAgentHeartbeat(_ context.Context, _ string, _ time.Time) error {
	return ErrNotImplemented
}
func (s *PostgreSQLStore) UpdateAgentStatus(_ context.Context, _ string, _ AgentStatus) error {
	return ErrNotImplemented
}
func (s *PostgreSQLStore) DeleteAgent(_ context.Context, _ string) error {
	return ErrNotImplemented
}

// CommandStore

func (s *PostgreSQLStore) CreateCommand(_ context.Context, _ *CommandRecord) error {
	return ErrNotImplemented
}
func (s *PostgreSQLStore) GetCommand(_ context.Context, _ string) (*CommandRecord, error) {
	return nil, ErrNotImplemented
}
func (s *PostgreSQLStore) ListCommands(_ context.Context, _ CommandFilter) ([]*CommandRecord, error) {
	return nil, ErrNotImplemented
}
func (s *PostgreSQLStore) UpdateCommandResult(_ context.Context, _ string, _ CommandResult) error {
	return ErrNotImplemented
}

// BatchJobStore

func (s *PostgreSQLStore) CreateBatchJob(_ context.Context, _ *BatchJobRecord) error {
	return ErrNotImplemented
}
func (s *PostgreSQLStore) GetBatchJob(_ context.Context, _ string) (*BatchJobRecord, error) {
	return nil, ErrNotImplemented
}
func (s *PostgreSQLStore) ListBatchJobs(_ context.Context, _ BatchJobFilter) ([]*BatchJobRecord, error) {
	return nil, ErrNotImplemented
}
func (s *PostgreSQLStore) UpdateBatchJobCounts(_ context.Context, _ string, _, _, _ int) error {
	return ErrNotImplemented
}
func (s *PostgreSQLStore) CreateBatchAgentResult(_ context.Context, _ *BatchAgentResultRecord) error {
	return ErrNotImplemented
}
func (s *PostgreSQLStore) ListBatchAgentResults(_ context.Context, _ string) ([]*BatchAgentResultRecord, error) {
	return nil, ErrNotImplemented
}
