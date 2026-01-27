package secrets

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

// HealthChecker verifies target health after secret rotation.
type HealthChecker interface {
	// Check performs a health check on the target.
	// Returns true if healthy, false otherwise.
	Check(ctx context.Context, target *RotationTarget, config *HealthCheckConfig) (bool, error)

	// Type returns the health check type.
	Type() string
}

// HealthCheckResult represents the result of a health check.
type HealthCheckResult struct {
	// TargetID is the target that was checked.
	TargetID string `json:"target_id"`

	// Healthy indicates whether the target is healthy.
	Healthy bool `json:"healthy"`

	// StatusCode is the HTTP status code (for HTTP checks).
	StatusCode int `json:"status_code,omitempty"`

	// ResponseTime is how long the check took.
	ResponseTime time.Duration `json:"response_time"`

	// Error is any error that occurred.
	Error string `json:"error,omitempty"`

	// Attempt is the attempt number (1-indexed).
	Attempt int `json:"attempt"`

	// Timestamp is when the check was performed.
	Timestamp time.Time `json:"timestamp"`
}

// HTTPHealthChecker performs HTTP health checks.
type HTTPHealthChecker struct {
	client *http.Client
}

// NewHTTPHealthChecker creates a new HTTP health checker.
func NewHTTPHealthChecker() *HTTPHealthChecker {
	return &HTTPHealthChecker{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Type returns the health check type.
func (h *HTTPHealthChecker) Type() string {
	return "http"
}

// Check performs an HTTP health check.
func (h *HTTPHealthChecker) Check(ctx context.Context, target *RotationTarget, config *HealthCheckConfig) (bool, error) {
	if config.Endpoint == "" && target.Endpoint == "" {
		return false, fmt.Errorf("no endpoint configured for HTTP health check")
	}

	endpoint := config.Endpoint
	if endpoint == "" {
		endpoint = target.Endpoint
	}

	// Replace target-specific placeholders
	endpoint = strings.ReplaceAll(endpoint, "{target_id}", target.ID)
	endpoint = strings.ReplaceAll(endpoint, "{target_name}", target.Name)
	endpoint = strings.ReplaceAll(endpoint, "{agent_id}", target.AgentID)

	timeout := config.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return false, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return false, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	// Drain body to allow connection reuse
	_, _ = io.Copy(io.Discard, resp.Body)

	expectedStatus := config.ExpectedStatus
	if expectedStatus == 0 {
		expectedStatus = http.StatusOK
	}

	if resp.StatusCode != expectedStatus {
		return false, fmt.Errorf("unexpected status code: got %d, expected %d", resp.StatusCode, expectedStatus)
	}

	return true, nil
}

// TCPHealthChecker performs TCP health checks.
type TCPHealthChecker struct{}

// NewTCPHealthChecker creates a new TCP health checker.
func NewTCPHealthChecker() *TCPHealthChecker {
	return &TCPHealthChecker{}
}

// Type returns the health check type.
func (t *TCPHealthChecker) Type() string {
	return "tcp"
}

// Check performs a TCP health check by attempting to connect.
func (t *TCPHealthChecker) Check(ctx context.Context, target *RotationTarget, config *HealthCheckConfig) (bool, error) {
	if config.Endpoint == "" && target.Endpoint == "" {
		return false, fmt.Errorf("no endpoint configured for TCP health check")
	}

	endpoint := config.Endpoint
	if endpoint == "" {
		endpoint = target.Endpoint
	}

	// Replace target-specific placeholders
	endpoint = strings.ReplaceAll(endpoint, "{target_id}", target.ID)
	endpoint = strings.ReplaceAll(endpoint, "{target_name}", target.Name)
	endpoint = strings.ReplaceAll(endpoint, "{agent_id}", target.AgentID)

	timeout := config.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", endpoint)
	if err != nil {
		return false, fmt.Errorf("TCP connection failed: %w", err)
	}
	defer conn.Close()

	return true, nil
}

// ExecHealthChecker performs health checks by executing commands.
type ExecHealthChecker struct{}

// NewExecHealthChecker creates a new exec health checker.
func NewExecHealthChecker() *ExecHealthChecker {
	return &ExecHealthChecker{}
}

// Type returns the health check type.
func (e *ExecHealthChecker) Type() string {
	return "exec"
}

// Check performs a health check by executing a command.
func (e *ExecHealthChecker) Check(ctx context.Context, target *RotationTarget, config *HealthCheckConfig) (bool, error) {
	if config.Command == "" {
		return false, fmt.Errorf("no command configured for exec health check")
	}

	command := config.Command
	// Replace target-specific placeholders
	command = strings.ReplaceAll(command, "{target_id}", target.ID)
	command = strings.ReplaceAll(command, "{target_name}", target.Name)
	command = strings.ReplaceAll(command, "{agent_id}", target.AgentID)

	timeout := config.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("command failed: %w (output: %s)", err, string(output))
	}

	return true, nil
}

// HealthCheckRegistry manages health checker implementations.
type HealthCheckRegistry struct {
	checkers map[string]HealthChecker
}

// NewHealthCheckRegistry creates a new health check registry with default checkers.
func NewHealthCheckRegistry() *HealthCheckRegistry {
	registry := &HealthCheckRegistry{
		checkers: make(map[string]HealthChecker),
	}

	// Register default checkers
	registry.Register(NewHTTPHealthChecker())
	registry.Register(NewTCPHealthChecker())
	registry.Register(NewExecHealthChecker())

	return registry
}

// Register adds a health checker to the registry.
func (r *HealthCheckRegistry) Register(checker HealthChecker) {
	r.checkers[checker.Type()] = checker
}

// Get returns a health checker by type.
func (r *HealthCheckRegistry) Get(checkType string) (HealthChecker, bool) {
	checker, ok := r.checkers[checkType]
	return checker, ok
}

// CheckTarget performs a health check on a target with retries.
func (r *HealthCheckRegistry) CheckTarget(
	ctx context.Context,
	target *RotationTarget,
	config *HealthCheckConfig,
) (*HealthCheckResult, error) {
	if config == nil {
		return nil, fmt.Errorf("health check config is nil")
	}

	checker, ok := r.Get(config.Type)
	if !ok {
		return nil, fmt.Errorf("unknown health check type: %s", config.Type)
	}

	retries := config.Retries
	if retries < 0 {
		retries = 0
	}

	interval := config.Interval
	if interval <= 0 {
		interval = 5 * time.Second
	}

	var lastErr error
	for attempt := 0; attempt <= retries; attempt++ {
		select {
		case <-ctx.Done():
			return &HealthCheckResult{
				TargetID:  target.ID,
				Healthy:   false,
				Error:     ctx.Err().Error(),
				Attempt:   attempt + 1,
				Timestamp: time.Now(),
			}, ctx.Err()
		default:
		}

		start := time.Now()
		healthy, err := checker.Check(ctx, target, config)
		elapsed := time.Since(start)

		if healthy && err == nil {
			return &HealthCheckResult{
				TargetID:     target.ID,
				Healthy:      true,
				ResponseTime: elapsed,
				Attempt:      attempt + 1,
				Timestamp:    time.Now(),
			}, nil
		}

		lastErr = err
		if attempt < retries {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(interval):
			}
		}
	}

	errMsg := ""
	if lastErr != nil {
		errMsg = lastErr.Error()
	}

	return &HealthCheckResult{
		TargetID:  target.ID,
		Healthy:   false,
		Error:     errMsg,
		Attempt:   retries + 1,
		Timestamp: time.Now(),
	}, lastErr
}

// CheckTargets performs health checks on multiple targets concurrently.
func (r *HealthCheckRegistry) CheckTargets(
	ctx context.Context,
	targets []*RotationTarget,
	config *HealthCheckConfig,
) ([]*HealthCheckResult, error) {
	if config == nil {
		return nil, fmt.Errorf("health check config is nil")
	}

	results := make([]*HealthCheckResult, len(targets))
	errChan := make(chan error, len(targets))

	for i, target := range targets {
		go func(idx int, t *RotationTarget) {
			result, err := r.CheckTarget(ctx, t, config)
			results[idx] = result
			errChan <- err
		}(i, target)
	}

	// Collect all errors
	var errors []error
	for range targets {
		if err := <-errChan; err != nil {
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		return results, fmt.Errorf("%d health checks failed", len(errors))
	}

	return results, nil
}

// Ensure implementations satisfy the interface.
var (
	_ HealthChecker = (*HTTPHealthChecker)(nil)
	_ HealthChecker = (*TCPHealthChecker)(nil)
	_ HealthChecker = (*ExecHealthChecker)(nil)
)
