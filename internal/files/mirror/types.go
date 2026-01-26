// Package mirror implements mirror groups and geographic routing for file distribution.
package mirror

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// MirrorState represents the health state of a mirror.
type MirrorState string

const (
	MirrorStateUnknown   MirrorState = "unknown"
	MirrorStateHealthy   MirrorState = "healthy"
	MirrorStateDegraded  MirrorState = "degraded"
	MirrorStateUnhealthy MirrorState = "unhealthy"
)

// ReadStrategy determines how reads are routed to mirrors.
type ReadStrategy string

const (
	// ReadStrategyNearest routes to the mirror with lowest latency.
	ReadStrategyNearest ReadStrategy = "nearest"

	// ReadStrategyRoundRobin distributes reads evenly across mirrors.
	ReadStrategyRoundRobin ReadStrategy = "round-robin"

	// ReadStrategyFailover routes to mirrors in priority order.
	ReadStrategyFailover ReadStrategy = "failover"

	// ReadStrategyFastest routes based on recent response times.
	ReadStrategyFastest ReadStrategy = "fastest"
)

// WritePolicy determines how writes are distributed to mirrors.
type WritePolicy string

const (
	// WritePolicyAll writes to all mirrors synchronously.
	WritePolicyAll WritePolicy = "all"

	// WritePolicyQuorum writes to a quorum of mirrors.
	WritePolicyQuorum WritePolicy = "quorum"

	// WritePolicyPrimaryOnly writes only to primary, async replication to others.
	WritePolicyPrimaryOnly WritePolicy = "primary-only"

	// WritePolicyPrimarySecondary writes sync to primary + one secondary.
	WritePolicyPrimarySecondary WritePolicy = "primary-secondary"
)

// Mirror represents a single file server instance in a mirror group.
type Mirror struct {
	// ID uniquely identifies this mirror.
	ID string `json:"id" yaml:"id"`

	// Name is a human-readable name.
	Name string `json:"name" yaml:"name"`

	// ClusterID is the cluster this mirror belongs to.
	ClusterID string `json:"cluster_id" yaml:"cluster_id"`

	// InstanceID is the specific server instance.
	InstanceID string `json:"instance_id,omitempty" yaml:"instance_id,omitempty"`

	// Priority for failover routing (lower = higher priority).
	Priority int `json:"priority" yaml:"priority"`

	// Weight for round-robin distribution.
	Weight int `json:"weight" yaml:"weight"`

	// Location is the geographic location of this mirror.
	Location *Location `json:"location,omitempty" yaml:"location,omitempty"`

	// Tags for filtering and routing.
	Tags map[string]string `json:"tags,omitempty" yaml:"tags,omitempty"`

	// Enabled indicates if this mirror is active.
	Enabled bool `json:"enabled" yaml:"enabled"`

	// ReadOnly indicates this mirror only serves reads.
	ReadOnly bool `json:"read_only" yaml:"read_only"`

	// IsPrimary indicates this is the primary mirror in the group.
	IsPrimary bool `json:"is_primary" yaml:"is_primary"`
}

// Location represents a geographic location.
type Location struct {
	// Region is a broad geographic area (e.g., "us-east", "eu-west").
	Region string `json:"region,omitempty" yaml:"region,omitempty"`

	// Zone is a more specific location within a region.
	Zone string `json:"zone,omitempty" yaml:"zone,omitempty"`

	// Datacenter is a specific datacenter identifier.
	Datacenter string `json:"datacenter,omitempty" yaml:"datacenter,omitempty"`

	// Latitude for coordinate-based routing.
	Latitude float64 `json:"latitude,omitempty" yaml:"latitude,omitempty"`

	// Longitude for coordinate-based routing.
	Longitude float64 `json:"longitude,omitempty" yaml:"longitude,omitempty"`

	// Country code (ISO 3166-1 alpha-2).
	Country string `json:"country,omitempty" yaml:"country,omitempty"`
}

// MirrorGroupConfig configures a mirror group.
type MirrorGroupConfig struct {
	// ID uniquely identifies this mirror group.
	ID string `json:"id" yaml:"id"`

	// Name is a human-readable name.
	Name string `json:"name" yaml:"name"`

	// Description provides details about this group.
	Description string `json:"description,omitempty" yaml:"description,omitempty"`

	// Mirrors in this group.
	Mirrors []*Mirror `json:"mirrors" yaml:"mirrors"`

	// ReadStrategy determines how reads are routed.
	ReadStrategy ReadStrategy `json:"read_strategy" yaml:"read_strategy"`

	// WritePolicy determines how writes are distributed.
	WritePolicy WritePolicy `json:"write_policy" yaml:"write_policy"`

	// QuorumSize for quorum write policy.
	QuorumSize int `json:"quorum_size,omitempty" yaml:"quorum_size,omitempty"`

	// PathPrefixes that this group handles.
	PathPrefixes []string `json:"path_prefixes,omitempty" yaml:"path_prefixes,omitempty"`

	// Namespaces that this group handles.
	Namespaces []string `json:"namespaces,omitempty" yaml:"namespaces,omitempty"`

	// HealthCheck configuration.
	HealthCheck *HealthCheckConfig `json:"health_check,omitempty" yaml:"health_check,omitempty"`

	// LatencyProbe configuration.
	LatencyProbe *LatencyProbeConfig `json:"latency_probe,omitempty" yaml:"latency_probe,omitempty"`
}

// HealthCheckConfig configures health checking for mirrors.
type HealthCheckConfig struct {
	// Interval between health checks.
	Interval time.Duration `json:"interval" yaml:"interval"`

	// Timeout for each health check.
	Timeout time.Duration `json:"timeout" yaml:"timeout"`

	// UnhealthyThreshold is failures before marking unhealthy.
	UnhealthyThreshold int `json:"unhealthy_threshold" yaml:"unhealthy_threshold"`

	// HealthyThreshold is successes before marking healthy.
	HealthyThreshold int `json:"healthy_threshold" yaml:"healthy_threshold"`
}

// LatencyProbeConfig configures latency probing.
type LatencyProbeConfig struct {
	// Interval between probes.
	Interval time.Duration `json:"interval" yaml:"interval"`

	// ProbeFile is the small file to fetch for probing.
	ProbeFile string `json:"probe_file,omitempty" yaml:"probe_file,omitempty"`

	// SmoothingFactor for exponential moving average (0-1).
	SmoothingFactor float64 `json:"smoothing_factor" yaml:"smoothing_factor"`
}

// MirrorHealth tracks the health status of a mirror.
type MirrorHealth struct {
	MirrorID         string        `json:"mirror_id"`
	State            MirrorState   `json:"state"`
	LastCheck        time.Time     `json:"last_check"`
	LastSuccess      time.Time     `json:"last_success,omitempty"`
	LastFailure      time.Time     `json:"last_failure,omitempty"`
	LastError        string        `json:"last_error,omitempty"`
	ConsecutiveFails int           `json:"consecutive_fails"`
	Latency          time.Duration `json:"latency,omitempty"`
	AvgLatency       time.Duration `json:"avg_latency,omitempty"`
}

// MirrorStats contains statistics for a mirror.
type MirrorStats struct {
	MirrorID     string        `json:"mirror_id"`
	ReadCount    int64         `json:"read_count"`
	WriteCount   int64         `json:"write_count"`
	ReadBytes    int64         `json:"read_bytes"`
	WriteBytes   int64         `json:"write_bytes"`
	ErrorCount   int64         `json:"error_count"`
	AvgLatency   time.Duration `json:"avg_latency"`
	P95Latency   time.Duration `json:"p95_latency"`
	P99Latency   time.Duration `json:"p99_latency"`
	LastActivity time.Time     `json:"last_activity"`
}

// MirrorGroup represents a group of file servers acting as mirrors.
type MirrorGroup struct {
	config  *MirrorGroupConfig
	mirrors map[string]*Mirror
	health  map[string]*MirrorHealth
	stats   map[string]*MirrorStats
	mu      sync.RWMutex
}

// NewMirrorGroup creates a new mirror group from configuration.
func NewMirrorGroup(config *MirrorGroupConfig) (*MirrorGroup, error) {
	if config.ID == "" {
		return nil, fmt.Errorf("mirror group ID is required")
	}
	if len(config.Mirrors) == 0 {
		return nil, fmt.Errorf("mirror group must have at least one mirror")
	}

	// Validate mirrors
	mirrors := make(map[string]*Mirror)
	for _, m := range config.Mirrors {
		if m.ID == "" {
			return nil, fmt.Errorf("mirror ID is required")
		}
		if _, exists := mirrors[m.ID]; exists {
			return nil, fmt.Errorf("duplicate mirror ID: %s", m.ID)
		}
		if m.ClusterID == "" {
			return nil, fmt.Errorf("mirror %s: cluster_id is required", m.ID)
		}
		// Set defaults
		if m.Weight <= 0 {
			m.Weight = 1
		}
		mirrors[m.ID] = m
	}

	// Validate read strategy
	switch config.ReadStrategy {
	case ReadStrategyNearest, ReadStrategyRoundRobin, ReadStrategyFailover, ReadStrategyFastest:
		// Valid
	case "":
		config.ReadStrategy = ReadStrategyFailover
	default:
		return nil, fmt.Errorf("invalid read strategy: %s", config.ReadStrategy)
	}

	// Validate write policy
	switch config.WritePolicy {
	case WritePolicyAll, WritePolicyQuorum, WritePolicyPrimaryOnly, WritePolicyPrimarySecondary:
		// Valid
	case "":
		config.WritePolicy = WritePolicyAll
	default:
		return nil, fmt.Errorf("invalid write policy: %s", config.WritePolicy)
	}

	// Validate quorum size
	if config.WritePolicy == WritePolicyQuorum {
		if config.QuorumSize <= 0 {
			config.QuorumSize = (len(config.Mirrors) / 2) + 1
		}
		if config.QuorumSize > len(config.Mirrors) {
			return nil, fmt.Errorf("quorum size %d exceeds mirror count %d", config.QuorumSize, len(config.Mirrors))
		}
	}

	// Set default health check config
	if config.HealthCheck == nil {
		config.HealthCheck = &HealthCheckConfig{
			Interval:           30 * time.Second,
			Timeout:            5 * time.Second,
			UnhealthyThreshold: 3,
			HealthyThreshold:   2,
		}
	}

	// Set default latency probe config
	if config.LatencyProbe == nil {
		config.LatencyProbe = &LatencyProbeConfig{
			Interval:        60 * time.Second,
			SmoothingFactor: 0.3,
		}
	}

	// Initialize health and stats
	health := make(map[string]*MirrorHealth)
	stats := make(map[string]*MirrorStats)
	for id := range mirrors {
		health[id] = &MirrorHealth{
			MirrorID: id,
			State:    MirrorStateUnknown,
		}
		stats[id] = &MirrorStats{
			MirrorID: id,
		}
	}

	return &MirrorGroup{
		config:  config,
		mirrors: mirrors,
		health:  health,
		stats:   stats,
	}, nil
}

// ID returns the mirror group ID.
func (g *MirrorGroup) ID() string {
	return g.config.ID
}

// Name returns the mirror group name.
func (g *MirrorGroup) Name() string {
	return g.config.Name
}

// Config returns the mirror group configuration.
func (g *MirrorGroup) Config() *MirrorGroupConfig {
	return g.config
}

// GetMirror returns a mirror by ID.
func (g *MirrorGroup) GetMirror(id string) (*Mirror, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	m, ok := g.mirrors[id]
	return m, ok
}

// GetMirrors returns all mirrors in the group.
func (g *MirrorGroup) GetMirrors() []*Mirror {
	g.mu.RLock()
	defer g.mu.RUnlock()
	mirrors := make([]*Mirror, 0, len(g.mirrors))
	for _, m := range g.mirrors {
		mirrors = append(mirrors, m)
	}
	return mirrors
}

// GetHealthyMirrors returns all healthy mirrors.
func (g *MirrorGroup) GetHealthyMirrors() []*Mirror {
	g.mu.RLock()
	defer g.mu.RUnlock()
	mirrors := make([]*Mirror, 0)
	for id, m := range g.mirrors {
		if !m.Enabled {
			continue
		}
		h := g.health[id]
		if h.State == MirrorStateHealthy || h.State == MirrorStateUnknown {
			mirrors = append(mirrors, m)
		}
	}
	return mirrors
}

// GetHealth returns the health status of a mirror.
func (g *MirrorGroup) GetHealth(mirrorID string) (*MirrorHealth, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	h, ok := g.health[mirrorID]
	return h, ok
}

// UpdateHealth updates the health status of a mirror.
func (g *MirrorGroup) UpdateHealth(mirrorID string, state MirrorState, latency time.Duration, err error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	h, ok := g.health[mirrorID]
	if !ok {
		return
	}

	h.LastCheck = time.Now()
	h.Latency = latency

	// If explicit state is provided, use it
	if state != "" {
		h.State = state
		if state == MirrorStateUnhealthy || state == MirrorStateDegraded {
			h.LastFailure = time.Now()
			if err != nil {
				h.LastError = err.Error()
			}
			h.ConsecutiveFails++
		} else if state == MirrorStateHealthy {
			h.LastSuccess = time.Now()
			h.LastError = ""
			h.ConsecutiveFails = 0
			// Update average latency with exponential moving average
			if h.AvgLatency == 0 {
				h.AvgLatency = latency
			} else {
				alpha := g.config.LatencyProbe.SmoothingFactor
				h.AvgLatency = time.Duration(float64(h.AvgLatency)*(1-alpha) + float64(latency)*alpha)
			}
		}
		return
	}

	// Infer state from error
	if err != nil {
		h.LastFailure = time.Now()
		h.LastError = err.Error()
		h.ConsecutiveFails++
		if h.ConsecutiveFails >= g.config.HealthCheck.UnhealthyThreshold {
			h.State = MirrorStateUnhealthy
		} else if h.State == MirrorStateHealthy {
			h.State = MirrorStateDegraded
		}
	} else {
		h.LastSuccess = time.Now()
		h.LastError = ""
		h.ConsecutiveFails = 0
		h.State = MirrorStateHealthy

		// Update average latency with exponential moving average
		if h.AvgLatency == 0 {
			h.AvgLatency = latency
		} else {
			alpha := g.config.LatencyProbe.SmoothingFactor
			h.AvgLatency = time.Duration(float64(h.AvgLatency)*(1-alpha) + float64(latency)*alpha)
		}
	}
}

// GetStats returns the statistics for a mirror.
func (g *MirrorGroup) GetStats(mirrorID string) (*MirrorStats, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	s, ok := g.stats[mirrorID]
	return s, ok
}

// RecordRead records a read operation to a mirror.
func (g *MirrorGroup) RecordRead(mirrorID string, bytes int64, latency time.Duration, err error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	s, ok := g.stats[mirrorID]
	if !ok {
		return
	}

	s.ReadCount++
	s.ReadBytes += bytes
	s.LastActivity = time.Now()

	if err != nil {
		s.ErrorCount++
	}
}

// RecordWrite records a write operation to a mirror.
func (g *MirrorGroup) RecordWrite(mirrorID string, bytes int64, latency time.Duration, err error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	s, ok := g.stats[mirrorID]
	if !ok {
		return
	}

	s.WriteCount++
	s.WriteBytes += bytes
	s.LastActivity = time.Now()

	if err != nil {
		s.ErrorCount++
	}
}

// MatchesPath checks if this group handles the given path.
func (g *MirrorGroup) MatchesPath(path string) bool {
	if len(g.config.PathPrefixes) == 0 {
		return true // Handle all paths
	}
	for _, prefix := range g.config.PathPrefixes {
		if len(path) >= len(prefix) && path[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

// MatchesNamespace checks if this group handles the given namespace.
func (g *MirrorGroup) MatchesNamespace(namespace string) bool {
	if len(g.config.Namespaces) == 0 {
		return true // Handle all namespaces
	}
	for _, ns := range g.config.Namespaces {
		if ns == namespace || ns == "*" {
			return true
		}
	}
	return false
}

// MarshalJSON implements json.Marshaler.
func (g *MirrorGroup) MarshalJSON() ([]byte, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	type groupJSON struct {
		ID       string                   `json:"id"`
		Name     string                   `json:"name"`
		Config   *MirrorGroupConfig       `json:"config"`
		Mirrors  map[string]*Mirror       `json:"mirrors"`
		Health   map[string]*MirrorHealth `json:"health"`
		Stats    map[string]*MirrorStats  `json:"stats"`
	}

	return json.Marshal(groupJSON{
		ID:      g.config.ID,
		Name:    g.config.Name,
		Config:  g.config,
		Mirrors: g.mirrors,
		Health:  g.health,
		Stats:   g.stats,
	})
}
