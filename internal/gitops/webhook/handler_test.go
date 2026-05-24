// SPDX-License-Identifier: Apache-2.0

package webhook

import (
	"net/http"
	"testing"
)

// badHandler reports an invalid provider, exercising Register's guard.
type badHandler struct{}

func (badHandler) Type() Provider                             { return Provider("nope") }
func (badHandler) DetectHeader() string                       { return "X-Nope" }
func (badHandler) Parse(*http.Request, []byte) (Event, error) { return Event{}, nil }

func TestRegistry_RegisterLookup(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()

	if err := reg.Register(ArgoCDHandler{}); err != nil {
		t.Fatalf("Register(ArgoCD): %v", err)
	}
	h, ok := reg.Lookup(ProviderArgoCD)
	if !ok {
		t.Fatal("Lookup(argocd) = !ok, want ok")
	}
	if h.Type() != ProviderArgoCD {
		t.Errorf("looked-up handler Type() = %q, want argocd", h.Type())
	}
	if _, ok := reg.Lookup(ProviderFlux); ok {
		t.Error("Lookup(flux) = ok, want !ok (not registered)")
	}
}

func TestRegistry_Register_Errors(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	if err := reg.Register(nil); err == nil {
		t.Error("Register(nil) = nil, want error")
	}
	if err := reg.Register(badHandler{}); err == nil {
		t.Error("Register(badHandler) = nil, want error (invalid provider)")
	}
}

func TestRegistry_Register_Overwrites(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	_ = reg.Register(GitHubHandler{})
	if err := reg.Register(GitHubHandler{}); err != nil {
		t.Fatalf("re-Register(github): %v", err)
	}
	if _, ok := reg.Lookup(ProviderGitHub); !ok {
		t.Error("Lookup(github) = !ok after re-register")
	}
}

func TestRegisterAll_And_DefaultRegistry(t *testing.T) {
	t.Parallel()
	reg := NewDefaultRegistry()
	for _, p := range []Provider{ProviderArgoCD, ProviderFlux, ProviderGitHub, ProviderGitLab} {
		if _, ok := reg.Lookup(p); !ok {
			t.Errorf("default registry missing handler for %q", p)
		}
	}
}
