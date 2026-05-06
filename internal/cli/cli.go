// Package cli builds the standard Cobra root command shared by every
// Keystone Core binary (kscore-server, kscore-agent, kscorectl, and the
// kscore-* helpers added by later epics).
//
// Each binary supplies its own Run callback; this package wires the common
// surface: --config flag, --version output, config + logger construction,
// SIGTERM/SIGINT-aware context, and the structured-log "starting"/"stopped"
// bookends with a per-process correlation ID.
package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"go.keystone-core.io/keystone-core/internal/config"
	"go.keystone-core.io/keystone-core/internal/logging"
	"go.keystone-core.io/keystone-core/pkg/version"
)

// RunFunc is the binary-specific work executed after config + logger are
// ready. The supplied context cancels on SIGTERM/SIGINT.
type RunFunc func(ctx context.Context, cfg *config.Config, log *slog.Logger) error

// Options configures a binary's root command.
type Options struct {
	// Name is the binary name (e.g. "kscore-server"). Required.
	Name string
	// Short is the one-line cobra short description.
	Short string
	// Long is the multi-line cobra long description (optional).
	Long string
	// Run is the binary-specific work. Required.
	Run RunFunc
}

// RootCommand returns a *cobra.Command pre-wired with --config / --version
// flags, config + logger construction in RunE, and a signal-cancelled
// context handed to opts.Run.
func RootCommand(opts Options) *cobra.Command {
	if opts.Name == "" {
		panic("cli.RootCommand: Name is required")
	}
	if opts.Run == nil {
		panic("cli.RootCommand: Run is required")
	}

	var configPath string
	info := version.Get()

	cmd := &cobra.Command{
		Use:           opts.Name,
		Short:         opts.Short,
		Long:          opts.Long,
		Version:       info.Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("config: %w", err)
			}

			log, err := logging.New(logging.Options{
				Level:  cfg.Logging.Level,
				Format: cfg.Logging.Format,
				Output: cmd.OutOrStdout(),
			})
			if err != nil {
				return fmt.Errorf("logger: %w", err)
			}

			ctx, cancel := signal.NotifyContext(cmd.Context(),
				os.Interrupt, syscall.SIGTERM)
			defer cancel()
			ctx = logging.WithCorrelationID(ctx, logging.NewCorrelationID())

			log.InfoContext(ctx, "starting",
				"binary", opts.Name,
				"version", info.Version,
				"commit", info.GitCommit,
				"mode", string(cfg.Mode),
			)
			runErr := opts.Run(ctx, cfg, log)
			log.InfoContext(ctx, "stopped", "binary", opts.Name)
			return runErr
		},
	}

	cmd.SetVersionTemplate(versionTemplate(opts.Name, info))
	cmd.PersistentFlags().StringVar(&configPath, "config", "",
		"path to YAML config file (KSCORE_-prefixed env vars overlay)")

	return cmd
}

// versionTemplate renders the multi-line --version output. cobra prints
// the template verbatim, so the trailing newline matters.
func versionTemplate(name string, info version.Info) string {
	return fmt.Sprintf("%s %s\ncommit: %s\nbuilt:  %s\n",
		name, info.Version, info.GitCommit, info.BuildDate)
}
