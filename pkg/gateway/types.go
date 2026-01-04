// Package gateway provides telemetry gateway services that aggregate metrics, logs,
// and traces from agents over NATS and expose them to observability backends.
package gateway

import (
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
	Enabled  bool   `yaml:"enabled"`
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
	CAFile   string `yaml:"ca_file"`
	Insecure bool   `yaml:"insecure"`
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
	Enabled   bool     `yaml:"enabled"`
	URLs      []string `yaml:"urls"`
	Index     string   `yaml:"index"`
	BatchSize int      `yaml:"batch_size"`
	TLS       TLSConfig `yaml:"tls"`
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
	Enabled       bool          `yaml:"enabled"`
	Rate          float64       `yaml:"rate"`
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
	Protocol      string            `yaml:"protocol"` // grpc, http
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
