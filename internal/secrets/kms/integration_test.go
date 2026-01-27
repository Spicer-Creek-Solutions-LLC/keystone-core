package kms

import (
	"context"
	"crypto/rand"
	"os"
	"testing"
	"time"
)

// Integration test environment variables
const (
	// AWS KMS
	envAWSRegion    = "KMS_TEST_AWS_REGION"
	envAWSKeyID     = "KMS_TEST_AWS_KEY_ID"
	envAWSAccessKey = "AWS_ACCESS_KEY_ID"     // Standard AWS env var
	envAWSSecretKey = "AWS_SECRET_ACCESS_KEY" // Standard AWS env var

	// Azure Key Vault
	envAzureVaultURL = "KMS_TEST_AZURE_VAULT_URL"
	envAzureKeyName  = "KMS_TEST_AZURE_KEY_NAME"
	envAzureTenantID = "AZURE_TENANT_ID" // Standard Azure env var
	envAzureClientID = "AZURE_CLIENT_ID"

	// GCP KMS
	envGCPProject  = "KMS_TEST_GCP_PROJECT"
	envGCPLocation = "KMS_TEST_GCP_LOCATION"
	envGCPKeyRing  = "KMS_TEST_GCP_KEY_RING"
	envGCPKeyName  = "KMS_TEST_GCP_KEY_NAME"
)

// skipIfNoAWS skips the test if AWS credentials/configuration are not available.
func skipIfNoAWS(t *testing.T) {
	t.Helper()
	if os.Getenv(envAWSRegion) == "" || os.Getenv(envAWSKeyID) == "" {
		t.Skip("AWS KMS integration test skipped: set KMS_TEST_AWS_REGION and KMS_TEST_AWS_KEY_ID")
	}
	if os.Getenv(envAWSAccessKey) == "" || os.Getenv(envAWSSecretKey) == "" {
		t.Skip("AWS KMS integration test skipped: AWS credentials not configured")
	}
}

// skipIfNoAzure skips the test if Azure credentials/configuration are not available.
func skipIfNoAzure(t *testing.T) {
	t.Helper()
	if os.Getenv(envAzureVaultURL) == "" || os.Getenv(envAzureKeyName) == "" {
		t.Skip("Azure KMS integration test skipped: set KMS_TEST_AZURE_VAULT_URL and KMS_TEST_AZURE_KEY_NAME")
	}
	if os.Getenv(envAzureTenantID) == "" || os.Getenv(envAzureClientID) == "" {
		t.Skip("Azure KMS integration test skipped: Azure credentials not configured")
	}
}

// skipIfNoGCP skips the test if GCP credentials/configuration are not available.
func skipIfNoGCP(t *testing.T) {
	t.Helper()
	required := []string{envGCPProject, envGCPLocation, envGCPKeyRing, envGCPKeyName}
	for _, env := range required {
		if os.Getenv(env) == "" {
			t.Skipf("GCP KMS integration test skipped: set %s", env)
		}
	}
}

// TestIntegrationAWSKMS tests the AWS KMS provider against real AWS infrastructure.
func TestIntegrationAWSKMS(t *testing.T) {
	skipIfNoAWS(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	keyID := os.Getenv(envAWSKeyID)
	config := &AWSConfig{
		Region: os.Getenv(envAWSRegion),
	}

	provider, err := NewAWSProvider(ctx, config)
	if err != nil {
		t.Fatalf("NewAWSProvider() failed: %v", err)
	}
	defer provider.Close()

	t.Run("Healthy", func(t *testing.T) {
		if !provider.Healthy(ctx) {
			t.Error("AWS KMS provider reports unhealthy")
		}
	})

	t.Run("EncryptDecryptRoundTrip", func(t *testing.T) {
		plaintext := []byte("integration-test-secret-data")

		encResp, err := provider.Encrypt(ctx, &EncryptRequest{
			KeyID:     keyID,
			Plaintext: plaintext,
		})
		if err != nil {
			t.Fatalf("Encrypt() failed: %v", err)
		}

		if len(encResp.Ciphertext) == 0 {
			t.Fatal("Encrypt() returned empty ciphertext")
		}

		decResp, err := provider.Decrypt(ctx, &DecryptRequest{
			KeyID:      keyID,
			Ciphertext: encResp.Ciphertext,
		})
		if err != nil {
			t.Fatalf("Decrypt() failed: %v", err)
		}

		if string(decResp.Plaintext) != string(plaintext) {
			t.Errorf("Decrypt() = %q, want %q", decResp.Plaintext, plaintext)
		}
	})

	t.Run("GenerateDataKey", func(t *testing.T) {
		dataKey, err := provider.GenerateDataKey(ctx, &GenerateDataKeyRequest{
			KeyID: keyID,
		})
		if err != nil {
			t.Fatalf("GenerateDataKey() failed: %v", err)
		}

		if len(dataKey.Plaintext) != 32 {
			t.Errorf("GenerateDataKey() plaintext length = %d, want 32", len(dataKey.Plaintext))
		}
		if len(dataKey.Ciphertext) == 0 {
			t.Error("GenerateDataKey() returned empty ciphertext")
		}
	})

	t.Run("KeyMetadata", func(t *testing.T) {
		metadata, err := provider.GetKeyMetadata(ctx, keyID)
		if err != nil {
			t.Fatalf("GetKeyMetadata() failed: %v", err)
		}

		if metadata.KeyID == "" {
			t.Error("GetKeyMetadata() returned empty KeyID")
		}
		if !metadata.Enabled {
			t.Error("GetKeyMetadata() shows key is disabled")
		}
	})

	t.Run("LargePayload", func(t *testing.T) {
		plaintext := make([]byte, 4096) // 4KB payload
		rand.Read(plaintext)

		encResp, err := provider.Encrypt(ctx, &EncryptRequest{
			KeyID:     keyID,
			Plaintext: plaintext,
		})
		if err != nil {
			t.Fatalf("Encrypt() with large payload failed: %v", err)
		}

		decResp, err := provider.Decrypt(ctx, &DecryptRequest{
			KeyID:      keyID,
			Ciphertext: encResp.Ciphertext,
		})
		if err != nil {
			t.Fatalf("Decrypt() with large payload failed: %v", err)
		}

		if string(decResp.Plaintext) != string(plaintext) {
			t.Error("Large payload round-trip failed")
		}
	})
}

// TestIntegrationAzureKMS tests the Azure Key Vault KMS provider.
func TestIntegrationAzureKMS(t *testing.T) {
	skipIfNoAzure(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	keyName := os.Getenv(envAzureKeyName)
	config := &AzureConfig{
		VaultURL: os.Getenv(envAzureVaultURL),
	}

	provider, err := NewAzureProvider(ctx, config)
	if err != nil {
		t.Fatalf("NewAzureProvider() failed: %v", err)
	}
	defer provider.Close()

	t.Run("Healthy", func(t *testing.T) {
		if !provider.Healthy(ctx) {
			t.Error("Azure KMS provider reports unhealthy")
		}
	})

	t.Run("EncryptDecryptRoundTrip", func(t *testing.T) {
		plaintext := []byte("azure-integration-test-data")

		encResp, err := provider.Encrypt(ctx, &EncryptRequest{
			KeyID:     keyName,
			Plaintext: plaintext,
		})
		if err != nil {
			t.Fatalf("Encrypt() failed: %v", err)
		}

		decResp, err := provider.Decrypt(ctx, &DecryptRequest{
			KeyID:      keyName,
			Ciphertext: encResp.Ciphertext,
		})
		if err != nil {
			t.Fatalf("Decrypt() failed: %v", err)
		}

		if string(decResp.Plaintext) != string(plaintext) {
			t.Errorf("Decrypt() = %q, want %q", decResp.Plaintext, plaintext)
		}
	})

	t.Run("WrapUnwrapKey", func(t *testing.T) {
		keyToWrap := make([]byte, 32)
		rand.Read(keyToWrap)

		wrapResp, err := provider.WrapKey(ctx, &WrapKeyRequest{
			WrapperKeyID: keyName,
			KeyToWrap:    keyToWrap,
		})
		if err != nil {
			t.Fatalf("WrapKey() failed: %v", err)
		}

		unwrapResp, err := provider.UnwrapKey(ctx, &UnwrapKeyRequest{
			WrapperKeyID: keyName,
			WrappedKey:   wrapResp.WrappedKey,
		})
		if err != nil {
			t.Fatalf("UnwrapKey() failed: %v", err)
		}

		if string(unwrapResp.PlaintextKey) != string(keyToWrap) {
			t.Error("WrapKey/UnwrapKey round-trip failed")
		}
	})
}

// TestIntegrationGCPKMS tests the GCP Cloud KMS provider.
func TestIntegrationGCPKMS(t *testing.T) {
	skipIfNoGCP(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	keyName := os.Getenv(envGCPKeyName)
	config := &GCPConfig{
		ProjectID: os.Getenv(envGCPProject),
		Location:  os.Getenv(envGCPLocation),
		KeyRing:   os.Getenv(envGCPKeyRing),
	}

	provider, err := NewGCPProvider(ctx, config)
	if err != nil {
		t.Fatalf("NewGCPProvider() failed: %v", err)
	}
	defer provider.Close()

	t.Run("Healthy", func(t *testing.T) {
		if !provider.Healthy(ctx) {
			t.Error("GCP KMS provider reports unhealthy")
		}
	})

	t.Run("EncryptDecryptRoundTrip", func(t *testing.T) {
		plaintext := []byte("gcp-integration-test-data")

		encResp, err := provider.Encrypt(ctx, &EncryptRequest{
			KeyID:     keyName,
			Plaintext: plaintext,
		})
		if err != nil {
			t.Fatalf("Encrypt() failed: %v", err)
		}

		decResp, err := provider.Decrypt(ctx, &DecryptRequest{
			KeyID:      keyName,
			Ciphertext: encResp.Ciphertext,
		})
		if err != nil {
			t.Fatalf("Decrypt() failed: %v", err)
		}

		if string(decResp.Plaintext) != string(plaintext) {
			t.Errorf("Decrypt() = %q, want %q", decResp.Plaintext, plaintext)
		}
	})

	t.Run("GenerateDataKey", func(t *testing.T) {
		dataKey, err := provider.GenerateDataKey(ctx, &GenerateDataKeyRequest{
			KeyID: keyName,
		})
		if err != nil {
			t.Fatalf("GenerateDataKey() failed: %v", err)
		}

		if len(dataKey.Plaintext) == 0 {
			t.Error("GenerateDataKey() returned empty plaintext")
		}
		if len(dataKey.Ciphertext) == 0 {
			t.Error("GenerateDataKey() returned empty ciphertext")
		}
	})
}

// TestIntegrationMultiProvider tests failover between multiple providers.
func TestIntegrationMultiProvider(t *testing.T) {
	if os.Getenv(envAWSKeyID) == "" && os.Getenv(envAzureKeyName) == "" && os.Getenv(envGCPKeyName) == "" {
		t.Skip("Multi-provider integration test skipped: no providers configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var providers []Provider

	// Add available providers
	if os.Getenv(envAWSRegion) != "" && os.Getenv(envAWSKeyID) != "" {
		awsProvider, err := NewAWSProvider(ctx, &AWSConfig{
			Region: os.Getenv(envAWSRegion),
		})
		if err == nil {
			providers = append(providers, awsProvider)
			t.Cleanup(func() { awsProvider.Close() })
		}
	}

	if os.Getenv(envAzureVaultURL) != "" && os.Getenv(envAzureKeyName) != "" {
		azureProvider, err := NewAzureProvider(ctx, &AzureConfig{
			VaultURL: os.Getenv(envAzureVaultURL),
		})
		if err == nil {
			providers = append(providers, azureProvider)
			t.Cleanup(func() { azureProvider.Close() })
		}
	}

	if len(providers) < 2 {
		t.Skip("Multi-provider test requires at least 2 providers")
	}

	t.Run("AllProvidersHealthy", func(t *testing.T) {
		for _, p := range providers {
			if !p.Healthy(ctx) {
				t.Errorf("Provider %s reports unhealthy", p.Name())
			}
		}
	})
}

// TestIntegrationTransitEngine tests transit encryption with a real provider.
func TestIntegrationTransitEngine(t *testing.T) {
	skipIfNoAWS(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	awsProvider, err := NewAWSProvider(ctx, &AWSConfig{
		Region: os.Getenv(envAWSRegion),
	})
	if err != nil {
		t.Fatalf("NewAWSProvider() failed: %v", err)
	}
	defer awsProvider.Close()

	config := DefaultTransitConfig()
	engine := NewTransitEngine(config, awsProvider)

	t.Run("CreateKey", func(t *testing.T) {
		key, err := engine.CreateKey(ctx, "integration-test-key", AlgorithmAESGCM, false, false)
		if err != nil {
			t.Fatalf("CreateKey() failed: %v", err)
		}

		if key.Name != "integration-test-key" {
			t.Errorf("CreateKey() name = %q, want %q", key.Name, "integration-test-key")
		}
	})

	t.Run("GetKey", func(t *testing.T) {
		// First create a key
		_, err := engine.CreateKey(ctx, "get-test-key", AlgorithmAESGCM, false, false)
		if err != nil {
			t.Fatalf("CreateKey() failed: %v", err)
		}

		key, err := engine.GetKey("get-test-key")
		if err != nil {
			t.Fatalf("GetKey() failed: %v", err)
		}

		if key.Name != "get-test-key" {
			t.Errorf("GetKey() name = %q, want %q", key.Name, "get-test-key")
		}
	})

	t.Run("BatchEncrypt", func(t *testing.T) {
		// Create a key for batch operations
		_, err := engine.CreateKey(ctx, "batch-test-key", AlgorithmAESGCM, false, false)
		if err != nil {
			t.Fatalf("CreateKey() failed: %v", err)
		}

		req := &BatchEncryptRequest{
			KeyName: "batch-test-key",
			Items: []BatchEncryptItem{
				{Plaintext: []byte("batch-item-1")},
				{Plaintext: []byte("batch-item-2")},
				{Plaintext: []byte("batch-item-3")},
			},
		}

		resp, err := engine.BatchEncrypt(ctx, req)
		if err != nil {
			t.Fatalf("BatchEncrypt() failed: %v", err)
		}

		if len(resp.Results) != 3 {
			t.Errorf("BatchEncrypt() returned %d results, want 3", len(resp.Results))
		}
	})
}

// TestIntegrationKeyHierarchy tests key hierarchy with a real provider.
func TestIntegrationKeyHierarchy(t *testing.T) {
	skipIfNoAWS(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	keyID := os.Getenv(envAWSKeyID)
	awsProvider, err := NewAWSProvider(ctx, &AWSConfig{
		Region: os.Getenv(envAWSRegion),
	})
	if err != nil {
		t.Fatalf("NewAWSProvider() failed: %v", err)
	}
	defer awsProvider.Close()

	config := &KeyHierarchyConfig{
		MasterKeyID:         keyID,
		RotationInterval:    24 * time.Hour,
		CacheDataKeyLocally: true,
	}

	manager, err := NewKeyHierarchyManager(awsProvider, config)
	if err != nil {
		t.Fatalf("NewKeyHierarchyManager() failed: %v", err)
	}

	t.Run("DeriveKey", func(t *testing.T) {
		key1, err := manager.DeriveKey(ctx, KeyPurposeCache, []byte("test-info"), 32)
		if err != nil {
			t.Fatalf("DeriveKey() failed: %v", err)
		}

		if len(key1) != 32 {
			t.Errorf("DeriveKey() returned key of length %d, want 32", len(key1))
		}

		// Same parameters should return same key (from cache or derivation)
		key2, err := manager.DeriveKey(ctx, KeyPurposeCache, []byte("test-info"), 32)
		if err != nil {
			t.Fatalf("DeriveKey() second call failed: %v", err)
		}

		if string(key1) != string(key2) {
			t.Error("DeriveKey() returned different keys for same parameters")
		}
	})

	t.Run("DifferentPurposes", func(t *testing.T) {
		cacheKey, err := manager.DeriveKey(ctx, KeyPurposeCache, []byte("test-info"), 32)
		if err != nil {
			t.Fatalf("DeriveKey(cache) failed: %v", err)
		}

		auditKey, err := manager.DeriveKey(ctx, KeyPurposeAudit, []byte("test-info"), 32)
		if err != nil {
			t.Fatalf("DeriveKey(audit) failed: %v", err)
		}

		if string(cacheKey) == string(auditKey) {
			t.Error("Different purposes returned same key")
		}
	})
}

// TestIntegrationConcurrency tests concurrent access to KMS providers.
func TestIntegrationConcurrency(t *testing.T) {
	skipIfNoAWS(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	keyID := os.Getenv(envAWSKeyID)
	provider, err := NewAWSProvider(ctx, &AWSConfig{
		Region: os.Getenv(envAWSRegion),
	})
	if err != nil {
		t.Fatalf("NewAWSProvider() failed: %v", err)
	}
	defer provider.Close()

	const numGoroutines = 10
	const opsPerGoroutine = 5

	errChan := make(chan error, numGoroutines*opsPerGoroutine)

	for i := 0; i < numGoroutines; i++ {
		go func(workerID int) {
			for j := 0; j < opsPerGoroutine; j++ {
				plaintext := []byte("concurrent-test-" + string(rune('0'+workerID)) + "-" + string(rune('0'+j)))

				encResp, err := provider.Encrypt(ctx, &EncryptRequest{
					KeyID:     keyID,
					Plaintext: plaintext,
				})
				if err != nil {
					errChan <- err
					continue
				}

				decResp, err := provider.Decrypt(ctx, &DecryptRequest{
					KeyID:      keyID,
					Ciphertext: encResp.Ciphertext,
				})
				if err != nil {
					errChan <- err
					continue
				}

				if string(decResp.Plaintext) != string(plaintext) {
					errChan <- err
				}
			}
		}(i)
	}

	// Wait and collect errors
	close(errChan)
	var errors []error
	for err := range errChan {
		if err != nil {
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		t.Errorf("Concurrent operations had %d errors: %v", len(errors), errors[0])
	}
}
