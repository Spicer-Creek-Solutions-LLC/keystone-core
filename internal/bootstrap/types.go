// Package bootstrap provides infrastructure for bootstrapping Keystone Core clusters
// from scratch, including seed configuration parsing, component installation,
// cluster formation, and handoff to self-management.
package bootstrap

import (
	"context"
	"time"
)

// Mode defines how the bootstrap process operates
type Mode string

const (
	// BootstrapModeSeed creates a new cluster from seed configuration
	BootstrapModeSeed Mode = "seed"
	// BootstrapModeRestore restores from a backup artifact
	BootstrapModeRestore Mode = "restore"
	// BootstrapModeImport imports an existing installation
	BootstrapModeImport Mode = "import"
)

// Phase represents the current phase of bootstrap
type Phase string

// Phase constants define the phases of the bootstrap process.
const (
	PhaseInitializing      Phase = "initializing"
	PhaseValidating        Phase = "validating"
	PhaseInstallingDeps    Phase = "installing_dependencies"
	PhaseInstallingServer  Phase = "installing_server"
	PhaseConfiguringServer Phase = "configuring_server"
	PhaseStartingServer    Phase = "starting_server"
	PhaseFormingCluster    Phase = "forming_cluster"
	PhaseInstallingAgents  Phase = "installing_agents"
	PhaseApplyingStates    Phase = "applying_states"
	PhaseVerifying         Phase = "verifying"
	PhaseHandoff           Phase = "handoff"
	PhaseComplete          Phase = "complete"
	PhaseFailed            Phase = "failed"
)

// Status represents the status of a bootstrap operation
type Status struct {
	Phase       Phase `json:"phase"`
	Message     string         `json:"message"`
	Progress    int            `json:"progress"` // 0-100
	StartTime   time.Time      `json:"start_time"`
	CurrentStep string         `json:"current_step"`
	Error       string         `json:"error,omitempty"`
}

// ComponentType identifies a Keystone Core component
type ComponentType string

// ComponentType constants define the Keystone Core components.
const (
	ComponentServer   ComponentType = "server"
	ComponentAgent    ComponentType = "agent"
	ComponentNATS     ComponentType = "nats"
	ComponentPostgres ComponentType = "postgres"
	ComponentEtcd     ComponentType = "etcd"
)

// ComponentStatus represents the status of an installed component
type ComponentStatus struct {
	Type      ComponentType `json:"type"`
	Version   string        `json:"version"`
	Installed bool          `json:"installed"`
	Running   bool          `json:"running"`
	Healthy   bool          `json:"healthy"`
	Error     string        `json:"error,omitempty"`
}

// InstallerType defines how a component should be installed
type InstallerType string

// InstallerType constants define how components should be installed.
const (
	InstallerPackage   InstallerType = "package"   // apt, yum, etc.
	InstallerBinary    InstallerType = "binary"    // Direct binary download
	InstallerContainer InstallerType = "container" // Docker/Podman
)

// ComponentInstaller defines the interface for installing components
type ComponentInstaller interface {
	// Install installs the component
	Install(ctx context.Context, config ComponentConfig) error
	// Uninstall removes the component
	Uninstall(ctx context.Context) error
	// Configure configures the component
	Configure(ctx context.Context, config ComponentConfig) error
	// Start starts the component service
	Start(ctx context.Context) error
	// Stop stops the component service
	Stop(ctx context.Context) error
	// Status returns the component status
	Status(ctx context.Context) (*ComponentStatus, error)
	// Version returns the installed version
	Version(ctx context.Context) (string, error)
}

// ComponentConfig holds configuration for installing a component
type ComponentConfig struct {
	Type          ComponentType  `yaml:"type" json:"type"`
	Version       string         `yaml:"version" json:"version"`
	InstallerType InstallerType  `yaml:"installer_type" json:"installer_type"`
	ConfigPath    string         `yaml:"config_path" json:"config_path"`
	DataDir       string         `yaml:"data_dir" json:"data_dir"`
	Settings      map[string]any `yaml:"settings" json:"settings"`
}

// NodeRole defines the role of a node in the cluster
type NodeRole string

// NodeRole constants define the roles of nodes in the cluster.
const (
	NodeRoleLeader   NodeRole = "leader"
	NodeRoleFollower NodeRole = "follower"
	NodeRoleWorker   NodeRole = "worker"
)

// NodeConfig represents configuration for a single node
type NodeConfig struct {
	Host       string            `yaml:"host" json:"host"`
	Port       int               `yaml:"port" json:"port"`
	Role       NodeRole          `yaml:"role" json:"role"`
	Labels     map[string]string `yaml:"labels" json:"labels"`
	SSHUser    string            `yaml:"ssh_user" json:"ssh_user"`
	SSHKeyPath string            `yaml:"ssh_key_path" json:"ssh_key_path"`
}

// NATSMode defines how NATS should be deployed
type NATSMode string

// NATSMode constants define how NATS should be deployed.
const (
	NATSModeEmbedded   NATSMode = "embedded"
	NATSModeStandalone NATSMode = "standalone"
	NATSModeCluster    NATSMode = "cluster"
)

// NATSConfig holds NATS-specific configuration
type NATSConfig struct {
	Mode        NATSMode `yaml:"mode" json:"mode"`
	Nodes       []string `yaml:"nodes" json:"nodes"`
	JetStream   bool     `yaml:"jetstream" json:"jetstream"`
	StoreDir    string   `yaml:"store_dir" json:"store_dir"`
	MaxMemory   string   `yaml:"max_memory" json:"max_memory"`
	MaxFile     string   `yaml:"max_file" json:"max_file"`
	ClusterName string   `yaml:"cluster_name" json:"cluster_name"`
	ClusterPort int      `yaml:"cluster_port" json:"cluster_port"`
}

// DatabaseType defines the database backend
type DatabaseType string

// DatabaseType constants define the supported database backends.
const (
	DatabaseTypeSQLite     DatabaseType = "sqlite"
	DatabaseTypePostgreSQL DatabaseType = "postgresql"
)

// DatabaseConfig holds database configuration
type DatabaseConfig struct {
	Type           DatabaseType `yaml:"type" json:"type"`
	Host           string       `yaml:"host" json:"host"`
	Port           int          `yaml:"port" json:"port"`
	Name           string       `yaml:"name" json:"name"`
	User           string       `yaml:"user" json:"user"`
	PasswordSecret string       `yaml:"password_secret" json:"password_secret"` // Secret reference
	SSLMode        string       `yaml:"ssl_mode" json:"ssl_mode"`
	Path           string       `yaml:"path" json:"path"` // For SQLite
}

// EtcdMode defines how etcd should be deployed
type EtcdMode string

// EtcdMode constants define how etcd should be deployed.
const (
	EtcdModeEmbedded EtcdMode = "embedded"
	EtcdModeExternal EtcdMode = "external"
)

// EtcdConfig holds etcd configuration
type EtcdConfig struct {
	Mode     EtcdMode `yaml:"mode" json:"mode"`
	Nodes    []string `yaml:"nodes" json:"nodes"`
	DataDir  string   `yaml:"data_dir" json:"data_dir"`
	CertFile string   `yaml:"cert_file" json:"cert_file"`
	KeyFile  string   `yaml:"key_file" json:"key_file"`
	CAFile   string   `yaml:"ca_file" json:"ca_file"`
}

// TLSConfig holds TLS configuration
type TLSConfig struct {
	Enabled      bool   `yaml:"enabled" json:"enabled"`
	CertFile     string `yaml:"cert_file" json:"cert_file"`
	KeyFile      string `yaml:"key_file" json:"key_file"`
	CAFile       string `yaml:"ca_file" json:"ca_file"`
	AutoGenerate bool   `yaml:"auto_generate" json:"auto_generate"`
}

// APIConfig holds API server configuration
type APIConfig struct {
	Listen string    `yaml:"listen" json:"listen"`
	TLS    TLSConfig `yaml:"tls" json:"tls"`
}

// ControlPlaneConfig holds control plane configuration
type ControlPlaneConfig struct {
	Replicas int          `yaml:"replicas" json:"replicas"`
	Nodes    []NodeConfig `yaml:"nodes" json:"nodes"`
	API      APIConfig    `yaml:"api" json:"api"`
}

// InitialAgentConfig defines an agent to install during bootstrap
type InitialAgentConfig struct {
	Host       string            `yaml:"host" json:"host"`
	Labels     map[string]string `yaml:"labels" json:"labels"`
	SSHUser    string            `yaml:"ssh_user" json:"ssh_user"`
	SSHKeyPath string            `yaml:"ssh_key_path" json:"ssh_key_path"`
}

// SeedConfig represents the complete seed configuration for bootstrapping
type SeedConfig struct {
	Cluster       ClusterConfig        `yaml:"cluster" json:"cluster"`
	ControlPlane  ControlPlaneConfig   `yaml:"control_plane" json:"control_plane"`
	NATS          NATSConfig           `yaml:"nats" json:"nats"`
	Database      DatabaseConfig       `yaml:"database" json:"database"`
	Etcd          EtcdConfig           `yaml:"etcd" json:"etcd"`
	InitialAgents []InitialAgentConfig `yaml:"initial_agents" json:"initial_agents"`
	PostBootstrap PostBootstrapConfig  `yaml:"post_bootstrap" json:"post_bootstrap"`
}

// ClusterConfig holds cluster-level configuration
type ClusterConfig struct {
	Name   string `yaml:"name" json:"name"`
	Domain string `yaml:"domain" json:"domain"`
}

// PostBootstrapConfig defines actions to take after bootstrap
type PostBootstrapConfig struct {
	ApplyStates    bool   `yaml:"apply_states" json:"apply_states"`
	VerifyHealth   bool   `yaml:"verify_health" json:"verify_health"`
	RegisterAgents bool   `yaml:"register_agents" json:"register_agents"`
	StatesPath     string `yaml:"states_path" json:"states_path"`
}

// Options configures how bootstrap should run
type Options struct {
	Mode             Mode `yaml:"mode" json:"mode"`
	SeedConfigPath   string        `yaml:"seed_config_path" json:"seed_config_path"`
	BackupPath       string        `yaml:"backup_path" json:"backup_path"`
	DecryptionKey    string        `yaml:"decryption_key" json:"decryption_key"`
	OutputDir        string        `yaml:"output_dir" json:"output_dir"`
	DryRun           bool          `yaml:"dry_run" json:"dry_run"`
	Verbose          bool          `yaml:"verbose" json:"verbose"`
	SkipVerification bool          `yaml:"skip_verification" json:"skip_verification"`
	Force            bool          `yaml:"force" json:"force"`
}

// Result holds the result of a bootstrap operation
type Result struct {
	Success       bool              `json:"success"`
	ClusterID     string            `json:"cluster_id"`
	APIEndpoint   string            `json:"api_endpoint"`
	AdminToken    string            `json:"admin_token,omitempty"`
	CAFingerprint string            `json:"ca_fingerprint"`
	Components    []ComponentStatus `json:"components"`
	Duration      time.Duration     `json:"duration"`
	Error         string            `json:"error,omitempty"`
}

// Bootstrapper defines the interface for bootstrap operations
type Bootstrapper interface {
	// Bootstrap performs the full bootstrap process
	Bootstrap(ctx context.Context, opts Options) (*Result, error)
	// Status returns the current bootstrap status
	Status() *Status
	// Verify verifies the bootstrap result
	Verify(ctx context.Context) error
	// Cleanup removes partial bootstrap artifacts on failure
	Cleanup(ctx context.Context) error
}

// ProgressCallback is called to report bootstrap progress
type ProgressCallback func(status *Status)

// Logger defines the interface for bootstrap logging
type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}
