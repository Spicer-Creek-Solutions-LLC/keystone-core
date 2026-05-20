package outbound

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"

	"go.keystone-core.io/keystone-core/internal/events"
)

// DefaultMaxConcurrentDeliveries caps the in-flight goroutines the
// Manager will spawn when fanning out one event to many matching
// subscriptions. Mirrors the events RouteAsync 100-goroutine bound
// in spirit (back-pressure on slow receivers).
const DefaultMaxConcurrentDeliveries = 32

// DefaultMaxPayloadBytes bounds the JSON payload sent to a receiver.
// Matches the §4.14 default (1 MiB).
const DefaultMaxPayloadBytes int64 = 1 << 20

// Dispatcher delivers a serialized event payload to one subscription
// and reports HTTP status + error. Task 13 will provide the concrete
// implementation (HTTP POST + custom headers + HMAC + per-sub
// timeout); task 12 tests fake it.
type Dispatcher interface {
	Deliver(ctx context.Context, sub *Subscription, payload []byte, deliveryID string) (int, error)
}

// Manager subscribes to the Keystone event bus (push-driven via
// [Manager.Handle] — boot wires `events.EventSubscriber.Subscribe` to
// call it), glob-matches each event against every enabled
// subscription's filter list, and fans out async via a bounded
// goroutine pool + [sync.WaitGroup] so [Manager.Stop] can drain.
// Retries and circuit-breaking are layered on by tasks 14/15; a v1.0
// failure here ends as one `failed` [DeliveryRecord].
type Manager struct {
	Store      SubscriptionStore
	Dispatcher Dispatcher
	Logger     *slog.Logger

	// MaxConcurrentDeliveries caps the in-flight fan-out goroutines.
	// 0 → [DefaultMaxConcurrentDeliveries].
	MaxConcurrentDeliveries int
	// MaxPayloadBytes caps the serialized event payload. 0 →
	// [DefaultMaxPayloadBytes]. Over-size events get one synthetic
	// `failed` [DeliveryRecord] (per-event, not per-sub) so the
	// audit history reflects the drop.
	MaxPayloadBytes int64
	// RefreshInterval drives the background subscription-cache
	// reload. 0 disables the ticker; on-demand [Manager.Refresh]
	// remains callable (the REST handler calls it after CRUD —
	// task 16).
	RefreshInterval time.Duration

	// Retry holds the shared exp-backoff tuning (task 14). The
	// per-call attempt budget is per-subscription
	// ([Subscription.MaxRetries], default 3 per the §4.14 schema).
	Retry RetryPolicy

	// IDGen / Now / Jitterer are deterministic seams for tests.
	IDGen    func() string
	Now      func() time.Time
	Jitterer func() float64

	mu        sync.RWMutex
	subs      []*Subscription
	startOnce sync.Once
	stopOnce  sync.Once
	startErr  error
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	sem       chan struct{}
}

// Start initialises the concurrency primitives and loads the
// subscription cache. When [Manager.RefreshInterval] > 0 it also
// kicks off a background ticker that periodically reloads the cache.
// Idempotent — repeat calls return the first call's result.
func (m *Manager) Start(ctx context.Context) error {
	m.startOnce.Do(func() {
		if m.Store == nil {
			m.startErr = errors.New("outbound: manager: Store is required")
			return
		}
		if m.Dispatcher == nil {
			m.startErr = errors.New("outbound: manager: Dispatcher is required")
			return
		}
		semCap := m.MaxConcurrentDeliveries
		if semCap <= 0 {
			semCap = DefaultMaxConcurrentDeliveries
		}
		m.sem = make(chan struct{}, semCap)
		if m.Logger == nil {
			m.Logger = slog.Default()
		}
		if m.IDGen == nil {
			m.IDGen = uuid.NewString
		}
		if m.Now == nil {
			m.Now = time.Now
		}
		if m.Jitterer == nil {
			m.Jitterer = rand.Float64
		}
		if err := m.Refresh(ctx); err != nil {
			m.startErr = err
			return
		}
		if m.RefreshInterval > 0 {
			runCtx, cancel := context.WithCancel(context.Background())
			m.cancel = cancel
			go m.refreshLoop(runCtx)
		}
	})
	return m.startErr
}

func (m *Manager) refreshLoop(ctx context.Context) {
	t := time.NewTicker(m.RefreshInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := m.Refresh(ctx); err != nil {
				m.Logger.Warn("outbound manager refresh failed",
					slog.String("error", err.Error()))
			}
		}
	}
}

// Refresh reloads the in-memory enabled-subscription cache from the
// store. Callable on-demand by REST/CLI handlers (task 16) after
// subscription CRUD, and by the background ticker.
func (m *Manager) Refresh(ctx context.Context) error {
	all, err := m.Store.ListSubscriptions(ctx)
	if err != nil {
		return fmt.Errorf("outbound: refresh subscriptions: %w", err)
	}
	out := make([]*Subscription, 0, len(all))
	for _, s := range all {
		if s.Enabled {
			out = append(out, s)
		}
	}
	m.mu.Lock()
	m.subs = out
	m.mu.Unlock()
	return nil
}

// Stop drains in-flight deliveries up to ctx's deadline. Idempotent;
// safe to call without a prior successful Start.
func (m *Manager) Stop(ctx context.Context) error {
	var ferr error
	m.stopOnce.Do(func() {
		if m.cancel != nil {
			m.cancel()
		}
		done := make(chan struct{})
		go func() { m.wg.Wait(); close(done) }()
		select {
		case <-done:
		case <-ctx.Done():
			ferr = ctx.Err()
		}
	})
	return ferr
}

// Handle is the per-event entry point boot wires to the event-bus
// subscription. It is non-blocking from the caller's POV: matched
// subscriptions are dispatched in bounded goroutines tracked by the
// internal WaitGroup. Errors during persistence / dispatch are
// captured on the [DeliveryRecord] (status `failed`) — Handle does
// not return them.
func (m *Manager) Handle(ctx context.Context, ev events.Event) {
	payload, err := json.Marshal(ev)
	if err != nil {
		m.Logger.Warn("outbound manager marshal event",
			slog.String("type", ev.Type.String()),
			slog.String("error", err.Error()))
		return
	}
	limit := m.MaxPayloadBytes
	if limit <= 0 {
		limit = DefaultMaxPayloadBytes
	}
	if int64(len(payload)) > limit {
		m.recordOversize(ctx, ev, int64(len(payload)), limit)
		return
	}

	m.mu.RLock()
	subs := m.subs
	m.mu.RUnlock()
	typ := ev.Type.String()
	for _, sub := range subs {
		if !matches(typ, sub.Events) {
			continue
		}
		m.fanOut(ctx, sub, ev, payload)
	}
}

// matches reports whether typ matches any of the subscription's
// glob patterns via stdlib filepath.Match (§4.14: no precompile in
// v1.0; `*`/`?`/`[]` work because filepath.Match's special separator
// is `/`, not `.`). Empty patterns = match nothing (opt-in).
func matches(typ string, patterns []string) bool {
	for _, p := range patterns {
		ok, err := filepath.Match(p, typ)
		if err == nil && ok {
			return true
		}
	}
	return false
}

func (m *Manager) recordOversize(ctx context.Context, ev events.Event, size, limit int64) {
	d := &DeliveryRecord{
		ID:          m.IDGen(),
		EventType:   ev.Type.String(),
		EventID:     ev.ID,
		Status:      DeliveryFailed,
		Error:       fmt.Sprintf("payload %d bytes > limit %d", size, limit),
		DeliveredAt: m.Now().UTC(),
	}
	if err := m.Store.SaveDelivery(ctx, d); err != nil {
		m.Logger.Warn("outbound manager save oversize record",
			slog.String("error", err.Error()))
	}
}

// fanOut spawns the bounded delivery goroutine for one matched
// subscription. Pre-emptively records a Pending [DeliveryRecord] so
// even an immediate dispatcher panic / crash leaves an audit trail.
func (m *Manager) fanOut(ctx context.Context, sub *Subscription, ev events.Event, payload []byte) {
	d := &DeliveryRecord{
		ID:             m.IDGen(),
		SubscriptionID: sub.ID,
		EventType:      ev.Type.String(),
		EventID:        ev.ID,
		Status:         DeliveryPending,
		Attempt:        1,
		DeliveredAt:    m.Now().UTC(),
	}
	if err := m.Store.SaveDelivery(ctx, d); err != nil {
		m.Logger.Warn("outbound manager save pending delivery",
			slog.String("subscription", sub.ID),
			slog.String("error", err.Error()))
		return
	}

	m.wg.Add(1)
	m.sem <- struct{}{}
	go func(sub *Subscription, d *DeliveryRecord, payload []byte) {
		defer m.wg.Done()
		defer func() { <-m.sem }()
		m.deliverOnce(ctx, sub, d, payload)
	}(sub, d, payload)
}

func (m *Manager) deliverOnce(ctx context.Context, sub *Subscription, d *DeliveryRecord, payload []byte) {
	maxAttempts := 1 + sub.MaxRetries
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		d.Attempt = attempt
		d.DeliveredAt = m.Now().UTC()
		code, err := m.Dispatcher.Deliver(ctx, sub, payload, d.ID)
		d.StatusCode = code
		if err == nil {
			d.Status = DeliverySuccess
			d.Error = ""
			m.saveFinal(ctx, sub, d)
			return
		}
		d.Error = err.Error()
		if attempt >= maxAttempts {
			d.Status = DeliveryFailed
			m.saveFinal(ctx, sub, d)
			return
		}
		// Persist the intermediate `retrying` state (§4.14) then
		// sleep with exp-backoff + jitter. A ctx cancel mid-backoff
		// is a terminal failure (record retained per §4.14).
		d.Status = DeliveryRetrying
		m.saveFinal(ctx, sub, d)
		if serr := ctxSleep(ctx, jitteredBackoff(m.Retry, attempt-1, m.Jitterer)); serr != nil {
			d.Status = DeliveryFailed
			d.Error = serr.Error()
			m.saveFinal(ctx, sub, d)
			return
		}
	}
}

// saveFinal persists the current state of d and warns on store
// errors — Manager never returns these (the audit record is
// best-effort once the dispatch decision is made).
func (m *Manager) saveFinal(ctx context.Context, sub *Subscription, d *DeliveryRecord) {
	if err := m.Store.SaveDelivery(ctx, d); err != nil {
		m.Logger.Warn("outbound manager save delivery",
			slog.String("subscription", sub.ID),
			slog.String("delivery", d.ID),
			slog.String("status", string(d.Status)),
			slog.String("error", err.Error()))
	}
}
