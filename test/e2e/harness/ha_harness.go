// Package harness provides a test harness for container-based E2E testing.
// This file contains the HA cluster-specific harness for multi-node testing.
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
	"github.com/shawnbutts/keystone-core/test/bootstrap/vm"
)

// HAClusterEnvironment represents a running HA cluster E2E test environment
type HAClusterEnvironment struct {
	mode string

	// ComposeFile is the path to the docker-compose.yml file
	ComposeFile string

	// ProjectName is the docker-compose project name
	ProjectName string

	// Servers holds info about each control plane server
	Servers []ServerInfo

	// ExpectedAgents is the number of agents expected in the cluster
	ExpectedAgents int

	// httpClient for health checks
	httpClient *http.Client

	// grpcConns holds gRPC connections to each server
	grpcConns []*grpc.ClientConn

	// clients holds gRPC clients for each server
	clients []pb.ControlPlaneServiceClient

	vmProvider vm.Provider
}

// ServerInfo contains information about a control plane server
type ServerInfo struct {
	Name     string
	GRPCAddr string
	HTTPAddr string
}

// HAClusterConfig holds configuration for the HA cluster test environment
type HAClusterConfig struct {
	// Mode selects how the environment is provisioned (e.g., "docker" or "vm").
	Mode string

	// ComposeFile is the path to the docker-compose.yml file
	ComposeFile string

	// ProjectName is the docker-compose project name
	ProjectName string

	// BuildImages forces rebuilding of container images
	BuildImages bool

	// StartupTimeout is how long to wait for services to start
	StartupTimeout time.Duration

	// Servers defines the control plane servers
	Servers []ServerInfo

	// ExpectedAgents is the number of agents expected
	ExpectedAgents int

	// VMProvider enables SSH control of VM nodes in VM mode.
	VMProvider vm.Provider
}

// DefaultHAClusterConfig returns the default HA cluster configuration
func DefaultHAClusterConfig() *HAClusterConfig {
	return &HAClusterConfig{
		ProjectName:    "kscore-e2e-ha",
		BuildImages:    true,
		StartupTimeout: 300 * time.Second, // HA cluster takes longer to start
		Servers: []ServerInfo{
			{Name: "server-1", GRPCAddr: "localhost:8080", HTTPAddr: "http://localhost:8081"},
			{Name: "server-2", GRPCAddr: "localhost:8082", HTTPAddr: "http://localhost:8083"},
			{Name: "server-3", GRPCAddr: "localhost:8084", HTTPAddr: "http://localhost:8085"},
		},
		ExpectedAgents: 5,
	}
}

// NewHACluster creates a new HA cluster test environment
func NewHACluster(cfg *HAClusterConfig) (*HAClusterEnvironment, error) {
	if cfg == nil {
		cfg = DefaultHAClusterConfig()
	}

	composeFile := cfg.ComposeFile
	if cfg.Mode != "vm" {
		// Find compose file
		if composeFile == "" {
			candidates := []string{
				"test/e2e/topologies/ha-cluster/docker-compose.yml",
				"../topologies/ha-cluster/docker-compose.yml",
				"../../topologies/ha-cluster/docker-compose.yml",
			}

			if root := os.Getenv("KSCORE_ROOT"); root != "" {
				candidates = append(candidates, filepath.Join(root, "test/e2e/topologies/ha-cluster/docker-compose.yml"))
			}

			for _, c := range candidates {
				if _, err := os.Stat(c); err == nil {
					composeFile = c
					break
				}
			}
		}

		if composeFile == "" {
			return nil, fmt.Errorf("could not find ha-cluster docker-compose.yml")
		}

		absPath, err := filepath.Abs(composeFile)
		if err != nil {
			return nil, fmt.Errorf("failed to get absolute path: %w", err)
		}
		composeFile = absPath
	}

	return &HAClusterEnvironment{
		mode:           cfg.Mode,
		ComposeFile:    composeFile,
		ProjectName:    cfg.ProjectName,
		Servers:        cfg.Servers,
		ExpectedAgents: cfg.ExpectedAgents,
		httpClient:     &http.Client{Timeout: 5 * time.Second},
		grpcConns:      make([]*grpc.ClientConn, len(cfg.Servers)),
		clients:        make([]pb.ControlPlaneServiceClient, len(cfg.Servers)),
		vmProvider:     cfg.VMProvider,
	}, nil
}

// Start starts the HA cluster test environment
func (e *HAClusterEnvironment) Start(ctx context.Context, cfg *HAClusterConfig) error {
	if cfg == nil {
		cfg = DefaultHAClusterConfig()
	}

	if cfg.Mode != "vm" {
		args := []string{
			"-f", e.ComposeFile,
			"-p", e.ProjectName,
		}

		// Always clean up first
		fmt.Print("Cleaning up any existing containers...")
		downArgs := append(args, "down", "-v", "--remove-orphans")
		_ = e.runCompose(ctx, downArgs...)
		fmt.Println(" done")

		if cfg.BuildImages {
			fmt.Print("Building container images (this may take a while)...")
			buildArgs := append(args, "build", "-q")
			if err := e.runCompose(ctx, buildArgs...); err != nil {
				fmt.Println(" FAILED")
				return fmt.Errorf("failed to build images: %w", err)
			}
			fmt.Println(" done")
		}

		// Start containers
		fmt.Print("Starting HA cluster containers...")
		upArgs := append(args, "up", "-d", "--wait")
		if err := e.runCompose(ctx, upArgs...); err != nil {
			fmt.Println(" FAILED")
			return fmt.Errorf("failed to start containers: %w", err)
		}
		fmt.Println(" done")
	} else if e.vmProvider != nil {
		if err := e.vmProvider.Setup(ctx); err != nil {
			return fmt.Errorf("vm provider setup failed: %w", err)
		}
	}

	// Wait for all servers to be healthy
	fmt.Print("Waiting for all control plane servers...")
	for i, server := range e.Servers {
		if err := e.waitForServerHealthy(ctx, server.HTTPAddr, cfg.StartupTimeout); err != nil {
			fmt.Printf(" server-%d FAILED\n", i+1)
			_ = e.Logs(ctx, server.Name)
			return fmt.Errorf("server %s failed to become healthy: %w", server.Name, err)
		}
		fmt.Printf(" server-%d ready", i+1)
	}
	fmt.Println(" - all servers ready")

	// Establish gRPC connections to all servers
	fmt.Print("Establishing gRPC connections...")
	for i, server := range e.Servers {
		conn, err := grpc.DialContext(ctx, server.GRPCAddr,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithBlock(),
		)
		if err != nil {
			fmt.Println(" FAILED")
			return fmt.Errorf("failed to connect to %s: %w", server.Name, err)
		}
		e.grpcConns[i] = conn
		e.clients[i] = pb.NewControlPlaneServiceClient(conn)
	}
	fmt.Println(" done")

	return nil
}

// Stop stops and removes the HA cluster test environment
func (e *HAClusterEnvironment) Stop(ctx context.Context) error {
	// Close all gRPC connections
	for _, conn := range e.grpcConns {
		if conn != nil {
			conn.Close()
		}
	}

	if e.ComposeFile == "" {
		if e.vmProvider != nil {
			return e.vmProvider.Cleanup(ctx)
		}
		return nil
	}

	// Stop and remove containers
	fmt.Print("Stopping HA cluster containers...")
	args := []string{
		"-f", e.ComposeFile,
		"-p", e.ProjectName,
		"down",
		"-v",
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

// Client returns the gRPC client for the primary server (server-1)
func (e *HAClusterEnvironment) Client() pb.ControlPlaneServiceClient {
	return e.clients[0]
}

// ClientForServer returns the gRPC client for a specific server (0-indexed)
func (e *HAClusterEnvironment) ClientForServer(index int) pb.ControlPlaneServiceClient {
	if index < 0 || index >= len(e.clients) {
		return nil
	}
	return e.clients[index]
}

// ServerCount returns the number of control plane servers
func (e *HAClusterEnvironment) ServerCount() int {
	return len(e.Servers)
}

// WaitForAgents waits for the expected number of agents to register
func (e *HAClusterEnvironment) WaitForAgents(ctx context.Context, expectedCount int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		// Try any server
		for _, client := range e.clients {
			if client == nil {
				continue
			}
			resp, err := client.ListAgents(ctx, &pb.ListAgentsRequest{PageSize: 100})
			if err == nil && len(resp.Agents) >= expectedCount {
				return nil
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(1 * time.Second):
			continue
		}
	}

	return fmt.Errorf("timed out waiting for %d agents", expectedCount)
}

// WaitForAllServersHealthy waits for all servers to be healthy
func (e *HAClusterEnvironment) WaitForAllServersHealthy(ctx context.Context, timeout time.Duration) error {
	for _, server := range e.Servers {
		if err := e.waitForServerHealthy(ctx, server.HTTPAddr, timeout); err != nil {
			return fmt.Errorf("server %s not healthy: %w", server.Name, err)
		}
	}
	return nil
}

// IsServerHealthy checks if a specific server is healthy
func (e *HAClusterEnvironment) IsServerHealthy(ctx context.Context, index int) bool {
	if index < 0 || index >= len(e.Servers) {
		return false
	}
	return e.checkServerHealth(ctx, e.Servers[index].HTTPAddr) == nil
}

// StopServer stops a specific server container
func (e *HAClusterEnvironment) StopServer(ctx context.Context, index int) error {
	if index < 0 || index >= len(e.Servers) {
		return fmt.Errorf("invalid server index: %d", index)
	}

	if e.mode == "vm" {
		return e.execServerCommand(ctx, e.Servers[index].Name, "systemctl stop kscore-server")
	}

	args := []string{
		"-f", e.ComposeFile,
		"-p", e.ProjectName,
		"stop", e.Servers[index].Name,
	}

	return e.runCompose(ctx, args...)
}

// StartServer starts a specific server container
func (e *HAClusterEnvironment) StartServer(ctx context.Context, index int) error {
	if index < 0 || index >= len(e.Servers) {
		return fmt.Errorf("invalid server index: %d", index)
	}

	if e.mode == "vm" {
		return e.execServerCommand(ctx, e.Servers[index].Name, "systemctl start kscore-server")
	}

	args := []string{
		"-f", e.ComposeFile,
		"-p", e.ProjectName,
		"start", e.Servers[index].Name,
	}

	return e.runCompose(ctx, args...)
}

// RestartServer restarts a specific server container
func (e *HAClusterEnvironment) RestartServer(ctx context.Context, index int) error {
	if index < 0 || index >= len(e.Servers) {
		return fmt.Errorf("invalid server index: %d", index)
	}

	if e.mode == "vm" {
		return e.execServerCommand(ctx, e.Servers[index].Name, "systemctl restart kscore-server")
	}

	args := []string{
		"-f", e.ComposeFile,
		"-p", e.ProjectName,
		"restart", e.Servers[index].Name,
	}

	return e.runCompose(ctx, args...)
}

// KillServer forcefully kills a specific server container
func (e *HAClusterEnvironment) KillServer(ctx context.Context, index int) error {
	if index < 0 || index >= len(e.Servers) {
		return fmt.Errorf("invalid server index: %d", index)
	}

	if e.mode == "vm" {
		return e.execServerCommand(ctx, e.Servers[index].Name, "pkill -9 kscore-server")
	}

	args := []string{
		"-f", e.ComposeFile,
		"-p", e.ProjectName,
		"kill", e.Servers[index].Name,
	}

	return e.runCompose(ctx, args...)
}

// Logs prints logs from a specific service
func (e *HAClusterEnvironment) Logs(ctx context.Context, service string) error {
	fmt.Printf("\n=== Logs from %s (last 100 lines) ===\n", service)
	args := []string{
		"-f", e.ComposeFile,
		"-p", e.ProjectName,
		"logs",
		"--tail", "100",
		"--no-color",
		service,
	}

	return e.runComposeWithOutput(ctx, true, args...)
}

// ExecuteCommandAndWait executes a command on an agent and waits for completion
func (e *HAClusterEnvironment) ExecuteCommandAndWait(ctx context.Context, agentID, command string, args ...string) (*CommandResult, error) {
	// Use the primary server for command execution
	return executeCommandAndWaitWithClient(ctx, e.clients[0], agentID, command, args...)
}

// executeCommandAndWaitWithClient executes a command using a specific client
func executeCommandAndWaitWithClient(ctx context.Context, client pb.ControlPlaneServiceClient, agentID, command string, args ...string) (*CommandResult, error) {
	stream, err := client.ExecuteCommand(ctx, &pb.ExecuteCommandRequest{
		AgentId: agentID,
		Command: command,
		Args:    args,
	})
	if err != nil {
		return nil, err
	}

	result := &CommandResult{}
	var stdout, stderr strings.Builder

	for {
		resp, err := stream.Recv()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return nil, err
		}

		switch resp.Type {
		case pb.CommandResponseType_COMMAND_RESPONSE_TYPE_STDOUT:
			stdout.Write(resp.Data)
		case pb.CommandResponseType_COMMAND_RESPONSE_TYPE_STDERR:
			stderr.Write(resp.Data)
		case pb.CommandResponseType_COMMAND_RESPONSE_TYPE_COMPLETED:
			result.ExitCode = resp.ExitCode
		case pb.CommandResponseType_COMMAND_RESPONSE_TYPE_FAILED:
			result.ExitCode = resp.ExitCode
		}
	}

	result.Stdout = stdout.String()
	result.Stderr = stderr.String()
	return result, nil
}

// runCompose runs a docker-compose command
func (e *HAClusterEnvironment) runCompose(ctx context.Context, args ...string) error {
	return e.runComposeWithOutput(ctx, false, args...)
}

func (e *HAClusterEnvironment) execServerCommand(ctx context.Context, serverName, command string) error {
	if e.vmProvider == nil {
		return fmt.Errorf("vm provider is not configured")
	}
	node, err := e.vmProvider.GetNode(serverName)
	if err != nil {
		return err
	}
	result, err := node.Exec(ctx, "sh", "-c", command)
	if err != nil {
		return fmt.Errorf("exec %s on %s: %w", command, serverName, err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("exec %s on %s failed: %s", command, serverName, result.Stderr)
	}
	return nil
}

// runComposeWithOutput runs a docker-compose command with optional output
func (e *HAClusterEnvironment) runComposeWithOutput(ctx context.Context, showOutput bool, args ...string) error {
	cmd := exec.CommandContext(ctx, "docker", append([]string{"compose"}, args...)...)

	if showOutput {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Fall back to docker-compose (legacy)
		cmd = exec.CommandContext(ctx, "docker-compose", args...)
		output, err = cmd.CombinedOutput()
		if err != nil {
			fmt.Fprintf(os.Stderr, "docker-compose failed:\n%s\n", output)
			return err
		}
	}

	return nil
}

// waitForServerHealthy waits for a server to pass health checks
func (e *HAClusterEnvironment) waitForServerHealthy(ctx context.Context, httpAddr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	healthURL := httpAddr + "/health/live"

	for time.Now().Before(deadline) {
		if err := e.checkServerHealth(ctx, httpAddr); err == nil {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
			continue
		}
	}

	return fmt.Errorf("health check timed out for %s after %s", healthURL, timeout)
}

// checkServerHealth checks if a server is healthy
func (e *HAClusterEnvironment) checkServerHealth(ctx context.Context, httpAddr string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", httpAddr+"/health/live", nil)
	if err != nil {
		return err
	}

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returned status %d", resp.StatusCode)
	}

	return nil
}
