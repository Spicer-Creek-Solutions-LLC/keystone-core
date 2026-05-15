package events

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
)

// JetStreamPublisher is the v1.0 implementation of [EventPublisher].
// Sync Publish goes straight to JetStream; PublishAsync enqueues onto
// a bounded channel that a single background worker drains through
// the same sync path.
//
// Store-first semantics: when [WithStore] is configured, every
// publish path persists to the store before NATS. A store failure
// aborts the publish (NATS untouched and the caller's error chain
// surfaces the store error). A NATS failure after a successful
// store write still returns an error to the caller, but the row
// remains queryable from the store — subscribers can recover via
// historical replay (task 4).
//
// The publisher does NOT manage the JetStream stream itself —
// [internal/nats.Manager.ensureStreams] owns lifecycle. The
// publisher merely requires that the stream's subject filter covers
// `kscore.<cluster>.events.>` (which [internal/nats.DefaultStreamDefs]
// configures by default).
//
// Concurrent use: Publish and PublishAsync are safe for concurrent
// calls from multiple goroutines. Start / Stop are NOT — they MUST
// be serialised by the caller (typical boot wiring is single-threaded
// in `cmd/kscore-server`).
type JetStreamPublisher struct {
	js          nats.JetStreamContext
	clusterName string
	cfg         publisherConfig

	// Async pipeline.
	queue     chan Event
	wg        sync.WaitGroup
	workerCtx context.Context
	cancel    context.CancelFunc
	stopCh    chan struct{}
	started   atomic.Bool

	// Metrics.
	failedPublishes atomic.Int64
}

// NewJetStreamPublisher builds a publisher against the given JetStream
// context + cluster name. Cluster name is used to stamp empty
// [Event.Subject] fields via [Event.StampSubject] at publish time;
// MUST match the cluster the stream was configured for (otherwise
// the publish lands on a subject the stream doesn't capture and
// JetStream returns a no-responders error).
//
// The returned publisher is unstarted — call [JetStreamPublisher.Start]
// before any publish.
func NewJetStreamPublisher(js nats.JetStreamContext, clusterName string, opts ...PublisherOption) *JetStreamPublisher {
	cfg := defaultPublisherConfig()
	for _, opt := range opts {
		opt(&cfg)
	}
	return &JetStreamPublisher{
		js:          js,
		clusterName: clusterName,
		cfg:         cfg,
		queue:       make(chan Event, cfg.bufferSize),
		stopCh:      make(chan struct{}),
	}
}

// Start launches the async worker goroutine. Double-Start without an
// intervening Stop is rejected — the channel + WaitGroup are recreated
// by Stop, not by Start, so a fresh Start on a stopped publisher
// needs explicit re-initialisation (which we don't support in v1.0;
// publishers are constructed-once per process boot).
//
// Start's ctx is not propagated to the worker goroutine — the worker
// owns its own cancelable context derived in this call (workerCtx),
// cancelled by Stop. This pattern mirrors Epic 10 task 6's LeaseManager
// scheduler loop and keeps gosec G118 happy.
func (p *JetStreamPublisher) Start(_ context.Context) error {
	if !p.started.CompareAndSwap(false, true) {
		return fmt.Errorf("events: publisher already started")
	}
	p.workerCtx, p.cancel = context.WithCancel(context.Background())
	p.wg.Add(1)
	go p.run(p.workerCtx)
	return nil
}

// Stop signals the async worker to drain and exit. Blocks until the
// worker returns OR the caller's context expires (whichever happens
// first). Idempotent — calling Stop on a never-started or already-
// stopped publisher returns nil.
//
// When the caller's context expires before the worker drains, Stop
// cancels the worker's context to release any in-flight publish
// (otherwise a hung publish could block shutdown indefinitely).
func (p *JetStreamPublisher) Stop(ctx context.Context) error {
	if !p.started.CompareAndSwap(true, false) {
		return nil
	}
	close(p.stopCh)

	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		p.cancel()
		return nil
	case <-ctx.Done():
		// Worker still draining — cancel its context to release any
		// blocked publish, then return the caller's error.
		p.cancel()
		return ctx.Err()
	}
}

// Publish is the synchronous publish path. Validates the event,
// stamps Subject if empty, persists to the configured store, then
// publishes to JetStream. Every step's error is returned to the
// caller; on a store failure, NATS is never called.
func (p *JetStreamPublisher) Publish(ctx context.Context, e Event) error {
	if !p.started.Load() {
		return ErrPublisherNotStarted
	}
	return p.publishOne(ctx, e)
}

// PublishAsync validates the event synchronously (so invalid events
// never enter the queue) then enqueues for background publish. Returns
// [ErrPublisherBufferFull] when the buffer is at capacity and the
// configured flush timeout elapses. Returns the caller's ctx error
// when the caller's context cancels before enqueue completes.
func (p *JetStreamPublisher) PublishAsync(ctx context.Context, e Event) error {
	if !p.started.Load() {
		return ErrPublisherNotStarted
	}
	if err := e.Validate(); err != nil {
		return err
	}
	// Stamp Subject ahead of enqueue so the worker doesn't have to
	// know about the cluster name — keeps the queue payload self-
	// sufficient and the worker stateless.
	if e.Subject == "" {
		if _, err := e.StampSubject(p.clusterName); err != nil {
			return err
		}
	}

	// Bounded enqueue: try immediate; fall back to timed wait;
	// then ctx-cancel; then ErrPublisherBufferFull.
	select {
	case p.queue <- e:
		return nil
	default:
	}

	timer := time.NewTimer(p.cfg.flushTimeout)
	defer timer.Stop()
	select {
	case p.queue <- e:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return ErrPublisherBufferFull
	}
}

// FailedPublishes returns the count of async publish failures since
// process start. Counter is monotone and atomic; callers may sample
// without external synchronisation.
func (p *JetStreamPublisher) FailedPublishes() int64 {
	return p.failedPublishes.Load()
}

// run is the async worker. Drains p.queue until stopCh fires, then
// drains whatever's left in the queue (non-blocking) and exits.
// ctx is the publisher-owned cancelable context — Stop cancels it
// when its own context expires so in-flight publishes can unstick.
func (p *JetStreamPublisher) run(ctx context.Context) {
	defer p.wg.Done()
	for {
		select {
		case e := <-p.queue:
			p.publishAsyncFromWorker(ctx, e)
		case <-p.stopCh:
			// Drain whatever is buffered before exiting.
			for {
				select {
				case e := <-p.queue:
					p.publishAsyncFromWorker(ctx, e)
				default:
					return
				}
			}
		}
	}
}

// publishAsyncFromWorker handles one event off the queue, surfacing
// failures via the counter + callback / log. The worker NEVER
// propagates errors via return — its job is to drain. Caller
// observability is the counter (always) + callback (if set) + log
// (when callback is nil).
func (p *JetStreamPublisher) publishAsyncFromWorker(ctx context.Context, e Event) {
	if err := p.publishOne(ctx, e); err != nil {
		p.failedPublishes.Add(1)
		if p.cfg.asyncOnError != nil {
			p.cfg.asyncOnError(e, err)
			return
		}
		p.cfg.logger.LogAttrs(ctx, slog.LevelWarn, "events: async publish failed",
			slog.String("event_id", e.ID),
			slog.String("event_type", string(e.Type)),
			slog.String("subject", e.Subject),
			slog.Any("error", err),
		)
	}
}

// publishOne is the shared sync publish path: validate (idempotent for
// already-validated events), stamp Subject if empty, persist, publish.
// Used by both Publish (caller-facing) and publishAsyncFromWorker
// (which has already validated + stamped).
func (p *JetStreamPublisher) publishOne(ctx context.Context, e Event) error {
	if err := e.Validate(); err != nil {
		return err
	}
	if e.Subject == "" {
		if _, err := e.StampSubject(p.clusterName); err != nil {
			return err
		}
	}
	if p.cfg.store != nil {
		if err := p.cfg.store.Store(ctx, e); err != nil {
			return fmt.Errorf("events: store: %w", err)
		}
	}
	payload, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("events: marshal: %w", err)
	}
	if _, err := p.js.PublishMsg(&nats.Msg{
		Subject: e.Subject,
		Data:    payload,
	}, nats.Context(ctx)); err != nil {
		return fmt.Errorf("events: jetstream publish: %w", err)
	}
	return nil
}

// Compile-time interface compliance.
var _ EventPublisher = (*JetStreamPublisher)(nil)
