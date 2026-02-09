package encryption

import (
	"context"
	"crypto/rand"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestKey_IsExpired(t *testing.T) {
	tests := []struct {
		name     string
		key      *Key
		expected bool
	}{
		{
			name:     "no expiry",
			key:      &Key{},
			expected: false,
		},
		{
			name: "not expired",
			key: &Key{
				ExpiresAt: time.Now().Add(time.Hour),
			},
			expected: false,
		},
		{
			name: "expired",
			key: &Key{
				ExpiresAt: time.Now().Add(-time.Hour),
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.key.IsExpired(); got != tt.expected {
				t.Errorf("IsExpired() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestEncryptor_AES256GCM(t *testing.T) {
	provider := NewInMemoryKeyProvider()
	ctx := context.Background()

	// Create a key
	key, err := provider.RotateKey(ctx, KeyTypeData)
	if err != nil {
		t.Fatalf("RotateKey failed: %v", err)
	}

	encryptor := NewEncryptor(provider, AlgorithmAES256GCM)

	plaintext := []byte("Hello, World!")

	// Encrypt
	encrypted, err := encryptor.Encrypt(ctx, plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	if encrypted.KeyID != key.ID {
		t.Errorf("KeyID = %v, want %v", encrypted.KeyID, key.ID)
	}
	if encrypted.Algorithm != AlgorithmAES256GCM {
		t.Errorf("Algorithm = %v, want %v", encrypted.Algorithm, AlgorithmAES256GCM)
	}

	// Decrypt
	decrypted, err := encryptor.Decrypt(ctx, encrypted)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if string(decrypted) != string(plaintext) {
		t.Errorf("Decrypted = %s, want %s", string(decrypted), string(plaintext))
	}
}

func TestEncryptor_AES256CBC(t *testing.T) {
	provider := NewInMemoryKeyProvider()
	ctx := context.Background()

	// Create a key
	provider.RotateKey(ctx, KeyTypeData)

	encryptor := NewEncryptor(provider, AlgorithmAES256CBC)

	plaintext := []byte("Hello, World!")

	// Encrypt
	encrypted, err := encryptor.Encrypt(ctx, plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	if encrypted.Algorithm != AlgorithmAES256CBC {
		t.Errorf("Algorithm = %v, want %v", encrypted.Algorithm, AlgorithmAES256CBC)
	}

	// Decrypt
	decrypted, err := encryptor.Decrypt(ctx, encrypted)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if string(decrypted) != string(plaintext) {
		t.Errorf("Decrypted = %s, want %s", string(decrypted), string(plaintext))
	}
}

func TestEncryptor_WithAAD(t *testing.T) {
	provider := NewInMemoryKeyProvider()
	ctx := context.Background()

	provider.RotateKey(ctx, KeyTypeData)

	encryptor := NewEncryptor(provider, AlgorithmAES256GCM)

	plaintext := []byte("Hello, World!")
	aad := []byte("additional data")

	// Encrypt with AAD
	encrypted, err := encryptor.EncryptWithAAD(ctx, plaintext, aad)
	if err != nil {
		t.Fatalf("EncryptWithAAD failed: %v", err)
	}

	// Decrypt
	decrypted, err := encryptor.Decrypt(ctx, encrypted)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if string(decrypted) != string(plaintext) {
		t.Errorf("Decrypted = %s, want %s", string(decrypted), string(plaintext))
	}
}

func TestEncryptor_ReEncrypt(t *testing.T) {
	provider := NewInMemoryKeyProvider()
	ctx := context.Background()

	// Create first key
	key1, _ := provider.RotateKey(ctx, KeyTypeData)

	encryptor := NewEncryptor(provider, AlgorithmAES256GCM)

	plaintext := []byte("Hello, World!")

	// Encrypt with first key
	encrypted1, err := encryptor.Encrypt(ctx, plaintext)
	if err != nil {
		t.Fatalf("First encrypt failed: %v", err)
	}

	// Rotate key
	key2, _ := provider.RotateKey(ctx, KeyTypeData)

	// Re-encrypt
	encrypted2, err := encryptor.ReEncrypt(ctx, encrypted1)
	if err != nil {
		t.Fatalf("ReEncrypt failed: %v", err)
	}

	if encrypted2.KeyID != key2.ID {
		t.Errorf("Re-encrypted KeyID = %v, want %v", encrypted2.KeyID, key2.ID)
	}
	if encrypted2.KeyID == key1.ID {
		t.Error("Re-encrypted data should use new key")
	}

	// Verify decryption still works
	decrypted, err := encryptor.Decrypt(ctx, encrypted2)
	if err != nil {
		t.Fatalf("Decrypt re-encrypted failed: %v", err)
	}

	if string(decrypted) != string(plaintext) {
		t.Errorf("Decrypted = %s, want %s", string(decrypted), string(plaintext))
	}
}

func TestEncryptor_ExpiredKey(t *testing.T) {
	provider := NewInMemoryKeyProvider()
	ctx := context.Background()

	// Create an expired key
	key := &Key{
		ID:        "expired-key",
		Type:      KeyTypeData,
		Algorithm: AlgorithmAES256GCM,
		Material:  make([]byte, 32),
		CreatedAt: time.Now().Add(-2 * time.Hour),
		ExpiresAt: time.Now().Add(-time.Hour),
	}
	rand.Read(key.Material)

	provider.AddKey(key)
	provider.SetCurrentKey(KeyTypeData, key.ID)

	encryptor := NewEncryptor(provider, AlgorithmAES256GCM)

	_, err := encryptor.Encrypt(ctx, []byte("test"))
	if !errors.Is(err, ErrKeyExpired) {
		t.Errorf("Encrypt with expired key = %v, want ErrKeyExpired", err)
	}
}

func TestEncryptor_Events(t *testing.T) {
	provider := NewInMemoryKeyProvider()
	ctx := context.Background()

	provider.RotateKey(ctx, KeyTypeData)

	encryptor := NewEncryptor(provider, AlgorithmAES256GCM)

	var events []*Event
	var mu sync.Mutex

	encryptor.AddListener(func(event *Event) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
	})

	plaintext := []byte("test")

	// Encrypt
	encrypted, _ := encryptor.Encrypt(ctx, plaintext)

	// Decrypt
	encryptor.Decrypt(ctx, encrypted)

	mu.Lock()
	defer mu.Unlock()

	if len(events) != 2 {
		t.Fatalf("Expected 2 events, got %d", len(events))
	}

	if events[0].Type != "encrypt" {
		t.Errorf("First event type = %v, want encrypt", events[0].Type)
	}
	if events[1].Type != "decrypt" {
		t.Errorf("Second event type = %v, want decrypt", events[1].Type)
	}
}

func TestInMemoryKeyProvider(t *testing.T) {
	provider := NewInMemoryKeyProvider()
	ctx := context.Background()

	t.Run("rotate key", func(t *testing.T) {
		key, err := provider.RotateKey(ctx, KeyTypeData)
		if err != nil {
			t.Fatalf("RotateKey failed: %v", err)
		}

		if key.Type != KeyTypeData {
			t.Errorf("Key type = %v, want %v", key.Type, KeyTypeData)
		}
		if len(key.Material) != 32 {
			t.Errorf("Key material length = %d, want 32", len(key.Material))
		}
	})

	t.Run("get key", func(t *testing.T) {
		key, _ := provider.RotateKey(ctx, KeyTypeMaster)

		retrieved, err := provider.GetKey(ctx, key.ID)
		if err != nil {
			t.Fatalf("GetKey failed: %v", err)
		}

		if retrieved.ID != key.ID {
			t.Errorf("Retrieved key ID = %v, want %v", retrieved.ID, key.ID)
		}
	})

	t.Run("get current key", func(t *testing.T) {
		key, _ := provider.RotateKey(ctx, KeyTypeWrapping)

		current, err := provider.GetCurrentKey(ctx, KeyTypeWrapping)
		if err != nil {
			t.Fatalf("GetCurrentKey failed: %v", err)
		}

		if current.ID != key.ID {
			t.Errorf("Current key ID = %v, want %v", current.ID, key.ID)
		}
	})

	t.Run("list keys", func(t *testing.T) {
		// Rotate a couple more times
		provider.RotateKey(ctx, KeyTypeData)
		provider.RotateKey(ctx, KeyTypeData)

		keys, err := provider.ListKeys(ctx, KeyTypeData)
		if err != nil {
			t.Fatalf("ListKeys failed: %v", err)
		}

		if len(keys) < 2 {
			t.Errorf("Expected at least 2 keys, got %d", len(keys))
		}
	})

	t.Run("key not found", func(t *testing.T) {
		_, err := provider.GetKey(ctx, "nonexistent")
		if !errors.Is(err, ErrKeyNotFound) {
			t.Errorf("GetKey = %v, want ErrKeyNotFound", err)
		}
	})

	t.Run("rotation tracking", func(t *testing.T) {
		key1, _ := provider.RotateKey(ctx, KeyTypeMaster)
		key2, _ := provider.RotateKey(ctx, KeyTypeMaster)

		if key2.RotatedFrom != key1.ID {
			t.Errorf("RotatedFrom = %v, want %v", key2.RotatedFrom, key1.ID)
		}
		if key2.Version != key1.Version+1 {
			t.Errorf("Version = %d, want %d", key2.Version, key1.Version+1)
		}
	})
}

func TestDerivedKeyProvider(t *testing.T) {
	masterSecret := []byte("my-master-secret")
	salt := []byte("my-salt")

	provider := NewDerivedKeyProvider(masterSecret, salt, 1000)
	ctx := context.Background()

	t.Run("derive key", func(t *testing.T) {
		key, err := provider.DeriveKey(ctx, KeyTypeData, "info1")
		if err != nil {
			t.Fatalf("DeriveKey failed: %v", err)
		}

		if key.Type != KeyTypeData {
			t.Errorf("Key type = %v, want %v", key.Type, KeyTypeData)
		}
		if len(key.Material) != 32 {
			t.Errorf("Key material length = %d, want 32", len(key.Material))
		}
	})

	t.Run("deterministic derivation", func(t *testing.T) {
		provider2 := NewDerivedKeyProvider(masterSecret, salt, 1000)

		key1, _ := provider.DeriveKey(ctx, KeyTypeMaster, "same-info")
		key2, _ := provider2.DeriveKey(ctx, KeyTypeMaster, "same-info")

		// Both should derive the same key material
		if string(key1.Material) != string(key2.Material) {
			t.Error("Same input should produce same key material")
		}
	})

	t.Run("different info produces different keys", func(t *testing.T) {
		key1, _ := provider.DeriveKey(ctx, KeyTypeMaster, "info-a")
		key2, _ := provider.DeriveKey(ctx, KeyTypeMaster, "info-b")

		if string(key1.Material) == string(key2.Material) {
			t.Error("Different info should produce different keys")
		}
	})
}

func TestEnvelopeEncryptor(t *testing.T) {
	provider := NewInMemoryKeyProvider()
	ctx := context.Background()

	// Create wrapping key (KEK)
	provider.RotateKey(ctx, KeyTypeWrapping)

	envelope := NewEnvelopeEncryptor(provider, AlgorithmAES256GCM)

	plaintext := []byte("Secret data to protect")

	// Encrypt
	encrypted, err := envelope.Encrypt(ctx, plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	if len(encrypted.WrappedDEK) == 0 {
		t.Error("WrappedDEK should not be empty")
	}
	if encrypted.Data == nil {
		t.Fatal("Encrypted data should not be nil")
	}

	// Decrypt
	decrypted, err := envelope.Decrypt(ctx, encrypted)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if string(decrypted) != string(plaintext) {
		t.Errorf("Decrypted = %s, want %s", string(decrypted), string(plaintext))
	}
}

func TestEncryptJSON(t *testing.T) {
	provider := NewInMemoryKeyProvider()
	ctx := context.Background()

	provider.RotateKey(ctx, KeyTypeData)

	encryptor := NewEncryptor(provider, AlgorithmAES256GCM)

	type testStruct struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	original := testStruct{Name: "test", Value: 42}

	// Encrypt
	encrypted, err := EncryptJSON(ctx, encryptor, original)
	if err != nil {
		t.Fatalf("EncryptJSON failed: %v", err)
	}

	if encrypted == "" {
		t.Error("Encrypted string should not be empty")
	}

	// Decrypt
	var decrypted testStruct
	err = DecryptJSON(ctx, encryptor, encrypted, &decrypted)
	if err != nil {
		t.Fatalf("DecryptJSON failed: %v", err)
	}

	if decrypted.Name != original.Name {
		t.Errorf("Decrypted.Name = %v, want %v", decrypted.Name, original.Name)
	}
	if decrypted.Value != original.Value {
		t.Errorf("Decrypted.Value = %v, want %v", decrypted.Value, original.Value)
	}
}

func TestEncryptor_DecryptInvalidData(t *testing.T) {
	provider := NewInMemoryKeyProvider()
	ctx := context.Background()

	provider.RotateKey(ctx, KeyTypeData)

	encryptor := NewEncryptor(provider, AlgorithmAES256GCM)

	// Encrypt some data
	encrypted, _ := encryptor.Encrypt(ctx, []byte("test"))

	// Modify ciphertext
	encrypted.Ciphertext[0] ^= 0xFF

	// Decrypt should fail
	_, err := encryptor.Decrypt(ctx, encrypted)
	if !errors.Is(err, ErrDecryptionFailed) {
		t.Errorf("Decrypt modified ciphertext = %v, want ErrDecryptionFailed", err)
	}
}

func TestEncryptor_DecryptMissingKey(t *testing.T) {
	provider := NewInMemoryKeyProvider()
	ctx := context.Background()

	provider.RotateKey(ctx, KeyTypeData)

	encryptor := NewEncryptor(provider, AlgorithmAES256GCM)

	// Encrypt some data
	encrypted, _ := encryptor.Encrypt(ctx, []byte("test"))

	// Change key ID
	encrypted.KeyID = "nonexistent-key"

	// Decrypt should fail
	_, err := encryptor.Decrypt(ctx, encrypted)
	if err == nil {
		t.Error("Expected error for missing key")
	}
}

func TestAESCBC_Padding(t *testing.T) {
	provider := NewInMemoryKeyProvider()
	ctx := context.Background()

	provider.RotateKey(ctx, KeyTypeData)

	encryptor := NewEncryptor(provider, AlgorithmAES256CBC)

	// Test various sizes that exercise padding
	sizes := []int{1, 15, 16, 17, 31, 32, 33, 100}

	for _, size := range sizes {
		plaintext := make([]byte, size)
		rand.Read(plaintext)

		encrypted, err := encryptor.Encrypt(ctx, plaintext)
		if err != nil {
			t.Fatalf("Encrypt size %d failed: %v", size, err)
		}

		decrypted, err := encryptor.Decrypt(ctx, encrypted)
		if err != nil {
			t.Fatalf("Decrypt size %d failed: %v", size, err)
		}

		if string(decrypted) != string(plaintext) {
			t.Errorf("Size %d: decrypted != plaintext", size)
		}
	}
}

func TestEncryptor_ConcurrentAccess(t *testing.T) {
	provider := NewInMemoryKeyProvider()
	ctx := context.Background()

	provider.RotateKey(ctx, KeyTypeData)

	encryptor := NewEncryptor(provider, AlgorithmAES256GCM)

	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			for j := 0; j < 100; j++ {
				plaintext := []byte("test message")

				encrypted, err := encryptor.Encrypt(ctx, plaintext)
				if err != nil {
					t.Errorf("Worker %d: Encrypt failed: %v", id, err)
					return
				}

				decrypted, err := encryptor.Decrypt(ctx, encrypted)
				if err != nil {
					t.Errorf("Worker %d: Decrypt failed: %v", id, err)
					return
				}

				if string(decrypted) != string(plaintext) {
					t.Errorf("Worker %d: mismatch", id)
				}
			}
		}(i)
	}

	wg.Wait()
}
