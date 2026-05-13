package identity

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ---- helpers -----------------------------------------------------

func mintSelfSignedCA(t *testing.T) (*x509.Certificate, crypto.Signer) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	tpl := &x509.Certificate{
		SerialNumber:          big.NewInt(99),
		Subject:               pkix.Name{CommonName: "storage-test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("ParseCertificate: %v", err)
	}
	return cert, key
}

// ---- NewFileCAStorage --------------------------------------------

func TestNewFileCAStorage_RejectsEmptyDir(t *testing.T) {
	t.Parallel()
	_, err := NewFileCAStorage("")
	if err == nil || !errors.Is(err, ErrInvalidCAStorage) {
		t.Fatalf("err = %v", err)
	}
}

func TestNewFileCAStorage_CreatesDir(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	nested := filepath.Join(base, "ca", "child")
	s, err := NewFileCAStorage(nested)
	if err != nil {
		t.Fatalf("NewFileCAStorage: %v", err)
	}
	if s == nil {
		t.Fatal("nil storage")
	}
	info, err := os.Stat(nested)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if !info.IsDir() {
		t.Errorf("created path is not a directory: %s", info.Mode())
	}
	// Mode mask drops the high bits; on Linux MkdirAll honours the
	// requested 0700 (modulo umask). Don't assert exact bits — just
	// that group + other are unset.
	if info.Mode().Perm()&0o077 != 0 {
		t.Errorf("dir permissions = %o, want group+other unset", info.Mode().Perm())
	}
}

func TestNewFileCAStorage_RejectsUnreachablePath(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("running as root; chmod-blocked path doesn't apply")
	}
	parent := t.TempDir()
	locked := filepath.Join(parent, "locked")
	if err := os.Mkdir(locked, 0o500); err != nil { // read+execute, no write
		t.Fatalf("Mkdir: %v", err)
	}
	defer func() { _ = os.Chmod(locked, 0o700) }()
	_, err := NewFileCAStorage(filepath.Join(locked, "child"))
	if err == nil || !errors.Is(err, ErrInvalidCAStorage) {
		t.Fatalf("err = %v", err)
	}
}

// ---- Save / Load round-trip --------------------------------------

func TestFileCAStorage_RoundTripRoot(t *testing.T) {
	t.Parallel()
	s, err := NewFileCAStorage(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileCAStorage: %v", err)
	}
	cert, key := mintSelfSignedCA(t)

	if has, err := s.HasRootCA(); err != nil || has {
		t.Errorf("HasRootCA before save: has=%v err=%v", has, err)
	}
	if err := s.SaveRootCA(cert, key); err != nil {
		t.Fatalf("SaveRootCA: %v", err)
	}
	if has, err := s.HasRootCA(); err != nil || !has {
		t.Errorf("HasRootCA after save: has=%v err=%v", has, err)
	}
	gotCert, gotKey, err := s.LoadRootCA()
	if err != nil {
		t.Fatalf("LoadRootCA: %v", err)
	}
	if !gotCert.Equal(cert) {
		t.Error("LoadRootCA cert != saved")
	}
	// crypto.Signer interface doesn't expose Equal; check public-key
	// equality the way our X509SVID validator does.
	if !publicKeysEqual(gotKey.Public(), key.Public()) {
		t.Error("LoadRootCA key.Public() != saved key.Public()")
	}
}

func TestFileCAStorage_RoundTripSigning(t *testing.T) {
	t.Parallel()
	s, err := NewFileCAStorage(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileCAStorage: %v", err)
	}
	cert, key := mintSelfSignedCA(t)

	if has, _ := s.HasSigningCA(); has {
		t.Error("HasSigningCA before save: true")
	}
	if err := s.SaveSigningCA(cert, key); err != nil {
		t.Fatalf("SaveSigningCA: %v", err)
	}
	if has, _ := s.HasSigningCA(); !has {
		t.Error("HasSigningCA after save: false")
	}
	gotCert, gotKey, err := s.LoadSigningCA()
	if err != nil {
		t.Fatalf("LoadSigningCA: %v", err)
	}
	if !gotCert.Equal(cert) {
		t.Error("LoadSigningCA cert != saved")
	}
	if !publicKeysEqual(gotKey.Public(), key.Public()) {
		t.Error("LoadSigningCA key.Public() != saved key.Public()")
	}
}

func TestFileCAStorage_LoadMissing(t *testing.T) {
	t.Parallel()
	s, _ := NewFileCAStorage(t.TempDir())
	_, _, err := s.LoadRootCA()
	if err == nil || !errors.Is(err, ErrCAStorageNotFound) {
		t.Errorf("LoadRootCA: err = %v, want ErrCAStorageNotFound", err)
	}
	_, _, err = s.LoadSigningCA()
	if err == nil || !errors.Is(err, ErrCAStorageNotFound) {
		t.Errorf("LoadSigningCA: err = %v, want ErrCAStorageNotFound", err)
	}
}

func TestFileCAStorage_FilePermissions(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s, _ := NewFileCAStorage(dir)
	cert, key := mintSelfSignedCA(t)
	if err := s.SaveRootCA(cert, key); err != nil {
		t.Fatalf("SaveRootCA: %v", err)
	}
	certInfo, err := os.Stat(filepath.Join(dir, rootCertFile))
	if err != nil {
		t.Fatalf("Stat cert: %v", err)
	}
	keyInfo, err := os.Stat(filepath.Join(dir, rootKeyFile))
	if err != nil {
		t.Fatalf("Stat key: %v", err)
	}
	if certInfo.Mode().Perm() != 0o644 {
		t.Errorf("cert perm = %o, want 0644", certInfo.Mode().Perm())
	}
	if keyInfo.Mode().Perm() != 0o600 {
		t.Errorf("key perm = %o, want 0600", keyInfo.Mode().Perm())
	}
}

// ---- Save error paths --------------------------------------------

func TestFileCAStorage_SaveRejectsNilCert(t *testing.T) {
	t.Parallel()
	s, _ := NewFileCAStorage(t.TempDir())
	_, key := mintSelfSignedCA(t)
	if err := s.SaveRootCA(nil, key); err == nil || !errors.Is(err, ErrInvalidCAStorage) {
		t.Errorf("err = %v", err)
	}
}

func TestFileCAStorage_SaveRejectsNilKey(t *testing.T) {
	t.Parallel()
	s, _ := NewFileCAStorage(t.TempDir())
	cert, _ := mintSelfSignedCA(t)
	if err := s.SaveRootCA(cert, nil); err == nil || !errors.Is(err, ErrInvalidCAStorage) {
		t.Errorf("err = %v", err)
	}
}

// ---- Load error paths --------------------------------------------

func TestFileCAStorage_LoadRejectsGarbageCert(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, rootCertFile), []byte("not pem"), 0o644); err != nil {
		t.Fatalf("seed cert: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, rootKeyFile), []byte("also not pem"), 0o600); err != nil {
		t.Fatalf("seed key: %v", err)
	}
	s, _ := NewFileCAStorage(dir)
	_, _, err := s.LoadRootCA()
	if err == nil || !errors.Is(err, ErrInvalidCAStorage) {
		t.Errorf("err = %v", err)
	}
}

func TestFileCAStorage_LoadRejectsWrongPEMType(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// PEM with the wrong block type.
	cert := "-----BEGIN NOT-A-CERT-----\nQQ==\n-----END NOT-A-CERT-----\n"
	keyPEM := "-----BEGIN PRIVATE KEY-----\nQQ==\n-----END PRIVATE KEY-----\n"
	if err := os.WriteFile(filepath.Join(dir, rootCertFile), []byte(cert), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, rootKeyFile), []byte(keyPEM), 0o600); err != nil {
		t.Fatal(err)
	}
	s, _ := NewFileCAStorage(dir)
	_, _, err := s.LoadRootCA()
	if err == nil || !errors.Is(err, ErrInvalidCAStorage) {
		t.Errorf("err = %v", err)
	}
}

func TestFileCAStorage_LoadRejectsGarbageKey(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s, _ := NewFileCAStorage(dir)
	cert, key := mintSelfSignedCA(t)
	if err := s.SaveRootCA(cert, key); err != nil {
		t.Fatalf("Save: %v", err)
	}
	// Overwrite key with garbage.
	if err := os.WriteFile(filepath.Join(dir, rootKeyFile), []byte("garbage"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err := s.LoadRootCA()
	if err == nil || !errors.Is(err, ErrInvalidCAStorage) {
		t.Errorf("err = %v", err)
	}
}

func TestFileCAStorage_LoadRejectsWrongKeyPEMType(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s, _ := NewFileCAStorage(dir)
	cert, key := mintSelfSignedCA(t)
	if err := s.SaveRootCA(cert, key); err != nil {
		t.Fatalf("Save: %v", err)
	}
	bad := "-----BEGIN EC PRIVATE KEY-----\nQQ==\n-----END EC PRIVATE KEY-----\n"
	if err := os.WriteFile(filepath.Join(dir, rootKeyFile), []byte(bad), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err := s.LoadRootCA()
	if err == nil || !errors.Is(err, ErrInvalidCAStorage) {
		t.Errorf("err = %v", err)
	}
}
