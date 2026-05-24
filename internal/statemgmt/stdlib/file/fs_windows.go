// SPDX-License-Identifier: Apache-2.0

//go:build windows

package file

import "os"

// fillOwnership is a no-op on Windows. The state-stdlib `file`
// module is Linux/Unix-only by design (Windows agent is post-v1.0
// per FEATURES.md); the package still compiles on Windows so
// operator-side `kscorectl` builds cleanly cross-platform.
func fillOwnership(m *meta, _ os.FileInfo) {
	m.UID = -1
	m.GID = -1
}
