// SPDX-License-Identifier: Apache-2.0

package identity

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sort"
	"sync"
	"time"
)

// InMemoryJoinTokenStore is the v0.1 default [JoinTokenStore] —
// a single-process map suitable for the v0.5 single-CP profile.
// Multi-CP HA (Epic 13 / gate-v1.0) uses the
// [StateJoinTokenStore] adapter which delegates to
// [internal/state.JoinTokenStore]; see ROADMAP "DB-backed
// JoinTokenStore" for the migration plan.
//
// Goroutine-safe via sync.RWMutex. [InMemoryJoinTokenStore.MarkUsed]
// runs under the write lock so the MaxUses contract is honoured
// even under N concurrent attestation attempts — verified by
// TestInMemoryJoinTokenStore_MarkUsed_Concurrent.
//
// Persists only Hash + Salt — Create forcibly clears Token before
// storing so cleartext can't accidentally leak via Get / Lookup /
// List.
type InMemoryJoinTokenStore struct {
	mu       sync.RWMutex
	byID     map[string]*JoinToken
	byPrefix map[string]*JoinToken
}

// Compile-time interface assertion.
var _ JoinTokenStore = (*InMemoryJoinTokenStore)(nil)

// NewInMemoryJoinTokenStore returns an empty store.
func NewInMemoryJoinTokenStore() *InMemoryJoinTokenStore {
	return &InMemoryJoinTokenStore{
		byID:     map[string]*JoinToken{},
		byPrefix: map[string]*JoinToken{},
	}
}

// Create validates the record + persists it. Token is forcibly
// cleared on the stored copy.
func (s *InMemoryJoinTokenStore) Create(_ context.Context, token JoinToken) error {
	if err := validateJoinTokenForCreate(&token); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.byID[token.ID]; exists {
		return fmt.Errorf("%w: id=%q", ErrJoinTokenDuplicate, token.ID)
	}
	if _, exists := s.byPrefix[token.Prefix]; exists {
		return fmt.Errorf("%w: prefix=%q", ErrJoinTokenDuplicate, token.Prefix)
	}
	stored := cloneJoinToken(&token)
	stored.Token = "" // never persist cleartext
	s.byID[stored.ID] = stored
	s.byPrefix[stored.Prefix] = stored
	return nil
}

// Get returns the record by ID. Defensive copy.
func (s *InMemoryJoinTokenStore) Get(_ context.Context, id string) (*JoinToken, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec, ok := s.byID[id]
	if !ok {
		return nil, fmt.Errorf("%w: id=%q", ErrJoinTokenNotFound, id)
	}
	return cloneJoinToken(rec), nil
}

// Lookup returns the record by prefix. Defensive copy.
func (s *InMemoryJoinTokenStore) Lookup(_ context.Context, prefix string) (*JoinToken, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec, ok := s.byPrefix[prefix]
	if !ok {
		return nil, fmt.Errorf("%w: prefix=%q", ErrJoinTokenNotFound, prefix)
	}
	return cloneJoinToken(rec), nil
}

// List returns the records matching filter, sorted by CreatedAt
// ascending (id tie-break for determinism). All returned records
// are defensive copies.
func (s *InMemoryJoinTokenStore) List(_ context.Context, filter ListJoinTokensFilter) ([]*JoinToken, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*JoinToken, 0, len(s.byID))
	for _, rec := range s.byID {
		if filter.AgentID != "" && rec.AgentID != filter.AgentID {
			continue
		}
		if filter.Unused && rec.MaxUses > 0 && rec.UsedCount >= rec.MaxUses {
			continue
		}
		if !filter.UnexpiredAt.IsZero() && !rec.ExpiresAt.After(filter.UnexpiredAt) {
			continue
		}
		out = append(out, cloneJoinToken(rec))
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// MarkUsed atomically increments UsedCount + sets UsedAt. The
// re-check of `UsedCount < MaxUses` under the write lock is what
// makes the MaxUses contract race-safe — concurrent goroutines
// see exactly MaxUses successes regardless of dispatch order.
func (s *InMemoryJoinTokenStore) MarkUsed(_ context.Context, id string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.byID[id]
	if !ok {
		return fmt.Errorf("%w: id=%q", ErrJoinTokenNotFound, id)
	}
	if rec.MaxUses > 0 && rec.UsedCount >= rec.MaxUses {
		return fmt.Errorf("%w: id=%q", ErrJoinTokenExhausted, id)
	}
	rec.UsedCount++
	usedAt := now
	rec.UsedAt = &usedAt
	return nil
}

// Delete removes the record by ID + prefix. Returns
// [ErrJoinTokenNotFound] when absent.
func (s *InMemoryJoinTokenStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.byID[id]
	if !ok {
		return fmt.Errorf("%w: id=%q", ErrJoinTokenNotFound, id)
	}
	delete(s.byID, id)
	delete(s.byPrefix, rec.Prefix)
	return nil
}

// Cleanup removes every record whose ExpiresAt is at or before
// `before`. Returns the number of records removed.
func (s *InMemoryJoinTokenStore) Cleanup(_ context.Context, before time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	victims := make([]string, 0)
	for id, rec := range s.byID {
		if !rec.ExpiresAt.After(before) {
			victims = append(victims, id)
		}
	}
	for _, id := range victims {
		rec := s.byID[id]
		delete(s.byID, id)
		delete(s.byPrefix, rec.Prefix)
	}
	return len(victims), nil
}

// ---- shared helpers (also used by the state adapter) ------------

// validateJoinTokenForCreate is the shared pre-Create shape check.
// Returns wrapped [ErrJoinTokenInvalid] on any failure.
func validateJoinTokenForCreate(t *JoinToken) error {
	if t == nil {
		return fmt.Errorf("%w: nil token", ErrJoinTokenInvalid)
	}
	if t.ID == "" {
		return fmt.Errorf("%w: ID is required", ErrJoinTokenInvalid)
	}
	if t.Prefix == "" {
		return fmt.Errorf("%w: Prefix is required", ErrJoinTokenInvalid)
	}
	if len(t.Hash) == 0 {
		return fmt.Errorf("%w: Hash is required", ErrJoinTokenInvalid)
	}
	if len(t.Salt) == 0 {
		return fmt.Errorf("%w: Salt is required", ErrJoinTokenInvalid)
	}
	if t.ExpiresAt.IsZero() {
		return fmt.Errorf("%w: ExpiresAt is required", ErrJoinTokenInvalid)
	}
	if t.MaxUses <= 0 {
		return fmt.Errorf("%w: MaxUses must be > 0", ErrJoinTokenInvalid)
	}
	return nil
}

// cloneJoinToken returns a deep copy of t. Pointer fields
// (UsedAt) are also copied so callers can't reach back through
// shared pointers.
func cloneJoinToken(t *JoinToken) *JoinToken {
	out := *t
	out.Hash = slices.Clone(t.Hash)
	out.Salt = slices.Clone(t.Salt)
	out.Metadata = maps.Clone(t.Metadata)
	if t.UsedAt != nil {
		ts := *t.UsedAt
		out.UsedAt = &ts
	}
	return &out
}
