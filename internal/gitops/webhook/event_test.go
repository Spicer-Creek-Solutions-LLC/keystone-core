package webhook

import "testing"

func TestProvider_Valid(t *testing.T) {
	t.Parallel()
	cases := []struct {
		p    Provider
		want bool
	}{
		{ProviderArgoCD, true},
		{ProviderFlux, true},
		{ProviderGitHub, true},
		{ProviderGitLab, true},
		{Provider("argo"), false},
		{Provider(""), false},
		{Provider("ARGOCD"), false},
	}
	for _, c := range cases {
		t.Run(string(c.p), func(t *testing.T) {
			t.Parallel()
			if got := c.p.Valid(); got != c.want {
				t.Errorf("Provider(%q).Valid() = %v, want %v", c.p, got, c.want)
			}
		})
	}
}

func TestProvider_String(t *testing.T) {
	t.Parallel()
	if got := ProviderGitHub.String(); got != "github" {
		t.Errorf("String() = %q, want \"github\"", got)
	}
}
