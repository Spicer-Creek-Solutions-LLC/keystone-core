// SPDX-License-Identifier: Apache-2.0

package hostname

import (
	"fmt"
	"regexp"
	"strings"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

const StatePresent = "present"

const paramSeverity = statemgmt.ReservedSeverityParamKey

var allowedKeys = map[string]struct{}{
	paramSeverity: {},
}

// hostnameRE is an RFC-1123-ish charset gate: starts with an
// alphanumeric, then alphanumerics / dots (for FQDNs) / hyphens.
// 253-char total cap is enforced separately; per-label 63-char cap
// is checked in validate(). hostnamectl rejects truly malformed
// names too — this is early feedback.
var hostnameRE = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9.-]{0,252}$`)

type params struct {
	Hostname string
}

func parseParams(decl *statemgmt.Declaration) (*params, error) {
	if decl == nil {
		return nil, fmt.Errorf("nil declaration")
	}
	for k := range decl.Params {
		if _, ok := allowedKeys[k]; !ok {
			return nil, fmt.Errorf("unknown param %q (allowed: severity)", k)
		}
	}
	return &params{Hostname: decl.Name}, nil
}

func (p *params) validate() error {
	if !hostnameRE.MatchString(p.Hostname) {
		return fmt.Errorf("invalid hostname %q (must match %s)", p.Hostname, hostnameRE)
	}
	for _, label := range strings.Split(p.Hostname, ".") {
		if len(label) > 63 {
			return fmt.Errorf("invalid hostname %q: label %q exceeds 63 chars", p.Hostname, label)
		}
		if label == "" {
			return fmt.Errorf("invalid hostname %q: empty label (consecutive or trailing dot)", p.Hostname)
		}
	}
	return nil
}
