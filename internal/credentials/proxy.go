// Package credentials provides secure credential management for proxy agents.
package credentials

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
)

// ProxyCredentialProviderConfig configures the NATS credential provider.
type ProxyCredentialProviderConfig struct {
	// NATSConn is the NATS connection.
	NATSConn *nats.Conn
	// ClusterName is the Keystone Core cluster name.
	ClusterName string
	// ProxyAgentID is the proxy agent ID.
	ProxyAgentID string
	// RequestTimeout is the timeout for credential requests.
	RequestTimeout time.Duration
	// Cache configures credential caching.
	Cache *CacheConfig
	// Encryptor handles encryption/decryption.
	Encryptor *CredentialEncryptor
}

// ProxyCredentialProvider fetches credentials from the control plane via NATS.
type ProxyCredentialProvider struct {
	config    *ProxyCredentialProviderConfig
	cache     *CredentialCache
	encryptor *CredentialEncryptor
}

// NewProxyCredentialProvider creates a new NATS-based credential provider.
func NewProxyCredentialProvider(config *ProxyCredentialProviderConfig) (*ProxyCredentialProvider, error) {
	if config.NATSConn == nil {
		return nil, fmt.Errorf("NATS connection is required")
	}
	if config.ClusterName == "" {
		config.ClusterName = "default"
	}
	if config.ProxyAgentID == "" {
		return nil, fmt.Errorf("proxy agent ID is required")
	}
	if config.RequestTimeout == 0 {
		config.RequestTimeout = 30 * time.Second
	}
	if config.Cache == nil {
		config.Cache = DefaultCacheConfig()
	}

	encryptor := config.Encryptor
	if encryptor == nil {
		var err error
		encryptor, err = NewCredentialEncryptor()
		if err != nil {
			return nil, fmt.Errorf("failed to create encryptor: %w", err)
		}
	}

	return &ProxyCredentialProvider{
		config:    config,
		cache:     NewCredentialCache(config.Cache, nil), // No backend, cache-only
		encryptor: encryptor,
	}, nil
}

// GetCredential retrieves a credential by reference.
func (p *ProxyCredentialProvider) GetCredential(ctx context.Context, ref string) (Credential, error) {
	// Try cache first
	cached, err := p.cache.Get(ctx, ref)
	if err == nil {
		return cached, nil
	}

	// Fetch from control plane
	cred, err := p.fetchCredential(ctx, ref)
	if err != nil {
		return nil, err
	}

	// Cache the result
	p.cache.cacheCredential(ref, cred, p.config.Cache.DefaultTTL)

	return cred, nil
}

// fetchCredential fetches a credential from the control plane via NATS.
func (p *ProxyCredentialProvider) fetchCredential(ctx context.Context, ref string) (Credential, error) {
	// Build subject
	subject := fmt.Sprintf("kscore.%s.proxy.%s.credential.fetch",
		p.config.ClusterName, p.config.ProxyAgentID)

	// Create request
	request := CredentialRequest{
		CredentialRef: ref,
		ProxyAgentID:  p.config.ProxyAgentID,
		RequestID:     uuid.New().String(),
		RequestTime:   time.Now(),
	}

	// Include our public key for encrypted response
	requestData, err := json.Marshal(struct {
		CredentialRequest
		PublicKey [32]byte `json:"public_key"`
	}{
		CredentialRequest: request,
		PublicKey:         p.encryptor.GetPublicKey(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Send request with timeout
	msg, err := p.config.NATSConn.RequestWithContext(ctx, subject, requestData)
	if err != nil {
		return nil, fmt.Errorf("credential request failed: %w", err)
	}

	// Parse response
	var response CredentialResponse
	if err := json.Unmarshal(msg.Data, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if response.Error != "" {
		if strings.Contains(response.Error, "not found") {
			return nil, ErrCredentialNotFound
		}
		return nil, fmt.Errorf("credential error: %s", response.Error)
	}

	// Decrypt credential
	var envelope EncryptedCredentialEnvelope
	if err := json.Unmarshal(response.Credential, &envelope); err != nil {
		return nil, fmt.Errorf("failed to parse encrypted envelope: %w", err)
	}

	decrypted, err := p.encryptor.DecryptFromPeer(&envelope)
	if err != nil {
		return nil, err
	}

	// Parse credential
	cred, err := ParseCredential(envelope.CredentialType, decrypted)
	if err != nil {
		return nil, err
	}

	return cred, nil
}

// InvalidateCredential removes a credential from the cache.
func (p *ProxyCredentialProvider) InvalidateCredential(ref string) {
	p.cache.Invalidate(ref)
}

// InvalidateAll clears all cached credentials.
func (p *ProxyCredentialProvider) InvalidateAll() {
	p.cache.InvalidateAll()
}

// GetPublicKey returns the provider's public key for key exchange.
func (p *ProxyCredentialProvider) GetPublicKey() [32]byte {
	return p.encryptor.GetPublicKey()
}

// Close closes the provider.
func (p *ProxyCredentialProvider) Close() error {
	return p.cache.Close()
}

// Stats returns cache statistics.
func (p *ProxyCredentialProvider) Stats() CacheStats {
	return p.cache.Stats()
}

// CredentialHandler handles credential requests on the control plane.
type CredentialHandler struct {
	store       CredentialStore
	encryptor   *CredentialEncryptor
	auditLogger AuditLogger
}

// CredentialHandlerConfig configures the credential handler.
type CredentialHandlerConfig struct {
	Store       CredentialStore
	AuditLogger AuditLogger
}

// NewCredentialHandler creates a new credential handler.
func NewCredentialHandler(config *CredentialHandlerConfig) (*CredentialHandler, error) {
	if config.Store == nil {
		return nil, fmt.Errorf("credential store is required")
	}

	encryptor, err := NewCredentialEncryptor()
	if err != nil {
		return nil, fmt.Errorf("failed to create encryptor: %w", err)
	}

	return &CredentialHandler{
		store:       config.Store,
		encryptor:   encryptor,
		auditLogger: config.AuditLogger,
	}, nil
}

// HandleRequest handles a credential request.
func (h *CredentialHandler) HandleRequest(msg *nats.Msg) {
	ctx := context.Background()
	startTime := time.Now()

	// Parse request
	var request struct {
		CredentialRequest
		PublicKey [32]byte `json:"public_key"`
	}
	if err := json.Unmarshal(msg.Data, &request); err != nil {
		h.sendError(msg, "invalid request format")
		return
	}

	// Audit the request
	if h.auditLogger != nil {
		_ = h.auditLogger.LogCredentialAccess(ctx, &CredentialAccessEvent{ //nolint:errcheck // best-effort audit
			CredentialRef: request.CredentialRef,
			ProxyAgentID:  request.ProxyAgentID,
			RequestID:     request.RequestID,
			Action:        "fetch",
			Timestamp:     startTime,
		})
	}

	// Parse credential reference
	credID, err := ParseCredentialRef(request.CredentialRef)
	if err != nil {
		h.sendError(msg, "invalid credential reference")
		return
	}

	// Fetch credential
	cred, err := h.store.Get(ctx, credID)
	if err != nil {
		switch {
		case errors.Is(err, ErrCredentialNotFound):
			h.sendError(msg, "credential not found")
		case errors.Is(err, ErrCredentialExpired):
			h.sendError(msg, "credential expired")
		default:
			h.sendError(msg, "failed to retrieve credential")
		}
		return
	}

	// Serialize credential
	credData, err := json.Marshal(cred)
	if err != nil {
		h.sendError(msg, "failed to serialize credential")
		return
	}

	// Encrypt for the requesting agent
	envelope, err := h.encryptor.EncryptForPeer(cred.Type(), credData, request.PublicKey)
	if err != nil {
		h.sendError(msg, "encryption failed")
		return
	}

	// Build response
	envelopeData, _ := json.Marshal(envelope)
	response := CredentialResponse{
		RequestID:      request.RequestID,
		Credential:     envelopeData,
		CredentialType: cred.Type(),
		ExpiresAt:      cred.ExpiresAt(),
		ResponseTime:   time.Now(),
	}

	responseData, _ := json.Marshal(response)
	_ = msg.Respond(responseData)

	// Audit success
	if h.auditLogger != nil {
		_ = h.auditLogger.LogCredentialAccess(ctx, &CredentialAccessEvent{ //nolint:errcheck // best-effort audit
			CredentialRef: request.CredentialRef,
			ProxyAgentID:  request.ProxyAgentID,
			RequestID:     request.RequestID,
			Action:        "fetch_success",
			Timestamp:     time.Now(),
			Duration:      time.Since(startTime),
		})
	}
}

// sendError sends an error response.
func (h *CredentialHandler) sendError(msg *nats.Msg, errMsg string) {
	response := CredentialResponse{
		Error:        errMsg,
		ResponseTime: time.Now(),
	}
	responseData, _ := json.Marshal(response)
	_ = msg.Respond(responseData)
}

// ParseCredentialRef parses a credential reference.
// Supported formats:
//   - "vault://path/to/secret" -> vault backend
//   - "k8s://namespace/secret/key" -> kubernetes secret
//   - "file://path/to/file" -> file backend
//   - "id:credential-id" -> direct ID lookup
func ParseCredentialRef(ref string) (string, error) {
	if ref == "" {
		return "", ErrInvalidCredentialRef
	}

	// Handle direct ID references
	if strings.HasPrefix(ref, "id:") {
		return strings.TrimPrefix(ref, "id:"), nil
	}

	// Handle vault references
	if strings.HasPrefix(ref, "vault://") {
		// Return the path as the ID
		return ref, nil
	}

	// Handle kubernetes references
	if strings.HasPrefix(ref, "k8s://") {
		return ref, nil
	}

	// Handle file references
	if strings.HasPrefix(ref, "file://") {
		return ref, nil
	}

	// Assume it's a direct ID if no prefix
	return ref, nil
}

// CredentialRefBuilder helps build credential references.
type CredentialRefBuilder struct{}

// NewCredentialRefBuilder creates a new credential reference builder.
func NewCredentialRefBuilder() *CredentialRefBuilder {
	return &CredentialRefBuilder{}
}

// Vault creates a Vault credential reference.
func (b *CredentialRefBuilder) Vault(path string) string {
	return fmt.Sprintf("vault://%s", path)
}

// K8s creates a Kubernetes secret credential reference.
func (b *CredentialRefBuilder) K8s(namespace, secretName, key string) string {
	return fmt.Sprintf("k8s://%s/%s/%s", namespace, secretName, key)
}

// File creates a file credential reference.
func (b *CredentialRefBuilder) File(path string) string {
	return fmt.Sprintf("file://%s", path)
}

// ID creates a direct ID credential reference.
func (b *CredentialRefBuilder) ID(id string) string {
	return fmt.Sprintf("id:%s", id)
}
