package verify

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateKeyPair_RSA(t *testing.T) {
	privateKey, publicKey, err := GenerateKeyPair("rsa", 2048)
	if err != nil {
		t.Fatalf("GenerateKeyPair(rsa) failed: %v", err)
	}

	if len(privateKey) == 0 {
		t.Error("private key is empty")
	}
	if len(publicKey) == 0 {
		t.Error("public key is empty")
	}

	// Verify PEM format (PKCS8 for all key types)
	if !strings.Contains(string(privateKey), "-----BEGIN PRIVATE KEY-----") {
		t.Errorf("private key not in PEM format: %s", string(privateKey[:50]))
	}
	if !strings.Contains(string(publicKey), "-----BEGIN PUBLIC KEY-----") {
		t.Errorf("public key not in PEM format: %s", string(publicKey[:50]))
	}
}

func TestGenerateKeyPair_ECDSA(t *testing.T) {
	privateKey, publicKey, err := GenerateKeyPair("ecdsa", 0)
	if err != nil {
		t.Fatalf("GenerateKeyPair(ecdsa) failed: %v", err)
	}

	if len(privateKey) == 0 {
		t.Error("private key is empty")
	}
	if len(publicKey) == 0 {
		t.Error("public key is empty")
	}

	// Verify PEM format (PKCS8 for all key types)
	if !strings.Contains(string(privateKey), "-----BEGIN PRIVATE KEY-----") {
		t.Errorf("private key not in PEM format: %s", string(privateKey[:50]))
	}
}

func TestGenerateKeyPair_Ed25519(t *testing.T) {
	privateKey, publicKey, err := GenerateKeyPair("ed25519", 0)
	if err != nil {
		t.Fatalf("GenerateKeyPair(ed25519) failed: %v", err)
	}

	if len(privateKey) == 0 {
		t.Error("private key is empty")
	}
	if len(publicKey) == 0 {
		t.Error("public key is empty")
	}

	// Verify PEM format (PKCS8 for all key types)
	if !strings.Contains(string(privateKey), "-----BEGIN PRIVATE KEY-----") {
		t.Errorf("private key not in PEM format: %s", string(privateKey[:50]))
	}
}

func TestGenerateKeyPair_InvalidType(t *testing.T) {
	_, _, err := GenerateKeyPair("invalid", 0)
	if err == nil {
		t.Error("expected error for invalid key type")
	}
}

func TestSigner_SignAndVerify_RSA(t *testing.T) {
	testSignAndVerify(t, "rsa", 2048)
}

func TestSigner_SignAndVerify_ECDSA(t *testing.T) {
	testSignAndVerify(t, "ecdsa", 0)
}

func TestSigner_SignAndVerify_Ed25519(t *testing.T) {
	testSignAndVerify(t, "ed25519", 0)
}

func testSignAndVerify(t *testing.T, keyType string, bits int) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "signer-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test module file
	modulePath := filepath.Join(tmpDir, "test-module.zip")
	moduleContent := []byte("test module content for signing")
	if err := os.WriteFile(modulePath, moduleContent, 0644); err != nil {
		t.Fatalf("failed to write test module: %v", err)
	}

	// Generate key pair
	privateKey, publicKey, err := GenerateKeyPair(keyType, bits)
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	// Sign the module
	signer := NewSigner()
	signature, err := signer.Sign(modulePath, privateKey)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	if len(signature) == 0 {
		t.Error("signature is empty")
	}

	// Write signature to file
	sigPath := modulePath + ".sig"
	if err := os.WriteFile(sigPath, signature, 0644); err != nil {
		t.Fatalf("failed to write signature: %v", err)
	}

	// Verify the signature
	verifier := NewSignatureVerifier(SignatureFormatPKCS1)
	valid, err := verifier.VerifySignature(modulePath, sigPath, publicKey)
	if err != nil {
		t.Fatalf("VerifySignature failed: %v", err)
	}

	if !valid {
		t.Error("signature verification failed")
	}
}

func TestSigner_SignToFile(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "signer-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test module file
	modulePath := filepath.Join(tmpDir, "test-module.zip")
	moduleContent := []byte("test module content")
	if err := os.WriteFile(modulePath, moduleContent, 0644); err != nil {
		t.Fatalf("failed to write test module: %v", err)
	}

	// Generate key pair
	privateKey, _, err := GenerateKeyPair("ed25519", 0)
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	// Sign to file
	signer := NewSigner()
	sigPath := filepath.Join(tmpDir, "output", "test.sig")
	if err := signer.SignToFile(modulePath, sigPath, privateKey); err != nil {
		t.Fatalf("SignToFile failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(sigPath); err != nil {
		t.Errorf("signature file not created: %v", err)
	}
}

func TestSigner_InvalidModulePath(t *testing.T) {
	privateKey, _, err := GenerateKeyPair("ed25519", 0)
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	signer := NewSigner()
	_, err = signer.Sign("/nonexistent/module.zip", privateKey)
	if err == nil {
		t.Error("expected error for nonexistent module")
	}
}

func TestSigner_InvalidPrivateKey(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "signer-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test module file
	modulePath := filepath.Join(tmpDir, "test-module.zip")
	if err := os.WriteFile(modulePath, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to write test module: %v", err)
	}

	signer := NewSigner()
	_, err = signer.Sign(modulePath, []byte("invalid key"))
	if err == nil {
		t.Error("expected error for invalid private key")
	}
}

func TestSigner_TamperedModule(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "signer-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test module file
	modulePath := filepath.Join(tmpDir, "test-module.zip")
	if err := os.WriteFile(modulePath, []byte("original content"), 0644); err != nil {
		t.Fatalf("failed to write test module: %v", err)
	}

	// Generate key pair and sign
	privateKey, publicKey, err := GenerateKeyPair("ed25519", 0)
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	signer := NewSigner()
	signature, err := signer.Sign(modulePath, privateKey)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	// Write signature
	sigPath := modulePath + ".sig"
	if err := os.WriteFile(sigPath, signature, 0644); err != nil {
		t.Fatalf("failed to write signature: %v", err)
	}

	// Tamper with module
	if err := os.WriteFile(modulePath, []byte("tampered content"), 0644); err != nil {
		t.Fatalf("failed to tamper module: %v", err)
	}

	// Verification should fail
	verifier := NewSignatureVerifier(SignatureFormatPKCS1)
	valid, err := verifier.VerifySignature(modulePath, sigPath, publicKey)
	if err != nil {
		t.Fatalf("VerifySignature failed: %v", err)
	}

	if valid {
		t.Error("verification should fail for tampered module")
	}
}

func TestSigner_WrongKey(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "signer-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test module file
	modulePath := filepath.Join(tmpDir, "test-module.zip")
	if err := os.WriteFile(modulePath, []byte("test content"), 0644); err != nil {
		t.Fatalf("failed to write test module: %v", err)
	}

	// Generate two different key pairs
	privateKey1, _, err := GenerateKeyPair("ed25519", 0)
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}
	_, publicKey2, err := GenerateKeyPair("ed25519", 0)
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	// Sign with key1
	signer := NewSigner()
	signature, err := signer.Sign(modulePath, privateKey1)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	// Write signature
	sigPath := modulePath + ".sig"
	if err := os.WriteFile(sigPath, signature, 0644); err != nil {
		t.Fatalf("failed to write signature: %v", err)
	}

	// Verify with key2 (should fail)
	verifier := NewSignatureVerifier(SignatureFormatPKCS1)
	valid, err := verifier.VerifySignature(modulePath, sigPath, publicKey2)
	if err != nil {
		t.Fatalf("VerifySignature failed: %v", err)
	}

	if valid {
		t.Error("verification should fail with wrong public key")
	}
}
