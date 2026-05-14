package secrets

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"

	"go.keystone-core.io/keystone-core/internal/state"
)

// DefaultLeasePollInterval is the cadence the scheduler ticks at when
// [LeaseManagerConfig.PollInterval] is zero. 30s balances "respond
// quickly when a short-TTL lease wants renewal" against "don't pound
// Vault during quiet periods."
const DefaultLeasePollInterval = 30 * time.Second

// DefaultLeaseRenewTimeout is the per-renewal context timeout. 15s
// is generous for a healthy Vault round-trip; a slow Vault hits this
// + the next tick retries.
const DefaultLeaseRenewTimeout = 15 * time.Second

// DefaultLeaseJitter randomises each tick by ±10% to prevent
// thundering-herd renewals when many leases were issued at the same
// instant (PROJECT-DETAILS §4.11 risk).
const DefaultLeaseJitter = 0.1

// LifecycleEventType enumerates the events the scheduler fires on
// every tracked lease state transition.
type LifecycleEventType string

const (
	// LifecycleEventRenewed fires after a successful renewal — the
	// store row's ExpiresAt + Duration + RenewCount + LastRenewedAt
	// are already updated when the callback fires.
	LifecycleEventRenewed LifecycleEventType = "renewed"

	// LifecycleEventRenewalFailed fires when the renewer returns a
	// non-terminal error (transient — the next tick retries). For
	// permanent failures (lease expired, not renewable) the
	// corresponding terminal event fires instead.
	LifecycleEventRenewalFailed LifecycleEventType = "renewal_failed"

	// LifecycleEventExpired fires when Vault reports the lease as
	// expired (or the local TTL check catches it before renewal).
	// The store row's state is "expired" when the callback fires;
	// subsequent ticks ignore the lease.
	LifecycleEventExpired LifecycleEventType = "expired"

	// LifecycleEventNotRenewable fires when Vault reports the lease
	// is not renewable. The store row's Renewable is false when the
	// callback fires; subsequent ticks ignore the lease.
	LifecycleEventNotRenewable LifecycleEventType = "not_renewable"
)

// LifecycleEvent is what subscribed callbacks receive. At is the
// scheduler's tick clock; Info is the post-event view (newly stamped
// for Renewed; pre-event for the terminal events). Err is set for
// RenewalFailed / Expired / NotRenewable.
type LifecycleEvent struct {
	Type    LifecycleEventType
	LeaseID string
	Backend string
	Path    string
	Info    *LeaseInfo
	Err     error
	At      time.Time
}

// LifecycleCallback is the subscriber shape. Callbacks MUST NOT block
// — the scheduler fires them inline. Operators that need heavy work
// (Slack alerts, PagerDuty pages) hand the event off to a goroutine
// + bounded channel.
type LifecycleCallback func(ctx context.Context, evt LifecycleEvent)

// LeaseManagerConfig drives [NewLeaseManager].
type LeaseManagerConfig struct {
	// Store is the persistent backing. Required.
	Store state.LeaseStore

	// DefaultStrategy is applied to records that didn't supply one at
	// Record time. Zero → [RenewStrategyLazy] (90% TTL — fewer Vault
	// renewals; matches Vault's batch-token default semantic).
	DefaultStrategy RenewStrategy

	// PollInterval is the scheduler tick rate. Zero →
	// [DefaultLeasePollInterval].
	PollInterval time.Duration

	// Jitter randomises each tick by ±this fraction. Zero →
	// [DefaultLeaseJitter]. Range [0, 0.5].
	Jitter float64

	// RenewTimeout is the per-renewal ctx timeout. Zero →
	// [DefaultLeaseRenewTimeout].
	RenewTimeout time.Duration

	// Clock injects testable now-time. Zero → time.Now().UTC().
	Clock func() time.Time

	// Logger drives the scheduler's structured log lines. Zero →
	// slog.Default.
	Logger *slog.Logger
}

// LeaseManager wraps a persistent [state.LeaseStore] with a renewal
// scheduler + lifecycle callbacks. Satisfies [LeaseDirectory] so the
// broker plugs it in as a drop-in replacement for
// [InMemoryLeaseDirectory].
//
// Concurrency: a single sync.Mutex protects the lifecycle bools +
// the callback slice; the store is responsible for its own
// thread-safety (SQLite/Postgres impls in `internal/state` are).
type LeaseManager struct {
	cfg     LeaseManagerConfig
	renewer func(ctx context.Context, req RenewLeaseRequest) (*LeaseInfo, error)

	mu        sync.Mutex
	callbacks []LifecycleCallback
	started   bool
	stopped   bool
	loopCtx   context.Context
	cancel    context.CancelFunc
	doneCh    chan struct{}
	rng       *rand.Rand
}

// NewLeaseManager validates the config + returns the manager. The
// manager isn't live until [LeaseManager.Start] runs; the broker
// gets registered + the renewer wired between construction and Start.
func NewLeaseManager(cfg LeaseManagerConfig) (*LeaseManager, error) {
	if cfg.Store == nil {
		return nil, fmt.Errorf("%w: LeaseManager: Store is required", ErrInvalidBackend)
	}
	if cfg.DefaultStrategy == RenewStrategyUnknown {
		cfg.DefaultStrategy = RenewStrategyLazy
	}
	if cfg.PollInterval == 0 {
		cfg.PollInterval = DefaultLeasePollInterval
	}
	if cfg.PollInterval < 0 {
		return nil, fmt.Errorf("%w: LeaseManager: PollInterval must be positive", ErrInvalidBackend)
	}
	if cfg.Jitter == 0 {
		cfg.Jitter = DefaultLeaseJitter
	}
	if cfg.Jitter < 0 || cfg.Jitter > 0.5 {
		return nil, fmt.Errorf("%w: LeaseManager: Jitter must be in [0, 0.5]", ErrInvalidBackend)
	}
	if cfg.RenewTimeout == 0 {
		cfg.RenewTimeout = DefaultLeaseRenewTimeout
	}
	if cfg.Clock == nil {
		cfg.Clock = func() time.Time { return time.Now().UTC() }
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &LeaseManager{
		cfg: cfg,
		rng: rand.New(rand.NewPCG(uint64(cfg.Clock().UnixNano()), 0xc0deba5e)), // #nosec G404 -- jitter is anti-thundering-herd, not cryptographic
	}, nil
}

// SetRenewer wires the function the scheduler calls to renew leases.
// In production this is `broker.RenewLease` (or
// `broker.RevokeLease`, but the manager doesn't call revoke from the
// scheduler — terminal failures mark the row as expired/not-renewable
// without an explicit Vault revoke).
//
// Wiring order is: NewLeaseManager → NewBroker(BrokerConfig{
// LeaseDirectory: lm, ...}) → lm.SetRenewer(broker.RenewLease) →
// lm.Start(ctx). The setter exists to break the
// broker-references-manager-references-broker cycle.
//
// Safe to call before or after [LeaseManager.Start]; the scheduler
// reads the renewer per-tick.
func (lm *LeaseManager) SetRenewer(fn func(ctx context.Context, req RenewLeaseRequest) (*LeaseInfo, error)) {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	lm.renewer = fn
}

// OnLifecycle appends a callback that fires for every lifecycle
// event. Callbacks fire in registration order, inline with the
// scheduler tick. Subscribers that do heavy work MUST dispatch to
// their own goroutine.
func (lm *LeaseManager) OnLifecycle(cb LifecycleCallback) {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	lm.callbacks = append(lm.callbacks, cb)
}

// Start spawns the scheduler goroutine, deriving a cancelable
// context from ctx. One-shot — second call rejects.
func (lm *LeaseManager) Start(ctx context.Context) error {
	lm.mu.Lock()
	if lm.stopped {
		lm.mu.Unlock()
		return fmt.Errorf("%w: LeaseManager: cannot Start after Stop", ErrInvalidBackend)
	}
	if lm.started {
		lm.mu.Unlock()
		return fmt.Errorf("%w: LeaseManager: already started", ErrInvalidBackend)
	}
	lm.started = true
	loopCtx, cancel := context.WithCancel(ctx)
	lm.loopCtx = loopCtx
	lm.cancel = cancel
	lm.doneCh = make(chan struct{})
	lm.mu.Unlock()

	go lm.run(loopCtx)
	return nil
}

// Stop cancels the scheduler. Idempotent.
func (lm *LeaseManager) Stop(ctx context.Context) error {
	lm.mu.Lock()
	if lm.stopped || !lm.started {
		lm.stopped = true
		lm.mu.Unlock()
		return nil
	}
	lm.stopped = true
	cancel := lm.cancel
	doneCh := lm.doneCh
	lm.mu.Unlock()

	cancel()
	select {
	case <-doneCh:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("%w: LeaseManager: stop deadline: %v", ErrInvalidBackend, ctx.Err())
	}
}

// ---- LeaseDirectory interface ------------------------------------

// Record persists a new lease. The Strategy field on the record (if
// non-zero) overrides the manager's [LeaseManagerConfig.DefaultStrategy];
// zero falls through to the default.
//
// Record matches the [LeaseDirectory] interface's signature (no
// return value), so persistence errors are logged at WARN. Callers
// that need persist-or-fail semantics use [LeaseManager.RecordWithError].
func (lm *LeaseManager) Record(leaseID string, record LeaseRecord) {
	if err := lm.RecordWithError(context.Background(), leaseID, record); err != nil {
		lm.cfg.Logger.LogAttrs(context.Background(), slog.LevelWarn,
			"lease_manager: failed to persist lease record",
			slog.String("lease_id", leaseID),
			slog.String("backend", record.Backend),
			slog.String("path", record.Path),
			slog.String("err", err.Error()),
		)
	}
}

// RecordWithError is the error-returning variant. The broker fires
// [LeaseDirectory.Record] from inside `IssueDynamicSecret` and
// can't easily propagate an error to its caller, so the no-error
// shape stays the interface; tests + CLI callers use this method
// when they want to surface a persist failure.
func (lm *LeaseManager) RecordWithError(ctx context.Context, leaseID string, record LeaseRecord) error {
	if leaseID == "" {
		return fmt.Errorf("%w: LeaseManager.Record: leaseID is required", ErrInvalidBackend)
	}
	strategy := record.Strategy
	if strategy == RenewStrategyUnknown {
		strategy = lm.cfg.DefaultStrategy
	}
	now := lm.cfg.Clock()
	existing, err := lm.cfg.Store.GetLease(ctx, leaseID)
	if err != nil && !errors.Is(err, state.ErrNotFound) {
		return fmt.Errorf("%w: LeaseManager.Record: lookup %q: %v", ErrInvalidBackend, leaseID, err)
	}

	if existing != nil {
		// Update the routing fields without losing renewal history.
		existing.Backend = record.Backend
		existing.SecretPath = record.Path
		existing.Strategy = strategy.String()
		if err := lm.cfg.Store.UpdateLease(ctx, existing); err != nil {
			return fmt.Errorf("%w: LeaseManager.Record: update %q: %v", ErrInvalidBackend, leaseID, err)
		}
		return nil
	}

	rec := &state.LeaseStoreRecord{
		ID:         leaseID,
		Backend:    record.Backend,
		SecretPath: record.Path,
		IssuedAt:   now,
		ExpiresAt:  now, // unknown TTL at Record time — scheduler / first renewal stamps the real value
		State:      LeaseStateActive.String(),
		Strategy:   strategy.String(),
	}
	if err := lm.cfg.Store.CreateLease(ctx, rec); err != nil {
		return fmt.Errorf("%w: LeaseManager.Record: create %q: %v", ErrInvalidBackend, leaseID, err)
	}
	return nil
}

// Lookup returns the routing projection for leaseID. Misses return
// (zero, false). Backed by a single GetLease call; revoked entries
// surface as misses to match the in-memory directory's behaviour.
func (lm *LeaseManager) Lookup(leaseID string) (LeaseRecord, bool) {
	rec, err := lm.cfg.Store.GetLease(context.Background(), leaseID)
	if err != nil || rec == nil {
		return LeaseRecord{}, false
	}
	if !rec.RevokedAt.IsZero() {
		return LeaseRecord{}, false
	}
	strategy, _ := ParseRenewStrategy(rec.Strategy)
	return LeaseRecord{
		Backend:  rec.Backend,
		Path:     rec.SecretPath,
		Strategy: strategy,
	}, true
}

// Forget removes the lease from the store. Idempotent.
func (lm *LeaseManager) Forget(leaseID string) {
	if err := lm.cfg.Store.DeleteLease(context.Background(), leaseID); err != nil && !errors.Is(err, state.ErrNotFound) {
		lm.cfg.Logger.LogAttrs(context.Background(), slog.LevelWarn,
			"lease_manager: failed to delete lease",
			slog.String("lease_id", leaseID),
			slog.String("err", err.Error()),
		)
	}
}

// ---- Operator-facing CLI surface ---------------------------------

// List returns leases matching filter, projected to the public
// [Lease] shape (which embeds [LeaseInfo]).
func (lm *LeaseManager) List(ctx context.Context, filter state.LeaseFilter) ([]Lease, error) {
	recs, err := lm.cfg.Store.ListLeases(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("%w: LeaseManager.List: %v", ErrInvalidBackend, err)
	}
	out := make([]Lease, 0, len(recs))
	for _, rec := range recs {
		out = append(out, leaseFromRecord(rec))
	}
	return out, nil
}

// MarkRevoked flips a lease to the revoked state + stamps RevokedAt
// in the store. This is NOT the same as the broker's
// [Broker.RevokeLease] (which performs the actual Vault revoke +
// cache invalidation + audit). It's a store-only operation for bulk
// operator cleanup ("backend X is being decommissioned; mark all its
// leases revoked").
func (lm *LeaseManager) MarkRevoked(ctx context.Context, leaseID string) error {
	rec, err := lm.cfg.Store.GetLease(ctx, leaseID)
	if err != nil {
		return err
	}
	rec.State = LeaseStateRevoked.String()
	rec.RevokedAt = lm.cfg.Clock()
	if err := lm.cfg.Store.UpdateLease(ctx, rec); err != nil {
		return fmt.Errorf("%w: LeaseManager.MarkRevoked: %v", ErrInvalidBackend, err)
	}
	return nil
}

// ExpireCleanup removes lease rows whose ExpiresAt is at or before
// `before` AND whose state is no longer active. Wraps the underlying
// [state.LeaseStore.DeleteExpiredLeases].
func (lm *LeaseManager) ExpireCleanup(ctx context.Context, before time.Time) (int, error) {
	n, err := lm.cfg.Store.DeleteExpiredLeases(ctx, before)
	if err != nil {
		return 0, fmt.Errorf("%w: LeaseManager.ExpireCleanup: %v", ErrInvalidBackend, err)
	}
	return n, nil
}

// SetStrategy overrides the renewal strategy on an existing lease.
// Operators use this via the `kscore-secrets leases set-strategy`
// CLI (task 10).
func (lm *LeaseManager) SetStrategy(ctx context.Context, leaseID string, strategy RenewStrategy) error {
	if strategy == RenewStrategyUnknown {
		return fmt.Errorf("%w: LeaseManager.SetStrategy: strategy is required", ErrInvalidBackend)
	}
	rec, err := lm.cfg.Store.GetLease(ctx, leaseID)
	if err != nil {
		return err
	}
	rec.Strategy = strategy.String()
	if err := lm.cfg.Store.UpdateLease(ctx, rec); err != nil {
		return fmt.Errorf("%w: LeaseManager.SetStrategy: %v", ErrInvalidBackend, err)
	}
	return nil
}

// ---- Scheduler ---------------------------------------------------

// run is the scheduler loop. Each tick lists active+renewable leases,
// evaluates ShouldRenew for each, and dispatches renewals serially
// through the configured renewer. Lifecycle callbacks fire inline.
func (lm *LeaseManager) run(ctx context.Context) {
	defer close(lm.doneCh)
	for {
		wait := lm.nextWait()
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		lm.tick(ctx)
	}
}

// nextWait returns the PollInterval +/- the Jitter fraction.
func (lm *LeaseManager) nextWait() time.Duration {
	if lm.cfg.Jitter == 0 {
		return lm.cfg.PollInterval
	}
	lm.mu.Lock()
	delta := (lm.rng.Float64()*2 - 1) * lm.cfg.Jitter
	lm.mu.Unlock()
	out := float64(lm.cfg.PollInterval) * (1 + delta)
	if out < 1 {
		out = 1
	}
	return time.Duration(out)
}

// tick runs one scheduler pass: list active+renewable leases, evaluate
// each against its strategy, dispatch renewals as needed.
func (lm *LeaseManager) tick(ctx context.Context) {
	recs, err := lm.cfg.Store.ListLeases(ctx, state.LeaseFilter{
		State: LeaseStateActive.String(),
	})
	if err != nil {
		lm.cfg.Logger.LogAttrs(ctx, slog.LevelWarn,
			"lease_manager: list active leases failed",
			slog.String("err", err.Error()),
		)
		return
	}
	now := lm.cfg.Clock()
	for _, rec := range recs {
		lm.evaluate(ctx, rec, now)
	}
}

// evaluate handles one lease per tick: terminal-expire if already
// past expiry; otherwise renew if ShouldRenew says so.
func (lm *LeaseManager) evaluate(ctx context.Context, rec *state.LeaseStoreRecord, now time.Time) {
	info := leaseInfoFromRecord(rec)
	strategy, _ := ParseRenewStrategy(rec.Strategy)

	// Local terminal-expire: if the lease is past expiry already,
	// don't bother calling Vault — mark it expired and fire the
	// callback. (Defensive; the scheduler also catches this on Vault's
	// renewal response.)
	if info.Expired(now) {
		lm.markExpired(ctx, rec, errors.New("local expiry check: lease has expired"))
		return
	}

	if !info.ShouldRenew(now, strategy) {
		return
	}

	lm.renew(ctx, rec, info)
}

// renew dispatches a single renewal. Errors map to the appropriate
// terminal lifecycle event when permanent ([ErrLeaseExpired] /
// [ErrLeaseNotRenewable]); other errors are transient and the next
// tick retries.
func (lm *LeaseManager) renew(ctx context.Context, rec *state.LeaseStoreRecord, info LeaseInfo) {
	lm.mu.Lock()
	renewer := lm.renewer
	lm.mu.Unlock()

	if renewer == nil {
		lm.cfg.Logger.LogAttrs(ctx, slog.LevelWarn,
			"lease_manager: no renewer configured; skipping",
			slog.String("lease_id", rec.ID),
		)
		return
	}

	renewCtx, cancel := context.WithTimeout(ctx, lm.cfg.RenewTimeout)
	defer cancel()

	updated, err := renewer(renewCtx, RenewLeaseRequest{LeaseID: rec.ID})
	if err != nil {
		switch {
		case errors.Is(err, ErrLeaseExpired):
			lm.markExpired(ctx, rec, err)
		case errors.Is(err, ErrLeaseNotRenewable):
			lm.markNotRenewable(ctx, rec, err)
		default:
			lm.cfg.Logger.LogAttrs(ctx, slog.LevelWarn,
				"lease_manager: renewal failed (transient)",
				slog.String("lease_id", rec.ID),
				slog.String("err", err.Error()),
			)
			lm.fireCallback(ctx, LifecycleEvent{
				Type: LifecycleEventRenewalFailed, LeaseID: rec.ID,
				Backend: rec.Backend, Path: rec.SecretPath,
				Info: &info, Err: err, At: lm.cfg.Clock(),
			})
		}
		return
	}

	// Successful renewal — stamp the row.
	now := lm.cfg.Clock()
	rec.LastRenewedAt = now
	rec.RenewCount++
	if updated != nil {
		if updated.Duration > 0 {
			rec.Duration = updated.Duration
		}
		if !updated.ExpiresAt.IsZero() {
			rec.ExpiresAt = updated.ExpiresAt
		} else if rec.Duration > 0 {
			rec.ExpiresAt = now.Add(rec.Duration)
		}
		rec.Renewable = updated.Renewable
	}
	if err := lm.cfg.Store.UpdateLease(ctx, rec); err != nil {
		lm.cfg.Logger.LogAttrs(ctx, slog.LevelWarn,
			"lease_manager: failed to persist renewal",
			slog.String("lease_id", rec.ID),
			slog.String("err", err.Error()),
		)
		return
	}
	newInfo := leaseInfoFromRecord(rec)
	lm.fireCallback(ctx, LifecycleEvent{
		Type: LifecycleEventRenewed, LeaseID: rec.ID,
		Backend: rec.Backend, Path: rec.SecretPath,
		Info: &newInfo, At: now,
	})
}

func (lm *LeaseManager) markExpired(ctx context.Context, rec *state.LeaseStoreRecord, cause error) {
	rec.State = LeaseStateExpired.String()
	if err := lm.cfg.Store.UpdateLease(ctx, rec); err != nil {
		lm.cfg.Logger.LogAttrs(ctx, slog.LevelWarn,
			"lease_manager: failed to mark expired",
			slog.String("lease_id", rec.ID),
			slog.String("err", err.Error()),
		)
		return
	}
	info := leaseInfoFromRecord(rec)
	lm.fireCallback(ctx, LifecycleEvent{
		Type: LifecycleEventExpired, LeaseID: rec.ID,
		Backend: rec.Backend, Path: rec.SecretPath,
		Info: &info, Err: cause, At: lm.cfg.Clock(),
	})
}

func (lm *LeaseManager) markNotRenewable(ctx context.Context, rec *state.LeaseStoreRecord, cause error) {
	rec.Renewable = false
	if err := lm.cfg.Store.UpdateLease(ctx, rec); err != nil {
		lm.cfg.Logger.LogAttrs(ctx, slog.LevelWarn,
			"lease_manager: failed to mark not-renewable",
			slog.String("lease_id", rec.ID),
			slog.String("err", err.Error()),
		)
		return
	}
	info := leaseInfoFromRecord(rec)
	lm.fireCallback(ctx, LifecycleEvent{
		Type: LifecycleEventNotRenewable, LeaseID: rec.ID,
		Backend: rec.Backend, Path: rec.SecretPath,
		Info: &info, Err: cause, At: lm.cfg.Clock(),
	})
}

func (lm *LeaseManager) fireCallback(ctx context.Context, evt LifecycleEvent) {
	lm.mu.Lock()
	cbs := make([]LifecycleCallback, len(lm.callbacks))
	copy(cbs, lm.callbacks)
	lm.mu.Unlock()
	for _, cb := range cbs {
		cb(ctx, evt)
	}
}

// ---- Record <-> public-type projections --------------------------

// leaseInfoFromRecord projects the persisted shape into the in-memory
// public [LeaseInfo].
func leaseInfoFromRecord(rec *state.LeaseStoreRecord) LeaseInfo {
	if rec == nil {
		return LeaseInfo{}
	}
	state, _ := ParseLeaseState(rec.State)
	return LeaseInfo{
		ID:            rec.ID,
		SecretPath:    rec.SecretPath,
		Backend:       rec.Backend,
		IssuedAt:      rec.IssuedAt,
		ExpiresAt:     rec.ExpiresAt,
		Duration:      rec.Duration,
		Renewable:     rec.Renewable,
		MaxTTL:        rec.MaxTTL,
		State:         state,
		LastRenewedAt: rec.LastRenewedAt,
		RenewCount:    rec.RenewCount,
		Metadata:      copyStringMap(rec.Metadata),
	}
}

// leaseFromRecord adds the [Lease]-only fields (IssuedFor, RevokedAt
// pointer) on top of the [LeaseInfo] projection.
func leaseFromRecord(rec *state.LeaseStoreRecord) Lease {
	out := Lease{LeaseInfo: leaseInfoFromRecord(rec), IssuedFor: rec.IssuedFor}
	if !rec.RevokedAt.IsZero() {
		t := rec.RevokedAt
		out.RevokedAt = &t
	}
	return out
}

// Compile-time interface assertion — LeaseManager satisfies the
// [LeaseDirectory] interface broker uses.
var _ LeaseDirectory = (*LeaseManager)(nil)
