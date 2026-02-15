// Package transfer provides tools for exporting and importing operational data
// across air-gapped boundaries. Export packages are signed, optionally encrypted
// archives containing audit logs, events, state history, and other data suitable
// for transfer via USB, sneakernet, or data diode.
package transfer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

// SchemaVersion is the current export manifest format version.
const SchemaVersion = "1.0"

// ExportType identifies the kind of data in an export package.
type ExportType string

// Export type constants.
const (
	ExportTypeAudit     ExportType = "audit"
	ExportTypeEvents    ExportType = "events"
	ExportTypeState     ExportType = "state"
	ExportTypeInventory ExportType = "inventory"
	ExportTypeFull      ExportType = "full"
)

// TimeRange specifies a time window for data collection.
type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// DataSetInfo describes one data set within an export package.
type DataSetInfo struct {
	Name      string    `json:"name"`
	Path      string    `json:"path"`
	SHA256    string    `json:"sha256"`
	Size      int64     `json:"size"`
	Count     int64     `json:"count"`
	TimeRange TimeRange `json:"time_range"`
}

// ExportManifest is the top-level manifest for an export package.
type ExportManifest struct {
	SchemaVersion        string        `json:"schema_version"`
	ExportType           ExportType    `json:"export_type"`
	Created              time.Time     `json:"created"`
	CreatedBy            string        `json:"created_by,omitempty"`
	TimeRange            TimeRange     `json:"time_range"`
	DataSets             []DataSetInfo `json:"data_sets"`
	Checksum             string        `json:"checksum"`
	Encrypted            bool          `json:"encrypted"`
	EncryptionRecipient  string        `json:"encryption_recipient,omitempty"`
	RequiresVerification bool          `json:"requires_verification"`
}

var sha256Regex = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Validate checks the manifest for required fields and consistency.
func (m *ExportManifest) Validate() error {
	if m.SchemaVersion == "" {
		return fmt.Errorf("schema_version is required")
	}
	if m.ExportType == "" {
		return fmt.Errorf("export_type is required")
	}
	switch m.ExportType {
	case ExportTypeAudit, ExportTypeEvents, ExportTypeState, ExportTypeInventory, ExportTypeFull:
	default:
		return fmt.Errorf("unknown export_type %q", m.ExportType)
	}
	if len(m.DataSets) == 0 {
		return fmt.Errorf("at least one data_set is required")
	}

	seen := make(map[string]bool)
	for i, ds := range m.DataSets {
		if ds.Name == "" {
			return fmt.Errorf("data_set[%d]: name is required", i)
		}
		if ds.Path == "" {
			return fmt.Errorf("data_set[%d] %q: path is required", i, ds.Name)
		}
		if err := validatePath(ds.Path); err != nil {
			return fmt.Errorf("data_set[%d] %q: %w", i, ds.Name, err)
		}
		if ds.SHA256 != "" && !sha256Regex.MatchString(ds.SHA256) {
			return fmt.Errorf("data_set[%d] %q: invalid sha256 format", i, ds.Name)
		}
		if seen[ds.Name] {
			return fmt.Errorf("data_set[%d] %q: duplicate name", i, ds.Name)
		}
		seen[ds.Name] = true
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

// WriteManifest serializes the export manifest to a JSON file.
func WriteManifest(m *ExportManifest, path string) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling manifest: %w", err)
	}
	//nolint:gosec // G306: manifest must be readable for verification
	return os.WriteFile(path, data, 0o644)
}

// ReadManifest reads and parses an export manifest from a JSON file.
func ReadManifest(path string) (*ExportManifest, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: caller controls path
	if err != nil {
		return nil, err
	}
	var m ExportManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}
	return &m, nil
}
