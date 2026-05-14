package secrets

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// BrokerConfig drives [NewBroker]. The only required fields are
// [BrokerConfig.Router] (may be empty — every lookup then falls
// through to DefaultBackend) and [BrokerConfig.Backends] (at least
// one backend is required so something is reachable).
//
// DefaultBackend is the fallback when the router returns no match. It
// MUST name one of the backends in Backends if set; an empty
// DefaultBackend with an empty router fails every dispatch, which is
// useful only in tests.
//
// Cache, Auditor, and LeaseDirectory default to the package no-ops.
// ExtractPrincipal defaults to a function that returns the zero
// [Principal] — task 9's gRPC service wires the real
// `pkg/api/auth.PrincipalFromContext` extractor.
//
// Clock is injectable for deterministic latency assertions in tests;
// nil uses `time.Now().UTC()`.
//
// Logger drives the broker's own startup / lifecycle log lines (NOT
// the audit trail — that goes through Auditor). nil → `slog.Default`.
type BrokerConfig struct {
	Router           *Router
	Backends         []SecretBackend
	DefaultBackend   string
	Cache            Cache
	Auditor          Auditor
	LeaseDirectory   LeaseDirectory
	ExtractPrincipal func(ctx context.Context) Principal
	Clock            func() time.Time
	Logger           *slog.Logger
}

// Broker is the single entry point higher layers (REST handlers,
// gRPC service, CLI) talk to per PROJECT-DETAILS §4.11. It joins:
//
//   - a [Router] (path-prefix → backend name) and a `default_backend`
//     fallback,
//   - a registry of [SecretBackend] instances keyed by `Name()`,
//   - a [Cache] consulted on reads and invalidated on writes /
//     deletes / revokes,
//   - an [Auditor] that records every operation (success or failure),
//   - a [LeaseDirectory] that maps `LeaseID → backend + path` so lease
//     ops route without the caller knowing which backend issued
//     them.
//
// Every operational method follows the same shape: extract principal
// from ctx → look up route / backend → capability-check → dispatch
// (with optional cache layer for `GetSecret`) → emit audit event.
// The audit event fires regardless of outcome so the `failure to log
// = bug` invariant from §4.11 stays intact.
//
// The broker owns its backends' lifecycles: [Broker.Start] fans out
// to every backend, [Broker.Stop] does the reverse, [Broker.Health]
// aggregates.
type Broker struct {
	router           *Router
	backends         map[string]SecretBackend
	defaultBackend   string
	cache            Cache
	auditor          Auditor
	leases           LeaseDirectory
	extractPrincipal func(ctx context.Context) Principal
	clock            func() time.Time
	logger           *slog.Logger

	mu      sync.Mutex
	started bool
	stopped bool
}

// NewBroker validates the config and returns the broker. Errors wrap
// [ErrInvalidBackend] so call sites match the family root with
// [errors.Is].
//
// Validation:
//   - At least one backend is required.
//   - Backend names MUST be unique — duplicates are rejected with all
//     offenders listed.
//   - DefaultBackend, if non-empty, MUST name a registered backend.
//   - Router MAY be nil; a nil router behaves like an empty router
//     (every lookup falls through to DefaultBackend).
//
// Cache, Auditor, LeaseDirectory, ExtractPrincipal, Clock, and Logger
// are defaulted to their no-op / standard-library equivalents.
func NewBroker(cfg BrokerConfig) (*Broker, error) {
	if len(cfg.Backends) == 0 {
		return nil, fmt.Errorf("%w: broker: at least one backend is required", ErrInvalidBackend)
	}

	backends := make(map[string]SecretBackend, len(cfg.Backends))
	duplicates := make(map[string][]int)
	for i, b := range cfg.Backends {
		if b == nil {
			return nil, fmt.Errorf("%w: broker: backends[%d] is nil", ErrInvalidBackend, i)
		}
		name := b.Name()
		if name == "" {
			return nil, fmt.Errorf("%w: broker: backends[%d] has empty Name()", ErrInvalidBackend, i)
		}
		if _, exists := backends[name]; exists {
			duplicates[name] = append(duplicates[name], i)
		}
		backends[name] = b
	}
	if len(duplicates) > 0 {
		names := make([]string, 0, len(duplicates))
		for n := range duplicates {
			names = append(names, fmt.Sprintf("%q", n))
		}
		return nil, fmt.Errorf("%w: broker: duplicate backend name(s): %v", ErrInvalidBackend, names)
	}

	if cfg.DefaultBackend != "" {
		if _, ok := backends[cfg.DefaultBackend]; !ok {
			return nil, fmt.Errorf("%w: broker: default_backend %q does not name a registered backend", ErrInvalidBackend, cfg.DefaultBackend)
		}
	}

	router := cfg.Router
	if router == nil {
		empty, _ := NewRouter(nil) // never errors on a nil slice
		router = empty
	}

	// Cross-check: every route's backend name MUST resolve. Catching
	// the drift here rather than at dispatch time means a stale config
	// file fails fast at boot.
	for _, route := range router.Routes() {
		if _, ok := backends[route.Backend]; !ok {
			return nil, fmt.Errorf("%w: broker: route %q references unknown backend %q", ErrInvalidBackend, route.Prefix, route.Backend)
		}
	}

	cache := cfg.Cache
	if cache == nil {
		cache = DefaultCache()
	}
	auditor := cfg.Auditor
	if auditor == nil {
		auditor = DefaultAuditor()
	}
	leases := cfg.LeaseDirectory
	if leases == nil {
		leases = NewInMemoryLeaseDirectory()
	}
	extract := cfg.ExtractPrincipal
	if extract == nil {
		extract = func(context.Context) Principal { return Principal{} }
	}
	clock := cfg.Clock
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &Broker{
		router:           router,
		backends:         backends,
		defaultBackend:   cfg.DefaultBackend,
		cache:            cache,
		auditor:          auditor,
		leases:           leases,
		extractPrincipal: extract,
		clock:            clock,
		logger:           logger,
	}, nil
}

// Start fans out to every registered backend's [SecretBackend.Start].
// If any backend fails, already-started backends are best-effort
// stopped before the error returns. Idempotent rejection of
// double-Start.
func (b *Broker) Start(ctx context.Context) error {
	b.mu.Lock()
	if b.stopped {
		b.mu.Unlock()
		return fmt.Errorf("%w: broker: cannot Start after Stop", ErrInvalidBackend)
	}
	if b.started {
		b.mu.Unlock()
		return fmt.Errorf("%w: broker: already started", ErrInvalidBackend)
	}
	b.started = true
	b.mu.Unlock()

	started := make([]SecretBackend, 0, len(b.backends))
	for name, backend := range b.backends {
		if err := backend.Start(ctx); err != nil {
			// Roll back: best-effort stop on what we already started.
			for _, prev := range started {
				_ = prev.Stop(ctx)
			}
			return fmt.Errorf("%w: broker: backend %q failed to start: %v", ErrInvalidBackend, name, err)
		}
		started = append(started, backend)
	}
	b.logger.LogAttrs(ctx, slog.LevelInfo, "secrets.broker: started",
		slog.Int("backends", len(b.backends)),
		slog.Int("routes", b.router.Len()),
		slog.String("default_backend", b.defaultBackend),
	)
	return nil
}

// Stop fans out to every backend's [SecretBackend.Stop]. Idempotent;
// errors are logged and continue (Stop is best-effort by contract).
func (b *Broker) Stop(ctx context.Context) error {
	b.mu.Lock()
	if b.stopped {
		b.mu.Unlock()
		return nil
	}
	b.stopped = true
	b.mu.Unlock()

	for name, backend := range b.backends {
		if err := backend.Stop(ctx); err != nil {
			b.logger.LogAttrs(ctx, slog.LevelWarn, "secrets.broker: backend stop error",
				slog.String("backend", name),
				slog.String("err", err.Error()),
			)
		}
	}
	return nil
}

// Health returns nil only when every backend's [SecretBackend.Health]
// returns nil. The first non-nil error wins and is prefixed with the
// failing backend's name.
func (b *Broker) Health(ctx context.Context) error {
	if !b.isStarted() {
		return ErrBackendNotStarted
	}
	for name, backend := range b.backends {
		if err := backend.Health(ctx); err != nil {
			return fmt.Errorf("backend %q unhealthy: %w", name, err)
		}
	}
	return nil
}

func (b *Broker) isStarted() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.started && !b.stopped
}

// resolve returns the backend instance the broker would dispatch a
// request for path to: router first, then DefaultBackend fallback.
// Returns ErrInvalidBackend wrapped when no route AND no default.
func (b *Broker) resolve(path string) (SecretBackend, Route, error) {
	if route, ok := b.router.Lookup(path); ok {
		backend := b.backends[route.Backend]
		return backend, route, nil
	}
	if b.defaultBackend == "" {
		return nil, Route{}, fmt.Errorf("%w: no route matched path %q and no default_backend configured", ErrInvalidBackend, path)
	}
	return b.backends[b.defaultBackend], Route{Prefix: "", Backend: b.defaultBackend}, nil
}

// requireCapability is the broker-side capability check that runs
// after routing and before dispatch. Returns nil on success, an
// ErrInvalidBackend-wrapped error otherwise.
func requireCapability(backend SecretBackend, capability BackendCapability) error {
	if HasCapability(backend.Capabilities(), capability) {
		return nil
	}
	return fmt.Errorf("%w: backend %q does not support capability %s", ErrInvalidBackend, backend.Name(), capability)
}

// errorReason returns the audit-row safe summary of err. Wrapped
// sentinels stringify their full chain; nil err returns "".
func errorReason(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// emit fires one audit event. Centralised so every dispatch path
// follows the same shape.
func (b *Broker) emit(ctx context.Context, evt SecretAccessEvent) {
	b.auditor.Emit(ctx, evt)
}

// GetSecret reads the secret at req.Path. Cache is consulted first;
// on miss, the matched backend is dispatched and a successful result
// is written back to the cache.
func (b *Broker) GetSecret(ctx context.Context, req GetSecretRequest) (*Secret, error) {
	principal := b.extractPrincipal(ctx)
	start := b.clock()

	if !req.Refresh {
		if cached, ok := b.cache.Get(req.Path); ok {
			b.emit(ctx, SecretAccessEvent{
				Timestamp: start,
				Action:    ActionGetSecret,
				Path:      req.Path,
				Backend:   CacheBackendLabel,
				Principal: principal,
				Allowed:   true,
				Duration:  b.clock().Sub(start),
			})
			return cached, nil
		}
	}

	backend, route, err := b.resolve(req.Path)
	if err != nil {
		b.emit(ctx, SecretAccessEvent{
			Timestamp:   start,
			Action:      ActionGetSecret,
			Path:        req.Path,
			Principal:   principal,
			Allowed:     false,
			ErrorReason: errorReason(err),
			Duration:    b.clock().Sub(start),
		})
		return nil, err
	}

	if err := requireCapability(backend, CapKV); err != nil {
		b.emit(ctx, SecretAccessEvent{
			Timestamp:   start,
			Action:      ActionGetSecret,
			Path:        req.Path,
			Backend:     route.Backend,
			Principal:   principal,
			Allowed:     false,
			ErrorReason: errorReason(err),
			Duration:    b.clock().Sub(start),
		})
		return nil, err
	}

	secret, err := backend.GetSecret(ctx, req)
	evt := SecretAccessEvent{
		Timestamp:   start,
		Action:      ActionGetSecret,
		Path:        req.Path,
		Backend:     route.Backend,
		Principal:   principal,
		Allowed:     err == nil,
		ErrorReason: errorReason(err),
		Duration:    b.clock().Sub(start),
	}
	b.emit(ctx, evt)
	if err == nil && secret != nil {
		b.cache.Put(req.Path, secret)
	}
	return secret, err
}

// WriteSecret writes (creates or updates) a secret. On success the
// cache entry for the path is invalidated.
func (b *Broker) WriteSecret(ctx context.Context, req WriteSecretRequest) (*Secret, error) {
	principal := b.extractPrincipal(ctx)
	start := b.clock()

	backend, route, err := b.resolve(req.Path)
	if err != nil {
		b.emit(ctx, SecretAccessEvent{
			Timestamp:   start,
			Action:      ActionWriteSecret,
			Path:        req.Path,
			Principal:   principal,
			Allowed:     false,
			ErrorReason: errorReason(err),
			Duration:    b.clock().Sub(start),
		})
		return nil, err
	}
	if err := requireCapability(backend, CapKV); err != nil {
		b.emit(ctx, SecretAccessEvent{
			Timestamp:   start,
			Action:      ActionWriteSecret,
			Path:        req.Path,
			Backend:     route.Backend,
			Principal:   principal,
			Allowed:     false,
			ErrorReason: errorReason(err),
			Duration:    b.clock().Sub(start),
		})
		return nil, err
	}

	secret, err := backend.WriteSecret(ctx, req)
	evt := SecretAccessEvent{
		Timestamp:   start,
		Action:      ActionWriteSecret,
		Path:        req.Path,
		Backend:     route.Backend,
		Principal:   principal,
		Allowed:     err == nil,
		ErrorReason: errorReason(err),
		Duration:    b.clock().Sub(start),
	}
	if req.Data != nil {
		evt.MaskedPayload = maskMap(req.Data)
	}
	b.emit(ctx, evt)
	if err == nil {
		b.cache.InvalidatePath(req.Path)
	}
	return secret, err
}

// ListSecrets enumerates paths under req.Prefix. No cache layer (list
// responses are metadata-only per the v1.0 contract; caching them adds
// staleness risk for no measurable win).
func (b *Broker) ListSecrets(ctx context.Context, req ListSecretsRequest) (*ListSecretsResponse, error) {
	principal := b.extractPrincipal(ctx)
	start := b.clock()

	backend, route, err := b.resolve(req.Prefix)
	if err != nil {
		b.emit(ctx, SecretAccessEvent{
			Timestamp:   start,
			Action:      ActionListSecrets,
			Path:        req.Prefix,
			Principal:   principal,
			Allowed:     false,
			ErrorReason: errorReason(err),
			Duration:    b.clock().Sub(start),
		})
		return nil, err
	}
	if err := requireCapability(backend, CapList); err != nil {
		b.emit(ctx, SecretAccessEvent{
			Timestamp:   start,
			Action:      ActionListSecrets,
			Path:        req.Prefix,
			Backend:     route.Backend,
			Principal:   principal,
			Allowed:     false,
			ErrorReason: errorReason(err),
			Duration:    b.clock().Sub(start),
		})
		return nil, err
	}

	resp, err := backend.ListSecrets(ctx, req)
	b.emit(ctx, SecretAccessEvent{
		Timestamp:   start,
		Action:      ActionListSecrets,
		Path:        req.Prefix,
		Backend:     route.Backend,
		Principal:   principal,
		Allowed:     err == nil,
		ErrorReason: errorReason(err),
		Duration:    b.clock().Sub(start),
	})
	return resp, err
}

// DeleteSecret removes the secret at req.Path; on success the cache
// entry is invalidated.
func (b *Broker) DeleteSecret(ctx context.Context, req DeleteSecretRequest) error {
	principal := b.extractPrincipal(ctx)
	start := b.clock()

	backend, route, err := b.resolve(req.Path)
	if err != nil {
		b.emit(ctx, SecretAccessEvent{
			Timestamp:   start,
			Action:      ActionDeleteSecret,
			Path:        req.Path,
			Principal:   principal,
			Allowed:     false,
			ErrorReason: errorReason(err),
			Duration:    b.clock().Sub(start),
		})
		return err
	}
	if err := requireCapability(backend, CapKV); err != nil {
		b.emit(ctx, SecretAccessEvent{
			Timestamp:   start,
			Action:      ActionDeleteSecret,
			Path:        req.Path,
			Backend:     route.Backend,
			Principal:   principal,
			Allowed:     false,
			ErrorReason: errorReason(err),
			Duration:    b.clock().Sub(start),
		})
		return err
	}

	err = backend.DeleteSecret(ctx, req)
	b.emit(ctx, SecretAccessEvent{
		Timestamp:   start,
		Action:      ActionDeleteSecret,
		Path:        req.Path,
		Backend:     route.Backend,
		Principal:   principal,
		Allowed:     err == nil,
		ErrorReason: errorReason(err),
		Duration:    b.clock().Sub(start),
	})
	if err == nil {
		b.cache.InvalidatePath(req.Path)
	}
	return err
}

// IssueDynamicSecret asks the matched backend for a fresh leased
// credential and records the lease in the directory so RenewLease /
// RevokeLease know which backend to route to. Dynamic secrets are
// NOT cached — they have one-time-issue semantics.
func (b *Broker) IssueDynamicSecret(ctx context.Context, req IssueDynamicSecretRequest) (*Secret, error) {
	principal := b.extractPrincipal(ctx)
	start := b.clock()

	backend, route, err := b.resolve(req.Path)
	if err != nil {
		b.emit(ctx, SecretAccessEvent{
			Timestamp:   start,
			Action:      ActionIssueDynamicSecret,
			Path:        req.Path,
			Principal:   principal,
			Allowed:     false,
			ErrorReason: errorReason(err),
			Duration:    b.clock().Sub(start),
		})
		return nil, err
	}
	if err := requireCapability(backend, CapDynamic); err != nil {
		b.emit(ctx, SecretAccessEvent{
			Timestamp:   start,
			Action:      ActionIssueDynamicSecret,
			Path:        req.Path,
			Backend:     route.Backend,
			Principal:   principal,
			Allowed:     false,
			ErrorReason: errorReason(err),
			Duration:    b.clock().Sub(start),
		})
		return nil, err
	}

	secret, err := backend.IssueDynamicSecret(ctx, req)
	evt := SecretAccessEvent{
		Timestamp:   start,
		Action:      ActionIssueDynamicSecret,
		Path:        req.Path,
		Backend:     route.Backend,
		Principal:   principal,
		Allowed:     err == nil,
		ErrorReason: errorReason(err),
		Duration:    b.clock().Sub(start),
	}
	if secret != nil {
		evt.LeaseID = secret.LeaseID
		if secret.Data != nil {
			evt.MaskedPayload = maskMap(secret.Data)
		}
	}
	b.emit(ctx, evt)
	if err == nil && secret != nil && secret.LeaseID != "" {
		b.leases.Record(secret.LeaseID, LeaseRecord{Backend: route.Backend, Path: req.Path})
	}
	return secret, err
}

// RenewLease extends a lease's TTL. The lease directory provides
// LeaseID → backend routing.
func (b *Broker) RenewLease(ctx context.Context, req RenewLeaseRequest) (*LeaseInfo, error) {
	principal := b.extractPrincipal(ctx)
	start := b.clock()

	record, ok := b.leases.Lookup(req.LeaseID)
	if !ok {
		err := fmt.Errorf("%w: lease %q", ErrLeaseNotFound, req.LeaseID)
		b.emit(ctx, SecretAccessEvent{
			Timestamp:   start,
			Action:      ActionRenewLease,
			LeaseID:     req.LeaseID,
			Principal:   principal,
			Allowed:     false,
			ErrorReason: errorReason(err),
			Duration:    b.clock().Sub(start),
		})
		return nil, err
	}

	backend, backendKnown := b.backends[record.Backend]
	if !backendKnown {
		err := fmt.Errorf("%w: lease %q references unknown backend %q", ErrInvalidBackend, req.LeaseID, record.Backend)
		b.emit(ctx, SecretAccessEvent{
			Timestamp:   start,
			Action:      ActionRenewLease,
			LeaseID:     req.LeaseID,
			Path:        record.Path,
			Backend:     record.Backend,
			Principal:   principal,
			Allowed:     false,
			ErrorReason: errorReason(err),
			Duration:    b.clock().Sub(start),
		})
		return nil, err
	}
	if err := requireCapability(backend, CapLeaseRenew); err != nil {
		b.emit(ctx, SecretAccessEvent{
			Timestamp:   start,
			Action:      ActionRenewLease,
			LeaseID:     req.LeaseID,
			Path:        record.Path,
			Backend:     record.Backend,
			Principal:   principal,
			Allowed:     false,
			ErrorReason: errorReason(err),
			Duration:    b.clock().Sub(start),
		})
		return nil, err
	}

	info, err := backend.RenewLease(ctx, req)
	b.emit(ctx, SecretAccessEvent{
		Timestamp:   start,
		Action:      ActionRenewLease,
		LeaseID:     req.LeaseID,
		Path:        record.Path,
		Backend:     record.Backend,
		Principal:   principal,
		Allowed:     err == nil,
		ErrorReason: errorReason(err),
		Duration:    b.clock().Sub(start),
	})
	if err != nil && errors.Is(err, ErrLeaseExpired) {
		// Backend says the lease is gone — drop it from the directory
		// so a follow-up RenewLease returns the canonical "not found"
		// rather than a confusing "expired" loop.
		b.leases.Forget(req.LeaseID)
	}
	return info, err
}

// RevokeLease tears a lease down. On success the lease is forgotten
// from the directory and the secret-path cache entry is invalidated
// so a revoked credential never reads from a stale hit.
func (b *Broker) RevokeLease(ctx context.Context, req RevokeLeaseRequest) error {
	principal := b.extractPrincipal(ctx)
	start := b.clock()

	record, ok := b.leases.Lookup(req.LeaseID)
	if !ok {
		err := fmt.Errorf("%w: lease %q", ErrLeaseNotFound, req.LeaseID)
		b.emit(ctx, SecretAccessEvent{
			Timestamp:   start,
			Action:      ActionRevokeLease,
			LeaseID:     req.LeaseID,
			Principal:   principal,
			Allowed:     false,
			ErrorReason: errorReason(err),
			Duration:    b.clock().Sub(start),
		})
		return err
	}

	backend, backendKnown := b.backends[record.Backend]
	if !backendKnown {
		err := fmt.Errorf("%w: lease %q references unknown backend %q", ErrInvalidBackend, req.LeaseID, record.Backend)
		b.emit(ctx, SecretAccessEvent{
			Timestamp:   start,
			Action:      ActionRevokeLease,
			LeaseID:     req.LeaseID,
			Path:        record.Path,
			Backend:     record.Backend,
			Principal:   principal,
			Allowed:     false,
			ErrorReason: errorReason(err),
			Duration:    b.clock().Sub(start),
		})
		return err
	}
	if err := requireCapability(backend, CapLeaseRevoke); err != nil {
		b.emit(ctx, SecretAccessEvent{
			Timestamp:   start,
			Action:      ActionRevokeLease,
			LeaseID:     req.LeaseID,
			Path:        record.Path,
			Backend:     record.Backend,
			Principal:   principal,
			Allowed:     false,
			ErrorReason: errorReason(err),
			Duration:    b.clock().Sub(start),
		})
		return err
	}

	err := backend.RevokeLease(ctx, req)
	b.emit(ctx, SecretAccessEvent{
		Timestamp:   start,
		Action:      ActionRevokeLease,
		LeaseID:     req.LeaseID,
		Path:        record.Path,
		Backend:     record.Backend,
		Principal:   principal,
		Allowed:     err == nil,
		ErrorReason: errorReason(err),
		Duration:    b.clock().Sub(start),
	})
	if err == nil {
		b.leases.Forget(req.LeaseID)
		if record.Path != "" {
			b.cache.InvalidatePath(record.Path)
		}
	}
	return err
}
