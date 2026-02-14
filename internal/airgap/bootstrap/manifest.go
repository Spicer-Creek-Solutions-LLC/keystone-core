package bootstrap

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"
)

// SchemaVersion is the current bootstrap manifest format version.
const SchemaVersion = "1.0"

// Manifest is the top-level manifest for a bootstrap package.
type Manifest struct {
	SchemaVersion        string           `json:"schema_version"`
	Version              string           `json:"version"`
	Platform             Platform         `json:"platform"`
	Created              time.Time        `json:"created"`
	CreatedBy            string           `json:"created_by,omitempty"`
	Components           []ComponentEntry `json:"components"`
	Modules              []ContentEntry   `json:"modules,omitempty"`
	Blueprints           []ContentEntry   `json:"blueprints,omitempty"`
	Policies             []ContentEntry   `json:"policies,omitempty"`
	ConfigTemplates      []ContentEntry   `json:"config_templates,omitempty"`
	Docs                 *ContentEntry    `json:"docs,omitempty"`
	Checksum             string           `json:"checksum"`
	RequiresVerification bool             `json:"requires_verification"`
}

// ComponentEntry describes a binary component in the package.
type ComponentEntry struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Path    string `json:"path"`
	SHA256  string `json:"sha256"`
	Size    int64  `json:"size"`
}

// ContentEntry describes a content item (module, blueprint, policy, etc.) in the package.
type ContentEntry struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
	Path    string `json:"path"`
	SHA256  string `json:"sha256"`
	Size    int64  `json:"size"`
}

var sha256Regex = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Validate checks the manifest for required fields and consistency.
func (m *Manifest) Validate() error {
	if m.SchemaVersion == "" {
		return fmt.Errorf("schema_version is required")
	}
	if m.Version == "" {
		return fmt.Errorf("version is required")
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

	for _, entries := range []struct {
		label string
		items []ContentEntry
	}{
		{"module", m.Modules},
		{"blueprint", m.Blueprints},
		{"policy", m.Policies},
		{"config_template", m.ConfigTemplates},
	} {
		for i, e := range entries.items {
			if e.Path == "" {
				return fmt.Errorf("%s[%d]: path is required", entries.label, i)
			}
			if err := validatePath(e.Path); err != nil {
				return fmt.Errorf("%s[%d]: %w", entries.label, i, err)
			}
			if e.SHA256 != "" && !sha256Regex.MatchString(e.SHA256) {
				return fmt.Errorf("%s[%d]: invalid sha256 format", entries.label, i)
			}
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
	cleaned := filepath.Clean(p)
	if cleaned != p && cleaned+"/" != p {
		return fmt.Errorf("path %q is not clean", p)
	}
	for _, part := range filepath.SplitList(p) {
		if part == ".." {
			return fmt.Errorf("path %q contains parent directory reference", p)
		}
	}
	if len(p) > 2 && p[:3] == "../" {
		return fmt.Errorf("path %q escapes package root", p)
	}
	if p == ".." {
		return fmt.Errorf("path %q escapes package root", p)
	}
	return nil
}

// CalculateChecksum computes the SHA256 checksum of all files in dir,
// excluding manifest.json and the signatures/ directory.
func CalculateChecksum(dir string) (string, error) {
	var paths []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}

		// Skip manifest.json and signatures/
		if rel == "manifest.json" {
			return nil
		}
		if rel == "signatures" || (len(rel) > 11 && rel[:11] == "signatures/") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if !info.IsDir() {
			paths = append(paths, rel)
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("walking directory: %w", err)
	}

	sort.Strings(paths)

	h := sha256.New()
	for _, rel := range paths {
		// Hash the path name
		fmt.Fprintf(h, "%s\n", rel)

		// Hash the file contents
		f, err := os.Open(filepath.Join(dir, rel)) //nolint:gosec // G304: paths come from filepath.Walk within dir
		if err != nil {
			return "", fmt.Errorf("opening %s: %w", rel, err)
		}
		if _, err := io.Copy(h, f); err != nil {
			f.Close()
			return "", fmt.Errorf("hashing %s: %w", rel, err)
		}
		f.Close()
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// HashFile computes the SHA256 hash of a file.
func HashFile(path string) (string, error) {
	f, err := os.Open(path) //nolint:gosec // G304: caller controls path
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// WriteManifest serializes the manifest to a JSON file.
func WriteManifest(manifest *Manifest, path string) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling manifest: %w", err)
	}
	//nolint:gosec // G306: manifest must be readable for verification
	return os.WriteFile(path, data, 0o644)
}

// ReadManifest reads and parses a manifest from a JSON file.
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
