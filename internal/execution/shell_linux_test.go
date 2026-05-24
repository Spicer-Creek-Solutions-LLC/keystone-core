// SPDX-License-Identifier: Apache-2.0

//go:build linux

package execution

import "testing"

// Linux runners are guaranteed by the v1.0 platform target, so bash
// and sh must both resolve. powershell and cmd should not.
func TestShell_IsAvailable_Linux(t *testing.T) {
	t.Parallel()

	if !ShellSh.IsAvailable() {
		t.Error("ShellSh.IsAvailable() = false on linux; sh must be present")
	}
	if !ShellBash.IsAvailable() {
		t.Error("ShellBash.IsAvailable() = false on linux; bash must be present in v0.1 platform target")
	}
	if ShellCmd.IsAvailable() {
		t.Error("ShellCmd.IsAvailable() = true on linux; cmd is windows-only")
	}
}

func TestShell_DefaultIsBash_Linux(t *testing.T) {
	t.Parallel()

	if got := GetDefaultShell(); got != ShellBash {
		t.Errorf("GetDefaultShell() = %v on linux, want ShellBash", got)
	}
}
