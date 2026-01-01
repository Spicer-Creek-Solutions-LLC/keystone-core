// Package nats provides NATS messaging infrastructure for Keystone Core.
package nats

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nkeys"
)

// Bootstrap credential errors
var (
	ErrBootstrapExpired       = errors.New("bootstrap credential has expired")
	ErrBootstrapRevoked       = errors.New("bootstrap credential has been revoked")
	ErrBootstrapInvalid       = errors.New("bootstrap credential is invalid")
	ErrBootstrapNotFound      = errors.New("bootstrap credential not found")
	ErrBootstrapAlreadyUsed   = errors.New("bootstrap credential has already been used")
	ErrBootstrapClusterMismatch = errors.New("bootstrap credential cluster mismatch")
)

// DefaultBootstrapTTL is the default time-to-live for bootstrap credentials
const DefaultBootstrapTTL = 5 * time.Minute

// MaxBootstrapTTL is the maximum allowed TTL for bootstrap credentials
const MaxBootstrapTTL = 24 * time.Hour

// BootstrapCredentialType represents the type of bootstrap credential
type BootstrapCredentialType string

const (
	// BootstrapCredentialTypeNKey uses NATS NKeys for authentication
	BootstrapCredentialTypeNKey BootstrapCredentialType = "nkey"
	// BootstrapCredentialTypeJWT uses NATS JWT for authentication
	BootstrapCredentialTypeJWT BootstrapCredentialType = "jwt"
	// BootstrapCredentialTypeToken uses a simple token for authentication
	BootstrapCredentialTypeToken BootstrapCredentialType = "token"
)

// BootstrapClaims contains the claims for a bootstrap credential.
// These are the minimal claims needed for initial agent registration.
type BootstrapClaims struct {
	// ID is the unique identifier for this bootstrap credential
	ID string `json:"id"`
	// Cluster is the cluster this credential is valid for
	Cluster string `json:"cluster"`
	// IssuedAt is when the credential was issued
	IssuedAt time.Time `json:"iat"`
	// ExpiresAt is when the credential expires
	ExpiresAt time.Time `json:"exp"`
	// AllowedAgentID is the expected agent ID (if pre-assigned)
	AllowedAgentID string `json:"agent_id,omitempty"`
	// AllowedLabels are labels that must match the registering agent
	AllowedLabels map[string]string `json:"labels,omitempty"`
	// MaxUses is the maximum number of times this credential can be used (0 = unlimited)
	MaxUses int `json:"max_uses,omitempty"`
	// Issuer identifies who issued this credential
	Issuer string `json:"iss,omitempty"`
	// Metadata contains additional metadata about the credential
	Metadata map[string]string `json:"metadata,omitempty"`
}

// IsExpired returns true if the credential has expired
func (c *BootstrapClaims) IsExpired() bool {
	return time.Now().After(c.ExpiresAt)
}

// TTL returns the remaining time-to-live
func (c *BootstrapClaims) TTL() time.Duration {
	return time.Until(c.ExpiresAt)
}

// BootstrapCredential represents a complete bootstrap credential
// that an agent uses for initial registration.
type BootstrapCredential struct {
	// Type is the credential type (nkey, jwt, token)
	Type BootstrapCredentialType `json:"type"`
	// Claims contains the credential claims
	Claims BootstrapClaims `json:"claims"`
	// PublicKey is the NKey public key (for nkey type)
	PublicKey string `json:"public_key,omitempty"`
	// PrivateKey is the NKey private key seed (for nkey type) - only returned to agent
	PrivateKey string `json:"private_key,omitempty"`
	// Token is the authentication token (for token type)
	Token string `json:"token,omitempty"`
	// JWT is the signed JWT (for jwt type)
	JWT string `json:"jwt,omitempty"`
	// NATSSubjects contains the subjects this credential can access
	NATSSubjects SubjectPermissions `json:"nats_subjects"`
}

// BootstrapCredentialRequest is a request to generate a new bootstrap credential
type BootstrapCredentialRequest struct {
	// Cluster is the cluster this credential should be valid for
	Cluster string `json:"cluster"`
	// TTL is the time-to-live for the credential (default: 5 minutes)
	TTL time.Duration `json:"ttl,omitempty"`
	// AllowedAgentID is the expected agent ID (optional)
	AllowedAgentID string `json:"agent_id,omitempty"`
	// AllowedLabels are labels that must match the registering agent (optional)
	AllowedLabels map[string]string `json:"labels,omitempty"`
	// MaxUses is the maximum number of times this credential can be used (0 = unlimited)
	MaxUses int `json:"max_uses,omitempty"`
	// Type is the credential type to generate (default: nkey)
	Type BootstrapCredentialType `json:"type,omitempty"`
	// Metadata contains additional metadata to include in the credential
	Metadata map[string]string `json:"metadata,omitempty"`
}

// BootstrapCredentialStatus tracks the status of a bootstrap credential
type BootstrapCredentialStatus struct {
	// ID is the credential ID
	ID string `json:"id"`
	// Claims are the credential claims
	Claims BootstrapClaims `json:"claims"`
	// UseCount is the number of times this credential has been used
	UseCount int `json:"use_count"`
	// Revoked indicates if the credential has been revoked
	Revoked bool `json:"revoked"`
	// RevokedAt is when the credential was revoked (if applicable)
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
	// RevokedReason is the reason for revocation
	RevokedReason string `json:"revoked_reason,omitempty"`
	// LastUsedAt is when the credential was last used
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	// UsedByAgents are the agent IDs that have used this credential
	UsedByAgents []string `json:"used_by_agents,omitempty"`
}

// BootstrapValidationResult contains the result of validating a bootstrap credential
type BootstrapValidationResult struct {
	// Valid indicates if the credential is valid
	Valid bool `json:"valid"`
	// Claims contains the credential claims (if valid)
	Claims *BootstrapClaims `json:"claims,omitempty"`
	// Error is the validation error (if invalid)
	Error error `json:"error,omitempty"`
	// Status is the credential status
	Status *BootstrapCredentialStatus `json:"status,omitempty"`
}

// BootstrapCredentialProvider generates and validates bootstrap credentials.
// This interface allows for different credential backends (in-memory, database, Vault, etc.)
type BootstrapCredentialProvider interface {
	// Generate creates a new bootstrap credential
	Generate(ctx context.Context, req BootstrapCredentialRequest) (*BootstrapCredential, error)
	// Validate validates a bootstrap credential
	Validate(ctx context.Context, credential *BootstrapCredential) (*BootstrapValidationResult, error)
	// Revoke revokes a bootstrap credential
	Revoke(ctx context.Context, credentialID string, reason string) error
	// RecordUse records that a credential was used by an agent
	RecordUse(ctx context.Context, credentialID string, agentID string) error
	// GetStatus returns the status of a bootstrap credential
	GetStatus(ctx context.Context, credentialID string) (*BootstrapCredentialStatus, error)
	// ListActive returns all active (non-expired, non-revoked) bootstrap credentials
	ListActive(ctx context.Context) ([]*BootstrapCredentialStatus, error)
	// Cleanup removes expired credentials
	Cleanup(ctx context.Context) (int, error)
}

// IdentityVerifier verifies agent identity during bootstrap registration.
// This interface allows for extensible identity verification (SPIFFE/SPIRE, cloud metadata, etc.)
type IdentityVerifier interface {
	// Verify verifies the identity claims provided by the agent
	Verify(ctx context.Context, claims map[string]interface{}) (*IdentityVerificationResult, error)
	// Type returns the verifier type (e.g., "spiffe", "cloud-metadata", "attestation")
	Type() string
}

// IdentityVerificationResult contains the result of identity verification
type IdentityVerificationResult struct {
	// Verified indicates if the identity was verified
	Verified bool `json:"verified"`
	// Identity is the verified identity (e.g., SPIFFE ID)
	Identity string `json:"identity,omitempty"`
	// TrustLevel indicates the level of trust in the verification
	TrustLevel string `json:"trust_level,omitempty"`
	// Attributes are additional verified attributes
	Attributes map[string]string `json:"attributes,omitempty"`
	// Error is the verification error (if not verified)
	Error error `json:"error,omitempty"`
}

// CredentialIssuer issues permanent credentials after successful bootstrap registration.
// This interface allows for different credential issuance backends.
type CredentialIssuer interface {
	// Issue issues permanent credentials for a registered agent
	Issue(ctx context.Context, req CredentialIssueRequest) (*IssuedCredentials, error)
	// Revoke revokes permanent credentials for an agent
	Revoke(ctx context.Context, agentID string, reason string) error
	// Rotate rotates credentials for an agent
	Rotate(ctx context.Context, agentID string) (*IssuedCredentials, error)
}

// CredentialIssueRequest is a request to issue permanent credentials
type CredentialIssueRequest struct {
	// AgentID is the agent ID to issue credentials for
	AgentID string `json:"agent_id"`
	// Cluster is the cluster the agent belongs to
	Cluster string `json:"cluster"`
	// Labels are the agent's labels
	Labels map[string]string `json:"labels,omitempty"`
	// BootstrapID is the ID of the bootstrap credential used
	BootstrapID string `json:"bootstrap_id,omitempty"`
	// VerifiedIdentity is the verified identity from IdentityVerifier
	VerifiedIdentity *IdentityVerificationResult `json:"verified_identity,omitempty"`
}

// IssuedCredentials contains the permanent credentials issued to an agent
type IssuedCredentials struct {
	// AgentID is the agent ID
	AgentID string `json:"agent_id"`
	// NKeyPublicKey is the agent's NKey public key
	NKeyPublicKey string `json:"nkey_public_key,omitempty"`
	// NKeyPrivateKey is the agent's NKey private key seed
	NKeyPrivateKey string `json:"nkey_private_key,omitempty"`
	// JWT is the agent's signed JWT credential
	JWT string `json:"jwt,omitempty"`
	// Token is the agent's authentication token
	Token string `json:"token,omitempty"`
	// Subjects contains the subjects the agent can access
	Subjects SubjectPermissions `json:"subjects"`
	// ExpiresAt is when the credentials expire (if applicable)
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// InMemoryBootstrapProvider is an in-memory implementation of BootstrapCredentialProvider.
// This is suitable for development/testing and small deployments.
// For production, use a database-backed implementation.
type InMemoryBootstrapProvider struct {
	issuer string
	mu     sync.RWMutex
	// credentials maps credential ID to status
	credentials map[string]*BootstrapCredentialStatus
	// publicKeys maps public key to credential ID (for nkey lookup)
	publicKeys map[string]string
	// tokens maps token to credential ID (for token lookup)
	tokens map[string]string
}

// NewInMemoryBootstrapProvider creates a new in-memory bootstrap credential provider
func NewInMemoryBootstrapProvider(issuer string) *InMemoryBootstrapProvider {
	return &InMemoryBootstrapProvider{
		issuer:      issuer,
		credentials: make(map[string]*BootstrapCredentialStatus),
		publicKeys:  make(map[string]string),
		tokens:      make(map[string]string),
	}
}

// Generate creates a new bootstrap credential
func (p *InMemoryBootstrapProvider) Generate(ctx context.Context, req BootstrapCredentialRequest) (*BootstrapCredential, error) {
	// Validate request
	if req.Cluster == "" {
		req.Cluster = DefaultCluster
	}

	// Set defaults
	if req.TTL <= 0 {
		req.TTL = DefaultBootstrapTTL
	}
	if req.TTL > MaxBootstrapTTL {
		req.TTL = MaxBootstrapTTL
	}
	if req.Type == "" {
		req.Type = BootstrapCredentialTypeNKey
	}

	// Generate credential ID
	credentialID, err := generateRandomID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate credential ID: %w", err)
	}

	// Create claims
	now := time.Now()
	claims := BootstrapClaims{
		ID:             credentialID,
		Cluster:        req.Cluster,
		IssuedAt:       now,
		ExpiresAt:      now.Add(req.TTL),
		AllowedAgentID: req.AllowedAgentID,
		AllowedLabels:  req.AllowedLabels,
		MaxUses:        req.MaxUses,
		Issuer:         p.issuer,
		Metadata:       req.Metadata,
	}

	// Create credential based on type
	cred := &BootstrapCredential{
		Type:   req.Type,
		Claims: claims,
	}

	// Generate subject permissions
	// Bootstrap credentials have minimal permissions - only registration
	cred.NATSSubjects = BootstrapPermissions(req.Cluster, credentialID)

	// Generate type-specific credential data
	switch req.Type {
	case BootstrapCredentialTypeNKey:
		kp, err := nkeys.CreateUser()
		if err != nil {
			return nil, fmt.Errorf("failed to create NKey: %w", err)
		}
		publicKey, err := kp.PublicKey()
		if err != nil {
			return nil, fmt.Errorf("failed to get public key: %w", err)
		}
		seed, err := kp.Seed()
		if err != nil {
			return nil, fmt.Errorf("failed to get private key seed: %w", err)
		}
		cred.PublicKey = publicKey
		cred.PrivateKey = string(seed)

		// Store public key mapping
		p.mu.Lock()
		p.publicKeys[publicKey] = credentialID
		p.mu.Unlock()

	case BootstrapCredentialTypeToken:
		token, err := generateRandomToken()
		if err != nil {
			return nil, fmt.Errorf("failed to generate token: %w", err)
		}
		cred.Token = token

		// Store token mapping
		p.mu.Lock()
		p.tokens[token] = credentialID
		p.mu.Unlock()

	case BootstrapCredentialTypeJWT:
		// JWT generation would require additional JWT signing infrastructure
		// For now, we use a simple encoded claims structure
		claimsJSON, err := json.Marshal(claims)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal claims: %w", err)
		}
		cred.JWT = base64.StdEncoding.EncodeToString(claimsJSON)
	}

	// Store credential status
	status := &BootstrapCredentialStatus{
		ID:           credentialID,
		Claims:       claims,
		UseCount:     0,
		Revoked:      false,
		UsedByAgents: []string{},
	}

	p.mu.Lock()
	p.credentials[credentialID] = status
	p.mu.Unlock()

	return cred, nil
}

// Validate validates a bootstrap credential
func (p *InMemoryBootstrapProvider) Validate(ctx context.Context, credential *BootstrapCredential) (*BootstrapValidationResult, error) {
	if credential == nil {
		return &BootstrapValidationResult{
			Valid: false,
			Error: ErrBootstrapInvalid,
		}, nil
	}

	// Look up credential ID
	credentialID := credential.Claims.ID
	if credentialID == "" {
		// Try to find by public key or token
		p.mu.RLock()
		switch credential.Type {
		case BootstrapCredentialTypeNKey:
			if id, ok := p.publicKeys[credential.PublicKey]; ok {
				credentialID = id
			}
		case BootstrapCredentialTypeToken:
			if id, ok := p.tokens[credential.Token]; ok {
				credentialID = id
			}
		}
		p.mu.RUnlock()
	}

	if credentialID == "" {
		return &BootstrapValidationResult{
			Valid: false,
			Error: ErrBootstrapNotFound,
		}, nil
	}

	// Get credential status
	p.mu.RLock()
	status, exists := p.credentials[credentialID]
	p.mu.RUnlock()

	if !exists {
		return &BootstrapValidationResult{
			Valid: false,
			Error: ErrBootstrapNotFound,
		}, nil
	}

	// Check if revoked
	if status.Revoked {
		return &BootstrapValidationResult{
			Valid:  false,
			Error:  ErrBootstrapRevoked,
			Status: status,
			Claims: &status.Claims,
		}, nil
	}

	// Check if expired
	if status.Claims.IsExpired() {
		return &BootstrapValidationResult{
			Valid:  false,
			Error:  ErrBootstrapExpired,
			Status: status,
			Claims: &status.Claims,
		}, nil
	}

	// Check max uses
	if status.Claims.MaxUses > 0 && status.UseCount >= status.Claims.MaxUses {
		return &BootstrapValidationResult{
			Valid:  false,
			Error:  ErrBootstrapAlreadyUsed,
			Status: status,
			Claims: &status.Claims,
		}, nil
	}

	return &BootstrapValidationResult{
		Valid:  true,
		Status: status,
		Claims: &status.Claims,
	}, nil
}

// Revoke revokes a bootstrap credential
func (p *InMemoryBootstrapProvider) Revoke(ctx context.Context, credentialID string, reason string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	status, exists := p.credentials[credentialID]
	if !exists {
		return ErrBootstrapNotFound
	}

	now := time.Now()
	status.Revoked = true
	status.RevokedAt = &now
	status.RevokedReason = reason

	return nil
}

// RecordUse records that a credential was used by an agent
func (p *InMemoryBootstrapProvider) RecordUse(ctx context.Context, credentialID string, agentID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	status, exists := p.credentials[credentialID]
	if !exists {
		return ErrBootstrapNotFound
	}

	now := time.Now()
	status.UseCount++
	status.LastUsedAt = &now
	status.UsedByAgents = append(status.UsedByAgents, agentID)

	return nil
}

// GetStatus returns the status of a bootstrap credential
func (p *InMemoryBootstrapProvider) GetStatus(ctx context.Context, credentialID string) (*BootstrapCredentialStatus, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	status, exists := p.credentials[credentialID]
	if !exists {
		return nil, ErrBootstrapNotFound
	}

	// Return a copy to prevent mutation
	statusCopy := *status
	statusCopy.UsedByAgents = make([]string, len(status.UsedByAgents))
	copy(statusCopy.UsedByAgents, status.UsedByAgents)

	return &statusCopy, nil
}

// ListActive returns all active (non-expired, non-revoked) bootstrap credentials
func (p *InMemoryBootstrapProvider) ListActive(ctx context.Context) ([]*BootstrapCredentialStatus, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var active []*BootstrapCredentialStatus
	now := time.Now()

	for _, status := range p.credentials {
		if !status.Revoked && status.Claims.ExpiresAt.After(now) {
			// Return a copy
			statusCopy := *status
			statusCopy.UsedByAgents = make([]string, len(status.UsedByAgents))
			copy(statusCopy.UsedByAgents, status.UsedByAgents)
			active = append(active, &statusCopy)
		}
	}

	return active, nil
}

// Cleanup removes expired credentials
func (p *InMemoryBootstrapProvider) Cleanup(ctx context.Context) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	removed := 0

	for id, status := range p.credentials {
		if status.Claims.ExpiresAt.Before(now) {
			delete(p.credentials, id)
			removed++
		}
	}

	// Also clean up public key and token mappings
	for pubKey, id := range p.publicKeys {
		if _, exists := p.credentials[id]; !exists {
			delete(p.publicKeys, pubKey)
		}
	}
	for token, id := range p.tokens {
		if _, exists := p.credentials[id]; !exists {
			delete(p.tokens, token)
		}
	}

	return removed, nil
}

// Helper functions

// generateRandomID generates a random credential ID
func generateRandomID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("bootstrap-%x", b), nil
}

// generateRandomToken generates a random authentication token
func generateRandomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// BootstrapRegistrationRequest represents an agent's request to register using bootstrap credentials
type BootstrapRegistrationRequest struct {
	// BootstrapCredential is the bootstrap credential being used
	BootstrapCredential *BootstrapCredential `json:"bootstrap_credential"`
	// AgentID is the agent's desired ID
	AgentID string `json:"agent_id"`
	// Labels are the agent's labels
	Labels map[string]string `json:"labels,omitempty"`
	// Metadata is additional agent metadata
	Metadata map[string]string `json:"metadata,omitempty"`
	// IdentityClaims are claims for identity verification (e.g., SPIFFE SVID)
	IdentityClaims map[string]interface{} `json:"identity_claims,omitempty"`
}

// BootstrapRegistrationResponse is the response to a bootstrap registration request
type BootstrapRegistrationResponse struct {
	// Success indicates if registration was successful
	Success bool `json:"success"`
	// AgentID is the assigned agent ID
	AgentID string `json:"agent_id,omitempty"`
	// Credentials are the permanent credentials issued to the agent
	Credentials *IssuedCredentials `json:"credentials,omitempty"`
	// Error is the error message if registration failed
	Error string `json:"error,omitempty"`
	// ErrorCode is a machine-readable error code
	ErrorCode string `json:"error_code,omitempty"`
}

// BootstrapAuditEvent represents an audit event for bootstrap operations
type BootstrapAuditEvent struct {
	// Timestamp is when the event occurred
	Timestamp time.Time `json:"timestamp"`
	// EventType is the type of event (generate, validate, revoke, use, etc.)
	EventType string `json:"event_type"`
	// CredentialID is the ID of the bootstrap credential
	CredentialID string `json:"credential_id,omitempty"`
	// AgentID is the agent ID (if applicable)
	AgentID string `json:"agent_id,omitempty"`
	// Cluster is the cluster
	Cluster string `json:"cluster"`
	// Success indicates if the operation was successful
	Success bool `json:"success"`
	// Error is the error message if the operation failed
	Error string `json:"error,omitempty"`
	// SourceIP is the source IP address (if available)
	SourceIP string `json:"source_ip,omitempty"`
	// Details contains additional event details
	Details map[string]interface{} `json:"details,omitempty"`
}

// BootstrapAuditEventType constants
const (
	BootstrapAuditEventGenerate   = "generate"
	BootstrapAuditEventValidate   = "validate"
	BootstrapAuditEventRevoke     = "revoke"
	BootstrapAuditEventUse        = "use"
	BootstrapAuditEventRegister   = "register"
	BootstrapAuditEventExpire     = "expire"
	BootstrapAuditEventCleanup    = "cleanup"
)

// BootstrapAuditLogger logs bootstrap audit events
type BootstrapAuditLogger interface {
	// Log logs a bootstrap audit event
	Log(ctx context.Context, event BootstrapAuditEvent) error
}

// NoOpBootstrapAuditLogger is a no-op audit logger
type NoOpBootstrapAuditLogger struct{}

// Log does nothing
func (l *NoOpBootstrapAuditLogger) Log(ctx context.Context, event BootstrapAuditEvent) error {
	return nil
}
