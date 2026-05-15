package policy

import (
	"fmt"
	"strings"
	"time"
)

// Binding attaches a Policy or PolicySet to a resource type per
// §4.12, with an optional action filter and an optional label
// selector. Exactly one of PolicyID / PolicySetID must be set —
// a binding targets a single policy or a whole set, never both
// and never neither.
//
// Match semantics (see Matches):
//
//   - ResourceType: required, exact match.
//   - Action: empty = matches any action ("all actions on this
//     resource type"); non-empty = exact match.
//   - Selector: empty = matches any labels; non-empty = every
//     selector key/value must be present in the candidate labels
//     (subset match — extra candidate labels are ignored).
type Binding struct {
	ID           string
	PolicyID     string
	PolicySetID  string
	ResourceType string
	Action       string
	Selector     map[string]string
	Enabled      bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Validate enforces structural invariants. Referential integrity
// (PolicyID / PolicySetID resolves) is the Registry's job at
// RegisterBinding time — Validate is shape-only.
func (b *Binding) Validate() error {
	if b == nil {
		return fmt.Errorf("%w: nil binding", ErrInvalidPolicy)
	}
	if strings.TrimSpace(b.ID) == "" {
		return fmt.Errorf("%w: Binding ID is required", ErrInvalidPolicy)
	}
	hasPolicy := strings.TrimSpace(b.PolicyID) != ""
	hasSet := strings.TrimSpace(b.PolicySetID) != ""
	switch {
	case hasPolicy && hasSet:
		return fmt.Errorf("%w: Binding %q sets both PolicyID and PolicySetID (exactly one required)", ErrInvalidPolicy, b.ID)
	case !hasPolicy && !hasSet:
		return fmt.Errorf("%w: Binding %q sets neither PolicyID nor PolicySetID (exactly one required)", ErrInvalidPolicy, b.ID)
	}
	if strings.TrimSpace(b.ResourceType) == "" {
		return fmt.Errorf("%w: Binding %q ResourceType is required", ErrInvalidPolicy, b.ID)
	}
	for k := range b.Selector {
		if strings.TrimSpace(k) == "" {
			return fmt.Errorf("%w: Binding %q Selector has an empty key", ErrInvalidPolicy, b.ID)
		}
	}
	return nil
}

// TargetsSet reports whether this binding targets a PolicySet
// (vs a single Policy). Valid only after Validate passes.
func (b *Binding) TargetsSet() bool {
	return strings.TrimSpace(b.PolicySetID) != ""
}

// Matches reports whether this binding applies to an operation on
// resourceType with the given action and resource labels.
//
//   - resourceType must equal b.ResourceType.
//   - action matches when b.Action is empty (any) or equals action.
//   - every b.Selector entry must appear in labels with the same
//     value (subset match); a nil/empty selector matches anything.
//
// A disabled binding never matches (callers may still inspect it,
// but Matches short-circuits so evaluation paths can filter in one
// call).
func (b *Binding) Matches(resourceType, action string, labels map[string]string) bool {
	if b == nil || !b.Enabled {
		return false
	}
	if b.ResourceType != resourceType {
		return false
	}
	if b.Action != "" && b.Action != action {
		return false
	}
	for k, want := range b.Selector {
		got, ok := labels[k]
		if !ok || got != want {
			return false
		}
	}
	return true
}

// Clone returns a deep copy so registry callers can't mutate stored
// binding state through the shared selector map header.
func (b *Binding) Clone() *Binding {
	if b == nil {
		return nil
	}
	cp := *b
	if b.Selector != nil {
		cp.Selector = make(map[string]string, len(b.Selector))
		for k, v := range b.Selector {
			cp.Selector[k] = v
		}
	}
	return &cp
}
