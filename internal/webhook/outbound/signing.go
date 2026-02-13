package outbound

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

const signaturePrefix = "sha256="

// Sign computes an HMAC-SHA256 signature over payload using secret and returns
// the result in GitHub-compatible format: "sha256=<hex>".
func Sign(secret, payload []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write(payload)
	return signaturePrefix + hex.EncodeToString(mac.Sum(nil))
}

// Verify checks whether signature matches the HMAC-SHA256 of payload under secret.
// The signature must be in "sha256=<hex>" format.
func Verify(secret, payload []byte, signature string) bool {
	if !strings.HasPrefix(signature, signaturePrefix) {
		return false
	}
	sigBytes, err := hex.DecodeString(strings.TrimPrefix(signature, signaturePrefix))
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, secret)
	mac.Write(payload)
	expected := mac.Sum(nil)
	return hmac.Equal(sigBytes, expected)
}

// SignatureHeader is the HTTP header name for HMAC signatures.
const SignatureHeader = "X-Signature-256"

// WebhookIDHeader is the HTTP header name for delivery IDs.
const WebhookIDHeader = "X-Webhook-ID"

// WebhookTimestampHeader is the HTTP header name for delivery timestamps.
const WebhookTimestampHeader = "X-Webhook-Timestamp"

// FormatTimestamp formats a Unix timestamp for the webhook timestamp header.
func FormatTimestamp(unix int64) string {
	return fmt.Sprintf("%d", unix)
}
