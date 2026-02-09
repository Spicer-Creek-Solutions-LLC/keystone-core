// Package main implements the kscore-server control plane daemon.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"google.golang.org/grpc"

	"github.com/shawnbutts/keystone-core/internal/config"
	"github.com/shawnbutts/keystone-core/internal/controlplane"
	"github.com/shawnbutts/keystone-core/internal/events"
	"github.com/shawnbutts/keystone-core/internal/gitops/webhook"
	"github.com/shawnbutts/keystone-core/internal/logging"
	natsmgr "github.com/shawnbutts/keystone-core/internal/nats"
	"github.com/shawnbutts/keystone-core/internal/policy"
	"github.com/shawnbutts/keystone-core/internal/state"
	"github.com/shawnbutts/keystone-core/pkg/api/auth"
	"github.com/shawnbutts/keystone-core/pkg/api/server"
	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
	"github.com/shawnbutts/keystone-core/pkg/version"
)

// logger is the structured logger for kscore-server (Epic 15)
var logger logging.Logger

var cfgFile string

func newRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "kscore-server",
		Short: "Keystone Core control plane server",
		Long: `Keystone Core control plane server manages agents and provides
the API for remote execution, state management, and policy enforcement.`,
		Run:           runServer,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./keystone-core.yaml)")
	rootCmd.AddCommand(newVersionCmd())

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

func runServer(cmd *cobra.Command, args []string) {
	// Capture server start time for uptime calculation
	startTime := time.Now()

	// Initialize logger early (use env vars before config is loaded)
	logger = logging.InitDefaultLogger("kscore-server")

	// Get version info
	info := version.Get()
	logger.Info("Starting Keystone Core server",
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

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		logger.Error("Invalid configuration", logging.Error(err))
		os.Exit(1)
	}

	// Re-initialize logger with config settings (Epic 15)
	logger = logging.InitDefaultLoggerFromConfig(&cfg.Logging, "kscore-server")

	logger.Info("Configuration loaded successfully")
	if cfg.NATS.Mode == config.NATSModeEmbedded {
		logger.Info("NATS configuration",
			logging.String("mode", string(cfg.NATS.Mode)),
			logging.Int("port", cfg.NATS.Embedded.Port),
			logging.Bool("jetstream", cfg.NATS.Embedded.EnableJetStream),
		)
	} else {
		logger.Info("NATS configuration",
			logging.String("mode", string(cfg.NATS.Mode)),
			logging.String("url", cfg.NATS.URL),
		)
	}
	logger.Info("Storage configuration", logging.String("backend", string(cfg.Storage.Backend)))

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize NATS manager
	logger.Info("Initializing NATS manager")
	natsManager, err := natsmgr.NewManager(&cfg.NATS)
	if err != nil {
		logger.Error("Failed to create NATS manager", logging.Error(err))
		return
	}

	// Start NATS manager
	if err := natsManager.Start(); err != nil {
		logger.Error("Failed to start NATS manager", logging.Error(err))
		return
	}
	defer natsManager.Shutdown()

	if natsManager.IsEmbedded() {
		logger.Info("Embedded NATS server started", logging.Int("port", cfg.NATS.Embedded.Port))
	} else {
		logger.Info("Connected to external NATS", logging.String("url", cfg.NATS.URL))
	}

	// Health check
	if err := natsManager.Health(); err != nil {
		logger.Error("NATS health check failed", logging.Error(err))
		return
	}
	logger.Info("NATS health check passed")

	// Initialize state store
	logger.Info("Initializing state storage")
	storeConfig := &state.Config{
		Backend:               string(cfg.Storage.Backend),
		SQLitePath:            cfg.Storage.SQLite.Path,
		SQLiteWAL:             cfg.Storage.SQLite.WAL,
		SQLiteBusyTimeout:     cfg.Storage.SQLite.BusyTimeout,
		PostgreSQLDSN:         cfg.Storage.PostgreSQL.DSN,
		PostgreSQLMaxOpen:     cfg.Storage.PostgreSQL.MaxOpenConns,
		PostgreSQLMaxIdle:     cfg.Storage.PostgreSQL.MaxIdleConns,
		PostgreSQLConnMaxLife: cfg.Storage.PostgreSQL.ConnMaxLifetime,
	}
	stateStore, err := state.NewStore(storeConfig)
	if err != nil {
		logger.Error("Failed to create state store", logging.Error(err))
		return
	}
	defer stateStore.Close()

	// Test database connection
	if err := stateStore.Ping(ctx); err != nil {
		logger.Error("Failed to connect to database", logging.Error(err))
		return
	}
	logger.Info("State storage initialized", logging.String("backend", string(cfg.Storage.Backend)))

	// Initialize connection manager
	logger.Info("Initializing connection manager")
	connMgr := controlplane.NewConnectionManager(natsManager)

	// Set state store for HA cluster support (allows loading agents from database)
	connMgr.SetStateStore(&stateStoreAdapter{store: stateStore})

	if err := connMgr.Start(); err != nil {
		logger.Error("Failed to start connection manager", logging.Error(err))
		return
	}
	defer connMgr.Stop()

	// Initialize command dispatcher
	logger.Info("Initializing command dispatcher")
	dispatcher := controlplane.NewCommandDispatcher(connMgr, stateStore)
	if err := dispatcher.Start(); err != nil {
		logger.Error("Failed to start command dispatcher", logging.Error(err))
		return
	}
	defer dispatcher.Stop()

	// Initialize batch dispatcher
	logger.Info("Initializing batch dispatcher")
	batchDispatcher := controlplane.NewBatchDispatcher(connMgr, dispatcher, stateStore)

	// Initialize gRPC API server
	logger.Info("Initializing gRPC API server")

	// Configure gRPC server options
	var grpcOpts []grpc.ServerOption

	// Set up authentication if enabled
	if cfg.Auth.Enabled {
		logger.Info("Initializing API authentication", logging.String("type", cfg.Auth.Type))

		authCfg, err := auth.NewInterceptorConfigFromConfig(cfg.Auth)
		if err != nil {
			logger.Error("Failed to initialize authentication", logging.Error(err))
			return
		}

		// Add audit logging for auth events (using structured logger)
		authCfg.AuditLogger = func(ctx context.Context, method string, principal *auth.Principal, authErr error) {
			if authErr != nil {
				logger.Warn("Authentication denied",
					logging.String("method", method),
					logging.Error(authErr),
				)
			} else if principal != nil {
				logger.Debug("Authentication allowed",
					logging.String("method", method),
					logging.String("principal", principal.Name),
					logging.String("role", string(principal.Role)),
				)
			}
		}

		// Add auth interceptors to gRPC server
		grpcOpts = append(grpcOpts,
			grpc.UnaryInterceptor(auth.UnaryServerInterceptor(authCfg)),
			grpc.StreamInterceptor(auth.StreamServerInterceptor(authCfg)),
		)

		keyCount := len(cfg.Auth.APIKey.Keys)
		logger.Info("API authentication enabled",
			logging.Int("key_count", keyCount),
			logging.Any("bypass_methods", cfg.Auth.BypassMethods),
		)
	} else {
		logger.Warn("API authentication is DISABLED - all requests will be allowed")
		logger.Warn("This is insecure for production use. Set auth.enabled=true in config.")
	}

	grpcServer := grpc.NewServer(grpcOpts...)
	apiServer := server.NewControlPlaneServer(connMgr, dispatcher, batchDispatcher, stateStore)
	pb.RegisterControlPlaneServiceServer(grpcServer, apiServer)

	// Start gRPC server with IPv6 support
	grpcListenAddrs := cfg.Server.GetEffectiveListenAddrs()
	grpcListenerCfg := &server.ListenerConfig{
		Addresses:     grpcListenAddrs,
		DefaultPort:   cfg.Server.GRPCPort,
		AddressFamily: cfg.Server.GetAddressFamilyPreference(),
	}
	grpcListeners, err := server.CreateListeners(grpcListenerCfg)
	if err != nil {
		logger.Error("Failed to create gRPC listeners", logging.Error(err))
		return
	}

	// Start gRPC server on all configured listeners
	for _, lr := range grpcListeners {
		go func(listener *server.ListenerResult) {
			ipVersion := "IPv4"
			if listener.IsIPv6 {
				ipVersion = "IPv6"
			}
			logger.Info("gRPC API server listening",
				logging.String("address", listener.Address),
				logging.String("ip_version", ipVersion),
			)
			if err := grpcServer.Serve(listener.Listener); err != nil {
				logger.Error("gRPC server error", logging.Error(err))
			}
		}(lr)
	}
	defer grpcServer.GracefulStop()

	// Start HTTP health server with IPv6 support
	httpMux := http.NewServeMux()

	// Health endpoints
	httpMux.HandleFunc("/health/live", func(w http.ResponseWriter, r *http.Request) {
		writeJSONResponse(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	httpMux.HandleFunc("/health/ready", func(w http.ResponseWriter, r *http.Request) {
		// Check if NATS is healthy
		if err := natsManager.Health(); err != nil {
			writeJSONResponse(w, http.StatusServiceUnavailable, map[string]string{
				"status": "not ready",
				"error":  err.Error(),
			})
			return
		}
		writeJSONResponse(w, http.StatusOK, map[string]string{"status": "ready"})
	})
	httpMux.HandleFunc("/health/status", func(w http.ResponseWriter, r *http.Request) {
		total := connMgr.GetAgentCount()
		online := connMgr.GetOnlineAgentCount()
		writeJSONResponse(w, http.StatusOK, map[string]interface{}{
			"status": "ok",
			"agents": map[string]int{
				"total":  total,
				"online": online,
			},
		})
	})

	// Server status endpoint for monitor TUI and other tools
	httpMux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		status := buildServerStatusResponse(connMgr, startTime)
		writeJSONResponse(w, http.StatusOK, status)
	})

	// Create HTTP listeners for each address
	httpListenerCfg := &server.ListenerConfig{
		Addresses:     grpcListenAddrs, // Use same addresses as gRPC
		DefaultPort:   cfg.Server.HTTPPort,
		AddressFamily: cfg.Server.GetAddressFamilyPreference(),
	}
	httpListeners, err := server.CreateListeners(httpListenerCfg)
	if err != nil {
		logger.Error("Failed to create HTTP listeners", logging.Error(err))
		return
	}

	// Track HTTP servers for graceful shutdown
	var httpServers []*http.Server
	for _, lr := range httpListeners {
		httpServer := &http.Server{
			Handler:           httpMux,
			ReadTimeout:       10 * time.Second,
			ReadHeaderTimeout: 10 * time.Second,
			WriteTimeout:      10 * time.Second,
			IdleTimeout:       120 * time.Second,
		}
		httpServers = append(httpServers, httpServer)

		go func(srv *http.Server, listener *server.ListenerResult) {
			ipVersion := "IPv4"
			if listener.IsIPv6 {
				ipVersion = "IPv6"
			}
			logger.Info("HTTP health server listening",
				logging.String("address", listener.Address),
				logging.String("ip_version", ipVersion),
			)
			if err := srv.Serve(listener.Listener); err != nil && err != http.ErrServerClosed {
				logger.Error("HTTP server error", logging.Error(err))
			}
		}(httpServer, lr)
	}
	defer func() {
		for _, srv := range httpServers {
			srv.Shutdown(context.Background())
		}
	}()

	// Initialize policy engine if enabled
	var policyEngine *policy.PolicyEngine
	var policyRegistry *policy.Registry
	if cfg.Policy.Enabled {
		logger.Info("Initializing policy engine")
		policyRegistry = policy.NewRegistry()

		// Register configured policies
		for _, policyDef := range cfg.Policy.Policies {
			if !policyDef.Enabled {
				continue
			}
			p := &policy.Policy{
				ID:          policyDef.ID,
				Name:        policyDef.Name,
				Description: policyDef.Description,
				Type:        policy.PolicyType(policyDef.Type),
				Category:    policy.PolicyCategory(policyDef.Category),
				Severity:    policy.Severity(policyDef.Severity),
				Policy:      policyDef.Code,
				Enabled:     policyDef.Enabled,
			}
			if err := policyRegistry.RegisterPolicy(p); err != nil {
				logger.Error("Failed to register policy",
					logging.String("policy_id", policyDef.ID),
					logging.Error(err),
				)
			} else {
				logger.Info("Registered policy",
					logging.String("policy_id", policyDef.ID),
					logging.String("type", policyDef.Type),
				)
			}
		}

		policyEngine = policy.NewPolicyEngine(policyRegistry)
		logger.Info("Policy engine initialized",
			logging.String("engine", cfg.Policy.Engine),
			logging.String("mode", cfg.Policy.EnforcementMode),
		)

		// Store policy engine in API server for use
		apiServer.SetPolicyEngine(policyEngine)
	}

	// Initialize webhook receiver if enabled
	var webhookReceiver *webhook.Receiver
	if cfg.Webhook.Enabled {
		logger.Info("Initializing webhook receiver")

		// Create webhook config
		webhookConfig := &webhook.Config{
			Enabled:  true,
			Addr:     fmt.Sprintf(":%d", cfg.Webhook.Port),
			Path:     cfg.Webhook.Path,
			Handlers: cfg.Webhook.Handlers,
		}

		// Set authentication
		switch cfg.Webhook.AuthType {
		case "hmac":
			webhookConfig.Auth = webhook.AuthConfig{
				Type:   webhook.AuthTypeHMAC,
				Secret: cfg.Webhook.HMACSecret,
			}
		case "bearer":
			webhookConfig.Auth = webhook.AuthConfig{
				Type:  webhook.AuthTypeBearer,
				Token: cfg.Webhook.BearerToken,
			}
		default:
			webhookConfig.Auth = webhook.AuthConfig{
				Type: webhook.AuthTypeNone,
			}
		}

		var processor webhook.EventProcessor = &loggingEventProcessor{logger: logger}
		if js := natsManager.JetStream(); js != nil {
			publisher, err := events.NewJetStreamPublisher(js)
			if err != nil {
				logger.Error("Failed to initialize webhook event publisher", logging.Error(err))
			} else {
				processor = webhook.NewEventBusProcessor(publisher)
				defer publisher.Close()
			}
		} else {
			logger.Warn("JetStream unavailable; webhook events will be logged only")
		}
		webhookReceiver = webhook.NewReceiver(webhookConfig, processor)

		go func() {
			webhookAddr := fmt.Sprintf("%s:%d", cfg.Server.ListenAddr, cfg.Webhook.Port)
			logger.Info("Webhook receiver listening",
				logging.String("address", webhookAddr),
				logging.String("path", cfg.Webhook.Path),
			)
			if err := webhookReceiver.Start(); err != nil {
				logger.Error("Webhook receiver error", logging.Error(err))
			}
		}()
		defer webhookReceiver.Stop(context.Background())
	}

	// Build address list for logging
	var grpcAddrs, httpAddrs []string
	for _, lr := range grpcListeners {
		grpcAddrs = append(grpcAddrs, lr.Address)
	}
	for _, lr := range httpListeners {
		httpAddrs = append(httpAddrs, lr.Address)
	}

	logger.Info("Keystone Core server started successfully",
		logging.Any("grpc_api", grpcAddrs),
		logging.Any("http_api", httpAddrs),
		logging.String("storage", string(cfg.Storage.Backend)),
		logging.Bool("auth_enabled", cfg.Auth.Enabled),
	)
	logger.Info("Waiting for agent connections")

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
			logger.Debug("Agent status",
				logging.Int("total", total),
				logging.Int("online", online),
			)
		case <-sigChan:
			logger.Info("Received shutdown signal, shutting down gracefully")
			goto shutdown
		case <-ctx.Done():
			logger.Info("Context cancelled, shutting down")
			goto shutdown
		}
	}

shutdown:
	// Graceful shutdown
	logger.Info("Stopping gRPC server")
	grpcServer.GracefulStop()

	logger.Info("Stopping connection manager")
	if err := connMgr.Stop(); err != nil {
		logger.Error("Error stopping connection manager", logging.Error(err))
	}

	logger.Info("Closing state store")
	if err := stateStore.Close(); err != nil {
		logger.Error("Error closing state store", logging.Error(err))
	}

	logger.Info("Shutting down NATS manager")
	if err := natsManager.Shutdown(); err != nil {
		logger.Error("Error during shutdown", logging.Error(err))
	}

	logger.Info("Server shutdown complete")
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// loggingEventProcessor logs webhook events (simple implementation for now)
type loggingEventProcessor struct {
	logger logging.Logger
}

func (p *loggingEventProcessor) ProcessEvent(ctx context.Context, event *webhook.Event) error {
	p.logger.Info("Webhook event received",
		logging.String("type", string(event.Type)),
		logging.String("source", event.Source),
	)
	return nil
}

// stateStoreAdapter adapts state.AgentStore to controlplane.AgentStore.
type stateStoreAdapter struct {
	store state.AgentStore
}

func (a *stateStoreAdapter) ListAgents(ctx context.Context, filter *controlplane.AgentFilter) ([]controlplane.StoredAgent, error) {
	var stateFilter *state.AgentFilter
	if filter != nil && filter.Status != "" {
		// Convert status string to pb.AgentStatus if needed
		stateFilter = &state.AgentFilter{}
	}

	records, err := a.store.ListAgents(ctx, stateFilter)
	if err != nil {
		return nil, err
	}

	agents := make([]controlplane.StoredAgent, len(records))
	for i, r := range records {
		agents[i] = controlplane.StoredAgent{
			ID:           r.ID,
			Hostname:     r.Hostname,
			OS:           r.OS,
			Arch:         r.Architecture,
			Status:       r.Status.String(),
			Labels:       r.Labels,
			IPAddresses:  r.IPAddresses,
			RegisteredAt: r.RegisteredAt,
			LastSeen:     r.LastHeartbeat,
		}
	}
	return agents, nil
}

func (a *stateStoreAdapter) GetAgent(ctx context.Context, agentID string) (*controlplane.StoredAgent, error) {
	r, err := a.store.GetAgent(ctx, agentID)
	if err != nil {
		return nil, err
	}

	return &controlplane.StoredAgent{
		ID:           r.ID,
		Hostname:     r.Hostname,
		OS:           r.OS,
		Arch:         r.Architecture,
		Status:       r.Status.String(),
		Labels:       r.Labels,
		IPAddresses:  r.IPAddresses,
		RegisteredAt: r.RegisteredAt,
		LastSeen:     r.LastHeartbeat,
	}, nil
}

func (a *stateStoreAdapter) SaveAgent(ctx context.Context, agent *controlplane.StoredAgent) error {
	// Convert status string to pb.AgentStatus
	status := pb.AgentStatus_AGENT_STATUS_ONLINE
	switch agent.Status {
	case "offline":
		status = pb.AgentStatus_AGENT_STATUS_OFFLINE
	case "degraded":
		status = pb.AgentStatus_AGENT_STATUS_DEGRADED
	}

	record := &state.AgentRecord{
		ID:            agent.ID,
		Hostname:      agent.Hostname,
		OS:            agent.OS,
		Architecture:  agent.Arch,
		Status:        status,
		Labels:        agent.Labels,
		IPAddresses:   agent.IPAddresses,
		RegisteredAt:  agent.RegisteredAt,
		LastHeartbeat: agent.LastSeen,
		UpdatedAt:     time.Now(),
	}

	return a.store.SaveAgent(ctx, record)
}

// buildServerStatusResponse returns server status for the /api/status endpoint.
// This provides version, uptime, memory, goroutines, and agent statistics.
func buildServerStatusResponse(connMgr *controlplane.ConnectionManager, startTime time.Time) serverStatusResponse {
	// Get memory stats
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// Calculate uptime
	uptime := time.Since(startTime)
	uptimeSeconds := int64(uptime.Seconds())

	// Get version info
	info := version.Get()

	// Get agent counts
	totalAgents := connMgr.GetAgentCount()
	onlineAgents := connMgr.GetOnlineAgentCount()

	// Get goroutine count
	goroutines := runtime.NumGoroutine()

	// Memory usage in MB
	memoryMB := float64(memStats.Alloc) / 1024 / 1024

	return serverStatusResponse{
		Version:       info.Version,
		GitCommit:     info.GitCommit,
		BuildDate:     info.BuildDate,
		UptimeSeconds: uptimeSeconds,
		StartedAt:     startTime.UTC().Format(time.RFC3339),
		Agents: serverStatusAgents{
			Total:   totalAgents,
			Online:  onlineAgents,
			Offline: totalAgents - onlineAgents,
		},
		Runtime: serverStatusRuntime{
			Goroutines:    goroutines,
			MemoryAllocMB: memoryMB,
			MemorySysMB:   float64(memStats.Sys) / 1024 / 1024,
			GCRuns:        memStats.NumGC,
		},
		Health: "healthy",
	}
}

type serverStatusResponse struct {
	Version       string              `json:"version"`
	GitCommit     string              `json:"git_commit"`
	BuildDate     string              `json:"build_date"`
	UptimeSeconds int64               `json:"uptime_seconds"`
	StartedAt     string              `json:"started_at"`
	Agents        serverStatusAgents  `json:"agents"`
	Runtime       serverStatusRuntime `json:"runtime"`
	Health        string              `json:"health"`
}

type serverStatusAgents struct {
	Total   int `json:"total"`
	Online  int `json:"online"`
	Offline int `json:"offline"`
}

type serverStatusRuntime struct {
	Goroutines    int     `json:"goroutines"`
	MemoryAllocMB float64 `json:"memory_alloc_mb"`
	MemorySysMB   float64 `json:"memory_sys_mb"`
	GCRuns        uint32  `json:"gc_runs"`
}

func writeJSONResponse(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		logger.Error("Failed to encode JSON response", logging.Error(err))
	}
}
