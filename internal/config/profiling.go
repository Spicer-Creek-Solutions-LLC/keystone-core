// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"time"
)

// ProfilingConfig configures the opt-in pprof endpoint. PROJECT-DETAILS
// §4.16 — "Default off; opt-in via profiling.enabled=true. Listen port
// default 6060." Default Host is 127.0.0.1: pprof can leak heap state
// and bottleneck under CPU profile load, so operators who flip Enabled
// must explicitly widen the bind to reach the LAN.
type ProfilingConfig struct {
	// Enabled toggles the whole pprof listener. Default false. When
	// false the server is never constructed.
	Enabled bool `koanf:"enabled"`

	// Host is the bind address. Default "127.0.0.1" (localhost-only).
	// Operators who want LAN reachability set this to "0.0.0.0" and
	// accept the security responsibility — pprof has no auth and is
	// not appropriate for public exposure.
	Host string `koanf:"host"`

	// Port is the bind port. Default 6060 (conventional Go pprof).
	// Range [1, 65535]; zero is rejected when Enabled.
	Port int `koanf:"port"`

	// MutexProfileFraction is passed to runtime.SetMutexProfileFraction
	// on Start. Default 0 means "no mutex profile" (the runtime default).
	// Non-zero N records 1/N mutex contention events. Has non-trivial
	// overhead; leave at 0 unless actively debugging contention.
	MutexProfileFraction int `koanf:"mutexprofilefraction"`

	// BlockProfileRate is passed to runtime.SetBlockProfileRate on
	// Start. Default 0 means "no block profile" (the runtime default).
	// Non-zero N samples blocking events whose duration exceeds N
	// nanoseconds. Same overhead caveat as MutexProfileFraction.
	BlockProfileRate int `koanf:"blockprofilerate"`

	// ShutdownTimeout caps the graceful-shutdown wait when the server
	// is asked to stop. Default 5 seconds.
	ShutdownTimeout time.Duration `koanf:"shutdowntimeout"`
}

// Validate rejects out-of-range fields when Enabled. Disabled configs
// pass trivially so dev fixtures can omit every field.
func (p ProfilingConfig) Validate() error {
	if !p.Enabled {
		return nil
	}
	if p.Host == "" {
		return fmt.Errorf("profiling.host: must not be empty when enabled")
	}
	if p.Port < 1 || p.Port > 65535 {
		return fmt.Errorf("profiling.port: must be in [1,65535], got %d", p.Port)
	}
	if p.MutexProfileFraction < 0 {
		return fmt.Errorf("profiling.mutexprofilefraction: must be >= 0, got %d", p.MutexProfileFraction)
	}
	if p.BlockProfileRate < 0 {
		return fmt.Errorf("profiling.blockprofilerate: must be >= 0, got %d", p.BlockProfileRate)
	}
	if p.ShutdownTimeout < 0 {
		return fmt.Errorf("profiling.shutdowntimeout: must be >= 0, got %s", p.ShutdownTimeout)
	}
	return nil
}
