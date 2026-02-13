// Package main implements the kscore-server control plane daemon.
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/shawnbutts/keystone-core/internal/cluster"
	"github.com/shawnbutts/keystone-core/internal/config"
	"github.com/shawnbutts/keystone-core/internal/controlplane"
	"github.com/shawnbutts/keystone-core/internal/events"
	"github.com/shawnbutts/keystone-core/internal/files/mirror"
	"github.com/shawnbutts/keystone-core/internal/gitops/rollback"
	"github.com/shawnbutts/keystone-core/internal/gitops/verification"
	"github.com/shawnbutts/keystone-core/internal/gitops/webhook"
	k8soperator "github.com/shawnbutts/keystone-core/internal/k8s"
	"github.com/shawnbutts/keystone-core/internal/webhook/outbound"
	"github.com/shawnbutts/keystone-core/internal/logging"
	"github.com/shawnbutts/keystone-core/internal/metrics"
	natsmgr "github.com/shawnbutts/keystone-core/internal/nats"
	"github.com/shawnbutts/keystone-core/internal/policy"
	"github.com/shawnbutts/keystone-core/internal/profiling"
	"github.com/shawnbutts/keystone-core/internal/proxy/discovery"
	"github.com/shawnbutts/keystone-core/internal/ratelimit"
	"github.com/shawnbutts/keystone-core/internal/runbook/approval"
	"github.com/shawnbutts/keystone-core/internal/runbook/intervention"
	"github.com/shawnbutts/keystone-core/pkg/api/apierror"
	"github.com/shawnbutts/keystone-core/internal/state"
	"github.com/shawnbutts/keystone-core/internal/statemgmt"
	internaltracing "github.com/shawnbutts/keystone-core/internal/tracing"
	"github.com/shawnbutts/keystone-core/pkg/api/agents"
	apiapikeys "github.com/shawnbutts/keystone-core/pkg/api/apikeys"
	"github.com/shawnbutts/keystone-core/pkg/api/auth"
	apiconfig "github.com/shawnbutts/keystone-core/pkg/api/config"
	apievents "github.com/shawnbutts/keystone-core/pkg/api/events"
	"github.com/shawnbutts/keystone-core/pkg/api/execution"
	apigitops "github.com/shawnbutts/keystone-core/pkg/api/gitops"
	"github.com/shawnbutts/keystone-core/pkg/api/maintenance"
	apipolicy "github.com/shawnbutts/keystone-core/pkg/api/policy"
	apirbac "github.com/shawnbutts/keystone-core/pkg/api/rbac"
	apirunbook "github.com/shawnbutts/keystone-core/pkg/api/runbook"
	apicluster "github.com/shawnbutts/keystone-core/pkg/api/cluster"
	apiwebhooks "github.com/shawnbutts/keystone-core/pkg/api/webhooks"
	apisecrets "github.com/shawnbutts/keystone-core/pkg/api/secrets"
	"github.com/shawnbutts/keystone-core/pkg/api/server"
	apistate "github.com/shawnbutts/keystone-core/pkg/api/state"
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

	// Initialize connection manager with config
	logger.Info("Initializing connection manager")
	connMgrCfg := &controlplane.ConnectionManagerConfig{
		HeartbeatTimeout:       cfg.AgentManagement.HeartbeatTimeout,
		StaleThreshold:         cfg.AgentManagement.StaleThreshold,
		MonitorInterval:        cfg.AgentManagement.MonitorInterval,
		AgentHeartbeatInterval: cfg.AgentManagement.HeartbeatInterval,
		AgentCommandTimeout:    cfg.Execution.DefaultTimeout,
		AgentMetadataInterval:  cfg.AgentManagement.MetadataRefresh,
		MaxConcurrentCommands:  cfg.AgentManagement.MaxConcurrentCommands,
	}
	connMgr := controlplane.NewConnectionManagerWithConfig(natsManager, connMgrCfg)

	// Set state store for HA cluster support (allows loading agents from database)
	connMgr.SetStateStore(&stateStoreAdapter{store: stateStore})

	if err := connMgr.Start(); err != nil {
		logger.Error("Failed to start connection manager", logging.Error(err))
		return
	}
	defer connMgr.Stop()

	// Initialize command dispatcher with config
	logger.Info("Initializing command dispatcher")
	cmdDispCfg := &controlplane.CommandDispatcherConfig{
		DefaultTimeout:  cfg.Execution.DefaultTimeout,
		MaxTimeout:      cfg.Execution.MaxTimeout,
		StreamingBuffer: cfg.Execution.StreamingBuffer,
		ResultRetention: cfg.Execution.ResultRetention,
	}
	dispatcher := controlplane.NewCommandDispatcherWithConfig(connMgr, stateStore, cmdDispCfg)
	if err := dispatcher.Start(); err != nil {
		logger.Error("Failed to start command dispatcher", logging.Error(err))
		return
	}
	defer dispatcher.Stop()

	// Initialize batch dispatcher with config
	logger.Info("Initializing batch dispatcher")
	batchDispCfg := &controlplane.BatchDispatcherConfig{
		DefaultBatchSize: cfg.Execution.BatchSize,
		BatchDelay:       cfg.Execution.BatchDelay,
	}
	batchDispatcher := controlplane.NewBatchDispatcherWithConfig(connMgr, dispatcher, stateStore, batchDispCfg)

	// Initialize tracing if enabled
	if cfg.Tracing.Enabled {
		tracingCfg := tracingConfigFromServerConfig(cfg.Tracing)
		tracingProvider, err := internaltracing.NewProvider(tracingCfg)
		if err != nil {
			logger.Error("Failed to initialize tracing", logging.Error(err))
			return
		}
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := tracingProvider.Shutdown(shutdownCtx); err != nil {
				logger.Error("Error shutting down tracing provider", logging.Error(err))
			}
		}()
		logger.Info("Tracing enabled",
			logging.String("exporter", cfg.Tracing.Exporter),
			logging.String("endpoint", cfg.Tracing.Endpoint),
			logging.String("sampling_strategy", cfg.Tracing.Sampling.Strategy),
		)
	}

	// Start profiling server if enabled
	if cfg.Profiling.Enabled {
		profServer := profiling.NewServer(&profiling.ProfileConfig{
			Enabled:              true,
			ListenAddr:           cfg.Profiling.Listen,
			BlockProfileRate:     cfg.Profiling.BlockProfileRate,
			MutexProfileFraction: cfg.Profiling.MutexProfileFraction,
		})
		if err := profServer.Start(ctx); err != nil {
			logger.Error("Failed to start profiling server", logging.Error(err))
			return
		}
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := profServer.Stop(shutdownCtx); err != nil {
				logger.Error("Error shutting down profiling server", logging.Error(err))
			}
		}()
		logger.Info("Profiling server started", logging.String("listen", cfg.Profiling.Listen))
	}

	// Initialize gRPC API server
	logger.Info("Initializing gRPC API server")

	// Configure gRPC server options
	var grpcOpts []grpc.ServerOption

	// Build TLS config for API servers if enabled
	var serverTLSConfig *tls.Config
	if cfg.TLS.Enabled {
		var err error
		serverTLSConfig, err = buildTLSConfig(cfg.TLS)
		if err != nil {
			logger.Error("Failed to build TLS configuration", logging.Error(err))
			return
		}
		grpcOpts = append(grpcOpts, grpc.Creds(credentials.NewTLS(serverTLSConfig)))
		logger.Info("TLS enabled for API servers",
			logging.String("cert_file", cfg.TLS.CertFile),
			logging.String("min_version", cfg.TLS.MinVersion),
		)
	}

	// Set up authentication if enabled
	var authInterceptorCfg *auth.InterceptorConfig
	if cfg.Auth.Enabled {
		logger.Info("Initializing API authentication", logging.String("type", cfg.Auth.Type))

		var err error
		authInterceptorCfg, err = auth.NewInterceptorConfigFromConfig(cfg.Auth)
		if err != nil {
			logger.Error("Failed to initialize authentication", logging.Error(err))
			return
		}

		// Add audit logging for auth events (using structured logger)
		authInterceptorCfg.AuditLogger = func(ctx context.Context, method string, principal *auth.Principal, authErr error) {
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
			grpc.UnaryInterceptor(auth.UnaryServerInterceptor(authInterceptorCfg)),
			grpc.StreamInterceptor(auth.StreamServerInterceptor(authInterceptorCfg)),
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

	secretsServer := server.NewSecretsServer(nil, nil, nil)
	pb.RegisterSecretsServiceServer(grpcServer, secretsServer)

	agentServer := server.NewAgentServer(connMgr)
	pb.RegisterAgentServiceServer(grpcServer, agentServer)

	stateExecutor := statemgmt.NewExecutor()
	stateDiffer := statemgmt.NewStateDiffer()
	stateServer := server.NewStateServer(stateExecutor, stateDiffer)
	pb.RegisterStateServiceServer(grpcServer, stateServer)

	// EventService — wire publisher and subscriber if JetStream is available
	var eventPublisher events.EventPublisher
	var eventSubscriber events.EventSubscriber
	if js := natsManager.JetStream(); js != nil {
		pubCfg := &events.JetStreamPublisherConfig{
			Retention:   cfg.Events.Retention,
			MaxBytes:    cfg.Events.MaxBytes,
			MaxMessages: cfg.Events.MaxMessages,
		}
		pub, pubErr := events.NewJetStreamPublisherWithConfig(js, pubCfg)
		if pubErr != nil {
			logger.Error("Failed to initialize event publisher for EventService", logging.Error(pubErr))
		} else {
			eventPublisher = pub
			defer pub.Close()
		}
		sub, subErr := events.NewJetStreamSubscriber(js)
		if subErr != nil {
			logger.Error("Failed to initialize event subscriber for EventService", logging.Error(subErr))
		} else {
			eventSubscriber = sub
			defer sub.Close()
		}
	}
	// Initialize SQLite event store for REST API and gRPC EventService
	var eventStore events.EventStore
	if cfg.Events.Enabled {
		esCfg := events.DefaultEventStoreConfig()
		esCfg.Path = deriveDataPath(cfg.Storage.SQLite.Path, "events.db")
		store, storeErr := events.NewSQLiteEventStore(esCfg)
		if storeErr != nil {
			logger.Error("Failed to initialize event store", logging.Error(storeErr))
		} else {
			eventStore = store
			defer store.Close()
		}
	}

	eventServer := server.NewEventServer(eventStore, eventPublisher, eventSubscriber)
	pb.RegisterEventServiceServer(grpcServer, eventServer)

	// ClusterService registered with nil deps — returns Unavailable until cluster mode is wired
	clusterServer := server.NewClusterServer(nil, nil, nil)
	pb.RegisterClusterServiceServer(grpcServer, clusterServer)

	// CoordinationService requires clustering infrastructure (MembershipManager, etcd).
	// It will be registered when cluster mode is enabled.
	logger.Info("CoordinationService available but not registered (requires cluster mode)")

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
	httpMux.HandleFunc("/health/ready", healthReadyHandler(cfg.Health, startTime, natsManager, stateStore))
	httpMux.HandleFunc("/health/status", healthStatusHandler(cfg.Health, startTime, natsManager, stateStore, connMgr))

	// Server status endpoint for monitor TUI and other tools
	httpMux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		status := buildServerStatusResponse(connMgr, startTime)
		writeJSONResponse(w, http.StatusOK, status)
	})

	// Register REST API handlers
	agents.NewHandler(connMgr).RegisterRoutes(httpMux)
	execution.NewHandler(dispatcher, connMgr).RegisterRoutes(httpMux)
	apistate.NewHandler().RegisterRoutes(httpMux)
	maintenance.NewHandler(maintenance.NewMemoryStore()).RegisterRoutes(httpMux)
	apiapikeys.NewHandler(apiapikeys.NewMemoryStore()).RegisterRoutes(httpMux)
	apiconfig.NewHandler(cfg).RegisterRoutes(httpMux)
	apirbac.NewHandler().RegisterRoutes(httpMux)
	apisecrets.NewHandler(nil, nil, nil, nil, nil).RegisterRoutes(httpMux)
	// Cluster handler: wired with real deps when clustering is enabled
	if cfg.Cluster.Enabled {
		clusterCfg := clusterConfigFromSettings(cfg.Cluster)
		etcdClient, etcdErr := cluster.NewEtcdClient(clusterCfg.Etcd, clusterCfg.MemberID)
		if etcdErr != nil {
			logger.Error("Failed to create etcd client for cluster handler", logging.Error(etcdErr))
			apicluster.NewHandler(nil, nil, nil, nil, nil).RegisterRoutes(httpMux)
		} else {
			defer etcdClient.Close()
			membership, _ := cluster.NewMembershipManager(clusterCfg, etcdClient)
			leader, _ := cluster.NewLeaderElector(clusterCfg, etcdClient, clusterCfg.MemberID)
			var health *cluster.HealthMonitor
			if membership != nil {
				health, _ = cluster.NewHealthMonitor(clusterCfg, membership, etcdClient, clusterCfg.MemberID)
			}
			apicluster.NewHandler(membership, leader, nil, health, clusterCfg).RegisterRoutes(httpMux)
			logger.Info("Registered REST handler", logging.String("handler", "cluster"), logging.Bool("enabled", true))
		}
	} else {
		apicluster.NewHandler(nil, nil, nil, nil, nil).RegisterRoutes(httpMux)
	}
	logger.Info("REST API handlers registered")

	// Register metrics endpoint if enabled
	if cfg.Metrics.Enabled {
		metricsCollector := metrics.NewPrometheusCollector()
		registry := metricsCollector.Registry()
		if cfg.Metrics.IncludeGoMetrics {
			registry.MustRegister(collectors.NewGoCollector())
		}
		if cfg.Metrics.IncludeProcessMetrics {
			registry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
		}
		metricsPath := cfg.Metrics.Path
		if metricsPath == "" {
			metricsPath = "/metrics"
		}
		httpMux.Handle(metricsPath, metricsCollector.Handler())
		logger.Info("Metrics endpoint registered",
			logging.String("path", metricsPath),
			logging.Bool("go_metrics", cfg.Metrics.IncludeGoMetrics),
			logging.Bool("process_metrics", cfg.Metrics.IncludeProcessMetrics),
		)
	}

	// Wrap HTTP mux with auth middleware if enabled
	var httpHandler http.Handler = httpMux
	if authInterceptorCfg != nil {
		httpHandler = httpAuthMiddleware(httpHandler, authInterceptorCfg, cfg.Auth)
		logger.Info("HTTP authentication middleware enabled")
	}

	// Wrap with rate limiting middleware if enabled
	if cfg.RateLimit.Enabled {
		limiter := newRateLimiterFromConfig(cfg.RateLimit)
		httpHandler = rateLimitMiddleware(httpHandler, limiter, cfg.RateLimit)
		logger.Info("HTTP rate limiting enabled",
			logging.Int("requests_per_minute", cfg.RateLimit.RequestsPerMinute),
			logging.Int("burst", cfg.RateLimit.Burst),
			logging.String("key_extractor", cfg.RateLimit.KeyExtractor),
		)
	}

	// Wrap with CORS middleware if enabled (outermost layer so preflight
	// responses are not subject to rate limiting or auth)
	if cfg.CORS.Enabled {
		httpHandler = corsMiddleware(httpHandler, cfg.CORS)
		logger.Info("CORS enabled",
			logging.Any("allowed_origins", cfg.CORS.AllowedOrigins),
		)
	}

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
			Handler:           httpHandler,
			ReadTimeout:       10 * time.Second,
			ReadHeaderTimeout: 10 * time.Second,
			WriteTimeout:      10 * time.Second,
			IdleTimeout:       120 * time.Second,
			TLSConfig:         serverTLSConfig, // nil when TLS disabled
		}
		httpServers = append(httpServers, httpServer)

		go func(srv *http.Server, listener *server.ListenerResult) {
			ipVersion := "IPv4"
			if listener.IsIPv6 {
				ipVersion = "IPv6"
			}
			proto := "HTTP"
			if serverTLSConfig != nil {
				proto = "HTTPS"
			}
			logger.Info(proto+" API server listening",
				logging.String("address", listener.Address),
				logging.String("ip_version", ipVersion),
			)
			var serveErr error
			if serverTLSConfig != nil {
				// Certs are already in TLSConfig, pass empty strings
				tlsListener := tls.NewListener(listener.Listener, srv.TLSConfig)
				serveErr = srv.Serve(tlsListener)
			} else {
				serveErr = srv.Serve(listener.Listener)
			}
			if serveErr != nil && serveErr != http.ErrServerClosed {
				logger.Error("HTTP server error", logging.Error(serveErr))
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

	// PolicyService — uses registry and engine (nil when policy is disabled)
	policyServer := server.NewPolicyServer(policyRegistry, policyEngine, nil, nil)
	pb.RegisterPolicyServiceServer(grpcServer, policyServer)

	// Initialize policy auditor and compliance reporter for REST API
	var policyAuditor *policy.Auditor
	var complianceReporter *policy.ComplianceReporter
	if cfg.Policy.Enabled {
		policyAuditor = policy.NewAuditor(10000)
		complianceReporter = policy.NewComplianceReporter(policyAuditor, policyRegistry)
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
			publisherCfg := &events.JetStreamPublisherConfig{
				Retention:   cfg.Events.Retention,
				MaxBytes:    cfg.Events.MaxBytes,
				MaxMessages: cfg.Events.MaxMessages,
			}
			publisher, err := events.NewJetStreamPublisherWithConfig(js, publisherCfg)
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

	// Initialize webhook handler registry for REST API
	var webhookRegistry *webhook.HandlerRegistry
	if cfg.Webhook.Enabled {
		webhookRegistry = webhook.NewHandlerRegistry()
	}

	// Start Kubernetes operator if enabled
	if cfg.Operator.Enabled {
		logger.Info("Initializing Kubernetes operator")
		k8sClient, err := k8soperator.NewClient(k8soperator.ClusterConfig{})
		if err != nil {
			logger.Error("Failed to create Kubernetes client", logging.Error(err))
			return
		}

		crdClient, err := k8soperator.NewCRDClient(k8sClient.RestConfig())
		if err != nil {
			logger.Error("Failed to create CRD client", logging.Error(err))
			return
		}

		operatorCfg := k8soperator.OperatorConfig{
			Namespace:               cfg.Operator.Namespace,
			LeaderElection:          cfg.Operator.LeaderElection,
			LeaderElectionID:        cfg.Operator.LeaderElectionID,
			ReconcileInterval:       cfg.Operator.ReconcileInterval,
			MaxConcurrentReconciles: cfg.Operator.MaxConcurrentReconciles,
		}

		operatorMgr := k8soperator.NewOperatorManager(k8sClient, crdClient, operatorCfg)

		// Add controllers
		reCtrl := k8soperator.NewRemoteExecutionController(k8sClient, crdClient, operatorCfg)
		scCtrl := k8soperator.NewStateConfigController(k8sClient, crdClient, operatorCfg)
		operatorMgr.AddController(reCtrl)
		operatorMgr.AddController(scCtrl)

		// Set up informers
		ns := cfg.Operator.Namespace
		im := k8soperator.NewInformerManager(crdClient.Dynamic(), ns, cfg.Operator.ReconcileInterval)
		operatorMgr.SetInformerManager(im)

		if cfg.Operator.LeaderElection {
			leNs := cfg.Operator.Namespace
			if leNs == "" {
				leNs = "kscore-system"
			}
			leCfg := k8soperator.DefaultLeaderElectionConfig(leNs)
			leCfg.LeaseName = cfg.Operator.LeaderElectionID
			le := k8soperator.NewLeaderElector(k8sClient.Clientset(), leCfg)
			operatorMgr.SetLeaderElector(le)
		}

		if err := operatorMgr.Start(ctx); err != nil {
			logger.Error("Failed to start Kubernetes operator", logging.Error(err))
			return
		}
		defer operatorMgr.Stop()

		logger.Info("Kubernetes operator started",
			logging.String("namespace", cfg.Operator.Namespace),
			logging.Bool("leader_election", cfg.Operator.LeaderElection),
			logging.Duration("reconcile_interval", cfg.Operator.ReconcileInterval),
		)
	}

	// Register additional REST API handlers whose dependencies are constructed above.
	// These are registered after all infrastructure init (policy, webhooks, K8s operator).
	if eventStore != nil {
		apievents.NewHandler(eventStore, eventPublisher).RegisterRoutes(httpMux)
		logger.Info("Registered REST handler", logging.String("handler", "events"))
	}
	if policyEngine != nil {
		apipolicy.NewHandler(policyEngine, policyAuditor, complianceReporter).RegisterRoutes(httpMux)
		logger.Info("Registered REST handler", logging.String("handler", "policy"))
	}
	if webhookReceiver != nil {
		apiwebhooks.NewHandler(webhookReceiver, webhookRegistry).RegisterRoutes(httpMux)
		logger.Info("Registered REST handler", logging.String("handler", "webhooks"))
	}

	// Outbound webhook subscriptions — event-driven delivery to external endpoints
	if cfg.Webhook.Outbound.Enabled && eventSubscriber != nil {
		outboundDBPath := deriveDataPath(cfg.Storage.SQLite.Path, "outbound-webhooks.db")
		outboundDB, outboundErr := sql.Open("sqlite", outboundDBPath)
		if outboundErr != nil {
			logger.Error("Failed to open outbound webhook database", logging.Error(outboundErr))
		} else {
			defer outboundDB.Close()
			if _, pragmaErr := outboundDB.ExecContext(ctx, "PRAGMA journal_mode=WAL"); pragmaErr != nil {
				logger.Warn("Failed to enable WAL for outbound webhook DB", logging.Error(pragmaErr))
			}
			outboundStore, storeErr := outbound.NewSQLiteStore(outboundDB)
			if storeErr != nil {
				logger.Error("Failed to init outbound webhook store", logging.Error(storeErr))
			} else {
				outboundDispatcher := outbound.NewDispatcher(cfg.Webhook.Outbound.Timeout)
				outboundMgr := outbound.NewManager(outboundStore, outboundDispatcher, eventSubscriber, outbound.ManagerConfig{
					MaxRetries:   cfg.Webhook.Outbound.MaxRetries,
					RetryBackoff: cfg.Webhook.Outbound.RetryBackoff,
				})
				if startErr := outboundMgr.Start(ctx); startErr != nil {
					logger.Error("Failed to start outbound webhook manager", logging.Error(startErr))
				} else {
					defer outboundMgr.Stop()
					logger.Info("Started outbound webhook manager")
				}
				outbound.NewAPIHandler(outboundStore).RegisterRoutes(httpMux)
				logger.Info("Registered REST handler", logging.String("handler", "outbound-webhooks"))
			}
		}
	}

	apigitops.NewHandler(verification.NewEngine(), rollback.NewEngine()).RegisterRoutes(httpMux)
	logger.Info("Registered REST handler", logging.String("handler", "gitops"))

	// Runbook handler — dedicated SQLite DB for approval/intervention tables
	runbookDBPath := deriveDataPath(cfg.Storage.SQLite.Path, "runbook.db")
	runbookDB, err := sql.Open("sqlite", runbookDBPath)
	if err != nil {
		logger.Error("Failed to open runbook database", logging.Error(err))
	} else {
		defer runbookDB.Close()
		if _, pragmaErr := runbookDB.ExecContext(ctx, "PRAGMA journal_mode=WAL"); pragmaErr != nil {
			logger.Warn("Failed to enable WAL for runbook DB", logging.Error(pragmaErr))
		}
		approvalStore, aErr := approval.NewSQLiteStorage(runbookDB)
		interventionStore, iErr := intervention.NewSQLiteStorage(runbookDB)
		if aErr != nil {
			logger.Error("Failed to init approval storage", logging.Error(aErr))
		}
		if iErr != nil {
			logger.Error("Failed to init intervention storage", logging.Error(iErr))
		}
		if aErr == nil && iErr == nil {
			approvalMgr := approval.NewManager(approvalStore)
			interventionMgr := intervention.NewManager(interventionStore)
			apirunbook.NewHandler(approvalStore, interventionStore, approvalMgr, interventionMgr).RegisterRoutes(httpMux)
			logger.Info("Registered REST handler", logging.String("handler", "runbook"))
		}
	}

	// Mirror handler — empty registry with default sync config
	mirrorReg := mirror.NewRegistry()
	mirrorSync := mirror.NewSyncEngine(mirrorReg, nil)
	mirror.NewAPIHandler(mirrorReg, mirrorSync).RegisterRoutes(httpMux)
	logger.Info("Registered REST handler", logging.String("handler", "mirror"))

	// Discovery handler — default config, no-op until scanners are configured
	discoveryCfg := discovery.Config{
		ScanInterval:  5 * time.Minute,
		ScanTimeout:   30 * time.Second,
		MaxConcurrent: 10,
	}
	discoveryEngine := discovery.NewDiscovery(discoveryCfg)
	discovery.NewAPI(discoveryEngine).RegisterRoutes(httpMux)
	logger.Info("Registered REST handler", logging.String("handler", "discovery"))

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
	logger.Info("Agent management config",
		logging.Duration("heartbeat_timeout", cfg.AgentManagement.HeartbeatTimeout),
		logging.Int("stale_threshold", cfg.AgentManagement.StaleThreshold),
		logging.Duration("monitor_interval", cfg.AgentManagement.MonitorInterval),
	)
	logger.Info("Execution config",
		logging.Duration("default_timeout", cfg.Execution.DefaultTimeout),
		logging.Duration("max_timeout", cfg.Execution.MaxTimeout),
		logging.Int("batch_size", cfg.Execution.BatchSize),
		logging.Int("streaming_buffer", cfg.Execution.StreamingBuffer),
	)
	logger.Info("Events config",
		logging.Bool("enabled", cfg.Events.Enabled),
		logging.Duration("retention", cfg.Events.Retention),
		logging.Int64("max_bytes", cfg.Events.MaxBytes),
		logging.Int64("max_messages", cfg.Events.MaxMessages),
	)
	if cfg.Auth.Enabled {
		logger.Info("Authorization config",
			logging.Bool("authz_enabled", cfg.Auth.Authorization.Enabled),
			logging.Bool("default_deny", cfg.Auth.Authorization.DefaultDeny),
		)
	}
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

	result := make([]controlplane.StoredAgent, len(records))
	for i, r := range records {
		result[i] = controlplane.StoredAgent{
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
	return result, nil
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

// tracingConfigFromServerConfig maps config.TracingConfig to internaltracing.Config.
func tracingConfigFromServerConfig(cfg config.TracingConfig) *internaltracing.Config {
	tracingCfg := internaltracing.DefaultConfig("kscore-server")
	tracingCfg.Enabled = cfg.Enabled

	// Map sampling strategy
	switch cfg.Sampling.Strategy {
	case "always":
		tracingCfg.Sampling.Type = internaltracing.SamplingAlwaysOn
	case "never":
		tracingCfg.Sampling.Type = internaltracing.SamplingAlwaysOff
	case "ratio":
		tracingCfg.Sampling.Type = internaltracing.SamplingProbabilistic
		tracingCfg.Sampling.Rate = cfg.Sampling.Ratio
	case "parent_based":
		tracingCfg.Sampling.Type = internaltracing.SamplingParentBased
		tracingCfg.Sampling.Rate = cfg.Sampling.Ratio
	default:
		tracingCfg.Sampling.Type = internaltracing.SamplingProbabilistic
		tracingCfg.Sampling.Rate = cfg.Sampling.Ratio
	}

	// Map exporter
	if cfg.Endpoint != "" {
		var exporterType internaltracing.ExporterType
		switch cfg.Exporter {
		case "otlp":
			exporterType = internaltracing.ExporterOTLP
		case "zipkin":
			exporterType = internaltracing.ExporterZipkin
		case "stdout":
			exporterType = internaltracing.ExporterStdout
		default:
			exporterType = internaltracing.ExporterOTLP
		}
		tracingCfg.Exporters = []internaltracing.ExporterConfig{
			{
				Type:     exporterType,
				Endpoint: cfg.Endpoint,
				Insecure: true,
			},
		}
	}

	// Map resource attributes
	for k, v := range cfg.Resource {
		switch k {
		case "service.name":
			tracingCfg.ServiceName = v
		case "service.version":
			tracingCfg.ServiceVersion = v
		case "deployment.environment":
			tracingCfg.Environment = v
		default:
			tracingCfg.ResourceAttributes[k] = v
		}
	}

	return tracingCfg
}

// buildTLSConfig creates a *tls.Config from the server TLS configuration.
func buildTLSConfig(cfg config.TLSConfig) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("loading TLS certificate: %w", err)
	}

	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS13,
	}

	switch cfg.MinVersion {
	case "1.2":
		tlsCfg.MinVersion = tls.VersionTLS12
	case "1.3", "":
		tlsCfg.MinVersion = tls.VersionTLS13
	}

	if cfg.CAFile != "" {
		caCert, err := os.ReadFile(cfg.CAFile)
		if err != nil {
			return nil, fmt.Errorf("reading CA certificate: %w", err)
		}
		caPool := x509.NewCertPool()
		if !caPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA certificate from %s", cfg.CAFile)
		}
		tlsCfg.ClientCAs = caPool
		tlsCfg.ClientAuth = tls.RequireAndVerifyClientCert
	}

	return tlsCfg, nil
}

// corsMiddleware wraps an http.Handler with CORS header handling.
func corsMiddleware(next http.Handler, cfg config.CORSConfig) http.Handler {
	allowAll := len(cfg.AllowedOrigins) == 1 && cfg.AllowedOrigins[0] == "*"
	methods := strings.Join(cfg.AllowedMethods, ", ")
	headers := strings.Join(cfg.AllowedHeaders, ", ")
	maxAge := strconv.Itoa(cfg.MaxAge)

	origins := make(map[string]struct{}, len(cfg.AllowedOrigins))
	for _, o := range cfg.AllowedOrigins {
		origins[o] = struct{}{}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" {
			next.ServeHTTP(w, r)
			return
		}

		allowed := allowAll
		if !allowed {
			_, allowed = origins[origin]
		}
		if !allowed {
			next.ServeHTTP(w, r)
			return
		}

		if allowAll {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		} else {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}

		if cfg.AllowCredentials {
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}

		// Handle preflight
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", methods)
			w.Header().Set("Access-Control-Allow-Headers", headers)
			w.Header().Set("Access-Control-Max-Age", maxAge)
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// newRateLimiterFromConfig creates a ratelimit.TokenBucket from the server config.
func newRateLimiterFromConfig(cfg config.RateLimitConfig) *ratelimit.TokenBucket {
	rlCfg := &ratelimit.Config{
		Strategy:        ratelimit.StrategyTokenBucket,
		Limit:           int64(cfg.RequestsPerMinute),
		Window:          time.Minute,
		BurstSize:       int64(cfg.Burst),
		CleanupInterval: 5 * time.Minute,
	}
	return ratelimit.NewTokenBucket(rlCfg)
}

// rateLimitMiddleware wraps an http.Handler with per-client rate limiting.
func rateLimitMiddleware(next http.Handler, limiter ratelimit.Limiter, cfg config.RateLimitConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := extractRateLimitKey(r, cfg)
		result, err := limiter.Allow(r.Context(), key)
		if err != nil {
			apierror.Write(w, http.StatusInternalServerError, "internal rate limiter error")
			return
		}

		w.Header().Set("X-RateLimit-Limit", strconv.FormatInt(result.Limit, 10))
		w.Header().Set("X-RateLimit-Remaining", strconv.FormatInt(result.Remaining, 10))
		resetAt := result.ResetAt
		if resetAt.IsZero() && result.RetryAfter > 0 {
			resetAt = time.Now().Add(result.RetryAfter)
		}
		if !resetAt.IsZero() {
			w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(resetAt.Unix(), 10))
		}

		if !result.Allowed {
			retryAfter := int(result.RetryAfter.Seconds())
			if retryAfter < 1 {
				retryAfter = 1
			}
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
			apierror.Write(w, http.StatusTooManyRequests, "Rate limit exceeded")
			return
		}

		next.ServeHTTP(w, r)
	})
}

// extractRateLimitKey returns the client key for rate limiting based on config.
func extractRateLimitKey(r *http.Request, cfg config.RateLimitConfig) string {
	switch cfg.KeyExtractor {
	case "apikey":
		if key := r.Header.Get("X-API-Key"); key != "" {
			return "apikey:" + key
		}
		return "apikey:anonymous"
	case "header":
		if cfg.HeaderName != "" {
			if val := r.Header.Get(cfg.HeaderName); val != "" {
				return "header:" + val
			}
		}
		return "header:unknown"
	default: // "ip"
		return "ip:" + clientIP(r)
	}
}

// clientIP extracts the client IP from the request, checking proxy headers.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first (original client) IP
		if idx := len(xff); idx > 0 {
			for i := 0; i < len(xff); i++ {
				if xff[i] == ',' {
					return xff[:i]
				}
			}
			return xff
		}
	}
	if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
		return realIP
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// httpAuthBypassPaths are HTTP paths that do not require authentication.
var httpAuthBypassPaths = []string{
	"/health/",
	"/api/status",
}

// httpAuthMiddleware wraps an http.Handler with authentication using the same
// authenticators configured for gRPC. It extracts credentials from the
// Authorization header (Bearer token) or X-API-Key header.
func httpAuthMiddleware(next http.Handler, interceptorCfg *auth.InterceptorConfig, authCfg config.AuthConfig) http.Handler {
	headerName := authCfg.APIKey.HeaderName
	if headerName == "" {
		headerName = "X-API-Key"
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth for health and status endpoints
		for _, prefix := range httpAuthBypassPaths {
			if strings.HasPrefix(r.URL.Path, prefix) {
				next.ServeHTTP(w, r)
				return
			}
		}

		// Extract credentials from HTTP headers
		creds := ""

		// Check Authorization: Bearer <token> header
		if authHeader := r.Header.Get("Authorization"); authHeader != "" {
			if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
				creds = authHeader[7:] // len("Bearer ") == 7
			}
		}

		// Fall back to X-API-Key header
		if creds == "" {
			creds = r.Header.Get(headerName)
		}

		// Check for mTLS client certificate
		hasMTLS := false
		for _, a := range interceptorCfg.Authenticators {
			if a.Name() == "mtls" {
				hasMTLS = true
				break
			}
		}

		if creds == "" && !hasMTLS {
			apierror.Write(w, http.StatusUnauthorized, "authentication required")
			return
		}

		// Try each authenticator
		var principal *auth.Principal
		var lastErr error
		for _, a := range interceptorCfg.Authenticators {
			if a.Name() == "mtls" {
				// mTLS uses TLS peer info, not header credentials
				if r.TLS != nil && len(r.TLS.PeerCertificates) > 0 {
					p, err := a.Authenticate(r.Context(), r.TLS.PeerCertificates[0].Subject.CommonName)
					if err == nil {
						principal = p
						break
					}
					lastErr = err
				}
				continue
			}
			p, err := a.Authenticate(r.Context(), creds)
			if err == nil {
				principal = p
				break
			}
			lastErr = err
		}

		if principal == nil {
			if lastErr != nil {
				apierror.Write(w, http.StatusUnauthorized, lastErr.Error())
			} else {
				apierror.Write(w, http.StatusUnauthorized, "authentication required")
			}
			return
		}

		// Add principal to request context
		ctx := auth.ContextWithPrincipal(r.Context(), principal)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func writeJSONResponse(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		logger.Error("Failed to encode JSON response", logging.Error(err))
	}
}

// healthChecker is the interface used by health handlers to check NATS connectivity.
type healthChecker interface {
	Health() error
}

// dbPinger is the interface used by health handlers to ping the database.
type dbPinger interface {
	Ping(ctx context.Context) error
}

// agentCounter is the interface used by health handlers for agent counts.
type agentCounter interface {
	GetAgentCount() int
	GetOnlineAgentCount() int
}

// healthReadyHandler returns an http.HandlerFunc that checks component health
// for the /health/ready endpoint. It respects the HealthConfig for per-check
// enable/disable and the startup grace period.
func healthReadyHandler(cfg config.HealthConfig, startTime time.Time, nats healthChecker, db dbPinger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		inGracePeriod := cfg.StartupGracePeriod > 0 && time.Since(startTime) < cfg.StartupGracePeriod

		var errors []string

		// Check NATS health
		if cfg.Checks.NATS.Enabled {
			checkCtx := r.Context()
			if cfg.Checks.NATS.Timeout > 0 {
				var cancel context.CancelFunc
				checkCtx, cancel = context.WithTimeout(checkCtx, cfg.Checks.NATS.Timeout)
				defer cancel()
			}
			_ = checkCtx // NATS Health() doesn't take context, but timeout bounds the handler
			if err := nats.Health(); err != nil {
				errors = append(errors, "nats: "+err.Error())
			}
		}

		// Check database health
		if cfg.Checks.Database.Enabled {
			checkCtx := r.Context()
			if cfg.Checks.Database.Timeout > 0 {
				var cancel context.CancelFunc
				checkCtx, cancel = context.WithTimeout(checkCtx, cfg.Checks.Database.Timeout)
				defer cancel()
			}
			if err := db.Ping(checkCtx); err != nil {
				errors = append(errors, "database: "+err.Error())
			}
		}

		if len(errors) > 0 && !inGracePeriod {
			writeJSONResponse(w, http.StatusServiceUnavailable, map[string]interface{}{
				"status": "not ready",
				"errors": errors,
			})
			return
		}

		writeJSONResponse(w, http.StatusOK, map[string]string{"status": "ready"})
	}
}

// componentHealth is the JSON structure for a single component in /health/status.
type componentHealth struct {
	Status    string `json:"status"`
	LatencyMs int64  `json:"latency_ms,omitempty"`
	Agents    int    `json:"agents,omitempty"`
	Error     string `json:"error,omitempty"`
}

// healthStatusResponse is the JSON structure for /health/status.
type healthStatusResponse struct {
	Status        string                     `json:"status"`
	Components    map[string]componentHealth `json:"components"`
	UptimeSeconds int64                      `json:"uptime_seconds"`
}

// healthStatusHandler returns an http.HandlerFunc that returns component-level
// health for the /health/status endpoint, matching the documented response format.
func healthStatusHandler(cfg config.HealthConfig, startTime time.Time, nats healthChecker, db dbPinger, agentCtr agentCounter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		components := make(map[string]componentHealth)
		overallHealthy := true

		// Check NATS
		if cfg.Checks.NATS.Enabled {
			start := time.Now()
			err := nats.Health()
			latency := time.Since(start).Milliseconds()
			if err != nil {
				components["nats"] = componentHealth{Status: "unhealthy", LatencyMs: latency, Error: err.Error()}
				overallHealthy = false
			} else {
				components["nats"] = componentHealth{Status: "healthy", LatencyMs: latency}
			}
		}

		// Check database
		if cfg.Checks.Database.Enabled {
			checkCtx := r.Context()
			if cfg.Checks.Database.Timeout > 0 {
				var cancel context.CancelFunc
				checkCtx, cancel = context.WithTimeout(checkCtx, cfg.Checks.Database.Timeout)
				defer cancel()
			}
			start := time.Now()
			err := db.Ping(checkCtx)
			latency := time.Since(start).Milliseconds()
			if err != nil {
				components["database"] = componentHealth{Status: "unhealthy", LatencyMs: latency, Error: err.Error()}
				overallHealthy = false
			} else {
				components["database"] = componentHealth{Status: "healthy", LatencyMs: latency}
			}
		}

		// Check agent pool
		if cfg.Checks.Agents.Enabled {
			total := agentCtr.GetAgentCount()
			online := agentCtr.GetOnlineAgentCount()
			agentStatus := "healthy"
			if total > 0 && cfg.Checks.Agents.MinHealthy > 0 {
				ratio := float64(online) / float64(total)
				if ratio < cfg.Checks.Agents.MinHealthy {
					agentStatus = "degraded"
					overallHealthy = false
				}
			}
			components["agent_pool"] = componentHealth{Status: agentStatus, Agents: online}
		}

		status := "healthy"
		if !overallHealthy {
			status = "degraded"
		}

		resp := healthStatusResponse{
			Status:        status,
			Components:    components,
			UptimeSeconds: int64(time.Since(startTime).Seconds()),
		}

		code := http.StatusOK
		if !overallHealthy {
			code = http.StatusServiceUnavailable
		}
		writeJSONResponse(w, code, resp)
	}
}

// clusterConfigFromSettings converts a config.ClusteringConfig to a full cluster.Config
// by starting from cluster defaults and overriding user-supplied fields.
func clusterConfigFromSettings(c config.ClusteringConfig) *cluster.Config {
	cfg := cluster.DefaultConfig()
	cfg.Enabled = c.Enabled
	if c.MemberID != "" {
		cfg.MemberID = c.MemberID
	}
	if c.ClusterName != "" {
		cfg.ClusterName = c.ClusterName
	}
	if c.AdvertiseAddress != "" {
		cfg.AdvertiseAddress = c.AdvertiseAddress
	}
	if len(c.EtcdEndpoints) > 0 {
		cfg.Etcd.Mode = cluster.EtcdModeExternal
		cfg.Etcd.Endpoints = c.EtcdEndpoints
	}
	if c.EtcdPrefix != "" {
		cfg.Etcd.KeyPrefix = c.EtcdPrefix
	}
	return cfg
}

// deriveDataPath returns a file path in the same directory as mainDBPath with the given filename.
func deriveDataPath(mainDBPath, filename string) string {
	return filepath.Join(filepath.Dir(mainDBPath), filename)
}
