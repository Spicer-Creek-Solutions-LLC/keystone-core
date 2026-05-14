package controlplane

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"go.keystone-core.io/keystone-core/internal/identity"
	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

// ---- test fixture ------------------------------------------------

// identityFixture spins up an [identity.EmbeddedProvider] backed by
// an in-memory JoinTokenStore + a tempdir-backed FileCAStorage,
// wraps it in an [IdentityGRPCServer], and serves it over bufconn.
// Each test gets a hermetic instance.
type identityFixture struct {
	t        *testing.T
	provider *identity.EmbeddedProvider
	store    *identity.InMemoryJoinTokenStore
	server   *IdentityGRPCServer
	grpcSrv  *grpc.Server
	conn     *grpc.ClientConn
	client   v1.IdentityServiceClient
}

func newIdentityFixture(t *testing.T) *identityFixture {
	t.Helper()
	caStorage, err := identity.NewFileCAStorage(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileCAStorage: %v", err)
	}
	store := identity.NewInMemoryJoinTokenStore()
	cfg := identity.EmbeddedProviderConfig{
		CAConfig: shortLifetimeCAConfig(),
		Storage:  caStorage,
		// Long rotator interval — tests that need rotation invoke
		// it manually via RotateSigningCA RPC.
		RotatorInterval: time.Hour,
		JoinTokenStore:  store,
	}
	provider, err := identity.NewEmbeddedProvider(cfg)
	if err != nil {
		t.Fatalf("NewEmbeddedProvider: %v", err)
	}
	if err := provider.Start(context.Background()); err != nil {
		t.Fatalf("provider.Start: %v", err)
	}

	srv := NewIdentityGRPCServer(provider)
	listener := bufconn.Listen(1 << 20)
	grpcSrv := grpc.NewServer()
	v1.RegisterIdentityServiceServer(grpcSrv, srv)
	go func() {
		if err := grpcSrv.Serve(listener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			t.Logf("grpc serve: %v", err)
		}
	}()

	conn, err := grpc.NewClient(
		"passthrough://bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return listener.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("grpc dial: %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
		grpcSrv.Stop()
		_ = listener.Close()
		stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = provider.Stop(stopCtx)
	})

	return &identityFixture{
		t:        t,
		provider: provider,
		store:    store,
		server:   srv,
		grpcSrv:  grpcSrv,
		conn:     conn,
		client:   v1.NewIdentityServiceClient(conn),
	}
}

// shortLifetimeCAConfig is suitable for tests: 10h root, 2h
// signing, 30m rotation lead.
func shortLifetimeCAConfig() identity.CAConfig {
	c := identity.DefaultCAConfig(identity.DefaultTrustDomain)
	c.RootCATTL = 10 * time.Hour
	c.SigningCATTL = 2 * time.Hour
	c.RotateBefore = 30 * time.Minute
	c.DefaultSVIDTTL = 15 * time.Minute
	c.MaxSVIDTTL = time.Hour
	return c
}

// ---- CreateJoinToken --------------------------------------------

func TestIdentityGRPC_CreateJoinToken_HappyPath(t *testing.T) {
	t.Parallel()
	f := newIdentityFixture(t)
	resp, err := f.client.CreateJoinToken(t.Context(), &v1.CreateJoinTokenRequest{
		AgentId:    "agent-1",
		TtlSeconds: 600,
		MaxUses:    1,
		Metadata:   map[string]string{"role": "web"},
	})
	if err != nil {
		t.Fatalf("CreateJoinToken: %v", err)
	}
	tok := resp.GetToken()
	if tok.GetToken() == "" {
		t.Error("cleartext Token empty in response")
	}
	if !strings.HasPrefix(tok.GetToken(), identity.JoinTokenScheme) {
		t.Errorf("Token = %q, want %s… prefix", tok.GetToken(), identity.JoinTokenScheme)
	}
	if tok.GetAgentId() != "agent-1" {
		t.Errorf("AgentId = %q", tok.GetAgentId())
	}
	if tok.GetMaxUses() != 1 {
		t.Errorf("MaxUses = %d, want 1", tok.GetMaxUses())
	}
	// Persisted store has Token cleared.
	persisted, _ := f.store.Get(t.Context(), tok.GetId())
	if persisted.Token != "" {
		t.Errorf("persisted Token = %q, want empty", persisted.Token)
	}
}

func TestIdentityGRPC_CreateJoinToken_MissingAgentID(t *testing.T) {
	t.Parallel()
	f := newIdentityFixture(t)
	_, err := f.client.CreateJoinToken(t.Context(), &v1.CreateJoinTokenRequest{})
	if got := status.Code(err); got != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", got)
	}
}

func TestIdentityGRPC_CreateJoinToken_TTLOverMax(t *testing.T) {
	t.Parallel()
	f := newIdentityFixture(t)
	_, err := f.client.CreateJoinToken(t.Context(), &v1.CreateJoinTokenRequest{
		AgentId:    "agent-x",
		TtlSeconds: int64((identity.MaxJoinTokenTTL + time.Hour) / time.Second),
	})
	if got := status.Code(err); got != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", got)
	}
}

// ---- ListJoinTokens ---------------------------------------------

func TestIdentityGRPC_ListJoinTokens_NoCleartext(t *testing.T) {
	t.Parallel()
	f := newIdentityFixture(t)
	for i := 0; i < 3; i++ {
		if _, err := f.client.CreateJoinToken(t.Context(), &v1.CreateJoinTokenRequest{
			AgentId: "agent-list",
		}); err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
	}
	resp, err := f.client.ListJoinTokens(t.Context(), &v1.ListJoinTokensRequest{})
	if err != nil {
		t.Fatalf("ListJoinTokens: %v", err)
	}
	if len(resp.GetTokens()) != 3 {
		t.Errorf("len = %d, want 3", len(resp.GetTokens()))
	}
	for _, tok := range resp.GetTokens() {
		if tok.GetToken() != "" {
			t.Errorf("listed token has cleartext: %q", tok.GetToken())
		}
	}
}

func TestIdentityGRPC_ListJoinTokens_FilterByAgent(t *testing.T) {
	t.Parallel()
	f := newIdentityFixture(t)
	for _, id := range []string{"alpha", "beta"} {
		if _, err := f.client.CreateJoinToken(t.Context(), &v1.CreateJoinTokenRequest{
			AgentId: id,
		}); err != nil {
			t.Fatalf("Create %s: %v", id, err)
		}
	}
	resp, err := f.client.ListJoinTokens(t.Context(), &v1.ListJoinTokensRequest{
		AgentId: "alpha",
	})
	if err != nil {
		t.Fatalf("ListJoinTokens: %v", err)
	}
	if len(resp.GetTokens()) != 1 || resp.GetTokens()[0].GetAgentId() != "alpha" {
		t.Errorf("filtered = %v", resp.GetTokens())
	}
}

// ---- DeleteJoinToken --------------------------------------------

func TestIdentityGRPC_DeleteJoinToken_HappyPath(t *testing.T) {
	t.Parallel()
	f := newIdentityFixture(t)
	created, _ := f.client.CreateJoinToken(t.Context(), &v1.CreateJoinTokenRequest{AgentId: "a"})
	if _, err := f.client.DeleteJoinToken(t.Context(), &v1.DeleteJoinTokenRequest{
		Id: created.GetToken().GetId(),
	}); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

func TestIdentityGRPC_DeleteJoinToken_NotFound(t *testing.T) {
	t.Parallel()
	f := newIdentityFixture(t)
	_, err := f.client.DeleteJoinToken(t.Context(), &v1.DeleteJoinTokenRequest{Id: "missing"})
	if got := status.Code(err); got != codes.NotFound {
		t.Errorf("code = %v, want NotFound", got)
	}
}

func TestIdentityGRPC_DeleteJoinToken_MissingID(t *testing.T) {
	t.Parallel()
	f := newIdentityFixture(t)
	_, err := f.client.DeleteJoinToken(t.Context(), &v1.DeleteJoinTokenRequest{})
	if got := status.Code(err); got != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", got)
	}
}

// ---- CleanupJoinTokens ------------------------------------------

func TestIdentityGRPC_CleanupJoinTokens(t *testing.T) {
	t.Parallel()
	f := newIdentityFixture(t)
	// Pre-seed an expired token via the store directly (Create won't
	// accept a past ExpiresAt through the provider's clamp, but the
	// store does — this matches how a cleanup loop would see drift).
	now := time.Now().Truncate(time.Second)
	expired := identity.JoinToken{
		ID:        "exp-rpc",
		Hash:      []byte("hash"),
		Salt:      []byte("salt"),
		Prefix:    "kscore-join-EXPRPC01",
		AgentID:   "agent-cleanup",
		TTL:       time.Hour,
		CreatedAt: now.Add(-2 * time.Hour),
		ExpiresAt: now.Add(-time.Hour),
		MaxUses:   1,
	}
	if err := f.store.Create(t.Context(), expired); err != nil {
		t.Fatalf("seed: %v", err)
	}
	resp, err := f.client.CleanupJoinTokens(t.Context(), &v1.CleanupJoinTokensRequest{})
	if err != nil {
		t.Fatalf("CleanupJoinTokens: %v", err)
	}
	if resp.GetRemoved() != 1 {
		t.Errorf("removed = %d, want 1", resp.GetRemoved())
	}
}

// ---- GetCAInfo ---------------------------------------------------

func TestIdentityGRPC_GetCAInfo(t *testing.T) {
	t.Parallel()
	f := newIdentityFixture(t)
	resp, err := f.client.GetCAInfo(t.Context(), &v1.GetCAInfoRequest{})
	if err != nil {
		t.Fatalf("GetCAInfo: %v", err)
	}
	if resp.GetTrustDomain() != identity.DefaultTrustDomain {
		t.Errorf("TrustDomain = %q", resp.GetTrustDomain())
	}
	if resp.GetRoot() == nil {
		t.Fatal("Root nil")
	}
	if resp.GetRoot().GetKeyType() != "ECDSA-P-256" {
		t.Errorf("Root KeyType = %q, want ECDSA-P-256", resp.GetRoot().GetKeyType())
	}
	if resp.GetJwtKid() == "" {
		t.Error("JwtKid empty")
	}
}

// ---- RotateSigningCA --------------------------------------------

func TestIdentityGRPC_RotateSigningCA(t *testing.T) {
	t.Parallel()
	f := newIdentityFixture(t)
	before, _ := f.client.GetCAInfo(t.Context(), &v1.GetCAInfoRequest{})
	resp, err := f.client.RotateSigningCA(t.Context(), &v1.RotateSigningCARequest{})
	if err != nil {
		t.Fatalf("RotateSigningCA: %v", err)
	}
	after, _ := f.client.GetCAInfo(t.Context(), &v1.GetCAInfoRequest{})
	if resp.GetNewJwtKid() == "" {
		t.Error("NewJwtKid empty")
	}
	if resp.GetNewJwtKid() == before.GetJwtKid() {
		t.Error("JWT kid unchanged after rotation")
	}
	if after.GetJwtKid() != resp.GetNewJwtKid() {
		t.Error("RotateSigningCA returned different kid than GetCAInfo after")
	}
}

// ---- ExportCA ----------------------------------------------------

func TestIdentityGRPC_ExportCA_Root(t *testing.T) {
	t.Parallel()
	f := newIdentityFixture(t)
	resp, err := f.client.ExportCA(t.Context(), &v1.ExportCARequest{
		What: v1.ExportCARequest_WHAT_ROOT,
	})
	if err != nil {
		t.Fatalf("ExportCA root: %v", err)
	}
	block, _ := pem.Decode(resp.GetPem())
	if block == nil || block.Type != "CERTIFICATE" {
		t.Fatalf("PEM decode failed: %q", resp.GetPem())
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("ParseCertificate: %v", err)
	}
	if !cert.IsCA {
		t.Error("exported cert is not a CA")
	}
}

func TestIdentityGRPC_ExportCA_Signing(t *testing.T) {
	t.Parallel()
	f := newIdentityFixture(t)
	resp, err := f.client.ExportCA(t.Context(), &v1.ExportCARequest{
		What: v1.ExportCARequest_WHAT_SIGNING,
	})
	if err != nil {
		t.Fatalf("ExportCA signing: %v", err)
	}
	block, _ := pem.Decode(resp.GetPem())
	if block == nil || block.Type != "CERTIFICATE" {
		t.Fatalf("PEM decode failed")
	}
	cert, _ := x509.ParseCertificate(block.Bytes)
	if !cert.IsCA {
		t.Error("exported signing cert is not a CA")
	}
}

func TestIdentityGRPC_ExportCA_Bundle(t *testing.T) {
	t.Parallel()
	f := newIdentityFixture(t)
	resp, err := f.client.ExportCA(t.Context(), &v1.ExportCARequest{
		What: v1.ExportCARequest_WHAT_BUNDLE,
	})
	if err != nil {
		t.Fatalf("ExportCA bundle: %v", err)
	}
	if !strings.Contains(string(resp.GetPem()), `"keys"`) {
		t.Errorf("bundle output doesn't look like JWKS: %q", resp.GetPem())
	}
}

func TestIdentityGRPC_ExportCA_Unspecified(t *testing.T) {
	t.Parallel()
	f := newIdentityFixture(t)
	_, err := f.client.ExportCA(t.Context(), &v1.ExportCARequest{
		What: v1.ExportCARequest_WHAT_UNSPECIFIED,
	})
	if got := status.Code(err); got != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", got)
	}
}

// ---- GetStatus ---------------------------------------------------

func TestIdentityGRPC_GetStatus(t *testing.T) {
	t.Parallel()
	f := newIdentityFixture(t)
	// Seed two tokens (one live, one already-expired via store).
	if _, err := f.client.CreateJoinToken(t.Context(), &v1.CreateJoinTokenRequest{
		AgentId: "live-agent",
	}); err != nil {
		t.Fatalf("Create live: %v", err)
	}
	now := time.Now().Truncate(time.Second)
	expired := identity.JoinToken{
		ID: "exp-status", Hash: []byte("h"), Salt: []byte("s"),
		Prefix: "kscore-join-STATUS01", AgentID: "exp-agent",
		TTL: time.Hour, CreatedAt: now.Add(-2 * time.Hour),
		ExpiresAt: now.Add(-time.Hour), MaxUses: 1,
	}
	if err := f.store.Create(t.Context(), expired); err != nil {
		t.Fatalf("seed expired: %v", err)
	}

	resp, err := f.client.GetStatus(t.Context(), &v1.GetStatusRequest{})
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if !resp.GetStarted() {
		t.Error("Started = false")
	}
	if resp.GetTrustDomain() != identity.DefaultTrustDomain {
		t.Errorf("TrustDomain = %q", resp.GetTrustDomain())
	}
	if resp.GetStartedAt() == nil {
		t.Error("StartedAt nil")
	}
	if resp.GetTokenTotal() != 2 {
		t.Errorf("TokenTotal = %d, want 2", resp.GetTokenTotal())
	}
	if resp.GetTokenExpired() != 1 {
		t.Errorf("TokenExpired = %d, want 1", resp.GetTokenExpired())
	}
}

// ---- Provider-not-running ---------------------------------------

func TestIdentityGRPC_NilProvider(t *testing.T) {
	t.Parallel()
	srv := NewIdentityGRPCServer(nil)
	listener := bufconn.Listen(1 << 20)
	grpcSrv := grpc.NewServer()
	v1.RegisterIdentityServiceServer(grpcSrv, srv)
	go func() { _ = grpcSrv.Serve(listener) }()
	t.Cleanup(func() { grpcSrv.Stop() })

	conn, _ := grpc.NewClient("passthrough://bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return listener.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	client := v1.NewIdentityServiceClient(conn)

	// Every method should refuse cleanly.
	_, err := client.CreateJoinToken(t.Context(), &v1.CreateJoinTokenRequest{AgentId: "a"})
	if got := status.Code(err); got != codes.FailedPrecondition {
		t.Errorf("nil provider Create: code = %v", got)
	}
}
