// SPDX-License-Identifier: Apache-2.0

package capability

import (
	"fmt"

	"go.keystone-core.io/keystone-core/pkg/module/manifest"
)

// FSRead is the scoped fs.read capability: path must match an
// allowed glob (and no denied glob); the read result must not
// exceed max_file_size.
type FSRead struct {
	scope *pathScope
	max   int64
	host  FSHost
}

// NewFSRead builds the capability from its manifest config + host.
func NewFSRead(cfg manifest.CapabilityConfig, host FSHost) (*FSRead, error) {
	sc, err := newPathScope(cfg.Paths, cfg.DeniedPaths)
	if err != nil {
		return nil, err
	}
	max, err := sizeLimit(cfg.MaxFileSize)
	if err != nil {
		return nil, fmt.Errorf("fs.read max_file_size: %w", err)
	}
	return &FSRead{scope: sc, max: max, host: host}, nil
}

// Read reads path after enforcing scope + size.
func (c *FSRead) Read(path string) ([]byte, error) {
	if c.host == nil {
		return nil, fmt.Errorf("fs.read: %w", ErrHostUnavailable)
	}
	if err := c.scope.check(path); err != nil {
		return nil, err
	}
	if c.max > 0 {
		if sz, err := c.host.Stat(path); err == nil {
			if err := withinSize(sz, c.max); err != nil {
				return nil, err
			}
		}
	}
	b, err := c.host.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if err := withinSize(int64(len(b)), c.max); err != nil {
		return nil, err
	}
	return b, nil
}

// FSWrite is the scoped fs.write capability.
type FSWrite struct {
	scope *pathScope
	max   int64
	host  FSHost
}

// NewFSWrite builds the capability from its manifest config + host.
func NewFSWrite(cfg manifest.CapabilityConfig, host FSHost) (*FSWrite, error) {
	sc, err := newPathScope(cfg.Paths, cfg.DeniedPaths)
	if err != nil {
		return nil, err
	}
	max, err := sizeLimit(cfg.MaxFileSize)
	if err != nil {
		return nil, fmt.Errorf("fs.write max_file_size: %w", err)
	}
	return &FSWrite{scope: sc, max: max, host: host}, nil
}

// Write writes data to path after enforcing scope + payload size.
func (c *FSWrite) Write(path string, data []byte, perm uint32) error {
	if c.host == nil {
		return fmt.Errorf("fs.write: %w", ErrHostUnavailable)
	}
	if err := c.scope.check(path); err != nil {
		return err
	}
	if err := withinSize(int64(len(data)), c.max); err != nil {
		return err
	}
	return c.host.WriteFile(path, data, perm)
}
