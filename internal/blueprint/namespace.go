// SPDX-License-Identifier: Apache-2.0

package blueprint

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

// ErrInvalidNamespace is returned by Namespace for an empty or
// malformed `as:` namespace.
var ErrInvalidNamespace = errors.New("blueprint: invalid namespace")

// ErrStateNameCollision is returned by DetectCollisions when two
// declarations across the merged collection share an ID.
var ErrStateNameCollision = errors.New("blueprint: state name collision")

// Namespace returns a deep copy of sf with every declaration's
// identity rewritten into ns. A declaration ID has the form
// "<prefix>:<name>" (e.g. "files:/etc/nginx.conf"); the name part is
// prefixed with "<ns>/" so the same blueprint deployed twice under
// different namespaces yields disjoint IDs. The declaration's Module,
// State and Name (the real resource key, e.g. a file path) are left
// untouched — only the state *identity* is namespaced.
//
// Intra-collection requisite references (require/watch/prereq/
// onchanges and their *_in forms) that point at a declaration in this
// same StateFile are rewritten to the namespaced ID so the dependency
// graph stays intact. References to IDs outside this collection are
// left alone (they target another blueprint and are not ours to
// rename). sf is not mutated.
func Namespace(sf *statemgmt.StateFile, ns string) (*statemgmt.StateFile, error) {
	if sf == nil {
		return nil, nil
	}
	if !nameRe.MatchString(ns) {
		return nil, fmt.Errorf("%w: %q must match %s", ErrInvalidNamespace, ns, nameRe)
	}

	// Original IDs in this collection → their namespaced form.
	idMap := make(map[string]string, len(sf.Declarations))
	for _, d := range sf.Declarations {
		if d != nil {
			idMap[d.ID] = namespaceID(d.ID, ns)
		}
	}

	out := make([]*statemgmt.Declaration, 0, len(sf.Declarations))
	for _, d := range sf.Declarations {
		if d == nil {
			continue
		}
		nd := &statemgmt.Declaration{
			ID:     idMap[d.ID],
			Module: d.Module,
			State:  d.State,
			Name:   d.Name,
			Params: rewriteParams(d.Params, idMap),
		}
		out = append(out, nd)
	}

	return &statemgmt.StateFile{
		Metadata:     sf.Metadata,
		Includes:     append([]string(nil), sf.Includes...),
		Variables:    sf.Variables,
		Declarations: out,
	}, nil
}

// namespaceID rewrites "<prefix>:<name>" → "<prefix>:<ns>/<name>".
// An ID with no ':' (unconventional) is namespaced whole.
func namespaceID(id, ns string) string {
	if i := strings.IndexByte(id, ':'); i >= 0 {
		return id[:i+1] + ns + "/" + id[i+1:]
	}
	return ns + "/" + id
}

// rewriteParams copies p and rewrites requisite entries whose target
// ID is in idMap (i.e. lives in this collection). Non-requisite keys
// and out-of-collection refs pass through unchanged. The copy is deep
// only along the requisite lists that are actually rewritten.
func rewriteParams(p map[string]any, idMap map[string]string) map[string]any {
	if len(p) == 0 {
		return p
	}
	out := make(map[string]any, len(p))
	for k, v := range p {
		out[k] = v
	}
	for _, key := range statemgmt.RequisiteKeys {
		raw, ok := out[key]
		if !ok {
			continue
		}
		list, ok := raw.([]any)
		if !ok {
			continue // shape errors are the Validator's job, not ours
		}
		newList := make([]any, 0, len(list))
		for _, entry := range list {
			m, ok := entry.(map[string]any)
			if !ok || len(m) != 1 {
				newList = append(newList, entry)
				continue
			}
			rewritten := make(map[string]any, 1)
			for prefix, name := range m {
				nameStr, ok := name.(string)
				if !ok {
					rewritten[prefix] = name
					continue
				}
				refID := prefix + ":" + nameStr
				if nsID, inCollection := idMap[refID]; inCollection {
					// Put the namespaced name back into the single-key
					// requisite map keyed by prefix.
					rewritten[prefix] = nsName(nsID, prefix)
				} else {
					rewritten[prefix] = nameStr
				}
			}
			newList = append(newList, rewritten)
		}
		out[key] = newList
	}
	return out
}

// nsName extracts the name part of a namespaced ID
// "<prefix>:<name>" so it can be put back into a single-key requisite
// map keyed by prefix.
func nsName(namespacedID, prefix string) string {
	return strings.TrimPrefix(namespacedID, prefix+":")
}

// DetectCollisions reports the first ID shared by two declarations
// across the merged set of state files (e.g. a namespaced instance
// colliding with an unnamespaced one, or two instances sharing a
// namespace). All offending IDs are listed, sorted, for a stable
// error.
func DetectCollisions(files ...*statemgmt.StateFile) error {
	seen := make(map[string]int)
	for _, sf := range files {
		if sf == nil {
			continue
		}
		for _, d := range sf.Declarations {
			if d != nil {
				seen[d.ID]++
			}
		}
	}
	var dups []string
	for id, n := range seen {
		if n > 1 {
			dups = append(dups, id)
		}
	}
	if len(dups) == 0 {
		return nil
	}
	sort.Strings(dups)
	return fmt.Errorf("%w: %s", ErrStateNameCollision, strings.Join(dups, ", "))
}
