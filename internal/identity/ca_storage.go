// SPDX-License-Identifier: Apache-2.0

package identity

import (
	"crypto"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrCAStorageNotFound is returned by [CAStorage] Load methods when
// the requested material is absent — distinguishes "no CA yet"
// (Initialize should generate one) from a real I/O failure.
var ErrCAStorageNotFound = errors.New("identity: CA storage not found")

// ErrInvalidCAStorage wraps every other [CAStorage] rejection
// (corrupt PEM, decode errors, permission failures, …).
var ErrInvalidCAStorage = errors.New("identity: invalid CA storage")

// CAStorage is the persistence surface for [CAManager]. Concrete
// implementations are [FileCAStorage] (plaintext PEM) and
// [EncryptedFileCAStorage] (the private-key files sealed with
// AES-256-GCM). The CAManager only touches storage through this
// interface, so the encrypted variant is a drop-in selected at the
// construction site (server config `identity.encryption_key`).
//
// Implementations MUST be goroutine-safe — the CAManager calls
// Save during rotation while readers may be calling Load (e.g.
// the Provider re-loading on a config reload).
type CAStorage interface {
	SaveRootCA(cert *x509.Certificate, key crypto.Signer) error
	LoadRootCA() (*x509.Certificate, crypto.Signer, error) // ErrCAStorageNotFound when absent
	HasRootCA() (bool, error)

	SaveSigningCA(cert *x509.Certificate, key crypto.Signer) error
	LoadSigningCA() (*x509.Certificate, crypto.Signer, error) // ErrCAStorageNotFound when absent
	HasSigningCA() (bool, error)
}

// FileCAStorage stores CA material as plaintext PEM-encoded files
// under a single directory. Layout:
//
//	<dir>/root-cert.pem      (CERTIFICATE)
//	<dir>/root-key.pem       (PRIVATE KEY, PKCS#8, mode 0600)
//	<dir>/signing-cert.pem   (CERTIFICATE)
//	<dir>/signing-key.pem    (PRIVATE KEY, PKCS#8, mode 0600)
//
// The directory itself is created with mode 0700 if absent.
//
// This variant protects the private keys with filesystem permissions
// only (0600 key files in a 0700 directory). For encryption-at-rest of
// the key material — PROJECT-DETAILS §4.10's "optional encryption key" —
// use [EncryptedFileCAStorage], which is a drop-in over the same layout.
type FileCAStorage struct {
	dir string
}

const (
	rootCertFile    = "root-cert.pem"
	rootKeyFile     = "root-key.pem"
	signingCertFile = "signing-cert.pem"
	signingKeyFile  = "signing-key.pem"
)

// NewFileCAStorage returns a storage rooted at dir. The directory
// is created with mode 0700 if absent.
func NewFileCAStorage(dir string) (*FileCAStorage, error) {
	if dir == "" {
		return nil, fmt.Errorf("%w: directory is required", ErrInvalidCAStorage)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("%w: mkdir %s: %v", ErrInvalidCAStorage, dir, err)
	}
	return &FileCAStorage{dir: dir}, nil
}

// SaveRootCA writes the cert + key as PEM. Existing files are
// overwritten.
func (s *FileCAStorage) SaveRootCA(cert *x509.Certificate, key crypto.Signer) error {
	return s.savePair(rootCertFile, rootKeyFile, cert, key)
}

// LoadRootCA returns the persisted root, or ErrCAStorageNotFound
// when neither cert nor key file exists.
func (s *FileCAStorage) LoadRootCA() (*x509.Certificate, crypto.Signer, error) {
	return s.loadPair(rootCertFile, rootKeyFile)
}

// HasRootCA reports whether both root files exist.
func (s *FileCAStorage) HasRootCA() (bool, error) {
	return s.hasPair(rootCertFile, rootKeyFile)
}

// SaveSigningCA writes the signing CA cert + key.
func (s *FileCAStorage) SaveSigningCA(cert *x509.Certificate, key crypto.Signer) error {
	return s.savePair(signingCertFile, signingKeyFile, cert, key)
}

// LoadSigningCA returns the persisted signing CA.
func (s *FileCAStorage) LoadSigningCA() (*x509.Certificate, crypto.Signer, error) {
	return s.loadPair(signingCertFile, signingKeyFile)
}

// HasSigningCA reports whether both signing files exist.
func (s *FileCAStorage) HasSigningCA() (bool, error) {
	return s.hasPair(signingCertFile, signingKeyFile)
}

func (s *FileCAStorage) savePair(certName, keyName string, cert *x509.Certificate, key crypto.Signer) error {
	certPEM, keyPEM, err := marshalCAPair(cert, key)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(s.dir, certName), certPEM, 0o644); err != nil { //nolint:gosec // cert is public material
		return fmt.Errorf("%w: write %s: %v", ErrInvalidCAStorage, certName, err)
	}
	if err := os.WriteFile(filepath.Join(s.dir, keyName), keyPEM, 0o600); err != nil {
		return fmt.Errorf("%w: write %s: %v", ErrInvalidCAStorage, keyName, err)
	}
	return nil
}

func (s *FileCAStorage) loadPair(certName, keyName string) (*x509.Certificate, crypto.Signer, error) {
	certPEM, keyPEM, err := readCAPairFiles(s.dir, certName, keyName)
	if err != nil {
		return nil, nil, err
	}
	cert, err := parseCACertPEM(certName, certPEM)
	if err != nil {
		return nil, nil, err
	}
	signer, err := parseCAKeyPEM(keyName, keyPEM)
	if err != nil {
		return nil, nil, err
	}
	return cert, signer, nil
}

// marshalCAPair encodes a CA cert + key to their PEM forms (CERTIFICATE
// and PKCS#8 PRIVATE KEY). Shared by [FileCAStorage] (which writes the
// key PEM as-is) and [EncryptedFileCAStorage] (which seals it first).
func marshalCAPair(cert *x509.Certificate, key crypto.Signer) (certPEM, keyPEM []byte, err error) {
	if cert == nil {
		return nil, nil, fmt.Errorf("%w: cert is nil", ErrInvalidCAStorage)
	}
	if key == nil {
		return nil, nil, fmt.Errorf("%w: key is nil", ErrInvalidCAStorage)
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: marshal key: %v", ErrInvalidCAStorage, err)
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM, nil
}

// readCAPairFiles reads the raw cert + key file bytes, mapping a missing
// file to [ErrCAStorageNotFound]. The key bytes are the on-disk form —
// plaintext PEM for [FileCAStorage], a sealed envelope for
// [EncryptedFileCAStorage].
func readCAPairFiles(dir, certName, keyName string) (certPEM, keyBytes []byte, err error) {
	certPEM, err = os.ReadFile(filepath.Join(dir, certName)) //nolint:gosec // operator-owned path
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, fmt.Errorf("%w: %s", ErrCAStorageNotFound, certName)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("%w: read %s: %v", ErrInvalidCAStorage, certName, err)
	}
	keyBytes, err = os.ReadFile(filepath.Join(dir, keyName)) //nolint:gosec // operator-owned path
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, fmt.Errorf("%w: %s", ErrCAStorageNotFound, keyName)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("%w: read %s: %v", ErrInvalidCAStorage, keyName, err)
	}
	return certPEM, keyBytes, nil
}

// parseCACertPEM decodes a CERTIFICATE PEM block into an x509 cert.
func parseCACertPEM(certName string, certPEM []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("%w: %s: PEM block not CERTIFICATE", ErrInvalidCAStorage, certName)
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("%w: parse %s: %v", ErrInvalidCAStorage, certName, err)
	}
	return cert, nil
}

// parseCAKeyPEM decodes a PKCS#8 PRIVATE KEY PEM block into a signer.
func parseCAKeyPEM(keyName string, keyPEM []byte) (crypto.Signer, error) {
	block, _ := pem.Decode(keyPEM)
	if block == nil || block.Type != "PRIVATE KEY" {
		return nil, fmt.Errorf("%w: %s: PEM block not PRIVATE KEY", ErrInvalidCAStorage, keyName)
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("%w: parse %s: %v", ErrInvalidCAStorage, keyName, err)
	}
	signer, ok := key.(crypto.Signer)
	if !ok {
		return nil, fmt.Errorf("%w: %s: key does not implement crypto.Signer", ErrInvalidCAStorage, keyName)
	}
	return signer, nil
}

func (s *FileCAStorage) hasPair(certName, keyName string) (bool, error) {
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
