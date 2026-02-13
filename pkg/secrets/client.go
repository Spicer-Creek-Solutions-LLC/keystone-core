package secrets

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/structpb"

	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
)

// Client provides access to the Keystone Core secrets API over gRPC.
type Client struct {
	conn   *grpc.ClientConn
	client pb.SecretsServiceClient
}

// NewClient creates a new secrets client that connects to the given address.
// The caller must call Close when done.
func NewClient(addr string, opts ...grpc.DialOption) (*Client, error) {
	if len(opts) == 0 {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	conn, err := grpc.NewClient(addr, opts...)
	if err != nil {
		return nil, err
	}
	return &Client{
		conn:   conn,
		client: pb.NewSecretsServiceClient(conn),
	}, nil
}

// NewClientFromConn creates a new secrets client from an existing gRPC connection.
// The caller retains ownership of the connection; Close is a no-op.
func NewClientFromConn(conn grpc.ClientConnInterface) *Client {
	return &Client{
		client: pb.NewSecretsServiceClient(conn),
	}
}

// Close closes the underlying gRPC connection.
// No-op if the client was created with NewClientFromConn.
func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// GetSecret retrieves a secret by path. Set version to 0 for the latest version.
func (c *Client) GetSecret(ctx context.Context, path string, version int) (*Secret, error) {
	resp, err := c.client.GetSecret(ctx, &pb.GetSecretRequest{
		Path:    path,
		Version: int32(version), //nolint:gosec // G115: version is small
	})
	if err != nil {
		return nil, grpcStatusToError(err)
	}
	return secretFromProto(resp.Secret), nil
}

// ListSecrets lists secret keys under a path prefix.
func (c *Client) ListSecrets(ctx context.Context, backend, prefix string, pageSize int32, pageToken string) (*ListSecretsResult, error) {
	resp, err := c.client.ListSecrets(ctx, &pb.ListSecretsRequest{
		Backend:   backend,
		Prefix:    prefix,
		PageSize:  pageSize,
		PageToken: pageToken,
	})
	if err != nil {
		return nil, grpcStatusToError(err)
	}
	keys := resp.Keys
	if keys == nil {
		keys = []string{}
	}
	return &ListSecretsResult{
		Keys:          keys,
		NextPageToken: resp.NextPageToken,
	}, nil
}

// WriteSecret writes a secret to a backend.
func (c *Client) WriteSecret(ctx context.Context, path string, data map[string]interface{}, metadata map[string]string) error {
	pbData, err := structpb.NewStruct(data)
	if err != nil {
		return err
	}
	_, err = c.client.WriteSecret(ctx, &pb.WriteSecretRequest{
		Path:     path,
		Data:     pbData,
		Metadata: metadata,
	})
	if err != nil {
		return grpcStatusToError(err)
	}
	return nil
}

// DeleteSecret deletes a secret from a backend.
func (c *Client) DeleteSecret(ctx context.Context, path string) error {
	_, err := c.client.DeleteSecret(ctx, &pb.DeleteSecretRequest{
		Path: path,
	})
	if err != nil {
		return grpcStatusToError(err)
	}
	return nil
}

// GetLease retrieves a lease by ID.
func (c *Client) GetLease(ctx context.Context, leaseID string) (*Lease, error) {
	resp, err := c.client.GetLease(ctx, &pb.GetLeaseRequest{
		LeaseId: leaseID,
	})
	if err != nil {
		return nil, grpcStatusToError(err)
	}
	return leaseFromProto(resp.Lease), nil
}

// ListLeases lists tracked leases with optional filtering.
func (c *Client) ListLeases(ctx context.Context, pathPrefix, backend string, pageSize int32, pageToken string) (*ListLeasesResult, error) {
	resp, err := c.client.ListLeases(ctx, &pb.ListLeasesRequest{
		PathPrefix: pathPrefix,
		Backend:    backend,
		PageSize:   pageSize,
		PageToken:  pageToken,
	})
	if err != nil {
		return nil, grpcStatusToError(err)
	}
	leases := make([]*Lease, len(resp.Leases))
	for i, l := range resp.Leases {
		leases[i] = leaseFromProto(l)
	}
	return &ListLeasesResult{
		Leases:        leases,
		NextPageToken: resp.NextPageToken,
		TotalCount:    int(resp.TotalCount),
	}, nil
}

// RenewLease renews a lease. Set increment to 0 to use the original TTL.
func (c *Client) RenewLease(ctx context.Context, leaseID string, increment time.Duration) (*Lease, error) {
	resp, err := c.client.RenewLease(ctx, &pb.RenewLeaseRequest{
		LeaseId:          leaseID,
		IncrementSeconds: int64(increment.Seconds()),
	})
	if err != nil {
		return nil, grpcStatusToError(err)
	}
	return leaseFromProto(resp.Lease), nil
}

// RevokeLease revokes a lease.
func (c *Client) RevokeLease(ctx context.Context, leaseID string) error {
	_, err := c.client.RevokeLease(ctx, &pb.RevokeLeaseRequest{
		LeaseId: leaseID,
	})
	if err != nil {
		return grpcStatusToError(err)
	}
	return nil
}

// Encrypt encrypts plaintext using a transit key. Set keyVersion to 0 for the latest version.
func (c *Client) Encrypt(ctx context.Context, keyName string, plaintext, aadContext []byte, keyVersion int) (*EncryptResult, error) {
	resp, err := c.client.Encrypt(ctx, &pb.EncryptRequest{
		KeyName:    keyName,
		Plaintext:  plaintext,
		Context:    aadContext,
		KeyVersion: int32(keyVersion), //nolint:gosec // G115: key version is small
	})
	if err != nil {
		return nil, grpcStatusToError(err)
	}
	return &EncryptResult{
		Ciphertext: resp.Ciphertext,
		KeyVersion: int(resp.KeyVersion),
	}, nil
}

// Decrypt decrypts ciphertext using a transit key.
func (c *Client) Decrypt(ctx context.Context, keyName, ciphertext string, aadContext []byte) (*DecryptResult, error) {
	resp, err := c.client.Decrypt(ctx, &pb.DecryptRequest{
		KeyName:    keyName,
		Ciphertext: ciphertext,
		Context:    aadContext,
	})
	if err != nil {
		return nil, grpcStatusToError(err)
	}
	return &DecryptResult{
		Plaintext:  resp.Plaintext,
		KeyVersion: int(resp.KeyVersion),
	}, nil
}

// Sign signs input data using a transit key. Set keyVersion to 0 for the latest version.
func (c *Client) Sign(ctx context.Context, keyName string, input []byte, algorithm string, keyVersion int) (*SignResult, error) {
	resp, err := c.client.Sign(ctx, &pb.SignRequest{
		KeyName:    keyName,
		Input:      input,
		Algorithm:  algorithm,
		KeyVersion: int32(keyVersion), //nolint:gosec // G115: key version is small
	})
	if err != nil {
		return nil, grpcStatusToError(err)
	}
	return &SignResult{
		Signature:  resp.Signature,
		KeyVersion: int(resp.KeyVersion),
	}, nil
}

// Verify verifies a signature against input data using a transit key.
func (c *Client) Verify(ctx context.Context, keyName string, input []byte, signature, algorithm string) (bool, error) {
	resp, err := c.client.Verify(ctx, &pb.VerifyRequest{
		KeyName:   keyName,
		Input:     input,
		Signature: signature,
		Algorithm: algorithm,
	})
	if err != nil {
		return false, grpcStatusToError(err)
	}
	return resp.Valid, nil
}

// secretFromProto converts a proto SecretData to a public Secret.
func secretFromProto(s *pb.SecretData) *Secret {
	if s == nil {
		return nil
	}
	secret := &Secret{
		Path:      s.Path,
		Backend:   s.Backend,
		Type:      s.Type,
		Metadata:  s.Metadata,
		Version:   int(s.Version),
		Renewable: s.Renewable,
		LeaseID:   s.LeaseId,
	}
	if s.Data != nil {
		secret.Data = s.Data.AsMap()
	}
	if s.CreatedAt != nil {
		secret.CreatedAt = s.CreatedAt.AsTime()
	}
	if s.ExpiresAt != nil {
		secret.ExpiresAt = s.ExpiresAt.AsTime()
	}
	return secret
}

// leaseFromProto converts a proto LeaseInfo to a public Lease.
func leaseFromProto(l *pb.LeaseInfo) *Lease {
	if l == nil {
		return nil
	}
	lease := &Lease{
		ID:           l.Id,
		SecretPath:   l.SecretPath,
		Backend:      l.Backend,
		State:        l.State,
		TTL:          time.Duration(l.TtlSeconds) * time.Second,
		MaxTTL:       time.Duration(l.MaxTtlSeconds) * time.Second,
		RenewalCount: int(l.RenewalCount),
		Renewable:    l.Renewable,
		Revocable:    l.Revocable,
		Metadata:     l.Metadata,
	}
	if l.IssuedAt != nil {
		lease.IssuedAt = l.IssuedAt.AsTime()
	}
	if l.ExpiresAt != nil {
		lease.ExpiresAt = l.ExpiresAt.AsTime()
	}
	if l.LastRenewal != nil {
		lease.LastRenewal = l.LastRenewal.AsTime()
	}
	return lease
}
