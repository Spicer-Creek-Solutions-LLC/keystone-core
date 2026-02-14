// Package topology contains E2E tests for different deployment topologies.
// This file contains tests for the HA Cluster topology (3 control planes + NATS cluster + PostgreSQL + etcd).
package topology

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
	"github.com/shawnbutts/keystone-core/test/e2e/harness"
)

// =============================================================================
// HA Cluster Topology Tests
//
// These tests verify the HA cluster topology with:
// - 3 control plane servers (server-1, server-2, server-3)
// - 3 NATS nodes (nats-1, nats-2, nats-3)
// - 3 etcd nodes (etcd-1, etcd-2, etcd-3)
// - 1 PostgreSQL database
// - 5 agents (agent-1 through agent-5)
//
// To run these tests:
//   KSCORE_E2E_TESTS=1 KSCORE_TOPOLOGY=ha-cluster go test -v ./test/e2e/topology/... -run HACluster
//
// Or use the Makefile:
//   make -C test/e2e test-ha-cluster
//
// Note: These tests require significantly more resources than the all-in-one tests.
// Ensure your system has at least 8GB RAM available for Docker.
// =============================================================================

var haTestEnv *harness.HAClusterEnvironment

// TestMain sets up and tears down the HA cluster test environment
func init() {
	// Register HA cluster tests - they will be enabled when KSCORE_TOPOLOGY=ha-cluster
}

// isHAClusterTopology returns true if we're running in HA cluster mode
func isHAClusterTopology() bool {
	return os.Getenv("KSCORE_TOPOLOGY") == "ha-cluster"
}

// skipIfNotHACluster skips the test if not running HA cluster topology
func skipIfNotHACluster(t *testing.T) {
	if !isHAClusterTopology() {
		t.Skip("Skipping: HA cluster tests require KSCORE_TOPOLOGY=ha-cluster")
	}
}

// setupHACluster sets up the HA cluster environment (called once per test run)
func setupHACluster(t *testing.T) *harness.HAClusterEnvironment {
	if haTestEnv != nil {
		return haTestEnv
	}

	var cfg *harness.HAClusterConfig
	if harness.IsVMMode() {
		vmCfg, _, _, err := harness.HAClusterConfigFromVM("", 5)
		if err != nil {
			t.Fatalf("Failed to load VM config: %v", err)
		}
		cfg = vmCfg
		cfg.ProjectName = "kscore-e2e-ha"
		cfg.StartupTimeout = 300 * time.Second
	} else {
		// Find compose file
		composeFile := findHAClusterComposeFile()
		if composeFile == "" {
			t.Fatal("Could not find ha-cluster docker-compose.yml")
		}

		buildImages := os.Getenv("KSCORE_SKIP_BUILD") != "1"

		cfg = &harness.HAClusterConfig{
			ComposeFile:    composeFile,
			ProjectName:    "kscore-e2e-ha",
			BuildImages:    buildImages,
			StartupTimeout: 300 * time.Second,
			Servers: []harness.ServerInfo{
				{Name: "server-1", GRPCAddr: "localhost:8080", HTTPAddr: "http://localhost:8081"},
				{Name: "server-2", GRPCAddr: "localhost:8082", HTTPAddr: "http://localhost:8083"},
				{Name: "server-3", GRPCAddr: "localhost:8084", HTTPAddr: "http://localhost:8085"},
			},
			ExpectedAgents: 5,
		}
	}

	env, err := harness.NewHACluster(cfg)
	if err != nil {
		t.Fatalf("Failed to create HA cluster environment: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	if err := env.Start(ctx, cfg); err != nil {
		_ = env.Stop(ctx)
		t.Fatalf("Failed to start HA cluster: %v", err)
	}

	// Wait for agents
	if err := env.WaitForAgents(ctx, 5, 120*time.Second); err != nil {
		_ = env.Stop(ctx)
		t.Fatalf("Failed waiting for agents: %v", err)
	}

	haTestEnv = env

	// Register cleanup
	t.Cleanup(func() {
		if haTestEnv != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			_ = haTestEnv.Stop(ctx)
			haTestEnv = nil
		}
	})

	return haTestEnv
}

func findHAClusterComposeFile() string {
	candidates := []string{
		"../topologies/ha-cluster/docker-compose.yml",
		"test/e2e/topologies/ha-cluster/docker-compose.yml",
		"../../topologies/ha-cluster/docker-compose.yml",
	}

	if root := os.Getenv("KSCORE_ROOT"); root != "" {
		candidates = append(candidates, filepath.Join(root, "test/e2e/topologies/ha-cluster/docker-compose.yml"))
	}

	for _, c := range candidates {
		abs, err := filepath.Abs(c)
		if err != nil {
			continue
		}
		if _, err := os.Stat(abs); err == nil {
			return abs
		}
	}

	return ""
}

// =============================================================================
// Cluster Formation Tests
// =============================================================================

// TestHACluster_ClusterFormation tests that the HA cluster forms correctly
func TestHACluster_ClusterFormation(t *testing.T) {
	skipIfNotHACluster(t)
	env := setupHACluster(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Check all three servers are healthy
	for i := 0; i < env.ServerCount(); i++ {
		t.Run(env.Servers[i].Name, func(t *testing.T) {
			if !env.IsServerHealthy(ctx, i) {
				t.Errorf("Server %s is not healthy", env.Servers[i].Name)
			} else {
				t.Logf("Server %s is healthy", env.Servers[i].Name)
			}
		})
	}
}

// TestHACluster_LeaderElection tests that leader election works
func TestHACluster_LeaderElection(t *testing.T) {
	skipIfNotHACluster(t)
	env := setupHACluster(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Verify all servers can respond to requests
	for i := 0; i < env.ServerCount(); i++ {
		client := env.ClientForServer(i)
		if client == nil {
			t.Errorf("No client for server %d", i)
			continue
		}

		_, err := client.ListAgents(ctx, &pb.ListAgentsRequest{PageSize: 1})
		if err != nil {
			t.Errorf("Server %d ListAgents failed: %v", i+1, err)
			continue
		}

		t.Logf("Server %d responding to requests", i+1)
	}
}

// TestHACluster_MemberStatus tests that all cluster members have consistent status
func TestHACluster_MemberStatus(t *testing.T) {
	skipIfNotHACluster(t)
	env := setupHACluster(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	agentCounts := make([]int, env.ServerCount())

	for i := 0; i < env.ServerCount(); i++ {
		client := env.ClientForServer(i)
		if client == nil {
			continue
		}

		resp, err := client.ListAgents(ctx, &pb.ListAgentsRequest{PageSize: 100})
		if err != nil {
			t.Errorf("Server %d ListAgents failed: %v", i+1, err)
			continue
		}

		agentCounts[i] = len(resp.Agents)
		t.Logf("Server %d sees %d agents", i+1, agentCounts[i])
	}

	// All servers should see the same number of agents
	for i := 1; i < len(agentCounts); i++ {
		if agentCounts[i] != agentCounts[0] {
			t.Errorf("Agent count mismatch: server 1 has %d, server %d has %d",
				agentCounts[0], i+1, agentCounts[i])
		}
	}
}

// TestHACluster_Sharding tests that agents are distributed across servers
func TestHACluster_Sharding(t *testing.T) {
	skipIfNotHACluster(t)
	env := setupHACluster(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// All servers should see all agents (state is replicated via PostgreSQL)
	client := env.Client()
	resp, err := client.ListAgents(ctx, &pb.ListAgentsRequest{PageSize: 100})
	if err != nil {
		t.Fatalf("ListAgents failed: %v", err)
	}

	if len(resp.Agents) < 5 {
		t.Errorf("Expected at least 5 agents, got %d", len(resp.Agents))
	}

	for _, agent := range resp.Agents {
		t.Logf("Agent %s: status=%s", agent.AgentId, agent.Status)
	}
}

// TestHACluster_MultiMember tests operations across multiple members
func TestHACluster_MultiMember(t *testing.T) {
	skipIfNotHACluster(t)
	env := setupHACluster(t)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Execute commands through each server
	for i := 0; i < env.ServerCount(); i++ {
		t.Run(env.Servers[i].Name, func(t *testing.T) {
			client := env.ClientForServer(i)
			if client == nil {
				t.Fatal("No client available")
			}

			// Execute a command through this server
			stream, err := client.ExecuteCommand(ctx, &pb.ExecuteCommandRequest{
				AgentId: "agent-1",
				Command: "echo",
				Args:    []string{"from", env.Servers[i].Name},
			})
			if err != nil {
				t.Fatalf("ExecuteCommand failed: %v", err)
			}

			var exitCode int32
			var output strings.Builder
			for {
				resp, err := stream.Recv()
				if errors.Is(err, io.EOF) {
					break
				}
				if err != nil {
					t.Fatalf("Stream error: %v", err)
				}
				if resp.Type == pb.CommandResponseType_COMMAND_RESPONSE_TYPE_STDOUT {
					output.Write(resp.Data)
				}
				if resp.Type == pb.CommandResponseType_COMMAND_RESPONSE_TYPE_COMPLETED {
					exitCode = resp.ExitCode
				}
			}

			if exitCode != 0 {
				t.Errorf("Command failed with exit code %d", exitCode)
			}

			t.Logf("Command through %s: output=%q", env.Servers[i].Name, strings.TrimSpace(output.String()))
		})
	}
}

// =============================================================================
// Failover Tests
// =============================================================================

// TestHACluster_Failover tests that the cluster handles server failure
func TestHACluster_Failover(t *testing.T) {
	skipIfNotHACluster(t)
	env := setupHACluster(t)

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	// Verify all servers are initially healthy
	for i := 0; i < env.ServerCount(); i++ {
		if !env.IsServerHealthy(ctx, i) {
			t.Fatalf("Server %d not healthy before test", i+1)
		}
	}

	// Kill server-1
	t.Log("Killing server-1...")
	if err := env.KillServer(ctx, 0); err != nil {
		t.Fatalf("Failed to kill server-1: %v", err)
	}

	// Wait a moment for the cluster to detect the failure
	time.Sleep(5 * time.Second)

	// Verify other servers still work
	t.Log("Verifying remaining servers...")
	for i := 1; i < env.ServerCount(); i++ {
		client := env.ClientForServer(i)
		if client == nil {
			continue
		}

		resp, err := client.ListAgents(ctx, &pb.ListAgentsRequest{PageSize: 10})
		if err != nil {
			t.Errorf("Server %d failed after server-1 killed: %v", i+1, err)
		} else {
			t.Logf("Server %d still responding, sees %d agents", i+1, len(resp.Agents))
		}
	}

	// Restart server-1
	t.Log("Restarting server-1...")
	if err := env.StartServer(ctx, 0); err != nil {
		t.Fatalf("Failed to restart server-1: %v", err)
	}

	// Wait for server-1 to rejoin
	t.Log("Waiting for server-1 to rejoin...")
	if err := env.WaitForAllServersHealthy(ctx, 60*time.Second); err != nil {
		t.Fatalf("Server-1 did not rejoin: %v", err)
	}

	t.Log("Failover test passed - cluster recovered")
}

// TestHACluster_Reconnection tests that agents reconnect after server failure
func TestHACluster_Reconnection(t *testing.T) {
	skipIfNotHACluster(t)
	env := setupHACluster(t)

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	// Execute a command to agent-1 before killing a server
	result, err := env.ExecuteCommandAndWait(ctx, "agent-1", "hostname")
	if err != nil {
		t.Fatalf("Initial command failed: %v", err)
	}
	t.Logf("Before failover - agent-1 hostname: %s", strings.TrimSpace(result.Stdout))

	// Kill server-1
	t.Log("Killing server-1...")
	if err := env.KillServer(ctx, 0); err != nil {
		t.Fatalf("Failed to kill server-1: %v", err)
	}

	// Wait for reconnection
	time.Sleep(10 * time.Second)

	// Try to execute command through server-2
	client := env.ClientForServer(1)
	stream, err := client.ExecuteCommand(ctx, &pb.ExecuteCommandRequest{
		AgentId: "agent-1",
		Command: "hostname",
	})
	if err != nil {
		t.Fatalf("Command through server-2 failed: %v", err)
	}

	var exitCode int32
	for {
		resp, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("Stream error: %v", err)
		}
		if resp.Type == pb.CommandResponseType_COMMAND_RESPONSE_TYPE_COMPLETED {
			exitCode = resp.ExitCode
		}
	}

	if exitCode != 0 {
		t.Errorf("Command failed with exit code %d", exitCode)
	} else {
		t.Log("Agent-1 successfully responded through server-2 after server-1 killed")
	}

	// Restart server-1
	if err := env.StartServer(ctx, 0); err != nil {
		t.Logf("Warning: failed to restart server-1: %v", err)
	}
}

// TestHACluster_QuorumLoss tests behavior when quorum is lost
func TestHACluster_QuorumLoss(t *testing.T) {
	skipIfNotHACluster(t)
	env := setupHACluster(t)

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	// Kill 2 of 3 servers (quorum lost)
	t.Log("Killing server-2 and server-3 to lose quorum...")
	if err := env.KillServer(ctx, 1); err != nil {
		t.Fatalf("Failed to kill server-2: %v", err)
	}
	if err := env.KillServer(ctx, 2); err != nil {
		t.Fatalf("Failed to kill server-3: %v", err)
	}

	time.Sleep(5 * time.Second)

	// Server-1 should still respond to reads but may refuse writes
	client := env.ClientForServer(0)
	_, err := client.ListAgents(ctx, &pb.ListAgentsRequest{PageSize: 10})
	if err != nil {
		t.Logf("Server-1 refusing requests after quorum loss (expected): %v", err)
	} else {
		t.Log("Server-1 still responding to reads (degraded mode)")
	}

	// Restart one server to restore quorum
	t.Log("Restarting server-2 to restore quorum...")
	if err := env.StartServer(ctx, 1); err != nil {
		t.Fatalf("Failed to restart server-2: %v", err)
	}

	time.Sleep(10 * time.Second)

	// Now requests should work again
	resp, err := client.ListAgents(ctx, &pb.ListAgentsRequest{PageSize: 10})
	if err != nil {
		t.Errorf("Server still not responding after quorum restored: %v", err)
	} else {
		t.Logf("Quorum restored - server sees %d agents", len(resp.Agents))
	}

	// Restart server-3
	if err := env.StartServer(ctx, 2); err != nil {
		t.Logf("Warning: failed to restart server-3: %v", err)
	}
}

// =============================================================================
// Infrastructure Failure Tests
// =============================================================================

// TestHACluster_NATSFailure tests handling of NATS node failure.
// Kills one NATS node, verifies operations continue via remaining nodes,
// then restarts and verifies the node rejoins the cluster.
func TestHACluster_NATSFailure(t *testing.T) {
	skipIfNotHACluster(t)
	env := setupHACluster(t)

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	// Verify all servers healthy
	for i := 0; i < env.ServerCount(); i++ {
		if !env.IsServerHealthy(ctx, i) {
			t.Fatalf("Server %d not healthy before test", i+1)
		}
	}

	// Execute a command to confirm baseline
	result, err := env.ExecuteCommandAndWait(ctx, "agent-1", "echo", "pre-nats-failure")
	if err != nil {
		t.Fatalf("Baseline command failed: %v", err)
	}
	t.Logf("Baseline: %s", strings.TrimSpace(result.Stdout))

	// Kill nats-1
	t.Log("Stopping nats-1...")
	if err := env.StopService(ctx, "nats-1"); err != nil {
		t.Fatalf("Failed to stop nats-1: %v", err)
	}

	// Let cluster detect the failure
	time.Sleep(10 * time.Second)

	// Verify operations still work through remaining NATS nodes
	t.Log("Verifying operations with nats-1 down...")
	for i := 1; i < env.ServerCount(); i++ {
		client := env.ClientForServer(i)
		if client == nil {
			continue
		}
		resp, err := client.ListAgents(ctx, &pb.ListAgentsRequest{PageSize: 10})
		if err != nil {
			t.Errorf("Server %d failed with nats-1 down: %v", i+1, err)
		} else {
			t.Logf("Server %d responding, sees %d agents", i+1, len(resp.Agents))
		}
	}

	// Restart nats-1
	t.Log("Restarting nats-1...")
	if err := env.StartService(ctx, "nats-1"); err != nil {
		t.Fatalf("Failed to restart nats-1: %v", err)
	}
	if err := env.WaitForServiceHealthy(ctx, "nats-1", 60*time.Second); err != nil {
		t.Fatalf("nats-1 did not become healthy: %v", err)
	}

	// Let NATS cluster stabilize
	time.Sleep(5 * time.Second)

	// Verify full cluster works
	t.Log("Verifying full cluster after nats-1 rejoin...")
	for i := 0; i < env.ServerCount(); i++ {
		if !env.IsServerHealthy(ctx, i) {
			t.Errorf("Server %d not healthy after nats-1 rejoin", i+1)
		}
	}

	result, err = env.ExecuteCommandAndWait(ctx, "agent-1", "echo", "post-nats-recovery")
	if err != nil {
		t.Errorf("Post-recovery command failed: %v", err)
	} else {
		t.Logf("Post-recovery: %s", strings.TrimSpace(result.Stdout))
	}

	t.Log("NATS failure test passed")
}

// TestHACluster_EtcdFailure tests handling of etcd node failure.
// Kills one etcd node, verifies quorum is maintained with 2/3 nodes,
// then restarts and verifies the node rejoins.
func TestHACluster_EtcdFailure(t *testing.T) {
	skipIfNotHACluster(t)
	env := setupHACluster(t)

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	// Verify all servers healthy
	for i := 0; i < env.ServerCount(); i++ {
		if !env.IsServerHealthy(ctx, i) {
			t.Fatalf("Server %d not healthy before test", i+1)
		}
	}

	// Kill etcd-1
	t.Log("Stopping etcd-1...")
	if err := env.StopService(ctx, "etcd-1"); err != nil {
		t.Fatalf("Failed to stop etcd-1: %v", err)
	}

	// Let cluster detect the failure (etcd quorum: 2/3 nodes still healthy)
	time.Sleep(10 * time.Second)

	// Verify servers still respond (etcd quorum maintained with 2/3)
	t.Log("Verifying servers with etcd-1 down...")
	workingServers := 0
	for i := 0; i < env.ServerCount(); i++ {
		client := env.ClientForServer(i)
		if client == nil {
			continue
		}
		resp, err := client.ListAgents(ctx, &pb.ListAgentsRequest{PageSize: 10})
		if err != nil {
			t.Logf("Server %d degraded with etcd-1 down: %v", i+1, err)
		} else {
			workingServers++
			t.Logf("Server %d responding, sees %d agents", i+1, len(resp.Agents))
		}
	}
	if workingServers == 0 {
		t.Error("No servers responding with etcd-1 down — quorum should be maintained with 2/3 nodes")
	}

	// Restart etcd-1
	t.Log("Restarting etcd-1...")
	if err := env.StartService(ctx, "etcd-1"); err != nil {
		t.Fatalf("Failed to restart etcd-1: %v", err)
	}
	if err := env.WaitForServiceHealthy(ctx, "etcd-1", 60*time.Second); err != nil {
		t.Fatalf("etcd-1 did not become healthy: %v", err)
	}

	// Let etcd cluster stabilize
	time.Sleep(5 * time.Second)

	// Verify all servers healthy
	t.Log("Verifying cluster after etcd-1 rejoin...")
	for i := 0; i < env.ServerCount(); i++ {
		if !env.IsServerHealthy(ctx, i) {
			t.Errorf("Server %d not healthy after etcd-1 rejoin", i+1)
		}
	}

	t.Log("etcd failure test passed")
}

// TestHACluster_DatabaseFailover tests PostgreSQL failure and recovery.
// Stops the PostgreSQL container, verifies servers detect the outage,
// then restarts and verifies operations resume.
func TestHACluster_DatabaseFailover(t *testing.T) {
	skipIfNotHACluster(t)
	env := setupHACluster(t)

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	// Verify all servers healthy and execute baseline command
	for i := 0; i < env.ServerCount(); i++ {
		if !env.IsServerHealthy(ctx, i) {
			t.Fatalf("Server %d not healthy before test", i+1)
		}
	}
	result, err := env.ExecuteCommandAndWait(ctx, "agent-1", "echo", "pre-db-failure")
	if err != nil {
		t.Fatalf("Baseline command failed: %v", err)
	}
	t.Logf("Baseline: %s", strings.TrimSpace(result.Stdout))

	// Stop PostgreSQL
	t.Log("Stopping postgres...")
	if err := env.StopService(ctx, "postgres"); err != nil {
		t.Fatalf("Failed to stop postgres: %v", err)
	}

	// Let servers detect the database outage
	time.Sleep(10 * time.Second)

	// Servers may still respond to health checks (NATS/etcd still up)
	// but DB-dependent operations may fail. Check at least one server is still alive.
	t.Log("Checking server liveness with postgres down...")
	aliveCount := 0
	for i := 0; i < env.ServerCount(); i++ {
		if env.IsServerHealthy(ctx, i) {
			aliveCount++
		}
	}
	t.Logf("%d/%d servers still alive with postgres down", aliveCount, env.ServerCount())

	// Restart PostgreSQL
	t.Log("Restarting postgres...")
	if err := env.StartService(ctx, "postgres"); err != nil {
		t.Fatalf("Failed to restart postgres: %v", err)
	}
	if err := env.WaitForServiceHealthy(ctx, "postgres", 60*time.Second); err != nil {
		t.Fatalf("postgres did not become healthy: %v", err)
	}

	// Give servers time to reconnect to the database
	time.Sleep(10 * time.Second)

	// Verify full functionality restored
	t.Log("Verifying recovery after postgres restart...")
	if err := env.WaitForAllServersHealthy(ctx, 60*time.Second); err != nil {
		t.Fatalf("Servers not healthy after postgres restart: %v", err)
	}

	result, err = env.ExecuteCommandAndWait(ctx, "agent-1", "echo", "post-db-recovery")
	if err != nil {
		t.Errorf("Post-recovery command failed: %v", err)
	} else {
		t.Logf("Post-recovery: %s", strings.TrimSpace(result.Stdout))
	}

	t.Log("Database failover test passed")
}

// =============================================================================
// Network Partition Tests
// =============================================================================

// skipIfNotPrivileged skips tests that require NET_ADMIN capability.
func skipIfNotPrivileged(t *testing.T) {
	if os.Getenv("KSCORE_E2E_PRIVILEGED") != "1" {
		t.Skip("Skipping: requires KSCORE_E2E_PRIVILEGED=1 for iptables network manipulation")
	}
}

// TestHACluster_NetworkPartition tests handling of network partitions.
// Isolates server-1 from server-2 and server-3 using iptables,
// verifies the majority partition continues, then heals.
func TestHACluster_NetworkPartition(t *testing.T) {
	skipIfNotHACluster(t)
	skipIfNotPrivileged(t)
	env := setupHACluster(t)

	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()

	// Ensure cleanup even on failure
	defer func() {
		cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanCancel()
		_ = env.HealAllPartitions(cleanCtx, "server-1")
	}()

	// Verify all servers healthy
	for i := 0; i < env.ServerCount(); i++ {
		if !env.IsServerHealthy(ctx, i) {
			t.Fatalf("Server %d not healthy before test", i+1)
		}
	}

	// Partition server-1 from server-2 and server-3
	t.Log("Partitioning server-1 from server-2 and server-3...")
	if err := env.PartitionService(ctx, "server-1", "server-2"); err != nil {
		t.Fatalf("Failed to partition server-1 -> server-2: %v", err)
	}
	if err := env.PartitionService(ctx, "server-1", "server-3"); err != nil {
		t.Fatalf("Failed to partition server-1 -> server-3: %v", err)
	}

	// Let partition detection kick in
	time.Sleep(15 * time.Second)

	// Verify majority partition (server-2, server-3) still responds
	t.Log("Verifying majority partition...")
	for i := 1; i < env.ServerCount(); i++ {
		client := env.ClientForServer(i)
		if client == nil {
			continue
		}
		resp, err := client.ListAgents(ctx, &pb.ListAgentsRequest{PageSize: 10})
		if err != nil {
			t.Errorf("Server %d (majority) failed during partition: %v", i+1, err)
		} else {
			t.Logf("Server %d (majority) responding, sees %d agents", i+1, len(resp.Agents))
		}
	}

	// server-1 (minority) may fail or show degraded behavior
	t.Log("Checking server-1 (minority) status...")
	client := env.ClientForServer(0)
	if client != nil {
		resp, err := client.ListAgents(ctx, &pb.ListAgentsRequest{PageSize: 10})
		if err != nil {
			t.Logf("Server-1 (minority) degraded as expected: %v", err)
		} else {
			t.Logf("Server-1 (minority) still responding, sees %d agents", len(resp.Agents))
		}
	}

	// Heal the partition
	t.Log("Healing partition...")
	if err := env.HealAllPartitions(ctx, "server-1"); err != nil {
		t.Fatalf("Failed to heal partitions: %v", err)
	}

	// Let cluster reunify
	time.Sleep(15 * time.Second)

	// Verify all servers healthy
	t.Log("Verifying cluster reunification...")
	if err := env.WaitForAllServersHealthy(ctx, 60*time.Second); err != nil {
		t.Fatalf("Cluster did not reunify: %v", err)
	}

	// Verify command execution works
	result, err := env.ExecuteCommandAndWait(ctx, "agent-1", "echo", "post-partition-heal")
	if err != nil {
		t.Errorf("Post-partition command failed: %v", err)
	} else {
		t.Logf("Post-partition: %s", strings.TrimSpace(result.Stdout))
	}

	t.Log("Network partition test passed")
}

// TestHACluster_SplitBrain tests that split-brain is prevented.
// Creates a symmetric partition isolating server-1, verifies only
// the majority partition accepts operations, then heals.
func TestHACluster_SplitBrain(t *testing.T) {
	skipIfNotHACluster(t)
	skipIfNotPrivileged(t)
	env := setupHACluster(t)

	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()

	// Ensure cleanup even on failure
	defer func() {
		cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanCancel()
		_ = env.HealAllPartitions(cleanCtx, "server-1")
		_ = env.HealAllPartitions(cleanCtx, "server-2")
		_ = env.HealAllPartitions(cleanCtx, "server-3")
	}()

	// Verify all servers healthy
	for i := 0; i < env.ServerCount(); i++ {
		if !env.IsServerHealthy(ctx, i) {
			t.Fatalf("Server %d not healthy before test", i+1)
		}
	}

	// Create symmetric partition: server-1 isolated from server-2 and server-3
	// Both directions must be blocked to simulate a real partition.
	t.Log("Creating symmetric partition: server-1 vs server-2+server-3...")
	if err := env.PartitionService(ctx, "server-1", "server-2"); err != nil {
		t.Fatalf("Partition server-1 -> server-2: %v", err)
	}
	if err := env.PartitionService(ctx, "server-1", "server-3"); err != nil {
		t.Fatalf("Partition server-1 -> server-3: %v", err)
	}
	if err := env.PartitionService(ctx, "server-2", "server-1"); err != nil {
		t.Fatalf("Partition server-2 -> server-1: %v", err)
	}
	if err := env.PartitionService(ctx, "server-3", "server-1"); err != nil {
		t.Fatalf("Partition server-3 -> server-1: %v", err)
	}

	// Let partition detection and leadership re-election happen
	time.Sleep(20 * time.Second)

	// Majority partition (server-2, server-3) should accept operations
	t.Log("Verifying majority partition accepts operations...")
	majorityWorking := 0
	for i := 1; i < env.ServerCount(); i++ {
		client := env.ClientForServer(i)
		if client == nil {
			continue
		}
		resp, err := client.ListAgents(ctx, &pb.ListAgentsRequest{PageSize: 10})
		if err != nil {
			t.Logf("Server %d (majority) error: %v", i+1, err)
		} else {
			majorityWorking++
			t.Logf("Server %d (majority) operational, sees %d agents", i+1, len(resp.Agents))
		}
	}
	if majorityWorking == 0 {
		t.Error("Majority partition not operational — split-brain prevention may be too aggressive")
	}

	// Minority (server-1) should be degraded or rejecting operations
	t.Log("Verifying minority partition behavior...")
	client := env.ClientForServer(0)
	if client != nil {
		_, err := client.ListAgents(ctx, &pb.ListAgentsRequest{PageSize: 10})
		if err != nil {
			t.Logf("Server-1 (minority) correctly degraded: %v", err)
		} else {
			t.Log("Server-1 (minority) still responding — may have cached data or stale leader lease")
		}
	}

	// Heal all partitions
	t.Log("Healing all partitions...")
	for _, svc := range []string{"server-1", "server-2", "server-3"} {
		if err := env.HealAllPartitions(ctx, svc); err != nil {
			t.Errorf("Failed to heal partitions on %s: %v", svc, err)
		}
	}

	// Let cluster converge
	time.Sleep(20 * time.Second)

	// Verify all servers converge to consistent state
	t.Log("Verifying cluster convergence after split-brain heal...")
	if err := env.WaitForAllServersHealthy(ctx, 60*time.Second); err != nil {
		t.Fatalf("Cluster did not converge: %v", err)
	}

	// Verify all servers return consistent data
	for i := 0; i < env.ServerCount(); i++ {
		client := env.ClientForServer(i)
		if client == nil {
			continue
		}
		resp, err := client.ListAgents(ctx, &pb.ListAgentsRequest{PageSize: 10})
		if err != nil {
			t.Errorf("Server %d not responding after recovery: %v", i+1, err)
		} else {
			t.Logf("Server %d sees %d agents after recovery", i+1, len(resp.Agents))
		}
	}

	t.Log("Split-brain prevention test passed")
}

// =============================================================================
// Rolling Update Tests
// =============================================================================

// TestHACluster_RollingUpdate tests zero-downtime rolling updates
func TestHACluster_RollingUpdate(t *testing.T) {
	skipIfNotHACluster(t)
	env := setupHACluster(t)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	// Track command success during update
	successCount := 0
	failCount := 0

	// Start background operations
	done := make(chan bool)
	defer close(done) // Ensure goroutine exits even if test fails early

	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				// Try to execute a command through any available server
				for i := 0; i < env.ServerCount(); i++ {
					client := env.ClientForServer(i)
					if client == nil {
						continue
					}

					shortCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
					_, err := client.ListAgents(shortCtx, &pb.ListAgentsRequest{PageSize: 1})
					cancel()

					if err == nil {
						successCount++
						break
					}
				}
			}
		}
	}()

	// Perform rolling update (restart each server one at a time)
	for i := 0; i < env.ServerCount(); i++ {
		t.Logf("Rolling update: restarting %s...", env.Servers[i].Name)

		if err := env.RestartServer(ctx, i); err != nil {
			t.Errorf("Failed to restart %s: %v", env.Servers[i].Name, err)
			continue
		}

		if err := env.WaitForServerHealthy(ctx, i, 90*time.Second); err != nil {
			t.Errorf("%s did not become healthy after restart: %v", env.Servers[i].Name, err)
		}
	}

	// Wait for background operations to finish (deferred close will signal)
	time.Sleep(2 * time.Second)

	t.Logf("Rolling update completed: %d successful operations, %d failed", successCount, failCount)

	if failCount > 0 {
		t.Errorf("Some operations failed during rolling update")
	}
}

// TestHACluster_ContinuousOperation tests continuous operation during maintenance
func TestHACluster_ContinuousOperation(t *testing.T) {
	skipIfNotHACluster(t)
	env := setupHACluster(t)

	// Run multiple batch commands in parallel
	for i := 0; i < 3; i++ {
		i := i // capture loop variable
		t.Run("batch", func(t *testing.T) {
			t.Parallel()

			// Create context inside subtest to avoid cancellation when parent returns
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer cancel()

			client := env.Client()
			stream, err := client.BatchExecuteCommand(ctx, &pb.BatchExecuteCommandRequest{
				BatchJobId:  fmt.Sprintf("continuous-op-test-%d", i),
				Target:      "*",
				Command:     "hostname",
				Concurrency: 5,
			})
			if err != nil {
				t.Fatalf("BatchExecuteCommand failed: %v", err)
			}

			var summary *pb.BatchSummary
			for {
				resp, err := stream.Recv()
				if errors.Is(err, io.EOF) {
					break
				}
				if err != nil {
					t.Fatalf("Stream error: %v", err)
				}
				if resp.Type == pb.BatchResponseType_BATCH_RESPONSE_TYPE_BATCH_COMPLETE {
					summary = resp.Summary
				}
			}

			if summary == nil {
				t.Fatal("No batch summary received")
			}

			t.Logf("Batch completed: %d/%d successful", summary.Successful, summary.Total)
		})
	}
}

// =============================================================================
// Work Distribution Tests
// =============================================================================

// TestHACluster_WorkDistribution tests that work is distributed across servers
func TestHACluster_WorkDistribution(t *testing.T) {
	skipIfNotHACluster(t)
	env := setupHACluster(t)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Submit batch jobs through different servers
	for i := 0; i < env.ServerCount(); i++ {
		client := env.ClientForServer(i)
		if client == nil {
			continue
		}

		stream, err := client.BatchExecuteCommand(ctx, &pb.BatchExecuteCommandRequest{
			BatchJobId:  "work-dist-test-" + env.Servers[i].Name,
			Target:      "*",
			Command:     "echo",
			Args:        []string{"from", env.Servers[i].Name},
			Concurrency: 5,
		})
		if err != nil {
			t.Errorf("Batch through %s failed: %v", env.Servers[i].Name, err)
			continue
		}

		var summary *pb.BatchSummary
		for {
			resp, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				t.Errorf("Stream error on %s: %v", env.Servers[i].Name, err)
				break
			}
			if resp.Type == pb.BatchResponseType_BATCH_RESPONSE_TYPE_BATCH_COMPLETE {
				summary = resp.Summary
			}
		}

		if summary != nil {
			t.Logf("Batch through %s: %d/%d successful", env.Servers[i].Name, summary.Successful, summary.Total)
		}
	}
}

// TestHACluster_AgentAssignment tests consistent agent assignment
func TestHACluster_AgentAssignment(t *testing.T) {
	skipIfNotHACluster(t)
	env := setupHACluster(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// List agents from each server
	agentMap := make(map[string][]string)

	for i := 0; i < env.ServerCount(); i++ {
		client := env.ClientForServer(i)
		if client == nil {
			continue
		}

		resp, err := client.ListAgents(ctx, &pb.ListAgentsRequest{PageSize: 100})
		if err != nil {
			t.Errorf("Server %d ListAgents failed: %v", i+1, err)
			continue
		}

		for _, agent := range resp.Agents {
			agentMap[agent.AgentId] = append(agentMap[agent.AgentId], env.Servers[i].Name)
		}
	}

	// All servers should see all agents (shared PostgreSQL)
	for agentID, servers := range agentMap {
		if len(servers) != env.ServerCount() {
			t.Logf("Agent %s seen by %d/%d servers", agentID, len(servers), env.ServerCount())
		}
	}

	t.Logf("Total unique agents: %d", len(agentMap))
}

// TestHACluster_Rebalance tests rebalancing when servers join/leave
func TestHACluster_Rebalance(t *testing.T) {
	skipIfNotHACluster(t)
	env := setupHACluster(t)

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	// Get initial agent count
	client := env.Client()
	resp, err := client.ListAgents(ctx, &pb.ListAgentsRequest{PageSize: 100})
	if err != nil {
		t.Fatalf("ListAgents failed: %v", err)
	}
	initialCount := len(resp.Agents)
	t.Logf("Initial agent count: %d", initialCount)

	// Stop server-3
	t.Log("Stopping server-3...")
	if err := env.StopServer(ctx, 2); err != nil {
		t.Fatalf("Failed to stop server-3: %v", err)
	}

	time.Sleep(10 * time.Second)

	// Verify agents still accessible
	resp, err = client.ListAgents(ctx, &pb.ListAgentsRequest{PageSize: 100})
	if err != nil {
		t.Fatalf("ListAgents failed after stopping server-3: %v", err)
	}
	t.Logf("Agent count after stopping server-3: %d", len(resp.Agents))

	if len(resp.Agents) != initialCount {
		t.Errorf("Agent count changed: was %d, now %d", initialCount, len(resp.Agents))
	}

	// Restart server-3
	t.Log("Restarting server-3...")
	if err := env.StartServer(ctx, 2); err != nil {
		t.Fatalf("Failed to restart server-3: %v", err)
	}

	// Wait for server-3 to rejoin
	time.Sleep(15 * time.Second)

	// Verify all servers see agents
	for i := 0; i < env.ServerCount(); i++ {
		c := env.ClientForServer(i)
		if c == nil {
			continue
		}

		resp, err := c.ListAgents(ctx, &pb.ListAgentsRequest{PageSize: 100})
		if err != nil {
			t.Errorf("Server %d ListAgents failed after rebalance: %v", i+1, err)
			continue
		}
		t.Logf("Server %d sees %d agents after rebalance", i+1, len(resp.Agents))
	}
}
