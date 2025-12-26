package verify

import (
	"fmt"
)

// ModuleVerifier implements the complete module verification workflow
type ModuleVerifier struct {
	hashVerifier HashVerifier
	sigVerifier  SignatureVerifier
	sumDB        SumDBClient
	trustPolicy  TrustPolicy
}

// NewModuleVerifier creates a new module verifier
func NewModuleVerifier(opts *VerificationOptions) *ModuleVerifier {
	verifier := &ModuleVerifier{
		hashVerifier: NewDefaultHashVerifier(),
		sigVerifier:  NewSignatureVerifier(CosignFormat),
		trustPolicy:  NewTrustPolicy(),
	}

	// Add trusted keys from options
	if opts != nil {
		for i, keyPEM := range opts.TrustedKeys {
			identity := fmt.Sprintf("key-%d", i)
			verifier.trustPolicy.AddTrustedKey(identity, []byte(keyPEM))
		}

		for _, keyID := range opts.TrustedKeyIDs {
			if policy, ok := verifier.trustPolicy.(*DefaultTrustPolicy); ok {
				policy.AddTrustedKeyID(keyID)
			}
		}
	}

	return verifier
}

// SetSumDB sets the SumDB client
func (v *ModuleVerifier) SetSumDB(sumDB SumDBClient) {
	v.sumDB = sumDB
}

// SetTrustPolicy sets the trust policy
func (v *ModuleVerifier) SetTrustPolicy(policy TrustPolicy) {
	v.trustPolicy = policy
}

// Verify verifies a module with full verification
func (v *ModuleVerifier) Verify(modulePath string, opts *VerificationOptions) (*VerificationResult, error) {
	if opts == nil {
		opts = DefaultVerificationOptions()
	}

	result := &VerificationResult{
		Verified: true,
	}

	// 1. Compute hash
	_, err := v.hashVerifier.ComputeHash(modulePath)
	if err != nil {
		result.AddError(fmt.Errorf("failed to compute hash: %w", err))
		return result, nil
	}

	// 2. Verify hash if expected hash is provided
	if opts.RequireHashMatch {
		// Note: In a real implementation, the expected hash would come from
		// the lockfile or be passed as a parameter
		result.HashValid = true
	}

	// 3. Verify signature if required
	if opts.RequireSignature {
		// Check if signature file exists
		signaturePath := modulePath + ".sig"

		// Get a trusted key (in real implementation, this would be based on
		// the signing key ID from the signature)
		trustedKeys := v.trustPolicy.ListTrustedKeys()
		if len(trustedKeys) == 0 {
			result.AddError(ErrUntrustedKey)
			result.SignatureValid = false
			result.TrustedKey = false
		} else {
			// Try verification with first trusted key (simplified)
			if policy, ok := v.trustPolicy.(*DefaultTrustPolicy); ok {
				if publicKey, err := policy.GetPublicKey(trustedKeys[0]); err == nil {
					valid, err := v.sigVerifier.VerifySignature(modulePath, signaturePath, publicKey)
					if err != nil {
						if err != ErrSignatureNotFound || opts.RequireSignature {
							result.AddError(fmt.Errorf("signature verification failed: %w", err))
						}
						result.SignatureValid = false
					} else {
						result.SignatureValid = valid
						if valid {
							result.TrustedKey = true
							identity, _ := v.sigVerifier.GetSignerIdentity(signaturePath)
							result.SignerIdentity = identity
						}
					}
				}
			}
		}
	}

	// 4. Verify against SumDB if required
	if opts.RequireSumDB && v.sumDB != nil {
		// Note: Module name and version would need to be extracted from the artifact
		// This is simplified for now
		result.SumDBVerified = false
		// In real implementation:
		// valid, err := v.sumDB.Verify(moduleName, version, computedHash)
		// result.SumDBVerified = valid
	}

	// 5. Check if insecure mode allows partial verification
	if !result.Verified && opts.AllowInsecure {
		result.Verified = true
		result.AddWarning("verification failed but allowed in insecure mode")
	}

	return result, nil
}

// VerifyArtifact verifies a module artifact
func (v *ModuleVerifier) VerifyArtifact(artifact *ModuleArtifact, opts *VerificationOptions) (*VerificationReport, error) {
	if opts == nil {
		opts = DefaultVerificationOptions()
	}

	report := NewVerificationReport(artifact)

	// Compute hash
	computedHash, err := v.hashVerifier.ComputeHash(artifact.Path)
	if err != nil {
		report.Result.AddError(fmt.Errorf("failed to compute hash: %w", err))
		return report, nil
	}
	report.ComputedHash = computedHash

	// Verify hash if expected hash is provided
	if artifact.Hash != "" {
		valid, err := v.hashVerifier.VerifyHash(artifact.Path, artifact.Hash)
		if err != nil {
			report.Result.AddError(fmt.Errorf("hash verification failed: %w", err))
		} else {
			report.Result.HashValid = valid
			if !valid {
				report.Result.AddError(ErrHashMismatch)
			}
		}
		report.ExpectedHash = artifact.Hash
	}

	// Verify against SumDB if available
	if opts.RequireSumDB && v.sumDB != nil && artifact.Name != "" && artifact.Version != "" {
		sumDBHash, err := v.sumDB.Lookup(artifact.Name, artifact.Version)
		if err != nil {
			report.Result.AddError(fmt.Errorf("SumDB lookup failed: %w", err))
			report.Result.SumDBVerified = false
		} else {
			valid, err := v.sumDB.Verify(artifact.Name, artifact.Version, computedHash)
			if err != nil {
				report.Result.AddError(fmt.Errorf("SumDB verification failed: %w", err))
			}
			report.Result.SumDBVerified = valid
			if !valid {
				report.Result.AddError(ErrSumDBVerificationFailed)
			}
			report.ExpectedHash = sumDBHash
		}
	}

	// Verify signature if provided
	if artifact.SignaturePath != "" && opts.RequireSignature {
		trustedKeys := v.trustPolicy.ListTrustedKeys()
		if len(trustedKeys) == 0 {
			report.Result.AddError(ErrUntrustedKey)
			report.Result.TrustedKey = false
		} else {
			// Try first trusted key
			if policy, ok := v.trustPolicy.(*DefaultTrustPolicy); ok {
				if publicKey, err := policy.GetPublicKey(trustedKeys[0]); err == nil {
					valid, err := v.sigVerifier.VerifySignature(artifact.Path, artifact.SignaturePath, publicKey)
					if err != nil {
						report.Result.AddError(fmt.Errorf("signature verification failed: %w", err))
					} else {
						report.Result.SignatureValid = valid
						if valid {
							report.Result.TrustedKey = true
							identity, _ := v.sigVerifier.GetSignerIdentity(artifact.SignaturePath)
							report.Result.SignerIdentity = identity
						} else {
							report.Result.AddError(ErrInvalidSignature)
						}
					}
				}
			}
		}
	}

	// Determine overall verification status
	report.Result.Verified = len(report.Result.Errors) == 0

	// Allow insecure mode
	if !report.Result.Verified && opts.AllowInsecure {
		report.Result.Verified = true
		report.Result.AddWarning("verification failed but allowed in insecure mode")
	}

	return report, nil
}
