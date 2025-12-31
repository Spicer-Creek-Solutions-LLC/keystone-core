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
}

// NewRBACAuthorizer creates a new RBAC authorizer with default permissions
func NewRBACAuthorizer(bypassMethods []string) *RBACAuthorizer {
	auth := &RBACAuthorizer{
		permissions:   make(map[string]Role),
		bypassMethods: make(map[string]bool),
	}

	// Set up bypass methods
	for _, method := range bypassMethods {
		auth.bypassMethods[method] = true
	}

	// Set default permissions for ControlPlaneService
	// Admin-only operations
	auth.SetPermission("/kscore.v1.ControlPlaneService/ExecuteCommand", RoleOperator)
	auth.SetPermission("/kscore.v1.ControlPlaneService/BatchExecuteCommand", RoleOperator)

	// Operator operations (also admin)
	auth.SetPermission("/kscore.v1.ControlPlaneService/GetAgent", RoleReadonly)
	auth.SetPermission("/kscore.v1.ControlPlaneService/ListAgents", RoleReadonly)
	auth.SetPermission("/kscore.v1.ControlPlaneService/GetCommandStatus", RoleReadonly)
	auth.SetPermission("/kscore.v1.ControlPlaneService/ListCommands", RoleReadonly)
	auth.SetPermission("/kscore.v1.ControlPlaneService/GetBatchJobStatus", RoleReadonly)
	auth.SetPermission("/kscore.v1.ControlPlaneService/ListBatchJobs", RoleReadonly)

	// Cluster operations
	auth.SetPermission("/kscore.cluster.v1.ClusterService/GetStatus", RoleReadonly)
	auth.SetPermission("/kscore.cluster.v1.ClusterService/ListMembers", RoleReadonly)
	auth.SetPermission("/kscore.cluster.v1.ClusterService/GetMember", RoleReadonly)
	auth.SetPermission("/kscore.cluster.v1.ClusterService/GetLeader", RoleReadonly)
	auth.SetPermission("/kscore.cluster.v1.ClusterService/Rebalance", RoleAdmin)
	auth.SetPermission("/kscore.cluster.v1.ClusterService/RemoveMember", RoleAdmin)

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

	// Principal must exist
	if principal == nil {
		return ErrAuthRequired
	}

	// Get required role for method
	a.mu.RLock()
	requiredRole, ok := a.permissions[method]
	a.mu.RUnlock()

	if !ok {
		// Default: require admin for unknown methods (secure by default)
		// This ensures new methods are protected until explicitly configured
		requiredRole = RoleAdmin
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
