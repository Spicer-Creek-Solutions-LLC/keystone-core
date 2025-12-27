//go:build windows

package statemgmt

import (
	"os"
)

// getFileOwnership is a stub on Windows - Unix-style ownership is not supported
func getFileOwnership(info os.FileInfo) (uid, gid uint32, ok bool) {
	// Windows doesn't use Unix-style UID/GID
	return 0, 0, false
}

// isOwnershipSupported returns false on Windows
func isOwnershipSupported() bool {
	return false
}

// isSymlinkFullySupported returns false on Windows
// Symlinks on Windows require elevated privileges and behave differently
func isSymlinkFullySupported() bool {
	return false
}
