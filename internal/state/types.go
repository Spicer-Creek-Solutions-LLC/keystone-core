package state

import "time"

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
type CommandRecord struct {
	ID             string
	AgentID        string
	Command        string
	Args           []string
	Env            map[string]string
	WorkingDir     string
	User           string
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
type BatchAgentResultRecord struct {
	BatchJobID  string
	AgentID     string
	Success     bool
	ExitCode    int
	Error       string
	StartedAt   time.Time
	CompletedAt time.Time
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

// APIKeyFilter narrows an APIKeyStore.ListAPIKeys query.
type APIKeyFilter struct {
	Role       string // empty = no filter
	Limit      int
	Offset     int
	SortColumn string
	SortDesc   bool
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
