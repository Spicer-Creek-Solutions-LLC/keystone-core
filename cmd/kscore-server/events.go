package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"

	"go.keystone-core.io/keystone-core/internal/config"
	"go.keystone-core.io/keystone-core/internal/events"
	natsmgr "go.keystone-core.io/keystone-core/internal/nats"
	"go.keystone-core.io/keystone-core/internal/state"
)

// eventsRuntime carries the live events infrastructure that the
// kscore-server boot wiring constructs from [config.EventsConfig]
// and that the server passes through to its API layer via
// [server.Options].
type eventsRuntime struct {
	Store      events.EventStore
	Publisher  events.EventPublisher
	Subscriber events.EventSubscriber
}

// stop tears down the runtime in reverse order. Logs every error +
// continues — best-effort shutdown.
func (r *eventsRuntime) stop(ctx context.Context, log *slog.Logger) {
	if r == nil {
		return
	}
	if r.Subscriber != nil {
		if err := r.Subscriber.Stop(ctx); err != nil {
			log.LogAttrs(ctx, slog.LevelWarn, "events: subscriber stop", slog.String("err", err.Error()))
		}
	}
	if r.Publisher != nil {
		if err := r.Publisher.Stop(ctx); err != nil {
			log.LogAttrs(ctx, slog.LevelWarn, "events: publisher stop", slog.String("err", err.Error()))
		}
	}
	if r.Store != nil {
		// Close is a no-op for the SQL wrapper (state.Store owns the
		// connection pool) but kept for symmetry with the interface.
		if err := r.Store.Close(); err != nil {
			log.LogAttrs(ctx, slog.LevelWarn, "events: store close", slog.String("err", err.Error()))
		}
	}
}

// startEvents constructs the events runtime per [config.EventsConfig].
// Returns nil + nil error when events is disabled in config; the
// caller treats nil as "no events surface configured" and the gRPC
// EventService returns Unavailable / REST returns 503.
//
// nm is the started NATS manager; events depends on JetStream being
// available. Validation (config.EventsConfig.Validate) already
// ensures NATS+JetStream is enabled when events is enabled, so this
// function reports a hard error if the manager refuses to hand back
// a JetStream context.
func startEvents(ctx context.Context, cfg config.EventsConfig, natsCfg config.NATSConfig, store state.Store, nm *natsmgr.Manager, log *slog.Logger) (*eventsRuntime, error) {
	if !cfg.Enabled {
		log.LogAttrs(ctx, slog.LevelInfo, "events: disabled in config; skipping")
		return nil, nil
	}

	// Store is always cheap — the SQL wrapper just records the
	// underlying state.Store. Constructed even when publisher /
	// subscriber are individually disabled so the REST list/get
	// surface stays available.
	rt := &eventsRuntime{
		Store: events.NewSQLEventStore(store),
	}

	// Publisher + subscriber both need a JetStream context. Asking
	// the manager up-front so failures surface before construction.
	var js nats.JetStreamContext
	if cfg.Publisher.Enabled || cfg.Subscriber.Enabled {
		got, err := nm.JetStream()
		if err != nil {
			return nil, fmt.Errorf("events: jetstream: %w", err)
		}
		js = got
	}

	clusterName := natsCfg.ClusterName

	if cfg.Publisher.Enabled {
		pubOpts := []events.PublisherOption{
			events.WithBufferSize(cfg.Publisher.BufferSize),
			events.WithFlushTimeout(cfg.Publisher.FlushTimeout),
			events.WithLogger(log),
		}
		if cfg.Publisher.StoreFirst {
			pubOpts = append(pubOpts, events.WithStore(rt.Store))
		}
		pub := events.NewJetStreamPublisher(js, clusterName, pubOpts...)
		if err := pub.Start(ctx); err != nil {
			return nil, fmt.Errorf("events: publisher start: %w", err)
		}
		rt.Publisher = pub
	}

	if cfg.Subscriber.Enabled {
		subOpts := []events.SubscriberOption{
			events.WithSubscriberStore(rt.Store),
			events.WithSubscriberLogger(log),
			events.WithDedupSize(cfg.Subscriber.DedupSize),
		}
		sub := events.NewJetStreamSubscriber(js, clusterName, subOpts...)
		if err := sub.Start(ctx); err != nil {
			if rt.Publisher != nil {
				_ = rt.Publisher.Stop(ctx)
			}
			return nil, fmt.Errorf("events: subscriber start: %w", err)
		}
		rt.Subscriber = sub
	}

	log.LogAttrs(ctx, slog.LevelInfo, "events: enabled",
		slog.Bool("publisher_enabled", cfg.Publisher.Enabled),
		slog.Bool("subscriber_enabled", cfg.Subscriber.Enabled),
		slog.Bool("store_first", cfg.Publisher.StoreFirst),
		slog.String("cluster", clusterName),
	)
	return rt, nil
}

// stopEventsCtx bounds the per-component stop wait at 10s so a stuck
// publisher / subscriber doesn't hang the whole shutdown.
func stopEventsCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 10*time.Second)
}
