// kscore-blueprint is the Keystone Core blueprint authoring + apply
// CLI (Epic 15 task 10). Local verbs (init/validate/lint/info/
// bundle/install/update/remove) need no wiring; apply/rollback run
// against a local-host State Runner wired here. Remote/distributed
// apply is a gate-v1.0 ROADMAP item. Once built it is also reachable
// as `kscorectl blueprint …` via the task-13 plugin dispatch.
package main

import (
	"io"
	"os"

	"github.com/spf13/cobra"

	bp "go.keystone-core.io/keystone-core/internal/blueprint"
	clibp "go.keystone-core.io/keystone-core/internal/cli/blueprint"
	"go.keystone-core.io/keystone-core/internal/runbook"
	"go.keystone-core.io/keystone-core/internal/runbook/steps"
	"go.keystone-core.io/keystone-core/internal/statemgmt"
	"go.keystone-core.io/keystone-core/internal/statemgmt/stdlib"
)

// localExecutor builds a blueprint.Executor whose State Runner
// converges the rendered state collection on the LOCAL host using
// the stdlib modules. Hooks run as runbooks with the v1.0 step set
// (unconfigured step types fail cleanly with ErrStepNotConfigured).
func localExecutor() (*bp.Executor, error) {
	stReg := statemgmt.NewRegistry()
	if err := stdlib.RegisterAll(stReg); err != nil {
		return nil, err
	}
	rbReg := runbook.NewRegistry()
	if err := steps.RegisterAll(rbReg, steps.Deps{}); err != nil {
		return nil, err
	}
	return &bp.Executor{
		StateRunner: statemgmt.NewRunner(stReg, nil),
		Hooks:       bp.NewRunbookHookRunner(&runbook.Executor{Registry: rbReg}),
		Store:       bp.NewMemoryAppliedStore(),
	}, nil
}

func newCmd() (*cobra.Command, error) {
	ex, err := localExecutor()
	if err != nil {
		return nil, err
	}
	return clibp.NewCommand(clibp.Deps{Executor: ex}), nil
}

func run(args []string, stdout, stderr io.Writer) int {
	c, err := newCmd()
	if err != nil {
		_, _ = io.WriteString(stderr, "kscore-blueprint: "+err.Error()+"\n")
		return 1
	}
	c.SetArgs(args)
	c.SetOut(stdout)
	c.SetErr(stderr)
	if err := c.Execute(); err != nil {
		_, _ = io.WriteString(stderr, "error: "+err.Error()+"\n")
		return 1
	}
	return 0
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}
