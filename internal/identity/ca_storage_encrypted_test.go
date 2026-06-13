// SPDX-License-Identifier: Apache-2.0

package identity

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"go.keystone-core.io/keystone-core/internal/masterkey"
)

func newEncStorage(t *testing.T) (*EncryptedFileCAStorage, string) {
	t.Helper()
	dir := t.TempDir()
	key, err := masterkey.NewRandom()
	if err != nil {
		t.Fatalf("NewRandom: %v", err)
	}
	s, err := NewEncryptedFileCAStorage(dir, key)
	if err != nil {
		t.Fatalf("NewEncryptedFileCAStorage: %v", err)
	}
	return s, dir
}

func TestNewEncryptedFileCAStorage_Rejects(t *testing.T) {
	t.Parallel()
	key, _ := masterkey.NewRandom()
	if _, err := NewEncryptedFileCAStorage("", key); !errors.Is(err, ErrInvalidCAStorage) {
		t.Errorf("empty dir err = %v, want ErrInvalidCAStorage", err)
	}
	if _, err := NewEncryptedFileCAStorage(t.TempDir(), masterkey.Key{}); !errors.Is(err, ErrInvalidCAStorage) {
		t.Errorf("zero key err = %v, want ErrInvalidCAStorage", err)
	}
}

func TestEncryptedFileCAStorage_RoundTrip(t *testing.T) {
	t.Parallel()
	s, _ := newEncStorage(t)
	cert, key := mintSelfSignedCA(t)

	if err := s.SaveRootCA(cert, key); err != nil {
		t.Fatalf("SaveRootCA: %v", err)
	}
	if err := s.SaveSigningCA(cert, key); err != nil {
		t.Fatalf("SaveSigningCA: %v", err)
	}

	gotCert, gotKey, err := s.LoadRootCA()
	if err != nil {
		t.Fatalf("LoadRootCA: %v", err)
	}
	if !gotCert.Equal(cert) {
		t.Error("root cert round-trip mismatch")
	}
	if !gotKey.Public().(*ecdsa.PublicKey).Equal(key.Public()) {
		t.Error("root key round-trip mismatch")
	}

	sCert, sKey, err := s.LoadSigningCA()
	if err != nil {
		t.Fatalf("LoadSigningCA: %v", err)
	}
	if !sCert.Equal(cert) || !sKey.Public().(*ecdsa.PublicKey).Equal(key.Public()) {
		t.Error("signing CA round-trip mismatch")
	}
}

func TestEncryptedFileCAStorage_OnDiskShape(t *testing.T) {
	t.Parallel()
	s, dir := newEncStorage(t)
	cert, key := mintSelfSignedCA(t)
	if err := s.SaveRootCA(cert, key); err != nil {
		t.Fatalf("SaveRootCA: %v", err)
	}

	// Cert file is plaintext PEM (public material).
	certBytes, err := os.ReadFile(filepath.Join(dir, rootCertFile))
	if err != nil {
		t.Fatalf("read cert: %v", err)
	}
	if blk, _ := pem.Decode(certBytes); blk == nil || blk.Type != "CERTIFICATE" {
		t.Error("cert file is not a plaintext CERTIFICATE PEM")
	}

	// Key file is an encrypted envelope: magic prefix, no PEM, mode 0600.
	keyBytes, err := os.ReadFile(filepath.Join(dir, rootKeyFile))
	if err != nil {
		t.Fatalf("read key: %v", err)
	}
	if [caEnvMagicLen]byte(keyBytes[:caEnvMagicLen]) != caKeyMagic {
		t.Error("key file missing CA envelope magic")
	}
	if bytes.Contains(keyBytes, []byte("PRIVATE KEY")) {
		t.Error("key file leaks plaintext PEM")
	}
	if fi, _ := os.Stat(filepath.Join(dir, rootKeyFile)); fi.Mode().Perm() != 0o600 {
		t.Errorf("key file mode = %o, want 0600", fi.Mode().Perm())
	}
}

func TestEncryptedFileCAStorage_WrongKey(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	k1, _ := masterkey.NewRandom()
	s1, _ := NewEncryptedFileCAStorage(dir, k1)
	cert, key := mintSelfSignedCA(t)
	if err := s1.SaveRootCA(cert, key); err != nil {
		t.Fatalf("SaveRootCA: %v", err)
	}

	k2, _ := masterkey.NewRandom()
	s2, _ := NewEncryptedFileCAStorage(dir, k2)
	_, _, err := s2.LoadRootCA()
	if !errors.Is(err, ErrInvalidCAStorage) {
		t.Errorf("wrong-key load err = %v, want ErrInvalidCAStorage", err)
	}
}

func TestEncryptedFileCAStorage_PlaintextKeyHint(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// FileCAStorage writes plaintext; EncryptedFileCAStorage must reject
	// the key file with a migration hint, not an opaque error.
	plain, _ := NewFileCAStorage(dir)
	cert, key := mintSelfSignedCA(t)
	if err := plain.SaveRootCA(cert, key); err != nil {
		t.Fatalf("plaintext SaveRootCA: %v", err)
	}

	mk, _ := masterkey.NewRandom()
	enc, _ := NewEncryptedFileCAStorage(dir, mk)
	_, _, err := enc.LoadRootCA()
	if !errors.Is(err, ErrInvalidCAStorage) {
		t.Fatalf("err = %v, want ErrInvalidCAStorage", err)
	}
	if !bytesContainsString(err.Error(), "ca encrypt") {
		t.Errorf("err = %q, want a migration hint mentioning `ca encrypt`", err.Error())
	}
}

func TestEncryptedFileCAStorage_HasAndMissing(t *testing.T) {
	t.Parallel()
	s, _ := newEncStorage(t)

	if has, err := s.HasRootCA(); err != nil || has {
		t.Errorf("HasRootCA on empty = (%v, %v), want (false, nil)", has, err)
	}
	if _, _, err := s.LoadRootCA(); !errors.Is(err, ErrCAStorageNotFound) {
		t.Errorf("LoadRootCA on empty err = %v, want ErrCAStorageNotFound", err)
	}

	cert, key := mintSelfSignedCA(t)
	if err := s.SaveRootCA(cert, key); err != nil {
		t.Fatalf("SaveRootCA: %v", err)
	}
	if has, err := s.HasRootCA(); err != nil || !has {
		t.Errorf("HasRootCA after save = (%v, %v), want (true, nil)", has, err)
	}
}

// bytesContainsString reports whether s contains substr.
func bytesContainsString(s, substr string) bool {
	return bytes.Contains([]byte(s), []byte(substr))
}
