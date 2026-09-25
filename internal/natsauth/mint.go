package natsauth

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
)

// Minter issues user JWTs against the Keystone account's signing key. The
// generator and the running server both mint through it, so an identity issued
// at enrollment and one issued at generation cannot disagree about grants,
// issuer or expiry (C05.md § 3.3).
type Minter struct {
	account string
	signing nkeys.KeyPair
}

// NewMinter binds the account's public key to its signing seed. Only a signing
// seed is accepted: an account seed would also sign, and would put the key
// that re-encodes the account where ADR-0002 § 2 puts only a signing key.
func NewMinter(accountPublicKey string, signingSeed []byte) (*Minter, error) {
	if !nkeys.IsValidPublicAccountKey(accountPublicKey) {
		return nil, fmt.Errorf("natsauth: %q is not an account public key", accountPublicKey)
	}
	kp, err := nkeys.FromSeed(signingSeed)
	if err != nil {
		return nil, fmt.Errorf("natsauth: account signing seed: %w", err)
	}
	pub, err := kp.PublicKey()
	if err != nil {
		return nil, fmt.Errorf("natsauth: account signing key: %w", err)
	}
	if !nkeys.IsValidPublicAccountKey(pub) {
		return nil, errors.New("natsauth: the signing seed is not an account key")
	}
	if pub == accountPublicKey {
		return nil, errors.New("natsauth: the account's own seed is not a signing key")
	}
	return &Minter{account: accountPublicKey, signing: kp}, nil
}

// SigningKey is the public half of the key every minted JWT is issued by.
func (m *Minter) SigningKey() string {
	pub, _ := m.signing.PublicKey()
	return pub
}

// Account is the account every minted identity belongs to.
func (m *Minter) Account() string { return m.account }

// Bootstrap mints one enrollment token's identity: a fresh keypair, its own
// request and reply subjects and nothing else, expiring at expires. The expiry
// is how bootstrap access ends (RFC 0005), so a zero expiry is refused rather
// than minted as a credential that never expires.
func (m *Minter) Bootstrap(token string, expires time.Time) (Identity, error) {
	if err := ValidateAgentIdentifier(token); err != nil {
		return Identity{}, fmt.Errorf("enrollment token: %w", err)
	}
	if expires.IsZero() {
		return Identity{}, errors.New("natsauth: a bootstrap identity must expire")
	}
	return newUser(token, bootstrapGrants(token), m, expires)
}

// Agent issues a permanent agent JWT against a public key the AGENT generated.
// No seed exists on this side to return, and Identity.Seed stays nil for exactly
// that reason -- the absence is the property, not an omission.
func (m *Minter) Agent(agent AgentKey) (Identity, error) {
	if err := ValidateAgentIdentifier(agent.ID); err != nil {
		return Identity{}, err
	}
	if !nkeys.IsValidPublicUserKey(agent.PublicKey) {
		return Identity{}, fmt.Errorf("agent %q: %w", agent.ID, ErrAgentPublicKey)
	}
	g := agentGrants(agent.ID)
	claims := jwt.NewUserClaims(agent.PublicKey)
	claims.Name = agent.ID
	claims.IssuerAccount = m.account
	applyPermissions(claims, g)
	encoded, err := claims.Encode(m.signing)
	if err != nil {
		return Identity{}, err
	}
	return Identity{
		Name:      agent.ID,
		PublicKey: agent.PublicKey,
		JWT:       encoded,
		Pub:       append([]string{}, g.pub...),
		Sub:       append([]string{}, g.sub...),
	}, nil
}

// Credentials renders an identity as the decorated file a NATS client reads.
func Credentials(id Identity) ([]byte, error) {
	if len(id.Seed) == 0 {
		return nil, errors.New("natsauth: an identity without a seed has no credentials file on this side")
	}
	return jwt.FormatUserConfig(id.JWT, id.Seed)
}

func newUser(name string, g grants, m *Minter, expires time.Time) (Identity, error) {
	kp, err := nkeys.CreateUser()
	if err != nil {
		return Identity{}, err
	}
	pub, err := kp.PublicKey()
	if err != nil {
		return Identity{}, err
	}
	seed, err := kp.Seed()
	if err != nil {
		return Identity{}, err
	}

	claims := jwt.NewUserClaims(pub)
	claims.Name = name
	claims.IssuerAccount = m.account
	applyPermissions(claims, g)
	if !expires.IsZero() {
		claims.Expires = expires.Unix()
	}
	encoded, err := claims.Encode(m.signing)
	if err != nil {
		return Identity{}, err
	}
	return Identity{
		Name:      name,
		PublicKey: pub,
		Seed:      seed,
		JWT:       encoded,
		Pub:       append([]string{}, g.pub...),
		Sub:       append([]string{}, g.sub...),
	}, nil
}

// ErrWrongRole is returned for a credential that is not the service principal
// its configuration slot names.
var ErrWrongRole = errors.New("natsauth: credential is not the configured service principal")

// ServiceCredential is one service principal's credential, read from its
// decorated file and checked against the slot it was configured for.
type ServiceCredential struct {
	Principal Principal
	Account   string
	Issuer    string
	PublicKey string
	File      []byte
}

// ReadServiceCredential accepts a credentials file only if it is exactly the
// named principal: its name, and both permission directions, equal what the
// generator issues that principal, and its seed is the key its JWT names. A
// presence-consumer credential in the enrollment-service slot is refused rather
// than used with whatever it happens to be allowed (ROLE-1).
func ReadServiceCredential(p Principal, file []byte) (ServiceCredential, error) {
	token, err := jwt.ParseDecoratedJWT(file)
	if err != nil {
		return ServiceCredential{}, fmt.Errorf("%w: %s: %v", ErrWrongRole, p, err)
	}
	claims, err := jwt.DecodeUserClaims(token)
	if err != nil {
		return ServiceCredential{}, fmt.Errorf("%w: %s: %v", ErrWrongRole, p, err)
	}
	kp, err := jwt.ParseDecoratedNKey(file)
	if err != nil {
		return ServiceCredential{}, fmt.Errorf("%w: %s: %v", ErrWrongRole, p, err)
	}
	pub, err := kp.PublicKey()
	if err != nil || pub != claims.Subject {
		return ServiceCredential{}, fmt.Errorf("%w: %s: the seed is not the key the JWT names", ErrWrongRole, p)
	}
	want := serviceGrants(p)
	if claims.Name != string(p) || claims.IssuerAccount == "" ||
		!samePermission(claims.Permissions.Pub, permissionFor(want.pub)) ||
		!samePermission(claims.Permissions.Sub, permissionFor(want.sub)) {
		return ServiceCredential{}, fmt.Errorf("%w: %s", ErrWrongRole, p)
	}
	return ServiceCredential{
		Principal: p,
		Account:   claims.IssuerAccount,
		Issuer:    claims.Issuer,
		PublicKey: pub,
		File:      file,
	}, nil
}

func samePermission(got, want jwt.Permission) bool {
	return sameSet(got.Allow, want.Allow) && sameSet(got.Deny, want.Deny)
}

func sameSet(a, b jwt.StringList) bool {
	x, y := slices.Clone([]string(a)), slices.Clone([]string(b))
	slices.Sort(x)
	slices.Sort(y)
	return slices.Equal(x, y)
}
