package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"google.golang.org/grpc"

	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
	"github.com/shawnbutts/keystone-core/pkg/api/server"
	"github.com/shawnbutts/keystone-core/pkg/config"
	"github.com/shawnbutts/keystone-core/pkg/controlplane"
	natsmgr "github.com/shawnbutts/keystone-core/pkg/nats"
	"github.com/shawnbutts/keystone-core/pkg/state"
	"github.com/shawnbutts/keystone-core/pkg/version"
)

var (
	cfgFile string
	rootCmd = &cobra.Command{
		Use:   "kscore-server",
		Short: "Keystone Core control plane server",
		Long: `Keystone Core control plane server manages agents and provides
the API for remote execution, state management, and policy enforcement.`,
		Run: runServer,
	}
)

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./keystone-core.yaml)")
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

func runServer(cmd *cobra.Command, args []string) {
	// Print version
	info := version.Get()
	fmt.Printf("Starting %s\n", info.String())

	// Load configuration
	cfg, err := config.LoadConfig(cfgFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "Invalid configuration: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Configuration loaded successfully\n")
	fmt.Printf("  NATS Mode: %s\n", cfg.NATS.Mode)
	if cfg.NATS.Mode == config.NATSModeEmbedded {
		fmt.Printf("  Embedded NATS Port: %d\n", cfg.NATS.Embedded.Port)
		fmt.Printf("  JetStream Enabled: %v\n", cfg.NATS.Embedded.EnableJetStream)
	} else {
		fmt.Printf("  NATS URL: %s\n", cfg.NATS.URL)
	}
	fmt.Printf("  Storage Backend: %s\n", cfg.Storage.Backend)

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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
	} else {
		fmt.Printf("Connected to external NATS at %s\n", cfg.NATS.URL)
	}

	// Health check
	if err := natsManager.Health(); err != nil {
		fmt.Fprintf(os.Stderr, "NATS health check failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("NATS health check passed")

	// Initialize state store
	fmt.Println("Initializing state storage...")
	storeConfig := &state.Config{
		Backend:           string(cfg.Storage.Backend),
		SQLitePath:        cfg.Storage.SQLite.Path,
		SQLiteWAL:         cfg.Storage.SQLite.WAL,
		SQLiteBusyTimeout: cfg.Storage.SQLite.BusyTimeout,
	}
	stateStore, err := state.NewStore(storeConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create state store: %v\n", err)
		os.Exit(1)
	}
	defer stateStore.Close()

	// Test database connection
	if err := stateStore.Ping(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect to database: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("State storage initialized (%s)\n", cfg.Storage.Backend)

	// Initialize connection manager
	fmt.Println("Initializing connection manager...")
	connMgr := controlplane.NewConnectionManager(natsManager)
	if err := connMgr.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start connection manager: %v\n", err)
		os.Exit(1)
	}
	defer connMgr.Stop()

	// Initialize command dispatcher
	fmt.Println("Initializing command dispatcher...")
	dispatcher := controlplane.NewCommandDispatcher(connMgr, stateStore)

	// Initialize batch dispatcher
	fmt.Println("Initializing batch dispatcher...")
	batchDispatcher := controlplane.NewBatchDispatcher(connMgr, dispatcher, stateStore)

	// Initialize gRPC API server
	fmt.Println("Initializing gRPC API server...")
	grpcServer := grpc.NewServer()
	apiServer := server.NewControlPlaneServer(connMgr, dispatcher, batchDispatcher, stateStore)
	pb.RegisterControlPlaneServiceServer(grpcServer, apiServer)

	// Start gRPC server
	listenAddr := fmt.Sprintf("%s:%d", cfg.Server.ListenAddr, cfg.Server.GRPCPort)
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to listen on %s: %v\n", listenAddr, err)
		os.Exit(1)
	}

	go func() {
		fmt.Printf("gRPC API server listening on %s\n", listenAddr)
		if err := grpcServer.Serve(listener); err != nil {
			fmt.Fprintf(os.Stderr, "gRPC server error: %v\n", err)
		}
	}()
	defer grpcServer.GracefulStop()

	fmt.Printf("\nKeystone Core server started successfully\n")
	fmt.Printf("  gRPC API: %s\n", listenAddr)
	fmt.Printf("  Storage: %s\n", cfg.Storage.Backend)
	fmt.Println("\nWaiting for agent connections...")

	// Status reporting loop
	statusTicker := time.NewTicker(30 * time.Second)
	defer statusTicker.Stop()

	// Wait for termination signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	for {
		select {
		case <-statusTicker.C:
			total := connMgr.GetAgentCount()
			online := connMgr.GetOnlineAgentCount()
			fmt.Printf("[%s] Agents: %d total, %d online\n", time.Now().Format("15:04:05"), total, online)
		case <-sigChan:
			fmt.Println("\nReceived shutdown signal, shutting down gracefully...")
			goto shutdown
		case <-ctx.Done():
			fmt.Println("\nContext cancelled, shutting down...")
			goto shutdown
		}
	}

shutdown:
	// Graceful shutdown
	fmt.Println("Stopping gRPC server...")
	grpcServer.GracefulStop()

	fmt.Println("Stopping connection manager...")
	if err := connMgr.Stop(); err != nil {
		fmt.Fprintf(os.Stderr, "Error stopping connection manager: %v\n", err)
	}

	fmt.Println("Closing state store...")
	if err := stateStore.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "Error closing state store: %v\n", err)
	}

	fmt.Println("Shutting down NATS manager...")
	if err := natsManager.Shutdown(); err != nil {
		fmt.Fprintf(os.Stderr, "Error during shutdown: %v\n", err)
	}

	fmt.Println("Server shutdown complete")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
