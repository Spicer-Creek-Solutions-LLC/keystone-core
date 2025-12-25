package state

import (
	"context"
	"time"

	pb "github.com/titananvil/titan-anvil/pkg/api/v1"
)

// Store defines the interface for state storage
type Store interface {
	// Agent operations
	SaveAgent(ctx context.Context, agent *AgentRecord) error
	GetAgent(ctx context.Context, agentID string) (*AgentRecord, error)
	ListAgents(ctx context.Context, filter *AgentFilter) ([]*AgentRecord, error)
	UpdateAgentStatus(ctx context.Context, agentID string, status pb.AgentStatus, lastHeartbeat time.Time) error
	UpdateAgentMetrics(ctx context.Context, agentID string, metrics *pb.SystemMetrics) error
	DeleteAgent(ctx context.Context, agentID string) error

	// Command operations
	SaveCommand(ctx context.Context, cmd *CommandRecord) error
	GetCommand(ctx context.Context, commandID string) (*CommandRecord, error)
	ListCommands(ctx context.Context, filter *CommandFilter) ([]*CommandRecord, error)
	UpdateCommandStatus(ctx context.Context, commandID string, status pb.CommandStatus) error
	UpdateCommandResult(ctx context.Context, commandID string, result *CommandResult) error

	// Health and lifecycle
	Ping(ctx context.Context) error
	Close() error
}

// AgentRecord represents an agent in the database
type AgentRecord struct {
	ID              string
	Hostname        string
	OS              string
	Architecture    string
	IPAddresses     []string
	PlatformVersion string
	AgentVersion    string
	Labels          map[string]string
	Status          pb.AgentStatus
	LastHeartbeat   time.Time
	RegisteredAt    time.Time
	UpdatedAt       time.Time

	// Latest metrics
	CPUPercent    float32
	MemoryPercent float32
	DiskPercent   float32
	LoadAverage   []float32
}

// AgentFilter defines filter criteria for listing agents
type AgentFilter struct {
	Status     *pb.AgentStatus
	Labels     map[string]string
	Limit      int
	Offset     int
	SortBy     string
	SortOrder  string // "asc" or "desc"
}

// CommandRecord represents a command execution in the database
type CommandRecord struct {
	ID          string
	AgentID     string
	Command     string
	Args        []string
	Env         map[string]string
	WorkingDir  string
	User        string
	Timeout     int32
	Status      pb.CommandStatus
	ExitCode    int32
	Stdout      string
	Stderr      string
	Error       string
	CreatedAt   time.Time
	StartedAt   *time.Time
	CompletedAt *time.Time
	DurationMs  int64
}

// CommandFilter defines filter criteria for listing commands
type CommandFilter struct {
	AgentID   string
	Status    *pb.CommandStatus
	StartTime *time.Time
	EndTime   *time.Time
	Limit     int
	Offset    int
	SortBy    string
	SortOrder string // "asc" or "desc"
}

// CommandResult represents the result of command execution
type CommandResult struct {
	Status      pb.CommandStatus
	ExitCode    int32
	Stdout      string
	Stderr      string
	Error       string
	StartedAt   time.Time
	CompletedAt time.Time
	DurationMs  int64
}

// Config holds database configuration
type Config struct {
	// Backend type: "sqlite" or "postgresql"
	Backend string

	// SQLite specific
	SQLitePath       string
	SQLiteWAL        bool
	SQLiteBusyTimeout int

	// PostgreSQL specific
	PostgreSQLDSN         string
	PostgreSQLMaxOpen     int
	PostgreSQLMaxIdle     int
	PostgreSQLConnMaxLife time.Duration
}
