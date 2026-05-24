// SPDX-License-Identifier: Apache-2.0

package outbound

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// SignaturePrefix is the GitHub-compatible header value prefix per
// PROJECT-DETAILS §4.14: every signed payload is "sha256=<hex>".
// Exported so receivers can build / strip it without re-deriving the
// constant.
const SignaturePrefix = "sha256="

// Sign returns the HMAC-SHA256 signature of payload using secret,
// formatted as the GitHub-compatible "sha256=<hex>" string the
// task-13 [HTTPDispatcher] sets on the X-Keystone-Signature header.
// Pure CPU operation — never errors.
//
// An empty secret yields a deterministic 32-byte HMAC under the
// empty key. Callers decide whether to skip signing entirely when
// the secret is empty (HTTPDispatcher does: no signature header at
// all rather than a meaningless sha256= over an empty key).
func Sign(secret, payload []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write(payload)
	return SignaturePrefix + hex.EncodeToString(mac.Sum(nil))
}

// Verify reports whether signature is a valid HMAC-SHA256 of payload
// under secret. The signature must be in the SignaturePrefix form
// ("sha256=<hex>"). Any parse failure (missing prefix, bad hex,
// length mismatch with the SHA-256 digest size) returns false —
// receivers only care: valid or not. Comparison is constant-time via
// [hmac.Equal].
func Verify(secret []byte, signature string, payload []byte) bool {
	hexPart, ok := strings.CutPrefix(signature, SignaturePrefix)
	if !ok {
		return false
	}
	got, err := hex.DecodeString(hexPart)
	if err != nil {
		return false
	}
	if len(got) != sha256.Size {
		// hmac.Equal would also return false here, but checking
		// length first avoids the timing leak the equal-length
		// guard documents in the stdlib.
		return false
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write(payload)
	return hmac.Equal(got, mac.Sum(nil))
}
