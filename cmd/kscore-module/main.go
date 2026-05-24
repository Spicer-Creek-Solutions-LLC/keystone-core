// SPDX-License-Identifier: Apache-2.0

// kscore-module is the Keystone Core module author + distribution
// CLI (Epic 14 task 14). It scaffolds, builds, signs, verifies,
// resolves, installs, and tests Starlark modules against a
// kscore-registry. Once built it is also reachable as
// `kscorectl module …` via the task-13 plugin mechanism.
package main

import (
	"context"
	"io"
	"os"

	"github.com/spf13/cobra"

	"go.keystone-core.io/keystone-core/internal/cli/module"
	moduletest "go.keystone-core.io/keystone-core/pkg/module/testing"
)

// testRunner adapts the task-15 pkg/module/testing runner onto the
// internal/cli/module.TestRunner seam. The real runner is wired
// here at the cmd boot layer (not in the dep-light CLI package) —
// the established Epic 14 production-wiring pattern.
type testRunner struct{}

func (testRunner) RunTests(
	ctx context.Context, moduleDir string, a module.AuditOptions,
) (passed, failed int, err error) {
	rep, err := moduletest.Run(ctx, moduleDir, moduletest.Options{
		Audit: moduletest.AuditOptions{Level: a.Level, Output: a.Output},
	})
	if err != nil {
		return 0, 0, err
	}
	return rep.Passed, rep.Failed, nil
}

func newCmd() *cobra.Command {
	return module.NewCommand(module.Deps{TestRunner: testRunner{}})
}

// run builds and executes the CLI, returning the process exit code
// (testable seam — the kscore-migrate/registry precedent).
func run(args []string, stdout, stderr io.Writer) int {
	c := newCmd()
	c.SetArgs(args)
	c.SetOut(stdout)
	c.SetErr(stderr)
	if err := c.Execute(); err != nil {
		return 1
	}
	return 0
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}
