// SPDX-License-Identifier: Apache-2.0

package file

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.keystone-core.io/keystone-core/internal/secrets"
)

func TestEncodeDecode_RoundTrip(t *testing.T) {
	t.Parallel()

	key, _ := NewRandomMasterKey()
	plaintext := []byte(`{"version":1,"secrets":{"kv/foo":{"data":{"password":"hunter2"}}}}`)

	framed, err := encode(plaintext, key)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if !bytes.HasPrefix(framed, FileMagic[:]) {
		t.Errorf("framed envelope missing magic prefix")
	}
	if framed[magicLen] != formatVersion {
		t.Errorf("format version byte = 0x%02x, want 0x%02x", framed[magicLen], formatVersion)
	}

	out, err := decode(framed, key)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !bytes.Equal(out, plaintext) {
		t.Errorf("decoded plaintext mismatch:\n got: %s\nwant: %s", out, plaintext)
	}
}

func TestEncode_NonceUniqueness(t *testing.T) {
	t.Parallel()

	key, _ := NewRandomMasterKey()
	plaintext := []byte("same plaintext, twice")

	a, err := encode(plaintext, key)
	if err != nil {
		t.Fatalf("encode a: %v", err)
	}
	b, err := encode(plaintext, key)
	if err != nil {
		t.Fatalf("encode b: %v", err)
	}
	if bytes.Equal(a, b) {
		t.Fatalf("identical encodes — nonce was reused (this is a security bug)")
	}
	// Nonces sit at envelopeFixed - (lenFieldLen + nonceLen) ... so:
	const nonceStart = magicLen + versionLen + keyIDLen
	if bytes.Equal(a[nonceStart:nonceStart+nonceLen], b[nonceStart:nonceStart+nonceLen]) {
		t.Fatalf("nonces matched across two encodes")
	}
}

func TestDecode_RejectsShortInput(t *testing.T) {
	t.Parallel()

	key, _ := NewRandomMasterKey()
	_, err := decode([]byte("too short"), key)
	if err == nil {
		t.Fatalf("decode short input = nil err")
	}
	if !errors.Is(err, secrets.ErrInvalidBackend) {
		t.Errorf("err does not wrap ErrInvalidBackend: %v", err)
	}
	if !errors.Is(err, errEnvelopeShort) {
		t.Errorf("err does not wrap errEnvelopeShort: %v", err)
	}
}

func TestDecode_RejectsWrongMagic(t *testing.T) {
	t.Parallel()

	key, _ := NewRandomMasterKey()
	framed, err := encode([]byte("hi"), key)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	// Corrupt magic.
	framed[0] = 'X'

	_, err = decode(framed, key)
	if !errors.Is(err, errEnvelopeMagic) {
		t.Errorf("err does not wrap errEnvelopeMagic: %v", err)
	}
}

func TestDecode_RejectsUnknownVersion(t *testing.T) {
	t.Parallel()

	key, _ := NewRandomMasterKey()
	framed, err := encode([]byte("hi"), key)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	framed[magicLen] = 0x99

	_, err = decode(framed, key)
	if !errors.Is(err, errEnvelopeVersion) {
		t.Errorf("err does not wrap errEnvelopeVersion: %v", err)
	}
}

func TestDecode_RejectsWrongKey(t *testing.T) {
	t.Parallel()

	key1, _ := NewRandomMasterKey()
	key2, _ := NewRandomMasterKey()
	framed, err := encode([]byte("hi"), key1)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	_, err = decode(framed, key2)
	if !errors.Is(err, errEnvelopeKeyMismatch) {
		t.Errorf("err does not wrap errEnvelopeKeyMismatch: %v", err)
	}
}

func TestDecode_RejectsTamperedCiphertext(t *testing.T) {
	t.Parallel()

	key, _ := NewRandomMasterKey()
	framed, err := encode([]byte("hello"), key)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	// Flip a bit in the ciphertext (after the 41-byte header).
	framed[envelopeFixed+1] ^= 0x01

	_, err = decode(framed, key)
	if !errors.Is(err, errEnvelopeAuthFailed) {
		t.Errorf("err does not wrap errEnvelopeAuthFailed: %v", err)
	}
}

func TestDecode_RejectsLengthOverflow(t *testing.T) {
	t.Parallel()

	key, _ := NewRandomMasterKey()
	framed, err := encode([]byte("hi"), key)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	// Truncate so the declared length exceeds remaining bytes.
	framed = framed[:len(framed)-1]
	_, err = decode(framed, key)
	if !errors.Is(err, errEnvelopeLenOverflow) {
		t.Errorf("err does not wrap errEnvelopeLenOverflow: %v", err)
	}
}

func TestWriteAtomic_RoundTrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "state.bin")

	content := []byte("hello atomic world")
	if err := writeAtomic(path, content); err != nil {
		t.Fatalf("writeAtomic: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("readback mismatch: got %q want %q", got, content)
	}

	// tmp file should be gone post-rename.
	if _, err := os.Stat(path + tempSuffix); !errors.Is(err, os.ErrNotExist) {
		t.Errorf(".tmp file still exists post-rename: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("permissions = %o, want 0600", perm)
	}
}

func TestWriteAtomic_FailureCleansUp(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// Make the target directory unwritable so rename fails. We need
	// to first create the tmp successfully, then have rename fail.
	// Easier: make the path a *directory* — open will succeed via
	// O_CREATE on the .tmp suffix but the rename will fail because
	// the destination is a non-empty dir.
	path := filepath.Join(dir, "state.bin")
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	// Drop something inside so non-empty-dir rename rejects.
	if err := os.WriteFile(filepath.Join(path, "anchor"), []byte("x"), 0600); err != nil {
		t.Fatalf("WriteFile anchor: %v", err)
	}

	err := writeAtomic(path, []byte("hi"))
	if err == nil {
		t.Fatalf("writeAtomic over a non-empty dir = nil err")
	}
	if !errors.Is(err, secrets.ErrInvalidBackend) {
		t.Errorf("err does not wrap ErrInvalidBackend: %v", err)
	}
	// tmp file should have been removed on failure.
	if _, statErr := os.Stat(path + tempSuffix); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf(".tmp not cleaned up after failure: stat err = %v", statErr)
	}
}

func TestCleanupStaleTemp(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "state.bin")

	// No tmp present: returns (false, nil).
	found, err := cleanupStaleTemp(path)
	if err != nil {
		t.Fatalf("cleanupStaleTemp on missing: %v", err)
	}
	if found {
		t.Errorf("found = true when no tmp existed")
	}

	// Create a stale tmp; cleanup should remove it.
	tmp := path + tempSuffix
	if err := os.WriteFile(tmp, []byte("interrupted"), 0600); err != nil {
		t.Fatalf("WriteFile tmp: %v", err)
	}
	found, err = cleanupStaleTemp(path)
	if err != nil {
		t.Fatalf("cleanupStaleTemp on stale: %v", err)
	}
	if !found {
		t.Errorf("found = false when stale tmp existed")
	}
	if _, statErr := os.Stat(tmp); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("stale tmp not removed: stat err = %v", statErr)
	}
}

func TestEnsureParentDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	nested := filepath.Join(dir, "a", "b", "c", "state.bin")
	if err := ensureParentDir(nested); err != nil {
		t.Fatalf("ensureParentDir: %v", err)
	}
	info, err := os.Stat(filepath.Dir(nested))
	if err != nil {
		t.Fatalf("Stat after ensureParentDir: %v", err)
	}
	if !info.IsDir() {
		t.Errorf("parent is not a directory")
	}

	// Idempotent.
	if err := ensureParentDir(nested); err != nil {
		t.Errorf("ensureParentDir second call: %v", err)
	}

	// "." and empty are no-ops.
	if err := ensureParentDir("state.bin"); err != nil {
		t.Errorf("ensureParentDir(\"state.bin\"): %v", err)
	}
}

func TestEncode_DistinctNoncesAcrossManyEncrypts(t *testing.T) {
	t.Parallel()
	// Defense against PRNG starvation / accidental key reuse.
	const n = 200
	key, _ := NewRandomMasterKey()
	seen := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		framed, err := encode([]byte("payload"), key)
		if err != nil {
			t.Fatalf("encode[%d]: %v", i, err)
		}
		const nonceStart = magicLen + versionLen + keyIDLen
		nonce := string(framed[nonceStart : nonceStart+nonceLen])
		if _, dup := seen[nonce]; dup {
			t.Fatalf("nonce collision after %d encrypts (catastrophic for AES-GCM)", i)
		}
		seen[nonce] = struct{}{}
	}
}

// Sanity check: error messages include enough context for operators.
func TestEnvelopeErrors_ContainContext(t *testing.T) {
	t.Parallel()

	key1, _ := NewRandomMasterKey()
	key2, _ := NewRandomMasterKey()
	framed, _ := encode([]byte("hi"), key1)
	_, err := decode(framed, key2)
	if !strings.Contains(err.Error(), "fingerprint mismatch") {
		t.Errorf("wrong-key err = %q, want fingerprint mismatch context", err.Error())
	}
}
