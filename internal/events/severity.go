// SPDX-License-Identifier: Apache-2.0

package events

import (
	"fmt"
	"strings"
)

// Severity is an ordered enum of the v1.0 event severity levels per
// PROJECT-DETAILS §4.9. The ordering is meaningful — [Severity.AtLeast]
// supports the canonical `severity >= 'warn'` filter pattern, and
// CEL filters (task 5) compile severity comparisons against this
// ordering.
//
// The zero value is [SeverityUnknown], which is invalid for emission
// and rejected by [Event.Validate]. [NewEvent] stamps [SeverityInfo]
// as the default.
type Severity uint8

const (
	// SeverityUnknown is the zero value. Reserved for the
	// "uninitialised" case so an accidental zero-value [Event]
	// fails [Event.Validate] loudly rather than emitting at debug
	// silently.
	SeverityUnknown Severity = iota

	// SeverityDebug is the lowest emission level. Diagnostic chatter
	// useful during development; typically filtered out in
	// production deployments.
	SeverityDebug

	// SeverityInfo is the default for [NewEvent]. Routine lifecycle
	// signals (agent connect, job start, state apply done).
	SeverityInfo

	// SeverityWarn signals something noteworthy but not failing —
	// retry attempts, degraded conditions, soft-quota approach.
	SeverityWarn

	// SeverityError signals an operation failed but the system as a
	// whole keeps running — a job failed, an agent disconnected
	// abruptly, a policy violation was logged.
	SeverityError

	// SeverityCritical signals a condition that demands operator
	// attention — leader election storms, encryption-key rotation
	// failure, audit-store write failure. Pages should be wired to
	// this level (Epic 17 observability).
	SeverityCritical
)

// severityNames maps the enum to its canonical lowercase string form
// used on the wire, in JSON, in CEL filters, and on the CLI. Ordered
// to mirror the enum so [AllSeverities] is trivially derivable.
var severityNames = map[Severity]string{
	SeverityUnknown:  "unknown",
	SeverityDebug:    "debug",
	SeverityInfo:     "info",
	SeverityWarn:     "warn",
	SeverityError:    "error",
	SeverityCritical: "critical",
}

// String returns the canonical lowercase name. Unknown enum values
// (someone cast an out-of-range uint8) stringify as "severity(N)"
// so the bad value surfaces in logs rather than silently coercing
// to "unknown."
func (s Severity) String() string {
	if name, ok := severityNames[s]; ok {
		return name
	}
	return fmt.Sprintf("severity(%d)", uint8(s))
}

// IsValid reports whether the receiver is one of the five emission
// levels — debug / info / warn / error / critical. [SeverityUnknown]
// reports false; [Event.Validate] uses this predicate to reject
// zero-value severities.
func (s Severity) IsValid() bool {
	return s >= SeverityDebug && s <= SeverityCritical
}

// AtLeast reports whether the receiver is at or above the given
// threshold in the ordering. Mirrors the §4.9 filter idiom
// `severity >= 'warn'`. Both sides must be valid emission levels —
// comparing against [SeverityUnknown] (or any out-of-range value)
// reports false, so a misconfigured threshold never silently passes
// every event.
func (s Severity) AtLeast(threshold Severity) bool {
	if !s.IsValid() || !threshold.IsValid() {
		return false
	}
	return s >= threshold
}

// ParseSeverity accepts the canonical lowercase names plus a small
// allowance for common aliases ("warning" for warn, "fatal" for
// critical) so operators editing config files by hand are not
// surprised. Errors wrap [ErrInvalidEvent].
func ParseSeverity(s string) (Severity, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return SeverityDebug, nil
	case "info":
		return SeverityInfo, nil
	case "warn", "warning":
		return SeverityWarn, nil
	case "error":
		return SeverityError, nil
	case "critical", "fatal":
		return SeverityCritical, nil
	case "":
		return SeverityUnknown, fmt.Errorf("%w: severity is empty", ErrInvalidEvent)
	default:
		return SeverityUnknown, fmt.Errorf("%w: unknown severity %q", ErrInvalidEvent, s)
	}
}

// MarshalText emits the canonical lowercase name. [SeverityUnknown]
// emits "unknown" (rather than erroring) so a zero-value [Event]
// round-trips through JSON for debug purposes — [Event.Validate] is
// the gate that rejects it for emission.
func (s Severity) MarshalText() ([]byte, error) {
	if name, ok := severityNames[s]; ok {
		return []byte(name), nil
	}
	return nil, fmt.Errorf("%w: severity value %d out of range", ErrInvalidEvent, uint8(s))
}

// UnmarshalText parses bytes (typically from `encoding/json` or a
// text-based config loader) into the receiver. Empty input decodes
// to [SeverityUnknown] rather than erroring so a missing JSON field
// round-trips cleanly — callers that require a level must check via
// [Event.Validate] or [Severity.IsValid].
func (s *Severity) UnmarshalText(b []byte) error {
	if len(b) == 0 {
		*s = SeverityUnknown
		return nil
	}
	parsed, err := ParseSeverity(string(b))
	if err != nil {
		return err
	}
	*s = parsed
	return nil
}

// AllSeverities returns the five emission levels in ascending order.
// Useful for CLI completion, documentation generation, and CEL
// filter validation. The returned slice is a fresh copy; the caller
// may mutate it without affecting future calls.
func AllSeverities() []Severity {
	return []Severity{
		SeverityDebug,
		SeverityInfo,
		SeverityWarn,
		SeverityError,
		SeverityCritical,
	}
}
