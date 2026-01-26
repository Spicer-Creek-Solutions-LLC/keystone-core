// Package nats provides NATS messaging infrastructure for Keystone Core.
package nats

import (
	"fmt"
	"regexp"
	"strings"
)

// SubjectBuilder constructs NATS subject strings with cluster prefixing.
// All subjects follow the pattern: kscore.{cluster}.{category}.{...}
//
// Subject Hierarchy:
//
//	kscore.{cluster}.agent.register              - Agent registration
//	kscore.{cluster}.agent.heartbeat             - Agent heartbeats
//	kscore.{cluster}.agent.{id}.command          - Commands to agent
//	kscore.{cluster}.agent.{id}.response         - Responses from agent
//	kscore.{cluster}.agent.{id}.state            - State operations
//	kscore.{cluster}.agent.{id}.events           - Agent events
//	kscore.{cluster}.server.announce             - Server announcements
//	kscore.{cluster}.server.{id}.control         - Server control channel
//	kscore.{cluster}.discovery                   - Peer discovery
//	kscore.{cluster}.bootstrap.{id}.register     - Bootstrap registration
//	kscore.{cluster}.bootstrap.{id}.response     - Bootstrap response
type SubjectBuilder struct {
	cluster string
}

// DefaultCluster is used when no cluster name is specified
const DefaultCluster = "default"

// SubjectPrefix is the root prefix for all Keystone Core subjects
const SubjectPrefix = "kscore"

// Subject category constants
const (
	CategoryAgent     = "agent"
	CategoryServer    = "server"
	CategoryDiscovery = "discovery"
	CategoryBootstrap = "bootstrap"
)

// Subject operation constants
const (
	OpRegister  = "register"
	OpHeartbeat = "heartbeat"
	OpCommand   = "command"
	OpResponse  = "response"
	OpState     = "state"
	OpEvents    = "events"
	OpAnnounce  = "announce"
	OpControl   = "control"
)

// NewSubjectBuilder creates a new SubjectBuilder with the given cluster name.
// If cluster is empty, DefaultCluster is used.
func NewSubjectBuilder(cluster string) *SubjectBuilder {
	if cluster == "" {
		cluster = DefaultCluster
	}
	return &SubjectBuilder{cluster: cluster}
}

// Cluster returns the cluster name
func (b *SubjectBuilder) Cluster() string {
	return b.cluster
}

// base returns the base subject prefix: kscore.{cluster}
func (b *SubjectBuilder) base() string {
	return fmt.Sprintf("%s.%s", SubjectPrefix, b.cluster)
}

// --- Agent Subjects ---

// AgentRegister returns the subject for agent registration requests.
// Subject: kscore.{cluster}.agent.register
func (b *SubjectBuilder) AgentRegister() string {
	return fmt.Sprintf("%s.%s.%s", b.base(), CategoryAgent, OpRegister)
}

// AgentHeartbeat returns the subject for agent heartbeats.
// Subject: kscore.{cluster}.agent.heartbeat
func (b *SubjectBuilder) AgentHeartbeat() string {
	return fmt.Sprintf("%s.%s.%s", b.base(), CategoryAgent, OpHeartbeat)
}

// AgentCommand returns the subject for sending commands to a specific agent.
// Subject: kscore.{cluster}.agent.{agentID}.command
func (b *SubjectBuilder) AgentCommand(agentID string) string {
	return fmt.Sprintf("%s.%s.%s.%s", b.base(), CategoryAgent, agentID, OpCommand)
}

// AgentResponse returns the subject for receiving responses from a specific agent.
// Subject: kscore.{cluster}.agent.{agentID}.response
func (b *SubjectBuilder) AgentResponse(agentID string) string {
	return fmt.Sprintf("%s.%s.%s.%s", b.base(), CategoryAgent, agentID, OpResponse)
}

// AgentState returns the subject for state operations to a specific agent.
// Subject: kscore.{cluster}.agent.{agentID}.state
func (b *SubjectBuilder) AgentState(agentID string) string {
	return fmt.Sprintf("%s.%s.%s.%s", b.base(), CategoryAgent, agentID, OpState)
}

// AgentEvents returns the subject for events from a specific agent.
// Subject: kscore.{cluster}.agent.{agentID}.events
func (b *SubjectBuilder) AgentEvents(agentID string) string {
	return fmt.Sprintf("%s.%s.%s.%s", b.base(), CategoryAgent, agentID, OpEvents)
}

// AgentWildcard returns a wildcard subject for all agent operations.
// Subject: kscore.{cluster}.agent.>
func (b *SubjectBuilder) AgentWildcard() string {
	return fmt.Sprintf("%s.%s.>", b.base(), CategoryAgent)
}

// AgentIDWildcard returns a wildcard subject for all operations on a specific agent.
// Subject: kscore.{cluster}.agent.{agentID}.*
func (b *SubjectBuilder) AgentIDWildcard(agentID string) string {
	return fmt.Sprintf("%s.%s.%s.*", b.base(), CategoryAgent, agentID)
}

// --- Server Subjects ---

// ServerAnnounce returns the subject for server announcements.
// Subject: kscore.{cluster}.server.announce
func (b *SubjectBuilder) ServerAnnounce() string {
	return fmt.Sprintf("%s.%s.%s", b.base(), CategoryServer, OpAnnounce)
}

// ServerControl returns the subject for control messages to a specific server.
// Subject: kscore.{cluster}.server.{serverID}.control
func (b *SubjectBuilder) ServerControl(serverID string) string {
	return fmt.Sprintf("%s.%s.%s.%s", b.base(), CategoryServer, serverID, OpControl)
}

// ServerWildcard returns a wildcard subject for all server operations.
// Subject: kscore.{cluster}.server.>
func (b *SubjectBuilder) ServerWildcard() string {
	return fmt.Sprintf("%s.%s.>", b.base(), CategoryServer)
}

// --- Discovery Subjects ---

// Discovery returns the subject for peer discovery.
// Subject: kscore.{cluster}.discovery
func (b *SubjectBuilder) Discovery() string {
	return fmt.Sprintf("%s.%s", b.base(), CategoryDiscovery)
}

// --- Bootstrap Subjects (for secure agent registration) ---

// BootstrapRegister returns the subject for bootstrap registration requests.
// Subject: kscore.{cluster}.bootstrap.{bootstrapID}.register
func (b *SubjectBuilder) BootstrapRegister(bootstrapID string) string {
	return fmt.Sprintf("%s.%s.%s.%s", b.base(), CategoryBootstrap, bootstrapID, OpRegister)
}

// BootstrapResponse returns the subject for bootstrap registration responses.
// Subject: kscore.{cluster}.bootstrap.{bootstrapID}.response
func (b *SubjectBuilder) BootstrapResponse(bootstrapID string) string {
	return fmt.Sprintf("%s.%s.%s.%s", b.base(), CategoryBootstrap, bootstrapID, OpResponse)
}

// BootstrapWildcard returns a wildcard for all bootstrap subjects (for server subscription).
// Subject: kscore.{cluster}.bootstrap.>
func (b *SubjectBuilder) BootstrapWildcard() string {
	return fmt.Sprintf("%s.%s.>", b.base(), CategoryBootstrap)
}

// --- Command Response Subjects ---

// CommandResponse returns the subject for command responses by command ID.
// Subject: kscore.{cluster}.command.{commandID}.response
func (b *SubjectBuilder) CommandResponse(commandID string) string {
	return fmt.Sprintf("%s.command.%s.%s", b.base(), commandID, OpResponse)
}

// CommandResponseWildcard returns a wildcard subject for all command responses.
// Subject: kscore.{cluster}.command.*.response
func (b *SubjectBuilder) CommandResponseWildcard() string {
	return fmt.Sprintf("%s.command.*.%s", b.base(), OpResponse)
}

// --- Subject Parsing ---

// ParsedSubject contains the parsed components of a NATS subject
type ParsedSubject struct {
	Cluster   string
	Category  string
	EntityID  string // Agent ID, Server ID, Bootstrap ID, or Command ID
	Operation string
	IsValid   bool
}

// subjectPattern matches: kscore.{cluster}.{category}[.{entityID}][.{operation}]
var subjectPattern = regexp.MustCompile(`^kscore\.([^.]+)\.([^.]+)(?:\.([^.]+))?(?:\.([^.]+))?$`)

// ParseSubject parses a NATS subject into its components.
// Returns a ParsedSubject with IsValid=false if the subject doesn't match the expected pattern.
func ParseSubject(subject string) ParsedSubject {
	matches := subjectPattern.FindStringSubmatch(subject)
	if matches == nil {
		return ParsedSubject{IsValid: false}
	}

	p := ParsedSubject{
		Cluster:  matches[1],
		Category: matches[2],
		IsValid:  true,
	}

	// Handle different patterns based on category
	switch p.Category {
	case CategoryAgent, CategoryServer, CategoryBootstrap:
		// These categories have: category.entityID.operation or category.operation
		if matches[3] != "" {
			// Check if matches[3] is an operation or an entity ID
			if isOperation(matches[3]) {
				p.Operation = matches[3]
			} else {
				p.EntityID = matches[3]
				if matches[4] != "" {
					p.Operation = matches[4]
				}
			}
		}
	case CategoryDiscovery:
		// Discovery has no additional components
	case "command":
		// command.{commandID}.response
		p.EntityID = matches[3]
		p.Operation = matches[4]
	}

	return p
}

// isOperation checks if a string is a known operation
func isOperation(s string) bool {
	switch s {
	case OpRegister, OpHeartbeat, OpCommand, OpResponse, OpState, OpEvents, OpAnnounce, OpControl:
		return true
	}
	return false
}

// --- Subject Validation ---

// ValidateSubject validates that a subject follows the Keystone Core naming conventions.
// Returns an error if the subject is invalid.
func ValidateSubject(subject string) error {
	if subject == "" {
		return fmt.Errorf("subject cannot be empty")
	}

	if !strings.HasPrefix(subject, SubjectPrefix+".") {
		return fmt.Errorf("subject must start with '%s.'", SubjectPrefix)
	}

	parts := strings.Split(subject, ".")
	if len(parts) < 3 {
		return fmt.Errorf("subject must have at least 3 parts: %s.{cluster}.{category}", SubjectPrefix)
	}

	// Validate cluster name (no wildcards allowed in cluster position for publishing)
	cluster := parts[1]
	if cluster == "" || cluster == "*" || cluster == ">" {
		return fmt.Errorf("cluster name cannot be empty or a wildcard")
	}

	// Validate category
	category := parts[2]
	switch category {
	case CategoryAgent, CategoryServer, CategoryDiscovery, CategoryBootstrap, "command":
		// Valid categories
	case "*", ">":
		// Wildcards allowed for subscriptions
	default:
		return fmt.Errorf("unknown category: %s", category)
	}

	return nil
}

// ValidateSubjectForPublish validates a subject specifically for publishing.
// Wildcards are not allowed in any position.
func ValidateSubjectForPublish(subject string) error {
	if err := ValidateSubject(subject); err != nil {
		return err
	}

	if strings.Contains(subject, "*") || strings.Contains(subject, ">") {
		return fmt.Errorf("wildcards are not allowed when publishing")
	}

	return nil
}

// --- Subject Access Control ---

// SubjectPermissions defines the publish/subscribe permissions for a subject pattern
type SubjectPermissions struct {
	Publish   []string // Subjects allowed to publish to
	Subscribe []string // Subjects allowed to subscribe to
}

// BootstrapPermissions returns the minimal permissions for bootstrap credentials.
// Bootstrap agents can only register and receive their registration response.
func BootstrapPermissions(cluster, bootstrapID string) SubjectPermissions {
	b := NewSubjectBuilder(cluster)
	return SubjectPermissions{
		Publish: []string{
			b.AgentRegister(), // Can only publish to registration topic
		},
		Subscribe: []string{
			b.BootstrapResponse(bootstrapID), // Can only subscribe to own response
		},
	}
}

// AgentPermissions returns the permissions for a fully registered agent.
// Agents can publish heartbeats and responses, and subscribe to commands.
func AgentPermissions(cluster, agentID string) SubjectPermissions {
	b := NewSubjectBuilder(cluster)
	return SubjectPermissions{
		Publish: []string{
			b.AgentHeartbeat(),       // Publish heartbeats
			b.AgentResponse(agentID), // Publish responses
			b.AgentEvents(agentID),   // Publish events
		},
		Subscribe: []string{
			b.AgentCommand(agentID), // Subscribe to commands
			b.AgentState(agentID),   // Subscribe to state operations
		},
	}
}

// ServerPermissions returns the permissions for a control plane server.
// Servers can publish commands and subscribe to all agent communications.
func ServerPermissions(cluster, serverID string) SubjectPermissions {
	b := NewSubjectBuilder(cluster)
	return SubjectPermissions{
		Publish: []string{
			b.ServerAnnounce(),        // Announce presence
			fmt.Sprintf("%s.>", b.base()), // Can publish to any subject in cluster
		},
		Subscribe: []string{
			b.AgentWildcard(),     // Subscribe to all agent messages
			b.ServerWildcard(),    // Subscribe to all server messages
			b.BootstrapWildcard(), // Subscribe to bootstrap requests
			b.Discovery(),         // Subscribe to discovery
		},
	}
}
