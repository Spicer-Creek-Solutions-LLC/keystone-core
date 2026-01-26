package registry

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestNewVerifier(t *testing.T) {
	// Generate a key pair for testing
	_, pub, err := GenerateKeyPair(KeyTypeECDSA, 256)
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "public.key")
	if err := os.WriteFile(keyPath, pub, 0644); err != nil {
		t.Fatalf("failed to write key file: %v", err)
	}

	t.Run("success with path", func(t *testing.T) {
		verifier, err := NewVerifier(&VerificationConfig{
			PublicKeyPath: keyPath,
		})
		if err != nil {
			t.Fatalf("NewVerifier failed: %v", err)
		}
		if verifier.KeyType() != KeyTypeECDSA {
			t.Errorf("key type = %s, want %s", verifier.KeyType(), KeyTypeECDSA)
		}
	})

	t.Run("success with data", func(t *testing.T) {
		verifier, err := NewVerifier(&VerificationConfig{
			PublicKeyData: pub,
		})
		if err != nil {
			t.Fatalf("NewVerifier failed: %v", err)
		}
		if verifier.KeyType() != KeyTypeECDSA {
			t.Errorf("key type = %s, want %s", verifier.KeyType(), KeyTypeECDSA)
		}
	})

	t.Run("nil config", func(t *testing.T) {
		_, err := NewVerifier(nil)
		if err == nil {
			t.Error("expected error for nil config")
		}
	})

	t.Run("missing key", func(t *testing.T) {
		_, err := NewVerifier(&VerificationConfig{})
		if err == nil {
			t.Error("expected error for missing key")
		}
	})

	t.Run("nonexistent key file", func(t *testing.T) {
		_, err := NewVerifier(&VerificationConfig{
			PublicKeyPath: "/nonexistent/key.pem",
		})
		if err == nil {
			t.Error("expected error for nonexistent key file")
		}
	})

	t.Run("invalid key data", func(t *testing.T) {
		_, err := NewVerifier(&VerificationConfig{
			PublicKeyData: []byte("not a key"),
		})
		if err == nil {
			t.Error("expected error for invalid key data")
		}
	})
}

func TestVerifier_Verify(t *testing.T) {
	keyTypes := []KeyType{KeyTypeECDSA, KeyTypeRSA, KeyTypeEd25519}

	for _, kt := range keyTypes {
		t.Run(string(kt), func(t *testing.T) {
			// Generate key pair
			priv, pub, err := GenerateKeyPair(kt, 2048)
			if err != nil {
				t.Fatalf("GenerateKeyPair failed: %v", err)
			}

			tmpDir := t.TempDir()
			privPath := filepath.Join(tmpDir, "private.key")
			pubPath := filepath.Join(tmpDir, "public.key")

			if err := os.WriteFile(privPath, priv, 0600); err != nil {
				t.Fatalf("failed to write private key: %v", err)
			}
			if err := os.WriteFile(pubPath, pub, 0644); err != nil {
				t.Fatalf("failed to write public key: %v", err)
			}

			// Create signer
			signer, err := NewSigner(&SigningConfig{
				KeyPath: privPath,
			})
			if err != nil {
				t.Fatalf("NewSigner failed: %v", err)
			}

			// Sign data
			data := []byte("test data to sign and verify")
			signResult, err := signer.Sign(context.Background(), data)
			if err != nil {
				t.Fatalf("Sign failed: %v", err)
			}

			// Create verifier
			verifier, err := NewVerifier(&VerificationConfig{
				PublicKeyPath: pubPath,
			})
			if err != nil {
				t.Fatalf("NewVerifier failed: %v", err)
			}

			// Verify
			result, err := verifier.Verify(context.Background(), data, signResult.Signature)
			if err != nil {
				t.Fatalf("Verify failed: %v", err)
			}

			if !result.Valid {
				t.Errorf("verification failed: %v", result.Errors)
			}
			if result.Digest == "" {
				t.Error("digest is empty")
			}
		})
	}
}

func TestVerifier_Verify_InvalidSignature(t *testing.T) {
	_, pub, err := GenerateKeyPair(KeyTypeECDSA, 256)
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	verifier, err := NewVerifier(&VerificationConfig{
		PublicKeyData: pub,
	})
	if err != nil {
		t.Fatalf("NewVerifier failed: %v", err)
	}

	data := []byte("test data")

	t.Run("malformed base64", func(t *testing.T) {
		result, err := verifier.Verify(context.Background(), data, "not-base64!!!")
		if err != nil {
			t.Fatalf("Verify failed: %v", err)
		}
		if result.Valid {
			t.Error("expected invalid result")
		}
		if len(result.Errors) == 0 {
			t.Error("expected errors")
		}
	})

	t.Run("wrong signature", func(t *testing.T) {
		// Valid base64 but wrong signature
		result, err := verifier.Verify(context.Background(), data, "dGVzdA==")
		if err != nil {
			t.Fatalf("Verify failed: %v", err)
		}
		if result.Valid {
			t.Error("expected invalid result")
		}
	})
}

func TestVerifier_VerifyBundle(t *testing.T) {
	// Generate key pair
	priv, pub, err := GenerateKeyPair(KeyTypeECDSA, 256)
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	tmpDir := t.TempDir()
	privPath := filepath.Join(tmpDir, "private.key")

	if err := os.WriteFile(privPath, priv, 0600); err != nil {
		t.Fatalf("failed to write private key: %v", err)
	}

	// Create signer with bundle format
	signer, err := NewSigner(&SigningConfig{
		KeyPath: privPath,
		Format:  SignatureFormatBundle,
		Annotations: map[string]string{
			"vendor": "test-vendor",
		},
	})
	if err != nil {
		t.Fatalf("NewSigner failed: %v", err)
	}

	data := []byte("test data to sign and verify")
	signResult, err := signer.Sign(context.Background(), data)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	// Create verifier
	verifier, err := NewVerifier(&VerificationConfig{
		PublicKeyData: pub,
	})
	if err != nil {
		t.Fatalf("NewVerifier failed: %v", err)
	}

	t.Run("valid bundle", func(t *testing.T) {
		result, err := verifier.VerifyBundle(context.Background(), data, signResult.Bundle)
		if err != nil {
			t.Fatalf("VerifyBundle failed: %v", err)
		}
		if !result.Valid {
			t.Errorf("verification failed: %v", result.Errors)
		}
		if result.Annotations["vendor"] != "test-vendor" {
			t.Error("annotations not preserved")
		}
	})

	t.Run("nil bundle", func(t *testing.T) {
		result, err := verifier.VerifyBundle(context.Background(), data, nil)
		if err != nil {
			t.Fatalf("VerifyBundle failed: %v", err)
		}
		if result.Valid {
			t.Error("expected invalid result for nil bundle")
		}
	})

	t.Run("empty signatures", func(t *testing.T) {
		bundle := &SignatureBundle{
			Payload:    signResult.Bundle.Payload,
			Signatures: []BundleSignature{},
		}
		result, err := verifier.VerifyBundle(context.Background(), data, bundle)
		if err != nil {
			t.Fatalf("VerifyBundle failed: %v", err)
		}
		if result.Valid {
			t.Error("expected invalid result for empty signatures")
		}
	})

	t.Run("wrong data", func(t *testing.T) {
		result, err := verifier.VerifyBundle(context.Background(), []byte("wrong data"), signResult.Bundle)
		if err != nil {
			t.Fatalf("VerifyBundle failed: %v", err)
		}
		if result.Valid {
			t.Error("expected invalid result for wrong data")
		}
	})
}

func TestVerifier_VerifyBundle_RequiredAnnotations(t *testing.T) {
	priv, pub, err := GenerateKeyPair(KeyTypeECDSA, 256)
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	tmpDir := t.TempDir()
	privPath := filepath.Join(tmpDir, "private.key")
	if err := os.WriteFile(privPath, priv, 0600); err != nil {
		t.Fatalf("failed to write private key: %v", err)
	}

	// Create signer with annotations
	signer, err := NewSigner(&SigningConfig{
		KeyPath: privPath,
		Format:  SignatureFormatBundle,
		Annotations: map[string]string{
			"vendor":  "test-vendor",
			"version": "1.0.0",
		},
	})
	if err != nil {
		t.Fatalf("NewSigner failed: %v", err)
	}

	data := []byte("test data")
	signResult, err := signer.Sign(context.Background(), data)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	t.Run("matching annotations", func(t *testing.T) {
		verifier, _ := NewVerifier(&VerificationConfig{
			PublicKeyData: pub,
			ExpectedAnnotations: map[string]string{
				"vendor": "test-vendor",
			},
		})

		result, err := verifier.VerifyBundle(context.Background(), data, signResult.Bundle)
		if err != nil {
			t.Fatalf("VerifyBundle failed: %v", err)
		}
		if !result.Valid {
			t.Errorf("verification failed: %v", result.Errors)
		}
	})

	t.Run("missing annotation", func(t *testing.T) {
		verifier, _ := NewVerifier(&VerificationConfig{
			PublicKeyData: pub,
			ExpectedAnnotations: map[string]string{
				"missing": "value",
			},
		})

		result, err := verifier.VerifyBundle(context.Background(), data, signResult.Bundle)
		if err != nil {
			t.Fatalf("VerifyBundle failed: %v", err)
		}
		if result.Valid {
			t.Error("expected invalid for missing annotation")
		}
	})

	t.Run("wrong annotation value", func(t *testing.T) {
		verifier, _ := NewVerifier(&VerificationConfig{
			PublicKeyData: pub,
			ExpectedAnnotations: map[string]string{
				"vendor": "wrong-vendor",
			},
		})

		result, err := verifier.VerifyBundle(context.Background(), data, signResult.Bundle)
		if err != nil {
			t.Fatalf("VerifyBundle failed: %v", err)
		}
		if result.Valid {
			t.Error("expected invalid for wrong annotation value")
		}
	})
}

func TestVerifier_VerifyBlueprint(t *testing.T) {
	priv, pub, err := GenerateKeyPair(KeyTypeECDSA, 256)
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	tmpDir := t.TempDir()
	privPath := filepath.Join(tmpDir, "private.key")
	archivePath := filepath.Join(tmpDir, "blueprint.tar.gz")

	if err := os.WriteFile(privPath, priv, 0600); err != nil {
		t.Fatalf("failed to write private key: %v", err)
	}
	if err := os.WriteFile(archivePath, []byte("fake archive content"), 0644); err != nil {
		t.Fatalf("failed to create archive: %v", err)
	}

	signer, err := NewSigner(&SigningConfig{KeyPath: privPath})
	if err != nil {
		t.Fatalf("NewSigner failed: %v", err)
	}

	signResult, err := signer.SignBlueprint(context.Background(), archivePath)
	if err != nil {
		t.Fatalf("SignBlueprint failed: %v", err)
	}

	verifier, err := NewVerifier(&VerificationConfig{PublicKeyData: pub})
	if err != nil {
		t.Fatalf("NewVerifier failed: %v", err)
	}

	result, err := verifier.VerifyBlueprint(context.Background(), archivePath, signResult.Signature)
	if err != nil {
		t.Fatalf("VerifyBlueprint failed: %v", err)
	}

	if !result.Valid {
		t.Errorf("verification failed: %v", result.Errors)
	}
}

func TestVerifier_VerifyBlueprint_FileNotFound(t *testing.T) {
	_, pub, err := GenerateKeyPair(KeyTypeECDSA, 256)
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	verifier, err := NewVerifier(&VerificationConfig{PublicKeyData: pub})
	if err != nil {
		t.Fatalf("NewVerifier failed: %v", err)
	}

	_, err = verifier.VerifyBlueprint(context.Background(), "/nonexistent/archive.tar.gz", "sig")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestVerifier_GetPublicKeyFingerprint(t *testing.T) {
	_, pub, err := GenerateKeyPair(KeyTypeECDSA, 256)
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	verifier, err := NewVerifier(&VerificationConfig{PublicKeyData: pub})
	if err != nil {
		t.Fatalf("NewVerifier failed: %v", err)
	}

	fingerprint := verifier.GetPublicKeyFingerprint()
	if fingerprint == "" {
		t.Error("fingerprint is empty")
	}
	if len(fingerprint) < 10 {
		t.Error("fingerprint is too short")
	}
}

func TestVerifyDigest(t *testing.T) {
	data := []byte("test data")

	t.Run("valid digest", func(t *testing.T) {
		// SHA-256 of "test data"
		digest := "sha256:916f0027a575074ce72a331777c3478d6513f786a591bd892da1a577bf2335f9"
		if !VerifyDigest(data, digest) {
			t.Error("digest verification failed")
		}
	})

	t.Run("valid digest without prefix", func(t *testing.T) {
		digest := "916f0027a575074ce72a331777c3478d6513f786a591bd892da1a577bf2335f9"
		if !VerifyDigest(data, digest) {
			t.Error("digest verification failed")
		}
	})

	t.Run("invalid digest", func(t *testing.T) {
		if VerifyDigest(data, "sha256:invalid") {
			t.Error("expected verification to fail for invalid digest")
		}
	})
}

func TestVerifyBlueprintDigest(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "archive.tar.gz")

	if err := os.WriteFile(archivePath, []byte("test content"), 0644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	t.Run("valid", func(t *testing.T) {
		// SHA-256 of "test content"
		digest := "sha256:6ae8a75555209fd6c44157c0aed8016e763ff435a19cf186f76863140143ff72"
		valid, err := VerifyBlueprintDigest(archivePath, digest)
		if err != nil {
			t.Fatalf("VerifyBlueprintDigest failed: %v", err)
		}
		if !valid {
			t.Error("digest verification failed")
		}
	})

	t.Run("file not found", func(t *testing.T) {
		_, err := VerifyBlueprintDigest("/nonexistent/file", "digest")
		if err == nil {
			t.Error("expected error for nonexistent file")
		}
	})
}

func TestTrustPolicyEvaluator(t *testing.T) {
	policy := &TrustPolicy{
		Name:               "test-policy",
		RequireSignature:   true,
		RequireCertificate: false,
		TrustedIdentities:  []string{"test@example.com", "*.example.org"},
		RequiredAnnotations: map[string]string{
			"vendor": "trusted-vendor",
		},
	}

	evaluator := NewTrustPolicyEvaluator(policy)

	t.Run("valid result", func(t *testing.T) {
		result := &VerificationResult{
			Valid:          true,
			SignerIdentity: "test@example.com",
			Annotations: map[string]string{
				"vendor": "trusted-vendor",
			},
		}

		valid, violations := evaluator.Evaluate(result)
		if !valid {
			t.Errorf("evaluation failed: %v", violations)
		}
	})

	t.Run("signature required but invalid", func(t *testing.T) {
		result := &VerificationResult{
			Valid: false,
		}

		valid, violations := evaluator.Evaluate(result)
		if valid {
			t.Error("expected evaluation to fail")
		}
		if len(violations) == 0 {
			t.Error("expected violations")
		}
	})

	t.Run("untrusted identity", func(t *testing.T) {
		result := &VerificationResult{
			Valid:          true,
			SignerIdentity: "untrusted@other.com",
			Annotations: map[string]string{
				"vendor": "trusted-vendor",
			},
		}

		valid, violations := evaluator.Evaluate(result)
		if valid {
			t.Error("expected evaluation to fail for untrusted identity")
		}
		if len(violations) == 0 {
			t.Error("expected violations")
		}
	})

	t.Run("wildcard identity match", func(t *testing.T) {
		result := &VerificationResult{
			Valid:          true,
			SignerIdentity: "user@sub.example.org",
			Annotations: map[string]string{
				"vendor": "trusted-vendor",
			},
		}

		valid, violations := evaluator.Evaluate(result)
		if !valid {
			t.Errorf("wildcard identity should match: %v", violations)
		}
	})

	t.Run("missing annotation", func(t *testing.T) {
		result := &VerificationResult{
			Valid:          true,
			SignerIdentity: "test@example.com",
			Annotations:    map[string]string{},
		}

		valid, violations := evaluator.Evaluate(result)
		if valid {
			t.Error("expected evaluation to fail for missing annotation")
		}
		if len(violations) == 0 {
			t.Error("expected violations")
		}
	})

	t.Run("wrong annotation value", func(t *testing.T) {
		result := &VerificationResult{
			Valid:          true,
			SignerIdentity: "test@example.com",
			Annotations: map[string]string{
				"vendor": "wrong-vendor",
			},
		}

		valid, violations := evaluator.Evaluate(result)
		if valid {
			t.Error("expected evaluation to fail for wrong annotation")
		}
		if len(violations) == 0 {
			t.Error("expected violations")
		}
	})
}

func TestMatchIdentity(t *testing.T) {
	tests := []struct {
		identity string
		pattern  string
		expected bool
	}{
		{"test@example.com", "test@example.com", true},
		{"test@example.com", "other@example.com", false},
		{"test@example.com", "*", true},
		{"test@example.com", "test@*", true},
		{"test@example.com", "*@example.com", true},
		{"test@example.com", "*@other.com", false},
		{"user@sub.example.org", "*.example.org", true},
		{"user@example.org", "*.example.org", false}, // No subdomain
	}

	for _, tt := range tests {
		t.Run(tt.identity+"_"+tt.pattern, func(t *testing.T) {
			if matchIdentity(tt.identity, tt.pattern) != tt.expected {
				t.Errorf("matchIdentity(%q, %q) = %v, want %v",
					tt.identity, tt.pattern, !tt.expected, tt.expected)
			}
		})
	}
}
