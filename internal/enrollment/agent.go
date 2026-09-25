package enrollment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"time"

	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nats.go"

	"go.keystone-core.io/keystone-core/internal/config"
	"go.keystone-core.io/keystone-core/internal/ledger"
	"go.keystone-core.io/keystone-core/internal/protocol"
)

// The agent's outcomes. Each maps to one of the charter's exit codes.
var (
	// ErrRefused: the token is spent, expired or invalid, or the bootstrap
	// credential expired before S6 confirmed. Exit 10 (D-C05-3).
	ErrRefused = errors.New("enrollment refused: the token is spent, expired or invalid")
	// ErrBundle: the bundle itself is unusable. Exit 1.
	ErrBundle = errors.New("enrollment: the token bundle is not a valid bundle")
)

// Timing of the agent's retries. Enrollment is core NATS, so a lost message is
// repeated rather than recovered (ADR-0003 § 3).
const (
	replyWait     = 3 * time.Second
	confirmPeriod = time.Second
)

var (
	token64 = regexp.MustCompile(`^[0-9a-f]{64}$`)
	agent32 = regexp.MustCompile(`^[0-9a-f]{32}$`)
)

// ParseBundle reads a version-1 bundle and the keys it carries.
func ParseBundle(b []byte) (Bundle, *protocol.VerifyingKey, error) {
	var bundle Bundle
	if err := strictJSON(b, &bundle); err != nil {
		return Bundle{}, nil, fmt.Errorf("%w: %v", ErrBundle, err)
	}
	if bundle.Version != BundleVersion || !token64.MatchString(bundle.TokenID) || !agent32.MatchString(bundle.AgentID) {
		return Bundle{}, nil, ErrBundle
	}
	raw, err := DecodeKey(bundle.ServiceSigningPublicKey)
	if err != nil {
		return Bundle{}, nil, ErrBundle
	}
	service, err := protocol.ParseVerifyingKey(raw)
	if err != nil {
		return Bundle{}, nil, ErrBundle
	}
	raw, err = DecodeKey(bundle.ResultEncryptionPublicKey)
	if err != nil {
		return Bundle{}, nil, ErrBundle
	}
	if _, err := protocol.ParseRecipientKey(raw); err != nil {
		return Bundle{}, nil, ErrBundle
	}
	if _, err := jwt.ParseDecoratedJWT([]byte(bundle.BootstrapCredentials)); err != nil {
		return Bundle{}, nil, ErrBundle
	}
	return bundle, service, nil
}

// Agent is one enrollment run.
type Agent struct {
	cfg     config.Agent
	ca      []byte
	bundle  Bundle
	service *protocol.VerifyingKey
	expires time.Time
	now     func() time.Time
}

// NewAgent checks the bundle and configuration before anything is written or
// sent.
func NewAgent(cfg config.Agent, bundleBytes []byte) (*Agent, error) {
	bundle, service, err := ParseBundle(bundleBytes)
	if err != nil {
		return nil, err
	}
	token, _ := jwt.ParseDecoratedJWT([]byte(bundle.BootstrapCredentials))
	claims, err := jwt.DecodeUserClaims(token)
	if err != nil || claims.Expires == 0 {
		return nil, ErrBundle
	}
	ca, err := os.ReadFile(cfg.NATS.CAFile)
	if err != nil {
		return nil, err
	}
	return &Agent{cfg: cfg, ca: ca, bundle: bundle, service: service,
		expires: time.Unix(claims.Expires, 0), now: time.Now}, nil
}

// Enroll runs S0 to S6 from wherever a previous run stopped (ADR-0003 § 7),
// and returns nil only once S6 has confirmed the identity active and the
// token spent.
func (a *Agent) Enroll(ctx context.Context) error {
	ctx, cancel := context.WithDeadline(ctx, a.expires)
	defer cancel()
	keys, err := a.keys()
	if err != nil {
		return err
	}
	boot, replies, err := a.bootstrap(ctx)
	if err != nil {
		return err
	}
	defer boot.Close()

	identity, err := ReadIdentityArtifact(a.cfg.Agent.IdentityPath, keys)
	if errors.Is(err, os.ErrNotExist) {
		if identity, err = a.credential(ctx, boot, replies, keys); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else if identity.AgentID != a.bundle.AgentID {
		return fmt.Errorf("%w: the identity artifact is another agent's", ErrArtifact)
	}

	perm, err := a.permanent(identity)
	if err != nil {
		return err
	}
	defer perm.Close()
	return a.confirm(ctx, boot, replies, perm, keys)
}

// keys returns the key artifact for this bundle's agent, creating it -- and
// making it durable -- before the first request is sent (KEY-2).
func (a *Agent) keys() (AgentKeys, error) {
	k, err := ReadKeyArtifact(a.cfg.Agent.PrivateKeysPath, a.bundle.AgentID)
	if !errors.Is(err, os.ErrNotExist) {
		return k, err
	}
	fresh, err := GenerateAgentKeys()
	if err != nil {
		return AgentKeys{}, err
	}
	return WriteKeyArtifact(a.cfg.Agent.PrivateKeysPath, a.bundle.AgentID, fresh)
}

func (a *Agent) connect(creds []byte, name string) (*nats.Conn, error) {
	opts, err := ClientOptions(a.ca, a.cfg.NATS.ServerName, creds)
	if err != nil {
		return nil, err
	}
	return nats.Connect(a.cfg.NATS.URL, append(opts, nats.Name(name), nats.MaxReconnects(-1))...)
}

// bootstrap connects with the bundle's credential and subscribes to the token's
// reply subject before anything is published on it.
func (a *Agent) bootstrap(ctx context.Context) (*nats.Conn, chan *nats.Msg, error) {
	c, err := a.connect([]byte(a.bundle.BootstrapCredentials), "keystone-agent bootstrap")
	if err != nil {
		if errors.Is(err, nats.ErrAuthorization) || errors.Is(err, nats.ErrAuthExpired) {
			return nil, nil, ErrRefused
		}
		return nil, nil, err
	}
	replies := make(chan *nats.Msg, 16)
	if _, err := c.ChanSubscribe(ReplySubject(a.bundle.TokenID), replies); err != nil {
		c.Close()
		return nil, nil, err
	}
	if err := c.Flush(); err != nil {
		c.Close()
		return nil, nil, err
	}
	return c, replies, nil
}

func (a *Agent) request(boot *nats.Conn, keys AgentKeys, kind string) error {
	payload, err := json.Marshal(Request{
		NATSPublicKey:       keys.NATSPublicKey(),
		SigningPublicKey:    EncodeKey(keys.Signing.Verifying().Bytes()),
		EncryptionPublicKey: EncodeKey(keys.Decryption.Recipient().Bytes()),
		Kind:                kind,
	})
	if err != nil {
		return err
	}
	b, err := Seal(keys.Signing, protocol.ClassEnrollmentRequest, a.bundle.AgentID, a.bundle.TokenID, payload, a.now())
	if err != nil {
		return err
	}
	msg := nats.NewMsg(RequestSubject(a.bundle.TokenID))
	msg.Data = b
	msg.Header.Set(protocol.HeaderVersion, fmt.Sprint(protocol.Version))
	msg.Header.Set(protocol.HeaderClass, string(protocol.ClassEnrollmentRequest))
	return boot.PublishMsg(msg)
}

// await returns the next reply the service signed for this token and agent.
// Anything else on the subject is ignored.
func (a *Agent) await(ctx context.Context, replies chan *nats.Msg, wait time.Duration) (*Reply, error) {
	timer := time.NewTimer(wait)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, ErrRefused
		case <-timer.C:
			return nil, nil
		case m := <-replies:
			e, err := OpenEnvelope(m.Data, protocol.ClassEnrollmentReply, protocol.SenderEnrollmentService)
			if err != nil || e.CorrelationID != a.bundle.TokenID || Verify(a.service, e) != nil {
				continue
			}
			var r Reply
			if err := strictJSON(e.Payload, &r); err != nil {
				continue
			}
			if r.Kind == KindDenied {
				return nil, ErrRefused
			}
			if r.AgentID != a.bundle.AgentID {
				continue
			}
			return &r, nil
		}
	}
}

// credential is S1 and S2: request until the service answers, check the JWT
// names this agent's key and identifier, and write the identity artifact and
// the ledger.
func (a *Agent) credential(ctx context.Context, boot *nats.Conn, replies chan *nats.Msg, keys AgentKeys) (IdentityArtifact, error) {
	for {
		if err := a.request(boot, keys, KindCredential); err != nil {
			return IdentityArtifact{}, err
		}
		r, err := a.await(ctx, replies, replyWait)
		if err != nil {
			return IdentityArtifact{}, err
		}
		if r == nil || r.Kind != KindCredential {
			continue
		}
		claims, err := jwt.DecodeUserClaims(r.PermanentJWT)
		if err != nil || claims.Subject != keys.NATSPublicKey() || claims.Name != a.bundle.AgentID {
			return IdentityArtifact{}, fmt.Errorf("enrollment: the service returned a credential for another identity")
		}
		seed, err := keys.NATS.Seed()
		if err != nil {
			return IdentityArtifact{}, err
		}
		creds, err := jwt.FormatUserConfig(r.PermanentJWT, seed)
		if err != nil {
			return IdentityArtifact{}, err
		}
		identity := IdentityArtifact{
			Version: 1, AgentID: a.bundle.AgentID, PermanentCredentials: string(creds),
			AgentKeySHA256:            keys.Digest,
			ServiceSigningPublicKey:   a.bundle.ServiceSigningPublicKey,
			ResultEncryptionPublicKey: a.bundle.ResultEncryptionPublicKey,
		}
		b, err := json.Marshal(identity)
		if err != nil {
			return IdentityArtifact{}, err
		}
		if err := WriteAtomic(a.cfg.Agent.IdentityPath, b); err != nil {
			return IdentityArtifact{}, err
		}
		// The ledger exists before S3 can lead to S4, so a command that arrives
		// the moment the identity is active has somewhere to be recorded
		// (ARCH-JOB-002, LEDGER-1).
		l, err := ledger.Open(a.cfg.Agent.LedgerPath)
		if err != nil {
			return IdentityArtifact{}, err
		}
		if err := l.Close(); err != nil {
			return IdentityArtifact{}, err
		}
		return identity, nil
	}
}

// permanent is S3's connection, with the credential S2 wrote.
func (a *Agent) permanent(identity IdentityArtifact) (*nats.Conn, error) {
	return a.connect([]byte(identity.PermanentCredentials), "keystone-agent")
}

// Presence publishes one signed presence envelope: S3's proof. Its payload is
// empty; presence's content is C10's.
func Presence(c *nats.Conn, agentID string, k *protocol.SigningKey, now time.Time) error {
	b, err := Seal(k, protocol.ClassPresence, agentID, "", []byte("{}"), now)
	if err != nil {
		return err
	}
	msg := nats.NewMsg(PresenceSubject(agentID))
	msg.Data = b
	msg.Header.Set(protocol.HeaderVersion, fmt.Sprint(protocol.Version))
	msg.Header.Set(protocol.HeaderClass, string(protocol.ClassPresence))
	if err := c.PublishMsg(msg); err != nil {
		return err
	}
	return c.Flush()
}

// confirm is S3 to S6: prove the permanent identity, and ask on the bootstrap
// connection until the service confirms it active and the token spent. Proof
// and request are both repeated: each is idempotent, and core NATS loses what
// no one was subscribed to hear.
func (a *Agent) confirm(ctx context.Context, boot *nats.Conn, replies chan *nats.Msg, perm *nats.Conn, keys AgentKeys) error {
	for {
		if err := Presence(perm, a.bundle.AgentID, keys.Signing, a.now()); err != nil {
			return err
		}
		if err := a.request(boot, keys, KindConfirmation); err != nil {
			return err
		}
		r, err := a.await(ctx, replies, confirmPeriod)
		if err != nil {
			return err
		}
		if r != nil && r.Kind == KindConfirmation && r.Identity == StateActive && r.Token == StateSpent {
			return nil
		}
	}
}
