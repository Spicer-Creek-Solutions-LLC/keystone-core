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

// NATSConfig configures the v1.0 NATS transport.
//
// Endpoints (Task 2/3) supersedes URLs for richer config (priority,
// weight, tags). Both are accepted for ergonomics: simple deployments
// list URLs; HA deployments use Endpoints. They are mutually exclusive
// in external mode — populating both is rejected at Validate.
type NATSConfig struct {
	Mode          NATSMode         `koanf:"mode"`
	URLs          []string         `koanf:"urls"`
	Endpoints     []EndpointConfig `koanf:"endpoints"`
	Token         string           `koanf:"token"`
	Credential    string           `koanf:"credential"`
	MaxReconnects int              `koanf:"maxreconnects"`
	ReconnectWait time.Duration    `koanf:"reconnectwait"`
	ClusterName   string           `koanf:"clustername"`

	JetStream      JetStreamConfig      `koanf:"jetstream"`
	Embedded       EmbeddedNATSConfig   `koanf:"embedded"`
	Dedup          DedupConfig          `koanf:"dedup"`
	CircuitBreaker CircuitBreakerConfig `koanf:"circuitbreaker"`
}

// CircuitBreakerConfig configures the per-endpoint circuit breaker
// (Epic 05 task 7). State machine: closed → open → half-open →
// closed. PROJECT-DETAILS §4.2.
//
// v1.0 wiring: each endpoint owns one breaker. ConnectionManager
// drives transitions via the disconnect/reconnect callbacks already
// in place from task 2; Health degrades to error when every endpoint
// is OPEN. Active dial-time eviction (skipping OPEN endpoints when
// nats.go picks the next reconnect target) is deferred to v1.x —
// it requires replacing nats.go's native multi-URL failover with a
// per-endpoint dial loop, which is a substantial refactor for
// marginal v1.0 benefit.
type CircuitBreakerConfig struct {
	Enabled             bool          `koanf:"enabled"`
	FailureThreshold    int           `koanf:"failurethreshold"`
	SuccessThreshold    int           `koanf:"successthreshold"`
	OpenDuration        time.Duration `koanf:"openduration"`
	HalfOpenMaxAttempts int           `koanf:"halfopenmaxattempts"`
}

// DedupConfig configures producer-side message dedup (Epic 05 task
// 6). Defaults follow PROJECT-DETAILS §4.2: 5m window, large but
// bounded entry cap. Per-subject overrides let operators shrink the
// window for low-RTT subjects (heartbeats) or enlarge it for high-
// retry ones.
type DedupConfig struct {
	Enabled             bool              `koanf:"enabled"`
	WindowDuration      time.Duration     `koanf:"windowduration"`
	MaxEntries          int               `koanf:"maxentries"`
	CleanupInterval     time.Duration     `koanf:"cleanupinterval"`
	PerSubjectOverrides []SubjectOverride `koanf:"persubjectoverrides"`
}

// SubjectOverride applies a non-default WindowDuration to subjects
// matching Prefix (longest prefix wins; equality counts as a
// prefix match).
type SubjectOverride struct {
	Prefix         string        `koanf:"prefix"`
	WindowDuration time.Duration `koanf:"windowduration"`
}

// EndpointConfig is the structured form of a single NATS endpoint
// consumed by ConnectionManager. Priority orders the connect-attempt
// list (higher first). Weight is reserved for v1.3+ load distribution
// and ignored today. Tags are operator labels passed through to
// EndpointSnapshot for observability.
type EndpointConfig struct {
	URL      string   `koanf:"url"`
	Priority int      `koanf:"priority"`
	Weight   int      `koanf:"weight"`
	Tags     []string `koanf:"tags"`
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
		if len(n.Endpoints) != 0 {
			return fmt.Errorf("endpoints: must be empty when mode=embedded")
		}
	case NATSModeExternal:
		if len(n.URLs) == 0 && len(n.Endpoints) == 0 {
			return fmt.Errorf("urls/endpoints: at least one must be set when mode=external")
		}
		if len(n.URLs) > 0 && len(n.Endpoints) > 0 {
			return fmt.Errorf("urls/endpoints: mutually exclusive; pick one form")
		}
		for i, u := range n.URLs {
			if u == "" {
				return fmt.Errorf("urls[%d]: must not be empty", i)
			}
		}
		for i, e := range n.Endpoints {
			if err := e.Validate(); err != nil {
				return fmt.Errorf("endpoints[%d]: %w", i, err)
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
	if err := n.Dedup.Validate(); err != nil {
		return fmt.Errorf("dedup: %w", err)
	}
	if err := n.CircuitBreaker.Validate(); err != nil {
		return fmt.Errorf("circuitbreaker: %w", err)
	}
	if n.Mode == NATSModeEmbedded {
		if err := n.Embedded.Validate(); err != nil {
			return fmt.Errorf("embedded: %w", err)
		}
	}
	return nil
}

// Validate enforces positive thresholds and a positive OpenDuration
// when enabled. Disabled config skips checks since the values are
// unused.
func (c CircuitBreakerConfig) Validate() error {
	if !c.Enabled {
		return nil
	}
	if c.FailureThreshold <= 0 {
		return fmt.Errorf("failurethreshold: must be positive when enabled, got %d", c.FailureThreshold)
	}
	if c.SuccessThreshold <= 0 {
		return fmt.Errorf("successthreshold: must be positive when enabled, got %d", c.SuccessThreshold)
	}
	if c.OpenDuration <= 0 {
		return fmt.Errorf("openduration: must be positive when enabled, got %s", c.OpenDuration)
	}
	if c.HalfOpenMaxAttempts <= 0 {
		return fmt.Errorf("halfopenmaxattempts: must be positive when enabled, got %d", c.HalfOpenMaxAttempts)
	}
	if c.HalfOpenMaxAttempts < c.SuccessThreshold {
		return fmt.Errorf("halfopenmaxattempts (%d) must be >= successthreshold (%d) so the breaker can close",
			c.HalfOpenMaxAttempts, c.SuccessThreshold)
	}
	return nil
}

// Validate enforces non-negative durations, a positive MaxEntries
// when enabled, and well-formed per-subject overrides. Disabled
// dedup skips structural checks since the values won't be used.
func (d DedupConfig) Validate() error {
	if !d.Enabled {
		return nil
	}
	if d.WindowDuration <= 0 {
		return fmt.Errorf("windowduration: must be positive when enabled, got %s", d.WindowDuration)
	}
	if d.MaxEntries <= 0 {
		return fmt.Errorf("maxentries: must be positive when enabled, got %d", d.MaxEntries)
	}
	if d.CleanupInterval <= 0 {
		return fmt.Errorf("cleanupinterval: must be positive when enabled, got %s", d.CleanupInterval)
	}
	for i, o := range d.PerSubjectOverrides {
		if o.Prefix == "" {
			return fmt.Errorf("persubjectoverrides[%d].prefix: must not be empty", i)
		}
		if o.WindowDuration <= 0 {
			return fmt.Errorf("persubjectoverrides[%d].windowduration: must be positive, got %s", i, o.WindowDuration)
		}
	}
	return nil
}

// Validate rejects empty URLs and negative weights. Priority can be
// any int (higher wins); weight 0 is allowed and treated as "use
// default" by ConnectionManager.
func (e EndpointConfig) Validate() error {
	if e.URL == "" {
		return fmt.Errorf("url: must not be empty")
	}
	if e.Weight < 0 {
		return fmt.Errorf("weight: must not be negative, got %d", e.Weight)
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
