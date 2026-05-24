// SPDX-License-Identifier: Apache-2.0

package blueprint

import (
	"errors"
	"fmt"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

// ErrUnknownFeature is returned when an enable/disable override names
// a feature the manifest does not declare.
var ErrUnknownFeature = errors.New("blueprint: unknown feature")

// ErrFeatureConflict is returned when the same feature appears in
// both the enable and disable override lists.
var ErrFeatureConflict = errors.New("blueprint: feature both enabled and disabled")

// EvaluateFeatures computes the effective enabled state of every
// declared feature. Each feature starts at its Feature.Default; an
// entry in disable forces it off, an entry in enable forces it on.
// A name present in both lists is ErrFeatureConflict; a name not
// declared by the manifest is ErrUnknownFeature.
//
// The returned map has one entry per declared feature (never partial)
// so callers can branch without nil-checking.
func EvaluateFeatures(m *Manifest, enable, disable []string) (map[string]bool, error) {
	out := make(map[string]bool, len(m.Features))
	for name, f := range m.Features {
		out[name] = f.Default
	}

	disableSet := make(map[string]bool, len(disable))
	for _, name := range disable {
		if _, ok := m.Features[name]; !ok {
			return nil, fmt.Errorf("%w: %q", ErrUnknownFeature, name)
		}
		disableSet[name] = true
		out[name] = false
	}
	for _, name := range enable {
		if _, ok := m.Features[name]; !ok {
			return nil, fmt.Errorf("%w: %q", ErrUnknownFeature, name)
		}
		if disableSet[name] {
			return nil, fmt.Errorf("%w: %q", ErrFeatureConflict, name)
		}
		out[name] = true
	}
	return out, nil
}

// FilterStateFile returns a copy of sf with every declaration owned by
// a disabled feature removed. A feature owns a declaration when the
// declaration's ID appears in that Feature.States list (exact match —
// the declaration is the grain the runner consumes; file-level
// inclusion is the executor's concern, Epic 15 task 5).
//
// A declaration listed by more than one feature survives if ANY
// owning feature is enabled. Declarations named by no feature are
// always kept. sf is not mutated.
func FilterStateFile(sf *statemgmt.StateFile, m *Manifest, enabled map[string]bool) (*statemgmt.StateFile, error) {
	if sf == nil {
		return nil, nil
	}

	// owners[declID] = set of features that list it.
	owners := make(map[string][]string)
	for fname, f := range m.Features {
		for _, declID := range f.States {
			owners[declID] = append(owners[declID], fname)
		}
	}

	keep := make([]*statemgmt.Declaration, 0, len(sf.Declarations))
	for _, d := range sf.Declarations {
		if d == nil {
			continue
		}
		feats, gated := owners[d.ID]
		if !gated {
			keep = append(keep, d)
			continue
		}
		anyEnabled := false
		for _, fn := range feats {
			if enabled[fn] {
				anyEnabled = true
				break
			}
		}
		if anyEnabled {
			keep = append(keep, d)
		}
	}

	return &statemgmt.StateFile{
		Metadata:     sf.Metadata,
		Includes:     append([]string(nil), sf.Includes...),
		Variables:    sf.Variables,
		Declarations: keep,
	}, nil
}
