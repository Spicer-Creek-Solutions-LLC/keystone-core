// Package main implements the kscore-agent daemon for managed nodes.
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/shawnbutts/keystone-core/internal/agent"
	"github.com/shawnbutts/keystone-core/internal/config"
)

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage agent configuration",
		Long:  `Commands for managing the agent's configuration file.`,
	}

	cmd.AddCommand(newEnableEmbeddedNATSCmd())
	cmd.AddCommand(newDisableEmbeddedNATSCmd())
	cmd.AddCommand(newShowConfigCmd())

	return cmd
}

func newEnableEmbeddedNATSCmd() *cobra.Command {
	var (
		configPath string
		host       string
		port       int
		restart    bool
	)

	cmd := &cobra.Command{
		Use:   "enable-embedded-nats",
		Short: "Enable embedded NATS server mode",
		Long: `Enables the embedded NATS server mode in the agent configuration.

This allows the agent to host its own NATS server, which is useful for:
- Edge deployments without a central NATS cluster
- Development and testing
- Single-node deployments

Security notes:
- By default, embedded NATS binds to 127.0.0.1 (localhost only)
- To allow external connections, you must configure TLS and authentication
- Use --host=0.0.0.0 only with proper TLS and auth configuration`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEnableEmbeddedNATS(configPath, host, port, restart)
		},
	}

	defaultConfig := agent.DefaultConfigPath()

	cmd.Flags().StringVar(&configPath, "config", defaultConfig, "Path to agent config file")
	cmd.Flags().StringVar(&host, "host", "127.0.0.1", "NATS server bind host (use 0.0.0.0 for external access, requires TLS)")
	cmd.Flags().IntVar(&port, "port", 4222, "NATS server port")
	cmd.Flags().BoolVar(&restart, "restart", false, "Restart the agent service after updating config")

	return cmd
}

func newDisableEmbeddedNATSCmd() *cobra.Command {
	var (
		configPath string
		natsURL    string
		restart    bool
	)

	cmd := &cobra.Command{
		Use:   "disable-embedded-nats",
		Short: "Disable embedded NATS and connect to external cluster",
		Long: `Disables the embedded NATS server and configures the agent to connect
to an external NATS cluster.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDisableEmbeddedNATS(configPath, natsURL, restart)
		},
	}

	defaultConfig := agent.DefaultConfigPath()

	cmd.Flags().StringVar(&configPath, "config", defaultConfig, "Path to agent config file")
	cmd.Flags().StringVar(&natsURL, "nats-url", "", "External NATS cluster URL (required)")
	cmd.Flags().BoolVar(&restart, "restart", false, "Restart the agent service after updating config")

	cmd.MarkFlagRequired("nats-url")

	return cmd
}

func newShowConfigCmd() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show current agent configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runShowConfig(cmd, configPath)
		},
	}

	defaultConfig := agent.DefaultConfigPath()
	cmd.Flags().StringVar(&configPath, "config", defaultConfig, "Path to agent config file")

	return cmd
}

func runEnableEmbeddedNATS(configPath, host string, port int, restart bool) error {
	// Load existing config or create new
	cfg, err := loadOrCreateConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Update NATS configuration
	cfg.NATS.Mode = config.NATSModeEmbedded
	cfg.NATS.Embedded.Host = host
	cfg.NATS.Embedded.Port = port

	// Validate the configuration
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	// Save config
	if err := saveConfig(configPath, cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("Embedded NATS enabled in %s\n", configPath)
	fmt.Printf("  Mode: embedded\n")
	fmt.Printf("  Host: %s\n", host)
	fmt.Printf("  Port: %d\n", port)

	if host == "127.0.0.1" || host == "::1" || host == "localhost" {
		fmt.Println("\nNote: NATS is bound to localhost only. This is the secure default.")
		fmt.Println("To allow external connections, reconfigure with --host=0.0.0.0 and set up TLS.")
	} else {
		fmt.Println("\nWarning: NATS is configured to allow external connections.")
		fmt.Println("Ensure TLS and authentication are properly configured.")
	}

	if restart {
		return restartAgentService()
	}

	fmt.Println("\nRestart the agent for changes to take effect:")
	fmt.Println("  systemctl restart kscore-agent  # Linux")
	fmt.Println("  sc.exe stop kscore-agent && sc.exe start kscore-agent  # Windows")

	return nil
}

func runDisableEmbeddedNATS(configPath, natsURL string, restart bool) error {
	// Load existing config or create new
	cfg, err := loadOrCreateConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Update NATS configuration
	cfg.NATS.Mode = config.NATSModeExternal
	cfg.NATS.URL = natsURL

	// Validate the configuration
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	// Save config
	if err := saveConfig(configPath, cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("External NATS configured in %s\n", configPath)
	fmt.Printf("  Mode: external\n")
	fmt.Printf("  URL: %s\n", natsURL)

	if restart {
		return restartAgentService()
	}

	fmt.Println("\nRestart the agent for changes to take effect.")

	return nil
}

func runShowConfig(cmd *cobra.Command, configPath string) error {
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Output sanitized config (hide secrets)
	fmt.Fprintf(cmd.OutOrStdout(), "Configuration from: %s\n\n", configPath)
	fmt.Fprintf(cmd.OutOrStdout(), "NATS:\n")
	fmt.Fprintf(cmd.OutOrStdout(), "  Mode: %s\n", cfg.NATS.Mode)

	if cfg.NATS.Mode == config.NATSModeEmbedded {
		fmt.Fprintf(cmd.OutOrStdout(), "  Embedded:\n")
		fmt.Fprintf(cmd.OutOrStdout(), "    Host: %s\n", cfg.NATS.Embedded.Host)
		fmt.Fprintf(cmd.OutOrStdout(), "    Port: %d\n", cfg.NATS.Embedded.Port)
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "  URL: %s\n", cfg.NATS.URL)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "\nAgent:\n")
	fmt.Fprintf(cmd.OutOrStdout(), "  ID: %s\n", cfg.Agent.ID)
	fmt.Fprintf(cmd.OutOrStdout(), "  Heartbeat Interval: %s\n", cfg.Agent.HeartbeatInterval)
	fmt.Fprintf(cmd.OutOrStdout(), "  Command Timeout: %s\n", cfg.Agent.CommandTimeout)

	return nil
}

func loadOrCreateConfig(path string) (*config.Config, error) {
	// Try to load existing config
	cfg, err := config.LoadConfig(path)
	if err != nil {
		// If file doesn't exist, create default config
		if os.IsNotExist(err) {
			cfg = &config.Config{}
			return cfg, nil
		}
		return nil, err
	}
	return cfg, nil
}

func saveConfig(path string, cfg *config.Config) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	//nolint:gosec // G301: config directory needs to be accessible by admin users
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Marshal to YAML
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write file
	//nolint:gosec // G306: agent config needs to be readable by the agent service
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

func restartAgentService() error {
	fmt.Println("\nRestarting agent service...")

	ctx := context.Background()
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "linux":
		cmd = exec.CommandContext(ctx, "systemctl", "restart", "kscore-agent")
	case "darwin":
		cmd = exec.CommandContext(ctx, "launchctl", "kickstart", "-k", "system/com.keystone.agent")
	case "windows":
		// Stop and start the service
		stopCmd := exec.CommandContext(ctx, "sc.exe", "stop", "kscore-agent")
		if err := stopCmd.Run(); err != nil {
			fmt.Printf("Warning: failed to stop service: %v\n", err)
		}
		cmd = exec.CommandContext(ctx, "sc.exe", "start", "kscore-agent")
	default:
		return fmt.Errorf("unsupported platform for service restart: %s", runtime.GOOS)
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to restart service: %w", err)
	}

	fmt.Println("Agent service restarted successfully")
	return nil
}

func init() {
	// Register the config command with the root command
	// This will be called by newRootCmd
}
