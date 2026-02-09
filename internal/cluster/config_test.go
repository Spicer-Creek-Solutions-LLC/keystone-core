package cluster

import (
	"os"
	"strings"
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
			name: "insecure skip verify blocked without env var",
			config: &EtcdTLSConfig{
				Enabled:            true,
				CertFile:           "/path/to/cert",
				KeyFile:            "/path/to/key",
				InsecureSkipVerify: true,
			},
			wantErr: "insecure_skip_verify is not allowed in production",
		},
		{
			name: "invalid min version",
			config: &EtcdTLSConfig{
				Enabled:    true,
				CertFile:   "/path/to/cert",
				KeyFile:    "/path/to/key",
				CAFile:     "/path/to/ca",
				MinVersion: "1.1",
			},
			wantErr: "min_version",
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

func TestEtcdTLSConfigValidate_InsecureWithEnvVar(t *testing.T) {
	// Test that InsecureSkipVerify is allowed when KSCORE_ALLOW_INSECURE_TLS=1 is set
	os.Setenv("KSCORE_ALLOW_INSECURE_TLS", "1")
	defer os.Unsetenv("KSCORE_ALLOW_INSECURE_TLS")

	config := &EtcdTLSConfig{
		Enabled:            true,
		CertFile:           "/path/to/cert",
		KeyFile:            "/path/to/key",
		InsecureSkipVerify: true,
	}

	err := config.Validate()
	assert.NoError(t, err, "InsecureSkipVerify should be allowed when KSCORE_ALLOW_INSECURE_TLS=1")
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

func TestStatus(t *testing.T) {
	tests := []Status{
		StatusHealthy,
		StatusDegraded,
		StatusUnhealthy,
		StatusForming,
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

func TestConfigGetGRPCAddress_IPv6(t *testing.T) {
	cfg := &Config{
		AdvertiseAddress: "2001:db8::1",
		GRPCPort:         9090,
	}

	addr, err := cfg.GetGRPCAddress()
	require.NoError(t, err)
	assert.Equal(t, "[2001:db8::1]:9090", addr)
}

func TestAddressFamilyPreference_String(t *testing.T) {
	tests := []struct {
		pref AddressFamilyPreference
		want string
	}{
		{PreferIPv4, "prefer_ipv4"},
		{PreferIPv6, "prefer_ipv6"},
		{IPv4Only, "ipv4_only"},
		{IPv6Only, "ipv6_only"},
		{AddressFamilyPreference(99), "unknown"},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, tt.pref.String())
	}
}

func TestParseAddressFamilyPreference(t *testing.T) {
	tests := []struct {
		input   string
		want    AddressFamilyPreference
		wantErr bool
	}{
		{"prefer_ipv4", PreferIPv4, false},
		{"preferipv4", PreferIPv4, false},
		{"prefer-ipv4", PreferIPv4, false},
		{"prefer_ipv6", PreferIPv6, false},
		{"preferipv6", PreferIPv6, false},
		{"prefer-ipv6", PreferIPv6, false},
		{"ipv4_only", IPv4Only, false},
		{"ipv4only", IPv4Only, false},
		{"ipv4-only", IPv4Only, false},
		{"ipv4", IPv4Only, false},
		{"ipv6_only", IPv6Only, false},
		{"ipv6only", IPv6Only, false},
		{"ipv6-only", IPv6Only, false},
		{"ipv6", IPv6Only, false},
		{"invalid", PreferIPv4, true},
	}

	for _, tt := range tests {
		got, err := ParseAddressFamilyPreference(tt.input)
		if tt.wantErr {
			assert.Error(t, err, "input=%s", tt.input)
		} else {
			assert.NoError(t, err, "input=%s", tt.input)
			assert.Equal(t, tt.want, got, "input=%s", tt.input)
		}
	}
}

func TestIsIPv6Address(t *testing.T) {
	tests := []struct {
		addr string
		want bool
	}{
		// IPv6 addresses
		{"::1", true},
		{"2001:db8::1", true},
		{"fe80::1", true},
		{"[2001:db8::1]", true}, // With brackets
		{"[::1]", true},
		// IPv4 addresses
		{"192.168.1.1", false},
		{"10.0.0.1", false},
		{"127.0.0.1", false},
		// Invalid
		{"invalid", false},
		{"", false},
	}

	for _, tt := range tests {
		got := IsIPv6Address(tt.addr)
		assert.Equal(t, tt.want, got, "addr=%s", tt.addr)
	}
}

func TestFormatHostPort(t *testing.T) {
	tests := []struct {
		host string
		port int
		want string
	}{
		// IPv4
		{"192.168.1.1", 2379, "192.168.1.1:2379"},
		{"10.0.0.1", 9090, "10.0.0.1:9090"},
		{"localhost", 8080, "localhost:8080"},
		// IPv6
		{"::1", 2379, "[::1]:2379"},
		{"2001:db8::1", 2379, "[2001:db8::1]:2379"},
		{"fe80::1", 9090, "[fe80::1]:9090"},
	}

	for _, tt := range tests {
		got := FormatHostPort(tt.host, tt.port)
		assert.Equal(t, tt.want, got, "host=%s port=%d", tt.host, tt.port)
	}
}

func TestFormatEtcdURL(t *testing.T) {
	tests := []struct {
		host   string
		port   int
		useTLS bool
		want   string
	}{
		// HTTP
		{"localhost", 2379, false, "http://localhost:2379"},
		{"192.168.1.1", 2379, false, "http://192.168.1.1:2379"},
		{"::1", 2379, false, "http://[::1]:2379"},
		{"2001:db8::1", 2380, false, "http://[2001:db8::1]:2380"},
		// HTTPS
		{"localhost", 2379, true, "https://localhost:2379"},
		{"192.168.1.1", 2379, true, "https://192.168.1.1:2379"},
		{"::1", 2379, true, "https://[::1]:2379"},
		{"2001:db8::1", 2380, true, "https://[2001:db8::1]:2380"},
	}

	for _, tt := range tests {
		got := FormatEtcdURL(tt.host, tt.port, tt.useTLS)
		assert.Equal(t, tt.want, got, "host=%s port=%d useTLS=%v", tt.host, tt.port, tt.useTLS)
	}
}

func TestEtcdEmbeddedConfig_GetAddresses(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		cfg := DefaultEtcdEmbeddedConfig()
		assert.Equal(t, "localhost", cfg.GetListenAddress())
		assert.Equal(t, "localhost", cfg.GetAdvertiseAddress())
	})

	t.Run("with listen address", func(t *testing.T) {
		cfg := DefaultEtcdEmbeddedConfig()
		cfg.ListenAddress = "0.0.0.0"
		assert.Equal(t, "0.0.0.0", cfg.GetListenAddress())
		assert.Equal(t, "0.0.0.0", cfg.GetAdvertiseAddress()) // Falls back to listen
	})

	t.Run("with both addresses", func(t *testing.T) {
		cfg := DefaultEtcdEmbeddedConfig()
		cfg.ListenAddress = "0.0.0.0"
		cfg.AdvertiseAddress = "192.168.1.100"
		assert.Equal(t, "0.0.0.0", cfg.GetListenAddress())
		assert.Equal(t, "192.168.1.100", cfg.GetAdvertiseAddress())
	})

	t.Run("IPv6 listen address", func(t *testing.T) {
		cfg := DefaultEtcdEmbeddedConfig()
		cfg.ListenAddress = "::"
		assert.Equal(t, "::", cfg.GetListenAddress())
		assert.Equal(t, "::", cfg.GetAdvertiseAddress())
	})

	t.Run("IPv6 advertise address", func(t *testing.T) {
		cfg := DefaultEtcdEmbeddedConfig()
		cfg.ListenAddress = "::"
		cfg.AdvertiseAddress = "2001:db8::1"
		assert.Equal(t, "::", cfg.GetListenAddress())
		assert.Equal(t, "2001:db8::1", cfg.GetAdvertiseAddress())
	})
}

func TestEtcdEmbeddedConfig_GetURLs(t *testing.T) {
	t.Run("default IPv4", func(t *testing.T) {
		cfg := DefaultEtcdEmbeddedConfig()
		assert.Equal(t, "http://localhost:2379", cfg.GetClientListenURL(false))
		assert.Equal(t, "http://localhost:2379", cfg.GetClientAdvertiseURL(false))
		assert.Equal(t, "http://localhost:2380", cfg.GetPeerListenURL(false))
		assert.Equal(t, "http://localhost:2380", cfg.GetPeerAdvertiseURL(false))
		assert.Equal(t, "localhost:2379", cfg.GetClientEndpoint())
	})

	t.Run("IPv6 loopback", func(t *testing.T) {
		cfg := DefaultEtcdEmbeddedConfig()
		cfg.ListenAddress = "::1"
		assert.Equal(t, "http://[::1]:2379", cfg.GetClientListenURL(false))
		assert.Equal(t, "http://[::1]:2379", cfg.GetClientAdvertiseURL(false))
		assert.Equal(t, "http://[::1]:2380", cfg.GetPeerListenURL(false))
		assert.Equal(t, "http://[::1]:2380", cfg.GetPeerAdvertiseURL(false))
		assert.Equal(t, "[::1]:2379", cfg.GetClientEndpoint())
	})

	t.Run("IPv6 global", func(t *testing.T) {
		cfg := DefaultEtcdEmbeddedConfig()
		cfg.ListenAddress = "::"
		cfg.AdvertiseAddress = "2001:db8::1"
		assert.Equal(t, "http://[::]:2379", cfg.GetClientListenURL(false))
		assert.Equal(t, "http://[2001:db8::1]:2379", cfg.GetClientAdvertiseURL(false))
		assert.Equal(t, "http://[::]:2380", cfg.GetPeerListenURL(false))
		assert.Equal(t, "http://[2001:db8::1]:2380", cfg.GetPeerAdvertiseURL(false))
		assert.Equal(t, "[2001:db8::1]:2379", cfg.GetClientEndpoint())
	})

	t.Run("with TLS", func(t *testing.T) {
		cfg := DefaultEtcdEmbeddedConfig()
		cfg.ListenAddress = "2001:db8::1"
		assert.Equal(t, "https://[2001:db8::1]:2379", cfg.GetClientListenURL(true))
		assert.Equal(t, "https://[2001:db8::1]:2380", cfg.GetPeerListenURL(true))
	})
}

func TestDefaultConfig_AddressFamilyPreference(t *testing.T) {
	cfg := DefaultConfig()
	assert.Equal(t, PreferIPv4, cfg.AddressFamilyPreference)
}

func TestEtcdEmbeddedTLSConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  *EtcdEmbeddedTLSConfig
		wantErr string
	}{
		{
			name:    "disabled tls",
			config:  &EtcdEmbeddedTLSConfig{Enabled: false},
			wantErr: "",
		},
		{
			name: "auto tls enabled",
			config: &EtcdEmbeddedTLSConfig{
				Enabled: true,
				AutoTLS: true,
			},
			wantErr: "",
		},
		{
			name: "peer auto tls enabled",
			config: &EtcdEmbeddedTLSConfig{
				Enabled:     true,
				AutoTLS:     true,
				PeerAutoTLS: true,
			},
			wantErr: "",
		},
		{
			name: "missing client cert without auto tls",
			config: &EtcdEmbeddedTLSConfig{
				Enabled:       true,
				ClientKeyFile: "/path/to/key",
			},
			wantErr: "client_cert_file is required when TLS is enabled",
		},
		{
			name: "missing client key without auto tls",
			config: &EtcdEmbeddedTLSConfig{
				Enabled:        true,
				ClientCertFile: "/path/to/cert",
			},
			wantErr: "client_key_file is required when TLS is enabled",
		},
		{
			name: "client cert file not found",
			config: &EtcdEmbeddedTLSConfig{
				Enabled:        true,
				ClientCertFile: "/nonexistent/client.crt",
				ClientKeyFile:  "/nonexistent/client.key",
			},
			wantErr: "client_cert_file not found",
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

func TestEtcdEmbeddedTLSConfigValidate_WithTempFiles(t *testing.T) {
	// Create temporary certificate and key files for testing
	tmpDir := t.TempDir()

	clientCertFile := tmpDir + "/client.crt"
	clientKeyFile := tmpDir + "/client.key"
	clientCAFile := tmpDir + "/client-ca.crt"
	peerCertFile := tmpDir + "/peer.crt"
	peerKeyFile := tmpDir + "/peer.key"
	peerCAFile := tmpDir + "/peer-ca.crt"

	// Create empty test files
	for _, f := range []string{clientCertFile, clientKeyFile, clientCAFile, peerCertFile, peerKeyFile, peerCAFile} {
		err := os.WriteFile(f, []byte("test"), 0644)
		require.NoError(t, err)
	}

	tests := []struct {
		name    string
		config  *EtcdEmbeddedTLSConfig
		wantErr string
	}{
		{
			name: "valid manual tls config",
			config: &EtcdEmbeddedTLSConfig{
				Enabled:        true,
				ClientCertFile: clientCertFile,
				ClientKeyFile:  clientKeyFile,
			},
			wantErr: "",
		},
		{
			name: "valid tls with client CA",
			config: &EtcdEmbeddedTLSConfig{
				Enabled:        true,
				ClientCertFile: clientCertFile,
				ClientKeyFile:  clientKeyFile,
				ClientCAFile:   clientCAFile,
			},
			wantErr: "",
		},
		{
			name: "valid tls with peer certs",
			config: &EtcdEmbeddedTLSConfig{
				Enabled:        true,
				ClientCertFile: clientCertFile,
				ClientKeyFile:  clientKeyFile,
				PeerCertFile:   peerCertFile,
				PeerKeyFile:    peerKeyFile,
			},
			wantErr: "",
		},
		{
			name: "valid tls with all certs",
			config: &EtcdEmbeddedTLSConfig{
				Enabled:        true,
				ClientCertFile: clientCertFile,
				ClientKeyFile:  clientKeyFile,
				ClientCAFile:   clientCAFile,
				PeerCertFile:   peerCertFile,
				PeerKeyFile:    peerKeyFile,
				PeerCAFile:     peerCAFile,
				ClientCertAuth: true,
				PeerCertAuth:   true,
			},
			wantErr: "",
		},
		{
			name: "peer cert file not found",
			config: &EtcdEmbeddedTLSConfig{
				Enabled:        true,
				ClientCertFile: clientCertFile,
				ClientKeyFile:  clientKeyFile,
				PeerCertFile:   "/nonexistent/peer.crt",
			},
			wantErr: "peer_cert_file not found",
		},
		{
			name: "peer key file not found",
			config: &EtcdEmbeddedTLSConfig{
				Enabled:        true,
				ClientCertFile: clientCertFile,
				ClientKeyFile:  clientKeyFile,
				PeerKeyFile:    "/nonexistent/peer.key",
			},
			wantErr: "peer_key_file not found",
		},
		{
			name: "client CA file not found",
			config: &EtcdEmbeddedTLSConfig{
				Enabled:        true,
				ClientCertFile: clientCertFile,
				ClientKeyFile:  clientKeyFile,
				ClientCAFile:   "/nonexistent/ca.crt",
			},
			wantErr: "client_ca_file not found",
		},
		{
			name: "peer CA file not found",
			config: &EtcdEmbeddedTLSConfig{
				Enabled:        true,
				ClientCertFile: clientCertFile,
				ClientKeyFile:  clientKeyFile,
				PeerCAFile:     "/nonexistent/peer-ca.crt",
			},
			wantErr: "peer_ca_file not found",
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

func TestEtcdEmbeddedTLSConfig_GetPeerCertFile(t *testing.T) {
	tests := []struct {
		name     string
		config   *EtcdEmbeddedTLSConfig
		expected string
	}{
		{
			name: "peer cert file specified",
			config: &EtcdEmbeddedTLSConfig{
				ClientCertFile: "/path/to/client.crt",
				PeerCertFile:   "/path/to/peer.crt",
			},
			expected: "/path/to/peer.crt",
		},
		{
			name: "peer cert file empty - falls back to client",
			config: &EtcdEmbeddedTLSConfig{
				ClientCertFile: "/path/to/client.crt",
				PeerCertFile:   "",
			},
			expected: "/path/to/client.crt",
		},
		{
			name:     "both empty",
			config:   &EtcdEmbeddedTLSConfig{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.config.GetPeerCertFile())
		})
	}
}

func TestEtcdEmbeddedTLSConfig_GetPeerKeyFile(t *testing.T) {
	tests := []struct {
		name     string
		config   *EtcdEmbeddedTLSConfig
		expected string
	}{
		{
			name: "peer key file specified",
			config: &EtcdEmbeddedTLSConfig{
				ClientKeyFile: "/path/to/client.key",
				PeerKeyFile:   "/path/to/peer.key",
			},
			expected: "/path/to/peer.key",
		},
		{
			name: "peer key file empty - falls back to client",
			config: &EtcdEmbeddedTLSConfig{
				ClientKeyFile: "/path/to/client.key",
				PeerKeyFile:   "",
			},
			expected: "/path/to/client.key",
		},
		{
			name:     "both empty",
			config:   &EtcdEmbeddedTLSConfig{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.config.GetPeerKeyFile())
		})
	}
}

func TestEtcdEmbeddedTLSConfig_GetPeerCAFile(t *testing.T) {
	tests := []struct {
		name     string
		config   *EtcdEmbeddedTLSConfig
		expected string
	}{
		{
			name: "peer CA file specified",
			config: &EtcdEmbeddedTLSConfig{
				ClientCAFile: "/path/to/client-ca.crt",
				PeerCAFile:   "/path/to/peer-ca.crt",
			},
			expected: "/path/to/peer-ca.crt",
		},
		{
			name: "peer CA file empty - falls back to client",
			config: &EtcdEmbeddedTLSConfig{
				ClientCAFile: "/path/to/client-ca.crt",
				PeerCAFile:   "",
			},
			expected: "/path/to/client-ca.crt",
		},
		{
			name:     "both empty",
			config:   &EtcdEmbeddedTLSConfig{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.config.GetPeerCAFile())
		})
	}
}

func TestEtcdEmbeddedConfig_IsTLSEnabled(t *testing.T) {
	tests := []struct {
		name     string
		config   *EtcdEmbeddedConfig
		expected bool
	}{
		{
			name:     "nil TLS config",
			config:   &EtcdEmbeddedConfig{TLS: nil},
			expected: false,
		},
		{
			name: "TLS disabled",
			config: &EtcdEmbeddedConfig{
				TLS: &EtcdEmbeddedTLSConfig{Enabled: false},
			},
			expected: false,
		},
		{
			name: "TLS enabled",
			config: &EtcdEmbeddedConfig{
				TLS: &EtcdEmbeddedTLSConfig{Enabled: true},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.config.IsTLSEnabled())
		})
	}
}

func TestEtcdEmbeddedConfigValidate_WithTLS(t *testing.T) {
	// Test that embedded config validation calls TLS validation
	config := &EtcdEmbeddedConfig{
		DataDir:             "data/etcd",
		ClientPort:          2379,
		PeerPort:            2380,
		InitialClusterState: "new",
		QuotaBackendBytes:   2 * 1024 * 1024 * 1024,
		TLS: &EtcdEmbeddedTLSConfig{
			Enabled:       true,
			ClientKeyFile: "/path/to/key", // Missing cert file
		},
	}

	err := config.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "client_cert_file is required")
}

// Tests for HA recommendation functions

func TestCountClusterMembers(t *testing.T) {
	tests := []struct {
		name           string
		initialCluster string
		expected       int
	}{
		{
			name:           "empty",
			initialCluster: "",
			expected:       0,
		},
		{
			name:           "single node",
			initialCluster: "kscore-1=http://192.168.1.10:2380",
			expected:       1,
		},
		{
			name:           "three nodes",
			initialCluster: "kscore-1=http://192.168.1.10:2380,kscore-2=http://192.168.1.11:2380,kscore-3=http://192.168.1.12:2380",
			expected:       3,
		},
		{
			name:           "five nodes with spaces",
			initialCluster: "node1=http://10.0.0.1:2380, node2=http://10.0.0.2:2380, node3=http://10.0.0.3:2380, node4=http://10.0.0.4:2380, node5=http://10.0.0.5:2380",
			expected:       5,
		},
		{
			name:           "malformed entry ignored",
			initialCluster: "kscore-1=http://host1:2380,badentry,kscore-2=http://host2:2380",
			expected:       2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count := countClusterMembers(tt.initialCluster)
			assert.Equal(t, tt.expected, count)
		})
	}
}

func TestIsOddClusterSize(t *testing.T) {
	tests := []struct {
		count    int
		expected bool
	}{
		{0, false},
		{1, true},
		{2, false},
		{3, true},
		{4, false},
		{5, true},
		{6, false},
		{7, true},
	}

	for _, tt := range tests {
		t.Run(string(rune('0'+tt.count)), func(t *testing.T) {
			assert.Equal(t, tt.expected, IsOddClusterSize(tt.count))
		})
	}
}

func TestRecommendedClusterSize(t *testing.T) {
	tests := []struct {
		tolerateFailures int
		expected         int
	}{
		{0, 1},
		{1, 3},
		{2, 5},
		{3, 7},
		{4, 9},
	}

	for _, tt := range tests {
		t.Run(string(rune('0'+tt.tolerateFailures)), func(t *testing.T) {
			assert.Equal(t, tt.expected, RecommendedClusterSize(tt.tolerateFailures))
		})
	}
}

func TestHARecommendations_Disabled(t *testing.T) {
	cfg := &Config{Enabled: false}
	recs := cfg.HARecommendations()
	assert.Empty(t, recs)
}

func TestHARecommendations_SingleNode(t *testing.T) {
	cfg := &Config{
		Enabled:           true,
		ClusterName:       "test",
		HeartbeatInterval: 5 * time.Second,
		HeartbeatTimeout:  30 * time.Second,
		ElectionTimeout:   15 * time.Second,
		Etcd: &EtcdConfig{
			Mode: EtcdModeEmbedded,
			Embedded: &EtcdEmbeddedConfig{
				InitialCluster: "kscore-1=http://192.168.1.10:2380",
			},
		},
	}

	recs := cfg.HARecommendations()

	// Should have critical recommendation about single node
	var foundSingleNode bool
	for _, r := range recs {
		if r.Level == "critical" && r.Component == "cluster" {
			foundSingleNode = true
			assert.Contains(t, r.Issue, "no fault tolerance")
		}
	}
	assert.True(t, foundSingleNode, "Should have critical recommendation for single-node cluster")
}

func TestHARecommendations_TwoNode(t *testing.T) {
	cfg := &Config{
		Enabled:           true,
		ClusterName:       "test",
		HeartbeatInterval: 5 * time.Second,
		HeartbeatTimeout:  30 * time.Second,
		ElectionTimeout:   15 * time.Second,
		Etcd: &EtcdConfig{
			Mode: EtcdModeEmbedded,
			Embedded: &EtcdEmbeddedConfig{
				InitialCluster: "kscore-1=http://192.168.1.10:2380,kscore-2=http://192.168.1.11:2380",
			},
		},
	}

	recs := cfg.HARecommendations()

	// Should have critical recommendation about 2 nodes and warning about even number
	var foundTwoNode, foundEven bool
	for _, r := range recs {
		if r.Level == "critical" && r.Component == "cluster" && strings.Contains(r.Issue, "2-node") {
			foundTwoNode = true
		}
		if r.Level == "warning" && r.Component == "cluster" && strings.Contains(r.Issue, "even number") {
			foundEven = true
		}
	}
	assert.True(t, foundTwoNode, "Should have critical recommendation for 2-node cluster")
	assert.True(t, foundEven, "Should have warning for even cluster size")
}

func TestHARecommendations_ThreeNode(t *testing.T) {
	cfg := &Config{
		Enabled:           true,
		ClusterName:       "test",
		HeartbeatInterval: 5 * time.Second,
		HeartbeatTimeout:  30 * time.Second,
		ElectionTimeout:   15 * time.Second,
		Etcd: &EtcdConfig{
			Mode: EtcdModeEmbedded,
			Embedded: &EtcdEmbeddedConfig{
				InitialCluster: "kscore-1=http://192.168.1.10:2380,kscore-2=http://192.168.1.11:2380,kscore-3=http://192.168.1.12:2380",
			},
		},
	}

	recs := cfg.HARecommendations()

	// Should NOT have critical about cluster size (3 is valid)
	for _, r := range recs {
		if r.Level == "critical" && r.Component == "cluster" {
			if strings.Contains(r.Issue, "no fault tolerance") || strings.Contains(r.Issue, "2-node") {
				t.Errorf("Should not have critical cluster size recommendation for 3-node cluster")
			}
		}
	}
}

func TestHARecommendations_FourNode_EvenWarning(t *testing.T) {
	cfg := &Config{
		Enabled:           true,
		ClusterName:       "test",
		HeartbeatInterval: 5 * time.Second,
		HeartbeatTimeout:  30 * time.Second,
		ElectionTimeout:   15 * time.Second,
		Etcd: &EtcdConfig{
			Mode: EtcdModeEmbedded,
			Embedded: &EtcdEmbeddedConfig{
				InitialCluster: "kscore-1=http://h1:2380,kscore-2=http://h2:2380,kscore-3=http://h3:2380,kscore-4=http://h4:2380",
			},
		},
	}

	recs := cfg.HARecommendations()

	// Should have warning about even number
	var foundEven bool
	for _, r := range recs {
		if r.Level == "warning" && r.Component == "cluster" && strings.Contains(r.Issue, "even number") {
			foundEven = true
		}
	}
	assert.True(t, foundEven, "Should have warning for 4-node (even) cluster")
}

func TestHARecommendations_HeartbeatTimeout(t *testing.T) {
	cfg := &Config{
		Enabled:           true,
		ClusterName:       "test",
		HeartbeatInterval: 5 * time.Second,
		HeartbeatTimeout:  10 * time.Second, // Less than 3x interval (15s)
		ElectionTimeout:   15 * time.Second,
		Etcd: &EtcdConfig{
			Mode:     EtcdModeEmbedded,
			Embedded: &EtcdEmbeddedConfig{},
		},
	}

	recs := cfg.HARecommendations()

	var foundHeartbeatRec bool
	for _, r := range recs {
		if r.Component == "cluster" && strings.Contains(r.Issue, "Heartbeat timeout") {
			foundHeartbeatRec = true
			assert.Equal(t, "warning", r.Level)
		}
	}
	assert.True(t, foundHeartbeatRec, "Should have warning about heartbeat timeout")
}

func TestHARecommendations_ElectionTimeout(t *testing.T) {
	cfg := &Config{
		Enabled:           true,
		ClusterName:       "test",
		HeartbeatInterval: 5 * time.Second,
		HeartbeatTimeout:  30 * time.Second,
		ElectionTimeout:   10 * time.Second, // Less than heartbeat timeout
		Etcd: &EtcdConfig{
			Mode:     EtcdModeEmbedded,
			Embedded: &EtcdEmbeddedConfig{},
		},
	}

	recs := cfg.HARecommendations()

	var foundElectionRec bool
	for _, r := range recs {
		if r.Component == "cluster" && strings.Contains(r.Issue, "Election timeout") {
			foundElectionRec = true
			assert.Equal(t, "warning", r.Level)
		}
	}
	assert.True(t, foundElectionRec, "Should have warning about election timeout")
}

func TestHARecommendations_MultiNodeNoTLS(t *testing.T) {
	cfg := &Config{
		Enabled:           true,
		ClusterName:       "test",
		HeartbeatInterval: 5 * time.Second,
		HeartbeatTimeout:  30 * time.Second,
		ElectionTimeout:   15 * time.Second,
		Etcd: &EtcdConfig{
			Mode: EtcdModeEmbedded,
			Embedded: &EtcdEmbeddedConfig{
				InitialCluster: "kscore-1=http://h1:2380,kscore-2=http://h2:2380,kscore-3=http://h3:2380",
				TLS:            nil, // No TLS
			},
		},
	}

	recs := cfg.HARecommendations()

	var foundTLSRec bool
	for _, r := range recs {
		if r.Component == "tls" && strings.Contains(r.Issue, "Multi-node cluster without TLS") {
			foundTLSRec = true
			assert.Equal(t, "critical", r.Level)
		}
	}
	assert.True(t, foundTLSRec, "Should have critical recommendation about TLS for multi-node cluster")
}

func TestValidateHARequirements(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr string
	}{
		{
			name:    "disabled clustering",
			config:  &Config{Enabled: false},
			wantErr: "clustering is not enabled",
		},
		{
			name: "missing etcd",
			config: &Config{
				Enabled: true,
				Etcd:    nil,
			},
			wantErr: "etcd configuration is required",
		},
		{
			name: "single node embedded",
			config: &Config{
				Enabled: true,
				Etcd: &EtcdConfig{
					Mode: EtcdModeEmbedded,
					Embedded: &EtcdEmbeddedConfig{
						InitialCluster: "kscore-1=http://192.168.1.10:2380",
					},
				},
			},
			wantErr: "minimum 3 members required",
		},
		{
			name: "two node embedded",
			config: &Config{
				Enabled: true,
				Etcd: &EtcdConfig{
					Mode: EtcdModeEmbedded,
					Embedded: &EtcdEmbeddedConfig{
						InitialCluster: "kscore-1=http://h1:2380,kscore-2=http://h2:2380",
					},
				},
			},
			wantErr: "minimum 3 members required",
		},
		{
			name: "three node embedded - valid",
			config: &Config{
				Enabled: true,
				Etcd: &EtcdConfig{
					Mode: EtcdModeEmbedded,
					Embedded: &EtcdEmbeddedConfig{
						InitialCluster: "kscore-1=http://h1:2380,kscore-2=http://h2:2380,kscore-3=http://h3:2380",
					},
				},
			},
			wantErr: "",
		},
		{
			name: "external etcd - too few endpoints",
			config: &Config{
				Enabled: true,
				Etcd: &EtcdConfig{
					Mode:      EtcdModeExternal,
					Endpoints: []string{"localhost:2379"},
				},
			},
			wantErr: "minimum 3 etcd endpoints",
		},
		{
			name: "external etcd - valid",
			config: &Config{
				Enabled: true,
				Etcd: &EtcdConfig{
					Mode:      EtcdModeExternal,
					Endpoints: []string{"host1:2379", "host2:2379", "host3:2379"},
				},
			},
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.ValidateHARequirements()
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
