package identity

import (
	"context"
	"errors"
	"time"
)

// JoinTokenScheme is the cleartext-token prefix every v0.1 join
// token starts with. Operators recognize one on sight in a log
// line, a paste buffer, or a config file:
//
//	kscore-join-<base62-random>
const JoinTokenScheme = "kscore-join-"

// JoinTokenPrefixLen is the length of the operator-visible prefix
// stored on each [JoinToken] record: scheme + 8 chars of random
// body. The attestor uses the prefix as the store-lookup key.
// 62^8 = 218 trillion distinct prefixes — collisions are
// statistically negligible at v0.1 fleet sizes.
const JoinTokenPrefixLen = len(JoinTokenScheme) + 8

// JoinTokenRandomMinLen is the minimum length of the random body
// per §4.10 ("≥ 32 chars"). The full token length is at least
// `JoinTokenScheme + JoinTokenRandomMinLen` characters.
const JoinTokenRandomMinLen = 32

// ErrJoinTokenNotFound is returned by [JoinTokenStore.Lookup]
// when no record matches the given prefix.
var ErrJoinTokenNotFound = errors.New("identity: join token not found")

// ErrJoinTokenExhausted is returned by [JoinTokenStore.MarkUsed]
// when the requested increment would push UsedCount past MaxUses.
// Callers (notably [JoinTokenAttestor]) MUST also check exhaustion
// at attestation time so the race window between Lookup and
// MarkUsed stays narrow.
var ErrJoinTokenExhausted = errors.New("identity: join token exhausted")

// JoinTokenStore is the narrow read surface the
// [JoinTokenAttestor] needs from the join-token persistence
// layer. Task 9 extends this interface with the write side
// (Create / Delete / List / Cleanup); task 8 ships only what
// attestation reads, so concrete store implementations can land
// independently in task 9 without ripping up the attestor.
//
// Implementations MUST be goroutine-safe — the attestor may be
// called concurrently from multiple gRPC handlers, and
// [JoinTokenStore.MarkUsed] in particular MUST be atomic so the
// MaxUses contract isn't violated under contention.
type JoinTokenStore interface {
	// Lookup finds a join-token record by its [JoinToken.Prefix].
	// Returns [ErrJoinTokenNotFound] when absent. Lookup does NOT
	// authenticate — the caller MUST verify the full salted hash
	// against the presented cleartext before trusting the record.
	Lookup(ctx context.Context, prefix string) (*JoinToken, error)

	// MarkUsed atomically increments UsedCount and sets UsedAt
	// to `now`. Returns [ErrJoinTokenExhausted] when the
	// increment would violate MaxUses. The atomicity guarantee
	// is what makes the "single-use" semantics safe under
	// concurrent attestation attempts.
	MarkUsed(ctx context.Context, id string, now time.Time) error
}
