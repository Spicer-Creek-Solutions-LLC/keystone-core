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
// implementations include [FileCAStorage] (plaintext PEM, v0.1
// default) and, in a future gate-v0.5 PR, an encrypted variant
// (see ROADMAP "Encrypt CA material at rest"). The CAManager only
// touches storage through this interface so encryption swap-in is
// a wire change at the construction site.
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
// Encryption-at-rest is a v0.5 gate per the ROADMAP entry "Encrypt
// CA material at rest" — until then plaintext on a restricted-mode
// directory is the v0.1 surface, matching the §4.10 "with optional
// encryption key" note's deferral semantics.
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
	if cert == nil {
		return fmt.Errorf("%w: cert is nil", ErrInvalidCAStorage)
	}
	if key == nil {
		return fmt.Errorf("%w: key is nil", ErrInvalidCAStorage)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return fmt.Errorf("%w: marshal key: %v", ErrInvalidCAStorage, err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})

	if err := os.WriteFile(filepath.Join(s.dir, certName), certPEM, 0o644); err != nil { //nolint:gosec // cert is public material
		return fmt.Errorf("%w: write %s: %v", ErrInvalidCAStorage, certName, err)
	}
	if err := os.WriteFile(filepath.Join(s.dir, keyName), keyPEM, 0o600); err != nil {
		return fmt.Errorf("%w: write %s: %v", ErrInvalidCAStorage, keyName, err)
	}
	return nil
}

func (s *FileCAStorage) loadPair(certName, keyName string) (*x509.Certificate, crypto.Signer, error) {
	certPath := filepath.Join(s.dir, certName)
	keyPath := filepath.Join(s.dir, keyName)

	certPEM, err := os.ReadFile(certPath) //nolint:gosec // operator-owned path
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, fmt.Errorf("%w: %s", ErrCAStorageNotFound, certName)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("%w: read %s: %v", ErrInvalidCAStorage, certName, err)
	}
	keyPEM, err := os.ReadFile(keyPath) //nolint:gosec // operator-owned path
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, fmt.Errorf("%w: %s", ErrCAStorageNotFound, keyName)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("%w: read %s: %v", ErrInvalidCAStorage, keyName, err)
	}

	certBlock, _ := pem.Decode(certPEM)
	if certBlock == nil || certBlock.Type != "CERTIFICATE" {
		return nil, nil, fmt.Errorf("%w: %s: PEM block not CERTIFICATE", ErrInvalidCAStorage, certName)
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: parse %s: %v", ErrInvalidCAStorage, certName, err)
	}

	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil || keyBlock.Type != "PRIVATE KEY" {
		return nil, nil, fmt.Errorf("%w: %s: PEM block not PRIVATE KEY", ErrInvalidCAStorage, keyName)
	}
	key, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: parse %s: %v", ErrInvalidCAStorage, keyName, err)
	}
	signer, ok := key.(crypto.Signer)
	if !ok {
		return nil, nil, fmt.Errorf("%w: %s: key does not implement crypto.Signer", ErrInvalidCAStorage, keyName)
	}
	return cert, signer, nil
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
