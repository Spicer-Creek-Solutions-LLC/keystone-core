package verify

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultTrustPolicy(t *testing.T) {
	policy := NewTrustPolicy()

	// Add a trusted key
	identity := "test@example.com"
	publicKey := []byte("test-public-key")

	err := policy.AddTrustedKey(identity, publicKey)
	if err != nil {
		t.Fatalf("failed to add trusted key: %v", err)
	}

	// Check if trusted
	if !policy.IsTrusted(identity) {
		t.Error("expected identity to be trusted")
	}

	// List trusted keys
	keys := policy.ListTrustedKeys()
	if len(keys) != 1 {
		t.Errorf("expected 1 trusted key, got %d", len(keys))
	}

	// Remove trusted key
	err = policy.RemoveTrustedKey(identity)
	if err != nil {
		t.Fatalf("failed to remove trusted key: %v", err)
	}

	if policy.IsTrusted(identity) {
		t.Error("expected identity to not be trusted after removal")
	}
}

func TestTrustPolicyKeyFingerprint(t *testing.T) {
	policy := NewTrustPolicy()

	identity := "test@example.com"
	publicKey := []byte("test-public-key")

	err := policy.AddTrustedKey(identity, publicKey)
	if err != nil {
		t.Fatalf("failed to add trusted key: %v", err)
	}

	// Compute fingerprint
	fingerprint := computeKeyFingerprint(publicKey)

	// Check if fingerprint is trusted
	if !policy.IsTrusted(fingerprint) {
		t.Error("expected fingerprint to be trusted")
	}
}

func TestCompositeTrustPolicy(t *testing.T) {
	policy1 := NewTrustPolicy()
	policy2 := NewTrustPolicy()

	composite := NewCompositeTrustPolicy(policy1, policy2)

	// Add key to first policy
	identity1 := "test1@example.com"
	policy1.AddTrustedKey(identity1, []byte("key1"))

	// Add key to second policy
	identity2 := "test2@example.com"
	policy2.AddTrustedKey(identity2, []byte("key2"))

	// Both should be trusted through composite
	if !composite.IsTrusted(identity1) {
		t.Error("expected identity1 to be trusted")
	}

	if !composite.IsTrusted(identity2) {
		t.Error("expected identity2 to be trusted")
	}

	// List should include both
	keys := composite.ListTrustedKeys()
	if len(keys) != 2 {
		t.Errorf("expected 2 trusted keys, got %d", len(keys))
	}
}

func TestInMemorySumDB(t *testing.T) {
	sumDB := NewInMemorySumDB()

	moduleName := "vendor/test-module"
	version := "v1.0.0"
	hash := "sha256:abcdef1234567890"

	// Submit a hash
	err := sumDB.Submit(moduleName, version, hash)
	if err != nil {
		t.Fatalf("failed to submit hash: %v", err)
	}

	// Lookup the hash
	retrievedHash, err := sumDB.Lookup(moduleName, version)
	if err != nil {
		t.Fatalf("failed to lookup hash: %v", err)
	}

	if retrievedHash != hash {
		t.Errorf("expected hash %s, got %s", hash, retrievedHash)
	}

	// Verify the hash
	valid, err := sumDB.Verify(moduleName, version, hash)
	if err != nil {
		t.Fatalf("verification failed: %v", err)
	}

	if !valid {
		t.Error("expected hash to be valid")
	}

	// Verify with wrong hash
	wrongHash := "sha256:0000000000000000"
	valid, err = sumDB.Verify(moduleName, version, wrongHash)
	if err != nil {
		t.Fatalf("verification failed: %v", err)
	}

	if valid {
		t.Error("expected hash to be invalid")
	}
}

func TestInMemorySumDB_DuplicateSubmit(t *testing.T) {
	sumDB := NewInMemorySumDB()

	moduleName := "vendor/test-module"
	version := "v1.0.0"
	hash := "sha256:abcdef1234567890"

	// Submit hash
	err := sumDB.Submit(moduleName, version, hash)
	if err != nil {
		t.Fatalf("failed to submit hash: %v", err)
	}

	// Submit same hash again (should succeed)
	err = sumDB.Submit(moduleName, version, hash)
	if err != nil {
		t.Errorf("duplicate submit with same hash should succeed: %v", err)
	}

	// Submit different hash (should fail)
	differentHash := "sha256:different123456"
	err = sumDB.Submit(moduleName, version, differentHash)
	if err == nil {
		t.Error("duplicate submit with different hash should fail")
	}
}

func TestModuleVerifier_Basic(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a test module
	modulePath := filepath.Join(tmpDir, "test-module.tar.gz")
	testData := []byte("test module data")
	if err := os.WriteFile(modulePath, testData, 0644); err != nil {
		t.Fatalf("failed to create test module: %v", err)
	}

	// Create verifier
	opts := DefaultVerificationOptions()
	opts.RequireSignature = false // Skip signature for this test
	opts.RequireSumDB = false     // Skip SumDB for this test
	opts.RequireHashMatch = false // Skip hash matching since we don't provide expected hash

	verifier, err := NewModuleVerifier(opts)
	if err != nil {
		t.Fatalf("NewModuleVerifier failed: %v", err)
	}

	// Verify module
	result, err := verifier.Verify(modulePath, opts)
	if err != nil {
		t.Fatalf("verification failed: %v", err)
	}

	if !result.Verified {
		t.Errorf("expected verification to succeed, errors: %v", result.Errors)
	}
}

func TestModuleVerifier_WithArtifact(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a test module (plain file, not ZIP)
	modulePath := filepath.Join(tmpDir, "test-module.tar.gz")
	testData := []byte("test module data")
	if err := os.WriteFile(modulePath, testData, 0644); err != nil {
		t.Fatalf("failed to create test module: %v", err)
	}

	// Compute expected hash
	hashVerifier := NewDefaultHashVerifier()
	expectedHash, err := hashVerifier.ComputeHash(modulePath)
	if err != nil {
		t.Fatalf("failed to compute hash: %v", err)
	}

	// Create artifact
	artifact := &ModuleArtifact{
		Name:    "vendor/test-module",
		Version: "v1.0.0",
		Path:    modulePath,
		Hash:    expectedHash,
	}

	// Create verifier
	opts := DefaultVerificationOptions()
	opts.RequireSignature = false
	opts.RequireSumDB = false

	verifier, err := NewModuleVerifier(opts)
	if err != nil {
		t.Fatalf("NewModuleVerifier failed: %v", err)
	}

	// Verify artifact
	report, err := verifier.VerifyArtifact(artifact, opts)
	if err != nil {
		t.Fatalf("verification failed: %v", err)
	}

	if !report.Result.Verified {
		t.Error("expected verification to succeed")
	}

	if !report.Result.HashValid {
		t.Error("expected hash to be valid")
	}

	if report.ComputedHash != expectedHash {
		t.Errorf("hash mismatch: expected %s, got %s", expectedHash, report.ComputedHash)
	}
}

func TestModuleVerifier_HashMismatch(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a test module
	modulePath := filepath.Join(tmpDir, "test-module.zip")
	testData := []byte("test module data")
	if err := os.WriteFile(modulePath, testData, 0644); err != nil {
		t.Fatalf("failed to create test module: %v", err)
	}

	// Create artifact with wrong hash
	artifact := &ModuleArtifact{
		Name:    "vendor/test-module",
		Version: "v1.0.0",
		Path:    modulePath,
		Hash:    "sha256:0000000000000000000000000000000000000000000000000000000000000000",
	}

	// Create verifier
	opts := DefaultVerificationOptions()
	opts.RequireSignature = false
	opts.RequireSumDB = false

	verifier, err := NewModuleVerifier(opts)
	if err != nil {
		t.Fatalf("NewModuleVerifier failed: %v", err)
	}

	// Verify artifact
	report, err := verifier.VerifyArtifact(artifact, opts)
	if err != nil {
		t.Fatalf("verification failed: %v", err)
	}

	if report.Result.Verified {
		t.Error("expected verification to fail due to hash mismatch")
	}

	if report.Result.HashValid {
		t.Error("expected hash to be invalid")
	}

	if len(report.Result.Errors) == 0 {
		t.Error("expected errors to be recorded")
	}
}

func TestModuleVerifier_WithSumDB(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a test module (plain file)
	modulePath := filepath.Join(tmpDir, "test-module.tar.gz")
	testData := []byte("test module data")
	if err := os.WriteFile(modulePath, testData, 0644); err != nil {
		t.Fatalf("failed to create test module: %v", err)
	}

	// Compute hash
	hashVerifier := NewDefaultHashVerifier()
	computedHash, err := hashVerifier.ComputeHash(modulePath)
	if err != nil {
		t.Fatalf("failed to compute hash: %v", err)
	}

	// Create SumDB with the hash
	sumDB := NewInMemorySumDB()
	moduleName := "vendor/test-module"
	version := "v1.0.0"
	sumDB.Submit(moduleName, version, computedHash)

	// Create artifact
	artifact := &ModuleArtifact{
		Name:    moduleName,
		Version: version,
		Path:    modulePath,
	}

	// Create verifier with SumDB
	opts := DefaultVerificationOptions()
	opts.RequireSignature = false
	opts.RequireSumDB = true

	verifier, err := NewModuleVerifier(opts)
	if err != nil {
		t.Fatalf("NewModuleVerifier failed: %v", err)
	}
	verifier.SetSumDB(sumDB)

	// Verify artifact
	report, err := verifier.VerifyArtifact(artifact, opts)
	if err != nil {
		t.Fatalf("verification failed: %v", err)
	}

	if !report.Result.Verified {
		t.Errorf("expected verification to succeed, errors: %v", report.Result.Errors)
	}

	if !report.Result.SumDBVerified {
		t.Error("expected SumDB verification to succeed")
	}
}

func TestVerificationOptions_Insecure(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a test module
	modulePath := filepath.Join(tmpDir, "test-module.tar.gz")
	testData := []byte("test module data")
	if err := os.WriteFile(modulePath, testData, 0644); err != nil {
		t.Fatalf("failed to create test module: %v", err)
	}

	// Create artifact with wrong hash
	artifact := &ModuleArtifact{
		Name:    "vendor/test-module",
		Version: "v1.0.0",
		Path:    modulePath,
		Hash:    "sha256:0000000000000000000000000000000000000000000000000000000000000000",
	}

	// Create verifier with insecure mode
	opts := DefaultVerificationOptions()
	opts.RequireSignature = false
	opts.RequireSumDB = false
	opts.AllowInsecure = true

	verifier, err := NewModuleVerifier(opts)
	if err != nil {
		t.Fatalf("NewModuleVerifier failed: %v", err)
	}

	// Verify artifact
	report, err := verifier.VerifyArtifact(artifact, opts)
	if err != nil {
		t.Fatalf("verification failed: %v", err)
	}

	// Should succeed in insecure mode despite hash mismatch
	if !report.Result.Verified {
		t.Errorf("expected verification to succeed in insecure mode, errors: %v", report.Result.Errors)
	}

	// Should have warnings
	if len(report.Result.Warnings) == 0 {
		t.Error("expected warnings in insecure mode")
	}
}

func TestVerificationResult_IsValid(t *testing.T) {
	result := &VerificationResult{
		Verified: true,
	}

	if !result.IsValid() {
		t.Error("expected result to be valid")
	}

	result.AddError(ErrHashMismatch)

	if result.IsValid() {
		t.Error("expected result to be invalid after error")
	}

	if result.Verified {
		t.Error("expected Verified to be false after error")
	}
}
