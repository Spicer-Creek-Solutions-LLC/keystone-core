package identity

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.keystone-core.io/keystone-core/internal/state"
)

// StateJoinTokenStore adapts a [state.JoinTokenStore] (SQLite or
// PostgreSQL) into the [identity.JoinTokenStore] interface the
// [JoinTokenAttestor] + [EmbeddedProvider] consume. v0.5
// single-CP deployments stick with the [InMemoryJoinTokenStore];
// multi-CP HA (Epic 13 / gate-v1.0) uses this adapter against
// the shared cluster DB.
//
// The adapter is a thin translator: it maps between the identity
// JoinToken (domain shape, pointer UsedAt) and the state
// JoinTokenRecord (persistence shape, value UsedAt), preserves
// errors via errors.Is on the state sentinels, and applies the
// same MaxUses semantics on top of the DB-backed MarkUsed that
// the in-memory store enforces under its mutex.
type StateJoinTokenStore struct {
	store state.JoinTokenStore
}

// Compile-time interface assertion.
var _ JoinTokenStore = (*StateJoinTokenStore)(nil)

// NewStateJoinTokenStore wires the adapter around a state-backed
// store. Returns [ErrInvalidProvider] on a nil store.
func NewStateJoinTokenStore(store state.JoinTokenStore) (*StateJoinTokenStore, error) {
	if store == nil {
		return nil, fmt.Errorf("%w: state.JoinTokenStore is required", ErrInvalidProvider)
	}
	return &StateJoinTokenStore{store: store}, nil
}

// Create writes the record. Maps state.ErrDuplicate →
// identity.ErrJoinTokenDuplicate so attestor + provider callers
// see the same sentinel regardless of backend.
func (s *StateJoinTokenStore) Create(ctx context.Context, token JoinToken) error {
	if err := validateJoinTokenForCreate(&token); err != nil {
		return err
	}
	rec := toJoinTokenRecord(&token)
	if err := s.store.CreateJoinToken(ctx, rec); err != nil {
		if errors.Is(err, state.ErrDuplicate) {
			return fmt.Errorf("%w: %v", ErrJoinTokenDuplicate, err)
		}
		return fmt.Errorf("state-backed create: %w", err)
	}
	return nil
}

// Get reads by ID. Maps state.ErrNotFound → identity.ErrJoinTokenNotFound.
func (s *StateJoinTokenStore) Get(ctx context.Context, id string) (*JoinToken, error) {
	rec, err := s.store.GetJoinToken(ctx, id)
	if err != nil {
		return nil, mapNotFound(err, id)
	}
	return fromJoinTokenRecord(rec), nil
}

// Lookup reads by prefix.
func (s *StateJoinTokenStore) Lookup(ctx context.Context, prefix string) (*JoinToken, error) {
	rec, err := s.store.LookupJoinTokenByPrefix(ctx, prefix)
	if err != nil {
		return nil, mapNotFound(err, prefix)
	}
	return fromJoinTokenRecord(rec), nil
}

// List maps the identity-side filter shape to the state-side one.
func (s *StateJoinTokenStore) List(ctx context.Context, filter ListJoinTokensFilter) ([]*JoinToken, error) {
	stateFilter := state.JoinTokenFilter{
		AgentID:     filter.AgentID,
		Unused:      filter.Unused,
		UnexpiredAt: filter.UnexpiredAt,
	}
	recs, err := s.store.ListJoinTokens(ctx, stateFilter)
	if err != nil {
		return nil, fmt.Errorf("state-backed list: %w", err)
	}
	out := make([]*JoinToken, 0, len(recs))
	for _, r := range recs {
		out = append(out, fromJoinTokenRecord(r))
	}
	return out, nil
}

// MarkUsed delegates to the state store's atomic UPDATE. The DB's
// WHERE `used_count < max_uses` clause means an exhausted token
// gets 0 rows updated → state.ErrNotFound. We map that to
// ErrJoinTokenExhausted ONLY if a subsequent Get shows the record
// is at MaxUses (otherwise the record really is missing).
func (s *StateJoinTokenStore) MarkUsed(ctx context.Context, id string, now time.Time) error {
	err := s.store.MarkJoinTokenUsed(ctx, id, now)
	if err == nil {
		return nil
	}
	if !errors.Is(err, state.ErrNotFound) {
		return fmt.Errorf("state-backed mark used: %w", err)
	}
	// MarkJoinTokenUsed returned NotFound — could be:
	//   (a) the record genuinely doesn't exist
	//   (b) the record is at MaxUses (the UPDATE's WHERE clause
	//       refused to bump UsedCount past the cap)
	// Distinguish via a follow-up Get so callers branch on the
	// right sentinel.
	rec, getErr := s.store.GetJoinToken(ctx, id)
	if getErr != nil {
		if errors.Is(getErr, state.ErrNotFound) {
			return fmt.Errorf("%w: id=%q", ErrJoinTokenNotFound, id)
		}
		return fmt.Errorf("state-backed mark used (post-fail get): %w", getErr)
	}
	if rec.MaxUses > 0 && rec.UsedCount >= rec.MaxUses {
		return fmt.Errorf("%w: id=%q", ErrJoinTokenExhausted, id)
	}
	// Unreachable in well-behaved storage — the UPDATE refused
	// for a reason we couldn't reconstruct. Surface as a generic
	// error so the caller doesn't silently retry forever.
	return fmt.Errorf("state-backed mark used: UPDATE returned 0 rows but record not exhausted")
}

// Delete maps state.ErrNotFound → identity.ErrJoinTokenNotFound.
func (s *StateJoinTokenStore) Delete(ctx context.Context, id string) error {
	if err := s.store.DeleteJoinToken(ctx, id); err != nil {
		return mapNotFound(err, id)
	}
	return nil
}

// Cleanup delegates to state.DeleteExpiredJoinTokens.
func (s *StateJoinTokenStore) Cleanup(ctx context.Context, before time.Time) (int, error) {
	n, err := s.store.DeleteExpiredJoinTokens(ctx, before)
	if err != nil {
		return 0, fmt.Errorf("state-backed cleanup: %w", err)
	}
	return n, nil
}

// ---- converters ------------------------------------------------

func toJoinTokenRecord(t *JoinToken) *state.JoinTokenRecord {
	rec := &state.JoinTokenRecord{
		ID:        t.ID,
		Hash:      t.Hash,
		Salt:      t.Salt,
		Prefix:    t.Prefix,
		AgentID:   t.AgentID,
		TTL:       t.TTL,
		CreatedAt: t.CreatedAt,
		ExpiresAt: t.ExpiresAt,
		MaxUses:   t.MaxUses,
		UsedCount: t.UsedCount,
		Metadata:  t.Metadata,
	}
	if t.UsedAt != nil {
		rec.UsedAt = *t.UsedAt
	}
	return rec
}

func fromJoinTokenRecord(r *state.JoinTokenRecord) *JoinToken {
	t := &JoinToken{
		ID:        r.ID,
		Hash:      r.Hash,
		Salt:      r.Salt,
		Prefix:    r.Prefix,
		AgentID:   r.AgentID,
		TTL:       r.TTL,
		CreatedAt: r.CreatedAt,
		ExpiresAt: r.ExpiresAt,
		MaxUses:   r.MaxUses,
		UsedCount: r.UsedCount,
		Metadata:  r.Metadata,
		// Token deliberately left "" — state never holds cleartext.
	}
	if !r.UsedAt.IsZero() {
		ts := r.UsedAt
		t.UsedAt = &ts
	}
	return t
}

func mapNotFound(err error, key string) error {
	if errors.Is(err, state.ErrNotFound) {
		return fmt.Errorf("%w: %q", ErrJoinTokenNotFound, key)
	}
	return fmt.Errorf("state-backed: %w", err)
}
