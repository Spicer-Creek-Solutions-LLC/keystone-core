// kscore-module is the Keystone Core module author + distribution
// CLI (Epic 14 task 14). It scaffolds, builds, signs, verifies,
// resolves, installs, and tests Starlark modules against a
// kscore-registry. Once built it is also reachable as
// `kscorectl module …` via the task-13 plugin mechanism.
package main

import (
	"os"

	"go.keystone-core.io/keystone-core/internal/cli/module"
)

func main() {
	if err := module.NewCommand(module.Deps{}).Execute(); err != nil {
		os.Exit(1)
	}
}
