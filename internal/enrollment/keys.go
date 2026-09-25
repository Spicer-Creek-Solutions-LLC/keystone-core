package enrollment

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"go.keystone-core.io/keystone-core/internal/protocol"
)

// ServiceKeys are the two service keys the server holds for enrollment: the
// envelope-signing private key it signs replies with (AST-7), and the result
// service's encryption public half (AST-8). Both public halves go into every
// token bundle, which is how an agent comes to trust them (ADR-0003 § 1).
type ServiceKeys struct {
	Signing         *protocol.SigningKey
	ResultRecipient *protocol.RecipientKey
}

// ErrKeyFilePermissions is returned for a signing-key file that anyone but its
// owner can read or write. Owner-only modes such as 0400 and 0600 are accepted
// (SKEY-1).
var ErrKeyFilePermissions = errors.New("enrollment: the service signing private-key file must not be accessible to group or other")

// ErrKeyFile is returned for a key file whose content is not a key. It wraps
// the protocol's refusal, which on its own says only "signature invalid" -- true
// of a signature, and no help to an operator looking at a damaged file.
var ErrKeyFile = errors.New("enrollment: key file does not contain a valid key")

// maxKeyFile bounds what is read from a key file. The encoded public half is
// under 2 KiB; anything much larger is not a key.
const maxKeyFile = 16 << 10

// LoadServiceKeys reads both key files. Each holds one line of standard base64:
// the signing key in protocol.MarshalSigningKey's form, the result key in its
// wire form.
func LoadServiceKeys(signingPath, resultPath string) (ServiceKeys, error) {
	raw, err := readKeyFile(signingPath, true)
	if err != nil {
		return ServiceKeys{}, err
	}
	signing, err := protocol.ParseSigningKey(raw)
	if err != nil {
		return ServiceKeys{}, fmt.Errorf("%w: %s is not a service signing private key: %v", ErrKeyFile, signingPath, err)
	}
	raw, err = readKeyFile(resultPath, false)
	if err != nil {
		return ServiceKeys{}, err
	}
	result, err := protocol.ParseRecipientKey(raw)
	if err != nil {
		return ServiceKeys{}, fmt.Errorf("%w: %s is not a result-encryption public key: %v", ErrKeyFile, resultPath, err)
	}
	return ServiceKeys{Signing: signing, ResultRecipient: result}, nil
}

// readKeyFile checks the mode of the file it opened rather than of the path, so
// a file swapped between the check and the read cannot pass one and supply the
// other.
func readKeyFile(path string, private bool) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: %s is not a regular file", ErrKeyFile, path)
	}
	if private && info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("%w: %s is mode %04o", ErrKeyFilePermissions, path, info.Mode().Perm())
	}
	b, err := io.ReadAll(io.LimitReader(f, maxKeyFile+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxKeyFile {
		return nil, fmt.Errorf("%w: %s is larger than any key", ErrKeyFile, path)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(b)))
	if err != nil {
		return nil, fmt.Errorf("%w: %s is not base64", ErrKeyFile, path)
	}
	return raw, nil
}

// EncodeKey is the one encoding every key file and bundle field uses.
func EncodeKey(b []byte) string { return base64.StdEncoding.EncodeToString(b) }
