// Package config implements the `config` stdlib state module —
// managing an individual key/value inside a config file per
// PROJECT-DETAILS §4.8 (Files & VCS category).
//
// It complements the `file` module: `file` owns whole-file content;
// `config` touches only the line that defines one key, leaving every
// comment, blank line, and other key exactly as it was. Two file
// shapes are supported in v1.0:
//
//	keyvalue (default) — flat "key=value" lines (whitespace around
//	                     '=' allowed); full-line comments only
//	                     ('#' / ';' first non-whitespace char).
//	ini                — the same lines plus "[section]" headers; a
//	                     key belongs to the section above it ("" =
//	                     the implicit top section before any header).
//
// Declaration.Name is the config file path.
//
// State semantics:
//
//	present — (section S, key K) in the file equals `value`. A new
//	          line is appended to the end of S (ini) / the file
//	          (keyvalue); a missing [section] is created. If the file
//	          itself is missing it is created (unless `create:
//	          false`). An existing key's value is replaced in place,
//	          preserving its leading whitespace and separator style.
//	absent  — every line defining (section S, key K) is removed; the
//	          [section] header is left in place even if it empties.
//
// Key matching is case-sensitive in v1.0. `present` updates the first
// occurrence of the key; `absent` removes all occurrences.
//
// v0.1 out of scope (v0.x candidates):
//   - Case-insensitive keys.
//   - Configurable separator (sshd-style "Key Value", "KEY: value",
//     …) — v1.0 is '='-delimited.
//   - Inline / trailing comments (a '#' or ';' only starts a comment
//     at the start of a line; "key=value # note" treats "# note" as
//     part of the value).
//   - Uncomment-aware updates ("#PermitRootLogin yes" → set the real
//     directive) — v1.0 just appends a new line.
//   - Repeated-key directives (multiple `HostKey` lines), multi-line
//     values / continuations.
//   - TOML / YAML / JSON / XML formats.
//   - Creating parent directories for a new file.
package config

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

// New is the Factory registered with the engine Registry.
func New() statemgmt.Module { return &Module{} }

// Module is the config state module. It is stateless; concurrent
// Check/Apply/Test calls on different Declarations are safe.
type Module struct{}

func (m *Module) Name() string { return "config" }

func (m *Module) ValidStates() []string { return []string{StatePresent, StateAbsent} }

func (m *Module) Validate(decl *statemgmt.Declaration) error {
	p, err := parseParams(decl)
	if err != nil {
		return err
	}
	return p.validate()
}

// DriftSeverity: a wrong / missing config key is a config-level
// mismatch (MEDIUM). A key declared absent but present is more
// suspicious — HIGH, mirroring the file/link/cron modules. Operators
// override via `severity:`.
func (m *Module) DriftSeverity(decl *statemgmt.Declaration, _ *statemgmt.ModuleCheckResult) statemgmt.DriftSeverity {
	if decl != nil && decl.State == StateAbsent {
		return statemgmt.DriftSeverityHigh
	}
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
	content, existed, err := readFile(p.Path)
	if err != nil {
		return nil, err
	}
	_, changed, diff := reconcile(content, existed, p)
	if changed {
		return &statemgmt.ModuleCheckResult{Matches: false, Diff: diff}, nil
	}
	return &statemgmt.ModuleCheckResult{Matches: true}, nil
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
	content, existed, err := readFile(p.Path)
	if err != nil {
		return nil, err
	}
	newContent, changed, diff := reconcile(content, existed, p)
	if !changed {
		return &statemgmt.StateResult{
			Success:  true,
			Changed:  false,
			Comment:  "already converged",
			Duration: time.Since(start),
		}, nil
	}
	if !existed && p.State == StatePresent && !p.Create {
		return &statemgmt.StateResult{
				Success:  false,
				Changed:  false,
				Diff:     diff,
				Duration: time.Since(start),
			},
			fmt.Errorf("config file %s does not exist (set create: true to create it)", p.Path)
	}
	if err := writeFileAtomic(p.Path, []byte(newContent)); err != nil {
		return &statemgmt.StateResult{
			Success:  false,
			Changed:  false,
			Diff:     diff,
			Duration: time.Since(start),
		}, err
	}
	return &statemgmt.StateResult{
		Success:  true,
		Changed:  true,
		Diff:     diff,
		Comment:  "applied",
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

// reconcile computes the file content that satisfies p. changed
// reports whether content needs to change (it doubles as Check's
// drift signal and Apply's write trigger, so the two never disagree);
// diff is a human-readable description of the gap.
func reconcile(content string, existed bool, p *params) (newContent string, changed bool, diff string) {
	ini := p.Format == FormatINI
	switch p.State {
	case StatePresent:
		if !existed {
			nc, _ := set("", ini, p.Section, p.Key, p.Value, p.SpaceAround)
			return nc, true, fmt.Sprintf("file missing; want %s=%q", p.Key, p.Value)
		}
		cur, found := get(content, ini, p.Section, p.Key)
		if found && cur == p.Value {
			return content, false, ""
		}
		nc, _ := set(content, ini, p.Section, p.Key, p.Value, p.SpaceAround)
		if found {
			return nc, true, fmt.Sprintf("%s: %q → %q", p.Key, cur, p.Value)
		}
		return nc, true, fmt.Sprintf("%s: missing → %q", p.Key, p.Value)
	case StateAbsent:
		if !existed {
			return content, false, ""
		}
		if _, found := get(content, ini, p.Section, p.Key); !found {
			return content, false, ""
		}
		nc, _ := del(content, ini, p.Section, p.Key)
		return nc, true, fmt.Sprintf("%s present; want absent", p.Key)
	}
	return content, false, ""
}

// readFile returns the file content and whether it existed. A missing
// file is ("", false, nil) — a normal state, not a failure.
func readFile(path string) (content string, existed bool, err error) {
	data, err := os.ReadFile(path) //nolint:gosec // operator-supplied config path from a validated state declaration
	if errors.Is(err, fs.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("read %s: %w", path, err)
	}
	return string(data), true, nil
}

// writeFileAtomic writes data to path via write-temp-then-rename in
// the same directory. An existing file's permission bits are
// preserved; a new file gets 0644.
func writeFileAtomic(path string, data []byte) error {
	mode := os.FileMode(0o644)
	if fi, err := os.Stat(path); err == nil {
		mode = fi.Mode().Perm()
	}
	tmp := path + ".keystone.tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil { //nolint:gosec // mode mirrors the existing file or is 0644 for a new config
		return fmt.Errorf("write tmp: %w", err)
	}
	if err := os.Chmod(tmp, mode); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("chmod tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename %s → %s: %w", tmp, path, err)
	}
	return nil
}
