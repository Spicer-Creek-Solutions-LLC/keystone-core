// Package manifest provides module manifest parsing and validation for
// module metadata, capabilities, and dependency declarations.
package manifest

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Manifest represents a module's metadata
type Manifest struct {
	Name               string                       `yaml:"name"`
	Version            string                       `yaml:"version"`
	Type               string                       `yaml:"type"`
	Entrypoint         string                       `yaml:"entrypoint"`
	Capabilities       []string                     `yaml:"-"` // Populated from CapabilitiesRaw
	CapabilitiesConfig map[string]*CapabilityConfig `yaml:"-"` // Detailed config per capability
	Limits             Limits                       `yaml:"limits"`
	Dependencies       map[string]string            `yaml:"dependencies,omitempty"`
	Description        string                       `yaml:"description,omitempty"`
	Author             string                       `yaml:"author,omitempty"`
	License            string                       `yaml:"license,omitempty"`

	// CapabilitiesRaw stores the raw YAML for flexible parsing
	CapabilitiesRaw yaml.Node `yaml:"capabilities"`
}

// CapabilityConfig holds fine-grained restrictions for a capability
type CapabilityConfig struct {
	// Filesystem restrictions (fs.read, fs.write)
	AllowedPaths []string `yaml:"allowed_paths,omitempty"`
	DeniedPaths  []string `yaml:"denied_paths,omitempty"`
	MaxFileSize  int64    `yaml:"max_file_size,omitempty"` // bytes

	// HTTP restrictions (http.get, http.post)
	AllowedDomains  []string      `yaml:"allowed_domains,omitempty"`
	DeniedDomains   []string      `yaml:"denied_domains,omitempty"`
	MaxResponseSize int64         `yaml:"max_response_size,omitempty"` // bytes
	MaxRequestSize  int64         `yaml:"max_request_size,omitempty"`  // bytes
	RateLimit       int           `yaml:"rate_limit,omitempty"`        // requests per minute
	Timeout         time.Duration `yaml:"timeout,omitempty"`

	// Exec restrictions
	AllowedCommands []string      `yaml:"allowed_commands,omitempty"`
	WorkingDir      string        `yaml:"working_dir,omitempty"`
	ExecTimeout     time.Duration `yaml:"exec_timeout,omitempty"`

	// Secrets restrictions (secrets.read, secrets.write)
	AllowedSecretPaths []string `yaml:"allowed_secret_paths,omitempty"`
	DeniedSecretPaths  []string `yaml:"denied_secret_paths,omitempty"`

	// KV restrictions
	Namespace    string `yaml:"namespace,omitempty"`
	MaxKeySize   int    `yaml:"max_key_size,omitempty"`
	MaxValueSize int    `yaml:"max_value_size,omitempty"`

	// Log restrictions
	MaxLogRate int `yaml:"max_log_rate,omitempty"` // logs per minute
}

// Limits define resource constraints
type Limits struct {
	Timeout time.Duration `yaml:"timeout"`
	Memory  string        `yaml:"memory"`
	CPU     float64       `yaml:"cpu,omitempty"`
}

// LockFile represents pinned dependencies
type LockFile struct {
	SchemaVersion int                     `yaml:"schema_version"`
	Modules       map[string]LockedModule `yaml:"modules"`
}

// LockedModule represents a locked module version
type LockedModule struct {
	Version string `yaml:"version"`
	Hash    string `yaml:"hash"`
}

// LoadManifest loads a manifest from a file
func LoadManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest: %w", err)
	}

	var manifest Manifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
	}

	// Parse capabilities from raw YAML (supports both list and map formats)
	if err := manifest.parseCapabilities(); err != nil {
		return nil, fmt.Errorf("failed to parse capabilities: %w", err)
	}

	return &manifest, nil
}

// parseCapabilities parses the capabilities field which can be either:
// - A simple list: ["fs.read", "exec"]
// - A detailed map: {fs.read: {allowed_paths: ["/app/**"]}, exec: {allowed_commands: ["/bin/cat"]}}
func (m *Manifest) parseCapabilities() error {
	m.Capabilities = []string{}
	m.CapabilitiesConfig = make(map[string]*CapabilityConfig)

	// Handle empty capabilities (Kind 0 is DocumentNode/empty, Kind 8 is ScalarNode for null/empty)
	if m.CapabilitiesRaw.Kind == 0 {
		return nil
	}

	// Handle null/empty scalar (capabilities: null or capabilities: ~)
	if m.CapabilitiesRaw.Kind == yaml.ScalarNode {
		// Empty, null, or ~ means no capabilities
		val := m.CapabilitiesRaw.Value
		if val == "" || val == "null" || val == "~" {
			return nil
		}
		return fmt.Errorf("capabilities must be a list or map, got scalar value: %q", val)
	}

	switch m.CapabilitiesRaw.Kind {
	case yaml.SequenceNode:
		// Simple list format: ["fs.read", "exec"]
		var caps []string
		if err := m.CapabilitiesRaw.Decode(&caps); err != nil {
			return fmt.Errorf("failed to decode capabilities list: %w", err)
		}
		m.Capabilities = caps
		// Create default (permissive) config for each
		for _, cap := range caps {
			m.CapabilitiesConfig[cap] = defaultCapabilityConfig(cap)
		}

	case yaml.MappingNode:
		// Detailed map format: {fs.read: {allowed_paths: [...]}}
		var capsMap map[string]*CapabilityConfig
		if err := m.CapabilitiesRaw.Decode(&capsMap); err != nil {
			// Try decoding as map with null values (just capability names as keys)
			var capsNames map[string]interface{}
			if err2 := m.CapabilitiesRaw.Decode(&capsNames); err2 != nil {
				return fmt.Errorf("failed to decode capabilities map: %w", err)
			}
			// Treat as simple list of names
			for name := range capsNames {
				m.Capabilities = append(m.Capabilities, name)
				m.CapabilitiesConfig[name] = defaultCapabilityConfig(name)
			}
			return nil
		}
		for name, config := range capsMap {
			m.Capabilities = append(m.Capabilities, name)
			if config == nil {
				config = defaultCapabilityConfig(name)
			} else {
				// Apply defaults for unspecified fields
				applyCapabilityDefaults(name, config)
			}
			m.CapabilitiesConfig[name] = config
		}

	default:
		return fmt.Errorf("capabilities must be a list or map, got %v", m.CapabilitiesRaw.Kind)
	}

	return nil
}

// defaultCapabilityConfig returns a permissive default config for a capability
// This maintains backwards compatibility with modules that don't specify restrictions
func defaultCapabilityConfig(capName string) *CapabilityConfig {
	config := &CapabilityConfig{}
	applyCapabilityDefaults(capName, config)
	return config
}

// applyCapabilityDefaults applies sensible defaults to a capability config
func applyCapabilityDefaults(capName string, config *CapabilityConfig) {
	switch capName {
	case "fs.read":
		if len(config.AllowedPaths) == 0 {
			config.AllowedPaths = []string{"**"} // All paths (backwards compat)
		}
		if config.MaxFileSize == 0 {
			config.MaxFileSize = 10 * 1024 * 1024 // 10MB
		}
	case "fs.write":
		if len(config.AllowedPaths) == 0 {
			config.AllowedPaths = []string{"**"}
		}
	case "http.get", "http.post":
		if len(config.AllowedDomains) == 0 {
			config.AllowedDomains = []string{"*"} // All domains
		}
		if config.Timeout == 0 {
			config.Timeout = 30 * time.Second
		}
		if config.MaxResponseSize == 0 {
			config.MaxResponseSize = 10 * 1024 * 1024 // 10MB
		}
		if config.RateLimit == 0 {
			config.RateLimit = 100 // 100 req/min
		}
		if capName == "http.post" && config.MaxRequestSize == 0 {
			config.MaxRequestSize = 1 * 1024 * 1024 // 1MB
		}
	case "exec":
		if len(config.AllowedCommands) == 0 {
			config.AllowedCommands = []string{"*"} // All commands (backwards compat)
		}
		if config.ExecTimeout == 0 {
			config.ExecTimeout = 30 * time.Second
		}
	case "secrets.read", "secrets.write":
		if len(config.AllowedSecretPaths) == 0 {
			config.AllowedSecretPaths = []string{"**"}
		}
	case "kv":
		if config.Namespace == "" {
			config.Namespace = "default"
		}
	case "log":
		if config.MaxLogRate == 0 {
			config.MaxLogRate = 1000 // 1000 logs/min
		}
	}
}

// GetCapabilityConfig returns the config for a specific capability
func (m *Manifest) GetCapabilityConfig(capName string) *CapabilityConfig {
	if config, ok := m.CapabilitiesConfig[capName]; ok {
		return config
	}
	return nil
}

// HasCapability checks if the manifest declares a specific capability
func (m *Manifest) HasCapability(capName string) bool {
	for _, cap := range m.Capabilities {
		if cap == capName {
			return true
		}
	}
	return false
}

// Validate checks if the manifest is valid
func (m *Manifest) Validate() error {
	if m.Name == "" {
		return fmt.Errorf("manifest missing required field: name")
	}
	if m.Version == "" {
		return fmt.Errorf("manifest missing required field: version")
	}
	if m.Type != "starlark" && m.Type != "wasm" {
		return fmt.Errorf("invalid module type: %s", m.Type)
	}
	return nil
}

// ToYAML serializes the manifest to YAML
func (m *Manifest) ToYAML() ([]byte, error) {
	// Ensure CapabilitiesRaw is populated from Capabilities for serialization
	if len(m.Capabilities) > 0 && m.CapabilitiesRaw.Kind == 0 {
		m.CapabilitiesRaw = yaml.Node{
			Kind:    yaml.SequenceNode,
			Tag:     "!!seq",
			Content: make([]*yaml.Node, len(m.Capabilities)),
		}
		for i, cap := range m.Capabilities {
			m.CapabilitiesRaw.Content[i] = &yaml.Node{
				Kind:  yaml.ScalarNode,
				Tag:   "!!str",
				Value: cap,
			}
		}
	}
	return yaml.Marshal(m)
}

// SaveManifest saves a manifest to a file
func (m *Manifest) SaveManifest(path string) error {
	data, err := m.ToYAML()
	if err != nil {
		return fmt.Errorf("failed to serialize manifest: %w", err)
	}
	//nolint:gosec // G306: module manifests need to be readable by module resolver
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("failed to write manifest: %w", err)
	}
	return nil
}
