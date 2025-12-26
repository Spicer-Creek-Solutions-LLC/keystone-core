package verify

import (
	"time"
)

// VerificationResult represents the result of a verification operation
type VerificationResult struct {
	// Verified indicates if the verification succeeded
	Verified bool

	// SignatureValid indicates if the cryptographic signature is valid
	SignatureValid bool

	// HashValid indicates if the hash matches
	HashValid bool

	// TrustedKey indicates if the signing key is trusted
	TrustedKey bool

	// SumDBVerified indicates if SumDB verification succeeded
	SumDBVerified bool

	// SignerIdentity is the identity of the signer (email, key ID, etc.)
	SignerIdentity string

	// Timestamp when the signature was created
	Timestamp time.Time

	// Errors encountered during verification
	Errors []error

	// Warnings that don't prevent verification but should be noted
	Warnings []string
}

// IsValid returns true if all verification checks passed
func (r *VerificationResult) IsValid() bool {
	return r.Verified && len(r.Errors) == 0
}

// AddError adds an error to the result
func (r *VerificationResult) AddError(err error) {
	r.Errors = append(r.Errors, err)
	r.Verified = false
}

// AddWarning adds a warning to the result
func (r *VerificationResult) AddWarning(warning string) {
	r.Warnings = append(r.Warnings, warning)
}

// VerificationOptions configures verification behavior
type VerificationOptions struct {
	// RequireSignature determines if a valid signature is required
	RequireSignature bool

	// RequireHashMatch determines if hash verification is required
	RequireHashMatch bool

	// RequireSumDB determines if SumDB verification is required
	RequireSumDB bool

	// TrustedKeys is a list of trusted public keys (PEM format)
	TrustedKeys []string

	// TrustedKeyIDs is a list of trusted key IDs
	TrustedKeyIDs []string

	// AllowInsecure allows verification to succeed even if checks fail (for testing)
	AllowInsecure bool

	// SkipTLSVerify skips TLS verification for remote operations
	SkipTLSVerify bool
}

// DefaultVerificationOptions returns secure default verification options
func DefaultVerificationOptions() *VerificationOptions {
	return &VerificationOptions{
		RequireSignature: true,
		RequireHashMatch: true,
		RequireSumDB:     true,
		AllowInsecure:    false,
		SkipTLSVerify:    false,
	}
}

// Verifier defines the interface for module verification
type Verifier interface {
	// Verify verifies a module
	Verify(modulePath string, opts *VerificationOptions) (*VerificationResult, error)
}

// HashVerifier verifies module hashes
type HashVerifier interface {
	// VerifyHash checks if the module's hash matches the expected hash
	VerifyHash(modulePath, expectedHash string) (bool, error)

	// ComputeHash computes the hash of a module
	ComputeHash(modulePath string) (string, error)
}

// SignatureVerifier verifies cryptographic signatures
type SignatureVerifier interface {
	// VerifySignature verifies a signature against a module
	VerifySignature(modulePath, signaturePath string, publicKey []byte) (bool, error)

	// GetSignerIdentity extracts the signer's identity from a signature
	GetSignerIdentity(signaturePath string) (string, error)
}

// SumDBClient interacts with a transparency log (SumDB)
type SumDBClient interface {
	// Lookup retrieves the hash for a module from the SumDB
	Lookup(moduleName, version string) (string, error)

	// Verify verifies a module against the SumDB
	Verify(moduleName, version, hash string) (bool, error)

	// Submit submits a new module hash to the SumDB
	Submit(moduleName, version, hash string) error
}

// TrustPolicy determines if a key or signer is trusted
type TrustPolicy interface {
	// IsTrusted checks if a key ID or identity is trusted
	IsTrusted(identity string) bool

	// AddTrustedKey adds a trusted key
	AddTrustedKey(identity string, publicKey []byte) error

	// RemoveTrustedKey removes a trusted key
	RemoveTrustedKey(identity string) error

	// ListTrustedKeys returns all trusted key identities
	ListTrustedKeys() []string
}

// ModuleArtifact represents a module package to be verified
type ModuleArtifact struct {
	// Name is the module name (e.g., "vendor/pkg_apt")
	Name string

	// Version is the module version (e.g., "v1.2.3")
	Version string

	// Path is the path to the module file (.zip or directory)
	Path string

	// Hash is the expected hash (if known)
	Hash string

	// SignaturePath is the path to the detached signature file
	SignaturePath string
}

// VerificationReport provides detailed verification information
type VerificationReport struct {
	// Artifact being verified
	Artifact *ModuleArtifact

	// Result of verification
	Result *VerificationResult

	// ComputedHash is the actual hash of the module
	ComputedHash string

	// ExpectedHash is the expected hash (from lockfile or SumDB)
	ExpectedHash string

	// SignatureDetails contains information about the signature
	SignatureDetails map[string]interface{}

	// TrustChain describes the chain of trust
	TrustChain []string

	// VerificationTime when verification was performed
	VerificationTime time.Time
}

// NewVerificationReport creates a new verification report
func NewVerificationReport(artifact *ModuleArtifact) *VerificationReport {
	return &VerificationReport{
		Artifact:         artifact,
		Result:           &VerificationResult{},
		SignatureDetails: make(map[string]interface{}),
		TrustChain:       make([]string, 0),
		VerificationTime: time.Now(),
	}
}

// HashAlgorithm defines supported hash algorithms
type HashAlgorithm string

const (
	// SHA256 is the SHA-256 hash algorithm
	SHA256 HashAlgorithm = "sha256"

	// SHA512 is the SHA-512 hash algorithm
	SHA512 HashAlgorithm = "sha512"
)

// SignatureFormat defines supported signature formats
type SignatureFormat string

const (
	// CosignFormat is the Cosign signature format
	CosignFormat SignatureFormat = "cosign"

	// PGPFormat is the PGP/GPG signature format
	PGPFormat SignatureFormat = "pgp"
)
