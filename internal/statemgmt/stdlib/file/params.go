// SPDX-License-Identifier: Apache-2.0

package file

import (
	"fmt"
	"strconv"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

// State string constants. Centralised so tests + the module body
// reference the same identifiers.
const (
	StatePresent   = "present"
	StateAbsent    = "absent"
	StateDirectory = "directory"
	StateSymlink   = "symlink"
)

// Param key constants. Engine reserves "severity" (Task 7).
const (
	paramContent  = "content"
	paramSource   = "source"
	paramMode     = "mode"
	paramOwner    = "owner"
	paramGroup    = "group"
	paramTarget   = "target"
	paramSeverity = statemgmt.ReservedSeverityParamKey
)

// allowedKeys is every Params key the file module understands. The
// validator rejects unknown keys to catch typos early ("contnet:"
// shouldn't silently get ignored).
var allowedKeys = map[string]struct{}{
	paramContent:  {},
	paramSource:   {},
	paramMode:     {},
	paramOwner:    {},
	paramGroup:    {},
	paramTarget:   {},
	paramSeverity: {},
}

// modeUnset is the sentinel returned by parseMode when no `mode`
// param is set. Distinguishes "no mode specified" from "mode 0000".
const modeUnset = 0o7777 + 1

// params is the parsed shape the Check/Apply paths consume. Strings
// stay strings to keep the param flow readable.
type params struct {
	Path       string // from Declaration.Name
	State      string // from Declaration.State
	Content    string
	HasContent bool
	Source     string
	Mode       uint32 // modeUnset when not provided
	Owner      string
	Group      string
	Target     string
}

// parseParams pulls a typed view out of a Declaration. Returns a
// user-facing error wrapped suitable for ValidationIssue messages.
func parseParams(decl *statemgmt.Declaration) (*params, error) {
	if decl == nil {
		return nil, fmt.Errorf("nil declaration")
	}
	p := &params{
		Path:  decl.Name,
		State: decl.State,
		Mode:  modeUnset,
	}
	for key := range decl.Params {
		if _, ok := allowedKeys[key]; !ok {
			return nil, fmt.Errorf("unknown param %q (allowed: content, source, mode, owner, group, target, severity)", key)
		}
	}
	if raw, ok := decl.Params[paramContent]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("content: expected string, got %T", raw)
		}
		p.Content = s
		p.HasContent = true
	}
	if raw, ok := decl.Params[paramSource]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("source: expected string, got %T", raw)
		}
		p.Source = s
	}
	if raw, ok := decl.Params[paramMode]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("mode: expected octal string (e.g. \"0644\"), got %T", raw)
		}
		mode, err := parseMode(s)
		if err != nil {
			return nil, fmt.Errorf("mode: %w", err)
		}
		p.Mode = mode
	}
	if raw, ok := decl.Params[paramOwner]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("owner: expected string, got %T", raw)
		}
		p.Owner = s
	}
	if raw, ok := decl.Params[paramGroup]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("group: expected string, got %T", raw)
		}
		p.Group = s
	}
	if raw, ok := decl.Params[paramTarget]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("target: expected string, got %T", raw)
		}
		p.Target = s
	}
	return p, nil
}

// parseMode parses an octal mode string like "0644" or "644".
// uint32 here is constrained to the 12-bit Linux mode range
// (sticky/setuid/setgid + perm bits). Returns modeUnset values are
// the caller's concern; this function always returns a real value
// or an error.
func parseMode(s string) (uint32, error) {
	if s == "" {
		return 0, fmt.Errorf("empty")
	}
	n, err := strconv.ParseUint(s, 8, 32)
	if err != nil {
		return 0, fmt.Errorf("not octal: %q", s)
	}
	if n > 0o7777 {
		return 0, fmt.Errorf("out of range: %q", s)
	}
	return uint32(n), nil
}

// validate runs the cross-field shape checks the engine Validator
// can't infer. Engine Validator already enforces Name + State
// non-empty and State ∈ ValidStates.
func (p *params) validate() error {
	switch p.State {
	case StatePresent:
		if p.HasContent && p.Source != "" {
			return fmt.Errorf("content and source are mutually exclusive")
		}
	case StateAbsent:
		// absent must not carry attribute params; they're a sign
		// of operator confusion.
		var leaked []string
		if p.HasContent {
			leaked = append(leaked, "content")
		}
		if p.Source != "" {
			leaked = append(leaked, "source")
		}
		if p.Mode != modeUnset {
			leaked = append(leaked, "mode")
		}
		if p.Owner != "" {
			leaked = append(leaked, "owner")
		}
		if p.Group != "" {
			leaked = append(leaked, "group")
		}
		if p.Target != "" {
			leaked = append(leaked, "target")
		}
		if len(leaked) > 0 {
			return fmt.Errorf("state=absent cannot carry attribute params: %v", leaked)
		}
	case StateDirectory:
		if p.HasContent || p.Source != "" {
			return fmt.Errorf("directory state does not accept content/source")
		}
		if p.Target != "" {
			return fmt.Errorf("directory state does not accept target")
		}
	case StateSymlink:
		if p.Target == "" {
			return fmt.Errorf("symlink state requires target")
		}
		if p.HasContent || p.Source != "" {
			return fmt.Errorf("symlink state does not accept content/source")
		}
		if p.Mode != modeUnset {
			return fmt.Errorf("symlink state does not accept mode (symlink perms are filesystem-ignored)")
		}
	default:
		return fmt.Errorf("invalid state %q", p.State)
	}
	return nil
}
