package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/shawnbutts/keystone-core/cmd/kscore-monitor/config"
	"github.com/shawnbutts/keystone-core/cmd/kscore-monitor/ui"
	"github.com/shawnbutts/keystone-core/pkg/version"
)

var (
	cfgFile string
	cfg     *config.Config
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
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
	RunE: runMonitor,
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: $HOME/.kscore/monitor.yaml)")
	rootCmd.Flags().String("control-plane", "localhost:50051", "Control plane gRPC address")
	rootCmd.Flags().String("nats-url", "nats://localhost:4222", "NATS server URL")
	rootCmd.Flags().String("theme", "dark", "UI theme (dark, light, solarized-dark, solarized-light, monokai)")
	rootCmd.Flags().Int("refresh", 2, "Refresh interval in seconds")
	rootCmd.Flags().Bool("no-color", false, "Disable colors")

	// Version command
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("kscore-monitor version %s\n", version.Version)
			fmt.Printf("  Build date: %s\n", version.BuildDate)
			fmt.Printf("  Git commit: %s\n", version.GitCommit)
		},
	}
	rootCmd.AddCommand(versionCmd)
}

func initConfig() {
	var err error
	cfg, err = config.Load(cfgFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		cfg = config.Default()
	}

	// Override with command-line flags
	if rootCmd.Flags().Changed("control-plane") {
		cfg.ControlPlane, _ = rootCmd.Flags().GetString("control-plane")
	}
	if rootCmd.Flags().Changed("nats-url") {
		cfg.NATSURL, _ = rootCmd.Flags().GetString("nats-url")
	}
	if rootCmd.Flags().Changed("theme") {
		cfg.Theme, _ = rootCmd.Flags().GetString("theme")
	}
	if rootCmd.Flags().Changed("refresh") {
		cfg.RefreshInterval, _ = rootCmd.Flags().GetInt("refresh")
	}
	if rootCmd.Flags().Changed("no-color") {
		noColor, _ := rootCmd.Flags().GetBool("no-color")
		cfg.NoColor = noColor
	}
}

func runMonitor(cmd *cobra.Command, args []string) error {
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
	program, err := ui.NewProgram(ctx, cfg)
	if err != nil {
		return fmt.Errorf("failed to create UI program: %w", err)
	}

	// Run the program
	if _, err := program.Run(); err != nil {
		return fmt.Errorf("error running TUI: %w", err)
	}

	return nil
}
