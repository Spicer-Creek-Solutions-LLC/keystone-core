package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"google.golang.org/grpc"

	"go.keystone-core.io/keystone-core/internal/config"
	"go.keystone-core.io/keystone-core/internal/controlplane"
	"go.keystone-core.io/keystone-core/internal/state"
	"go.keystone-core.io/keystone-core/pkg/version"
)

// DefaultStatusTickerInterval matches PROJECT-DETAILS §4.4 step 21.
const DefaultStatusTickerInterval = 30 * time.Second

// httpShutdownTimeout is the per-listener deadline for http.Server.Shutdown.
// Task 8 will tighten / configure this; for now we honor the spec's 30s.
const httpShutdownTimeout = 30 * time.Second

// Addrs are the bound listener addresses, populated by New.
type Addrs struct {
	GRPC string
	HTTP string
}

// Server orchestrates the kscore-server runtime per PROJECT-DETAILS
// §4.4. New executes init steps 1-13 + 15-18 (binding listeners but
// not yet serving); Start executes step 14 + serving + step 21
// (status ticker); Stop runs the reverse-of-init shutdown.
type Server struct {
	cfg     *config.Config
	logger  *slog.Logger
	store   state.Store
	nats    NATSManager
	now     func() time.Time
	version string

	connMgr         *controlplane.ConnectionManager
	cmdDispatcher   *controlplane.CommandDispatcher
	batchDispatcher *controlplane.BatchDispatcher

	grpcServer   *grpc.Server
	grpcListener net.Listener

	httpServer   *http.Server
	httpListener net.Listener

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
		now:                  clock,
		version:              version.Get().Version,
		statusTickerInterval: tick,
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
		Store:     s.store,
		Agents:    s.connMgr,
		Publisher: natsPublisherAdapter{s.nats},
		Logger:    s.logger,
		Clock:     s.now,
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
		// task 4 deliberately fails closed: TLS configured but not
		// yet wireable from this package. Task 8 / Epic 13 land it.
		return errors.New("server.TLS.Enabled is not yet supported (Epic 13)")
	}

	// step 10/11: middleware chain placeholder. Task 6 builds the real
	// chain (CORS → rate-limit → auth) using AuthInterceptor from
	// Options; for now the gRPC server has no interceptors.
	s.grpcServer = grpc.NewServer(grpcOpts...)
	// step 12: register gRPC services (none yet — nil-guarded).
	// Concrete impls land with their owning epics (06, 07, 09, …).

	addr := net.JoinHostPort(s.cfg.Server.Host, strconv.Itoa(s.cfg.Server.GRPCPort))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen gRPC %s: %w", addr, err)
	}
	s.grpcListener = ln
	s.addrs.GRPC = ln.Addr().String()
	return nil
}

// initSteps15to18: HTTP mux, health endpoints, REST handler
// registration, middleware wrap, bind HTTP listener.
func (s *Server) initSteps15to18() error {
	mux := http.NewServeMux()
	s.registerHealthEndpoints(mux)
	s.registerDomainHandlers(mux)

	// step 17: middleware chain is a passthrough for task 4; task 6
	// wraps with CORS → rate-limit → auth.
	var handler http.Handler = mux

	addr := net.JoinHostPort(s.cfg.Server.Host, strconv.Itoa(s.cfg.Server.HTTPPort))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen HTTP %s: %w", addr, err)
	}
	s.httpListener = ln
	s.addrs.HTTP = ln.Addr().String()

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
		s.startedAt = s.now()

		go func() {
			if err := s.grpcServer.Serve(s.grpcListener); err != nil {
				s.logger.Warn("server: gRPC serve exited", "err", err)
			}
		}()
		go func() {
			err := s.httpServer.Serve(s.httpListener)
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				s.logger.Warn("server: HTTP serve exited", "err", err)
			}
		}()

		go s.runStatusTicker()

		s.logger.Info(banner(
			version.Get(), s.addrs,
			s.authMode(), string(s.cfg.Storage.Driver),
			s.cfg.ProductionWarnings(),
		))
	})
	return startErr
}

// Stop runs the reverse-of-init shutdown sequence. Bounded by ctx.
// Idempotent; safe to call before Start.
func (s *Server) Stop(ctx context.Context) error {
	var stopErr error
	s.stopOnce.Do(func() {
		close(s.stopCh)
		defer close(s.stopped)

		// Step 14 reversed: stop accepting new gRPC RPCs and wait
		// for in-flight ones.
		if s.grpcServer != nil {
			s.grpcServer.GracefulStop()
		}

		// Symmetric reverse of step 7 (added — not in spec but
		// matches the task-2 dispatcher's lifecycle).
		if s.cmdDispatcher != nil {
			if err := s.cmdDispatcher.Stop(ctx); err != nil {
				s.logger.Warn("server: command dispatcher stop", "err", err)
				if stopErr == nil {
					stopErr = err
				}
			}
		}

		// Step 6 reversed.
		if s.connMgr != nil {
			if err := s.connMgr.Stop(ctx); err != nil {
				s.logger.Warn("server: connection manager stop", "err", err)
				if stopErr == nil {
					stopErr = err
				}
			}
		}

		// Step 5 reversed.
		if s.store != nil {
			if err := s.store.Close(); err != nil {
				s.logger.Warn("server: store close", "err", err)
				if stopErr == nil {
					stopErr = err
				}
			}
		}

		// Step 3 reversed.
		if s.nats != nil {
			if err := s.nats.Shutdown(ctx); err != nil {
				s.logger.Warn("server: NATS shutdown", "err", err)
				if stopErr == nil {
					stopErr = err
				}
			}
		}

		// Step 18 reversed. Bounded by ctx OR httpShutdownTimeout,
		// whichever is shorter.
		if s.httpServer != nil {
			httpCtx, cancel := contextWithTimeoutMin(ctx, httpShutdownTimeout)
			if err := s.httpServer.Shutdown(httpCtx); err != nil {
				s.logger.Warn("server: HTTP shutdown", "err", err)
				if stopErr == nil {
					stopErr = err
				}
			}
			cancel()
		}

		// Tracing/profiling cleanup placeholder — task 8.
	})
	// If Stop is called twice, the second caller blocks until the
	// first finishes, so callers see consistent ordering.
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
	// Task 6 will populate based on AuthInterceptor; for now report
	// disabled when no interceptor is wired.
	return "disabled"
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
	if s.cmdDispatcher != nil {
		_ = s.cmdDispatcher.Stop(ctx)
	}
	s.unwindFromStep6(ctx)
}

func (s *Server) unwindFromStep13(ctx context.Context) {
	if s.grpcListener != nil {
		_ = s.grpcListener.Close()
	}
	s.unwindFromStep7(ctx)
}

// ---- adapters -----------------------------------------------------------

// natsPublisherAdapter satisfies controlplane.NATSPublisher by
// delegating to NATSManager.Publish. Avoids leaking the broader
// NATSManager interface into the controlplane package.
type natsPublisherAdapter struct{ m NATSManager }

func (a natsPublisherAdapter) Publish(ctx context.Context, subject string, data []byte) error {
	return a.m.Publish(ctx, subject, data)
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

