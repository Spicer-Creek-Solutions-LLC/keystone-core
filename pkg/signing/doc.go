// Package signing provides unified cryptographic signing and verification.
//
// This package consolidates signing operations for both modules and blueprints,
// supporting both traditional key-based signing and keyless signing via Sigstore/Fulcio.
//
// # Key-Based Signing
//
// Key-based signing uses traditional private/public key pairs (RSA, ECDSA, Ed25519):
//
//	// Generate a key pair
//	keyPair, err := signing.GenerateKeyPair(signing.KeyTypeECDSA, 256)
//
//	// Create a signer
//	signer, err := signing.NewKeySigner(&signing.KeySignerConfig{
//	    PrivateKeyPEM: keyPair.PrivateKey,
//	})
//
//	// Sign data
//	result, err := signer.Sign(ctx, data)
//
//	// Verify signature
//	verifier, err := signing.NewKeyVerifier(&signing.KeyVerifierConfig{
//	    PublicKeyPEM: keyPair.PublicKey,
//	})
//	valid, err := verifier.Verify(ctx, data, result.Signature)
//
// # Keyless Signing (CI/CD)
//
// Keyless signing uses Sigstore/Fulcio to obtain short-lived certificates
// tied to an OIDC identity. This is ideal for CI/CD environments where
// OIDC tokens are available automatically:
//
//	// In GitHub Actions, GitLab CI, etc.
//	signer, err := signing.NewKeylessSigner(&signing.KeylessSignerConfig{
//	    OIDCToken: os.Getenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN"),
//	})
//
//	// Sign data (generates ephemeral key, gets Fulcio certificate, signs)
//	result, err := signer.Sign(ctx, data)
//
//	// The result includes the signature and certificate
//	fmt.Println(result.SignerIdentity) // e.g., "user@example.com"
//
// # Signature Formats
//
// The package supports multiple signature formats:
//   - FormatRaw: Raw binary signature
//   - FormatBase64: Base64-encoded signature
//   - FormatCosign: Cosign-compatible format
//   - FormatBundle: JSON bundle with signature and metadata
//
// # Supported Key Types
//
//   - ECDSA (P-256, P-384, P-521)
//   - RSA (2048+)
//   - Ed25519
//
// # Hash Algorithms
//
//   - SHA-256 (default)
//   - SHA-384
//   - SHA-512
package signing
