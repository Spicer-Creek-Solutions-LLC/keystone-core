// SPDX-License-Identifier: Apache-2.0

package controlplane

import (
	"context"
	"errors"
	"fmt"

	"github.com/gobwas/glob"

	"go.keystone-core.io/keystone-core/internal/state"
)

// ResolveTarget walks the agent registry and returns the records that
// satisfy the proto Target. All three Target dimensions (AgentIDs,
// Labels, HostnamePattern) are AND'd: a record must match every set
// dimension to be included.
//
// An entirely-empty Target intentionally returns (nil, nil) so the
// gRPC layer can short-circuit to a zero-agent BatchTerminal —
// matching the proto's documented "an empty target matches no agents."
//
// Server-side filtering (versus client-side resolve-then-send-IDs) is
// the v1.0 wire-format contract: callers expressing fleet-wide globs
// like `hostname:web-*` over a stretched fleet shouldn't have to round-
// trip the entire agent list back through kscorectl. The expression
// shorthand from internal/targeting is reserved for the CLI; the wire
// stays narrow.
func ResolveTarget(ctx context.Context, store state.AgentStore, t Target) ([]state.AgentRecord, error) {
	if t.isEmpty() {
		return nil, nil
	}
	var hostnameGlob glob.Glob
	if t.HostnamePattern != "" {
		g, err := glob.Compile(t.HostnamePattern)
		if err != nil {
			return nil, fmt.Errorf("controlplane: hostname pattern %q: %w", t.HostnamePattern, err)
		}
		hostnameGlob = g
	}

	agents, err := store.ListAgents(ctx, state.AgentFilter{Limit: -1})
	if err != nil {
		return nil, fmt.Errorf("controlplane: list agents: %w", err)
	}

	idSet := make(map[string]struct{}, len(t.AgentIDs))
	for _, id := range t.AgentIDs {
		idSet[id] = struct{}{}
	}

	out := make([]state.AgentRecord, 0, len(agents))
	for _, rec := range agents {
		if rec == nil {
			continue
		}
		if len(t.AgentIDs) > 0 {
			if _, ok := idSet[rec.ID]; !ok {
				continue
			}
		}
		if len(t.Labels) > 0 {
			match := true
			for k, v := range t.Labels {
				if rec.Labels[k] != v {
					match = false
					break
				}
			}
			if !match {
				continue
			}
		}
		if hostnameGlob != nil && !hostnameGlob.Match(rec.Hostname) {
			continue
		}
		out = append(out, *rec)
	}
	return out, nil
}

// Target is the controlplane-internal projection of v1.Target. Mirrors
// the proto shape but keeps the gRPC-layer types out of internal/state-
// adjacent packages. The gRPC server constructs one of these from the
// inbound proto request.
type Target struct {
	AgentIDs        []string
	Labels          map[string]string
	HostnamePattern string
}

func (t Target) isEmpty() bool {
	return len(t.AgentIDs) == 0 && len(t.Labels) == 0 && t.HostnamePattern == ""
}

// ErrTargetEmpty is returned by ResolveTarget callers that want to
// treat zero-criterion targets as user errors rather than silent
// matches. ResolveTarget itself returns (nil, nil) for empty targets;
// callers can compose this sentinel if they prefer to error.
var ErrTargetEmpty = errors.New("controlplane: empty target matches no agents")
