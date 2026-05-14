package controlplane

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.keystone-core.io/keystone-core/internal/secrets"
	"go.keystone-core.io/keystone-core/internal/state"
	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

// SecretsGRPCServer implements [v1.SecretsServiceServer] by delegating
// to a [*secrets.Broker] (KV + dynamic + leases) + a
// [secrets.TransitBackend] (encrypt/decrypt/sign/verify; v1.0 only
// from the Vault backend) + a [*secrets.LeaseManager] (lease list /
// get for the operator surface).
//
// Any of the three may be nil when the corresponding feature is not
// configured — the affected RPCs return codes.Unavailable.
type SecretsGRPCServer struct {
	v1.UnimplementedSecretsServiceServer

	Broker  *secrets.Broker
	Transit secrets.TransitBackend
	Leases  *secrets.LeaseManager
}

var _ v1.SecretsServiceServer = (*SecretsGRPCServer)(nil)

// NewSecretsGRPCServer wires the gRPC adapter.
func NewSecretsGRPCServer(broker *secrets.Broker, transit secrets.TransitBackend, leases *secrets.LeaseManager) *SecretsGRPCServer {
	return &SecretsGRPCServer{Broker: broker, Transit: transit, Leases: leases}
}

// ---- KV RPCs -----------------------------------------------------

// GetSecret reads a secret. Returns codes.NotFound on miss,
// codes.InvalidArgument on routing/cap failures.
func (s *SecretsGRPCServer) GetSecret(ctx context.Context, req *v1.GetSecretRequest) (*v1.GetSecretResponse, error) {
	if err := s.requireBroker(); err != nil {
		return nil, err
	}
	if req.GetPath() == "" {
		return nil, status.Error(codes.InvalidArgument, "path is required")
	}
	br := secrets.GetSecretRequest{Path: req.GetPath()}
	if v := req.GetVersion(); v != "" {
		parsed, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "version %q is not a valid uint", v)
		}
		br.Version = parsed
	}
	secret, err := s.Broker.GetSecret(ctx, br)
	if err != nil {
		return nil, secretsErrToStatus(err)
	}
	return &v1.GetSecretResponse{
		Metadata: metadataFromSecret(secret),
		Data:     stringMapFromData(secret.Data),
	}, nil
}

// WriteSecret writes a secret.
func (s *SecretsGRPCServer) WriteSecret(ctx context.Context, req *v1.WriteSecretRequest) (*v1.WriteSecretResponse, error) {
	if err := s.requireBroker(); err != nil {
		return nil, err
	}
	if req.GetPath() == "" {
		return nil, status.Error(codes.InvalidArgument, "path is required")
	}

	data := make(map[string]any, len(req.GetData()))
	for k, v := range req.GetData() {
		data[k] = v
	}
	meta := make(map[string]string, len(req.GetLabels())+1)
	for k, v := range req.GetLabels() {
		meta[k] = v
	}
	if ttl := req.GetTtlSeconds(); ttl > 0 {
		meta["ttl_seconds"] = strconv.Itoa(int(ttl))
	}

	out, err := s.Broker.WriteSecret(ctx, secrets.WriteSecretRequest{
		Path:     req.GetPath(),
		Data:     data,
		Metadata: meta,
	})
	if err != nil {
		return nil, secretsErrToStatus(err)
	}
	return &v1.WriteSecretResponse{Metadata: metadataFromSecret(out)}, nil
}

// ListSecrets enumerates paths under the prefix; metadata-only.
func (s *SecretsGRPCServer) ListSecrets(ctx context.Context, req *v1.ListSecretsRequest) (*v1.ListSecretsResponse, error) {
	if err := s.requireBroker(); err != nil {
		return nil, err
	}
	resp, err := s.Broker.ListSecrets(ctx, secrets.ListSecretsRequest{
		Prefix: req.GetPathPrefix(),
		Limit:  int(req.GetPageSize()),
		Cursor: req.GetPageToken(),
	})
	if err != nil {
		return nil, secretsErrToStatus(err)
	}
	entries := make([]*v1.SecretMetadata, 0, len(resp.Entries))
	for _, e := range resp.Entries {
		entries = append(entries, &v1.SecretMetadata{
			Path:      e.Path,
			Version:   strconv.FormatUint(e.Version, 10),
			Labels:    e.Metadata,
			UpdatedAt: timestampFrom(e.UpdatedAt),
		})
	}
	return &v1.ListSecretsResponse{
		Secrets:       entries,
		NextPageToken: resp.NextCursor,
	}, nil
}

// DeleteSecret removes a secret.
func (s *SecretsGRPCServer) DeleteSecret(ctx context.Context, req *v1.DeleteSecretRequest) (*v1.DeleteSecretResponse, error) {
	if err := s.requireBroker(); err != nil {
		return nil, err
	}
	if req.GetPath() == "" {
		return nil, status.Error(codes.InvalidArgument, "path is required")
	}
	if err := s.Broker.DeleteSecret(ctx, secrets.DeleteSecretRequest{Path: req.GetPath()}); err != nil {
		return nil, secretsErrToStatus(err)
	}
	return &v1.DeleteSecretResponse{}, nil
}

// ---- Lease RPCs --------------------------------------------------

// GetLease returns the LeaseManager's persisted record for the ID.
// Returns codes.NotFound on miss.
func (s *SecretsGRPCServer) GetLease(ctx context.Context, req *v1.GetLeaseRequest) (*v1.GetLeaseResponse, error) {
	if s.Leases == nil {
		return nil, status.Error(codes.Unavailable, "lease manager not configured")
	}
	if req.GetLeaseId() == "" {
		return nil, status.Error(codes.InvalidArgument, "lease_id is required")
	}
	lease, err := s.Leases.GetLease(ctx, req.GetLeaseId())
	if err != nil {
		if errors.Is(err, state.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "lease %q: %v", req.GetLeaseId(), err)
		}
		return nil, status.Errorf(codes.Internal, "GetLease: %v", err)
	}
	return &v1.GetLeaseResponse{Lease: leaseToProto(lease)}, nil
}

// ListLeases returns leases matching the filter.
func (s *SecretsGRPCServer) ListLeases(ctx context.Context, req *v1.ListLeasesRequest) (*v1.ListLeasesResponse, error) {
	if s.Leases == nil {
		return nil, status.Error(codes.Unavailable, "lease manager not configured")
	}
	filter := state.LeaseFilter{
		PathPrefix: req.GetSecretPath(),
		Limit:      int(req.GetPageSize()),
	}
	if tok := req.GetPageToken(); tok != "" {
		off, err := strconv.Atoi(tok)
		if err != nil || off < 0 {
			return nil, status.Errorf(codes.InvalidArgument, "page_token %q is not a valid offset", tok)
		}
		filter.Offset = off
	}
	leases, err := s.Leases.List(ctx, filter)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "ListLeases: %v", err)
	}
	out := make([]*v1.Lease, 0, len(leases))
	for _, l := range leases {
		out = append(out, leaseToProto(l))
	}
	return &v1.ListLeasesResponse{Leases: out}, nil
}

// RenewLease extends a lease's TTL via the broker (which also fires
// the audit event for the renewal).
func (s *SecretsGRPCServer) RenewLease(ctx context.Context, req *v1.RenewLeaseRequest) (*v1.RenewLeaseResponse, error) {
	if err := s.requireBroker(); err != nil {
		return nil, err
	}
	if req.GetLeaseId() == "" {
		return nil, status.Error(codes.InvalidArgument, "lease_id is required")
	}
	info, err := s.Broker.RenewLease(ctx, secrets.RenewLeaseRequest{LeaseID: req.GetLeaseId()})
	if err != nil {
		return nil, secretsErrToStatus(err)
	}
	return &v1.RenewLeaseResponse{Lease: leaseInfoToProto(info)}, nil
}

// RevokeLease tears down a lease.
func (s *SecretsGRPCServer) RevokeLease(ctx context.Context, req *v1.RevokeLeaseRequest) (*v1.RevokeLeaseResponse, error) {
	if err := s.requireBroker(); err != nil {
		return nil, err
	}
	if req.GetLeaseId() == "" {
		return nil, status.Error(codes.InvalidArgument, "lease_id is required")
	}
	if err := s.Broker.RevokeLease(ctx, secrets.RevokeLeaseRequest{LeaseID: req.GetLeaseId()}); err != nil {
		return nil, secretsErrToStatus(err)
	}
	return &v1.RevokeLeaseResponse{}, nil
}

// ---- Transit RPCs ------------------------------------------------

// Encrypt encrypts a single plaintext via the configured
// TransitBackend.
func (s *SecretsGRPCServer) Encrypt(ctx context.Context, req *v1.EncryptRequest) (*v1.EncryptResponse, error) {
	if s.Transit == nil {
		return nil, status.Error(codes.Unavailable, "transit backend not configured")
	}
	if req.GetKeyName() == "" {
		return nil, status.Error(codes.InvalidArgument, "key_name is required")
	}
	resp, err := s.Transit.Encrypt(ctx, secrets.EncryptRequest{
		Key:   req.GetKeyName(),
		Items: []secrets.EncryptInput{{Plaintext: req.GetPlaintext(), Context: req.GetContext()}},
	})
	if err != nil {
		return nil, secretsErrToStatus(err)
	}
	if len(resp.Results) == 0 || resp.Results[0].Err != "" {
		return nil, status.Errorf(codes.Internal, "encrypt: %s", firstResultErr(resp.Results))
	}
	r := resp.Results[0]
	return &v1.EncryptResponse{
		Ciphertext: []byte(r.Ciphertext),
		KeyVersion: strconv.Itoa(r.KeyVersion),
	}, nil
}

// Decrypt decrypts a single ciphertext.
func (s *SecretsGRPCServer) Decrypt(ctx context.Context, req *v1.DecryptRequest) (*v1.DecryptResponse, error) {
	if s.Transit == nil {
		return nil, status.Error(codes.Unavailable, "transit backend not configured")
	}
	if req.GetKeyName() == "" {
		return nil, status.Error(codes.InvalidArgument, "key_name is required")
	}
	resp, err := s.Transit.Decrypt(ctx, secrets.DecryptRequest{
		Key:   req.GetKeyName(),
		Items: []secrets.DecryptInput{{Ciphertext: string(req.GetCiphertext()), Context: req.GetContext()}},
	})
	if err != nil {
		return nil, secretsErrToStatus(err)
	}
	if len(resp.Results) == 0 || resp.Results[0].Err != "" {
		return nil, status.Errorf(codes.Internal, "decrypt: %s", firstDecryptErr(resp.Results))
	}
	return &v1.DecryptResponse{Plaintext: resp.Results[0].Plaintext}, nil
}

// Sign signs a single payload.
func (s *SecretsGRPCServer) Sign(ctx context.Context, req *v1.SignRequest) (*v1.SignResponse, error) {
	if s.Transit == nil {
		return nil, status.Error(codes.Unavailable, "transit backend not configured")
	}
	if req.GetKeyName() == "" {
		return nil, status.Error(codes.InvalidArgument, "key_name is required")
	}
	hashAlgo, sigAlgo := splitVaultSignAlgorithm(req.GetAlgorithm())
	resp, err := s.Transit.Sign(ctx, secrets.SignRequest{
		Key:                req.GetKeyName(),
		HashAlgorithm:      hashAlgo,
		SignatureAlgorithm: sigAlgo,
		Items:              []secrets.SignInput{{Input: req.GetMessage()}},
	})
	if err != nil {
		return nil, secretsErrToStatus(err)
	}
	if len(resp.Results) == 0 || resp.Results[0].Err != "" {
		return nil, status.Errorf(codes.Internal, "sign: %s", firstSignErr(resp.Results))
	}
	r := resp.Results[0]
	return &v1.SignResponse{
		Signature:  []byte(r.Signature),
		KeyVersion: strconv.Itoa(r.KeyVersion),
	}, nil
}

// Verify checks a signature.
func (s *SecretsGRPCServer) Verify(ctx context.Context, req *v1.VerifyRequest) (*v1.VerifyResponse, error) {
	if s.Transit == nil {
		return nil, status.Error(codes.Unavailable, "transit backend not configured")
	}
	if req.GetKeyName() == "" {
		return nil, status.Error(codes.InvalidArgument, "key_name is required")
	}
	hashAlgo, sigAlgo := splitVaultSignAlgorithm(req.GetAlgorithm())
	resp, err := s.Transit.Verify(ctx, secrets.VerifyRequest{
		Key:                req.GetKeyName(),
		HashAlgorithm:      hashAlgo,
		SignatureAlgorithm: sigAlgo,
		Items: []secrets.VerifyInput{
			{Input: req.GetMessage(), Signature: string(req.GetSignature())},
		},
	})
	if err != nil {
		return nil, secretsErrToStatus(err)
	}
	if len(resp.Results) == 0 {
		return nil, status.Error(codes.Internal, "verify: empty response")
	}
	return &v1.VerifyResponse{Valid: resp.Results[0].Valid}, nil
}

// ---- helpers -----------------------------------------------------

func (s *SecretsGRPCServer) requireBroker() error {
	if s.Broker == nil {
		return status.Error(codes.Unavailable, "secrets broker not configured")
	}
	return nil
}

// secretsErrToStatus funnels a secrets sentinel into a gRPC code.
// v1.0 over-coalesces ErrInvalidBackend (cap refusal / bad config /
// 403 from Vault) into codes.InvalidArgument — clients see "this
// request can't be served" without leaking backend-specific detail.
func secretsErrToStatus(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, secrets.ErrSecretNotFound):
		return status.Errorf(codes.NotFound, "%v", err)
	case errors.Is(err, secrets.ErrLeaseNotFound):
		return status.Errorf(codes.NotFound, "%v", err)
	case errors.Is(err, secrets.ErrLeaseExpired):
		return status.Errorf(codes.FailedPrecondition, "%v", err)
	case errors.Is(err, secrets.ErrLeaseNotRenewable):
		return status.Errorf(codes.FailedPrecondition, "%v", err)
	case errors.Is(err, secrets.ErrBackendNotStarted):
		return status.Errorf(codes.Unavailable, "%v", err)
	case errors.Is(err, secrets.ErrInvalidBackend):
		return status.Errorf(codes.InvalidArgument, "%v", err)
	default:
		return status.Errorf(codes.Internal, "%v", err)
	}
}

// metadataFromSecret projects a [secrets.Secret] into the
// [v1.SecretMetadata] proto. Version is stringified per the proto's
// wire shape.
func metadataFromSecret(s *secrets.Secret) *v1.SecretMetadata {
	if s == nil {
		return nil
	}
	return &v1.SecretMetadata{
		Path:      s.Path,
		Version:   strconv.FormatUint(s.Version, 10),
		Labels:    s.Metadata,
		CreatedAt: timestampFrom(s.CreatedAt),
		UpdatedAt: timestampFrom(s.UpdatedAt),
	}
}

// stringMapFromData coerces [secrets.Secret.Data] (map[string]any)
// into the proto's wire shape (map[string]string). Non-string leaves
// are formatted via fmt.Sprint — Vault KV almost always stores
// values as strings; structured payloads would round-trip lossily
// and are an explicit v1.0 limitation.
func stringMapFromData(in map[string]any) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		switch t := v.(type) {
		case string:
			out[k] = t
		default:
			out[k] = fmt.Sprint(v)
		}
	}
	return out
}

// leaseToProto projects a [secrets.Lease] into the proto wire shape.
// Slim by design — the proto's Lease doesn't carry Backend / Strategy
// / RenewCount.
func leaseToProto(l secrets.Lease) *v1.Lease {
	return &v1.Lease{
		Id:         l.ID,
		SecretPath: l.SecretPath,
		Holder:     l.IssuedFor,
		IssuedAt:   timestampFrom(l.IssuedAt),
		ExpiresAt:  timestampFrom(l.ExpiresAt),
		Renewable:  l.Renewable,
	}
}

// leaseInfoToProto projects a renewed-lease info into the proto's
// Lease shape. Used by RenewLease which returns the post-renewal
// snapshot, not a full Lease.
func leaseInfoToProto(info *secrets.LeaseInfo) *v1.Lease {
	if info == nil {
		return nil
	}
	return &v1.Lease{
		Id:         info.ID,
		SecretPath: info.SecretPath,
		IssuedAt:   timestampFrom(info.IssuedAt),
		ExpiresAt:  timestampFrom(info.ExpiresAt),
		Renewable:  info.Renewable,
	}
}

// firstResultErr extracts the first non-empty Err from a batch
// response — singular Encrypt/Decrypt RPC failures need a useful
// reason in the gRPC status message.
func firstResultErr(rs []secrets.EncryptResult) string {
	for _, r := range rs {
		if r.Err != "" {
			return r.Err
		}
	}
	return "empty response"
}

func firstDecryptErr(rs []secrets.DecryptResult) string {
	for _, r := range rs {
		if r.Err != "" {
			return r.Err
		}
	}
	return "empty response"
}

func firstSignErr(rs []secrets.SignResult) string {
	for _, r := range rs {
		if r.Err != "" {
			return r.Err
		}
	}
	return "empty response"
}

// splitVaultSignAlgorithm splits the proto's "algorithm" field
// (e.g. "rsa-pss-sha256", "ed25519", "sha2-256") into Vault's
// (HashAlgorithm, SignatureAlgorithm) pair. For ECDSA / Ed25519
// keys the algorithm is implicit, so we let Vault default by
// returning empty strings; for RSA-PSS we split.
//
// v1.0 is lenient — unknown algorithms pass through verbatim as the
// hash algorithm so operators using RSA-PKCS1v15 or alternate digests
// can drive Vault directly. The proto field's contract isn't pinned
// at the wire layer yet (v1.x ROADMAP entry tracks tightening it).
func splitVaultSignAlgorithm(algo string) (hash, sig string) {
	switch algo {
	case "":
		return "", ""
	case "ed25519":
		// Ed25519 keys take no hash + no signature algorithm.
		return "", ""
	case "rsa-pss-sha256":
		return "sha2-256", "pss"
	case "rsa-pss-sha384":
		return "sha2-384", "pss"
	case "rsa-pss-sha512":
		return "sha2-512", "pss"
	case "rsa-pkcs1v15-sha256":
		return "sha2-256", "pkcs1v15"
	case "rsa-pkcs1v15-sha384":
		return "sha2-384", "pkcs1v15"
	case "rsa-pkcs1v15-sha512":
		return "sha2-512", "pkcs1v15"
	default:
		// Pass through — Vault accepts the algo string as the hash
		// suffix on the URL path; non-matching algorithms surface as
		// a Vault 400, which the error translation propagates.
		return algo, ""
	}
}

// timestampFrom returns a non-nil [timestamppb.Timestamp] when t is
// non-zero. The proto convention is to omit timestamps that we don't
// know.
func timestampFrom(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}
	return timestamppb.New(t)
}
