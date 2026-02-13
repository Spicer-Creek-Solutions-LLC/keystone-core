package server

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/shawnbutts/keystone-core/internal/secrets"
	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
)

// mockSecretsBackend implements secrets.SecretBackend for testing.
type mockSecretsBackend struct {
	secretStore map[string]*secrets.Secret
	listStore   map[string][]string
	healthy     bool
}

func newMockSecretsBackend() *mockSecretsBackend {
	return &mockSecretsBackend{
		secretStore: make(map[string]*secrets.Secret),
		listStore:   make(map[string][]string),
		healthy:     true,
	}
}

func (m *mockSecretsBackend) Type() secrets.BackendType          { return secrets.BackendTypeVault }
func (m *mockSecretsBackend) Name() string                       { return "mock" }
func (m *mockSecretsBackend) Healthy(_ context.Context) bool     { return m.healthy }
func (m *mockSecretsBackend) Close() error                       { return nil }
func (m *mockSecretsBackend) RenewLease(_ context.Context, _ string, _ time.Duration) (*secrets.Lease, error) {
	return nil, nil
}
func (m *mockSecretsBackend) RevokeLease(_ context.Context, _ string) error { return nil }

func (m *mockSecretsBackend) Read(_ context.Context, req *secrets.SecretRequest) (*secrets.Secret, error) {
	s, ok := m.secretStore[req.Path]
	if !ok {
		return nil, secrets.ErrSecretNotFound
	}
	return s, nil
}

func (m *mockSecretsBackend) ReadDynamic(ctx context.Context, req *secrets.SecretRequest) (*secrets.Secret, error) {
	return m.Read(ctx, req)
}

func (m *mockSecretsBackend) List(_ context.Context, prefix string) ([]string, error) {
	keys, ok := m.listStore[prefix]
	if !ok {
		return []string{}, nil
	}
	return keys, nil
}

// mockLeaseManager implements secrets.LeaseManager for testing.
type mockLeaseManager struct {
	leases map[string]*secrets.Lease
}

func newMockLeaseManager() *mockLeaseManager {
	return &mockLeaseManager{
		leases: make(map[string]*secrets.Lease),
	}
}

func (m *mockLeaseManager) Track(_ context.Context, lease *secrets.Lease) error {
	m.leases[lease.ID] = lease
	return nil
}

func (m *mockLeaseManager) Get(_ context.Context, leaseID string) (*secrets.Lease, error) {
	l, ok := m.leases[leaseID]
	if !ok {
		return nil, secrets.ErrLeaseNotFound
	}
	return l, nil
}

func (m *mockLeaseManager) List(_ context.Context) ([]*secrets.Lease, error) {
	result := make([]*secrets.Lease, 0, len(m.leases))
	for _, l := range m.leases {
		result = append(result, l)
	}
	return result, nil
}

func (m *mockLeaseManager) ListByPath(_ context.Context, path string) ([]*secrets.Lease, error) {
	var result []*secrets.Lease
	for _, l := range m.leases {
		if strings.HasPrefix(l.SecretPath, path) {
			result = append(result, l)
		}
	}
	return result, nil
}

func (m *mockLeaseManager) ListByBackend(_ context.Context, backend secrets.BackendType) ([]*secrets.Lease, error) {
	var result []*secrets.Lease
	for _, l := range m.leases {
		if l.Backend == backend {
			result = append(result, l)
		}
	}
	return result, nil
}

func (m *mockLeaseManager) ListExpiring(_ context.Context, _ time.Duration) ([]*secrets.Lease, error) {
	return nil, nil
}

func (m *mockLeaseManager) Renew(_ context.Context, leaseID string, increment time.Duration) (*secrets.Lease, error) {
	l, ok := m.leases[leaseID]
	if !ok {
		return nil, secrets.ErrLeaseNotFound
	}
	if !l.Renewable {
		return nil, secrets.ErrLeaseNotRenewable
	}
	l.RenewalCount++
	l.LastRenewal = time.Now()
	if increment > 0 {
		l.TTL = increment
	}
	l.ExpiresAt = time.Now().Add(l.TTL)
	return l, nil
}

func (m *mockLeaseManager) Revoke(_ context.Context, leaseID string) error {
	if _, ok := m.leases[leaseID]; !ok {
		return secrets.ErrLeaseNotFound
	}
	delete(m.leases, leaseID)
	return nil
}

func (m *mockLeaseManager) RevokeByPath(_ context.Context, _ string) (int, error) { return 0, nil }
func (m *mockLeaseManager) Remove(_ context.Context, _ string) error               { return nil }
func (m *mockLeaseManager) Stats(_ context.Context) (*secrets.LeaseStats, error) {
	return &secrets.LeaseStats{ActiveLeases: len(m.leases)}, nil
}
func (m *mockLeaseManager) Start(_ context.Context) error { return nil }
func (m *mockLeaseManager) Stop() error                   { return nil }

// mockTransit implements secrets.TransitBackend for testing.
type mockTransit struct {
	failDecrypt bool
}

func (m *mockTransit) Encrypt(_ context.Context, req *secrets.TransitRequest) (*secrets.TransitResponse, error) {
	return &secrets.TransitResponse{
		Ciphertext: "vault:v1:encrypted-" + string(req.Plaintext),
		KeyVersion: 1,
	}, nil
}

func (m *mockTransit) Decrypt(_ context.Context, req *secrets.TransitRequest) (*secrets.TransitResponse, error) {
	if m.failDecrypt {
		return nil, errors.New("decryption failed")
	}
	plain := strings.TrimPrefix(req.Ciphertext, "vault:v1:encrypted-")
	return &secrets.TransitResponse{
		Plaintext:  []byte(plain),
		KeyVersion: 1,
	}, nil
}

func (m *mockTransit) Rewrap(_ context.Context, _ *secrets.TransitRequest) (*secrets.TransitResponse, error) {
	return &secrets.TransitResponse{Ciphertext: "vault:v2:rewrapped"}, nil
}

func (m *mockTransit) Sign(_ context.Context, _ *secrets.TransitRequest) (*secrets.TransitResponse, error) {
	return &secrets.TransitResponse{Signature: "vault:v1:sig-abc123", KeyVersion: 1}, nil
}

func (m *mockTransit) Verify(_ context.Context, req *secrets.TransitRequest) (*secrets.TransitResponse, error) {
	valid := req.Signature == "vault:v1:sig-abc123"
	return &secrets.TransitResponse{Valid: valid}, nil
}

func (m *mockTransit) HMAC(_ context.Context, _ *secrets.TransitRequest) (*secrets.TransitResponse, error) {
	return &secrets.TransitResponse{HMAC: "vault:v1:hmac-abc123"}, nil
}

func (m *mockTransit) Hash(_ context.Context, _ *secrets.TransitRequest) (*secrets.TransitResponse, error) {
	return &secrets.TransitResponse{Hash: "sha256:abc123"}, nil
}

func (m *mockTransit) BatchEncrypt(_ context.Context, reqs []*secrets.TransitRequest) ([]*secrets.TransitResponse, error) {
	results := make([]*secrets.TransitResponse, len(reqs))
	for i, req := range reqs {
		results[i] = &secrets.TransitResponse{Ciphertext: "vault:v1:batch-" + string(req.Plaintext)}
	}
	return results, nil
}

func (m *mockTransit) BatchDecrypt(_ context.Context, reqs []*secrets.TransitRequest) ([]*secrets.TransitResponse, error) {
	results := make([]*secrets.TransitResponse, len(reqs))
	for i, req := range reqs {
		results[i] = &secrets.TransitResponse{Plaintext: []byte(strings.TrimPrefix(req.Ciphertext, "vault:v1:batch-"))}
	}
	return results, nil
}

func setupTestSecretsServer(t *testing.T) (*SecretsServer, *mockSecretsBackend, *mockLeaseManager, *mockTransit) {
	t.Helper()
	backend := newMockSecretsBackend()
	broker := secrets.NewSecretBroker(nil)
	if err := broker.RegisterBackend("mock", backend); err != nil {
		t.Fatal(err)
	}
	lm := newMockLeaseManager()
	transit := &mockTransit{}
	srv := NewSecretsServer(broker, lm, transit)
	return srv, backend, lm, transit
}

func TestNewSecretsServer(t *testing.T) {
	srv := NewSecretsServer(nil, nil, nil)
	if srv == nil {
		t.Fatal("expected non-nil server")
	}
	if srv.broker != nil {
		t.Error("expected nil broker")
	}
	if srv.leaseManager != nil {
		t.Error("expected nil leaseManager")
	}
	if srv.transit != nil {
		t.Error("expected nil transit")
	}
}

// --- GetSecret tests ---

func TestGetSecret_Success(t *testing.T) {
	srv, backend, _, _ := setupTestSecretsServer(t)
	ctx := context.Background()

	backend.secretStore["kv/myapp"] = &secrets.Secret{
		Path:    "mock/kv/myapp",
		Backend: secrets.BackendTypeVault,
		Type:    secrets.SecretTypeStatic,
		Data:    map[string]interface{}{"username": "admin"},
		Version: 1,
	}

	resp, err := srv.GetSecret(ctx, &pb.GetSecretRequest{Path: "mock/kv/myapp"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Secret.Path != "mock/kv/myapp" {
		t.Errorf("expected path mock/kv/myapp, got %s", resp.Secret.Path)
	}
	if resp.Secret.Version != 1 {
		t.Errorf("expected version 1, got %d", resp.Secret.Version)
	}
}

func TestGetSecret_NotFound(t *testing.T) {
	srv, _, _, _ := setupTestSecretsServer(t)
	ctx := context.Background()

	_, err := srv.GetSecret(ctx, &pb.GetSecretRequest{Path: "mock/nonexistent"})
	if err == nil {
		t.Fatal("expected error")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.NotFound {
		t.Errorf("expected NotFound, got %s", st.Code())
	}
}

func TestGetSecret_NilBroker(t *testing.T) {
	srv := NewSecretsServer(nil, nil, nil)
	ctx := context.Background()

	_, err := srv.GetSecret(ctx, &pb.GetSecretRequest{Path: "vault/kv/app"})
	if err == nil {
		t.Fatal("expected error")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.Unavailable {
		t.Errorf("expected Unavailable, got %s", st.Code())
	}
}

func TestGetSecret_EmptyPath(t *testing.T) {
	srv, _, _, _ := setupTestSecretsServer(t)
	ctx := context.Background()

	_, err := srv.GetSecret(ctx, &pb.GetSecretRequest{Path: ""})
	if err == nil {
		t.Fatal("expected error")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %s", st.Code())
	}
}

// --- ListSecrets tests ---

func TestListSecrets_Success(t *testing.T) {
	srv, backend, _, _ := setupTestSecretsServer(t)
	ctx := context.Background()

	backend.listStore["kv/"] = []string{"app1", "app2", "app3"}

	resp, err := srv.ListSecrets(ctx, &pb.ListSecretsRequest{
		Backend: "mock",
		Prefix:  "kv/",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Keys) != 3 {
		t.Errorf("expected 3 keys, got %d", len(resp.Keys))
	}
}

func TestListSecrets_EmptyResult(t *testing.T) {
	srv, _, _, _ := setupTestSecretsServer(t)
	ctx := context.Background()

	resp, err := srv.ListSecrets(ctx, &pb.ListSecretsRequest{
		Backend: "mock",
		Prefix:  "empty/",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Keys == nil {
		t.Error("expected non-nil keys")
	}
	if len(resp.Keys) != 0 {
		t.Errorf("expected 0 keys, got %d", len(resp.Keys))
	}
}

// --- WriteSecret tests ---

func TestWriteSecret_Unimplemented(t *testing.T) {
	srv, _, _, _ := setupTestSecretsServer(t)
	ctx := context.Background()

	data, _ := structpb.NewStruct(map[string]interface{}{"key": "value"})
	_, err := srv.WriteSecret(ctx, &pb.WriteSecretRequest{
		Path: "mock/kv/myapp",
		Data: data,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.Unimplemented {
		t.Errorf("expected Unimplemented, got %s", st.Code())
	}
}

func TestWriteSecret_NilBroker(t *testing.T) {
	srv := NewSecretsServer(nil, nil, nil)
	ctx := context.Background()

	data, _ := structpb.NewStruct(map[string]interface{}{"key": "value"})
	_, err := srv.WriteSecret(ctx, &pb.WriteSecretRequest{
		Path: "vault/kv/myapp",
		Data: data,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.Unavailable {
		t.Errorf("expected Unavailable, got %s", st.Code())
	}
}

// --- DeleteSecret tests ---

func TestDeleteSecret_Unimplemented(t *testing.T) {
	srv, _, _, _ := setupTestSecretsServer(t)
	ctx := context.Background()

	_, err := srv.DeleteSecret(ctx, &pb.DeleteSecretRequest{Path: "mock/kv/myapp"})
	if err == nil {
		t.Fatal("expected error")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.Unimplemented {
		t.Errorf("expected Unimplemented, got %s", st.Code())
	}
}

// --- GetLease tests ---

func TestGetLease_Success(t *testing.T) {
	srv, _, lm, _ := setupTestSecretsServer(t)
	ctx := context.Background()

	lm.leases["lease-123"] = &secrets.Lease{
		ID:         "lease-123",
		SecretPath: "vault/db/creds",
		Backend:    secrets.BackendTypeVault,
		State:      secrets.LeaseStateActive,
		TTL:        1 * time.Hour,
		Renewable:  true,
		IssuedAt:   time.Now(),
		ExpiresAt:  time.Now().Add(1 * time.Hour),
	}

	resp, err := srv.GetLease(ctx, &pb.GetLeaseRequest{LeaseId: "lease-123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Lease.Id != "lease-123" {
		t.Errorf("expected lease ID lease-123, got %s", resp.Lease.Id)
	}
	if !resp.Lease.Renewable {
		t.Error("expected renewable")
	}
	if resp.Lease.TtlSeconds != 3600 {
		t.Errorf("expected TTL 3600, got %d", resp.Lease.TtlSeconds)
	}
}

func TestGetLease_NotFound(t *testing.T) {
	srv, _, _, _ := setupTestSecretsServer(t)
	ctx := context.Background()

	_, err := srv.GetLease(ctx, &pb.GetLeaseRequest{LeaseId: "nonexistent"})
	if err == nil {
		t.Fatal("expected error")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.NotFound {
		t.Errorf("expected NotFound, got %s", st.Code())
	}
}

func TestGetLease_NilManager(t *testing.T) {
	srv := NewSecretsServer(nil, nil, nil)
	ctx := context.Background()

	_, err := srv.GetLease(ctx, &pb.GetLeaseRequest{LeaseId: "lease-123"})
	if err == nil {
		t.Fatal("expected error")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.Unavailable {
		t.Errorf("expected Unavailable, got %s", st.Code())
	}
}

// --- ListLeases tests ---

func TestListLeases_Success(t *testing.T) {
	srv, _, lm, _ := setupTestSecretsServer(t)
	ctx := context.Background()

	lm.leases["lease-1"] = &secrets.Lease{
		ID:         "lease-1",
		SecretPath: "vault/db/creds",
		Backend:    secrets.BackendTypeVault,
		State:      secrets.LeaseStateActive,
		TTL:        1 * time.Hour,
		IssuedAt:   time.Now(),
		ExpiresAt:  time.Now().Add(1 * time.Hour),
	}
	lm.leases["lease-2"] = &secrets.Lease{
		ID:         "lease-2",
		SecretPath: "vault/db/creds-2",
		Backend:    secrets.BackendTypeVault,
		State:      secrets.LeaseStateActive,
		TTL:        2 * time.Hour,
		IssuedAt:   time.Now(),
		ExpiresAt:  time.Now().Add(2 * time.Hour),
	}

	resp, err := srv.ListLeases(ctx, &pb.ListLeasesRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Leases) != 2 {
		t.Errorf("expected 2 leases, got %d", len(resp.Leases))
	}
	if resp.TotalCount != 2 {
		t.Errorf("expected total_count 2, got %d", resp.TotalCount)
	}
}

func TestListLeases_WithPagination(t *testing.T) {
	srv, _, lm, _ := setupTestSecretsServer(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		id := "lease-" + string(rune('a'+i))
		lm.leases[id] = &secrets.Lease{
			ID:       id,
			State:    secrets.LeaseStateActive,
			TTL:      1 * time.Hour,
			IssuedAt: time.Now(),
		}
	}

	resp, err := srv.ListLeases(ctx, &pb.ListLeasesRequest{PageSize: 2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Leases) != 2 {
		t.Errorf("expected 2 leases, got %d", len(resp.Leases))
	}
	if resp.NextPageToken == "" {
		t.Error("expected non-empty next_page_token")
	}
	if resp.TotalCount != 5 {
		t.Errorf("expected total_count 5, got %d", resp.TotalCount)
	}
}

// --- RenewLease tests ---

func TestRenewLease_Success(t *testing.T) {
	srv, _, lm, _ := setupTestSecretsServer(t)
	ctx := context.Background()

	lm.leases["lease-123"] = &secrets.Lease{
		ID:        "lease-123",
		State:     secrets.LeaseStateActive,
		TTL:       1 * time.Hour,
		Renewable: true,
		IssuedAt:  time.Now(),
		ExpiresAt: time.Now().Add(30 * time.Minute),
	}

	resp, err := srv.RenewLease(ctx, &pb.RenewLeaseRequest{
		LeaseId:          "lease-123",
		IncrementSeconds: 7200,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Lease.Id != "lease-123" {
		t.Errorf("expected lease ID lease-123, got %s", resp.Lease.Id)
	}
}

func TestRenewLease_NotRenewable(t *testing.T) {
	srv, _, lm, _ := setupTestSecretsServer(t)
	ctx := context.Background()

	lm.leases["lease-456"] = &secrets.Lease{
		ID:        "lease-456",
		State:     secrets.LeaseStateActive,
		TTL:       1 * time.Hour,
		Renewable: false,
		IssuedAt:  time.Now(),
		ExpiresAt: time.Now().Add(30 * time.Minute),
	}

	_, err := srv.RenewLease(ctx, &pb.RenewLeaseRequest{LeaseId: "lease-456"})
	if err == nil {
		t.Fatal("expected error")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %s", st.Code())
	}
}

// --- RevokeLease tests ---

func TestRevokeLease_Success(t *testing.T) {
	srv, _, lm, _ := setupTestSecretsServer(t)
	ctx := context.Background()

	lm.leases["lease-789"] = &secrets.Lease{
		ID:    "lease-789",
		State: secrets.LeaseStateActive,
	}

	resp, err := srv.RevokeLease(ctx, &pb.RevokeLeaseRequest{LeaseId: "lease-789"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Revoked {
		t.Error("expected revoked to be true")
	}
	if resp.LeaseId != "lease-789" {
		t.Errorf("expected lease_id lease-789, got %s", resp.LeaseId)
	}
}

func TestRevokeLease_NotFound(t *testing.T) {
	srv, _, _, _ := setupTestSecretsServer(t)
	ctx := context.Background()

	_, err := srv.RevokeLease(ctx, &pb.RevokeLeaseRequest{LeaseId: "nonexistent"})
	if err == nil {
		t.Fatal("expected error")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.NotFound {
		t.Errorf("expected NotFound, got %s", st.Code())
	}
}

// --- Encrypt tests ---

func TestEncrypt_Success(t *testing.T) {
	srv, _, _, _ := setupTestSecretsServer(t)
	ctx := context.Background()

	resp, err := srv.Encrypt(ctx, &pb.EncryptRequest{
		KeyName:   "my-key",
		Plaintext: []byte("hello world"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Ciphertext == "" {
		t.Error("expected non-empty ciphertext")
	}
	if resp.KeyVersion != 1 {
		t.Errorf("expected key_version 1, got %d", resp.KeyVersion)
	}
}

func TestEncrypt_NilTransit(t *testing.T) {
	srv := NewSecretsServer(nil, nil, nil)
	ctx := context.Background()

	_, err := srv.Encrypt(ctx, &pb.EncryptRequest{
		KeyName:   "my-key",
		Plaintext: []byte("hello"),
	})
	if err == nil {
		t.Fatal("expected error")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.Unavailable {
		t.Errorf("expected Unavailable, got %s", st.Code())
	}
}

// --- Decrypt tests ---

func TestDecrypt_Success(t *testing.T) {
	srv, _, _, _ := setupTestSecretsServer(t)
	ctx := context.Background()

	resp, err := srv.Decrypt(ctx, &pb.DecryptRequest{
		KeyName:    "my-key",
		Ciphertext: "vault:v1:encrypted-hello",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(resp.Plaintext) != "hello" {
		t.Errorf("expected plaintext 'hello', got %q", resp.Plaintext)
	}
}

func TestDecrypt_Error(t *testing.T) {
	srv, _, _, transit := setupTestSecretsServer(t)
	transit.failDecrypt = true
	ctx := context.Background()

	_, err := srv.Decrypt(ctx, &pb.DecryptRequest{
		KeyName:    "my-key",
		Ciphertext: "invalid",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.Internal {
		t.Errorf("expected Internal, got %s", st.Code())
	}
}

// --- Sign tests ---

func TestSign_Success(t *testing.T) {
	srv, _, _, _ := setupTestSecretsServer(t)
	ctx := context.Background()

	resp, err := srv.Sign(ctx, &pb.SignRequest{
		KeyName: "my-key",
		Input:   []byte("data to sign"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Signature == "" {
		t.Error("expected non-empty signature")
	}
	if resp.KeyVersion != 1 {
		t.Errorf("expected key_version 1, got %d", resp.KeyVersion)
	}
}

// --- Verify tests ---

func TestVerify_Success(t *testing.T) {
	srv, _, _, _ := setupTestSecretsServer(t)
	ctx := context.Background()

	resp, err := srv.Verify(ctx, &pb.VerifyRequest{
		KeyName:   "my-key",
		Input:     []byte("data to sign"),
		Signature: "vault:v1:sig-abc123",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Valid {
		t.Error("expected valid signature")
	}
}

func TestVerify_InvalidSignature(t *testing.T) {
	srv, _, _, _ := setupTestSecretsServer(t)
	ctx := context.Background()

	resp, err := srv.Verify(ctx, &pb.VerifyRequest{
		KeyName:   "my-key",
		Input:     []byte("data to sign"),
		Signature: "wrong-signature",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Valid {
		t.Error("expected invalid signature")
	}
}

// --- Error mapping tests ---

func TestSecretErrorToGRPCStatus(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected codes.Code
	}{
		{
			name:     "SecretError not found code",
			err:      secrets.NewSecretError(secrets.ErrorCodeNotFound, "not found", nil),
			expected: codes.NotFound,
		},
		{
			name:     "SecretError access denied code",
			err:      secrets.NewSecretError(secrets.ErrorCodeAccessDenied, "denied", nil),
			expected: codes.PermissionDenied,
		},
		{
			name:     "SecretError authentication code",
			err:      secrets.NewSecretError(secrets.ErrorCodeAuthentication, "auth", nil),
			expected: codes.Unauthenticated,
		},
		{
			name:     "SecretError invalid request code",
			err:      secrets.NewSecretError(secrets.ErrorCodeInvalidRequest, "bad", nil),
			expected: codes.InvalidArgument,
		},
		{
			name:     "SecretError rate limit code",
			err:      secrets.NewSecretError(secrets.ErrorCodeRateLimit, "slow down", nil),
			expected: codes.ResourceExhausted,
		},
		{
			name:     "SecretError backend unavailable code",
			err:      secrets.NewSecretError(secrets.ErrorCodeBackendUnavailable, "down", nil),
			expected: codes.Unavailable,
		},
		{
			name:     "SecretError timeout code",
			err:      secrets.NewSecretError(secrets.ErrorCodeTimeout, "timeout", nil),
			expected: codes.DeadlineExceeded,
		},
		{
			name:     "SecretError internal code",
			err:      secrets.NewSecretError(secrets.ErrorCodeInternal, "error", nil),
			expected: codes.Internal,
		},
		{
			name:     "sentinel ErrSecretNotFound",
			err:      secrets.ErrSecretNotFound,
			expected: codes.NotFound,
		},
		{
			name:     "sentinel ErrLeaseNotFound",
			err:      secrets.ErrLeaseNotFound,
			expected: codes.NotFound,
		},
		{
			name:     "sentinel ErrAccessDenied",
			err:      secrets.ErrAccessDenied,
			expected: codes.PermissionDenied,
		},
		{
			name:     "sentinel ErrLeaseNotRenewable",
			err:      secrets.ErrLeaseNotRenewable,
			expected: codes.InvalidArgument,
		},
		{
			name:     "sentinel ErrInvalidPath",
			err:      secrets.ErrInvalidPath,
			expected: codes.InvalidArgument,
		},
		{
			name:     "sentinel ErrBackendUnavailable",
			err:      secrets.ErrBackendUnavailable,
			expected: codes.Unavailable,
		},
		{
			name:     "sentinel ErrLeaseExpired",
			err:      secrets.ErrLeaseExpired,
			expected: codes.FailedPrecondition,
		},
		{
			name:     "sentinel ErrRotationInProgress",
			err:      secrets.ErrRotationInProgress,
			expected: codes.AlreadyExists,
		},
		{
			name:     "sentinel ErrBackendNotFound",
			err:      secrets.ErrBackendNotFound,
			expected: codes.NotFound,
		},
		{
			name:     "unknown error",
			err:      errors.New("something else"),
			expected: codes.Internal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			grpcErr := secretErrorToGRPCStatus(tt.err)
			st, ok := status.FromError(grpcErr)
			if !ok {
				t.Fatalf("expected gRPC status error, got %v", grpcErr)
			}
			if st.Code() != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, st.Code())
			}
		})
	}
}
