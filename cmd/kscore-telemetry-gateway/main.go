// Package main is the entry point for the Keystone Core telemetry gateway.
//
// The telemetry gateway aggregates metrics, logs, and traces from agents over NATS
// and exposes them to observability backends (Prometheus, Loki, Tempo/Jaeger).
package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/shawnbutts/keystone-core/pkg/gateway"
	"github.com/shawnbutts/keystone-core/pkg/version"
)

// Config holds CLI configuration
type Config struct {
	ConfigFile     string
	ListenAddr     string
	NATSURL        string
	MetricsEnabled bool
	LogsEnabled    bool
	TracesEnabled  bool
}

func newRootCmd() *cobra.Command {
	cfg := &Config{
		MetricsEnabled: true,
		LogsEnabled:    true,
		TracesEnabled:  true,
	}

	rootCmd := &cobra.Command{
		Use:   "kscore-telemetry-gateway",
		Short: "Telemetry aggregation gateway for Keystone Core",
		Long: `The telemetry gateway aggregates metrics, logs, and traces from agents over NATS
and exposes them to observability backends (Prometheus, Loki, Tempo/Jaeger).

The gateway subscribes to NATS subjects for each telemetry type and provides:
  - Prometheus scrape endpoint for metrics
  - Loki push integration for logs
  - OTLP export for traces

Examples:
  # Start with default settings
  kscore-telemetry-gateway serve

  # Start with custom config file
  kscore-telemetry-gateway serve --config /etc/kscore/gateway.yaml

  # Override specific settings
  kscore-telemetry-gateway serve --listen 0.0.0.0:9091 --nats-url nats://localhost:4222`,
	}

	rootCmd.PersistentFlags().StringVar(&cfg.ConfigFile, "config", "", "Path to configuration file")
	rootCmd.PersistentFlags().StringVar(&cfg.ListenAddr, "listen", "", "Listen address (e.g., 0.0.0.0:9091)")
	rootCmd.PersistentFlags().StringVar(&cfg.NATSURL, "nats-url", "", "NATS server URL")
	rootCmd.PersistentFlags().BoolVar(&cfg.MetricsEnabled, "metrics", true, "Enable metrics gateway")
	rootCmd.PersistentFlags().BoolVar(&cfg.LogsEnabled, "logs", true, "Enable logs gateway")
	rootCmd.PersistentFlags().BoolVar(&cfg.TracesEnabled, "traces", true, "Enable traces gateway")

	rootCmd.AddCommand(newVersionCmd())
	rootCmd.AddCommand(newServeCmd(cfg))

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

// newServeCmd creates the serve command
func newServeCmd(cfg *Config) *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Start the telemetry gateway server",
		Long: `Start the telemetry gateway server to aggregate metrics, logs, and traces.

The server will:
  - Connect to NATS and subscribe to telemetry subjects
  - Expose a Prometheus-compatible /metrics endpoint
  - Push logs to Loki if configured
  - Export traces via OTLP if configured

Examples:
  # Start with defaults
  kscore-telemetry-gateway serve

  # Start with config file
  kscore-telemetry-gateway serve --config /etc/kscore/gateway.yaml

  # Start with overrides
  kscore-telemetry-gateway serve --listen :9091 --nats-url nats://nats:4222`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServe(cfg)
		},
	}
}

func runServe(cfg *Config) error {
	config := gateway.DefaultConfig()

	if cfg.ConfigFile != "" {
		data, err := os.ReadFile(cfg.ConfigFile)
		if err != nil {
			return fmt.Errorf("failed to read config file: %w", err)
		}
		if err := yaml.Unmarshal(data, config); err != nil {
			return fmt.Errorf("failed to parse config file: %w", err)
		}
		log.Printf("Loaded configuration from %s", cfg.ConfigFile)
	}

	if cfg.ListenAddr != "" {
		config.Server.Listen = cfg.ListenAddr
	}
	if cfg.NATSURL != "" {
		config.NATS.URLs = []string{cfg.NATSURL}
	}
	config.Metrics.Enabled = cfg.MetricsEnabled
	config.Logs.Enabled = cfg.LogsEnabled
	config.Traces.Enabled = cfg.TracesEnabled

	server := gateway.NewServer(config)

	if err := server.Start(); err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}

	log.Printf("Telemetry gateway started")
	log.Printf("  Listen: %s", config.Server.Listen)
	log.Printf("  Metrics: %v (subject: %s)", config.Metrics.Enabled, config.Metrics.Subject)
	log.Printf("  Logs: %v (subject: %s)", config.Logs.Enabled, config.Logs.Subject)
	log.Printf("  Traces: %v (subject: %s)", config.Traces.Enabled, config.Traces.Subject)

	log.Printf("Endpoints:")
	log.Printf("  GET %s - Prometheus metrics", config.Server.MetricsPath)
	log.Printf("  GET %s - Health check", config.Server.HealthPath)
	log.Printf("  GET %s - Readiness check", config.Server.ReadyPath)
	if config.Metrics.Federation.Enabled {
		log.Printf("  GET %s - Prometheus federation", config.Server.FederatePath)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				stats := server.Stats()
				if stats.Metrics != nil {
					log.Printf("Metrics: agents=%d series=%d msgs=%d bytes=%d",
						stats.Metrics.AgentCount,
						stats.Metrics.SeriesCount,
						stats.Metrics.MessagesRecv,
						stats.Metrics.BytesRecv)
				}
				if stats.Logs != nil {
					log.Printf("Logs: entries=%d msgs=%d bytes=%d",
						stats.Logs.EntryCount,
						stats.Logs.MessagesRecv,
						stats.Logs.BytesRecv)
				}
				if stats.Traces != nil {
					log.Printf("Traces: traces=%d msgs=%d bytes=%d",
						stats.Traces.TraceCount,
						stats.Traces.MessagesRecv,
						stats.Traces.BytesRecv)
				}
			case sig := <-sigCh:
				log.Printf("Received signal %v, shutting down", sig)
				return
			}
		}
	}()

	<-sigCh

	if err := server.Stop(); err != nil {
		log.Printf("Error during shutdown: %v", err)
	}

	log.Printf("Telemetry gateway stopped")
	return nil
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
