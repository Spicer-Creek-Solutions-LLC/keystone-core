package bootstrap

import (
	"fmt"
	"net"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// ConfigLoader handles loading and parsing seed configurations
type ConfigLoader struct {
	envPrefix string
}

// NewConfigLoader creates a new configuration loader
func NewConfigLoader() *ConfigLoader {
	return &ConfigLoader{
		envPrefix: "KSCORE_",
	}
}

// LoadSeedConfig loads a seed configuration from a YAML file
func (c *ConfigLoader) LoadSeedConfig(path string) (*SeedConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read seed config: %w", err)
	}

	return c.ParseSeedConfig(data)
}

// ParseSeedConfig parses seed configuration from YAML bytes
func (c *ConfigLoader) ParseSeedConfig(data []byte) (*SeedConfig, error) {
	// Expand environment variables
	expanded := c.expandEnvVars(string(data))

	var config SeedConfig
	if err := yaml.Unmarshal([]byte(expanded), &config); err != nil {
		return nil, fmt.Errorf("failed to parse seed config: %w", err)
	}

	// Apply defaults
	c.applyDefaults(&config)

	return &config, nil
}

// expandEnvVars expands environment variable references in the format ${VAR} or $VAR
func (c *ConfigLoader) expandEnvVars(input string) string {
	// Pattern for ${VAR} or ${VAR:-default}
	re := regexp.MustCompile(`\$\{([^}]+)\}`)
	result := re.ReplaceAllStringFunc(input, func(match string) string {
		// Remove ${ and }
		inner := match[2 : len(match)-1]

		// Check for default value syntax ${VAR:-default}
		if idx := strings.Index(inner, ":-"); idx != -1 {
			varName := inner[:idx]
			defaultVal := inner[idx+2:]
			if val := os.Getenv(varName); val != "" {
				return val
			}
			return defaultVal
		}

		// Check for required variable syntax ${VAR:?error}
		if idx := strings.Index(inner, ":?"); idx != -1 {
			varName := inner[:idx]
			if val := os.Getenv(varName); val != "" {
				return val
			}
			// Return placeholder - validation will catch missing required vars
			return match
		}

		// Simple variable reference
		if val := os.Getenv(inner); val != "" {
			return val
		}
		return match // Leave unexpanded if not found
	})

	// Also handle $VAR format (without braces)
	re2 := regexp.MustCompile(`\$([A-Za-z_][A-Za-z0-9_]*)`)
	result = re2.ReplaceAllStringFunc(result, func(match string) string {
		varName := match[1:]
		if val := os.Getenv(varName); val != "" {
			return val
		}
		return match
	})

	return result
}

// applyDefaults applies default values to the configuration
func (c *ConfigLoader) applyDefaults(config *SeedConfig) {
	// Cluster defaults
	if config.Cluster.Name == "" {
		config.Cluster.Name = "default"
	}

	// Control plane defaults
	if config.ControlPlane.Replicas == 0 {
		config.ControlPlane.Replicas = 1
	}
	if config.ControlPlane.API.Listen == "" {
		config.ControlPlane.API.Listen = ":8080"
	}

	// NATS defaults
	if config.NATS.Mode == "" {
		config.NATS.Mode = NATSModeEmbedded
	}
	if config.NATS.StoreDir == "" {
		config.NATS.StoreDir = "/var/lib/keystone-core/nats"
	}
	if config.NATS.MaxMemory == "" {
		config.NATS.MaxMemory = "1GB"
	}
	if config.NATS.MaxFile == "" {
		config.NATS.MaxFile = "10GB"
	}
	if config.NATS.ClusterPort == 0 {
		config.NATS.ClusterPort = 6222
	}

	// Database defaults
	if config.Database.Type == "" {
		config.Database.Type = DatabaseTypeSQLite
	}
	if config.Database.Type == DatabaseTypeSQLite && config.Database.Path == "" {
		config.Database.Path = "/var/lib/keystone-core/state.db"
	}
	if config.Database.Type == DatabaseTypePostgreSQL {
		if config.Database.Port == 0 {
			config.Database.Port = 5432
		}
		if config.Database.SSLMode == "" {
			config.Database.SSLMode = "prefer"
		}
	}

	// Etcd defaults
	if config.Etcd.Mode == "" {
		config.Etcd.Mode = EtcdModeEmbedded
	}
	if config.Etcd.DataDir == "" {
		config.Etcd.DataDir = "/var/lib/keystone-core/etcd"
	}

	// Post-bootstrap defaults
	if !config.PostBootstrap.ApplyStates && !config.PostBootstrap.VerifyHealth && !config.PostBootstrap.RegisterAgents {
		config.PostBootstrap.VerifyHealth = true
	}

	// Apply defaults to control plane nodes
	for i := range config.ControlPlane.Nodes {
		if config.ControlPlane.Nodes[i].Port == 0 {
			config.ControlPlane.Nodes[i].Port = 8080
		}
		if config.ControlPlane.Nodes[i].Role == "" {
			if i == 0 {
				config.ControlPlane.Nodes[i].Role = NodeRoleLeader
			} else {
				config.ControlPlane.Nodes[i].Role = NodeRoleFollower
			}
		}
	}
}

// ValidationError represents a configuration validation error
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// ValidationErrors is a collection of validation errors
type ValidationErrors []ValidationError

func (e ValidationErrors) Error() string {
	if len(e) == 0 {
		return ""
	}
	var msgs []string
	for _, err := range e {
		msgs = append(msgs, err.Error())
	}
	return fmt.Sprintf("validation failed: %s", strings.Join(msgs, "; "))
}

// ValidateSeedConfig validates a seed configuration
func ValidateSeedConfig(config *SeedConfig) error {
	var errs ValidationErrors

	// Validate cluster
	if config.Cluster.Name == "" {
		errs = append(errs, ValidationError{Field: "cluster.name", Message: "required"})
	}

	// Validate control plane
	if config.ControlPlane.Replicas < 1 {
		errs = append(errs, ValidationError{Field: "control_plane.replicas", Message: "must be at least 1"})
	}
	if config.ControlPlane.Replicas > 1 && len(config.ControlPlane.Nodes) < config.ControlPlane.Replicas {
		errs = append(errs, ValidationError{
			Field:   "control_plane.nodes",
			Message: fmt.Sprintf("need at least %d nodes for %d replicas", config.ControlPlane.Replicas, config.ControlPlane.Replicas),
		})
	}

	// Validate nodes
	for i, node := range config.ControlPlane.Nodes {
		if node.Host == "" {
			errs = append(errs, ValidationError{
				Field:   fmt.Sprintf("control_plane.nodes[%d].host", i),
				Message: "required",
			})
		} else if !isValidHost(node.Host) {
			errs = append(errs, ValidationError{
				Field:   fmt.Sprintf("control_plane.nodes[%d].host", i),
				Message: "must be a valid hostname or IP address",
			})
		}
	}

	// Validate NATS
	switch config.NATS.Mode {
	case NATSModeEmbedded, NATSModeStandalone, NATSModeCluster:
		// Valid modes
	case "":
		// Will use default
	default:
		errs = append(errs, ValidationError{
			Field:   "nats.mode",
			Message: fmt.Sprintf("invalid mode: %s (must be embedded, standalone, or cluster)", config.NATS.Mode),
		})
	}

	if config.NATS.Mode == NATSModeCluster && len(config.NATS.Nodes) < 3 {
		errs = append(errs, ValidationError{
			Field:   "nats.nodes",
			Message: "cluster mode requires at least 3 nodes",
		})
	}

	// Validate database
	switch config.Database.Type {
	case DatabaseTypeSQLite:
		// SQLite is always valid
	case DatabaseTypePostgreSQL:
		if config.Database.Host == "" {
			errs = append(errs, ValidationError{Field: "database.host", Message: "required for postgresql"})
		}
		if config.Database.Name == "" {
			errs = append(errs, ValidationError{Field: "database.name", Message: "required for postgresql"})
		}
		if config.Database.User == "" {
			errs = append(errs, ValidationError{Field: "database.user", Message: "required for postgresql"})
		}
	case "":
		// Will use default
	default:
		errs = append(errs, ValidationError{
			Field:   "database.type",
			Message: fmt.Sprintf("invalid type: %s (must be sqlite or postgresql)", config.Database.Type),
		})
	}

	// Validate etcd
	switch config.Etcd.Mode {
	case EtcdModeEmbedded, EtcdModeExternal:
		// Valid modes
	case "":
		// Will use default
	default:
		errs = append(errs, ValidationError{
			Field:   "etcd.mode",
			Message: fmt.Sprintf("invalid mode: %s (must be embedded or external)", config.Etcd.Mode),
		})
	}

	if config.Etcd.Mode == EtcdModeExternal && len(config.Etcd.Nodes) == 0 {
		errs = append(errs, ValidationError{
			Field:   "etcd.nodes",
			Message: "required for external mode",
		})
	}

	// Validate initial agents
	for i, agent := range config.InitialAgents {
		if agent.Host == "" {
			errs = append(errs, ValidationError{
				Field:   fmt.Sprintf("initial_agents[%d].host", i),
				Message: "required",
			})
		}
	}

	if len(errs) > 0 {
		return errs
	}
	return nil
}

// isValidHost checks if a string is a valid hostname or IP address
func isValidHost(host string) bool {
	// Check if it's a valid IP address
	if net.ParseIP(host) != nil {
		return true
	}

	// Check if it's a valid hostname
	// RFC 1123: hostname can start with a digit
	hostnameRegex := regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$`)
	return hostnameRegex.MatchString(host)
}

// DefaultSeedConfig returns a default seed configuration for single-node deployment
func DefaultSeedConfig() *SeedConfig {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "localhost"
	}

	return &SeedConfig{
		Cluster: ClusterConfig{
			Name:   "default",
			Domain: hostname,
		},
		ControlPlane: ControlPlaneConfig{
			Replicas: 1,
			Nodes: []NodeConfig{
				{
					Host: hostname,
					Port: 8080,
					Role: NodeRoleLeader,
				},
			},
			API: APIConfig{
				Listen: ":8080",
				TLS: TLSConfig{
					Enabled:      true,
					AutoGenerate: true,
				},
			},
		},
		NATS: NATSConfig{
			Mode:      NATSModeEmbedded,
			JetStream: true,
			StoreDir:  "/var/lib/keystone-core/nats",
			MaxMemory: "1GB",
			MaxFile:   "10GB",
		},
		Database: DatabaseConfig{
			Type: DatabaseTypeSQLite,
			Path: "/var/lib/keystone-core/state.db",
		},
		Etcd: EtcdConfig{
			Mode:    EtcdModeEmbedded,
			DataDir: "/var/lib/keystone-core/etcd",
		},
		PostBootstrap: PostBootstrapConfig{
			ApplyStates:    false,
			VerifyHealth:   true,
			RegisterAgents: false,
		},
	}
}

// MergeSeedConfig merges override values into a base configuration
func MergeSeedConfig(base, override *SeedConfig) *SeedConfig {
	result := *base

	// Merge cluster
	if override.Cluster.Name != "" {
		result.Cluster.Name = override.Cluster.Name
	}
	if override.Cluster.Domain != "" {
		result.Cluster.Domain = override.Cluster.Domain
	}

	// Merge control plane
	if override.ControlPlane.Replicas > 0 {
		result.ControlPlane.Replicas = override.ControlPlane.Replicas
	}
	if len(override.ControlPlane.Nodes) > 0 {
		result.ControlPlane.Nodes = override.ControlPlane.Nodes
	}
	if override.ControlPlane.API.Listen != "" {
		result.ControlPlane.API.Listen = override.ControlPlane.API.Listen
	}

	// Merge NATS
	if override.NATS.Mode != "" {
		result.NATS.Mode = override.NATS.Mode
	}
	if len(override.NATS.Nodes) > 0 {
		result.NATS.Nodes = override.NATS.Nodes
	}
	if override.NATS.StoreDir != "" {
		result.NATS.StoreDir = override.NATS.StoreDir
	}
	if override.NATS.MaxMemory != "" {
		result.NATS.MaxMemory = override.NATS.MaxMemory
	}
	if override.NATS.MaxFile != "" {
		result.NATS.MaxFile = override.NATS.MaxFile
	}

	// Merge database
	if override.Database.Type != "" {
		result.Database.Type = override.Database.Type
	}
	if override.Database.Host != "" {
		result.Database.Host = override.Database.Host
	}
	if override.Database.Port > 0 {
		result.Database.Port = override.Database.Port
	}
	if override.Database.Name != "" {
		result.Database.Name = override.Database.Name
	}
	if override.Database.User != "" {
		result.Database.User = override.Database.User
	}
	if override.Database.Path != "" {
		result.Database.Path = override.Database.Path
	}

	// Merge etcd
	if override.Etcd.Mode != "" {
		result.Etcd.Mode = override.Etcd.Mode
	}
	if len(override.Etcd.Nodes) > 0 {
		result.Etcd.Nodes = override.Etcd.Nodes
	}
	if override.Etcd.DataDir != "" {
		result.Etcd.DataDir = override.Etcd.DataDir
	}

	// Merge initial agents
	if len(override.InitialAgents) > 0 {
		result.InitialAgents = override.InitialAgents
	}

	return &result
}

// ExportSeedConfig exports a seed configuration to YAML
func ExportSeedConfig(config *SeedConfig) ([]byte, error) {
	return yaml.Marshal(config)
}
