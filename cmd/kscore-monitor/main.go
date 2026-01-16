package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/shawnbutts/keystone-core/cmd/kscore-monitor/config"
	"github.com/shawnbutts/keystone-core/cmd/kscore-monitor/ui"
	"github.com/shawnbutts/keystone-core/pkg/cli/auditutil"
	"github.com/shawnbutts/keystone-core/pkg/version"
	"github.com/spf13/cobra"
)

// Options holds CLI options
type Options struct {
	ConfigFile   string
	ControlPlane string
	NATSURL      string
	Theme        string
	Refresh      int
	NoColor      bool
	cfg          *config.Config
}

var (
	auditLevel  string
	auditOutput string
)

func main() {
	rootCmd := newRootCmd()
	auditHandler := auditutil.Attach(rootCmd, "kscore-monitor", &auditLevel, &auditOutput)
	if err := rootCmd.Execute(); err != nil {
		auditHandler.LogFailure(err)
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// newRootCmd creates the root command
func newRootCmd() *cobra.Command {
	opts := &Options{}

	rootCmd := &cobra.Command{
		Use:   "kscore-monitor",
		Short: "Keystone Core TUI monitoring interface",
		Long: `kscore-monitor provides a terminal-based user interface for monitoring
Keystone Core infrastructure in real-time.

Features:
  - Live dashboard with system metrics
  - Agent status and health monitoring
  - Event stream viewer with filtering
  - State drift detection and visualization
  - Policy violation tracking
  - Job execution monitoring
  - Log streaming with search

Navigate between views using number keys (1-8) or arrow keys.
Press 'q' to quit, '?' for help.`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return initConfig(cmd, opts)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMonitor(cmd, args, opts)
		},
	}

	rootCmd.PersistentFlags().StringVar(&opts.ConfigFile, "config", "", "config file (default: $HOME/.kscore/monitor.yaml)")
	rootCmd.PersistentFlags().StringVar(&auditLevel, "audit-level", "all", "Audit logging level (all, errors, none)")
	rootCmd.PersistentFlags().StringVar(&auditOutput, "audit-output", "auto", "Audit output backend (auto, syslog, journald, stderr, none)")
	rootCmd.Flags().StringVar(&opts.ControlPlane, "control-plane", "localhost:50051", "Control plane gRPC address")
	rootCmd.Flags().StringVar(&opts.NATSURL, "nats-url", "nats://localhost:4222", "NATS server URL")
	rootCmd.Flags().StringVar(&opts.Theme, "theme", "dark", "UI theme (dark, light, solarized-dark, solarized-light, monokai)")
	rootCmd.Flags().IntVar(&opts.Refresh, "refresh", 2, "Refresh interval in seconds")
	rootCmd.Flags().BoolVar(&opts.NoColor, "no-color", false, "Disable colors")

	rootCmd.AddCommand(newVersionCmd())

	return rootCmd
}

// newVersionCmd creates the version command
func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			info := version.Get()
			fmt.Fprintln(cmd.OutOrStdout(), info.String())
		},
	}
}

func initConfig(cmd *cobra.Command, opts *Options) error {
	var err error
	opts.cfg, err = config.Load(opts.ConfigFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		opts.cfg = config.Default()
	}

	// Override with command-line flags
	if cmd.Flags().Changed("control-plane") {
		opts.cfg.ControlPlane = opts.ControlPlane
	}
	if cmd.Flags().Changed("nats-url") {
		opts.cfg.NATSURL = opts.NATSURL
	}
	if cmd.Flags().Changed("theme") {
		opts.cfg.Theme = opts.Theme
	}
	if cmd.Flags().Changed("refresh") {
		opts.cfg.RefreshInterval = opts.Refresh
	}
	if cmd.Flags().Changed("no-color") {
		opts.cfg.NoColor = opts.NoColor
	}
	return nil
}

func runMonitor(cmd *cobra.Command, args []string, opts *Options) error {
	// Create context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle signals for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		cancel()
	}()

	// Create and run the TUI
	program, err := ui.NewProgram(ctx, opts.cfg)
	if err != nil {
		return fmt.Errorf("failed to create UI program: %w", err)
	}

	// Run the program
	if _, err := program.Run(); err != nil {
		return fmt.Errorf("error running TUI: %w", err)
	}

	return nil
}
