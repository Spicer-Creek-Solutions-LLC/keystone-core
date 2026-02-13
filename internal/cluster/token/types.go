// Package token implements secure join token generation, storage, and lifecycle
// management for cluster membership authentication.
package token

import (
	"context"
	"errors"
	"time"
)

const (
	// TokenPrefix is prepended to all generated join tokens.
	TokenPrefix = "kscore-join-"

	// TokenLength is the number of random base62 characters in a token.
	TokenLength = 32

	// DefaultTTL is the default time-to-live for join tokens.
	DefaultTTL = 24 * time.Hour

	// etcdKeyPrefix is the key prefix for tokens stored in etcd.
	etcdKeyPrefix = "/cluster/tokens/"
)

// Error sentinels for token operations.
var (
	ErrTokenNotFound   = errors.New("token: not found")
	ErrTokenExpired    = errors.New("token: expired")
	ErrTokenRevoked    = errors.New("token: revoked")
	ErrTokenExhausted  = errors.New("token: max uses reached")
	ErrTokenDuplicate  = errors.New("token: already exists")
	ErrCASMaxRetries   = errors.New("token: compare-and-swap max retries exceeded")
)

// JoinToken represents a cluster join token.
type JoinToken struct {
	ID        string    `json:"id"`
	TokenHash string    `json:"token_hash"`
	Salt      string    `json:"salt"`
	Label     string    `json:"label,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
	MaxUses   int       `json:"max_uses"`
	UsedCount int       `json:"used_count"`
	CreatedBy string    `json:"created_by"`
	Revoked   bool      `json:"revoked"`
}

// IsValid returns true if the token can be used for joining.
func (t *JoinToken) IsValid() bool {
	return !t.Revoked && !t.IsExpired() && t.HasUsesRemaining()
}

// IsExpired returns true if the token has passed its expiration time.
func (t *JoinToken) IsExpired() bool {
	return time.Now().After(t.ExpiresAt)
}

// HasUsesRemaining returns true if the token has not exhausted its use limit.
// A MaxUses of 0 means unlimited.
func (t *JoinToken) HasUsesRemaining() bool {
	return t.MaxUses == 0 || t.UsedCount < t.MaxUses
}

// Store defines the storage interface for cluster join tokens.
type Store interface {
	// Create persists a new join token.
	Create(ctx context.Context, token *JoinToken) error

	// GetByID retrieves a token by its unique ID.
	GetByID(ctx context.Context, id string) (*JoinToken, error)

	// Lookup finds a token by its raw (plaintext) value, hashing against
	// each stored salt until a match is found.
	Lookup(ctx context.Context, rawToken string) (*JoinToken, error)

	// List returns all stored tokens.
	List(ctx context.Context) ([]*JoinToken, error)

	// Revoke marks a token as revoked (soft-delete for audit trail).
	Revoke(ctx context.Context, id string) error

	// IncrementUses atomically increments UsedCount, returning an error
	// if the token is invalid, expired, revoked, or exhausted.
	IncrementUses(ctx context.Context, id string) error

	// DeleteExpired removes expired and revoked tokens. Returns the count deleted.
	DeleteExpired(ctx context.Context) (int, error)
}

// EtcdBackend abstracts the etcd operations needed by EtcdStore.
// *cluster.EtcdClient satisfies this interface.
type EtcdBackend interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Put(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, prefix string) (map[string][]byte, error)
	CompareAndSwap(ctx context.Context, key string, expected, value []byte) (bool, error)
}
