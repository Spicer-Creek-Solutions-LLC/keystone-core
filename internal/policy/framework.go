package policy

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"go.keystone-core.io/keystone-core/internal/audit"
)

// Framework is the §4.12 compliance framework a ComplianceControl
// belongs to. Like Category this is policy-local metadata — the
// audit log records policy IDs, and the ControlMapping ties those
// IDs to framework controls for ComplianceReport (task 11).
type Framework string

const (
	FrameworkCIS       Framework = "cis"
	FrameworkSOC2      Framework = "soc2"
	FrameworkNIST80053 Framework = "nist-800-53"
	FrameworkHIPAA     Framework = "hipaa"
	FrameworkPCIDSS    Framework = "pci-dss"
	FrameworkGDPR      Framework = "gdpr"
	FrameworkISO27001  Framework = "iso-27001"
	FrameworkCustom    Framework = "custom"
)

// String returns the underlying string.
func (f Framework) String() string { return string(f) }

// IsKnown reports whether the receiver is one of the eight v1.0
// frameworks. Empty string reports false.
func (f Framework) IsKnown() bool {
	switch f {
	case FrameworkCIS, FrameworkSOC2, FrameworkNIST80053, FrameworkHIPAA,
		FrameworkPCIDSS, FrameworkGDPR, FrameworkISO27001, FrameworkCustom:
		return true
	}
	return false
}

// ParseFramework accepts the canonical lowercase names. Whitespace
// is trimmed and case folded. Empty / unknown is an error.
func ParseFramework(s string) (Framework, error) {
	trimmed := strings.ToLower(strings.TrimSpace(s))
	if trimmed == "" {
		return "", fmt.Errorf("%w: framework is empty", ErrInvalidPolicy)
	}
	f := Framework(trimmed)
	if !f.IsKnown() {
		return "", fmt.Errorf("%w: unknown framework %q (known: cis, soc2, nist-800-53, hipaa, pci-dss, gdpr, iso-27001, custom)", ErrInvalidPolicy, s)
	}
	return f, nil
}

// MarshalText implements encoding.TextMarshaler.
func (f Framework) MarshalText() ([]byte, error) {
	if !f.IsKnown() {
		return nil, fmt.Errorf("%w: unknown framework %q", ErrInvalidPolicy, string(f))
	}
	return []byte(f), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (f *Framework) UnmarshalText(b []byte) error {
	parsed, err := ParseFramework(string(b))
	if err != nil {
		return err
	}
	*f = parsed
	return nil
}

// AllFrameworks returns the eight v1.0 frameworks in declaration
// order. Fresh slice per call.
func AllFrameworks() []Framework {
	return []Framework{
		FrameworkCIS, FrameworkSOC2, FrameworkNIST80053, FrameworkHIPAA,
		FrameworkPCIDSS, FrameworkGDPR, FrameworkISO27001, FrameworkCustom,
	}
}

// ComplianceControl is a §4.12 framework control mapped to the
// policies that satisfy it. Severity is the control's own
// criticality (audit.Severity, reused — a control failing maps to
// an audit severity for reporting).
type ComplianceControl struct {
	ID        string
	Framework Framework
	Title     string
	Severity  audit.Severity
	PolicyIDs []string
}

// Validate enforces structural invariants. Referential integrity
// (every PolicyID resolves to a registered Policy) is intentionally
// NOT checked here — a ControlMapping is reporting metadata that
// can legitimately reference policies registered in a separate
// Registry, and controls are often authored before their policies.
func (c *ComplianceControl) Validate() error {
	if c == nil {
		return fmt.Errorf("%w: nil control", ErrInvalidPolicy)
	}
	if strings.TrimSpace(c.ID) == "" {
		return fmt.Errorf("%w: ComplianceControl ID is required", ErrInvalidPolicy)
	}
	if !c.Framework.IsKnown() {
		return fmt.Errorf("%w: control %q framework %q is not known", ErrInvalidPolicy, c.ID, c.Framework)
	}
	if strings.TrimSpace(c.Title) == "" {
		return fmt.Errorf("%w: control %q Title is required", ErrInvalidPolicy, c.ID)
	}
	if !c.Severity.IsValid() {
		return fmt.Errorf("%w: control %q Severity is not a valid level", ErrInvalidPolicy, c.ID)
	}
	if len(c.PolicyIDs) == 0 {
		return fmt.Errorf("%w: control %q has no PolicyIDs", ErrInvalidPolicy, c.ID)
	}
	seen := make(map[string]struct{}, len(c.PolicyIDs))
	for _, pid := range c.PolicyIDs {
		if strings.TrimSpace(pid) == "" {
			return fmt.Errorf("%w: control %q has an empty policy ID", ErrInvalidPolicy, c.ID)
		}
		if _, dup := seen[pid]; dup {
			return fmt.Errorf("%w: control %q lists policy %q twice", ErrInvalidPolicy, c.ID, pid)
		}
		seen[pid] = struct{}{}
	}
	return nil
}

// Clone returns a deep copy so ControlMapping callers can't mutate
// stored state through the shared PolicyIDs slice header.
func (c *ComplianceControl) Clone() *ComplianceControl {
	if c == nil {
		return nil
	}
	cp := *c
	if c.PolicyIDs != nil {
		cp.PolicyIDs = append([]string(nil), c.PolicyIDs...)
	}
	return &cp
}

// ControlMapping is the §4.12 in-memory 2-way framework↔policies
// registry. Safe for concurrent use; mirrors Registry's shape
// (RWMutex, deep-clone in AND out, sentinels ErrDuplicateID /
// ErrNotFound reused from registry.go). No Deregister in v1.0 —
// control CRUD is post-v1.0 (like policy CRUD).
type ControlMapping struct {
	mu       sync.RWMutex
	controls map[string]*ComplianceControl
}

// NewControlMapping returns an empty ControlMapping. Tests use this
// rather than a package global so state doesn't leak between cases.
func NewControlMapping() *ControlMapping {
	return &ControlMapping{controls: make(map[string]*ComplianceControl)}
}

// RegisterControl validates + stores a deep copy of c. Returns
// ErrInvalidPolicy (wrapped) on shape failure, ErrDuplicateID when
// c.ID is already registered.
func (m *ControlMapping) RegisterControl(c *ComplianceControl) error {
	if err := c.Validate(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.controls[c.ID]; exists {
		return fmt.Errorf("%w: control %q", ErrDuplicateID, c.ID)
	}
	m.controls[c.ID] = c.Clone()
	return nil
}

// GetControl returns a deep copy of the registered control, or
// ErrNotFound.
func (m *ControlMapping) GetControl(id string) (*ComplianceControl, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	c, ok := m.controls[id]
	if !ok {
		return nil, fmt.Errorf("%w: control %q", ErrNotFound, id)
	}
	return c.Clone(), nil
}

// ListControls returns deep copies of every control, sorted by ID.
func (m *ControlMapping) ListControls() []*ComplianceControl {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*ComplianceControl, 0, len(m.controls))
	for _, c := range m.controls {
		out = append(out, c.Clone())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// ControlsForPolicy returns every control whose PolicyIDs include
// policyID, sorted by control ID (the policy→controls direction).
func (m *ControlMapping) ControlsForPolicy(policyID string) []*ComplianceControl {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []*ComplianceControl
	for _, c := range m.controls {
		for _, pid := range c.PolicyIDs {
			if pid == policyID {
				out = append(out, c.Clone())
				break
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// ControlsForFramework returns every control in fw, sorted by ID.
func (m *ControlMapping) ControlsForFramework(fw Framework) []*ComplianceControl {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []*ComplianceControl
	for _, c := range m.controls {
		if c.Framework == fw {
			out = append(out, c.Clone())
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// PoliciesForFramework returns the de-duplicated, sorted set of
// policy IDs referenced by any control in fw (the framework→policies
// direction — the bounded policy set ComplianceReport.PolicyStats
// uses when a Framework filter is given).
func (m *ControlMapping) PoliciesForFramework(fw Framework) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	set := map[string]struct{}{}
	for _, c := range m.controls {
		if c.Framework != fw {
			continue
		}
		for _, pid := range c.PolicyIDs {
			set[pid] = struct{}{}
		}
	}
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for pid := range set {
		out = append(out, pid)
	}
	sort.Strings(out)
	return out
}
