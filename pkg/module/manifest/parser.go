package manifest

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Parse parses a module.yaml file
func Parse(data []byte) (*Manifest, error) {
	var manifest Manifest

	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidYAML, err)
	}

	if err := manifest.Validate(); err != nil {
		return nil, err
	}

	return &manifest, nil
}

// ParseFile parses a module.yaml file from disk
func ParseFile(filename string) (*Manifest, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest: %w", err)
	}

	return Parse(data)
}

// ParseLockFile parses a module.lock file
func ParseLockFile(data []byte) (*LockFile, error) {
	var lockfile LockFile

	if err := yaml.Unmarshal(data, &lockfile); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidYAML, err)
	}

	if lockfile.SchemaVersion != 1 {
		return nil, ErrInvalidSchemaVersion
	}

	return &lockfile, nil
}

// ParseLockFileFromFile parses a module.lock file from disk
func ParseLockFileFromFile(filename string) (*LockFile, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read lockfile: %w", err)
	}

	return ParseLockFile(data)
}

// Marshal converts a manifest to YAML bytes
func Marshal(manifest *Manifest) ([]byte, error) {
	return yaml.Marshal(manifest)
}

// MarshalLockFile converts a lockfile to YAML bytes
func MarshalLockFile(lockfile *LockFile) ([]byte, error) {
	return yaml.Marshal(lockfile)
}

// WriteFile writes a manifest to a file
func WriteFile(filename string, manifest *Manifest) error {
	data, err := Marshal(manifest)
	if err != nil {
		return fmt.Errorf("failed to marshal manifest: %w", err)
	}

	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("failed to write manifest: %w", err)
	}

	return nil
}

// WriteLockFile writes a lockfile to a file
func WriteLockFile(filename string, lockfile *LockFile) error {
	data, err := MarshalLockFile(lockfile)
	if err != nil {
		return fmt.Errorf("failed to marshal lockfile: %w", err)
	}

	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("failed to write lockfile: %w", err)
	}

	return nil
}
