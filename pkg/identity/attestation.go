package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// AttestationEngineConfig contains configuration for the attestation engine.
type AttestationEngineConfig struct {
	// TrustDomain is the SPIFFE trust domain.
	TrustDomain string

	// AllowedAttestors lists which attestors are enabled.
	AllowedAttestors []string

	// AllowNone allows the "none" attestor (dev only).
	AllowNone bool

	// JoinTokenStore is the store for join tokens.
	// If nil, join token attestation will not be available.
	//
	// For development/testing, use InMemoryTokenStore (default):
	//   config.JoinTokenStore = NewInMemoryTokenStore()
	//
	// For production deployments requiring persistence, use SQLiteTokenStore:
	//   store, _ := NewSQLiteTokenStore(&SQLiteTokenStoreConfig{Path: "/data/tokens.db"})
	//   config.JoinTokenStore = store
	//
	// Limitations of InMemoryTokenStore (default):
	//   - All data is lost on server restart
	//   - Cannot be shared across multiple control plane instances
	//   - Suitable for: development, testing, single-instance deployments
	//
	// Limitations of SQLiteTokenStore:
	//   - Slower for high-frequency operations due to disk I/O
	//   - Requires filesystem access
	//   - Suitable for: production, persistence required, HA deployments
	JoinTokenStore JoinTokenStore
}

// AttestationEngine handles workload attestation.
type AttestationEngine struct {
	config    *AttestationEngineConfig
	attestors map[string]Attestor
	mu        sync.RWMutex
}

// NewAttestationEngine creates a new attestation engine.
func NewAttestationEngine(config *AttestationEngineConfig) (*AttestationEngine, error) {
	if config == nil {
		return nil, fmt.Errorf("config required")
	}

	if config.TrustDomain == "" {
		return nil, fmt.Errorf("trust domain required")
	}

	engine := &AttestationEngine{
		config:    config,
		attestors: make(map[string]Attestor),
	}

	// Register built-in attestors
	for _, name := range config.AllowedAttestors {
		switch name {
		case AttestationTypeJoinToken:
			if config.JoinTokenStore == nil {
				return nil, fmt.Errorf("join token store required for join token attestor")
			}
			engine.attestors[name] = NewJoinTokenAttestor(config.TrustDomain, config.JoinTokenStore)

		case AttestationTypeAWSIID:
			engine.attestors[name] = NewAWSIIDAttestor(config.TrustDomain)

		case AttestationTypeGCPIIT:
			engine.attestors[name] = NewGCPIITAttestor(config.TrustDomain)

		case AttestationTypeAzureIMDS:
			engine.attestors[name] = NewAzureIMDSAttestor(config.TrustDomain)

		case AttestationTypeK8sSAT:
			engine.attestors[name] = NewK8sSATAttestor(config.TrustDomain)

		case AttestationTypeNone:
			if !config.AllowNone {
				return nil, fmt.Errorf("none attestor not allowed")
			}
			engine.attestors[name] = NewNoneAttestor(config.TrustDomain)
		}
	}

	return engine, nil
}

// Attest performs attestation using the appropriate attestor.
func (e *AttestationEngine) Attest(ctx context.Context, evidence *AttestationEvidence) (*AttestationResult, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if evidence == nil {
		return &AttestationResult{
			Success: false,
			Error:   "no attestation evidence provided",
		}, nil
	}

	// Find attestor that can handle this evidence
	attestor, ok := e.attestors[evidence.Type]
	if !ok {
		return &AttestationResult{
			Success: false,
			Error:   fmt.Sprintf("no attestor found for type: %s", evidence.Type),
		}, nil
	}

	if !attestor.CanAttest(ctx, evidence) {
		return &AttestationResult{
			Success: false,
			Error:   fmt.Sprintf("attestor %s cannot handle this evidence", evidence.Type),
		}, nil
	}

	return attestor.Attest(ctx, evidence)
}

// RegisterAttestor registers a custom attestor.
func (e *AttestationEngine) RegisterAttestor(attestor Attestor) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.attestors[attestor.Name()] = attestor
}

// ListAttestors returns the list of registered attestors.
func (e *AttestationEngine) ListAttestors() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	names := make([]string, 0, len(e.attestors))
	for name := range e.attestors {
		names = append(names, name)
	}
	return names
}

// JoinTokenAttestor handles join token attestation.
type JoinTokenAttestor struct {
	trustDomain string
	store       JoinTokenStore
}

// NewJoinTokenAttestor creates a new join token attestor.
func NewJoinTokenAttestor(trustDomain string, store JoinTokenStore) *JoinTokenAttestor {
	return &JoinTokenAttestor{
		trustDomain: trustDomain,
		store:       store,
	}
}

// Name returns the attestor name.
func (a *JoinTokenAttestor) Name() string {
	return AttestationTypeJoinToken
}

// CanAttest returns true if this attestor can handle the evidence.
func (a *JoinTokenAttestor) CanAttest(ctx context.Context, evidence *AttestationEvidence) bool {
	return evidence.Type == AttestationTypeJoinToken && len(evidence.Data) > 0
}

// Attest verifies the join token and returns the result.
func (a *JoinTokenAttestor) Attest(ctx context.Context, evidence *AttestationEvidence) (*AttestationResult, error) {
	tokenValue := string(evidence.Data)

	token, err := a.store.Get(ctx, tokenValue)
	if err != nil {
		return &AttestationResult{
			Success:  false,
			Error:    "token not found",
			Attestor: a.Name(),
		}, nil
	}

	if !token.IsValid() {
		return &AttestationResult{
			Success:  false,
			Error:    "token is expired or already used",
			Attestor: a.Name(),
		}, nil
	}

	// Get agent ID from evidence metadata or token
	agentID := evidence.Metadata["agent_id"]
	if agentID == "" {
		agentID = token.AgentID
	}
	if agentID == "" {
		// Generate a random agent ID if not provided
		var err error
		agentID, err = generateAgentID()
		if err != nil {
			return nil, fmt.Errorf("failed to generate agent ID: %w", err)
		}
	}

	// Verify agent ID matches if token has one
	if token.AgentID != "" && token.AgentID != agentID {
		return &AttestationResult{
			Success:  false,
			Error:    "agent ID does not match token",
			Attestor: a.Name(),
		}, nil
	}

	// Mark token as used
	if err := a.store.MarkUsed(ctx, tokenValue, agentID); err != nil {
		return nil, fmt.Errorf("failed to mark token as used: %w", err)
	}

	// Build SPIFFE ID
	spiffeID := NewAgentSPIFFEID(a.trustDomain, agentID)

	// Build selectors from metadata
	selectors := make(map[string]string)
	for k, v := range evidence.Metadata {
		selectors[k] = v
	}
	selectors["attestor"] = a.Name()

	return &AttestationResult{
		Success:   true,
		SPIFFEID:  spiffeID,
		Selectors: selectors,
		ExpiresAt: time.Now().Add(24 * time.Hour), // Attestation valid for 24 hours
		Attestor:  a.Name(),
	}, nil
}

// NoneAttestor allows unauthenticated attestation (dev only).
type NoneAttestor struct {
	trustDomain string
}

// NewNoneAttestor creates a new none attestor.
func NewNoneAttestor(trustDomain string) *NoneAttestor {
	return &NoneAttestor{trustDomain: trustDomain}
}

// Name returns the attestor name.
func (a *NoneAttestor) Name() string {
	return AttestationTypeNone
}

// CanAttest returns true if this attestor can handle the evidence.
func (a *NoneAttestor) CanAttest(ctx context.Context, evidence *AttestationEvidence) bool {
	return evidence.Type == AttestationTypeNone
}

// Attest accepts any attestation (dev only).
func (a *NoneAttestor) Attest(ctx context.Context, evidence *AttestationEvidence) (*AttestationResult, error) {
	agentID := evidence.Metadata["agent_id"]
	if agentID == "" {
		var err error
		agentID, err = generateAgentID()
		if err != nil {
			return nil, fmt.Errorf("failed to generate agent ID: %w", err)
		}
	}

	spiffeID := NewAgentSPIFFEID(a.trustDomain, agentID)

	return &AttestationResult{
		Success:   true,
		SPIFFEID:  spiffeID,
		Selectors: map[string]string{"attestor": a.Name()},
		ExpiresAt: time.Now().Add(24 * time.Hour),
		Attestor:  a.Name(),
	}, nil
}

// AWSIIDAttestor handles AWS Instance Identity Document attestation.
type AWSIIDAttestor struct {
	trustDomain string
}

// NewAWSIIDAttestor creates a new AWS IID attestor.
func NewAWSIIDAttestor(trustDomain string) *AWSIIDAttestor {
	return &AWSIIDAttestor{trustDomain: trustDomain}
}

// Name returns the attestor name.
func (a *AWSIIDAttestor) Name() string {
	return AttestationTypeAWSIID
}

// CanAttest returns true if this attestor can handle the evidence.
func (a *AWSIIDAttestor) CanAttest(ctx context.Context, evidence *AttestationEvidence) bool {
	return evidence.Type == AttestationTypeAWSIID && len(evidence.Data) > 0
}

// Attest verifies the AWS Instance Identity Document.
func (a *AWSIIDAttestor) Attest(ctx context.Context, evidence *AttestationEvidence) (*AttestationResult, error) {
	// Parse the instance identity document
	var iid struct {
		InstanceID       string `json:"instanceId"`
		AccountID        string `json:"accountId"`
		Region           string `json:"region"`
		AvailabilityZone string `json:"availabilityZone"`
		ImageID          string `json:"imageId"`
		Architecture     string `json:"architecture"`
	}

	if err := json.Unmarshal(evidence.Data, &iid); err != nil {
		return &AttestationResult{
			Success:  false,
			Error:    fmt.Sprintf("invalid instance identity document: %v", err),
			Attestor: a.Name(),
		}, nil
	}

	// Note: In production, verify the signature from AWS
	// The signature is in evidence.Metadata["signature"]
	// and should be verified against AWS's public key

	if iid.InstanceID == "" {
		return &AttestationResult{
			Success:  false,
			Error:    "instance ID not found in identity document",
			Attestor: a.Name(),
		}, nil
	}

	// Build SPIFFE ID using AWS-specific path
	spiffeID := SPIFFEID{
		TrustDomain: a.trustDomain,
		Path:        fmt.Sprintf("/agent/aws/%s/%s", iid.Region, iid.InstanceID),
	}

	return &AttestationResult{
		Success:  true,
		SPIFFEID: spiffeID,
		Selectors: map[string]string{
			"attestor":    a.Name(),
			"aws:account": iid.AccountID,
			"aws:region":  iid.Region,
			"aws:az":      iid.AvailabilityZone,
			"aws:image":   iid.ImageID,
		},
		ExpiresAt: time.Now().Add(24 * time.Hour),
		Attestor:  a.Name(),
	}, nil
}

// GCPIITAttestor handles GCP Instance Identity Token attestation.
type GCPIITAttestor struct {
	trustDomain string
}

// NewGCPIITAttestor creates a new GCP IIT attestor.
func NewGCPIITAttestor(trustDomain string) *GCPIITAttestor {
	return &GCPIITAttestor{trustDomain: trustDomain}
}

// Name returns the attestor name.
func (a *GCPIITAttestor) Name() string {
	return AttestationTypeGCPIIT
}

// CanAttest returns true if this attestor can handle the evidence.
func (a *GCPIITAttestor) CanAttest(ctx context.Context, evidence *AttestationEvidence) bool {
	return evidence.Type == AttestationTypeGCPIIT && len(evidence.Data) > 0
}

// Attest verifies the GCP Instance Identity Token.
func (a *GCPIITAttestor) Attest(ctx context.Context, evidence *AttestationEvidence) (*AttestationResult, error) {
	// Parse the identity token claims
	// Note: In production, verify the JWT signature against Google's public keys
	var claims struct {
		ProjectID   string `json:"google.compute_engine.project_id"`
		Zone        string `json:"google.compute_engine.zone"`
		InstanceID  string `json:"google.compute_engine.instance_id"`
		InstanceNam string `json:"google.compute_engine.instance_name"`
	}

	if err := json.Unmarshal(evidence.Data, &claims); err != nil {
		return &AttestationResult{
			Success:  false,
			Error:    fmt.Sprintf("invalid identity token: %v", err),
			Attestor: a.Name(),
		}, nil
	}

	if claims.InstanceID == "" {
		return &AttestationResult{
			Success:  false,
			Error:    "instance ID not found in identity token",
			Attestor: a.Name(),
		}, nil
	}

	// Build SPIFFE ID using GCP-specific path
	spiffeID := SPIFFEID{
		TrustDomain: a.trustDomain,
		Path:        fmt.Sprintf("/agent/gcp/%s/%s", claims.ProjectID, claims.InstanceID),
	}

	return &AttestationResult{
		Success:  true,
		SPIFFEID: spiffeID,
		Selectors: map[string]string{
			"attestor":    a.Name(),
			"gcp:project": claims.ProjectID,
			"gcp:zone":    claims.Zone,
		},
		ExpiresAt: time.Now().Add(24 * time.Hour),
		Attestor:  a.Name(),
	}, nil
}

// AzureIMDSAttestor handles Azure Instance Metadata Service attestation.
type AzureIMDSAttestor struct {
	trustDomain string
}

// NewAzureIMDSAttestor creates a new Azure IMDS attestor.
func NewAzureIMDSAttestor(trustDomain string) *AzureIMDSAttestor {
	return &AzureIMDSAttestor{trustDomain: trustDomain}
}

// Name returns the attestor name.
func (a *AzureIMDSAttestor) Name() string {
	return AttestationTypeAzureIMDS
}

// CanAttest returns true if this attestor can handle the evidence.
func (a *AzureIMDSAttestor) CanAttest(ctx context.Context, evidence *AttestationEvidence) bool {
	return evidence.Type == AttestationTypeAzureIMDS && len(evidence.Data) > 0
}

// Attest verifies the Azure Instance Metadata.
func (a *AzureIMDSAttestor) Attest(ctx context.Context, evidence *AttestationEvidence) (*AttestationResult, error) {
	var metadata struct {
		Compute struct {
			VMID              string `json:"vmId"`
			Name              string `json:"name"`
			SubscriptionID    string `json:"subscriptionId"`
			ResourceGroupName string `json:"resourceGroupName"`
			Location          string `json:"location"`
		} `json:"compute"`
	}

	if err := json.Unmarshal(evidence.Data, &metadata); err != nil {
		return &AttestationResult{
			Success:  false,
			Error:    fmt.Sprintf("invalid instance metadata: %v", err),
			Attestor: a.Name(),
		}, nil
	}

	if metadata.Compute.VMID == "" {
		return &AttestationResult{
			Success:  false,
			Error:    "VM ID not found in instance metadata",
			Attestor: a.Name(),
		}, nil
	}

	// Build SPIFFE ID using Azure-specific path
	spiffeID := SPIFFEID{
		TrustDomain: a.trustDomain,
		Path:        fmt.Sprintf("/agent/azure/%s/%s", metadata.Compute.SubscriptionID, metadata.Compute.VMID),
	}

	return &AttestationResult{
		Success:  true,
		SPIFFEID: spiffeID,
		Selectors: map[string]string{
			"attestor":           a.Name(),
			"azure:subscription": metadata.Compute.SubscriptionID,
			"azure:rg":           metadata.Compute.ResourceGroupName,
			"azure:location":     metadata.Compute.Location,
		},
		ExpiresAt: time.Now().Add(24 * time.Hour),
		Attestor:  a.Name(),
	}, nil
}

// K8sSATAttestor handles Kubernetes Service Account Token attestation.
type K8sSATAttestor struct {
	trustDomain string
}

// NewK8sSATAttestor creates a new Kubernetes SAT attestor.
func NewK8sSATAttestor(trustDomain string) *K8sSATAttestor {
	return &K8sSATAttestor{trustDomain: trustDomain}
}

// Name returns the attestor name.
func (a *K8sSATAttestor) Name() string {
	return AttestationTypeK8sSAT
}

// CanAttest returns true if this attestor can handle the evidence.
func (a *K8sSATAttestor) CanAttest(ctx context.Context, evidence *AttestationEvidence) bool {
	return evidence.Type == AttestationTypeK8sSAT && len(evidence.Data) > 0
}

// Attest verifies the Kubernetes Service Account Token.
func (a *K8sSATAttestor) Attest(ctx context.Context, evidence *AttestationEvidence) (*AttestationResult, error) {
	// Parse the service account token claims
	// Note: In production, verify the JWT against the Kubernetes API
	var claims struct {
		Namespace      string `json:"kubernetes.io/serviceaccount/namespace"`
		ServiceAccount string `json:"kubernetes.io/serviceaccount/service-account.name"`
		PodName        string `json:"kubernetes.io/pod/name"`
		PodUID         string `json:"kubernetes.io/pod/uid"`
	}

	if err := json.Unmarshal(evidence.Data, &claims); err != nil {
		return &AttestationResult{
			Success:  false,
			Error:    fmt.Sprintf("invalid service account token: %v", err),
			Attestor: a.Name(),
		}, nil
	}

	if claims.Namespace == "" || claims.ServiceAccount == "" {
		return &AttestationResult{
			Success:  false,
			Error:    "namespace or service account not found in token",
			Attestor: a.Name(),
		}, nil
	}

	// Build SPIFFE ID using K8s-specific path
	spiffeID := SPIFFEID{
		TrustDomain: a.trustDomain,
		Path:        fmt.Sprintf("/agent/k8s/%s/%s", claims.Namespace, claims.PodUID),
	}

	return &AttestationResult{
		Success:  true,
		SPIFFEID: spiffeID,
		Selectors: map[string]string{
			"attestor": a.Name(),
			"k8s:ns":   claims.Namespace,
			"k8s:sa":   claims.ServiceAccount,
			"k8s:pod":  claims.PodName,
		},
		ExpiresAt: time.Now().Add(24 * time.Hour),
		Attestor:  a.Name(),
	}, nil
}

// InMemoryTokenStore is an in-memory implementation of JoinTokenStore.
// Tokens are stored as salted hashes for security - the raw token value
// is never persisted and can only be retrieved at creation time.
//
// Limitations:
//   - All data is lost on server restart
//   - Cannot be shared across multiple control plane instances
//   - Suitable for: development, testing, single-instance deployments
//
// For production deployments requiring persistence, use SQLiteTokenStore:
//
//	store, err := NewSQLiteTokenStore(&SQLiteTokenStoreConfig{
//	    Path:    "/data/tokens.db",
//	    WALMode: true,
//	})
type InMemoryTokenStore struct {
	// tokens is keyed by token hash (not the raw token value)
	tokens map[string]*JoinToken
	mu     sync.RWMutex
}

// NewInMemoryTokenStore creates a new in-memory token store.
func NewInMemoryTokenStore() *InMemoryTokenStore {
	return &InMemoryTokenStore{
		tokens: make(map[string]*JoinToken),
	}
}

// Create creates a new join token. The token must have the Token field set
// with the plaintext value. This function will generate a salt, compute the
// hash, and store only the hash. The Token field is cleared before storage.
// The caller should save the returned token's Token field (from the input)
// as it will not be available after this call.
func (s *InMemoryTokenStore) Create(ctx context.Context, token *JoinToken) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if token.Token == "" {
		return fmt.Errorf("token value required")
	}

	// Generate salt for this token
	salt, err := generateSalt()
	if err != nil {
		return fmt.Errorf("failed to generate salt: %w", err)
	}

	// Compute the salted hash
	tokenHash := hashToken(token.Token, salt)

	// Check if this hash already exists (extremely unlikely with random salt)
	if _, exists := s.tokens[tokenHash]; exists {
		return fmt.Errorf("token already exists")
	}

	// Store the token prefix for identification
	token.TokenPrefix = tokenPrefix(token.Token)
	token.Salt = salt
	token.TokenHash = tokenHash

	// Clear the raw token value before storing - it should never be persisted
	token.Token = ""

	s.tokens[tokenHash] = token
	return nil
}

// Get retrieves a join token by its plaintext value. The store will hash
// the provided value with each token's salt to find a match.
func (s *InMemoryTokenStore) Get(ctx context.Context, tokenValue string) (*JoinToken, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Search for the token by computing hash with each token's salt
	for _, token := range s.tokens {
		if hashToken(tokenValue, token.Salt) == token.TokenHash {
			return token, nil
		}
	}

	return nil, fmt.Errorf("token not found")
}

// MarkUsed marks a token as used. The tokenValue should be the plaintext token.
func (s *InMemoryTokenStore) MarkUsed(ctx context.Context, tokenValue, usedBy string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Search for the token by computing hash with each token's salt
	for _, token := range s.tokens {
		if hashToken(tokenValue, token.Salt) == token.TokenHash {
			token.Used = true
			token.UsedAt = time.Now()
			token.UsedBy = usedBy
			return nil
		}
	}

	return fmt.Errorf("token not found")
}

// Delete deletes a token by its plaintext value.
func (s *InMemoryTokenStore) Delete(ctx context.Context, tokenValue string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Search for the token by computing hash with each token's salt
	for hash, token := range s.tokens {
		if hashToken(tokenValue, token.Salt) == token.TokenHash {
			delete(s.tokens, hash)
			return nil
		}
	}

	return fmt.Errorf("token not found")
}

// List lists all tokens. The Token field will be empty (redacted) for security.
// Only the TokenPrefix is available for identification.
func (s *InMemoryTokenStore) List(ctx context.Context) ([]*JoinToken, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tokens := make([]*JoinToken, 0, len(s.tokens))
	for _, token := range s.tokens {
		// Create a copy with the Token field explicitly cleared (already empty, but be explicit)
		tokenCopy := &JoinToken{
			TokenHash:   token.TokenHash,
			Salt:        token.Salt,
			TokenPrefix: token.TokenPrefix,
			AgentID:     token.AgentID,
			ExpiresAt:   token.ExpiresAt,
			CreatedAt:   token.CreatedAt,
			Used:        token.Used,
			UsedAt:      token.UsedAt,
			UsedBy:      token.UsedBy,
			Metadata:    token.Metadata,
			// Token field is intentionally left empty
		}
		tokens = append(tokens, tokenCopy)
	}

	return tokens, nil
}

// Cleanup removes expired and used tokens.
func (s *InMemoryTokenStore) Cleanup(ctx context.Context) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	count := 0
	now := time.Now()

	for hash, token := range s.tokens {
		if token.Used || now.After(token.ExpiresAt) {
			delete(s.tokens, hash)
			count++
		}
	}

	return count, nil
}

// Helper functions

func generateToken(length int) (string, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate random token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b)[:length], nil
}

func generateAgentID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate random agent ID: %w", err)
	}
	return fmt.Sprintf("agent-%s", base64.RawURLEncoding.EncodeToString(b)[:16]), nil
}

// generateSalt generates a random salt for token hashing.
func generateSalt() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate random salt: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// hashToken computes a salted SHA-256 hash of the token.
func hashToken(token, salt string) string {
	h := sha256.New()
	h.Write([]byte(salt))
	h.Write([]byte(token))
	return hex.EncodeToString(h.Sum(nil))
}

// tokenPrefix returns the first 8 characters of a token for display purposes.
func tokenPrefix(token string) string {
	if len(token) <= 8 {
		return token
	}
	return token[:8]
}
