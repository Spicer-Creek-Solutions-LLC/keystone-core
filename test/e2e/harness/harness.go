// Package harness provides a test harness for container-based E2E testing.
// It manages Docker/Podman containers using docker-compose.
package harness

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
)

// TestEnvironment represents a running E2E test environment
type TestEnvironment struct {
	// ComposeFile is the path to the docker-compose.yml file
	ComposeFile string

	// ProjectName is the docker-compose project name
	ProjectName string

	// ServerGRPCAddr is the address of the control plane gRPC server
	ServerGRPCAddr string

	// ServerHTTPAddr is the address of the control plane HTTP server
	ServerHTTPAddr string

	// WebhookAddr is the address of the webhook receiver
	WebhookAddr string

	// grpcConn is the gRPC connection to the server
	grpcConn *grpc.ClientConn

	// client is the control plane gRPC client
	client pb.ControlPlaneServiceClient

	// httpClient for health checks
	httpClient *http.Client
}

// Config holds configuration for the test environment
type Config struct {
	// ComposeFile is the path to the docker-compose.yml file
	// If empty, defaults to test/e2e/containers/docker-compose.yml
	ComposeFile string

	// ProjectName is the docker-compose project name
	// If empty, defaults to "kscore-e2e-test"
	ProjectName string

	// BuildImages forces rebuilding of container images
	BuildImages bool

	// StartupTimeout is how long to wait for services to start
	StartupTimeout time.Duration

	// ServerGRPCPort is the host port for the gRPC server
	// Default: 8080
	ServerGRPCPort int

	// ServerHTTPPort is the host port for the HTTP server
	// Default: 8081
	ServerHTTPPort int

	// WebhookPort is the host port for the webhook receiver
	// Default: 8082
	WebhookPort int
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	return &Config{
		ProjectName:    "kscore-e2e-test",
		BuildImages:    true,
		StartupTimeout: 120 * time.Second,
		ServerGRPCPort: 8080,
		ServerHTTPPort: 8081,
		WebhookPort:    8082,
	}
}

// New creates a new test environment with the given configuration
func New(cfg *Config) (*TestEnvironment, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	// Find compose file
	composeFile := cfg.ComposeFile
	if composeFile == "" {
		// Try to find it relative to common locations
		candidates := []string{
			"test/e2e/containers/docker-compose.yml",
			"../containers/docker-compose.yml",
			"docker-compose.yml",
		}
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				composeFile = c
				break
			}
		}
	}

	if composeFile == "" {
		return nil, fmt.Errorf("could not find docker-compose.yml")
	}

	absPath, err := filepath.Abs(composeFile)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	return &TestEnvironment{
		ComposeFile:    absPath,
		ProjectName:    cfg.ProjectName,
		ServerGRPCAddr: fmt.Sprintf("localhost:%d", cfg.ServerGRPCPort),
		ServerHTTPAddr: fmt.Sprintf("http://localhost:%d", cfg.ServerHTTPPort),
		WebhookAddr:    fmt.Sprintf("http://localhost:%d", cfg.WebhookPort),
		httpClient:     &http.Client{Timeout: 5 * time.Second},
	}, nil
}

// Start starts the test environment
func (e *TestEnvironment) Start(ctx context.Context, cfg *Config) error {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	// Build and start containers
	args := []string{
		"-f", e.ComposeFile,
		"-p", e.ProjectName,
	}

	// Always clean up any existing containers first to avoid name conflicts
	// This handles cases where previous test runs crashed or containers
	// were started manually outside the test harness
	fmt.Print("Cleaning up any existing containers...")
	downArgs := append(args, "down", "-v", "--remove-orphans")
	_ = e.runCompose(ctx, downArgs...) // Ignore errors - containers might not exist
	fmt.Println(" done")

	if cfg.BuildImages {
		// Build images first (quiet mode to avoid flooding terminal)
		fmt.Print("Building container images...")
		buildArgs := append(args, "build", "-q")
		if err := e.runCompose(ctx, buildArgs...); err != nil {
			fmt.Println(" FAILED")
			return fmt.Errorf("failed to build images: %w", err)
		}
		fmt.Println(" done")
	}

	// Start containers
	fmt.Print("Starting containers...")
	upArgs := append(args, "up", "-d", "--wait")
	if err := e.runCompose(ctx, upArgs...); err != nil {
		fmt.Println(" FAILED")
		return fmt.Errorf("failed to start containers: %w", err)
	}
	fmt.Println(" done")

	// Wait for server to be healthy
	fmt.Print("Waiting for server health check...")
	if err := e.waitForHealthy(ctx, cfg.StartupTimeout); err != nil {
		fmt.Println(" FAILED")
		// Try to get logs for debugging
		_ = e.Logs(ctx, "server")
		return fmt.Errorf("server failed to become healthy: %w", err)
	}
	fmt.Println(" ready")

	// Establish gRPC connection
	conn, err := grpc.DialContext(ctx, e.ServerGRPCAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return fmt.Errorf("failed to connect to gRPC server: %w", err)
	}

	e.grpcConn = conn
	e.client = pb.NewControlPlaneServiceClient(conn)

	return nil
}

// Stop stops and removes the test environment
func (e *TestEnvironment) Stop(ctx context.Context) error {
	// Close gRPC connection
	if e.grpcConn != nil {
		e.grpcConn.Close()
	}

	// Stop and remove containers
	fmt.Print("Stopping containers...")
	args := []string{
		"-f", e.ComposeFile,
		"-p", e.ProjectName,
		"down",
		"-v", // Remove volumes
		"--remove-orphans",
	}

	err := e.runCompose(ctx, args...)
	if err != nil {
		fmt.Println(" FAILED")
	} else {
		fmt.Println(" done")
	}
	return err
}

// Client returns the gRPC client for the control plane
func (e *TestEnvironment) Client() pb.ControlPlaneServiceClient {
	return e.client
}

// GRPCConn returns the raw gRPC connection
func (e *TestEnvironment) GRPCConn() *grpc.ClientConn {
	return e.grpcConn
}

// Logs prints logs from a specific container (last 100 lines)
func (e *TestEnvironment) Logs(ctx context.Context, service string) error {
	fmt.Printf("\n=== Logs from %s (last 100 lines) ===\n", service)
	args := []string{
		"-f", e.ComposeFile,
		"-p", e.ProjectName,
		"logs",
		"--tail", "100",
		"--no-color",
		service,
	}

	// For logs, we actually want to show output
	return e.runComposeWithOutput(ctx, true, args...)
}

// Exec executes a command in a container
func (e *TestEnvironment) Exec(ctx context.Context, service string, command ...string) (string, error) {
	args := []string{
		"-f", e.ComposeFile,
		"-p", e.ProjectName,
		"exec",
		"-T", // No TTY
		service,
	}
	args = append(args, command...)

	cmd := exec.CommandContext(ctx, "docker", append([]string{"compose"}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("exec failed: %w: %s", err, output)
	}

	return strings.TrimSpace(string(output)), nil
}

// WaitForAgents waits for the expected number of agents to register
func (e *TestEnvironment) WaitForAgents(ctx context.Context, expectedCount int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		// List agents
		resp, err := e.client.ListAgents(ctx, &pb.ListAgentsRequest{
			PageSize: 100,
		})
		if err == nil && len(resp.Agents) >= expectedCount {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
			continue
		}
	}

	return fmt.Errorf("timed out waiting for %d agents", expectedCount)
}

// runCompose runs a docker-compose command
// Output is captured and only printed on error to avoid flooding the terminal
func (e *TestEnvironment) runCompose(ctx context.Context, args ...string) error {
	return e.runComposeWithOutput(ctx, false, args...)
}

// runComposeWithOutput runs a docker-compose command
// If showOutput is true, streams output to stdout/stderr
// Otherwise captures output and only prints on error
func (e *TestEnvironment) runComposeWithOutput(ctx context.Context, showOutput bool, args ...string) error {
	// Try docker compose first (Docker Compose V2)
	cmd := exec.CommandContext(ctx, "docker", append([]string{"compose"}, args...)...)

	if showOutput {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	// Capture output, only print on error
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Fall back to docker-compose (legacy)
		cmd = exec.CommandContext(ctx, "docker-compose", args...)
		output, err = cmd.CombinedOutput()
		if err != nil {
			// Print captured output on failure for debugging
			fmt.Fprintf(os.Stderr, "docker-compose failed:\n%s\n", output)
			return err
		}
	}

	return nil
}

// waitForHealthy waits for the server to pass health checks
func (e *TestEnvironment) waitForHealthy(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	healthURL := e.ServerHTTPAddr + "/health/live"

	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(ctx, "GET", healthURL, nil)
		if err != nil {
			return err
		}

		resp, err := e.httpClient.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return nil
		}
		if resp != nil {
			resp.Body.Close()
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(1 * time.Second):
			continue
		}
	}

	return fmt.Errorf("health check timed out after %s", timeout)
}

// Scale scales a service to the specified number of replicas
func (e *TestEnvironment) Scale(ctx context.Context, service string, replicas int) error {
	args := []string{
		"-f", e.ComposeFile,
		"-p", e.ProjectName,
		"up",
		"-d",
		"--scale", fmt.Sprintf("%s=%d", service, replicas),
		service,
	}

	return e.runCompose(ctx, args...)
}
