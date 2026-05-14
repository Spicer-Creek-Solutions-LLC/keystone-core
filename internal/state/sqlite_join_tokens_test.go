package state

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func sampleJoinToken(id, prefix string) *JoinTokenRecord {
	now := time.Now().UTC().Truncate(time.Second)
	return &JoinTokenRecord{
		ID:        id,
		Hash:      []byte("hash-of-token-" + id),
		Salt:      []byte("salt-" + id),
		Prefix:    prefix,
		AgentID:   "agent-" + id,
		TTL:       5 * time.Minute,
		CreatedAt: now,
		ExpiresAt: now.Add(5 * time.Minute),
		MaxUses:   1,
		Metadata:  map[string]string{"role": "web", "env": "prod"},
	}
}

// ---- Create + Get round-trip ------------------------------------

func TestSQLiteStore_CreateJoinToken_RoundTrip(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := context.Background()

	in := sampleJoinToken("k-1", "kscore-join-AAAAAAAA")
	if err := s.CreateJoinToken(ctx, in); err != nil {
		t.Fatalf("CreateJoinToken: %v", err)
	}
	got, err := s.GetJoinToken(ctx, "k-1")
	if err != nil {
		t.Fatalf("GetJoinToken: %v", err)
	}
	if got.ID != in.ID || got.Prefix != in.Prefix || got.AgentID != in.AgentID {
		t.Errorf("round-trip mismatch: got %+v", got)
	}
	if string(got.Hash) != string(in.Hash) {
		t.Errorf("Hash round-trip: got %q", got.Hash)
	}
	if string(got.Salt) != string(in.Salt) {
		t.Errorf("Salt round-trip: got %q", got.Salt)
	}
	if got.MaxUses != in.MaxUses {
		t.Errorf("MaxUses = %d, want %d", got.MaxUses, in.MaxUses)
	}
	if got.UsedCount != 0 {
		t.Errorf("UsedCount = %d, want 0", got.UsedCount)
	}
	if !got.UsedAt.IsZero() {
		t.Errorf("UsedAt = %s, want zero", got.UsedAt)
	}
	if got.Metadata["role"] != "web" || got.Metadata["env"] != "prod" {
		t.Errorf("Metadata = %v", got.Metadata)
	}
}

// ---- Create validation ------------------------------------------

func TestSQLiteStore_CreateJoinToken_RejectsNil(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	if err := s.CreateJoinToken(context.Background(), nil); err == nil {
		t.Error("nil record accepted")
	}
}

func TestSQLiteStore_CreateJoinToken_RejectsMissingFields(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := context.Background()
	cases := []struct {
		name string
		mut  func(r *JoinTokenRecord)
	}{
		{"empty ID", func(r *JoinTokenRecord) { r.ID = "" }},
		{"empty Prefix", func(r *JoinTokenRecord) { r.Prefix = "" }},
		{"empty Hash", func(r *JoinTokenRecord) { r.Hash = nil }},
		{"empty Salt", func(r *JoinTokenRecord) { r.Salt = nil }},
		{"zero ExpiresAt", func(r *JoinTokenRecord) { r.ExpiresAt = time.Time{} }},
		{"zero MaxUses", func(r *JoinTokenRecord) { r.MaxUses = 0 }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := sampleJoinToken("k-bad", "kscore-join-BAD1234567")
			c.mut(r)
			if err := s.CreateJoinToken(ctx, r); err == nil {
				t.Errorf("%s: accepted", c.name)
			}
		})
	}
}

func TestSQLiteStore_CreateJoinToken_DuplicateID(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := context.Background()
	a := sampleJoinToken("dup-id", "kscore-join-PREFIXAA")
	if err := s.CreateJoinToken(ctx, a); err != nil {
		t.Fatalf("Create a: %v", err)
	}
	b := sampleJoinToken("dup-id", "kscore-join-PREFIXBB") // same ID, different prefix
	err := s.CreateJoinToken(ctx, b)
	if err == nil || !errors.Is(err, ErrDuplicate) {
		t.Errorf("err = %v, want ErrDuplicate", err)
	}
}

func TestSQLiteStore_CreateJoinToken_DuplicatePrefix(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := context.Background()
	a := sampleJoinToken("id-a", "kscore-join-SAMEPRE1")
	if err := s.CreateJoinToken(ctx, a); err != nil {
		t.Fatalf("Create a: %v", err)
	}
	b := sampleJoinToken("id-b", "kscore-join-SAMEPRE1") // different ID, same prefix
	err := s.CreateJoinToken(ctx, b)
	if err == nil || !errors.Is(err, ErrDuplicate) {
		t.Errorf("err = %v, want ErrDuplicate", err)
	}
}

// ---- Get / Lookup missing ---------------------------------------

func TestSQLiteStore_GetJoinToken_NotFound(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	_, err := s.GetJoinToken(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestSQLiteStore_LookupJoinTokenByPrefix(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := context.Background()
	in := sampleJoinToken("lkup-1", "kscore-join-LOOKUP01")
	if err := s.CreateJoinToken(ctx, in); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := s.LookupJoinTokenByPrefix(ctx, "kscore-join-LOOKUP01")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if got.ID != "lkup-1" {
		t.Errorf("ID = %q, want lkup-1", got.ID)
	}
}

func TestSQLiteStore_LookupJoinTokenByPrefix_NotFound(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	_, err := s.LookupJoinTokenByPrefix(context.Background(), "kscore-join-MISSING0")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

// ---- List filters ------------------------------------------------

func TestSQLiteStore_ListJoinTokens_Filters(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := context.Background()

	a := sampleJoinToken("a", "kscore-join-AAAAAAAA")
	a.AgentID = "agent-web"
	b := sampleJoinToken("b", "kscore-join-BBBBBBBB")
	b.AgentID = "agent-db"
	b.MaxUses = 5
	b.UsedCount = 5 // exhausted
	c := sampleJoinToken("c", "kscore-join-CCCCCCCC")
	c.AgentID = "agent-web"
	c.ExpiresAt = time.Now().Add(-time.Hour) // already expired

	for _, r := range []*JoinTokenRecord{a, b, c} {
		if err := s.CreateJoinToken(ctx, r); err != nil {
			t.Fatalf("Create %s: %v", r.ID, err)
		}
	}

	// All.
	all, err := s.ListJoinTokens(ctx, JoinTokenFilter{})
	if err != nil {
		t.Fatalf("List all: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("List all len = %d, want 3", len(all))
	}

	// By AgentID.
	byAgent, _ := s.ListJoinTokens(ctx, JoinTokenFilter{AgentID: "agent-web"})
	if len(byAgent) != 2 {
		t.Errorf("by agent len = %d, want 2", len(byAgent))
	}

	// Unused only (b is exhausted).
	unused, _ := s.ListJoinTokens(ctx, JoinTokenFilter{Unused: true})
	if len(unused) != 2 {
		t.Errorf("unused len = %d, want 2 (a + c)", len(unused))
	}
	for _, r := range unused {
		if r.ID == "b" {
			t.Errorf("unused returned exhausted token b")
		}
	}

	// UnexpiredAt (c is already expired).
	unexpired, _ := s.ListJoinTokens(ctx, JoinTokenFilter{UnexpiredAt: time.Now()})
	if len(unexpired) != 2 {
		t.Errorf("unexpired len = %d, want 2", len(unexpired))
	}
	for _, r := range unexpired {
		if r.ID == "c" {
			t.Errorf("unexpired returned expired token c")
		}
	}

	// Combined: agent-web + unused + unexpired → only a.
	combined, _ := s.ListJoinTokens(ctx, JoinTokenFilter{
		AgentID:     "agent-web",
		Unused:      true,
		UnexpiredAt: time.Now(),
	})
	if len(combined) != 1 || combined[0].ID != "a" {
		t.Errorf("combined = %d records, IDs %v, want [a]", len(combined),
			func() []string { ids := []string{}; for _, r := range combined { ids = append(ids, r.ID) }; return ids }())
	}
}

func TestSQLiteStore_ListJoinTokens_BadSortColumn(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	_, err := s.ListJoinTokens(context.Background(), JoinTokenFilter{SortColumn: "DROP TABLE"})
	if err == nil {
		t.Error("bad sort column accepted")
	}
}

// ---- MarkUsed ---------------------------------------------------

func TestSQLiteStore_MarkJoinTokenUsed(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := context.Background()
	r := sampleJoinToken("mu-1", "kscore-join-USE00001")
	r.MaxUses = 3
	if err := s.CreateJoinToken(ctx, r); err != nil {
		t.Fatalf("Create: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.MarkJoinTokenUsed(ctx, "mu-1", now); err != nil {
		t.Fatalf("MarkUsed: %v", err)
	}
	got, _ := s.GetJoinToken(ctx, "mu-1")
	if got.UsedCount != 1 {
		t.Errorf("UsedCount = %d, want 1", got.UsedCount)
	}
	if !got.UsedAt.Equal(now) {
		t.Errorf("UsedAt = %s, want %s", got.UsedAt, now)
	}
}

func TestSQLiteStore_MarkJoinTokenUsed_NotFound(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	err := s.MarkJoinTokenUsed(context.Background(), "missing", time.Now())
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestSQLiteStore_MarkJoinTokenUsed_Exhausted(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := context.Background()
	r := sampleJoinToken("exhausted", "kscore-join-EXHAUST1")
	r.MaxUses = 1
	r.UsedCount = 1
	if err := s.CreateJoinToken(ctx, r); err != nil {
		t.Fatalf("Create: %v", err)
	}
	// UPDATE's `used_count < max_uses` clause means a no-op →
	// ErrNotFound (matches affectsRow semantics).
	err := s.MarkJoinTokenUsed(ctx, "exhausted", time.Now())
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound (exhausted token sees 0 rows updated)", err)
	}
}

// Race correctness — N goroutines, MaxUses=K, exactly K must succeed.
func TestSQLiteStore_MarkJoinTokenUsed_ConcurrentMaxUses(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := context.Background()
	const N = 20
	const K = 5
	r := sampleJoinToken("race", "kscore-join-RACE0001")
	r.MaxUses = K
	if err := s.CreateJoinToken(ctx, r); err != nil {
		t.Fatalf("Create: %v", err)
	}

	var succeeded atomic.Int64
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			if err := s.MarkJoinTokenUsed(ctx, "race", time.Now()); err == nil {
				succeeded.Add(1)
			}
		}()
	}
	wg.Wait()

	if got := int(succeeded.Load()); got != K {
		t.Errorf("succeeded = %d, want exactly %d", got, K)
	}
	got, _ := s.GetJoinToken(ctx, "race")
	if got.UsedCount != K {
		t.Errorf("final UsedCount = %d, want %d", got.UsedCount, K)
	}
}

// ---- Delete -----------------------------------------------------

func TestSQLiteStore_DeleteJoinToken(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := context.Background()
	r := sampleJoinToken("del-1", "kscore-join-DELETE01")
	if err := s.CreateJoinToken(ctx, r); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := s.DeleteJoinToken(ctx, "del-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.GetJoinToken(ctx, "del-1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("post-delete Get err = %v, want ErrNotFound", err)
	}
}

func TestSQLiteStore_DeleteJoinToken_Missing(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	err := s.DeleteJoinToken(context.Background(), "no-such-token")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

// ---- DeleteExpired ----------------------------------------------

func TestSQLiteStore_DeleteExpiredJoinTokens(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := context.Background()
	now := time.Now().UTC()

	expired := sampleJoinToken("expired", "kscore-join-EXPIRED1")
	expired.ExpiresAt = now.Add(-time.Hour)
	live := sampleJoinToken("live", "kscore-join-LIVE0001")
	live.ExpiresAt = now.Add(time.Hour)

	for _, r := range []*JoinTokenRecord{expired, live} {
		if err := s.CreateJoinToken(ctx, r); err != nil {
			t.Fatalf("Create %s: %v", r.ID, err)
		}
	}

	n, err := s.DeleteExpiredJoinTokens(ctx, now)
	if err != nil {
		t.Fatalf("DeleteExpiredJoinTokens: %v", err)
	}
	if n != 1 {
		t.Errorf("deleted = %d, want 1", n)
	}
	if _, err := s.GetJoinToken(ctx, "expired"); !errors.Is(err, ErrNotFound) {
		t.Errorf("expired still present: %v", err)
	}
	if _, err := s.GetJoinToken(ctx, "live"); err != nil {
		t.Errorf("live removed: %v", err)
	}
}

func TestSQLiteStore_DeleteExpiredJoinTokens_Empty(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	n, err := s.DeleteExpiredJoinTokens(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if n != 0 {
		t.Errorf("n = %d, want 0", n)
	}
}
