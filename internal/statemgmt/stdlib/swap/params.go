package swap

import (
	"fmt"
	"strconv"
	"strings"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

const (
	StateOn      = "on"
	StatePresent = "present"
	StateOff     = "off"
	StateAbsent  = "absent"
)

// priorityAuto is the sentinel "no explicit priority" value (swapon's
// default). Stored as the value of p.Priority when `priority` is
// unset; it disables both the `swapon -p` flag and the fstab `pri=`
// option.
const priorityAuto = -1

const (
	paramSize     = "size"
	paramPriority = "priority"
	paramSeverity = statemgmt.ReservedSeverityParamKey
)

var allowedKeys = map[string]struct{}{
	paramSize:     {},
	paramPriority: {},
	paramSeverity: {},
}

type params struct {
	Source    string // Declaration.Name — a swapfile path or a block-device path
	State     string
	SizeBytes int64 // 0 = unset (only used to create a not-yet-existing swapfile under state=on)
	HasSize   bool
	Priority  int // priorityAuto (-1) when unset

	seen map[string]struct{}
}

func parseParams(decl *statemgmt.Declaration) (*params, error) {
	if decl == nil {
		return nil, fmt.Errorf("nil declaration")
	}
	seen := make(map[string]struct{}, len(decl.Params))
	for k := range decl.Params {
		if _, ok := allowedKeys[k]; !ok {
			return nil, fmt.Errorf("unknown param %q (allowed: size, priority, severity)", k)
		}
		seen[k] = struct{}{}
	}
	p := &params{Source: decl.Name, State: decl.State, Priority: priorityAuto, seen: seen}
	if raw, ok := decl.Params[paramSize]; ok {
		n, err := parseSize(raw)
		if err != nil {
			return nil, fmt.Errorf("size: %w", err)
		}
		p.SizeBytes = n
		p.HasSize = true
	}
	if raw, ok := decl.Params[paramPriority]; ok {
		n, err := parseInt(raw)
		if err != nil {
			return nil, fmt.Errorf("priority: %w", err)
		}
		p.Priority = n
	}
	return p, nil
}

// parseSize accepts a string like "2G" / "512M" / "1024K" (1024-based)
// or a YAML integer/string of those forms. A bare number is rejected
// — the unit must be explicit. The result is in bytes and is always a
// whole number of KiB.
func parseSize(raw any) (int64, error) {
	s, ok := raw.(string)
	if !ok {
		// allow ints only if they came as a string elsewhere; a bare
		// YAML int is ambiguous (bytes? MiB?) — reject it.
		return 0, fmt.Errorf("expected a size string with a unit (e.g. \"2G\", \"512M\"), got %T", raw)
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty")
	}
	unit := s[len(s)-1]
	numPart := s[:len(s)-1]
	var mult int64
	switch unit {
	case 'K', 'k':
		mult = 1024
	case 'M', 'm':
		mult = 1024 * 1024
	case 'G', 'g':
		mult = 1024 * 1024 * 1024
	default:
		return 0, fmt.Errorf("missing or unknown unit in %q (use K, M or G)", s)
	}
	n, err := strconv.ParseInt(strings.TrimSpace(numPart), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("not a number: %q", numPart)
	}
	if n <= 0 {
		return 0, fmt.Errorf("must be > 0, got %q", s)
	}
	return n * mult, nil
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
	if strings.TrimSpace(p.Source) == "" {
		return fmt.Errorf("swap source (the declaration name) is required")
	}
	if !strings.HasPrefix(p.Source, "/") {
		return fmt.Errorf("swap source %q must be an absolute path (a swapfile or a device; UUID=/LABEL= sources are not supported in v1.0)", p.Source)
	}
	if hasWhitespace(p.Source) {
		return fmt.Errorf("swap source %q must not contain whitespace", p.Source)
	}
	if p.Priority < priorityAuto || p.Priority > 32767 {
		return fmt.Errorf("priority: must be between -1 and 32767, got %d", p.Priority)
	}
	switch p.State {
	case StateOn:
		// size + priority allowed (size optional — only used to
		// create a not-yet-existing swapfile).
	case StatePresent:
		if _, ok := p.seen[paramSize]; ok {
			return fmt.Errorf("size is only valid with state=on (it governs swapfile creation)")
		}
	case StateOff, StateAbsent:
		var leaked []string
		if _, ok := p.seen[paramSize]; ok {
			leaked = append(leaked, paramSize)
		}
		if _, ok := p.seen[paramPriority]; ok {
			leaked = append(leaked, paramPriority)
		}
		if len(leaked) > 0 {
			return fmt.Errorf("state=%s cannot carry these params: %v", p.State, leaked)
		}
	default:
		return fmt.Errorf("invalid state %q", p.State)
	}
	return nil
}
