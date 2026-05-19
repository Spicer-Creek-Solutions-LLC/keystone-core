// kscore-gitops is the Keystone Core GitOps CLI (Epic 16 task 10):
// `verify <file>` runs a verification workflow locally and `rollback`
// (plus its approve / reject / get / list subcommands) drives the
// rollback engine over a configurable SQLite store. The K8s
// client-go adapter is the deferred ROADMAP item — `--executor k8s`
// returns `ErrNotConfigured` until that lands. Reachable as
// `kscorectl gitops …` via the Epic-14 plugin dispatch.
package main

import (
	"io"
	"os"

	cligitops "go.keystone-core.io/keystone-core/internal/cli/gitops"
	"go.keystone-core.io/keystone-core/internal/gitops/rollback"
	"go.keystone-core.io/keystone-core/internal/gitops/rollback/argoexec"
	"go.keystone-core.io/keystone-core/internal/gitops/rollback/gitexec"
)

func newDeps() cligitops.Deps {
	return cligitops.Deps{
		CmdRunner: cligitops.ExecCommandRunner{},
		GitClient: &gitexec.Client{},
		NewArgoClient: func(server, token string) rollback.ArgoClient {
			return &argoexec.Client{BaseURL: server, Token: token}
		},
		// K8sClient: nil — concrete client-go adapter deferred (see
		// the gate-v1.0 "K8s rollout-undo client-go adapter +
		// GitOps rollback boot wiring" ROADMAP entry).
	}
}

func run(args []string, stdout, stderr io.Writer) int {
	cmd := cligitops.NewCommand(newDeps())
	cmd.SetArgs(args)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	if err := cmd.Execute(); err != nil {
		_, _ = io.WriteString(stderr, "error: "+err.Error()+"\n")
		return 1
	}
	return 0
}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }
