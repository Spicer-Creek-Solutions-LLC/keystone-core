// Package upgrade provides upgrade orchestration for Keystone Core components.
// It supports rolling, blue-green, and canary upgrade strategies with automatic
// rollback on failure.
package upgrade

import (
	"context"
	"fmt"
	"time"
)

// Strategy defines the upgrade approach.
type Strategy string

const (
	// StrategyRolling updates one node at a time with health checks.
	StrategyRolling Strategy = "rolling"

	// StrategyBlueGreen deploys a new cluster and switches traffic.
	StrategyBlueGreen Strategy = "blue-green"

	// StrategyCanary gradually shifts traffic to new version.
	StrategyCanary Strategy = "canary"

	// StrategyInPlace stops, upgrades, and starts (has downtime).
	StrategyInPlace Strategy = "in-place"
)

// Phase represents the current phase of an upgrade.
type Phase string

// PhaseIdle constants define the phases.
const (
	PhaseIdle        Phase = "idle"
	PhasePending     Phase = "pending"
	PhaseValidating  Phase = "validating"
	PhasePreparing   Phase = "preparing"
	PhaseUpgrading   Phase = "upgrading"
	PhaseVerifying   Phase = "verifying"
	PhaseCompleted   Phase = "completed"
	PhaseFailed      Phase = "failed"
	PhaseRollingBack Phase = "rolling_back"
	PhaseRolledBack  Phase = "rolled_back"
)

// Status represents the overall status of an upgrade.
type Status string

// StatusPending constants define the possible statuses.
const (
	StatusPending    Status = "pending"
	StatusInProgress Status = "in_progress"
	StatusCompleted  Status = "completed"
	StatusFailed     Status = "failed"
	StatusRolledBack Status = "rolled_back"
	StatusCancelled  Status = "cancelled"
)

// ComponentType identifies what is being upgraded.
type ComponentType string

// ComponentServer constants define the components.
const (
	ComponentServer   ComponentType = "server"
	ComponentAgent    ComponentType = "agent"
	ComponentNATS     ComponentType = "nats"
	ComponentDatabase ComponentType = "database"
	ComponentEtcd     ComponentType = "etcd"
)

// HealthStatus represents the health of a component.
type HealthStatus string

// HealthUnknown and related constants.
const (
	HealthUnknown   HealthStatus = "unknown"
	HealthHealthy   HealthStatus = "healthy"
	HealthDegraded  HealthStatus = "degraded"
	HealthUnhealthy HealthStatus = "unhealthy"
)

// Version represents a semantic version.
type Version struct {
	Major      int    `json:"major" yaml:"major"`
	Minor      int    `json:"minor" yaml:"minor"`
	Patch      int    `json:"patch" yaml:"patch"`
	Prerelease string `json:"prerelease,omitempty" yaml:"prerelease,omitempty"`
	Build      string `json:"build,omitempty" yaml:"build,omitempty"`
}

// String returns the version as a string.
func (v Version) String() string {
	s := ""
	if v.Major > 0 || v.Minor > 0 || v.Patch > 0 {
		s = fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	}
	if v.Prerelease != "" {
		s += "-" + v.Prerelease
	}
	if v.Build != "" {
		s += "+" + v.Build
	}
	return s
}

// Compare compares two versions. Returns -1 if v < other, 0 if equal, 1 if v > other.
func (v Version) Compare(other Version) int {
	if v.Major != other.Major {
		if v.Major < other.Major {
			return -1
		}
		return 1
	}
	if v.Minor != other.Minor {
		if v.Minor < other.Minor {
			return -1
		}
		return 1
	}
	if v.Patch != other.Patch {
		if v.Patch < other.Patch {
			return -1
		}
		return 1
	}
	// Prerelease versions have lower precedence than normal versions
	if v.Prerelease != "" && other.Prerelease == "" {
		return -1
	}
	if v.Prerelease == "" && other.Prerelease != "" {
		return 1
	}
	if v.Prerelease < other.Prerelease {
		return -1
	}
	if v.Prerelease > other.Prerelease {
		return 1
	}
	return 0
}

// IsNewerThan returns true if v is newer than other.
func (v Version) IsNewerThan(other Version) bool {
	return v.Compare(other) > 0
}

// IsOlderThan returns true if v is older than other.
func (v Version) IsOlderThan(other Version) bool {
	return v.Compare(other) < 0
}

// IsCompatibleWith returns true if v is semver-compatible with other.
func (v Version) IsCompatibleWith(other Version) bool {
	if v.Major != other.Major {
		return false
	}
	if v.Major == 0 {
		return v.Minor == other.Minor
	}
	return true
}

// IsZero returns true if this is the zero version.
func (v Version) IsZero() bool {
	return v.Major == 0 && v.Minor == 0 && v.Patch == 0 && v.Prerelease == "" && v.Build == ""
}

// VersionInfo contains metadata about a version.
type VersionInfo struct {
	Version      Version   `json:"version" yaml:"version"`
	ReleaseDate  time.Time `json:"release_date" yaml:"release_date"`
	Channel      string    `json:"channel" yaml:"channel"` // stable, beta, nightly
	Changelog    string    `json:"changelog,omitempty" yaml:"changelog,omitempty"`
	MinUpgrade   *Version  `json:"min_upgrade,omitempty" yaml:"min_upgrade,omitempty"`   // Minimum version to upgrade from
	MaxUpgrade   *Version  `json:"max_upgrade,omitempty" yaml:"max_upgrade,omitempty"`   // Maximum version to upgrade from
	Dependencies []string  `json:"dependencies,omitempty" yaml:"dependencies,omitempty"` // Required component versions
	Deprecated   bool      `json:"deprecated,omitempty" yaml:"deprecated,omitempty"`
	SecurityFix  bool      `json:"security_fix,omitempty" yaml:"security_fix,omitempty"`
	Checksum     string    `json:"checksum,omitempty" yaml:"checksum,omitempty"`   // SHA-256 checksum
	Signature    string    `json:"signature,omitempty" yaml:"signature,omitempty"` // Cosign signature
}

// Config defines the upgrade configuration.
type Config struct {
	// Target version to upgrade to.
	TargetVersion string `json:"target_version" yaml:"target_version"`

	// Strategy to use for the upgrade.
	Strategy Strategy `json:"strategy" yaml:"strategy"`

	// DryRun performs validation without making changes.
	DryRun bool `json:"dry_run,omitempty" yaml:"dry_run,omitempty"`

	// Force skips compatibility checks (use with caution).
	Force bool `json:"force,omitempty" yaml:"force,omitempty"`

	// Rolling upgrade configuration.
	Rolling *RollingConfig `json:"rolling,omitempty" yaml:"rolling,omitempty"`

	// Canary upgrade configuration.
	Canary *CanaryConfig `json:"canary,omitempty" yaml:"canary,omitempty"`

	// Health check configuration.
	HealthCheck *HealthCheckConfig `json:"health_check,omitempty" yaml:"health_check,omitempty"`

	// Rollback configuration.
	Rollback *RollbackConfig `json:"rollback,omitempty" yaml:"rollback,omitempty"`

	// Components to upgrade (empty means all).
	Components []ComponentType `json:"components,omitempty" yaml:"components,omitempty"`

	// Timeout for the entire upgrade operation.
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// RollingConfig configures rolling upgrade behavior.
type RollingConfig struct {
	// MaxUnavailable is the maximum number of nodes that can be unavailable.
	MaxUnavailable int `json:"max_unavailable" yaml:"max_unavailable"`

	// HealthCheckInterval is the time between health checks.
	HealthCheckInterval time.Duration `json:"health_check_interval" yaml:"health_check_interval"`

	// HealthCheckTimeout is the maximum time to wait for a health check.
	HealthCheckTimeout time.Duration `json:"health_check_timeout" yaml:"health_check_timeout"`

	// DrainTimeout is the maximum time to wait for connections to drain.
	DrainTimeout time.Duration `json:"drain_timeout" yaml:"drain_timeout"`

	// NodeDelay is the delay between upgrading nodes.
	NodeDelay time.Duration `json:"node_delay,omitempty" yaml:"node_delay,omitempty"`

	// Order specifies the order to upgrade nodes (leader_last, leader_first, any).
	Order string `json:"order,omitempty" yaml:"order,omitempty"`
}

// CanaryConfig configures canary upgrade behavior.
type CanaryConfig struct {
	// InitialPercentage is the initial percentage of traffic to new version.
	InitialPercentage int `json:"initial_percentage" yaml:"initial_percentage"`

	// Increment is the percentage to increase each step.
	Increment int `json:"increment" yaml:"increment"`

	// Interval is the time between increments.
	Interval time.Duration `json:"interval" yaml:"interval"`

	// SuccessThreshold is the number of successful checks required before incrementing.
	SuccessThreshold int `json:"success_threshold" yaml:"success_threshold"`

	// FailureThreshold is the number of failures that trigger rollback.
	FailureThreshold int `json:"failure_threshold" yaml:"failure_threshold"`

	// Metrics are the metrics to monitor during canary.
	Metrics []CanaryMetric `json:"metrics,omitempty" yaml:"metrics,omitempty"`

	// PrometheusAddress is the Prometheus base URL for metric queries.
	PrometheusAddress string `json:"prometheus_address,omitempty" yaml:"prometheus_address,omitempty"`

	// QueryTimeout is the timeout for each Prometheus query.
	QueryTimeout time.Duration `json:"query_timeout,omitempty" yaml:"query_timeout,omitempty"`
}

// CanaryMetric defines a metric to monitor during canary deployment.
type CanaryMetric struct {
	// Name of the metric.
	Name string `json:"name" yaml:"name"`

	// Query to evaluate (PromQL).
	Query string `json:"query" yaml:"query"`

	// Threshold value.
	Threshold float64 `json:"threshold" yaml:"threshold"`

	// Comparison operator (lt, le, gt, ge, eq, ne).
	Comparison string `json:"comparison" yaml:"comparison"`
}

// HealthCheckConfig configures health checking during upgrades.
type HealthCheckConfig struct {
	// Enabled determines if health checks are performed.
	Enabled bool `json:"enabled" yaml:"enabled"`

	// Checks are the health checks to perform.
	Checks []HealthCheck `json:"checks,omitempty" yaml:"checks,omitempty"`

	// SuccessThreshold is the number of successful checks required.
	SuccessThreshold int `json:"success_threshold" yaml:"success_threshold"`

	// FailureThreshold is the number of failures before marking unhealthy.
	FailureThreshold int `json:"failure_threshold" yaml:"failure_threshold"`

	// Interval between health checks.
	Interval time.Duration `json:"interval" yaml:"interval"`

	// Timeout for individual health checks.
	Timeout time.Duration `json:"timeout" yaml:"timeout"`
}

// HealthCheck defines a single health check.
type HealthCheck struct {
	// Type of health check (http, tcp, exec, metric).
	Type string `json:"type" yaml:"type"`

	// Endpoint for HTTP/TCP checks.
	Endpoint string `json:"endpoint,omitempty" yaml:"endpoint,omitempty"`

	// ExpectedStatus for HTTP checks.
	ExpectedStatus int `json:"expected_status,omitempty" yaml:"expected_status,omitempty"`

	// Command for exec checks.
	Command string `json:"command,omitempty" yaml:"command,omitempty"`

	// Query for metric checks.
	Query string `json:"query,omitempty" yaml:"query,omitempty"`

	// Expected value for metric checks.
	Expected float64 `json:"expected,omitempty" yaml:"expected,omitempty"`
}

// RollbackConfig configures automatic rollback behavior.
type RollbackConfig struct {
	// Automatic enables automatic rollback on failure.
	Automatic bool `json:"automatic" yaml:"automatic"`

	// OnFailureCount triggers rollback after this many failures.
	OnFailureCount int `json:"on_failure_count" yaml:"on_failure_count"`

	// KeepPreviousVersion retains the previous version for rollback.
	KeepPreviousVersion bool `json:"keep_previous_version" yaml:"keep_previous_version"`

	// Timeout for rollback operation.
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// State represents the current state of an upgrade operation.
type State struct {
	// ID is a unique identifier for this upgrade.
	ID string `json:"id" yaml:"id"`

	// Phase is the current upgrade phase.
	Phase Phase `json:"phase" yaml:"phase"`

	// Status is the overall status.
	Status Status `json:"status" yaml:"status"`

	// Config is the upgrade configuration.
	Config *Config `json:"config" yaml:"config"`

	// FromVersion is the version being upgraded from.
	FromVersion Version `json:"from_version" yaml:"from_version"`

	// ToVersion is the version being upgraded to.
	ToVersion Version `json:"to_version" yaml:"to_version"`

	// StartTime is when the upgrade started.
	StartTime time.Time `json:"start_time" yaml:"start_time"`

	// EndTime is when the upgrade completed.
	EndTime *time.Time `json:"end_time,omitempty" yaml:"end_time,omitempty"`

	// Progress is the completion percentage (0-100).
	Progress int `json:"progress" yaml:"progress"`

	// Message provides status details.
	Message string `json:"message,omitempty" yaml:"message,omitempty"`

	// NodeStates tracks the state of each node being upgraded.
	NodeStates map[string]*NodeUpgradeState `json:"node_states,omitempty" yaml:"node_states,omitempty"`

	// Errors contains any errors that occurred.
	Errors []Error `json:"errors,omitempty" yaml:"errors,omitempty"`

	// RollbackState contains rollback information if rollback occurred.
	RollbackState *RollbackState `json:"rollback_state,omitempty" yaml:"rollback_state,omitempty"`
}

// NodeUpgradeState tracks the upgrade state of a single node.
type NodeUpgradeState struct {
	// NodeID is the identifier of the node.
	NodeID string `json:"node_id" yaml:"node_id"`

	// Component being upgraded on this node.
	Component ComponentType `json:"component" yaml:"component"`

	// Status of this node's upgrade.
	Status Status `json:"status" yaml:"status"`

	// FromVersion on this node.
	FromVersion Version `json:"from_version" yaml:"from_version"`

	// ToVersion on this node.
	ToVersion Version `json:"to_version" yaml:"to_version"`

	// Health of the node after upgrade.
	Health HealthStatus `json:"health" yaml:"health"`

	// StartTime of this node's upgrade.
	StartTime time.Time `json:"start_time" yaml:"start_time"`

	// EndTime of this node's upgrade.
	EndTime *time.Time `json:"end_time,omitempty" yaml:"end_time,omitempty"`

	// Error if upgrade failed.
	Error string `json:"error,omitempty" yaml:"error,omitempty"`
}

// Error represents an error during upgrade.
type Error struct {
	// Time when the error occurred.
	Time time.Time `json:"time" yaml:"time"`

	// Phase during which the error occurred.
	Phase Phase `json:"phase" yaml:"phase"`

	// NodeID where the error occurred (if applicable).
	NodeID string `json:"node_id,omitempty" yaml:"node_id,omitempty"`

	// Message describing the error.
	Message string `json:"message" yaml:"message"`

	// Recoverable indicates if the error can be recovered from.
	Recoverable bool `json:"recoverable" yaml:"recoverable"`
}

// RollbackState contains information about a rollback.
type RollbackState struct {
	// Reason for the rollback.
	Reason string `json:"reason" yaml:"reason"`

	// Automatic indicates if this was an automatic rollback.
	Automatic bool `json:"automatic" yaml:"automatic"`

	// StartTime of the rollback.
	StartTime time.Time `json:"start_time" yaml:"start_time"`

	// EndTime of the rollback.
	EndTime *time.Time `json:"end_time,omitempty" yaml:"end_time,omitempty"`

	// Status of the rollback.
	Status Status `json:"status" yaml:"status"`

	// Error if rollback failed.
	Error string `json:"error,omitempty" yaml:"error,omitempty"`
}

// Result contains the result of an upgrade operation.
type Result struct {
	// Success indicates if the upgrade completed successfully.
	Success bool `json:"success" yaml:"success"`

	// State is the final upgrade state.
	State *State `json:"state" yaml:"state"`

	// Duration of the upgrade.
	Duration time.Duration `json:"duration" yaml:"duration"`

	// NodesUpgraded is the count of nodes that were upgraded.
	NodesUpgraded int `json:"nodes_upgraded" yaml:"nodes_upgraded"`

	// NodesFailed is the count of nodes that failed to upgrade.
	NodesFailed int `json:"nodes_failed" yaml:"nodes_failed"`

	// RolledBack indicates if the upgrade was rolled back.
	RolledBack bool `json:"rolled_back" yaml:"rolled_back"`

	// Message provides additional context.
	Message string `json:"message,omitempty" yaml:"message,omitempty"`
}

// AgentBatchConfig configures agent upgrade batching.
type AgentBatchConfig struct {
	// BatchSize is the number of agents to upgrade simultaneously.
	BatchSize int `json:"batch_size" yaml:"batch_size"`

	// BatchDelay is the delay between batches.
	BatchDelay time.Duration `json:"batch_delay" yaml:"batch_delay"`

	// MaxFailures is the maximum failures before stopping.
	MaxFailures int `json:"max_failures" yaml:"max_failures"`

	// Selectors filter which agents to upgrade.
	Selectors map[string]string `json:"selectors,omitempty" yaml:"selectors,omitempty"`

	// ExcludeSelectors filter which agents to exclude.
	ExcludeSelectors map[string]string `json:"exclude_selectors,omitempty" yaml:"exclude_selectors,omitempty"`

	// Priority orders agents by these labels.
	Priority []string `json:"priority,omitempty" yaml:"priority,omitempty"`
}

// Manager is the interface for upgrade orchestration.
type Manager interface {
	// CheckUpgrade checks if an upgrade is available and compatible.
	CheckUpgrade(ctx context.Context, targetVersion string) (*Check, error)

	// PlanUpgrade creates an upgrade plan without executing it.
	PlanUpgrade(ctx context.Context, config *Config) (*Plan, error)

	// StartUpgrade begins an upgrade operation.
	StartUpgrade(ctx context.Context, config *Config) (*State, error)

	// GetUpgradeStatus returns the current upgrade status.
	GetUpgradeStatus(ctx context.Context, upgradeID string) (*State, error)

	// CancelUpgrade cancels an in-progress upgrade.
	CancelUpgrade(ctx context.Context, upgradeID string) error

	// Rollback rolls back to the previous version.
	Rollback(ctx context.Context, upgradeID string) (*RollbackState, error)

	// GetUpgradeHistory returns upgrade history.
	GetUpgradeHistory(ctx context.Context, limit int) ([]*State, error)

	// GetAvailableVersions returns available versions.
	GetAvailableVersions(ctx context.Context, channel string) ([]VersionInfo, error)
}

// Check contains the result of checking upgrade compatibility.
type Check struct {
	// Compatible indicates if the upgrade is compatible.
	Compatible bool `json:"compatible" yaml:"compatible"`

	// CurrentVersion is the current version.
	CurrentVersion Version `json:"current_version" yaml:"current_version"`

	// TargetVersion is the target version.
	TargetVersion Version `json:"target_version" yaml:"target_version"`

	// VersionInfo contains metadata about the target version.
	VersionInfo *VersionInfo `json:"version_info,omitempty" yaml:"version_info,omitempty"`

	// Warnings are non-blocking issues.
	Warnings []string `json:"warnings,omitempty" yaml:"warnings,omitempty"`

	// Blockers are issues that prevent the upgrade.
	Blockers []string `json:"blockers,omitempty" yaml:"blockers,omitempty"`

	// RequiredSteps are manual steps required before upgrade.
	RequiredSteps []string `json:"required_steps,omitempty" yaml:"required_steps,omitempty"`

	// EstimatedDuration is the estimated upgrade duration.
	EstimatedDuration time.Duration `json:"estimated_duration" yaml:"estimated_duration"`
}

// Plan contains the planned upgrade steps.
type Plan struct {
	// ID is a unique identifier for this plan.
	ID string `json:"id" yaml:"id"`

	// Config is the upgrade configuration.
	Config *Config `json:"config" yaml:"config"`

	// Check is the upgrade compatibility check.
	Check *Check `json:"check" yaml:"check"`

	// Steps are the planned upgrade steps.
	Steps []Step `json:"steps" yaml:"steps"`

	// TotalNodes is the total number of nodes to upgrade.
	TotalNodes int `json:"total_nodes" yaml:"total_nodes"`

	// EstimatedDuration is the estimated total duration.
	EstimatedDuration time.Duration `json:"estimated_duration" yaml:"estimated_duration"`

	// CreatedAt is when the plan was created.
	CreatedAt time.Time `json:"created_at" yaml:"created_at"`
}

// Step represents a single step in an upgrade.
type Step struct {
	// Order is the execution order of this step.
	Order int `json:"order" yaml:"order"`

	// Name is a human-readable name for the step.
	Name string `json:"name" yaml:"name"`

	// Description describes what this step does.
	Description string `json:"description" yaml:"description"`

	// Component being upgraded.
	Component ComponentType `json:"component" yaml:"component"`

	// Nodes to upgrade in this step.
	Nodes []string `json:"nodes" yaml:"nodes"`

	// EstimatedDuration for this step.
	EstimatedDuration time.Duration `json:"estimated_duration" yaml:"estimated_duration"`

	// HealthChecks to perform after this step.
	HealthChecks []HealthCheck `json:"health_checks,omitempty" yaml:"health_checks,omitempty"`

	// Rollbackable indicates if this step can be rolled back.
	Rollbackable bool `json:"rollbackable" yaml:"rollbackable"`
}

// VersionProvider is the interface for retrieving version information.
type VersionProvider interface {
	// GetCurrentVersion returns the current installed version.
	GetCurrentVersion(ctx context.Context, component ComponentType) (Version, error)

	// GetAvailableVersions returns available versions for a component.
	GetAvailableVersions(ctx context.Context, component ComponentType, channel string) ([]VersionInfo, error)

	// GetVersionInfo returns detailed information about a version.
	GetVersionInfo(ctx context.Context, component ComponentType, version string) (*VersionInfo, error)

	// DownloadVersion downloads a specific version.
	DownloadVersion(ctx context.Context, component ComponentType, version string) (string, error)

	// VerifyVersion verifies the integrity of a downloaded version.
	VerifyVersion(ctx context.Context, component ComponentType, version string, path string) error
}

// NodeManager is the interface for managing nodes during upgrades.
type NodeManager interface {
	// GetNodes returns nodes of the specified component type.
	GetNodes(ctx context.Context, component ComponentType) ([]NodeInfo, error)

	// DrainNode removes a node from service.
	DrainNode(ctx context.Context, nodeID string, timeout time.Duration) error

	// UncordonNode returns a node to service.
	UncordonNode(ctx context.Context, nodeID string) error

	// UpgradeNode upgrades a specific node.
	UpgradeNode(ctx context.Context, nodeID string, version string) error

	// GetNodeHealth returns the health status of a node.
	GetNodeHealth(ctx context.Context, nodeID string) (HealthStatus, error)

	// GetNodeVersion returns the version running on a node.
	GetNodeVersion(ctx context.Context, nodeID string) (Version, error)
}

// NodeInfo contains information about a node.
type NodeInfo struct {
	// ID is the unique identifier of the node.
	ID string `json:"id" yaml:"id"`

	// Address is the network address of the node.
	Address string `json:"address" yaml:"address"`

	// Component type running on this node.
	Component ComponentType `json:"component" yaml:"component"`

	// Version currently running.
	Version Version `json:"version" yaml:"version"`

	// Health status.
	Health HealthStatus `json:"health" yaml:"health"`

	// IsLeader indicates if this is the leader node.
	IsLeader bool `json:"is_leader" yaml:"is_leader"`

	// Labels associated with this node.
	Labels map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
}

// Logger is the interface for logging upgrade operations.
type Logger interface {
	Debug(msg string, args ...interface{})
	Info(msg string, args ...interface{})
	Warn(msg string, args ...interface{})
	Error(msg string, args ...interface{})
}

// ProgressCallback is called to report upgrade progress.
type ProgressCallback func(state *State)

// DefaultRollingConfig returns default rolling upgrade configuration.
func DefaultRollingConfig() *RollingConfig {
	return &RollingConfig{
		MaxUnavailable:      1,
		HealthCheckInterval: 10 * time.Second,
		HealthCheckTimeout:  5 * time.Minute,
		DrainTimeout:        2 * time.Minute,
		NodeDelay:           30 * time.Second,
		Order:               "leader_last",
	}
}

// DefaultCanaryConfig returns default canary configuration.
func DefaultCanaryConfig() *CanaryConfig {
	return &CanaryConfig{
		InitialPercentage: 5,
		Increment:         10,
		Interval:          5 * time.Minute,
		SuccessThreshold:  3,
		FailureThreshold:  2,
		QueryTimeout:      15 * time.Second,
	}
}

// DefaultHealthCheckConfig returns default health check configuration.
func DefaultHealthCheckConfig() *HealthCheckConfig {
	return &HealthCheckConfig{
		Enabled:          true,
		SuccessThreshold: 3,
		FailureThreshold: 2,
		Interval:         10 * time.Second,
		Timeout:          5 * time.Second,
		Checks: []HealthCheck{
			{
				Type:           "http",
				Endpoint:       "/health/ready",
				ExpectedStatus: 200,
			},
		},
	}
}

// DefaultRollbackConfig returns default rollback configuration.
func DefaultRollbackConfig() *RollbackConfig {
	return &RollbackConfig{
		Automatic:           true,
		OnFailureCount:      3,
		KeepPreviousVersion: true,
		Timeout:             10 * time.Minute,
	}
}

// DefaultAgentBatchConfig returns default agent batch configuration.
func DefaultAgentBatchConfig() *AgentBatchConfig {
	return &AgentBatchConfig{
		BatchSize:   10,
		BatchDelay:  30 * time.Second,
		MaxFailures: 2,
	}
}
