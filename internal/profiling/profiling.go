// Package profiling provides pprof-based performance profiling for Keystone Core.
// It includes:
//   - HTTP endpoints for pprof profiles (/debug/pprof/*)
//   - Profile capture API for CPU, heap, goroutine, mutex, block, and trace
//   - Runtime statistics collection
//   - Configurable profiling rates
//
// The profiling server can be enabled on demand for debugging performance
// issues without rebuilding binaries.
package profiling

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/pprof"
	"os"
	"runtime"
	pprofruntime "runtime/pprof"
	"runtime/trace"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/wait"
)

// ProfileType represents the type of profile to capture
type ProfileType string

const (
	ProfileTypeCPU        ProfileType = "cpu"
	ProfileTypeHeap       ProfileType = "heap"
	ProfileTypeGoroutine  ProfileType = "goroutine"
	ProfileTypeMutex      ProfileType = "mutex"
	ProfileTypeBlock      ProfileType = "block"
	ProfileTypeThreads    ProfileType = "threadcreate"
	ProfileTypeAllocs     ProfileType = "allocs"
	ProfileTypeTrace      ProfileType = "trace"
)

// ProfileConfig configures the profiling server
type ProfileConfig struct {
	// Enabled enables the profiling endpoints
	Enabled bool

	// ListenAddr is the address to listen on for profiling endpoints
	// Default: "localhost:6060"
	ListenAddr string

	// BlockProfileRate controls the fraction of goroutine blocking events
	// that are reported in the blocking profile. Default: 1
	BlockProfileRate int

	// MutexProfileFraction controls the fraction of mutex contention events
	// that are reported in the mutex profile. Default: 1
	MutexProfileFraction int
}

// DefaultConfig returns a default profiling configuration
func DefaultConfig() *ProfileConfig {
	return &ProfileConfig{
		Enabled:              true,
		ListenAddr:           "localhost:6060",
		BlockProfileRate:     1,
		MutexProfileFraction: 1,
	}
}

// Server manages the profiling HTTP server
type Server struct {
	config *ProfileConfig
	server *http.Server
}

// NewServer creates a new profiling server
func NewServer(config *ProfileConfig) *Server {
	if config == nil {
		config = DefaultConfig()
	}

	// Set profiling rates
	runtime.SetBlockProfileRate(config.BlockProfileRate)
	runtime.SetMutexProfileFraction(config.MutexProfileFraction)

	mux := http.NewServeMux()

	// Register pprof handlers
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	server := &http.Server{
		Addr:    config.ListenAddr,
		Handler: mux,
	}

	return &Server{
		config: config,
		server: server,
	}
}

// Start starts the profiling HTTP server
func (s *Server) Start(ctx context.Context) error {
	if !s.config.Enabled {
		return nil
	}

	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "Profiling server error: %v\n", err)
		}
	}()

	return nil
}

// Stop stops the profiling HTTP server
func (s *Server) Stop(ctx context.Context) error {
	if !s.config.Enabled {
		return nil
	}

	return s.server.Shutdown(ctx)
}

// ProfileRequest represents a request to capture a profile
type ProfileRequest struct {
	// Type is the type of profile to capture
	Type ProfileType

	// Duration is how long to capture the profile (for CPU and trace)
	Duration time.Duration

	// Output is where to write the profile
	Output io.Writer
}

// CaptureProfile captures a profile of the specified type
func CaptureProfile(req *ProfileRequest) error {
	if req.Output == nil {
		return fmt.Errorf("output writer is required")
	}

	switch req.Type {
	case ProfileTypeCPU:
		return captureCPUProfile(req.Output, req.Duration)
	case ProfileTypeHeap:
		return captureHeapProfile(req.Output)
	case ProfileTypeGoroutine:
		return captureGoroutineProfile(req.Output)
	case ProfileTypeMutex:
		return captureMutexProfile(req.Output)
	case ProfileTypeBlock:
		return captureBlockProfile(req.Output)
	case ProfileTypeThreads:
		return captureThreadCreateProfile(req.Output)
	case ProfileTypeAllocs:
		return captureAllocsProfile(req.Output)
	case ProfileTypeTrace:
		return captureTrace(req.Output, req.Duration)
	default:
		return fmt.Errorf("unknown profile type: %s", req.Type)
	}
}

func captureCPUProfile(w io.Writer, duration time.Duration) error {
	if duration == 0 {
		duration = 30 * time.Second
	}

	if err := pprofruntime.StartCPUProfile(w); err != nil {
		return fmt.Errorf("failed to start CPU profile: %w", err)
	}

	wait.ForDuration(duration)
	pprofruntime.StopCPUProfile()
	return nil
}

func captureHeapProfile(w io.Writer) error {
	runtime.GC() // Get up-to-date statistics
	return pprofruntime.WriteHeapProfile(w)
}

func captureGoroutineProfile(w io.Writer) error {
	profile := pprofruntime.Lookup("goroutine")
	if profile == nil {
		return fmt.Errorf("goroutine profile not found")
	}
	return profile.WriteTo(w, 0)
}

func captureMutexProfile(w io.Writer) error {
	profile := pprofruntime.Lookup("mutex")
	if profile == nil {
		return fmt.Errorf("mutex profile not found")
	}
	return profile.WriteTo(w, 0)
}

func captureBlockProfile(w io.Writer) error {
	profile := pprofruntime.Lookup("block")
	if profile == nil {
		return fmt.Errorf("block profile not found")
	}
	return profile.WriteTo(w, 0)
}

func captureThreadCreateProfile(w io.Writer) error {
	profile := pprofruntime.Lookup("threadcreate")
	if profile == nil {
		return fmt.Errorf("threadcreate profile not found")
	}
	return profile.WriteTo(w, 0)
}

func captureAllocsProfile(w io.Writer) error {
	profile := pprofruntime.Lookup("allocs")
	if profile == nil {
		return fmt.Errorf("allocs profile not found")
	}
	return profile.WriteTo(w, 0)
}

func captureTrace(w io.Writer, duration time.Duration) error {
	if duration == 0 {
		duration = 5 * time.Second
	}

	if err := trace.Start(w); err != nil {
		return fmt.Errorf("failed to start trace: %w", err)
	}

	wait.ForDuration(duration)
	trace.Stop()
	return nil
}

// ProfileStats returns current profiling statistics
type ProfileStats struct {
	// NumGoroutine is the number of goroutines
	NumGoroutine int `json:"num_goroutine"`

	// MemStats contains memory statistics
	MemStats runtime.MemStats `json:"mem_stats"`

	// NumCPU is the number of logical CPUs
	NumCPU int `json:"num_cpu"`

	// GOMAXPROCS is the maximum number of CPUs that can execute simultaneously
	GOMAXPROCS int `json:"gomaxprocs"`
}

// GetStats returns current profiling statistics
func GetStats() *ProfileStats {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	return &ProfileStats{
		NumGoroutine: runtime.NumGoroutine(),
		MemStats:     memStats,
		NumCPU:       runtime.NumCPU(),
		GOMAXPROCS:   runtime.GOMAXPROCS(-1),
	}
}
