package identity

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/state"
)

// stateSQLiteStoreForTest spins up an in-memory SQLite-backed
// state.Store for the adapter tests. Mirrors the
// newSQLiteStoreForTest helper inside the state package's tests,
// but available here through the public state.NewStore surface.
func stateSQLiteStoreForTest(t *testing.T) state.Store {
	t.Helper()
	store, err := state.NewStore(&state.Config{
		Backend: state.BackendSQLite,
		SQLite:  state.SQLiteConfig{Path: ":memory:"},
	})
	if err != nil {
		t.Fatalf("state.NewStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

// ---- Constructor -------------------------------------------------

func TestNewStateJoinTokenStore_RejectsNil(t *testing.T) {
	t.Parallel()
	_, err := NewStateJoinTokenStore(nil)
	if err == nil || !errors.Is(err, ErrInvalidProvider) {
		t.Errorf("err = %v", err)
	}
}

// ---- CRUD round-trip --------------------------------------------

func TestStateJoinTokenStore_CreateGetRoundTrip(t *testing.T) {
	t.Parallel()
	store := stateSQLiteStoreForTest(t)
	a, err := NewStateJoinTokenStore(store)
	if err != nil {
		t.Fatalf("NewStateJoinTokenStore: %v", err)
	}
	ctx := context.Background()

	tok := sampleInMemToken("s-1", "kscore-join-STATE001")
	tok.Token = "cleartext-leaked"

	if err := a.Create(ctx, tok); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := a.Get(ctx, "s-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != "s-1" || got.Prefix != "kscore-join-STATE001" {
		t.Errorf("round-trip: %+v", got)
	}
	if got.Token != "" {
		t.Errorf("Token leaked: %q", got.Token)
	}
}

// ---- Duplicate detection ----------------------------------------

func TestStateJoinTokenStore_Create_DuplicateID(t *testing.T) {
	t.Parallel()
	store := stateSQLiteStoreForTest(t)
	a, _ := NewStateJoinTokenStore(store)
	ctx := context.Background()
	x := sampleInMemToken("dup", "kscore-join-PFX1AAAA")
	y := sampleInMemToken("dup", "kscore-join-PFX2BBBB") // same id
	if err := a.Create(ctx, x); err != nil {
		t.Fatalf("Create x: %v", err)
	}
	err := a.Create(ctx, y)
	if !errors.Is(err, ErrJoinTokenDuplicate) {
		t.Errorf("err = %v, want ErrJoinTokenDuplicate", err)
	}
}

func TestStateJoinTokenStore_Create_DuplicatePrefix(t *testing.T) {
	t.Parallel()
	store := stateSQLiteStoreForTest(t)
	a, _ := NewStateJoinTokenStore(store)
	ctx := context.Background()
	x := sampleInMemToken("id-a", "kscore-join-SAMEPREF")
	y := sampleInMemToken("id-b", "kscore-join-SAMEPREF") // same prefix
	if err := a.Create(ctx, x); err != nil {
		t.Fatalf("Create x: %v", err)
	}
	err := a.Create(ctx, y)
	if !errors.Is(err, ErrJoinTokenDuplicate) {
		t.Errorf("err = %v, want ErrJoinTokenDuplicate", err)
	}
}

// ---- Validation pre-empts the DB ---------------------------------

func TestStateJoinTokenStore_Create_Validation(t *testing.T) {
	t.Parallel()
	store := stateSQLiteStoreForTest(t)
	a, _ := NewStateJoinTokenStore(store)
	bad := sampleInMemToken("v", "kscore-join-V0000001")
	bad.ID = "" // adapter-side validation rejects before hitting state
	err := a.Create(context.Background(), bad)
	if !errors.Is(err, ErrJoinTokenInvalid) {
		t.Errorf("err = %v, want ErrJoinTokenInvalid", err)
	}
}

// ---- Get / Lookup missing ---------------------------------------

func TestStateJoinTokenStore_Get_NotFound(t *testing.T) {
	t.Parallel()
	store := stateSQLiteStoreForTest(t)
	a, _ := NewStateJoinTokenStore(store)
	_, err := a.Get(context.Background(), "missing")
	if !errors.Is(err, ErrJoinTokenNotFound) {
		t.Errorf("err = %v", err)
	}
}

func TestStateJoinTokenStore_Lookup_NotFound(t *testing.T) {
	t.Parallel()
	store := stateSQLiteStoreForTest(t)
	a, _ := NewStateJoinTokenStore(store)
	_, err := a.Lookup(context.Background(), "kscore-join-NOSUCHPP")
	if !errors.Is(err, ErrJoinTokenNotFound) {
		t.Errorf("err = %v", err)
	}
}

// ---- List filters ------------------------------------------------

func TestStateJoinTokenStore_List_Filters(t *testing.T) {
	t.Parallel()
	store := stateSQLiteStoreForTest(t)
	a, _ := NewStateJoinTokenStore(store)
	ctx := context.Background()

	now := time.Now().Truncate(time.Second)
	mk := func(id, prefix, agent string, mut func(*JoinToken)) JoinToken {
		tok := sampleInMemToken(id, prefix)
		tok.AgentID = agent
		mut(&tok)
		return tok
	}
	x := mk("x", "kscore-join-X0000001", "agent-web", func(*JoinToken) {})
	y := mk("y", "kscore-join-Y0000001", "agent-db", func(t *JoinToken) {
		t.MaxUses = 2
		t.UsedCount = 2 // exhausted
	})
	z := mk("z", "kscore-join-Z0000001", "agent-web", func(t *JoinToken) {
		t.ExpiresAt = now.Add(-time.Hour) // expired
	})
	for _, tok := range []JoinToken{x, y, z} {
		if err := a.Create(ctx, tok); err != nil {
			t.Fatalf("Create %s: %v", tok.ID, err)
		}
	}

	all, _ := a.List(ctx, ListJoinTokensFilter{})
	if len(all) != 3 {
		t.Errorf("List all len = %d, want 3", len(all))
	}

	byAgent, _ := a.List(ctx, ListJoinTokensFilter{AgentID: "agent-web"})
	if len(byAgent) != 2 {
		t.Errorf("byAgent len = %d, want 2", len(byAgent))
	}

	unused, _ := a.List(ctx, ListJoinTokensFilter{Unused: true})
	if len(unused) != 2 {
		t.Errorf("unused len = %d, want 2", len(unused))
	}

	unexpired, _ := a.List(ctx, ListJoinTokensFilter{UnexpiredAt: now})
	if len(unexpired) != 2 {
		t.Errorf("unexpired len = %d, want 2", len(unexpired))
	}
}

// ---- MarkUsed semantics -----------------------------------------

func TestStateJoinTokenStore_MarkUsed(t *testing.T) {
	t.Parallel()
	store := stateSQLiteStoreForTest(t)
	a, _ := NewStateJoinTokenStore(store)
	ctx := context.Background()
	tok := sampleInMemToken("mu", "kscore-join-MU000001")
	tok.MaxUses = 3
	if err := a.Create(ctx, tok); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := a.MarkUsed(ctx, "mu", time.Now()); err != nil {
		t.Fatalf("MarkUsed: %v", err)
	}
	got, _ := a.Get(ctx, "mu")
	if got.UsedCount != 1 {
		t.Errorf("UsedCount = %d, want 1", got.UsedCount)
	}
	if got.UsedAt == nil {
		t.Error("UsedAt nil")
	}
}

func TestStateJoinTokenStore_MarkUsed_NotFound(t *testing.T) {
	t.Parallel()
	store := stateSQLiteStoreForTest(t)
	a, _ := NewStateJoinTokenStore(store)
	err := a.MarkUsed(context.Background(), "missing", time.Now())
	if !errors.Is(err, ErrJoinTokenNotFound) {
		t.Errorf("err = %v, want ErrJoinTokenNotFound", err)
	}
}

// MarkUsed against a record at MaxUses: the state-side UPDATE
// returns 0 rows (which maps to state.ErrNotFound); the adapter
// re-fetches and reports ErrJoinTokenExhausted because the record
// IS at the cap.
func TestStateJoinTokenStore_MarkUsed_ExhaustedMaps(t *testing.T) {
	t.Parallel()
	store := stateSQLiteStoreForTest(t)
	a, _ := NewStateJoinTokenStore(store)
	ctx := context.Background()
	tok := sampleInMemToken("ex", "kscore-join-EX000001")
	tok.MaxUses = 1
	tok.UsedCount = 1 // already at cap
	if err := a.Create(ctx, tok); err != nil {
		t.Fatalf("Create: %v", err)
	}
	err := a.MarkUsed(ctx, "ex", time.Now())
	if !errors.Is(err, ErrJoinTokenExhausted) {
		t.Errorf("err = %v, want ErrJoinTokenExhausted", err)
	}
}

// ---- Delete ------------------------------------------------------

func TestStateJoinTokenStore_Delete(t *testing.T) {
	t.Parallel()
	store := stateSQLiteStoreForTest(t)
	a, _ := NewStateJoinTokenStore(store)
	ctx := context.Background()
	tok := sampleInMemToken("d", "kscore-join-D0000001")
	if err := a.Create(ctx, tok); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := a.Delete(ctx, "d"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := a.Get(ctx, "d"); !errors.Is(err, ErrJoinTokenNotFound) {
		t.Errorf("post-delete Get: %v", err)
	}
}

func TestStateJoinTokenStore_Delete_Missing(t *testing.T) {
	t.Parallel()
	store := stateSQLiteStoreForTest(t)
	a, _ := NewStateJoinTokenStore(store)
	err := a.Delete(context.Background(), "no-such-id")
	if !errors.Is(err, ErrJoinTokenNotFound) {
		t.Errorf("err = %v", err)
	}
}

// ---- Cleanup -----------------------------------------------------

func TestStateJoinTokenStore_Cleanup(t *testing.T) {
	t.Parallel()
	store := stateSQLiteStoreForTest(t)
	a, _ := NewStateJoinTokenStore(store)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	expired := sampleInMemToken("expired", "kscore-join-EXP00001")
	expired.ExpiresAt = now.Add(-time.Hour)
	live := sampleInMemToken("live", "kscore-join-LIVE0001")
	live.ExpiresAt = now.Add(time.Hour)

	for _, tok := range []JoinToken{expired, live} {
		if err := a.Create(ctx, tok); err != nil {
			t.Fatalf("Create %s: %v", tok.ID, err)
		}
	}

	n, err := a.Cleanup(ctx, now)
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if n != 1 {
		t.Errorf("n = %d, want 1", n)
	}
	if _, err := a.Get(ctx, "expired"); !errors.Is(err, ErrJoinTokenNotFound) {
		t.Errorf("expired still present: %v", err)
	}
	if _, err := a.Get(ctx, "live"); err != nil {
		t.Errorf("live removed: %v", err)
	}
}

// ---- End-to-end with JoinTokenAttestor --------------------------

func TestStateJoinTokenStore_WithAttestor_RoundTrip(t *testing.T) {
	t.Parallel()
	store := stateSQLiteStoreForTest(t)
	a, _ := NewStateJoinTokenStore(store)
	ctx := context.Background()

	token, rec := validTokenAndRecord(t, "agent-state-e2e", time.Hour, 1)
	if err := a.Create(ctx, *rec); err != nil {
		t.Fatalf("Create: %v", err)
	}

	att, err := NewJoinTokenAttestor(JoinTokenAttestorConfig{
		Store:       a,
		TrustDomain: DefaultTrustDomain,
	})
	if err != nil {
		t.Fatalf("NewJoinTokenAttestor: %v", err)
	}

	res, err := att.Attest(ctx, []byte(token))
	if err != nil {
		t.Fatalf("Attest: %v", err)
	}
	wantID, _ := AgentID(DefaultTrustDomain, "agent-state-e2e")
	if !res.ID.Equal(wantID) {
		t.Errorf("ID = %q, want %q", res.ID, wantID)
	}
	// Second attempt — exhausted.
	_, err = att.Attest(ctx, []byte(token))
	if !errors.Is(err, ErrJoinTokenExhausted) {
		t.Errorf("second Attest err = %v, want ErrJoinTokenExhausted", err)
	}
}
