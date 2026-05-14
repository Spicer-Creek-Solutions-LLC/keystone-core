package identity

import (
	"context"
	"crypto/rand"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// ---- in-test fake JoinTokenStore --------------------------------

type fakeJoinTokenStore struct {
	mu sync.Mutex

	// lookupRecord is returned by Lookup when set; lookupErr
	// overrides it when set.
	lookupRecord *JoinToken
	lookupErr    error

	// markUsedErr lets tests inject a MarkUsed failure.
	markUsedErr error

	// Call counters.
	lookupCalls    int
	lookupArgs     []string
	markUsedCalls  int
	markUsedIDs    []string
	markUsedTimes  []time.Time
}

func (f *fakeJoinTokenStore) Lookup(_ context.Context, prefix string) (*JoinToken, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lookupCalls++
	f.lookupArgs = append(f.lookupArgs, prefix)
	if f.lookupErr != nil {
		return nil, f.lookupErr
	}
	return f.lookupRecord, nil
}

func (f *fakeJoinTokenStore) MarkUsed(_ context.Context, id string, now time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.markUsedCalls++
	f.markUsedIDs = append(f.markUsedIDs, id)
	f.markUsedTimes = append(f.markUsedTimes, now)
	return f.markUsedErr
}

// ---- helper: build a valid token + matching record --------------

// validTokenAndRecord generates a cleartext token + a JoinToken
// record whose Salt + Hash match the token. agentID populates
// AgentID; lifetime + maxUses populate the record's window /
// limit.
func validTokenAndRecord(t *testing.T, agentID string, lifetime time.Duration, maxUses int) (string, *JoinToken) {
	t.Helper()
	// 40-char random body (well above the 32-char min). Built
	// from rand.Read + a simple base62-ish encoding — good
	// enough for tests; task 9's real Create logic will use a
	// vetted base62.
	body := make([]byte, 40)
	if _, err := rand.Read(body); err != nil {
		t.Fatalf("rand: %v", err)
	}
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	bodyStr := make([]byte, len(body))
	for i, b := range body {
		bodyStr[i] = alphabet[int(b)%len(alphabet)]
	}
	token := JoinTokenScheme + string(bodyStr)
	prefix := token[:JoinTokenPrefixLen]

	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		t.Fatalf("salt: %v", err)
	}
	hash := saltedHash(salt, token)

	now := time.Now().Truncate(time.Second)
	rec := &JoinToken{
		ID:        "rec-" + prefix,
		Hash:      hash,
		Salt:      salt,
		Prefix:    prefix,
		AgentID:   agentID,
		TTL:       lifetime,
		CreatedAt: now,
		ExpiresAt: now.Add(lifetime),
		MaxUses:   maxUses,
		UsedCount: 0,
		Metadata:  map[string]string{"role": "web"},
	}
	return token, rec
}

// ---- NewJoinTokenAttestor ---------------------------------------

func TestNewJoinTokenAttestor_RejectsNilStore(t *testing.T) {
	t.Parallel()
	_, err := NewJoinTokenAttestor(JoinTokenAttestorConfig{TrustDomain: DefaultTrustDomain})
	if err == nil || !errors.Is(err, ErrInvalidProvider) {
		t.Fatalf("err = %v", err)
	}
}

func TestNewJoinTokenAttestor_RejectsEmptyTrustDomain(t *testing.T) {
	t.Parallel()
	_, err := NewJoinTokenAttestor(JoinTokenAttestorConfig{Store: &fakeJoinTokenStore{}})
	if err == nil || !errors.Is(err, ErrInvalidProvider) {
		t.Fatalf("err = %v", err)
	}
}

func TestJoinTokenAttestor_Type(t *testing.T) {
	t.Parallel()
	a, err := NewJoinTokenAttestor(JoinTokenAttestorConfig{
		Store:       &fakeJoinTokenStore{},
		TrustDomain: DefaultTrustDomain,
	})
	if err != nil {
		t.Fatalf("NewJoinTokenAttestor: %v", err)
	}
	if a.Type() != AttestorTypeJoinToken {
		t.Errorf("Type = %q, want %q", a.Type(), AttestorTypeJoinToken)
	}
}

// ---- Attest happy path ------------------------------------------

func TestJoinTokenAttestor_HappyPath(t *testing.T) {
	t.Parallel()
	token, rec := validTokenAndRecord(t, "agent-happy", time.Hour, 1)
	store := &fakeJoinTokenStore{lookupRecord: rec}
	a, _ := NewJoinTokenAttestor(JoinTokenAttestorConfig{
		Store:       store,
		TrustDomain: DefaultTrustDomain,
	})

	res, err := a.Attest(context.Background(), []byte(token))
	if err != nil {
		t.Fatalf("Attest: %v", err)
	}
	wantID, _ := AgentID(DefaultTrustDomain, "agent-happy")
	if !res.ID.Equal(wantID) {
		t.Errorf("ID = %q, want %q", res.ID, wantID)
	}
	// Selectors are sorted; must include agent + join_token + the
	// metadata key.
	if !slices.Contains(res.Selectors, "agent:agent-happy") {
		t.Errorf("selectors missing agent: %v", res.Selectors)
	}
	if !slices.Contains(res.Selectors, "join_token:"+rec.Prefix) {
		t.Errorf("selectors missing join_token: %v", res.Selectors)
	}
	if !slices.Contains(res.Selectors, "role:web") {
		t.Errorf("selectors missing role:web (metadata): %v", res.Selectors)
	}
	if !slices.IsSorted(res.Selectors) {
		t.Errorf("selectors not sorted: %v", res.Selectors)
	}
	// MarkUsed was called with the record's ID.
	if store.markUsedCalls != 1 {
		t.Errorf("MarkUsed calls = %d, want 1", store.markUsedCalls)
	}
	if store.markUsedIDs[0] != rec.ID {
		t.Errorf("MarkUsed id = %q, want %q", store.markUsedIDs[0], rec.ID)
	}
	// Lookup was called with the right prefix.
	if store.lookupArgs[0] != rec.Prefix {
		t.Errorf("Lookup prefix = %q, want %q", store.lookupArgs[0], rec.Prefix)
	}
}

// ---- format rejections ------------------------------------------

func TestJoinTokenAttestor_RejectsEmptyData(t *testing.T) {
	t.Parallel()
	a, _ := NewJoinTokenAttestor(JoinTokenAttestorConfig{
		Store:       &fakeJoinTokenStore{},
		TrustDomain: DefaultTrustDomain,
	})
	_, err := a.Attest(context.Background(), nil)
	if err == nil || !errors.Is(err, ErrAttestation) {
		t.Errorf("err = %v", err)
	}
}

func TestJoinTokenAttestor_RejectsBadScheme(t *testing.T) {
	t.Parallel()
	a, _ := NewJoinTokenAttestor(JoinTokenAttestorConfig{
		Store:       &fakeJoinTokenStore{},
		TrustDomain: DefaultTrustDomain,
	})
	_, err := a.Attest(context.Background(), []byte("not-kscore-join-abc123"))
	if err == nil || !errors.Is(err, ErrAttestation) {
		t.Errorf("err = %v", err)
	}
	if !strings.Contains(err.Error(), JoinTokenScheme) {
		t.Errorf("err = %v; want scheme cited", err)
	}
}

func TestJoinTokenAttestor_RejectsTooShort(t *testing.T) {
	t.Parallel()
	a, _ := NewJoinTokenAttestor(JoinTokenAttestorConfig{
		Store:       &fakeJoinTokenStore{},
		TrustDomain: DefaultTrustDomain,
	})
	// Scheme + 8 random chars only — that's the prefix length,
	// not the full token. Need scheme + 32+ random chars.
	tooShort := JoinTokenScheme + "ABCDEFGH"
	_, err := a.Attest(context.Background(), []byte(tooShort))
	if err == nil || !errors.Is(err, ErrAttestation) {
		t.Errorf("err = %v", err)
	}
}

// ---- store-side rejections --------------------------------------

func TestJoinTokenAttestor_StoreLookupMiss(t *testing.T) {
	t.Parallel()
	token, _ := validTokenAndRecord(t, "agent-miss", time.Hour, 1)
	store := &fakeJoinTokenStore{lookupErr: ErrJoinTokenNotFound}
	a, _ := NewJoinTokenAttestor(JoinTokenAttestorConfig{
		Store: store, TrustDomain: DefaultTrustDomain,
	})
	_, err := a.Attest(context.Background(), []byte(token))
	if err == nil || !errors.Is(err, ErrAttestation) {
		t.Fatalf("err = %v", err)
	}
	if !errors.Is(err, ErrJoinTokenNotFound) {
		t.Errorf("err = %v; want ErrJoinTokenNotFound chained", err)
	}
	if store.markUsedCalls != 0 {
		t.Errorf("MarkUsed called %d times on missing token; want 0", store.markUsedCalls)
	}
}

func TestJoinTokenAttestor_StoreLookupIOError(t *testing.T) {
	t.Parallel()
	token, _ := validTokenAndRecord(t, "agent-io", time.Hour, 1)
	store := &fakeJoinTokenStore{lookupErr: errors.New("synthetic disk error")}
	a, _ := NewJoinTokenAttestor(JoinTokenAttestorConfig{
		Store: store, TrustDomain: DefaultTrustDomain,
	})
	_, err := a.Attest(context.Background(), []byte(token))
	if err == nil || !errors.Is(err, ErrAttestation) {
		t.Errorf("err = %v", err)
	}
}

func TestJoinTokenAttestor_StoreNilRecord(t *testing.T) {
	t.Parallel()
	token, _ := validTokenAndRecord(t, "agent-nil", time.Hour, 1)
	store := &fakeJoinTokenStore{} // lookupRecord nil, no err
	a, _ := NewJoinTokenAttestor(JoinTokenAttestorConfig{
		Store: store, TrustDomain: DefaultTrustDomain,
	})
	_, err := a.Attest(context.Background(), []byte(token))
	if err == nil || !errors.Is(err, ErrAttestation) {
		t.Errorf("err = %v", err)
	}
}

func TestJoinTokenAttestor_HashMismatch(t *testing.T) {
	t.Parallel()
	// Generate two distinct tokens; use the second's hash with
	// the first's prefix → mismatch.
	tokenA, recA := validTokenAndRecord(t, "agent-a", time.Hour, 1)
	_, recB := validTokenAndRecord(t, "agent-b", time.Hour, 1)
	// Splice: A's prefix + B's hash + B's salt → cleartext A
	// will not hash to B's recorded hash.
	mixed := *recA
	mixed.Hash = recB.Hash
	mixed.Salt = recB.Salt
	store := &fakeJoinTokenStore{lookupRecord: &mixed}
	a, _ := NewJoinTokenAttestor(JoinTokenAttestorConfig{
		Store: store, TrustDomain: DefaultTrustDomain,
	})
	_, err := a.Attest(context.Background(), []byte(tokenA))
	if err == nil || !errors.Is(err, ErrAttestation) {
		t.Errorf("err = %v", err)
	}
	if !strings.Contains(err.Error(), "hash mismatch") {
		t.Errorf("err = %v; want \"hash mismatch\" cited", err)
	}
	if store.markUsedCalls != 0 {
		t.Errorf("MarkUsed called on hash mismatch")
	}
}

func TestJoinTokenAttestor_Expired(t *testing.T) {
	t.Parallel()
	token, rec := validTokenAndRecord(t, "agent-expired", time.Hour, 1)
	// Push ExpiresAt into the past.
	rec.ExpiresAt = time.Now().Add(-time.Hour)
	store := &fakeJoinTokenStore{lookupRecord: rec}
	a, _ := NewJoinTokenAttestor(JoinTokenAttestorConfig{
		Store: store, TrustDomain: DefaultTrustDomain,
	})
	_, err := a.Attest(context.Background(), []byte(token))
	if err == nil || !errors.Is(err, ErrAttestation) {
		t.Fatalf("err = %v", err)
	}
	if !errors.Is(err, ErrJoinTokenExpired) {
		t.Errorf("err = %v; want ErrJoinTokenExpired chained", err)
	}
	if store.markUsedCalls != 0 {
		t.Errorf("MarkUsed called on expired token")
	}
}

func TestJoinTokenAttestor_Exhausted(t *testing.T) {
	t.Parallel()
	token, rec := validTokenAndRecord(t, "agent-exhausted", time.Hour, 1)
	rec.UsedCount = 1 // already at MaxUses
	store := &fakeJoinTokenStore{lookupRecord: rec}
	a, _ := NewJoinTokenAttestor(JoinTokenAttestorConfig{
		Store: store, TrustDomain: DefaultTrustDomain,
	})
	_, err := a.Attest(context.Background(), []byte(token))
	if err == nil || !errors.Is(err, ErrAttestation) {
		t.Fatalf("err = %v", err)
	}
	if !errors.Is(err, ErrJoinTokenExhausted) {
		t.Errorf("err = %v; want ErrJoinTokenExhausted chained", err)
	}
	if store.markUsedCalls != 0 {
		t.Errorf("MarkUsed called on exhausted token")
	}
}

func TestJoinTokenAttestor_EmptyAgentID(t *testing.T) {
	t.Parallel()
	token, rec := validTokenAndRecord(t, "", time.Hour, 1)
	store := &fakeJoinTokenStore{lookupRecord: rec}
	a, _ := NewJoinTokenAttestor(JoinTokenAttestorConfig{
		Store: store, TrustDomain: DefaultTrustDomain,
	})
	_, err := a.Attest(context.Background(), []byte(token))
	if err == nil || !errors.Is(err, ErrAttestation) {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(err.Error(), "any-agent") {
		t.Errorf("err = %v; want \"any-agent\" v0.x note cited", err)
	}
}

func TestJoinTokenAttestor_MarkUsedError(t *testing.T) {
	t.Parallel()
	token, rec := validTokenAndRecord(t, "agent-mu-fail", time.Hour, 1)
	store := &fakeJoinTokenStore{
		lookupRecord: rec,
		markUsedErr:  ErrJoinTokenExhausted, // race with a concurrent caller
	}
	a, _ := NewJoinTokenAttestor(JoinTokenAttestorConfig{
		Store: store, TrustDomain: DefaultTrustDomain,
	})
	_, err := a.Attest(context.Background(), []byte(token))
	if err == nil || !errors.Is(err, ErrAttestation) {
		t.Fatalf("err = %v", err)
	}
	if !errors.Is(err, ErrJoinTokenExhausted) {
		t.Errorf("err = %v; want ErrJoinTokenExhausted chained", err)
	}
}

// ---- clock injection --------------------------------------------

func TestJoinTokenAttestor_ClockInjection(t *testing.T) {
	t.Parallel()
	token, rec := validTokenAndRecord(t, "agent-clock", time.Hour, 1)
	frozen := time.Now().Add(2 * time.Hour) // past ExpiresAt
	store := &fakeJoinTokenStore{lookupRecord: rec}
	a, _ := NewJoinTokenAttestor(JoinTokenAttestorConfig{
		Store:       store,
		TrustDomain: DefaultTrustDomain,
		Clock:       func() time.Time { return frozen },
	})
	_, err := a.Attest(context.Background(), []byte(token))
	if !errors.Is(err, ErrJoinTokenExpired) {
		t.Errorf("err = %v; want ErrJoinTokenExpired", err)
	}
}

// ---- EmbeddedProvider.Attest dispatch ---------------------------

// fakeAttestor is a synthetic Attestor with a configurable Type
// for the dispatch-by-type test.
type fakeAttestor struct {
	typ AttestorType
}

func (f *fakeAttestor) Type() AttestorType { return f.typ }
func (f *fakeAttestor) Attest(_ context.Context, _ []byte) (*AttestResult, error) {
	id, _ := AgentID(DefaultTrustDomain, "fake-"+string(f.typ))
	return &AttestResult{ID: id}, nil
}

func TestEmbeddedProvider_Attest_NoAttestorsConfigured(t *testing.T) {
	t.Parallel()
	p := newStartedProvider(t)
	_, err := p.Attest(context.Background(), AttestRequest{Type: AttestorTypeJoinToken})
	if !errors.Is(err, ErrNotImplementedYet) {
		t.Errorf("err = %v; want ErrNotImplementedYet (no attestors registered)", err)
	}
}

func TestEmbeddedProvider_Attest_DispatchByType(t *testing.T) {
	t.Parallel()
	cfg := newProviderConfig(t)
	cfg.Attestors = []Attestor{
		&fakeAttestor{typ: "type-a"},
		&fakeAttestor{typ: "type-b"},
	}
	p, err := NewEmbeddedProvider(cfg)
	if err != nil {
		t.Fatalf("NewEmbeddedProvider: %v", err)
	}
	if err := p.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = p.Stop(context.Background()) })

	res, err := p.Attest(context.Background(), AttestRequest{Type: "type-a"})
	if err != nil {
		t.Fatalf("Attest type-a: %v", err)
	}
	if !strings.Contains(res.ID.String(), "fake-type-a") {
		t.Errorf("dispatch returned %q, want type-a", res.ID)
	}

	res, err = p.Attest(context.Background(), AttestRequest{Type: "type-b"})
	if err != nil {
		t.Fatalf("Attest type-b: %v", err)
	}
	if !strings.Contains(res.ID.String(), "fake-type-b") {
		t.Errorf("dispatch returned %q, want type-b", res.ID)
	}
}

func TestEmbeddedProvider_Attest_UnknownType(t *testing.T) {
	t.Parallel()
	cfg := newProviderConfig(t)
	cfg.Attestors = []Attestor{&fakeAttestor{typ: "type-a"}}
	p, _ := NewEmbeddedProvider(cfg)
	if err := p.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = p.Stop(context.Background()) })

	_, err := p.Attest(context.Background(), AttestRequest{Type: "unknown"})
	if err == nil || !errors.Is(err, ErrAttestorNotConfigured) {
		t.Errorf("err = %v; want ErrAttestorNotConfigured", err)
	}
}

func TestNewEmbeddedProvider_RejectsDuplicateAttestorTypes(t *testing.T) {
	t.Parallel()
	cfg := newProviderConfig(t)
	cfg.Attestors = []Attestor{
		&fakeAttestor{typ: "dup"},
		&fakeAttestor{typ: "dup"},
	}
	_, err := NewEmbeddedProvider(cfg)
	if err == nil || !errors.Is(err, ErrInvalidProvider) {
		t.Errorf("err = %v", err)
	}
}

func TestNewEmbeddedProvider_RejectsNilAttestor(t *testing.T) {
	t.Parallel()
	cfg := newProviderConfig(t)
	cfg.Attestors = []Attestor{nil}
	_, err := NewEmbeddedProvider(cfg)
	if err == nil || !errors.Is(err, ErrInvalidProvider) {
		t.Errorf("err = %v", err)
	}
}

func TestNewEmbeddedProvider_RejectsEmptyAttestorType(t *testing.T) {
	t.Parallel()
	cfg := newProviderConfig(t)
	cfg.Attestors = []Attestor{&fakeAttestor{typ: ""}}
	_, err := NewEmbeddedProvider(cfg)
	if err == nil || !errors.Is(err, ErrInvalidProvider) {
		t.Errorf("err = %v", err)
	}
}

// ---- EmbeddedProvider + JoinTokenAttestor end-to-end ------------

func TestEmbeddedProvider_WithJoinTokenAttestor_EndToEnd(t *testing.T) {
	t.Parallel()
	token, rec := validTokenAndRecord(t, "agent-e2e", time.Hour, 1)
	store := &fakeJoinTokenStore{lookupRecord: rec}
	att, err := NewJoinTokenAttestor(JoinTokenAttestorConfig{
		Store:       store,
		TrustDomain: DefaultTrustDomain,
	})
	if err != nil {
		t.Fatalf("NewJoinTokenAttestor: %v", err)
	}

	cfg := newProviderConfig(t)
	cfg.Attestors = []Attestor{att}
	p, err := NewEmbeddedProvider(cfg)
	if err != nil {
		t.Fatalf("NewEmbeddedProvider: %v", err)
	}
	if err := p.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = p.Stop(context.Background()) })

	res, err := p.Attest(context.Background(), AttestRequest{
		Type: AttestorTypeJoinToken,
		Data: []byte(token),
	})
	if err != nil {
		t.Fatalf("Attest: %v", err)
	}
	wantID, _ := AgentID(DefaultTrustDomain, "agent-e2e")
	if !res.ID.Equal(wantID) {
		t.Errorf("ID = %q, want %q", res.ID, wantID)
	}
	if store.markUsedCalls != 1 {
		t.Errorf("MarkUsed calls = %d, want 1", store.markUsedCalls)
	}
}
