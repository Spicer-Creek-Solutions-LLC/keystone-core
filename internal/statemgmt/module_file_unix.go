//go:build unix

package statemgmt

import (
	"os"
	"syscall"
)

// getFileOwnership extracts UID and GID from file info on Unix systems
func getFileOwnership(info os.FileInfo) (uid, gid uint32, ok bool) {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return stat.Uid, stat.Gid, true
	}
	return 0, 0, false
}

// isOwnershipSupported returns true on Unix systems
func isOwnershipSupported() bool {
	return true
}

// isSymlinkFullySupported returns true on Unix systems
func isSymlinkFullySupported() bool {
	return true
}
