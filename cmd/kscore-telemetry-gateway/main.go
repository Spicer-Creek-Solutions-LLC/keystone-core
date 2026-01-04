// Package main is the entry point for the Keystone Core telemetry gateway.
//
// The telemetry gateway aggregates metrics, logs, and traces from agents over NATS
// and exposes them to observability backends (Prometheus, Loki, Tempo/Jaeger).
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/shawnbutts/keystone-core/pkg/gateway"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"
)

func main() {
	// Parse command line flags
	configFile := flag.String("config", "", "Path to configuration file")
	listenAddr := flag.String("listen", "", "Listen address (e.g., 0.0.0.0:9091)")
	natsURL := flag.String("nats-url", "", "NATS server URL")
	metricsEnabled := flag.Bool("metrics", true, "Enable metrics gateway")
	logsEnabled := flag.Bool("logs", true, "Enable logs gateway")
	tracesEnabled := flag.Bool("traces", true, "Enable traces gateway")
	showVersion := flag.Bool("version", false, "Show version information")

	flag.Parse()

	if *showVersion {
		fmt.Printf("kscore-telemetry-gateway %s\n", version)
		fmt.Printf("  commit: %s\n", commit)
		fmt.Printf("  built: %s\n", buildDate)
		os.Exit(0)
	}

	// Load configuration
	config := gateway.DefaultConfig()

	if *configFile != "" {
		data, err := os.ReadFile(*configFile)
		if err != nil {
			log.Fatalf("Failed to read config file: %v", err)
		}
		if err := yaml.Unmarshal(data, config); err != nil {
			log.Fatalf("Failed to parse config file: %v", err)
		}
		log.Printf("Loaded configuration from %s", *configFile)
	}

	// Override with command line flags
	if *listenAddr != "" {
		config.Server.Listen = *listenAddr
	}
	if *natsURL != "" {
		config.NATS.URLs = []string{*natsURL}
	}
	config.Metrics.Enabled = *metricsEnabled
	config.Logs.Enabled = *logsEnabled
	config.Traces.Enabled = *tracesEnabled

	// Create and start the server
	server := gateway.NewServer(config)

	if err := server.Start(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	log.Printf("Telemetry gateway started")
	log.Printf("  Listen: %s", config.Server.Listen)
	log.Printf("  Metrics: %v (subject: %s)", config.Metrics.Enabled, config.Metrics.Subject)
	log.Printf("  Logs: %v (subject: %s)", config.Logs.Enabled, config.Logs.Subject)
	log.Printf("  Traces: %v (subject: %s)", config.Traces.Enabled, config.Traces.Subject)

	// Print endpoints
	log.Printf("Endpoints:")
	log.Printf("  GET %s - Prometheus metrics", config.Server.MetricsPath)
	log.Printf("  GET %s - Health check", config.Server.HealthPath)
	log.Printf("  GET %s - Readiness check", config.Server.ReadyPath)
	if config.Metrics.Federation.Enabled {
		log.Printf("  GET %s - Prometheus federation", config.Server.FederatePath)
	}

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Stats logging
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
}
