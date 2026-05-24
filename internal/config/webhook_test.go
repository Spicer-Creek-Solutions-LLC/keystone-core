// SPDX-License-Identifier: Apache-2.0

package config

import (
	"strings"
	"testing"
	"time"
)

func TestWebhookConfig_Defaults(t *testing.T) {
	t.Parallel()
	c := defaultConfig()
	o := c.Webhook.Outbound
	if o.Enabled {
		t.Errorf("Enabled = true, want false (opt-in)")
	}
	if o.MaxRetries != 3 || o.RetryBackoff != time.Second || o.Timeout != 10*time.Second {
		t.Errorf("retry defaults wrong: %+v", o)
	}
	if o.MaxPayloadSize != 1<<20 {
		t.Errorf("MaxPayloadSize = %d, want 1 MiB", o.MaxPayloadSize)
	}
	if o.DeliveryRetention != 7*24*time.Hour {
		t.Errorf("DeliveryRetention = %v, want 168h", o.DeliveryRetention)
	}
	if o.MaxConcurrentDeliveries != 32 || o.RefreshInterval != 30*time.Second {
		t.Errorf("manager tunable defaults wrong: %+v", o)
	}
}

func TestWebhookConfig_Validate_DisabledIsAlwaysOK(t *testing.T) {
	t.Parallel()
	c := WebhookConfig{Outbound: WebhookOutboundConfig{Enabled: false, MaxRetries: -1, Timeout: 0}}
	if err := c.Validate(); err != nil {
		t.Errorf("disabled validates: got %v", err)
	}
}

func TestWebhookConfig_Validate_Enabled(t *testing.T) {
	t.Parallel()
	base := WebhookOutboundConfig{
		Enabled:                 true,
		MaxRetries:              3,
		RetryBackoff:            time.Second,
		Timeout:                 10 * time.Second,
		MaxPayloadSize:          1024,
		DeliveryRetention:       time.Hour,
		MaxConcurrentDeliveries: 4,
		RefreshInterval:         30 * time.Second,
	}
	cases := []struct {
		name    string
		mut     func(*WebhookOutboundConfig)
		wantErr string
	}{
		{"valid", func(*WebhookOutboundConfig) {}, ""},
		{"negative retries", func(o *WebhookOutboundConfig) { o.MaxRetries = -1 }, "max_retries"},
		{"negative backoff", func(o *WebhookOutboundConfig) { o.RetryBackoff = -1 }, "retry_backoff"},
		{"zero timeout", func(o *WebhookOutboundConfig) { o.Timeout = 0 }, "timeout"},
		{"zero payload", func(o *WebhookOutboundConfig) { o.MaxPayloadSize = 0 }, "max_payload_size"},
		{"zero retention", func(o *WebhookOutboundConfig) { o.DeliveryRetention = 0 }, "delivery_retention"},
		{"zero concurrency", func(o *WebhookOutboundConfig) { o.MaxConcurrentDeliveries = 0 }, "max_concurrent_deliveries"},
		{"negative refresh", func(o *WebhookOutboundConfig) { o.RefreshInterval = -1 }, "refresh_interval"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			o := base
			tc.mut(&o)
			err := (&WebhookConfig{Outbound: o}).Validate()
			switch {
			case tc.wantErr == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
				t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}
