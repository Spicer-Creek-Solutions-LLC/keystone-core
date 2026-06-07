// SPDX-License-Identifier: Apache-2.0

package security

import (
	"fmt"
	"regexp"
	"strings"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

// StatePresent is the only allowed Declaration.State value — see the
// package comment for why `absent` is V1X.
const StatePresent = "present"

// SELinux modes.
const (
	ModeEnforcing  = "enforcing"
	ModePermissive = "permissive"
	ModeDisabled   = "disabled"
)

var validModes = map[string]struct{}{
	ModeEnforcing:  {},
	ModePermissive: {},
	ModeDisabled:   {},
}

// AppArmor per-profile modes. These match the mode strings `aa-status
// --json` reports ("enforce" / "complain") so the converged check is a
// direct comparison; "disable" maps to a profile being unloaded
// (absent from aa-status).
const (
	AAEnforce  = "enforce"
	AAComplain = "complain"
	AADisable  = "disable"
)

var validAAModes = map[string]struct{}{
	AAEnforce:  {},
	AAComplain: {},
	AADisable:  {},
}

const (
	paramMode      = "mode"
	paramBoolean   = "boolean"
	paramValue     = "value"
	paramAAProfile = "apparmor.profile"
	paramAAMode    = "apparmor.profile_mode"
	paramSeverity  = statemgmt.ReservedSeverityParamKey
)

var allowedKeys = map[string]struct{}{
	paramMode:      {},
	paramBoolean:   {},
	paramValue:     {},
	paramAAProfile: {},
	paramAAMode:    {},
	paramSeverity:  {},
}

// booleanRE matches a SELinux boolean identifier (the
// `selinux-policy` shipped names are `[A-Za-z_][A-Za-z0-9_]*`).
var booleanRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// aaProfileRE matches an AppArmor profile name as `aa-status` reports
// it — usually the confined program's path (`/usr/bin/foo`) or a named
// profile. We allow path / name characters and disallow whitespace
// (the value is passed as a single exec argument).
var aaProfileRE = regexp.MustCompile(`^[A-Za-z0-9_./:@-]+$`)

// Op identifies which operation the declaration is performing. The
// op is implied by which params are set (mode → OpMode; boolean +
// value → OpBoolean).
type Op int

const (
	OpUnknown Op = iota
	OpMode
	OpBoolean
	OpAppArmorProfile
)

func (o Op) String() string {
	switch o {
	case OpMode:
		return "mode"
	case OpBoolean:
		return "boolean"
	case OpAppArmorProfile:
		return "apparmor.profile"
	}
	return "unknown"
}

type params struct {
	Label string // Declaration.Name — a human label (decl ID; not used for matching)
	State string
	Op    Op

	// OpMode
	Mode string

	// OpBoolean
	BooleanName  string
	BooleanValue bool

	// OpAppArmorProfile
	AAProfile string
	AAMode    string // enforce | complain | disable
}

func parseParams(decl *statemgmt.Declaration) (*params, error) {
	if decl == nil {
		return nil, fmt.Errorf("nil declaration")
	}
	for k := range decl.Params {
		if _, ok := allowedKeys[k]; !ok {
			return nil, fmt.Errorf("unknown param %q (allowed: mode, boolean, value, severity)", k)
		}
	}
	p := &params{Label: decl.Name, State: decl.State}

	modeRaw, hasMode := decl.Params[paramMode]
	booleanRaw, hasBoolean := decl.Params[paramBoolean]
	valueRaw, hasValue := decl.Params[paramValue]
	aaProfileRaw, hasAA := decl.Params[paramAAProfile]
	aaModeRaw, hasAAMode := decl.Params[paramAAMode]

	set := 0
	for _, b := range []bool{hasMode, hasBoolean, hasAA} {
		if b {
			set++
		}
	}
	if set != 1 {
		return nil, fmt.Errorf("exactly one of mode / boolean / apparmor.profile must be set (got %d)", set)
	}
	// Auxiliary params must accompany their op.
	if hasValue && !hasBoolean {
		return nil, fmt.Errorf("value is only valid with boolean")
	}
	if hasAAMode && !hasAA {
		return nil, fmt.Errorf("apparmor.profile_mode is only valid with apparmor.profile")
	}

	switch {
	case hasMode:
		s, ok := modeRaw.(string)
		if !ok {
			return nil, fmt.Errorf("mode: expected string, got %T", modeRaw)
		}
		p.Op = OpMode
		p.Mode = strings.ToLower(strings.TrimSpace(s))
	case hasBoolean:
		if !hasValue {
			return nil, fmt.Errorf("boolean requires value")
		}
		s, ok := booleanRaw.(string)
		if !ok {
			return nil, fmt.Errorf("boolean: expected string, got %T", booleanRaw)
		}
		p.Op = OpBoolean
		p.BooleanName = strings.TrimSpace(s)
		v, err := coerceBool(valueRaw)
		if err != nil {
			return nil, fmt.Errorf("value: %w", err)
		}
		p.BooleanValue = v
	case hasAA:
		if !hasAAMode {
			return nil, fmt.Errorf("apparmor.profile requires apparmor.profile_mode (enforce|complain|disable)")
		}
		s, ok := aaProfileRaw.(string)
		if !ok {
			return nil, fmt.Errorf("apparmor.profile: expected string, got %T", aaProfileRaw)
		}
		ms, ok := aaModeRaw.(string)
		if !ok {
			return nil, fmt.Errorf("apparmor.profile_mode: expected string, got %T", aaModeRaw)
		}
		p.Op = OpAppArmorProfile
		p.AAProfile = strings.TrimSpace(s)
		p.AAMode = strings.ToLower(strings.TrimSpace(ms))
	}
	return p, nil
}

// coerceBool accepts a YAML/JSON bool plus the common SELinux-tooling
// strings (on/off/yes/no/true/false/1/0).
func coerceBool(raw any) (bool, error) {
	switch v := raw.(type) {
	case bool:
		return v, nil
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "on", "true", "yes", "1":
			return true, nil
		case "off", "false", "no", "0":
			return false, nil
		}
		return false, fmt.Errorf("expected on/off/true/false/yes/no/1/0, got %q", v)
	case int:
		switch v {
		case 0:
			return false, nil
		case 1:
			return true, nil
		}
		return false, fmt.Errorf("expected 0 or 1, got %d", v)
	case int64:
		switch v {
		case 0:
			return false, nil
		case 1:
			return true, nil
		}
		return false, fmt.Errorf("expected 0 or 1, got %d", v)
	default:
		return false, fmt.Errorf("expected a boolean, got %T", raw)
	}
}

func (p *params) validate() error {
	if p.State != StatePresent {
		return fmt.Errorf("state must be %q (got %q) — `absent` is not supported by the security module in v1.0", StatePresent, p.State)
	}
	switch p.Op {
	case OpMode:
		if _, ok := validModes[p.Mode]; !ok {
			return fmt.Errorf("mode: must be one of enforcing, permissive, disabled; got %q", p.Mode)
		}
	case OpBoolean:
		if p.BooleanName == "" {
			return fmt.Errorf("boolean: empty")
		}
		if !booleanRE.MatchString(p.BooleanName) {
			return fmt.Errorf("invalid boolean name %q", p.BooleanName)
		}
	case OpAppArmorProfile:
		if p.AAProfile == "" {
			return fmt.Errorf("apparmor.profile: empty")
		}
		if !aaProfileRE.MatchString(p.AAProfile) {
			return fmt.Errorf("invalid apparmor.profile %q (a profile name / program path, no whitespace)", p.AAProfile)
		}
		if _, ok := validAAModes[p.AAMode]; !ok {
			return fmt.Errorf("apparmor.profile_mode: must be one of enforce, complain, disable; got %q", p.AAMode)
		}
	default:
		return fmt.Errorf("internal: no op selected")
	}
	return nil
}
