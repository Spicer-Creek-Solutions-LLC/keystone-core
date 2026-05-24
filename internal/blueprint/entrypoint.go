// SPDX-License-Identifier: Apache-2.0

package blueprint

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrEntrypointMissing is returned when a requested entrypoint is not
// declared by the manifest.
var ErrEntrypointMissing = errors.New("blueprint: entrypoint not defined")

// resolveEntrypoint maps a logical entrypoint name to a state-file
// path relative to the manifest's SourcePath, then reads it. name ""
// selects entrypoints.default; "rollback" selects entrypoints.rollback;
// anything else is looked up in entrypoints.named.
func resolveEntrypoint(m *Manifest, name string) (rel string, data []byte, err error) {
	switch name {
	case "":
		rel = m.Entrypoints.Default
	case "rollback":
		rel = m.Entrypoints.Rollback
	default:
		rel = m.Entrypoints.Named[name]
	}
	if rel == "" {
		return "", nil, fmt.Errorf("%w: %q", ErrEntrypointMissing, name)
	}
	path := filepath.Join(m.SourcePath, rel)
	b, err := os.ReadFile(path) //nolint:gosec // path under the loaded blueprint dir
	if err != nil {
		return "", nil, fmt.Errorf("blueprint: read entrypoint %q (%s): %w", name, path, err)
	}
	return rel, b, nil
}
