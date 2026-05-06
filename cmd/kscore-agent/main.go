// kscore-agent is the Keystone Core agent daemon.
//
// v1.0 hello-world: parses --config, emits a JSON startup log line, blocks
// until SIGTERM/SIGINT, and exits cleanly. NATS connect, registration,
// heartbeats, and command execution land in epic 06.
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
		Name:  "kscore-agent",
		Short: "Keystone Core agent daemon",
		Run:   run,
	})
}

func run(ctx context.Context, _ *config.Config, _ *slog.Logger) error {
	<-ctx.Done()
	return nil
}
