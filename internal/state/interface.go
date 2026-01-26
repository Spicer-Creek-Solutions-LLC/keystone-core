package state

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
)

// AgentStore defines the interface for agent persistence.
type AgentStore interface {
	SaveAgent(ctx context.Context, agent *AgentRecord) error
	GetAgent(ctx context.Context, agentID string) (*AgentRecord, error)
	ListAgents(ctx context.Context, filter *AgentFilter) ([]*AgentRecord, error)
	UpdateAgentStatus(ctx context.Context, agentID string, status pb.AgentStatus, lastHeartbeat time.Time) error
	UpdateAgentMetrics(ctx context.Context, agentID string, metrics *pb.SystemMetrics) error
	DeleteAgent(ctx context.Context, agentID string) error
}

// CommandStore defines the interface for command persistence.
type CommandStore interface {
	SaveCommand(ctx context.Context, cmd *CommandRecord) error
	GetCommand(ctx context.Context, commandID string) (*CommandRecord, error)
	ListCommands(ctx context.Context, filter *CommandFilter) ([]*CommandRecord, error)
	UpdateCommandStatus(ctx context.Context, commandID string, status pb.CommandStatus) error
	UpdateCommandResult(ctx context.Context, commandID string, result *CommandResult) error
}

// BatchJobStore defines the interface for batch job persistence.
type BatchJobStore interface {
	SaveBatchJob(ctx context.Context, job *BatchJobRecord) error
	GetBatchJob(ctx context.Context, batchJobID string) (*BatchJobRecord, error)
	ListBatchJobs(ctx context.Context, filter *BatchJobFilter) ([]*BatchJobRecord, error)
	UpdateBatchJobStatus(ctx context.Context, batchJobID string, status pb.BatchJobStatus) error
	UpdateBatchJobProgress(ctx context.Context, batchJobID string, progress *BatchJobProgress) error
	SaveBatchAgentResult(ctx context.Context, batchJobID string, result *BatchAgentResultRecord) error
}

// HealthStore defines the interface for health checks and lifecycle management.
type HealthStore interface {
	Ping(ctx context.Context) error
	Close() error
}

// ControlPlaneStore groups agent, command, and batch job storage requirements.
type ControlPlaneStore interface {
	AgentStore
	CommandStore
	BatchJobStore
}

// Store defines the interface for full state storage.
type Store interface {
	ControlPlaneStore
	HealthStore
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

// BatchJobRecord represents a batch job in the database
type BatchJobRecord struct {
	ID          string
	Target      string
	Command     string
	Args        []string
	Env         map[string]string
	WorkingDir  string
	User        string
	Timeout     int32
	Concurrency int32
	Status      pb.BatchJobStatus
	CreatedAt   time.Time
	StartedAt   *time.Time
	CompletedAt *time.Time
	DurationMs  int64

	// Progress
	TotalAgents      int32
	CompletedAgents  int32
	SuccessfulAgents int32
	FailedAgents     int32
	SuccessRate      float32

	// Results
	AgentResults []*BatchAgentResultRecord
}

// BatchAgentResultRecord represents a per-agent result in a batch job
type BatchAgentResultRecord struct {
	BatchJobID string
	AgentID    string
	Success    bool
	ExitCode   int32
	Error      string
	DurationMs int64
	CreatedAt  time.Time
}

// BatchJobFilter defines filter criteria for listing batch jobs
type BatchJobFilter struct {
	Status    *pb.BatchJobStatus
	Target    string
	StartTime *time.Time
	EndTime   *time.Time
	Limit     int
	Offset    int
	SortBy    string
	SortOrder string // "asc" or "desc"
}

// BatchJobProgress represents batch job progress update
type BatchJobProgress struct {
	TotalAgents      int32
	CompletedAgents  int32
	SuccessfulAgents int32
	FailedAgents     int32
	SuccessRate      float32
	StartedAt        *time.Time
	CompletedAt      *time.Time
	DurationMs       int64
}

// Config holds database configuration
type Config struct {
	// Backend type: "sqlite" or "postgresql"
	Backend string

	// SQLite specific
	SQLitePath        string
	SQLiteWAL         bool
	SQLiteBusyTimeout int

	// PostgreSQL specific
	PostgreSQLDSN         string
	PostgreSQLMaxOpen     int
	PostgreSQLMaxIdle     int
	PostgreSQLConnMaxLife time.Duration

	// PostgreSQL structured config (alternative to DSN)
	// If PostgreSQLConfig is set and PostgreSQLDSN is empty, DSN will be built from config
	PostgreSQLConfig *PostgreSQLConfig
}

// PostgreSQLConfig holds structured PostgreSQL connection configuration.
// This provides an alternative to specifying a raw DSN, with proper IPv6 support.
type PostgreSQLConfig struct {
	// Host is the PostgreSQL server address.
	// IPv6 addresses should be specified without brackets (e.g., "2001:db8::1")
	// Brackets will be added automatically when building the DSN.
	Host string `yaml:"host" mapstructure:"host"`

	// Port is the PostgreSQL server port (default: 5432)
	Port int `yaml:"port" mapstructure:"port"`

	// Database is the database name
	Database string `yaml:"database" mapstructure:"database"`

	// User is the database user
	User string `yaml:"user" mapstructure:"user"`

	// Password is the database password
	Password string `yaml:"password" mapstructure:"password"`

	// SSLMode specifies the SSL connection mode.
	// Valid values: disable, allow, prefer, require, verify-ca, verify-full
	SSLMode string `yaml:"sslmode" mapstructure:"sslmode"`

	// SSLRootCert is the path to the root CA certificate
	SSLRootCert string `yaml:"sslrootcert" mapstructure:"sslrootcert"`

	// SSLCert is the path to the client certificate
	SSLCert string `yaml:"sslcert" mapstructure:"sslcert"`

	// SSLKey is the path to the client key
	SSLKey string `yaml:"sslkey" mapstructure:"sslkey"`

	// ConnectTimeout is the connection timeout in seconds
	ConnectTimeout int `yaml:"connect_timeout" mapstructure:"connect_timeout"`

	// ApplicationName identifies the connecting application
	ApplicationName string `yaml:"application_name" mapstructure:"application_name"`
}

// BuildDSN constructs a PostgreSQL connection string (DSN) from the structured config.
// IPv6 addresses are automatically wrapped in brackets as required by PostgreSQL.
func (c *PostgreSQLConfig) BuildDSN() string {
	if c == nil {
		return ""
	}

	// Format host for IPv6 - PostgreSQL requires brackets around IPv6 addresses
	host := c.Host
	if host != "" && isIPv6Address(host) {
		// Strip any existing brackets and re-add them
		host = strings.Trim(host, "[]")
		host = "[" + host + "]"
	}

	// Build DSN with required fields
	var parts []string

	if host != "" {
		parts = append(parts, "host="+host)
	}
	if c.Port > 0 {
		parts = append(parts, fmt.Sprintf("port=%d", c.Port))
	}
	if c.Database != "" {
		parts = append(parts, "dbname="+c.Database)
	}
	if c.User != "" {
		parts = append(parts, "user="+c.User)
	}
	if c.Password != "" {
		parts = append(parts, "password="+c.Password)
	}
	if c.SSLMode != "" {
		parts = append(parts, "sslmode="+c.SSLMode)
	}
	if c.SSLRootCert != "" {
		parts = append(parts, "sslrootcert="+c.SSLRootCert)
	}
	if c.SSLCert != "" {
		parts = append(parts, "sslcert="+c.SSLCert)
	}
	if c.SSLKey != "" {
		parts = append(parts, "sslkey="+c.SSLKey)
	}
	if c.ConnectTimeout > 0 {
		parts = append(parts, fmt.Sprintf("connect_timeout=%d", c.ConnectTimeout))
	}
	if c.ApplicationName != "" {
		parts = append(parts, "application_name="+c.ApplicationName)
	}

	return strings.Join(parts, " ")
}

// Validate checks the PostgreSQLConfig for common errors.
func (c *PostgreSQLConfig) Validate() error {
	if c == nil {
		return fmt.Errorf("postgresql config is nil")
	}
	if c.Host == "" {
		return fmt.Errorf("postgresql host is required")
	}
	if c.Database == "" {
		return fmt.Errorf("postgresql database is required")
	}

	// Validate SSL mode if specified
	validSSLModes := map[string]bool{
		"":            true, // empty is allowed
		"disable":     true,
		"allow":       true,
		"prefer":      true,
		"require":     true,
		"verify-ca":   true,
		"verify-full": true,
	}
	if !validSSLModes[c.SSLMode] {
		return fmt.Errorf("invalid sslmode: %s", c.SSLMode)
	}

	return nil
}

// isIPv6Address checks if the given string is an IPv6 address.
// It handles addresses with and without brackets and zone IDs.
func isIPv6Address(addr string) bool {
	// Remove brackets if present
	addr = strings.Trim(addr, "[]")

	// Remove zone ID if present (e.g., "fe80::1%eth0")
	if idx := strings.Index(addr, "%"); idx != -1 {
		addr = addr[:idx]
	}

	// Check for IPv6 address format (contains colons but not a port number pattern)
	// IPv6 addresses always contain at least 2 colons
	colonCount := strings.Count(addr, ":")
	if colonCount < 2 {
		return false
	}

	// Parse to verify it's a valid IP
	ip := net.ParseIP(addr)
	return ip != nil && ip.To4() == nil
}
