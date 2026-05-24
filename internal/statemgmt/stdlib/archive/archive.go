// SPDX-License-Identifier: Apache-2.0

// Package archive implements the `archive` stdlib state module —
// extracting an archive into a directory per PROJECT-DETAILS §4.8
// (Files & VCS category). Pair it with the `file` / `git` modules to
// fetch the archive, then extract it once.
//
// Declaration.Name is the archive file path. One state, `present`:
// the archive's contents are extracted under `target`.
//
// Idempotency: if `creates` is set it is the sole check — the path
// existing means "already extracted" and the archive file is never
// even read. Otherwise the module writes a sentinel
// (<target>/.keystone-archive.<hash>) recording the archive's path,
// size and mtime; a later run re-extracts when the archive's size or
// mtime changes (touching the archive without changing it triggers a
// needless re-extract — acceptable for v1.0).
//
// Re-extraction overwrites archived files in place; files in `target`
// that aren't in the archive are left alone (a "clean: true" mode
// that nukes `target` first is a v1.x item). Every entry path is
// validated to stay within `target` (zip-slip / '..' defense), and
// symlink / hardlink / device entries are skipped (not creating them
// is the v1.0-safe choice).
//
// Supported formats (v1.0, stdlib-only): tar, tar.gz (tgz), tar.bz2
// (tbz2/tbz), zip. `format: auto` (the default) infers from the
// filename extension.
//
// v0.1 out of scope (v0.x candidates):
//   - `state: absent` (needs an extraction manifest to remove only
//     what was extracted — use `file: <target>` `state: absent`).
//   - `clean: true` (remove `target` before extracting).
//   - Safe symlink / hardlink extraction.
//   - .tar.xz / .tar.zst / .7z and other formats (need a dep).
//   - sha-based source identity (v1.0 uses size+mtime).
//   - owner / group chown of the extracted tree; mtime preservation.
//   - Extraction size / entry-count limits (zip-bomb hardening).
//   - skip_existing (don't overwrite already-present files).
package archive

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

// New is the Factory registered with the engine Registry.
func New() statemgmt.Module { return &Module{} }

// Module is the archive state module. It is stateless; concurrent
// Check/Apply/Test calls on different Declarations are safe.
type Module struct{}

func (m *Module) Name() string { return "archive" }

func (m *Module) ValidStates() []string { return []string{StatePresent} }

func (m *Module) Validate(decl *statemgmt.Declaration) error {
	p, err := parseParams(decl)
	if err != nil {
		return err
	}
	return p.validate()
}

// DriftSeverity: an un-extracted archive usually means a deployed
// component is missing or stale — config-level (MEDIUM). Operators
// override via `severity:`.
func (m *Module) DriftSeverity(_ *statemgmt.Declaration, _ *statemgmt.ModuleCheckResult) statemgmt.DriftSeverity {
	return statemgmt.DriftSeverityMedium
}

func (m *Module) Check(_ context.Context, decl *statemgmt.Declaration) (*statemgmt.ModuleCheckResult, error) {
	p, err := parseParams(decl)
	if err != nil {
		return nil, err
	}
	if err := p.validate(); err != nil {
		return nil, err
	}
	converged, diff, err := checkConverged(p)
	if err != nil {
		return nil, err
	}
	if converged {
		return &statemgmt.ModuleCheckResult{Matches: true}, nil
	}
	return &statemgmt.ModuleCheckResult{Matches: false, Diff: diff}, nil
}

func (m *Module) Apply(_ context.Context, decl *statemgmt.Declaration) (*statemgmt.StateResult, error) {
	start := time.Now()
	p, err := parseParams(decl)
	if err != nil {
		return nil, err
	}
	if err := p.validate(); err != nil {
		return nil, err
	}
	converged, diff, err := checkConverged(p)
	if err != nil {
		return nil, err
	}
	if converged {
		return &statemgmt.StateResult{
			Success:  true,
			Changed:  false,
			Comment:  "already converged",
			Duration: time.Since(start),
		}, nil
	}

	format, err := detectFormat(p.Archive, p.Format)
	if err != nil {
		return failure(start, diff), err
	}
	size, mtime, err := sourceIdentity(p.Archive)
	if err != nil {
		return failure(start, diff), fmt.Errorf("archive %s: %w", p.Archive, err)
	}
	if err := os.MkdirAll(p.Target, 0o755); err != nil { //nolint:gosec // operator-supplied target; 0755 is the conventional default
		return failure(start, diff), fmt.Errorf("mkdir %s: %w", p.Target, err)
	}
	if err := extract(p.Archive, p.Target, format, p.StripComponents); err != nil {
		return failure(start, diff), err
	}
	if p.Creates == "" {
		if err := writeSentinel(sentinelPath(p.Target, p.Archive), p.Archive, size, mtime); err != nil {
			// The extraction did happen; only the marker write
			// failed (next run will re-extract). Report it.
			return &statemgmt.StateResult{
				Success:  false,
				Changed:  true,
				Diff:     diff,
				Duration: time.Since(start),
			}, fmt.Errorf("write extraction marker: %w", err)
		}
	}
	return &statemgmt.StateResult{
		Success:  true,
		Changed:  true,
		Diff:     diff,
		Comment:  "extracted",
		Duration: time.Since(start),
	}, nil
}

func (m *Module) Test(ctx context.Context, decl *statemgmt.Declaration) (bool, error) {
	res, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return res.Matches, nil
}

func failure(start time.Time, diff string) *statemgmt.StateResult {
	return &statemgmt.StateResult{
		Success:  false,
		Changed:  false,
		Diff:     diff,
		Duration: time.Since(start),
	}
}

// checkConverged reports whether the archive is already extracted.
// When `creates` is set and present, that's the answer — the archive
// file is not consulted. Otherwise the archive file must exist (else
// error: there is nothing sensible to do), and a matching sentinel
// (size+mtime) means converged.
func checkConverged(p *params) (converged bool, diff string, err error) {
	if p.Creates != "" {
		cp := p.Creates
		if !filepath.IsAbs(cp) {
			cp = filepath.Join(p.Target, cp)
		}
		if _, statErr := os.Stat(cp); statErr == nil {
			return true, "", nil
		} else if !errors.Is(statErr, fs.ErrNotExist) {
			return false, "", fmt.Errorf("stat creates path %s: %w", cp, statErr)
		}
		// creates path missing → fall through; we still need the
		// archive to extract.
	}
	size, mtime, err := sourceIdentity(p.Archive)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, "", fmt.Errorf("archive file %s does not exist", p.Archive)
		}
		return false, "", fmt.Errorf("archive %s: %w", p.Archive, err)
	}
	if p.Creates != "" {
		return false, fmt.Sprintf("creates path missing → extract %s into %s", p.Archive, p.Target), nil
	}
	rs, rm, ok := readSentinel(sentinelPath(p.Target, p.Archive))
	if ok && rs == size && rm == mtime {
		return true, "", nil
	}
	if !ok {
		return false, fmt.Sprintf("not yet extracted → extract %s into %s", p.Archive, p.Target), nil
	}
	return false, fmt.Sprintf("archive changed (size/mtime) → re-extract %s into %s", p.Archive, p.Target), nil
}
