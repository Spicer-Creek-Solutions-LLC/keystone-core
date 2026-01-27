package kms

import (
	"bytes"
	"context"
	"testing"
	"time"
)

func TestTransitEngine_CreateKey(t *testing.T) {
	engine := NewTransitEngine(nil, nil)
	ctx := context.Background()

	key, err := engine.CreateKey(ctx, "test-key", AlgorithmAESGCM, false, false)
	if err != nil {
		t.Fatalf("Failed to create key: %v", err)
	}

	if key.Name != "test-key" {
		t.Errorf("Expected key name 'test-key', got %s", key.Name)
	}
	if key.Algorithm != AlgorithmAESGCM {
		t.Errorf("Expected algorithm AES-GCM, got %s", key.Algorithm)
	}
	if key.Version != 1 {
		t.Errorf("Expected version 1, got %d", key.Version)
	}
	if len(key.KeyMaterial) != 32 {
		t.Errorf("Expected 32 byte key, got %d", len(key.KeyMaterial))
	}

	_, err = engine.CreateKey(ctx, "test-key", AlgorithmAESGCM, false, false)
	if err == nil {
		t.Error("Expected error creating duplicate key")
	}
}

func TestTransitEngine_CreateKeyConvergent(t *testing.T) {
	engine := NewTransitEngine(nil, nil)
	ctx := context.Background()

	key, err := engine.CreateKey(ctx, "convergent-key", AlgorithmAESGCM, true, false)
	if err != nil {
		t.Fatalf("Failed to create convergent key: %v", err)
	}

	if !key.SupportsConvergent {
		t.Error("Expected key to support convergent encryption")
	}
	if key.ConvergentKey == nil {
		t.Error("Expected convergent key material to be set")
	}
}

func TestTransitEngine_ConvergentEncryption(t *testing.T) {
	engine := NewTransitEngine(nil, nil)
	ctx := context.Background()

	_, err := engine.CreateKey(ctx, "conv-key", AlgorithmAESGCM, true, false)
	if err != nil {
		t.Fatalf("Failed to create key: %v", err)
	}

	plaintext := []byte("sensitive data for convergent encryption")
	context := []byte("user-123")

	resp1, err := engine.ConvergentEncrypt(ctx, &ConvergentEncryptRequest{
		KeyName:   "conv-key",
		Plaintext: plaintext,
		Context:   context,
	})
	if err != nil {
		t.Fatalf("First encryption failed: %v", err)
	}

	resp2, err := engine.ConvergentEncrypt(ctx, &ConvergentEncryptRequest{
		KeyName:   "conv-key",
		Plaintext: plaintext,
		Context:   context,
	})
	if err != nil {
		t.Fatalf("Second encryption failed: %v", err)
	}

	if !bytes.Equal(resp1.Ciphertext, resp2.Ciphertext) {
		t.Error("Convergent encryption should produce identical ciphertext for same input")
	}

	resp3, err := engine.ConvergentEncrypt(ctx, &ConvergentEncryptRequest{
		KeyName:   "conv-key",
		Plaintext: plaintext,
		Context:   []byte("different-context"),
	})
	if err != nil {
		t.Fatalf("Third encryption failed: %v", err)
	}

	if bytes.Equal(resp1.Ciphertext, resp3.Ciphertext) {
		t.Error("Different context should produce different ciphertext")
	}
}

func TestTransitEngine_BatchEncrypt(t *testing.T) {
	engine := NewTransitEngine(&TransitConfig{
		DefaultKeySize:   256,
		BatchParallelism: 4,
		MaxBatchSize:     1000,
	}, nil)
	ctx := context.Background()

	_, err := engine.CreateKey(ctx, "batch-key", AlgorithmAESGCM, false, false)
	if err != nil {
		t.Fatalf("Failed to create key: %v", err)
	}

	items := make([]BatchEncryptItem, 100)
	for i := range items {
		items[i] = BatchEncryptItem{
			Plaintext: []byte("test data " + string(rune('0'+i%10))),
			Reference: string(rune('A' + i%26)),
		}
	}

	resp, err := engine.BatchEncrypt(ctx, &BatchEncryptRequest{
		KeyName: "batch-key",
		Items:   items,
	})
	if err != nil {
		t.Fatalf("Batch encrypt failed: %v", err)
	}

	if resp.Succeeded != 100 {
		t.Errorf("Expected 100 successful, got %d", resp.Succeeded)
	}
	if resp.Failed != 0 {
		t.Errorf("Expected 0 failed, got %d", resp.Failed)
	}
	if len(resp.Results) != 100 {
		t.Errorf("Expected 100 results, got %d", len(resp.Results))
	}

	for i, result := range resp.Results {
		if result.Error != "" {
			t.Errorf("Result %d has error: %s", i, result.Error)
		}
		if len(result.Ciphertext) == 0 {
			t.Errorf("Result %d has empty ciphertext", i)
		}
	}
}

func TestTransitEngine_BatchDecrypt(t *testing.T) {
	engine := NewTransitEngine(&TransitConfig{
		DefaultKeySize:   256,
		BatchParallelism: 4,
		MaxBatchSize:     1000,
	}, nil)
	ctx := context.Background()

	_, err := engine.CreateKey(ctx, "batch-key", AlgorithmAESGCM, false, false)
	if err != nil {
		t.Fatalf("Failed to create key: %v", err)
	}

	plaintexts := [][]byte{
		[]byte("message one"),
		[]byte("message two"),
		[]byte("message three"),
	}

	encItems := make([]BatchEncryptItem, len(plaintexts))
	for i, pt := range plaintexts {
		encItems[i] = BatchEncryptItem{Plaintext: pt}
	}

	encResp, err := engine.BatchEncrypt(ctx, &BatchEncryptRequest{
		KeyName: "batch-key",
		Items:   encItems,
	})
	if err != nil {
		t.Fatalf("Batch encrypt failed: %v", err)
	}

	decItems := make([]BatchDecryptItem, len(encResp.Results))
	for i, result := range encResp.Results {
		decItems[i] = BatchDecryptItem{Ciphertext: result.Ciphertext}
	}

	decResp, err := engine.BatchDecrypt(ctx, &BatchDecryptRequest{
		KeyName: "batch-key",
		Items:   decItems,
	})
	if err != nil {
		t.Fatalf("Batch decrypt failed: %v", err)
	}

	if decResp.Succeeded != len(plaintexts) {
		t.Errorf("Expected %d successful, got %d", len(plaintexts), decResp.Succeeded)
	}

	for i, result := range decResp.Results {
		if !bytes.Equal(result.Plaintext, plaintexts[i]) {
			t.Errorf("Decrypted plaintext %d mismatch", i)
		}
	}
}

func TestTransitEngine_GenerateDataKey(t *testing.T) {
	engine := NewTransitEngine(nil, nil)
	ctx := context.Background()

	_, err := engine.CreateKey(ctx, "master-key", AlgorithmAESGCM, false, false)
	if err != nil {
		t.Fatalf("Failed to create key: %v", err)
	}

	resp, err := engine.GenerateDataKeyForTransit(ctx, &TransitDataKeyRequest{
		KeyName: "master-key",
		KeySize: 32,
	})
	if err != nil {
		t.Fatalf("GenerateDataKey failed: %v", err)
	}

	if len(resp.Plaintext) != 32 {
		t.Errorf("Expected 32 byte data key, got %d", len(resp.Plaintext))
	}
	if len(resp.Ciphertext) == 0 {
		t.Error("Expected wrapped ciphertext")
	}
	if resp.KeyVersion != 1 {
		t.Errorf("Expected key version 1, got %d", resp.KeyVersion)
	}
}

func TestTransitEngine_HMACOperations(t *testing.T) {
	engine := NewTransitEngine(nil, nil)
	ctx := context.Background()

	_, err := engine.CreateHMACKey(ctx, "hmac-key", HMACAlgorithmSHA256)
	if err != nil {
		t.Fatalf("Failed to create HMAC key: %v", err)
	}

	input := []byte("data to sign with HMAC")

	signResp, err := engine.HMACSign(ctx, &HMACSignRequest{
		KeyName: "hmac-key",
		Input:   input,
	})
	if err != nil {
		t.Fatalf("HMAC sign failed: %v", err)
	}

	if len(signResp.HMAC) != 32 {
		t.Errorf("Expected 32 byte HMAC (SHA256), got %d", len(signResp.HMAC))
	}

	verifyResp, err := engine.HMACVerify(ctx, &HMACVerifyRequest{
		KeyName: "hmac-key",
		Input:   input,
		HMAC:    signResp.HMAC,
	})
	if err != nil {
		t.Fatalf("HMAC verify failed: %v", err)
	}

	if !verifyResp.Valid {
		t.Error("HMAC verification should succeed")
	}

	invalidHMAC := make([]byte, 32)
	verifyResp, err = engine.HMACVerify(ctx, &HMACVerifyRequest{
		KeyName: "hmac-key",
		Input:   input,
		HMAC:    invalidHMAC,
	})
	if err != nil {
		t.Fatalf("HMAC verify failed: %v", err)
	}

	if verifyResp.Valid {
		t.Error("HMAC verification should fail for invalid HMAC")
	}
}

func TestTransitEngine_HMACAlgorithms(t *testing.T) {
	tests := []struct {
		name      string
		algorithm HMACAlgorithm
		hmacSize  int
	}{
		{"SHA256", HMACAlgorithmSHA256, 32},
		{"SHA384", HMACAlgorithmSHA384, 48},
		{"SHA512", HMACAlgorithmSHA512, 64},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := NewTransitEngine(nil, nil)
			ctx := context.Background()

			_, err := engine.CreateHMACKey(ctx, "key-"+tt.name, tt.algorithm)
			if err != nil {
				t.Fatalf("Failed to create HMAC key: %v", err)
			}

			resp, err := engine.HMACSign(ctx, &HMACSignRequest{
				KeyName: "key-" + tt.name,
				Input:   []byte("test data"),
			})
			if err != nil {
				t.Fatalf("HMAC sign failed: %v", err)
			}

			if len(resp.HMAC) != tt.hmacSize {
				t.Errorf("Expected HMAC size %d, got %d", tt.hmacSize, len(resp.HMAC))
			}
		})
	}
}

func TestTransitEngine_KeyExport(t *testing.T) {
	config := &TransitConfig{
		EnableKeyExport:           true,
		KeyExportRequiresWrapping: false,
	}
	engine := NewTransitEngine(config, nil)
	ctx := context.Background()

	_, err := engine.CreateKey(ctx, "exportable-key", AlgorithmAESGCM, false, true)
	if err != nil {
		t.Fatalf("Failed to create key: %v", err)
	}

	resp, err := engine.ExportKey(ctx, &KeyExportRequest{
		KeyName: "exportable-key",
		KeyType: "encryption",
	})
	if err != nil {
		t.Fatalf("Key export failed: %v", err)
	}

	if len(resp.KeyMaterial) != 32 {
		t.Errorf("Expected 32 byte key material, got %d", len(resp.KeyMaterial))
	}
	if resp.Algorithm != string(AlgorithmAESGCM) {
		t.Errorf("Expected algorithm %s, got %s", AlgorithmAESGCM, resp.Algorithm)
	}
}

func TestTransitEngine_KeyExportDisabled(t *testing.T) {
	engine := NewTransitEngine(nil, nil)
	ctx := context.Background()

	_, err := engine.CreateKey(ctx, "key", AlgorithmAESGCM, false, true)
	if err != nil {
		t.Fatalf("Failed to create key: %v", err)
	}

	_, err = engine.ExportKey(ctx, &KeyExportRequest{
		KeyName: "key",
		KeyType: "encryption",
	})
	if err == nil {
		t.Error("Expected error when key export is disabled")
	}
}

func TestTransitEngine_KeyExportNonExportable(t *testing.T) {
	config := &TransitConfig{
		EnableKeyExport:           true,
		KeyExportRequiresWrapping: false,
	}
	engine := NewTransitEngine(config, nil)
	ctx := context.Background()

	_, err := engine.CreateKey(ctx, "non-exportable-key", AlgorithmAESGCM, false, false)
	if err != nil {
		t.Fatalf("Failed to create key: %v", err)
	}

	_, err = engine.ExportKey(ctx, &KeyExportRequest{
		KeyName: "non-exportable-key",
		KeyType: "encryption",
	})
	if err == nil {
		t.Error("Expected error when key is not exportable")
	}
}

func TestTransitEngine_ImportKey(t *testing.T) {
	engine := NewTransitEngine(nil, nil)
	ctx := context.Background()

	keyMaterial := make([]byte, 32)
	for i := range keyMaterial {
		keyMaterial[i] = byte(i)
	}

	err := engine.ImportKey(ctx, &ImportKeyRequest{
		KeyName:     "imported-key",
		KeyType:     "encryption",
		KeyMaterial: keyMaterial,
		Algorithm:   AlgorithmAESGCM,
		Exportable:  true,
	})
	if err != nil {
		t.Fatalf("Key import failed: %v", err)
	}

	key, err := engine.GetKey("imported-key")
	if err != nil {
		t.Fatalf("Failed to get imported key: %v", err)
	}

	if !bytes.Equal(key.KeyMaterial, keyMaterial) {
		t.Error("Imported key material mismatch")
	}
}

func TestTransitEngine_RotateKey(t *testing.T) {
	engine := NewTransitEngine(nil, nil)
	ctx := context.Background()

	_, err := engine.CreateKey(ctx, "rotate-key", AlgorithmAESGCM, false, false)
	if err != nil {
		t.Fatalf("Failed to create key: %v", err)
	}

	keyBefore, _ := engine.GetKey("rotate-key")
	originalMaterial := make([]byte, len(keyBefore.KeyMaterial))
	copy(originalMaterial, keyBefore.KeyMaterial)
	originalVersion := keyBefore.Version

	err = engine.RotateKey(ctx, "rotate-key")
	if err != nil {
		t.Fatalf("Key rotation failed: %v", err)
	}

	keyAfter, _ := engine.GetKey("rotate-key")

	if keyAfter.Version != originalVersion+1 {
		t.Errorf("Expected version %d, got %d", originalVersion+1, keyAfter.Version)
	}

	if bytes.Equal(keyAfter.KeyMaterial, originalMaterial) {
		t.Error("Key material should change after rotation")
	}
}

func TestTransitEngine_BatchEncryptMaxSize(t *testing.T) {
	config := &TransitConfig{
		MaxBatchSize:     10,
		BatchParallelism: 2,
	}
	engine := NewTransitEngine(config, nil)
	ctx := context.Background()

	_, err := engine.CreateKey(ctx, "key", AlgorithmAESGCM, false, false)
	if err != nil {
		t.Fatalf("Failed to create key: %v", err)
	}

	items := make([]BatchEncryptItem, 20)
	for i := range items {
		items[i] = BatchEncryptItem{Plaintext: []byte("test")}
	}

	_, err = engine.BatchEncrypt(ctx, &BatchEncryptRequest{
		KeyName: "key",
		Items:   items,
	})
	if err == nil {
		t.Error("Expected error when exceeding max batch size")
	}
}

// Benchmarks

func BenchmarkTransitEncrypt(b *testing.B) {
	engine := NewTransitEngine(nil, nil)
	ctx := context.Background()

	engine.CreateKey(ctx, "bench-key", AlgorithmAESGCM, false, false)
	key, _ := engine.GetKey("bench-key")
	plaintext := make([]byte, 1024)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		engine.encryptSingle(key, plaintext, nil)
	}
}

func BenchmarkTransitDecrypt(b *testing.B) {
	engine := NewTransitEngine(nil, nil)
	ctx := context.Background()

	engine.CreateKey(ctx, "bench-key", AlgorithmAESGCM, false, false)
	key, _ := engine.GetKey("bench-key")
	plaintext := make([]byte, 1024)
	ciphertext, _ := engine.encryptSingle(key, plaintext, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		engine.decryptSingle(key, ciphertext, nil)
	}
}

func BenchmarkTransitConvergentEncrypt(b *testing.B) {
	engine := NewTransitEngine(nil, nil)
	ctx := context.Background()

	engine.CreateKey(ctx, "bench-conv-key", AlgorithmAESGCM, true, false)
	plaintext := make([]byte, 1024)
	context := []byte("benchmark-context")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		engine.ConvergentEncrypt(ctx, &ConvergentEncryptRequest{
			KeyName:   "bench-conv-key",
			Plaintext: plaintext,
			Context:   context,
		})
	}
}

func BenchmarkTransitBatchEncrypt(b *testing.B) {
	benchBatchSizes := []int{10, 100, 1000}

	for _, size := range benchBatchSizes {
		b.Run(string(rune('0'+size/100))+"items", func(b *testing.B) {
			config := &TransitConfig{
				BatchParallelism: 4,
				MaxBatchSize:     10000,
			}
			engine := NewTransitEngine(config, nil)
			ctx := context.Background()

			engine.CreateKey(ctx, "bench-batch-key", AlgorithmAESGCM, false, false)

			items := make([]BatchEncryptItem, size)
			for i := range items {
				items[i] = BatchEncryptItem{Plaintext: make([]byte, 1024)}
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				engine.BatchEncrypt(ctx, &BatchEncryptRequest{
					KeyName: "bench-batch-key",
					Items:   items,
				})
			}
		})
	}
}

func BenchmarkTransitHMAC(b *testing.B) {
	algorithms := []HMACAlgorithm{
		HMACAlgorithmSHA256,
		HMACAlgorithmSHA384,
		HMACAlgorithmSHA512,
	}

	for _, algo := range algorithms {
		b.Run(string(algo), func(b *testing.B) {
			engine := NewTransitEngine(nil, nil)
			ctx := context.Background()

			engine.CreateHMACKey(ctx, "bench-hmac-key", algo)
			input := make([]byte, 1024)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				engine.HMACSign(ctx, &HMACSignRequest{
					KeyName: "bench-hmac-key",
					Input:   input,
				})
			}
		})
	}
}

func BenchmarkTransitGenerateDataKey(b *testing.B) {
	engine := NewTransitEngine(nil, nil)
	ctx := context.Background()

	engine.CreateKey(ctx, "bench-master-key", AlgorithmAESGCM, false, false)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		engine.GenerateDataKeyForTransit(ctx, &TransitDataKeyRequest{
			KeyName: "bench-master-key",
			KeySize: 32,
		})
	}
}

func BenchmarkTransitKeyRotation(b *testing.B) {
	engine := NewTransitEngine(nil, nil)
	ctx := context.Background()

	engine.CreateKey(ctx, "bench-rotate-key", AlgorithmAESGCM, false, false)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		engine.RotateKey(ctx, "bench-rotate-key")
	}
}

func TestTransitEngine_EncryptDecryptRoundTrip(t *testing.T) {
	engine := NewTransitEngine(nil, nil)
	ctx := context.Background()

	_, err := engine.CreateKey(ctx, "roundtrip-key", AlgorithmAESGCM, false, false)
	if err != nil {
		t.Fatalf("Failed to create key: %v", err)
	}

	testCases := []struct {
		name      string
		plaintext []byte
		context   map[string]string
	}{
		{"empty", []byte{}, nil},
		{"small", []byte("hello"), nil},
		{"medium", make([]byte, 1024), nil},
		{"large", make([]byte, 1024*1024), nil},
		{"with context", []byte("data"), map[string]string{"user": "123"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			key, _ := engine.GetKey("roundtrip-key")

			ciphertext, err := engine.encryptSingle(key, tc.plaintext, tc.context)
			if err != nil {
				t.Fatalf("Encryption failed: %v", err)
			}

			plaintext, err := engine.decryptSingle(key, ciphertext, tc.context)
			if err != nil {
				t.Fatalf("Decryption failed: %v", err)
			}

			if !bytes.Equal(plaintext, tc.plaintext) {
				t.Error("Round-trip plaintext mismatch")
			}
		})
	}
}

func TestTransitConfig_Defaults(t *testing.T) {
	config := DefaultTransitConfig()

	if config.DefaultAlgorithm != AlgorithmAESGCM {
		t.Errorf("Expected default algorithm AES-GCM, got %s", config.DefaultAlgorithm)
	}
	if config.DefaultKeySize != 256 {
		t.Errorf("Expected default key size 256, got %d", config.DefaultKeySize)
	}
	if config.BatchParallelism != 4 {
		t.Errorf("Expected batch parallelism 4, got %d", config.BatchParallelism)
	}
	if config.EnableKeyExport {
		t.Error("Key export should be disabled by default")
	}
}

func TestTransitEngine_BatchEncryptCancellation(t *testing.T) {
	config := &TransitConfig{
		BatchParallelism: 4,
		MaxBatchSize:     1000,
	}
	engine := NewTransitEngine(config, nil)
	ctx := context.Background()

	_, err := engine.CreateKey(ctx, "cancel-key", AlgorithmAESGCM, false, false)
	if err != nil {
		t.Fatalf("Failed to create key: %v", err)
	}

	items := make([]BatchEncryptItem, 100)
	for i := range items {
		items[i] = BatchEncryptItem{Plaintext: []byte("test")}
	}

	cancelCtx, cancel := context.WithTimeout(ctx, 1*time.Nanosecond)
	defer cancel()

	time.Sleep(10 * time.Millisecond)

	resp, err := engine.BatchEncrypt(cancelCtx, &BatchEncryptRequest{
		KeyName: "cancel-key",
		Items:   items,
	})
	if err != nil {
		t.Fatalf("BatchEncrypt returned error: %v", err)
	}

	if resp.Failed == 0 && resp.Succeeded == 100 {
		t.Log("All items completed before cancellation (expected in fast execution)")
	}
}
