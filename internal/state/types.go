// SPDX-License-Identifier: Apache-2.0

package state

import (
	"database/sql"
	"time"
)

// ---- Status enums ---------------------------------------------------------

// AgentStatus tracks an agent's lifecycle in the control plane.
//
// Intended transitions (enforced by the control plane / agent lifecycle
// manager, NOT by the storage layer):
//
//	pending --(register OK)--> connected
//	connected --(no heartbeat)--> stale
//	stale --(heartbeat resumes)--> connected
//	* --(operator action)--> disabled
//	disabled --(re-enable)--> pending
//
// The storage layer treats each value as opaque and does not enforce
// transitions; callers are responsible for supplying a valid next state.
// A future pkg/statemachine library will guard transitions at the
// caller's layer (likely epic 04 control-plane core).
type AgentStatus string

const (
	AgentStatusPending   AgentStatus = "pending"
	AgentStatusConnected AgentStatus = "connected"
	AgentStatusStale     AgentStatus = "stale"
	AgentStatusDisabled  AgentStatus = "disabled"
)

// CommandStatus tracks a single command execution.
//
// Intended transitions:
//
//	pending --(dispatched)--> running
//	running --> completed | failed | timeout | cancelled
type CommandStatus string

const (
	CommandStatusPending   CommandStatus = "pending"
	CommandStatusRunning   CommandStatus = "running"
	CommandStatusCompleted CommandStatus = "completed"
	CommandStatusFailed    CommandStatus = "failed"
	CommandStatusTimeout   CommandStatus = "timeout"
	CommandStatusCancelled CommandStatus = "cancelled"
)

// BatchJobStatus tracks a batch (multi-agent) command job.
//
// Intended transitions mirror CommandStatus, with `partial` denoting
// "some agents succeeded, some failed; job is closed."
type BatchJobStatus string

const (
	BatchJobStatusPending   BatchJobStatus = "pending"
	BatchJobStatusRunning   BatchJobStatus = "running"
	BatchJobStatusCompleted BatchJobStatus = "completed"
	BatchJobStatusFailed    BatchJobStatus = "failed"
	BatchJobStatusPartial   BatchJobStatus = "partial"
	BatchJobStatusCancelled BatchJobStatus = "cancelled"
)

// isTerminalBatchStatus reports whether s is one of the terminal
// BatchJobStatus values (completed, failed, partial, cancelled).
// Used by FinalizeBatchJob to refuse non-terminal targets.
func isTerminalBatchStatus(s BatchJobStatus) bool {
	switch s {
	case BatchJobStatusCompleted, BatchJobStatusFailed,
		BatchJobStatusPartial, BatchJobStatusCancelled:
		return true
	}
	return false
}

// ---- Records --------------------------------------------------------------

// AgentRecord is the persistent shape of a registered agent.
// Maps to the `agents` table.
type AgentRecord struct {
	ID              string
	Hostname        string
	OS              string
	Architecture    string
	IPAddresses     []string
	PlatformVersion string
	AgentVersion    string
	Labels          map[string]string
	Status          AgentStatus
	RegisteredAt    time.Time
	LastHeartbeatAt time.Time
	Metrics         map[string]any
}

// CommandRecord is the persistent shape of a single command execution.
// Maps to the `commands` table.
//
// User vs Principal: User is the OS-level user the command runs AS on
// the target host (sudo-as-someone); Principal is the operator/SPIFFE
// identity that requested the dispatch (the audit-log actor). The two
// are unrelated — e.g., an operator alice@example.com may dispatch a
// command that runs as root on the host.
type CommandRecord struct {
	ID             string
	AgentID        string
	Command        string
	Args           []string
	Env            map[string]string
	WorkingDir     string
	User           string
	Principal      string
	TimeoutSeconds int
	Status         CommandStatus
	ExitCode       int
	Stdout         string
	Stderr         string
	StartedAt      time.Time
	CompletedAt    time.Time
}

// CommandResult bundles the post-execution fields written when a command
// finishes (regardless of outcome).
type CommandResult struct {
	Status      CommandStatus
	ExitCode    int
	Stdout      string
	Stderr      string
	CompletedAt time.Time
}

// BatchJobRecord is the persistent shape of a batch command job.
// Maps to the `batch_jobs` table.
//
// Target is a structured selector describing which agents the job applies
// to (e.g., {role: "web", environment: "prod"}). A richer Target type
// lands with epic 07 (Remote Execution); for now it's an opaque
// map[string]any that the storage layer marshals as JSON.
type BatchJobRecord struct {
	ID                string
	Target            map[string]any
	Command           string
	Args              []string
	Status            BatchJobStatus
	Concurrency       int
	TotalAgents       int
	CompletedAgents   int
	SuccessfulAgents  int
	FailedAgents      int
	CreatedAt         time.Time
	StartedAt         time.Time
	CompletedAt       time.Time
}

// BatchAgentResultRecord is one row in the per-agent results table for a
// batch job. Maps to `batch_agent_results`; primary key is
// (BatchJobID, AgentID).
//
// Stdout / Stderr carry the agent's captured output post-truncation;
// the *Truncated flags signal the agent's execution-time output-cap
// hit (see PROJECT-DETAILS §4.7 output truncation defaults: stdout
// 1 MiB / stderr 256 KiB).
type BatchAgentResultRecord struct {
	BatchJobID      string
	AgentID         string
	Success         bool
	ExitCode        int
	Error           string
	Stdout          []byte
	Stderr          []byte
	StdoutTruncated bool
	StderrTruncated bool
	StartedAt       time.Time
	CompletedAt     time.Time
}

// StateRunMode tags whether a stored run was an apply, a dry-run
// check, or a drift sweep. Mirrors statemgmt.RunMode + drift mode
// without importing statemgmt — the storage layer stays foundational.
type StateRunMode string

const (
	StateRunModeApply StateRunMode = "apply"
	StateRunModeCheck StateRunMode = "check"
	StateRunModeDrift StateRunMode = "drift"
)

// StateRunStatus is the lifecycle status of a stored run.
//
// Intended transitions:
//
//	running --> completed | failed | cancelled
type StateRunStatus string

const (
	StateRunStatusRunning   StateRunStatus = "running"
	StateRunStatusCompleted StateRunStatus = "completed"
	StateRunStatusFailed    StateRunStatus = "failed"
	StateRunStatusCancelled StateRunStatus = "cancelled"
)

// StateRunOutcome mirrors statemgmt.Outcome as the storage string
// enum. The bridge that translates statemgmt.RunReport into
// StateRun* records (lives elsewhere — gRPC service or CLI helper)
// maps the in-memory enum to these strings.
type StateRunOutcome string

const (
	StateRunOutcomeUnchanged     StateRunOutcome = "unchanged"
	StateRunOutcomeChanged       StateRunOutcome = "changed"
	StateRunOutcomeNoOp          StateRunOutcome = "no-op"
	StateRunOutcomeFailed        StateRunOutcome = "failed"
	StateRunOutcomeDriftDetected StateRunOutcome = "drift-detected"
	StateRunOutcomeSkipped       StateRunOutcome = "skipped"
)

// StateRunRecord is the persistent shape of one state-management run.
// Maps to the `state_runs` table.
//
// DeclarationsJSON carries the rendered declaration list that was
// attempted, JSON-encoded. The `kscorectl state rollback <id>` flow
// (Task 10) re-applies the previous successful run's declarations,
// so this column must round-trip the post-render Declaration shape.
//
// AgentID is the target the run was scoped to; "" means the run was
// not agent-scoped (e.g., compile-only operations).
type StateRunRecord struct {
	ID               string
	Mode             StateRunMode
	Source           string
	ClusterID        string
	AgentID          string
	StartedAt        time.Time
	EndedAt          time.Time // zero until finalized
	Status           StateRunStatus
	ErrorMessage     string
	Total            int
	Changed          int
	Unchanged        int
	Failed           int
	Skipped          int
	Drifted          int
	DeclarationsJSON string
}

// StateRunResultRecord is one row in the per-declaration results
// table for a state run. Maps to `state_run_results`; primary key is
// (RunID, DeclID).
//
// CheckMatches / ApplyChanged / TestResult are tri-state: Valid=false
// signals the corresponding phase did not run (e.g., ApplyChanged is
// Valid=false when Check matched and Apply was skipped).
type StateRunResultRecord struct {
	RunID        string
	DeclID       string
	Module       string
	Outcome      StateRunOutcome
	CheckMatches sql.NullBool
	CheckDiff    string
	ApplyChanged sql.NullBool
	ApplyDiff    string
	ApplyComment string
	TestResult   sql.NullBool
	ErrorMessage string
	StartedAt    time.Time
	DurationMS   int64
}

// StateRunEnd carries the terminal fields stamped when a run
// finishes, regardless of outcome.
type StateRunEnd struct {
	Status       StateRunStatus
	EndedAt      time.Time
	ErrorMessage string
	Total        int
	Changed      int
	Unchanged    int
	Failed       int
	Skipped      int
	Drifted      int
}

// APIKeyRecord is the persistent shape of an API key. KeyHash is the
// hex-encoded SHA-256 of the cleartext value generated at creation
// time; cleartext is never stored — it's returned to the operator
// once on creation and discarded.
//
// Role is held as a string ("admin" | "operator" | "readonly") to
// keep the storage layer free of pkg/api/auth dependencies; the auth
// adapter parses it on read.
type APIKeyRecord struct {
	ID        string
	Name      string
	KeyHash   string
	Role      string
	CreatedAt time.Time
	ExpiresAt time.Time // zero = never expires
	LastUsed  time.Time // zero = never used
}

// JoinTokenRecord is the persisted shape of an identity join token.
// Epic 09 task 9 — the cleartext Token field is NEVER stored; only
// Hash + Salt are. The Prefix is the operator-visible identifier
// used as the lookup key by the JoinTokenAttestor.
//
// Metadata is operator-supplied free-form tags persisted as JSON.
type JoinTokenRecord struct {
	ID        string
	Hash      []byte
	Salt      []byte
	Prefix    string
	AgentID   string
	TTL       time.Duration
	CreatedAt time.Time
	ExpiresAt time.Time
	UsedAt    time.Time // zero = never used
	MaxUses   int
	UsedCount int
	Metadata  map[string]string
}

// LeaseStoreRecord is the persisted shape of a tracked Vault / dynamic
// secret lease. Maps to the `secret_leases` table. Epic 10 task 6's
// LeaseManager owns the lifecycle: the broker's IssueDynamicSecret
// inserts a row via the manager; the scheduler updates / cleans up.
//
// State + Strategy are stored as the canonical lowercase strings
// produced by [secrets.LeaseState.String] / [secrets.RenewStrategy.String]
// (e.g. "active" / "expired" / "eager" / "lazy" / "on_demand") so
// `sqlite3` + `psql` reads stay human-readable.
//
// RevokedAt is non-nil only after a successful revocation. The row
// stays in the table after revoke so audit can still answer
// "what was this lease, when did it die" — DeleteExpiredLeases is
// the operator hook for retention.
//
// Metadata is operator-supplied free-form tags persisted as JSON.
type LeaseStoreRecord struct {
	ID            string
	Backend       string
	SecretPath    string
	IssuedAt      time.Time
	ExpiresAt     time.Time
	Duration      time.Duration
	Renewable     bool
	MaxTTL        time.Duration // 0 = unbounded
	State         string
	Strategy      string
	IssuedFor     string    // SPIFFE ID or empty
	LastRenewedAt time.Time // zero = never renewed
	RenewCount    int
	RevokedAt     time.Time // zero = unrevoked
	Metadata      map[string]string
}

// LeaseFilter narrows a LeaseStore.ListLeases query.
//
// Zero-value fields are ignored. `PathPrefix` (when non-empty) does
// SQL `LIKE 'prefix%'` against `secret_path`. `IncludeRevoked`
// defaults to false — operators rarely want revoked rows in routine
// queries; the audit-mode CLI sets it to surface them.
type LeaseFilter struct {
	Backend        string
	State          string // exact match
	PathPrefix     string
	IncludeRevoked bool
	Limit          int
	Offset         int
	SortColumn     string
	SortDesc       bool
}

// EventStoreRecord is the DB-shape value for an event row per PROJECT-
// DETAILS §4.9. Mirrors `internal/events.Event` exactly, with
// strings/time.Time on the wire so the state package stays free of
// the events package import (and reverse). `internal/events/sql_store.go`
// translates between them.
//
// Time is always UTC after a successful round trip. Severity is the
// canonical lowercase name from `internal/events.Severity.String()`.
// Type is the canonical `<category>.<subtype>` string. CorrelationID
// and Subject default to "" in the schema; nil-tag and nil-data round
// trip as nil (NOT the empty map) so test assertions read cleanly.
type EventStoreRecord struct {
	ID            string
	Type          string
	Source        string
	Time          time.Time
	Severity      string
	CorrelationID string
	Tags          map[string]string
	Data          map[string]any
	Subject       string
}

// EventFilter narrows an EventStore.ListEvents or EventStore.CountEvents
// query.
//
// Zero-value fields are ignored. Cursor pagination is keyed on the
// `id` column (PRIMARY KEY) — because `internal/events.NewEvent` stamps
// `uuid.NewV7()`, sort order by id is also time order. Cursor MAY be
// empty for the first page; subsequent pages pass the last returned
// row's ID.
//
//   - Type / TypePrefix: mutually exclusive. Type is exact-match;
//     TypePrefix does SQL `LIKE 'prefix%'`. The events-package wrapper
//     converts `EventQuery.Category` into a `TypePrefix = "<cat>."`.
//   - Severities: IN-clause filter; canonical lowercase names. The
//     wrapper translates `EventQuery.MinSeverity` into the closed-set
//     "at or above" slice (e.g. `MinSeverity=warn` → ["warn",
//     "error", "critical"]).
//   - Tags: ANDed exact-match filter on the JSON tag map. SQLite uses
//     `json_extract(tags, '$.key') = ?`; Postgres uses
//     `tags->>'key' = $n`. Unindexed in v1.0 — scans the filtered
//     time/type subset.
//   - Since / Until: half-open `[Since, Until)`. Either or both zero
//     means unbounded on that side.
//   - Cursor + Descending: cursor is interpreted as
//     `id > ?` (ascending, default) or `id < ?` (descending).
//   - Limit: caller-supplied cap; 0 means "no limit imposed here"
//     (the events-package wrapper applies a default before reaching
//     the store).
type EventFilter struct {
	Type          string
	TypePrefix    string
	Source        string
	Severities    []string
	Tags          map[string]string
	CorrelationID string
	Since         time.Time
	Until         time.Time
	Cursor        string
	Limit         int
	Descending    bool
}

// EventsRetentionPolicy is one row in the retention enforcer's
// per-type policy table. Mirrors `internal/events.RetentionPolicy`;
// the type field is the canonical `<category>.<subtype>` string and
// is empty for the catch-all "applies to every type" rule.
//
// Either MaxAge or MaxCount (or both) must be > 0 for the policy to
// remove anything — a zero-zero policy is a no-op. The enforcer
// (Epic 11 task 8) typically ships several policies in one call:
// one global catch-all, plus targeted overrides for high-volume types.
type EventsRetentionPolicy struct {
	Type     string
	MaxAge   time.Duration
	MaxCount int
}

// AuditEntryStoreRecord is the DB-shape value for an audit row per
// PROJECT-DETAILS §4.12. Mirrors `internal/audit.AuditEntry` exactly,
// with strings/time.Time on the wire so the state package stays free
// of the audit package import. `internal/audit/sql_store.go`
// translates between them.
//
// Timestamp is always UTC after a successful round trip. Severity,
// EnforcementMode, PolicyType are the canonical lowercase strings
// from the audit-package enums. Violations + Metadata are
// JSON-encoded.
type AuditEntryStoreRecord struct {
	ID              string
	Timestamp       time.Time
	PolicyID        string
	PolicyName      string
	PolicyType      string // "opa" | "cel" | "builtin" | ""
	ResourceType    string
	Allowed         bool
	DurationNS      int64
	Violations      []byte // JSON array of [{rule, message, severity, ...}]
	EnforcementMode string // "audit" | "warn" | "enforce"
	Severity        string // "low" | "medium" | "high" | "critical"
	User            string
	Action          string
	Metadata        map[string]string
}

// AuditEntryFilter narrows an AuditStore.ListAuditEntries or
// AuditStore.CountAuditEntries query. Task 2 ships the preview shape;
// task 3 enriches it.
//
// Zero-value fields are ignored. Cursor pagination keys on the `id`
// PRIMARY KEY (UUIDv7 — time-ordered).
//
//   - PolicyID / User / ResourceType / Action: exact-match filters.
//   - Severities: IN-clause filter; canonical lowercase names. The
//     audit-package wrapper translates `AuditQuery.MinSeverity` into
//     the closed-set "at or above" slice (e.g. `MinSeverity=high` →
//     ["high", "critical"]).
//   - Allowed: pointer so callers can distinguish "allowed=false
//     wanted" from "field not set" (the latter doesn't filter).
//   - Since / Until: half-open `[Since, Until)`.
//   - Cursor + Descending: `id > ?` or `id < ?`.
//   - Limit: 0 means "no limit imposed here" (audit wrapper applies
//     a default).
type AuditEntryFilter struct {
	PolicyID     string
	User         string
	ResourceType string
	Action       string
	Severities   []string
	Allowed      *bool
	Since        time.Time
	Until        time.Time
	Cursor       string
	Limit        int
	Descending   bool
}

// AuditRetentionPolicy is the single retention rule applied by the
// audit retention enforcer per §4.12 default 90d / 100k / 1h.
//
// MinSeverity exempts entries at or above the threshold from
// deletion — operators keep `critical` audit forever even when
// MaxAge / MaxCount would otherwise prune them. Empty MinSeverity
// = no exemption (every entry subject to retention).
//
// Either MaxAge or MaxCount (or both) must be > 0 for the policy
// to remove anything; the audit-package validator rejects
// zero-zero.
type AuditRetentionPolicy struct {
	MaxAge      time.Duration
	MaxCount    int
	MinSeverity string // "low" | "medium" | "high" | "critical" | ""
}

// AuditEntrySummaryRecord is the DB-shape aggregation result for
// SummarizeAuditEntries per §4.12 `AuditSummary` shape. Filter +
// time-range semantics match AuditEntryFilter (Cursor/Limit ignored;
// pagination doesn't apply to aggregations).
//
// ViolationsByPolicy and ViolationsBySeverity count DENIED entries
// only (allowed=false) — they answer "what is failing policy" not
// "what was evaluated". RangeStart / RangeEnd report the actual
// min/max timestamp of the filtered set; both are zero when no
// rows match.
type AuditEntrySummaryRecord struct {
	TotalEvaluations     int
	AllowedCount         int
	DeniedCount          int
	ViolationsByPolicy   map[string]int // policy_id → denied count
	ViolationsBySeverity map[string]int // canonical lowercase severity → denied count
	RangeStart           time.Time      // min(timestamp); zero on empty
	RangeEnd             time.Time      // max(timestamp); zero on empty
}

// ---- Filters --------------------------------------------------------------

// AgentFilter narrows an AgentStore.ListAgents query.
//
// Zero-value fields are ignored. SortColumn must be one of the values
// returned by AllowedAgentSortColumns; backends reject unknown columns
// to prevent SQL injection through ORDER BY.
type AgentFilter struct {
	Status      AgentStatus
	LabelKey    string
	LabelValue  string
	Limit       int
	Offset      int
	SortColumn  string
	SortDesc    bool
}

// CommandFilter narrows a CommandStore.ListCommands query.
type CommandFilter struct {
	AgentID    string
	Status     CommandStatus
	Since      time.Time
	Until      time.Time
	Limit      int
	Offset     int
	SortColumn string
	SortDesc   bool
}

// BatchJobFilter narrows a BatchJobStore.ListBatchJobs query.
type BatchJobFilter struct {
	Status     BatchJobStatus
	Since      time.Time
	Until      time.Time
	Limit      int
	Offset     int
	SortColumn string
	SortDesc   bool
}

// StateRunFilter narrows a StateHistoryStore.ListStateRuns query.
// Zero-value scalar fields are ignored; zero times are interpreted
// as "no bound".
type StateRunFilter struct {
	AgentID    string
	Mode       StateRunMode
	Status     StateRunStatus
	After      time.Time
	Before     time.Time
	Limit      int
	Offset     int
	SortColumn string
	SortDesc   bool
}

// APIKeyFilter narrows an APIKeyStore.ListAPIKeys query.
type APIKeyFilter struct {
	Role       string // empty = no filter
	Limit      int
	Offset     int
	SortColumn string
	SortDesc   bool
}

// JoinTokenFilter narrows a JoinTokenStore.ListJoinTokens query.
//
// Zero-value fields are ignored. `Unused` (when true) restricts to
// tokens whose UsedCount < MaxUses. `UnexpiredAt` (when non-zero)
// restricts to tokens whose ExpiresAt > UnexpiredAt — useful for
// excluding already-expired records without first running Cleanup.
type JoinTokenFilter struct {
	AgentID     string
	Unused      bool
	UnexpiredAt time.Time
	Limit       int
	Offset      int
	SortColumn  string
	SortDesc    bool
}

// AllowedAgentSortColumns is the allowlist of column names usable in
// AgentFilter.SortColumn. Backends reject any value not in this set.
var AllowedAgentSortColumns = []string{
	"id", "hostname", "status", "registered_at", "last_heartbeat_at",
}

// AllowedCommandSortColumns is the allowlist for CommandFilter.SortColumn.
var AllowedCommandSortColumns = []string{
	"id", "agent_id", "status", "started_at", "completed_at",
}

// AllowedBatchJobSortColumns is the allowlist for BatchJobFilter.SortColumn.
var AllowedBatchJobSortColumns = []string{
	"id", "status", "created_at", "started_at", "completed_at",
}

// AllowedAPIKeySortColumns is the allowlist for APIKeyFilter.SortColumn.
var AllowedAPIKeySortColumns = []string{
	"id", "name", "role", "created_at", "expires_at", "last_used",
}

// AllowedJoinTokenSortColumns is the allowlist for
// JoinTokenFilter.SortColumn.
var AllowedJoinTokenSortColumns = []string{
	"id", "prefix", "agent_id", "created_at", "expires_at", "used_at",
}

// AllowedStateRunSortColumns is the allowlist for StateRunFilter.SortColumn.
var AllowedStateRunSortColumns = []string{
	"id", "mode", "status", "agent_id", "started_at", "ended_at",
}

// AllowedLeaseSortColumns is the allowlist for LeaseFilter.SortColumn.
var AllowedLeaseSortColumns = []string{
	"id", "backend", "secret_path", "state", "strategy",
	"issued_at", "expires_at", "last_renewed_at",
}
