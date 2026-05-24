// SPDX-License-Identifier: Apache-2.0

package file

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
)

// fileType classifies a stat result for compare against the
// declared state.
type fileType int

const (
	typeMissing fileType = iota
	typeRegular
	typeDirectory
	typeSymlink
	typeOther // device / pipe / socket — treated as "wrong type"
)

func (t fileType) String() string {
	switch t {
	case typeMissing:
		return "missing"
	case typeRegular:
		return "regular"
	case typeDirectory:
		return "directory"
	case typeSymlink:
		return "symlink"
	default:
		return "other"
	}
}

// meta is the inspected on-disk state of a path. ContentHash is the
// hex SHA-256 of the file's content; empty when Type != regular.
// SymlinkTarget is the link's destination; empty when Type !=
// symlink. UID/GID are -1 when the lookup failed (rare; usually
// only on broken filesystems).
type meta struct {
	Type          fileType
	Mode          uint32 // 12-bit Linux mode bits
	UID           int
	GID           int
	OwnerName     string // resolved username; "" on lookup failure
	GroupName     string
	ContentHash   string
	SymlinkTarget string
}

// readMeta inspects path. Uses lstat so symlinks register as
// symlinks (not as their targets). Missing → typeMissing without
// error.
func readMeta(path string) (*meta, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return &meta{Type: typeMissing}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("lstat %s: %w", path, err)
	}
	m := &meta{Mode: uint32(info.Mode().Perm())}
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		m.Type = typeSymlink
		target, err := os.Readlink(path)
		if err != nil {
			return nil, fmt.Errorf("readlink %s: %w", path, err)
		}
		m.SymlinkTarget = target
	case info.IsDir():
		m.Type = typeDirectory
	case info.Mode().IsRegular():
		m.Type = typeRegular
		hash, err := hashFile(path)
		if err != nil {
			return nil, err
		}
		m.ContentHash = hash
	default:
		m.Type = typeOther
	}

	// uid/gid via syscall.Stat_t lives in fs_unix.go (Linux + Darwin
	// + BSD). The Windows shim (fs_windows.go) sets UID/GID = -1.
	fillOwnership(m, info)
	return m, nil
}

// hashFile returns the hex SHA-256 of path's content.
func hashFile(path string) (string, error) {
	f, err := os.Open(path) //nolint:gosec // operator-managed path; CLI input boundary
	if err != nil {
		return "", fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("hash %s: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// hashBytes returns the hex SHA-256 of b. Used for comparing
// declared content against on-disk content without writing first.
func hashBytes(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// writeFileAtomic writes content to path via the
// write-temp-then-rename idiom. Mode is applied via Chmod after the
// rename (most filesystems honour the umask on Create even when
// passed perm, so we chmod explicitly).
func writeFileAtomic(path string, content []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	var randBytes [8]byte
	if _, err := rand.Read(randBytes[:]); err != nil {
		return fmt.Errorf("random suffix: %w", err)
	}
	tmpPath := filepath.Join(dir, fmt.Sprintf(".%s.tmp.%x", filepath.Base(path), randBytes))

	f, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode) //nolint:gosec // tmpPath under operator-supplied dir
	if err != nil {
		return fmt.Errorf("open tmp: %w", err)
	}
	if _, err := f.Write(content); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write tmp: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("fsync tmp: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close tmp: %w", err)
	}
	if err := os.Chmod(tmpPath, mode); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("chmod tmp: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename tmp → %s: %w", path, err)
	}
	return nil
}

// resolveOwner converts a username or uid string to an integer uid.
// Numeric strings bypass user.Lookup so unknown-name doesn't block
// numeric-uid declarations.
func resolveOwner(s string) (int, error) {
	if n, err := strconv.Atoi(s); err == nil {
		return n, nil
	}
	u, err := user.Lookup(s)
	if err != nil {
		return 0, fmt.Errorf("user %q: %w", s, err)
	}
	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return 0, fmt.Errorf("user %q uid: %w", s, err)
	}
	return uid, nil
}

// resolveGroup mirrors resolveOwner for groups.
func resolveGroup(s string) (int, error) {
	if n, err := strconv.Atoi(s); err == nil {
		return n, nil
	}
	g, err := user.LookupGroup(s)
	if err != nil {
		return 0, fmt.Errorf("group %q: %w", s, err)
	}
	gid, err := strconv.Atoi(g.Gid)
	if err != nil {
		return 0, fmt.Errorf("group %q gid: %w", s, err)
	}
	return gid, nil
}

// ownerMatches reports whether the declared owner string (username
// or numeric uid) matches the meta's UID/OwnerName.
func ownerMatches(declared string, m *meta) bool {
	if declared == "" {
		return true // no owner declared → no constraint
	}
	if n, err := strconv.Atoi(declared); err == nil {
		return m.UID == n
	}
	return m.OwnerName == declared
}

func groupMatches(declared string, m *meta) bool {
	if declared == "" {
		return true
	}
	if n, err := strconv.Atoi(declared); err == nil {
		return m.GID == n
	}
	return m.GroupName == declared
}
