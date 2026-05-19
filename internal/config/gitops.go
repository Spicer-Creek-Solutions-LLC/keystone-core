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
	return nil
}
