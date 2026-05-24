// SPDX-License-Identifier: Apache-2.0

package kmod

import (
	"fmt"
	"regexp"
	"strings"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

const (
	StatePresent = "present"
	StateAbsent  = "absent"
)

const (
	paramPersist  = "persist"
	paramSeverity = statemgmt.ReservedSeverityParamKey
)

var allowedKeys = map[string]struct{}{
	paramPersist:  {},
	paramSeverity: {},
}

// moduleNameRE matches kernel-module names. The kernel uses
// underscores internally even when modprobe accepts dashes; we
// accept either form and normalise dashes → underscores so the
// /proc/modules compare and the persist filename are stable.
var moduleNameRE = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

type params struct {
	Name    string // normalised: dashes → underscores
	State   string
	Persist bool
}

func parseParams(decl *statemgmt.Declaration) (*params, error) {
	if decl == nil {
		return nil, fmt.Errorf("nil declaration")
	}
	for k := range decl.Params {
		if _, ok := allowedKeys[k]; !ok {
			return nil, fmt.Errorf("unknown param %q (allowed: persist, severity)", k)
		}
	}
	p := &params{Name: normalizeName(decl.Name), State: decl.State, Persist: true}
	if raw, ok := decl.Params[paramPersist]; ok {
		b, ok := raw.(bool)
		if !ok {
			return nil, fmt.Errorf("persist: expected bool, got %T", raw)
		}
		p.Persist = b
	}
	return p, nil
}

func normalizeName(name string) string {
	return strings.ReplaceAll(name, "-", "_")
}

func (p *params) validate() error {
	if !moduleNameRE.MatchString(p.Name) {
		return fmt.Errorf("invalid kernel-module name %q (must match %s)", p.Name, moduleNameRE)
	}
	return nil
}
