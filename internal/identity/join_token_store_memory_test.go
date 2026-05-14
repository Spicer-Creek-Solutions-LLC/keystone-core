package identity

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ---- helpers -----------------------------------------------------

func sampleInMemToken(id, prefix string) JoinToken {
	now := time.Now().Truncate(time.Second)
	return JoinToken{
		ID:        id,
		Hash:      []byte("hash-" + id),
		Salt:      []byte("salt-" + id),
		Prefix:    prefix,
		AgentID:   "agent-" + id,
		TTL:       5 * time.Minute,
		CreatedAt: now,
		ExpiresAt: now.Add(5 * time.Minute),
		MaxUses:   1,
		Metadata:  map[string]string{"role": "web"},
	}
}

// ---- Create ------------------------------------------------------

func TestInMemoryJoinTokenStore_CreateGet_RoundTrip(t *testing.T) {
	t.Parallel()
	s := NewInMemoryJoinTokenStore()
	ctx := context.Background()
	tok := sampleInMemToken("k1", "kscore-join-PFXAAAAA")
	tok.Token = "cleartext-leaked" // Create must wipe this

	if err := s.Create(ctx, tok); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := s.Get(ctx, "k1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != "k1" || got.Prefix != "kscore-join-PFXAAAAA" {
		t.Errorf("round-trip mismatch: got %+v", got)
	}
	if got.Token != "" {
		t.Errorf("Token leaked: %q (must be cleared on Create)", got.Token)
	}
	if string(got.Hash) != "hash-k1" {
		t.Errorf("Hash = %q", got.Hash)
	}
}

func TestInMemoryJoinTokenStore_Create_Validation(t *testing.T) {
	t.Parallel()
	s := NewInMemoryJoinTokenStore()
	cases := []struct {
		name string
		mut  func(t *JoinToken)
	}{
		{"empty ID", func(t *JoinToken) { t.ID = "" }},
		{"empty Prefix", func(t *JoinToken) { t.Prefix = "" }},
		{"empty Hash", func(t *JoinToken) { t.Hash = nil }},
		{"empty Salt", func(t *JoinToken) { t.Salt = nil }},
		{"zero ExpiresAt", func(t *JoinToken) { t.ExpiresAt = time.Time{} }},
		{"zero MaxUses", func(t *JoinToken) { t.MaxUses = 0 }},
		{"negative MaxUses", func(t *JoinToken) { t.MaxUses = -1 }},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			tok := sampleInMemToken("v-"+c.name, "kscore-join-V"+c.name[:1]+"123456")
			c.mut(&tok)
			err := s.Create(context.Background(), tok)
			if !errors.Is(err, ErrJoinTokenInvalid) {
				t.Errorf("err = %v, want ErrJoinTokenInvalid", err)
			}
		})
	}
}

func TestInMemoryJoinTokenStore_Create_DuplicateID(t *testing.T) {
	t.Parallel()
	s := NewInMemoryJoinTokenStore()
	ctx := context.Background()
	a := sampleInMemToken("dup", "kscore-join-AAAAAAAA")
	b := sampleInMemToken("dup", "kscore-join-BBBBBBBB") // same ID
	if err := s.Create(ctx, a); err != nil {
		t.Fatalf("Create a: %v", err)
	}
	err := s.Create(ctx, b)
	if !errors.Is(err, ErrJoinTokenDuplicate) {
		t.Errorf("err = %v, want ErrJoinTokenDuplicate", err)
	}
}

func TestInMemoryJoinTokenStore_Create_DuplicatePrefix(t *testing.T) {
	t.Parallel()
	s := NewInMemoryJoinTokenStore()
	ctx := context.Background()
	a := sampleInMemToken("id-a", "kscore-join-SAMEXXXX")
	b := sampleInMemToken("id-b", "kscore-join-SAMEXXXX") // same prefix
	if err := s.Create(ctx, a); err != nil {
		t.Fatalf("Create a: %v", err)
	}
	err := s.Create(ctx, b)
	if !errors.Is(err, ErrJoinTokenDuplicate) {
		t.Errorf("err = %v, want ErrJoinTokenDuplicate", err)
	}
}

// ---- Get / Lookup -----------------------------------------------

func TestInMemoryJoinTokenStore_Get_NotFound(t *testing.T) {
	t.Parallel()
	s := NewInMemoryJoinTokenStore()
	_, err := s.Get(context.Background(), "missing")
	if !errors.Is(err, ErrJoinTokenNotFound) {
		t.Errorf("err = %v, want ErrJoinTokenNotFound", err)
	}
}

func TestInMemoryJoinTokenStore_Lookup_RoundTrip(t *testing.T) {
	t.Parallel()
	s := NewInMemoryJoinTokenStore()
	ctx := context.Background()
	tok := sampleInMemToken("k-lkup", "kscore-join-LKUP0001")
	if err := s.Create(ctx, tok); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := s.Lookup(ctx, "kscore-join-LKUP0001")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if got.ID != "k-lkup" {
		t.Errorf("ID = %q", got.ID)
	}
}

func TestInMemoryJoinTokenStore_Lookup_NotFound(t *testing.T) {
	t.Parallel()
	s := NewInMemoryJoinTokenStore()
	_, err := s.Lookup(context.Background(), "kscore-join-MISSING")
	if !errors.Is(err, ErrJoinTokenNotFound) {
		t.Errorf("err = %v", err)
	}
}

func TestInMemoryJoinTokenStore_Get_DefensiveCopy(t *testing.T) {
	t.Parallel()
	s := NewInMemoryJoinTokenStore()
	ctx := context.Background()
	tok := sampleInMemToken("k-def", "kscore-join-DEF00001")
	if err := s.Create(ctx, tok); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got1, _ := s.Get(ctx, "k-def")
	got1.Metadata["leaked"] = "should-not-appear-in-store"
	got1.Hash[0] = 'X'
	got2, _ := s.Get(ctx, "k-def")
	if _, ok := got2.Metadata["leaked"]; ok {
		t.Error("Metadata not defensive")
	}
	if got2.Hash[0] == 'X' {
		t.Error("Hash not defensive")
	}
}

// ---- List --------------------------------------------------------

func TestInMemoryJoinTokenStore_List(t *testing.T) {
	t.Parallel()
	s := NewInMemoryJoinTokenStore()
	ctx := context.Background()

	now := time.Now().Truncate(time.Second)
	build := func(id, prefix, agent string, mut func(*JoinToken)) JoinToken {
		tok := sampleInMemToken(id, prefix)
		tok.AgentID = agent
		tok.CreatedAt = now
		mut(&tok)
		return tok
	}
	a := build("a", "kscore-join-A0000001", "agent-web", func(t *JoinToken) { t.CreatedAt = now.Add(time.Second) })
	b := build("b", "kscore-join-B0000001", "agent-db", func(t *JoinToken) {
		t.MaxUses = 3
		t.UsedCount = 3 // exhausted
		t.CreatedAt = now.Add(2 * time.Second)
	})
	c := build("c", "kscore-join-C0000001", "agent-web", func(t *JoinToken) {
		t.ExpiresAt = now.Add(-time.Hour) // expired
		t.CreatedAt = now.Add(3 * time.Second)
	})

	for _, tok := range []JoinToken{a, b, c} {
		if err := s.Create(ctx, tok); err != nil {
			t.Fatalf("Create %s: %v", tok.ID, err)
		}
	}

	all, err := s.List(ctx, ListJoinTokensFilter{})
	if err != nil {
		t.Fatalf("List all: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("len(all) = %d, want 3", len(all))
	}
	// Sorted by CreatedAt asc → a, b, c.
	if all[0].ID != "a" || all[1].ID != "b" || all[2].ID != "c" {
		t.Errorf("List order = %s, %s, %s; want a, b, c",
			all[0].ID, all[1].ID, all[2].ID)
	}

	byAgent, _ := s.List(ctx, ListJoinTokensFilter{AgentID: "agent-web"})
	if len(byAgent) != 2 {
		t.Errorf("byAgent len = %d, want 2", len(byAgent))
	}

	unused, _ := s.List(ctx, ListJoinTokensFilter{Unused: true})
	if len(unused) != 2 {
		t.Errorf("unused len = %d, want 2 (a + c)", len(unused))
	}

	unexpired, _ := s.List(ctx, ListJoinTokensFilter{UnexpiredAt: now})
	if len(unexpired) != 2 {
		t.Errorf("unexpired len = %d, want 2 (a + b)", len(unexpired))
	}

	combined, _ := s.List(ctx, ListJoinTokensFilter{
		AgentID:     "agent-web",
		Unused:      true,
		UnexpiredAt: now,
	})
	if len(combined) != 1 || combined[0].ID != "a" {
		t.Errorf("combined = %+v, want [a]", combined)
	}
}

func TestInMemoryJoinTokenStore_List_DefensiveCopy(t *testing.T) {
	t.Parallel()
	s := NewInMemoryJoinTokenStore()
	ctx := context.Background()
	if err := s.Create(ctx, sampleInMemToken("a", "kscore-join-A0000001")); err != nil {
		t.Fatalf("Create: %v", err)
	}
	out, _ := s.List(ctx, ListJoinTokensFilter{})
	out[0].Metadata["leaked"] = "x"
	again, _ := s.List(ctx, ListJoinTokensFilter{})
	if _, ok := again[0].Metadata["leaked"]; ok {
		t.Error("List not defensive")
	}
}

// ---- MarkUsed ----------------------------------------------------

func TestInMemoryJoinTokenStore_MarkUsed(t *testing.T) {
	t.Parallel()
	s := NewInMemoryJoinTokenStore()
	ctx := context.Background()
	tok := sampleInMemToken("mu", "kscore-join-MU000001")
	tok.MaxUses = 3
	if err := s.Create(ctx, tok); err != nil {
		t.Fatalf("Create: %v", err)
	}
	now := time.Now().Truncate(time.Second)
	if err := s.MarkUsed(ctx, "mu", now); err != nil {
		t.Fatalf("MarkUsed: %v", err)
	}
	got, _ := s.Get(ctx, "mu")
	if got.UsedCount != 1 {
		t.Errorf("UsedCount = %d, want 1", got.UsedCount)
	}
	if got.UsedAt == nil || !got.UsedAt.Equal(now) {
		t.Errorf("UsedAt = %v, want %v", got.UsedAt, now)
	}
}

func TestInMemoryJoinTokenStore_MarkUsed_NotFound(t *testing.T) {
	t.Parallel()
	s := NewInMemoryJoinTokenStore()
	err := s.MarkUsed(context.Background(), "missing", time.Now())
	if !errors.Is(err, ErrJoinTokenNotFound) {
		t.Errorf("err = %v", err)
	}
}

func TestInMemoryJoinTokenStore_MarkUsed_Exhausted(t *testing.T) {
	t.Parallel()
	s := NewInMemoryJoinTokenStore()
	ctx := context.Background()
	tok := sampleInMemToken("ex", "kscore-join-EX000001")
	tok.MaxUses = 1
	if err := s.Create(ctx, tok); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := s.MarkUsed(ctx, "ex", time.Now()); err != nil {
		t.Fatalf("MarkUsed 1: %v", err)
	}
	err := s.MarkUsed(ctx, "ex", time.Now())
	if !errors.Is(err, ErrJoinTokenExhausted) {
		t.Errorf("MarkUsed 2 err = %v, want ErrJoinTokenExhausted", err)
	}
}

// Concurrent-correctness — N goroutines × MaxUses=K → exactly K
// succeed. This is the signature test that proves the lock
// discipline is right.
func TestInMemoryJoinTokenStore_MarkUsed_ConcurrentMaxUses(t *testing.T) {
	t.Parallel()
	s := NewInMemoryJoinTokenStore()
	ctx := context.Background()
	const N = 50
	const K = 7
	tok := sampleInMemToken("race", "kscore-join-RACE0001")
	tok.MaxUses = K
	if err := s.Create(ctx, tok); err != nil {
		t.Fatalf("Create: %v", err)
	}

	var succeeded atomic.Int64
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			if err := s.MarkUsed(ctx, "race", time.Now()); err == nil {
				succeeded.Add(1)
			}
		}()
	}
	wg.Wait()

	if got := int(succeeded.Load()); got != K {
		t.Errorf("succeeded = %d, want exactly %d", got, K)
	}
	got, _ := s.Get(ctx, "race")
	if got.UsedCount != K {
		t.Errorf("UsedCount = %d, want %d", got.UsedCount, K)
	}
}

// ---- Delete ------------------------------------------------------

func TestInMemoryJoinTokenStore_Delete(t *testing.T) {
	t.Parallel()
	s := NewInMemoryJoinTokenStore()
	ctx := context.Background()
	tok := sampleInMemToken("d", "kscore-join-D0000001")
	if err := s.Create(ctx, tok); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := s.Delete(ctx, "d"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Get(ctx, "d"); !errors.Is(err, ErrJoinTokenNotFound) {
		t.Errorf("post-delete Get err = %v", err)
	}
	if _, err := s.Lookup(ctx, "kscore-join-D0000001"); !errors.Is(err, ErrJoinTokenNotFound) {
		t.Errorf("post-delete Lookup: prefix index not cleared, err = %v", err)
	}
}

func TestInMemoryJoinTokenStore_Delete_Missing(t *testing.T) {
	t.Parallel()
	s := NewInMemoryJoinTokenStore()
	err := s.Delete(context.Background(), "no-such-id")
	if !errors.Is(err, ErrJoinTokenNotFound) {
		t.Errorf("err = %v", err)
	}
}

// ---- Cleanup -----------------------------------------------------

func TestInMemoryJoinTokenStore_Cleanup(t *testing.T) {
	t.Parallel()
	s := NewInMemoryJoinTokenStore()
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	expired := sampleInMemToken("expired", "kscore-join-EXP00001")
	expired.ExpiresAt = now.Add(-time.Hour)
	live := sampleInMemToken("live", "kscore-join-LIVE0001")
	live.ExpiresAt = now.Add(time.Hour)

	for _, tok := range []JoinToken{expired, live} {
		if err := s.Create(ctx, tok); err != nil {
			t.Fatalf("Create %s: %v", tok.ID, err)
		}
	}

	n, err := s.Cleanup(ctx, now)
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if n != 1 {
		t.Errorf("n = %d, want 1", n)
	}
	if _, err := s.Get(ctx, "expired"); !errors.Is(err, ErrJoinTokenNotFound) {
		t.Errorf("expired still present: %v", err)
	}
	if _, err := s.Get(ctx, "live"); err != nil {
		t.Errorf("live removed: %v", err)
	}
	// Prefix index also cleared.
	if _, err := s.Lookup(ctx, "kscore-join-EXP00001"); !errors.Is(err, ErrJoinTokenNotFound) {
		t.Errorf("Cleanup didn't clear prefix index: %v", err)
	}
}

func TestInMemoryJoinTokenStore_Cleanup_Empty(t *testing.T) {
	t.Parallel()
	s := NewInMemoryJoinTokenStore()
	n, err := s.Cleanup(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if n != 0 {
		t.Errorf("n = %d, want 0", n)
	}
}

// ---- End-to-end with JoinTokenAttestor --------------------------

func TestInMemoryJoinTokenStore_WithAttestor_RoundTrip(t *testing.T) {
	t.Parallel()
	s := NewInMemoryJoinTokenStore()
	ctx := context.Background()

	// Build a real cleartext token + matching hash.
	token, rec := validTokenAndRecord(t, "agent-e2e", time.Hour, 1)
	// rec.UsedAt is nil-by-default in validTokenAndRecord (returns
	// *JoinToken). Use it directly.
	if err := s.Create(ctx, *rec); err != nil {
		t.Fatalf("Create: %v", err)
	}

	att, err := NewJoinTokenAttestor(JoinTokenAttestorConfig{
		Store:       s,
		TrustDomain: DefaultTrustDomain,
	})
	if err != nil {
		t.Fatalf("NewJoinTokenAttestor: %v", err)
	}

	res, err := att.Attest(ctx, []byte(token))
	if err != nil {
		t.Fatalf("Attest: %v", err)
	}
	wantID, _ := AgentID(DefaultTrustDomain, "agent-e2e")
	if !res.ID.Equal(wantID) {
		t.Errorf("ID = %q, want %q", res.ID, wantID)
	}

	// Second attempt — token is exhausted (MaxUses=1).
	_, err = att.Attest(ctx, []byte(token))
	if !errors.Is(err, ErrJoinTokenExhausted) {
		t.Errorf("second Attest err = %v, want ErrJoinTokenExhausted", err)
	}
}
