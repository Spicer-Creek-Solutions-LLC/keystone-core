package secrets

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
)

// mockSecretsServiceClient implements pb.SecretsServiceClient for testing.
type mockSecretsServiceClient struct {
	getSecretFn    func(ctx context.Context, in *pb.GetSecretRequest, opts ...grpc.CallOption) (*pb.GetSecretResponse, error)
	listSecretsFn  func(ctx context.Context, in *pb.ListSecretsRequest, opts ...grpc.CallOption) (*pb.ListSecretsResponse, error)
	writeSecretFn  func(ctx context.Context, in *pb.WriteSecretRequest, opts ...grpc.CallOption) (*pb.WriteSecretResponse, error)
	deleteSecretFn func(ctx context.Context, in *pb.DeleteSecretRequest, opts ...grpc.CallOption) (*pb.DeleteSecretResponse, error)
	getLeaseFn     func(ctx context.Context, in *pb.GetLeaseRequest, opts ...grpc.CallOption) (*pb.GetLeaseResponse, error)
	listLeasesFn   func(ctx context.Context, in *pb.ListLeasesRequest, opts ...grpc.CallOption) (*pb.ListLeasesResponse, error)
	renewLeaseFn   func(ctx context.Context, in *pb.RenewLeaseRequest, opts ...grpc.CallOption) (*pb.RenewLeaseResponse, error)
	revokeLeaseFn  func(ctx context.Context, in *pb.RevokeLeaseRequest, opts ...grpc.CallOption) (*pb.RevokeLeaseResponse, error)
	encryptFn      func(ctx context.Context, in *pb.EncryptRequest, opts ...grpc.CallOption) (*pb.EncryptResponse, error)
	decryptFn      func(ctx context.Context, in *pb.DecryptRequest, opts ...grpc.CallOption) (*pb.DecryptResponse, error)
	signFn         func(ctx context.Context, in *pb.SignRequest, opts ...grpc.CallOption) (*pb.SignResponse, error)
	verifyFn       func(ctx context.Context, in *pb.VerifyRequest, opts ...grpc.CallOption) (*pb.VerifyResponse, error)
}

func (m *mockSecretsServiceClient) GetSecret(ctx context.Context, in *pb.GetSecretRequest, opts ...grpc.CallOption) (*pb.GetSecretResponse, error) {
	if m.getSecretFn != nil {
		return m.getSecretFn(ctx, in, opts...)
	}
	return nil, status.Error(codes.Unimplemented, "not configured")
}

func (m *mockSecretsServiceClient) ListSecrets(ctx context.Context, in *pb.ListSecretsRequest, opts ...grpc.CallOption) (*pb.ListSecretsResponse, error) {
	if m.listSecretsFn != nil {
		return m.listSecretsFn(ctx, in, opts...)
	}
	return nil, status.Error(codes.Unimplemented, "not configured")
}

func (m *mockSecretsServiceClient) WriteSecret(ctx context.Context, in *pb.WriteSecretRequest, opts ...grpc.CallOption) (*pb.WriteSecretResponse, error) {
	if m.writeSecretFn != nil {
		return m.writeSecretFn(ctx, in, opts...)
	}
	return nil, status.Error(codes.Unimplemented, "not configured")
}

func (m *mockSecretsServiceClient) DeleteSecret(ctx context.Context, in *pb.DeleteSecretRequest, opts ...grpc.CallOption) (*pb.DeleteSecretResponse, error) {
	if m.deleteSecretFn != nil {
		return m.deleteSecretFn(ctx, in, opts...)
	}
	return nil, status.Error(codes.Unimplemented, "not configured")
}

func (m *mockSecretsServiceClient) GetLease(ctx context.Context, in *pb.GetLeaseRequest, opts ...grpc.CallOption) (*pb.GetLeaseResponse, error) {
	if m.getLeaseFn != nil {
		return m.getLeaseFn(ctx, in, opts...)
	}
	return nil, status.Error(codes.Unimplemented, "not configured")
}

func (m *mockSecretsServiceClient) ListLeases(ctx context.Context, in *pb.ListLeasesRequest, opts ...grpc.CallOption) (*pb.ListLeasesResponse, error) {
	if m.listLeasesFn != nil {
		return m.listLeasesFn(ctx, in, opts...)
	}
	return nil, status.Error(codes.Unimplemented, "not configured")
}

func (m *mockSecretsServiceClient) RenewLease(ctx context.Context, in *pb.RenewLeaseRequest, opts ...grpc.CallOption) (*pb.RenewLeaseResponse, error) {
	if m.renewLeaseFn != nil {
		return m.renewLeaseFn(ctx, in, opts...)
	}
	return nil, status.Error(codes.Unimplemented, "not configured")
}

func (m *mockSecretsServiceClient) RevokeLease(ctx context.Context, in *pb.RevokeLeaseRequest, opts ...grpc.CallOption) (*pb.RevokeLeaseResponse, error) {
	if m.revokeLeaseFn != nil {
		return m.revokeLeaseFn(ctx, in, opts...)
	}
	return nil, status.Error(codes.Unimplemented, "not configured")
}

func (m *mockSecretsServiceClient) Encrypt(ctx context.Context, in *pb.EncryptRequest, opts ...grpc.CallOption) (*pb.EncryptResponse, error) {
	if m.encryptFn != nil {
		return m.encryptFn(ctx, in, opts...)
	}
	return nil, status.Error(codes.Unimplemented, "not configured")
}

func (m *mockSecretsServiceClient) Decrypt(ctx context.Context, in *pb.DecryptRequest, opts ...grpc.CallOption) (*pb.DecryptResponse, error) {
	if m.decryptFn != nil {
		return m.decryptFn(ctx, in, opts...)
	}
	return nil, status.Error(codes.Unimplemented, "not configured")
}

func (m *mockSecretsServiceClient) Sign(ctx context.Context, in *pb.SignRequest, opts ...grpc.CallOption) (*pb.SignResponse, error) {
	if m.signFn != nil {
		return m.signFn(ctx, in, opts...)
	}
	return nil, status.Error(codes.Unimplemented, "not configured")
}

func (m *mockSecretsServiceClient) Verify(ctx context.Context, in *pb.VerifyRequest, opts ...grpc.CallOption) (*pb.VerifyResponse, error) {
	if m.verifyFn != nil {
		return m.verifyFn(ctx, in, opts...)
	}
	return nil, status.Error(codes.Unimplemented, "not configured")
}

func newTestClient(mock *mockSecretsServiceClient) *Client {
	return &Client{client: mock}
}

// --- NewClientFromConn / Close ---

func TestNewClientFromConn(t *testing.T) {
	mock := &mockSecretsServiceClient{}
	c := &Client{client: mock}
	if c.client == nil {
		t.Fatal("expected non-nil client")
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}
}

// --- GetSecret ---

func TestGetSecret_Success(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	data, _ := structpb.NewStruct(map[string]interface{}{"username": "admin"})
	mock := &mockSecretsServiceClient{
		getSecretFn: func(_ context.Context, in *pb.GetSecretRequest, _ ...grpc.CallOption) (*pb.GetSecretResponse, error) {
			if in.Path != "vault/kv/myapp" {
				t.Errorf("expected path vault/kv/myapp, got %s", in.Path)
			}
			return &pb.GetSecretResponse{
				Secret: &pb.SecretData{
					Path:      "vault/kv/myapp",
					Backend:   "vault",
					Type:      "static",
					Data:      data,
					Metadata:  map[string]string{"env": "prod"},
					Version:   3,
					CreatedAt: timestamppb.New(now),
					Renewable: true,
					LeaseId:   "lease-abc",
				},
			}, nil
		},
	}

	c := newTestClient(mock)
	secret, err := c.GetSecret(context.Background(), "vault/kv/myapp", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if secret.Path != "vault/kv/myapp" {
		t.Errorf("expected path vault/kv/myapp, got %s", secret.Path)
	}
	if secret.Version != 3 {
		t.Errorf("expected version 3, got %d", secret.Version)
	}
	if v, ok := secret.GetString("username"); !ok || v != "admin" {
		t.Errorf("expected username=admin, got %q ok=%v", v, ok)
	}
	if !secret.Renewable {
		t.Error("expected renewable")
	}
	if secret.LeaseID != "lease-abc" {
		t.Errorf("expected lease_id lease-abc, got %s", secret.LeaseID)
	}
	if !secret.CreatedAt.Equal(now) {
		t.Errorf("expected created_at %v, got %v", now, secret.CreatedAt)
	}
}

func TestGetSecret_NotFound(t *testing.T) {
	mock := &mockSecretsServiceClient{
		getSecretFn: func(_ context.Context, _ *pb.GetSecretRequest, _ ...grpc.CallOption) (*pb.GetSecretResponse, error) {
			return nil, status.Error(codes.NotFound, "secret not found")
		},
	}

	c := newTestClient(mock)
	_, err := c.GetSecret(context.Background(), "vault/kv/nonexistent", 0)
	if !errors.Is(err, ErrSecretNotFound) {
		t.Errorf("expected ErrSecretNotFound, got %v", err)
	}
}

func TestGetSecret_Unavailable(t *testing.T) {
	mock := &mockSecretsServiceClient{
		getSecretFn: func(_ context.Context, _ *pb.GetSecretRequest, _ ...grpc.CallOption) (*pb.GetSecretResponse, error) {
			return nil, status.Error(codes.Unavailable, "backend down")
		},
	}

	c := newTestClient(mock)
	_, err := c.GetSecret(context.Background(), "vault/kv/app", 0)
	if !errors.Is(err, ErrBackendUnavailable) {
		t.Errorf("expected ErrBackendUnavailable, got %v", err)
	}
}

// --- ListSecrets ---

func TestListSecrets_Success(t *testing.T) {
	mock := &mockSecretsServiceClient{
		listSecretsFn: func(_ context.Context, in *pb.ListSecretsRequest, _ ...grpc.CallOption) (*pb.ListSecretsResponse, error) {
			return &pb.ListSecretsResponse{
				Keys:          []string{"app1", "app2", "app3"},
				NextPageToken: "next",
			}, nil
		},
	}

	c := newTestClient(mock)
	result, err := c.ListSecrets(context.Background(), "vault", "kv/", 10, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Keys) != 3 {
		t.Errorf("expected 3 keys, got %d", len(result.Keys))
	}
	if result.NextPageToken != "next" {
		t.Errorf("expected next page token 'next', got %s", result.NextPageToken)
	}
}

func TestListSecrets_Empty(t *testing.T) {
	mock := &mockSecretsServiceClient{
		listSecretsFn: func(_ context.Context, _ *pb.ListSecretsRequest, _ ...grpc.CallOption) (*pb.ListSecretsResponse, error) {
			return &pb.ListSecretsResponse{}, nil
		},
	}

	c := newTestClient(mock)
	result, err := c.ListSecrets(context.Background(), "vault", "kv/empty", 10, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Keys == nil {
		t.Error("expected non-nil keys")
	}
	if len(result.Keys) != 0 {
		t.Errorf("expected 0 keys, got %d", len(result.Keys))
	}
}

// --- WriteSecret ---

func TestWriteSecret_Success(t *testing.T) {
	mock := &mockSecretsServiceClient{
		writeSecretFn: func(_ context.Context, in *pb.WriteSecretRequest, _ ...grpc.CallOption) (*pb.WriteSecretResponse, error) {
			if in.Path != "vault/kv/myapp" {
				t.Errorf("expected path vault/kv/myapp, got %s", in.Path)
			}
			return &pb.WriteSecretResponse{Path: in.Path, Written: true}, nil
		},
	}

	c := newTestClient(mock)
	err := c.WriteSecret(context.Background(), "vault/kv/myapp", map[string]interface{}{"key": "value"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWriteSecret_Error(t *testing.T) {
	mock := &mockSecretsServiceClient{
		writeSecretFn: func(_ context.Context, _ *pb.WriteSecretRequest, _ ...grpc.CallOption) (*pb.WriteSecretResponse, error) {
			return nil, status.Error(codes.Unimplemented, "backend does not support writes")
		},
	}

	c := newTestClient(mock)
	err := c.WriteSecret(context.Background(), "vault/kv/myapp", map[string]interface{}{"key": "value"}, nil)
	if !errors.Is(err, ErrUnimplemented) {
		t.Errorf("expected ErrUnimplemented, got %v", err)
	}
}

// --- DeleteSecret ---

func TestDeleteSecret_Success(t *testing.T) {
	mock := &mockSecretsServiceClient{
		deleteSecretFn: func(_ context.Context, in *pb.DeleteSecretRequest, _ ...grpc.CallOption) (*pb.DeleteSecretResponse, error) {
			return &pb.DeleteSecretResponse{Path: in.Path, Deleted: true}, nil
		},
	}

	c := newTestClient(mock)
	err := c.DeleteSecret(context.Background(), "vault/kv/myapp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteSecret_Error(t *testing.T) {
	mock := &mockSecretsServiceClient{
		deleteSecretFn: func(_ context.Context, _ *pb.DeleteSecretRequest, _ ...grpc.CallOption) (*pb.DeleteSecretResponse, error) {
			return nil, status.Error(codes.PermissionDenied, "access denied")
		},
	}

	c := newTestClient(mock)
	err := c.DeleteSecret(context.Background(), "vault/kv/myapp")
	if !errors.Is(err, ErrAccessDenied) {
		t.Errorf("expected ErrAccessDenied, got %v", err)
	}
}

// --- GetLease ---

func TestGetLease_Success(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	mock := &mockSecretsServiceClient{
		getLeaseFn: func(_ context.Context, in *pb.GetLeaseRequest, _ ...grpc.CallOption) (*pb.GetLeaseResponse, error) {
			return &pb.GetLeaseResponse{
				Lease: &pb.LeaseInfo{
					Id:            in.LeaseId,
					SecretPath:    "vault/db/creds",
					Backend:       "vault",
					State:         "active",
					TtlSeconds:    3600,
					MaxTtlSeconds: 86400,
					IssuedAt:      timestamppb.New(now),
					ExpiresAt:     timestamppb.New(now.Add(1 * time.Hour)),
					RenewalCount:  2,
					Renewable:     true,
					Revocable:     true,
				},
			}, nil
		},
	}

	c := newTestClient(mock)
	lease, err := c.GetLease(context.Background(), "lease-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lease.ID != "lease-123" {
		t.Errorf("expected ID lease-123, got %s", lease.ID)
	}
	if lease.TTL != 1*time.Hour {
		t.Errorf("expected TTL 1h, got %v", lease.TTL)
	}
	if lease.MaxTTL != 24*time.Hour {
		t.Errorf("expected MaxTTL 24h, got %v", lease.MaxTTL)
	}
	if !lease.Renewable {
		t.Error("expected renewable")
	}
	if lease.RenewalCount != 2 {
		t.Errorf("expected renewal_count 2, got %d", lease.RenewalCount)
	}
}

func TestGetLease_NotFound(t *testing.T) {
	mock := &mockSecretsServiceClient{
		getLeaseFn: func(_ context.Context, _ *pb.GetLeaseRequest, _ ...grpc.CallOption) (*pb.GetLeaseResponse, error) {
			return nil, status.Error(codes.NotFound, "lease not found")
		},
	}

	c := newTestClient(mock)
	_, err := c.GetLease(context.Background(), "nonexistent")
	if !errors.Is(err, ErrSecretNotFound) {
		t.Errorf("expected ErrSecretNotFound, got %v", err)
	}
}

// --- ListLeases ---

func TestListLeases_Success(t *testing.T) {
	mock := &mockSecretsServiceClient{
		listLeasesFn: func(_ context.Context, _ *pb.ListLeasesRequest, _ ...grpc.CallOption) (*pb.ListLeasesResponse, error) {
			return &pb.ListLeasesResponse{
				Leases: []*pb.LeaseInfo{
					{Id: "lease-1", State: "active", TtlSeconds: 3600},
					{Id: "lease-2", State: "active", TtlSeconds: 7200},
				},
				TotalCount:    2,
				NextPageToken: "",
			}, nil
		},
	}

	c := newTestClient(mock)
	result, err := c.ListLeases(context.Background(), "", "", 100, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Leases) != 2 {
		t.Errorf("expected 2 leases, got %d", len(result.Leases))
	}
	if result.TotalCount != 2 {
		t.Errorf("expected total_count 2, got %d", result.TotalCount)
	}
}

func TestListLeases_WithFilter(t *testing.T) {
	mock := &mockSecretsServiceClient{
		listLeasesFn: func(_ context.Context, in *pb.ListLeasesRequest, _ ...grpc.CallOption) (*pb.ListLeasesResponse, error) {
			if in.PathPrefix != "vault/db/" {
				t.Errorf("expected path_prefix vault/db/, got %s", in.PathPrefix)
			}
			return &pb.ListLeasesResponse{
				Leases:     []*pb.LeaseInfo{{Id: "lease-1", TtlSeconds: 3600}},
				TotalCount: 1,
			}, nil
		},
	}

	c := newTestClient(mock)
	result, err := c.ListLeases(context.Background(), "vault/db/", "", 100, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Leases) != 1 {
		t.Errorf("expected 1 lease, got %d", len(result.Leases))
	}
}

// --- RenewLease ---

func TestRenewLease_Success(t *testing.T) {
	mock := &mockSecretsServiceClient{
		renewLeaseFn: func(_ context.Context, in *pb.RenewLeaseRequest, _ ...grpc.CallOption) (*pb.RenewLeaseResponse, error) {
			if in.IncrementSeconds != 7200 {
				t.Errorf("expected increment 7200, got %d", in.IncrementSeconds)
			}
			return &pb.RenewLeaseResponse{
				Lease: &pb.LeaseInfo{
					Id:         in.LeaseId,
					TtlSeconds: 7200,
					Renewable:  true,
				},
			}, nil
		},
	}

	c := newTestClient(mock)
	lease, err := c.RenewLease(context.Background(), "lease-123", 2*time.Hour)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lease.TTL != 2*time.Hour {
		t.Errorf("expected TTL 2h, got %v", lease.TTL)
	}
}

func TestRenewLease_NotRenewable(t *testing.T) {
	mock := &mockSecretsServiceClient{
		renewLeaseFn: func(_ context.Context, _ *pb.RenewLeaseRequest, _ ...grpc.CallOption) (*pb.RenewLeaseResponse, error) {
			return nil, status.Error(codes.InvalidArgument, "lease not renewable")
		},
	}

	c := newTestClient(mock)
	_, err := c.RenewLease(context.Background(), "lease-456", 1*time.Hour)
	if !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("expected ErrInvalidArgument, got %v", err)
	}
}

// --- RevokeLease ---

func TestRevokeLease_Success(t *testing.T) {
	mock := &mockSecretsServiceClient{
		revokeLeaseFn: func(_ context.Context, _ *pb.RevokeLeaseRequest, _ ...grpc.CallOption) (*pb.RevokeLeaseResponse, error) {
			return &pb.RevokeLeaseResponse{LeaseId: "lease-789", Revoked: true}, nil
		},
	}

	c := newTestClient(mock)
	err := c.RevokeLease(context.Background(), "lease-789")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRevokeLease_NotFound(t *testing.T) {
	mock := &mockSecretsServiceClient{
		revokeLeaseFn: func(_ context.Context, _ *pb.RevokeLeaseRequest, _ ...grpc.CallOption) (*pb.RevokeLeaseResponse, error) {
			return nil, status.Error(codes.NotFound, "lease not found")
		},
	}

	c := newTestClient(mock)
	err := c.RevokeLease(context.Background(), "nonexistent")
	if !errors.Is(err, ErrSecretNotFound) {
		t.Errorf("expected ErrSecretNotFound, got %v", err)
	}
}

// --- Encrypt ---

func TestEncrypt_Success(t *testing.T) {
	mock := &mockSecretsServiceClient{
		encryptFn: func(_ context.Context, in *pb.EncryptRequest, _ ...grpc.CallOption) (*pb.EncryptResponse, error) {
			if in.KeyName != "my-key" {
				t.Errorf("expected key_name my-key, got %s", in.KeyName)
			}
			return &pb.EncryptResponse{
				Ciphertext: "vault:v1:encrypted",
				KeyVersion: 1,
			}, nil
		},
	}

	c := newTestClient(mock)
	result, err := c.Encrypt(context.Background(), "my-key", []byte("hello"), nil, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Ciphertext != "vault:v1:encrypted" {
		t.Errorf("expected ciphertext vault:v1:encrypted, got %s", result.Ciphertext)
	}
	if result.KeyVersion != 1 {
		t.Errorf("expected key_version 1, got %d", result.KeyVersion)
	}
}

func TestEncrypt_Error(t *testing.T) {
	mock := &mockSecretsServiceClient{
		encryptFn: func(_ context.Context, _ *pb.EncryptRequest, _ ...grpc.CallOption) (*pb.EncryptResponse, error) {
			return nil, status.Error(codes.Unavailable, "transit not available")
		},
	}

	c := newTestClient(mock)
	_, err := c.Encrypt(context.Background(), "my-key", []byte("hello"), nil, 0)
	if !errors.Is(err, ErrBackendUnavailable) {
		t.Errorf("expected ErrBackendUnavailable, got %v", err)
	}
}

// --- Decrypt ---

func TestDecrypt_Success(t *testing.T) {
	mock := &mockSecretsServiceClient{
		decryptFn: func(_ context.Context, _ *pb.DecryptRequest, _ ...grpc.CallOption) (*pb.DecryptResponse, error) {
			return &pb.DecryptResponse{
				Plaintext:  []byte("hello"),
				KeyVersion: 1,
			}, nil
		},
	}

	c := newTestClient(mock)
	result, err := c.Decrypt(context.Background(), "my-key", "vault:v1:encrypted", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(result.Plaintext) != "hello" {
		t.Errorf("expected plaintext 'hello', got %q", result.Plaintext)
	}
}

func TestDecrypt_Error(t *testing.T) {
	mock := &mockSecretsServiceClient{
		decryptFn: func(_ context.Context, _ *pb.DecryptRequest, _ ...grpc.CallOption) (*pb.DecryptResponse, error) {
			return nil, status.Error(codes.InvalidArgument, "invalid ciphertext")
		},
	}

	c := newTestClient(mock)
	_, err := c.Decrypt(context.Background(), "my-key", "bad", nil)
	if !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("expected ErrInvalidArgument, got %v", err)
	}
}

// --- Sign ---

func TestSign_Success(t *testing.T) {
	mock := &mockSecretsServiceClient{
		signFn: func(_ context.Context, _ *pb.SignRequest, _ ...grpc.CallOption) (*pb.SignResponse, error) {
			return &pb.SignResponse{
				Signature:  "vault:v1:sig-abc123",
				KeyVersion: 1,
			}, nil
		},
	}

	c := newTestClient(mock)
	result, err := c.Sign(context.Background(), "my-key", []byte("data"), "sha256", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Signature != "vault:v1:sig-abc123" {
		t.Errorf("expected signature vault:v1:sig-abc123, got %s", result.Signature)
	}
}

// --- Verify ---

func TestVerify_Valid(t *testing.T) {
	mock := &mockSecretsServiceClient{
		verifyFn: func(_ context.Context, _ *pb.VerifyRequest, _ ...grpc.CallOption) (*pb.VerifyResponse, error) {
			return &pb.VerifyResponse{Valid: true}, nil
		},
	}

	c := newTestClient(mock)
	valid, err := c.Verify(context.Background(), "my-key", []byte("data"), "vault:v1:sig-abc123", "sha256")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !valid {
		t.Error("expected valid")
	}
}

func TestVerify_Invalid(t *testing.T) {
	mock := &mockSecretsServiceClient{
		verifyFn: func(_ context.Context, _ *pb.VerifyRequest, _ ...grpc.CallOption) (*pb.VerifyResponse, error) {
			return &pb.VerifyResponse{Valid: false}, nil
		},
	}

	c := newTestClient(mock)
	valid, err := c.Verify(context.Background(), "my-key", []byte("data"), "wrong-sig", "sha256")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if valid {
		t.Error("expected invalid")
	}
}

// --- Error mapping ---

func TestGRPCStatusToError(t *testing.T) {
	tests := []struct {
		name     string
		code     codes.Code
		expected error
	}{
		{"OK", codes.OK, nil},
		{"NotFound", codes.NotFound, ErrSecretNotFound},
		{"PermissionDenied", codes.PermissionDenied, ErrAccessDenied},
		{"InvalidArgument", codes.InvalidArgument, ErrInvalidArgument},
		{"Unavailable", codes.Unavailable, ErrBackendUnavailable},
		{"FailedPrecondition", codes.FailedPrecondition, ErrLeaseExpired},
		{"AlreadyExists", codes.AlreadyExists, ErrAlreadyExists},
		{"Unauthenticated", codes.Unauthenticated, ErrUnauthenticated},
		{"Unimplemented", codes.Unimplemented, ErrUnimplemented},
		{"ResourceExhausted", codes.ResourceExhausted, ErrResourceExhausted},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var input error
			if tt.code != codes.OK {
				input = status.Error(tt.code, "test")
			}
			got := grpcStatusToError(input)
			if tt.expected == nil {
				if got != nil {
					t.Errorf("expected nil, got %v", got)
				}
				return
			}
			if !errors.Is(got, tt.expected) {
				t.Errorf("expected %v, got %v", tt.expected, got)
			}
		})
	}
}

func TestGRPCStatusToError_NilError(t *testing.T) {
	if got := grpcStatusToError(nil); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestGRPCStatusToError_NonGRPCError(t *testing.T) {
	plain := errors.New("plain error")
	got := grpcStatusToError(plain)
	if !errors.Is(got, plain) {
		t.Errorf("expected original error, got %v", got)
	}
}

func TestGRPCStatusToError_UnknownCode(t *testing.T) {
	err := status.Error(codes.DataLoss, "data loss")
	got := grpcStatusToError(err)
	st, ok := status.FromError(got)
	if !ok {
		t.Fatal("expected gRPC status error")
	}
	if st.Code() != codes.DataLoss {
		t.Errorf("expected DataLoss, got %s", st.Code())
	}
}

// --- Type helper methods ---

func TestSecret_Get(t *testing.T) {
	s := &Secret{Data: map[string]interface{}{"key": "value", "num": float64(42)}}
	v, ok := s.Get("key")
	if !ok || v != "value" {
		t.Errorf("expected 'value', got %v ok=%v", v, ok)
	}
	_, ok = s.Get("missing")
	if ok {
		t.Error("expected ok=false for missing key")
	}
}

func TestSecret_Get_NilData(t *testing.T) {
	s := &Secret{}
	_, ok := s.Get("key")
	if ok {
		t.Error("expected ok=false for nil data")
	}
}

func TestSecret_GetString(t *testing.T) {
	s := &Secret{Data: map[string]interface{}{"str": "hello", "num": float64(42)}}
	v, ok := s.GetString("str")
	if !ok || v != "hello" {
		t.Errorf("expected 'hello', got %q ok=%v", v, ok)
	}
	_, ok = s.GetString("num")
	if ok {
		t.Error("expected ok=false for non-string value")
	}
}

func TestSecret_IsExpired(t *testing.T) {
	s := &Secret{}
	if s.IsExpired() {
		t.Error("zero time should not be expired")
	}
	s.ExpiresAt = time.Now().Add(-1 * time.Hour)
	if !s.IsExpired() {
		t.Error("past time should be expired")
	}
	s.ExpiresAt = time.Now().Add(1 * time.Hour)
	if s.IsExpired() {
		t.Error("future time should not be expired")
	}
}

func TestLease_IsExpired(t *testing.T) {
	l := &Lease{}
	if l.IsExpired() {
		t.Error("zero time should not be expired")
	}
	l.ExpiresAt = time.Now().Add(-1 * time.Hour)
	if !l.IsExpired() {
		t.Error("past time should be expired")
	}
}

func TestLease_TimeRemaining(t *testing.T) {
	l := &Lease{}
	if l.TimeRemaining() != 0 {
		t.Error("zero time should return 0")
	}
	l.ExpiresAt = time.Now().Add(-1 * time.Hour)
	if l.TimeRemaining() != 0 {
		t.Error("expired lease should return 0")
	}
	l.ExpiresAt = time.Now().Add(30 * time.Minute)
	remaining := l.TimeRemaining()
	if remaining < 29*time.Minute || remaining > 31*time.Minute {
		t.Errorf("expected ~30m remaining, got %v", remaining)
	}
}

// --- Proto conversion helpers ---

func TestSecretFromProto_Nil(t *testing.T) {
	s := secretFromProto(nil)
	if s != nil {
		t.Error("expected nil")
	}
}

func TestLeaseFromProto_Nil(t *testing.T) {
	l := leaseFromProto(nil)
	if l != nil {
		t.Error("expected nil")
	}
}
