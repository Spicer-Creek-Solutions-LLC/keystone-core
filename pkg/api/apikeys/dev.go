// SPDX-License-Identifier: Apache-2.0

package apikeys

import (
	"context"
	"fmt"
	"time"

	"go.keystone-core.io/keystone-core/internal/state"
)

// DevKeyName is the canonical Name used for the auto-generated
// dev-mode admin key. EnsureDevKey looks for an existing record with
// this name and only generates a new key when absent.
//
// Operators may rename, delete, or replace this key via the standard
// /api/v1/apikeys CRUD endpoints — the EnsureDevKey contract is
// "ensure SOME key with this Name exists," not "always preserve the
// originally-generated one."
const DevKeyName = "dev-default"

// EnsureDevKey ensures a default admin-role API key exists in store.
//
// On first invocation against a store that has no record with
// Name=DevKeyName, it generates a fresh admin-role key, persists the
// hash, and returns the cleartext + generated=true. The cleartext
// MUST be surfaced to the operator immediately and never persisted
// — by design, subsequent invocations cannot recover it.
//
// On subsequent invocations (existing dev key found), returns
// ("", false, nil). The cmd binary's dev-mode bootstrap relies on
// this idempotence so repeat runs don't spam new keys.
//
// Caller is responsible for the loud-warning log line when
// generated == true.
func EnsureDevKey(ctx context.Context, store state.APIKeyStore) (cleartext string, generated bool, err error) {
	// APIKeyFilter has no name predicate at v1.0; list everything and
	// match. Dev stores have a handful of keys, so the cost is fine.
	existing, err := store.ListAPIKeys(ctx, state.APIKeyFilter{})
	if err != nil {
		return "", false, fmt.Errorf("apikeys: list for dev-key check: %w", err)
	}
	for _, k := range existing {
		if k.Name == DevKeyName {
			return "", false, nil
		}
	}

	gen, err := Generate(DevKeyName, "admin", time.Time{})
	if err != nil {
		return "", false, fmt.Errorf("apikeys: generate dev key: %w", err)
	}
	if err := store.CreateAPIKey(ctx, gen.Record()); err != nil {
		return "", false, fmt.Errorf("apikeys: persist dev key: %w", err)
	}
	return gen.Cleartext, true, nil
}
