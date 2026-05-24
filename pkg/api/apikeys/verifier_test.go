// SPDX-License-Identifier: Apache-2.0

package apikeys_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/state"
	"go.keystone-core.io/keystone-core/pkg/api/apikeys"
	"go.keystone-core.io/keystone-core/pkg/api/auth"
)

// fakeAPIKeyStore is the minimum surface StoreVerifier needs.
type fakeAPIKeyStore struct {
	byHash      map[string]*state.APIKeyRecord
	getByHashEr error
}

func (f *fakeAPIKeyStore) CreateAPIKey(_ context.Context, k *state.APIKeyRecord) error {
	if f.byHash == nil {
		f.byHash = map[string]*state.APIKeyRecord{}
	}
	f.byHash[k.KeyHash] = k
	return nil
}

func (f *fakeAPIKeyStore) GetAPIKey(_ context.Context, id string) (*state.APIKeyRecord, error) {
	for _, r := range f.byHash {
		if r.ID == id {
			return r, nil
		}
	}
	return nil, state.ErrNotFound
}

func (f *fakeAPIKeyStore) GetAPIKeyByHash(_ context.Context, hash string) (*state.APIKeyRecord, error) {
	if f.getByHashEr != nil {
		return nil, f.getByHashEr
	}
	r, ok := f.byHash[hash]
	if !ok {
		return nil, state.ErrNotFound
	}
	return r, nil
}

func (f *fakeAPIKeyStore) ListAPIKeys(_ context.Context, _ state.APIKeyFilter) ([]*state.APIKeyRecord, error) {
	out := make([]*state.APIKeyRecord, 0, len(f.byHash))
	for _, k := range f.byHash {
		out = append(out, k)
	}
	return out, nil
}

func (f *fakeAPIKeyStore) UpdateAPIKeyLastUsed(_ context.Context, _ string, _ time.Time) error {
	return nil
}

func (f *fakeAPIKeyStore) DeleteAPIKey(_ context.Context, id string) error {
	for h, r := range f.byHash {
		if r.ID == id {
			delete(f.byHash, h)
			return nil
		}
	}
	return state.ErrNotFound
}

func TestStoreVerifier_VerifyKey_Success(t *testing.T) {
	g, err := apikeys.Generate("ops-key", "operator", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	store := &fakeAPIKeyStore{}
	if err := store.CreateAPIKey(context.Background(), g.Record()); err != nil {
		t.Fatal(err)
	}

	v := apikeys.NewStoreVerifier(store)
	got, err := v.VerifyKey(context.Background(), g.Cleartext)
	if err != nil {
		t.Fatalf("VerifyKey: %v", err)
	}
	if got.ID != g.ID || got.Name != g.Name || got.Role != auth.RoleOperator {
		t.Errorf("VerifiedKey: %+v", got)
	}
}

func TestStoreVerifier_UnknownCleartextRejected(t *testing.T) {
	v := apikeys.NewStoreVerifier(&fakeAPIKeyStore{})
	_, err := v.VerifyKey(context.Background(), "some-fake-key-no-match-in-store")
	if !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Errorf("err = %v, want ErrInvalidCredentials", err)
	}
}

func TestStoreVerifier_EmptyCleartext(t *testing.T) {
	v := apikeys.NewStoreVerifier(&fakeAPIKeyStore{})
	_, err := v.VerifyKey(context.Background(), "")
	if !errors.Is(err, auth.ErrCredentialsNotFound) {
		t.Errorf("err = %v, want ErrCredentialsNotFound", err)
	}
}

func TestStoreVerifier_StoreErrorPropagates(t *testing.T) {
	custom := errors.New("custom: db unavailable")
	v := apikeys.NewStoreVerifier(&fakeAPIKeyStore{getByHashEr: custom})
	_, err := v.VerifyKey(context.Background(), "x")
	if err != custom {
		t.Errorf("err = %v, want custom (verbatim)", err)
	}
}

func TestStoreVerifier_BadStoredRoleRejected(t *testing.T) {
	store := &fakeAPIKeyStore{
		byHash: map[string]*state.APIKeyRecord{
			auth.HashAPIKey("ct"): {
				ID:   "k-1",
				Role: "superuser", // not a known role
			},
		},
	}
	v := apikeys.NewStoreVerifier(store)
	_, err := v.VerifyKey(context.Background(), "ct")
	if !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Errorf("err = %v, want ErrInvalidCredentials", err)
	}
}

func TestStoreVerifier_EmptyStoredRoleRejected(t *testing.T) {
	store := &fakeAPIKeyStore{
		byHash: map[string]*state.APIKeyRecord{
			auth.HashAPIKey("ct"): {
				ID:   "k-1",
				Role: "", // empty -> RoleNone after parse
			},
		},
	}
	v := apikeys.NewStoreVerifier(store)
	_, err := v.VerifyKey(context.Background(), "ct")
	if !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Errorf("err = %v, want ErrInvalidCredentials", err)
	}
}

// Ensure StoreVerifier plugs into the auth chain as a KeyVerifier.
func TestStoreVerifier_SatisfiesKeyVerifier(t *testing.T) {
	var _ auth.KeyVerifier = (*apikeys.StoreVerifier)(nil)
}
