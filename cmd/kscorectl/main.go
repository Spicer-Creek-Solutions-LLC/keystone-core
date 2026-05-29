// SPDX-License-Identifier: Apache-2.0

// kscorectl is the Keystone Core operator CLI.
//
// v1.0 hello-world: parses --config, emits a JSON startup log line, and
// exits 0. Subcommands (exec, state, agents, secrets, …) accumulate
// across epics 03+.
package main

import (
	"context"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"go.keystone-core.io/keystone-core/internal/cli"
	"go.keystone-core.io/keystone-core/internal/cli/exec"
	"go.keystone-core.io/keystone-core/internal/cli/state"
	"go.keystone-core.io/keystone-core/internal/config"
	"go.keystone-core.io/keystone-core/pkg/plugin"
)

func main() {
	os.Exit(runCLI(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// runCLI is the testable entry point: it returns the process exit
// code instead of calling os.Exit (the kscore-migrate/kscore-registry
// precedent).
//
// Git/kubectl-style plugin dispatch is tried first: `kscorectl
// <name> …` where <name> is not a registered subcommand but a
// `kscore-<name>` binary is on $PATH (e.g. kscore-module, task 14).
// It never shadows a registered subcommand; otherwise cobra runs.
func runCLI(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	root := newCommand()
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if handled, code, err := plugin.Dispatch(ctx, root, args,
		plugin.New(""), plugin.Executor{}, stdin, stdout, stderr); handled {
		if err != nil {
			_, _ = io.WriteString(stderr, "error: "+err.Error()+"\n")
			return 1
		}
		return code
	}
	if err := root.Execute(); err != nil {
		_, _ = io.WriteString(stderr, "error: "+err.Error()+"\n")
		return 1
	}
	return 0
}

func newCommand() *cobra.Command {
	root := cli.RootCommand(cli.Options{
		Name:  "kscorectl",
		Short: "Keystone Core operator CLI",
		Run:   run,
	})
	root.AddCommand(exec.NewCommand(exec.Deps{}))
	root.AddCommand(state.NewCommand(state.Deps{}))
	return root
}

// run is a no-op for the v1.0 hello-world: kscorectl is a client, not a
// daemon, so it returns immediately rather than blocking on ctx.Done.
func run(_ context.Context, _ *config.Config, _ *slog.Logger) error {
	return nil
}
