package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	mcpserver "github.com/shawnbutts/keystone-core/internal/mcp"
	"github.com/shawnbutts/keystone-core/internal/mcp/resources"
	"github.com/shawnbutts/keystone-core/internal/mcp/tools"
	"github.com/shawnbutts/keystone-core/pkg/version"
)

func main() {
	var configPath string

	rootCmd := &cobra.Command{
		Use:   "kscore-mcp",
		Short: "MCP server for AI-assisted Keystone Core operations",
		Long: `kscore-mcp exposes Keystone Core operations to AI clients via the
Model Context Protocol (MCP). It translates MCP tool calls into gRPC/REST
API calls using the operator's own credentials.

Configure with a YAML file and add to your AI client's MCP settings:

  {"mcpServers": {"keystone": {"command": "kscore-mcp", "args": ["--config", "mcp.yaml"]}}}`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd.Context(), configPath)
		},
	}

	rootCmd.Flags().StringVar(&configPath, "config", "", "path to MCP config file (required)")
	_ = rootCmd.MarkFlagRequired("config")

	rootCmd.AddCommand(newVersionCmd())
	rootCmd.AddCommand(newValidateCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func run(ctx context.Context, configPath string) error {
	cfg, err := mcpserver.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	sessionID := uuid.New().String()
	aiClient := "unknown"

	client, err := mcpserver.NewGRPCClient(cfg, sessionID, aiClient)
	if err != nil {
		return fmt.Errorf("connecting to kscore-server: %w", err)
	}
	defer client.Close()

	allTools := tools.RegisterAll(client, cfg.Features.MaxTargetCount)
	allResources := resources.RegisterAll(client)

	srv := mcpserver.NewServer(cfg, client, allTools, allResources)

	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	return srv.Run(ctx)
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

func newValidateCmd() *cobra.Command {
	var configPath string
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate MCP config file",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := mcpserver.LoadConfig(configPath)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Config valid: server=%s auth=%s profile=%s\n",
				cfg.Server.Address, cfg.Auth.Method, cfg.Profile())
			return nil
		},
	}
	cmd.Flags().StringVar(&configPath, "config", "", "path to MCP config file")
	_ = cmd.MarkFlagRequired("config")
	return cmd
}
