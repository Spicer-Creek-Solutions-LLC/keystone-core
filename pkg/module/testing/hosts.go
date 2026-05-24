// SPDX-License-Identifier: Apache-2.0

package moduletest

import (
	"os"

	"go.keystone-core.io/keystone-core/pkg/module/capability"
)

// defaultHosts are the capability hosts a test run wires when the
// caller does not override them:
//
//   - fs:    os-backed (the capability layer still enforces the
//     manifest's AllowedPaths/DeniedPaths/MaxFileSize, so a
//     write outside the manifest scope fails + is audited just
//     as in production).
//   - log:   discarded (module print() output is what tests see;
//     the log capability's effect is the audit trail).
//   - http / exec / secrets: nil -> fail closed + audited if a
//     module both requested and tries to use them. Injectable
//     record/replay test hosts are deferred (ROADMAP v1.x).
func defaultHosts() capability.Hosts {
	return capability.Hosts{
		FS:     osFSHost{},
		Logger: discardLogger{},
	}
}

// osFSHost performs the unscoped filesystem effect. The
// capability.FSRead / FSWrite scope (manifest globs + max size)
// gates every call before it reaches the host — the same contract
// production wiring relies on.
type osFSHost struct{}

func (osFSHost) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path) //nolint:gosec // G304: path scoped by the capability layer (manifest AllowedPaths)
}

func (osFSHost) WriteFile(path string, data []byte, perm uint32) error {
	return os.WriteFile(path, data, os.FileMode(perm)) //nolint:gosec // G306: perm derived from the manifest-scoped capability
}

func (osFSHost) Stat(path string) (int64, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return fi.Size(), nil
}

// discardLogger drops module log capability output during tests.
type discardLogger struct{}

func (discardLogger) Log(_, _ string, _ map[string]string) {}
