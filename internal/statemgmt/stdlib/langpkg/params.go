// SPDX-License-Identifier: Apache-2.0

package langpkg

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

const (
	StatePresent = "present"
	StateAbsent  = "absent"
)

// Manager names.
const (
	ManagerPip = "pip"
	ManagerNpm = "npm"
	ManagerGem = "gem"
)

var validManagers = map[string]struct{}{
	ManagerPip: {},
	ManagerNpm: {},
	ManagerGem: {},
}

// KnownManagers returns the v1.0 catalog in sorted order. Used in
// error messages and exported for documentation tooling.
func KnownManagers() []string {
	out := make([]string, 0, len(validManagers))
	for k := range validManagers {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

const (
	paramName     = "name"
	paramManager  = "manager"
	paramVersion  = "version"
	paramSeverity = statemgmt.ReservedSeverityParamKey
)

var allowedKeys = map[string]struct{}{
	paramName:     {},
	paramManager:  {},
	paramVersion:  {},
	paramSeverity: {},
}

// pkgNameRE matches a v1.0-acceptable package name across the three
// ecosystems: an optional `@scope/` prefix (npm scoped packages),
// then a name with the union of allowed characters
// (alphanumeric + `_`/`.`/`-`). PEP 503 and gem naming are subsets;
// operators who write `@scope/foo` for pip / gem will get the
// backend's own "no such package" error.
var pkgNameRE = regexp.MustCompile(`^(@[A-Za-z0-9._-]+/)?[A-Za-z0-9][A-Za-z0-9._-]*$`)

// versionRE matches an acceptable version string. Permissive on
// purpose — semver, PEP 440, Rubygems versions all use overlapping
// charsets (digits + `.`, plus letters for pre-release tags, plus
// `+` / `-` / `~` / `_` for various conventions).
var versionRE = regexp.MustCompile(`^[A-Za-z0-9.+~_-]+$`)

type params struct {
	Label   string
	State   string
	Name    string
	Manager string // pip|npm|gem
	Version string // optional; "" means "any installed version satisfies"
}

func parseParams(decl *statemgmt.Declaration) (*params, error) {
	if decl == nil {
		return nil, fmt.Errorf("nil declaration")
	}
	for k := range decl.Params {
		if _, ok := allowedKeys[k]; !ok {
			return nil, fmt.Errorf("unknown param %q (allowed: name, manager, version, severity)", k)
		}
	}
	p := &params{Label: decl.Name, State: decl.State}
	if raw, ok := decl.Params[paramName]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("name: expected string, got %T", raw)
		}
		p.Name = strings.TrimSpace(s)
	}
	if raw, ok := decl.Params[paramManager]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("manager: expected string, got %T", raw)
		}
		p.Manager = strings.ToLower(strings.TrimSpace(s))
	}
	if raw, ok := decl.Params[paramVersion]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("version: expected string, got %T", raw)
		}
		p.Version = strings.TrimSpace(s)
	}
	return p, nil
}

func (p *params) validate() error {
	switch p.State {
	case StatePresent, StateAbsent:
	default:
		return fmt.Errorf("invalid state %q", p.State)
	}
	if p.Name == "" {
		return fmt.Errorf("name: required")
	}
	if !pkgNameRE.MatchString(p.Name) {
		return fmt.Errorf("name: %q contains unsupported characters", p.Name)
	}
	if p.Manager == "" {
		return fmt.Errorf("manager: required (one of %s)", strings.Join(KnownManagers(), ", "))
	}
	if _, ok := validManagers[p.Manager]; !ok {
		return fmt.Errorf("manager: %q is not supported (allowed: %s)", p.Manager, strings.Join(KnownManagers(), ", "))
	}
	if p.Version != "" {
		if !versionRE.MatchString(p.Version) {
			return fmt.Errorf("version: %q contains unsupported characters", p.Version)
		}
		if p.State == StateAbsent {
			return fmt.Errorf("version is only valid with state=present (absent removes any/all versions)")
		}
	}
	return nil
}
