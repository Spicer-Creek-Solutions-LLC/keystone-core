package config

import "fmt"

// GitOpsConfig drives the Epic 16 GitOps integration. Task 1 ships the
// inbound webhook receiver sub-config only; verification, rollback and
// outbound webhooks extend this struct in later tasks.
//
//	gitops:
//	  webhook:
//	    enabled: false
//	    addr: ":8081"
//	    path: "/webhooks"
//	    max_body_bytes: 1048576
//	    sources:
//	      github:
//	        method: hmac          # none|hmac|bearer
//	        secret: ${KSCORE_GITOPS_WEBHOOK_SOURCES_GITHUB_SECRET}
//	      gitlab:
//	        method: bearer
//	        secret: ...
//
// Enabled defaults to false: the receiver opens a network port and
// (from task 4) re-emits onto the event bus, so it is opt-in rather
// than on-by-default infrastructure. Operators turn it on with
// `gitops.webhook.enabled: true`.
type GitOpsConfig struct {
	Webhook GitOpsWebhookConfig `koanf:"webhook"`
}

// GitOpsWebhookConfig is the inbound webhook receiver block. Addr
// defaults to ":8081" — distinct from the main REST API on
// server.httpport (:8080); colliding the two would wedge boot.
type GitOpsWebhookConfig struct {
	Enabled      bool   `koanf:"enabled"`
	Addr         string `koanf:"addr"`
	Path         string `koanf:"path"`
	MaxBodyBytes int64  `koanf:"max_body_bytes"`
	// Sources maps a provider (github|gitlab|argocd|flux) to its
	// inbound authentication. A provider absent from the map is
	// authenticated open ([webhook.NoneAuthenticator]) and flagged by
	// [Config.ProductionWarnings]. v1.0: one secret per source;
	// rotation requires a restart (PROJECT-DETAILS §4.13).
	Sources map[string]GitOpsSourceAuthConfig `koanf:"sources"`
}

// GitOpsSourceAuthConfig is one source's inbound auth. SignatureHeader
// / HeaderPrefix are optional — the receiver fills provider-aware
// defaults (GitHub → X-Hub-Signature-256/sha256=; GitLab →
// X-Gitlab-Token; generic → X-Signature/sha256=).
type GitOpsSourceAuthConfig struct {
	Method          string `koanf:"method"`
	Secret          string `koanf:"secret"`
	SignatureHeader string `koanf:"signature_header"`
	HeaderPrefix    string `koanf:"header_prefix"`
	RequireScheme   bool   `koanf:"require_scheme"`
}

// knownWebhookSources is the closed set of provider keys accepted in
// gitops.webhook.sources — mirrors webhook.Provider without importing
// internal/gitops (config must not depend on it).
var knownWebhookSources = map[string]struct{}{
	"github": {}, "gitlab": {}, "argocd": {}, "flux": {},
}

// applyGitOpsDefaults seeds the defaults operators inherit when they
// don't set the field. Called from defaultConfig().
func applyGitOpsDefaults(c *GitOpsConfig) {
	c.Webhook.Enabled = false
	c.Webhook.Addr = ":8081"
	c.Webhook.Path = "/webhooks"
	c.Webhook.MaxBodyBytes = 1 << 20 // 1 MiB
}

// Validate enforces structural invariants. Disabled is always OK so a
// default config validates without the receiver configured.
func (c *GitOpsConfig) Validate() error {
	if !c.Webhook.Enabled {
		return nil
	}
	if c.Webhook.Addr == "" {
		return fmt.Errorf("gitops.webhook.addr must be set when gitops.webhook.enabled (set gitops.webhook.enabled: false to opt out)")
	}
	if c.Webhook.Path == "" || c.Webhook.Path[0] != '/' {
		return fmt.Errorf("gitops.webhook.path must be an absolute path beginning with '/', got %q", c.Webhook.Path)
	}
	if c.Webhook.MaxBodyBytes <= 0 {
		return fmt.Errorf("gitops.webhook.max_body_bytes must be > 0 when enabled, got %d", c.Webhook.MaxBodyBytes)
	}
	for src, auth := range c.Webhook.Sources {
		if _, ok := knownWebhookSources[src]; !ok {
			return fmt.Errorf("gitops.webhook.sources: unknown provider %q (want github|gitlab|argocd|flux)", src)
		}
		switch auth.Method {
		case "none":
		case "hmac", "bearer":
			if auth.Secret == "" {
				return fmt.Errorf("gitops.webhook.sources.%s: %s requires a non-empty secret", src, auth.Method)
			}
		case "":
			return fmt.Errorf("gitops.webhook.sources.%s: method is required (none|hmac|bearer)", src)
		default:
			return fmt.Errorf("gitops.webhook.sources.%s: unknown method %q (want none|hmac|bearer)", src, auth.Method)
		}
	}
	return nil
}

// UnauthenticatedWebhookSources reports the gitops webhook sources that
// would be served without authentication: any configured `none` source
// plus every known provider absent from the map (defaulted open). Empty
// when the receiver is disabled. Drives a production warning.
func (c *GitOpsConfig) UnauthenticatedWebhookSources() []string {
	if !c.Webhook.Enabled {
		return nil
	}
	var open []string
	for src := range knownWebhookSources {
		a, ok := c.Webhook.Sources[src]
		if !ok || a.Method == "none" {
			open = append(open, src)
		}
	}
	return open
}
