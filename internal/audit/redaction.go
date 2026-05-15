package audit

import (
	"fmt"
	"regexp"
)

// DefaultRedactionReplacement is the literal substituted in place of
// every redacted field / regex match. Mirrors the
// `internal/secrets.MaskedValue` "***" convention.
const DefaultRedactionReplacement = "***"

// RedactionConfig drives [Apply] per §4.12. Operators configure
// three independent redactors:
//
//   - RedactMetadataKeys: drop every listed key from
//     [AuditEntry.Metadata] before export.
//   - RedactPatterns: regex-replace any match in metadata VALUES
//     and Violation.Message strings with Replacement.
//   - RedactUser: blank the [AuditEntry.User] field (for GDPR
//     subject-access compliance — operators can satisfy a "delete
//     all my data" request without dropping the underlying audit
//     record).
//
// Replacement defaults to [DefaultRedactionReplacement] when
// empty. Patterns are compiled at construction (via
// [NewRedactionConfig]) so [Apply] avoids the compile cost per
// entry.
type RedactionConfig struct {
	RedactMetadataKeys []string
	RedactPatterns     []*regexp.Regexp
	RedactUser         bool
	Replacement        string
}

// RedactionConfigInput is the operator-facing shape consumed by
// [NewRedactionConfig]. Patterns are supplied as strings and
// compiled into [regexp.Regexp] at construction.
type RedactionConfigInput struct {
	RedactMetadataKeys []string
	RedactPatterns     []string
	RedactUser         bool
	Replacement        string
}

// NewRedactionConfig builds a [RedactionConfig] from input. Compiles
// every pattern up-front; returns an error on the first invalid
// regex so operator misconfigurations surface at config-load time
// rather than at export time. Empty input returns a no-op config
// (Apply returns the entry verbatim).
func NewRedactionConfig(in RedactionConfigInput) (*RedactionConfig, error) {
	patterns := make([]*regexp.Regexp, 0, len(in.RedactPatterns))
	for i, p := range in.RedactPatterns {
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, fmt.Errorf("audit: redaction pattern[%d] %q: %w", i, p, err)
		}
		patterns = append(patterns, re)
	}
	replacement := in.Replacement
	if replacement == "" {
		replacement = DefaultRedactionReplacement
	}
	// Defensive copy of the keys slice.
	keys := append([]string(nil), in.RedactMetadataKeys...)
	return &RedactionConfig{
		RedactMetadataKeys: keys,
		RedactPatterns:     patterns,
		RedactUser:         in.RedactUser,
		Replacement:        replacement,
	}, nil
}

// IsNoop reports whether the config does anything. Useful for
// callers that want to skip the [Apply] call entirely when nothing
// is configured.
func (c *RedactionConfig) IsNoop() bool {
	if c == nil {
		return true
	}
	return !c.RedactUser && len(c.RedactMetadataKeys) == 0 && len(c.RedactPatterns) == 0
}

// Apply returns a redacted copy of entry per the configured rules.
// The original entry is never mutated. Order of operations:
//
//  1. Copy Metadata (so the original is untouched).
//  2. Drop every key in RedactMetadataKeys.
//  3. For every remaining metadata value, replace regex matches
//     with Replacement.
//  4. For every Violation.Message, replace regex matches.
//  5. If RedactUser, blank User.
//
// Returns the entry verbatim when the receiver is nil OR a no-op
// (no fields configured for redaction). Defensive copy of slices
// and maps ensures callers can mutate the result.
func (c *RedactionConfig) Apply(entry AuditEntry) AuditEntry {
	if c.IsNoop() {
		return entry
	}
	out := entry

	// Metadata copy + key drops.
	if len(entry.Metadata) > 0 {
		md := make(map[string]string, len(entry.Metadata))
		for k, v := range entry.Metadata {
			if containsString(c.RedactMetadataKeys, k) {
				continue
			}
			md[k] = c.redactString(v)
		}
		out.Metadata = md
	}

	// Violations: deep copy + redact each Message.
	if len(entry.Violations) > 0 && len(c.RedactPatterns) > 0 {
		vs := make([]Violation, len(entry.Violations))
		for i, v := range entry.Violations {
			v.Message = c.redactString(v.Message)
			vs[i] = v
		}
		out.Violations = vs
	}

	if c.RedactUser {
		out.User = ""
	}
	return out
}

// redactString walks every configured pattern in registration order,
// replacing matches with Replacement. Returns input unchanged when
// no patterns are configured.
func (c *RedactionConfig) redactString(in string) string {
	if len(c.RedactPatterns) == 0 {
		return in
	}
	out := in
	for _, re := range c.RedactPatterns {
		out = re.ReplaceAllString(out, c.Replacement)
	}
	return out
}

// containsString is a small linear-search helper. RedactMetadataKeys
// is operator-configured and typically small (< 10 entries) so a
// map would be overkill.
func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
