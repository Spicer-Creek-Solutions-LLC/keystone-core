// SPDX-License-Identifier: Apache-2.0

// Package masterkey resolves and represents the 32-byte AES-256 master
// key used to encrypt material at rest. It is the single, neutral
// key-sourcing layer shared by every component that does
// encryption-at-rest:
//
//   - the secrets file backend (internal/secrets/file), which encrypts
//     the cleartext secrets state file, and
//   - the identity CA storage (internal/identity), which encrypts the
//     embedded provider's root + signing CA private keys.
//
// [Resolve] parses a scheme-prefixed operator-config source —
// `env:VAR`, `file:/path`, or `inline:<hex|base64>` — into a [Key].
// The key bytes never appear in logs: [Key.String] and
// [Key.Fingerprint] surface only a short `sha256` prefix, so a
// wrong-key boot fails fast with a recognisable fingerprint mismatch
// rather than an opaque "decryption failed."
//
// The package has no dependencies on its consumers; each wraps
// [ErrInvalidKey] in its own sentinel at the boundary.
package masterkey
