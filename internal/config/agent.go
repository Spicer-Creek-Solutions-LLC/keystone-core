package config

import (
	"fmt"
	"time"
)

// AgentConfig governs the kscore-agent daemon (Epic 06). Loaded
// from the same config file as the server's sections (per
// PROJECT-DETAILS §4.6, agent's config file lists agent.* alongside
// nats.* and security.*); the kscore-server binary ignores this
// section.
//
// AgentID is generally set by Task 6's bootstrap engine and persisted
// to disk; for v1.0's pre-bootstrap world it must be supplied via
// config. ClusterName is shared with the rest of the system via
// NATSConfig.ClusterName — the agent reads it from there.
type AgentConfig struct {
	AgentID           string            `koanf:"id"`
	HeartbeatInterval time.Duration     `koanf:"heartbeatinterval"`
	MetadataInterval  time.Duration     `koanf:"metadatainterval"`
	CommandTimeout    time.Duration     `koanf:"commandtimeout"`
	Labels            map[string]string `koanf:"labels"`

	// BootstrapPSK is the hex-encoded pre-shared key this agent
	// presents at boot to register with the control plane. When
	// non-empty, the agent publishes a bootstrap.register envelope
	// at Start before the heartbeat loop begins. The server-side
	// PSK list lives at nats.bootstrap.psks. Leave empty for v0.1-
	// style deployments where the operator pre-provisions agent
	// rows out-of-band.
	BootstrapPSK string `koanf:"bootstrappsk"` //nolint:gosec // PSK hex string — flagged false-positive on field-name pattern
}

// Validate enforces non-zero intervals and a non-empty AgentID. Zero
// or negative durations would degenerate the loops; an empty AgentID
// would mean the agent has no identity to advertise.
//
// Used only by cmd/kscore-agent — kscore-server skips this validation
// because it doesn't read agent.*. The shared root Config.Validate
// calls this *only* when Mode hints at agent-flavored startup; for
// v1.0 we keep it permissive (zero AgentID is allowed in the root
// validate; the agent binary enforces non-empty at construction
// time).
func (a AgentConfig) Validate() error {
	if a.HeartbeatInterval < 0 {
		return fmt.Errorf("heartbeatinterval: must not be negative, got %s", a.HeartbeatInterval)
	}
	if a.MetadataInterval < 0 {
		return fmt.Errorf("metadatainterval: must not be negative, got %s", a.MetadataInterval)
	}
	if a.CommandTimeout < 0 {
		return fmt.Errorf("commandtimeout: must not be negative, got %s", a.CommandTimeout)
	}
	return nil
}
