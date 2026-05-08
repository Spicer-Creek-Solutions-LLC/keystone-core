package config

import (
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
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
	Bootstrap      BootstrapConfig      `koanf:"bootstrap"`
}

// BootstrapConfig governs the v1.0 server-side bootstrap registration
// handler (Epic 05 task 9). Defaults to disabled — operators opt in
// by setting Enabled=true and populating PSKs. A "default on with no
// PSKs" stance would be a footgun (looks permissive, actually
// rejects everything); explicit opt-in surfaces the dependency.
//
// PSKs are config-listed in v1.0; consumed PSKs are tracked in-
// memory so a server restart wipes the consumption record.
// Operators are expected to rotate PSKs (issue fresh ones per
// agent, don't reuse). v1.x will persist consumption in the state
// DB and add an API endpoint to issue PSKs.
type BootstrapConfig struct {
	Enabled bool           `koanf:"enabled"`
	PSKs    []BootstrapPSK `koanf:"psks"`
}

// BootstrapPSK describes one operator-issued bootstrap credential.
// Secret is hex-encoded so config files stay copy-pasteable; the
// validator hex-decodes once at construction. ExpiresAt is the
// hard cutoff after which Validate rejects the PSK.
type BootstrapPSK struct {
	AgentID   string    `koanf:"agentid"`
	Secret    string    `koanf:"secret"` //nolint:gosec // PSK hex string from config — flagged false-positive on field-name pattern
	ExpiresAt time.Time `koanf:"expiresat"`
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

// JetStreamConfig governs JetStream enablement and per-stream
// defaults. v1.0 streams (commands + events) inherit StreamMaxAge,
// StreamMaxBytes, StreamMaxMsgs, and StreamReplicas; per-stream
// overrides are reserved for v1.x.
//
// Enabled is the single switch — when true, the embedded server (if
// running) starts with JetStream and Manager auto-creates the
// commands/events streams. When false, no streams are touched and
// the embedded server runs without JetStream.
type JetStreamConfig struct {
	Enabled        bool          `koanf:"enabled"`
	StoreDir       string        `koanf:"storedir"`
	MaxStorage     int64         `koanf:"maxstorage"`
	StreamMaxAge   time.Duration `koanf:"streammaxage"`
	StreamMaxBytes int64         `koanf:"streammaxbytes"`
	StreamMaxMsgs  int64         `koanf:"streammaxmsgs"`
	StreamReplicas int           `koanf:"streamreplicas"`
}

// EmbeddedNATSConfig configures the in-process nats-server/v2 instance
// started when Mode == NATSModeEmbedded. JetStream is gated by
// JetStreamConfig.Enabled — there is no separate embedded toggle so
// the two flags can never disagree.
type EmbeddedNATSConfig struct {
	Host           string `koanf:"host"`
	Port           int    `koanf:"port"`
	MaxConnections int    `koanf:"maxconnections"`
	MaxMemory      int64  `koanf:"maxmemory"`
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
			if err := validateExternalURL(u); err != nil {
				return fmt.Errorf("urls[%d]: %w", i, err)
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
	if err := n.Bootstrap.Validate(); err != nil {
		return fmt.Errorf("bootstrap: %w", err)
	}
	if n.Mode == NATSModeEmbedded {
		if err := n.Embedded.Validate(); err != nil {
			return fmt.Errorf("embedded: %w", err)
		}
	}
	return nil
}

// Validate accepts disabled config without checking PSK structure
// (operators may want to populate PSKs first then flip Enabled
// later). When enabled, every PSK must have a non-empty AgentID, a
// hex-decodable Secret, and a non-zero ExpiresAt.
func (b BootstrapConfig) Validate() error {
	if !b.Enabled {
		return nil
	}
	seen := map[string]struct{}{}
	for i, psk := range b.PSKs {
		if psk.AgentID == "" {
			return fmt.Errorf("psks[%d].agentid: must not be empty", i)
		}
		if _, dup := seen[psk.AgentID]; dup {
			return fmt.Errorf("psks[%d].agentid: duplicate %q", i, psk.AgentID)
		}
		seen[psk.AgentID] = struct{}{}
		if psk.Secret == "" {
			return fmt.Errorf("psks[%d].secret: must not be empty", i)
		}
		if _, err := hex.DecodeString(psk.Secret); err != nil {
			return fmt.Errorf("psks[%d].secret: not hex: %w", i, err)
		}
		if psk.ExpiresAt.IsZero() {
			return fmt.Errorf("psks[%d].expiresat: must not be zero", i)
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

// Validate rejects empty URLs, malformed bracketing, and negative
// weights. Priority can be any int (higher wins); weight 0 is
// allowed and treated as "use default" by ConnectionManager.
func (e EndpointConfig) Validate() error {
	if e.URL == "" {
		return fmt.Errorf("url: must not be empty")
	}
	if err := validateExternalURL(e.URL); err != nil {
		return fmt.Errorf("url: %w", err)
	}
	if e.Weight < 0 {
		return fmt.Errorf("weight: must not be negative, got %d", e.Weight)
	}
	return nil
}

// validateExternalURL rejects two common operator errors:
//
//  1. Unbracketed IPv6 addresses (`nats://::1:4222`) — ambiguous
//     because the trailing :4222 is indistinguishable from a port
//     when not bracketed. PROJECT-DETAILS §4.2 calls this out
//     explicitly: "[::]:4222, not :4222 or ::4222."
//  2. Garbage that url.Parse cannot decode at all.
//
// The IPv6 check runs *before* url.Parse because url.Parse rejects
// "nats://::1:4222" with a generic "invalid port" error that
// doesn't tell the operator what to do; surfacing the bracket fix
// inline is the whole point.
//
// We intentionally do NOT validate the scheme (nats / tls / etc.)
// here — StrategySelector dispatches on scheme, and unknown schemes
// fall back to Direct. Operators get a clean nats-level error if
// the scheme is wrong.
func validateExternalURL(raw string) error {
	// Extract the host portion manually for the IPv6 check.
	afterScheme := raw
	if i := strings.Index(raw, "://"); i >= 0 {
		afterScheme = raw[i+3:]
	}
	if i := strings.IndexAny(afterScheme, "/?#"); i >= 0 {
		afterScheme = afterScheme[:i]
	}
	// Hostnames / IPv4 have at most one colon (the port separator).
	// Anything with two or more colons and no opening bracket is an
	// IPv6 address typed without brackets — the §4.2 footgun.
	if !strings.HasPrefix(afterScheme, "[") && strings.Count(afterScheme, ":") > 1 {
		return fmt.Errorf("unbracketed IPv6 address %q (use nats://[::1]:port — see PROJECT-DETAILS §4.2)", raw)
	}

	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("not a valid URL %q: %w", raw, err)
	}
	if u.Host == "" {
		return fmt.Errorf("missing host: %q", raw)
	}
	return nil
}

// Validate enforces non-negative storage and a non-empty store dir
// whenever JetStream is enabled, plus positive per-stream defaults
// (Task 8). The directory is created on Manager start; missing-dir-
// on-disk is not an error here.
func (j JetStreamConfig) Validate() error {
	if j.MaxStorage < 0 {
		return fmt.Errorf("maxstorage: must not be negative, got %d", j.MaxStorage)
	}
	if !j.Enabled {
		return nil
	}
	if j.StoreDir == "" {
		return fmt.Errorf("storedir: must not be empty when enabled=true")
	}
	if j.StreamMaxAge <= 0 {
		return fmt.Errorf("streammaxage: must be positive when enabled, got %s", j.StreamMaxAge)
	}
	if j.StreamMaxBytes <= 0 {
		return fmt.Errorf("streammaxbytes: must be positive when enabled, got %d", j.StreamMaxBytes)
	}
	if j.StreamMaxMsgs <= 0 {
		return fmt.Errorf("streammaxmsgs: must be positive when enabled, got %d", j.StreamMaxMsgs)
	}
	if j.StreamReplicas < 1 || j.StreamReplicas > 5 {
		return fmt.Errorf("streamreplicas: must be in [1,5], got %d", j.StreamReplicas)
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

// JetStreamSafeName returns name with characters that are illegal in
// JetStream stream names replaced with underscores. NATS allows
// [A-Za-z0-9_-]; cluster names already pass the SubjectBuilder
// validator (alphanum + dash + underscore), so this is a defensive
// pass-through with explicit intent.
func JetStreamSafeName(name string) string {
	out := make([]byte, len(name))
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'A' && c <= 'Z',
			c >= 'a' && c <= 'z',
			c >= '0' && c <= '9',
			c == '_', c == '-':
			out[i] = c
		default:
			out[i] = '_'
		}
	}
	return string(out)
}
