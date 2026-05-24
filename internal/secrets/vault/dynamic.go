// SPDX-License-Identifier: Apache-2.0

package vault

import (
	"context"
	"errors"
	"fmt"
	"time"

	vaultapi "github.com/hashicorp/vault/api"

	"go.keystone-core.io/keystone-core/internal/secrets"
)

// issueDynamicSecret invokes a Vault dynamic-secret engine at
// req.Path. The engine path is the full Vault path including the
// engine's operation segment — e.g. `database/creds/app`,
// `pki/issue/role`, `ssh/creds/user`. The backend is engine-agnostic
// at v1.0; operators encode the engine semantics in the path.
//
// req.TTL / req.MaxTTL / req.Params are merged into the Vault request
// body. The response carries the dynamic credential plus a Vault
// lease ID + TTL + renewable bit that the broker records in its
// [secrets.LeaseDirectory].
func (b *Backend) issueDynamicSecret(ctx context.Context, req secrets.IssueDynamicSecretRequest) (*secrets.Secret, error) {
	if req.Path == "" {
		return nil, errInvalid("dynamic: Path is required (e.g. \"database/creds/app\")")
	}
	body := make(map[string]any, len(req.Params)+2)
	for k, v := range req.Params {
		body[k] = v
	}
	// Some engines (e.g. PKI, certain DB plugins) accept `ttl` /
	// `max_ttl` directly in the issue request. Others ignore them.
	// Passing-through is correct + harmless.
	if req.TTL > 0 {
		body["ttl"] = formatVaultDuration(req.TTL)
	}
	if req.MaxTTL > 0 {
		body["max_ttl"] = formatVaultDuration(req.MaxTTL)
	}

	var vaultSecret *vaultapi.Secret
	var err error
	if len(body) == 0 {
		// Some engines reject an empty POST body; use the
		// no-payload Read shape which Vault interprets identically
		// for read-style issue endpoints (e.g. `database/creds/*`).
		vaultSecret, err = b.client.Logical().ReadWithContext(ctx, req.Path)
	} else {
		vaultSecret, err = b.client.Logical().WriteWithContext(ctx, req.Path, body)
	}
	if err != nil {
		return nil, translateError("dynamic issue", req.Path, err)
	}
	if vaultSecret == nil {
		return nil, fmt.Errorf("%w: vault: dynamic issue %q: response had no secret data", secrets.ErrInvalidBackend, req.Path)
	}

	out := &secrets.Secret{
		Path:          req.Path,
		Data:          vaultSecret.Data,
		LeaseID:       vaultSecret.LeaseID,
		LeaseDuration: time.Duration(vaultSecret.LeaseDuration) * time.Second,
		Renewable:     vaultSecret.Renewable,
		CreatedAt:     b.cfg.Clock(),
	}
	return out, nil
}

// renewLease invokes Vault's `sys/leases/renew` endpoint.
func (b *Backend) renewLease(ctx context.Context, req secrets.RenewLeaseRequest) (*secrets.LeaseInfo, error) {
	if req.LeaseID == "" {
		return nil, errInvalid("renew: LeaseID is required")
	}
	body := map[string]any{"lease_id": req.LeaseID}
	if req.Increment > 0 {
		body["increment"] = int(req.Increment / time.Second)
	}
	vaultSecret, err := b.client.Logical().WriteWithContext(ctx, "sys/leases/renew", body)
	if err != nil {
		return nil, translateError("lease renew", req.LeaseID, err)
	}
	if vaultSecret == nil {
		return nil, fmt.Errorf("%w: vault: lease renew %q: response had no data", secrets.ErrInvalidBackend, req.LeaseID)
	}

	now := b.cfg.Clock()
	duration := time.Duration(vaultSecret.LeaseDuration) * time.Second
	return &secrets.LeaseInfo{
		ID:            vaultSecret.LeaseID,
		Backend:       b.name,
		Duration:      duration,
		ExpiresAt:     now.Add(duration),
		Renewable:     vaultSecret.Renewable,
		State:         secrets.LeaseStateActive,
		LastRenewedAt: now,
	}, nil
}

// revokeLease invokes Vault's `sys/leases/revoke` endpoint.
// Idempotent: revoking an unknown lease returns nil (Vault may
// itself 204 on a missing lease, or 400 with "not found" — both map
// to "the credential is gone, which is what the caller wanted").
func (b *Backend) revokeLease(ctx context.Context, req secrets.RevokeLeaseRequest) error {
	if req.LeaseID == "" {
		return errInvalid("revoke: LeaseID is required")
	}
	body := map[string]any{"lease_id": req.LeaseID}
	if req.Force {
		body["sync"] = true
	}
	_, err := b.client.Logical().WriteWithContext(ctx, "sys/leases/revoke", body)
	if err == nil {
		return nil
	}
	translated := translateError("lease revoke", req.LeaseID, err)
	// Idempotency: a "not found" revoke is success from the caller's POV.
	if isLeaseGone(translated) {
		return nil
	}
	return translated
}

func isLeaseGone(err error) bool {
	if err == nil {
		return false
	}
	// errors.Is checks the chain; both ErrLeaseNotFound and
	// ErrLeaseExpired count as "the credential is no longer valid",
	// which is the post-condition of a successful revoke.
	return errors.Is(err, secrets.ErrLeaseNotFound) || errors.Is(err, secrets.ErrLeaseExpired)
}

// formatVaultDuration emits Vault's preferred duration string —
// seconds as a plain integer + "s", which the API accepts uniformly
// across every engine.
func formatVaultDuration(d time.Duration) string {
	return fmt.Sprintf("%ds", int64(d/time.Second))
}
