// SPDX-License-Identifier: Apache-2.0

// Package config defines the runtime configuration for all Keystone Core
// binaries. It loads from YAML files plus KSCORE_-prefixed env vars,
// validates after unmarshal, and emits production warnings for risky
// combinations.
package config

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/structs"
	"github.com/knadh/koanf/v2"
)

// Mode is the runtime deployment mode.
type Mode string

const (
	ModeDevelopment Mode = "development"
	ModeProduction  Mode = "production"
)

// Validate returns an error if the mode is not a recognized value.
func (m Mode) Validate() error {
	switch m {
	case ModeDevelopment, ModeProduction:
		return nil
	default:
		return fmt.Errorf("mode: %q (must be development or production)", string(m))
	}
}

// Config is the root runtime configuration for all Keystone Core binaries.
// Foundation sub-configs (Server, Logging, Storage) are defined here; later
// epics extend Config with their own domain sub-configs.
type Config struct {
	Mode     Mode           `koanf:"mode"`
	Server   ServerConfig   `koanf:"server"`
	Logging  LoggingConfig  `koanf:"logging"`
	Storage  StorageConfig  `koanf:"storage"`
	Health   HealthConfig   `koanf:"health"`
	NATS     NATSConfig     `koanf:"nats"`
	Agent    AgentConfig    `koanf:"agent"`
	Security SecurityConfig `koanf:"security"`
	Identity IdentityConfig `koanf:"identity"`
	Secrets  SecretsConfig  `koanf:"secrets"`
	Events   EventsConfig   `koanf:"events"`
	Cluster  ClusterConfig  `koanf:"cluster"`
	GitOps   GitOpsConfig   `koanf:"gitops"`
	Webhook  WebhookConfig  `koanf:"webhook"`
	Metrics   MetricsConfig   `koanf:"metrics"`
	Tracing   TracingConfig   `koanf:"tracing"`
	Profiling  ProfilingConfig  `koanf:"profiling"`
	Blueprints BlueprintsConfig `koanf:"blueprints"`
}

// defaultConfig returns the built-in defaults applied before YAML/env overlays.
func defaultConfig() *Config {
	c := &Config{
		Mode: ModeDevelopment,
		Server: ServerConfig{
			Host:     "0.0.0.0",
			GRPCPort: 5397,
			HTTPPort: 8080,
			// Epic 09 task 13: TLS defaults on — a fresh
			// kscore-server boots with mTLS-aware TLS, deriving
			// the server cert from the identity provider. Dev
			// fixtures (testdata/dev.yaml) explicitly opt out
			// via `server: { tls: { enabled: false } }`.
			TLS: TLSConfig{Enabled: true},
			CORS: CORSConfig{
				Enabled:        true,
				AllowedOrigins: []string{"*"},
				AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
				AllowedHeaders: []string{"Authorization", "Content-Type"},
			},
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
		},
		Storage: StorageConfig{
			Driver: "sqlite",
			DSN:    "./data/keystone.db",
		},
		Health: HealthConfig{
			StartupGracePeriod: 30 * time.Second,
			CheckTimeout:       2 * time.Second,
		},
		NATS: NATSConfig{
			Mode:              NATSModeEmbedded,
			ClusterName:       "default",
			MaxReconnects:     60,
			ReconnectWait:     2 * time.Second,
			MaxReconnectDelay: 30 * time.Second,
			ReconnectJitter:   0.2,
			JetStream: JetStreamConfig{
				Enabled:        true,
				StoreDir:       "./data/jetstream",
				MaxStorage:     10 * 1024 * 1024 * 1024, // 10 GiB server-level cap
				StreamMaxAge:   7 * 24 * time.Hour,      // §4.2 default
				StreamMaxBytes: 10 * 1024 * 1024 * 1024, // 10 GiB per stream
				StreamMaxMsgs:  1_000_000,
				StreamReplicas: 1, // single-node v1.0; epic 13 raises for HA
			},
			Embedded: EmbeddedNATSConfig{
				Host:           "127.0.0.1",
				Port:           4222,
				MaxConnections: 0,
				MaxMemory:      0,
			},
			Dedup: DedupConfig{
				Enabled:         true,
				WindowDuration:  5 * time.Minute, // PROJECT-DETAILS §4.2 default
				MaxEntries:      100_000,
				CleanupInterval: 30 * time.Second,
			},
			CircuitBreaker: CircuitBreakerConfig{
				Enabled:             true,
				FailureThreshold:    5,
				SuccessThreshold:    2,
				OpenDuration:        30 * time.Second,
				HalfOpenMaxAttempts: 3,
			},
		},
		Agent: AgentConfig{
			HeartbeatInterval: 30 * time.Second, // §4.6 default
			MetadataInterval:  60 * time.Second, // §4.6 default
			CommandTimeout:    5 * time.Minute,
		},
		Security: SecurityConfig{
			MaxArgsBytes:  64 * 1024, // §4.7 default
			DefaultPolicy: "deny",    // safer baseline; operators explicitly allow
		},
		Identity: IdentityConfig{
			Enabled:     true,
			TrustDomain: "kscore.local",
			StoragePath: "./data/identity",
			KeyType:     "ecdsa-p256",
		},
		Metrics: MetricsConfig{
			Enabled: true,
			Path:    "/metrics",
		},
		Tracing: TracingConfig{
			Enabled:            false, // opt-in; sampling overhead
			ServiceName:        "kscore-server",
			Exporter:           TracingExporterStdout,
			Sampler:            TracingSamplerProbabilistic,
			SampleRate:         0.1,
			RateLimitPerSecond: 100,
			BatchSize:          512,
			QueueSize:          2048,
			FlushInterval:      5 * time.Second,
		},
		Profiling: ProfilingConfig{
			Enabled:         false, // opt-in; pprof leaks heap state
			Host:            "127.0.0.1",
			Port:            6060,
			ShutdownTimeout: 5 * time.Second,
		},
	}
	applyEventsDefaults(&c.Events)
	applyClusterDefaults(&c.Cluster)
	applyGitOpsDefaults(&c.GitOps)
	applyWebhookDefaults(&c.Webhook)
	return c
}

// Load reads YAML at path (pass "" to skip), applies defaults, overlays
// KSCORE_-prefixed env vars, validates, and returns the Config.
func Load(path string) (*Config, error) {
	k := koanf.New(".")

	if err := k.Load(structs.Provider(defaultConfig(), "koanf"), nil); err != nil {
		return nil, fmt.Errorf("load defaults: %w", err)
	}

	if path != "" {
		if err := k.Load(file.Provider(path), yaml.Parser()); err != nil {
			return nil, fmt.Errorf("load yaml %q: %w", path, err)
		}
	}

	if err := k.Load(env.Provider("KSCORE_", ".", envKeyMapper), nil); err != nil {
		return nil, fmt.Errorf("load env: %w", err)
	}

	cfg := &Config{}
	if err := k.UnmarshalWithConf("", cfg, koanf.UnmarshalConf{Tag: "koanf"}); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate: %w", err)
	}
	return cfg, nil
}

// envKeyMapper converts KSCORE_SERVER_HOST → server.host. Single-word koanf
// keys make this lossless without needing escape conventions.
func envKeyMapper(s string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimPrefix(s, "KSCORE_")), "_", ".")
}

// Validate returns an error if any field is out of range or contradictory.
func (c *Config) Validate() error {
	if err := c.Mode.Validate(); err != nil {
		return err
	}
	if err := c.Server.Validate(); err != nil {
		return fmt.Errorf("server: %w", err)
	}
	if err := c.Logging.Validate(); err != nil {
		return fmt.Errorf("logging: %w", err)
	}
	if err := c.Storage.Validate(); err != nil {
		return fmt.Errorf("storage: %w", err)
	}
	if err := c.Health.Validate(); err != nil {
		return fmt.Errorf("health: %w", err)
	}
	if err := c.NATS.Validate(); err != nil {
		return fmt.Errorf("nats: %w", err)
	}
	if err := c.Agent.Validate(); err != nil {
		return fmt.Errorf("agent: %w", err)
	}
	if err := c.Security.Validate(); err != nil {
		return fmt.Errorf("security: %w", err)
	}
	if err := c.Secrets.Validate(); err != nil {
		return err
	}
	if err := c.Events.Validate(c.NATS); err != nil {
		return err
	}
	if err := c.Cluster.Validate(); err != nil {
		return err
	}
	if err := c.GitOps.Validate(); err != nil {
		return err
	}
	if err := c.Webhook.Validate(); err != nil {
		return err
	}
	if err := c.Metrics.Validate(); err != nil {
		return fmt.Errorf("metrics: %w", err)
	}
	if err := c.Tracing.Validate(); err != nil {
		return fmt.Errorf("tracing: %w", err)
	}
	if err := c.Profiling.Validate(); err != nil {
		return fmt.Errorf("profiling: %w", err)
	}
	return nil
}

// ProductionWarnings returns human-readable warnings for risky combinations.
// Empty if Mode != ModeProduction. Caller decides whether to log/print/fail.
//
// Server-state warnings (e.g., "auth disabled") are added on top by
// pkg/api/server.Server.ProductionWarnings, which augments this list
// with runtime-only signals.
func (c *Config) ProductionWarnings() []string {
	if c.Mode != ModeProduction {
		return nil
	}
	var w []string
	if !c.Server.TLS.Enabled {
		w = append(w, "TLS is disabled in production")
	}
	if c.Storage.Driver == "sqlite" {
		w = append(w, "SQLite is not recommended for production (use postgres for HA)")
	}
	// Single-node (clustering disabled) is a supported topology,
	// not a misconfiguration — no warning for it. Only the
	// embedded-etcd scaling caveat is worth surfacing when the
	// operator has opted into clustering.
	if c.Cluster.Enabled && c.Cluster.Etcd.Mode == clusterModeEmbedded {
		w = append(w, "embedded etcd is fine for ≤3 members; use external etcd for 5+ member production clusters")
	}
	if c.Server.CORS.Enabled {
		for _, o := range c.Server.CORS.AllowedOrigins {
			if o == "*" {
				w = append(w, "CORS allows all origins (*) in production")
				break
			}
		}
	}
	if open := c.GitOps.UnauthenticatedWebhookSources(); len(open) > 0 {
		sort.Strings(open)
		w = append(w, fmt.Sprintf("gitops webhook receiver enabled with unauthenticated sources %v in production (set gitops.webhook.sources.<provider>.method to hmac or bearer)", open))
	}
	// Epic 19 task 8 — surface dev-mode-only credential knobs that
	// silently work but should never ship.
	// Phase B5 finding C1: an empty HMACSecret silently disables
	// command-signature verification on every agent (see
	// internal/agent/security.go::checkHMAC). Bootstrap UX needs the
	// empty-default to work; production deployments must not.
	if c.Security.HMACSecret == "" {
		w = append(w, "security.hmacsecret is EMPTY in production — agent command-signature verification is disabled; any caller able to publish to the agent's NATS subject can execute commands. Set hmacsecret (rotate out-of-band in production) before going live.")
	} else {
		w = append(w, "security.hmacsecret is set to a static value (rotate via an out-of-band secret manager in production; in v1.x this graduates to KMS-backed rotation)")
	}
	if c.Secrets.Enabled {
		for _, b := range c.Secrets.Backends {
			if b.Type == SecretsBackendTypeFile && b.File != nil &&
				strings.HasPrefix(b.File.MasterKey, "inline:") {
				w = append(w, fmt.Sprintf("secrets.backends[%q].file.master_key uses inline:hex in production (use env: or file: to keep the key out of the config file)", b.Name))
			}
		}
	}
	if c.NATS.Bootstrap.Enabled {
		w = append(w, "nats.bootstrap.enabled uses static PSK material in production (graduate to identity-issued join tokens via identity.enabled: true)")
	}
	return w
}
