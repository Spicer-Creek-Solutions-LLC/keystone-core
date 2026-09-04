// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"
	"crypto"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"go.keystone-core.io/keystone-core/internal/sealed"
	"go.keystone-core.io/keystone-core/pkg/envelope"
)

// ErrSecretDenied means the control plane authenticated this agent and
// refused the path anyway. Callers surface it as a policy problem, not
// a transport one.
var ErrSecretDenied = errors.New("agent: secret lookup denied")

// SecretClient asks the control plane for a secret value, once per
// use, and never keeps the answer.
//
// No cache, deliberately. A cached secret outlives the authorization
// that produced it: revoking an agent's grant would not take effect
// until the entry expired, and the value would sit in agent memory
// long after the render that needed it. Every use asks again.
type SecretClient struct {
	nats     NATSClient
	subjects Subjects
	signer   *SVIDSigner
	key      crypto.PrivateKey
	timeout  time.Duration
	log      *slog.Logger

	mu      sync.Mutex
	waiters map[string]chan *SecretLookupResponse
	sub     Subscription
}

// SecretClientConfig constructs a SecretClient. Signer and Key come
// from the agent's stored credentials, so an agent without an SVID
// cannot build one -- which is correct: with no way to prove who it
// is, and no key to seal a reply to, there is nothing for the control
// plane to answer.
type SecretClientConfig struct {
	NATS     NATSClient
	Subjects Subjects
	Signer   *SVIDSigner
	// Key is the private half of the SVID, used to open sealed
	// replies.
	Key     crypto.PrivateKey
	Timeout time.Duration
	Logger  *slog.Logger
}

// NewSecretClient validates cfg and returns an unstarted client.
func NewSecretClient(cfg SecretClientConfig) (*SecretClient, error) {
	if cfg.NATS == nil {
		return nil, errors.New("agent: secret client: NATS is required")
	}
	if cfg.Subjects == nil {
		return nil, errors.New("agent: secret client: Subjects is required")
	}
	if cfg.Signer == nil {
		return nil, errors.New("agent: secret client: Signer is required")
	}
	if cfg.Key == nil {
		return nil, errors.New("agent: secret client: Key is required")
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = SecretLookupTimeout * time.Second
	}
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	return &SecretClient{
		nats:     cfg.NATS,
		subjects: cfg.Subjects,
		signer:   cfg.Signer,
		key:      cfg.Key,
		timeout:  timeout,
		log:      log,
		waiters:  map[string]chan *SecretLookupResponse{},
	}, nil
}

// Start subscribes to this agent's secret-response subject.
func (c *SecretClient) Start() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sub != nil {
		return nil
	}
	subject := c.subjects.SecretResponse(c.signer.AgentID())
	sub, err := c.nats.Subscribe(subject, c.handleResponse)
	if err != nil {
		return fmt.Errorf("agent: secret client subscribe %q: %w", subject, err)
	}
	c.sub = sub
	return nil
}

// Stop unsubscribes. Safe to call before Start.
func (c *SecretClient) Stop() error {
	c.mu.Lock()
	sub := c.sub
	c.sub = nil
	c.mu.Unlock()
	if sub == nil {
		return nil
	}
	if err := sub.Unsubscribe(); err != nil {
		return fmt.Errorf("agent: secret client unsubscribe: %w", err)
	}
	return nil
}

// Lookup fetches one secret value.
//
// The reply channel is registered BEFORE the request is published. The
// control plane can answer faster than this goroutine reaches its
// select, and an answer that arrives with no waiter registered is an
// answer thrown away -- the caller would then wait out the full
// timeout for a reply that already came and went.
func (c *SecretClient) Lookup(ctx context.Context, path, key string) (string, error) {
	if path == "" {
		return "", errors.New("agent: secret lookup: path is required")
	}
	payload, err := json.Marshal(SecretLookupRequest{Path: path, Key: key})
	if err != nil {
		return "", fmt.Errorf("agent: secret lookup marshal: %w", err)
	}
	signed, err := c.signer.Sign(payload)
	if err != nil {
		return "", fmt.Errorf("agent: secret lookup sign: %w", err)
	}
	body, err := json.Marshal(signed)
	if err != nil {
		return "", fmt.Errorf("agent: secret lookup marshal request: %w", err)
	}

	ch := make(chan *SecretLookupResponse, 1)
	c.mu.Lock()
	c.waiters[signed.Nonce] = ch
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.waiters, signed.Nonce)
		c.mu.Unlock()
	}()

	env := envelope.New(body, c.subjects.Prefix(), envelope.WithMessageID(signed.Nonce))
	if err := c.nats.PublishEnvelope(ctx, c.subjects.SecretRequest(), env); err != nil {
		return "", fmt.Errorf("agent: secret lookup publish: %w", err)
	}

	timer := time.NewTimer(c.timeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-timer.C:
		return "", fmt.Errorf("agent: secret lookup %q: timed out after %s", path, c.timeout)
	case resp := <-ch:
		return c.open(path, signed.Nonce, resp)
	}
}

// open unseals a response.
func (c *SecretClient) open(path, nonce string, resp *SecretLookupResponse) (string, error) {
	if resp.Denied {
		reason := resp.Error
		if reason == "" {
			reason = "not granted"
		}
		return "", fmt.Errorf("%w: %q: %s", ErrSecretDenied, path, reason)
	}
	if resp.Error != "" {
		return "", fmt.Errorf("agent: secret lookup %q: %s", path, resp.Error)
	}
	if resp.Box == nil {
		return "", fmt.Errorf("agent: secret lookup %q: response carried no value", path)
	}
	plaintext, err := sealed.Open(c.key, resp.Box, secretAAD(c.signer.AgentID(), nonce))
	if err != nil {
		return "", fmt.Errorf("agent: secret lookup %q: %w", path, err)
	}
	return string(plaintext), nil
}

// handleResponse routes a reply to the waiting Lookup.
func (c *SecretClient) handleResponse(_ context.Context, subject string, env envelope.Envelope) error {
	var resp SecretLookupResponse
	if err := json.Unmarshal(env.Payload, &resp); err != nil {
		c.log.Warn("agent: secret response unmarshal", "subject", subject, "err", err)
		return nil
	}
	c.mu.Lock()
	ch, ok := c.waiters[resp.Nonce]
	c.mu.Unlock()
	if !ok {
		// A late reply after the caller gave up, or one for a nonce we
		// never issued. Nothing to do; the sealed box is unreadable to
		// anyone but us anyway.
		return nil
	}
	select {
	case ch <- &resp:
	default:
		// Buffered channel already holds a reply; a duplicate delivery.
	}
	return nil
}

// secretAAD binds a sealed reply to the agent and the request it
// answers. Shared with the control plane so both sides compute the
// same associated data -- a mismatch would fail every open with no
// indication of why.
func secretAAD(agentID, nonce string) []byte {
	return []byte(agentID + "|" + nonce)
}

// SecretAAD is the exported form for the control-plane sealer.
func SecretAAD(agentID, nonce string) []byte { return secretAAD(agentID, nonce) }
