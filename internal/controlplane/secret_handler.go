// SPDX-License-Identifier: Apache-2.0

package controlplane

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"

	"go.keystone-core.io/keystone-core/internal/agent"
	"go.keystone-core.io/keystone-core/internal/sealed"
	"go.keystone-core.io/keystone-core/internal/secrets"
	"go.keystone-core.io/keystone-core/internal/state"
	"go.keystone-core.io/keystone-core/pkg/envelope"
)

// SecretReader is the slice of the secrets broker this handler needs.
type SecretReader interface {
	GetSecret(ctx context.Context, req secrets.GetSecretRequest) (*secrets.Secret, error)
}

// AgentLabelSource supplies the labels a grant decision is made
// against. They come from the control plane's own agent record, never
// from the request: an agent that could state its own labels could
// label its way into any rule.
type AgentLabelSource interface {
	GetAgent(ctx context.Context, id string) (*state.AgentRecord, error)
}

// SecretHandlerConfig constructs a SecretRequestHandler.
type SecretHandlerConfig struct {
	Subscriber Subscriber
	Publisher  NATSPublisher
	Subjects   SecretSubjects
	Verifier   *SVIDVerifier
	Grants     *secrets.AgentGrants
	Broker     SecretReader
	Agents     AgentLabelSource
	Logger     *slog.Logger
}

// SecretSubjects is the subject surface the handler needs.
type SecretSubjects interface {
	SecretRequest() string
	SecretResponse(agentID string) string
	Prefix() string
}

// SecretRequestHandler answers agent secret lookups.
//
// Four things happen in order, and the order is the design:
//
//  1. Authenticate. The SVID verifier establishes WHICH agent is
//     asking, from the certificate rather than from any field.
//  2. Authorize. Grants are evaluated against that verified id and the
//     labels on the control plane's own agent record.
//  3. Read. Only now does the broker see the path.
//  4. Seal. The value is encrypted to the same certificate that
//     authenticated the request, so the reply is readable only by the
//     agent that asked -- necessary because every agent can subscribe
//     to the response subject.
//
// A failure at any step returns the same shape of answer, and no step
// tells the asker anything about a path it is not entitled to.
type SecretRequestHandler struct {
	sub          Subscriber
	publisher    NATSPublisher
	subjects     SecretSubjects
	verifier     *SVIDVerifier
	grants       *secrets.AgentGrants
	broker       SecretReader
	agents       AgentLabelSource
	log          *slog.Logger
	subscription Subscription
}

// NewSecretRequestHandler validates cfg.
//
// Grants may be nil: that is a server with no agent grants configured,
// which denies every lookup. Broker may not -- a handler that cannot
// read is misconfiguration, not policy.
func NewSecretRequestHandler(cfg SecretHandlerConfig) (*SecretRequestHandler, error) {
	if cfg.Subscriber == nil {
		return nil, errors.New("controlplane: secret handler: Subscriber is required")
	}
	if cfg.Publisher == nil {
		return nil, errors.New("controlplane: secret handler: Publisher is required")
	}
	if cfg.Subjects == nil {
		return nil, errors.New("controlplane: secret handler: Subjects is required")
	}
	if cfg.Verifier == nil {
		return nil, errors.New("controlplane: secret handler: Verifier is required")
	}
	if cfg.Broker == nil {
		return nil, errors.New("controlplane: secret handler: Broker is required")
	}
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	return &SecretRequestHandler{
		sub:       cfg.Subscriber,
		publisher: cfg.Publisher,
		subjects:  cfg.Subjects,
		verifier:  cfg.Verifier,
		grants:    cfg.Grants,
		broker:    cfg.Broker,
		agents:    cfg.Agents,
		log:       log,
	}, nil
}

// Start subscribes to the shared secret-request subject.
func (h *SecretRequestHandler) Start(_ context.Context) error {
	if h.subscription != nil {
		return nil
	}
	subject := h.subjects.SecretRequest()
	sub, err := h.sub.Subscribe(subject, h.handle)
	if err != nil {
		return fmt.Errorf("controlplane: secret handler subscribe %q: %w", subject, err)
	}
	h.subscription = sub
	return nil
}

// Stop unsubscribes. Safe before Start.
func (h *SecretRequestHandler) Stop() error {
	if h.subscription == nil {
		return nil
	}
	sub := h.subscription
	h.subscription = nil
	if err := sub.Unsubscribe(); err != nil {
		return fmt.Errorf("controlplane: secret handler unsubscribe: %w", err)
	}
	return nil
}

func (h *SecretRequestHandler) handle(ctx context.Context, subject string, env envelope.Envelope) error {
	var signed agent.SignedRequest
	if err := json.Unmarshal(env.Payload, &signed); err != nil {
		// Nowhere to reply: without a parsed request there is no
		// verified agent to address, and guessing from the envelope
		// would let an unauthenticated publish steer a reply.
		h.log.Warn("controlplane: secret request unmarshal", "subject", subject, "err", err)
		return nil
	}

	agentID, payload, err := h.verifier.Verify(&signed)
	if err != nil {
		// Same reasoning: an unverified request names no one we are
		// willing to answer. Logged at warn because a burst here is an
		// attack signature, not noise.
		h.log.Warn("controlplane: secret request rejected",
			"subject", subject, "claimed_agent_id", signed.AgentID, "err", err)
		return nil
	}

	var req agent.SecretLookupRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		h.reply(ctx, agentID, signed.Nonce, &agent.SecretLookupResponse{
			Nonce: signed.Nonce, Error: "malformed lookup request",
		})
		return nil
	}

	identity := secrets.AgentIdentity{AgentID: agentID}
	if h.agents != nil {
		rec, err := h.agents.GetAgent(ctx, agentID)
		if err != nil {
			// A verified certificate with no agent record is an agent
			// the control plane has forgotten. Fail closed: label-based
			// grants would otherwise silently evaluate against no
			// labels and could match a rule that wanted none.
			h.log.Warn("controlplane: secret request: agent record lookup",
				"agent_id", agentID, "err", err)
			h.reply(ctx, agentID, signed.Nonce, &agent.SecretLookupResponse{
				Nonce: signed.Nonce, Denied: true, Error: "no agent record",
			})
			return nil
		}
		if rec != nil {
			identity.Labels = rec.Labels
		}
	}

	if !h.grants.Allows(identity, req.Path) {
		// The denial names the path the agent already sent us and
		// nothing else -- not whether it exists, not what would have
		// been granted.
		h.log.Info("controlplane: secret request denied",
			"agent_id", agentID, "path", req.Path)
		h.reply(ctx, agentID, signed.Nonce, &agent.SecretLookupResponse{
			Nonce: signed.Nonce, Denied: true, Error: "path not granted to this agent",
		})
		return nil
	}

	secret, err := h.broker.GetSecret(ctx, secrets.GetSecretRequest{Path: req.Path})
	if err != nil {
		h.log.Warn("controlplane: secret request: broker read",
			"agent_id", agentID, "path", req.Path, "err", err)
		h.reply(ctx, agentID, signed.Nonce, &agent.SecretLookupResponse{
			Nonce: signed.Nonce, Error: "secret unavailable",
		})
		return nil
	}

	value, err := secretValue(secret, req.Key)
	if err != nil {
		h.reply(ctx, agentID, signed.Nonce, &agent.SecretLookupResponse{
			Nonce: signed.Nonce, Error: err.Error(),
		})
		return nil
	}

	leaf, err := leafFromChainPEM(signed.CertChainPEM)
	if err != nil {
		h.log.Warn("controlplane: secret request: leaf parse", "agent_id", agentID, "err", err)
		return nil
	}
	box, err := sealed.Seal(leaf.PublicKey, []byte(value), agent.SecretAAD(agentID, signed.Nonce))
	if err != nil {
		h.log.Error("controlplane: secret request: seal", "agent_id", agentID, "err", err)
		h.reply(ctx, agentID, signed.Nonce, &agent.SecretLookupResponse{
			Nonce: signed.Nonce, Error: "cannot seal response",
		})
		return nil
	}

	h.log.Info("controlplane: secret request served",
		"agent_id", agentID, "path", req.Path)
	h.reply(ctx, agentID, signed.Nonce, &agent.SecretLookupResponse{Nonce: signed.Nonce, Box: box})
	return nil
}

// reply publishes an answer on the requesting agent's subject.
func (h *SecretRequestHandler) reply(ctx context.Context, agentID, nonce string, resp *agent.SecretLookupResponse) {
	body, err := json.Marshal(resp)
	if err != nil {
		h.log.Error("controlplane: secret response marshal", "agent_id", agentID, "err", err)
		return
	}
	env := envelope.New(body, h.subjects.Prefix(), envelope.WithCorrelationID(nonce))
	if err := h.publisher.PublishEnvelope(ctx, h.subjects.SecretResponse(agentID), env); err != nil {
		h.log.Error("controlplane: secret response publish", "agent_id", agentID, "err", err)
	}
}

// secretValue selects one field of a secret's data.
func secretValue(secret *secrets.Secret, key string) (string, error) {
	if secret == nil || len(secret.Data) == 0 {
		return "", errors.New("secret carries no data")
	}
	if key == "" {
		if len(secret.Data) != 1 {
			// Guessing among several fields would sometimes return the
			// wrong credential, which is worse than refusing.
			return "", fmt.Errorf("secret has %d fields; name one with a key", len(secret.Data))
		}
		for _, v := range secret.Data {
			return stringifySecret(v)
		}
	}
	v, ok := secret.Data[key]
	if !ok {
		return "", fmt.Errorf("secret has no field %q", key)
	}
	return stringifySecret(v)
}

// stringifySecret renders a secret field as text. Only scalars: a
// structured value rendered into a config file would leak Go's
// formatting of it, which is not what anyone meant.
func stringifySecret(v any) (string, error) {
	switch t := v.(type) {
	case string:
		return t, nil
	case []byte:
		return string(t), nil
	case bool:
		return fmt.Sprintf("%t", t), nil
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", t), nil
	case float32, float64:
		return fmt.Sprintf("%v", t), nil
	default:
		return "", fmt.Errorf("secret field is %T, not a scalar", v)
	}
}

// leafFromChainPEM pulls the leaf out of a verified chain.
func leafFromChainPEM(chainPEM string) (*x509.Certificate, error) {
	block, _ := pem.Decode([]byte(chainPEM))
	if block == nil {
		return nil, errors.New("controlplane: certificate chain is not PEM")
	}
	return x509.ParseCertificate(block.Bytes)
}
