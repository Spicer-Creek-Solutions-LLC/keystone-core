// SPDX-License-Identifier: Apache-2.0

package manifest

import (
	"fmt"
	"strings"

	yaml "go.yaml.in/yaml/v3"

	"go.keystone-core.io/keystone-core/pkg/semver"
)

// LockFileSchemaVersion is the only schema this codec accepts.
const LockFileSchemaVersion = 1

// LockedModule pins a resolved dependency to an exact version +
// content hash (reproducible installs).
type LockedModule struct {
	Version string `yaml:"version"`
	Hash    string `yaml:"hash"` // "sha256:<hex>"
}

// LockFile is the resolver output (`module.lock`). The Modules map
// marshals with deterministically sorted keys (go.yaml.in/yaml/v3
// sorts map keys), so re-resolution with the same inputs yields a
// byte-identical lockfile — the reproducibility acceptance line.
type LockFile struct {
	SchemaVersion int                     `yaml:"schema_version"`
	Modules       map[string]LockedModule `yaml:"modules"`
}

// MarshalLockFile renders lf as YAML (stable: sorted module keys).
func MarshalLockFile(lf *LockFile) ([]byte, error) {
	if lf == nil {
		return nil, fmt.Errorf("lockfile: nil lockfile")
	}
	return yaml.Marshal(lf)
}

// UnmarshalLockFile parses a lockfile from YAML (no validation —
// call Validate separately).
func UnmarshalLockFile(b []byte) (*LockFile, error) {
	var lf LockFile
	if err := yaml.Unmarshal(b, &lf); err != nil {
		return nil, fmt.Errorf("lockfile: parse: %w", err)
	}
	return &lf, nil
}

// Validate checks the schema version and every pinned entry.
func (lf *LockFile) Validate() error {
	if lf == nil {
		return fmt.Errorf("lockfile: nil lockfile")
	}
	if lf.SchemaVersion != LockFileSchemaVersion {
		return fmt.Errorf("lockfile: schema_version %d unsupported (want %d)",
			lf.SchemaVersion, LockFileSchemaVersion)
	}
	for name, lm := range lf.Modules {
		if !nameRE.MatchString(name) {
			return fmt.Errorf("lockfile: module name %q must be namespaced vendor/pkg", name)
		}
		if _, err := semver.Parse(lm.Version); err != nil {
			return fmt.Errorf("lockfile: %q version %q invalid semver: %w", name, lm.Version, err)
		}
		if !strings.HasPrefix(lm.Hash, "sha256:") || len(lm.Hash) != len("sha256:")+64 {
			return fmt.Errorf("lockfile: %q hash %q must be sha256:<64 hex>", name, lm.Hash)
		}
	}
	return nil
}
