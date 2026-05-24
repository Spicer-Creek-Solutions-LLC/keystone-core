// SPDX-License-Identifier: Apache-2.0

package webhook

// RegisterAll binds the four v1.0 provider handlers (ArgoCD, Flux,
// GitHub, GitLab) onto reg. It is the single wiring point callers use
// to build a fully populated [Registry]; mirrors the runbook step
// registry's RegisterAll pattern so the set is enumerated in one place.
func RegisterAll(reg *Registry) error {
	handlers := []Handler{
		ArgoCDHandler{},
		FluxHandler{},
		GitHubHandler{},
		GitLabHandler{},
	}
	for _, h := range handlers {
		if err := reg.Register(h); err != nil {
			return err
		}
	}
	return nil
}

// NewDefaultRegistry returns a [Registry] with all four v1.0 handlers
// registered. Panics only on a programming error (a handler reporting
// an invalid provider), which RegisterAll cannot hit with the built-in
// set — kept as a guard for future additions.
func NewDefaultRegistry() *Registry {
	reg := NewRegistry()
	if err := RegisterAll(reg); err != nil {
		panic("gitops/webhook: built-in handler registration failed: " + err.Error())
	}
	return reg
}
