package file

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"

	"go.keystone-core.io/keystone-core/internal/secrets"
)

// KeyLen is the AES-256 key size — 32 bytes.
const KeyLen = 32

// FingerprintLen is the prefix of `sha256(key)` written into the
// on-disk envelope's `keyID` field. 8 bytes balances "obvious
// mismatch on wrong key" against "no useful information about the
// key bytes."
const FingerprintLen = 8

// MasterKey is the AES-256 key used to encrypt the cleartext state
// file at rest. Constructed via [ResolveMasterKey] (operator config)
// or [NewRandomMasterKey] (tests). The zero value is invalid.
//
// MasterKey is a value type — copy-by-value is fine and the key bytes
// are never exported. [MasterKey.Fingerprint] returns a stable hex
// prefix derived from `sha256(key)` for diagnostic log lines; the key
// bytes themselves never appear in [MasterKey.String] / Fingerprint.
type MasterKey struct {
	key [KeyLen]byte
}

// Bytes returns a defensive copy of the key bytes. Backends call this
// to feed `crypto/aes.NewCipher`; callers MUST zero the slice when
// done. Tests use it to encode test fixtures.
func (k MasterKey) Bytes() []byte {
	out := make([]byte, KeyLen)
	copy(out, k.key[:])
	return out
}

// Fingerprint returns the hex of the first [FingerprintLen] bytes of
// `sha256(key)`. Written into the on-disk envelope and surfaced in
// operator logs so a wrong-key boot fails fast with a recognisable
// fingerprint mismatch instead of an opaque "decryption failed."
func (k MasterKey) Fingerprint() string {
	sum := sha256.Sum256(k.key[:])
	return hex.EncodeToString(sum[:FingerprintLen])
}

// FingerprintBytes returns the binary fingerprint for use in the
// on-disk envelope. Same prefix as [MasterKey.Fingerprint] —
// callers that need to write the fingerprint to bytes (storage layer)
// use this; callers logging the fingerprint use the hex form.
func (k MasterKey) FingerprintBytes() [FingerprintLen]byte {
	sum := sha256.Sum256(k.key[:])
	var out [FingerprintLen]byte
	copy(out[:], sum[:FingerprintLen])
	return out
}

// String is intentionally diagnostic-only — emits the fingerprint
// (not the key bytes) so accidental `%v` / `%s` formatting never
// leaks key material.
func (k MasterKey) String() string {
	return fmt.Sprintf("master-key(fp=%s)", k.Fingerprint())
}

// IsZero reports whether the receiver is the uninitialised value.
func (k MasterKey) IsZero() bool {
	return k.key == [KeyLen]byte{}
}

// NewRandomMasterKey returns a freshly random key via `crypto/rand`.
// Test / dev only — operators use [ResolveMasterKey] against a
// config-supplied source.
func NewRandomMasterKey() (MasterKey, error) {
	var k MasterKey
	if _, err := rand.Read(k.key[:]); err != nil {
		return MasterKey{}, fmt.Errorf("%w: random master key: %v", secrets.ErrInvalidBackend, err)
	}
	return k, nil
}

// MasterKeyFromBytes wraps an already-derived 32-byte key. The
// internal [ResolveMasterKey] machinery uses it; exposed for the
// `kscore-secrets` CLI (task 10) and rotation tooling.
func MasterKeyFromBytes(b []byte) (MasterKey, error) {
	if len(b) != KeyLen {
		return MasterKey{}, fmt.Errorf("%w: master key must be %d bytes, got %d", secrets.ErrInvalidBackend, KeyLen, len(b))
	}
	var k MasterKey
	copy(k.key[:], b)
	return k, nil
}

// ResolveMasterKey parses a scheme-prefixed source string from the
// operator config and returns the resolved [MasterKey].
//
// v1.0 schemes:
//
//   - `env:VAR_NAME` — read from the named env var (hex or base64).
//   - `file:/path/to/keyfile` — read from a file (binary 32 bytes,
//     hex, or base64). Trailing newlines are trimmed.
//   - `inline:<hex|base64>` — direct, intended for tests / dev. The
//     caller (the [file.Backend]) logs a WARN at boot when this
//     scheme resolves so operators don't ship `inline:` to production
//     unnoticed.
//
// Cloud KMS schemes are detected and rejected with a v2.0 pointer
// per FEATURES.md ("Cloud KMS for master keys — v2.0"). Unknown
// schemes return [secrets.ErrInvalidBackend] with the scheme name
// in the message.
//
// All errors wrap [secrets.ErrInvalidBackend].
func ResolveMasterKey(source string) (MasterKey, error) {
	if source == "" {
		return MasterKey{}, fmt.Errorf("%w: master key source is required", secrets.ErrInvalidBackend)
	}

	idx := strings.IndexByte(source, ':')
	if idx <= 0 {
		return MasterKey{}, fmt.Errorf("%w: master key source %q missing scheme (expected `env:`, `file:`, or `inline:`)", secrets.ErrInvalidBackend, source)
	}
	scheme, value := source[:idx], source[idx+1:]

	switch scheme {
	case "env":
		return resolveEnv(value)
	case "file":
		return resolveFile(value)
	case "inline":
		return decodeKeyMaterial(value, "inline")
	case "gcp-kms", "aws-kms", "azure-kv":
		return MasterKey{}, fmt.Errorf("%w: master key scheme %q is v2.0 (cloud KMS); v1.0 supports env, file, inline schemes", secrets.ErrInvalidBackend, scheme)
	default:
		return MasterKey{}, fmt.Errorf("%w: unknown master key scheme %q (expected env:, file:, or inline:)", secrets.ErrInvalidBackend, scheme)
	}
}

func resolveEnv(varName string) (MasterKey, error) {
	if varName == "" {
		return MasterKey{}, fmt.Errorf("%w: env: scheme requires a variable name", secrets.ErrInvalidBackend)
	}
	raw, ok := os.LookupEnv(varName)
	if !ok {
		return MasterKey{}, fmt.Errorf("%w: env var %q is not set", secrets.ErrInvalidBackend, varName)
	}
	if raw == "" {
		return MasterKey{}, fmt.Errorf("%w: env var %q is empty", secrets.ErrInvalidBackend, varName)
	}
	return decodeKeyMaterial(raw, fmt.Sprintf("env:%s", varName))
}

func resolveFile(path string) (MasterKey, error) {
	if path == "" {
		return MasterKey{}, fmt.Errorf("%w: file: scheme requires a path", secrets.ErrInvalidBackend)
	}
	raw, err := os.ReadFile(path) // #nosec G304 -- operator-supplied master key path from secrets.backends config
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return MasterKey{}, fmt.Errorf("%w: master key file %q does not exist", secrets.ErrInvalidBackend, path)
		}
		return MasterKey{}, fmt.Errorf("%w: read master key file %q: %v", secrets.ErrInvalidBackend, path, err)
	}
	// Allow binary 32-byte files OR an encoded form with a trailing newline.
	if len(raw) == KeyLen {
		return MasterKeyFromBytes(raw)
	}
	trimmed := strings.TrimSpace(string(raw))
	return decodeKeyMaterial(trimmed, fmt.Sprintf("file:%s", path))
}

// decodeKeyMaterial accepts hex (64 chars) or base64 (44 chars for
// 32 bytes, with or without padding) and returns a [MasterKey].
// Source describes where the material came from for error messages.
func decodeKeyMaterial(raw, source string) (MasterKey, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return MasterKey{}, fmt.Errorf("%w: %s: master key material is empty", secrets.ErrInvalidBackend, source)
	}

	if b, err := hex.DecodeString(raw); err == nil {
		if len(b) != KeyLen {
			return MasterKey{}, fmt.Errorf("%w: %s: hex-encoded key is %d bytes, want %d", secrets.ErrInvalidBackend, source, len(b), KeyLen)
		}
		return MasterKeyFromBytes(b)
	}

	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if b, err := enc.DecodeString(raw); err == nil {
			if len(b) != KeyLen {
				return MasterKey{}, fmt.Errorf("%w: %s: base64-encoded key is %d bytes, want %d", secrets.ErrInvalidBackend, source, len(b), KeyLen)
			}
			return MasterKeyFromBytes(b)
		}
	}

	return MasterKey{}, fmt.Errorf("%w: %s: master key material is not valid hex or base64", secrets.ErrInvalidBackend, source)
}
