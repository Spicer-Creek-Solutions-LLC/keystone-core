package config

import (
	"fmt"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/netutil"
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
	Auth    AuthConfig
	Webhook WebhookConfig
	Policy  PolicyConfig
	Logging LoggingConfig
}

// LoggingConfig contains logging settings
type LoggingConfig struct {
	// Level: debug, info, warn, error (default: info)
	Level string
	// Format: json (default), logfmt, text
	Format string
	// Output: stdout (default), syslog
	// Note: file output is intentionally not supported - use journald or container log drivers
	Output string
	// IncludeCaller includes caller file:line in log entries
	IncludeCaller bool
	// IncludeStacktrace includes stack traces for error level logs
	IncludeStacktrace bool
	// Syslog configuration (when output: syslog)
	Syslog SyslogConfig
}

// SyslogConfig contains syslog output settings (RFC 5424)
type SyslogConfig struct {
	// Network: unix, udp, tcp, tcp+tls (default: unix)
	Network string
	// Address: /dev/log (unix) or host:port (udp/tcp)
	Address string
	// Facility: local0-local7, daemon, user (default: daemon)
	Facility string
	// AppName: application name in syslog messages (default: kscore-server/kscore-agent)
	AppName string
	// TLS configuration for tcp+tls
	TLS SyslogTLSConfig
}

// SyslogTLSConfig contains TLS settings for syslog
type SyslogTLSConfig struct {
	// Enabled enables TLS for syslog connections
	Enabled bool
	// CACert is the CA certificate file path
	CACert string
	// Cert is the client certificate file path
	Cert string
	// Key is the client key file path
	Key string
	// SkipVerify skips TLS verification (insecure)
	SkipVerify bool
}

// AuthConfig contains API authentication settings
type AuthConfig struct {
	// Enabled controls whether authentication is required (default: true for security)
	Enabled bool
	// Type of authentication: apikey, jwt, mtls, or multi (multiple methods)
	Type string
	// APIKey authentication settings
	APIKey APIKeyAuthConfig
	// JWT authentication settings
	JWT JWTAuthConfig
	// mTLS authentication settings (uses TLS config for certificates)
	MTLS MTLSAuthConfig
	// Methods allowed to bypass authentication (for health checks, etc.)
	// Format: "/package.Service/Method" e.g., "/kscore.v1.ControlPlaneService/HealthCheck"
	BypassMethods []string
}

// APIKeyAuthConfig contains API key authentication settings
type APIKeyAuthConfig struct {
	// Header name for API key (default: X-API-Key)
	HeaderName string
	// Metadata key for gRPC (default: x-api-key)
	MetadataKey string
	// Static API keys with their associated roles/permissions
	// Key: API key value, Value: key configuration
	Keys map[string]APIKeyConfig
}

// APIKeyConfig contains configuration for a single API key
type APIKeyConfig struct {
	// Human-readable name/description for the key
	Name string
	// Role assigned to this key: admin, operator, readonly
	Role string
	// Enabled flag to allow disabling without removing
	Enabled bool
	// Optional expiration time (RFC3339 format)
	ExpiresAt string
}

// JWTAuthConfig contains JWT authentication settings
type JWTAuthConfig struct {
	// Secret for HS256 signing (mutually exclusive with PublicKeyFile)
	Secret string
	// Public key file for RS256/ES256 verification
	PublicKeyFile string
	// Issuer to validate (optional)
	Issuer string
	// Audience to validate (optional)
	Audience string
	// Claim name containing the role (default: role)
	RoleClaim string
}

// MTLSAuthConfig contains mTLS authentication settings
type MTLSAuthConfig struct {
	// Require client certificates
	RequireClientCert bool
	// Map of certificate CN/SAN to role
	// Key: CN or SAN pattern, Value: role
	CertRoles map[string]string
}

// ServerConfig contains control plane server settings
type ServerConfig struct {
	// Listen address for API server (single address, backward compatible)
	// For IPv6, use "[::]:port" or "[::1]:port" format
	ListenAddr string
	// ListenAddrs for multi-address binding (dual-stack support)
	// Example: ["[::]:8080", "0.0.0.0:8080"] for dual-stack
	ListenAddrs []string
	// gRPC port
	GRPCPort int
	// HTTP/REST port
	HTTPPort int
	// AddressFamily preference for outbound connections
	// Options: prefer_ipv4, prefer_ipv6, ipv4_only, ipv6_only
	AddressFamily string
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
	// Host address for embedded NATS server (default: 127.0.0.1)
	// For IPv6, use "::1" (loopback) or "::" (all interfaces)
	Host string
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
	// AddressFamily preference: prefer_ipv4, prefer_ipv6, ipv4_only, ipv6_only
	AddressFamily string
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
	// AddressFamily preference for outbound connections
	// Options: prefer_ipv4, prefer_ipv6, ipv4_only, ipv6_only
	AddressFamily string
	// AdvertiseAddrs specifies addresses to advertise to control plane
	// If empty, addresses are auto-detected from network interfaces
	AdvertiseAddrs []string
	// Labels are key-value pairs for agent categorization and targeting
	Labels map[string]string
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

// WebhookConfig contains webhook receiver settings
type WebhookConfig struct {
	// Enable webhook receiver
	Enabled bool
	// Port for webhook HTTP server (separate from main HTTP port)
	Port int
	// Path prefix for webhook endpoints
	Path string
	// Authentication type: none, hmac, bearer
	AuthType string
	// Secret for HMAC authentication
	HMACSecret string
	// Token for Bearer authentication
	BearerToken string
	// Enabled webhook handlers: argocd, flux, github, gitlab
	Handlers []string
}

// PolicyConfig contains policy engine settings
type PolicyConfig struct {
	// Enable policy engine
	Enabled bool
	// Policy engine type: opa, cel, both
	Engine string
	// Enforcement mode: enforce, audit, warn
	EnforcementMode string
	// Built-in policies to enable
	Policies []PolicyDefinition
}

// PolicyDefinition defines a policy
type PolicyDefinition struct {
	// Unique policy ID
	ID string
	// Policy name
	Name string
	// Policy description
	Description string
	// Policy type: opa or cel
	Type string
	// Policy category: security, compliance, operational
	Category string
	// Severity: low, medium, high, critical
	Severity string
	// Policy code (Rego for OPA, CEL expression for CEL)
	Code string
	// Whether policy is enabled
	Enabled bool
}

// Default configuration values
const (
	DefaultServerListenAddr  = "0.0.0.0"
	DefaultServerListenAddr6 = "::"
	DefaultGRPCPort          = 9090
	DefaultHTTPPort          = 8080
	DefaultAddressFamily     = "prefer_ipv4" // prefer_ipv4, prefer_ipv6, ipv4_only, ipv6_only

	DefaultNATSMode          = NATSModeEmbedded
	DefaultNATSEmbeddedHost  = "127.0.0.1"
	DefaultNATSEmbeddedHost6 = "::1"
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

	DefaultWebhookPort = 8082
	DefaultWebhookPath = "/webhooks"

	DefaultPolicyEngine          = "both"
	DefaultPolicyEnforcementMode = "enforce"

	// Auth defaults - secure by default
	DefaultAuthEnabled       = true
	DefaultAuthType          = "apikey"
	DefaultAPIKeyHeaderName  = "X-API-Key"
	DefaultAPIKeyMetadataKey = "x-api-key"
	DefaultJWTRoleClaim      = "role"

	// Logging defaults - stdout first, no file output
	DefaultLoggingLevel         = "info"
	DefaultLoggingFormat        = "json"
	DefaultLoggingOutput        = "stdout"
	DefaultSyslogNetwork        = "unix"
	DefaultSyslogAddress        = "/dev/log"
	DefaultSyslogFacility       = "daemon"
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
	v.SetEnvPrefix("KSCORE")
	v.AutomaticEnv()

	// Explicit bindings for commonly overridden settings
	v.BindEnv("agent.id", "KSCORE_AGENT_ID")

	// Logging environment variable bindings (T1.4: Epic 15)
	v.BindEnv("logging.level", "KSCORE_LOG_LEVEL")
	v.BindEnv("logging.format", "KSCORE_LOG_FORMAT")
	v.BindEnv("logging.output", "KSCORE_LOG_OUTPUT")

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
	v.SetDefault("server.addressfamily", DefaultAddressFamily)

	// NATS defaults
	v.SetDefault("nats.mode", DefaultNATSMode)
	v.SetDefault("nats.embedded.host", DefaultNATSEmbeddedHost)
	v.SetDefault("nats.embedded.port", DefaultNATSEmbeddedPort)
	v.SetDefault("nats.embedded.addressfamily", DefaultAddressFamily)
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
	v.SetDefault("agent.addressfamily", DefaultAddressFamily)

	// TLS defaults
	v.SetDefault("tls.enabled", false)

	// Auth defaults - secure by default
	v.SetDefault("auth.enabled", DefaultAuthEnabled)
	v.SetDefault("auth.type", DefaultAuthType)
	v.SetDefault("auth.apikey.headername", DefaultAPIKeyHeaderName)
	v.SetDefault("auth.apikey.metadatakey", DefaultAPIKeyMetadataKey)
	v.SetDefault("auth.jwt.roleclaim", DefaultJWTRoleClaim)
	v.SetDefault("auth.mtls.requireclientcert", true)

	// Webhook defaults
	v.SetDefault("webhook.enabled", false)
	v.SetDefault("webhook.port", DefaultWebhookPort)
	v.SetDefault("webhook.path", DefaultWebhookPath)
	v.SetDefault("webhook.authtype", "none")
	v.SetDefault("webhook.handlers", []string{"argocd", "flux", "github", "gitlab"})

	// Policy defaults
	v.SetDefault("policy.enabled", false)
	v.SetDefault("policy.engine", DefaultPolicyEngine)
	v.SetDefault("policy.enforcementmode", DefaultPolicyEnforcementMode)

	// Logging defaults - stdout first, no file output (Epic 15)
	v.SetDefault("logging.level", DefaultLoggingLevel)
	v.SetDefault("logging.format", DefaultLoggingFormat)
	v.SetDefault("logging.output", DefaultLoggingOutput)
	v.SetDefault("logging.includecaller", false)
	v.SetDefault("logging.includestacktrace", true)
	v.SetDefault("logging.syslog.network", DefaultSyslogNetwork)
	v.SetDefault("logging.syslog.address", DefaultSyslogAddress)
	v.SetDefault("logging.syslog.facility", DefaultSyslogFacility)
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

	// Validate address family settings
	if err := validateAddressFamily(c.Server.AddressFamily, "server"); err != nil {
		return err
	}
	if err := validateAddressFamily(c.NATS.Embedded.AddressFamily, "nats.embedded"); err != nil {
		return err
	}
	if err := validateAddressFamily(c.Agent.AddressFamily, "agent"); err != nil {
		return err
	}

	// Validate listen addresses if specified (can be host:port format)
	for i, addr := range c.Server.ListenAddrs {
		if _, err := netutil.ParseAddress(addr); err != nil {
			return fmt.Errorf("invalid server.listenaddrs[%d] %q: %w", i, addr, err)
		}
	}

	// Validate agent advertise addresses if specified (can be just IP or host:port)
	for i, addr := range c.Agent.AdvertiseAddrs {
		if err := netutil.ValidateAddress(addr); err != nil {
			// Also try parsing as host:port
			if _, err2 := netutil.ParseAddress(addr); err2 != nil {
				return fmt.Errorf("invalid agent.advertiseaddrs[%d] %q: %w", i, addr, err)
			}
		}
	}

	// Validate webhook config
	if c.Webhook.Enabled {
		if c.Webhook.Port <= 0 || c.Webhook.Port > 65535 {
			return fmt.Errorf("invalid webhook port: %d", c.Webhook.Port)
		}
		switch c.Webhook.AuthType {
		case "", "none", "hmac", "bearer":
			// valid
		default:
			return fmt.Errorf("invalid webhook auth type: %s (must be none, hmac, or bearer)", c.Webhook.AuthType)
		}
		if c.Webhook.AuthType == "hmac" && c.Webhook.HMACSecret == "" {
			return fmt.Errorf("HMAC secret required when auth type is hmac")
		}
		if c.Webhook.AuthType == "bearer" && c.Webhook.BearerToken == "" {
			return fmt.Errorf("bearer token required when auth type is bearer")
		}
	}

	// Validate policy config
	if c.Policy.Enabled {
		switch c.Policy.Engine {
		case "opa", "cel", "both":
			// valid
		default:
			return fmt.Errorf("invalid policy engine: %s (must be opa, cel, or both)", c.Policy.Engine)
		}
		switch c.Policy.EnforcementMode {
		case "enforce", "audit", "warn":
			// valid
		default:
			return fmt.Errorf("invalid enforcement mode: %s (must be enforce, audit, or warn)", c.Policy.EnforcementMode)
		}
	}

	// Validate auth config
	if c.Auth.Enabled {
		switch c.Auth.Type {
		case "apikey", "jwt", "mtls", "multi":
			// valid
		default:
			return fmt.Errorf("invalid auth type: %s (must be apikey, jwt, mtls, or multi)", c.Auth.Type)
		}

		// Validate API key config when using apikey or multi auth
		if c.Auth.Type == "apikey" || c.Auth.Type == "multi" {
			if len(c.Auth.APIKey.Keys) == 0 {
				return fmt.Errorf("at least one API key must be configured when auth type is %s", c.Auth.Type)
			}
			for key, cfg := range c.Auth.APIKey.Keys {
				if key == "" {
					return fmt.Errorf("API key value cannot be empty")
				}
				if len(key) < 32 {
					return fmt.Errorf("API key %q is too short (minimum 32 characters for security)", cfg.Name)
				}
				switch cfg.Role {
				case "admin", "operator", "readonly":
					// valid
				default:
					return fmt.Errorf("invalid role %q for API key %q (must be admin, operator, or readonly)", cfg.Role, cfg.Name)
				}
			}
		}

		// Validate JWT config when using jwt or multi auth
		if c.Auth.Type == "jwt" || c.Auth.Type == "multi" {
			if c.Auth.JWT.Secret == "" && c.Auth.JWT.PublicKeyFile == "" {
				return fmt.Errorf("JWT secret or public key file must be configured when auth type is %s", c.Auth.Type)
			}
			if c.Auth.JWT.Secret != "" && c.Auth.JWT.PublicKeyFile != "" {
				return fmt.Errorf("JWT secret and public key file are mutually exclusive")
			}
		}

		// Validate mTLS config when using mtls or multi auth
		if c.Auth.Type == "mtls" || c.Auth.Type == "multi" {
			if !c.TLS.Enabled {
				return fmt.Errorf("TLS must be enabled when using mTLS authentication")
			}
			if c.TLS.CAFile == "" {
				return fmt.Errorf("CA file must be configured for mTLS authentication")
			}
		}
	}

	// Validate logging config (Epic 15)
	switch c.Logging.Level {
	case "", "debug", "info", "warn", "error":
		// valid (empty uses default)
	default:
		return fmt.Errorf("invalid log level: %s (must be debug, info, warn, or error)", c.Logging.Level)
	}
	switch c.Logging.Format {
	case "", "json", "logfmt", "text":
		// valid (empty uses default)
	default:
		return fmt.Errorf("invalid log format: %s (must be json, logfmt, or text)", c.Logging.Format)
	}
	switch c.Logging.Output {
	case "", "stdout", "syslog":
		// valid (empty uses default)
	default:
		return fmt.Errorf("invalid log output: %s (must be stdout or syslog)", c.Logging.Output)
	}
	// Validate syslog config when output is syslog
	if c.Logging.Output == "syslog" {
		switch c.Logging.Syslog.Network {
		case "", "unix", "udp", "tcp", "tcp+tls":
			// valid
		default:
			return fmt.Errorf("invalid syslog network: %s (must be unix, udp, tcp, or tcp+tls)", c.Logging.Syslog.Network)
		}
		switch c.Logging.Syslog.Facility {
		case "", "daemon", "user", "local0", "local1", "local2", "local3", "local4", "local5", "local6", "local7":
			// valid
		default:
			return fmt.Errorf("invalid syslog facility: %s", c.Logging.Syslog.Facility)
		}
	}

	return nil
}

// validateAddressFamily validates an address family string.
func validateAddressFamily(af, field string) error {
	if af == "" {
		return nil // empty uses default
	}
	switch af {
	case "prefer_ipv4", "prefer_ipv6", "ipv4_only", "ipv6_only":
		return nil
	default:
		return fmt.Errorf("invalid %s.addressfamily: %s (must be prefer_ipv4, prefer_ipv6, ipv4_only, or ipv6_only)", field, af)
	}
}

// GetAddressFamilyPreference returns the AddressFamilyPreference for the server config.
func (c *ServerConfig) GetAddressFamilyPreference() netutil.AddressFamilyPreference {
	return parseAddressFamilyPreference(c.AddressFamily)
}

// GetEffectiveListenAddrs returns the effective listen addresses.
// If ListenAddrs is specified, returns those. Otherwise, returns ListenAddr.
// For dual-stack mode, returns both IPv4 and IPv6 addresses.
func (c *ServerConfig) GetEffectiveListenAddrs() []string {
	if len(c.ListenAddrs) > 0 {
		return c.ListenAddrs
	}
	if c.ListenAddr != "" {
		return []string{c.ListenAddr}
	}
	// Return default based on address family preference
	switch c.AddressFamily {
	case "ipv6_only":
		return []string{DefaultServerListenAddr6}
	case "prefer_ipv6":
		return []string{DefaultServerListenAddr6, DefaultServerListenAddr}
	default:
		return []string{DefaultServerListenAddr}
	}
}

// GetAddressFamilyPreference returns the AddressFamilyPreference for the agent config.
func (c *AgentConfig) GetAddressFamilyPreference() netutil.AddressFamilyPreference {
	return parseAddressFamilyPreference(c.AddressFamily)
}

// GetAddressFamilyPreference returns the AddressFamilyPreference for the embedded NATS config.
func (c *NATSEmbeddedConfig) GetAddressFamilyPreference() netutil.AddressFamilyPreference {
	return parseAddressFamilyPreference(c.AddressFamily)
}

// GetEffectiveHost returns the effective host for embedded NATS.
// For IPv6-only or prefer-IPv6 modes, returns IPv6 loopback.
func (c *NATSEmbeddedConfig) GetEffectiveHost() string {
	if c.Host != "" {
		return c.Host
	}
	switch c.AddressFamily {
	case "ipv6_only", "prefer_ipv6":
		return DefaultNATSEmbeddedHost6
	default:
		return DefaultNATSEmbeddedHost
	}
}

// parseAddressFamilyPreference converts a string to AddressFamilyPreference.
func parseAddressFamilyPreference(af string) netutil.AddressFamilyPreference {
	switch af {
	case "prefer_ipv6":
		return netutil.PreferIPv6
	case "ipv4_only":
		return netutil.IPv4Only
	case "ipv6_only":
		return netutil.IPv6Only
	default:
		return netutil.PreferIPv4
	}
}
