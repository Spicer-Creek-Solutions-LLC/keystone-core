package enrollment

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"go.keystone-core.io/keystone-core/internal/protocol"
)

// The enrollment payloads, inside ADR-0005's enrollment-request and
// enrollment-reply classes. Their shapes are C05-A's frozen surface; the
// encodings of the key fields are C05's (C05.md § 3.3), and are the bundle's:
// standard base64 of the wire form.
const (
	KindCredential   = "credential"
	KindConfirmation = "confirmation"

	StateActive = "active"
	StateSpent  = "spent"

	// MaxEnvelope bounds an enrollment or presence envelope. ADR-0005 § 7
	// calls for a small fixed bound on all three classes; the largest is a
	// request, whose two hybrid public halves and signature are under 9 KiB.
	MaxEnvelope = 16 << 10

	// PresenceStaleness is how old a presence envelope may be and still count
	// as S3's proof (ADR-0005 § 6). ADR-0003 § 12 assumes the clocks agree to
	// within about a minute.
	PresenceStaleness = 2 * time.Minute
)

// Request is the enrollment request payload.
type Request struct {
	NATSPublicKey       string `json:"nats_public_key"`
	SigningPublicKey    string `json:"signing_public_key"`
	EncryptionPublicKey string `json:"encryption_public_key"`
	Kind                string `json:"kind"`
}

// Reply is the enrollment reply payload.
type Reply struct {
	Kind         string `json:"kind"`
	AgentID      string `json:"agent_id"`
	PermanentJWT string `json:"permanent_jwt,omitempty"`
	Identity     string `json:"identity_state,omitempty"`
	Token        string `json:"token_state,omitempty"`
}

// Subjects, ADR-0004 § 1.
func RequestSubject(token string) string  { return "ks.enroll." + token + ".request" }
func ReplySubject(token string) string    { return "ks.enroll." + token + ".reply" }
func PresenceSubject(agent string) string { return "ks.out." + agent + ".presence" }

// RequestSubjects and PresenceSubjects are what the two service principals
// subscribe to; their grants are exactly these.
const (
	RequestSubjects  = "ks.enroll.*.request"
	PresenceSubjects = "ks.out.*.presence"
)

// Seal builds and signs an envelope of a class without an encryption
// recipient. The correlation identifier of an enrollment envelope is its token
// identifier: the replay key ADR-0005 § 6 gives the class.
func Seal(k *protocol.SigningKey, class protocol.Class, sender, correlation string, payload []byte, now time.Time) ([]byte, error) {
	nonce, err := protocol.NewNonce()
	if err != nil {
		return nil, err
	}
	e, err := protocol.SignEnvelope(k, protocol.Envelope{
		Version: protocol.Version, Class: class, CorrelationID: correlation,
		Sender: sender, Timestamp: now, Nonce: nonce, Payload: payload,
	})
	if err != nil {
		return nil, err
	}
	b, ok := e.Encode()
	if !ok {
		return nil, errors.New("enrollment: envelope does not frame")
	}
	return b, nil
}

// ErrEnvelope is any envelope this side refuses: malformed, the wrong class,
// an unexpected sender, or a signature that does not verify. The reason is
// carried for the audit record and never for the peer.
var ErrEnvelope = errors.New("enrollment: envelope refused")

// OpenEnvelope decodes an envelope, checks it is of class and from sender, and returns
// it unverified: the caller verifies once it knows which key to verify with.
func OpenEnvelope(b []byte, class protocol.Class, sender string) (protocol.Envelope, error) {
	e, err := protocol.Decode(b, MaxEnvelope)
	if err != nil {
		return protocol.Envelope{}, fmt.Errorf("%w: %v", ErrEnvelope, err)
	}
	if e.Class != class || e.Sender != sender {
		return protocol.Envelope{}, fmt.Errorf("%w: class %q from %q", ErrEnvelope, e.Class, e.Sender)
	}
	return e, nil
}

// Verify checks an envelope's signature against a verifying key.
func Verify(v *protocol.VerifyingKey, e protocol.Envelope) error {
	in, ok := e.SignedInput()
	if !ok {
		return fmt.Errorf("%w: unframeable", ErrEnvelope)
	}
	if err := protocol.Verify(v, in, e.Signature); err != nil {
		return fmt.Errorf("%w: signature", ErrEnvelope)
	}
	return nil
}

// DecodeKey reverses EncodeKey.
func DecodeKey(s string) ([]byte, error) { return base64.StdEncoding.DecodeString(s) }

// strictJSON decodes exactly one object with no unknown members.
func strictJSON(b []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return err
	}
	if d.More() {
		return errors.New("trailing data")
	}
	return nil
}
