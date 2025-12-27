package manifest

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Manifest represents a module's metadata
type Manifest struct {
	Name         string            `yaml:"name"`
	Version      string            `yaml:"version"`
	Type         string            `yaml:"type"`
	Entrypoint   string            `yaml:"entrypoint"`
	Capabilities []string          `yaml:"capabilities"`
	Limits       Limits            `yaml:"limits"`
	Dependencies map[string]string `yaml:"dependencies,omitempty"`
	Description  string            `yaml:"description,omitempty"`
	Author       string            `yaml:"author,omitempty"`
	License      string            `yaml:"license,omitempty"`
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

	return &manifest, nil
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
