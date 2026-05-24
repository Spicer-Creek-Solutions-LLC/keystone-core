// SPDX-License-Identifier: Apache-2.0

package identity

import (
	"context"
	"crypto/sha256"
	"errors"
	"strings"
	"testing"
	"time"
)

// ---- helpers -----------------------------------------------------

// newStartedProviderWithTokens spins up a provider with an
// in-memory JoinTokenStore wired in. The store is returned so
// tests can inspect persisted state directly.
func newStartedProviderWithTokens(t *testing.T) (*EmbeddedProvider, *InMemoryJoinTokenStore) {
	t.Helper()
	store := NewInMemoryJoinTokenStore()
	cfg := newProviderConfig(t)
	cfg.JoinTokenStore = store
	p, err := NewEmbeddedProvider(cfg)
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
	return p, store
}

// ---- CreateJoinToken --------------------------------------------

func TestCreateJoinToken_HappyPath(t *testing.T) {
	t.Parallel()
	p, store := newStartedProviderWithTokens(t)
	before := time.Now()
	got, err := p.CreateJoinToken(context.Background(), CreateJoinTokenRequest{
		AgentID: "agent-create",
	})
	if err != nil {
		t.Fatalf("CreateJoinToken: %v", err)
	}
	after := time.Now()

	if !strings.HasPrefix(got.Token, JoinTokenScheme) {
		t.Errorf("Token = %q, want %s… prefix", got.Token, JoinTokenScheme)
	}
	wantBodyLen := joinTokenBodyLen
	if got, want := len(got.Token)-len(JoinTokenScheme), wantBodyLen; got != want {
		t.Errorf("Token body len = %d, want %d", got, want)
	}
	if got, want := got.Prefix, got.Token[:JoinTokenPrefixLen]; got != want {
		t.Errorf("Prefix = %q, want %q (= first %d chars of Token)", got, want, JoinTokenPrefixLen)
	}
	if got.ID == "" {
		t.Error("ID empty")
	}
	if len(got.Hash) != sha256.Size {
		t.Errorf("Hash len = %d, want %d", len(got.Hash), sha256.Size)
	}
	if len(got.Salt) != joinTokenSaltLen {
		t.Errorf("Salt len = %d, want %d", len(got.Salt), joinTokenSaltLen)
	}
	if got.AgentID != "agent-create" {
		t.Errorf("AgentID = %q", got.AgentID)
	}
	if got.MaxUses != DefaultJoinTokenMaxUses {
		t.Errorf("MaxUses = %d, want %d", got.MaxUses, DefaultJoinTokenMaxUses)
	}
	if got.TTL != DefaultJoinTokenTTL {
		t.Errorf("TTL = %s, want %s", got.TTL, DefaultJoinTokenTTL)
	}
	if got.CreatedAt.Before(before.Add(-time.Second)) || got.CreatedAt.After(after.Add(time.Second)) {
		t.Errorf("CreatedAt = %s, not within window [%s, %s]", got.CreatedAt, before, after)
	}
	if exp := got.CreatedAt.Add(DefaultJoinTokenTTL); !got.ExpiresAt.Equal(exp) {
		t.Errorf("ExpiresAt = %s, want %s", got.ExpiresAt, exp)
	}
	if got.UsedCount != 0 {
		t.Errorf("UsedCount = %d, want 0", got.UsedCount)
	}
	if got.UsedAt != nil {
		t.Errorf("UsedAt = %v, want nil", got.UsedAt)
	}

	// Hash consistency: sha256(salt || token) == got.Hash.
	gotHash := saltedHash(got.Salt, got.Token)
	if string(gotHash) != string(got.Hash) {
		t.Error("Hash does not match saltedHash(Salt, Token) — generation is inconsistent")
	}

	// Persisted record has Token wiped.
	persisted, err := store.Get(context.Background(), got.ID)
	if err != nil {
		t.Fatalf("store.Get: %v", err)
	}
	if persisted.Token != "" {
		t.Errorf("persisted Token = %q, want \"\" (store must wipe cleartext)", persisted.Token)
	}
	if persisted.Prefix != got.Prefix {
		t.Errorf("persisted Prefix mismatch")
	}
}

func TestCreateJoinToken_OverrideTTLAndMaxUses(t *testing.T) {
	t.Parallel()
	p, _ := newStartedProviderWithTokens(t)
	got, err := p.CreateJoinToken(context.Background(), CreateJoinTokenRequest{
		TTL:     2 * time.Hour,
		MaxUses: 5,
		AgentID: "agent-override",
	})
	if err != nil {
		t.Fatalf("CreateJoinToken: %v", err)
	}
	if got.TTL != 2*time.Hour {
		t.Errorf("TTL = %s, want 2h", got.TTL)
	}
	if got.MaxUses != 5 {
		t.Errorf("MaxUses = %d, want 5", got.MaxUses)
	}
	if want := got.CreatedAt.Add(2 * time.Hour); !got.ExpiresAt.Equal(want) {
		t.Errorf("ExpiresAt = %s, want %s", got.ExpiresAt, want)
	}
}

func TestCreateJoinToken_RejectsTTLOverMax(t *testing.T) {
	t.Parallel()
	p, _ := newStartedProviderWithTokens(t)
	_, err := p.CreateJoinToken(context.Background(), CreateJoinTokenRequest{
		TTL:     MaxJoinTokenTTL + time.Hour,
		AgentID: "agent-too-long",
	})
	if err == nil || !errors.Is(err, ErrInvalidProvider) {
		t.Fatalf("err = %v, want ErrInvalidProvider", err)
	}
	if !strings.Contains(err.Error(), "exceeds max") {
		t.Errorf("err message = %v, want \"exceeds max\" mention", err)
	}
}

func TestCreateJoinToken_MetadataPropagates(t *testing.T) {
	t.Parallel()
	p, store := newStartedProviderWithTokens(t)
	md := map[string]string{"role": "web", "env": "prod"}
	got, err := p.CreateJoinToken(context.Background(), CreateJoinTokenRequest{
		AgentID:  "agent-md",
		Metadata: md,
	})
	if err != nil {
		t.Fatalf("CreateJoinToken: %v", err)
	}
	if got.Metadata["role"] != "web" || got.Metadata["env"] != "prod" {
		t.Errorf("returned Metadata = %v", got.Metadata)
	}
	persisted, _ := store.Get(context.Background(), got.ID)
	if persisted.Metadata["role"] != "web" || persisted.Metadata["env"] != "prod" {
		t.Errorf("persisted Metadata = %v", persisted.Metadata)
	}
}

func TestCreateJoinToken_DistinctTokens(t *testing.T) {
	t.Parallel()
	p, _ := newStartedProviderWithTokens(t)
	a, err := p.CreateJoinToken(context.Background(), CreateJoinTokenRequest{AgentID: "a"})
	if err != nil {
		t.Fatalf("Create a: %v", err)
	}
	b, err := p.CreateJoinToken(context.Background(), CreateJoinTokenRequest{AgentID: "b"})
	if err != nil {
		t.Fatalf("Create b: %v", err)
	}
	if a.ID == b.ID {
		t.Error("two creates produced same ID")
	}
	if a.Token == b.Token {
		t.Error("two creates produced same Token")
	}
	if a.Prefix == b.Prefix {
		t.Error("two creates produced same Prefix")
	}
	if string(a.Salt) == string(b.Salt) {
		t.Error("two creates produced same Salt")
	}
}

func TestCreateJoinToken_BeforeStart(t *testing.T) {
	t.Parallel()
	cfg := newProviderConfig(t)
	cfg.JoinTokenStore = NewInMemoryJoinTokenStore()
	p, _ := NewEmbeddedProvider(cfg)
	_, err := p.CreateJoinToken(context.Background(), CreateJoinTokenRequest{AgentID: "a"})
	if !errors.Is(err, ErrProviderNotRunning) {
		t.Errorf("err = %v, want ErrProviderNotRunning", err)
	}
}

func TestCreateJoinToken_NoStoreConfigured(t *testing.T) {
	t.Parallel()
	p := newStartedProvider(t) // no JoinTokenStore in this fixture
	_, err := p.CreateJoinToken(context.Background(), CreateJoinTokenRequest{AgentID: "a"})
	if !errors.Is(err, ErrJoinTokenStoreNotConfigured) {
		t.Errorf("err = %v, want ErrJoinTokenStoreNotConfigured", err)
	}
}

// ---- ListJoinTokens ---------------------------------------------

func TestListJoinTokens_RoundTrip(t *testing.T) {
	t.Parallel()
	p, _ := newStartedProviderWithTokens(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if _, err := p.CreateJoinToken(ctx, CreateJoinTokenRequest{AgentID: "agent-list"}); err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
	}
	got, err := p.ListJoinTokens(ctx, ListJoinTokensFilter{})
	if err != nil {
		t.Fatalf("ListJoinTokens: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("List len = %d, want 3", len(got))
	}
	for _, tok := range got {
		if tok.Token != "" {
			t.Errorf("listed token has cleartext Token = %q", tok.Token)
		}
	}
}

func TestListJoinTokens_FilterByAgent(t *testing.T) {
	t.Parallel()
	p, _ := newStartedProviderWithTokens(t)
	ctx := context.Background()
	if _, err := p.CreateJoinToken(ctx, CreateJoinTokenRequest{AgentID: "alpha"}); err != nil {
		t.Fatalf("Create alpha: %v", err)
	}
	if _, err := p.CreateJoinToken(ctx, CreateJoinTokenRequest{AgentID: "beta"}); err != nil {
		t.Fatalf("Create beta: %v", err)
	}
	got, err := p.ListJoinTokens(ctx, ListJoinTokensFilter{AgentID: "alpha"})
	if err != nil {
		t.Fatalf("ListJoinTokens: %v", err)
	}
	if len(got) != 1 || got[0].AgentID != "alpha" {
		t.Errorf("filtered list = %v", got)
	}
}

func TestListJoinTokens_Empty(t *testing.T) {
	t.Parallel()
	p, _ := newStartedProviderWithTokens(t)
	got, err := p.ListJoinTokens(context.Background(), ListJoinTokensFilter{})
	if err != nil {
		t.Fatalf("ListJoinTokens: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("empty-store List len = %d, want 0", len(got))
	}
}

func TestListJoinTokens_BeforeStart(t *testing.T) {
	t.Parallel()
	cfg := newProviderConfig(t)
	cfg.JoinTokenStore = NewInMemoryJoinTokenStore()
	p, _ := NewEmbeddedProvider(cfg)
	_, err := p.ListJoinTokens(context.Background(), ListJoinTokensFilter{})
	if !errors.Is(err, ErrProviderNotRunning) {
		t.Errorf("err = %v", err)
	}
}

func TestListJoinTokens_NoStoreConfigured(t *testing.T) {
	t.Parallel()
	p := newStartedProvider(t)
	_, err := p.ListJoinTokens(context.Background(), ListJoinTokensFilter{})
	if !errors.Is(err, ErrJoinTokenStoreNotConfigured) {
		t.Errorf("err = %v", err)
	}
}

// ---- DeleteJoinToken --------------------------------------------

func TestDeleteJoinToken_HappyPath(t *testing.T) {
	t.Parallel()
	p, store := newStartedProviderWithTokens(t)
	ctx := context.Background()
	tok, err := p.CreateJoinToken(ctx, CreateJoinTokenRequest{AgentID: "agent-del"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := p.DeleteJoinToken(ctx, tok.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := store.Get(ctx, tok.ID); !errors.Is(err, ErrJoinTokenNotFound) {
		t.Errorf("post-delete Get: %v", err)
	}
}

func TestDeleteJoinToken_PropagatesNotFound(t *testing.T) {
	t.Parallel()
	p, _ := newStartedProviderWithTokens(t)
	err := p.DeleteJoinToken(context.Background(), "no-such-id")
	if !errors.Is(err, ErrJoinTokenNotFound) {
		t.Errorf("err = %v, want ErrJoinTokenNotFound", err)
	}
}

func TestDeleteJoinToken_BeforeStart(t *testing.T) {
	t.Parallel()
	cfg := newProviderConfig(t)
	cfg.JoinTokenStore = NewInMemoryJoinTokenStore()
	p, _ := NewEmbeddedProvider(cfg)
	err := p.DeleteJoinToken(context.Background(), "any-id")
	if !errors.Is(err, ErrProviderNotRunning) {
		t.Errorf("err = %v", err)
	}
}

func TestDeleteJoinToken_NoStoreConfigured(t *testing.T) {
	t.Parallel()
	p := newStartedProvider(t)
	err := p.DeleteJoinToken(context.Background(), "any-id")
	if !errors.Is(err, ErrJoinTokenStoreNotConfigured) {
		t.Errorf("err = %v", err)
	}
}

// ---- End-to-end: Create → Attest → token spent ------------------

// The integration test that demonstrates the full provider
// lifecycle from operator's "create a token for agent X" to the
// agent presenting it and the store recording the use.
func TestEmbeddedProvider_CreateThenAttest_EndToEnd(t *testing.T) {
	t.Parallel()
	store := NewInMemoryJoinTokenStore()
	att, err := NewJoinTokenAttestor(JoinTokenAttestorConfig{
		Store:       store,
		TrustDomain: DefaultTrustDomain,
	})
	if err != nil {
		t.Fatalf("NewJoinTokenAttestor: %v", err)
	}
	cfg := newProviderConfig(t)
	cfg.JoinTokenStore = store
	cfg.Attestors = []Attestor{att}
	p, err := NewEmbeddedProvider(cfg)
	if err != nil {
		t.Fatalf("NewEmbeddedProvider: %v", err)
	}
	if err := p.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = p.Stop(context.Background()) })

	ctx := context.Background()
	created, err := p.CreateJoinToken(ctx, CreateJoinTokenRequest{
		AgentID:  "agent-e2e-10",
		Metadata: map[string]string{"role": "web"},
	})
	if err != nil {
		t.Fatalf("CreateJoinToken: %v", err)
	}
	if created.Token == "" {
		t.Fatal("created.Token empty — operator can't hand cleartext to the agent")
	}

	// Agent presents the cleartext token for attestation.
	res, err := p.Attest(ctx, AttestRequest{
		Type: AttestorTypeJoinToken,
		Data: []byte(created.Token),
	})
	if err != nil {
		t.Fatalf("Attest: %v", err)
	}
	wantID, _ := AgentID(DefaultTrustDomain, "agent-e2e-10")
	if !res.ID.Equal(wantID) {
		t.Errorf("attested ID = %q, want %q", res.ID, wantID)
	}

	// Token is now spent (MaxUses defaults to 1).
	persisted, err := store.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("store.Get post-attest: %v", err)
	}
	if persisted.UsedCount != 1 {
		t.Errorf("UsedCount = %d, want 1 after attestation", persisted.UsedCount)
	}
	if persisted.UsedAt == nil {
		t.Error("UsedAt nil after attestation")
	}

	// Second attestation fails — exhausted.
	_, err = p.Attest(ctx, AttestRequest{
		Type: AttestorTypeJoinToken,
		Data: []byte(created.Token),
	})
	if !errors.Is(err, ErrJoinTokenExhausted) {
		t.Errorf("second Attest err = %v, want ErrJoinTokenExhausted", err)
	}
}
