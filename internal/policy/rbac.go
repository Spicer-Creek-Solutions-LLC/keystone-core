package policy

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// RBACContextKey is the context key for RBAC context
type RBACContextKey struct{}

// Permission represents an RBAC permission
type Permission struct {
	// Resource the permission applies to
	Resource string `json:"resource"`

	// Action allowed (e.g., read, write, delete, admin)
	Action string `json:"action"`

	// Conditions for the permission (optional)
	Conditions map[string]interface{} `json:"conditions,omitempty"`
}

// Role represents an RBAC role
type Role struct {
	// ID is the unique role identifier
	ID string `json:"id"`

	// Name is the display name
	Name string `json:"name"`

	// Description of the role
	Description string `json:"description,omitempty"`

	// Permissions granted by this role
	Permissions []Permission `json:"permissions"`

	// InheritedRoles are roles this role inherits from
	InheritedRoles []string `json:"inherited_roles,omitempty"`

	// Priority for role conflict resolution (higher = more important)
	Priority int `json:"priority"`

	// Metadata for additional information
	Metadata map[string]string `json:"metadata,omitempty"`

	// CreatedAt timestamp
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt timestamp
	UpdatedAt time.Time `json:"updated_at"`
}

// Principal represents an entity that can have roles
type Principal struct {
	// ID is the unique principal identifier
	ID string `json:"id"`

	// Type of principal (user, service, group)
	Type string `json:"type"`

	// Name is the display name
	Name string `json:"name,omitempty"`

	// Roles assigned to this principal
	Roles []string `json:"roles"`

	// DirectPermissions granted directly (not via roles)
	DirectPermissions []Permission `json:"direct_permissions,omitempty"`

	// Attributes for attribute-based access control
	Attributes map[string]interface{} `json:"attributes,omitempty"`

	// Metadata for additional information
	Metadata map[string]string `json:"metadata,omitempty"`
}

// RBACContext holds RBAC information for policy evaluation
type RBACContext struct {
	// Principal making the request
	Principal *Principal `json:"principal"`

	// EffectiveRoles are all roles (including inherited)
	EffectiveRoles []*Role `json:"effective_roles"`

	// EffectivePermissions are all permissions (from roles and direct)
	EffectivePermissions []Permission `json:"effective_permissions"`

	// SessionAttributes for session-specific ABAC
	SessionAttributes map[string]interface{} `json:"session_attributes,omitempty"`

	// EvaluatedAt when the context was evaluated
	EvaluatedAt time.Time `json:"evaluated_at"`
}

// NewRBACContext creates a new RBAC context
func NewRBACContext(principal *Principal) *RBACContext {
	return &RBACContext{
		Principal:            principal,
		EffectiveRoles:       make([]*Role, 0),
		EffectivePermissions: make([]Permission, 0),
		SessionAttributes:    make(map[string]interface{}),
		EvaluatedAt:          time.Now(),
	}
}

// HasPermission checks if the context grants a specific permission
func (rc *RBACContext) HasPermission(resource, action string) bool {
	for _, perm := range rc.EffectivePermissions {
		if matchesPattern(perm.Resource, resource) && matchesPattern(perm.Action, action) {
			return true
		}
	}
	return false
}

// HasRole checks if the context includes a specific role
func (rc *RBACContext) HasRole(roleID string) bool {
	for _, role := range rc.EffectiveRoles {
		if role.ID == roleID {
			return true
		}
	}
	return false
}

// HasAnyRole checks if the context includes any of the specified roles
func (rc *RBACContext) HasAnyRole(roleIDs ...string) bool {
	for _, roleID := range roleIDs {
		if rc.HasRole(roleID) {
			return true
		}
	}
	return false
}

// HasAllRoles checks if the context includes all specified roles
func (rc *RBACContext) HasAllRoles(roleIDs ...string) bool {
	for _, roleID := range roleIDs {
		if !rc.HasRole(roleID) {
			return false
		}
	}
	return true
}

// GetAttribute gets an attribute from principal or session
func (rc *RBACContext) GetAttribute(key string) (interface{}, bool) {
	// Check session attributes first
	if val, ok := rc.SessionAttributes[key]; ok {
		return val, true
	}

	// Check principal attributes
	if rc.Principal != nil && rc.Principal.Attributes != nil {
		if val, ok := rc.Principal.Attributes[key]; ok {
			return val, true
		}
	}

	return nil, false
}

// GetHighestPriorityRole returns the role with highest priority
func (rc *RBACContext) GetHighestPriorityRole() *Role {
	if len(rc.EffectiveRoles) == 0 {
		return nil
	}

	highest := rc.EffectiveRoles[0]
	for _, role := range rc.EffectiveRoles[1:] {
		if role.Priority > highest.Priority {
			highest = role
		}
	}
	return highest
}

// ToMap converts the RBAC context to a map for policy evaluation
func (rc *RBACContext) ToMap() map[string]interface{} {
	m := make(map[string]interface{})

	if rc.Principal != nil {
		m["principal_id"] = rc.Principal.ID
		m["principal_type"] = rc.Principal.Type
		m["principal_name"] = rc.Principal.Name
		m["principal_roles"] = rc.Principal.Roles
		m["principal_attributes"] = rc.Principal.Attributes
	}

	// Add role IDs
	roleIDs := make([]string, len(rc.EffectiveRoles))
	for i, role := range rc.EffectiveRoles {
		roleIDs[i] = role.ID
	}
	m["roles"] = roleIDs

	// Add permissions as map
	permissions := make([]map[string]interface{}, len(rc.EffectivePermissions))
	for i, perm := range rc.EffectivePermissions {
		permissions[i] = map[string]interface{}{
			"resource":   perm.Resource,
			"action":     perm.Action,
			"conditions": perm.Conditions,
		}
	}
	m["permissions"] = permissions

	// Add session attributes
	m["session"] = rc.SessionAttributes

	return m
}

// matchesPattern checks if a value matches a pattern (supports * wildcard)
func matchesPattern(pattern, value string) bool {
	if pattern == "*" {
		return true
	}
	if pattern == value {
		return true
	}

	// Support prefix wildcards (e.g., "agents:*")
	if strings.HasSuffix(pattern, ":*") {
		prefix := strings.TrimSuffix(pattern, ":*")
		return strings.HasPrefix(value, prefix+":")
	}

	// Support suffix wildcards (e.g., "*:read")
	if strings.HasPrefix(pattern, "*:") {
		suffix := strings.TrimPrefix(pattern, "*:")
		return strings.HasSuffix(value, ":"+suffix)
	}

	return false
}

// RBACStore manages roles and principals
type RBACStore struct {
	roles      map[string]*Role
	principals map[string]*Principal
	mu         sync.RWMutex
}

// NewRBACStore creates a new RBAC store
func NewRBACStore() *RBACStore {
	return &RBACStore{
		roles:      make(map[string]*Role),
		principals: make(map[string]*Principal),
	}
}

// RegisterRole registers a role
func (s *RBACStore) RegisterRole(role *Role) error {
	if role.ID == "" {
		return fmt.Errorf("role ID is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if role.CreatedAt.IsZero() {
		role.CreatedAt = time.Now()
	}
	role.UpdatedAt = time.Now()

	s.roles[role.ID] = role
	return nil
}

// GetRole gets a role by ID
func (s *RBACStore) GetRole(id string) (*Role, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	role, ok := s.roles[id]
	return role, ok
}

// DeleteRole deletes a role
func (s *RBACStore) DeleteRole(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.roles, id)
}

// ListRoles lists all roles
func (s *RBACStore) ListRoles() []*Role {
	s.mu.RLock()
	defer s.mu.RUnlock()

	roles := make([]*Role, 0, len(s.roles))
	for _, role := range s.roles {
		roles = append(roles, role)
	}

	// Sort by priority (descending) then name
	sort.Slice(roles, func(i, j int) bool {
		if roles[i].Priority != roles[j].Priority {
			return roles[i].Priority > roles[j].Priority
		}
		return roles[i].Name < roles[j].Name
	})

	return roles
}

// RegisterPrincipal registers a principal
func (s *RBACStore) RegisterPrincipal(principal *Principal) error {
	if principal.ID == "" {
		return fmt.Errorf("principal ID is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.principals[principal.ID] = principal
	return nil
}

// GetPrincipal gets a principal by ID
func (s *RBACStore) GetPrincipal(id string) (*Principal, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	principal, ok := s.principals[id]
	return principal, ok
}

// DeletePrincipal deletes a principal
func (s *RBACStore) DeletePrincipal(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.principals, id)
}

// ListPrincipals lists all principals
func (s *RBACStore) ListPrincipals() []*Principal {
	s.mu.RLock()
	defer s.mu.RUnlock()

	principals := make([]*Principal, 0, len(s.principals))
	for _, principal := range s.principals {
		principals = append(principals, principal)
	}

	return principals
}

// BuildRBACContext builds a complete RBAC context for a principal
func (s *RBACStore) BuildRBACContext(principalID string) (*RBACContext, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	principal, ok := s.principals[principalID]
	if !ok {
		return nil, fmt.Errorf("principal not found: %s", principalID)
	}

	ctx := NewRBACContext(principal)

	// Resolve all effective roles (including inherited)
	resolvedRoles := make(map[string]*Role)
	for _, roleID := range principal.Roles {
		s.resolveRoleHierarchy(roleID, resolvedRoles)
	}

	for _, role := range resolvedRoles {
		ctx.EffectiveRoles = append(ctx.EffectiveRoles, role)
	}

	// Sort roles by priority
	sort.Slice(ctx.EffectiveRoles, func(i, j int) bool {
		return ctx.EffectiveRoles[i].Priority > ctx.EffectiveRoles[j].Priority
	})

	// Collect all permissions
	permSet := make(map[string]Permission)

	// Add direct permissions
	for _, perm := range principal.DirectPermissions {
		key := perm.Resource + ":" + perm.Action
		permSet[key] = perm
	}

	// Add role permissions
	for _, role := range ctx.EffectiveRoles {
		for _, perm := range role.Permissions {
			key := perm.Resource + ":" + perm.Action
			if _, exists := permSet[key]; !exists {
				permSet[key] = perm
			}
		}
	}

	for _, perm := range permSet {
		ctx.EffectivePermissions = append(ctx.EffectivePermissions, perm)
	}

	return ctx, nil
}

// resolveRoleHierarchy recursively resolves role inheritance
func (s *RBACStore) resolveRoleHierarchy(roleID string, resolved map[string]*Role) {
	if _, exists := resolved[roleID]; exists {
		return // Already resolved (avoid cycles)
	}

	role, ok := s.roles[roleID]
	if !ok {
		return
	}

	resolved[roleID] = role

	// Resolve inherited roles
	for _, inheritedID := range role.InheritedRoles {
		s.resolveRoleHierarchy(inheritedID, resolved)
	}
}

// AssignRole assigns a role to a principal
func (s *RBACStore) AssignRole(principalID, roleID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	principal, ok := s.principals[principalID]
	if !ok {
		return fmt.Errorf("principal not found: %s", principalID)
	}

	if _, ok := s.roles[roleID]; !ok {
		return fmt.Errorf("role not found: %s", roleID)
	}

	// Check if already assigned
	for _, r := range principal.Roles {
		if r == roleID {
			return nil // Already assigned
		}
	}

	principal.Roles = append(principal.Roles, roleID)
	return nil
}

// RemoveRole removes a role from a principal
func (s *RBACStore) RemoveRole(principalID, roleID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	principal, ok := s.principals[principalID]
	if !ok {
		return fmt.Errorf("principal not found: %s", principalID)
	}

	newRoles := make([]string, 0, len(principal.Roles))
	for _, r := range principal.Roles {
		if r != roleID {
			newRoles = append(newRoles, r)
		}
	}
	principal.Roles = newRoles
	return nil
}

// GrantPermission grants a direct permission to a principal
func (s *RBACStore) GrantPermission(principalID string, perm Permission) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	principal, ok := s.principals[principalID]
	if !ok {
		return fmt.Errorf("principal not found: %s", principalID)
	}

	principal.DirectPermissions = append(principal.DirectPermissions, perm)
	return nil
}

// RevokePermission revokes a direct permission from a principal
func (s *RBACStore) RevokePermission(principalID string, resource, action string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	principal, ok := s.principals[principalID]
	if !ok {
		return fmt.Errorf("principal not found: %s", principalID)
	}

	newPerms := make([]Permission, 0, len(principal.DirectPermissions))
	for _, perm := range principal.DirectPermissions {
		if perm.Resource != resource || perm.Action != action {
			newPerms = append(newPerms, perm)
		}
	}
	principal.DirectPermissions = newPerms
	return nil
}

// WithRBACContext adds RBAC context to a context.Context
func WithRBACContext(ctx context.Context, rbac *RBACContext) context.Context {
	return context.WithValue(ctx, RBACContextKey{}, rbac)
}

// RBACContextFromContext extracts RBAC context from context.Context
func RBACContextFromContext(ctx context.Context) *RBACContext {
	val := ctx.Value(RBACContextKey{})
	if val == nil {
		return nil
	}
	return val.(*RBACContext)
}

// RBACEvaluationInput extends EvaluationInput with RBAC context
type RBACEvaluationInput struct {
	*EvaluationInput

	// RBACContext provides RBAC information
	RBACContext *RBACContext `json:"rbac_context,omitempty"`
}

// NewRBACEvaluationInput creates an RBAC-aware evaluation input
func NewRBACEvaluationInput(input *EvaluationInput, rbac *RBACContext) *RBACEvaluationInput {
	return &RBACEvaluationInput{
		EvaluationInput: input,
		RBACContext:     rbac,
	}
}

// ToEvaluationInput converts to a standard EvaluationInput with RBAC in context
func (ri *RBACEvaluationInput) ToEvaluationInput() *EvaluationInput {
	input := ri.EvaluationInput
	if input.Context == nil {
		input.Context = make(map[string]interface{})
	}

	if ri.RBACContext != nil {
		input.Context["rbac"] = ri.RBACContext.ToMap()
	}

	return input
}

// RBACEnrichedEngine wraps PolicyEngine with RBAC context support
type RBACEnrichedEngine struct {
	engine    *PolicyEngine
	rbacStore *RBACStore
}

// NewRBACEnrichedEngine creates an RBAC-enriched policy engine
func NewRBACEnrichedEngine(engine *PolicyEngine, rbacStore *RBACStore) *RBACEnrichedEngine {
	return &RBACEnrichedEngine{
		engine:    engine,
		rbacStore: rbacStore,
	}
}

// EvaluateWithRBAC evaluates a policy with automatic RBAC context enrichment
func (e *RBACEnrichedEngine) EvaluateWithRBAC(ctx context.Context, policyID, principalID string, input *EvaluationInput) (*EvaluationResult, error) {
	// Build RBAC context
	rbacCtx, err := e.rbacStore.BuildRBACContext(principalID)
	if err != nil {
		return nil, fmt.Errorf("failed to build RBAC context: %w", err)
	}

	// Enrich input with RBAC context
	rbacInput := NewRBACEvaluationInput(input, rbacCtx)
	enrichedInput := rbacInput.ToEvaluationInput()

	// Add RBAC context to context.Context for tracing
	ctx = WithRBACContext(ctx, rbacCtx)

	return e.engine.Evaluate(ctx, policyID, enrichedInput)
}

// CheckPermission is a convenience method to check a simple permission
func (e *RBACEnrichedEngine) CheckPermission(principalID, resource, action string) (bool, error) {
	rbacCtx, err := e.rbacStore.BuildRBACContext(principalID)
	if err != nil {
		return false, err
	}
	return rbacCtx.HasPermission(resource, action), nil
}

// GetPrincipalRoles returns all effective roles for a principal
func (e *RBACEnrichedEngine) GetPrincipalRoles(principalID string) ([]*Role, error) {
	rbacCtx, err := e.rbacStore.BuildRBACContext(principalID)
	if err != nil {
		return nil, err
	}
	return rbacCtx.EffectiveRoles, nil
}

// GetPrincipalPermissions returns all effective permissions for a principal
func (e *RBACEnrichedEngine) GetPrincipalPermissions(principalID string) ([]Permission, error) {
	rbacCtx, err := e.rbacStore.BuildRBACContext(principalID)
	if err != nil {
		return nil, err
	}
	return rbacCtx.EffectivePermissions, nil
}

// StandardRoles provides common predefined roles
var StandardRoles = struct {
	Admin    string
	Operator string
	Viewer   string
	Service  string
}{
	Admin:    "admin",
	Operator: "operator",
	Viewer:   "viewer",
	Service:  "service",
}

// CreateStandardRoles creates and registers standard roles
func CreateStandardRoles(store *RBACStore) error {
	roles := []*Role{
		{
			ID:          StandardRoles.Admin,
			Name:        "Administrator",
			Description: "Full access to all resources",
			Permissions: []Permission{
				{Resource: "*", Action: "*"},
			},
			Priority: 100,
		},
		{
			ID:          StandardRoles.Operator,
			Name:        "Operator",
			Description: "Can manage agents and execute commands",
			Permissions: []Permission{
				{Resource: "agents", Action: "*"},
				{Resource: "commands", Action: "*"},
				{Resource: "states", Action: "read"},
				{Resource: "states", Action: "apply"},
				{Resource: "events", Action: "read"},
			},
			Priority: 50,
		},
		{
			ID:          StandardRoles.Viewer,
			Name:        "Viewer",
			Description: "Read-only access",
			Permissions: []Permission{
				{Resource: "agents", Action: "read"},
				{Resource: "commands", Action: "read"},
				{Resource: "states", Action: "read"},
				{Resource: "events", Action: "read"},
				{Resource: "policies", Action: "read"},
			},
			Priority: 10,
		},
		{
			ID:          StandardRoles.Service,
			Name:        "Service Account",
			Description: "For automated services",
			Permissions: []Permission{
				{Resource: "agents", Action: "register"},
				{Resource: "agents", Action: "heartbeat"},
				{Resource: "commands", Action: "execute"},
				{Resource: "events", Action: "publish"},
			},
			Priority: 30,
		},
	}

	for _, role := range roles {
		if err := store.RegisterRole(role); err != nil {
			return fmt.Errorf("failed to register role %s: %w", role.ID, err)
		}
	}

	return nil
}
