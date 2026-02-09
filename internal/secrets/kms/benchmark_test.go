package kms

import (
	"context"
	"crypto/rand"
	"fmt"
	"testing"
	"time"
)

// Comprehensive benchmark suite for KMS operations

// BenchmarkKMSEncrypt benchmarks KMS encryption operations.
func BenchmarkKMSEncrypt(b *testing.B) {
	provider := newBenchMockProvider()
	ctx := context.Background()

	sizes := []int{16, 256, 1024, 4096, 65536}

	for _, size := range sizes {
		plaintext := make([]byte, size)
		rand.Read(plaintext)

		b.Run(fmt.Sprintf("size_%d", size), func(b *testing.B) {
			b.SetBytes(int64(size))
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				_, err := provider.Encrypt(ctx, &EncryptRequest{
					KeyID:     "test-key",
					Plaintext: plaintext,
				})
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkKMSDecrypt benchmarks KMS decryption operations.
func BenchmarkKMSDecrypt(b *testing.B) {
	provider := newBenchMockProvider()
	ctx := context.Background()

	sizes := []int{16, 256, 1024, 4096}

	for _, size := range sizes {
		plaintext := make([]byte, size)
		rand.Read(plaintext)

		// Pre-encrypt
		resp, _ := provider.Encrypt(ctx, &EncryptRequest{
			KeyID:     "test-key",
			Plaintext: plaintext,
		})

		b.Run(fmt.Sprintf("size_%d", size), func(b *testing.B) {
			b.SetBytes(int64(size))
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				_, err := provider.Decrypt(ctx, &DecryptRequest{
					KeyID:      "test-key",
					Ciphertext: resp.Ciphertext,
				})
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkKMSEncryptParallel benchmarks parallel KMS encryption.
func BenchmarkKMSEncryptParallel(b *testing.B) {
	provider := newBenchMockProvider()
	ctx := context.Background()

	plaintext := make([]byte, 1024)
	rand.Read(plaintext)

	b.SetBytes(1024)
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := provider.Encrypt(ctx, &EncryptRequest{
				KeyID:     "test-key",
				Plaintext: plaintext,
			})
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkKMSDataKeyGeneration benchmarks data key generation.
func BenchmarkKMSDataKeyGeneration(b *testing.B) {
	provider := newBenchMockProvider()
	ctx := context.Background()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := provider.GenerateDataKey(ctx, &GenerateDataKeyRequest{
			KeyID: "master-key",
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSecureMemoryZero benchmarks secure memory zeroing.
func BenchmarkSecureMemoryZero(b *testing.B) {
	sizes := []int{32, 256, 1024, 4096, 65536}

	for _, size := range sizes {
		data := make([]byte, size)

		b.Run(fmt.Sprintf("size_%d", size), func(b *testing.B) {
			b.SetBytes(int64(size))
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				SecureZero(data)
			}
		})
	}
}

// BenchmarkSecureMemoryCompare benchmarks constant-time comparison.
func BenchmarkSecureMemoryCompare(b *testing.B) {
	sizes := []int{32, 64, 128, 256}

	for _, size := range sizes {
		a := make([]byte, size)
		c := make([]byte, size)
		rand.Read(a)
		copy(c, a)

		b.Run(fmt.Sprintf("size_%d", size), func(b *testing.B) {
			b.SetBytes(int64(size))
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				SecureCompare(a, c)
			}
		})
	}
}

// BenchmarkLogMasking benchmarks log masking.
func BenchmarkLogMasking(b *testing.B) {
	masker := NewLogMasker()

	messages := []string{
		"Simple message without secrets",
		"Password: secretpassword123 in message",
		"API key: sk_live_abcdef123456 and token: eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
		"Connection string: postgres://user:password@localhost:5432/db",
	}

	for i, msg := range messages {
		b.Run(fmt.Sprintf("msg_%d_len_%d", i, len(msg)), func(b *testing.B) {
			b.SetBytes(int64(len(msg)))
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				masker.Mask(msg)
			}
		})
	}
}

// BenchmarkAnomalyDetection benchmarks anomaly detection.
func BenchmarkAnomalyDetection(b *testing.B) {
	config := DefaultAnomalyConfig()
	detector := NewAnomalyDetector(config)

	ctx := context.Background()

	b.Run("record_access", func(b *testing.B) {
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			detector.RecordAccess(ctx, &SecretAccess{
				Timestamp: time.Now(),
				Principal: fmt.Sprintf("user-%d", i%100),
				SecretID:  fmt.Sprintf("secret-%d", i%50),
				SourceIP:  "10.0.1.50",
				Success:   true,
			})
		}
	})

	// Pre-populate for analysis benchmark
	for i := 0; i < 1000; i++ {
		detector.RecordAccess(ctx, &SecretAccess{
			Timestamp: time.Now(),
			Principal: fmt.Sprintf("user-%d", i%10),
			SecretID:  fmt.Sprintf("secret-%d", i%50),
			SourceIP:  "10.0.1.50",
			Success:   i%20 != 0,
		})
	}
}

// BenchmarkComplianceReportGeneration benchmarks compliance report generation.
func BenchmarkComplianceReportGeneration(b *testing.B) {
	config := DefaultComplianceConfig()
	reporter := NewComplianceReporter(config, nil)

	// Register keys
	now := time.Now()
	for i := 0; i < 100; i++ {
		lastRotated := now.Add(-time.Duration(i) * 24 * time.Hour)
		nextRotation := now.Add(time.Duration(365-i) * 24 * time.Hour)

		reporter.RegisterKey(KeyInventoryItem{
			KeyID:        fmt.Sprintf("key-%d", i),
			KeyType:      "symmetric",
			Algorithm:    "aes-256-gcm",
			KeySize:      256,
			CreatedAt:    now.Add(-time.Duration(i+30) * 24 * time.Hour),
			LastRotated:  &lastRotated,
			NextRotation: &nextRotation,
			Status:       "active",
		})
	}

	// Record accesses
	for i := 0; i < 1000; i++ {
		reporter.RecordAccess(
			fmt.Sprintf("user-%d", i%10),
			fmt.Sprintf("/secrets/path-%d", i%50),
			"read",
			i%20 != 0,
		)
	}

	period := ReportPeriod{
		Start: now.Add(-30 * 24 * time.Hour),
		End:   now,
	}

	ctx := context.Background()

	frameworks := []ComplianceFramework{
		FrameworkSOC2,
		FrameworkPCIDSS,
		FrameworkHIPAA,
	}

	for _, fw := range frameworks {
		b.Run(string(fw), func(b *testing.B) {
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				_, err := reporter.GenerateReport(ctx, fw, period)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkPentestExecution benchmarks security test execution.
func BenchmarkPentestExecution(b *testing.B) {
	provider := &MockPentestProvider{}
	config := DefaultPentestConfig()
	config.Categories = []PentestCategory{CategoryAuthentication}

	suite := NewPentestSuite(config, provider)
	ctx := context.Background()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		suite.RunAllTests(ctx)
	}
}

// Helper: create mock provider for benchmarks
func newBenchMockProvider() *benchMockProvider {
	return &benchMockProvider{
		keys: make(map[string][]byte),
	}
}

type benchMockProvider struct {
	keys map[string][]byte
}

func (m *benchMockProvider) Type() ProviderType               { return "mock" }
func (m *benchMockProvider) Name() string                     { return "mock-bench" }
func (m *benchMockProvider) Healthy(ctx context.Context) bool { return true }
func (m *benchMockProvider) Close() error                     { return nil }

func (m *benchMockProvider) GetKeyMetadata(ctx context.Context, keyID string) (*KeyMetadata, error) {
	return &KeyMetadata{KeyID: keyID, Enabled: true}, nil
}

func (m *benchMockProvider) Encrypt(ctx context.Context, req *EncryptRequest) (*EncryptResponse, error) {
	// Simple mock encryption - just prepend "enc:" for benchmarking
	ciphertext := append([]byte("enc:"), req.Plaintext...)
	return &EncryptResponse{Ciphertext: ciphertext}, nil
}

func (m *benchMockProvider) Decrypt(ctx context.Context, req *DecryptRequest) (*DecryptResponse, error) {
	if len(req.Ciphertext) < 4 {
		return nil, fmt.Errorf("invalid ciphertext")
	}
	return &DecryptResponse{Plaintext: req.Ciphertext[4:]}, nil
}

func (m *benchMockProvider) GenerateDataKey(ctx context.Context, req *GenerateDataKeyRequest) (*DataKey, error) {
	key := make([]byte, 32)
	rand.Read(key)
	return &DataKey{Plaintext: key, Ciphertext: append([]byte("wrapped:"), key...)}, nil
}

func (m *benchMockProvider) WrapKey(ctx context.Context, req *WrapKeyRequest) (*WrapKeyResponse, error) {
	return &WrapKeyResponse{WrappedKey: append([]byte("wrapped:"), req.KeyToWrap...)}, nil
}

func (m *benchMockProvider) UnwrapKey(ctx context.Context, req *UnwrapKeyRequest) (*UnwrapKeyResponse, error) {
	if len(req.WrappedKey) < 8 {
		return nil, fmt.Errorf("invalid wrapped key")
	}
	return &UnwrapKeyResponse{PlaintextKey: req.WrappedKey[8:]}, nil
}
