package server

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"go.keystone-core.io/keystone-core/internal/config"
	"go.keystone-core.io/keystone-core/internal/controlplane"
	"go.keystone-core.io/keystone-core/internal/events"
	"go.keystone-core.io/keystone-core/internal/health"
	metricspkg "go.keystone-core.io/keystone-core/internal/metrics"
	"go.keystone-core.io/keystone-core/internal/policy"
	"go.keystone-core.io/keystone-core/internal/secrets"
	"go.keystone-core.io/keystone-core/internal/audit"
	"go.keystone-core.io/keystone-core/internal/state"
	"go.keystone-core.io/keystone-core/pkg/api/auth"
	"go.keystone-core.io/keystone-core/pkg/envelope"
	"go.keystone-core.io/keystone-core/pkg/version"
)

// DefaultStatusTickerInterval matches PROJECT-DETAILS §4.4 step 21.
const DefaultStatusTickerInterval = 30 * time.Second

// Per-step shutdown timeouts. PROJECT-DETAILS §4.4 step 6 specifies
// 30s for HTTP and step 7 specifies 5s for tracing/profiling; the
// other ceilings are sized so the worst-case total stays within a
// 30s top-level ctx (typical kscore-server invocation).
//
// Each helper uses contextWithTimeoutMin(parent, ceiling) so a stuck
// step can't starve later ones, and the parent ctx can still cap
// the total budget tighter than the sum of these ceilings.
const (
	grpcGraceTimeout         = 10 * time.Second
	cmdDispatcherStopTimeout = 5 * time.Second
	connMgrStopTimeout       = 5 * time.Second
	natsShutdownTimeout      = 5 * time.Second
	httpShutdownTimeout      = 30 * time.Second
	tracingShutdownTimeout   = 5 * time.Second
)

// Addrs are the bound listener addresses, populated by New.
//
// GRPC and HTTP report the primary listener (index 0; IPv4 when the
// configured Host implies dual-stack). AllGRPC and AllHTTP expose
// every bound listener so callers and tests can verify dual-stack
// configurations independently.
type Addrs struct {
	GRPC    string
	HTTP    string
	AllGRPC []string
	AllHTTP []string
}

// Server orchestrates the kscore-server runtime per PROJECT-DETAILS
// §4.4. New executes init steps 1-13 + 15-18 (binding listeners but
// not yet serving); Start executes step 14 + serving + step 21
// (status ticker); Stop runs the reverse-of-init shutdown.
type Server struct {
	cfg                *config.Config
	logger             *slog.Logger
	store              state.Store
	nats               NATSManager
	subjects           controlplane.Subjects
	signer             controlplane.Signer
	subscriber         controlplane.Subscriber
	bootstrapValidator controlplane.BootstrapValidator
	credentialIssuer   controlplane.CredentialIssuer
	authInterceptor    *auth.InterceptorConfig
	tlsConfig          *tls.Config
	now                func() time.Time
	version            string

	secretsBroker  *secrets.Broker
	secretsTransit secrets.TransitBackend
	secretsLeases  *secrets.LeaseManager

	eventStore      events.EventStore
	eventPublisher  events.EventPublisher
	eventSubscriber events.EventSubscriber //nolint:unused // wired for the gRPC handler when SubscribeEvents lands in main.go

	commandTerminalHook controlplane.TerminalCommandFunc
	stateAuditor        audit.Auditor

	policyEngine   *policy.Engine
	policyReports  *policy.ReportGenerator
	policyAuditLog audit.AuditStore
	policyAuditor  audit.Auditor

	metrics             *Metrics
	controlPlaneMetrics *controlplane.Metrics
	metricsRegistry     *metricspkg.Registry

	connMgr          *controlplane.ConnectionManager
	cmdDispatcher    *controlplane.CommandDispatcher
	batchDispatcher  *controlplane.BatchDispatcher
	bootstrapHandler *controlplane.BootstrapHandler

	grpcServer    *grpc.Server
	grpcListeners []net.Listener

	httpServer    *http.Server
	httpListeners []net.Listener

	healthChecker *healthChecker

	addrs                Addrs
	startedAt            time.Time
	statusTickerInterval time.Duration

	startOnce sync.Once
	stopOnce  sync.Once
	stopCh    chan struct{}
	stopped   chan struct{}
}

// New executes the §4.4 init steps that don't require serving:
//  3-4   NATS start + verify
//  5     Store ping
//  6     ConnectionManager start
//  7     CommandDispatcher start
//  8     BatchDispatcher (lazy)
//  9-12  TLS / interceptor / gRPC server / service registration
//  13    bind gRPC listener
//  15-16 build HTTP mux + register endpoints
//  17    wrap HTTP middleware
//  18    bind HTTP listener
//
// Steps 1-2 (config validate + logger) are the caller's job; step 14
// (start gRPC serving) and 19-21 (optional components, banner, ticker)
// run in Start. On any failure during init, the partial state is
// unwound (NATS shut down, listeners closed, dispatchers stopped) so
// callers can retry without leaking resources.
func New(opts Options) (*Server, error) {
	if err := opts.validate(); err != nil {
		return nil, err
	}

	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}
	tick := opts.StatusTickerInterval
	if tick == 0 {
		tick = DefaultStatusTickerInterval
	}

	s := &Server{
		cfg:                  opts.Config,
		logger:               opts.Logger,
		store:                opts.Store,
		nats:                 opts.NATSManager,
		subjects:             opts.Subjects,
		signer:               opts.Signer,
		subscriber:           opts.Subscriber,
		bootstrapValidator:   opts.BootstrapValidator,
		credentialIssuer:     opts.CredentialIssuer,
		authInterceptor:      opts.AuthInterceptor,
		tlsConfig:            opts.TLSConfig,
		now:                  clock,
		version:              version.Get().Version,
		statusTickerInterval: tick,
		secretsBroker:        opts.SecretsBroker,
		secretsTransit:       opts.SecretsTransit,
		secretsLeases:        opts.SecretsLeases,
		eventStore:           opts.EventStore,
		eventPublisher:       opts.EventPublisher,
		eventSubscriber:      opts.EventSubscriber,
		commandTerminalHook:  opts.CommandTerminalHook,
		stateAuditor:         opts.StateAuditor,
		policyEngine:         opts.PolicyEngine,
		policyReports:        opts.PolicyReports,
		policyAuditLog:       opts.PolicyAuditLog,
		policyAuditor:        opts.PolicyAuditor,
		metrics:              opts.Metrics,
		controlPlaneMetrics:  opts.ControlPlaneMetrics,
		metricsRegistry:      opts.MetricsRegistry,
		stopCh:               make(chan struct{}),
		stopped:              make(chan struct{}),
	}

	// Init runs in order; on error we unwind in reverse.
	initCtx := context.Background()
	if err := s.initSteps3to4(initCtx); err != nil {
		return nil, fmt.Errorf("server: NATS init: %w", err)
	}
	if err := s.initStep5(initCtx); err != nil {
		s.unwindFromStep5(initCtx)
		return nil, fmt.Errorf("server: store ping: %w", err)
	}
	if err := s.initStep6(initCtx); err != nil {
		s.unwindFromStep6(initCtx)
		return nil, fmt.Errorf("server: connection manager: %w", err)
	}
	if err := s.initStep7(initCtx); err != nil {
		s.unwindFromStep7(initCtx)
		return nil, fmt.Errorf("server: command dispatcher: %w", err)
	}
	if err := s.initStep7b(initCtx); err != nil {
		s.unwindFromStep7(initCtx)
		return nil, fmt.Errorf("server: bootstrap handler: %w", err)
	}
	if err := s.initStep8(); err != nil {
		s.unwindFromStep7(initCtx)
		return nil, fmt.Errorf("server: batch dispatcher: %w", err)
	}
	if err := s.initSteps9to13(); err != nil {
		s.unwindFromStep7(initCtx)
		return nil, fmt.Errorf("server: gRPC bind: %w", err)
	}
	if err := s.initSteps15to18(); err != nil {
		s.unwindFromStep13(initCtx)
		return nil, fmt.Errorf("server: HTTP bind: %w", err)
	}

	// startedAt is fixed when New returns successfully so the health
	// grace period covers from "ready to serve" rather than "Start
	// goroutine launched" (a few ms earlier — irrelevant in practice
	// but keeps the semantic stable across test fixtures).
	s.startedAt = s.now()
	extras := make([]health.Checker, 0, 1+len(opts.HealthCheckers))
	if opts.JetStreamPinger != nil {
		extras = append(extras, health.NewJetStreamChecker(opts.JetStreamPinger, 0))
	}
	extras = append(extras, opts.HealthCheckers...)
	s.healthChecker = newHealthChecker(
		s.nats, s.store, s.startedAt,
		s.cfg.Health.StartupGracePeriod, s.cfg.Health.CheckTimeout,
		s.now, s.logger,
		extras...,
	)
	return s, nil
}

// initSteps3to4: bring NATS up + sanity-check it.
func (s *Server) initSteps3to4(ctx context.Context) error {
	if err := s.nats.Start(ctx); err != nil {
		return err
	}
	return s.nats.Health(ctx)
}

// initStep5: confirm store is alive.
func (s *Server) initStep5(ctx context.Context) error {
	return s.store.Ping(ctx)
}

// initStep6: ConnectionManager + heartbeat monitor.
func (s *Server) initStep6(ctx context.Context) error {
	mgr, err := controlplane.New(controlplane.Config{
		Store:  s.store,
		Logger: s.logger,
		Clock:  s.now,
	})
	if err != nil {
		return err
	}
	if err := mgr.Start(ctx); err != nil {
		return err
	}
	s.connMgr = mgr
	return nil
}

// initStep7: CommandDispatcher + retention + timeout loops.
func (s *Server) initStep7(ctx context.Context) error {
	disp, err := controlplane.NewDispatcher(controlplane.DispatcherConfig{
		Store:             s.store,
		Agents:            s.connMgr,
		Publisher:         natsPublisherAdapter{s.nats},
		Subjects:          s.subjects,
		Signer:            s.signer,
		Logger:            s.logger,
		Clock:             s.now,
		OnCommandTerminal: s.commandTerminalHook,
	})
	if err != nil {
		return err
	}
	if err := disp.Start(ctx); err != nil {
		return err
	}
	s.cmdDispatcher = disp
	return nil
}

// initStep7b: BootstrapHandler (Epic 05 task 9). Skipped when
// cfg.NATS.Bootstrap.Enabled is false — operator opt-in.
func (s *Server) initStep7b(ctx context.Context) error {
	if !s.cfg.NATS.Bootstrap.Enabled {
		return nil
	}
	h, err := controlplane.NewBootstrapHandler(controlplane.BootstrapHandlerConfig{
		Subjects:   s.subjects,
		Subscriber: s.subscriber,
		Publisher:  natsPublisherAdapter{s.nats},
		Store:      s.store,
		Validator:  s.bootstrapValidator,
		Issuer:     s.credentialIssuer,
		Logger:     s.logger,
		Clock:      s.now,
	})
	if err != nil {
		return err
	}
	if err := h.Start(ctx); err != nil {
		return err
	}
	s.bootstrapHandler = h
	return nil
}

// initStep8: BatchDispatcher (no goroutine; lazy per §4.4 step 8).
func (s *Server) initStep8() error {
	disp, err := controlplane.NewBatchDispatcher(controlplane.BatchDispatcherConfig{
		Store:  s.store,
		Logger: s.logger,
		Clock:  s.now,
	})
	if err != nil {
		return err
	}
	s.batchDispatcher = disp
	return nil
}

// initSteps9to13: TLS config, auth chain, gRPC server, register
// services, bind gRPC listener.
//
// Task 4 places minimal scaffolding here:
//   step 9  — TLS not yet supported (nil); task 8/Epic 13 wire real TLS.
//   step 10 — interceptor chain placeholder; task 6 builds the real one.
//   step 11 — grpc.NewServer with whatever interceptors are wired.
//   step 12 — service registration loop is empty (services land with
//             their epics; nil-guarded).
//   step 13 — single-stack net.Listen; task 5 swaps to dual-stack.
func (s *Server) initSteps9to13() error {
	var grpcOpts []grpc.ServerOption
	if s.cfg.Server.TLS.Enabled {
		// Epic 09 task 13 — TLS wired via the identity provider OR
		// a file-sourced cert/key pair. Either way the operator
		// passes a fully-built *tls.Config through Options.TLSConfig;
		// the cmd/kscore-server boot is responsible for choosing
		// between the two sources.
		if s.tlsConfig == nil {
			return errors.New("server: TLS.Enabled=true but Options.TLSConfig is nil")
		}
		grpcOpts = append(grpcOpts, grpc.Creds(credentials.NewTLS(s.tlsConfig)))
		s.logger.Info("grpc TLS enabled",
			"min_version", s.tlsConfig.MinVersion,
			"client_auth", s.tlsConfig.ClientAuth,
		)
	}

	// step 10/11: chain rate-limit → auth → authorize via the auth
	// package's InterceptorConfig. CORS doesn't apply to gRPC (binary
	// HTTP/2; browser clients use grpc-web through a separate gateway
	// in v2.x). When AuthInterceptor is nil, the server runs with no
	// auth — appropriate for dev, surfaced in the banner.
	//
	// Metrics interceptors (Epic 17 task 2) sit *outermost* so they
	// observe the auth-rejected path too — operators want to alert on
	// 401/403 spikes via the same histogram. Chain order:
	//
	//	metrics → auth (rate-limit → auth → authorize) → handler
	// Epic 17 task 10 — correlation ID sits OUTERMOST so every RPC
	// (including auth-rejected) carries an ID into the log stream and
	// back to the caller via trailer.
	unaryChain := []grpc.UnaryServerInterceptor{UnaryCorrelationInterceptor()}
	streamChain := []grpc.StreamServerInterceptor{StreamCorrelationInterceptor()}
	if s.metrics != nil {
		unaryChain = append(unaryChain, s.metrics.UnaryServerInterceptor())
		streamChain = append(streamChain, s.metrics.StreamServerInterceptor())
	}
	if s.authInterceptor != nil {
		unary, err := s.authInterceptor.UnaryServerInterceptor()
		if err != nil {
			return fmt.Errorf("server: gRPC unary interceptor: %w", err)
		}
		stream, err := s.authInterceptor.StreamServerInterceptor()
		if err != nil {
			return fmt.Errorf("server: gRPC stream interceptor: %w", err)
		}
		unaryChain = append(unaryChain, unary)
		streamChain = append(streamChain, stream)
	}
	if len(unaryChain) > 0 {
		grpcOpts = append(grpcOpts, grpc.ChainUnaryInterceptor(unaryChain...))
	}
	if len(streamChain) > 0 {
		grpcOpts = append(grpcOpts, grpc.ChainStreamInterceptor(streamChain...))
	}
	s.grpcServer = grpc.NewServer(grpcOpts...)
	// step 12: register gRPC services (none yet — nil-guarded).
	// Concrete impls land with their owning epics (06, 07, 09, …).

	lns, err := listen(s.cfg.Server.Host, s.cfg.Server.GRPCPort)
	if err != nil {
		return fmt.Errorf("gRPC: %w", err)
	}
	s.grpcListeners = lns
	s.addrs.AllGRPC = addrs(lns)
	s.addrs.GRPC = s.addrs.AllGRPC[0]
	return nil
}

// initSteps15to18: HTTP mux, health endpoints, REST handler
// registration, middleware wrap, bind HTTP listener.
func (s *Server) initSteps15to18() error {
	// step 15-17: build the routing tree (CORS → router → auth → mux).
	// See middleware.go.
	handler, err := s.buildHTTPHandler()
	if err != nil {
		return err
	}

	lns, err := listen(s.cfg.Server.Host, s.cfg.Server.HTTPPort)
	if err != nil {
		return fmt.Errorf("HTTP: %w", err)
	}
	s.httpListeners = lns
	s.addrs.AllHTTP = addrs(lns)
	s.addrs.HTTP = s.addrs.AllHTTP[0]

	s.httpServer = &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	return nil
}

// Start runs step 14 (gRPC serve) + step 18 (HTTP serve) + step 20
// (banner) + step 21 (status ticker). Returns once both listeners are
// accepting; serving continues in goroutines until Stop.
func (s *Server) Start(ctx context.Context) error {
	var startErr error
	s.startOnce.Do(func() {
		// startedAt is set in New; Start does not overwrite it.
		for _, ln := range s.grpcListeners {
			ln := ln
			go func() {
				if err := s.grpcServer.Serve(ln); err != nil {
					s.logger.Warn("server: gRPC serve exited",
						"addr", ln.Addr().String(), "err", err)
				}
			}()
		}
		for _, ln := range s.httpListeners {
			ln := ln
			go func() {
				err := s.httpServer.Serve(ln)
				if err != nil && !errors.Is(err, http.ErrServerClosed) {
					s.logger.Warn("server: HTTP serve exited",
						"addr", ln.Addr().String(), "err", err)
				}
			}()
		}

		go s.runStatusTicker()

		s.logger.Info(banner(
			version.Get(), s.addrs,
			s.authMode(), s.cfg.Storage.Driver,
			s.ProductionWarnings(),
		))
	})
	return startErr
}

// Stop runs the §4.4 reverse-of-init shutdown sequence:
//
//	1. log "shutdown begin"
//	2. gRPC.GracefulStop  (forcible Stop fallback if it hangs)
//	3. CommandDispatcher.Stop  (added: keeps the task-2 dispatcher's
//	    retention/timeout loops from outliving the server)
//	4. ConnectionManager.Stop
//	5. Store.Close
//	6. NATSManager.Shutdown
//	7. HTTP.Shutdown (per spec: AFTER NATS so /health/ready signals
//	    503 to load balancers before HTTP itself stops accepting)
//	8. tracing/profiling cleanup placeholder (Epic 17 wires the real
//	    teardown; the 5s budget is in place from day one)
//	9. log "shutdown complete"
//
// Bounded by ctx; idempotent; safe to call before Start. Each step
// uses contextWithTimeoutMin(ctx, stepCeiling) so a stuck step
// can't starve later ones. The aggregated error is the FIRST error
// encountered — subsequent failures still execute to keep the
// teardown ordered.
func (s *Server) Stop(ctx context.Context) error {
	var stopErr error
	s.stopOnce.Do(func() {
		close(s.stopCh)
		defer close(s.stopped)

		start := s.now()
		s.logger.Info("server: shutdown begin")

		stopErr = firstErr(stopErr, s.stopGRPC(ctx))
		stopErr = firstErr(stopErr, s.stopBootstrap(ctx))
		stopErr = firstErr(stopErr, s.stopCmdDispatcher(ctx))
		stopErr = firstErr(stopErr, s.stopConnMgr(ctx))
		stopErr = firstErr(stopErr, s.stopStore())
		stopErr = firstErr(stopErr, s.stopNATS(ctx))
		stopErr = firstErr(stopErr, s.stopHTTP(ctx))
		stopErr = firstErr(stopErr, s.stopTracing(ctx))

		elapsed := s.now().Sub(start)
		if stopErr != nil {
			s.logger.Info("server: shutdown complete", "elapsed", elapsed, "err", stopErr)
		} else {
			s.logger.Info("server: shutdown complete", "elapsed", elapsed)
		}
	})
	// Concurrent Stop callers block until the in-flight shutdown
	// completes, then receive a stable error.
	<-s.stopped
	return stopErr
}

// Addrs returns the bound listener addresses. Valid after New.
func (s *Server) Addrs() Addrs { return s.addrs }

func (s *Server) runStatusTicker() {
	t := time.NewTicker(s.statusTickerInterval)
	defer t.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case <-t.C:
			counts := s.connMgr.Counts()
			s.controlPlaneMetrics.SetAgentCounts(s.cfg.NATS.ClusterName, counts)
			s.logger.Info("server: status",
				"agents_total", counts.Total,
				"agents_connected", counts.Connected,
				"agents_stale", counts.Stale,
				"agents_disabled", counts.Disabled,
				"in_flight_commands", s.cmdDispatcher.InFlight(),
			)
		}
	}
}

func (s *Server) authMode() string {
	if s.authInterceptor == nil {
		return "disabled"
	}
	return "enabled"
}

// ProductionWarnings returns the union of config-level warnings
// (Config.ProductionWarnings) and server-state warnings that aren't
// observable from config alone — currently just "auth is disabled in
// production" when no AuthInterceptor was wired into Options.
//
// Empty when Mode != production. Used by the startup banner and the
// /api/status payload so operators see the same warning set in both
// surfaces.
func (s *Server) ProductionWarnings() []string {
	w := s.cfg.ProductionWarnings()
	if s.cfg.Mode == config.ModeProduction && s.authInterceptor == nil {
		w = append(w, "auth is disabled in production (no AuthInterceptor wired)")
	}
	return w
}

// RegisterService registers a gRPC service against the underlying
// *grpc.Server. Mirrors grpc.Server.RegisterService so concrete
// services (Epic 06's AgentService, Epic 07's ControlPlaneService,
// etc.) can wire themselves through the Server type without dropping
// down to the raw gRPC handle.
//
// MUST be called between New and Start — RegisterService panics if
// invoked while serving (per gRPC's contract).
func (s *Server) RegisterService(desc *grpc.ServiceDesc, impl any) {
	s.grpcServer.RegisterService(desc, impl)
}

// CommandDispatcher exposes the dispatcher built in init step 7 so
// callers (e.g., cmd/kscore-server) can wire downstream consumers
// like the response router. Available after New; populated during
// Start. Nil before Start.
func (s *Server) CommandDispatcher() *controlplane.CommandDispatcher {
	return s.cmdDispatcher
}

// ---- per-step shutdown helpers ------------------------------------------

// stopGRPC stops the gRPC server. Wraps GracefulStop in a goroutine
// so we can fall back to forcible Stop() if the graceful path doesn't
// return within grpcGraceTimeout (or the parent ctx, whichever is
// shorter). Without the fallback, a single hung stream blocks
// shutdown indefinitely.
func (s *Server) stopGRPC(ctx context.Context) error {
	if s.grpcServer == nil {
		return nil
	}
	grpcCtx, cancel := contextWithTimeoutMin(ctx, grpcGraceTimeout)
	defer cancel()

	done := make(chan struct{})
	go func() {
		s.grpcServer.GracefulStop()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-grpcCtx.Done():
		s.logger.Warn("server: gRPC graceful stop timed out; forcing Stop()",
			"timeout", grpcGraceTimeout)
		s.grpcServer.Stop()
		<-done // GracefulStop returns once Stop() unblocks it
		return grpcCtx.Err()
	}
}

func (s *Server) stopCmdDispatcher(ctx context.Context) error {
	if s.cmdDispatcher == nil {
		return nil
	}
	stepCtx, cancel := contextWithTimeoutMin(ctx, cmdDispatcherStopTimeout)
	defer cancel()
	if err := s.cmdDispatcher.Stop(stepCtx); err != nil {
		s.logger.Warn("server: command dispatcher stop", "err", err)
		return err
	}
	return nil
}

func (s *Server) stopBootstrap(ctx context.Context) error {
	if s.bootstrapHandler == nil {
		return nil
	}
	stepCtx, cancel := contextWithTimeoutMin(ctx, cmdDispatcherStopTimeout)
	defer cancel()
	if err := s.bootstrapHandler.Stop(stepCtx); err != nil {
		s.logger.Warn("server: bootstrap handler stop", "err", err)
		return err
	}
	return nil
}

func (s *Server) stopConnMgr(ctx context.Context) error {
	if s.connMgr == nil {
		return nil
	}
	stepCtx, cancel := contextWithTimeoutMin(ctx, connMgrStopTimeout)
	defer cancel()
	if err := s.connMgr.Stop(stepCtx); err != nil {
		s.logger.Warn("server: connection manager stop", "err", err)
		return err
	}
	return nil
}

// stopStore takes no ctx — state.Store.Close is non-blocking on
// SQLite and bounded internally on Postgres. Wrapping in a ctx
// without a Close-with-ctx storage API would only add complexity.
func (s *Server) stopStore() error {
	if s.store == nil {
		return nil
	}
	if err := s.store.Close(); err != nil {
		s.logger.Warn("server: store close", "err", err)
		return err
	}
	return nil
}

func (s *Server) stopNATS(ctx context.Context) error {
	if s.nats == nil {
		return nil
	}
	stepCtx, cancel := contextWithTimeoutMin(ctx, natsShutdownTimeout)
	defer cancel()
	if err := s.nats.Shutdown(stepCtx); err != nil {
		s.logger.Warn("server: NATS shutdown", "err", err)
		return err
	}
	return nil
}

func (s *Server) stopHTTP(ctx context.Context) error {
	if s.httpServer == nil {
		return nil
	}
	stepCtx, cancel := contextWithTimeoutMin(ctx, httpShutdownTimeout)
	defer cancel()
	if err := s.httpServer.Shutdown(stepCtx); err != nil {
		s.logger.Warn("server: HTTP shutdown", "err", err)
		return err
	}
	return nil
}

// stopTracing is the §4.4 step 7 hook for tracing/profiling cleanup.
// Epic 17 wires the real OTel + pprof teardown here; the 5s budget
// is in place from day one so the integration point doesn't have to
// thread its own timeout. No-op for v1.0 task 8.
func (s *Server) stopTracing(ctx context.Context) error {
	_, cancel := contextWithTimeoutMin(ctx, tracingShutdownTimeout)
	defer cancel()
	return nil
}

// firstErr returns prior if non-nil, otherwise next. Used by Stop to
// keep the first error while still running every step (ordered
// teardown matters more than failing fast on the first step).
func firstErr(prior, next error) error {
	if prior != nil {
		return prior
	}
	return next
}

// ---- unwind helpers (used by New on partial-init failure) ---------------

func (s *Server) unwindFromStep5(_ context.Context) {
	// step 5 already done — only NATS to roll back.
	_ = s.nats.Shutdown(context.Background())
}

func (s *Server) unwindFromStep6(ctx context.Context) {
	if s.connMgr != nil {
		_ = s.connMgr.Stop(ctx)
	}
	s.unwindFromStep5(ctx)
}

func (s *Server) unwindFromStep7(ctx context.Context) {
	if s.bootstrapHandler != nil {
		_ = s.bootstrapHandler.Stop(ctx)
	}
	if s.cmdDispatcher != nil {
		_ = s.cmdDispatcher.Stop(ctx)
	}
	s.unwindFromStep6(ctx)
}

func (s *Server) unwindFromStep13(ctx context.Context) {
	_ = closeAll(s.grpcListeners)
	s.unwindFromStep7(ctx)
}

// ---- adapters -----------------------------------------------------------

// natsPublisherAdapter satisfies controlplane.NATSPublisher by
// delegating to NATSManager.PublishEnvelope. Avoids leaking the
// broader NATSManager interface into the controlplane package.
type natsPublisherAdapter struct{ m NATSManager }

func (a natsPublisherAdapter) PublishEnvelope(ctx context.Context, subject string, env envelope.Envelope) error {
	return a.m.PublishEnvelope(ctx, subject, env)
}

// contextWithTimeoutMin returns a child context bounded by the smaller
// of (parent's remaining time, fallback). If parent has no deadline,
// fallback applies. Always returns a cancel — caller must invoke it.
func contextWithTimeoutMin(parent context.Context, fallback time.Duration) (context.Context, context.CancelFunc) {
	if dl, ok := parent.Deadline(); ok {
		remaining := time.Until(dl)
		if remaining < fallback && remaining > 0 {
			return context.WithDeadline(parent, dl)
		}
	}
	return context.WithTimeout(parent, fallback)
}

