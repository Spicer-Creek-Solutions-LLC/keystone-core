// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"go.keystone-core.io/keystone-core/internal/config"
	"go.keystone-core.io/keystone-core/internal/events"
	"go.keystone-core.io/keystone-core/internal/webhook/outbound"
	"go.keystone-core.io/keystone-core/pkg/api/webhooks"
)

// webhookOutboundRuntime carries the live outbound-webhook
// infrastructure constructed at boot. Stop tears down in reverse.
type webhookOutboundRuntime struct {
	Manager *outbound.Manager
	Store   *outbound.SQLiteStore
	Sub     events.Subscription
}

func (r *webhookOutboundRuntime) stop(ctx context.Context, log *slog.Logger) {
	if r == nil {
		return
	}
	if r.Sub != nil {
		if err := r.Sub.Unsubscribe(); err != nil {
			log.LogAttrs(ctx, slog.LevelWarn, "outbound webhook: unsubscribe", slog.String("err", err.Error()))
		}
	}
	if r.Manager != nil {
		if err := r.Manager.Stop(ctx); err != nil {
			log.LogAttrs(ctx, slog.LevelWarn, "outbound webhook: manager stop", slog.String("err", err.Error()))
		}
	}
	if r.Store != nil {
		if err := r.Store.Close(); err != nil {
			log.LogAttrs(ctx, slog.LevelWarn, "outbound webhook: store close", slog.String("err", err.Error()))
		}
	}
}

// startOutboundWebhook builds the outbound-webhook subsystem when
// cfg.Webhook.Outbound.Enabled. Returns (nil, nil) when disabled so
// the REST handler runs against an empty Providers (503s on every
// route — the documented not-yet-wired posture).
//
// The SQLite store lives next to the rest of kscore-server's local
// state (./data/webhooks.db relative to working dir). v1.x can
// migrate this onto the Postgres store alongside other domains.
func startOutboundWebhook(
	ctx context.Context,
	cfg config.WebhookOutboundConfig,
	clusterName string,
	subscriber events.EventSubscriber,
	log *slog.Logger,
) (*webhookOutboundRuntime, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	if err := os.MkdirAll("data", 0o750); err != nil {
		return nil, fmt.Errorf("outbound webhook: data dir: %w", err)
	}
	dbPath := filepath.Join("data", "webhooks.db")
	store, err := outbound.NewSQLiteStore(dbPath)
	if err != nil {
		return nil, fmt.Errorf("outbound webhook: store: %w", err)
	}

	mgr := &outbound.Manager{
		Store:                   store,
		Dispatcher:              &outbound.HTTPDispatcher{DefaultTimeout: cfg.Timeout},
		Logger:                  log,
		MaxConcurrentDeliveries: cfg.MaxConcurrentDeliveries,
		MaxPayloadBytes:         cfg.MaxPayloadSize,
		RefreshInterval:         cfg.RefreshInterval,
		Retry: outbound.RetryPolicy{
			BaseBackoff: cfg.RetryBackoff,
		},
	}
	if err := mgr.Start(ctx); err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("outbound webhook: manager start: %w", err)
	}

	// Subscribe to the full events firehose for this cluster. Subject
	// pattern must be a subset of the JetStream events stream's
	// subjects (kscore.<cluster>.events.>) — overly-broad patterns
	// like "kscore.>" don't match any stream.
	pattern := fmt.Sprintf("kscore.%s.events.>", clusterName)
	sub, err := subscriber.Subscribe(ctx, pattern, func(hctx context.Context, ev events.Event) error {
		mgr.Handle(hctx, ev)
		return nil
	})
	if err != nil {
		_ = mgr.Stop(context.Background())
		_ = store.Close()
		return nil, fmt.Errorf("outbound webhook: subscribe: %w", err)
	}

	log.Info("outbound webhook subsystem started",
		"max_concurrent_deliveries", cfg.MaxConcurrentDeliveries,
		"max_retries", cfg.MaxRetries,
	)
	return &webhookOutboundRuntime{Manager: mgr, Store: store, Sub: sub}, nil
}

// providersFrom builds the REST handler's Providers from a started
// runtime. Returns the zero-value when rt is nil so the routes 503.
func (r *webhookOutboundRuntime) providersFrom() webhooks.Providers {
	if r == nil {
		return webhooks.Providers{}
	}
	return webhooks.Providers{Store: r.Manager.Store, Manager: r.Manager}
}
