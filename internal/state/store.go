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

// JoinTokenStore manages cluster-join token records used by the
// identity provider's join-token attestor (Epic 09 task 8) +
// CreateJoinToken / ListJoinTokens / DeleteJoinToken methods
// (Epic 09 task 10). Background cleanup (Epic 09 task 11) calls
// DeleteExpiredJoinTokens hourly on the cluster leader.
//
// Storage MUST persist only Hash + Salt — never cleartext Token.
// LookupJoinTokenByPrefix narrows the search; the caller (the
// attestor) MUST verify the salted hash before trusting the record.
//
// MarkJoinTokenUsed MUST be atomic — concurrent attestation attempts
// against a max-uses=1 token MUST see exactly one success.
type JoinTokenStore interface {
	CreateJoinToken(ctx context.Context, r *JoinTokenRecord) error
	GetJoinToken(ctx context.Context, id string) (*JoinTokenRecord, error)
	LookupJoinTokenByPrefix(ctx context.Context, prefix string) (*JoinTokenRecord, error)
	ListJoinTokens(ctx context.Context, filter JoinTokenFilter) ([]*JoinTokenRecord, error)
	MarkJoinTokenUsed(ctx context.Context, id string, now time.Time) error
	DeleteJoinToken(ctx context.Context, id string) error

	// DeleteExpiredJoinTokens removes every token whose ExpiresAt is
	// at or before `before`. Returns the number of records removed.
	// Hourly cleanup loop (Epic 09 task 11) is the production caller.
	DeleteExpiredJoinTokens(ctx context.Context, before time.Time) (int, error)
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

// LeaseStore manages the persistent record of tracked dynamic-secret
// leases per PROJECT-DETAILS §4.11. Epic 10 task 6's LeaseManager wraps
// this store + a background scheduler + lifecycle callbacks.
//
// CreateLease wraps state.ErrDuplicate on a PRIMARY KEY collision so
// callers branch with errors.Is. UpdateLease is a full-row replace
// (the scheduler stamps every field at once after a renewal); it
// returns state.ErrNotFound when no row matches the supplied ID.
// DeleteExpiredLeases is the operator-facing retention hook; the
// LeaseManager's lifecycle loop fires it on a configurable cadence.
type LeaseStore interface {
	CreateLease(ctx context.Context, r *LeaseStoreRecord) error
	GetLease(ctx context.Context, id string) (*LeaseStoreRecord, error)
	UpdateLease(ctx context.Context, r *LeaseStoreRecord) error
	ListLeases(ctx context.Context, filter LeaseFilter) ([]*LeaseStoreRecord, error)
	DeleteLease(ctx context.Context, id string) error

	// DeleteExpiredLeases removes every lease whose ExpiresAt is at
	// or before `before` AND whose State is not "active" (the
	// scheduler may still be actively trying to renew an
	// up-against-the-wire active lease). Returns the number of
	// records removed.
	DeleteExpiredLeases(ctx context.Context, before time.Time) (int, error)
}

// EventStore manages the persistent record of emitted events per
// PROJECT-DETAILS §4.9. Epic 11 task 3's JetStreamPublisher fans every
// successful publish through CreateEvent (or CreateEventsBatch for
// buffered emit); the retention enforcer (Epic 11 task 8) calls
// ApplyEventsRetention hourly on the cluster leader once Epic 13
// leader election lands.
//
// CreateEvent wraps state.ErrDuplicate on a PRIMARY KEY collision so
// callers branch with errors.Is — re-emitting the same event ID
// against an at-least-once delivery is the canonical retry case.
//
// CreateEventsBatch is atomic: either every record lands or none do
// (SQLite via transaction; Postgres via single multi-row INSERT).
// All-or-nothing keeps the publisher's idempotency story simple — a
// failed batch is retried whole.
//
// ListEvents pages through events in id order (k-sortable UUIDv7 ==
// time-order). Cursor is the last seen id; first page passes "".
//
// ApplyEventsRetention applies every policy in order, returning the
// total number of rows deleted. Per-type policies remove rows of the
// matching type that are either older than MaxAge OR (after applying
// MaxAge) beyond the newest MaxCount. A policy with empty Type is the
// catch-all; targeted policies take effect in addition to (not instead
// of) the catch-all because retention is monotonic — a row deleted by
// one policy is gone for subsequent policies.
type EventStore interface {
	CreateEvent(ctx context.Context, r *EventStoreRecord) error
	CreateEventsBatch(ctx context.Context, recs []*EventStoreRecord) error
	GetEvent(ctx context.Context, id string) (*EventStoreRecord, error)
	ListEvents(ctx context.Context, filter EventFilter) ([]*EventStoreRecord, error)
	CountEvents(ctx context.Context, filter EventFilter) (int, error)
	DeleteEvent(ctx context.Context, id string) error

	// ApplyEventsRetention applies the given policies and returns the
	// number of rows removed. An empty policy slice is a no-op
	// returning (0, nil). The reference time is the wall clock at call
	// time; callers wanting reproducible cutoffs should freeze time at
	// the policy layer (Epic 11 task 8).
	ApplyEventsRetention(ctx context.Context, policies []EventsRetentionPolicy) (int, error)
}

// Store is the root persistence interface composing all v1.0 sub-interfaces.
//
// Other domains add their own sub-interfaces in their respective epics
// (SecretsStore §4.11, AuditStore §4.12, PolicyStore §4.12,
// ClusterStore §4.13). Each owning epic appends its sub-interface to
// this composition.
type Store interface {
	AgentStore
	CommandStore
	BatchJobStore
	APIKeyStore
	JoinTokenStore
	LeaseStore
	EventStore
	StateHistoryStore
	HealthStore

	// Close releases resources held by the backend. Safe to call on a
	// half-initialized Store.
	Close() error
}
