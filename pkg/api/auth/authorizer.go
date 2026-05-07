package auth

import (
	"context"
	"errors"
	"fmt"
)

// Authorizer decides whether p may invoke method.
type Authorizer interface {
	Authorize(ctx context.Context, p *Principal, method string) error
}

// ErrUnauthorized indicates the principal lacks the required role
// (or is missing entirely).
var ErrUnauthorized = errors.New("auth: unauthorized")

// RBACAuthorizer maps gRPC fully-qualified methods to required roles
// (admin > operator > readonly). Bypass methods require no auth at
// all. mTLS-required methods reject any AuthMethod other than
// AuthMethodMTLS.
//
// v1.0 ships a hardcoded method map matching the eight services in
// keystone.core.v1. Tests may override individual methods via the
// Set/Add helpers; epic 03 task 6 (versioning registry) and v1.2
// (full Role/Permission CRUD) extend this.
type RBACAuthorizer struct {
	methodRequirements  map[string]Role
	bypassMethods       map[string]bool
	mtlsRequiredMethods map[string]bool

	// defaultRole is applied to methods absent from methodRequirements.
	// v1.0 default-deny: unknown methods require admin.
	defaultRole Role
}

// NewRBACAuthorizer returns the v1.0 default authorizer.
func NewRBACAuthorizer() *RBACAuthorizer {
	return &RBACAuthorizer{
		methodRequirements:  defaultMethodRequirements(),
		bypassMethods:       defaultBypassMethods(),
		mtlsRequiredMethods: defaultMTLSRequiredMethods(),
		defaultRole:         RoleAdmin,
	}
}

// SetMethodRequirement overrides the required role for method.
func (a *RBACAuthorizer) SetMethodRequirement(method string, role Role) {
	if a.methodRequirements == nil {
		a.methodRequirements = map[string]Role{}
	}
	a.methodRequirements[method] = role
}

// AddBypass marks method as no-auth-required.
func (a *RBACAuthorizer) AddBypass(method string) {
	if a.bypassMethods == nil {
		a.bypassMethods = map[string]bool{}
	}
	a.bypassMethods[method] = true
}

// AddMTLSRequirement marks method as mTLS-only — any other AuthMethod
// is rejected even with sufficient role.
func (a *RBACAuthorizer) AddMTLSRequirement(method string) {
	if a.mtlsRequiredMethods == nil {
		a.mtlsRequiredMethods = map[string]bool{}
	}
	a.mtlsRequiredMethods[method] = true
}

// Authorize returns nil if p may invoke method, else a wrapped
// ErrUnauthorized describing the failure.
func (a *RBACAuthorizer) Authorize(_ context.Context, p *Principal, method string) error {
	if a.bypassMethods[method] {
		return nil
	}
	if a.mtlsRequiredMethods[method] {
		if p == nil || p.AuthMethod != AuthMethodMTLS {
			return fmt.Errorf("%w: method %q requires mTLS", ErrUnauthorized, method)
		}
	}
	if p == nil {
		return fmt.Errorf("%w: method %q requires authentication", ErrUnauthorized, method)
	}
	required, ok := a.methodRequirements[method]
	if !ok {
		required = a.defaultRole
	}
	if !p.HasRole(required) {
		return fmt.Errorf("%w: method %q requires %s; principal has %s",
			ErrUnauthorized, method, required, p.Role)
	}
	return nil
}

// defaultMethodRequirements returns the hardcoded v1.0 method→role
// map. Method names are the gRPC fully-qualified form
// /package.Service/Method that grpc-go reports for unary + stream
// dispatches.
//
// Heuristics applied:
//   - List/Get/Status/Watch endpoints: readonly minimum.
//   - Apply/Execute/Emit/Evaluate: operator minimum.
//   - Topology / lifecycle / secret writes: admin.
//   - CoordinationService entries are admin AND in the mTLS-required
//     set; defense in depth even if the server's TLS config is
//     misconfigured to accept non-cert clients.
func defaultMethodRequirements() map[string]Role {
	const (
		agent   = "/keystone.core.v1.AgentService/"
		cp      = "/keystone.core.v1.ControlPlaneService/"
		state   = "/keystone.core.v1.StateService/"
		events  = "/keystone.core.v1.EventService/"
		policy  = "/keystone.core.v1.PolicyService/"
		secrets = "/keystone.core.v1.SecretsService/"
		cluster = "/keystone.core.v1.ClusterService/"
		coord   = "/keystone.core.v1.CoordinationService/"
	)
	return map[string]Role{
		// AgentService: Register / Heartbeat / SubmitCommandStream are
		// in the bypass list (agents authenticate at the transport
		// layer). Only GetAgentInfo reaches the authorizer — operator.
		agent + "GetAgentInfo": RoleOperator,

		// ControlPlaneService: read endpoints readonly, dispatch operator.
		cp + "ServerStatus":        RoleReadonly,
		cp + "ListAgents":          RoleReadonly,
		cp + "GetAgent":            RoleReadonly,
		cp + "GetCommandStatus":    RoleReadonly,
		cp + "ListCommandHistory":  RoleReadonly,
		cp + "ExecuteCommand":      RoleOperator,
		cp + "BatchExecuteCommand": RoleOperator,

		// StateService: read endpoints readonly, apply operator.
		state + "GetStateHistory": RoleReadonly,
		state + "GetStateStatus":  RoleReadonly,
		state + "CheckState":      RoleReadonly,
		state + "DetectDrift":     RoleReadonly,
		state + "ApplyState":      RoleOperator,

		// EventService: queries readonly, emit operator.
		events + "ListEvents":      RoleReadonly,
		events + "GetEvent":        RoleReadonly,
		events + "GetEventTypes":   RoleReadonly,
		events + "GetEventStats":   RoleReadonly,
		events + "SubscribeEvents": RoleReadonly,
		events + "EmitEvent":       RoleOperator,

		// PolicyService: queries readonly, eval operator.
		policy + "ListPolicies":        RoleReadonly,
		policy + "ListPolicySets":      RoleReadonly,
		policy + "ListBindings":        RoleReadonly,
		policy + "ListViolations":      RoleReadonly,
		policy + "GetComplianceReport": RoleReadonly,
		policy + "GetAuditLog":         RoleReadonly,
		policy + "EvaluatePolicy":      RoleOperator,
		policy + "EvaluatePolicySet":   RoleOperator,

		// SecretsService: read & transit operator; mutations admin.
		secrets + "GetSecret":    RoleOperator,
		secrets + "ListSecrets":  RoleOperator,
		secrets + "GetLease":     RoleOperator,
		secrets + "ListLeases":   RoleOperator,
		secrets + "Encrypt":      RoleOperator,
		secrets + "Decrypt":      RoleOperator,
		secrets + "Sign":         RoleOperator,
		secrets + "Verify":       RoleOperator,
		secrets + "WriteSecret":  RoleAdmin,
		secrets + "DeleteSecret": RoleAdmin,
		secrets + "RenewLease":   RoleAdmin,
		secrets + "RevokeLease":  RoleAdmin,

		// ClusterService: read readonly; topology mutations admin.
		cluster + "GetClusterStatus": RoleReadonly,
		cluster + "ListMembers":      RoleReadonly,
		cluster + "GetMember":        RoleReadonly,
		cluster + "GetLeader":        RoleReadonly,
		cluster + "WatchMembership":  RoleReadonly,
		cluster + "WatchLeadership":  RoleReadonly,
		cluster + "AddMember":        RoleAdmin,
		cluster + "RemoveMember":     RoleAdmin,
		cluster + "TransferLeader":   RoleAdmin,
		cluster + "Rebalance":        RoleAdmin,
		cluster + "CreateBackup":     RoleAdmin,
		cluster + "RestoreBackup":    RoleAdmin,

		// CoordinationService: server↔server only; admin + mTLS.
		coord + "ClusterHealth":      RoleAdmin,
		coord + "LookupLeader":       RoleAdmin,
		coord + "NATSStatus":         RoleAdmin,
		coord + "RecoveryCoordinate": RoleAdmin,
		coord + "NodeHeartbeat":      RoleAdmin,
		coord + "PropagateState":     RoleAdmin,
	}
}

// defaultBypassMethods returns the v1.0 set of methods that require no
// API-layer auth. Agents authenticate at the transport layer
// (NATS-mTLS, epic 09) before issuing these calls.
func defaultBypassMethods() map[string]bool {
	const agent = "/keystone.core.v1.AgentService/"
	return map[string]bool{
		agent + "Register":            true,
		agent + "Heartbeat":           true,
		agent + "SubmitCommandStream": true,

		// Standard gRPC health probes + reflection. Production wiring
		// in epic 04 registers the canonical service names here.
		"/grpc.health.v1.Health/Check":                              true,
		"/grpc.health.v1.Health/Watch":                              true,
		"/grpc.reflection.v1.ServerReflection/ServerReflectionInfo": true,
	}
}

// defaultMTLSRequiredMethods returns the v1.0 set of methods that
// MUST be over mTLS — every CoordinationService method per
// PROJECT-DETAILS §4.5. Auth-layer enforcement is defense-in-depth on
// top of the server's TLS config.
func defaultMTLSRequiredMethods() map[string]bool {
	const coord = "/keystone.core.v1.CoordinationService/"
	return map[string]bool{
		coord + "ClusterHealth":      true,
		coord + "LookupLeader":       true,
		coord + "NATSStatus":         true,
		coord + "RecoveryCoordinate": true,
		coord + "NodeHeartbeat":      true,
		coord + "PropagateState":     true,
	}
}
