package controlplane

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	natsmgr "github.com/titananvil/titan-anvil/pkg/nats"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/titananvil/titan-anvil/pkg/api/v1"
)

// AgentInfo represents information about a connected agent
type AgentInfo struct {
	ID              string
	Metadata        *pb.AgentMetadata
	Status          pb.AgentStatus
	LastHeartbeat   time.Time
	RegisteredAt    time.Time
	LastMetrics     *pb.SystemMetrics
	HeartbeatMissed int
}

// ConnectionManager manages agent connections and state
type ConnectionManager struct {
	nats   *natsmgr.Manager
	agents map[string]*AgentInfo
	mu     sync.RWMutex

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Configuration
	heartbeatTimeout time.Duration
	staleThreshold   int
}

// NewConnectionManager creates a new connection manager
func NewConnectionManager(natsManager *natsmgr.Manager) *ConnectionManager {
	ctx, cancel := context.WithCancel(context.Background())

	return &ConnectionManager{
		nats:             natsManager,
		agents:           make(map[string]*AgentInfo),
		ctx:              ctx,
		cancel:           cancel,
		heartbeatTimeout: 60 * time.Second, // Consider agent stale after 60s
		staleThreshold:   3,                // Mark offline after 3 missed heartbeats
	}
}

// Start starts the connection manager
func (cm *ConnectionManager) Start() error {
	fmt.Println("Starting connection manager...")

	// Subscribe to agent registration requests
	if err := cm.subscribeToRegistrations(); err != nil {
		return fmt.Errorf("failed to subscribe to registrations: %w", err)
	}

	// Subscribe to agent heartbeats
	if err := cm.subscribeToHeartbeats(); err != nil {
		return fmt.Errorf("failed to subscribe to heartbeats: %w", err)
	}

	// Start agent monitoring loop
	cm.wg.Add(1)
	go cm.monitorAgents()

	fmt.Println("Connection manager started")
	return nil
}

// Stop stops the connection manager
func (cm *ConnectionManager) Stop() error {
	fmt.Println("Stopping connection manager...")
	cm.cancel()
	cm.wg.Wait()
	fmt.Println("Connection manager stopped")
	return nil
}

// subscribeToRegistrations subscribes to agent registration requests
func (cm *ConnectionManager) subscribeToRegistrations() error {
	subject := "titan.agent.register"

	_, err := cm.nats.Subscribe(subject, func(msg *nats.Msg) {
		cm.handleRegistration(msg)
	})

	if err != nil {
		return fmt.Errorf("failed to subscribe to %s: %w", subject, err)
	}

	fmt.Printf("Subscribed to agent registrations on %s\n", subject)
	return nil
}

// subscribeToHeartbeats subscribes to agent heartbeats
func (cm *ConnectionManager) subscribeToHeartbeats() error {
	subject := "titan.agent.heartbeat"

	_, err := cm.nats.Subscribe(subject, func(msg *nats.Msg) {
		cm.handleHeartbeat(msg)
	})

	if err != nil {
		return fmt.Errorf("failed to subscribe to %s: %w", subject, err)
	}

	fmt.Printf("Subscribed to agent heartbeats on %s\n", subject)
	return nil
}

// handleRegistration processes agent registration requests
func (cm *ConnectionManager) handleRegistration(msg *nats.Msg) {
	// Parse registration request
	var req pb.RegisterRequest
	if err := proto.Unmarshal(msg.Data, &req); err != nil {
		fmt.Printf("Failed to unmarshal registration request: %v\n", err)
		return
	}

	fmt.Printf("Agent registration request: ID=%s, Hostname=%s\n", req.AgentId, req.Metadata.Hostname)

	// Register the agent
	cm.mu.Lock()
	info, exists := cm.agents[req.AgentId]
	if !exists {
		info = &AgentInfo{
			ID:           req.AgentId,
			RegisteredAt: time.Now(),
		}
		cm.agents[req.AgentId] = info
		fmt.Printf("New agent registered: %s\n", req.AgentId)
	} else {
		fmt.Printf("Agent re-registered: %s\n", req.AgentId)
	}

	info.Metadata = req.Metadata
	info.Status = pb.AgentStatus_AGENT_STATUS_ONLINE
	info.LastHeartbeat = time.Now()
	info.HeartbeatMissed = 0
	cm.mu.Unlock()

	// Send registration response
	resp := &pb.RegisterResponse{
		AgentId:      req.AgentId,
		RegisteredAt: timestamppb.New(info.RegisteredAt),
		Config: &pb.AgentConfig{
			HeartbeatInterval: 30, // 30 seconds
			CommandTimeout:    300, // 5 minutes
			MetadataInterval:  300, // 5 minutes
		},
	}

	data, err := proto.Marshal(resp)
	if err != nil {
		fmt.Printf("Failed to marshal registration response: %v\n", err)
		return
	}

	// Reply to registration request
	if msg.Reply != "" {
		if err := cm.nats.Publish(msg.Reply, data); err != nil {
			fmt.Printf("Failed to send registration response: %v\n", err)
		}
	}
}

// handleHeartbeat processes agent heartbeat messages
func (cm *ConnectionManager) handleHeartbeat(msg *nats.Msg) {
	// Parse heartbeat request
	var req pb.HeartbeatRequest
	if err := proto.Unmarshal(msg.Data, &req); err != nil {
		fmt.Printf("Failed to unmarshal heartbeat: %v\n", err)
		return
	}

	cm.mu.Lock()
	info, exists := cm.agents[req.AgentId]
	if !exists {
		cm.mu.Unlock()
		fmt.Printf("Heartbeat from unknown agent: %s\n", req.AgentId)
		return
	}

	// Update agent info
	info.LastHeartbeat = time.Now()
	info.Status = req.Status
	info.LastMetrics = req.Metrics
	info.HeartbeatMissed = 0
	cm.mu.Unlock()
}

// monitorAgents periodically checks agent health
func (cm *ConnectionManager) monitorAgents() {
	defer cm.wg.Done()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			cm.checkAgentHealth()
		case <-cm.ctx.Done():
			return
		}
	}
}

// checkAgentHealth checks if agents are still alive
func (cm *ConnectionManager) checkAgentHealth() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	now := time.Now()
	for id, info := range cm.agents {
		timeSinceHeartbeat := now.Sub(info.LastHeartbeat)

		if timeSinceHeartbeat > cm.heartbeatTimeout {
			info.HeartbeatMissed++

			if info.HeartbeatMissed >= cm.staleThreshold {
				if info.Status != pb.AgentStatus_AGENT_STATUS_OFFLINE {
					fmt.Printf("Agent %s marked as OFFLINE (no heartbeat for %s)\n", id, timeSinceHeartbeat)
					info.Status = pb.AgentStatus_AGENT_STATUS_OFFLINE
				}
			} else if info.Status != pb.AgentStatus_AGENT_STATUS_DEGRADED {
				fmt.Printf("Agent %s marked as DEGRADED (heartbeat delayed)\n", id)
				info.Status = pb.AgentStatus_AGENT_STATUS_DEGRADED
			}
		}
	}
}

// GetAgent returns information about a specific agent
func (cm *ConnectionManager) GetAgent(agentID string) (*AgentInfo, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	info, exists := cm.agents[agentID]
	if !exists {
		return nil, fmt.Errorf("agent %s not found", agentID)
	}

	// Return a copy
	infoCopy := *info
	return &infoCopy, nil
}

// ListAgents returns all registered agents
func (cm *ConnectionManager) ListAgents() []*AgentInfo {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	agents := make([]*AgentInfo, 0, len(cm.agents))
	for _, info := range cm.agents {
		infoCopy := *info
		agents = append(agents, &infoCopy)
	}

	return agents
}

// GetAgentCount returns the number of registered agents
func (cm *ConnectionManager) GetAgentCount() int {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return len(cm.agents)
}

// GetOnlineAgentCount returns the number of online agents
func (cm *ConnectionManager) GetOnlineAgentCount() int {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	count := 0
	for _, info := range cm.agents {
		if info.Status == pb.AgentStatus_AGENT_STATUS_ONLINE {
			count++
		}
	}
	return count
}

// SendCommand sends a command to a specific agent
func (cm *ConnectionManager) SendCommand(agentID string, command *pb.ExecuteCommandRequest) error {
	// Check if agent exists and is online
	info, err := cm.GetAgent(agentID)
	if err != nil {
		return err
	}

	if info.Status == pb.AgentStatus_AGENT_STATUS_OFFLINE {
		return fmt.Errorf("agent %s is offline", agentID)
	}

	// Serialize command
	data, err := proto.Marshal(command)
	if err != nil {
		return fmt.Errorf("failed to marshal command: %w", err)
	}

	// Send command to agent-specific subject
	subject := fmt.Sprintf("titan.agent.%s.command", agentID)
	if err := cm.nats.Publish(subject, data); err != nil {
		return fmt.Errorf("failed to send command: %w", err)
	}

	fmt.Printf("Command %s sent to agent %s\n", command.CommandId, agentID)
	return nil
}
