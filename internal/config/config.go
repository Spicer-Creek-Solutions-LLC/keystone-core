// Package config handles configuration loading and validation for Keystone Core.
// It provides:
//   - Configuration structures for all components (server, agent, NATS, storage)
//   - YAML configuration file parsing via Viper
//   - Environment variable support for containerized deployments
//   - Validation of configuration values
//   - Default values for zero-configuration startup
//
// Configuration can be provided via files, environment variables, or command-line
// flags, with proper precedence handling and validation.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/shawnbutts/keystone-core/internal/netutil"
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
	Server          ServerConfig
	NATS            NATSConfig
	Storage         StorageConfig
	Agent           AgentConfig
	AgentManagement AgentManagementConfig
	Execution       ExecutionConfig
	StateManagement StateManagementConfig
	Events          EventsConfig
	GitOps          GitOpsConfig
	TLS             TLSConfig
	Auth            AuthConfig
	Webhook         WebhookConfig
	Policy          PolicyConfig
	Logging         LoggingConfig
	CORS            CORSConfig
	RateLimit       RateLimitConfig
	Metrics         MetricsConfig
	Tracing         TracingConfig
	Health          HealthConfig
	Profiling       ProfilingConfig
	Operator        KubernetesOperatorConfig
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
	// MinVersion is the minimum TLS version (1.2 or 1.3)
	MinVersion string
}

// CORSConfig contains CORS (Cross-Origin Resource Sharing) settings
type CORSConfig struct {
	// Enabled controls whether CORS is enabled (default: true)
	Enabled bool
	// AllowedOrigins lists the origins allowed to access the API
	// Use ["*"] to allow all origins (not recommended for production)
	AllowedOrigins []string
	// AllowedMethods lists the HTTP methods allowed (default: GET, POST, PUT, DELETE, OPTIONS)
	AllowedMethods []string
	// AllowedHeaders lists the headers allowed in requests
	AllowedHeaders []string
	// AllowCredentials indicates whether credentials are allowed
	AllowCredentials bool
	// MaxAge is the max age (in seconds) for preflight cache
	MaxAge int
}

// RateLimitConfig contains API rate limiting settings
type RateLimitConfig struct {
	// Enabled controls whether rate limiting is enabled (default: true)
	Enabled bool
	// RequestsPerMinute is the maximum requests per minute per key
	RequestsPerMinute int
	// Burst is the burst capacity allowing temporary spikes
	Burst int
	// KeyExtractor determines how to identify clients: ip, apikey, header
	KeyExtractor string
	// HeaderName is the header to use when KeyExtractor is "header"
	HeaderName string
}

// MetricsConfig contains metrics endpoint settings
type MetricsConfig struct {
	// Enabled controls whether metrics endpoint is enabled (default: true)
	Enabled bool
	// Listen address for metrics endpoint (e.g., ":8080" or "0.0.0.0:9100")
	// If empty, metrics are served on the main API server
	Listen string
	// Path for metrics endpoint (default: /metrics)
	Path string
	// IncludeGoMetrics includes Go runtime metrics
	IncludeGoMetrics bool
	// IncludeProcessMetrics includes process metrics (open FDs, etc.)
	IncludeProcessMetrics bool
}

// TracingConfig contains distributed tracing settings
type TracingConfig struct {
	// Enabled controls whether tracing is enabled (default: false)
	Enabled bool
	// Exporter type: otlp, jaeger, zipkin
	Exporter string
	// Endpoint for trace exporter (e.g., "http://jaeger:4318")
	Endpoint string
	// Sampling configuration
	Sampling TracingSamplingConfig
	// Resource attributes for traces
	Resource map[string]string
}

// TracingSamplingConfig contains trace sampling settings
type TracingSamplingConfig struct {
	// Strategy: always, never, ratio, parent_based
	Strategy string
	// Ratio is the sampling ratio (0.0-1.0) when Strategy is "ratio"
	Ratio float64
}

// ProfilingConfig contains pprof profiling settings
type ProfilingConfig struct {
	// Enabled controls whether profiling endpoints are enabled (default: false)
	Enabled bool
	// Listen address for profiling server (default: "localhost:6060")
	Listen string
	// BlockProfileRate controls the fraction of goroutine blocking events reported (0 = disabled)
	BlockProfileRate int
	// MutexProfileFraction controls the fraction of mutex contention events reported (0 = disabled)
	MutexProfileFraction int
}

// HealthConfig contains health check settings
type HealthConfig struct {
	// Enabled controls whether health checks are enabled (default: true)
	Enabled bool
	// StartupGracePeriod is the grace period after startup during which
	// the service reports as healthy even if checks fail
	StartupGracePeriod time.Duration
	// CheckInterval is the interval between health checks
	CheckInterval time.Duration
	// Checks configures individual health checks
	Checks HealthChecksConfig
}

// HealthChecksConfig contains configuration for individual health checks
type HealthChecksConfig struct {
	// NATS health check
	NATS HealthCheckConfig
	// Database health check
	Database HealthCheckConfig
	// Agents health check
	Agents AgentHealthCheckConfig
}

// HealthCheckConfig contains settings for a single health check
type HealthCheckConfig struct {
	// Enabled controls whether this check is enabled
	Enabled bool
	// Timeout for the health check
	Timeout time.Duration
}

// AgentHealthCheckConfig contains settings for agent health check
type AgentHealthCheckConfig struct {
	// Enabled controls whether this check is enabled
	Enabled bool
	// Timeout for the health check
	Timeout time.Duration
	// MinHealthy is the minimum fraction of agents that must be healthy (0.0-1.0)
	MinHealthy float64
}

// AuthConfig contains API authentication and authorization settings
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
	// Authorization contains RBAC authorization settings
	Authorization AuthorizationConfig
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
	// HTTPListen in "host:port" format for HTTP/REST API (e.g., "0.0.0.0:8080")
	// This is the documented api.listen format. If set, overrides ListenAddr and HTTPPort.
	HTTPListen string
	// GRPCListen in "host:port" format for gRPC API (e.g., "0.0.0.0:9090")
	// This is the documented api.grpc_listen format. If set, overrides ListenAddr and GRPCPort.
	GRPCListen string
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
	// AllowInsecureNonLoopback explicitly allows listening on non-loopback
	// addresses without TLS enabled. This is a security risk and should only
	// be used in development/testing environments or when TLS termination
	// is handled by a reverse proxy.
	// WARNING: Setting this to true exposes the control plane to network attacks.
	AllowInsecureNonLoopback bool
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
	// Listen address in "host:port" format (e.g., "0.0.0.0:4222")
	// This is an alternative to setting Host and Port separately.
	// If Listen is set, it takes precedence over Host and Port.
	Listen string
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

// Validate validates the NATS configuration with detailed error messages.
// Returns nil if the configuration is valid, or an error with actionable guidance.
func (c *NATSConfig) Validate() error {
	// Validate NATS mode
	switch c.Mode {
	case NATSModeEmbedded, NATSModeExternal, NATSModeLeaf:
		// valid modes
	case "":
		return &NATSConfigError{
			Field:   "mode",
			Message: "NATS mode is required",
			Hint:    "Set to 'embedded' for single-node deployments, 'external' to connect to existing NATS cluster, or 'leaf' for edge deployments",
		}
	default:
		return &NATSConfigError{
			Field:   "mode",
			Message: fmt.Sprintf("invalid NATS mode: %q", c.Mode),
			Hint:    "Valid modes are: 'embedded', 'external', or 'leaf'",
		}
	}

	// Validate based on mode
	switch c.Mode {
	case NATSModeExternal:
		if err := c.validateExternalMode(); err != nil {
			return err
		}
	case NATSModeLeaf:
		if err := c.validateLeafMode(); err != nil {
			return err
		}
	case NATSModeEmbedded:
		if err := c.validateEmbeddedMode(); err != nil {
			return err
		}
	}

	// Validate JetStream configuration
	if err := c.JetStream.Validate(); err != nil {
		return err
	}

	// Validate connection settings
	if c.MaxReconnects < -1 {
		return &NATSConfigError{
			Field:   "maxreconnects",
			Message: fmt.Sprintf("invalid max reconnects value: %d", c.MaxReconnects),
			Hint:    "Use -1 for unlimited reconnects, 0 to disable reconnection, or a positive number for a specific limit",
		}
	}

	if c.ReconnectWait < 0 {
		return &NATSConfigError{
			Field:   "reconnectwait",
			Message: "reconnect wait cannot be negative",
			Hint:    "Use a positive duration like '2s' or '5s' for reconnection delay",
		}
	}

	// Validate authentication consistency
	if c.Token != "" && c.Credential != "" {
		return &NATSConfigError{
			Field:   "authentication",
			Message: "both token and credential file are specified",
			Hint:    "Use either 'token' for simple authentication or 'credential' for NKey/JWT authentication, not both",
		}
	}

	return nil
}

// validateExternalMode validates configuration for external NATS mode.
func (c *NATSConfig) validateExternalMode() error {
	if c.URL == "" {
		return &NATSConfigError{
			Field:   "url",
			Message: "NATS URL is required for external mode",
			Hint:    "Provide the URL(s) of your NATS cluster, e.g., 'nats://nats.example.com:4222' or comma-separated for multiple servers",
		}
	}

	// Validate URL format
	if err := validateNATSURL(c.URL); err != nil {
		return &NATSConfigError{
			Field:   "url",
			Message: fmt.Sprintf("invalid NATS URL: %s", err),
			Hint:    "Use format: nats://host:port, tls://host:port, or ws(s)://host:port",
		}
	}

	return nil
}

// validateLeafMode validates configuration for leaf node mode.
func (c *NATSConfig) validateLeafMode() error {
	if len(c.Embedded.LeafNodeURLs) == 0 {
		return &NATSConfigError{
			Field:   "embedded.leafnodeurls",
			Message: "leaf node parent URLs are required for leaf mode",
			Hint:    "Provide the URL(s) of the parent NATS cluster leaf node endpoints, e.g., 'nats-leaf://hub.example.com:7422'",
		}
	}

	// Validate each leaf node URL
	for i, leafURL := range c.Embedded.LeafNodeURLs {
		if err := validateNATSLeafURL(leafURL); err != nil {
			return &NATSConfigError{
				Field:   fmt.Sprintf("embedded.leafnodeurls[%d]", i),
				Message: fmt.Sprintf("invalid leaf node URL %q: %s", leafURL, err),
				Hint:    "Use format: nats-leaf://host:port or tls://host:port for secure connections",
			}
		}
	}

	// Also validate embedded settings since leaf mode uses embedded server
	return c.validateEmbeddedSettings()
}

// validateEmbeddedMode validates configuration for embedded NATS mode.
func (c *NATSConfig) validateEmbeddedMode() error {
	return c.validateEmbeddedSettings()
}

// validateEmbeddedSettings validates embedded NATS server settings.
func (c *NATSConfig) validateEmbeddedSettings() error {
	// Validate port (0 means use default, which is valid)
	if c.Embedded.Port < 0 || c.Embedded.Port > 65535 {
		return &NATSConfigError{
			Field:   "embedded.port",
			Message: fmt.Sprintf("invalid embedded NATS port: %d", c.Embedded.Port),
			Hint:    "Port must be between 0 and 65535. Use 0 for default (4222)",
		}
	}

	// Note: Ports < 1024 are privileged and require root. We don't error here
	// as the port is still valid, but the user should be aware of this.

	// Validate max connections
	if c.Embedded.MaxConnections < 0 {
		return &NATSConfigError{
			Field:   "embedded.maxconnections",
			Message: "max connections cannot be negative",
			Hint:    "Use 0 for unlimited connections or a positive number to limit concurrent connections",
		}
	}

	// Validate max memory
	if c.Embedded.MaxMemory < 0 {
		return &NATSConfigError{
			Field:   "embedded.maxmemory",
			Message: "max memory cannot be negative",
			Hint:    "Use 0 for automatic sizing based on available system memory, or specify bytes (e.g., 1073741824 for 1GB)",
		}
	}

	// Validate JetStream store directory if JetStream is enabled
	if c.Embedded.EnableJetStream && c.Embedded.StoreDir != "" {
		// Check if parent directory exists and is writable
		parentDir := filepath.Dir(c.Embedded.StoreDir)
		if _, err := os.Stat(parentDir); os.IsNotExist(err) {
			return &NATSConfigError{
				Field:   "embedded.storedir",
				Message: fmt.Sprintf("JetStream store parent directory does not exist: %s", parentDir),
				Hint:    "Create the parent directory or specify a different path. The store directory will be created automatically.",
			}
		}
	}

	// Validate address family
	if c.Embedded.AddressFamily != "" {
		switch c.Embedded.AddressFamily {
		case "prefer_ipv4", "prefer_ipv6", "ipv4_only", "ipv6_only":
			// valid
		default:
			return &NATSConfigError{
				Field:   "embedded.addressfamily",
				Message: fmt.Sprintf("invalid address family: %q", c.Embedded.AddressFamily),
				Hint:    "Valid options: 'prefer_ipv4' (default), 'prefer_ipv6', 'ipv4_only', 'ipv6_only'",
			}
		}
	}

	return nil
}

// Validate validates the JetStream configuration.
func (c *JetStreamConfig) Validate() error {
	if !c.Enabled {
		return nil
	}

	if c.MaxStorage < 0 {
		return &NATSConfigError{
			Field:   "jetstream.maxstorage",
			Message: "JetStream max storage cannot be negative",
			Hint:    "Use 0 for automatic sizing or specify bytes (e.g., 10737418240 for 10GB)",
		}
	}

	// If store directory is specified, validate it
	if c.StoreDir != "" {
		parentDir := filepath.Dir(c.StoreDir)
		if _, err := os.Stat(parentDir); os.IsNotExist(err) {
			return &NATSConfigError{
				Field:   "jetstream.storedir",
				Message: fmt.Sprintf("JetStream store parent directory does not exist: %s", parentDir),
				Hint:    "Create the parent directory or specify a different path",
			}
		}
	}

	return nil
}

// NATSConfigError provides detailed error information for NATS configuration issues.
type NATSConfigError struct {
	Field   string // Configuration field that has the issue
	Message string // Description of the error
	Hint    string // Actionable guidance on how to fix the issue
}

// Error implements the error interface.
func (e *NATSConfigError) Error() string {
	if e.Hint != "" {
		return fmt.Sprintf("NATS configuration error in %s: %s. Hint: %s", e.Field, e.Message, e.Hint)
	}
	return fmt.Sprintf("NATS configuration error in %s: %s", e.Field, e.Message)
}

// validateNATSURL validates a NATS connection URL.
func validateNATSURL(urlStr string) error {
	// Handle comma-separated URLs
	urls := strings.Split(urlStr, ",")
	for _, u := range urls {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		if err := validateSingleNATSURL(u); err != nil {
			return err
		}
	}
	return nil
}

// validateSingleNATSURL validates a single NATS URL.
func validateSingleNATSURL(urlStr string) error {
	parsed, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("malformed URL: %w", err)
	}

	// Check scheme
	switch parsed.Scheme {
	case "nats", "tls", "ws", "wss":
		// Valid NATS schemes
	case "":
		return fmt.Errorf("URL scheme is required (nats://, tls://, or ws(s)://)")
	default:
		return fmt.Errorf("unsupported URL scheme %q (use nats://, tls://, or ws(s)://)", parsed.Scheme)
	}

	// Check host
	if parsed.Host == "" {
		return fmt.Errorf("URL host is required")
	}

	return nil
}

// validateNATSLeafURL validates a NATS leaf node URL.
func validateNATSLeafURL(urlStr string) error {
	parsed, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("malformed URL: %w", err)
	}

	// Check scheme - leaf nodes typically use nats-leaf or tls
	switch parsed.Scheme {
	case "nats", "nats-leaf", "tls", "ws", "wss":
		// Valid leaf node schemes
	case "":
		return fmt.Errorf("URL scheme is required (nats-leaf://, nats://, tls://, or ws(s)://)")
	default:
		return fmt.Errorf("unsupported URL scheme %q (use nats-leaf://, nats://, tls://, or ws(s)://)", parsed.Scheme)
	}

	// Check host
	if parsed.Host == "" {
		return fmt.Errorf("URL host is required")
	}

	return nil
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

// AgentManagementConfig contains server-side agent management settings.
// These control how the control plane monitors and manages connected agents.
type AgentManagementConfig struct {
	// HeartbeatInterval is the interval agents should send heartbeats.
	// This value is sent to agents during registration.
	HeartbeatInterval time.Duration
	// HeartbeatTimeout is how long to wait before considering an agent stale.
	HeartbeatTimeout time.Duration
	// StaleThreshold is how many missed heartbeats before marking an agent offline.
	StaleThreshold int
	// MonitorInterval is how often the control plane checks agent health.
	MonitorInterval time.Duration
	// MetadataRefresh is the interval agents should refresh system metadata.
	// This value is sent to agents during registration.
	MetadataRefresh time.Duration
	// MaxConcurrentCommands is the maximum number of concurrent commands per agent.
	// 0 means unlimited.
	MaxConcurrentCommands int
}

// Validate checks the agent management configuration.
func (c *AgentManagementConfig) Validate() error {
	if c.HeartbeatInterval < 0 {
		return fmt.Errorf("agentmanagement.heartbeatinterval cannot be negative")
	}
	if c.HeartbeatTimeout < 0 {
		return fmt.Errorf("agentmanagement.heartbeattimeout cannot be negative")
	}
	if c.HeartbeatTimeout > 0 && c.HeartbeatInterval > 0 && c.HeartbeatTimeout < c.HeartbeatInterval {
		return fmt.Errorf("agentmanagement.heartbeattimeout (%v) must be >= heartbeatinterval (%v)", c.HeartbeatTimeout, c.HeartbeatInterval)
	}
	if c.StaleThreshold < 0 {
		return fmt.Errorf("agentmanagement.stalethreshold cannot be negative")
	}
	if c.MonitorInterval < 0 {
		return fmt.Errorf("agentmanagement.monitorinterval cannot be negative")
	}
	if c.MetadataRefresh < 0 {
		return fmt.Errorf("agentmanagement.metadatarefresh cannot be negative")
	}
	if c.MaxConcurrentCommands < 0 {
		return fmt.Errorf("agentmanagement.maxconcurrentcommands cannot be negative")
	}
	return nil
}

// ExecutionConfig contains command execution settings.
type ExecutionConfig struct {
	// DefaultTimeout is the default timeout for command execution.
	DefaultTimeout time.Duration
	// MaxTimeout is the maximum allowed timeout for command execution.
	// Requests with timeout exceeding this value will be clamped.
	MaxTimeout time.Duration
	// BatchSize is the default number of agents to execute on concurrently in batch operations.
	BatchSize int
	// BatchDelay is the delay between starting execution on each batch group.
	BatchDelay time.Duration
	// StreamingBuffer is the buffer size for streaming command responses.
	StreamingBuffer int
	// ResultRetention is how long to retain command execution results.
	ResultRetention time.Duration
}

// Validate checks the execution configuration.
func (c *ExecutionConfig) Validate() error {
	if c.DefaultTimeout < 0 {
		return fmt.Errorf("execution.defaulttimeout cannot be negative")
	}
	if c.MaxTimeout < 0 {
		return fmt.Errorf("execution.maxtimeout cannot be negative")
	}
	if c.MaxTimeout > 0 && c.DefaultTimeout > c.MaxTimeout {
		return fmt.Errorf("execution.defaulttimeout (%v) cannot exceed maxtimeout (%v)", c.DefaultTimeout, c.MaxTimeout)
	}
	if c.BatchSize < 0 {
		return fmt.Errorf("execution.batchsize cannot be negative")
	}
	if c.BatchDelay < 0 {
		return fmt.Errorf("execution.batchdelay cannot be negative")
	}
	if c.StreamingBuffer < 0 {
		return fmt.Errorf("execution.streamingbuffer cannot be negative")
	}
	if c.ResultRetention < 0 {
		return fmt.Errorf("execution.resultretention cannot be negative")
	}
	return nil
}

// StateManagementConfig contains state management settings.
type StateManagementConfig struct {
	// DefaultTimeout is the default timeout for state apply operations.
	DefaultTimeout time.Duration
	// MaxConcurrent is the maximum number of concurrent state apply operations.
	MaxConcurrent int
	// DriftCheckInterval is how often to check for configuration drift.
	// 0 disables automatic drift checking.
	DriftCheckInterval time.Duration
	// ResultRetention is how long to retain state apply results.
	ResultRetention time.Duration
}

// Validate checks the state management configuration.
func (c *StateManagementConfig) Validate() error {
	if c.DefaultTimeout < 0 {
		return fmt.Errorf("statemanagement.defaulttimeout cannot be negative")
	}
	if c.MaxConcurrent < 0 {
		return fmt.Errorf("statemanagement.maxconcurrent cannot be negative")
	}
	if c.DriftCheckInterval < 0 {
		return fmt.Errorf("statemanagement.driftcheckinterval cannot be negative")
	}
	if c.ResultRetention < 0 {
		return fmt.Errorf("statemanagement.resultretention cannot be negative")
	}
	return nil
}

// EventsConfig contains event system settings.
type EventsConfig struct {
	// Enabled controls whether the event system is active (default: true).
	Enabled bool
	// Retention is the maximum age of stored events before they are discarded.
	Retention time.Duration
	// MaxBytes is the maximum total size of stored events in bytes.
	// 0 means use JetStream default.
	MaxBytes int64
	// MaxMessages is the maximum number of stored events.
	// 0 means use JetStream default.
	MaxMessages int64
	// PublisherBufferSize is the buffer size for the event publisher channel.
	PublisherBufferSize int
	// SubscriberBufferSize is the buffer size for event subscriber channels.
	SubscriberBufferSize int
	// SubscriberAckWait is how long to wait for a subscriber to acknowledge an event.
	SubscriberAckWait time.Duration
}

// Validate checks the events configuration.
func (c *EventsConfig) Validate() error {
	if c.Retention < 0 {
		return fmt.Errorf("events.retention cannot be negative")
	}
	if c.MaxBytes < 0 {
		return fmt.Errorf("events.maxbytes cannot be negative")
	}
	if c.MaxMessages < 0 {
		return fmt.Errorf("events.maxmessages cannot be negative")
	}
	if c.PublisherBufferSize < 0 {
		return fmt.Errorf("events.publisherbuffersize cannot be negative")
	}
	if c.SubscriberBufferSize < 0 {
		return fmt.Errorf("events.subscriberbuffersize cannot be negative")
	}
	if c.SubscriberAckWait < 0 {
		return fmt.Errorf("events.subscriberrackwait cannot be negative")
	}
	return nil
}

// GitOpsConfig contains GitOps integration settings.
type GitOpsConfig struct {
	// GitSync configures automatic git repository synchronization.
	GitSync GitSyncConfig
}

// GitSyncConfig contains git repository sync settings.
type GitSyncConfig struct {
	// Enabled controls whether git-sync is active.
	Enabled bool
	// Repositories lists git repositories to synchronize.
	Repositories []GitSyncRepository
}

// GitSyncRepository defines a git repository to synchronize.
type GitSyncRepository struct {
	// URL is the git repository URL.
	URL string
	// Branch is the branch to track (default: main).
	Branch string
	// Interval is how often to poll for changes.
	Interval time.Duration
	// Path is the subdirectory within the repo to sync (empty = root).
	Path string
}

// Validate checks the GitOps configuration.
func (c *GitOpsConfig) Validate() error {
	if c.GitSync.Enabled {
		for i, repo := range c.GitSync.Repositories {
			if repo.URL == "" {
				return fmt.Errorf("gitops.gitsync.repositories[%d].url is required", i)
			}
			if repo.Interval < 0 {
				return fmt.Errorf("gitops.gitsync.repositories[%d].interval cannot be negative", i)
			}
		}
	}
	return nil
}

// AuthorizationConfig contains RBAC authorization settings.
type AuthorizationConfig struct {
	// Enabled controls whether authorization (RBAC) checks are performed.
	// When false, all authenticated requests are allowed regardless of role.
	Enabled bool
	// DefaultDeny controls behavior for unmapped methods.
	// When true, requests to methods without explicit role mappings are denied.
	// When false, requests to unmapped methods are allowed for any authenticated user.
	DefaultDeny bool
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
	// MinVersion is the minimum TLS version (1.2 or 1.3)
	MinVersion string
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

// KubernetesOperatorConfig configures the Kubernetes operator mode.
type KubernetesOperatorConfig struct {
	// Enabled enables the Kubernetes operator (watches CRDs, runs reconciliation)
	Enabled bool
	// Namespace restricts the operator to a single namespace (empty = all namespaces)
	Namespace string
	// LeaderElection enables leader election for HA deployments
	LeaderElection bool
	// LeaderElectionID is the lease name for leader election
	LeaderElectionID string
	// ReconcileInterval is how often periodic reconciliation runs
	ReconcileInterval time.Duration
	// MaxConcurrentReconciles limits parallel reconciliations
	MaxConcurrentReconciles int
}

// Validate checks the Kubernetes operator configuration.
func (c KubernetesOperatorConfig) Validate() error {
	if !c.Enabled {
		return nil
	}
	if c.ReconcileInterval < 10*time.Second {
		return fmt.Errorf("operator reconcile interval must be >= 10s, got %s", c.ReconcileInterval)
	}
	if c.MaxConcurrentReconciles < 1 {
		return fmt.Errorf("operator max concurrent reconciles must be >= 1, got %d", c.MaxConcurrentReconciles)
	}
	if c.LeaderElection && c.LeaderElectionID == "" {
		return fmt.Errorf("operator leader election ID is required when leader election is enabled")
	}
	return nil
}

// Default configuration values
const (
	// DefaultServerListenAddr defaults to loopback for security (requires TLS for non-loopback)
	DefaultServerListenAddr  = "127.0.0.1"
	DefaultServerListenAddr6 = "::1"
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

	// Agent management defaults (server-side control plane settings)
	DefaultAgentMgmtHeartbeatTimeout  = 60 * time.Second
	DefaultAgentMgmtStaleThreshold    = 3
	DefaultAgentMgmtMonitorInterval   = 10 * time.Second
	DefaultAgentMgmtMaxConcurrentCmds = 0 // unlimited

	// Execution defaults
	DefaultExecDefaultTimeout  = 5 * time.Minute
	DefaultExecMaxTimeout      = 1 * time.Hour
	DefaultExecBatchSize       = 10
	DefaultExecStreamingBuffer = 100
	DefaultExecResultRetention = 24 * time.Hour

	// State management defaults
	DefaultStateMgmtDefaultTimeout  = 10 * time.Minute
	DefaultStateMgmtMaxConcurrent   = 5
	DefaultStateMgmtResultRetention = 7 * 24 * time.Hour // 7 days

	// Event system defaults
	DefaultEventsEnabled           = true
	DefaultEventsRetention         = 7 * 24 * time.Hour   // 7 days
	DefaultEventsMaxBytes    int64 = 10 * 1024 * 1024 * 1024 // 10GB
	DefaultEventsMaxMessages int64 = 1000000                  // 1M messages
	DefaultEventsPublisherBufferSize  = 256
	DefaultEventsSubscriberBufferSize = 256
	DefaultEventsSubscriberAckWait    = 30 * time.Second

	// GitSync defaults
	DefaultGitSyncBranch   = "main"
	DefaultGitSyncInterval = 5 * time.Minute

	DefaultWebhookPort = 8082
	DefaultWebhookPath = "/webhooks"

	DefaultPolicyEngine          = "both"
	DefaultPolicyEnforcementMode = "enforce"

	// Auth defaults - secure by default
	DefaultAuthEnabled       = true
	DefaultAuthType          = "apikey"
	DefaultAuthzEnabled      = true
	DefaultAuthzDefaultDeny  = true
	//nolint:gosec // G101: false positive - this is a header name, not a hardcoded secret
	DefaultAPIKeyHeaderName = "X-API-Key"
	DefaultAPIKeyMetadataKey = "x-api-key"
	DefaultJWTRoleClaim      = "role"

	// Logging defaults - stdout first, no file output
	DefaultLoggingLevel   = "info"
	DefaultLoggingFormat  = "json"
	DefaultLoggingOutput  = "stdout"
	DefaultSyslogNetwork  = "unix"
	DefaultSyslogAddress  = "/dev/log"
	DefaultSyslogFacility = "daemon"

	// CORS defaults
	DefaultCORSEnabled = true

	// Rate limit defaults
	DefaultRateLimitEnabled        = true
	DefaultRateLimitRequestsPerMin = 100
	DefaultRateLimitBurst          = 20
	DefaultRateLimitKeyExtractor   = "ip"

	// Metrics defaults
	DefaultMetricsEnabled        = true
	DefaultMetricsPath           = "/metrics"
	DefaultMetricsIncludeGo      = true
	DefaultMetricsIncludeProcess = true

	// Tracing defaults
	DefaultTracingEnabled          = false
	DefaultTracingExporter         = "otlp"
	DefaultTracingSamplingStrategy = "ratio"
	DefaultTracingSamplingRatio    = 0.1

	// Profiling defaults
	DefaultProfilingEnabled          = false
	DefaultProfilingListen           = "localhost:6060"
	DefaultProfilingBlockProfileRate = 0
	DefaultProfilingMutexFraction    = 0

	// Kubernetes operator defaults
	DefaultOperatorEnabled             = false
	DefaultOperatorLeaderElection      = true
	DefaultOperatorLeaderElectionID    = "kscore-operator"
	DefaultOperatorReconcileInterval   = 1 * time.Minute
	DefaultOperatorMaxConcurrentReconciles = 3

	// Health check defaults
	DefaultHealthEnabled            = true
	DefaultHealthStartupGracePeriod = 30 * time.Second
	DefaultHealthCheckInterval      = 10 * time.Second
	DefaultHealthCheckTimeout       = 5 * time.Second
	DefaultHealthAgentMinHealthy    = 0.8
)

// Production scaling thresholds - beyond these values, users should migrate
// from embedded defaults to external/production-grade infrastructure
const (
	// EmbeddedNATSMaxAgents is the recommended maximum number of agents
	// for embedded NATS mode. Beyond this, use an external NATS cluster.
	EmbeddedNATSMaxAgents = 100

	// EmbeddedNATSMaxMemoryDefault is the default memory limit for embedded NATS (256MB)
	EmbeddedNATSMaxMemoryDefault = 256 * 1024 * 1024

	// EmbeddedNATSMaxConnectionsDefault is the default max connections for embedded NATS
	EmbeddedNATSMaxConnectionsDefault = 1000

	// JetStreamMaxStorageDefault is the default storage limit for JetStream (1GB)
	JetStreamMaxStorageDefault = 1 * 1024 * 1024 * 1024

	// SQLiteMaxAgents is the recommended maximum number of agents
	// for SQLite storage. Beyond this, use PostgreSQL.
	SQLiteMaxAgents = 100

	// SQLiteMaxStateResources is the recommended maximum state resources
	// for SQLite storage. Beyond this, use PostgreSQL.
	SQLiteMaxStateResources = 10000

	// SQLiteMaxEventsPerDay is the recommended maximum events per day
	// for SQLite storage. Beyond this, use PostgreSQL.
	SQLiteMaxEventsPerDay = 100000
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
	_ = v.BindEnv("agent.id", "KSCORE_AGENT_ID")

	// Logging environment variable bindings (T1.4: Epic 15)
	_ = v.BindEnv("logging.level", "KSCORE_LOG_LEVEL")
	_ = v.BindEnv("logging.format", "KSCORE_LOG_FORMAT")
	_ = v.BindEnv("logging.output", "KSCORE_LOG_OUTPUT")

	// Agent management environment variable bindings (Epic 45 Phase 3)
	_ = v.BindEnv("agentmanagement.heartbeatinterval", "KSCORE_AGENT_MGMT_HEARTBEAT_INTERVAL")
	_ = v.BindEnv("agentmanagement.heartbeattimeout", "KSCORE_AGENT_MGMT_HEARTBEAT_TIMEOUT")
	_ = v.BindEnv("agentmanagement.stalethreshold", "KSCORE_AGENT_MGMT_STALE_THRESHOLD")
	_ = v.BindEnv("agentmanagement.monitorinterval", "KSCORE_AGENT_MGMT_MONITOR_INTERVAL")
	_ = v.BindEnv("agentmanagement.metadatarefresh", "KSCORE_AGENT_MGMT_METADATA_REFRESH")
	_ = v.BindEnv("agentmanagement.maxconcurrentcommands", "KSCORE_AGENT_MGMT_MAX_CONCURRENT_COMMANDS")

	// Execution environment variable bindings (Epic 45 Phase 3)
	_ = v.BindEnv("execution.defaulttimeout", "KSCORE_EXEC_DEFAULT_TIMEOUT")
	_ = v.BindEnv("execution.maxtimeout", "KSCORE_EXEC_MAX_TIMEOUT")
	_ = v.BindEnv("execution.batchsize", "KSCORE_EXEC_BATCH_SIZE")
	_ = v.BindEnv("execution.batchdelay", "KSCORE_EXEC_BATCH_DELAY")
	_ = v.BindEnv("execution.streamingbuffer", "KSCORE_EXEC_STREAMING_BUFFER")
	_ = v.BindEnv("execution.resultretention", "KSCORE_EXEC_RESULT_RETENTION")

	// State management environment variable bindings (Epic 45 Phase 3)
	_ = v.BindEnv("statemanagement.defaulttimeout", "KSCORE_STATE_DEFAULT_TIMEOUT")
	_ = v.BindEnv("statemanagement.maxconcurrent", "KSCORE_STATE_MAX_CONCURRENT")
	_ = v.BindEnv("statemanagement.driftcheckinterval", "KSCORE_STATE_DRIFT_CHECK_INTERVAL")
	_ = v.BindEnv("statemanagement.resultretention", "KSCORE_STATE_RESULT_RETENTION")

	// Events environment variable bindings (Epic 45 Phase 3)
	_ = v.BindEnv("events.enabled", "KSCORE_EVENTS_ENABLED")
	_ = v.BindEnv("events.retention", "KSCORE_EVENTS_RETENTION")
	_ = v.BindEnv("events.maxbytes", "KSCORE_EVENTS_MAX_BYTES")
	_ = v.BindEnv("events.maxmessages", "KSCORE_EVENTS_MAX_MESSAGES")

	// Kubernetes operator environment variable bindings (Epic 48)
	_ = v.BindEnv("operator.enabled", "KSCORE_OPERATOR_ENABLED")
	_ = v.BindEnv("operator.namespace", "KSCORE_OPERATOR_NAMESPACE")
	_ = v.BindEnv("operator.leaderelection", "KSCORE_OPERATOR_LEADER_ELECTION")
	_ = v.BindEnv("operator.leaderelectionid", "KSCORE_OPERATOR_LEADER_ELECTION_ID")
	_ = v.BindEnv("operator.reconcileinterval", "KSCORE_OPERATOR_RECONCILE_INTERVAL")
	_ = v.BindEnv("operator.maxconcurrentreconciles", "KSCORE_OPERATOR_MAX_CONCURRENT_RECONCILES")

	// Authorization environment variable bindings (Epic 45 Phase 3)
	_ = v.BindEnv("auth.authorization.enabled", "KSCORE_AUTHZ_ENABLED")
	_ = v.BindEnv("auth.authorization.defaultdeny", "KSCORE_AUTHZ_DEFAULT_DENY")

	// Read config file (optional)
	if err := v.ReadInConfig(); err != nil {
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if !errors.As(err, &configFileNotFoundError) {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	}

	// Unmarshal into config struct
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Post-process listen addresses
	if err := parseServerListenAddresses(&cfg); err != nil {
		return nil, err
	}
	if err := parseNATSListenAddress(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// parseServerListenAddresses parses the Server HTTPListen and GRPCListen fields
// This allows users to use the documented api.listen and api.grpc_listen formats
func parseServerListenAddresses(cfg *Config) error {
	// Parse HTTPListen (api.listen format)
	if cfg.Server.HTTPListen != "" {
		addr, err := netutil.ParseAddress(cfg.Server.HTTPListen)
		if err != nil {
			return fmt.Errorf("invalid server.httplisten format %q: %w", cfg.Server.HTTPListen, err)
		}
		// Override ListenAddr and HTTPPort
		cfg.Server.ListenAddr = addr.Host
		cfg.Server.HTTPPort = addr.Port
	}

	// Parse GRPCListen (api.grpc_listen format)
	if cfg.Server.GRPCListen != "" {
		addr, err := netutil.ParseAddress(cfg.Server.GRPCListen)
		if err != nil {
			return fmt.Errorf("invalid server.grpclisten format %q: %w", cfg.Server.GRPCListen, err)
		}
		// Override ListenAddr (if not already set by HTTPListen) and GRPCPort
		if cfg.Server.ListenAddr == "" || cfg.Server.ListenAddr == DefaultServerListenAddr {
			cfg.Server.ListenAddr = addr.Host
		}
		cfg.Server.GRPCPort = addr.Port
	}

	return nil
}

// parseNATSListenAddress parses the NATS embedded Listen field into Host and Port
// This allows users to use the combined format "0.0.0.0:4222" as documented
func parseNATSListenAddress(cfg *Config) error {
	listen := cfg.NATS.Embedded.Listen
	if listen == "" {
		return nil
	}

	// Use netutil to parse the address (handles IPv4, IPv6, and port)
	addr, err := netutil.ParseAddress(listen)
	if err != nil {
		return fmt.Errorf("invalid nats.embedded.listen format %q: %w", listen, err)
	}

	// Only override if Listen was explicitly set
	cfg.NATS.Embedded.Host = addr.Host
	cfg.NATS.Embedded.Port = addr.Port
	return nil
}

// setDefaults sets default configuration values
func setDefaults(v *viper.Viper) {
	// Server defaults
	v.SetDefault("server.listenaddr", DefaultServerListenAddr)
	v.SetDefault("server.grpcport", DefaultGRPCPort)
	v.SetDefault("server.httpport", DefaultHTTPPort)
	v.SetDefault("server.addressfamily", DefaultAddressFamily)
	// Allow api.listen and api.grpc_listen as aliases (for documentation compatibility)
	v.RegisterAlias("api.listen", "server.httplisten")
	v.RegisterAlias("api.grpc_listen", "server.grpclisten")

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
	// Allow nats.listen as alias for nats.embedded.listen (for documentation compatibility)
	v.RegisterAlias("nats.listen", "nats.embedded.listen")

	// Storage defaults
	v.SetDefault("storage.backend", DefaultStorageBackend)
	v.SetDefault("storage.sqlite.path", DefaultSQLitePath)
	v.SetDefault("storage.sqlite.wal", DefaultSQLiteWAL)
	v.SetDefault("storage.sqlite.busytimeout", DefaultSQLiteBusyTimeout)
	v.SetDefault("storage.postgresql.maxopenconns", DefaultPostgreSQLMaxOpen)
	v.SetDefault("storage.postgresql.maxidleconns", DefaultPostgreSQLMaxIdle)
	v.SetDefault("storage.postgresql.connmaxlifetime", DefaultPostgreSQLConnLife)
	// Allow storage.type as alias for storage.backend (for documentation compatibility)
	v.RegisterAlias("storage.type", "storage.backend")

	// Agent defaults
	v.SetDefault("agent.heartbeatinterval", DefaultHeartbeatInterval)
	v.SetDefault("agent.commandtimeout", DefaultCommandTimeout)
	v.SetDefault("agent.metadatainterval", DefaultMetadataInterval)
	v.SetDefault("agent.addressfamily", DefaultAddressFamily)

	// Agent management defaults (server-side)
	v.SetDefault("agentmanagement.heartbeatinterval", DefaultHeartbeatInterval)
	v.SetDefault("agentmanagement.heartbeattimeout", DefaultAgentMgmtHeartbeatTimeout)
	v.SetDefault("agentmanagement.stalethreshold", DefaultAgentMgmtStaleThreshold)
	v.SetDefault("agentmanagement.monitorinterval", DefaultAgentMgmtMonitorInterval)
	v.SetDefault("agentmanagement.metadatarefresh", DefaultMetadataInterval)
	v.SetDefault("agentmanagement.maxconcurrentcommands", DefaultAgentMgmtMaxConcurrentCmds)

	// Execution defaults
	v.SetDefault("execution.defaulttimeout", DefaultExecDefaultTimeout)
	v.SetDefault("execution.maxtimeout", DefaultExecMaxTimeout)
	v.SetDefault("execution.batchsize", DefaultExecBatchSize)
	v.SetDefault("execution.batchdelay", 0)
	v.SetDefault("execution.streamingbuffer", DefaultExecStreamingBuffer)
	v.SetDefault("execution.resultretention", DefaultExecResultRetention)

	// State management defaults
	v.SetDefault("statemanagement.defaulttimeout", DefaultStateMgmtDefaultTimeout)
	v.SetDefault("statemanagement.maxconcurrent", DefaultStateMgmtMaxConcurrent)
	v.SetDefault("statemanagement.driftcheckinterval", 0)
	v.SetDefault("statemanagement.resultretention", DefaultStateMgmtResultRetention)

	// Event system defaults
	v.SetDefault("events.enabled", DefaultEventsEnabled)
	v.SetDefault("events.retention", DefaultEventsRetention)
	v.SetDefault("events.maxbytes", DefaultEventsMaxBytes)
	v.SetDefault("events.maxmessages", DefaultEventsMaxMessages)
	v.SetDefault("events.publisherbuffersize", DefaultEventsPublisherBufferSize)
	v.SetDefault("events.subscriberbuffersize", DefaultEventsSubscriberBufferSize)
	v.SetDefault("events.subscriberackwait", DefaultEventsSubscriberAckWait)

	// GitOps defaults
	v.SetDefault("gitops.gitsync.enabled", false)

	// TLS defaults
	v.SetDefault("tls.enabled", false)
	v.SetDefault("tls.minversion", "1.3")

	// Auth defaults - secure by default
	v.SetDefault("auth.enabled", DefaultAuthEnabled)
	v.SetDefault("auth.type", DefaultAuthType)
	v.SetDefault("auth.authorization.enabled", DefaultAuthzEnabled)
	v.SetDefault("auth.authorization.defaultdeny", DefaultAuthzDefaultDeny)
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
	v.SetDefault("logging.syslog.tls.minversion", "1.3")

	// CORS defaults
	v.SetDefault("cors.enabled", DefaultCORSEnabled)
	v.SetDefault("cors.allowedorigins", []string{"*"})
	v.SetDefault("cors.allowedmethods", []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"})
	v.SetDefault("cors.allowedheaders", []string{"Content-Type", "Authorization", "X-API-Key"})
	v.SetDefault("cors.allowcredentials", false)
	v.SetDefault("cors.maxage", 86400) // 24 hours
	// Allow api.cors.* as aliases for cors.*
	v.RegisterAlias("api.cors.enabled", "cors.enabled")
	v.RegisterAlias("api.cors.allowed_origins", "cors.allowedorigins")

	// Rate limit defaults
	v.SetDefault("ratelimit.enabled", DefaultRateLimitEnabled)
	v.SetDefault("ratelimit.requestsperminute", DefaultRateLimitRequestsPerMin)
	v.SetDefault("ratelimit.burst", DefaultRateLimitBurst)
	v.SetDefault("ratelimit.keyextractor", DefaultRateLimitKeyExtractor)
	// Allow api.rate_limit.* as aliases for ratelimit.*
	v.RegisterAlias("api.rate_limit.enabled", "ratelimit.enabled")
	v.RegisterAlias("api.rate_limit.requests_per_minute", "ratelimit.requestsperminute")
	v.RegisterAlias("api.rate_limit.burst", "ratelimit.burst")

	// Metrics defaults
	v.SetDefault("metrics.enabled", DefaultMetricsEnabled)
	v.SetDefault("metrics.path", DefaultMetricsPath)
	v.SetDefault("metrics.includegometrics", DefaultMetricsIncludeGo)
	v.SetDefault("metrics.includeprocessmetrics", DefaultMetricsIncludeProcess)

	// Tracing defaults
	v.SetDefault("tracing.enabled", DefaultTracingEnabled)
	v.SetDefault("tracing.exporter", DefaultTracingExporter)
	v.SetDefault("tracing.sampling.strategy", DefaultTracingSamplingStrategy)
	v.SetDefault("tracing.sampling.ratio", DefaultTracingSamplingRatio)

	// Profiling defaults
	v.SetDefault("profiling.enabled", DefaultProfilingEnabled)
	v.SetDefault("profiling.listen", DefaultProfilingListen)
	v.SetDefault("profiling.blockprofilerate", DefaultProfilingBlockProfileRate)
	v.SetDefault("profiling.mutexprofilefraction", DefaultProfilingMutexFraction)

	// Kubernetes operator defaults
	v.SetDefault("operator.enabled", DefaultOperatorEnabled)
	v.SetDefault("operator.namespace", "")
	v.SetDefault("operator.leaderelection", DefaultOperatorLeaderElection)
	v.SetDefault("operator.leaderelectionid", DefaultOperatorLeaderElectionID)
	v.SetDefault("operator.reconcileinterval", DefaultOperatorReconcileInterval)
	v.SetDefault("operator.maxconcurrentreconciles", DefaultOperatorMaxConcurrentReconciles)

	// Health check defaults
	v.SetDefault("health.enabled", DefaultHealthEnabled)
	v.SetDefault("health.startupgraceperiod", DefaultHealthStartupGracePeriod)
	v.SetDefault("health.checkinterval", DefaultHealthCheckInterval)
	v.SetDefault("health.checks.nats.enabled", true)
	v.SetDefault("health.checks.nats.timeout", DefaultHealthCheckTimeout)
	v.SetDefault("health.checks.database.enabled", true)
	v.SetDefault("health.checks.database.timeout", DefaultHealthCheckTimeout)
	v.SetDefault("health.checks.agents.enabled", true)
	v.SetDefault("health.checks.agents.timeout", DefaultHealthCheckTimeout)
	v.SetDefault("health.checks.agents.minhealthy", DefaultHealthAgentMinHealthy)
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	// Validate NATS configuration with detailed error messages
	if err := c.NATS.Validate(); err != nil {
		return err
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

	// Validate agent management config
	if err := c.AgentManagement.Validate(); err != nil {
		return err
	}

	// Validate execution config
	if err := c.Execution.Validate(); err != nil {
		return err
	}

	// Validate state management config
	if err := c.StateManagement.Validate(); err != nil {
		return err
	}

	// Validate events config
	if err := c.Events.Validate(); err != nil {
		return err
	}

	// Validate GitOps config
	if err := c.GitOps.Validate(); err != nil {
		return err
	}

	// Validate operator config
	if err := c.Operator.Validate(); err != nil {
		return err
	}

	if err := validateTLSMinVersion(c.TLS.MinVersion, "tls.minversion"); err != nil {
		return err
	}

	if err := validateTLSMinVersion(c.Logging.Syslog.TLS.MinVersion, "logging.syslog.tls.minversion"); err != nil {
		return err
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
	// NATS embedded address family is validated in c.NATS.Validate()
	if err := validateAddressFamily(c.Agent.AddressFamily, "agent"); err != nil {
		return err
	}

	// Validate listen addresses if specified (can be host:port format)
	for i, addr := range c.Server.ListenAddrs {
		if _, err := netutil.ParseAddress(addr); err != nil {
			return fmt.Errorf("invalid server.listenaddrs[%d] %q: %w", i, addr, err)
		}
	}

	// SECURITY: Validate TLS is enabled when listening on non-loopback addresses
	// This prevents accidentally exposing the control plane without encryption
	if err := validateTLSForNonLoopback(c); err != nil {
		return err
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

	// Validate rate limit config
	if c.RateLimit.Enabled {
		if c.RateLimit.RequestsPerMinute <= 0 {
			return fmt.Errorf("rate limit requests_per_minute must be positive, got: %d", c.RateLimit.RequestsPerMinute)
		}
		if c.RateLimit.Burst <= 0 {
			return fmt.Errorf("rate limit burst must be positive, got: %d", c.RateLimit.Burst)
		}
		switch c.RateLimit.KeyExtractor {
		case "", "ip", "apikey", "header":
			// valid (empty uses default)
		default:
			return fmt.Errorf("invalid rate limit key_extractor: %s (must be ip, apikey, or header)", c.RateLimit.KeyExtractor)
		}
		if c.RateLimit.KeyExtractor == "header" && c.RateLimit.HeaderName == "" {
			return fmt.Errorf("rate limit header_name is required when key_extractor is 'header'")
		}
	}

	// Validate metrics config
	if c.Metrics.Enabled {
		if c.Metrics.Path != "" && !strings.HasPrefix(c.Metrics.Path, "/") {
			return fmt.Errorf("metrics path must start with '/', got: %s", c.Metrics.Path)
		}
		if c.Metrics.Listen != "" {
			if _, err := netutil.ParseAddress(c.Metrics.Listen); err != nil {
				return fmt.Errorf("invalid metrics listen address %q: %w", c.Metrics.Listen, err)
			}
		}
	}

	// Validate tracing config
	if c.Tracing.Enabled {
		switch c.Tracing.Exporter {
		case "", "otlp", "jaeger", "zipkin":
			// valid (empty uses default)
		default:
			return fmt.Errorf("invalid tracing exporter: %s (must be otlp, jaeger, or zipkin)", c.Tracing.Exporter)
		}
		if c.Tracing.Endpoint == "" {
			return fmt.Errorf("tracing endpoint is required when tracing is enabled")
		}
		switch c.Tracing.Sampling.Strategy {
		case "", "always", "never", "ratio", "parent_based":
			// valid (empty uses default)
		default:
			return fmt.Errorf("invalid tracing sampling strategy: %s (must be always, never, ratio, or parent_based)", c.Tracing.Sampling.Strategy)
		}
		if c.Tracing.Sampling.Strategy == "ratio" {
			if c.Tracing.Sampling.Ratio < 0 || c.Tracing.Sampling.Ratio > 1 {
				return fmt.Errorf("tracing sampling ratio must be between 0.0 and 1.0, got: %f", c.Tracing.Sampling.Ratio)
			}
		}
	}

	// Validate profiling config
	if c.Profiling.Enabled {
		if c.Profiling.Listen == "" {
			return fmt.Errorf("profiling listen address is required when profiling is enabled")
		}
	}

	// Validate health check config
	if c.Health.Enabled {
		if c.Health.StartupGracePeriod < 0 {
			return fmt.Errorf("health startup_grace_period cannot be negative")
		}
		if c.Health.CheckInterval < 0 {
			return fmt.Errorf("health check_interval cannot be negative")
		}
		if c.Health.CheckInterval > 0 && c.Health.CheckInterval < time.Second {
			return fmt.Errorf("health check_interval should be at least 1 second, got: %v", c.Health.CheckInterval)
		}
		// Validate agent health check min_healthy
		if c.Health.Checks.Agents.Enabled {
			if c.Health.Checks.Agents.MinHealthy < 0 || c.Health.Checks.Agents.MinHealthy > 1 {
				return fmt.Errorf("health checks.agents.min_healthy must be between 0.0 and 1.0, got: %f", c.Health.Checks.Agents.MinHealthy)
			}
		}
	}

	return nil
}

// Redact returns a copy of the config with sensitive fields replaced by "[REDACTED]".
// This is used when exposing config via the API to avoid leaking secrets.
func (c *Config) Redact() Config {
	redacted := *c

	// Auth: JWT secret
	if redacted.Auth.JWT.Secret != "" {
		redacted.Auth.JWT.Secret = "[REDACTED]"
	}

	// Auth: API key map (keys are the secret values)
	if len(redacted.Auth.APIKey.Keys) > 0 {
		redactedKeys := make(map[string]APIKeyConfig, len(redacted.Auth.APIKey.Keys))
		i := 0
		for _, v := range redacted.Auth.APIKey.Keys {
			key := fmt.Sprintf("[REDACTED-%d]", i)
			redactedKeys[key] = v
			i++
		}
		redacted.Auth.APIKey.Keys = redactedKeys
	}

	// NATS credentials
	if redacted.NATS.Token != "" {
		redacted.NATS.Token = "[REDACTED]"
	}
	if redacted.NATS.Credential != "" {
		redacted.NATS.Credential = "[REDACTED]"
	}

	// Webhook secrets
	if redacted.Webhook.HMACSecret != "" {
		redacted.Webhook.HMACSecret = "[REDACTED]"
	}
	if redacted.Webhook.BearerToken != "" {
		redacted.Webhook.BearerToken = "[REDACTED]"
	}

	return redacted
}

func validateTLSMinVersion(value, field string) error {
	if value == "" {
		return nil
	}

	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1.2", "tls1.2", "tls12", "1.3", "tls1.3", "tls13":
		return nil
	default:
		return fmt.Errorf("invalid %s: %q (must be 1.2 or 1.3)", field, value)
	}
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

// validateTLSForNonLoopback ensures TLS is enabled when listening on non-loopback addresses.
// This is a critical security check to prevent accidentally exposing the control plane
// without encryption on network interfaces.
func validateTLSForNonLoopback(c *Config) error {
	// If TLS is enabled, we're secure regardless of listen address
	if c.TLS.Enabled {
		return nil
	}

	// If explicitly allowing insecure non-loopback, skip the check
	// (but this should be used with extreme caution)
	if c.Server.AllowInsecureNonLoopback {
		return nil
	}

	// Get effective listen addresses
	listenAddrs := c.Server.GetEffectiveListenAddrs()

	// Check each listen address
	for _, addrStr := range listenAddrs {
		addr, err := netutil.ParseAddress(addrStr)
		if err != nil {
			// Address parsing is validated elsewhere, skip here
			continue
		}

		// If listening on loopback only, it's safe (local connections only)
		if addr.IsLoopback() {
			continue
		}

		// If listening on unspecified (0.0.0.0 or ::), it binds to ALL interfaces
		// including network interfaces - this is NOT safe without TLS
		if addr.IsUnspecified() {
			return fmt.Errorf(
				"SECURITY ERROR: server listening on all interfaces (%s) without TLS enabled. "+
					"This exposes the control plane to network attacks. "+
					"Either: (1) enable TLS with tls.enabled=true and configure certificates, "+
					"(2) change listen address to loopback (127.0.0.1 or ::1), or "+
					"(3) set server.allowinsecurenonloopback=true if TLS is terminated by a reverse proxy "+
					"(NOT RECOMMENDED for direct exposure)",
				addrStr,
			)
		}

		// Non-loopback specific IP address - also not safe without TLS
		return fmt.Errorf(
			"SECURITY ERROR: server listening on non-loopback address (%s) without TLS enabled. "+
				"This exposes the control plane to network attacks. "+
				"Either: (1) enable TLS with tls.enabled=true and configure certificates, "+
				"(2) change listen address to loopback (127.0.0.1 or ::1), or "+
				"(3) set server.allowinsecurenonloopback=true if TLS is terminated by a reverse proxy "+
				"(NOT RECOMMENDED for direct exposure)",
			addrStr,
		)
	}

	return nil
}

// ProductionWarning represents a warning about production readiness
type ProductionWarning struct {
	// Component is the subsystem (nats, storage, jetstream)
	Component string
	// Level is the severity (info, warning, critical)
	Level string
	// Message is the warning message
	Message string
	// Recommendation is what the user should do
	Recommendation string
}

// ProductionWarnings returns warnings about embedded defaults that may not be
// suitable for production deployments. These are not errors - the configuration
// is valid - but users should be aware of scaling limitations.
//
// Call this after Validate() and log warnings during startup.
func (c *Config) ProductionWarnings() []ProductionWarning {
	var warnings []ProductionWarning

	// Check NATS mode
	if c.NATS.Mode == NATSModeEmbedded {
		warnings = append(warnings, ProductionWarning{
			Component: "nats",
			Level:     "warning",
			Message: fmt.Sprintf(
				"Embedded NATS mode is recommended for <%d agents and development/testing. "+
					"Current limits: MaxMemory=%dMB, MaxConnections=%d.",
				EmbeddedNATSMaxAgents,
				getEffectiveNATSMaxMemory(c)/(1024*1024),
				getEffectiveNATSMaxConnections(c),
			),
			Recommendation: "For production deployments with >100 agents, configure an external NATS cluster " +
				"with nats.mode=external and nats.url=nats://your-cluster:4222",
		})

		// Check if JetStream storage is at default
		if c.NATS.JetStream.Enabled && c.NATS.JetStream.MaxStorage == 0 {
			warnings = append(warnings, ProductionWarning{
				Component: "jetstream",
				Level:     "info",
				Message: fmt.Sprintf(
					"JetStream is using default storage limit of %dGB. "+
						"Event storage may fill up under heavy load.",
					JetStreamMaxStorageDefault/(1024*1024*1024),
				),
				Recommendation: "Set nats.jetstream.maxstorage to a value appropriate for your " +
					"expected event volume. Consider external NATS for unlimited JetStream storage.",
			})
		}
	}

	// Check storage backend
	if c.Storage.Backend == StorageBackendSQLite {
		warnings = append(warnings, ProductionWarning{
			Component: "storage",
			Level:     "warning",
			Message: fmt.Sprintf(
				"SQLite storage is recommended for <%d agents and <%d state resources. "+
					"SQLite works well for small deployments but has concurrency limitations.",
				SQLiteMaxAgents,
				SQLiteMaxStateResources,
			),
			Recommendation: "For production deployments with >100 agents, configure PostgreSQL " +
				"with storage.backend=postgresql. Use 'kscore-migrate' to migrate existing data.",
		})
	}

	// Check if running without TLS in a way that suggests production
	// (non-loopback address or external NATS URL)
	if !c.TLS.Enabled {
		isProduction := false
		reason := ""

		// Check for external NATS (suggests production deployment)
		if c.NATS.Mode == NATSModeExternal {
			isProduction = true
			reason = "external NATS cluster configured"
		}

		// Check for PostgreSQL (suggests production deployment)
		if c.Storage.Backend == StorageBackendPostgreSQL {
			isProduction = true
			reason = "PostgreSQL storage configured"
		}

		if isProduction {
			warnings = append(warnings, ProductionWarning{
				Component: "tls",
				Level:     "critical",
				Message: fmt.Sprintf(
					"TLS is disabled but configuration suggests production use (%s). "+
						"All agent communication is unencrypted.",
					reason,
				),
				Recommendation: "Enable TLS with tls.enabled=true and configure certificates. " +
					"See documentation for certificate generation with 'kscore-identity ca'.",
			})
		}
	}

	return warnings
}

// getEffectiveNATSMaxMemory returns the effective max memory for embedded NATS
func getEffectiveNATSMaxMemory(c *Config) int64 {
	if c.NATS.Embedded.MaxMemory > 0 {
		return c.NATS.Embedded.MaxMemory
	}
	return EmbeddedNATSMaxMemoryDefault
}

// getEffectiveNATSMaxConnections returns the effective max connections for embedded NATS
func getEffectiveNATSMaxConnections(c *Config) int {
	if c.NATS.Embedded.MaxConnections > 0 {
		return c.NATS.Embedded.MaxConnections
	}
	return EmbeddedNATSMaxConnectionsDefault
}
