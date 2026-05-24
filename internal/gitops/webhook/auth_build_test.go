// SPDX-License-Identifier: Apache-2.0

package webhook

import (
	"strings"
	"testing"
)

func TestBuildAuthenticators_Defaults(t *testing.T) {
	t.Parallel()
	auths, err := BuildAuthenticators(map[Provider]AuthSpec{
		ProviderGitHub: {Method: AuthHMAC, Secret: "gh"},
		ProviderGitLab: {Method: AuthBearer, Secret: "gl"},
		ProviderArgoCD: {Method: AuthHMAC, Secret: "ac"},
		ProviderFlux:   {Method: AuthNone},
	})
	if err != nil {
		t.Fatalf("BuildAuthenticators: %v", err)
	}

	gh, ok := auths[ProviderGitHub].(HMACAuthenticator)
	if !ok {
		t.Fatalf("github = %T, want HMACAuthenticator", auths[ProviderGitHub])
	}
	if gh.SignatureHeader != "X-Hub-Signature-256" || gh.Prefix != "sha256=" {
		t.Errorf("github defaults = %q/%q, want X-Hub-Signature-256/sha256=", gh.SignatureHeader, gh.Prefix)
	}
	gl, ok := auths[ProviderGitLab].(BearerAuthenticator)
	if !ok {
		t.Fatalf("gitlab = %T, want BearerAuthenticator", auths[ProviderGitLab])
	}
	if gl.Header != "X-Gitlab-Token" {
		t.Errorf("gitlab default header = %q, want X-Gitlab-Token", gl.Header)
	}
	ac := auths[ProviderArgoCD].(HMACAuthenticator)
	if ac.SignatureHeader != "X-Signature" {
		t.Errorf("argocd default header = %q, want X-Signature (generic fallback)", ac.SignatureHeader)
	}
	if _, ok := auths[ProviderFlux].(NoneAuthenticator); !ok {
		t.Errorf("flux = %T, want NoneAuthenticator", auths[ProviderFlux])
	}
}

func TestBuildAuthenticators_HeaderOverride(t *testing.T) {
	t.Parallel()
	auths, err := BuildAuthenticators(map[Provider]AuthSpec{
		ProviderGitHub: {Method: AuthHMAC, Secret: "x", SignatureHeader: "X-Custom", HeaderPrefix: "v1="},
	})
	if err != nil {
		t.Fatal(err)
	}
	gh := auths[ProviderGitHub].(HMACAuthenticator)
	if gh.SignatureHeader != "X-Custom" || gh.Prefix != "v1=" {
		t.Errorf("override = %q/%q, want X-Custom/v1=", gh.SignatureHeader, gh.Prefix)
	}
}

func TestBuildAuthenticators_Errors(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		spec map[Provider]AuthSpec
		want string
	}{
		{"hmac no secret", map[Provider]AuthSpec{ProviderGitHub: {Method: AuthHMAC}}, "hmac requires"},
		{"bearer no secret", map[Provider]AuthSpec{ProviderGitLab: {Method: AuthBearer}}, "bearer requires"},
		{"unknown method", map[Provider]AuthSpec{ProviderFlux: {Method: AuthMethod("weird")}}, "unknown auth method"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := BuildAuthenticators(tc.spec)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestBuildAuthenticators_Empty(t *testing.T) {
	t.Parallel()
	auths, err := BuildAuthenticators(nil)
	if err != nil {
		t.Fatalf("nil specs: %v", err)
	}
	if len(auths) != 0 {
		t.Errorf("len = %d, want 0 (receiver defaults absent providers to None)", len(auths))
	}
}
