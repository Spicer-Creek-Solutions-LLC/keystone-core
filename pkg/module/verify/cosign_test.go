package verify

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestNewCosignVerifier(t *testing.T) {
	v := NewCosignVerifier()
	if v == nil {
		t.Fatal("NewCosignVerifier returned nil")
	}
	if v.RekorURL != "https://rekor.sigstore.dev" {
		t.Errorf("expected default Rekor URL, got %s", v.RekorURL)
	}
	if !v.SkipRekor {
		t.Error("expected SkipRekor to be true by default")
	}
}

func TestNewCosignVerifier_WithOptions(t *testing.T) {
	v := NewCosignVerifier(
		WithRekorURL("https://custom.rekor.example.com"),
		WithSkipRekor(),
	)
	if v.RekorURL != "https://custom.rekor.example.com" {
		t.Errorf("expected custom Rekor URL, got %s", v.RekorURL)
	}
}

func TestCosignVerifier_VerifySignature_ECDSA(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "cosign-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test module file
	modulePath := filepath.Join(tmpDir, "test-module.zip")
	moduleContent := []byte("test module content for cosign verification")
	if err := os.WriteFile(modulePath, moduleContent, 0644); err != nil {
		t.Fatalf("failed to write test module: %v", err)
	}

	// Generate ECDSA P-256 key pair (cosign default)
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	// Create signature (SHA256 hash, ASN.1 DER format)
	hash := sha256.Sum256(moduleContent)
	signature, err := ecdsa.SignASN1(rand.Reader, privateKey, hash[:])
	if err != nil {
		t.Fatalf("failed to sign: %v", err)
	}

	// Encode signature as base64 (cosign format)
	sigB64 := base64.StdEncoding.EncodeToString(signature)

	// Write signature file
	sigPath := modulePath + ".sig"
	if err := os.WriteFile(sigPath, []byte(sigB64), 0644); err != nil {
		t.Fatalf("failed to write signature: %v", err)
	}

	// Marshal public key to PEM
	pubKeyBytes, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatalf("failed to marshal public key: %v", err)
	}
	pubKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubKeyBytes,
	})

	// Verify
	verifier := NewCosignVerifier()
	valid, err := verifier.VerifySignature(modulePath, sigPath, pubKeyPEM)
	if err != nil {
		t.Fatalf("VerifySignature failed: %v", err)
	}
	if !valid {
		t.Error("signature verification failed")
	}
}

func TestCosignVerifier_VerifySignature_RawRS(t *testing.T) {
	// Test raw R||S format (64 bytes for P-256)
	tmpDir, err := os.MkdirTemp("", "cosign-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	modulePath := filepath.Join(tmpDir, "test-module.zip")
	moduleContent := []byte("test content for raw RS signature")
	if err := os.WriteFile(modulePath, moduleContent, 0644); err != nil {
		t.Fatalf("failed to write test module: %v", err)
	}

	// Generate key
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	// Sign and create raw R||S format
	hash := sha256.Sum256(moduleContent)
	r, s, err := ecdsa.Sign(rand.Reader, privateKey, hash[:])
	if err != nil {
		t.Fatalf("failed to sign: %v", err)
	}

	// Pad to 32 bytes each
	rBytes := r.Bytes()
	sBytes := s.Bytes()
	rawSig := make([]byte, 64)
	copy(rawSig[32-len(rBytes):32], rBytes)
	copy(rawSig[64-len(sBytes):64], sBytes)

	sigB64 := base64.StdEncoding.EncodeToString(rawSig)
	sigPath := modulePath + ".sig"
	if err := os.WriteFile(sigPath, []byte(sigB64), 0644); err != nil {
		t.Fatalf("failed to write signature: %v", err)
	}

	pubKeyBytes, _ := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	pubKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubKeyBytes})

	verifier := NewCosignVerifier()
	valid, err := verifier.VerifySignature(modulePath, sigPath, pubKeyPEM)
	if err != nil {
		t.Fatalf("VerifySignature failed: %v", err)
	}
	if !valid {
		t.Error("raw R||S signature verification failed")
	}
}

func TestCosignVerifier_VerifySignature_TamperedModule(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "cosign-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	modulePath := filepath.Join(tmpDir, "test-module.zip")
	originalContent := []byte("original content")
	if err := os.WriteFile(modulePath, originalContent, 0644); err != nil {
		t.Fatalf("failed to write test module: %v", err)
	}

	// Generate key and sign
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	hash := sha256.Sum256(originalContent)
	signature, _ := ecdsa.SignASN1(rand.Reader, privateKey, hash[:])
	sigB64 := base64.StdEncoding.EncodeToString(signature)

	sigPath := modulePath + ".sig"
	if err := os.WriteFile(sigPath, []byte(sigB64), 0644); err != nil {
		t.Fatalf("failed to write signature: %v", err)
	}

	// Tamper with module
	if err := os.WriteFile(modulePath, []byte("tampered content"), 0644); err != nil {
		t.Fatalf("failed to tamper module: %v", err)
	}

	pubKeyBytes, _ := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	pubKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubKeyBytes})

	verifier := NewCosignVerifier()
	valid, err := verifier.VerifySignature(modulePath, sigPath, pubKeyPEM)
	if err != nil {
		t.Fatalf("VerifySignature failed: %v", err)
	}
	if valid {
		t.Error("verification should fail for tampered module")
	}
}

func TestCosignVerifier_VerifySignature_WrongKey(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "cosign-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	modulePath := filepath.Join(tmpDir, "test-module.zip")
	moduleContent := []byte("test content")
	if err := os.WriteFile(modulePath, moduleContent, 0644); err != nil {
		t.Fatalf("failed to write test module: %v", err)
	}

	// Generate two different keys
	privateKey1, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	privateKey2, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)

	// Sign with key1
	hash := sha256.Sum256(moduleContent)
	signature, _ := ecdsa.SignASN1(rand.Reader, privateKey1, hash[:])
	sigB64 := base64.StdEncoding.EncodeToString(signature)

	sigPath := modulePath + ".sig"
	if err := os.WriteFile(sigPath, []byte(sigB64), 0644); err != nil {
		t.Fatalf("failed to write signature: %v", err)
	}

	// Verify with key2 (should fail)
	pubKeyBytes2, _ := x509.MarshalPKIXPublicKey(&privateKey2.PublicKey)
	pubKeyPEM2 := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubKeyBytes2})

	verifier := NewCosignVerifier()
	valid, err := verifier.VerifySignature(modulePath, sigPath, pubKeyPEM2)
	if err != nil {
		t.Fatalf("VerifySignature failed: %v", err)
	}
	if valid {
		t.Error("verification should fail with wrong public key")
	}
}

func TestCosignVerifier_VerifySignature_InvalidSignature(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "cosign-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	modulePath := filepath.Join(tmpDir, "test-module.zip")
	if err := os.WriteFile(modulePath, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to write test module: %v", err)
	}

	// Write invalid base64 signature
	sigPath := modulePath + ".sig"
	if err := os.WriteFile(sigPath, []byte("not-valid-base64!!!"), 0644); err != nil {
		t.Fatalf("failed to write signature: %v", err)
	}

	privateKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	pubKeyBytes, _ := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	pubKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubKeyBytes})

	verifier := NewCosignVerifier()
	_, err = verifier.VerifySignature(modulePath, sigPath, pubKeyPEM)
	if err == nil {
		t.Error("expected error for invalid base64 signature")
	}
}

func TestCosignVerifier_VerifySignature_InvalidPublicKey(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "cosign-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	modulePath := filepath.Join(tmpDir, "test-module.zip")
	if err := os.WriteFile(modulePath, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to write test module: %v", err)
	}

	sigPath := modulePath + ".sig"
	if err := os.WriteFile(sigPath, []byte(base64.StdEncoding.EncodeToString([]byte("sig"))), 0644); err != nil {
		t.Fatalf("failed to write signature: %v", err)
	}

	verifier := NewCosignVerifier()
	_, err = verifier.VerifySignature(modulePath, sigPath, []byte("invalid key"))
	if err == nil {
		t.Error("expected error for invalid public key")
	}
}

func TestCosignVerifier_VerifySignature_MissingSignature(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "cosign-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	modulePath := filepath.Join(tmpDir, "test-module.zip")
	if err := os.WriteFile(modulePath, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to write test module: %v", err)
	}

	privateKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	pubKeyBytes, _ := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	pubKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubKeyBytes})

	verifier := NewCosignVerifier()
	_, err = verifier.VerifySignature(modulePath, "/nonexistent/signature.sig", pubKeyPEM)
	if !errors.Is(err, ErrSignatureNotFound) {
		t.Errorf("expected ErrSignatureNotFound, got %v", err)
	}
}

func TestCosignVerifier_VerifySignature_MissingModule(t *testing.T) {
	privateKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	pubKeyBytes, _ := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	pubKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubKeyBytes})

	verifier := NewCosignVerifier()
	_, err := verifier.VerifySignature("/nonexistent/module.zip", "/some/sig", pubKeyPEM)
	if err == nil {
		t.Error("expected error for missing module")
	}
}

func TestCosignVerifier_GetSignerIdentity_RawSignature(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "cosign-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	sigPath := filepath.Join(tmpDir, "test.sig")
	// Write a raw base64 signature (not a bundle)
	if err := os.WriteFile(sigPath, []byte("MEUCIQDX2BAbcd..."), 0644); err != nil {
		t.Fatalf("failed to write signature: %v", err)
	}

	verifier := NewCosignVerifier()
	identity, err := verifier.GetSignerIdentity(sigPath)
	if err != nil {
		t.Fatalf("GetSignerIdentity failed: %v", err)
	}
	if identity != "key-based-signature" {
		t.Errorf("expected 'key-based-signature', got %s", identity)
	}
}

func TestCosignVerifier_GetSignerIdentity_MissingFile(t *testing.T) {
	verifier := NewCosignVerifier()
	_, err := verifier.GetSignerIdentity("/nonexistent/signature.sig")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestDecodeCosignSignature(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantLen int
		wantErr bool
	}{
		{
			name:    "standard base64",
			input:   base64.StdEncoding.EncodeToString([]byte("test signature")),
			wantLen: 14,
			wantErr: false,
		},
		{
			name:    "URL-safe base64",
			input:   base64.URLEncoding.EncodeToString([]byte("test signature")),
			wantLen: 14,
			wantErr: false,
		},
		{
			name:    "raw base64 (no padding)",
			input:   base64.RawStdEncoding.EncodeToString([]byte("test")),
			wantLen: 4,
			wantErr: false,
		},
		{
			name:    "with whitespace",
			input:   "  " + base64.StdEncoding.EncodeToString([]byte("test")) + "  \n",
			wantLen: 4,
			wantErr: false,
		},
		{
			name:    "invalid base64",
			input:   "not-valid-base64!!!",
			wantLen: 0,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := decodeCosignSignature([]byte(tt.input))
			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(result) != tt.wantLen {
				t.Errorf("expected length %d, got %d", tt.wantLen, len(result))
			}
		})
	}
}

func TestParseCosignPublicKey(t *testing.T) {
	// Generate an ECDSA key
	privateKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	pubKeyBytes, _ := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)

	tests := []struct {
		name    string
		input   []byte
		wantErr bool
	}{
		{
			name: "valid PKIX public key",
			input: pem.EncodeToMemory(&pem.Block{
				Type:  "PUBLIC KEY",
				Bytes: pubKeyBytes,
			}),
			wantErr: false,
		},
		{
			name:    "invalid PEM",
			input:   []byte("not a PEM block"),
			wantErr: true,
		},
		{
			name: "invalid key data",
			input: pem.EncodeToMemory(&pem.Block{
				Type:  "PUBLIC KEY",
				Bytes: []byte("invalid"),
			}),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseCosignPublicKey(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestVerifyCosignECDSA(t *testing.T) {
	privateKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	message := []byte("test message")
	hash := sha256.Sum256(message)

	// Sign
	signature, _ := ecdsa.SignASN1(rand.Reader, privateKey, hash[:])

	tests := []struct {
		name      string
		hash      []byte
		signature []byte
		wantValid bool
	}{
		{
			name:      "valid ASN.1 signature",
			hash:      hash[:],
			signature: signature,
			wantValid: true,
		},
		{
			name:      "wrong hash",
			hash:      sha256.New().Sum([]byte("wrong")),
			signature: signature,
			wantValid: false,
		},
		{
			name:      "invalid signature",
			hash:      hash[:],
			signature: []byte("invalid"),
			wantValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, err := verifyCosignECDSA(&privateKey.PublicKey, tt.hash, tt.signature)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if valid != tt.wantValid {
				t.Errorf("expected valid=%v, got %v", tt.wantValid, valid)
			}
		})
	}
}

func TestExtractIdentityFromBundle(t *testing.T) {
	tests := []struct {
		name       string
		bundleJSON string
		wantErr    bool
	}{
		{
			name:       "invalid JSON",
			bundleJSON: "not json",
			wantErr:    true,
		},
		{
			name:       "missing cert",
			bundleJSON: `{"base64Signature": "abc"}`,
			wantErr:    true,
		},
		{
			name:       "invalid cert encoding",
			bundleJSON: `{"base64Signature": "abc", "cert": "not-base64!!!"}`,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := extractIdentityFromBundle([]byte(tt.bundleJSON))
			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestCosignBundle_JSON(t *testing.T) {
	bundle := CosignBundle{
		Base64Signature: "test-signature",
		Cert:            "test-cert",
	}

	data, err := json.Marshal(bundle)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var parsed CosignBundle
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if parsed.Base64Signature != bundle.Base64Signature {
		t.Error("signature mismatch")
	}
	if parsed.Cert != bundle.Cert {
		t.Error("cert mismatch")
	}
}
