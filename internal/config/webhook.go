// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"time"
)

// WebhookConfig drives the outbound webhooks subsystem (Epic 16
// tasks 11..18 / PROJECT-DETAILS §4.14). The top-level `webhook` key
// is owned by outbound for v1.0; inbound non-GitOps webhooks are
// v1.x (the GitOps inbound block lives at `gitops.webhook`).
//
//	webhook:
//	  outbound:
//	    enabled: false
//	    max_retries: 3
//	    retry_backoff: 1s
//	    timeout: 10s
//	    max_payload_size: 1048576
//	    delivery_retention: 168h
//	    max_concurrent_deliveries: 32
//	    refresh_interval: 30s
//
// Enabled defaults to false (opt-in; the same posture as
// `gitops.webhook.enabled`).
type WebhookConfig struct {
	Outbound WebhookOutboundConfig `koanf:"outbound"`
}

// WebhookOutboundConfig is the outbound-webhooks block.
//
//   - MaxRetries / RetryBackoff drive the task-14 RetryQueue.
//   - Timeout caps each task-13 Dispatcher.Deliver call.
//   - MaxPayloadSize bounds the JSON event payload — over-size
//     events are recorded as one synthetic `failed` delivery.
//   - DeliveryRetention is the task-9 / task-11 retention enforcer
//     horizon (auto-invocation = post-v1.0 dot release per §4.14).
//   - MaxConcurrentDeliveries / RefreshInterval drive the task-12
//     Manager (fan-out cap and subscription-cache reload cadence).
type WebhookOutboundConfig struct {
	Enabled                 bool          `koanf:"enabled"`
	MaxRetries              int           `koanf:"max_retries"`
	RetryBackoff            time.Duration `koanf:"retry_backoff"`
	Timeout                 time.Duration `koanf:"timeout"`
	MaxPayloadSize          int64         `koanf:"max_payload_size"`
	DeliveryRetention       time.Duration `koanf:"delivery_retention"`
	MaxConcurrentDeliveries int           `koanf:"max_concurrent_deliveries"`
	RefreshInterval         time.Duration `koanf:"refresh_interval"`
}

// applyWebhookDefaults seeds §4.14 defaults plus the task-12 tunables.
func applyWebhookDefaults(c *WebhookConfig) {
	c.Outbound.Enabled = false
	c.Outbound.MaxRetries = 3
	c.Outbound.RetryBackoff = time.Second
	c.Outbound.Timeout = 10 * time.Second
	c.Outbound.MaxPayloadSize = 1 << 20 // 1 MiB
	c.Outbound.DeliveryRetention = 7 * 24 * time.Hour
	c.Outbound.MaxConcurrentDeliveries = 32
	c.Outbound.RefreshInterval = 30 * time.Second
}

// Validate enforces structural invariants. Disabled is always OK so a
// default config validates without the subsystem configured.
func (c *WebhookConfig) Validate() error {
	o := &c.Outbound
	if !o.Enabled {
		return nil
	}
	if o.MaxRetries < 0 {
		return fmt.Errorf("webhook.outbound.max_retries must be >= 0, got %d", o.MaxRetries)
	}
	if o.RetryBackoff < 0 {
		return fmt.Errorf("webhook.outbound.retry_backoff must be >= 0, got %v", o.RetryBackoff)
	}
	if o.Timeout <= 0 {
		return fmt.Errorf("webhook.outbound.timeout must be > 0 when enabled, got %v", o.Timeout)
	}
	if o.MaxPayloadSize <= 0 {
		return fmt.Errorf("webhook.outbound.max_payload_size must be > 0 when enabled, got %d", o.MaxPayloadSize)
	}
	if o.DeliveryRetention <= 0 {
		return fmt.Errorf("webhook.outbound.delivery_retention must be > 0 when enabled, got %v", o.DeliveryRetention)
	}
	if o.MaxConcurrentDeliveries <= 0 {
		return fmt.Errorf("webhook.outbound.max_concurrent_deliveries must be > 0 when enabled, got %d", o.MaxConcurrentDeliveries)
	}
	if o.RefreshInterval < 0 {
		return fmt.Errorf("webhook.outbound.refresh_interval must be >= 0, got %v", o.RefreshInterval)
	}
	return nil
}
