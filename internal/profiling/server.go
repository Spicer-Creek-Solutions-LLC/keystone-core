package profiling

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/pprof"
	"runtime"
	"strconv"
	"sync"
	"time"

	"go.keystone-core.io/keystone-core/internal/config"
)

// Server hosts the pprof HTTP listener on a dedicated address.
// Constructed via New and started exactly once.
type Server struct {
	cfg    config.ProfilingConfig
	logger *slog.Logger

	mu       sync.Mutex
	listener net.Listener
	srv      *http.Server
	started  bool

	// Runtime knobs we captured at Start so Stop can restore them.
	// Zero in either field means "not changed" and Stop skips.
	prevMutexFraction int
	prevBlockRate     int
	runtimeApplied    bool
}

// New constructs a Server from cfg. Returns (nil, nil) when
// cfg.Enabled is false so callers can pass through without a guard.
func New(cfg config.ProfilingConfig, log *slog.Logger) (*Server, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("profiling: invalid config: %w", err)
	}
	if log == nil {
		log = slog.Default()
	}
	return &Server{cfg: cfg, logger: log}, nil
}

// Start binds the listener, applies the runtime profile knobs, and
// launches the serve goroutine. Idempotent: double-Start is rejected.
// Safe on a nil *Server (returns nil).
func (s *Server) Start(_ context.Context) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		return errors.New("profiling: already started")
	}

	addr := net.JoinHostPort(s.cfg.Host, strconv.Itoa(s.cfg.Port))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("profiling: listen %s: %w", addr, err)
	}
	s.listener = ln

	mux := http.NewServeMux()
	registerPprof(mux)

	s.srv = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	s.applyRuntimeKnobsLocked()
	s.started = true

	go func() {
		err := s.srv.Serve(ln)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Warn("profiling: serve exited", "addr", addr, "err", err)
		}
	}()
	s.logger.Info("profiling: pprof listener started", "addr", addr)
	return nil
}

// Stop gracefully shuts the listener within
// cfg.ShutdownTimeout ∩ ctx, restores any runtime knobs that Start
// changed, and releases the bound port. Idempotent; safe on nil and
// on never-started.
func (s *Server) Stop(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.started {
		return nil
	}
	s.started = false

	timeout := s.cfg.ShutdownTimeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	shutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	err := s.srv.Shutdown(shutCtx)
	s.restoreRuntimeKnobsLocked()
	return err
}

// Addr returns the bound address (host:port). Empty when not started.
func (s *Server) Addr() string {
	if s == nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

// registerPprof attaches the canonical pprof handlers to mux without
// touching http.DefaultServeMux. Mirrors what net/http/pprof's init
// does globally, but localised so multiple processes (or tests) can
// coexist.
func registerPprof(mux *http.ServeMux) {
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	// /debug/pprof/{heap,goroutine,block,mutex,threadcreate,allocs}
	// land via pprof.Index — it consults runtime/pprof's named-profile
	// registry on each request.
}

// applyRuntimeKnobsLocked sets mutex/block profile knobs when configured
// non-zero, capturing the previous values for restoreRuntimeKnobsLocked.
// Must hold s.mu.
func (s *Server) applyRuntimeKnobsLocked() {
	if s.cfg.MutexProfileFraction > 0 {
		s.prevMutexFraction = runtime.SetMutexProfileFraction(s.cfg.MutexProfileFraction)
		s.runtimeApplied = true
	}
	if s.cfg.BlockProfileRate > 0 {
		// SetBlockProfileRate doesn't return the previous value, so we
		// can't restore it precisely. Record the configured rate so
		// Stop can zero it out (the "leave alone" baseline).
		runtime.SetBlockProfileRate(s.cfg.BlockProfileRate)
		s.prevBlockRate = s.cfg.BlockProfileRate
		s.runtimeApplied = true
	}
}

// restoreRuntimeKnobsLocked undoes applyRuntimeKnobsLocked. Must hold s.mu.
func (s *Server) restoreRuntimeKnobsLocked() {
	if !s.runtimeApplied {
		return
	}
	if s.cfg.MutexProfileFraction > 0 {
		runtime.SetMutexProfileFraction(s.prevMutexFraction)
	}
	if s.cfg.BlockProfileRate > 0 {
		runtime.SetBlockProfileRate(0)
	}
	s.runtimeApplied = false
}
