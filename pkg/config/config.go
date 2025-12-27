package config

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

// NATSMode defines the NATS deployment mode
type NATSMode string

const (
	// NATSModeEmbedded runs NATS server in-process
	NATSModeEmbedded NATSMode = "embedded"
	// NATSModeExternal connects to external NATS cluster
	NATSModeExternal NATSMode = "external"
	// NATSModeLeaf runs embedded NATS as a leaf node
	NATSModeLeaf NATSMode = "leaf"
)

// StorageBackend defines the state storage backend
type StorageBackend string

const (
	// StorageBackendSQLite uses embedded SQLite
	StorageBackendSQLite StorageBackend = "sqlite"
	// StorageBackendPostgreSQL uses external PostgreSQL
	StorageBackendPostgreSQL StorageBackend = "postgresql"
)

// Config represents the complete Keystone Core configuration
type Config struct {
	Server  ServerConfig
	NATS    NATSConfig
	Storage StorageConfig
	Agent   AgentConfig
	TLS     TLSConfig
}

// ServerConfig contains control plane server settings
type ServerConfig struct {
	// Listen address for API server
	ListenAddr string
	// gRPC port
	GRPCPort int
	// HTTP/REST port
	HTTPPort int
}

// NATSConfig contains NATS connection settings
type NATSConfig struct {
	// Mode: embedded, external, or leaf
	Mode NATSMode
	// URL for external NATS cluster (used when Mode=external or leaf)
	URL string
	// Embedded NATS settings
	Embedded NATSEmbeddedConfig
	// JetStream settings
	JetStream JetStreamConfig
	// Connection settings
	MaxReconnects int
	ReconnectWait time.Duration
	// Authentication
	Token      string
	Credential string
}

// NATSEmbeddedConfig contains settings for embedded NATS mode
type NATSEmbeddedConfig struct {
	// Port for embedded NATS server
	Port int
	// Enable JetStream for embedded mode
	EnableJetStream bool
	// Storage directory for JetStream
	StoreDir string
	// Maximum memory for embedded NATS (bytes)
	MaxMemory int64
	// Maximum number of connections
	MaxConnections int
	// Leaf node parent URLs (for leaf mode)
	LeafNodeURLs []string
}

// JetStreamConfig contains JetStream settings
type JetStreamConfig struct {
	// Enable JetStream
	Enabled bool
	// Storage directory (for embedded mode)
	StoreDir string
	// Maximum storage size (bytes)
	MaxStorage int64
}

// StorageConfig contains state storage settings
type StorageConfig struct {
	// Backend: sqlite or postgresql
	Backend StorageBackend
	// SQLite settings
	SQLite SQLiteConfig
	// PostgreSQL settings
	PostgreSQL PostgreSQLConfig
}

// SQLiteConfig contains SQLite-specific settings
type SQLiteConfig struct {
	// Database file path
	Path string
	// Enable WAL mode for better concurrency
	WAL bool
	// Busy timeout (milliseconds)
	BusyTimeout int
}

// PostgreSQLConfig contains PostgreSQL-specific settings
type PostgreSQLConfig struct {
	// Connection string
	DSN string
	// Maximum open connections
	MaxOpenConns int
	// Maximum idle connections
	MaxIdleConns int
	// Connection max lifetime
	ConnMaxLifetime time.Duration
}

// AgentConfig contains agent-specific settings
type AgentConfig struct {
	// Agent ID (auto-generated if empty)
	ID string
	// Heartbeat interval
	HeartbeatInterval time.Duration
	// Command execution timeout
	CommandTimeout time.Duration
	// System metadata collection interval
	MetadataInterval time.Duration
}

// TLSConfig contains TLS/mTLS settings
type TLSConfig struct {
	// Enable mTLS
	Enabled bool
	// Certificate file path
	CertFile string
	// Private key file path
	KeyFile string
	// CA certificate file path
	CAFile string
	// Skip TLS verification (insecure, for testing only)
	InsecureSkipVerify bool
}

// Default configuration values
const (
	DefaultServerListenAddr = "0.0.0.0"
	DefaultGRPCPort         = 9090
	DefaultHTTPPort         = 8080

	DefaultNATSMode          = NATSModeEmbedded
	DefaultNATSEmbeddedPort  = 4222
	DefaultNATSMaxReconnects = -1 // unlimited
	DefaultNATSReconnectWait = 2 * time.Second

	DefaultStorageBackend     = StorageBackendSQLite
	DefaultSQLitePath         = "./data/keystone-core.db"
	DefaultSQLiteWAL          = true
	DefaultSQLiteBusyTimeout  = 5000 // 5 seconds
	DefaultPostgreSQLMaxOpen  = 25
	DefaultPostgreSQLMaxIdle  = 5
	DefaultPostgreSQLConnLife = 5 * time.Minute

	DefaultHeartbeatInterval = 30 * time.Second
	DefaultCommandTimeout    = 5 * time.Minute
	DefaultMetadataInterval  = 5 * time.Minute
)

// LoadConfig loads configuration from file and environment variables
func LoadConfig(cfgFile string) (*Config, error) {
	v := viper.New()

	// Set defaults
	setDefaults(v)

	// Configuration file settings
	if cfgFile != "" {
		v.SetConfigFile(cfgFile)
	} else {
		v.SetConfigName("keystone-core")
		v.SetConfigType("yaml")
		v.AddConfigPath("/etc/keystone-core/")
		v.AddConfigPath("$HOME/.keystone-core")
		v.AddConfigPath(".")
	}

	// Read from environment variables
	v.SetEnvPrefix("TITAN")
	v.AutomaticEnv()

	// Read config file (optional)
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	}

	// Unmarshal into config struct
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &cfg, nil
}

// setDefaults sets default configuration values
func setDefaults(v *viper.Viper) {
	// Server defaults
	v.SetDefault("server.listenaddr", DefaultServerListenAddr)
	v.SetDefault("server.grpcport", DefaultGRPCPort)
	v.SetDefault("server.httpport", DefaultHTTPPort)

	// NATS defaults
	v.SetDefault("nats.mode", DefaultNATSMode)
	v.SetDefault("nats.embedded.port", DefaultNATSEmbeddedPort)
	v.SetDefault("nats.embedded.enablejetstream", true)
	v.SetDefault("nats.embedded.storedir", "./data/nats")
	v.SetDefault("nats.embedded.maxmemory", 1024*1024*1024) // 1GB
	v.SetDefault("nats.embedded.maxconnections", 1000)
	v.SetDefault("nats.jetstream.enabled", true)
	v.SetDefault("nats.jetstream.storedir", "./data/jetstream")
	v.SetDefault("nats.jetstream.maxstorage", 10*1024*1024*1024) // 10GB
	v.SetDefault("nats.maxreconnects", DefaultNATSMaxReconnects)
	v.SetDefault("nats.reconnectwait", DefaultNATSReconnectWait)

	// Storage defaults
	v.SetDefault("storage.backend", DefaultStorageBackend)
	v.SetDefault("storage.sqlite.path", DefaultSQLitePath)
	v.SetDefault("storage.sqlite.wal", DefaultSQLiteWAL)
	v.SetDefault("storage.sqlite.busytimeout", DefaultSQLiteBusyTimeout)
	v.SetDefault("storage.postgresql.maxopenconns", DefaultPostgreSQLMaxOpen)
	v.SetDefault("storage.postgresql.maxidleconns", DefaultPostgreSQLMaxIdle)
	v.SetDefault("storage.postgresql.connmaxlifetime", DefaultPostgreSQLConnLife)

	// Agent defaults
	v.SetDefault("agent.heartbeatinterval", DefaultHeartbeatInterval)
	v.SetDefault("agent.commandtimeout", DefaultCommandTimeout)
	v.SetDefault("agent.metadatainterval", DefaultMetadataInterval)

	// TLS defaults
	v.SetDefault("tls.enabled", false)
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	// Validate NATS mode
	switch c.NATS.Mode {
	case NATSModeEmbedded, NATSModeExternal, NATSModeLeaf:
		// valid
	default:
		return fmt.Errorf("invalid NATS mode: %s (must be embedded, external, or leaf)", c.NATS.Mode)
	}

	// Validate external mode has URL
	if c.NATS.Mode == NATSModeExternal && c.NATS.URL == "" {
		return fmt.Errorf("NATS URL is required when mode is external")
	}

	// Validate leaf mode has parent URLs
	if c.NATS.Mode == NATSModeLeaf && len(c.NATS.Embedded.LeafNodeURLs) == 0 {
		return fmt.Errorf("leaf node parent URLs required when mode is leaf")
	}

	// Validate storage backend
	switch c.Storage.Backend {
	case StorageBackendSQLite, StorageBackendPostgreSQL:
		// valid
	default:
		return fmt.Errorf("invalid storage backend: %s (must be sqlite or postgresql)", c.Storage.Backend)
	}

	// Validate PostgreSQL DSN
	if c.Storage.Backend == StorageBackendPostgreSQL && c.Storage.PostgreSQL.DSN == "" {
		return fmt.Errorf("PostgreSQL DSN is required when backend is postgresql")
	}

	// Validate ports
	if c.Server.GRPCPort <= 0 || c.Server.GRPCPort > 65535 {
		return fmt.Errorf("invalid gRPC port: %d", c.Server.GRPCPort)
	}
	if c.Server.HTTPPort <= 0 || c.Server.HTTPPort > 65535 {
		return fmt.Errorf("invalid HTTP port: %d", c.Server.HTTPPort)
	}

	return nil
}
