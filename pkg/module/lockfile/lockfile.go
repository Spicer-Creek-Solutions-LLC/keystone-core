// Package lockfile provides version-aware lock file management with migration support
package lockfile

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// CurrentSchemaVersion is the latest lock file schema version
const CurrentSchemaVersion = 2

// SchemaVersions maps version numbers to their handlers
var schemaVersions = map[int]SchemaHandler{
	1: &schemaV1Handler{},
	2: &schemaV2Handler{},
}

// LockFile represents a versioned module lock file
type LockFile struct {
	// SchemaVersion is the lock file format version
	SchemaVersion int `yaml:"schema_version" json:"schema_version"`

	// Metadata contains lock file metadata
	Metadata *Metadata `yaml:"metadata,omitempty" json:"metadata,omitempty"`

	// Modules contains locked module versions
	Modules map[string]*LockedModule `yaml:"modules" json:"modules"`

	// Checksums contains integrity checksums
	Checksums *ChecksumBlock `yaml:"checksums,omitempty" json:"checksums,omitempty"`
}

// Metadata contains metadata about the lock file
type Metadata struct {
	// GeneratedAt is when the lock file was created/updated
	GeneratedAt time.Time `yaml:"generated_at" json:"generated_at"`

	// GeneratedBy is who/what generated the lock file
	GeneratedBy string `yaml:"generated_by,omitempty" json:"generated_by,omitempty"`

	// KeystoneVersion is the version of Keystone that generated the file
	KeystoneVersion string `yaml:"keystone_version,omitempty" json:"keystone_version,omitempty"`

	// Comment is an optional user comment
	Comment string `yaml:"comment,omitempty" json:"comment,omitempty"`

	// RegistryURL is the default registry used
	RegistryURL string `yaml:"registry_url,omitempty" json:"registry_url,omitempty"`

	// RootModule is the root module name (if applicable)
	RootModule string `yaml:"root_module,omitempty" json:"root_module,omitempty"`

	// RootVersion is the root module version
	RootVersion string `yaml:"root_version,omitempty" json:"root_version,omitempty"`
}

// LockedModule represents a single locked module
type LockedModule struct {
	// Version is the locked version
	Version string `yaml:"version" json:"version"`

	// Hash is the primary content hash (h1: prefix for sha256)
	Hash string `yaml:"hash" json:"hash"`

	// Source is where the module was fetched from
	Source *ModuleSource `yaml:"source,omitempty" json:"source,omitempty"`

	// Dependencies are the direct dependencies (name -> version)
	Dependencies map[string]string `yaml:"dependencies,omitempty" json:"dependencies,omitempty"`

	// ResolvedAt is when this module was resolved
	ResolvedAt time.Time `yaml:"resolved_at,omitempty" json:"resolved_at,omitempty"`

	// Deprecated marks if this version is deprecated
	Deprecated bool `yaml:"deprecated,omitempty" json:"deprecated,omitempty"`

	// DeprecationMessage explains why the version is deprecated
	DeprecationMessage string `yaml:"deprecation_message,omitempty" json:"deprecation_message,omitempty"`

	// Replacements holds module replacement information
	Replacement *ModuleReplacement `yaml:"replacement,omitempty" json:"replacement,omitempty"`
}

// ModuleSource describes where a module was fetched from
type ModuleSource struct {
	// Type is the source type (registry, git, local, oci)
	Type string `yaml:"type" json:"type"`

	// URL is the source URL
	URL string `yaml:"url,omitempty" json:"url,omitempty"`

	// Ref is the git ref (branch, tag, commit)
	Ref string `yaml:"ref,omitempty" json:"ref,omitempty"`

	// Subdir is the subdirectory within the source
	Subdir string `yaml:"subdir,omitempty" json:"subdir,omitempty"`
}

// ModuleReplacement describes a module replacement
type ModuleReplacement struct {
	// OriginalModule is the module being replaced
	OriginalModule string `yaml:"original_module" json:"original_module"`

	// OriginalVersion is the version being replaced
	OriginalVersion string `yaml:"original_version,omitempty" json:"original_version,omitempty"`

	// ReplacementPath is the local path (for local replacements)
	ReplacementPath string `yaml:"replacement_path,omitempty" json:"replacement_path,omitempty"`
}

// ChecksumBlock contains integrity checksums for all modules
type ChecksumBlock struct {
	// Algorithm is the checksum algorithm (sha256, sha512)
	Algorithm string `yaml:"algorithm" json:"algorithm"`

	// Entries maps "module@version" to checksum
	Entries map[string]string `yaml:"entries" json:"entries"`
}

// New creates a new lock file with current schema version
func New() *LockFile {
	return &LockFile{
		SchemaVersion: CurrentSchemaVersion,
		Metadata: &Metadata{
			GeneratedAt: time.Now(),
		},
		Modules: make(map[string]*LockedModule),
		Checksums: &ChecksumBlock{
			Algorithm: "sha256",
			Entries:   make(map[string]string),
		},
	}
}

// Load loads a lock file from disk
func Load(path string) (*LockFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, &NotFoundError{Path: path}
		}
		return nil, fmt.Errorf("failed to read lock file: %w", err)
	}

	return Parse(data)
}

// Parse parses lock file data
func Parse(data []byte) (*LockFile, error) {
	// First, detect the schema version
	var versionProbe struct {
		SchemaVersion int `yaml:"schema_version" json:"schema_version"`
	}
	if err := yaml.Unmarshal(data, &versionProbe); err != nil {
		return nil, fmt.Errorf("failed to detect schema version: %w", err)
	}

	version := versionProbe.SchemaVersion
	if version == 0 {
		version = 1 // Default to v1 for legacy files
	}

	handler, ok := schemaVersions[version]
	if !ok {
		return nil, &UnsupportedSchemaError{
			Version:        version,
			LatestVersion:  CurrentSchemaVersion,
			SupportedRange: getSupportedVersionRange(),
		}
	}

	return handler.Parse(data)
}

// Save saves the lock file to disk
func (lf *LockFile) Save(path string) error {
	// Update metadata
	if lf.Metadata == nil {
		lf.Metadata = &Metadata{}
	}
	lf.Metadata.GeneratedAt = time.Now()

	// Ensure checksum block exists
	if lf.Checksums == nil {
		lf.Checksums = &ChecksumBlock{
			Algorithm: "sha256",
			Entries:   make(map[string]string),
		}
	}

	data, err := lf.Marshal()
	if err != nil {
		return fmt.Errorf("failed to marshal lock file: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	//nolint:gosec // G301: lockfile directory needs to be accessible by service user
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	//nolint:gosec // G306: lock files need to be readable by module resolver
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("failed to write lock file: %w", err)
	}

	return nil
}

// Marshal serializes the lock file to YAML
func (lf *LockFile) Marshal() ([]byte, error) {
	// Add header comment
	header := fmt.Sprintf(`# Keystone Core Module Lock File
# Schema Version: %d
# Generated: %s
# DO NOT EDIT - This file is automatically generated
#
# To update dependencies, run: kscore module update
# To verify checksums, run: kscore module verify
`, lf.SchemaVersion, lf.Metadata.GeneratedAt.Format(time.RFC3339))

	data, err := yaml.Marshal(lf)
	if err != nil {
		return nil, err
	}

	return append([]byte(header+"\n"), data...), nil
}

// MarshalJSON implements json.Marshaler
func (lf *LockFile) MarshalJSON() ([]byte, error) {
	type alias LockFile
	return json.MarshalIndent((*alias)(lf), "", "  ")
}

// AddModule adds or updates a module in the lock file
func (lf *LockFile) AddModule(name string, module *LockedModule) {
	if lf.Modules == nil {
		lf.Modules = make(map[string]*LockedModule)
	}
	module.ResolvedAt = time.Now()
	lf.Modules[name] = module

	// Update checksum
	if module.Hash != "" && lf.Checksums != nil {
		key := fmt.Sprintf("%s@%s", name, module.Version)
		lf.Checksums.Entries[key] = module.Hash
	}
}

// RemoveModule removes a module from the lock file
func (lf *LockFile) RemoveModule(name string) {
	if lf.Modules == nil {
		return
	}

	module, ok := lf.Modules[name]
	if ok && lf.Checksums != nil {
		key := fmt.Sprintf("%s@%s", name, module.Version)
		delete(lf.Checksums.Entries, key)
	}

	delete(lf.Modules, name)
}

// GetModule returns a locked module by name
func (lf *LockFile) GetModule(name string) (*LockedModule, bool) {
	if lf.Modules == nil {
		return nil, false
	}
	module, ok := lf.Modules[name]
	return module, ok
}

// HasModule checks if a module is in the lock file
func (lf *LockFile) HasModule(name string) bool {
	_, ok := lf.GetModule(name)
	return ok
}

// ListModules returns all module names sorted alphabetically
func (lf *LockFile) ListModules() []string {
	if lf.Modules == nil {
		return nil
	}

	names := make([]string, 0, len(lf.Modules))
	for name := range lf.Modules {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Validate validates the lock file integrity
func (lf *LockFile) Validate() error {
	if lf.SchemaVersion < 1 || lf.SchemaVersion > CurrentSchemaVersion {
		return &ValidationError{
			Field:   "schema_version",
			Message: fmt.Sprintf("unsupported schema version %d", lf.SchemaVersion),
		}
	}

	for name, module := range lf.Modules {
		if module.Version == "" {
			return &ValidationError{
				Field:   fmt.Sprintf("modules.%s.version", name),
				Message: "version is required",
			}
		}
		if module.Hash == "" {
			return &ValidationError{
				Field:   fmt.Sprintf("modules.%s.hash", name),
				Message: "hash is required",
			}
		}
		if !isValidHash(module.Hash) {
			return &ValidationError{
				Field:   fmt.Sprintf("modules.%s.hash", name),
				Message: fmt.Sprintf("invalid hash format: %s", module.Hash),
			}
		}
	}

	return nil
}

// VerifyChecksums verifies all checksums in the lock file
func (lf *LockFile) VerifyChecksums() []ChecksumMismatch {
	var mismatches []ChecksumMismatch

	if lf.Checksums == nil {
		return mismatches
	}

	for name, module := range lf.Modules {
		key := fmt.Sprintf("%s@%s", name, module.Version)
		expected, ok := lf.Checksums.Entries[key]
		if !ok {
			mismatches = append(mismatches, ChecksumMismatch{
				Module:   name,
				Version:  module.Version,
				Expected: "",
				Actual:   module.Hash,
				Missing:  true,
			})
			continue
		}

		if expected != module.Hash {
			mismatches = append(mismatches, ChecksumMismatch{
				Module:   name,
				Version:  module.Version,
				Expected: expected,
				Actual:   module.Hash,
			})
		}
	}

	return mismatches
}

// NeedsMigration returns true if the lock file needs migration
func (lf *LockFile) NeedsMigration() bool {
	return lf.SchemaVersion < CurrentSchemaVersion
}

// Migrate migrates the lock file to the latest schema version
func (lf *LockFile) Migrate() (*MigrationResult, error) {
	result := &MigrationResult{
		FromVersion: lf.SchemaVersion,
		ToVersion:   CurrentSchemaVersion,
		Steps:       []MigrationStep{},
	}

	if lf.SchemaVersion >= CurrentSchemaVersion {
		result.Skipped = true
		return result, nil
	}

	// Migrate step by step
	for v := lf.SchemaVersion; v < CurrentSchemaVersion; v++ {
		step := MigrationStep{
			FromVersion: v,
			ToVersion:   v + 1,
			StartedAt:   time.Now(),
		}

		migrator, ok := getMigrator(v, v+1)
		if !ok {
			step.Error = fmt.Sprintf("no migrator for v%d to v%d", v, v+1)
			result.Steps = append(result.Steps, step)
			return result, fmt.Errorf("migration failed: %s", step.Error)
		}

		if err := migrator.Migrate(lf); err != nil {
			step.Error = err.Error()
			result.Steps = append(result.Steps, step)
			return result, fmt.Errorf("migration from v%d to v%d failed: %w", v, v+1, err)
		}

		lf.SchemaVersion = v + 1
		step.CompletedAt = time.Now()
		step.Changes = migrator.Describe()
		result.Steps = append(result.Steps, step)
	}

	return result, nil
}

// Diff returns the differences between two lock files
func (lf *LockFile) Diff(other *LockFile) *Diff {
	diff := &Diff{
		Added:     make(map[string]*LockedModule),
		Removed:   make(map[string]*LockedModule),
		Changed:   make(map[string]*ModuleChange),
		Unchanged: []string{},
	}

	// Find added and changed modules
	for name, module := range other.Modules {
		existing, ok := lf.Modules[name]
		switch {
		case !ok:
			diff.Added[name] = module
		case existing.Version != module.Version || existing.Hash != module.Hash:
			diff.Changed[name] = &ModuleChange{
				OldVersion: existing.Version,
				NewVersion: module.Version,
				OldHash:    existing.Hash,
				NewHash:    module.Hash,
			}
		default:
			diff.Unchanged = append(diff.Unchanged, name)
		}
	}

	// Find removed modules
	for name, module := range lf.Modules {
		if _, ok := other.Modules[name]; !ok {
			diff.Removed[name] = module
		}
	}

	return diff
}

// isValidHash checks if a hash string is valid
func isValidHash(hash string) bool {
	// Format: h1:base64hash or sha256:hexhash
	if strings.HasPrefix(hash, "h1:") {
		// h1: format uses base64, minimum reasonable length
		return len(hash) > 10
	}
	if strings.HasPrefix(hash, "sha256:") {
		// sha256: + 64 hex chars = 71 total
		hexPart := strings.TrimPrefix(hash, "sha256:")
		if len(hexPart) != 64 {
			return false
		}
		// Verify all characters are hex
		for _, c := range hexPart {
			if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
				return false
			}
		}
		return true
	}
	return false
}

// ComputeHash computes the hash of module content
func ComputeHash(content []byte) string {
	hash := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(hash[:])
}

// getSupportedVersionRange returns the range of supported versions
func getSupportedVersionRange() string {
	versions := make([]int, 0, len(schemaVersions))
	for v := range schemaVersions {
		versions = append(versions, v)
	}
	sort.Ints(versions)
	if len(versions) == 0 {
		return "none"
	}
	return fmt.Sprintf("%d-%d", versions[0], versions[len(versions)-1])
}
