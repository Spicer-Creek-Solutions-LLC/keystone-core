package cron

import (
	"fmt"
	"regexp"
	"strings"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

const (
	StatePresent = "present"
	StateAbsent  = "absent"
)

// defaultCronUser is whose crontab is managed when `user` is unset.
const defaultCronUser = "root"

const (
	paramCommand  = "command"
	paramSchedule = "schedule"
	paramUser     = "user"
	paramSeverity = statemgmt.ReservedSeverityParamKey
)

var allowedKeys = map[string]struct{}{
	paramCommand:  {},
	paramSchedule: {},
	paramUser:     {},
	paramSeverity: {},
}

// jobIDRE bounds the declaration name that becomes the marker-comment
// identifier. Forbids newlines / carriage returns (which would break
// the single-line marker) and a leading '#' (so an identifier can't
// masquerade as a comment of its own). The rest is permissive — the
// identifier only ever lives in a comment line.
var jobIDRE = regexp.MustCompile(`^[^\r\n#][^\r\n]*$`)

// cronUserRE matches a POSIX-ish username; rejects shell metacharacters
// so a hostile `user` can't reach `crontab -u`.
var cronUserRE = regexp.MustCompile(`^[a-zA-Z0-9_][a-zA-Z0-9_-]*$`)

// specialSchedules is the set of `@`-shortcut schedules cron accepts.
var specialSchedules = map[string]struct{}{
	"@reboot": {}, "@yearly": {}, "@annually": {}, "@monthly": {},
	"@weekly": {}, "@daily": {}, "@midnight": {}, "@hourly": {},
}

type params struct {
	ID       string // Declaration.Name — the job identifier (marker comment)
	State    string
	Command  string
	Schedule string
	User     string
}

func parseParams(decl *statemgmt.Declaration) (*params, error) {
	if decl == nil {
		return nil, fmt.Errorf("nil declaration")
	}
	for k := range decl.Params {
		if _, ok := allowedKeys[k]; !ok {
			return nil, fmt.Errorf("unknown param %q (allowed: command, schedule, user, severity)", k)
		}
	}
	p := &params{ID: decl.Name, State: decl.State, User: defaultCronUser}
	if raw, ok := decl.Params[paramCommand]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("command: expected string, got %T", raw)
		}
		p.Command = s
	}
	if raw, ok := decl.Params[paramSchedule]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("schedule: expected string, got %T", raw)
		}
		p.Schedule = s
	}
	if raw, ok := decl.Params[paramUser]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("user: expected string, got %T", raw)
		}
		if s != "" {
			p.User = s
		}
	}
	return p, nil
}

func (p *params) validate() error {
	if !jobIDRE.MatchString(p.ID) {
		return fmt.Errorf("invalid cron job identifier %q (no newlines; must not start with '#')", p.ID)
	}
	if !cronUserRE.MatchString(p.User) {
		return fmt.Errorf("invalid user %q (must match %s)", p.User, cronUserRE)
	}
	switch p.State {
	case StatePresent:
		if strings.TrimSpace(p.Command) == "" {
			return fmt.Errorf("state=present requires command")
		}
		if strings.TrimSpace(p.Schedule) == "" {
			return fmt.Errorf("state=present requires schedule")
		}
		if err := validateSchedule(p.Schedule); err != nil {
			return fmt.Errorf("schedule: %w", err)
		}
		// The command must be a single line — it lives on one
		// crontab entry line.
		if strings.ContainsAny(p.Command, "\r\n") {
			return fmt.Errorf("command must be a single line")
		}
	case StateAbsent:
		var leaked []string
		if strings.TrimSpace(p.Command) != "" {
			leaked = append(leaked, "command")
		}
		if strings.TrimSpace(p.Schedule) != "" {
			leaked = append(leaked, "schedule")
		}
		if len(leaked) > 0 {
			return fmt.Errorf("state=absent cannot carry attribute params: %v", leaked)
		}
	default:
		return fmt.Errorf("invalid state %q", p.State)
	}
	return nil
}

// validateSchedule accepts either an `@`-shortcut or a five-field
// cron spec. Field *contents* (`*/5`, `1-5`, `MON-FRI`, …) are not
// validated — cron itself rejects malformed fields, and a full
// validator is a v1.x item; here we only catch the obvious shape
// mistakes (wrong field count, unknown shortcut).
func validateSchedule(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return fmt.Errorf("empty")
	}
	if strings.HasPrefix(s, "@") {
		if _, ok := specialSchedules[s]; !ok {
			return fmt.Errorf("unknown special schedule %q", s)
		}
		return nil
	}
	if n := len(strings.Fields(s)); n != 5 {
		return fmt.Errorf("expected 5 fields or an @-shortcut, got %d", n)
	}
	return nil
}

// canonSchedule collapses runs of whitespace in a schedule to single
// spaces so "*/5  *  *  *  *" and "*/5 * * * *" compare equal. An
// @-shortcut is returned unchanged (Fields → one element → joined).
func canonSchedule(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// desiredLine is the crontab entry line a `present` declaration wants:
// the canonicalised schedule, a space, then the (trimmed) command.
func desiredLine(p *params) string {
	return canonSchedule(p.Schedule) + " " + strings.TrimSpace(p.Command)
}
