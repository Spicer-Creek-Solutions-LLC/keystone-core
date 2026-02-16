package mcp

import "fmt"

// Profile controls which MCP tools are registered. This is client-side
// defense-in-depth — server-side RBAC remains the authoritative enforcement.
type Profile string

// MCP capability profiles controlling tool availability.
const (
	ProfileReadOnly Profile = "read_only"
	ProfileOpsSafe  Profile = "ops_safe"
	ProfileOpsAdmin Profile = "ops_admin"
)

// profileTools maps each profile to the set of allowed tool names.
var profileTools = map[Profile]map[string]bool{
	ProfileReadOnly: {
		"agent_list":     true,
		"agent_show":     true,
		"agent_health":   true,
		"exec_status":    true,
		"exec_history":   true,
		"state_check":    true,
		"state_drift":    true,
		"state_history":  true,
		"event_query":    true,
		"event_stats":    true,
		"runbook_list":   true,
		"runbook_status": true,
		"cluster_status": true,
	},
	ProfileOpsSafe: {
		// Inherits all read_only tools
		"agent_list":      true,
		"agent_show":      true,
		"agent_health":    true,
		"exec_run":        true,
		"exec_status":     true,
		"exec_history":    true,
		"state_check":     true,
		"state_drift":     true,
		"state_history":   true,
		"event_query":     true,
		"event_stats":     true,
		"runbook_list":    true,
		"runbook_execute": true,
		"runbook_status":  true,
		"cluster_status":  true,
	},
	ProfileOpsAdmin: {
		// All tools
		"agent_list":      true,
		"agent_show":      true,
		"agent_health":    true,
		"exec_run":        true,
		"exec_status":     true,
		"exec_history":    true,
		"state_check":     true,
		"state_apply":     true,
		"state_drift":     true,
		"state_history":   true,
		"event_query":     true,
		"event_stats":     true,
		"runbook_list":    true,
		"runbook_execute": true,
		"runbook_status":  true,
		"cluster_status":  true,
	},
}

// ParseProfile validates and returns a Profile.
func ParseProfile(s string) (Profile, error) {
	p := Profile(s)
	if _, ok := profileTools[p]; !ok {
		return "", fmt.Errorf("unknown profile %q: must be read_only, ops_safe, or ops_admin", s)
	}
	return p, nil
}

// Allows returns true if the profile permits the given tool name.
func (p Profile) Allows(toolName string) bool {
	allowed, ok := profileTools[p]
	if !ok {
		return false
	}
	return allowed[toolName]
}
