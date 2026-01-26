// Package proxy implements proxy agent support for managing devices that cannot
// run native Keystone Core agents.
package proxy

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the proxy agent configuration.
type Config struct {
	// Agent configures the proxy agent itself.
	Agent AgentConfig `yaml:"agent"`

	// NATS configures NATS connectivity.
	NATS NATSConfig `yaml:"nats"`

	// Health configures health checking.
	Health HealthConfig `yaml:"health"`

	// Devices defines the managed devices.
	Devices []DeviceConfig `yaml:"devices"`

	// Credentials configures credential providers.
	Credentials CredentialsConfig `yaml:"credentials"`
}

// AgentConfig configures the proxy agent.
type AgentConfig struct {
	// ID is the unique identifier for this proxy agent.
	ID string `yaml:"id"`

	// ClusterName is the Keystone Core cluster name.
	ClusterName string `yaml:"cluster_name"`

	// Labels are key-value pairs for targeting.
	Labels map[string]string `yaml:"labels"`
}

// NATSConfig configures NATS connectivity.
type NATSConfig struct {
	// URLs are NATS server URLs.
	URLs []string `yaml:"urls"`

	// TLS configures TLS for NATS.
	TLS TLSConfig `yaml:"tls"`

	// CredentialsFile is the path to NATS credentials.
	CredentialsFile string `yaml:"credentials_file"`
}

// TLSConfig configures TLS.
type TLSConfig struct {
	// Enabled enables TLS.
	Enabled bool `yaml:"enabled"`

	// CertFile is the path to the client certificate.
	CertFile string `yaml:"cert_file"`

	// KeyFile is the path to the client key.
	KeyFile string `yaml:"key_file"`

	// CAFile is the path to the CA certificate.
	CAFile string `yaml:"ca_file"`

	// InsecureSkipVerify skips certificate verification.
	InsecureSkipVerify bool `yaml:"insecure_skip_verify"`
}

// HealthConfig configures health checking.
type HealthConfig struct {
	// Interval is how often to check device health.
	Interval Duration `yaml:"interval"`

	// Timeout is the timeout for health checks.
	Timeout Duration `yaml:"timeout"`

	// MaxConcurrent limits concurrent health checks.
	MaxConcurrent int `yaml:"max_concurrent"`

	// StaleThreshold is how long before a device is stale.
	StaleThreshold Duration `yaml:"stale_threshold"`
}

// DeviceConfig defines a device to manage.
type DeviceConfig struct {
	// ID is the unique identifier for this device.
	ID string `yaml:"id"`

	// Name is a human-readable name.
	Name string `yaml:"name"`

	// Type is the device type.
	Type string `yaml:"type"`

	// Vendor is the device vendor.
	Vendor string `yaml:"vendor"`

	// Model is the device model.
	Model string `yaml:"model"`

	// Protocol is the communication protocol.
	Protocol string `yaml:"protocol"`

	// Address is the network address.
	Address string `yaml:"address"`

	// Port is the port number (0 = default).
	Port int `yaml:"port"`

	// ProfileID is the device profile to use.
	ProfileID string `yaml:"profile_id"`

	// CredentialRef references a credential.
	CredentialRef string `yaml:"credential_ref"`

	// Metadata contains device-specific metadata.
	Metadata map[string]string `yaml:"metadata"`

	// Labels are key-value pairs for targeting.
	Labels map[string]string `yaml:"labels"`

	// HealthCheckInterval overrides the default interval.
	HealthCheckInterval Duration `yaml:"health_check_interval"`
}

// CredentialsConfig configures credential providers.
type CredentialsConfig struct {
	// Provider is the credential provider type.
	Provider string `yaml:"provider"`

	// Vault configures HashiCorp Vault.
	Vault VaultCredentialConfig `yaml:"vault"`

	// Kubernetes configures Kubernetes secrets.
	Kubernetes K8sCredentialConfig `yaml:"kubernetes"`

	// File configures file-based credentials.
	File FileCredentialConfig `yaml:"file"`
}

// VaultCredentialConfig configures Vault credential provider.
type VaultCredentialConfig struct {
	// Address is the Vault server address.
	Address string `yaml:"address"`

	// Token is the Vault token.
	Token string `yaml:"token"`

	// Path is the secret path prefix.
	Path string `yaml:"path"`

	// TLS configures TLS for Vault.
	TLS TLSConfig `yaml:"tls"`
}

// K8sCredentialConfig configures Kubernetes credential provider.
type K8sCredentialConfig struct {
	// Namespace is the namespace for secrets.
	Namespace string `yaml:"namespace"`

	// LabelSelector filters secrets by labels.
	LabelSelector string `yaml:"label_selector"`
}

// FileCredentialConfig configures file-based credential provider.
type FileCredentialConfig struct {
	// Path is the path to the credentials file.
	Path string `yaml:"path"`

	// EncryptionKey is the key for decrypting credentials.
	EncryptionKey string `yaml:"encryption_key"`
}

// Duration is a YAML-friendly time.Duration.
type Duration time.Duration

// UnmarshalYAML implements yaml.Unmarshaler.
func (d *Duration) UnmarshalYAML(node *yaml.Node) error {
	var s string
	if err := node.Decode(&s); err != nil {
		return err
	}
	parsed, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	*d = Duration(parsed)
	return nil
}

// MarshalYAML implements yaml.Marshaler.
func (d Duration) MarshalYAML() (interface{}, error) {
	return time.Duration(d).String(), nil
}

// Duration returns the time.Duration value.
func (d Duration) Duration() time.Duration {
	return time.Duration(d)
}

// LoadConfig loads a proxy agent configuration from a file.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	return ParseConfig(data)
}

// ParseConfig parses a proxy agent configuration from YAML.
func ParseConfig(data []byte) (*Config, error) {
	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	if err := config.Validate(); err != nil {
		return nil, err
	}

	return &config, nil
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.Agent.ID == "" {
		return fmt.Errorf("agent.id is required")
	}

	if len(c.NATS.URLs) == 0 {
		return fmt.Errorf("nats.urls is required")
	}

	// Validate devices
	deviceIDs := make(map[string]bool)
	for i, device := range c.Devices {
		if device.ID == "" {
			return fmt.Errorf("devices[%d].id is required", i)
		}
		if deviceIDs[device.ID] {
			return fmt.Errorf("duplicate device ID: %s", device.ID)
		}
		deviceIDs[device.ID] = true

		if device.Address == "" {
			return fmt.Errorf("devices[%d].address is required", i)
		}
		if device.Protocol == "" {
			return fmt.Errorf("devices[%d].protocol is required", i)
		}
		if !ProtocolType(device.Protocol).Valid() {
			return fmt.Errorf("devices[%d].protocol is invalid: %s", i, device.Protocol)
		}
		if device.Type != "" && !DeviceType(device.Type).Valid() {
			return fmt.Errorf("devices[%d].type is invalid: %s", i, device.Type)
		}
	}

	return nil
}

// ToManagerConfig converts the configuration to a ManagerConfig.
func (c *Config) ToManagerConfig() *ManagerConfig {
	mc := DefaultManagerConfig()
	mc.AgentID = c.Agent.ID
	mc.ClusterName = c.Agent.ClusterName

	if c.Health.Interval.Duration() > 0 {
		mc.HealthCheckInterval = c.Health.Interval.Duration()
	}
	if c.Health.Timeout.Duration() > 0 {
		mc.HealthCheckTimeout = c.Health.Timeout.Duration()
	}
	if c.Health.MaxConcurrent > 0 {
		mc.MaxConcurrentHealthChecks = c.Health.MaxConcurrent
	}
	if c.Health.StaleThreshold.Duration() > 0 {
		mc.StaleDeviceThreshold = c.Health.StaleThreshold.Duration()
	}

	return mc
}

// ToProxiedDevices converts device configurations to ProxiedDevice objects.
func (c *Config) ToProxiedDevices() []*ProxiedDevice {
	devices := make([]*ProxiedDevice, 0, len(c.Devices))

	for _, dc := range c.Devices {
		device := &ProxiedDevice{
			ID:            dc.ID,
			ProxyAgentID:  c.Agent.ID,
			Name:          dc.Name,
			Type:          DeviceType(dc.Type),
			Vendor:        dc.Vendor,
			Model:         dc.Model,
			Protocol:      ProtocolType(dc.Protocol),
			Address:       dc.Address,
			Port:          dc.Port,
			ProfileID:     dc.ProfileID,
			CredentialRef: dc.CredentialRef,
			Metadata:      dc.Metadata,
			Labels:        dc.Labels,
			Status:        DeviceStatusUnknown,
		}

		if dc.HealthCheckInterval.Duration() > 0 {
			device.HealthCheckInterval = dc.HealthCheckInterval.Duration()
		}

		// Set default type if not specified
		if device.Type == "" {
			device.Type = DeviceTypeCustom
		}

		devices = append(devices, device)
	}

	return devices
}

// DefaultConfig returns a configuration with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Agent: AgentConfig{
			ClusterName: "default",
		},
		Health: HealthConfig{
			Interval:       Duration(30 * time.Second),
			Timeout:        Duration(10 * time.Second),
			MaxConcurrent:  10,
			StaleThreshold: Duration(5 * time.Minute),
		},
	}
}
