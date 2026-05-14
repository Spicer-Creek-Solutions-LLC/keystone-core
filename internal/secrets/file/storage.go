package file

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"go.keystone-core.io/keystone-core/internal/secrets"
)

// On-disk envelope (all big-endian where multi-byte numerics apply):
//
//	[16] magic            — "KSCORE-SECRETS\0\0" (constant)
//	[1]  format version   — 0x01 in v1.0
//	[8]  key fingerprint  — sha256(master)[:8]
//	[12] nonce            — fresh crypto/rand per write
//	[4]  ciphertext length (u32)
//	[N]  ciphertext       — AES-256-GCM output (includes 16-byte tag)
//
// Total fixed header = 16 + 1 + 8 + 12 + 4 = 41 bytes; payload is
// ciphertext.
const (
	magicLen      = 16
	versionLen    = 1
	keyIDLen      = FingerprintLen
	nonceLen      = 12
	lenFieldLen   = 4
	envelopeFixed = magicLen + versionLen + keyIDLen + nonceLen + lenFieldLen

	formatVersion byte = 0x01
)

// FileMagic is the literal 16-byte magic written into every encrypted
// state file. Constant so test fixtures and forensics tools can find
// the envelope start.
var FileMagic = [magicLen]byte{
	'K', 'S', 'C', 'O', 'R', 'E', '-', 'S', 'E', 'C', 'R', 'E', 'T', 'S', 0x00, 0x00,
}

// errBadEnvelope wraps every framing rejection so call sites match
// the family root [secrets.ErrInvalidBackend] AND can branch on the
// envelope-specific cause via [errors.Is] against the more specific
// sentinels below.
var (
	errEnvelopeShort         = errors.New("file: envelope is too short")
	errEnvelopeMagic         = errors.New("file: envelope magic mismatch (not a KSCORE-SECRETS file)")
	errEnvelopeVersion       = errors.New("file: envelope format version is unsupported")
	errEnvelopeKeyMismatch   = errors.New("file: master key fingerprint mismatch (wrong key or rotated key)")
	errEnvelopeLenOverflow   = errors.New("file: envelope ciphertext length overflows the available bytes")
	errEnvelopeAuthFailed    = errors.New("file: decryption failed (corrupted or tampered ciphertext)")
)

// encode produces the on-disk framed envelope for plaintext using
// the supplied master key. A fresh nonce is generated per call —
// reusing a nonce with the same key under AES-GCM is catastrophic, so
// the generator MUST be the package-level crypto/rand.
func encode(plaintext []byte, key MasterKey) ([]byte, error) {
	block, err := aes.NewCipher(key.Bytes())
	if err != nil {
		return nil, fmt.Errorf("%w: aes cipher: %v", secrets.ErrInvalidBackend, err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("%w: aes-gcm: %v", secrets.ErrInvalidBackend, err)
	}
	if aead.NonceSize() != nonceLen {
		// Belt and braces — Go's stdlib aes-gcm always returns 12.
		return nil, fmt.Errorf("%w: aes-gcm nonce size = %d, want %d", secrets.ErrInvalidBackend, aead.NonceSize(), nonceLen)
	}

	var nonce [nonceLen]byte
	if _, err := io.ReadFull(rand.Reader, nonce[:]); err != nil {
		return nil, fmt.Errorf("%w: nonce: %v", secrets.ErrInvalidBackend, err)
	}

	ciphertext := aead.Seal(nil, nonce[:], plaintext, nil)

	out := make([]byte, 0, envelopeFixed+len(ciphertext))
	out = append(out, FileMagic[:]...)
	out = append(out, formatVersion)
	fp := key.FingerprintBytes()
	out = append(out, fp[:]...)
	out = append(out, nonce[:]...)
	var lenBuf [lenFieldLen]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(ciphertext)))
	out = append(out, lenBuf[:]...)
	out = append(out, ciphertext...)
	return out, nil
}

// decode verifies the framed envelope and returns the plaintext.
// All errors wrap [secrets.ErrInvalidBackend] plus a more specific
// sentinel for envelope-layer diagnostics.
func decode(framed []byte, key MasterKey) ([]byte, error) {
	if len(framed) < envelopeFixed {
		return nil, fmt.Errorf("%w: %w (have %d bytes, need at least %d)", secrets.ErrInvalidBackend, errEnvelopeShort, len(framed), envelopeFixed)
	}
	offset := 0

	var magic [magicLen]byte
	copy(magic[:], framed[offset:offset+magicLen])
	offset += magicLen
	if magic != FileMagic {
		return nil, fmt.Errorf("%w: %w", secrets.ErrInvalidBackend, errEnvelopeMagic)
	}

	version := framed[offset]
	offset += versionLen
	if version != formatVersion {
		return nil, fmt.Errorf("%w: %w (have 0x%02x, want 0x%02x)", secrets.ErrInvalidBackend, errEnvelopeVersion, version, formatVersion)
	}

	var fileKeyID [keyIDLen]byte
	copy(fileKeyID[:], framed[offset:offset+keyIDLen])
	offset += keyIDLen
	expected := key.FingerprintBytes()
	if fileKeyID != expected {
		return nil, fmt.Errorf("%w: %w (file fp=%x, key fp=%s)", secrets.ErrInvalidBackend, errEnvelopeKeyMismatch, fileKeyID, key.Fingerprint())
	}

	nonce := framed[offset : offset+nonceLen]
	offset += nonceLen

	ctLen := binary.BigEndian.Uint32(framed[offset : offset+lenFieldLen])
	offset += lenFieldLen
	remaining := uint32(len(framed) - offset)
	if ctLen != remaining {
		return nil, fmt.Errorf("%w: %w (header says %d, %d remain)", secrets.ErrInvalidBackend, errEnvelopeLenOverflow, ctLen, remaining)
	}
	ciphertext := framed[offset:]

	block, err := aes.NewCipher(key.Bytes())
	if err != nil {
		return nil, fmt.Errorf("%w: aes cipher: %v", secrets.ErrInvalidBackend, err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("%w: aes-gcm: %v", secrets.ErrInvalidBackend, err)
	}
	plain, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %w: %v", secrets.ErrInvalidBackend, errEnvelopeAuthFailed, err)
	}
	return plain, nil
}

// tempSuffix is the suffix appended to the canonical path when
// staging an atomic write. Exported so the backend's startup
// recovery can clean up leftovers.
const tempSuffix = ".tmp"

// writeAtomic stages content to `<path>.tmp` (0600 perms), fsync,
// close, then `os.Rename` to path. The rename is atomic on POSIX so a
// reader sees either the old content or the new — never a partial
// write. On any error before rename, the tmp file is removed.
func writeAtomic(path string, content []byte) error {
	tmp := path + tempSuffix
	f, err := os.OpenFile(tmp, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600) // #nosec G304 -- operator-supplied state-file path from secrets.backends config
	if err != nil {
		return fmt.Errorf("%w: open tmp %q: %v", secrets.ErrInvalidBackend, tmp, err)
	}
	cleanup := func() { _ = os.Remove(tmp) }

	if _, err := f.Write(content); err != nil {
		_ = f.Close()
		cleanup()
		return fmt.Errorf("%w: write tmp %q: %v", secrets.ErrInvalidBackend, tmp, err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		cleanup()
		return fmt.Errorf("%w: fsync tmp %q: %v", secrets.ErrInvalidBackend, tmp, err)
	}
	if err := f.Close(); err != nil {
		cleanup()
		return fmt.Errorf("%w: close tmp %q: %v", secrets.ErrInvalidBackend, tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		cleanup()
		return fmt.Errorf("%w: rename %q -> %q: %v", secrets.ErrInvalidBackend, tmp, path, err)
	}
	return nil
}

// cleanupStaleTemp removes a leftover `<path>.tmp` if one exists.
// Invoked by [Backend.Start] before reading the canonical path —
// any tmp file is a crashed write that was never durable, so dropping
// it is safe (the canonical path either has the previous good
// content or doesn't exist yet).
//
// Returns whether a stale tmp was found + an error if removal failed.
func cleanupStaleTemp(path string) (found bool, err error) {
	tmp := path + tempSuffix
	if _, statErr := os.Stat(tmp); statErr != nil {
		if errors.Is(statErr, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("%w: stat tmp %q: %v", secrets.ErrInvalidBackend, tmp, statErr)
	}
	if rmErr := os.Remove(tmp); rmErr != nil {
		return true, fmt.Errorf("%w: remove stale tmp %q: %v", secrets.ErrInvalidBackend, tmp, rmErr)
	}
	return true, nil
}

// ensureParentDir is a small convenience used by tests + the backend
// to be defensive about the parent directory existing. Operators are
// responsible for creating the directory in production deployments,
// but the test surface benefits from an idempotent helper.
func ensureParentDir(path string) error {
	dir := filepath.Dir(path)
	if dir == "" || dir == "." {
		return nil
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("%w: mkdir parent %q: %v", secrets.ErrInvalidBackend, dir, err)
	}
	return nil
}
