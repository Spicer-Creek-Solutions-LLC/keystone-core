// Package loadtest provides load testing infrastructure for Keystone Core.
package loadtest

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
	natsmgr "github.com/shawnbutts/keystone-core/pkg/nats"
)

// SimulatedAgent represents a simulated agent for load testing.
type SimulatedAgent struct {
	ID       string
	Hostname string

	nats     *natsmgr.Manager
	subjects *natsmgr.SubjectBuilder

	heartbeatInterval time.Duration
	commandTimeout    time.Duration

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Metrics
	registrationTime time.Duration
	heartbeatsSent   int64
	commandsReceived int64
	commandsExecuted int64
	commandLatencies []time.Duration
	errors           int64

	mu sync.Mutex
}

// SimulatedAgentConfig holds configuration for a simulated agent.
type SimulatedAgentConfig struct {
	ID                string
	Hostname          string
	HeartbeatInterval time.Duration
	CommandTimeout    time.Duration
	Cluster           string
}

// NewSimulatedAgent creates a new simulated agent.
func NewSimulatedAgent(natsManager *natsmgr.Manager, cfg *SimulatedAgentConfig) *SimulatedAgent {
	if cfg.ID == "" {
		cfg.ID = uuid.New().String()
	}
	if cfg.Hostname == "" {
		cfg.Hostname = fmt.Sprintf("loadtest-host-%s", cfg.ID[:8])
	}
	if cfg.Cluster == "" {
		cfg.Cluster = natsmgr.DefaultCluster
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &SimulatedAgent{
		ID:                cfg.ID,
		Hostname:          cfg.Hostname,
		nats:              natsManager,
		subjects:          natsmgr.NewSubjectBuilder(cfg.Cluster),
		heartbeatInterval: cfg.HeartbeatInterval,
		commandTimeout:    cfg.CommandTimeout,
		ctx:               ctx,
		cancel:            cancel,
		commandLatencies:  make([]time.Duration, 0, 100),
	}
}

// Start registers the agent and begins heartbeat loop.
func (a *SimulatedAgent) Start() error {
	// Register with control plane
	start := time.Now()
	if err := a.register(); err != nil {
		return fmt.Errorf("failed to register: %w", err)
	}
	a.registrationTime = time.Since(start)

	// Subscribe to commands
	if err := a.subscribeToCommands(); err != nil {
		return fmt.Errorf("failed to subscribe to commands: %w", err)
	}

	// Start heartbeat loop
	a.wg.Add(1)
	go a.heartbeatLoop()

	return nil
}

// Stop stops the simulated agent.
func (a *SimulatedAgent) Stop() {
	a.cancel()
	a.wg.Wait()
}

// Metrics returns the agent's metrics.
func (a *SimulatedAgent) Metrics() AgentMetric {
	a.mu.Lock()
	defer a.mu.Unlock()

	var avgCommandTime time.Duration
	if len(a.commandLatencies) > 0 {
		var sum time.Duration
		for _, l := range a.commandLatencies {
			sum += l
		}
		avgCommandTime = sum / time.Duration(len(a.commandLatencies))
	}

	return AgentMetric{
		AgentID:          a.ID,
		RegistrationTime: a.registrationTime,
		HeartbeatsSent:   atomic.LoadInt64(&a.heartbeatsSent),
		CommandsReceived: atomic.LoadInt64(&a.commandsReceived),
		CommandsExecuted: atomic.LoadInt64(&a.commandsExecuted),
		AvgCommandTime:   avgCommandTime,
		Errors:           atomic.LoadInt64(&a.errors),
	}
}

// register sends a registration request.
func (a *SimulatedAgent) register() error {
	req := &pb.RegisterRequest{
		AgentId: a.ID,
		Metadata: &pb.AgentMetadata{
			Hostname:        a.Hostname,
			Os:              "linux",
			Arch:            "amd64",
			Ipv4Addresses:   []string{"192.168.1.100"},
			PlatformVersion: "simulated",
			AgentVersion:    "loadtest-1.0.0",
			Labels: map[string]string{
				"loadtest": "true",
				"agent_id": a.ID,
			},
		},
	}

	data, err := proto.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal registration: %w", err)
	}

	subject := a.subjects.AgentRegister()
	msg, err := a.nats.PublishRequest(subject, data, 10*time.Second)
	if err != nil {
		// In load tests without a control plane, just consider registered
		return nil
	}

	// Parse response if available
	var resp pb.RegisterResponse
	if err := proto.Unmarshal(msg.Data, &resp); err != nil {
		return fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return nil
}

// heartbeatLoop sends periodic heartbeats.
func (a *SimulatedAgent) heartbeatLoop() {
	defer a.wg.Done()

	ticker := time.NewTicker(a.heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := a.sendHeartbeat(); err != nil {
				atomic.AddInt64(&a.errors, 1)
			}
		case <-a.ctx.Done():
			return
		}
	}
}

// sendHeartbeat sends a heartbeat.
func (a *SimulatedAgent) sendHeartbeat() error {
	req := &pb.HeartbeatRequest{
		AgentId:   a.ID,
		Timestamp: timestamppb.Now(),
		Status:    pb.AgentStatus_AGENT_STATUS_ONLINE,
		Metrics: &pb.SystemMetrics{
			CpuPercent:    25.0,
			MemoryPercent: 50.0,
			DiskPercent:   40.0,
			LoadAverage:   []float32{1.5, 1.2, 1.0},
		},
	}

	data, err := proto.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal heartbeat: %w", err)
	}

	subject := a.subjects.AgentHeartbeat()
	if err := a.nats.Publish(subject, data); err != nil {
		return fmt.Errorf("failed to publish heartbeat: %w", err)
	}

	atomic.AddInt64(&a.heartbeatsSent, 1)
	return nil
}

// subscribeToCommands subscribes to command requests.
func (a *SimulatedAgent) subscribeToCommands() error {
	subject := a.subjects.AgentCommand(a.ID)

	_, err := a.nats.Subscribe(subject, func(msg *nats.Msg) {
		a.handleCommand(msg)
	})

	return err
}

// handleCommand processes a command request.
func (a *SimulatedAgent) handleCommand(msg *nats.Msg) {
	start := time.Now()
	atomic.AddInt64(&a.commandsReceived, 1)

	var req pb.ExecuteCommandRequest
	if err := proto.Unmarshal(msg.Data, &req); err != nil {
		atomic.AddInt64(&a.errors, 1)
		return
	}

	// Simulate command execution with a small delay
	time.Sleep(10 * time.Millisecond)

	// Send response
	resp := &pb.ExecuteCommandResponse{
		CommandId: req.CommandId,
		Type:      pb.CommandResponseType_COMMAND_RESPONSE_TYPE_COMPLETED,
		ExitCode:  0,
		Timestamp: timestamppb.Now(),
	}

	data, err := proto.Marshal(resp)
	if err != nil {
		atomic.AddInt64(&a.errors, 1)
		return
	}

	if msg.Reply != "" {
		if err := a.nats.Publish(msg.Reply, data); err != nil {
			atomic.AddInt64(&a.errors, 1)
			return
		}
	}

	latency := time.Since(start)
	a.mu.Lock()
	a.commandLatencies = append(a.commandLatencies, latency)
	a.mu.Unlock()

	atomic.AddInt64(&a.commandsExecuted, 1)
}

// AgentPool manages a pool of simulated agents.
type AgentPool struct {
	config  *Config
	nats    *natsmgr.Manager
	agents  []*SimulatedAgent
	mu      sync.RWMutex
	started bool
}

// NewAgentPool creates a new agent pool.
func NewAgentPool(cfg *Config, natsManager *natsmgr.Manager) *AgentPool {
	return &AgentPool{
		config: cfg,
		nats:   natsManager,
		agents: make([]*SimulatedAgent, 0, cfg.AgentCount),
	}
}

// StartAll starts all agents in the pool with optional ramp-up.
func (p *AgentPool) StartAll(ctx context.Context) error {
	p.mu.Lock()
	if p.started {
		p.mu.Unlock()
		return fmt.Errorf("pool already started")
	}
	p.started = true
	p.mu.Unlock()

	// Calculate delay between agent starts for ramp-up
	var delay time.Duration
	if p.config.RampUpDuration > 0 && p.config.AgentCount > 1 {
		delay = p.config.RampUpDuration / time.Duration(p.config.AgentCount-1)
	}

	var wg sync.WaitGroup
	errChan := make(chan error, p.config.AgentCount)

	for i := 0; i < p.config.AgentCount; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		agent := NewSimulatedAgent(p.nats, &SimulatedAgentConfig{
			ID:                fmt.Sprintf("loadtest-agent-%d", i),
			Hostname:          fmt.Sprintf("loadtest-host-%d", i),
			HeartbeatInterval: p.config.HeartbeatInterval,
			CommandTimeout:    p.config.CommandTimeout,
		})

		p.mu.Lock()
		p.agents = append(p.agents, agent)
		p.mu.Unlock()

		wg.Add(1)
		go func(a *SimulatedAgent) {
			defer wg.Done()
			if err := a.Start(); err != nil {
				errChan <- err
			}
		}(agent)

		// Apply ramp-up delay
		if delay > 0 && i < p.config.AgentCount-1 {
			time.Sleep(delay)
		}
	}

	wg.Wait()
	close(errChan)

	// Collect any errors
	var errors []error
	for err := range errChan {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		return fmt.Errorf("%d agents failed to start: %v", len(errors), errors[0])
	}

	return nil
}

// StopAll stops all agents in the pool.
func (p *AgentPool) StopAll() {
	p.mu.RLock()
	agents := p.agents
	p.mu.RUnlock()

	var wg sync.WaitGroup
	for _, agent := range agents {
		wg.Add(1)
		go func(a *SimulatedAgent) {
			defer wg.Done()
			a.Stop()
		}(agent)
	}
	wg.Wait()
}

// Agents returns all agents in the pool.
func (p *AgentPool) Agents() []*SimulatedAgent {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.agents
}

// AgentCount returns the current number of agents.
func (p *AgentPool) AgentCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.agents)
}

// CollectMetrics collects metrics from all agents.
func (p *AgentPool) CollectMetrics() []AgentMetric {
	p.mu.RLock()
	agents := p.agents
	p.mu.RUnlock()

	metrics := make([]AgentMetric, len(agents))
	for i, agent := range agents {
		metrics[i] = agent.Metrics()
	}
	return metrics
}

// AggregateMetrics aggregates metrics across all agents.
func (p *AgentPool) AggregateMetrics() Metrics {
	agentMetrics := p.CollectMetrics()

	var metrics Metrics
	collector := NewLatencyCollector()

	var totalRegTime time.Duration
	for _, am := range agentMetrics {
		metrics.HeartbeatsSent += am.HeartbeatsSent
		metrics.CommandsCompleted += am.CommandsExecuted
		metrics.FailedOps += am.Errors
		totalRegTime += am.RegistrationTime
		collector.Add(am.RegistrationTime)
	}

	metrics.AgentsRegistered = len(agentMetrics)
	if len(agentMetrics) > 0 {
		metrics.RegistrationTime = totalRegTime / time.Duration(len(agentMetrics))
	}

	metrics.TotalOps = metrics.HeartbeatsSent + metrics.CommandsCompleted
	metrics.SuccessfulOps = metrics.TotalOps - metrics.FailedOps

	if metrics.TotalOps > 0 {
		metrics.ErrorRate = float64(metrics.FailedOps) / float64(metrics.TotalOps) * 100
	}

	metrics.MinLatency, metrics.MaxLatency, metrics.AvgLatency,
		metrics.P50Latency, metrics.P95Latency, metrics.P99Latency = collector.Calculate()

	return metrics
}
