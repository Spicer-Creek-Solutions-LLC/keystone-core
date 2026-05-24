// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Severity is the ordered severity enum from PROJECT-DETAILS §4.12.
// Used by both [AuditEntry] (the entry-wide severity, typically
// max of [Violation] severities or the policy's declared severity)
// and [Violation] (per-rule severity).
//
// The zero value is [SeverityUnknown], reserved for the
// "uninitialised" case. Emit-time validation requires entries to
// declare a valid severity — [NewAuditEntry] defaults to
// [SeverityLow] when the caller leaves it empty.
type Severity uint8

const (
	// SeverityUnknown is the zero value. Reserved so a zero-value
	// [AuditEntry] fails [AuditEntry.Validate] loudly rather than
	// silently emitting at Low.
	SeverityUnknown Severity = iota

	// SeverityLow is the routine-event level — informational
	// audit records, allowed operations, low-impact violations.
	SeverityLow

	// SeverityMedium is the noteworthy level — policy warnings,
	// retry attempts, soft-quota approach.
	SeverityMedium

	// SeverityHigh signals an operation was rejected by a serious
	// policy or that the system would block under post-v1.0 enforcement.
	SeverityHigh

	// SeverityCritical is the operator-attention level — security-
	// relevant policy violations, encryption-key compromise
	// indicators, audit-store failures.
	SeverityCritical
)

// severityNames maps the enum to its canonical lowercase string
// form used on the wire, in JSON, in CEL filters, and on the CLI.
var severityNames = map[Severity]string{
	SeverityUnknown:  "unknown",
	SeverityLow:      "low",
	SeverityMedium:   "medium",
	SeverityHigh:     "high",
	SeverityCritical: "critical",
}

// String returns the canonical lowercase name. Out-of-range values
// stringify as "severity(N)" so bad casts surface in logs.
func (s Severity) String() string {
	if name, ok := severityNames[s]; ok {
		return name
	}
	return fmt.Sprintf("severity(%d)", uint8(s))
}

// IsValid reports whether the receiver is one of the four
// emission levels (Low / Medium / High / Critical).
// [SeverityUnknown] reports false.
func (s Severity) IsValid() bool {
	return s >= SeverityLow && s <= SeverityCritical
}

// AtLeast reports whether the receiver is at or above the given
// threshold in the ordering. Invalid sides report false so a
// misconfigured threshold never silently passes every entry.
func (s Severity) AtLeast(threshold Severity) bool {
	if !s.IsValid() || !threshold.IsValid() {
		return false
	}
	return s >= threshold
}

// ParseSeverity accepts the canonical lowercase names. Whitespace
// trimmed; case-fold. Errors wrap [ErrInvalidAuditEntry].
func ParseSeverity(s string) (Severity, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "low":
		return SeverityLow, nil
	case "medium":
		return SeverityMedium, nil
	case "high":
		return SeverityHigh, nil
	case "critical":
		return SeverityCritical, nil
	case "":
		return SeverityUnknown, fmt.Errorf("%w: severity is empty", ErrInvalidAuditEntry)
	default:
		return SeverityUnknown, fmt.Errorf("%w: unknown severity %q", ErrInvalidAuditEntry, s)
	}
}

// MarshalText emits the canonical lowercase name. [SeverityUnknown]
// emits "unknown" rather than erroring so a zero-value entry
// round-trips for debug — [AuditEntry.Validate] rejects it.
func (s Severity) MarshalText() ([]byte, error) {
	if name, ok := severityNames[s]; ok {
		return []byte(name), nil
	}
	return nil, fmt.Errorf("%w: severity %d out of range", ErrInvalidAuditEntry, uint8(s))
}

// UnmarshalText parses bytes (typically from `encoding/json`). Empty
// input decodes to [SeverityUnknown] rather than erroring so a
// missing JSON field round-trips cleanly.
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

// AllSeverities returns the four emission levels in ascending
// order. Used by CLI completion + the SQL filter's IN-clause
// translation in task 2.
func AllSeverities() []Severity {
	return []Severity{
		SeverityLow,
		SeverityMedium,
		SeverityHigh,
		SeverityCritical,
	}
}

// EnforcementMode is the §4.12 policy-enforcement intent. v1.0
// records the value on every [AuditEntry] but the [internal/policy.
// Enforcer] stub ignores it (always returns `Allowed=true`); post-v1.0
// honours [Enforce] (block) + [Warn] (log loudly) per §4.12.
type EnforcementMode uint8

const (
	// EnforcementModeUnknown is the zero value. Rejected by entry
	// validation.
	EnforcementModeUnknown EnforcementMode = iota

	// EnforcementModeAudit is the v1.0 default — log the decision,
	// always allow.
	EnforcementModeAudit

	// EnforcementModeWarn (post-v1.0) — log loudly, but still allow.
	EnforcementModeWarn

	// EnforcementModeEnforce (post-v1.0) — block on policy denial.
	EnforcementModeEnforce
)

var enforcementModeNames = map[EnforcementMode]string{
	EnforcementModeUnknown: "unknown",
	EnforcementModeAudit:   "audit",
	EnforcementModeWarn:    "warn",
	EnforcementModeEnforce: "enforce",
}

// String returns the canonical lowercase name.
func (m EnforcementMode) String() string {
	if name, ok := enforcementModeNames[m]; ok {
		return name
	}
	return fmt.Sprintf("enforcement_mode(%d)", uint8(m))
}

// IsValid reports whether the receiver is one of the three
// emission values (Audit / Warn / Enforce).
func (m EnforcementMode) IsValid() bool {
	return m >= EnforcementModeAudit && m <= EnforcementModeEnforce
}

// ParseEnforcementMode accepts the canonical lowercase names.
func ParseEnforcementMode(s string) (EnforcementMode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "audit":
		return EnforcementModeAudit, nil
	case "warn":
		return EnforcementModeWarn, nil
	case "enforce":
		return EnforcementModeEnforce, nil
	case "":
		return EnforcementModeUnknown, fmt.Errorf("%w: enforcement_mode is empty", ErrInvalidAuditEntry)
	default:
		return EnforcementModeUnknown, fmt.Errorf("%w: unknown enforcement_mode %q", ErrInvalidAuditEntry, s)
	}
}

// MarshalText emits the canonical lowercase name.
func (m EnforcementMode) MarshalText() ([]byte, error) {
	if name, ok := enforcementModeNames[m]; ok {
		return []byte(name), nil
	}
	return nil, fmt.Errorf("%w: enforcement_mode %d out of range", ErrInvalidAuditEntry, uint8(m))
}

// UnmarshalText parses bytes. Empty input decodes to
// [EnforcementModeUnknown].
func (m *EnforcementMode) UnmarshalText(b []byte) error {
	if len(b) == 0 {
		*m = EnforcementModeUnknown
		return nil
	}
	parsed, err := ParseEnforcementMode(string(b))
	if err != nil {
		return err
	}
	*m = parsed
	return nil
}

// PolicyType identifies the evaluator that produced a policy
// audit entry. Empty for non-policy entries (auth, secrets, exec,
// state-apply hooks from task 4 — those producers don't run a
// policy evaluation, they just record that an op happened).
type PolicyType string

const (
	// PolicyTypeOPA — entry produced by the [open-policy-agent/opa]
	// evaluator (task 6).
	PolicyTypeOPA PolicyType = "opa"

	// PolicyTypeCEL — entry produced by the [google/cel-go]
	// evaluator (task 7).
	PolicyTypeCEL PolicyType = "cel"

	// PolicyTypeBuiltin — entry produced by the hardcoded-rule
	// builtin evaluator (task 8).
	PolicyTypeBuiltin PolicyType = "builtin"
)

// String returns the underlying string.
func (p PolicyType) String() string { return string(p) }

// IsKnown reports whether the receiver is one of the three v1.0
// evaluator types. Empty string (non-policy entry) reports false.
func (p PolicyType) IsKnown() bool {
	switch p {
	case PolicyTypeOPA, PolicyTypeCEL, PolicyTypeBuiltin:
		return true
	}
	return false
}

// ParsePolicyType accepts the canonical lowercase names. Empty
// string is accepted as the "non-policy entry" sentinel and
// returns `("", nil)` — only known names or empty are valid.
func ParsePolicyType(s string) (PolicyType, error) {
	trimmed := strings.ToLower(strings.TrimSpace(s))
	if trimmed == "" {
		return "", nil
	}
	pt := PolicyType(trimmed)
	if !pt.IsKnown() {
		return "", fmt.Errorf("%w: unknown policy_type %q (known: opa, cel, builtin, or empty for non-policy)", ErrInvalidAuditEntry, s)
	}
	return pt, nil
}

// Violation is the per-rule denial detail produced by the policy
// engine. Surfaced in [AuditEntry.Violations] AND returned to
// callers via the policy-evaluation result type.
//
// Severity here is per-rule. The aggregate [AuditEntry.Severity]
// is conventionally `max(violations[].Severity)` falling back to
// the policy's declared severity when there are no violations
// (allowed entries) — the producer computes this at emit time.
type Violation struct {
	Rule        string   `json:"rule"`
	Message     string   `json:"message"`
	Severity    Severity `json:"severity"`
	Path        string   `json:"path,omitempty"`
	Expected    string   `json:"expected,omitempty"`
	Actual      string   `json:"actual,omitempty"`
	Remediation string   `json:"remediation,omitempty"`
}

// AuditEntry is the canonical audit record per PROJECT-DETAILS
// §4.12. Every sensitive op produces one of these — policy
// evaluations populate the Policy* fields; auth / secrets / exec /
// state-apply hooks (task 4) leave them empty + populate Action
// + User + Metadata.
//
// Field notes:
//
//   - ID: UUIDv7 stamped by [NewAuditEntry]; k-sortable matches
//     Epic 11 [internal/events.Event] precedent for SQL index
//     locality (task 2's SQL store indexes on timestamp; the v7
//     prefix gives ascending-id ≈ ascending-time without a
//     correlated index).
//   - Timestamp: wall-clock UTC at emit time.
//   - PolicyID / PolicyName / PolicyType: empty for non-policy
//     entries.
//   - ResourceType: domain identifier (`secret`, `lease`,
//     `policy`, `agent`, `identity`, `command`, `state`, etc.).
//   - Allowed: did the op proceed? In v1.0 audit-mode-only this
//     ALWAYS matches the underlying op's outcome regardless of
//     policy decision (Enforcer is a stub).
//   - Duration: wall-clock time the op took.
//   - Violations: empty when Allowed=true. For non-policy entries
//     leave empty.
//   - EnforcementMode: the policy's declared mode (Audit / Warn /
//     Enforce). v1.0 records but ignores at the Enforcer.
//   - Severity: the entry-wide severity. Producers stamp it from
//     max(Violations[].Severity) or fall back to the policy's
//     declared severity (allowed entries) or [SeverityLow] (non-
//     policy entries).
//   - User: principal identifier — SPIFFE ID > AgentID > User per
//     the Epic 11 task 10 audit-bridge precedent.
//   - Action: canonical action verb (`get_secret`, `apply_state`,
//     `policy.evaluate`, `auth.login`).
//   - Metadata: free-form tag map for additional context.
//     Cleartext secret values MUST NOT enter here — task 2 ships
//     the redaction config that strips configured patterns on
//     export.
type AuditEntry struct {
	ID              string            `json:"id"`
	Timestamp       time.Time         `json:"timestamp"`
	PolicyID        string            `json:"policy_id,omitempty"`
	PolicyName      string            `json:"policy_name,omitempty"`
	PolicyType      PolicyType        `json:"policy_type,omitempty"`
	ResourceType    string            `json:"resource_type,omitempty"`
	Allowed         bool              `json:"allowed"`
	Duration        time.Duration     `json:"duration"`
	Violations      []Violation       `json:"violations,omitempty"`
	EnforcementMode EnforcementMode   `json:"enforcement_mode"`
	Severity        Severity          `json:"severity"`
	User            string            `json:"user,omitempty"`
	Action          string            `json:"action"`
	Metadata        map[string]string `json:"metadata,omitempty"`
}

// AuditEntryInput is the producer-facing shape used by
// [NewAuditEntry]. Lets producers omit defaultable fields (ID,
// Timestamp) while still expressing the required ones (Action +
// EnforcementMode) at construction.
type AuditEntryInput struct {
	PolicyID        string
	PolicyName      string
	PolicyType      PolicyType
	ResourceType    string
	Allowed         bool
	Duration        time.Duration
	Violations      []Violation
	EnforcementMode EnforcementMode
	Severity        Severity
	User            string
	Action          string
	Metadata        map[string]string
}

// NewAuditEntry constructs an [AuditEntry] from input. Stamps:
//
//   - ID: fresh UUIDv7 (k-sortable; matches [internal/events.Event]
//     precedent).
//   - Timestamp: `time.Now().UTC()`.
//
// Severity defaults to [SeverityLow] when input leaves it
// [SeverityUnknown]; producers typically set it explicitly from
// violation max / policy fallback. EnforcementMode defaults to
// [EnforcementModeAudit] (the v1.0 default).
//
// Errors wrap [ErrInvalidAuditEntry]:
//
//   - Action empty (the canonical verb is required).
//   - PolicyType set but unknown.
func NewAuditEntry(in AuditEntryInput) (AuditEntry, error) {
	if in.Action == "" {
		return AuditEntry{}, fmt.Errorf("%w: action is required", ErrInvalidAuditEntry)
	}
	if in.PolicyType != "" && !in.PolicyType.IsKnown() {
		return AuditEntry{}, fmt.Errorf("%w: unknown policy_type %q", ErrInvalidAuditEntry, in.PolicyType)
	}
	id, err := uuid.NewV7()
	if err != nil {
		return AuditEntry{}, fmt.Errorf("%w: uuidv7: %v", ErrInvalidAuditEntry, err)
	}
	sev := in.Severity
	if !sev.IsValid() {
		sev = SeverityLow
	}
	mode := in.EnforcementMode
	if !mode.IsValid() {
		mode = EnforcementModeAudit
	}
	return AuditEntry{
		ID:              id.String(),
		Timestamp:       time.Now().UTC(),
		PolicyID:        in.PolicyID,
		PolicyName:      in.PolicyName,
		PolicyType:      in.PolicyType,
		ResourceType:    in.ResourceType,
		Allowed:         in.Allowed,
		Duration:        in.Duration,
		Violations:      in.Violations,
		EnforcementMode: mode,
		Severity:        sev,
		User:            in.User,
		Action:          in.Action,
		Metadata:        in.Metadata,
	}, nil
}

// MustNewAuditEntry is the panic-on-error sibling of
// [NewAuditEntry]. Test-only — production code should always
// handle the error.
func MustNewAuditEntry(in AuditEntryInput) AuditEntry {
	e, err := NewAuditEntry(in)
	if err != nil {
		panic(err)
	}
	return e
}

// IsZero reports whether the receiver is the uninitialised value.
// A successful [NewAuditEntry] never returns a zero entry.
func (e AuditEntry) IsZero() bool {
	return e.ID == "" &&
		e.Timestamp.IsZero() &&
		e.PolicyID == "" &&
		e.PolicyName == "" &&
		e.PolicyType == "" &&
		e.ResourceType == "" &&
		!e.Allowed &&
		e.Duration == 0 &&
		len(e.Violations) == 0 &&
		e.EnforcementMode == EnforcementModeUnknown &&
		e.Severity == SeverityUnknown &&
		e.User == "" &&
		e.Action == "" &&
		len(e.Metadata) == 0
}

// Validate enforces the structural invariants the SQL store and
// gRPC handler (later tasks) depend on. Wraps
// [ErrInvalidAuditEntry].
func (e AuditEntry) Validate() error {
	if e.ID == "" {
		return fmt.Errorf("%w: id is required", ErrInvalidAuditEntry)
	}
	if e.Action == "" {
		return fmt.Errorf("%w: action is required", ErrInvalidAuditEntry)
	}
	if e.Timestamp.IsZero() {
		return fmt.Errorf("%w: timestamp is required", ErrInvalidAuditEntry)
	}
	if !e.Severity.IsValid() {
		return fmt.Errorf("%w: severity %s is not a valid level", ErrInvalidAuditEntry, e.Severity)
	}
	if !e.EnforcementMode.IsValid() {
		return fmt.Errorf("%w: enforcement_mode %s is not a valid value", ErrInvalidAuditEntry, e.EnforcementMode)
	}
	if e.PolicyType != "" && !e.PolicyType.IsKnown() {
		return fmt.Errorf("%w: unknown policy_type %q", ErrInvalidAuditEntry, e.PolicyType)
	}
	return nil
}
