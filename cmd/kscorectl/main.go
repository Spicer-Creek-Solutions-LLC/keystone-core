// kscorectl is the Keystone Core operator CLI.
//
// v1.0 hello-world: parses --config, emits a JSON startup log line, and
// exits 0. Subcommands (exec, state, agents, secrets, …) accumulate
// across epics 03+.
package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"go.keystone-core.io/keystone-core/internal/cli"
	"go.keystone-core.io/keystone-core/internal/config"
)

func main() {
	if err := newCommand().Execute(); err != nil {
		os.Exit(1)
	}
}

func newCommand() *cobra.Command {
	return cli.RootCommand(cli.Options{
		Name:  "kscorectl",
		Short: "Keystone Core operator CLI",
		Run:   run,
	})
}

// run is a no-op for the v1.0 hello-world: kscorectl is a client, not a
// daemon, so it returns immediately rather than blocking on ctx.Done.
func run(_ context.Context, _ *config.Config, _ *slog.Logger) error {
	return nil
}
