// SPDX-License-Identifier: Apache-2.0

package webhook

import (
	"errors"
	"net/http"
)

// Provider-identifying request headers (Epic 16 task 2). Detection is
// by header *presence* — the value carries the provider's own event
// type and is parsed by the handler, not here. http.Header.Get
// canonicalizes keys, so inbound casing is irrelevant.
//
// ArgoCD/Flux do not emit their marker headers natively; operators
// configure them on the notification webhook (the documented contract,
// PROJECT-DETAILS §4.13).
const (
	HeaderGitHub = "X-GitHub-Event"
	HeaderGitLab = "X-Gitlab-Event"
	HeaderArgoCD = "X-Argo-CD-Webhook"
	HeaderFlux   = "X-Flux-Event"
)

// ErrNoProvider means no registered handler's detection header was
// present on the request.
var ErrNoProvider = errors.New("gitops/webhook: no provider header on request")

// ErrAmbiguousProvider means two or more registered handlers' detection
// headers were present. Rather than guess (a header-spoof vector before
// authentication lands in task 3), the receiver rejects deterministically.
var ErrAmbiguousProvider = errors.New("gitops/webhook: ambiguous provider (multiple source headers present)")

// Detect resolves the source provider from r's headers by scanning the
// detection header of every registered handler. Exactly one match
// returns that provider; zero matches returns [ErrNoProvider]; two or
// more returns [ErrAmbiguousProvider].
func (r *Registry) Detect(req *http.Request) (Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var found Provider
	matches := 0
	for p, h := range r.m {
		if req.Header.Get(h.DetectHeader()) != "" {
			found = p
			matches++
		}
	}
	switch matches {
	case 0:
		return "", ErrNoProvider
	case 1:
		return found, nil
	default:
		return "", ErrAmbiguousProvider
	}
}
