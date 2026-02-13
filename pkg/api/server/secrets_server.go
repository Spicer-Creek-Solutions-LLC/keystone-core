package server

import (
	"context"
	"errors"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/shawnbutts/keystone-core/internal/secrets"
	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
)

// WritableBackend is the interface for backends that support writes.
type WritableBackend interface {
	Write(path string, data map[string]interface{}, metadata map[string]string) error
}

// DeletableBackend is the interface for backends that support deletes.
type DeletableBackend interface {
	Delete(path string) error
}

// SecretsServer implements the SecretsService gRPC server.
type SecretsServer struct {
	pb.UnimplementedSecretsServiceServer
	broker       *secrets.SecretBroker
	leaseManager secrets.LeaseManager
	transit      secrets.TransitBackend
}

// NewSecretsServer creates a new SecretsServer.
// Any dependency may be nil — RPCs return codes.Unavailable if the required dep is nil.
func NewSecretsServer(broker *secrets.SecretBroker, leaseManager secrets.LeaseManager, transit secrets.TransitBackend) *SecretsServer {
	return &SecretsServer{
		broker:       broker,
		leaseManager: leaseManager,
		transit:      transit,
	}
}

// GetSecret retrieves a secret by path.
func (s *SecretsServer) GetSecret(ctx context.Context, req *pb.GetSecretRequest) (*pb.GetSecretResponse, error) {
	if s.broker == nil {
		return nil, status.Error(codes.Unavailable, "secrets backend not available")
	}
	if req.Path == "" {
		return nil, status.Error(codes.InvalidArgument, "path is required")
	}

	secretReq := &secrets.SecretRequest{
		Path:    req.Path,
		Version: int(req.Version),
	}

	secret, err := s.broker.Read(ctx, secretReq)
	if err != nil {
		return nil, secretErrorToGRPCStatus(err)
	}

	return &pb.GetSecretResponse{
		Secret: secretToProto(secret),
	}, nil
}

// ListSecrets lists secret keys under a path prefix.
func (s *SecretsServer) ListSecrets(ctx context.Context, req *pb.ListSecretsRequest) (*pb.ListSecretsResponse, error) {
	if s.broker == nil {
		return nil, status.Error(codes.Unavailable, "secrets backend not available")
	}

	path := req.Prefix
	if req.Backend != "" && req.Prefix != "" {
		path = req.Backend + "/" + req.Prefix
	} else if req.Backend != "" {
		path = req.Backend
	}

	keys, err := s.broker.List(ctx, path)
	if err != nil {
		return nil, secretErrorToGRPCStatus(err)
	}

	if keys == nil {
		keys = []string{}
	}

	return &pb.ListSecretsResponse{
		Keys: keys,
	}, nil
}

// WriteSecret writes a secret to a backend.
func (s *SecretsServer) WriteSecret(ctx context.Context, req *pb.WriteSecretRequest) (*pb.WriteSecretResponse, error) {
	if s.broker == nil {
		return nil, status.Error(codes.Unavailable, "secrets backend not available")
	}
	if req.Path == "" {
		return nil, status.Error(codes.InvalidArgument, "path is required")
	}
	if req.Data == nil {
		return nil, status.Error(codes.InvalidArgument, "data is required")
	}

	backend, err := s.resolveBackendByPath(req.Path)
	if err != nil {
		return nil, secretErrorToGRPCStatus(err)
	}

	wb, ok := backend.(WritableBackend)
	if !ok {
		return nil, status.Error(codes.Unimplemented, "backend does not support writes")
	}

	data := req.Data.AsMap()
	if err := wb.Write(req.Path, data, req.Metadata); err != nil {
		return nil, secretErrorToGRPCStatus(err)
	}

	return &pb.WriteSecretResponse{
		Path:    req.Path,
		Written: true,
	}, nil
}

// DeleteSecret deletes a secret from a backend.
func (s *SecretsServer) DeleteSecret(ctx context.Context, req *pb.DeleteSecretRequest) (*pb.DeleteSecretResponse, error) {
	if s.broker == nil {
		return nil, status.Error(codes.Unavailable, "secrets backend not available")
	}
	if req.Path == "" {
		return nil, status.Error(codes.InvalidArgument, "path is required")
	}

	backend, err := s.resolveBackendByPath(req.Path)
	if err != nil {
		return nil, secretErrorToGRPCStatus(err)
	}

	db, ok := backend.(DeletableBackend)
	if !ok {
		return nil, status.Error(codes.Unimplemented, "backend does not support deletes")
	}

	if err := db.Delete(req.Path); err != nil {
		return nil, secretErrorToGRPCStatus(err)
	}

	return &pb.DeleteSecretResponse{
		Path:    req.Path,
		Deleted: true,
	}, nil
}

// GetLease retrieves a lease by ID.
func (s *SecretsServer) GetLease(ctx context.Context, req *pb.GetLeaseRequest) (*pb.GetLeaseResponse, error) {
	if s.leaseManager == nil {
		return nil, status.Error(codes.Unavailable, "lease manager not available")
	}
	if req.LeaseId == "" {
		return nil, status.Error(codes.InvalidArgument, "lease_id is required")
	}

	lease, err := s.leaseManager.Get(ctx, req.LeaseId)
	if err != nil {
		return nil, secretErrorToGRPCStatus(err)
	}

	return &pb.GetLeaseResponse{
		Lease: leaseToProto(lease),
	}, nil
}

// ListLeases lists tracked leases with optional filtering.
func (s *SecretsServer) ListLeases(ctx context.Context, req *pb.ListLeasesRequest) (*pb.ListLeasesResponse, error) {
	if s.leaseManager == nil {
		return nil, status.Error(codes.Unavailable, "lease manager not available")
	}

	var leases []*secrets.Lease
	var err error

	switch {
	case req.PathPrefix != "":
		leases, err = s.leaseManager.ListByPath(ctx, req.PathPrefix)
	case req.Backend != "":
		bt, parseErr := secrets.ParseBackendType(req.Backend)
		if parseErr != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid backend type: %s", req.Backend)
		}
		leases, err = s.leaseManager.ListByBackend(ctx, bt)
	default:
		leases, err = s.leaseManager.List(ctx)
	}
	if err != nil {
		return nil, secretErrorToGRPCStatus(err)
	}

	// Apply pagination
	offset := parsePageToken(req.PageToken)
	pageSize := int(req.PageSize)
	if pageSize <= 0 {
		pageSize = 100
	}

	totalCount := len(leases)
	start := offset
	if start > totalCount {
		start = totalCount
	}
	end := start + pageSize
	if end > totalCount {
		end = totalCount
	}

	pbLeases := make([]*pb.LeaseInfo, 0, end-start)
	for _, lease := range leases[start:end] {
		pbLeases = append(pbLeases, leaseToProto(lease))
	}

	var nextPageToken string
	if end < totalCount {
		nextPageToken = encodePageToken(end)
	}

	return &pb.ListLeasesResponse{
		Leases:        pbLeases,
		NextPageToken: nextPageToken,
		TotalCount:    int32(totalCount), //nolint:gosec // G115: lease count bounded by backend
	}, nil
}

// RenewLease renews a lease.
func (s *SecretsServer) RenewLease(ctx context.Context, req *pb.RenewLeaseRequest) (*pb.RenewLeaseResponse, error) {
	if s.leaseManager == nil {
		return nil, status.Error(codes.Unavailable, "lease manager not available")
	}
	if req.LeaseId == "" {
		return nil, status.Error(codes.InvalidArgument, "lease_id is required")
	}

	increment := time.Duration(req.IncrementSeconds) * time.Second
	lease, err := s.leaseManager.Renew(ctx, req.LeaseId, increment)
	if err != nil {
		return nil, secretErrorToGRPCStatus(err)
	}

	return &pb.RenewLeaseResponse{
		Lease: leaseToProto(lease),
	}, nil
}

// RevokeLease revokes a lease.
func (s *SecretsServer) RevokeLease(ctx context.Context, req *pb.RevokeLeaseRequest) (*pb.RevokeLeaseResponse, error) {
	if s.leaseManager == nil {
		return nil, status.Error(codes.Unavailable, "lease manager not available")
	}
	if req.LeaseId == "" {
		return nil, status.Error(codes.InvalidArgument, "lease_id is required")
	}

	if err := s.leaseManager.Revoke(ctx, req.LeaseId); err != nil {
		return nil, secretErrorToGRPCStatus(err)
	}

	return &pb.RevokeLeaseResponse{
		LeaseId: req.LeaseId,
		Revoked: true,
	}, nil
}

// Encrypt encrypts plaintext using a transit key.
func (s *SecretsServer) Encrypt(ctx context.Context, req *pb.EncryptRequest) (*pb.EncryptResponse, error) {
	if s.transit == nil {
		return nil, status.Error(codes.Unavailable, "transit backend not available")
	}
	if req.KeyName == "" {
		return nil, status.Error(codes.InvalidArgument, "key_name is required")
	}

	transitReq := &secrets.TransitRequest{
		Operation:  secrets.TransitOperationEncrypt,
		KeyName:    req.KeyName,
		Plaintext:  req.Plaintext,
		Context:    req.Context,
		KeyVersion: int(req.KeyVersion),
	}

	resp, err := s.transit.Encrypt(ctx, transitReq)
	if err != nil {
		return nil, secretErrorToGRPCStatus(err)
	}

	return &pb.EncryptResponse{
		Ciphertext: resp.Ciphertext,
		KeyVersion: int32(resp.KeyVersion), //nolint:gosec // G115: key version is small
	}, nil
}

// Decrypt decrypts ciphertext using a transit key.
func (s *SecretsServer) Decrypt(ctx context.Context, req *pb.DecryptRequest) (*pb.DecryptResponse, error) {
	if s.transit == nil {
		return nil, status.Error(codes.Unavailable, "transit backend not available")
	}
	if req.KeyName == "" {
		return nil, status.Error(codes.InvalidArgument, "key_name is required")
	}

	transitReq := &secrets.TransitRequest{
		Operation:  secrets.TransitOperationDecrypt,
		KeyName:    req.KeyName,
		Ciphertext: req.Ciphertext,
		Context:    req.Context,
	}

	resp, err := s.transit.Decrypt(ctx, transitReq)
	if err != nil {
		return nil, secretErrorToGRPCStatus(err)
	}

	return &pb.DecryptResponse{
		Plaintext:  resp.Plaintext,
		KeyVersion: int32(resp.KeyVersion), //nolint:gosec // G115: key version is small
	}, nil
}

// Sign signs input data using a transit key.
func (s *SecretsServer) Sign(ctx context.Context, req *pb.SignRequest) (*pb.SignResponse, error) {
	if s.transit == nil {
		return nil, status.Error(codes.Unavailable, "transit backend not available")
	}
	if req.KeyName == "" {
		return nil, status.Error(codes.InvalidArgument, "key_name is required")
	}

	transitReq := &secrets.TransitRequest{
		Operation:  secrets.TransitOperationSign,
		KeyName:    req.KeyName,
		Input:      req.Input,
		Algorithm:  req.Algorithm,
		KeyVersion: int(req.KeyVersion),
	}

	resp, err := s.transit.Sign(ctx, transitReq)
	if err != nil {
		return nil, secretErrorToGRPCStatus(err)
	}

	return &pb.SignResponse{
		Signature:  resp.Signature,
		KeyVersion: int32(resp.KeyVersion), //nolint:gosec // G115: key version is small
	}, nil
}

// Verify verifies a signature against input data.
func (s *SecretsServer) Verify(ctx context.Context, req *pb.VerifyRequest) (*pb.VerifyResponse, error) {
	if s.transit == nil {
		return nil, status.Error(codes.Unavailable, "transit backend not available")
	}
	if req.KeyName == "" {
		return nil, status.Error(codes.InvalidArgument, "key_name is required")
	}

	transitReq := &secrets.TransitRequest{
		Operation: secrets.TransitOperationVerify,
		KeyName:   req.KeyName,
		Input:     req.Input,
		Signature: req.Signature,
		Algorithm: req.Algorithm,
	}

	resp, err := s.transit.Verify(ctx, transitReq)
	if err != nil {
		return nil, secretErrorToGRPCStatus(err)
	}

	return &pb.VerifyResponse{
		Valid: resp.Valid,
	}, nil
}

// resolveBackendByPath extracts the backend name from the first path segment.
func (s *SecretsServer) resolveBackendByPath(path string) (secrets.SecretBackend, error) {
	parts := strings.SplitN(path, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		return nil, secrets.ErrBackendNotFound
	}
	backend, err := s.broker.GetBackend(parts[0])
	if err != nil {
		backend, err = s.broker.GetBackend(path)
		if err != nil {
			return nil, secrets.ErrBackendNotFound
		}
	}
	return backend, nil
}

// secretToProto converts an internal Secret to a protobuf SecretData.
func secretToProto(secret *secrets.Secret) *pb.SecretData {
	sd := &pb.SecretData{
		Path:      secret.Path,
		Backend:   string(secret.Backend),
		Type:      string(secret.Type),
		Metadata:  secret.Metadata,
		Version:   int32(secret.Version), //nolint:gosec // G115: version is small
		Renewable: secret.Renewable,
	}

	if secret.Data != nil {
		data, err := structpb.NewStruct(secret.Data)
		if err == nil {
			sd.Data = data
		}
	}

	if !secret.CreatedAt.IsZero() {
		sd.CreatedAt = timestamppb.New(secret.CreatedAt)
	}
	if !secret.ExpiresAt.IsZero() {
		sd.ExpiresAt = timestamppb.New(secret.ExpiresAt)
	}
	if secret.Lease != nil {
		sd.LeaseId = secret.Lease.ID
	}

	return sd
}

// leaseToProto converts an internal Lease to a protobuf LeaseInfo.
func leaseToProto(lease *secrets.Lease) *pb.LeaseInfo {
	li := &pb.LeaseInfo{
		Id:            lease.ID,
		SecretPath:    lease.SecretPath,
		Backend:       string(lease.Backend),
		State:         string(lease.State),
		TtlSeconds:    int64(lease.TTL.Seconds()),
		MaxTtlSeconds: int64(lease.MaxTTL.Seconds()),
		RenewalCount:  int32(lease.RenewalCount), //nolint:gosec // G115: renewal count is small
		Renewable:     lease.Renewable,
		Revocable:     lease.Revocable,
		Metadata:      lease.Metadata,
	}

	if !lease.IssuedAt.IsZero() {
		li.IssuedAt = timestamppb.New(lease.IssuedAt)
	}
	if !lease.ExpiresAt.IsZero() {
		li.ExpiresAt = timestamppb.New(lease.ExpiresAt)
	}
	if !lease.LastRenewal.IsZero() {
		li.LastRenewal = timestamppb.New(lease.LastRenewal)
	}

	return li
}

// secretErrorToGRPCStatus maps internal secrets errors to gRPC status errors.
func secretErrorToGRPCStatus(err error) error {
	var secretErr *secrets.SecretError
	if errors.As(err, &secretErr) {
		switch secretErr.Code {
		case secrets.ErrorCodeNotFound, secrets.ErrorCodeVersionNotFound:
			return status.Error(codes.NotFound, secretErr.Message)
		case secrets.ErrorCodeAccessDenied, secrets.ErrorCodeAuthorization:
			return status.Error(codes.PermissionDenied, secretErr.Message)
		case secrets.ErrorCodeAuthentication:
			return status.Error(codes.Unauthenticated, secretErr.Message)
		case secrets.ErrorCodeInvalidRequest, secrets.ErrorCodeConfiguration:
			return status.Error(codes.InvalidArgument, secretErr.Message)
		case secrets.ErrorCodeRateLimit:
			return status.Error(codes.ResourceExhausted, secretErr.Message)
		case secrets.ErrorCodeBackendUnavailable:
			return status.Error(codes.Unavailable, secretErr.Message)
		case secrets.ErrorCodeTimeout:
			return status.Error(codes.DeadlineExceeded, secretErr.Message)
		default:
			return status.Error(codes.Internal, secretErr.Message)
		}
	}

	switch {
	case errors.Is(err, secrets.ErrSecretNotFound), errors.Is(err, secrets.ErrLeaseNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, secrets.ErrAccessDenied):
		return status.Error(codes.PermissionDenied, err.Error())
	case errors.Is(err, secrets.ErrLeaseNotRenewable), errors.Is(err, secrets.ErrInvalidPath):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, secrets.ErrBackendUnavailable):
		return status.Error(codes.Unavailable, err.Error())
	case errors.Is(err, secrets.ErrLeaseExpired):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, secrets.ErrRotationInProgress):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, secrets.ErrBackendNotFound):
		return status.Error(codes.NotFound, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}
