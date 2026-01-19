package lockfile

import (
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

// SchemaHandler handles parsing for a specific schema version
type SchemaHandler interface {
	// Parse parses lock file data for this schema version
	Parse(data []byte) (*LockFile, error)

	// Version returns the schema version this handler supports
	Version() int
}

// Migrator migrates a lock file between versions
type Migrator interface {
	// Migrate performs the migration
	Migrate(lf *LockFile) error

	// Describe returns a description of changes made
	Describe() []string

	// FromVersion returns the source version
	FromVersion() int

	// ToVersion returns the target version
	ToVersion() int
}

// migrators stores available migrators
var migrators = map[string]Migrator{
	"1-2": &migratorV1ToV2{},
}

// getMigrator returns a migrator for the given version transition
func getMigrator(from, to int) (Migrator, bool) {
	key := fmt.Sprintf("%d-%d", from, to)
	m, ok := migrators[key]
	return m, ok
}

// ============================================================================
// Schema V1 Handler
// ============================================================================

// schemaV1Handler handles schema version 1 (legacy format)
type schemaV1Handler struct{}

func (h *schemaV1Handler) Version() int {
	return 1
}

// lockFileV1 represents the legacy v1 lock file format
type lockFileV1 struct {
	SchemaVersion int                    `yaml:"schema_version"`
	Modules       map[string]lockedModuleV1 `yaml:"modules"`
}

type lockedModuleV1 struct {
	Version string `yaml:"version"`
	Hash    string `yaml:"hash"`
}

func (h *schemaV1Handler) Parse(data []byte) (*LockFile, error) {
	var v1 lockFileV1
	if err := yaml.Unmarshal(data, &v1); err != nil {
		return nil, fmt.Errorf("failed to parse v1 lock file: %w", err)
	}

	// Convert to current format
	lf := &LockFile{
		SchemaVersion: 1,
		Metadata: &LockFileMetadata{
			GeneratedAt: time.Now(),
			Comment:     "Migrated from schema v1",
		},
		Modules: make(map[string]*LockedModule),
		Checksums: &ChecksumBlock{
			Algorithm: "sha256",
			Entries:   make(map[string]string),
		},
	}

	for name, module := range v1.Modules {
		lf.Modules[name] = &LockedModule{
			Version: module.Version,
			Hash:    module.Hash,
		}
		if module.Hash != "" {
			key := fmt.Sprintf("%s@%s", name, module.Version)
			lf.Checksums.Entries[key] = module.Hash
		}
	}

	return lf, nil
}

// ============================================================================
// Schema V2 Handler
// ============================================================================

// schemaV2Handler handles schema version 2 (current format)
type schemaV2Handler struct{}

func (h *schemaV2Handler) Version() int {
	return 2
}

func (h *schemaV2Handler) Parse(data []byte) (*LockFile, error) {
	var lf LockFile
	if err := yaml.Unmarshal(data, &lf); err != nil {
		return nil, fmt.Errorf("failed to parse v2 lock file: %w", err)
	}

	// Ensure maps are initialized
	if lf.Modules == nil {
		lf.Modules = make(map[string]*LockedModule)
	}
	if lf.Checksums == nil {
		lf.Checksums = &ChecksumBlock{
			Algorithm: "sha256",
			Entries:   make(map[string]string),
		}
	}
	if lf.Checksums.Entries == nil {
		lf.Checksums.Entries = make(map[string]string)
	}

	return &lf, nil
}

// ============================================================================
// V1 to V2 Migrator
// ============================================================================

// migratorV1ToV2 migrates from schema v1 to v2
type migratorV1ToV2 struct{}

func (m *migratorV1ToV2) FromVersion() int { return 1 }
func (m *migratorV1ToV2) ToVersion() int   { return 2 }

func (m *migratorV1ToV2) Migrate(lf *LockFile) error {
	// Add metadata if missing
	if lf.Metadata == nil {
		lf.Metadata = &LockFileMetadata{
			GeneratedAt: time.Now(),
			Comment:     "Migrated from schema v1",
		}
	}

	// Add checksums block if missing
	if lf.Checksums == nil {
		lf.Checksums = &ChecksumBlock{
			Algorithm: "sha256",
			Entries:   make(map[string]string),
		}
	}

	// Populate checksums from module hashes
	for name, module := range lf.Modules {
		if module.Hash != "" {
			key := fmt.Sprintf("%s@%s", name, module.Version)
			lf.Checksums.Entries[key] = module.Hash
		}

		// Ensure module has all v2 fields initialized
		if module.Dependencies == nil {
			module.Dependencies = make(map[string]string)
		}
	}

	return nil
}

func (m *migratorV1ToV2) Describe() []string {
	return []string{
		"Added metadata block with generation timestamp",
		"Added checksums block for integrity verification",
		"Added support for module dependencies tracking",
		"Added support for module source information",
		"Added support for deprecation markers",
		"Added support for module replacements",
	}
}

// ============================================================================
// Migration Utilities
// ============================================================================

// MigrateFile loads, migrates, and saves a lock file
func MigrateFile(path string) (*MigrationResult, error) {
	lf, err := Load(path)
	if err != nil {
		return nil, fmt.Errorf("failed to load lock file: %w", err)
	}

	result, err := lf.Migrate()
	if err != nil {
		return result, fmt.Errorf("migration failed: %w", err)
	}

	if !result.Skipped {
		if err := lf.Save(path); err != nil {
			return result, fmt.Errorf("failed to save migrated lock file: %w", err)
		}
	}

	return result, nil
}

// CanMigrate checks if a lock file can be migrated
func CanMigrate(path string) (bool, int, error) {
	lf, err := Load(path)
	if err != nil {
		return false, 0, err
	}

	if lf.SchemaVersion >= CurrentSchemaVersion {
		return false, lf.SchemaVersion, nil
	}

	// Check that all required migrators exist
	for v := lf.SchemaVersion; v < CurrentSchemaVersion; v++ {
		if _, ok := getMigrator(v, v+1); !ok {
			return false, lf.SchemaVersion, fmt.Errorf("no migrator for v%d to v%d", v, v+1)
		}
	}

	return true, lf.SchemaVersion, nil
}

// GetMigrationPath returns the sequence of versions for migration
func GetMigrationPath(fromVersion int) []int {
	if fromVersion >= CurrentSchemaVersion {
		return nil
	}

	path := make([]int, 0, CurrentSchemaVersion-fromVersion+1)
	for v := fromVersion; v <= CurrentSchemaVersion; v++ {
		path = append(path, v)
	}
	return path
}

// DescribeMigration returns a description of what migration would do
func DescribeMigration(fromVersion int) ([]string, error) {
	if fromVersion >= CurrentSchemaVersion {
		return nil, nil
	}

	var allChanges []string
	for v := fromVersion; v < CurrentSchemaVersion; v++ {
		migrator, ok := getMigrator(v, v+1)
		if !ok {
			return nil, fmt.Errorf("no migrator for v%d to v%d", v, v+1)
		}
		changes := migrator.Describe()
		allChanges = append(allChanges, fmt.Sprintf("v%d -> v%d:", v, v+1))
		for _, change := range changes {
			allChanges = append(allChanges, "  - "+change)
		}
	}

	return allChanges, nil
}
