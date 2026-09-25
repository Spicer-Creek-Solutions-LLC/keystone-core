package enrollment

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"time"
	"unicode"
	"unicode/utf8"

	"go.keystone-core.io/keystone-core/internal/natsauth"
	"go.keystone-core.io/keystone-core/internal/store"
)

// Token lifetimes, D-C05-9: fifteen minutes unless the operator asks for
// another, and never under one minute or an hour or more. The ceiling keeps the
// window RSK-15 accepts to minutes.
const (
	DefaultTokenTTL = 15 * time.Minute
	MinTokenTTL     = time.Minute
	MaxTokenTTL     = 59 * time.Minute

	// MaxAgentName bounds the operator's label, which is stored and shown and
	// never used as an identifier (D-C05-2).
	MaxAgentName = 128

	// BundleVersion is the token bundle's format.
	BundleVersion = 1

	// ActionTokenIssued is the audit action S0 records.
	ActionTokenIssued = "enrollment.token.issued"
)

// ErrInvalidRequest is returned for an issuance request whose name or lifetime
// is outside what the service accepts.
var ErrInvalidRequest = errors.New("enrollment: invalid token request")

// Bundle is the version-1 token bundle: everything the agent needs and nothing
// it could be told out of band instead. The broker's address and CA are
// deployment configuration, not bundle content (C05-A evidence).
type Bundle struct {
	Version                   int    `json:"version"`
	TokenID                   string `json:"token_id"`
	AgentID                   string `json:"agent_id"`
	BootstrapCredentials      string `json:"bootstrap_credentials"`
	ServiceSigningPublicKey   string `json:"service_signing_public_key"`
	ResultEncryptionPublicKey string `json:"result_encryption_public_key"`
}

// Actor is who asked, as the operator socket established it from the kernel.
type Actor struct {
	UID              uint32
	UsernameSnapshot *string
}

// Issuer is S0: token creation.
type Issuer struct {
	store  *store.Store
	minter *natsauth.Minter
	keys   ServiceKeys
	now    func() time.Time
	random io.Reader
}

func NewIssuer(st *store.Store, minter *natsauth.Minter, keys ServiceKeys) *Issuer {
	return &Issuer{store: st, minter: minter, keys: keys, now: time.Now, random: rand.Reader}
}

// TokenTTL resolves a requested lifetime: zero is the default, anything else
// must lie within the bounds.
func TokenTTL(requested time.Duration) (time.Duration, error) {
	switch {
	case requested == 0:
		return DefaultTokenTTL, nil
	case requested < MinTokenTTL || requested > MaxTokenTTL:
		return 0, fmt.Errorf("%w: lifetime %v is outside %v to %v", ErrInvalidRequest, requested, MinTokenTTL, MaxTokenTTL)
	}
	return requested, nil
}

// ValidAgentName reports whether a label is acceptable: present, bounded, and
// printable, since it is shown to operators.
func ValidAgentName(name string) bool {
	if name == "" || len(name) > MaxAgentName || !utf8.ValidString(name) {
		return false
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// Issue creates one token and its bootstrap identity, records both durably, and
// only then returns the bundle. The agent identifier is assigned here, from
// fresh randomness, and never derived from the name (D-C05-2).
func (i *Issuer) Issue(ctx context.Context, actor Actor, agentName string, ttl time.Duration) (Bundle, error) {
	if !ValidAgentName(agentName) {
		return Bundle{}, fmt.Errorf("%w: agent name", ErrInvalidRequest)
	}
	ttl, err := TokenTTL(ttl)
	if err != nil {
		return Bundle{}, err
	}
	tokenID, err := i.randomHex(32)
	if err != nil {
		return Bundle{}, err
	}
	agentID, err := i.randomHex(16)
	if err != nil {
		return Bundle{}, err
	}
	if err := natsauth.ValidateAgentIdentifier(agentID); err != nil {
		return Bundle{}, err
	}
	issued := i.now()
	// JWT expiry has one-second resolution; the record uses the same instant so
	// the token and the credential end together.
	expires := issued.Add(ttl).Truncate(time.Second)
	identity, err := i.minter.Bootstrap(tokenID, expires)
	if err != nil {
		return Bundle{}, err
	}
	creds, err := natsauth.Credentials(identity)
	if err != nil {
		return Bundle{}, err
	}
	target := agentID
	if err := i.store.IssueEnrollment(ctx, store.EnrollmentToken{
		TokenID: tokenID, AgentID: agentID, AgentName: agentName,
		BootstrapPublicKey: identity.PublicKey,
		IssuedAt:           issued, ExpiresAt: expires, IssuedByUID: actor.UID,
	}, store.AuditRecord{
		ActorUID: &actor.UID, ActorUsernameSnapshot: actor.UsernameSnapshot, Target: &target,
		Action: ActionTokenIssued, Result: "issued", At: issued,
	}); err != nil {
		return Bundle{}, err
	}
	return Bundle{
		Version:                   BundleVersion,
		TokenID:                   tokenID,
		AgentID:                   agentID,
		BootstrapCredentials:      string(creds),
		ServiceSigningPublicKey:   EncodeKey(i.keys.Signing.Verifying().Bytes()),
		ResultEncryptionPublicKey: EncodeKey(i.keys.ResultRecipient.Bytes()),
	}, nil
}

func (i *Issuer) randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := io.ReadFull(i.random, b); err != nil {
		return "", fmt.Errorf("enrollment: randomness: %w", err)
	}
	return hex.EncodeToString(b), nil
}
