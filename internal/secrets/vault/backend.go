// SPDX-License-Identifier: Apache-2.0

package vault

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	vaultapi "github.com/hashicorp/vault/api"

	"go.keystone-core.io/keystone-core/internal/secrets"
)

// Backend is the HashiCorp Vault [secrets.SecretBackend]. Composes
// the validated [Config], an authenticated [*vaultapi.Client], and a
// background [tokenRenewer]. All operational methods route through
// the helpers in `kv.go` and `dynamic.go`.
//
// Concurrency: [vaultapi.Client] is safe for concurrent use; the
// backend itself only locks for lifecycle transitions.
type Backend struct {
	cfg    Config
	name   string
	client *vaultapi.Client

	renewer *tokenRenewer

	mu      sync.Mutex
	started bool
	stopped bool
}

// NewBackend validates the config + builds the (unauthenticated) Vault
// client. The backend isn't live until [Backend.Start] runs.
func NewBackend(cfg Config) (*Backend, error) {
	validated, err := cfg.validate()
	if err != nil {
		return nil, err
	}
	client, err := newClient(validated)
	if err != nil {
		return nil, err
	}
	if validated.TLS.Insecure {
		validated.Logger.LogAttrs(context.Background(), slog.LevelWarn,
			"vault: TLS verification is disabled (production deployments should pin a CA)")
	}
	return &Backend{
		cfg:    validated,
		name:   validated.Name,
		client: client,
	}, nil
}

// Name returns the backend's operator-facing name.
func (b *Backend) Name() string { return b.name }

// Capabilities reports the v1.0 Vault backend's surface: KV + list +
// dynamic + lease renew/revoke + transit.
func (b *Backend) Capabilities() []secrets.BackendCapability {
	return []secrets.BackendCapability{
		secrets.CapKV,
		secrets.CapList,
		secrets.CapDynamic,
		secrets.CapLeaseRenew,
		secrets.CapLeaseRevoke,
		secrets.CapTransit,
	}
}

// Start authenticates against Vault, sets the resulting token on the
// client, and spawns the renewer when the token is renewable.
func (b *Backend) Start(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.stopped {
		return fmt.Errorf("%w: vault backend: cannot Start after Stop", secrets.ErrInvalidBackend)
	}
	if b.started {
		return fmt.Errorf("%w: vault backend: already started", secrets.ErrInvalidBackend)
	}

	result, err := authenticate(ctx, b.client, b.cfg.Auth)
	if err != nil {
		return err
	}
	b.client.SetToken(result.Token)

	b.cfg.Logger.LogAttrs(ctx, slog.LevelInfo, "vault backend: authenticated",
		slog.String("backend", b.name),
		slog.String("address", b.cfg.Address),
		slog.String("namespace", b.cfg.Namespace),
		slog.String("auth_method", b.cfg.Auth.Method),
		slog.Int("token_ttl_sec", result.TTLSec),
		slog.Bool("renewable", result.Renewable),
	)

	if result.Renewable && result.TTLSec > 0 {
		b.renewer = newTokenRenewer(b.client, b.cfg)
		if err := b.renewer.start(ctx, result.TTLSec); err != nil {
			return err
		}
	}

	b.started = true
	return nil
}

// Stop cancels the renewer and issues a best-effort
// `auth/token/revoke-self` so Vault's audit trail stays clean.
// Idempotent.
func (b *Backend) Stop(ctx context.Context) error {
	b.mu.Lock()
	if b.stopped {
		b.mu.Unlock()
		return nil
	}
	b.stopped = true
	renewer := b.renewer
	started := b.started
	b.mu.Unlock()

	if renewer != nil {
		if err := renewer.stop(ctx); err != nil {
			b.cfg.Logger.LogAttrs(ctx, slog.LevelWarn, "vault: renewer stop error",
				slog.String("err", err.Error()),
			)
		}
	}
	if started {
		if err := b.client.Auth().Token().RevokeSelfWithContext(ctx, ""); err != nil {
			b.cfg.Logger.LogAttrs(ctx, slog.LevelWarn, "vault: revoke-self failed (continuing)",
				slog.String("err", err.Error()),
			)
		}
	}
	return nil
}

// Health round-trips `sys/health` and, when present, reports whether
// the renewer's last attempt succeeded.
func (b *Backend) Health(ctx context.Context) error {
	b.mu.Lock()
	started := b.started && !b.stopped
	renewer := b.renewer
	b.mu.Unlock()
	if !started {
		return secrets.ErrBackendNotStarted
	}
	if renewer != nil && !renewer.isHealthy() {
		return fmt.Errorf("%w: vault: token renewer is unhealthy (last renewal failed)", secrets.ErrInvalidBackend)
	}
	if _, err := b.client.Sys().HealthWithContext(ctx); err != nil {
		return translateError("health", "", err)
	}
	return nil
}

// GetSecret reads a KV secret.
func (b *Backend) GetSecret(ctx context.Context, req secrets.GetSecretRequest) (*secrets.Secret, error) {
	if err := b.ensureStarted(); err != nil {
		return nil, err
	}
	return b.kvGet(ctx, req)
}

// WriteSecret writes a KV secret. CAS is honored on KV v2 mounts and
// rejected on KV v1 mounts (where it would silently lie).
func (b *Backend) WriteSecret(ctx context.Context, req secrets.WriteSecretRequest) (*secrets.Secret, error) {
	if err := b.ensureStarted(); err != nil {
		return nil, err
	}
	return b.kvWrite(ctx, req)
}

// ListSecrets enumerates keys under req.Prefix.
func (b *Backend) ListSecrets(ctx context.Context, req secrets.ListSecretsRequest) (*secrets.ListSecretsResponse, error) {
	if err := b.ensureStarted(); err != nil {
		return nil, err
	}
	return b.kvList(ctx, req)
}

// DeleteSecret removes a KV secret. On KV v2, `req.Destroy=true`
// hard-destroys; `req.Version` non-zero deletes a specific version.
func (b *Backend) DeleteSecret(ctx context.Context, req secrets.DeleteSecretRequest) error {
	if err := b.ensureStarted(); err != nil {
		return err
	}
	return b.kvDelete(ctx, req)
}

// IssueDynamicSecret invokes a Vault dynamic engine at req.Path.
func (b *Backend) IssueDynamicSecret(ctx context.Context, req secrets.IssueDynamicSecretRequest) (*secrets.Secret, error) {
	if err := b.ensureStarted(); err != nil {
		return nil, err
	}
	return b.issueDynamicSecret(ctx, req)
}

// RenewLease extends a Vault lease.
func (b *Backend) RenewLease(ctx context.Context, req secrets.RenewLeaseRequest) (*secrets.LeaseInfo, error) {
	if err := b.ensureStarted(); err != nil {
		return nil, err
	}
	return b.renewLease(ctx, req)
}

// RevokeLease tears a Vault lease down (idempotent).
func (b *Backend) RevokeLease(ctx context.Context, req secrets.RevokeLeaseRequest) error {
	if err := b.ensureStarted(); err != nil {
		return err
	}
	return b.revokeLease(ctx, req)
}

func (b *Backend) ensureStarted() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.started || b.stopped {
		return secrets.ErrBackendNotStarted
	}
	return nil
}

// internalClient exposes the underlying Vault client for the
// gated integration test only. Not part of the public surface.
func (b *Backend) internalClient() *vaultapi.Client {
	return b.client
}

// Compile-time interface assertions — the Vault backend satisfies
// both the KV/lease surface and the transit surface.
var (
	_ secrets.SecretBackend  = (*Backend)(nil)
	_ secrets.TransitBackend = (*Backend)(nil)
)
