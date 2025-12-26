package verify

import "errors"

var (
	// ErrHashMismatch indicates the computed hash doesn't match the expected hash
	ErrHashMismatch = errors.New("hash mismatch")

	// ErrInvalidSignature indicates the signature is invalid
	ErrInvalidSignature = errors.New("invalid signature")

	// ErrSignatureNotFound indicates no signature file was found
	ErrSignatureNotFound = errors.New("signature not found")

	// ErrUntrustedKey indicates the signing key is not trusted
	ErrUntrustedKey = errors.New("untrusted signing key")

	// ErrSumDBVerificationFailed indicates SumDB verification failed
	ErrSumDBVerificationFailed = errors.New("SumDB verification failed")

	// ErrModuleNotFound indicates the module was not found
	ErrModuleNotFound = errors.New("module not found")

	// ErrInvalidModulePath indicates an invalid module path
	ErrInvalidModulePath = errors.New("invalid module path")

	// ErrInvalidHash indicates an invalid hash format
	ErrInvalidHash = errors.New("invalid hash format")

	// ErrInvalidPublicKey indicates an invalid public key format
	ErrInvalidPublicKey = errors.New("invalid public key format")

	// ErrVerificationFailed indicates verification failed
	ErrVerificationFailed = errors.New("verification failed")

	// ErrSumDBUnavailable indicates the SumDB is unavailable
	ErrSumDBUnavailable = errors.New("SumDB unavailable")

	// ErrInvalidSignatureFormat indicates an unsupported signature format
	ErrInvalidSignatureFormat = errors.New("invalid signature format")
)
