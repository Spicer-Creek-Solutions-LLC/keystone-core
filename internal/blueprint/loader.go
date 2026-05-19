package blueprint

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	yaml "go.yaml.in/yaml/v3"
)

// ManifestFilename is the fixed manifest name within a blueprint
// directory.
const ManifestFilename = "blueprint.yaml"

// ErrNotFound is returned by Load when no manifest exists at the
// resolved path.
var ErrNotFound = errors.New("blueprint: manifest not found")

// Load reads and validates a blueprint manifest. path may be a
// directory containing blueprint.yaml or the manifest file itself.
// Decoding is strict: unknown fields fail (blueprint authoring
// quality is an epic risk — typos must not pass silently).
//
// On success Manifest.SourcePath is the absolute directory holding
// the manifest. A returned error is ErrNotFound, a YAML decode
// error, or a wrapped ErrInvalidManifest from validation.
func Load(path string) (*Manifest, error) {
	manifestPath, dir, err := resolveManifestPath(path)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(manifestPath) //nolint:gosec // path resolved from caller-provided blueprint dir
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, manifestPath)
		}
		return nil, fmt.Errorf("blueprint: open manifest: %w", err)
	}
	defer func() { _ = f.Close() }()

	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)
	var m Manifest
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("blueprint: decode %s: %w", manifestPath, err)
	}
	m.SourcePath = dir

	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

// resolveManifestPath returns the manifest file path and its
// containing directory (both absolute) for a directory- or
// file-shaped input.
func resolveManifestPath(path string) (manifestPath, dir string, err error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", "", fmt.Errorf("blueprint: resolve path: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", "", fmt.Errorf("%w: %s", ErrNotFound, abs)
		}
		return "", "", fmt.Errorf("blueprint: stat %s: %w", abs, err)
	}
	if info.IsDir() {
		return filepath.Join(abs, ManifestFilename), abs, nil
	}
	return abs, filepath.Dir(abs), nil
}
