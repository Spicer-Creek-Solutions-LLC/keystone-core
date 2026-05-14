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

// Defaults + cap per PROJECT-DETAILS §4.10:
//
//   - Default TTL 5m (short enough that an unused token doesn't
//     linger past an operator's attention span).
//   - Max TTL 24h (long enough to issue a token in the morning
//     and have a remote agent claim it before EOD).
//   - One-time-use by default; MaxUses > 1 supports cohort
//     rollouts.
const (
	DefaultJoinTokenTTL     = 5 * time.Minute
	MaxJoinTokenTTL         = 24 * time.Hour
	DefaultJoinTokenMaxUses = 1
)

// ErrJoinTokenStoreNotConfigured is returned by the
// [EmbeddedProvider] join-token methods when called before the
// operator wires a [JoinTokenStore] into
// [EmbeddedProviderConfig.JoinTokenStore]. Distinct from
// [ErrNotImplementedYet] (which task 7 returned before task 10
// landed) — this sentinel means the surface IS implemented; the
// operator just hasn't enabled the storage backend.
var ErrJoinTokenStoreNotConfigured = errors.New("identity: provider has no JoinTokenStore configured")

// ErrJoinTokenNotFound is returned by [JoinTokenStore.Lookup]
// when no record matches the given prefix.
var ErrJoinTokenNotFound = errors.New("identity: join token not found")

// ErrJoinTokenExhausted is returned by [JoinTokenStore.MarkUsed]
// when the requested increment would push UsedCount past MaxUses.
// Callers (notably [JoinTokenAttestor]) MUST also check exhaustion
// at attestation time so the race window between Lookup and
// MarkUsed stays narrow.
var ErrJoinTokenExhausted = errors.New("identity: join token exhausted")

// ErrJoinTokenInvalid wraps every Create-time shape rejection
// (empty ID / Prefix / Hash / Salt; zero ExpiresAt; zero
// MaxUses). Distinct from [ErrJoinTokenNotFound] +
// [ErrJoinTokenExhausted] so call sites branch on whether the
// input was malformed vs. the store state.
var ErrJoinTokenInvalid = errors.New("identity: invalid join token")

// ErrJoinTokenDuplicate is returned by [JoinTokenStore.Create]
// when the new record's ID or Prefix collides with an existing
// record.
var ErrJoinTokenDuplicate = errors.New("identity: join token already exists")

// JoinTokenStore is the full persistence surface for join tokens.
// Task 8 shipped the narrow Lookup + MarkUsed half (what the
// attestor reads); task 9 extends with the write side that the
// EmbeddedProvider's CreateJoinToken / ListJoinTokens /
// DeleteJoinToken methods (task 10) + the background cleanup
// loop (task 11) need.
//
// Implementations MUST be goroutine-safe — the attestor may be
// called concurrently from multiple gRPC handlers, and
// [JoinTokenStore.MarkUsed] in particular MUST be atomic so the
// MaxUses contract isn't violated under contention.
//
// Implementations MUST persist only the hash + salt — never
// cleartext Token. Get / List / Lookup MUST always return records
// with Token == "".
type JoinTokenStore interface {
	// Create persists a new join token. Validates ID / Prefix /
	// Hash / Salt are non-empty; ExpiresAt > 0; MaxUses > 0.
	// Stores the record with Token forcibly cleared. Returns
	// ErrJoinTokenDuplicate when ID or Prefix collides.
	Create(ctx context.Context, token JoinToken) error

	// Get returns a token by ID. ErrJoinTokenNotFound when absent.
	Get(ctx context.Context, id string) (*JoinToken, error)

	// Lookup finds a join-token record by its [JoinToken.Prefix].
	// Returns [ErrJoinTokenNotFound] when absent. Lookup does NOT
	// authenticate — the caller MUST verify the full salted hash
	// against the presented cleartext before trusting the record.
	Lookup(ctx context.Context, prefix string) (*JoinToken, error)

	// List returns tokens matching filter, oldest-first by default.
	List(ctx context.Context, filter ListJoinTokensFilter) ([]*JoinToken, error)

	// MarkUsed atomically increments UsedCount and sets UsedAt
	// to `now`. Returns [ErrJoinTokenExhausted] when the
	// increment would violate MaxUses. The atomicity guarantee
	// is what makes the "single-use" semantics safe under
	// concurrent attestation attempts.
	MarkUsed(ctx context.Context, id string, now time.Time) error

	// Delete removes a token by ID. Returns [ErrJoinTokenNotFound]
	// when absent; callers that want idempotent delete branch on
	// errors.Is(err, ErrJoinTokenNotFound).
	Delete(ctx context.Context, id string) error

	// Cleanup removes every token whose ExpiresAt is at or
	// before `before`. Returns the count removed. Task 11's
	// hourly cleanup loop is the production caller.
	Cleanup(ctx context.Context, before time.Time) (int, error)
}
