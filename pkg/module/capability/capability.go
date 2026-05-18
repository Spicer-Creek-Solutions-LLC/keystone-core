// Package capability holds the granted-capability registry and the
// audited invoker that gates every module capability call
// (Epic 14 task 2).
//
// A [Registry] is the immutable set of capabilities a loaded module
// was *granted* — built from its validated manifest, populated by
// the loader's "register only granted capabilities" step (task 10).
// [Invoker] wraps each capability call: a non-granted capability is
// refused with [ErrCapabilityNotGranted] and a denied audit entry;
// a granted call is timed and audited (success or failure).
//
// The 9 capability *backends* + their path/domain/command scoping
// are task 3 — this task delivers the grant gate + audit wrapper.
package capability

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	maudit "go.keystone-core.io/keystone-core/pkg/module/audit"
	"go.keystone-core.io/keystone-core/pkg/module/manifest"
)

// ErrCapabilityNotGranted is returned by [Invoker.Invoke] when a
// module invokes a capability its manifest did not request/grant.
var ErrCapabilityNotGranted = errors.New("capability: not granted to this module")

// ErrUnknownCapability is returned by [NewRegistryFromManifest]
// when a manifest names a capability outside the 9 core set.
var ErrUnknownCapability = errors.New("capability: unknown capability name")

// Registry is the immutable granted-capability set for one loaded
// module. Construct via [NewRegistryFromManifest]; not mutated
// after construction (grant-at-load — supports the §4.18 step-5
// capability-lock check later).
type Registry struct {
	module  string
	version string
	granted map[string]struct{}
}

// NewRegistryFromManifest projects m's requested capabilities into
// a granted set. m is expected to have passed [manifest.Manifest.Validate];
// unknown capability names are still rejected defensively.
func NewRegistryFromManifest(m *manifest.Manifest) (*Registry, error) {
	if m == nil {
		return nil, fmt.Errorf("capability: nil manifest")
	}
	g := make(map[string]struct{}, len(m.Capabilities))
	for name := range m.Capabilities {
		if !manifest.KnownCapability(name) {
			return nil, fmt.Errorf("%w: %q", ErrUnknownCapability, name)
		}
		g[name] = struct{}{}
	}
	return &Registry{module: m.Name, version: m.Version, granted: g}, nil
}

// Has reports whether cap was granted.
func (r *Registry) Has(capName string) bool {
	if r == nil {
		return false
	}
	_, ok := r.granted[capName]
	return ok
}

// List returns the granted capabilities, sorted (deterministic).
func (r *Registry) List() []string {
	if r == nil {
		return nil
	}
	out := make([]string, 0, len(r.granted))
	for c := range r.granted {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}

// Invoker gates + audits capability calls for one module.
type Invoker struct {
	reg     *Registry
	auditor maudit.Auditor
}

// NewInvoker wires the invoker. A nil auditor falls back to
// [maudit.NoopAuditor]; a nil registry denies everything.
func NewInvoker(reg *Registry, auditor maudit.Auditor) *Invoker {
	if auditor == nil {
		auditor = maudit.NoopAuditor{}
	}
	return &Invoker{reg: reg, auditor: auditor}
}

// Invoke runs fn as capability capName's operation op. If the
// capability was not granted, fn is NOT run: a denied audit entry
// is emitted and [ErrCapabilityNotGranted] is returned. Otherwise
// fn is timed, its outcome audited (Success = fn returned nil), and
// its error propagated unchanged.
func (i *Invoker) Invoke(ctx context.Context, capName, op string, fn func(context.Context) error) error {
	mod, ver := "", ""
	if i.reg != nil {
		mod, ver = i.reg.module, i.reg.version
	}
	if i.reg == nil || !i.reg.Has(capName) {
		i.auditor.Emit(ctx, maudit.Entry{
			Timestamp:  time.Now().UTC(),
			Module:     mod,
			Version:    ver,
			Capability: capName,
			Operation:  "denied",
			Success:    false,
			Details:    map[string]string{"requested_operation": op},
		})
		return fmt.Errorf("%w: %q", ErrCapabilityNotGranted, capName)
	}
	start := time.Now()
	err := fn(ctx)
	i.auditor.Emit(ctx, maudit.Entry{
		Timestamp:  start.UTC(),
		Module:     mod,
		Version:    ver,
		Capability: capName,
		Operation:  op,
		Success:    err == nil,
		Duration:   time.Since(start),
	})
	return err
}
