package token

import (
	"context"
	"encoding/json"
	"fmt"
)

const casMaxRetries = 5

// EtcdStore implements Store using an etcd backend.
type EtcdStore struct {
	backend EtcdBackend
}

// NewEtcdStore creates a new etcd-backed token store.
func NewEtcdStore(backend EtcdBackend) (*EtcdStore, error) {
	if backend == nil {
		return nil, fmt.Errorf("etcd backend is required")
	}
	return &EtcdStore{backend: backend}, nil
}

func (s *EtcdStore) tokenKey(id string) string {
	return etcdKeyPrefix + id
}

// Create persists a new join token.
func (s *EtcdStore) Create(ctx context.Context, token *JoinToken) error {
	data, err := json.Marshal(token)
	if err != nil {
		return fmt.Errorf("failed to marshal token: %w", err)
	}

	// Use CAS to ensure no duplicate ID.
	ok, err := s.backend.CompareAndSwap(ctx, s.tokenKey(token.ID), nil, data)
	if err != nil {
		return fmt.Errorf("failed to create token: %w", err)
	}
	if !ok {
		return ErrTokenDuplicate
	}
	return nil
}

// GetByID retrieves a token by its unique ID.
func (s *EtcdStore) GetByID(ctx context.Context, id string) (*JoinToken, error) {
	data, err := s.backend.Get(ctx, s.tokenKey(id))
	if err != nil {
		return nil, fmt.Errorf("failed to get token: %w", err)
	}
	if data == nil {
		return nil, ErrTokenNotFound
	}

	var token JoinToken
	if err := json.Unmarshal(data, &token); err != nil {
		return nil, fmt.Errorf("failed to unmarshal token: %w", err)
	}
	return &token, nil
}

// Lookup finds a token by hashing the raw value against each stored salt.
func (s *EtcdStore) Lookup(ctx context.Context, rawToken string) (*JoinToken, error) {
	entries, err := s.backend.List(ctx, etcdKeyPrefix)
	if err != nil {
		return nil, fmt.Errorf("failed to list tokens: %w", err)
	}

	for _, data := range entries {
		var token JoinToken
		if err := json.Unmarshal(data, &token); err != nil {
			continue
		}
		if HashToken(rawToken, token.Salt) == token.TokenHash {
			return &token, nil
		}
	}
	return nil, ErrTokenNotFound
}

// List returns all stored tokens.
func (s *EtcdStore) List(ctx context.Context) ([]*JoinToken, error) {
	entries, err := s.backend.List(ctx, etcdKeyPrefix)
	if err != nil {
		return nil, fmt.Errorf("failed to list tokens: %w", err)
	}

	tokens := make([]*JoinToken, 0, len(entries))
	for _, data := range entries {
		var token JoinToken
		if err := json.Unmarshal(data, &token); err != nil {
			continue
		}
		tokens = append(tokens, &token)
	}
	return tokens, nil
}

// Revoke marks a token as revoked.
func (s *EtcdStore) Revoke(ctx context.Context, id string) error {
	token, err := s.GetByID(ctx, id)
	if err != nil {
		return err
	}

	token.Revoked = true
	data, err := json.Marshal(token)
	if err != nil {
		return fmt.Errorf("failed to marshal token: %w", err)
	}
	return s.backend.Put(ctx, s.tokenKey(id), data, 0)
}

// IncrementUses atomically increments UsedCount via compare-and-swap.
func (s *EtcdStore) IncrementUses(ctx context.Context, id string) error {
	for i := 0; i < casMaxRetries; i++ {
		data, err := s.backend.Get(ctx, s.tokenKey(id))
		if err != nil {
			return fmt.Errorf("failed to get token: %w", err)
		}
		if data == nil {
			return ErrTokenNotFound
		}

		var token JoinToken
		if err := json.Unmarshal(data, &token); err != nil {
			return fmt.Errorf("failed to unmarshal token: %w", err)
		}

		if token.Revoked {
			return ErrTokenRevoked
		}
		if token.IsExpired() {
			return ErrTokenExpired
		}
		if !token.HasUsesRemaining() {
			return ErrTokenExhausted
		}

		token.UsedCount++
		newData, err := json.Marshal(&token)
		if err != nil {
			return fmt.Errorf("failed to marshal token: %w", err)
		}

		ok, err := s.backend.CompareAndSwap(ctx, s.tokenKey(id), data, newData)
		if err != nil {
			return fmt.Errorf("failed to update token: %w", err)
		}
		if ok {
			return nil
		}
		// CAS failed — retry.
	}
	return ErrCASMaxRetries
}

// DeleteExpired removes expired and revoked tokens.
func (s *EtcdStore) DeleteExpired(ctx context.Context) (int, error) {
	entries, err := s.backend.List(ctx, etcdKeyPrefix)
	if err != nil {
		return 0, fmt.Errorf("failed to list tokens: %w", err)
	}

	deleted := 0
	for key, data := range entries {
		var token JoinToken
		if err := json.Unmarshal(data, &token); err != nil {
			continue
		}
		if token.IsExpired() || token.Revoked {
			if err := s.backend.Delete(ctx, key); err != nil {
				return deleted, fmt.Errorf("failed to delete token %s: %w", token.ID, err)
			}
			deleted++
		}
	}
	return deleted, nil
}

var _ Store = (*EtcdStore)(nil)
