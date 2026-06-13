// SPDX-License-Identifier: Apache-2.0

package masterkey

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewRandom(t *testing.T) {
	t.Parallel()

	k, err := NewRandom()
	if err != nil {
		t.Fatalf("NewRandom: %v", err)
	}
	if k.IsZero() {
		t.Errorf("NewRandom returned zero value")
	}
	if len(k.Bytes()) != KeyLen {
		t.Errorf("Bytes() = %d, want %d", len(k.Bytes()), KeyLen)
	}
	if len(k.Fingerprint()) != FingerprintLen*2 {
		t.Errorf("Fingerprint() len = %d, want %d (hex of %d bytes)", len(k.Fingerprint()), FingerprintLen*2, FingerprintLen)
	}
	if !strings.HasPrefix(k.String(), "master-key(fp=") {
		t.Errorf("String() = %q, want master-key(fp=…)", k.String())
	}
}

func TestKey_NoLeak(t *testing.T) {
	t.Parallel()

	// Build a known-bytes key. Fingerprint + String MUST NOT expose
	// the raw bytes.
	keyBytes := make([]byte, KeyLen)
	for i := range keyBytes {
		keyBytes[i] = byte(i + 1)
	}
	k, err := FromBytes(keyBytes)
	if err != nil {
		t.Fatalf("FromBytes: %v", err)
	}

	formatted := k.String()
	rawHex := hex.EncodeToString(keyBytes)
	if strings.Contains(formatted, rawHex) {
		t.Errorf("String() leaked raw key hex: %q", formatted)
	}
	// First fingerprint bytes are derived; the raw bytes themselves
	// should not appear anywhere.
	for _, byteForm := range []string{string(keyBytes), rawHex} {
		if strings.Contains(formatted, byteForm) {
			t.Errorf("String() contains raw key form")
		}
	}
}

func TestFromBytes_WrongLength(t *testing.T) {
	t.Parallel()

	cases := []int{0, 16, 31, 33, 64}
	for _, n := range cases {
		_, err := FromBytes(make([]byte, n))
		if err == nil {
			t.Errorf("FromBytes(%d bytes) = nil err, want failure", n)
			continue
		}
		if !errors.Is(err, ErrInvalidKey) {
			t.Errorf("FromBytes(%d bytes) err does not wrap ErrInvalidKey: %v", n, err)
		}
	}
}

func TestKey_BytesDefensiveCopy(t *testing.T) {
	t.Parallel()
	k, _ := NewRandom()
	first := k.Bytes()
	first[0] ^= 0xff // mutate the returned slice
	second := k.Bytes()
	if second[0] == first[0] {
		t.Errorf("Bytes() did not return a defensive copy; mutation leaked")
	}
}

func TestKey_FingerprintDeterministic(t *testing.T) {
	t.Parallel()
	a, _ := FromBytes(make([]byte, KeyLen))
	b, _ := FromBytes(make([]byte, KeyLen))
	if a.Fingerprint() != b.Fingerprint() {
		t.Errorf("Fingerprint not deterministic across construction")
	}
	// FingerprintBytes shares the prefix with the hex form.
	fpb := a.FingerprintBytes()
	if hex.EncodeToString(fpb[:]) != a.Fingerprint() {
		t.Errorf("FingerprintBytes != hex(Fingerprint)")
	}
	// A different key MUST produce a different fingerprint.
	other := make([]byte, KeyLen)
	other[0] = 0xff
	c, _ := FromBytes(other)
	if a.Fingerprint() == c.Fingerprint() {
		t.Errorf("Fingerprint collision across distinct keys")
	}
}

func TestResolve_Schemes(t *testing.T) {
	t.Parallel()

	keyBytes := make([]byte, KeyLen)
	for i := range keyBytes {
		keyBytes[i] = byte(i)
	}
	hexKey := hex.EncodeToString(keyBytes)
	b64Key := base64.StdEncoding.EncodeToString(keyBytes)
	rawB64Key := base64.RawStdEncoding.EncodeToString(keyBytes)

	t.Run("inline hex", func(t *testing.T) {
		t.Parallel()
		k, err := Resolve("inline:" + hexKey)
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if k.IsZero() {
			t.Errorf("resolved key is zero")
		}
	})

	t.Run("inline base64 std", func(t *testing.T) {
		t.Parallel()
		k, err := Resolve("inline:" + b64Key)
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if k.IsZero() {
			t.Errorf("resolved key is zero")
		}
	})

	t.Run("inline base64 raw", func(t *testing.T) {
		t.Parallel()
		k, err := Resolve("inline:" + rawB64Key)
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if k.IsZero() {
			t.Errorf("resolved key is zero")
		}
	})

	t.Run("file scheme binary 32 bytes", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "key.bin")
		if err := os.WriteFile(path, keyBytes, 0600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		k, err := Resolve("file:" + path)
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if k.IsZero() {
			t.Errorf("resolved key is zero")
		}
	})

	t.Run("file scheme hex with trailing newline", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "key.hex")
		if err := os.WriteFile(path, []byte(hexKey+"\n"), 0600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		k, err := Resolve("file:" + path)
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if k.IsZero() {
			t.Errorf("resolved key is zero")
		}
	})
}

func TestResolve_Errors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		source  string
		wantSub string
	}{
		{"empty source", "", "source is required"},
		{"no scheme", "abcdef", "missing scheme"},
		{"unknown scheme", "kubernetes:foo", "unknown master key scheme"},
		{"gcp-kms is v2.x+", "gcp-kms:projects/x", "v2.x+"},
		{"aws-kms is v2.x+", "aws-kms:arn:aws:kms:foo", "v2.x+"},
		{"azure-kv is v2.x+", "azure-kv:vault.example", "v2.x+"},
		{"empty env var name", "env:", "variable name"},
		{"env var not set", "env:KSCORE_TEST_NEVER_SET_42", "is not set"},
		{"file scheme empty path", "file:", "requires a path"},
		{"file not exist", "file:/no/such/path/here-really", "does not exist"},
		{"inline empty", "inline:", "is empty"},
		{"inline bad", "inline:notvalid!@#", "not valid hex or base64"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := Resolve(tc.source)
			if err == nil {
				t.Fatalf("Resolve(%q) = nil err, want %q", tc.source, tc.wantSub)
			}
			if !errors.Is(err, ErrInvalidKey) {
				t.Errorf("err does not wrap ErrInvalidKey: %v", err)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("err = %q, want substring %q", err.Error(), tc.wantSub)
			}
		})
	}
}

func TestResolve_WrongLength(t *testing.T) {
	t.Parallel()

	short := hex.EncodeToString(make([]byte, 16))
	_, err := Resolve("inline:" + short)
	if err == nil {
		t.Fatalf("short key accepted")
	}
	if !errors.Is(err, ErrInvalidKey) {
		t.Errorf("err does not wrap ErrInvalidKey: %v", err)
	}
	if !strings.Contains(err.Error(), "16 bytes, want 32") {
		t.Errorf("err = %q, want length-mismatch message", err.Error())
	}
}

// TestResolve_EnvScheme is intentionally non-parallel — it uses
// t.Setenv, which forbids parallel ancestors.
func TestResolve_EnvScheme(t *testing.T) {
	const varName = "KSCORE_TEST_MASTER_KEY_OK"
	keyBytes := make([]byte, KeyLen)
	for i := range keyBytes {
		keyBytes[i] = byte(i)
	}
	hexKey := hex.EncodeToString(keyBytes)

	t.Setenv(varName, hexKey)
	k, err := Resolve("env:" + varName)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if k.IsZero() {
		t.Errorf("resolved key is zero")
	}
}

// TestResolve_EnvEmpty exercises the "env var set but empty" branch.
// Non-parallel for the same reason.
func TestResolve_EnvEmpty(t *testing.T) {
	const varName = "KSCORE_TEST_MASTER_KEY_EMPTY"
	t.Setenv(varName, "")
	_, err := Resolve("env:" + varName)
	if err == nil {
		t.Fatalf("empty env var accepted")
	}
	if !errors.Is(err, ErrInvalidKey) {
		t.Errorf("err does not wrap ErrInvalidKey: %v", err)
	}
	if !strings.Contains(err.Error(), "is empty") {
		t.Errorf("err = %q, want is-empty message", err.Error())
	}
}
