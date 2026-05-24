// SPDX-License-Identifier: Apache-2.0

package at

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

// defaultQueue is the `at` queue used when `queue` is unset.
const defaultQueue = "a"

const (
	paramCommand  = "command"
	paramTime     = "time"
	paramQueue    = "queue"
	paramSeverity = statemgmt.ReservedSeverityParamKey
)

var allowedKeys = map[string]struct{}{
	paramCommand:  {},
	paramTime:     {},
	paramQueue:    {},
	paramSeverity: {},
}

// jobIDRE bounds the declaration name that becomes the marker-comment
// identifier: no newlines / carriage returns (it must be a single
// comment line) and not a leading '#' (so it can't masquerade as a
// comment of its own). The rest is permissive — the identifier only
// ever lives in a comment line.
var jobIDRE = regexp.MustCompile(`^[^\r\n#][^\r\n]*$`)

// queueRE matches an `at` queue name: a single ASCII letter.
var queueRE = regexp.MustCompile(`^[a-zA-Z]$`)

// markerLine is the comment line prepended to a submitted job's
// script so the module can find its own jobs in the `at` queue
// (which otherwise only identifies jobs by a daemon-assigned number).
func markerLine(id string) string { return "# keystone-at: " + id }

type params struct {
	ID      string // Declaration.Name — the job identifier (marker comment)
	State   string
	Command string
	Time    string
	Queue   string
}

func parseParams(decl *statemgmt.Declaration) (*params, error) {
	if decl == nil {
		return nil, fmt.Errorf("nil declaration")
	}
	for k := range decl.Params {
		if _, ok := allowedKeys[k]; !ok {
			return nil, fmt.Errorf("unknown param %q (allowed: command, time, queue, severity)", k)
		}
	}
	p := &params{ID: decl.Name, State: decl.State, Queue: defaultQueue}
	if raw, ok := decl.Params[paramCommand]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("command: expected string, got %T", raw)
		}
		p.Command = s
	}
	if raw, ok := decl.Params[paramTime]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("time: expected string, got %T", raw)
		}
		p.Time = s
	}
	if raw, ok := decl.Params[paramQueue]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("queue: expected a single letter, got %T", raw)
		}
		if s != "" {
			p.Queue = s
		}
	}
	return p, nil
}

func (p *params) validate() error {
	if !jobIDRE.MatchString(p.ID) {
		return fmt.Errorf("invalid at job identifier %q (no newlines; must not start with '#')", p.ID)
	}
	if !queueRE.MatchString(p.Queue) {
		return fmt.Errorf("invalid queue %q (must be a single letter)", p.Queue)
	}
	switch p.State {
	case StatePresent:
		if strings.TrimSpace(p.Command) == "" {
			return fmt.Errorf("state=present requires command")
		}
		if strings.TrimSpace(p.Time) == "" {
			return fmt.Errorf("state=present requires time")
		}
		if strings.ContainsAny(p.Time, "\r\n") {
			return fmt.Errorf("time must be a single line")
		}
	case StateAbsent:
		var leaked []string
		if strings.TrimSpace(p.Command) != "" {
			leaked = append(leaked, "command")
		}
		if strings.TrimSpace(p.Time) != "" {
			leaked = append(leaked, "time")
		}
		if len(leaked) > 0 {
			return fmt.Errorf("state=absent cannot carry attribute params: %v", leaked)
		}
	default:
		return fmt.Errorf("invalid state %q", p.State)
	}
	return nil
}

// oneline renders a (possibly multi-line) command for a diff message:
// the first line, with an ellipsis when there is more.
func oneline(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		return strings.TrimSpace(s[:i]) + " …"
	}
	return s
}
