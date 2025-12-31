package main

import (
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/shawnbutts/keystone-core/pkg/agent"
	"github.com/shawnbutts/keystone-core/pkg/config"
	natsmgr "github.com/shawnbutts/keystone-core/pkg/nats"
	"github.com/shawnbutts/keystone-core/pkg/version"
)

var (
	cfgFile string
	rootCmd = &cobra.Command{
		Use:   "kscore-agent",
		Short: "Keystone Core agent",
		Long: `Keystone Core agent runs on managed nodes and executes commands
from the control plane. It supports embedded NATS mode for edge deployments.`,
		Run: runAgent,
	}
)

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./keystone-core-agent.yaml)")
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		info := version.Get()
		fmt.Println(info.String())
	},
}

func runAgent(cmd *cobra.Command, args []string) {
	// Print version
	info := version.Get()
	fmt.Printf("Starting %s\n", info.String())

	// Load configuration
	cfg, err := config.LoadConfig(cfgFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// For agents, default to embedded NATS if not configured
	if cfg.NATS.Mode == "" {
		cfg.NATS.Mode = config.NATSModeEmbedded
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "Invalid configuration: %v\n", err)
		os.Exit(1)
	}

	// Collect system metadata
	hostname, _ := os.Hostname()
	fmt.Printf("Agent Information:\n")
	fmt.Printf("  Hostname: %s\n", hostname)
	fmt.Printf("  OS: %s\n", runtime.GOOS)
	fmt.Printf("  Architecture: %s\n", runtime.GOARCH)
	fmt.Printf("  Agent ID: %s\n", cfg.Agent.ID)
	fmt.Printf("  NATS Mode: %s\n", cfg.NATS.Mode)

	// Initialize NATS manager
	fmt.Println("Initializing NATS manager...")
	natsManager, err := natsmgr.NewManager(&cfg.NATS)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create NATS manager: %v\n", err)
		os.Exit(1)
	}

	// Start NATS manager
	if err := natsManager.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start NATS manager: %v\n", err)
		os.Exit(1)
	}
	defer natsManager.Shutdown()

	if natsManager.IsEmbedded() {
		fmt.Printf("Embedded NATS server started on port %d\n", cfg.NATS.Embedded.Port)
		if cfg.NATS.Mode == config.NATSModeLeaf {
			fmt.Printf("Running as leaf node, connected to: %v\n", cfg.NATS.Embedded.LeafNodeURLs)
		}
	} else {
		fmt.Printf("Connected to external NATS at %s\n", cfg.NATS.URL)
	}

	// Health check
	if err := natsManager.Health(); err != nil {
		fmt.Fprintf(os.Stderr, "NATS health check failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("NATS health check passed")

	// Create agent
	fmt.Println("Creating agent instance...")
	agnt, err := agent.NewAgent(
		cfg.Agent.ID,
		natsManager,
		cfg.Agent.HeartbeatInterval,
		cfg.Agent.MetadataInterval,
		cfg.Agent.CommandTimeout,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create agent: %v\n", err)
		os.Exit(1)
	}

	// Start agent services (registration, heartbeat, command subscription)
	if err := agnt.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start agent: %v\n", err)
		os.Exit(1)
	}
	defer agnt.Stop()

	fmt.Printf("Agent %s is running\n", agnt.ID())
	fmt.Printf("  Heartbeat Interval: %s\n", cfg.Agent.HeartbeatInterval)
	fmt.Println("Waiting for commands...")

	// Wait for termination signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	select {
	case <-sigChan:
		fmt.Println("\nReceived shutdown signal, shutting down gracefully...")
	}

	// Graceful shutdown
	fmt.Println("Stopping agent...")
	if err := agnt.Stop(); err != nil {
		fmt.Fprintf(os.Stderr, "Error stopping agent: %v\n", err)
	}

	fmt.Println("Shutting down NATS manager...")
	if err := natsManager.Shutdown(); err != nil {
		fmt.Fprintf(os.Stderr, "Error during shutdown: %v\n", err)
	}

	fmt.Println("Agent shutdown complete")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
