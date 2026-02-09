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

	default: // includes PreferIPv4
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

	// TLS contains TLS configuration for embedded etcd client and peer connections.
	// When enabled, all client and peer communication will be encrypted.
	TLS *EtcdEmbeddedTLSConfig `yaml:"tls" mapstructure:"tls"`
}

// EtcdEmbeddedTLSConfig contains TLS configuration for embedded etcd server.
type EtcdEmbeddedTLSConfig struct {
	// Enabled indicates if TLS is enabled for embedded etcd.
	Enabled bool `yaml:"enabled" mapstructure:"enabled"`

	// ClientCertFile is the path to the certificate for client connections.
	ClientCertFile string `yaml:"client_cert_file" mapstructure:"client_cert_file"`

	// ClientKeyFile is the path to the private key for client connections.
	ClientKeyFile string `yaml:"client_key_file" mapstructure:"client_key_file"`

	// ClientCAFile is the path to the CA certificate for verifying client certificates.
	// If set, client certificate authentication (mTLS) is required.
	ClientCAFile string `yaml:"client_ca_file" mapstructure:"client_ca_file"`

	// PeerCertFile is the path to the certificate for peer connections.
	// If empty, uses ClientCertFile.
	PeerCertFile string `yaml:"peer_cert_file" mapstructure:"peer_cert_file"`

	// PeerKeyFile is the path to the private key for peer connections.
	// If empty, uses ClientKeyFile.
	PeerKeyFile string `yaml:"peer_key_file" mapstructure:"peer_key_file"`

	// PeerCAFile is the path to the CA certificate for verifying peer certificates.
	// If empty, uses ClientCAFile.
	PeerCAFile string `yaml:"peer_ca_file" mapstructure:"peer_ca_file"`

	// ClientCertAuth enables client certificate authentication (mTLS) for client connections.
	ClientCertAuth bool `yaml:"client_cert_auth" mapstructure:"client_cert_auth"`

	// PeerCertAuth enables peer certificate authentication (mTLS) for peer connections.
	// This is strongly recommended for multi-node clusters.
	PeerCertAuth bool `yaml:"peer_cert_auth" mapstructure:"peer_cert_auth"`

	// AutoTLS enables automatic TLS certificate generation.
	// Uses self-signed certificates generated at startup.
	// Useful for development but NOT recommended for production.
	AutoTLS bool `yaml:"auto_tls" mapstructure:"auto_tls"`

	// PeerAutoTLS enables automatic TLS certificate generation for peer connections.
	// Uses self-signed certificates generated at startup.
	// Useful for development but NOT recommended for production.
	PeerAutoTLS bool `yaml:"peer_auto_tls" mapstructure:"peer_auto_tls"`
}

// Validate checks if the embedded TLS configuration is valid.
func (c *EtcdEmbeddedTLSConfig) Validate() error {
	if c == nil || !c.Enabled {
		return nil
	}

	// If AutoTLS is enabled, certificates are auto-generated
	if c.AutoTLS {
		return nil
	}

	// Manual TLS requires cert and key files
	if c.ClientCertFile == "" {
		return fmt.Errorf("cluster: embedded etcd tls client_cert_file is required when TLS is enabled")
	}
	if c.ClientKeyFile == "" {
		return fmt.Errorf("cluster: embedded etcd tls client_key_file is required when TLS is enabled")
	}

	// Verify files exist
	if _, err := os.Stat(c.ClientCertFile); err != nil {
		return fmt.Errorf("cluster: embedded etcd tls client_cert_file not found: %w", err)
	}
	if _, err := os.Stat(c.ClientKeyFile); err != nil {
		return fmt.Errorf("cluster: embedded etcd tls client_key_file not found: %w", err)
	}

	// If client CA is specified, verify it exists
	if c.ClientCAFile != "" {
		if _, err := os.Stat(c.ClientCAFile); err != nil {
			return fmt.Errorf("cluster: embedded etcd tls client_ca_file not found: %w", err)
		}
	}

	// If separate peer certs are specified, verify they exist
	if c.PeerCertFile != "" {
		if _, err := os.Stat(c.PeerCertFile); err != nil {
			return fmt.Errorf("cluster: embedded etcd tls peer_cert_file not found: %w", err)
		}
	}
	if c.PeerKeyFile != "" {
		if _, err := os.Stat(c.PeerKeyFile); err != nil {
			return fmt.Errorf("cluster: embedded etcd tls peer_key_file not found: %w", err)
		}
	}
	if c.PeerCAFile != "" {
		if _, err := os.Stat(c.PeerCAFile); err != nil {
			return fmt.Errorf("cluster: embedded etcd tls peer_ca_file not found: %w", err)
		}
	}

	return nil
}

// GetPeerCertFile returns the peer certificate file, falling back to client cert.
func (c *EtcdEmbeddedTLSConfig) GetPeerCertFile() string {
	if c.PeerCertFile != "" {
		return c.PeerCertFile
	}
	return c.ClientCertFile
}

// GetPeerKeyFile returns the peer key file, falling back to client key.
func (c *EtcdEmbeddedTLSConfig) GetPeerKeyFile() string {
	if c.PeerKeyFile != "" {
		return c.PeerKeyFile
	}
	return c.ClientKeyFile
}

// GetPeerCAFile returns the peer CA file, falling back to client CA.
func (c *EtcdEmbeddedTLSConfig) GetPeerCAFile() string {
	if c.PeerCAFile != "" {
		return c.PeerCAFile
	}
	return c.ClientCAFile
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

	// Validate TLS configuration if present
	if c.TLS != nil {
		if err := c.TLS.Validate(); err != nil {
			return err
		}
	}

	return nil
}

// IsTLSEnabled returns true if TLS is enabled for embedded etcd.
func (c *EtcdEmbeddedConfig) IsTLSEnabled() bool {
	return c.TLS != nil && c.TLS.Enabled
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

	// MinVersion is the minimum TLS version (1.2 or 1.3).
	MinVersion string `yaml:"min_version" mapstructure:"min_version"`
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

	if err := validateTLSMinVersion(c.MinVersion); err != nil {
		return fmt.Errorf("cluster: %w", err)
	}

	return nil
}

func validateTLSMinVersion(value string) error {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "1.3", "tls1.3", "tls13", "1.2", "tls1.2", "tls12":
		return nil
	default:
		return fmt.Errorf("min_version %q is invalid (must be 1.2 or 1.3)", value)
	}
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

// HARecommendation represents a recommendation for HA deployments.
type HARecommendation struct {
	// Component is the subsystem (cluster, etcd, nats, storage, tls)
	Component string `json:"component"`
	// Level is the severity: critical (must fix), warning (should fix), info (best practice)
	Level string `json:"level"`
	// Issue describes the problem
	Issue string `json:"issue"`
	// Recommendation is what the user should do
	Recommendation string `json:"recommendation"`
	// AffectsAvailability indicates if this affects fault tolerance
	AffectsAvailability bool `json:"affects_availability"`
}

// HARecommendations returns recommendations for improving HA configuration.
// These are checked at runtime when cluster.enabled=true.
//
// Level meanings:
//   - critical: Configuration prevents proper HA operation
//   - warning: Configuration degrades HA capabilities
//   - info: Best practice that improves HA
//
// Call this after Validate() passes and log recommendations during startup.
func (c *Config) HARecommendations() []HARecommendation {
	var recs []HARecommendation

	if !c.Enabled {
		return recs // No HA checks needed for standalone mode
	}

	// Check for even number of cluster members (from InitialCluster)
	if c.Etcd != nil && c.Etcd.Embedded != nil && c.Etcd.Embedded.InitialCluster != "" {
		memberCount := countClusterMembers(c.Etcd.Embedded.InitialCluster)
		if memberCount > 0 {
			// Even number check
			if memberCount%2 == 0 {
				recs = append(recs, HARecommendation{
					Component:           "cluster",
					Level:               "warning",
					Issue:               fmt.Sprintf("Cluster has %d members (even number). Odd cluster sizes (3, 5, 7) provide better fault tolerance.", memberCount),
					Recommendation:      fmt.Sprintf("Add one more member to have %d nodes. With %d nodes, you can tolerate %d failures. With %d nodes, you could tolerate %d failures.", memberCount+1, memberCount, CanTolerateFailures(memberCount), memberCount+1, CanTolerateFailures(memberCount+1)),
					AffectsAvailability: true,
				})
			}

			// Minimum cluster size check
			switch memberCount {
			case 1:
				recs = append(recs, HARecommendation{
					Component:           "cluster",
					Level:               "critical",
					Issue:               "Single-node cluster provides no fault tolerance. Any node failure results in complete outage.",
					Recommendation:      "Add at least 2 more members to form a 3-node cluster that can tolerate 1 failure.",
					AffectsAvailability: true,
				})
			case 2:
				recs = append(recs, HARecommendation{
					Component:           "cluster",
					Level:               "critical",
					Issue:               "2-node cluster provides no fault tolerance (requires majority for quorum). Both nodes must be healthy for the cluster to operate.",
					Recommendation:      "Add 1 more member to form a 3-node cluster. A 3-node cluster can tolerate 1 failure.",
					AffectsAvailability: true,
				})
			}
		}
	}

	// Check heartbeat timeout relationship
	if c.HeartbeatTimeout < c.HeartbeatInterval*3 {
		recs = append(recs, HARecommendation{
			Component:           "cluster",
			Level:               "warning",
			Issue:               fmt.Sprintf("Heartbeat timeout (%v) is less than 3x heartbeat interval (%v). This may cause unnecessary leader elections during brief network hiccups.", c.HeartbeatTimeout, c.HeartbeatInterval),
			Recommendation:      fmt.Sprintf("Set heartbeat_timeout to at least %v (3x heartbeat_interval) or increase to 6x for unstable networks.", c.HeartbeatInterval*3),
			AffectsAvailability: true,
		})
	}

	// Check election timeout relationship
	if c.ElectionTimeout < c.HeartbeatTimeout {
		recs = append(recs, HARecommendation{
			Component:           "cluster",
			Level:               "warning",
			Issue:               fmt.Sprintf("Election timeout (%v) is shorter than heartbeat timeout (%v). This can cause premature leader elections.", c.ElectionTimeout, c.HeartbeatTimeout),
			Recommendation:      fmt.Sprintf("Set election_timeout to at least %v (same as heartbeat_timeout) or higher for stable leader elections.", c.HeartbeatTimeout),
			AffectsAvailability: true,
		})
	}

	// Check for embedded etcd in HA mode
	if c.Etcd != nil && c.Etcd.Mode == EtcdModeEmbedded {
		memberCount := 0
		if c.Etcd.Embedded != nil && c.Etcd.Embedded.InitialCluster != "" {
			memberCount = countClusterMembers(c.Etcd.Embedded.InitialCluster)
		}
		if memberCount <= 1 {
			// Single embedded etcd with cluster enabled
			recs = append(recs, HARecommendation{
				Component:           "etcd",
				Level:               "info",
				Issue:               "Using embedded etcd in single-node mode. For production HA, consider multi-node embedded etcd or external etcd cluster.",
				Recommendation:      "Configure initial_cluster with multiple members for embedded multi-node etcd, or use etcd.mode=external with a dedicated etcd cluster.",
				AffectsAvailability: false,
			})
		}
	}

	// Check TLS for multi-node cluster
	if c.Etcd != nil && c.Etcd.Mode == EtcdModeEmbedded {
		memberCount := 0
		if c.Etcd.Embedded != nil && c.Etcd.Embedded.InitialCluster != "" {
			memberCount = countClusterMembers(c.Etcd.Embedded.InitialCluster)
		}
		if memberCount > 1 {
			// Multi-node cluster
			tlsEnabled := c.Etcd.Embedded != nil && c.Etcd.Embedded.TLS != nil && c.Etcd.Embedded.TLS.Enabled
			if !tlsEnabled {
				recs = append(recs, HARecommendation{
					Component:           "tls",
					Level:               "critical",
					Issue:               "Multi-node cluster without TLS. Cluster communication (etcd peer traffic, Raft consensus) is unencrypted.",
					Recommendation:      "Enable TLS with cluster.etcd.embedded.tls.enabled=true. For production, configure proper certificates. For testing, use auto_tls=true.",
					AffectsAvailability: false, // Doesn't affect availability but is a security issue
				})
			}

			// Check for peer cert auth in multi-node
			if tlsEnabled && c.Etcd.Embedded.TLS != nil && !c.Etcd.Embedded.TLS.PeerCertAuth {
				recs = append(recs, HARecommendation{
					Component:           "tls",
					Level:               "warning",
					Issue:               "Peer certificate authentication is disabled. Any node with TLS client cert can join the cluster.",
					Recommendation:      "Enable cluster.etcd.embedded.tls.peer_cert_auth=true to require mutual TLS between cluster members.",
					AffectsAvailability: false,
				})
			}
		}
	}

	// Check quorum configuration
	if c.QuorumSize > 0 {
		memberCount := 0
		if c.Etcd != nil && c.Etcd.Embedded != nil && c.Etcd.Embedded.InitialCluster != "" {
			memberCount = countClusterMembers(c.Etcd.Embedded.InitialCluster)
		}
		if memberCount > 0 {
			expectedQuorum := CalculateQuorumSize(memberCount)
			if c.QuorumSize != expectedQuorum {
				recs = append(recs, HARecommendation{
					Component:           "cluster",
					Level:               "warning",
					Issue:               fmt.Sprintf("Custom quorum_size=%d differs from calculated value %d (N/2+1 for %d members).", c.QuorumSize, expectedQuorum, memberCount),
					Recommendation:      "Set quorum_size=0 for automatic calculation, or ensure custom value is intentional. Non-standard quorum may affect fault tolerance.",
					AffectsAvailability: true,
				})
			}
		}
	}

	return recs
}

// countClusterMembers counts members in an initial cluster string.
// Format: "name1=url1,name2=url2,..."
func countClusterMembers(initialCluster string) int {
	if initialCluster == "" {
		return 0
	}
	// Split by comma to get each member entry
	members := strings.Split(initialCluster, ",")
	count := 0
	for _, m := range members {
		m = strings.TrimSpace(m)
		if m != "" && strings.Contains(m, "=") {
			count++
		}
	}
	return count
}

// ValidateHARequirements returns an error if the configuration cannot
// support HA operation. This is stricter than Validate() and should be
// called when strict HA mode is required.
//
// Returns nil if configuration meets minimum HA requirements.
func (c *Config) ValidateHARequirements() error {
	if !c.Enabled {
		return fmt.Errorf("cluster: clustering is not enabled (cluster.enabled=false)")
	}

	if c.Etcd == nil {
		return fmt.Errorf("cluster: etcd configuration is required for HA")
	}

	// Check minimum cluster size for HA
	switch c.Etcd.Mode {
	case EtcdModeEmbedded:
		if c.Etcd.Embedded == nil || c.Etcd.Embedded.InitialCluster == "" {
			return fmt.Errorf("cluster: initial_cluster must be configured for HA (embedded etcd mode)")
		}
		memberCount := countClusterMembers(c.Etcd.Embedded.InitialCluster)
		if memberCount < 3 {
			return fmt.Errorf("cluster: minimum 3 members required for HA (found %d). 2-node clusters cannot maintain quorum if one node fails", memberCount)
		}
	case EtcdModeExternal:
		if len(c.Etcd.Endpoints) < 3 {
			return fmt.Errorf("cluster: minimum 3 etcd endpoints recommended for HA (found %d)", len(c.Etcd.Endpoints))
		}
	}

	return nil
}

// IsOddClusterSize returns true if the cluster has an odd number of members.
// Odd-sized clusters have better fault tolerance characteristics.
func IsOddClusterSize(memberCount int) bool {
	return memberCount > 0 && memberCount%2 == 1
}

// RecommendedClusterSize returns the recommended cluster size for a given
// desired fault tolerance level.
//
// Examples:
//   - tolerateFailures=1 -> 3 nodes
//   - tolerateFailures=2 -> 5 nodes
//   - tolerateFailures=3 -> 7 nodes
func RecommendedClusterSize(tolerateFailures int) int {
	if tolerateFailures <= 0 {
		return 1
	}
	return (tolerateFailures * 2) + 1
}
