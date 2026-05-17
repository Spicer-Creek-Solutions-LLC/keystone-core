package identity

import (
	"context"
	"errors"
	"net"
	"time"
)

// ErrInvalidProvider wraps every rejection from this file —
// constructor validation, lifecycle-protocol violations, attempts
// to call provider methods before [Provider.Start]. Mirrors the
// ErrInvalid* pattern on the other identity types.
var ErrInvalidProvider = errors.New("identity: invalid Provider")

// ErrProviderNotRunning is the sentinel a provider method returns
// when called before [Provider.Start] or after [Provider.Stop].
// Wraps [ErrInvalidProvider] so existing call sites can keep
// using [errors.Is] against the family root.
var ErrProviderNotRunning = errors.New("identity: provider not running")

// ErrNotImplementedYet is the sentinel the placeholder Provider
// methods return during the v0.1 transitional window: [Provider.Attest]
// (task 8), [Provider.CreateJoinToken] / [Provider.ListJoinTokens]
// / [Provider.DeleteJoinToken] (tasks 9-11). Once those tasks land,
// the sentinel disappears from the call sites; downstream code
// branches with [errors.Is] in the meantime.
var ErrNotImplementedYet = errors.New("identity: provider method not implemented yet (Epic 09 in flight)")

// Provider is the top-level v0.1 identity surface. The embedded
// CA provider (this epic) and the future SPIRE provider (post-v1.0)
// both implement it. The interface is the single seam every
// consumer (pkg/api/auth, NATS bootstrap, the agent runtime, the
// kscore-identity CLI) talks to — nothing else imports
// CAManager / CARotator / etc. directly outside this package.
//
// Lifecycle: NewEmbeddedProvider → Start(ctx) → (running) →
// Stop(ctx). Start is one-shot. Stop is idempotent.
type Provider interface {
	// Lifecycle.
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Health(ctx context.Context) error

	// Identity.
	TrustDomain() string

	// Trust bundle.
	GetTrustBundle(ctx context.Context) (*TrustBundle, error)
	WatchTrustBundle(ctx context.Context) (<-chan *TrustBundle, error)

	// SVID issuance — task 7 implements both.
	IssueX509SVID(ctx context.Context, req IssueX509SVIDRequest) (X509SVID, error)
	IssueJWTSVID(ctx context.Context, req IssueJWTSVIDRequest) (JWTSVID, error)

	// Attestation — task 8 fills in. Task 7 returns
	// [ErrNotImplementedYet].
	Attest(ctx context.Context, req AttestRequest) (*AttestResult, error)

	// Join tokens — tasks 9-11 fill in. Task 7 returns
	// [ErrNotImplementedYet].
	CreateJoinToken(ctx context.Context, req CreateJoinTokenRequest) (JoinToken, error)
	ListJoinTokens(ctx context.Context, filter ListJoinTokensFilter) ([]JoinToken, error)
	DeleteJoinToken(ctx context.Context, id string) error
}

// IssueX509SVIDRequest drives [Provider.IssueX509SVID]. The
// provider generates the subject's private key (matching its CA's
// KeyType by default) so the returned [X509SVID] carries both the
// chain and the key.
type IssueX509SVIDRequest struct {
	ID          SPIFFEID         // required → URI SAN
	KeyType     CAKeyType        // optional; "" → provider's CA KeyType
	TTL         time.Duration    // 0 → CAConfig.DefaultSVIDTTL; > MaxSVIDTTL → capped
	DNSNames    []string
	IPAddresses []net.IP
	Hint        string
}

// IssueJWTSVIDRequest drives [Provider.IssueJWTSVID]. Audience must
// be non-empty; Lifetime is derived from TTL (0 → provider's
// DefaultSVIDTTL).
type IssueJWTSVIDRequest struct {
	ID          SPIFFEID
	Audience    []string         // required, ≥ 1
	TTL         time.Duration    // 0 → CAConfig.DefaultSVIDTTL; > MaxSVIDTTL → capped
	Hint        string
	ExtraClaims map[string]any
}

// AttestorType identifies a workload-attestation strategy. v0.1
// ships `join_token` only; future tasks may add platform attestors
// (TPM, kubelet, AWS IMDS, …).
type AttestorType string

// AttestorTypeJoinToken is the only v0.1-supported attestor —
// validates a join token created via [Provider.CreateJoinToken].
const AttestorTypeJoinToken AttestorType = "join_token"

// AttestRequest is the input to [Provider.Attest]. Data is the
// attestor-specific payload (a join token string, a TPM quote,
// an instance-identity document, …); the attestor decodes it.
// Placeholder until task 8 refines.
type AttestRequest struct {
	Type AttestorType
	Data []byte
}

// AttestResult is what [Provider.Attest] returns on success: the
// attested SPIFFE ID + the selectors the attestor extracted from
// the attestation evidence. Placeholder until task 8 refines.
type AttestResult struct {
	ID        SPIFFEID
	Selectors []string
}

// JoinToken is a single-use (by default) credential a new agent
// presents during cluster join. Shape mirrors PROJECT-DETAILS
// §4.10 — `Token, Hash, Salt, Prefix, AgentID, TTL, UsedAt,
// Metadata`. Tasks 9-11 fill in the storage + lifecycle; task 7
// stabilizes the wire shape.
type JoinToken struct {
	// ID is the server-assigned identifier (UUID). Stable across
	// the token's lifetime; used as the lookup key by
	// [Provider.DeleteJoinToken].
	ID string

	// Token is the cleartext token string. Populated **only** on
	// creation by [Provider.CreateJoinToken]; the persistent store
	// keeps only Hash + Salt. The operator presents this string
	// to the new agent.
	Token string

	// Hash is the SHA-256 of (Salt || Token). What the store
	// persists; never plaintext after creation.
	Hash []byte

	// Salt is a per-token random value combined with the cleartext
	// before hashing. 16 bytes.
	Salt []byte

	// Prefix is a stable, operator-readable identifier — e.g.
	// `kscore-join-<first-8-base62-chars>`. Helps operators
	// identify which token they're revoking without revealing the
	// full plaintext.
	Prefix string

	// AgentID optionally binds the token to a specific agent
	// identifier; empty means any agent may use it.
	AgentID string

	// TTL is the token's lifetime; ExpiresAt = CreatedAt + TTL.
	// Defaults to 5m on creation per §4.10.
	TTL time.Duration

	CreatedAt time.Time
	ExpiresAt time.Time

	// UsedAt is the timestamp of the most-recent successful use,
	// nil when never used.
	UsedAt *time.Time

	// MaxUses caps how many agents may use this token. Defaults
	// to 1 (one-time-use) per §4.10; max-uses > 1 supports bulk
	// agent rollouts.
	MaxUses int

	// UsedCount is the running tally of successful uses; the
	// store rejects use when UsedCount >= MaxUses.
	UsedCount int

	// Metadata is operator-supplied free-form tags (`role: web`,
	// `cluster: prod`, …) — surfaced through [Provider.ListJoinTokens]
	// to help with auditing.
	Metadata map[string]string
}

// CreateJoinTokenRequest drives [Provider.CreateJoinToken].
type CreateJoinTokenRequest struct {
	TTL      time.Duration     // 0 → 5m default
	AgentID  string            // optional binding
	MaxUses  int               // 0 → 1 (one-time-use)
	Metadata map[string]string // optional tags
}

// ListJoinTokensFilter scopes [Provider.ListJoinTokens]. Zero
// value matches all tokens.
type ListJoinTokensFilter struct {
	AgentID     string
	Unused      bool      // only tokens whose UsedCount < MaxUses
	UnexpiredAt time.Time // only tokens whose ExpiresAt > UnexpiredAt
}
