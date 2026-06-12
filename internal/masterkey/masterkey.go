// SPDX-License-Identifier: Apache-2.0

package masterkey

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
)

// ErrInvalidKey wraps every rejection from this package — a bad source
// string, an unreadable key file, wrong-length material, an unknown
// scheme. Callers that need their own sentinel (e.g. the secrets file
// backend's ErrInvalidBackend) wrap this at their boundary.
var ErrInvalidKey = errors.New("masterkey: invalid master key")

// KeyLen is the AES-256 key size — 32 bytes.
const KeyLen = 32

// FingerprintLen is the prefix of `sha256(key)` callers write into an
// on-disk envelope's key-id field. 8 bytes balances "obvious mismatch
// on wrong key" against "no useful information about the key bytes."
const FingerprintLen = 8

// Key is a 32-byte AES-256 master key used to encrypt material at rest.
// Constructed via [Resolve] (operator config), [FromBytes], or
// [NewRandom] (tests). The zero value is invalid.
//
// Key is a value type — copy-by-value is fine and the key bytes are
// never exported. [Key.Fingerprint] returns a stable hex prefix derived
// from `sha256(key)` for diagnostic log lines; the key bytes themselves
// never appear in [Key.String] / Fingerprint.
type Key struct {
	key [KeyLen]byte
}

// Bytes returns a defensive copy of the key bytes. Callers feed this to
// `crypto/aes.NewCipher`; callers MUST zero the slice when done.
func (k Key) Bytes() []byte {
	out := make([]byte, KeyLen)
	copy(out, k.key[:])
	return out
}

// Fingerprint returns the hex of the first [FingerprintLen] bytes of
// `sha256(key)`. Written into an on-disk envelope and surfaced in
// operator logs so a wrong-key boot fails fast with a recognisable
// fingerprint mismatch instead of an opaque "decryption failed."
func (k Key) Fingerprint() string {
	sum := sha256.Sum256(k.key[:])
	return hex.EncodeToString(sum[:FingerprintLen])
}

// FingerprintBytes returns the binary fingerprint for use in an on-disk
// envelope. Same prefix as [Key.Fingerprint] — callers that write the
// fingerprint to bytes (storage layer) use this; callers logging it use
// the hex form.
func (k Key) FingerprintBytes() [FingerprintLen]byte {
	sum := sha256.Sum256(k.key[:])
	var out [FingerprintLen]byte
	copy(out[:], sum[:FingerprintLen])
	return out
}

// String is intentionally diagnostic-only — emits the fingerprint (not
// the key bytes) so accidental `%v` / `%s` formatting never leaks key
// material.
func (k Key) String() string {
	return fmt.Sprintf("master-key(fp=%s)", k.Fingerprint())
}

// IsZero reports whether the receiver is the uninitialised value.
func (k Key) IsZero() bool {
	return k.key == [KeyLen]byte{}
}

// NewRandom returns a freshly random key via `crypto/rand`. Test / dev
// only — operators use [Resolve] against a config-supplied source.
func NewRandom() (Key, error) {
	var k Key
	if _, err := rand.Read(k.key[:]); err != nil {
		return Key{}, fmt.Errorf("%w: random master key: %v", ErrInvalidKey, err)
	}
	return k, nil
}

// FromBytes wraps an already-derived 32-byte key. The internal [Resolve]
// machinery uses it; exposed for CLI and rotation tooling.
func FromBytes(b []byte) (Key, error) {
	if len(b) != KeyLen {
		return Key{}, fmt.Errorf("%w: master key must be %d bytes, got %d", ErrInvalidKey, KeyLen, len(b))
	}
	var k Key
	copy(k.key[:], b)
	return k, nil
}

// Resolve parses a scheme-prefixed source string from operator config
// and returns the resolved [Key].
//
// v1.0 schemes:
//
//   - `env:VAR_NAME` — read from the named env var (hex or base64).
//   - `file:/path/to/keyfile` — read from a file (binary 32 bytes, hex,
//     or base64). Trailing newlines are trimmed.
//   - `inline:<hex|base64>` — direct, intended for tests / dev. Callers
//     should log a WARN at boot when this scheme resolves so operators
//     don't ship `inline:` to production unnoticed.
//
// Cloud KMS schemes are detected and rejected with a v2.x+ pointer per
// FEATURES.md ("Cloud KMS for master keys — v2.x+"). Unknown schemes
// return [ErrInvalidKey] with the scheme name in the message.
//
// All errors wrap [ErrInvalidKey].
func Resolve(source string) (Key, error) {
	if source == "" {
		return Key{}, fmt.Errorf("%w: master key source is required", ErrInvalidKey)
	}

	idx := strings.IndexByte(source, ':')
	if idx <= 0 {
		return Key{}, fmt.Errorf("%w: master key source %q missing scheme (expected `env:`, `file:`, or `inline:`)", ErrInvalidKey, source)
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
		return Key{}, fmt.Errorf("%w: master key scheme %q is v2.x+ (cloud KMS); v1.0 supports env, file, inline schemes", ErrInvalidKey, scheme)
	default:
		return Key{}, fmt.Errorf("%w: unknown master key scheme %q (expected env:, file:, or inline:)", ErrInvalidKey, scheme)
	}
}

func resolveEnv(varName string) (Key, error) {
	if varName == "" {
		return Key{}, fmt.Errorf("%w: env: scheme requires a variable name", ErrInvalidKey)
	}
	raw, ok := os.LookupEnv(varName)
	if !ok {
		return Key{}, fmt.Errorf("%w: env var %q is not set", ErrInvalidKey, varName)
	}
	if raw == "" {
		return Key{}, fmt.Errorf("%w: env var %q is empty", ErrInvalidKey, varName)
	}
	return decodeKeyMaterial(raw, fmt.Sprintf("env:%s", varName))
}

func resolveFile(path string) (Key, error) {
	if path == "" {
		return Key{}, fmt.Errorf("%w: file: scheme requires a path", ErrInvalidKey)
	}
	raw, err := os.ReadFile(path) // #nosec G304 -- operator-supplied master key path from config
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Key{}, fmt.Errorf("%w: master key file %q does not exist", ErrInvalidKey, path)
		}
		return Key{}, fmt.Errorf("%w: read master key file %q: %v", ErrInvalidKey, path, err)
	}
	// Allow binary 32-byte files OR an encoded form with a trailing newline.
	if len(raw) == KeyLen {
		return FromBytes(raw)
	}
	trimmed := strings.TrimSpace(string(raw))
	return decodeKeyMaterial(trimmed, fmt.Sprintf("file:%s", path))
}

// decodeKeyMaterial accepts hex (64 chars) or base64 (44 chars for 32
// bytes, with or without padding) and returns a [Key]. Source describes
// where the material came from for error messages.
func decodeKeyMaterial(raw, source string) (Key, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Key{}, fmt.Errorf("%w: %s: master key material is empty", ErrInvalidKey, source)
	}

	if b, err := hex.DecodeString(raw); err == nil {
		if len(b) != KeyLen {
			return Key{}, fmt.Errorf("%w: %s: hex-encoded key is %d bytes, want %d", ErrInvalidKey, source, len(b), KeyLen)
		}
		return FromBytes(b)
	}

	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if b, err := enc.DecodeString(raw); err == nil {
			if len(b) != KeyLen {
				return Key{}, fmt.Errorf("%w: %s: base64-encoded key is %d bytes, want %d", ErrInvalidKey, source, len(b), KeyLen)
			}
			return FromBytes(b)
		}
	}

	return Key{}, fmt.Errorf("%w: %s: master key material is not valid hex or base64", ErrInvalidKey, source)
}
