// Package cas is the content-addressed module store — the SHA-256
// hashing + CAS storage half of the Epic 14 verification pipeline
// (PROJECT-DETAILS §4.18). The signature half is pkg/module/verify;
// the size/age/readonly cache policy on top of this store is task 7.
//
// Hashes use the `sha256:<64 hex>` form — the exact
// manifest.LockedModule.Hash format, so a stored blob's hash drops
// straight into a lockfile and back.
//
// Pure standard library; no new dependency.
package cas

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	// ErrInvalidHash — a hash string is not `sha256:<64 lowercase hex>`.
	ErrInvalidHash = errors.New("cas: invalid hash")
	// ErrHashMismatch — content did not hash to the expected value.
	ErrHashMismatch = errors.New("cas: content hash mismatch")
	// ErrNotFound — no stored content for the hash.
	ErrNotFound = errors.New("cas: content not found")
	// ErrCorrupted — stored content no longer hashes to its address.
	ErrCorrupted = errors.New("cas: stored content corrupted")
)

const hashPrefix = "sha256:"

var hexRE = regexp.MustCompile(`^[0-9a-f]{64}$`)

// HashBytes returns the `sha256:<hex>` content hash of b.
func HashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hashPrefix + hex.EncodeToString(sum[:])
}

// Hash returns the `sha256:<hex>` content hash of everything read
// from r.
func Hash(r io.Reader) (string, error) {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", fmt.Errorf("cas: hash: %w", err)
	}
	return hashPrefix + hex.EncodeToString(h.Sum(nil)), nil
}

// FormatHash wraps a bare 64-char lowercase hex digest as
// `sha256:<hex>`.
func FormatHash(hexDigest string) (string, error) {
	if !hexRE.MatchString(hexDigest) {
		return "", fmt.Errorf("%w: %q", ErrInvalidHash, hexDigest)
	}
	return hashPrefix + hexDigest, nil
}

// ParseHash validates a `sha256:<hex>` string and returns the bare
// hex digest.
func ParseHash(h string) (string, error) {
	d, ok := strings.CutPrefix(h, hashPrefix)
	if !ok || !hexRE.MatchString(d) {
		return "", fmt.Errorf("%w: %q", ErrInvalidHash, h)
	}
	return d, nil
}

// Store is a content-addressed filesystem store. Layout follows
// §4.18: <root>/<hex>/content, written atomically (temp + rename)
// so concurrent Puts of identical content cannot tear.
type Store struct {
	root string
}

// DefaultRoot is <user-home>/.kscore/modules, falling back to
// ./.kscore/modules when the home dir is unresolvable (the
// internal/config/identity overridable-default precedent).
func DefaultRoot() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".kscore", "modules")
	}
	return filepath.Join(home, ".kscore", "modules")
}

// New returns a Store rooted at root (empty → DefaultRoot). The
// root directory is created if absent.
func New(root string) (*Store, error) {
	if root == "" {
		root = DefaultRoot()
	}
	if err := os.MkdirAll(root, 0o750); err != nil {
		return nil, fmt.Errorf("cas: create root: %w", err)
	}
	return &Store{root: root}, nil
}

func (s *Store) dir(hexDigest string) string  { return filepath.Join(s.root, hexDigest) }
func (s *Store) blob(hexDigest string) string { return filepath.Join(s.dir(hexDigest), "content") }

// Path returns the on-disk path of the content for hash (whether or
// not it exists). Errors only on a malformed hash.
func (s *Store) Path(hash string) (string, error) {
	d, err := ParseHash(hash)
	if err != nil {
		return "", err
	}
	return s.blob(d), nil
}

// Has reports whether content for hash is stored.
func (s *Store) Has(hash string) bool {
	p, err := s.Path(hash)
	if err != nil {
		return false
	}
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular()
}

// Put streams r into the store, addressing it by its SHA-256.
// Idempotent: identical content already present is not rewritten.
func (s *Store) Put(r io.Reader) (string, error) {
	return s.put(r, "")
}

// PutExpected is Put with an integrity gate: if the content does
// not hash to want, nothing is committed and ErrHashMismatch is
// returned (the loader's "downloaded ZIP matches the lockfile"
// check).
func (s *Store) PutExpected(r io.Reader, want string) (string, error) {
	if _, err := ParseHash(want); err != nil {
		return "", err
	}
	return s.put(r, want)
}

func (s *Store) put(r io.Reader, want string) (string, error) {
	tmp, err := os.CreateTemp(s.root, ".put-*")
	if err != nil {
		return "", fmt.Errorf("cas: temp: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }

	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, h), r); err != nil {
		_ = tmp.Close()
		cleanup()
		return "", fmt.Errorf("cas: write: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return "", fmt.Errorf("cas: sync: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return "", fmt.Errorf("cas: close: %w", err)
	}

	hexDigest := hex.EncodeToString(h.Sum(nil))
	hash := hashPrefix + hexDigest
	if want != "" && hash != want {
		cleanup()
		return "", fmt.Errorf("%w: got %s want %s", ErrHashMismatch, hash, want)
	}

	if s.Has(hash) { // idempotent — content already present
		cleanup()
		return hash, nil
	}
	if err := os.MkdirAll(s.dir(hexDigest), 0o750); err != nil {
		cleanup()
		return "", fmt.Errorf("cas: mkdir: %w", err)
	}
	if err := os.Rename(tmpName, s.blob(hexDigest)); err != nil {
		// A racing Put may have created it first — that's fine.
		if s.Has(hash) {
			cleanup()
			return hash, nil
		}
		cleanup()
		return "", fmt.Errorf("cas: commit: %w", err)
	}
	return hash, nil
}

// Get opens the stored content for hash. ErrNotFound if absent.
func (s *Store) Get(hash string) (io.ReadCloser, error) {
	p, err := s.Path(hash)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(p) //nolint:gosec // p derives from a validated content hash under root
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, hash)
		}
		return nil, err
	}
	return f, nil
}

// Verify re-hashes the stored content and confirms it still matches
// its address. ErrNotFound if absent, ErrCorrupted on mismatch.
func (s *Store) Verify(hash string) error {
	rc, err := s.Get(hash)
	if err != nil {
		return err
	}
	defer func() { _ = rc.Close() }()
	got, err := Hash(rc)
	if err != nil {
		return err
	}
	if got != hash {
		return fmt.Errorf("%w: %s now hashes to %s", ErrCorrupted, hash, got)
	}
	return nil
}

// Delete removes the content for hash (idempotent).
func (s *Store) Delete(hash string) error {
	d, err := ParseHash(hash)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(s.dir(d)); err != nil {
		return fmt.Errorf("cas: delete: %w", err)
	}
	return nil
}
