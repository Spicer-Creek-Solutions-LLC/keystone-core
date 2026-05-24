// SPDX-License-Identifier: Apache-2.0

package capability

import (
	"fmt"
	"path"

	"github.com/gobwas/glob"

	"go.keystone-core.io/keystone-core/pkg/module/manifest"
)

// pathScope compiles allow/deny path globs once at construction.
// gobwas/glob with the '/' separator gives proper path semantics:
// `*` stays within a segment, `**` crosses `/` (the §4.18 manifest
// uses patterns like /etc/apt/**). An empty allow list denies all
// (fail-closed — capabilities must be explicitly scoped).
type pathScope struct {
	allow []glob.Glob
	deny  []glob.Glob
}

func compileGlobs(patterns []string, what string) ([]glob.Glob, error) {
	out := make([]glob.Glob, 0, len(patterns))
	for _, p := range patterns {
		g, err := glob.Compile(p, '/')
		if err != nil {
			return nil, fmt.Errorf("capability: invalid %s glob %q: %w", what, p, err)
		}
		out = append(out, g)
	}
	return out, nil
}

func newPathScope(allow, deny []string) (*pathScope, error) {
	a, err := compileGlobs(allow, "allow-path")
	if err != nil {
		return nil, err
	}
	d, err := compileGlobs(deny, "deny-path")
	if err != nil {
		return nil, err
	}
	return &pathScope{allow: a, deny: d}, nil
}

// check returns ErrPathDenied unless p matches an allow glob and no
// deny glob. p is lexically cleaned first so `/a/../etc/x` cannot
// slip past an `/a/**` allow.
func (s *pathScope) check(p string) error {
	cp := path.Clean(p)
	for _, d := range s.deny {
		if d.Match(cp) {
			return fmt.Errorf("%w: %q (denied)", ErrPathDenied, p)
		}
	}
	for _, a := range s.allow {
		if a.Match(cp) {
			return nil
		}
	}
	return fmt.Errorf("%w: %q", ErrPathDenied, p)
}

// sizeLimit parses an optional CapabilityConfig size string. Empty
// → 0 (meaning "unlimited").
func sizeLimit(raw string) (int64, error) {
	if raw == "" {
		return 0, nil
	}
	n, err := manifest.ParseSize(raw)
	if err != nil {
		return 0, err
	}
	return n, nil
}

func withinSize(n int64, limit int64) error {
	if limit > 0 && n > limit {
		return fmt.Errorf("%w: %d > %d", ErrSizeExceeded, n, limit)
	}
	return nil
}
