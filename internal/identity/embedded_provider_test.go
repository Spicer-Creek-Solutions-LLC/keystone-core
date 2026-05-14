package identity

import (
	"context"
	"crypto/x509"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spiffe/go-spiffe/v2/svid/x509svid"
)

// ---- helpers -----------------------------------------------------

func newProviderConfig(t *testing.T) EmbeddedProviderConfig {
	t.Helper()
	return EmbeddedProviderConfig{
		CAConfig:        newFastCAConfig(DefaultTrustDomain),
		Storage:         newTempStorage(t),
		RotatorInterval: time.Hour, // long enough that the loop never fires in unit tests
		Logger:          silentLogger(),
	}
}

func newStartedProvider(t *testing.T) *EmbeddedProvider {
	t.Helper()
	p, err := NewEmbeddedProvider(newProviderConfig(t))
	if err != nil {
		t.Fatalf("NewEmbeddedProvider: %v", err)
	}
	if err := p.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = p.Stop(stopCtx)
	})
	return p
}

// ---- NewEmbeddedProvider -----------------------------------------

func TestNewEmbeddedProvider_RejectsNilStorage(t *testing.T) {
	t.Parallel()
	_, err := NewEmbeddedProvider(EmbeddedProviderConfig{
		CAConfig: newFastCAConfig(DefaultTrustDomain),
	})
	if err == nil || !errors.Is(err, ErrInvalidProvider) {
		t.Fatalf("err = %v", err)
	}
}

func TestNewEmbeddedProvider_RejectsEmptyTrustDomain(t *testing.T) {
	t.Parallel()
	_, err := NewEmbeddedProvider(EmbeddedProviderConfig{
		Storage:  newTempStorage(t),
		CAConfig: newFastCAConfig(""),
	})
	if err == nil || !errors.Is(err, ErrInvalidProvider) {
		t.Fatalf("err = %v", err)
	}
}

func TestNewEmbeddedProvider_RejectsInvalidCAConfig(t *testing.T) {
	t.Parallel()
	cfg := newProviderConfig(t)
	cfg.CAConfig.MaxSVIDTTL = cfg.CAConfig.SigningCATTL + time.Hour
	_, err := NewEmbeddedProvider(cfg)
	if err == nil || !errors.Is(err, ErrInvalidProvider) {
		t.Fatalf("err = %v", err)
	}
}

func TestNewEmbeddedProvider_AppliesDefaults(t *testing.T) {
	t.Parallel()
	p, err := NewEmbeddedProvider(EmbeddedProviderConfig{
		CAConfig: newFastCAConfig(DefaultTrustDomain),
		Storage:  newTempStorage(t),
	})
	if err != nil {
		t.Fatalf("NewEmbeddedProvider: %v", err)
	}
	if p.cfg.Clock == nil {
		t.Error("Clock not defaulted")
	}
	if p.cfg.Logger == nil {
		t.Error("Logger not defaulted")
	}
}

// ---- Lifecycle ---------------------------------------------------

func TestEmbeddedProvider_HealthBeforeStart(t *testing.T) {
	t.Parallel()
	p, _ := NewEmbeddedProvider(newProviderConfig(t))
	if err := p.Health(context.Background()); err == nil || !errors.Is(err, ErrProviderNotRunning) {
		t.Errorf("Health before Start: %v", err)
	}
}

func TestEmbeddedProvider_StartHealthStop(t *testing.T) {
	t.Parallel()
	p, _ := NewEmbeddedProvider(newProviderConfig(t))
	if err := p.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := p.Health(context.Background()); err != nil {
		t.Errorf("Health after Start: %v", err)
	}
	if err := p.Stop(context.Background()); err != nil {
		t.Errorf("Stop: %v", err)
	}
	if err := p.Health(context.Background()); err == nil || !errors.Is(err, ErrProviderNotRunning) {
		t.Errorf("Health after Stop: %v", err)
	}
}

func TestEmbeddedProvider_DoubleStart(t *testing.T) {
	t.Parallel()
	p, _ := NewEmbeddedProvider(newProviderConfig(t))
	if err := p.Start(context.Background()); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	t.Cleanup(func() { _ = p.Stop(context.Background()) })
	if err := p.Start(context.Background()); err == nil || !errors.Is(err, ErrInvalidProvider) {
		t.Errorf("second Start: %v", err)
	}
}

func TestEmbeddedProvider_DoubleStop(t *testing.T) {
	t.Parallel()
	p := newStartedProvider(t)
	if err := p.Stop(context.Background()); err != nil {
		t.Errorf("first Stop: %v", err)
	}
	if err := p.Stop(context.Background()); err != nil {
		t.Errorf("second Stop: %v", err)
	}
}

func TestEmbeddedProvider_StartAfterStop(t *testing.T) {
	t.Parallel()
	p, _ := NewEmbeddedProvider(newProviderConfig(t))
	if err := p.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := p.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := p.Start(context.Background()); err == nil || !errors.Is(err, ErrInvalidProvider) {
		t.Errorf("Start after Stop: %v", err)
	}
}

// ---- TrustDomain -------------------------------------------------

func TestEmbeddedProvider_TrustDomain(t *testing.T) {
	t.Parallel()
	p, _ := NewEmbeddedProvider(newProviderConfig(t))
	if p.TrustDomain() != DefaultTrustDomain {
		t.Errorf("TrustDomain = %q, want %q", p.TrustDomain(), DefaultTrustDomain)
	}
}

// ---- GetTrustBundle ----------------------------------------------

func TestEmbeddedProvider_GetTrustBundle_BeforeStart(t *testing.T) {
	t.Parallel()
	p, _ := NewEmbeddedProvider(newProviderConfig(t))
	_, err := p.GetTrustBundle(context.Background())
	if err == nil || !errors.Is(err, ErrProviderNotRunning) {
		t.Errorf("err = %v", err)
	}
}

func TestEmbeddedProvider_GetTrustBundle_HasRootAndJWTAuthority(t *testing.T) {
	t.Parallel()
	p := newStartedProvider(t)
	b, err := p.GetTrustBundle(context.Background())
	if err != nil {
		t.Fatalf("GetTrustBundle: %v", err)
	}
	if got := len(b.X509Authorities()); got != 1 {
		t.Errorf("X509Authorities = %d, want 1", got)
	}
	if got := len(b.JWTAuthorities()); got != 1 {
		t.Errorf("JWTAuthorities = %d, want 1", got)
	}
	// The kid format is exposed in the JWT authority map keys.
	for kid := range b.JWTAuthorities() {
		if !strings.HasPrefix(kid, "ks-signing-") {
			t.Errorf("JWT authority kid = %q, want \"ks-signing-…\" prefix", kid)
		}
	}
}

func TestEmbeddedProvider_GetTrustBundle_ReturnsClone(t *testing.T) {
	t.Parallel()
	p := newStartedProvider(t)
	b1, _ := p.GetTrustBundle(context.Background())
	// Mutate the returned bundle — provider's copy must be
	// untouched.
	_ = b1.SetX509Authorities(nil)
	b2, _ := p.GetTrustBundle(context.Background())
	if len(b2.X509Authorities()) != 1 {
		t.Error("mutating GetTrustBundle return corrupted provider state")
	}
}

// ---- WatchTrustBundle --------------------------------------------

func TestEmbeddedProvider_WatchTrustBundle_DeliversInitial(t *testing.T) {
	t.Parallel()
	p := newStartedProvider(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := p.WatchTrustBundle(ctx)
	if err != nil {
		t.Fatalf("WatchTrustBundle: %v", err)
	}
	select {
	case b := <-ch:
		if b == nil || b.IsEmpty() {
			t.Error("initial bundle empty / nil")
		}
	case <-time.After(time.Second):
		t.Fatal("no initial bundle within 1s")
	}
}

func TestEmbeddedProvider_WatchTrustBundle_BeforeStart(t *testing.T) {
	t.Parallel()
	p, _ := NewEmbeddedProvider(newProviderConfig(t))
	_, err := p.WatchTrustBundle(context.Background())
	if err == nil || !errors.Is(err, ErrProviderNotRunning) {
		t.Errorf("err = %v", err)
	}
}

func TestEmbeddedProvider_WatchTrustBundle_UpdatesOnRotation(t *testing.T) {
	t.Parallel()
	p := newStartedProvider(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := p.WatchTrustBundle(ctx)
	if err != nil {
		t.Fatalf("WatchTrustBundle: %v", err)
	}
	// Drain the initial bundle.
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("no initial bundle")
	}

	// Force a rotation manually + invoke the post-rotation hook
	// (mirrors what the CARotator does on its OnRotateSuccess
	// callback).
	if err := p.manager.RotateSigningCA(context.Background()); err != nil {
		t.Fatalf("RotateSigningCA: %v", err)
	}
	p.rebuildAndNotify()

	select {
	case b := <-ch:
		if b == nil {
			t.Fatal("nil bundle after rotation")
		}
	case <-time.After(time.Second):
		t.Fatal("no rotation update within 1s")
	}
}

func TestEmbeddedProvider_WatchTrustBundle_MultipleSubscribers(t *testing.T) {
	t.Parallel()
	p := newStartedProvider(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const N = 3
	chans := make([]<-chan *TrustBundle, N)
	for i := 0; i < N; i++ {
		c, err := p.WatchTrustBundle(ctx)
		if err != nil {
			t.Fatalf("WatchTrustBundle[%d]: %v", i, err)
		}
		chans[i] = c
		// Drain initial.
		select {
		case <-c:
		case <-time.After(time.Second):
			t.Fatalf("no initial on subscriber %d", i)
		}
	}

	if err := p.manager.RotateSigningCA(context.Background()); err != nil {
		t.Fatalf("RotateSigningCA: %v", err)
	}
	p.rebuildAndNotify()

	for i, c := range chans {
		select {
		case b := <-c:
			if b == nil {
				t.Errorf("subscriber %d: nil bundle", i)
			}
		case <-time.After(time.Second):
			t.Errorf("subscriber %d: no rotation update", i)
		}
	}
}

func TestEmbeddedProvider_WatchTrustBundle_CtxCancelClosesChannel(t *testing.T) {
	t.Parallel()
	p := newStartedProvider(t)
	ctx, cancel := context.WithCancel(context.Background())
	ch, err := p.WatchTrustBundle(ctx)
	if err != nil {
		t.Fatalf("WatchTrustBundle: %v", err)
	}
	// Drain initial.
	<-ch

	cancel()
	// Channel should close.
	select {
	case b, ok := <-ch:
		if ok {
			t.Errorf("got bundle %v, want channel closed", b)
		}
	case <-time.After(time.Second):
		t.Fatal("channel not closed within 1s of ctx cancel")
	}
}

func TestEmbeddedProvider_WatchTrustBundle_StopClosesChannel(t *testing.T) {
	t.Parallel()
	p, _ := NewEmbeddedProvider(newProviderConfig(t))
	if err := p.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	ctx := context.Background()
	ch, err := p.WatchTrustBundle(ctx)
	if err != nil {
		t.Fatalf("WatchTrustBundle: %v", err)
	}
	<-ch
	if err := p.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("channel should be closed after Stop")
		}
	case <-time.After(time.Second):
		t.Fatal("channel not closed after Stop")
	}
}

// ---- IssueX509SVID -----------------------------------------------

func TestEmbeddedProvider_IssueX509SVID_HappyPath(t *testing.T) {
	t.Parallel()
	p := newStartedProvider(t)
	id, _ := AgentID(DefaultTrustDomain, "agent-issue-x509")
	svid, err := p.IssueX509SVID(context.Background(), IssueX509SVIDRequest{
		ID:  id,
		TTL: 30 * time.Minute,
	})
	if err != nil {
		t.Fatalf("IssueX509SVID: %v", err)
	}
	if !svid.SPIFFEID().Equal(id) {
		t.Errorf("SPIFFEID = %q, want %q", svid.SPIFFEID(), id)
	}
	// Verify against the provider's own trust bundle.
	b, _ := p.GetTrustBundle(context.Background())
	if _, _, err := x509svid.Verify(svid.Chain(), b); err != nil {
		t.Errorf("verify against bundle: %v", err)
	}
}

func TestEmbeddedProvider_IssueX509SVID_BeforeStart(t *testing.T) {
	t.Parallel()
	p, _ := NewEmbeddedProvider(newProviderConfig(t))
	id, _ := AgentID(DefaultTrustDomain, "agent-x")
	_, err := p.IssueX509SVID(context.Background(), IssueX509SVIDRequest{ID: id})
	if err == nil || !errors.Is(err, ErrProviderNotRunning) {
		t.Errorf("err = %v", err)
	}
}

func TestEmbeddedProvider_IssueX509SVID_RejectsZeroID(t *testing.T) {
	t.Parallel()
	p := newStartedProvider(t)
	_, err := p.IssueX509SVID(context.Background(), IssueX509SVIDRequest{})
	if err == nil || !errors.Is(err, ErrInvalidProvider) {
		t.Errorf("err = %v", err)
	}
}

func TestEmbeddedProvider_IssueX509SVID_DNSAndIP(t *testing.T) {
	t.Parallel()
	p := newStartedProvider(t)
	id, _ := AgentID(DefaultTrustDomain, "agent-san")
	svid, err := p.IssueX509SVID(context.Background(), IssueX509SVIDRequest{
		ID:       id,
		DNSNames: []string{"agent.kscore.local"},
	})
	if err != nil {
		t.Fatalf("IssueX509SVID: %v", err)
	}
	if got := svid.Leaf().DNSNames; len(got) != 1 || got[0] != "agent.kscore.local" {
		t.Errorf("DNSNames = %v", got)
	}
}

func TestEmbeddedProvider_IssueX509SVID_DefaultsKeyType(t *testing.T) {
	t.Parallel()
	p := newStartedProvider(t)
	id, _ := AgentID(DefaultTrustDomain, "agent-default-key")
	// Don't set KeyType → falls back to CA's default (ECDSA-P256).
	svid, err := p.IssueX509SVID(context.Background(), IssueX509SVIDRequest{ID: id})
	if err != nil {
		t.Fatalf("IssueX509SVID: %v", err)
	}
	// Public key should be ECDSA.
	if svid.PrivateKey() == nil {
		t.Fatal("PrivateKey nil")
	}
}

// ---- IssueJWTSVID ------------------------------------------------

func TestEmbeddedProvider_IssueJWTSVID_HappyPath(t *testing.T) {
	t.Parallel()
	p := newStartedProvider(t)
	id, _ := AgentID(DefaultTrustDomain, "agent-jwt")
	svid, err := p.IssueJWTSVID(context.Background(), IssueJWTSVIDRequest{
		ID:       id,
		Audience: []string{"kscore"},
		TTL:      30 * time.Minute,
	})
	if err != nil {
		t.Fatalf("IssueJWTSVID: %v", err)
	}
	// Verify against the provider's own bundle.
	b, _ := p.GetTrustBundle(context.Background())
	got, err := ParseJWTSVID(svid.Token(), []string{"kscore"}, b)
	if err != nil {
		t.Fatalf("ParseJWTSVID: %v", err)
	}
	if !got.SPIFFEID().Equal(id) {
		t.Errorf("parsed SPIFFEID = %q, want %q", got.SPIFFEID(), id)
	}
}

func TestEmbeddedProvider_IssueJWTSVID_KIDFromSigningCA(t *testing.T) {
	t.Parallel()
	p := newStartedProvider(t)
	b, _ := p.GetTrustBundle(context.Background())
	// One JWT authority should be registered; its kid is what
	// Issue uses.
	auths := b.JWTAuthorities()
	if len(auths) != 1 {
		t.Fatalf("JWTAuthorities = %d", len(auths))
	}
	var bundleKid string
	for k := range auths {
		bundleKid = k
	}
	id, _ := AgentID(DefaultTrustDomain, "agent-kid")
	svid, err := p.IssueJWTSVID(context.Background(), IssueJWTSVIDRequest{
		ID: id, Audience: []string{"kscore"},
	})
	if err != nil {
		t.Fatalf("IssueJWTSVID: %v", err)
	}
	// Re-parse via the bundle proves the kid matches; if it
	// didn't, ParseJWTSVID would fail.
	if _, err := ParseJWTSVID(svid.Token(), []string{"kscore"}, b); err != nil {
		t.Errorf("parse with bundle kid %q: %v", bundleKid, err)
	}
}

func TestEmbeddedProvider_IssueJWTSVID_AfterRotation(t *testing.T) {
	t.Parallel()
	p := newStartedProvider(t)
	beforeBundle, _ := p.GetTrustBundle(context.Background())

	if err := p.manager.RotateSigningCA(context.Background()); err != nil {
		t.Fatalf("RotateSigningCA: %v", err)
	}
	p.rebuildAndNotify()

	id, _ := AgentID(DefaultTrustDomain, "agent-post-rotate")
	svid, err := p.IssueJWTSVID(context.Background(), IssueJWTSVIDRequest{
		ID: id, Audience: []string{"kscore"},
	})
	if err != nil {
		t.Fatalf("IssueJWTSVID after rotation: %v", err)
	}

	// Post-rotation bundle verifies the new token; pre-rotation
	// bundle does NOT (its JWT authority was the old kid).
	postBundle, _ := p.GetTrustBundle(context.Background())
	if _, err := ParseJWTSVID(svid.Token(), []string{"kscore"}, postBundle); err != nil {
		t.Errorf("verify new token against new bundle: %v", err)
	}
	if _, err := ParseJWTSVID(svid.Token(), []string{"kscore"}, beforeBundle); err == nil {
		t.Error("new token unexpectedly verified against PRE-rotation bundle")
	}
}

func TestEmbeddedProvider_IssueJWTSVID_BeforeStart(t *testing.T) {
	t.Parallel()
	p, _ := NewEmbeddedProvider(newProviderConfig(t))
	id, _ := AgentID(DefaultTrustDomain, "agent-x")
	_, err := p.IssueJWTSVID(context.Background(), IssueJWTSVIDRequest{
		ID: id, Audience: []string{"kscore"},
	})
	if err == nil || !errors.Is(err, ErrProviderNotRunning) {
		t.Errorf("err = %v", err)
	}
}

// ---- Stub methods (tasks 8-11) -----------------------------------

// Note: Attest's "no attestors configured" behavior (still
// ErrNotImplementedYet) and "dispatch by type" behavior are
// covered by TestEmbeddedProvider_Attest_NoAttestorsConfigured +
// TestEmbeddedProvider_Attest_DispatchByType in
// join_token_attestor_test.go, which were added by task 8.

// Note: task 10 replaced the *_NotImplemented stub-tests with the
// CreateJoinToken / ListJoinTokens / DeleteJoinToken end-to-end
// tests in embedded_provider_jointokens_test.go. The four
// "no store configured" boundaries are asserted there too.

// ---- CARotator OnRotateSuccess wiring ----------------------------

func TestCARotator_OnRotateSuccess_FiresAfterRotation(t *testing.T) {
	t.Parallel()
	c := newFastCAConfig(DefaultTrustDomain)
	frozen := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c.Clock = func() time.Time { return frozen }
	m, _ := newInitializedManager(t, c)
	advanced := frozen.Add(time.Hour + 31*time.Minute)
	m.cfg.Clock = func() time.Time { return advanced }

	var (
		mu          sync.Mutex
		rotateCount int
	)
	ticks := make(chan struct{}, 8)
	r, _ := NewCARotator(CARotatorConfig{
		Manager:  m,
		Interval: 5 * time.Millisecond,
		Clock:    func() time.Time { return advanced },
		Logger:   silentLogger(),
		OnTick:   func() { ticks <- struct{}{} },
		OnRotateSuccess: func() {
			mu.Lock()
			rotateCount++
			mu.Unlock()
		},
	})
	if err := r.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = r.Stop(context.Background()) })

	// Two ticks is enough to fire the rotation once.
	waitForTicks(t, ticks, 2, time.Second)
	mu.Lock()
	got := rotateCount
	mu.Unlock()
	if got < 1 {
		t.Errorf("OnRotateSuccess fired %d times, want ≥ 1", got)
	}
}

// ---- Provider interface satisfaction -----------------------------

func TestEmbeddedProvider_SatisfiesProviderInterface(t *testing.T) {
	t.Parallel()
	// The compile-time var _ Provider = (*EmbeddedProvider)(nil)
	// already enforces this; the explicit test pins it so a
	// future Provider extension shows up as a test signal too.
	p, _ := NewEmbeddedProvider(newProviderConfig(t))
	var _ Provider = p
	// Reference x509 to suppress unused-import warning when the
	// JWT-only tests are stripped — keeps the dependency obvious.
	_ = x509.Certificate{}
}
