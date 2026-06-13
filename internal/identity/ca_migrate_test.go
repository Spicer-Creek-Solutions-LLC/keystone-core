// SPDX-License-Identifier: Apache-2.0

package identity

import (
	"crypto/ecdsa"
	"errors"
	"sort"
	"testing"

	"go.keystone-core.io/keystone-core/internal/masterkey"
)

func TestEncryptCADirectory_RoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cert, key := mintSelfSignedCA(t)

	// Seed a plaintext directory via FileCAStorage.
	plain, _ := NewFileCAStorage(dir)
	if err := plain.SaveRootCA(cert, key); err != nil {
		t.Fatalf("SaveRootCA: %v", err)
	}
	if err := plain.SaveSigningCA(cert, key); err != nil {
		t.Fatalf("SaveSigningCA: %v", err)
	}

	mk, _ := masterkey.NewRandom()
	migrated, err := EncryptCADirectory(dir, mk)
	if err != nil {
		t.Fatalf("EncryptCADirectory: %v", err)
	}
	sort.Strings(migrated)
	if len(migrated) != 2 || migrated[0] != "root" || migrated[1] != "signing" {
		t.Fatalf("migrated = %v, want [root signing]", migrated)
	}

	// The encrypted storage can now load it; the plaintext one cannot.
	enc, _ := NewEncryptedFileCAStorage(dir, mk)
	gotCert, gotKey, err := enc.LoadRootCA()
	if err != nil {
		t.Fatalf("encrypted LoadRootCA after migrate: %v", err)
	}
	if !gotCert.Equal(cert) || !gotKey.Public().(*ecdsa.PublicKey).Equal(key.Public()) {
		t.Error("root CA changed across migration")
	}
	if _, _, err := plain.LoadRootCA(); err == nil {
		t.Error("plaintext LoadRootCA still succeeds after migration (key not sealed?)")
	}
}

func TestEncryptCADirectory_RefusesDoubleEncrypt(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cert, key := mintSelfSignedCA(t)
	plain, _ := NewFileCAStorage(dir)
	_ = plain.SaveRootCA(cert, key)

	mk, _ := masterkey.NewRandom()
	if _, err := EncryptCADirectory(dir, mk); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	// Second run must refuse — already encrypted.
	_, err := EncryptCADirectory(dir, mk)
	if !errors.Is(err, ErrInvalidCAStorage) {
		t.Errorf("double-encrypt err = %v, want ErrInvalidCAStorage", err)
	}
}

func TestEncryptCADirectory_Empty(t *testing.T) {
	t.Parallel()
	mk, _ := masterkey.NewRandom()
	_, err := EncryptCADirectory(t.TempDir(), mk)
	if !errors.Is(err, ErrCAStorageNotFound) {
		t.Errorf("empty dir err = %v, want ErrCAStorageNotFound", err)
	}
}

func TestEncryptCADirectory_Rejects(t *testing.T) {
	t.Parallel()
	mk, _ := masterkey.NewRandom()
	if _, err := EncryptCADirectory("", mk); !errors.Is(err, ErrInvalidCAStorage) {
		t.Errorf("empty path err = %v, want ErrInvalidCAStorage", err)
	}
	if _, err := EncryptCADirectory(t.TempDir(), masterkey.Key{}); !errors.Is(err, ErrInvalidCAStorage) {
		t.Errorf("zero key err = %v, want ErrInvalidCAStorage", err)
	}
}

func TestEncryptCADirectory_RootOnly(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cert, key := mintSelfSignedCA(t)
	plain, _ := NewFileCAStorage(dir)
	_ = plain.SaveRootCA(cert, key) // signing intentionally absent

	mk, _ := masterkey.NewRandom()
	migrated, err := EncryptCADirectory(dir, mk)
	if err != nil {
		t.Fatalf("EncryptCADirectory: %v", err)
	}
	if len(migrated) != 1 || migrated[0] != "root" {
		t.Errorf("migrated = %v, want [root]", migrated)
	}
}
