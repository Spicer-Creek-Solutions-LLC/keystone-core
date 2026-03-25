package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
)

// BootstrapHandlerConfig contains configuration for the bootstrap registration handler
type BootstrapHandlerConfig struct {
	// Cluster is the cluster name for subject namespacing
	Cluster string
	// ServerID identifies this control plane server
	ServerID string
	// CredentialTTL is the TTL for issued permanent credentials (0 means no expiry)
	CredentialTTL time.Duration
	// RequireIdentityVerification requires successful identity verification for registration
	RequireIdentityVerification bool
}

// DefaultBootstrapHandlerConfig returns the default configuration
func DefaultBootstrapHandlerConfig(cluster, serverID string) *BootstrapHandlerConfig {
	if cluster == "" {
		cluster = DefaultCluster
	}
	return &BootstrapHandlerConfig{
		Cluster:       cluster,
		ServerID:      serverID,
		CredentialTTL: 0, // No expiry by default
	}
}

// RegistrationCallback is called when an agent successfully registers
type RegistrationCallback func(ctx context.Context, agentID string, labels map[string]string, credentials *IssuedCredentials) error

// BootstrapRegistrationHandler handles the bootstrap registration flow.
// It subscribes to bootstrap registration subjects, validates credentials,
// and issues permanent credentials to agents.
type BootstrapRegistrationHandler struct {
	config             *BootstrapHandlerConfig
	credentialProvider BootstrapCredentialProvider
	credentialIssuer   CredentialIssuer
	identityVerifiers  []IdentityVerifier
	auditLogger        BootstrapAuditLogger
	subjects           *SubjectBuilder

	conn *nats.Conn
	sub  *nats.Subscription

	mu                   sync.RWMutex
	registrationCallback RegistrationCallback

	ctx    context.Context
	cancel context.CancelFunc
}

// NewBootstrapRegistrationHandler creates a new bootstrap registration handler
func NewBootstrapRegistrationHandler(
	config *BootstrapHandlerConfig,
	provider BootstrapCredentialProvider,
) (*BootstrapRegistrationHandler, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}
	if provider == nil {
		return nil, fmt.Errorf("credential provider is required")
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &BootstrapRegistrationHandler{
		config:             config,
		credentialProvider: provider,
		subjects:           NewSubjectBuilder(config.Cluster),
		auditLogger:        &NoOpBootstrapAuditLogger{},
		ctx:                ctx,
		cancel:             cancel,
	}, nil
}

// SetCredentialIssuer sets the credential issuer for issuing permanent credentials
func (h *BootstrapRegistrationHandler) SetCredentialIssuer(issuer CredentialIssuer) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.credentialIssuer = issuer
}

// AddIdentityVerifier adds an identity verifier for additional verification
func (h *BootstrapRegistrationHandler) AddIdentityVerifier(verifier IdentityVerifier) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.identityVerifiers = append(h.identityVerifiers, verifier)
}

// SetAuditLogger sets the audit logger
func (h *BootstrapRegistrationHandler) SetAuditLogger(logger BootstrapAuditLogger) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.auditLogger = logger
}

// SetRegistrationCallback sets the callback for successful registrations
func (h *BootstrapRegistrationHandler) SetRegistrationCallback(callback RegistrationCallback) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.registrationCallback = callback
}

// Start starts the bootstrap registration handler
func (h *BootstrapRegistrationHandler) Start(conn *nats.Conn) error {
	if conn == nil {
		return fmt.Errorf("NATS connection is required")
	}

	h.conn = conn

	// Subscribe to all bootstrap registration requests
	subject := h.subjects.BootstrapWildcard()
	sub, err := conn.Subscribe(subject, h.handleMessage)
	if err != nil {
		return fmt.Errorf("failed to subscribe to %s: %w", subject, err)
	}

	h.sub = sub
	slog.Info("bootstrap registration handler started", "subject", subject)

	return nil
}

// Stop stops the bootstrap registration handler
func (h *BootstrapRegistrationHandler) Stop() error {
	h.cancel()

	if h.sub != nil {
		if err := h.sub.Unsubscribe(); err != nil {
			return fmt.Errorf("failed to unsubscribe: %w", err)
		}
	}

	return nil
}

// handleMessage handles incoming NATS messages
func (h *BootstrapRegistrationHandler) handleMessage(msg *nats.Msg) {
	// Parse subject to extract bootstrap ID
	parsed := ParseSubject(msg.Subject)
	if !parsed.IsValid || parsed.Category != CategoryBootstrap {
		slog.Warn("ignoring invalid subject", "subject", msg.Subject)
		return
	}

	// Only handle registration requests
	if parsed.Operation != OpRegister {
		return
	}

	// Handle the registration request
	h.handleRegistration(msg, parsed.EntityID)
}

// handleRegistration processes a bootstrap registration request
func (h *BootstrapRegistrationHandler) handleRegistration(msg *nats.Msg, bootstrapID string) {
	ctx, cancel := context.WithTimeout(h.ctx, 30*time.Second)
	defer cancel()

	// Parse the registration request
	var req BootstrapRegistrationRequest
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		h.sendErrorResponse(msg, bootstrapID, "invalid_request", "failed to parse registration request")
		h.logAuditEvent(ctx, BootstrapAuditEventRegister, bootstrapID, "", false, "invalid request format", nil)
		return
	}

	// Validate bootstrap credential
	if req.BootstrapCredential == nil {
		h.sendErrorResponse(msg, bootstrapID, "missing_credential", "bootstrap credential is required")
		h.logAuditEvent(ctx, BootstrapAuditEventRegister, bootstrapID, req.AgentID, false, "missing credential", nil)
		return
	}

	// Validate the bootstrap credential
	result, err := h.credentialProvider.Validate(ctx, req.BootstrapCredential)
	if err != nil {
		h.sendErrorResponse(msg, bootstrapID, "validation_error", err.Error())
		h.logAuditEvent(ctx, BootstrapAuditEventValidate, bootstrapID, req.AgentID, false, err.Error(), nil)
		return
	}

	if !result.Valid {
		errMsg := "credential validation failed"
		if result.Error != nil {
			errMsg = result.Error.Error()
		}
		h.sendErrorResponse(msg, bootstrapID, "invalid_credential", errMsg)
		h.logAuditEvent(ctx, BootstrapAuditEventValidate, bootstrapID, req.AgentID, false, errMsg, nil)
		return
	}

	// Check if agent ID is allowed (if restricted)
	if result.Claims.AllowedAgentID != "" && result.Claims.AllowedAgentID != req.AgentID {
		h.sendErrorResponse(msg, bootstrapID, "agent_id_mismatch", "agent ID does not match allowed agent ID")
		h.logAuditEvent(ctx, BootstrapAuditEventRegister, bootstrapID, req.AgentID, false, "agent ID mismatch", map[string]interface{}{
			"allowed_agent_id": result.Claims.AllowedAgentID,
			"actual_agent_id":  req.AgentID,
		})
		return
	}

	// Check if labels match (if restricted)
	if len(result.Claims.AllowedLabels) > 0 {
		for key, expectedValue := range result.Claims.AllowedLabels {
			if actualValue, ok := req.Labels[key]; !ok || actualValue != expectedValue {
				h.sendErrorResponse(msg, bootstrapID, "label_mismatch", fmt.Sprintf("label %s does not match", key))
				h.logAuditEvent(ctx, BootstrapAuditEventRegister, bootstrapID, req.AgentID, false, "label mismatch", map[string]interface{}{
					"label":          key,
					"expected_value": expectedValue,
					"actual_value":   req.Labels[key],
				})
				return
			}
		}
	}

	// Verify identity (if verifiers are configured and required)
	var verifiedIdentity *IdentityVerificationResult
	if h.config.RequireIdentityVerification || len(req.IdentityClaims) > 0 {
		verifiedIdentity, err = h.verifyIdentity(ctx, req.IdentityClaims)
		if err != nil && h.config.RequireIdentityVerification {
			h.sendErrorResponse(msg, bootstrapID, "identity_verification_failed", err.Error())
			h.logAuditEvent(ctx, BootstrapAuditEventRegister, bootstrapID, req.AgentID, false, "identity verification failed", map[string]interface{}{
				"error": err.Error(),
			})
			return
		}
	}

	// Record credential use
	if err := h.credentialProvider.RecordUse(ctx, result.Claims.ID, req.AgentID); err != nil {
		slog.Warn("failed to record credential use", "error", err)
	}

	// Log successful validation
	h.logAuditEvent(ctx, BootstrapAuditEventValidate, bootstrapID, req.AgentID, true, "", nil)

	// Issue permanent credentials
	credentials, err := h.issuePermanentCredentials(ctx, req, result.Claims, verifiedIdentity)
	if err != nil {
		h.sendErrorResponse(msg, bootstrapID, "credential_issue_failed", err.Error())
		h.logAuditEvent(ctx, BootstrapAuditEventRegister, bootstrapID, req.AgentID, false, "failed to issue credentials", map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	// Send success response
	resp := BootstrapRegistrationResponse{
		Success:     true,
		AgentID:     req.AgentID,
		Credentials: credentials,
	}

	respData, err := json.Marshal(resp)
	if err != nil {
		slog.Error("failed to marshal bootstrap response", "error", err)
		return
	}

	// Publish response to the bootstrap response subject
	responseSubject := h.subjects.BootstrapResponse(bootstrapID)
	if err := h.conn.Publish(responseSubject, respData); err != nil {
		slog.Error("failed to publish bootstrap response", "error", err)
		return
	}

	// Log successful registration
	h.logAuditEvent(ctx, BootstrapAuditEventRegister, bootstrapID, req.AgentID, true, "", map[string]interface{}{
		"verified_identity": verifiedIdentity != nil && verifiedIdentity.Verified,
	})

	// Call registration callback if set
	h.mu.RLock()
	callback := h.registrationCallback
	h.mu.RUnlock()

	if callback != nil {
		if err := callback(ctx, req.AgentID, req.Labels, credentials); err != nil {
			slog.Error("registration callback failed", "error", err)
		}
	}

	slog.Info("agent registered via bootstrap", "agent_id", req.AgentID, "bootstrap_id", bootstrapID)
}

// verifyIdentity verifies identity claims using configured verifiers
func (h *BootstrapRegistrationHandler) verifyIdentity(ctx context.Context, claims map[string]interface{}) (*IdentityVerificationResult, error) {
	h.mu.RLock()
	verifiers := h.identityVerifiers
	h.mu.RUnlock()

	if len(verifiers) == 0 {
		return nil, fmt.Errorf("no identity verifiers configured")
	}

	// Try each verifier until one succeeds
	var lastErr error
	for _, verifier := range verifiers {
		result, err := verifier.Verify(ctx, claims)
		if err != nil {
			lastErr = err
			continue
		}
		if result.Verified {
			return result, nil
		}
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("identity verification failed")
}

// issuePermanentCredentials issues permanent credentials for an agent
func (h *BootstrapRegistrationHandler) issuePermanentCredentials(
	ctx context.Context,
	req BootstrapRegistrationRequest,
	bootstrapClaims *BootstrapClaims,
	verifiedIdentity *IdentityVerificationResult,
) (*IssuedCredentials, error) {
	// If a credential issuer is configured, use it
	h.mu.RLock()
	issuer := h.credentialIssuer
	h.mu.RUnlock()

	if issuer != nil {
		issueReq := CredentialIssueRequest{
			AgentID:          req.AgentID,
			Cluster:          bootstrapClaims.Cluster,
			Labels:           req.Labels,
			BootstrapID:      bootstrapClaims.ID,
			VerifiedIdentity: verifiedIdentity,
		}
		return issuer.Issue(ctx, issueReq)
	}

	// Default: generate NKey credentials
	return h.generateDefaultCredentials(req.AgentID, bootstrapClaims.Cluster)
}

// generateDefaultCredentials generates default NKey credentials for an agent
func (h *BootstrapRegistrationHandler) generateDefaultCredentials(agentID, cluster string) (*IssuedCredentials, error) {
	// Generate NKey pair for the agent
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

	// Get subject permissions for the agent
	permissions := AgentPermissions(cluster, agentID)

	credentials := &IssuedCredentials{
		AgentID:        agentID,
		NKeyPublicKey:  publicKey,
		NKeyPrivateKey: string(seed),
		Subjects:       permissions,
	}

	// Set expiry if configured
	if h.config.CredentialTTL > 0 {
		expiry := time.Now().Add(h.config.CredentialTTL)
		credentials.ExpiresAt = &expiry
	}

	return credentials, nil
}

// sendErrorResponse sends an error response to the bootstrap response subject
func (h *BootstrapRegistrationHandler) sendErrorResponse(msg *nats.Msg, bootstrapID, errorCode, errorMsg string) {
	resp := BootstrapRegistrationResponse{
		Success:   false,
		Error:     errorMsg,
		ErrorCode: errorCode,
	}

	respData, err := json.Marshal(resp)
	if err != nil {
		slog.Error("failed to marshal error response", "error", err)
		return
	}

	responseSubject := h.subjects.BootstrapResponse(bootstrapID)
	if err := h.conn.Publish(responseSubject, respData); err != nil {
		slog.Error("failed to publish error response", "error", err)
	}
}

// logAuditEvent logs a bootstrap audit event
func (h *BootstrapRegistrationHandler) logAuditEvent(
	ctx context.Context,
	eventType string,
	credentialID string,
	agentID string,
	success bool,
	errorMsg string,
	details map[string]interface{},
) {
	h.mu.RLock()
	logger := h.auditLogger
	h.mu.RUnlock()

	if logger == nil {
		return
	}

	event := BootstrapAuditEvent{
		Timestamp:    time.Now(),
		EventType:    eventType,
		CredentialID: credentialID,
		AgentID:      agentID,
		Cluster:      h.config.Cluster,
		Success:      success,
		Error:        errorMsg,
		Details:      details,
	}

	if err := logger.Log(ctx, event); err != nil {
		slog.Warn("failed to log audit event", "error", err)
	}
}

// GenerateBootstrapCredential is a helper method to generate a bootstrap credential
func (h *BootstrapRegistrationHandler) GenerateBootstrapCredential(ctx context.Context, req BootstrapCredentialRequest) (*BootstrapCredential, error) {
	if req.Cluster == "" {
		req.Cluster = h.config.Cluster
	}

	cred, err := h.credentialProvider.Generate(ctx, req)
	if err != nil {
		h.logAuditEvent(ctx, BootstrapAuditEventGenerate, "", "", false, err.Error(), nil)
		return nil, err
	}

	h.logAuditEvent(ctx, BootstrapAuditEventGenerate, cred.Claims.ID, "", true, "", map[string]interface{}{
		"ttl":              cred.Claims.TTL().String(),
		"allowed_agent_id": req.AllowedAgentID,
		"max_uses":         req.MaxUses,
	})

	return cred, nil
}

// RevokeBootstrapCredential is a helper method to revoke a bootstrap credential
func (h *BootstrapRegistrationHandler) RevokeBootstrapCredential(ctx context.Context, credentialID, reason string) error {
	err := h.credentialProvider.Revoke(ctx, credentialID, reason)
	if err != nil {
		h.logAuditEvent(ctx, BootstrapAuditEventRevoke, credentialID, "", false, err.Error(), nil)
		return err
	}

	h.logAuditEvent(ctx, BootstrapAuditEventRevoke, credentialID, "", true, "", map[string]interface{}{
		"reason": reason,
	})

	return nil
}

// ListActiveCredentials lists all active bootstrap credentials
func (h *BootstrapRegistrationHandler) ListActiveCredentials(ctx context.Context) ([]*BootstrapCredentialStatus, error) {
	return h.credentialProvider.ListActive(ctx)
}

// GetCredentialStatus returns the status of a bootstrap credential
func (h *BootstrapRegistrationHandler) GetCredentialStatus(ctx context.Context, credentialID string) (*BootstrapCredentialStatus, error) {
	return h.credentialProvider.GetStatus(ctx, credentialID)
}

// CleanupExpiredCredentials removes expired credentials
func (h *BootstrapRegistrationHandler) CleanupExpiredCredentials(ctx context.Context) (int, error) {
	removed, err := h.credentialProvider.Cleanup(ctx)
	if err != nil {
		return 0, err
	}

	if removed > 0 {
		h.logAuditEvent(ctx, BootstrapAuditEventCleanup, "", "", true, "", map[string]interface{}{
			"removed_count": removed,
		})
	}

	return removed, nil
}
