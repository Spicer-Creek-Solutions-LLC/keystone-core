package auth

import (
	"context"
	"strings"
	"sync"
)

// MethodPermission defines the required role for a gRPC method
type MethodPermission struct {
	// Method is the full gRPC method name
	Method string
	// RequiredRole is the minimum role required
	RequiredRole Role
}

// RBACAuthorizer implements role-based access control
type RBACAuthorizer struct {
	mu          sync.RWMutex
	permissions map[string]Role
	// Methods that bypass authorization entirely (e.g., health checks)
	bypassMethods map[string]bool
	// enabled controls whether authorization checks are performed
	enabled bool
	// defaultDeny controls behavior for unmapped methods:
	// true = deny (require admin), false = allow any authenticated user
	defaultDeny bool
}

// NewRBACAuthorizer creates a new RBAC authorizer with default permissions.
//
// RBAC Permission Design:
//   - RoleReadonly: View/list operations that expose no sensitive data
//   - RoleOperator: Execute commands, apply state, emit events - day-to-day operations
//   - RoleAdmin: Destructive operations (delete), cluster management, backup/restore
//
// Agent endpoints use bypass (authenticated via mTLS/join tokens, not user RBAC).
// Coordination endpoints use bypass (internal server-to-server, mTLS-only).
func NewRBACAuthorizer(bypassMethods []string) *RBACAuthorizer {
	return NewRBACAuthorizerWithConfig(bypassMethods, true, true)
}

// NewRBACAuthorizerWithConfig creates a new RBAC authorizer with configurable authorization behavior.
// When enabled is false, all authorization checks are skipped (any authenticated user is allowed).
// When defaultDeny is true, unmapped methods require admin role; when false, any authenticated user is allowed.
func NewRBACAuthorizerWithConfig(bypassMethods []string, enabled, defaultDeny bool) *RBACAuthorizer {
	auth := &RBACAuthorizer{
		permissions:   make(map[string]Role),
		bypassMethods: make(map[string]bool),
		enabled:       enabled,
		defaultDeny:   defaultDeny,
	}

	// Set up bypass methods
	for _, method := range bypassMethods {
		auth.bypassMethods[method] = true
	}

	// =========================================================================
	// AgentService - Agent-to-server communication (bypass RBAC, use agent auth)
	// =========================================================================
	// Agents authenticate via mTLS certificates or join tokens, not user roles.
	// These methods are bypassed from RBAC but still require valid agent identity.
	auth.AddBypassMethod("/kscore.v1.AgentService/Register")       // Agent registration with join token
	auth.AddBypassMethod("/kscore.v1.AgentService/Heartbeat")      // Agent health reporting
	auth.AddBypassMethod("/kscore.v1.AgentService/ExecuteCommand") // Agent receives commands to execute
	auth.AddBypassMethod("/kscore.v1.AgentService/GetAgentInfo")   // Agent self-info retrieval

	// =========================================================================
	// ControlPlaneService - Primary API for operators and tools
	// =========================================================================
	// Server status: any authenticated user can check health
	auth.SetPermission("/kscore.v1.ControlPlaneService/GetServerStatus", RoleReadonly)

	// Agent information: view fleet status (no sensitive data)
	auth.SetPermission("/kscore.v1.ControlPlaneService/ListAgents", RoleReadonly)
	auth.SetPermission("/kscore.v1.ControlPlaneService/GetAgent", RoleReadonly)

	// Command execution: operators can run commands on infrastructure
	auth.SetPermission("/kscore.v1.ControlPlaneService/ExecuteCommand", RoleOperator)
	auth.SetPermission("/kscore.v1.ControlPlaneService/BatchExecuteCommand", RoleOperator)

	// Command status: view execution results (may contain command output)
	auth.SetPermission("/kscore.v1.ControlPlaneService/GetCommandStatus", RoleReadonly)
	auth.SetPermission("/kscore.v1.ControlPlaneService/ListCommands", RoleReadonly)
	auth.SetPermission("/kscore.v1.ControlPlaneService/GetBatchJobStatus", RoleReadonly)
	auth.SetPermission("/kscore.v1.ControlPlaneService/ListBatchJobs", RoleReadonly)

	// =========================================================================
	// StateService - Declarative state management
	// =========================================================================
	// Apply state: modifies infrastructure, requires operator role
	auth.SetPermission("/kscore.v1.StateService/ApplyState", RoleOperator)

	// Check/drift: read-only inspection of state compliance
	auth.SetPermission("/kscore.v1.StateService/CheckState", RoleReadonly)
	auth.SetPermission("/kscore.v1.StateService/DetectDrift", RoleReadonly)

	// State history/status: view past state applications
	auth.SetPermission("/kscore.v1.StateService/GetStateHistory", RoleReadonly)
	auth.SetPermission("/kscore.v1.StateService/GetStateStatus", RoleReadonly)

	// =========================================================================
	// EventService - Event bus access
	// =========================================================================
	// Read events: any authenticated user can view events
	auth.SetPermission("/kscore.v1.EventService/ListEvents", RoleReadonly)
	auth.SetPermission("/kscore.v1.EventService/GetEvent", RoleReadonly)
	auth.SetPermission("/kscore.v1.EventService/SubscribeEvents", RoleReadonly)
	auth.SetPermission("/kscore.v1.EventService/GetEventTypes", RoleReadonly)
	auth.SetPermission("/kscore.v1.EventService/GetEventStats", RoleReadonly)

	// Emit events: operators can publish custom events
	auth.SetPermission("/kscore.v1.EventService/EmitEvent", RoleOperator)

	// =========================================================================
	// PolicyService - Policy management and compliance
	// =========================================================================
	// Policy evaluation: read-only policy checks
	auth.SetPermission("/kscore.v1.PolicyService/EvaluatePolicy", RoleReadonly)
	auth.SetPermission("/kscore.v1.PolicyService/EvaluatePolicySet", RoleReadonly)

	// Policy listing/viewing: any authenticated user can view policies
	auth.SetPermission("/kscore.v1.PolicyService/ListPolicies", RoleReadonly)
	auth.SetPermission("/kscore.v1.PolicyService/GetPolicy", RoleReadonly)
	auth.SetPermission("/kscore.v1.PolicyService/ListPolicySets", RoleReadonly)
	auth.SetPermission("/kscore.v1.PolicyService/GetPolicySet", RoleReadonly)

	// Policy management: operators can create/update policies
	auth.SetPermission("/kscore.v1.PolicyService/CreatePolicy", RoleOperator)
	auth.SetPermission("/kscore.v1.PolicyService/UpdatePolicy", RoleOperator)

	// Policy deletion: admin only (destructive, may affect compliance)
	auth.SetPermission("/kscore.v1.PolicyService/DeletePolicy", RoleAdmin)

	// Compliance reporting: any authenticated user can view compliance status
	auth.SetPermission("/kscore.v1.PolicyService/ListViolations", RoleReadonly)
	auth.SetPermission("/kscore.v1.PolicyService/GetComplianceReport", RoleReadonly)

	// Audit log: operators can view policy evaluation history (may be sensitive)
	auth.SetPermission("/kscore.v1.PolicyService/GetAuditLog", RoleOperator)

	// =========================================================================
	// ClusterService - Cluster management (HA deployments)
	// =========================================================================
	// Cluster status: any authenticated user can view cluster health
	auth.SetPermission("/kscore.cluster.v1.ClusterService/GetClusterStatus", RoleReadonly)
	auth.SetPermission("/kscore.cluster.v1.ClusterService/GetStatus", RoleReadonly) // Alias
	auth.SetPermission("/kscore.cluster.v1.ClusterService/ListMembers", RoleReadonly)
	auth.SetPermission("/kscore.cluster.v1.ClusterService/GetMember", RoleReadonly)
	auth.SetPermission("/kscore.cluster.v1.ClusterService/GetLeader", RoleReadonly)

	// Cluster watching: streaming cluster state changes
	auth.SetPermission("/kscore.cluster.v1.ClusterService/WatchMembership", RoleReadonly)
	auth.SetPermission("/kscore.cluster.v1.ClusterService/WatchLeadership", RoleReadonly)

	// Cluster management: admin only (affects cluster availability)
	auth.SetPermission("/kscore.cluster.v1.ClusterService/AddMember", RoleAdmin)
	auth.SetPermission("/kscore.cluster.v1.ClusterService/RemoveMember", RoleAdmin)
	auth.SetPermission("/kscore.cluster.v1.ClusterService/TransferLeader", RoleAdmin)
	auth.SetPermission("/kscore.cluster.v1.ClusterService/Rebalance", RoleAdmin)

	// Backup/restore: admin only (data-level operations)
	auth.SetPermission("/kscore.cluster.v1.ClusterService/CreateBackup", RoleAdmin)
	auth.SetPermission("/kscore.cluster.v1.ClusterService/RestoreBackup", RoleAdmin)

	// =========================================================================
	// CoordinationService - Internal server-to-server (bypass RBAC, mTLS-only)
	// =========================================================================
	// These methods are for control plane instances to coordinate with each other.
	// They require mTLS authentication but bypass user RBAC entirely.
	auth.AddBypassMethod("/kscore.cluster.v1.CoordinationService/ClusterHealth")
	auth.AddBypassMethod("/kscore.cluster.v1.CoordinationService/GetLeader")
	auth.AddBypassMethod("/kscore.cluster.v1.CoordinationService/NATSStatus")
	auth.AddBypassMethod("/kscore.cluster.v1.CoordinationService/RecoveryCoordinate")
	auth.AddBypassMethod("/kscore.cluster.v1.CoordinationService/Heartbeat")
	auth.AddBypassMethod("/kscore.cluster.v1.CoordinationService/PropagateState")

	return auth
}

// SetPermission sets the required role for a method
func (a *RBACAuthorizer) SetPermission(method string, role Role) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.permissions[method] = role
}

// GetPermission returns the required role for a method
func (a *RBACAuthorizer) GetPermission(method string) (Role, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	role, ok := a.permissions[method]
	return role, ok
}

// AddBypassMethod adds a method to the bypass list
func (a *RBACAuthorizer) AddBypassMethod(method string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.bypassMethods[method] = true
}

// IsBypassMethod checks if a method bypasses authorization
func (a *RBACAuthorizer) IsBypassMethod(method string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.bypassMethods[method]
}

// Authorize checks if the principal can perform the given method
func (a *RBACAuthorizer) Authorize(ctx context.Context, principal *Principal, method string) error {
	// Check if method bypasses authorization
	if a.IsBypassMethod(method) {
		return nil
	}

	// If authorization is disabled, allow any authenticated user
	if !a.enabled {
		return nil
	}

	// Principal must exist
	if principal == nil {
		return ErrAuthRequired
	}

	// Get required role for method
	a.mu.RLock()
	requiredRole, ok := a.permissions[method]
	a.mu.RUnlock()

	if !ok {
		if a.defaultDeny {
			// Default deny: require admin for unknown methods (secure by default)
			requiredRole = RoleAdmin
		} else {
			// Default allow: any authenticated user can access unmapped methods
			return nil
		}
	}

	// Check if principal has required role
	if !principal.HasRole(requiredRole) {
		return ErrInsufficientRole
	}

	return nil
}

// ParseMethodName extracts service and method from a full gRPC method name
// e.g., "/kscore.v1.ControlPlaneService/ListAgents" -> ("kscore.v1.ControlPlaneService", "ListAgents")
func ParseMethodName(fullMethod string) (service, method string) {
	// Remove leading slash
	fullMethod = strings.TrimPrefix(fullMethod, "/")

	// Split on last slash
	idx := strings.LastIndex(fullMethod, "/")
	if idx < 0 {
		return fullMethod, ""
	}

	return fullMethod[:idx], fullMethod[idx+1:]
}
