// Package upgrade provides tools for creating, verifying, and applying
// upgrade packages for air-gapped Keystone Core deployments.
package upgrade

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/shawnbutts/keystone-core/internal/airgap/bootstrap"
)

// SchemaVersion is the current upgrade manifest format version.
const SchemaVersion = "1.0"

// Manifest describes an upgrade package for air-gapped deployments.
type Manifest struct {
	SchemaVersion        string                     `json:"schema_version"`
	FromVersion          string                     `json:"from_version"`
	ToVersion            string                     `json:"to_version"`
	Platform             bootstrap.Platform         `json:"platform"`
	Created              time.Time                  `json:"created"`
	CreatedBy            string                     `json:"created_by,omitempty"`
	BreakingChanges      []string                   `json:"breaking_changes,omitempty"`
	Components           []bootstrap.ComponentEntry `json:"components"`
	Modules              []bootstrap.ContentEntry   `json:"modules,omitempty"`
	Migrations           []MigrationEntry           `json:"migrations,omitempty"`
	PreScripts           []ScriptEntry              `json:"pre_scripts,omitempty"`
	PostScripts          []ScriptEntry              `json:"post_scripts,omitempty"`
	ConfigChanges        []ConfigChange             `json:"config_changes,omitempty"`
	Checksum             string                     `json:"checksum"`
	RequiresVerification bool                       `json:"requires_verification"`
}

// MigrationEntry describes a database migration in the upgrade package.
type MigrationEntry struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Description string `json:"description,omitempty"`
	Order       int    `json:"order"`
}

// ScriptEntry describes a pre- or post-upgrade script.
type ScriptEntry struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required"`
}

// ConfigChange describes a configuration key change between versions.
type ConfigChange struct {
	Key         string `json:"key"`
	OldDefault  string `json:"old_default,omitempty"`
	NewDefault  string `json:"new_default,omitempty"`
	Description string `json:"description,omitempty"`
	Breaking    bool   `json:"breaking"`
}

var sha256Regex = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Validate checks the manifest for required fields and consistency.
func (m *Manifest) Validate() error {
	if m.SchemaVersion == "" {
		return fmt.Errorf("schema_version is required")
	}
	if m.FromVersion == "" {
		return fmt.Errorf("from_version is required")
	}
	if m.ToVersion == "" {
		return fmt.Errorf("to_version is required")
	}
	if err := m.Platform.Validate(); err != nil {
		return fmt.Errorf("platform: %w", err)
	}
	if len(m.Components) == 0 {
		return fmt.Errorf("at least one component is required")
	}

	seen := make(map[string]bool)
	for i, c := range m.Components {
		if c.Name == "" {
			return fmt.Errorf("component[%d]: name is required", i)
		}
		if c.Path == "" {
			return fmt.Errorf("component[%d] %q: path is required", i, c.Name)
		}
		if err := validatePath(c.Path); err != nil {
			return fmt.Errorf("component[%d] %q: %w", i, c.Name, err)
		}
		if c.SHA256 != "" && !sha256Regex.MatchString(c.SHA256) {
			return fmt.Errorf("component[%d] %q: invalid sha256 format", i, c.Name)
		}
		if seen[c.Name] {
			return fmt.Errorf("component[%d] %q: duplicate name", i, c.Name)
		}
		seen[c.Name] = true
	}

	for i, entry := range m.Modules {
		if entry.Path == "" {
			return fmt.Errorf("module[%d]: path is required", i)
		}
		if err := validatePath(entry.Path); err != nil {
			return fmt.Errorf("module[%d]: %w", i, err)
		}
	}

	for i, mig := range m.Migrations {
		if mig.Name == "" {
			return fmt.Errorf("migration[%d]: name is required", i)
		}
		if mig.Path == "" {
			return fmt.Errorf("migration[%d] %q: path is required", i, mig.Name)
		}
		if err := validatePath(mig.Path); err != nil {
			return fmt.Errorf("migration[%d] %q: %w", i, mig.Name, err)
		}
	}

	for i, s := range m.PreScripts {
		if s.Path == "" {
			return fmt.Errorf("pre_script[%d]: path is required", i)
		}
		if err := validatePath(s.Path); err != nil {
			return fmt.Errorf("pre_script[%d]: %w", i, err)
		}
	}
	for i, s := range m.PostScripts {
		if s.Path == "" {
			return fmt.Errorf("post_script[%d]: path is required", i)
		}
		if err := validatePath(s.Path); err != nil {
			return fmt.Errorf("post_script[%d]: %w", i, err)
		}
	}

	if m.Checksum != "" && !sha256Regex.MatchString(m.Checksum) {
		return fmt.Errorf("invalid checksum format")
	}

	return nil
}

func validatePath(p string) error {
	if filepath.IsAbs(p) {
		return fmt.Errorf("path %q must be relative", p)
	}
	if p == ".." || (len(p) > 2 && p[:3] == "../") {
		return fmt.Errorf("path %q escapes package root", p)
	}
	return nil
}

// WriteManifest serializes the upgrade manifest to a JSON file.
func WriteManifest(m *Manifest, path string) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling manifest: %w", err)
	}
	//nolint:gosec // G306: manifest must be readable for verification
	return os.WriteFile(path, data, 0o644)
}

// ReadManifest reads and parses an upgrade manifest from a JSON file.
func ReadManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: caller controls path
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}
	return &m, nil
}
