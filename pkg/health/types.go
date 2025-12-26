package health

import (
	"context"
	"time"
)

// Status represents the health status of a component
type Status string

const (
	StatusHealthy   Status = "healthy"
	StatusDegraded  Status = "degraded"
	StatusUnhealthy Status = "unhealthy"
	StatusUnknown   Status = "unknown"
)

// String returns the string representation of the status
func (s Status) String() string {
	return string(s)
}

// CheckResult represents the result of a health check
type CheckResult struct {
	Status    Status                 `json:"status"`
	Message   string                 `json:"message,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
	Duration  time.Duration          `json:"duration_ms"`
	Details   map[string]interface{} `json:"details,omitempty"`
}

// Checker is the interface that health checks must implement
type Checker interface {
	// Check performs the health check and returns the result
	Check(ctx context.Context) CheckResult

	// Name returns the name of the health check
	Name() string
}

// ComponentStatus represents the health status of a component
type ComponentStatus struct {
	Status  Status                 `json:"status"`
	Message string                 `json:"message,omitempty"`
	Details map[string]interface{} `json:"details,omitempty"`
}

// LivenessResponse is the response for the liveness probe
type LivenessResponse struct {
	Status Status `json:"status"`
}

// ReadinessResponse is the response for the readiness probe
type ReadinessResponse struct {
	Status Status                      `json:"status"`
	Checks map[string]ComponentStatus `json:"checks"`
}

// StatusResponse is the detailed status response
type StatusResponse struct {
	Status     Status                      `json:"status"`
	Version    string                      `json:"version"`
	Uptime     string                      `json:"uptime"`
	StartTime  time.Time                   `json:"start_time"`
	Components map[string]ComponentStatus `json:"components"`
}

// Config holds configuration for health checks
type Config struct {
	// CheckInterval is the interval at which background health checks are performed
	CheckInterval time.Duration

	// CheckTimeout is the timeout for individual health checks
	CheckTimeout time.Duration

	// ReadinessChecks are the checks required for readiness
	ReadinessChecks []string

	// StartupGracePeriod is the duration to wait before marking components unhealthy on startup
	StartupGracePeriod time.Duration
}

// DefaultConfig returns the default health check configuration
func DefaultConfig() *Config {
	return &Config{
		CheckInterval:      30 * time.Second,
		CheckTimeout:       5 * time.Second,
		ReadinessChecks:    []string{"nats", "state_backend"},
		StartupGracePeriod: 30 * time.Second,
	}
}
