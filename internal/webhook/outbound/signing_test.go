package outbound

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSign_Verify_Roundtrip(t *testing.T) {
	secret := []byte("test-secret")
	payload := []byte(`{"event":"agent.connect","id":"123"}`)

	sig := Sign(secret, payload)
	assert.True(t, Verify(secret, payload, sig))
}

func TestVerify_WrongSecret(t *testing.T) {
	payload := []byte(`{"event":"test"}`)
	sig := Sign([]byte("secret-a"), payload)
	assert.False(t, Verify([]byte("secret-b"), payload, sig))
}

func TestVerify_TamperedPayload(t *testing.T) {
	secret := []byte("secret")
	sig := Sign(secret, []byte("original"))
	assert.False(t, Verify(secret, []byte("tampered"), sig))
}

func TestVerify_InvalidFormat(t *testing.T) {
	secret := []byte("secret")
	payload := []byte("data")

	assert.False(t, Verify(secret, payload, "not-a-signature"))
	assert.False(t, Verify(secret, payload, "sha256=zzzz_invalid_hex"))
	assert.False(t, Verify(secret, payload, ""))
}

func TestSign_EmptySecret(t *testing.T) {
	sig := Sign([]byte(""), []byte("payload"))
	assert.Contains(t, sig, "sha256=")
	assert.True(t, Verify([]byte(""), []byte("payload"), sig))
}

func TestSign_Format(t *testing.T) {
	sig := Sign([]byte("key"), []byte("data"))
	assert.Regexp(t, `^sha256=[0-9a-f]{64}$`, sig)
}
