package cluster

import (
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

// AddressFamilyPreference specifies IPv4/IPv6 preference for cluster communication.
type AddressFamilyPreference int

const (
	// PreferIPv4 prefers IPv4 addresses but falls back to IPv6 if unavailable.
	PreferIPv4 AddressFamilyPreference = iota
	// PreferIPv6 prefers IPv6 addresses but falls back to IPv4 if unavailable.
	PreferIPv6
	// IPv4Only uses only IPv4 addresses.
	IPv4Only
	// IPv6Only uses only IPv6 addresses.
	IPv6Only
)

// String returns the string representation of the preference.
func (p AddressFamilyPreference) String() string {
	switch p {
	case PreferIPv4:
		return "prefer_ipv4"
	case PreferIPv6:
		return "prefer_ipv6"
	case IPv4Only:
		return "ipv4_only"
	case IPv6Only:
		return "ipv6_only"
	default:
		return "unknown"
	}
}

// ParseAddressFamilyPreference parses a string to AddressFamilyPreference.
func ParseAddressFamilyPreference(s string) (AddressFamilyPreference, error) {
	switch strings.ToLower(s) {
	case "prefer_ipv4", "preferipv4", "prefer-ipv4":
		return PreferIPv4, nil
	case "prefer_ipv6", "preferipv6", "prefer-ipv6":
		return PreferIPv6, nil
	case "ipv4_only", "ipv4only", "ipv4-only", "ipv4":
		return IPv4Only, nil
	case "ipv6_only", "ipv6only", "ipv6-only", "ipv6":
		return IPv6Only, nil
	default:
		return PreferIPv4, fmt.Errorf("unknown address family preference: %s", s)
	}
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Enabled:                 false,
		MemberID:                "",
		MemberName:              "",
		ClusterName:             "keystone-core",
		AdvertiseAddress:        "",
		GRPCPort:                9090,
		HeartbeatInterval:       5 * time.Second,
		HeartbeatTimeout:        30 * time.Second,
		ElectionTimeout:         15 * time.Second,
		QuorumSize:              0, // Auto-calculated as N/2 + 1
		AddressFamilyPreference: PreferIPv4,
		Etcd:                    DefaultEtcdConfig(),
	}
}

// Config contains cluster configuration settings.
type Config struct {
	// Enabled indicates if clustering is enabled.
	// When false, the server runs in standalone mode.
	Enabled bool `yaml:"enabled" mapstructure:"enabled"`

	// MemberID is the unique identifier for this cluster member.
	// If empty, a UUID will be generated.
	MemberID string `yaml:"member_id" mapstructure:"member_id"`

	// MemberName is a human-readable name for this member.
	// If empty, defaults to hostname.
	MemberName string `yaml:"member_name" mapstructure:"member_name"`

	// ClusterName is the name of the cluster.
	// All members must use the same cluster name.
	ClusterName string `yaml:"cluster_name" mapstructure:"cluster_name"`

	// AdvertiseAddress is the address other members use to reach this member.
	// If empty, the system attempts to detect a suitable address.
	AdvertiseAddress string `yaml:"advertise_address" mapstructure:"advertise_address"`

	// GRPCPort is the port for gRPC communication between members.
	GRPCPort int `yaml:"grpc_port" mapstructure:"grpc_port"`

	// HeartbeatInterval is how often to send heartbeats.
	HeartbeatInterval time.Duration `yaml:"heartbeat_interval" mapstructure:"heartbeat_interval"`

	// HeartbeatTimeout is how long before a member is considered dead.
	// Should be at least 3x HeartbeatInterval.
	HeartbeatTimeout time.Duration `yaml:"heartbeat_timeout" mapstructure:"heartbeat_timeout"`

	// ElectionTimeout is how long to wait for leader election.
	ElectionTimeout time.Duration `yaml:"election_timeout" mapstructure:"election_timeout"`

	// QuorumSize is the minimum number of members required for quorum.
	// If 0, auto-calculated as N/2 + 1 where N is the cluster size.
	QuorumSize int `yaml:"quorum_size" mapstructure:"quorum_size"`

	// AddressFamilyPreference specifies IPv4/IPv6 preference for cluster communication.
	// Default is PreferIPv4.
	AddressFamilyPreference AddressFamilyPreference `yaml:"address_family" mapstructure:"address_family"`

	// Etcd contains etcd-specific configuration.
	Etcd *EtcdConfig `yaml:"etcd" mapstructure:"etcd"`
}

// Validate checks if the configuration is valid.
func (c *Config) Validate() error {
	if !c.Enabled {
		return nil // No validation needed for standalone mode
	}

	if c.ClusterName == "" {
		return fmt.Errorf("cluster: cluster_name is required")
	}

	if c.HeartbeatInterval <= 0 {
		return fmt.Errorf("cluster: heartbeat_interval must be positive")
	}

	if c.HeartbeatTimeout < c.HeartbeatInterval*3 {
		return fmt.Errorf("cluster: heartbeat_timeout should be at least 3x heartbeat_interval")
	}

	if c.ElectionTimeout <= 0 {
		return fmt.Errorf("cluster: election_timeout must be positive")
	}

	if c.GRPCPort <= 0 || c.GRPCPort > 65535 {
		return fmt.Errorf("cluster: grpc_port must be between 1 and 65535")
	}

	if c.QuorumSize < 0 {
		return fmt.Errorf("cluster: quorum_size cannot be negative")
	}

	if c.Etcd == nil {
		return fmt.Errorf("cluster: etcd configuration is required")
	}

	return c.Etcd.Validate()
}

// GetAdvertiseAddress returns the advertise address, detecting it if necessary.
// Respects AddressFamilyPreference for IPv4/IPv6 selection.
func (c *Config) GetAdvertiseAddress() (string, error) {
	if c.AdvertiseAddress != "" {
		return c.AdvertiseAddress, nil
	}

	// Try to detect a suitable address based on preference
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", fmt.Errorf("failed to get interface addresses: %w", err)
	}

	var ipv4Addrs []string
	var ipv6Addrs []string

	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok || ipNet.IP.IsLoopback() {
			continue
		}

		// Skip link-local addresses
		if ipNet.IP.IsLinkLocalUnicast() {
			continue
		}

		if ipNet.IP.To4() != nil {
			ipv4Addrs = append(ipv4Addrs, ipNet.IP.String())
		} else if ipNet.IP.To16() != nil {
			ipv6Addrs = append(ipv6Addrs, ipNet.IP.String())
		}
	}

	switch c.AddressFamilyPreference {
	case IPv4Only:
		if len(ipv4Addrs) > 0 {
			return ipv4Addrs[0], nil
		}
		return "", fmt.Errorf("no IPv4 advertise address found (ipv4_only mode)")

	case IPv6Only:
		if len(ipv6Addrs) > 0 {
			return ipv6Addrs[0], nil
		}
		return "", fmt.Errorf("no IPv6 advertise address found (ipv6_only mode)")

	case PreferIPv6:
		if len(ipv6Addrs) > 0 {
			return ipv6Addrs[0], nil
		}
		if len(ipv4Addrs) > 0 {
			return ipv4Addrs[0], nil
		}

	case PreferIPv4:
		fallthrough
	default:
		if len(ipv4Addrs) > 0 {
			return ipv4Addrs[0], nil
		}
		if len(ipv6Addrs) > 0 {
			return ipv6Addrs[0], nil
		}
	}

	return "", fmt.Errorf("no suitable advertise address found")
}

// GetGRPCAddress returns the full gRPC address (host:port).
// IPv6 addresses are properly formatted with brackets.
func (c *Config) GetGRPCAddress() (string, error) {
	addr, err := c.GetAdvertiseAddress()
	if err != nil {
		return "", err
	}
	return FormatHostPort(addr, c.GRPCPort), nil
}

// FormatHostPort formats an address and port for use in URLs.
// IPv6 addresses are wrapped in brackets: [::1]:port
// IPv4 addresses are returned as-is: 192.168.1.1:port
func FormatHostPort(host string, port int) string {
	if IsIPv6Address(host) {
		return fmt.Sprintf("[%s]:%d", host, port)
	}
	return fmt.Sprintf("%s:%d", host, port)
}

// IsIPv6Address returns true if the address is an IPv6 address.
func IsIPv6Address(addr string) bool {
	// Remove brackets if present
	addr = strings.TrimPrefix(addr, "[")
	addr = strings.TrimSuffix(addr, "]")

	ip := net.ParseIP(addr)
	if ip == nil {
		return false
	}
	return ip.To4() == nil && ip.To16() != nil
}

// FormatEtcdURL formats an address and port as an etcd URL.
// Uses http:// scheme by default, with proper IPv6 bracket formatting.
func FormatEtcdURL(host string, port int, useTLS bool) string {
	scheme := "http"
	if useTLS {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s", scheme, FormatHostPort(host, port))
}

// DefaultEtcdConfig returns default etcd configuration.
func DefaultEtcdConfig() *EtcdConfig {
	return &EtcdConfig{
		Mode:              EtcdModeEmbedded,
		Endpoints:         []string{"localhost:2379"},
		DialTimeout:       5 * time.Second,
		RequestTimeout:    10 * time.Second,
		AutoSyncInterval:  30 * time.Second,
		MaxRetries:        3,
		RetryInterval:     time.Second,
		LeasesTTL:         15,
		Embedded:          DefaultEtcdEmbeddedConfig(),
		TLS:               nil, // No TLS by default
		KeyPrefix:         "/keystone-core",
		Username:          "",
		Password:          "",
		PermitWithoutAuth: true,
	}
}

// EtcdConfig contains etcd client and server configuration.
type EtcdConfig struct {
	// Mode is the etcd deployment mode (embedded or external).
	Mode EtcdMode `yaml:"mode" mapstructure:"mode"`

	// Endpoints is the list of etcd endpoints for external mode.
	// Ignored in embedded mode.
	Endpoints []string `yaml:"endpoints" mapstructure:"endpoints"`

	// DialTimeout is the timeout for establishing a connection.
	DialTimeout time.Duration `yaml:"dial_timeout" mapstructure:"dial_timeout"`

	// RequestTimeout is the timeout for etcd requests.
	RequestTimeout time.Duration `yaml:"request_timeout" mapstructure:"request_timeout"`

	// AutoSyncInterval is how often to sync the etcd cluster membership.
	AutoSyncInterval time.Duration `yaml:"auto_sync_interval" mapstructure:"auto_sync_interval"`

	// MaxRetries is the maximum number of retries for failed operations.
	MaxRetries int `yaml:"max_retries" mapstructure:"max_retries"`

	// RetryInterval is the interval between retries.
	RetryInterval time.Duration `yaml:"retry_interval" mapstructure:"retry_interval"`

	// LeasesTTL is the TTL for etcd leases in seconds.
	LeasesTTL int64 `yaml:"leases_ttl" mapstructure:"leases_ttl"`

	// Embedded contains embedded etcd configuration.
	// Only used when Mode is "embedded".
	Embedded *EtcdEmbeddedConfig `yaml:"embedded" mapstructure:"embedded"`

	// TLS contains TLS configuration for etcd connections.
	TLS *EtcdTLSConfig `yaml:"tls" mapstructure:"tls"`

	// KeyPrefix is the prefix for all keys stored in etcd.
	KeyPrefix string `yaml:"key_prefix" mapstructure:"key_prefix"`

	// Username for etcd authentication.
	Username string `yaml:"username" mapstructure:"username"`

	// Password for etcd authentication.
	Password string `yaml:"password" mapstructure:"password"`

	// PermitWithoutAuth allows connections without authentication.
	PermitWithoutAuth bool `yaml:"permit_without_auth" mapstructure:"permit_without_auth"`
}

// Validate checks if the etcd configuration is valid.
func (c *EtcdConfig) Validate() error {
	switch c.Mode {
	case EtcdModeEmbedded:
		if c.Embedded == nil {
			return fmt.Errorf("cluster: embedded etcd configuration is required when mode is 'embedded'")
		}
		return c.Embedded.Validate()

	case EtcdModeExternal:
		if len(c.Endpoints) == 0 {
			return fmt.Errorf("cluster: at least one etcd endpoint is required when mode is 'external'")
		}
		for _, ep := range c.Endpoints {
			if ep == "" {
				return fmt.Errorf("cluster: etcd endpoint cannot be empty")
			}
		}

	default:
		return fmt.Errorf("cluster: invalid etcd mode: %s", c.Mode)
	}

	if c.DialTimeout <= 0 {
		return fmt.Errorf("cluster: dial_timeout must be positive")
	}

	if c.RequestTimeout <= 0 {
		return fmt.Errorf("cluster: request_timeout must be positive")
	}

	if c.LeasesTTL < 5 {
		return fmt.Errorf("cluster: leases_ttl must be at least 5 seconds")
	}

	if c.KeyPrefix == "" {
		return fmt.Errorf("cluster: key_prefix is required")
	}

	if c.TLS != nil {
		if err := c.TLS.Validate(); err != nil {
			return err
		}
	}

	return nil
}

// DefaultEtcdEmbeddedConfig returns default embedded etcd configuration.
func DefaultEtcdEmbeddedConfig() *EtcdEmbeddedConfig {
	return &EtcdEmbeddedConfig{
		DataDir:              "data/etcd",
		ListenAddress:        "", // Empty means localhost (IPv4)
		AdvertiseAddress:     "", // Empty means auto-detect
		ClientPort:           2379,
		PeerPort:             2380,
		InitialCluster:       "",
		InitialClusterState:  "new",
		InitialClusterToken:  "keystone-core-cluster",
		MaxSnapFiles:         5,
		MaxWALFiles:          5,
		QuotaBackendBytes:    2 * 1024 * 1024 * 1024, // 2GB
		AutoCompactionMode:   "periodic",
		AutoCompactionPeriod: time.Hour,
		EnablePProf:          false,
		LogLevel:             "warn",
	}
}

// EtcdEmbeddedConfig contains configuration for embedded etcd.
type EtcdEmbeddedConfig struct {
	// DataDir is the directory for etcd data.
	DataDir string `yaml:"data_dir" mapstructure:"data_dir"`

	// ListenAddress is the address to listen on for client and peer connections.
	// If empty, defaults to "localhost" (IPv4).
	// Use "0.0.0.0" to listen on all IPv4 interfaces.
	// Use "::" to listen on all IPv6 interfaces.
	// Use "[::1]" to listen on IPv6 loopback only.
	ListenAddress string `yaml:"listen_address" mapstructure:"listen_address"`

	// AdvertiseAddress is the address advertised to other cluster members.
	// If empty, uses the same as ListenAddress.
	// For IPv6, should include brackets: "[2001:db8::1]"
	AdvertiseAddress string `yaml:"advertise_address" mapstructure:"advertise_address"`

	// ClientPort is the port for client connections.
	ClientPort int `yaml:"client_port" mapstructure:"client_port"`

	// PeerPort is the port for peer connections.
	PeerPort int `yaml:"peer_port" mapstructure:"peer_port"`

	// InitialCluster is the initial cluster configuration.
	// Format: "name1=http://host1:2380,name2=http://host2:2380"
	// If empty, starts as a single-node cluster.
	InitialCluster string `yaml:"initial_cluster" mapstructure:"initial_cluster"`

	// InitialClusterState is "new" for new clusters or "existing" for joining.
	InitialClusterState string `yaml:"initial_cluster_state" mapstructure:"initial_cluster_state"`

	// InitialClusterToken is a unique token for the cluster.
	InitialClusterToken string `yaml:"initial_cluster_token" mapstructure:"initial_cluster_token"`

	// MaxSnapFiles is the maximum number of snapshot files to retain.
	MaxSnapFiles uint `yaml:"max_snap_files" mapstructure:"max_snap_files"`

	// MaxWALFiles is the maximum number of WAL files to retain.
	MaxWALFiles uint `yaml:"max_wal_files" mapstructure:"max_wal_files"`

	// QuotaBackendBytes is the backend size limit in bytes.
	QuotaBackendBytes int64 `yaml:"quota_backend_bytes" mapstructure:"quota_backend_bytes"`

	// AutoCompactionMode is "periodic" or "revision".
	AutoCompactionMode string `yaml:"auto_compaction_mode" mapstructure:"auto_compaction_mode"`

	// AutoCompactionPeriod is the compaction interval for periodic mode.
	AutoCompactionPeriod time.Duration `yaml:"auto_compaction_period" mapstructure:"auto_compaction_period"`

	// EnablePProf enables pprof endpoints.
	EnablePProf bool `yaml:"enable_pprof" mapstructure:"enable_pprof"`

	// LogLevel is the etcd log level.
	LogLevel string `yaml:"log_level" mapstructure:"log_level"`
}

// Validate checks if the embedded etcd configuration is valid.
func (c *EtcdEmbeddedConfig) Validate() error {
	if c.DataDir == "" {
		return fmt.Errorf("cluster: embedded etcd data_dir is required")
	}

	if c.ClientPort <= 0 || c.ClientPort > 65535 {
		return fmt.Errorf("cluster: embedded etcd client_port must be between 1 and 65535")
	}

	if c.PeerPort <= 0 || c.PeerPort > 65535 {
		return fmt.Errorf("cluster: embedded etcd peer_port must be between 1 and 65535")
	}

	if c.ClientPort == c.PeerPort {
		return fmt.Errorf("cluster: embedded etcd client_port and peer_port must be different")
	}

	if c.InitialClusterState != "new" && c.InitialClusterState != "existing" {
		return fmt.Errorf("cluster: initial_cluster_state must be 'new' or 'existing'")
	}

	if c.QuotaBackendBytes < 100*1024*1024 {
		return fmt.Errorf("cluster: quota_backend_bytes must be at least 100MB")
	}

	return nil
}

// GetListenAddress returns the listen address, defaulting to localhost if empty.
func (c *EtcdEmbeddedConfig) GetListenAddress() string {
	if c.ListenAddress != "" {
		return c.ListenAddress
	}
	return "localhost"
}

// GetAdvertiseAddress returns the advertise address, defaulting to listen address.
func (c *EtcdEmbeddedConfig) GetAdvertiseAddress() string {
	if c.AdvertiseAddress != "" {
		return c.AdvertiseAddress
	}
	return c.GetListenAddress()
}

// GetClientListenURL returns the client listen URL with proper IPv6 formatting.
func (c *EtcdEmbeddedConfig) GetClientListenURL(useTLS bool) string {
	return FormatEtcdURL(c.GetListenAddress(), c.ClientPort, useTLS)
}

// GetClientAdvertiseURL returns the client advertise URL with proper IPv6 formatting.
func (c *EtcdEmbeddedConfig) GetClientAdvertiseURL(useTLS bool) string {
	return FormatEtcdURL(c.GetAdvertiseAddress(), c.ClientPort, useTLS)
}

// GetPeerListenURL returns the peer listen URL with proper IPv6 formatting.
func (c *EtcdEmbeddedConfig) GetPeerListenURL(useTLS bool) string {
	return FormatEtcdURL(c.GetListenAddress(), c.PeerPort, useTLS)
}

// GetPeerAdvertiseURL returns the peer advertise URL with proper IPv6 formatting.
func (c *EtcdEmbeddedConfig) GetPeerAdvertiseURL(useTLS bool) string {
	return FormatEtcdURL(c.GetAdvertiseAddress(), c.PeerPort, useTLS)
}

// GetClientEndpoint returns the client endpoint (host:port) for connecting to this server.
func (c *EtcdEmbeddedConfig) GetClientEndpoint() string {
	return FormatHostPort(c.GetAdvertiseAddress(), c.ClientPort)
}

// EtcdTLSConfig contains TLS configuration for etcd.
type EtcdTLSConfig struct {
	// Enabled indicates if TLS is enabled.
	Enabled bool `yaml:"enabled" mapstructure:"enabled"`

	// CertFile is the path to the client certificate.
	CertFile string `yaml:"cert_file" mapstructure:"cert_file"`

	// KeyFile is the path to the client key.
	KeyFile string `yaml:"key_file" mapstructure:"key_file"`

	// CAFile is the path to the CA certificate.
	CAFile string `yaml:"ca_file" mapstructure:"ca_file"`

	// ServerName is the expected server name for verification.
	ServerName string `yaml:"server_name" mapstructure:"server_name"`

	// InsecureSkipVerify disables certificate verification.
	// WARNING: Only use for testing.
	InsecureSkipVerify bool `yaml:"insecure_skip_verify" mapstructure:"insecure_skip_verify"`
}

// Validate checks if the TLS configuration is valid.
func (c *EtcdTLSConfig) Validate() error {
	if !c.Enabled {
		return nil
	}

	if c.CertFile == "" {
		return fmt.Errorf("cluster: tls cert_file is required when TLS is enabled")
	}

	if c.KeyFile == "" {
		return fmt.Errorf("cluster: tls key_file is required when TLS is enabled")
	}

	// Block InsecureSkipVerify in production - this is a critical security vulnerability
	// that allows man-in-the-middle attacks on cluster communication.
	// Only allow when KSCORE_ALLOW_INSECURE_TLS=1 is set (for development/testing only).
	if c.InsecureSkipVerify {
		if os.Getenv("KSCORE_ALLOW_INSECURE_TLS") != "1" {
			return fmt.Errorf("cluster: insecure_skip_verify is not allowed in production (allows MITM attacks). " +
				"Set KSCORE_ALLOW_INSECURE_TLS=1 to override for development/testing only")
		}
	}

	if c.CAFile == "" && !c.InsecureSkipVerify {
		return fmt.Errorf("cluster: tls ca_file is required when TLS is enabled and insecure_skip_verify is false")
	}

	return nil
}

// CalculateQuorumSize returns the quorum size for a given cluster size.
// Quorum is N/2 + 1 where N is the number of members.
func CalculateQuorumSize(memberCount int) int {
	if memberCount <= 0 {
		return 0
	}
	return (memberCount / 2) + 1
}

// CanTolerateFailures returns the number of failures a cluster can tolerate.
func CanTolerateFailures(memberCount int) int {
	if memberCount <= 0 {
		return 0
	}
	return (memberCount - 1) / 2
}
