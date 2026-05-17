package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
)

// JetStreamSubscriber is the v1.0 implementation of [EventSubscriber].
// Push-based JetStream consumers per the project's existing pattern
// (Epic 05 command subscribers); manual ack with handler-error → Nak
// backoff; client-side filter predicates; replay-from-store with ID
// dedup against the live JetStream slice.
//
// Concurrent use: Subscribe is safe from multiple goroutines.
// Start / Stop MUST be serialised by the caller (typical boot wiring
// is single-threaded).
type JetStreamSubscriber struct {
	js          nats.JetStreamContext
	clusterName string
	cfg         subscriberConfig

	started atomic.Bool

	mu   sync.Mutex
	subs map[*subscription]struct{}
}

// NewJetStreamSubscriber builds a subscriber against the given
// JetStream context + cluster name. Cluster name is currently
// informational (logs); the subject patterns callers pass to
// Subscribe must already be fully qualified — the subscriber does
// not prefix or rewrite them.
//
// The returned subscriber is unstarted — call
// [JetStreamSubscriber.Start] before any Subscribe.
func NewJetStreamSubscriber(js nats.JetStreamContext, clusterName string, opts ...SubscriberOption) *JetStreamSubscriber {
	cfg := defaultSubscriberConfig()
	for _, opt := range opts {
		opt(&cfg)
	}
	return &JetStreamSubscriber{
		js:          js,
		clusterName: clusterName,
		cfg:         cfg,
		subs:        make(map[*subscription]struct{}),
	}
}

// Start marks the subscriber as ready for Subscribe calls. Idempotent
// across Stop / Start cycles; double-Start without an intervening
// Stop is rejected.
func (s *JetStreamSubscriber) Start(_ context.Context) error {
	if !s.started.CompareAndSwap(false, true) {
		return fmt.Errorf("events: subscriber already started")
	}
	return nil
}

// Stop unsubscribes every tracked subscription and waits for in-flight
// handlers to drain. Idempotent. Returns ctx.Err() if the caller's
// deadline expires before all handlers complete.
func (s *JetStreamSubscriber) Stop(ctx context.Context) error {
	if !s.started.CompareAndSwap(true, false) {
		return nil
	}
	s.mu.Lock()
	subs := make([]*subscription, 0, len(s.subs))
	for sub := range s.subs {
		subs = append(subs, sub)
	}
	s.subs = make(map[*subscription]struct{})
	s.mu.Unlock()

	for _, sub := range subs {
		if err := sub.unsubscribeUnlocked(); err != nil {
			s.cfg.logger.LogAttrs(ctx, slog.LevelWarn, "events: subscription unsubscribe on Stop",
				slog.Any("error", err),
			)
		}
	}

	// Wait for in-flight handlers across every subscription.
	done := make(chan struct{})
	go func() {
		for _, sub := range subs {
			sub.wg.Wait()
		}
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Subscribe creates a new subscription on pattern with the given
// handler. See [SubscribeOption] for the full set of knobs.
func (s *JetStreamSubscriber) Subscribe(ctx context.Context, pattern string, h EventHandler, opts ...SubscribeOption) (Subscription, error) {
	if !s.started.Load() {
		return nil, ErrSubscriberNotStarted
	}
	if pattern == "" {
		return nil, fmt.Errorf("events: Subscribe: pattern is required")
	}
	if h == nil {
		return nil, fmt.Errorf("events: Subscribe: handler is required")
	}
	cfg := defaultSubscribeConfig()
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.replay > 0 && s.cfg.store == nil {
		return nil, fmt.Errorf("events: Subscribe: WithReplay requires WithSubscriberStore at construction time")
	}

	sub := &subscription{
		parent:  s,
		handler: h,
		filter:  cfg.filter,
		cfg:     cfg,
	}
	sub.ctx, sub.cancel = context.WithCancel(context.Background())
	if cfg.replay > 0 {
		sub.dedup = newIDDedup(s.cfg.dedupSize)
	}

	// Replay phase first (if requested) so the dedup set is
	// populated before any live message is dispatched.
	handoffTime := time.Now().UTC()
	if cfg.replay > 0 {
		if err := s.replayFromStore(ctx, sub, handoffTime); err != nil {
			sub.cancel()
			return nil, err
		}
	}

	// Live subscription.
	natsSub, err := s.attachLive(pattern, sub, cfg, handoffTime)
	if err != nil {
		sub.cancel()
		return nil, err
	}
	sub.natsSub = natsSub

	s.mu.Lock()
	s.subs[sub] = struct{}{}
	s.mu.Unlock()
	return sub, nil
}

// replayFromStore pages through the store for events in
// `[handoffTime - replay, handoffTime)` and dispatches them through
// the handler, populating the dedup set as it goes. Errors from the
// handler during replay are logged and the dispatch continues — the
// live phase will redeliver if JetStream still has the message;
// otherwise the failed event lives in the store for the next
// replay attempt.
func (s *JetStreamSubscriber) replayFromStore(ctx context.Context, sub *subscription, handoffTime time.Time) error {
	q := EventQuery{
		Since: handoffTime.Add(-sub.cfg.replay),
		Until: handoffTime,
		Limit: 200,
	}
	for {
		page, err := s.cfg.store.Query(ctx, q)
		if err != nil {
			return fmt.Errorf("events: replay query: %w", err)
		}
		for _, e := range page.Events {
			if sub.dedup.SeenAndAdd(e.ID) {
				continue
			}
			if sub.filter != nil && !sub.filter(e) {
				continue
			}
			sub.wg.Add(1)
			func() {
				defer sub.wg.Done()
				if err := sub.handler(sub.ctx, e); err != nil {
					s.cfg.logger.LogAttrs(ctx, slog.LevelWarn,
						"events: replay handler failed",
						slog.String("event_id", e.ID),
						slog.String("subject", e.Subject),
						slog.Any("error", err),
					)
				}
			}()
		}
		if page.NextCursor == "" {
			break
		}
		q.Cursor = page.NextCursor
	}
	return nil
}

// attachLive opens the JetStream subscription. Returns the underlying
// *nats.Subscription so the [subscription] wrapper can Unsubscribe
// later. Subject pattern flows through verbatim; subscriber-side
// validation is the caller's job (use [SubjectFor] to build a
// canonical subject for a specific type, or `kscore.<cluster>.events.>`
// for broadcast).
func (s *JetStreamSubscriber) attachLive(pattern string, sub *subscription, cfg subscribeConfig, handoffTime time.Time) (*nats.Subscription, error) {
	opts := []nats.SubOpt{
		nats.AckExplicit(),
		nats.MaxDeliver(cfg.maxRedeliveries + 1),
		nats.AckWait(cfg.ackTimeout),
		nats.ManualAck(),
	}
	if cfg.durableName != "" {
		opts = append(opts, nats.Durable(cfg.durableName))
	} else if cfg.queueGroup != "" {
		opts = append(opts, nats.Durable("events_qg_"+cfg.queueGroup))
	}
	if cfg.replay > 0 {
		// Re-deliver from JetStream starting at the replay window's
		// lower bound. Events also covered by the store-replay phase
		// get filtered by the dedup set.
		opts = append(opts, nats.StartTime(handoffTime.Add(-cfg.replay)))
	} else {
		opts = append(opts, nats.DeliverNew())
	}

	if cfg.queueGroup != "" {
		return s.js.QueueSubscribe(pattern, cfg.queueGroup, sub.dispatch, opts...)
	}
	return s.js.Subscribe(pattern, sub.dispatch, opts...)
}

// removeSubscription is called by [subscription.Unsubscribe] so the
// parent's tracking map stays in sync with what's actually live.
func (s *JetStreamSubscriber) removeSubscription(sub *subscription) {
	s.mu.Lock()
	delete(s.subs, sub)
	s.mu.Unlock()
}

// subscription is the concrete [Subscription] implementation tracking
// one JetStream subscription's state.
type subscription struct {
	parent  *JetStreamSubscriber
	natsSub *nats.Subscription
	handler EventHandler
	filter  func(Event) bool
	cfg     subscribeConfig
	dedup   *idDedup // nil when replay was not requested
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup

	closed atomic.Bool
}

// Unsubscribe stops delivery and removes this subscription from the
// parent's tracking. Safe to call multiple times; second + later
// calls are no-ops.
func (s *subscription) Unsubscribe() error {
	if !s.closed.CompareAndSwap(false, true) {
		return nil
	}
	err := s.unsubscribeUnlocked()
	s.parent.removeSubscription(s)
	return err
}

// unsubscribeUnlocked is the inner Unsubscribe used by Stop, which
// already pops the sub from the parent map and so should NOT call
// removeSubscription. Avoids re-entering the parent's mutex.
func (s *subscription) unsubscribeUnlocked() error {
	s.cancel()
	if s.natsSub == nil {
		return nil
	}
	if err := s.natsSub.Unsubscribe(); err != nil && !errors.Is(err, nats.ErrConnectionClosed) {
		return err
	}
	return nil
}

// Pending returns the number of messages buffered locally by nats.go
// awaiting handler dispatch.
func (s *subscription) Pending() (uint64, error) {
	if s.natsSub == nil {
		return 0, fmt.Errorf("events: subscription not started")
	}
	msgs, _, err := s.natsSub.Pending()
	if err != nil {
		return 0, err
	}
	if msgs < 0 {
		return 0, nil
	}
	return uint64(msgs), nil //nolint:gosec // msgs >= 0 from the branch above
}

// dispatch is the nats.go-side callback for one delivered message.
// Unmarshals → optional dedup → optional filter → handler → Ack or
// Nak-with-backoff.
func (s *subscription) dispatch(msg *nats.Msg) {
	s.wg.Add(1)
	defer s.wg.Done()

	var e Event
	if err := json.Unmarshal(msg.Data, &e); err != nil {
		s.parent.cfg.logger.LogAttrs(s.ctx, slog.LevelWarn,
			"events: unmarshal failed; acking poison message",
			slog.String("subject", msg.Subject),
			slog.Any("error", err),
		)
		_ = msg.Ack()
		return
	}

	if s.dedup != nil && s.dedup.SeenAndAdd(e.ID) {
		_ = msg.Ack()
		return
	}
	if s.filter != nil && !s.filter(e) {
		_ = msg.Ack()
		return
	}
	if err := s.handler(s.ctx, e); err != nil {
		_ = msg.NakWithDelay(backoffForDelivery(msg))
		return
	}
	_ = msg.Ack()
}

// backoffForDelivery returns the §4.9 backoff schedule keyed on the
// JetStream delivery counter (1-based: the initial delivery is 1).
// Schedule: 1s → 5s → 15s. Beyond the third redelivery, JetStream's
// MaxDeliver caps the redelivery loop entirely; the message becomes
// undeliverable and (for post-v1.0) lands in the DLQ.
func backoffForDelivery(msg *nats.Msg) time.Duration {
	meta, err := msg.Metadata()
	if err != nil || meta == nil {
		return time.Second
	}
	switch meta.NumDelivered {
	case 1:
		return time.Second
	case 2:
		return 5 * time.Second
	default:
		return 15 * time.Second
	}
}

// idDedup is a bounded set of recently-seen Event.IDs. SeenAndAdd is
// the only public method: it returns true when id was already in the
// set (caller should skip) and inserts id otherwise.
//
// Implementation is a map + ring buffer: O(1) insert/lookup with
// FIFO eviction. We don't promote on read because the replay use
// case writes once per ID and reads it back exactly once.
type idDedup struct {
	mu   sync.Mutex
	set  map[string]struct{}
	ring []string
	next int
}

func newIDDedup(capacity int) *idDedup {
	if capacity <= 0 {
		capacity = DefaultDedupSize
	}
	return &idDedup{
		set:  make(map[string]struct{}, capacity),
		ring: make([]string, capacity),
	}
}

// SeenAndAdd returns true when id was already tracked. False (and
// records id) otherwise. Concurrent-safe.
func (d *idDedup) SeenAndAdd(id string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.set[id]; ok {
		return true
	}
	// Evict the slot we're about to overwrite (if any) so the map
	// stays in sync with the ring.
	if old := d.ring[d.next]; old != "" {
		delete(d.set, old)
	}
	d.ring[d.next] = id
	d.set[id] = struct{}{}
	d.next = (d.next + 1) % len(d.ring)
	return false
}

// Compile-time interface compliance.
var (
	_ EventSubscriber = (*JetStreamSubscriber)(nil)
	_ Subscription    = (*subscription)(nil)
)
