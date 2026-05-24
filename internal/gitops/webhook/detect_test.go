// SPDX-License-Identifier: Apache-2.0

package webhook

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func detectReq(t *testing.T, headers map[string]string) *http.Request {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/webhooks", nil)
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	return r
}

func TestRegistry_Detect(t *testing.T) {
	t.Parallel()
	reg := NewDefaultRegistry()
	cases := []struct {
		name    string
		headers map[string]string
		want    Provider
		wantErr error
	}{
		{"github", map[string]string{HeaderGitHub: "push"}, ProviderGitHub, nil},
		{"gitlab", map[string]string{HeaderGitLab: "Push Hook"}, ProviderGitLab, nil},
		{"argocd", map[string]string{HeaderArgoCD: "true"}, ProviderArgoCD, nil},
		{"flux", map[string]string{HeaderFlux: "Kustomization"}, ProviderFlux, nil},
		{
			name:    "lowercase header still detected (canonicalization)",
			headers: map[string]string{"x-github-event": "push"},
			want:    ProviderGitHub,
		},
		{"none", nil, "", ErrNoProvider},
		{"empty value is not present", map[string]string{HeaderGitHub: ""}, "", ErrNoProvider},
		{
			name:    "ambiguous",
			headers: map[string]string{HeaderGitHub: "push", HeaderFlux: "Kustomization"},
			wantErr: ErrAmbiguousProvider,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := reg.Detect(detectReq(t, tc.headers))
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("Detect = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRegistry_Detect_OnlyScansRegistered(t *testing.T) {
	t.Parallel()
	// Empty registry → every request resolves to ErrNoProvider even
	// with a well-formed provider header present.
	reg := NewRegistry()
	_, err := reg.Detect(detectReq(t, map[string]string{HeaderGitHub: "push"}))
	if !errors.Is(err, ErrNoProvider) {
		t.Errorf("err = %v, want ErrNoProvider", err)
	}
}
