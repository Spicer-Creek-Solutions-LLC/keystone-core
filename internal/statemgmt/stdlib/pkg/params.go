// SPDX-License-Identifier: Apache-2.0

package pkg

import (
	"fmt"
	"regexp"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

const (
	StateInstalled = "installed"
	StateAbsent    = "absent"
)

const (
	paramVersion  = "version"
	paramSeverity = statemgmt.ReservedSeverityParamKey
)

var allowedKeys = map[string]struct{}{
	paramVersion:  {},
	paramSeverity: {},
}

// pkgNameRE matches Debian's policy: starts with an alphanumeric;
// continues with alphanumerics, plus, dot, underscore, dash. Rejects
// pipe/quote/shell-metacharacters early so they don't reach apt-get.
// Source: https://www.debian.org/doc/debian-policy/ch-controlfields.html#source
var pkgNameRE = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._+-]*$`)

type params struct {
	Name    string
	State   string
	Version string
}

func parseParams(decl *statemgmt.Declaration) (*params, error) {
	if decl == nil {
		return nil, fmt.Errorf("nil declaration")
	}
	for key := range decl.Params {
		if _, ok := allowedKeys[key]; !ok {
			return nil, fmt.Errorf("unknown param %q (allowed: version, severity)", key)
		}
	}
	p := &params{Name: decl.Name, State: decl.State}
	if raw, ok := decl.Params[paramVersion]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("version: expected string, got %T", raw)
		}
		p.Version = s
	}
	return p, nil
}

func (p *params) validate() error {
	if !pkgNameRE.MatchString(p.Name) {
		return fmt.Errorf("invalid package name %q (must match %s)", p.Name, pkgNameRE)
	}
	if p.State == StateAbsent && p.Version != "" {
		return fmt.Errorf("version cannot be set when state=absent")
	}
	// state=installed with empty Version is fine (no version pin).
	return nil
}
