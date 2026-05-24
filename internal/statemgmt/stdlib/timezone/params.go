// SPDX-License-Identifier: Apache-2.0

package timezone

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

// tzNameRE accepts the IANA zone-name charset: letters, digits,
// underscores, slashes (region/city), plus and minus (for the
// "Etc/GMT+5" family). The dot is deliberately excluded — IANA
// zone names never contain it — which also makes ".." path
// traversal impossible by construction. Leading/trailing slashes
// are still rejected separately.
var tzNameRE = regexp.MustCompile(`^[A-Za-z0-9_/+-]+$`)

type params struct {
	Zone string
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
	return &params{Zone: decl.Name}, nil
}

func (p *params) validate() error {
	if !tzNameRE.MatchString(p.Zone) {
		return fmt.Errorf("invalid timezone %q (must match %s)", p.Zone, tzNameRE)
	}
	if strings.HasPrefix(p.Zone, "/") || strings.HasSuffix(p.Zone, "/") {
		return fmt.Errorf("invalid timezone %q: leading/trailing slash not allowed", p.Zone)
	}
	return nil
}
