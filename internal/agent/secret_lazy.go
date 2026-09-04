// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"
	"crypto"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// LazySecretClient builds a SecretClient the first time one is needed.
//
// It exists because of an ordering problem the agent binary cannot
// avoid: on a first boot the agent has no credential until bootstrap
// delivers one, and bootstrap completes asynchronously after Start.
// Constructing the real client eagerly would permanently disable
// secret rendering on exactly the run that enrolled the host.
//
// Deferring to first use also means an agent whose credential is
// replaced on disk picks up the new one on the next process start
// without special handling, and one that never renders a secret never
// subscribes at all.
type LazySecretClient struct {
	store    *CredentialStore
	nats     NATSClient
	subjects Subjects
	timeout  time.Duration
	log      *slog.Logger

	mu     sync.Mutex
	client *SecretClient
}

// LazySecretClientConfig constructs a LazySecretClient.
type LazySecretClientConfig struct {
	Store    *CredentialStore
	NATS     NATSClient
	Subjects Subjects
	Timeout  time.Duration
	Logger   *slog.Logger
}

// NewLazySecretClient validates cfg. It does not touch the credential
// store: a missing credential at construction time is normal.
func NewLazySecretClient(cfg LazySecretClientConfig) (*LazySecretClient, error) {
	if cfg.Store == nil {
		return nil, errors.New("agent: lazy secret client: Store is required")
	}
	if cfg.NATS == nil {
		return nil, errors.New("agent: lazy secret client: NATS is required")
	}
	if cfg.Subjects == nil {
		return nil, errors.New("agent: lazy secret client: Subjects is required")
	}
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	return &LazySecretClient{
		store: cfg.Store, nats: cfg.NATS, subjects: cfg.Subjects,
		timeout: cfg.Timeout, log: log,
	}, nil
}

// Lookup resolves one secret, building the underlying client if this
// is the first call.
func (l *LazySecretClient) Lookup(ctx context.Context, path, key string) (string, error) {
	client, err := l.resolve()
	if err != nil {
		return "", err
	}
	return client.Lookup(ctx, path, key)
}

// Stop tears down the underlying client if one was ever built.
func (l *LazySecretClient) Stop() error {
	l.mu.Lock()
	client := l.client
	l.client = nil
	l.mu.Unlock()
	if client == nil {
		return nil
	}
	return client.Stop()
}

// resolve returns the client, constructing it once.
func (l *LazySecretClient) resolve() (*SecretClient, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.client != nil {
		return l.client, nil
	}

	creds, err := l.store.Load()
	if errors.Is(err, ErrNoCredentials) {
		return nil, errors.New("agent: secret lookup: this agent has no credential yet; " +
			"it must complete bootstrap before it can read secrets")
	}
	if err != nil {
		return nil, fmt.Errorf("agent: secret lookup: %w", err)
	}
	signer, err := NewSVIDSigner(creds)
	if errors.Is(err, ErrNoSVID) {
		// An API-key-only credential means the control plane runs with
		// identity disabled. Say that, rather than reporting a signing
		// failure the operator would go looking for in the agent.
		return nil, errors.New("agent: secret lookup: this agent has no SVID; " +
			"the control plane must run with identity.enabled to authorize per-agent secret reads")
	}
	if err != nil {
		return nil, fmt.Errorf("agent: secret lookup: %w", err)
	}
	key, err := privateKeyFromPEM(creds.PrivateKeyPEM)
	if err != nil {
		return nil, err
	}

	client, err := NewSecretClient(SecretClientConfig{
		NATS: l.nats, Subjects: l.subjects, Signer: signer, Key: key,
		Timeout: l.timeout, Logger: l.log,
	})
	if err != nil {
		return nil, err
	}
	if err := client.Start(); err != nil {
		return nil, err
	}
	l.client = client
	return client, nil
}

// privateKeyFromPEM decodes a PKCS#8 private key.
func privateKeyFromPEM(keyPEM string) (crypto.PrivateKey, error) {
	block, _ := pem.Decode([]byte(keyPEM))
	if block == nil {
		return nil, errors.New("agent: secret lookup: private key is not PEM")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("agent: secret lookup: parse private key: %w", err)
	}
	return key, nil
}
