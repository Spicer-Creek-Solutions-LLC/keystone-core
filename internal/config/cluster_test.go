// SPDX-License-Identifier: Apache-2.0

package config

import (
	"strings"
	"testing"
	"time"
)

func validEmbeddedCluster() ClusterConfig {
	c := ClusterConfig{}
	applyClusterDefaults(&c)
	c.Enabled = true
	return c
}

func TestClusterConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*ClusterConfig)
		wantErr string
	}{
		{"disabled is always ok", func(c *ClusterConfig) { c.Enabled = false; c.Etcd.Mode = "garbage" }, ""},
		{"valid embedded defaults", func(c *ClusterConfig) {}, ""},
		{
			"valid external",
			func(c *ClusterConfig) {
				c.Etcd.Mode = "external"
				c.Etcd.Endpoints = []string{"http://10.0.0.1:2379"}
			},
			"",
		},
		{"bad mode", func(c *ClusterConfig) { c.Etcd.Mode = "raft" }, "mode must be"},
		{"embedded missing name", func(c *ClusterConfig) { c.Etcd.Name = "" }, "name is required"},
		{"embedded missing data_dir", func(c *ClusterConfig) { c.Etcd.DataDir = "" }, "data_dir is required"},
		{"embedded missing client_urls", func(c *ClusterConfig) { c.Etcd.ClientURLs = nil }, "client_urls is required"},
		{"embedded missing peer_urls", func(c *ClusterConfig) { c.Etcd.PeerURLs = nil }, "peer_urls is required"},
		{
			"external missing endpoints",
			func(c *ClusterConfig) { c.Etcd.Mode = "external"; c.Etcd.Endpoints = nil },
			"endpoints is required",
		},
		{"lease ttl too low", func(c *ClusterConfig) { c.Etcd.LeaseTTLSeconds = 0 }, "lease_ttl_seconds must be >="},
		{"negative dial timeout", func(c *ClusterConfig) { c.Etcd.DialTimeout = -time.Second }, "dial_timeout must be non-negative"},
		{"negative autosync", func(c *ClusterConfig) { c.Etcd.AutoSyncInterval = -time.Second }, "auto_sync_interval must be non-negative"},
		{
			"tls enabled without files",
			func(c *ClusterConfig) { c.Etcd.TLS.Enabled = true },
			"tls requires certfile and keyfile",
		},
		{"heartbeat zero", func(c *ClusterConfig) { c.Membership.HeartbeatInterval = 0 }, "heartbeat_interval must be > 0"},
		{"empty key prefix", func(c *ClusterConfig) { c.Membership.KeyPrefix = "" }, "key_prefix is required"},
		{
			"anti-flap: ttl < 3x heartbeat",
			func(c *ClusterConfig) { c.Etcd.LeaseTTLSeconds = 10; c.Membership.HeartbeatInterval = 5 * time.Second },
			"must be >= 3x",
		},
		{
			"anti-flap: ttl exactly 3x heartbeat ok",
			func(c *ClusterConfig) { c.Etcd.LeaseTTLSeconds = 15; c.Membership.HeartbeatInterval = 5 * time.Second },
			"",
		},
		{
			"anti-flap: sub-second heartbeat rounds up",
			func(c *ClusterConfig) {
				c.Etcd.LeaseTTLSeconds = 2
				c.Membership.HeartbeatInterval = 900 * time.Millisecond
			},
			"must be >= 3x", // 900ms rounds to 1s → need ttl ≥ 3
		},
		{"election session_ttl too low", func(c *ClusterConfig) { c.Election.SessionTTLSeconds = 0 }, "session_ttl_seconds must be >="},
		{"election negative recampaign", func(c *ClusterConfig) { c.Election.ReCampaignDelay = -time.Second }, "recampaign_delay must be non-negative"},
		{"shard virtual_nodes too low", func(c *ClusterConfig) { c.Shard.VirtualNodes = 0 }, "virtual_nodes must be >= 1"},
		{"shard negative rebalance_cooldown", func(c *ClusterConfig) { c.Shard.RebalanceCooldown = -time.Second }, "rebalance_cooldown must be non-negative"},
		{"health check_interval zero", func(c *ClusterConfig) { c.Health.CheckInterval = 0 }, "check_interval must be > 0"},
		{"health failure_threshold too low", func(c *ClusterConfig) { c.Health.FailureThreshold = 0 }, "failure_threshold must be >= 1"},
		{"health latency_window too low", func(c *ClusterConfig) { c.Health.LatencyWindow = 0 }, "latency_window must be >= 1"},
		{"failover negative cooldown", func(c *ClusterConfig) { c.Failover.Cooldown = -time.Second }, "failover.cooldown must be non-negative"},
		{"failover agent_batch too low", func(c *ClusterConfig) { c.Failover.AgentBatch = 0 }, "agent_batch must be >= 1"},
		{"failover job_batch too low", func(c *ClusterConfig) { c.Failover.JobBatch = 0 }, "job_batch must be >= 1"},
		{"recovery connect_timeout zero", func(c *ClusterConfig) { c.Recovery.ConnectTimeout = 0 }, "connect_timeout must be > 0"},
		{"recovery connect_retries too low", func(c *ClusterConfig) { c.Recovery.ConnectRetries = 0 }, "connect_retries must be >= 1"},
		{"fencing bad mode", func(c *ClusterConfig) { c.Fencing.Mode = "halt" }, "fencing.mode must be"},
		{"fencing strict ok", func(c *ClusterConfig) { c.Fencing.Mode = "strict" }, ""},
		{"fencing graceful ok", func(c *ClusterConfig) { c.Fencing.Mode = "graceful" }, ""},
		{"coord heartbeat_interval zero", func(c *ClusterConfig) { c.Coordination.HeartbeatInterval = 0 }, "coordination.heartbeat_interval must be > 0"},
		{"coord heartbeat_timeout zero", func(c *ClusterConfig) { c.Coordination.HeartbeatTimeout = 0 }, "coordination.heartbeat_timeout must be > 0"},
		{"coord failure_threshold low", func(c *ClusterConfig) { c.Coordination.FailureThreshold = 0 }, "coordination.failure_threshold must be >= 1"},
		{"coord retry_max low", func(c *ClusterConfig) { c.Coordination.RetryMax = 0 }, "coordination.retry_max must be >= 1"},
		{"coord base>max delay", func(c *ClusterConfig) {
			c.Coordination.RetryBaseDelay = 3 * time.Second
			c.Coordination.RetryMaxDelay = time.Second
		}, "retry_base_delay"},
		{"shutdown timeout zero", func(c *ClusterConfig) { c.Shutdown.Timeout = 0 }, "shutdown.timeout must be > 0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := validEmbeddedCluster()
			tt.mutate(&c)
			err := c.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate() = %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestApplyClusterDefaults(t *testing.T) {
	var c ClusterConfig
	applyClusterDefaults(&c)
	if c.Enabled {
		t.Error("clustering must default to disabled (opt-in)")
	}
	if c.Etcd.Mode != clusterModeEmbedded {
		t.Errorf("default mode = %q, want embedded", c.Etcd.Mode)
	}
	if c.Etcd.LeaseTTLSeconds != 15 {
		t.Errorf("default lease ttl = %d, want 15", c.Etcd.LeaseTTLSeconds)
	}
	if len(c.Etcd.ClientURLs) == 0 || len(c.Etcd.PeerURLs) == 0 {
		t.Error("default embedded URLs must be populated")
	}
	// Defaults must themselves validate once enabled.
	c.Enabled = true
	if err := c.Validate(); err != nil {
		t.Fatalf("defaulted cluster config invalid: %v", err)
	}
}

func TestDefaultConfig_ClusterDisabled(t *testing.T) {
	c := defaultConfig()
	if c.Cluster.Enabled {
		t.Fatal("defaultConfig must leave clustering disabled")
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("default config invalid: %v", err)
	}
}

func TestProductionWarnings_Cluster(t *testing.T) {
	c := defaultConfig()
	c.Mode = ModeProduction
	// Single-node (clustering disabled) is supported — no warning.
	if containsSubstr(c.ProductionWarnings(), "clustering") || containsSubstr(c.ProductionWarnings(), "etcd") {
		t.Errorf("disabled clustering must not warn, got %v", c.ProductionWarnings())
	}

	c.Cluster.Enabled = true // embedded by default
	if !containsSubstr(c.ProductionWarnings(), "embedded etcd") {
		t.Error("expected an embedded-etcd production warning when clustering enabled")
	}

	c.Cluster.Etcd.Mode = "external"
	c.Cluster.Etcd.Endpoints = []string{"http://10.0.0.1:2379"}
	if containsSubstr(c.ProductionWarnings(), "embedded etcd") {
		t.Error("external etcd must not trigger the embedded warning")
	}
}

func containsSubstr(ss []string, sub string) bool {
	for _, s := range ss {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
