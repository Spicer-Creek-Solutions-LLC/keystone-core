// SPDX-License-Identifier: Apache-2.0

package secrets

import "context"

// TransitBackend is the encryption-as-a-service surface per
// PROJECT-DETAILS §4.11. Distinct from [SecretBackend] because the
// transit primitives don't map onto KV / lease semantics — operators
// hand a TransitBackend instance directly to whatever consumer needs
// inline encrypt / decrypt / sign / verify / HMAC.
//
// v1.0 ships one implementation: Vault's transit engine (see
// internal/secrets/vault). Cloud KMS backends are v2.x+ per FEATURES.md.
// The encrypted-file backend (task 4) deliberately does NOT implement
// TransitBackend — a future "file transit" would need its own
// AES-GCM key-management story.
//
// Single vs batch: every method accepts a slice of items. Single-op
// callers pass a one-element slice; bulk ops pass many. The
// implementation selects Vault's `plaintext` / `batch_input` wire
// shape per call. Partial-batch failures return a non-nil response
// + nil top-level error with per-item `Err` populated; check the
// per-item field. Whole-batch failures (auth, network, malformed
// request) return a non-nil top-level error.
type TransitBackend interface {
	// Encrypt encrypts each item's plaintext under the named transit
	// key. Returns ciphertexts in Vault's wire format
	// `vault:vN:base64...` — store verbatim, pass back unchanged.
	Encrypt(ctx context.Context, req EncryptRequest) (*EncryptResponse, error)

	// Decrypt is the inverse of [TransitBackend.Encrypt].
	Decrypt(ctx context.Context, req DecryptRequest) (*DecryptResponse, error)

	// Sign signs each item's Input under the named transit key.
	// SignatureAlgorithm / HashAlgorithm pin the signer; key type
	// must support the requested signature scheme.
	Sign(ctx context.Context, req SignRequest) (*SignResponse, error)

	// Verify checks each item's Signature against Input. Mismatch
	// returns Valid=false per-item, NOT a top-level error — a
	// failed verify is the expected outcome of `valid=false`.
	Verify(ctx context.Context, req VerifyRequest) (*VerifyResponse, error)

	// HMAC computes the MAC of each item's Input. Faster than
	// [TransitBackend.Sign] on symmetric keys for integrity-only
	// use cases.
	HMAC(ctx context.Context, req HMACRequest) (*HMACResponse, error)

	// VerifyHMAC checks each item's MAC against Input.
	VerifyHMAC(ctx context.Context, req VerifyHMACRequest) (*VerifyResponse, error)

	// Rewrap re-encrypts each item's Ciphertext under the latest
	// version of the named key — post-rotation maintenance.
	Rewrap(ctx context.Context, req RewrapRequest) (*RewrapResponse, error)

	// GenerateDataKey returns a fresh data key suitable for
	// envelope encryption. Plaintext is the key bytes for in-process
	// use; Ciphertext is the same key wrapped under the transit KEK
	// — the caller stores Ciphertext alongside its data and
	// decrypts to recover Plaintext later.
	GenerateDataKey(ctx context.Context, req GenerateDataKeyRequest) (*GenerateDataKeyResponse, error)
}

// EncryptRequest drives [TransitBackend.Encrypt]. Key names the
// transit key; Items is the per-input slice.
type EncryptRequest struct {
	Key   string
	Items []EncryptInput
}

// EncryptInput is one plaintext + its per-item options.
//
// Context is the derivation context for `derived` / convergent keys
// — same plaintext + same context + same key + same nonce produces
// the same ciphertext (deterministic encryption for searchable
// fields). Convergent v3 keys auto-generate the nonce; older
// convergent versions require an explicit Nonce.
//
// KeyVersion picks the version to encrypt with — 0 means "latest"
// (Vault default). Operators only need this when they're explicitly
// pinning to an older version for compatibility.
type EncryptInput struct {
	Plaintext  []byte
	Context    []byte
	KeyVersion int
	Nonce      []byte
}

// EncryptResponse carries per-item results. Top-level Err is empty
// when the whole batch dispatched cleanly; per-item Err in Results
// reports partial failures (e.g., wrong Context on one convergent
// item).
type EncryptResponse struct {
	Results []EncryptResult
}

// EncryptResult is one encrypted item.
type EncryptResult struct {
	Ciphertext string
	KeyVersion int
	Err        string
}

// DecryptRequest drives [TransitBackend.Decrypt].
type DecryptRequest struct {
	Key   string
	Items []DecryptInput
}

// DecryptInput is one ciphertext + its per-item options. Context
// must match what was supplied at encrypt time for convergent keys.
type DecryptInput struct {
	Ciphertext string
	Context    []byte
	Nonce      []byte
}

// DecryptResponse carries per-item plaintexts.
type DecryptResponse struct {
	Results []DecryptResult
}

// DecryptResult is one decrypted item. Plaintext is empty when
// Err is non-empty.
type DecryptResult struct {
	Plaintext []byte
	Err       string
}

// SignRequest drives [TransitBackend.Sign].
//
// SignatureAlgorithm + HashAlgorithm are key-type-specific:
//   - RSA keys: SignatureAlgorithm in {"pss", "pkcs1v15"}; HashAlgorithm
//     in {"sha2-224", "sha2-256", "sha2-384", "sha2-512"}.
//   - ECDSA keys: SignatureAlgorithm typically empty (Vault default);
//     HashAlgorithm picks the digest.
//   - Ed25519 keys: both empty; Vault picks the canonical algorithm.
//
// Empty values fall through to Vault's defaults.
type SignRequest struct {
	Key                string
	HashAlgorithm      string
	SignatureAlgorithm string
	Items              []SignInput
}

// SignInput is one to-be-signed item.
//
// Prehashed reports that Input is already the digest (so Vault
// signs the bytes directly without re-hashing).
type SignInput struct {
	Input      []byte
	Context    []byte
	KeyVersion int
	Prehashed  bool
}

// SignResponse carries per-item signatures.
type SignResponse struct {
	Results []SignResult
}

// SignResult is one signature. Signature is Vault's
// `vault:vN:base64...` wire form.
type SignResult struct {
	Signature  string
	KeyVersion int
	Err        string
}

// VerifyRequest drives [TransitBackend.Verify].
type VerifyRequest struct {
	Key                string
	HashAlgorithm      string
	SignatureAlgorithm string
	Items              []VerifyInput
}

// VerifyInput is one to-be-verified pair.
type VerifyInput struct {
	Input     []byte
	Signature string
	Context   []byte
	Prehashed bool
}

// VerifyResponse carries per-item validity. Valid=false is the
// expected outcome of a mismatched signature/HMAC; the top-level
// call still succeeds.
type VerifyResponse struct {
	Results []VerifyResult
}

// VerifyResult is one verification outcome.
type VerifyResult struct {
	Valid bool
	Err   string
}

// HMACRequest drives [TransitBackend.HMAC].
type HMACRequest struct {
	Key           string
	Algorithm     string // "sha2-256" / "sha2-384" / "sha2-512" / etc; empty = Vault default
	KeyVersion    int
	Items         []HMACInput
}

// HMACInput is one to-be-MAC'd item.
type HMACInput struct {
	Input []byte
}

// HMACResponse carries per-item MACs.
type HMACResponse struct {
	Results []HMACResult
}

// HMACResult is one MAC in Vault's wire form.
type HMACResult struct {
	HMAC       string
	KeyVersion int
	Err        string
}

// VerifyHMACRequest drives [TransitBackend.VerifyHMAC].
type VerifyHMACRequest struct {
	Key       string
	Algorithm string
	Items     []VerifyHMACInput
}

// VerifyHMACInput is one to-be-verified MAC pair.
type VerifyHMACInput struct {
	Input []byte
	HMAC  string
}

// RewrapRequest drives [TransitBackend.Rewrap]. Items carry the old
// ciphertexts (in Vault wire form); the response carries the
// re-encrypted ones bumped to the key's latest version.
type RewrapRequest struct {
	Key   string
	Items []RewrapInput
}

// RewrapInput is one to-be-rewrapped ciphertext.
type RewrapInput struct {
	Ciphertext string
	Context    []byte
	Nonce      []byte
	KeyVersion int // target version; 0 = latest
}

// RewrapResponse carries per-item re-encrypted ciphertexts.
type RewrapResponse struct {
	Results []RewrapResult
}

// RewrapResult is one re-wrapped ciphertext.
type RewrapResult struct {
	Ciphertext string
	KeyVersion int
	Err        string
}

// DataKeyMode picks whether [TransitBackend.GenerateDataKey] returns
// the plaintext key alongside the wrapped form.
type DataKeyMode string

const (
	// DataKeyModePlaintext returns both the plaintext key bytes
	// and the Vault-wrapped ciphertext. Use when the caller needs
	// to use the key immediately AND store the wrapped form for
	// future decryption.
	DataKeyModePlaintext DataKeyMode = "plaintext"

	// DataKeyModeWrapped returns only the wrapped ciphertext (the
	// plaintext is generated inside Vault and never crosses the
	// wire). The caller decrypts the wrapped form on demand. Use
	// for at-rest storage of the key where the caller's process
	// doesn't need the cleartext at issue time.
	DataKeyModeWrapped DataKeyMode = "wrapped"
)

// GenerateDataKeyRequest drives [TransitBackend.GenerateDataKey].
//
// Bits picks the AES key size — 128, 256, or 512 (512 is supported
// only for HMAC-only use; Vault rejects it for encryption). Default
// (0) falls through to Vault's default (256).
type GenerateDataKeyRequest struct {
	Key     string
	Mode    DataKeyMode
	Context []byte
	Bits    int
}

// GenerateDataKeyResponse carries the materialised data key.
//
// Plaintext is empty when Mode == DataKeyModeWrapped — the caller
// recovers the key bytes by Decrypting Ciphertext through the same
// transit key.
type GenerateDataKeyResponse struct {
	Plaintext  []byte
	Ciphertext string
	KeyVersion int
}
