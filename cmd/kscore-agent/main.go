package main

import (
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/spf13/cobra"

	agentbootstrap "github.com/shawnbutts/keystone-core/cmd/kscore-agent/bootstrap"
	"github.com/shawnbutts/keystone-core/pkg/agent"
	"github.com/shawnbutts/keystone-core/pkg/config"
	"github.com/shawnbutts/keystone-core/pkg/logging"
	natsmgr "github.com/shawnbutts/keystone-core/pkg/nats"
	"github.com/shawnbutts/keystone-core/pkg/version"
)

// logger is the structured logger for kscore-agent (Epic 15)
var logger logging.Logger

var cfgFile string

func newRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "kscore-agent",
		Short: "Keystone Core agent",
		Long: `Keystone Core agent runs on managed nodes and executes commands
from the control plane. It supports embedded NATS mode for edge deployments.`,
		Run:           runAgent,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./keystone-core-agent.yaml)")
	rootCmd.AddCommand(newVersionCmd())
	rootCmd.AddCommand(newConfigCmd())
	rootCmd.AddCommand(agentbootstrap.NewCommand())

	// Add Windows service management commands
	rootCmd.AddCommand(newServiceInstallCmd())
	rootCmd.AddCommand(newServiceUninstallCmd())
	rootCmd.AddCommand(newServiceStartCmd())
	rootCmd.AddCommand(newServiceStopCmd())
	rootCmd.AddCommand(newServiceStatusCmd())

	return rootCmd
}

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

func runAgent(cmd *cobra.Command, args []string) {
	// Initialize logger early (use env vars before config is loaded)
	logger = logging.InitDefaultLogger("kscore-agent")

	// Get version info
	info := version.Get()
	logger.Info("Starting Keystone Core agent",
		logging.String("version", info.Version),
		logging.String("commit", info.GitCommit),
		logging.String("build_date", info.BuildDate),
	)

	// Load configuration
	cfg, err := config.LoadConfig(cfgFile)
	if err != nil {
		logger.Error("Failed to load configuration", logging.Error(err))
		os.Exit(1)
	}

	// Security: Require explicit NATS mode configuration
	// Embedded mode must be explicitly enabled for security
	if cfg.NATS.Mode == "" {
		logger.Error("NATS mode must be explicitly configured",
			logging.String("hint", "Set nats.mode to 'external' to connect to a NATS cluster, or use 'kscore-agent config enable-embedded-nats' to enable embedded mode"),
		)
		os.Exit(1)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		logger.Error("Invalid configuration", logging.Error(err))
		os.Exit(1)
	}

	// Re-initialize logger with config settings (Epic 15)
	logger = logging.InitDefaultLoggerFromConfig(&cfg.Logging, "kscore-agent")

	// Collect system metadata
	hostname, _ := os.Hostname()
	logger.Info("Agent information",
		logging.String("hostname", hostname),
		logging.String("os", runtime.GOOS),
		logging.String("arch", runtime.GOARCH),
		logging.String("agent_id", cfg.Agent.ID),
		logging.String("nats_mode", string(cfg.NATS.Mode)),
	)

	// Initialize NATS manager
	logger.Info("Initializing NATS manager")
	natsManager, err := natsmgr.NewManager(&cfg.NATS)
	if err != nil {
		logger.Error("Failed to create NATS manager", logging.Error(err))
		os.Exit(1)
	}

	// Start NATS manager
	if err := natsManager.Start(); err != nil {
		logger.Error("Failed to start NATS manager", logging.Error(err))
		os.Exit(1)
	}
	defer natsManager.Shutdown()

	if natsManager.IsEmbedded() {
		logger.Info("Embedded NATS server started", logging.Int("port", cfg.NATS.Embedded.Port))
		if cfg.NATS.Mode == config.NATSModeLeaf {
			logger.Info("Running as leaf node", logging.Any("leaf_urls", cfg.NATS.Embedded.LeafNodeURLs))
		}
	} else {
		logger.Info("Connected to external NATS", logging.String("url", cfg.NATS.URL))
	}

	// Health check
	if err := natsManager.Health(); err != nil {
		logger.Error("NATS health check failed", logging.Error(err))
		os.Exit(1)
	}
	logger.Info("NATS health check passed")

	// Create agent
	logger.Info("Creating agent instance")
	agnt, err := agent.NewAgentWithConfig(natsManager, &agent.AgentConfig{
		ID:                cfg.Agent.ID,
		HeartbeatInterval: cfg.Agent.HeartbeatInterval,
		MetadataInterval:  cfg.Agent.MetadataInterval,
		CommandTimeout:    cfg.Agent.CommandTimeout,
		Labels:            cfg.Agent.Labels,
	})
	if err != nil {
		logger.Error("Failed to create agent", logging.Error(err))
		os.Exit(1)
	}

	// Start agent services (registration, heartbeat, command subscription)
	if err := agnt.Start(); err != nil {
		logger.Error("Failed to start agent", logging.Error(err))
		os.Exit(1)
	}
	defer agnt.Stop()

	logger.Info("Agent is running",
		logging.String("agent_id", agnt.ID()),
		logging.Duration("heartbeat_interval", cfg.Agent.HeartbeatInterval),
	)
	logger.Info("Waiting for commands")

	// Wait for termination signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	select {
	case <-sigChan:
		logger.Info("Received shutdown signal, shutting down gracefully")
	}

	// Graceful shutdown
	logger.Info("Stopping agent")
	if err := agnt.Stop(); err != nil {
		logger.Error("Error stopping agent", logging.Error(err))
	}

	logger.Info("Shutting down NATS manager")
	if err := natsManager.Shutdown(); err != nil {
		logger.Error("Error during shutdown", logging.Error(err))
	}

	logger.Info("Agent shutdown complete")
}

func main() {
	// Check if running as a Windows service
	if agent.IsWindowsService() {
		// Running as a Windows service - create and run agent in service mode
		runAsWindowsService()
		return
	}

	// Running interactively - use Cobra CLI
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// runAsWindowsService runs the agent as a Windows service
func runAsWindowsService() {
	// Initialize logger for service mode
	logger = logging.InitDefaultLogger("kscore-agent")

	// Use Windows-specific config path if not set via environment
	configPath := os.Getenv("KSCORE_CONFIG")
	if configPath == "" {
		configPath = agent.DefaultConfigPath()
	}

	// Load configuration
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		logger.Error("Failed to load configuration", logging.Error(err))
		os.Exit(1)
	}

	// Security: Require explicit NATS mode configuration
	if cfg.NATS.Mode == "" {
		logger.Error("NATS mode must be explicitly configured")
		os.Exit(1)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		logger.Error("Invalid configuration", logging.Error(err))
		os.Exit(1)
	}

	// Re-initialize logger with config settings
	logger = logging.InitDefaultLoggerFromConfig(&cfg.Logging, "kscore-agent")

	// Initialize NATS manager
	natsManager, err := natsmgr.NewManager(&cfg.NATS)
	if err != nil {
		logger.Error("Failed to create NATS manager", logging.Error(err))
		os.Exit(1)
	}

	if err := natsManager.Start(); err != nil {
		logger.Error("Failed to start NATS manager", logging.Error(err))
		os.Exit(1)
	}
	defer natsManager.Shutdown()

	// Create agent
	agnt, err := agent.NewAgentWithConfig(natsManager, &agent.AgentConfig{
		ID:                cfg.Agent.ID,
		HeartbeatInterval: cfg.Agent.HeartbeatInterval,
		MetadataInterval:  cfg.Agent.MetadataInterval,
		CommandTimeout:    cfg.Agent.CommandTimeout,
		Labels:            cfg.Agent.Labels,
	})
	if err != nil {
		logger.Error("Failed to create agent", logging.Error(err))
		os.Exit(1)
	}

	// Run as Windows service
	logger.Info("Starting agent as Windows service")
	if err := agent.RunAsService(agnt); err != nil {
		logger.Error("Service failed", logging.Error(err))
		os.Exit(1)
	}
}
