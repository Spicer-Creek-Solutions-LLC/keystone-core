package git

import (
	"fmt"
	"strconv"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

// State string constants.
//
//	present — a working tree exists at Name, cloned from `url`, with
//	          the named remote pointing at `url`. The checked-out
//	          revision is not enforced after the initial clone.
//	latest  — like present, and HEAD matches `rev` on the remote
//	          (`rev` defaults to the remote's default branch). Apply
//	          fetches and (with force, the default) hard-resets.
//	absent  — Name does not exist. A non-repo directory at Name is
//	          left in place with an error.
const (
	StatePresent = "present"
	StateLatest  = "latest"
	StateAbsent  = "absent"
)

const (
	paramURL      = "url"
	paramRev      = "rev"
	paramDepth    = "depth"
	paramRemote   = "remote"
	paramForce    = "force"
	paramSeverity = statemgmt.ReservedSeverityParamKey
)

var allowedKeys = map[string]struct{}{
	paramURL:      {},
	paramRev:      {},
	paramDepth:    {},
	paramRemote:   {},
	paramForce:    {},
	paramSeverity: {},
}

// defaultRemote is the conventional remote name used when `remote` is
// not given.
const defaultRemote = "origin"

// revUnset is the rev value meaning "the remote's default branch".
// git ls-remote / git fetch both accept the literal "HEAD" for this.
const revHEAD = "HEAD"

// params is the parsed view the Check/Apply paths consume. Dir is
// Declaration.Name (the working-tree path).
type params struct {
	Dir    string
	State  string
	URL    string
	Rev    string // branch / tag / sha; "HEAD" means the remote default branch
	Depth  int    // 0 = full clone
	Remote string // defaults to "origin"
	Force  bool   // allow `latest` to discard local changes (default true for latest)

	forceSet bool
}

func parseParams(decl *statemgmt.Declaration) (*params, error) {
	if decl == nil {
		return nil, fmt.Errorf("nil declaration")
	}
	for k := range decl.Params {
		if _, ok := allowedKeys[k]; !ok {
			return nil, fmt.Errorf("unknown param %q (allowed: url, rev, depth, remote, force, severity)", k)
		}
	}
	p := &params{
		Dir:    decl.Name,
		State:  decl.State,
		Rev:    revHEAD,
		Remote: defaultRemote,
	}
	if raw, ok := decl.Params[paramURL]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("url: expected string, got %T", raw)
		}
		p.URL = s
	}
	if raw, ok := decl.Params[paramRev]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("rev: expected string, got %T", raw)
		}
		if s != "" {
			p.Rev = s
		}
	}
	if raw, ok := decl.Params[paramRemote]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("remote: expected string, got %T", raw)
		}
		if s != "" {
			p.Remote = s
		}
	}
	if raw, ok := decl.Params[paramDepth]; ok {
		n, err := parseDepth(raw)
		if err != nil {
			return nil, fmt.Errorf("depth: %w", err)
		}
		p.Depth = n
	}
	if raw, ok := decl.Params[paramForce]; ok {
		b, ok := raw.(bool)
		if !ok {
			return nil, fmt.Errorf("force: expected bool, got %T", raw)
		}
		p.Force = b
		p.forceSet = true
	}
	// `latest` defaults to force=true: it is a declarative "make the
	// tree match the remote" instruction, so discarding local edits
	// is the expected behaviour. Operators opt out with force: false.
	if p.State == StateLatest && !p.forceSet {
		p.Force = true
	}
	return p, nil
}

func parseDepth(raw any) (int, error) {
	switch v := raw.(type) {
	case int:
		return checkDepth(v)
	case int64:
		return checkDepth(int(v))
	case float64:
		if v != float64(int(v)) {
			return 0, fmt.Errorf("expected a whole number, got %v", v)
		}
		return checkDepth(int(v))
	case string:
		n, err := strconv.Atoi(v)
		if err != nil {
			return 0, fmt.Errorf("expected an integer, got %q", v)
		}
		return checkDepth(n)
	default:
		return 0, fmt.Errorf("expected an integer, got %T", raw)
	}
}

func checkDepth(n int) (int, error) {
	if n < 0 {
		return 0, fmt.Errorf("must be >= 0 (0 = full clone), got %d", n)
	}
	return n, nil
}

func (p *params) validate() error {
	switch p.State {
	case StatePresent, StateLatest:
		if p.URL == "" {
			return fmt.Errorf("state=%s requires url", p.State)
		}
		if p.Dir == "" {
			return fmt.Errorf("state=%s requires a working-tree path (the declaration name)", p.State)
		}
	case StateAbsent:
		var leaked []string
		if p.URL != "" {
			leaked = append(leaked, "url")
		}
		if p.Rev != revHEAD {
			leaked = append(leaked, "rev")
		}
		if p.Depth != 0 {
			leaked = append(leaked, "depth")
		}
		if p.Remote != defaultRemote {
			leaked = append(leaked, "remote")
		}
		if len(leaked) > 0 {
			return fmt.Errorf("state=absent cannot carry attribute params: %v", leaked)
		}
	default:
		return fmt.Errorf("invalid state %q", p.State)
	}
	return nil
}

// isFullSHA reports whether rev looks like a full 40-char hex SHA-1.
// Such a rev is its own "latest" — no remote lookup is needed to
// resolve it.
func isFullSHA(rev string) bool {
	if len(rev) != 40 {
		return false
	}
	for _, c := range rev {
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f', c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return true
}
