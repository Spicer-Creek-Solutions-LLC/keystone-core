package targeting

import (
	"fmt"
	"strings"

	pb "github.com/titananvil/titan-anvil/pkg/api/v1"
)

// Matcher filters agents based on target expressions
type Matcher struct {
	expression *TargetExpression
}

// NewMatcher creates a new matcher from a target expression string
func NewMatcher(expression string) (*Matcher, error) {
	expr, err := Parse(expression)
	if err != nil {
		return nil, fmt.Errorf("failed to parse target expression: %w", err)
	}

	return &Matcher{
		expression: expr,
	}, nil
}

// Match filters a list of agents and returns only those that match the target expression
func (m *Matcher) Match(agents []*AgentInfo) ([]*AgentInfo, error) {
	var matched []*AgentInfo

	for _, agent := range agents {
		metadata := agentToMetadata(agent)
		isMatch, err := m.expression.Matches(metadata)
		if err != nil {
			return nil, fmt.Errorf("failed to match agent %s: %w", agent.ID, err)
		}

		if isMatch {
			matched = append(matched, agent)
		}
	}

	return matched, nil
}

// MatchIDs filters a list of agents and returns the IDs of those that match
func (m *Matcher) MatchIDs(agents []*AgentInfo) ([]string, error) {
	matched, err := m.Match(agents)
	if err != nil {
		return nil, err
	}

	ids := make([]string, len(matched))
	for i, agent := range matched {
		ids[i] = agent.ID
	}

	return ids, nil
}

// agentToMetadata converts an AgentInfo to a flat metadata map for matching.
//
// The following fields are available for matching:
//   - id: Agent ID
//   - hostname: Hostname from agent metadata
//   - os: Operating system (linux, darwin, windows)
//   - arch: Architecture (amd64, arm64)
//   - platform_version: Platform/kernel version
//   - agent_version: TitanAnvil agent version
//   - status: Current agent status (online, offline, degraded)
//   - ip: Any of the agent's IP addresses (matches if pattern matches any IP)
//   - All custom labels from agent metadata labels map
func agentToMetadata(agent *AgentInfo) map[string]string {
	metadata := make(map[string]string)

	// Add agent ID
	metadata["id"] = agent.ID

	// Add status
	if agent.Status != pb.AgentStatus_AGENT_STATUS_UNSPECIFIED {
		metadata["status"] = strings.ToLower(agent.Status.String())
	}

	// Add fields from agent metadata if available
	if agent.Metadata != nil {
		metadata["hostname"] = agent.Metadata.Hostname
		metadata["os"] = agent.Metadata.Os
		metadata["arch"] = agent.Metadata.Arch
		metadata["platform_version"] = agent.Metadata.PlatformVersion
		metadata["agent_version"] = agent.Metadata.AgentVersion

		// Add IP addresses as a comma-separated list for matching
		// This allows matching like "ip:192.168.*" to match any IP
		if len(agent.Metadata.IpAddresses) > 0 {
			metadata["ip"] = strings.Join(agent.Metadata.IpAddresses, ",")
		}

		// Add custom labels directly to the metadata
		for k, v := range agent.Metadata.Labels {
			metadata[k] = v
		}
	}

	return metadata
}
