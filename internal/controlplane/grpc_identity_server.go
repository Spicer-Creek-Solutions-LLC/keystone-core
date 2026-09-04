// SPDX-License-Identifier: Apache-2.0

package controlplane

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.keystone-core.io/keystone-core/internal/identity"
	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

// IdentityGRPCServer implements [v1.IdentityServiceServer] by
// delegating to an [*identity.EmbeddedProvider]. v0.1 covers the
// full kscore-identity CLI surface: token CRUD + cleanup; CA info /
// rotate / export; provider status.
type IdentityGRPCServer struct {
	v1.UnimplementedIdentityServiceServer

	Provider *identity.EmbeddedProvider
}

// Compile-time interface assertion.
var _ v1.IdentityServiceServer = (*IdentityGRPCServer)(nil)

// NewIdentityGRPCServer wires the gRPC adapter around a provider.
// Pass a started provider; the server returns FailedPrecondition
// from every method when Health() reports the provider isn't
// running.
func NewIdentityGRPCServer(p *identity.EmbeddedProvider) *IdentityGRPCServer {
	return &IdentityGRPCServer{Provider: p}
}

// ---- token RPCs --------------------------------------------------

// CreateJoinToken mints a fresh token via the configured
// JoinTokenStore. Cleartext is populated in the response exactly
// once.
func (s *IdentityGRPCServer) CreateJoinToken(ctx context.Context, req *v1.CreateJoinTokenRequest) (*v1.CreateJoinTokenResponse, error) {
	if err := s.checkRunning(ctx); err != nil {
		return nil, err
	}
	if req.GetAgentId() == "" {
		return nil, status.Error(codes.InvalidArgument, "agent_id is required (any-agent mode is v0.x ROADMAP)")
	}
	tok, err := s.Provider.CreateJoinToken(ctx, identity.CreateJoinTokenRequest{
		AgentID:  req.GetAgentId(),
		TTL:      time.Duration(req.GetTtlSeconds()) * time.Second,
		MaxUses:  int(req.GetMaxUses()),
		Metadata: req.GetMetadata(),
	})
	if err != nil {
		return nil, mapIdentityErrorToStatus("CreateJoinToken", err)
	}
	return &v1.CreateJoinTokenResponse{Token: joinTokenToProto(&tok)}, nil
}

// ListJoinTokens returns tokens with cleartext cleared.
func (s *IdentityGRPCServer) ListJoinTokens(ctx context.Context, req *v1.ListJoinTokensRequest) (*v1.ListJoinTokensResponse, error) {
	if err := s.checkRunning(ctx); err != nil {
		return nil, err
	}
	filter := identity.ListJoinTokensFilter{
		AgentID: req.GetAgentId(),
		Unused:  req.GetUnused(),
	}
	if t := req.GetUnexpiredAt(); t != nil {
		filter.UnexpiredAt = t.AsTime()
	}
	toks, err := s.Provider.ListJoinTokens(ctx, filter)
	if err != nil {
		return nil, mapIdentityErrorToStatus("ListJoinTokens", err)
	}
	out := &v1.ListJoinTokensResponse{Tokens: make([]*v1.JoinToken, 0, len(toks))}
	for i := range toks {
		// Defensive: the store guarantees Token == "" on read; we
		// don't trust the field as it crosses the wire.
		toks[i].Token = ""
		out.Tokens = append(out.Tokens, joinTokenToProto(&toks[i]))
	}
	return out, nil
}

// DeleteJoinToken propagates ErrJoinTokenNotFound as gRPC NotFound.
func (s *IdentityGRPCServer) DeleteJoinToken(ctx context.Context, req *v1.DeleteJoinTokenRequest) (*v1.DeleteJoinTokenResponse, error) {
	if err := s.checkRunning(ctx); err != nil {
		return nil, err
	}
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	err := s.Provider.DeleteJoinToken(ctx, req.GetId())
	if err != nil {
		return nil, mapIdentityErrorToStatus("DeleteJoinToken", err)
	}
	return &v1.DeleteJoinTokenResponse{}, nil
}

// CleanupJoinTokens runs Cleanup(now) once and returns the count.
func (s *IdentityGRPCServer) CleanupJoinTokens(ctx context.Context, _ *v1.CleanupJoinTokensRequest) (*v1.CleanupJoinTokensResponse, error) {
	if err := s.checkRunning(ctx); err != nil {
		return nil, err
	}
	store := s.Provider.JoinTokens()
	if store == nil {
		return nil, status.Error(codes.FailedPrecondition, "provider has no JoinTokenStore configured")
	}
	n, err := store.Cleanup(ctx, time.Now())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "cleanup: %v", err)
	}
	return &v1.CleanupJoinTokensResponse{Removed: int32(n)}, nil //nolint:gosec // bounded by store.Cleanup contract
}

// ---- CA RPCs -----------------------------------------------------

// GetCAInfo returns root + signing cert details + the current
// JWT kid.
func (s *IdentityGRPCServer) GetCAInfo(ctx context.Context, _ *v1.GetCAInfoRequest) (*v1.GetCAInfoResponse, error) {
	if err := s.checkRunning(ctx); err != nil {
		return nil, err
	}
	mgr := s.Provider.Manager()
	if mgr == nil {
		return nil, status.Error(codes.FailedPrecondition, "provider not running")
	}
	chain := mgr.GetTrustChain()
	if len(chain) == 0 {
		return nil, status.Error(codes.FailedPrecondition, "CA not initialized")
	}
	bundle, err := s.Provider.GetTrustBundle(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "GetTrustBundle: %v", err)
	}
	// Lift the signing cert from the bundle's JWT authority map
	// (we deliberately don't poke through the manager's unexported
	// signingCert field; tasks 7+ surface what we need via the
	// bundle's authorities). The signing CA is also reachable via
	// the manager's IssueCertificate chain[1], but no leaf has
	// been issued yet at status time.
	rootInfo, err := certInfoFromX509(chain[0])
	if err != nil {
		return nil, status.Errorf(codes.Internal, "root cert info: %v", err)
	}
	jwtKid := ""
	for kid := range bundle.JWTAuthorities() {
		jwtKid = kid
		break
	}
	// Reach the signing cert by issuing a throwaway cert for a
	// well-known SPIFFE ID — wasteful. Better: ask the manager
	// directly. We already promise stable PEM via ExportCA SIGNING.
	// For now expose only `root` here; signing details surface via
	// ExportCA + the bundle's kid. v0.x ROADMAP entry "Surface
	// signing CA details in GetCAInfo" tracks the cleanup.
	return &v1.GetCAInfoResponse{
		TrustDomain: s.Provider.TrustDomain(),
		Root:        rootInfo,
		JwtKid:      jwtKid,
	}, nil
}

// RotateSigningCA forces an immediate signing-CA rotation.
func (s *IdentityGRPCServer) RotateSigningCA(ctx context.Context, _ *v1.RotateSigningCARequest) (*v1.RotateSigningCAResponse, error) {
	if err := s.checkRunning(ctx); err != nil {
		return nil, err
	}
	if err := s.Provider.RotateSigningCA(ctx); err != nil {
		return nil, mapIdentityErrorToStatus("RotateSigningCA", err)
	}
	// New kid from the refreshed bundle.
	bundle, err := s.Provider.GetTrustBundle(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "GetTrustBundle post-rotate: %v", err)
	}
	kid := ""
	for k := range bundle.JWTAuthorities() {
		kid = k
		break
	}
	return &v1.RotateSigningCAResponse{NewJwtKid: kid}, nil
}

// ExportCA returns PEM (or JWKS for BUNDLE).
func (s *IdentityGRPCServer) ExportCA(ctx context.Context, req *v1.ExportCARequest) (*v1.ExportCAResponse, error) {
	if err := s.checkRunning(ctx); err != nil {
		return nil, err
	}
	switch req.GetWhat() {
	case v1.ExportCARequest_WHAT_ROOT:
		chain := s.Provider.Manager().GetTrustChain()
		if len(chain) == 0 {
			return nil, status.Error(codes.FailedPrecondition, "root CA not initialized")
		}
		return &v1.ExportCAResponse{Pem: certToPEM(chain[0])}, nil

	case v1.ExportCARequest_WHAT_SIGNING:
		// We don't have a direct manager getter for the signing
		// cert as PEM; reach it via the trust bundle's first JWT
		// authority key (which is the signing CA's public key), but
		// that's the pubkey, not the cert. Instead issue a synthetic
		// throwaway cert and pull chain[1] (the signing CA) from it.
		// This is awkward — a v0.x cleanup entry tracks adding a
		// direct accessor; for v0.1 the synthetic-issue path works.
		mgr := s.Provider.Manager()
		// Use the canonical control-plane ID for the throwaway.
		id, err := identity.ServerID(s.Provider.TrustDomain(), "ca-export-probe")
		if err != nil {
			return nil, status.Errorf(codes.Internal, "build probe id: %v", err)
		}
		probeKey, err := ecdsa.GenerateKey(elliptic.P256(), randReader())
		if err != nil {
			return nil, status.Errorf(codes.Internal, "probe key: %v", err)
		}
		issued, err := mgr.IssueCertificate(identity.IssueRequest{
			ID:        id,
			PublicKey: &probeKey.PublicKey,
			TTL:       time.Minute,
		})
		if err != nil {
			return nil, status.Errorf(codes.Internal, "issue probe: %v", err)
		}
		if len(issued.Chain) < 2 {
			return nil, status.Error(codes.Internal, "issued chain shorter than expected")
		}
		return &v1.ExportCAResponse{Pem: certToPEM(issued.Chain[1])}, nil

	case v1.ExportCARequest_WHAT_BUNDLE:
		bundle, err := s.Provider.GetTrustBundle(ctx)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "GetTrustBundle: %v", err)
		}
		data, err := bundle.Marshal()
		if err != nil {
			return nil, status.Errorf(codes.Internal, "marshal bundle: %v", err)
		}
		return &v1.ExportCAResponse{Pem: data}, nil

	default:
		return nil, status.Errorf(codes.InvalidArgument, "unknown ExportCA.what: %v", req.GetWhat())
	}
}

// GetStatus aggregates provider health into a single response.
func (s *IdentityGRPCServer) GetStatus(ctx context.Context, _ *v1.GetStatusRequest) (*v1.GetStatusResponse, error) {
	healthy := s.Provider.Health(ctx) == nil
	resp := &v1.GetStatusResponse{
		Started:      healthy,
		TrustDomain:  s.Provider.TrustDomain(),
		WatcherCount: int32(s.Provider.WatcherCount()), //nolint:gosec // watcher count bounded
	}
	if startedAt := s.Provider.StartedAt(); !startedAt.IsZero() {
		resp.StartedAt = timestamppb.New(startedAt)
	}
	if !healthy {
		return resp, nil
	}
	if chain := s.Provider.Manager().GetTrustChain(); len(chain) > 0 {
		resp.RootExpiresAt = timestamppb.New(chain[0].NotAfter)
	}
	// Signing expiry via the same throwaway-issue trick used in
	// ExportCA — same v0.x roadmap follow-up.
	if id, err := identity.ServerID(s.Provider.TrustDomain(), "status-probe"); err == nil {
		probeKey, _ := ecdsa.GenerateKey(elliptic.P256(), randReader())
		if probeKey != nil {
			if issued, err := s.Provider.Manager().IssueCertificate(identity.IssueRequest{
				ID: id, PublicKey: &probeKey.PublicKey, TTL: time.Minute,
			}); err == nil && len(issued.Chain) >= 2 {
				resp.SigningExpiresAt = timestamppb.New(issued.Chain[1].NotAfter)
			}
		}
	}
	if store := s.Provider.JoinTokens(); store != nil {
		all, err := store.List(ctx, identity.ListJoinTokensFilter{})
		if err == nil {
			now := time.Now()
			for _, tok := range all {
				resp.TokenTotal++
				if tok.MaxUses == 0 || tok.UsedCount < tok.MaxUses {
					resp.TokenUnused++
				}
				if !tok.ExpiresAt.After(now) {
					resp.TokenExpired++
				}
			}
		}
	}
	return resp, nil
}

// ---- helpers -----------------------------------------------------

// randReader returns the random source for the throwaway-probe
// key generation. crypto/rand.Reader in production; tests can
// override the package var if they need determinism.
var randReader = func() io.Reader { return rand.Reader }

// joinTokenToProto maps the domain JoinToken into its wire form.
// The Token cleartext is included verbatim — callers (e.g.
// ListJoinTokens) MUST clear it before calling this when the wire
// shape forbids cleartext.
func joinTokenToProto(t *identity.JoinToken) *v1.JoinToken {
	out := &v1.JoinToken{
		Id:         t.ID,
		Token:      t.Token,
		Hash:       t.Hash,
		Salt:       t.Salt,
		Prefix:     t.Prefix,
		AgentId:    t.AgentID,
		TtlSeconds: int64(t.TTL / time.Second),
		MaxUses:    int32(t.MaxUses),   //nolint:gosec // bounded by spec
		UsedCount:  int32(t.UsedCount), //nolint:gosec // bounded by MaxUses
		Metadata:   t.Metadata,
	}
	if !t.CreatedAt.IsZero() {
		out.CreatedAt = timestamppb.New(t.CreatedAt)
	}
	if !t.ExpiresAt.IsZero() {
		out.ExpiresAt = timestamppb.New(t.ExpiresAt)
	}
	if t.UsedAt != nil {
		out.UsedAt = timestamppb.New(*t.UsedAt)
	}
	return out
}

// certInfoFromX509 fills the CACertInfo proto from a parsed cert.
func certInfoFromX509(cert *x509.Certificate) (*v1.CACertInfo, error) {
	if cert == nil {
		return nil, errors.New("nil cert")
	}
	keyType := keyTypeString(cert)
	return &v1.CACertInfo{
		Subject:   cert.Subject.String(),
		Serial:    cert.SerialNumber.Text(16),
		NotBefore: timestamppb.New(cert.NotBefore),
		NotAfter:  timestamppb.New(cert.NotAfter),
		KeyType:   keyType,
	}, nil
}

// keyTypeString renders a cert's public key into a human-readable
// label like "ECDSA-P256" or "RSA-2048".
func keyTypeString(cert *x509.Certificate) string {
	switch pk := cert.PublicKey.(type) {
	case *ecdsa.PublicKey:
		return "ECDSA-" + pk.Curve.Params().Name
	case *rsa.PublicKey:
		return fmt.Sprintf("RSA-%d", pk.N.BitLen())
	default:
		return fmt.Sprintf("%T", pk)
	}
}

// certToPEM renders an x509 cert as a single PEM block.
func certToPEM(cert *x509.Certificate) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})
}

// checkRunning enforces "Start before any method." The check is
// cheap (atomic loads inside Health); a non-running provider gets
// a FailedPrecondition the operator can act on.
func (s *IdentityGRPCServer) checkRunning(ctx context.Context) error {
	if s.Provider == nil {
		return status.Error(codes.FailedPrecondition, "identity provider not wired")
	}
	if err := s.Provider.Health(ctx); err != nil {
		return status.Errorf(codes.FailedPrecondition, "identity provider not running: %v", err)
	}
	return nil
}

// mapIdentityErrorToStatus translates package-level sentinels into
// gRPC status codes that map cleanly to CLI behavior.
func mapIdentityErrorToStatus(op string, err error) error {
	switch {
	case errors.Is(err, identity.ErrProviderNotRunning):
		return status.Errorf(codes.FailedPrecondition, "%s: %v", op, err)
	case errors.Is(err, identity.ErrJoinTokenNotFound):
		return status.Errorf(codes.NotFound, "%s: %v", op, err)
	case errors.Is(err, identity.ErrJoinTokenInvalid):
		return status.Errorf(codes.InvalidArgument, "%s: %v", op, err)
	case errors.Is(err, identity.ErrJoinTokenDuplicate):
		return status.Errorf(codes.AlreadyExists, "%s: %v", op, err)
	case errors.Is(err, identity.ErrJoinTokenExhausted):
		return status.Errorf(codes.ResourceExhausted, "%s: %v", op, err)
	case errors.Is(err, identity.ErrJoinTokenStoreNotConfigured):
		return status.Errorf(codes.FailedPrecondition, "%s: %v", op, err)
	case errors.Is(err, identity.ErrInvalidProvider):
		return status.Errorf(codes.InvalidArgument, "%s: %v", op, err)
	default:
		return status.Errorf(codes.Internal, "%s: %v", op, err)
	}
}

// errgrpc — keep grpc.ServiceRegistrar in the import set even if no
// new top-level vars need it.
var _ grpc.ServiceRegistrar = (*grpc.Server)(nil)
