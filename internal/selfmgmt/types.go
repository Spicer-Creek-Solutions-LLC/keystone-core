// Package selfmgmt provides state modules for managing Keystone Core components.
// These modules enable infrastructure-as-code for the control plane itself,
// allowing Keystone Core to be self-managed through declarative state files.
package selfmgmt

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ComponentType represents a Keystone Core component
type ComponentType string

const (
	ComponentServer   ComponentType = "server"
	ComponentAgent    ComponentType = "agent"
	ComponentNATS     ComponentType = "nats"
	ComponentDatabase ComponentType = "database"
	ComponentBackup   ComponentType = "backup"
)

// ComponentState represents the desired state of a component
type ComponentState string

const (
	StateInstalled   ComponentState = "installed"
	StateUninstalled ComponentState = "uninstalled"
	StateRunning     ComponentState = "running"
	StateStopped     ComponentState = "stopped"
	StateConfigured  ComponentState = "configured"
	StateEnabled     ComponentState = "enabled"
	StateDisabled    ComponentState = "disabled"
)

// InstallMethod represents how to install a component
type InstallMethod string

const (
	InstallMethodPackage InstallMethod = "package" // System package manager
	InstallMethodBinary  InstallMethod = "binary"  // Direct binary download
	InstallMethodDocker  InstallMethod = "docker"  // Docker container
	InstallMethodHelm    InstallMethod = "helm"    // Kubernetes Helm chart

	// Aliases for compatibility
	InstallPackage = InstallMethodPackage
	InstallBinary  = InstallMethodBinary
	InstallDocker  = InstallMethodDocker
	InstallHelm    = InstallMethodHelm
)

// Logger interface for self-management operations
type Logger interface {
	Debug(msg string, args ...interface{})
	Info(msg string, args ...interface{})
	Warn(msg string, args ...interface{})
	Error(msg string, args ...interface{})
}

// noopLogger is a no-op logger
type noopLogger struct{}

func (l *noopLogger) Debug(msg string, args ...interface{}) {}
func (l *noopLogger) Info(msg string, args ...interface{})  {}
func (l *noopLogger) Warn(msg string, args ...interface{})  {}
func (l *noopLogger) Error(msg string, args ...interface{}) {}

// SelfMgmtModule is the interface for self-management modules
type SelfMgmtModule interface {
	// Name returns the module name
	Name() string

	// ComponentType returns the type of component this module manages
	ComponentType() ComponentType

	// ValidStates returns valid state values
	ValidStates() []ComponentState

	// Check checks the current state
	Check(ctx context.Context, config interface{}) (*CheckResult, error)

	// Apply applies the desired state
	Apply(ctx context.Context, config interface{}, dryRun bool) (*ApplyResult, error)

	// Validate validates the configuration
	Validate(config interface{}) error
}

// CheckResult contains the result of checking current state
type CheckResult struct {
	// Component being checked
	Component ComponentType

	// Present indicates if the component exists
	Present bool

	// CurrentState is the current state
	CurrentState ComponentState

	// DesiredState is the desired state
	DesiredState ComponentState

	// Matches indicates if current state matches desired state
	Matches bool

	// Installed version
	InstalledVersion string

	// DesiredVersion is the desired version
	DesiredVersion string

	// Running indicates if the component is running
	Running bool

	// Enabled indicates if the component is enabled at boot
	Enabled bool

	// ConfigValid indicates if configuration is valid
	ConfigValid bool

	// ConfigPath is the path to the configuration file
	ConfigPath string

	// Diff contains details of what would change
	Diff map[string]interface{}

	// Metadata contains additional information
	Metadata map[string]interface{}

	// Errors encountered during check
	Errors []string

	// Warnings encountered during check
	Warnings []string
}

// ApplyResult contains the result of applying state
type ApplyResult struct {
	// Component being managed
	Component ComponentType

	// Success indicates if the operation succeeded
	Success bool

	// Changed indicates if any changes were made
	Changed bool

	// Changes contains details of changes made
	Changes map[string]interface{}

	// PreviousState is the state before apply
	PreviousState ComponentState

	// NewState is the state after apply
	NewState ComponentState

	// StartTime when operation started
	StartTime time.Time

	// EndTime when operation completed
	EndTime time.Time

	// Duration of the operation
	Duration time.Duration

	// Comment describing the result
	Comment string

	// Error if operation failed
	Error error

	// RequiresRestart indicates if a restart is needed
	RequiresRestart bool

	// RestartedServices lists services that were restarted
	RestartedServices []string
}

// BaseConfig contains common configuration for all components
type BaseConfig struct {
	// State is the desired state
	State ComponentState `yaml:"state" json:"state"`

	// Version is the desired version (empty for latest)
	Version string `yaml:"version,omitempty" json:"version,omitempty"`

	// Channel is the release channel (stable, beta, nightly)
	Channel string `yaml:"channel,omitempty" json:"channel,omitempty"`

	// InstallMethod is how to install the component
	InstallMethod InstallMethod `yaml:"install_method,omitempty" json:"install_method,omitempty"`

	// ConfigPath is the path to the configuration file
	ConfigPath string `yaml:"config_path,omitempty" json:"config_path,omitempty"`

	// BinaryPath is the path to the binary
	BinaryPath string `yaml:"binary_path,omitempty" json:"binary_path,omitempty"`

	// DataDir is the data directory
	DataDir string `yaml:"data_dir,omitempty" json:"data_dir,omitempty"`

	// LogDir is the log directory
	LogDir string `yaml:"log_dir,omitempty" json:"log_dir,omitempty"`

	// User to run the service as
	User string `yaml:"user,omitempty" json:"user,omitempty"`

	// Group to run the service as
	Group string `yaml:"group,omitempty" json:"group,omitempty"`

	// Environment variables
	Environment map[string]string `yaml:"environment,omitempty" json:"environment,omitempty"`

	// DryRun mode - don't make actual changes
	DryRun bool `yaml:"dry_run,omitempty" json:"dry_run,omitempty"`
}

// ServerConfig is configuration for kscore-server
type ServerConfig struct {
	BaseConfig `yaml:",inline" json:",inline"`

	// ClusterID is the cluster identifier
	ClusterID string `yaml:"cluster_id,omitempty" json:"cluster_id,omitempty"`

	// ListenAddress for the API server
	ListenAddress string `yaml:"listen_address,omitempty" json:"listen_address,omitempty"`

	// GRPCAddress for gRPC server
	GRPCAddress string `yaml:"grpc_address,omitempty" json:"grpc_address,omitempty"`

	// NATSURLs are the NATS server URLs
	NATSURLs []string `yaml:"nats_urls,omitempty" json:"nats_urls,omitempty"`

	// NATSMode is the NATS mode (embedded, external, leaf)
	NATSMode string `yaml:"nats_mode,omitempty" json:"nats_mode,omitempty"`

	// DatabaseType is the database type (sqlite, postgresql)
	DatabaseType string `yaml:"database_type,omitempty" json:"database_type,omitempty"`

	// DatabaseURL is the database connection URL
	DatabaseURL string `yaml:"database_url,omitempty" json:"database_url,omitempty"`

	// TLSEnabled enables TLS
	TLSEnabled bool `yaml:"tls_enabled,omitempty" json:"tls_enabled,omitempty"`

	// TLSCertFile is the TLS certificate file
	TLSCertFile string `yaml:"tls_cert_file,omitempty" json:"tls_cert_file,omitempty"`

	// TLSKeyFile is the TLS key file
	TLSKeyFile string `yaml:"tls_key_file,omitempty" json:"tls_key_file,omitempty"`

	// TLSCAFile is the TLS CA file
	TLSCAFile string `yaml:"tls_ca_file,omitempty" json:"tls_ca_file,omitempty"`

	// EnableClustering enables HA clustering
	EnableClustering bool `yaml:"enable_clustering,omitempty" json:"enable_clustering,omitempty"`

	// EtcdEndpoints for clustering
	EtcdEndpoints []string `yaml:"etcd_endpoints,omitempty" json:"etcd_endpoints,omitempty"`
}

// AgentConfig is configuration for kscore-agent
type AgentConfig struct {
	BaseConfig `yaml:",inline" json:",inline"`

	// AgentID is the agent identifier
	AgentID string `yaml:"agent_id,omitempty" json:"agent_id,omitempty"`

	// ServerURL is the control plane URL
	ServerURL string `yaml:"server_url,omitempty" json:"server_url,omitempty"`

	// NATSURLs for direct NATS connection
	NATSURLs []string `yaml:"nats_urls,omitempty" json:"nats_urls,omitempty"`

	// Labels for the agent
	Labels map[string]string `yaml:"labels,omitempty" json:"labels,omitempty"`

	// HeartbeatInterval is the heartbeat interval
	HeartbeatInterval time.Duration `yaml:"heartbeat_interval,omitempty" json:"heartbeat_interval,omitempty"`

	// TLSEnabled enables TLS
	TLSEnabled bool `yaml:"tls_enabled,omitempty" json:"tls_enabled,omitempty"`

	// TLSCertFile is the TLS certificate file
	TLSCertFile string `yaml:"tls_cert_file,omitempty" json:"tls_cert_file,omitempty"`

	// TLSKeyFile is the TLS key file
	TLSKeyFile string `yaml:"tls_key_file,omitempty" json:"tls_key_file,omitempty"`

	// TLSCAFile is the TLS CA file
	TLSCAFile string `yaml:"tls_ca_file,omitempty" json:"tls_ca_file,omitempty"`

	// EmbeddedNATS enables embedded NATS (leaf node mode)
	EmbeddedNATS bool `yaml:"embedded_nats,omitempty" json:"embedded_nats,omitempty"`
}

// NATSConfig is configuration for NATS
type NATSConfig struct {
	BaseConfig `yaml:",inline" json:",inline"`

	// Mode is the NATS mode (standalone, cluster, leaf)
	Mode string `yaml:"mode,omitempty" json:"mode,omitempty"`

	// ClientPort is the client connection port
	ClientPort int `yaml:"client_port,omitempty" json:"client_port,omitempty"`

	// ClusterPort is the cluster port
	ClusterPort int `yaml:"cluster_port,omitempty" json:"cluster_port,omitempty"`

	// HTTPPort is the HTTP monitoring port
	HTTPPort int `yaml:"http_port,omitempty" json:"http_port,omitempty"`

	// ClusterName is the cluster name
	ClusterName string `yaml:"cluster_name,omitempty" json:"cluster_name,omitempty"`

	// Routes are the cluster routes
	Routes []string `yaml:"routes,omitempty" json:"routes,omitempty"`

	// JetStreamEnabled enables JetStream
	JetStreamEnabled bool `yaml:"jetstream_enabled,omitempty" json:"jetstream_enabled,omitempty"`

	// JetStreamStorePath is the JetStream storage path
	JetStreamStorePath string `yaml:"jetstream_store_path,omitempty" json:"jetstream_store_path,omitempty"`

	// JetStreamMaxMemory is the max memory for JetStream
	JetStreamMaxMemory int64 `yaml:"jetstream_max_memory,omitempty" json:"jetstream_max_memory,omitempty"`

	// JetStreamMaxFile is the max file storage for JetStream
	JetStreamMaxFile int64 `yaml:"jetstream_max_file,omitempty" json:"jetstream_max_file,omitempty"`

	// TLSEnabled enables TLS
	TLSEnabled bool `yaml:"tls_enabled,omitempty" json:"tls_enabled,omitempty"`

	// TLSCertFile is the TLS certificate file
	TLSCertFile string `yaml:"tls_cert_file,omitempty" json:"tls_cert_file,omitempty"`

	// TLSKeyFile is the TLS key file
	TLSKeyFile string `yaml:"tls_key_file,omitempty" json:"tls_key_file,omitempty"`

	// TLSCAFile is the TLS CA file
	TLSCAFile string `yaml:"tls_ca_file,omitempty" json:"tls_ca_file,omitempty"`

	// Authorization configuration
	Authorization *NATSAuthConfig `yaml:"authorization,omitempty" json:"authorization,omitempty"`
}

// NATSAuthConfig is NATS authorization configuration
type NATSAuthConfig struct {
	// Users are the configured users
	Users []NATSUser `yaml:"users,omitempty" json:"users,omitempty"`

	// Token is a simple token auth
	Token string `yaml:"token,omitempty" json:"token,omitempty"`
}

// NATSUser is a NATS user configuration
type NATSUser struct {
	User      string   `yaml:"user" json:"user"`
	Password  string   `yaml:"password" json:"password"`
	Publish   []string `yaml:"publish,omitempty" json:"publish,omitempty"`
	Subscribe []string `yaml:"subscribe,omitempty" json:"subscribe,omitempty"`
}

// DatabaseConfig is configuration for database management
type DatabaseConfig struct {
	BaseConfig `yaml:",inline" json:",inline"`

	// Type is the database type (sqlite, postgresql)
	Type string `yaml:"type,omitempty" json:"type,omitempty"`

	// Host is the database host
	Host string `yaml:"host,omitempty" json:"host,omitempty"`

	// Port is the database port
	Port int `yaml:"port,omitempty" json:"port,omitempty"`

	// Name is the database name
	Name string `yaml:"name,omitempty" json:"name,omitempty"`

	// User is the database user
	DBUser string `yaml:"db_user,omitempty" json:"db_user,omitempty"`

	// Password is the database password
	Password string `yaml:"password,omitempty" json:"password,omitempty"`

	// SSLMode is the SSL mode (disable, require, verify-ca, verify-full)
	SSLMode string `yaml:"ssl_mode,omitempty" json:"ssl_mode,omitempty"`

	// SQLitePath is the SQLite database path
	SQLitePath string `yaml:"sqlite_path,omitempty" json:"sqlite_path,omitempty"`

	// MaxConnections is the max connection pool size
	MaxConnections int `yaml:"max_connections,omitempty" json:"max_connections,omitempty"`

	// MigrationsPath is the path to migration files
	MigrationsPath string `yaml:"migrations_path,omitempty" json:"migrations_path,omitempty"`

	// AutoMigrate enables automatic migrations
	AutoMigrate bool `yaml:"auto_migrate,omitempty" json:"auto_migrate,omitempty"`
}

// BackupConfig is configuration for backup management
type BackupConfig struct {
	BaseConfig `yaml:",inline" json:",inline"`

	// Schedule is the backup schedule (cron format)
	Schedule string `yaml:"schedule,omitempty" json:"schedule,omitempty"`

	// Destinations are the backup destinations
	Destinations []BackupDestination `yaml:"destinations,omitempty" json:"destinations,omitempty"`

	// Retention is the retention policy
	Retention *BackupRetention `yaml:"retention,omitempty" json:"retention,omitempty"`

	// EncryptionType is the encryption type
	EncryptionType string `yaml:"encryption_type,omitempty" json:"encryption_type,omitempty"`

	// EncryptionKey is the encryption key/recipient
	EncryptionKey string `yaml:"encryption_key,omitempty" json:"encryption_key,omitempty"`

	// IncludeDatabase includes database in backup
	IncludeDatabase bool `yaml:"include_database,omitempty" json:"include_database,omitempty"`

	// IncludeConfig includes configuration in backup
	IncludeConfig bool `yaml:"include_config,omitempty" json:"include_config,omitempty"`

	// IncludeSecrets includes secrets in backup
	IncludeSecrets bool `yaml:"include_secrets,omitempty" json:"include_secrets,omitempty"`

	// IncludeJetStream includes JetStream in backup
	IncludeJetStream bool `yaml:"include_jetstream,omitempty" json:"include_jetstream,omitempty"`

	// IncludeEtcd includes etcd in backup
	IncludeEtcd bool `yaml:"include_etcd,omitempty" json:"include_etcd,omitempty"`
}

// BackupDestination is a backup destination configuration
type BackupDestination struct {
	// Name is the destination name
	Name string `yaml:"name" json:"name"`

	// Type is the destination type (s3, gcs, azure, local, sftp)
	Type string `yaml:"type" json:"type"`

	// Bucket is the bucket name (for cloud storage)
	Bucket string `yaml:"bucket,omitempty" json:"bucket,omitempty"`

	// Region is the region (for cloud storage)
	Region string `yaml:"region,omitempty" json:"region,omitempty"`

	// Prefix is the path prefix
	Prefix string `yaml:"prefix,omitempty" json:"prefix,omitempty"`

	// Path is the local path
	Path string `yaml:"path,omitempty" json:"path,omitempty"`

	// Credentials for the destination
	Credentials map[string]string `yaml:"credentials,omitempty" json:"credentials,omitempty"`
}

// BackupRetention is backup retention configuration
type BackupRetention struct {
	// KeepLast keeps the last N backups
	KeepLast int `yaml:"keep_last,omitempty" json:"keep_last,omitempty"`

	// KeepHourly keeps hourly backups for N hours
	KeepHourly int `yaml:"keep_hourly,omitempty" json:"keep_hourly,omitempty"`

	// KeepDaily keeps daily backups for N days
	KeepDaily int `yaml:"keep_daily,omitempty" json:"keep_daily,omitempty"`

	// KeepWeekly keeps weekly backups for N weeks
	KeepWeekly int `yaml:"keep_weekly,omitempty" json:"keep_weekly,omitempty"`

	// KeepMonthly keeps monthly backups for N months
	KeepMonthly int `yaml:"keep_monthly,omitempty" json:"keep_monthly,omitempty"`

	// KeepYearly keeps yearly backups for N years
	KeepYearly int `yaml:"keep_yearly,omitempty" json:"keep_yearly,omitempty"`
}

// PostgreSQLConfig is PostgreSQL-specific configuration (for backward compat)
type PostgreSQLConfig struct {
	Host     string `yaml:"host,omitempty" json:"host,omitempty"`
	Port     int    `yaml:"port,omitempty" json:"port,omitempty"`
	Database string `yaml:"database,omitempty" json:"database,omitempty"`
	User     string `yaml:"user,omitempty" json:"user,omitempty"`
	Password string `yaml:"password,omitempty" json:"password,omitempty"`
	SSLMode  string `yaml:"sslmode,omitempty" json:"sslmode,omitempty"`
}

// SQLiteConfig is SQLite-specific configuration (for backward compat)
type SQLiteConfig struct {
	Path string `yaml:"path,omitempty" json:"path,omitempty"`
}

// GetPostgreSQLConfig returns PostgreSQL config from DatabaseConfig
func (c *DatabaseConfig) GetPostgreSQLConfig() *PostgreSQLConfig {
	return &PostgreSQLConfig{
		Host:     c.Host,
		Port:     c.Port,
		Database: c.Name,
		User:     c.DBUser,
		Password: c.Password,
		SSLMode:  c.SSLMode,
	}
}

// GetSQLiteConfig returns SQLite config from DatabaseConfig
func (c *DatabaseConfig) GetSQLiteConfig() *SQLiteConfig {
	return &SQLiteConfig{
		Path: c.SQLitePath,
	}
}

// GetDestination returns the first local destination path (for simple backup configs)
func (c *BackupConfig) GetDestination() string {
	for _, dest := range c.Destinations {
		if dest.Type == "local" && dest.Path != "" {
			return dest.Path
		}
	}
	if len(c.Destinations) > 0 {
		return c.Destinations[0].Path
	}
	return ""
}

// GetRetentionDays returns the retention days from the retention policy
func (c *BackupConfig) GetRetentionDays() int {
	if c.Retention != nil {
		return c.Retention.KeepDaily
	}
	return 0
}

// Helper functions

// DetectInitSystem detects the init system on the current platform
func DetectInitSystem() string {
	switch runtime.GOOS {
	case "linux":
		// Check for systemd
		if _, err := os.Stat("/run/systemd/system"); err == nil {
			return "systemd"
		}
		// Check for OpenRC
		if _, err := os.Stat("/sbin/openrc"); err == nil {
			return "openrc"
		}
		// Check for Upstart
		if _, err := os.Stat("/sbin/initctl"); err == nil {
			return "upstart"
		}
		return "sysvinit"
	case "darwin":
		return "launchd"
	case "windows":
		return "windows"
	default:
		return "unknown"
	}
}

// DetectPackageManager detects the package manager on the current platform
func DetectPackageManager() string {
	switch runtime.GOOS {
	case "linux":
		// Check for apt
		if _, err := exec.LookPath("apt-get"); err == nil {
			return "apt"
		}
		// Check for dnf
		if _, err := exec.LookPath("dnf"); err == nil {
			return "dnf"
		}
		// Check for yum
		if _, err := exec.LookPath("yum"); err == nil {
			return "yum"
		}
		// Check for apk
		if _, err := exec.LookPath("apk"); err == nil {
			return "apk"
		}
		// Check for pacman
		if _, err := exec.LookPath("pacman"); err == nil {
			return "pacman"
		}
		// Check for zypper
		if _, err := exec.LookPath("zypper"); err == nil {
			return "zypper"
		}
		return "unknown"
	case "darwin":
		if _, err := exec.LookPath("brew"); err == nil {
			return "brew"
		}
		return "unknown"
	case "windows":
		if _, err := exec.LookPath("choco"); err == nil {
			return "chocolatey"
		}
		if _, err := exec.LookPath("winget"); err == nil {
			return "winget"
		}
		return "unknown"
	default:
		return "unknown"
	}
}

// GetDefaultConfigPath returns the default configuration path for a component
func GetDefaultConfigPath(component ComponentType) string {
	switch runtime.GOOS {
	case "windows":
		programData := os.Getenv("PROGRAMDATA")
		if programData == "" {
			programData = `C:\ProgramData`
		}
		switch component {
		case ComponentServer:
			return filepath.Join(programData, "kscore", "server.yaml")
		case ComponentAgent:
			return filepath.Join(programData, "kscore", "agent.yaml")
		case ComponentNATS:
			return filepath.Join(programData, "nats", "nats-server.conf")
		case ComponentDatabase:
			return filepath.Join(programData, "kscore", "database.yaml")
		case ComponentBackup:
			return filepath.Join(programData, "kscore", "backup.yaml")
		}
	default:
		switch component {
		case ComponentServer:
			return "/etc/kscore/server.yaml"
		case ComponentAgent:
			return "/etc/kscore/agent.yaml"
		case ComponentNATS:
			return "/etc/nats/nats-server.conf"
		case ComponentDatabase:
			return "/etc/kscore/database.yaml"
		case ComponentBackup:
			return "/etc/kscore/backup.yaml"
		}
	}
	return ""
}

// GetDefaultDataDir returns the default data directory for a component
func GetDefaultDataDir(component ComponentType) string {
	switch runtime.GOOS {
	case "windows":
		programData := os.Getenv("PROGRAMDATA")
		if programData == "" {
			programData = `C:\ProgramData`
		}
		switch component {
		case ComponentServer, ComponentAgent, ComponentDatabase, ComponentBackup:
			return filepath.Join(programData, "kscore", "data")
		case ComponentNATS:
			return filepath.Join(programData, "nats", "data")
		}
	default:
		switch component {
		case ComponentServer, ComponentAgent, ComponentDatabase, ComponentBackup:
			return "/var/lib/kscore"
		case ComponentNATS:
			return "/var/lib/nats"
		}
	}
	return ""
}

// GetDefaultKscoreConfigPath returns the default kscore config directory
func GetDefaultKscoreConfigPath() string {
	switch runtime.GOOS {
	case "windows":
		programData := os.Getenv("PROGRAMDATA")
		if programData == "" {
			programData = `C:\ProgramData`
		}
		return filepath.Join(programData, "kscore")
	default:
		return "/etc/kscore"
	}
}

// GetDefaultKscoreDataDir returns the default kscore data directory
func GetDefaultKscoreDataDir() string {
	switch runtime.GOOS {
	case "windows":
		programData := os.Getenv("PROGRAMDATA")
		if programData == "" {
			programData = `C:\ProgramData`
		}
		return filepath.Join(programData, "kscore", "data")
	default:
		return "/var/lib/kscore"
	}
}

// GetDefaultBinaryPath returns the default binary path for a component
func GetDefaultBinaryPath(component ComponentType) string {
	switch runtime.GOOS {
	case "windows":
		programFiles := os.Getenv("PROGRAMFILES")
		if programFiles == "" {
			programFiles = `C:\Program Files`
		}
		switch component {
		case ComponentServer:
			return filepath.Join(programFiles, "kscore", "kscore-server.exe")
		case ComponentAgent:
			return filepath.Join(programFiles, "kscore", "kscore-agent.exe")
		case ComponentNATS:
			return filepath.Join(programFiles, "nats", "nats-server.exe")
		}
	default:
		switch component {
		case ComponentServer:
			return "/usr/bin/kscore-server"
		case ComponentAgent:
			return "/usr/bin/kscore-agent"
		case ComponentNATS:
			return "/usr/bin/nats-server"
		}
	}
	return ""
}

// ServiceName returns the service name for a component
func ServiceName(component ComponentType) string {
	switch component {
	case ComponentServer:
		return "kscore-server"
	case ComponentAgent:
		return "kscore-agent"
	case ComponentNATS:
		return "nats-server"
	default:
		return string(component)
	}
}

// RunCommand runs a command and returns output
func RunCommand(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	output, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(output)), err
}

// FileExists checks if a file exists
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// EnsureDir ensures a directory exists
func EnsureDir(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

// WriteFile writes content to a file
func WriteFile(path string, content []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := EnsureDir(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(path, content, perm)
}

// CopyFile copies a file from src to dst
func CopyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	return WriteFile(dst, data, info.Mode())
}

// ValidationError represents a validation error
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// ValidationErrors is a collection of validation errors
type ValidationErrors []ValidationError

func (e ValidationErrors) Error() string {
	if len(e) == 0 {
		return "no errors"
	}
	var msgs []string
	for _, err := range e {
		msgs = append(msgs, err.Error())
	}
	return strings.Join(msgs, "; ")
}

// HasErrors returns true if there are errors
func (e ValidationErrors) HasErrors() bool {
	return len(e) > 0
}

// Add adds a validation error
func (e *ValidationErrors) Add(field, message string) {
	*e = append(*e, ValidationError{Field: field, Message: message})
}
