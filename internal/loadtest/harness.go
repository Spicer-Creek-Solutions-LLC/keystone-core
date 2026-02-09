// Package loadtest provides load testing infrastructure for Keystone Core.
package loadtest

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"

	"github.com/shawnbutts/keystone-core/internal/config"
	natsmgr "github.com/shawnbutts/keystone-core/internal/nats"
	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
)

// TestHarness provides the infrastructure for load tests.
type TestHarness struct {
	config     *Config
	natsServer *server.Server
	natsURL    string

	// Control plane simulation
	controlPlane *SimulatedControlPlane

	mu      sync.Mutex
	started bool
}

// NewTestHarness creates a new test harness.
func NewTestHarness(cfg *Config) *TestHarness {
	return &TestHarness{
		config: cfg,
	}
}

// Start starts the test harness infrastructure.
func (h *TestHarness) Start() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.started {
		return fmt.Errorf("harness already started")
	}

	// Start embedded NATS server
	opts := &server.Options{
		Port:      h.config.NATSPort,
		NoLog:     true,
		NoSigs:    true,
		JetStream: true,
		StoreDir:  "",
	}

	ns, err := server.NewServer(opts)
	if err != nil {
		return fmt.Errorf("failed to create NATS server: %w", err)
	}

	go ns.Start()

	if !ns.ReadyForConnections(10 * time.Second) {
		return fmt.Errorf("NATS server failed to start")
	}

	h.natsServer = ns
	h.natsURL = fmt.Sprintf("nats://127.0.0.1:%d", h.config.NATSPort)

	// Start simulated control plane
	h.controlPlane, err = NewSimulatedControlPlane(h.natsURL)
	if err != nil {
		h.natsServer.Shutdown()
		return fmt.Errorf("failed to start control plane: %w", err)
	}

	h.started = true
	return nil
}

// Stop stops the test harness.
func (h *TestHarness) Stop() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.controlPlane != nil {
		h.controlPlane.Stop()
	}

	if h.natsServer != nil {
		h.natsServer.Shutdown()
	}

	h.started = false
}

// NATSURL returns the NATS server URL.
func (h *TestHarness) NATSURL() string {
	return h.natsURL
}

// ControlPlane returns the simulated control plane.
func (h *TestHarness) ControlPlane() *SimulatedControlPlane {
	return h.controlPlane
}

// CreateNATSManager creates a new NATS manager connected to the test server.
func (h *TestHarness) CreateNATSManager() (*natsmgr.Manager, error) {
	cfg := &config.NATSConfig{
		Mode: config.NATSModeExternal,
		URL:  h.natsURL,
	}
	mgr, err := natsmgr.NewManager(cfg)
	if err != nil {
		return nil, err
	}
	if err := mgr.Start(); err != nil {
		return nil, err
	}
	return mgr, nil
}

// CreateAgentPool creates a new agent pool connected to the test server.
func (h *TestHarness) CreateAgentPool() (*AgentPool, error) {
	natsManager, err := h.CreateNATSManager()
	if err != nil {
		return nil, fmt.Errorf("failed to create NATS manager: %w", err)
	}

	return NewAgentPool(h.config, natsManager), nil
}

// SimulatedControlPlane simulates control plane behavior for load testing.
type SimulatedControlPlane struct {
	nats     *natsmgr.Manager
	subjects *natsmgr.SubjectBuilder

	ctx    context.Context
	cancel context.CancelFunc

	// Metrics
	registrations    int64
	heartbeats       int64
	commandsSent     int64
	commandsComplete int64
	commandsFailed   int64

	// Agent tracking
	agents map[string]*RegisteredAgent
	mu     sync.RWMutex

	// Latency tracking
	commandLatencies *LatencyCollector
}

// RegisteredAgent tracks a registered agent.
type RegisteredAgent struct {
	ID            string
	Hostname      string
	RegisteredAt  time.Time
	LastHeartbeat time.Time
}

// NewSimulatedControlPlane creates a new simulated control plane.
func NewSimulatedControlPlane(natsURL string) (*SimulatedControlPlane, error) {
	cfg := &config.NATSConfig{
		Mode: config.NATSModeExternal,
		URL:  natsURL,
	}
	natsManager, err := natsmgr.NewManager(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create NATS manager: %w", err)
	}
	if err := natsManager.Start(); err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	cp := &SimulatedControlPlane{
		nats:             natsManager,
		subjects:         natsmgr.NewSubjectBuilder(natsmgr.DefaultCluster),
		ctx:              ctx,
		cancel:           cancel,
		agents:           make(map[string]*RegisteredAgent),
		commandLatencies: NewLatencyCollector(),
	}

	// Subscribe to registrations
	if err := cp.subscribeToRegistrations(); err != nil {
		cancel()
		return nil, err
	}

	// Subscribe to heartbeats
	if err := cp.subscribeToHeartbeats(); err != nil {
		cancel()
		return nil, err
	}

	return cp, nil
}

// Stop stops the control plane.
func (cp *SimulatedControlPlane) Stop() {
	cp.cancel()
	if cp.nats != nil {
		cp.nats.Shutdown()
	}
}

// subscribeToRegistrations handles agent registrations.
func (cp *SimulatedControlPlane) subscribeToRegistrations() error {
	subject := cp.subjects.AgentRegister()

	_, err := cp.nats.Subscribe(subject, func(msg *nats.Msg) {
		cp.handleRegistration(msg)
	})

	return err
}

// handleRegistration processes an agent registration.
func (cp *SimulatedControlPlane) handleRegistration(msg *nats.Msg) {
	var req pb.RegisterRequest
	if err := proto.Unmarshal(msg.Data, &req); err != nil {
		return
	}

	atomic.AddInt64(&cp.registrations, 1)

	// Track the agent
	cp.mu.Lock()
	cp.agents[req.AgentId] = &RegisteredAgent{
		ID:           req.AgentId,
		Hostname:     req.Metadata.Hostname,
		RegisteredAt: time.Now(),
	}
	cp.mu.Unlock()

	// Send response
	resp := &pb.RegisterResponse{
		AgentId: req.AgentId,
		Config: &pb.AgentConfig{
			HeartbeatInterval: 5,
			CommandTimeout:    30,
		},
	}

	data, err := proto.Marshal(resp)
	if err != nil {
		return
	}

	if msg.Reply != "" {
		cp.nats.Publish(msg.Reply, data)
	}
}

// subscribeToHeartbeats handles agent heartbeats.
func (cp *SimulatedControlPlane) subscribeToHeartbeats() error {
	subject := cp.subjects.AgentHeartbeat()

	_, err := cp.nats.Subscribe(subject, func(msg *nats.Msg) {
		cp.handleHeartbeat(msg)
	})

	return err
}

// handleHeartbeat processes an agent heartbeat.
func (cp *SimulatedControlPlane) handleHeartbeat(msg *nats.Msg) {
	var req pb.HeartbeatRequest
	if err := proto.Unmarshal(msg.Data, &req); err != nil {
		return
	}

	atomic.AddInt64(&cp.heartbeats, 1)

	// Update agent last heartbeat
	cp.mu.Lock()
	if agent, ok := cp.agents[req.AgentId]; ok {
		agent.LastHeartbeat = time.Now()
	}
	cp.mu.Unlock()
}

// SendCommand sends a command to an agent and waits for response.
func (cp *SimulatedControlPlane) SendCommand(ctx context.Context, agentID, command string, args []string, timeout time.Duration) (*pb.ExecuteCommandResponse, error) {
	start := time.Now()

	req := &pb.ExecuteCommandRequest{
		CommandId: uuid.New().String(),
		Command:   command,
		Args:      args,
		Timeout:   int32(timeout.Seconds()),
	}

	data, err := proto.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal command: %w", err)
	}

	subject := cp.subjects.AgentCommand(agentID)
	atomic.AddInt64(&cp.commandsSent, 1)

	msg, err := cp.nats.PublishRequest(subject, data, timeout)
	if err != nil {
		atomic.AddInt64(&cp.commandsFailed, 1)
		return nil, fmt.Errorf("command failed: %w", err)
	}

	var resp pb.ExecuteCommandResponse
	if err := proto.Unmarshal(msg.Data, &resp); err != nil {
		atomic.AddInt64(&cp.commandsFailed, 1)
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	latency := time.Since(start)
	cp.commandLatencies.Add(latency)

	if resp.Type == pb.CommandResponseType_COMMAND_RESPONSE_TYPE_COMPLETED {
		atomic.AddInt64(&cp.commandsComplete, 1)
	} else {
		atomic.AddInt64(&cp.commandsFailed, 1)
	}

	return &resp, nil
}

// BroadcastCommand sends a command to multiple agents concurrently.
func (cp *SimulatedControlPlane) BroadcastCommand(ctx context.Context, agentIDs []string, command string, args []string, timeout time.Duration, concurrency int) (successCount, failCount int, latencies *LatencyCollector) {
	if concurrency <= 0 {
		concurrency = 50
	}

	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	var success, failed int64
	collector := NewLatencyCollector()

	for _, agentID := range agentIDs {
		select {
		case <-ctx.Done():
			goto done
		case sem <- struct{}{}:
		}

		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			defer func() { <-sem }()

			start := time.Now()
			_, err := cp.SendCommand(ctx, id, command, args, timeout)
			latency := time.Since(start)
			collector.Add(latency)

			if err != nil {
				atomic.AddInt64(&failed, 1)
			} else {
				atomic.AddInt64(&success, 1)
			}
		}(agentID)
	}

done:
	wg.Wait()
	return int(success), int(failed), collector
}

// Metrics returns control plane metrics.
func (cp *SimulatedControlPlane) Metrics() ControlPlaneMetrics {
	return ControlPlaneMetrics{
		Registrations:        atomic.LoadInt64(&cp.registrations),
		Heartbeats:           atomic.LoadInt64(&cp.heartbeats),
		CommandsSent:         atomic.LoadInt64(&cp.commandsSent),
		CommandsComplete:     atomic.LoadInt64(&cp.commandsComplete),
		CommandsFailed:       atomic.LoadInt64(&cp.commandsFailed),
		CommandLatencyP50:    cp.commandLatencies.Percentile(50),
		CommandLatencyP95:    cp.commandLatencies.Percentile(95),
		CommandLatencyP99:    cp.commandLatencies.Percentile(99),
		RegisteredAgentCount: cp.RegisteredAgentCount(),
	}
}

// RegisteredAgentCount returns the number of registered agents.
func (cp *SimulatedControlPlane) RegisteredAgentCount() int {
	cp.mu.RLock()
	defer cp.mu.RUnlock()
	return len(cp.agents)
}

// RegisteredAgentIDs returns the IDs of all registered agents.
func (cp *SimulatedControlPlane) RegisteredAgentIDs() []string {
	cp.mu.RLock()
	defer cp.mu.RUnlock()

	ids := make([]string, 0, len(cp.agents))
	for id := range cp.agents {
		ids = append(ids, id)
	}
	return ids
}

// ControlPlaneMetrics holds control plane metrics.
type ControlPlaneMetrics struct {
	Registrations        int64
	Heartbeats           int64
	CommandsSent         int64
	CommandsComplete     int64
	CommandsFailed       int64
	CommandLatencyP50    time.Duration
	CommandLatencyP95    time.Duration
	CommandLatencyP99    time.Duration
	RegisteredAgentCount int
}
