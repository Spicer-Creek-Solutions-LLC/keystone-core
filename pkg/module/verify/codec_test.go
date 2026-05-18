package verify_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"errors"
	"testing"

	"go.keystone-core.io/keystone-core/pkg/module/verify"
)

func TestSignatureCodec_RoundTrip(t *testing.T) {
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sig, err := verify.Sign([]byte("the module zip"), k)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	b, err := verify.MarshalSignature(sig)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got, err := verify.UnmarshalSignature(b)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.KeyID != sig.KeyID || got.Algorithm != sig.Algorithm ||
		string(got.Value) != string(sig.Value) {
		t.Fatalf("round-trip mismatch: %+v vs %+v", got, sig)
	}
	// The decoded signature still verifies.
	tp := verify.NewTrustPolicy()
	if _, err := tp.AddKey(k.Public()); err != nil {
		t.Fatal(err)
	}
	if err := verify.NewVerifier(tp).Verify([]byte("the module zip"), got); err != nil {
		t.Fatalf("decoded sig fails verify: %v", err)
	}
}

func TestSignatureCodec_Errors(t *testing.T) {
	if _, err := verify.UnmarshalSignature([]byte("not json")); err == nil {
		t.Fatal("garbage: want error")
	}
	for _, partial := range []string{
		`{}`,
		`{"key_id":"x"}`,
		`{"key_id":"x","algorithm":"ecdsa-sha256"}`, // no value
	} {
		if _, err := verify.UnmarshalSignature([]byte(partial)); !errors.Is(err, verify.ErrInvalidKey) {
			t.Fatalf("incomplete %q = %v, want ErrInvalidKey", partial, err)
		}
	}
}
