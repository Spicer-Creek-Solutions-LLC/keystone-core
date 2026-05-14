package identity

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// ErrJoinTokenExpired is the [JoinTokenAttestor]-side sentinel for
// "the record was found but its TTL has passed at attestation
// time." Wraps [ErrAttestation] so call sites doing `errors.Is(err,
// ErrAttestation)` still match.
var ErrJoinTokenExpired = errors.New("identity: join token expired")

// JoinTokenAttestor implements [Attestor] for
// [AttestorTypeJoinToken]. Validates the cleartext token against a
// stored [JoinToken] via a constant-time hash comparison, enforces
// expiry + max-uses + AgentID-required (v0.1), marks the record
// used, and returns an [AttestResult] whose ID is
// `spiffe://<td>/agent/<record.AgentID>` and whose selectors carry
// the token prefix + bound agent + operator metadata.
//
// "Any-agent" tokens (records with empty AgentID) are rejected in
// v0.1 — tracked under the ROADMAP entry "Join-token any-agent
// mode (AgentID-less binding)" for v0.x.
type JoinTokenAttestor struct {
	store       JoinTokenStore
	trustDomain string
	clock       func() time.Time
}

// JoinTokenAttestorConfig drives [NewJoinTokenAttestor].
type JoinTokenAttestorConfig struct {
	Store       JoinTokenStore   // required
	TrustDomain string           // required
	Clock       func() time.Time // optional; defaults to time.Now
}

// NewJoinTokenAttestor validates cfg and returns an attestor ready
// for [EmbeddedProvider.Attest] dispatch.
func NewJoinTokenAttestor(cfg JoinTokenAttestorConfig) (*JoinTokenAttestor, error) {
	if cfg.Store == nil {
		return nil, fmt.Errorf("%w: Store is required", ErrInvalidProvider)
	}
	if cfg.TrustDomain == "" {
		return nil, fmt.Errorf("%w: TrustDomain is required", ErrInvalidProvider)
	}
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	return &JoinTokenAttestor{
		store:       cfg.Store,
		trustDomain: cfg.TrustDomain,
		clock:       cfg.Clock,
	}, nil
}

// Type returns [AttestorTypeJoinToken].
func (a *JoinTokenAttestor) Type() AttestorType { return AttestorTypeJoinToken }

// Attest runs the v0.1 join-token attestation flow:
//
//  1. Decode `data` → cleartext token string; reject bad format
//  2. Extract prefix → store.Lookup
//  3. Constant-time sha256(record.Salt || token) vs. record.Hash
//  4. Expiry check (now < ExpiresAt)
//  5. Max-uses check (UsedCount < MaxUses)
//  6. AgentID non-empty (v0.1 requirement)
//  7. store.MarkUsed (atomic increment; race-safe)
//  8. Return AttestResult with sorted selectors
func (a *JoinTokenAttestor) Attest(ctx context.Context, data []byte) (*AttestResult, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("%w: empty attestation data", ErrAttestation)
	}
	token := string(data)
	if !strings.HasPrefix(token, JoinTokenScheme) {
		return nil, fmt.Errorf("%w: token missing scheme prefix %q", ErrAttestation, JoinTokenScheme)
	}
	if len(token) < JoinTokenPrefixLen+JoinTokenRandomMinLen-8 {
		// Prefix already includes 8 random chars; require at least
		// the remaining min-length so the random body satisfies
		// §4.10 ≥ 32-char rule.
		return nil, fmt.Errorf("%w: token too short", ErrAttestation)
	}
	prefix := token[:JoinTokenPrefixLen]

	record, err := a.store.Lookup(ctx, prefix)
	if err != nil {
		if errors.Is(err, ErrJoinTokenNotFound) {
			return nil, fmt.Errorf("%w: %w", ErrAttestation, err)
		}
		return nil, fmt.Errorf("%w: lookup: %w", ErrAttestation, err)
	}
	if record == nil {
		return nil, fmt.Errorf("%w: store returned nil record", ErrAttestation)
	}

	// Constant-time hash comparison guards against timing side
	// channels — never reveal which byte first mismatched.
	want := record.Hash
	got := saltedHash(record.Salt, token)
	if subtle.ConstantTimeCompare(want, got) != 1 {
		return nil, fmt.Errorf("%w: token hash mismatch", ErrAttestation)
	}

	now := a.clock()
	if !record.ExpiresAt.IsZero() && !now.Before(record.ExpiresAt) {
		return nil, fmt.Errorf("%w: %w", ErrAttestation, ErrJoinTokenExpired)
	}
	if record.MaxUses > 0 && record.UsedCount >= record.MaxUses {
		return nil, fmt.Errorf("%w: %w", ErrAttestation, ErrJoinTokenExhausted)
	}
	if record.AgentID == "" {
		// v0.1 rejection — see the v0.x ROADMAP "Join-token
		// any-agent mode" entry for the planned lift.
		return nil, fmt.Errorf("%w: token has no AgentID; any-agent tokens are v0.x scope", ErrAttestation)
	}

	if err := a.store.MarkUsed(ctx, record.ID, now); err != nil {
		return nil, fmt.Errorf("%w: mark used: %w", ErrAttestation, err)
	}

	id, err := AgentID(a.trustDomain, record.AgentID)
	if err != nil {
		return nil, fmt.Errorf("%w: build agent id: %v", ErrAttestation, err)
	}
	return &AttestResult{
		ID:        id,
		Selectors: buildSelectors(record),
	}, nil
}

// saltedHash returns sha256(salt || token) — the canonical record
// hash. The salt is per-token so a token leak doesn't compromise
// any other record even if a future store collision exposes
// cleartext bytes.
func saltedHash(salt []byte, token string) []byte {
	h := sha256.New()
	h.Write(salt)
	h.Write([]byte(token))
	return h.Sum(nil)
}

// buildSelectors emits the SPIFFE-style selectors for an attested
// agent. Sorted deterministically so the result is stable for
// audit-log diffing + downstream caching. Format `<key>:<value>`
// per SPIFFE convention.
func buildSelectors(record *JoinToken) []string {
	out := make([]string, 0, 2+len(record.Metadata))
	out = append(out, "join_token:"+record.Prefix)
	out = append(out, "agent:"+record.AgentID)
	for k, v := range record.Metadata {
		out = append(out, k+":"+v)
	}
	sort.Strings(out)
	return out
}
