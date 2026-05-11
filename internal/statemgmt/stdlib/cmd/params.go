package cmd

import (
	"fmt"
	"path/filepath"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

// State constants.
const StateRun = "run"

// Param key constants.
const (
	paramCommand        = "command"
	paramCwd            = "cwd"
	paramEnv            = "env"
	paramTimeoutSeconds = "timeout_seconds"
	paramUnless         = "unless"
	paramOnlyIf         = "onlyif"
	paramCreates        = "creates"
	paramShell          = "shell"
	paramSeverity       = statemgmt.ReservedSeverityParamKey
)

// Defaults + bounds.
const (
	defaultTimeoutSeconds = 60
	maxTimeoutSeconds     = 3600
	defaultShell          = "/bin/sh"
)

var allowedKeys = map[string]struct{}{
	paramCommand:        {},
	paramCwd:            {},
	paramEnv:            {},
	paramTimeoutSeconds: {},
	paramUnless:         {},
	paramOnlyIf:         {},
	paramCreates:        {},
	paramShell:          {},
	paramSeverity:       {},
}

// params is the typed view the Check / Apply paths consume.
type params struct {
	Command        string
	Cwd            string
	Env            map[string]string
	TimeoutSeconds int
	Unless         string
	OnlyIf         string
	Creates        string
	Shell          string
}

// parseParams pulls a typed view out of a Declaration. Returns a
// user-facing error suitable for ValidationIssue messages.
func parseParams(decl *statemgmt.Declaration) (*params, error) {
	if decl == nil {
		return nil, fmt.Errorf("nil declaration")
	}
	for key := range decl.Params {
		if _, ok := allowedKeys[key]; !ok {
			return nil, fmt.Errorf("unknown param %q (allowed: command, cwd, env, timeout_seconds, unless, onlyif, creates, shell, severity)", key)
		}
	}
	p := &params{
		TimeoutSeconds: defaultTimeoutSeconds,
		Shell:          defaultShell,
	}
	if raw, ok := decl.Params[paramCommand]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("command: expected string, got %T", raw)
		}
		p.Command = s
	}
	if raw, ok := decl.Params[paramCwd]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("cwd: expected string, got %T", raw)
		}
		p.Cwd = s
	}
	if raw, ok := decl.Params[paramEnv]; ok {
		m, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("env: expected map, got %T", raw)
		}
		p.Env = make(map[string]string, len(m))
		for k, v := range m {
			str, ok := v.(string)
			if !ok {
				return nil, fmt.Errorf("env[%s]: expected string value, got %T", k, v)
			}
			p.Env[k] = str
		}
	}
	if raw, ok := decl.Params[paramTimeoutSeconds]; ok {
		n, err := coerceInt(raw)
		if err != nil {
			return nil, fmt.Errorf("timeout_seconds: %w", err)
		}
		p.TimeoutSeconds = n
	}
	if raw, ok := decl.Params[paramUnless]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("unless: expected string, got %T", raw)
		}
		p.Unless = s
	}
	if raw, ok := decl.Params[paramOnlyIf]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("onlyif: expected string, got %T", raw)
		}
		p.OnlyIf = s
	}
	if raw, ok := decl.Params[paramCreates]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("creates: expected string, got %T", raw)
		}
		p.Creates = s
	}
	if raw, ok := decl.Params[paramShell]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("shell: expected string, got %T", raw)
		}
		p.Shell = s
	}
	return p, nil
}

// coerceInt accepts the various numeric shapes YAML may decode into.
// Both yaml.v3 ints and floats round-trip cleanly here.
func coerceInt(v any) (int, error) {
	switch n := v.(type) {
	case int:
		return n, nil
	case int64:
		return int(n), nil
	case float64:
		// Reject non-whole floats.
		if n != float64(int(n)) {
			return 0, fmt.Errorf("expected integer, got %v", v)
		}
		return int(n), nil
	default:
		return 0, fmt.Errorf("expected integer, got %T", v)
	}
}

// validate enforces the cross-field rules per state.
func (p *params) validate() error {
	if p.Command == "" {
		return fmt.Errorf("command is required")
	}
	if p.TimeoutSeconds < 0 || p.TimeoutSeconds > maxTimeoutSeconds {
		return fmt.Errorf("timeout_seconds out of range [0, %d]: %d", maxTimeoutSeconds, p.TimeoutSeconds)
	}
	if p.Shell != defaultShell {
		return fmt.Errorf("shell %q not supported in v1.0; only %q (alternate shells are v1.x)", p.Shell, defaultShell)
	}
	if p.Creates != "" && !filepath.IsAbs(p.Creates) {
		return fmt.Errorf("creates must be an absolute path; got %q", p.Creates)
	}
	if p.Unless == "" && p.OnlyIf == "" && p.Creates == "" {
		return fmt.Errorf("state=run requires at least one guard (unless / onlyif / creates) for idempotency; " +
			"use `onlyif: /bin/true` to opt into always-run")
	}
	return nil
}
