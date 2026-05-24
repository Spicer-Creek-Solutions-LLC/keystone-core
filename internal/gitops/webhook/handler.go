// SPDX-License-Identifier: Apache-2.0

package webhook

import (
	"errors"
	"fmt"
	"net/http"
	"sync"
)

// ErrUnknownProvider is returned by [Registry.Lookup]'s callers when a
// request resolves to a provider with no registered [Handler].
var ErrUnknownProvider = errors.New("gitops/webhook: unknown provider")

// Handler parses one provider's webhook payload into a unified [Event].
//
// Parse receives the original *http.Request (for provider headers such
// as GitHub's `X-GitHub-Event`) and the already-read request body —
// the receiver reads the body once, size-capped, so handlers must not
// read r.Body. Implementations must not mutate r. A parse failure
// (malformed JSON, missing required identity) returns a non-nil error
// and the zero Event.
type Handler interface {
	// Type returns the provider this handler parses.
	Type() Provider
	// DetectHeader returns the request header whose presence
	// identifies this provider (e.g. "X-GitHub-Event"). The
	// [Registry.Detect] auto-detection scans these.
	DetectHeader() string
	// Parse maps the request + body to a normalized Event.
	Parse(r *http.Request, body []byte) (Event, error)
}

// Registry maps [Provider] → [Handler]. Safe for concurrent use; the
// receiver only reads it while serving.
type Registry struct {
	mu sync.RWMutex
	m  map[Provider]Handler
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{m: make(map[Provider]Handler)}
}

// Register binds a handler to its [Handler.Type]. Re-registering a
// provider overwrites. A nil handler, or one whose Type is not a known
// [Provider], is rejected.
func (r *Registry) Register(h Handler) error {
	if h == nil {
		return errors.New("gitops/webhook: nil handler")
	}
	p := h.Type()
	if !p.Valid() {
		return fmt.Errorf("gitops/webhook: handler reports invalid provider %q", p)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[p] = h
	return nil
}

// Lookup returns the handler for p, or false.
func (r *Registry) Lookup(p Provider) (Handler, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	h, ok := r.m[p]
	return h, ok
}
