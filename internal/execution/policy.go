// SPDX-License-Identifier: Apache-2.0

package execution

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// DefaultMaxCommandLength is the §4.7 v1.0 ceiling for the
// summed-byte-length of req.Command + req.Args. Operators can override
// via CommandPolicySpec.MaxCommandLength.
const DefaultMaxCommandLength = 64 << 10 // 64 KiB

// PolicyMode controls allow-list semantics. See CommandPolicy.Validate.
type PolicyMode int

const (
	// PolicyNormal: default. Block list applies; allow list is
	// optional. Empty allow list means "anything not blocked."
	PolicyNormal PolicyMode = iota
	// PolicyStrict: only commands matching an explicit allow-list
	// entry pass. Empty allow list denies everything.
	PolicyStrict
	// PolicyPermissive: only the block list applies; allow list is
	// ignored.
	PolicyPermissive
)

// String renders the mode for diagnostics.
func (m PolicyMode) String() string {
	switch m {
	case PolicyNormal:
		return "normal"
	case PolicyStrict:
		return "strict"
	case PolicyPermissive:
		return "permissive"
	default:
		return fmt.Sprintf("PolicyMode(%d)", int(m))
	}
}

// CommandPolicySpec is the operator-facing configuration for a
// CommandPolicy. All pattern fields are RE2 regex strings matched
// against the full command line ("Command Arg1 Arg2 ..."). Command
// fields match exactly against req.Command.
type CommandPolicySpec struct {
	Mode                PolicyMode
	AllowedCommands     []string
	AllowedPatterns     []string
	BlockedCommands     []string
	BlockedPatterns     []string
	AllowShellExecution bool
	MaxCommandLength    int // 0 → DefaultMaxCommandLength
}

// CommandPolicy is the compiled, ready-to-evaluate form of
// CommandPolicySpec. Build via NewCommandPolicy. The struct is a value
// type — copies are independent and concurrency-safe to use across
// goroutines.
type CommandPolicy struct {
	mode                PolicyMode
	allowedCommands     []string
	allowedPatterns     []*regexp.Regexp
	blockedCommands     []string
	blockedPatterns     []*regexp.Regexp
	allowShellExecution bool
	maxCommandLength    int
}

// Sentinel errors. Callers should match via errors.Is.
var (
	ErrCommandTooLong      = errors.New("execution: command exceeds max length")
	ErrCommandBlocked      = errors.New("execution: command blocked by policy")
	ErrCommandNotAllowed   = errors.New("execution: command not in allow list")
	ErrShellMetachar       = errors.New("execution: shell metacharacter not allowed in non-shell mode")
	ErrShellExecDisallowed = errors.New("execution: shell execution disabled by policy")
)

// shellMetachars is the §4.7 v1.0 metacharacter set blocked by
// ValidateNoShell. Picked specifically: `;` chains, `&` backgrounds,
// `|` pipes, backtick command substitutes.
const shellMetachars = ";&|`"

// NewCommandPolicy compiles a spec into a ready CommandPolicy. Pattern
// compilation errors are returned with the offending pattern in the
// message.
func NewCommandPolicy(s CommandPolicySpec) (CommandPolicy, error) {
	allowed, err := compilePatterns(s.AllowedPatterns, "allowed")
	if err != nil {
		return CommandPolicy{}, err
	}
	blocked, err := compilePatterns(s.BlockedPatterns, "blocked")
	if err != nil {
		return CommandPolicy{}, err
	}
	maxLen := s.MaxCommandLength
	if maxLen <= 0 {
		maxLen = DefaultMaxCommandLength
	}
	return CommandPolicy{
		mode:                s.Mode,
		allowedCommands:     append([]string(nil), s.AllowedCommands...),
		allowedPatterns:     allowed,
		blockedCommands:     append([]string(nil), s.BlockedCommands...),
		blockedPatterns:     blocked,
		allowShellExecution: s.AllowShellExecution,
		maxCommandLength:    maxLen,
	}, nil
}

func compilePatterns(raw []string, label string) ([]*regexp.Regexp, error) {
	out := make([]*regexp.Regexp, 0, len(raw))
	for _, p := range raw {
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, fmt.Errorf("execution: %s pattern %q: %w", label, p, err)
		}
		out = append(out, re)
	}
	return out, nil
}

// Mode returns the policy's mode for diagnostics.
func (p CommandPolicy) Mode() PolicyMode { return p.mode }

// AllowsShell reports whether shell-mode execution is permitted at all
// under this policy. Callers consult this before wrapping a command
// in Shell.CommandLine — if false, return ErrShellExecDisallowed
// without further work.
func (p CommandPolicy) AllowsShell() bool { return p.allowShellExecution }

// MaxCommandLength returns the byte ceiling applied to req.Command
// plus the joined req.Args.
func (p CommandPolicy) MaxCommandLength() int { return p.maxCommandLength }

// Validate checks req against the allow / block lists and length
// limit. Use this for **shell-mode execution**, where the command
// will be wrapped by `bash -c` (or similar) and shell metacharacters
// are legitimate user input. Pair with ValidateNoShell for direct
// (non-shell) execution.
//
// Order: length → blocks (always denied) → mode-driven allow gating.
func (p CommandPolicy) Validate(req ExecuteRequest) error {
	if err := p.checkLength(req); err != nil {
		return err
	}
	full := commandLine(req)

	for _, c := range p.blockedCommands {
		if c == req.Command {
			return fmt.Errorf("%w: command %q on block list", ErrCommandBlocked, req.Command)
		}
	}
	for _, re := range p.blockedPatterns {
		if re.MatchString(full) {
			return fmt.Errorf("%w: pattern %q matches", ErrCommandBlocked, re.String())
		}
	}

	if p.mode == PolicyPermissive {
		return nil
	}

	hasAllow := len(p.allowedCommands) > 0 || len(p.allowedPatterns) > 0
	if p.mode == PolicyNormal && !hasAllow {
		return nil
	}
	if p.mode == PolicyStrict && !hasAllow {
		return fmt.Errorf("%w: strict mode with empty allow list rejects everything", ErrCommandNotAllowed)
	}
	for _, c := range p.allowedCommands {
		if c == req.Command {
			return nil
		}
	}
	for _, re := range p.allowedPatterns {
		if re.MatchString(full) {
			return nil
		}
	}
	return fmt.Errorf("%w: %q not in allow list", ErrCommandNotAllowed, req.Command)
}

// ValidateNoShell is Validate plus a shell-metacharacter scan. Use
// this for **direct (non-shell) execution**, where any of `;`, `&`,
// `|`, or backtick in the command line indicates either a useless
// no-op (no shell will interpret them) or an injection attempt.
//
// Renamed from PROJECT-DETAILS §4.7's ValidateForShell — the original
// name read backwards (suggesting "validate before passing to a
// shell"); the actual semantic is "validate in a no-shell context,"
// which this name makes explicit.
func (p CommandPolicy) ValidateNoShell(req ExecuteRequest) error {
	if err := p.Validate(req); err != nil {
		return err
	}
	full := commandLine(req)
	if i := strings.IndexAny(full, shellMetachars); i >= 0 {
		return fmt.Errorf("%w: %q at position %d", ErrShellMetachar, string(full[i]), i)
	}
	return nil
}

func (p CommandPolicy) checkLength(req ExecuteRequest) error {
	total := len(req.Command)
	for _, a := range req.Args {
		total += len(a)
	}
	if total > p.maxCommandLength {
		return fmt.Errorf("%w: %d > %d", ErrCommandTooLong, total, p.maxCommandLength)
	}
	return nil
}

// commandLine renders the request as a single space-joined string for
// regex / metachar inspection. Args with internal spaces are joined
// raw — callers using shell-aware quoting must handle escaping at
// the dispatch boundary, not here.
func commandLine(req ExecuteRequest) string {
	if len(req.Args) == 0 {
		return req.Command
	}
	var b strings.Builder
	b.Grow(len(req.Command) + 1 + sumLens(req.Args) + len(req.Args))
	b.WriteString(req.Command)
	for _, a := range req.Args {
		b.WriteByte(' ')
		b.WriteString(a)
	}
	return b.String()
}

func sumLens(ss []string) int {
	n := 0
	for _, s := range ss {
		n += len(s)
	}
	return n
}
