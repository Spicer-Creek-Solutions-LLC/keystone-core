package cluster

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.False(t, cfg.Enabled)
	assert.Equal(t, "keystone-core", cfg.ClusterName)
	assert.Equal(t, 5*time.Second, cfg.HeartbeatInterval)
	assert.Equal(t, 30*time.Second, cfg.HeartbeatTimeout)
	assert.Equal(t, 15*time.Second, cfg.ElectionTimeout)
	assert.Equal(t, 9090, cfg.GRPCPort)
	assert.NotNil(t, cfg.Etcd)
}

func TestDefaultEtcdConfig(t *testing.T) {
	cfg := DefaultEtcdConfig()

	assert.Equal(t, EtcdModeEmbedded, cfg.Mode)
	assert.Equal(t, []string{"localhost:2379"}, cfg.Endpoints)
	assert.Equal(t, 5*time.Second, cfg.DialTimeout)
	assert.Equal(t, 10*time.Second, cfg.RequestTimeout)
	assert.Equal(t, int64(15), cfg.LeasesTTL)
	assert.Equal(t, "/keystone-core", cfg.KeyPrefix)
	assert.NotNil(t, cfg.Embedded)
}

func TestDefaultEtcdEmbeddedConfig(t *testing.T) {
	cfg := DefaultEtcdEmbeddedConfig()

	assert.Equal(t, "data/etcd", cfg.DataDir)
	assert.Equal(t, 2379, cfg.ClientPort)
	assert.Equal(t, 2380, cfg.PeerPort)
	assert.Equal(t, "new", cfg.InitialClusterState)
	assert.Equal(t, int64(2*1024*1024*1024), cfg.QuotaBackendBytes)
}

func TestConfigValidate_Disabled(t *testing.T) {
	cfg := &Config{Enabled: false}
	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestConfigValidate_Enabled(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr string
	}{
		{
			name: "valid config",
			config: &Config{
				Enabled:           true,
				ClusterName:       "test-cluster",
				HeartbeatInterval: 5 * time.Second,
				HeartbeatTimeout:  30 * time.Second,
				ElectionTimeout:   15 * time.Second,
				GRPCPort:          9090,
				Etcd:              DefaultEtcdConfig(),
			},
			wantErr: "",
		},
		{
			name: "missing cluster name",
			config: &Config{
				Enabled:           true,
				HeartbeatInterval: 5 * time.Second,
				HeartbeatTimeout:  30 * time.Second,
				ElectionTimeout:   15 * time.Second,
				GRPCPort:          9090,
				Etcd:              DefaultEtcdConfig(),
			},
			wantErr: "cluster_name is required",
		},
		{
			name: "invalid heartbeat interval",
			config: &Config{
				Enabled:           true,
				ClusterName:       "test",
				HeartbeatInterval: 0,
				HeartbeatTimeout:  30 * time.Second,
				ElectionTimeout:   15 * time.Second,
				GRPCPort:          9090,
				Etcd:              DefaultEtcdConfig(),
			},
			wantErr: "heartbeat_interval must be positive",
		},
		{
			name: "heartbeat timeout too short",
			config: &Config{
				Enabled:           true,
				ClusterName:       "test",
				HeartbeatInterval: 5 * time.Second,
				HeartbeatTimeout:  10 * time.Second, // Less than 3x heartbeat
				ElectionTimeout:   15 * time.Second,
				GRPCPort:          9090,
				Etcd:              DefaultEtcdConfig(),
			},
			wantErr: "heartbeat_timeout should be at least 3x heartbeat_interval",
		},
		{
			name: "invalid election timeout",
			config: &Config{
				Enabled:           true,
				ClusterName:       "test",
				HeartbeatInterval: 5 * time.Second,
				HeartbeatTimeout:  30 * time.Second,
				ElectionTimeout:   0,
				GRPCPort:          9090,
				Etcd:              DefaultEtcdConfig(),
			},
			wantErr: "election_timeout must be positive",
		},
		{
			name: "invalid grpc port",
			config: &Config{
				Enabled:           true,
				ClusterName:       "test",
				HeartbeatInterval: 5 * time.Second,
				HeartbeatTimeout:  30 * time.Second,
				ElectionTimeout:   15 * time.Second,
				GRPCPort:          0,
				Etcd:              DefaultEtcdConfig(),
			},
			wantErr: "grpc_port must be between 1 and 65535",
		},
		{
			name: "negative quorum size",
			config: &Config{
				Enabled:           true,
				ClusterName:       "test",
				HeartbeatInterval: 5 * time.Second,
				HeartbeatTimeout:  30 * time.Second,
				ElectionTimeout:   15 * time.Second,
				GRPCPort:          9090,
				QuorumSize:        -1,
				Etcd:              DefaultEtcdConfig(),
			},
			wantErr: "quorum_size cannot be negative",
		},
		{
			name: "missing etcd config",
			config: &Config{
				Enabled:           true,
				ClusterName:       "test",
				HeartbeatInterval: 5 * time.Second,
				HeartbeatTimeout:  30 * time.Second,
				ElectionTimeout:   15 * time.Second,
				GRPCPort:          9090,
				Etcd:              nil,
			},
			wantErr: "etcd configuration is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr == "" {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}

func TestEtcdConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  *EtcdConfig
		wantErr string
	}{
		{
			name:    "valid embedded config",
			config:  DefaultEtcdConfig(),
			wantErr: "",
		},
		{
			name: "valid external config",
			config: &EtcdConfig{
				Mode:           EtcdModeExternal,
				Endpoints:      []string{"localhost:2379", "localhost:2380"},
				DialTimeout:    5 * time.Second,
				RequestTimeout: 10 * time.Second,
				LeasesTTL:      15,
				KeyPrefix:      "/test",
			},
			wantErr: "",
		},
		{
			name: "external mode missing endpoints",
			config: &EtcdConfig{
				Mode:           EtcdModeExternal,
				Endpoints:      []string{},
				DialTimeout:    5 * time.Second,
				RequestTimeout: 10 * time.Second,
				LeasesTTL:      15,
				KeyPrefix:      "/test",
			},
			wantErr: "at least one etcd endpoint is required",
		},
		{
			name: "embedded mode missing config",
			config: &EtcdConfig{
				Mode:           EtcdModeEmbedded,
				DialTimeout:    5 * time.Second,
				RequestTimeout: 10 * time.Second,
				LeasesTTL:      15,
				KeyPrefix:      "/test",
				Embedded:       nil,
			},
			wantErr: "embedded etcd configuration is required",
		},
		{
			name: "invalid mode",
			config: &EtcdConfig{
				Mode:           "invalid",
				DialTimeout:    5 * time.Second,
				RequestTimeout: 10 * time.Second,
				LeasesTTL:      15,
				KeyPrefix:      "/test",
			},
			wantErr: "invalid etcd mode",
		},
		{
			name: "invalid dial timeout",
			config: &EtcdConfig{
				Mode:           EtcdModeExternal,
				Endpoints:      []string{"localhost:2379"},
				DialTimeout:    0,
				RequestTimeout: 10 * time.Second,
				LeasesTTL:      15,
				KeyPrefix:      "/test",
			},
			wantErr: "dial_timeout must be positive",
		},
		{
			name: "invalid request timeout",
			config: &EtcdConfig{
				Mode:           EtcdModeExternal,
				Endpoints:      []string{"localhost:2379"},
				DialTimeout:    5 * time.Second,
				RequestTimeout: 0,
				LeasesTTL:      15,
				KeyPrefix:      "/test",
			},
			wantErr: "request_timeout must be positive",
		},
		{
			name: "lease ttl too short",
			config: &EtcdConfig{
				Mode:           EtcdModeExternal,
				Endpoints:      []string{"localhost:2379"},
				DialTimeout:    5 * time.Second,
				RequestTimeout: 10 * time.Second,
				LeasesTTL:      2,
				KeyPrefix:      "/test",
			},
			wantErr: "leases_ttl must be at least 5 seconds",
		},
		{
			name: "missing key prefix",
			config: &EtcdConfig{
				Mode:           EtcdModeExternal,
				Endpoints:      []string{"localhost:2379"},
				DialTimeout:    5 * time.Second,
				RequestTimeout: 10 * time.Second,
				LeasesTTL:      15,
				KeyPrefix:      "",
			},
			wantErr: "key_prefix is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr == "" {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}

func TestEtcdEmbeddedConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  *EtcdEmbeddedConfig
		wantErr string
	}{
		{
			name:    "valid config",
			config:  DefaultEtcdEmbeddedConfig(),
			wantErr: "",
		},
		{
			name: "missing data dir",
			config: &EtcdEmbeddedConfig{
				DataDir:             "",
				ClientPort:          2379,
				PeerPort:            2380,
				InitialClusterState: "new",
				QuotaBackendBytes:   2 * 1024 * 1024 * 1024,
			},
			wantErr: "data_dir is required",
		},
		{
			name: "invalid client port",
			config: &EtcdEmbeddedConfig{
				DataDir:             "data/etcd",
				ClientPort:          0,
				PeerPort:            2380,
				InitialClusterState: "new",
				QuotaBackendBytes:   2 * 1024 * 1024 * 1024,
			},
			wantErr: "client_port must be between 1 and 65535",
		},
		{
			name: "invalid peer port",
			config: &EtcdEmbeddedConfig{
				DataDir:             "data/etcd",
				ClientPort:          2379,
				PeerPort:            0,
				InitialClusterState: "new",
				QuotaBackendBytes:   2 * 1024 * 1024 * 1024,
			},
			wantErr: "peer_port must be between 1 and 65535",
		},
		{
			name: "same client and peer port",
			config: &EtcdEmbeddedConfig{
				DataDir:             "data/etcd",
				ClientPort:          2379,
				PeerPort:            2379,
				InitialClusterState: "new",
				QuotaBackendBytes:   2 * 1024 * 1024 * 1024,
			},
			wantErr: "client_port and peer_port must be different",
		},
		{
			name: "invalid cluster state",
			config: &EtcdEmbeddedConfig{
				DataDir:             "data/etcd",
				ClientPort:          2379,
				PeerPort:            2380,
				InitialClusterState: "invalid",
				QuotaBackendBytes:   2 * 1024 * 1024 * 1024,
			},
			wantErr: "initial_cluster_state must be 'new' or 'existing'",
		},
		{
			name: "quota too small",
			config: &EtcdEmbeddedConfig{
				DataDir:             "data/etcd",
				ClientPort:          2379,
				PeerPort:            2380,
				InitialClusterState: "new",
				QuotaBackendBytes:   50 * 1024 * 1024,
			},
			wantErr: "quota_backend_bytes must be at least 100MB",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr == "" {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}

func TestEtcdTLSConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  *EtcdTLSConfig
		wantErr string
	}{
		{
			name:    "disabled tls",
			config:  &EtcdTLSConfig{Enabled: false},
			wantErr: "",
		},
		{
			name: "valid tls config",
			config: &EtcdTLSConfig{
				Enabled:  true,
				CertFile: "/path/to/cert",
				KeyFile:  "/path/to/key",
				CAFile:   "/path/to/ca",
			},
			wantErr: "",
		},
		{
			name: "missing cert file",
			config: &EtcdTLSConfig{
				Enabled: true,
				KeyFile: "/path/to/key",
				CAFile:  "/path/to/ca",
			},
			wantErr: "cert_file is required",
		},
		{
			name: "missing key file",
			config: &EtcdTLSConfig{
				Enabled:  true,
				CertFile: "/path/to/cert",
				CAFile:   "/path/to/ca",
			},
			wantErr: "key_file is required",
		},
		{
			name: "missing ca file without insecure",
			config: &EtcdTLSConfig{
				Enabled:  true,
				CertFile: "/path/to/cert",
				KeyFile:  "/path/to/key",
			},
			wantErr: "ca_file is required",
		},
		{
			name: "insecure skip verify",
			config: &EtcdTLSConfig{
				Enabled:            true,
				CertFile:           "/path/to/cert",
				KeyFile:            "/path/to/key",
				InsecureSkipVerify: true,
			},
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr == "" {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}

func TestCalculateQuorumSize(t *testing.T) {
	tests := []struct {
		memberCount int
		expected    int
	}{
		{0, 0},
		{1, 1},
		{2, 2},
		{3, 2},
		{4, 3},
		{5, 3},
		{6, 4},
		{7, 4},
	}

	for _, tt := range tests {
		result := CalculateQuorumSize(tt.memberCount)
		assert.Equal(t, tt.expected, result, "memberCount=%d", tt.memberCount)
	}
}

func TestCanTolerateFailures(t *testing.T) {
	tests := []struct {
		memberCount int
		expected    int
	}{
		{0, 0},
		{1, 0},
		{2, 0},
		{3, 1},
		{4, 1},
		{5, 2},
		{6, 2},
		{7, 3},
	}

	for _, tt := range tests {
		result := CanTolerateFailures(tt.memberCount)
		assert.Equal(t, tt.expected, result, "memberCount=%d", tt.memberCount)
	}
}

func TestMemberStatus(t *testing.T) {
	tests := []struct {
		status    MemberStatus
		isHealthy bool
	}{
		{MemberStatusHealthy, true},
		{MemberStatusDegraded, true},
		{MemberStatusUnhealthy, false},
		{MemberStatusUnknown, false},
		{MemberStatusLeaving, false},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.isHealthy, tt.status.IsHealthy(), "status=%s", tt.status)
		assert.Equal(t, string(tt.status), tt.status.String())
	}
}

func TestClusterStatus(t *testing.T) {
	tests := []ClusterStatus{
		ClusterStatusHealthy,
		ClusterStatusDegraded,
		ClusterStatusUnhealthy,
		ClusterStatusForming,
	}

	for _, status := range tests {
		assert.Equal(t, string(status), status.String())
	}
}

func TestEtcdMode(t *testing.T) {
	tests := []EtcdMode{
		EtcdModeEmbedded,
		EtcdModeExternal,
	}

	for _, mode := range tests {
		assert.Equal(t, string(mode), mode.String())
	}
}

func TestConfigGetGRPCAddress(t *testing.T) {
	cfg := &Config{
		AdvertiseAddress: "192.168.1.100",
		GRPCPort:         9090,
	}

	addr, err := cfg.GetGRPCAddress()
	require.NoError(t, err)
	assert.Equal(t, "192.168.1.100:9090", addr)
}
