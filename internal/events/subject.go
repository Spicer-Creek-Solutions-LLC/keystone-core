// SPDX-License-Identifier: Apache-2.0

package events

import (
	"fmt"
	"strings"
	"unicode"
)

// SubjectPrefix is the project-wide NATS subject namespace root per
// PROJECT-DETAILS §4.9. All event subjects are
// `<SubjectPrefix>.<cluster>.events.<category>.<subtype>`. Constant
// rather than literal so the namespace moves in one place if the
// project ever picks a different root.
const SubjectPrefix = "kscore"

// subjectEventsSegment is the fixed third segment of the subject
// scheme — `kscore.<cluster>.events.<category>.<subtype>`. The
// "events" literal disambiguates from sibling NATS subject trees
// the project will introduce (state subjects, command subjects, etc.).
const subjectEventsSegment = "events"

// SubjectFor builds the canonical NATS subject for an event in the
// given cluster:
//
//	kscore.<cluster>.events.<category>.<subtype>
//
// The cluster name is validated for NATS-subject-token rules — no
// whitespace, no `.`, no `>` or `*` wildcards, non-empty. The
// [EventType] is validated via [ParseEventType]. Errors wrap
// [ErrInvalidEvent].
//
// Multi-segment subtypes (e.g. `state.apply.start`) flow through
// verbatim — the resulting subject
// `kscore.<cluster>.events.state.apply.start` is a valid NATS
// subject and subscribers can use `kscore.<cluster>.events.state.>`
// to fan in everything under the state category.
func SubjectFor(clusterName string, typ EventType) (string, error) {
	if err := validateClusterToken(clusterName); err != nil {
		return "", err
	}
	if _, err := ParseEventType(string(typ)); err != nil {
		return "", err
	}
	return strings.Join([]string{
		SubjectPrefix,
		clusterName,
		subjectEventsSegment,
		string(typ),
	}, "."), nil
}

// SubjectPatternForCategory returns the NATS subject pattern that
// fans in every event in a single category at the canonical
// one-level depth: `kscore.<cluster>.events.<cat>.*`. The `*`
// matches exactly one subject token — correct for 19 of the 22
// canonical types (e.g. `agent.connect`, `job.start`, `system.error`).
//
// **Important**: the `state.apply.start` / `state.apply.done` /
// `state.apply.fail` types have multi-segment subtypes and do NOT
// match the single-`*` form. Operators wanting full state-category
// fan-in use [SubjectDeepPatternForCategory] instead.
//
// Errors wrap [ErrInvalidEvent]: empty / malformed cluster name
// (NATS subject-token rules) or unknown category.
func SubjectPatternForCategory(clusterName string, c Category) (string, error) {
	if err := validateClusterToken(clusterName); err != nil {
		return "", err
	}
	if !c.IsKnown() {
		return "", fmt.Errorf("%w: unknown category %q (known: %v)", ErrInvalidEvent, c, KnownCategories())
	}
	return strings.Join([]string{
		SubjectPrefix,
		clusterName,
		subjectEventsSegment,
		string(c),
		"*",
	}, "."), nil
}

// SubjectDeepPatternForCategory returns the NATS subject pattern
// that fans in every event in a category at ANY depth:
// `kscore.<cluster>.events.<cat>.>`. The `>` is the multi-token
// wildcard — required for the [CategoryState] subtree because the
// `state.apply.*` family has two-token subtypes that the single-`*`
// pattern would miss.
//
// For categories with only one-token subtypes (everything except
// state), either pattern works — [SubjectPatternForCategory] is
// marginally more specific (won't accidentally match a future
// nested subtype an operator introduces).
//
// Errors wrap [ErrInvalidEvent].
func SubjectDeepPatternForCategory(clusterName string, c Category) (string, error) {
	if err := validateClusterToken(clusterName); err != nil {
		return "", err
	}
	if !c.IsKnown() {
		return "", fmt.Errorf("%w: unknown category %q (known: %v)", ErrInvalidEvent, c, KnownCategories())
	}
	return strings.Join([]string{
		SubjectPrefix,
		clusterName,
		subjectEventsSegment,
		string(c),
		">",
	}, "."), nil
}

// validateClusterToken enforces the NATS subject-token rules on the
// cluster name segment: non-empty, no whitespace, no `.`, no `*`
// or `>` (the NATS wildcards). The set of allowed characters
// matches the operator-config naming conventions used by Epic 13
// clustering — alphanumerics, `-`, and `_`. Errors wrap
// [ErrInvalidEvent] so callers can match a single sentinel family.
func validateClusterToken(s string) error {
	if s == "" {
		return fmt.Errorf("%w: cluster name is required", ErrInvalidEvent)
	}
	for _, r := range s {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			continue
		case r == '-' || r == '_':
			continue
		default:
			return fmt.Errorf("%w: cluster name %q contains illegal character %q (allowed: alphanumeric, '-', '_')", ErrInvalidEvent, s, r)
		}
	}
	return nil
}
