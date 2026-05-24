// SPDX-License-Identifier: Apache-2.0

package apikeys

import (
	"context"
	"errors"
	"fmt"

	"go.keystone-core.io/keystone-core/internal/state"
	"go.keystone-core.io/keystone-core/pkg/api/auth"
)

// StoreVerifier adapts a state.APIKeyStore to auth.KeyVerifier so the
// task-4 APIKeyAuthenticator can validate keys against this storage
// backend.
//
// Verification flow:
//  1. SHA-256 hash the inbound cleartext via auth.HashAPIKey.
//  2. Look up the row by hash via APIKeyStore.GetAPIKeyByHash.
//  3. Parse the stored role string into an auth.Role.
//  4. Return a *auth.VerifiedKey for the chain to construct a
//     Principal from.
//
// Unknown hashes / not-found rows surface as auth.ErrInvalidCredentials.
type StoreVerifier struct {
	store state.APIKeyStore
}

// NewStoreVerifier returns a StoreVerifier backed by store.
func NewStoreVerifier(store state.APIKeyStore) *StoreVerifier {
	return &StoreVerifier{store: store}
}

// VerifyKey implements auth.KeyVerifier.
func (v *StoreVerifier) VerifyKey(ctx context.Context, cleartext string) (*auth.VerifiedKey, error) {
	if cleartext == "" {
		return nil, auth.ErrCredentialsNotFound
	}

	hash := auth.HashAPIKey(cleartext)
	rec, err := v.store.GetAPIKeyByHash(ctx, hash)
	if err != nil {
		if errors.Is(err, state.ErrNotFound) {
			return nil, auth.ErrInvalidCredentials
		}
		return nil, err
	}

	role, err := auth.ParseRole(rec.Role)
	if err != nil {
		return nil, fmt.Errorf("%w: stored role %q invalid", auth.ErrInvalidCredentials, rec.Role)
	}
	if role == auth.RoleNone {
		return nil, fmt.Errorf("%w: stored role is empty", auth.ErrInvalidCredentials)
	}

	return &auth.VerifiedKey{
		ID:        rec.ID,
		Name:      rec.Name,
		Role:      role,
		ExpiresAt: rec.ExpiresAt,
	}, nil
}
