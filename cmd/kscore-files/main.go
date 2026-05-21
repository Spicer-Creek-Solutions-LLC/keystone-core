// kscore-files is the operator CLI for the kscore file service
// (Epic 18 task 15). Subcommands: put, get, delete, list, stat.
package main

import (
	"os"

	"go.keystone-core.io/keystone-core/internal/cli/files"
)

func main() {
	if err := files.NewFilesCommand().Execute(); err != nil {
		os.Exit(1)
	}
}
