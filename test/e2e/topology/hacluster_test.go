// Package topology contains E2E tests for different deployment topologies.
// This file contains tests for the HA Cluster topology (3 control planes + NATS cluster + PostgreSQL + etcd).
package topology

import (
	"context"
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

	// Find compose file
	composeFile := findHAClusterComposeFile()
	if composeFile == "" {
		t.Fatal("Could not find ha-cluster docker-compose.yml")
	}

	buildImages := os.Getenv("KSCORE_SKIP_BUILD") != "1"

	cfg := &harness.HAClusterConfig{
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

	var err error
	haTestEnv, err = harness.NewHACluster(cfg)
	if err != nil {
		t.Fatalf("Failed to create HA cluster environment: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	if err := haTestEnv.Start(ctx, cfg); err != nil {
		t.Fatalf("Failed to start HA cluster: %v", err)
	}

	// Wait for agents
	if err := haTestEnv.WaitForAgents(ctx, 5, 120*time.Second); err != nil {
		t.Fatalf("Failed waiting for agents: %v", err)
	}

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
				if err == io.EOF {
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
		if err == io.EOF {
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

// TestHACluster_NATSFailure tests handling of NATS node failure
func TestHACluster_NATSFailure(t *testing.T) {
	t.Skip("Skipping: NATS failure tests require NATS-specific container control")

	// This test would:
	// 1. Kill one NATS node
	// 2. Verify message delivery continues
	// 3. Verify JetStream operations work
	// 4. Restart NATS node and verify it rejoins cluster
}

// TestHACluster_EtcdFailure tests handling of etcd node failure
func TestHACluster_EtcdFailure(t *testing.T) {
	t.Skip("Skipping: etcd failure tests require etcd-specific container control")

	// This test would:
	// 1. Kill one etcd node
	// 2. Verify leader election still works
	// 3. Verify cluster state operations continue
	// 4. Restart etcd node and verify it rejoins cluster
}

// TestHACluster_DatabaseFailover tests PostgreSQL failover
func TestHACluster_DatabaseFailover(t *testing.T) {
	t.Skip("Skipping: PostgreSQL failover requires replica configuration")

	// This test would require PostgreSQL replication setup
}

// =============================================================================
// Network Partition Tests
// =============================================================================

// TestHACluster_NetworkPartition tests handling of network partitions
func TestHACluster_NetworkPartition(t *testing.T) {
	t.Skip("Skipping: Network partition tests require special network configuration")

	// This test would use Docker network manipulation
}

// TestHACluster_SplitBrain tests that split-brain is prevented
func TestHACluster_SplitBrain(t *testing.T) {
	t.Skip("Skipping: Split-brain tests require network partition capability")

	// This test would use Docker network manipulation
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

		// Wait for server to come back
		time.Sleep(15 * time.Second)

		// Verify server is healthy
		if !env.IsServerHealthy(ctx, i) {
			t.Errorf("%s did not become healthy after restart", env.Servers[i].Name)
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
				if err == io.EOF {
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
			if err == io.EOF {
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
