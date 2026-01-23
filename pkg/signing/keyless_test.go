package signing

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestNewKeylessSigner(t *testing.T) {
	tests := []struct {
		name    string
		config  *KeylessSignerConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			config: &KeylessSignerConfig{
				OIDCToken: "test-token",
			},
		},
		{
			name: "with custom URLs",
			config: &KeylessSignerConfig{
				OIDCToken: "test-token",
				FulcioURL: "https://custom.fulcio.example.com",
				RekorURL:  "https://custom.rekor.example.com",
			},
		},
		{
			name: "with custom timeout",
			config: &KeylessSignerConfig{
				OIDCToken: "test-token",
				Timeout:   60 * time.Second,
			},
		},
		{
			name: "with annotations",
			config: &KeylessSignerConfig{
				OIDCToken: "test-token",
				Annotations: map[string]string{
					"build_id": "123",
				},
			},
		},
		{
			name: "with custom HTTP client",
			config: &KeylessSignerConfig{
				OIDCToken:  "test-token",
				HTTPClient: &http.Client{Timeout: 5 * time.Second},
			},
		},
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
			errMsg:  "config is required",
		},
		{
			name:    "missing OIDC token",
			config:  &KeylessSignerConfig{},
			wantErr: true,
			errMsg:  "OIDC token is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			signer, err := NewKeylessSigner(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewKeylessSigner() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				if tt.errMsg != "" && err != nil {
					if !contains(err.Error(), tt.errMsg) {
						t.Errorf("error message = %v, want to contain %v", err.Error(), tt.errMsg)
					}
				}
				return
			}
			if signer == nil {
				t.Error("expected non-nil signer")
			}
		})
	}
}

func TestKeylessSignerDefaults(t *testing.T) {
	config := &KeylessSignerConfig{
		OIDCToken: "test-token",
	}

	signer, err := NewKeylessSigner(config)
	if err != nil {
		t.Fatalf("NewKeylessSigner() error = %v", err)
	}

	// Check defaults were applied
	if config.FulcioURL != "https://fulcio.sigstore.dev" {
		t.Errorf("FulcioURL = %v, want default", config.FulcioURL)
	}
	if config.RekorURL != "https://rekor.sigstore.dev" {
		t.Errorf("RekorURL = %v, want default", config.RekorURL)
	}
	if config.Timeout != 30*time.Second {
		t.Errorf("Timeout = %v, want default", config.Timeout)
	}
	if signer.httpClient == nil {
		t.Error("expected non-nil HTTP client")
	}
}

func TestKeylessSignerKeyType(t *testing.T) {
	signer, _ := NewKeylessSigner(&KeylessSignerConfig{
		OIDCToken: "test-token",
	})

	if signer.KeyType() != KeyTypeECDSA {
		t.Errorf("KeyType() = %v, want %v", signer.KeyType(), KeyTypeECDSA)
	}
}

func TestKeylessSignerPublicKey(t *testing.T) {
	signer, _ := NewKeylessSigner(&KeylessSignerConfig{
		OIDCToken: "test-token",
	})

	_, err := signer.PublicKey()
	if err == nil {
		t.Error("expected error for keyless signer PublicKey()")
	}
}

// Helper to create a mock certificate chain for testing
func createMockCertChain() []byte {
	// Generate a test key pair
	privateKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)

	// Create a self-signed certificate
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "test@example.com",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageCodeSigning},
		BasicConstraintsValid: true,
		EmailAddresses:        []string{"test@example.com"},
	}

	certDER, _ := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)

	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	return certPEM
}

func TestKeylessSignerWithMockFulcio(t *testing.T) {
	ctx := context.Background()

	// Create mock Fulcio server
	certChain := createMockCertChain()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/signingCert" {
			// Check authorization header
			if r.Header.Get("Authorization") == "" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			w.Header().Set("Content-Type", "application/pem-certificate-chain")
			w.WriteHeader(http.StatusCreated)
			w.Write(certChain)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	signer, err := NewKeylessSigner(&KeylessSignerConfig{
		OIDCToken: "test-oidc-token",
		FulcioURL: server.URL,
	})
	if err != nil {
		t.Fatalf("NewKeylessSigner() error = %v", err)
	}

	testData := []byte("test data for keyless signing")

	result, err := signer.Sign(ctx, testData)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}

	// Verify result fields
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Signature) == 0 {
		t.Error("expected non-empty signature")
	}
	if result.SignatureBase64 == "" {
		t.Error("expected non-empty base64 signature")
	}
	if result.Certificate == nil {
		t.Error("expected non-nil certificate")
	}
	if result.SignerIdentity == "" {
		t.Error("expected non-empty signer identity")
	}
	if result.Bundle == nil {
		t.Error("expected non-nil bundle")
	}
	if result.Timestamp.IsZero() {
		t.Error("expected non-zero timestamp")
	}
}

func TestKeylessSignerFulcioError(t *testing.T) {
	ctx := context.Background()

	// Create mock Fulcio server that returns errors
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal server error"))
	}))
	defer server.Close()

	signer, _ := NewKeylessSigner(&KeylessSignerConfig{
		OIDCToken: "test-token",
		FulcioURL: server.URL,
	})

	_, err := signer.Sign(ctx, []byte("test data"))
	if err == nil {
		t.Error("expected error when Fulcio returns error")
	}
}

func TestKeylessSignerSignFile(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	// Create mock Fulcio server
	certChain := createMockCertChain()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pem-certificate-chain")
		w.WriteHeader(http.StatusCreated)
		w.Write(certChain)
	}))
	defer server.Close()

	signer, _ := NewKeylessSigner(&KeylessSignerConfig{
		OIDCToken: "test-token",
		FulcioURL: server.URL,
	})

	// Create test file
	testFile := tmpDir + "/test.txt"
	if err := os.WriteFile(testFile, []byte("file content"), 0644); err != nil {
		t.Fatal(err)
	}

	t.Run("sign file", func(t *testing.T) {
		result, err := signer.SignFile(ctx, testFile)
		if err != nil {
			t.Errorf("SignFile() error = %v", err)
			return
		}
		if result == nil {
			t.Error("expected non-nil result")
		}
	})

	t.Run("file not found", func(t *testing.T) {
		_, err := signer.SignFile(ctx, tmpDir+"/nonexistent.txt")
		if err == nil {
			t.Error("expected error for nonexistent file")
		}
	})
}

func TestNewKeylessVerifier(t *testing.T) {
	verifier := NewKeylessVerifier()
	if verifier == nil {
		t.Error("expected non-nil verifier")
	}
}

func TestKeylessVerifierOptions(t *testing.T) {
	verifier := NewKeylessVerifier().
		WithTrustedRoots(x509.NewCertPool()).
		WithExpectedIssuer("https://accounts.google.com").
		WithExpectedIdentity("test@example.com")

	if verifier.TrustedRoots == nil {
		t.Error("expected non-nil trusted roots")
	}
	if verifier.ExpectedIssuer != "https://accounts.google.com" {
		t.Error("expected issuer not set")
	}
	if verifier.ExpectedIdentity != "test@example.com" {
		t.Error("expected identity not set")
	}
}

func TestKeylessVerifierVerify(t *testing.T) {
	ctx := context.Background()
	testData := []byte("test data for verification")

	// Create a signed result with mock certificate
	certPEM := createMockCertChain()

	// Parse the cert to get the public key for signing
	block, _ := pem.Decode(certPEM)
	cert, _ := x509.ParseCertificate(block.Bytes)

	// We need to create a signature with the corresponding private key
	// For testing, we'll create our own key pair
	privateKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)

	// Create a certificate with the private key's public key
	template := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject: pkix.Name{
			CommonName: "verifier-test@example.com",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		EmailAddresses:        []string{"verifier-test@example.com"},
	}

	certDER, _ := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	testCertPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	// Sign the data
	signer, _ := NewKeySigner(&KeySignerConfig{
		PrivateKeyPEM: func() []byte {
			privBytes, _ := x509.MarshalPKCS8PrivateKey(privateKey)
			return pem.EncodeToMemory(&pem.Block{
				Type:  "PRIVATE KEY",
				Bytes: privBytes,
			})
		}(),
	})

	signResult, _ := signer.Sign(ctx, testData)

	t.Run("verify with certificate", func(t *testing.T) {
		verifier := NewKeylessVerifier()

		result, err := verifier.Verify(ctx, testData, signResult.Signature, testCertPEM)
		if err != nil {
			t.Errorf("Verify() error = %v", err)
			return
		}
		if !result.Valid {
			t.Error("signature should be valid")
		}
		if result.SignerIdentity != "verifier-test@example.com" {
			t.Errorf("signer identity = %v, want verifier-test@example.com", result.SignerIdentity)
		}
	})

	t.Run("identity mismatch warning", func(t *testing.T) {
		verifier := NewKeylessVerifier().
			WithExpectedIdentity("different@example.com")

		result, err := verifier.Verify(ctx, testData, signResult.Signature, testCertPEM)
		if err != nil {
			t.Errorf("Verify() error = %v", err)
			return
		}
		if !result.Valid {
			t.Error("signature should still be valid")
		}
		if len(result.Warnings) == 0 {
			t.Error("expected warning for identity mismatch")
		}
	})

	t.Run("invalid certificate", func(t *testing.T) {
		verifier := NewKeylessVerifier()

		_, err := verifier.Verify(ctx, testData, signResult.Signature, []byte("not a certificate"))
		if err == nil {
			t.Error("expected error for invalid certificate")
		}
	})

	// Need to use the real cert for this test
	_ = cert
}

func TestKeylessVerifierVerifyBundle(t *testing.T) {
	ctx := context.Background()
	testData := []byte("bundle test data")

	// Create a key pair and certificate for testing
	privateKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)

	privPEM := func() []byte {
		privBytes, _ := x509.MarshalPKCS8PrivateKey(privateKey)
		return pem.EncodeToMemory(&pem.Block{
			Type:  "PRIVATE KEY",
			Bytes: privBytes,
		})
	}()

	// Create a mock Fulcio server that issues certificates for the public key in the request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Parse the request to extract the public key
		var reqBody map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// Extract the public key from the request
		pubKeyReq, ok := reqBody["publicKeyRequest"].(map[string]interface{})
		if !ok {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		pubKey, ok := pubKeyReq["publicKey"].(map[string]interface{})
		if !ok {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		contentB64, ok := pubKey["content"].(string)
		if !ok {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		pubKeyBytes, err := base64.StdEncoding.DecodeString(contentB64)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		pubKeyParsed, err := x509.ParsePKIXPublicKey(pubKeyBytes)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// Issue a certificate for the public key
		template := &x509.Certificate{
			SerialNumber: big.NewInt(time.Now().UnixNano()),
			Subject: pkix.Name{
				CommonName: "bundle-test@example.com",
			},
			NotBefore:             time.Now(),
			NotAfter:              time.Now().Add(24 * time.Hour),
			KeyUsage:              x509.KeyUsageDigitalSignature,
			BasicConstraintsValid: true,
			EmailAddresses:        []string{"bundle-test@example.com"},
		}

		// Create a CA key for signing (in practice Fulcio has its own CA)
		caKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		certDER, err := x509.CreateCertificate(rand.Reader, template, template, pubKeyParsed, caKey)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		certPEM := pem.EncodeToMemory(&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: certDER,
		})

		w.Header().Set("Content-Type", "application/pem-certificate-chain")
		w.WriteHeader(http.StatusCreated)
		w.Write(certPEM)
	}))
	defer server.Close()

	// Use the keyless signer with mock Fulcio
	keylessSigner, _ := NewKeylessSigner(&KeylessSignerConfig{
		OIDCToken: "test-token",
		FulcioURL: server.URL,
	})

	keylessResult, err := keylessSigner.Sign(ctx, testData)
	if err != nil {
		t.Fatalf("keyless Sign() error = %v", err)
	}

	t.Run("verify bundle", func(t *testing.T) {
		verifier := NewKeylessVerifier()

		result, err := verifier.VerifyBundle(ctx, testData, keylessResult.Bundle)
		if err != nil {
			t.Errorf("VerifyBundle() error = %v", err)
			return
		}
		if !result.Valid {
			t.Error("signature should be valid")
		}
	})

	t.Run("empty bundle", func(t *testing.T) {
		verifier := NewKeylessVerifier()

		_, err := verifier.VerifyBundle(ctx, testData, &SignatureBundle{})
		if err == nil {
			t.Error("expected error for empty bundle")
		}
	})

	t.Run("bundle without certificate", func(t *testing.T) {
		verifier := NewKeylessVerifier()

		// Create a key-based signature (no certificate)
		keySigner, _ := NewKeySigner(&KeySignerConfig{
			PrivateKeyPEM: privPEM,
			Format:        FormatBundle,
		})
		keyResult, _ := keySigner.Sign(ctx, testData)

		_, err := verifier.VerifyBundle(ctx, testData, keyResult.Bundle)
		if err == nil {
			t.Error("expected error for bundle without certificate")
		}
	})
}

func TestGetSigner(t *testing.T) {
	keyPair, _ := GenerateKeyPair(KeyTypeECDSA, 256)

	t.Run("KeySignerConfig", func(t *testing.T) {
		signer, err := GetSigner(&KeySignerConfig{
			PrivateKeyPEM: keyPair.PrivateKey,
		})
		if err != nil {
			t.Errorf("GetSigner() error = %v", err)
			return
		}
		if _, ok := signer.(*KeySigner); !ok {
			t.Error("expected *KeySigner")
		}
	})

	t.Run("KeylessSignerConfig", func(t *testing.T) {
		signer, err := GetSigner(&KeylessSignerConfig{
			OIDCToken: "test-token",
		})
		if err != nil {
			t.Errorf("GetSigner() error = %v", err)
			return
		}
		if _, ok := signer.(*KeylessSigner); !ok {
			t.Error("expected *KeylessSigner")
		}
	})

	t.Run("unsupported config type", func(t *testing.T) {
		_, err := GetSigner("invalid")
		if err == nil {
			t.Error("expected error for unsupported config type")
		}
	})
}

func TestExtractIdentityFromCert(t *testing.T) {
	tests := []struct {
		name     string
		setupFn  func() *x509.Certificate
		expected string
	}{
		{
			name: "email in SAN",
			setupFn: func() *x509.Certificate {
				return &x509.Certificate{
					EmailAddresses: []string{"user@example.com"},
				}
			},
			expected: "user@example.com",
		},
		{
			name: "multiple emails returns first",
			setupFn: func() *x509.Certificate {
				return &x509.Certificate{
					EmailAddresses: []string{"first@example.com", "second@example.com"},
				}
			},
			expected: "first@example.com",
		},
		{
			name: "common name fallback",
			setupFn: func() *x509.Certificate {
				return &x509.Certificate{
					Subject: pkix.Name{
						CommonName: "Test User",
					},
				}
			},
			expected: "Test User",
		},
		{
			name: "unknown identity",
			setupFn: func() *x509.Certificate {
				return &x509.Certificate{}
			},
			expected: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cert := tt.setupFn()
			identity := extractIdentityFromCert(cert)
			if identity != tt.expected {
				t.Errorf("extractIdentityFromCert() = %v, want %v", identity, tt.expected)
			}
		})
	}
}

func TestParseCertificateChain(t *testing.T) {
	// Create a chain of certificates
	rootKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	leafKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)

	rootTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Root CA"},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}

	rootDER, _ := x509.CreateCertificate(rand.Reader, rootTemplate, rootTemplate, &rootKey.PublicKey, rootKey)

	leafTemplate := &x509.Certificate{
		SerialNumber:   big.NewInt(2),
		Subject:        pkix.Name{CommonName: "Leaf"},
		NotBefore:      time.Now(),
		NotAfter:       time.Now().Add(24 * time.Hour),
		EmailAddresses: []string{"leaf@example.com"},
		KeyUsage:       x509.KeyUsageDigitalSignature,
	}

	rootCert, _ := x509.ParseCertificate(rootDER)
	leafDER, _ := x509.CreateCertificate(rand.Reader, leafTemplate, rootCert, &leafKey.PublicKey, rootKey)

	// Create PEM chain
	chainPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER})
	chainPEM = append(chainPEM, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: rootDER})...)

	signer := &KeylessSigner{}

	t.Run("valid chain", func(t *testing.T) {
		signingCert, chain, identity, err := signer.parseCertificateChain(chainPEM)
		if err != nil {
			t.Errorf("parseCertificateChain() error = %v", err)
			return
		}
		if signingCert == nil {
			t.Error("expected non-nil signing cert")
		}
		if len(chain) != 1 {
			t.Errorf("chain length = %d, want 1", len(chain))
		}
		if identity != "leaf@example.com" {
			t.Errorf("identity = %v, want leaf@example.com", identity)
		}
	})

	t.Run("empty response", func(t *testing.T) {
		_, _, _, err := signer.parseCertificateChain([]byte{})
		if err == nil {
			t.Error("expected error for empty response")
		}
	})
}

// Helper functions

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
