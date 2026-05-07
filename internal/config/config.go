// Package config defines the runtime configuration for all Keystone Core
// binaries. It loads from YAML files plus KSCORE_-prefixed env vars,
// validates after unmarshal, and emits production warnings for risky
// combinations.
package config

import (
	"fmt"
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
	Mode    Mode          `koanf:"mode"`
	Server  ServerConfig  `koanf:"server"`
	Logging LoggingConfig `koanf:"logging"`
	Storage StorageConfig `koanf:"storage"`
	Health  HealthConfig  `koanf:"health"`
	NATS    NATSConfig    `koanf:"nats"`
}

// defaultConfig returns the built-in defaults applied before YAML/env overlays.
func defaultConfig() *Config {
	return &Config{
		Mode: ModeDevelopment,
		Server: ServerConfig{
			Host:     "0.0.0.0",
			GRPCPort: 9090,
			HTTPPort: 8080,
			TLS:      TLSConfig{Enabled: false},
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
			Mode:          NATSModeEmbedded,
			ClusterName:   "default",
			MaxReconnects: 60,
			ReconnectWait: 2 * time.Second,
			JetStream: JetStreamConfig{
				Enabled:    true,
				StoreDir:   "./data/jetstream",
				MaxStorage: 10 * 1024 * 1024 * 1024, // 10 GiB
			},
			Embedded: EmbeddedNATSConfig{
				Host:            "127.0.0.1",
				Port:            4222,
				MaxConnections:  0,
				EnableJetStream: true,
				MaxMemory:       0,
			},
			Dedup: DedupConfig{
				Enabled:         true,
				WindowDuration:  5 * time.Minute, // PROJECT-DETAILS §4.2 default
				MaxEntries:      100_000,
				CleanupInterval: 30 * time.Second,
			},
		},
	}
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
	if c.Server.CORS.Enabled {
		for _, o := range c.Server.CORS.AllowedOrigins {
			if o == "*" {
				w = append(w, "CORS allows all origins (*) in production")
				break
			}
		}
	}
	return w
}
