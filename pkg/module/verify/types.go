package verify

// HashAlgorithm represents a cryptographic hash algorithm
type HashAlgorithm string

// SHA256 and related constants.
const (
	SHA256 HashAlgorithm = "sha256"
	SHA512 HashAlgorithm = "sha512"
)

// SignatureFormat represents a signature format
type SignatureFormat string

// SignatureFormatPKCS1 constants define the output formats.
const (
	SignatureFormatPKCS1 SignatureFormat = "pkcs1"
	SignatureFormatPSS   SignatureFormat = "pss"
	CosignFormat         SignatureFormat = "cosign"
)

// HashVerifier computes and verifies hashes
type HashVerifier interface {
	ComputeHash(path string) (string, error)
	VerifyHash(modulePath, expectedHash string) (bool, error)
}

// SignatureVerifier verifies digital signatures
type SignatureVerifier interface {
	VerifySignature(modulePath, signaturePath string, publicKey []byte) (bool, error)
	GetSignerIdentity(signaturePath string) (string, error)
}

// TrustPolicy determines module trust
type TrustPolicy interface {
	IsTrusted(identity string) bool
	AddTrustedKey(identity string, publicKey []byte) error
	RemoveTrustedKey(identity string) error
	ListTrustedKeys() []string
}

// SumDBClient interacts with a transparency log
type SumDBClient interface {
	Lookup(moduleName, version string) (string, error)
	Record(moduleName, version, hash string) error
	Verify(moduleName, version, hash string) (bool, error)
}

// VerificationOptions configures verification
type VerificationOptions struct {
	RequireSignature bool
	RequireSumDB     bool
	RequireHashMatch bool
	AllowInsecure    bool
	TrustedKeys      [][]byte
	TrustedKeyIDs    []string
}

// DefaultVerificationOptions returns default verification options
func DefaultVerificationOptions() *VerificationOptions {
	return &VerificationOptions{
		RequireSignature: true,
		RequireSumDB:     false,
		RequireHashMatch: false,
	}
}

// VerificationResult contains verification results
type VerificationResult struct {
	ContentHash    string
	Verified       bool
	HashValid      bool
	SignatureValid bool
	TrustedKey     bool
	SumDBVerified  bool
	SignerIdentity string
	Errors         []error
	Warnings       []string
}

// AddError adds an error to the result
func (r *VerificationResult) AddError(err error) {
	r.Verified = false
	r.Errors = append(r.Errors, err)
}

// AddWarning adds a warning to the result
func (r *VerificationResult) AddWarning(warning string) {
	r.Warnings = append(r.Warnings, warning)
}

// IsValid returns true if the verification result is valid (no errors)
func (r *VerificationResult) IsValid() bool {
	return r.Verified && len(r.Errors) == 0
}

// ModuleArtifact represents a module being verified
type ModuleArtifact struct {
	Name          string
	Version       string
	Path          string
	Hash          string
	SignaturePath string
}

// SumDB is an interface for transparency logs
type SumDB interface {
	Verify(moduleName, version, hash string) (bool, error)
}

// VerificationReport contains detailed verification information
type VerificationReport struct {
	Result       *VerificationResult
	ComputedHash string
	ExpectedHash string
	Verified     bool
	HashMatches  bool
	SignatureOK  bool
	SumDBChecked bool
	Errors       []error
}

// NewVerificationReport creates a new verification report
func NewVerificationReport() *VerificationReport {
	return &VerificationReport{
		Result: &VerificationResult{
			Verified: true,
			Errors:   make([]error, 0),
			Warnings: make([]string, 0),
		},
		Verified:     true,
		HashMatches:  false,
		SignatureOK:  false,
		SumDBChecked: false,
		Errors:       make([]error, 0),
	}
}
