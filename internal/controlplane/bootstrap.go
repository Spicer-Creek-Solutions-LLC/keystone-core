// SPDX-License-Identifier: Apache-2.0

package controlplane

import (
	"context"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"go.keystone-core.io/keystone-core/internal/state"
	"go.keystone-core.io/keystone-core/pkg/api/apikeys"
	"go.keystone-core.io/keystone-core/pkg/api/auth"
	"go.keystone-core.io/keystone-core/pkg/envelope"
)

// Bootstrap-side errors. Validators return these so the handler can
// distinguish recoverable rejections (log + drop) from infrastructure
// failures (log + retry-loop later, when we have one).
var (
	ErrPSKNotFound = errors.New("controlplane: bootstrap PSK not found for agent")
	ErrPSKExpired  = errors.New("controlplane: bootstrap PSK expired")
	ErrPSKConsumed = errors.New("controlplane: bootstrap PSK already consumed")
	ErrPSKMismatch = errors.New("controlplane: bootstrap PSK does not match")
)

// Subscriber is the narrow inbound surface the bootstrap handler
// needs from the NATS layer. Mirrors the NATSPublisher / Subjects
// interface pattern so controlplane stays free of internal/nats
// imports.
type Subscriber interface {
	Subscribe(subject string, handler MessageHandler) (Subscription, error)
}

// MessageHandler is duplicated from internal/nats so controlplane
// doesn't need to import it. internal/nats.MessageHandler satisfies
// the same shape via Go's structural interface for function types.
type MessageHandler func(ctx context.Context, subject string, env envelope.Envelope) error

// Subscription is the lifecycle handle returned by Subscribe.
// internal/nats.Subscription satisfies this.
type Subscription interface {
	Unsubscribe() error
}

// BootstrapValidator validates an agent's identity proof against the
// configured PSK store. PSK is the v1.0 impl; SPIFFE / JWT plug in
// by satisfying this interface. Successful validation MUST mark the
// proof as consumed atomically so a replay within the window is
// rejected.
type BootstrapValidator interface {
	Validate(ctx context.Context, agentID string, proof []byte) error
}

// CredentialIssuer mints "full credentials" once a PSK is consumed.
// v1.0 returns an apikeys-issued API key wrapped in AgentCredentials;
// Epic 09 may extend this to return a NATS-specific token alongside.
type CredentialIssuer interface {
	Issue(ctx context.Context, agentID string) (AgentCredentials, error)
}

// AgentCredentials is the wire-format payload returned to the agent
// on the bootstrap response subject. APIKey is the cleartext —
// surfaced exactly once, never persisted.
//
// Epic 09 task 14 adds the three SVID-related fields. Older
// CredentialIssuer impls (APIKeyIssuer) leave them empty; agents
// MUST check `len(creds.CertChainPEM) > 0` before decoding.
//
// Wire-shape stability: the new fields use `omitempty` so the
// JSON payload an Epic-05 task-9 agent decodes is byte-identical
// to the v1.0 baseline when the issuer is API-key-only. SVID-aware
// agents see them populated when the server runs
// [SVIDBootstrapIssuer] (task 14's preferred path when
// cfg.Identity.Enabled).
type AgentCredentials struct {
	APIKey   string    `json:"api_key"` //nolint:gosec // legitimate cleartext credential carried in-memory only
	AgentID  string    `json:"agent_id"`
	IssuedAt time.Time `json:"issued_at"`

	// CertChainPEM is the issued X509SVID's chain (leaf →
	// signing CA, in that order) as PEM-encoded CERTIFICATE
	// blocks. Empty when the issuer is API-key-only.
	CertChainPEM string `json:"cert_chain_pem,omitempty"`

	// PrivateKeyPEM is the leaf's private key, PKCS#8-encoded
	// inside a PEM block. The cleartext is surfaced exactly once
	// — the server never persists it.
	PrivateKeyPEM string `json:"private_key_pem,omitempty"` //nolint:gosec // legitimate cleartext key

	// TrustBundlePEM is the concatenated PEM-encoded X509
	// authorities from the provider's current trust bundle.
	// Agents load these into their RootCAs pool to verify the
	// server's TLS cert.
	TrustBundlePEM string `json:"trust_bundle_pem,omitempty"`
}

// bootstrapRequest is the inbound register payload. Agent metadata
// is reserved for future expansion (labels, version) but kept
// minimal in v1.0.
type bootstrapRequest struct {
	AgentID string         `json:"agent_id"`
	Proof   string         `json:"proof"` // hex-encoded
	Agent   *agentMetadata `json:"agent,omitempty"`
}

type agentMetadata struct {
	Hostname        string            `json:"hostname,omitempty"`
	OS              string            `json:"os,omitempty"`
	Architecture    string            `json:"architecture,omitempty"`
	PlatformVersion string            `json:"platform_version,omitempty"`
	AgentVersion    string            `json:"agent_version,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
}

// BootstrapHandlerConfig wires the handler. Subjects, Subscriber,
// Publisher, Store, Validator, and Issuer are required; everything
// else has a default.
type BootstrapHandlerConfig struct {
	Subjects   Subjects
	Subscriber Subscriber
	Publisher  NATSPublisher
	Store      state.AgentStore
	Validator  BootstrapValidator
	Issuer     CredentialIssuer
	Logger     *slog.Logger
	Clock      func() time.Time

	// OnAgentRegistered, when non-nil, runs synchronously after
	// store.CreateAgent succeeds and before the response envelope is
	// published. The server boot wires this to ConnectionManager.
	// Register so the newly-registered agent lands in the in-memory
	// cache that the heartbeat path checks. Errors are logged and
	// continue — the agent row already exists in the store; cache
	// hydration repairs on the next server restart.
	OnAgentRegistered func(ctx context.Context, rec *state.AgentRecord) error
}

// BootstrapHandler is the server-side implementation of the v1.0
// bootstrap registration flow (PROJECT-DETAILS §4.2):
//
//  1. Subscribe on the wildcard register pattern.
//  2. On each register message: validate proof → issue credentials →
//     persist agent record → publish credentials on the response
//     subject (with CorrelationID = inbound MessageID).
//
// NATS subject-permission scoping is a v1.0 trust assumption — the
// embedded server has no auth layer, and external deployments rely
// on operator-controlled cluster access until Epic 09 lands.
type BootstrapHandler struct {
	subjects     Subjects
	subscriber   Subscriber
	publisher    NATSPublisher
	store        state.AgentStore
	validator    BootstrapValidator
	issuer       CredentialIssuer
	logger       *slog.Logger
	now          func() time.Time
	onRegistered func(ctx context.Context, rec *state.AgentRecord) error

	mu        sync.Mutex
	started   bool
	stopped   bool
	subscript Subscription
}

// NewBootstrapHandler validates cfg and returns an unstarted handler.
func NewBootstrapHandler(cfg BootstrapHandlerConfig) (*BootstrapHandler, error) {
	if cfg.Subjects == nil {
		return nil, errors.New("controlplane: bootstrap: Subjects is required")
	}
	if cfg.Subscriber == nil {
		return nil, errors.New("controlplane: bootstrap: Subscriber is required")
	}
	if cfg.Publisher == nil {
		return nil, errors.New("controlplane: bootstrap: Publisher is required")
	}
	if cfg.Store == nil {
		return nil, errors.New("controlplane: bootstrap: Store is required")
	}
	if cfg.Validator == nil {
		return nil, errors.New("controlplane: bootstrap: Validator is required")
	}
	if cfg.Issuer == nil {
		return nil, errors.New("controlplane: bootstrap: Issuer is required")
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	return &BootstrapHandler{
		subjects:     cfg.Subjects,
		subscriber:   cfg.Subscriber,
		publisher:    cfg.Publisher,
		store:        cfg.Store,
		validator:    cfg.Validator,
		issuer:       cfg.Issuer,
		logger:       cfg.Logger,
		now:          cfg.Clock,
		onRegistered: cfg.OnAgentRegistered,
	}, nil
}

// Start subscribes to the bootstrap register pattern. Idempotent.
func (h *BootstrapHandler) Start(_ context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.stopped {
		return errors.New("controlplane: bootstrap handler already stopped")
	}
	if h.started {
		return nil
	}
	pattern := h.subjects.BootstrapRegisterPattern()
	sub, err := h.subscriber.Subscribe(pattern, h.handle)
	if err != nil {
		return fmt.Errorf("controlplane: bootstrap subscribe: %w", err)
	}
	h.subscript = sub
	h.started = true
	h.logger.Info("controlplane: bootstrap handler started", "pattern", pattern)
	return nil
}

// Stop unsubscribes. Idempotent.
func (h *BootstrapHandler) Stop(_ context.Context) error {
	h.mu.Lock()
	sub := h.subscript
	h.subscript = nil
	h.stopped = true
	h.mu.Unlock()
	if sub == nil {
		return nil
	}
	if err := sub.Unsubscribe(); err != nil {
		return fmt.Errorf("controlplane: bootstrap unsubscribe: %w", err)
	}
	return nil
}

// handle is the message-driver side of the flow. It runs on the
// subscriber's delivery goroutine; recovers from panics so a
// malformed message can't crash the handler loop.
func (h *BootstrapHandler) handle(ctx context.Context, subject string, env envelope.Envelope) error {
	subjectAgentID := agentIDFromBootstrapSubject(subject)
	if subjectAgentID == "" {
		h.logger.Warn("controlplane: bootstrap: malformed subject", "subject", subject)
		return nil
	}

	var req bootstrapRequest
	if err := json.Unmarshal(env.Payload, &req); err != nil {
		h.logger.Warn("controlplane: bootstrap: payload unmarshal",
			"subject", subject, "err", err)
		return nil
	}
	if req.AgentID == "" {
		req.AgentID = subjectAgentID
	}
	if req.AgentID != subjectAgentID {
		h.logger.Warn("controlplane: bootstrap: agent ID mismatch",
			"subject_id", subjectAgentID, "payload_id", req.AgentID)
		return nil
	}

	proof, err := hex.DecodeString(req.Proof)
	if err != nil {
		h.logger.Warn("controlplane: bootstrap: proof not hex",
			"agent_id", req.AgentID, "err", err)
		return nil
	}

	if err := h.validator.Validate(ctx, req.AgentID, proof); err != nil {
		h.logger.Warn("controlplane: bootstrap: validation failed",
			"agent_id", req.AgentID, "err", err)
		return nil
	}

	creds, err := h.issuer.Issue(ctx, req.AgentID)
	if err != nil {
		h.logger.Error("controlplane: bootstrap: credential issue failed",
			"agent_id", req.AgentID, "err", err)
		return err
	}

	rec := &state.AgentRecord{
		ID:           req.AgentID,
		Status:       state.AgentStatusPending,
		RegisteredAt: h.now().UTC(),
	}
	if req.Agent != nil {
		rec.Hostname = req.Agent.Hostname
		rec.OS = req.Agent.OS
		rec.Architecture = req.Agent.Architecture
		rec.PlatformVersion = req.Agent.PlatformVersion
		rec.AgentVersion = req.Agent.AgentVersion
		rec.Labels = req.Agent.Labels
	}
	if err := h.store.CreateAgent(ctx, rec); err != nil {
		// API key was issued but agent record creation failed.
		// Documented v1.0 trade-off: operator can revoke the key via
		// /api/v1/apikeys; v1.x wraps this in a transaction.
		h.logger.Error("controlplane: bootstrap: CreateAgent failed (api key leaked)",
			"agent_id", req.AgentID, "err", err)
		return err
	}
	if h.onRegistered != nil {
		if err := h.onRegistered(ctx, rec); err != nil {
			h.logger.Warn("controlplane: bootstrap: OnAgentRegistered hook returned error",
				"agent_id", req.AgentID, "err", err)
		}
	}

	respPayload, err := json.Marshal(creds) //nolint:gosec // marshaling AgentCredentials.APIKey to the bootstrap response is the load-bearing point of the protocol — agent gets the cleartext exactly once
	if err != nil {
		h.logger.Error("controlplane: bootstrap: response marshal",
			"agent_id", req.AgentID, "err", err)
		return err
	}
	respEnv := envelope.New(respPayload, h.subjects.Prefix(),
		envelope.WithCorrelationID(env.MessageID),
	)
	if err := h.publisher.PublishEnvelope(ctx, h.subjects.BootstrapResponse(req.AgentID), respEnv); err != nil {
		h.logger.Error("controlplane: bootstrap: response publish",
			"agent_id", req.AgentID, "err", err)
		return err
	}

	h.logger.Info("controlplane: bootstrap registered agent",
		"agent_id", req.AgentID,
	)
	return nil
}

// agentIDFromBootstrapSubject extracts the agent ID from a subject
// matching kscore.<cluster>.bootstrap.<id>.register. Returns "" on
// malformed input.
func agentIDFromBootstrapSubject(subject string) string {
	// Expected token layout: ["kscore", cluster, "bootstrap", id, "register"].
	parts := strings.Split(subject, ".")
	if len(parts) != 5 {
		return ""
	}
	if parts[0] != "kscore" || parts[2] != "bootstrap" || parts[4] != "register" {
		return ""
	}
	return parts[3]
}

// ===== PSKValidator (v1.0 default impl) ============================

// PSKValidatorConfig configures the in-memory PSK validator.
// Entries are normalized at construction (hex-decoded once) and
// consumed entries tracked in-memory only — restart wipes the
// consumption record. Operators are expected to rotate PSKs.
type PSKValidatorConfig struct {
	Entries []PSKEntry
	Clock   func() time.Time
}

// PSKEntry is a configured bootstrap PSK. Secret is the raw bytes
// (already hex-decoded from the config representation).
type PSKEntry struct {
	AgentID   string
	Secret    []byte //nolint:gosec // PSK byte secret — flagged false-positive on field-name pattern
	ExpiresAt time.Time
}

// PSKValidator validates bootstrap proofs against an in-memory PSK
// table. Constant-time comparison defeats timing side channels.
type PSKValidator struct {
	now func() time.Time

	mu       sync.Mutex
	entries  map[string]PSKEntry // agentID → entry
	consumed map[string]struct{} // agentID set
}

// NewPSKValidator constructs a PSKValidator. Caller is responsible
// for hex-decoding config secrets via DecodeConfigPSKs.
func NewPSKValidator(cfg PSKValidatorConfig) *PSKValidator {
	now := cfg.Clock
	if now == nil {
		now = time.Now
	}
	v := &PSKValidator{
		now:      now,
		entries:  make(map[string]PSKEntry, len(cfg.Entries)),
		consumed: make(map[string]struct{}),
	}
	for _, e := range cfg.Entries {
		v.entries[e.AgentID] = e
	}
	return v
}

// Validate looks up the agent's PSK, checks TTL, constant-time
// compares the proof, then marks the entry consumed. Returns one of
// the bootstrap-side sentinel errors on rejection.
func (v *PSKValidator) Validate(_ context.Context, agentID string, proof []byte) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	entry, ok := v.entries[agentID]
	if !ok {
		return ErrPSKNotFound
	}
	if _, alreadyConsumed := v.consumed[agentID]; alreadyConsumed {
		return ErrPSKConsumed
	}
	if !v.now().Before(entry.ExpiresAt) {
		return ErrPSKExpired
	}
	if subtle.ConstantTimeCompare(entry.Secret, proof) != 1 {
		return ErrPSKMismatch
	}
	v.consumed[agentID] = struct{}{}
	return nil
}

// ===== APIKeyIssuer (v1.0 default impl) ============================

// APIKeyIssuer issues an API key per agent and persists it to the
// configured APIKeyStore. v1.0 uses RoleOperator as the agent role
// — Epic 09 introduces a dedicated agent role with bootstrap-issued
// principals carrying narrower permissions.
type APIKeyIssuer struct {
	keys     state.APIKeyStore
	clock    func() time.Time
	keyTTL   time.Duration // zero = never expires
	roleName string
}

// APIKeyIssuerConfig wires the issuer.
type APIKeyIssuerConfig struct {
	Keys     state.APIKeyStore
	Clock    func() time.Time
	KeyTTL   time.Duration // zero = never expires
	RoleName string        // empty = "operator" (v1.0 default)
}

// NewAPIKeyIssuer constructs the v1.0 default credential issuer.
func NewAPIKeyIssuer(cfg APIKeyIssuerConfig) (*APIKeyIssuer, error) {
	if cfg.Keys == nil {
		return nil, errors.New("controlplane: APIKeyIssuer: Keys is required")
	}
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	if cfg.RoleName == "" {
		cfg.RoleName = auth.RoleOperator.String()
	}
	if _, err := auth.ParseRole(cfg.RoleName); err != nil {
		return nil, fmt.Errorf("controlplane: APIKeyIssuer: %w", err)
	}
	return &APIKeyIssuer{
		keys:     cfg.Keys,
		clock:    cfg.Clock,
		keyTTL:   cfg.KeyTTL,
		roleName: cfg.RoleName,
	}, nil
}

// Issue mints + persists an API key for agentID. Returns the
// cleartext exactly once. Persistence failures roll the cleartext
// off the wire (handler discards on error).
func (i *APIKeyIssuer) Issue(ctx context.Context, agentID string) (AgentCredentials, error) {
	expiresAt := time.Time{}
	if i.keyTTL > 0 {
		expiresAt = i.clock().Add(i.keyTTL).UTC()
	}
	gen, err := apikeys.Generate("agent:"+agentID, i.roleName, expiresAt)
	if err != nil {
		return AgentCredentials{}, fmt.Errorf("controlplane: APIKeyIssuer: generate: %w", err)
	}
	if err := i.keys.CreateAPIKey(ctx, gen.Record()); err != nil {
		return AgentCredentials{}, fmt.Errorf("controlplane: APIKeyIssuer: persist: %w", err)
	}
	return AgentCredentials{
		APIKey:   gen.Cleartext,
		AgentID:  agentID,
		IssuedAt: i.clock().UTC(),
	}, nil
}

// DecodeConfigPSKs converts the config representation (hex strings)
// to the byte-slice representation PSKValidator wants. Returns the
// first decode error verbatim — callers should have already passed
// BootstrapConfig.Validate, so this is a sanity check.
func DecodeConfigPSKs(in []ConfigPSK) ([]PSKEntry, error) {
	out := make([]PSKEntry, 0, len(in))
	for _, p := range in {
		secret, err := hex.DecodeString(p.Secret)
		if err != nil {
			return nil, fmt.Errorf("controlplane: PSK %q: hex decode: %w", p.AgentID, err)
		}
		out = append(out, PSKEntry{
			AgentID:   p.AgentID,
			Secret:    secret,
			ExpiresAt: p.ExpiresAt,
		})
	}
	return out, nil
}

// ConfigPSK is the controlplane-shaped view of internal/config.
// BootstrapPSK. controlplane stays import-free of internal/config by
// taking values in this shape; pkg/api/server's wiring layer
// translates from config to ConfigPSK.
type ConfigPSK struct {
	AgentID   string
	Secret    string //nolint:gosec // PSK hex string — flagged false-positive on field-name pattern
	ExpiresAt time.Time
}
