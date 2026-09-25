package enrollment

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nats-io/nkeys"

	"go.keystone-core.io/keystone-core/internal/protocol"
)

// ArtifactMode is the mode of both agent artifacts: each holds a private key.
const ArtifactMode = 0o600

// KeyArtifact is the agent's private keys, written before S1 so a restart
// presents the same public halves (KEY-2). The private halves never leave the
// host (ADR-0003 § 4).
type KeyArtifact struct {
	Version              int    `json:"version"`
	AgentID              string `json:"agent_id"`
	NATSSeed             string `json:"nats_seed"`
	SigningPrivateKey    string `json:"signing_private_key"`
	DecryptionPrivateKey string `json:"decryption_private_key"`
}

// IdentityArtifact is S2's write: the permanent credential and the service
// trust halves together, bound to the exact key artifact bytes.
type IdentityArtifact struct {
	Version                   int    `json:"version"`
	AgentID                   string `json:"agent_id"`
	PermanentCredentials      string `json:"permanent_credentials"`
	AgentKeySHA256            string `json:"agent_key_sha256"`
	ServiceSigningPublicKey   string `json:"service_signing_public_key"`
	ResultEncryptionPublicKey string `json:"result_encryption_public_key"`
}

// AgentKeys are the three keys the agent generates on the host.
type AgentKeys struct {
	NATS       nkeys.KeyPair
	Signing    *protocol.SigningKey
	Decryption *protocol.DecryptionKey
	// Digest is the SHA-256 of the artifact bytes the keys were read from or
	// written as.
	Digest string
}

func (k AgentKeys) NATSPublicKey() string {
	pub, _ := k.NATS.PublicKey()
	return pub
}

// GenerateAgentKeys creates the three separate keys ARCH-NATS-006 requires.
func GenerateAgentKeys() (AgentKeys, error) {
	kp, err := nkeys.CreateUser()
	if err != nil {
		return AgentKeys{}, err
	}
	signing, err := protocol.GenerateSigningKey()
	if err != nil {
		return AgentKeys{}, err
	}
	decryption, err := protocol.GenerateDecryptionKey()
	if err != nil {
		return AgentKeys{}, err
	}
	return AgentKeys{NATS: kp, Signing: signing, Decryption: decryption}, nil
}

// WriteKeyArtifact persists the keys for agentID and returns them with the
// digest of the bytes written.
func WriteKeyArtifact(path, agentID string, k AgentKeys) (AgentKeys, error) {
	seed, err := k.NATS.Seed()
	if err != nil {
		return AgentKeys{}, err
	}
	b, err := json.Marshal(KeyArtifact{
		Version: 1, AgentID: agentID, NATSSeed: string(seed),
		SigningPrivateKey:    EncodeKey(protocol.MarshalSigningKey(k.Signing)),
		DecryptionPrivateKey: EncodeKey(protocol.MarshalDecryptionKey(k.Decryption)),
	})
	if err != nil {
		return AgentKeys{}, err
	}
	if err := WriteAtomic(path, b); err != nil {
		return AgentKeys{}, err
	}
	k.Digest = digest(b)
	return k, nil
}

// ErrArtifact is an artifact that exists and is not usable: another agent's,
// malformed, or readable by anyone but its owner.
var ErrArtifact = errors.New("enrollment: agent artifact unusable")

// ReadKeyArtifact restores the keys persisted for agentID.
func ReadKeyArtifact(path, agentID string) (AgentKeys, error) {
	b, err := readPrivate(path)
	if err != nil {
		return AgentKeys{}, err
	}
	var a KeyArtifact
	if err := strictJSON(b, &a); err != nil || a.Version != 1 || a.AgentID != agentID {
		return AgentKeys{}, fmt.Errorf("%w: %s is not this agent's key artifact", ErrArtifact, path)
	}
	kp, err := nkeys.FromSeed([]byte(a.NATSSeed))
	if err != nil {
		return AgentKeys{}, fmt.Errorf("%w: %s: NATS seed", ErrArtifact, path)
	}
	raw, err := DecodeKey(a.SigningPrivateKey)
	if err != nil {
		return AgentKeys{}, fmt.Errorf("%w: %s: signing key", ErrArtifact, path)
	}
	signing, err := protocol.ParseSigningKey(raw)
	if err != nil {
		return AgentKeys{}, fmt.Errorf("%w: %s: signing key", ErrArtifact, path)
	}
	raw, err = DecodeKey(a.DecryptionPrivateKey)
	if err != nil {
		return AgentKeys{}, fmt.Errorf("%w: %s: decryption key", ErrArtifact, path)
	}
	decryption, err := protocol.ParseDecryptionKey(raw)
	if err != nil {
		return AgentKeys{}, fmt.Errorf("%w: %s: decryption key", ErrArtifact, path)
	}
	return AgentKeys{NATS: kp, Signing: signing, Decryption: decryption, Digest: digest(b)}, nil
}

// ReadIdentityArtifact reads S2's artifact and checks it belongs to the keys it
// is to be used with.
func ReadIdentityArtifact(path string, keys AgentKeys) (IdentityArtifact, error) {
	b, err := readPrivate(path)
	if err != nil {
		return IdentityArtifact{}, err
	}
	var a IdentityArtifact
	if err := strictJSON(b, &a); err != nil || a.Version != 1 {
		return IdentityArtifact{}, fmt.Errorf("%w: %s is malformed", ErrArtifact, path)
	}
	if a.AgentKeySHA256 != keys.Digest {
		return IdentityArtifact{}, fmt.Errorf("%w: %s was written for other keys", ErrArtifact, path)
	}
	return a, nil
}

func readPrivate(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("%w: %s must be a regular file accessible only to its owner", ErrArtifact, path)
	}
	b := make([]byte, info.Size())
	if _, err := f.ReadAt(b, 0); err != nil && info.Size() > 0 {
		return nil, err
	}
	return b, nil
}

func digest(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// WriteAtomic makes b visible at path whole or not at all: a temporary file in
// the same directory, written and fsynced at mode 0600, renamed over path, and
// the directory fsynced so the rename survives a crash (ADR-0003 § 6, S2).
func WriteAtomic(path string, b []byte) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err := f.Chmod(ArtifactMode); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
