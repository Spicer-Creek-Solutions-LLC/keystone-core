// SPDX-License-Identifier: Apache-2.0

package policy

import (
	"fmt"
	"strings"
	"time"

	"go.keystone-core.io/keystone-core/internal/audit"
)

// PolicySet groups policies for collective evaluation per §4.12.
// EnforcementOverride, when non-nil, forces every member policy to
// that enforcement mode regardless of the policy's own declared
// mode — the set-level "make this whole bundle Enforce" knob. Nil
// means each member policy keeps its own mode.
//
// The override is a pointer (not a sentinel zero value) so the
// "no override" state is unambiguous: audit.EnforcementModeUnknown
// is a real invalid value, not "inherit."
type PolicySet struct {
	ID                  string
	Name                string
	PolicyIDs           []string
	EnforcementOverride *audit.EnforcementMode
	Enabled             bool
	Tags                []string
	Metadata            map[string]string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// Validate enforces structural invariants. Referential integrity
// (every PolicyID resolves to a registered Policy) is checked by
// the Registry at RegisterPolicySet time, not here — Validate is
// shape-only so it can run before the registry exists.
func (s *PolicySet) Validate() error {
	if s == nil {
		return fmt.Errorf("%w: nil policy set", ErrInvalidPolicy)
	}
	if strings.TrimSpace(s.ID) == "" {
		return fmt.Errorf("%w: PolicySet ID is required", ErrInvalidPolicy)
	}
	if strings.TrimSpace(s.Name) == "" {
		return fmt.Errorf("%w: PolicySet Name is required", ErrInvalidPolicy)
	}
	if len(s.PolicyIDs) == 0 {
		return fmt.Errorf("%w: PolicySet %q has no member policies", ErrInvalidPolicy, s.ID)
	}
	seen := make(map[string]struct{}, len(s.PolicyIDs))
	for _, pid := range s.PolicyIDs {
		if strings.TrimSpace(pid) == "" {
			return fmt.Errorf("%w: PolicySet %q has an empty member policy ID", ErrInvalidPolicy, s.ID)
		}
		if _, dup := seen[pid]; dup {
			return fmt.Errorf("%w: PolicySet %q lists policy %q twice", ErrInvalidPolicy, s.ID, pid)
		}
		seen[pid] = struct{}{}
	}
	if s.EnforcementOverride != nil && !s.EnforcementOverride.IsValid() {
		return fmt.Errorf("%w: PolicySet %q EnforcementOverride is not a valid mode", ErrInvalidPolicy, s.ID)
	}
	return nil
}

// EffectiveMode returns the enforcement mode a member policy should
// run under within this set: the override when set, else the
// policy's own mode.
func (s *PolicySet) EffectiveMode(policyMode audit.EnforcementMode) audit.EnforcementMode {
	if s != nil && s.EnforcementOverride != nil {
		return *s.EnforcementOverride
	}
	return policyMode
}

// Clone returns a deep copy so registry callers can't mutate stored
// set state through a shared slice/map/pointer header.
func (s *PolicySet) Clone() *PolicySet {
	if s == nil {
		return nil
	}
	cp := *s
	if s.PolicyIDs != nil {
		cp.PolicyIDs = append([]string(nil), s.PolicyIDs...)
	}
	if s.Tags != nil {
		cp.Tags = append([]string(nil), s.Tags...)
	}
	if s.Metadata != nil {
		cp.Metadata = make(map[string]string, len(s.Metadata))
		for k, v := range s.Metadata {
			cp.Metadata[k] = v
		}
	}
	if s.EnforcementOverride != nil {
		mode := *s.EnforcementOverride
		cp.EnforcementOverride = &mode
	}
	return &cp
}
