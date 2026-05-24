// SPDX-License-Identifier: Apache-2.0

package file

import (
	"time"

	"go.keystone-core.io/keystone-core/internal/secrets"
)

// stateSchemaVersion is the cleartext-JSON schema version (distinct
// from the envelope format version). v1.0 = 1; future field
// migrations bump it.
const stateSchemaVersion = 1

// fileState is the JSON shape inside the encrypted envelope.
// Marshalled at every mutation and re-encrypted as a whole; realistic
// trial-scale deployments hold hundreds of secrets and the rewrite
// cost is dwarfed by fsync.
type fileState struct {
	Version int                       `json:"version"`
	Secrets map[string]*storedSecret  `json:"secrets"`
}

// newFileState returns an empty state at the current schema version.
func newFileState() *fileState {
	return &fileState{
		Version: stateSchemaVersion,
		Secrets: make(map[string]*storedSecret),
	}
}

// storedSecret is the per-path on-disk shape. Mirrors
// [secrets.Secret] minus the path (which is the map key) and minus
// lease fields (the file backend doesn't issue dynamic secrets).
type storedSecret struct {
	Data      map[string]any    `json:"data,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	Version   uint64            `json:"version"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

// toSecret hydrates a public [secrets.Secret] from the on-disk row.
// Path is supplied by the caller (the map key).
func (s *storedSecret) toSecret(path string) *secrets.Secret {
	if s == nil {
		return nil
	}
	return &secrets.Secret{
		Path:      path,
		Data:      cloneAnyMap(s.Data),
		Metadata:  cloneStringMap(s.Metadata),
		Version:   s.Version,
		CreatedAt: s.CreatedAt,
		UpdatedAt: s.UpdatedAt,
	}
}

// cloneAnyMap deep-copies a `map[string]any`. Local helper (not
// exported from internal/secrets — that package's `cloneAnyMap` is
// unexported by intent; we duplicate the small body here rather than
// widen the public surface).
func cloneAnyMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = cloneAnyValue(v)
	}
	return out
}

func cloneAnyValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		return cloneAnyMap(t)
	case []any:
		dup := make([]any, len(t))
		for i, e := range t {
			dup[i] = cloneAnyValue(e)
		}
		return dup
	default:
		return v
	}
}

func cloneStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
