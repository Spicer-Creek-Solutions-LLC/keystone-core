package config

import (
	"fmt"
	"time"
)

// ClusterConfig drives the Epic 13 clustering/HA boot in
// kscore-server (PROJECT-DETAILS §4.15).
//
//	cluster:
//	  enabled: false
//	  etcd:
//	    mode: embedded            # embedded | external
//	    name: kscore-1
//	    data_dir: ./data/etcd
//	    client_urls: ["http://127.0.0.1:2379"]
//	    peer_urls:   ["http://127.0.0.1:2380"]
//	    endpoints:   []           # external mode only
//	    lease_ttl_seconds: 15
//	    dial_timeout: 5s
//	    auto_sync_interval: 5m
//	  membership:
//	    heartbeat_interval: 5s
//	    key_prefix: /kscore/cluster
//
// Enabled defaults to false: the single-node path stays the default
// and clustering is strictly opt-in. When disabled, no etcd is
// started and the ClusterService/CoordinationService are not
// registered (later Epic 13 tasks). The wiring that translates this
// into a running internal/cluster.EtcdClient lands with a later
// Epic 13 task; Task 1 ships the config surface + validation.
type ClusterConfig struct {
	Enabled    bool                    `koanf:"enabled"`
	Etcd       ClusterEtcdConfig       `koanf:"etcd"`
	Membership ClusterMembershipConfig `koanf:"membership"`
}

// ClusterMembershipConfig is the operator-facing membership config
// (Epic 13 task 2). The runtime equivalent is
// cluster.MembershipConfig; boot wiring (later task) maps onto it.
type ClusterMembershipConfig struct {
	// HeartbeatInterval is how often a member refreshes its
	// observable liveness. §4.15 default 5s.
	HeartbeatInterval time.Duration `koanf:"heartbeat_interval"`
	// KeyPrefix roots the etcd keyspace for this cluster's member
	// records (and, later, shard/leader keys).
	KeyPrefix string `koanf:"key_prefix"`
}

// ClusterEtcdConfig is the operator-facing etcd backend config.
// internal/cluster owns the runtime equivalent (cluster.EtcdConfig);
// boot wiring (later task) maps this onto it.
type ClusterEtcdConfig struct {
	Mode             string        `koanf:"mode"`
	Name             string        `koanf:"name"`
	DataDir          string        `koanf:"data_dir"`
	ClientURLs       []string      `koanf:"client_urls"`
	PeerURLs         []string      `koanf:"peer_urls"`
	Endpoints        []string      `koanf:"endpoints"`
	LeaseTTLSeconds  int           `koanf:"lease_ttl_seconds"`
	DialTimeout      time.Duration `koanf:"dial_timeout"`
	AutoSyncInterval time.Duration `koanf:"auto_sync_interval"`
	TLS              TLSConfig     `koanf:"tls"`
}

const (
	clusterModeEmbedded = "embedded"
	clusterModeExternal = "external"

	// minLeaseTTLSeconds is a conservative floor against a
	// zero/negative TTL that etcd would reject outright. The
	// stronger anti-flap rule (lease TTL ≥ 3× heartbeat) is
	// enforced as a cross-field check below — see
	// minLeaseHeartbeatRatio.
	minLeaseTTLSeconds = 1

	// minLeaseHeartbeatRatio is the Epic 13 leader-flap guard:
	// lease_ttl_seconds must be at least 3× the heartbeat
	// interval. A tighter ratio causes a transient missed
	// heartbeat to expire the lease and trigger spurious
	// failover/leader churn. The risk list is explicit that CI
	// must not allow tighter.
	minLeaseHeartbeatRatio = 3
)

// Validate enforces structural invariants. Disabled is always OK.
// Self-contained (no sibling config needed) so the cluster block
// stays table-driven-testable.
func (c *ClusterConfig) Validate() error {
	if !c.Enabled {
		return nil
	}
	switch c.Etcd.Mode {
	case clusterModeEmbedded:
		if c.Etcd.Name == "" {
			return fmt.Errorf("cluster.etcd.name is required in embedded mode")
		}
		if c.Etcd.DataDir == "" {
			return fmt.Errorf("cluster.etcd.data_dir is required in embedded mode")
		}
		if len(c.Etcd.ClientURLs) == 0 {
			return fmt.Errorf("cluster.etcd.client_urls is required in embedded mode")
		}
		if len(c.Etcd.PeerURLs) == 0 {
			return fmt.Errorf("cluster.etcd.peer_urls is required in embedded mode")
		}
	case clusterModeExternal:
		if len(c.Etcd.Endpoints) == 0 {
			return fmt.Errorf("cluster.etcd.endpoints is required in external mode")
		}
	default:
		return fmt.Errorf("cluster.etcd.mode must be %q or %q, got %q",
			clusterModeEmbedded, clusterModeExternal, c.Etcd.Mode)
	}
	if c.Etcd.LeaseTTLSeconds < minLeaseTTLSeconds {
		return fmt.Errorf("cluster.etcd.lease_ttl_seconds must be >= %d, got %d",
			minLeaseTTLSeconds, c.Etcd.LeaseTTLSeconds)
	}
	if c.Etcd.DialTimeout < 0 {
		return fmt.Errorf("cluster.etcd.dial_timeout must be non-negative, got %v", c.Etcd.DialTimeout)
	}
	if c.Etcd.AutoSyncInterval < 0 {
		return fmt.Errorf("cluster.etcd.auto_sync_interval must be non-negative, got %v", c.Etcd.AutoSyncInterval)
	}
	if c.Etcd.TLS.Enabled && (c.Etcd.TLS.CertFile == "" || c.Etcd.TLS.KeyFile == "") {
		return fmt.Errorf("cluster.etcd.tls requires certfile and keyfile when enabled")
	}
	if c.Membership.HeartbeatInterval <= 0 {
		return fmt.Errorf("cluster.membership.heartbeat_interval must be > 0, got %v",
			c.Membership.HeartbeatInterval)
	}
	if c.Membership.KeyPrefix == "" {
		return fmt.Errorf("cluster.membership.key_prefix is required")
	}
	// Anti-flap: lease TTL must be ≥ 3× the heartbeat interval.
	// Compare in seconds (etcd lease granularity); round the
	// heartbeat up so sub-second intervals can't sneak under.
	hbSeconds := int((c.Membership.HeartbeatInterval + time.Second - 1) / time.Second)
	if c.Etcd.LeaseTTLSeconds < minLeaseHeartbeatRatio*hbSeconds {
		return fmt.Errorf(
			"cluster.etcd.lease_ttl_seconds (%d) must be >= %dx cluster.membership.heartbeat_interval (%v ≈ %ds) to avoid leader flapping",
			c.Etcd.LeaseTTLSeconds, minLeaseHeartbeatRatio, c.Membership.HeartbeatInterval, hbSeconds)
	}
	return nil
}

// applyClusterDefaults seeds the etcd sub-config. Cluster stays
// disabled by default; the defaults only take effect once an
// operator flips cluster.enabled: true (or overrides individual
// fields), matching the disabled-by-default opt-in contract.
func applyClusterDefaults(c *ClusterConfig) {
	c.Enabled = false
	c.Etcd = ClusterEtcdConfig{
		Mode:             clusterModeEmbedded,
		Name:             "kscore-1",
		DataDir:          "./data/etcd",
		ClientURLs:       []string{"http://127.0.0.1:2379"},
		PeerURLs:         []string{"http://127.0.0.1:2380"},
		Endpoints:        nil,
		LeaseTTLSeconds:  15, // §4.15 default (exactly 3× the 5s heartbeat)
		DialTimeout:      5 * time.Second,
		AutoSyncInterval: 5 * time.Minute,
	}
	c.Membership = ClusterMembershipConfig{
		HeartbeatInterval: 5 * time.Second, // §4.15 default
		KeyPrefix:         "/kscore/cluster",
	}
}
