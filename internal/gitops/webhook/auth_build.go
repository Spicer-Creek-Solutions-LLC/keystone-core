package webhook

import "fmt"

// AuthSpec is the package-local, config-agnostic description of one
// source's authentication. The boot/CLI layer translates
// config.GitOpsSourceAuthConfig into this; the webhook package keeps no
// internal/config import (mirrors [ReceiverConfig]).
//
// SignatureHeader/HeaderPrefix are optional: when empty,
// [BuildAuthenticators] fills provider-aware defaults so operators
// rarely set them by hand.
type AuthSpec struct {
	Method          AuthMethod
	Secret          string
	SignatureHeader string
	HeaderPrefix    string
	RequireScheme   bool
}

// providerAuthDefaults supplies the well-known signature header +
// prefix per provider when the spec omits them. The generic fallback
// (used for ArgoCD/Flux, whose markers are operator-configured) is a
// bare "X-Signature: sha256=<hex>".
func providerAuthDefaults(p Provider) (header, prefix string) {
	switch p {
	case ProviderGitHub:
		return "X-Hub-Signature-256", "sha256="
	case ProviderGitLab:
		return "X-Gitlab-Token", ""
	default: // ArgoCD, Flux, future providers
		return "X-Signature", "sha256="
	}
}

// BuildAuthenticators turns per-provider specs into authenticators,
// applying provider defaults. A provider absent from specs is the
// caller's concern — the receiver defaults it to [NoneAuthenticator].
//
// Errors: an unknown [AuthMethod], or an empty Secret for hmac/bearer
// (a None source needs no secret). The error names the provider so a
// config typo is easy to locate.
func BuildAuthenticators(specs map[Provider]AuthSpec) (map[Provider]Authenticator, error) {
	out := make(map[Provider]Authenticator, len(specs))
	for p, s := range specs {
		header, prefix := providerAuthDefaults(p)
		if s.SignatureHeader != "" {
			header = s.SignatureHeader
		}
		if s.HeaderPrefix != "" {
			prefix = s.HeaderPrefix
		}
		switch s.Method {
		case AuthNone:
			out[p] = NoneAuthenticator{}
		case AuthHMAC:
			if s.Secret == "" {
				return nil, fmt.Errorf("gitops/webhook: source %q: hmac requires a non-empty secret", p)
			}
			out[p] = HMACAuthenticator{
				Secret:          []byte(s.Secret),
				SignatureHeader: header,
				Prefix:          prefix,
			}
		case AuthBearer:
			if s.Secret == "" {
				return nil, fmt.Errorf("gitops/webhook: source %q: bearer requires a non-empty secret", p)
			}
			out[p] = BearerAuthenticator{
				Header:        header,
				Token:         []byte(s.Secret),
				RequireScheme: s.RequireScheme,
			}
		default:
			return nil, fmt.Errorf("gitops/webhook: source %q: unknown auth method %q (want none|hmac|bearer)", p, s.Method)
		}
	}
	return out, nil
}
