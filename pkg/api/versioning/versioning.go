// Package versioning tracks per-endpoint API lifecycle status
// (alpha/beta/current/supported/deprecated/retired) and emits standard
// deprecation/sunset headers on outbound responses.
//
// Backed by an in-memory Registry loaded at server startup. The auth
// chain runs first; this layer wraps handlers to inject headers
// (RFC 9745 Deprecation, RFC 8594 Sunset, RFC 8288 Link) and to refuse
// retired endpoints (HTTP 410, gRPC Unimplemented). See PROJECT-DETAILS
// §4.5.
package versioning

import (
	"fmt"
	"sync"
	"time"
)

// Status is an endpoint's declared lifecycle state.
type Status int

const (
	StatusUnspecified Status = iota
	StatusAlpha
	StatusBeta
	StatusCurrent
	StatusSupported
	StatusDeprecated
	StatusRetired
)

// String returns the canonical lower-case name.
func (s Status) String() string {
	switch s {
	case StatusAlpha:
		return "alpha"
	case StatusBeta:
		return "beta"
	case StatusCurrent:
		return "current"
	case StatusSupported:
		return "supported"
	case StatusDeprecated:
		return "deprecated"
	case StatusRetired:
		return "retired"
	default:
		return "unspecified"
	}
}

// ParseStatus accepts the canonical names. Empty string maps to
// StatusUnspecified.
func ParseStatus(s string) (Status, error) {
	switch s {
	case "alpha":
		return StatusAlpha, nil
	case "beta":
		return StatusBeta, nil
	case "current":
		return StatusCurrent, nil
	case "supported":
		return StatusSupported, nil
	case "deprecated":
		return StatusDeprecated, nil
	case "retired":
		return StatusRetired, nil
	case "":
		return StatusUnspecified, nil
	default:
		return StatusUnspecified, fmt.Errorf("versioning: unknown status %q", s)
	}
}

// Endpoint is one tracked API method.
type Endpoint struct {
	// Method is the gRPC fully-qualified method name
	// (/<package>.<Service>/<Method>). The HTTP middleware uses the
	// same key after URL-to-method translation.
	Method string

	// Status is the operator's declared lifecycle state. SunsetAt in
	// the past auto-overrides to StatusRetired regardless.
	Status Status

	// ReleasedAt records when the method first went live.
	ReleasedAt time.Time

	// DeprecatedAt: when set, drives the Deprecation header.
	DeprecatedAt time.Time

	// SunsetAt: when set, drives the Sunset header. When in the past,
	// the endpoint is treated as retired.
	SunsetAt time.Time

	// Replacement is the suggested successor method
	// (gRPC fully-qualified or HTTP path). Drives the Link header.
	Replacement string

	// Notes is free-form text intended for the Warning header.
	Notes string
}

// Registry is a thread-safe Endpoint store.
type Registry struct {
	mu        sync.RWMutex
	endpoints map[string]Endpoint
	now       func() time.Time
}

// NewRegistry returns an empty Registry using time.Now for the clock.
func NewRegistry() *Registry {
	return &Registry{
		endpoints: map[string]Endpoint{},
		now:       time.Now,
	}
}

// SetClock overrides the clock used by EffectiveStatus / IsRetired.
// Tests only.
func (r *Registry) SetClock(now func() time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.now = now
}

// Register adds or overwrites the entry for e.Method.
func (r *Registry) Register(e Endpoint) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.endpoints[e.Method] = e
}

// Lookup returns the entry for method (and ok=true) or a zero
// Endpoint + ok=false.
func (r *Registry) Lookup(method string) (Endpoint, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.endpoints[method]
	return e, ok
}

// EffectiveStatus returns the runtime status of method, applying the
// "SunsetAt in the past -> retired" override regardless of declared
// Status. Returns StatusUnspecified if method isn't registered.
func (r *Registry) EffectiveStatus(method string) Status {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.endpoints[method]
	if !ok {
		return StatusUnspecified
	}
	if !e.SunsetAt.IsZero() && r.now().After(e.SunsetAt) {
		return StatusRetired
	}
	return e.Status
}

// IsRetired reports whether method is at end-of-life and should be
// refused. Untracked methods are NOT retired (server keeps default
// behavior — the auth/RBAC layer is the gate for unknown endpoints).
func (r *Registry) IsRetired(method string) bool {
	return r.EffectiveStatus(method) == StatusRetired
}
