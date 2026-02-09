package visualization

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// AgentProvider defines the interface for getting agent data
type AgentProvider interface {
	// GetAgents returns all agents
	GetAgents() []*Agent

	// GetAgent returns a specific agent by ID
	GetAgent(id string) (*Agent, error)
}

// Server provides the visualization HTTP/WebSocket server
type Server struct {
	config    *Config
	provider  AgentProvider
	server    *http.Server
	upgrader  websocket.Upgrader
	clients   map[*websocket.Conn]bool
	clientsMu sync.RWMutex
	broadcast chan TopologyUpdate
	stopCh    chan struct{}
}

// NewServer creates a new visualization server
func NewServer(config *Config, provider AgentProvider) *Server {
	if config == nil {
		config = DefaultConfig()
	}

	return &Server{
		config:   config,
		provider: provider,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				origin := r.Header.Get("Origin")
				if origin == "" {
					return true
				}
				parsed, err := url.Parse(origin)
				if err != nil {
					return false
				}
				return strings.EqualFold(parsed.Host, r.Host)
			},
		},
		clients:   make(map[*websocket.Conn]bool),
		broadcast: make(chan TopologyUpdate, 100),
		stopCh:    make(chan struct{}),
	}
}

// Start starts the visualization server
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	// Register HTTP handlers
	mux.HandleFunc("/api/agents", s.handleAgents)
	mux.HandleFunc("/api/agents/", s.handleAgent)
	mux.HandleFunc("/api/topology", s.handleTopology)
	mux.HandleFunc("/api/graph", s.handleGraph)

	// Register WebSocket handler if enabled
	if s.config.EnableWebSocket {
		mux.HandleFunc("/ws/topology", s.handleWebSocket)
		go s.broadcastLoop()
	}

	s.server = &http.Server{
		Addr:              s.config.ListenAddr,
		Handler:           mux,
		ReadTimeout:       10 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("Visualization server error: %v\n", err)
		}
	}()

	return nil
}

// Stop stops the visualization server
func (s *Server) Stop(ctx context.Context) error {
	close(s.stopCh)

	if s.server != nil {
		return s.server.Shutdown(ctx)
	}

	return nil
}

// BroadcastUpdate broadcasts a topology update to all connected clients
func (s *Server) BroadcastUpdate(update TopologyUpdate) {
	select {
	case s.broadcast <- update:
	default:
		// Channel full, skip this update
	}
}

// handleAgents handles GET /api/agents
func (s *Server) handleAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get filter options from query parameters
	filter := FilterOptions{
		Datacenter:  r.URL.Query().Get("datacenter"),
		Environment: r.URL.Query().Get("environment"),
		Role:        r.URL.Query().Get("role"),
	}

	if status := r.URL.Query().Get("status"); status != "" {
		filter.Status = AgentStatus(status)
	}

	agents := s.provider.GetAgents()

	// Apply filters
	filtered := make([]*Agent, 0)
	for _, agent := range agents {
		if matchesFilter(agent, &filter) {
			filtered = append(filtered, agent)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"agents": filtered,
		"total":  len(filtered),
	})
}

// handleAgent handles GET /api/agents/{id}
func (s *Server) handleAgent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract agent ID from path
	id := r.URL.Path[len("/api/agents/"):]
	if id == "" {
		http.Error(w, "Agent ID required", http.StatusBadRequest)
		return
	}

	agent, err := s.provider.GetAgent(id)
	if err != nil {
		http.Error(w, "Agent not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(agent)
}

// handleTopology handles GET /api/topology
func (s *Server) handleTopology(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	agents := s.provider.GetAgents()
	topology := buildTopology(agents)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(topology)
}

// handleGraph handles GET /api/graph
func (s *Server) handleGraph(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	agents := s.provider.GetAgents()
	graph := buildGraph(agents)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(graph)
}

// handleWebSocket handles WebSocket connections for real-time updates
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil) // nosemgrep: go.gorilla.security.audit.websocket-missing-origin-check.websocket-missing-origin-check -- CheckOrigin enforces same-host policy in NewServer
	if err != nil {
		return
	}

	s.clientsMu.Lock()
	s.clients[conn] = true
	s.clientsMu.Unlock()

	// Send initial topology
	agents := s.provider.GetAgents()
	topology := buildTopology(agents)
	_ = conn.WriteJSON(map[string]interface{}{ //nolint:errcheck // best-effort initial message
		"type": "initial",
		"data": topology,
	})

	// Handle client disconnect
	go func() {
		defer func() {
			s.clientsMu.Lock()
			delete(s.clients, conn)
			s.clientsMu.Unlock()
			conn.Close()
		}()

		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				break
			}
		}
	}()
}

// broadcastLoop broadcasts updates to all connected WebSocket clients
func (s *Server) broadcastLoop() {
	ticker := time.NewTicker(s.config.UpdateInterval)
	defer ticker.Stop()

	for {
		select {
		case update := <-s.broadcast:
			s.broadcastToClients(update)
		case <-ticker.C:
			// Periodic update with current topology
			agents := s.provider.GetAgents()
			topology := buildTopology(agents)
			s.broadcastToClients(TopologyUpdate{
				Type:      "periodic_update",
				Timestamp: time.Now(),
				Agent:     nil,
			})
			_ = topology // Avoid unused warning
		case <-s.stopCh:
			return
		}
	}
}

func (s *Server) broadcastToClients(update TopologyUpdate) {
	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()

	for client := range s.clients {
		err := client.WriteJSON(update)
		if err != nil {
			client.Close()
			delete(s.clients, client)
		}
	}
}

// matchesFilter checks if an agent matches the filter options
func matchesFilter(agent *Agent, filter *FilterOptions) bool {
	if filter.Datacenter != "" && agent.Datacenter != filter.Datacenter {
		return false
	}

	if filter.Environment != "" && agent.Environment != filter.Environment {
		return false
	}

	if filter.Role != "" && agent.Role != filter.Role {
		return false
	}

	if filter.Status != "" && agent.Status != filter.Status {
		return false
	}

	if len(filter.Tags) > 0 {
		agentTags := make(map[string]bool)
		for _, tag := range agent.Tags {
			agentTags[tag] = true
		}

		for _, tag := range filter.Tags {
			if !agentTags[tag] {
				return false
			}
		}
	}

	return true
}

// buildTopology builds a hierarchical topology tree from agents
func buildTopology(agents []*Agent) *TopologyNode {
	root := &TopologyNode{
		ID:     "root",
		Name:   "Infrastructure",
		Type:   "root",
		Status: AgentStatusHealthy,
		Stats:  &NodeStats{},
	}

	// Group by datacenter -> environment -> role
	datacenters := make(map[string]*TopologyNode)

	for _, agent := range agents {
		datacenter := agent.Datacenter
		if datacenter == "" {
			datacenter = "unknown"
		}

		// Get or create datacenter node
		dcNode, exists := datacenters[datacenter]
		if !exists {
			dcNode = &TopologyNode{
				ID:     datacenter,
				Name:   datacenter,
				Type:   "datacenter",
				Status: AgentStatusHealthy,
				Stats:  &NodeStats{},
			}
			datacenters[datacenter] = dcNode
			root.Children = append(root.Children, dcNode)
		}

		// Get or create environment node
		environment := agent.Environment
		if environment == "" {
			environment = "default"
		}

		var envNode *TopologyNode
		for _, child := range dcNode.Children {
			if child.ID == datacenter+"/"+environment {
				envNode = child
				break
			}
		}

		if envNode == nil {
			envNode = &TopologyNode{
				ID:     datacenter + "/" + environment,
				Name:   environment,
				Type:   "environment",
				Status: AgentStatusHealthy,
				Stats:  &NodeStats{},
			}
			dcNode.Children = append(dcNode.Children, envNode)
		}

		// Get or create role node
		role := agent.Role
		if role == "" {
			role = "default"
		}

		var roleNode *TopologyNode
		for _, child := range envNode.Children {
			if child.ID == datacenter+"/"+environment+"/"+role {
				roleNode = child
				break
			}
		}

		if roleNode == nil {
			roleNode = &TopologyNode{
				ID:     datacenter + "/" + environment + "/" + role,
				Name:   role,
				Type:   "role",
				Status: AgentStatusHealthy,
				Stats:  &NodeStats{},
			}
			envNode.Children = append(envNode.Children, roleNode)
		}

		// Add agent node
		agentNode := &TopologyNode{
			ID:     agent.ID,
			Name:   agent.Hostname,
			Type:   "agent",
			Status: agent.Status,
			Agent:  agent,
		}
		roleNode.Children = append(roleNode.Children, agentNode)

		// Update stats
		updateStats(roleNode, agent.Status)
		updateStats(envNode, agent.Status)
		updateStats(dcNode, agent.Status)
		updateStats(root, agent.Status)
	}

	// Update aggregated status for all nodes
	updateNodeStatus(root)

	return root
}

// updateStats updates node statistics
func updateStats(node *TopologyNode, status AgentStatus) {
	if node.Stats == nil {
		node.Stats = &NodeStats{}
	}

	node.Stats.Total++

	switch status {
	case AgentStatusHealthy:
		node.Stats.Healthy++
	case AgentStatusDegraded:
		node.Stats.Degraded++
	case AgentStatusOffline:
		node.Stats.Offline++
	default:
		// AgentStatusUnknown not counted in individual categories
	}
}

// updateNodeStatus updates the aggregated status for a node based on child statuses
func updateNodeStatus(node *TopologyNode) AgentStatus {
	if node.Type == "agent" {
		return node.Status
	}

	if len(node.Children) == 0 {
		return AgentStatusUnknown
	}

	hasOffline := false
	hasDegraded := false

	for _, child := range node.Children {
		childStatus := updateNodeStatus(child)

		switch childStatus {
		case AgentStatusOffline:
			hasOffline = true
		case AgentStatusDegraded:
			hasDegraded = true
		default:
			// AgentStatusHealthy, AgentStatusUnknown don't set flags
		}
	}

	switch {
	case hasOffline:
		node.Status = AgentStatusDegraded
	case hasDegraded:
		node.Status = AgentStatusDegraded
	default:
		node.Status = AgentStatusHealthy
	}

	return node.Status
}

// buildGraph builds a graph representation of the infrastructure
func buildGraph(agents []*Agent) *Graph {
	graph := &Graph{
		Nodes: make([]GraphNode, 0),
		Edges: make([]GraphEdge, 0),
	}

	// Add agent nodes
	for _, agent := range agents {
		graph.Nodes = append(graph.Nodes, GraphNode{
			ID:     agent.ID,
			Label:  agent.Hostname,
			Type:   "agent",
			Status: agent.Status,
			Metadata: map[string]interface{}{
				"datacenter":  agent.Datacenter,
				"environment": agent.Environment,
				"role":        agent.Role,
			},
		})
	}

	// Add edges between agents in the same datacenter
	for i, agent1 := range agents {
		for _, agent2 := range agents[i+1:] {
			if agent1.Datacenter != "" && agent1.Datacenter == agent2.Datacenter {
				graph.Edges = append(graph.Edges, GraphEdge{
					Source: agent1.ID,
					Target: agent2.ID,
					Type:   "same_datacenter",
				})
			}
		}
	}

	return graph
}
