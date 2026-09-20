package natsauth

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"

	"go.keystone-core.io/keystone-core/internal/protocol"
)

// ErrIdentifier is returned for an agent identifier the subject grammar refuses
// or that is reserved for a service principal.
var ErrIdentifier = errors.New("identifier is not usable as a subject token")

// Config is what a deployment supplies. Agents and Tokens are the identities
// that exist at generation time; C05 mints further agent identities against the
// account signing key as enrollment completes.
type Config struct {
	FleetSize int64
	Agents    []string
	Tokens    []string

	// RevokedTokens name bootstrap identities whose access has ended. ADR-0003
	// § 6 S5 revokes at the account, which is why this is generated here rather
	// than expressed as an absent user.
	RevokedTokens []string

	// BootstrapTTL is ADR-0003 § 8's short expiry, the failure backstop.
	BootstrapTTL time.Duration

	// Limits defaults to DefaultLimits(FleetSize) when nil.
	Limits *Limits
}

// Identity is one NATS principal: its key pair, its JWT, and the permission
// lists that JWT carries. The lists are kept beside the JWT because GEN-3 and
// GEN-4 read the JWT, and a reader comparing the two needs no decoder.
type Identity struct {
	Name      string
	PublicKey string
	Seed      []byte
	JWT       string
	Pub       []string
	Sub       []string
}

// Account is the Keystone or system account. SigningSeed is the account signing
// key ADR-0002 § 2 places with the server; the operator seed is not here and
// never is.
type Account struct {
	Name        string
	PublicKey   string
	JWT         string
	Seed        []byte
	SigningSeed []byte
	SigningKey  string
}

// Deployment is everything generation produces. OperatorSeed is returned to the
// CALLER and is never written by Write: ADR-0002 § 2 holds it outside every
// Keystone process, and GEN-7 is the case that checks the artifacts agree.
type Deployment struct {
	OperatorSeed      []byte
	OperatorPublicKey string
	OperatorJWT       string

	System   Account
	Keystone Account

	Services   map[Principal]Identity
	Agents     map[string]Identity
	Bootstraps map[string]Identity

	Revoked []string
	Limits  Limits
}

// Generate produces a complete authorization set. It creates keys, so two calls
// never agree; nothing in it is derived from a committed fixture.
func Generate(cfg Config) (*Deployment, error) {
	for _, id := range cfg.Agents {
		if err := ValidateAgentIdentifier(id); err != nil {
			return nil, err
		}
	}
	for _, token := range cfg.Tokens {
		if err := ValidateAgentIdentifier(token); err != nil {
			return nil, fmt.Errorf("enrollment token %q: %w", token, err)
		}
	}
	if cfg.BootstrapTTL <= 0 {
		cfg.BootstrapTTL = 15 * time.Minute
	}
	limits := DefaultLimits(cfg.FleetSize)
	if cfg.Limits != nil {
		limits = *cfg.Limits
	}

	operator, err := nkeys.CreateOperator()
	if err != nil {
		return nil, fmt.Errorf("create operator: %w", err)
	}
	operatorPub, err := operator.PublicKey()
	if err != nil {
		return nil, fmt.Errorf("operator public key: %w", err)
	}
	operatorSeed, err := operator.Seed()
	if err != nil {
		return nil, fmt.Errorf("operator seed: %w", err)
	}

	system, err := newAccount("SYS", operator, nil)
	if err != nil {
		return nil, fmt.Errorf("system account: %w", err)
	}
	keystone, err := newAccount("KEYSTONE", operator, &limits)
	if err != nil {
		return nil, fmt.Errorf("keystone account: %w", err)
	}

	operatorClaims := jwt.NewOperatorClaims(operatorPub)
	operatorClaims.Name = "keystone"
	operatorClaims.SystemAccount = system.PublicKey
	operatorJWT, err := operatorClaims.Encode(operator)
	if err != nil {
		return nil, fmt.Errorf("encode operator: %w", err)
	}

	d := &Deployment{
		OperatorSeed:      operatorSeed,
		OperatorPublicKey: operatorPub,
		OperatorJWT:       operatorJWT,
		System:            system,
		Keystone:          keystone,
		Services:          map[Principal]Identity{},
		Agents:            map[string]Identity{},
		Bootstraps:        map[string]Identity{},
		Limits:            limits,
	}

	signing, err := nkeys.FromSeed(keystone.SigningSeed)
	if err != nil {
		return nil, fmt.Errorf("account signing key: %w", err)
	}

	for _, p := range Services() {
		identity, err := newUser(string(p), serviceGrants(p), keystone, signing, 0)
		if err != nil {
			return nil, fmt.Errorf("service %s: %w", p, err)
		}
		d.Services[p] = identity
	}
	for _, id := range cfg.Agents {
		identity, err := newUser(id, agentGrants(id), keystone, signing, 0)
		if err != nil {
			return nil, fmt.Errorf("agent %s: %w", id, err)
		}
		d.Agents[id] = identity
	}
	for _, token := range cfg.Tokens {
		identity, err := newUser(token, bootstrapGrants(token), keystone, signing, cfg.BootstrapTTL)
		if err != nil {
			return nil, fmt.Errorf("bootstrap %s: %w", token, err)
		}
		d.Bootstraps[token] = identity
	}

	if err := d.revoke(cfg.RevokedTokens, operator); err != nil {
		return nil, err
	}
	return d, nil
}

// ValidateAgentIdentifier refuses what ADR-0004 § 1's grammar refuses and what
// ADR-0005 § 3 reserves.
//
// It delegates rather than restating: internal/protocol already implements the
// grammar and holds the reserved-token list, and a second copy here would be a
// second thing to drift. WHERE the identifier comes from is not settled --
// ADR-0004 § 1 requires it server-assigned and ADR-0003 establishes no such
// identifier -- and C03 consumes one rather than deciding that.
func ValidateAgentIdentifier(id string) error {
	if !protocol.ValidIdentifier(id) {
		return fmt.Errorf("%q: %w", id, ErrIdentifier)
	}
	if protocol.ReservedSender(id) {
		return fmt.Errorf("%q is reserved for a service principal: %w", id, ErrIdentifier)
	}
	return nil
}

func newAccount(name string, operator nkeys.KeyPair, limits *Limits) (Account, error) {
	kp, err := nkeys.CreateAccount()
	if err != nil {
		return Account{}, err
	}
	pub, err := kp.PublicKey()
	if err != nil {
		return Account{}, err
	}
	seed, err := kp.Seed()
	if err != nil {
		return Account{}, err
	}
	signing, err := nkeys.CreateAccount()
	if err != nil {
		return Account{}, err
	}
	signingPub, err := signing.PublicKey()
	if err != nil {
		return Account{}, err
	}
	signingSeed, err := signing.Seed()
	if err != nil {
		return Account{}, err
	}

	claims := jwt.NewAccountClaims(pub)
	claims.Name = name
	claims.SigningKeys.Add(signingPub)
	if limits != nil {
		applyAccountLimits(claims, *limits)
	}
	encoded, err := claims.Encode(operator)
	if err != nil {
		return Account{}, err
	}
	return Account{
		Name:        name,
		PublicKey:   pub,
		JWT:         encoded,
		Seed:        seed,
		SigningSeed: signingSeed,
		SigningKey:  signingPub,
	}, nil
}

func applyAccountLimits(claims *jwt.AccountClaims, limits Limits) {
	claims.Limits.Conn = limits.Account.MaxConnections
	claims.Limits.Subs = limits.Account.MaxSubscriptions
	claims.Limits.Payload = limits.Account.MaxPayloadBytes
	claims.Limits.DiskStorage = limits.Account.MaxJetStreamDiskByte
	claims.Limits.MemoryStorage = 0
	// Data is the account's total byte budget; JetStream storage is bounded
	// separately above and ADR-0002 § 9 requires neither to be infinite.
	claims.Limits.Data = limits.Account.MaxJetStreamDiskByte
}

func newUser(name string, g grants, account Account, signing nkeys.KeyPair, ttl time.Duration) (Identity, error) {
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
	claims.IssuerAccount = account.PublicKey
	claims.Permissions.Pub.Allow = append(jwt.StringList{}, g.pub...)
	claims.Permissions.Sub.Allow = append(jwt.StringList{}, g.sub...)
	if ttl > 0 {
		claims.Expires = time.Now().Add(ttl).Unix()
	}
	encoded, err := claims.Encode(signing)
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

// revoke re-encodes the Keystone account with each named bootstrap identity in
// its revocation list. ADR-0003 § 6 S5 revokes at the account, so the account
// JWT is the artifact that changes.
func (d *Deployment) revoke(tokens []string, operator nkeys.KeyPair) error {
	if len(tokens) == 0 {
		return nil
	}
	claims, err := jwt.DecodeAccountClaims(d.Keystone.JWT)
	if err != nil {
		return fmt.Errorf("decode keystone account: %w", err)
	}
	for _, token := range tokens {
		identity, ok := d.Bootstraps[token]
		if !ok {
			return fmt.Errorf("revoke %q: no bootstrap identity was generated for it", token)
		}
		claims.Revoke(identity.PublicKey)
		d.Revoked = append(d.Revoked, identity.PublicKey)
	}
	sort.Strings(d.Revoked)
	encoded, err := claims.Encode(operator)
	if err != nil {
		return fmt.Errorf("re-encode keystone account: %w", err)
	}
	d.Keystone.JWT = encoded
	return nil
}
