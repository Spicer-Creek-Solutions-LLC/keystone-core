// Package controlplane implements the Keystone Core control plane server.
// It provides:
//   - Agent connection management and lifecycle
//   - Command dispatch and job tracking
//   - Batch execution across multiple agents
//   - State synchronization
//   - Event emission for agent lifecycle events
//
// The control plane coordinates all managed agents, handles command routing,
// and maintains the authoritative state of the infrastructure.
package controlplane

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/shawnbutts/keystone-core/internal/events"
	natsmgr "github.com/shawnbutts/keystone-core/internal/nats"
	"github.com/shawnbutts/keystone-core/internal/tracing"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
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

// AgentStore is an interface for persisting agent state
type AgentStore interface {
	// ListAgents returns all agents from the database
	ListAgents(ctx context.Context, filter *AgentFilter) ([]StoredAgent, error)
	// GetAgent returns an agent by ID
	GetAgent(ctx context.Context, agentID string) (*StoredAgent, error)
	// SaveAgent saves an agent to the database (for HA cluster support)
	SaveAgent(ctx context.Context, agent *StoredAgent) error
}

// AgentFilter is used to filter agents when listing
type AgentFilter struct {
	// Status filters by agent status (empty means all)
	Status string
}

// StoredAgent represents an agent stored in the database
type StoredAgent struct {
	ID           string
	Hostname     string
	OS           string
	Arch         string
	Status       string
	Labels       map[string]string
	IPAddresses  []string
	RegisteredAt time.Time
	LastSeen     time.Time
}

// ConnectionManagerConfig holds configuration for the connection manager
type ConnectionManagerConfig struct {
	// Cluster is the logical cluster name for subject namespacing
	// If empty, defaults to "default"
	Cluster string
	// ServerID is a unique identifier for this control plane server
	// If empty, a UUID will be generated
	ServerID string
	// HeartbeatTimeout is how long to wait before considering an agent stale
	HeartbeatTimeout time.Duration
	// StaleThreshold is how many missed heartbeats before marking agent offline
	StaleThreshold int
	// MonitorInterval is how often to check agent health
	MonitorInterval time.Duration
	// AgentHeartbeatInterval is sent to agents during registration as the
	// recommended heartbeat interval (seconds).
	AgentHeartbeatInterval time.Duration
	// AgentCommandTimeout is sent to agents during registration as the
	// default command execution timeout (seconds).
	AgentCommandTimeout time.Duration
	// AgentMetadataInterval is sent to agents during registration as the
	// recommended metadata refresh interval (seconds).
	AgentMetadataInterval time.Duration
	// MaxConcurrentCommands is the maximum number of concurrent commands per agent.
	// 0 means unlimited.
	MaxConcurrentCommands int
}

// DefaultConnectionManagerConfig returns the default configuration
func DefaultConnectionManagerConfig() *ConnectionManagerConfig {
	return &ConnectionManagerConfig{
		HeartbeatTimeout:       60 * time.Second,
		StaleThreshold:         3,
		MonitorInterval:        10 * time.Second,
		AgentHeartbeatInterval: 30 * time.Second,
		AgentCommandTimeout:    5 * time.Minute,
		AgentMetadataInterval:  5 * time.Minute,
	}
}

// ConnectionManager manages agent connections and state
type ConnectionManager struct {
	nats   *natsmgr.Manager
	agents map[string]*AgentInfo
	mu     sync.RWMutex

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// NATS subject management
	subjects *natsmgr.SubjectBuilder
	cluster  string
	serverID string

	// Configuration
	heartbeatTimeout time.Duration
	staleThreshold   int
	monitorInterval  time.Duration

	// Agent registration config (sent to agents)
	agentHeartbeatInterval time.Duration
	agentCommandTimeout    time.Duration
	agentMetadataInterval  time.Duration
	maxConcurrentCommands  int

	// Event publishing
	eventPublisher events.EventPublisher

	// State store for agent persistence (optional, for HA clusters)
	stateStore AgentStore
}

// NewConnectionManager creates a new connection manager with default configuration
func NewConnectionManager(natsManager *natsmgr.Manager) *ConnectionManager {
	return NewConnectionManagerWithConfig(natsManager, nil)
}

// NewConnectionManagerWithConfig creates a new connection manager with custom configuration
func NewConnectionManagerWithConfig(natsManager *natsmgr.Manager, cfg *ConnectionManagerConfig) *ConnectionManager {
	if cfg == nil {
		cfg = DefaultConnectionManagerConfig()
	}

	// Set defaults for cluster and server ID
	cluster := cfg.Cluster
	if cluster == "" {
		cluster = natsmgr.DefaultCluster
	}

	serverID := cfg.ServerID
	if serverID == "" {
		serverID = uuid.New().String()
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &ConnectionManager{
		nats:                   natsManager,
		agents:                 make(map[string]*AgentInfo),
		ctx:                    ctx,
		cancel:                 cancel,
		subjects:               natsmgr.NewSubjectBuilder(cluster),
		cluster:                cluster,
		serverID:               serverID,
		heartbeatTimeout:       cfg.HeartbeatTimeout,
		staleThreshold:         cfg.StaleThreshold,
		monitorInterval:        cfg.MonitorInterval,
		agentHeartbeatInterval: cfg.AgentHeartbeatInterval,
		agentCommandTimeout:    cfg.AgentCommandTimeout,
		agentMetadataInterval:  cfg.AgentMetadataInterval,
		maxConcurrentCommands:  cfg.MaxConcurrentCommands,
	}
}

// SetStateStore sets the state store for loading agents from the database
// This is optional but recommended for HA clusters
func (cm *ConnectionManager) SetStateStore(store AgentStore) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.stateStore = store
}

// Cluster returns the cluster name
func (cm *ConnectionManager) Cluster() string {
	return cm.cluster
}

// ServerID returns the server ID
func (cm *ConnectionManager) ServerID() string {
	return cm.serverID
}

// Start starts the connection manager
func (cm *ConnectionManager) Start() error {
	fmt.Printf("Starting connection manager (cluster=%s, server=%s)...\n", cm.cluster, cm.serverID)

	// Load agents from database if state store is configured (for HA clusters)
	if cm.stateStore != nil {
		if err := cm.loadAgentsFromStore(); err != nil {
			fmt.Printf("Warning: failed to load agents from database: %v\n", err)
			// Continue anyway - agents will register as they come online
		}
	}

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

// loadAgentsFromStore loads agents from the database into memory
func (cm *ConnectionManager) loadAgentsFromStore() error {
	ctx, cancel := context.WithTimeout(cm.ctx, 30*time.Second)
	defer cancel()

	agents, err := cm.stateStore.ListAgents(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to list agents: %w", err)
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	loaded := 0
	for i := range agents {
		sa := &agents[i]
		// Skip if already in memory (shouldn't happen on startup, but be safe)
		if _, exists := cm.agents[sa.ID]; exists {
			continue
		}

		// Convert stored agent to AgentInfo
		info := &AgentInfo{
			ID:            sa.ID,
			RegisteredAt:  sa.RegisteredAt,
			LastHeartbeat: sa.LastSeen,
			Status:        pb.AgentStatus_AGENT_STATUS_UNSPECIFIED, // Will be updated on first heartbeat
			Metadata: &pb.AgentMetadata{
				Hostname:    sa.Hostname,
				Os:          sa.OS,
				Arch:        sa.Arch,
				Labels:      sa.Labels,
				IpAddresses: sa.IPAddresses,
			},
		}

		cm.agents[sa.ID] = info
		loaded++
	}

	if loaded > 0 {
		fmt.Printf("Loaded %d agents from database\n", loaded)
	}

	return nil
}

// tryLoadAgentFromStore attempts to load a single agent from the database
func (cm *ConnectionManager) tryLoadAgentFromStore(agentID string) error {
	if cm.stateStore == nil {
		return fmt.Errorf("no state store configured")
	}

	ctx, cancel := context.WithTimeout(cm.ctx, 5*time.Second)
	defer cancel()

	sa, err := cm.stateStore.GetAgent(ctx, agentID)
	if err != nil {
		return err
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Double-check it wasn't added while we were looking it up
	if _, exists := cm.agents[agentID]; exists {
		return nil
	}

	// Convert stored agent to AgentInfo
	info := &AgentInfo{
		ID:            sa.ID,
		RegisteredAt:  sa.RegisteredAt,
		LastHeartbeat: sa.LastSeen,
		Status:        pb.AgentStatus_AGENT_STATUS_ONLINE, // They're sending heartbeats, so they're online
		Metadata: &pb.AgentMetadata{
			Hostname:    sa.Hostname,
			Os:          sa.OS,
			Arch:        sa.Arch,
			Labels:      sa.Labels,
			IpAddresses: sa.IPAddresses,
		},
	}

	cm.agents[agentID] = info
	fmt.Printf("Loaded agent %s from database (responding to heartbeat)\n", agentID)
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
	subject := cm.subjects.AgentRegister()

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
	subject := cm.subjects.AgentHeartbeat()

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
	// Extract trace context from NATS message
	ctx := tracing.ExtractTraceContext(cm.ctx, msg)
	ctx, span := tracing.StartControlPlaneSpan(ctx, tracing.SpanAgentConnect)
	defer span.End()

	// Parse registration request
	var req pb.RegisterRequest
	if err := proto.Unmarshal(msg.Data, &req); err != nil {
		fmt.Printf("Failed to unmarshal registration request: %v\n", err)
		tracing.RecordError(span, err)
		return
	}

	// Add span attributes
	tracing.SetAttributes(span,
		tracing.StringAttr(tracing.AttrAgentID, req.AgentId),
		tracing.StringAttr(tracing.AttrAgentHostname, req.Metadata.GetHostname()),
		tracing.StringAttr(tracing.AttrAgentOS, req.Metadata.GetOs()),
	)

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

	// Save agent to database for HA cluster support
	if cm.stateStore != nil {
		storedAgent := &StoredAgent{
			ID:           req.AgentId,
			Hostname:     req.Metadata.GetHostname(),
			OS:           req.Metadata.GetOs(),
			Arch:         req.Metadata.GetArch(),
			Status:       "online",
			Labels:       req.Metadata.GetLabels(),
			IPAddresses:  req.Metadata.GetIpAddresses(),
			RegisteredAt: info.RegisteredAt,
			LastSeen:     info.LastHeartbeat,
		}
		if err := cm.stateStore.SaveAgent(ctx, storedAgent); err != nil {
			fmt.Printf("Warning: failed to save agent %s to database: %v\n", req.AgentId, err)
		}
	}

	// Emit agent.connect event
	if !exists {
		cm.emitEvent(events.EventTypeAgentConnect, events.SeverityInfo, map[string]interface{}{
			"agent_id":         req.AgentId,
			"hostname":         req.Metadata.Hostname,
			"os":               req.Metadata.Os,
			"arch":             req.Metadata.Arch,
			"ip_addresses":     req.Metadata.IpAddresses,
			"platform_version": req.Metadata.PlatformVersion,
		})
	}

	// Build agent config from connection manager settings
	hbInterval := int32(cm.agentHeartbeatInterval.Seconds())
	if hbInterval <= 0 {
		hbInterval = 30 // fallback default
	}
	cmdTimeout := int32(cm.agentCommandTimeout.Seconds())
	if cmdTimeout <= 0 {
		cmdTimeout = 300 // fallback default
	}
	metaInterval := int32(cm.agentMetadataInterval.Seconds())
	if metaInterval <= 0 {
		metaInterval = 300 // fallback default
	}

	// Send registration response
	resp := &pb.RegisterResponse{
		AgentId:      req.AgentId,
		RegisteredAt: timestamppb.New(info.RegisteredAt),
		Config: &pb.AgentConfig{
			HeartbeatInterval: hbInterval,
			CommandTimeout:    cmdTimeout,
			MetadataInterval:  metaInterval,
		},
	}

	data, err := proto.Marshal(resp)
	if err != nil {
		fmt.Printf("Failed to marshal registration response: %v\n", err)
		return
	}

	// Reply to registration request
	if msg.Reply != "" {
		// Create reply message with trace context
		replyMsg := &nats.Msg{
			Subject: msg.Reply,
			Data:    data,
			Header:  nats.Header{},
		}
		tracing.InjectTraceContext(ctx, replyMsg)

		if err := cm.nats.Conn().PublishMsg(replyMsg); err != nil {
			fmt.Printf("Failed to send registration response: %v\n", err)
			tracing.RecordError(span, err)
		} else {
			tracing.RecordSuccess(span, "agent registered successfully")
		}
	}
}

// handleHeartbeat processes agent heartbeat messages
func (cm *ConnectionManager) handleHeartbeat(msg *nats.Msg) {
	// Extract trace context from NATS message
	ctx := tracing.ExtractTraceContext(cm.ctx, msg)
	_, span := tracing.StartControlPlaneSpan(ctx, tracing.SpanAgentHeartbeat)
	defer span.End()

	// Parse heartbeat request
	var req pb.HeartbeatRequest
	if err := proto.Unmarshal(msg.Data, &req); err != nil {
		fmt.Printf("Failed to unmarshal heartbeat: %v\n", err)
		tracing.RecordError(span, err)
		return
	}

	// Add span attributes
	tracing.SetAttributes(span,
		tracing.StringAttr(tracing.AttrAgentID, req.AgentId),
		tracing.StringAttr("agent.status", req.Status.String()),
	)

	cm.mu.Lock()
	info, exists := cm.agents[req.AgentId]
	if !exists {
		// Try to find agent in database (for HA clusters where agent registered with another server)
		if cm.stateStore != nil {
			cm.mu.Unlock()
			if err := cm.tryLoadAgentFromStore(req.AgentId); err == nil {
				// Agent loaded from database, try again
				cm.mu.Lock()
				info, exists = cm.agents[req.AgentId]
				if !exists {
					cm.mu.Unlock()
					fmt.Printf("Heartbeat from unknown agent (not in DB): %s\n", req.AgentId)
					tracing.AddEvent(span, "unknown_agent")
					return
				}
				// Fall through to update heartbeat
			} else {
				fmt.Printf("Heartbeat from unknown agent: %s\n", req.AgentId)
				tracing.AddEvent(span, "unknown_agent")
				return
			}
		} else {
			cm.mu.Unlock()
			fmt.Printf("Heartbeat from unknown agent: %s\n", req.AgentId)
			tracing.AddEvent(span, "unknown_agent")
			return
		}
	}

	// Update agent info
	info.LastHeartbeat = time.Now()
	info.Status = req.Status
	info.LastMetrics = req.Metrics
	info.HeartbeatMissed = 0
	cm.mu.Unlock()

	tracing.RecordSuccess(span, "heartbeat processed")
}

// monitorAgents periodically checks agent health
func (cm *ConnectionManager) monitorAgents() {
	defer cm.wg.Done()

	interval := cm.monitorInterval
	if interval == 0 {
		interval = 10 * time.Second // fallback default
	}

	ticker := time.NewTicker(interval)
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
					previousStatus := info.Status
					info.Status = pb.AgentStatus_AGENT_STATUS_OFFLINE

					// Emit agent.disconnect event
					cm.emitEvent(events.EventTypeAgentDisconnect, events.SeverityWarning, map[string]interface{}{
						"agent_id":             id,
						"hostname":             info.Metadata.GetHostname(),
						"previous_status":      previousStatus.String(),
						"heartbeat_missed":     info.HeartbeatMissed,
						"time_since_heartbeat": timeSinceHeartbeat.Seconds(),
					})
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
	info, exists := cm.agents[agentID]
	if exists {
		// Return a copy while holding the lock
		infoCopy := *info
		cm.mu.RUnlock()
		return &infoCopy, nil
	}
	cm.mu.RUnlock()

	// Try to find agent in database (for HA clusters where agent registered with another server)
	if cm.stateStore != nil {
		if err := cm.tryLoadAgentFromStore(agentID); err == nil {
			// Agent loaded from database, try again
			cm.mu.RLock()
			info, exists = cm.agents[agentID]
			if exists {
				// Return a copy while holding the lock
				infoCopy := *info
				cm.mu.RUnlock()
				return &infoCopy, nil
			}
			cm.mu.RUnlock()
			return nil, fmt.Errorf("agent %s not found", agentID)
		}
		return nil, fmt.Errorf("agent %s not found", agentID)
	}
	return nil, fmt.Errorf("agent %s not found", agentID)
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
	return cm.SendCommandWithContext(cm.ctx, agentID, command)
}

// SendCommandWithContext sends a command to a specific agent with context
func (cm *ConnectionManager) SendCommandWithContext(ctx context.Context, agentID string, command *pb.ExecuteCommandRequest) error {
	ctx, span := tracing.StartControlPlaneSpan(ctx, tracing.SpanCommandDispatch,
		tracing.StringAttr(tracing.AttrAgentID, agentID),
		tracing.StringAttr(tracing.AttrJobID, command.CommandId),
		tracing.StringAttr(tracing.AttrJobCommand, command.Command),
	)
	defer span.End()

	// Check if agent exists and is online
	info, err := cm.GetAgent(agentID) //nolint:contextcheck // GetAgent API doesn't take context
	if err != nil {
		tracing.RecordError(span, err)
		return err
	}

	if info.Status == pb.AgentStatus_AGENT_STATUS_OFFLINE {
		err := fmt.Errorf("agent %s is offline", agentID)
		tracing.RecordError(span, err)
		return err
	}

	tracing.SetAttributes(span,
		tracing.StringAttr(tracing.AttrAgentHostname, info.Metadata.GetHostname()),
		tracing.StringAttr("agent.status", info.Status.String()),
	)

	// Serialize command
	data, err := proto.Marshal(command)
	if err != nil {
		err = fmt.Errorf("failed to marshal command: %w", err)
		tracing.RecordError(span, err)
		return err
	}

	// Send command to agent-specific subject with trace context
	// Set reply subject so agent can send responses back
	subject := cm.subjects.AgentCommand(agentID)
	replySubject := cm.subjects.CommandResponse(command.CommandId)
	msg := &nats.Msg{
		Subject: subject,
		Reply:   replySubject,
		Data:    data,
		Header:  nats.Header{},
	}
	tracing.InjectTraceContext(ctx, msg)

	if err := cm.nats.Conn().PublishMsg(msg); err != nil {
		err = fmt.Errorf("failed to send command: %w", err)
		tracing.RecordError(span, err)
		return err
	}

	fmt.Printf("Command %s sent to agent %s\n", command.CommandId, agentID)
	tracing.RecordSuccess(span, "command dispatched to agent")
	return nil
}

// SetEventPublisher sets the event publisher for emitting events
func (cm *ConnectionManager) SetEventPublisher(publisher events.EventPublisher) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.eventPublisher = publisher
}

// emitEvent emits an event if EventPublisher is configured
func (cm *ConnectionManager) emitEvent(eventType events.EventType, severity events.Severity, data map[string]interface{}) {
	if cm.eventPublisher == nil {
		return
	}

	// Extract correlation ID from data if present (use agent_id as correlation for agent events)
	correlationID := ""
	if aid, ok := data["agent_id"].(string); ok {
		correlationID = "agent-" + aid
	}

	eventBuilder := events.NewEvent(eventType).
		Source("/control-plane").
		Severity(severity).
		DataMap(data)

	// Set correlation ID if available
	if correlationID != "" {
		eventBuilder = eventBuilder.CorrelationID(correlationID)
	}

	event := eventBuilder.Build()

	// Use async publish to avoid blocking
	if err := cm.eventPublisher.PublishAsync(event); err != nil {
		fmt.Printf("Warning: failed to emit event %s: %v\n", eventType, err)
	}
}
