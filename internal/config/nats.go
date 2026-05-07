package config

import (
	"fmt"
	"time"
)

// NATSMode picks between an in-process embedded NATS server and a
// connection to an external cluster. PROJECT-DETAILS §4.2 lists these
// as the two v1.0 deployment modes.
type NATSMode string

const (
	NATSModeEmbedded NATSMode = "embedded"
	NATSModeExternal NATSMode = "external"
)

// NATSConfig configures the v1.0 NATS transport. Multi-endpoint
// failover, circuit breakers, and bootstrap registration are wired
// in later Epic 05 tasks; this struct only carries fields needed to
// stand up a single embedded server or open a single client connection.
type NATSConfig struct {
	Mode          NATSMode      `koanf:"mode"`
	URLs          []string      `koanf:"urls"`
	Token         string        `koanf:"token"`
	Credential    string        `koanf:"credential"`
	MaxReconnects int           `koanf:"maxreconnects"`
	ReconnectWait time.Duration `koanf:"reconnectwait"`
	ClusterName   string        `koanf:"clustername"`

	JetStream JetStreamConfig    `koanf:"jetstream"`
	Embedded  EmbeddedNATSConfig `koanf:"embedded"`
}

// JetStreamConfig governs JetStream enablement on the embedded server
// and (later, Task 8) the stream definitions used by external mode.
type JetStreamConfig struct {
	Enabled    bool   `koanf:"enabled"`
	StoreDir   string `koanf:"storedir"`
	MaxStorage int64  `koanf:"maxstorage"`
}

// EmbeddedNATSConfig configures the in-process nats-server/v2 instance
// started when Mode == NATSModeEmbedded.
type EmbeddedNATSConfig struct {
	Host            string `koanf:"host"`
	Port            int    `koanf:"port"`
	MaxConnections  int    `koanf:"maxconnections"`
	EnableJetStream bool   `koanf:"enablejetstream"`
	MaxMemory       int64  `koanf:"maxmemory"`
}

// Validate returns an error if any NATS field is invalid. Mode/URLs
// are mutually constrained: embedded must not list URLs, external must.
func (n NATSConfig) Validate() error {
	switch n.Mode {
	case NATSModeEmbedded:
		if len(n.URLs) != 0 {
			return fmt.Errorf("urls: must be empty when mode=embedded")
		}
	case NATSModeExternal:
		if len(n.URLs) == 0 {
			return fmt.Errorf("urls: must be non-empty when mode=external")
		}
		for i, u := range n.URLs {
			if u == "" {
				return fmt.Errorf("urls[%d]: must not be empty", i)
			}
		}
	default:
		return fmt.Errorf("mode: %q (must be embedded or external)", string(n.Mode))
	}

	if n.ClusterName == "" {
		return fmt.Errorf("clustername: must not be empty")
	}
	if n.MaxReconnects < -1 {
		return fmt.Errorf("maxreconnects: %d (must be >= -1; -1 = infinite)", n.MaxReconnects)
	}
	if n.ReconnectWait < 0 {
		return fmt.Errorf("reconnectwait: must not be negative, got %s", n.ReconnectWait)
	}

	if err := n.JetStream.Validate(); err != nil {
		return fmt.Errorf("jetstream: %w", err)
	}
	if n.Mode == NATSModeEmbedded {
		if err := n.Embedded.Validate(); err != nil {
			return fmt.Errorf("embedded: %w", err)
		}
	}
	return nil
}

// Validate enforces non-negative storage and a non-empty store dir
// whenever JetStream is enabled. The directory is created on Manager
// start; missing-dir-on-disk is not an error here.
func (j JetStreamConfig) Validate() error {
	if j.MaxStorage < 0 {
		return fmt.Errorf("maxstorage: must not be negative, got %d", j.MaxStorage)
	}
	if j.Enabled && j.StoreDir == "" {
		return fmt.Errorf("storedir: must not be empty when enabled=true")
	}
	return nil
}

// Validate enforces port range and non-negative resource limits for
// the embedded server. Zero MaxConnections / MaxMemory mean "no limit"
// matching nats-server semantics.
func (e EmbeddedNATSConfig) Validate() error {
	if e.Host == "" {
		return fmt.Errorf("host: must not be empty")
	}
	if e.Port < 1 || e.Port > 65535 {
		return fmt.Errorf("port: %d out of range [1,65535]", e.Port)
	}
	if e.MaxConnections < 0 {
		return fmt.Errorf("maxconnections: must not be negative, got %d", e.MaxConnections)
	}
	if e.MaxMemory < 0 {
		return fmt.Errorf("maxmemory: must not be negative, got %d", e.MaxMemory)
	}
	return nil
}
