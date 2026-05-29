// SPDX-License-Identifier: Apache-2.0

// kscore-backup is the Keystone Core backup + restore CLI
// (Epic 18 task 7). Task 7a ships the read-only verify + list
// subcommands; task 7b adds create + restore.
//
//	verify   read an artifact, validate manifest + per-component
//	         SHA-256s; reports OK or the integrity violation.
//	list     enumerate artifacts at a destination prefix.
package main

import (
	"fmt"
	"os"

	"go.keystone-core.io/keystone-core/internal/cli/backup"
)

func main() {
	if err := backup.NewBackupCommand().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
