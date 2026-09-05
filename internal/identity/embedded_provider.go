// SPDX-License-Identifier: Apache-2.0

package identity

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

// EmbeddedProvider is the v0.1 default [Provider] implementation.
// It composes the task-5 [CAManager] (root + signing CA, persistence,
// rotation primitives) and the task-6 [CARotator] (background
// polling loop) plus — once tasks 8-11 land — an attestation
// engine and a join-token store. Tasks 8-11 swap in real impls
// behind the placeholder methods.
//
// SVID issuance:
//   - [EmbeddedProvider.IssueX509SVID] generates the subject's
//     private key, mints a leaf via [CAManager.IssueCertificate],
//     and wraps the result in [X509SVID] (both halves returned to
//     the caller).
//   - [EmbeddedProvider.IssueJWTSVID] signs with the CA's signing
//     key, with a kid derived from the signing CA's serial number.
//     On signing-CA rotation the kid rotates with it; the trust
//     bundle's JWT authority set follows.
//
// Trust bundle:
//   - [EmbeddedProvider.GetTrustBundle] returns a clone of the
//     current bundle (callers can't mutate provider state).
//   - [EmbeddedProvider.WatchTrustBundle] returns a buffered
//     channel that receives the current bundle immediately + an
//     update after every successful CA rotation. The channel
//     closes when its ctx is canceled or [Provider.Stop] runs.
//     Slow consumers don't pin the rotator: sends are non-blocking
//     against a cap-1 buffer; a full buffer drops the update
//     (consumer can always call [Provider.GetTrustBundle] for the
//     latest).
type EmbeddedProvider struct {
	cfg EmbeddedProviderConfig

	manager   *CAManager
	rotator   *CARotator
	attestors map[AttestorType]Attestor
	tokens    JoinTokenStore

	mu        sync.RWMutex
	bundle    *TrustBundle
	watchers  map[chan *TrustBundle]struct{}
	startedAt time.Time // wall-clock of last Start; reported via Status

	started  atomic.Bool
	stopped  atomic.Bool
	stopOnce sync.Once
}

// EmbeddedProviderConfig drives [NewEmbeddedProvider]. Storage +
// CAConfig.TrustDomain are required; everything else falls back
// to documented defaults.
type EmbeddedProviderConfig struct {
	CAConfig        CAConfig
	Storage         CAStorage
	RotatorInterval time.Duration    // optional; defaults to DefaultCARotatorInterval
	Clock           func() time.Time // optional; defaults to time.Now
	Logger          *slog.Logger     // optional; defaults to slog.Default

	// Attestors is the v0.1 pluggable attestation registry.
	// Each entry handles one [AttestorType]; duplicates are
	// rejected at construction. Leave empty (or nil) to leave
	// [EmbeddedProvider.Attest] returning [ErrNotImplementedYet]
	// — the v0.1 default until the operator wires
	// [NewJoinTokenAttestor] (task 8) once tasks 9-11 ship the
	// concrete [JoinTokenStore].
	Attestors []Attestor

	// JoinTokenStore backs the CreateJoinToken / ListJoinTokens /
	// DeleteJoinToken methods (task 10). Leave nil to leave those
	// methods returning [ErrJoinTokenStoreNotConfigured] —
	// suitable for read-only / SVID-issuance-only deployments.
	// v0.1 typically wires [NewInMemoryJoinTokenStore]; multi-CP
	// HA (Epic 13) wires [NewStateJoinTokenStore] against the
	// cluster's [state.Store].
	JoinTokenStore JoinTokenStore
}

// Compile-time interface assertion.
var _ Provider = (*EmbeddedProvider)(nil)

// NewEmbeddedProvider validates cfg + storage and returns an
// unstarted provider. Call [EmbeddedProvider.Start] before any
// other method.
func NewEmbeddedProvider(cfg EmbeddedProviderConfig) (*EmbeddedProvider, error) {
	if cfg.Storage == nil {
		return nil, fmt.Errorf("%w: Storage is required", ErrInvalidProvider)
	}
	if cfg.CAConfig.TrustDomain == "" {
		return nil, fmt.Errorf("%w: CAConfig.TrustDomain is required", ErrInvalidProvider)
	}
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.CAConfig.Clock == nil {
		cfg.CAConfig.Clock = cfg.Clock
	}
	manager, err := NewCAManager(cfg.CAConfig, cfg.Storage)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidProvider, err)
	}
	attestors, err := buildAttestorRegistry(cfg.Attestors)
	if err != nil {
		return nil, err
	}
	return &EmbeddedProvider{
		cfg:       cfg,
		manager:   manager,
		attestors: attestors,
		tokens:    cfg.JoinTokenStore, // may be nil
		watchers:  make(map[chan *TrustBundle]struct{}),
	}, nil
}

// buildAttestorRegistry validates the Attestors slice and indexes
// it by Type. Rejects nil entries + duplicate types.
func buildAttestorRegistry(in []Attestor) (map[AttestorType]Attestor, error) {
	out := make(map[AttestorType]Attestor, len(in))
	for i, a := range in {
		if a == nil {
			return nil, fmt.Errorf("%w: Attestors[%d] is nil", ErrInvalidProvider, i)
		}
		typ := a.Type()
		if typ == "" {
			return nil, fmt.Errorf("%w: Attestors[%d].Type() is empty", ErrInvalidProvider, i)
		}
		if _, dup := out[typ]; dup {
			return nil, fmt.Errorf("%w: Attestors[%d] duplicates type %q", ErrInvalidProvider, i, typ)
		}
		out[typ] = a
	}
	return out, nil
}

// Start initializes the CA + builds the initial trust bundle +
// spawns the rotation loop. One-shot — a provider that has been
// stopped cannot be restarted.
func (p *EmbeddedProvider) Start(ctx context.Context) error {
	if p.stopped.Load() {
		return fmt.Errorf("%w: provider already stopped (build a fresh one)", ErrInvalidProvider)
	}
	if !p.started.CompareAndSwap(false, true) {
		return fmt.Errorf("%w: already started", ErrInvalidProvider)
	}
	if err := p.manager.Initialize(ctx); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidProvider, err)
	}
	bundle, err := p.buildBundle()
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidProvider, err)
	}
	p.mu.Lock()
	p.bundle = bundle
	p.startedAt = p.cfg.Clock()
	p.mu.Unlock()

	rot, err := NewCARotator(CARotatorConfig{
		Manager:         p.manager,
		Interval:        p.cfg.RotatorInterval,
		Clock:           p.cfg.Clock,
		Logger:          p.cfg.Logger,
		OnRotateSuccess: p.rebuildAndNotify,
	})
	if err != nil {
		return fmt.Errorf("%w: rotator: %v", ErrInvalidProvider, err)
	}
	if err := rot.Start(ctx); err != nil {
		return fmt.Errorf("%w: rotator: %v", ErrInvalidProvider, err)
	}
	p.rotator = rot
	return nil
}

// Stop terminates the rotator + closes every active watcher.
// Idempotent.
func (p *EmbeddedProvider) Stop(ctx context.Context) error {
	var stopErr error
	p.stopOnce.Do(func() {
		p.stopped.Store(true)
		if p.rotator != nil {
			stopErr = p.rotator.Stop(ctx)
		}
		p.mu.Lock()
		for ch := range p.watchers {
			close(ch)
			delete(p.watchers, ch)
		}
		p.mu.Unlock()
	})
	return stopErr
}

// Health returns nil when the provider is running, [ErrProviderNotRunning]
// otherwise.
func (p *EmbeddedProvider) Health(_ context.Context) error {
	if !p.started.Load() || p.stopped.Load() {
		return ErrProviderNotRunning
	}
	return nil
}

// TrustDomain returns the configured trust domain. Safe to call
// before [EmbeddedProvider.Start].
func (p *EmbeddedProvider) TrustDomain() string {
	return p.cfg.CAConfig.TrustDomain
}

// GetTrustBundle returns a clone of the current trust bundle.
// [ErrProviderNotRunning] before Start.
func (p *EmbeddedProvider) GetTrustBundle(_ context.Context) (*TrustBundle, error) {
	if !p.started.Load() || p.stopped.Load() {
		return nil, ErrProviderNotRunning
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.bundle == nil {
		return nil, ErrProviderNotRunning
	}
	return p.bundle.Clone(), nil
}

// WatchTrustBundle returns a buffered channel that receives the
// current trust bundle immediately + an update on each successful
// rotation. The channel closes when ctx is canceled or Stop runs.
//
// The channel is buffered (cap 1); slow consumers don't block
// rotation — a full buffer drops the update, and the consumer can
// call [EmbeddedProvider.GetTrustBundle] for the latest at any
// time.
func (p *EmbeddedProvider) WatchTrustBundle(ctx context.Context) (<-chan *TrustBundle, error) {
	if !p.started.Load() || p.stopped.Load() {
		return nil, ErrProviderNotRunning
	}
	ch := make(chan *TrustBundle, 1)
	p.mu.Lock()
	current := p.bundle
	p.watchers[ch] = struct{}{}
	p.mu.Unlock()

	if current != nil {
		ch <- current.Clone()
	}
	// Unregister on ctx cancellation. The goroutine exits
	// quickly — it just closes the channel + removes the entry.
	go func() {
		<-ctx.Done()
		p.mu.Lock()
		if _, ok := p.watchers[ch]; ok {
			close(ch)
			delete(p.watchers, ch)
		}
		p.mu.Unlock()
	}()
	return ch, nil
}

// IssueX509SVID generates a fresh subject key, mints a leaf via
// the CA, and wraps the result in an [X509SVID] (both chain and
// key). The returned SVID is the on-wire form for the agent's
// initial enrollment + every subsequent rotation.
func (p *EmbeddedProvider) IssueX509SVID(_ context.Context, req IssueX509SVIDRequest) (X509SVID, error) {
	if !p.started.Load() || p.stopped.Load() {
		return X509SVID{}, ErrProviderNotRunning
	}
	if req.ID.IsZero() {
		return X509SVID{}, fmt.Errorf("%w: ID is required", ErrInvalidProvider)
	}
	keyType := req.KeyType
	if keyType == "" {
		keyType = p.cfg.CAConfig.KeyType
	}
	subjKey, err := generateKey(keyType)
	if err != nil {
		return X509SVID{}, fmt.Errorf("%w: subject key: %v", ErrInvalidProvider, err)
	}
	issued, err := p.manager.IssueCertificate(IssueRequest{
		ID:          req.ID,
		PublicKey:   subjKey.Public(),
		TTL:         req.TTL,
		DNSNames:    req.DNSNames,
		IPAddresses: req.IPAddresses,
	})
	if err != nil {
		return X509SVID{}, fmt.Errorf("%w: %v", ErrInvalidProvider, err)
	}
	svid, err := NewX509SVID(req.ID, issued.Chain, subjKey, req.Hint)
	if err != nil {
		return X509SVID{}, fmt.Errorf("%w: wrap svid: %v", ErrInvalidProvider, err)
	}
	return svid, nil
}

// IssueJWTSVID signs a JWT-SVID with the CA's signing key, using
// a kid derived from the signing CA's serial. The kid matches the
// JWT authority registered in the current trust bundle so a
// [ParseJWTSVID] call against the bundle verifies the token.
func (p *EmbeddedProvider) IssueJWTSVID(_ context.Context, req IssueJWTSVIDRequest) (JWTSVID, error) {
	if !p.started.Load() || p.stopped.Load() {
		return JWTSVID{}, ErrProviderNotRunning
	}
	ttl := req.TTL
	if ttl <= 0 {
		ttl = p.cfg.CAConfig.DefaultSVIDTTL
	}
	if ttl > p.cfg.CAConfig.MaxSVIDTTL {
		ttl = p.cfg.CAConfig.MaxSVIDTTL
	}
	p.mu.RLock()
	signingKey := p.manager.signingKey
	kid := p.currentSigningKIDLocked()
	p.mu.RUnlock()

	return SignJWTSVID(SignJWTSVIDRequest{
		ID:          req.ID,
		Audience:    req.Audience,
		Lifetime:    ttl,
		Key:         signingKey,
		KeyID:       kid,
		Hint:        req.Hint,
		ExtraClaims: req.ExtraClaims,
		Now:         p.cfg.Clock(),
	})
}

// ---- placeholder methods (filled in by tasks 8-11) --------------

// Attest dispatches the request to the registered [Attestor]
// whose Type matches. Returns [ErrNotImplementedYet] when no
// attestors are configured (the no-op default that preserves
// task-7 behavior). Returns [ErrAttestorNotConfigured] when
// attestors are configured but none handle the requested type.
// Returns [ErrProviderNotRunning] before Start / after Stop.
func (p *EmbeddedProvider) Attest(ctx context.Context, req AttestRequest) (*AttestResult, error) {
	if !p.started.Load() || p.stopped.Load() {
		return nil, ErrProviderNotRunning
	}
	if len(p.attestors) == 0 {
		return nil, ErrNotImplementedYet
	}
	att, ok := p.attestors[req.Type]
	if !ok {
		return nil, fmt.Errorf("%w: type=%q", ErrAttestorNotConfigured, req.Type)
	}
	return att.Attest(ctx, req.Data)
}

// CreateJoinToken mints a fresh cleartext token, persists the
// salted hash via the configured [JoinTokenStore], and returns
// the [JoinToken] with the cleartext `Token` set. **The cleartext
// is returned ONLY this once** — subsequent Get / List / Lookup
// calls return records with Token cleared. Defaults per §4.10:
// TTL [DefaultJoinTokenTTL] (5m), MaxUses [DefaultJoinTokenMaxUses]
// (1). Hard cap: TTL ≤ [MaxJoinTokenTTL] (24h).
//
// Returns [ErrProviderNotRunning] before Start / after Stop;
// [ErrJoinTokenStoreNotConfigured] when no store is wired;
// [ErrInvalidProvider] on TTL-out-of-bounds; the store's error
// otherwise.
func (p *EmbeddedProvider) CreateJoinToken(ctx context.Context, req CreateJoinTokenRequest) (JoinToken, error) {
	if !p.started.Load() || p.stopped.Load() {
		return JoinToken{}, ErrProviderNotRunning
	}
	if p.tokens == nil {
		return JoinToken{}, ErrJoinTokenStoreNotConfigured
	}

	ttl := req.TTL
	if ttl <= 0 {
		ttl = DefaultJoinTokenTTL
	}
	if ttl > MaxJoinTokenTTL {
		return JoinToken{}, fmt.Errorf("%w: TTL %s exceeds max %s", ErrInvalidProvider, ttl, MaxJoinTokenTTL)
	}
	maxUses := req.MaxUses
	if maxUses <= 0 {
		maxUses = DefaultJoinTokenMaxUses
	}

	body, err := randomBase62(joinTokenBodyLen)
	if err != nil {
		return JoinToken{}, fmt.Errorf("%w: %v", ErrInvalidProvider, err)
	}
	cleartext := JoinTokenScheme + body
	prefix := cleartext[:JoinTokenPrefixLen]
	salt, err := randomSalt()
	if err != nil {
		return JoinToken{}, fmt.Errorf("%w: %v", ErrInvalidProvider, err)
	}
	hash := saltedHash(salt, cleartext)

	now := p.cfg.Clock()
	token := JoinToken{
		ID:        uuid.NewString(),
		Token:     cleartext, // returned to caller — store will wipe before persist
		Hash:      hash,
		Salt:      salt,
		Prefix:    prefix,
		AgentID:   req.AgentID,
		TTL:       ttl,
		CreatedAt: now,
		ExpiresAt: now.Add(ttl),
		MaxUses:   maxUses,
		Metadata:  req.Metadata,
	}
	if err := p.tokens.Create(ctx, token); err != nil {
		return JoinToken{}, fmt.Errorf("%w: %w", ErrInvalidProvider, err)
	}
	return token, nil
}

// ListJoinTokens returns matching join-token records (with
// `Token` cleartext cleared, as the store guarantees). Delegates
// to [JoinTokenStore.List].
func (p *EmbeddedProvider) ListJoinTokens(ctx context.Context, filter ListJoinTokensFilter) ([]JoinToken, error) {
	if !p.started.Load() || p.stopped.Load() {
		return nil, ErrProviderNotRunning
	}
	if p.tokens == nil {
		return nil, ErrJoinTokenStoreNotConfigured
	}
	ptrs, err := p.tokens.List(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidProvider, err)
	}
	out := make([]JoinToken, 0, len(ptrs))
	for _, p := range ptrs {
		if p != nil {
			out = append(out, *p)
		}
	}
	return out, nil
}

// DeleteJoinToken removes a join-token record by ID. Propagates
// [ErrJoinTokenNotFound] verbatim so callers can branch with
// [errors.Is].
func (p *EmbeddedProvider) DeleteJoinToken(ctx context.Context, id string) error {
	if !p.started.Load() || p.stopped.Load() {
		return ErrProviderNotRunning
	}
	if p.tokens == nil {
		return ErrJoinTokenStoreNotConfigured
	}
	err := p.tokens.Delete(ctx, id)
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrJoinTokenNotFound) {
		return err
	}
	return fmt.Errorf("%w: %w", ErrInvalidProvider, err)
}

// ---- helpers -----------------------------------------------------

// buildBundle constructs a fresh trust bundle from the manager's
// current state: root cert as the X509 authority + signing CA's
// pubkey under a deterministic kid as the JWT authority. The kid
// is derived from the signing CA's serial number so a rotation
// changes both the kid AND the pubkey.
func (p *EmbeddedProvider) buildBundle() (*TrustBundle, error) {
	bundle, err := p.manager.BuildTrustBundle()
	if err != nil {
		return nil, err
	}
	p.manager.mu.RLock()
	signingCert := p.manager.signingCert
	p.manager.mu.RUnlock()
	if signingCert == nil {
		return nil, fmt.Errorf("%w: signing CA missing", ErrInvalidProvider)
	}
	kid := signingKID(signingCert.SerialNumber.Bytes())
	if err := bundle.AddJWTAuthority(kid, signingCert.PublicKey); err != nil {
		return nil, fmt.Errorf("%w: jwt authority: %v", ErrInvalidProvider, err)
	}
	return bundle, nil
}

// currentSigningKIDLocked returns the kid for the active signing
// CA. Caller must hold p.mu (read or write). Reads
// p.manager.signingCert behind the manager's own lock so a
// concurrent rotation doesn't tear the read.
func (p *EmbeddedProvider) currentSigningKIDLocked() string {
	p.manager.mu.RLock()
	defer p.manager.mu.RUnlock()
	if p.manager.signingCert == nil {
		return ""
	}
	return signingKID(p.manager.signingCert.SerialNumber.Bytes())
}

// signingKID renders a signing CA's serial bytes into a stable
// operator-readable kid. Truncated to 8 bytes (16 hex chars) — the
// full 16-byte serial is overkill, and the truncated form fits
// comfortably in log lines + JWT headers.
func signingKID(serial []byte) string {
	const truncTo = 8
	if len(serial) > truncTo {
		serial = serial[:truncTo]
	}
	return fmt.Sprintf("ks-signing-%x", serial)
}

// RotateSigningCA forces an immediate signing-CA rotation,
// bypassing the CARotator's normal "ShouldRotate" predicate.
// Persists the new signing CA, rebuilds the trust bundle, and
// notifies every watcher. Operators reach this via the
// IdentityService gRPC (task 12). Old leaves continue to verify —
// their chain still includes the old signing CA cert under the
// unchanged root.
//
// Returns [ErrProviderNotRunning] before Start / after Stop; the
// manager's error wrapped in [ErrInvalidProvider] otherwise.
func (p *EmbeddedProvider) RotateSigningCA(ctx context.Context) error {
	if !p.started.Load() || p.stopped.Load() {
		return ErrProviderNotRunning
	}
	if err := p.manager.RotateSigningCA(ctx); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidProvider, err)
	}
	p.rebuildAndNotify()
	return nil
}

// Manager returns the underlying [*CAManager]. The
// IdentityService gRPC server (task 12) uses this for the
// `GetCAInfo` + `ExportCA` paths that need to read the root +
// signing certs directly.
//
// Returns nil before [EmbeddedProvider.Start] has run successfully.
func (p *EmbeddedProvider) Manager() *CAManager {
	if !p.started.Load() || p.stopped.Load() {
		return nil
	}
	return p.manager
}

// JoinTokens returns the configured [JoinTokenStore]. Used by the
// IdentityService gRPC `GetStatus` path to surface token counts.
// nil before Start, when no store is configured, or after Stop.
func (p *EmbeddedProvider) JoinTokens() JoinTokenStore {
	if !p.started.Load() || p.stopped.Load() {
		return nil
	}
	return p.tokens
}

// WatcherCount returns the live trust-bundle subscriber count.
// Surfaced via the IdentityService `GetStatus` RPC.
func (p *EmbeddedProvider) WatcherCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.watchers)
}

// StartedAt returns the wall-clock time of the last successful
// [EmbeddedProvider.Start]. Zero before Start. Surfaced via the
// IdentityService `GetStatus` RPC for uptime calculation.
func (p *EmbeddedProvider) StartedAt() time.Time {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.startedAt
}

// rebuildAndNotify is the CARotator OnRotateSuccess callback —
// re-derives the trust bundle (kid + pubkey changed when the
// signing CA rotated) and pushes the fresh bundle to every active
// watcher. Sends are non-blocking against the cap-1 buffer.
func (p *EmbeddedProvider) rebuildAndNotify() {
	bundle, err := p.buildBundle()
	if err != nil {
		p.cfg.Logger.Error("rebuild trust bundle after rotation failed",
			"err", err,
		)
		return
	}
	p.mu.Lock()
	p.bundle = bundle
	watchers := make([]chan *TrustBundle, 0, len(p.watchers))
	for ch := range p.watchers {
		watchers = append(watchers, ch)
	}
	p.mu.Unlock()

	for _, ch := range watchers {
		select {
		case ch <- bundle.Clone():
		default:
			// Buffer full — drop. Slow consumers can always pull
			// the latest via GetTrustBundle.
		}
	}
}
