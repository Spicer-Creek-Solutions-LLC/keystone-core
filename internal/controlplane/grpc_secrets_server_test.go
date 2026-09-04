// SPDX-License-Identifier: Apache-2.0

package controlplane

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.keystone-core.io/keystone-core/internal/secrets"
	"go.keystone-core.io/keystone-core/internal/state"
	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

// secretsTestRig is the shared test fixture — file backend + broker
// + lease manager wired together against an in-memory state store.
type secretsTestRig struct {
	server *SecretsGRPCServer
	broker *secrets.Broker
	store  state.Store
}

func newSecretsRig(t *testing.T) *secretsTestRig {
	t.Helper()
	store, err := state.NewStore(&state.Config{
		Backend: state.BackendSQLite,
		SQLite:  state.SQLiteConfig{Path: filepath.Join(t.TempDir(), "store.db")},
	})
	if err != nil {
		t.Fatalf("state.NewStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	// In-memory test backend with full capability set.
	be := &grpcTestBackend{
		name: "test",
		caps: []secrets.BackendCapability{
			secrets.CapKV, secrets.CapList,
			secrets.CapDynamic, secrets.CapLeaseRenew, secrets.CapLeaseRevoke,
		},
		secrets: make(map[string]*secrets.Secret),
		leases:  make(map[string]*secrets.LeaseInfo),
	}

	lm, err := secrets.NewLeaseManager(secrets.LeaseManagerConfig{Store: store})
	if err != nil {
		t.Fatalf("NewLeaseManager: %v", err)
	}

	router, err := secrets.NewRouter([]secrets.Route{
		{Prefix: "kv/", Backend: "test"},
		{Prefix: "database/", Backend: "test"},
	})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	broker, err := secrets.NewBroker(secrets.BrokerConfig{
		Router:         router,
		Backends:       []secrets.SecretBackend{be},
		DefaultBackend: "test",
		LeaseDirectory: lm,
	})
	if err != nil {
		t.Fatalf("NewBroker: %v", err)
	}
	lm.SetRenewer(broker.RenewLease)

	server := NewSecretsGRPCServer(broker, &grpcTestTransit{}, lm)
	return &secretsTestRig{server: server, broker: broker, store: store}
}

func TestSecretsGRPC_WriteGet_RoundTrip(t *testing.T) {
	t.Parallel()
	r := newSecretsRig(t)
	ctx := context.Background()

	_, err := r.server.WriteSecret(ctx, &v1.WriteSecretRequest{
		Path: "kv/app/db",
		Data: map[string]string{"password": "hunter2"},
	})
	if err != nil {
		t.Fatalf("WriteSecret: %v", err)
	}

	resp, err := r.server.GetSecret(ctx, &v1.GetSecretRequest{Path: "kv/app/db"})
	if err != nil {
		t.Fatalf("GetSecret: %v", err)
	}
	if resp.GetData()["password"] != "hunter2" {
		t.Errorf("round-trip lost password: %#v", resp.GetData())
	}
}

func TestSecretsGRPC_GetSecret_NotFound(t *testing.T) {
	t.Parallel()
	r := newSecretsRig(t)
	_, err := r.server.GetSecret(context.Background(), &v1.GetSecretRequest{Path: "kv/missing"})
	if status.Code(err) != codes.NotFound {
		t.Errorf("err code = %v, want NotFound", status.Code(err))
	}
}

func TestSecretsGRPC_GetSecret_RequiresPath(t *testing.T) {
	t.Parallel()
	r := newSecretsRig(t)
	_, err := r.server.GetSecret(context.Background(), &v1.GetSecretRequest{})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("err code = %v, want InvalidArgument", status.Code(err))
	}
}

func TestSecretsGRPC_WriteSecret_LabelsAndTTLPropagate(t *testing.T) {
	t.Parallel()
	r := newSecretsRig(t)

	_, err := r.server.WriteSecret(context.Background(), &v1.WriteSecretRequest{
		Path:       "kv/x",
		Data:       map[string]string{"k": "v"},
		Labels:     map[string]string{"env": "prod"},
		TtlSeconds: 60,
	})
	if err != nil {
		t.Fatalf("WriteSecret: %v", err)
	}

	// Re-fetch from the backend directly to inspect metadata.
	be := r.broker
	got, err := be.GetSecret(context.Background(), secrets.GetSecretRequest{Path: "kv/x"})
	if err != nil {
		t.Fatalf("GetSecret: %v", err)
	}
	if got.Metadata["env"] != "prod" {
		t.Errorf("env label lost: %#v", got.Metadata)
	}
	if got.Metadata["ttl_seconds"] != "60" {
		t.Errorf("ttl_seconds metadata = %q, want 60", got.Metadata["ttl_seconds"])
	}
}

func TestSecretsGRPC_ListSecrets(t *testing.T) {
	t.Parallel()
	r := newSecretsRig(t)
	ctx := context.Background()

	for _, p := range []string{"kv/a", "kv/b", "kv/c"} {
		_, _ = r.server.WriteSecret(ctx, &v1.WriteSecretRequest{Path: p, Data: map[string]string{"k": "v"}})
	}

	resp, err := r.server.ListSecrets(ctx, &v1.ListSecretsRequest{PathPrefix: "kv/"})
	if err != nil {
		t.Fatalf("ListSecrets: %v", err)
	}
	if len(resp.GetSecrets()) != 3 {
		t.Errorf("secrets = %d, want 3", len(resp.GetSecrets()))
	}
}

func TestSecretsGRPC_DeleteSecret(t *testing.T) {
	t.Parallel()
	r := newSecretsRig(t)
	ctx := context.Background()

	_, _ = r.server.WriteSecret(ctx, &v1.WriteSecretRequest{Path: "kv/x", Data: map[string]string{"k": "v"}})

	if _, err := r.server.DeleteSecret(ctx, &v1.DeleteSecretRequest{Path: "kv/x"}); err != nil {
		t.Fatalf("DeleteSecret: %v", err)
	}
	_, err := r.server.GetSecret(ctx, &v1.GetSecretRequest{Path: "kv/x"})
	if status.Code(err) != codes.NotFound {
		t.Errorf("err code = %v, want NotFound", status.Code(err))
	}
}

func TestSecretsGRPC_LeaseFlow(t *testing.T) {
	t.Parallel()
	r := newSecretsRig(t)
	ctx := context.Background()

	// Issue a dynamic secret through the broker (the gRPC service
	// doesn't expose IssueDynamicSecret RPC — operators issue via
	// CLI). This populates the lease manager.
	issued, err := r.broker.IssueDynamicSecret(ctx, secrets.IssueDynamicSecretRequest{Path: "database/creds/app"})
	if err != nil {
		t.Fatalf("IssueDynamicSecret: %v", err)
	}
	if issued.LeaseID == "" {
		t.Fatalf("dynamic issue returned no LeaseID")
	}

	// Get the lease via gRPC.
	getResp, err := r.server.GetLease(ctx, &v1.GetLeaseRequest{LeaseId: issued.LeaseID})
	if err != nil {
		t.Fatalf("GetLease: %v", err)
	}
	if getResp.GetLease().GetId() != issued.LeaseID {
		t.Errorf("lease id mismatch: %q vs %q", getResp.GetLease().GetId(), issued.LeaseID)
	}

	// Renew.
	renewResp, err := r.server.RenewLease(ctx, &v1.RenewLeaseRequest{LeaseId: issued.LeaseID})
	if err != nil {
		t.Fatalf("RenewLease: %v", err)
	}
	if renewResp.GetLease() == nil {
		t.Errorf("RenewLease returned nil Lease")
	}

	// List.
	list, err := r.server.ListLeases(ctx, &v1.ListLeasesRequest{})
	if err != nil {
		t.Fatalf("ListLeases: %v", err)
	}
	if len(list.GetLeases()) < 1 {
		t.Errorf("ListLeases returned %d leases, want ≥1", len(list.GetLeases()))
	}

	// Revoke.
	if _, err := r.server.RevokeLease(ctx, &v1.RevokeLeaseRequest{LeaseId: issued.LeaseID}); err != nil {
		t.Fatalf("RevokeLease: %v", err)
	}

	// Post-revoke GetLease — store retains the row for audit but
	// LeaseDirectory.Lookup misses; broker's RevokeLease forgets the
	// directory entry. State-store-side it's actually deleted, so
	// GetLease should return NotFound.
	_, err = r.server.GetLease(ctx, &v1.GetLeaseRequest{LeaseId: issued.LeaseID})
	if status.Code(err) != codes.NotFound {
		t.Errorf("post-revoke GetLease code = %v, want NotFound", status.Code(err))
	}
}

func TestSecretsGRPC_GetLease_UnknownReturnsNotFound(t *testing.T) {
	t.Parallel()
	r := newSecretsRig(t)
	_, err := r.server.GetLease(context.Background(), &v1.GetLeaseRequest{LeaseId: "ghost"})
	if status.Code(err) != codes.NotFound {
		t.Errorf("err code = %v, want NotFound", status.Code(err))
	}
}

func TestSecretsGRPC_RenewLease_RequiresID(t *testing.T) {
	t.Parallel()
	r := newSecretsRig(t)
	_, err := r.server.RenewLease(context.Background(), &v1.RenewLeaseRequest{})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("err code = %v, want InvalidArgument", status.Code(err))
	}
}

func TestSecretsGRPC_TransitRoundTrip(t *testing.T) {
	t.Parallel()
	r := newSecretsRig(t)
	ctx := context.Background()

	enc, err := r.server.Encrypt(ctx, &v1.EncryptRequest{
		KeyName:   "k",
		Plaintext: []byte("hello"),
	})
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if len(enc.GetCiphertext()) == 0 {
		t.Errorf("Encrypt returned empty ciphertext")
	}

	dec, err := r.server.Decrypt(ctx, &v1.DecryptRequest{
		KeyName:    "k",
		Ciphertext: enc.GetCiphertext(),
	})
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if string(dec.GetPlaintext()) != "hello" {
		t.Errorf("Decrypt mismatch: %q", dec.GetPlaintext())
	}
}

func TestSecretsGRPC_Sign_Verify(t *testing.T) {
	t.Parallel()
	r := newSecretsRig(t)
	ctx := context.Background()

	sig, err := r.server.Sign(ctx, &v1.SignRequest{
		KeyName: "k",
		Message: []byte("p"),
	})
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	v, err := r.server.Verify(ctx, &v1.VerifyRequest{
		KeyName:   "k",
		Message:   []byte("p"),
		Signature: sig.GetSignature(),
	})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !v.GetValid() {
		t.Errorf("Verify on freshly-signed payload: Valid=false")
	}
}

func TestSecretsGRPC_TransitUnavailable(t *testing.T) {
	t.Parallel()
	// No Transit backend wired.
	s := NewSecretsGRPCServer(nil, nil, nil)
	_, err := s.Encrypt(context.Background(), &v1.EncryptRequest{KeyName: "k", Plaintext: []byte("x")})
	if status.Code(err) != codes.Unavailable {
		t.Errorf("Encrypt without Transit: code = %v, want Unavailable", status.Code(err))
	}
}

func TestSecretsGRPC_BrokerUnavailable(t *testing.T) {
	t.Parallel()
	s := NewSecretsGRPCServer(nil, nil, nil)
	_, err := s.GetSecret(context.Background(), &v1.GetSecretRequest{Path: "kv/x"})
	if status.Code(err) != codes.Unavailable {
		t.Errorf("GetSecret without Broker: code = %v, want Unavailable", status.Code(err))
	}
}

func TestSplitVaultSignAlgorithm(t *testing.T) {
	t.Parallel()
	cases := []struct {
		algo       string
		wantHash   string
		wantSigAlg string
	}{
		{"", "", ""},
		{"ed25519", "", ""},
		{"rsa-pss-sha256", "sha2-256", "pss"},
		{"rsa-pss-sha512", "sha2-512", "pss"},
		{"rsa-pkcs1v15-sha384", "sha2-384", "pkcs1v15"},
		{"custom-algo", "custom-algo", ""}, // pass-through
	}
	for _, tc := range cases {
		t.Run(tc.algo, func(t *testing.T) {
			t.Parallel()
			h, s := splitVaultSignAlgorithm(tc.algo)
			if h != tc.wantHash || s != tc.wantSigAlg {
				t.Errorf("split(%q) = (%q, %q), want (%q, %q)", tc.algo, h, s, tc.wantHash, tc.wantSigAlg)
			}
		})
	}
}

func TestSecretsErrToStatus(t *testing.T) {
	t.Parallel()
	cases := []struct {
		err  error
		want codes.Code
	}{
		{nil, codes.OK},
		{fmt.Errorf("%w: x", secrets.ErrSecretNotFound), codes.NotFound},
		{fmt.Errorf("%w: x", secrets.ErrLeaseNotFound), codes.NotFound},
		{fmt.Errorf("%w: x", secrets.ErrLeaseExpired), codes.FailedPrecondition},
		{fmt.Errorf("%w: x", secrets.ErrLeaseNotRenewable), codes.FailedPrecondition},
		{fmt.Errorf("%w: x", secrets.ErrBackendNotStarted), codes.Unavailable},
		{fmt.Errorf("%w: x", secrets.ErrInvalidBackend), codes.InvalidArgument},
		{errors.New("random"), codes.Internal},
	}
	for _, tc := range cases {
		got := secretsErrToStatus(tc.err)
		if got == nil && tc.want != codes.OK {
			t.Errorf("err=%v: got OK, want %v", tc.err, tc.want)
			continue
		}
		if got != nil && status.Code(got) != tc.want {
			t.Errorf("err=%v: code = %v, want %v", tc.err, status.Code(got), tc.want)
		}
	}
}

// ---- in-memory test backend + transit ---------------------------

type grpcTestBackend struct {
	mu      sync.Mutex
	name    string
	caps    []secrets.BackendCapability
	secrets map[string]*secrets.Secret
	leases  map[string]*secrets.LeaseInfo
	leaseID int
}

func (b *grpcTestBackend) Name() string                              { return b.name }
func (b *grpcTestBackend) Capabilities() []secrets.BackendCapability { return b.caps }
func (b *grpcTestBackend) Start(context.Context) error               { return nil }
func (b *grpcTestBackend) Stop(context.Context) error                { return nil }
func (b *grpcTestBackend) Health(context.Context) error              { return nil }

func (b *grpcTestBackend) GetSecret(_ context.Context, req secrets.GetSecretRequest) (*secrets.Secret, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	s, ok := b.secrets[req.Path]
	if !ok {
		return nil, fmt.Errorf("%w: %q", secrets.ErrSecretNotFound, req.Path)
	}
	return s, nil
}

func (b *grpcTestBackend) WriteSecret(_ context.Context, req secrets.WriteSecretRequest) (*secrets.Secret, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	s := &secrets.Secret{
		Path:     req.Path,
		Data:     req.Data,
		Metadata: req.Metadata,
		Version:  1,
	}
	b.secrets[req.Path] = s
	return s, nil
}

func (b *grpcTestBackend) ListSecrets(_ context.Context, req secrets.ListSecretsRequest) (*secrets.ListSecretsResponse, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := &secrets.ListSecretsResponse{}
	for path := range b.secrets {
		if len(req.Prefix) == 0 || hasPrefix(path, req.Prefix) {
			out.Entries = append(out.Entries, secrets.ListEntry{Path: path})
		}
	}
	return out, nil
}

func (b *grpcTestBackend) DeleteSecret(_ context.Context, req secrets.DeleteSecretRequest) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.secrets, req.Path)
	return nil
}

func (b *grpcTestBackend) IssueDynamicSecret(_ context.Context, req secrets.IssueDynamicSecretRequest) (*secrets.Secret, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.leaseID++
	lid := fmt.Sprintf("lease-%d", b.leaseID)
	b.leases[lid] = &secrets.LeaseInfo{ID: lid}
	return &secrets.Secret{
		Path:    req.Path,
		Data:    map[string]any{"password": "dyn"},
		LeaseID: lid,
	}, nil
}

func (b *grpcTestBackend) RenewLease(_ context.Context, req secrets.RenewLeaseRequest) (*secrets.LeaseInfo, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.leases[req.LeaseID]; !ok {
		return nil, fmt.Errorf("%w: %q", secrets.ErrLeaseNotFound, req.LeaseID)
	}
	return &secrets.LeaseInfo{ID: req.LeaseID}, nil
}

func (b *grpcTestBackend) RevokeLease(_ context.Context, req secrets.RevokeLeaseRequest) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.leases, req.LeaseID)
	return nil
}

func hasPrefix(s, p string) bool {
	return len(s) >= len(p) && s[:len(p)] == p
}

// grpcTestTransit is a fake TransitBackend — Encrypt/Decrypt are
// pseudo (the ciphertext is the plaintext base64'd; nothing
// cryptographic). Sign/Verify use the same trick — signature is a
// deterministic byte stream over the message. Enough to exercise the
// gRPC plumbing.
type grpcTestTransit struct{}

func (grpcTestTransit) Encrypt(_ context.Context, req secrets.EncryptRequest) (*secrets.EncryptResponse, error) {
	out := &secrets.EncryptResponse{}
	for _, it := range req.Items {
		out.Results = append(out.Results, secrets.EncryptResult{
			Ciphertext: "vault:v1:" + string(it.Plaintext),
			KeyVersion: 1,
		})
	}
	return out, nil
}

func (grpcTestTransit) Decrypt(_ context.Context, req secrets.DecryptRequest) (*secrets.DecryptResponse, error) {
	out := &secrets.DecryptResponse{}
	for _, it := range req.Items {
		const prefix = "vault:v1:"
		if !hasPrefix(it.Ciphertext, prefix) {
			out.Results = append(out.Results, secrets.DecryptResult{Err: "bad ciphertext"})
			continue
		}
		out.Results = append(out.Results, secrets.DecryptResult{
			Plaintext: []byte(it.Ciphertext[len(prefix):]),
		})
	}
	return out, nil
}

func (grpcTestTransit) Sign(_ context.Context, req secrets.SignRequest) (*secrets.SignResponse, error) {
	out := &secrets.SignResponse{}
	for _, it := range req.Items {
		out.Results = append(out.Results, secrets.SignResult{
			Signature:  "vault:v1:sig(" + string(it.Input) + ")",
			KeyVersion: 1,
		})
	}
	return out, nil
}

func (grpcTestTransit) Verify(_ context.Context, req secrets.VerifyRequest) (*secrets.VerifyResponse, error) {
	out := &secrets.VerifyResponse{}
	for _, it := range req.Items {
		want := "vault:v1:sig(" + string(it.Input) + ")"
		out.Results = append(out.Results, secrets.VerifyResult{Valid: it.Signature == want})
	}
	return out, nil
}

func (grpcTestTransit) HMAC(context.Context, secrets.HMACRequest) (*secrets.HMACResponse, error) {
	return &secrets.HMACResponse{}, nil
}

func (grpcTestTransit) VerifyHMAC(context.Context, secrets.VerifyHMACRequest) (*secrets.VerifyResponse, error) {
	return &secrets.VerifyResponse{}, nil
}

func (grpcTestTransit) Rewrap(context.Context, secrets.RewrapRequest) (*secrets.RewrapResponse, error) {
	return &secrets.RewrapResponse{}, nil
}

func (grpcTestTransit) GenerateDataKey(context.Context, secrets.GenerateDataKeyRequest) (*secrets.GenerateDataKeyResponse, error) {
	return &secrets.GenerateDataKeyResponse{}, nil
}

var _ secrets.TransitBackend = grpcTestTransit{}

// errPaths exercises the proto-version conversion edge cases.
func TestSecretsGRPC_GetSecret_BadVersion(t *testing.T) {
	t.Parallel()
	r := newSecretsRig(t)
	_, err := r.server.GetSecret(context.Background(), &v1.GetSecretRequest{
		Path:    "kv/x",
		Version: "not-a-number",
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("err code = %v, want InvalidArgument", status.Code(err))
	}
}

func TestSecretsGRPC_ListLeases_BadPageToken(t *testing.T) {
	t.Parallel()
	r := newSecretsRig(t)
	_, err := r.server.ListLeases(context.Background(), &v1.ListLeasesRequest{PageToken: "abc"})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("err code = %v, want InvalidArgument", status.Code(err))
	}
}

// Suppress unused import warnings when state.ErrNotFound goes
// unreferenced in some test files.
var _ = strconv.Itoa
