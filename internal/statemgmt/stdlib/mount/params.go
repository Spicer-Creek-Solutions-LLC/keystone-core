// SPDX-License-Identifier: Apache-2.0

package mount

import (
	"fmt"
	"strconv"
	"strings"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

const (
	StateMounted   = "mounted"
	StatePresent   = "present"
	StateUnmounted = "unmounted"
	StateAbsent    = "absent"
)

const defaultOpts = "defaults"

const (
	paramDevice   = "device"
	paramFSType   = "fstype"
	paramOpts     = "opts"
	paramDump     = "dump"
	paramPass     = "pass"
	paramMkmnt    = "mkmnt"
	paramSeverity = statemgmt.ReservedSeverityParamKey
)

var allowedKeys = map[string]struct{}{
	paramDevice:   {},
	paramFSType:   {},
	paramOpts:     {},
	paramDump:     {},
	paramPass:     {},
	paramMkmnt:    {},
	paramSeverity: {},
}

// fstabShapingKeys are the params that describe the fstab entry; an
// `unmounted` / `absent` declaration may not carry them.
var fstabShapingKeys = []string{paramDevice, paramFSType, paramOpts, paramDump, paramPass}

type params struct {
	MountPoint string // Declaration.Name
	State      string
	Device     string
	FSType     string
	Opts       string
	Dump       int
	Pass       int
	Mkmnt      bool

	seen map[string]struct{}
}

func parseParams(decl *statemgmt.Declaration) (*params, error) {
	if decl == nil {
		return nil, fmt.Errorf("nil declaration")
	}
	seen := make(map[string]struct{}, len(decl.Params))
	for k := range decl.Params {
		if _, ok := allowedKeys[k]; !ok {
			return nil, fmt.Errorf("unknown param %q (allowed: device, fstype, opts, dump, pass, mkmnt, severity)", k)
		}
		seen[k] = struct{}{}
	}
	p := &params{MountPoint: decl.Name, State: decl.State, Opts: defaultOpts, Mkmnt: true, seen: seen}
	if raw, ok := decl.Params[paramDevice]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("device: expected string, got %T", raw)
		}
		p.Device = s
	}
	if raw, ok := decl.Params[paramFSType]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("fstype: expected string, got %T", raw)
		}
		p.FSType = s
	}
	if raw, ok := decl.Params[paramOpts]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("opts: expected string, got %T", raw)
		}
		if strings.TrimSpace(s) != "" {
			p.Opts = s
		}
	}
	if raw, ok := decl.Params[paramDump]; ok {
		n, err := parseInt(raw)
		if err != nil {
			return nil, fmt.Errorf("dump: %w", err)
		}
		p.Dump = n
	}
	if raw, ok := decl.Params[paramPass]; ok {
		n, err := parseInt(raw)
		if err != nil {
			return nil, fmt.Errorf("pass: %w", err)
		}
		p.Pass = n
	}
	if raw, ok := decl.Params[paramMkmnt]; ok {
		b, ok := raw.(bool)
		if !ok {
			return nil, fmt.Errorf("mkmnt: expected bool, got %T", raw)
		}
		p.Mkmnt = b
	}
	return p, nil
}

func parseInt(raw any) (int, error) {
	switch v := raw.(type) {
	case int:
		return v, nil
	case int64:
		return int(v), nil
	case float64:
		if v != float64(int64(v)) {
			return 0, fmt.Errorf("expected a whole number, got %v", v)
		}
		return int(v), nil
	case string:
		n, err := strconv.Atoi(v)
		if err != nil {
			return 0, fmt.Errorf("expected an integer, got %q", v)
		}
		return n, nil
	default:
		return 0, fmt.Errorf("expected an integer, got %T", raw)
	}
}

func hasWhitespace(s string) bool {
	return strings.IndexFunc(s, func(r rune) bool { return r == ' ' || r == '\t' || r == '\n' || r == '\r' }) >= 0
}

func (p *params) validate() error {
	if strings.TrimSpace(p.MountPoint) == "" {
		return fmt.Errorf("mount point (the declaration name) is required")
	}
	if !strings.HasPrefix(p.MountPoint, "/") {
		return fmt.Errorf("mount point %q must be an absolute path", p.MountPoint)
	}
	if hasWhitespace(p.MountPoint) {
		return fmt.Errorf("mount point %q must not contain whitespace (fstab \\040 escaping is not supported in v1.0)", p.MountPoint)
	}
	switch p.State {
	case StateMounted, StatePresent:
		if strings.TrimSpace(p.Device) == "" {
			return fmt.Errorf("state=%s requires device", p.State)
		}
		if strings.TrimSpace(p.FSType) == "" {
			return fmt.Errorf("state=%s requires fstype", p.State)
		}
		if hasWhitespace(p.Device) || hasWhitespace(p.FSType) || hasWhitespace(p.Opts) {
			return fmt.Errorf("device, fstype and opts must not contain whitespace")
		}
		if p.Dump < 0 {
			return fmt.Errorf("dump: must be >= 0, got %d", p.Dump)
		}
		if p.Pass < 0 {
			return fmt.Errorf("pass: must be >= 0, got %d", p.Pass)
		}
	case StateUnmounted, StateAbsent:
		var leaked []string
		for _, k := range fstabShapingKeys {
			if _, ok := p.seen[k]; ok {
				leaked = append(leaked, k)
			}
		}
		if len(leaked) > 0 {
			return fmt.Errorf("state=%s cannot carry fstab params: %v", p.State, leaked)
		}
	default:
		return fmt.Errorf("invalid state %q", p.State)
	}
	return nil
}
