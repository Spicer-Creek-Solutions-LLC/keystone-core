// Package visualization provides topology visualization for Keystone Core.
// It implements:
//   - Agent topology views (datacenter, environment, role hierarchy)
//   - Graph representation with nodes and edges
//   - Real-time updates via WebSocket
//   - Filtering by datacenter, environment, role, status, and tags
//   - HTTP API for topology and graph data
//
// The visualization package enables dashboards and UIs to display
// infrastructure topology and agent relationships.
package visualization

import (
	"time"
)

// AgentStatus represents the status of an agent
type AgentStatus string

const (
	AgentStatusHealthy   AgentStatus = "healthy"
	AgentStatusDegraded  AgentStatus = "degraded"
	AgentStatusOffline   AgentStatus = "offline"
	AgentStatusUnknown   AgentStatus = "unknown"
)

// Agent represents an agent in the visualization
type Agent struct {
	// ID is the unique agent identifier
	ID string `json:"id"`

	// Hostname is the agent hostname
	Hostname string `json:"hostname"`

	// Status is the current agent status
	Status AgentStatus `json:"status"`

	// LastSeen is when the agent was last seen
	LastSeen time.Time `json:"last_seen"`

	// Metadata contains agent metadata
	Metadata map[string]string `json:"metadata"`

	// Tags are agent tags
	Tags []string `json:"tags,omitempty"`

	// Datacenter is the datacenter this agent belongs to
	Datacenter string `json:"datacenter,omitempty"`

	// Environment is the environment (prod, staging, dev)
	Environment string `json:"environment,omitempty"`

	// Role is the agent role (web, db, cache, etc.)
	Role string `json:"role,omitempty"`

	// Version is the agent version
	Version string `json:"version,omitempty"`
}

// TopologyNode represents a node in the topology tree
type TopologyNode struct {
	// ID is the unique node identifier
	ID string `json:"id"`

	// Name is the node display name
	Name string `json:"name"`

	// Type is the node type (datacenter, environment, role, agent)
	Type string `json:"type"`

	// Status is the aggregated status for this node
	Status AgentStatus `json:"status"`

	// Children are child nodes
	Children []*TopologyNode `json:"children,omitempty"`

	// Agent is the agent details (if this is an agent node)
	Agent *Agent `json:"agent,omitempty"`

	// Stats contains statistics for this node
	Stats *NodeStats `json:"stats,omitempty"`
}

// NodeStats contains statistics for a topology node
type NodeStats struct {
	// Total is the total number of agents under this node
	Total int `json:"total"`

	// Healthy is the number of healthy agents
	Healthy int `json:"healthy"`

	// Degraded is the number of degraded agents
	Degraded int `json:"degraded"`

	// Offline is the number of offline agents
	Offline int `json:"offline"`
}

// GraphNode represents a node in a graph visualization
type GraphNode struct {
	// ID is the unique node identifier
	ID string `json:"id"`

	// Label is the node label
	Label string `json:"label"`

	// Type is the node type
	Type string `json:"type"`

	// Status is the node status
	Status AgentStatus `json:"status"`

	// Metadata contains additional node data
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// GraphEdge represents an edge in a graph visualization
type GraphEdge struct {
	// Source is the source node ID
	Source string `json:"source"`

	// Target is the target node ID
	Target string `json:"target"`

	// Label is the edge label
	Label string `json:"label,omitempty"`

	// Type is the edge type (e.g., "depends_on", "same_datacenter")
	Type string `json:"type,omitempty"`
}

// Graph represents a graph visualization of the infrastructure
type Graph struct {
	// Nodes are the graph nodes
	Nodes []GraphNode `json:"nodes"`

	// Edges are the graph edges
	Edges []GraphEdge `json:"edges"`
}

// TopologyUpdate represents a real-time topology update
type TopologyUpdate struct {
	// Type is the update type (agent_added, agent_removed, agent_updated, status_changed)
	Type string `json:"type"`

	// AgentID is the affected agent ID
	AgentID string `json:"agent_id,omitempty"`

	// Agent is the updated agent data
	Agent *Agent `json:"agent,omitempty"`

	// Timestamp is when the update occurred
	Timestamp time.Time `json:"timestamp"`
}

// FilterOptions defines filtering options for agent lists
type FilterOptions struct {
	// Datacenter filters by datacenter
	Datacenter string `json:"datacenter,omitempty"`

	// Environment filters by environment
	Environment string `json:"environment,omitempty"`

	// Role filters by role
	Role string `json:"role,omitempty"`

	// Status filters by status
	Status AgentStatus `json:"status,omitempty"`

	// Tags filters by tags (all tags must match)
	Tags []string `json:"tags,omitempty"`
}

// VisualizationConfig configures the visualization server
type VisualizationConfig struct {
	// ListenAddr is the address to listen on
	ListenAddr string

	// EnableWebSocket enables WebSocket support for real-time updates
	EnableWebSocket bool

	// UpdateInterval is how often to broadcast updates (default: 5s)
	UpdateInterval time.Duration
}

// DefaultConfig returns a default visualization configuration
func DefaultConfig() *VisualizationConfig {
	return &VisualizationConfig{
		ListenAddr:      "localhost:8080",
		EnableWebSocket: true,
		UpdateInterval:  5 * time.Second,
	}
}
