package state

import (
	"context"
	"time"
)

// AgentStore manages the agent registry.
type AgentStore interface {
	CreateAgent(ctx context.Context, a *AgentRecord) error
	GetAgent(ctx context.Context, id string) (*AgentRecord, error)
	ListAgents(ctx context.Context, filter AgentFilter) ([]*AgentRecord, error)
	UpdateAgent(ctx context.Context, a *AgentRecord) error
	UpdateAgentHeartbeat(ctx context.Context, id string, t time.Time) error
	UpdateAgentStatus(ctx context.Context, id string, status AgentStatus) error
	DeleteAgent(ctx context.Context, id string) error
}

// CommandStore manages individual command executions.
type CommandStore interface {
	CreateCommand(ctx context.Context, c *CommandRecord) error
	GetCommand(ctx context.Context, id string) (*CommandRecord, error)
	ListCommands(ctx context.Context, filter CommandFilter) ([]*CommandRecord, error)
	UpdateCommandResult(ctx context.Context, id string, result CommandResult) error

	// DeleteCommandsBefore removes terminal command rows whose
	// CompletedAt is strictly older than cutoff and whose status is in
	// the supplied allowlist. Returns the number of rows removed.
	//
	// statuses MUST be non-empty — empty would imply "delete every row
	// older than cutoff regardless of status," which would silently
	// drop pending/running rows during retention sweeps. Backends
	// reject the empty case.
	//
	// Rows whose CompletedAt is the zero value (never finalized) are
	// never matched. Used by controlplane.CommandDispatcher's
	// retention loop (see PROJECT-DETAILS §4.4 step 7).
	DeleteCommandsBefore(ctx context.Context, cutoff time.Time, statuses []CommandStatus) (deleted int, err error)
}

// BatchJobStore manages batch (multi-agent) command jobs and their
// per-agent results.
type BatchJobStore interface {
	CreateBatchJob(ctx context.Context, b *BatchJobRecord) error
	GetBatchJob(ctx context.Context, id string) (*BatchJobRecord, error)
	ListBatchJobs(ctx context.Context, filter BatchJobFilter) ([]*BatchJobRecord, error)
	UpdateBatchJobCounts(ctx context.Context, id string, completed, successful, failed int) error

	// MarkBatchJobRunning sets status='running' and started_at=startedAt.
	// Used by controlplane.BatchDispatcher's state machine on the
	// pending → running transition. The storage layer does not enforce
	// the previous status — caller is responsible for guard rails.
	MarkBatchJobRunning(ctx context.Context, id string, startedAt time.Time) error

	// FinalizeBatchJob sets status to one of the terminal values
	// (completed, failed, partial, cancelled) and stamps completed_at.
	// As with MarkBatchJobRunning, the storage layer does not enforce
	// transitions; the dispatcher's state machine does.
	FinalizeBatchJob(ctx context.Context, id string, status BatchJobStatus, completedAt time.Time) error

	CreateBatchAgentResult(ctx context.Context, r *BatchAgentResultRecord) error
	GetBatchAgentResult(ctx context.Context, batchJobID, agentID string) (*BatchAgentResultRecord, error)
	ListBatchAgentResults(ctx context.Context, batchJobID string) ([]*BatchAgentResultRecord, error)
}

// HealthStore exposes connectivity health.
type HealthStore interface {
	Ping(ctx context.Context) error
}

// APIKeyStore manages API key records used by pkg/api/apikeys.
//
// GetAPIKeyByHash powers the auth verifier — callers SHA-256-hash the
// inbound cleartext, then look up the row by that hash. Storage holds
// only the hash; cleartext exists only at creation time and is
// returned exactly once.
type APIKeyStore interface {
	CreateAPIKey(ctx context.Context, k *APIKeyRecord) error
	GetAPIKey(ctx context.Context, id string) (*APIKeyRecord, error)
	GetAPIKeyByHash(ctx context.Context, keyHash string) (*APIKeyRecord, error)
	ListAPIKeys(ctx context.Context, filter APIKeyFilter) ([]*APIKeyRecord, error)
	UpdateAPIKeyLastUsed(ctx context.Context, id string, t time.Time) error
	DeleteAPIKey(ctx context.Context, id string) error
}

// StateHistoryStore persists the past-runs record used by
// `kscorectl state history` + `kscorectl state rollback`. PROJECT-
// DETAILS §4.8 specifies this sub-interface; Epic 08 task 8 ships
// the SQLite + Postgres implementations.
//
// CreateStateRun + FinalizeStateRun are split so a long-running
// apply can stream per-decl rows into state_run_results as they
// complete (AddStateRunResult per decl), then stamp terminal
// status + aggregates at the end. An interrupted run leaves
// partial-but-recoverable rows on disk.
type StateHistoryStore interface {
	// CreateStateRun inserts the header row. Caller sets ID
	// (typically UUID); StartedAt is recorded.
	CreateStateRun(ctx context.Context, r *StateRunRecord) error

	// FinalizeStateRun stamps the terminal Status, EndedAt,
	// ErrorMessage, and aggregate counters. Returns ErrNotFound
	// when id does not match a stored run. Callers MAY call
	// FinalizeStateRun more than once (e.g., on retry); the latest
	// call wins.
	FinalizeStateRun(ctx context.Context, id string, end StateRunEnd) error

	// AddStateRunResult inserts one per-declaration result row.
	// PRIMARY KEY (run_id, decl_id) guards against duplicates;
	// re-insertion of the same row returns an error.
	AddStateRunResult(ctx context.Context, runID string, r *StateRunResultRecord) error

	// GetStateRun returns the header plus all result rows for
	// runID. Results are ordered by started_at ASC (the runner's
	// execution order). Returns ErrNotFound when the header is
	// missing.
	GetStateRun(ctx context.Context, id string) (*StateRunRecord, []*StateRunResultRecord, error)

	// ListStateRuns returns headers only (NOT results) matching
	// filter. Result-row hydration is GetStateRun's job — the list
	// query stays fast for the CLI `state history` table view.
	ListStateRuns(ctx context.Context, filter StateRunFilter) ([]*StateRunRecord, error)

	// DeleteStateRunsBefore removes state_runs rows whose EndedAt
	// is strictly older than cutoff and whose Status is in
	// statuses. Cascading FK drops dependent state_run_results
	// rows.
	//
	// statuses MUST be non-empty — empty would imply "delete every
	// run older than cutoff regardless of status," which would
	// silently drop running rows during retention sweeps. Backends
	// reject the empty case. Rows whose EndedAt is the zero value
	// (still running) are never matched.
	DeleteStateRunsBefore(ctx context.Context, cutoff time.Time, statuses []StateRunStatus) (deleted int, err error)
}

// Store is the root persistence interface composing all v1.0 sub-interfaces.
//
// Other domains add their own sub-interfaces in their respective epics
// (EventStore §4.9, SecretsStore §4.11, AuditStore §4.12, PolicyStore
// §4.12, ClusterStore §4.13). Each owning epic appends its sub-interface
// to this composition.
type Store interface {
	AgentStore
	CommandStore
	BatchJobStore
	APIKeyStore
	StateHistoryStore
	HealthStore

	// Close releases resources held by the backend. Safe to call on a
	// half-initialized Store.
	Close() error
}
