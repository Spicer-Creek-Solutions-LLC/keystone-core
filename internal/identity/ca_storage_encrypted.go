// SPDX-License-Identifier: Apache-2.0

package identity

import (
	"crypto"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"go.keystone-core.io/keystone-core/internal/masterkey"
)

// EncryptedFileCAStorage is a [CAStorage] that stores the CA private
// keys encrypted at rest, satisfying PROJECT-DETAILS §4.10's "optional
// encryption key" on persisted CA material. It is a drop-in for
// [FileCAStorage]: same directory, same filenames, same interface.
//
// Only the private-key files are encrypted — the cert files are public
// material and are written as ordinary PEM, identical to
// [FileCAStorage]:
//
//	<dir>/root-cert.pem      (CERTIFICATE,  plaintext, mode 0644)
//	<dir>/root-key.pem       (sealed CA envelope,      mode 0600)
//	<dir>/signing-cert.pem   (CERTIFICATE,  plaintext, mode 0644)
//	<dir>/signing-key.pem    (sealed CA envelope,      mode 0600)
//
// The key files are AES-256-GCM envelopes (see ca_envelope.go) sealed
// under a [masterkey.Key] resolved from operator config (env / file /
// inline) via the shared masterkey package. Loading verifies the
// envelope's key fingerprint before decrypting, so a wrong-key boot
// fails fast with a recognisable mismatch. A key file that is still
// plaintext PEM (a not-yet-migrated [FileCAStorage] deployment) is
// reported with a hint to run `kscore-identity ca encrypt`.
//
// Migrate an existing plaintext directory with that command, or
// construct fresh: the embedded provider's CAManager generates new CA
// material through this storage when none exists.
type EncryptedFileCAStorage struct {
	dir string
	key masterkey.Key
}

// NewEncryptedFileCAStorage returns an encrypted storage rooted at dir,
// sealing/opening key files with key. The directory is created with
// mode 0700 if absent. A zero-value key is rejected.
func NewEncryptedFileCAStorage(dir string, key masterkey.Key) (*EncryptedFileCAStorage, error) {
	if dir == "" {
		return nil, fmt.Errorf("%w: directory is required", ErrInvalidCAStorage)
	}
	if key.IsZero() {
		return nil, fmt.Errorf("%w: master key is required", ErrInvalidCAStorage)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("%w: mkdir %s: %v", ErrInvalidCAStorage, dir, err)
	}
	return &EncryptedFileCAStorage{dir: dir, key: key}, nil
}

// SaveRootCA implements [CAStorage].
func (s *EncryptedFileCAStorage) SaveRootCA(cert *x509.Certificate, key crypto.Signer) error {
	return s.savePair(rootCertFile, rootKeyFile, cert, key)
}

// LoadRootCA implements [CAStorage].
func (s *EncryptedFileCAStorage) LoadRootCA() (*x509.Certificate, crypto.Signer, error) {
	return s.loadPair(rootCertFile, rootKeyFile)
}

// HasRootCA implements [CAStorage].
func (s *EncryptedFileCAStorage) HasRootCA() (bool, error) {
	return s.hasPair(rootCertFile, rootKeyFile)
}

// SaveSigningCA implements [CAStorage].
func (s *EncryptedFileCAStorage) SaveSigningCA(cert *x509.Certificate, key crypto.Signer) error {
	return s.savePair(signingCertFile, signingKeyFile, cert, key)
}

// LoadSigningCA implements [CAStorage].
func (s *EncryptedFileCAStorage) LoadSigningCA() (*x509.Certificate, crypto.Signer, error) {
	return s.loadPair(signingCertFile, signingKeyFile)
}

// HasSigningCA implements [CAStorage].
func (s *EncryptedFileCAStorage) HasSigningCA() (bool, error) {
	return s.hasPair(signingCertFile, signingKeyFile)
}

func (s *EncryptedFileCAStorage) savePair(certName, keyName string, cert *x509.Certificate, key crypto.Signer) error {
	certPEM, keyPEM, err := marshalCAPair(cert, key)
	if err != nil {
		return err
	}
	sealed, err := sealCAKey(keyPEM, s.key)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(s.dir, certName), certPEM, 0o644); err != nil { //nolint:gosec // cert is public material
		return fmt.Errorf("%w: write %s: %v", ErrInvalidCAStorage, certName, err)
	}
	if err := os.WriteFile(filepath.Join(s.dir, keyName), sealed, 0o600); err != nil {
		return fmt.Errorf("%w: write %s: %v", ErrInvalidCAStorage, keyName, err)
	}
	return nil
}

func (s *EncryptedFileCAStorage) loadPair(certName, keyName string) (*x509.Certificate, crypto.Signer, error) {
	certPEM, sealed, err := readCAPairFiles(s.dir, certName, keyName)
	if err != nil {
		return nil, nil, err
	}
	cert, err := parseCACertPEM(certName, certPEM)
	if err != nil {
		return nil, nil, err
	}
	keyPEM, err := openCAKey(sealed, s.key)
	if errors.Is(err, errNotCAEnvelope) {
		return nil, nil, fmt.Errorf("%w: %s is not encrypted (plaintext PEM); migrate with `kscore-identity ca encrypt`", ErrInvalidCAStorage, keyName)
	}
	if err != nil {
		return nil, nil, err
	}
	signer, err := parseCAKeyPEM(keyName, keyPEM)
	if err != nil {
		return nil, nil, err
	}
	return cert, signer, nil
}

// hasPair reports presence of both files (same semantics as
// [FileCAStorage] — existence, not validity).
func (s *EncryptedFileCAStorage) hasPair(certName, keyName string) (bool, error) {
	for _, name := range []string{certName, keyName} {
		_, err := os.Stat(filepath.Join(s.dir, name))
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		if err != nil {
			return false, fmt.Errorf("%w: stat %s: %v", ErrInvalidCAStorage, name, err)
		}
	}
	return true, nil
}
