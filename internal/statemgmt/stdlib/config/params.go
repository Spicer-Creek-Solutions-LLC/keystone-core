package config

import (
	"fmt"
	"strconv"
	"strings"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

const (
	StatePresent = "present"
	StateAbsent  = "absent"
)

// Supported file formats.
const (
	FormatKeyValue = "keyvalue"
	FormatINI      = "ini"
)

const (
	paramKey                  = "key"
	paramValue                = "value"
	paramFormat               = "format"
	paramSection              = "section"
	paramSpaceAroundSeparator = "space_around_separator"
	paramCreate               = "create"
	paramSeverity             = statemgmt.ReservedSeverityParamKey
)

var allowedKeys = map[string]struct{}{
	paramKey:                  {},
	paramValue:                {},
	paramFormat:               {},
	paramSection:              {},
	paramSpaceAroundSeparator: {},
	paramCreate:               {},
	paramSeverity:             {},
}

type params struct {
	Path        string // Declaration.Name — the config file path
	State       string
	Key         string
	Value       string // string-coerced
	HasValue    bool
	Format      string // FormatKeyValue | FormatINI
	Section     string // ini only; "" = the implicit top section
	SpaceAround bool   // write "key = value" vs "key=value" for *new* lines
	Create      bool   // present: create the file if missing (default true)

	seen map[string]struct{}
}

func parseParams(decl *statemgmt.Declaration) (*params, error) {
	if decl == nil {
		return nil, fmt.Errorf("nil declaration")
	}
	seen := make(map[string]struct{}, len(decl.Params))
	for k := range decl.Params {
		if _, ok := allowedKeys[k]; !ok {
			return nil, fmt.Errorf("unknown param %q (allowed: key, value, format, section, space_around_separator, create, severity)", k)
		}
		seen[k] = struct{}{}
	}
	p := &params{
		Path:   decl.Name,
		State:  decl.State,
		Format: FormatKeyValue,
		Create: true,
		seen:   seen,
	}
	if raw, ok := decl.Params[paramKey]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("key: expected string, got %T", raw)
		}
		p.Key = s
	}
	if raw, ok := decl.Params[paramValue]; ok {
		s, err := coerceString(raw)
		if err != nil {
			return nil, fmt.Errorf("value: %w", err)
		}
		p.Value = s
		p.HasValue = true
	}
	if raw, ok := decl.Params[paramFormat]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("format: expected %q or %q, got %T", FormatKeyValue, FormatINI, raw)
		}
		if s != "" {
			p.Format = s
		}
	}
	if raw, ok := decl.Params[paramSection]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("section: expected string, got %T", raw)
		}
		p.Section = s
	}
	if raw, ok := decl.Params[paramSpaceAroundSeparator]; ok {
		b, ok := raw.(bool)
		if !ok {
			return nil, fmt.Errorf("space_around_separator: expected bool, got %T", raw)
		}
		p.SpaceAround = b
	}
	if raw, ok := decl.Params[paramCreate]; ok {
		b, ok := raw.(bool)
		if !ok {
			return nil, fmt.Errorf("create: expected bool, got %T", raw)
		}
		p.Create = b
	}
	return p, nil
}

// coerceString accepts the YAML scalar forms an operator is likely to
// write for a config value and renders them to the string the file
// will hold. Integral floats ("1024.0") render without a fractional
// part.
func coerceString(raw any) (string, error) {
	switch v := raw.(type) {
	case string:
		return v, nil
	case bool:
		return strconv.FormatBool(v), nil
	case int:
		return strconv.Itoa(v), nil
	case int64:
		return strconv.FormatInt(v, 10), nil
	case float64:
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10), nil
		}
		return strconv.FormatFloat(v, 'g', -1, 64), nil
	default:
		return "", fmt.Errorf("expected a string, number or bool, got %T", raw)
	}
}

func (p *params) validate() error {
	if p.Path == "" {
		return fmt.Errorf("config file path (the declaration name) is required")
	}
	if p.Format != FormatKeyValue && p.Format != FormatINI {
		return fmt.Errorf("format: must be %q or %q, got %q", FormatKeyValue, FormatINI, p.Format)
	}
	if p.Key == "" {
		return fmt.Errorf("key is required")
	}
	if strings.ContainsAny(p.Key, "\r\n=") {
		return fmt.Errorf("key must not contain newlines or '='")
	}
	if p.Key != strings.TrimSpace(p.Key) {
		return fmt.Errorf("key must not have leading or trailing whitespace")
	}
	if strings.HasPrefix(p.Key, "#") || strings.HasPrefix(p.Key, ";") || strings.HasPrefix(p.Key, "[") {
		return fmt.Errorf("key must not start with '#', ';' or '[' (those would be parsed as a comment or section header)")
	}
	if _, ok := p.seen[paramSection]; ok && p.Format != FormatINI {
		return fmt.Errorf("section is only valid with format: ini")
	}
	switch p.State {
	case StatePresent:
		if !p.HasValue {
			return fmt.Errorf("state=present requires value")
		}
		if strings.ContainsAny(p.Value, "\r\n") {
			return fmt.Errorf("value must be a single line")
		}
	case StateAbsent:
		var leaked []string
		if p.HasValue {
			leaked = append(leaked, "value")
		}
		if _, ok := p.seen[paramSpaceAroundSeparator]; ok {
			leaked = append(leaked, "space_around_separator")
		}
		if len(leaked) > 0 {
			return fmt.Errorf("state=absent cannot carry attribute params: %v", leaked)
		}
	default:
		return fmt.Errorf("invalid state %q", p.State)
	}
	return nil
}
