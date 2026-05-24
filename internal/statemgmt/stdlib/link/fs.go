// SPDX-License-Identifier: Apache-2.0

package link

import (
	"errors"
	"fmt"
	"os"
)

// pathKind classifies an lstat result.
type pathKind int

const (
	kindMissing pathKind = iota
	kindSymlink
	kindRegular
	kindDirectory
	kindOther // device / pipe / socket
)

func (k pathKind) String() string {
	switch k {
	case kindMissing:
		return "missing"
	case kindSymlink:
		return "symlink"
	case kindRegular:
		return "regular file"
	case kindDirectory:
		return "directory"
	default:
		return "non-regular file"
	}
}

// linkInfo is the inspected on-disk state of the link path.
// SymlinkTarget is populated only when Kind == kindSymlink.
type linkInfo struct {
	Kind          pathKind
	SymlinkTarget string
}

// inspect lstats path. A missing path is (kindMissing, nil) without
// error — that is a normal state, not a failure.
func inspect(path string) (*linkInfo, error) {
	fi, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return &linkInfo{Kind: kindMissing}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("lstat %s: %w", path, err)
	}
	li := &linkInfo{}
	switch {
	case fi.Mode()&os.ModeSymlink != 0:
		li.Kind = kindSymlink
		target, err := os.Readlink(path)
		if err != nil {
			return nil, fmt.Errorf("readlink %s: %w", path, err)
		}
		li.SymlinkTarget = target
	case fi.IsDir():
		li.Kind = kindDirectory
	case fi.Mode().IsRegular():
		li.Kind = kindRegular
	default:
		li.Kind = kindOther
	}
	return li, nil
}

// sameInode reports whether path and target resolve to the same
// underlying file (same device + inode) — i.e. they are hard links
// to each other. Both are lstat'd: a hard link is itself a regular
// file, so following symlinks is neither needed nor wanted (a
// symlink at path is "not a hard link", which is the answer we
// want). target must exist; a missing target is an error because a
// hard link cannot be validated or created against it.
func sameInode(path, target string) (bool, error) {
	ti, err := os.Lstat(target)
	if err != nil {
		return false, fmt.Errorf("lstat hard-link target %s: %w", target, err)
	}
	pi, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("lstat %s: %w", path, err)
	}
	return os.SameFile(pi, ti), nil
}

// removeForReplace removes whatever is at path so a fresh link can
// be created. Directories are never removed — that risks data loss
// and is out of this module's remit. Caller must have already
// decided a replacement is wanted (Force).
func removeForReplace(path string, li *linkInfo) error {
	switch li.Kind {
	case kindMissing:
		return nil
	case kindDirectory:
		return fmt.Errorf("refusing to replace directory %s with a link", path)
	default:
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("remove %s: %w", path, err)
		}
		return nil
	}
}
