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
}

// BatchJobStore manages batch (multi-agent) command jobs and their
// per-agent results.
type BatchJobStore interface {
	CreateBatchJob(ctx context.Context, b *BatchJobRecord) error
	GetBatchJob(ctx context.Context, id string) (*BatchJobRecord, error)
	ListBatchJobs(ctx context.Context, filter BatchJobFilter) ([]*BatchJobRecord, error)
	UpdateBatchJobCounts(ctx context.Context, id string, completed, successful, failed int) error
	CreateBatchAgentResult(ctx context.Context, r *BatchAgentResultRecord) error
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
	HealthStore

	// Close releases resources held by the backend. Safe to call on a
	// half-initialized Store.
	Close() error
}
