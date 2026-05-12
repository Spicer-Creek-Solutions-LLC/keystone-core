package archive

import (
	"fmt"
	"strconv"
	"strings"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

const StatePresent = "present"

// Archive format identifiers. FormatAuto infers from the filename
// extension at Check/Apply time.
const (
	FormatAuto   = "auto"
	FormatTar    = "tar"
	FormatTarGz  = "tar.gz"
	FormatTarBz2 = "tar.bz2"
	FormatZip    = "zip"
)

const (
	paramTarget          = "target"
	paramFormat          = "format"
	paramCreates         = "creates"
	paramStripComponents = "strip_components"
	paramSeverity        = statemgmt.ReservedSeverityParamKey
)

var allowedKeys = map[string]struct{}{
	paramTarget:          {},
	paramFormat:          {},
	paramCreates:         {},
	paramStripComponents: {},
	paramSeverity:        {},
}

// formatAliases maps the operator-facing format values (including a
// few common aliases) to the canonical identifier.
var formatAliases = map[string]string{
	"auto":    FormatAuto,
	"tar":     FormatTar,
	"tar.gz":  FormatTarGz,
	"tgz":     FormatTarGz,
	"tar.bz2": FormatTarBz2,
	"tbz2":    FormatTarBz2,
	"tbz":     FormatTarBz2,
	"zip":     FormatZip,
}

type params struct {
	Archive         string // Declaration.Name — the archive file path
	State           string
	Target          string
	Format          string // canonical: auto|tar|tar.gz|tar.bz2|zip
	Creates         string // optional idempotency short-circuit path
	StripComponents int
}

func parseParams(decl *statemgmt.Declaration) (*params, error) {
	if decl == nil {
		return nil, fmt.Errorf("nil declaration")
	}
	for k := range decl.Params {
		if _, ok := allowedKeys[k]; !ok {
			return nil, fmt.Errorf("unknown param %q (allowed: target, format, creates, strip_components, severity)", k)
		}
	}
	p := &params{Archive: decl.Name, State: decl.State, Format: FormatAuto}
	if raw, ok := decl.Params[paramTarget]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("target: expected string, got %T", raw)
		}
		p.Target = s
	}
	if raw, ok := decl.Params[paramFormat]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("format: expected string, got %T", raw)
		}
		if s != "" {
			canon, ok := formatAliases[strings.ToLower(s)]
			if !ok {
				return nil, fmt.Errorf("format: unknown value %q (want auto, tar, tar.gz, tar.bz2 or zip)", s)
			}
			p.Format = canon
		}
	}
	if raw, ok := decl.Params[paramCreates]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("creates: expected string, got %T", raw)
		}
		p.Creates = s
	}
	if raw, ok := decl.Params[paramStripComponents]; ok {
		n, err := parseStripComponents(raw)
		if err != nil {
			return nil, fmt.Errorf("strip_components: %w", err)
		}
		p.StripComponents = n
	}
	return p, nil
}

func parseStripComponents(raw any) (int, error) {
	switch v := raw.(type) {
	case int:
		return checkStrip(v)
	case int64:
		return checkStrip(int(v))
	case float64:
		if v != float64(int(v)) {
			return 0, fmt.Errorf("expected a whole number, got %v", v)
		}
		return checkStrip(int(v))
	case string:
		n, err := strconv.Atoi(v)
		if err != nil {
			return 0, fmt.Errorf("expected an integer, got %q", v)
		}
		return checkStrip(n)
	default:
		return 0, fmt.Errorf("expected an integer, got %T", raw)
	}
}

func checkStrip(n int) (int, error) {
	if n < 0 {
		return 0, fmt.Errorf("must be >= 0, got %d", n)
	}
	return n, nil
}

func (p *params) validate() error {
	if p.Archive == "" {
		return fmt.Errorf("archive file path (the declaration name) is required")
	}
	if p.State != StatePresent {
		return fmt.Errorf("invalid state %q (only %q is supported; use the file module's state=absent to remove an extracted tree)", p.State, StatePresent)
	}
	if strings.TrimSpace(p.Target) == "" {
		return fmt.Errorf("target is required")
	}
	if strings.ContainsAny(p.Creates, "\r\n") {
		return fmt.Errorf("creates must be a single line")
	}
	switch p.Format {
	case FormatAuto, FormatTar, FormatTarGz, FormatTarBz2, FormatZip:
	default:
		return fmt.Errorf("format: unknown value %q", p.Format)
	}
	return nil
}
