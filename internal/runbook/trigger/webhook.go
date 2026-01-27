package trigger

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/shawnbutts/keystone-core/internal/events"
	"github.com/shawnbutts/keystone-core/internal/runbook"
)

// WebhookTrigger represents a webhook-based runbook trigger.
type WebhookTrigger struct {
	// ID is the unique trigger identifier.
	ID string `yaml:"id" json:"id"`

	// Name is a human-readable name.
	Name string `yaml:"name" json:"name"`

	// Description explains what this trigger does.
	Description string `yaml:"description,omitempty" json:"description,omitempty"`

	// RunbookRef references the runbook to execute.
	RunbookRef RunbookRef `yaml:"runbook" json:"runbook"`

	// Path is the URL path for this webhook (e.g., "/webhooks/deploy").
	Path string `yaml:"path" json:"path"`

	// Methods are the allowed HTTP methods (default: POST).
	Methods []string `yaml:"methods,omitempty" json:"methods,omitempty"`

	// Authentication configures webhook authentication.
	Authentication *WebhookAuth `yaml:"authentication,omitempty" json:"authentication,omitempty"`

	// InputMappings maps request data to runbook inputs.
	InputMappings map[string]string `yaml:"inputMappings,omitempty" json:"input_mappings,omitempty"`

	// StaticInputs are static values passed to the runbook.
	StaticInputs map[string]interface{} `yaml:"staticInputs,omitempty" json:"static_inputs,omitempty"`

	// RateLimit configures rate limiting.
	RateLimit *RateLimitConfig `yaml:"rateLimit,omitempty" json:"rate_limit,omitempty"`

	// AllowedIPs restricts access to specific IP addresses.
	AllowedIPs []string `yaml:"allowedIPs,omitempty" json:"allowed_ips,omitempty"`

	// Enabled indicates if the trigger is active.
	Enabled bool `yaml:"enabled" json:"enabled"`

	// Tags for categorization.
	Tags map[string]string `yaml:"tags,omitempty" json:"tags,omitempty"`

	// CreatedAt is when the trigger was created.
	CreatedAt time.Time `yaml:"createdAt,omitempty" json:"created_at,omitempty"`

	// UpdatedAt is when the trigger was last updated.
	UpdatedAt time.Time `yaml:"updatedAt,omitempty" json:"updated_at,omitempty"`
}

// WebhookAuth configures webhook authentication.
type WebhookAuth struct {
	// Type is the authentication type.
	Type WebhookAuthType `yaml:"type" json:"type"`

	// Secret is used for HMAC signature verification.
	Secret string `yaml:"secret,omitempty" json:"secret,omitempty"`

	// Header is the header name for token authentication.
	Header string `yaml:"header,omitempty" json:"header,omitempty"`

	// Token is the expected token value.
	Token string `yaml:"token,omitempty" json:"token,omitempty"`

	// SignatureHeader is the header containing the HMAC signature.
	SignatureHeader string `yaml:"signatureHeader,omitempty" json:"signature_header,omitempty"`

	// SignaturePrefix is the prefix before the signature (e.g., "sha256=").
	SignaturePrefix string `yaml:"signaturePrefix,omitempty" json:"signature_prefix,omitempty"`
}

// WebhookAuthType represents the type of webhook authentication.
type WebhookAuthType string

const (
	// WebhookAuthNone indicates no authentication.
	WebhookAuthNone WebhookAuthType = "none"

	// WebhookAuthToken requires a token in a header.
	WebhookAuthToken WebhookAuthType = "token"

	// WebhookAuthHMAC uses HMAC signature verification.
	WebhookAuthHMAC WebhookAuthType = "hmac"

	// WebhookAuthBasic uses HTTP Basic authentication.
	WebhookAuthBasic WebhookAuthType = "basic"
)

// Validate validates the webhook trigger.
func (t *WebhookTrigger) Validate() error {
	if t.ID == "" {
		return &ValidationError{Field: "id", Message: "id is required"}
	}
	if t.Name == "" {
		return &ValidationError{Field: "name", Message: "name is required"}
	}
	if t.RunbookRef.Name == "" {
		return &ValidationError{Field: "runbook.name", Message: "runbook name is required"}
	}
	if t.Path == "" {
		return &ValidationError{Field: "path", Message: "path is required"}
	}
	if !strings.HasPrefix(t.Path, "/") {
		return &ValidationError{Field: "path", Message: "path must start with /"}
	}

	if t.Authentication != nil {
		switch t.Authentication.Type {
		case WebhookAuthToken:
			if t.Authentication.Header == "" {
				return &ValidationError{Field: "authentication.header", Message: "header is required for token auth"}
			}
			if t.Authentication.Token == "" {
				return &ValidationError{Field: "authentication.token", Message: "token is required for token auth"}
			}
		case WebhookAuthHMAC:
			if t.Authentication.Secret == "" {
				return &ValidationError{Field: "authentication.secret", Message: "secret is required for HMAC auth"}
			}
			if t.Authentication.SignatureHeader == "" {
				return &ValidationError{Field: "authentication.signatureHeader", Message: "signatureHeader is required for HMAC auth"}
			}
		case WebhookAuthBasic:
			if t.Authentication.Token == "" {
				return &ValidationError{Field: "authentication.token", Message: "credentials required for basic auth"}
			}
		}
	}

	return nil
}

// WebhookRequest represents an incoming webhook request.
type WebhookRequest struct {
	Method      string
	Path        string
	Headers     http.Header
	Body        []byte
	RemoteAddr  string
	QueryParams map[string]string
}

// WebhookResponse represents a webhook response.
type WebhookResponse struct {
	StatusCode int                    `json:"status_code"`
	Message    string                 `json:"message"`
	ExecutionID string                `json:"execution_id,omitempty"`
	Error      string                 `json:"error,omitempty"`
	Data       map[string]interface{} `json:"data,omitempty"`
}

// WebhookTriggerManager manages webhook-based triggers.
type WebhookTriggerManager struct {
	mu       sync.RWMutex
	triggers map[string]*WebhookTrigger
	byPath   map[string]*WebhookTrigger

	repository  RunbookRepository
	executor    RunbookExecutor
	publisher   events.EventPublisher
	rateLimiter *RateLimiter
	auditLog    WebhookAuditLogger
}

// WebhookAuditLogger logs webhook events.
type WebhookAuditLogger interface {
	LogRequest(ctx context.Context, entry *WebhookAuditEntry) error
}

// WebhookAuditEntry represents an audit log entry.
type WebhookAuditEntry struct {
	ID          string                 `json:"id"`
	TriggerID   string                 `json:"trigger_id"`
	TriggerName string                 `json:"trigger_name"`
	Method      string                 `json:"method"`
	Path        string                 `json:"path"`
	RemoteAddr  string                 `json:"remote_addr"`
	Headers     map[string]string      `json:"headers,omitempty"`
	StatusCode  int                    `json:"status_code"`
	ExecutionID string                 `json:"execution_id,omitempty"`
	Error       string                 `json:"error,omitempty"`
	Duration    time.Duration          `json:"duration"`
	Timestamp   time.Time              `json:"timestamp"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// WebhookManagerOption configures a WebhookTriggerManager.
type WebhookManagerOption func(*WebhookTriggerManager)

// WithWebhookRepository sets the runbook repository.
func WithWebhookRepository(repo RunbookRepository) WebhookManagerOption {
	return func(m *WebhookTriggerManager) {
		m.repository = repo
	}
}

// WithWebhookExecutor sets the runbook executor.
func WithWebhookExecutor(executor RunbookExecutor) WebhookManagerOption {
	return func(m *WebhookTriggerManager) {
		m.executor = executor
	}
}

// WithWebhookPublisher sets the event publisher.
func WithWebhookPublisher(publisher events.EventPublisher) WebhookManagerOption {
	return func(m *WebhookTriggerManager) {
		m.publisher = publisher
	}
}

// WithWebhookAuditLogger sets the audit logger.
func WithWebhookAuditLogger(logger WebhookAuditLogger) WebhookManagerOption {
	return func(m *WebhookTriggerManager) {
		m.auditLog = logger
	}
}

// NewWebhookTriggerManager creates a new webhook trigger manager.
func NewWebhookTriggerManager(opts ...WebhookManagerOption) *WebhookTriggerManager {
	m := &WebhookTriggerManager{
		triggers:    make(map[string]*WebhookTrigger),
		byPath:      make(map[string]*WebhookTrigger),
		rateLimiter: NewRateLimiter(),
	}

	for _, opt := range opts {
		opt(m)
	}

	return m
}

// Register adds a webhook trigger.
func (m *WebhookTriggerManager) Register(trigger *WebhookTrigger) error {
	if err := trigger.Validate(); err != nil {
		return fmt.Errorf("invalid trigger: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.triggers[trigger.ID]; exists {
		return fmt.Errorf("trigger %s already registered", trigger.ID)
	}

	if _, exists := m.byPath[trigger.Path]; exists {
		return fmt.Errorf("path %s already registered", trigger.Path)
	}

	now := time.Now()
	if trigger.CreatedAt.IsZero() {
		trigger.CreatedAt = now
	}
	trigger.UpdatedAt = now

	// Set default methods
	if len(trigger.Methods) == 0 {
		trigger.Methods = []string{"POST"}
	}

	m.triggers[trigger.ID] = trigger
	m.byPath[trigger.Path] = trigger

	return nil
}

// Unregister removes a webhook trigger.
func (m *WebhookTriggerManager) Unregister(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	trigger, exists := m.triggers[id]
	if !exists {
		return fmt.Errorf("trigger %s not found", id)
	}

	delete(m.triggers, id)
	delete(m.byPath, trigger.Path)

	return nil
}

// Get retrieves a webhook trigger by ID.
func (m *WebhookTriggerManager) Get(id string) (*WebhookTrigger, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	trigger, ok := m.triggers[id]
	return trigger, ok
}

// GetByPath retrieves a webhook trigger by path.
func (m *WebhookTriggerManager) GetByPath(path string) (*WebhookTrigger, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	trigger, ok := m.byPath[path]
	return trigger, ok
}

// List returns all webhook triggers.
func (m *WebhookTriggerManager) List() []*WebhookTrigger {
	m.mu.RLock()
	defer m.mu.RUnlock()

	triggers := make([]*WebhookTrigger, 0, len(m.triggers))
	for _, t := range m.triggers {
		triggers = append(triggers, t)
	}
	return triggers
}

// Enable enables a webhook trigger.
func (m *WebhookTriggerManager) Enable(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	trigger, exists := m.triggers[id]
	if !exists {
		return fmt.Errorf("trigger %s not found", id)
	}

	trigger.Enabled = true
	trigger.UpdatedAt = time.Now()
	return nil
}

// Disable disables a webhook trigger.
func (m *WebhookTriggerManager) Disable(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	trigger, exists := m.triggers[id]
	if !exists {
		return fmt.Errorf("trigger %s not found", id)
	}

	trigger.Enabled = false
	trigger.UpdatedAt = time.Now()
	return nil
}

// HandleRequest processes an incoming webhook request.
func (m *WebhookTriggerManager) HandleRequest(ctx context.Context, req *WebhookRequest) *WebhookResponse {
	startTime := time.Now()

	// Find trigger by path
	trigger, ok := m.GetByPath(req.Path)
	if !ok {
		return &WebhookResponse{
			StatusCode: http.StatusNotFound,
			Message:    "webhook not found",
			Error:      "no webhook registered for path",
		}
	}

	// Check if enabled
	if !trigger.Enabled {
		return &WebhookResponse{
			StatusCode: http.StatusServiceUnavailable,
			Message:    "webhook disabled",
			Error:      "this webhook is currently disabled",
		}
	}

	// Check method
	if !m.isMethodAllowed(trigger, req.Method) {
		return &WebhookResponse{
			StatusCode: http.StatusMethodNotAllowed,
			Message:    "method not allowed",
			Error:      fmt.Sprintf("method %s not allowed for this webhook", req.Method),
		}
	}

	// Check IP allowlist
	if len(trigger.AllowedIPs) > 0 {
		if !m.isIPAllowed(trigger, req.RemoteAddr) {
			m.logAudit(ctx, trigger, req, http.StatusForbidden, "", "ip not allowed", startTime)
			return &WebhookResponse{
				StatusCode: http.StatusForbidden,
				Message:    "access denied",
				Error:      "IP address not allowed",
			}
		}
	}

	// Authenticate
	if err := m.authenticate(trigger, req); err != nil {
		m.logAudit(ctx, trigger, req, http.StatusUnauthorized, "", err.Error(), startTime)
		return &WebhookResponse{
			StatusCode: http.StatusUnauthorized,
			Message:    "authentication failed",
			Error:      err.Error(),
		}
	}

	// Check rate limit
	if trigger.RateLimit != nil {
		if !m.rateLimiter.Allow(trigger.ID, trigger.RateLimit.MaxExecutions, trigger.RateLimit.Window) {
			m.logAudit(ctx, trigger, req, http.StatusTooManyRequests, "", "rate limit exceeded", startTime)
			return &WebhookResponse{
				StatusCode: http.StatusTooManyRequests,
				Message:    "rate limit exceeded",
				Error:      "too many requests",
			}
		}
	}

	// Publish webhook received event
	if m.publisher != nil {
		_ = m.publisher.Publish(&events.Event{
			ID:     uuid.New().String(),
			Type:   events.EventType("runbook.webhook.received"),
			Source: "/runbook/webhook/" + trigger.ID,
			Time:   time.Now(),
			Data: map[string]interface{}{
				"trigger_id":   trigger.ID,
				"trigger_name": trigger.Name,
				"path":         req.Path,
				"method":       req.Method,
			},
		})
	}

	// Execute runbook
	resp := m.executeRunbook(ctx, trigger, req)

	// Log audit
	m.logAudit(ctx, trigger, req, resp.StatusCode, resp.ExecutionID, resp.Error, startTime)

	return resp
}

// isMethodAllowed checks if the HTTP method is allowed.
func (m *WebhookTriggerManager) isMethodAllowed(trigger *WebhookTrigger, method string) bool {
	for _, allowed := range trigger.Methods {
		if strings.EqualFold(allowed, method) {
			return true
		}
	}
	return false
}

// isIPAllowed checks if the IP address is in the allowlist.
func (m *WebhookTriggerManager) isIPAllowed(trigger *WebhookTrigger, remoteAddr string) bool {
	// Extract IP from host:port
	ip := remoteAddr
	if idx := strings.LastIndex(remoteAddr, ":"); idx != -1 {
		ip = remoteAddr[:idx]
	}

	for _, allowed := range trigger.AllowedIPs {
		if allowed == ip {
			return true
		}
		// Simple CIDR matching could be added here
	}
	return false
}

// authenticate authenticates the webhook request.
func (m *WebhookTriggerManager) authenticate(trigger *WebhookTrigger, req *WebhookRequest) error {
	if trigger.Authentication == nil || trigger.Authentication.Type == WebhookAuthNone {
		return nil
	}

	switch trigger.Authentication.Type {
	case WebhookAuthToken:
		return m.authenticateToken(trigger, req)
	case WebhookAuthHMAC:
		return m.authenticateHMAC(trigger, req)
	case WebhookAuthBasic:
		return m.authenticateBasic(trigger, req)
	default:
		return fmt.Errorf("unknown authentication type: %s", trigger.Authentication.Type)
	}
}

// authenticateToken verifies a token header.
func (m *WebhookTriggerManager) authenticateToken(trigger *WebhookTrigger, req *WebhookRequest) error {
	token := req.Headers.Get(trigger.Authentication.Header)
	if token == "" {
		return fmt.Errorf("missing authentication header")
	}

	// Use constant-time comparison
	if subtle.ConstantTimeCompare([]byte(token), []byte(trigger.Authentication.Token)) != 1 {
		return fmt.Errorf("invalid token")
	}

	return nil
}

// authenticateHMAC verifies an HMAC signature.
func (m *WebhookTriggerManager) authenticateHMAC(trigger *WebhookTrigger, req *WebhookRequest) error {
	signature := req.Headers.Get(trigger.Authentication.SignatureHeader)
	if signature == "" {
		return fmt.Errorf("missing signature header")
	}

	// Remove prefix if present
	if trigger.Authentication.SignaturePrefix != "" {
		if !strings.HasPrefix(signature, trigger.Authentication.SignaturePrefix) {
			return fmt.Errorf("invalid signature format")
		}
		signature = signature[len(trigger.Authentication.SignaturePrefix):]
	}

	// Decode signature
	expectedSig, err := hex.DecodeString(signature)
	if err != nil {
		return fmt.Errorf("invalid signature encoding")
	}

	// Calculate expected signature
	mac := hmac.New(sha256.New, []byte(trigger.Authentication.Secret))
	mac.Write(req.Body)
	calculatedSig := mac.Sum(nil)

	// Constant-time comparison
	if !hmac.Equal(expectedSig, calculatedSig) {
		return fmt.Errorf("signature verification failed")
	}

	return nil
}

// authenticateBasic verifies HTTP Basic authentication.
func (m *WebhookTriggerManager) authenticateBasic(trigger *WebhookTrigger, req *WebhookRequest) error {
	auth := req.Headers.Get("Authorization")
	if auth == "" {
		return fmt.Errorf("missing Authorization header")
	}

	if !strings.HasPrefix(auth, "Basic ") {
		return fmt.Errorf("invalid authorization type")
	}

	// Compare the full Authorization header
	expected := "Basic " + trigger.Authentication.Token
	if subtle.ConstantTimeCompare([]byte(auth), []byte(expected)) != 1 {
		return fmt.Errorf("invalid credentials")
	}

	return nil
}

// executeRunbook executes the runbook for a webhook.
func (m *WebhookTriggerManager) executeRunbook(ctx context.Context, trigger *WebhookTrigger, req *WebhookRequest) *WebhookResponse {
	if m.repository == nil || m.executor == nil {
		return &WebhookResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "internal error",
			Error:      "executor not configured",
		}
	}

	// Get runbook
	rb, err := m.repository.GetRunbook(trigger.RunbookRef.Name, trigger.RunbookRef.Version)
	if err != nil {
		return &WebhookResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "runbook not found",
			Error:      err.Error(),
		}
	}

	// Build inputs
	inputs := m.buildInputs(trigger, req)

	// Execute
	exec, err := m.executor.Execute(rb, inputs)
	if err != nil {
		return &WebhookResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "execution failed",
			Error:      err.Error(),
		}
	}

	if exec.State == runbook.ExecutionStateFailed {
		return &WebhookResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "runbook failed",
			ExecutionID: exec.ID,
			Error:      exec.Error,
		}
	}

	// Publish completion event
	if m.publisher != nil {
		_ = m.publisher.Publish(&events.Event{
			ID:     uuid.New().String(),
			Type:   events.EventType("runbook.webhook.executed"),
			Source: "/runbook/webhook/" + trigger.ID,
			Time:   time.Now(),
			Data: map[string]interface{}{
				"trigger_id":   trigger.ID,
				"trigger_name": trigger.Name,
				"execution_id": exec.ID,
				"success":      true,
			},
		})
	}

	return &WebhookResponse{
		StatusCode:  http.StatusOK,
		Message:     "runbook executed successfully",
		ExecutionID: exec.ID,
	}
}

// buildInputs builds runbook inputs from the webhook request.
func (m *WebhookTriggerManager) buildInputs(trigger *WebhookTrigger, req *WebhookRequest) map[string]interface{} {
	inputs := make(map[string]interface{})

	// Add static inputs
	for k, v := range trigger.StaticInputs {
		inputs[k] = v
	}

	// Add webhook metadata
	inputs["__webhook_id"] = trigger.ID
	inputs["__webhook_path"] = req.Path
	inputs["__webhook_method"] = req.Method
	inputs["__trigger_type"] = "webhook"

	// Parse body as JSON
	var bodyData map[string]interface{}
	if len(req.Body) > 0 {
		if err := json.Unmarshal(req.Body, &bodyData); err == nil {
			for k, v := range bodyData {
				inputs["body_"+k] = v
			}
		}
	}

	// Add query params
	for k, v := range req.QueryParams {
		inputs["query_"+k] = v
	}

	// Apply input mappings
	for inputName, source := range trigger.InputMappings {
		value := m.resolveInputMapping(source, req, bodyData)
		if value != nil {
			inputs[inputName] = value
		}
	}

	return inputs
}

// resolveInputMapping resolves an input mapping source.
func (m *WebhookTriggerManager) resolveInputMapping(source string, req *WebhookRequest, bodyData map[string]interface{}) interface{} {
	// Check for body reference: {{ .body.field }}
	if strings.HasPrefix(source, "{{ .body.") && strings.HasSuffix(source, " }}") {
		field := source[9 : len(source)-3]
		if v, ok := bodyData[field]; ok {
			return v
		}
		return nil
	}

	// Check for query reference: {{ .query.field }}
	if strings.HasPrefix(source, "{{ .query.") && strings.HasSuffix(source, " }}") {
		field := source[10 : len(source)-3]
		if v, ok := req.QueryParams[field]; ok {
			return v
		}
		return nil
	}

	// Check for header reference: {{ .header.field }}
	if strings.HasPrefix(source, "{{ .header.") && strings.HasSuffix(source, " }}") {
		field := source[11 : len(source)-3]
		return req.Headers.Get(field)
	}

	// Return as literal value
	return source
}

// logAudit logs an audit entry for the webhook request.
func (m *WebhookTriggerManager) logAudit(ctx context.Context, trigger *WebhookTrigger, req *WebhookRequest, statusCode int, executionID, errorMsg string, startTime time.Time) {
	if m.auditLog == nil {
		return
	}

	// Convert headers to map
	headers := make(map[string]string)
	for k, v := range req.Headers {
		if len(v) > 0 {
			// Redact sensitive headers
			if strings.EqualFold(k, "Authorization") || strings.Contains(strings.ToLower(k), "token") {
				headers[k] = "[REDACTED]"
			} else {
				headers[k] = v[0]
			}
		}
	}

	entry := &WebhookAuditEntry{
		ID:          uuid.New().String(),
		TriggerID:   trigger.ID,
		TriggerName: trigger.Name,
		Method:      req.Method,
		Path:        req.Path,
		RemoteAddr:  req.RemoteAddr,
		Headers:     headers,
		StatusCode:  statusCode,
		ExecutionID: executionID,
		Error:       errorMsg,
		Duration:    time.Since(startTime),
		Timestamp:   time.Now(),
	}

	_ = m.auditLog.LogRequest(ctx, entry)
}

// HTTPHandler returns an http.Handler for the webhook manager.
func (m *WebhookTriggerManager) HTTPHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Read body
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read body", http.StatusBadRequest)
			return
		}

		// Build query params map
		queryParams := make(map[string]string)
		for k, v := range r.URL.Query() {
			if len(v) > 0 {
				queryParams[k] = v[0]
			}
		}

		// Build webhook request
		req := &WebhookRequest{
			Method:      r.Method,
			Path:        r.URL.Path,
			Headers:     r.Header,
			Body:        body,
			RemoteAddr:  r.RemoteAddr,
			QueryParams: queryParams,
		}

		// Handle request
		resp := m.HandleRequest(r.Context(), req)

		// Write response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		_ = json.NewEncoder(w).Encode(resp)
	})
}
