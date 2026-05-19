// kscore-runbook is the Keystone Core runbook CLI (Epic 15 task 10):
// list, execute, status, list-executions, audit, test. The engine is
// wired with the v1.0 step set; step types whose backends are not
// configured here fail cleanly with ErrStepNotConfigured. Execution
// persistence is in-memory (durable store is a gate-v1.0 ROADMAP
// item). Reachable as `kscorectl runbook …` via plugin dispatch.
package main

import (
	"io"
	"os"

	clirb "go.keystone-core.io/keystone-core/internal/cli/runbook"
	"go.keystone-core.io/keystone-core/internal/runbook"
	"go.keystone-core.io/keystone-core/internal/runbook/steps"
)

func newCmd() (*clirb.Deps, error) {
	reg := runbook.NewRegistry()
	if err := steps.RegisterAll(reg, steps.Deps{}); err != nil {
		return nil, err
	}
	return &clirb.Deps{
		Executor: &runbook.Executor{Registry: reg},
		Store:    clirb.NewMemoryExecutionStore(),
	}, nil
}

func run(args []string, stdout, stderr io.Writer) int {
	d, err := newCmd()
	if err != nil {
		_, _ = io.WriteString(stderr, "kscore-runbook: "+err.Error()+"\n")
		return 1
	}
	c := clirb.NewCommand(*d)
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
