// SPDX-License-Identifier: Apache-2.0

package identity

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/masterkey"
)

// TestEmbeddedProvider_EncryptedStorageEndToEnd is the gate-v0.5
// acceptance for "Encrypt CA material at rest": a clean embedded
// provider boots with encryption enabled, generates its CA through the
// encrypted storage, persists it sealed, reloads it across a restart
// with the right key, and refuses to boot with the wrong key.
func TestEmbeddedProvider_EncryptedStorageEndToEnd(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	key, err := masterkey.NewRandom()
	if err != nil {
		t.Fatalf("NewRandom: %v", err)
	}

	startProvider := func(t *testing.T, storage CAStorage) *EmbeddedProvider {
		t.Helper()
		p, err := NewEmbeddedProvider(EmbeddedProviderConfig{
			CAConfig:        newFastCAConfig(DefaultTrustDomain),
			Storage:         storage,
			RotatorInterval: time.Hour,
			Logger:          silentLogger(),
		})
		if err != nil {
			t.Fatalf("NewEmbeddedProvider: %v", err)
		}
		if err := p.Start(context.Background()); err != nil {
			t.Fatalf("Start: %v", err)
		}
		return p
	}
	stop := func(p *EmbeddedProvider) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = p.Stop(ctx)
	}

	// 1. Fresh boot — generates + seals the CA through encrypted storage.
	enc1, err := NewEncryptedFileCAStorage(dir, key)
	if err != nil {
		t.Fatalf("NewEncryptedFileCAStorage: %v", err)
	}
	p1 := startProvider(t, enc1)
	rootCert1, _, err := enc1.LoadRootCA()
	if err != nil {
		t.Fatalf("LoadRootCA after first boot: %v", err)
	}
	stop(p1)

	// On disk the key is sealed (magic prefix), not plaintext PEM.
	keyBytes, err := os.ReadFile(filepath.Join(dir, rootKeyFile))
	if err != nil {
		t.Fatalf("read root key: %v", err)
	}
	if [caEnvMagicLen]byte(keyBytes[:caEnvMagicLen]) != caKeyMagic {
		t.Fatal("persisted CA key is not an encrypted envelope")
	}

	// 2. Restart with the same key — loads the existing CA, not a new one.
	enc2, _ := NewEncryptedFileCAStorage(dir, key)
	p2 := startProvider(t, enc2)
	rootCert2, _, err := enc2.LoadRootCA()
	if err != nil {
		t.Fatalf("LoadRootCA after restart: %v", err)
	}
	stop(p2)
	if !rootCert2.Equal(rootCert1) {
		t.Error("CA was regenerated across restart instead of loaded from sealed storage")
	}

	// 3. Boot with the wrong key — must fail (cannot decrypt the CA).
	wrongKey, _ := masterkey.NewRandom()
	enc3, _ := NewEncryptedFileCAStorage(dir, wrongKey)
	p3, err := NewEmbeddedProvider(EmbeddedProviderConfig{
		CAConfig:        newFastCAConfig(DefaultTrustDomain),
		Storage:         enc3,
		RotatorInterval: time.Hour,
		Logger:          silentLogger(),
	})
	if err != nil {
		t.Fatalf("NewEmbeddedProvider (wrong key): %v", err)
	}
	if err := p3.Start(context.Background()); err == nil {
		stop(p3)
		t.Fatal("provider booted with the wrong master key — sealed CA should be undecryptable")
	}
}
