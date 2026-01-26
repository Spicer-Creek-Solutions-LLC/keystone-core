// Package access provides access control for the file distribution system.
package access

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Permission represents a file operation permission.
type Permission string

const (
	PermissionRead    Permission = "read"
	PermissionWrite   Permission = "write"
	PermissionDelete  Permission = "delete"
	PermissionList    Permission = "list"
	PermissionAdmin   Permission = "admin"
)

// Action represents a file operation for access control.
type Action string

const (
	ActionGet      Action = "get"
	ActionPut      Action = "put"
	ActionDelete   Action = "delete"
	ActionList     Action = "list"
	ActionStat     Action = "stat"
)

// ActionToPermission maps actions to required permissions.
var ActionToPermission = map[Action]Permission{
	ActionGet:    PermissionRead,
	ActionPut:    PermissionWrite,
	ActionDelete: PermissionDelete,
	ActionList:   PermissionList,
	ActionStat:   PermissionRead,
}

// Identity represents an authenticated identity.
type Identity struct {
	// ID is the unique identifier (e.g., SPIFFE ID).
	ID string

	// Type is the identity type (agent, user, service).
	Type string

	// Roles are the roles assigned to this identity.
	Roles []string

	// Attributes are additional identity attributes.
	Attributes map[string]string

	// ValidatedAt is when the identity was validated.
	ValidatedAt time.Time

	// ExpiresAt is when the identity validation expires.
	ExpiresAt time.Time
}

// IsExpired returns true if the identity validation has expired.
func (i *Identity) IsExpired() bool {
	return !i.ExpiresAt.IsZero() && time.Now().After(i.ExpiresAt)
}

// HasRole returns true if the identity has the specified role.
func (i *Identity) HasRole(role string) bool {
	for _, r := range i.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// AccessRequest represents a file access request.
type AccessRequest struct {
	// Identity is the authenticated identity making the request.
	Identity *Identity

	// Namespace is the file namespace (e.g., "packages", "configs").
	Namespace string

	// Path is the file path within the namespace.
	Path string

	// Action is the requested operation.
	Action Action

	// Metadata contains additional request metadata.
	Metadata map[string]string
}

// FullPath returns the full path including namespace.
func (r *AccessRequest) FullPath() string {
	return "/" + r.Namespace + r.Path
}

// AccessResult represents the result of an access check.
type AccessResult struct {
	// Allowed indicates if access is granted.
	Allowed bool

	// Reason provides details about the decision.
	Reason string

	// MatchedRule is the rule that determined the outcome.
	MatchedRule string

	// Duration is how long the check took.
	Duration time.Duration
}

// ACLEntry represents an access control list entry.
type ACLEntry struct {
	// ID is the unique identifier for this entry.
	ID string

	// Priority determines evaluation order (higher = first).
	Priority int

	// IdentityPattern matches identity IDs (supports glob).
	IdentityPattern string

	// IdentityType matches specific identity types (empty = any).
	IdentityType string

	// Roles matches identities with specific roles (empty = any).
	Roles []string

	// NamespacePattern matches namespaces (supports glob).
	NamespacePattern string

	// PathPattern matches paths (supports glob).
	PathPattern string

	// Actions specifies which actions this entry applies to.
	Actions []Action

	// Effect is "allow" or "deny".
	Effect string

	// Description provides human-readable context.
	Description string

	// compiledIdentity is the compiled regex for identity matching.
	compiledIdentity *regexp.Regexp

	// compiledNamespace is the compiled regex for namespace matching.
	compiledNamespace *regexp.Regexp

	// compiledPath is the compiled regex for path matching.
	compiledPath *regexp.Regexp
}

// Compile compiles the patterns into regular expressions.
func (e *ACLEntry) Compile() error {
	var err error

	if e.IdentityPattern != "" {
		pattern := globToRegex(e.IdentityPattern)
		e.compiledIdentity, err = regexp.Compile(pattern)
		if err != nil {
			return fmt.Errorf("invalid identity pattern: %w", err)
		}
	}

	if e.NamespacePattern != "" {
		pattern := globToRegex(e.NamespacePattern)
		e.compiledNamespace, err = regexp.Compile(pattern)
		if err != nil {
			return fmt.Errorf("invalid namespace pattern: %w", err)
		}
	}

	if e.PathPattern != "" {
		pattern := globToRegex(e.PathPattern)
		e.compiledPath, err = regexp.Compile(pattern)
		if err != nil {
			return fmt.Errorf("invalid path pattern: %w", err)
		}
	}

	return nil
}

// Matches returns true if this entry matches the request.
func (e *ACLEntry) Matches(req *AccessRequest) bool {
	// Check identity pattern
	if e.compiledIdentity != nil && !e.compiledIdentity.MatchString(req.Identity.ID) {
		return false
	}

	// Check identity type
	if e.IdentityType != "" && e.IdentityType != req.Identity.Type {
		return false
	}

	// Check roles
	if len(e.Roles) > 0 {
		hasRole := false
		for _, role := range e.Roles {
			if req.Identity.HasRole(role) {
				hasRole = true
				break
			}
		}
		if !hasRole {
			return false
		}
	}

	// Check namespace pattern
	if e.compiledNamespace != nil && !e.compiledNamespace.MatchString(req.Namespace) {
		return false
	}

	// Check path pattern
	if e.compiledPath != nil && !e.compiledPath.MatchString(req.Path) {
		return false
	}

	// Check action
	if len(e.Actions) > 0 {
		hasAction := false
		for _, action := range e.Actions {
			if action == req.Action {
				hasAction = true
				break
			}
		}
		if !hasAction {
			return false
		}
	}

	return true
}

// ACL is an access control list.
type ACL struct {
	entries []*ACLEntry
	mu      sync.RWMutex
}

// NewACL creates a new ACL.
func NewACL() *ACL {
	return &ACL{
		entries: make([]*ACLEntry, 0),
	}
}

// AddEntry adds an entry to the ACL.
func (a *ACL) AddEntry(entry *ACLEntry) error {
	if err := entry.Compile(); err != nil {
		return err
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	a.entries = append(a.entries, entry)
	a.sortEntries()

	return nil
}

// RemoveEntry removes an entry from the ACL by ID.
func (a *ACL) RemoveEntry(id string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	for i, entry := range a.entries {
		if entry.ID == id {
			a.entries = append(a.entries[:i], a.entries[i+1:]...)
			return true
		}
	}

	return false
}

// Evaluate evaluates the ACL for a request.
func (a *ACL) Evaluate(req *AccessRequest) *AccessResult {
	start := time.Now()

	a.mu.RLock()
	defer a.mu.RUnlock()

	for _, entry := range a.entries {
		if entry.Matches(req) {
			return &AccessResult{
				Allowed:     entry.Effect == "allow",
				Reason:      entry.Description,
				MatchedRule: entry.ID,
				Duration:    time.Since(start),
			}
		}
	}

	// Default deny
	return &AccessResult{
		Allowed:     false,
		Reason:      "no matching ACL entry",
		MatchedRule: "",
		Duration:    time.Since(start),
	}
}

// sortEntries sorts entries by priority (descending).
func (a *ACL) sortEntries() {
	// Simple bubble sort for now (typically few entries)
	for i := 0; i < len(a.entries); i++ {
		for j := i + 1; j < len(a.entries); j++ {
			if a.entries[j].Priority > a.entries[i].Priority {
				a.entries[i], a.entries[j] = a.entries[j], a.entries[i]
			}
		}
	}
}

// NamespaceConfig defines access control for a namespace.
type NamespaceConfig struct {
	// Name is the namespace name.
	Name string

	// AllowedRoles are roles that can access this namespace.
	AllowedRoles []string

	// AllowedIdentityTypes are identity types that can access this namespace.
	AllowedIdentityTypes []string

	// RequireAuthentication requires valid identity for access.
	RequireAuthentication bool

	// ReadOnly disables write operations.
	ReadOnly bool

	// MaxFileSize is the maximum file size allowed (0 = unlimited).
	MaxFileSize int64

	// AllowedExtensions restricts file extensions (empty = all).
	AllowedExtensions []string

	// DeniedExtensions blocks specific extensions.
	DeniedExtensions []string
}

// Authorizer performs access control checks.
type Authorizer struct {
	acl              *ACL
	namespaces       map[string]*NamespaceConfig
	policyEvaluator  PolicyEvaluator
	defaultDeny      bool
	mu               sync.RWMutex
}

// PolicyEvaluator is an interface for external policy evaluation (OPA/CEL).
type PolicyEvaluator interface {
	// Evaluate evaluates a policy for the given request.
	Evaluate(ctx context.Context, req *AccessRequest) (*AccessResult, error)
}

// AuthorizerConfig configures the authorizer.
type AuthorizerConfig struct {
	// DefaultDeny denies access by default if no rule matches.
	DefaultDeny bool

	// PolicyEvaluator is an optional external policy evaluator.
	PolicyEvaluator PolicyEvaluator
}

// NewAuthorizer creates a new authorizer.
func NewAuthorizer(config *AuthorizerConfig) *Authorizer {
	if config == nil {
		config = &AuthorizerConfig{DefaultDeny: true}
	}

	return &Authorizer{
		acl:             NewACL(),
		namespaces:      make(map[string]*NamespaceConfig),
		policyEvaluator: config.PolicyEvaluator,
		defaultDeny:     config.DefaultDeny,
	}
}

// AddACLEntry adds an ACL entry.
func (a *Authorizer) AddACLEntry(entry *ACLEntry) error {
	return a.acl.AddEntry(entry)
}

// RemoveACLEntry removes an ACL entry.
func (a *Authorizer) RemoveACLEntry(id string) bool {
	return a.acl.RemoveEntry(id)
}

// SetNamespaceConfig sets the configuration for a namespace.
func (a *Authorizer) SetNamespaceConfig(config *NamespaceConfig) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.namespaces[config.Name] = config
}

// GetNamespaceConfig gets the configuration for a namespace.
func (a *Authorizer) GetNamespaceConfig(name string) *NamespaceConfig {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.namespaces[name]
}

// Authorize checks if a request is authorized.
func (a *Authorizer) Authorize(ctx context.Context, req *AccessRequest) (*AccessResult, error) {
	start := time.Now()

	// Validate identity
	if req.Identity == nil {
		return &AccessResult{
			Allowed:  false,
			Reason:   "no identity provided",
			Duration: time.Since(start),
		}, nil
	}

	if req.Identity.IsExpired() {
		return &AccessResult{
			Allowed:  false,
			Reason:   "identity has expired",
			Duration: time.Since(start),
		}, nil
	}

	// Check namespace config
	a.mu.RLock()
	nsConfig := a.namespaces[req.Namespace]
	a.mu.RUnlock()

	if nsConfig != nil {
		if result := a.checkNamespaceConfig(nsConfig, req); !result.Allowed {
			result.Duration = time.Since(start)
			return result, nil
		}
	}

	// Check ACL
	aclResult := a.acl.Evaluate(req)
	if aclResult.MatchedRule != "" {
		aclResult.Duration = time.Since(start)
		return aclResult, nil
	}

	// Check external policy evaluator
	if a.policyEvaluator != nil {
		result, err := a.policyEvaluator.Evaluate(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("policy evaluation failed: %w", err)
		}
		if result != nil && (result.Allowed || result.Reason != "") {
			result.Duration = time.Since(start)
			return result, nil
		}
	}

	// Default decision
	return &AccessResult{
		Allowed:  !a.defaultDeny,
		Reason:   "default policy",
		Duration: time.Since(start),
	}, nil
}

// checkNamespaceConfig checks namespace-specific access rules.
func (a *Authorizer) checkNamespaceConfig(config *NamespaceConfig, req *AccessRequest) *AccessResult {
	// Check authentication requirement
	if config.RequireAuthentication && req.Identity.ID == "" {
		return &AccessResult{
			Allowed: false,
			Reason:  "namespace requires authentication",
		}
	}

	// Check identity type
	if len(config.AllowedIdentityTypes) > 0 {
		allowed := false
		for _, t := range config.AllowedIdentityTypes {
			if t == req.Identity.Type {
				allowed = true
				break
			}
		}
		if !allowed {
			return &AccessResult{
				Allowed: false,
				Reason:  "identity type not allowed for namespace",
			}
		}
	}

	// Check roles
	if len(config.AllowedRoles) > 0 {
		hasRole := false
		for _, role := range config.AllowedRoles {
			if req.Identity.HasRole(role) {
				hasRole = true
				break
			}
		}
		if !hasRole {
			return &AccessResult{
				Allowed: false,
				Reason:  "no matching role for namespace",
			}
		}
	}

	// Check read-only
	if config.ReadOnly && (req.Action == ActionPut || req.Action == ActionDelete) {
		return &AccessResult{
			Allowed: false,
			Reason:  "namespace is read-only",
		}
	}

	// Check file extension
	if len(config.DeniedExtensions) > 0 || len(config.AllowedExtensions) > 0 {
		ext := getExtension(req.Path)

		// Check denied extensions
		for _, denied := range config.DeniedExtensions {
			if strings.EqualFold(ext, denied) {
				return &AccessResult{
					Allowed: false,
					Reason:  "file extension is denied",
				}
			}
		}

		// Check allowed extensions (if specified)
		if len(config.AllowedExtensions) > 0 {
			allowed := false
			for _, allowedExt := range config.AllowedExtensions {
				if strings.EqualFold(ext, allowedExt) {
					allowed = true
					break
				}
			}
			if !allowed {
				return &AccessResult{
					Allowed: false,
					Reason:  "file extension not allowed",
				}
			}
		}
	}

	return &AccessResult{Allowed: true}
}

// RequestSigner signs file requests.
type RequestSigner struct {
	secret []byte
}

// NewRequestSigner creates a new request signer.
func NewRequestSigner(secret string) *RequestSigner {
	return &RequestSigner{
		secret: []byte(secret),
	}
}

// Sign signs a request and returns the signature.
func (s *RequestSigner) Sign(req *AccessRequest) string {
	data := fmt.Sprintf("%s:%s:%s:%s", req.Identity.ID, req.Namespace, req.Path, req.Action)
	hash := sha256.New()
	hash.Write([]byte(data))
	hash.Write(s.secret)
	return hex.EncodeToString(hash.Sum(nil))
}

// Verify verifies a request signature.
func (s *RequestSigner) Verify(req *AccessRequest, signature string) bool {
	expected := s.Sign(req)
	return signature == expected
}

// Helper functions

// globToRegex converts a glob pattern to a regex pattern.
func globToRegex(glob string) string {
	var result strings.Builder
	result.WriteString("^")

	for i := 0; i < len(glob); i++ {
		c := glob[i]
		switch c {
		case '*':
			if i+1 < len(glob) && glob[i+1] == '*' {
				// ** matches any path including /
				result.WriteString(".*")
				i++
			} else {
				// * matches any character except /
				result.WriteString("[^/]*")
			}
		case '?':
			result.WriteString("[^/]")
		case '.', '+', '^', '$', '(', ')', '[', ']', '{', '}', '|', '\\':
			result.WriteString("\\")
			result.WriteByte(c)
		default:
			result.WriteByte(c)
		}
	}

	result.WriteString("$")
	return result.String()
}

// getExtension extracts the file extension from a path.
func getExtension(path string) string {
	for i := len(path) - 1; i >= 0 && path[i] != '/'; i-- {
		if path[i] == '.' {
			return path[i+1:]
		}
	}
	return ""
}
