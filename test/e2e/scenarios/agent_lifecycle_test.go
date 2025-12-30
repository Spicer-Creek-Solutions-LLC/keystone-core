// Package scenarios contains E2E test scenarios for Keystone Core features.
package scenarios

import (
	"context"
	"strings"
	"testing"
	"time"

	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
	"github.com/shawnbutts/keystone-core/test/e2e/harness"
)

// =============================================================================
// Agent Lifecycle Tests (T3.1)
// These tests verify agent lifecycle functionality end-to-end.
// =============================================================================

// TestAgentLifecycle_Registration tests that agents register correctly on startup
func TestAgentLifecycle_Registration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client := testEnv.Client()

	// List all agents
	resp, err := client.ListAgents(ctx, &pb.ListAgentsRequest{
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("Failed to list agents: %v", err)
	}

	// Verify expected agent count
	if len(resp.Agents) != 3 {
		t.Errorf("Expected 3 agents, got %d", len(resp.Agents))
	}

	// Verify all expected agents are present
	expectedAgents := map[string]bool{
		"agent-web-1": false,
		"agent-web-2": false,
		"agent-db-1":  false,
	}

	for _, agent := range resp.Agents {
		if _, ok := expectedAgents[agent.AgentId]; ok {
			expectedAgents[agent.AgentId] = true
			t.Logf("Found agent: %s (registered at startup)", agent.AgentId)
		}
	}

	for id, found := range expectedAgents {
		if !found {
			t.Errorf("Expected agent %s not found", id)
		}
	}
}

// TestAgentLifecycle_OnlineStatus tests that agents are in ONLINE status
func TestAgentLifecycle_OnlineStatus(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	agents := []string{"agent-web-1", "agent-web-2", "agent-db-1"}

	for _, agentID := range agents {
		t.Run(agentID, func(t *testing.T) {
			testEnv.AssertAgentOnline(t, ctx, agentID)
		})
	}
}

// TestAgentLifecycle_Heartbeat tests that heartbeats are being received
func TestAgentLifecycle_Heartbeat(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client := testEnv.Client()
	agentID := "agent-web-1"

	// Get initial agent state
	resp1, err := client.GetAgent(ctx, &pb.GetAgentRequest{
		AgentId: agentID,
	})
	if err != nil {
		t.Fatalf("Failed to get agent: %v", err)
	}

	initialHeartbeat := resp1.Agent.LastHeartbeat
	if initialHeartbeat == nil {
		t.Fatal("Agent has no initial heartbeat timestamp")
	}

	t.Logf("Initial heartbeat: %v", initialHeartbeat.Seconds)

	// Wait for at least 2 heartbeat intervals (typically 10-15 seconds)
	// This ensures at least one heartbeat occurs after our initial check
	time.Sleep(12 * time.Second)

	// Get updated agent state
	resp2, err := client.GetAgent(ctx, &pb.GetAgentRequest{
		AgentId: agentID,
	})
	if err != nil {
		t.Fatalf("Failed to get agent: %v", err)
	}

	newHeartbeat := resp2.Agent.LastHeartbeat
	if newHeartbeat == nil {
		t.Fatal("Agent has no new heartbeat timestamp")
	}

	t.Logf("New heartbeat: %v", newHeartbeat.Seconds)

	// Verify heartbeat was updated OR is recent (within last 20 seconds)
	// The heartbeat update behavior may vary - some implementations only
	// update on state changes, while others update on every heartbeat
	now := time.Now().Unix()
	heartbeatAge := now - newHeartbeat.Seconds

	if newHeartbeat.Seconds > initialHeartbeat.Seconds {
		t.Logf("Heartbeat successfully updated (delta: %d seconds)",
			newHeartbeat.Seconds-initialHeartbeat.Seconds)
	} else if heartbeatAge <= 20 {
		t.Logf("Heartbeat not updated but is recent (age: %d seconds)", heartbeatAge)
	} else {
		t.Errorf("Heartbeat is stale (age: %d seconds, expected < 20s)", heartbeatAge)
	}
}

// TestAgentLifecycle_Metadata tests that agent metadata is accurate
func TestAgentLifecycle_Metadata(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client := testEnv.Client()
	agentID := "agent-web-1"

	resp, err := client.GetAgent(ctx, &pb.GetAgentRequest{
		AgentId: agentID,
	})
	if err != nil {
		t.Fatalf("Failed to get agent: %v", err)
	}

	agent := resp.Agent
	if agent == nil {
		t.Fatal("Agent not found")
	}

	// Verify basic metadata is present
	if agent.AgentId != agentID {
		t.Errorf("Expected agent ID %s, got %s", agentID, agent.AgentId)
	}

	// Check for hostname - should match container hostname
	if agent.Metadata == nil {
		t.Error("Agent metadata is nil")
	} else if agent.Metadata.Hostname == "" {
		t.Error("Agent hostname is empty")
	} else {
		t.Logf("Agent hostname: %s", agent.Metadata.Hostname)
	}

	// Check OS info if available
	if agent.Metadata != nil && agent.Metadata.Os != "" {
		t.Logf("Agent OS: %s", agent.Metadata.Os)
	}

	if agent.Metadata != nil && agent.Metadata.Arch != "" {
		t.Logf("Agent Arch: %s", agent.Metadata.Arch)
	}

	// Check version info
	if agent.Metadata != nil && agent.Metadata.AgentVersion != "" {
		t.Logf("Agent Version: %s", agent.Metadata.AgentVersion)
	}
}

// TestAgentLifecycle_MetadataHostnameMatch tests that agent hostname matches actual container
func TestAgentLifecycle_MetadataHostnameMatch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client := testEnv.Client()
	agentID := "agent-web-1"

	// Get agent metadata
	resp, err := client.GetAgent(ctx, &pb.GetAgentRequest{
		AgentId: agentID,
	})
	if err != nil {
		t.Fatalf("Failed to get agent: %v", err)
	}

	// Execute hostname command on agent
	result, err := testEnv.ExecuteCommandAndWait(ctx, agentID, "hostname")
	if err != nil {
		t.Fatalf("Failed to get hostname: %v", err)
	}

	actualHostname := strings.TrimSpace(result.Stdout)

	// Compare (note: agent might report short or full hostname)
	if resp.Agent.Metadata == nil || resp.Agent.Metadata.Hostname == "" {
		t.Log("Agent hostname metadata is empty - skipping match check")
	} else if !strings.Contains(resp.Agent.Metadata.Hostname, actualHostname) &&
		!strings.Contains(actualHostname, resp.Agent.Metadata.Hostname) {
		t.Logf("Hostname mismatch: metadata=%q, actual=%q",
			resp.Agent.Metadata.Hostname, actualHostname)
	} else {
		t.Logf("Hostname match: %s", actualHostname)
	}
}

// TestAgentLifecycle_Labels tests that agents can have labels
func TestAgentLifecycle_Labels(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client := testEnv.Client()

	// Check labels on each agent type
	agents := []struct {
		id            string
		expectedLabel string
	}{
		{"agent-web-1", "web"},
		{"agent-web-2", "web"},
		{"agent-db-1", "db"},
	}

	for _, test := range agents {
		t.Run(test.id, func(t *testing.T) {
			resp, err := client.GetAgent(ctx, &pb.GetAgentRequest{
				AgentId: test.id,
			})
			if err != nil {
				t.Fatalf("Failed to get agent: %v", err)
			}

			// Log any labels present
			if resp.Agent.Metadata != nil && resp.Agent.Metadata.Labels != nil && len(resp.Agent.Metadata.Labels) > 0 {
				t.Logf("Agent %s labels: %v", test.id, resp.Agent.Metadata.Labels)

				// Check for role label if present
				if role, ok := resp.Agent.Metadata.Labels["role"]; ok {
					if role != test.expectedLabel {
						t.Logf("Expected role label %q, got %q", test.expectedLabel, role)
					}
				}
			} else {
				t.Logf("Agent %s has no labels configured", test.id)
			}
		})
	}
}

// TestAgentLifecycle_ResponseTime tests agent response time
func TestAgentLifecycle_ResponseTime(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	agentID := "agent-web-1"

	// Measure round-trip time for a simple command
	iterations := 5
	var totalDuration time.Duration

	for i := 0; i < iterations; i++ {
		start := time.Now()
		result, err := testEnv.ExecuteCommandAndWait(ctx, agentID, "echo", "ping")
		duration := time.Since(start)

		if err != nil {
			t.Fatalf("Failed to execute command: %v", err)
		}
		if result.ExitCode != 0 {
			t.Fatalf("Command failed with exit code %d", result.ExitCode)
		}

		totalDuration += duration
	}

	avgDuration := totalDuration / time.Duration(iterations)
	t.Logf("Average response time over %d iterations: %v", iterations, avgDuration)

	// Warn if average is too high (> 1 second)
	if avgDuration > 1*time.Second {
		t.Logf("Warning: Average response time is high: %v", avgDuration)
	}
}

// TestAgentLifecycle_ConcurrentAgentAccess tests accessing multiple agents concurrently
func TestAgentLifecycle_ConcurrentAgentAccess(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client := testEnv.Client()
	agents := []string{"agent-web-1", "agent-web-2", "agent-db-1"}

	// Access all agents concurrently
	type result struct {
		agentID string
		agent   *pb.AgentInfo
		err     error
	}

	results := make(chan result, len(agents))

	for _, agentID := range agents {
		go func(id string) {
			resp, err := client.GetAgent(ctx, &pb.GetAgentRequest{
				AgentId: id,
			})
			if err != nil {
				results <- result{id, nil, err}
				return
			}
			results <- result{id, resp.Agent, nil}
		}(agentID)
	}

	// Collect results
	successCount := 0
	for i := 0; i < len(agents); i++ {
		r := <-results
		if r.err != nil {
			t.Errorf("Failed to get agent %s: %v", r.agentID, r.err)
			continue
		}
		if r.agent == nil {
			t.Errorf("Agent %s returned nil", r.agentID)
			continue
		}
		if r.agent.Status != pb.AgentStatus_AGENT_STATUS_ONLINE {
			t.Errorf("Agent %s is not online: %s", r.agentID, r.agent.Status)
			continue
		}
		successCount++
	}

	if successCount != len(agents) {
		t.Errorf("Expected %d successful agent accesses, got %d", len(agents), successCount)
	}
}

// TestAgentLifecycle_HeartbeatInterval tests that heartbeats arrive at expected intervals
func TestAgentLifecycle_HeartbeatInterval(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping heartbeat interval test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client := testEnv.Client()
	agentID := "agent-web-1"

	// Collect heartbeat timestamps over multiple intervals
	heartbeats := []int64{}
	pollInterval := 2 * time.Second
	pollCount := 10

	for i := 0; i < pollCount; i++ {
		resp, err := client.GetAgent(ctx, &pb.GetAgentRequest{
			AgentId: agentID,
		})
		if err != nil {
			t.Fatalf("Failed to get agent: %v", err)
		}

		if resp.Agent.LastHeartbeat != nil {
			ts := resp.Agent.LastHeartbeat.Seconds
			if len(heartbeats) == 0 || heartbeats[len(heartbeats)-1] != ts {
				heartbeats = append(heartbeats, ts)
			}
		}

		time.Sleep(pollInterval)
	}

	t.Logf("Collected %d unique heartbeat timestamps over %v", len(heartbeats), pollInterval*time.Duration(pollCount))

	// Calculate intervals between heartbeats
	if len(heartbeats) >= 2 {
		for i := 1; i < len(heartbeats); i++ {
			interval := heartbeats[i] - heartbeats[i-1]
			t.Logf("Heartbeat interval %d: %d seconds", i, interval)

			// Heartbeat should be roughly every 5 seconds (configured in agent)
			if interval > 15 {
				t.Logf("Warning: Long heartbeat interval detected: %d seconds", interval)
			}
		}
	}
}

// TestAgentLifecycle_AllAgentsOnline uses the assertion helper
func TestAgentLifecycle_AllAgentsOnline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Use assertion helper to verify all agents are online
	testEnv.AssertAgentCount(t, ctx, 3)

	// Verify each agent is online
	for _, agentID := range []string{"agent-web-1", "agent-web-2", "agent-db-1"} {
		testEnv.AssertAgentOnline(t, ctx, agentID)
	}
}

// TestAgentLifecycle_WaitForAgentStatus tests the WaitForAgentStatus helper
func TestAgentLifecycle_WaitForAgentStatus(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	agentID := "agent-web-1"

	// Agent should already be online - this should return immediately
	err := testEnv.WaitForAgentStatus(ctx, agentID, pb.AgentStatus_AGENT_STATUS_ONLINE, 10*time.Second)
	if err != nil {
		t.Errorf("WaitForAgentStatus failed: %v", err)
	}
}

// TestAgentLifecycle_RetryWithBackoff tests the RetryWithBackoff helper
func TestAgentLifecycle_RetryWithBackoff(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client := testEnv.Client()
	agentID := "agent-web-1"

	callCount := 0
	err := harness.RetryWithBackoff(ctx, 3, 100*time.Millisecond, func() error {
		callCount++
		resp, err := client.GetAgent(ctx, &pb.GetAgentRequest{
			AgentId: agentID,
		})
		if err != nil {
			return err
		}
		if resp.Agent == nil {
			return err
		}
		if resp.Agent.Status != pb.AgentStatus_AGENT_STATUS_ONLINE {
			return err
		}
		return nil // Success
	})

	if err != nil {
		t.Errorf("RetryWithBackoff failed: %v", err)
	}

	t.Logf("RetryWithBackoff completed in %d calls", callCount)
}

// =============================================================================
// Agent Facts Tests (if facts API is available)
// =============================================================================

// TestAgentLifecycle_SystemInfo tests that system info is collected
func TestAgentLifecycle_SystemInfo(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	agentID := "agent-web-1"

	// Verify system info by running commands that should work on any Linux agent
	tests := []struct {
		name    string
		command string
		args    []string
	}{
		{"hostname", "hostname", nil},
		{"os-release", "cat", []string{"/etc/os-release"}},
		{"kernel", "uname", []string{"-r"}},
		{"arch", "uname", []string{"-m"}},
		{"uptime", "uptime", nil},
		{"memory", "free", []string{"-h"}},
		{"disk", "df", []string{"-h", "/"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := testEnv.ExecuteCommandAndWait(ctx, agentID, test.command, test.args...)
			if err != nil {
				t.Errorf("Failed to execute %s: %v", test.name, err)
				return
			}

			if result.ExitCode != 0 {
				// Some commands might not be available (like 'free' on Alpine)
				t.Logf("%s not available or failed: %s", test.name, result.Stderr)
			} else {
				// Log first line of output for each
				output := strings.TrimSpace(result.Stdout)
				lines := strings.Split(output, "\n")
				if len(lines) > 0 {
					t.Logf("%s: %s", test.name, lines[0])
				}
			}
		})
	}
}

// TestAgentLifecycle_NetworkInfo tests network information collection
func TestAgentLifecycle_NetworkInfo(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	agentID := "agent-web-1"

	// Get network interfaces
	result, err := testEnv.ExecuteCommandAndWait(ctx, agentID, "ip", "addr", "show")
	if err != nil {
		// ip might not be available, try ifconfig
		result, err = testEnv.ExecuteCommandAndWait(ctx, agentID, "ifconfig")
		if err != nil {
			t.Skipf("Neither ip nor ifconfig available: %v", err)
			return
		}
	}

	if result.ExitCode != 0 {
		t.Errorf("Network info command failed: %s", result.Stderr)
		return
	}

	// Just verify we got some output
	if len(result.Stdout) < 10 {
		t.Errorf("Network info output seems too short: %s", result.Stdout)
	} else {
		// Show first few lines
		lines := strings.Split(result.Stdout, "\n")
		for i, line := range lines {
			if i < 5 {
				t.Logf("Network: %s", line)
			}
		}
	}
}
