// Package gateway provides telemetry gateway services that aggregate metrics, logs,
// and traces from agents over NATS and expose them to observability backends.
package gateway

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Config holds the complete gateway configuration.
type Config struct {
	// NATS connection configuration
	NATS NATSConfig `yaml:"nats"`

	// HTTP server configuration
	Server ServerConfig `yaml:"server"`

	// Metrics gateway configuration
	Metrics MetricsConfig `yaml:"metrics"`

	// Logs gateway configuration
	Logs LogsConfig `yaml:"logs"`

	// Traces gateway configuration
	Traces TracesConfig `yaml:"traces"`

	// High availability configuration
	HA HAConfig `yaml:"ha"`
}

// NATSConfig holds NATS connection settings.
type NATSConfig struct {
	// URLs is a list of NATS server URLs
	URLs []string `yaml:"urls"`

	// Cluster name for subject prefixing
	Cluster string `yaml:"cluster"`

	// TLS configuration
	TLS TLSConfig `yaml:"tls"`

	// Credentials file path (for JWT/NKey auth)
	CredentialsFile string `yaml:"credentials_file"`

	// Reconnect settings
	MaxReconnects   int           `yaml:"max_reconnects"`
	ReconnectWait   time.Duration `yaml:"reconnect_wait"`
	ReconnectJitter time.Duration `yaml:"reconnect_jitter"`
}

// TLSConfig holds TLS settings.
type TLSConfig struct {
	Enabled    bool   `yaml:"enabled"`
	CertFile   string `yaml:"cert_file"`
	KeyFile    string `yaml:"key_file"`
	CAFile     string `yaml:"ca_file"`
	Insecure   bool   `yaml:"insecure"`
	MinVersion string `yaml:"min_version"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	// Listen address (e.g., "0.0.0.0:9091")
	Listen string `yaml:"listen"`

	// Metrics endpoint path
	MetricsPath string `yaml:"metrics_path"`

	// Health check path
	HealthPath string `yaml:"health_path"`

	// Readiness check path
	ReadyPath string `yaml:"ready_path"`

	// Federation endpoint path
	FederatePath string `yaml:"federate_path"`

	// Read timeout
	ReadTimeout time.Duration `yaml:"read_timeout"`

	// Write timeout
	WriteTimeout time.Duration `yaml:"write_timeout"`
}

// MetricsConfig holds metrics gateway settings.
type MetricsConfig struct {
	// Enabled indicates if metrics gateway is active
	Enabled bool `yaml:"enabled"`

	// Subject to subscribe to for metrics (supports wildcards)
	Subject string `yaml:"subject"`

	// StaleTimeout removes agents not seen for this duration
	StaleTimeout time.Duration `yaml:"stale_timeout"`

	// Labels configuration
	Labels LabelsConfig `yaml:"labels"`

	// Cardinality control settings
	Cardinality CardinalityConfig `yaml:"cardinality"`

	// Remote write configuration (push to Prometheus)
	RemoteWrite RemoteWriteConfig `yaml:"remote_write"`

	// Federation settings
	Federation FederationConfig `yaml:"federation"`
}

// LabelsConfig holds label manipulation settings.
type LabelsConfig struct {
	// Add labels to all metrics
	Add map[string]string `yaml:"add"`

	// Drop these label names
	Drop []string `yaml:"drop"`

	// Rewrite rules (source -> target)
	Rewrite []LabelRewrite `yaml:"rewrite"`
}

// LabelRewrite defines a label rewriting rule.
type LabelRewrite struct {
	Source string `yaml:"source"`
	Target string `yaml:"target"`
}

// CardinalityConfig holds cardinality control settings.
type CardinalityConfig struct {
	// MaxSeries limits total metric series
	MaxSeries int `yaml:"max_series"`

	// MaxLabelsPerSeries limits labels per series
	MaxLabelsPerSeries int `yaml:"max_labels_per_series"`

	// DropHighCardinality automatically drops high cardinality metrics
	DropHighCardinality bool `yaml:"drop_high_cardinality"`
}

// RemoteWriteConfig holds Prometheus remote write settings.
type RemoteWriteConfig struct {
	Enabled       bool          `yaml:"enabled"`
	URL           string        `yaml:"url"`
	BatchSize     int           `yaml:"batch_size"`
	FlushInterval time.Duration `yaml:"flush_interval"`
	Auth          AuthConfig    `yaml:"auth"`
	TLS           TLSConfig     `yaml:"tls"`
	Retry         RetryConfig   `yaml:"retry"`
}

// AuthConfig holds authentication settings.
type AuthConfig struct {
	// Type: none, basic, bearer
	Type     string `yaml:"type"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	Token    string `yaml:"token"`
}

// RetryConfig holds retry settings.
type RetryConfig struct {
	MaxAttempts int           `yaml:"max_attempts"`
	Backoff     time.Duration `yaml:"backoff"`
	MaxBackoff  time.Duration `yaml:"max_backoff"`
}

// FederationConfig holds Prometheus federation settings.
type FederationConfig struct {
	Enabled bool   `yaml:"enabled"`
	Path    string `yaml:"path"`
}

// LogsConfig holds logs gateway settings.
type LogsConfig struct {
	// Enabled indicates if logs gateway is active
	Enabled bool `yaml:"enabled"`

	// Subject to subscribe to for logs
	Subject string `yaml:"subject"`

	// MinLevel filters logs below this level (debug, info, warn, error)
	MinLevel string `yaml:"min_level"`

	// Sources filtering
	Sources SourcesFilter `yaml:"sources"`

	// Loki output configuration
	Loki LokiConfig `yaml:"loki"`

	// Elasticsearch output configuration
	Elasticsearch ElasticsearchConfig `yaml:"elasticsearch"`
}

// SourcesFilter holds source filtering settings.
type SourcesFilter struct {
	Include []string `yaml:"include"`
	Exclude []string `yaml:"exclude"`
}

// LokiConfig holds Loki push settings.
type LokiConfig struct {
	Enabled   bool          `yaml:"enabled"`
	URL       string        `yaml:"url"`
	BatchSize int           `yaml:"batch_size"`
	BatchWait time.Duration `yaml:"batch_wait"`
	TenantID  string        `yaml:"tenant_id"`
	Labels    []string      `yaml:"labels"`
	Retry     RetryConfig   `yaml:"retry"`
	TLS       TLSConfig     `yaml:"tls"`
	Auth      AuthConfig    `yaml:"auth"`
}

// ElasticsearchConfig holds Elasticsearch settings.
type ElasticsearchConfig struct {
	Enabled   bool       `yaml:"enabled"`
	URLs      []string   `yaml:"urls"`
	Index     string     `yaml:"index"`
	BatchSize int        `yaml:"batch_size"`
	TLS       TLSConfig  `yaml:"tls"`
	Auth      AuthConfig `yaml:"auth"`
}

// TracesConfig holds traces gateway settings.
type TracesConfig struct {
	// Enabled indicates if traces gateway is active
	Enabled bool `yaml:"enabled"`

	// Subject to subscribe to for traces
	Subject string `yaml:"subject"`

	// Sampling configuration
	Sampling SamplingConfig `yaml:"sampling"`

	// OTLP output configuration
	OTLP OTLPConfig `yaml:"otlp"`
}

// SamplingConfig holds trace sampling settings.
type SamplingConfig struct {
	Enabled        bool                 `yaml:"enabled"`
	Rate           float64              `yaml:"rate"`
	PrioritySample PrioritySampleConfig `yaml:"priority_sample"`
}

// PrioritySampleConfig holds priority-based sampling settings.
type PrioritySampleConfig struct {
	Errors        bool          `yaml:"errors"`
	SlowThreshold time.Duration `yaml:"slow_threshold"`
}

// OTLPConfig holds OTLP exporter settings.
type OTLPConfig struct {
	Enabled       bool              `yaml:"enabled"`
	Endpoint      string            `yaml:"endpoint"`
	Protocol      string            `yaml:"protocol"`    // grpc, http
	Compression   string            `yaml:"compression"` // gzip, none
	BatchSize     int               `yaml:"batch_size"`
	FlushInterval time.Duration     `yaml:"flush_interval"`
	Headers       map[string]string `yaml:"headers"`
	TLS           TLSConfig         `yaml:"tls"`
}

// HAConfig holds high availability settings.
type HAConfig struct {
	Enabled        bool                 `yaml:"enabled"`
	QueueGroup     string               `yaml:"queue_group"`
	LeaderElection LeaderElectionConfig `yaml:"leader_election"`
}

// LeaderElectionConfig holds leader election settings for HA.
type LeaderElectionConfig struct {
	Enabled       bool          `yaml:"enabled"`
	LeaseDuration time.Duration `yaml:"lease_duration"`
	RenewDeadline time.Duration `yaml:"renew_deadline"`
}

// DefaultConfig returns a configuration with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		NATS: NATSConfig{
			URLs:            []string{"nats://localhost:4222"},
			Cluster:         "default",
			MaxReconnects:   -1, // Infinite
			ReconnectWait:   2 * time.Second,
			ReconnectJitter: 500 * time.Millisecond,
		},
		Server: ServerConfig{
			Listen:       "0.0.0.0:9091",
			MetricsPath:  "/metrics",
			HealthPath:   "/health",
			ReadyPath:    "/ready",
			FederatePath: "/federate",
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
		},
		Metrics: MetricsConfig{
			Enabled:      true,
			Subject:      "kscore.telemetry.metrics.>",
			StaleTimeout: 60 * time.Second,
			Labels: LabelsConfig{
				Add:  map[string]string{},
				Drop: []string{},
			},
			Cardinality: CardinalityConfig{
				MaxSeries:          100000,
				MaxLabelsPerSeries: 20,
			},
			RemoteWrite: RemoteWriteConfig{
				BatchSize:     1000,
				FlushInterval: 15 * time.Second,
				Retry: RetryConfig{
					MaxAttempts: 3,
					Backoff:     1 * time.Second,
					MaxBackoff:  30 * time.Second,
				},
			},
			Federation: FederationConfig{
				Enabled: true,
				Path:    "/federate",
			},
		},
		Logs: LogsConfig{
			Enabled:  true,
			Subject:  "kscore.telemetry.logs.>",
			MinLevel: "info",
			Loki: LokiConfig{
				BatchSize: 100,
				BatchWait: 1 * time.Second,
				Labels:    []string{"agent_id", "level", "source"},
				Retry: RetryConfig{
					MaxAttempts: 3,
					Backoff:     1 * time.Second,
					MaxBackoff:  30 * time.Second,
				},
			},
			Elasticsearch: ElasticsearchConfig{
				Index:     "kscore-logs-%Y.%m.%d",
				BatchSize: 500,
			},
		},
		Traces: TracesConfig{
			Enabled: true,
			Subject: "kscore.telemetry.traces.>",
			Sampling: SamplingConfig{
				Enabled: true,
				Rate:    1.0,
				PrioritySample: PrioritySampleConfig{
					Errors:        true,
					SlowThreshold: 1 * time.Second,
				},
			},
			OTLP: OTLPConfig{
				Protocol:      "grpc",
				Compression:   "gzip",
				BatchSize:     100,
				FlushInterval: 5 * time.Second,
			},
		},
		HA: HAConfig{
			QueueGroup: "kscore-gateway",
			LeaderElection: LeaderElectionConfig{
				LeaseDuration: 15 * time.Second,
				RenewDeadline: 10 * time.Second,
			},
		},
	}
}

// ValidationError represents a configuration validation error
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// ValidationErrors is a collection of validation errors
type ValidationErrors []ValidationError

func (e ValidationErrors) Error() string {
	if len(e) == 0 {
		return "no errors"
	}
	if len(e) == 1 {
		return e[0].Error()
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%d validation errors:\n", len(e)))
	for _, err := range e {
		sb.WriteString(fmt.Sprintf("  - %s\n", err.Error()))
	}
	return sb.String()
}

// HasErrors returns true if there are any validation errors
func (e ValidationErrors) HasErrors() bool {
	return len(e) > 0
}

// Validate validates the complete gateway configuration
func (c *Config) Validate() error {
	var errors ValidationErrors

	// Validate NATS config
	errors = append(errors, c.NATS.Validate("nats")...)

	// Validate Server config
	errors = append(errors, c.Server.Validate("server")...)

	// Validate Metrics config
	errors = append(errors, c.Metrics.Validate("metrics")...)

	// Validate Logs config
	errors = append(errors, c.Logs.Validate("logs")...)

	// Validate Traces config
	errors = append(errors, c.Traces.Validate("traces")...)

	// Validate HA config
	errors = append(errors, c.HA.Validate("ha")...)

	// Cross-field validation
	if !c.Metrics.Enabled && !c.Logs.Enabled && !c.Traces.Enabled {
		errors = append(errors, ValidationError{
			Field:   "metrics/logs/traces",
			Message: "at least one telemetry type (metrics, logs, or traces) must be enabled",
		})
	}

	if errors.HasErrors() {
		return errors
	}
	return nil
}

// Validate validates NATS configuration
func (c *NATSConfig) Validate(prefix string) ValidationErrors {
	var errors ValidationErrors

	if len(c.URLs) == 0 {
		errors = append(errors, ValidationError{
			Field:   prefix + ".urls",
			Message: "at least one NATS URL is required",
		})
	}

	for i, u := range c.URLs {
		if err := validateNATSURL(u); err != nil {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("%s.urls[%d]", prefix, i),
				Message: err.Error(),
			})
		}
	}

	if c.Cluster == "" {
		errors = append(errors, ValidationError{
			Field:   prefix + ".cluster",
			Message: "cluster name is required",
		})
	}

	if c.ReconnectWait < 0 {
		errors = append(errors, ValidationError{
			Field:   prefix + ".reconnect_wait",
			Message: "reconnect_wait cannot be negative",
		})
	}

	errors = append(errors, c.TLS.Validate(prefix+".tls")...)

	return errors
}

// validateNATSURL validates a NATS URL format
func validateNATSURL(u string) error {
	parsed, err := url.Parse(u)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	switch parsed.Scheme {
	case "nats", "tls", "ws", "wss":
		// Valid schemes
	case "":
		return fmt.Errorf("URL scheme is required (nats://, tls://, ws(s)://)")
	default:
		return fmt.Errorf("unsupported URL scheme %q (expected nats, tls, ws, or wss)", parsed.Scheme)
	}

	if parsed.Host == "" {
		return fmt.Errorf("URL host is required")
	}

	return nil
}

// Validate validates TLS configuration
func (c *TLSConfig) Validate(prefix string) ValidationErrors {
	var errors ValidationErrors

	if !c.Enabled {
		return errors
	}

	if c.Insecure {
		// Insecure mode, no cert validation needed
		return errors
	}

	if c.MinVersion != "" {
		switch strings.ToLower(strings.TrimSpace(c.MinVersion)) {
		case "1.2", "tls1.2", "tls12", "1.3", "tls1.3", "tls13":
			// valid
		default:
			errors = append(errors, ValidationError{
				Field:   prefix + ".min_version",
				Message: "min_version must be 1.2 or 1.3",
			})
		}
	}

	if c.CertFile != "" && c.KeyFile == "" {
		errors = append(errors, ValidationError{
			Field:   prefix + ".key_file",
			Message: "key_file is required when cert_file is specified",
		})
	}

	if c.KeyFile != "" && c.CertFile == "" {
		errors = append(errors, ValidationError{
			Field:   prefix + ".cert_file",
			Message: "cert_file is required when key_file is specified",
		})
	}

	return errors
}

// Validate validates Server configuration
func (c *ServerConfig) Validate(prefix string) ValidationErrors {
	var errors ValidationErrors

	if c.Listen == "" {
		errors = append(errors, ValidationError{
			Field:   prefix + ".listen",
			Message: "listen address is required",
		})
	}

	if c.MetricsPath == "" {
		errors = append(errors, ValidationError{
			Field:   prefix + ".metrics_path",
			Message: "metrics_path is required",
		})
	}

	if c.ReadTimeout < 0 {
		errors = append(errors, ValidationError{
			Field:   prefix + ".read_timeout",
			Message: "read_timeout cannot be negative",
		})
	}

	if c.WriteTimeout < 0 {
		errors = append(errors, ValidationError{
			Field:   prefix + ".write_timeout",
			Message: "write_timeout cannot be negative",
		})
	}

	return errors
}

// Validate validates Metrics configuration
func (c *MetricsConfig) Validate(prefix string) ValidationErrors {
	var errors ValidationErrors

	if !c.Enabled {
		return errors
	}

	if c.Subject == "" {
		errors = append(errors, ValidationError{
			Field:   prefix + ".subject",
			Message: "subject is required when metrics is enabled",
		})
	}

	if c.StaleTimeout < 0 {
		errors = append(errors, ValidationError{
			Field:   prefix + ".stale_timeout",
			Message: "stale_timeout cannot be negative",
		})
	}

	if c.Cardinality.MaxSeries < 0 {
		errors = append(errors, ValidationError{
			Field:   prefix + ".cardinality.max_series",
			Message: "max_series cannot be negative",
		})
	}

	if c.Cardinality.MaxLabelsPerSeries < 0 {
		errors = append(errors, ValidationError{
			Field:   prefix + ".cardinality.max_labels_per_series",
			Message: "max_labels_per_series cannot be negative",
		})
	}

	errors = append(errors, c.RemoteWrite.Validate(prefix+".remote_write")...)

	return errors
}

// Validate validates RemoteWrite configuration
func (c *RemoteWriteConfig) Validate(prefix string) ValidationErrors {
	var errors ValidationErrors

	if !c.Enabled {
		return errors
	}

	if c.URL == "" {
		errors = append(errors, ValidationError{
			Field:   prefix + ".url",
			Message: "url is required when remote_write is enabled",
		})
	} else if _, err := url.Parse(c.URL); err != nil {
		errors = append(errors, ValidationError{
			Field:   prefix + ".url",
			Message: fmt.Sprintf("invalid URL: %v", err),
		})
	}

	if c.BatchSize <= 0 {
		errors = append(errors, ValidationError{
			Field:   prefix + ".batch_size",
			Message: "batch_size must be greater than 0",
		})
	}

	if c.FlushInterval <= 0 {
		errors = append(errors, ValidationError{
			Field:   prefix + ".flush_interval",
			Message: "flush_interval must be greater than 0",
		})
	}

	errors = append(errors, c.Auth.Validate(prefix+".auth")...)
	errors = append(errors, c.TLS.Validate(prefix+".tls")...)
	errors = append(errors, c.Retry.Validate(prefix+".retry")...)

	return errors
}

// Validate validates Auth configuration
func (c *AuthConfig) Validate(prefix string) ValidationErrors {
	var errors ValidationErrors

	switch c.Type {
	case "", "none":
		// No auth, valid
	case "basic":
		if c.Username == "" {
			errors = append(errors, ValidationError{
				Field:   prefix + ".username",
				Message: "username is required for basic auth",
			})
		}
		if c.Password == "" {
			errors = append(errors, ValidationError{
				Field:   prefix + ".password",
				Message: "password is required for basic auth",
			})
		}
	case "bearer":
		if c.Token == "" {
			errors = append(errors, ValidationError{
				Field:   prefix + ".token",
				Message: "token is required for bearer auth",
			})
		}
	default:
		errors = append(errors, ValidationError{
			Field:   prefix + ".type",
			Message: fmt.Sprintf("unsupported auth type %q (expected: none, basic, or bearer)", c.Type),
		})
	}

	return errors
}

// Validate validates Retry configuration
func (c *RetryConfig) Validate(prefix string) ValidationErrors {
	var errors ValidationErrors

	if c.MaxAttempts < 0 {
		errors = append(errors, ValidationError{
			Field:   prefix + ".max_attempts",
			Message: "max_attempts cannot be negative",
		})
	}

	if c.Backoff < 0 {
		errors = append(errors, ValidationError{
			Field:   prefix + ".backoff",
			Message: "backoff cannot be negative",
		})
	}

	if c.MaxBackoff < 0 {
		errors = append(errors, ValidationError{
			Field:   prefix + ".max_backoff",
			Message: "max_backoff cannot be negative",
		})
	}

	if c.MaxBackoff > 0 && c.Backoff > 0 && c.MaxBackoff < c.Backoff {
		errors = append(errors, ValidationError{
			Field:   prefix + ".max_backoff",
			Message: "max_backoff must be greater than or equal to backoff",
		})
	}

	return errors
}

// Validate validates Logs configuration
func (c *LogsConfig) Validate(prefix string) ValidationErrors {
	var errors ValidationErrors

	if !c.Enabled {
		return errors
	}

	if c.Subject == "" {
		errors = append(errors, ValidationError{
			Field:   prefix + ".subject",
			Message: "subject is required when logs is enabled",
		})
	}

	// Validate min_level
	switch strings.ToLower(c.MinLevel) {
	case "", "debug", "info", "warn", "warning", "error", "fatal":
		// Valid levels
	default:
		errors = append(errors, ValidationError{
			Field:   prefix + ".min_level",
			Message: fmt.Sprintf("invalid log level %q (expected: debug, info, warn, error, or fatal)", c.MinLevel),
		})
	}

	errors = append(errors, c.Loki.Validate(prefix+".loki")...)
	errors = append(errors, c.Elasticsearch.Validate(prefix+".elasticsearch")...)

	// Warn if no output is configured
	if c.Enabled && !c.Loki.Enabled && !c.Elasticsearch.Enabled {
		errors = append(errors, ValidationError{
			Field:   prefix,
			Message: "logs is enabled but no output (loki or elasticsearch) is configured",
		})
	}

	return errors
}

// Validate validates Loki configuration
func (c *LokiConfig) Validate(prefix string) ValidationErrors {
	var errors ValidationErrors

	if !c.Enabled {
		return errors
	}

	if c.URL == "" {
		errors = append(errors, ValidationError{
			Field:   prefix + ".url",
			Message: "url is required when loki is enabled",
		})
	} else if _, err := url.Parse(c.URL); err != nil {
		errors = append(errors, ValidationError{
			Field:   prefix + ".url",
			Message: fmt.Sprintf("invalid URL: %v", err),
		})
	}

	if c.BatchSize <= 0 {
		errors = append(errors, ValidationError{
			Field:   prefix + ".batch_size",
			Message: "batch_size must be greater than 0",
		})
	}

	if c.BatchWait <= 0 {
		errors = append(errors, ValidationError{
			Field:   prefix + ".batch_wait",
			Message: "batch_wait must be greater than 0",
		})
	}

	errors = append(errors, c.TLS.Validate(prefix+".tls")...)
	errors = append(errors, c.Auth.Validate(prefix+".auth")...)
	errors = append(errors, c.Retry.Validate(prefix+".retry")...)

	return errors
}

// Validate validates Elasticsearch configuration
func (c *ElasticsearchConfig) Validate(prefix string) ValidationErrors {
	var errors ValidationErrors

	if !c.Enabled {
		return errors
	}

	if len(c.URLs) == 0 {
		errors = append(errors, ValidationError{
			Field:   prefix + ".urls",
			Message: "at least one URL is required when elasticsearch is enabled",
		})
	}

	for i, u := range c.URLs {
		if _, err := url.Parse(u); err != nil {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("%s.urls[%d]", prefix, i),
				Message: fmt.Sprintf("invalid URL: %v", err),
			})
		}
	}

	if c.Index == "" {
		errors = append(errors, ValidationError{
			Field:   prefix + ".index",
			Message: "index is required when elasticsearch is enabled",
		})
	}

	if c.BatchSize <= 0 {
		errors = append(errors, ValidationError{
			Field:   prefix + ".batch_size",
			Message: "batch_size must be greater than 0",
		})
	}

	errors = append(errors, c.TLS.Validate(prefix+".tls")...)
	errors = append(errors, c.Auth.Validate(prefix+".auth")...)

	return errors
}

// Validate validates Traces configuration
func (c *TracesConfig) Validate(prefix string) ValidationErrors {
	var errors ValidationErrors

	if !c.Enabled {
		return errors
	}

	if c.Subject == "" {
		errors = append(errors, ValidationError{
			Field:   prefix + ".subject",
			Message: "subject is required when traces is enabled",
		})
	}

	errors = append(errors, c.Sampling.Validate(prefix+".sampling")...)
	errors = append(errors, c.OTLP.Validate(prefix+".otlp")...)

	return errors
}

// Validate validates Sampling configuration
func (c *SamplingConfig) Validate(prefix string) ValidationErrors {
	var errors ValidationErrors

	if !c.Enabled {
		return errors
	}

	if c.Rate < 0 || c.Rate > 1 {
		errors = append(errors, ValidationError{
			Field:   prefix + ".rate",
			Message: "rate must be between 0 and 1",
		})
	}

	if c.PrioritySample.SlowThreshold < 0 {
		errors = append(errors, ValidationError{
			Field:   prefix + ".priority_sample.slow_threshold",
			Message: "slow_threshold cannot be negative",
		})
	}

	return errors
}

// Validate validates OTLP configuration
func (c *OTLPConfig) Validate(prefix string) ValidationErrors {
	var errors ValidationErrors

	if !c.Enabled {
		return errors
	}

	if c.Endpoint == "" {
		errors = append(errors, ValidationError{
			Field:   prefix + ".endpoint",
			Message: "endpoint is required when otlp is enabled",
		})
	}

	// Validate protocol
	switch strings.ToLower(c.Protocol) {
	case "", "grpc", "http", "http/protobuf":
		// Valid protocols
	default:
		errors = append(errors, ValidationError{
			Field:   prefix + ".protocol",
			Message: fmt.Sprintf("unsupported protocol %q (expected: grpc or http)", c.Protocol),
		})
	}

	// Validate compression
	switch strings.ToLower(c.Compression) {
	case "", "none", "gzip":
		// Valid compression options
	default:
		errors = append(errors, ValidationError{
			Field:   prefix + ".compression",
			Message: fmt.Sprintf("unsupported compression %q (expected: none or gzip)", c.Compression),
		})
	}

	if c.BatchSize <= 0 {
		errors = append(errors, ValidationError{
			Field:   prefix + ".batch_size",
			Message: "batch_size must be greater than 0",
		})
	}

	if c.FlushInterval <= 0 {
		errors = append(errors, ValidationError{
			Field:   prefix + ".flush_interval",
			Message: "flush_interval must be greater than 0",
		})
	}

	errors = append(errors, c.TLS.Validate(prefix+".tls")...)

	return errors
}

// Validate validates HA configuration
func (c *HAConfig) Validate(prefix string) ValidationErrors {
	var errors ValidationErrors

	if !c.Enabled {
		return errors
	}

	if c.QueueGroup == "" {
		errors = append(errors, ValidationError{
			Field:   prefix + ".queue_group",
			Message: "queue_group is required when HA is enabled",
		})
	}

	errors = append(errors, c.LeaderElection.Validate(prefix+".leader_election")...)

	return errors
}

// Validate validates LeaderElection configuration
func (c *LeaderElectionConfig) Validate(prefix string) ValidationErrors {
	var errors ValidationErrors

	if !c.Enabled {
		return errors
	}

	if c.LeaseDuration <= 0 {
		errors = append(errors, ValidationError{
			Field:   prefix + ".lease_duration",
			Message: "lease_duration must be greater than 0",
		})
	}

	if c.RenewDeadline <= 0 {
		errors = append(errors, ValidationError{
			Field:   prefix + ".renew_deadline",
			Message: "renew_deadline must be greater than 0",
		})
	}

	if c.LeaseDuration > 0 && c.RenewDeadline > 0 && c.RenewDeadline >= c.LeaseDuration {
		errors = append(errors, ValidationError{
			Field:   prefix + ".renew_deadline",
			Message: "renew_deadline must be less than lease_duration",
		})
	}

	return errors
}
