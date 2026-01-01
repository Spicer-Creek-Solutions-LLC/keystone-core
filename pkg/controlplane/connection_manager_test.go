package controlplane

import (
	"fmt"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
	"github.com/shawnbutts/keystone-core/pkg/config"
	natsmgr "github.com/shawnbutts/keystone-core/pkg/nats"
	"google.golang.org/protobuf/proto"
)

func setupTestConnectionManager(t *testing.T) (*ConnectionManager, *natsmgr.Manager, func()) {
	// Create embedded NATS for testing
	cfg := &config.NATSConfig{
		Mode: config.NATSModeEmbedded,
		Embedded: config.NATSEmbeddedConfig{
			Port:            4222, // Use standard NATS port for testing
			EnableJetStream: true,
		},
	}

	natsMgr, err := natsmgr.NewManager(cfg)
	if err != nil {
		t.Fatalf("Failed to create NATS manager: %v", err)
	}

	if err := natsMgr.Start(); err != nil {
		t.Fatalf("Failed to start NATS manager: %v", err)
	}

	// Use short timeouts for testing
	cmCfg := &ConnectionManagerConfig{
		HeartbeatTimeout: 100 * time.Millisecond,
		StaleThreshold:   2,
		MonitorInterval:  50 * time.Millisecond,
	}
	cm := NewConnectionManagerWithConfig(natsMgr, cmCfg)

	cleanup := func() {
		cm.Stop()
		natsMgr.Shutdown()
	}

	return cm, natsMgr, cleanup
}

func TestNewConnectionManager(t *testing.T) {
	cm, _, cleanup := setupTestConnectionManager(t)
	defer cleanup()

	if cm == nil {
		t.Fatal("ConnectionManager should not be nil")
	}

	if cm.agents == nil {
		t.Fatal("Agents map should be initialized")
	}

	if cm.heartbeatTimeout == 0 {
		t.Error("Heartbeat timeout should be set")
	}

	if cm.staleThreshold == 0 {
		t.Error("Stale threshold should be set")
	}
}

func TestConnectionManager_StartStop(t *testing.T) {
	cm, _, cleanup := setupTestConnectionManager(t)
	defer cleanup()

	// Start the connection manager
	if err := cm.Start(); err != nil {
		t.Fatalf("Failed to start connection manager: %v", err)
	}

	// Give it a moment to subscribe
	time.Sleep(100 * time.Millisecond)

	// Stop the connection manager
	if err := cm.Stop(); err != nil {
		t.Errorf("Failed to stop connection manager: %v", err)
	}
}

func TestConnectionManager_AgentRegistration(t *testing.T) {
	cm, natsMgr, cleanup := setupTestConnectionManager(t)
	defer cleanup()

	if err := cm.Start(); err != nil {
		t.Fatalf("Failed to start connection manager: %v", err)
	}

	// Give subscriptions time to be established
	time.Sleep(100 * time.Millisecond)

	// Create registration request
	req := &pb.RegisterRequest{
		AgentId: "test-agent-1",
		Metadata: &pb.AgentMetadata{
			Hostname:    "test-host",
			Os:          "linux",
			Arch:        "amd64",
			IpAddresses: []string{"192.168.1.1"},
		},
	}

	data, err := proto.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}

	// Send registration request (using default cluster)
	respMsg, err := natsMgr.PublishRequest("kscore.default.agent.register", data, 1*time.Second)
	if err != nil {
		t.Fatalf("Failed to send registration request: %v", err)
	}

	// Parse response
	var resp pb.RegisterResponse
	if err := proto.Unmarshal(respMsg.Data, &resp); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if resp.AgentId != "test-agent-1" {
		t.Errorf("Expected agent ID 'test-agent-1', got '%s'", resp.AgentId)
	}

	if resp.Config == nil {
		t.Fatal("Agent config should not be nil")
	}

	// Verify agent was registered
	time.Sleep(100 * time.Millisecond)
	agent, err := cm.GetAgent("test-agent-1")
	if err != nil {
		t.Fatalf("Failed to get agent: %v", err)
	}

	if agent.ID != "test-agent-1" {
		t.Errorf("Expected agent ID 'test-agent-1', got '%s'", agent.ID)
	}

	if agent.Status != pb.AgentStatus_AGENT_STATUS_ONLINE {
		t.Errorf("Expected agent status ONLINE, got %v", agent.Status)
	}

	if agent.Metadata.Hostname != "test-host" {
		t.Errorf("Expected hostname 'test-host', got '%s'", agent.Metadata.Hostname)
	}
}

func TestConnectionManager_Heartbeat(t *testing.T) {
	cm, natsMgr, cleanup := setupTestConnectionManager(t)
	defer cleanup()

	if err := cm.Start(); err != nil {
		t.Fatalf("Failed to start connection manager: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// Register an agent first
	regReq := &pb.RegisterRequest{
		AgentId: "test-agent-2",
		Metadata: &pb.AgentMetadata{
			Hostname: "test-host-2",
		},
	}
	regData, _ := proto.Marshal(regReq)
	natsMgr.PublishRequest("kscore.default.agent.register", regData, 1*time.Second)

	time.Sleep(100 * time.Millisecond)

	// Send heartbeat
	hbReq := &pb.HeartbeatRequest{
		AgentId: "test-agent-2",
		Status:  pb.AgentStatus_AGENT_STATUS_ONLINE,
		Metrics: &pb.SystemMetrics{
			CpuPercent:    25.5,
			MemoryPercent: 60.0,
		},
	}

	hbData, err := proto.Marshal(hbReq)
	if err != nil {
		t.Fatalf("Failed to marshal heartbeat: %v", err)
	}

	// Send heartbeat
	if err := natsMgr.Publish("kscore.default.agent.heartbeat", hbData); err != nil {
		t.Fatalf("Failed to send heartbeat: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// Verify agent received heartbeat
	agent, err := cm.GetAgent("test-agent-2")
	if err != nil {
		t.Fatalf("Failed to get agent: %v", err)
	}

	if agent.LastMetrics == nil {
		t.Fatal("Agent metrics should be set")
	}

	if agent.LastMetrics.CpuPercent != 25.5 {
		t.Errorf("Expected CPU 25.5%%, got %.1f%%", agent.LastMetrics.CpuPercent)
	}

	if agent.HeartbeatMissed != 0 {
		t.Errorf("Expected 0 missed heartbeats, got %d", agent.HeartbeatMissed)
	}
}

func TestConnectionManager_ListAgents(t *testing.T) {
	cm, natsMgr, cleanup := setupTestConnectionManager(t)
	defer cleanup()

	if err := cm.Start(); err != nil {
		t.Fatalf("Failed to start connection manager: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// Register multiple agents
	for i := 1; i <= 3; i++ {
		req := &pb.RegisterRequest{
			AgentId: fmt.Sprintf("agent-%d", i),
			Metadata: &pb.AgentMetadata{
				Hostname: fmt.Sprintf("host-%d", i),
			},
		}
		data, _ := proto.Marshal(req)
		natsMgr.PublishRequest("kscore.default.agent.register", data, 1*time.Second)
	}

	time.Sleep(200 * time.Millisecond)

	// List agents
	agents := cm.ListAgents()

	if len(agents) != 3 {
		t.Errorf("Expected 3 agents, got %d", len(agents))
	}

	// Verify each agent
	agentIDs := make(map[string]bool)
	for _, agent := range agents {
		agentIDs[agent.ID] = true
	}

	for i := 1; i <= 3; i++ {
		expectedID := fmt.Sprintf("agent-%d", i)
		if !agentIDs[expectedID] {
			t.Errorf("Expected to find agent %s in list", expectedID)
		}
	}
}

func TestConnectionManager_GetAgentCount(t *testing.T) {
	cm, natsMgr, cleanup := setupTestConnectionManager(t)
	defer cleanup()

	if err := cm.Start(); err != nil {
		t.Fatalf("Failed to start connection manager: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// Initially should be 0
	if count := cm.GetAgentCount(); count != 0 {
		t.Errorf("Expected 0 agents initially, got %d", count)
	}

	// Register 2 agents
	for i := 1; i <= 2; i++ {
		req := &pb.RegisterRequest{
			AgentId: fmt.Sprintf("agent-%d", i),
			Metadata: &pb.AgentMetadata{
				Hostname: fmt.Sprintf("host-%d", i),
			},
		}
		data, _ := proto.Marshal(req)
		natsMgr.PublishRequest("kscore.default.agent.register", data, 1*time.Second)
	}

	time.Sleep(200 * time.Millisecond)

	if count := cm.GetAgentCount(); count != 2 {
		t.Errorf("Expected 2 agents, got %d", count)
	}
}

func TestConnectionManager_GetOnlineAgentCount(t *testing.T) {
	cm, _, cleanup := setupTestConnectionManager(t)
	defer cleanup()

	// Manually add agents with different statuses
	cm.mu.Lock()
	cm.agents["agent-1"] = &AgentInfo{
		ID:     "agent-1",
		Status: pb.AgentStatus_AGENT_STATUS_ONLINE,
	}
	cm.agents["agent-2"] = &AgentInfo{
		ID:     "agent-2",
		Status: pb.AgentStatus_AGENT_STATUS_ONLINE,
	}
	cm.agents["agent-3"] = &AgentInfo{
		ID:     "agent-3",
		Status: pb.AgentStatus_AGENT_STATUS_OFFLINE,
	}
	cm.mu.Unlock()

	onlineCount := cm.GetOnlineAgentCount()
	if onlineCount != 2 {
		t.Errorf("Expected 2 online agents, got %d", onlineCount)
	}
}

func TestConnectionManager_GetAgent_NotFound(t *testing.T) {
	cm, _, cleanup := setupTestConnectionManager(t)
	defer cleanup()

	_, err := cm.GetAgent("nonexistent-agent")
	if err == nil {
		t.Error("Expected error when getting non-existent agent")
	}
}

func TestConnectionManager_SendCommand(t *testing.T) {
	cm, natsMgr, cleanup := setupTestConnectionManager(t)
	defer cleanup()

	if err := cm.Start(); err != nil {
		t.Fatalf("Failed to start connection manager: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// Register an agent
	regReq := &pb.RegisterRequest{
		AgentId: "test-agent-cmd",
		Metadata: &pb.AgentMetadata{
			Hostname: "test-host-cmd",
		},
	}
	regData, _ := proto.Marshal(regReq)
	natsMgr.PublishRequest("kscore.default.agent.register", regData, 1*time.Second)

	time.Sleep(100 * time.Millisecond)

	// Subscribe to agent commands (simulate agent)
	received := make(chan *pb.ExecuteCommandRequest, 1)
	natsMgr.Subscribe("kscore.default.agent.test-agent-cmd.command", func(msg *nats.Msg) {
		var cmd pb.ExecuteCommandRequest
		if err := proto.Unmarshal(msg.Data, &cmd); err == nil {
			received <- &cmd
		}
	})

	time.Sleep(50 * time.Millisecond)

	// Send a heartbeat to keep agent online (with short test timeouts, it can go offline quickly)
	hbReq := &pb.HeartbeatRequest{
		AgentId: "test-agent-cmd",
		Status:  pb.AgentStatus_AGENT_STATUS_ONLINE,
	}
	hbData, _ := proto.Marshal(hbReq)
	natsMgr.Publish("kscore.default.agent.heartbeat", hbData)

	time.Sleep(50 * time.Millisecond)

	// Send command to agent
	cmdReq := &pb.ExecuteCommandRequest{
		CommandId: "cmd-1",
		Command:   "echo",
		Args:      []string{"hello"},
		Timeout:   300,
	}

	if err := cm.SendCommand("test-agent-cmd", cmdReq); err != nil {
		t.Fatalf("Failed to send command: %v", err)
	}

	// Wait for command to be received
	select {
	case cmd := <-received:
		if cmd.CommandId != "cmd-1" {
			t.Errorf("Expected command ID 'cmd-1', got '%s'", cmd.CommandId)
		}
		if cmd.Command != "echo" {
			t.Errorf("Expected command 'echo', got '%s'", cmd.Command)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for command")
	}
}

func TestConnectionManager_SendCommand_OfflineAgent(t *testing.T) {
	cm, _, cleanup := setupTestConnectionManager(t)
	defer cleanup()

	// Manually add an offline agent
	cm.mu.Lock()
	cm.agents["offline-agent"] = &AgentInfo{
		ID:     "offline-agent",
		Status: pb.AgentStatus_AGENT_STATUS_OFFLINE,
	}
	cm.mu.Unlock()

	cmdReq := &pb.ExecuteCommandRequest{
		CommandId: "cmd-1",
		Command:   "echo",
	}

	err := cm.SendCommand("offline-agent", cmdReq)
	if err == nil {
		t.Error("Expected error when sending command to offline agent")
	}
}

func TestConnectionManager_SendCommand_UnknownAgent(t *testing.T) {
	cm, _, cleanup := setupTestConnectionManager(t)
	defer cleanup()

	cmdReq := &pb.ExecuteCommandRequest{
		CommandId: "cmd-1",
		Command:   "echo",
	}

	err := cm.SendCommand("unknown-agent", cmdReq)
	if err == nil {
		t.Error("Expected error when sending command to unknown agent")
	}
}

func TestConnectionManager_HealthMonitoring(t *testing.T) {
	cm, _, cleanup := setupTestConnectionManager(t)
	defer cleanup()

	// Set shorter timeout for testing
	cm.heartbeatTimeout = 500 * time.Millisecond
	cm.staleThreshold = 2

	// Manually add an agent with old heartbeat
	cm.mu.Lock()
	cm.agents["stale-agent"] = &AgentInfo{
		ID:            "stale-agent",
		Status:        pb.AgentStatus_AGENT_STATUS_ONLINE,
		LastHeartbeat: time.Now().Add(-1 * time.Second), // Old heartbeat
	}
	cm.mu.Unlock()

	// Run health check manually
	cm.checkAgentHealth()

	// Agent should be marked as DEGRADED after first check
	agent, _ := cm.GetAgent("stale-agent")
	if agent.Status != pb.AgentStatus_AGENT_STATUS_DEGRADED {
		t.Errorf("Expected agent to be DEGRADED, got %v", agent.Status)
	}

	if agent.HeartbeatMissed != 1 {
		t.Errorf("Expected 1 missed heartbeat, got %d", agent.HeartbeatMissed)
	}

	// Run health check again
	cm.checkAgentHealth()

	// Agent should now be OFFLINE (threshold reached)
	agent, _ = cm.GetAgent("stale-agent")
	if agent.Status != pb.AgentStatus_AGENT_STATUS_OFFLINE {
		t.Errorf("Expected agent to be OFFLINE, got %v", agent.Status)
	}

	if agent.HeartbeatMissed != 2 {
		t.Errorf("Expected 2 missed heartbeats, got %d", agent.HeartbeatMissed)
	}
}

func TestConnectionManager_HeartbeatResetsStatus(t *testing.T) {
	cm, natsMgr, cleanup := setupTestConnectionManager(t)
	defer cleanup()

	if err := cm.Start(); err != nil {
		t.Fatalf("Failed to start connection manager: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// Register an agent
	regReq := &pb.RegisterRequest{
		AgentId: "recovery-agent",
		Metadata: &pb.AgentMetadata{
			Hostname: "recovery-host",
		},
	}
	regData, _ := proto.Marshal(regReq)
	natsMgr.PublishRequest("kscore.default.agent.register", regData, 1*time.Second)

	time.Sleep(100 * time.Millisecond)

	// Manually mark agent as degraded
	cm.mu.Lock()
	cm.agents["recovery-agent"].Status = pb.AgentStatus_AGENT_STATUS_DEGRADED
	cm.agents["recovery-agent"].HeartbeatMissed = 1
	cm.mu.Unlock()

	// Send heartbeat
	hbReq := &pb.HeartbeatRequest{
		AgentId: "recovery-agent",
		Status:  pb.AgentStatus_AGENT_STATUS_ONLINE,
	}
	hbData, _ := proto.Marshal(hbReq)
	natsMgr.Publish("kscore.default.agent.heartbeat", hbData)

	time.Sleep(100 * time.Millisecond)

	// Agent should be back to ONLINE with reset missed count
	agent, _ := cm.GetAgent("recovery-agent")
	if agent.Status != pb.AgentStatus_AGENT_STATUS_ONLINE {
		t.Errorf("Expected agent to be ONLINE after heartbeat, got %v", agent.Status)
	}

	if agent.HeartbeatMissed != 0 {
		t.Errorf("Expected 0 missed heartbeats after recovery, got %d", agent.HeartbeatMissed)
	}
}
