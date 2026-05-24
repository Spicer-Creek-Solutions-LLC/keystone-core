// SPDX-License-Identifier: Apache-2.0

package runbook

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	yaml "go.yaml.in/yaml/v3"
)

// ErrNotFound is returned by Load when path does not exist.
var ErrNotFound = errors.New("runbook: file not found")

// Load reads and validates a runbook from a YAML file. Decoding is
// strict: unknown fields fail (a typo'd key must not pass silently).
// A returned error is ErrNotFound, a YAML decode error, or a wrapped
// ErrInvalidRunbook.
func Load(path string) (*Runbook, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("runbook: resolve path: %w", err)
	}
	f, err := os.Open(abs) //nolint:gosec // caller-provided runbook path
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, abs)
		}
		return nil, fmt.Errorf("runbook: open %s: %w", abs, err)
	}
	defer func() { _ = f.Close() }()

	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)
	var rb Runbook
	if err := dec.Decode(&rb); err != nil {
		return nil, fmt.Errorf("runbook: decode %s: %w", abs, err)
	}
	if err := rb.Validate(); err != nil {
		return nil, err
	}
	return &rb, nil
}
