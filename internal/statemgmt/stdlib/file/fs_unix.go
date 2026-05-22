//go:build unix

package file

import (
	"os"
	"os/user"
	"strconv"
	"syscall"
)

// fillOwnership populates m.UID / m.GID / m.OwnerName / m.GroupName
// from the Unix stat result. Falls back to -1 when info.Sys() isn't
// a *syscall.Stat_t (unusual; defensive).
func fillOwnership(m *meta, info os.FileInfo) {
	if sys, ok := info.Sys().(*syscall.Stat_t); ok {
		m.UID = int(sys.Uid)
		m.GID = int(sys.Gid)
		if u, err := user.LookupId(strconv.Itoa(m.UID)); err == nil {
			m.OwnerName = u.Username
		}
		if g, err := user.LookupGroupId(strconv.Itoa(m.GID)); err == nil {
			m.GroupName = g.Name
		}
		return
	}
	m.UID = -1
	m.GID = -1
}
